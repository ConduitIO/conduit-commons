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

	// ErrDecodeNotImplemented is returned by Serde.Unmarshal, after the
	// message index is resolved, until field mapping (PB-4) lands. It is a
	// placeholder for unreleased work, not part of the decode contract.
	ErrDecodeNotImplemented = errors.New("protobuf decoding is not implemented yet")

	// ErrMessageIndex is returned by Serde.Unmarshal when the Confluent
	// message index at the start of the value is malformed or doesn't select
	// a message in the schema. The payload is not decoded.
	ErrMessageIndex = errors.New("invalid protobuf message index")

	// ErrSchemaCompile is returned by Parse when protocompile rejects the
	// schema source. The wrapped error carries the file position of the
	// first problem (see protocompile's reporter.ErrorWithPos).
	ErrSchemaCompile = errors.New("failed to compile protobuf schema")

	// ErrCompileTimeout is returned by Parse when the compile does not finish
	// within the compile timeout. The returned error also matches
	// context.DeadlineExceeded, which is how schema.Schema.Serde knows not to
	// cache it.
	ErrCompileTimeout = errors.New("protobuf schema compile timed out")

	// ErrUnresolvedImport is returned by Parse when the schema imports a
	// file that is neither a standard google/protobuf/*.proto import nor the
	// name of one of its references.
	ErrUnresolvedImport = errors.New("protobuf schema import is not a declared reference")

	// ErrReferenceResolve is returned by Parse when the ResolveFunc fails to
	// fetch a reference, or when the schema has references and no ResolveFunc
	// was given. It wraps the resolver's error. It says nothing about the
	// schema itself (the registry may be unreachable), so schema.Schema.Serde
	// does not cache it.
	ErrReferenceResolve = errors.New("failed to resolve protobuf schema reference")

	// ErrReferenceCycle is returned by Parse when a schema's references lead
	// back to a schema that is still being resolved (A -> B -> A).
	ErrReferenceCycle = errors.New("protobuf schema references form a cycle")

	// ErrReferenceLimit is returned by Parse when a reference chain is
	// deeper than MaxReferenceDepth, or a schema reaches more than
	// MaxReferences distinct schemas.
	ErrReferenceLimit = errors.New("protobuf schema references exceed a limit")

	// ErrInvalidReference is returned by Parse for a reference that can't be
	// compiled as given: no name or subject, the name reserved for the
	// schema being parsed, or one import name used for two different
	// schemas.
	ErrInvalidReference = errors.New("invalid protobuf schema reference")

	// ErrSchemaTooLarge is returned by Parse when the schema source, plus
	// the source of every schema it references, exceeds the schema size cap.
	ErrSchemaTooLarge = errors.New("protobuf schema is too large")

	// ErrCanceled is returned by Parse when the caller's context ends before
	// the schema is resolved and compiled. The returned error also matches
	// the context's error (context.Canceled or context.DeadlineExceeded).
	ErrCanceled = errors.New("protobuf schema parse canceled")

	// ErrInvalidOption is returned by Parse (for a rejected Option, such as
	// WithCompileTimeout(0)) and by SetDefaultCompileTimeout and
	// SetDefaultMaxSchemaSize when given a value they reject. The compile
	// timeout and the schema size cap must be > 0.
	ErrInvalidOption = errors.New("invalid protobuf serde option")
)
