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
//   - PB-1 (this state): Parse for a single, self-contained .proto file. The
//     standard google/protobuf/*.proto imports resolve; any other import is a
//     schema reference and fails with ErrReferencesNotSupported.
//   - PB-2: Confluent message-index handling, selecting the message a payload
//     was encoded with.
//   - PB-3: schema-reference resolution against the registry.
//   - PB-4: decoding a payload into structured data.
//
// Until PB-2 and PB-4 land, Serde.Unmarshal returns ErrDecodeNotImplemented.
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
//
// # Errors
//
// Every error Parse returns matches exactly one of ErrInvalidOption,
// ErrCompileTimeout, ErrReferencesNotSupported or ErrSchemaCompile under
// errors.Is. These sentinels are the stable identities that callers (the
// conduit protobuf.decode processor) map to error codes.
package protobuf
