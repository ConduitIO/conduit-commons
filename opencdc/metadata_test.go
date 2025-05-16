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
	"testing"

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
