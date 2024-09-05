// Copyright © 2024 Meroxa, Inc.
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

package schema

import (
	schemav1 "github.com/conduitio/conduit-commons/proto/schema/v1"
)

func _() {
	// An "invalid array index" compiler error signifies that the constant values have changed.
	var cTypes [1]struct{}
	// Compatibility between the schema type in conduit-commons and the Protobuf schema type
	_ = cTypes[int(TypeAvro)-int(schemav1.Schema_TYPE_AVRO)]
}

// -- From Proto To Schema ----------------------------------------------------

// FromProto takes data from the supplied proto object and populates the
// receiver. If the proto object is nil, the receiver is set to its zero value.
// If the function returns an error, the receiver could be partially populated.
func (s *Schema) FromProto(proto *schemav1.Schema) error {
	if proto == nil {
		*s = Schema{}
		return nil
	}

	s.Subject = proto.Subject
	s.Version = int(proto.Version)
	s.ID = int(proto.Id)
	s.Type = Type(proto.Type)
	s.Bytes = proto.Bytes

	return nil
}

// -- From Schema To Proto ----------------------------------------------------

// ToProto takes data from the receiver and populates the supplied proto object.
// If the function returns an error, the proto object could be partially
// populated.
func (s *Schema) ToProto(proto *schemav1.Schema) error {
	if proto == nil {
		return ErrInvalidProtoIsNil
	}

	proto.Subject = s.Subject
	proto.Version = int32(s.Version) //nolint:gosec // no risk of overflow
	proto.Id = int32(s.ID)           //nolint:gosec // no risk of overflow
	proto.Type = schemav1.Schema_Type(s.Type)
	proto.Bytes = s.Bytes

	return nil
}
