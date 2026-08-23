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
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"github.com/matryer/is"
)

// zigzagVarint encodes an int64 the way Avro encodes a `long`: zigzag then
// unsigned LEB128 varint. Used below to hand-craft decoder inputs that real
// avro.Marshal calls would never produce, because the whole point is to
// exercise inputs a legitimate encoder would not emit but an attacker could.
func zigzagVarint(n int64) []byte {
	zz := uint64((n << 1) ^ (n >> 63))
	buf := make([]byte, binary.MaxVarintLen64)
	i := binary.PutUvarint(buf, zz)
	return buf[:i]
}

// TestMaxInputSize_RejectsOversizedInput proves ErrInputTooLarge fires, fails
// closed (v is left untouched, nothing is partially decoded), and does so
// before the expensive decode path runs -- not after truncating or coercing
// the input. This is the direct regression test for the mitigation.
func TestMaxInputSize_RejectsOversizedInput(t *testing.T) {
	is := is.New(t)

	serde, err := SerdeForType(map[string]any{"foo": "bar"})
	is.NoErr(err)

	oversized := make([]byte, MaxInputSize+1)

	var out map[string]any
	start := time.Now()
	err = serde.Unmarshal(oversized, &out)
	elapsed := time.Since(start)

	is.True(err != nil)
	is.True(errors.Is(err, ErrInputTooLarge))
	is.Equal(len(out), 0) // fail closed: nothing decoded, v untouched

	// The check must happen before handing bytes to hamba/avro. If it didn't,
	// this call would be doing real work over a >10MiB buffer instead of a
	// single length comparison.
	is.True(elapsed < 50*time.Millisecond)
}

// TestMaxInputSize_AllowsInputJustUnderLimit proves the mitigation doesn't
// change behavior for legitimate records: a real (Marshal-produced) payload
// sized close to MaxInputSize decodes exactly as it would have before this
// change. It's built as many small map entries, not one big field, because
// maxByteSliceSize (1 MiB, hamba/avro's own pre-existing default for a
// single bytes/string field, unchanged by this mitigation) would otherwise
// reject a single huge field for an unrelated reason.
func TestMaxInputSize_AllowsInputJustUnderLimit(t *testing.T) {
	is := is.New(t)

	serde, err := SerdeForType(map[string]string{})
	is.NoErr(err)

	src := map[string]string{}
	for i := 0; len(src) < 400_000; i++ {
		src[fmt.Sprintf("k%07d", i)] = fmt.Sprintf("v%07d", i)
	}
	encoded, err := serde.Marshal(src)
	is.NoErr(err)
	if len(encoded) > MaxInputSize {
		t.Fatalf("test setup produced %d bytes, want <= %d; reduce entry count", len(encoded), MaxInputSize)
	}

	var out map[string]string
	err = serde.Unmarshal(encoded, &out)
	is.NoErr(err)
	is.Equal(len(out), len(src))
}

// TestDecodeAPI_ArrayBoundFires proves the second layer of defense --
// hamba/avro's own Config.MaxSliceAllocSize, which we set explicitly in
// limits.go instead of leaving at its default of 1<<48 (256 TiB on
// amd64/arm64, i.e. no real bound) -- actually rejects a crafted array
// whose declared cumulative size exceeds it. The crafted payload is only a
// handful of bytes, so it passes the MaxInputSize check and reaches
// hamba/avro; this test shows the library-level guard, tightened, catches
// it from there.
//
// This is a real hamba/avro behavior, not a hypothetical: an array schema's
// wire encoding starts with a zigzag-varint block length, and hamba/avro
// accumulates declared block sizes into a plain `int` before comparing
// against MaxSliceAllocSize (codec_array.go). Declaring one block of
// ~2^40 elements (self-evidently absurd for any real record) exceeds our
// 128 MiB maxSliceAllocSize long before any element is actually decoded.
func TestDecodeAPI_ArrayBoundFires(t *testing.T) {
	is := is.New(t)

	serde, err := SerdeForType([]int64{})
	is.NoErr(err)

	// One block, ~1.1 trillion declared elements, then nothing else.
	payload := zigzagVarint(1 << 40)

	var out []int64
	err = serde.Unmarshal(payload, &out)
	is.True(err != nil)
	if !strings.Contains(err.Error(), "MaxSliceAllocSize") {
		t.Fatalf("expected error mentioning MaxSliceAllocSize, got: %v", err)
	}
}

// TestUnboundedMapAllocation_Documented is not a pass/fail regression test
// in the usual sense: it is the empirical proof, run as part of the normal
// test suite, of the residual risk this mitigation accepts rather than
// closes.
//
// hamba/avro's map decoder (codec_map.go) has no Config field bounding
// cumulative map size at all -- unlike arrays, there is nothing to tighten.
// The only lever available at this layer is MaxInputSize, which bounds wire
// size, not decoded heap size. This test builds a real (not hand-crafted)
// Avro map -- something SerdeForType/Marshal would happily produce for a
// legitimate record with a large map field -- with enough entries to sit
// just under MaxInputSize, and measures the live heap growth from decoding
// it.
//
// On github.com/hamba/avro/v2 v2.28.0 (pinned in go.mod), decoding a
// 3,000,000-entry map[string]string with ~18 bytes/entry (51.6 MiB wire)
// measured a ~4.0x wire-to-heap amplification (+215.9 MiB) in ~0.6s. This
// test uses a smaller entry count to stay fast in CI while still
// demonstrating the amplification is real and > 1x; see
// docs/design-documents/20260823-avro-codec-archived-decoder-advisories.md
// in ConduitIO/conduit for the full-scale measurement this number is drawn
// from. capping MaxInputSize caps the wire size an attacker can supply, and
// therefore caps this amplification to a bounded (if not small) multiple of
// a number we control -- it does not make map decoding safe against a
// crafted payload up to that size.
func TestUnboundedMapAllocation_Documented(t *testing.T) {
	if testing.Short() {
		t.Skip("allocates real heap memory to measure amplification; skipped with -short")
	}
	is := is.New(t)

	serde, err := SerdeForType(map[string]string{})
	is.NoErr(err)

	// 1M entries stays comfortably under MaxInputSize (~18 MiB would not,
	// so key/value width is kept small) and, unlike a 300K run, gives a
	// stable signal above GC/allocator noise. The 3M-entry, 51.6 MiB run
	// used for the design-doc measurement (+215.9 MiB, ~4.0x) doesn't fit
	// under MaxInputSize by design -- that's the point of the cap -- so it
	// is not reproduced here; see the design doc for that measurement.
	const n = 550_000
	wire := func() []byte {
		// Scoped so the source map is unreachable (not just nil-ed, actually
		// out of scope) by the time we measure the decode side below.
		src := make(map[string]string, n)
		for i := 0; i < n; i++ {
			src[fmt.Sprintf("k%07d", i)] = fmt.Sprintf("v%07d", i)
		}
		b, err := serde.Marshal(src)
		is.NoErr(err)
		return b
	}()
	if len(wire) > MaxInputSize {
		t.Fatalf("test setup produced %d bytes, want <= MaxInputSize (%d); reduce n", len(wire), MaxInputSize)
	}

	// Disable GC during the measurement window so a concurrent collection
	// can't attribute one side's garbage to the other; force two full GCs
	// at each edge instead so HeapAlloc reflects live objects only.
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

	// This is a floor, not a ceiling: the point being documented is that
	// decoding a map costs meaningfully more heap than its wire size, with
	// no library-level knob to cap it. If this ever stops being true (e.g.
	// after a codec replacement that bounds map allocation), that's good
	// news and this assertion should be revisited, not tightened defensively.
	is.True(delta > int64(len(wire)))
}
