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
	"sort"
	"sync"
	"testing"

	"github.com/matryer/is"
)

func TestMap_NewMap(t *testing.T) {
	is := is.New(t)
	m := NewMap[string, int]()

	is.True(m != nil)
	is.Equal(m.Len(), 0)
}

func TestMap_SetAndGet(t *testing.T) {
	is := is.New(t)
	m := NewMap[string, int]()

	m.Set("foo", 1)
	v, ok := m.Get("foo")

	is.True(ok)
	is.Equal(v, 1)
}

func TestMap_GetNonExistent(t *testing.T) {
	is := is.New(t)
	m := NewMap[string, int]()

	v, ok := m.Get("bar")
	is.True(!ok)
	is.Equal(v, 0)
}

func TestMap_Delete(t *testing.T) {
	is := is.New(t)
	m := NewMap[string, int]()

	m.Set("foo", 1)
	m.Delete("foo")

	_, ok := m.Get("foo")
	is.True(!ok)
}

func TestMap_All(t *testing.T) {
	is := is.New(t)
	m := NewMap[string, int]()

	m.Set("foo", 1)
	m.Set("bar", 2)

	got := make(map[string]int)
	for key, value := range m.All() {
		got[key] = value
	}

	is.Equal(got, map[string]int{"foo": 1, "bar": 2})
}

func TestMap_Len(t *testing.T) {
	is := is.New(t)
	m := NewMap[string, int]()

	is.Equal(m.Len(), 0)
	m.Set("foo", 1)
	is.Equal(m.Len(), 1)
}

func TestMap_Keys(t *testing.T) {
	is := is.New(t)
	m := NewMap[string, int]()

	m.Set("foo", 1)
	m.Set("bar", 2)

	keys := m.Keys()
	sort.Strings(keys)
	is.Equal(keys, []string{"bar", "foo"})
}

func TestMap_Values(t *testing.T) {
	is := is.New(t)
	m := NewMap[string, int]()

	m.Set("foo", 1)
	m.Set("bar", 2)

	values := m.Values()
	sort.Ints(values)
	is.Equal(values, []int{1, 2})
}

func TestMap_Clear(t *testing.T) {
	is := is.New(t)
	m := NewMap[string, int]()

	m.Set("foo", 1)
	m.Clear()
	is.Equal(m.Len(), 0)
}

func TestMap_Copy(t *testing.T) {
	is := is.New(t)
	m := NewMap[string, int]()

	m.Set("foo", 1)

	got := m.Copy()
	is.Equal(got.Len(), 1)

	m.Set("foo", 2)
	v, ok := got.Get("foo")
	is.True(ok)
	is.Equal(v, 1)
}

func TestMap_Merge(t *testing.T) {
	is := is.New(t)
	m1 := NewMap[string, int]()
	m1.Set("foo", 1)
	m2 := NewMap[string, int]()
	m2.Set("bar", 2)

	m1.Merge(m2)
	is.Equal(m1.Len(), 2)

	v, ok := m1.Get("bar")
	is.True(ok)
	is.Equal(v, 2)
}

func TestMap_ToGoMap(t *testing.T) {
	is := is.New(t)
	m := NewMap[string, int]()

	m.Set("foo", 1)
	m.Set("bar", 2)

	got := m.ToGoMap()
	is.Equal(got, map[string]int{"foo": 1, "bar": 2})
}

func TestMap_Concurrency(t *testing.T) {
	is := is.New(t)
	m := NewMap[int, int]()
	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			m.Set(i, i)
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _ = m.Get(i)
		}(i)
	}

	wg.Wait()
	is.Equal(m.Len(), 1000)
}
