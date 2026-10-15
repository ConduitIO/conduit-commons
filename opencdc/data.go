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
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"maps"
	"reflect"
	"slices"

	opencdcv1 "github.com/conduitio/conduit-commons/proto/opencdc/v1"
	"github.com/goccy/go-json"
)

// Data is a structure that contains some bytes. The only structs implementing
// Data are RawData and StructuredData.
type Data interface {
	isData() // Ensure structs outside of this package can't implement this interface.
	Bytes() []byte
	Clone() Data
	ToProto(*opencdcv1.Data) error
}

type Change struct {
	// Before contains the data before the operation occurred. This field is
	// optional and should only be populated for operations OperationUpdate
	// OperationDelete (if the system supports fetching the data before the
	// operation).
	Before Data `json:"before"`
	// After contains the data after the operation occurred. This field should
	// be populated for all operations except OperationDelete.
	After Data `json:"after"`
}

// StructuredData contains data in form of a map with string keys and arbitrary
// values.
type StructuredData map[string]interface{}

func (StructuredData) isData() {}

func (d StructuredData) Bytes() []byte {
	b, err := json.Marshal(d)
	if err != nil {
		// Unlikely to happen, records travel from/to plugins through GRPC.
		// If the content can be marshaled as protobuf it can be as JSON.
		panic(fmt.Errorf("error while marshaling StructuredData as JSON: %w", err))
	}
	return b
}

// Clone returns a deep copy of d. The copy shares no mutable memory with d:
// nested maps, slices and arrays are copied element by element, at any
// depth, so changing the copy never changes d (Invariant 6: fan-out, the
// clone and split processors, and the engine all rely on that).
//
// On this release line Clone keeps one type change from earlier releases on
// purpose: a map[string]any held directly as a value of d comes back as
// StructuredData, and so, recursively, do the map[string]any values held
// directly inside it. Every other value keeps its Go type, including a
// map[string]any inside a slice or inside a nested StructuredData. The
// result has exactly the types earlier releases produced.
// Changing that would change the Avro schemas inferred from cloned records
// within a patch release. From v0.9.0 on, Clone keeps every type.
//
// Pointers, channels and funcs are copied by reference, and structs by value
// (so a struct's own slice or map fields are shared). Record data does not
// normally hold them. The data must not contain cycles.
func (d StructuredData) Clone() Data {
	cloned := make(StructuredData, len(d))
	for k, v := range d {
		switch v := v.(type) {
		case map[string]any:
			// Kept from earlier releases: becomes StructuredData, and so do
			// the map[string]any values directly inside it.
			cloned[k] = StructuredData(v).Clone()
		default:
			// Everything else, a nested StructuredData included, keeps its
			// type at every depth, exactly as before (earlier releases
			// shared these values instead of copying them).
			cloned[k] = cloneValue(v)
		}
	}
	return cloned
}

// cloneValue deep-copies v, keeping its type. The common record shapes have
// fast paths; other maps, slices and arrays go through reflection.
func cloneValue(v any) any {
	switch v := v.(type) {
	case nil, bool, string,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return v
	case map[string]any:
		if v == nil {
			return v
		}
		m := make(map[string]any, len(v))
		for k, e := range v {
			m[k] = cloneValue(e)
		}
		return m
	case StructuredData:
		if v == nil {
			return v
		}
		m := make(StructuredData, len(v))
		for k, e := range v {
			m[k] = cloneValue(e)
		}
		return m
	case []any:
		if v == nil {
			return v
		}
		s := make([]any, len(v))
		for i, e := range v {
			s[i] = cloneValue(e)
		}
		return s
	case []byte:
		return bytes.Clone(v) // nil stays nil
	case RawData:
		if v == nil {
			return v
		}
		return RawData(bytes.Clone(v))
	case []string:
		return slices.Clone(v) // strings are immutable; nil stays nil
	case map[string]string:
		return maps.Clone(v) // nil stays nil
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() { //nolint:exhaustive // only containers need copying
	case reflect.Map, reflect.Slice, reflect.Array:
		return cloneReflect(rv).Interface()
	default:
		return v // scalars, time.Time and other structs are values; pointers are shared (see Clone)
	}
}

// cloneReflect deep-copies maps, slices and arrays of any type, and the
// values behind interfaces. Everything else is returned as is.
func cloneReflect(v reflect.Value) reflect.Value {
	switch v.Kind() { //nolint:exhaustive // every other kind is copied by value or shared on purpose
	case reflect.Interface:
		if v.IsNil() {
			return v
		}
		out := reflect.New(v.Type()).Elem()
		out.Set(cloneReflect(v.Elem()))
		return out
	case reflect.Map:
		if v.IsNil() {
			return v
		}
		out := reflect.MakeMapWithSize(v.Type(), v.Len())
		iter := v.MapRange()
		for iter.Next() {
			out.SetMapIndex(iter.Key(), cloneReflect(iter.Value()))
		}
		return out
	case reflect.Slice:
		if v.IsNil() {
			return v
		}
		out := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		for i := 0; i < v.Len(); i++ {
			out.Index(i).Set(cloneReflect(v.Index(i)))
		}
		return out
	case reflect.Array:
		out := reflect.New(v.Type()).Elem()
		for i := 0; i < v.Len(); i++ {
			out.Index(i).Set(cloneReflect(v.Index(i)))
		}
		return out
	default:
		return v
	}
}

// RawData contains unstructured data in form of a byte slice.
type RawData []byte

func (RawData) isData() {}

func (d RawData) Bytes() []byte {
	return d
}

func (d RawData) Clone() Data {
	return RawData(bytes.Clone(d))
}

func (d RawData) MarshalJSON(ctx context.Context) ([]byte, error) {
	if ctx != nil {
		s := ctx.Value(jsonMarshalOptionsCtxKey{})
		//nolint:forcetypeassert // We know the type of the value.
		if s != nil && s.(*JSONMarshalOptions).RawDataAsString {
			// We should serialize RawData as a string.
			//nolint:wrapcheck // If we didn't implement MarshalJSON this would be done by the json package.
			return json.Marshal(string(d))
		}
	}

	// We could use json.Marshal([]byte(d)) here, but it would be 3 times slower,
	// and since this is in the hot path, we need to optimize it.

	if d == nil {
		return []byte(`null`), nil
	}
	encodedLen := base64.StdEncoding.EncodedLen(len(d))
	out := make([]byte, encodedLen+2)
	out[0] = '"' // add leading quote
	base64.StdEncoding.Encode(out[1:], d)
	out[encodedLen+1] = '"' // add trailing quote
	return out, nil
}
