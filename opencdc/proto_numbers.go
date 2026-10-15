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

package opencdc

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"google.golang.org/protobuf/types/known/structpb"
)

// maxSafeInteger is the largest integer n such that every integer in
// [-n, n] maps to a distinct float64 (JavaScript's Number.MAX_SAFE_INTEGER,
// the I-JSON limit of RFC 7493 section 2.2). A protobuf Struct carries every
// number as a float64, so only integers in this range reach the receiver
// unambiguously: 2^53 itself is exact but collides with a rounded 2^53+1.
const maxSafeInteger = 1<<53 - 1

// OpenCDC record field names, as used by Record.Map and as the roots that
// Record.ToProto and Change.ToProto prefix to the path of a number error.
const (
	fieldKey     = "key"
	fieldPayload = "payload"
	fieldBefore  = "before"
	fieldAfter   = "after"
)

// IntegerNotRepresentableError is returned (wrapped) by ToProto when
// structured data contains an integer outside [-(2^53-1), 2^53-1]. It matches
// ErrIntegerNotRepresentable with errors.Is.
//
// It carries where the value was and its Go type, never the value itself: a
// wide integer can be an account or card number.
type IntegerNotRepresentableError struct {
	// Path is the field path, e.g. "id" or "items[2].n" when returned by
	// StructuredData.ToProto. Record.ToProto prefixes it with the location in
	// the record, e.g. "key.id" or "payload.after.id".
	Path string
	// GoType is the Go type of the value, e.g. "int64" or "json.Number".
	GoType string
}

func (e *IntegerNotRepresentableError) Error() string {
	return fmt.Sprintf(
		"field %q (%s) is outside the safe integer range ±(2^53-1) of a protobuf number; it would be rounded",
		e.Path, e.GoType,
	)
}

// Is reports whether target is ErrIntegerNotRepresentable.
func (e *IntegerNotRepresentableError) Is(target error) bool {
	return target == ErrIntegerNotRepresentable
}

// InvalidNumberError is returned (wrapped) by ToProto for a json.Number whose
// literal is not a JSON number (RFC 8259). It matches ErrInvalidNumber with
// errors.Is. Like IntegerNotRepresentableError it never carries the value.
type InvalidNumberError struct {
	// Path is the field path, with the same conventions as
	// IntegerNotRepresentableError.Path.
	Path string
}

func (e *InvalidNumberError) Error() string {
	return fmt.Sprintf("field %q (json.Number) is not a valid JSON number", e.Path)
}

// Is reports whether target is ErrInvalidNumber.
func (e *InvalidNumberError) Is(target error) bool {
	return target == ErrInvalidNumber
}

// prefixNumberErrorPath prepends prefix to the path of the number error in
// err's chain, if there is one, and returns err.
func prefixNumberErrorPath(err error, prefix string) error {
	var intErr *IntegerNotRepresentableError
	if errors.As(err, &intErr) {
		intErr.Path = joinPath(prefix, intErr.Path)
		return err
	}
	var numErr *InvalidNumberError
	if errors.As(err, &numErr) {
		numErr.Path = joinPath(prefix, numErr.Path)
	}
	return err
}

// wrapProtoError adds context to a conversion error, like
// fmt.Errorf(context+": %w", err), but renders the message lazily: an outer
// caller may still prefix the path of a number error in the chain (Change
// adds "after", Record adds "payload"), and the full message must show the
// final path.
func wrapProtoError(context string, err error) error {
	return &protoContextError{context: context, err: err}
}

type protoContextError struct {
	context string
	err     error
}

func (e *protoContextError) Error() string { return e.context + ": " + e.err.Error() }
func (e *protoContextError) Unwrap() error { return e.err }

func joinPath(prefix, path string) string {
	switch {
	case path == "":
		return prefix
	case strings.HasPrefix(path, "["):
		return prefix + path
	default:
		return prefix + "." + path
	}
}

// pathSegment is a map key or, when isIndex is set, a list index.
type pathSegment struct {
	key     string
	index   int
	isIndex bool
}

// pathStack records the location of the value being walked. It is rendered
// only when an error is reported, so the happy path builds no strings.
type pathStack []pathSegment

func (p pathStack) String() string {
	var sb strings.Builder
	for _, seg := range p {
		if seg.isIndex {
			sb.WriteByte('[')
			sb.WriteString(strconv.Itoa(seg.index))
			sb.WriteByte(']')
			continue
		}
		if sb.Len() > 0 {
			sb.WriteByte('.')
		}
		sb.WriteString(seg.key)
	}
	return sb.String()
}

// structWalker converts structured data to a protobuf Struct exactly like
// structpb.NewStruct, except that it refuses to round integers (invariant 6).
// With build == false it only validates and allocates no proto messages, so
// ValidateProto and ToProto share one code path and cannot disagree.
type structWalker struct {
	build bool
	path  pathStack
}

// newStructWalker returns a walker with room for a typical nesting depth, so
// the path stack is allocated once per walk.
func newStructWalker(build bool) *structWalker {
	return &structWalker{build: build, path: make(pathStack, 0, 8)}
}

func (w *structWalker) structValue(m map[string]any) (*structpb.Struct, error) {
	var out *structpb.Struct
	if w.build {
		out = &structpb.Struct{Fields: make(map[string]*structpb.Value, len(m))}
	}
	for k, v := range m {
		if !utf8.ValidString(k) {
			_, err := structpb.NewValue(k)
			return nil, err //nolint:wrapcheck // the error structpb.NewStruct returns, wrapped once by the caller
		}
		w.path = append(w.path, pathSegment{key: k})
		val, err := w.value(v)
		w.path = w.path[:len(w.path)-1]
		if err != nil {
			return nil, err
		}
		if w.build {
			out.Fields[k] = val
		}
	}
	return out, nil
}

func (w *structWalker) listValue(l []any) (*structpb.ListValue, error) {
	var out *structpb.ListValue
	if w.build {
		out = &structpb.ListValue{Values: make([]*structpb.Value, len(l))}
	}
	for i, v := range l {
		w.path = append(w.path, pathSegment{index: i, isIndex: true})
		val, err := w.value(v)
		w.path = w.path[:len(w.path)-1]
		if err != nil {
			return nil, err
		}
		if w.build {
			out.Values[i] = val
		}
	}
	return out, nil
}

func (w *structWalker) notRepresentable(goType string) error {
	return &IntegerNotRepresentableError{Path: w.path.String(), GoType: goType}
}

// integer handles the Go integer types structpb would round; handled is false
// when v is not one of them. Invariant 6: refuse to round an integer at the
// plugin boundary; a rounded key deletes or overwrites the wrong row
// downstream.
func (w *structWalker) integer(v any) (val *structpb.Value, handled bool, err error) {
	var f float64
	switch v := v.(type) {
	case int:
		if int64(v) < -maxSafeInteger || int64(v) > maxSafeInteger {
			return nil, true, w.notRepresentable("int")
		}
		f = float64(v)
	case int64:
		if v < -maxSafeInteger || v > maxSafeInteger {
			return nil, true, w.notRepresentable("int64")
		}
		f = float64(v)
	case uint:
		if uint64(v) > maxSafeInteger {
			return nil, true, w.notRepresentable("uint")
		}
		f = float64(v)
	case uint64:
		if v > maxSafeInteger {
			return nil, true, w.notRepresentable("uint64")
		}
		f = float64(v)
	default:
		return nil, false, nil
	}
	if w.build {
		val = structpb.NewNumberValue(f)
	}
	return val, true, nil
}

// jsonNumber checks v, which holds a json.Number with the literal lit. It
// takes the interface value too, so delegating does not box it again.
func (w *structWalker) jsonNumber(v any, lit string) (*structpb.Value, error) {
	switch classifyJSONNumber(lit) {
	case jsonNumberInvalid:
		return nil, &InvalidNumberError{Path: w.path.String()}
	case jsonNumberIntegerOutOfRange:
		return nil, w.notRepresentable("json.Number")
	case jsonNumberSafeInteger, jsonNumberNonInteger:
		// Exactly what structpb does today. Non-integer (decimal) precision is
		// out of scope (#3100).
	}
	return w.delegate(v)
}

// delegate converts v with structpb.NewValue, or does nothing when only
// validating.
func (w *structWalker) delegate(v any) (*structpb.Value, error) {
	if !w.build {
		return nil, nil //nolint:nilnil // validation only
	}
	return structpb.NewValue(v) //nolint:wrapcheck // wrapped once by wrapStructuredDataError, as ToProto always has
}

// value mirrors structpb.NewValue case by case. Every case that cannot round
// is delegated to structpb, so its behavior and error texts are unchanged.
func (w *structWalker) value(v any) (*structpb.Value, error) {
	if val, handled, err := w.integer(v); handled {
		return val, err
	}
	orig := v // delegate the interface value itself: re-boxing would allocate
	switch v := v.(type) {
	case json.Number:
		return w.jsonNumber(orig, string(v))
	case map[string]any:
		s, err := w.structValue(v)
		if err != nil || !w.build {
			return nil, err
		}
		return structpb.NewStructValue(s), nil
	case []any:
		l, err := w.listValue(v)
		if err != nil || !w.build {
			return nil, err
		}
		return structpb.NewListValue(l), nil
	case string:
		if !utf8.ValidString(v) {
			return structpb.NewValue(orig) //nolint:wrapcheck // structpb's own error, wrapped once by the caller
		}
		return w.delegate(orig)
	case nil, bool, int8, int16, int32, uint8, uint16, uint32, float32, float64, []byte:
		return w.delegate(v) // cannot round an integer and cannot fail
	default:
		// structpb rejects every other type ("invalid type: %T").
		return structpb.NewValue(v) //nolint:wrapcheck // structpb's own error, wrapped once by the caller
	}
}

type jsonNumberClass int

const (
	jsonNumberInvalid jsonNumberClass = iota
	jsonNumberNonInteger
	jsonNumberSafeInteger
	jsonNumberIntegerOutOfRange
)

// exponentSaturation bounds the parsed exponent. The other term of the
// effective exponent is bounded by the literal's length, so their sum cannot
// overflow an int64 for any literal that fits in memory, and saturation keeps
// both facts the classification needs: the sign of the effective exponent and
// whether the integer has more than 16 digits.
const exponentSaturation = 1 << 61

// jsonNumberParts is a literal split by the RFC 8259 number grammar,
// [ minus ] int [ frac ] [ exp ]. The sign is not kept: the safe range is
// symmetric.
type jsonNumberParts struct {
	intDigits  string
	fracDigits string
	exp        int64 // saturated at ±exponentSaturation
}

// scanJSONNumber splits s by the RFC 8259 number grammar, or reports false if
// s is not a JSON number. strconv.ParseFloat (behind json.Number.Float64)
// accepts hex floats, a leading '+' and "Infinity", so such literals must
// never reach it.
func scanJSONNumber(s string) (jsonNumberParts, bool) {
	var p jsonNumberParts
	i := 0
	if i < len(s) && s[i] == '-' {
		i++
	}
	// int = zero / ( digit1-9 *DIGIT )
	intStart := i
	switch {
	case i < len(s) && s[i] == '0':
		i++
	case i < len(s) && s[i] >= '1' && s[i] <= '9':
		i = skipDigits(s, i)
	default:
		return p, false
	}
	p.intDigits = s[intStart:i]
	// frac = decimal-point 1*DIGIT
	if i < len(s) && s[i] == '.' {
		fracStart := i + 1
		i = skipDigits(s, fracStart)
		if i == fracStart {
			return p, false
		}
		p.fracDigits = s[fracStart:i]
	}
	// exp = e [ minus / plus ] 1*DIGIT
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		var ok bool
		p.exp, i, ok = scanExponent(s, i+1)
		if !ok {
			return p, false
		}
	}
	return p, i == len(s)
}

// scanExponent parses [ minus / plus ] 1*DIGIT at s[i:], saturating the
// magnitude at exponentSaturation, and returns the index after it.
func scanExponent(s string, i int) (int64, int, bool) {
	negative := false
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		negative = s[i] == '-'
		i++
	}
	start := i
	var exp int64
	for ; i < len(s) && isDigit(s[i]); i++ {
		exp = min(exp*10+int64(s[i]-'0'), exponentSaturation)
	}
	if i == start {
		return 0, i, false
	}
	if negative {
		exp = -exp
	}
	return exp, i, true
}

func skipDigits(s string, i int) int {
	for i < len(s) && isDigit(s[i]) {
		i++
	}
	return i
}

// classifyJSONNumber classifies a json.Number literal without big-number
// arithmetic: the cost is one scan of the literal, whatever its exponent.
func classifyJSONNumber(s string) jsonNumberClass {
	p, ok := scanJSONNumber(s)
	if !ok {
		return jsonNumberInvalid
	}
	// Significant digits d and effective exponent e: value = d * 10^e.
	d := strings.TrimLeft(p.intDigits+p.fracDigits, "0")
	e := p.exp - int64(len(p.fracDigits))
	trimmed := strings.TrimRight(d, "0")
	e += int64(len(d) - len(trimmed))
	d = trimmed

	switch {
	case d == "":
		return jsonNumberSafeInteger // zero, in any notation
	case e < 0:
		return jsonNumberNonInteger
	case int64(len(d))+e > 16:
		return jsonNumberIntegerOutOfRange // at least 10^16, beyond 2^53-1
	}
	n, err := strconv.ParseInt(d+strings.Repeat("0", int(e)), 10, 64)
	if err != nil || n > maxSafeInteger {
		return jsonNumberIntegerOutOfRange
	}
	return jsonNumberSafeInteger
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// ValidateProto reports the first error ToProto would return for r, without
// allocating proto messages. Callers that encode a batch use it to attribute a
// refusal to one record before encoding the batch. It is safe for concurrent
// use and does not modify r.
func (r Record) ValidateProto() error {
	if err := validateData(r.Key); err != nil {
		return wrapProtoError("error converting key", prefixNumberErrorPath(err, fieldKey))
	}
	if err := r.Payload.validateProto(); err != nil {
		return wrapProtoError("error converting payload", prefixNumberErrorPath(err, fieldPayload))
	}
	return nil
}

func (c Change) validateProto() error {
	if err := validateData(c.Before); err != nil {
		return wrapProtoError("error converting before", prefixNumberErrorPath(err, fieldBefore))
	}
	if err := validateData(c.After); err != nil {
		return wrapProtoError("error converting after", prefixNumberErrorPath(err, fieldAfter))
	}
	return nil
}

func validateData(d Data) error {
	sd, ok := d.(StructuredData)
	if !ok {
		return nil // RawData and nil cannot fail
	}
	w := newStructWalker(false)
	_, err := w.structValue(sd)
	return wrapStructuredDataError(err)
}

// wrapStructuredDataError keeps number errors bare, so their message leads
// with the field path, and wraps every other error as ToProto always has.
func wrapStructuredDataError(err error) error {
	if err == nil || errors.Is(err, ErrIntegerNotRepresentable) || errors.Is(err, ErrInvalidNumber) {
		return err
	}
	return fmt.Errorf("could not convert structured data to proto: %w", err)
}
