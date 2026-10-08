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

import "errors"

var (
	// ErrEncodingNotSupported is returned by Serde.Marshal and by the
	// SerdeForType function registered for schema.TypeProtobuf. Protobuf
	// support is decode-only: encoding needs a pre-authored .proto schema
	// with deliberately assigned field numbers, which can't be inferred from
	// a Go value.
	ErrEncodingNotSupported = errors.New("protobuf encoding is not supported")

	// ErrDecodeNotImplemented is returned by Serde.Unmarshal until
	// message-index handling (PB-2) and field mapping (PB-4) land. It is a
	// placeholder for unreleased work, not part of the decode contract.
	ErrDecodeNotImplemented = errors.New("protobuf decoding is not implemented yet")

	// ErrSchemaCompile is returned by Parse when protocompile rejects the
	// schema source. The wrapped error carries the file position of the
	// first problem (see protocompile's reporter.ErrorWithPos).
	ErrSchemaCompile = errors.New("failed to compile protobuf schema")

	// ErrCompileTimeout is returned by Parse when the compile does not finish
	// within the compile timeout. The returned error also matches
	// context.DeadlineExceeded, which is how schema.Schema.Serde knows not to
	// cache it.
	ErrCompileTimeout = errors.New("protobuf schema compile timed out")

	// ErrReferencesNotSupported is returned by Parse when the schema imports
	// a file other than a standard google/protobuf/*.proto import. Such an
	// import is a schema reference to another registry subject, which is not
	// resolved yet (PB-3).
	ErrReferencesNotSupported = errors.New("protobuf schema references are not supported")

	// ErrInvalidOption is returned by Parse (for a rejected Option, such as
	// WithCompileTimeout(0)) and by SetDefaultCompileTimeout when given a
	// value it rejects. The compile timeout must be > 0.
	ErrInvalidOption = errors.New("invalid protobuf serde option")
)
