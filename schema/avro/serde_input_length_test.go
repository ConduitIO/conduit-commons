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
	"errors"
	"testing"

	"github.com/conduitio/conduit-commons/opencdc"
)

// lengthCases are schemas with one valid encoding each. They cover every
// way a value can end: a varint, a length-prefixed string or bytes, a
// fixed, a block-encoded array or map, a union index, and a record with
// several fields.
var lengthCases = []struct {
	name   string
	schema string
	valid  []byte
}{
	{"string", `{"type":"record","name":"r","fields":[{"name":"f","type":"string"}]}`, []byte{0x06, 'a', 'b', 'c'}},
	{"int", `{"type":"record","name":"r","fields":[{"name":"f","type":"int"}]}`, []byte{0x02}},
	{"long", `{"type":"record","name":"r","fields":[{"name":"f","type":"long"}]}`, []byte{0xfe, 0xff, 0x03}},
	{"double", `{"type":"record","name":"r","fields":[{"name":"f","type":"double"}]}`, []byte{0, 0, 0, 0, 0, 0, 0xf8, 0x3f}},
	{"bytes", `{"type":"record","name":"r","fields":[{"name":"f","type":"bytes"}]}`, []byte{0x04, 0x01, 0x02}},
	{"fixed", `{"type":"record","name":"r","fields":[{"name":"f","type":{"type":"fixed","name":"fx","size":3}}]}`, []byte{1, 2, 3}},
	{"array of int", `{"type":"record","name":"r","fields":[{"name":"f","type":{"type":"array","items":"int"}}]}`, []byte{0x04, 0x02, 0x04, 0x00}},
	{"map of string", `{"type":"record","name":"r","fields":[{"name":"f","type":{"type":"map","values":"string"}}]}`, []byte{0x02, 0x02, 'k', 0x02, 'v', 0x00}},
	{"nullable string", `{"type":"record","name":"r","fields":[{"name":"f","type":["null","string"]}]}`, []byte{0x02, 0x02, 'x'}},
	{"two fields", `{"type":"record","name":"r","fields":[{"name":"f","type":"int"},{"name":"g","type":"string"}]}`, []byte{0x02, 0x02, 'x'}},
}

// TestSerde_Unmarshal_RejectsTruncatedInput is the regression test for
// ConduitIO/conduit-commons#304. Every strict prefix of a valid encoding is
// missing bytes the schema requires, and must fail. Before the fix the
// codec reported running out of input as io.EOF and treated that as
// success, so an empty payload decoded to {"f": nil} and was written
// downstream as if it were a record.
func TestSerde_Unmarshal_RejectsTruncatedInput(t *testing.T) {
	for _, tc := range lengthCases {
		t.Run(tc.name, func(t *testing.T) {
			s, err := Parse([]byte(tc.schema))
			if err != nil {
				t.Fatal(err)
			}
			var full opencdc.StructuredData
			if err := s.Unmarshal(tc.valid, &full); err != nil {
				t.Fatalf("valid encoding: %v", err)
			}
			for n := range len(tc.valid) {
				var got opencdc.StructuredData
				err := s.Unmarshal(tc.valid[:n], &got)
				if err == nil {
					t.Errorf("%d of %d bytes: want an error, got %v", n, len(tc.valid), got)
				} else if !errors.Is(err, ErrTruncatedInput) {
					t.Errorf("%d of %d bytes: want ErrTruncatedInput, got %v", n, len(tc.valid), err)
				}
			}
		})
	}
}

// TestSerde_Unmarshal_RejectsTrailingBytes checks that bytes left over after
// a complete value are an error. They mean the payload was not written with
// this schema, so decoding it as if it were would drop data.
func TestSerde_Unmarshal_RejectsTrailingBytes(t *testing.T) {
	for _, tc := range lengthCases {
		t.Run(tc.name, func(t *testing.T) {
			s, err := Parse([]byte(tc.schema))
			if err != nil {
				t.Fatal(err)
			}
			for _, extra := range [][]byte{{0x00}, {0xff, 0xff}} {
				in := append(append([]byte{}, tc.valid...), extra...)
				var got opencdc.StructuredData
				err := s.Unmarshal(in, &got)
				if !errors.Is(err, ErrTrailingBytes) {
					t.Errorf("valid encoding + %x: want ErrTrailingBytes, got %v (value %v)", extra, err, got)
				}
			}
		})
	}
}

// TestSerde_Unmarshal_IssueCases pins the three inputs from #304.
func TestSerde_Unmarshal_IssueCases(t *testing.T) {
	testCases := []struct {
		name    string
		schema  string
		in      []byte
		wantErr error
	}{
		{"empty payload", `{"type":"record","name":"r","fields":[{"name":"f","type":"string"}]}`, []byte{}, ErrTruncatedInput},
		{"nil payload", `{"type":"record","name":"r","fields":[{"name":"f","type":"string"}]}`, nil, ErrTruncatedInput},
		{"array claims 1,000,000 items, has none", `{"type":"record","name":"r","fields":[{"name":"f","type":{"type":"array","items":"int"}}]}`, []byte{0x80, 0x89, 0x7a}, ErrTruncatedInput},
		{"trailing garbage", `{"type":"record","name":"r","fields":[{"name":"f","type":"int"}]}`, []byte{0x02, 0xff, 0xff}, ErrTrailingBytes},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			s, err := Parse([]byte(tc.schema))
			if err != nil {
				t.Fatal(err)
			}
			var got opencdc.StructuredData
			err = s.Unmarshal(tc.in, &got)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("want %v, got %v (value %v)", tc.wantErr, err, got)
			}
		})
	}
}

// TestSerde_Unmarshal_EmptyEncodingsStillDecode checks that a value whose
// encoding is zero bytes long still decodes from empty input.
func TestSerde_Unmarshal_EmptyEncodingsStillDecode(t *testing.T) {
	for _, schema := range []string{
		`{"type":"record","name":"r","fields":[]}`,
		`{"type":"record","name":"r","fields":[{"name":"f","type":"null"}]}`,
	} {
		s, err := Parse([]byte(schema))
		if err != nil {
			t.Fatal(err)
		}
		var got opencdc.StructuredData
		if err := s.Unmarshal(nil, &got); err != nil {
			t.Errorf("%s: %v", schema, err)
		}
	}
}

// FuzzSerde_Unmarshal decodes arbitrary bytes with a fixed set of schemas.
// The properties: decoding never panics, and if it succeeds, the value
// encodes again and decodes to the same value from exactly the encoded
// bytes. Before the fix, truncated input decoded to values with nil fields
// that could not be encoded again.
func FuzzSerde_Unmarshal(f *testing.F) {
	serdes := make([]*Serde, len(lengthCases))
	for i, tc := range lengthCases {
		s, err := Parse([]byte(tc.schema))
		if err != nil {
			f.Fatal(err)
		}
		serdes[i] = s
		f.Add(uint8(i), tc.valid)
		f.Add(uint8(i), tc.valid[:len(tc.valid)/2])
		f.Add(uint8(i), append(append([]byte{}, tc.valid...), 0))
	}
	f.Add(uint8(6), []byte{0x80, 0x89, 0x7a})
	f.Fuzz(func(t *testing.T, which uint8, in []byte) {
		s := serdes[int(which)%len(serdes)]
		var v opencdc.StructuredData
		if err := s.Unmarshal(in, &v); err != nil {
			return
		}
		out, err := s.Marshal(v.Clone())
		if err != nil {
			t.Fatalf("decoded %x to %v, which does not encode again: %v", in, v, err)
		}
		var v2 opencdc.StructuredData
		if err := s.Unmarshal(out, &v2); err != nil {
			t.Fatalf("re-encoded %v to %x, which does not decode: %v", v, out, err)
		}
	})
}
