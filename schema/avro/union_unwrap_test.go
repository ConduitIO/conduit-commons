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
	"fmt"
	"math/big"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/conduitio/conduit-commons/opencdc"
	"github.com/google/go-cmp/cmp"
	"github.com/iskorotkov/avro/v2"
)

// TestSerde_Unmarshal_NestedUnionsKeepAllData is the regression test for
// ConduitIO/conduit-commons#312. A union the codec wraps, that is not an
// array item or map value, with another union below it, used to be walked
// one level off: a map key was dropped, or an inner union was left wrapped,
// or decoding failed.
func TestSerde_Unmarshal_NestedUnionsKeepAllData(t *testing.T) {
	testCases := []struct {
		name  string
		field string
		codec any // the value in the codec's form, as encoded
		want  any // what Unmarshal returns for the field
	}{{
		name:  "map branch of a 3-branch union, array of nullable ints below (key k was dropped)",
		field: `["null",{"type":"map","values":{"type":"array","items":"int"}},{"type":"array","items":["null","int"]}]`,
		codec: map[string]any{ukMap: map[string]any{"k": []any{1}}},
		want:  map[string]any{ukMap: map[string]any{"k": []any{1}}},
	}, {
		name:  "nullable map of maps of unions (key b was dropped)",
		field: `["null",{"type":"map","values":{"type":"map","values":["null","int",{"type":"array","items":"int"}]}}]`,
		codec: map[string]any{ukMap: map[string]any{"a": map[string]any{"b": map[string]any{"int": 1}}}},
		want:  map[string]any{"a": map[string]any{"b": 1}},
	}, {
		name:  "same, inner map with two keys (was an error)",
		field: `["null",{"type":"map","values":{"type":"map","values":["null","int",{"type":"array","items":"int"}]}}]`,
		codec: map[string]any{ukMap: map[string]any{"a": map[string]any{"b": map[string]any{"int": 1}, "c": map[string]any{"int": 2}}}},
		want:  map[string]any{"a": map[string]any{"b": 1, "c": 2}},
	}, {
		name:  "nullable map of unions (inner union was left wrapped)",
		field: `["null",{"type":"map","values":["null","int",{"type":"array","items":"int"}]}]`,
		codec: map[string]any{ukMap: map[string]any{"a": map[string]any{"int": 1}}},
		want:  map[string]any{"a": 1},
	}, {
		name:  "nullable record with a nullable field",
		field: `["null",{"type":"record","name":"R","fields":[{"name":"x","type":["null","string"]}]}]`,
		codec: map[string]any{"R": map[string]any{"x": map[string]any{"string": "s"}}},
		want:  map[string]any{"x": "s"},
	}}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sch := `{"type":"record","name":"r","fields":[{"name":"f","type":` + tc.field + `}]}`
			b, err := avro.Config{}.Freeze().Marshal(avro.MustParse(sch), map[string]any{"f": tc.codec})
			if err != nil {
				t.Fatal(err)
			}
			s, err := Parse([]byte(sch))
			if err != nil {
				t.Fatal(err)
			}
			var got opencdc.StructuredData
			if err := s.Unmarshal(b, &got); err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tc.want, got["f"]); diff != "" {
				t.Errorf("(-want +got):\n%s", diff)
			}
		})
	}
}

// TestSerde_Unmarshal_PropertyUnionShapes decodes random schemas with nested
// unions, maps, arrays, records and decimals (the generator of the
// Elasticsearch connector's TestProperty_UnionShapes, which had to skip the
// #312 class) and checks the result against the codec's own decoding: every
// value and every map key must be there, and each union must be unwrapped
// exactly where unwrapsAt says.
func TestSerde_Unmarshal_PropertyUnionShapes(t *testing.T) {
	r := rand.New(rand.NewPCG(312, 312))
	n := propertyIterations()
	for i := range n {
		checkUnionShapes(t, r.Uint64(), i)
	}
}

// FuzzSerde_Unmarshal_UnionShapes is the fuzz form of
// TestSerde_Unmarshal_PropertyUnionShapes.
func FuzzSerde_Unmarshal_UnionShapes(f *testing.F) {
	f.Add(uint64(312))
	f.Add(uint64(1))
	f.Fuzz(func(t *testing.T, seed uint64) {
		checkUnionShapes(t, seed, 0)
	})
}

func checkUnionShapes(t *testing.T, seed uint64, i int) {
	t.Helper()
	g := &ugGen{rng: rand.New(rand.NewPCG(seed, 1)), id: fmt.Sprintf("t%d", i)}
	f := g.union(0)
	if g.rng.IntN(3) == 0 {
		f = g.typ(0)
	}
	text := `{"type":"record","name":"` + g.next() + `","fields":[{"name":"f","type":` + f.json + `}]}`
	value := g.value(f)

	sch := avro.MustParse(text)
	api := avro.Config{}.Freeze()
	encoded, err := api.Marshal(sch, map[string]any{"f": value})
	if err != nil {
		t.Fatalf("seed %d: encode %s: %v", seed, text, err)
	}
	var raw map[string]any
	if err := api.Unmarshal(sch, encoded, &raw); err != nil {
		t.Fatalf("seed %d: codec decode: %v", seed, err)
	}

	s, err := Parse([]byte(text))
	if err != nil {
		t.Fatalf("seed %d: parse: %v", seed, err)
	}
	var got opencdc.StructuredData
	if err := s.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("seed %d: unmarshal: %v\nschema %s\nvalue  %v", seed, err, text, value)
	}
	if err := matchUnwrapped(f, raw["f"], got["f"], posField, "f"); err != nil {
		t.Fatalf("seed %d: %v\nschema  %s\ncodec   %v\ndecoded %v", seed, err, text, raw["f"], got["f"])
	}
}

// matchUnwrapped checks that decoded is raw (the codec's decoding) with
// exactly the union wrappers removed that unwrapsAt removes.
func matchUnwrapped(t *ugType, raw, decoded any, pos unionPosition, path string) error {
	switch t.kind {
	case ukDecimal, ukFixed:
		rr, ok1 := raw.(*big.Rat)
		dr, ok2 := decoded.(*big.Rat)
		if !ok1 || !ok2 || rr.Cmp(dr) != 0 {
			return fmt.Errorf("%s: decimal %v decoded as %v", path, raw, decoded)
		}
	case ukRecord:
		rm, ok1 := raw.(map[string]any)
		dm, ok2 := decoded.(map[string]any)
		if !ok1 || !ok2 || len(rm) != len(dm) {
			return fmt.Errorf("%s: record %v decoded as %v", path, raw, decoded)
		}
		for i, f := range t.items {
			k := fmt.Sprintf("f%d", i)
			if err := matchUnwrapped(f, rm[k], dm[k], posField, path+"."+k); err != nil {
				return err
			}
		}
	case ukMap:
		rm, ok1 := raw.(map[string]any)
		dm, ok2 := decoded.(map[string]any)
		if !ok1 || !ok2 || len(rm) != len(dm) {
			return fmt.Errorf("%s: map %v decoded as %v", path, raw, decoded)
		}
		for k, rv := range rm {
			dv, ok := dm[k]
			if !ok {
				return fmt.Errorf("%s: key %q dropped", path, k)
			}
			if err := matchUnwrapped(t.elem, rv, dv, posMapValue, path+"."+k); err != nil {
				return err
			}
		}
	case ukArray:
		ra, ok1 := raw.([]any)
		da, ok2 := decoded.([]any)
		if !ok1 || !ok2 || len(ra) != len(da) {
			return fmt.Errorf("%s: array %v decoded as %v", path, raw, decoded)
		}
		for i := range ra {
			if err := matchUnwrapped(t.elem, ra[i], da[i], posArrayItem, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	case ukUnion:
		if raw == nil {
			if decoded != nil {
				return fmt.Errorf("%s: null decoded as %v", path, decoded)
			}
			return nil
		}
		rw, ok := raw.(map[string]any)
		if !ok {
			// The codec returns a union value bare when it can resolve every
			// branch to a Go type (primitives and logical types like
			// bytes.decimal). There is nothing to unwrap.
			if !cmp.Equal(raw, decoded, ratComparer) {
				return fmt.Errorf("%s: bare union value %v decoded as %v", path, raw, decoded)
			}
			return nil
		}
		if len(rw) != 1 {
			return fmt.Errorf("%s: codec union value %v", path, raw)
		}
		name, inner := singleEntry(rw)
		var branch *ugType
		for _, b := range t.items {
			if b.name == name {
				branch = b
			}
		}
		if branch == nil {
			return fmt.Errorf("%s: codec used unknown branch %q", path, name)
		}
		if t.unwrapsAt(pos) {
			return matchUnwrapped(branch, inner, decoded, posTop, path)
		}
		dw, ok := decoded.(map[string]any)
		if !ok || len(dw) != 1 {
			return fmt.Errorf("%s: union at this position stays wrapped, decoded %v", path, decoded)
		}
		di, ok := dw[name]
		if !ok {
			return fmt.Errorf("%s: wrapper %q decoded as %v", path, name, decoded)
		}
		return matchUnwrapped(branch, inner, di, posTop, path+"."+name)
	}
	return nil
}

func singleEntry(m map[string]any) (string, any) {
	for k, v := range m {
		return k, v
	}
	return "", nil
}

// The generator below is the one from ConduitIO/conduit-connector-
// elasticsearch's TestProperty_UnionShapes (internal/decimaltext), with
// names changed to fit this package. It builds random nested schemas with
// unions, maps, arrays, records and decimals, and values for them in the
// codec's encoding shape. Map keys are drawn from the branch names in the
// schema, so data that looks like a wrapper is common.

const (
	ugBytesDecimal = "bytes.decimal"

	ukDecimal = "decimal"
	ukFixed   = "fixed"
	ukRecord  = "record"
	ukMap     = "map"
	ukArray   = "array"
	ukUnion   = "union"
)

type ugGen struct {
	rng   *rand.Rand
	id    string
	n     int
	names []string
}

type ugType struct {
	json  string
	kind  string // decimal, fixed, record, map, array, union
	name  string // branch name in a wrapper
	scale int
	elem  *ugType   // map values, array items
	items []*ugType // record fields, union branches (without null)
}

// unwrapsAt is unwrapsAt for the generated union: items never hold null, and
// every generated union has a null branch.
func (t *ugType) unwrapsAt(pos unionPosition) bool {
	nullable := len(t.items) == 1
	container := false
	for _, b := range t.items {
		if b.kind == ukMap || b.kind == ukArray {
			container = true
		}
	}
	switch pos {
	case posField:
		return nullable
	case posMapValue:
		return nullable || container
	case posArrayItem:
		return container
	case posTop:
		return false
	}
	return false
}

func (g *ugGen) next() string {
	g.n++
	return fmt.Sprintf("%s_%d", g.id, g.n)
}

func (g *ugGen) decimalType(allowBytes bool) *ugType {
	scale := 3 + g.rng.IntN(6)
	if allowBytes && g.rng.IntN(2) == 0 {
		return &ugType{
			kind: ukDecimal, name: ugBytesDecimal, scale: scale,
			json: fmt.Sprintf(`{"type":"bytes","logicalType":"decimal","precision":12,"scale":%d}`, scale),
		}
	}
	name := g.next()
	g.names = append(g.names, name)
	return &ugType{
		kind: ukFixed, name: name, scale: scale,
		json: fmt.Sprintf(`{"type":"fixed","name":%q,"size":8,"logicalType":"decimal","precision":12,"scale":%d}`, name, scale),
	}
}

func (g *ugGen) typ(depth int) *ugType {
	choice := g.rng.IntN(6)
	if depth >= 3 {
		choice = 0
	}
	switch choice {
	case 0:
		return g.decimalType(true)
	case 1:
		name := g.next()
		g.names = append(g.names, name)
		t := &ugType{kind: ukRecord, name: name}
		var fields []string
		for i := range 1 + g.rng.IntN(2) {
			f := g.typ(depth + 1)
			t.items = append(t.items, f)
			fields = append(fields, fmt.Sprintf(`{"name":"f%d","type":%s}`, i, f.json))
		}
		t.json = fmt.Sprintf(`{"type":"record","name":%q,"fields":[%s]}`, name, strings.Join(fields, ","))
		return t
	case 2:
		e := g.typ(depth + 1)
		return &ugType{kind: ukMap, name: ukMap, elem: e, json: `{"type":"map","values":` + e.json + `}`}
	case 3:
		e := g.typ(depth + 1)
		return &ugType{kind: ukArray, name: ukArray, elem: e, json: `{"type":"array","items":` + e.json + `}`}
	default:
		return g.union(depth)
	}
}

// union builds ["null", X] or ["null", X, Y] with at most one bytes, map and
// array branch, as Avro requires.
func (g *ugGen) union(depth int) *ugType {
	t := &ugType{kind: ukUnion}
	used := map[string]bool{}
	branches := make([]string, 1, 3)
	branches[0] = `"null"`
	for range 1 + g.rng.IntN(2) {
		var b *ugType
		for {
			switch g.rng.IntN(4) {
			case 0:
				b = g.decimalType(!used[ugBytesDecimal])
			case 1:
				b = g.typ(depth + 1)
				if b.kind == ukUnion {
					continue
				}
			case 2:
				e := g.typ(depth + 1)
				b = &ugType{kind: ukMap, name: ukMap, elem: e, json: `{"type":"map","values":` + e.json + `}`}
			default:
				e := g.typ(depth + 1)
				b = &ugType{kind: ukArray, name: ukArray, elem: e, json: `{"type":"array","items":` + e.json + `}`}
			}
			if b.kind != ukUnion && !used[b.name] {
				break
			}
		}
		used[b.name] = true
		t.items = append(t.items, b)
		branches = append(branches, b.json)
	}
	t.json = "[" + strings.Join(branches, ",") + "]"
	return t
}

func (g *ugGen) value(t *ugType) any {
	switch t.kind {
	case ukDecimal, ukFixed:
		return new(big.Rat).SetFrac64(int64(t.scale)*10+1, 10)
	case ukRecord:
		m := map[string]any{}
		for i, f := range t.items {
			m[fmt.Sprintf("f%d", i)] = g.value(f)
		}
		return m
	case ukMap:
		m := map[string]any{}
		for range 1 + g.rng.IntN(2) {
			key := "k"
			if len(g.names) > 0 && g.rng.IntN(2) == 0 {
				key = g.names[g.rng.IntN(len(g.names))]
			} else if g.rng.IntN(2) == 0 {
				key = []string{ugBytesDecimal, ukMap, ukArray, "null"}[g.rng.IntN(4)]
			}
			m[key] = g.value(t.elem)
		}
		return m
	case ukArray:
		var a []any
		for range 1 + g.rng.IntN(2) {
			a = append(a, g.value(t.elem))
		}
		return a
	case ukUnion:
		if g.rng.IntN(5) == 0 {
			return nil
		}
		b := t.items[g.rng.IntN(len(t.items))]
		return map[string]any{b.name: g.value(b)}
	}
	panic(t.kind)
}
