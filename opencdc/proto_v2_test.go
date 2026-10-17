// Copyright © 2026 Meroxa, Inc.
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

package opencdc

import (
	"encoding/json"
	"errors"
	"math"
	"testing"

	opencdcv1 "github.com/conduitio/conduit-commons/proto/opencdc/v1"
	opencdcv2 "github.com/conduitio/conduit-commons/proto/opencdc/v2"
	"github.com/matryer/is"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/structpb"
	"pgregory.net/rapid"
)

// TestStructuredData_ToProtoV2_Kinds pins the encoder's type mapping: int*
// becomes the int64 kind, uint* becomes the uint64 kind, []byte becomes the
// bytes kind, json.Number becomes int64 or decimal, and NaN and ±Inf become
// the double kind. See decision item 2 of the ADR
// 20261015-lossless-structured-values-protocol-v3.md.
func TestStructuredData_ToProtoV2_Kinds(t *testing.T) {
	is := is.New(t)

	tests := []struct {
		name string
		in   any
		want *opencdcv2.Value
	}{
		{name: "null", in: nil, want: &opencdcv2.Value{Kind: &opencdcv2.Value_Null{Null: structpb.NullValue_NULL_VALUE}}},
		{name: "bool", in: true, want: &opencdcv2.Value{Kind: &opencdcv2.Value_Bool{Bool: true}}},
		{name: "string", in: "foo", want: &opencdcv2.Value{Kind: &opencdcv2.Value_String_{String_: "foo"}}},
		{name: "float64", in: 1.5, want: &opencdcv2.Value{Kind: &opencdcv2.Value_Double{Double: 1.5}}},
		{name: "float32", in: float32(1.5), want: &opencdcv2.Value{Kind: &opencdcv2.Value_Double{Double: float64(float32(1.5))}}},
		{name: "float64_nan", in: math.NaN(), want: &opencdcv2.Value{Kind: &opencdcv2.Value_Double{Double: math.NaN()}}},
		{name: "float64_pos_inf", in: math.Inf(1), want: &opencdcv2.Value{Kind: &opencdcv2.Value_Double{Double: math.Inf(1)}}},
		{name: "float64_neg_inf", in: math.Inf(-1), want: &opencdcv2.Value{Kind: &opencdcv2.Value_Double{Double: math.Inf(-1)}}},
		{name: "int", in: int(1), want: &opencdcv2.Value{Kind: &opencdcv2.Value_Int64{Int64: 1}}},
		{name: "int8", in: int8(1), want: &opencdcv2.Value{Kind: &opencdcv2.Value_Int64{Int64: 1}}},
		{name: "int16", in: int16(1), want: &opencdcv2.Value{Kind: &opencdcv2.Value_Int64{Int64: 1}}},
		{name: "int32", in: int32(1), want: &opencdcv2.Value{Kind: &opencdcv2.Value_Int64{Int64: 1}}},
		{name: "int64", in: int64(1), want: &opencdcv2.Value{Kind: &opencdcv2.Value_Int64{Int64: 1}}},
		{name: "uint", in: uint(1), want: &opencdcv2.Value{Kind: &opencdcv2.Value_Uint64{Uint64: 1}}},
		{name: "uint8", in: uint8(1), want: &opencdcv2.Value{Kind: &opencdcv2.Value_Uint64{Uint64: 1}}},
		{name: "uint16", in: uint16(1), want: &opencdcv2.Value{Kind: &opencdcv2.Value_Uint64{Uint64: 1}}},
		{name: "uint32", in: uint32(1), want: &opencdcv2.Value{Kind: &opencdcv2.Value_Uint64{Uint64: 1}}},
		{name: "uint64", in: uint64(1), want: &opencdcv2.Value{Kind: &opencdcv2.Value_Uint64{Uint64: 1}}},
		{name: "bytes", in: []byte("foo"), want: &opencdcv2.Value{Kind: &opencdcv2.Value_Bytes{Bytes: []byte("foo")}}},
		{name: "json_number_int", in: json.Number("42"), want: &opencdcv2.Value{Kind: &opencdcv2.Value_Int64{Int64: 42}}},
		{name: "json_number_decimal", in: json.Number("1.5"), want: &opencdcv2.Value{Kind: &opencdcv2.Value_Decimal{Decimal: "1.5"}}},
		{name: "json_number_exp_int", in: json.Number("1e5"), want: &opencdcv2.Value{Kind: &opencdcv2.Value_Int64{Int64: 100000}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			is := is.New(t)

			var data opencdcv2.Data
			err := StructuredData{"v": tc.in}.ToProtoV2(&data)
			is.NoErr(err)

			got := data.GetStructuredData().Fields["v"]
			if wantF, ok := tc.want.Kind.(*opencdcv2.Value_Double); ok {
				// NaN never equals itself; compare via bits or IsNaN.
				gotF, ok := got.Kind.(*opencdcv2.Value_Double)
				is.True(ok)
				if math.IsNaN(wantF.Double) {
					is.True(math.IsNaN(gotF.Double))
					return
				}
				is.Equal(gotF.Double, wantF.Double)
				return
			}
			is.Equal(got, tc.want)
		})
	}
}

// TestStructuredData_ToProtoV2_RoundTrip proves encode then decode is lossless
// for every Value kind. The Go values below are the ones the decoder returns,
// so each must come back exactly equal (NaN excepted: it comes back as some
// NaN).
func TestStructuredData_ToProtoV2_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		in   any
	}{
		{name: "null", in: nil},
		{name: "bool", in: true},
		{name: "string", in: "hello"},
		{name: "double", in: 1.5},
		{name: "double_nan", in: math.NaN()},
		{name: "double_pos_inf", in: math.Inf(1)},
		{name: "double_neg_inf", in: math.Inf(-1)},
		{name: "int64_min", in: int64(math.MinInt64)},
		{name: "int64_max", in: int64(math.MaxInt64)},
		{name: "uint64_max", in: uint64(math.MaxUint64)},
		{name: "bytes", in: []byte("hello")},
		{name: "decimal_fraction", in: json.Number("1.5")},
		{name: "decimal_exp", in: json.Number("1e20")},
		{name: "decimal_huge_int", in: json.Number("123456789012345678901234567890")},
		{name: "list", in: []any{int64(1), "two", true, nil}},
		{name: "struct", in: map[string]any{"a": int64(1), "b": map[string]any{"c": []byte("x")}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			is := is.New(t)

			got := roundTripValueV2(t, tc.in)
			assertV2ValueEqual(t, is, tc.in, got)
		})
	}
}

// TestStructuredData_ToProtoV2_WireRoundTrip proves the losslessness across the
// actual protobuf wire: marshal to bytes and unmarshal back before decoding.
// In particular it pins that a null value (google.protobuf.NullValue, whose
// enum value is 0) survives the wire, which oneof presence guarantees.
func TestStructuredData_ToProtoV2_WireRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		in   any
	}{
		{name: "null", in: nil},
		{name: "bool", in: true},
		{name: "string", in: "hello"},
		{name: "double", in: 1.5},
		{name: "double_nan", in: math.NaN()},
		{name: "double_pos_inf", in: math.Inf(1)},
		{name: "double_neg_inf", in: math.Inf(-1)},
		{name: "int64", in: int64(1) << 53},
		{name: "uint64", in: uint64(math.MaxUint64)},
		{name: "bytes", in: []byte("hello")},
		{name: "decimal", in: json.Number("1.5")},
		{name: "list", in: []any{int64(1), "two", true, nil}},
		{name: "struct", in: map[string]any{"a": int64(1), "b": map[string]any{"c": []byte("x")}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			is := is.New(t)

			var data opencdcv2.Data
			err := StructuredData{"v": tc.in}.ToProtoV2(&data)
			is.NoErr(err)

			b, err := proto.Marshal(&data)
			is.NoErr(err)

			var wire opencdcv2.Data
			err = proto.Unmarshal(b, &wire)
			is.NoErr(err)

			got, err := dataFromProtoV2(&wire)
			is.NoErr(err)

			assertV2ValueEqual(t, is, tc.in, got.(StructuredData)["v"])
		})
	}
}

// TestStructuredData_ToProtoV2_OutsideSafeIntegerRange is the table proof that
// an integer outside ±(2^53-1) round-trips exactly through the v2 wire format,
// where the v1 wire format (google.protobuf.Struct) would round it. See #3100.
func TestStructuredData_ToProtoV2_OutsideSafeIntegerRange(t *testing.T) {
	const two53 = int64(1) << 53 // 2^53, the first integer a float64 cannot keep distinct from 2^53+1

	tests := []struct {
		name string
		in   any
		want any
	}{
		{name: "int64_2p53", in: two53, want: two53},
		{name: "int64_2p53_plus_1", in: two53 + 1, want: two53 + 1},
		{name: "int64_minus_2p53", in: -two53, want: -two53},
		{name: "int64_minus_2p53_minus_1", in: -two53 - 1, want: -two53 - 1},
		{name: "int64_max", in: int64(math.MaxInt64), want: int64(math.MaxInt64)},
		{name: "int64_min", in: int64(math.MinInt64), want: int64(math.MinInt64)},
		{name: "uint64_2p53", in: uint64(two53), want: uint64(two53)},
		{name: "uint64_max", in: uint64(math.MaxUint64), want: uint64(math.MaxUint64)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			is := is.New(t)
			got := roundTripValueV2(t, tc.in)
			is.Equal(got, tc.want) // exact, not approximate
		})
	}
}

// TestStructuredData_ToProtoV2_PropertyInt64Uint64RoundTrip is the property
// behind the table above: every int64 and uint64 survives a v2 round trip
// exactly, however large.
func TestStructuredData_ToProtoV2_PropertyInt64Uint64RoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		wantI := rapid.Int64().Draw(t, "int64")
		wantU := rapid.Uint64().Draw(t, "uint64")

		var data opencdcv2.Data
		if err := (StructuredData{"i": wantI, "u": wantU}).ToProtoV2(&data); err != nil {
			t.Fatalf("ToProtoV2: %v", err)
		}
		got, err := dataFromProtoV2(&data)
		if err != nil {
			t.Fatalf("dataFromProtoV2: %v", err)
		}
		gotSD := got.(StructuredData)
		if gotSD["i"] != wantI {
			t.Fatalf("int64 %d round-tripped as %v", wantI, gotSD["i"])
		}
		if gotSD["u"] != wantU {
			t.Fatalf("uint64 %d round-tripped as %v", wantU, gotSD["u"])
		}
	})
}

// TestRecord_ToProtoV2_FromProtoV2_RoundTrip exercises the full record path:
// raw and structured data, operation, metadata and position.
func TestRecord_ToProtoV2_FromProtoV2_RoundTrip(t *testing.T) {
	is := is.New(t)

	in := Record{
		Position:  Position("standing"),
		Operation: OperationUpdate,
		Metadata:  Metadata{"foo": "bar"},
		Key:       RawData("padlock-key"),
		Payload: Change{
			Before: RawData("yellow"),
			After: StructuredData{
				"id":     uint64(math.MaxUint64),
				"amount": json.Number("123.45"),
				"data":   []byte("raw bytes"),
				"nested": map[string]any{"ok": true, "count": int64(1 << 53)},
			},
		},
	}

	var proto opencdcv2.Record
	err := in.ToProtoV2(&proto)
	is.NoErr(err)

	var got Record
	err = got.FromProtoV2(&proto)
	is.NoErr(err)
	is.Equal(got, in)
}

// TestOpenCDCV1Proto_Unchanged pins the opencdc.v1.Data shape so a future
// change cannot add a field in place, which would silently drop data for old
// peers (the ADR's "No field is ever added to opencdc.v1.Data in place").
func TestOpenCDCV1Proto_Unchanged(t *testing.T) {
	is := is.New(t)

	dataMsg := opencdcv1.File_opencdc_v1_opencdc_proto.Messages().ByName("Data")
	if dataMsg == nil {
		t.Fatal("opencdc.v1.Data message not found")
	}

	oos := dataMsg.Oneofs()
	is.Equal(oos.Len(), 1)
	is.Equal(oos.Get(0).Name(), protoreflect.Name("data"))
	is.Equal(oos.Get(0).Fields().Len(), 2)

	rawData := dataMsg.Fields().ByName("raw_data")
	is.True(rawData != nil)
	is.Equal(rawData.Number(), protoreflect.FieldNumber(1))
	is.Equal(rawData.Kind(), protoreflect.BytesKind)

	structuredData := dataMsg.Fields().ByName("structured_data")
	is.True(structuredData != nil)
	is.Equal(structuredData.Number(), protoreflect.FieldNumber(2))
	is.Equal(structuredData.Kind(), protoreflect.MessageKind)
	is.Equal(structuredData.Message().FullName(), protoreflect.FullName("google.protobuf.Struct"))
}

// TestOpenCDCV1_ConversionUnchanged proves the v1 conversion is untouched: it
// still refuses an integer outside ±(2^53-1) and still turns []byte into a
// base64 string. Both are the v1 limitations that the v2 format fixes; if this
// test fails, the v1 wire behavior changed.
func TestOpenCDCV1_ConversionUnchanged(t *testing.T) {
	is := is.New(t)

	// v1 still refuses to round a wide integer.
	var data opencdcv1.Data
	err := StructuredData{"id": int64(1) << 53}.ToProto(&data)
	is.True(errors.Is(err, ErrIntegerNotRepresentable))

	// v1 still has no bytes kind: []byte comes back as a base64 string.
	var got Record
	err = got.FromProto(&opencdcv1.Record{
		Payload: &opencdcv1.Change{
			After: &opencdcv1.Data{Data: &opencdcv1.Data_StructuredData{
				StructuredData: mustStruct(t, map[string]any{"v": []byte("hi")}),
			}},
		},
	})
	is.NoErr(err)

	after, ok := got.Payload.After.(StructuredData)
	is.True(ok)
	is.Equal(after["v"], "aGk=") // base64 of "hi", a string, not []byte
}

func mustStruct(t *testing.T, m map[string]any) *structpb.Struct {
	t.Helper()
	s, err := structpb.NewStruct(m)
	if err != nil {
		t.Fatalf("structpb.NewStruct: %v", err)
	}
	return s
}

// roundTripValueV2 encodes a single value as structured data and returns the
// decoded value.
func roundTripValueV2(t *testing.T, in any) any {
	t.Helper()
	is := is.New(t)

	var data opencdcv2.Data
	err := StructuredData{"v": in}.ToProtoV2(&data)
	is.NoErr(err)

	got, err := dataFromProtoV2(&data)
	is.NoErr(err)
	return got.(StructuredData)["v"]
}

// assertV2ValueEqual compares a decoded v2 value to the value it was encoded
// from. NaN is the one value that is not equal to itself, so it is handled
// explicitly; everything else must be exactly equal.
func assertV2ValueEqual(t *testing.T, is *is.I, want, got any) {
	t.Helper()
	if wf, ok := want.(float64); ok {
		gf, ok := got.(float64)
		if !ok {
			t.Fatalf("expected float64, got %T (%v)", got, got)
		}
		if math.IsNaN(wf) {
			if !math.IsNaN(gf) {
				t.Fatalf("expected NaN, got %v", gf)
			}
			return
		}
	}
	is.Equal(got, want)
}
