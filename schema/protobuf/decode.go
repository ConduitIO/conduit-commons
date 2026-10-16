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
	"fmt"
	"time"

	"github.com/conduitio/conduit-commons/opencdc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// decodePayload builds the message md selects, unmarshals payload into it, and
// walks it into opencdc.StructuredData.
//
// Invariant 6 (schema handling never silently mangles data): a payload whose
// wire carries fields the resolved descriptor does not know fails with
// ErrUnknownField rather than dropping them; a payload that does not parse
// against the descriptor fails with ErrDecode rather than guessing a mapping.
func decodePayload(md protoreflect.MessageDescriptor, payload []byte) (opencdc.StructuredData, error) {
	msg := dynamicpb.NewMessage(md)
	if err := proto.Unmarshal(payload, msg); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDecode, err)
	}
	if unknown := msg.GetUnknown(); len(unknown) > 0 {
		return nil, fmt.Errorf("%w: %d bytes of fields absent from the schema", ErrUnknownField, len(unknown))
	}
	return messageToStructuredData(msg), nil
}

// messageToStructuredData walks a decoded message into opencdc.StructuredData.
// A field with presence (proto3 optional, proto2, singular message, or a oneof
// member) is emitted only when set; a plain proto3 scalar, repeated, or map
// field has no presence and is always emitted (with its zero value). See the
// design doc's "Message decoding" section.
func messageToStructuredData(msg protoreflect.Message) opencdc.StructuredData {
	md := msg.Descriptor()
	out := make(opencdc.StructuredData, md.Fields().Len())
	fields := md.Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		if fd.HasPresence() && !msg.Has(fd) {
			continue
		}
		out[string(fd.Name())] = fieldValue(msg, fd)
	}
	return out
}

// fieldValue returns the Go value of fd in msg, handling lists and maps.
func fieldValue(msg protoreflect.Message, fd protoreflect.FieldDescriptor) any {
	switch {
	case fd.IsList():
		lst := msg.Get(fd).List()
		out := make([]any, 0, lst.Len())
		for i := 0; i < lst.Len(); i++ {
			out = append(out, scalarValue(fd, lst.Get(i)))
		}
		return out
	case fd.IsMap():
		mp := msg.Get(fd).Map()
		out := make(map[string]any, mp.Len())
		mp.Range(func(k protoreflect.MapKey, v protoreflect.Value) bool {
			// Proto map keys are string/int/bool by spec; String() keeps them
			// JSON-object-safe in StructuredData.
			out[k.String()] = scalarValue(fd.MapValue(), v)
			return true
		})
		return out
	default:
		return scalarValue(fd, msg.Get(fd))
	}
}

// scalarValue maps a single protoreflect.Value to its native Go value. Enums
// decode by name (matching protojson's canonical mapping); an enum number with
// no name in the schema keeps its wire value so it is never silently dropped.
func scalarValue(fd protoreflect.FieldDescriptor, v protoreflect.Value) any {
	switch fd.Kind() {
	case protoreflect.BoolKind:
		return v.Bool()
	case protoreflect.EnumKind:
		if ed := fd.Enum().Values().ByNumber(v.Enum()); ed != nil {
			return string(ed.Name())
		}
		return int32(v.Enum())
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return int32(v.Int())
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return v.Int()
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return uint32(v.Uint())
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return v.Uint()
	case protoreflect.FloatKind:
		return float32(v.Float())
	case protoreflect.DoubleKind:
		return v.Float()
	case protoreflect.StringKind:
		return v.String()
	case protoreflect.BytesKind:
		return v.Bytes()
	case protoreflect.MessageKind, protoreflect.GroupKind:
		return messageValue(fd, v.Message())
	default:
		return nil
	}
}

// messageValue maps a message-kind value, special-casing the well-known types
// and otherwise recursing into the message's own fields.
func messageValue(fd protoreflect.FieldDescriptor, msg protoreflect.Message) any {
	switch string(fd.Message().FullName()) {
	case "google.protobuf.Timestamp":
		return timestampValue(msg)
	case "google.protobuf.Duration":
		return durationValue(msg)
	case "google.protobuf.Struct":
		return structValue(msg)
	case "google.protobuf.Value":
		return valueValue(msg)
	case "google.protobuf.ListValue":
		return listValue(msg)
	case "google.protobuf.Any":
		return anyValue(msg)
	case "google.protobuf.DoubleValue", "google.protobuf.FloatValue",
		"google.protobuf.Int64Value", "google.protobuf.UInt64Value",
		"google.protobuf.Int32Value", "google.protobuf.UInt32Value",
		"google.protobuf.BoolValue", "google.protobuf.StringValue",
		"google.protobuf.BytesValue":
		return wrapperValue(msg)
	default:
		return messageToStructuredData(msg)
	}
}

// timestampValue converts a google.protobuf.Timestamp to time.Time.
func timestampValue(msg protoreflect.Message) any {
	fields := msg.Descriptor().Fields()
	secs := msg.Get(fields.ByName("seconds")).Int()
	nanos := msg.Get(fields.ByName("nanos")).Int()
	return time.Unix(secs, nanos)
}

// durationValue converts a google.protobuf.Duration to time.Duration.
func durationValue(msg protoreflect.Message) any {
	fields := msg.Descriptor().Fields()
	secs := msg.Get(fields.ByName("seconds")).Int()
	nanos := msg.Get(fields.ByName("nanos")).Int()
	return time.Duration(secs)*time.Second + time.Duration(nanos)*time.Nanosecond
}

// structValue converts a google.protobuf.Struct to map[string]any.
func structValue(msg protoreflect.Message) any {
	mp := msg.Get(msg.Descriptor().Fields().ByName("fields")).Map()
	out := make(map[string]any, mp.Len())
	mp.Range(func(k protoreflect.MapKey, v protoreflect.Value) bool {
		out[k.String()] = valueValue(v.Message())
		return true
	})
	return out
}

// valueValue converts a google.protobuf.Value to its native Go value by
// resolving its oneof.
func valueValue(msg protoreflect.Message) any {
	od := msg.Descriptor().Oneofs().ByName("kind")
	fd := msg.WhichOneof(od)
	if fd == nil {
		return nil
	}
	v := msg.Get(fd)
	switch fd.Name() {
	case "null_value":
		return nil
	case "number_value":
		return v.Float()
	case "string_value":
		return v.String()
	case "bool_value":
		return v.Bool()
	case "struct_value":
		return structValue(v.Message())
	case "list_value":
		return listValue(v.Message())
	default:
		return nil
	}
}

// listValue converts a google.protobuf.ListValue to []any.
func listValue(msg protoreflect.Message) any {
	lst := msg.Get(msg.Descriptor().Fields().ByName("values")).List()
	out := make([]any, 0, lst.Len())
	for i := 0; i < lst.Len(); i++ {
		out = append(out, valueValue(lst.Get(i).Message()))
	}
	return out
}

// anyValue converts a google.protobuf.Any to an explicit raw form: the type
// URL and the undecoded bytes, so a known type is not decoded while an unknown
// one silently fails (documented limitation, not a silent drop).
func anyValue(msg protoreflect.Message) any {
	fields := msg.Descriptor().Fields()
	return map[string]any{
		"type_url": msg.Get(fields.ByName("type_url")).String(),
		"value":    msg.Get(fields.ByName("value")).Bytes(),
	}
}

// wrapperValue unwraps a google.protobuf wrapper (Int32Value, StringValue, ...)
// to its scalar, or nil when unset.
func wrapperValue(msg protoreflect.Message) any {
	fd := msg.Descriptor().Fields().ByName("value")
	if !msg.Has(fd) {
		return nil
	}
	return scalarValue(fd, msg.Get(fd))
}
