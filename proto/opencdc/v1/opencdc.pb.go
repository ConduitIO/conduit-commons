// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.31.0
// 	protoc        (unknown)
// source: opencdc/v1/opencdc.proto

package opencdcv1

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	descriptorpb "google.golang.org/protobuf/types/descriptorpb"
	structpb "google.golang.org/protobuf/types/known/structpb"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// Operation defines what triggered the creation of a record.
type Operation int32

const (
	Operation_OPERATION_UNSPECIFIED Operation = 0
	// Records with operation create contain data of a newly created entity.
	Operation_OPERATION_CREATE Operation = 1
	// Records with operation update contain data of an updated entity.
	Operation_OPERATION_UPDATE Operation = 2
	// Records with operation delete contain data of a deleted entity.
	Operation_OPERATION_DELETE Operation = 3
	// Records with operation snapshot contain data of a previously existing
	// entity, fetched as part of a snapshot.
	Operation_OPERATION_SNAPSHOT Operation = 4
)

// Enum value maps for Operation.
var (
	Operation_name = map[int32]string{
		0: "OPERATION_UNSPECIFIED",
		1: "OPERATION_CREATE",
		2: "OPERATION_UPDATE",
		3: "OPERATION_DELETE",
		4: "OPERATION_SNAPSHOT",
	}
	Operation_value = map[string]int32{
		"OPERATION_UNSPECIFIED": 0,
		"OPERATION_CREATE":      1,
		"OPERATION_UPDATE":      2,
		"OPERATION_DELETE":      3,
		"OPERATION_SNAPSHOT":    4,
	}
)

func (x Operation) Enum() *Operation {
	p := new(Operation)
	*p = x
	return p
}

func (x Operation) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (Operation) Descriptor() protoreflect.EnumDescriptor {
	return file_opencdc_v1_opencdc_proto_enumTypes[0].Descriptor()
}

func (Operation) Type() protoreflect.EnumType {
	return &file_opencdc_v1_opencdc_proto_enumTypes[0]
}

func (x Operation) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use Operation.Descriptor instead.
func (Operation) EnumDescriptor() ([]byte, []int) {
	return file_opencdc_v1_opencdc_proto_rawDescGZIP(), []int{0}
}

// Record contains data about a single change event related to a single entity.
type Record struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Position uniquely identifies the record.
	Position []byte `protobuf:"bytes,1,opt,name=position,proto3" json:"position,omitempty"`
	// Operation defines what triggered the creation of a record. There are four
	// possibilities: create, update, delete or snapshot. The first three
	// operations are encountered during normal CDC operation, while "snapshot" is
	// meant to represent records during an initial load. Depending on the
	// operation, the record will contain either the payload before the change,
	// after the change, or both (see field payload).
	Operation Operation `protobuf:"varint,2,opt,name=operation,proto3,enum=opencdc.v1.Operation" json:"operation,omitempty"`
	// Metadata contains optional information related to the record. Although the
	// map can contain arbitrary keys, the standard provides a set of standard
	// metadata fields (see options prefixed with metadata_*).
	Metadata map[string]string `protobuf:"bytes,3,rep,name=metadata,proto3" json:"metadata,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
	// Key represents a value that should identify the entity (e.g. database row).
	Key *Data `protobuf:"bytes,4,opt,name=key,proto3" json:"key,omitempty"`
	// Payload holds the payload change (data before and after the operation
	// occurred).
	Payload *Change `protobuf:"bytes,5,opt,name=payload,proto3" json:"payload,omitempty"`
}

func (x *Record) Reset() {
	*x = Record{}
	if protoimpl.UnsafeEnabled {
		mi := &file_opencdc_v1_opencdc_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Record) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Record) ProtoMessage() {}

func (x *Record) ProtoReflect() protoreflect.Message {
	mi := &file_opencdc_v1_opencdc_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Record.ProtoReflect.Descriptor instead.
func (*Record) Descriptor() ([]byte, []int) {
	return file_opencdc_v1_opencdc_proto_rawDescGZIP(), []int{0}
}

func (x *Record) GetPosition() []byte {
	if x != nil {
		return x.Position
	}
	return nil
}

func (x *Record) GetOperation() Operation {
	if x != nil {
		return x.Operation
	}
	return Operation_OPERATION_UNSPECIFIED
}

func (x *Record) GetMetadata() map[string]string {
	if x != nil {
		return x.Metadata
	}
	return nil
}

func (x *Record) GetKey() *Data {
	if x != nil {
		return x.Key
	}
	return nil
}

func (x *Record) GetPayload() *Change {
	if x != nil {
		return x.Payload
	}
	return nil
}

// Change represents the data before and after the operation occurred.
type Change struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Before contains the data before the operation occurred. This field is
	// optional and should only be populated for operations "update" and "delete"
	// (if the system supports fetching the data before the operation).
	Before *Data `protobuf:"bytes,1,opt,name=before,proto3" json:"before,omitempty"`
	// After contains the data after the operation occurred. This field should be
	// populated for all operations except "delete".
	After *Data `protobuf:"bytes,2,opt,name=after,proto3" json:"after,omitempty"`
}

func (x *Change) Reset() {
	*x = Change{}
	if protoimpl.UnsafeEnabled {
		mi := &file_opencdc_v1_opencdc_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Change) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Change) ProtoMessage() {}

func (x *Change) ProtoReflect() protoreflect.Message {
	mi := &file_opencdc_v1_opencdc_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Change.ProtoReflect.Descriptor instead.
func (*Change) Descriptor() ([]byte, []int) {
	return file_opencdc_v1_opencdc_proto_rawDescGZIP(), []int{1}
}

func (x *Change) GetBefore() *Data {
	if x != nil {
		return x.Before
	}
	return nil
}

func (x *Change) GetAfter() *Data {
	if x != nil {
		return x.After
	}
	return nil
}

// Data is used to represent the record key and payload. It can be either raw
// data (byte array) or structured data (struct).
type Data struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Types that are assignable to Data:
	//
	//	*Data_RawData
	//	*Data_StructuredData
	Data isData_Data `protobuf_oneof:"data"`
}

func (x *Data) Reset() {
	*x = Data{}
	if protoimpl.UnsafeEnabled {
		mi := &file_opencdc_v1_opencdc_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Data) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Data) ProtoMessage() {}

func (x *Data) ProtoReflect() protoreflect.Message {
	mi := &file_opencdc_v1_opencdc_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Data.ProtoReflect.Descriptor instead.
func (*Data) Descriptor() ([]byte, []int) {
	return file_opencdc_v1_opencdc_proto_rawDescGZIP(), []int{2}
}

func (m *Data) GetData() isData_Data {
	if m != nil {
		return m.Data
	}
	return nil
}

func (x *Data) GetRawData() []byte {
	if x, ok := x.GetData().(*Data_RawData); ok {
		return x.RawData
	}
	return nil
}

func (x *Data) GetStructuredData() *structpb.Struct {
	if x, ok := x.GetData().(*Data_StructuredData); ok {
		return x.StructuredData
	}
	return nil
}

type isData_Data interface {
	isData_Data()
}

type Data_RawData struct {
	// Raw data contains unstructured data in form of a byte array.
	RawData []byte `protobuf:"bytes,1,opt,name=raw_data,json=rawData,proto3,oneof"`
}

type Data_StructuredData struct {
	// Structured data contains data in form of a struct with fields.
	StructuredData *structpb.Struct `protobuf:"bytes,2,opt,name=structured_data,json=structuredData,proto3,oneof"`
}

func (*Data_RawData) isData_Data() {}

func (*Data_StructuredData) isData_Data() {}

var file_opencdc_v1_opencdc_proto_extTypes = []protoimpl.ExtensionInfo{
	{
		ExtendedType:  (*descriptorpb.FileOptions)(nil),
		ExtensionType: (*string)(nil),
		Field:         9999,
		Name:          "opencdc.v1.opencdc_version",
		Tag:           "bytes,9999,opt,name=opencdc_version",
		Filename:      "opencdc/v1/opencdc.proto",
	},
	{
		ExtendedType:  (*descriptorpb.FileOptions)(nil),
		ExtensionType: (*string)(nil),
		Field:         10000,
		Name:          "opencdc.v1.metadata_version",
		Tag:           "bytes,10000,opt,name=metadata_version",
		Filename:      "opencdc/v1/opencdc.proto",
	},
	{
		ExtendedType:  (*descriptorpb.FileOptions)(nil),
		ExtensionType: (*string)(nil),
		Field:         10001,
		Name:          "opencdc.v1.metadata_created_at",
		Tag:           "bytes,10001,opt,name=metadata_created_at",
		Filename:      "opencdc/v1/opencdc.proto",
	},
	{
		ExtendedType:  (*descriptorpb.FileOptions)(nil),
		ExtensionType: (*string)(nil),
		Field:         10002,
		Name:          "opencdc.v1.metadata_read_at",
		Tag:           "bytes,10002,opt,name=metadata_read_at",
		Filename:      "opencdc/v1/opencdc.proto",
	},
}

// Extension fields to descriptorpb.FileOptions.
var (
	// optional string opencdc_version = 9999;
	E_OpencdcVersion = &file_opencdc_v1_opencdc_proto_extTypes[0]
	// optional string metadata_version = 10000;
	E_MetadataVersion = &file_opencdc_v1_opencdc_proto_extTypes[1]
	// optional string metadata_created_at = 10001;
	E_MetadataCreatedAt = &file_opencdc_v1_opencdc_proto_extTypes[2]
	// optional string metadata_read_at = 10002;
	E_MetadataReadAt = &file_opencdc_v1_opencdc_proto_extTypes[3]
)

var File_opencdc_v1_opencdc_proto protoreflect.FileDescriptor

var file_opencdc_v1_opencdc_proto_rawDesc = []byte{
	0x0a, 0x18, 0x6f, 0x70, 0x65, 0x6e, 0x63, 0x64, 0x63, 0x2f, 0x76, 0x31, 0x2f, 0x6f, 0x70, 0x65,
	0x6e, 0x63, 0x64, 0x63, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x0a, 0x6f, 0x70, 0x65, 0x6e,
	0x63, 0x64, 0x63, 0x2e, 0x76, 0x31, 0x1a, 0x20, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74,
	0x6f, 0x72, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x1c, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65,
	0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x73, 0x74, 0x72, 0x75, 0x63, 0x74,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0xa6, 0x02, 0x0a, 0x06, 0x52, 0x65, 0x63, 0x6f, 0x72,
	0x64, 0x12, 0x1a, 0x0a, 0x08, 0x70, 0x6f, 0x73, 0x69, 0x74, 0x69, 0x6f, 0x6e, 0x18, 0x01, 0x20,
	0x01, 0x28, 0x0c, 0x52, 0x08, 0x70, 0x6f, 0x73, 0x69, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x33, 0x0a,
	0x09, 0x6f, 0x70, 0x65, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0e,
	0x32, 0x15, 0x2e, 0x6f, 0x70, 0x65, 0x6e, 0x63, 0x64, 0x63, 0x2e, 0x76, 0x31, 0x2e, 0x4f, 0x70,
	0x65, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x52, 0x09, 0x6f, 0x70, 0x65, 0x72, 0x61, 0x74, 0x69,
	0x6f, 0x6e, 0x12, 0x3c, 0x0a, 0x08, 0x6d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0x18, 0x03,
	0x20, 0x03, 0x28, 0x0b, 0x32, 0x20, 0x2e, 0x6f, 0x70, 0x65, 0x6e, 0x63, 0x64, 0x63, 0x2e, 0x76,
	0x31, 0x2e, 0x52, 0x65, 0x63, 0x6f, 0x72, 0x64, 0x2e, 0x4d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74,
	0x61, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x52, 0x08, 0x6d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61,
	0x12, 0x22, 0x0a, 0x03, 0x6b, 0x65, 0x79, 0x18, 0x04, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x10, 0x2e,
	0x6f, 0x70, 0x65, 0x6e, 0x63, 0x64, 0x63, 0x2e, 0x76, 0x31, 0x2e, 0x44, 0x61, 0x74, 0x61, 0x52,
	0x03, 0x6b, 0x65, 0x79, 0x12, 0x2c, 0x0a, 0x07, 0x70, 0x61, 0x79, 0x6c, 0x6f, 0x61, 0x64, 0x18,
	0x05, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x12, 0x2e, 0x6f, 0x70, 0x65, 0x6e, 0x63, 0x64, 0x63, 0x2e,
	0x76, 0x31, 0x2e, 0x43, 0x68, 0x61, 0x6e, 0x67, 0x65, 0x52, 0x07, 0x70, 0x61, 0x79, 0x6c, 0x6f,
	0x61, 0x64, 0x1a, 0x3b, 0x0a, 0x0d, 0x4d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0x45, 0x6e,
	0x74, 0x72, 0x79, 0x12, 0x10, 0x0a, 0x03, 0x6b, 0x65, 0x79, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x03, 0x6b, 0x65, 0x79, 0x12, 0x14, 0x0a, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x18, 0x02,
	0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x3a, 0x02, 0x38, 0x01, 0x22,
	0x5a, 0x0a, 0x06, 0x43, 0x68, 0x61, 0x6e, 0x67, 0x65, 0x12, 0x28, 0x0a, 0x06, 0x62, 0x65, 0x66,
	0x6f, 0x72, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x10, 0x2e, 0x6f, 0x70, 0x65, 0x6e,
	0x63, 0x64, 0x63, 0x2e, 0x76, 0x31, 0x2e, 0x44, 0x61, 0x74, 0x61, 0x52, 0x06, 0x62, 0x65, 0x66,
	0x6f, 0x72, 0x65, 0x12, 0x26, 0x0a, 0x05, 0x61, 0x66, 0x74, 0x65, 0x72, 0x18, 0x02, 0x20, 0x01,
	0x28, 0x0b, 0x32, 0x10, 0x2e, 0x6f, 0x70, 0x65, 0x6e, 0x63, 0x64, 0x63, 0x2e, 0x76, 0x31, 0x2e,
	0x44, 0x61, 0x74, 0x61, 0x52, 0x05, 0x61, 0x66, 0x74, 0x65, 0x72, 0x22, 0x6f, 0x0a, 0x04, 0x44,
	0x61, 0x74, 0x61, 0x12, 0x1b, 0x0a, 0x08, 0x72, 0x61, 0x77, 0x5f, 0x64, 0x61, 0x74, 0x61, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x0c, 0x48, 0x00, 0x52, 0x07, 0x72, 0x61, 0x77, 0x44, 0x61, 0x74, 0x61,
	0x12, 0x42, 0x0a, 0x0f, 0x73, 0x74, 0x72, 0x75, 0x63, 0x74, 0x75, 0x72, 0x65, 0x64, 0x5f, 0x64,
	0x61, 0x74, 0x61, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x17, 0x2e, 0x67, 0x6f, 0x6f, 0x67,
	0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x53, 0x74, 0x72, 0x75,
	0x63, 0x74, 0x48, 0x00, 0x52, 0x0e, 0x73, 0x74, 0x72, 0x75, 0x63, 0x74, 0x75, 0x72, 0x65, 0x64,
	0x44, 0x61, 0x74, 0x61, 0x42, 0x06, 0x0a, 0x04, 0x64, 0x61, 0x74, 0x61, 0x2a, 0x80, 0x01, 0x0a,
	0x09, 0x4f, 0x70, 0x65, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x19, 0x0a, 0x15, 0x4f, 0x50,
	0x45, 0x52, 0x41, 0x54, 0x49, 0x4f, 0x4e, 0x5f, 0x55, 0x4e, 0x53, 0x50, 0x45, 0x43, 0x49, 0x46,
	0x49, 0x45, 0x44, 0x10, 0x00, 0x12, 0x14, 0x0a, 0x10, 0x4f, 0x50, 0x45, 0x52, 0x41, 0x54, 0x49,
	0x4f, 0x4e, 0x5f, 0x43, 0x52, 0x45, 0x41, 0x54, 0x45, 0x10, 0x01, 0x12, 0x14, 0x0a, 0x10, 0x4f,
	0x50, 0x45, 0x52, 0x41, 0x54, 0x49, 0x4f, 0x4e, 0x5f, 0x55, 0x50, 0x44, 0x41, 0x54, 0x45, 0x10,
	0x02, 0x12, 0x14, 0x0a, 0x10, 0x4f, 0x50, 0x45, 0x52, 0x41, 0x54, 0x49, 0x4f, 0x4e, 0x5f, 0x44,
	0x45, 0x4c, 0x45, 0x54, 0x45, 0x10, 0x03, 0x12, 0x16, 0x0a, 0x12, 0x4f, 0x50, 0x45, 0x52, 0x41,
	0x54, 0x49, 0x4f, 0x4e, 0x5f, 0x53, 0x4e, 0x41, 0x50, 0x53, 0x48, 0x4f, 0x54, 0x10, 0x04, 0x3a,
	0x46, 0x0a, 0x0f, 0x6f, 0x70, 0x65, 0x6e, 0x63, 0x64, 0x63, 0x5f, 0x76, 0x65, 0x72, 0x73, 0x69,
	0x6f, 0x6e, 0x12, 0x1c, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x62, 0x75, 0x66, 0x2e, 0x46, 0x69, 0x6c, 0x65, 0x4f, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x73,
	0x18, 0x8f, 0x4e, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0e, 0x6f, 0x70, 0x65, 0x6e, 0x63, 0x64, 0x63,
	0x56, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, 0x3a, 0x48, 0x0a, 0x10, 0x6d, 0x65, 0x74, 0x61, 0x64,
	0x61, 0x74, 0x61, 0x5f, 0x76, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, 0x12, 0x1c, 0x2e, 0x67, 0x6f,
	0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x46, 0x69,
	0x6c, 0x65, 0x4f, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x18, 0x90, 0x4e, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x0f, 0x6d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0x56, 0x65, 0x72, 0x73, 0x69, 0x6f,
	0x6e, 0x3a, 0x4d, 0x0a, 0x13, 0x6d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0x5f, 0x63, 0x72,
	0x65, 0x61, 0x74, 0x65, 0x64, 0x5f, 0x61, 0x74, 0x12, 0x1c, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c,
	0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x46, 0x69, 0x6c, 0x65, 0x4f,
	0x70, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x18, 0x91, 0x4e, 0x20, 0x01, 0x28, 0x09, 0x52, 0x11, 0x6d,
	0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0x43, 0x72, 0x65, 0x61, 0x74, 0x65, 0x64, 0x41, 0x74,
	0x3a, 0x47, 0x0a, 0x10, 0x6d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0x5f, 0x72, 0x65, 0x61,
	0x64, 0x5f, 0x61, 0x74, 0x12, 0x1c, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x46, 0x69, 0x6c, 0x65, 0x4f, 0x70, 0x74, 0x69, 0x6f,
	0x6e, 0x73, 0x18, 0x92, 0x4e, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0e, 0x6d, 0x65, 0x74, 0x61, 0x64,
	0x61, 0x74, 0x61, 0x52, 0x65, 0x61, 0x64, 0x41, 0x74, 0x42, 0xe8, 0x01, 0xfa, 0xf0, 0x04, 0x02,
	0x76, 0x31, 0x82, 0xf1, 0x04, 0x0f, 0x6f, 0x70, 0x65, 0x6e, 0x63, 0x64, 0x63, 0x2e, 0x76, 0x65,
	0x72, 0x73, 0x69, 0x6f, 0x6e, 0x8a, 0xf1, 0x04, 0x11, 0x6f, 0x70, 0x65, 0x6e, 0x63, 0x64, 0x63,
	0x2e, 0x63, 0x72, 0x65, 0x61, 0x74, 0x65, 0x64, 0x41, 0x74, 0x92, 0xf1, 0x04, 0x0e, 0x6f, 0x70,
	0x65, 0x6e, 0x63, 0x64, 0x63, 0x2e, 0x72, 0x65, 0x61, 0x64, 0x41, 0x74, 0x0a, 0x0e, 0x63, 0x6f,
	0x6d, 0x2e, 0x6f, 0x70, 0x65, 0x6e, 0x63, 0x64, 0x63, 0x2e, 0x76, 0x31, 0x42, 0x0c, 0x4f, 0x70,
	0x65, 0x6e, 0x63, 0x64, 0x63, 0x50, 0x72, 0x6f, 0x74, 0x6f, 0x50, 0x01, 0x5a, 0x3f, 0x67, 0x69,
	0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x63, 0x6f, 0x6e, 0x64, 0x75, 0x69, 0x74,
	0x69, 0x6f, 0x2f, 0x63, 0x6f, 0x6e, 0x64, 0x75, 0x69, 0x74, 0x2d, 0x63, 0x6f, 0x6d, 0x6d, 0x6f,
	0x6e, 0x73, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x6f, 0x70, 0x65, 0x6e, 0x63, 0x64, 0x63,
	0x2f, 0x76, 0x31, 0x3b, 0x6f, 0x70, 0x65, 0x6e, 0x63, 0x64, 0x63, 0x76, 0x31, 0xa2, 0x02, 0x03,
	0x4f, 0x58, 0x58, 0xaa, 0x02, 0x0a, 0x4f, 0x70, 0x65, 0x6e, 0x63, 0x64, 0x63, 0x2e, 0x56, 0x31,
	0xca, 0x02, 0x0a, 0x4f, 0x70, 0x65, 0x6e, 0x63, 0x64, 0x63, 0x5c, 0x56, 0x31, 0xe2, 0x02, 0x16,
	0x4f, 0x70, 0x65, 0x6e, 0x63, 0x64, 0x63, 0x5c, 0x56, 0x31, 0x5c, 0x47, 0x50, 0x42, 0x4d, 0x65,
	0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0xea, 0x02, 0x0b, 0x4f, 0x70, 0x65, 0x6e, 0x63, 0x64, 0x63,
	0x3a, 0x3a, 0x56, 0x31, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_opencdc_v1_opencdc_proto_rawDescOnce sync.Once
	file_opencdc_v1_opencdc_proto_rawDescData = file_opencdc_v1_opencdc_proto_rawDesc
)

func file_opencdc_v1_opencdc_proto_rawDescGZIP() []byte {
	file_opencdc_v1_opencdc_proto_rawDescOnce.Do(func() {
		file_opencdc_v1_opencdc_proto_rawDescData = protoimpl.X.CompressGZIP(file_opencdc_v1_opencdc_proto_rawDescData)
	})
	return file_opencdc_v1_opencdc_proto_rawDescData
}

var file_opencdc_v1_opencdc_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_opencdc_v1_opencdc_proto_msgTypes = make([]protoimpl.MessageInfo, 4)
var file_opencdc_v1_opencdc_proto_goTypes = []interface{}{
	(Operation)(0),                   // 0: opencdc.v1.Operation
	(*Record)(nil),                   // 1: opencdc.v1.Record
	(*Change)(nil),                   // 2: opencdc.v1.Change
	(*Data)(nil),                     // 3: opencdc.v1.Data
	nil,                              // 4: opencdc.v1.Record.MetadataEntry
	(*structpb.Struct)(nil),          // 5: google.protobuf.Struct
	(*descriptorpb.FileOptions)(nil), // 6: google.protobuf.FileOptions
}
var file_opencdc_v1_opencdc_proto_depIdxs = []int32{
	0,  // 0: opencdc.v1.Record.operation:type_name -> opencdc.v1.Operation
	4,  // 1: opencdc.v1.Record.metadata:type_name -> opencdc.v1.Record.MetadataEntry
	3,  // 2: opencdc.v1.Record.key:type_name -> opencdc.v1.Data
	2,  // 3: opencdc.v1.Record.payload:type_name -> opencdc.v1.Change
	3,  // 4: opencdc.v1.Change.before:type_name -> opencdc.v1.Data
	3,  // 5: opencdc.v1.Change.after:type_name -> opencdc.v1.Data
	5,  // 6: opencdc.v1.Data.structured_data:type_name -> google.protobuf.Struct
	6,  // 7: opencdc.v1.opencdc_version:extendee -> google.protobuf.FileOptions
	6,  // 8: opencdc.v1.metadata_version:extendee -> google.protobuf.FileOptions
	6,  // 9: opencdc.v1.metadata_created_at:extendee -> google.protobuf.FileOptions
	6,  // 10: opencdc.v1.metadata_read_at:extendee -> google.protobuf.FileOptions
	11, // [11:11] is the sub-list for method output_type
	11, // [11:11] is the sub-list for method input_type
	11, // [11:11] is the sub-list for extension type_name
	7,  // [7:11] is the sub-list for extension extendee
	0,  // [0:7] is the sub-list for field type_name
}

func init() { file_opencdc_v1_opencdc_proto_init() }
func file_opencdc_v1_opencdc_proto_init() {
	if File_opencdc_v1_opencdc_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_opencdc_v1_opencdc_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Record); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_opencdc_v1_opencdc_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Change); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_opencdc_v1_opencdc_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Data); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	file_opencdc_v1_opencdc_proto_msgTypes[2].OneofWrappers = []interface{}{
		(*Data_RawData)(nil),
		(*Data_StructuredData)(nil),
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_opencdc_v1_opencdc_proto_rawDesc,
			NumEnums:      1,
			NumMessages:   4,
			NumExtensions: 4,
			NumServices:   0,
		},
		GoTypes:           file_opencdc_v1_opencdc_proto_goTypes,
		DependencyIndexes: file_opencdc_v1_opencdc_proto_depIdxs,
		EnumInfos:         file_opencdc_v1_opencdc_proto_enumTypes,
		MessageInfos:      file_opencdc_v1_opencdc_proto_msgTypes,
		ExtensionInfos:    file_opencdc_v1_opencdc_proto_extTypes,
	}.Build()
	File_opencdc_v1_opencdc_proto = out.File
	file_opencdc_v1_opencdc_proto_rawDesc = nil
	file_opencdc_v1_opencdc_proto_goTypes = nil
	file_opencdc_v1_opencdc_proto_depIdxs = nil
}
