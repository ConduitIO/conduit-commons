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
	"errors"
	"fmt"

	opencdcv1 "github.com/conduitio/conduit-commons/proto/opencdc/v1"
)

func _() {
	// An "invalid array index" compiler error signifies that the constant values have changed.
	var cTypes [1]struct{}
	_ = cTypes[int(OperationCreate)-int(opencdcv1.Operation_OPERATION_CREATE)]
	_ = cTypes[int(OperationUpdate)-int(opencdcv1.Operation_OPERATION_UPDATE)]
	_ = cTypes[int(OperationDelete)-int(opencdcv1.Operation_OPERATION_DELETE)]
	_ = cTypes[int(OperationSnapshot)-int(opencdcv1.Operation_OPERATION_SNAPSHOT)]
}

func (r *Record) FromProto(proto *opencdcv1.Record) error {
	if proto == nil {
		*r = Record{}
		return nil
	}

	var err error
	r.Key, err = dataFromProto(proto.Key)
	if err != nil {
		return fmt.Errorf("error converting key: %w", err)
	}

	if proto.Payload != nil {
		err := r.Payload.FromProto(proto.Payload)
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

func (c *Change) FromProto(proto *opencdcv1.Change) error {
	if proto == nil {
		*c = Change{}
		return nil
	}

	var err error
	c.Before, err = dataFromProto(proto.Before)
	if err != nil {
		return fmt.Errorf("error converting before: %w", err)
	}

	c.After, err = dataFromProto(proto.After)
	if err != nil {
		return fmt.Errorf("error converting after: %w", err)
	}

	return nil
}

func dataFromProto(proto *opencdcv1.Data) (Data, error) {
	if proto == nil {
		return nil, nil
	}

	switch v := proto.Data.(type) {
	case *opencdcv1.Data_RawData:
		return RawData(v.RawData), nil
	case *opencdcv1.Data_StructuredData:
		return StructuredData(v.StructuredData.AsMap()), nil
	case nil:
		return nil, nil
	default:
		return nil, errors.New("invalid Data type")
	}
}
