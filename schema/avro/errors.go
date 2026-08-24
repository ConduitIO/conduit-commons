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
	// a Serde's configured input-size ceiling (see WithMaxInputSize in
	// limits.go; there is no ceiling, and this error is never returned,
	// unless one was explicitly configured). See limits.go for why this
	// bound exists: it started as part of the near-term mitigation for
	// three decoder advisories in the previously used, now-archived
	// github.com/hamba/avro/v2 (GO-2026-5046, GO-2026-5047, GO-2026-5048),
	// and remains permanent, optional defense-in-depth after the codec was
	// replaced with the maintained github.com/iskorotkov/avro/v2 fork, per
	// docs/design-documents/20260823-avro-codec-archived-decoder-advisories.md
	// in ConduitIO/conduit -- see that file's package doc for which
	// advisories the codec swap does and does not close on its own.
	// Failing closed here rather than truncating or otherwise coercing the
	// input is deliberate: this package never silently mangles data.
	ErrInputTooLarge = errors.New("avro input exceeds maximum allowed size")

	// ErrInvalidOption is returned by Parse/SerdeForType (wrapped with
	// context identifying which option and value) when an Option given to
	// them rejects its own argument -- currently WithMaxSliceAllocSize,
	// WithMaxMapAllocSize, and the package-level SetDefaultMaxSliceAllocSize
	// / SetDefaultMaxMapAllocSize all reject n <= 0 this way, since neither
	// has an "unlimited" value (see limits.go's package doc, "Default
	// allocation ceilings"). This is a construction-time error, never
	// returned by Marshal/Unmarshal.
	ErrInvalidOption = errors.New("invalid avro serde option")
)
