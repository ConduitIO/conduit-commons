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
	"testing"

	"github.com/hamba/avro/v2"
	"github.com/matryer/is"
)

func TestAvroBuilder_Build(t *testing.T) {
	is := is.New(t)

	enumSchema, err := avro.NewEnumSchema("enum_schema", "enum_namespace", []string{"val1", "val2", "val3"})
	is.NoErr(err)

	idField, err := avro.NewField("int_field", avro.NewPrimitiveSchema(avro.Int, nil), avro.WithDefault(100))
	is.NoErr(err)

	enumField, err := avro.NewField("enum_field", enumSchema)
	is.NoErr(err)

	wantSchema, err := avro.NewRecordSchema(
		"schema_name",
		"schema_namespace",
		[]*avro.Field{idField, enumField},
	)
	is.NoErr(err)

	want, err := wantSchema.MarshalJSON()
	is.NoErr(err)

	got, err := NewAvroBuilder("schema_name", "schema_namespace").
		AddField("int_field", avro.NewPrimitiveSchema(avro.Int, nil), avro.WithDefault(100)).
		AddField("enum_field", enumSchema).
		MarshalJSON()
	is.NoErr(err)

	is.Equal(want, got)
}
