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
	"math/bits"
	"sync"

	"github.com/hamba/avro/v2"
)

// Decode-side input bounds for github.com/hamba/avro/v2.
//
// hamba/avro was archived by its maintainer on 2026-01-18 (final release
// v2.31.0) carrying three unfixed decoder advisories, all reachable by
// decoding untrusted Avro bytes -- exactly what Serde.Unmarshal does with
// records arriving from an upstream Conduit does not control:
//
//   - GO-2026-5046 -- CPU exhaustion in the array/map decoders
//   - GO-2026-5047 -- integer overflow in cumulative-size arithmetic
//   - GO-2026-5048 -- unbounded map allocation
//
// The long-term call -- replacing hamba/avro with an actively maintained
// fork, github.com/iskorotkov/avro/v2 -- is made in
// docs/design-documents/20260823-avro-codec-archived-decoder-advisories.md
// in ConduitIO/conduit. This file is the near-term, codec-independent
// mitigation: bound what hamba/avro's Config lets us bound, and be explicit
// about what it does not.
//
// This is a second pass at that mitigation. The first version (reviewed and
// rejected) claimed protections it did not have. What changed and why, with
// every claim re-verified by a runnable reproduction in limits_test.go
// against github.com/hamba/avro/v2 v2.28.0 (the version pinned in go.mod)
// on 2026-08-22:
//
//  1. maxSliceAllocSize was a flat 128 MiB constant compared against a raw
//     *element count*, not a byte budget (codec_array.go:
//     `if size > r.cfg.getMaxSliceAllocSize()` where size accumulates
//     declared block lengths -- element counts -- before a single element is
//     decoded). A 5-byte payload declaring an array of 134,217,728 "null"
//     items (zero wire bytes each) passed that check exactly at the
//     boundary and allocated 2-3 GiB, err == nil
//     (TestArrayBound_ElementCountVsInputSize). This is not specific to
//     zero-cost items: the same payload shape with realistic non-null items
//     (e.g. "long") allocates the full declared backing array
//     (sliceType.UnsafeGrow) *before* decoding a single element, so the
//     amplification is real regardless of per-element wire cost -- only the
//     eventual "ran out of bytes" error is swallowed as io.EOF (see point 4
//     below), not the allocation. Fixed here by replacing the flat constant
//     with decodeAPIForInputSize, which derives the array-allocation
//     ceiling from the actual input length instead of a number picked once
//     and applied to every call regardless of size. See its doc comment for
//     what this does and does not close.
//  2. MaxInputSize (10 MiB) was justified as matching "Conduit's own plugin
//     gRPC transport ceiling" -- specifically
//     grpc.MaxRecvMsgSize(10*1024*1024) in pkg/conduit/runtime.go. That
//     citation is wrong: that call is inside Runtime.serveGRPCAPI, which
//     registers the pipeline/processor/connector/plugin CRUD API -- the
//     control plane. It carries no records. The actual data-path ceiling,
//     for standalone plugins, is hashicorp/go-plugin's
//     grpc.MaxCallRecvMsgSize(math.MaxInt32) (go-plugin@v1.8.0
//     grpc_client.go:40) -- effectively unbounded (~2047 MiB) -- and
//     built-in connectors never cross gRPC at all, so they have no ceiling
//     of any kind. There is no tight, defensible number to cite here. See
//     MaxInputSize's doc comment for the corrected (honest) justification:
//     it is a deliberately generous, explicitly arbitrary backstop, not a
//     derived transport limit.
//  3. GO-2026-5046 (CPU exhaustion) was reported as "could not reproduce as
//     described." It reproduces cleanly, just not through the code path
//     that was tried. hamba/avro's *string-keyed* map decoder
//     (mapDecoder.Decode, codec_map.go) does check the reader's error state
//     after every element and bails on the first failed read -- this is the
//     path Serde.Unmarshal's own map[string]any/map[string]string usage
//     always takes, and it is not vulnerable (confirmed:
//     TestMapDecodeCPUExhaustion_SafeStringKeyedPath, ~10us for a
//     20-million-entry declared block backed by 4 bytes of input). But
//     Serde.Unmarshal is public API with a caller-controlled destination
//     type, and hamba/avro has a *second* map decoder,
//     mapDecoderUnmarshaler (taken when the destination map's key type
//     implements encoding.TextUnmarshaler via a pointer receiver), which
//     has no such check at all. A 4-byte payload declaring a 20-million-
//     entry block, backed by no further data, decodes 20,000,000 fabricated
//     entries in ~5 seconds of CPU with err == nil
//     (TestMapDecodeCPUExhaustion_UnsafeTextUnmarshalerPath). MaxInputSize
//     gives zero protection here: the declared block length is not
//     compared against remaining input at all, by either map decoder, in
//     any released hamba/avro version. This is not mitigated by anything in
//     this file -- there is no Config field to tighten, unlike arrays. It
//     is the strongest concrete argument for the codec replacement: the
//     iskorotkov/avro fork adds Config.MaxMapAllocSize, which this class of
//     bug has no equivalent bound against on hamba/avro at all.
//  4. GO-2026-5047 (integer overflow) was reported as "partially open,
//     could not construct a working crash PoC." It reproduces directly.
//     Reader.ReadBlockHeader does `-length` on a value that decoded to
//     math.MinInt64 (an 11-byte crafted payload: a raw varint bit pattern
//     of all-1s, which zigzag-decodes to math.MinInt64, plus a second long
//     for the block-size-in-bytes field the negative-count encoding
//     requires). Negating math.MinInt64 overflows back to math.MinInt64 --
//     still negative -- which passes into the array decoder's element-count
//     accumulator (int(l) added to size) and then unsafe slice-growth code,
//     producing len(out) == math.MinInt64 (-9223372036854775808), cap(out)
//     == 0, err == nil (TestArrayBound_NegativeLengthOverflow). This is not
//     just an inert negative number: the crafted payload was observed to
//     leave the process in a state where subsequent, logically-unrelated
//     boolean comparisons in the same test misbehaved -- consistent with
//     memory corruption from the unsafe growth path, not merely a "wrong
//     answer." Treat any successful decode into this state as unsafe to
//     touch further (no append, no indexing, no range) rather than merely
//     "wrong." Nothing in this file catches this: it happens inside
//     ReadBlockHeader, upstream of both maxSliceAllocSize and
//     decodeAPIForInputSize. This is the one advisory in this list that
//     hamba/avro's public Config API cannot bound *at all*, at any value --
//     the fix has to be in the reader itself. Verified fixed at the source
//     in github.com/iskorotkov/avro/v2 v2.34.0: Reader.ReadBlockHeader
//     explicitly checks `length64 <= math.MinInt` before negating and
//     rejects it with an error, unconditionally, regardless of Config
//     (TestArrayBound_NegativeLengthOverflow's fork counterpart in the
//     migration-readiness spike referenced in the design doc). This is the
//     other strongest concrete argument for the codec replacement.
//
// What the codec replacement does NOT fix on its own (verified against
// github.com/iskorotkov/avro/v2 v2.34.0, not assumed from its being a
// fork): point 1 above -- the array decoder's allocate-before-validate
// pattern, and MaxSliceAllocSize being an element count rather than a byte
// budget -- is unchanged in the fork. The exact TestArrayBound_
// ElementCountVsInputSize payload, run against the fork with the *same*
// 128 MiB MaxSliceAllocSize value this PR used to hard-code, reproduces
// identically: err == nil, 100,000,000 fabricated "long" elements
// allocated from a 5-byte payload. The fork's array codec restructured the
// bound check (compares the *current block's* declared length against
// remaining budget before growing, closing a related but distinct
// cumulative-overflow risk across multiple blocks) but did not change what
// unit the budget is denominated in. decodeAPIForInputSize's approach
// (derive the ceiling from actual input size) is therefore not superseded
// by the codec replacement -- it is independently useful under either
// codec, which is why it lives here rather than being deferred to the
// migration PR.
//
// Also not addressed by either codec: hamba/avro's MaxSliceAllocSize (and
// the fork's, and its new MaxMapAllocSize) bounds allocation *per
// field-decode call*, not cumulatively across a whole decoded message. A
// record with N independent array (or, on the fork, map) fields can each
// independently claim up to the configured ceiling, multiplying the
// worst-case allocation by N. There is no whole-message allocation budget
// hook in either library's public API. The only thing bounding the total
// blast radius across all fields in one call is MaxInputSize, and only in
// the sense that a large N of large declared blocks still requires a
// nonzero number of wire bytes to declare each block header -- which does
// not bound allocation, only how many block declarations fit in the input.
//
// maxByteSliceSize is unchanged from the original mitigation and remains
// what it always was: a no-op by construction. hamba/avro's
// getMaxByteSliceSize() resolves 0 to its own built-in default
// (defaultMaxByteSliceSize, 1 MiB) exactly as our explicit
// 1*1024*1024 does -- setting the field cannot, by construction, ever
// produce different behavior than leaving it unset. It stays here as
// documentation of intent (explicit instead of implicit-in-an-archived-
// dependency), not as a behavior change, and TestByteSliceBound_Fires below
// proves the *ceiling itself* (whichever value resolves it) actually
// rejects an oversized field -- the previous version of this file asserted
// no such thing.
//
// Known compatibility caveat, not fixed here: decodeAPIForInputSize's
// avro.Config.Freeze() calls each allocate a fresh *TypeResolver
// (hamba/avro's Freeze() always does this; see config.go). This package's
// own union handling does not depend on it -- unionResolver in union.go
// carries its own independent avro.TypeResolver, populated per-Serde via
// BeforeMarshal/AfterUnmarshal, and every existing test in this package
// exercises that path, unaffected by this file. But hamba/avro also has its
// *own*, separate, resolver-driven union-decoding fallback inside
// codec_union.go, consulted through the frozen API's resolver field. Before
// this mitigation, Serde.Unmarshal called the package-level avro.Unmarshal,
// which resolves through avro.DefaultConfig -- a single process-wide
// instance any caller's avro.Register(name, obj) populates. Now that
// Serde.Unmarshal calls into decodeAPIForInputSize's own frozen APIs
// instead, any such external avro.Register call no longer reaches this
// package's decode path: hamba/avro's own union-resolution fallback (not
// this package's unionResolver) silently degrades to a generic map with no
// error instead of resolving to the registered Go type. No code in this
// repository calls avro.Register (grep confirms zero hits outside
// hamba/avro's own source), so this has no effect on any current caller.
// It is a real, silent behavior difference for any external module that
// both imports conduit-commons/schema/avro directly and separately calls
// hamba/avro's package-level Register -- documented here because the
// previous version of this file did not acknowledge it at all, not because
// a fix is in scope for this PR. A caller in that position needs a
// Register passthrough this package does not currently expose.
const (
	// defaultMaxInputSize is deliberately generous and, unlike the previous
	// version of this constant, is NOT derived from any known Conduit
	// transport limit. No defensible tight ceiling exists to cite: see
	// point 2 in the package doc above. hashicorp/go-plugin's data-path
	// gRPC client sets grpc.MaxCallRecvMsgSize(math.MaxInt32) (effectively
	// unbounded, ~2047 MiB), and built-in connectors never cross gRPC at
	// all. 256 MiB is chosen only because it is comfortably larger than any
	// legitimate Conduit record this project is aware of (including large
	// jsonb/bytea Postgres columns in the tens of MiB) while still keeping
	// a single malicious record's worst-case blast radius an order of
	// magnitude below what go-plugin's transport would otherwise allow
	// through unbounded. If a real, tighter, defensible ceiling is ever
	// identified (e.g. a documented Conduit-wide record-size limit), this
	// should be re-derived from that number, not from this comment.
	defaultMaxInputSize = 256 * 1024 * 1024

	// maxByteSliceSize bounds a single `bytes`/`string` field. See the
	// package doc above: this is a no-op by construction (hamba/avro
	// resolves an unset field to the same 1 MiB default), kept only for
	// documentation.
	maxByteSliceSize = 1 * 1024 * 1024
)

// decodeAPICache lazily builds and caches one frozen hamba/avro API per
// input-size bucket (bucketed to the next power of two, via
// decodeAPIForInputSize below), so repeated Serde.Unmarshal calls with
// similarly sized input reuse the same frozen API -- and, importantly, its
// decoder/encoder reflection caches -- instead of paying hamba/avro's
// per-type codec construction cost (via Config.Freeze) on every call. A
// sync.Map is safe for concurrent use without an explicit lock, so building
// distinct APIs per input-size bucket does not reintroduce the shared-
// mutable-state problem the previous MaxInputSize package var had (see
// WithMaxInputSize's doc comment for that history): entries here are
// write-once-per-bucket and never mutated after being stored.
var decodeAPICache sync.Map // map[int]avro.API, keyed by bits.Len(bucket ceiling)

// decodeAPIForInputSize returns a frozen hamba/avro API whose
// MaxSliceAllocSize is the smallest power of two >= n (n = the input's byte
// length, i.e. len(b) in Serde.Unmarshal).
//
// This directly addresses point 1 in the package doc above: hamba/avro's
// MaxSliceAllocSize bounds a declared array's cumulative element count, not
// its byte size, so a single fixed constant is either too loose for small
// inputs (a 5-byte payload can still declare hundreds of millions of
// elements, as demonstrated) or too tight for legitimate large ones.
// Deriving the ceiling from the actual input size bounds a declared element
// count to within ~2x of the number of bytes actually available (rounding
// up to the next power of two so buckets -- and therefore frozen APIs --
// are reused across calls instead of rebuilt every time), instead of an
// amount unrelated to input size entirely.
//
// This is not a precise per-byte bound (every array element of a non-null
// type costs >= 1 wire byte, which would justify capping declared count at
// exactly len(b); this implementation trades that precision for reusing a
// small, bounded number of frozen APIs instead of calling Config.Freeze on
// every Unmarshal, which would evict hamba/avro's per-type decoder cache on
// every call). It is also not a whole-message bound (see the package doc's
// note on per-field-call budgets), and it does nothing for "null"-typed
// array items, which cost zero wire bytes per element and so are not
// bounded by input size at all -- closing that residual gap would require
// bounding declared element count against a value not derived from the
// schema at all, which is out of scope here because it requires the schema
// itself to be attacker-influenced (Conduit's schemas are established
// out-of-band, not carried per-record; this residual applies only if a
// caller both trusts a schema with a bare "null"-typed array/map field and
// distrusts the bytes decoded against it -- an unusual combination, called
// out explicitly rather than silently assumed away).
func decodeAPIForInputSize(n int) avro.API {
	if n < 1 {
		n = 1
	}
	exp := bits.Len(uint(n - 1)) // ceiling = 1<<exp, smallest power of two >= n
	if v, ok := decodeAPICache.Load(exp); ok {
		return v.(avro.API) //nolint:forcetypeassert // only this function stores into decodeAPICache
	}
	api := avro.Config{
		MaxByteSliceSize:  maxByteSliceSize,
		MaxSliceAllocSize: 1 << exp,
	}.Freeze()
	actual, _ := decodeAPICache.LoadOrStore(exp, api)
	return actual.(avro.API) //nolint:forcetypeassert // see above
}

// Option configures a Serde constructed by Parse or SerdeForType.
type Option func(*serdeOptions)

type serdeOptions struct {
	maxInputSize int
}

func resolveOptions(opts []Option) serdeOptions {
	o := serdeOptions{maxInputSize: defaultMaxInputSize}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// WithMaxInputSize overrides the default input-size ceiling (see
// defaultMaxInputSize) for a specific Serde. Most callers should not need
// this.
//
// This replaces what was, in the previous version of this mitigation, an
// exported mutable package variable (avro.MaxInputSize) read by every
// Serde.Unmarshal call and writable by any caller at any time. That was a
// confirmed data race under `go test -race`: schema.Schema.Serde() caches
// Serde instances in a single process-wide globalSerdeCache
// (schema/schema.go), so every pipeline in a process shares the same Serde
// for a given schema fingerprint, and every one of them read the same
// package var that any other goroutine (a test, a differently-configured
// pipeline, a plugin) could mutate concurrently. Making the ceiling a field
// set once at construction time and never mutated afterward removes the
// race, narrows the blast radius of an override to the Serde it was
// requested on instead of the whole process, and removes the exported
// mutable global from this package's API surface entirely.
func WithMaxInputSize(n int) Option {
	return func(o *serdeOptions) { o.maxInputSize = n }
}
