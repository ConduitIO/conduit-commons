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
	"os"
	"sort"
	"strconv"
	"testing"

	"github.com/conduitio/conduit-commons/opencdc"
)

// determinismShapes are values whose inference used to depend on Go's map
// iteration order (conduit-commons#292). Each constructor returns a fresh
// value, because Marshal may mutate its input and maps must be iterated in a
// new random order every time.
//
// alwaysFails marks shapes that are rejected on every run, by design of the
// current inference: a slice with more than one record names each record after
// its index ("record.item0", "record.item1"), so the records can never be
// combined into one union member. That is a deterministic limitation, not
// nondeterminism, and changing it would change the name (and so the schema) of
// slices that infer today. The test pins that it fails the same way every
// time.
var determinismShapes = []struct {
	name        string
	alwaysFails bool
	make        func() any
}{
	{"issue292_map_of_two_records", false, func() any {
		return opencdc.StructuredData{"a": map[string]any{
			"x": opencdc.StructuredData{"id": 1, "name": "n"},
			"y": opencdc.StructuredData{"id": 2, "name": "m"},
		}}
	}},
	{"slice_of_two_records", true, func() any {
		return []any{
			opencdc.StructuredData{"id": 1, "name": "n", "ok": true},
			opencdc.StructuredData{"id": 2, "name": "m", "ok": false},
		}
	}},
	{"map_of_many_wide_records", false, func() any {
		m := map[string]any{}
		for _, k := range []string{"a", "b", "c", "d", "e"} {
			m[k] = opencdc.StructuredData{"f1": 1, "f2": "s", "f3": 1.5, "f4": true, "f5": []byte("b"), "f6": int32(7)}
		}
		return m
	}},
	{"wide_top_level_record", false, func() any {
		sd := opencdc.StructuredData{}
		for i := range 16 {
			sd[fmt.Sprintf("f%02d", i)] = i
		}
		sd["m"] = map[string]any{"x": opencdc.StructuredData{"p": 1, "q": 2}, "y": opencdc.StructuredData{"p": 3, "q": 4}}
		return sd
	}},
	{"records_in_records_in_map", false, func() any {
		inner := func() opencdc.StructuredData {
			return opencdc.StructuredData{"deep": opencdc.StructuredData{"u": 1, "v": "w"}, "id": 1}
		}
		return opencdc.StructuredData{"m": map[string]any{"x": inner(), "y": inner(), "z": inner()}}
	}},
	{"map_of_slices_of_records", true, func() any {
		rec := func() opencdc.StructuredData { return opencdc.StructuredData{"id": 1, "name": "n"} }
		return map[string]any{
			"x": []any{rec(), rec()},
			"y": []any{rec()},
		}
	}},
	{"slices_of_slices_of_records", true, func() any {
		rec := func() opencdc.StructuredData { return opencdc.StructuredData{"id": 1, "name": "n"} }
		return []any{[]any{rec(), rec()}, []any{rec(), rec()}}
	}},
	{"mixed_scalars_and_records", false, func() any {
		rec := func() opencdc.StructuredData { return opencdc.StructuredData{"id": 1, "name": "n"} }
		return map[string]any{"a": 1, "b": "s", "c": rec(), "d": rec(), "e": nil, "f": 2.5}
	}},
	{"slice_of_maps_of_records", true, func() any {
		rec := func() opencdc.StructuredData { return opencdc.StructuredData{"id": 1, "name": "n"} }
		return []any{
			map[string]any{"x": rec(), "y": rec()},
			map[string]any{"x": rec(), "y": rec(), "z": rec()},
		}
	}},
}

// determinismIterations returns the number of iterations per shape. Override
// with CONDUIT_DETERMINISM_ITERS to run longer.
func determinismIterations() int {
	if s := os.Getenv("CONDUIT_DETERMINISM_ITERS"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return n
		}
	}
	return 1000
}

// TestSerdeForType_Deterministic infers the schema of the same logical value
// many times and requires that (1) inference never fails for a value it can
// infer (before the fix the map shapes here failed on 20% to 100% of
// runs), and (2) every run yields byte-identical schema text. (2) is a data
// integrity property (invariant 6): a record's schema is the contract
// downstream encoders and decoders use, so the same shape must never get two
// different schemas.
func TestSerdeForType_Deterministic(t *testing.T) {
	n := determinismIterations()
	for _, shape := range determinismShapes {
		t.Run(shape.name, func(t *testing.T) {
			schemas := map[string]int{}
			errs := map[string]int{}
			for range n {
				s, err := SerdeForType(shape.make())
				if err != nil {
					errs[err.Error()]++
					continue
				}
				schemas[s.String()]++
			}
			failures := 0
			for _, c := range errs {
				failures += c
			}
			t.Logf("%d iterations: %d errors (%.1f%%), %d distinct schemas among %d successes",
				n, failures, 100*float64(failures)/float64(n), len(schemas), n-failures)
			if shape.alwaysFails {
				if len(errs) != 1 || failures != n {
					t.Errorf("want the same error on all %d runs, got %d distinct errors, %d failures", n, len(errs), failures)
				}
				return
			}
			for msg, c := range errs {
				t.Errorf("inference failed %d times: %s", c, msg)
			}
			if len(schemas) > 1 {
				keys := make([]string, 0, len(schemas))
				for k := range schemas {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					t.Errorf("%d runs produced schema: %s", schemas[k], k)
				}
			}
		})
	}
}
