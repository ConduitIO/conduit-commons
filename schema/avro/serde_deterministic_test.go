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
	"testing"

	"github.com/conduitio/conduit-commons/opencdc"
	"github.com/google/go-cmp/cmp"
	"github.com/matryer/is"
)

// TestSerde_Marshal_Deterministic is the regression test for the
// non-deterministic avro.encode output under v0.9.0: the codec encodes an Avro
// map by ranging over the Go map, and Go randomizes map iteration order, so the
// same value encoded to different bytes on each run. The value below mirrors
// the avro.encode example: a record with a string, int, float, a map and a
// nested record. Marshaling it many times -- each with a fresh Serde, as the
// processor does -- must always produce the same bytes, and those bytes must
// match the sorted-key golden.
func TestSerde_Marshal_Deterministic(t *testing.T) {
	is := is.New(t)

	// A fresh value per run: Marshal may mutate its input.
	makeValue := func() opencdc.StructuredData {
		return opencdc.StructuredData{
			"myString": "bar",
			"myInt":    1,
			"myFloat":  2.3,
			"myMap": map[string]any{
				"foo": true,
				"bar": 2.2,
			},
			"myStruct": opencdc.StructuredData{
				"foo": 1,
				"bar": false,
			},
		}
	}

	// myMap's two entries are written in sorted key order ("bar" then "foo").
	want := []byte{
		0x66, 0x66, 0x66, 0x66, 0x66, 0x66, 0x02, 0x40, // myFloat (double 2.3)
		0x02,       // myInt (long 1)
		0x03, 0x26, // myMap block header: 2 entries, 19 bytes
		0x06, 0x62, 0x61, 0x72, // "bar"
		0x02,                                           // union index 1 (double)
		0x9a, 0x99, 0x99, 0x99, 0x99, 0x99, 0x01, 0x40, // 2.2
		0x06, 0x66, 0x6f, 0x6f, // "foo"
		0x00,                   // union index 0 (boolean)
		0x01,                   // true
		0x00,                   // map terminator
		0x06, 0x62, 0x61, 0x72, // myString ("bar")
		0x00, // myStruct.bar (false)
		0x02, // myStruct.foo (int 1)
	}

	for i := 0; i < 30; i++ {
		value := makeValue()
		s, err := SerdeForType(value)
		is.NoErr(err)
		got, err := s.Marshal(value)
		is.NoErr(err)
		if !bytes.Equal(got, want) {
			t.Fatalf("run %d: got %x, want %x", i, got, want)
		}

		// The bytes must round-trip back to the original value.
		var decoded opencdc.StructuredData
		is.NoErr(s.Unmarshal(got, &decoded))
		is.Equal("", cmp.Diff(opencdc.StructuredData{
			"myString": "bar",
			"myInt":    int64(1),
			"myFloat":  2.3,
			"myMap": map[string]any{
				"foo": true,
				"bar": 2.2,
			},
			"myStruct": map[string]any{
				"foo": int64(1),
				"bar": false,
			},
		}, decoded))
	}
}

// TestSerde_Marshal_Deterministic_NestedMap covers a map whose values are
// themselves maps, which flows through the union resolver and the union
// encoder -- the path that wraps map values in a single-key map before
// encoding. It must also be deterministic and round-trip.
func TestSerde_Marshal_Deterministic_NestedMap(t *testing.T) {
	is := is.New(t)
	makeValue := func() opencdc.StructuredData {
		return opencdc.StructuredData{
			"m": map[string]any{
				"b": map[string]any{"y": 2, "x": 1},
				"a": "str",
			},
		}
	}

	var first []byte
	for i := 0; i < 30; i++ {
		value := makeValue()
		s, err := SerdeForType(value)
		is.NoErr(err)
		got, err := s.Marshal(value)
		is.NoErr(err)
		if i == 0 {
			first = got
		} else if !bytes.Equal(first, got) {
			t.Fatalf("run %d: got %x, want %x", i, got, first)
		}
	}

	value := makeValue()
	s, err := SerdeForType(value)
	is.NoErr(err)
	var decoded opencdc.StructuredData
	is.NoErr(s.Unmarshal(first, &decoded))
	is.Equal("", cmp.Diff(opencdc.StructuredData{
		"m": map[string]any{
			"b": map[string]any{"y": int64(2), "x": int64(1)},
			"a": "str",
		},
	}, decoded))
}

// TestSerde_Marshal_Deterministic_TopLevelMap covers a top-level map schema
// (a map[string]any that is not opencdc.StructuredData).
func TestSerde_Marshal_Deterministic_TopLevelMap(t *testing.T) {
	is := is.New(t)
	makeValue := func() map[string]any {
		return map[string]any{"b": 2, "a": 1, "c": "x"}
	}

	var first []byte
	for i := 0; i < 30; i++ {
		value := makeValue()
		s, err := SerdeForType(value)
		is.NoErr(err)
		got, err := s.Marshal(value)
		is.NoErr(err)
		if i == 0 {
			first = got
		} else if !bytes.Equal(first, got) {
			t.Fatalf("run %d: got %x, want %x", i, got, first)
		}
	}

	value := makeValue()
	s, err := SerdeForType(value)
	is.NoErr(err)
	var decoded map[string]any
	is.NoErr(s.Unmarshal(first, &decoded))
	is.Equal("", cmp.Diff(map[string]any{"b": int64(2), "a": int64(1), "c": "x"}, decoded))
}

// TestSerde_Marshal_Deterministic_ArrayOfMaps covers an array whose items are
// maps, so the deterministic encoder recurses through the array to sort each
// map.
func TestSerde_Marshal_Deterministic_ArrayOfMaps(t *testing.T) {
	is := is.New(t)
	makeValue := func() []any {
		return []any{
			map[string]any{"y": 2, "x": 1},
			map[string]any{"b": "z", "a": "q"},
		}
	}

	var first []byte
	for i := 0; i < 30; i++ {
		value := makeValue()
		s, err := SerdeForType(value)
		is.NoErr(err)
		got, err := s.Marshal(value)
		is.NoErr(err)
		if i == 0 {
			first = got
		} else if !bytes.Equal(first, got) {
			t.Fatalf("run %d: got %x, want %x", i, got, first)
		}
	}

	// The bytes are valid Avro: decoding them must succeed. (The decoded shape
	// of a map union inside an array is the codec's native single-key map; that
	// shape is orthogonal to this encoding-determinism fix.)
	value := makeValue()
	s, err := SerdeForType(value)
	is.NoErr(err)
	var decoded []any
	is.NoErr(s.Unmarshal(first, &decoded))
}
