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
	"strings"
	"testing"
)

// FuzzParse feeds arbitrary bytes to Parse, the boundary where network-supplied
// schema text enters. The seeds run in every `go test`; run the fuzzer with
// `go test -run '^$' -fuzz FuzzParse ./schema/protobuf`.
//
// Invariants: Parse never panics; it returns either a Serde that reproduces
// its input or an error matching exactly one sentinel, never both and never
// neither.
func FuzzParse(f *testing.F) {
	for _, seed := range []string{
		flatSchema,
		multipleMessagesSchema,
		wellKnownImportSchema,
		proto2Schema,
		referenceSchema,
		selfImportSchema,
		malformedSchema,
		duplicateFieldNumberSchema,
		unknownTypeSchema,
		``,
		`syntax = "proto3";`,
		`syntax = "proto4";`,
		`edition = "2023"; message M { string s = 1; }`,
		`syntax = "proto3"; message M { map<string, M> m = 1; oneof o { int32 a = 2; string b = 3; } }`,
		`syntax = "proto3"; enum E { E_UNSPECIFIED = 0; } message M { E e = 1; }`,
		`syntax = "proto3"; message M { string s = 536870912; }`,
		`syntax = "proto3"; import "google/protobuf/any.proto"; message M { google.protobuf.Any a = 1; }`,
		`syntax = "proto3"; message M { reserved 1 to max; }`,
		"syntax = \"proto3\"; message M { string s = 1 [default = \"\xff\"]; }",
		strings.Repeat("message M { ", 100) + strings.Repeat("}", 100),
		`{"type": "record", "name": "avro"}`,
	} {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, text []byte) {
		srd, err := Parse(text)
		if err != nil {
			if srd != nil {
				t.Fatalf("Parse returned both a Serde and an error: %v", err)
			}
			if got := matchingSentinels(err); len(got) != 1 {
				t.Fatalf("error %q matches %d sentinels, want exactly 1", err, len(got))
			}
			return
		}
		if srd == nil || srd.file == nil {
			t.Fatal("Parse returned neither a Serde nor an error")
		}
		if srd.String() != string(text) {
			t.Fatal("Serde.String does not reproduce the input")
		}
	})
}
