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

// This file holds a random Avro schema and value generator for the property
// tests. It builds record schemas that define named types (fixed, decimal
// fixed, enum, record) once and then refer to them by name from inside
// arrays, maps and unions. That shape is where a cache keyed by a schema's
// string form goes wrong, because the string form of a reference is only the
// name (ConduitIO/conduit-commons#305).
//
// The generator is deterministic for a given seed, so a failing case can be
// replayed from the seed printed in the failure message.

import (
	"fmt"
	"math"
	"math/big"
	"math/rand/v2"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

type genKind int

const (
	gNull genKind = iota
	gBool
	gInt
	gLong
	gFloat
	gDouble
	gString
	gBytes
	gBytesDecimal
	gFixed
	gFixedDecimal
	gEnum
	gRecord
	gArray
	gMap
	gNullUnion
	gRef
)

// genType describes one Avro type. A named type (fixed, enum, record) is
// written out in full where it is defined; a gRef points at that definition
// and is written as the bare name.
type genType struct {
	kind    genKind
	name    string // named types and references
	size    int    // fixed
	prec    int    // decimals
	scale   int    // decimals
	symbols []string
	fields  []genField
	elem    *genType // array items, map values, the non-null branch of a union
	def     *genType // gRef: the definition it refers to

	bare bool // gNullUnion: value() returns a named branch bare, see genOptions
}

type genField struct {
	name string
	typ  *genType
}

func (t *genType) json() string {
	switch t.kind {
	case gNull:
		return `"null"`
	case gBool:
		return `"boolean"`
	case gInt:
		return `"int"`
	case gLong:
		return `"long"`
	case gFloat:
		return `"float"`
	case gDouble:
		return `"double"`
	case gString:
		return `"string"`
	case gBytes:
		return `"bytes"`
	case gBytesDecimal:
		return fmt.Sprintf(`{"type":"bytes","logicalType":"decimal","precision":%d,"scale":%d}`, t.prec, t.scale)
	case gFixed:
		return fmt.Sprintf(`{"type":"fixed","name":%q,"size":%d}`, t.name, t.size)
	case gFixedDecimal:
		return fmt.Sprintf(`{"type":"fixed","name":%q,"size":%d,"logicalType":"decimal","precision":%d,"scale":%d}`, t.name, t.size, t.prec, t.scale)
	case gEnum:
		return fmt.Sprintf(`{"type":"enum","name":%q,"symbols":["%s"]}`, t.name, strings.Join(t.symbols, `","`))
	case gRecord:
		fs := make([]string, len(t.fields))
		for i, f := range t.fields {
			fs[i] = fmt.Sprintf(`{"name":%q,"type":%s}`, f.name, f.typ.json())
		}
		return fmt.Sprintf(`{"type":"record","name":%q,"fields":[%s]}`, t.name, strings.Join(fs, ","))
	case gArray:
		return `{"type":"array","items":` + t.elem.json() + `}`
	case gMap:
		return `{"type":"map","values":` + t.elem.json() + `}`
	case gNullUnion:
		return `["null",` + t.elem.json() + `]`
	case gRef:
		return strconv.Quote(t.name)
	}
	panic(fmt.Sprintf("unknown kind %d", t.kind))
}

// isNamed reports whether t is a named type.
func (t *genType) isNamed() bool {
	switch t.kind { //nolint:exhaustive // the rest are not named
	case gFixed, gFixedDecimal, gEnum, gRecord:
		return true
	}
	return false
}

// resolved returns the definition behind a reference, or t itself.
func (t *genType) resolved() *genType {
	if t.kind == gRef {
		return t.def
	}
	return t
}

// value returns a random value of type t, in a shape Serde.Marshal
// accepts. A named type (fixed, enum, record) or a map inside a nullable
// union is passed in the codec's named form ({"name": value}) unless the
// union is marked bare, in which case it is passed bare, as Unmarshal
// returns it.
func (t *genType) value(r *rand.Rand) any {
	switch t.kind {
	case gNull:
		return nil
	case gBool:
		return r.IntN(2) == 1
	case gInt:
		return int(r.Int32()) - math.MaxInt32/2
	case gLong:
		return r.Int64() - math.MaxInt64/2
	case gFloat:
		return float32(r.IntN(1<<20)) / 8
	case gDouble:
		return float64(r.Int64N(1<<40)) / 16
	case gString:
		return pick(r, []string{"", "a", "héllo", "x y z", strings.Repeat("s", 40)})
	case gBytes:
		b := make([]byte, r.IntN(6))
		for i := range b {
			b[i] = byte(r.IntN(256))
		}
		return b
	case gFixed:
		v := reflect.New(reflect.ArrayOf(t.size, reflect.TypeFor[byte]())).Elem()
		for i := range t.size {
			v.Index(i).SetUint(uint64(r.IntN(256)))
		}
		return v.Interface()
	case gBytesDecimal, gFixedDecimal:
		// prec is at most 18, so 10^prec fits in an int64.
		unscaled := big.NewInt(r.Int64N(pow10(t.prec)))
		if r.IntN(2) == 1 {
			unscaled.Neg(unscaled)
		}
		den := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(t.scale)), nil)
		return new(big.Rat).SetFrac(unscaled, den)
	case gEnum:
		return pick(r, t.symbols)
	case gRecord:
		m := make(map[string]any, len(t.fields))
		for _, f := range t.fields {
			m[f.name] = f.typ.value(r)
		}
		return m
	case gArray:
		n := r.IntN(4)
		a := make([]any, n)
		for i := range a {
			a[i] = t.elem.value(r)
		}
		return a
	case gMap:
		n := r.IntN(4)
		m := make(map[string]any, n)
		for i := range n {
			m["k"+strconv.Itoa(i)] = t.elem.value(r)
		}
		return m
	case gNullUnion:
		if r.IntN(3) == 0 {
			return nil
		}
		v := t.elem.value(r)
		if t.bare {
			return v
		}
		switch e := t.elem.resolved(); {
		case e.isNamed():
			return map[string]any{e.name: v}
		case e.kind == gMap:
			return map[string]any{"map": v}
		}
		return v
	case gRef:
		return t.def.value(r)
	}
	panic(fmt.Sprintf("unknown kind %d", t.kind))
}

func pick[T any](r *rand.Rand, s []T) T { return s[r.IntN(len(s))] }

func pow10(n int) int64 {
	v := int64(1)
	for range n {
		v *= 10
	}
	return v
}

var primitiveKinds = []genKind{gBool, gInt, gLong, gFloat, gDouble, gString, gBytes, gBytesDecimal}

func genPrimitive(r *rand.Rand) *genType {
	t := &genType{kind: pick(r, primitiveKinds)}
	if t.kind == gBytesDecimal {
		t.prec = 1 + r.IntN(18)
		t.scale = r.IntN(t.prec + 1)
	}
	return t
}

// genNamedDef returns a definition for the named type name, drawn from r.
func genNamedDef(r *rand.Rand, name string) *genType {
	switch r.IntN(4) {
	case 0:
		return &genType{kind: gFixed, name: name, size: 1 + r.IntN(8)}
	case 1:
		t := &genType{kind: gFixedDecimal, name: name, size: 4 + r.IntN(5)}
		// The largest precision a size-byte two's complement integer holds.
		maxPrec := int(math.Floor(float64(8*t.size-1) * math.Log10(2)))
		t.prec = 1 + r.IntN(min(maxPrec, 18))
		t.scale = r.IntN(t.prec + 1)
		return t
	case 2:
		syms := []string{"A", "B", "C", "D"}
		r.Shuffle(len(syms), func(i, j int) { syms[i], syms[j] = syms[j], syms[i] })
		return &genType{kind: gEnum, name: name, symbols: syms[:2+r.IntN(3)]}
	default:
		names := []string{"p", "q", "s", "u"}
		r.Shuffle(len(names), func(i, j int) { names[i], names[j] = names[j], names[i] })
		t := &genType{kind: gRecord, name: name}
		for _, n := range names[:1+r.IntN(4)] {
			t.fields = append(t.fields, genField{name: n, typ: genPrimitive(r)})
		}
		return t
	}
}

// genOptions limit what the generator emits, so a property can leave out a
// shape with a separately tracked bug instead of failing on it.
type genOptions struct {
	// bareNamed makes the generator pass the value of every nullable named
	// type or map bare, the way Serde.Unmarshal returns it, instead of in
	// the codec's named form.
	bareNamed bool
}

// genNamedSchema builds a record schema named recName. layoutSeed decides
// the structure: how many named types there are and where they are
// referenced. defSeed decides what each named type is. Two schemas built
// with the same layoutSeed and different defSeeds use the same names, in the
// same places, for different types. That is the situation of two versions of
// one subject, or two subjects that share a type name, in one process.
func genNamedSchema(recName string, layoutSeed, defSeed uint64, opts genOptions) *genType {
	lr := rand.New(rand.NewPCG(layoutSeed, 1))
	dr := rand.New(rand.NewPCG(defSeed, 2))

	root := &genType{kind: gRecord, name: recName}
	defs := make([]*genType, 1+lr.IntN(3))
	for i := range defs {
		defs[i] = genNamedDef(dr, "n"+strconv.Itoa(i))
		// The first occurrence defines the type. It sits in a nullable field
		// so the record value can leave it out.
		u := &genType{kind: gNullUnion, elem: defs[i]}
		root.fields = append(root.fields, genField{name: "d" + strconv.Itoa(i), typ: u})
	}
	for j := range 1 + lr.IntN(5) {
		var ft *genType
		if lr.IntN(4) == 0 {
			ft = genPrimitive(lr)
		} else {
			d := defs[lr.IntN(len(defs))]
			ref := &genType{kind: gRef, name: d.name, def: d}
			switch lr.IntN(8) {
			case 0:
				ft = ref
			case 1:
				ft = &genType{kind: gArray, elem: ref}
			case 2:
				ft = &genType{kind: gMap, elem: ref}
			case 3:
				ft = &genType{kind: gArray, elem: &genType{kind: gArray, elem: ref}}
			case 4:
				ft = &genType{kind: gMap, elem: &genType{kind: gArray, elem: ref}}
			case 5:
				ft = &genType{kind: gNullUnion, elem: &genType{kind: gMap, elem: ref}}
			case 6:
				ft = &genType{kind: gMap, elem: &genType{kind: gNullUnion, elem: ref}}
			default:
				ft = &genType{kind: gNullUnion, elem: ref}
			}
		}
		root.fields = append(root.fields, genField{name: "u" + strconv.Itoa(j), typ: ft})
	}
	if opts.bareNamed {
		setBare(root)
	}
	return root
}

// setBare marks every nullable union under t as bare.
func setBare(t *genType) {
	switch t.kind { //nolint:exhaustive // only these hold nullable unions
	case gRecord:
		for _, f := range t.fields {
			setBare(f.typ)
		}
	case gArray, gMap:
		setBare(t.elem)
	case gNullUnion:
		t.bare = true
		setBare(t.elem)
	}
}

// propertyIterations is the number of random cases per property test. Set
// CONDUIT_PROPERTY_LONG to run more.
func propertyIterations() int {
	if os.Getenv("CONDUIT_PROPERTY_LONG") != "" {
		return 20000
	}
	return 500
}

// mustParseGen parses the schema of t, failing the test on error.
func mustParseGen(tb testing.TB, t *genType) *Serde {
	tb.Helper()
	s, err := Parse([]byte(t.json()))
	if err != nil {
		tb.Fatalf("parse generated schema: %v\n%s", err, t.json())
	}
	return s
}
