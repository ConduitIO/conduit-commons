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
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"sync"

	"google.golang.org/protobuf/encoding/protowire"
)

// MarshalV2 returns the opencdc.v2 wire encoding of r: the exact bytes that
// proto.MarshalOptions{Deterministic: true} produces for the record's ToProtoV2
// tree. It is equivalent to AppendV2(nil). The returned slice is a freshly
// allocated buffer owned by the caller.
//
// MarshalV2 is the fast path for the record → wire boundary. It writes the
// protobuf directly from the Go record, so it does not build the intermediate
// opencdc.v2 message tree that ToProtoV2 builds and it does not run proto
// reflection, both of which allocate per record. Map fields (metadata and
// structured data) are emitted in sorted key order, the same order protobuf's
// deterministic marshaler uses.
func (r Record) MarshalV2() ([]byte, error) {
	return r.AppendV2(nil)
}

// AppendV2 appends the opencdc.v2 wire encoding of r to dst and returns the
// extended slice. The bytes are identical to proto.MarshalOptions{Deterministic:
// true} applied to the record's ToProtoV2 tree: the same field numbers, types
// and values, with map entries in sorted key order.
//
// dst is reused when it has spare capacity, so a caller that marshals many
// records (for example the connector SDK reading a batch) can pass a buffer it
// keeps across calls and allocate nothing per record:
//
//	var buf []byte
//	for _, rec := range records {
//		buf, err = rec.AppendV2(buf[:0])
//		// write buf to the wire, then loop
//	}
//
// The encoder is pooled and reuses its scratch state, so AppendV2 allocates
// only when dst must grow. It is safe for concurrent use.
func (r Record) AppendV2(dst []byte) ([]byte, error) {
	w := v2MarshalerPool.Get().(*v2Marshaler) //nolint:forcetypeassert // the pool only ever holds *v2Marshaler
	defer v2MarshalerPool.Put(w)
	return w.record(dst, r)
}

// v2MarshalerPool reuses the marshaler and its scratch state (the error path
// stack and the per-depth key slices used for deterministic map ordering) across
// calls, so the happy path allocates nothing beyond the output buffer.
var v2MarshalerPool = sync.Pool{
	New: func() any { return &v2Marshaler{path: make(pathStack, 0, 8)} },
}

// v2Marshaler writes the opencdc.v2 wire encoding of a record. It mirrors the
// type mapping of v2ValueEncoder (see proto_v2_values.go) and the error
// wrapping of Record.ToProtoV2, so the bytes and the errors are identical to the
// ToProtoV2 + proto.Marshal path.
type v2Marshaler struct {
	// path records the location of the value being encoded, for error messages.
	// It is rendered only on error, so the happy path builds no strings. The
	// backing array is reused across calls.
	path pathStack

	// keys holds one scratch []string per nesting depth, used to sort map keys
	// for deterministic output. Depth d uses keys[d]; sibling maps at the same
	// depth reuse their slot sequentially, so the scratch never grows past the
	// maximum nesting depth of the data.
	keys [][]string
}

// record writes field 1 (position), 2 (operation), 3 (metadata), 4 (key) and 5
// (payload), in field-number order as the wire format requires.
func (w *v2Marshaler) record(dst []byte, r Record) ([]byte, error) {
	if len(r.Position) > 0 {
		dst = protowire.AppendTag(dst, 1, protowire.BytesType)
		dst = protowire.AppendBytes(dst, r.Position)
	}
	if r.Operation != 0 {
		dst = protowire.AppendTag(dst, 2, protowire.VarintType)
		dst = protowire.AppendVarint(dst, uint64(r.Operation)) //nolint:gosec // Operation is a non-negative enum
	}
	if len(r.Metadata) > 0 {
		var err error
		dst, err = w.metadata(dst, r.Metadata)
		if err != nil {
			return dst, err
		}
	}
	if r.Key != nil {
		var err error
		dst, err = w.data(dst, 4, r.Key)
		if err != nil {
			return dst, wrapProtoError("error converting key", prefixNumberErrorPath(err, fieldKey))
		}
	}
	// Payload is always present: ToProtoV2 always allocates a Change, and a
	// non-nil message field is emitted even when empty (the zero record encodes
	// as "2a00").
	var err error
	dst, err = w.change(dst, 5, r.Payload)
	if err != nil {
		return dst, wrapProtoError("error converting payload", prefixNumberErrorPath(err, fieldPayload))
	}
	return dst, nil
}

// metadata writes the metadata map field (3) as repeated entry messages with a
// string key (1) and string value (2), keys in sorted order.
func (w *v2Marshaler) metadata(dst []byte, m Metadata) ([]byte, error) {
	keys := w.keysAt(0)
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	w.keys[0] = keys // persist the grown backing array for reuse
	for _, k := range keys {
		var err error
		dst, err = appendMessage(dst, 3, func(dst []byte) ([]byte, error) {
			dst = protowire.AppendTag(dst, 1, protowire.BytesType)
			dst = protowire.AppendString(dst, k)
			dst = protowire.AppendTag(dst, 2, protowire.BytesType)
			dst = protowire.AppendString(dst, m[k])
			return dst, nil
		})
		if err != nil {
			return dst, err
		}
	}
	return dst, nil
}

// change writes the change message at field num with before (1) and after (2).
// It mirrors Change.ToProtoV2's error wrapping.
func (w *v2Marshaler) change(dst []byte, num protowire.Number, c Change) ([]byte, error) {
	return appendMessage(dst, num, func(dst []byte) ([]byte, error) {
		if c.Before != nil {
			var err error
			dst, err = w.data(dst, 1, c.Before)
			if err != nil {
				return dst, wrapProtoError("error converting before", prefixNumberErrorPath(err, fieldBefore))
			}
		}
		if c.After != nil {
			var err error
			dst, err = w.data(dst, 2, c.After)
			if err != nil {
				return dst, wrapProtoError("error converting after", prefixNumberErrorPath(err, fieldAfter))
			}
		}
		return dst, nil
	})
}

// data writes the Data message at field num: raw data as bytes (1) or structured
// data as a Struct message (2). It mirrors Data.ToProtoV2's error wrapping.
func (w *v2Marshaler) data(dst []byte, num protowire.Number, d Data) ([]byte, error) {
	return appendMessage(dst, num, func(dst []byte) ([]byte, error) {
		switch d := d.(type) {
		case RawData:
			// The raw_data oneof is present even when d is empty: RawData(nil)
			// encodes as "0a00" inside the Data message.
			dst = protowire.AppendTag(dst, 1, protowire.BytesType)
			dst = protowire.AppendBytes(dst, d)
			return dst, nil
		case StructuredData:
			var err error
			dst, err = appendMessage(dst, 2, func(dst []byte) ([]byte, error) {
				return w.structBody(dst, map[string]any(d))
			})
			if err != nil {
				return dst, wrapStructuredDataError(err)
			}
			return dst, nil
		default:
			return dst, ErrInvalidProtoDataType
		}
	})
}

// structBody writes the Struct message body: a repeated fields entry (1) with a
// string key (1) and a Value message (2), keys in sorted order.
func (w *v2Marshaler) structBody(dst []byte, m map[string]any) ([]byte, error) {
	depth := len(w.path)
	keys := w.keysAt(depth)
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	w.keys[depth] = keys // persist the grown backing array for reuse
	for _, k := range keys {
		w.path = append(w.path, pathSegment{key: k})
		var err error
		dst, err = appendMessage(dst, 1, func(dst []byte) ([]byte, error) {
			dst = protowire.AppendTag(dst, 1, protowire.BytesType)
			dst = protowire.AppendString(dst, k)
			return w.value(dst, 2, m[k])
		})
		w.path = w.path[:len(w.path)-1]
		if err != nil {
			return dst, err
		}
	}
	return dst, nil
}

// listBody writes the List message body: repeated Value messages (1), in order.
func (w *v2Marshaler) listBody(dst []byte, l []any) ([]byte, error) {
	for i, v := range l {
		w.path = append(w.path, pathSegment{index: i, isIndex: true})
		var err error
		dst, err = w.value(dst, 1, v)
		w.path = w.path[:len(w.path)-1]
		if err != nil {
			return dst, err
		}
	}
	return dst, nil
}

// value writes a Value message at field num, wrapping valueBody.
func (w *v2Marshaler) value(dst []byte, num protowire.Number, v any) ([]byte, error) {
	return appendMessage(dst, num, func(dst []byte) ([]byte, error) {
		return w.valueBody(dst, v)
	})
}

// valueBody writes the Value message body: exactly one kind field. It mirrors
// v2ValueEncoder.value and v2ValueEncoder.scalar, keeping the Go type: int*
// becomes int64 (5), uint* becomes uint64 (6), []byte becomes bytes (7), and
// json.Number becomes int64 (5) or decimal (10). float* becomes the double kind
// (3), including NaN and ±Inf.
func (w *v2Marshaler) valueBody(dst []byte, v any) ([]byte, error) {
	if dst, handled, err := w.scalarBody(dst, v); handled {
		return dst, err
	}
	switch v := v.(type) {
	case map[string]any:
		return appendMessage(dst, 9, func(dst []byte) ([]byte, error) {
			return w.structBody(dst, v)
		})
	case []any:
		return appendMessage(dst, 8, func(dst []byte) ([]byte, error) {
			return w.listBody(dst, v)
		})
	default:
		// Mirrors v2ValueEncoder.value: reject types a record does not carry
		// (invariant 6), instead of silently coercing or dropping them.
		return dst, fmt.Errorf("invalid type: %T", v)
	}
}

// scalarBody writes the scalar Value kinds. handled is false when v is a
// container or any other type valueBody handles.
func (w *v2Marshaler) scalarBody(dst []byte, v any) ([]byte, bool, error) {
	switch v := v.(type) {
	case nil:
		return appendVarintField(dst, 1, 0), true, nil // null
	case bool:
		return appendVarintField(dst, 2, boolToVarint(v)), true, nil
	case string:
		return appendStringField(dst, 4, v), true, nil
	case float64:
		return appendFixed64Field(dst, 3, math.Float64bits(v)), true, nil
	case float32:
		return appendFixed64Field(dst, 3, math.Float64bits(float64(v))), true, nil
	case int:
		return w.appendInt64(dst, int64(v)), true, nil
	case int8:
		return w.appendInt64(dst, int64(v)), true, nil
	case int16:
		return w.appendInt64(dst, int64(v)), true, nil
	case int32:
		return w.appendInt64(dst, int64(v)), true, nil
	case int64:
		return w.appendInt64(dst, v), true, nil
	case uint:
		return w.appendUint64(dst, uint64(v)), true, nil
	case uint8:
		return w.appendUint64(dst, uint64(v)), true, nil
	case uint16:
		return w.appendUint64(dst, uint64(v)), true, nil
	case uint32:
		return w.appendUint64(dst, uint64(v)), true, nil
	case uint64:
		return w.appendUint64(dst, v), true, nil
	case []byte:
		return appendBytesField(dst, 7, v), true, nil
	case json.Number:
		b, err := w.jsonNumber(dst, string(v))
		return b, true, err
	default:
		return dst, false, nil
	}
}

func (w *v2Marshaler) appendInt64(dst []byte, v int64) []byte {
	return appendVarintField(dst, 5, uint64(v)) //nolint:gosec // protobuf encodes int64 as its two's-complement uint64
}

func (w *v2Marshaler) appendUint64(dst []byte, v uint64) []byte {
	return appendVarintField(dst, 6, v)
}

// jsonNumber writes a json.Number exactly as v2ValueEncoder.jsonNumber does: an
// integer in the int64 range becomes int64 (5), any other valid JSON number
// becomes decimal (10, its literal), and an invalid literal is an error.
func (w *v2Marshaler) jsonNumber(dst []byte, lit string) ([]byte, error) {
	class, n := classifyJSONNumberV2(lit)
	switch class {
	case jsonNumberV2Invalid:
		return dst, &InvalidNumberError{Path: w.path.String(), Reason: reasonNotJSONNumber}
	case jsonNumberV2Integer:
		dst = protowire.AppendTag(dst, 5, protowire.VarintType)
		dst = protowire.AppendVarint(dst, uint64(n)) //nolint:gosec // protobuf encodes int64 as its two's-complement uint64
		return dst, nil
	case jsonNumberV2Decimal:
		dst = protowire.AppendTag(dst, 10, protowire.BytesType)
		dst = protowire.AppendString(dst, lit)
		return dst, nil
	default:
		panic("unreachable json.Number class")
	}
}

// keysAt returns the scratch key slice for nesting depth d, growing the stack if
// needed. The slice is reset to length zero; its backing array is reused, so
// sorting map keys allocates nothing after the first time a depth is reached.
func (w *v2Marshaler) keysAt(d int) []string {
	for len(w.keys) <= d {
		w.keys = append(w.keys, nil)
	}
	w.keys[d] = w.keys[d][:0]
	return w.keys[d]
}

// appendVarintField appends a varint-typed field with the given tag and value.
func appendVarintField(dst []byte, num protowire.Number, v uint64) []byte {
	return protowire.AppendVarint(protowire.AppendTag(dst, num, protowire.VarintType), v)
}

// appendStringField appends a length-delimited (string) field.
func appendStringField(dst []byte, num protowire.Number, v string) []byte {
	return protowire.AppendString(protowire.AppendTag(dst, num, protowire.BytesType), v)
}

// appendBytesField appends a length-delimited (bytes) field.
func appendBytesField(dst []byte, num protowire.Number, v []byte) []byte {
	return protowire.AppendBytes(protowire.AppendTag(dst, num, protowire.BytesType), v)
}

// appendFixed64Field appends a fixed64-typed field.
func appendFixed64Field(dst []byte, num protowire.Number, v uint64) []byte {
	return protowire.AppendFixed64(protowire.AppendTag(dst, num, protowire.Fixed64Type), v)
}

func boolToVarint(b bool) uint64 {
	if b {
		return 1
	}
	return 0
}

// appendMessage writes a length-delimited sub-message for field num. It reserves
// the maximum varint length prefix, lets fn append the body, then moves the body
// left to fit the real length prefix. This is allocation-free: the reservation
// uses spare capacity of dst, and the copy moves the body within dst.
func appendMessage(dst []byte, num protowire.Number, fn func([]byte) ([]byte, error)) ([]byte, error) {
	dst = protowire.AppendTag(dst, num, protowire.BytesType)
	const maxLen = binary.MaxVarintLen64 // 10
	bodyStart := len(dst)
	dst = append(dst, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0) // reserve maxLen bytes
	contentStart := bodyStart + maxLen
	var err error
	dst, err = fn(dst)
	if err != nil {
		return dst, err
	}
	bodyLen := len(dst) - contentStart
	var tmp [maxLen]byte
	lenPrefix := protowire.AppendVarint(tmp[:0], uint64(bodyLen)) //nolint:gosec // bodyLen is a non-negative length
	m := len(lenPrefix)
	copy(dst[bodyStart+m:], dst[contentStart:]) // shift body left by (maxLen - m)
	dst = dst[:bodyStart+m+bodyLen]
	copy(dst[bodyStart:], lenPrefix)
	return dst, nil
}
