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
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/conduitio/conduit-commons/rabin"
	"github.com/conduitio/conduit-commons/schema/avro"
	"github.com/twmb/go-cache/cache"
)

type Type int32

const (
	TypeAvro Type = iota + 1 // avro
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

// Serde returns the serde for the schema.
func (s Schema) Serde() (Serde, error) {
	srd, err, _ := globalSerdeCache.Get(s.Fingerprint(), func() (Serde, error) {
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
		return nil, err //nolint:wrapcheck // errors are already wrapped in the miss function
	}
	return srd, nil
}

// globalSerdeCache is a concurrency safe cache of serdes by schema fingerprint.
// Every process uses a global cache to avoid re-parsing the same schema multiple
// times. Since the cache is global, it is important to ensure that the cache is
// cleaned up periodically to avoid memory leaks (e.g. if a pipeline is stopped
// and the schemas it processed are no longer needed).
var globalSerdeCache = cache.New[uint64, Serde](
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

type SerdeFactory struct {
	// Parse takes the textual representation of the schema and parses it into
	// a Schema.
	Parse func([]byte) (Serde, error)
	// SerdeForType returns a Schema that matches the structure of v.
	SerdeForType func(v any) (Serde, error)
}

var KnownSerdeFactories = map[Type]SerdeFactory{
	TypeAvro: {
		Parse:        func(s []byte) (Serde, error) { return avro.Parse(s) },
		SerdeForType: func(v any) (Serde, error) { return avro.SerdeForType(v) },
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
