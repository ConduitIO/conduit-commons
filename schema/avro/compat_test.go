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
	"encoding/hex"
	"encoding/json"
	"math/big"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/matryer/is"
)

var ratComparer = cmp.Comparer(func(a, b *big.Rat) bool { return a.Cmp(b) == 0 })

// TestHambaWireCompat pins the Avro wire format across the codec swap from
// github.com/hamba/avro/v2 (archived, three unfixed decoder advisories) to
// github.com/iskorotkov/avro/v2.
//
// testdata/hamba_compat_fixtures.json holds bytes written by conduit-commons
// v0.6.0, i.e. by the old codec (regenerate-to-extend instructions in
// testdata/hamba-gen/gen.sh). For every case this test checks that
//
//  1. the current codec decodes the old bytes to the expected value, and
//  2. the current codec encodes the same value to the identical bytes.
//
// (1) guards records already written to topics, files and stores by existing
// pipelines; (2) guards consumers still running the old codec reading what we
// write now.
func TestHambaWireCompat(t *testing.T) {
	raw, err := os.ReadFile("testdata/hamba_compat_fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Name   string `json:"name"`
		Schema string `json:"schema"`
		Hex    string `json:"hex"`
	}
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}

	cases := compatCases()
	if len(fixtures) != len(cases) {
		t.Fatalf("fixture file has %d entries, compatCases() has %d: regenerate with testdata/hamba-gen/gen.sh (add-only)", len(fixtures), len(cases))
	}

	for i, c := range cases {
		f := fixtures[i]
		t.Run(c.name, func(t *testing.T) {
			is := is.New(t)
			is.Equal(f.Name, c.name)
			is.Equal(f.Schema, c.schema) // the fixture must have been encoded with this schema

			old, err := hex.DecodeString(f.Hex)
			is.NoErr(err)

			serde, err := Parse([]byte(c.schema))
			is.NoErr(err)

			// 1. old bytes decode with the current codec.
			var got any
			is.NoErr(serde.Unmarshal(old, &got))
			if diff := cmp.Diff(c.want(), got, ratComparer); diff != "" {
				t.Fatalf("decoding bytes written by hamba/avro: mismatch (-want +got):\n%s", diff)
			}

			// 2. the current codec writes the same bytes.
			now, err := serde.Marshal(c.value())
			is.NoErr(err)
			if c.unordered {
				var again any
				is.NoErr(serde.Unmarshal(now, &again))
				if diff := cmp.Diff(c.want(), again, ratComparer); diff != "" {
					t.Fatalf("re-encoded value does not round-trip (-want +got):\n%s", diff)
				}
				return
			}
			is.Equal(hex.EncodeToString(now), f.Hex)
		})
	}
}
