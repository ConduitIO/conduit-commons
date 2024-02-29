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

package opencdc

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/matryer/is"
)

func TestJSONSerializer(t *testing.T) {
	rec := Record{
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

	testCases := []struct {
		name       string
		serializer JSONSerializer
		want       string
	}{{
		name:       "default",
		serializer: JSONSerializer{},
		want:       `{"position":"c3RhbmRpbmc=","operation":"update","metadata":{"foo":"bar"},"key":"cGFkbG9jay1rZXk=","payload":{"before":"eWVsbG93","after":{"bool":true,"float32":1.2,"float64":1.2,"int":1,"int32":1,"int64":1,"string":"orange"}}}`,
	}, {
		name:       "raw data as string",
		serializer: JSONSerializer{RawDataAsString: true},
		want:       `{"position":"c3RhbmRpbmc=","operation":"update","metadata":{"foo":"bar"},"key":"padlock-key","payload":{"before":"yellow","after":{"bool":true,"float32":1.2,"float64":1.2,"int":1,"int32":1,"int64":1,"string":"orange"}}}`,
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			is := is.New(t)
			rec.SetSerializer(tc.serializer)
			b := rec.Bytes()
			is.Equal(cmp.Diff(string(b), tc.want), "")
		})
	}
}
