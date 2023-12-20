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
	"unicode/utf8"

	opencdcv1 "github.com/conduitio/conduit-commons/proto/opencdc/v1"
	"google.golang.org/protobuf/runtime/protoimpl"
	"google.golang.org/protobuf/types/known/structpb"
)

func _() {
	// An "invalid array index" compiler error signifies that the constant values have changed.
	var cTypes [1]struct{}
	_ = cTypes[int(OperationCreate)-int(opencdcv1.Operation_OPERATION_CREATE)]
	_ = cTypes[int(OperationUpdate)-int(opencdcv1.Operation_OPERATION_UPDATE)]
	_ = cTypes[int(OperationDelete)-int(opencdcv1.Operation_OPERATION_DELETE)]
	_ = cTypes[int(OperationSnapshot)-int(opencdcv1.Operation_OPERATION_SNAPSHOT)]
}

func (r Record) ToProto(proto *opencdcv1.Record) error {
	if r.Key != nil {
		if proto.Key == nil {
			proto.Key = &opencdcv1.Data{}
		}
		err := r.Key.ToProto(proto.Key)
		if err != nil {
			return fmt.Errorf("error converting key: %w", err)
		}
	} else {
		proto.Key = nil
	}

	if proto.Payload == nil {
		proto.Payload = &opencdcv1.Change{}
	}
	err := r.Payload.ToProto(proto.Payload)
	if err != nil {
		return fmt.Errorf("error converting payload: %w", err)
	}

	proto.Position = r.Position
	proto.Operation = opencdcv1.Operation(r.Operation)
	proto.Metadata = r.Metadata
	return nil
}

func (c Change) ToProto(proto *opencdcv1.Change) error {
	if c.Before != nil {
		if proto.Before == nil {
			proto.Before = &opencdcv1.Data{}
		}
		err := c.Before.ToProto(proto.Before)
		if err != nil {
			return fmt.Errorf("error converting before: %w", err)
		}
	} else {
		proto.Before = nil
	}

	if c.After != nil {
		if proto.After == nil {
			proto.After = &opencdcv1.Data{}
		}
		err := c.After.ToProto(proto.After)
		if err != nil {
			return fmt.Errorf("error converting after: %w", err)
		}
	} else {
		proto.After = nil
	}

	return nil
}

func (d RawData) ToProto(proto *opencdcv1.Data) error {
	protoRawData, ok := proto.Data.(*opencdcv1.Data_RawData)
	if !ok {
		protoRawData = &opencdcv1.Data_RawData{}
		proto.Data = protoRawData
	}
	protoRawData.RawData = d
	return nil
}

func (d StructuredData) ToProto(proto *opencdcv1.Data) error {
	protoStructuredData, ok := proto.Data.(*opencdcv1.Data_StructuredData)
	if !ok {
		protoStructuredData = &opencdcv1.Data_StructuredData{}
		proto.Data = protoStructuredData
	}

	data := protoStructuredData.StructuredData
	if data == nil {
		protoStructuredData.StructuredData = &structpb.Struct{
			Fields: make(map[string]*structpb.Value, len(d)),
		}
		data = protoStructuredData.StructuredData
	}

	for k, v := range d {
		if !utf8.ValidString(k) {
			return protoimpl.X.NewError("invalid UTF-8 in string: %q", k)
		}
		var err error
		data.Fields[k], err = structpb.NewValue(v)
		if err != nil {
			return err
		}
	}
	return nil
}
