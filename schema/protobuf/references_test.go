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
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/matryer/is"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const (
	// Imported by referenceSchema as example/v1/customer.proto.
	customerSchema = `syntax = "proto3";

package example.v1;

import "example/v1/address.proto";

message Customer {
  string id = 1;
  example.v1.Address address = 2;
}
`

	// Imported by customerSchema as example/v1/address.proto.
	addressSchema = `syntax = "proto3";

package example.v1;

message Address {
  string street = 1;
}
`
)

var (
	customerRef = Reference{Name: "example/v1/customer.proto", Subject: "customer", Version: 3}
	addressRef  = Reference{Name: "example/v1/address.proto", Subject: "address", Version: 1}
)

// fakeRegistry is a ResolveFunc over a fixed set of schemas that counts calls
// per subject and version.
type fakeRegistry struct {
	mu      sync.Mutex
	schemas map[refKey]ReferencedSchema
	calls   map[refKey]int
}

func newFakeRegistry() *fakeRegistry {
	return &fakeRegistry{schemas: map[refKey]ReferencedSchema{}, calls: map[refKey]int{}}
}

func (r *fakeRegistry) add(ref Reference, text string, refs ...Reference) *fakeRegistry {
	r.schemas[refKey{ref.Subject, ref.Version}] = ReferencedSchema{Text: []byte(text), References: refs}
	return r
}

func (r *fakeRegistry) resolve(_ context.Context, ref Reference) (ReferencedSchema, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := refKey{ref.Subject, ref.Version}
	r.calls[key]++
	rs, ok := r.schemas[key]
	if !ok {
		return ReferencedSchema{}, fmt.Errorf("subject %s version %d: %w", ref.Subject, ref.Version, errNotFound)
	}
	return rs, nil
}

func (r *fakeRegistry) totalCalls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, c := range r.calls {
		n += c
	}
	return n
}

var errNotFound = errors.New("not found")

func TestParse_References(t *testing.T) {
	is := is.New(t)
	reg := newFakeRegistry().
		add(customerRef, customerSchema, addressRef).
		add(addressRef, addressSchema)

	srd, err := Parse(t.Context(), []byte(referenceSchema), WithReferences([]Reference{customerRef}, reg.resolve))
	is.NoErr(err)
	is.Equal(srd.String(), referenceSchema)

	// The reference of the reference is linked too.
	customer := srd.file.Messages().ByName("Order").Fields().ByName("customer").Message()
	is.Equal(customer.FullName(), protoreflect.FullName("example.v1.Customer"))
	address := customer.Fields().ByName("address").Message()
	is.Equal(address.FullName(), protoreflect.FullName("example.v1.Address"))
	is.Equal(reg.totalCalls(), 2)
}

// A schema reached along two paths is fetched once and compiled once.
func TestParse_References_Diamond(t *testing.T) {
	is := is.New(t)
	root := `syntax = "proto3";
import "left.proto";
import "right.proto";
message Root { Left l = 1; Right r = 2; }
`
	common := Reference{Name: "common.proto", Subject: "common", Version: 1}
	left := Reference{Name: "left.proto", Subject: "left", Version: 1}
	right := Reference{Name: "right.proto", Subject: "right", Version: 1}
	reg := newFakeRegistry().
		add(left, `syntax = "proto3"; import "common.proto"; message Left { Common c = 1; }`, common).
		add(right, `syntax = "proto3"; import "common.proto"; message Right { Common c = 1; }`, common).
		add(common, `syntax = "proto3"; message Common { string id = 1; }`)

	_, err := Parse(t.Context(), []byte(root), WithReferences([]Reference{left, right}, reg.resolve))
	is.NoErr(err)
	is.Equal(reg.calls[refKey{"common", 1}], 1)
	is.Equal(reg.totalCalls(), 3)
}

// The adversarial case: subject-level cycles. protocompile only sees import
// paths, so without the walk's own guard this would fetch forever.
func TestParse_References_Cycle(t *testing.T) {
	a := Reference{Name: "a.proto", Subject: "a", Version: 1}
	b := Reference{Name: "b.proto", Subject: "b", Version: 1}
	// Same subject and version as a, under a different import path, so
	// protocompile's own path-based cycle check could never see it.
	aAlias := Reference{Name: "alias/a.proto", Subject: "a", Version: 1}

	testCases := []struct {
		name     string
		reg      *fakeRegistry
		refs     []Reference
		wantPath string
	}{{
		name: "A -> B -> A",
		reg: newFakeRegistry().
			add(a, `syntax = "proto3"; import "b.proto"; message A {}`, b).
			add(b, `syntax = "proto3"; import "a.proto"; message B {}`, a),
		refs:     []Reference{a},
		wantPath: `schema -> "a.proto" (a version 1) -> "b.proto" (b version 1) -> "a.proto" (a version 1)`,
	}, {
		name: "A -> B -> A under another name",
		reg: newFakeRegistry().
			add(a, `syntax = "proto3"; import "b.proto"; message A {}`, b).
			add(b, `syntax = "proto3"; import "alias/a.proto"; message B {}`, aAlias),
		refs:     []Reference{a},
		wantPath: `-> "alias/a.proto" (a version 1)`,
	}, {
		name:     "A -> A",
		reg:      newFakeRegistry().add(a, `syntax = "proto3"; import "a.proto"; message A {}`, a),
		refs:     []Reference{a},
		wantPath: `schema -> "a.proto" (a version 1) -> "a.proto" (a version 1)`,
	}}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			is := is.New(t)
			srd, err := Parse(t.Context(), []byte(`syntax = "proto3"; import "a.proto";`), WithReferences(tc.refs, tc.reg.resolve))
			is.True(srd == nil)
			is.Equal(matchingSentinels(err), []error{ErrReferenceCycle})
			if !strings.Contains(err.Error(), tc.wantPath) {
				t.Fatalf("error %q does not contain %q", err, tc.wantPath)
			}
			is.True(tc.reg.totalCalls() <= 2) // each schema fetched once, then the cycle is caught
		})
	}
}

// chainRegistry returns a registry where file i imports file i+1, n files in
// all, and the reference to the first.
func chainRegistry(n int) (*fakeRegistry, Reference) {
	reg := newFakeRegistry()
	ref := func(i int) Reference {
		return Reference{Name: fmt.Sprintf("chain/%d.proto", i), Subject: fmt.Sprintf("chain-%d", i), Version: 1}
	}
	for i := range n {
		if i == n-1 {
			reg.add(ref(i), fmt.Sprintf(`syntax = "proto3"; message M%d {}`, i))
			continue
		}
		reg.add(ref(i), fmt.Sprintf(`syntax = "proto3"; import %q; message M%d {}`, ref(i+1).Name, i), ref(i+1))
	}
	return reg, ref(0)
}

func TestParse_References_MaxDepth(t *testing.T) {
	root := []byte(`syntax = "proto3"; import "chain/0.proto";`)
	t.Run("at the limit", func(t *testing.T) {
		is := is.New(t)
		reg, first := chainRegistry(MaxReferenceDepth)
		_, err := Parse(t.Context(), root, WithReferences([]Reference{first}, reg.resolve))
		is.NoErr(err)
	})
	t.Run("one past the limit", func(t *testing.T) {
		is := is.New(t)
		reg, first := chainRegistry(MaxReferenceDepth + 1)
		_, err := Parse(t.Context(), root, WithReferences([]Reference{first}, reg.resolve))
		is.Equal(matchingSentinels(err), []error{ErrReferenceLimit})
		is.True(strings.Contains(err.Error(), "deeper than 32"))
		is.Equal(reg.totalCalls(), MaxReferenceDepth) // stops before fetching the next one
	})
}

func TestParse_References_MaxReferences(t *testing.T) {
	is := is.New(t)
	reg := newFakeRegistry()
	refs := make([]Reference, MaxReferences+1)
	for i := range refs {
		refs[i] = Reference{Name: fmt.Sprintf("r%d.proto", i), Subject: fmt.Sprintf("r%d", i), Version: 1}
		reg.add(refs[i], `syntax = "proto3";`)
	}

	_, err := Parse(t.Context(), []byte(`syntax = "proto3";`), WithReferences(refs, reg.resolve))
	is.Equal(matchingSentinels(err), []error{ErrReferenceLimit})
	is.True(strings.Contains(err.Error(), "more than 256"))
	is.Equal(reg.totalCalls(), MaxReferences)
}

func TestParse_SchemaSizeCap(t *testing.T) {
	t.Run("schema alone", func(t *testing.T) {
		is := is.New(t)
		_, err := Parse(t.Context(), []byte(flatSchema), WithMaxSchemaSize(len(flatSchema)-1))
		is.Equal(matchingSentinels(err), []error{ErrSchemaTooLarge})

		_, err = Parse(t.Context(), []byte(flatSchema), WithMaxSchemaSize(len(flatSchema)))
		is.NoErr(err)
	})
	t.Run("schema plus references", func(t *testing.T) {
		is := is.New(t)
		total := len(referenceSchema) + len(customerSchema) + len(addressSchema)
		reg := newFakeRegistry().
			add(customerRef, customerSchema, addressRef).
			add(addressRef, addressSchema)
		opt := WithReferences([]Reference{customerRef}, reg.resolve)

		_, err := Parse(t.Context(), []byte(referenceSchema), opt, WithMaxSchemaSize(total-1))
		is.Equal(matchingSentinels(err), []error{ErrSchemaTooLarge})
		is.True(strings.Contains(err.Error(), "example/v1/address.proto"))

		_, err = Parse(t.Context(), []byte(referenceSchema), opt, WithMaxSchemaSize(total))
		is.NoErr(err)
	})
	t.Run("process-wide default", func(t *testing.T) {
		is := is.New(t)
		t.Cleanup(func() { defaultMaxSchemaSize.Store(0) })

		is.Equal(currentDefaultMaxSchemaSize(), DefaultMaxSchemaSize)
		is.NoErr(SetDefaultMaxSchemaSize(10))
		_, err := Parse(t.Context(), []byte(flatSchema))
		is.Equal(matchingSentinels(err), []error{ErrSchemaTooLarge})

		// a per-call option overrides it
		_, err = Parse(t.Context(), []byte(flatSchema), WithMaxSchemaSize(DefaultMaxSchemaSize))
		is.NoErr(err)
	})
	t.Run("can't be disabled", func(t *testing.T) {
		for _, n := range []int{0, -1} {
			is := is.New(t)
			_, err := Parse(t.Context(), []byte(flatSchema), WithMaxSchemaSize(n))
			is.Equal(matchingSentinels(err), []error{ErrInvalidOption})
			is.True(errors.Is(SetDefaultMaxSchemaSize(n), ErrInvalidOption))
			is.Equal(currentDefaultMaxSchemaSize(), DefaultMaxSchemaSize)
		}
	})
}

func TestParse_References_Errors(t *testing.T) {
	testCases := []struct {
		name    string
		text    string
		refs    []Reference
		resolve ResolveFunc
		wantErr error
		wantMsg string
	}{{
		name:    "no resolver",
		text:    referenceSchema,
		refs:    []Reference{customerRef},
		wantErr: ErrReferenceResolve,
		wantMsg: "no resolver",
	}, {
		name:    "resolver fails",
		text:    referenceSchema,
		refs:    []Reference{customerRef},
		resolve: newFakeRegistry().resolve,
		wantErr: ErrReferenceResolve,
		wantMsg: `"example/v1/customer.proto" (customer version 3): subject customer version 3: not found`,
	}, {
		name:    "import not among the references",
		text:    referenceSchema,
		refs:    []Reference{addressRef},
		resolve: newFakeRegistry().add(addressRef, addressSchema).resolve,
		wantErr: ErrUnresolvedImport,
		wantMsg: `"example/v1/customer.proto"`,
	}, {
		name:    "reference without a name",
		text:    flatSchema,
		refs:    []Reference{{Subject: "s", Version: 1}},
		resolve: newFakeRegistry().resolve,
		wantErr: ErrInvalidReference,
	}, {
		name:    "reference without a subject",
		text:    flatSchema,
		refs:    []Reference{{Name: "a.proto", Version: 1}},
		resolve: newFakeRegistry().resolve,
		wantErr: ErrInvalidReference,
	}, {
		name:    "reference named like the schema",
		text:    flatSchema,
		refs:    []Reference{{Name: schemaFileName, Subject: "s", Version: 1}},
		resolve: newFakeRegistry().resolve,
		wantErr: ErrInvalidReference,
		wantMsg: "reserved",
	}, {
		name: "one name, two schemas",
		text: flatSchema,
		refs: []Reference{
			{Name: "a.proto", Subject: "a", Version: 1},
			{Name: "a.proto", Subject: "a", Version: 2},
		},
		resolve: newFakeRegistry().
			add(Reference{Subject: "a", Version: 1}, `syntax = "proto3";`).
			add(Reference{Subject: "a", Version: 2}, `syntax = "proto3";`).resolve,
		wantErr: ErrInvalidReference,
		wantMsg: `import "a.proto" refers to both a version 1 and a version 2`,
	}, {
		name:    "referenced schema does not compile",
		text:    referenceSchema,
		refs:    []Reference{customerRef},
		resolve: newFakeRegistry().add(customerRef, malformedSchema).resolve,
		wantErr: ErrSchemaCompile,
		wantMsg: "example/v1/customer.proto:7:",
	}}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			is := is.New(t)
			srd, err := Parse(t.Context(), []byte(tc.text), WithReferences(tc.refs, tc.resolve))
			is.True(srd == nil)
			is.Equal(matchingSentinels(err), []error{tc.wantErr})
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Fatalf("error %q does not contain %q", err, tc.wantMsg)
			}
		})
	}
}

func TestParse_References_ResolverErrorIsWrapped(t *testing.T) {
	is := is.New(t)
	_, err := Parse(t.Context(), []byte(referenceSchema), WithReferences([]Reference{customerRef}, newFakeRegistry().resolve))
	is.True(errors.Is(err, errNotFound))
}

func TestParse_References_DoesNotRetainResolvedText(t *testing.T) {
	is := is.New(t)
	text := []byte(customerSchema)
	resolve := func(_ context.Context, ref Reference) (ReferencedSchema, error) {
		if ref == customerRef {
			return ReferencedSchema{Text: text, References: []Reference{addressRef}}, nil
		}
		return ReferencedSchema{Text: []byte(addressSchema)}, nil
	}
	srd, err := Parse(t.Context(), []byte(referenceSchema), WithReferences([]Reference{customerRef}, resolve))
	is.NoErr(err)
	for i := range text {
		text[i] = 'x'
	}
	is.Equal(srd.file.Messages().ByName("Order").Fields().ByName("customer").Message().FullName(),
		protoreflect.FullName("example.v1.Customer"))
}

func TestParse_Canceled(t *testing.T) {
	t.Run("before Parse", func(t *testing.T) {
		is := is.New(t)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		reg := newFakeRegistry()

		_, err := Parse(ctx, []byte(referenceSchema), WithReferences([]Reference{customerRef}, reg.resolve))
		is.Equal(matchingSentinels(err), []error{ErrCanceled})
		is.True(errors.Is(err, context.Canceled))
		is.Equal(reg.totalCalls(), 0)
	})

	t.Run("during resolution", func(t *testing.T) {
		is := is.New(t)
		ctx, cancel := context.WithCancel(t.Context())
		resolve := func(ctx context.Context, _ Reference) (ReferencedSchema, error) {
			cancel()
			<-ctx.Done()
			return ReferencedSchema{}, fmt.Errorf("registry request: %w", ctx.Err())
		}

		_, err := Parse(ctx, []byte(referenceSchema), WithReferences([]Reference{customerRef}, resolve))
		// Not ErrReferenceResolve: the caller gave up, the registry didn't fail.
		is.Equal(matchingSentinels(err), []error{ErrCanceled})
		is.True(errors.Is(err, context.Canceled))
	})

	// The compile timeout is long; only the caller's ctx can end this
	// compile early.
	t.Run("during compile", func(t *testing.T) {
		is := is.New(t)
		ctx, cancel := context.WithCancel(t.Context())
		release := make(chan struct{})
		hook := func(o *options) error {
			o.sourceHook = func() {
				cancel()
				<-release
			}
			return nil
		}

		start := time.Now()
		_, err := Parse(ctx, []byte(flatSchema), WithCompileTimeout(time.Minute), hook)
		elapsed := time.Since(start)
		close(release) // let the orphaned compile goroutine finish

		is.Equal(matchingSentinels(err), []error{ErrCanceled})
		is.True(errors.Is(err, context.Canceled))
		is.True(!errors.Is(err, context.DeadlineExceeded))
		if elapsed > 10*time.Second {
			t.Fatalf("Parse returned after %s, want it to return on cancel", elapsed)
		}
	})

	t.Run("caller deadline is not the compile timeout", func(t *testing.T) {
		is := is.New(t)
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
		defer cancel()
		release := make(chan struct{})
		defer close(release)
		hook := func(o *options) error {
			o.sourceHook = func() { <-release }
			return nil
		}

		_, err := Parse(ctx, []byte(flatSchema), WithCompileTimeout(time.Minute), hook)
		is.Equal(matchingSentinels(err), []error{ErrCanceled})
		is.True(errors.Is(err, context.DeadlineExceeded))
	})
}

// FuzzParseReferences fuzzes the source of a referenced schema, which comes
// from the registry and is as untrusted as the schema itself. The root
// imports it as dep.proto. The seeds run in every `go test`; run the fuzzer
// with `go test -run '^$' -fuzz FuzzParseReferences ./schema/protobuf`.
//
// Invariants: Parse never panics, and returns either a Serde or an error
// matching exactly one sentinel, never both and never neither.
func FuzzParseReferences(f *testing.F) {
	for _, seed := range []string{
		`syntax = "proto3"; package dep; message Dep { string id = 1; }`,
		`syntax = "proto3"; package dep; import "dep.proto"; message Dep {}`,
		`syntax = "proto3"; package dep; import "schema.proto"; message Dep {}`,
		`syntax = "proto3"; package dep; import "other.proto"; message Dep {}`,
		`syntax = "proto3"; package dep; import "google/protobuf/any.proto"; message Dep { google.protobuf.Any a = 1; }`,
		`syntax = "proto3"; package root; message Root {}`, // duplicate symbol with the root
		malformedSchema,
		``,
	} {
		f.Add([]byte(seed))
	}

	root := []byte(`syntax = "proto3";
package root;
import "dep.proto";
message Root { dep.Dep dep = 1; }
`)
	ref := Reference{Name: "dep.proto", Subject: "dep", Version: 1}

	f.Fuzz(func(t *testing.T, dep []byte) {
		resolve := func(context.Context, Reference) (ReferencedSchema, error) {
			return ReferencedSchema{Text: dep}, nil
		}
		srd, err := Parse(t.Context(), root, WithReferences([]Reference{ref}, resolve))
		if err != nil {
			if srd != nil {
				t.Fatalf("Parse returned both a Serde and an error: %v", err)
			}
			if got := matchingSentinels(err); len(got) != 1 {
				t.Fatalf("error %q matches %d sentinels, want exactly 1", err, len(got))
			}
			return
		}
		if srd == nil || srd.file == nil {
			t.Fatal("Parse returned neither a Serde nor an error")
		}
	})
}
