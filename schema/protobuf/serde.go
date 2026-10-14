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
	"strings"

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

// Parse compiles .proto source text into a Serde.
//
// The source may import the standard google/protobuf/*.proto files and the
// schemas given with WithReferences, which Parse resolves recursively before
// compiling (see the package doc). Any other import fails with
// ErrUnresolvedImport.
//
// ctx bounds the whole call: cancelling it stops reference resolution and the
// compile, and Parse returns ErrCanceled. Reference resolution is also bounded
// by the resolution timeout (DefaultResolveTimeout, WithResolveTimeout); the
// earlier of that and ctx's deadline ends it. Running out of the resolution
// timeout is ErrReferenceResolve. The compile is also bounded by the
// compile timeout, and the source plus all referenced sources by the schema
// size cap (see the package doc). Every returned error matches exactly one
// sentinel listed there. Parse does not retain text or the resolved sources;
// it works on copies.
func Parse(ctx context.Context, text []byte, opts ...Option) (*Serde, error) {
	o, err := resolveOptions(opts)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCanceled, err)
	}
	if len(text) > o.maxSchemaSize {
		return nil, fmt.Errorf("%w: the schema is %d bytes, the limit is %d", ErrSchemaTooLarge, len(text), o.maxSchemaSize)
	}

	// Copy the source: on timeout, protocompile may still be reading it in a
	// background goroutine after Parse returns, and the caller owns text.
	src := bytes.Clone(text)

	imports, err := resolveWithTimeout(ctx, o, len(src))
	if err != nil {
		return nil, err
	}

	compileCtx, cancel := context.WithTimeout(ctx, o.compileTimeout)
	defer cancel()

	compiler := protocompile.Compiler{
		Resolver: protocompile.ResolverFunc(func(path string) (protocompile.SearchResult, error) {
			if path == schemaFileName {
				if o.sourceHook != nil {
					o.sourceHook()
				}
				return protocompile.SearchResult{Source: bytes.NewReader(src)}, nil
			}
			// The built-in google/protobuf/*.proto files win over a declared
			// reference of the same name. A registered copy could redefine a
			// well-known type (a Timestamp whose seconds is a string), and
			// every schema that imports it would then decode against that
			// definition. The declared text is ignored.
			if res, err := standardImports.FindFileByPath(path); err == nil {
				return res, nil
			}
			if imported, ok := imports[path]; ok {
				return protocompile.SearchResult{Source: bytes.NewReader(imported)}, nil
			}
			return protocompile.SearchResult{}, fmt.Errorf("%w: import %q", ErrUnresolvedImport, path)
		}),
		// The size cap bounds the work; parallelism would only let one
		// schema occupy every core.
		MaxParallelism: 1,
	}

	files, err := compiler.Compile(compileCtx, schemaFileName)
	if err != nil {
		return nil, classifyCompileError(ctx, compileCtx, o, err)
	}
	if len(files) != 1 || files[0] == nil {
		// Not expected from protocompile for a successful single-file
		// compile; fail closed rather than return a Serde with no schema.
		return nil, fmt.Errorf("%w: compiler returned %d files for one input", ErrSchemaCompile, len(files))
	}
	if err := checkReservedPackages(files[0]); err != nil {
		return nil, err
	}

	return &Serde{
		text:          src,
		file:          files[0],
		maxIndexDepth: messageNestingDepth(files[0].Messages()),
	}, nil
}

// resolveWithTimeout resolves the schema's references. It runs before the
// compile timeout starts: it is network I/O bounded by ctx, the resolver and
// the resolution timeout (whichever ends first), not compile work.
func resolveWithTimeout(ctx context.Context, o options, rootSize int) (map[string][]byte, error) {
	resolveCtx := ctx
	if len(o.references) > 0 {
		// WithTimeout keeps the earlier of the caller's deadline and ours.
		var cancel context.CancelFunc
		resolveCtx, cancel = context.WithTimeout(ctx, o.resolveTimeout)
		defer cancel()
	}
	imports, err := resolveReferences(resolveCtx, o.references, o.resolve, rootSize, o.maxSchemaSize)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("%w: %w", ErrCanceled, ctxErr)
		}
		if resolveCtx.Err() != nil {
			// Only the resolution timeout ended: the registry is slow or
			// unreachable. Like any resolver failure, not a property of the
			// schema, so the caller must not keep it.
			return nil, fmt.Errorf("%w: resolving references timed out after %s: %w", ErrReferenceResolve, o.resolveTimeout, context.DeadlineExceeded)
		}
		return nil, err
	}
	return imports, nil
}

// classifyCompileError maps a protocompile error onto exactly one sentinel.
// ctx is the caller's context, compileCtx the one carrying the timeout.
func classifyCompileError(ctx, compileCtx context.Context, o options, err error) error {
	switch {
	// The context checks come first: once a context ends, protocompile
	// returns the bare context error, and whatever else failed is moot.
	case ctx.Err() != nil:
		return fmt.Errorf("%w: %w", ErrCanceled, ctx.Err())
	case errors.Is(compileCtx.Err(), context.DeadlineExceeded):
		return fmt.Errorf("%w after %s: %w", ErrCompileTimeout, o.compileTimeout, context.DeadlineExceeded)
	case errors.Is(err, ErrUnresolvedImport):
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

// checkReservedPackages rejects a schema that, or whose imports, declare
// anything named google.protobuf or under it outside the built-in files.
//
// Serving the built-in files for google/protobuf/* paths is not enough: a
// reference at another path, say evil.proto, can redefine a well-known type
// for every schema that imports it, and the root's google.protobuf.Timestamp
// resolves to it. The package declaration is only one way to get there. The
// full name google.protobuf.Timestamp is also reached by "package google;
// message protobuf { message Timestamp {...} }" or, with no package, by
// "message google { message protobuf {...} }". So for every file that is not
// built in, every declared name (messages, enums and their values, fields,
// oneofs, extensions, services and methods) is checked, not just the package.
func checkReservedPackages(root protoreflect.FileDescriptor) error {
	seen := map[string]bool{}
	var walk func(fd protoreflect.FileDescriptor) error
	walk = func(fd protoreflect.FileDescriptor) error {
		if seen[fd.Path()] {
			return nil
		}
		seen[fd.Path()] = true
		if !isBuiltIn(fd.Path()) {
			if name, ok := reservedName(fd); ok {
				return fmt.Errorf("%w: %q declares %s", ErrReservedPackage, fd.Path(), name)
			}
		}
		imports := fd.Imports()
		for i := range imports.Len() {
			if err := walk(imports.Get(i).FileDescriptor); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(root)
}

// reservedName returns the first name declared in fd that is google.protobuf
// or under it, checking the package declaration and every declared name.
func reservedName(fd protoreflect.FileDescriptor) (string, bool) {
	if pkg := string(fd.Package()); isReservedName(pkg) {
		return "package " + pkg, true
	}
	for _, d := range declaredDescriptors(fd) {
		if name := string(d.FullName()); isReservedName(name) {
			return name, true
		}
	}
	return "", false
}

// declaredDescriptors returns every named declaration in fd: messages, their
// fields and oneofs, enums and their values, extensions, services and
// methods, nested ones included.
func declaredDescriptors(fd protoreflect.FileDescriptor) []protoreflect.Descriptor {
	var out []protoreflect.Descriptor
	appendEnums(&out, fd.Enums())
	appendExtensions(&out, fd.Extensions())
	appendMessages(&out, fd.Messages())
	svcs := fd.Services()
	for i := range svcs.Len() {
		out = append(out, svcs.Get(i))
		methods := svcs.Get(i).Methods()
		for j := range methods.Len() {
			out = append(out, methods.Get(j))
		}
	}
	return out
}

func appendMessages(out *[]protoreflect.Descriptor, mds protoreflect.MessageDescriptors) {
	for i := range mds.Len() {
		md := mds.Get(i)
		*out = append(*out, md)
		for j := range md.Fields().Len() {
			*out = append(*out, md.Fields().Get(j))
		}
		for j := range md.Oneofs().Len() {
			*out = append(*out, md.Oneofs().Get(j))
		}
		appendEnums(out, md.Enums())
		appendExtensions(out, md.Extensions())
		appendMessages(out, md.Messages())
	}
}

func appendEnums(out *[]protoreflect.Descriptor, eds protoreflect.EnumDescriptors) {
	for i := range eds.Len() {
		*out = append(*out, eds.Get(i))
		values := eds.Get(i).Values()
		for j := range values.Len() {
			*out = append(*out, values.Get(j))
		}
	}
}

func appendExtensions(out *[]protoreflect.Descriptor, xds protoreflect.ExtensionDescriptors) {
	for i := range xds.Len() {
		*out = append(*out, xds.Get(i))
	}
}

// isReservedName reports whether name is google.protobuf or under it. The
// check is on the dot boundary: google.protobufx is not reserved.
func isReservedName(name string) bool {
	return name == "google.protobuf" || strings.HasPrefix(name, "google.protobuf.")
}

// isBuiltIn reports whether path is one of the google/protobuf/*.proto files
// that ship with the compiler.
func isBuiltIn(path string) bool {
	_, err := standardImports.FindFileByPath(path)
	return err == nil
}
