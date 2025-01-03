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
	"reflect"
	"testing"

	"github.com/conduitio/conduit-commons/opencdc"
	"github.com/google/go-cmp/cmp"
	"github.com/matryer/is"
)

func TestSerde_MarshalUnmarshalNullableFields(t *testing.T) {
	is := is.New(t)

	sd := opencdc.StructuredData{
		"appearance": map[string]interface{}{
			"mode":  "dark",
			"color": "purple",
		},
		"website": nil,
	}
	serde, err := SerdeForType(sd)
	is.NoErr(err)

	bytes, err := serde.Marshal(sd)
	is.NoErr(err)

	var structuredData opencdc.StructuredData
	err = serde.Unmarshal(bytes, &structuredData)
	is.NoErr(err)
}

func TestUnionResolver(t *testing.T) {
	is := is.New(t)

	testCases := []struct {
		name string
		have any
		want any
	}{{
		name: "string",
		have: "foo",
		want: map[string]any{"string": "foo"},
	}, {
		name: "int",
		have: 123,
		want: map[string]any{"long": 123},
	}, {
		name: "int32",
		have: int32(123),
		want: map[string]any{"int": int32(123)},
	}, {
		name: "bool",
		have: true,
		want: map[string]any{"boolean": true},
	}, {
		name: "float64",
		have: 1.23,
		want: map[string]any{"double": 1.23},
	}, {
		name: "float32",
		have: float32(1.23),
		want: map[string]any{"float": float32(1.23)},
	}, {
		name: "int64",
		have: int64(321),
		want: map[string]any{"long": int64(321)},
	}, {
		name: "[]byte",
		have: []byte{1, 2, 3, 4},
		want: map[string]any{"bytes": []byte{1, 2, 3, 4}},
	}, {
		name: "null",
		have: nil,
		want: nil,
	}, {
		name: "[]int",
		have: []int{1, 2, 3, 4},
		want: map[string]any{"array": []int{1, 2, 3, 4}},
	}, {
		name: "nil []bool",
		have: []bool(nil),
		want: map[string]any{"array": []bool(nil)},
	}}

	isSlice := func(a any) bool {
		if a == nil {
			return false
		}
		// returns true if the type is a slice and not a byte slice
		t := reflect.TypeOf(a)
		return t.Kind() == reflect.Slice && !t.Elem().AssignableTo(byteType)
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			is := is.New(t)

			newRecord := func() opencdc.StructuredData {
				sd := opencdc.StructuredData{
					"foo1": tc.have,
					"map1": map[string]any{
						"foo2": tc.have,
						"map2": map[string]any{
							"foo3": tc.have,
						},
					},
					"arr1": []any{
						tc.have,
						[]any{tc.have},
					},
				}
				return sd
			}
			want := opencdc.StructuredData{
				"foo1": tc.have, // normal field shouldn't change
				"map1": map[string]any{
					"foo2": tc.want,
					"map2": map[string]any{
						"map": map[string]any{
							"foo3": func() any {
								// if the original value is a slice, we consider
								// the type a union and wrap it in a map, otherwise
								// we keep the original value
								if isSlice(tc.have) {
									return tc.want
								}
								return tc.have
							}(),
						},
					},
				},
				"arr1": []any{
					tc.want,
					map[string]any{
						"array": []any{
							func() any {
								// if the original value is a slice, we consider
								// the type a union and wrap it in a map, otherwise
								// we keep the original value
								if isSlice(tc.have) {
									return tc.want
								}
								return tc.have
							}(),
						},
					},
				},
			}
			have := newRecord()

			serde, err := SerdeForType(have)
			is.NoErr(err)
			mur := newUnionResolver(serde.schema)

			// before marshal we should change the nested map
			err = mur.BeforeMarshal(have)
			is.NoErr(err)
			is.Equal("", cmp.Diff(want, have))

			// after unmarshal we should have the same record as at the start
			err = mur.AfterUnmarshal(have)
			is.NoErr(err)
			is.Equal("", cmp.Diff(newRecord(), have))
		})
	}
}
