// Copyright © 2026 Meroxa, Inc.
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
	"encoding/binary"
	"fmt"
	"reflect"

	"github.com/iskorotkov/avro/v2"
)

// This file contains a deterministic Avro encoder. The codec this package
// delegates to (github.com/iskorotkov/avro/v2) encodes an Avro map by ranging
// over the Go map, and Go randomizes map iteration order on every range, so
// the same value encodes to different bytes on each run. Every other part of
// the codec's encoder is deterministic: records write their fields in schema
// order, arrays write their items in slice order, and unions write the branch
// index the type resolver picks.
//
// The fix is scoped to maps only. A schema that does not contain a map
// anywhere is still encoded entirely by the codec (Serde.Marshal checks
// Serde.hasMap first), so this code is only reached for schemas with maps.
// It recurses through the container types that can hold a map -- record,
// array, union and reference -- and delegates every leaf (primitive, enum,
// fixed, and their logical types) back to the codec, whose encoding of a leaf
// is deterministic. The bytes are otherwise identical to the codec's: the
// same block framing (a size-prefixed block header per the codec's default
// configuration, a 100-element block length, and a terminating zero block) is
// reproduced, only with map entries emitted in sorted key order.
//
// A recursive named type is delegated back to the codec at the point the
// recursion is detected, so this encoder terminates exactly where the codec's
// deferred-encoder cache breaks the cycle. A recursive type that contains a
// map is therefore ordered by the codec (non-deterministically) inside the
// cycle; that is a pre-existing limitation of the codec, not something this
// package widens, and recursive types are rare enough that it is accepted.

// blockLength is the codec's default map/array block length, reproduced so the
// deterministic encoder's block framing matches the codec's byte for byte.
const blockLength = 100

// schemaContainsMap reports whether schema contains an Avro map anywhere below
// it, following named-type references. The result is memoized on Serde.hasMap.
func schemaContainsMap(schema avro.Schema) bool {
	return schemaContainsMapSeen(schema, make(map[avro.Schema]bool))
}

func schemaContainsMapSeen(schema avro.Schema, seen map[avro.Schema]bool) bool {
	if seen[schema] {
		return false // already on this path (a recursive reference)
	}
	seen[schema] = true

	switch s := schema.(type) {
	case *avro.MapSchema:
		return true
	case *avro.ArraySchema:
		return schemaContainsMapSeen(s.Items(), seen)
	case *avro.RecordSchema:
		for _, field := range s.Fields() {
			if schemaContainsMapSeen(field.Type(), seen) {
				return true
			}
		}
		return false
	case *avro.UnionSchema:
		for _, typ := range s.Types() {
			if schemaContainsMapSeen(typ, seen) {
				return true
			}
		}
		return false
	case *avro.RefSchema:
		return schemaContainsMapSeen(s.Schema(), seen)
	default:
		return false
	}
}

// marshalDeterministic returns the Avro encoding of v, with every map written
// in sorted key order. See the file comment for why this exists.
func (s *Serde) marshalDeterministic(v any) ([]byte, error) {
	w := avro.NewWriter(nil, 512, avro.WithWriterConfig(s.encodeAPI))
	if err := s.encodeValue(w, s.schema, v, make(map[avro.Schema]bool)); err != nil {
		return nil, err
	}
	if w.Error != nil {
		return nil, w.Error
	}
	return w.Buffer(), nil
}

// encodeValue writes the Avro encoding of v against schema into w. A subtree
// with no map is delegated to the codec wholesale (it is deterministic there);
// a subtree with a map is traversed here so the map can be sorted. path holds
// the schemas currently being encoded so a recursive reference is detected and
// delegated to the codec instead of recursing forever.
func (s *Serde) encodeValue(w *avro.Writer, schema avro.Schema, v any, path map[avro.Schema]bool) error {
	if !schemaContainsMap(schema) {
		return s.delegate(w, schema, v)
	}
	if path[schema] {
		return s.delegate(w, schema, v)
	}
	path[schema] = true
	defer delete(path, schema)

	switch sch := schema.(type) {
	case *avro.MapSchema:
		return s.encodeMap(w, sch, v, path)
	case *avro.ArraySchema:
		return s.encodeArray(w, sch, v, path)
	case *avro.RecordSchema:
		return s.encodeRecord(w, sch, v, path)
	case *avro.UnionSchema:
		return s.encodeUnion(w, sch, v, path)
	case *avro.RefSchema:
		return s.encodeValue(w, sch.Schema(), v, path)
	default:
		return s.delegate(w, schema, v)
	}
}

// delegate encodes a subtree with the codec, whose encoding of it is
// deterministic (the subtree contains no map) or, for a recursive reference,
// is the only terminating option.
func (s *Serde) delegate(w *avro.Writer, schema avro.Schema, v any) error {
	b, err := s.encodeAPI.Marshal(schema, v)
	if err != nil {
		return fmt.Errorf("failed to marshal avro value: %w", err)
	}
	_, _ = w.Write(b)
	return w.Error
}

// encodeMap writes an Avro map with its entries in sorted key order.
func (s *Serde) encodeMap(w *avro.Writer, m *avro.MapSchema, v any, path map[avro.Schema]bool) error {
	mv := derefValue(reflect.ValueOf(v))
	if !mv.IsValid() || mv.Kind() != reflect.Map {
		return fmt.Errorf("avro: cannot encode %T as an Avro map", v)
	}
	if mv.Type().Key().Kind() != reflect.String {
		// Non-string keys (e.g. a key type implementing TextMarshaler) cannot
		// be sorted here; leave them to the codec, which already encodes them.
		return s.delegate(w, m, v)
	}

	keys := sortedMapKeys(mv)
	for start := 0; start < len(keys); start += blockLength {
		end := min(start+blockLength, len(keys))

		// Encode this block into a scratch writer so its byte size is known
		// before the block header is written, mirroring the codec's framing.
		scratch := avro.NewWriter(nil, 512, avro.WithWriterConfig(s.encodeAPI))
		for _, key := range keys[start:end] {
			scratch.WriteString(key.String())
			if err := s.encodeValue(scratch, m.Values(), mv.MapIndex(key).Interface(), path); err != nil {
				return err
			}
		}
		if scratch.Error != nil {
			return scratch.Error
		}

		entries := scratch.Buffer()
		w.WriteBlockHeader(int64(end-start), int64(len(entries)))
		_, _ = w.Write(entries)
		if w.Error != nil {
			return w.Error
		}
	}

	w.WriteBlockHeader(0, 0)
	return w.Error
}

// encodeArray writes an Avro array with its items in slice order. It is only
// reached when the items contain a map, which is why it recurses instead of
// delegating to the codec.
func (s *Serde) encodeArray(w *avro.Writer, a *avro.ArraySchema, v any, path map[avro.Schema]bool) error {
	av := derefValue(reflect.ValueOf(v))
	if !av.IsValid() || (av.Kind() != reflect.Slice && av.Kind() != reflect.Array) {
		return fmt.Errorf("avro: cannot encode %T as an Avro array", v)
	}

	for start := 0; start < av.Len(); start += blockLength {
		end := min(start+blockLength, av.Len())

		scratch := avro.NewWriter(nil, 512, avro.WithWriterConfig(s.encodeAPI))
		for i := start; i < end; i++ {
			if err := s.encodeValue(scratch, a.Items(), av.Index(i).Interface(), path); err != nil {
				return err
			}
		}
		if scratch.Error != nil {
			return scratch.Error
		}

		items := scratch.Buffer()
		w.WriteBlockHeader(int64(end-start), int64(len(items)))
		_, _ = w.Write(items)
		if w.Error != nil {
			return w.Error
		}
	}

	w.WriteBlockHeader(0, 0)
	return w.Error
}

// encodeRecord writes an Avro record's fields in schema order. It is only
// reached when the record contains a map. Records built by SerdeForType are
// represented as map[string]any (opencdc.StructuredData); any other shape is
// delegated to the codec unchanged, which is deterministic except for map
// fields it holds -- a pre-existing limitation this package does not widen.
func (s *Serde) encodeRecord(w *avro.Writer, r *avro.RecordSchema, v any, path map[avro.Schema]bool) error {
	rv := derefValue(reflect.ValueOf(v))
	if !rv.IsValid() || rv.Kind() != reflect.Map || rv.Type().Key().Kind() != reflect.String {
		return s.delegate(w, r, v)
	}

	for _, field := range r.Fields() {
		fv := rv.MapIndex(reflect.ValueOf(field.Name()))
		if !fv.IsValid() {
			if field.HasDefault() {
				if err := s.encodeValue(w, field.Type(), field.Default(), path); err != nil {
					return err
				}
				continue
			}
			return fmt.Errorf("avro: record %s is missing required field %q", r.FullName(), field.Name())
		}
		if err := s.encodeValue(w, field.Type(), fv.Interface(), path); err != nil {
			return err
		}
	}
	return w.Error
}

// encodeUnion writes an Avro union: the branch index followed by the branch
// value. The branch is determined by asking the codec to encode the value (its
// resolution is deterministic and is what Marshal would otherwise have used),
// reading the leading index it wrote, and then re-encoding only when the
// resolved branch is (or contains) a map, which is the one case the codec
// cannot order deterministically.
func (s *Serde) encodeUnion(w *avro.Writer, u *avro.UnionSchema, v any, path map[avro.Schema]bool) error {
	enc, err := s.encodeAPI.Marshal(u, v)
	if err != nil {
		return fmt.Errorf("failed to marshal avro union value: %w", err)
	}

	idx, n := decodeUnionIndex(enc)
	if n == 0 || idx < 0 || idx >= len(u.Types()) {
		// The codec's output is the only option; this is not reachable for a
		// value the codec encoded successfully.
		_, _ = w.Write(enc)
		return w.Error
	}

	branch := u.Types()[idx]
	if !schemaContainsMap(branch) {
		// The branch is deterministic; keep the codec's bytes as-is.
		_, _ = w.Write(enc)
		return w.Error
	}

	w.WriteInt(int32(idx)) //nolint:gosec // idx is bounds-checked above: 0 <= idx < len(u.Types()), so it fits in int32
	return s.encodeValue(w, branch, unionBranchValue(branch, v), path)
}

// unionBranchValue returns the value to encode against the resolved union
// branch. A map/array union value was hoisted into a single-key
// map[string]any{"type": value} by unionResolver.BeforeMarshal, and a nullable
// branch may be behind a pointer; every other shape is the value itself.
func unionBranchValue(branch avro.Schema, v any) any {
	if branch.Type() == avro.Null {
		return nil
	}
	rv := reflect.ValueOf(v)
	if rv.IsValid() && rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil
		}
		return rv.Elem().Interface()
	}
	if m, ok := v.(map[string]any); ok && len(m) == 1 {
		for _, val := range m {
			return val
		}
	}
	return v
}

// derefValue follows pointers until it reaches a non-pointer or nil value.
func derefValue(rv reflect.Value) reflect.Value {
	for rv.IsValid() && rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return reflect.Value{}
		}
		rv = rv.Elem()
	}
	return rv
}

// decodeUnionIndex reads the leading branch-index varint from a union's Avro
// encoding and returns the index and the number of bytes it occupied.
func decodeUnionIndex(b []byte) (idx int, n int) {
	u, n := binary.Uvarint(b)
	if n <= 0 {
		return 0, n
	}
	// The codec writes the index with Writer.WriteInt, a zigzag varint.
	return int(int64(u>>1) ^ -int64(u&1)), n
}
