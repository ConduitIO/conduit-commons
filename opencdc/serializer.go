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
	"context"
	"fmt"

	"github.com/goccy/go-json"
)

// RecordSerializer is a type that can serialize a record to bytes. It's used in
// destination connectors to change the output structure and format.
type RecordSerializer interface {
	Serialize(Record) ([]byte, error)
}

// JSONSerializer is a RecordSerializer that serializes records to JSON using
// the configured options.
type JSONSerializer JSONMarshalOptions

func (s JSONSerializer) Serialize(r Record) ([]byte, error) {
	ctx := WithJSONMarshalOptions(context.Background(), (*JSONMarshalOptions)(&s))
	defer func() {
		// Workaround because of https://github.com/goccy/go-json/issues/499.
		// TODO: Remove this when the issue is fixed and store value in context
		//  instead of pointer.
		s = JSONSerializer{}
	}()
	bytes, err := json.MarshalContext(ctx, r)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize record to JSON: %w", err)
	}
	return bytes, nil
}
