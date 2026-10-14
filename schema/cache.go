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
// Results, errors included, are kept for maxAge. A load that fails with a
// transient error (see isTransientParseError) is not kept at all: the loading
// caller removes its own entry before waking the callers that joined it, so
// those callers see the error and the next call loads again. Nothing ever
// removes an entry it did not create, so a reload in flight is never
// disturbed.
//
// The cache assumes one schema registry per process: a key (type,
// fingerprint, references) names the same schema for every caller, whichever
// Resolver they pass. Expired entries are dropped lazily, at most once an
// hour, so an idle process holds on to them but never grows without bound
// under load and has no goroutine to leak.
type serdeCache struct {
	maxAge time.Duration
	now    func() time.Time // replaced in tests

	mu        sync.Mutex
	entries   map[serdeCacheKey]*serdeEntry
	lastSweep time.Time
}

// serdeSweepInterval is how often get drops expired entries.
const serdeSweepInterval = time.Hour

type serdeEntry struct {
	done chan struct{} // closed once srd and err are set
	srd  Serde
	err  error
	// expires is set before done is closed. It is only read for entries
	// found in the map after done is closed.
	expires time.Time
}

func newSerdeCache(maxAge time.Duration, now func() time.Time) *serdeCache {
	return &serdeCache{maxAge: maxAge, now: now, entries: make(map[serdeCacheKey]*serdeEntry)}
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
	c.sweepLocked(now)
	if e, ok := c.entries[key]; ok {
		select {
		case <-e.done:
			if now.Before(e.expires) {
				c.mu.Unlock()
				return e.srd, false, e.err
			}
			delete(c.entries, key) // expired; load again below
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
	}
	c.mu.Unlock()
	close(e.done)
}

func (c *serdeCache) sweepLocked(now time.Time) {
	if now.Sub(c.lastSweep) < serdeSweepInterval {
		return
	}
	c.lastSweep = now
	for k, e := range c.entries {
		select {
		case <-e.done:
			if !now.Before(e.expires) {
				delete(c.entries, k)
			}
		default: // still loading
		}
	}
}
