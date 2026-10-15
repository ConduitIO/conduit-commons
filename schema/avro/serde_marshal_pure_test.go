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
	"bytes"
	"fmt"
	"math/rand/v2"
	"reflect"
	"testing"

	"github.com/conduitio/conduit-commons/opencdc"
)

// This file tests that Serde.Marshal does not modify its input
// (ConduitIO/conduit-commons#342, invariant 6).

const (
	// pureMapArraySchema has union paths in both a map and an array.
	pureMapArraySchema = `{"type":"record","name":"r","fields":[
		{"name":"m","type":{"type":"map","values":["null","string","long",{"type":"array","items":"string"}]}},
		{"name":"a","type":{"type":"array","items":["null","string","long",{"type":"map","values":"string"}]}}]}`
	// pureSilentSchema is the one where the wrapper map is itself a valid
	// branch, so a second encode of a wrapped value succeeds and is wrong.
	pureSilentSchema = `{"type":"record","name":"r","fields":[
		{"name":"a","type":{"type":"array","items":["null","string",{"type":"map","values":"string"}]}}]}`
	// pureNestedSchema nests the unions below a record and a map.
	pureNestedSchema = `{"type":"record","name":"r","fields":[
		{"name":"id","type":"long"},
		{"name":"inner","type":{"type":"record","name":"in","fields":[
			{"name":"tags","type":{"type":"array","items":["null","string",{"type":"array","items":"string"}]}},
			{"name":"attrs","type":{"type":"map","values":["null","long",{"type":"map","values":"long"}]}}]}},
		{"name":"outer","type":{"type":"map","values":["null","string",{"type":"map","values":["null","string",{"type":"array","items":"string"}]}]}}]}`
)

func mustParse(tb testing.TB, schema string) *Serde {
	tb.Helper()
	srd, err := Parse([]byte(schema))
	if err != nil {
		tb.Fatal(err)
	}
	return srd
}

// TestSerde_Marshal_SharedValueEncodesTwice is the issue's repro: two records
// share nested containers, as a record's Before and After, a shallow clone or
// two fan-out branches do.
func TestSerde_Marshal_SharedValueEncodesTwice(t *testing.T) {
	srd := mustParse(t, pureMapArraySchema)
	shared := map[string]any{"k": "foo"}
	sharedArr := []any{"bar", int64(1)}
	rec1 := opencdc.StructuredData{"m": shared, "a": sharedArr}
	rec2 := opencdc.StructuredData{"m": shared, "a": sharedArr}

	enc1, err := srd.Marshal(rec1)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := shared["k"].(string); !ok || v != "foo" {
		t.Errorf("Marshal changed its input: m.k is %#v", shared["k"])
	}
	if v, ok := sharedArr[0].(string); !ok || v != "bar" {
		t.Errorf("Marshal changed its input: a[0] is %#v", sharedArr[0])
	}
	enc2, err := srd.Marshal(rec2)
	if err != nil {
		t.Fatalf("second Marshal of a shared value failed: %v", err)
	}
	if !bytes.Equal(enc1, enc2) {
		t.Errorf("second Marshal of a shared value wrote different bytes:\n%x\n%x", enc1, enc2)
	}
}

// TestSerde_Marshal_SharedValueSilent covers the case where the second encode
// used to succeed and write the wrapper map in place of the value.
func TestSerde_Marshal_SharedValueSilent(t *testing.T) {
	srd := mustParse(t, pureSilentSchema)
	sharedArr := []any{"bar"}
	rec1 := opencdc.StructuredData{"a": sharedArr}
	rec2 := opencdc.StructuredData{"a": sharedArr}
	if _, err := srd.Marshal(rec1); err != nil {
		t.Fatal(err)
	}
	enc2, err := srd.Marshal(rec2)
	if err != nil {
		t.Fatalf("second Marshal: %v", err)
	}
	var out opencdc.StructuredData
	if err := srd.Unmarshal(enc2, &out); err != nil {
		t.Fatal(err)
	}
	if got := out["a"].([]any)[0]; got != "bar" {
		t.Errorf("second record wrote a[0]=\"bar\" as %#v", got)
	}
}

// TestSerde_Marshal_SameValueTwice encodes a value produced by schema
// extraction twice. It used to fail the second time.
func TestSerde_Marshal_SameValueTwice(t *testing.T) {
	data := opencdc.StructuredData{
		"tags":  []any{"bar", map[string]any{"x": "y"}},
		"attrs": map[string]any{"a": "s", "b": []any{"t"}},
	}
	srd, err := SerdeForType(data)
	if err != nil {
		t.Fatal(err)
	}
	enc1, err := srd.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	enc2, err := srd.Marshal(data)
	if err != nil {
		t.Fatalf("second Marshal of the same value failed: %v", err)
	}
	if !sameEncoding(srd, enc1, enc2) {
		t.Errorf("Marshal of the same value wrote a different encoding:\n%x\n%x", enc1, enc2)
	}
}

// TestSerde_Marshal_BeforeAfterShareNestedValue is the SDK middleware shape
// (ConduitIO/conduit-connector-sdk#420): Before and After of one record hold
// the same nested value, and each is encoded on its own.
func TestSerde_Marshal_BeforeAfterShareNestedValue(t *testing.T) {
	srd := mustParse(t, pureMapArraySchema)
	nested := []any{"x", int64(2), map[string]any{"p": "q"}}
	before := opencdc.StructuredData{"m": map[string]any{"k": "v"}, "a": nested}
	after := opencdc.StructuredData{"m": map[string]any{"k": "v2"}, "a": nested}

	encBefore, err := srd.Marshal(before)
	if err != nil {
		t.Fatal(err)
	}
	encAfter, err := srd.Marshal(after)
	if err != nil {
		t.Fatalf("After shares a nested value with Before and failed to encode: %v", err)
	}

	var gotBefore, gotAfter opencdc.StructuredData
	if err := srd.Unmarshal(encBefore, &gotBefore); err != nil {
		t.Fatal(err)
	}
	if err := srd.Unmarshal(encAfter, &gotAfter); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotBefore["a"], nested) || !reflect.DeepEqual(gotAfter["a"], nested) {
		t.Errorf("shared value did not round trip:\nbefore %#v\nafter  %#v\nwant   %#v", gotBefore["a"], gotAfter["a"], nested)
	}
	if gotAfter["m"].(map[string]any)["k"] != "v2" {
		t.Errorf("After.m = %#v", gotAfter["m"])
	}
}

// TestSerde_Marshal_ConcurrentSharedValue encodes one value from many
// goroutines. It is meaningful under the race detector: Marshal used to write
// into the shared containers.
func TestSerde_Marshal_ConcurrentSharedValue(t *testing.T) {
	srd := mustParse(t, pureNestedSchema)
	rec := pureNestedRecord()
	want, err := srd.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	errs := make(chan error, 16)
	for range 16 {
		go func() {
			for range 50 {
				got, err := srd.Marshal(rec)
				if err != nil {
					errs <- err
					return
				}
				if !sameEncoding(srd, got, want) {
					errs <- fmt.Errorf("encodings differ: %x vs %x", got, want)
					return
				}
			}
			errs <- nil
		}()
	}
	for range 16 {
		if err := <-errs; err != nil {
			t.Error(err)
		}
	}
}

func pureNestedRecord() opencdc.StructuredData {
	return opencdc.StructuredData{
		"id": int64(7),
		"inner": map[string]any{
			"tags":  []any{"a", []string{"b", "c"}, nil},
			"attrs": map[string]any{"x": int64(1), "y": map[string]int64{"z": 2}, "n": nil},
		},
		"outer": map[string]any{
			"s": "str",
			"m": map[string]any{"k": "v", "arr": []string{"1"}, "nil": nil},
		},
	}
}

// TestSerde_Marshal_CyclicUnrelatedKey: a cycle under a key the schema does
// not mention is not reachable by the encoder, so it must not hang the copy
// Marshal makes.
func TestSerde_Marshal_CyclicUnrelatedKey(t *testing.T) {
	srd := mustParse(t, pureSilentSchema)
	rec := opencdc.StructuredData{"a": []any{"x"}}
	rec["self"] = rec
	if _, err := srd.Marshal(rec); err != nil {
		t.Fatal(err)
	}
	if _, ok := rec["a"].([]any)[0].(string); !ok {
		t.Errorf("input changed: %#v", rec["a"])
	}
}

// ---- property test ----

// sameEncoding reports whether two encodings are the same. The codec writes
// the entries of a Go map in iteration order, which is random, so two
// encodings of one value with several map entries differ in bytes. They are
// the same when they are byte-identical or decode to the same value; the
// tests that use single-entry maps compare bytes directly.
func sameEncoding(srd *Serde, a, b []byte) bool {
	if bytes.Equal(a, b) {
		return true
	}
	if len(a) != len(b) {
		return false // same value, same length: only the entry order differs
	}
	var da, db opencdc.StructuredData
	if srd.Unmarshal(a, &da) != nil || srd.Unmarshal(b, &db) != nil {
		return false
	}
	return reflect.DeepEqual(da, db)
}

// refMarshal is Marshal as it was before the fix: BeforeMarshal in place,
// then encode. It runs on a clone made by the test, so the clone's mutation
// is irrelevant, and it is the reference the differential test compares
// against.
func refMarshal(s *Serde, v any) ([]byte, error) {
	if err := s.unionResolver.BeforeMarshal(v); err != nil {
		return nil, err
	}
	b, err := s.encodeAPI.Marshal(s.schema, v)
	if err != nil {
		return nil, fmt.Errorf("could not marshal into avro: %w", err)
	}
	return b, nil
}

// testClone is a deep copy of the containers the generator produces,
// written independently of containerCopier. It keeps types and nil-ness.
func testClone(v any) any {
	switch v := v.(type) {
	case map[string]any:
		if v == nil {
			return v
		}
		c := make(map[string]any, len(v))
		for k, e := range v {
			c[k] = testClone(e)
		}
		return c
	case opencdc.StructuredData:
		if v == nil {
			return v
		}
		c := make(opencdc.StructuredData, len(v))
		for k, e := range v {
			c[k] = testClone(e)
		}
		return c
	case []any:
		if v == nil {
			return v
		}
		c := make([]any, len(v))
		for i, e := range v {
			c[i] = testClone(e)
		}
		return c
	case *opencdc.StructuredData:
		c := testClone(*v).(opencdc.StructuredData)
		return &c
	case *map[string]any:
		c := testClone(*v).(map[string]any)
		return &c
	case map[string]string:
		c := make(map[string]string, len(v))
		for k, e := range v {
			c[k] = e
		}
		return c
	case map[string]int64:
		c := make(map[string]int64, len(v))
		for k, e := range v {
			c[k] = e
		}
		return c
	case []string:
		return append([]string(nil), v...)
	}
	return v
}

type valueGen struct {
	r      *rand.Rand
	shared []any // containers reused at several places
}

func (g *valueGen) leaf() any {
	switch g.r.IntN(9) {
	case 0:
		return nil
	case 1:
		return fmt.Sprintf("s%d", g.r.IntN(5))
	case 2:
		return int64(g.r.IntN(100))
	case 3:
		return g.r.IntN(100) // int, resolves to long
	case 4:
		return g.r.IntN(2) == 0 // bool, matches no branch
	case 5:
		return []string{"a", "b"}
	case 6:
		return map[string]string{"k": "v"}
	case 7:
		return map[string]int64{"k": 1}
	default:
		return float32(1.5) // matches no branch
	}
}

func (g *valueGen) container(depth int) any {
	if len(g.shared) > 0 && g.r.IntN(5) == 0 {
		return g.shared[g.r.IntN(len(g.shared))]
	}
	var c any
	n := g.r.IntN(4)
	switch g.r.IntN(5) {
	case 0, 1:
		a := make([]any, n)
		for i := range a {
			a[i] = g.value(depth + 1)
		}
		c = a
	case 2:
		var m map[string]any
		if g.r.IntN(8) != 0 { // sometimes a nil map
			m = make(map[string]any, n)
			for i := range n {
				m[fmt.Sprintf("k%d", i)] = g.value(depth + 1)
			}
		}
		c = m
	case 3:
		m := make(opencdc.StructuredData, n)
		for i := range n {
			m[fmt.Sprintf("k%d", i)] = g.value(depth + 1)
		}
		c = m
	default:
		a := []any{}
		c = a
	}
	g.shared = append(g.shared, c)
	return c
}

func (g *valueGen) value(depth int) any {
	if depth < 4 && g.r.IntN(2) == 0 {
		return g.container(depth)
	}
	return g.leaf()
}

// record generates a top-level value for the given schema: usually the shape
// the schema describes, with generated values below it.
func (g *valueGen) record(schema int) any {
	g.shared = nil
	m := map[string]any{}
	switch schema {
	case 0:
		m["m"] = g.container(2)
		m["a"] = g.container(2)
	case 1:
		m["a"] = g.container(2)
	default:
		m["id"] = int64(g.r.IntN(10))
		m["inner"] = g.container(1)
		m["outer"] = g.container(1)
	}
	// Bias toward valid values: half the time keep only well-typed ones.
	if g.r.IntN(2) == 0 {
		switch schema {
		case 0:
			m["m"] = map[string]any{"k": "v", "n": nil, "l": int64(1), "arr": g.shareable([]any{"x"})}
			m["a"] = g.shareable([]any{"s", nil, int64(3), g.shareable(map[string]any{"p": "q"})})
		case 1:
			m["a"] = g.shareable([]any{"s", nil, g.shareable(map[string]any{"p": "q"})})
		default:
			m["inner"] = map[string]any{
				"tags":  []any{"a", []string{"b"}, nil},
				"attrs": map[string]any{"x": int64(1), "y": map[string]int64{"z": 2}},
			}
			m["outer"] = map[string]any{"s": "str", "m": map[string]any{"k": "v", "arr": []string{"1"}}}
		}
	}
	switch g.r.IntN(4) {
	case 0:
		return opencdc.StructuredData(m)
	case 1:
		sd := opencdc.StructuredData(m)
		return &sd
	case 2:
		return &m
	default:
		return m
	}
}

func (g *valueGen) shareable(c any) any {
	g.shared = append(g.shared, c)
	return c
}

func TestSerde_Marshal_Pure(t *testing.T) {
	serdes := []*Serde{
		mustParse(t, pureMapArraySchema),
		mustParse(t, pureSilentSchema),
		mustParse(t, pureNestedSchema),
	}
	const iterations = 4000
	var ok, failed int
	for i := range iterations {
		seed := uint64(i)
		which := i % len(serdes)
		srd := serdes[which]
		v := (&valueGen{r: rand.New(rand.NewPCG(seed, 1))}).record(which)

		snapshot := testClone(v)
		oracleInput := testClone(v)

		first, err1 := srd.Marshal(v)
		if !reflect.DeepEqual(v, snapshot) {
			t.Fatalf("seed %d: Marshal changed its input (schema %d):\nbefore %#v\nafter  %#v", seed, which, snapshot, v)
		}
		second, err2 := srd.Marshal(v)
		if (err1 == nil) != (err2 == nil) || !sameEncoding(srd, first, second) {
			t.Fatalf("seed %d: two Marshals of one value differ: %v / %v, %x / %x", seed, err1, err2, first, second)
		}
		if !reflect.DeepEqual(v, snapshot) {
			t.Fatalf("seed %d: second Marshal changed its input", seed)
		}

		// Differential: same bytes (or both an error; which of several bad
		// entries the error names depends on map order) as the pre-fix Marshal
		// on an input it was allowed to mutate.
		want, errRef := refMarshal(srd, oracleInput)
		if (err1 == nil) != (errRef == nil) {
			t.Fatalf("seed %d: outcome differs from reference: %v vs %v", seed, err1, errRef)
		}
		if !sameEncoding(srd, first, want) {
			t.Fatalf("seed %d: bytes differ from reference:\n%x\n%x", seed, first, want)
		}
		if err1 == nil {
			ok++
		} else {
			failed++
		}
	}
	// The generator must exercise both outcomes, or the property is vacuous.
	t.Logf("%d encoded, %d rejected", ok, failed)
	if ok < iterations/10 || failed < iterations/10 {
		t.Errorf("generator is lopsided: %d encoded, %d rejected", ok, failed)
	}
}

// ---- benchmark ----

// benchMarshal encodes a freshly built value on every iteration, because
// before the fix a second encode of one value failed, so the old code could
// not be measured on a reused one. The cost of building the value is in both
// sides of any before/after comparison.
func benchMarshal(b *testing.B, schema string, rec func() any) {
	b.Helper()
	srd := mustParse(b, schema)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := srd.Marshal(rec()); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSerde_Marshal_NoUnionPaths: a flat record, the common case. The
// copy must not be paid here.
func BenchmarkSerde_Marshal_NoUnionPaths(b *testing.B) {
	benchMarshal(b, `{"type":"record","name":"r","fields":[{"name":"id","type":"long"},{"name":"name","type":"string"}]}`,
		func() any { return opencdc.StructuredData{"id": int64(1), "name": "x"} })
}

// BenchmarkSerde_Marshal_UnionPaths: a record with union paths, the case
// that is copied.
func BenchmarkSerde_Marshal_UnionPaths(b *testing.B) {
	benchMarshal(b, pureNestedSchema, func() any { return pureNestedRecord() })
}

// BenchmarkSerde_Marshal_UnionPathsBuildOnly measures building the value
// alone, to subtract from the two above.
func BenchmarkSerde_Marshal_UnionPathsBuildOnly(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		_ = pureNestedRecord()
	}
}
