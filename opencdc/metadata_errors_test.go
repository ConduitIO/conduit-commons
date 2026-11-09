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
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/conduitio/conduit-commons/logsafe"
	"github.com/matryer/is"
)

// TestMetadata_ParseErrorsDoNotContainValues checks that errors from parsing
// metadata values never contain the value, which comes from the
// record; constant keys may be printed.
func TestMetadata_ParseErrorsDoNotContainValues(t *testing.T) {
	const secret = "s3cr3t-value-9f2"

	testCases := []struct {
		name string
		key  string
		get  func(Metadata) error
		is   error
	}{
		{"created at", MetadataCreatedAt, func(m Metadata) error { _, err := m.GetCreatedAt(); return err }, strconv.ErrSyntax},
		{"read at", MetadataReadAt, func(m Metadata) error { _, err := m.GetReadAt(); return err }, strconv.ErrSyntax},
		{"key schema version", MetadataKeySchemaVersion, func(m Metadata) error { _, err := m.GetKeySchemaVersion(); return err }, strconv.ErrSyntax},
		{"payload schema version", MetadataPayloadSchemaVersion, func(m Metadata) error { _, err := m.GetPayloadSchemaVersion(); return err }, strconv.ErrSyntax},
		{"file size", MetadataFileSize, func(m Metadata) error { _, err := m.GetFileSize(); return err }, strconv.ErrSyntax},
		{"file chunked", MetadataFileChunked, func(m Metadata) error { _, err := m.GetFileChunked(); return err }, strconv.ErrSyntax},
		{"chunk index", MetadataFileChunkIndex, func(m Metadata) error { _, err := m.GetFileChunkIndex(); return err }, strconv.ErrSyntax},
		{"chunk count", MetadataFileChunkCount, func(m Metadata) error { _, err := m.GetFileChunkCount(); return err }, strconv.ErrSyntax},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			is := is.New(t)
			err := tc.get(Metadata{tc.key: secret})
			is.True(err != nil)
			is.True(!strings.Contains(err.Error(), secret))
			is.True(strings.Contains(err.Error(), logsafe.HashString(secret)))
			is.True(strings.Contains(err.Error(), "len "+strconv.Itoa(len(secret))))
			if tc.key == MetadataCreatedAt || tc.key == MetadataReadAt {
				is.True(strings.Contains(err.Error(), tc.key)) // constant key is fine to print
			}
			is.True(errors.Is(err, tc.is))
		})
	}

	t.Run("out of range keeps ErrRange", func(t *testing.T) {
		is := is.New(t)
		huge := "99999999999999999999999"
		_, err := Metadata{MetadataFileSize: huge}.GetFileSize()
		is.True(errors.Is(err, strconv.ErrRange))
		is.True(!strings.Contains(err.Error(), huge))
	})

	t.Run("missing value", func(t *testing.T) {
		is := is.New(t)
		_, err := Metadata{}.GetFileSize()
		is.True(errors.Is(err, ErrMetadataFieldNotFound))
	})
}

func TestOperation_UnmarshalText_ErrorDoesNotContainValue(t *testing.T) {
	is := is.New(t)
	const secret = "s3cr3t-op-77"

	var op Operation
	err := op.UnmarshalText([]byte(secret))
	is.True(errors.Is(err, ErrUnknownOperation))
	is.True(!strings.Contains(err.Error(), secret))
	is.True(strings.Contains(err.Error(), logsafe.HashString(secret)))
}
