# A direct, zero-allocation encoder for the opencdc.v2 record wire format

## Summary

Add `Record.MarshalV2()` and `Record.AppendV2(dst []byte)` to `conduit-commons/opencdc`.
They encode a record to the `opencdc.v2` wire format by writing protobuf bytes directly from the Go
record, instead of building the intermediate `*opencdc.v2.Record` tree with `ToProtoV2` and then
running `proto.Marshal`'s reflection-based encoder over it. The bytes are identical to
`proto.MarshalOptions{Deterministic: true}` applied to the `ToProtoV2` tree, and the errors are
identical to `ToProtoV2`. `AppendV2` appends into a caller-owned buffer and reuses a pooled encoder,
so the steady state allocates nothing per record.

The current wire boundary is ~6.6 µs and 88 allocations per record; `MarshalV2` cuts it to ~1.7 µs
and 7 allocations (the 7 are the output-buffer growth), and `AppendV2` with a reused buffer reaches
~1.1 µs and zero allocations. The proto schema, the public `opencdc.Record` shape, and the wire
format are unchanged.

## Context

Profiling the Postgres CDC connector showed that `opencdc.Record.ToProtoV2` + `proto.Marshal` is the
dominant per-record cost on the source → protocol wire boundary. A `-benchmem` run on a
representative record (a structured payload with int64, uint64, decimal, float, bytes, a nested map
and an array, plus a structured key and metadata) measured:

| Path | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| `ToProtoV2` (build the tree) | ~1.8 µs | 3168 | 52 |
| `ToProtoV2` + `proto.Marshal` | ~6.6 µs | 4128 | 88 |

A CPU/allocation profile splits the cost in two, and the marshaling is the larger half:

1. **`ToProtoV2` builds a fresh `*opencdc.v2.Record` tree.** Every structured value allocates a
   `Value` message and its oneof wrapper (`&Value{Kind: &Value_Int64{...}}`), a `Struct` and its
   `Fields` map, a `Data` and its oneof wrapper, and so on. This is the "double traversal" the task
   describes: the SDK already built the `map[string]any` StructuredData, and the wire boundary walks
   it again to build a typed tree.
2. **`proto.Marshal` walks that tree through reflection.** `sizePointerSlow` /
   `marshalAppendPointer`, map iteration via `reflect.MapIter`, and the output buffer account for
   ~2.4 µs and ~24 allocations on their own — `MarshalAppend` with a pre-sized buffer removes only
   one of them, because the reflection machinery and the per-entry boxing remain.

`proto.Marshal` cannot be made cheap here: protoc-gen-go v1.31 generates reflection-only code, and
the opencdc.v2 message is deeply nested with maps, so the reflection path allocates on every entry.
The connector protocol wraps each record in a `connectorv3.Source_Run_Response` and lets gRPC marshal
the outer message, so the nested-record marshal cost lands on the connector's hot path.

The SDK `ReadN` ownership contract (conduit-connector-sdk#426) forces three fresh maps per record that
are then re-walked here. This change gives the commons side of the fix a zero-allocation escape hatch;
the SDK/protocol adoption that reuses a buffer and calls it is the follow-up.

## Alternatives

### A. Hand-written direct encoder, pooled (chosen)

Encode the fixed `opencdc.v2` schema directly into a caller-owned byte slice using `protowire` for the
low-level varint/length/fixed64 encoding, and a `sync.Pool` of a small marshaler struct that reuses
its error-path stack and its per-depth key-sort scratch. Map fields are emitted in sorted key order,
matching protobuf's deterministic marshaler, which makes the output byte-comparable to the reference
for every input (not just map-free records).

### B. Reuse the proto message tree with a `sync.Pool` (rejected)

Pool the `*opencdc.v2.Record` and reset it recursively between records, so `ToProtoV2` reuses the
Value/Struct/List nodes instead of allocating them.

Rejected because it fixes only the smaller half. Resetting the tree correctly requires walking it to
clear every oneof and map entry, which is itself a traversal and a source of retention bugs; a stale
`Value` leaked into the next record corrupts it silently. And it does not touch the ~2.4 µs /
24-alloc reflection marshal, because the protocol still marshals the tree through gRPC. The measured
"reuse the root Record only" experiment confirms this: reusing `&opencdc.v2.Record{}` across calls
saved 6 of 88 allocations and no meaningful time, because the nested tree is still built fresh.

### C. A custom `proto.Message` wrapper carrying pre-marshaled bytes (rejected)

Implement `proto.Message` over a type that holds `Record.MarshalV2`'s bytes and returns them from
`Marshal`, so the protocol could embed it without schema changes.

Rejected because the `connectorv3` messages are generated and their `Records` field is typed
`[]*opencdc.v2.Record`; there is no way to substitute a wrapper without changing the connector
protocol schema (out of scope for this task, and a separate breaking-change discussion). The direct
encoder is the enabling step for that follow-up, not a substitute for it.

## Decision

Ship alternative A.

- `Record.MarshalV2() ([]byte, error)` returns a freshly allocated, caller-owned buffer.
- `Record.AppendV2(dst []byte) ([]byte, error)` appends to a caller-owned buffer and reuses it, so a
  stream of records allocates nothing per record.
- The encoder reuses `classifyJSONNumberV2` (so `int64`/`uint64`/`bytes`/`decimal` stay lossless and
  type-exact), the `pathStack` (so error paths render identically), and the `wrapProtoError` /
  `prefixNumberErrorPath` / `wrapStructuredDataError` helpers (so errors are identical to `ToProtoV2`).
- Output is deterministic (map keys sorted), byte-identical to
  `proto.MarshalOptions{Deterministic: true}` on the `ToProtoV2` tree. Determinism is a strict
  subset of correctness: any decoder accepts it, and it makes the byte-identity property test exact.

The existing `ToProtoV2` path is untouched; it remains the typed-tree API the connector protocol uses
until it adopts this encoder.

## Failure modes

- **Wire-format drift.** A hand-written encoder can disagree with protobuf on a field number, tag,
  varint, or map-entry encoding. Mitigation: `TestRecord_MarshalV2_ByteIdentical` compares
  `MarshalV2` byte-for-byte to `proto.MarshalOptions{Deterministic: true}(ToProtoV2(r))` across every
  value kind and edge case (empty messages, `RawData(nil)`, `StructuredData(nil)`, empty map
  key/value, NaN/±Inf, nested maps and arrays), and `TestRecord_MarshalV2_PropertyByteIdentical`
  property-tests it over random nested records. Any field-number or type regression fails these.
- **Allocation bugs in the pooled encoder.** The marshaler pool and its scratch slices could retain
  memory or alias across recursive calls. The per-depth key-sort scratch is indexed by `len(path)`
  so a nested map can never overwrite its parent's scratch; `sync.Pool` entries are reclaimed by GC
  between cycles, so any retained key strings are bounded and transient, not a leak. The benchmark
  asserts zero allocations in steady state, which fails if a per-record allocation sneaks back in.
- **Arena lifetime / stale references.** `AppendV2` returns a slice into the caller's `dst`; a caller
  that keeps one result while appending the next into the *same* buffer would observe the first slice
  mutate. This is the standard `append` contract and is documented; `MarshalV2` returns a fresh
  buffer so the default is safe.
- **Lossless semantics regress.** If a future edit re-boxes an integer through `float64` or parses a
  decimal, the value rounds or changes type. The round-trip test (marshal → `proto.Unmarshal` →
  `FromProtoV2`) plus the byte-identity tests pin `int64`/`uint64`/`bytes`/`decimal` to their exact
  wire encodings.
- **Determinism is a behavior change vs `proto.Marshal`.** The protocol today sends non-deterministic
  map order; `MarshalV2` sorts. No receiver compares raw bytes, and deterministic output is strictly
  more stable, but it is a documented difference from the non-deterministic `proto.Marshal`.

## Upgrade / rollback

`MarshalV2` and `AppendV2` are additive; nothing existing changes. Rollback is deleting the two
methods and the tests — no caller in this repo depends on them, and the wire format is unchanged, so
no migration or versioning is needed. The proto schema is untouched (`buf` output unchanged).
