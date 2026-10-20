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

package avro

import (
	"strconv"
	"strings"
	"testing"

	"github.com/conduitio/conduit-commons/opencdc"
)

// The benchmarks below measure what BeforeMarshal and AfterUnmarshal add to
// Marshal and Unmarshal for two common schema shapes: a wide CDC-style
// record of nullable primitive columns, and nested nullable records. Run
// with -benchmem -cpu 1. They are not a regression gate (no benchi job runs
// them); they exist so allocation changes in the union walk are visible.

// benchWideNullable is a record of n fields typed ["null","string"].
func benchWideNullable(n int) (string, func() opencdc.StructuredData) {
	fs := make([]string, n)
	for i := range n {
		fs[i] = `{"name":"c` + strconv.Itoa(i) + `","type":["null","string"]}`
	}
	return `{"type":"record","name":"r","fields":[` + strings.Join(fs, ",") + `]}`, func() opencdc.StructuredData {
		v := opencdc.StructuredData{}
		for i := range n {
			v["c"+strconv.Itoa(i)] = "value"
		}
		return v
	}
}

// benchDeepNullable is d levels of ["null", {v: int, n: ...}].
func benchDeepNullable(d int) (string, func() opencdc.StructuredData) {
	var b strings.Builder
	for k := range d {
		b.WriteString(`{"name":"n","type":["null",{"type":"record","name":"R` + strconv.Itoa(k) + `","fields":[{"name":"v","type":"int"},`)
	}
	b.WriteString(`{"name":"n","type":"null"}`)
	for range d {
		b.WriteString(`]}]}`)
	}
	return `{"type":"record","name":"top","fields":[` + b.String() + `]}`, func() opencdc.StructuredData {
		var mk func(k int) any
		mk = func(k int) any {
			if k == d {
				return nil
			}
			return map[string]any{"v": int32(k), "n": mk(k + 1)}
		}
		return opencdc.StructuredData{"n": mk(0)}
	}
}

func benchmarkMarshal(b *testing.B, schema string, value func() opencdc.StructuredData) {
	b.Helper()
	s, err := Parse([]byte(schema))
	if err != nil {
		b.Fatal(err)
	}
	vals := make([]opencdc.StructuredData, b.N)
	for i := range vals {
		vals[i] = value()
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		if _, err := s.Marshal(vals[i]); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkUnmarshal(b *testing.B, schema string, value func() opencdc.StructuredData) {
	b.Helper()
	s, err := Parse([]byte(schema))
	if err != nil {
		b.Fatal(err)
	}
	bs, err := s.Marshal(value())
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		var v opencdc.StructuredData
		if err := s.Unmarshal(bs, &v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSerde_Marshal_WideNullablePrimitives(b *testing.B) {
	schema, value := benchWideNullable(200)
	benchmarkMarshal(b, schema, value)
}

func BenchmarkSerde_Unmarshal_WideNullablePrimitives(b *testing.B) {
	schema, value := benchWideNullable(200)
	benchmarkUnmarshal(b, schema, value)
}

func BenchmarkSerde_Marshal_DeepNullableRecords(b *testing.B) {
	for _, d := range []int{4, 16, 64} {
		b.Run(strconv.Itoa(d), func(b *testing.B) {
			schema, value := benchDeepNullable(d)
			benchmarkMarshal(b, schema, value)
		})
	}
}
