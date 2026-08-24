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

// TestArrayBound_UnboundedByDefault documents the direct consequence of
// "unlimited by default": with no WithMaxInputSize configured, this
// package applies no array-allocation ceiling either (see limits.go's
// package doc for why one derived from a specific call's observed length
// would be unsound -- the padding-inflation bypass -- and why deriving one
// from nothing is worse than not pretending to have one). The finding this
// documents (C1: a handful of bytes can declare an absurd element count
// and allocate gigabytes) is therefore NOT mitigated by this package
// unless the caller opts into WithMaxInputSize; see
// TestArrayBound_OptInRejectsDeclaredCountAboveCeiling for the opt-in case.
func TestArrayBound_UnboundedByDefault(t *testing.T) {
	if testing.Short() {
		t.Skip("allocates multiple GiB to demonstrate the unmitigated amplification; skipped with -short")
	}

	const declared = 134_217_728 // 128 MiB as a raw element count
	payload := zigzagVarint(declared)
	t.Logf("wire bytes for block header: %d", len(payload))

	s, err := Parse([]byte(`{"type":"array","items":"null"}`))
	if err != nil {
		t.Fatal(err)
	}

	var out []any
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	err = s.Unmarshal(payload, &out)
	runtime.ReadMemStats(&after)

	t.Logf("GO-2026-5047-adjacent (element-count-vs-byte-budget), NOT mitigated without WithMaxInputSize: "+
		"err=%v len(out)=%d heapDelta=%.2f MiB", err, len(out), float64(after.HeapAlloc-before.HeapAlloc)/(1<<20))
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

// TestArrayBound_NegativeLengthOverflow_Documented is the direct
// reproduction of finding H1 (see limits.go point 2, GO-2026-5047):
// Reader.ReadBlockHeader negates a value that decoded to math.MinInt64,
// which overflows back to math.MinInt64 (still negative) instead of
// becoming positive, bypassing every positive-ceiling guard by sign alone
// -- at any input-size policy, opted in or not.
//
// This is deliberately NOT asserted as "must return an error" -- it
// currently does not, against hamba/avro, and nothing in this package can
// fix it (the bug is upstream of any Config field, inside ReadBlockHeader
// itself). This test exists so the exact reproduction is committed and
// runnable, and so it will visibly start failing (a good outcome) the day
// this package moves off hamba/avro, at which point it should be rewritten
// to assert the error instead of documenting its absence. See the
// migration PR referenced in the design doc.
//
// Do not add assertions here that read `out` after Unmarshal beyond len/cap
// captured into plain int locals immediately: the corrupted slice header
// this produces is unsafe to touch further (no append, no indexing, no
// range) -- see limits.go point 2.
func TestArrayBound_NegativeLengthOverflow_Documented(t *testing.T) {
	s, err := Parse([]byte(`{"type":"array","items":"long"}`))
	if err != nil {
		t.Fatal(err)
	}

	first := rawUvarint(math.MaxUint64) // zigzag-decodes to math.MinInt64
	second := rawUvarint(0)             // block-size-in-bytes field the negative-count encoding requires
	payload := append(append([]byte{}, first...), second...)
	if len(payload) != 11 {
		t.Fatalf("payload construction changed: got %d bytes, want 11", len(payload))
	}

	var out []int64
	err = s.Unmarshal(payload, &out)
	n, c := len(out), cap(out) // capture immediately; do not touch out again

	t.Logf("GO-2026-5047, NOT mitigated by this package against hamba/avro: "+
		"11-byte payload, err=%v, len(out)=%d, cap(out)=%d", err, n, c)
}

// TestMapDecodeCPUExhaustion_SafeStringKeyedPath documents that
// Serde.Unmarshal's own usage (map[string]any / map[string]string, always
// string-keyed) takes hamba/avro's safe mapDecoder path, which checks the
// reader's error state after every element and bails on the first failed
// read. This is the control for
// TestMapDecodeCPUExhaustion_UnsafeTextUnmarshalerPath below: same crafted
// payload shape, different destination type, dramatically different
// outcome.
func TestMapDecodeCPUExhaustion_SafeStringKeyedPath(t *testing.T) {
	s, err := Parse([]byte(`{"type":"map","values":"string"}`))
	if err != nil {
		t.Fatal(err)
	}
	const declared = 20_000_000
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
// (it's a struct), which are both required for hamba/avro's
// createDecoderOfMap to route to mapDecoderUnmarshaler instead of the safe
// mapDecoder: the switch checks keyType.Kind() == reflect.String first (a
// string-kind named type, even with a value-receiver UnmarshalText, would
// still take the safe path), then keyType.Implements(textUnmarshalerType)
// with no PtrTo unwrap (so the map key type itself must be the pointer
// type, matching hamba's nil-check-and-alloc dance in
// decoderOfMapUnmarshaler).
type textKey struct{ s string }

func (k *textKey) UnmarshalText(b []byte) error {
	k.s = string(b)
	return nil
}

// TestMapDecodeCPUExhaustion_UnsafeTextUnmarshalerPath is the direct
// reproduction of finding C3 (see limits.go point 1, GO-2026-5046):
// hamba/avro's mapDecoderUnmarshaler.Decode has no per-element error check
// at all (unlike the safe mapDecoder above), and no Config field bounds a
// map's declared block length either, at any input-size policy. A 4-byte
// payload declaring a multi-million-entry block, backed by no further
// data, decodes millions of fabricated entries in real (multi-second) CPU
// time with err == nil.
//
// Reachable only through a caller-controlled destination type whose map key
// implements encoding.TextUnmarshaler via a pointer receiver -- Conduit's
// own Serde usage never does this (always map[string]any /
// map[string]string), but Serde.Unmarshal is public API and does not
// restrict destination types. Not mitigated by anything in this package,
// with or without WithMaxInputSize: there is no Config field to bound map
// cardinality against hamba/avro at all (see limits.go). Skipped by
// default (allocates real CPU time); run explicitly with
// `-run TestMapDecodeCPUExhaustion_UnsafeTextUnmarshalerPath` or without
// `-short`.
func TestMapDecodeCPUExhaustion_UnsafeTextUnmarshalerPath(t *testing.T) {
	if testing.Short() {
		t.Skip("allocates several seconds of real CPU time to demonstrate the exhaustion; skipped with -short")
	}

	s, err := Parse([]byte(`{"type":"map","values":"string"}`))
	if err != nil {
		t.Fatal(err)
	}
	// 5,000,000 rather than the 20,000,000 used in the original
	// investigation: still several seconds of real CPU (order-of-magnitude
	// proof the exhaustion is real), while keeping -race runs (which add
	// substantial per-op overhead to this instrumentation-heavy loop)
	// reasonable in CI.
	const declared = 5_000_000
	payload := zigzagVarint(declared)

	var out map[*textKey]string
	start := time.Now()
	err = s.Unmarshal(payload, &out)
	elapsed := time.Since(start)

	t.Logf("GO-2026-5046, NOT mitigated by this package against hamba/avro: "+
		"4-byte payload, err=%v, len(out)=%d, elapsed=%s", err, len(out), elapsed)
}

// TestByteSliceBound_Fires proves the maxByteSliceSize ceiling actually
// rejects an oversized bytes/string field. maxByteSliceSize is a no-op by
// construction (see limits.go's package doc): hamba/avro's own default
// resolves an unset field to the identical 1 MiB value, so this test's job
// is to prove the *ceiling itself* -- whichever value resolves it --
// actually functions.
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
// concern from TestMapDecodeCPUExhaustion_UnsafeTextUnmarshalerPath above
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
