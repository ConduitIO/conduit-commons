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

// Package avrotest provides property-test generators for Avro schemas and
// for values those schemas accept. It is test support: production code must
// not import it.
//
// The generators are built on pgregory.net/rapid, so a failing case shrinks
// to a small schema and value, and rapid prints the seed that replays it.
// Use them from a rapid.Check property:
//
//	rapid.Check(t, func(t *rapid.T) {
//		typ := avrotest.NamedSchema("R", avrotest.Options{}).Draw(t, "schema")
//		serde, err := avro.Parse([]byte(typ.Schema()))
//		...
//		v := typ.Value().Draw(t, "value")
//		want := typ.Decoded(v) // before Marshal, which may mutate v
//		b, err := serde.Marshal(v)
//		...
//	})
//
// # What the generators produce
//
// NamedSchema builds record schemas that define named types (fixed, decimal
// fixed, enum, record) once and refer to them by name from inside arrays,
// maps, arrays of arrays, maps of arrays and nullable unions. That shape is
// where a codec cache keyed by a schema's string form goes wrong, because
// the string form of a reference is only the name
// (ConduitIO/conduit-commons#305). NamedSchemaFamily builds several such
// schemas that use the same names in the same places for different types:
// several versions of one subject, or several subjects that share a type
// name, in one process.
//
// Type.Value draws a value in the shape conduit-commons schema/avro
// Serde.Marshal accepts, and Type.Decoded says what Serde.Unmarshal returns
// for it. A property can compare the two with CmpOptions.
//
// # Relationship to schema/avro
//
// This package does not import conduit-commons schema/avro, so the tests
// inside that package can import it without an import cycle. It knows the
// Serde's value shapes (which Go types it accepts and returns) and documents
// them in Type.Value and Type.Decoded. If Serde's shapes change, change them
// here in the same PR.
package avrotest
