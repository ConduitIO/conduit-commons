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
	"fmt"

	opencdcv2 "github.com/conduitio/conduit-commons/proto/opencdc/v2"
)

func _() {
	// An "invalid array index" compiler error signifies that the constant values have changed.
	var cTypes [1]struct{}
	_ = cTypes[int(OperationCreate)-int(opencdcv2.Operation_OPERATION_CREATE)]
	_ = cTypes[int(OperationUpdate)-int(opencdcv2.Operation_OPERATION_UPDATE)]
	_ = cTypes[int(OperationDelete)-int(opencdcv2.Operation_OPERATION_DELETE)]
	_ = cTypes[int(OperationSnapshot)-int(opencdcv2.Operation_OPERATION_SNAPSHOT)]
}

// -- From Proto To OpenCDC ----------------------------------------------------

// FromProtoV2 takes data from the supplied opencdc.v2 proto object and
// populates the receiver. If the proto object is nil, the receiver is set to
// its zero value. If the function returns an error, the receiver could be
// partially populated.
func (r *Record) FromProtoV2(proto *opencdcv2.Record) error {
	if proto == nil {
		*r = Record{}
		return nil
	}

	var err error
	r.Key, err = dataFromProtoV2(proto.Key)
	if err != nil {
		return fmt.Errorf("error converting key: %w", err)
	}

	if proto.Payload != nil {
		err := r.Payload.FromProtoV2(proto.Payload)
		if err != nil {
			return fmt.Errorf("error converting payload: %w", err)
		}
	} else {
		r.Payload = Change{}
	}

	r.Position = proto.Position
	r.Metadata = proto.Metadata
	r.Operation = Operation(proto.Operation)
	return nil
}

// FromProtoV2 takes data from the supplied opencdc.v2 proto object and
// populates the receiver. If the proto object is nil, the receiver is set to
// its zero value. If the function returns an error, the receiver could be
// partially populated.
func (c *Change) FromProtoV2(proto *opencdcv2.Change) error {
	if proto == nil {
		*c = Change{}
		return nil
	}

	var err error
	c.Before, err = dataFromProtoV2(proto.Before)
	if err != nil {
		return fmt.Errorf("error converting before: %w", err)
	}

	c.After, err = dataFromProtoV2(proto.After)
	if err != nil {
		return fmt.Errorf("error converting after: %w", err)
	}

	return nil
}

func dataFromProtoV2(proto *opencdcv2.Data) (Data, error) {
	if proto == nil {
		return nil, nil //nolint:nilnil // This is the expected behavior.
	}

	switch v := proto.Data.(type) {
	case *opencdcv2.Data_RawData:
		return RawData(v.RawData), nil
	case *opencdcv2.Data_StructuredData:
		m, err := structFromProtoV2(v.StructuredData)
		if err != nil {
			return nil, err
		}
		return StructuredData(m), nil
	case nil:
		return nil, nil //nolint:nilnil // This is the expected behavior.
	default:
		return nil, ErrInvalidProtoDataType
	}
}

// -- From OpenCDC To Proto ----------------------------------------------------

// ToProtoV2 takes data from the receiver and populates the supplied opencdc.v2
// proto object. If the function returns an error, the proto object could be
// partially populated.
func (r Record) ToProtoV2(proto *opencdcv2.Record) error {
	if r.Key != nil {
		if proto.Key == nil {
			proto.Key = &opencdcv2.Data{}
		}
		err := r.Key.ToProtoV2(proto.Key)
		if err != nil {
			return wrapProtoError("error converting key", prefixNumberErrorPath(err, fieldKey))
		}
	} else {
		proto.Key = nil
	}

	if proto.Payload == nil {
		proto.Payload = &opencdcv2.Change{}
	}
	err := r.Payload.ToProtoV2(proto.Payload)
	if err != nil {
		return wrapProtoError("error converting payload", prefixNumberErrorPath(err, fieldPayload))
	}

	proto.Position = r.Position
	proto.Metadata = r.Metadata
	proto.Operation = opencdcv2.Operation(r.Operation) //nolint:gosec // no risk of overflow
	return nil
}

// ToProtoV2 takes data from the receiver and populates the supplied opencdc.v2
// proto object. If the function returns an error, the proto object could be
// partially populated.
func (c Change) ToProtoV2(proto *opencdcv2.Change) error {
	if c.Before != nil {
		if proto.Before == nil {
			proto.Before = &opencdcv2.Data{}
		}
		err := c.Before.ToProtoV2(proto.Before)
		if err != nil {
			return wrapProtoError("error converting before", prefixNumberErrorPath(err, fieldBefore))
		}
	} else {
		proto.Before = nil
	}

	if c.After != nil {
		if proto.After == nil {
			proto.After = &opencdcv2.Data{}
		}
		err := c.After.ToProtoV2(proto.After)
		if err != nil {
			return wrapProtoError("error converting after", prefixNumberErrorPath(err, fieldAfter))
		}
	} else {
		proto.After = nil
	}

	return nil
}

// ToProtoV2 takes data from the receiver and populates the supplied opencdc.v2
// proto object.
func (d RawData) ToProtoV2(proto *opencdcv2.Data) error {
	protoRawData, ok := proto.Data.(*opencdcv2.Data_RawData)
	if !ok {
		protoRawData = &opencdcv2.Data_RawData{}
		proto.Data = protoRawData
	}
	protoRawData.RawData = d
	return nil
}

// ToProtoV2 takes data from the receiver and populates the supplied opencdc.v2
// proto object.
//
// Unlike ToProto, the v2 wire format is lossless: the encoder keeps the Go
// type of every number and byte slice, so no integer is refused and nothing is
// rounded. int* becomes the int64 kind, uint* becomes the uint64 kind, and
// []byte becomes the bytes kind. json.Number integers that fit in an int64
// become the int64 kind; every other json.Number becomes the decimal kind (its
// exact string form, never parsed through a float64). NaN and ±Inf become the
// double kind. See the ADR
// 20261015-lossless-structured-values-protocol-v3.md and conduit#3100.
func (d StructuredData) ToProtoV2(proto *opencdcv2.Data) error {
	w := &v2ValueEncoder{path: make(pathStack, 0, 8)}
	s, err := w.structValue(map[string]any(d))
	if err != nil {
		return wrapStructuredDataError(err)
	}
	protoStructuredData, ok := proto.Data.(*opencdcv2.Data_StructuredData)
	if !ok {
		protoStructuredData = &opencdcv2.Data_StructuredData{}
		proto.Data = protoStructuredData
	}
	protoStructuredData.StructuredData = s
	return nil
}
