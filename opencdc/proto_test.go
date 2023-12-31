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
	"fmt"
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
			After: &opencdcv1.Data{
				Data: &opencdcv1.Data_StructuredData{StructuredData: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"bool":    {Kind: &structpb.Value_BoolValue{BoolValue: true}},
						"int":     {Kind: &structpb.Value_NumberValue{NumberValue: 1}},
						"int32":   {Kind: &structpb.Value_NumberValue{NumberValue: 1}},
						"int64":   {Kind: &structpb.Value_NumberValue{NumberValue: 1}},
						"float32": {Kind: &structpb.Value_NumberValue{NumberValue: 1.2}},
						"float64": {Kind: &structpb.Value_NumberValue{NumberValue: 1.2}},
						"string":  {Kind: &structpb.Value_StringValue{StringValue: "orange"}},
						"nested": {Kind: &structpb.Value_StructValue{StructValue: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"bool":    {Kind: &structpb.Value_BoolValue{BoolValue: true}},
								"int":     {Kind: &structpb.Value_NumberValue{NumberValue: 2}},
								"float32": {Kind: &structpb.Value_NumberValue{NumberValue: 2.3}},
								"string":  {Kind: &structpb.Value_StringValue{StringValue: "blue"}},
							},
						}}},
					},
				}},
			},
		},
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
			After: &opencdcv1.Data{
				Data: &opencdcv1.Data_StructuredData{StructuredData: &structpb.Struct{
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
			},
		},
	}

	// reuse the same target record
	var r2 Record

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r2.FromProto(r1)
	}
	_ = r2
}

func TestRecord_ToProto(t *testing.T) {
	is := is.New(t)

	r1 := Record{
		Position:  Position("standing"),
		Operation: OperationUpdate,
		Metadata:  Metadata{"foo": "bar"},
		Key:       RawData("padlock-key"),
		Payload: Change{
			Before: RawData("yellow"),
			After: StructuredData{
				"bool": true,

				"int":   1,
				"int32": int32(1),
				"int64": int64(1),

				"float32": float32(1.2),
				"float64": 1.2,

				"string": "orange",

				"nested": map[string]any{
					"bool":   true,
					"float":  2.3,
					"string": "blue",
				},
			},
		},
	}

	after, err := structpb.NewStruct(r1.Payload.After.(StructuredData))
	is.NoErr(err)
	want := &opencdcv1.Record{
		Position:  r1.Position,
		Operation: opencdcv1.Operation(r1.Operation),
		Metadata:  r1.Metadata,
		Key:       &opencdcv1.Data{Data: &opencdcv1.Data_RawData{RawData: r1.Key.(RawData)}},
		Payload: &opencdcv1.Change{
			Before: &opencdcv1.Data{Data: &opencdcv1.Data_RawData{RawData: r1.Payload.Before.(RawData)}},
			After:  &opencdcv1.Data{Data: &opencdcv1.Data_StructuredData{StructuredData: after}},
		},
	}

	var got opencdcv1.Record
	err = r1.ToProto(&got)
	is.NoErr(err)
	is.Equal(&got, want)

	// writing another record to the same target should overwrite the previous

	want2 := &opencdcv1.Record{
		Payload: &opencdcv1.Change{}, // there's always a change
	}
	err = Record{}.ToProto(&got)
	is.NoErr(err)
	is.Equal(&got, want2)
}

func BenchmarkRecord_ToProto_Structured(b *testing.B) {
	r1 := Record{
		Position:  Position("standing"),
		Operation: OperationUpdate,
		Metadata:  Metadata{"foo": "bar"},
		Key:       RawData("padlock-key"),
		Payload: Change{
			Before: RawData("yellow"),
			After: StructuredData{
				"bool": true,

				"int":   1,
				"int32": int32(1),
				"int64": int64(1),

				"float32": float32(1.2),
				"float64": 1.2,

				"string": "orange",
			},
		},
	}

	// reuse the same target record
	var r2 opencdcv1.Record

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r1.ToProto(&r2)
	}
}

func BenchmarkRecord_ToProto_Raw(b *testing.B) {
	for _, size := range []int{1, 100, 10000, 1000000} {
		payload := make([]byte, size)
		r1 := Record{
			Position:  Position("standing"),
			Operation: OperationUpdate,
			Metadata:  Metadata{"foo": "bar"},
			Key:       RawData("padlock-key"),
			Payload: Change{
				Before: RawData("yellow"),
				After:  RawData(payload),
			},
		}

		b.Run(fmt.Sprintf("%d", size), func(b *testing.B) {
			// reuse the same target record
			var r2 opencdcv1.Record
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = r1.ToProto(&r2)
			}
		})
	}
}
