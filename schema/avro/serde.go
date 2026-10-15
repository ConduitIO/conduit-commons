// Copyright © 2023 Meroxa, Inc.
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

package avro

import (
	"fmt"

	"github.com/iskorotkov/avro/v2"
)

// Serde represents an Avro schema. It exposes methods for marshaling and
// unmarshalling data.
type Serde struct {
	schema        avro.Schema
	unionResolver unionResolver
	maxInputSize  int      // 0 = unlimited (default); see WithMaxInputSize
	decodeAPI     avro.API // built once at construction from maxInputSize; see limits.go
	encodeAPI     avro.API // this Serde's own encoder cache; see newEncodeAPI
}

// newEncodeAPI returns the codec configuration a Serde encodes with. It has
// the same settings as the codec's package-level DefaultConfig, so the bytes
// are the same, but its own encoder cache.
//
// Invariant 6 (schema handling never silently mangles data): a Serde must
// never encode with an encoder compiled for another schema. The codec caches
// compiled encoders by the fingerprint of a schema's string form, and the
// string form of a reference to a named type is only the name. So
// {"type":"array","items":"fx"} has one fingerprint no matter how fx is
// defined, and with one shared cache, a Serde whose fx is decimal(10,2)
// reused the encoder of a Serde whose fx is decimal(10,6) and wrote 1.5 as
// 15000 (ConduitIO/conduit-commons#305). Enum indexes and record field order
// went wrong the same way. Names are unique within one schema, so a cache
// that only ever sees one schema cannot mix them up.
func newEncodeAPI() avro.API {
	return avro.Config{}.Freeze()
}

// Marshal returns the Avro encoding of v. Marshal does not modify v, and is
// safe to call concurrently on values that share nested maps and slices as
// long as nobody else writes to them (ConduitIO/conduit-commons#342).
// Limitations:
// - Map keys need to be of type string,
// - Array values need to be of type uint8 (byte).
func (s *Serde) Marshal(v any) ([]byte, error) {
	// Invariant 6: the union wrapping happens on a copy, never on v.
	v = s.unionResolver.copyForWrap(v)
	root, subs, err := s.unionResolver.wrapSubstitutions(v)
	if err != nil {
		return nil, err
	}
	if root != nil {
		v = root // a top-level union value in the named form
	}
	for _, sub := range subs {
		sub.substitute()
	}
	bytes, err := s.encodeAPI.Marshal(s.schema, v)
	if err != nil {
		return nil, fmt.Errorf("could not marshal into avro: %w", err)
	}
	return bytes, nil
}

// Unmarshal parses the Avro encoded data and stores the result in the value
// pointed to by v. If v is nil or not a pointer, Unmarshal returns an error.
// Note that arrays and maps are unmarshalled into slices and maps with untyped
// values (i.e. []any and map[string]any). This is a limitation of the Avro
// library used for encoding/decoding the payload.
//
// By default Unmarshal enforces no input-size ceiling (see WithMaxInputSize
// for why, and for how to opt into one). It does always decode with
// non-zero array/map allocation ceilings (MaxSliceAllocSize,
// MaxMapAllocSize -- see limits.go's "Default allocation ceilings"),
// tightened further by any configured input-size ceiling. This package
// decodes with github.com/iskorotkov/avro/v2, a maintained fork adopted
// specifically because the previously used github.com/hamba/avro/v2 was
// archived carrying three unfixed decoder advisories (GO-2026-5046,
// GO-2026-5047, GO-2026-5048) reachable by decoding untrusted Avro bytes.
// The fork fixes the integer-overflow class (GO-2026-5047) unconditionally
// at the source; the array- and map-allocation ceilings this package sets
// are what close the other two (element-count-vs-byte-budget amplification
// and unbounded map cardinality, including through a destination map keyed
// by a type implementing encoding.TextUnmarshaler) -- neither is closed by
// the codec alone. See limits.go's package doc for the full, re-verified
// justification, including the table of what the codec swap fixes on its
// own versus what requires this package's Config values too.
//
// Failure leaves v in one of two different states depending on which check
// rejected the input, and callers must not assume either without checking
// the error: (1) rejection by the ErrInputTooLarge byte-length check above
// happens before the decoder is invoked at all, so v is left completely
// untouched, not partially written, not reset to a zero value; (2)
// rejection by the decoder itself -- including a MaxSliceAllocSize/
// MaxMapAllocSize ceiling firing partway through a record with multiple
// fields -- can leave v partially populated: fields decoded before the
// one that failed keep their decoded values; for a map[string]any
// destination, the field that failed is present as an untyped nil (the
// record decoder assigns the destination map key before attempting to
// decode its value), not omitted and not its real (possibly truncated)
// value; any field after it is never assigned at all. Both are "fail with
// an error, not silently mangled data" (invariant 6): the error is always
// non-nil when v is not guaranteed fully populated, so a caller that
// checks the error before using v is safe either way; the distinction
// matters only for a caller that (incorrectly) inspects v without
// checking err first. See TestUnmarshal_AllocCeilingRejection_LeavesPartialData
// in limits_test.go for the exact shape this produces.
func (s *Serde) Unmarshal(b []byte, v any) error {
	if s.maxInputSize > 0 && len(b) > s.maxInputSize {
		return fmt.Errorf("%w: %d bytes exceeds the %d byte limit", ErrInputTooLarge, len(b), s.maxInputSize)
	}
	err := s.decodeAPI.Unmarshal(s.schema, b, v)
	if err != nil {
		return fmt.Errorf("could not unmarshal from avro: %w", err)
	}
	err = s.unionResolver.AfterUnmarshal(v)
	if err != nil {
		return err
	}
	return nil
}

// String returns the canonical form of the schema.
func (s *Serde) String() string {
	return s.schema.String()
}

// sort fields in the schema. It can be used in tests to ensure the schemas can
// be compared.
func (s *Serde) sort() {
	// Only SerdeForType sorts, after newUnionResolver traversed the same
	// schema without hitting the limit, so this cannot fail.
	_ = traverseSchema(s.schema, sortFn)
}

// Parse parses a schema byte slice. By default the returned Serde enforces
// no input-size ceiling on Unmarshal; pass WithMaxInputSize to opt into
// one.
func Parse(text []byte, opts ...Option) (*Serde, error) {
	// Parse with a cache of this schema's own. avro.ParseBytes registers
	// every named type in the codec's process-wide DefaultSchemaCache, so a
	// schema could use a name it never defines and silently get whatever
	// another schema, parsed earlier in the process, defined under that name.
	// Invariant 6: a schema only ever means what its own text says.
	schema, err := avro.ParseBytesWithCache(text, "", &avro.SchemaCache{})
	if err != nil {
		return nil, fmt.Errorf("could not parse avro schema: %w", err)
	}
	o, err := resolveOptions(opts)
	if err != nil {
		return nil, err
	}
	ur, err := newUnionResolver(schema)
	if err != nil {
		return nil, fmt.Errorf("could not parse avro schema: %w", err)
	}
	// Note: We do not sort fields here because field order is significant in
	// Avro schemas. Sorting would alter the schema and change the output. In
	// SerdeForType, sorting ensures consistency when creating a schema from a
	// value. However, when using Parse, we must preserve the original field
	// order to match the schema definition.
	return &Serde{
		schema:        schema,
		unionResolver: ur,
		maxInputSize:  o.maxInputSize,
		decodeAPI:     buildDecodeAPI(schema, o),
		encodeAPI:     newEncodeAPI(),
	}, nil
}

// SerdeForType uses reflection to extract an Avro schema from v. Maps are
// regarded as structs. By default the returned Serde enforces no
// input-size ceiling on Unmarshal; pass WithMaxInputSize to opt into one.
func SerdeForType(v any, opts ...Option) (*Serde, error) {
	schema, err := extractor{}.Extract(v)
	if err != nil {
		return nil, err
	}
	o, err := resolveOptions(opts)
	if err != nil {
		return nil, err
	}
	ur, err := newUnionResolver(schema)
	if err != nil {
		return nil, err
	}
	s := &Serde{
		schema:        schema,
		unionResolver: ur,
		maxInputSize:  o.maxInputSize,
		decodeAPI:     buildDecodeAPI(schema, o),
		encodeAPI:     newEncodeAPI(),
	}
	// Sort fields to ensure consistent schema representation.
	s.sort()
	return s, nil
}
