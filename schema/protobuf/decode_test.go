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
	"errors"
	"testing"
	"time"

	"github.com/conduitio/conduit-commons/opencdc"
	"github.com/matryer/is"
	"github.com/twmb/franz-go/pkg/sr"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// decodeSchema covers every field kind PB-4 maps: scalars, enum, nested
// message, repeated, map, oneof, optional presence, and the well-known types.
const decodeSchema = `syntax = "proto3";

package example.v1;

import "google/protobuf/timestamp.proto";
import "google/protobuf/duration.proto";
import "google/protobuf/struct.proto";
import "google/protobuf/wrappers.proto";
import "google/protobuf/any.proto";

enum Status {
  STATUS_UNSPECIFIED = 0;
  STATUS_ACTIVE = 1;
  STATUS_INACTIVE = 2;
}

message Inner {
  string name = 1;
}

message Everything {
  int32 i32 = 1;
  int64 i64 = 2;
  uint32 u32 = 3;
  uint64 u64 = 4;
  float f32 = 5;
  double f64 = 6;
  bool b = 7;
  string s = 8;
  bytes raw = 9;
  Status status = 10;
  Inner inner = 11;
  repeated string tags = 12;
  map<string, string> labels = 13;
  oneof choice {
    string name = 14;
    int64 count = 15;
  }
  google.protobuf.Timestamp created_at = 16;
  google.protobuf.Duration ttl = 17;
  google.protobuf.Struct attrs = 18;
  google.protobuf.StringValue note = 19;
  google.protobuf.Any packed = 20;
  optional int32 opt = 21;
}
`

// confluentPayload encodes msg under index the way a Confluent producer does
// and strips the 5-byte header, producing the exact input Serde.Unmarshal
// expects (index then payload).
func confluentPayload(t *testing.T, index []int, msg proto.Message) []byte {
	t.Helper()
	payload, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	b, err := (&sr.ConfluentHeader{}).AppendEncode(nil, 42, index)
	if err != nil {
		t.Fatal(err)
	}
	return append(b[5:], payload...)
}

func TestDecode_Everything(t *testing.T) {
	is := is.New(t)
	srd := mustParse(t, decodeSchema)

	msg := dynamicpb.NewMessage(srd.file.Messages().ByName("Everything"))
	md := msg.Descriptor()
	fields := md.Fields()

	set := func(name string, v protoreflect.Value) { msg.Set(fields.ByName(protoreflect.Name(name)), v) }
	set("i32", protoreflect.ValueOfInt32(-7))
	set("i64", protoreflect.ValueOfInt64(9007199254740993))
	set("u32", protoreflect.ValueOfUint32(7))
	set("u64", protoreflect.ValueOfUint64(9007199254740993))
	set("f32", protoreflect.ValueOfFloat32(1.5))
	set("f64", protoreflect.ValueOfFloat64(-2.25))
	set("b", protoreflect.ValueOfBool(true))
	set("s", protoreflect.ValueOfString("hi"))
	set("raw", protoreflect.ValueOfBytes([]byte{0xde, 0xad}))
	set("status", protoreflect.ValueOfEnum(1)) // STATUS_ACTIVE

	inner := dynamicpb.NewMessage(fields.ByName("inner").Message())
	inner.Set(inner.Descriptor().Fields().ByName("name"), protoreflect.ValueOfString("n"))
	set("inner", protoreflect.ValueOfMessage(inner))

	lst := msg.Mutable(fields.ByName("tags")).List()
	lst.Append(protoreflect.ValueOfString("a"))
	lst.Append(protoreflect.ValueOfString("b"))

	mp := msg.Mutable(fields.ByName("labels")).Map()
	mp.Set(protoreflect.ValueOfString("k").MapKey(), protoreflect.ValueOfString("v"))

	set("name", protoreflect.ValueOfString("chosen")) // oneof choice

	set("created_at", protoreflect.ValueOfMessage(wellKnown(t, timestamppb.New(time.Unix(1700000000, 123)))))
	set("ttl", protoreflect.ValueOfMessage(wellKnown(t, durationpb.New(90*time.Second))))
	set("attrs", protoreflect.ValueOfMessage(wellKnown(t, mustStruct(t, map[string]any{"x": float64(1)}))))
	set("note", protoreflect.ValueOfMessage(wellKnown(t, wrapperspb.String("note"))))
	set("opt", protoreflect.ValueOfInt32(5))

	var got opencdc.StructuredData
	is.NoErr(srd.Unmarshal(confluentPayload(t, []int{1}, msg), &got))

	is.Equal(got["i32"], int32(-7))
	is.Equal(got["i64"], int64(9007199254740993))
	is.Equal(got["u32"], uint32(7))
	is.Equal(got["u64"], uint64(9007199254740993))
	is.Equal(got["f32"], float32(1.5))
	is.Equal(got["f64"], -2.25)
	is.Equal(got["b"], true)
	is.Equal(got["s"], "hi")
	is.Equal(got["raw"], []byte{0xde, 0xad})
	is.Equal(got["status"], "STATUS_ACTIVE")
	is.Equal(got["inner"], opencdc.StructuredData{"name": "n"})
	is.Equal(got["tags"], []any{"a", "b"})
	is.Equal(got["labels"], map[string]any{"k": "v"})
	is.Equal(got["name"], "chosen") // oneof: only the set member is emitted
	_, hasCount := got["count"]
	is.True(!hasCount) // the unset oneof member is absent
	is.Equal(got["created_at"], time.Unix(1700000000, 123))
	is.Equal(got["ttl"], 90*time.Second)
	is.Equal(got["attrs"], map[string]any{"x": float64(1)})
	is.Equal(got["note"], "note") // wrapper unwrapped
	is.Equal(got["opt"], int32(5))
}

// TestDecode_OneofUnsetAndOptionalAbsent pins presence: an unset oneof member
// and an unset optional field are absent, while a plain proto3 scalar is always
// present (with its zero value).
func TestDecode_OneofUnsetAndOptionalAbsent(t *testing.T) {
	is := is.New(t)
	srd := mustParse(t, decodeSchema)

	msg := dynamicpb.NewMessage(srd.file.Messages().ByName("Everything"))
	// Leave the oneof unset and opt unset; only a plain scalar is present.

	var got opencdc.StructuredData
	is.NoErr(srd.Unmarshal(confluentPayload(t, []int{1}, msg), &got))

	_, hasChoice := got["name"]
	_, hasCount := got["count"]
	_, hasOpt := got["opt"]
	is.True(!hasChoice && !hasCount && !hasOpt)
	is.Equal(got["i32"], int32(0)) // plain proto3 scalar has no presence
}

// TestDecode_UnknownFieldRejects pins invariant 6: a payload carrying a field
// the resolved descriptor does not declare fails with ErrUnknownField rather
// than silently dropping it.
func TestDecode_UnknownFieldRejects(t *testing.T) {
	is := is.New(t)
	srd := mustParse(t, flatSchema)

	// field 1 (id) = "x", then field 99 (unknown, wire type 0) = 1.
	b := append([]byte{0x00}, []byte{0x0a, 0x01, 0x78, 0x98, 0x06, 0x01}...)

	var v opencdc.StructuredData
	err := srd.Unmarshal(b, &v)
	is.True(errors.Is(err, ErrUnknownField))
	is.Equal(v, nil)
}

// TestDecode_MalformedPayloadRejects pins that a payload that does not parse
// against the descriptor fails with ErrDecode (loud, not silent).
func TestDecode_MalformedPayloadRejects(t *testing.T) {
	is := is.New(t)
	srd := mustParse(t, flatSchema)

	// field 1 declares a string, but the bytes are a truncated varint for a
	// different wire type.
	b := append([]byte{0x00}, []byte{0x0a, 0x05, 0xff, 0xff, 0xff, 0xff}...)

	var v opencdc.StructuredData
	err := srd.Unmarshal(b, &v)
	is.True(errors.Is(err, ErrDecode))
	is.Equal(v, nil)
}

// wellKnown converts a generated well-known message to its dynamic form.
func wellKnown(t *testing.T, m proto.Message) protoreflect.Message {
	t.Helper()
	b, err := proto.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	dm := dynamicpb.NewMessage(m.ProtoReflect().Descriptor())
	if err := proto.Unmarshal(b, dm); err != nil {
		t.Fatal(err)
	}
	return dm
}

func mustStruct(t *testing.T, m map[string]any) *structpb.Struct {
	t.Helper()
	s, err := structpb.NewStruct(m)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
