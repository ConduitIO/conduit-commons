// Copyright © 2023 Meroxa, Inc.
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
	"fmt"

	"github.com/hamba/avro/v2"
)

// Serde represents an Avro schema. It exposes methods for marshaling and
// unmarshalling data.
type Serde struct {
	schema        avro.Schema
	unionResolver unionResolver
}

// Marshal returns the Avro encoding of v. Note that this function may mutate v.
// Limitations:
// - Map keys need to be of type string,
// - Array values need to be of type uint8 (byte).
func (s *Serde) Marshal(v any) ([]byte, error) {
	err := s.unionResolver.BeforeMarshal(v)
	if err != nil {
		return nil, err
	}
	bytes, err := avro.Marshal(s.schema, v)
	if err != nil {
		return nil, fmt.Errorf("could not marshal into avro: %w", err)
	}
	return bytes, nil
}

// Unmarshal parses the Avro encoded data and stores the result in the value
// pointed to by v. If v is nil or not a pointer, Unmarshal returns an error.
// Note that arrays and maps are unmarshalled into slices and maps with untyped
// values (i.e. []any and map[string]any). This is a limitation of the Avro
// library used for encoding/decoding the payload.
func (s *Serde) Unmarshal(b []byte, v any) error {
	err := avro.Unmarshal(s.schema, b, v)
	if err != nil {
		return fmt.Errorf("could not unmarshal from avro: %w", err)
	}
	err = s.unionResolver.AfterUnmarshal(v)
	if err != nil {
		return err
	}
	return nil
}

// String returns the canonical form of the schema.
func (s *Serde) String() string {
	return s.schema.String()
}

// sort fields in the schema. It can be used in tests to ensure the schemas can
// be compared.
func (s *Serde) sort() {
	traverseSchema(s.schema, sortFn)
}

// Parse parses a schema byte slice.
func Parse(text []byte) (*Serde, error) {
	schema, err := avro.ParseBytes(text)
	if err != nil {
		return nil, fmt.Errorf("could not parse avro schema: %w", err)
	}
	// Note: We do not sort fields here because field order is significant in
	// Avro schemas. Sorting would alter the schema and change the output. In
	// SerdeForType, sorting ensures consistency when creating a schema from a
	// value. However, when using Parse, we must preserve the original field
	// order to match the schema definition.
	return &Serde{
		schema:        schema,
		unionResolver: newUnionResolver(schema),
	}, nil
}

// SerdeForType uses reflection to extract an Avro schema from v. Maps are
// regarded as structs.
func SerdeForType(v any) (*Serde, error) {
	schema, err := extractor{}.Extract(v)
	if err != nil {
		return nil, err
	}
	s := &Serde{
		schema:        schema,
		unionResolver: newUnionResolver(schema),
	}
	// Sort fields to ensure consistent schema representation.
	s.sort()
	return s, nil
}
