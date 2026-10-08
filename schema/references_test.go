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

package schema

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	schemav1 "github.com/conduitio/conduit-commons/proto/schema/v1"
	"github.com/conduitio/conduit-commons/schema/protobuf"
	"github.com/matryer/is"
)

// registerFakeFactory installs a Parse function for an unused Type for the
// duration of the test.
func registerFakeFactory(t *testing.T, typ Type, parse func(context.Context, Schema, Resolver) (Serde, error)) {
	t.Helper()
	if _, ok := KnownSerdeFactories[typ]; ok {
		t.Fatalf("type %d is already registered", typ)
	}
	KnownSerdeFactories[typ] = SerdeFactory{
		Parse:        parse,
		SerdeForType: func(any) (Serde, error) { return nil, errors.New("not used") },
	}
	t.Cleanup(func() { delete(KnownSerdeFactories, typ) })
}

// uniqueBytes returns text no other test, or other run of this test
// (-count), shares, so the process-wide Serde cache can't hand one test
// another's result.
func uniqueBytes(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("%s %d", t.Name(), time.Now().UnixNano())
}

// resolverFunc adapts a function to Resolver.
type resolverFunc func(ctx context.Context, ref Reference) (Schema, error)

func (f resolverFunc) ResolveReference(ctx context.Context, ref Reference) (Schema, error) {
	return f(ctx, ref)
}

// mapResolver resolves from a fixed set of schemas and counts calls.
type mapResolver struct {
	schemas map[Reference]Schema
	calls   atomic.Int64
}

func (r *mapResolver) ResolveReference(_ context.Context, ref Reference) (Schema, error) {
	r.calls.Add(1)
	s, ok := r.schemas[Reference{Subject: ref.Subject, Version: ref.Version}]
	if !ok {
		return Schema{}, fmt.Errorf("subject %s version %d not found", ref.Subject, ref.Version)
	}
	return s, nil
}

func TestSchema_SerdeWithResolver_ProtobufReferences(t *testing.T) {
	is := is.New(t)

	ref := Reference{Name: "example/v1/customer.proto", Subject: "customer", Version: 2}
	resolver := &mapResolver{schemas: map[Reference]Schema{
		{Subject: "customer", Version: 2}: {
			Subject: "customer", Version: 2, Type: TypeProtobuf,
			Bytes: []byte(`syntax = "proto3"; package example.v1; message Customer { string id = 1; }`),
		},
	}}
	s := Schema{
		Subject: "orders-value", Version: 1, ID: 11, Type: TypeProtobuf,
		Bytes: []byte(`syntax = "proto3";
package example.v1;
import "example/v1/customer.proto";
// ` + uniqueBytes(t) + `
message Order { example.v1.Customer customer = 1; }
`),
		References: []Reference{ref},
	}

	// Without a resolver the references can't be resolved. That failure
	// is not cached: it says nothing about the schema.
	_, err := s.Serde()
	is.True(errors.Is(err, protobuf.ErrReferenceResolve))

	srd, err := s.SerdeWithResolver(t.Context(), resolver)
	is.NoErr(err)
	is.Equal(srd.String(), string(s.Bytes))
	is.Equal(resolver.calls.Load(), int64(1))

	// Cached: no second resolution.
	_, err = s.SerdeWithResolver(t.Context(), resolver)
	is.NoErr(err)
	is.Equal(resolver.calls.Load(), int64(1))
}

// The adversarial A -> B -> A fixture, through the schema package.
func TestSchema_SerdeWithResolver_ProtobufReferenceCycle(t *testing.T) {
	is := is.New(t)

	a := Reference{Name: "a.proto", Subject: "a", Version: 1}
	b := Reference{Name: "b.proto", Subject: "b", Version: 1}
	resolver := &mapResolver{schemas: map[Reference]Schema{
		{Subject: "a", Version: 1}: {Type: TypeProtobuf, Bytes: []byte(`syntax = "proto3"; import "b.proto";`), References: []Reference{b}},
		{Subject: "b", Version: 1}: {Type: TypeProtobuf, Bytes: []byte(`syntax = "proto3"; import "a.proto";`), References: []Reference{a}},
	}}
	s := Schema{Type: TypeProtobuf, Bytes: []byte(`syntax = "proto3"; import "a.proto"; // ` + uniqueBytes(t)), References: []Reference{a}}

	for range 2 {
		_, err := s.SerdeWithResolver(t.Context(), resolver)
		is.True(errors.Is(err, protobuf.ErrReferenceCycle))
	}
	// A cycle is a property of the schemas, so it is cached like any parse
	// error: two fetches for the first call, none for the second.
	is.Equal(resolver.calls.Load(), int64(2))
}

// Identical text with different references is a different schema: the
// references are part of the cache key.
func TestSchema_Serde_SameBytesDifferentReferences(t *testing.T) {
	is := is.New(t)

	var calls atomic.Int64
	typ := Type(1010)
	registerFakeFactory(t, typ, func(_ context.Context, s Schema, _ Resolver) (Serde, error) {
		calls.Add(1)
		return fakeSerde{name: referencesKey(s.References)}, nil
	})
	b := []byte(uniqueBytes(t))
	v1 := []Reference{{Name: "dep.proto", Subject: "dep", Version: 1}}
	v2 := []Reference{{Name: "dep.proto", Subject: "dep", Version: 2}}

	srd1, err := Schema{Type: typ, Bytes: b, References: v1}.Serde()
	is.NoErr(err)
	srd2, err := Schema{Type: typ, Bytes: b, References: v2}.Serde()
	is.NoErr(err)
	srd0, err := Schema{Type: typ, Bytes: b}.Serde()
	is.NoErr(err)

	is.Equal(srd1, fakeSerde{name: referencesKey(v1)})
	is.Equal(srd2, fakeSerde{name: referencesKey(v2)})
	is.Equal(srd0, fakeSerde{name: ""})
	is.Equal(calls.Load(), int64(3))
}

func TestReferencesKey_Unambiguous(t *testing.T) {
	is := is.New(t)
	// Without length prefixes these two would encode the same.
	a := referencesKey([]Reference{{Name: "a", Subject: "b1", Version: 2}})
	b := referencesKey([]Reference{{Name: "a", Subject: "b", Version: 12}})
	is.True(a != b)
	is.Equal(referencesKey(nil), "")
}

func TestSchema_Serde_ReferenceResolveErrorIsNotCached(t *testing.T) {
	is := is.New(t)

	var fail atomic.Bool
	fail.Store(true)
	resolver := resolverFunc(func(context.Context, Reference) (Schema, error) {
		if fail.Load() {
			return Schema{}, errors.New("registry unavailable")
		}
		return Schema{Type: TypeProtobuf, Bytes: []byte(`syntax = "proto3"; package dep; message Dep {}`)}, nil
	})
	s := Schema{
		Type:       TypeProtobuf,
		Bytes:      []byte(`syntax = "proto3"; import "dep.proto"; message M { dep.Dep d = 1; } // ` + uniqueBytes(t)),
		References: []Reference{{Name: "dep.proto", Subject: "dep", Version: 1}},
	}

	_, err := s.SerdeWithResolver(t.Context(), resolver)
	is.True(errors.Is(err, protobuf.ErrReferenceResolve))

	fail.Store(false) // the registry is back
	_, err = s.SerdeWithResolver(t.Context(), resolver)
	is.NoErr(err)
}

func TestSchema_SerdeWithResolver_CallerCancels(t *testing.T) {
	is := is.New(t)

	var calls atomic.Int64
	typ := Type(1011)
	registerFakeFactory(t, typ, func(ctx context.Context, _ Schema, _ Resolver) (Serde, error) {
		calls.Add(1)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	s := Schema{Type: typ, Bytes: []byte(uniqueBytes(t))}

	ctx, cancel := context.WithCancel(t.Context())
	time.AfterFunc(20*time.Millisecond, cancel)
	_, err := s.SerdeWithResolver(ctx, nil)
	is.True(errors.Is(err, context.Canceled))
	is.Equal(calls.Load(), int64(1)) // our own parse: no retry

	// Not cached: the next caller parses again.
	ctx2, cancel2 := context.WithCancel(t.Context())
	cancel2()
	_, err = s.SerdeWithResolver(ctx2, nil)
	is.True(errors.Is(err, context.Canceled))
	is.Equal(calls.Load(), int64(2))
}

// Two pipelines need the same schema at once. The first one's parse is
// shared; when that pipeline stops (cancels its ctx) mid-parse, the second
// must not fail with the first one's cancellation.
func TestSchema_SerdeWithResolver_JoinedCallerSurvivesOthersCancel(t *testing.T) {
	is := is.New(t)

	started := make(chan struct{})
	var calls atomic.Int64
	typ := Type(1012)
	registerFakeFactory(t, typ, func(ctx context.Context, _ Schema, _ Resolver) (Serde, error) {
		if calls.Add(1) == 1 {
			close(started)
			<-ctx.Done() // the first parse only ends when its caller cancels
			return nil, fmt.Errorf("parse: %w", ctx.Err())
		}
		return fakeSerde{name: "ok"}, nil
	})
	s := Schema{Type: typ, Bytes: []byte(uniqueBytes(t))}

	ctxA, cancelA := context.WithCancel(t.Context())
	errA := make(chan error, 1)
	go func() {
		_, err := s.SerdeWithResolver(ctxA, nil)
		errA <- err
	}()
	<-started

	type result struct {
		srd Serde
		err error
	}
	resB := make(chan result, 1)
	go func() {
		srd, err := s.SerdeWithResolver(t.Context(), nil)
		resB <- result{srd, err}
	}()
	time.Sleep(50 * time.Millisecond) // let B join A's in-flight parse
	cancelA()

	is.True(errors.Is(<-errA, context.Canceled))
	b := <-resB
	is.NoErr(b.err)
	is.Equal(b.srd, fakeSerde{name: "ok"})
	is.Equal(calls.Load(), int64(2))
}

// References survive the wire type both ways, so a standalone processor gets
// the same schema the host resolved.
func TestSchema_ProtoRoundTripsReferences(t *testing.T) {
	is := is.New(t)
	in := Schema{
		Subject: "orders-value", Version: 3, ID: 42, Type: TypeProtobuf, Bytes: []byte("x"),
		References: []Reference{
			{Name: "example/v1/customer.proto", Subject: "customer", Version: 2},
			{Name: "example/v1/address.proto", Subject: "address", Version: 1},
		},
	}
	var p schemav1.Schema
	is.NoErr(in.ToProto(&p))
	is.Equal(len(p.References), 2)
	is.Equal(p.References[0].GetSubject(), "customer")

	var out Schema
	is.NoErr(out.FromProto(&p))
	is.Equal(out, in)
}

// A receiver reused for a schema without references must not keep the
// previous schema's references, and a proto reused likewise.
func TestSchema_ProtoClearsStaleReferences(t *testing.T) {
	is := is.New(t)
	s := Schema{References: []Reference{{Name: "a.proto", Subject: "a", Version: 1}}}
	is.NoErr(s.FromProto(&schemav1.Schema{Type: schemav1.Schema_TYPE_PROTOBUF, Bytes: []byte("x")}))
	is.Equal(s.References, nil)

	p := schemav1.Schema{References: []*schemav1.Schema_Reference{{Name: "a.proto"}}}
	src := Schema{Type: TypeAvro}
	is.NoErr(src.ToProto(&p))
	is.Equal(len(p.References), 0)
}

func TestSchema_Avro_IgnoresResolver(t *testing.T) {
	is := is.New(t)
	s := Schema{
		Type:       TypeAvro,
		Bytes:      []byte(`{"type":"record","name":"R` + fmt.Sprint(time.Now().UnixNano()) + `","fields":[{"name":"a","type":"int"}]}`),
		References: []Reference{{Name: "x", Subject: "x", Version: 1}},
	}
	resolver := &mapResolver{}
	_, err := s.SerdeWithResolver(t.Context(), resolver)
	is.NoErr(err)
	is.Equal(resolver.calls.Load(), int64(0))
}
