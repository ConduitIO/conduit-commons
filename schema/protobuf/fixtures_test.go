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

package protobuf

// Schema sources shared by the unit tests and the fuzz seed corpus.
const (
	flatSchema = `syntax = "proto3";

package example.v1;

message Order {
  string id = 1;
  int64 amount_cents = 2;
  bool paid = 3;
  repeated string tags = 4;
}
`

	multipleMessagesSchema = `syntax = "proto3";

package example.v1;

message Customer {
  string id = 1;
  string name = 2;

  message Address {
    string street = 1;
    string city = 2;
  }

  Address address = 3;
}

message Invoice {
  string id = 1;
  Customer customer = 2;
}
`

	wellKnownImportSchema = `syntax = "proto3";

package example.v1;

import "google/protobuf/timestamp.proto";

message Event {
  string id = 1;
  google.protobuf.Timestamp created_at = 2;
}
`

	proto2Schema = `syntax = "proto2";

package example.v1;

message Legacy {
  required string id = 1;
  optional int32 count = 2 [default = 7];
}
`

	referenceSchema = `syntax = "proto3";

package example.v1;

import "example/v1/customer.proto";

message Order {
  string id = 1;
  example.v1.Customer customer = 2;
}
`

	selfImportSchema = `syntax = "proto3";

import "schema.proto";

message Loop {
  string id = 1;
}
`

	// Missing ';' after field 1.
	malformedSchema = `syntax = "proto3";

package example.v1;

message Order {
  string id = 1
  int64 amount_cents = 2;
}
`

	// Field number reused: parses, then fails in the linker.
	duplicateFieldNumberSchema = `syntax = "proto3";

message Order {
  string id = 1;
  string name = 1;
}
`

	unknownTypeSchema = `syntax = "proto3";

message Order {
  example.v1.DoesNotExist thing = 1;
}
`

	// Message indexes (see messageForIndex), in declaration order:
	//   [0]          Envelope
	//   [0 0]        Envelope.Header
	//   [0 1]        Envelope.Body
	//   [0 1 0]      Envelope.Body.Part
	//   [0 1 1]      Envelope.Body.Attachment
	//   [0 1 1 0]    Envelope.Body.Attachment.Meta
	//   [1]          Audit
	//   [1 0]        Audit.Change
	//   [2]          Tags
	//   [2 0]        Tags.ValuesEntry (synthetic map entry)
	nestedSchema = `syntax = "proto3";

package example.v1;

message Envelope {
  message Header {
    string id = 1;
  }
  message Body {
    message Part {
      string text = 1;
    }
    message Attachment {
      message Meta {
        string name = 1;
      }
      bytes data = 1;
      Meta meta = 2;
    }
    repeated Part parts = 1;
    repeated Attachment attachments = 2;
  }
  Header header = 1;
  Body body = 2;
}

message Audit {
  message Change {
    string field = 1;
  }
  string actor = 1;
  repeated Change changes = 2;
}

message Tags {
  map<string, string> values = 1;
}
`

	noMessagesSchema = `syntax = "proto3";

package example.v1;

enum Status {
  STATUS_UNSPECIFIED = 0;
}
`
)
