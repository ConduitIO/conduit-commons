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

package csync

import (
	"iter"
	"sync"
)

// Map is a thread-safe map.
type Map[K comparable, T any] struct {
	m    map[K]T
	lock sync.RWMutex
}

// NewMap creates a new map.
func NewMap[K comparable, T any]() *Map[K, T] {
	return &Map[K, T]{
		m: make(map[K]T),
	}
}

// Set sets the value for a key.
func (m *Map[K, T]) Set(key K, value T) {
	m.lock.Lock()
	m.m[key] = value
	m.lock.Unlock()
}

// Get retrieves the value for a key.
func (m *Map[K, T]) Get(key K) (T, bool) {
	m.lock.RLock()
	value, ok := m.m[key]
	m.lock.RUnlock()
	return value, ok
}

// Delete removes the value for a key.
func (m *Map[K, T]) Delete(key K) {
	m.lock.Lock()
	delete(m.m, key)
	m.lock.Unlock()
}

// Len returns the number of items in the map.
func (m *Map[K, T]) Len() int {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return len(m.m)
}

// Keys returns all the keys in the map.
func (m *Map[K, T]) Keys() []K {
	m.lock.RLock()
	defer m.lock.RUnlock()
	keys := make([]K, 0, len(m.m))
	for k := range m.m {
		keys = append(keys, k)
	}
	return keys
}

// Values returns all the values in the map.
func (m *Map[K, T]) Values() []T {
	m.lock.RLock()
	defer m.lock.RUnlock()
	values := make([]T, 0, len(m.m))
	for _, v := range m.m {
		values = append(values, v)
	}
	return values
}

// Clear removes all items from the map.
func (m *Map[K, T]) Clear() {
	m.lock.Lock()
	m.m = make(map[K]T)
	m.lock.Unlock()
}

// Copy returns a new map with the same key-value pairs.
func (m *Map[K, T]) Copy() *Map[K, T] {
	m.lock.RLock()
	defer m.lock.RUnlock()

	newMap := NewMap[K, T]()
	for k, v := range m.m {
		newMap.m[k] = v
	}
	return newMap
}

// Merge adds all key-value pairs from another map to this map.
func (m *Map[K, T]) Merge(other *Map[K, T]) {
	other.lock.RLock()
	defer other.lock.RUnlock()

	m.lock.Lock()
	defer m.lock.Unlock()

	for k, v := range other.m {
		m.m[k] = v
	}
}

// ToGoMap returns a copy of the map as a Go map.
func (m *Map[K, T]) ToGoMap() map[K]T {
	return m.Copy().m
}

// All returns an iterator over the map's key-value pairs. This can be used to
// iterate over the map using a for-range loop.
//
// Example:
//
//	for key, value := range m.All() {
//	    fmt.Println(key, value)
//	}
func (m *Map[K, T]) All() iter.Seq2[K, T] {
	return func(yield func(K, T) bool) {
		m.lock.RLock()
		defer m.lock.RUnlock()
		for k, v := range m.m {
			if !yield(k, v) {
				return
			}
		}
	}
}
