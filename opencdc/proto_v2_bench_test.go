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

package opencdc

import (
	"encoding/json"
	"math"
	"testing"

	opencdcv2 "github.com/conduitio/conduit-commons/proto/opencdc/v2"
	"google.golang.org/protobuf/proto"
)

// benchRecord is a representative CDC record: a structured payload with the
// field mix a Postgres source produces (an int64 id, a string, a bool, a
// timestamp, a decimal amount, a uint64 and raw bytes, plus one nested map and
// one array), a one-field structured key, and a couple of metadata entries.
// This is the shape that dominates the wire serialization hot path.
func benchRecord() Record {
	return Record{
		Position:  Position("0/1F2E3D4"),
		Operation: OperationCreate,
		Metadata:  Metadata{"opencdc.createdAt": "1710000000000000000", "table": "users"},
		Key:       StructuredData{"id": int64(42)},
		Payload: Change{
			After: StructuredData{
				"id":         int64(42),
				"name":       "alice",
				"active":     true,
				"created_at": int64(1710000000000000000),
				"amount":     json.Number("123.45"),
				"rating":     4.75,
				"seen":       uint64(math.MaxUint64),
				"blob":       []byte("raw bytes"),
				"tags":       []any{"a", "b", int64(3)},
				"nested":     map[string]any{"ok": true, "count": int64(7)},
			},
		},
	}
}

// BenchmarkRecord_ToProtoV2 measures the opencdc.Record → opencdc.v2.Record
// conversion alone, allocating a fresh proto message each iteration exactly
// like the connector protocol does (pconnector/v3/toproto).
func BenchmarkRecord_ToProtoV2(b *testing.B) {
	rec := benchRecord()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out := &opencdcv2.Record{}
		if err := rec.ToProtoV2(out); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRecord_ToProtoV2_Marshal measures the full wire boundary: convert
// to opencdc.v2 and marshal to bytes.
func BenchmarkRecord_ToProtoV2_Marshal(b *testing.B) {
	rec := benchRecord()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out := &opencdcv2.Record{}
		if err := rec.ToProtoV2(out); err != nil {
			b.Fatal(err)
		}
		if _, err := proto.Marshal(out); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRecord_ToProtoV2_Marshal_ReusedProto measures the marshal path with
// a reused proto message (ToProtoV2 re-populates it in place), isolating the
// cost of the message construction from the fresh-allocation cost.
func BenchmarkRecord_ToProtoV2_Marshal_ReusedProto(b *testing.B) {
	rec := benchRecord()
	out := &opencdcv2.Record{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := rec.ToProtoV2(out); err != nil {
			b.Fatal(err)
		}
		if _, err := proto.Marshal(out); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRecord_MarshalV2 measures the new direct wire encoder with a fresh
// output buffer per iteration.
func BenchmarkRecord_MarshalV2(b *testing.B) {
	rec := benchRecord()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := rec.MarshalV2(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRecord_AppendV2 measures the new direct wire encoder with a reused
// output buffer, which is how a caller that marshals a stream of records would
// use it. This isolates the per-record cost from the output buffer allocation.
func BenchmarkRecord_AppendV2(b *testing.B) {
	rec := benchRecord()
	buf := make([]byte, 0, 512)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var err error
		buf, err = rec.AppendV2(buf[:0])
		if err != nil {
			b.Fatal(err)
		}
	}
}
