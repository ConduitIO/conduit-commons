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
	"fmt"
)

const (
	// MaxReferenceDepth is the longest chain of references Parse follows
	// (the schema's own references are depth 1). A longer chain fails with
	// ErrReferenceLimit.
	MaxReferenceDepth = 32

	// MaxReferences is the most distinct schemas (subject and version) Parse
	// resolves for one schema. Each one is a resolver call, usually a
	// registry request. More fail with ErrReferenceLimit.
	MaxReferences = 256
)

// Reference is a schema's reference to another registered schema, as
// Confluent Schema Registry stores it.
type Reference struct {
	// Name is the path the referencing schema imports the referenced one
	// by.
	Name string
	// Subject and Version identify the referenced schema in the registry.
	Subject string
	Version int
}

// ReferencedSchema is a schema returned by a ResolveFunc.
type ReferencedSchema struct {
	// Text is the .proto source of the referenced schema.
	Text []byte
	// References are the referenced schema's own references, resolved in
	// turn.
	References []Reference
}

// ResolveFunc fetches the schema a Reference points to, usually from the
// schema registry. It must honor ctx. Its errors are wrapped in
// ErrReferenceResolve.
type ResolveFunc func(ctx context.Context, ref Reference) (ReferencedSchema, error)

type refKey struct {
	subject string
	version int
}

// referenceWalk resolves a schema's references, and theirs, into the import
// path -> source map the compiler reads from.
//
// The walk is depth-first. A reference is resolved once per subject and
// version, however many schemas import it. A reference back to a schema that
// is still being resolved is a cycle: the registry fetches would never end,
// and protocompile can't see it because it only sees import paths, not
// subjects. MaxReferenceDepth, MaxReferences and the schema size cap bound the
// rest.
type referenceWalk struct {
	resolve ResolveFunc
	maxSize int

	size     int               // bytes of source collected so far, root included
	files    map[string][]byte // import path -> source
	fileKey  map[string]refKey // import path -> the schema it was resolved from
	resolved map[refKey]ReferencedSchema
	visiting map[refKey]bool // on the current path, for cycle detection
	path     []string        // the current path, for error messages
}

// resolveReferences returns the source of every schema refs reaches, keyed by
// import path. rootSize is the size of the referencing schema's own source,
// which counts toward maxSize.
func resolveReferences(ctx context.Context, refs []Reference, resolve ResolveFunc, rootSize, maxSize int) (map[string][]byte, error) {
	if len(refs) > 0 && resolve == nil {
		return nil, fmt.Errorf("%w: the schema has %d references but no resolver was given", ErrReferenceResolve, len(refs))
	}
	w := &referenceWalk{
		resolve:  resolve,
		maxSize:  maxSize,
		size:     rootSize,
		files:    make(map[string][]byte),
		fileKey:  make(map[string]refKey),
		resolved: make(map[refKey]ReferencedSchema),
		visiting: make(map[refKey]bool),
	}
	for _, ref := range refs {
		if err := w.visit(ctx, ref, 1); err != nil {
			return nil, err
		}
	}
	return w.files, nil
}

func (w *referenceWalk) visit(ctx context.Context, ref Reference, depth int) error {
	key := refKey{subject: ref.Subject, version: ref.Version}
	switch {
	case ref.Name == "" || ref.Subject == "":
		return fmt.Errorf("%w: %+v needs a name and a subject", ErrInvalidReference, ref)
	case ref.Name == schemaFileName:
		return fmt.Errorf("%w: name %q is reserved for the schema being parsed", ErrInvalidReference, ref.Name)
	case w.visiting[key]:
		return fmt.Errorf("%w: %s -> %s", ErrReferenceCycle, w.pathString(), refString(ref))
	case depth > MaxReferenceDepth:
		return fmt.Errorf("%w: reference chain %s -> %s is deeper than %d", ErrReferenceLimit, w.pathString(), refString(ref), MaxReferenceDepth)
	}
	if prev, ok := w.fileKey[ref.Name]; ok {
		if prev != key {
			return fmt.Errorf("%w: import %q refers to both %s version %d and %s version %d",
				ErrInvalidReference, ref.Name, prev.subject, prev.version, ref.Subject, ref.Version)
		}
		return nil // same file, already collected with its references
	}

	rs, ok := w.resolved[key]
	if !ok {
		if len(w.resolved) >= MaxReferences {
			return fmt.Errorf("%w: more than %d referenced schemas", ErrReferenceLimit, MaxReferences)
		}
		var err error
		rs, err = w.fetch(ctx, ref)
		if err != nil {
			return err
		}
		w.resolved[key] = rs
	}

	// The same schema imported under a second name is compiled a second
	// time, so it counts toward the size cap again.
	w.size += len(rs.Text)
	if w.size > w.maxSize {
		return fmt.Errorf("%w: the schema and its references exceed %d bytes (at %s)", ErrSchemaTooLarge, w.maxSize, refString(ref))
	}
	w.files[ref.Name] = rs.Text
	w.fileKey[ref.Name] = key

	w.visiting[key] = true
	w.path = append(w.path, refString(ref))
	for _, child := range rs.References {
		if err := w.visit(ctx, child, depth+1); err != nil {
			return err
		}
	}
	w.path = w.path[:len(w.path)-1]
	delete(w.visiting, key)
	return nil
}

// fetch calls the resolver, and copies what it returns: on timeout the
// compile may still be reading the source after Parse returns.
func (w *referenceWalk) fetch(ctx context.Context, ref Reference) (ReferencedSchema, error) {
	if err := ctx.Err(); err != nil {
		return ReferencedSchema{}, err
	}
	rs, err := w.resolve(ctx, ref)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ReferencedSchema{}, ctxErr // classified as ErrCanceled by Parse
		}
		return ReferencedSchema{}, fmt.Errorf("%w: %s: %w", ErrReferenceResolve, refString(ref), err)
	}
	return ReferencedSchema{
		Text:       bytes.Clone(rs.Text),
		References: append([]Reference(nil), rs.References...),
	}, nil
}

func (w *referenceWalk) pathString() string {
	out := "schema"
	for _, p := range w.path {
		out += " -> " + p
	}
	return out
}

func refString(ref Reference) string {
	return fmt.Sprintf("%q (%s version %d)", ref.Name, ref.Subject, ref.Version)
}
