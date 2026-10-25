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

// Package dialer lets the host that runs a connector supply the function the
// connector uses to open network connections.
//
// A connector that talks to a database, a message broker or an object store
// builds a client from a library, and every such library lets its user replace
// the function that opens the TCP connection. The host (the Conduit engine, a
// test harness, a sandbox) may need to decide how those connections are made:
// to route them through a particular network path, to record or limit them, or
// to refuse some destinations. This package is the seam: the host puts a
// [DialContextFunc] on the context it passes to the connector with
// [WithDialContext], and the connector reads it with [FromContext] when it
// builds its clients.
//
// # Contract for connectors
//
//   - Read the dialer from the context of the call that builds the client
//     (typically Open) and install it on every client the connector creates:
//     the data connections and also the admin, metadata, replication and
//     health-check connections.
//   - Capture the function when the client is built and keep it for the life of
//     the client. A context value is not available to a reconnect that runs on
//     a background goroutine, but the captured function is.
//   - When [FromContext] reports false, change nothing: keep the library's own
//     default dialer, so that behavior without a host-supplied dialer is exactly
//     what it was.
//
// # Invariants
//
//   - A nil [DialContextFunc] never travels on a context. [WithDialContext]
//     with nil returns its context unchanged, and [FromContext] returns a
//     non-nil function whenever it reports true.
//   - A connector parameter cannot set the dialer. Only code that builds the
//     context can call [WithDialContext]; connector settings never reach it.
//   - The function is called concurrently. Implementations must be safe for
//     concurrent use.
//
// Enforcement is cooperative: the connector chooses to honor the dialer for the
// clients it builds. The package is a hand-off point, not a sandbox.
package dialer
