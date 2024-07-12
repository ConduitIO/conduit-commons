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

package inmemory

import (
	"context"
	"strings"
	"sync"

	"github.com/conduitio/conduit-commons/database"
)

// DB is a naive store that stores values in memory. Do not use in production,
// changes are lost on restart.
type DB struct {
	initOnce sync.Once
	values   map[string][]byte
	m        sync.RWMutex
}

var _ database.DB = (*DB)(nil)

func (d *DB) Ping(_ context.Context) error {
	return nil
}

func (d *DB) NewTransaction(ctx context.Context, _ bool) (database.Transaction, context.Context, error) {
	d.m.Lock()
	defer d.m.Unlock()

	valuesCopy := make(map[string][]byte, len(d.values))
	for k, v := range d.values {
		valuesCopy[k] = v
	}

	t := &Txn{
		db:      d,
		old:     valuesCopy,
		changes: make(map[string][]byte),
	}
	return t, database.ContextWithTransaction(ctx, t), nil
}

func (d *DB) Set(ctx context.Context, key string, value []byte) error {
	d.init()
	txn := d.getTxn(ctx)
	if txn != nil {
		return txn.set(key, value)
	}

	d.m.Lock()
	defer d.m.Unlock()

	if value != nil {
		d.values[key] = value
	} else {
		delete(d.values, key)
	}
	return nil
}

func (d *DB) Get(ctx context.Context, key string) ([]byte, error) {
	d.init()
	txn := d.getTxn(ctx)
	if txn != nil {
		return txn.get(key)
	}

	d.m.RLock()
	defer d.m.RUnlock()

	val, ok := d.values[key]
	if !ok {
		return nil, database.ErrKeyNotExist
	}
	return val, nil
}

func (d *DB) GetKeys(ctx context.Context, prefix string) ([]string, error) {
	d.init()
	txn := d.getTxn(ctx)
	if txn != nil {
		return txn.getKeys(prefix)
	}

	d.m.RLock()
	defer d.m.RUnlock()

	var result []string
	for k := range d.values {
		if strings.HasPrefix(k, prefix) {
			result = append(result, k)
		}
	}
	return result, nil
}

// init lazily initializes the values map.
func (d *DB) init() {
	d.initOnce.Do(func() {
		d.values = make(map[string][]byte)
	})
}

// Close is a noop.
func (d *DB) Close() error {
	return nil
}

func (d *DB) getTxn(ctx context.Context) *Txn {
	txn, ok := database.TransactionFromContext(ctx).(*Txn)
	if !ok {
		return nil
	}
	return txn
}
