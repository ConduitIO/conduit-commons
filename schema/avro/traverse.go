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
	"sort"

	"github.com/iskorotkov/avro/v2"
)

type (
	// path represents a path from the root to a certain field/type in an Avro schema.
	path []leg
	// leg is a single leg of a path.
	leg struct {
		// schema is the schema of the object that contains the below field
		// (i.e. it's not the schema of the field itself).
		schema avro.Schema
		field  *avro.Field
	}
)

// traverseSchema is a utility for traversing an avro schema and executing fn on
// every schema in the tree.
//
// A reference is followed unless the type it refers to is already being
// traversed further up the same path. Such a reference is a cycle (a
// recursive type like a linked list), and following it would never end
// (ConduitIO/conduit-commons#314). fn still sees the reference itself, but
// not the types below it, so callers see each recursive type expanded once
// per path.
//
// Expanding a type once per path still grows fast when recursive types
// refer to each other (n mutually recursive records expand to about n!
// paths), so traverseSchema stops after maxSchemaTraversal schemas and
// returns ErrSchemaTooComplex. The callers precompute from these paths and
// walk them on every Marshal, so an unbounded traversal is a denial of
// service from one schema.
func traverseSchema(s avro.Schema, fn func(path)) error {
	// open holds the record types on the current path.
	open := make(map[avro.Schema]bool)
	visited, exceeded := 0, false
	var traverse func(avro.Schema, path)
	traverse = func(s avro.Schema, p path) {
		if visited == maxSchemaTraversal {
			exceeded = true // this would be visit maxSchemaTraversal+1
			return
		}
		visited++
		p = append(p, leg{s, nil})
		fn(p)

		// traverse deeper into types that have nested types
		switch s := s.(type) {
		case *avro.MapSchema:
			traverse(s.Values(), p)
		case *avro.ArraySchema:
			traverse(s.Items(), p)
		case *avro.RefSchema:
			if open[s.Schema()] {
				return // a cycle back to a type on this path
			}
			traverse(s.Schema(), p)
		case *avro.RecordSchema:
			// A record can be reached again inline, not only through a
			// reference, when a reference led back into the schema that
			// defines it. Check here too, or the inner visit would clear
			// the outer one's entry when it returns.
			if open[s] {
				return
			}
			open[s] = true
			defer delete(open, s)
			fields := s.Fields()
			p = p[:len(p)-1]
			for _, field := range fields {
				p = append(p, leg{s, field})
				traverse(field.Type(), p)
				p = p[:len(p)-1]
			}
		case *avro.UnionSchema:
			for _, st := range s.Types() {
				traverse(st, p)
			}
		}
	}
	traverse(s, nil)
	if exceeded {
		return fmt.Errorf("%w: expanding its named types visits more than %d schemas "+
			"(records that refer to each other recursively are expanded once per path, which grows factorially); "+
			"reduce the number of records that refer to each other, or break a cycle by replacing a reference with "+
			"a non-recursive type", ErrSchemaTooComplex, maxSchemaTraversal)
	}
	return nil
}

// maxSchemaTraversal is the most schemas traverseSchema visits; a schema
// that needs more is rejected. Every schema in the tree counts once per path it is reached by.
// Real schemas, including large CDC schemas with hundreds of columns, visit
// a few thousand.
const maxSchemaTraversal = 100_000

// sortFn can be passed to traverse to deterministically sort fields in every
// record and types in every union.
func sortFn(p path) {
	switch s := p[len(p)-1].schema.(type) {
	case *avro.RecordSchema:
		fields := s.Fields()
		sort.Slice(fields, func(i, j int) bool {
			return fields[i].Name() < fields[j].Name()
		})
	case *avro.UnionSchema:
		schemas := s.Types()
		sort.Slice(schemas, func(i, j int) bool {
			return schemas[i].String() < schemas[j].String()
		})
	}
}
