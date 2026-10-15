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
	"encoding/binary"
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
	return newSerdeCache(4*time.Hour, 1<<20, clk.now), clk
}

// testCacheKey returns a distinct cache key per n. The cache treats keys as
// opaque, so tests do not need real digests.
func testCacheKey(n uint64) serdeCacheKey {
	var k serdeCacheKey
	binary.BigEndian.PutUint64(k[:], n)
	return k
}

var testKey = testCacheKey(1)

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
		key := testCacheKey(uint64(i))
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
				key := testCacheKey(fp)
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

func TestSerdeCache_SizeCapEvictsEarliestExpiringFinishedEntry(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	c := newSerdeCache(4*time.Hour, 3, clk.now)
	key := func(i int) serdeCacheKey { return testCacheKey(uint64(i)) }
	loads := map[int]int{}
	get := func(i int) {
		t.Helper()
		_, _, err := c.get(t.Context(), key(i), func() (Serde, error) { loads[i]++; return fakeSerde{}, nil })
		if err != nil {
			t.Fatal(err)
		}
	}

	for i := range 3 {
		get(i)
		clk.advance(time.Minute) // entry 0 expires first, then 1, then 2
	}
	get(3) // full: evicts 0
	if c.size() != 3 {
		t.Fatalf("size = %d, want 3", c.size())
	}
	get(1)
	get(2)
	get(3)
	if loads[1] != 1 || loads[2] != 1 || loads[3] != 1 {
		t.Fatalf("survivors were reloaded: %v", loads)
	}
	get(0)
	if loads[0] != 2 {
		t.Fatalf("entry 0 was not evicted: %v", loads)
	}
}

// A load in flight is never evicted: removing it would let a second load of
// the same key start. With every entry in flight, a new key loads without
// being cached, and the cache does not grow.
func TestSerdeCache_SizeCapNeverEvictsInFlightLoad(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	c := newSerdeCache(4*time.Hour, 2, clk.now)
	key := func(i int) serdeCacheKey { return testCacheKey(uint64(i)) }
	release := make(chan struct{})
	var started, wg sync.WaitGroup
	var loads atomic.Int64
	for i := range 2 {
		started.Add(1)
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _ = c.get(t.Context(), key(i), func() (Serde, error) {
				loads.Add(1)
				started.Done()
				<-release
				return fakeSerde{}, nil
			})
		}()
	}
	started.Wait()

	// Full of in-flight loads: a third key runs, uncached, and does not block.
	srd, ran, err := c.get(t.Context(), key(2), func() (Serde, error) { loads.Add(1); return fakeSerde{name: "uncached"}, nil })
	if err != nil || !ran || srd != (fakeSerde{name: "uncached"}) {
		t.Fatalf("srd=%v ran=%v err=%v", srd, ran, err)
	}
	if c.size() != 2 {
		t.Fatalf("size = %d, want 2", c.size())
	}

	// An in-flight key is joined, not evicted and not loaded twice.
	joined := make(chan struct{})
	go func() {
		defer close(joined)
		_, ran, _ := c.get(t.Context(), key(0), func() (Serde, error) { loads.Add(1); return fakeSerde{}, nil })
		if ran {
			t.Error("joiner ran its own load")
		}
	}()
	time.Sleep(10 * time.Millisecond)
	close(release)
	wg.Wait()
	<-joined
	if loads.Load() != 3 {
		t.Fatalf("loads = %d, want 3 (two in flight, one uncached)", loads.Load())
	}

	// The uncached key was not kept; with room freed by eviction it loads again and is kept.
	var again int
	for range 2 {
		_, _, _ = c.get(t.Context(), key(2), func() (Serde, error) { again++; return fakeSerde{}, nil })
	}
	if again != 1 || c.size() != 2 {
		t.Fatalf("again=%d size=%d, want 1 and 2", again, c.size())
	}
}

// Many goroutines load many distinct schemas, with transient failures and
// slow loads. The cache never holds more than its cap, never returns a nil
// Serde with a nil error, and always returns the key's own Serde. Run with
// -race.
func TestSerdeCache_CapHoldsUnderConcurrentFlood(t *testing.T) {
	const maxEntries = 64
	c := newSerdeCache(4*time.Hour, maxEntries, time.Now)
	var over, nilnil, wrong atomic.Int64
	deadline := time.Now().Add(2 * time.Second)
	var wg sync.WaitGroup
	for g := range 256 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; time.Now().Before(deadline); i++ {
				fp := uint64(g)<<32 | uint64(i)
				srd, _, err := c.get(t.Context(), testCacheKey(fp), func() (Serde, error) {
					time.Sleep(time.Duration(i%3) * 20 * time.Microsecond)
					if i%5 == 0 {
						return nil, fmt.Errorf("flaky: %w", context.DeadlineExceeded)
					}
					return fakeSerde{name: fmt.Sprint(fp)}, nil
				})
				if n := c.size(); n > maxEntries {
					over.Add(1)
				}
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
	if over.Load() != 0 || nilnil.Load() != 0 || wrong.Load() != 0 {
		t.Fatalf("over=%d nilnil=%d wrong=%d", over.Load(), nilnil.Load(), wrong.Load())
	}
	if n := c.size(); n > maxEntries {
		t.Fatalf("size = %d, cap %d", n, maxEntries)
	}
}

func TestSerdeCacheSize_ReportsTheGlobalCache(t *testing.T) {
	before := SerdeCacheSize()
	typ, _ := registerFakeType(t, func(int64) (Serde, error) { return fakeSerde{}, nil })
	_, err := Schema{Type: typ, Bytes: uniqueSchemaBytes(t)}.Serde()
	if err != nil {
		t.Fatal(err)
	}
	if SerdeCacheSize() < before { // other entries may be evicted or expire, but not drop below a fresh insert's worth
		t.Fatalf("size went from %d to %d", before, SerdeCacheSize())
	}
}

// Hammer distinct schemas from many goroutines while an observer polls the
// size, and check the size from inside loads too: the map must never exceed
// the cap at any observation point, in flight entries included. Run with
// -race.
func TestSerdeCache_MapNeverExceedsCap(t *testing.T) {
	const maxEntries = 32
	c := newSerdeCache(4*time.Hour, maxEntries, time.Now)
	var maxSeen, violations atomic.Int64
	observe := func() {
		n := int64(c.size())
		for {
			old := maxSeen.Load()
			if n <= old || maxSeen.CompareAndSwap(old, n) {
				break
			}
		}
		if n > maxEntries {
			violations.Add(1)
		}
	}

	stop := make(chan struct{})
	observed := make(chan struct{})
	go func() {
		defer close(observed)
		for {
			select {
			case <-stop:
				return
			default:
				observe()
			}
		}
	}()

	deadline := time.Now().Add(2 * time.Second)
	var wg sync.WaitGroup
	for g := range 128 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; time.Now().Before(deadline); i++ {
				fp := uint64(g)<<32 | uint64(i)
				_, _, _ = c.get(t.Context(), testCacheKey(fp), func() (Serde, error) {
					observe() // this load is in flight
					time.Sleep(time.Duration(i%4) * 50 * time.Microsecond)
					return fakeSerde{}, nil
				})
				observe()
			}
		}()
	}
	wg.Wait()
	close(stop)
	<-observed

	if violations.Load() != 0 {
		t.Fatalf("size exceeded %d %d times (max seen %d)", maxEntries, violations.Load(), maxSeen.Load())
	}
	if maxSeen.Load() < maxEntries {
		t.Fatalf("max seen %d: the flood never reached the cap, so the test proved nothing", maxSeen.Load())
	}
}

func TestSerdeCache_ExpiredEntriesLeaveTheFinishedList(t *testing.T) {
	c, clk := newTestCache()
	for i := range 10 {
		_, _, _ = c.get(t.Context(), testCacheKey(uint64(i)), func() (Serde, error) { return fakeSerde{}, nil })
	}
	clk.advance(5 * time.Hour)
	_, _, _ = c.get(t.Context(), testKey, func() (Serde, error) { return fakeSerde{}, nil })
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) != 1 || c.finished.Len() != 1 {
		t.Fatalf("entries=%d finished=%d, want 1 and 1", len(c.entries), c.finished.Len())
	}
}
