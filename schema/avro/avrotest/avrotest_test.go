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
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/conduitio/conduit-commons/schema/avro"
	"github.com/conduitio/conduit-commons/schema/avro/avrotest"
	"github.com/google/go-cmp/cmp"
	"pgregory.net/rapid"
)

// TestNamedSchema_PropertyParses checks that every generated schema is
// valid JSON that schema/avro parses, and that Value draws values its Serde
// encodes. A generator that emits invalid schemas would make every property
// built on it fail for the wrong reason.
func TestNamedSchema_PropertyParses(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		typ := avrotest.NamedSchema("R", avrotest.Options{}).Draw(t, "schema")
		if !json.Valid([]byte(typ.Schema())) {
			t.Fatalf("not valid JSON: %s", typ.Schema())
		}
		s, err := avro.Parse([]byte(typ.Schema()))
		if err != nil {
			t.Fatalf("parse: %v\n%s", err, typ.Schema())
		}
		if _, err := s.Marshal(typ.Value().Draw(t, "value")); err != nil {
			t.Fatalf("marshal: %v\n%s", err, typ.Schema())
		}
	})
}

// TestNamedSchemaFamily_PropertySharesLayout checks that the members of a
// family use the same field names in the same order, so that they really do
// refer to the same type names from the same places.
func TestNamedSchemaFamily_PropertySharesLayout(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		family := avrotest.NamedSchemaFamily("F", 3, avrotest.Options{}).Draw(t, "family")
		if len(family) != 3 {
			t.Fatalf("got %d schemas, want 3", len(family))
		}
		want := fieldNames(t, family[0])
		for i, typ := range family[1:] {
			if got := fieldNames(t, typ); !reflect.DeepEqual(got, want) {
				t.Fatalf("schema %d has fields %v, schema 0 has %v", i+1, got, want)
			}
		}
	})
}

// TestDecoded_PropertyDoesNotAlias checks that Decoded copies the value, so
// a property can compute it before Serde.Marshal mutates the input.
func TestDecoded_PropertyDoesNotAlias(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		typ := avrotest.NamedSchema("R", avrotest.Options{}).Draw(t, "schema")
		v := typ.Value().Draw(t, "value")
		before := typ.Decoded(v)
		snapshot := deepCopy(before)

		s, err := avro.Parse([]byte(typ.Schema()))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Marshal(v); err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(snapshot, before, avrotest.CmpOptions()); diff != "" {
			t.Fatalf("Marshal changed the result of an earlier Decoded (-before +after):\n%s", diff)
		}
	})
}

// TestOptions_PropertyNoNullableNamed checks that NoNullableNamed leaves
// every named-form union value out.
func TestOptions_PropertyNoNullableNamed(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		typ := avrotest.NamedSchema("R", avrotest.Options{NoNullableNamed: true}).Draw(t, "schema")
		v := typ.Value().Draw(t, "value")
		// Without named-form values, Decoded changes nothing.
		if diff := cmp.Diff(v, typ.Decoded(v), avrotest.CmpOptions()); diff != "" {
			t.Fatalf("value holds a named-form union value (-value +decoded):\n%s", diff)
		}
	})
}

// deepCopy copies the maps and slices of a decoded value, independently of
// Decoded, so the copy cannot share Decoded's aliasing bugs.
func deepCopy(v any) any {
	switch v := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, e := range v {
			out[k] = deepCopy(e)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, e := range v {
			out[i] = deepCopy(e)
		}
		return out
	case []byte:
		return bytes.Clone(v)
	}
	return v
}

func fieldNames(t *rapid.T, typ *avrotest.Type) []string {
	var rec struct {
		Fields []struct {
			Name string `json:"name"`
		} `json:"fields"`
	}
	if err := json.Unmarshal([]byte(typ.Schema()), &rec); err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(rec.Fields))
	for i, f := range rec.Fields {
		names[i] = f.Name
	}
	return names
}
