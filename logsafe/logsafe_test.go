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

package logsafe

import (
	"math/big"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/matryer/is"
)

func TestHash(t *testing.T) {
	is := is.New(t)

	h := Hash([]byte("customer-SECRET-4711"))
	is.Equal(len(h), 16)
	is.True(!strings.Contains(h, "SECRET"))
	is.Equal(h, Hash([]byte("customer-SECRET-4711")))  // stable within the process
	is.True(h != Hash([]byte("customer-SECRET-4712"))) // distinct inputs differ
	is.Equal(len(Hash(nil)), 16)                       // nil does not panic
	is.Equal(HashString("customer-SECRET-4711"), h)
}

func TestHashValue(t *testing.T) {
	is := is.New(t)

	a := HashValue(map[string]any{"tenant": "acme", "id": int64(42)})
	for i := 0; i < 20; i++ {
		is.Equal(a, HashValue(map[string]any{"id": int64(42), "tenant": "acme"})) // map order does not matter
	}
	is.True(a != HashValue(map[string]any{"id": int64(43), "tenant": "acme"}))
	is.True(HashValue(map[string]any{"id": int64(1)}) != HashValue(map[string]any{"id": "1"})) // type matters
}

func TestHashValue_Canonical(t *testing.T) {
	is := is.New(t)

	// a pointer is its value, not its address
	a, b := "customer-SECRET-4711", "customer-SECRET-4711"
	is.Equal(HashValue(&a), HashValue(&b))
	is.Equal(HashValue(&a), HashValue(a))

	// an instant is the same instant in any time zone
	utc := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	la := utc.In(time.FixedZone("x", -7*3600))
	is.Equal(HashValue(utc), HashValue(la))
	is.True(HashValue(utc) != HashValue(utc.Add(time.Second)))

	// nested values, slices and bytes
	v1 := map[string]any{"id": []int{1, 2}, "k": []byte("x"), "m": map[string]any{"a": 1.5}}
	v2 := map[string]any{"m": map[string]any{"a": 1.5}, "k": []byte("x"), "id": []int{1, 2}}
	is.Equal(HashValue(v1), HashValue(v2))
	is.True(HashValue(v1) != HashValue(map[string]any{"id": []int{1, 3}, "k": []byte("x"), "m": map[string]any{"a": 1.5}}))

	// no ambiguity between concatenations
	is.True(HashValue([]string{"ab", "c"}) != HashValue([]string{"a", "bc"}))
	// nil values do not panic and differ from empty strings
	is.True(HashValue(nil) != HashValue(""))
	is.True(HashValue((*string)(nil)) == HashValue(nil))
}

func TestHashValue_Cycles(t *testing.T) {
	is := is.New(t)

	m := map[string]any{"id": 1}
	m["self"] = m
	is.Equal(len(HashValue(m)), 16) // does not overflow the stack

	s := make([]any, 2)
	s[0], s[1] = 1, s
	is.Equal(len(HashValue(s)), 16)

	type node struct{ next *node }
	n := &node{}
	n.next = n
	is.Equal(len(HashValue(n)), 16)

	// a cycle is a function of the shape, not of the address
	m2 := map[string]any{"id": 1}
	m2["self"] = m2
	is.Equal(HashValue(m), HashValue(m2))
	is.True(HashValue(m) != HashValue(map[string]any{"id": 2, "self": nil}))
}

func TestHashValue_TypesWithUnexportedFields(t *testing.T) {
	is := is.New(t)

	is.True(HashValue(big.NewInt(1)) != HashValue(big.NewInt(2)))
	is.Equal(HashValue(big.NewInt(42)), HashValue(*big.NewInt(42)))
	is.Equal(HashValue(big.NewInt(42)), HashValue(big.NewInt(42)))

	is.True(HashValue(big.NewRat(1, 3)) != HashValue(big.NewRat(2, 3)))
	is.True(HashValue(new(big.Float).SetFloat64(1.5)) != HashValue(new(big.Float).SetFloat64(2.5)))

	a, b := netip.MustParseAddr("10.0.0.1"), netip.MustParseAddr("10.0.0.2")
	is.True(HashValue(a) != HashValue(b))

	// nested in a key map
	is.True(HashValue(map[string]any{"id": big.NewInt(1)}) != HashValue(map[string]any{"id": big.NewInt(2)}))
}
