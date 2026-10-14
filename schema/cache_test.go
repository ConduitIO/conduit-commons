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
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newTestCache() (*serdeCache, *fakeClock) {
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	return newSerdeCache(4*time.Hour, clk.now), clk
}

var testKey = serdeCacheKey{typ: Type(7), fingerprint: 1}

func TestSerdeCache_SingleFlight(t *testing.T) {
	c, _ := newTestCache()
	var loads atomic.Int64
	release := make(chan struct{})
	started := make(chan struct{})

	const callers = 32
	var wg sync.WaitGroup
	results := make([]Serde, callers)
	var ranCount atomic.Int64
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			srd, ran, err := c.get(t.Context(), testKey, func() (Serde, error) {
				loads.Add(1)
				close(started)
				<-release
				return fakeSerde{name: "x"}, nil
			})
			if err != nil {
				t.Errorf("get: %v", err)
			}
			if ran {
				ranCount.Add(1)
			}
			results[i] = srd
		}()
	}
	<-started
	time.Sleep(10 * time.Millisecond) // let the others queue; the assertions don't depend on it
	close(release)
	wg.Wait()

	if loads.Load() != 1 || ranCount.Load() != 1 {
		t.Fatalf("loads=%d ran=%d, want 1 and 1", loads.Load(), ranCount.Load())
	}
	for _, r := range results {
		if r != (fakeSerde{name: "x"}) {
			t.Fatalf("got %v", r)
		}
	}
}

func TestSerdeCache_ExpiresAfterMaxAge(t *testing.T) {
	c, clk := newTestCache()
	var loads int
	load := func() (Serde, error) { loads++; return fakeSerde{name: "x"}, nil }

	for range 3 {
		if _, _, err := c.get(t.Context(), testKey, load); err != nil {
			t.Fatal(err)
		}
	}
	if loads != 1 {
		t.Fatalf("loads = %d, want 1", loads)
	}
	clk.advance(4*time.Hour - time.Second)
	_, _, _ = c.get(t.Context(), testKey, load)
	if loads != 1 {
		t.Fatalf("loads = %d before expiry, want 1", loads)
	}
	clk.advance(2 * time.Second)
	_, _, _ = c.get(t.Context(), testKey, load)
	if loads != 2 {
		t.Fatalf("loads = %d after expiry, want 2", loads)
	}
}

func TestSerdeCache_DeterministicErrorIsCached(t *testing.T) {
	c, _ := newTestCache()
	var loads int
	want := errors.New("bad schema")
	for range 3 {
		_, _, err := c.get(t.Context(), testKey, func() (Serde, error) { loads++; return nil, want })
		if !errors.Is(err, want) {
			t.Fatalf("err = %v", err)
		}
	}
	if loads != 1 {
		t.Fatalf("loads = %d, want 1", loads)
	}
}

// A transient error reaches the callers that joined the load, and is not
// kept: the next call loads again.
func TestSerdeCache_TransientErrorIsNotKept(t *testing.T) {
	c, _ := newTestCache()
	var loads int
	srd, _, err := c.get(t.Context(), testKey, func() (Serde, error) {
		loads++
		return nil, fmt.Errorf("slow: %w", context.DeadlineExceeded)
	})
	if srd != nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("srd=%v err=%v", srd, err)
	}
	srd, _, err = c.get(t.Context(), testKey, func() (Serde, error) { loads++; return fakeSerde{name: "ok"}, nil })
	if err != nil || srd != (fakeSerde{name: "ok"}) || loads != 2 {
		t.Fatalf("srd=%v err=%v loads=%d", srd, err, loads)
	}
}

func TestSerdeCache_NilSerdeWithNilErrorIsAnErrorAndNotKept(t *testing.T) {
	c, _ := newTestCache()
	srd, _, err := c.get(t.Context(), testKey, func() (Serde, error) { return nil, nil }) //nolint:nilnil // the case under test
	if srd != nil || !errors.Is(err, errNilSerde) {
		t.Fatalf("srd=%v err=%v", srd, err)
	}
	srd, _, err = c.get(t.Context(), testKey, func() (Serde, error) { return fakeSerde{name: "ok"}, nil })
	if err != nil || srd == nil {
		t.Fatalf("srd=%v err=%v", srd, err)
	}
}

func TestSerdeCache_WaiterStopsWhenItsContextEnds(t *testing.T) {
	c, _ := newTestCache()
	started, release := make(chan struct{}), make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _, _ = c.get(t.Context(), testKey, func() (Serde, error) {
			close(started)
			<-release
			return fakeSerde{name: "x"}, nil
		})
	}()
	<-started

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	srd, ran, err := c.get(ctx, testKey, func() (Serde, error) { t.Error("must not load"); return nil, errors.New("unused") })
	if srd != nil || !errors.Is(err, context.Canceled) || ran {
		t.Fatalf("srd=%v err=%v ran=%v", srd, err, ran)
	}
	close(release)
	<-done
}

func TestSerdeCache_PanickingLoadWakesWaitersAndIsNotKept(t *testing.T) {
	c, _ := newTestCache()
	started, release := make(chan struct{}), make(chan struct{})
	panicked := make(chan any, 1)
	go func() {
		defer func() { panicked <- recover() }()
		_, _, _ = c.get(t.Context(), testKey, func() (Serde, error) {
			close(started)
			<-release
			panic("boom")
		})
	}()
	<-started
	waiter := make(chan error, 1)
	go func() {
		_, _, err := c.get(t.Context(), testKey, func() (Serde, error) { return fakeSerde{}, nil })
		waiter <- err
	}()
	time.Sleep(10 * time.Millisecond)
	close(release)

	if p := <-panicked; p != "boom" {
		t.Fatalf("recovered %v", p)
	}
	if err := <-waiter; err != nil && !errors.Is(err, errLoadPanicked) {
		t.Fatalf("waiter err = %v", err) // nil if it arrived after the drop and loaded itself
	}
	srd, _, err := c.get(t.Context(), testKey, func() (Serde, error) { return fakeSerde{name: "ok"}, nil })
	if err != nil || srd == nil {
		t.Fatalf("srd=%v err=%v", srd, err)
	}
}

func TestSerdeCache_SweepDropsExpiredEntries(t *testing.T) {
	c, clk := newTestCache()
	for i := range 10 {
		key := serdeCacheKey{typ: Type(7), fingerprint: uint64(i)}
		_, _, _ = c.get(t.Context(), key, func() (Serde, error) { return fakeSerde{}, nil })
	}
	clk.advance(5 * time.Hour)
	_, _, _ = c.get(t.Context(), testKey, func() (Serde, error) { return fakeSerde{}, nil })
	c.mu.Lock()
	n := len(c.entries)
	c.mu.Unlock()
	if n != 1 {
		t.Fatalf("%d entries after sweep, want 1", n)
	}
}

// Many goroutines hammer a few keys whose loads fail transiently half the
// time and take varying time, with keys changing so entries keep being
// created, joined and dropped. No call may return a nil Serde with a nil
// error, and every successful call must return the key's own Serde. Run with
// -race.
func TestSerdeCache_ConcurrentTransientFailuresNeverYieldNilSerde(t *testing.T) {
	c, _ := newTestCache()
	var calls, nilnil, wrong atomic.Int64
	deadline := time.Now().Add(2 * time.Second)
	var wg sync.WaitGroup
	for g := range 256 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; time.Now().Before(deadline); i++ {
				fp := uint64(g%4)<<32 | uint64(i/8)
				key := serdeCacheKey{typ: Type(7), fingerprint: fp}
				srd, _, err := c.get(t.Context(), key, func() (Serde, error) {
					n := calls.Add(1)
					time.Sleep(time.Duration(n%3) * 20 * time.Microsecond)
					if n%2 == 0 {
						return nil, fmt.Errorf("flaky: %w", context.DeadlineExceeded)
					}
					return fakeSerde{name: fmt.Sprint(fp)}, nil
				})
				switch {
				case srd == nil && err == nil:
					nilnil.Add(1)
				case err == nil && srd != (fakeSerde{name: fmt.Sprint(fp)}):
					wrong.Add(1)
				}
			}
		}()
	}
	wg.Wait()
	if nilnil.Load() != 0 || wrong.Load() != 0 {
		t.Fatalf("nilnil=%d wrong=%d over %d loads", nilnil.Load(), wrong.Load(), calls.Load())
	}
}
