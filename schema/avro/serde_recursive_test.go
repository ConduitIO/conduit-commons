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
	"testing"
	"time"

	"github.com/conduitio/conduit-commons/opencdc"
)

// recursiveSchemas are schemas that refer to themselves, directly or
// through another named type.
var recursiveSchemas = []struct {
	name   string
	schema string
	value  func() opencdc.StructuredData
}{{
	name:   "linked list",
	schema: `{"type":"record","name":"P","namespace":"t","fields":[{"name":"v","type":{"type":"record","name":"L","fields":[{"name":"v","type":"long"},{"name":"next","type":["null","L"]}]}}]}`,
	value: func() opencdc.StructuredData {
		return opencdc.StructuredData{"v": map[string]any{"v": int64(1), "next": map[string]any{"t.L": map[string]any{"v": int64(2), "next": nil}}}}
	},
}, {
	name:   "top-level self reference",
	schema: `{"type":"record","name":"N","fields":[{"name":"v","type":"long"},{"name":"next","type":["null","N"]}]}`,
	value: func() opencdc.StructuredData {
		return opencdc.StructuredData{"v": int64(1), "next": nil}
	},
}, {
	name:   "tree through array and map",
	schema: `{"type":"record","name":"T","fields":[{"name":"kids","type":{"type":"array","items":"T"}},{"name":"named","type":{"type":"map","values":"T"}}]}`,
	value: func() opencdc.StructuredData {
		return opencdc.StructuredData{"kids": []any{map[string]any{"kids": []any{}, "named": map[string]any{}}}, "named": map[string]any{}}
	},
}, {
	name:   "mutual recursion",
	schema: `{"type":"record","name":"A","fields":[{"name":"b","type":["null",{"type":"record","name":"B","fields":[{"name":"a","type":["null","A"]}]}]}]}`,
	value: func() opencdc.StructuredData {
		return opencdc.StructuredData{"b": nil}
	},
}}

// TestSerde_RecursiveSchema_Terminates is the regression test for
// ConduitIO/conduit-commons#314. traverseSchema followed every
// reference, so a schema that refers to itself made Parse (and with it
// schema.Create and every Marshal) recurse until the process ran out of
// memory. Each step has a deadline so a regression fails instead of hanging
// the test binary.
func TestSerde_RecursiveSchema_Terminates(t *testing.T) {
	for _, tc := range recursiveSchemas {
		t.Run(tc.name, func(t *testing.T) {
			done := make(chan error, 1)
			go func() {
				s, err := Parse([]byte(tc.schema))
				if err != nil {
					done <- err
					return
				}
				b, err := s.Marshal(tc.value())
				if err != nil {
					done <- err
					return
				}
				var got opencdc.StructuredData
				done <- s.Unmarshal(b, &got)
			}()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(10 * time.Second):
				// The goroutine keeps recursing; fail loudly and stop the
				// binary before it takes the machine with it.
				panic("parse/marshal/unmarshal of a recursive schema did not return within 10s")
			}
		})
	}
}
