// Copyright © 2024 Meroxa, Inc.
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
	"bytes"
	"fmt"
	"strings"

	"github.com/conduitio/conduit-commons/database"
)

type Txn struct {
	db      *DB
	old     map[string][]byte
	changes map[string][]byte
}

var _ database.Transaction = (*Txn)(nil)

func (t *Txn) Commit() error {
	t.db.m.Lock()
	defer t.db.m.Unlock()

	for k := range t.changes {
		oldVal, oldOk := t.old[k]
		newVal, newOk := t.db.values[k]

		if !bytes.Equal(oldVal, newVal) || oldOk != newOk {
			return fmt.Errorf("failed to set key %s: the key was changed since the transaction started", k)
		}
	}

	for k, v := range t.changes {
		t.db.values[k] = v
		if v == nil {
			delete(t.db.values, k)
		}
	}
	return nil
}

func (t *Txn) Discard() {
	// do nothing
}

func (t *Txn) set(key string, value []byte) error {
	t.changes[key] = value
	return nil
}

func (t *Txn) get(key string) ([]byte, error) {
	val, ok := t.changes[key]
	if ok {
		if val == nil {
			return nil, database.ErrKeyNotExist
		}
		return val, nil
	}
	val, ok = t.old[key]
	if !ok {
		return nil, database.ErrKeyNotExist
	}
	return val, nil
}

func (t *Txn) getKeys(prefix string) ([]string, error) {
	var result []string
	for k := range t.old {
		if strings.HasPrefix(k, prefix) {
			result = append(result, k)
		}
	}
	for k, v := range t.changes {
		if strings.HasPrefix(k, prefix) {
			if v == nil {
				// it's a delete, remove the key
				for i, rk := range result {
					if rk == k {
						result = append(result[:i], result[i+1:]...)
						break
					}
				}
			} else {
				result = append(result, k)
			}
		}
	}
	return result, nil
}
