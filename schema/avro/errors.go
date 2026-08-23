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

package avro

import "errors"

var (
	ErrUnsupportedType     = errors.New("unsupported avro type")
	ErrSchemaValueMismatch = errors.New("avro schema doesn't match supplied value")

	// ErrInputTooLarge is returned by Serde.Unmarshal when the input exceeds
	// MaxInputSize. See limits.go for why this bound exists: it is the
	// near-term mitigation for hamba/avro's unfixed decoder advisories
	// (GO-2026-5046, GO-2026-5047, GO-2026-5048), decided in
	// docs/design-documents/20260823-avro-codec-archived-decoder-advisories.md
	// in ConduitIO/conduit. Failing closed here rather than truncating or
	// otherwise coercing the input is deliberate: this package never
	// silently mangles data.
	ErrInputTooLarge = errors.New("avro input exceeds maximum allowed size")
)
