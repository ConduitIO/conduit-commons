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
	enumSchema, err := avro.NewEnumSchema("enum_schema", "enum_namespace", []string{"val1", "val2", "val3"})
	if err != nil {
		panic(err)
	}
	bytes, err := NewAvroBuilder("schema_name", "schema_namespace").
		AddField("int_field", avro.NewPrimitiveSchema(avro.Int, nil), avro.WithDefault(100)).
		AddField("enum_field", enumSchema).
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
	//         "name": "enum_namespace.enum_schema",
	//         "symbols": [
	//           "val1",
	//           "val2",
	//           "val3"
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
