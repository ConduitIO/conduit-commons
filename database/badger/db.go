// Copyright © 2022 Meroxa, Inc.
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

package badger

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"github.com/conduitio/conduit-commons/database"
	"github.com/dgraph-io/badger/v4"
	"github.com/rs/zerolog"
)

// ErrWriteAbandoned is returned (wrapped together with ctx.Err()) by Set when
// its context ended after the write was handed to badger. Set has stopped
// waiting, but the write keeps going in the background: it either commits
// in full or not at all, and the caller cannot know which. See Set.
var ErrWriteAbandoned = errors.New("badger: stopped waiting for write, it may still complete")

// DB implements the database.DB interface for server persistence.
//
// DB is safe for concurrent use.
type DB struct {
	db *badger.DB

	// keyLocks orders Set calls (outside a transaction) per key, so a write
	// abandoned on context cancel can't land after a later write to the same
	// key. See Set.
	keyLocks keyLocks
	// inflight counts Set writes that are still running, including
	// abandoned ones. Close waits for them. closeMu and closed keep a Set
	// from adding to inflight once Close has started waiting.
	inflight sync.WaitGroup
	closeMu  sync.RWMutex
	closed   bool
	// commit commits a transaction. It is txn.Commit, swapped in tests to
	// simulate a write that blocks.
	commit func(*badger.Txn) error

	logger zerolog.Logger
	// abandoned and abandonedFailed count writes Set stopped waiting for,
	// and those of them whose commit then failed. See AbandonedWrites.
	abandoned       atomic.Uint64
	abandonedFailed atomic.Uint64
}

// AbandonedWriteStats counts writes whose caller stopped waiting
// (ErrWriteAbandoned) since the DB was opened.
type AbandonedWriteStats struct {
	// Abandoned is the number of writes Set returned ErrWriteAbandoned for.
	Abandoned uint64
	// Failed is how many of those failed to commit after Set returned, so
	// the key kept its old value. The rest committed, or are still running.
	// Failed <= Abandoned always holds. Abandoned can briefly count a write
	// that finished at the same moment its ctx ended and was reported to
	// the caller after all; that count is taken back right away.
	Failed uint64
}

// AbandonedWrites returns the abandoned-write counters. Each failure is
// also logged at error level, because no caller is left to see it.
func (d *DB) AbandonedWrites() AbandonedWriteStats {
	return AbandonedWriteStats{
		Abandoned: d.abandoned.Load(),
		Failed:    d.abandonedFailed.Load(),
	}
}

var _ database.DB = (*DB)(nil)

// New returns a DB implementation that fulfills the Store interface.
func New(l zerolog.Logger, path string) (*DB, error) {
	opt := badger.DefaultOptions(path)
	opt.Logger = logger(l.With().Str("component", "badger.DB").Logger())

	db, err := badger.Open(opt)
	if err != nil {
		return nil, fmt.Errorf("badger: could not open db: %w", err)
	}

	return &DB{
		db:       db,
		keyLocks: keyLocks{m: make(map[string]*keyLock)},
		logger:   l.With().Str("component", "badger.DB").Logger(),
		commit:   (*badger.Txn).Commit,
	}, nil
}

func (d *DB) Ping(context.Context) error {
	// Writing or reading data through any of the BadgerDB APIs
	// (transactions, batch writer etc.)
	// doesn't trigger a complete flush to disk, until the DB
	// is closed or keys are dropped.
	// The latter can be done in this ping, but it's a "heavy" operation
	// since it stops all writes, compacts the DB etc.
	// Hence, for now, we'll only check if the directory backing BadgerDB
	// is accessible.
	_, err := os.ReadDir(d.db.Opts().Dir)
	if err != nil {
		return fmt.Errorf("failed reading key data dir %s: %w", d.db.Opts().Dir, err)
	}

	_, err = os.ReadDir(d.db.Opts().ValueDir)
	if err != nil {
		return fmt.Errorf("failed reading value data dir %s: %w", d.db.Opts().ValueDir, err)
	}

	return nil
}

// NewTransaction starts a new transaction and returns a context that contains
// it. If the context is passed to other functions in this DB the transaction
// will be used for executing the operation.
func (d *DB) NewTransaction(ctx context.Context, update bool) (database.Transaction, context.Context, error) {
	txn := d.db.NewTransaction(update)
	return txn, database.ContextWithTransaction(ctx, txn), nil
}

// Close waits for Set writes still in flight, including ones whose caller
// stopped waiting (ErrWriteAbandoned), then flushes pending writes and closes
// the db. After Close returns, every abandoned write has either committed or
// failed, so a reopened db shows its final outcome.
func (d *DB) Close() error {
	d.closeMu.Lock()
	d.closed = true
	d.closeMu.Unlock()
	d.inflight.Wait()
	if err := d.db.Close(); err != nil {
		return fmt.Errorf("failed to close badger db: %w", err)
	}
	return nil
}

// Get finds the value corresponding to the key and returns it or an error.
// If the key doesn't exist an error is returned.
func (d *DB) Get(ctx context.Context, key string) ([]byte, error) {
	txn := d.getTxn(ctx)
	if txn == nil {
		// create new transaction just for this call
		txn = d.db.NewTransaction(false)
		defer txn.Discard()
	}
	return d.getWithTxn(txn, key)
}

func (d *DB) getWithTxn(txn *badger.Txn, key string) ([]byte, error) {
	item, err := txn.Get([]byte(key))
	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			err = database.ErrKeyNotExist // translate to internal error
		}
		return nil, fmt.Errorf("badger: could not get key %q: %w", key, err)
	}

	val, err := item.ValueCopy(nil)
	if err != nil {
		return nil, fmt.Errorf("badger: could not get value for key %q: %w", key, err)
	}

	return val, nil
}

// Set updates key to value. A nil value deletes the key; to store an empty
// value use an empty, non-nil slice.
//
// If ctx carries a transaction (see NewTransaction), the write is only staged
// in it, which does not block; the caller's Commit applies it.
//
// Otherwise Set writes and commits in its own transaction and honours ctx:
//
//   - If ctx ends before the write starts (including while waiting behind an
//     earlier write to the same key), Set returns ctx.Err() and nothing is
//     written.
//   - If ctx ends while the commit is running, Set returns an error that
//     matches both ErrWriteAbandoned and ctx.Err(). The commit keeps running
//     in the background and may still succeed. The caller must treat the key
//     as unknown: either the old value or the new one, never a mix.
//     Invariant 5: badger commits a transaction atomically, and on a crash
//     replay drops a transaction whose commit marker was not written, so the
//     key is never torn.
//     The background outcome is logged (error level if the commit failed)
//     and counted in AbandonedWrites.
//
// A Set does not start its write while an earlier Set to the same key is
// still running, abandoned or not; it waits, bounded by its own ctx. So an
// abandoned write can never land on top of a value written after it. Writes
// to different keys do not wait for each other. Close waits for abandoned
// writes to finish.
func (d *DB) Set(ctx context.Context, key string, value []byte) error {
	if txn := d.getTxn(ctx); txn != nil {
		return d.setWithTxn(txn, key, value)
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("badger: did not set key %q: %w", key, err)
	}
	unlock, err := d.keyLocks.lock(ctx, key)
	if err != nil {
		return fmt.Errorf("badger: did not set key %q: %w", key, err)
	}
	// lock's select may pick the free lock even though ctx has ended, so
	// check again: a Set whose ctx ended before the write starts writes
	// nothing.
	if err := ctx.Err(); err != nil {
		unlock()
		return fmt.Errorf("badger: did not set key %q: %w", key, err)
	}

	d.closeMu.RLock()
	if d.closed {
		d.closeMu.RUnlock()
		unlock()
		return fmt.Errorf("badger: did not set key %q: %w", key, badger.ErrDBClosed)
	}
	d.inflight.Add(1)
	d.closeMu.RUnlock()

	// The goroutine owns the transaction and the key lock from here on, so
	// it can finish (or fail) the write after we stop waiting for it.
	// handoff decides, exactly once, who reports the outcome: the goroutine
	// hands it to Set (handoffDelivered), or Set gives up first
	// (handoffAbandoned) and the goroutine logs and counts it instead.
	var handoff atomic.Int32
	done := make(chan error, 1)
	go func() {
		defer d.inflight.Done()
		defer unlock()
		err := d.setAndCommit(key, value)
		if handoff.CompareAndSwap(handoffWaiting, handoffDelivered) {
			done <- err
			return
		}
		d.reportAbandoned(key, err)
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		// Count before the CAS, so a reader never sees Failed > Abandoned:
		// once Set wins, the goroutine may count a failure at any moment.
		d.abandoned.Add(1)
		if !handoff.CompareAndSwap(handoffWaiting, handoffAbandoned) {
			d.abandoned.Add(^uint64(0)) // not abandoned after all
			return <-done               // finished at the same moment; report what happened
		}
		return fmt.Errorf("badger: set key %q: %w: %w", key, ErrWriteAbandoned, ctx.Err())
	}
}

const (
	handoffWaiting int32 = iota
	handoffDelivered
	handoffAbandoned
)

// reportAbandoned records the outcome of a write whose caller already got
// ErrWriteAbandoned. Nobody else will see a failure, so it is logged at
// error level and counted.
func (d *DB) reportAbandoned(key string, err error) {
	if err != nil {
		d.abandonedFailed.Add(1)
		d.logger.Error().Err(err).Str("key", key).
			Msg("badger: abandoned write failed, the key keeps its previous value")
		return
	}
	d.logger.Info().Str("key", key).Msg("badger: abandoned write committed after its caller stopped waiting")
}

// setAndCommit writes key in a new transaction and commits it. The
// transaction is discarded if anything fails, so nothing is half-applied.
func (d *DB) setAndCommit(key string, value []byte) error {
	txn := d.db.NewTransaction(true)
	defer txn.Discard() // no-op after a successful Commit
	if err := d.setWithTxn(txn, key, value); err != nil {
		return err
	}
	commit := d.commit
	if commit == nil {
		commit = (*badger.Txn).Commit
	}
	if err := commit(txn); err != nil {
		return fmt.Errorf("badger: could not commit transaction setting key %q: %w", key, err)
	}
	return nil
}

// setWithTxn will update a key using the provided transaction.
func (d *DB) setWithTxn(txn *badger.Txn, key string, value []byte) error {
	// if we set the value to nothing, we're considering that a delete.
	// if we need to maintain an empty value, it should be an empty byte slice.
	if value == nil {
		err := txn.Delete([]byte(key))
		if err != nil {
			return fmt.Errorf("badger: could not delete key %q: %w", key, err)
		}
	} else {
		err := txn.Set([]byte(key), value)
		if err != nil {
			return fmt.Errorf("badger: could not set key %q: %w", key, err)
		}
	}
	return nil
}

// GetKeys will range over DB keys and return the ones with the given prefix.
func (d *DB) GetKeys(ctx context.Context, prefix string) ([]string, error) {
	txn := d.getTxn(ctx)
	if txn == nil {
		// create new transaction just for this call
		txn = d.db.NewTransaction(false)
		defer txn.Discard()
	}
	return d.getKeysWithTxn(txn, prefix)
}

// getKeysWithTxn will return keys with a prefix using the provided transaction.
func (d *DB) getKeysWithTxn(txn *badger.Txn, prefix string) ([]string, error) {
	opt := badger.DefaultIteratorOptions
	opt.Prefix = []byte(prefix)
	opt.PrefetchValues = false // only iterate keys
	it := txn.NewIterator(opt)
	defer it.Close()

	var results []string
	for it.Rewind(); it.Valid(); it.Next() {
		results = append(results, string(it.Item().Key()))
	}

	return results, nil
}

// getTxn takes the transaction out of the context and returns it. If the
// context does not contain a transaction it returns nil.
func (d *DB) getTxn(ctx context.Context) *badger.Txn {
	txn, ok := database.TransactionFromContext(ctx).(*badger.Txn)
	if !ok {
		return nil
	}
	return txn
}

// keyLocks hands out one lock per key. Entries exist only while a lock is
// held or waited for, so the map does not grow with the key space.
type keyLocks struct {
	mu sync.Mutex
	m  map[string]*keyLock
}

type keyLock struct {
	ch   chan struct{} // capacity 1; holding the lock = a value in ch
	refs int           // holders plus waiters, guarded by keyLocks.mu
}

// lock acquires the lock for key, or returns ctx.Err() if ctx ends first.
// The returned func releases it and must be called exactly once.
func (k *keyLocks) lock(ctx context.Context, key string) (func(), error) {
	k.mu.Lock()
	if k.m == nil {
		k.m = make(map[string]*keyLock)
	}
	l, ok := k.m[key]
	if !ok {
		l = &keyLock{ch: make(chan struct{}, 1)}
		k.m[key] = l
	}
	l.refs++
	k.mu.Unlock()

	select {
	case l.ch <- struct{}{}:
		return func() {
			<-l.ch
			k.release(key, l)
		}, nil
	case <-ctx.Done():
		k.release(key, l)
		return nil, ctx.Err()
	}
}

func (k *keyLocks) release(key string, l *keyLock) {
	k.mu.Lock()
	defer k.mu.Unlock()
	l.refs--
	if l.refs == 0 {
		delete(k.m, key)
	}
}
