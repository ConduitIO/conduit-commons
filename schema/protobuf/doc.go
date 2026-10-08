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

// Package protobuf implements schema support for Protobuf schemas as stored by
// Confluent Schema Registry, which keeps them as .proto source text rather
// than compiled descriptors.
//
// Parse compiles that source at runtime with github.com/bufbuild/protocompile
// into a Serde. The design is ConduitIO/conduit
// docs/design-documents/20260823-protobuf-schema-support.md; this package
// grows in the slices listed there:
//
//   - PB-1: Parse for a single .proto file, importing only the standard
//     google/protobuf/*.proto files.
//   - PB-2: Confluent message-index handling, selecting the message a payload
//     was encoded with.
//   - PB-3 (this state): schema-reference resolution through a caller-supplied
//     resolver, and a caller context that cancels the parse.
//   - PB-4: decoding a payload into structured data.
//
// Until PB-4 lands, Serde.Unmarshal resolves the message index and then
// returns ErrDecodeNotImplemented.
// Support is decode-only by design: Serde.Marshal, and the SerdeForType entry
// for this type in schema.KnownSerdeFactories, return ErrEncodingNotSupported,
// because a Protobuf schema (field numbers, message identity) can't be
// inferred from a Go value.
//
// # Compile timeout
//
// Schema text comes from the network, and a parsed Serde is cached
// process-wide, so every compile is bounded by a timeout. The timeout is
// always on: it defaults to DefaultCompileTimeout, can be changed per call
// with WithCompileTimeout or process-wide with SetDefaultCompileTimeout, and
// can't be disabled. A compile that runs out of time returns
// ErrCompileTimeout, which also matches context.DeadlineExceeded;
// schema.Schema.Serde does not cache that error, so the next record retries
// the compile.
//
// The timeout bounds how long Parse blocks, not how long protocompile works:
// protocompile stops scheduling work when its context ends, but a file
// already being parsed finishes in the background before its goroutine exits.
// The schema size cap bounds that orphaned work.
//
// # Schema size cap
//
// Parse rejects a schema whose source, plus the source of every schema it
// references, exceeds the cap with ErrSchemaTooLarge, before compiling. The
// cap defaults to DefaultMaxSchemaSize, can be changed per call with
// WithMaxSchemaSize or process-wide with SetDefaultMaxSchemaSize, and can't be
// disabled.
//
// # Schema references
//
// A Confluent Protobuf schema can import other registered schemas. Each
// import path is the Name of one of the schema's references, which names a
// subject and version in the registry. WithReferences gives Parse those
// references and a ResolveFunc that fetches them. Parse resolves them
// depth-first, with their own references, before compiling, and then
// compiles everything together. A reference reached twice is fetched once.
//
// The walk is guarded, because each step is a registry fetch and
// protocompile's own import-cycle check only sees import paths, not
// subjects: a reference back to a schema still being resolved fails with
// ErrReferenceCycle, a chain deeper than MaxReferenceDepth or more than
// MaxReferences distinct schemas fails with ErrReferenceLimit, and the size
// cap counts every referenced source. A resolver failure is
// ErrReferenceResolve; schema.Schema.Serde does not cache it.
//
// Resolution is bounded by the caller's context and the resolver, not by the
// compile timeout, which starts after it.
//
// # Message index
//
// A Confluent Protobuf value is the magic byte, the 4-byte schema ID, a
// message index, then the Protobuf payload. One .proto file can declare many
// messages; the index says which one the payload is. It is a zigzag varint
// count followed by that many zigzag varint indexes: the first selects a
// top-level message in declaration order, each further one a nested message
// of the previous. A count of 0 is a shortcut for [0], the first top-level
// message. Serde.Unmarshal takes the value after the schema ID, index first.
// An index that doesn't select a message fails with ErrMessageIndex; it never
// falls back to another message, because a payload decoded against the wrong
// descriptor can succeed and produce garbage.
//
// # Errors
//
// Every error Parse returns matches exactly one of ErrInvalidOption,
// ErrCanceled, ErrCompileTimeout, ErrSchemaTooLarge, ErrReferenceResolve,
// ErrReferenceCycle, ErrReferenceLimit, ErrInvalidReference,
// ErrUnresolvedImport or ErrSchemaCompile under errors.Is. Every error Serde.Unmarshal returns matches exactly one of
// ErrMessageIndex or ErrDecodeNotImplemented. These sentinels are the stable
// identities that callers (the conduit protobuf.decode processor) map to
// error codes.
package protobuf
