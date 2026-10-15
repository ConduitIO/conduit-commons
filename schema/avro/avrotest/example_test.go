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

package avrotest_test

import (
	"fmt"

	"github.com/conduitio/conduit-commons/opencdc"
	"github.com/conduitio/conduit-commons/schema/avro"
	"github.com/conduitio/conduit-commons/schema/avro/avrotest"
	"github.com/google/go-cmp/cmp"
)

// Example shows the round trip a property checks, outside rapid: draw a
// schema and a value, compute the expected decoding before Marshal, encode,
// decode, and compare. In a test, the same steps go inside rapid.Check, with
// Draw instead of Example, so a failure shrinks and prints its seed.
//
// The drawn schema and value depend on rapid's internals, so the example
// prints only the outcome, which holds for every seed.
func Example() {
	ok := 0
	for seed := range 20 {
		typ := avrotest.NamedSchema("R", avrotest.Options{}).Example(seed)
		serde, err := avro.Parse([]byte(typ.Schema()))
		if err != nil {
			fmt.Println("parse:", err)
			return
		}

		v := typ.Value().Example(seed)
		want := opencdc.StructuredData(typ.Decoded(v).(map[string]any)) // before Marshal, which may mutate v

		b, err := serde.Marshal(v)
		if err != nil {
			fmt.Println("marshal:", err)
			return
		}
		var got opencdc.StructuredData
		if err := serde.Unmarshal(b, &got); err != nil {
			fmt.Println("unmarshal:", err)
			return
		}
		if cmp.Equal(want, got, avrotest.CmpOptions()) {
			ok++
		}
	}
	fmt.Printf("%d of 20 values round-tripped\n", ok)
	// Output: 20 of 20 values round-tripped
}
