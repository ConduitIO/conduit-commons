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
	"net"
)

// DialContextFunc opens a network connection to address on the named network
// ("tcp", "tcp4", "unix", ...), like [net.Dialer.DialContext]. It must be safe
// for concurrent use.
type DialContextFunc func(ctx context.Context, network, address string) (net.Conn, error)

type ctxKey struct{}

// WithDialContext returns a context that carries dial. A dialer already on ctx
// is replaced. A nil dial returns ctx unchanged.
func WithDialContext(ctx context.Context, dial DialContextFunc) context.Context {
	if dial == nil {
		return ctx
	}
	return context.WithValue(ctx, ctxKey{}, dial)
}

// FromContext returns the dialer the host put on ctx and true, or nil and false
// when it supplied none (a standalone plugin, a unit test, an older host). The
// caller keeps the client library's default dialer in that case.
//
// The returned function is meant to be captured when a client is built and used
// for every connection that client opens, reconnects included; it does not
// consult ctx again.
func FromContext(ctx context.Context) (DialContextFunc, bool) {
	dial, ok := ctx.Value(ctxKey{}).(DialContextFunc)
	return dial, ok && dial != nil
}
