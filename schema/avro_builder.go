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

package schema

import (
	"errors"
	"fmt"

	"github.com/hamba/avro/v2"
)

type Builder struct {
	errs      error
	fields    []*avro.Field
	name      string
	namespace string
}

func NewBuilder(name, namespace string) *Builder {
	return &Builder{
		name:      name,
		namespace: namespace,
	}
}

func (b *Builder) AddField(name string, typ avro.Schema, opts ...avro.SchemaOption) *Builder {
	f, err := avro.NewField(name, typ, opts...)
	if err != nil {
		b.errs = errors.Join(b.errs, fmt.Errorf("field %v: %w", name, err))
	} else {
		b.fields = append(b.fields, f)
	}

	return b
}

func (b *Builder) AddFieldWithSchemaBuilder(name string, schemaBuilder func() (avro.Schema, error), opts ...avro.SchemaOption) *Builder {
	typ, err := schemaBuilder()
	if err != nil {
		b.errs = errors.Join(b.errs, fmt.Errorf("field %v: %w", name, err))
		return b
	}

	f, err := avro.NewField(name, typ, opts...)
	if err != nil {
		b.errs = errors.Join(b.errs, fmt.Errorf("field %v: %w", name, err))
	} else {
		b.fields = append(b.fields, f)
	}

	return b
}

func (b *Builder) Build() (*avro.RecordSchema, error) {
	if b.errs != nil {
		return nil, b.errs
	}

	schema, err := avro.NewRecordSchema(b.name, b.namespace, b.fields)
	if err != nil {
		return nil, fmt.Errorf("failed building schema: %w", err)
	}
	return schema, nil
}

func (b *Builder) MarshalJSON() ([]byte, error) {
	if b.errs != nil {
		return nil, b.errs
	}

	schema, err := avro.NewRecordSchema(b.name, b.namespace, b.fields)
	if err != nil {
		return nil, fmt.Errorf("failed building schema: %w", err)
	}

	bytes, err := schema.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("failed marshaling schema to JSON: %w", err)
	}
	return bytes, nil
}
