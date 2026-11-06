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
	"fmt"
	"math/big"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/iskorotkov/avro/v2"
)

// This file checks that every decimal value Serde.Marshal encodes fits its
// schema exactly.
//
// The codec writes a decimal as the unscaled integer Num*10^scale/Denom,
// computed with integer division, and does not check the remainder. A value
// with more fractional digits than the schema's scale was therefore truncated
// without an error: 1.123456789012 against decimal(31,10) was written as
// 1.1234567890. A negative value was rounded toward negative infinity instead
// (big.Int.Div is Euclidean), and a value with no finite decimal expansion,
// such as 1/3, was cut at the scale. The codec does check the precision, but
// only of the already-truncated integer.
//
// This applies to every schema, not only the decimal(31,10) that
// SerdeForType infers for a *big.Rat: a parsed or registered schema with a
// smaller scale truncated the same way. So the check runs in Serde.Marshal,
// before the codec sees the value, and not in the extractor.
//
// Invariant 6 (schema handling never silently mangles data): Marshal encodes
// a decimal only if decoding it gives back exactly the same number. Otherwise
// it returns ErrDecimalNotRepresentable, naming the field, the precision and
// scale the value needs and the ones the schema has (not the value), and a
// fix.

// schemaContainsDecimal reports whether schema contains a decimal logical
// type anywhere below it, following named-type references. The result is
// memoized on Serde.hasDecimal, so schemas without decimals skip the check.
func schemaContainsDecimal(schema avro.Schema) bool {
	return schemaContainsDecimalSeen(schema, make(map[avro.Schema]bool))
}

func schemaContainsDecimalSeen(schema avro.Schema, seen map[avro.Schema]bool) bool {
	if seen[schema] {
		// Already visited, on this path (a recursive reference) or another
		// one. Returning false is safe: the search stops at the first true,
		// so a schema visited before did not contain a decimal, or is still
		// being searched further up.
		return false
	}
	seen[schema] = true

	if _, ok := decimalOf(schema); ok {
		return true
	}
	switch s := schema.(type) {
	case *avro.ArraySchema:
		return schemaContainsDecimalSeen(s.Items(), seen)
	case *avro.MapSchema:
		return schemaContainsDecimalSeen(s.Values(), seen)
	case *avro.RecordSchema:
		for _, field := range s.Fields() {
			if schemaContainsDecimalSeen(field.Type(), seen) {
				return true
			}
		}
		return false
	case *avro.UnionSchema:
		for _, typ := range s.Types() {
			if schemaContainsDecimalSeen(typ, seen) {
				return true
			}
		}
		return false
	case *avro.RefSchema:
		return schemaContainsDecimalSeen(s.Schema(), seen)
	default:
		return false
	}
}

// decimalOf returns the decimal logical schema of a bytes or fixed schema.
func decimalOf(schema avro.Schema) (*avro.DecimalLogicalSchema, bool) {
	var ls avro.LogicalSchema
	switch s := schema.(type) {
	case *avro.PrimitiveSchema:
		if s.Type() != avro.Bytes {
			return nil, false
		}
		ls = s.Logical()
	case *avro.FixedSchema:
		ls = s.Logical()
	default:
		return nil, false
	}
	dec, ok := ls.(*avro.DecimalLogicalSchema)
	return dec, ok && dec != nil
}

var ratType = reflect.TypeFor[big.Rat]()

// checkDecimals walks v, the value Serde.Marshal is about to hand to the
// codec, together with schema, and returns ErrDecimalNotRepresentable for the
// first decimal value that the codec would not encode exactly.
//
// It follows the codec's own rules for which part of the value goes with
// which part of the schema (record fields by name or by the avro struct tag,
// a union value in the named form {"branch": value}, a pointer in a nullable
// union, and a bare *big.Rat in a union resolved to its bytes.decimal
// branch). Where the codec cannot encode the value at all, the walk does not
// descend: the codec returns its own error there.
func checkDecimals(schema avro.Schema, v any) error {
	var w decimalWalker
	return w.value(schema, reflect.ValueOf(v))
}

// decimalWalker holds the state of one checkDecimals walk: the path to the
// value being checked, kept as a stack so descending does not allocate, and
// scratch integers for the arithmetic.
type decimalWalker struct {
	path    []pathElem
	scratch [2]big.Int
}

// pathElem is one step of the path to a value: a field or map key, or an
// array index.
type pathElem struct {
	key        string
	index      int
	isIndex    bool
	isMapValue bool // a map key is data, so the path does not name it
}

func (w *decimalWalker) push(e pathElem) { w.path = append(w.path, e) }
func (w *decimalWalker) pop()            { w.path = w.path[:len(w.path)-1] }

func (w *decimalWalker) value(schema avro.Schema, v reflect.Value) error {
	v = derefDecimalValue(v)
	if !v.IsValid() {
		return nil
	}

	if dec, ok := decimalOf(schema); ok {
		r, ok := asRat(v)
		if !ok {
			return nil // not a decimal value; the codec rejects it itself
		}
		return w.fits(r, dec)
	}

	switch s := schema.(type) {
	case *avro.RefSchema:
		return w.value(s.Schema(), v)
	case *avro.RecordSchema:
		return w.record(s, v)
	case *avro.ArraySchema:
		if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
			return nil
		}
		for i := range v.Len() {
			w.push(pathElem{index: i, isIndex: true})
			if err := w.value(s.Items(), v.Index(i)); err != nil {
				return err
			}
			w.pop()
		}
		return nil
	case *avro.MapSchema:
		if v.Kind() != reflect.Map {
			return nil
		}
		keys := v.MapKeys()
		if v.Type().Key().Kind() == reflect.String {
			keys = sortedMapKeys(v) // report the same field on every run
		}
		for _, k := range keys {
			w.push(pathElem{isMapValue: true})
			if err := w.value(s.Values(), v.MapIndex(k)); err != nil {
				return err
			}
			w.pop()
		}
		return nil
	case *avro.UnionSchema:
		return w.union(s, v)
	default:
		return nil
	}
}

// derefDecimalValue follows pointers and interfaces down to the concrete
// value, the one the codec picks its encoder for. A nil one is invalid.
func derefDecimalValue(v reflect.Value) reflect.Value {
	for v.IsValid() && (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) {
		if v.IsNil() {
			return reflect.Value{}
		}
		v = v.Elem()
	}
	return v
}

func (w *decimalWalker) record(s *avro.RecordSchema, v reflect.Value) error {
	var lookup func(name string) (reflect.Value, bool)
	switch {
	case v.Kind() == reflect.Map && v.Type().Key().Kind() == reflect.String:
		if m, ok := asStringMap(v.Interface()); ok {
			// The common case, opencdc.StructuredData: index the Go map
			// directly, without reflect allocating a key.
			lookup = func(name string) (reflect.Value, bool) {
				fv, ok := m[name]
				return reflect.ValueOf(fv), ok
			}
			break
		}
		keyType := v.Type().Key()
		lookup = func(name string) (reflect.Value, bool) {
			fv := v.MapIndex(reflect.ValueOf(name).Convert(keyType))
			return fv, fv.IsValid()
		}
	case v.Kind() == reflect.Struct:
		fields := structFieldsByAvroName(v.Type())
		lookup = func(name string) (reflect.Value, bool) {
			index, ok := fields[name]
			if !ok {
				return reflect.Value{}, false
			}
			fv, err := v.FieldByIndexErr(index)
			return fv, err == nil // err: the field is in a nil embedded struct pointer
		}
	default:
		return nil // the codec cannot encode it as a record and says so
	}
	for _, f := range s.Fields() {
		fv, ok := lookup(f.Name())
		if !ok {
			continue // the codec writes the field's default, or reports it missing
		}
		w.push(pathElem{key: f.Name()})
		if err := w.value(f.Type(), fv); err != nil {
			return err
		}
		w.pop()
	}
	return nil
}

// structFieldsByAvroName returns the index (for reflect.Value.FieldByIndex)
// of each exported field of the struct type t, by the name the codec matches
// it with: the avro tag up to its first comma, or the Go field name. Like the
// codec's describeStruct, it includes the fields of embedded structs and
// struct pointers, breadth first, and the first field found with a name
// wins. It works on the type, as the codec does, so the result is cached per
// type and a struct value costs no allocation.
func structFieldsByAvroName(t reflect.Type) map[string][]int {
	if c, ok := structFieldsCache.Load(t); ok {
		return c.(map[string][]int) //nolint:forcetypeassert // only this function stores
	}
	type embedded struct {
		t     reflect.Type
		index []int
	}
	out := make(map[string][]int)
	visited := make(map[reflect.Type]bool)
	next := []embedded{{t: t}}
	for len(next) > 0 {
		curr := next
		next = nil
		for _, e := range curr {
			if visited[e.t] {
				continue
			}
			visited[e.t] = true
			for i := range e.t.NumField() {
				sf := e.t.Field(i)
				index := append(append(make([]int, 0, len(e.index)+1), e.index...), i)
				if sf.Anonymous {
					ft := sf.Type
					if ft.Kind() == reflect.Pointer {
						ft = ft.Elem()
					}
					if ft.Kind() == reflect.Struct {
						next = append(next, embedded{t: ft, index: index})
					}
					continue
				}
				if !sf.IsExported() {
					continue
				}
				name := sf.Name
				if tag, ok := sf.Tag.Lookup("avro"); ok {
					name, _, _ = strings.Cut(tag, ",")
				}
				if _, ok := out[name]; !ok {
					out[name] = index
				}
			}
		}
	}
	c, _ := structFieldsCache.LoadOrStore(t, out)
	return c.(map[string][]int) //nolint:forcetypeassert // only this function stores
}

// structFieldsCache maps a struct reflect.Type to structFieldsByAvroName's
// result. It grows with the number of distinct Go struct types encoded,
// which a program fixes at compile time.
var structFieldsCache sync.Map

func (w *decimalWalker) union(s *avro.UnionSchema, v reflect.Value) error {
	// The named form: the codec reads every map[string]any in a union
	// position as {"branch": value}.
	if v.Kind() == reflect.Map && v.Type().Key().Kind() == reflect.String &&
		v.Type().Elem().Kind() == reflect.Interface {
		if v.Len() != 1 {
			return nil // null, or an error the codec reports
		}
		iter := v.MapRange()
		iter.Next()
		branch, _ := s.Types().Get(iter.Key().String())
		if branch == nil {
			return nil // the codec reports the unknown branch
		}
		return w.value(branch, iter.Value())
	}

	// A bare decimal resolves to the union's bytes.decimal branch.
	if _, ok := asRat(v); ok {
		branch := bytesDecimalBranch(s)
		if branch == nil {
			if !s.Nullable() {
				return nil // the codec reports the unknown branch
			}
			branch = nonNullBranch(s)
		}
		return w.value(branch, v)
	}

	// Any other value is encoded with the non-null branch of a nullable
	// union. Elsewhere the codec resolves it by registered Go type, and a
	// Serde registers none beyond the codec's primitives, so no decimal can
	// be below it.
	if s.Nullable() {
		return w.value(nonNullBranch(s), v)
	}
	return nil
}

// bytesDecimalBranch returns the branch the codec encodes a bare *big.Rat
// with, the one its union resolver names bytes.decimal, or nil.
func bytesDecimalBranch(s *avro.UnionSchema) avro.Schema {
	for _, t := range s.Types() {
		if p, ok := t.(*avro.PrimitiveSchema); ok && p.Type() == avro.Bytes {
			if ls := p.Logical(); ls != nil && ls.Type() == avro.Decimal {
				return t
			}
		}
	}
	return nil
}

func nonNullBranch(s *avro.UnionSchema) avro.Schema {
	_, typ := s.Indices()
	return s.Types()[typ]
}

// asRat returns v as a *big.Rat if its type is big.Rat or convertible to it,
// the same types the codec encodes as a decimal.
func asRat(v reflect.Value) (*big.Rat, bool) {
	if !v.Type().ConvertibleTo(ratType) {
		return nil, false
	}
	if v.CanAddr() {
		return v.Addr().Convert(reflect.PointerTo(ratType)).Interface().(*big.Rat), true //nolint:forcetypeassert // converted to *big.Rat just above
	}
	r := v.Convert(ratType).Interface().(big.Rat) //nolint:forcetypeassert // converted to big.Rat just above
	return &r, true
}

// fits returns nil if r encodes exactly as dec: it has no more fractional
// digits than the scale, and no more significant digits than the precision
// once scaled.
func (w *decimalWalker) fits(r *big.Rat, dec *avro.DecimalLogicalSchema) error {
	if decimalFits(r, dec.Precision(), dec.Scale(), &w.scratch) {
		return nil
	}

	// The message names the value's position and size, never the value
	// itself, so it can be shown wherever record values cannot. It ends with
	// the fix: a schema wide enough for both the old schema's range and the
	// value, or rounding upstream when the value has too many fractional
	// digits.
	schemaPrec, schemaScale := dec.Precision(), dec.Scale()
	scale, ok := neededScale(r.Denom())
	if !ok {
		return fmt.Errorf(
			"%w: field %s: the value needs a non-terminating decimal expansion, the schema is decimal(%d,%d); "+
				"fix: round the value to at most %d fractional digits before encoding",
			ErrDecimalNotRepresentable, formatPath(w.path), schemaPrec, schemaScale, schemaScale,
		)
	}
	exact := new(big.Int).Mul(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil), r.Num())
	exact.Quo(exact, r.Denom())
	prec := max(numDigits(exact), scale)
	wideScale := max(scale, schemaScale)
	widePrec := wideScale + max(prec-scale, schemaPrec-schemaScale)
	fix := fmt.Sprintf("use a schema with at least %d integer and %d fractional digits, such as decimal(%d,%d)",
		widePrec-wideScale, wideScale, widePrec, wideScale)
	if scale > schemaScale {
		fix += fmt.Sprintf(", or round the value to at most %d fractional digits before encoding", schemaScale)
	}
	return fmt.Errorf(
		"%w: field %s: the value needs precision %d and scale %d, the schema is decimal(%d,%d); fix: %s",
		ErrDecimalNotRepresentable, formatPath(w.path), prec, scale, schemaPrec, schemaScale, fix,
	)
}

// decimalFits reports whether r encodes exactly as decimal(prec, scale). It
// runs for every decimal value Marshal encodes, so it avoids allocating
// where it can.
//
// r is a reduced fraction n/d. It has a finite expansion of at most scale
// digits exactly when d divides 10^scale, and its unscaled value r*10^scale
// has at most prec digits exactly when |r| < 10^(prec-scale), that is when
// |n| < 10^(prec-scale) * d. scratch is overwritten.
func decimalFits(r *big.Rat, prec, scale int, scratch *[2]big.Int) bool {
	if prec < scale {
		// Not a valid schema (the codec's parser rejects it); decide the
		// slow way rather than assume.
		unscaled, rem := new(big.Int).QuoRem(new(big.Int).Mul(bigPow10(scale), r.Num()), r.Denom(), new(big.Int))
		return rem.Sign() == 0 && numDigits(unscaled) <= prec
	}
	if r.IsInt() {
		return r.Num().CmpAbs(bigPow10(prec-scale)) < 0
	}
	q, rem := &scratch[0], &scratch[1]
	if q.QuoRem(bigPow10(scale), r.Denom(), rem); rem.Sign() != 0 {
		return false
	}
	return r.Num().CmpAbs(q.Mul(bigPow10(prec-scale), r.Denom())) < 0
}

// bigPow10Cache holds 10^0 through 10^76, which covers every precision the
// common databases allow. The values are shared: never modify one.
var bigPow10Cache = func() [77]*big.Int {
	var c [77]*big.Int
	c[0] = big.NewInt(1)
	for i := 1; i < len(c); i++ {
		c[i] = new(big.Int).Mul(c[i-1], big.NewInt(10))
	}
	return c
}()

// bigPow10Large caches 10^n for n above 76, computed on first use. Only
// call bigPow10 with an exponent taken from a schema (a precision, a scale
// or their difference), so the cache grows with the schemas in use, not with
// the data; an exponent derived from a value is computed per call instead
// (see fits).
var bigPow10Large sync.Map // int -> *big.Int

// bigPow10 returns 10^n for a schema-derived n. The result is shared and
// must not be modified.
func bigPow10(n int) *big.Int {
	if n < len(bigPow10Cache) {
		return bigPow10Cache[n]
	}
	if c, ok := bigPow10Large.Load(n); ok {
		return c.(*big.Int) //nolint:forcetypeassert // only this function stores
	}
	c, _ := bigPow10Large.LoadOrStore(n, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil))
	return c.(*big.Int) //nolint:forcetypeassert // only this function stores
}

// neededScale returns the number of fractional digits of a reduced fraction
// with denominator d, which is finite only if d has no prime factor other
// than 2 and 5.
func neededScale(d *big.Int) (int, bool) {
	if d.Sign() <= 0 {
		return 0, false
	}
	twos := int(d.TrailingZeroBits())
	n := new(big.Int).Rsh(d, uint(twos))
	fives := 0
	five := big.NewInt(5)
	q, m := new(big.Int), new(big.Int)
	for {
		q.QuoRem(n, five, m)
		if m.Sign() != 0 {
			break
		}
		n.Set(q)
		fives++
	}
	if n.Cmp(big.NewInt(1)) != 0 {
		return 0, false
	}
	return max(twos, fives), true
}

// numDigits returns the number of decimal digits of |i|; zero has one.
func numDigits(i *big.Int) int {
	return len(new(big.Int).Abs(i).String())
}

// formatPath renders path as a quoted dotted field path, for example
// "a.b[2].c[*]" (the value of some entry of the map c), or <root> for the
// top-level value.
func formatPath(path []pathElem) string {
	if len(path) == 0 {
		return "<root>"
	}
	var sb strings.Builder
	for i, p := range path {
		if p.isIndex {
			sb.WriteString("[" + strconv.Itoa(p.index) + "]")
			continue
		}
		if p.isMapValue {
			sb.WriteString("[*]")
			continue
		}
		if i > 0 {
			sb.WriteByte('.')
		}
		sb.WriteString(p.key)
	}
	return strconv.Quote(sb.String())
}
