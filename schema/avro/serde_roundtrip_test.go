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

package avro

import (
	"errors"
	"math/big"
	"math/rand/v2"
	"testing"

	"github.com/conduitio/conduit-commons/opencdc"
	"github.com/google/go-cmp/cmp"
)

// TestSerde_DecodedNullableNamedValuesEncodeAgain is the regression test for
// ConduitIO/conduit-commons#306. For a field typed ["null", T] where T is a
// record, map, enum or fixed, Unmarshal returns the bare value, and Marshal
// with the same schema used to reject it ("unknown union type n").
func TestSerde_DecodedNullableNamedValuesEncodeAgain(t *testing.T) {
	testCases := []struct {
		name   string
		branch string
		value  func() any // the bare value, as Unmarshal returns it
	}{
		{"record (issue case)", `{"type":"record","name":"R","fields":[{"name":"n","type":"int"}]}`, func() any { return map[string]any{"n": 1} }},
		{"record with namespace", `{"type":"record","name":"R","namespace":"a.b","fields":[{"name":"n","type":"int"}]}`, func() any { return map[string]any{"n": 1} }},
		{"record with a field named like the record", `{"type":"record","name":"R","fields":[{"name":"R","type":"int"}]}`, func() any { return map[string]any{"R": 7} }},
		{"map (issue case)", `{"type":"map","values":"int"}`, func() any { return map[string]any{"a": 1, "b": 2} }},
		{"map with a key named map", `{"type":"map","values":"int"}`, func() any { return map[string]any{"map": 3} }},
		{"enum", `{"type":"enum","name":"E","symbols":["X","Y"]}`, func() any { return "Y" }},
		{"fixed", `{"type":"fixed","name":"F","size":2}`, func() any { return [2]byte{1, 2} }},
		{"decimal fixed", `{"type":"fixed","name":"FD","size":8,"logicalType":"decimal","precision":10,"scale":2}`, func() any { return big.NewRat(3, 2) }},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			shapes := []struct {
				schema string
				value  func() opencdc.StructuredData
			}{{
				`{"type":"record","name":"T","fields":[{"name":"v","type":["null",` + tc.branch + `]}]}`,
				func() opencdc.StructuredData { return opencdc.StructuredData{"v": tc.value()} },
			}, {
				// The same union as the value type of a map.
				`{"type":"record","name":"T","fields":[{"name":"v","type":{"type":"map","values":["null",` + tc.branch + `]}}]}`,
				func() opencdc.StructuredData { return opencdc.StructuredData{"v": map[string]any{"k": tc.value()}} },
			}}
			for _, shape := range shapes {
				sch := shape.schema
				s, err := Parse([]byte(sch))
				if err != nil {
					t.Fatal(err)
				}
				in, want := shape.value(), shape.value()

				b, err := s.Marshal(in)
				if err != nil {
					t.Fatalf("%s: marshal bare value: %v", sch, err)
				}
				var got opencdc.StructuredData
				if err := s.Unmarshal(b, &got); err != nil {
					t.Fatalf("%s: unmarshal: %v", sch, err)
				}
				if diff := cmp.Diff(want, got, ratComparer); diff != "" {
					t.Fatalf("%s: (-want +got):\n%s", sch, diff)
				}
				// And what Unmarshal returned encodes again.
				if _, err := s.Marshal(got); err != nil {
					t.Fatalf("%s: marshal decoded value: %v", sch, err)
				}
			}
		})
	}
}

// TestSerde_NullableNamedForm_StillAccepted checks that the codec's named
// form, which Marshal required before #306, is still accepted and means the
// same value.
func TestSerde_NullableNamedForm_StillAccepted(t *testing.T) {
	s, err := Parse([]byte(`{"type":"record","name":"T","fields":[{"name":"v","type":["null",{"type":"record","name":"R","fields":[{"name":"n","type":"int"}]}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	named, err := s.Marshal(opencdc.StructuredData{"v": map[string]any{"R": map[string]any{"n": 1}}})
	if err != nil {
		t.Fatal(err)
	}
	bare, err := s.Marshal(opencdc.StructuredData{"v": map[string]any{"n": 1}})
	if err != nil {
		t.Fatal(err)
	}
	if string(named) != string(bare) {
		t.Fatalf("named form encodes as %x, bare as %x", named, bare)
	}
}

// TestSerde_PropertyRoundTrip checks encode, decode, encode, decode for
// random schemas and values: the two decoded values must be equal. Values
// are generated both in the codec's named form and bare, the way Unmarshal
// returns them.
func TestSerde_PropertyRoundTrip(t *testing.T) {
	r := rand.New(rand.NewPCG(306, 306))
	for range propertyIterations() {
		checkRoundTrip(t, r.Uint64(), r.Uint64(), r.Uint64(), r.IntN(2) == 1)
	}
}

// FuzzSerde_RoundTrip is the fuzz form of TestSerde_PropertyRoundTrip.
func FuzzSerde_RoundTrip(f *testing.F) {
	f.Add(uint64(1), uint64(2), uint64(3), false)
	f.Add(uint64(306), uint64(0), uint64(1), true)
	f.Fuzz(func(t *testing.T, layout, def, val uint64, bare bool) {
		checkRoundTrip(t, layout, def, val, bare)
	})
}

func checkRoundTrip(t *testing.T, layout, def, val uint64, bare bool) {
	t.Helper()
	gen := genNamedSchema("RT", layout, def, genOptions{bareNamed: bare})
	s := mustParseGen(t, gen)

	b1, err := s.Marshal(gen.value(rand.New(rand.NewPCG(val, 3))))
	if err != nil {
		t.Fatalf("seeds (%d,%d,%d,%v): marshal: %v\n%s", layout, def, val, bare, err, gen.json())
	}
	var v1 opencdc.StructuredData
	if err := s.Unmarshal(b1, &v1); err != nil {
		t.Fatalf("seeds (%d,%d,%d,%v): unmarshal: %v\n%s", layout, def, val, bare, err, gen.json())
	}
	b2, err := s.Marshal(v1.Clone())
	if err != nil {
		t.Fatalf("seeds (%d,%d,%d,%v): marshal decoded value %v: %v\n%s", layout, def, val, bare, v1, err, gen.json())
	}
	var v2 opencdc.StructuredData
	if err := s.Unmarshal(b2, &v2); err != nil {
		t.Fatalf("seeds (%d,%d,%d,%v): unmarshal again: %v\n%s", layout, def, val, bare, err, gen.json())
	}
	if diff := cmp.Diff(v1, v2, ratComparer); diff != "" {
		t.Fatalf("seeds (%d,%d,%d,%v): round trip changed the value (-first +second):\n%s\n%s", layout, def, val, bare, diff, gen.json())
	}
}

// TestSerde_NullableRecordWithFieldNamedLikeIt covers a record R with a
// field that is also named R (review of #320). Its bare value {"R": {...}}
// has the shape of the named form, and was misread as one, which dropped the
// field's contents ({f:{R:{a:b}}} encoded as {f:{R:nil}}).
func TestSerde_NullableRecordWithFieldNamedLikeIt(t *testing.T) {
	testCases := []struct {
		name  string
		field string // the type of R's field R
		value any    // a bare value of R
	}{
		{"field is a map", `{"type":"map","values":"string"}`, map[string]any{"R": map[string]any{"a": "b"}}},
		{"field is a map, with a default", `{"type":"map","values":"string"},"default":{}`, map[string]any{"R": map[string]any{"a": "b"}}},
		{"field is a record", `{"type":"record","name":"S","fields":[{"name":"a","type":"string"}]}`, map[string]any{"R": map[string]any{"a": "x"}}},
		{"field is a record, with a default", `{"type":"record","name":"S","fields":[{"name":"a","type":"string"}]},"default":{"a":"d"}`, map[string]any{"R": map[string]any{"a": "x"}}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sch := `{"type":"record","name":"T","fields":[{"name":"f","type":["null",{"type":"record","name":"R","fields":[{"name":"R","type":` + tc.field + `}]}]}]}`
			s, err := Parse([]byte(sch))
			if err != nil {
				t.Fatal(err)
			}
			b, err := s.Marshal(opencdc.StructuredData{"f": tc.value})
			if err != nil {
				t.Fatal(err)
			}
			var got opencdc.StructuredData
			if err := s.Unmarshal(b, &got); err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tc.value, got["f"]); diff != "" {
				t.Fatalf("(-want +got):\n%s", diff)
			}
			// The named form of the same value still works.
			b2, err := s.Marshal(opencdc.StructuredData{"f": map[string]any{"R": tc.value}})
			if err != nil {
				t.Fatal(err)
			}
			if string(b) != string(b2) {
				t.Fatalf("named form encodes as %x, bare as %x", b2, b)
			}
		})
	}
}

// TestSerde_NullableAmbiguousValue_IsAnError checks that a value that reads
// both as the named form and as a bare value is rejected instead of being
// guessed. R's only field R is nullable with a default, so {"R": {}} is both
// a bare R whose field R is an empty map and the named form of an R whose
// field R takes its default.
func TestSerde_NullableAmbiguousValue_IsAnError(t *testing.T) {
	s, err := Parse([]byte(`{"type":"record","name":"T","fields":[{"name":"f","type":["null",{"type":"record","name":"R","fields":[{"name":"R","type":["null",{"type":"map","values":"string"}],"default":null}]}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Marshal(opencdc.StructuredData{"f": map[string]any{"R": map[string]any{}}})
	if !errors.Is(err, ErrSchemaValueMismatch) {
		t.Fatalf("want ErrSchemaValueMismatch, got %v", err)
	}
}

// TestSerde_Marshal_CodecPanicIsAnError checks that a codec panic becomes an
// error instead of taking down the process (review of #320). The codec
// panics encoding {"N": {"N": nil}} with the recursive N{N: ["null","N"]}.
// Marshal of a Parse'd Serde wraps that value first and so never reaches
// the panic; the Serde here has the parsed schema but no union resolver, so
// the value goes to the codec as it is.
func TestSerde_Marshal_CodecPanicIsAnError(t *testing.T) {
	parsed, err := Parse([]byte(`{"type":"record","name":"N","fields":[{"name":"N","type":["null","N"]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	s := &Serde{
		schema:    parsed.schema,
		encodeAPI: newEncodeAPI(),
	}
	_, err = s.Marshal(map[string]any{"N": map[string]any{"N": nil}})
	if !errors.Is(err, ErrCodecPanic) {
		t.Fatalf("want ErrCodecPanic, got %v", err)
	}
}
