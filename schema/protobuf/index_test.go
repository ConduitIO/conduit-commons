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
	"errors"
	"strings"
	"testing"

	"github.com/matryer/is"
	"github.com/twmb/franz-go/pkg/sr"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// payload is an arbitrary Protobuf payload (field 1, string "1"). The message
// index tests only check that it comes back untouched after the index.
var payload = []byte{0x0a, 0x01, 0x31}

// confluentValue encodes index the way a Confluent producer does, via
// franz-go's encoder, and strips the 5-byte header as the caller of
// Serde.Unmarshal does.
func confluentValue(t *testing.T, index []int) []byte {
	t.Helper()
	b, err := (&sr.ConfluentHeader{}).AppendEncode(nil, 42, index)
	if err != nil {
		t.Fatal(err)
	}
	return append(b[5:], payload...)
}

func mustParse(t *testing.T, text string) *Serde {
	t.Helper()
	srd, err := Parse([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return srd
}

func TestMessageForIndex(t *testing.T) {
	testCases := []struct {
		name   string
		schema string
		value  []byte
		want   protoreflect.FullName
	}{
		// Hand-written bytes, so the tests don't only check franz-go
		// against itself.
		{"flat, shortcut 0", flatSchema, []byte{0x00}, "example.v1.Order"},
		{"flat, explicit [0]", flatSchema, []byte{0x02, 0x00}, "example.v1.Order"},
		{"second top-level", nestedSchema, []byte{0x02, 0x02}, "example.v1.Audit"},
		{"nested depth 2", nestedSchema, []byte{0x04, 0x02, 0x00}, "example.v1.Audit.Change"},
		{"nested depth 3", nestedSchema, []byte{0x06, 0x00, 0x02, 0x02}, "example.v1.Envelope.Body.Attachment"},
		{"nested depth 4", nestedSchema, []byte{0x08, 0x00, 0x02, 0x02, 0x00}, "example.v1.Envelope.Body.Attachment.Meta"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			is := is.New(t)
			srd := mustParse(t, tc.schema)

			md, rest, err := srd.messageForIndex(append(tc.value, payload...))
			is.NoErr(err)
			is.Equal(md.FullName(), tc.want)
			is.Equal(rest, payload)
		})
	}
}

// Every message in the schema is reachable through the index a Confluent
// producer writes for it, and the index selects that message and no other.
func TestMessageForIndex_EveryMessage(t *testing.T) {
	for _, schema := range []string{flatSchema, multipleMessagesSchema, nestedSchema} {
		srd := mustParse(t, schema)

		var walk func(mds protoreflect.MessageDescriptors, prefix []int)
		walk = func(mds protoreflect.MessageDescriptors, prefix []int) {
			for i := range mds.Len() {
				want := mds.Get(i)
				index := append(append([]int{}, prefix...), i)
				t.Run(string(want.FullName()), func(t *testing.T) {
					is := is.New(t)
					md, rest, err := srd.messageForIndex(confluentValue(t, index))
					if want.IsMapEntry() {
						is.True(errors.Is(err, ErrMessageIndex))
						is.True(strings.Contains(err.Error(), "map entry"))
						return
					}
					is.NoErr(err)
					is.Equal(md.FullName(), want.FullName())
					is.Equal(rest, payload)
				})
				walk(want.Messages(), index)
			}
		}
		walk(srd.file.Messages(), nil)
	}
}

func TestMessageForIndex_Malformed(t *testing.T) {
	testCases := []struct {
		name    string
		schema  string
		value   []byte
		wantMsg string
	}{
		{"empty", nestedSchema, nil, "empty"},
		{"truncated count varint", nestedSchema, []byte{0x80}, "not a valid varint"},
		{"count varint overflows", nestedSchema, bytes.Repeat([]byte{0xff}, 11), "not a valid varint"},
		{"negative count", nestedSchema, []byte{0x01, 0x00}, "negative count -1"},
		{"count beyond nesting depth", nestedSchema, []byte{0x0a, 0, 0, 0, 0, 0}, "count 5 exceeds"},
		{"count 1<<32+1", nestedSchema, []byte{0x82, 0x80, 0x80, 0x80, 0x20}, "exceeds"},
		{"count beyond depth of flat schema", flatSchema, []byte{0x04, 0x00, 0x00}, "count 2 exceeds"},
		{"truncated index", nestedSchema, []byte{0x04, 0x02}, "EOF"},
		{"over-long shortcut", nestedSchema, []byte{0x80, 0x00}, "non-canonical"},
		{"over-long index entry", nestedSchema, []byte{0x02, 0x82, 0x00}, "non-canonical"},
		{"top-level out of range", nestedSchema, []byte{0x02, 0x06}, "entry 0 is 3, but there are 3 messages"},
		{"negative index entry", nestedSchema, []byte{0x02, 0x01}, "entry 0 is -1"},
		{"nested out of range", nestedSchema, []byte{0x04, 0x02, 0x02}, "entry 1 is 1, but there are 1 messages"},
		{"descends below a leaf", nestedSchema, []byte{0x06, 0x00, 0x00, 0x00}, "entry 2 is 0, but there are 0 messages"},
		{"huge index entry", nestedSchema, []byte{0x02, 0x80, 0x80, 0x80, 0x80, 0x20}, ""}, // message differs on 32-bit
		{"map entry", nestedSchema, []byte{0x04, 0x04, 0x00}, "map entry example.v1.Tags.ValuesEntry"},
		{"schema without messages", noMessagesSchema, []byte{0x00}, "no messages"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			is := is.New(t)
			srd := mustParse(t, tc.schema)

			md, rest, err := srd.messageForIndex(tc.value)
			is.True(md == nil)
			is.True(rest == nil)
			is.True(errors.Is(err, ErrMessageIndex))
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Fatalf("error %q does not contain %q", err, tc.wantMsg)
			}

			// Unmarshal surfaces the same error and decodes nothing.
			var v any
			err = srd.Unmarshal(tc.value, &v)
			is.Equal(matchingUnmarshalSentinels(err), []error{ErrMessageIndex})
			is.Equal(v, nil)
		})
	}
}

func TestMessageNestingDepth(t *testing.T) {
	testCases := []struct {
		schema string
		want   int
	}{
		{noMessagesSchema, 0},
		{flatSchema, 1},
		{multipleMessagesSchema, 2},
		{nestedSchema, 4},
	}
	for _, tc := range testCases {
		is := is.New(t)
		is.Equal(mustParse(t, tc.schema).maxIndexDepth, tc.want)
	}
}

// unmarshalSentinels lists every error Serde.Unmarshal may return; each error
// must match exactly one of them.
var unmarshalSentinels = []error{ErrMessageIndex, ErrDecodeNotImplemented}

func matchingUnmarshalSentinels(err error) []error {
	var out []error
	for _, s := range unmarshalSentinels {
		if errors.Is(err, s) {
			out = append(out, s)
		}
	}
	return out
}

// FuzzMessageIndex feeds arbitrary bytes, as the value after the Confluent
// schema ID, to the message-index decoder of a schema with nested messages.
// The seeds run in every `go test`; run the fuzzer with
// `go test -run '^$' -fuzz FuzzMessageIndex ./schema/protobuf`.
//
// Invariants: it never panics; on error it returns no descriptor and an error
// matching ErrMessageIndex; on success the descriptor belongs to the schema,
// is not a map entry, and the remaining payload is a suffix of the input.
func FuzzMessageIndex(f *testing.F) {
	for _, seed := range [][]byte{
		nil,
		{0x00},
		{0x02, 0x00},
		{0x04, 0x02, 0x00},
		{0x08, 0x00, 0x02, 0x02, 0x00},
		{0x04, 0x04, 0x00},
		{0x01},
		{0x80, 0x00},
		{0x82, 0x80, 0x80, 0x80, 0x20},
		bytes.Repeat([]byte{0xff}, 11),
		append([]byte{0x00}, payload...),
	} {
		f.Add(seed)
	}

	srd, err := Parse([]byte(nestedSchema))
	if err != nil {
		f.Fatal(err)
	}

	f.Fuzz(func(t *testing.T, b []byte) {
		md, rest, err := srd.messageForIndex(b)
		if err != nil {
			if md != nil || rest != nil {
				t.Fatalf("returned a result and an error: %v", err)
			}
			if !errors.Is(err, ErrMessageIndex) {
				t.Fatalf("error %q does not match ErrMessageIndex", err)
			}
			return
		}
		if md == nil || md.ParentFile() != srd.file {
			t.Fatal("returned a descriptor outside the schema")
		}
		if md.IsMapEntry() {
			t.Fatal("returned a map entry")
		}
		if !bytes.HasSuffix(b, rest) || len(rest) >= len(b) {
			t.Fatal("remaining payload is not a proper suffix of the input")
		}
	})
}
