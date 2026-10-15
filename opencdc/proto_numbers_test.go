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
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	opencdcv1 "github.com/conduitio/conduit-commons/proto/opencdc/v1"
	"github.com/matryer/is"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

const safe = 1<<53 - 1

// TestStructuredData_ToProto_WideIntegerIsRefused is the regression test for
// ConduitIO/conduit#3100: before the fix, 2^53+1 came back as 2^53 with a nil
// error, so a destination deleted or overwrote the wrong row.
func TestStructuredData_ToProto_WideIntegerIsRefused(t *testing.T) {
	is := is.New(t)

	in := Record{Key: StructuredData{"id": int64(1<<53 + 1)}}
	var out opencdcv1.Record
	err := in.ToProto(&out)
	if err == nil {
		// Show what the receiver would have seen.
		var got Record
		is.NoErr(got.FromProto(&out))
		t.Fatalf("ToProto rounded silently: sent %d, receiver got %v", int64(1<<53+1), got.Key)
	}
	is.True(errors.Is(err, ErrIntegerNotRepresentable))
}

// wideInt and wideUint return 2^53 as int and uint on 64-bit platforms. They
// are computed at run time so the test still compiles on 386, where int is
// 32 bits and always in range.
func wideInt() any {
	shift := 53
	return int(int64(1) << shift)
}

func wideUint() any {
	shift := 53
	return uint(uint64(1) << shift)
}

func TestStructuredData_ToProto_IntegerRange(t *testing.T) {
	testCases := []struct {
		name  string
		value any
		ok    bool
	}{
		{"int64 2^53-1", int64(safe), true},
		{"int64 -(2^53-1)", int64(-safe), true},
		{"int64 2^53", int64(1 << 53), false},
		{"int64 2^53+1", int64(1<<53 + 1), false},
		{"int64 -(2^53)", int64(-(1 << 53)), false},
		{"int64 10^18", int64(1_000_000_000_000_000_000), false},
		{"int64 max", int64(math.MaxInt64), false},
		{"int64 min", int64(math.MinInt64), false},
		{"int max32", int(math.MaxInt32), true},
		{"uint64 2^53-1", uint64(safe), true},
		{"uint64 2^53", uint64(1 << 53), false},
		{"uint64 1<<63", uint64(1 << 63), false},
		{"uint64 max", uint64(math.MaxUint64), false},
		{"int32 max", int32(math.MaxInt32), true},
		{"uint32 max", uint32(math.MaxUint32), true},
		{"int8 min", int8(math.MinInt8), true},
		{"float64 2^60 unchanged", float64(1 << 60), true},
		{"float32 unchanged", float32(1.5), true},
	}
	if strconv.IntSize == 64 {
		testCases = append(testCases,
			struct {
				name  string
				value any
				ok    bool
			}{"int 2^53", wideInt(), false},
			struct {
				name  string
				value any
				ok    bool
			}{"uint 2^53", wideUint(), false},
		)
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			is := is.New(t)
			var out opencdcv1.Data
			err := StructuredData{"v": tc.value}.ToProto(&out)
			if tc.ok {
				is.NoErr(err)
				want, err := structpb.NewStruct(map[string]any{"v": tc.value})
				is.NoErr(err)
				is.True(proto.Equal(out.GetStructuredData(), want)) // same as structpb
				return
			}
			is.True(errors.Is(err, ErrIntegerNotRepresentable))
			var e *IntegerNotRepresentableError
			is.True(errors.As(err, &e))
			is.Equal(e.Path, "v")
		})
	}
}

func TestStructuredData_ToProto_ErrorNeverContainsTheValue(t *testing.T) {
	is := is.New(t)
	const v = int64(4111111111111111111)
	var out opencdcv1.Record
	err := Record{Payload: Change{After: StructuredData{"pan": v}}}.ToProto(&out)
	is.True(err != nil)
	is.True(!strings.Contains(err.Error(), strconv.FormatInt(v, 10)))
	is.True(!strings.Contains(err.Error(), "4.111111111111111"))
}

func TestRecord_ToProto_NumberErrorPaths(t *testing.T) {
	wide := int64(1<<53 + 1)
	testCases := []struct {
		name string
		rec  Record
		path string
	}{
		{"key", Record{Key: StructuredData{"id": wide}}, "key.id"},
		{"before", Record{Payload: Change{Before: StructuredData{"id": wide}}}, "payload.before.id"},
		{"after", Record{Payload: Change{After: StructuredData{"id": wide}}}, "payload.after.id"},
		{
			"nested map and list",
			Record{Payload: Change{After: StructuredData{
				"items": []any{map[string]any{"n": 1}, map[string]any{"n": 2}, map[string]any{"n": wide}},
			}}},
			"payload.after.items[2].n",
		},
		{"list of lists", Record{Key: StructuredData{"m": []any{[]any{1, wide}}}}, "key.m[0][1]"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			is := is.New(t)

			var out opencdcv1.Record
			err := tc.rec.ToProto(&out)
			var e *IntegerNotRepresentableError
			is.True(errors.As(err, &e))
			is.Equal(e.Path, tc.path)
			is.Equal(e.GoType, "int64")
			is.True(strings.Contains(err.Error(), `field "`+tc.path+`" (int64)`))

			// ValidateProto reports the same error without encoding.
			verr := tc.rec.ValidateProto()
			is.True(verr != nil)
			is.Equal(verr.Error(), err.Error())
		})
	}
}

func TestStructuredData_ToProto_JSONNumber(t *testing.T) {
	testCases := []struct {
		lit  string
		want error // nil: accepted
	}{
		// integers, in any notation
		{"9007199254740991", nil},
		{"-9007199254740991", nil},
		{"90071992547409.91e2", nil},
		{"9007199254740993", ErrIntegerNotRepresentable},
		{"9007199254740992", ErrIntegerNotRepresentable},
		{"9007199254740993e0", ErrIntegerNotRepresentable},
		{"1e20", ErrIntegerNotRepresentable},
		{"1E16", ErrIntegerNotRepresentable},
		{"1e15", nil},
		{"1e999999", ErrIntegerNotRepresentable},
		{"1e99999999999999999999", ErrIntegerNotRepresentable},
		{"-1e99999999999999999999", ErrIntegerNotRepresentable},
		// zero
		{"0", nil},
		{"-0", nil},
		{"0.000", nil},
		{"0e99999999999999999999", nil},
		{"0e-99999999999999999999", nil},
		// non-integers: unchanged (decimal precision is out of scope)
		{"1.5", nil},
		{"12345678901234567.5", nil},
		{"1e-99999999999999999999", nil},
		{"1.0000000000000000000000001", nil},
		// outside the RFC 8259 grammar
		{"0x20000000000001p0", ErrInvalidNumber},
		{"+9007199254740993", ErrInvalidNumber},
		{"Infinity", ErrInvalidNumber},
		{"-Inf", ErrInvalidNumber},
		{"NaN", ErrInvalidNumber},
		{"1_000", ErrInvalidNumber},
		{"01", ErrInvalidNumber},
		{"1.", ErrInvalidNumber},
		{".5", ErrInvalidNumber},
		{"1e", ErrInvalidNumber},
		{"1e+", ErrInvalidNumber},
		{"-", ErrInvalidNumber},
		{" 1", ErrInvalidNumber},
		{"", ErrInvalidNumber},
	}
	for _, tc := range testCases {
		t.Run(tc.lit, func(t *testing.T) {
			is := is.New(t)
			var out opencdcv1.Data
			err := StructuredData{"n": json.Number(tc.lit)}.ToProto(&out)
			if tc.want == nil {
				is.NoErr(err)
				want, err := structpb.NewStruct(map[string]any{"n": json.Number(tc.lit)})
				is.NoErr(err)
				is.True(proto.Equal(out.GetStructuredData(), want))
				return
			}
			is.True(errors.Is(err, tc.want))
			is.True(!strings.Contains(err.Error(), tc.lit) || tc.lit == "")
		})
	}
}

func TestStructuredData_ToProto_HugeJSONNumberIsCheap(t *testing.T) {
	is := is.New(t)
	lit := json.Number("1e" + strings.Repeat("9", 1<<20)) // a 1 MiB exponent
	start := time.Now()
	allocs := testing.AllocsPerRun(5, func() {
		var out opencdcv1.Data
		err := StructuredData{"n": lit}.ToProto(&out)
		is.True(errors.Is(err, ErrIntegerNotRepresentable))
	})
	is.True(time.Since(start) < 2*time.Second)
	is.True(allocs < 50)
}

func TestStructuredData_ToProto_UnchangedBehavior(t *testing.T) {
	testCases := []struct {
		name string
		in   map[string]any
	}{
		{"bytes become base64", map[string]any{"b": []byte{0, 1, 2}}},
		{"nil, bool, string", map[string]any{"a": nil, "b": true, "c": "x"}},
		{"nested", map[string]any{"m": map[string]any{"l": []any{1, "x", nil}}}},
		{"NaN", map[string]any{"f": math.NaN()}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			is := is.New(t)
			var out opencdcv1.Data
			is.NoErr(StructuredData(tc.in).ToProto(&out))
			want, err := structpb.NewStruct(tc.in)
			is.NoErr(err)
			is.True(proto.Equal(out.GetStructuredData(), want))
		})
	}

	errCases := []struct {
		name string
		in   map[string]any
	}{
		{"invalid type", map[string]any{"t": time.Now()}},
		{"nested StructuredData", map[string]any{"s": StructuredData{"a": 1}}},
		{"invalid UTF-8 value", map[string]any{"s": "\xff"}},
		{"invalid UTF-8 key", map[string]any{"\xff": 1}},
	}
	for _, tc := range errCases {
		t.Run(tc.name, func(t *testing.T) {
			is := is.New(t)
			var out opencdcv1.Data
			err := StructuredData(tc.in).ToProto(&out)
			_, want := structpb.NewStruct(tc.in)
			is.True(want != nil)
			is.Equal(err.Error(), "could not convert structured data to proto: "+want.Error())

			verr := Record{Key: StructuredData(tc.in)}.ValidateProto()
			is.True(verr != nil)
			is.Equal(verr.Error(), "error converting key: "+err.Error())
		})
	}
}

func TestRecord_ValidateProto(t *testing.T) {
	is := is.New(t)
	is.NoErr(Record{}.ValidateProto())
	is.NoErr(Record{Key: RawData("x"), Payload: Change{After: RawData("y")}}.ValidateProto())
	is.NoErr(Record{Payload: Change{After: StructuredData{"id": int64(safe)}}}.ValidateProto())

	err := Record{Payload: Change{After: StructuredData{"n": json.Number("Infinity")}}}.ValidateProto()
	var e *InvalidNumberError
	is.True(errors.As(err, &e))
	is.Equal(e.Path, "payload.after.n")
}

func TestRecord_ValidateProto_AllocatesOnlyTheWalker(t *testing.T) {
	is := is.New(t)
	rec := Record{
		Key: StructuredData{"id": int64(42)},
		Payload: Change{After: StructuredData{
			"id": int64(42), "name": "x", "tags": []any{"a", "b"}, "m": map[string]any{"n": 1.5},
		}},
	}
	// Per StructuredData only the walker and its path stack: no proto
	// messages and no strings.
	allocs := testing.AllocsPerRun(100, func() { is.NoErr(rec.ValidateProto()) })
	is.True(allocs <= 4)
}

// FuzzStructuredDataIntegerRange: ToProto succeeds if and only if the value
// is in the safe range, and then the receiver gets exactly that value.
func FuzzStructuredDataIntegerRange(f *testing.F) {
	for _, v := range []int64{0, 1, -1, safe, -safe, 1 << 53, 1<<53 + 1, -(1<<53 + 1), math.MaxInt64, math.MinInt64} {
		f.Add(v, uint64(v))
	}
	f.Fuzz(func(t *testing.T, i int64, u uint64) {
		for _, v := range []any{i, u} {
			var out opencdcv1.Data
			err := StructuredData{"v": v}.ToProto(&out)

			var inRange bool
			var exact float64
			switch v := v.(type) {
			case int64:
				inRange = v >= -safe && v <= safe
				exact = float64(v)
			case uint64:
				inRange = v <= safe
				exact = float64(v)
			}
			if inRange != (err == nil) {
				t.Fatalf("%T %v: inRange=%v err=%v", v, v, inRange, err)
			}
			if err != nil {
				if !errors.Is(err, ErrIntegerNotRepresentable) {
					t.Fatalf("unexpected error: %v", err)
				}
				continue
			}
			var got Data
			got, err = dataFromProto(&out)
			if err != nil {
				t.Fatal(err)
			}
			if got.(StructuredData)["v"] != exact {
				t.Fatalf("%T %v arrived as %v", v, v, got.(StructuredData)["v"])
			}
		}
	})
}

var jsonNumberGrammar = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)

// FuzzJSONNumberClassification compares the scanner with an exact oracle
// (the RFC 8259 grammar and big.Rat), for literals small enough for big.Rat.
func FuzzJSONNumberClassification(f *testing.F) {
	for _, s := range []string{
		"0", "-0", "1", "9007199254740991", "9007199254740992", "1e15", "1e16", "1.5", "0.000",
		"90071992547409.91e2", "0x10", "+1", "Infinity", "01", "1.", ".5", "1e", "", "1e-5", "100e-2",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 64 {
			return
		}
		got := classifyJSONNumber(s)

		if !jsonNumberGrammar.MatchString(s) {
			if got != jsonNumberInvalid {
				t.Fatalf("%q: got %v, want invalid", s, got)
			}
			return
		}
		if m := jsonNumberGrammar.FindStringSubmatch(s); len(m[3]) > 6 {
			return // exponent too large for the big.Rat oracle
		}
		r, ok := new(big.Rat).SetString(s)
		if !ok {
			t.Fatalf("%q: oracle failed to parse a grammar-valid literal", s)
		}
		var want jsonNumberClass
		switch {
		case !r.IsInt():
			want = jsonNumberNonInteger
		case new(big.Int).Abs(r.Num()).Cmp(big.NewInt(safe)) <= 0:
			want = jsonNumberSafeInteger
		default:
			want = jsonNumberIntegerOutOfRange
		}
		if got != want {
			t.Fatalf("%q: got %v, want %v", s, got, want)
		}
	})
}
