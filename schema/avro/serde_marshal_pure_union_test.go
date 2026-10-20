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
	"reflect"
	"sync"
	"testing"
	"time"
)

// These tests cover the union shapes that Serde.Marshal wraps into the
// codec's named form (a nullable enum, a nullable fixed, a union with two time
// branches) and a *any root. Marshal wraps them on a private copy of the
// value (ConduitIO/conduit-commons#312 on top of #342), so it must neither
// modify its input nor race with another Marshal of the same value.

const (
	pureEnumSchema = `{"type":"record","name":"r","fields":[
		{"name":"id","type":"long"},
		{"name":"e","type":["null",{"type":"enum","name":"E","symbols":["A","B"]}]},
		{"name":"l","type":{"type":"array","items":["null",{"type":"enum","name":"E2","symbols":["X","Y"]}]}}]}`
	pureFixedSchema = `{"type":"record","name":"r","fields":[
		{"name":"id","type":"long"},
		{"name":"f","type":["null",{"type":"fixed","name":"F","size":2}]},
		{"name":"m","type":{"type":"map","values":["null",{"type":"fixed","name":"F2","size":2}]}}]}`
	pureTimeSchema = `{"type":"record","name":"r","fields":[
		{"name":"id","type":"long"},
		{"name":"t","type":["null",{"type":"long","logicalType":"timestamp-millis"},{"type":"int","logicalType":"date"}]},
		{"name":"l","type":{"type":"array","items":["null",{"type":"long","logicalType":"timestamp-millis"},{"type":"int","logicalType":"date"}]}}]}`
	pureRootEnumSchema = `["null",{"type":"enum","name":"E","symbols":["A","B"]}]`
)

// purityClone is testClone, plus *any.
func purityClone(v any) any {
	if p, ok := v.(*any); ok {
		c := purityClone(*p)
		return &c
	}
	return testClone(v)
}

type purityCase struct {
	name   string
	schema string
	value  func() any // returns a fresh value on every call
}

func purityCases() []purityCase {
	ts := time.Date(2026, 10, 19, 12, 30, 15, 0, time.UTC)
	return []purityCase{
		{
			name:   "null-enum",
			schema: pureEnumSchema,
			value: func() any {
				return map[string]any{"id": int64(1), "e": "A", "l": []any{"X", nil, "Y"}}
			},
		},
		{
			name:   "null-fixed",
			schema: pureFixedSchema,
			value: func() any {
				return map[string]any{
					"id": int64(1),
					"f":  [2]byte{1, 2},
					"m":  map[string]any{"a": [2]byte{3, 4}, "b": nil},
				}
			},
		},
		{
			name:   "time-branches",
			schema: pureTimeSchema,
			value: func() any {
				return map[string]any{"id": int64(1), "t": ts, "l": []any{ts, nil, ts.Add(time.Hour)}}
			},
		},
		{
			name:   "any-root-record",
			schema: pureEnumSchema,
			value: func() any {
				var root any = map[string]any{"id": int64(1), "e": "B", "l": []any{"Y"}}
				return &root
			},
		},
		{
			name:   "any-root-union",
			schema: pureRootEnumSchema,
			value: func() any {
				var root any = "A"
				return &root
			},
		},
	}
}

// TestSerde_Marshal_PureUnionShapes checks that Marshal leaves its input as it
// was and that the same value encodes to the same bytes every time.
func TestSerde_Marshal_PureUnionShapes(t *testing.T) {
	for _, tc := range purityCases() {
		t.Run(tc.name, func(t *testing.T) {
			srd := mustParse(t, tc.schema)
			v := tc.value()
			before := purityClone(v)

			first, err := srd.Marshal(v)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, v) {
				t.Fatalf("Marshal changed its input:\nbefore %#v\nafter  %#v", before, v)
			}
			second, err := srd.Marshal(v)
			if err != nil {
				t.Fatalf("second Marshal of the same value: %v", err)
			}
			if !sameEncoding(srd, first, second) {
				t.Fatalf("second Marshal differs: %x vs %x", first, second)
			}
			if !reflect.DeepEqual(before, v) {
				t.Fatalf("second Marshal changed its input: %#v", v)
			}
		})
	}
}

// TestSerde_Marshal_ConcurrentUnionShapes encodes one value from several
// goroutines. Run it with -race: before the copy, Marshal wrote the named form
// into the shared containers.
func TestSerde_Marshal_ConcurrentUnionShapes(t *testing.T) {
	for _, tc := range purityCases() {
		t.Run(tc.name, func(t *testing.T) {
			srd := mustParse(t, tc.schema)
			// want comes from a separate value, so v is still unwrapped when
			// the goroutines start: a Marshal that wraps v in place would
			// then write to it from all of them.
			want, err := srd.Marshal(tc.value())
			if err != nil {
				t.Fatal(err)
			}
			v := tc.value()
			const workers, rounds = 8, 50
			errs := make(chan error, workers*rounds)
			var wg sync.WaitGroup
			for range workers {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for range rounds {
						got, err := srd.Marshal(v)
						switch {
						case err != nil:
							errs <- err
						case !sameEncoding(srd, got, want):
							errs <- fmt.Errorf("concurrent Marshal differs from the first: %x vs %x", got, want)
						}
					}
				}()
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				t.Error(err)
			}
		})
	}
}
