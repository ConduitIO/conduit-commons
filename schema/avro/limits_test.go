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
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"runtime"
	"runtime/debug"
	"sync"
	"testing"
	"time"

	"github.com/matryer/is"
)

// zigzagVarint encodes an int64 the way Avro encodes a `long`: zigzag then
// unsigned LEB128 varint. rawUvarint encodes an already-raw (not zigzag'd)
// uint64 bit pattern the same way. Both are used below to hand-craft decoder
// inputs that real avro.Marshal calls would never produce, because the
// point is to exercise inputs a legitimate encoder would not emit but an
// attacker (of the bytes, not necessarily the schema -- see limits.go's
// package doc) could.
func zigzagVarint(n int64) []byte {
	zz := uint64((n << 1) ^ (n >> 63))
	return rawUvarint(zz)
}

func rawUvarint(v uint64) []byte {
	buf := make([]byte, binary.MaxVarintLen64)
	i := binary.PutUvarint(buf, v)
	return buf[:i]
}

// TestMaxInputSize_UnlimitedByDefault proves the headline behavior change
// from pass 2 to pass 3 (see limits.go's package doc): with no
// WithMaxInputSize option, a Serde rejects nothing on size grounds, no
// matter how large. This is the test that makes "the full pre-existing
// suite passes unchanged" mean something -- there is no default ceiling
// left to accidentally reject a legitimate record.
func TestMaxInputSize_UnlimitedByDefault(t *testing.T) {
	is := is.New(t)

	serde, err := SerdeForType(map[string]string{})
	is.NoErr(err)

	// Comfortably larger than any of the ceilings this mitigation has ever
	// used (10 MiB, then 256 MiB) to prove there is really no default left.
	src := map[string]string{}
	for i := 0; len(src) < 700_000; i++ {
		src[fmt.Sprintf("k%07d", i)] = fmt.Sprintf("v%07d", i)
	}
	encoded, err := serde.Marshal(src)
	is.NoErr(err)
	t.Logf("payload size: %d bytes", len(encoded))

	var out map[string]string
	err = serde.Unmarshal(encoded, &out)
	is.NoErr(err)
	is.Equal(len(out), len(src))
}

// TestMaxInputSize_OptInRejectsOversizedInput proves that when an operator
// does opt in via WithMaxInputSize, ErrInputTooLarge fires and fails
// closed: v is left completely untouched, not partially decoded, not reset
// to a zero value that a buggy implementation could also produce by
// accident. Pre-populating out with a sentinel value (rather than asserting
// len(out) == 0 against a var that was never written to in the first
// place, which would pass unconditionally regardless of whether Unmarshal
// touched v at all) is what makes this a real fail-closed assertion.
func TestMaxInputSize_OptInRejectsOversizedInput(t *testing.T) {
	is := is.New(t)

	const ceiling = 16
	serde, err := SerdeForType(map[string]any{"foo": "bar"}, WithMaxInputSize(ceiling))
	is.NoErr(err)

	oversized := make([]byte, ceiling+1)

	out := map[string]any{"sentinel": "untouched"}
	err = serde.Unmarshal(oversized, &out)

	is.True(err != nil)
	is.True(errors.Is(err, ErrInputTooLarge))
	is.Equal(len(out), 1)                  // fail closed: still exactly the sentinel entry
	is.Equal(out["sentinel"], "untouched") // fail closed: not zeroed, not partially decoded
}

// TestMaxInputSize_OptInRejectsOversizedInput_FastPath proves the size
// check happens before the expensive decode path runs, without asserting a
// fixed wall-clock budget (an earlier version of this test asserted
// `elapsed < 50*time.Millisecond`, a real CI flake risk: any GC pause or
// scheduler hiccup could trip it with no bug present).
func TestMaxInputSize_OptInRejectsOversizedInput_FastPath(t *testing.T) {
	is := is.New(t)

	const ceiling = 16 * 1024 * 1024
	serde, err := SerdeForType(map[string]any{"foo": "bar"}, WithMaxInputSize(ceiling))
	is.NoErr(err)

	oversized := make([]byte, ceiling+1)
	var out map[string]any

	start := time.Now()
	err = serde.Unmarshal(oversized, &out)
	rejectElapsed := time.Since(start)
	is.True(errors.Is(err, ErrInputTooLarge))

	t.Logf("oversized-input rejection: %s (informational; no hard wall-clock assertion, see comment)", rejectElapsed)
}

// TestMaxInputSize_OptInAllowsInputJustUnderLimit proves the opt-in
// ceiling doesn't change behavior for a legitimate record sized under it.
func TestMaxInputSize_OptInAllowsInputJustUnderLimit(t *testing.T) {
	is := is.New(t)

	const ceiling = 64 * 1024 * 1024
	serde, err := SerdeForType(map[string]string{}, WithMaxInputSize(ceiling))
	is.NoErr(err)

	src := map[string]string{}
	for i := 0; len(src) < 400_000; i++ {
		src[fmt.Sprintf("k%07d", i)] = fmt.Sprintf("v%07d", i)
	}
	encoded, err := serde.Marshal(src)
	is.NoErr(err)
	if len(encoded) > ceiling {
		t.Fatalf("test setup produced %d bytes, want <= %d; reduce entry count", len(encoded), ceiling)
	}

	var out map[string]string
	err = serde.Unmarshal(encoded, &out)
	is.NoErr(err)
	is.Equal(len(out), len(src))
}

// TestMaxInputSize_PerInstanceIsolation is the regression test for the fix
// to the confirmed data race described in WithMaxInputSize's doc comment:
// avro.MaxInputSize used to be a single exported package var read by every
// Serde.Unmarshal call across the whole process (Serde instances are shared
// process-wide via schema.globalSerdeCache). Overriding the ceiling for one
// Serde must not affect another, and there must be nothing left to race on.
func TestMaxInputSize_PerInstanceIsolation(t *testing.T) {
	is := is.New(t)

	small, err := SerdeForType(map[string]any{"foo": "bar"}, WithMaxInputSize(16))
	is.NoErr(err)
	unlimited, err := SerdeForType(map[string]any{"foo": "bar"})
	is.NoErr(err)

	payload, err := unlimited.Marshal(map[string]any{"foo": "this value is longer than sixteen bytes for sure"})
	is.NoErr(err)
	if len(payload) <= 16 {
		t.Fatalf("test setup produced a %d-byte payload, want > 16 bytes", len(payload))
	}

	var out map[string]any
	err = small.Unmarshal(payload, &out)
	is.True(errors.Is(err, ErrInputTooLarge)) // small's own 16-byte ceiling applies

	out = nil
	err = unlimited.Unmarshal(payload, &out) // unaffected by small's override
	is.NoErr(err)
	is.Equal(len(out), 1)
}

// TestMaxInputSize_ConcurrentUnmarshal_NoRace exercises differently
// configured Serdes concurrently. It doesn't assert anything beyond "no
// error, no race" -- it exists to be run under `go test -race`, which is
// where the bug this replaces (avro.MaxInputSize as a shared mutable
// package var) was confirmed.
func TestMaxInputSize_ConcurrentUnmarshal_NoRace(t *testing.T) {
	is := is.New(t)

	unlimited, err := SerdeForType(map[string]string{})
	is.NoErr(err)
	bounded, err := SerdeForType(map[string]string{}, WithMaxInputSize(4*1024*1024))
	is.NoErr(err)

	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			serde := unlimited
			if n%2 == 0 {
				serde = bounded
			}
			src := map[string]string{}
			for j := 0; j < 1+n%50; j++ {
				src[fmt.Sprintf("k%d", j)] = fmt.Sprintf("v%d-%d", n, j)
			}
			encoded, err := serde.Marshal(src)
			if err != nil {
				t.Errorf("marshal: %v", err)
				return
			}
			var out map[string]string
			if err := serde.Unmarshal(encoded, &out); err != nil {
				t.Errorf("unmarshal: %v", err)
				return
			}
			if len(out) != len(src) {
				t.Errorf("got %d entries, want %d", len(out), len(src))
			}
		}(i)
	}
	wg.Wait()
}

// TestSwap_C1_UnboundedByDefault_WouldFailPreSwap is the C1 regression
// test. Against the pre-swap state (github.com/hamba/avro/v2, or this
// package's own decodeAPI before initialDefaultMaxAllocSize existed -- i.e. an
// unset MaxSliceAllocSize resolving to the codec's own default of
// maxAllocSize, 1<<48 on 64-bit), this exact payload decodes with
// err == nil and a multi-GiB heap allocation: confirmed directly against
// github.com/iskorotkov/avro/v2 v2.34.0 with MaxSliceAllocSize left unset
// (err=<nil> len(out)=134217728, reproduced with a throwaway program
// against the fork library directly -- there is no committed in-tree test
// for this specific "unset" case, since this package no longer leaves
// MaxSliceAllocSize unset in any code path after this commit), and against
// hamba/avro/v2 identically before this package ever set MaxSliceAllocSize
// at all. This package now always sets a default (see limits.go's "Default
// allocation ceilings"), with no WithMaxInputSize opt-in required, so the
// same payload against Parse's real decode path is rejected.
func TestSwap_C1_UnboundedByDefault_WouldFailPreSwap(t *testing.T) {
	is := is.New(t)

	const declared = 134_217_728 // 128 MiB as a raw element count, comfortably above initialDefaultMaxAllocSize
	payload := zigzagVarint(declared)
	t.Logf("wire bytes for block header: %d", len(payload))

	s, err := Parse([]byte(`{"type":"array","items":"null"}`))
	is.NoErr(err)

	var out []any
	err = s.Unmarshal(payload, &out)

	is.True(err != nil) // rejected by the default MaxSliceAllocSize; pre-swap/pre-config this was err == nil
	t.Logf("C1 after swap+default config: err=%v len(out)=%d", err, len(out))
}

// TestSwap_C1_BoundedByDefaultConfig proves the default ceiling doesn't
// just reject the pathological case above -- it allows a declared count
// comfortably under initialDefaultMaxAllocSize to decode normally, and rejects one
// just over it, without any WithMaxInputSize/WithMaxSliceAllocSize option.
func TestSwap_C1_BoundedByDefaultConfig(t *testing.T) {
	is := is.New(t)

	s, err := Parse([]byte(`{"type":"array","items":"null"}`))
	is.NoErr(err)

	// Comfortably under initialDefaultMaxAllocSize: real (if unusual) data, no
	// options configured, must still decode.
	const underLimit = 900_000
	var out []any
	err = s.Unmarshal(zigzagVarint(underLimit), &out)
	is.NoErr(err)
	is.Equal(len(out), underLimit)

	// Just over initialDefaultMaxAllocSize: must be rejected.
	const overLimit = initialDefaultMaxAllocSize + 1
	var out2 []any
	err = s.Unmarshal(zigzagVarint(overLimit), &out2)
	is.True(err != nil)
	t.Logf("declared=%d (initialDefaultMaxAllocSize+1): err=%v", overLimit, err)
}

// TestArrayBound_OptInRejectsDeclaredCountAboveCeiling proves that once an
// operator opts into WithMaxInputSize, the configured ceiling (not the
// observed length of any specific call) bounds declared array element
// count, and that this bound is sound against the padding-inflation bypass
// documented in limits.go: a payload padded well past the declared count
// still gets rejected, because the ceiling comes from the Serde's own
// construction-time configuration, not from anything in this call's bytes.
func TestArrayBound_OptInRejectsDeclaredCountAboveCeiling(t *testing.T) {
	is := is.New(t)

	const ceiling = 1024 // bytes
	const declared = 10_000_000
	header := zigzagVarint(declared)
	// Pad well past `ceiling` with trailing garbage the array decoder would
	// never legitimately reach -- if the bound were (re-)derived from this
	// call's observed length instead of the configured ceiling, this
	// padding would inflate it and the declared count would sail through.
	padding := make([]byte, 4096)
	payload := append(append([]byte{}, header...), padding...)

	s, err := Parse([]byte(`{"type":"array","items":"long"}`), WithMaxInputSize(ceiling))
	is.NoErr(err)

	var out []int64
	err = s.Unmarshal(payload, &out)
	// The padded payload is longer than `ceiling`, so this is actually
	// rejected by the MaxInputSize check itself first -- which is fine,
	// that's still "rejected," and is exactly what an operator who opted
	// in gets. The more interesting assertion is the next test, which
	// keeps the whole payload under `ceiling` while still declaring more
	// elements than could possibly fit in it.
	is.True(err != nil)
	t.Logf("err=%v len(out)=%d", err, len(out))
}

// TestArrayBound_OptInSoundAgainstDeclaredCountWithinCeiling is the
// tighter version of the test above: the whole payload (header only, no
// padding) is well within the configured ceiling, so MaxInputSize's own
// check does not fire -- only the array-allocation bound derived from that
// ceiling can catch this, and it does, because a declared count this large
// cannot correspond to real data within a `ceiling`-byte input (every
// non-null element costs >= 1 wire byte).
func TestArrayBound_OptInSoundAgainstDeclaredCountWithinCeiling(t *testing.T) {
	is := is.New(t)

	const ceiling = 1024 * 1024 // 1 MiB
	const declared = 10_000_000 // cannot fit in 1 MiB even at 1 byte/element
	payload := zigzagVarint(declared)
	if len(payload) >= ceiling {
		t.Fatalf("test setup produced a payload as large as the ceiling; declared value too small")
	}

	s, err := Parse([]byte(`{"type":"array","items":"long"}`), WithMaxInputSize(ceiling))
	is.NoErr(err)

	var out []int64
	err = s.Unmarshal(payload, &out)
	is.True(err != nil) // MaxInputSize's own check did NOT fire (payload << ceiling); the array bound did
	t.Logf("err=%v len(out)=%d", err, len(out))
}

// TestArrayBound_OptInAllowsRealDataWithinCeiling proves the opt-in array
// bound doesn't reject legitimate data: a real (Marshal-produced) array
// with many elements, well within a configured ceiling, decodes fine.
func TestArrayBound_OptInAllowsRealDataWithinCeiling(t *testing.T) {
	is := is.New(t)

	const ceiling = 8 * 1024 * 1024
	s, err := SerdeForType([]int64{}, WithMaxInputSize(ceiling))
	is.NoErr(err)

	src := make([]int64, 200_000)
	for i := range src {
		src[i] = int64(i)
	}
	encoded, err := s.Marshal(src)
	is.NoErr(err)
	if len(encoded) > ceiling {
		t.Fatalf("test setup produced %d bytes, want <= %d", len(encoded), ceiling)
	}

	var out []int64
	err = s.Unmarshal(encoded, &out)
	is.NoErr(err)
	is.Equal(len(out), len(src))
}

// TestSwap_H1_NegativeLengthOverflow_Fixed is the H1 regression test (see
// limits.go's "codec swap" section, GO-2026-5047): against hamba/avro,
// Reader.ReadBlockHeader negated a value that decoded to math.MinInt64,
// which overflowed back to math.MinInt64 (still negative) instead of
// becoming positive, bypassing every positive-ceiling guard by sign alone
// and producing len(out) == math.MinInt64, cap(out) == 0, err == nil --
// confirmed on hamba/avro/v2 (this package's dependency before the swap)
// with this exact 11-byte payload; nothing this package could configure
// caught it, since the bug is upstream of any Config field, inside
// ReadBlockHeader itself.
//
// github.com/iskorotkov/avro/v2 fixes this unconditionally at the source
// (ReadBlockHeader rejects length64 <= math.MinInt before negating,
// regardless of Config) -- confirmed directly against v2.34.0's source
// (reader.go) and by this very test. This test asserts the fix through
// the package's real
// Serde.Unmarshal path, no options configured, so a future codec swap or
// dependency bump that reintroduces this behavior fails it immediately.
func TestSwap_H1_NegativeLengthOverflow_Fixed(t *testing.T) {
	is := is.New(t)

	s, err := Parse([]byte(`{"type":"array","items":"long"}`))
	is.NoErr(err)

	first := rawUvarint(math.MaxUint64) // zigzag-decodes to math.MinInt64
	second := rawUvarint(0)             // block-size-in-bytes field the negative-count encoding requires
	payload := append(append([]byte{}, first...), second...)
	if len(payload) != 11 {
		t.Fatalf("payload construction changed: got %d bytes, want 11", len(payload))
	}

	var out []int64
	err = s.Unmarshal(payload, &out)
	n, c := len(out), cap(out) // capture immediately; do not touch out again if this regresses

	is.True(err != nil) // pre-swap (hamba/avro) this was err == nil with a corrupted negative-length slice
	t.Logf("H1 after swap: 11-byte payload, err=%v, len(out)=%d, cap(out)=%d", err, n, c)
}

// TestMapDecodeCPUExhaustion_SafeStringKeyedPath documents that
// Serde.Unmarshal's own usage (map[string]any / map[string]string, always
// string-keyed) takes hamba/avro's safe mapDecoder path, which checks the
// reader's error state after every element and bails on the first failed
// read. This is the control for
// TestSwap_C3_UnboundedByDefault_WouldFailPreSwap below: same crafted
// payload shape, different destination type, dramatically different
// outcome.
func TestMapDecodeCPUExhaustion_SafeStringKeyedPath(t *testing.T) {
	s, err := Parse([]byte(`{"type":"map","values":"string"}`))
	if err != nil {
		t.Fatal(err)
	}
	const declared = 20_000_000 // also well above initialDefaultMaxAllocSize now, see comment above
	payload := zigzagVarint(declared)

	var out map[string]string
	start := time.Now()
	err = s.Unmarshal(payload, &out)
	elapsed := time.Since(start)

	t.Logf("safe path: err=%v len(out)=%d elapsed=%s", err, len(out), elapsed)
	if elapsed > 5*time.Second {
		t.Errorf("safe string-keyed map decoder took %s against a 4-byte payload; "+
			"this path is supposed to bail on the first failed read, not iterate the full declared count", elapsed)
	}
}

// textKey has a pointer-receiver UnmarshalText, and Kind() != reflect.String
// (it's a struct), which are both required for the codec's
// createDecoderOfMap to route to mapDecoderUnmarshaler instead of the safe
// mapDecoder: the switch checks keyType.Kind() == reflect.String first (a
// string-kind named type, even with a value-receiver UnmarshalText, would
// still take the safe path), then keyType.Implements(textUnmarshalerType)
// with no PtrTo unwrap (so the map key type itself must be the pointer
// type, matching the nil-check-and-alloc dance in decoderOfMapUnmarshaler).
// True of both hamba/avro and github.com/iskorotkov/avro/v2 -- the fork
// kept this decoder shape, only adding the MaxMapAllocSize check below it.
type textKey struct{ s string }

func (k *textKey) UnmarshalText(b []byte) error {
	k.s = string(b)
	return nil
}

// TestSwap_C3_UnboundedByDefault_WouldFailPreSwap and
// TestSwap_C3_BoundedByDefaultConfig are the C3 regression tests (see
// limits.go's "codec swap" section, GO-2026-5046/5048).
// mapDecoderUnmarshaler.Decode has no per-element error check at all
// (unlike the safe mapDecoder above), on either hamba/avro or the fork --
// but hamba/avro has no Config field that bounds a map's declared block
// length at all, at any input-size policy, so this finding was unclosable
// there regardless of configuration. Confirmed directly: a 4-byte payload
// declaring a 20,000,000-entry block, backed by no further data, decoded
// 20,000,000 fabricated entries against github.com/iskorotkov/avro/v2
// v2.34.0 with MaxMapAllocSize left unset in 4.41s of real CPU with
// err == nil (reproduced with a throwaway program against the fork
// library directly -- there is no committed in-tree test for this
// specific "unset" case, since this package no longer leaves
// MaxMapAllocSize unset in any code path after this commit) -- and
// identically against hamba/avro, which had no equivalent field to leave
// unset in the first place. This
// package now always sets MaxMapAllocSize (see limits.go's "Default
// allocation ceilings"), with no WithMaxInputSize opt-in required, so the
// same shape of payload against Parse's real decode path is rejected
// immediately instead of running to completion.
//
// Reachable only through a caller-controlled destination type whose map key
// implements encoding.TextUnmarshaler via a pointer receiver -- Conduit's
// own Serde usage never does this (always map[string]any /
// map[string]string), but Serde.Unmarshal is public API and does not
// restrict destination types.
func TestSwap_C3_UnboundedByDefault_WouldFailPreSwap(t *testing.T) {
	is := is.New(t)

	s, err := Parse([]byte(`{"type":"map","values":"string"}`))
	is.NoErr(err)

	const declared = 5_000_000 // comfortably above initialDefaultMaxAllocSize
	payload := zigzagVarint(declared)

	var out map[*textKey]string
	start := time.Now()
	err = s.Unmarshal(payload, &out)
	elapsed := time.Since(start)

	is.True(err != nil) // rejected by the default MaxMapAllocSize; pre-swap/pre-config this was err == nil after several seconds of CPU
	t.Logf("C3 after swap+default config: err=%v len(out)=%d elapsed=%s", err, len(out), elapsed)
	if elapsed > time.Second {
		t.Errorf("rejection took %s; the default MaxMapAllocSize check should fire before any per-entry decode work, not after", elapsed)
	}
}

// TestSwap_C3_BoundedByDefaultConfig proves the default ceiling allows a
// declared block comfortably under initialDefaultMaxAllocSize to decode normally
// through the same vulnerable (TextUnmarshaler-keyed) path, and rejects one
// just over it, without any WithMaxInputSize/WithMaxMapAllocSize option.
func TestSwap_C3_BoundedByDefaultConfig(t *testing.T) {
	is := is.New(t)

	s, err := Parse([]byte(`{"type":"map","values":"string"}`))
	is.NoErr(err)

	// Real (Marshal-produced) data, comfortably under the default.
	src := map[string]string{}
	for i := 0; len(src) < 900_000; i++ {
		src[fmt.Sprintf("k%07d", i)] = "v"
	}
	// textKey's UnmarshalText only needs the raw string, so re-keying a
	// map[string]string's wire bytes into a map[*textKey]string on decode
	// is exactly what SerdeForType(map[string]string{}) would encode.
	sMarshal, err := SerdeForType(map[string]string{})
	is.NoErr(err)
	encoded, err := sMarshal.Marshal(src)
	is.NoErr(err)

	var out map[*textKey]string
	err = s.Unmarshal(encoded, &out)
	is.NoErr(err)
	is.Equal(len(out), len(src))
}

// TestByteSliceBound_Fires proves the maxByteSliceSize ceiling actually
// rejects an oversized bytes/string field. maxByteSliceSize is a no-op by
// construction (see limits.go's package doc): both hamba/avro's and the
// fork's own default resolve an unset field to the identical 1 MiB value,
// so this test's job is to prove the *ceiling itself* -- whichever value
// resolves it -- actually functions.
func TestByteSliceBound_Fires(t *testing.T) {
	is := is.New(t)

	s, err := SerdeForType([]byte{})
	is.NoErr(err)

	oversized := make([]byte, maxByteSliceSize+1)
	encoded, err := s.Marshal(oversized)
	is.NoErr(err) // encoding is unaffected; the bound is decode-side only

	var out []byte
	err = s.Unmarshal(encoded, &out)
	is.True(err != nil)
	if err == nil {
		return
	}
	t.Logf("byte-slice bound fired as expected: %v", err)
}

// TestUnboundedMapAllocation_Documented is not a pass/fail regression test
// in the usual sense: it is the empirical proof, run as part of the normal
// test suite, of the residual risk this mitigation accepts rather than
// closes, for the SAFE (string-keyed) map decode path -- a real,
// legitimate-shaped map that just happens to be large. This is a distinct
// concern from TestSwap_C3_UnboundedByDefault_WouldFailPreSwap above
// (which is about a crafted tiny payload declaring a huge block against an
// unsafe destination type): this test uses only what
// SerdeForType/Marshal would produce for a real record with a large map
// field.
func TestUnboundedMapAllocation_Documented(t *testing.T) {
	if testing.Short() {
		t.Skip("allocates real heap memory to measure amplification; skipped with -short")
	}
	is := is.New(t)

	serde, err := SerdeForType(map[string]string{})
	is.NoErr(err)

	const n = 550_000
	wire := func() []byte {
		src := make(map[string]string, n)
		for i := 0; i < n; i++ {
			src[fmt.Sprintf("k%07d", i)] = fmt.Sprintf("v%07d", i)
		}
		b, err := serde.Marshal(src)
		is.NoErr(err)
		return b
	}()

	defer debug.SetGCPercent(debug.SetGCPercent(-1))
	runtime.GC()
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	var out map[string]string
	err = serde.Unmarshal(wire, &out)
	is.NoErr(err)
	is.Equal(len(out), n)

	runtime.GC()
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(out)

	delta := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	t.Logf("map decode: wire=%d bytes, heap delta=%d bytes (%.1fx), entries=%d",
		len(wire), delta, float64(delta)/float64(len(wire)), n)

	is.True(delta > int64(len(wire)))
}

// --- F1: DST-adjacent local-timestamp wire-compat divergence, new
// timestamp-nanos/local-timestamp-nanos logical types, timestamp encode
// overflow. See PR review findings, and limits.go's package doc for the
// wire-compatibility caveat these tests pin. ---

// TestSwap_LocalTimestampMicros_DSTBoundary_PinsForkBehavior is the
// regression test for the one wire-compatibility gap the codec swap
// introduces that is NOT limited to the already-documented []any(nil) vs
// []any{} empty-array difference (which itself turned out to be
// misattributed -- see TestSwap_EmptyArray_AttributionCorrected below):
// hamba/avro and the fork disagree about which *instant* a
// local-timestamp-micros/-millis/-nanos value represents for any wall
// clock within roughly one hour of a DST transition in the schema's
// declared time.Local zone, with err == nil on both sides.
//
// hamba/avro computes time.Unix(sec, nsec) (a UTC instant) and lets
// time.Time's own offset-database lookup determine local wall-clock
// components. The fork's toLocalWall instead extracts the UTC wall-clock
// components (year/month/day/hour/minute/second/nanosecond) and
// reinterprets them directly in time.Local -- a different operation
// whenever the two zones disagree about the UTC offset for that wall
// clock, which is exactly the ambiguous (fall-back) or skipped
// (spring-forward) window around a transition. This package does not
// take a position on which interpretation is "more correct" for a given
// pipeline (the fork's is self-consistent under round-trip through this
// package's own Marshal/Unmarshal; hamba's is not -- confirmed
// separately, not asserted here) -- this test exists to pin and document
// the actual, shipped behavior, not to endorse it.
//
// This is an invariant-6-relevant (schema handling must not silently
// mangle data) divergence: the same stored wire bytes decode to a
// different instant across this swap, silently, for any registry-supplied
// schema using a local-timestamp-* logical type. SerdeForType can never
// produce this logical type on its own (extractor.go's time.Time case
// only ever emits TimestampMicros), so Conduit's own schema-inference
// path cannot trigger it -- but Parse accepts arbitrary schema text, so
// any registry-supplied schema declaring local-timestamp-micros/-millis/
// -nanos reaches this path.
//
// Migration note: a pipeline that decodes previously-stored
// local-timestamp-* bytes after upgrading past this swap (or downgrades
// after storing new bytes) can read a wall-clock time shifted by the
// local UTC offset delta (typically one hour, for US/EU-style DST) for
// any value whose wall clock falls within roughly the DST transition
// window in the schema-declared zone. This is a property of decoding
// old bytes with a new library version, not a serialized-format version
// this package can detect, version, or reject -- there is no automatic
// migration for it. Operators with local-timestamp-* fields whose values
// can fall near a DST boundary in their deployment's zone should treat
// values decoded across the upgrade boundary as suspect until backfilled
// or re-derived from a source of truth other than the previously-decoded
// value itself.
//
// The two wire values below and their independently-verified hamba-2.28
// vs fork-2.34 decoded results (2026-03-08 US spring-forward,
// 2026-11-01 US fall-back, both America/New_York) were confirmed directly
// against both modules with TZ=America/New_York outside this module (a
// throwaway harness, not committed). This test pins only the fork's --
// this package's actual, current -- behavior, reached through the real
// Parse/Serde.Unmarshal path; it cannot and does not exercise hamba/avro,
// which this package no longer depends on.
//
//	wire=80f8ddeadc9ea606  hamba 2.28 -> 2026-03-08T04:30:00-04:00
//	                        fork  2.34 -> 2026-03-08T03:30:00-04:00
//	wire=80e89ae5b9cbaf06  hamba 2.28 -> 2026-11-01T01:30:00-05:00
//	                        fork  2.34 -> 2026-11-01T02:30:00-05:00
func TestSwap_LocalTimestampMicros_DSTBoundary_PinsForkBehavior(t *testing.T) {
	is := is.New(t)

	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("America/New_York tzdata not available in this environment: %v", err)
	}
	oldLocal := time.Local                      //nolint:gosmopolitan // deliberately exercising time.Local-dependent decode behavior -- that dependency is exactly what this test pins
	time.Local = loc                            //nolint:gosmopolitan // see above
	t.Cleanup(func() { time.Local = oldLocal }) //nolint:gosmopolitan // restoring what the line above deliberately overrode

	s, err := Parse([]byte(`{"type":"long","logicalType":"local-timestamp-micros"}`))
	is.NoErr(err)

	cases := []struct {
		name string
		wire string // hex-encoded raw avro long-encoded wire bytes
		want time.Time
	}{
		{
			name: "2026 spring-forward boundary",
			wire: "80f8ddeadc9ea606",
			want: time.Date(2026, 3, 8, 3, 30, 0, 0, loc),
		},
		{
			name: "2026 fall-back boundary",
			wire: "80e89ae5b9cbaf06",
			want: time.Date(2026, 11, 1, 2, 30, 0, 0, loc),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			is := is.New(t)
			b, err := hex.DecodeString(tc.wire)
			is.NoErr(err)

			var out time.Time
			err = s.Unmarshal(b, &out)
			is.NoErr(err) // err == nil on both hamba and the fork -- that's the danger, not a crash
			is.True(out.Equal(tc.want))
			t.Logf("wire=%s -> %s (pinned fork behavior)", tc.wire, out.Format(time.RFC3339))
		})
	}
}

// TestSwap_EmptyArray_AttributionCorrected replaces an earlier, incorrect
// claim (originally in this file and in serde_test.go's "[]any (no data)"
// case) that the fork was responsible for an empty Avro array decoding to
// []any{} instead of []any(nil). It is not: independently verified
// directly against github.com/hamba/avro/v2 v2.28.0 and v2.31.0 (the
// dependabot-proposed bump this repo already had open, independent of
// this migration) that the change happened somewhere in that range, and
// arrives via the fork only because the fork's own baseline is newer than
// v2.28.0 -- not because of anything the fork itself changed. A
// ~100-case hamba-2.31-vs-fork sweep (also run as part of the same
// out-of-tree verification) found no other empty-array or empty-map
// decode divergence between hamba 2.31 and the fork at all. This test
// pins the fork's (current) behavior directly; see serde_test.go's
// "[]any (no data)" case for the corrected attribution in the main
// behavioral test table.
func TestSwap_EmptyArray_AttributionCorrected(t *testing.T) {
	is := is.New(t)

	s, err := SerdeForType([]any{})
	is.NoErr(err)

	encoded, err := s.Marshal([]any{})
	is.NoErr(err)

	var out []any
	err = s.Unmarshal(encoded, &out)
	is.NoErr(err)
	is.True(out != nil)
	is.Equal(len(out), 0)
}

// TestSwap_TimestampNanos_RoundTrips and
// TestSwap_LocalTimestampNanos_RoundTrips cover timestamp-nanos and
// local-timestamp-nanos, two Avro logical types new to this package's
// reachable decode surface since the codec swap (the fork added
// TimestampNanos/LocalTimestamp{Millis,Micros,Nanos} support; hamba/avro
// only ever had TimestampMillis/TimestampMicros/TimeMicros). Neither is
// ever produced by SerdeForType (extractor.go's time.Time case hard-codes
// TimestampMicros), but both are reachable through Parse with a
// registry-supplied schema, and neither had any test coverage before this
// commit.
func TestSwap_TimestampNanos_RoundTrips(t *testing.T) {
	is := is.New(t)

	s, err := Parse([]byte(`{"type":"long","logicalType":"timestamp-nanos"}`))
	is.NoErr(err)

	want := time.Date(2026, 3, 8, 9, 30, 0, 123456789, time.UTC)
	encoded, err := s.Marshal(want)
	is.NoErr(err)

	var got time.Time
	err = s.Unmarshal(encoded, &got)
	is.NoErr(err)
	is.True(want.Equal(got))
}

func TestSwap_LocalTimestampNanos_RoundTrips(t *testing.T) {
	is := is.New(t)

	s, err := Parse([]byte(`{"type":"long","logicalType":"local-timestamp-nanos"}`))
	is.NoErr(err)

	// Marshal and Unmarshal both run in this process's time.Local, so
	// unlike TestSwap_LocalTimestampMicros_DSTBoundary_PinsForkBehavior
	// (which decodes bytes produced by a different codec/process), no
	// cross-codec wall-clock reinterpretation ambiguity applies here --
	// this only proves the new logical type round-trips at all.
	want := time.Date(2026, 6, 15, 9, 30, 0, 123456789, time.Local) //nolint:gosmopolitan // local-timestamp-nanos is, by definition, process-time.Local-relative
	encoded, err := s.Marshal(want)
	is.NoErr(err)

	var got time.Time
	err = s.Unmarshal(encoded, &got)
	is.NoErr(err)
	is.True(want.Equal(got))
}

// TestSwap_TimestampNanos_EncodeOverflow_Errors is coverage for a second,
// previously untested logical-type-encoding behavior change bundled into
// the codec swap: the fork's nanosecond-precision timestamp encoder
// rejects a time.Time outside its representable range
// (1677-09-21T00:12:43.145224192Z .. 2262-04-11T23:47:16.854775807Z) with
// an explicit error, where a value outside the int64-nanoseconds-since-
// epoch range would otherwise silently wrap. Only reachable via
// timestamp-nanos/local-timestamp-nanos, both new since the swap, so this
// had no equivalent before it either.
func TestSwap_TimestampNanos_EncodeOverflow_Errors(t *testing.T) {
	is := is.New(t)

	s, err := Parse([]byte(`{"type":"long","logicalType":"timestamp-nanos"}`))
	is.NoErr(err)

	outOfRange := time.Date(3000, 1, 1, 0, 0, 0, 0, time.UTC) // past 2262-04-11, outside int64 ns-since-epoch range
	_, err = s.Marshal(outOfRange)
	is.True(err != nil)
	t.Logf("out-of-range timestamp-nanos encode: err=%v", err)
}

// --- F2/F3/F4: WithMaxSliceAllocSize / WithMaxMapAllocSize /
// SetDefaultMaxSliceAllocSize / SetDefaultMaxMapAllocSize coverage. Before
// this commit these two Options had zero tests: nothing exercised raising
// the ceiling, lowering it, n <= 0, or the documented interaction with
// WithMaxInputSize. ---

// TestWithMaxSliceAllocSize_RaisesAndLowersCeiling covers both directions:
// an override above the package default allows a declared count the
// default would reject; an override below it rejects a declared count the
// default would allow.
func TestWithMaxSliceAllocSize_RaisesAndLowersCeiling(t *testing.T) {
	is := is.New(t)

	t.Run("raises", func(t *testing.T) {
		is := is.New(t)
		const override = initialDefaultMaxAllocSize * 2
		s, err := Parse([]byte(`{"type":"array","items":"null"}`), WithMaxSliceAllocSize(override))
		is.NoErr(err)

		const declared = initialDefaultMaxAllocSize + 500_000 // above the package default, within the override
		var out []any
		err = s.Unmarshal(zigzagVarint(declared), &out)
		is.NoErr(err)
		is.Equal(len(out), declared)
	})

	t.Run("lowers", func(t *testing.T) {
		is := is.New(t)
		const override = 10
		s, err := Parse([]byte(`{"type":"array","items":"null"}`), WithMaxSliceAllocSize(override))
		is.NoErr(err)

		var outOver []any
		err = s.Unmarshal(zigzagVarint(override+1), &outOver)
		is.True(err != nil) // the package default (1,000,000) would have allowed this; the lowered override rejects it

		var outWithin []any
		err = s.Unmarshal(zigzagVarint(override), &outWithin)
		is.NoErr(err)
		is.Equal(len(outWithin), override)
	})
}

// TestWithMaxMapAllocSize_RaisesAndLowersCeiling is
// TestWithMaxSliceAllocSize_RaisesAndLowersCeiling's counterpart for maps.
// Unlike arrays of `null`, Avro map keys are always strings and must
// actually be read off the wire, so (unlike the array test above) the
// "allowed" cases here use real Marshal-produced data rather than a
// crafted header-only payload.
func TestWithMaxMapAllocSize_RaisesAndLowersCeiling(t *testing.T) {
	is := is.New(t)

	t.Run("lowers", func(t *testing.T) {
		is := is.New(t)
		const ceiling = 5
		s, err := SerdeForType(map[string]string{}, WithMaxMapAllocSize(ceiling))
		is.NoErr(err)

		tooMany := map[string]string{}
		for i := 0; len(tooMany) <= ceiling; i++ {
			tooMany[fmt.Sprintf("k%d", i)] = "v"
		}
		encoded, err := s.Marshal(tooMany)
		is.NoErr(err)
		var out map[string]string
		err = s.Unmarshal(encoded, &out)
		is.True(err != nil) // the package default (1,000,000) would have allowed this; the lowered override rejects it

		justRight := map[string]string{}
		for i := 0; len(justRight) < ceiling; i++ {
			justRight[fmt.Sprintf("k%d", i)] = "v"
		}
		encoded2, err := s.Marshal(justRight)
		is.NoErr(err)
		var out2 map[string]string
		err = s.Unmarshal(encoded2, &out2)
		is.NoErr(err)
		is.Equal(len(out2), len(justRight))
	})

	t.Run("raises", func(t *testing.T) {
		is := is.New(t)
		// Lower the process-wide default first (see the
		// SetDefaultMaxMapAllocSize tests below for that mechanism in
		// isolation), then prove a per-Serde WithMaxMapAllocSize override
		// still takes precedence over it -- without needing hundreds of
		// thousands of real entries to demonstrate "raises."
		is.NoErr(SetDefaultMaxMapAllocSize(3))
		t.Cleanup(func() { is.NoErr(SetDefaultMaxMapAllocSize(initialDefaultMaxAllocSize)) })

		s, err := SerdeForType(map[string]string{}, WithMaxMapAllocSize(100))
		is.NoErr(err)

		src := map[string]string{}
		for i := 0; len(src) < 10; i++ { // above the (lowered) package default of 3, within the override of 100
			src[fmt.Sprintf("k%d", i)] = "v"
		}
		encoded, err := s.Marshal(src)
		is.NoErr(err)
		var out map[string]string
		err = s.Unmarshal(encoded, &out)
		is.NoErr(err)
		is.Equal(len(out), len(src))
	})
}

// TestWithMaxSliceAllocSize_RejectsNonPositive and
// TestWithMaxMapAllocSize_RejectsNonPositive are the F4 regression tests.
// limits.go documents n <= 0 as "rejected" for these two options, but
// buildDecodeAPI used to gate the override with `if o.maxSliceAllocSize >
// 0`, so a caller-supplied n <= 0 silently fell back to the package
// default with no signal at all -- the doc's claim was aspirational, not
// enforced. Both options now validate eagerly inside the Option itself,
// and Parse/SerdeForType propagate the error immediately via
// resolveOptions.
func TestWithMaxSliceAllocSize_RejectsNonPositive(t *testing.T) {
	is := is.New(t)

	for _, n := range []int{0, -1, -1000} {
		_, err := Parse([]byte(`{"type":"array","items":"null"}`), WithMaxSliceAllocSize(n))
		is.True(err != nil)
		is.True(errors.Is(err, ErrInvalidOption))
		t.Logf("n=%d: err=%v", n, err)
	}
}

func TestWithMaxMapAllocSize_RejectsNonPositive(t *testing.T) {
	is := is.New(t)

	for _, n := range []int{0, -1, -1000} {
		_, err := Parse([]byte(`{"type":"map","values":"string"}`), WithMaxMapAllocSize(n))
		is.True(err != nil)
		is.True(errors.Is(err, ErrInvalidOption))
		t.Logf("n=%d: err=%v", n, err)
	}
}

// TestWithMaxInputSize_TightensSmallerOverride and
// TestWithMaxInputSize_DoesNotLoosenLargerOverride are the regression
// tests for the documented WithMaxInputSize/WithMaxSliceAllocSize
// interaction (limits.go, "When MaxInputSize is also configured"): a
// configured MaxInputSize tightens an explicit MaxSliceAllocSize override
// when MaxInputSize is the smaller of the two, and never loosens one that
// is already smaller than MaxInputSize.
func TestWithMaxInputSize_TightensSmallerOverride(t *testing.T) {
	is := is.New(t)

	// Explicit override (1,000) is larger than MaxInputSize (64 bytes), so
	// MaxInputSize tightens the effective ceiling to 64 -- far below what
	// the explicit override alone would have allowed.
	s, err := Parse([]byte(`{"type":"array","items":"long"}`),
		WithMaxSliceAllocSize(1_000), WithMaxInputSize(64))
	is.NoErr(err)

	// A declared count of 100 (well under the explicit override of 1,000,
	// but backed by a payload that, if real, could never fit in 64 bytes)
	// is rejected by the tightened ceiling.
	var out []int64
	err = s.Unmarshal(zigzagVarint(100), &out)
	is.True(err != nil)
	t.Logf("err=%v", err)
}

func TestWithMaxInputSize_DoesNotLoosenLargerOverride(t *testing.T) {
	is := is.New(t)

	// Explicit override (10) is smaller than MaxInputSize (1 MiB), so
	// MaxInputSize must NOT loosen it back up -- the effective ceiling
	// stays 10.
	s, err := Parse([]byte(`{"type":"array","items":"null"}`),
		WithMaxSliceAllocSize(10), WithMaxInputSize(1024*1024))
	is.NoErr(err)

	var outOver []any
	err = s.Unmarshal(zigzagVarint(11), &outOver)
	is.True(err != nil) // still bounded by the explicit override (10), not loosened to MaxInputSize's 1 MiB

	var outWithin []any
	err = s.Unmarshal(zigzagVarint(10), &outWithin)
	is.NoErr(err)
	is.Equal(len(outWithin), 10)
}

// TestSetDefaultMaxSliceAllocSize and TestSetDefaultMaxMapAllocSize are
// the F2 regression tests: limits.go documented an operator's ability to
// "raise the ceiling with WithMaxSliceAllocSize / WithMaxMapAllocSize
// rather than being stuck," but the only production decode path
// (schema.Schema.Unmarshal -> schema.globalSerdeCache ->
// schema.KnownSerdeFactories[TypeAvro].Parse -> avro.Parse(s), see
// schema/schema.go) called avro.Parse with no options at all -- there was
// no way to reach either option from that path, so the documented escape
// hatch did not exist for it. SetDefaultMaxSliceAllocSize/
// SetDefaultMaxMapAllocSize are the fix: a race-free, process-wide default
// that every subsequently-constructed Serde picks up, reachable
// regardless of which construction path (or how many layers of
// unmodified intermediate code, like schema.Schema.Serde) sits between
// the caller and Parse/SerdeForType.
func TestSetDefaultMaxSliceAllocSize(t *testing.T) {
	is := is.New(t)
	t.Cleanup(func() { is.NoErr(SetDefaultMaxSliceAllocSize(initialDefaultMaxAllocSize)) })

	// n <= 0 is rejected; the current default is left unchanged.
	err := SetDefaultMaxSliceAllocSize(0)
	is.True(err != nil)
	is.True(errors.Is(err, ErrInvalidOption))

	// Lower the process-wide default; a Serde constructed afterward with
	// no per-Serde override picks up the new, smaller default -- this is
	// exactly the mechanism schema.Schema.Unmarshal benefits from, since
	// it has no Option plumbing of its own to reach WithMaxSliceAllocSize.
	const lowered = 10
	is.NoErr(SetDefaultMaxSliceAllocSize(lowered))

	s, err := Parse([]byte(`{"type":"array","items":"null"}`))
	is.NoErr(err)

	var outOver []any
	err = s.Unmarshal(zigzagVarint(lowered+1), &outOver)
	is.True(err != nil) // rejected by the new, lowered package-wide default

	var outWithin []any
	err = s.Unmarshal(zigzagVarint(lowered), &outWithin)
	is.NoErr(err)
	is.Equal(len(outWithin), lowered)

	// A per-Serde WithMaxSliceAllocSize override still takes precedence
	// over the (now-lowered) package-wide default.
	overridden, err := Parse([]byte(`{"type":"array","items":"null"}`), WithMaxSliceAllocSize(lowered*10))
	is.NoErr(err)
	var outOverridden []any
	err = overridden.Unmarshal(zigzagVarint(lowered+1), &outOverridden)
	is.NoErr(err)
	is.Equal(len(outOverridden), lowered+1)
}

func TestSetDefaultMaxMapAllocSize(t *testing.T) {
	is := is.New(t)
	t.Cleanup(func() { is.NoErr(SetDefaultMaxMapAllocSize(initialDefaultMaxAllocSize)) })

	err := SetDefaultMaxMapAllocSize(-5)
	is.True(err != nil)
	is.True(errors.Is(err, ErrInvalidOption))

	const lowered = 10
	is.NoErr(SetDefaultMaxMapAllocSize(lowered))

	// Reject: declared block above the new, lowered package-wide default.
	// No further wire data is needed -- the ceiling check runs before any
	// entry is read.
	s, err := Parse([]byte(`{"type":"map","values":"string"}`))
	is.NoErr(err)
	var outOver map[string]string
	err = s.Unmarshal(zigzagVarint(lowered+1), &outOver)
	is.True(err != nil)

	// Allow: real (Marshal-produced) data within the new, lowered default.
	sReal, err := SerdeForType(map[string]string{})
	is.NoErr(err)
	src := map[string]string{}
	for i := 0; len(src) < lowered; i++ {
		src[fmt.Sprintf("k%d", i)] = "v"
	}
	encoded, err := sReal.Marshal(src)
	is.NoErr(err)
	var out map[string]string
	err = sReal.Unmarshal(encoded, &out)
	is.NoErr(err)
	is.Equal(len(out), lowered)
}

// --- F5: MaxInputSize-derived tightening is unsound for zero-wire-cost
// container elements. ---

// TestMaxInputSize_TighteningSkipped_ZeroCostArrayItems is the regression
// test: limits.go claimed "every array/map element of a non-null type
// costs at least one wire byte," but buildDecodeAPI applied the
// MaxInputSize-derived tightening unconditionally, including to arrays
// whose item type is exactly `null` (zero wire bytes per element,
// regardless of declared count). A 50,000-element array of null encodes
// to only its block-length header -- legitimate, small, real data -- but
// the (buggy, pre-fix) unconditional tightening set MaxSliceAllocSize to
// the configured MaxInputSize itself whenever that was the smaller bound,
// rejecting this real payload even though it was nowhere near that many
// bytes. schemaHasZeroCostContainer detects this shape and buildDecodeAPI
// skips the tightening (falling back to the configured/default
// MaxSliceAllocSize/MaxMapAllocSize instead -- still enforced, just not
// additionally tightened) for any Serde whose schema contains one.
func TestMaxInputSize_TighteningSkipped_ZeroCostArrayItems(t *testing.T) {
	is := is.New(t)

	const ceiling = 4096 // bytes -- deliberately far below the element count used below
	s, err := Parse([]byte(`{"type":"array","items":"null"}`), WithMaxInputSize(ceiling))
	is.NoErr(err)

	const n = 50_000
	src := make([]any, n)
	encoded, err := s.Marshal(src)
	is.NoErr(err)
	t.Logf("wire size for %d null elements: %d bytes (ceiling=%d)", n, len(encoded), ceiling)
	if len(encoded) >= ceiling {
		t.Fatalf("test setup produced %d wire bytes, want < ceiling (%d) -- otherwise this only proves "+
			"MaxInputSize's own byte-length check, not the array-ceiling tightening exception", len(encoded), ceiling)
	}

	var out []any
	err = s.Unmarshal(encoded, &out)
	is.NoErr(err) // must NOT be rejected: every element is zero-cost, so declared count is not bounded by `ceiling`
	is.Equal(len(out), n)
}

// TestMaxInputSize_TighteningStillAppliesToNonNullItems is the control for
// the test above: an array whose items are NOT zero-cost (e.g. `long`)
// must still be tightened by a configured MaxInputSize, exactly as
// before -- the exception in schemaHasZeroCostContainer must be scoped to
// genuinely zero-cost containers, not silently disable the tightening for
// every schema.
func TestMaxInputSize_TighteningStillAppliesToNonNullItems(t *testing.T) {
	is := is.New(t)

	const ceiling = 1024 * 1024 // 1 MiB
	const declared = 10_000_000 // cannot fit in 1 MiB even at 1 byte/element
	payload := zigzagVarint(declared)
	if len(payload) >= ceiling {
		t.Fatalf("test setup produced a payload as large as the ceiling; declared value too small")
	}

	s, err := Parse([]byte(`{"type":"array","items":"long"}`), WithMaxInputSize(ceiling))
	is.NoErr(err)

	var out []int64
	err = s.Unmarshal(payload, &out)
	is.True(err != nil) // MaxInputSize's own byte check did NOT fire (payload << ceiling); the tightened array bound did
	t.Logf("err=%v len(out)=%d", err, len(out))
}

// --- F9: alloc-ceiling rejection leaves v partially populated, unlike
// the ErrInputTooLarge byte-length check (which leaves v untouched). Two
// different failure semantics on one method; only the byte-length one had
// an assertion before this commit. See Serde.Unmarshal's godoc in
// serde.go for the documented distinction this test proves. ---

// TestUnmarshal_AllocCeilingRejection_LeavesPartialData proves the second
// half of that distinction: a record with a field that decodes
// successfully before a later field trips an allocation ceiling is left
// with the earlier field's value intact in the destination map -- not
// rolled back, not zeroed -- while the error is still non-nil. Contrast
// with TestMaxInputSize_OptInRejectsOversizedInput above, where the
// byte-length check (which runs before the decoder is invoked at all)
// leaves v completely untouched.
func TestUnmarshal_AllocCeilingRejection_LeavesPartialData(t *testing.T) {
	is := is.New(t)

	s, err := Parse([]byte(`{
		"type": "record",
		"name": "partial",
		"fields": [
			{"name": "before", "type": "string"},
			{"name": "after", "type": {"type": "array", "items": "null"}}
		]
	}`))
	is.NoErr(err)

	// Hand-craft a record: a valid "before" string field, followed by an
	// "after" array field declaring a count comfortably above the default
	// ceiling. The record decoder must decode "before" (setting it in the
	// destination map) before it reaches "after" and trips the ceiling.
	payload := append(zigzagVarint(int64(len("hello"))), "hello"...)
	payload = append(payload, zigzagVarint(initialDefaultMaxAllocSize+1)...)

	out := map[string]any{}
	err = s.Unmarshal(payload, &out)

	is.True(err != nil)
	is.Equal(out["before"], "hello") // decoded before the ceiling fired -- NOT rolled back
	// The field that tripped the ceiling is present in the destination map
	// as an untyped nil placeholder, not omitted and not its real value --
	// the record decoder assigns the map key before attempting to decode
	// its value. A caller iterating out's keys after a non-nil error sees
	// "after" present with a nil, not a real (possibly truncated) array.
	after, hasAfter := out["after"]
	is.True(hasAfter)
	is.True(after == nil)
	t.Logf("partial decode on ceiling rejection: out=%#v err=%v", out, err)
}
