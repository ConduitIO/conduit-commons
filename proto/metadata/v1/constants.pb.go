// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.31.0
// 	protoc        (unknown)
// source: metadata/v1/constants.proto

package metadatav1

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	descriptorpb "google.golang.org/protobuf/types/descriptorpb"
	reflect "reflect"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

var file_metadata_v1_constants_proto_extTypes = []protoimpl.ExtensionInfo{
	{
		ExtendedType:  (*descriptorpb.FileOptions)(nil),
		ExtensionType: (*string)(nil),
		Field:         20000,
		Name:          "metadata.v1.metadata_conduit_source_plugin_name",
		Tag:           "bytes,20000,opt,name=metadata_conduit_source_plugin_name",
		Filename:      "metadata/v1/constants.proto",
	},
	{
		ExtendedType:  (*descriptorpb.FileOptions)(nil),
		ExtensionType: (*string)(nil),
		Field:         20001,
		Name:          "metadata.v1.metadata_conduit_source_plugin_version",
		Tag:           "bytes,20001,opt,name=metadata_conduit_source_plugin_version",
		Filename:      "metadata/v1/constants.proto",
	},
	{
		ExtendedType:  (*descriptorpb.FileOptions)(nil),
		ExtensionType: (*string)(nil),
		Field:         20002,
		Name:          "metadata.v1.metadata_conduit_destination_plugin_name",
		Tag:           "bytes,20002,opt,name=metadata_conduit_destination_plugin_name",
		Filename:      "metadata/v1/constants.proto",
	},
	{
		ExtendedType:  (*descriptorpb.FileOptions)(nil),
		ExtensionType: (*string)(nil),
		Field:         20003,
		Name:          "metadata.v1.metadata_conduit_destination_plugin_version",
		Tag:           "bytes,20003,opt,name=metadata_conduit_destination_plugin_version",
		Filename:      "metadata/v1/constants.proto",
	},
	{
		ExtendedType:  (*descriptorpb.FileOptions)(nil),
		ExtensionType: (*string)(nil),
		Field:         20004,
		Name:          "metadata.v1.metadata_conduit_source_connector_id",
		Tag:           "bytes,20004,opt,name=metadata_conduit_source_connector_id",
		Filename:      "metadata/v1/constants.proto",
	},
	{
		ExtendedType:  (*descriptorpb.FileOptions)(nil),
		ExtensionType: (*string)(nil),
		Field:         20005,
		Name:          "metadata.v1.metadata_conduit_dlq_nack_error",
		Tag:           "bytes,20005,opt,name=metadata_conduit_dlq_nack_error",
		Filename:      "metadata/v1/constants.proto",
	},
	{
		ExtendedType:  (*descriptorpb.FileOptions)(nil),
		ExtensionType: (*string)(nil),
		Field:         20006,
		Name:          "metadata.v1.metadata_conduit_dlq_nack_node_id",
		Tag:           "bytes,20006,opt,name=metadata_conduit_dlq_nack_node_id",
		Filename:      "metadata/v1/constants.proto",
	},
}

// Extension fields to descriptorpb.FileOptions.
var (
	// Metadata field "conduit.source.plugin.name" contains the name of the source
	// plugin that created this record.
	//
	// optional string metadata_conduit_source_plugin_name = 20000;
	E_MetadataConduitSourcePluginName = &file_metadata_v1_constants_proto_extTypes[0]
	// Metadata field "conduit.source.plugin.version" contains the version of the
	// source plugin that created this record.
	//
	// optional string metadata_conduit_source_plugin_version = 20001;
	E_MetadataConduitSourcePluginVersion = &file_metadata_v1_constants_proto_extTypes[1]
	// Metadata field "conduit.destination.plugin.name" contains the name of the
	// destination plugin that has written this record (only available in records
	// once they are written by a destination).
	//
	// optional string metadata_conduit_destination_plugin_name = 20002;
	E_MetadataConduitDestinationPluginName = &file_metadata_v1_constants_proto_extTypes[2]
	// Metadata field "conduit.destination.plugin.version" contains the version of
	// the destination plugin that has written this record (only available in
	// records once they are written by a destination).
	//
	// optional string metadata_conduit_destination_plugin_version = 20003;
	E_MetadataConduitDestinationPluginVersion = &file_metadata_v1_constants_proto_extTypes[3]
	// Metadata field "conduit.source.connector.id" contains the ID of the source
	// connector that produced this record.
	//
	// optional string metadata_conduit_source_connector_id = 20004;
	E_MetadataConduitSourceConnectorId = &file_metadata_v1_constants_proto_extTypes[4]
	// Metadata field "conduit.dlq.nack.error" contains the error that caused a
	// record to be nacked and pushed to the dead-letter queue.
	//
	// optional string metadata_conduit_dlq_nack_error = 20005;
	E_MetadataConduitDlqNackError = &file_metadata_v1_constants_proto_extTypes[5]
	// Metadata field "conduit.dlq.nack.node.id" contains the ID of the internal
	// node that nacked the record.
	//
	// optional string metadata_conduit_dlq_nack_node_id = 20006;
	E_MetadataConduitDlqNackNodeId = &file_metadata_v1_constants_proto_extTypes[6]
)

var File_metadata_v1_constants_proto protoreflect.FileDescriptor

var file_metadata_v1_constants_proto_rawDesc = []byte{
	0x0a, 0x1b, 0x6d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0x2f, 0x76, 0x31, 0x2f, 0x63, 0x6f,
	0x6e, 0x73, 0x74, 0x61, 0x6e, 0x74, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x0b, 0x6d,
	0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0x2e, 0x76, 0x31, 0x1a, 0x20, 0x67, 0x6f, 0x6f, 0x67,
	0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x64, 0x65, 0x73, 0x63,
	0x72, 0x69, 0x70, 0x74, 0x6f, 0x72, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x3a, 0x6c, 0x0a, 0x23,
	0x6d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0x5f, 0x63, 0x6f, 0x6e, 0x64, 0x75, 0x69, 0x74,
	0x5f, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x5f, 0x70, 0x6c, 0x75, 0x67, 0x69, 0x6e, 0x5f, 0x6e,
	0x61, 0x6d, 0x65, 0x12, 0x1c, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x46, 0x69, 0x6c, 0x65, 0x4f, 0x70, 0x74, 0x69, 0x6f, 0x6e,
	0x73, 0x18, 0xa0, 0x9c, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x1f, 0x6d, 0x65, 0x74, 0x61, 0x64,
	0x61, 0x74, 0x61, 0x43, 0x6f, 0x6e, 0x64, 0x75, 0x69, 0x74, 0x53, 0x6f, 0x75, 0x72, 0x63, 0x65,
	0x50, 0x6c, 0x75, 0x67, 0x69, 0x6e, 0x4e, 0x61, 0x6d, 0x65, 0x3a, 0x72, 0x0a, 0x26, 0x6d, 0x65,
	0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0x5f, 0x63, 0x6f, 0x6e, 0x64, 0x75, 0x69, 0x74, 0x5f, 0x73,
	0x6f, 0x75, 0x72, 0x63, 0x65, 0x5f, 0x70, 0x6c, 0x75, 0x67, 0x69, 0x6e, 0x5f, 0x76, 0x65, 0x72,
	0x73, 0x69, 0x6f, 0x6e, 0x12, 0x1c, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x46, 0x69, 0x6c, 0x65, 0x4f, 0x70, 0x74, 0x69, 0x6f,
	0x6e, 0x73, 0x18, 0xa1, 0x9c, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x22, 0x6d, 0x65, 0x74, 0x61,
	0x64, 0x61, 0x74, 0x61, 0x43, 0x6f, 0x6e, 0x64, 0x75, 0x69, 0x74, 0x53, 0x6f, 0x75, 0x72, 0x63,
	0x65, 0x50, 0x6c, 0x75, 0x67, 0x69, 0x6e, 0x56, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, 0x3a, 0x76,
	0x0a, 0x28, 0x6d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0x5f, 0x63, 0x6f, 0x6e, 0x64, 0x75,
	0x69, 0x74, 0x5f, 0x64, 0x65, 0x73, 0x74, 0x69, 0x6e, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x5f, 0x70,
	0x6c, 0x75, 0x67, 0x69, 0x6e, 0x5f, 0x6e, 0x61, 0x6d, 0x65, 0x12, 0x1c, 0x2e, 0x67, 0x6f, 0x6f,
	0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x46, 0x69, 0x6c,
	0x65, 0x4f, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x18, 0xa2, 0x9c, 0x01, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x24, 0x6d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0x43, 0x6f, 0x6e, 0x64, 0x75, 0x69,
	0x74, 0x44, 0x65, 0x73, 0x74, 0x69, 0x6e, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x50, 0x6c, 0x75, 0x67,
	0x69, 0x6e, 0x4e, 0x61, 0x6d, 0x65, 0x3a, 0x7c, 0x0a, 0x2b, 0x6d, 0x65, 0x74, 0x61, 0x64, 0x61,
	0x74, 0x61, 0x5f, 0x63, 0x6f, 0x6e, 0x64, 0x75, 0x69, 0x74, 0x5f, 0x64, 0x65, 0x73, 0x74, 0x69,
	0x6e, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x5f, 0x70, 0x6c, 0x75, 0x67, 0x69, 0x6e, 0x5f, 0x76, 0x65,
	0x72, 0x73, 0x69, 0x6f, 0x6e, 0x12, 0x1c, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x46, 0x69, 0x6c, 0x65, 0x4f, 0x70, 0x74, 0x69,
	0x6f, 0x6e, 0x73, 0x18, 0xa3, 0x9c, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x27, 0x6d, 0x65, 0x74,
	0x61, 0x64, 0x61, 0x74, 0x61, 0x43, 0x6f, 0x6e, 0x64, 0x75, 0x69, 0x74, 0x44, 0x65, 0x73, 0x74,
	0x69, 0x6e, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x50, 0x6c, 0x75, 0x67, 0x69, 0x6e, 0x56, 0x65, 0x72,
	0x73, 0x69, 0x6f, 0x6e, 0x3a, 0x6e, 0x0a, 0x24, 0x6d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61,
	0x5f, 0x63, 0x6f, 0x6e, 0x64, 0x75, 0x69, 0x74, 0x5f, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x5f,
	0x63, 0x6f, 0x6e, 0x6e, 0x65, 0x63, 0x74, 0x6f, 0x72, 0x5f, 0x69, 0x64, 0x12, 0x1c, 0x2e, 0x67,
	0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x46,
	0x69, 0x6c, 0x65, 0x4f, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x18, 0xa4, 0x9c, 0x01, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x20, 0x6d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0x43, 0x6f, 0x6e, 0x64,
	0x75, 0x69, 0x74, 0x53, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x43, 0x6f, 0x6e, 0x6e, 0x65, 0x63, 0x74,
	0x6f, 0x72, 0x49, 0x64, 0x3a, 0x64, 0x0a, 0x1f, 0x6d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61,
	0x5f, 0x63, 0x6f, 0x6e, 0x64, 0x75, 0x69, 0x74, 0x5f, 0x64, 0x6c, 0x71, 0x5f, 0x6e, 0x61, 0x63,
	0x6b, 0x5f, 0x65, 0x72, 0x72, 0x6f, 0x72, 0x12, 0x1c, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x46, 0x69, 0x6c, 0x65, 0x4f, 0x70,
	0x74, 0x69, 0x6f, 0x6e, 0x73, 0x18, 0xa5, 0x9c, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x1b, 0x6d,
	0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0x43, 0x6f, 0x6e, 0x64, 0x75, 0x69, 0x74, 0x44, 0x6c,
	0x71, 0x4e, 0x61, 0x63, 0x6b, 0x45, 0x72, 0x72, 0x6f, 0x72, 0x3a, 0x67, 0x0a, 0x21, 0x6d, 0x65,
	0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0x5f, 0x63, 0x6f, 0x6e, 0x64, 0x75, 0x69, 0x74, 0x5f, 0x64,
	0x6c, 0x71, 0x5f, 0x6e, 0x61, 0x63, 0x6b, 0x5f, 0x6e, 0x6f, 0x64, 0x65, 0x5f, 0x69, 0x64, 0x12,
	0x1c, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75,
	0x66, 0x2e, 0x46, 0x69, 0x6c, 0x65, 0x4f, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x18, 0xa6, 0x9c,
	0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x1c, 0x6d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0x43,
	0x6f, 0x6e, 0x64, 0x75, 0x69, 0x74, 0x44, 0x6c, 0x71, 0x4e, 0x61, 0x63, 0x6b, 0x4e, 0x6f, 0x64,
	0x65, 0x49, 0x64, 0x42, 0x8e, 0x03, 0x82, 0xe2, 0x09, 0x1a, 0x63, 0x6f, 0x6e, 0x64, 0x75, 0x69,
	0x74, 0x2e, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x2e, 0x70, 0x6c, 0x75, 0x67, 0x69, 0x6e, 0x2e,
	0x6e, 0x61, 0x6d, 0x65, 0x8a, 0xe2, 0x09, 0x1d, 0x63, 0x6f, 0x6e, 0x64, 0x75, 0x69, 0x74, 0x2e,
	0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x2e, 0x70, 0x6c, 0x75, 0x67, 0x69, 0x6e, 0x2e, 0x76, 0x65,
	0x72, 0x73, 0x69, 0x6f, 0x6e, 0x92, 0xe2, 0x09, 0x1f, 0x63, 0x6f, 0x6e, 0x64, 0x75, 0x69, 0x74,
	0x2e, 0x64, 0x65, 0x73, 0x74, 0x69, 0x6e, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x2e, 0x70, 0x6c, 0x75,
	0x67, 0x69, 0x6e, 0x2e, 0x6e, 0x61, 0x6d, 0x65, 0x9a, 0xe2, 0x09, 0x22, 0x63, 0x6f, 0x6e, 0x64,
	0x75, 0x69, 0x74, 0x2e, 0x64, 0x65, 0x73, 0x74, 0x69, 0x6e, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x2e,
	0x70, 0x6c, 0x75, 0x67, 0x69, 0x6e, 0x2e, 0x76, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, 0xa2, 0xe2,
	0x09, 0x1b, 0x63, 0x6f, 0x6e, 0x64, 0x75, 0x69, 0x74, 0x2e, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65,
	0x2e, 0x63, 0x6f, 0x6e, 0x6e, 0x65, 0x63, 0x74, 0x6f, 0x72, 0x2e, 0x69, 0x64, 0xaa, 0xe2, 0x09,
	0x16, 0x63, 0x6f, 0x6e, 0x64, 0x75, 0x69, 0x74, 0x2e, 0x64, 0x6c, 0x71, 0x2e, 0x6e, 0x61, 0x63,
	0x6b, 0x2e, 0x65, 0x72, 0x72, 0x6f, 0x72, 0xb2, 0xe2, 0x09, 0x18, 0x63, 0x6f, 0x6e, 0x64, 0x75,
	0x69, 0x74, 0x2e, 0x64, 0x6c, 0x71, 0x2e, 0x6e, 0x61, 0x63, 0x6b, 0x2e, 0x6e, 0x6f, 0x64, 0x65,
	0x2e, 0x69, 0x64, 0x0a, 0x0f, 0x63, 0x6f, 0x6d, 0x2e, 0x6d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74,
	0x61, 0x2e, 0x76, 0x31, 0x42, 0x0e, 0x43, 0x6f, 0x6e, 0x73, 0x74, 0x61, 0x6e, 0x74, 0x73, 0x50,
	0x72, 0x6f, 0x74, 0x6f, 0x50, 0x01, 0x5a, 0x41, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63,
	0x6f, 0x6d, 0x2f, 0x63, 0x6f, 0x6e, 0x64, 0x75, 0x69, 0x74, 0x69, 0x6f, 0x2f, 0x63, 0x6f, 0x6e,
	0x64, 0x75, 0x69, 0x74, 0x2d, 0x63, 0x6f, 0x6d, 0x6d, 0x6f, 0x6e, 0x73, 0x2f, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x2f, 0x6d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0x2f, 0x76, 0x31, 0x3b, 0x6d,
	0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0x76, 0x31, 0xa2, 0x02, 0x03, 0x4d, 0x58, 0x58, 0xaa,
	0x02, 0x0b, 0x4d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0x2e, 0x56, 0x31, 0xca, 0x02, 0x0b,
	0x4d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0x5c, 0x56, 0x31, 0xe2, 0x02, 0x17, 0x4d, 0x65,
	0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0x5c, 0x56, 0x31, 0x5c, 0x47, 0x50, 0x42, 0x4d, 0x65, 0x74,
	0x61, 0x64, 0x61, 0x74, 0x61, 0xea, 0x02, 0x0c, 0x4d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61,
	0x3a, 0x3a, 0x56, 0x31, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var file_metadata_v1_constants_proto_goTypes = []interface{}{
	(*descriptorpb.FileOptions)(nil), // 0: google.protobuf.FileOptions
}
var file_metadata_v1_constants_proto_depIdxs = []int32{
	0, // 0: metadata.v1.metadata_conduit_source_plugin_name:extendee -> google.protobuf.FileOptions
	0, // 1: metadata.v1.metadata_conduit_source_plugin_version:extendee -> google.protobuf.FileOptions
	0, // 2: metadata.v1.metadata_conduit_destination_plugin_name:extendee -> google.protobuf.FileOptions
	0, // 3: metadata.v1.metadata_conduit_destination_plugin_version:extendee -> google.protobuf.FileOptions
	0, // 4: metadata.v1.metadata_conduit_source_connector_id:extendee -> google.protobuf.FileOptions
	0, // 5: metadata.v1.metadata_conduit_dlq_nack_error:extendee -> google.protobuf.FileOptions
	0, // 6: metadata.v1.metadata_conduit_dlq_nack_node_id:extendee -> google.protobuf.FileOptions
	7, // [7:7] is the sub-list for method output_type
	7, // [7:7] is the sub-list for method input_type
	7, // [7:7] is the sub-list for extension type_name
	0, // [0:7] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_metadata_v1_constants_proto_init() }
func file_metadata_v1_constants_proto_init() {
	if File_metadata_v1_constants_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_metadata_v1_constants_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   0,
			NumExtensions: 7,
			NumServices:   0,
		},
		GoTypes:           file_metadata_v1_constants_proto_goTypes,
		DependencyIndexes: file_metadata_v1_constants_proto_depIdxs,
		ExtensionInfos:    file_metadata_v1_constants_proto_extTypes,
	}.Build()
	File_metadata_v1_constants_proto = out.File
	file_metadata_v1_constants_proto_rawDesc = nil
	file_metadata_v1_constants_proto_goTypes = nil
	file_metadata_v1_constants_proto_depIdxs = nil
}
