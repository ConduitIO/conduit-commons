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
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"testing"

	opencdcv2 "github.com/conduitio/conduit-commons/proto/opencdc/v2"
	"github.com/matryer/is"
	"google.golang.org/protobuf/proto"
	"pgregory.net/rapid"
)

// marshalV2Ref encodes r exactly as the current implementation does: build the
// opencdc.v2 tree with ToProtoV2 and marshal it deterministically. This is the
// ground truth MarshalV2 must reproduce byte for byte.
func marshalV2Ref(r Record) ([]byte, error) {
	var p opencdcv2.Record
	if err := r.ToProtoV2(&p); err != nil {
		return nil, err
	}
	return proto.MarshalOptions{Deterministic: true}.Marshal(&p) //nolint:wrapcheck // test helper: the caller reports the error
}

// TestRecord_MarshalV2_ByteIdentical proves MarshalV2 emits the exact bytes the
// ToProtoV2 + proto.Marshal(Deterministic) path emits, for every record shape:
// raw and structured data, every value kind (int64, uint64, bytes, decimal,
// double, NaN, ±Inf, null, bool, string), nested maps and arrays, empty and nil
// fields. This is the wire-format regression gate: if this fails, the wire
// format changed.
func TestRecord_MarshalV2_ByteIdentical(t *testing.T) {
	cases := []Record{
		{},
		{Position: Position("standing")},
		{Operation: OperationSnapshot},
		{Metadata: Metadata{"": ""}},
		{Key: RawData(nil)},
		{Key: RawData("padlock-key")},
		{Key: StructuredData(nil)},
		{Payload: Change{}},
		{
			Position:  Position("0/1F2E3D4"),
			Operation: OperationUpdate,
			Metadata:  Metadata{"z": "last", "a": "first"},
			Key:       RawData("key"),
			Payload:   Change{Before: RawData("yellow"), After: RawData("green")},
		},
		{Key: StructuredData{"id": int64(42)}, Payload: Change{After: StructuredData{"v": nil}}},
		{
			Payload: Change{After: StructuredData{
				"u":    uint64(math.MaxUint64),
				"i":    int64(math.MinInt64),
				"b":    []byte{0x00, 0xff, 0x7f},
				"d":    json.Number("12345678901234567890.5"),
				"f":    -2.5,
				"nan":  math.NaN(),
				"inf":  math.Inf(1),
				"ninf": math.Inf(-1),
				"list": []any{int64(1), "two", true, nil, 2.5, []byte("x")},
				"nested": map[string]any{
					"a": int64(1),
					"b": map[string]any{"c": []byte("z"), "d": []any{uint64(7), json.Number("1e2")}},
				},
			}},
		},
		{Key: StructuredData{"id": uint64(1) << 53}, Payload: Change{After: StructuredData{
			"int":   int(1),
			"int8":  int8(2),
			"int16": int16(3),
			"int32": int32(4),
			"uint":  uint(5),
			"uint8": uint8(6),
			"f32":   float32(1.5),
		}}},
	}

	for i, r := range cases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			is := is.New(t)
			want, err := marshalV2Ref(r)
			is.NoErr(err)
			got, err := r.MarshalV2()
			is.NoErr(err)
			if !bytes.Equal(want, got) {
				t.Errorf("byte mismatch\nwant=%x\ngot =%x", want, got)
			}
		})
	}
}

// TestRecord_MarshalV2_PropertyByteIdentical is the property version of the
// byte-identical gate: for randomly generated nested records (every value kind,
// nested maps and arrays, any metadata and key), MarshalV2 must emit the same
// bytes as ToProtoV2 + proto.Marshal(Deterministic).
func TestRecord_MarshalV2_PropertyByteIdentical(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		r := Record{
			Position:  Position(rapid.String().Draw(t, "position")),
			Operation: Operation(rapid.IntRange(0, 4).Draw(t, "operation")),
			Metadata:  rapid.MapOf(rapid.String(), rapid.String()).Draw(t, "metadata"),
			Key:       genV2Data(2).Draw(t, "key"),
			Payload: Change{
				Before: genV2Data(2).Draw(t, "before"),
				After:  genV2Data(2).Draw(t, "after"),
			},
		}

		want, err := marshalV2Ref(r)
		if err != nil {
			t.Fatalf("marshalV2Ref: %v", err)
		}
		got, err := r.MarshalV2()
		if err != nil {
			t.Fatalf("MarshalV2: %v", err)
		}
		if !bytes.Equal(want, got) {
			t.Fatalf("byte mismatch\nwant=%x\ngot =%x", want, got)
		}
	})
}

// genV2Data generates a random opencdc Data value: raw bytes, nil, or
// structured data with nested values up to depth deep.
func genV2Data(depth int) *rapid.Generator[Data] {
	return rapid.OneOf[Data](
		rapid.Just[Data](nil),
		rapid.Map(rapid.SliceOf(rapid.Byte()), func(b []byte) Data { return RawData(b) }),
		rapid.Map(rapid.MapOf(rapid.String(), genV2Value(depth)), func(m map[string]any) Data { return StructuredData(m) }),
	)
}

// genV2Value generates a random structured-data value: every scalar kind a
// record carries (int64, uint64, bytes, decimal, double, bool, string, null,
// every int/uint width) plus nested lists and maps, up to depth deep.
func genV2Value(depth int) *rapid.Generator[any] {
	scalars := rapid.OneOf[any](
		rapid.Just[any](nil),
		rapid.Bool().AsAny(),
		rapid.String().AsAny(),
		rapid.Float64().AsAny(),
		rapid.Float32().AsAny(),
		rapid.Int().AsAny(),
		rapid.Int8().AsAny(),
		rapid.Int16().AsAny(),
		rapid.Int32().AsAny(),
		rapid.Int64().AsAny(),
		rapid.Uint().AsAny(),
		rapid.Uint8().AsAny(),
		rapid.Uint16().AsAny(),
		rapid.Uint32().AsAny(),
		rapid.Uint64().AsAny(),
		rapid.SliceOf(rapid.Byte()).AsAny(),
		rapid.Map(rapid.Int64(), func(i int64) any { return json.Number(strconv.FormatInt(i, 10)) }).AsAny(),
		rapid.Map(rapid.Int64(), func(i int64) any { return json.Number(strconv.FormatInt(i, 10) + ".5") }).AsAny(),
	)
	if depth <= 0 {
		return scalars
	}
	return rapid.OneOf(
		scalars,
		rapid.SliceOf(genV2Value(depth-1)).AsAny(),
		rapid.MapOf(rapid.String(), genV2Value(depth-1)).AsAny(),
	)
}

// TestRecord_MarshalV2_RoundTrip proves MarshalV2 output decodes back to the
// same record: marshal to bytes, unmarshal the bytes, and FromProtoV2 must
// reproduce the record. This is the losslessness guarantee across the real wire.
func TestRecord_MarshalV2_RoundTrip(t *testing.T) {
	rec := Record{
		Position:  Position("standing"),
		Operation: OperationUpdate,
		Metadata:  Metadata{"foo": "bar"},
		Key:       StructuredData{"id": uint64(math.MaxUint64)},
		Payload: Change{
			After: StructuredData{
				"amount": json.Number("123.45"),
				"data":   []byte("raw bytes"),
				"nested": map[string]any{"ok": true, "count": int64(1 << 53)},
			},
		},
	}

	b, err := rec.MarshalV2()
	is.New(t).NoErr(err)

	var wire opencdcv2.Record
	is.New(t).NoErr(proto.Unmarshal(b, &wire))

	var got Record
	is.New(t).NoErr(got.FromProtoV2(&wire))
	is.New(t).Equal(got, rec)
}

// TestRecord_MarshalV2_ErrorParity proves MarshalV2 returns the same error (same
// message, same field path) as ToProtoV2 for the inputs the wire encoder
// refuses: an invalid json.Number literal and a Go type a record does not carry.
func TestRecord_MarshalV2_ErrorParity(t *testing.T) {
	type notARecordValue struct{}

	cases := []Record{
		{Payload: Change{After: StructuredData{"n": json.Number("not-a-number")}}},
		{Key: StructuredData{"n": json.Number("0x1a")}},
		{Payload: Change{Before: StructuredData{"bad": notARecordValue{}}}},
		{Payload: Change{After: StructuredData{"list": []any{json.Number("1.5"), notARecordValue{}}}}},
	}

	for i, rec := range cases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			var p opencdcv2.Record
			wantErr := rec.ToProtoV2(&p)

			_, gotErr := rec.MarshalV2()

			if wantErr == nil {
				t.Fatal("ToProtoV2 unexpectedly succeeded")
			}
			if gotErr == nil {
				t.Fatal("MarshalV2 unexpectedly succeeded")
			}
			if gotErr.Error() != wantErr.Error() {
				t.Errorf("error mismatch\nToProtoV2: %v\nMarshalV2: %v", wantErr, gotErr)
			}
			// The invalid-number error must carry a path and match the sentinel.
			if errors.Is(gotErr, ErrInvalidNumber) != errors.Is(wantErr, ErrInvalidNumber) {
				t.Errorf("ErrInvalidNumber parity mismatch: got %v, want %v", gotErr, wantErr)
			}
		})
	}
}

// TestRecord_AppendV2_Appends proves AppendV2 appends to a non-empty dst
// instead of overwriting it, and that MarshalV2 equals AppendV2(nil).
func TestRecord_AppendV2_Appends(t *testing.T) {
	is := is.New(t)
	rec := Record{Key: StructuredData{"id": int64(7)}, Payload: Change{After: RawData("x")}}

	full, err := rec.MarshalV2()
	is.NoErr(err)

	prefix := []byte("prefix:")
	appended, err := rec.AppendV2(prefix)
	is.NoErr(err)
	is.True(bytes.HasPrefix(appended, prefix))
	is.Equal(appended[len(prefix):], full)

	// AppendV2(nil) is MarshalV2.
	fromNil, err := rec.AppendV2(nil)
	is.NoErr(err)
	is.Equal(fromNil, full)
}
