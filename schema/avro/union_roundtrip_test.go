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
	"fmt"
	"math/big"
	"math/rand/v2"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/conduitio/conduit-commons/opencdc"
	"github.com/google/go-cmp/cmp"
	"github.com/iskorotkov/avro/v2"
)

// TestSerde_UnionRoundTrip_DecodedValueEncodesAgain checks that a value
// Unmarshal returns encodes again to the same data:
//
//	codec.Unmarshal(Marshal(Unmarshal(b))) == codec.Unmarshal(b)
//
// for random schemas with nested unions. The oracle is the codec alone: both
// sides are the codec's own decoding of bytes, so the check does not depend
// on the package's rules for where a union is unwrapped.
//
// The schemas only have unions whose branches decode to different Go types
// (at most one of record and map, of string, uuid and enum, and so on), so
// every decoded value has exactly one reading and the round trip must
// succeed. TestSerde_UnionRoundTrip_NeverSilentlyChanges covers the rest.
//
// Before BeforeMarshal walked the schema with the value, this failed for a
// nullable record, map, enum or fixed below a union whose value the old walk
// stepped over (ConduitIO/conduit-commons#312, review of #322).
func TestSerde_UnionRoundTrip_DecodedValueEncodesAgain(t *testing.T) {
	r := rand.New(rand.NewPCG(322, 1))
	for range roundTripIterations() {
		checkUnionRoundTrip(t, r.Uint64(), true)
	}
}

// TestSerde_UnionRoundTrip_NeverSilentlyChanges is the round trip over
// schemas that may have unions whose branches decode to the same Go type
// (a record and a map, a string and an enum, two records), and map keys that
// collide with the branch name "map". Such a decoded value can have more
// than one reading. Marshal must then return ErrSchemaValueMismatch, never
// encode a different value (invariant 6), and must leave the value it was
// given unchanged.
func TestSerde_UnionRoundTrip_NeverSilentlyChanges(t *testing.T) {
	r := rand.New(rand.NewPCG(322, 2))
	n := roundTripIterations()
	failed := 0
	for range n {
		if !checkUnionRoundTrip(t, r.Uint64(), false) {
			failed++
		}
	}
	// Ambiguous values are rare; many more errors would mean a reading is
	// missed, not that the schemas are ambiguous.
	if failed*20 > n {
		t.Errorf("%d of %d round trips returned an ambiguity error", failed, n)
	}
	t.Logf("%d of %d round trips returned an ambiguity error", failed, n)
}

// FuzzSerde_UnionRoundTrip is the fuzz form of the two round-trip tests.
func FuzzSerde_UnionRoundTrip(f *testing.F) {
	f.Add(uint64(322), true)
	f.Add(uint64(312), false)
	f.Fuzz(func(t *testing.T, seed uint64, distinct bool) {
		checkUnionRoundTrip(t, seed, distinct)
	})
}

func roundTripIterations() int {
	if os.Getenv("CONDUIT_PROPERTY_LONG") != "" {
		return 20000
	}
	return 1000
}

// checkUnionRoundTrip runs the round trip for the schema and value generated
// from seed. It returns false if Marshal returned an ambiguity error, which
// is only allowed if distinct is false.
func checkUnionRoundTrip(t *testing.T, seed uint64, distinct bool) bool {
	t.Helper()
	g := &rtGen{rng: rand.New(rand.NewPCG(seed, 322)), distinct: distinct}
	root, topUnion := g.root()

	s, err := Parse([]byte(root.json))
	if err != nil {
		t.Fatalf("seed %d: parse: %v\n%s", seed, err, root.json)
	}
	api := avro.Config{}.Freeze()
	encoded, err := api.Marshal(s.schema, g.value(root, 0))
	if err != nil {
		t.Fatalf("seed %d: codec encode: %v\n%s", seed, err, root.json)
	}
	var want any
	if err := api.Unmarshal(s.schema, encoded, &want); err != nil {
		t.Fatalf("seed %d: codec decode: %v", seed, err)
	}

	// Decode with the package, into the target a caller would use.
	var decoded any
	if topUnion {
		err = s.Unmarshal(encoded, &decoded)
	} else {
		var sd opencdc.StructuredData
		err = s.Unmarshal(encoded, &sd)
		decoded = sd
	}
	if err != nil {
		t.Fatalf("seed %d: unmarshal: %v\n%s", seed, err, root.json)
	}

	before := rtDeepCopy(decoded)
	reencoded, err := s.Marshal(decoded)
	if err != nil {
		if distinct || !errors.Is(err, ErrSchemaValueMismatch) {
			t.Fatalf("seed %d (distinct=%v): marshal of the decoded value: %v\nschema  %s\ndecoded %v", seed, distinct, err, root.json, before)
		}
		if diff := cmp.Diff(before, decoded, rtCmp...); diff != "" {
			t.Fatalf("seed %d: failed marshal changed its input (-before +after):\n%s", seed, diff)
		}
		return false
	}

	var got any
	if err := api.Unmarshal(s.schema, reencoded, &got); err != nil {
		t.Fatalf("seed %d: codec decode of the re-encoded value: %v", seed, err)
	}
	if diff := cmp.Diff(want, got, rtCmp...); diff != "" {
		t.Fatalf("seed %d: re-encoding the decoded value changed the data (-want +got):\n%s\nschema  %s\ndecoded %v", seed, diff, root.json, before)
	}
	return true
}

var rtCmp = []cmp.Option{cmp.Comparer(func(a, b *big.Rat) bool { return a.Cmp(b) == 0 })}

func rtDeepCopy(v any) any {
	switch v := v.(type) {
	case opencdc.StructuredData:
		return opencdc.StructuredData(rtDeepCopy(map[string]any(v)).(map[string]any))
	case map[string]any:
		m := make(map[string]any, len(v))
		for k, x := range v {
			m[k] = rtDeepCopy(x)
		}
		return m
	case []any:
		a := make([]any, len(v))
		for i, x := range v {
			a[i] = rtDeepCopy(x)
		}
		return a
	}
	return v
}

// rtGen generates random schemas and values for them in the form the codec
// encodes. It covers every Avro type, logical types, named types with and
// without namespaces, references to named types defined earlier, nullable
// and non-nullable unions of two to four branches, nested at any position,
// and map keys that are branch names.
type rtGen struct {
	rng *rand.Rand
	// distinct limits unions to branches that decode to different Go types,
	// and leaves out the map key "map".
	distinct bool
	n        int
	named    []*rtType // complete named types, for references
	ns       string    // namespace of the enclosing record
}

type rtType struct {
	kind    string // Avro type, a logical type name, "ref" or "union"
	json    string
	name    string // the codec's name for the type as a union branch
	group   string // types with the same group can't share a union (Avro rule)
	class   string // types with the same class decode to the same Go type
	elem    *rtType
	items   []*rtType // record fields or union branches
	symbols []string
	size    int
	target  *rtType // for "ref"
}

func (t *rtType) deref() *rtType {
	if t.kind == "ref" {
		return t.target
	}
	return t
}

// root returns a record with one to three fields, or, sometimes, a union.
// topUnion reports the latter.
func (g *rtGen) root() (*rtType, bool) {
	if g.rng.IntN(8) == 0 {
		return g.union(0), true
	}
	return g.record(0), false
}

// fullName returns a new name for a named type: a short name, which takes
// the namespace of the enclosing record, a name with a namespace attribute,
// or a dotted name.
func (g *rtGen) fullName() (full, ns, attrs string) {
	g.n++
	short := fmt.Sprintf("N%d", g.n)
	switch g.rng.IntN(3) {
	case 0:
		if g.ns == "" {
			return short, "", fmt.Sprintf(`"name":%q`, short)
		}
		return g.ns + "." + short, g.ns, fmt.Sprintf(`"name":%q`, short)
	case 1:
		ns := []string{"a", "b.c", "z.y.x"}[g.rng.IntN(3)]
		return ns + "." + short, ns, fmt.Sprintf(`"name":%q,"namespace":%q`, short, ns)
	default:
		ns := []string{"d", "e.f"}[g.rng.IntN(2)]
		return ns + "." + short, ns, fmt.Sprintf(`"name":%q`, ns+"."+short)
	}
}

func (g *rtGen) primitive() *rtType {
	type p struct{ kind, json, name, group, class string }
	ps := []p{
		{"boolean", `"boolean"`, "boolean", "boolean", "bool"},
		{"int", `"int"`, "int", "int", "int32"},
		{"long", `"long"`, "long", "long", "int64"},
		{"float", `"float"`, "float", "float", "float32"},
		{"double", `"double"`, "double", "double", "float64"},
		{"string", `"string"`, "string", "string", "string"},
		{"bytes", `"bytes"`, "bytes", "bytes", "bytes"},
		{"date", `{"type":"int","logicalType":"date"}`, "int.date", "int", "time"},
		{"timestamp-millis", `{"type":"long","logicalType":"timestamp-millis"}`, "long.timestamp-millis", "long", "time"},
		{"time-micros", `{"type":"long","logicalType":"time-micros"}`, "long.time-micros", "long", "duration"},
		{"uuid", `{"type":"string","logicalType":"uuid"}`, "string.uuid", "string", "string"},
		{"bytes-decimal", `{"type":"bytes","logicalType":"decimal","precision":10,"scale":2}`, "bytes.decimal", "bytes", "rat"},
	}
	x := ps[g.rng.IntN(len(ps))]
	return &rtType{kind: x.kind, json: x.json, name: x.name, group: x.group, class: x.class}
}

func (g *rtGen) named0() *rtType {
	if g.rng.IntN(3) == 0 {
		// A reference by full name. A name without a namespace can't be
		// referred to from inside a namespace: it would resolve in it.
		var targets []*rtType
		for _, n := range g.named {
			if strings.Contains(n.name, ".") || g.ns == "" {
				targets = append(targets, n)
			}
		}
		if len(targets) > 0 {
			target := targets[g.rng.IntN(len(targets))]
			return &rtType{kind: "ref", json: fmt.Sprintf("%q", target.name), name: target.name, group: target.group, class: target.class, target: target}
		}
	}
	full, _, attrs := g.fullName()
	var t *rtType
	switch g.rng.IntN(3) {
	case 0:
		t = &rtType{kind: "enum", name: full, class: "string", symbols: []string{"A", "B", "C"}}
		t.json = `{"type":"enum",` + attrs + `,"symbols":["A","B","C"]}`
	case 1:
		size := 1 + g.rng.IntN(4)
		t = &rtType{kind: "fixed", name: full, class: fmt.Sprintf("fixed%d", size), size: size}
		t.json = fmt.Sprintf(`{"type":"fixed",%s,"size":%d}`, attrs, size)
	default:
		t = &rtType{kind: "fixed-decimal", name: full, class: "rat", size: 8}
		t.json = `{"type":"fixed",` + attrs + `,"size":8,"logicalType":"decimal","precision":10,"scale":2}`
	}
	t.group = "named:" + full
	g.named = append(g.named, t)
	return t
}

func (g *rtGen) record(depth int) *rtType {
	full, ns, attrs := g.fullName()
	t := &rtType{kind: "record", name: full, group: "named:" + full, class: "maplike"}
	n := 1 + g.rng.IntN(3)
	fields := make([]string, 0, n)
	outer := g.ns
	g.ns = ns
	for i := range n {
		f := g.typ(depth + 1)
		t.items = append(t.items, f)
		fields = append(fields, fmt.Sprintf(`{"name":"f%d","type":%s}`, i, f.json))
	}
	g.ns = outer
	t.json = `{"type":"record",` + attrs + `,"fields":[` + strings.Join(fields, ",") + `]}`
	g.named = append(g.named, t) // only now: a reference to it can't be recursive
	return t
}

func (g *rtGen) container(depth int, kind string) *rtType {
	e := g.typ(depth + 1)
	if kind == "map" {
		return &rtType{kind: "map", name: "map", group: "map", class: "maplike", elem: e, json: `{"type":"map","values":` + e.json + `}`}
	}
	return &rtType{kind: "array", name: "array", group: "array", class: "array", elem: e, json: `{"type":"array","items":` + e.json + `}`}
}

// typ returns any type. Deep down it returns only types without children.
func (g *rtGen) typ(depth int) *rtType {
	choice := g.rng.IntN(10)
	if depth >= 4 {
		choice = g.rng.IntN(3)
	}
	switch {
	case choice < 2:
		return g.primitive()
	case choice < 3:
		return g.named0()
	case choice < 5:
		return g.record(depth)
	case choice < 6:
		return g.container(depth, "array")
	case choice < 7:
		return g.container(depth, "map")
	default:
		return g.union(depth)
	}
}

// union returns a union of two to four branches, often with null, and
// favors records, maps and arrays as branches so values are wrapped.
func (g *rtGen) union(depth int) *rtType {
	nb := 2 + g.rng.IntN(2)
	if g.rng.IntN(6) == 0 {
		nb = 4
	}
	t := &rtType{kind: "union"}
	groups, classes := map[string]bool{}, map[string]bool{}
	add := func(b *rtType) bool {
		if groups[b.group] || (g.distinct && classes[b.class]) {
			return false
		}
		groups[b.group], classes[b.class] = true, true
		t.items = append(t.items, b)
		return true
	}
	if g.rng.IntN(5) < 3 {
		add(&rtType{kind: "null", json: `"null"`, name: "null", group: "null", class: "null"})
	}
	for tries := 0; len(t.items) < nb && tries < 20; tries++ {
		mark := len(g.named)
		var b *rtType
		switch r := g.rng.IntN(6); {
		case depth >= 4 || r == 0:
			b = g.primitive()
		case r == 1:
			b = g.named0()
		case r == 2:
			b = g.record(depth + 1)
		case r == 3:
			b = g.container(depth+1, "map")
		case r == 4:
			b = g.container(depth+1, "array")
		default:
			b = g.typ(depth + 1)
		}
		if b.kind == "union" || !add(b) {
			g.named = g.named[:mark] // its named types are not in the schema
		}
	}
	if len(t.items) < 2 {
		add(&rtType{kind: "null", json: `"null"`, name: "null", group: "null", class: "null"})
	}
	if len(t.items) < 2 {
		add(&rtType{kind: "boolean", json: `"boolean"`, name: "boolean", group: "boolean", class: "bool"})
	}
	// Move null anywhere. The other branches keep their order: a branch may
	// refer to a named type an earlier branch defines.
	if t.items[0].kind == "null" {
		to := g.rng.IntN(len(t.items))
		null := t.items[0]
		copy(t.items, t.items[1:to+1])
		t.items[to] = null
	}
	js := make([]string, 0, len(t.items))
	for _, b := range t.items {
		js = append(js, b.json)
	}
	t.json = "[" + strings.Join(js, ",") + "]"
	return t
}

// value returns a random value of t in the form the codec encodes: a union
// value is always in the named form, {"branch": value}.
func (g *rtGen) value(t *rtType, depth int) any {
	t = t.deref()
	switch t.kind {
	case "null":
		return nil
	case "boolean":
		return g.rng.IntN(2) == 0
	case "int":
		return int32(g.rng.IntN(1000))
	case "long":
		return int64(g.rng.IntN(1000))
	case "float":
		return float32(g.rng.IntN(1000)) / 4
	case "double":
		return float64(g.rng.IntN(1000)) / 4
	case "string":
		return []string{"A", "s", "map", "null", ""}[g.rng.IntN(5)]
	case "bytes":
		return []byte{byte(g.rng.IntN(256))}
	case "date":
		return time.Date(2026, 1, 1+g.rng.IntN(300), 0, 0, 0, 0, time.UTC)
	case "timestamp-millis":
		return time.UnixMilli(int64(g.rng.IntN(1e12))).UTC()
	case "time-micros":
		return time.Duration(g.rng.IntN(1e9)) * time.Microsecond
	case "uuid":
		return fmt.Sprintf("123e4567-e89b-12d3-a456-4266141740%02d", g.rng.IntN(100))
	case "bytes-decimal", "fixed-decimal":
		return big.NewRat(int64(g.rng.IntN(100000)-50000), 100)
	case "enum":
		return t.symbols[g.rng.IntN(len(t.symbols))]
	case "fixed":
		a := reflect.New(reflect.ArrayOf(t.size, reflect.TypeFor[byte]())).Elem()
		for i := range t.size {
			a.Index(i).SetUint(uint64(g.rng.IntN(256)))
		}
		return a.Interface()
	case "record":
		m := map[string]any{}
		for i, f := range t.items {
			m[fmt.Sprintf("f%d", i)] = g.value(f, depth+1)
		}
		return m
	case "map":
		m := map[string]any{}
		for range g.length(depth) {
			m[g.mapKey()] = g.value(t.elem, depth+1)
		}
		return m
	case "array":
		a := make([]any, g.length(depth))
		for i := range a {
			a[i] = g.value(t.elem, depth+1)
		}
		return a
	case "union":
		b := t.items[g.rng.IntN(len(t.items))]
		if b.kind == "null" {
			return nil
		}
		return map[string]any{b.name: g.value(b, depth+1)}
	}
	panic("rtGen: unknown kind " + t.kind)
}

func (g *rtGen) length(depth int) int {
	if depth > 8 {
		return 0
	}
	return g.rng.IntN(3)
}

// mapKey returns a map key, often one that is also a union branch name.
func (g *rtGen) mapKey() string {
	keys := []string{"k0", "k1", "null", "int", "long", "string", "array", "boolean", "bytes.decimal"}
	if !g.distinct {
		keys = append(keys, "map", "map")
	}
	if len(g.named) > 0 && g.rng.IntN(4) == 0 {
		return g.named[g.rng.IntN(len(g.named))].name
	}
	return keys[g.rng.IntN(len(keys))]
}

// TestSerde_Marshal_NullableBelowWrappedUnion is the regression test for the
// review of #322: a nullable record, map, enum or fixed below a union that
// stays wrapped decodes bare since #312 was fixed, and Marshal has to wrap
// it again. Before BeforeMarshal walked the schema with the value, an empty
// map was silently written as null, a map with several entries and a nested
// record or enum failed.
func TestSerde_Marshal_NullableBelowWrappedUnion(t *testing.T) {
	rec := func(fieldType string) string {
		return `{"type":"record","name":"R","fields":[{"name":"m","type":["null",` + fieldType + `]}]}`
	}
	testCases := []struct {
		name  string
		field string // type of field f
		value any    // value of f, in the codec's form
	}{
		{
			"3-branch union, nullable empty map below", `["null",` + rec(`{"type":"map","values":"int"}`) + `,"string"]`,
			map[string]any{"R": map[string]any{"m": map[string]any{"map": map[string]any{}}}},
		},
		{
			"3-branch union, nullable map below", `["null",` + rec(`{"type":"map","values":"int"}`) + `,"string"]`,
			map[string]any{"R": map[string]any{"m": map[string]any{"map": map[string]any{"a": int32(1), "b": int32(2)}}}},
		},
		{
			"3-branch union, nullable record below", `["null",` + rec(`{"type":"record","name":"S","namespace":"x","fields":[{"name":"x","type":"int"}]}`) + `,"string"]`,
			map[string]any{"R": map[string]any{"m": map[string]any{"x.S": map[string]any{"x": int32(7)}}}},
		},
		{
			"3-branch union, nullable enum below", `["null",` + rec(`{"type":"enum","name":"E","symbols":["A","B"]}`) + `,"string"]`,
			map[string]any{"R": map[string]any{"m": map[string]any{"E": "B"}}},
		},
		{
			"3-branch union, nullable fixed below", `["null",` + rec(`{"type":"fixed","name":"F","size":2}`) + `,"string"]`,
			map[string]any{"R": map[string]any{"m": map[string]any{"F": [2]byte{1, 2}}}},
		},
		{
			"2-branch union in an array, nullable map below", `{"type":"array","items":["null",` + rec(`{"type":"map","values":"int"}`) + `,{"type":"array","items":"int"}]}`,
			[]any{map[string]any{"R": map[string]any{"m": map[string]any{"map": map[string]any{"a": int32(1), "b": int32(2)}}}}},
		},
		{
			"nullable record, nullable map below", `["null",` + rec(`{"type":"map","values":"int"}`) + `]`,
			map[string]any{"R": map[string]any{"m": map[string]any{"map": map[string]any{"a": int32(1), "b": int32(2)}}}},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			text := `{"type":"record","name":"Root","fields":[{"name":"f","type":` + tc.field + `}]}`
			s, err := Parse([]byte(text))
			if err != nil {
				t.Fatal(err)
			}
			api := avro.Config{}.Freeze()
			b, err := api.Marshal(s.schema, map[string]any{"f": tc.value})
			if err != nil {
				t.Fatal(err)
			}
			var want any
			if err := api.Unmarshal(s.schema, b, &want); err != nil {
				t.Fatal(err)
			}

			var decoded opencdc.StructuredData
			if err := s.Unmarshal(b, &decoded); err != nil {
				t.Fatal(err)
			}
			b2, err := s.Marshal(decoded)
			if err != nil {
				t.Fatalf("marshal of the decoded value %v: %v", decoded, err)
			}
			var got any
			if err := api.Unmarshal(s.schema, b2, &got); err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(want, got, rtCmp...); diff != "" {
				t.Fatalf("round trip changed the data (-want +got):\n%s", diff)
			}
		})
	}
}

// TestSerde_Marshal_AmbiguousUnionValue checks that a value that reads as
// more than one union branch is an error that leaves the value unchanged.
func TestSerde_Marshal_AmbiguousUnionValue(t *testing.T) {
	testCases := []struct {
		name  string
		field string
		value any // the value of f, as Marshal gets it
	}{
		{
			"map or record, as an array item", `{"type":"array","items":["null",{"type":"map","values":"int"},{"type":"record","name":"R","fields":[{"name":"a","type":"int"}]}]}`,
			[]any{map[string]any{"a": 1}},
		},
		{
			"string or enum, as a map value", `{"type":"map","values":["null","string",{"type":"enum","name":"E","symbols":["A"]},{"type":"array","items":"int"}]}`,
			map[string]any{"k": "A"},
		},
		{
			"date or timestamp", `["null",{"type":"int","logicalType":"date"},{"type":"long","logicalType":"timestamp-millis"}]`,
			time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			s, err := Parse([]byte(`{"type":"record","name":"Root","fields":[{"name":"f","type":` + tc.field + `}]}`))
			if err != nil {
				t.Fatal(err)
			}
			v := opencdc.StructuredData{"f": tc.value}
			before := rtDeepCopy(v)
			_, err = s.Marshal(v)
			if !errors.Is(err, ErrSchemaValueMismatch) || !strings.Contains(err.Error(), "ambiguous") {
				t.Fatalf("want an ambiguity error, got %v", err)
			}
			if diff := cmp.Diff(before, v); diff != "" {
				t.Fatalf("failed marshal changed its input (-before +after):\n%s", diff)
			}
		})
	}
}
