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
//   - GO-2026-5048 -- unbounded map allocation
//
// The long-term call -- replacing hamba/avro with an actively maintained
// fork, github.com/iskorotkov/avro/v2 -- is made in
// docs/design-documents/20260823-avro-codec-archived-decoder-advisories.md
// in ConduitIO/conduit. This file is the near-term, codec-independent
// mitigation.
//
// This is a third pass at that mitigation. Read this history before
// changing anything below -- both previous versions were rejected for
// reasons that constrain what a correct version can look like.
//
// # Pass 1 (rejected): a hard default ceiling justified against the wrong
// number
//
// The first version hard-capped input at 10 MiB and justified it as
// matching "Conduit's own plugin gRPC transport ceiling"
// (grpc.MaxRecvMsgSize(10*1024*1024) in pkg/conduit/runtime.go). That
// citation was wrong: that call is inside Runtime.serveGRPCAPI, which
// registers the pipeline/processor/connector/plugin CRUD API -- the control
// plane. It carries no records. The actual data-path ceiling, for
// standalone plugins, is hashicorp/go-plugin's
// grpc.MaxCallRecvMsgSize(math.MaxInt32) (go-plugin@v1.8.0 grpc_client.go:
// 40) -- effectively unbounded, ~2047 MiB -- and built-in connectors never
// cross gRPC at all, so they have no ceiling of any kind. A 10 MiB default
// would have rejected legitimate records this project has no ability to
// rule out (e.g. a Postgres source emitting a row with several MiB of
// jsonb/bytea columns).
//
// # Pass 2 (rejected): a "safer" hard default, still guessing
//
// The second version raised the default to 256 MiB and reframed it
// honestly as "not derived from any known Conduit transport limit, a
// deliberately generous backstop." That framing was honest but the
// decision underneath it was still wrong: this repo has no telemetry on
// real-world Conduit record sizes, and nothing on the data path caps
// record size today (see Pass 1's citations -- they did not change).
// Picking *any* nonzero default, however generous, is choosing a number
// with someone else's pipeline data. If that number is ever too small for
// a real (if unusual) record, this package silently breaks a working
// pipeline by default, for a security property that -- once the codec
// replacement lands -- is not even this package's job to provide (see
// below).
//
// # Pass 3 (this version): unlimited by default, opt-in ceiling, no
// input-length-derived array bound
//
// MaxInputSize now defaults to 0 (unlimited): Serde.Unmarshal enforces no
// input-size ceiling unless a caller explicitly opts in via
// WithMaxInputSize. This is a deliberate policy choice, not an oversight:
// once the codec replacement lands, GO-2026-5046/5047/5048 close at the
// decoder (Reader.ReadBlockHeader's overflow guard, Config.MaxMapAllocSize)
// regardless of input size, so an input-size ceiling stops being a security
// control and becomes what it always should have been framed as: an
// operator policy knob -- "refuse absurd records early, before allocating
// anything" -- for an operator who actually knows their own record shapes
// and wants that belt-and-braces behavior on top of the codec fix, not a
// substitute for it. A policy knob must not break a stranger's working
// pipeline by default. See WithMaxInputSize's doc comment for how to opt
// in, and why an operator reading from an untrusted upstream should.
//
// This has a direct consequence for the array-allocation bound
// (MaxSliceAllocSize) that pass 2 derived from each call's input length:
// that derivation is only sound if input length itself is bounded by
// something the attacker cannot inflate. It is not, once MaxInputSize
// defaults to unlimited: an attacker can pad an otherwise-tiny malicious
// payload (a handful of bytes declaring an absurd element count) with
// arbitrary trailing garbage the array decoder never reaches, inflating
// the *observed* input length past their own declared count and defeating
// a bound derived from it. Confirmed directly: a 100,000,000-element
// declared count, backed by a 4-byte header plus 200 MiB of irrelevant
// padding, sails through a MaxSliceAllocSize set to len(the whole
// padded payload) with err == nil. Deriving the array bound from a
// specific call's observed byte length is therefore not a sound
// mitigation on its own and has been removed as pass 2 implemented it.
//
// What replaces it: decodeAPI's MaxSliceAllocSize is now derived from the
// Serde's own *configured* maxInputSize (an operator-chosen, pre-decode-
// enforced constant established at construction time), not from any
// per-call, attacker-observable value -- when maxInputSize is unset
// (the default), no array-allocation ceiling is applied here either, for
// the same "don't guess" reasoning as MaxInputSize itself: this package
// cannot know what a legitimate array field looks like across every
// Conduit deployment, any more than it can know what a legitimate record
// size looks like. When an operator does opt into WithMaxInputSize(n),
// Serde.Unmarshal already rejects any input longer than n before decoding
// starts, which makes capping declared array-element count at n sound
// (not padding-bypassable, because n bounds the *whole* input, checked
// up front, not the array decoder's specific slice of it): every element
// of a non-null type costs at least one wire byte, so a declared count
// exceeding the total number of bytes the caller was allowed to send at
// all is provably impossible for real data.
//
// This still does not close GO-2026-5046, GO-2026-5047, or, for
// non-null-item arrays with a caller who has not opted into
// WithMaxInputSize, the pass-1/pass-2 style unbounded-allocation case
// either -- it is deliberately scoped to be sound rather than to look more
// protective than it is. Verified by direct reproduction (see
// limits_test.go):
//
//  1. GO-2026-5046 (map CPU exhaustion): hamba/avro's *string-keyed* map
//     decoder (mapDecoder.Decode, codec_map.go) -- the only path
//     Serde.Unmarshal's own map[string]any/map[string]string usage
//     ever takes -- checks the reader's error state after every element
//     and bails on the first failed read; not vulnerable in practice for
//     this package's own usage (confirmed:
//     TestMapDecodeCPUExhaustion_SafeStringKeyedPath, ~60us for a
//     5-million-entry declared block backed by 4 bytes of input). But
//     Serde.Unmarshal is public API with a caller-controlled destination
//     type, and hamba/avro's *second* map decoder, mapDecoderUnmarshaler
//     (taken when the destination map's key type implements
//     encoding.TextUnmarshaler via a pointer receiver), has no such check
//     at all, and no Config field bounds a map's declared block length
//     either way. A 4-byte payload declaring a 5-million-entry block,
//     backed by no further data, decodes 5,000,000 fabricated entries in
//     several seconds of real CPU time with err == nil
//     (TestMapDecodeCPUExhaustion_UnsafeTextUnmarshalerPath). Nothing in
//     this file mitigates this -- there is no Config field to tighten,
//     unlike arrays, at any input-size policy. This is the strongest
//     concrete argument for the codec replacement: the iskorotkov/avro
//     fork adds Config.MaxMapAllocSize, which this class of bug has no
//     equivalent bound against on hamba/avro at all.
//  2. GO-2026-5047 (integer overflow): Reader.ReadBlockHeader does
//     `-length` on a value that decoded to math.MinInt64 (an 11-byte
//     crafted payload). Negating math.MinInt64 overflows back to
//     math.MinInt64 -- still negative -- bypassing every positive-ceiling
//     guard by sign alone, regardless of what any Config field is set to,
//     and producing len(out) == math.MinInt64, cap(out) == 0, err == nil
//     (TestArrayBound_NegativeLengthOverflow_Documented). This is not
//     just an inert negative number: the crafted payload was observed to
//     leave the process in a state where subsequent, logically-unrelated
//     boolean comparisons in the same test misbehaved -- consistent with
//     memory corruption from the unsafe growth path that follows, not
//     merely a "wrong answer." Treat any successful decode into this
//     state as unsafe to touch further (no append, no indexing, no
//     range). Nothing in this file catches this: it happens inside
//     ReadBlockHeader, upstream of any Config field this package sets.
//     Verified fixed at the source in github.com/iskorotkov/avro/v2
//     v2.34.0: Reader.ReadBlockHeader explicitly checks
//     `length64 <= math.MinInt` before negating and rejects it with an
//     error, unconditionally, regardless of Config. This is the other
//     strongest concrete argument for the codec replacement.
//
// What the codec replacement does NOT fix on its own (verified against
// github.com/iskorotkov/avro/v2 v2.34.0, not assumed from its being a
// fork): the array decoder's allocate-before-validate pattern, and
// MaxSliceAllocSize being an element count rather than a byte budget, is
// unchanged in the fork -- the exact TestArrayBound_ElementCountVsInputSize
// payload, run against the fork with the same numeric MaxSliceAllocSize
// value, reproduces identically. The fork's array codec restructured the
// bound check (compares the current block's declared length against
// remaining budget before growing, closing a related but distinct
// cumulative-overflow risk across multiple blocks) but did not change what
// unit the budget is denominated in, and does not close the padding-
// inflation bypass either (that bypass is about how a *caller* derives the
// ceiling it passes to Config, not about which codec enforces it). This
// package's opt-in, configured-ceiling-derived MaxSliceAllocSize is
// therefore not superseded by the codec replacement and stays useful under
// either codec -- but it is real protection only for a caller who has
// opted into WithMaxInputSize. There is no upstream (hamba or fork) fix
// for a caller who has not, and there being no such fix even on the fork
// is a real, open gap worth tracking as a follow-up against
// github.com/iskorotkov/avro/v2 once this repo depends on it.
//
// Also not addressed by either codec at any input-size policy: hamba/avro's
// MaxSliceAllocSize (and the fork's, and its new MaxMapAllocSize) bounds
// allocation *per field-decode call*, not cumulatively across a whole
// decoded message. A record with N independent array (or, on the fork,
// map) fields can each independently claim up to the configured ceiling,
// multiplying the worst-case allocation by N. There is no whole-message
// allocation budget hook in either library's public API.
//
// maxByteSliceSize is a no-op by construction: hamba/avro's
// getMaxByteSliceSize() resolves 0 to its own built-in default
// (defaultMaxByteSliceSize, 1 MiB) exactly as our explicit 1*1024*1024
// does -- setting the field cannot, by construction, ever produce
// different behavior than leaving it unset. It stays here as documentation
// of intent (explicit instead of implicit-in-an-archived-dependency), not
// as a behavior change; TestByteSliceBound_Fires proves the ceiling itself
// (whichever value resolves it) actually rejects an oversized field.
//
// Known compatibility caveat, not fixed here: every avro.Config.Freeze()
// call this package makes allocates a fresh *TypeResolver (hamba/avro's
// Freeze() always does this; see config.go). This package's own union
// handling does not depend on it -- unionResolver in union.go carries its
// own independent avro.TypeResolver, populated per-Serde via
// BeforeMarshal/AfterUnmarshal, and every existing test in this package
// exercises that path, unaffected by this file. But hamba/avro also has
// its own, separate, resolver-driven union-decoding fallback inside
// codec_union.go, consulted through the frozen API's resolver field.
// Before this mitigation, Serde.Unmarshal called the package-level
// avro.Unmarshal, which resolves through avro.DefaultConfig -- a single
// process-wide instance any caller's avro.Register(name, obj) populates.
// Now that Serde.Unmarshal calls into this package's own frozen API
// instead, any such external avro.Register call no longer reaches this
// package's decode path: hamba/avro's own union-resolution fallback (not
// this package's unionResolver) silently degrades to a generic map with
// no error instead of resolving to the registered Go type. No code in
// this repository calls avro.Register (grep confirms zero hits outside
// hamba/avro's own source), so this has no effect on any current caller.
// It is a real, silent behavior difference for any external module that
// both imports conduit-commons/schema/avro directly and separately calls
// hamba/avro's package-level Register -- documented here, not fixed, since
// no current caller needs a Register passthrough this package does not
// expose.
const (
	// maxByteSliceSize bounds a single `bytes`/`string` field. See the
	// package doc above: this is a no-op by construction (hamba/avro
	// resolves an unset field to the same 1 MiB default), kept only for
	// documentation.
	maxByteSliceSize = 1 * 1024 * 1024
)

// Option configures a Serde constructed by Parse or SerdeForType.
type Option func(*serdeOptions)

type serdeOptions struct {
	maxInputSize int
}

func resolveOptions(opts []Option) serdeOptions {
	var o serdeOptions // maxInputSize: 0, i.e. unlimited, by default -- see limits.go's package doc
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// WithMaxInputSize makes a Serde reject Unmarshal input larger than n bytes
// with ErrInputTooLarge, before it reaches the underlying decoder. n <= 0
// means unlimited, which is also the default when this option is not
// passed at all.
//
// There is no default ceiling. See limits.go's package doc for the full
// reasoning; in short: nothing on Conduit's data path bounds record size
// today (hashicorp/go-plugin's client sets
// grpc.MaxCallRecvMsgSize(math.MaxInt32), and built-in connectors never
// cross gRPC at all), this package has no telemetry on real-world record
// shapes across every Conduit deployment, and picking a default ceiling
// anyway means guessing with someone else's pipeline -- silently breaking
// a legitimate large record the day it shows up.
//
// Opt into this if the Serde will decode bytes from a source this operator
// does not fully trust (e.g. records crossing a boundary from an external
// or third-party upstream) and the operator can state a real ceiling for
// their own record shapes. Treat it as belt-and-braces on top of the codec
// fix (see the design doc), not a substitute for it: setting this also
// tightens the decoder's array-allocation ceiling (MaxSliceAllocSize) to
// n, which is a sound bound only because Unmarshal already rejects any
// input longer than n before decoding starts -- every array element of a
// non-null type costs at least one wire byte, so a declared element count
// exceeding n is provably impossible for real data once n itself is
// enforced up front. Leaving this unset (the default) applies no
// array-allocation ceiling either, for the same reason: this package
// cannot know what a legitimate array field looks like any more than it
// can know what a legitimate record size looks like.
//
// This also replaces what was, in an earlier version of this mitigation,
// an exported mutable package variable (avro.MaxInputSize) read by every
// Serde.Unmarshal call and writable by any caller at any time -- a
// confirmed data race under `go test -race`, since Serde instances are
// shared process-wide via schema.globalSerdeCache. Making the ceiling a
// field set once at construction time and never mutated afterward removes
// the race and narrows the blast radius of an override to the Serde it
// was requested on instead of the whole process.
func WithMaxInputSize(n int) Option {
	return func(o *serdeOptions) { o.maxInputSize = n }
}

// buildDecodeAPI returns the frozen hamba/avro API a Serde with the given
// configured maxInputSize should decode with. See the package doc above
// and WithMaxInputSize for why MaxSliceAllocSize is only set (to
// maxInputSize itself) when maxInputSize is a real, enforced ceiling, and
// left at hamba/avro's own default otherwise.
func buildDecodeAPI(maxInputSize int) avro.API {
	cfg := avro.Config{MaxByteSliceSize: maxByteSliceSize}
	if maxInputSize > 0 {
		cfg.MaxSliceAllocSize = maxInputSize
	}
	return cfg.Freeze()
}
