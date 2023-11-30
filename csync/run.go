// Copyright © 2023 Meroxa, Inc.
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

package csync

import (
	"context"
	"time"

	"github.com/conduitio/conduit-commons/cchan"
)

// Run executes fn in a goroutine and waits for it to return. If the context
// gets canceled before that happens the method returns the context error.
//
// This is useful for executing long-running functions like sync.WaitGroup.Wait
// that don't take a context and can potentially block the execution forever.
func Run(ctx context.Context, fn func()) error {
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()

	_, _, err := cchan.ChanOut[struct{}](done).Recv(ctx)
	return err //nolint:wrapcheck // errors from cchan are context errors and we don't wrap them
}

// RunTimeout executes fn in a goroutine and waits for it to return. If the
// context gets canceled before that happens or the timeout is reached the
// method returns the context error.
func RunTimeout(ctx context.Context, fn func(), timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return Run(ctx, fn)
}
