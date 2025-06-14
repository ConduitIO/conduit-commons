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
	"errors"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/conduitio/conduit-commons/opencdc"
	"github.com/google/go-cmp/cmp"
	"github.com/hamba/avro/v2"
	"github.com/matryer/is"
)

func TestSerde_MarshalUnmarshal(t *testing.T) {
	now := time.Now().UTC()

	testCases := []struct {
		name string
		// haveValue is the value we use to extract the schema and which gets marshaled
		haveValue any
		// wantValue is the expected value we get when haveValue gets marshaled and unmarshalled
		wantValue any
		// wantSchema is the schema expected to be extracted from haveValue
		wantSchema avro.Schema
	}{{
		name:       "boolean",
		haveValue:  true,
		wantValue:  true,
		wantSchema: avro.NewPrimitiveSchema(avro.Boolean, nil),
	}, {
		name:      "boolean ptr (false)",
		haveValue: func() *bool { var v bool; return &v }(),
		wantValue: false, // ptr is unmarshalled into value
		wantSchema: must(avro.NewUnionSchema(
			[]avro.Schema{
				avro.NewPrimitiveSchema(avro.Boolean, nil),
				avro.NewPrimitiveSchema(avro.Null, nil),
			},
		)),
	}, {
		name:      "boolean ptr (nil)",
		haveValue: (*bool)(nil),
		wantValue: nil, // when unmarshalling we get an untyped nil
		wantSchema: must(avro.NewUnionSchema(
			[]avro.Schema{
				avro.NewPrimitiveSchema(avro.Boolean, nil),
				avro.NewPrimitiveSchema(avro.Null, nil),
			},
		)),
	}, {
		name:       "int",
		haveValue:  int(4525347614434344400),
		wantValue:  int64(4525347614434344400),
		wantSchema: avro.NewPrimitiveSchema(avro.Long, nil),
	}, {
		name:      "int ptr (0)",
		haveValue: func() *int { var v int; return &v }(),
		wantValue: int64(0), // ptr is unmarshalled into value
		wantSchema: must(avro.NewUnionSchema(
			[]avro.Schema{
				avro.NewPrimitiveSchema(avro.Long, nil),
				avro.NewPrimitiveSchema(avro.Null, nil),
			},
		)),
	}, {
		name:      "int ptr (nil)",
		haveValue: (*int)(nil),
		wantValue: nil, // when unmarshalling we get an untyped nil
		wantSchema: must(avro.NewUnionSchema(
			[]avro.Schema{
				avro.NewPrimitiveSchema(avro.Long, nil),
				avro.NewPrimitiveSchema(avro.Null, nil),
			},
		)),
	}, {
		name:       "int64",
		haveValue:  int64(1),
		wantValue:  int64(1),
		wantSchema: avro.NewPrimitiveSchema(avro.Long, nil),
	}, {
		name:      "int64 ptr (0)",
		haveValue: func() *int64 { var v int64; return &v }(),
		wantValue: int64(0), // ptr is unmarshalled into value
		wantSchema: must(avro.NewUnionSchema(
			[]avro.Schema{
				avro.NewPrimitiveSchema(avro.Long, nil),
				avro.NewPrimitiveSchema(avro.Null, nil),
			},
		)),
	}, {
		name:      "int64 ptr (nil)",
		haveValue: (*int64)(nil),
		wantValue: nil, // when unmarshalling we get an untyped nil
		wantSchema: must(avro.NewUnionSchema(
			[]avro.Schema{
				avro.NewPrimitiveSchema(avro.Long, nil),
				avro.NewPrimitiveSchema(avro.Null, nil),
			},
		)),
	}, {
		name:       "int32",
		haveValue:  int32(1),
		wantValue:  int(1),
		wantSchema: avro.NewPrimitiveSchema(avro.Int, nil),
	}, {
		name:      "int32 ptr (0)",
		haveValue: func() *int32 { var v int32; return &v }(),
		wantValue: int(0), // ptr is unmarshalled into value
		wantSchema: must(avro.NewUnionSchema(
			[]avro.Schema{
				avro.NewPrimitiveSchema(avro.Int, nil),
				avro.NewPrimitiveSchema(avro.Null, nil),
			},
		)),
	}, {
		name:      "int32 ptr (nil)",
		haveValue: (*int32)(nil),
		wantValue: nil, // when unmarshalling we get an untyped nil
		wantSchema: must(avro.NewUnionSchema(
			[]avro.Schema{
				avro.NewPrimitiveSchema(avro.Int, nil),
				avro.NewPrimitiveSchema(avro.Null, nil),
			},
		)),
	}, {
		name:       "int16",
		haveValue:  int16(1),
		wantValue:  int(1),
		wantSchema: avro.NewPrimitiveSchema(avro.Int, nil),
	}, {
		name:      "int16 ptr (0)",
		haveValue: func() *int16 { var v int16; return &v }(),
		wantValue: int(0), // ptr is unmarshalled into value
		wantSchema: must(avro.NewUnionSchema(
			[]avro.Schema{
				avro.NewPrimitiveSchema(avro.Int, nil),
				avro.NewPrimitiveSchema(avro.Null, nil),
			},
		)),
	}, {
		name:      "int16 ptr (nil)",
		haveValue: (*int16)(nil),
		wantValue: nil, // when unmarshalling we get an untyped nil
		wantSchema: must(avro.NewUnionSchema(
			[]avro.Schema{
				avro.NewPrimitiveSchema(avro.Int, nil),
				avro.NewPrimitiveSchema(avro.Null, nil),
			},
		)),
	}, {
		name:       "int8",
		haveValue:  int8(1),
		wantValue:  int(1),
		wantSchema: avro.NewPrimitiveSchema(avro.Int, nil),
	}, {
		name:      "int8 ptr (0)",
		haveValue: func() *int8 { var v int8; return &v }(),
		wantValue: int(0), // ptr is unmarshalled into value
		wantSchema: must(avro.NewUnionSchema(
			[]avro.Schema{
				avro.NewPrimitiveSchema(avro.Int, nil),
				avro.NewPrimitiveSchema(avro.Null, nil),
			},
		)),
	}, {
		name:      "int8 ptr (nil)",
		haveValue: (*int8)(nil),
		wantValue: nil, // when unmarshalling we get an untyped nil
		wantSchema: must(avro.NewUnionSchema(
			[]avro.Schema{
				avro.NewPrimitiveSchema(avro.Int, nil),
				avro.NewPrimitiveSchema(avro.Null, nil),
			},
		)),
	}, {
		name:       "uint32",
		haveValue:  uint32(1),
		wantValue:  int64(1),
		wantSchema: avro.NewPrimitiveSchema(avro.Long, nil),
	}, {
		name:      "uint32 ptr (0)",
		haveValue: func() *uint32 { var v uint32; return &v }(),
		wantValue: int64(0), // ptr is unmarshalled into value
		wantSchema: must(avro.NewUnionSchema(
			[]avro.Schema{
				avro.NewPrimitiveSchema(avro.Long, nil),
				avro.NewPrimitiveSchema(avro.Null, nil),
			},
		)),
	}, {
		name:      "uint32 ptr (nil)",
		haveValue: (*uint32)(nil),
		wantValue: nil, // when unmarshalling we get an untyped nil
		wantSchema: must(avro.NewUnionSchema(
			[]avro.Schema{
				avro.NewPrimitiveSchema(avro.Long, nil),
				avro.NewPrimitiveSchema(avro.Null, nil),
			},
		)),
	}, {
		name:       "uint16",
		haveValue:  uint16(1),
		wantValue:  int(1),
		wantSchema: avro.NewPrimitiveSchema(avro.Int, nil),
	}, {
		name:      "uint16 ptr (0)",
		haveValue: func() *uint16 { var v uint16; return &v }(),
		wantValue: int(0), // ptr is unmarshalled into value
		wantSchema: must(avro.NewUnionSchema(
			[]avro.Schema{
				avro.NewPrimitiveSchema(avro.Int, nil),
				avro.NewPrimitiveSchema(avro.Null, nil),
			},
		)),
	}, {
		name:      "uint16 ptr (nil)",
		haveValue: (*uint16)(nil),
		wantValue: nil, // when unmarshalling we get an untyped nil
		wantSchema: must(avro.NewUnionSchema(
			[]avro.Schema{
				avro.NewPrimitiveSchema(avro.Int, nil),
				avro.NewPrimitiveSchema(avro.Null, nil),
			},
		)),
	}, {
		name:       "uint8",
		haveValue:  uint8(1),
		wantValue:  int(1),
		wantSchema: avro.NewPrimitiveSchema(avro.Int, nil),
	}, {
		name:      "uint8 ptr (0)",
		haveValue: func() *uint8 { var v uint8; return &v }(),
		wantValue: int(0), // ptr is unmarshalled into value
		wantSchema: must(avro.NewUnionSchema(
			[]avro.Schema{
				avro.NewPrimitiveSchema(avro.Int, nil),
				avro.NewPrimitiveSchema(avro.Null, nil),
			},
		)),
	}, {
		name:      "uint8 ptr (nil)",
		haveValue: (*uint8)(nil),
		wantValue: nil, // when unmarshalling we get an untyped nil
		wantSchema: must(avro.NewUnionSchema(
			[]avro.Schema{
				avro.NewPrimitiveSchema(avro.Int, nil),
				avro.NewPrimitiveSchema(avro.Null, nil),
			},
		)),
	}, {
		name:       "float64",
		haveValue:  float64(1),
		wantValue:  float64(1),
		wantSchema: avro.NewPrimitiveSchema(avro.Double, nil),
	}, {
		name:      "float64 ptr (0)",
		haveValue: func() *float64 { var v float64; return &v }(),
		wantValue: float64(0), // ptr is unmarshalled into value
		wantSchema: must(avro.NewUnionSchema(
			[]avro.Schema{
				avro.NewPrimitiveSchema(avro.Double, nil),
				avro.NewPrimitiveSchema(avro.Null, nil),
			},
		)),
	}, {
		name:      "float64 ptr (nil)",
		haveValue: (*float64)(nil),
		wantValue: nil, // when unmarshalling we get an untyped nil
		wantSchema: must(avro.NewUnionSchema(
			[]avro.Schema{
				avro.NewPrimitiveSchema(avro.Double, nil),
				avro.NewPrimitiveSchema(avro.Null, nil),
			},
		)),
	}, {
		name:       "float32",
		haveValue:  float32(1),
		wantValue:  float32(1),
		wantSchema: avro.NewPrimitiveSchema(avro.Float, nil),
	}, {
		name:      "float32 ptr (0)",
		haveValue: func() *float32 { var v float32; return &v }(),
		wantValue: float32(0), // ptr is unmarshalled into value
		wantSchema: must(avro.NewUnionSchema(
			[]avro.Schema{
				avro.NewPrimitiveSchema(avro.Float, nil),
				avro.NewPrimitiveSchema(avro.Null, nil),
			},
		)),
	}, {
		name:      "float32 ptr (nil)",
		haveValue: (*float32)(nil),
		wantValue: nil, // when unmarshalling we get an untyped nil
		wantSchema: must(avro.NewUnionSchema(
			[]avro.Schema{
				avro.NewPrimitiveSchema(avro.Float, nil),
				avro.NewPrimitiveSchema(avro.Null, nil),
			},
		)),
	}, {
		name:      "big.Rat",
		haveValue: *big.NewRat(2, 5),
		wantValue: big.NewRat(2, 5),
		wantSchema: avro.NewPrimitiveSchema(
			avro.Bytes,
			avro.NewDecimalLogicalSchema(31, 10),
		),
	}, {
		name:      "big.Rat (with rounding)",
		haveValue: *big.NewRat(2, 3),
		// rounded to 10 decimals
		wantValue: big.NewRat(6666666666, 10000000000),
		wantSchema: avro.NewPrimitiveSchema(
			avro.Bytes,
			avro.NewDecimalLogicalSchema(31, 10),
		),
	}, {
		name:      "big.Rat (ptr)",
		haveValue: big.NewRat(2, 5),
		wantValue: big.NewRat(2, 5),
		wantSchema: must(avro.NewUnionSchema(
			[]avro.Schema{
				avro.NewPrimitiveSchema(avro.Null, nil),
				avro.NewPrimitiveSchema(
					avro.Bytes,
					avro.NewDecimalLogicalSchema(31, 10),
				),
			},
		)),
	}, {
		name:       "string",
		haveValue:  "1",
		wantValue:  "1",
		wantSchema: avro.NewPrimitiveSchema(avro.String, nil),
	}, {
		name:      "string ptr (empty)",
		haveValue: func() *string { var v string; return &v }(),
		wantValue: "", // ptr is unmarshalled into value
		wantSchema: must(avro.NewUnionSchema(
			[]avro.Schema{
				avro.NewPrimitiveSchema(avro.String, nil),
				avro.NewPrimitiveSchema(avro.Null, nil),
			},
		)),
	}, {
		name:      "string ptr (nil)",
		haveValue: (*string)(nil),
		wantValue: nil, // when unmarshalling we get an untyped nil
		wantSchema: must(avro.NewUnionSchema(
			[]avro.Schema{
				avro.NewPrimitiveSchema(avro.String, nil),
				avro.NewPrimitiveSchema(avro.Null, nil),
			},
		)),
	}, {
		name:       "[]byte",
		haveValue:  []byte{1, 2, 3},
		wantValue:  []byte{1, 2, 3},
		wantSchema: avro.NewPrimitiveSchema(avro.Bytes, nil),
	}, {
		name:       "[4]byte",
		haveValue:  [4]byte{1, 2, 3, 4},
		wantValue:  [4]byte{1, 2, 3, 4},
		wantSchema: must(avro.NewFixedSchema("record.foo", "", 4, nil)),
	}, {
		name:      "nil",
		haveValue: nil,
		wantValue: nil,
		wantSchema: must(avro.NewUnionSchema( // untyped nils default to nullable strings
			[]avro.Schema{
				avro.NewPrimitiveSchema(avro.String, nil),
				avro.NewPrimitiveSchema(avro.Null, nil),
			},
		)),
	}, {
		name:       "duration",
		haveValue:  time.Duration(12345678999),
		wantValue:  time.Duration(12345678000), // duration is truncated to milliseconds
		wantSchema: avro.NewPrimitiveSchema(avro.Long, avro.NewPrimitiveLogicalSchema(avro.TimeMicros)),
	}, {
		name:      "duration ptr (0)",
		haveValue: func() *time.Duration { var v time.Duration; return &v }(),
		wantValue: time.Duration(0), // ptr is unmarshalled into value
		wantSchema: must(avro.NewUnionSchema(
			[]avro.Schema{
				avro.NewPrimitiveSchema(avro.Long, avro.NewPrimitiveLogicalSchema(avro.TimeMicros)),
				avro.NewPrimitiveSchema(avro.Null, nil),
			},
		)),
	}, {
		name:      "duration ptr (nil)",
		haveValue: (*time.Duration)(nil),
		wantValue: nil, // when unmarshaling we get an untyped nil
		wantSchema: must(avro.NewUnionSchema(
			[]avro.Schema{
				avro.NewPrimitiveSchema(avro.Long, avro.NewPrimitiveLogicalSchema(avro.TimeMicros)),
				avro.NewPrimitiveSchema(avro.Null, nil),
			},
		)),
	}, {
		name:       "[]int",
		haveValue:  []int{1, 2, 3},
		wantValue:  []any{int64(1), int64(2), int64(3)},
		wantSchema: avro.NewArraySchema(avro.NewPrimitiveSchema(avro.Long, nil)),
	}, {
		name:      "[]any (with data)",
		haveValue: []any{1, "foo"},
		wantValue: []any{int64(1), "foo"},
		wantSchema: avro.NewArraySchema(must(avro.NewUnionSchema(
			[]avro.Schema{
				avro.NewPrimitiveSchema(avro.String, nil),
				avro.NewPrimitiveSchema(avro.Long, nil),
				avro.NewPrimitiveSchema(avro.Null, nil),
			},
		))),
	}, {
		name:      "[]any (no data)",
		haveValue: []any{},
		wantValue: []any(nil), // TODO: smells like a bug, should be []any{}
		wantSchema: avro.NewArraySchema(must(avro.NewUnionSchema( // empty slice values default to nullable strings
			[]avro.Schema{
				avro.NewPrimitiveSchema(avro.String, nil),
				avro.NewPrimitiveSchema(avro.Null, nil),
			},
		))),
	}, {
		name:       "[][]int",
		haveValue:  [][]int{{1}, {2, 3}},
		wantValue:  []any{[]any{int64(1)}, []any{int64(2), int64(3)}},
		wantSchema: avro.NewArraySchema(avro.NewArraySchema(avro.NewPrimitiveSchema(avro.Long, nil))),
	}, {
		name: "map[string]int",
		haveValue: map[string]int{
			"foo": 1,
			"bar": 2,
		},
		wantValue: map[string]any{ // all maps are unmarshalled into map[string]any
			"foo": int64(1),
			"bar": int64(2),
		},
		wantSchema: avro.NewMapSchema(avro.NewPrimitiveSchema(avro.Long, nil)),
	}, {
		name: "map[string]any (with primitive data)",
		haveValue: map[string]any{
			"foo":  "bar",
			"foo2": "bar2",
			"bar":  1,
			"baz":  true,
		},
		wantValue: map[string]any{
			"foo":  "bar",
			"foo2": "bar2",
			"bar":  int64(1),
			"baz":  true,
		},
		wantSchema: avro.NewMapSchema(must(avro.NewUnionSchema([]avro.Schema{
			&avro.NullSchema{},
			avro.NewPrimitiveSchema(avro.Long, nil),
			avro.NewPrimitiveSchema(avro.String, nil),
			avro.NewPrimitiveSchema(avro.Boolean, nil),
		}))),
	}, {
		name: "map[string]any (with primitive array)",
		haveValue: map[string]any{
			"foo":  "bar",
			"foo2": "bar2",
			"bar":  1,
			"baz":  []int{1, 2, 3},
		},
		wantValue: map[string]any{
			"foo":  "bar",
			"foo2": "bar2",
			"bar":  int64(1),
			"baz":  []any{int64(1), int64(2), int64(3)},
		},
		wantSchema: avro.NewMapSchema(must(avro.NewUnionSchema([]avro.Schema{
			&avro.NullSchema{},
			avro.NewPrimitiveSchema(avro.Long, nil),
			avro.NewPrimitiveSchema(avro.String, nil),
			avro.NewArraySchema(avro.NewPrimitiveSchema(avro.Long, nil)),
		}))),
	}, {
		name: "map[string]any (with union array)",
		haveValue: map[string]any{
			"foo":  "bar",
			"foo2": "bar2",
			"bar":  1,
			"baz":  []int{1, 2, 3},
			"baz2": []any{"foo", true},
		},
		wantValue: map[string]any{
			"foo":  "bar",
			"foo2": "bar2",
			"bar":  int64(1),
			"baz":  []any{int64(1), int64(2), int64(3)},
			"baz2": []any{"foo", true},
		},
		wantSchema: avro.NewMapSchema(must(avro.NewUnionSchema([]avro.Schema{
			&avro.NullSchema{},
			avro.NewPrimitiveSchema(avro.Long, nil),
			avro.NewPrimitiveSchema(avro.String, nil),
			avro.NewArraySchema(must(avro.NewUnionSchema([]avro.Schema{
				&avro.NullSchema{},
				avro.NewPrimitiveSchema(avro.Long, nil),
				avro.NewPrimitiveSchema(avro.String, nil),
				avro.NewPrimitiveSchema(avro.Boolean, nil),
			}))),
		}))),
	}, {
		name:      "map[string]any (no data)",
		haveValue: map[string]any{},
		wantValue: map[string]any{},
		wantSchema: avro.NewMapSchema(must(avro.NewUnionSchema([]avro.Schema{ // empty map values default to nullable strings
			&avro.NullSchema{},
			avro.NewPrimitiveSchema(avro.String, nil),
		}))),
	}, {
		name: "map[string]any (nested)",
		haveValue: map[string]any{
			"foo": map[string]any{
				"bar": "baz",
				"baz": 1,
			},
		},
		wantValue: map[string]any{
			"foo": map[string]any{
				"bar": "baz",
				"baz": int64(1),
			},
		},
		wantSchema: avro.NewMapSchema(must(avro.NewUnionSchema([]avro.Schema{
			&avro.NullSchema{},
			avro.NewMapSchema(must(avro.NewUnionSchema([]avro.Schema{
				&avro.NullSchema{},
				avro.NewPrimitiveSchema(avro.Long, nil),
				avro.NewPrimitiveSchema(avro.String, nil),
			}))),
		}))),
	}, {
		name: "opencdc.StructuredData",
		haveValue: opencdc.StructuredData{
			"foo": "bar",
			"bar": 1,
			"baz": []int{1, 2, 3},
			"tz":  now,
		},
		wantValue: map[string]any{ // structured data is unmarshalled into a map
			"foo": "bar",
			"bar": int64(1),
			"baz": []any{int64(1), int64(2), int64(3)},
			"tz":  now.Truncate(time.Microsecond), // Avro cannot does not support nanoseconds
		},
		wantSchema: must(avro.NewRecordSchema(
			"record.foo",
			"",
			[]*avro.Field{
				must(avro.NewField("foo", avro.NewPrimitiveSchema(avro.String, nil))),
				must(avro.NewField("bar", avro.NewPrimitiveSchema(avro.Long, nil))),
				must(avro.NewField("baz", avro.NewArraySchema(avro.NewPrimitiveSchema(avro.Long, nil)))),
				must(avro.NewField("tz", avro.NewPrimitiveSchema(avro.Long, avro.NewPrimitiveLogicalSchema(avro.TimestampMicros)))),
			},
		)),
	}}

	newRecord := func(v any) opencdc.StructuredData {
		return opencdc.StructuredData{"foo": v}
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			is := is.New(t)

			// create a new record with haveValue in the field "foo"
			haveValue := newRecord(tc.haveValue)

			// extract serde and ensure it matches the expectation
			gotSerde, err := SerdeForType(haveValue)
			is.NoErr(err)

			wantSerde := &Serde{
				schema: must(avro.NewRecordSchema("record", "",
					[]*avro.Field{must(avro.NewField("foo", tc.wantSchema))},
				)),
			}
			wantSerde.sort()
			is.Equal("", cmp.Diff(wantSerde.String(), gotSerde.String()))

			// now try to marshal the value with the schema
			bytes, err := gotSerde.Marshal(haveValue)
			is.NoErr(err)

			// unmarshal the bytes back into structured data and compare the value
			var gotValue opencdc.StructuredData
			err = gotSerde.Unmarshal(bytes, &gotValue)
			is.NoErr(err)

			wantValue := newRecord(tc.wantValue)
			is.Equal("", cmp.Diff(wantValue, gotValue, cmp.Comparer(func(x, y *big.Rat) bool {
				return x.Cmp(y) == 0
			})))
		})
	}
}

func TestSerdeForType_NestedStructuredData(t *testing.T) {
	is := is.New(t)

	have := opencdc.StructuredData{
		"foo": "bar",
		"level1": opencdc.StructuredData{
			"foo": "bar",
			"level2": opencdc.StructuredData{
				"foo": "bar",
				"level3": opencdc.StructuredData{
					"foo":        "bar",
					"regularMap": map[string]bool{},
				},
			},
		},
	}

	want := &Serde{schema: must(avro.NewRecordSchema(
		"record", "",
		[]*avro.Field{
			must(avro.NewField("foo", avro.NewPrimitiveSchema(avro.String, nil))),
			must(avro.NewField("level1",
				must(avro.NewRecordSchema(
					"record.level1", "",
					[]*avro.Field{
						must(avro.NewField("foo", avro.NewPrimitiveSchema(avro.String, nil))),
						must(avro.NewField("level2",
							must(avro.NewRecordSchema(
								"record.level1.level2", "",
								[]*avro.Field{
									must(avro.NewField("foo", avro.NewPrimitiveSchema(avro.String, nil))),
									must(avro.NewField("level3",
										must(avro.NewRecordSchema(
											"record.level1.level2.level3", "",
											[]*avro.Field{
												must(avro.NewField("foo", avro.NewPrimitiveSchema(avro.String, nil))),
												must(avro.NewField("regularMap", avro.NewMapSchema(
													avro.NewPrimitiveSchema(avro.Boolean, nil),
												))),
											},
										)),
									)),
								},
							)),
						)),
					},
				)),
			)),
		},
	))}
	want.sort()

	got, err := SerdeForType(have)
	is.NoErr(err)
	is.Equal(want.String(), got.String())

	bytes, err := got.Marshal(have)
	is.NoErr(err)
	// only try to unmarshal to ensure there's no error, other tests assert that
	// unmarshalled data matches the expectations
	var unmarshalled opencdc.StructuredData
	err = got.Unmarshal(bytes, &unmarshalled)
	is.NoErr(err)
}

func TestSerdeForType_UnsupportedTypes(t *testing.T) {
	testCases := []struct {
		val     any
		wantErr error
	}{
		// avro only supports fixed byte arrays
		{val: [4]int{}, wantErr: errors.New("record: arrays with value type int not supported, avro only supports bytes as values: unsupported avro type")},
		{val: [4]bool{}, wantErr: errors.New("record: arrays with value type bool not supported, avro only supports bytes as values: unsupported avro type")},
		// avro only supports maps with string keys
		{val: map[int]string{}, wantErr: errors.New("record: maps with key type int not supported, avro only supports strings as keys: unsupported avro type")},
		{val: map[bool]string{}, wantErr: errors.New("record: maps with key type bool not supported, avro only supports strings as keys: unsupported avro type")},
		// avro only supports signed integers
		{val: uint64(1), wantErr: errors.New("record: can't get schema for type uint64: unsupported avro type")},
		{val: uint(1), wantErr: errors.New("record: can't get schema for type uint: unsupported avro type")},
	}
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%T", tc.val), func(t *testing.T) {
			is := is.New(t)
			_, err := SerdeForType(tc.val)
			is.True(err != nil)
			is.Equal(err.Error(), tc.wantErr.Error())
		})
	}
}

func must[T any](f T, err error) T {
	if err != nil {
		panic(err)
	}
	return f
}
