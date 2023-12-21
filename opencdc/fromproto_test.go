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

package opencdc

import (
	"testing"

	opencdcv1 "github.com/conduitio/conduit-commons/proto/opencdc/v1"
	"github.com/matryer/is"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestRecord_FromProto(t *testing.T) {
	is := is.New(t)

	r1 := &opencdcv1.Record{
		Position:  []byte("standing"),
		Operation: opencdcv1.Operation_OPERATION_UPDATE,
		Metadata:  map[string]string{"foo": "bar"},
		Key:       &opencdcv1.Data{Data: &opencdcv1.Data_RawData{RawData: []byte("padlock-key")}},
		Payload: &opencdcv1.Change{
			Before: &opencdcv1.Data{Data: &opencdcv1.Data_RawData{RawData: []byte("yellow")}},
			After: &opencdcv1.Data{Data: &opencdcv1.Data_StructuredData{StructuredData: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"bool":    {Kind: &structpb.Value_BoolValue{BoolValue: true}},
					"int":     {Kind: &structpb.Value_NumberValue{NumberValue: 1}},
					"int32":   {Kind: &structpb.Value_NumberValue{NumberValue: 1}},
					"int64":   {Kind: &structpb.Value_NumberValue{NumberValue: 1}},
					"float32": {Kind: &structpb.Value_NumberValue{NumberValue: 1.2}},
					"float64": {Kind: &structpb.Value_NumberValue{NumberValue: 1.2}},
					"string":  {Kind: &structpb.Value_StringValue{StringValue: "orange"}},
				},
			}},
			}},
	}

	want := Record{
		Position:  r1.Position,
		Operation: Operation(r1.Operation),
		Metadata:  r1.Metadata,
		Key:       RawData(r1.Key.GetRawData()),
		Payload: Change{
			Before: RawData(r1.Payload.Before.GetRawData()),
			After:  StructuredData(r1.Payload.After.GetStructuredData().AsMap()),
		},
	}

	var got Record
	err := got.FromProto(r1)
	is.NoErr(err)
	is.Equal(got, want)

	// writing another record to the same target should overwrite the previous

	want2 := Record{}
	err = got.FromProto(&opencdcv1.Record{})
	is.NoErr(err)
	is.Equal(got, want2)
}

func BenchmarkRecord_FromProto_Structured(b *testing.B) {
	r1 := &opencdcv1.Record{
		Position:  []byte("standing"),
		Operation: opencdcv1.Operation_OPERATION_UPDATE,
		Metadata:  map[string]string{"foo": "bar"},
		Key:       &opencdcv1.Data{Data: &opencdcv1.Data_RawData{RawData: []byte("padlock-key")}},
		Payload: &opencdcv1.Change{
			Before: &opencdcv1.Data{Data: &opencdcv1.Data_RawData{RawData: []byte("yellow")}},
			After: &opencdcv1.Data{Data: &opencdcv1.Data_StructuredData{StructuredData: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"bool":    {Kind: &structpb.Value_BoolValue{BoolValue: true}},
					"int":     {Kind: &structpb.Value_NumberValue{NumberValue: 1}},
					"int32":   {Kind: &structpb.Value_NumberValue{NumberValue: 1}},
					"int64":   {Kind: &structpb.Value_NumberValue{NumberValue: 1}},
					"float32": {Kind: &structpb.Value_NumberValue{NumberValue: 1.2}},
					"float64": {Kind: &structpb.Value_NumberValue{NumberValue: 1.2}},
					"string":  {Kind: &structpb.Value_StringValue{StringValue: "orange"}},
				},
			}},
			}},
	}

	// reuse the same target record
	var r2 Record

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r2.FromProto(r1)
	}
	_ = r2
}
