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

	// References are the other registered schemas this schema refers to
	// (for Protobuf, the files it imports). They are resolved through the
	// Resolver given to SerdeWithResolver. References are not carried by
	// the schema.v1.Schema wire type: ToProto drops them and FromProto
	// clears them.
	References []Reference
}

// Reference is a schema's reference to another schema registered in the
// schema registry, as Confluent Schema Registry stores it.
type Reference struct {
	// Name is how the referencing schema refers to the other one. For
	// Protobuf it is the import path.
	Name string
	// Subject and Version identify the referenced schema in the registry.
	Subject string
	Version int
}

// Resolver fetches the schemas that other schemas reference, usually from
// the schema registry.
type Resolver interface {
	// ResolveReference returns the schema registered under ref.Subject and
	// ref.Version, including its own References. It must honor ctx.
	ResolveReference(ctx context.Context, ref Reference) (Schema, error)
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

// Serde returns the serde for the schema. It is SerdeWithResolver with
// context.Background() and no Resolver, so a schema with References fails
// to parse.
func (s Schema) Serde() (Serde, error) {
	return s.SerdeWithResolver(context.Background(), nil)
}

// SerdeWithResolver returns the serde for the schema, resolving its
// References through r. ctx bounds the parse, if this call is the one that
// runs it: cancelling ctx stops reference resolution and a Protobuf compile.
//
// Serdes are cached process-wide by schema type, fingerprint and references,
// and so are parse errors, except those that say nothing about the schema: a
// parse that ran out of time or was cancelled (an error matching
// context.DeadlineExceeded or context.Canceled), or failed to fetch a
// reference (protobuf.ErrReferenceResolve). Those are evicted at once, so the
// next call parses again instead of getting the cached error until the
// entry expires.
//
// Concurrent calls for the same schema share one parse, which runs with the
// first caller's ctx and Resolver. A caller that joined a parse which failed
// for one of the reasons above retries with its own, so another pipeline
// stopping does not fail this one.
func (s Schema) SerdeWithResolver(ctx context.Context, r Resolver) (Serde, error) {
	key := serdeCacheKey{typ: s.Type, fingerprint: s.Fingerprint(), references: referencesKey(s.References)}
	var (
		srd Serde
		err error
	)
	for range maxJoinedParseRetries + 1 {
		ran := false // whether this call's miss function ran the parse
		srd, err, _ = globalSerdeCache.Get(key, func() (Serde, error) {
			ran = true
			return s.parse(ctx, r)
		})
		if err == nil {
			return srd, nil
		}
		if !isTransientParseError(err) {
			return nil, err //nolint:wrapcheck // errors are already wrapped in the miss function
		}
		// The error must not be cached the way a deterministic parse error
		// is. Callers that joined this same load still get it; the next call
		// after the eviction starts a fresh parse.
		_, _, _ = globalSerdeCache.Delete(key) // only the eviction matters, not the evicted value
		if ran || ctx.Err() != nil {
			return nil, err //nolint:wrapcheck // errors are already wrapped in the miss function
		}
		// Joined someone else's parse, which failed for a reason of its own
		// (its context, its resolver). Retry with ours.
	}
	return nil, err //nolint:wrapcheck // errors are already wrapped in the miss function
}

// maxJoinedParseRetries bounds how often SerdeWithResolver retries after
// joining another caller's parse that failed transiently.
const maxJoinedParseRetries = 3

func (s Schema) parse(ctx context.Context, r Resolver) (Serde, error) {
	factory, ok := KnownSerdeFactories[s.Type]
	if !ok {
		return nil, fmt.Errorf("failed to get serde for schema type %s: %w", s.Type, ErrUnsupportedType)
	}
	srd, err := factory.Parse(ctx, s, r)
	if err != nil {
		return nil, fmt.Errorf("failed to parse schema of type %s: %w", s.Type, err)
	}
	return srd, nil
}

// isTransientParseError reports whether a parse error says nothing about the
// schema itself, so caching it would fail every later call for no reason.
func isTransientParseError(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, protobuf.ErrReferenceResolve)
}

// referencesKey encodes references for serdeCacheKey. Identical schema text
// with different references compiles to a different schema. Fields are
// length-prefixed so no two different lists encode the same.
func referencesKey(refs []Reference) string {
	if len(refs) == 0 {
		return ""
	}
	var b strings.Builder
	for _, ref := range refs {
		fmt.Fprintf(&b, "%d:%s%d:%s%d;", len(ref.Name), ref.Name, len(ref.Subject), ref.Subject, ref.Version)
	}
	return b.String()
}

// serdeCacheKey identifies a cached Serde. The fingerprint covers only the
// schema bytes, so the type has to be part of the key: the same bytes
// registered under two types must parse with each type's own factory, and
// must not share a Serde or a cached parse error. The same goes for the
// references, encoded by referencesKey.
//
// The Resolver is not part of the key: the cache assumes a subject and
// version name the same schema for every caller in the process.
type serdeCacheKey struct {
	typ         Type
	fingerprint uint64
	references  string
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
	// Parse parses the schema's textual representation (s.Bytes) into a
	// Serde. It resolves s.References through r, which may be nil when s
	// has none, and stops when ctx is cancelled. Types that don't support
	// references ignore r.
	Parse func(ctx context.Context, s Schema, r Resolver) (Serde, error)
	// SerdeForType returns a Schema that matches the structure of v.
	SerdeForType func(v any) (Serde, error)
}

// KnownSerdeFactories maps every supported schema Type to its SerdeFactory.
var KnownSerdeFactories = map[Type]SerdeFactory{
	TypeAvro: {
		Parse:        func(_ context.Context, s Schema, _ Resolver) (Serde, error) { return avro.Parse(s.Bytes) },
		SerdeForType: func(v any) (Serde, error) { return avro.SerdeForType(v) },
	},
	TypeProtobuf: {
		// This path can't pass per-call options, so the compile runs with
		// the package-level timeout and size cap
		// (protobuf.SetDefaultCompileTimeout, SetDefaultMaxSchemaSize).
		Parse: func(ctx context.Context, s Schema, r Resolver) (Serde, error) {
			var opts []protobuf.Option
			if len(s.References) > 0 {
				opts = append(opts, protobuf.WithReferences(toProtobufReferences(s.References), protobufResolveFunc(r)))
			}
			srd, err := protobuf.Parse(ctx, s.Bytes, opts...)
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

func toProtobufReferences(refs []Reference) []protobuf.Reference {
	out := make([]protobuf.Reference, len(refs))
	for i, ref := range refs {
		out[i] = protobuf.Reference{Name: ref.Name, Subject: ref.Subject, Version: ref.Version}
	}
	return out
}

// protobufResolveFunc adapts r for protobuf.Parse. A nil r gives a nil
// ResolveFunc, which protobuf.Parse reports as ErrReferenceResolve.
func protobufResolveFunc(r Resolver) protobuf.ResolveFunc {
	if r == nil {
		return nil
	}
	return func(ctx context.Context, ref protobuf.Reference) (protobuf.ReferencedSchema, error) {
		s, err := r.ResolveReference(ctx, Reference{Name: ref.Name, Subject: ref.Subject, Version: ref.Version})
		if err != nil {
			return protobuf.ReferencedSchema{}, err //nolint:wrapcheck // protobuf.Parse wraps it with the reference
		}
		return protobuf.ReferencedSchema{Text: s.Bytes, References: toProtobufReferences(s.References)}, nil
	}
}
