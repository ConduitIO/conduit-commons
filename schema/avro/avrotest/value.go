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

package avrotest

import (
	"bytes"
	"fmt"
	"math"
	"math/big"
	"reflect"

	"github.com/google/go-cmp/cmp"
	"pgregory.net/rapid"
)

// Value returns a generator of values of type t, in the shape Serde.Marshal
// accepts:
//
//   - boolean: bool; int: int; long: int64; float: float32; double:
//     float64; string: string; bytes: []byte.
//   - decimal (bytes or fixed): *big.Rat, exactly representable at the
//     schema's scale and within its precision.
//   - fixed: a [size]byte array; enum: string, one of the symbols.
//   - record: map[string]any with every field; map: map[string]any; array:
//     []any. Arrays and maps hold zero to three elements.
//   - nullable union: nil, or the branch's value. A named branch (fixed,
//     enum, record) is passed in the codec's named form, map[string]any{name:
//     value}, because the codec resolves a bare union value by its Go type and
//     cannot tell an enum symbol from a string or a fixed from any other byte
//     array.
//
// Floats are finite. NaN would fail every equality check, and the codec
// writes IEEE 754 bits as they are, so it adds no coverage worth a special
// comparer.
func (t *Type) Value() *rapid.Generator[any] {
	return rapid.Custom(t.draw)
}

//nolint:funlen // one case per kind reads better than a table of closures
func (t *Type) draw(rt *rapid.T) any {
	switch t.kind {
	case kBool:
		return rapid.Bool().Draw(rt, "boolean")
	case kInt:
		return int(rapid.Int32().Draw(rt, "int"))
	case kLong:
		return rapid.Int64().Draw(rt, "long")
	case kFloat:
		return rapid.Float32Range(-math.MaxFloat32, math.MaxFloat32).Draw(rt, "float")
	case kDouble:
		return rapid.Float64Range(-math.MaxFloat64, math.MaxFloat64).Draw(rt, "double")
	case kString:
		return rapid.String().Draw(rt, "string")
	case kBytes:
		return rapid.SliceOfN(rapid.Byte(), 0, 8).Draw(rt, "bytes")
	case kFixed:
		b := rapid.SliceOfN(rapid.Byte(), t.size, t.size).Draw(rt, "fixed")
		v := reflect.New(reflect.ArrayOf(t.size, reflect.TypeFor[byte]())).Elem()
		reflect.Copy(v, reflect.ValueOf(b))
		return v.Interface()
	case kBytesDecimal, kFixedDecimal:
		limit := pow10(t.prec) - 1
		unscaled := big.NewInt(rapid.Int64Range(-limit, limit).Draw(rt, "unscaled"))
		den := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(t.scale)), nil)
		return new(big.Rat).SetFrac(unscaled, den)
	case kEnum:
		return rapid.SampledFrom(t.symbols).Draw(rt, "enum")
	case kRecord:
		m := make(map[string]any, len(t.fields))
		for _, f := range t.fields {
			m[f.name] = f.typ.draw(rt)
		}
		return m
	case kArray:
		a := make([]any, rapid.IntRange(0, 3).Draw(rt, "array length"))
		for i := range a {
			a[i] = t.elem.draw(rt)
		}
		return a
	case kMap:
		keys := rapid.SliceOfNDistinct(rapid.String(), 0, 3, rapid.ID[string]).Draw(rt, "map keys")
		m := make(map[string]any, len(keys))
		for _, k := range keys {
			m[k] = t.elem.draw(rt)
		}
		return m
	case kNullUnion:
		// Draw even when alwaysNull, so a value made only of always-null
		// unions still reads from rapid's input, which rapid requires.
		isNull := rapid.Bool().Draw(rt, "null")
		if t.alwaysNull || isNull {
			return nil
		}
		v := t.elem.draw(rt)
		if def := t.elem.resolved(); def.isNamed() {
			return map[string]any{def.name: v}
		}
		return v
	case kRef:
		return t.def.draw(rt)
	}
	panic(fmt.Sprintf("avrotest: unknown kind %d", t.kind))
}

// Decoded returns what Serde.Unmarshal returns for v, a value drawn from
// t.Value(). It is v with every named-form union value unwrapped: the
// codec decodes a nullable union as a record field to its bare branch value
// (ConduitIO/conduit-commons#306). Everything else decodes to the Go type
// Value draws.
//
// Decoded copies every map, slice and byte slice, so the result stays valid
// after Serde.Marshal mutates v. Call it before Marshal.
func (t *Type) Decoded(v any) any {
	if v == nil {
		return nil
	}
	switch t.kind { //nolint:exhaustive // the remaining kinds are scalars
	case kRecord:
		in := v.(map[string]any) //nolint:forcetypeassert // Value draws a map for a record
		out := make(map[string]any, len(in))
		for _, f := range t.fields {
			out[f.name] = f.typ.Decoded(in[f.name])
		}
		return out
	case kMap:
		in := v.(map[string]any) //nolint:forcetypeassert // Value draws a map for a map
		out := make(map[string]any, len(in))
		for k, e := range in {
			out[k] = t.elem.Decoded(e)
		}
		return out
	case kArray:
		in := v.([]any) //nolint:forcetypeassert // Value draws a []any for an array
		out := make([]any, len(in))
		for i, e := range in {
			out[i] = t.elem.Decoded(e)
		}
		return out
	case kNullUnion:
		if def := t.elem.resolved(); def.isNamed() {
			//nolint:forcetypeassert // Value draws the named form for a named branch
			v = v.(map[string]any)[def.name]
		}
		return t.elem.Decoded(v)
	case kRef:
		return t.def.Decoded(v)
	case kBytes:
		return bytes.Clone(v.([]byte)) //nolint:forcetypeassert // Value draws []byte for bytes
	case kBytesDecimal, kFixedDecimal:
		return new(big.Rat).Set(v.(*big.Rat)) //nolint:forcetypeassert // Value draws *big.Rat for a decimal
	}
	return v
}

// CmpOptions are the go-cmp options a property needs to compare a decoded
// value with Decoded: decimals compare by value, not by representation.
func CmpOptions() cmp.Options {
	return cmp.Options{
		cmp.Comparer(func(a, b *big.Rat) bool { return a.Cmp(b) == 0 }),
	}
}

func pow10(n int) int64 {
	v := int64(1)
	for range n {
		v *= 10
	}
	return v
}
