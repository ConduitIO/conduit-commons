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
	"math"
	"math/big"
	"reflect"
	"time"

	"github.com/conduitio/conduit-commons/opencdc"
	"github.com/iskorotkov/avro/v2"
	"github.com/modern-go/reflect2"
)

// BeforeMarshal is the inverse of AfterUnmarshal. Where AfterUnmarshal
// unwraps a union value (see unwrapsAt), BeforeMarshal puts the bare value
// back into the codec's named form, {"branch": value}, whenever the codec
// could not encode the bare value correctly itself. Everywhere else a union
// value is expected in the named form already, and BeforeMarshal only looks
// inside it.
//
// Like AfterUnmarshal, it walks the schema and the value together, so every
// union is matched to its own value at any depth. It used to follow
// precomputed schema paths through the value, and those stepped over the
// value of a union that is not an array item or map value. Once
// AfterUnmarshal reached every union (ConduitIO/conduit-commons#312), a
// nullable record, map, enum or fixed below such a union decoded bare and
// could no longer be encoded again.
//
// The codec reads every map[string]any in a union position as the named
// form, so a bare record or map value must be wrapped, and it resolves any
// other bare value by its Go type, which does not identify an enum or a
// fixed. A bare value is wrapped into the branch it fits. A value that fits
// no branch, or more than one (for example a map that is also a valid
// record of another branch), is an error.
//
// Invariant 6 (schema handling never silently mangles data): a value is
// never assigned to a union branch by guessing.
//
// BeforeMarshal changes val only if it returns nil. It first collects every
// change and applies them once the whole value has been checked.
// Serde.Marshal uses wrapSubstitutions instead, to undo the changes after
// encoding.
func (r unionResolver) BeforeMarshal(val any) error {
	root, subs, err := r.wrapSubstitutions(val)
	if err != nil {
		return err
	}
	for _, sub := range subs {
		sub.substitute()
	}
	if p, ok := val.(*any); ok && root != nil {
		*p = root
	}
	return nil
}

// wrapSubstitutions returns the changes BeforeMarshal makes to val, without
// making them. If the schema is a union and the value itself has to be
// wrapped, root is its replacement (and val can't be changed in place).
func (r unionResolver) wrapSubstitutions(val any) (root any, subs []substitution, err error) {
	if !r.wrapsUnions {
		return nil, nil, nil
	}

	w := unionWrapper{r: r}
	replace := false
	switch v := val.(type) {
	case *opencdc.StructuredData:
		if v != nil && *v != nil {
			root, replace, err = w.value(r.schema, map[string]any(*v), posTop)
		}
	case *map[string]any:
		if v != nil && *v != nil {
			root, replace, err = w.value(r.schema, *v, posTop)
		}
	case *any:
		if v != nil {
			root, replace, err = w.value(r.schema, *v, posTop)
		}
	default:
		root, replace, err = w.value(r.schema, val, posTop)
	}
	if err != nil {
		return nil, nil, err
	}
	if !replace {
		root = nil
	}
	return root, w.subs, nil
}

// unionWrapper collects the substitutions BeforeMarshal applies. It never
// changes the value it walks.
type unionWrapper struct {
	r    unionResolver
	subs []substitution
}

// value walks v, a value of schema s at position pos. If v must be replaced
// in its parent, value returns the replacement and true; the caller records
// it as a substitution for its own slot.
func (w *unionWrapper) value(s avro.Schema, v any, pos unionPosition) (any, bool, error) {
	if v == nil {
		return nil, false, nil
	}
	switch s := s.(type) {
	case *avro.RefSchema:
		return w.value(s.Schema(), v, pos)
	case *avro.RecordSchema:
		return nil, false, w.record(s, v)
	case *avro.ArraySchema:
		return nil, false, w.array(s, v)
	case *avro.MapSchema:
		return nil, false, w.mapValues(s, v)
	case *avro.UnionSchema:
		if isTrivialUnion(s) {
			return nil, false, nil // the codec resolves the value itself
		}
		if !unwrapsAt(s, pos) {
			return w.wrappedUnion(s, v)
		}
		return w.unwrappedUnion(s, v, pos)
	}
	return nil, false, nil
}

func (w *unionWrapper) record(s *avro.RecordSchema, v any) error {
	m, ok := asStringMap(v)
	if !ok {
		return nil // a Go struct or another type the codec handles itself
	}
	for _, f := range s.Fields() {
		fv, ok := m[f.Name()]
		if !ok {
			continue
		}
		nv, replace, err := w.value(f.Type(), fv, posField)
		if err != nil {
			return fmt.Errorf("%s: %w", f.Name(), err)
		}
		if replace {
			w.subs = append(w.subs, mapSubstitution{m: m, key: f.Name(), val: nv, old: fv})
		}
	}
	return nil
}

func (w *unionWrapper) array(s *avro.ArraySchema, v any) error {
	a, ok := v.([]any)
	if !ok {
		return nil // a typed slice holds no union values to wrap
	}
	for i, item := range a {
		nv, replace, err := w.value(s.Items(), item, posArrayItem)
		if err != nil {
			return fmt.Errorf("[%d]: %w", i, err)
		}
		if replace {
			w.subs = append(w.subs, arraySubstitution{a: a, index: i, val: nv, old: item})
		}
	}
	return nil
}

func (w *unionWrapper) mapValues(s *avro.MapSchema, v any) error {
	m, ok := asStringMap(v)
	if !ok {
		return nil // a typed map holds no union values to wrap
	}
	for k, mv := range m {
		nv, replace, err := w.value(s.Values(), mv, posMapValue)
		if err != nil {
			return fmt.Errorf("%s: %w", k, err)
		}
		if replace {
			w.subs = append(w.subs, mapSubstitution{m: m, key: k, val: nv, old: mv})
		}
	}
	return nil
}

// wrappedUnion walks v, a value of a union that AfterUnmarshal leaves in the
// codec's form: the named form, or a bare value when the codec resolves
// every branch by Go type (and then nothing below it is a union to wrap). It
// looks inside a named value. A bare value is wrapped only if the codec
// would resolve it to the wrong branch, and anything else is left for the
// codec to accept or reject.
func (w *unionWrapper) wrappedUnion(us *avro.UnionSchema, v any) (any, bool, error) {
	named, ok := asStringMap(v)
	if !ok {
		// The codec resolves a bare value to the first branch its Go type
		// names. A time.Time in ["int.date", "long.timestamp-millis"] names
		// both, and the codec would write a timestamp with a time of day as
		// a date.
		rd, err := w.r.readOther(us, v, false, true)
		if err != nil || !rd.wrap {
			return nil, false, err
		}
		return map[string]any{rd.name: v}, true, nil
	}
	if len(named) != 1 {
		return nil, false, nil
	}
	name, inner := singleMapEntry(named)
	branch, idx := us.Types().Get(name)
	if idx < 0 {
		return nil, false, nil
	}
	// A union never directly contains a union, so the branch's own children
	// decide their positions and inner is never replaced.
	_, _, err := w.value(branch, inner, posTop)
	return nil, false, err
}

// unwrappedUnion walks v, a value of a union that AfterUnmarshal unwraps at
// pos, so v is a bare value of one of its branches, or a caller's value in
// the named form. It returns the named form of a bare value the codec can't
// encode bare.
func (w *unionWrapper) unwrappedUnion(us *avro.UnionSchema, v any, pos unionPosition) (any, bool, error) {
	var rd unionReading
	var err error
	if m, ok := asStringMap(v); ok {
		rd, err = readMap(us, m, isContainerUnionAt(us, pos))
	} else {
		rd, err = w.r.readOther(us, v, isContainerUnionAt(us, pos), false)
	}
	if err != nil || rd.branch == nil {
		return nil, false, err // no reading: the codec accepts or rejects v itself
	}
	if rd.named {
		_, _, err := w.value(rd.branch, rd.inner, posTop)
		return nil, false, err
	}
	if _, _, err := w.value(rd.branch, v, posTop); err != nil {
		return nil, false, err
	}
	if rd.wrap {
		return map[string]any{rd.name: v}, true, nil
	}
	return nil, false, nil
}

func singleMapEntry(m map[string]any) (string, any) {
	for k, v := range m {
		return k, v
	}
	return "", nil
}

// isContainerUnionAt reports whether union us at position pos is one that
// BeforeMarshal has always wrapped: a union with an array or map branch as
// an array item or map value. Every non-nil value there is wrapped, as
// before; elsewhere a value is only wrapped when the codec needs it.
func isContainerUnionAt(us *avro.UnionSchema, pos unionPosition) bool {
	return (pos == posArrayItem || pos == posMapValue) && unionHasContainer(us)
}

// unionReading is how a value at a union position is read: the branch it
// belongs to, and whether it is already in the named form.
type unionReading struct {
	name   string      // the codec's name for the branch, the key of the named form
	branch avro.Schema // nil if v could not be read as any branch
	named  bool        // v is {name: inner}
	inner  any         // the branch value, if named
	wrap   bool        // v is bare and must be wrapped into {name: v}
}

// readMap reads m, a value at a position where AfterUnmarshal unwraps union
// us. m is read as a bare value of each record or map branch it fits, and as
// the named form if it has a single key naming a branch whose value fits
// that branch. It must have exactly one reading: none or several is an
// error, because the codec would either reject it later with a less useful
// error, or encode it as a branch that was not meant.
func readMap(us *avro.UnionSchema, m map[string]any, container bool) (unionReading, error) {
	var readings []unionReading
	for _, t := range us.Types() {
		switch derefSchema(t).(type) {
		case *avro.RecordSchema, *avro.MapSchema:
			if fitsSchema(t, m) {
				readings = append(readings, unionReading{name: unionBranchName(t), branch: t, wrap: true})
			}
		}
	}
	// Where BeforeMarshal has always wrapped every value (container), a
	// caller never had to pass the named form, and a decoded value is always
	// bare, so a bare reading wins over the named one.
	if len(m) == 1 && (!container || len(readings) == 0) {
		name, inner := singleMapEntry(m)
		if t, idx := us.Types().Get(name); idx >= 0 && t.Type() != avro.Null && fitsSchema(t, inner) {
			readings = append(readings, unionReading{name: name, branch: t, named: true, inner: inner})
		}
	}
	if len(readings) == 0 {
		return unionReading{}, fmt.Errorf("value %v fits no branch of union %s: %w", m, us.String(), ErrSchemaValueMismatch)
	}
	return oneReading(us, m, readings)
}

// readOther reads v, a value that is not a map[string]any, at a union
// position. It reads v by its Go type, the way the codec resolves it, and
// also as an enum symbol, a fixed value or a fixed decimal, which the codec
// cannot resolve by Go type. alwaysWrap is set where BeforeMarshal has
// always wrapped every value (see isContainerUnionAt); only there is no
// reading an error, elsewhere v is left for the codec to accept or reject.
// More than one reading is an error. codecBare is set where the codec
// itself decodes v bare (see resolvedReadings).
func (r unionResolver) readOther(us *avro.UnionSchema, v any, alwaysWrap, codecBare bool) (unionReading, error) {
	var readings []unionReading
	t := reflect.TypeOf(v)
	switch {
	case t.Kind() == reflect.Map:
		// A typed map, e.g. map[string]int.
		if branch, idx := us.Types().Get(avroMapType); idx >= 0 {
			readings = append(readings, unionReading{name: avroMapType, branch: branch, wrap: alwaysWrap})
		}
	case t.Kind() == reflect.Slice && !t.Elem().AssignableTo(byteType): // []byte is bytes
		if branch, idx := us.Types().Get(avroArrayType); idx >= 0 {
			readings = append(readings, unionReading{name: avroArrayType, branch: branch, wrap: alwaysWrap})
		}
	default:
		readings = r.resolvedReadings(us, v, alwaysWrap, codecBare)
		for _, b := range us.Types() {
			if fitsUnresolvableBranch(b, v) {
				readings = append(readings, unionReading{name: unionBranchName(b), branch: b, wrap: true})
			}
		}
	}
	if len(readings) == 0 {
		if alwaysWrap {
			return unionReading{}, fmt.Errorf("can't resolve %T in union type %v: %w", v, us.String(), ErrSchemaValueMismatch)
		}
		return unionReading{}, nil
	}
	return oneReading(us, v, readings)
}

// resolvedReadings returns the branches of us that the codec's type
// resolver names for v. The codec takes the first of them. More than one is
// ambiguous, except:
//   - for a Go int: the codec decodes an int into it and a long into an
//     int64, so int, which it names first, is the right reading;
//   - for a time.Time or time.Duration, only the branches that hold v without
//     losing precision count. The codec decodes a date to midnight UTC, so a
//     time.Time with a time of day is a timestamp, and a Duration with a
//     sub-millisecond part is time-micros. A value that several candidates
//     hold exactly (midnight, whole milliseconds) decodes to the same value
//     from each. Where the codec itself decodes the union bare (codecBare),
//     the branch it picked is lost anyway, so the codec's own pick stands, as
//     it always has. Where AfterUnmarshal removed the codec's named form, the
//     value stays ambiguous, because the branch is what changes.
func (r unionResolver) resolvedReadings(us *avro.UnionSchema, v any, wrap, codecBare bool) []unionReading {
	names, err := r.resolver.Name(reflect2.TypeOf(v))
	if err != nil {
		return nil
	}
	var readings []unionReading
	for _, n := range names {
		if branch, idx := us.Types().Get(n); idx >= 0 {
			readings = append(readings, unionReading{name: n, branch: branch, wrap: wrap})
			if _, isInt := v.(int); isInt {
				break
			}
		}
	}
	if len(readings) > 1 {
		first := readings[0].name
		exact := readings[:0:0]
		for _, rd := range readings {
			if holdsExactly(rd.branch, v) {
				exact = append(exact, rd)
			}
		}
		if len(exact) > 0 {
			readings = exact
		}
		if codecBare && len(readings) > 1 && readings[0].name == first {
			readings = readings[:1]
		}
		if len(readings) == 1 && readings[0].name != first {
			readings[0].wrap = true // the codec would take first
		}
	}
	return readings
}

// holdsExactly reports whether a time.Time or time.Duration v survives
// encoding with the logical type of s and decoding again. It is true for
// any other value.
func holdsExactly(s avro.Schema, v any) bool {
	var lt avro.LogicalType
	if ls, ok := s.(avro.LogicalTypeSchema); ok && ls.Logical() != nil {
		lt = ls.Logical().Type()
	}
	switch v := v.(type) {
	case time.Time:
		u := v.UTC()
		switch lt { //nolint:exhaustive // only time logical types hold a time.Time
		case avro.Date:
			return u.Hour() == 0 && u.Minute() == 0 && u.Second() == 0 && u.Nanosecond() == 0
		case avro.TimestampMillis, avro.LocalTimestampMillis:
			return u.Nanosecond()%int(time.Millisecond) == 0
		case avro.TimestampMicros, avro.LocalTimestampMicros:
			return u.Nanosecond()%int(time.Microsecond) == 0
		}
	case time.Duration:
		switch lt { //nolint:exhaustive // only time logical types hold a time.Duration
		case avro.TimeMillis:
			return v%time.Millisecond == 0 && v/time.Millisecond <= math.MaxInt32 && v/time.Millisecond >= math.MinInt32
		case avro.TimeMicros:
			return v%time.Microsecond == 0
		}
	}
	return true
}

// oneReading returns the only reading, or an error if there are several.
func oneReading(us *avro.UnionSchema, v any, readings []unionReading) (unionReading, error) {
	if len(readings) == 1 {
		return readings[0], nil
	}
	return unionReading{}, ambiguousUnionValueError(us, v, readings)
}

func ambiguousUnionValueError(us *avro.UnionSchema, v any, readings []unionReading) error {
	names := make([]string, len(readings))
	for i, rd := range readings {
		names[i] = rd.name
		if rd.named {
			names[i] = fmt.Sprintf("{%q: value}", rd.name)
		}
	}
	return fmt.Errorf("value %v of union %s is ambiguous, it reads as each of %v: %w", v, us.String(), names, ErrSchemaValueMismatch)
}

// fitsUnresolvableBranch reports whether v is a bare value of branch b that
// the codec cannot resolve by its Go type: an enum symbol, a fixed value
// ([N]byte), a fixed decimal (*big.Rat) or a fixed duration.
func fitsUnresolvableBranch(b avro.Schema, v any) bool {
	switch b := derefSchema(b).(type) {
	case *avro.EnumSchema:
		sym, ok := v.(string)
		if !ok {
			return false
		}
		for _, s := range b.Symbols() {
			if s == sym {
				return true
			}
		}
	case *avro.FixedSchema:
		if ls := b.Logical(); ls != nil {
			switch ls.Type() { //nolint:exhaustive // fixed only has these logical types
			case avro.Decimal:
				switch v.(type) {
				case *big.Rat, big.Rat:
					return true
				}
				return false
			case avro.Duration:
				_, ok := v.(avro.LogicalDuration)
				return ok
			}
		}
		t := reflect.TypeOf(v)
		return t.Kind() == reflect.Array && t.Elem() == byteType && t.Len() == b.Size()
	}
	return false
}

// unionBranchName is the codec's name for union branch t, the key of its
// named form: the full name of a named type, otherwise the type with its
// logical type (e.g. "map", "long.timestamp-millis").
func unionBranchName(t avro.Schema) string {
	t = derefSchema(t)
	if n, ok := t.(avro.NamedSchema); ok {
		return n.FullName()
	}
	name := string(t.Type())
	if lt, ok := t.(avro.LogicalTypeSchema); ok && lt.Logical() != nil {
		name += "." + string(lt.Logical().Type())
	}
	return name
}

func derefSchema(s avro.Schema) avro.Schema {
	if ref, ok := s.(*avro.RefSchema); ok {
		return ref.Schema()
	}
	return s
}

// fitsSchema reports whether v has the shape of a value of s, in the form
// Unmarshal returns and Marshal accepts. It only tells the readings of a
// union value apart, so it errs on the side of fitting: a record must have
// exactly its fields (every field without a default, and no others), a
// map's values and an array's items must fit, a union value must fit a
// branch or be one branch's named form, and a primitive must have a Go type
// the codec could encode as it (see fitsPrimitive).
//
// It is adapted from ConduitIO/conduit-commons#320.
func fitsSchema(s avro.Schema, v any) bool {
	s = derefSchema(s)
	switch s := s.(type) {
	case *avro.RecordSchema:
		return fitsRecord(s, v)
	case *avro.MapSchema:
		m, ok := asStringMap(v)
		if !ok {
			return false
		}
		for _, mv := range m {
			if !fitsSchema(s.Values(), mv) {
				return false
			}
		}
		return true
	case *avro.ArraySchema:
		a, ok := v.([]any)
		if !ok {
			// nil, or a typed slice
			return v == nil || (reflect.TypeOf(v).Kind() == reflect.Slice && !reflect.TypeOf(v).Elem().AssignableTo(byteType))
		}
		for _, item := range a {
			if !fitsSchema(s.Items(), item) {
				return false
			}
		}
		return true
	case *avro.UnionSchema:
		return fitsUnion(s, v)
	case *avro.NullSchema:
		return v == nil
	}
	_, isMap := asStringMap(v)
	_, isList := v.([]any)
	return !isMap && !isList && fitsPrimitive(s, v)
}

// fitsPrimitive reports whether v, which is not a map[string]any or []any,
// can be a value of s, a schema that is not a record, map, array, union or
// null. It follows the Go types the codec's encoders accept for s, so that
// a record and a map, or two records, whose fields have different types are
// not mistaken for each other. A Go type it does not know fits, and so does
// nil: a false "fits" can only lead to an ambiguity error, while a false
// "does not fit" could reject a value the codec encodes.
func fitsPrimitive(s avro.Schema, v any) bool {
	if v == nil {
		return true
	}
	st := s.Type()
	var lt avro.LogicalType
	if ls, ok := s.(avro.LogicalTypeSchema); ok && ls.Logical() != nil {
		lt = ls.Logical().Type()
	}

	switch v.(type) {
	case time.Time:
		return (st == avro.Int && lt == avro.Date) || (st == avro.Long && lt != "" && lt != avro.TimeMicros)
	case *big.Rat, big.Rat:
		return (st == avro.Bytes || st == avro.Fixed) && lt == avro.Decimal
	case avro.LogicalDuration:
		return st == avro.Fixed && lt == avro.Duration
	}

	return fitsPrimitiveKind(s, st, lt, v)
}

// fitsPrimitiveKind is fitsPrimitive for a value that is not a logical type
// value, by its Go kind.
func fitsPrimitiveKind(s avro.Schema, st avro.Type, lt avro.LogicalType, v any) bool {
	t := reflect.TypeOf(v)
	switch t.Kind() { //nolint:exhaustive // other kinds are not ruled out
	case reflect.Bool:
		return st == avro.Boolean
	case reflect.Int, reflect.Int32:
		return st == avro.Int || st == avro.Long
	case reflect.Int8, reflect.Uint8, reflect.Int16, reflect.Uint16:
		return st == avro.Int
	case reflect.Uint32:
		return st == avro.Long
	case reflect.Int64: // also time.Duration
		return st == avro.Long || (st == avro.Int && lt == avro.TimeMillis)
	case reflect.Uint64:
		fs, ok := s.(*avro.FixedSchema)
		return ok && fs.Size() == 8
	case reflect.Float32:
		return st == avro.Float || st == avro.Double
	case reflect.Float64:
		return st == avro.Double
	case reflect.String:
		if es, ok := s.(*avro.EnumSchema); ok {
			for _, sym := range es.Symbols() {
				if sym == v {
					return true
				}
			}
			return false
		}
		return st == avro.String
	case reflect.Slice:
		return t.Elem().Kind() == reflect.Uint8 && st == avro.Bytes
	case reflect.Array:
		fs, ok := s.(*avro.FixedSchema)
		return ok && t.Elem().Kind() == reflect.Uint8 && t.Len() == fs.Size()
	}
	return true
}

func fitsRecord(s *avro.RecordSchema, v any) bool {
	m, ok := asStringMap(v)
	if !ok {
		return false
	}
	fields := s.Fields()
	present := 0
	for _, f := range fields {
		fv, ok := m[f.Name()]
		if !ok {
			if !f.HasDefault() {
				return false
			}
			continue
		}
		present++
		if !fitsSchema(f.Type(), fv) {
			return false
		}
	}
	return present == len(m) // no keys that are not fields
}

func fitsUnion(s *avro.UnionSchema, v any) bool {
	for _, t := range s.Types() {
		if fitsSchema(t, v) {
			return true
		}
	}
	// The named form of one of the branches.
	m, ok := asStringMap(v)
	if !ok || len(m) != 1 {
		return false
	}
	for name, inner := range m {
		if t, i := s.Types().Get(name); i >= 0 {
			return fitsSchema(t, inner)
		}
	}
	return false
}
