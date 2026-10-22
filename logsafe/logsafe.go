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

// Package logsafe holds the one helper the Conduit ecosystem uses to refer to
// record data in a log line or an error without writing the data.
//
// Record keys, positions (which embed key values for some connectors) and
// payload values must not appear in logs or in errors: errors reach the DLQ,
// the API and CLI output. A salted digest lets two lines about the same value
// in one process be matched, which is all a log reader needs.
package logsafe

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"reflect"
	"sort"
	"time"
)

// salt is random per process: a logged digest cannot be reversed by hashing
// guessed values (record keys are often sequential ids) and cannot be matched
// across processes or restarts.
var salt = func() []byte {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("logsafe: cannot read random bytes for the salt: %v", err))
	}
	return b
}()

// Hash returns a short (16 hex characters), salted digest of b. The same bytes
// give the same digest within a process. Never log the bytes themselves; log
// their length and this.
func Hash(b []byte) string {
	mac := hmac.New(sha256.New, salt)
	mac.Write(b)
	return hex.EncodeToString(mac.Sum(nil)[:8])
}

// HashString is Hash for a string.
func HashString(s string) string { return Hash([]byte(s)) }

// HashValue is Hash for a structured value such as a key map. The digest is
// computed over a canonical encoding, not over fmt's output: equal values give
// equal digests whatever the map iteration order, pointer address or time zone
// of a time.Time (an instant is encoded in UTC), and values of different types
// differ, so 1 and "1" are different keys.
func HashValue(v any) string {
	var b bytes.Buffer
	canonical(&b, reflect.ValueOf(v))
	return Hash(b.Bytes())
}

var timeType = reflect.TypeOf(time.Time{})

// canonical writes a self-delimiting, type-tagged encoding of v.
func canonical(b *bytes.Buffer, v reflect.Value) {
	if !v.IsValid() {
		b.WriteString("n;")
		return
	}
	// Dereference pointers and interfaces: the address is not the value.
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			b.WriteString("n;")
			return
		}
		v = v.Elem()
	}
	if v.Type() == timeType {
		t, _ := v.Interface().(time.Time)
		fmt.Fprintf(b, "t%d;", t.UnixNano())
		return
	}
	switch v.Kind() {
	case reflect.Bool:
		fmt.Fprintf(b, "b%t;", v.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		fmt.Fprintf(b, "i%d;", v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		fmt.Fprintf(b, "u%d;", v.Uint())
	case reflect.Float32, reflect.Float64:
		fmt.Fprintf(b, "f%d;", math.Float64bits(v.Float()))
	case reflect.String:
		fmt.Fprintf(b, "s%d:%s;", v.Len(), v.String())
	case reflect.Slice, reflect.Array:
		if v.Kind() == reflect.Slice && v.Type().Elem().Kind() == reflect.Uint8 {
			fmt.Fprintf(b, "y%d:", v.Len())
			b.Write(v.Bytes())
			b.WriteString(";")
			return
		}
		fmt.Fprintf(b, "l%d[", v.Len())
		for i := range v.Len() {
			canonical(b, v.Index(i))
		}
		b.WriteString("];")
	case reflect.Map:
		type kv struct {
			key string
			val reflect.Value
		}
		entries := make([]kv, 0, v.Len())
		for it := v.MapRange(); it.Next(); {
			var kb bytes.Buffer
			canonical(&kb, it.Key())
			entries = append(entries, kv{kb.String(), it.Value()})
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].key < entries[j].key })
		fmt.Fprintf(b, "m%d{", len(entries))
		for _, e := range entries {
			b.WriteString(e.key)
			canonical(b, e.val)
		}
		b.WriteString("};")
	case reflect.Struct:
		fmt.Fprintf(b, "S%s{", v.Type().String())
		for i := range v.NumField() {
			f := v.Type().Field(i)
			if !f.IsExported() {
				continue
			}
			fmt.Fprintf(b, "%s=", f.Name)
			canonical(b, v.Field(i))
		}
		b.WriteString("};")
	default:
		// channels, functions: their identity is not a value
		fmt.Fprintf(b, "x%s;", v.Type().String())
	}
}
