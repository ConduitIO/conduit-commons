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
	"fmt"
	"github.com/goccy/go-json"
	"github.com/hamba/avro/v2"
)

func ExampleAvroBuilder() {
	bytes, err := NewBuilder("schema_name", "schema_namespace").
		AddField("int_field", avro.NewPrimitiveSchema(avro.Int, nil), avro.WithDefault(100)).
		AddFieldWithSchemaBuilder(
			"enum_field",
			func() (avro.Schema, error) {
				return avro.NewEnumSchema("dept_schema", "namespace", []string{"finance", "legal", "eng"})
			},
		).
		MarshalJSON()

	if err != nil {
		panic(err)
	}

	prettyPrint(bytes)
	// Output:
	// {
	//   "fields": [
	//     {
	//       "default": 100,
	//       "name": "int_field",
	//       "type": "int"
	//     },
	//     {
	//       "name": "enum_field",
	//       "type": {
	//         "name": "namespace.dept_schema",
	//         "symbols": [
	//           "finance",
	//           "legal",
	//           "eng"
	//         ],
	//         "type": "enum"
	//       }
	//     }
	//   ],
	//   "name": "schema_namespace.schema_name",
	//   "type": "record"
	// }
}

func prettyPrint(bytes []byte) {
	m := map[string]interface{}{}
	err := json.Unmarshal(bytes, &m)
	if err != nil {
		panic(err)
	}

	pretty, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		panic(err)
	}

	fmt.Println(string(pretty))
}
