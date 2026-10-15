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
	"fmt"

	"github.com/conduitio/conduit-commons/opencdc"
	"github.com/iskorotkov/avro/v2"
)

// unionPosition is where a union sits in its parent. It decides whether
// AfterUnmarshal unwraps the union's value (see unwrapsAt).
type unionPosition int

const (
	posTop unionPosition = iota
	posField
	posArrayItem
	posMapValue
)

// AfterUnmarshal replaces the codec's representation of union values with
// the values themselves. The codec decodes a non-null union value into a map
// with a single key, the branch name (e.g. {"R": {...}}), unless it can
// resolve every branch of the union to a Go type (primitives and logical
// types such as bytes.decimal), in which case the value is bare.
// AfterUnmarshal unwraps such a value to "foo" where unwrapsAt says so and
// leaves the named form everywhere else, which is the shape this package has
// always returned.
//
// It walks the schema and the value together, so every union is matched to
// its own wrapper, at any depth. The walk used to precompute schema paths
// and follow them through the value, but it stepped over the wrapper of a
// union that is not an array item or map value. Below such a union every
// leg was one level off, and a map key was dropped without an error
// (ConduitIO/conduit-commons#312).
//
// Invariant 6 (schema handling never silently mangles data): a wrapper that
// does not match the schema is an error, never skipped.
func (r unionResolver) AfterUnmarshal(val any) error {
	if len(r.mapUnionPaths) == 0 &&
		len(r.arrayUnionPaths) == 0 &&
		len(r.nullUnionPaths) == 0 {
		return nil // no union is unwrapped anywhere in this schema
	}

	var err error
	switch v := val.(type) {
	case *opencdc.StructuredData:
		if v != nil && *v != nil {
			_, err = unwrapUnions(r.schema, map[string]any(*v), posTop)
		}
	case *map[string]any:
		if v != nil && *v != nil {
			_, err = unwrapUnions(r.schema, *v, posTop)
		}
	case *any:
		if v != nil {
			*v, err = unwrapUnions(r.schema, *v, posTop)
		}
	case opencdc.StructuredData:
		_, err = unwrapUnions(r.schema, map[string]any(v), posTop)
	case map[string]any:
		_, err = unwrapUnions(r.schema, v, posTop)
	}
	return err
}

// unwrapUnions returns v, a value of schema s decoded by the codec, with
// union wrappers removed where unwrapsAt says so. Maps and slices are
// changed in place. A value whose Go type does not fit the schema is not
// codec output (callers may pass their own values) and is returned as is.
func unwrapUnions(s avro.Schema, v any, pos unionPosition) (any, error) {
	if v == nil {
		return v, nil
	}
	switch s := s.(type) {
	case *avro.RefSchema:
		return unwrapUnions(s.Schema(), v, pos)
	case *avro.RecordSchema:
		return v, unwrapRecord(s, v)
	case *avro.ArraySchema:
		return v, unwrapArray(s, v)
	case *avro.MapSchema:
		return v, unwrapMap(s, v)
	case *avro.UnionSchema:
		return unwrapUnion(s, v, pos)
	}
	return v, nil
}

func unwrapRecord(s *avro.RecordSchema, v any) error {
	m, ok := asStringMap(v)
	if !ok {
		return nil
	}
	for _, f := range s.Fields() {
		fv, ok := m[f.Name()]
		if !ok {
			continue
		}
		nv, err := unwrapUnions(f.Type(), fv, posField)
		if err != nil {
			return fmt.Errorf("%s: %w", f.Name(), err)
		}
		m[f.Name()] = nv
	}
	return nil
}

func unwrapArray(s *avro.ArraySchema, v any) error {
	a, ok := v.([]any)
	if !ok {
		return nil // a typed slice holds no union wrappers
	}
	for i, item := range a {
		nv, err := unwrapUnions(s.Items(), item, posArrayItem)
		if err != nil {
			return fmt.Errorf("[%d]: %w", i, err)
		}
		a[i] = nv
	}
	return nil
}

func unwrapMap(s *avro.MapSchema, v any) error {
	m, ok := asStringMap(v)
	if !ok {
		return nil // a typed map holds no union wrappers
	}
	for k, mv := range m {
		nv, err := unwrapUnions(s.Values(), mv, posMapValue)
		if err != nil {
			return fmt.Errorf("%s: %w", k, err)
		}
		m[k] = nv
	}
	return nil
}

func unwrapUnion(s *avro.UnionSchema, v any, pos unionPosition) (any, error) {
	w, ok := v.(map[string]any)
	if !ok {
		// A bare value: the codec resolved the branch itself, and such a
		// branch is never a record, map or array, so nothing below it needs
		// unwrapping either.
		return v, nil
	}
	if len(w) != 1 {
		return nil, fmt.Errorf("expected single value encoded as a map, got %d elements: %w", len(w), ErrSchemaValueMismatch)
	}
	for name, inner := range w {
		branch, idx := s.Types().Get(name)
		if idx < 0 {
			return nil, fmt.Errorf("union %s has no branch %q: %w", s.String(), name, ErrSchemaValueMismatch)
		}
		// A union never directly contains a union, so the branch's own
		// children decide their positions.
		ni, err := unwrapUnions(branch, inner, posTop)
		if err != nil {
			return nil, err
		}
		if unwrapsAt(s, pos) {
			return ni, nil
		}
		w[name] = ni
	}
	return w, nil
}

// unwrapsAt reports whether AfterUnmarshal unwraps a value of union us at
// position pos. These are the positions this package has always unwrapped:
// a nullable union (["null", T]) as a record field or map value, and a union
// with an array or map branch as an array item or map value.
func unwrapsAt(us *avro.UnionSchema, pos unionPosition) bool {
	switch pos {
	case posField:
		return isNullUnion(us)
	case posMapValue:
		return isNullUnion(us) || unionHasContainer(us)
	case posArrayItem:
		return unionHasContainer(us)
	case posTop:
		return false
	}
	return false
}

// unionHasContainer reports whether us has an array or map branch.
func unionHasContainer(us *avro.UnionSchema) bool {
	for _, t := range us.Types() {
		if t.Type() == avro.Array || t.Type() == avro.Map {
			return true
		}
	}
	return false
}

func asStringMap(v any) (map[string]any, bool) {
	switch v := v.(type) {
	case map[string]any:
		return v, true
	case opencdc.StructuredData:
		return v, true
	}
	return nil, false
}
