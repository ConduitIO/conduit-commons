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

package dialer

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/matryer/is"
)

var errRefused = errors.New("refused")

func refuse(context.Context, string, string) (net.Conn, error) { return nil, errRefused }

func TestFromContext_Absent(t *testing.T) {
	is := is.New(t)
	dial, ok := FromContext(context.Background())
	is.True(!ok)
	is.True(dial == nil)
}

func TestWithDialContext_NilIsNoOp(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	is.Equal(WithDialContext(ctx, nil), ctx)

	dial, ok := FromContext(WithDialContext(WithDialContext(ctx, refuse), nil))
	is.True(ok)
	is.True(dial != nil) // a nil never replaces a real dialer
}

func TestWithDialContext_RoundTrip(t *testing.T) {
	is := is.New(t)
	var gotNetwork, gotAddr string
	rec := func(_ context.Context, network, address string) (net.Conn, error) {
		gotNetwork, gotAddr = network, address
		return nil, errRefused
	}
	dial, ok := FromContext(WithDialContext(context.Background(), rec))
	is.True(ok)
	_, err := dial(context.Background(), "tcp", "db.internal:5432")
	is.True(errors.Is(err, errRefused))
	is.Equal(gotNetwork, "tcp")
	is.Equal(gotAddr, "db.internal:5432")
}

func TestWithDialContext_Inherited(t *testing.T) {
	is := is.New(t)
	parent := WithDialContext(context.Background(), refuse)
	child, cancel := context.WithCancel(parent)
	defer cancel()
	_, ok := FromContext(child)
	is.True(ok) // derived contexts keep the dialer
}

func TestWithDialContext_Replaced(t *testing.T) {
	is := is.New(t)
	var called string
	first := func(context.Context, string, string) (net.Conn, error) { called = "first"; return nil, errRefused }
	second := func(context.Context, string, string) (net.Conn, error) { called = "second"; return nil, errRefused }
	dial, _ := FromContext(WithDialContext(WithDialContext(context.Background(), first), second))
	_, _ = dial(context.Background(), "tcp", "x:1")
	is.Equal(called, "second")
}
