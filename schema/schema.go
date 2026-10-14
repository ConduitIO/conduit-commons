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
	// Resolver given to SerdeWithResolver, and carried by the
	// schema.v1.Schema wire type (field 6), so a standalone processor
	// receives them too. Code that converts a Schema to or from
	// schema.v1.Schema must use ToProto and FromProto: copying the fields one
	// by one drops References.
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
//
// Parsed Serdes are cached process-wide, and results are shared across
// Resolvers: whatever the first
// caller's Resolver returned for a subject and version is what every later
// caller gets. A process must therefore use one Resolver identity, one
// registry, for all calls. Constructors of Resolvers that are exposed to
// configuration (the conduit protobuf.decode processor, PB-5) must enforce
// this, and must check that the schema they return has the requested subject,
// version and type, and map registry errors to status and error codes without
// passing the raw response body through.
//
// A Resolver must not call SerdeWithResolver for the schema being parsed (or
// one that leads back to it): that call waits for the parse it is part of,
// and deadlocks until its context ends.
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
// A schema with References and a nil r fails with ErrResolverRequired,
// without touching the cache.
//
// Serdes are cached process-wide by schema type, fingerprint and references,
// and so are parse errors, except those that say nothing about the schema: a
// parse that ran out of time or was cancelled (an error matching
// context.DeadlineExceeded or context.Canceled), or failed to fetch a
// reference (protobuf.ErrReferenceResolve). Those are not kept, so the
// next call parses again instead of getting the cached error until the
// entry expires.
//
// Reference resolution is also bounded by protobuf.DefaultResolveTimeout
// (30s for the whole walk); that and ctx's deadline apply, whichever is
// earlier. Running out of the resolution timeout is
// protobuf.ErrReferenceResolve, which is not kept.
//
// Concurrent calls for the same schema share one parse, which runs with the
// first caller's ctx and Resolver. A caller that joined a parse which failed
// for one of the reasons above retries with its own, so another pipeline
// stopping does not fail this one.
func (s Schema) SerdeWithResolver(ctx context.Context, r Resolver) (Serde, error) {
	if len(s.References) > 0 && r == nil {
		// Checked before the cache, and so never cached: a later call with
		// a Resolver must get a real parse, not this error. It is not a
		// transient failure either (nothing to retry without a Resolver).
		return nil, fmt.Errorf("failed to get serde for schema type %s with %d references: %w", s.Type, len(s.References), ErrResolverRequired)
	}
	key := serdeCacheKey{typ: s.Type, fingerprint: s.Fingerprint(), references: referencesKey(s.References)}
	var err error
	for range maxJoinedParseRetries + 1 {
		if beforeCacheGet != nil {
			beforeCacheGet()
		}
		var (
			srd Serde
			ran bool // whether this call ran the parse
		)
		srd, ran, err = globalSerdeCache.get(ctx, key, func() (Serde, error) { return s.parse(ctx, r) })
		if err == nil {
			return srd, nil
		}
		if !isTransientParseError(err) {
			return nil, err
		}
		// The cache did not keep the error, and callers that joined this
		// load get it too. If the parse was this call's, or this call's own
		// context ended, it is final.
		if ran || ctx.Err() != nil {
			return nil, err
		}
		// Joined someone else's parse, which failed for a reason of its own
		// (its context, its resolver). Retry with ours.
	}
	return nil, err
}

// beforeCacheGet, if set, runs before each cache lookup. Tests use it to
// order callers around a shared in-flight parse; it is not a supported
// extension point.
var beforeCacheGet func()

// maxJoinedParseRetries bounds how often SerdeWithResolver retries after
// joining another caller's parse that failed transiently.
const maxJoinedParseRetries = 3

func (s Schema) parse(ctx context.Context, r Resolver) (Serde, error) {
	factory, ok := KnownSerdeFactories[s.Type]
	if !ok {
		return nil, fmt.Errorf("failed to get serde for schema type %s: %w", s.Type, ErrUnsupportedType)
	}
	srd, err := factory.Parse(ctx, ParseInput{Bytes: s.Bytes, References: s.References}, r)
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
		errors.Is(err, protobuf.ErrReferenceResolve) ||
		errors.Is(err, errNilSerde)
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
// The Resolver is not part of the key. The cache assumes one schema registry
// per process: a subject and version name the same schema for every caller,
// whichever Resolver they pass.
type serdeCacheKey struct {
	typ         Type
	fingerprint uint64
	references  string
}

// globalSerdeCache caches Serdes by schema type, fingerprint and references.
// Every process uses a global cache to avoid re-parsing the same schema
// multiple times; entries expire after 4 hours and at most 4096 are kept, so
// schemas no pipeline uses any more are dropped. See serdeCache.
var globalSerdeCache = newSerdeCache(4*time.Hour, serdeCacheMaxEntries, time.Now)

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

// ParseInput is what SerdeFactory.Parse gets to build a Serde from: exactly
// the parts of a Schema that identify the cached result (the Type selects
// the factory). Subject, Version and ID are deliberately absent: the Serde
// is shared by every schema with the same type, bytes and references, so a
// Parse that depended on them would give the first caller's answer to the
// rest.
type ParseInput struct {
	Bytes      []byte
	References []Reference
}

// SerdeFactory constructs Serdes for one schema Type. Both fields of every
// entry in KnownSerdeFactories are non-nil: callers index the map and call
// either function directly, so a nil field would be a nil-func panic rather
// than an error. A Type that can't support one of the operations returns an
// error from it instead.
type SerdeFactory struct {
	// Parse parses the schema's textual representation (in.Bytes) into a
	// Serde. It resolves in.References through r, which may be nil when
	// there are none, and stops when ctx is cancelled. Types that don't
	// support references ignore r.
	Parse func(ctx context.Context, in ParseInput, r Resolver) (Serde, error)
	// SerdeForType returns a Schema that matches the structure of v.
	SerdeForType func(v any) (Serde, error)
}

// KnownSerdeFactories maps every supported schema Type to its SerdeFactory.
var KnownSerdeFactories = map[Type]SerdeFactory{
	TypeAvro: {
		Parse:        func(_ context.Context, in ParseInput, _ Resolver) (Serde, error) { return avro.Parse(in.Bytes) },
		SerdeForType: func(v any) (Serde, error) { return avro.SerdeForType(v) },
	},
	TypeProtobuf: {
		// This path can't pass per-call options, so the compile runs with
		// the package-level timeout and size cap
		// (protobuf.SetDefaultCompileTimeout, SetDefaultMaxSchemaSize).
		Parse: func(ctx context.Context, in ParseInput, r Resolver) (Serde, error) {
			var opts []protobuf.Option
			if len(in.References) > 0 {
				opts = append(opts, protobuf.WithReferences(toProtobufReferences(in.References), protobufResolveFunc(r)))
			}
			srd, err := protobuf.Parse(ctx, in.Bytes, opts...)
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
