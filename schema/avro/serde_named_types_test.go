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
	"math/big"
	"math/rand/v2"
	"testing"

	"github.com/conduitio/conduit-commons/opencdc"
	"github.com/google/go-cmp/cmp"
	"github.com/iskorotkov/avro/v2"
)

// TestSerde_Marshal_NamedTypeReuseAcrossSchemas is the regression test for
// ConduitIO/conduit-commons#305. Two schemas define a named type with the
// same name and different contents, and refer to it by name from inside a
// container. After the first schema has encoded a value, the second must
// still encode with its own definition. Before the fix both shared the
// codec's process-wide encoder cache, whose key for a container of a
// reference is the reference's name only, so the second schema silently
// reused the first schema's encoder.
func TestSerde_Marshal_NamedTypeReuseAcrossSchemas(t *testing.T) {
	// schema builds a record named recName that defines the named type def
	// in a nullable field "top" and refers to it by name from field "a".
	schema := func(recName, def, ref string) string {
		return `{"type":"record","name":"` + recName + `","fields":[` +
			`{"name":"top","type":["null",` + def + `]},` +
			`{"name":"a","type":` + ref + `}]}`
	}
	fixedDecimal := func(scale string) string {
		return `{"type":"fixed","name":"fx","size":8,"logicalType":"decimal","precision":10,"scale":` + scale + `}`
	}
	record := func(fields string) string {
		return `{"type":"record","name":"R","fields":[` + fields + `]}`
	}
	enum := func(symbols string) string {
		return `{"type":"enum","name":"E","symbols":[` + symbols + `]}`
	}

	testCases := []struct {
		name   string
		first  string
		second string
		value  func() opencdc.StructuredData
	}{{
		// The case from the issue: 1.5 was written as 15000.
		name:   "decimal fixed scale, array of reference",
		first:  schema("dec1", fixedDecimal("6"), `{"type":"array","items":"fx"}`),
		second: schema("dec2", fixedDecimal("2"), `{"type":"array","items":"fx"}`),
		value: func() opencdc.StructuredData {
			return opencdc.StructuredData{"top": nil, "a": []any{big.NewRat(3, 2)}}
		},
	}, {
		name:   "decimal fixed scale, map of reference",
		first:  schema("decm1", fixedDecimal("6"), `{"type":"map","values":"fx"}`),
		second: schema("decm2", fixedDecimal("2"), `{"type":"map","values":"fx"}`),
		value: func() opencdc.StructuredData {
			return opencdc.StructuredData{"top": nil, "a": map[string]any{"k": big.NewRat(3, 2)}}
		},
	}, {
		// The enum index is written, so another symbol order writes another
		// symbol.
		name:   "enum symbol order, array of reference",
		first:  schema("enum1", enum(`"X","Y"`), `{"type":"array","items":"E"}`),
		second: schema("enum2", enum(`"Y","X"`), `{"type":"array","items":"E"}`),
		value: func() opencdc.StructuredData {
			return opencdc.StructuredData{"top": nil, "a": []any{"X"}}
		},
	}, {
		// Fields are written in schema order, so another field order swaps
		// the values.
		name:   "record field order, array of reference",
		first:  schema("rec1", record(`{"name":"p","type":"long"},{"name":"q","type":"long"}`), `{"type":"array","items":"R"}`),
		second: schema("rec2", record(`{"name":"q","type":"long"},{"name":"p","type":"long"}`), `{"type":"array","items":"R"}`),
		value: func() opencdc.StructuredData {
			return opencdc.StructuredData{"top": nil, "a": []any{map[string]any{"p": int64(1), "q": int64(2)}}}
		},
	}, {
		name:   "record field order, map of reference",
		first:  schema("recm1", record(`{"name":"p","type":"long"},{"name":"q","type":"long"}`), `{"type":"map","values":"R"}`),
		second: schema("recm2", record(`{"name":"q","type":"long"},{"name":"p","type":"long"}`), `{"type":"map","values":"R"}`),
		value: func() opencdc.StructuredData {
			return opencdc.StructuredData{"top": nil, "a": map[string]any{"k": map[string]any{"p": int64(1), "q": int64(2)}}}
		},
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			first, err := Parse([]byte(tc.first))
			if err != nil {
				t.Fatal(err)
			}
			second, err := Parse([]byte(tc.second))
			if err != nil {
				t.Fatal(err)
			}

			// Warm whatever cache the first schema uses.
			if _, err := first.Marshal(tc.value()); err != nil {
				t.Fatalf("marshal with first schema: %v", err)
			}

			b, err := second.Marshal(tc.value())
			if err != nil {
				t.Fatalf("marshal with second schema: %v", err)
			}
			var got opencdc.StructuredData
			if err := second.Unmarshal(b, &got); err != nil {
				t.Fatalf("unmarshal with second schema: %v", err)
			}
			if diff := cmp.Diff(tc.value(), got, ratComparer); diff != "" {
				t.Errorf("value encoded with the second schema decodes differently (-want +got):\n%s", diff)
			}
		})
	}
}

// TestSerde_Marshal_PropertyNamedTypeIsolation checks, for random pairs of
// schemas that use the same type names in the same places for different
// types, that encoding with the second schema after the first gives the
// same value as encoding with a codec configuration that has never seen any
// other schema.
func TestSerde_Marshal_PropertyNamedTypeIsolation(t *testing.T) {
	r := rand.New(rand.NewPCG(305, 305))
	for range propertyIterations() {
		checkNamedTypeIsolation(t, r.Uint64(), r.Uint64(), r.Uint64(), r.Uint64())
	}
}

// FuzzSerde_Marshal_NamedTypeIsolation is the fuzz form of
// TestSerde_Marshal_PropertyNamedTypeIsolation. The fuzzer picks the seeds.
func FuzzSerde_Marshal_NamedTypeIsolation(f *testing.F) {
	f.Add(uint64(1), uint64(2), uint64(3), uint64(4))
	f.Add(uint64(305), uint64(0), uint64(1), uint64(0))
	f.Fuzz(func(t *testing.T, layout, defA, defB, val uint64) {
		checkNamedTypeIsolation(t, layout, defA, defB, val)
	})
}

func checkNamedTypeIsolation(t *testing.T, layout, defA, defB, val uint64) {
	t.Helper()
	genA := genNamedSchema("A", layout, defA, genOptions{})
	genB := genNamedSchema("B", layout, defB, genOptions{})
	a := mustParseGen(t, genA)
	b := mustParseGen(t, genB)

	if _, err := a.Marshal(genA.value(rand.New(rand.NewPCG(val, 1)))); err != nil {
		t.Fatalf("seeds (%d,%d,%d,%d): marshal with A: %v\nA: %s", layout, defA, defB, val, err, genA.json())
	}

	valueB := func() any { return genB.value(rand.New(rand.NewPCG(val, 2))) }
	got, err := b.Marshal(valueB())
	if err != nil {
		t.Fatalf("seeds (%d,%d,%d,%d): marshal with B: %v\nB: %s", layout, defA, defB, val, err, genB.json())
	}

	// The reference encoding: the same value, through the same union
	// handling, with a configuration whose caches are empty.
	ref := valueB()
	if err := b.unionResolver.BeforeMarshal(ref); err != nil {
		t.Fatal(err)
	}
	want, err := avro.Config{}.Freeze().Marshal(b.schema, ref)
	if err != nil {
		t.Fatalf("seeds (%d,%d,%d,%d): reference marshal: %v\nB: %s", layout, defA, defB, val, err, genB.json())
	}

	// Map entries are written in Go map iteration order, so compare decoded
	// values, not bytes.
	var gotV, wantV opencdc.StructuredData
	if err := b.Unmarshal(got, &gotV); err != nil {
		t.Fatalf("seeds (%d,%d,%d,%d): unmarshal: %v", layout, defA, defB, val, err)
	}
	if err := b.Unmarshal(want, &wantV); err != nil {
		t.Fatalf("seeds (%d,%d,%d,%d): unmarshal reference: %v", layout, defA, defB, val, err)
	}
	if diff := cmp.Diff(wantV, gotV, ratComparer); diff != "" {
		t.Fatalf("seeds (%d,%d,%d,%d): B encodes differently after A (-want +got):\n%s\nA: %s\nB: %s",
			layout, defA, defB, val, diff, genA.json(), genB.json())
	}
}

// ratComparer compares decoded decimals by value.
var ratComparer = cmp.Comparer(func(a, b *big.Rat) bool { return a.Cmp(b) == 0 })
