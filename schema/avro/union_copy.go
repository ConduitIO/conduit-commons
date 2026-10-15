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

package avro

import (
	"reflect"

	"github.com/conduitio/conduit-commons/opencdc"
)

// copyForWrap returns the value Serde.Marshal hands to wrapSubstitutions in
// place of val.
//
// The wrapping rewrites union values inside the containers it walks, so
// running it on the caller's value changes what the caller holds, and what
// everything else that shares those containers holds (a record's Before and
// After, fan-out branches, the connector's own retained data). A second
// encode of such a value either failed or, when the wrapper map was itself a
// valid union branch, silently wrote the wrapper in place of the value
// (ConduitIO/conduit-commons#342).
//
// Invariant 6 (schema handling never silently mangles data): Marshal never
// changes its input. When the schema has unions to wrap, the containers
// wrapSubstitutions could reach are copied first and it rewrites the copy.
// Only map and slice containers are copied; leaf values (strings, numbers,
// []byte, time.Time, ...) are shared, because the wrapping never writes to
// them. The copy is gated on wrapsUnions, the same flag wrapSubstitutions
// uses to return early, so the two cannot disagree about whether anything is
// rewritten.
func (r unionResolver) copyForWrap(val any) any {
	if !r.wrapsUnions {
		return val // wrapSubstitutions returns no changes
	}
	return newContainerCopier().copy(val)
}

// containerKey identifies a map or slice by identity.
type containerKey struct {
	ptr uintptr
	len int
}

// containerCopier copies the map and slice containers that traverseValue and
// BeforeMarshal can reach: map[string]any, []any, opencdc.StructuredData and
// pointers to the maps. It keeps the dynamic types, nil-ness and sharing of
// what it copies, including cycles, so BeforeMarshal behaves on the copy
// exactly as it would have on the original.
type containerCopier struct {
	maps   map[containerKey]map[string]any
	slices map[containerKey][]any
}

func newContainerCopier() *containerCopier {
	return &containerCopier{
		maps:   make(map[containerKey]map[string]any),
		slices: make(map[containerKey][]any),
	}
}

func (c *containerCopier) copy(v any) any {
	switch v := v.(type) {
	case map[string]any:
		return c.copyMap(v)
	case opencdc.StructuredData:
		return opencdc.StructuredData(c.copyMap(v))
	case []any:
		return c.copySlice(v)
	case *any:
		if v == nil {
			return v
		}
		x := c.copy(*v)
		return &x
	case *map[string]any:
		if v == nil {
			return v
		}
		m := c.copyMap(*v)
		return &m
	case *opencdc.StructuredData:
		if v == nil {
			return v
		}
		m := opencdc.StructuredData(c.copyMap(*v))
		return &m
	default:
		return v
	}
}

func (c *containerCopier) copyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	key := containerKey{ptr: reflect.ValueOf(m).Pointer()}
	if cp, ok := c.maps[key]; ok {
		return cp
	}
	cp := make(map[string]any, len(m))
	c.maps[key] = cp // before recursing, so a cycle finds it
	for k, v := range m {
		cp[k] = c.copy(v)
	}
	return cp
}

func (c *containerCopier) copySlice(s []any) []any {
	if s == nil {
		return nil
	}
	if len(s) == 0 {
		return []any{}
	}
	key := containerKey{ptr: reflect.ValueOf(s).Pointer(), len: len(s)}
	if cp, ok := c.slices[key]; ok {
		return cp
	}
	cp := make([]any, len(s))
	c.slices[key] = cp
	for i, v := range s {
		cp[i] = c.copy(v)
	}
	return cp
}
