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

package protobuf

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/sr"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const mapBeforeNestedSchema = `syntax = "proto2";
package p;
message Root {
  message A { optional string x = 1; }
  map<string, string> m1 = 1;
  message B { optional string y = 1; }
  optional group G = 2 { optional int32 z = 1; }
  map<int32, A> m2 = 3;
  message C { optional string w = 1; }
}
message Second { message Inner { message Deep {} } }
`

// indexOf rebuilds the Confluent index of md from the descriptor tree.
func indexOf(md protoreflect.MessageDescriptor) []int {
	var out []int
	var d protoreflect.Descriptor = md
	for {
		out = append([]int{d.Index()}, out...)
		p := d.Parent()
		if _, ok := p.(protoreflect.FileDescriptor); ok {
			return out
		}
		d = p
	}
}

// FuzzMessageIndexRoundTrip fuzzes the index bytes against schemas with
// nesting, map fields and groups. Invariant: whatever message messageForIndex
// selects, its position in the descriptor tree re-encodes to exactly the bytes
// consumed (canonical form or the [0] shortcut), and decoding that encoding
// selects the same message.
func FuzzMessageIndexRoundTrip(f *testing.F) {
	schemas := []string{flatSchema, multipleMessagesSchema, nestedSchema, mapBeforeNestedSchema, proto2Schema, wellKnownImportSchema}
	srds := make([]*Serde, len(schemas))
	for i, s := range schemas {
		srd, err := Parse(context.Background(), []byte(s))
		if err != nil {
			f.Fatal(err)
		}
		srds[i] = srd
	}
	for i := range schemas {
		for _, seed := range [][]byte{{0}, {2, 0}, {4, 0, 2}, {6, 0, 2, 2}, {4, 2, 0}, {8, 0, 2, 2, 0}, {0x80, 0}, {1}} {
			f.Add(uint8(i), append(seed, 0x0a, 0x01, 0x31))
		}
	}
	f.Fuzz(func(t *testing.T, which uint8, b []byte) {
		srd := srds[int(which)%len(srds)]
		md, rest, err := srd.messageForIndex(b)
		if err != nil {
			if md != nil || rest != nil || !errors.Is(err, ErrMessageIndex) {
				t.Fatalf("bad error result: md=%v rest=%v err=%v", md, rest, err)
			}
			return
		}
		consumed := b[:len(b)-len(rest)]
		idx := indexOf(md)
		if len(idx) > srd.maxIndexDepth {
			t.Fatalf("index %v longer than depth %d", idx, srd.maxIndexDepth)
		}
		// The selected descriptor's own position must re-encode to exactly the
		// consumed bytes (either canonical explicit form or the [0] shortcut).
		explicit := appendIndex(idx)
		enc, _ := (&sr.ConfluentHeader{}).AppendEncode(nil, 1, idx)
		franz := enc[5:]
		if !bytes.Equal(consumed, explicit) && !bytes.Equal(consumed, franz) {
			t.Fatalf("consumed % x, descriptor %s index %v encodes to % x / % x", consumed, md.FullName(), idx, explicit, franz)
		}
		// And decoding franz-go's encoding of that index selects the same message.
		md2, _, err := srd.messageForIndex(append(franz, rest...))
		if err != nil || md2 != md {
			t.Fatalf("round trip failed for %v: %v", idx, err)
		}
	})
}

func appendIndex(idx []int) []byte {
	var out []byte
	out = appendVarint(out, int64(len(idx)))
	for _, i := range idx {
		out = appendVarint(out, int64(i))
	}
	return out
}

func appendVarint(b []byte, v int64) []byte {
	ux := uint64(v) << 1
	if v < 0 {
		ux = ^ux
	}
	for ux >= 0x80 {
		b = append(b, byte(ux)|0x80)
		ux >>= 7
	}
	return append(b, byte(ux))
}

// FuzzSchemaAndIndex fuzzes the schema text and the index together.
//
// Invariants: a failed lookup returns no descriptor and ErrMessageIndex; a
// successful one returns a message of this schema that is not a map entry, no
// deeper than the schema nests, and a payload that is a proper suffix.
func FuzzSchemaAndIndex(f *testing.F) {
	for _, s := range []string{
		flatSchema, nestedSchema, mapBeforeNestedSchema, noMessagesSchema,
		strings.Repeat("message M { ", 200) + strings.Repeat("}", 200),
	} {
		f.Add([]byte(s), []byte{0})
		f.Add([]byte(s), []byte{4, 0, 0})
	}
	f.Fuzz(func(t *testing.T, text, b []byte) {
		srd, err := Parse(context.Background(), text, WithCompileTimeout(2*time.Second))
		if err != nil {
			return
		}
		md, rest, err := srd.messageForIndex(b)
		if err != nil {
			if md != nil || rest != nil || !errors.Is(err, ErrMessageIndex) {
				t.Fatalf("bad error result: %v", err)
			}
			return
		}
		if md.ParentFile() != srd.file || md.IsMapEntry() || len(indexOf(md)) > srd.maxIndexDepth {
			t.Fatalf("bad descriptor %s", md.FullName())
		}
		if !bytes.HasSuffix(b, rest) || len(rest) >= len(b) {
			t.Fatal("rest not a proper suffix")
		}
	})
}

// FuzzReferenceGraph fuzzes the reference graph: up to 6 schemas, with edges
// drawn from small name and subject alphabets so cycles, diamonds, aliases
// and name collisions are common.
//
// Invariants: Parse returns a Serde or an error matching exactly one
// sentinel; it makes at most MaxReferences resolver calls; and the outcome is
// the same on a second run, because the Serde cache keeps deterministic
// errors.
func FuzzReferenceGraph(f *testing.F) {
	f.Add([]byte{0x01, 0x10, 0x21, 0x02})
	f.Add([]byte{0x01, 0x12, 0x20, 0x01})
	f.Add([]byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x05})
	f.Add([]byte{0xff, 0xff, 0xff, 0xff})
	f.Fuzz(func(t *testing.T, g []byte) {
		const nodes = 6
		names := []string{"a.proto", "b.proto", "c.proto", "schema.proto", "", "google/protobuf/any.proto"}
		edges := make([][]Reference, nodes+1) // index nodes = root
		for i, x := range g {
			if i >= 64 {
				break
			}
			from := int(x>>4) % (nodes + 1)
			to := int(x & 0x0f)
			ref := Reference{Name: names[to%len(names)], Subject: fmt.Sprintf("s%d", to%nodes), Version: 1 + (to/nodes)%2}
			edges[from] = append(edges[from], ref)
		}
		text := func(n int) []byte {
			var sb strings.Builder
			fmt.Fprintf(&sb, "syntax = \"proto3\";\npackage n%d;\n", n)
			seen := map[string]bool{}
			for _, r := range edges[n] {
				if r.Name != "" && !seen[r.Name] {
					seen[r.Name] = true
					fmt.Fprintf(&sb, "import %q;\n", r.Name)
				}
			}
			fmt.Fprintf(&sb, "message M%d {}\n", n)
			return []byte(sb.String())
		}
		var calls atomic.Int64
		resolve := func(_ context.Context, ref Reference) (ReferencedSchema, error) {
			calls.Add(1)
			var n int
			fmt.Sscanf(ref.Subject, "s%d", &n)
			return ReferencedSchema{Text: text(n), References: edges[n]}, nil
		}
		run := func() error {
			srd, err := Parse(context.Background(), text(nodes), WithReferences(edges[nodes], resolve), WithCompileTimeout(2*time.Second))
			if err != nil {
				if srd != nil {
					t.Fatal("serde and error")
				}
				if got := matchingSentinels(err); len(got) != 1 {
					t.Fatalf("error %q matches %d sentinels", err, len(got))
				}
				return err
			}
			if srd == nil {
				t.Fatal("neither")
			}
			return nil
		}
		err1 := run()
		if c := calls.Load(); c > MaxReferences {
			t.Fatalf("%d resolver calls", c)
		}
		// Determinism: non-transient errors get cached, so the same graph must
		// give the same sentinel every time.
		err2 := run()
		s1, s2 := matchingSentinels(err1), matchingSentinels(err2)
		if (err1 == nil) != (err2 == nil) || (err1 != nil && !errors.Is(s1[0], s2[0])) {
			t.Fatalf("nondeterministic: %v vs %v", err1, err2)
		}
	})
}

// Many import names for one tiny schema: the distinct-schema count stays at
// 2 and the text is empty, so neither MaxReferences nor the size cap trips.
// The work must stay linear in the entries (and capped by
// MaxReferenceEdges), not names times children.
func TestParse_References_FanOutIsBounded(t *testing.T) {
	for _, n := range []int{1000, 4000, 16000} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			leafRefs := make([]Reference, n)
			midRefs := make([]Reference, n)
			for i := range n {
				leafRefs[i] = Reference{Name: fmt.Sprintf("l%d.proto", i), Subject: "leaf", Version: 1}
				midRefs[i] = Reference{Name: fmt.Sprintf("m%d.proto", i), Subject: "mid", Version: 1}
			}
			var calls atomic.Int64
			resolve := func(_ context.Context, ref Reference) (ReferencedSchema, error) {
				calls.Add(1)
				if ref.Subject == "mid" {
					return ReferencedSchema{References: leafRefs}, nil
				}
				return ReferencedSchema{}, nil
			}

			start := time.Now()
			_, err := Parse(t.Context(), []byte(`syntax = "proto3"; message R {}`), WithReferences(midRefs, resolve))
			if elapsed := time.Since(start); elapsed > 5*time.Second {
				t.Errorf("took %s", elapsed)
			}
			if calls.Load() > 2 {
				t.Errorf("%d resolver calls, want at most 2", calls.Load())
			}
			// 2n entries: within the edge limit up to 2n <= MaxReferenceEdges.
			if want2n := 2 * n; want2n > MaxReferenceEdges {
				if !errors.Is(err, ErrReferenceLimit) {
					t.Errorf("err = %v, want ErrReferenceLimit", err)
				}
			} else if err != nil {
				t.Errorf("err = %v", err)
			}
		})
	}
}
