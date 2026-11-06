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

package avro_test

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/conduitio/conduit-commons/opencdc"
	"github.com/conduitio/conduit-commons/schema/avro"
	"github.com/matryer/is"
	"pgregory.net/rapid"
)

func mustRat(tb testing.TB, s string) *big.Rat {
	tb.Helper()
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		tb.Fatalf("bad rat %q", s)
	}
	return r
}

func decimalRecordSchema(typ string) string {
	return `{"type":"record","name":"r","fields":[{"name":"d","type":` + typ + `}]}`
}

func bytesDecimal(prec, scale int) string {
	return fmt.Sprintf(`{"type":"bytes","logicalType":"decimal","precision":%d,"scale":%d}`, prec, scale)
}

// roundTripDecimal encodes {"d": r} with serde and decodes it again.
func roundTripDecimal(serde *avro.Serde, r *big.Rat) (*big.Rat, error) {
	b, err := serde.Marshal(opencdc.StructuredData{"d": r})
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	var out map[string]any
	if err := serde.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	got, ok := out["d"].(*big.Rat)
	if !ok {
		return nil, fmt.Errorf("decoded %T, want *big.Rat", out["d"])
	}
	return got, nil
}

// TestSerde_Marshal_DecimalInferred covers the schema SerdeForType infers for
// a *big.Rat, decimal(31,10). Before the fix, 1.123456789012 was encoded as
// 1.1234567890 without an error.
func TestSerde_Marshal_DecimalInferred(t *testing.T) {
	testCases := []struct {
		value   string
		wantErr string // empty: must round-trip exactly
	}{
		{value: "1.123456789012", wantErr: `field "d": the value needs precision 13 and scale 12, the schema is decimal(31,10); ` +
			"fix: use a schema with at least 21 integer and 12 fractional digits, such as decimal(33,12), " +
			"or round the value to at most 10 fractional digits before encoding"},
		{value: "-1.123456789012", wantErr: `needs precision 13 and scale 12, the schema is decimal(31,10)`},
		{value: "0.00000000001", wantErr: `needs precision 11 and scale 11`},
		{value: "-0.00000000001", wantErr: `needs precision 11 and scale 11`},
		{value: "1/3", wantErr: `needs a non-terminating decimal expansion, the schema is decimal(31,10); ` +
			"fix: round the value to at most 10 fractional digits before encoding"},
		{value: "-2/3", wantErr: `needs a non-terminating decimal expansion`},
		// 22 integer digits do not fit 31-10 = 21.
		// Rounding would not help, so the fix only widens the schema.
		{value: "1000000000000000000000", wantErr: `needs precision 22 and scale 0, the schema is decimal(31,10); ` +
			"fix: use a schema with at least 22 integer and 10 fractional digits, such as decimal(32,10)"},
		{value: "-1000000000000000000000.5", wantErr: `needs precision 23 and scale 1`},

		{value: "0"},
		{value: "1.1234567890"},
		{value: "-1.1234567890"},
		{value: "0.0000000001"},
		{value: "-0.0000000001"},
		{value: "999999999999999999999.9999999999"},  // exactly 31 digits
		{value: "-999999999999999999999.9999999999"}, // exactly 31 digits
		{value: "32.32"},
		{value: "-0.01"},
		{value: "1/8"}, // 0.125
	}
	for _, tc := range testCases {
		t.Run(tc.value, func(t *testing.T) {
			is := is.New(t)
			r := mustRat(t, tc.value)
			serde, err := avro.SerdeForType(opencdc.StructuredData{"d": r}) // as avro.encode infers it
			is.NoErr(err)
			is.True(strings.Contains(serde.String(), `"precision":31,"scale":10`)) // the inferred shape is unchanged

			got, err := roundTripDecimal(serde, r)
			if tc.wantErr != "" {
				is.True(errors.Is(err, avro.ErrDecimalNotRepresentable))
				is.True(strings.Contains(err.Error(), tc.wantErr)) // error names field, need and schema
				is.True(!strings.Contains(err.Error(), tc.value))  // error does not repeat the value
				return
			}
			is.NoErr(err)
			is.Equal(got.Cmp(r), 0) // decoded value differs from the encoded one
		})
	}
}

// TestSerde_Marshal_DecimalParsedSchemas covers user-provided or registered
// schemas, whose decimals truncated (or, for negative values, rounded toward
// negative infinity) the same way, at every position the codec can put a
// decimal.
func TestSerde_Marshal_DecimalParsedSchemas(t *testing.T) {
	type tagged struct {
		Amount *big.Rat `avro:"amount"`
	}
	type embedded struct {
		tagged
		Other int `avro:"other"`
	}
	type embeddedPtr struct {
		*tagged
		Other int `avro:"other"`
	}
	fixedDec := `{"type":"fixed","name":"fx","size":8,"logicalType":"decimal","precision":10,"scale":2}`

	testCases := []struct {
		name    string
		schema  string
		value   any
		wantErr string // empty: must encode, unless codecErr
		// codecErr: the codec itself must reject the value; the test only
		// asserts that Marshal fails, so nothing is encoded silently.
		codecErr bool
	}{
		{
			name:    "bytes scale 2, three fractional digits",
			schema:  decimalRecordSchema(bytesDecimal(10, 2)),
			value:   map[string]any{"d": mustRat(t, "1.234")},
			wantErr: `field "d": the value needs precision 4 and scale 3, the schema is decimal(10,2)`,
		},
		{
			name:    "bytes scale 0, negative half rounds away",
			schema:  decimalRecordSchema(bytesDecimal(5, 0)),
			value:   map[string]any{"d": mustRat(t, "-1.5")},
			wantErr: `needs precision 2 and scale 1, the schema is decimal(5,0)`,
		},
		{
			name:   "bytes exact fit",
			schema: decimalRecordSchema(bytesDecimal(4, 2)),
			value:  map[string]any{"d": mustRat(t, "-99.99")},
		},
		{
			name:    "bytes one integer digit too many",
			schema:  decimalRecordSchema(bytesDecimal(4, 2)),
			value:   map[string]any{"d": mustRat(t, "100")},
			wantErr: `needs precision 3 and scale 0, the schema is decimal(4,2)`,
		},
		{
			name:    "fixed",
			schema:  decimalRecordSchema(fixedDec),
			value:   map[string]any{"d": mustRat(t, "0.001")},
			wantErr: `field "d": the value needs precision 3 and scale 3, the schema is decimal(10,2)`,
		},
		{
			name:   "fixed exact",
			schema: decimalRecordSchema(fixedDec),
			value:  map[string]any{"d": mustRat(t, "-12345678.91")},
		},
		{
			name:    "nullable union, bare value",
			schema:  decimalRecordSchema(`["null",` + bytesDecimal(10, 2) + `]`),
			value:   map[string]any{"d": mustRat(t, "0.125")},
			wantErr: `field "d": the value needs precision 3 and scale 3`,
		},
		{
			name:   "nullable union, null",
			schema: decimalRecordSchema(`["null",` + bytesDecimal(10, 2) + `]`),
			value:  map[string]any{"d": nil},
		},
		{
			name:    "union, named form",
			schema:  decimalRecordSchema(`["null","string",` + bytesDecimal(10, 2) + `]`),
			value:   map[string]any{"d": map[string]any{"bytes.decimal": mustRat(t, "0.125")}},
			wantErr: `field "d": the value needs precision 3 and scale 3`,
		},
		{
			name:    "union, fixed branch in named form",
			schema:  decimalRecordSchema(`["null",` + fixedDec + `]`),
			value:   map[string]any{"d": map[string]any{"fx": mustRat(t, "0.125")}},
			wantErr: `schema is decimal(10,2)`,
		},
		{
			name:    "nullable union of fixed, bare pointer",
			schema:  decimalRecordSchema(`["null",` + fixedDec + `]`),
			value:   map[string]any{"d": mustRat(t, "0.125")},
			wantErr: `field "d": the value needs precision 3 and scale 3, the schema is decimal(10,2)`,
		},
		// Non-nullable unions: the Serde's union resolver leaves a bare
		// *big.Rat unwrapped here, so only the walker's bare-decimal path
		// resolves it to the bytes.decimal branch the codec encodes it with.
		{
			name:    "union string first, bare value",
			schema:  decimalRecordSchema(`["string",` + bytesDecimal(10, 2) + `]`),
			value:   map[string]any{"d": mustRat(t, "1.255")},
			wantErr: `field "d": the value needs precision 4 and scale 3, the schema is decimal(10,2)`,
		},
		{
			name:   "union string first, bare value, exact",
			schema: decimalRecordSchema(`["string",` + bytesDecimal(10, 2) + `]`),
			value:  map[string]any{"d": mustRat(t, "1.25")},
		},
		{
			name:    "union decimal first, bare value",
			schema:  decimalRecordSchema(`[` + bytesDecimal(10, 2) + `,"string"]`),
			value:   map[string]any{"d": mustRat(t, "-1.255")},
			wantErr: `field "d": the value needs precision 4 and scale 3`,
		},
		{
			name:    "union with null and string, bare value",
			schema:  decimalRecordSchema(`["null","string",` + bytesDecimal(10, 2) + `]`),
			value:   map[string]any{"d": mustRat(t, "1.255")},
			wantErr: `field "d": the value needs precision 4 and scale 3`,
		},
		{
			name:    "union of int and decimal, bare value too many integer digits",
			schema:  decimalRecordSchema(`["int",` + bytesDecimal(4, 2) + `]`),
			value:   map[string]any{"d": mustRat(t, "100.5")},
			wantErr: `field "d": the value needs precision 4 and scale 1, the schema is decimal(4,2)`,
		},
		{
			name:    "union in an array item, bare value",
			schema:  decimalRecordSchema(`{"type":"array","items":["string",` + bytesDecimal(10, 2) + `]}`),
			value:   map[string]any{"d": []any{"x", mustRat(t, "1.255")}},
			wantErr: `field "d[1]": the value needs precision 4 and scale 3`,
		},
		{
			name:    "top-level union, bare value",
			schema:  `["string",` + bytesDecimal(10, 2) + `]`,
			value:   mustRat(t, "1.255"),
			wantErr: `field <root>: the value needs precision 4 and scale 3`,
		},
		{
			name:   "top-level union, bare value, exact",
			schema: `["string",` + bytesDecimal(10, 2) + `]`,
			value:  mustRat(t, "1.25"),
		},
		{
			// Both decimal kinds in one union: a bare value reads as either
			// branch, so Marshal must report it as ambiguous, not pick one.
			name:     "union of bytes and fixed decimals, bare value",
			schema:   decimalRecordSchema(`["null",` + bytesDecimal(10, 1) + `,` + fixedDec + `]`),
			value:    map[string]any{"d": mustRat(t, "1.2")},
			codecErr: true,
		},
		{
			name:    "union of bytes and fixed decimals, fixed branch named",
			schema:  decimalRecordSchema(`["null",` + bytesDecimal(10, 1) + `,` + fixedDec + `]`),
			value:   map[string]any{"d": map[string]any{"fx": mustRat(t, "1.255")}},
			wantErr: `field "d": the value needs precision 4 and scale 3, the schema is decimal(10,2)`,
		},
		{
			name:   "union of bytes and fixed decimals, fixed branch named, exact",
			schema: decimalRecordSchema(`["null",` + bytesDecimal(10, 1) + `,` + fixedDec + `]`),
			value:  map[string]any{"d": map[string]any{"fx": mustRat(t, "1.25")}},
		},
		{
			// No bytes.decimal branch: the Serde resolves the bare value to
			// the fixed branch, and that branch's scale applies.
			name:    "non-nullable union of string and fixed decimal, bare value",
			schema:  decimalRecordSchema(`["string",` + fixedDec + `]`),
			value:   map[string]any{"d": mustRat(t, "1.255")},
			wantErr: `field "d": the value needs precision 4 and scale 3, the schema is decimal(10,2)`,
		},
		{
			name:   "non-nullable union of string and fixed decimal, bare value, exact",
			schema: decimalRecordSchema(`["string",` + fixedDec + `]`),
			value:  map[string]any{"d": mustRat(t, "1.25")},
		},
		{
			name:    "array item",
			schema:  decimalRecordSchema(`{"type":"array","items":` + bytesDecimal(10, 2) + `}`),
			value:   map[string]any{"d": []any{mustRat(t, "1.25"), mustRat(t, "1.255")}},
			wantErr: `field "d[1]": the value needs precision 4 and scale 3`,
		},
		{
			name:    "map value",
			schema:  decimalRecordSchema(`{"type":"map","values":` + bytesDecimal(10, 2) + `}`),
			value:   map[string]any{"d": map[string]any{"a": mustRat(t, "1.255")}},
			wantErr: `field "d[*]": the value needs precision 4 and scale 3`,
		},
		{
			name:    "nested record",
			schema:  decimalRecordSchema(`{"type":"record","name":"inner","fields":[{"name":"x","type":` + bytesDecimal(10, 2) + `}]}`),
			value:   map[string]any{"d": map[string]any{"x": mustRat(t, "1.255")}},
			wantErr: `field "d.x": the value needs precision 4 and scale 3`,
		},
		{
			name:    "go struct with avro tag",
			schema:  `{"type":"record","name":"r","fields":[{"name":"amount","type":` + bytesDecimal(10, 2) + `}]}`,
			value:   tagged{Amount: mustRat(t, "1.255")},
			wantErr: `field "amount": the value needs precision 4 and scale 3`,
		},
		{
			name:    "go struct, embedded",
			schema:  `{"type":"record","name":"r","fields":[{"name":"amount","type":` + bytesDecimal(10, 2) + `},{"name":"other","type":"int"}]}`,
			value:   &embedded{tagged: tagged{Amount: mustRat(t, "1.255")}},
			wantErr: `field "amount": the value needs precision 4 and scale 3`,
		},
		{
			name:    "go struct, embedded pointer",
			schema:  `{"type":"record","name":"r","fields":[{"name":"amount","type":` + bytesDecimal(10, 2) + `},{"name":"other","type":"int"}]}`,
			value:   embeddedPtr{tagged: &tagged{Amount: mustRat(t, "1.255")}},
			wantErr: `field "amount": the value needs precision 4 and scale 3`,
		},
		{
			name:   "go struct, embedded pointer, exact",
			schema: `{"type":"record","name":"r","fields":[{"name":"amount","type":` + bytesDecimal(10, 2) + `},{"name":"other","type":"int"}]}`,
			value:  embeddedPtr{tagged: &tagged{Amount: mustRat(t, "1.25")}},
		},
		{
			// A nil embedded pointer: the walker skips its fields and the
			// codec reports it.
			name:     "go struct, nil embedded pointer",
			schema:   `{"type":"record","name":"r","fields":[{"name":"amount","type":["null",` + bytesDecimal(10, 2) + `]},{"name":"other","type":"int"}]}`,
			value:    embeddedPtr{},
			codecErr: true,
		},
		{
			// Above the cached powers of ten (10^76).
			name:    "precision 100, scale 80",
			schema:  decimalRecordSchema(bytesDecimal(100, 80)),
			value:   map[string]any{"d": new(big.Rat).SetFrac(big.NewInt(1), new(big.Int).Lsh(big.NewInt(1), 81))},
			wantErr: `field "d": the value needs precision 81 and scale 81, the schema is decimal(100,80)`,
		},
		{
			name:   "precision 100, scale 80, exact",
			schema: decimalRecordSchema(bytesDecimal(100, 80)),
			value:  map[string]any{"d": new(big.Rat).SetFrac(big.NewInt(1), new(big.Int).Lsh(big.NewInt(1), 80))},
		},
		{
			name:    "precision 100, scale 80, too many integer digits",
			schema:  decimalRecordSchema(bytesDecimal(100, 80)),
			value:   map[string]any{"d": new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(20), nil))},
			wantErr: `field "d": the value needs precision 21 and scale 0, the schema is decimal(100,80)`,
		},
		{
			name:   "top-level decimal",
			schema: bytesDecimal(10, 2),
			value:  mustRat(t, "1.255"),
			wantErr: `field <root>: the value needs precision 4 and scale 3, the schema is decimal(10,2); ` +
				"fix: use a schema with at least 8 integer and 3 fractional digits, such as decimal(11,3), " +
				"or round the value to at most 2 fractional digits before encoding",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			is := is.New(t)
			serde, err := avro.Parse([]byte(tc.schema))
			is.NoErr(err)
			_, err = serde.Marshal(tc.value)
			if tc.codecErr {
				is.True(err != nil)
				return
			}
			if tc.wantErr == "" {
				is.NoErr(err)
				return
			}
			is.True(errors.Is(err, avro.ErrDecimalNotRepresentable))
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not contain %q", err, tc.wantErr)
			}
		})
	}
}

// TestSerde_Marshal_DecimalProperty: for any rational value and any
// decimal(precision, scale), Marshal either returns
// ErrDecimalNotRepresentable or encodes a value that decodes to exactly the
// same number, and it errors exactly when the value does not fit.
func TestSerde_Marshal_DecimalProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		prec := rapid.IntRange(1, 38).Draw(t, "precision")
		scale := rapid.IntRange(0, prec).Draw(t, "scale")
		num := new(big.Int).SetInt64(rapid.Int64().Draw(t, "num"))
		num.Mul(num, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(rapid.IntRange(0, 25).Draw(t, "num exp"))), nil))
		den := new(big.Int).Exp(big.NewInt(2), big.NewInt(int64(rapid.IntRange(0, 20).Draw(t, "twos"))), nil)
		den.Mul(den, new(big.Int).Exp(big.NewInt(5), big.NewInt(int64(rapid.IntRange(0, 20).Draw(t, "fives"))), nil))
		den.Mul(den, big.NewInt(rapid.SampledFrom([]int64{1, 1, 1, 3, 7}).Draw(t, "other factor")))
		r := new(big.Rat).SetFrac(num, den)

		serde, err := avro.Parse([]byte(decimalRecordSchema(bytesDecimal(prec, scale))))
		if err != nil {
			t.Fatal(err)
		}
		got, err := roundTripDecimal(serde, r)

		// Independent oracle: the value fits if printing it with the scale
		// and parsing it back gives the same number, and the printed digits
		// fit the precision.
		s := r.FloatString(scale)
		back, _ := new(big.Rat).SetString(s)
		digits := len(strings.TrimLeft(strings.NewReplacer("-", "", ".", "").Replace(s), "0"))
		fits := back.Cmp(r) == 0 && digits <= prec

		switch {
		case fits && err != nil:
			t.Fatalf("%s fits decimal(%d,%d) but Marshal failed: %v", r.RatString(), prec, scale, err)
		case !fits && err == nil:
			t.Fatalf("%s does not fit decimal(%d,%d) but Marshal succeeded, decoded as %s", r.RatString(), prec, scale, got.RatString())
		case !fits && !errors.Is(err, avro.ErrDecimalNotRepresentable):
			t.Fatalf("%s: want ErrDecimalNotRepresentable, got %v", r.RatString(), err)
		case fits && got.Cmp(r) != 0:
			t.Fatalf("%s decoded as %s", r.RatString(), got.RatString())
		}
	})
}

// TestSerde_Marshal_DecimalErrorOmitsValue: the error names where the value
// is and how big it is, not the value, also for a value with many digits
// below a map entry, whose key is data too.
func TestSerde_Marshal_DecimalErrorOmitsValue(t *testing.T) {
	is := is.New(t)
	serde, err := avro.Parse([]byte(decimalRecordSchema(`{"type":"map","values":` + bytesDecimal(10, 2) + `}`)))
	is.NoErr(err)
	r := new(big.Rat).SetFrac(big.NewInt(1), new(big.Int).Lsh(big.NewInt(1), 400)) // 400 fractional digits
	_, err = serde.Marshal(map[string]any{"d": map[string]any{"secret-key": r}})
	is.True(errors.Is(err, avro.ErrDecimalNotRepresentable))
	is.True(strings.Contains(err.Error(), `field "d[*]": the value needs precision 400 and scale 400, the schema is decimal(10,2)`))
	is.True(!strings.Contains(err.Error(), "secret-key"))
	is.True(!strings.Contains(err.Error(), r.FloatString(400)[:20]))
}

// BenchmarkSerde_Marshal_Decimal measures the decimal check's share of
// Marshal for a record whose every field is a decimal.
func BenchmarkSerde_Marshal_Decimal(b *testing.B) {
	v := opencdc.StructuredData{}
	for i := range 20 {
		v[fmt.Sprintf("f%02d", i)] = big.NewRat(int64(i)*12345+1, 100)
	}
	serde, err := avro.SerdeForType(v)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := serde.Marshal(v); err != nil {
			b.Fatal(err)
		}
	}
}
