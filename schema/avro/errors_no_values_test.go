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
	"errors"
	"strings"
	"testing"

	"github.com/conduitio/conduit-commons/logsafe"
	"github.com/conduitio/conduit-commons/opencdc"
	"github.com/iskorotkov/avro/v2"
	"github.com/matryer/is"
)

// TestSerde_Marshal_ErrorsDoNotContainValues checks that union resolution
// errors name the Go type and the field path but never echo the record value,
// since errors end up in logs and API responses.
func TestSerde_Marshal_ErrorsDoNotContainValues(t *testing.T) {
	const (
		sampleInt    = "90817263"
		sampleString = "distinctive-sample-value"
	)
	testCases := []struct {
		name     string
		field    string
		value    any
		wantPath string // field path expected in the message
		wantType string
	}{{
		name:     "ambiguous reading (map or record)",
		field:    `{"type":"array","items":["null",{"type":"map","values":"int"},{"type":"record","name":"R","fields":[{"name":"a","type":"int"}]}]}`,
		value:    []any{map[string]any{"a": 90817263}},
		wantPath: "f: [0]:",
		wantType: "map[string]interface {}",
	}, {
		name:     "no branch fits a map",
		field:    `{"type":"array","items":["null",{"type":"map","values":"int"},{"type":"record","name":"R","fields":[{"name":"a","type":"int"}]}]}`,
		value:    []any{map[string]any{"a": sampleString}},
		wantPath: "f: [0]:",
		wantType: "map[string]interface {}",
	}}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			is := is.New(t)
			s, err := Parse([]byte(`{"type":"record","name":"Root","fields":[{"name":"f","type":` + tc.field + `}]}`))
			is.NoErr(err)
			_, err = s.Marshal(opencdc.StructuredData{"f": tc.value})
			is.True(errors.Is(err, ErrSchemaValueMismatch))
			msg := err.Error()
			is.True(!strings.Contains(msg, sampleInt))
			is.True(!strings.Contains(msg, sampleString))
			is.True(strings.Contains(msg, tc.wantPath)) // field path
			is.True(strings.Contains(msg, tc.wantType)) // type information
		})
	}
}

// TestUnwrapUnions_UnknownBranchDoesNotEchoKey checks that the wrapper key of
// a decoded union, which comes from the record data, is not echoed.
func TestUnwrapUnions_UnknownBranchDoesNotEchoKey(t *testing.T) {
	is := is.New(t)
	rs, err := Parse([]byte(`{"type":"record","name":"Root","fields":[{"name":"f","type":["null","int","string"]}]}`))
	is.NoErr(err)
	us := rs.schema.(*avro.RecordSchema).Fields()[0].Type()
	_, err = unwrapUnions(us, map[string]any{"key-sample-77": 1}, posTop)
	is.True(errors.Is(err, ErrSchemaValueMismatch))
	is.True(strings.Contains(err.Error(), "no branch"))
	is.True(!strings.Contains(err.Error(), "key-sample-77"))
}

// TestUnionErrors_MapKeyNotEchoed checks that a map key, which is record data,
// is named in a union error only by its salted digest, both when encoding and
// when unwrapping a decoded value.
func TestUnionErrors_MapKeyNotEchoed(t *testing.T) {
	const secretKey = "secret-key-xyz"
	field := `{"type":"map","values":["null",{"type":"map","values":"int"},{"type":"record","name":"R","fields":[{"name":"a","type":"int"}]}]}`

	t.Run("marshal", func(t *testing.T) {
		is := is.New(t)
		s, err := Parse([]byte(`{"type":"record","name":"Root","fields":[{"name":"f","type":` + field + `}]}`))
		is.NoErr(err)
		_, err = s.Marshal(opencdc.StructuredData{"f": map[string]any{secretKey: map[string]any{"a": "x"}}})
		is.True(err != nil)
		is.True(!strings.Contains(err.Error(), secretKey))
		is.True(strings.Contains(err.Error(), "[key "+logsafe.HashString(secretKey)+"]"))
	})

	t.Run("unwrap", func(t *testing.T) {
		is := is.New(t)
		rs, err := Parse([]byte(`{"type":"record","name":"Root","fields":[{"name":"f","type":` + field + `}]}`))
		is.NoErr(err)
		ms := rs.schema.(*avro.RecordSchema).Fields()[0].Type()
		_, err = unwrapUnions(ms, map[string]any{secretKey: map[string]any{"unknown-branch": 1}}, posTop)
		is.True(errors.Is(err, ErrSchemaValueMismatch))
		is.True(!strings.Contains(err.Error(), secretKey))
		is.True(strings.Contains(err.Error(), "[key "+logsafe.HashString(secretKey)+"]"))
	})
}
