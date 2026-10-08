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

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/bufbuild/protocompile"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// schemaFileName is the name the schema source is compiled under. Confluent
// stores a schema without a file name, so any fixed name works; it only
// appears in compile error positions. A schema that imports this exact name
// imports itself, which protocompile reports as an import cycle.
const schemaFileName = "schema.proto"

// standardImports resolves the google/protobuf/*.proto files that ship with
// protoc. They are not registry references: Confluent treats them as
// built in, and schemas use them without declaring a reference.
var standardImports = protocompile.WithStandardImports(
	protocompile.ResolverFunc(func(string) (protocompile.SearchResult, error) {
		return protocompile.SearchResult{}, fs.ErrNotExist
	}),
)

// Serde is a compiled Protobuf schema. It is immutable after Parse and safe
// for concurrent use; schema.Schema.Serde shares one instance across every
// pipeline that uses the same schema.
type Serde struct {
	text []byte
	file protoreflect.FileDescriptor

	// maxIndexDepth is the deepest message nesting in file, which bounds
	// the length of a valid message index (see messageForIndex).
	maxIndexDepth int
}

// Parse compiles .proto source text into a Serde. The source must be a single
// self-contained file: it may import the standard google/protobuf/*.proto
// files, but any other import fails with ErrReferencesNotSupported.
//
// The compile is bounded by a timeout (see the package doc). Every returned
// error matches one of ErrInvalidOption, ErrCompileTimeout,
// ErrReferencesNotSupported or ErrSchemaCompile. Parse does not retain text;
// it works on a copy.
func Parse(text []byte, opts ...Option) (*Serde, error) {
	o, err := resolveOptions(opts)
	if err != nil {
		return nil, err
	}

	// Copy the source: on timeout, protocompile may still be reading it in a
	// background goroutine after Parse returns, and the caller owns text.
	src := bytes.Clone(text)

	ctx, cancel := context.WithTimeout(context.Background(), o.compileTimeout)
	defer cancel()

	compiler := protocompile.Compiler{
		Resolver: protocompile.ResolverFunc(func(path string) (protocompile.SearchResult, error) {
			if path == schemaFileName {
				if o.sourceHook != nil {
					o.sourceHook()
				}
				return protocompile.SearchResult{Source: bytes.NewReader(src)}, nil
			}
			if res, err := standardImports.FindFileByPath(path); err == nil {
				return res, nil
			}
			return protocompile.SearchResult{}, fmt.Errorf("%w: import %q", ErrReferencesNotSupported, path)
		}),
		// One file plus at most a few standard imports; parallelism buys
		// nothing and would let one schema occupy every core.
		MaxParallelism: 1,
	}

	files, err := compiler.Compile(ctx, schemaFileName)
	if err != nil {
		return nil, classifyCompileError(ctx, o, err)
	}
	if len(files) != 1 || files[0] == nil {
		// Not expected from protocompile for a successful single-file
		// compile; fail closed rather than return a Serde with no schema.
		return nil, fmt.Errorf("%w: compiler returned %d files for one input", ErrSchemaCompile, len(files))
	}

	return &Serde{
		text:          src,
		file:          files[0],
		maxIndexDepth: messageNestingDepth(files[0].Messages()),
	}, nil
}

// classifyCompileError maps a protocompile error onto exactly one sentinel.
func classifyCompileError(ctx context.Context, o options, err error) error {
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		// Checked first: once the deadline passes, protocompile returns the
		// bare context error, and whatever else failed is moot.
		return fmt.Errorf("%w after %s: %w", ErrCompileTimeout, o.compileTimeout, context.DeadlineExceeded)
	case errors.Is(err, ErrReferencesNotSupported):
		return err
	default:
		return fmt.Errorf("%w: %w", ErrSchemaCompile, err)
	}
}

// Marshal always fails with ErrEncodingNotSupported: Protobuf support is
// decode-only.
func (s *Serde) Marshal(any) ([]byte, error) {
	return nil, ErrEncodingNotSupported
}

// Unmarshal decodes a Confluent Protobuf value. b is the value after the
// 5-byte Confluent header (magic byte and schema ID), which the caller strips
// to find the schema: the message index, then the Protobuf payload.
//
// A message index that doesn't select a message in the schema fails with
// ErrMessageIndex. Until field mapping (PB-4) lands, a valid index then fails
// with ErrDecodeNotImplemented, and v is never written.
func (s *Serde) Unmarshal(b []byte, _ any) error {
	if _, _, err := s.messageForIndex(b); err != nil {
		return err
	}
	return ErrDecodeNotImplemented
}

// String returns the .proto source the Serde was compiled from, unchanged.
func (s *Serde) String() string {
	return string(s.text)
}
