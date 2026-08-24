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

package avro

import (
	"errors"
	"fmt"

	"github.com/iskorotkov/avro/v2"
)

// Builder builds avro.RecordSchema instances and marshals them into JSON.
// Builder accepts arguments for creating fields and creates them internally
// (i.e. a user doesn't need to create the fields).
// All errors will be returned as a joined error when marshaling the schema to JSON.
type Builder struct {
	errs      []error
	fields    []*avro.Field
	name      string
	namespace string
}

// NewBuilder constructs a new Builder and initializes it
// with the given name and namespace.
func NewBuilder(name, namespace string) *Builder {
	return &Builder{
		name:      name,
		namespace: namespace,
	}
}

// AddField adds a new field with the given name, schema and schema options.
// If creating the field returns an error, the error is saved, joined with
// other errors (if any), and returned when marshaling to JSON.
//
// Breaking change (github.com/iskorotkov/avro/v2 codec swap): typ and opts
// are avro.Schema/avro.SchemaOption from this package's codec dependency,
// which changed import path from github.com/hamba/avro/v2 to
// github.com/iskorotkov/avro/v2. Because Go requires exact type identity
// for a concrete value to satisfy an interface parameter's method set
// (not merely a structurally identical shape from a different package), a
// caller that imports github.com/hamba/avro/v2 directly and constructs an
// avro.Schema/avro.SchemaOption value to pass into this method no longer
// compiles after upgrading past this change -- the hamba-typed value's
// methods return hamba-defined named types (e.g. its own Type), which do
// not satisfy the fork's Schema interface even though the two packages'
// type shapes are identical (it is a fork). There is no known caller of
// this pattern within ConduitIO/* (grep confirms Builder is used only
// internally within this repository today), but this is a public,
// exported method signature and the break is real for any external
// consumer that does. Migration: replace the github.com/hamba/avro/v2
// import with github.com/iskorotkov/avro/v2 at the call site; the two
// packages' schema construction APIs (NewPrimitiveSchema, NewRecordSchema,
// NewUnionSchema, etc.) are drop-in compatible.
func (b *Builder) AddField(name string, typ avro.Schema, opts ...avro.SchemaOption) *Builder {
	f, err := avro.NewField(name, typ, opts...)
	if err != nil {
		b.errs = append(b.errs, fmt.Errorf("field %v: %w", name, err))
	} else {
		b.fields = append(b.fields, f)
	}

	return b
}

// Build builds the underlying schema.
// Errors that occurred while creating fields or constructing
// the schema will be returned as a joined error.
//
// Breaking change: the returned *avro.RecordSchema is now
// github.com/iskorotkov/avro/v2's type, not github.com/hamba/avro/v2's --
// see AddField's doc comment for the full reasoning and migration note.
func (b *Builder) Build() (*avro.RecordSchema, error) {
	if b.errs != nil {
		return nil, errors.Join(b.errs...)
	}

	schema, err := avro.NewRecordSchema(b.name, b.namespace, b.fields)
	if err != nil {
		return nil, fmt.Errorf("failed building schema: %w", err)
	}

	return schema, nil
}

// MarshalJSON marshals the underlying schema to JSON.
// Errors that occurred while creating fields, constructing
// the schema or marshaling it will be returned as a joined error.
func (b *Builder) MarshalJSON() ([]byte, error) {
	schema, err := b.Build()
	if err != nil {
		return nil, err
	}

	bytes, err := schema.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("failed marshaling schema to JSON: %w", err)
	}
	return bytes, nil
}
