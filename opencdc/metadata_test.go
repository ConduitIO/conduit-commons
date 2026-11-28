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
	"strconv"
	"testing"
	"time"

	metadatav1 "github.com/conduitio/conduit-commons/proto/metadata/v1"
	opencdcv1 "github.com/conduitio/conduit-commons/proto/opencdc/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/runtime/protoimpl"
)

func TestMetadataConstants(t *testing.T) {
	wantMapping := map[string]*protoimpl.ExtensionInfo{
		OpenCDCVersion:               opencdcv1.E_OpencdcVersion,
		MetadataOpenCDCVersion:       opencdcv1.E_MetadataVersion,
		MetadataCreatedAt:            opencdcv1.E_MetadataCreatedAt,
		MetadataReadAt:               opencdcv1.E_MetadataReadAt,
		MetadataCollection:           opencdcv1.E_MetadataCollection,
		MetadataKeySchemaSubject:     opencdcv1.E_MetadataKeySchemaSubject,
		MetadataKeySchemaVersion:     opencdcv1.E_MetadataKeySchemaVersion,
		MetadataPayloadSchemaSubject: opencdcv1.E_MetadataPayloadSchemaSubject,
		MetadataPayloadSchemaVersion: opencdcv1.E_MetadataPayloadSchemaVersion,

		MetadataFileName:       opencdcv1.E_MetadataFileName,
		MetadataFileSize:       opencdcv1.E_MetadataFileSize,
		MetadataFileHash:       opencdcv1.E_MetadataFileHash,
		MetadataFileChunked:    opencdcv1.E_MetadataFileChunked,
		MetadataFileChunkIndex: opencdcv1.E_MetadataFileChunkIndex,
		MetadataFileChunkCount: opencdcv1.E_MetadataFileChunkCount,

		MetadataConduitSourcePluginName:         metadatav1.E_MetadataConduitSourcePluginName,
		MetadataConduitSourcePluginVersion:      metadatav1.E_MetadataConduitSourcePluginVersion,
		MetadataConduitDestinationPluginName:    metadatav1.E_MetadataConduitDestinationPluginName,
		MetadataConduitDestinationPluginVersion: metadatav1.E_MetadataConduitDestinationPluginVersion,
		MetadataConduitSourceConnectorID:        metadatav1.E_MetadataConduitSourceConnectorId,
		MetadataConduitDLQNackError:             metadatav1.E_MetadataConduitDlqNackError,
		MetadataConduitDLQNackNodeID:            metadatav1.E_MetadataConduitDlqNackNodeId,
	}
	for goConstant, extensionInfo := range wantMapping {
		protoConstant := proto.GetExtension(extensionInfo.TypeDescriptor().ParentFile().Options(), extensionInfo)
		if goConstant != protoConstant {
			t.Fatalf("go constant %q doesn't match proto constant %q", goConstant, protoConstant)
		}
	}
}

func TestMetadata_LookupReadAt(t *testing.T) {
	readAt := time.Unix(0, 1764201600123456789)

	testCases := []struct {
		name     string
		metadata Metadata
		want     time.Time
		wantOK   bool
	}{{
		name:     "set",
		metadata: Metadata{MetadataReadAt: strconv.FormatInt(readAt.UnixNano(), 10)},
		want:     readAt,
		wantOK:   true,
	}, {
		name:     "negative timestamp",
		metadata: Metadata{MetadataReadAt: "-1"},
		want:     time.Unix(0, -1),
		wantOK:   true,
	}, {
		name:     "absent",
		metadata: Metadata{MetadataCreatedAt: "1764201600123456789"},
		wantOK:   false,
	}, {
		name:     "empty",
		metadata: Metadata{MetadataReadAt: ""},
		wantOK:   false,
	}, {
		name:     "nil metadata",
		metadata: nil,
		wantOK:   false,
	}, {
		name:     "not a number",
		metadata: Metadata{MetadataReadAt: "yesterday"},
		wantOK:   false,
	}, {
		name:     "out of int64 range",
		metadata: Metadata{MetadataReadAt: "99999999999999999999"},
		wantOK:   false,
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := tc.metadata.LookupReadAt()
			if ok != tc.wantOK {
				t.Fatalf("LookupReadAt() ok = %v, want %v", ok, tc.wantOK)
			}
			if !got.Equal(tc.want) {
				t.Fatalf("LookupReadAt() = %v, want %v", got, tc.want)
			}

			// LookupReadAt must agree with GetReadAt on every input: it is the
			// same parse, only without the error.
			getGot, getErr := tc.metadata.GetReadAt()
			if (getErr == nil) != tc.wantOK {
				t.Fatalf("GetReadAt() err = %v, want err == nil to be %v", getErr, tc.wantOK)
			}
			if getErr == nil && !getGot.Equal(got) {
				t.Fatalf("GetReadAt() = %v, LookupReadAt() = %v, want equal", getGot, got)
			}
		})
	}
}

// TestMetadata_LookupReadAt_NoAllocs is the regression test for the reason
// LookupReadAt exists: GetReadAt reports a missing key by building an error
// with fmt.Errorf, which costs an allocation even when the caller has a
// fallback and discards it. A caller on a per-record path pays that allocation
// for every record. LookupReadAt must stay allocation-free.
func TestMetadata_LookupReadAt_NoAllocs(t *testing.T) {
	cases := map[string]Metadata{
		"absent": {MetadataConduitSourceConnectorID: "src"},
		"empty":  {MetadataReadAt: ""},
		"set":    {MetadataReadAt: "1764201600123456789"},
	}

	for name, metadata := range cases {
		t.Run(name, func(t *testing.T) {
			var (
				got time.Time
				ok  bool
			)
			allocs := testing.AllocsPerRun(100, func() {
				got, ok = metadata.LookupReadAt()
			})
			_, _ = got, ok
			if allocs != 0 {
				t.Fatalf("LookupReadAt() allocated %v objects per call, want 0", allocs)
			}
		})
	}
}

// BenchmarkMetadata_ReadAt is the before/after for the allocation: GetReadAt on
// a record without the key against LookupReadAt on the same input.
func BenchmarkMetadata_ReadAt(b *testing.B) {
	absent := Metadata{MetadataConduitSourceConnectorID: "src"}
	present := Metadata{MetadataReadAt: "1764201600123456789"}

	b.Run("GetReadAt/absent", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			_, _ = absent.GetReadAt()
		}
	})
	b.Run("LookupReadAt/absent", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			_, _ = absent.LookupReadAt()
		}
	})
	b.Run("GetReadAt/present", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			_, _ = present.GetReadAt()
		}
	})
	b.Run("LookupReadAt/present", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			_, _ = present.LookupReadAt()
		}
	})
}
