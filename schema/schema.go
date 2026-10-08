// Copyright © 2024 Meroxa, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:generate stringer -type=Type -linecomment

package schema

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/conduitio/conduit-commons/rabin"
	"github.com/conduitio/conduit-commons/schema/avro"
	"github.com/conduitio/conduit-commons/schema/protobuf"
	"github.com/twmb/go-cache/cache"
)

// Type identifies the format of a schema. Its numeric values are a wire
// contract: they match the schema.v1.Schema.Type enum in
// proto/schema/v1/schema.proto and are append-only. A reader built against an
// older version of this package carries an unknown value through unchanged
// and fails with ErrUnsupportedType when asked for a Serde (see
// proto_compat_test.go).
type Type int32

const (
	TypeAvro     Type = iota + 1 // avro
	TypeProtobuf                 // protobuf
)

type Schema struct {
	Subject string
	Version int
	ID      int
	Type    Type
	Bytes   []byte
}

// Marshal returns the encoded representation of v.
func (s Schema) Marshal(v any) ([]byte, error) {
	srd, err := s.Serde()
	if err != nil {
		return nil, err
	}
	out, err := srd.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data with schema %v:%v (id: %v): %w", s.Subject, s.Version, s.ID, err)
	}
	return out, nil
}

// Unmarshal parses encoded data and stores the result in the value pointed
// to by v. If v is nil or not a pointer, Unmarshal returns an error.
func (s Schema) Unmarshal(b []byte, v any) error {
	srd, err := s.Serde()
	if err != nil {
		return err
	}
	err = srd.Unmarshal(b, v)
	if err != nil {
		return fmt.Errorf("failed to unmarshal data with schema %v:%v (id: %v): %w", s.Subject, s.Version, s.ID, err)
	}
	return nil
}

// Fingerprint returns a unique 64 bit identifier for the schema.
func (s Schema) Fingerprint() uint64 {
	return rabin.Bytes(s.Bytes)
}

// Serde returns the serde for the schema. Serdes are cached process-wide by
// schema type and fingerprint, and so are parse errors, with one exception: a parse
// that failed because it ran out of time (an error matching
// context.DeadlineExceeded, such as a timed-out Protobuf compile) is evicted
// at once. The next call parses again, instead of every pipeline using the
// schema getting the cached timeout until the entry expires.
func (s Schema) Serde() (Serde, error) {
	key := serdeCacheKey{typ: s.Type, fingerprint: s.Fingerprint()}
	srd, err, _ := globalSerdeCache.Get(key, func() (Serde, error) {
		factory, ok := KnownSerdeFactories[s.Type]
		if !ok {
			return nil, fmt.Errorf("failed to get serde for schema type %s: %w", s.Type, ErrUnsupportedType)
		}
		srd, err := factory.Parse(s.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse schema of type %s: %w", s.Type, err)
		}
		return srd, nil
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			// A timeout says nothing about the schema, so it must not be
			// cached the way a deterministic parse error is. Callers that
			// joined this same load still get the timeout; the next call
			// after the eviction starts a fresh parse.
			_, _, _ = globalSerdeCache.Delete(key) // only the eviction matters, not the evicted value
		}
		return nil, err //nolint:wrapcheck // errors are already wrapped in the miss function
	}
	return srd, nil
}

// serdeCacheKey identifies a cached Serde. The fingerprint covers only the
// schema bytes, so the type has to be part of the key: the same bytes
// registered under two types must parse with each type's own factory, and
// must not share a Serde or a cached parse error.
type serdeCacheKey struct {
	typ         Type
	fingerprint uint64
}

// globalSerdeCache is a concurrency safe cache of serdes by schema type and
// fingerprint. Every process uses a global cache to avoid re-parsing the same
// schema multiple times. Since the cache is global, it is important to ensure
// that the cache is cleaned up periodically to avoid memory leaks (e.g. if a
// pipeline is stopped and the schemas it processed are no longer needed).
var globalSerdeCache = cache.New[serdeCacheKey, Serde](
	cache.AutoCleanInterval(time.Hour), // clean up every hour
	cache.MaxAge(4*time.Hour),          // expire entries after 4 hours
)

// Serde represents a serializer/deserializer.
type Serde interface {
	// Marshal returns the encoded representation of v.
	Marshal(v any) ([]byte, error)
	// Unmarshal parses encoded data and stores the result in the value pointed
	// to by v. If v is nil or not a pointer, Unmarshal returns an error.
	Unmarshal(b []byte, v any) error
	// String returns the textual representation of the schema used by this serde.
	String() string
}

// SerdeFactory constructs Serdes for one schema Type. Both fields of every
// entry in KnownSerdeFactories are non-nil: callers index the map and call
// either function directly, so a nil field would be a nil-func panic rather
// than an error. A Type that can't support one of the operations returns an
// error from it instead.
type SerdeFactory struct {
	// Parse takes the textual representation of the schema and parses it into
	// a Schema.
	Parse func([]byte) (Serde, error)
	// SerdeForType returns a Schema that matches the structure of v.
	SerdeForType func(v any) (Serde, error)
}

// KnownSerdeFactories maps every supported schema Type to its SerdeFactory.
var KnownSerdeFactories = map[Type]SerdeFactory{
	TypeAvro: {
		Parse:        func(s []byte) (Serde, error) { return avro.Parse(s) },
		SerdeForType: func(v any) (Serde, error) { return avro.SerdeForType(v) },
	},
	TypeProtobuf: {
		// This path can't pass per-call options, so the compile runs with
		// the package-level timeout (protobuf.SetDefaultCompileTimeout).
		Parse: func(s []byte) (Serde, error) {
			srd, err := protobuf.Parse(s)
			if err != nil {
				return nil, err //nolint:wrapcheck // Schema.Serde wraps it
			}
			return srd, nil
		},
		// Protobuf support is decode-only: a Protobuf schema can't be
		// inferred from a Go value (no field numbers, no message identity).
		// This must stay a function that returns an error, never nil; see
		// SerdeFactory.
		SerdeForType: func(any) (Serde, error) {
			return nil, fmt.Errorf("schema type %s: %w", TypeProtobuf, protobuf.ErrEncodingNotSupported)
		},
	},
}

// MarshalText returns the textual representation of the schema type.
func (t Type) MarshalText() ([]byte, error) {
	return []byte(t.String()), nil
}

// UnmarshalText parses the textual representation of the schema type.
func (t *Type) UnmarshalText(b []byte) error {
	if len(b) == 0 {
		return nil // empty string, do nothing
	}

	switch string(b) {
	case TypeAvro.String():
		*t = TypeAvro
	case TypeProtobuf.String():
		*t = TypeProtobuf
	default:
		// it's not a known type, but we also allow Type(int)
		valIntRaw := strings.TrimSuffix(strings.TrimPrefix(string(b), "Type("), ")")
		valInt, err := strconv.Atoi(valIntRaw)
		if err != nil {
			return fmt.Errorf("schema type %q: %w", b, ErrUnsupportedType)
		}
		*t = Type(valInt) //nolint:gosec // no risk of overflow
	}

	return nil
}
