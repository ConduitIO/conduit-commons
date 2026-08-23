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

import "github.com/hamba/avro/v2"

// Decode-side input bounds for github.com/hamba/avro/v2.
//
// hamba/avro was archived by its maintainer on 2026-01-18 (final release
// v2.31.0) carrying three unfixed decoder advisories, all reachable by
// decoding untrusted Avro bytes -- exactly what Serde.Unmarshal does with
// records arriving from an upstream Conduit does not control:
//
//   - GO-2026-5046 -- CPU exhaustion in the array/map decoders
//   - GO-2026-5047 -- integer overflow in cumulative-size arithmetic
//   - GO-2026-5048 -- unbounded map allocation (no cap exists for this in
//     any released version of hamba/avro; the fix landed only in a fork,
//     github.com/iskorotkov/avro/v2 v2.33.0, as an opt-in Config field that
//     does not exist on the module we depend on)
//
// The full analysis, including which of these bounds are and are not
// expressible against hamba/avro's public API, and the long-term
// fork/replace/accept decision, lives in
// docs/design-documents/20260823-avro-codec-archived-decoder-advisories.md
// in ConduitIO/conduit. This file is the near-term mitigation decided
// there: bound what the library lets us bound, and put a hard ceiling on
// input size to cap the blast radius of what it does not.
//
// Verified against github.com/hamba/avro/v2 v2.28.0 (the version pinned in
// go.mod) on 2026-08-22:
//   - Config.MaxByteSliceSize exists and is enforced per bytes/string field
//     (reader.go). hamba's own default (1 MiB) already applied before this
//     change via the package-level avro.Unmarshal/DefaultConfig path; we
//     pin the same value explicitly so it is visible here instead of
//     implicit in an archived dependency.
//   - Config.MaxSliceAllocSize exists and is enforced cumulatively across
//     array blocks (codec_array.go: `if size > r.cfg.getMaxSliceAllocSize()`
//     before growing, confirmed to fire against a crafted ~2^62-element
//     block declaration). Its *default* resolves to 1<<48 (256 TiB on
//     amd64/arm64 -- config_x64.go) when unset, which is not a real bound
//     for a component processing records. We tighten it explicitly below.
//   - No Config field bounds cumulative map allocation. codec_map.go's
//     mapDecoder.Decode loop grows the destination map on every declared
//     entry with no size check at all. We reproduced this directly: a
//     crafted map of 3,000,000 small string entries (51.6 MiB Avro wire
//     encoding) decoded into +215.9 MiB of live heap (measured via
//     runtime.MemStats after a forced GC), a ~4.0x wire-to-heap
//     amplification, in 577ms, with no available Config knob to cap it.
//     This is the concrete case the input-size ceiling below exists to
//     bound: it cannot make map decoding safe, but it makes the worst case
//     a known, bounded multiple of a number we control instead of
//     unbounded.
//   - No Config field, or any other public API, bounds recursion or value
//     nesting depth. We did not find a lever for this in hamba/avro and are
//     not claiming one exists; the input-size ceiling is, again, the only
//     backstop available (a deeply nested value still has to be encoded in
//     bytes somewhere).
//   - We could not construct a working crash/panic proof-of-concept for the
//     GO-2026-5047 integer-overflow class specifically (e.g. negating
//     math.MinInt64 in Reader.ReadBlockHeader, which does overflow back to
//     itself). Empirically the crafted payloads we tried decoded without
//     error and without incident. That does not mean the class is not
//     real -- the unchecked arithmetic described in the advisory is
//     present in the source -- only that we did not find a reachable
//     bypass of the existing MaxSliceAllocSize guard in the time
//     available. Treated as an open question, not a cleared one.
const (
	// MaxInputSize is the largest number of Avro-encoded bytes Serde.Unmarshal
	// will attempt to decode. Larger input is rejected before it reaches
	// hamba/avro at all, with ErrInputTooLarge.
	//
	// 10 MiB matches Conduit's own plugin gRPC transport ceiling
	// (grpc.MaxRecvMsgSize(10*1024*1024) in pkg/conduit/runtime.go as of
	// 2026-08-22): no legitimate record can already be larger than that and
	// still have reached this code, so this rejects nothing that works
	// today. Combined with the measured ~4x map amplification above, it
	// keeps the worst case for a single malicious record in the tens of
	// MiB instead of unbounded.
	//
	// This is a package variable, not a constant, so a consumer with a
	// legitimate need for a different ceiling (e.g. a deliberately raised
	// gRPC message limit) can override it. Most callers should not need to.
	defaultMaxInputSize = 10 * 1024 * 1024

	// maxByteSliceSize bounds a single `bytes`/`string` field. This is
	// hamba/avro's own default (defaultMaxByteSliceSize in its config.go);
	// pinned explicitly here rather than left implicit. It does not change
	// decode behavior versus the unpatched package-level avro.Unmarshal path.
	maxByteSliceSize = 1 * 1024 * 1024

	// maxSliceAllocSize bounds cumulative `array` allocation across all
	// blocks of a single field. hamba/avro's default is 1<<48 (256 TiB on
	// amd64/arm64), which is not a real bound. With input already capped at
	// MaxInputSize and Avro's minimum encoding of one byte per element (a
	// zigzag-varint-encoded zero), the worst case from a 10 MiB input is on
	// the order of 10M elements; 128 MiB gives headroom for 8-byte (long/
	// double) elements at that count while remaining far below what any
	// legitimate Conduit record needs in a single array field.
	maxSliceAllocSize = 128 * 1024 * 1024
)

// MaxInputSize is the enforced ceiling on Serde.Unmarshal input size. See
// defaultMaxInputSize for the justification. Exported as a var so a caller
// with a real reason to raise or lower it can, without a new API surface.
var MaxInputSize = defaultMaxInputSize

// decodeAPI is a frozen hamba/avro API configured with the explicit bounds
// documented above, used for all Serde.Unmarshal calls instead of the
// package-level avro.Unmarshal (which resolves to avro.DefaultConfig, i.e.
// hamba/avro's untightened defaults).
var decodeAPI = avro.Config{
	MaxByteSliceSize:  maxByteSliceSize,
	MaxSliceAllocSize: maxSliceAllocSize,
}.Freeze()
