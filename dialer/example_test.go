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

package dialer_test

import (
	"context"
	"fmt"
	"net"

	"github.com/conduitio/conduit-commons/dialer"
)

// The host puts a dialer on the context it passes to a connector.
func ExampleWithDialContext() {
	logging := func(ctx context.Context, network, address string) (net.Conn, error) {
		fmt.Println("dialing", network, address)
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}
	ctx := dialer.WithDialContext(context.Background(), logging)

	dial, ok := dialer.FromContext(ctx)
	fmt.Println(ok, dial != nil)
	// Output: true true
}

// A connector installs the host's dialer on the client it builds, and keeps its
// library default when the host supplied none.
func ExampleFromContext() {
	type clientConfig struct {
		DialContext func(ctx context.Context, network, address string) (net.Conn, error)
	}

	build := func(ctx context.Context) clientConfig {
		var cfg clientConfig // library defaults
		if dial, ok := dialer.FromContext(ctx); ok {
			cfg.DialContext = dial
		}
		return cfg
	}

	refuse := func(context.Context, string, string) (net.Conn, error) {
		return nil, fmt.Errorf("dialing is not permitted")
	}

	fmt.Println(build(context.Background()).DialContext == nil)
	fmt.Println(build(dialer.WithDialContext(context.Background(), refuse)).DialContext == nil)
	// Output:
	// true
	// false
}
