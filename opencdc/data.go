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

func (d StructuredData) Clone() Data {
	cloned := make(map[string]any, len(d))
	for k, v := range d {
		if vmap, ok := v.(map[string]any); ok {
			cloned[k] = StructuredData(vmap).Clone()
		} else {
			cloned[k] = v
		}
	}
	return StructuredData(cloned)
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
