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

	"github.com/conduitio/conduit-commons/schema/protobuf"
	"github.com/matryer/is"
)

// Callers index KnownSerdeFactories and call SerdeForType/Parse directly
// (connector SDK source middleware, processor SDK middleware, conduit's Avro
// encoder), so a nil field is a nil-func panic waiting for the first caller
// that reaches it with that Type.
func TestKnownSerdeFactories_NoNilFuncs(t *testing.T) {
	for typ, f := range KnownSerdeFactories {
		t.Run(typ.String(), func(t *testing.T) {
			is := is.New(t)
			is.True(f.Parse != nil)
			is.True(f.SerdeForType != nil)
		})
	}
}

func TestKnownSerdeFactories_ProtobufSerdeForTypeReturnsCodedError(t *testing.T) {
	is := is.New(t)

	f, ok := KnownSerdeFactories[TypeProtobuf]
	is.True(ok)

	srd, err := f.SerdeForType(map[string]any{"id": "1"})
	is.True(srd == nil)
	is.True(errors.Is(err, protobuf.ErrEncodingNotSupported))
	is.Equal(err.Error(), "schema type protobuf: protobuf encoding is not supported")
}

func TestSchema_Protobuf(t *testing.T) {
	is := is.New(t)

	s := Schema{
		Subject: "orders-value",
		Version: 1,
		ID:      7,
		Type:    TypeProtobuf,
		Bytes: []byte(`syntax = "proto3";
package example.v1;
// ` + t.Name() + `
message Order { string id = 1; }
`),
	}

	srd, err := s.Serde()
	is.NoErr(err)
	is.Equal(srd.String(), string(s.Bytes))

	_, err = s.Marshal(map[string]any{"id": "1"})
	is.True(errors.Is(err, protobuf.ErrEncodingNotSupported))

	var v any
	err = s.Unmarshal([]byte{0x0a, 0x01, 0x31}, &v)
	is.True(errors.Is(err, protobuf.ErrDecodeNotImplemented))
}

func TestSchema_Protobuf_CompileErrorIsUnwrappable(t *testing.T) {
	is := is.New(t)

	s := Schema{Type: TypeProtobuf, Bytes: []byte("message { // " + t.Name())}
	_, err := s.Serde()
	is.True(errors.Is(err, protobuf.ErrSchemaCompile))
}

// registerFakeType installs a SerdeFactory for an unused Type for the
// duration of the test and returns the Type and a counter of Parse calls.
func registerFakeType(t *testing.T, parse func(call int64) (Serde, error)) (Type, *atomic.Int64) {
	t.Helper()
	typ := Type(1000)
	if _, ok := KnownSerdeFactories[typ]; ok {
		t.Fatalf("type %d is already registered", typ)
	}
	var calls atomic.Int64
	KnownSerdeFactories[typ] = SerdeFactory{
		Parse:        func([]byte) (Serde, error) { return parse(calls.Add(1)) },
		SerdeForType: func(any) (Serde, error) { return nil, errors.New("not used") },
	}
	t.Cleanup(func() { delete(KnownSerdeFactories, typ) })
	return typ, &calls
}

type fakeSerde struct{}

func (fakeSerde) Marshal(any) ([]byte, error) { return nil, nil }
func (fakeSerde) Unmarshal([]byte, any) error { return nil }
func (fakeSerde) String() string              { return "fake" }

// A timed-out parse must not be cached: the timeout says nothing about the
// schema, and caching it would fail every pipeline using the schema for the
// cache's MaxAge (4h).
func TestSchema_Serde_TimeoutIsNotCached(t *testing.T) {
	is := is.New(t)

	typ, calls := registerFakeType(t, func(call int64) (Serde, error) {
		if call == 1 {
			return nil, fmt.Errorf("%w: %w", protobuf.ErrCompileTimeout, context.DeadlineExceeded)
		}
		return fakeSerde{}, nil
	})
	s := Schema{Type: typ, Bytes: []byte(t.Name())}

	_, err := s.Serde()
	is.True(errors.Is(err, protobuf.ErrCompileTimeout))

	srd, err := s.Serde()
	is.NoErr(err)
	is.Equal(srd, fakeSerde{})
	is.Equal(calls.Load(), int64(2))

	// the success is cached as usual
	_, err = s.Serde()
	is.NoErr(err)
	is.Equal(calls.Load(), int64(2))
}

// Contrast: a deterministic parse error stays cached, as before this change.
func TestSchema_Serde_ParseErrorIsCached(t *testing.T) {
	is := is.New(t)

	wantErr := errors.New("bad schema")
	typ, calls := registerFakeType(t, func(int64) (Serde, error) { return nil, wantErr })
	s := Schema{Type: typ, Bytes: []byte(t.Name())}

	for range 3 {
		_, err := s.Serde()
		is.True(errors.Is(err, wantErr))
	}
	is.Equal(calls.Load(), int64(1))
}
