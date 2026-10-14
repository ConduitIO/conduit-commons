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
	"container/list"
	"context"
	"errors"
	"sync"
	"time"
)

// serdeCache is a process-wide cache of parsed Serdes with single-flight
// loading: concurrent callers for one key share one load. It replaces a
// third-party cache whose Delete could clear an entry while another caller's
// load was still in flight, which made waiting callers return a nil Serde
// with a nil error.
//
// The cache never returns a nil Serde with a nil error: a load that produces
// one is reported as errNilSerde and not cached.
//
// The cache holds at most maxEntries entries, in flight or finished, and the
// cap is enforced under the lock that inserts: room is made before a new
// entry goes in, so the map never exceeds it, not even briefly. Making room
// evicts the finished entry that expires first, in O(1): finished entries sit
// in a list in the order they finished, which, with one maxAge, is expiry
// order. A load in flight is never evicted (that would let a second load of
// the same key start). If every entry is a load in flight, the new load runs
// without being cached and without single-flight, on its caller's goroutine:
// it neither blocks nor adds to the map. The number of such loads is bounded
// by the number of concurrent callers, and each holds nothing after it
// returns.
//
// Results, errors included, are kept for maxAge. A load that fails with a
// transient error (see isTransientParseError) is not kept at all: the loading
// caller removes its own entry before waking the callers that joined it, so
// those callers see the error and the next call loads again. Nothing ever
// removes an entry it did not create, so a reload in flight is never
// disturbed.
//
// The cache assumes one schema registry per process: a key (type,
// fingerprint, references) names the same schema for every caller, whichever
// Resolver they pass. Expired entries are dropped lazily, from the front of the
// finished list, as callers arrive, so there is no goroutine to leak and an
// idle process keeps its last entries until the next call.
type serdeCache struct {
	maxAge     time.Duration
	maxEntries int
	now        func() time.Time // replaced in tests

	mu      sync.Mutex
	entries map[serdeCacheKey]*serdeEntry
	// finished lists the keys of the finished, kept entries in the order they
	// finished, oldest first. In-flight entries are not in it.
	finished *list.List // of serdeCacheKey
}

// serdeCacheMaxEntries is the most Serdes the process-wide cache holds. It is
// not configurable: the cache is process-wide and shared by every pipeline.
const serdeCacheMaxEntries = 4096

// SerdeCacheSize returns how many entries (finished and in flight) the
// process-wide Serde cache holds now, for callers that export it as a metric.
// It never exceeds 4096.
func SerdeCacheSize() int { return globalSerdeCache.size() }

func (c *serdeCache) size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

type serdeEntry struct {
	done chan struct{} // closed once srd and err are set
	srd  Serde
	err  error
	// expires is set before done is closed. It is only read for entries
	// found in the map after done is closed.
	expires time.Time
	// elem is the entry's place in serdeCache.finished, nil while in flight.
	elem *list.Element
}

func newSerdeCache(maxAge time.Duration, maxEntries int, now func() time.Time) *serdeCache {
	return &serdeCache{maxAge: maxAge, maxEntries: maxEntries, now: now, entries: make(map[serdeCacheKey]*serdeEntry), finished: list.New()}
}

// errNilSerde is what a load that returned a nil Serde with a nil error
// becomes. It should be unreachable; it exists so the failure is an error and
// not a nil dereference in the caller. It is transient, so it is retried and
// never cached.
var errNilSerde = errors.New("schema cache: load returned no serde")

// errLoadPanicked is the error joined callers get when the load they were
// waiting for panicked. The panic itself continues in the loading goroutine.
var errLoadPanicked = errors.New("schema cache: load panicked")

// get returns the Serde cached under key, calling load to produce it if there
// is none. ran reports whether this call ran load, as opposed to using a
// cached result or one produced by another caller's load.
//
// A caller that waits for another caller's load stops waiting, with ctx's
// error, if ctx ends first. The load itself runs in the calling goroutine and
// is expected to honor its own context.
func (c *serdeCache) get(ctx context.Context, key serdeCacheKey, load func() (Serde, error)) (srd Serde, ran bool, err error) {
	c.mu.Lock()
	now := c.now()
	c.dropExpiredLocked(now)
	if e, ok := c.entries[key]; ok {
		select {
		case <-e.done:
			if now.Before(e.expires) {
				c.mu.Unlock()
				return e.srd, false, e.err
			}
			c.removeLocked(key, e) // expired; load again below
		default:
			c.mu.Unlock()
			select {
			case <-e.done:
				return e.srd, false, e.err
			case <-ctx.Done():
				return nil, false, ctx.Err()
			}
		}
	}
	if len(c.entries) >= c.maxEntries && !c.evictOldestLocked() {
		// Every entry is a load in flight, so there is nothing to evict.
		// Load without caching, rather than block this caller or let the
		// cache grow past the cap.
		c.mu.Unlock()
		srd, err = load()
		if err == nil && srd == nil {
			err = errNilSerde
		}
		if err != nil {
			srd = nil
		}
		return srd, true, err
	}
	e := &serdeEntry{done: make(chan struct{})}
	c.entries[key] = e
	c.mu.Unlock()

	c.load(key, e, load)
	return e.srd, true, e.err
}

func (c *serdeCache) load(key serdeCacheKey, e *serdeEntry, load func() (Serde, error)) {
	finished := false
	defer func() {
		if !finished { // load panicked: wake the waiters, and let the panic go on
			e.srd, e.err = nil, errLoadPanicked
			c.finish(key, e, true)
		}
	}()
	srd, err := load()
	if err == nil && srd == nil {
		err = errNilSerde
	}
	if err != nil {
		srd = nil
	}
	e.srd, e.err = srd, err
	finished = true
	c.finish(key, e, isTransientParseError(err))
}

// finish publishes e's result. A dropped entry is removed from the map
// before its waiters wake, and only if the map still holds e.
func (c *serdeCache) finish(key serdeCacheKey, e *serdeEntry, drop bool) {
	c.mu.Lock()
	if drop {
		if c.entries[key] == e {
			delete(c.entries, key)
		}
	} else {
		e.expires = c.now().Add(c.maxAge)
		e.elem = c.finished.PushBack(key)
	}
	c.mu.Unlock()
	close(e.done)
}

// removeLocked deletes e, stored under key, from the map and the finished
// list.
func (c *serdeCache) removeLocked(key serdeCacheKey, e *serdeEntry) {
	delete(c.entries, key)
	if e.elem != nil {
		c.finished.Remove(e.elem)
		e.elem = nil
	}
}

// evictOldestLocked removes the finished entry that expires first and
// reports whether there was one.
func (c *serdeCache) evictOldestLocked() bool {
	front := c.finished.Front()
	if front == nil {
		return false
	}
	key := front.Value.(serdeCacheKey) //nolint:forcetypeassert // the list only holds keys
	c.removeLocked(key, c.entries[key])
	return true
}

// dropExpiredLocked removes expired entries from the front of the finished
// list. Each entry is removed once, so the cost is amortized O(1) per call.
func (c *serdeCache) dropExpiredLocked(now time.Time) {
	for front := c.finished.Front(); front != nil; front = c.finished.Front() {
		key := front.Value.(serdeCacheKey) //nolint:forcetypeassert // the list only holds keys
		e := c.entries[key]
		if now.Before(e.expires) {
			return
		}
		c.removeLocked(key, e)
	}
}
