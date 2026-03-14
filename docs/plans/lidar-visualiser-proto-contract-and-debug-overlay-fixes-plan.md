# LiDAR Visualiser Proto Contract and Debug Overlay Fixes Plan

**Status:** Partially implemented — Track field parity, ObjectClass enum, and `peak` → `max` rename are complete; debug overlay serialisation, cluster proto serialisation, and positive end-to-end serialiser tests remain
**Layers:** L9 Endpoints
**Scope:** gRPC/protobuf contract parity for visualiser streaming, debug overlays, and track speed summary fields before `v0.5.0`
**Related:** [`proto/velocity_visualiser/v1/visualiser.proto`](../../proto/velocity_visualiser/v1/visualiser.proto), [`internal/lidar/visualiser/grpc_server.go`](../../internal/lidar/visualiser/grpc_server.go), [`internal/lidar/visualiser/adapter.go`](../../internal/lidar/visualiser/adapter.go), [`tools/visualiser-macos/VelocityVisualiser/gRPC/VisualiserClient.swift`](../../tools/visualiser-macos/VelocityVisualiser/gRPC/VisualiserClient.swift), [`tools/visualiser-macos/VelocityVisualiser/UI/ContentView.swift`](../../tools/visualiser-macos/VelocityVisualiser/UI/ContentView.swift)

**Update (March 8, 2026):** The speed-summary portion of this plan is
superseded. Track-level aggregate-percentile labels will not ship. Percentiles are
reserved for grouped/report aggregates only, and any branch-local proto/model/UI
work that adds superseded single-track speed-label fields should be backed out before merge.

## 1. Problem

The visualiser protobuf schema advertises fields and controls that are not fully
implemented in the gRPC stream path:

1. `FrameBundle.debug` exists in protobuf but is not serialised from the Go
   visualiser server.
2. `StreamRequest.include_debug` and `SetOverlayModes(...)` are accepted but are
   not applied to streamed payloads.
3. ~~Several `Track` and `Cluster` fields are populated in the internal model but
   are dropped during protobuf serialisation.~~ Track fields are now fully
   serialised; Cluster feature fields (`height_p95`, `intensity_mean`,
   `sample_points`) remain unserialised.
4. The branch-local `Track` speed-summary expansion moved in the wrong
   direction. Aggregate percentile labels should not be added to the public
   proto contract; track speed metrics need a separate redesign with distinct
   non-percentile names.
5. `Track.class_label` (string) was replaced with `ObjectClass object_class`
   (enum, field `26`) with a 10-value enumeration. ✅ Implemented.

This creates UI/runtime mismatch (especially debug overlays) and weakens trust
in the proto as a contract.

## 2. Goals

1. Make protobuf stream output match the declared `visualiser.proto` contract.
2. Restore debug overlays end-to-end (adapter -> gRPC -> Swift client -> renderer).
3. Keep track-level speed fields limited to a stable non-percentile contract
   while the redesign is pending.
4. Do not ship track-level aggregate-percentile label additions from this branch.
5. Add serialisation tests that fail on future field drops.

## 3. Non-Goals

1. Redesigning debug overlay geometry/rendering in Metal.
2. Large UI workflow changes beyond inspector label/value updates.
3. Backward compatibility guarantees for pre-`v0.5.0` proto consumers.

## 4. Current Gaps (Observed)

### 4.1 Debug overlays

1. `FrameAdapter.adaptDebugFrame(...)` builds `DebugOverlaySet` correctly.
2. `frameBundleToProto(...)` does not map `frame.Debug` into `pb.FrameBundle.Debug`.
3. Existing tests explicitly assert the broken behaviour (`Debug == nil`).

### 4.2 Overlay mode controls

1. `SetOverlayModes(...)` stores preferences only.
2. Stored preferences are not used during stream serialisation/filtering.
3. `supports_debug=true` can mislead clients when debug payloads are absent.

### 4.3 Cluster field parity

Declared but currently not serialised in `frameBundleToProto(...)`:

1. `Cluster.height_p95`
2. `Cluster.intensity_mean`
3. `Cluster.sample_points`

Notes:

1. `height_p95` and `intensity_mean` already exist in the internal model **and**
   are populated by `FrameAdapter.adaptClusters(...)`, but `frameBundleToProto`
   does not copy them into the proto `Cluster` message.
2. `sample_points` is declared in the proto and the visualiser model
   (`Cluster.SamplePoints`) but is not currently propagated from
   `l4perception.WorldCluster.SamplePoints` into the adapter output.

### 4.4 Track field parity

**Resolved.** All Track fields declared in `visualiser.proto` are now serialised
by `frameBundleToProto(...)` in `grpc_server.go`. The original gap list and
current status:

1. ~~`Track.covariance_4x4`~~ — ✅ serialised (copied from `Covariance4x4` slice)
2. ~~`Track.height_p95_max`~~ — ✅ serialised
3. ~~`Track.intensity_mean_avg`~~ — ✅ serialised
4. ~~`Track` speed summary fields~~ — Branch-local serialisation exists for the
   superseded percentile-field direction, but that contract reset still needs
   to be backed out before merge. The stable merge-target direction remains
   `avg_speed_mps` plus the raw maximum field for now.
5. ~~`Track.peak_speed_mps`~~ — ✅ serialised
6. ~~`Track.class_label`~~ — **Superseded.** Proto field `26` is now `ObjectClass object_class`
   (an `ObjectClass` enum, not a string). See [§4.5 ObjectClass enum](#45-objectclass-enum) below.
7. ~~`Track.class_confidence`~~ — ✅ serialised
8. ~~`Track.track_length_metres`~~ — ✅ serialised
9. ~~`Track.track_duration_secs`~~ — ✅ serialised
10. ~~`Track.occlusion_count`~~ — ✅ serialised
11. ~~`Track.occlusion_state`~~ — ✅ serialised (as `pb.OcclusionState` enum)

Test coverage: `TestFrameBundleToProto_TrackFieldCompleteness` in
`grpc_server_test.go` asserts every Track field round-trips correctly.

### 4.5 ObjectClass enum

The original plan referenced `Track.class_label` (a string field). The proto now
defines a typed `ObjectClass` enum on field `26`:

```protobuf
enum ObjectClass {
  OBJECT_CLASS_UNSPECIFIED  = 0;
  OBJECT_CLASS_NOISE        = 1;
  OBJECT_CLASS_DYNAMIC      = 2;
  OBJECT_CLASS_PEDESTRIAN   = 3;
  OBJECT_CLASS_CYCLIST      = 4;
  OBJECT_CLASS_BIRD         = 5;
  OBJECT_CLASS_BUS          = 6;
  OBJECT_CLASS_CAR          = 7;
  OBJECT_CLASS_TRUCK        = 8;
  OBJECT_CLASS_MOTORCYCLIST = 9;
}
```

Conversion is handled by two functions in `grpc_server.go`:

- `objectClassFromString(s string) pb.ObjectClass` — maps canonical class strings
  (e.g. `"car"`, `"pedestrian"`) to the proto enum.
- `classifyOrConvert(t Track) pb.ObjectClass` — returns the stored class if
  present; for VRLOG recordings that pre-date classification, re-classifies from
  per-frame features as a fallback.

The Swift client converts the proto enum back to display strings via a private
`objectClassLabel(...)` function in `VisualiserClient.swift`.

Test coverage:

- `TestObjectClassFromString` — all 9 classes + unknown → UNSPECIFIED.
- `TestTrackObjectClassPropagation` — `l6objects.Class*` constants round-trip correctly.
- `TestObjectClassRoundtrip` — string → enum → proto name → verify no loss.
- `TestEmptyObjectClassToUnspecified` — empty/uninitialised → UNSPECIFIED.
- `TestAllObjectClassConstantsConvertible` — meta-test ensures no `l6objects` constant is missed.
- `TestObjectClassConversionInProtoMessages` — full proto message round-trip.
- Swift: `ObjectClassConversionTests` in `VisualiserClientTests.swift`.

## 5. Protocol Change Direction (Pre-`v0.5.0`)

### 5.1 Track speed summary fields

Revised direction for `Track` speed fields in `visualiser.proto`:

1. Keep field `24` as `avg_speed_mps` (running mean) for now.
2. Rename the current raw `peak_speed_mps` field on `Track` to `max_speed_mps`
   before merge if the contract is still unshipped.
3. Do **not** ship single-track aggregate-percentile label additions in the merge target. If
   fields `36-38` exist on this branch, they should be backed out before merge.
4. Reserve the name `peak_speed_mps` for a future filtered/context-aware
   top-speed metric if that measure is later added on a new field number.
5. Define any replacement track-level speed fields in a separate redesign, with
   names that are distinct from report/group percentiles.

### 5.2 Aggregate-only percentile rule

Percentile computation still applies to grouped/report surfaces, but not to the
`Track` message itself. Any future track metric redesign must use distinct
terminology. The branch-local `speedPercentiles()` helper and related bindings
should not be treated as the merge target for the visualiser contract.

## 6. Implementation Plan

### Phase A: gRPC serialiser parity (P0)

1. Update `frameBundleToProto(...)` to serialise `FrameBundle.debug` when
   `StreamRequest.include_debug=true`.
2. Serialize all currently-dropped `Cluster` fields that are available in the
   internal model (`height_p95`, `intensity_mean`; `sample_points` requires
   adapter propagation first).
3. ~~Serialize all currently-dropped `Track` fields that are already populated in
   the internal model.~~ ✅ Complete — all Track fields are now serialised
   including `ObjectClass` enum via `classifyOrConvert()`.
4. ~~Add/expand tests for `FrameBundle.background`, `frame_type`, and
   `background_seq`.~~ ✅ Complete — background snapshot serialisation implemented
   in `frameBundleToProto(...)` (M3.5).
5. ~~Add `ObjectClass` enum conversion with `objectClassFromString()` and
   `classifyOrConvert()` for VRLOG backward compatibility.~~ ✅ Complete.

### Phase B: Overlay mode behaviour (P1)

Decision recorded in [DECISIONS.md](../DECISIONS.md): `include_debug` gates
whether debug payloads are emitted by the server. `SetOverlayModes(...)`
remains client-side/advisory and does not drive server-side subset filtering.

1. Gate `FrameBundle.debug` serialisation strictly on
   `StreamRequest.include_debug`.
2. Do not apply stored overlay preferences in `frameBundleToProto(...)` or the
   stream path; the client is responsible for filtering/rendering overlay
   subsets locally.
3. Document `supports_debug` as stream-level capability, not per-overlay
   server-side filtering support.

### Phase C: Track speed summary schema — Retired

**Retired:** This scope is now tracked in [BACKLOG.md](../BACKLOG.md) v0.5.0
under "Visualiser track proto parity". The proto rename, percentile back-out, and
binding regeneration are owned by that backlog item.

### Phase D: Swift client/UI parity — Retired

**Retired:** Swift-side `peak` → `max` rename and inspector label updates are
now tracked in [BACKLOG.md](../BACKLOG.md) v0.5.0 under "Visualiser track proto
parity".

### Phase E: Test hardening (P1)

1. Replace "debug not converted" tests with positive serialisation tests.
   `TestFrameBundleToProto_DebugNotConverted` and
   `TestFrameBundleToProto_DebugFieldAbsent` currently assert `Debug == nil`;
   update these to assert non-nil debug output once serialisation is implemented.
2. Add round-trip field assertions for:
   - debug overlays (`association`, `gating`, `residuals`, `predictions`)
   - cluster feature fields
   - ~~track feature/classification/quality fields~~ ✅ `TestFrameBundleToProto_TrackFieldCompleteness`
   - merge-target track speed summary fields (`avg_speed_mps` plus the raw maximum field)
3. Add a regression test for `include_debug=false` to ensure payload omission is
   intentional and explicit.
4. ~~ObjectClass conversion tests~~ ✅ Comprehensive coverage in
   `object_class_conversion_test.go` and `VisualiserClientTests.swift`.

### Phase E.1: SeekToTimestamp diagnostic logging behind debug flag (#381)

`SeekToTimestamp()` currently logs index entry dumps and seek diagnostics
unconditionally on every call. This should be gated behind the existing
`include_debug` stream flag or a replayer-level debug toggle, to avoid
noisy logs during normal VRLOG playback.

- Guard index dump loop (`index[%d] ts=...`) behind a debug flag
- Guard seek result log (`landed on index...`) behind debug flag
- The existing `showDebug` / `include_debug` mechanism in gRPC streams
  already provides the right toggle; thread it through to the replayer

### Phase E.2: VRLOG timestamp index optimisation (#381)

Replace the O(n) linear scan in `SeekToTimestamp()` with a prebuilt sorted
timestamp index at `NewReplayer` load time:

1. At `NewReplayer()`, build a `[]int` sorted by `TimestampNs` (secondary sort
   by index position for stability)
2. Use `sort.Search()` for O(log n) binary search on seek
3. Add a loading/indexing state to the macOS UI (spinner) while the index
   is being built — visible when loading large VRLOGs

## 7. Acceptance Criteria

1. Enabling debug overlays in stream requests produces non-empty `FrameBundle.debug`
   when debug data exists upstream.
2. Swift visualiser receives and renders debug overlays without relying on local
   test-only stub data.
3. Track inspector shows the stable non-percentile track speed fields from
   streamed data and does not standardise on aggregate percentile labels for a single track.
4. Protobuf serialiser tests cover all non-trivial `Track` and `Cluster` fields
   defined by the current schema.
5. `visualiser.proto` field semantics for speed summaries match UI labels.

## 8. Risks and Open Questions

1. Mixed-version client/server compatibility during local development:
   backing out the branch-local speed-summary expansion can temporarily leave
   generated clients or local UI code out of sync until proto bindings are
   regenerated together.
2. Percentile method consistency:
   `p98` may differ slightly between floor-index and interpolated definitions.
3. Capability wording:
   `supports_debug` and overlay-mode docs must make the client-side/advisory
   behaviour explicit to avoid implying server-side subset filtering.

## 9. Task Checklist

- [ ] Add debug overlay protobuf serialisation in `frameBundleToProto(...)`
- [ ] Gate debug serialisation by `include_debug`
- [ ] Serialize missing `Cluster` feature fields (`height_p95`, `intensity_mean`, `sample_points`)
- [x] Serialize missing `Track` feature/classification/quality fields
- [x] Add `ObjectClass` enum to proto (9 classes + UNSPECIFIED) with `objectClassFromString()` / `classifyOrConvert()` conversion
- [x] Add ObjectClass conversion tests (`object_class_conversion_test.go`, `VisualiserClientTests.swift`)
- [x] Serialize background snapshot and frame type in `frameBundleToProto(...)` (M3.5)
- [x] Add `TestFrameBundleToProto_TrackFieldCompleteness` test covering all Track fields
- ~~Back out the branch-local track speed-summary field expansion before merge~~ — retired to [BACKLOG.md](../BACKLOG.md) v0.5.0
- ~~Regenerate protobuf bindings (Go + Swift) after removing superseded percentile-style track fields~~ — retired to [BACKLOG.md](../BACKLOG.md) v0.5.0
- ~~Remove branch-local percentile-style track computation and propagation from the merge-target contract work~~ — retired to [BACKLOG.md](../BACKLOG.md) v0.5.0
- ~~Update Swift visualiser inspector labels and values to the stable non-percentile track speed fields~~ — retired to [BACKLOG.md](../BACKLOG.md) v0.5.0
- [ ] Replace negative debug tests with positive end-to-end serialisation tests
