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
	"fmt"
	"sync/atomic"

	"github.com/iskorotkov/avro/v2"
)

// Decode-side input bounds for github.com/iskorotkov/avro/v2.
//
// This package originally decoded with github.com/hamba/avro/v2, which was
// archived by its maintainer on 2026-01-18 (final release v2.31.0) carrying
// three unfixed decoder advisories, all reachable by decoding untrusted
// Avro bytes -- exactly what Serde.Unmarshal does with records arriving
// from an upstream Conduit does not control:
//
//   - GO-2026-5046 -- CPU exhaustion in the array/map decoders
//   - GO-2026-5047 -- integer overflow in cumulative-size arithmetic
//   - GO-2026-5048 -- unbounded map allocation
//
// The codec was replaced with github.com/iskorotkov/avro/v2 (an actively
// maintained fork created specifically in response to hamba/avro's
// archival) per the decision in
// docs/design-documents/20260823-avro-codec-archived-decoder-advisories.md
// in ConduitIO/conduit. This file's history below (the three "Pass"
// sections) predates the codec swap and describes the near-term,
// codec-independent mitigation this package shipped first, against
// hamba/avro, before the swap landed. It is preserved because the
// reasoning still applies unchanged to the fork -- MaxInputSize's
// unlimited-by-default policy, and why, did not change when the codec
// did. What changed with the swap is documented in the section after it,
// "The codec swap: what it fixes and what it does not," which is the part
// to read first if you are here because of GO-2026-5046/5047/5048
// specifically.
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
// replacement lands -- is not even this package's job to provide alone
// (see below).
//
// # Pass 3: unlimited by default, opt-in ceiling, no input-length-derived
// array bound
//
// MaxInputSize defaults to 0 (unlimited): Serde.Unmarshal enforces no
// input-size ceiling unless a caller explicitly opts in via
// WithMaxInputSize. This is a deliberate policy choice, not an oversight:
// nothing on Conduit's data path bounds record size today (Pass 1's
// citations), this repo has no telemetry on real-world record shapes, and
// this package cannot know what a legitimate record looks like across
// every Conduit deployment any more than it can know what a legitimate
// array field looks like (see the codec-swap section below for that other
// half). A policy knob must not break a stranger's working pipeline by
// default. See WithMaxInputSize's doc comment for how to opt in, and why
// an operator reading from an untrusted upstream should.
//
// This has a consequence for how MaxInputSize, when configured, interacts
// with the array/map bounds described below: deriving an allocation
// ceiling from a given call's *observed* input length (rather than a
// configured constant) is not sound, because that observed length is
// itself attacker-inflatable by padding. Confirmed directly: a
// 100,000,000-element declared count, backed by a 4-byte header plus
// 200 MiB of irrelevant trailing padding, sails through an allocation
// ceiling set to len(the whole padded payload) with err == nil. Any
// ceiling this package derives from MaxInputSize is therefore derived from
// the Serde's *configured* maxInputSize (an operator-chosen, pre-decode-
// enforced constant established at construction time), never from a
// specific call's observed byte length.
//
// maxByteSliceSize is a no-op by construction: both hamba/avro's and the
// fork's getMaxByteSliceSize() resolve an unset (0) field to the same
// 1 MiB default our explicit 1*1024*1024 sets -- setting the field cannot,
// by construction, ever produce different behavior than leaving it unset.
// It stays here as documentation of intent (explicit instead of implicit
// in a dependency whose defaults a reader would otherwise have to go
// looking up), not as a behavior change; TestByteSliceBound_Fires proves
// the ceiling itself (whichever value resolves it) actually rejects an
// oversized field.
//
// # The codec swap: what it fixes and what it does not
//
// Replacing the codec closes one of the three advisories unconditionally
// and makes a second one closable for the first time -- but neither
// happens just by changing the import path. Verified directly against
// github.com/iskorotkov/avro/v2 v2.34.0 (not assumed from it being a
// fork), and re-verified against this package's own reproductions after
// the swap landed:
//
// Three findings, summarized here and detailed in the numbered list below
// -- "closed by the fork alone" versus "closed only once this file also
// sets the corresponding Config field":
//
//   - H1 (GO-2026-5047, ReadBlockHeader overflow -> negative-length slice,
//     err == nil): closed by the fork alone, unconditionally.
//   - C1 (2 GiB allocated from 5 wire bytes, array element count vs. byte
//     budget): left open by the fork alone; closed only once
//     MaxSliceAllocSize is set.
//   - C3 (GO-2026-5046/5048, unbounded map cardinality via a
//     TextUnmarshaler-keyed map): left open by the fork alone; closed only
//     once MaxMapAllocSize is set (and unclosable at all against
//     hamba/avro, which has no such field).
//
// # H1 detail
//
// Closed unconditionally, by the codec itself, regardless of Config.
// Reader.ReadBlockHeader negated a value that decoded to math.MinInt64 in
// hamba/avro (an 11-byte crafted payload). Negating math.MinInt64
// overflows back to math.MinInt64 -- still negative -- bypassing every
// positive-ceiling guard by sign alone, producing len(out) ==
// math.MinInt64, cap(out) == 0, err == nil. The fork's ReadBlockHeader
// explicitly checks `length64 <= math.MinInt` before negating and rejects
// it with an error, unconditionally -- TestSwap_H1_NegativeLengthOverflow_Fixed
// proves this against the package's real Serde.Unmarshal path.
//
// # C1 detail
//
// Left open by the codec swap alone. MaxSliceAllocSize bounds a declared
// array's cumulative *element count*, not its *byte size*, in both
// hamba/avro and the fork -- the fork restructured the check (it compares
// the current block's declared length against remaining budget before
// growing, closing a related but distinct cumulative-overflow risk across
// multiple blocks) but kept the same unit. A 5-byte payload declaring an
// array of 134,217,728 null items allocates the full backing array with
// err == nil unless MaxSliceAllocSize is set. Closed by this file always
// setting MaxSliceAllocSize (see "Default allocation ceilings" below) --
// TestSwap_C1_UnboundedByDefault_WouldFailPreSwap documents the
// pre-swap-equivalent failure mode and TestSwap_C1_BoundedByDefaultConfig
// proves the post-swap default rejects it.
//
// # C3 detail
//
// Left open by the codec swap alone, and it is the strongest concrete
// argument for having swapped codecs at all: hamba/avro has no Config
// field that bounds a map's declared block length, at any input-size
// policy -- MaxMapAllocSize does not exist on hamba/avro, full stop, so
// this finding was unclosable there regardless of how this package was
// configured. The fork adds Config.MaxMapAllocSize. A 4-byte payload
// declaring a multi-million-entry map block, decoded into a map keyed by a
// type implementing encoding.TextUnmarshaler (mapDecoderUnmarshaler,
// reachable through Serde.Unmarshal's caller-controlled destination type),
// fabricates that many entries from no further input, several seconds of
// real CPU time, err == nil, unless MaxMapAllocSize is set. Closed by this
// file always setting MaxMapAllocSize --
// TestSwap_C3_UnboundedByDefault_WouldFailPreSwap and
// TestSwap_C3_BoundedByDefaultConfig.
// Also unaddressed by either codec at any Config: neither library bounds
// allocation cumulatively across a whole decoded message, only per
// field-decode call. A record with N independent array or map fields can
// each independently claim up to the configured ceiling, multiplying the
// worst case by N. There is no whole-message allocation budget hook in
// either library's public API.
//
// # Default allocation ceilings: real defaults, not opt-in
//
// Unlike MaxInputSize (Pass 3 above), MaxSliceAllocSize and MaxMapAllocSize
// are NOT unlimited by default. Once the codec swap landed, these two
// fields are the only remaining closure for C1 and C3 above -- the
// strongest concrete security argument for the swap was that
// MaxMapAllocSize exists at all on the fork. Shipping them opt-in-only,
// mirroring MaxInputSize's reasoning, would mean the swap fixes nothing by
// default beyond H1: an operator who never calls WithMaxInputSize (the
// common case, since that option's own default is unlimited) would decode
// with unset MaxSliceAllocSize/MaxMapAllocSize, which the fork resolves to
// maxAllocSize (1<<48 on 64-bit) -- functionally the same unbounded
// exposure as hamba/avro today. That would make the swap a compliance
// exercise (govulncheck goes quiet) without an accompanying reduction in
// real exposure, which is not the point of doing it.
//
// The reason MaxInputSize and these two are not symmetric is the unit each
// is denominated in. MaxInputSize bounds *bytes*, and this repo has no
// telemetry ruling out a legitimate multi-hundred-MiB record (Pass 1/2's
// citations). MaxSliceAllocSize and MaxMapAllocSize bound *element/entry
// counts*, a different axis: a Conduit record's top-level array or map
// field holding on the order of a million entries is already an extreme,
// almost certainly batch-shaped payload, not an ordinary large field --
// unlike a single large byte/string value, which is a completely mundane
// shape (see MaxByteSliceSize above, and jsonb/bytea in Pass 1). A default
// on this axis can be generous enough to be invisible to every real record
// shape this project has ever seen while still bounding the worst case an
// attacker can force from a handful of wire bytes to a fixed, tolerable
// amount of memory and CPU -- which is not true of a byte-budget default.
//
// initialDefaultMaxAllocSize = 1,000,000 (elements/entries), applied to
// both MaxSliceAllocSize and MaxMapAllocSize (via
// defaultMaxSliceAllocSize/defaultMaxMapAllocSize, below) unless changed
// process-wide by SetDefaultMaxSliceAllocSize/SetDefaultMaxMapAllocSize.
// Justified as an element count, not a byte budget, per the reasoning
// above:
//
//   - Array worst case, widest common element type: decoding into []any
//     costs 16 bytes/element on 64-bit (a 2-word interface header: type
//     pointer + data pointer). 1,000,000 * 16 bytes = 16,000,000 bytes,
//     ~15.3 MiB, allocated as a single backing array before a single
//     element is decoded (the array decoder's allocate-before-validate
//     pattern -- see C1 above -- is unchanged by the ceiling; the ceiling
//     bounds how large that up-front allocation can be, not whether one
//     happens).
//   - Map worst case: measured directly (not estimated), decoding
//     1,000,000 entries into a map[string]any with realistic short string
//     keys and int64 values costs ~114 bytes/entry including Go's map
//     bucket overhead -- ~109 MiB. The CPU cost of the vulnerable
//     TextUnmarshaler-keyed decode path (C3 above) was measured at
//     ~220 ns/entry (20,000,000 entries took 4.41s of real CPU against an
//     unbounded Config), preserved as a reproduction in this file's git
//     history and re-run as part of TestSwap_C3_UnboundedByDefault_WouldFailPreSwap;
//     at the 1,000,000-entry default that bounds a single malicious map
//     field to ~220 ms of CPU, not the multi-second-to-unbounded cost an
//     attacker could force before this ceiling existed.
//   - This is a per-field-decode-call bound, not a per-message one (see
//     above): a record with many independent array/map fields multiplies
//     both numbers by the field count. Tracked as a known, documented gap
//     rather than solved here -- there is no whole-message allocation
//     budget hook in the codec's public API to hang a fix on.
//
// An operator with a genuine need for a single field larger than this
// (uncommon, but not impossible -- e.g. a bulk-embedding vector batch) can
// raise the ceiling one of two ways:
//
//   - Per Serde, via WithMaxSliceAllocSize / WithMaxMapAllocSize, for a
//     caller that constructs its own Serde through Parse/SerdeForType.
//   - Process-wide, via SetDefaultMaxSliceAllocSize /
//     SetDefaultMaxMapAllocSize, for the path that has no Option plumbing
//     of its own: schema.Schema.Unmarshal, which resolves a Serde through
//     schema.globalSerdeCache -> schema.KnownSerdeFactories[TypeAvro].Parse
//     -> avro.Parse(s) with no options (schema/schema.go). See those
//     functions' doc comments for the race-free mechanism and its
//     limitations (in particular: it does not retroactively change Serdes
//     already constructed, including ones already cached by fingerprint).
//
// There is deliberately no "unlimited" sentinel for either axis (n <= 0 is
// rejected by WithMaxSliceAllocSize / WithMaxMapAllocSize /
// SetDefaultMaxSliceAllocSize / SetDefaultMaxMapAllocSize, returned as an
// error, never silently treated as "unlimited" or silently ignored) --
// unlike MaxInputSize, an unbounded value on this axis is never the right
// default or a supportable override, since it is the control that makes
// C1/C3 closable at all.
//
// When MaxInputSize is also configured (WithMaxInputSize), it tightens
// both ceilings further when it is the smaller bound: every array/map
// element or entry of a non-null type costs at least one wire byte, so a
// declared count exceeding the total number of bytes the caller was
// allowed to send at all is provably impossible for real data -- the same
// soundness argument Pass 3 makes, generalized to also cover the map case
// the fork newly makes expressible.
//
// That soundness argument has one documented exception: a container whose
// element/value type is exactly the Avro `null` type (not a union with a
// null branch -- a union's branch index still costs a wire byte per
// element) costs zero wire bytes per element, so a declared count for such
// a container is not bounded by input size at all -- a 50,000-null array
// encodes to nothing but its block-length header. buildDecodeAPI skips the
// MaxInputSize-derived tightening (falling back to the configured/default
// MaxSliceAllocSize/MaxMapAllocSize instead, still enforced, just not
// additionally tightened) for any Serde whose schema contains such a
// container anywhere in its tree -- see schemaHasZeroCostContainer.
// TestMaxInputSize_TighteningSkipped_ZeroCostArrayItems is the regression
// test: it proves a legitimate small-input, many-null-element payload that
// the (unconditional) tightening used to reject now decodes.
const (
	// maxByteSliceSize bounds a single `bytes`/`string` field. See the
	// package doc above: this is a no-op by construction (both hamba/avro
	// and the fork resolve an unset field to the same 1 MiB default), kept
	// only for documentation.
	maxByteSliceSize = 1 * 1024 * 1024

	// initialDefaultMaxAllocSize is the built-in starting value for
	// defaultMaxSliceAllocSize / defaultMaxMapAllocSize (below), applied to
	// MaxSliceAllocSize and MaxMapAllocSize when no WithMaxSliceAllocSize /
	// WithMaxMapAllocSize override is given and no
	// SetDefaultMaxSliceAllocSize / SetDefaultMaxMapAllocSize call has
	// changed the package-wide default. See "Default allocation ceilings"
	// above for the worst-case heap and CPU math this value is justified
	// against.
	initialDefaultMaxAllocSize = 1_000_000
)

// defaultMaxSliceAllocSize and defaultMaxMapAllocSize back the package-wide
// defaults applied to MaxSliceAllocSize/MaxMapAllocSize when a Serde is
// constructed with no WithMaxSliceAllocSize/WithMaxMapAllocSize override.
// Left at their Go zero value (0) until SetDefaultMaxSliceAllocSize/
// SetDefaultMaxMapAllocSize is called -- effectiveDefaultMaxSliceAllocSize/
// effectiveDefaultMaxMapAllocSize below treat 0 as "never overridden,
// use initialDefaultMaxAllocSize", so no package-level init function is
// needed to seed a non-zero starting value (gochecknoinits). See those
// Set* functions' doc comments for why this exists (the
// schema.Schema.Unmarshal path has no per-call Option plumbing to reach
// WithMaxSliceAllocSize/WithMaxMapAllocSize) and what it does and does not
// affect. atomic.Int64, not a plain int guarded by a mutex, so reads on
// the hot Parse/SerdeForType construction path never block a concurrent
// writer and vice versa; every Serde still freezes its own immutable
// avro.API at construction time (see buildDecodeAPI), so these variables
// are only ever read, never held across a decode call.
var (
	defaultMaxSliceAllocSize atomic.Int64
	defaultMaxMapAllocSize   atomic.Int64
)

// effectiveDefaultMaxSliceAllocSize and effectiveDefaultMaxMapAllocSize
// resolve the current package-wide default, falling back to
// initialDefaultMaxAllocSize for the zero value (SetDefaultMax*AllocSize
// never called).
func effectiveDefaultMaxSliceAllocSize() int {
	if v := defaultMaxSliceAllocSize.Load(); v > 0 {
		return int(v)
	}
	return initialDefaultMaxAllocSize
}

func effectiveDefaultMaxMapAllocSize() int {
	if v := defaultMaxMapAllocSize.Load(); v > 0 {
		return int(v)
	}
	return initialDefaultMaxAllocSize
}

// SetDefaultMaxSliceAllocSize changes the package-wide default ceiling
// (initially initialDefaultMaxAllocSize, see the package doc's "Default
// allocation ceilings") applied to MaxSliceAllocSize for every Serde
// subsequently constructed by Parse or SerdeForType without an explicit
// WithMaxSliceAllocSize override. n must be > 0; n <= 0 returns an error
// and leaves the current default unchanged -- there is no "unlimited"
// value for this axis, see the package doc for why.
//
// This is the escape hatch for the one construction path that has no
// per-call Option of its own: schema.Schema.Unmarshal resolves a Serde
// through schema.globalSerdeCache -> schema.KnownSerdeFactories[TypeAvro].
// Parse -> avro.Parse(s), with no options threaded through
// schema.SerdeFactory. A caller who has determined that their deployment
// legitimately needs a larger single array field on that path can call
// SetDefaultMaxSliceAllocSize once, process-wide (e.g. at startup, before
// any pipeline runs), rather than being stuck with
// initialDefaultMaxAllocSize with no way to reach it.
//
// Race-free: backed by atomic.Int64, safe to call concurrently with
// Parse/SerdeForType/Unmarshal on any goroutine. It is not, however,
// retroactive: it changes what effectiveDefaultMaxSliceAllocSize() returns
// for Serdes constructed *after* the call, not the frozen avro.API of a Serde
// already constructed -- each Serde's decodeAPI is built once, at
// construction, from whatever the default (or override) resolved to at
// that moment (see buildDecodeAPI). In particular, a schema already parsed
// and cached in schema.globalSerdeCache keeps its existing ceiling until
// that cache entry expires (schema.globalSerdeCache.MaxAge, 4 hours) or is
// otherwise evicted and re-parsed. Call this as early as possible in a
// process's lifetime, ideally before any schema has been parsed at all, to
// avoid that window.
func SetDefaultMaxSliceAllocSize(n int) error {
	if n <= 0 {
		return fmt.Errorf("%w: SetDefaultMaxSliceAllocSize requires n > 0, got %d", ErrInvalidOption, n)
	}
	defaultMaxSliceAllocSize.Store(int64(n))
	return nil
}

// SetDefaultMaxMapAllocSize is SetDefaultMaxSliceAllocSize's counterpart
// for MaxMapAllocSize. See SetDefaultMaxSliceAllocSize's doc comment for
// the full reasoning, the race-free/non-retroactive guarantees, and why
// this exists at all (schema.Schema.Unmarshal has no per-call Option
// plumbing to reach WithMaxMapAllocSize).
func SetDefaultMaxMapAllocSize(n int) error {
	if n <= 0 {
		return fmt.Errorf("%w: SetDefaultMaxMapAllocSize requires n > 0, got %d", ErrInvalidOption, n)
	}
	defaultMaxMapAllocSize.Store(int64(n))
	return nil
}

// Option configures a Serde constructed by Parse or SerdeForType.
type Option func(*serdeOptions) error

type serdeOptions struct {
	maxInputSize      int
	maxSliceAllocSize int // 0 = use defaultMaxSliceAllocSize; see WithMaxSliceAllocSize
	maxMapAllocSize   int // 0 = use defaultMaxMapAllocSize; see WithMaxMapAllocSize
}

func resolveOptions(opts []Option) (serdeOptions, error) {
	var o serdeOptions // maxInputSize: 0, i.e. unlimited, by default -- see limits.go's package doc
	for _, opt := range opts {
		if err := opt(&o); err != nil {
			return serdeOptions{}, err
		}
	}
	return o, nil
}

// WithMaxInputSize makes a Serde reject Unmarshal input larger than n bytes
// with ErrInputTooLarge, before it reaches the underlying decoder. n <= 0
// means unlimited, which is also the default when this option is not
// passed at all -- unlike WithMaxSliceAllocSize/WithMaxMapAllocSize below,
// n <= 0 here is a valid, meaningful value (not an error), because
// "unlimited" is a legitimate and, in fact, the recommended default on
// this axis. See limits.go's package doc for the full reasoning; in short:
// nothing on Conduit's data path bounds record size today (hashicorp/
// go-plugin's client sets grpc.MaxCallRecvMsgSize(math.MaxInt32), and
// built-in connectors never cross gRPC at all), this package has no
// telemetry on real-world record shapes across every Conduit deployment,
// and picking a default ceiling anyway means guessing with someone else's
// pipeline -- silently breaking a legitimate large record the day it shows
// up. This is unlike WithMaxSliceAllocSize / WithMaxMapAllocSize below,
// which do have non-zero defaults; see "Default allocation ceilings" in
// the package doc for why that asymmetry is deliberate.
//
// Opt into this if the Serde will decode bytes from a source this operator
// does not fully trust (e.g. records crossing a boundary from an external
// or third-party upstream) and the operator can state a real ceiling for
// their own record shapes. When set, this also tightens
// MaxSliceAllocSize/MaxMapAllocSize further, to n, whenever n is smaller
// than their own (already-enforced) default or override, UNLESS the
// Serde's schema contains an array/map whose item/value type is exactly
// the Avro `null` type (see the package doc's exception to this
// soundness argument, and schemaHasZeroCostContainer) -- sound in the
// general case because Unmarshal already rejects any input longer than n
// before decoding starts, and every array/map element of a non-null type
// costs at least one wire byte, so a declared count exceeding n is
// provably impossible for real data once n itself is enforced up front.
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
	return func(o *serdeOptions) error {
		o.maxInputSize = n
		return nil
	}
}

// WithMaxSliceAllocSize overrides, for this Serde only, the package-wide
// default ceiling (defaultMaxSliceAllocSize, see the package doc) on the
// cumulative element count a single declared Avro array field may allocate
// before this package rejects it. n must be > 0, checked eagerly: Parse/
// SerdeForType return an error immediately (wrapping ErrInvalidOption) if
// n <= 0, rather than silently falling back to the default -- there is no
// "unlimited" value for this option, unlike WithMaxInputSize, and a
// caller-supplied n <= 0 (e.g. an unvalidated, accidentally-zero computed
// value) is far more likely to be a bug at the call site than a deliberate
// request for "unlimited," so this fails closed and loud instead of
// silently substituting a value the caller didn't ask for.
//
// Use this only if a real record shape needs a single array field larger
// than the default (~15.3 MiB worst case for []any at the default); most
// callers should not need it. For the one construction path that has no
// Option of its own (schema.Schema.Unmarshal), see
// SetDefaultMaxSliceAllocSize instead.
func WithMaxSliceAllocSize(n int) Option {
	return func(o *serdeOptions) error {
		if n <= 0 {
			return fmt.Errorf("%w: WithMaxSliceAllocSize requires n > 0, got %d", ErrInvalidOption, n)
		}
		o.maxSliceAllocSize = n
		return nil
	}
}

// WithMaxMapAllocSize overrides, for this Serde only, the package-wide
// default ceiling (defaultMaxMapAllocSize, see the package doc) on the
// cumulative entry count a single declared Avro map field may allocate
// before this package rejects it. n must be > 0, checked eagerly, for the
// same reasons as WithMaxSliceAllocSize -- see its doc comment.
//
// Use this only if a real record shape needs a single map field larger
// than the default (~109 MiB measured worst case for map[string]any at the
// default); most callers should not need it. For the one construction path
// that has no Option of its own (schema.Schema.Unmarshal), see
// SetDefaultMaxMapAllocSize instead.
func WithMaxMapAllocSize(n int) Option {
	return func(o *serdeOptions) error {
		if n <= 0 {
			return fmt.Errorf("%w: WithMaxMapAllocSize requires n > 0, got %d", ErrInvalidOption, n)
		}
		o.maxMapAllocSize = n
		return nil
	}
}

// schemaHasZeroCostContainer reports whether s contains, anywhere in its
// tree, an array or map whose element/value type is exactly the Avro
// `null` type -- as opposed to, say, a union with a null branch, which
// still costs at least one wire byte per element for the union's branch
// index. Such a container can encode an arbitrarily large declared element
// count in a handful of wire bytes (a 50,000-element array of `null`
// encodes to nothing but its block-length header), which is exactly the
// case the MaxInputSize-derived tightening in buildDecodeAPI is unsound
// for: see the package doc's "one documented exception" paragraph. The
// check is schema-wide, not per-field, because the tightened ceiling
// itself (avro.Config.MaxSliceAllocSize/MaxMapAllocSize) applies uniformly
// to every array/map this Serde's decodeAPI decodes, not to one field in
// isolation -- so if any container anywhere in the schema is zero-cost,
// the tightening is skipped for the whole Serde rather than silently
// applied unsoundly to just that one field.
func schemaHasZeroCostContainer(s avro.Schema) bool {
	found := false
	traverseSchema(s, func(p path) {
		if found || len(p) == 0 {
			return
		}
		switch t := p[len(p)-1].schema.(type) {
		case *avro.ArraySchema:
			if t.Items().Type() == avro.Null {
				found = true
			}
		case *avro.MapSchema:
			if t.Values().Type() == avro.Null {
				found = true
			}
		}
	})
	return found
}

// buildDecodeAPI returns the frozen avro.API a Serde with the given schema
// and options should decode with. See the package doc above for why
// MaxSliceAllocSize/MaxMapAllocSize default to the package-wide
// defaultMaxSliceAllocSize/defaultMaxMapAllocSize rather than being
// unbounded, why that default differs from MaxInputSize's, how a
// configured MaxInputSize additionally tightens both when it is the
// smaller bound, and the zero-cost-container exception to that tightening
// (schemaHasZeroCostContainer).
func buildDecodeAPI(schema avro.Schema, o serdeOptions) avro.API {
	sliceLimit := effectiveDefaultMaxSliceAllocSize()
	if o.maxSliceAllocSize > 0 {
		sliceLimit = o.maxSliceAllocSize
	}
	mapLimit := effectiveDefaultMaxMapAllocSize()
	if o.maxMapAllocSize > 0 {
		mapLimit = o.maxMapAllocSize
	}
	if o.maxInputSize > 0 && !schemaHasZeroCostContainer(schema) {
		if o.maxInputSize < sliceLimit {
			sliceLimit = o.maxInputSize
		}
		if o.maxInputSize < mapLimit {
			mapLimit = o.maxInputSize
		}
	}
	cfg := avro.Config{
		MaxByteSliceSize:  maxByteSliceSize,
		MaxSliceAllocSize: sliceLimit,
		MaxMapAllocSize:   mapLimit,
	}
	return cfg.Freeze()
}
