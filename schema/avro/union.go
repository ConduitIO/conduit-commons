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
	"github.com/iskorotkov/avro/v2"
	"github.com/modern-go/reflect2"
)

// avroArrayType is the Avro type name of an array, used as the union branch
// name for array values.
const avroArrayType = "array"

// avroMapType is the Avro type name of a map, used as the union branch name
// for map values.
const avroMapType = "map"

// unionResolver provides hooks before marshaling and after unmarshaling a value
// with an Avro schema, which make sure that values under the schema type Union
// are in the correct shape (see https://github.com/hamba/avro#unions --
// github.com/iskorotkov/avro/v2 is a fork and shares this union
// representation unchanged).
// NB: It currently supports union types nested in maps, but not nested in
// slices. For example, hooks will not work for values like []any{[]any{"foo"}}.
type unionResolver struct {
	// mapUnionPaths are all the paths to map fields within a schema,
	// where value types are union types
	mapUnionPaths []path
	// arrayUnionPaths are all the paths to array fields within a schema,
	// where value types are union types
	arrayUnionPaths []path
	// nullUnionPaths are all the paths to nullable fields within a schema.
	nullUnionPaths []path
	// wrappedNullUnions are the nullable unions whose non-null branch the
	// codec only encodes from its named form ({"branch": value}). See
	// beforeMarshalNullUnionSubstitutions.
	wrappedNullUnions []wrappedNullUnion
	resolver          *avro.TypeResolver
}

// wrappedNullUnion is a nullable union, ["null", T], where T is a record,
// map, enum or fixed. The codec decodes a value of T into its named form,
// which AfterUnmarshal unwraps into the bare value, but it cannot encode the
// bare value: it resolves a bare union value by its Go type, and a
// map[string]any, a string or a byte array does not identify T.
type wrappedNullUnion struct {
	parent path        // path to the value that holds the union, as in nullUnionPaths
	branch avro.Schema // T, with references resolved
	name   string      // the codec's name for T: the full name, or "map"
}

// newUnionResolver takes a schema and extracts the paths to all maps and arrays
// with union types. With this information the resolver can traverse the values
// in BeforeMarshal and AfterUnmarshal directly to the value that needs to be
// substituted.
func newUnionResolver(schema avro.Schema) unionResolver {
	var mapUnionPaths []path
	var arrayUnionPaths []path
	var nullUnionPaths []path
	var wrappedNullUnions []wrappedNullUnion
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
			if us, ok := p[len(p)-1].schema.(*avro.UnionSchema); ok {
				if branch, name, ok := wrappedNullUnionBranch(us); ok {
					wrappedNullUnions = append(wrappedNullUnions, wrappedNullUnion{parent: pCopy, branch: branch, name: name})
				}
			}
		}
	})
	return unionResolver{
		mapUnionPaths:     mapUnionPaths,
		arrayUnionPaths:   arrayUnionPaths,
		nullUnionPaths:    nullUnionPaths,
		wrappedNullUnions: wrappedNullUnions,
		resolver:          avro.NewTypeResolver(),
	}
}

// wrappedNullUnionBranch returns the non-null branch of the nullable union
// us and the codec's name for it, if values of that branch need to be
// wrapped before encoding.
func wrappedNullUnionBranch(us *avro.UnionSchema) (avro.Schema, string, bool) {
	for _, t := range us.Types() {
		if ref, ok := t.(*avro.RefSchema); ok {
			t = ref.Schema()
		}
		switch t := t.(type) {
		case *avro.RecordSchema:
			return t, t.FullName(), true
		case *avro.EnumSchema:
			return t, t.FullName(), true
		case *avro.FixedSchema:
			return t, t.FullName(), true
		case *avro.MapSchema:
			return t, avroMapType, true
		}
	}
	return nil, "", false
}

// AfterUnmarshal traverses the input value 'val' using the schema and finds all
// fields that are of the Avro Union type. In the input 'val', these Union type
// fields are represented as maps with a single key that contains the name of
// the type (e.g. map[string]any{"string":"foo"}). This function processes those
// maps and extracts the actual value from them (e.g. "foo"), replacing the map
// representation with the actual value.
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
	for _, nullUnionPath := range r.nullUnionPaths {
		// first collect all parents that contain a value that is nullable
		parentMaps, err := r.collectParentsForNullUnionPath(val, nullUnionPath)
		if err != nil {
			return nil, err
		}

		// nullUnionField is the fields that needs to be substitured.
		// It's the last leg in the path.
		nullUnionField := nullUnionPath[len(nullUnionPath)-1].field

		// Loop through collected parent maps and collect all substitutions.
		for _, parentMap := range parentMaps {
			// nullUnionField is nil if the field represents a key in a map.
			// In that case, all the values in the map need to be checked and substituted.
			if nullUnionField == nil {
				for key := range parentMap {
					sub, err := r.substitute(parentMap, key)
					if err != nil {
						return nil, err
					}
					// substitution not needed for this key, skip to next
					if sub == nil {
						continue
					}
					substitutions = append(substitutions, sub)
				}
				continue
			}
			// nullUnionField is not nil if it's a field within a record schema.
			// In that case, we only substitute that field.
			sub, err := r.substitute(parentMap, nullUnionField.Name())
			if err != nil {
				return nil, err
			}
			// substitution not needed for this key, skip to next
			if sub == nil {
				continue
			}
			substitutions = append(substitutions, sub)
		}
	}
	return substitutions, nil
}

// substitute substitutes maps inserted by the underlying codec's Unmarshal()
// function (github.com/iskorotkov/avro/v2, a fork of hamba/avro that shares
// this behavior unchanged) with actual values. The input map (returned by
// Unmarshal())
// contain values encoded as maps with a single key:value pair, where
// key is the type name (e.g. {"int":1}). We want to replace all these
// maps with the actual value (e.g. 1).
func (r unionResolver) substitute(parentMap map[string]any, name string) (substitution, error) {
	avroVal := parentMap[name]
	if avroVal == nil {
		// don't change nil values
		return nil, nil //nolint:nilnil // This is the expected behavior.
	}
	vmap, ok := avroVal.(map[string]any)
	if !ok {
		// if the value is not a map, it's not a nil value
		return nil, nil //nolint:nilnil // This is the expected behavior.
	}
	if len(vmap) != 1 {
		return nil, fmt.Errorf("expected single value for %s encoded as a map, got %d elements: %w", name, len(vmap), ErrSchemaValueMismatch)
	}

	// this is a map with a single value, store the substitution
	for _, actualVal := range vmap {
		return mapSubstitution{
			m:   parentMap,
			key: name,
			val: actualVal,
		}, nil
	}

	// we can reach this line only if we didn't return
	// the substitution from the loop above
	panic("substitution not returned (this is a bug in the code)")
}

// BeforeMarshal traverses the value using the schema and finds all values that
// have the Avro type Union. Those values need to be changed to a map with a
// single key that contains the name of the type. This function takes that value
// (e.g. "foo") and hoists it into a map (e.g. map[string]any{"string":"foo"}).
func (r unionResolver) BeforeMarshal(val any) error {
	if len(r.mapUnionPaths) == 0 && len(r.arrayUnionPaths) == 0 && len(r.wrappedNullUnions) == 0 {
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
	substitutions, err = r.beforeMarshalNullUnionSubstitutions(val, substitutions)
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

// beforeMarshalNullUnionSubstitutions is the reverse of
// afterUnmarshalNullUnionSubstitutions for the unions in wrappedNullUnions:
// it wraps each bare non-nil value v into {name: v}, so that a value
// Unmarshal returned encodes again with the same schema
// (ConduitIO/conduit-commons#306).
//
// Values already in the named form are left alone, so callers that build
// that form themselves keep working. For an enum or fixed branch a map is
// always the named form. For a record or map branch, see classify: a value
// that fits only one reading gets that reading, and a value that fits both
// is an error, because picking either could silently write the wrong data.
func (r unionResolver) beforeMarshalNullUnionSubstitutions(val any, substitutions []substitution) ([]substitution, error) {
	for _, u := range r.wrappedNullUnions {
		var parents []map[string]any
		err := traverseValue(val, u.parent, false, func(v any) {
			switch v := v.(type) {
			case map[string]any:
				parents = append(parents, v)
			case opencdc.StructuredData:
				parents = append(parents, v)
			case *map[string]any:
				parents = append(parents, *v)
			case *opencdc.StructuredData:
				parents = append(parents, *v)
			}
		})
		if err != nil {
			return nil, err
		}

		field := u.parent[len(u.parent)-1].field
		for _, parent := range parents {
			if field != nil {
				sub, err := u.wrap(parent, field.Name())
				if err != nil {
					return nil, fmt.Errorf("%s: %w", field.Name(), err)
				}
				if sub != nil {
					substitutions = append(substitutions, sub)
				}
				continue
			}
			// The union is the value type of a map: wrap every value.
			for key := range parent {
				sub, err := u.wrap(parent, key)
				if err != nil {
					return nil, fmt.Errorf("%s: %w", key, err)
				}
				if sub != nil {
					substitutions = append(substitutions, sub)
				}
			}
		}
	}
	return substitutions, nil
}

// wrap returns the substitution that puts parent[key] into the named form,
// or nil if it is nil or already in that form.
func (u wrappedNullUnion) wrap(parent map[string]any, key string) (substitution, error) {
	v, ok := parent[key]
	if !ok || v == nil {
		return nil, nil //nolint:nilnil // no substitution needed
	}
	named, err := u.classify(v)
	if err != nil || named {
		return nil, err
	}
	return mapSubstitution{m: parent, key: key, val: map[string]any{u.name: v}}, nil
}

// classify reports whether v is already in the named form ({name: value})
// rather than a bare value of the branch.
//
// For an enum or fixed branch a bare value is never a map, so any map is the
// named form. For a record or map branch both readings are maps, and v is
// read as:
//   - named, if it has one entry, keyed by the branch name, whose value
//     fits the branch (for a record: a map whose keys are all fields);
//   - bare, if it fits the branch itself (for a record: all keys are fields
//     and each field's value fits its type; for a map: every value fits the
//     map's value type);
//   - an error if it fits both, for example a record R with a single field
//     also named R that holds a record or map. Guessing either reading could
//     write the wrong data (invariant 6).
//
// A value that fits neither is left bare, and the codec reports the
// mismatch.
func (u wrappedNullUnion) classify(v any) (bool, error) {
	m, ok := asMap(v)
	if !ok {
		return false, nil
	}
	switch u.branch.(type) {
	case *avro.RecordSchema, *avro.MapSchema:
	default:
		return true, nil
	}
	named := false
	if len(m) == 1 {
		if inner, ok := m[u.name]; ok {
			named = fitsSchema(u.branch, inner)
		}
	}
	bare := fitsSchema(u.branch, v)
	if named && bare {
		return false, fmt.Errorf(
			"value %v of union branch %s is ambiguous: it reads both as the codec's named form {%q: value} "+
				"and as a bare value, and the schema cannot tell them apart: %w", v, u.name, u.name, ErrSchemaValueMismatch,
		)
	}
	return named, nil
}

// fitsSchema reports whether v has the shape of a value of s, in the form
// Marshal accepts. It is only used to tell the named form of a union value
// from a bare one, so it checks structure, not every primitive type: a
// record must have exactly its fields (every field without a default, and
// no others), a map's values and an array's items must fit, and anything
// that is not a record, map, array or union must not be a map or a list.
func fitsSchema(s avro.Schema, v any) bool {
	if ref, ok := s.(*avro.RefSchema); ok {
		s = ref.Schema()
	}
	switch s := s.(type) {
	case *avro.RecordSchema:
		return fitsRecord(s, v)
	case *avro.MapSchema:
		m, ok := asMap(v)
		if !ok {
			return false
		}
		for _, mv := range m {
			if !fitsSchema(s.Values(), mv) {
				return false
			}
		}
		return true
	case *avro.ArraySchema:
		a, ok := v.([]any)
		if !ok {
			_, isMap := asMap(v)
			return !isMap // a typed slice
		}
		for _, item := range a {
			if !fitsSchema(s.Items(), item) {
				return false
			}
		}
		return true
	case *avro.UnionSchema:
		return fitsUnion(s, v)
	case *avro.NullSchema:
		return v == nil
	}
	_, isMap := asMap(v)
	_, isList := v.([]any)
	return !isMap && !isList
}

func fitsRecord(s *avro.RecordSchema, v any) bool {
	m, ok := asMap(v)
	if !ok {
		return false
	}
	for k := range m {
		if recordField(s, k) == nil {
			return false
		}
	}
	for _, f := range s.Fields() {
		fv, ok := m[f.Name()]
		if !ok {
			if !f.HasDefault() {
				return false
			}
			continue
		}
		if !fitsSchema(f.Type(), fv) {
			return false
		}
	}
	return true
}

func fitsUnion(s *avro.UnionSchema, v any) bool {
	for _, t := range s.Types() {
		if fitsSchema(t, v) {
			return true
		}
	}
	// The named form of one of the branches.
	m, ok := asMap(v)
	if !ok || len(m) != 1 {
		return false
	}
	for name, inner := range m {
		if t, i := s.Types().Get(name); i >= 0 {
			return fitsSchema(t, inner)
		}
	}
	return false
}

func recordField(s *avro.RecordSchema, name string) *avro.Field {
	for _, f := range s.Fields() {
		if f.Name() == name {
			return f
		}
	}
	return nil
}

func asMap(v any) (map[string]any, bool) {
	switch v := v.(type) {
	case map[string]any:
		return v, true
	case opencdc.StructuredData:
		return v, true
	}
	return nil, false
}

func (r unionResolver) resolveNameForType(v any, us *avro.UnionSchema) (string, error) {
	var names []string

	t := reflect2.TypeOf(v)
	switch t.Kind() { //nolint:exhaustive // some types are not supported
	case reflect.Map:
		names = []string{avroMapType}
	case reflect.Slice:
		if !t.Type1().Elem().AssignableTo(byteType) { // []byte is handled differently
			names = []string{avroArrayType}
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

func (r unionResolver) collectParentsForNullUnionPath(val any, p path) ([]map[string]any, error) {
	var parentMaps []map[string]any
	err := traverseValue(val, p, true, func(v any) {
		switch v := v.(type) {
		case map[string]any:
			parentMaps = append(parentMaps, v)
		case *map[string]any:
			parentMaps = append(parentMaps, *v)
		case *opencdc.StructuredData:
			parentMaps = append(parentMaps, *v)
		}
	})

	return parentMaps, err
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
