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

	"github.com/goccy/go-json"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/matryer/is"
)

func TestRecord_UnmarshalJSON(t *testing.T) {
	is := is.New(t)
	want := Record{
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
	b, err := json.Marshal(want)
	is.NoErr(err)

	var got Record
	err = json.Unmarshal(b, &got)
	is.NoErr(err)

	diff := cmp.Diff(want, got, cmpopts.IgnoreUnexported(Record{}))
	is.Equal(diff, "")
}

func BenchmarkRecord_MarshalJSON(b *testing.B) {
	r := Record{
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

	b.ResetTimer()
	var bytes []byte
	for i := 0; i < b.N; i++ {
		bytes, _ = json.Marshal(r)
	}
	_ = bytes
}

func BenchmarkRecord_UnmarshalJSON(b *testing.B) {
	r := Record{
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
	bytes, _ := json.Marshal(r)

	b.ResetTimer()
	var got Record
	for i := 0; i < b.N; i++ {
		_ = json.Unmarshal(bytes, &got)
	}
	_ = got
}
