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

package badger

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/conduitio/conduit-commons/database"
	"github.com/dgraph-io/badger/v4"
	"github.com/matryer/is"
	"github.com/rs/zerolog"
)

// Regression tests for ConduitIO/conduit#2964: Set ignored its context, so a
// commit that hung could not be bounded by a deadline.

// returnBound is how long Set may take to return after its context ends.
const returnBound = 2 * time.Second

// blockingCommit makes the first commit for blockKey wait until release is
// closed, then fail with failErr if set. Later commits, and commits for
// other keys, go straight through, so a test that relies on Set's per-key
// ordering fails deterministically without it. entered is closed when the
// blocking commit starts.
type blockingCommit struct {
	blockKey string
	release  chan struct{}
	entered  chan struct{}
	failErr  error
	once     sync.Once
	unblockO sync.Once
}

// unblock releases the blocking commit. It is safe to call more than once,
// so cleanups can call it too and a failed test never leaves Close waiting
// on a commit that is never released.
func (b *blockingCommit) unblock() { b.unblockO.Do(func() { close(b.release) }) }

func newBlockingCommit(key string) *blockingCommit {
	return &blockingCommit{blockKey: key, release: make(chan struct{}), entered: make(chan struct{})}
}

func (b *blockingCommit) commit(txn *badger.Txn) error {
	if _, err := txn.Get([]byte(b.blockKey)); err == nil {
		// txn has a pending write for blockKey (Get sees its own writes)
		first := false
		b.once.Do(func() { first = true })
		if first {
			close(b.entered)
			<-b.release
			if b.failErr != nil {
				return b.failErr
			}
		}
	}
	return txn.Commit() //nolint:wrapcheck // stands in for (*badger.Txn).Commit
}

func newTestDB(t *testing.T) (*DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "badger.db")
	db, err := New(zerolog.Nop(), path)
	if err != nil {
		t.Fatal(err)
	}
	return db, path
}

// setAsync runs Set and returns its result on a channel.
func setAsync(ctx context.Context, db *DB, key string, value []byte) <-chan error {
	ch := make(chan error, 1)
	go func() { ch <- db.Set(ctx, key, value) }()
	return ch
}

func waitResult(t *testing.T, ch <-chan error) error {
	t.Helper()
	select {
	case err := <-ch:
		return err
	case <-time.After(returnBound):
		t.Fatalf("Set did not return within %v after its context ended", returnBound)
		return nil
	}
}

func waitClosed(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(returnBound):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func TestDB_Set_ReturnsWhenContextEndsDuringCommit(t *testing.T) {
	is := is.New(t)
	db, path := newTestDB(t)
	bc := newBlockingCommit("status")
	db.commit = bc.commit

	ctx, cancel := context.WithCancel(context.Background())
	res := setAsync(ctx, db, "status", []byte("running"))
	waitClosed(t, bc.entered, "the commit to start")
	cancel()

	err := waitResult(t, res)
	is.True(errors.Is(err, ErrWriteAbandoned))
	is.True(errors.Is(err, context.Canceled))

	// The abandoned write is still running; let it finish and check it
	// committed in full.
	bc.unblock()
	is.NoErr(db.Close())

	db2, err := New(zerolog.Nop(), path)
	is.NoErr(err)
	defer db2.Close()
	got, err := db2.Get(context.Background(), "status")
	is.NoErr(err)
	is.Equal(string(got), "running")
}

func TestDB_Set_DeadlineBoundsBlockedCommit(t *testing.T) {
	is := is.New(t)
	db, _ := newTestDB(t)
	bc := newBlockingCommit("status")
	db.commit = bc.commit
	t.Cleanup(func() {
		bc.unblock()
		is.NoErr(db.Close())
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := waitResult(t, setAsync(ctx, db, "status", []byte("running")))
	is.True(errors.Is(err, context.DeadlineExceeded))
	is.True(errors.Is(err, ErrWriteAbandoned))
	is.True(time.Since(start) < returnBound)
}

// An abandoned write must not land after a later write to the same key.
func TestDB_Set_LaterWriteToSameKeyWaitsForAbandonedOne(t *testing.T) {
	is := is.New(t)
	db, _ := newTestDB(t)
	bc := newBlockingCommit("status")
	db.commit = bc.commit
	t.Cleanup(func() { bc.unblock(); is.NoErr(db.Close()) })

	ctx1, cancel1 := context.WithCancel(context.Background())
	res1 := setAsync(ctx1, db, "status", []byte("running"))
	waitClosed(t, bc.entered, "the first commit to start")
	cancel1()
	is.True(errors.Is(waitResult(t, res1), ErrWriteAbandoned))

	// Second write: its context is ample, so it waits behind the first.
	res2 := setAsync(context.Background(), db, "status", []byte("stopped"))
	select {
	case err := <-res2:
		t.Fatalf("second Set returned (%v) while the first write was still running", err)
	case <-time.After(100 * time.Millisecond):
	}
	bc.unblock()
	is.NoErr(waitResult(t, res2))

	got, err := db.Get(context.Background(), "status")
	is.NoErr(err)
	is.Equal(string(got), "stopped") // newest value wins, not the abandoned one
}

// A Set whose context ends while it waits behind an earlier write writes
// nothing and says so with a plain context error.
func TestDB_Set_WaitingWriteGivesUpWithoutWriting(t *testing.T) {
	is := is.New(t)
	db, _ := newTestDB(t)
	bc := newBlockingCommit("status")
	db.commit = bc.commit
	t.Cleanup(func() { bc.unblock(); is.NoErr(db.Close()) })

	ctx1, cancel1 := context.WithCancel(context.Background())
	res1 := setAsync(ctx1, db, "status", []byte("running"))
	waitClosed(t, bc.entered, "the first commit to start")

	ctx2, cancel2 := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel2()
	err := waitResult(t, setAsync(ctx2, db, "status", []byte("stopped")))
	is.True(errors.Is(err, context.DeadlineExceeded))
	is.True(!errors.Is(err, ErrWriteAbandoned)) // never started, so nothing to wonder about

	bc.unblock()
	is.NoErr(waitResult(t, res1)) // the first write, still waited for, succeeds
	cancel1()

	got, err := db.Get(context.Background(), "status")
	is.NoErr(err)
	is.Equal(string(got), "running")
}

func TestDB_Set_OtherKeysNotBlocked(t *testing.T) {
	is := is.New(t)
	db, _ := newTestDB(t)
	bc := newBlockingCommit("status")
	db.commit = bc.commit
	t.Cleanup(func() {
		bc.unblock()
		is.NoErr(db.Close())
	})

	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	_ = setAsync(ctx1, db, "status", []byte("running"))
	waitClosed(t, bc.entered, "the first commit to start")

	is.NoErr(waitResult(t, setAsync(context.Background(), db, "other", []byte("v"))))
}

func TestDB_Set_CanceledContextWritesNothing(t *testing.T) {
	is := is.New(t)
	db, _ := newTestDB(t)
	t.Cleanup(func() { is.NoErr(db.Close()) })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := db.Set(ctx, "k", []byte("v"))
	is.True(errors.Is(err, context.Canceled))
	is.True(!errors.Is(err, ErrWriteAbandoned))

	_, err = db.Get(context.Background(), "k")
	is.True(errors.Is(err, database.ErrKeyNotExist))
}

// Close must not return while an abandoned write is still running.
func TestDB_Close_WaitsForAbandonedWrite(t *testing.T) {
	is := is.New(t)
	db, path := newTestDB(t)
	bc := newBlockingCommit("status")
	db.commit = bc.commit

	ctx, cancel := context.WithCancel(context.Background())
	res := setAsync(ctx, db, "status", []byte("running"))
	waitClosed(t, bc.entered, "the commit to start")
	cancel()
	is.True(errors.Is(waitResult(t, res), ErrWriteAbandoned))

	closed := make(chan error, 1)
	go func() { closed <- db.Close() }()
	select {
	case err := <-closed:
		t.Fatalf("Close returned (%v) while a write was still running", err)
	case <-time.After(100 * time.Millisecond):
	}
	bc.unblock()
	select {
	case err := <-closed:
		is.NoErr(err)
	case <-time.After(returnBound):
		t.Fatal("Close did not return after the write finished")
	}

	db2, err := New(zerolog.Nop(), path)
	is.NoErr(err)
	defer db2.Close()
	got, err := db2.Get(context.Background(), "status")
	is.NoErr(err)
	is.Equal(string(got), "running")

	// Set after Close fails and writes nothing.
	err = db.Set(context.Background(), "late", []byte("v"))
	is.True(errors.Is(err, badger.ErrDBClosed))
}

// A Set inside a transaction only stages the write; the context does not
// change that, and the write lands on Commit.
func TestDB_Set_InTransactionUnchanged(t *testing.T) {
	is := is.New(t)
	db, _ := newTestDB(t)
	t.Cleanup(func() { is.NoErr(db.Close()) })

	txn, ctx, err := db.NewTransaction(context.Background(), true)
	is.NoErr(err)
	defer txn.Discard()
	is.NoErr(db.Set(ctx, "k", []byte("v")))
	is.NoErr(txn.Commit())

	got, err := db.Get(context.Background(), "k")
	is.NoErr(err)
	is.Equal(string(got), "v")
}

// flipCtx is a context that is already done, but whose Err reports nil on
// the first call. It reaches the key lock with an ended context, which is
// the window the re-check after lock closes.
type flipCtx struct {
	context.Context //nolint:containedctx // wraps a context to change what Err reports
	calls           atomic.Int32
}

func (c *flipCtx) Err() error {
	if c.calls.Add(1) == 1 {
		return nil
	}
	return context.Canceled
}

func TestDB_Set_ContextEndedAtLockWritesNothing(t *testing.T) {
	is := is.New(t)
	db, _ := newTestDB(t)
	t.Cleanup(func() { is.NoErr(db.Close()) })

	// The lock is free, so lock's select picks it or ctx.Done at random.
	// Either way nothing may be written. 64 tries miss a broken re-check
	// with probability 2^-64.
	for i := 0; i < 64; i++ {
		done, cancel := context.WithCancel(context.Background())
		cancel()
		ctx := &flipCtx{Context: done}
		key := fmt.Sprintf("k%d", i)

		err := db.Set(ctx, key, []byte("v"))
		is.True(errors.Is(err, context.Canceled))
		is.True(!errors.Is(err, ErrWriteAbandoned))

		_, err = db.Get(context.Background(), key)
		is.True(errors.Is(err, database.ErrKeyNotExist))
	}
}

func TestDB_AbandonedWrites_CountsOutcome(t *testing.T) {
	for _, tc := range []struct {
		name       string
		failErr    error
		wantFailed uint64
	}{
		{name: "commit succeeds", failErr: nil, wantFailed: 0},
		{name: "commit fails", failErr: errors.New("disk on fire"), wantFailed: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			is := is.New(t)
			db, _ := newTestDB(t)
			bc := newBlockingCommit("status")
			bc.failErr = tc.failErr
			db.commit = bc.commit

			ctx, cancel := context.WithCancel(context.Background())
			res := setAsync(ctx, db, "status", []byte("running"))
			waitClosed(t, bc.entered, "the commit to start")
			cancel()
			is.True(errors.Is(waitResult(t, res), ErrWriteAbandoned))
			is.Equal(db.AbandonedWrites(), AbandonedWriteStats{Abandoned: 1})

			bc.unblock()
			is.NoErr(db.Close()) // waits for the abandoned write
			is.Equal(db.AbandonedWrites(), AbandonedWriteStats{Abandoned: 1, Failed: tc.wantFailed})
		})
	}
}
