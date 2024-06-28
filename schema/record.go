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

import "github.com/conduitio/conduit-commons/opencdc"

func AttachKeySchemaToRecord(r opencdc.Record, keySchema Instance) {
	r.Metadata.SetKeySchemaType(keySchema.Type.String())
	r.Metadata.SetKeySchemaName(keySchema.Subject)
	r.Metadata.SetKeySchemaVersion(keySchema.Version)
}

func AttachPayloadSchemaToRecord(r opencdc.Record, payloadSchema Instance) {
	r.Metadata.SetPayloadSchemaType(payloadSchema.Type.String())
	r.Metadata.SetPayloadSchemaName(payloadSchema.Subject)
	r.Metadata.SetPayloadSchemaVersion(payloadSchema.Version)
}