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

package schema

// Backward-compatibility proof for the two additions to schema.v1.Schema
// since v0.6.0: TYPE_PROTOBUF = 2 in the Type enum, and the repeated
// Reference references = 6 field with its nested Reference message. A reader
// built before the change (any conduit-commons <= v0.7.x, and every
// connector, processor or SDK pinned to one) round-trips a schema of type 2
// with references without losing or changing it, and fails loudly, not
// silently, when it gets as far as needing a Serde.
//
// The old reader is real, not simulated: v060SchemaRawDesc is the serialized
// file descriptor embedded in proto/schema/v1/schema.pb.go at tag v0.6.0
// (commit 431cc0bfe7721893d13b5276be1bbcb2cde1d3c6; the file is unchanged
// from there to the parent of this change). Generated proto code decodes
// using exactly this descriptor, so a dynamicpb message built from it decodes
// the way the old generated type does. It is built against a private
// registry, so it does not clash with the current schema.v1 registration.
// TestCompat_VendoredDescriptorIsThePredecessor pins that the vendored bytes
// are the current descriptor minus the new value, field and message, and
// nothing else.

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	schemav1 "github.com/conduitio/conduit-commons/proto/schema/v1"
	"github.com/matryer/is"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// v060SchemaRawDesc is file_schema_v1_schema_proto_rawDesc from
// proto/schema/v1/schema.pb.go at v0.6.0, copied verbatim. Do not regenerate.
var v060SchemaRawDesc = []byte{
	0x0a, 0x16, 0x73, 0x63, 0x68, 0x65, 0x6d, 0x61, 0x2f, 0x76, 0x31, 0x2f, 0x73, 0x63, 0x68, 0x65,
	0x6d, 0x61, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x09, 0x73, 0x63, 0x68, 0x65, 0x6d, 0x61,
	0x2e, 0x76, 0x31, 0x22, 0xbb, 0x01, 0x0a, 0x06, 0x53, 0x63, 0x68, 0x65, 0x6d, 0x61, 0x12, 0x18,
	0x0a, 0x07, 0x73, 0x75, 0x62, 0x6a, 0x65, 0x63, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x07, 0x73, 0x75, 0x62, 0x6a, 0x65, 0x63, 0x74, 0x12, 0x18, 0x0a, 0x07, 0x76, 0x65, 0x72, 0x73,
	0x69, 0x6f, 0x6e, 0x18, 0x02, 0x20, 0x01, 0x28, 0x05, 0x52, 0x07, 0x76, 0x65, 0x72, 0x73, 0x69,
	0x6f, 0x6e, 0x12, 0x0e, 0x0a, 0x02, 0x69, 0x64, 0x18, 0x03, 0x20, 0x01, 0x28, 0x05, 0x52, 0x02,
	0x69, 0x64, 0x12, 0x2a, 0x0a, 0x04, 0x74, 0x79, 0x70, 0x65, 0x18, 0x04, 0x20, 0x01, 0x28, 0x0e,
	0x32, 0x16, 0x2e, 0x73, 0x63, 0x68, 0x65, 0x6d, 0x61, 0x2e, 0x76, 0x31, 0x2e, 0x53, 0x63, 0x68,
	0x65, 0x6d, 0x61, 0x2e, 0x54, 0x79, 0x70, 0x65, 0x52, 0x04, 0x74, 0x79, 0x70, 0x65, 0x12, 0x14,
	0x0a, 0x05, 0x62, 0x79, 0x74, 0x65, 0x73, 0x18, 0x05, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x05, 0x62,
	0x79, 0x74, 0x65, 0x73, 0x22, 0x2b, 0x0a, 0x04, 0x54, 0x79, 0x70, 0x65, 0x12, 0x14, 0x0a, 0x10,
	0x54, 0x59, 0x50, 0x45, 0x5f, 0x55, 0x4e, 0x53, 0x50, 0x45, 0x43, 0x49, 0x46, 0x49, 0x45, 0x44,
	0x10, 0x00, 0x12, 0x0d, 0x0a, 0x09, 0x54, 0x59, 0x50, 0x45, 0x5f, 0x41, 0x56, 0x52, 0x4f, 0x10,
	0x01, 0x42, 0xa0, 0x01, 0x0a, 0x0d, 0x63, 0x6f, 0x6d, 0x2e, 0x73, 0x63, 0x68, 0x65, 0x6d, 0x61,
	0x2e, 0x76, 0x31, 0x42, 0x0b, 0x53, 0x63, 0x68, 0x65, 0x6d, 0x61, 0x50, 0x72, 0x6f, 0x74, 0x6f,
	0x50, 0x01, 0x5a, 0x3d, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x63,
	0x6f, 0x6e, 0x64, 0x75, 0x69, 0x74, 0x69, 0x6f, 0x2f, 0x63, 0x6f, 0x6e, 0x64, 0x75, 0x69, 0x74,
	0x2d, 0x63, 0x6f, 0x6d, 0x6d, 0x6f, 0x6e, 0x73, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x73,
	0x63, 0x68, 0x65, 0x6d, 0x61, 0x2f, 0x76, 0x31, 0x3b, 0x73, 0x63, 0x68, 0x65, 0x6d, 0x61, 0x76,
	0x31, 0xa2, 0x02, 0x03, 0x53, 0x58, 0x58, 0xaa, 0x02, 0x09, 0x53, 0x63, 0x68, 0x65, 0x6d, 0x61,
	0x2e, 0x56, 0x31, 0xca, 0x02, 0x09, 0x53, 0x63, 0x68, 0x65, 0x6d, 0x61, 0x5c, 0x56, 0x31, 0xe2,
	0x02, 0x15, 0x53, 0x63, 0x68, 0x65, 0x6d, 0x61, 0x5c, 0x56, 0x31, 0x5c, 0x47, 0x50, 0x42, 0x4d,
	0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0xea, 0x02, 0x0a, 0x53, 0x63, 0x68, 0x65, 0x6d, 0x61,
	0x3a, 0x3a, 0x56, 0x31, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

// v060UnmarshalText is Type.UnmarshalText at v0.6.0, copied verbatim apart
// from being a function (and a lint directive this file does not need): the
// text form a v0.6.0 reader accepts.
func v060UnmarshalText(t *Type, b []byte) error {
	if len(b) == 0 {
		return nil // empty string, do nothing
	}

	switch string(b) {
	case "avro": // TypeAvro.String()
		*t = TypeAvro
	default:
		// it's not a known type, but we also allow Type(int)
		valIntRaw := strings.TrimSuffix(strings.TrimPrefix(string(b), "Type("), ")")
		valInt, err := strconv.Atoi(valIntRaw)
		if err != nil {
			return fmt.Errorf("schema type %q: %w", b, ErrUnsupportedType)
		}
		*t = Type(valInt)
	}

	return nil
}

func v060SchemaFileDescriptorProto(t *testing.T) *descriptorpb.FileDescriptorProto {
	t.Helper()
	var fdp descriptorpb.FileDescriptorProto
	if err := proto.Unmarshal(v060SchemaRawDesc, &fdp); err != nil {
		t.Fatalf("unmarshal vendored v0.6.0 descriptor: %v", err)
	}
	return &fdp
}

// v060SchemaDescriptor returns the v0.6.0 schema.v1.Schema message descriptor,
// built against an empty private registry.
func v060SchemaDescriptor(t *testing.T) protoreflect.MessageDescriptor {
	t.Helper()
	fd, err := protodesc.NewFile(v060SchemaFileDescriptorProto(t), new(protoregistry.Files))
	if err != nil {
		t.Fatalf("build vendored v0.6.0 descriptor: %v", err)
	}
	md := fd.Messages().ByName("Schema")
	if md == nil {
		t.Fatal("vendored v0.6.0 descriptor has no Schema message")
	}
	return md
}

func TestCompat_VendoredDescriptorIsThePredecessor(t *testing.T) {
	is := is.New(t)

	current := protodesc.ToFileDescriptorProto(schemav1.File_schema_v1_schema_proto)
	schemaMsg := current.GetMessageType()[0]

	// remove exactly the references field (6) and the Reference message
	keptFields := make([]*descriptorpb.FieldDescriptorProto, 0, len(schemaMsg.GetField()))
	for _, f := range schemaMsg.GetField() {
		if f.GetName() == "references" && f.GetNumber() == 6 {
			continue
		}
		keptFields = append(keptFields, f)
	}
	is.Equal(len(keptFields), len(schemaMsg.GetField())-1)
	schemaMsg.Field = keptFields
	is.Equal(len(schemaMsg.GetNestedType()), 1)
	is.Equal(schemaMsg.GetNestedType()[0].GetName(), "Reference")
	schemaMsg.NestedType = nil

	var typeEnum *descriptorpb.EnumDescriptorProto
	for _, e := range schemaMsg.GetEnumType() {
		if e.GetName() == "Type" {
			typeEnum = e
		}
	}
	is.True(typeEnum != nil)

	// remove exactly TYPE_PROTOBUF = 2 and nothing else
	kept := make([]*descriptorpb.EnumValueDescriptorProto, 0, len(typeEnum.GetValue()))
	removed := 0
	for _, v := range typeEnum.GetValue() {
		if v.GetName() == "TYPE_PROTOBUF" && v.GetNumber() == 2 {
			removed++
			continue
		}
		kept = append(kept, v)
	}
	is.Equal(removed, 1)
	typeEnum.Value = kept

	if !proto.Equal(current, v060SchemaFileDescriptorProto(t)) {
		t.Fatal("current schema.proto minus TYPE_PROTOBUF and references differs from v0.6.0: the change is not purely additive")
	}
}

// TestCompat_OldReaderRoundTripsTypeProtobuf is the sign-off's required proof:
// new writer -> old reader -> old writer -> new reader, binary wire format.
func TestCompat_OldReaderRoundTripsTypeProtobuf(t *testing.T) {
	is := is.New(t)

	written := &schemav1.Schema{
		Subject: "orders-value",
		Version: 3,
		Id:      42,
		Type:    schemav1.Schema_TYPE_PROTOBUF,
		Bytes:   []byte(`syntax = "proto3"; import "customer.proto"; message Order { Customer c = 1; }`),
		References: []*schemav1.Schema_Reference{
			{Name: "customer.proto", Subject: "customer", Version: 2},
		},
	}
	deterministic := proto.MarshalOptions{Deterministic: true}
	wire, err := deterministic.Marshal(written)
	is.NoErr(err)

	// old reader
	oldMD := v060SchemaDescriptor(t)
	old := dynamicpb.NewMessage(oldMD)
	is.NoErr(proto.Unmarshal(wire, old))

	typeField := oldMD.Fields().ByName("type")
	is.True(oldMD.Enums().ByName("Type").Values().ByNumber(2) == nil) // the old reader really doesn't know 2
	is.Equal(old.Get(typeField).Enum(), protoreflect.EnumNumber(2))   // ...and keeps it anyway
	is.Equal(old.Get(oldMD.Fields().ByName("subject")).String(), "orders-value")
	is.Equal(old.Get(oldMD.Fields().ByName("version")).Int(), int64(3))
	is.Equal(old.Get(oldMD.Fields().ByName("id")).Int(), int64(42))
	is.Equal(old.Get(oldMD.Fields().ByName("bytes")).Bytes(), written.Bytes)
	// type 2 is stored as a field value, not shunted to unknown fields; the
	// references field (6), which the old reader has no descriptor for, is
	// kept as unknown bytes and re-emitted as is
	is.True(oldMD.Fields().ByNumber(6) == nil)
	is.True(len(old.GetUnknown()) > 0)

	// old writer re-emits identical bytes
	rewired, err := deterministic.Marshal(old)
	is.NoErr(err)
	is.True(bytes.Equal(rewired, wire))

	// new reader gets TYPE_PROTOBUF back
	var read schemav1.Schema
	is.NoErr(proto.Unmarshal(rewired, &read))
	is.True(proto.Equal(&read, written))

	var s Schema
	is.NoErr(s.FromProto(&read))
	is.Equal(s.Type, TypeProtobuf)
	is.Equal(s.References, []Reference{{Name: "customer.proto", Subject: "customer", Version: 2}})
}

// The same round trip for the JSON form. An old reader emits an enum value
// it doesn't know as a number, which a new reader accepts. The reverse
// direction fails loudly: an old reader rejects the new name rather than
// guessing a value.
func TestCompat_OldReaderJSON(t *testing.T) {
	is := is.New(t)

	oldMD := v060SchemaDescriptor(t)

	// old reader, numeric form: preserved
	old := dynamicpb.NewMessage(oldMD)
	is.NoErr(protojson.Unmarshal([]byte(`{"subject":"s","type":2}`), old))
	is.Equal(old.Get(oldMD.Fields().ByName("type")).Enum(), protoreflect.EnumNumber(2))

	out, err := protojson.Marshal(old)
	is.NoErr(err)
	var read schemav1.Schema
	is.NoErr(protojson.Unmarshal(out, &read))
	is.Equal(read.GetType(), schemav1.Schema_TYPE_PROTOBUF)

	// old reader, new name: rejected, not coerced
	newJSON, err := protojson.Marshal(&schemav1.Schema{Type: schemav1.Schema_TYPE_PROTOBUF})
	is.NoErr(err)
	is.True(strings.Contains(string(newJSON), "TYPE_PROTOBUF"))
	err = protojson.Unmarshal(newJSON, dynamicpb.NewMessage(oldMD))
	is.True(err != nil)

	// old reader, references: rejected as an unknown field, not dropped
	refJSON, err := protojson.Marshal(&schemav1.Schema{
		Type:       schemav1.Schema_TYPE_AVRO,
		References: []*schemav1.Schema_Reference{{Name: "a.proto", Subject: "a", Version: 1}},
	})
	is.NoErr(err)
	err = protojson.Unmarshal(refJSON, dynamicpb.NewMessage(oldMD))
	is.True(err != nil)
	is.True(strings.Contains(err.Error(), "references"))
}

// Type's text form (used by configuration such as the connector SDK's
// sdk.schema.extract.type). The old text form of 2 ("Type(2)") still reads
// as TypeProtobuf. The new form ("protobuf") is rejected by an old reader
// with ErrUnsupportedType, not mapped to some other type.
func TestCompat_TypeText(t *testing.T) {
	is := is.New(t)

	var typ Type
	is.NoErr(typ.UnmarshalText([]byte("Type(2)")))
	is.Equal(typ, TypeProtobuf)

	text, err := TypeProtobuf.MarshalText()
	is.NoErr(err)
	is.Equal(string(text), "protobuf")

	var old Type
	err = v060UnmarshalText(&old, text)
	is.True(errors.Is(err, ErrUnsupportedType))
	is.Equal(old, Type(0))
}

// What an old reader does once it has carried Type(2) forward: v0.6.0's
// FromProto is the same raw cast as today's, and its Serde() lookup misses
// for a type it has no factory for. This exercises that unchanged lookup with
// a type the current build doesn't know, which is the path v0.6.0 takes for 2.
func TestCompat_UnknownTypeFailsLoudly(t *testing.T) {
	is := is.New(t)

	var s Schema
	is.NoErr(s.FromProto(&schemav1.Schema{Type: schemav1.Schema_Type(3), Bytes: []byte(t.Name())}))
	is.Equal(s.Type, Type(3))

	_, err := s.Serde()
	is.True(errors.Is(err, ErrUnsupportedType))
}
