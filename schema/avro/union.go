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
	"reflect"

	"github.com/conduitio/conduit-commons/opencdc"
	"github.com/hamba/avro/v2"
	"github.com/modern-go/reflect2"
)

// unionResolver provides hooks before marshaling and after unmarshaling a value
// with an Avro schema, which make sure that values under the schema type Union
// are in the correct shape (see https://github.com/hamba/avro#unions).
// NB: It currently supports union types nested in maps, but not nested in
// slices. For example, hooks will not work for values like []any{[]any{"foo"}}.
type unionResolver struct {
	mapUnionPaths   []path
	arrayUnionPaths []path
	nullUnionPaths  []path
	resolver        *avro.TypeResolver
}

// newUnionResolver takes a schema and extracts the paths to all maps and arrays
// with union types. With this information the resolver can traverse the values
// in BeforeMarshal and AfterUnmarshal directly to the value that needs to be
// substituted.
func newUnionResolver(schema avro.Schema) unionResolver {
	var mapUnionPaths []path
	var arrayUnionPaths []path
	var nullUnionPaths []path
	// traverse the schema and extract paths to all maps and arrays with a union
	// as the value type
	traverseSchema(schema, func(p path) {
		switch {
		case isMapUnion(p[len(p)-1].schema):
			// path points to a map with a union type, copy and store it
			pCopy := make(path, len(p))
			copy(pCopy, p)
			mapUnionPaths = append(mapUnionPaths, pCopy)
		case isArrayUnion(p[len(p)-1].schema):
			// path points to an array with a union type, copy and store it
			pCopy := make(path, len(p))
			copy(pCopy, p)
			arrayUnionPaths = append(arrayUnionPaths, pCopy)
		case isNullUnion(p[len(p)-1].schema):
			// path points to a null union, copy and store it
			pCopy := make(path, len(p)-1)
			copy(pCopy, p[:len(p)-1])
			nullUnionPaths = append(nullUnionPaths, pCopy)
		}
	})
	return unionResolver{
		mapUnionPaths:   mapUnionPaths,
		arrayUnionPaths: arrayUnionPaths,
		nullUnionPaths:  nullUnionPaths,
		resolver:        avro.NewTypeResolver(),
	}
}

// AfterUnmarshal traverses the value using the schema and finds all values that
// have the Avro type Union. Those values are unmarshaled into a map with a
// single key that contains the name of the type
// (e.g. map[string]any{"string":"foo"}). This function takes that map and
// extracts the actual value from it (e.g. "foo").
func (r unionResolver) AfterUnmarshal(val any) error {
	if len(r.mapUnionPaths) == 0 &&
		len(r.arrayUnionPaths) == 0 &&
		len(r.nullUnionPaths) == 0 {
		return nil // shortcut
	}

	substitutions, err := r.afterUnmarshalMapSubstitutions(val, nil)
	if err != nil {
		return err
	}
	substitutions, err = r.afterUnmarshalArraySubstitutions(val, substitutions)
	if err != nil {
		return err
	}
	substitutions, err = r.afterUnmarshalNullUnionSubstitutions(val, substitutions)
	if err != nil {
		return err
	}

	// We now have a list of substitutions, simply apply them.
	for _, sub := range substitutions {
		sub.substitute()
	}
	return nil
}

func (r unionResolver) afterUnmarshalMapSubstitutions(val any, substitutions []substitution) ([]substitution, error) {
	for _, p := range r.mapUnionPaths {
		// first collect all maps that have a union type in the schema
		var maps []map[string]any
		err := traverseValue(val, p, true, func(v any) {
			if mapUnion, ok := v.(map[string]any); ok {
				maps = append(maps, mapUnion)
			}
		})
		if err != nil {
			return nil, err
		}

		// Loop through collected maps and collect all substitutions. These maps
		// contain values encoded as maps with a single key:value pair, where
		// key is the type name (e.g. {"int":1}). We want to replace all these
		// maps with the actual value (e.g. 1).
		// We don't replace them in the loop, because we want to make sure all
		// maps actually contain only 1 value.
		for i, mapUnion := range maps {
			for k, v := range mapUnion {
				if v == nil {
					// do no change nil values
					continue
				}
				vmap, ok := v.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("expected map[string]any, got %T: %w", v, ErrSchemaValueMismatch)
				}
				if len(vmap) != 1 {
					return nil, fmt.Errorf("expected single value encoded as a map, got %d elements: %w", len(vmap), ErrSchemaValueMismatch)
				}

				// this is a map with a single value, store the substitution
				for _, actualVal := range vmap {
					substitutions = append(substitutions, mapSubstitution{
						m:   maps[i],
						key: k,
						val: actualVal,
					})
					break
				}
			}
		}
	}
	return substitutions, nil
}

func (r unionResolver) afterUnmarshalArraySubstitutions(val any, substitutions []substitution) ([]substitution, error) {
	for _, p := range r.arrayUnionPaths {
		// first collect all arrays that have a union type in the schema
		var arrays [][]any
		err := traverseValue(val, p, true, func(v any) {
			if arrayUnion, ok := v.([]any); ok {
				arrays = append(arrays, arrayUnion)
			}
		})
		if err != nil {
			return nil, err
		}

		// Loop through collected arrays and collect all substitutions. These
		// arrays contain values encoded as maps with a single key:value pair,
		// where key is the type name (e.g. {"int":1}). We want to replace all
		// these maps with the actual value (e.g. 1).
		// We don't replace them in the loop, because we want to make sure all
		// maps actually contain only 1 value.
		for i, arrayUnion := range arrays {
			for index, v := range arrayUnion {
				if v == nil {
					// do no change nil values
					continue
				}
				vmap, ok := v.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("expected map[string]any, got %T: %w", v, ErrSchemaValueMismatch)
				}
				if len(vmap) != 1 {
					return nil, fmt.Errorf("expected single value encoded as a map, got %d elements: %w", len(vmap), ErrSchemaValueMismatch)
				}

				// this is a map with a single value, store the substitution
				for _, actualVal := range vmap {
					substitutions = append(substitutions, arraySubstitution{
						a:     arrays[i],
						index: index,
						val:   actualVal,
					})
					break
				}
			}
		}
	}
	return substitutions, nil
}

func (r unionResolver) afterUnmarshalNullUnionSubstitutions(val any, substitutions []substitution) ([]substitution, error) {
	for _, p := range r.nullUnionPaths {
		// first collect all values that are nullable
		var maps []map[string]any
		err := traverseValue(val, p, true, func(v any) {
			switch v := v.(type) {
			case map[string]any:
				maps = append(maps, v)
			case *map[string]any:
				maps = append(maps, *v)
			case *opencdc.StructuredData:
				maps = append(maps, *v)
			}
		})
		if err != nil {
			return nil, err
		}

		// Loop through collected maps and collect all substitutions. These maps
		// contain values encoded as maps with a single key:value pair, where
		// key is the type name (e.g. {"int":1}). We want to replace all these
		// maps with the actual value (e.g. 1).
		// We don't replace them in the loop, because we want to make sure all
		// maps actually contain only 1 value.
		for i, mapUnion := range maps {
			for k, v := range mapUnion {
				if v == nil {
					// do no change nil values
					continue
				}
				vmap, ok := v.(map[string]any)
				if !ok {
					// if the value is not a map, it's not a nil value
					continue
				}
				if len(vmap) != 1 {
					return nil, fmt.Errorf("expected single value encoded as a map, got %d elements: %w", len(vmap), ErrSchemaValueMismatch)
				}

				// this is a map with a single value, store the substitution
				for _, actualVal := range vmap {
					substitutions = append(substitutions, mapSubstitution{
						m:   maps[i],
						key: k,
						val: actualVal,
					})
					break
				}
			}
		}
	}
	return substitutions, nil
}

// BeforeMarshal traverses the value using the schema and finds all values that
// have the Avro type Union. Those values need to be changed to a map with a
// single key that contains the name of the type. This function takes that value
// (e.g. "foo") and hoists it into a map (e.g. map[string]any{"string":"foo"}).
func (r unionResolver) BeforeMarshal(val any) error {
	if len(r.mapUnionPaths) == 0 && len(r.arrayUnionPaths) == 0 {
		return nil // shortcut
	}

	substitutions, err := r.beforeMarshalMapSubstitutions(val, nil)
	if err != nil {
		return err
	}
	substitutions, err = r.beforeMarshalArraySubstitutions(val, substitutions)
	if err != nil {
		return err
	}

	// We now have a list of substitutions, simply apply them.
	for _, sub := range substitutions {
		sub.substitute()
	}
	return nil
}

func (r unionResolver) beforeMarshalMapSubstitutions(val any, substitutions []substitution) ([]substitution, error) {
	for _, p := range r.mapUnionPaths {
		mapSchema, ok := p[len(p)-1].schema.(*avro.MapSchema)
		if !ok {
			return nil, fmt.Errorf("expected *avro.MapSchema, got %T: %w", p[len(p)-1].schema, ErrSchemaValueMismatch)
		}
		unionSchema, ok := mapSchema.Values().(*avro.UnionSchema)
		if !ok {
			return nil, fmt.Errorf("expected *avro.UnionSchema, got %T: %w", mapSchema.Values(), ErrSchemaValueMismatch)
		}

		// first collect all maps that have a union type in the schema
		var maps []map[string]any
		err := traverseValue(val, p, false, func(v any) {
			if mapUnion, ok := v.(map[string]any); ok {
				maps = append(maps, mapUnion)
			}
		})
		if err != nil {
			return nil, err
		}

		// Loop through collected maps and collect all substitutions. We want
		// to replace all non-nil values in these maps with maps that contain a
		// single value, the key corresponds to the resolved name.
		// We don't replace them in the loop, because we want to make sure all
		// type names can be resolved first.
		for i, mapUnion := range maps {
			for k, v := range mapUnion {
				if v == nil {
					// do no change nil values
					continue
				}
				valTypeName, err := r.resolveNameForType(v, unionSchema)
				if err != nil {
					return nil, err
				}
				substitutions = append(substitutions, mapSubstitution{
					m:   maps[i],
					key: k,
					val: map[string]any{valTypeName: v},
				})
			}
		}
	}
	return substitutions, nil
}

func (r unionResolver) beforeMarshalArraySubstitutions(val any, substitutions []substitution) ([]substitution, error) {
	for _, p := range r.arrayUnionPaths {
		arraySchema, ok := p[len(p)-1].schema.(*avro.ArraySchema)
		if !ok {
			return nil, fmt.Errorf("expected *avro.ArraySchema, got %T: %w", p[len(p)-1].schema, ErrSchemaValueMismatch)
		}
		unionSchema, ok := arraySchema.Items().(*avro.UnionSchema)
		if !ok {
			return nil, fmt.Errorf("expected *avro.UnionSchema, got %T: %w", arraySchema.Items(), ErrSchemaValueMismatch)
		}

		// first collect all array that have a union type in the schema
		var arrays [][]any
		err := traverseValue(val, p, false, func(v any) {
			if arrayUnion, ok := v.([]any); ok {
				arrays = append(arrays, arrayUnion)
			}
		})
		if err != nil {
			return nil, err
		}

		// Loop through collected arrays and collect all substitutions. We want
		// to replace all non-nil values in these arrays with maps that contain a
		// single value, the key corresponds to the resolved name.
		// We don't replace them in the loop, because we want to make sure all
		// type names can be resolved first.
		for i, arrayUnion := range arrays {
			for index, v := range arrayUnion {
				if v == nil {
					// do no change nil values
					continue
				}
				valTypeName, err := r.resolveNameForType(v, unionSchema)
				if err != nil {
					return nil, err
				}
				substitutions = append(substitutions, arraySubstitution{
					a:     arrays[i],
					index: index,
					val:   map[string]any{valTypeName: v},
				})
			}
		}
	}
	return substitutions, nil
}

func (r unionResolver) resolveNameForType(v any, us *avro.UnionSchema) (string, error) {
	var names []string

	t := reflect2.TypeOf(v)
	switch t.Kind() { //nolint:exhaustive // some types are not supported
	case reflect.Map:
		names = []string{"map"}
	case reflect.Slice:
		if !t.Type1().Elem().AssignableTo(byteType) { // []byte is handled differently
			names = []string{"array"}
			break
		}
		fallthrough
	default:
		var err error
		names, err = r.resolver.Name(t)
		if err != nil {
			return "", fmt.Errorf("could not resolve type name for %T: %w", v, err)
		}
	}

	for _, n := range names {
		_, pos := us.Types().Get(n)
		if pos > -1 {
			return n, nil
		}
	}
	return "", fmt.Errorf("can't resolve %v in union type %v: %w", names, us.String(), ErrSchemaValueMismatch)
}

func isMapUnion(schema avro.Schema) bool {
	s, ok := schema.(*avro.MapSchema)
	if !ok {
		return false
	}
	us, ok := s.Values().(*avro.UnionSchema)
	if !ok {
		return false
	}
	for _, s := range us.Types() {
		// at least one of the types in the union must be a map or array for this
		// to count as a map with a union type
		if s.Type() == avro.Array || s.Type() == avro.Map {
			return true
		}
	}
	return false
}

func isArrayUnion(schema avro.Schema) bool {
	s, ok := schema.(*avro.ArraySchema)
	if !ok {
		return false
	}
	us, ok := s.Items().(*avro.UnionSchema)
	if !ok {
		return false
	}
	for _, s := range us.Types() {
		// at least one of the types in the union must be a map or array for this
		// to count as a map with a union type
		if s.Type() == avro.Array || s.Type() == avro.Map {
			return true
		}
	}
	return false
}

func isNullUnion(schema avro.Schema) bool {
	s, ok := schema.(*avro.UnionSchema)
	if !ok {
		return false
	}
	if len(s.Types()) != 2 {
		return false
	}
	for _, s := range s.Types() {
		// at least one of the types in the union must be a map or array for this
		// to count as a map with a union type
		if s.Type() == avro.Null {
			return true
		}
	}
	return false
}

type substitution interface {
	substitute()
}

type mapSubstitution struct {
	m   map[string]any
	key string
	val any
}

func (s mapSubstitution) substitute() { s.m[s.key] = s.val }

type arraySubstitution struct {
	a     []any
	index int
	val   any
}

func (s arraySubstitution) substitute() { s.a[s.index] = s.val }
