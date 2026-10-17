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
	"fmt"
	"strconv"
	"strings"

	opencdcv2 "github.com/conduitio/conduit-commons/proto/opencdc/v2"
	"google.golang.org/protobuf/types/known/structpb"
)

// v2ValueEncoder converts the Go values StructuredData holds to opencdc.v2.Value
// messages. It mirrors the shape of structWalker (the v1 encoder) but never
// refuses or rounds: the int64, uint64, bytes and decimal kinds carry every
// value exactly. The only errors are an invalid json.Number literal and a Go
// type the record does not carry (the same "invalid type" error structpb
// returns).
type v2ValueEncoder struct {
	path pathStack
}

func (w *v2ValueEncoder) structValue(m map[string]any) (*opencdcv2.Struct, error) {
	out := &opencdcv2.Struct{Fields: make(map[string]*opencdcv2.Value, len(m))}
	for k, v := range m {
		w.path = append(w.path, pathSegment{key: k})
		val, err := w.value(v)
		w.path = w.path[:len(w.path)-1]
		if err != nil {
			return nil, err
		}
		out.Fields[k] = val
	}
	return out, nil
}

func (w *v2ValueEncoder) listValue(l []any) (*opencdcv2.List, error) {
	out := &opencdcv2.List{Items: make([]*opencdcv2.Value, len(l))}
	for i, v := range l {
		w.path = append(w.path, pathSegment{index: i, isIndex: true})
		val, err := w.value(v)
		w.path = w.path[:len(w.path)-1]
		if err != nil {
			return nil, err
		}
		out.Items[i] = val
	}
	return out, nil
}

// value converts a single Go value. It keeps the Go type: int* becomes int64,
// uint* becomes uint64, and []byte becomes bytes, so nothing is rounded.
func (w *v2ValueEncoder) value(v any) (*opencdcv2.Value, error) {
	if val, handled, err := w.scalar(v); handled {
		return val, err
	}

	switch v := v.(type) {
	case map[string]any:
		s, err := w.structValue(v)
		if err != nil {
			return nil, err
		}
		return &opencdcv2.Value{Kind: &opencdcv2.Value_Struct{Struct: s}}, nil
	case []any:
		l, err := w.listValue(v)
		if err != nil {
			return nil, err
		}
		return &opencdcv2.Value{Kind: &opencdcv2.Value_List{List: l}}, nil
	default:
		// Mirrors structpb.NewValue: reject types a record does not carry
		// (invariant 6), instead of silently coercing or dropping them.
		return nil, fmt.Errorf("invalid type: %T", v)
	}
}

// scalar converts the scalar Go values a record carries. handled is false when
// v is a container or any other type value handles.
func (w *v2ValueEncoder) scalar(v any) (val *opencdcv2.Value, handled bool, err error) {
	switch v := v.(type) {
	case nil:
		return &opencdcv2.Value{Kind: &opencdcv2.Value_Null{Null: structpb.NullValue_NULL_VALUE}}, true, nil
	case bool:
		return &opencdcv2.Value{Kind: &opencdcv2.Value_Bool{Bool: v}}, true, nil
	case string:
		return &opencdcv2.Value{Kind: &opencdcv2.Value_String_{String_: v}}, true, nil
	case float64:
		return &opencdcv2.Value{Kind: &opencdcv2.Value_Double{Double: v}}, true, nil
	case float32:
		return &opencdcv2.Value{Kind: &opencdcv2.Value_Double{Double: float64(v)}}, true, nil
	case int:
		return &opencdcv2.Value{Kind: &opencdcv2.Value_Int64{Int64: int64(v)}}, true, nil
	case int8:
		return &opencdcv2.Value{Kind: &opencdcv2.Value_Int64{Int64: int64(v)}}, true, nil
	case int16:
		return &opencdcv2.Value{Kind: &opencdcv2.Value_Int64{Int64: int64(v)}}, true, nil
	case int32:
		return &opencdcv2.Value{Kind: &opencdcv2.Value_Int64{Int64: int64(v)}}, true, nil
	case int64:
		return &opencdcv2.Value{Kind: &opencdcv2.Value_Int64{Int64: v}}, true, nil
	case uint:
		return &opencdcv2.Value{Kind: &opencdcv2.Value_Uint64{Uint64: uint64(v)}}, true, nil
	case uint8:
		return &opencdcv2.Value{Kind: &opencdcv2.Value_Uint64{Uint64: uint64(v)}}, true, nil
	case uint16:
		return &opencdcv2.Value{Kind: &opencdcv2.Value_Uint64{Uint64: uint64(v)}}, true, nil
	case uint32:
		return &opencdcv2.Value{Kind: &opencdcv2.Value_Uint64{Uint64: uint64(v)}}, true, nil
	case uint64:
		return &opencdcv2.Value{Kind: &opencdcv2.Value_Uint64{Uint64: v}}, true, nil
	case []byte:
		return &opencdcv2.Value{Kind: &opencdcv2.Value_Bytes{Bytes: v}}, true, nil
	case json.Number:
		val, err := w.jsonNumber(string(v))
		return val, true, err
	default:
		return nil, false, nil
	}
}

func (w *v2ValueEncoder) jsonNumber(lit string) (*opencdcv2.Value, error) {
	class, n := classifyJSONNumberV2(lit)
	switch class {
	case jsonNumberV2Invalid:
		return nil, &InvalidNumberError{Path: w.path.String(), Reason: reasonNotJSONNumber}
	case jsonNumberV2Integer:
		return &opencdcv2.Value{Kind: &opencdcv2.Value_Int64{Int64: n}}, nil
	case jsonNumberV2Decimal:
		// The decimal kind stores the literal as written: it is already a
		// valid JSON number (scanJSONNumber accepted it), so it round-trips
		// exactly and never passes through a float64.
		return &opencdcv2.Value{Kind: &opencdcv2.Value_Decimal{Decimal: lit}}, nil
	default:
		panic("unreachable json.Number class")
	}
}

// -- From Proto To Go ---------------------------------------------------------

// valueFromProtoV2 converts an opencdc.v2.Value back to the Go value a built-in
// plugin sees: int64 for the int64 kind, uint64 for the uint64 kind, []byte for
// the bytes kind, and json.Number for the decimal kind (so it re-encodes as the
// same decimal string). NaN and ±Inf come back as float64.
func valueFromProtoV2(v *opencdcv2.Value) (any, error) {
	if v == nil {
		return nil, ErrInvalidProtoDataType
	}
	switch k := v.Kind.(type) {
	case *opencdcv2.Value_Null:
		return nil, nil //nolint:nilnil // a null value is the nil Go value, with no error
	case *opencdcv2.Value_Bool:
		return k.Bool, nil
	case *opencdcv2.Value_Double:
		return k.Double, nil
	case *opencdcv2.Value_String_:
		return k.String_, nil
	case *opencdcv2.Value_Int64:
		return k.Int64, nil
	case *opencdcv2.Value_Uint64:
		return k.Uint64, nil
	case *opencdcv2.Value_Bytes:
		return k.Bytes, nil
	case *opencdcv2.Value_List:
		return listFromProtoV2(k.List)
	case *opencdcv2.Value_Struct:
		return structFromProtoV2(k.Struct)
	case *opencdcv2.Value_Decimal:
		return json.Number(k.Decimal), nil
	case nil:
		// A Value with no kind set is malformed, never "null".
		return nil, ErrInvalidProtoDataType
	default:
		return nil, ErrInvalidProtoDataType
	}
}

func structFromProtoV2(s *opencdcv2.Struct) (map[string]any, error) {
	if s == nil {
		return nil, ErrInvalidProtoDataType
	}
	out := make(map[string]any, len(s.Fields))
	for k, v := range s.Fields {
		val, err := valueFromProtoV2(v)
		if err != nil {
			return nil, fmt.Errorf("error converting field %q: %w", k, err)
		}
		out[k] = val
	}
	return out, nil
}

func listFromProtoV2(l *opencdcv2.List) ([]any, error) {
	if l == nil {
		return nil, ErrInvalidProtoDataType
	}
	out := make([]any, len(l.Items))
	for i, v := range l.Items {
		val, err := valueFromProtoV2(v)
		if err != nil {
			return nil, fmt.Errorf("error converting item %d: %w", i, err)
		}
		out[i] = val
	}
	return out, nil
}

// -- json.Number classification ----------------------------------------------

type jsonNumberV2Class int

const (
	jsonNumberV2Invalid jsonNumberV2Class = iota
	jsonNumberV2Integer                   // integer within the int64 range
	jsonNumberV2Decimal                   // a valid JSON number that is not an integer in the int64 range
)

// classifyJSONNumberV2 classifies a json.Number literal without big-number
// arithmetic: one scan of the literal, whatever its exponent. When the class is
// jsonNumberV2Integer, n is the parsed int64. It reuses scanJSONNumber (the v1
// grammar scan), so a literal that is not a JSON number (RFC 8259) — a hex
// float, "Infinity", a leading "+" — is reported as jsonNumberV2Invalid, never
// parsed leniently.
func classifyJSONNumberV2(s string) (class jsonNumberV2Class, n int64) {
	p, ok := scanJSONNumber(s)
	if !ok {
		return jsonNumberV2Invalid, 0
	}

	// Significant digits d and effective exponent e: value = ±d * 10^e.
	d := strings.TrimLeft(p.intDigits+p.fracDigits, "0")
	e := p.exp - int64(len(p.fracDigits))
	trimmed := strings.TrimRight(d, "0")
	e += int64(len(d) - len(trimmed))
	d = trimmed

	switch {
	case d == "":
		return jsonNumberV2Integer, 0 // zero, in any notation
	case e < 0:
		return jsonNumberV2Decimal, 0 // has a fractional part
	case int64(len(d))+e > 19:
		return jsonNumberV2Decimal, 0 // at least 10^19, beyond the int64 range
	}

	lit := d + strings.Repeat("0", int(e))
	if s[0] == '-' {
		lit = "-" + lit
	}
	n, err := strconv.ParseInt(lit, 10, 64)
	if err != nil {
		// 19 digits but outside the int64 range (e.g. 9223372036854775808).
		return jsonNumberV2Decimal, 0
	}
	return jsonNumberV2Integer, n
}
