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
)

// TestParse_NamedTypesDoNotLeakAcrossSchemas checks that a schema can only
// refer to named types it defines itself. Before the fix Parse registered
// every named type in the codec's process-wide schema cache, so after one
// schema defined fixed "leakfx", an unrelated schema that used "leakfx"
// without defining it parsed fine and encoded with the other schema's
// definition.
func TestParse_NamedTypesDoNotLeakAcrossSchemas(t *testing.T) {
	_, err := Parse([]byte(`{"type":"record","name":"definer","fields":[{"name":"f","type":{"type":"fixed","name":"leakfx","size":4}}]}`))
	if err != nil {
		t.Fatal(err)
	}

	_, err = Parse([]byte(`{"type":"record","name":"user","fields":[{"name":"f","type":"leakfx"}]}`))
	if err == nil {
		t.Fatal("expected an error for a schema that uses a named type it does not define, got nil")
	}
}

// TestParse_RedefinedNamedTypeUsesOwnDefinition checks that a schema that
// defines a name another schema defined earlier gets its own definition.
func TestParse_RedefinedNamedTypeUsesOwnDefinition(t *testing.T) {
	a, err := Parse([]byte(`{"type":"record","name":"ra","fields":[{"name":"f","type":{"type":"enum","name":"redefE","symbols":["X","Y"]}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	b, err := Parse([]byte(`{"type":"record","name":"rb","fields":[{"name":"f","type":{"type":"enum","name":"redefE","symbols":["Y","X","Z"]}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"name":"ra","type":"record","fields":[{"name":"f","type":{"name":"redefE","type":"enum","symbols":["X","Y"]}}]}`
	if got := a.String(); got != want {
		t.Errorf("first schema changed after the second was parsed:\ngot  %s\nwant %s", got, want)
	}
	want = `{"name":"rb","type":"record","fields":[{"name":"f","type":{"name":"redefE","type":"enum","symbols":["Y","X","Z"]}}]}`
	if got := b.String(); got != want {
		t.Errorf("second schema:\ngot  %s\nwant %s", got, want)
	}
}
