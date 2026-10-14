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
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// A transient parse failure drops its cache entry. With the third-party cache
// this package used before, dropping it with Delete while other callers
// waited on the same load made those callers return (nil, nil), which
// panics in Schema.Unmarshal.
// Concurrent callers, flaky parses and fresh schema bytes every few calls
// keep entries being loaded and evicted at the same time.
func TestSchema_Serde_TransientErrorNeverYieldsNilSerde(t *testing.T) {
	typ, _ := registerFakeTypeAs(t, Type(1901), func(call int64) (Serde, error) {
		time.Sleep(time.Duration(call%3) * 20 * time.Microsecond)
		if call%2 == 0 {
			return nil, fmt.Errorf("flaky: %w", context.DeadlineExceeded)
		}
		return fakeSerde{name: "ok"}, nil
	})

	var nilnil, errs, oks atomic.Int64
	var wg sync.WaitGroup
	deadline := time.Now().Add(2 * time.Second)
	for g := range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; time.Now().Before(deadline); i++ {
				s := Schema{Type: typ, Bytes: []byte(fmt.Sprintf("race-%d-%d", g%4, i/8))}
				srd, err := s.Serde()
				switch {
				case srd == nil && err == nil:
					nilnil.Add(1)
				case err != nil:
					errs.Add(1)
				default:
					oks.Add(1)
				}
			}
		}()
	}
	wg.Wait()
	t.Logf("ok=%d err=%d nilnil=%d", oks.Load(), errs.Load(), nilnil.Load())
	if n := nilnil.Load(); n > 0 {
		t.Fatalf("Serde returned (nil, nil) %d times", n)
	}
}
