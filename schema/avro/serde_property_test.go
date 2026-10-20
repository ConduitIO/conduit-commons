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

package avro_test

import (
	"testing"

	"github.com/conduitio/conduit-commons/opencdc"
	"github.com/conduitio/conduit-commons/schema/avro"
	"github.com/conduitio/conduit-commons/schema/avro/avrotest"
	"github.com/google/go-cmp/cmp"
	"pgregory.net/rapid"
)

// TestSerde_PropertyRoundTripManySchemas is the property behind
// ConduitIO/conduit-commons#305: several Serdes alive in one process, whose
// schemas use the same type names in the same places for different types,
// each encode and decode their own values correctly, in any interleaving.
// Before the fix the Serdes shared the codec's encoder cache, keyed by a
// string form in which a reference is only the name, so one schema encoded
// with another's definition of the same name.
//
// The check count follows rapid: -rapid.checks or RAPID_CHECKS (default
// 100). The nightly property job runs it with many more.
func TestSerde_PropertyRoundTripManySchemas(t *testing.T) {
	rapid.Check(t, checkRoundTripManySchemas)
}

// FuzzSerde_RoundTripManySchemas is the fuzz form of
// TestSerde_PropertyRoundTripManySchemas: the fuzzer drives rapid's
// choices.
func FuzzSerde_RoundTripManySchemas(f *testing.F) {
	// rapid reads its choices from the input; an empty input draws the
	// smallest case.
	f.Add([]byte{})
	f.Add([]byte{0xff, 0x00, 0x7f, 0x80, 0x01, 0xfe, 0x10, 0x20})
	f.Fuzz(rapid.MakeFuzz(checkRoundTripManySchemas))
}

func checkRoundTripManySchemas(t *rapid.T) {
	n := rapid.IntRange(2, 4).Draw(t, "schema count")
	family := avrotest.NamedSchemaFamily("S", n, avrotest.Options{}).Draw(t, "schemas")

	// Parse every schema before encoding anything, so all of them are alive
	// in the process while each one encodes.
	serdes := make([]*avro.Serde, n)
	for i, typ := range family {
		s, err := avro.Parse([]byte(typ.Schema()))
		if err != nil {
			t.Fatalf("parse schema %d: %v\n%s", i, err, typ.Schema())
		}
		serdes[i] = s
	}

	// Every schema encodes at least once, in a drawn order, then a drawn
	// number of further encodes interleave them at random.
	order := rapid.Permutation(indexes(n)).Draw(t, "first order")
	more := rapid.SliceOfN(rapid.IntRange(0, n-1), 0, 2*n).Draw(t, "then")
	for step, i := range append(order, more...) {
		v := family[i].Value().Draw(t, "value")
		// Decoded before Marshal, which may mutate v. The root is a record,
		// so the decoded value is a map.
		want := opencdc.StructuredData(family[i].Decoded(v).(map[string]any))

		b, err := serdes[i].Marshal(v)
		if err != nil {
			t.Fatalf("step %d, schema %d: marshal: %v\n%s", step, i, err, family[i].Schema())
		}
		// Decode into StructuredData, the way connectors and the avro.decode
		// processor do.
		var got opencdc.StructuredData
		if err := serdes[i].Unmarshal(b, &got); err != nil {
			t.Fatalf("step %d, schema %d: unmarshal: %v\n%s", step, i, err, family[i].Schema())
		}
		if diff := cmp.Diff(want, got, avrotest.CmpOptions()); diff != "" {
			t.Fatalf("step %d, schema %d: decoded value differs (-want +got):\n%s\nschema: %s",
				step, i, diff, family[i].Schema())
		}
	}
}

func indexes(n int) []int {
	s := make([]int, n)
	for i := range s {
		s[i] = i
	}
	return s
}
