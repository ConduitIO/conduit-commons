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

// Regression and property tests for conduit-commons#336: Clone must be a
// deep copy that keeps every value's Go type.

func TestStructuredData_Clone_Issue336(t *testing.T) {
	orig := StructuredData{"arr": []any{map[string]any{"x": 1}}, "m": map[string]any{"a": []any{1}}}
	c := orig.Clone().(StructuredData)

	if _, ok := c["m"].(map[string]any); !ok {
		t.Fatalf("nested map came back as %T, want map[string]any", c["m"])
	}
	if !reflect.DeepEqual(orig, c) {
		t.Fatalf("clone differs from original:\n%#v\n%#v", orig, c)
	}

	c["arr"].([]any)[0].(map[string]any)["x"] = 2
	c["m"].(map[string]any)["a"].([]any)[0] = 9

	want := StructuredData{"arr": []any{map[string]any{"x": 1}}, "m": map[string]any{"a": []any{1}}}
	if !reflect.DeepEqual(orig, want) {
		t.Fatalf("mutating the clone changed the original: %#v", orig)
	}
}

// genValue builds a random nested value from r. Calling it twice with
// generators in the same state gives two equal values that share nothing,
// which is what lets the property test compare against an untouched twin.
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
	for i := range r.IntN(4) {
		m[fmt.Sprintf("f%d", i)] = genValue(r, depth)
	}
	return m
}

// mutate changes every mutable container reachable from v in place, at
// every depth.
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
		// arrays are values; mutate what they point into
		mutate(v[0])
		mutate(v[1])
	}
}

func TestStructuredData_Clone_Property(t *testing.T) {
	const iterations = 2000
	for i := range iterations {
		seed := uint64(i)
		orig := StructuredData(genMap(rand.New(rand.NewPCG(seed, 1)), 4))
		twin := StructuredData(genMap(rand.New(rand.NewPCG(seed, 1)), 4))

		c := orig.Clone()

		// Property 1: the clone equals the original, Go types included.
		if !reflect.DeepEqual(c, orig) {
			t.Fatalf("seed %d: clone differs from original:\n%#v\n%#v", seed, orig, c)
		}
		// Property 2: changing the clone at any depth leaves the original
		// as it was.
		mutate(c)
		if !reflect.DeepEqual(orig, twin) {
			t.Fatalf("seed %d: mutating the clone changed the original:\n%#v\nwant\n%#v", seed, orig, twin)
		}
		// Property 3: and the other way round.
		c2 := twin.Clone()
		mutate(twin)
		if !reflect.DeepEqual(c2, orig) {
			t.Fatalf("seed %d: mutating the original changed the clone", seed)
		}
	}
}

func TestRecord_Clone_DeepAndTypePreserving(t *testing.T) {
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
	if !reflect.DeepEqual(rec, c) {
		t.Fatalf("clone differs:\n%#v\n%#v", rec, c)
	}
	c.Key.(StructuredData)["id"].([]any)[0] = 9
	c.Payload.After.(StructuredData)["nested"].(map[string]any)["tags"].([]any)[0] = "z"
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

func TestStructuredData_Clone_NilsStayNil(t *testing.T) {
	d := StructuredData{
		"m":  map[string]any(nil),
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

// RawData nested inside structured data is copied too, and keeps its type.
func TestStructuredData_Clone_NestedRawData(t *testing.T) {
	orig := StructuredData{"raw": RawData("abc"), "list": []any{RawData("def")}}
	c := orig.Clone().(StructuredData)
	if !reflect.DeepEqual(orig, c) {
		t.Fatalf("clone differs: %#v", c)
	}
	c["raw"].(RawData)[0] = 'X'
	c["list"].([]any)[0].(RawData)[0] = 'Y'
	if string(orig["raw"].(RawData)) != "abc" || string(orig["list"].([]any)[0].(RawData)) != "def" {
		t.Fatalf("nested RawData shared with the clone: %#v", orig)
	}
}

// Cloning a nil StructuredData gives an empty, non-nil one, as before.
func TestStructuredData_Clone_TopLevelNil(t *testing.T) {
	c := StructuredData(nil).Clone().(StructuredData)
	if c == nil || len(c) != 0 {
		t.Fatalf("got %#v, want an empty, non-nil StructuredData", c)
	}
}
