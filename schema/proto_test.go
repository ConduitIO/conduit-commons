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

package schema

import (
	"testing"

	schemav1 "github.com/conduitio/conduit-commons/proto/schema/v1"
	"github.com/matryer/is"
)

func TestSchema_FromProto(t *testing.T) {
	is := is.New(t)

	s1 := &schemav1.Schema{
		Subject: "subject",
		Version: 1,
		Type:    schemav1.Schema_TYPE_AVRO,
		Bytes:   []byte("bytes"),
	}

	want := Schema{
		Subject: "subject",
		Version: 1,
		Type:    TypeAvro,
		Bytes:   []byte("bytes"),
	}

	var got Schema
	err := got.FromProto(s1)
	is.NoErr(err)
	is.Equal(got, want)
}

func TestSchema_ToProto(t *testing.T) {
	is := is.New(t)

	testCases := []struct {
		name    string
		in      *schemav1.Schema
		want    *schemav1.Schema
		wantErr error
	}{
		{
			name:    "when proto object is nil",
			in:      nil,
			want:    nil,
			wantErr: errInvalidProtoIsNil,
		},
		{
			name: "when proto object is not nil",
			in:   &schemav1.Schema{},
			want: &schemav1.Schema{
				Subject: "subject",
				Version: 1,
				Type:    schemav1.Schema_TYPE_AVRO,
				Bytes:   []byte("bytes"),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			is := is.New(t)

			s1 := Schema{
				Subject: "subject",
				Version: 1,
				Type:    TypeAvro,
				Bytes:   []byte("bytes"),
			}

			err := s1.ToProto(tc.in)

			if tc.wantErr == nil {
				is.NoErr(err)
				is.Equal(tc.in, tc.want)
			} else {
				is.Equal(err.Error(), tc.wantErr.Error())
			}
		})
	}
}
