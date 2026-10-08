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

package protobuf

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/twmb/franz-go/pkg/sr"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// messageForIndex reads the Confluent message index at the start of b and
// returns the message descriptor it selects and the payload that follows it.
//
// b is a Confluent Protobuf value with the 5-byte header (magic byte and
// schema ID) already stripped: a zigzag varint count, that many zigzag varint
// indexes, then the Protobuf payload. A count of 0 is a shortcut for the
// index [0]. The first index selects a top-level message in declaration
// order, and each further index selects a nested message of the one before.
//
// Any index that doesn't name a message in the schema is an error matching
// ErrMessageIndex. It never falls back to another message: decoding a payload
// against the wrong descriptor can succeed and produce garbage.
func (s *Serde) messageForIndex(b []byte) (protoreflect.MessageDescriptor, []byte, error) {
	if s.maxIndexDepth == 0 {
		return nil, nil, fmt.Errorf("%w: schema declares no messages", ErrMessageIndex)
	}

	// Check the count before DecodeIndex sees it. DecodeIndex allocates the
	// index slice from the count, and on 32-bit platforms its own bound check
	// compares a truncated int: a count like 1<<32+1 passes the check and
	// panics in make. The count can't usefully exceed the schema's nesting
	// depth anyway.
	count, n := binary.Varint(b)
	switch {
	case len(b) == 0:
		return nil, nil, fmt.Errorf("%w: missing (empty input)", ErrMessageIndex)
	case n <= 0: // 0: truncated, < 0: overflows 64 bits
		return nil, nil, fmt.Errorf("%w: count is not a valid varint", ErrMessageIndex)
	case count < 0:
		return nil, nil, fmt.Errorf("%w: negative count %d", ErrMessageIndex, count)
	case count > int64(s.maxIndexDepth):
		return nil, nil, fmt.Errorf("%w: count %d exceeds the schema's message nesting depth %d", ErrMessageIndex, count, s.maxIndexDepth)
	}

	index, payload, err := (&sr.ConfluentHeader{}).DecodeIndex(b, s.maxIndexDepth)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrMessageIndex, err)
	}

	// DecodeIndex converts each int64 to int, which truncates on 32-bit
	// platforms, so a huge index could alias a small valid one. Re-encoding
	// the decoded index and comparing it with the bytes consumed catches
	// that. It also rejects over-long varints, which no Confluent producer
	// writes.
	consumed := b[:len(b)-len(payload)]
	if !bytes.Equal(consumed, encodeIndex(count, index)) {
		return nil, nil, fmt.Errorf("%w: non-canonical or out-of-range encoding % x", ErrMessageIndex, consumed)
	}

	var md protoreflect.MessageDescriptor
	mds := s.file.Messages()
	for depth, i := range index {
		if i < 0 || i >= mds.Len() {
			return nil, nil, fmt.Errorf("%w: index %v: entry %d is %d, but there are %d messages at that level", ErrMessageIndex, index, depth, i, mds.Len())
		}
		md = mds.Get(i)
		mds = md.Messages()
	}
	if md.IsMapEntry() {
		// Map entries are synthetic nested messages; no producer encodes
		// one as a top-level value.
		return nil, nil, fmt.Errorf("%w: index %v selects map entry %s", ErrMessageIndex, index, md.FullName())
	}
	return md, payload, nil
}

// encodeIndex is the encoding Confluent producers write for index: the
// single byte 0 when the count was the [0] shortcut, otherwise the count and
// each index as zigzag varints.
func encodeIndex(count int64, index []int) []byte {
	if count == 0 {
		return []byte{0}
	}
	out := binary.AppendVarint(nil, count)
	for _, i := range index {
		out = binary.AppendVarint(out, int64(i))
	}
	return out
}

// messageNestingDepth returns the length of the longest message index the
// file can have: 1 for top-level messages only, 0 for no messages.
func messageNestingDepth(mds protoreflect.MessageDescriptors) int {
	deepest := 0
	for i := range mds.Len() {
		deepest = max(deepest, 1+messageNestingDepth(mds.Get(i).Messages()))
	}
	return deepest
}
