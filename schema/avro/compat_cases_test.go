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
	"time"
)

// compatCase is one schema/value pair of the wire-compatibility corpus. The
// corpus is encoded by conduit-commons v0.6.0 (github.com/hamba/avro/v2
// v2.28.0, the codec that was in use before the move to
// github.com/iskorotkov/avro/v2) and committed under
// testdata/hamba_compat_fixtures.json. See compat_test.go.
//
// This file is deliberately self-contained (no helpers from other test files)
// so that the fixture generator can compile it unchanged against the old
// release; see testdata/gen/README.md.
type compatCase struct {
	name   string
	schema string
	// value returns a fresh value every call, because Serde.Marshal may mutate
	// its argument.
	value func() any
	// want is the value Unmarshal is expected to produce from the encoded
	// bytes, when it differs from value() (for example unions, which decode
	// to the plain Go value, or ints, which decode to int64).
	want func() any
	// unordered marks values containing a map with more than one entry. Go
	// map iteration order is random and so is the entry order on the wire,
	// so the bytes are not comparable; the test checks decode and a
	// re-encode/decode round trip instead.
	unordered bool
}

func compatCases() []compatCase {
	ts := time.Date(2024, 5, 17, 13, 45, 10, 123000000, time.UTC)
	return []compatCase{{
		name:   "primitives",
		schema: `{"type":"record","name":"prims","fields":[{"name":"i","type":"int"},{"name":"l","type":"long"},{"name":"f","type":"float"},{"name":"d","type":"double"},{"name":"s","type":"string"},{"name":"b","type":"boolean"},{"name":"by","type":"bytes"}]}`,
		value: func() any {
			return map[string]any{"i": 42, "l": int64(1) << 40, "f": float32(1.5), "d": 2.25, "s": "héllo wörld", "b": true, "by": []byte{0, 1, 2, 0xff}}
		},
		want: func() any {
			return map[string]any{"i": 42, "l": int64(1) << 40, "f": float32(1.5), "d": 2.25, "s": "héllo wörld", "b": true, "by": []byte{0, 1, 2, 0xff}}
		},
	}, {
		name:   "extreme integers",
		schema: `{"type":"record","name":"ints","fields":[{"name":"imin","type":"int"},{"name":"imax","type":"int"},{"name":"lmin","type":"long"},{"name":"lmax","type":"long"}]}`,
		value: func() any {
			return map[string]any{"imin": -2147483648, "imax": 2147483647, "lmin": int64(-9223372036854775808), "lmax": int64(9223372036854775807)}
		},
		want: func() any {
			return map[string]any{"imin": -2147483648, "imax": 2147483647, "lmin": int64(-9223372036854775808), "lmax": int64(9223372036854775807)}
		},
	}, {
		name:   "array of long",
		schema: `{"type":"record","name":"arr","fields":[{"name":"a","type":{"type":"array","items":"long"}}]}`,
		value:  func() any { return map[string]any{"a": []any{int64(1), int64(-2), int64(300000000000)}} },
		want:   func() any { return map[string]any{"a": []any{int64(1), int64(-2), int64(300000000000)}} },
	}, {
		name:   "map of int",
		schema: `{"type":"record","name":"m","fields":[{"name":"m","type":{"type":"map","values":"int"}}]}`,
		value:  func() any { return map[string]any{"m": map[string]any{"a": 1}} },
		want:   func() any { return map[string]any{"m": map[string]any{"a": 1}} },
	}, {
		name:      "map of int, several entries",
		schema:    `{"type":"record","name":"m","fields":[{"name":"m","type":{"type":"map","values":"int"}}]}`,
		value:     func() any { return map[string]any{"m": map[string]any{"a": 1, "b": -2, "": 3}} },
		want:      func() any { return map[string]any{"m": map[string]any{"a": 1, "b": -2, "": 3}} },
		unordered: true,
	}, {
		name:   "nullable string, null",
		schema: `{"type":"record","name":"u","fields":[{"name":"u","type":["null","string"]}]}`,
		value:  func() any { return map[string]any{"u": nil} },
		want:   func() any { return map[string]any{"u": nil} },
	}, {
		name:   "nullable string, set",
		schema: `{"type":"record","name":"u","fields":[{"name":"u","type":["null","string"]}]}`,
		value:  func() any { return map[string]any{"u": "x"} },
		want:   func() any { return map[string]any{"u": "x"} },
	}, {
		name:   "multi-branch union, int",
		schema: `{"type":"record","name":"u","fields":[{"name":"u","type":["null","int","string"]}]}`,
		value:  func() any { return map[string]any{"u": 7} },
		want:   func() any { return map[string]any{"u": 7} },
	}, {
		name:   "multi-branch union, string",
		schema: `{"type":"record","name":"u","fields":[{"name":"u","type":["null","int","string"]}]}`,
		value:  func() any { return map[string]any{"u": "seven"} },
		want:   func() any { return map[string]any{"u": "seven"} },
	}, {
		name:   "nested record",
		schema: `{"type":"record","name":"outer","fields":[{"name":"id","type":"long"},{"name":"inner","type":{"type":"record","name":"inner","fields":[{"name":"name","type":"string"},{"name":"tags","type":{"type":"array","items":"string"}}]}}]}`,
		value: func() any {
			return map[string]any{"id": int64(9), "inner": map[string]any{"name": "n", "tags": []any{"a", "b"}}}
		},
		want: func() any {
			return map[string]any{"id": int64(9), "inner": map[string]any{"name": "n", "tags": []any{"a", "b"}}}
		},
	}, {
		name:   "array of records",
		schema: `{"type":"record","name":"outer","fields":[{"name":"rows","type":{"type":"array","items":{"type":"record","name":"row","fields":[{"name":"k","type":"string"},{"name":"v","type":"double"}]}}}]}`,
		value: func() any {
			return map[string]any{"rows": []any{map[string]any{"k": "a", "v": 0.5}, map[string]any{"k": "b", "v": -1.0}}}
		},
		want: func() any {
			return map[string]any{"rows": []any{map[string]any{"k": "a", "v": 0.5}, map[string]any{"k": "b", "v": -1.0}}}
		},
	}, {
		name:   "enum and fixed",
		schema: `{"type":"record","name":"ef","fields":[{"name":"e","type":{"type":"enum","name":"color","symbols":["RED","GREEN","BLUE"]}},{"name":"f","type":{"type":"fixed","name":"four","size":4}}]}`,
		value:  func() any { return map[string]any{"e": "GREEN", "f": [4]byte{1, 2, 3, 4}} },
		want:   func() any { return map[string]any{"e": "GREEN", "f": [4]byte{1, 2, 3, 4}} },
	}, {
		name:   "decimal",
		schema: `{"type":"record","name":"dec","fields":[{"name":"d","type":{"type":"bytes","logicalType":"decimal","precision":10,"scale":3}}]}`,
		value:  func() any { return map[string]any{"d": big.NewRat(-12345, 1000)} },
		want:   func() any { return map[string]any{"d": big.NewRat(-12345, 1000)} },
	}, {
		name:   "time-micros",
		schema: `{"type":"record","name":"tm","fields":[{"name":"t","type":{"type":"long","logicalType":"time-micros"}}]}`,
		value: func() any {
			return map[string]any{"t": 13*time.Hour + 45*time.Minute + 10*time.Second + 123456*time.Microsecond}
		},
		want: func() any {
			return map[string]any{"t": 13*time.Hour + 45*time.Minute + 10*time.Second + 123456*time.Microsecond}
		},
	}, {
		name:   "timestamp and date",
		schema: `{"type":"record","name":"tsd","fields":[{"name":"ms","type":{"type":"long","logicalType":"timestamp-millis"}},{"name":"us","type":{"type":"long","logicalType":"timestamp-micros"}},{"name":"d","type":{"type":"int","logicalType":"date"}}]}`,
		value: func() any {
			return map[string]any{"ms": ts, "us": ts, "d": time.Date(2024, 5, 17, 0, 0, 0, 0, time.UTC)}
		},
		want: func() any {
			return map[string]any{"ms": ts, "us": ts, "d": time.Date(2024, 5, 17, 0, 0, 0, 0, time.UTC)}
		},
	}, {
		name:   "empty collections",
		schema: `{"type":"record","name":"e","fields":[{"name":"a","type":{"type":"array","items":"int"}},{"name":"m","type":{"type":"map","values":"string"}},{"name":"s","type":"string"},{"name":"b","type":"bytes"}]}`,
		value:  func() any { return map[string]any{"a": []any{}, "m": map[string]any{}, "s": "", "b": []byte{}} },
		// NB: the empty array is the one known decode difference between the
		// two codecs: hamba decodes it to []any(nil), the maintained fork to
		// []any{}. See the design doc "Avro codec (hamba/avro) is archived".
		want: func() any { return map[string]any{"a": []any{}, "m": map[string]any{}, "s": "", "b": []byte{}} },
	}}
}
