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
	"fmt"
	"math"
	"strconv"
	"strings"

	"pgregory.net/rapid"
)

type kind int

const (
	kBool kind = iota
	kInt
	kLong
	kFloat
	kDouble
	kString
	kBytes
	kBytesDecimal
	kFixed
	kFixedDecimal
	kEnum
	kRecord
	kArray
	kMap
	kNullUnion
	kRef
)

// Type is one generated Avro type. A named type (fixed, enum, record) is
// written out in full where it is defined. A reference to it is written as
// the bare name. A Type is immutable once a generator returns it, and safe
// to use from several goroutines.
type Type struct {
	kind    kind
	name    string // named types and references
	size    int    // fixed
	prec    int    // decimals
	scale   int    // decimals
	symbols []string
	fields  []field
	elem    *Type // array items, map values, the non-null branch of a union
	def     *Type // kRef: the definition it refers to

	alwaysNull bool // kNullUnion: Value always returns nil, see Options
}

type field struct {
	name string
	typ  *Type
}

// Options limit what the generators emit, so a property can leave out a
// shape that has a separately tracked bug instead of failing on it. The
// zero value emits everything.
type Options struct {
	// NoNullableNamed makes Value use null for every nullable union whose
	// branch is a named type. Serde.Unmarshal returns those values bare and
	// Serde.Marshal only accepts them in the codec's named form
	// (ConduitIO/conduit-commons#306), so a decode-then-encode property
	// leaves them out until that is fixed. An encode-then-decode property
	// does not need it.
	NoNullableNamed bool
}

// Schema returns the Avro schema of t as JSON.
func (t *Type) Schema() string {
	var sb strings.Builder
	t.writeSchema(&sb)
	return sb.String()
}

// String returns the schema.
func (t *Type) String() string { return t.Schema() }

// GoString returns the schema, so rapid's draw log (which prints with %#v)
// shows the schema of a failing case instead of a struct dump.
func (t *Type) GoString() string { return t.Schema() }

var primitiveNames = map[kind]string{
	kBool:   `"boolean"`,
	kInt:    `"int"`,
	kLong:   `"long"`,
	kFloat:  `"float"`,
	kDouble: `"double"`,
	kString: `"string"`,
	kBytes:  `"bytes"`,
}

func (t *Type) writeSchema(sb *strings.Builder) {
	if name, ok := primitiveNames[t.kind]; ok {
		sb.WriteString(name)
		return
	}
	switch t.kind { //nolint:exhaustive // primitives are handled above
	case kBytesDecimal:
		fmt.Fprintf(sb, `{"type":"bytes","logicalType":"decimal","precision":%d,"scale":%d}`, t.prec, t.scale)
	case kFixed:
		fmt.Fprintf(sb, `{"type":"fixed","name":%q,"size":%d}`, t.name, t.size)
	case kFixedDecimal:
		fmt.Fprintf(sb, `{"type":"fixed","name":%q,"size":%d,"logicalType":"decimal","precision":%d,"scale":%d}`,
			t.name, t.size, t.prec, t.scale)
	case kEnum:
		fmt.Fprintf(sb, `{"type":"enum","name":%q,"symbols":["%s"]}`, t.name, strings.Join(t.symbols, `","`))
	case kRecord:
		fmt.Fprintf(sb, `{"type":"record","name":%q,"fields":[`, t.name)
		for i, f := range t.fields {
			if i > 0 {
				sb.WriteByte(',')
			}
			fmt.Fprintf(sb, `{"name":%q,"type":`, f.name)
			f.typ.writeSchema(sb)
			sb.WriteByte('}')
		}
		sb.WriteString(`]}`)
	case kArray:
		sb.WriteString(`{"type":"array","items":`)
		t.elem.writeSchema(sb)
		sb.WriteByte('}')
	case kMap:
		sb.WriteString(`{"type":"map","values":`)
		t.elem.writeSchema(sb)
		sb.WriteByte('}')
	case kNullUnion:
		sb.WriteString(`["null",`)
		t.elem.writeSchema(sb)
		sb.WriteByte(']')
	case kRef:
		sb.WriteString(strconv.Quote(t.name))
	}
}

// isNamed reports whether t is a named type.
func (t *Type) isNamed() bool {
	switch t.kind { //nolint:exhaustive // the rest are not named
	case kFixed, kFixedDecimal, kEnum, kRecord:
		return true
	}
	return false
}

// resolved returns the definition behind a reference, or t itself.
func (t *Type) resolved() *Type {
	if t.kind == kRef {
		return t.def
	}
	return t
}

var primitiveKinds = []kind{kBool, kInt, kLong, kFloat, kDouble, kString, kBytes, kBytesDecimal}

// Primitive returns a generator of the primitive Avro types, plus the
// decimal logical type on bytes.
func Primitive() *rapid.Generator[*Type] {
	return rapid.Custom(drawPrimitive)
}

func drawPrimitive(t *rapid.T) *Type {
	typ := &Type{kind: rapid.SampledFrom(primitiveKinds).Draw(t, "primitive")}
	if typ.kind == kBytesDecimal {
		// At most 18, so 10^prec fits in an int64 when drawing values.
		typ.prec = rapid.IntRange(1, 18).Draw(t, "precision")
		typ.scale = rapid.IntRange(0, typ.prec).Draw(t, "scale")
	}
	return typ
}

// drawNamedDef draws a definition for the named type name.
func drawNamedDef(t *rapid.T, name string) *Type {
	switch rapid.IntRange(0, 3).Draw(t, "named kind") {
	case 0:
		return &Type{kind: kFixed, name: name, size: rapid.IntRange(1, 8).Draw(t, "fixed size")}
	case 1:
		typ := &Type{kind: kFixedDecimal, name: name, size: rapid.IntRange(4, 8).Draw(t, "fixed size")}
		// The largest precision a size-byte two's complement integer holds.
		maxPrec := int(math.Floor(float64(8*typ.size-1) * math.Log10(2)))
		typ.prec = rapid.IntRange(1, min(maxPrec, 18)).Draw(t, "precision")
		typ.scale = rapid.IntRange(0, typ.prec).Draw(t, "scale")
		return typ
	case 2:
		syms := rapid.Permutation([]string{"A", "B", "C", "D"}).Draw(t, "symbols")
		return &Type{kind: kEnum, name: name, symbols: syms[:rapid.IntRange(2, 4).Draw(t, "symbol count")]}
	default:
		names := rapid.Permutation([]string{"p", "q", "s", "u"}).Draw(t, "field names")
		typ := &Type{kind: kRecord, name: name}
		for _, n := range names[:rapid.IntRange(1, 4).Draw(t, "field count")] {
			typ.fields = append(typ.fields, field{name: n, typ: drawPrimitive(t)})
		}
		return typ
	}
}

// container is where a field refers to a named type.
type container int

const (
	cBare container = iota
	cArray
	cMap
	cArrayOfArray
	cMapOfArray
	cNullable
	containerCount
)

func (c container) wrap(ref *Type) *Type {
	switch c {
	case cArray:
		return &Type{kind: kArray, elem: ref}
	case cMap:
		return &Type{kind: kMap, elem: ref}
	case cArrayOfArray:
		return &Type{kind: kArray, elem: &Type{kind: kArray, elem: ref}}
	case cMapOfArray:
		return &Type{kind: kMap, elem: &Type{kind: kArray, elem: ref}}
	case cNullable:
		return &Type{kind: kNullUnion, elem: ref}
	case cBare, containerCount:
	}
	return ref
}

// layout is the structure of a named schema without the definitions of its
// named types: how many there are, and where each field refers to one.
type layout struct {
	defs   int
	fields []layoutField
}

type layoutField struct {
	primitive *Type // a field of primitive type, or nil
	def       int   // otherwise: which named type it refers to
	in        container
}

func drawLayout(t *rapid.T) layout {
	l := layout{defs: rapid.IntRange(1, 3).Draw(t, "named types")}
	for range rapid.IntRange(1, 5).Draw(t, "fields") {
		if rapid.IntRange(0, 3).Draw(t, "primitive field") == 0 {
			l.fields = append(l.fields, layoutField{primitive: drawPrimitive(t)})
			continue
		}
		l.fields = append(l.fields, layoutField{
			def: rapid.IntRange(0, l.defs-1).Draw(t, "refers to"),
			in:  container(rapid.IntRange(0, int(containerCount)-1).Draw(t, "container")),
		})
	}
	return l
}

// instantiate builds the record schema recName from l, drawing a fresh
// definition for each named type.
func (l layout) instantiate(t *rapid.T, recName string, opts Options) *Type {
	root := &Type{kind: kRecord, name: recName}
	defs := make([]*Type, l.defs)
	for i := range defs {
		defs[i] = drawNamedDef(t, "n"+strconv.Itoa(i))
		// The first occurrence defines the type. It sits in a nullable
		// field so the record value can leave it out.
		root.fields = append(root.fields, field{
			name: "d" + strconv.Itoa(i),
			typ:  &Type{kind: kNullUnion, elem: defs[i]},
		})
	}
	for j, lf := range l.fields {
		ft := lf.primitive
		if ft == nil {
			d := defs[lf.def]
			ft = lf.in.wrap(&Type{kind: kRef, name: d.name, def: d})
		}
		root.fields = append(root.fields, field{name: "u" + strconv.Itoa(j), typ: ft})
	}
	if opts.NoNullableNamed {
		disableNullableNamed(root)
	}
	return root
}

// NamedSchema returns a generator of record schemas named recName. Each
// defines one to three named types n0, n1, n2 in nullable fields d0, d1, d2
// and refers to them by name, bare or inside arrays, maps, arrays of arrays,
// maps of arrays and nullable unions, from fields u0, u1, and so on. Some
// u fields are primitives instead.
func NamedSchema(recName string, opts Options) *rapid.Generator[*Type] {
	return rapid.Custom(func(t *rapid.T) *Type {
		return drawLayout(t).instantiate(t, recName, opts)
	})
}

// NamedSchemaFamily returns a generator of n record schemas named
// prefix+"0", prefix+"1", and so on, that share one layout: the same type
// names, referenced from the same places, each defined independently. Two
// members can define n0 as different types, which is the situation of two
// versions of one subject, or two subjects that share a type name, in one
// process (ConduitIO/conduit-commons#305). Members can also come out
// identical, which a property must allow.
func NamedSchemaFamily(prefix string, n int, opts Options) *rapid.Generator[[]*Type] {
	return rapid.Custom(func(t *rapid.T) []*Type {
		l := drawLayout(t)
		out := make([]*Type, n)
		for i := range out {
			out[i] = l.instantiate(t, prefix+strconv.Itoa(i), opts)
		}
		return out
	})
}

// disableNullableNamed marks every nullable union whose branch is a named
// type so that Value returns nil for it.
func disableNullableNamed(t *Type) {
	switch t.kind { //nolint:exhaustive // only containers hold nullable unions
	case kRecord:
		for _, f := range t.fields {
			disableNullableNamed(f.typ)
		}
	case kArray, kMap:
		disableNullableNamed(t.elem)
	case kNullUnion:
		if t.elem.resolved().isNamed() {
			t.alwaysNull = true
		}
		disableNullableNamed(t.elem)
	}
}
