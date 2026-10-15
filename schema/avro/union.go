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
	"github.com/iskorotkov/avro/v2"
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
// representation unchanged). AfterUnmarshal (union_unwrap.go) and
// BeforeMarshal (union_wrap.go) walk the schema together with the value and
// are each other's inverse.
type unionResolver struct {
	// mapUnionPaths are all the paths to map fields within a schema,
	// where value types are union types
	mapUnionPaths []path
	// arrayUnionPaths are all the paths to array fields within a schema,
	// where value types are union types
	arrayUnionPaths []path
	// nullUnionPaths are all the paths to nullable fields within a schema.
	nullUnionPaths []path
	// hasUnion is set if the schema has a union anywhere. BeforeMarshal
	// checks every union value, not only those AfterUnmarshal unwraps.
	hasUnion bool
	resolver *avro.TypeResolver
	// schema is the schema the resolver was built for. AfterUnmarshal and
	// BeforeMarshal walk it alongside the value.
	schema avro.Schema
}

// newUnionResolver takes a schema and extracts the paths to all maps and arrays
// with union types and to all nullable unions. AfterUnmarshal uses them, and
// BeforeMarshal uses hasUnion, only to skip schemas in which there is nothing
// to do.
func newUnionResolver(schema avro.Schema) (unionResolver, error) {
	var mapUnionPaths []path
	var arrayUnionPaths []path
	var nullUnionPaths []path
	hasUnion := false
	// traverse the schema and extract paths to all maps and arrays with a union
	// as the value type
	err := traverseSchema(schema, func(p path) {
		if p[len(p)-1].schema.Type() == avro.Union {
			hasUnion = true
		}
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
	if err != nil {
		return unionResolver{}, err
	}
	return unionResolver{
		mapUnionPaths:   mapUnionPaths,
		arrayUnionPaths: arrayUnionPaths,
		nullUnionPaths:  nullUnionPaths,
		hasUnion:        hasUnion,
		resolver:        avro.NewTypeResolver(),
		schema:          schema,
	}, nil
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
