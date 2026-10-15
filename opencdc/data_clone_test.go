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
	"fmt"
	"math/rand/v2"
	"reflect"
	"testing"
	"time"
)

// Tests for conduit-commons#336 on a release line. Clone must be a deep copy
// (no memory shared with the original), but it keeps the earlier releases'
// types exactly, so Avro schemas inferred from cloned records don't change
// within a patch release. main (v0.9.0+) keeps every type instead.

// legacyClone is StructuredData.Clone as released up to v0.7.1 / v0.8.2. The
// tests use it as the reference for what types Clone must produce.
func legacyClone(d StructuredData) StructuredData {
	cloned := make(map[string]any, len(d))
	for k, v := range d {
		if vmap, ok := v.(map[string]any); ok {
			cloned[k] = legacyClone(vmap)
		} else {
			cloned[k] = v
		}
	}
	return cloned
}

func TestStructuredData_Clone_Issue336(t *testing.T) {
	orig := StructuredData{"arr": []any{map[string]any{"x": 1}}, "m": map[string]any{"a": []any{1}}}
	c := orig.Clone().(StructuredData)

	c["arr"].([]any)[0].(map[string]any)["x"] = 2
	c["m"].(StructuredData)["a"].([]any)[0] = 9 // still StructuredData on this release line

	want := StructuredData{"arr": []any{map[string]any{"x": 1}}, "m": map[string]any{"a": []any{1}}}
	if !reflect.DeepEqual(orig, want) {
		t.Fatalf("mutating the clone changed the original: %#v", orig)
	}
}

// The retained retyping, pinned: a nested map[string]any becomes
// StructuredData, recursively through such maps; nothing else changes type.
func TestStructuredData_Clone_KeepsLegacyTypes(t *testing.T) {
	orig := StructuredData{
		"m":    map[string]any{"inner": map[string]any{"x": 1}, "arr": []any{map[string]any{"y": 2}}},
		"sd":   StructuredData{"inner": map[string]any{"z": 3}},
		"arr":  []any{map[string]any{"w": 4}},
		"tags": []string{"a"},
	}
	c := orig.Clone().(StructuredData)

	checks := []struct {
		name string
		got  any
		want any
	}{
		{"m", c["m"], StructuredData(nil)},
		{"m.inner", c["m"].(StructuredData)["inner"], StructuredData(nil)},
		{"m.arr[0]", c["m"].(StructuredData)["arr"].([]any)[0], map[string]any(nil)},
		{"sd", c["sd"], StructuredData(nil)},
		{"sd.inner", c["sd"].(StructuredData)["inner"], map[string]any(nil)},
		{"arr[0]", c["arr"].([]any)[0], map[string]any(nil)},
		{"tags", c["tags"], []string(nil)},
	}
	for _, ck := range checks {
		if reflect.TypeOf(ck.got) != reflect.TypeOf(ck.want) {
			t.Errorf("%s: got %T, want %T", ck.name, ck.got, ck.want)
		}
	}
	if !reflect.DeepEqual(c, legacyClone(orig)) {
		t.Fatalf("clone differs from the legacy clone:\n%#v\n%#v", c, legacyClone(orig))
	}
}

// genValue builds a random nested value from r. Two generators in the same
// state give two equal values that share nothing.
func genValue(r *rand.Rand, depth int) any {
	n := 10
	if depth <= 0 {
		n = 6 // scalars only
	}
	switch r.IntN(n) {
	case 0:
		return nil
	case 1:
		return r.IntN(2) == 0
	case 2:
		return r.Int64()
	case 3:
		return r.Float64()
	case 4:
		return fmt.Sprintf("s%d", r.IntN(1000))
	case 5:
		return time.Unix(r.Int64N(1<<32), 0).UTC()
	case 6:
		s := make([]any, r.IntN(4))
		for i := range s {
			s[i] = genValue(r, depth-1)
		}
		return s
	case 7:
		return genMap(r, depth-1)
	case 8:
		return StructuredData(genMap(r, depth-1))
	default:
		switch r.IntN(5) {
		case 0:
			b := make([]byte, r.IntN(4))
			for i := range b {
				b[i] = byte(r.IntN(256))
			}
			return b
		case 1:
			return []string{fmt.Sprint(r.IntN(9)), fmt.Sprint(r.IntN(9))}
		case 2:
			return map[string]string{"k": fmt.Sprint(r.IntN(9))}
		case 3:
			return map[string][]any{"k": {genValue(r, depth-1)}}
		default:
			return [2]any{genValue(r, depth-1), genValue(r, depth-1)}
		}
	}
}

func genMap(r *rand.Rand, depth int) map[string]any {
	m := make(map[string]any)
	for i := 0; i < r.IntN(4); i++ {
		m[fmt.Sprintf("f%d", i)] = genValue(r, depth)
	}
	return m
}

// mutate changes every mutable container reachable from v in place.
func mutate(v any) {
	switch v := v.(type) {
	case map[string]any:
		for k, e := range v {
			mutate(e)
			v[k] = "mutated"
		}
		v["added"] = true
	case StructuredData:
		mutate(map[string]any(v))
	case []any:
		for i, e := range v {
			mutate(e)
			v[i] = "mutated"
		}
	case []byte:
		for i := range v {
			v[i] ^= 0xff
		}
	case []string:
		for i := range v {
			v[i] = "mutated"
		}
	case map[string]string:
		v["k"] = "mutated"
	case map[string][]any:
		for _, e := range v["k"] {
			mutate(e)
		}
		if len(v["k"]) > 0 {
			v["k"][0] = "mutated"
		}
	case [2]any:
		mutate(v[0])
		mutate(v[1])
	}
}

func TestStructuredData_Clone_Property(t *testing.T) {
	for i := 0; i < 2000; i++ {
		seed := uint64(i)
		orig := StructuredData(genMap(rand.New(rand.NewPCG(seed, 1)), 4))
		twin := StructuredData(genMap(rand.New(rand.NewPCG(seed, 1)), 4))

		c := orig.Clone()

		// Property 1: same values and Go types as the released Clone.
		if !reflect.DeepEqual(c, legacyClone(orig)) {
			t.Fatalf("seed %d: clone differs from the legacy clone:\n%#v\n%#v", seed, c, legacyClone(orig))
		}
		// Property 2: changing the clone at any depth leaves the original
		// as it was.
		mutate(c)
		if !reflect.DeepEqual(orig, twin) {
			t.Fatalf("seed %d: mutating the clone changed the original:\n%#v\nwant\n%#v", seed, orig, twin)
		}
		// Property 3: and the other way round.
		c2 := twin.Clone()
		want := legacyClone(StructuredData(genMap(rand.New(rand.NewPCG(seed, 1)), 4)))
		mutate(twin)
		if !reflect.DeepEqual(c2, want) {
			t.Fatalf("seed %d: mutating the original changed the clone", seed)
		}
	}
}

func TestRecord_Clone_Deep(t *testing.T) {
	rec := Record{
		Position: Position("p"),
		Metadata: Metadata{"k": "v"},
		Key:      StructuredData{"id": []any{1, 2}},
		Payload: Change{
			Before: RawData("old-value"),
			After:  StructuredData{"nested": map[string]any{"tags": []any{"a"}}},
		},
	}
	c := rec.Clone()
	c.Key.(StructuredData)["id"].([]any)[0] = 9
	c.Payload.After.(StructuredData)["nested"].(StructuredData)["tags"].([]any)[0] = "z"
	c.Payload.Before.(RawData)[0] = 'X'
	c.Position[0] = 'X'
	c.Metadata["k"] = "changed"

	if rec.Key.(StructuredData)["id"].([]any)[0] != 1 ||
		rec.Payload.After.(StructuredData)["nested"].(map[string]any)["tags"].([]any)[0] != "a" ||
		string(rec.Payload.Before.(RawData)) != "old-value" ||
		string(rec.Position) != "p" ||
		rec.Metadata["k"] != "v" {
		t.Fatalf("mutating the clone changed the original: %#v", rec)
	}
}

// RawData nested inside structured data is copied too.
func TestStructuredData_Clone_NestedRawData(t *testing.T) {
	orig := StructuredData{"raw": RawData("abc"), "list": []any{RawData("def")}}
	c := orig.Clone().(StructuredData)
	c["raw"].(RawData)[0] = 'X'
	c["list"].([]any)[0].(RawData)[0] = 'Y'
	if string(orig["raw"].(RawData)) != "abc" || string(orig["list"].([]any)[0].(RawData)) != "def" {
		t.Fatalf("nested RawData shared with the clone: %#v", orig)
	}
}

func TestStructuredData_Clone_NilsStayNil(t *testing.T) {
	d := StructuredData{
		"sd": StructuredData(nil),
		"s":  []any(nil),
		"b":  []byte(nil),
		"ss": []string(nil),
	}
	c := d.Clone().(StructuredData)
	if !reflect.DeepEqual(d, c) {
		t.Fatalf("nil containers changed: %#v", c)
	}
}
