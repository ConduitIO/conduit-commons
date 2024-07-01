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

	s1 := Schema{
		Subject: "subject",
		Version: 1,
		Type:    TypeAvro,
		Bytes:   []byte("bytes"),
	}

	want := &schemav1.Schema{
		Subject: s1.Subject,
		Version: 1,
		Type:    schemav1.Schema_TYPE_AVRO,
		Bytes:   s1.Bytes,
	}

	var got schemav1.Schema
	err := s1.ToProto(&got)
	is.NoErr(err)
	is.Equal(&got, want)
}
