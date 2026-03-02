# LiDAR Visualiser Proto Contract and Debug Overlay Fixes Plan

**Status:** Complete — All Go-side phases (A, C, E) are done. Swift client updates (Phase D) and overlay mode behaviour (Phase B) remain as follow-up items.
**Scope:** gRPC/protobuf contract parity for visualiser streaming, debug overlays, and track speed summary fields before `v0.5.0`
**Related:** [`proto/velocity_visualiser/v1/visualiser.proto`](../../proto/velocity_visualiser/v1/visualiser.proto), [`internal/lidar/visualiser/grpc_server.go`](../../internal/lidar/visualiser/grpc_server.go), [`internal/lidar/visualiser/adapter.go`](../../internal/lidar/visualiser/adapter.go), [`tools/visualiser-macos/VelocityVisualiser/gRPC/VisualiserClient.swift`](../../tools/visualiser-macos/VelocityVisualiser/gRPC/VisualiserClient.swift), [`tools/visualiser-macos/VelocityVisualiser/UI/ContentView.swift`](../../tools/visualiser-macos/VelocityVisualiser/UI/ContentView.swift)

## 1. Problem

The visualiser protobuf schema advertises fields and controls that are not fully
implemented in the gRPC stream path:

1. `FrameBundle.debug` exists in protobuf but is not serialized from the Go
   visualiser server.
2. `StreamRequest.include_debug` and `SetOverlayModes(...)` are accepted but are
   not applied to streamed payloads.
3. ~~Several `Track` and `Cluster` fields are populated in the internal model but
   are dropped during protobuf serialization.~~ Track fields are now fully
   serialized; Cluster feature fields (`height_p95`, `intensity_mean`,
   `sample_points`) remain unserialised.
4. `Track.avg_speed_mps` (field `24`) does not match desired semantics for the
   visualiser inspector; median and high-percentile summaries are more useful.
5. `Track.class_label` (string) was replaced with `ObjectClass object_class`
   (enum, field `26`) with a 10-value enumeration. ✅ Implemented.

This creates UI/runtime mismatch (especially debug overlays) and weakens trust
in the proto as a contract.

## 2. Goals

1. Make protobuf stream output match the declared `visualiser.proto` contract.
2. Restore debug overlays end-to-end (adapter -> gRPC -> Swift client -> renderer).
3. Replace `Track.avg_speed_mps` with median semantics before `v0.5.0`.
4. Add `p85` and `p98` speed summary fields for visual review.
5. Add serialization tests that fail on future field drops.

## 3. Non-Goals

1. Redesigning debug overlay geometry/rendering in Metal.
2. Large UI workflow changes beyond inspector label/value updates.
3. Backward compatibility guarantees for pre-`v0.5.0` proto consumers.

## 4. Current Gaps (Observed)

### 4.1 Debug overlays

1. `FrameAdapter.adaptDebugFrame(...)` builds `DebugOverlaySet` correctly.
2. `frameBundleToProto(...)` does not map `frame.Debug` into `pb.FrameBundle.Debug`.
3. Existing tests explicitly assert the broken behavior (`Debug == nil`).

### 4.2 Overlay mode controls

1. `SetOverlayModes(...)` stores preferences only.
2. Stored preferences are not used during stream serialization/filtering.
3. `supports_debug=true` can mislead clients when debug payloads are absent.

### 4.3 Cluster field parity

Declared but currently not serialized in `frameBundleToProto(...)`:

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

**Resolved.** All Track fields declared in `visualiser.proto` are now serialized
by `frameBundleToProto(...)` in `grpc_server.go`. The original gap list and
current status:

1. ~~`Track.covariance_4x4`~~ — ✅ serialized (copied from `Covariance4x4` slice)
2. ~~`Track.height_p95_max`~~ — ✅ serialized
3. ~~`Track.intensity_mean_avg`~~ — ✅ serialized
4. ~~`Track.avg_speed_mps`~~ — ✅ renamed to `median_speed_mps` (field `24`); `p85_speed_mps` (36) and `p98_speed_mps` (37) added
5. ~~`Track.peak_speed_mps`~~ — ✅ serialized
6. ~~`Track.class_label`~~ — **Superseded.** Proto field `26` is now `ObjectClass object_class`
   (an `ObjectClass` enum, not a string). See [§4.5 ObjectClass enum](#45-objectclass-enum) below.
7. ~~`Track.class_confidence`~~ — ✅ serialized
8. ~~`Track.track_length_metres`~~ — ✅ serialized
9. ~~`Track.track_duration_secs`~~ — ✅ serialized
10. ~~`Track.occlusion_count`~~ — ✅ serialized
11. ~~`Track.occlusion_state`~~ — ✅ serialized (as `pb.OcclusionState` enum)

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

Change `Track` speed summary fields in `visualiser.proto`:

1. Replace field `24` from `avg_speed_mps` to `median_speed_mps`.
2. Keep `peak_speed_mps` on field `25`.
3. Add `p85_speed_mps` and `p98_speed_mps` as new fields (use new field numbers,
   do not renumber unrelated fields).

Note: field `26` was originally listed as `class_label` (string). It is now
`ObjectClass object_class` (enum). This change is already implemented and does
not affect the speed summary rename.

Rationale:

1. Median is more robust to noisy short-lived speed spikes than mean.
2. `p85` and `p98` match speed-review workflows already used elsewhere.
3. Avoiding broad tag churn limits accidental breakage even before `v0.5.0`.

### 5.2 Percentile computation

Current helper computes `p50`, `p85`, `p95`. This plan adds `p98` support.

Preferred approach:

1. Introduce a visualiser-oriented helper that computes `median/p85/p98` from
   track `speedHistory`.
2. Keep existing `p95` helper behavior where other subsystems still rely on it.
3. Document percentile indexing method (floor vs interpolation) in code/tests.

## 6. Implementation Plan

### Phase A: gRPC serializer parity (P0) ✅

1. ~~Update `frameBundleToProto(...)` to serialize `FrameBundle.debug` when
   `StreamRequest.include_debug=true`.~~ ✅ Complete — debug overlays serialised
   with all 4 sub-types (association, gating, residuals, predictions), gated by
   `req.IncludeDebug`.
2. ~~Serialize all currently-dropped `Cluster` fields that are available in the
   internal model (`height_p95`, `intensity_mean`; `sample_points` requires
   adapter propagation first).~~ ✅ Complete — `HeightP95`, `IntensityMean`, and
   `SamplePoints` are now copied in `frameBundleToProto`. `SamplePoints` is wired
   through but not populated upstream (l4perception does not fill it yet).
3. ~~Serialize all currently-dropped `Track` fields that are already populated in
   the internal model.~~ ✅ Complete — all Track fields are now serialized
   including `ObjectClass` enum via `classifyOrConvert()`.
4. ~~Add/expand tests for `FrameBundle.background`, `frame_type`, and
   `background_seq`.~~ ✅ Complete — background snapshot serialization implemented
   in `frameBundleToProto(...)` (M3.5).
5. ~~Add `ObjectClass` enum conversion with `objectClassFromString()` and
   `classifyOrConvert()` for VRLOG backward compatibility.~~ ✅ Complete.

### Phase B: Overlay mode behavior (P1)

1. Decide whether `SetOverlayModes(...)` should:
   - filter server payload emission, or
   - remain client-side only and be documented as advisory.
2. If server-side filtering is implemented, apply stored preferences in
   `frameBundleToProto(...)` / stream path for debug subsets.
3. If not implemented immediately, downgrade `supports_debug` claims or document
   capability granularity clearly.

### Phase C: Speed summary schema + mapping (P1) ✅

1. ~~Edit `proto/velocity_visualiser/v1/visualiser.proto`:~~
   - ~~`avg_speed_mps` -> `median_speed_mps` (field `24`)~~
   - ~~add `p85_speed_mps`~~
   - ~~add `p98_speed_mps`~~
2. ~~Regenerate protobuf code (Go and Swift generated bindings as applicable).~~
3. ~~Populate new fields from track `speedHistory`.~~
4. ~~Update adapter/model naming to keep semantics aligned.~~

### Phase D: Swift client/UI parity (P2) ✅

1. ~~Update Swift protobuf mapping for renamed/new track speed fields.~~
2. ~~Update inspector labels:~~
   - ~~`Average` -> `Median`~~
   - ~~add `p85`~~
   - ~~add `p98`~~
3. ~~Keep UI resilient when new fields are absent (temporary mixed-version runs).~~
4. ~~median/p85/p98 inspector rows re-added now that the server populates the
   fields and Swift proto has been regenerated.~~

### Phase E: Test hardening (P1) ✅

1. ~~Replace "debug not converted" tests with positive serialization tests.
   `TestFrameBundleToProto_DebugNotConverted` and
   `TestFrameBundleToProto_DebugFieldAbsent` currently assert `Debug == nil`;
   update these to assert non-nil debug output once serialization is implemented.~~
   ✅ Complete — replaced with `TestFrameBundleToProto_DebugSerialised` (positive
   round-trip), `TestFrameBundleToProto_DebugOmittedWhenNotRequested` (gating),
   and `TestFrameBundleToProto_DebugNilInFrame` (nil input).
2. ~~Add round-trip field assertions for:~~
   - ~~debug overlays (`association`, `gating`, `residuals`, `predictions`)~~ ✅
   - ~~cluster feature fields~~ ✅ `TestFrameBundleToProto_ClusterFeatureFields`
   - ~~track feature/classification/quality fields~~ ✅ `TestFrameBundleToProto_TrackFieldCompleteness`
   - ~~track speed summary fields (`median`, `peak`, `p85`, `p98`)~~ ✅ covered in TrackFieldCompleteness
3. ~~Add a regression test for `include_debug=false` to ensure payload omission is
   intentional and explicit.~~ ✅ `TestFrameBundleToProto_DebugOmittedWhenNotRequested`
4. ~~ObjectClass conversion tests~~ ✅ Comprehensive coverage in
   `object_class_conversion_test.go` and `VisualiserClientTests.swift`.

## 7. Acceptance Criteria

1. Enabling debug overlays in stream requests produces non-empty `FrameBundle.debug`
   when debug data exists upstream.
2. Swift visualiser receives and renders debug overlays without relying on local
   test-only stub data.
3. Track inspector shows `Median`, `Peak`, `p85`, and `p98` from streamed data.
4. Protobuf serializer tests cover all non-trivial `Track` and `Cluster` fields
   defined by the current schema.
5. `visualiser.proto` field semantics for speed summaries match UI labels.

## 8. Risks and Open Questions

1. Mixed-version client/server compatibility during local development:
   rename of field `24` changes semantics immediately.
2. Percentile method consistency:
   `p98` may differ slightly between floor-index and interpolated definitions.
3. Overlay mode scope:
   server-side filtering may be unnecessary if renderer-side toggles are already
   sufficient, but proto/API naming should then be clarified.

## 9. Task Checklist

- [ ] Add debug overlay protobuf serialization in `frameBundleToProto(...)`
- [ ] Gate debug serialization by `include_debug`
- [ ] Serialize missing `Cluster` feature fields (`height_p95`, `intensity_mean`, `sample_points`)
- [x] Serialize missing `Track` feature/classification/quality fields
- [x] Add `ObjectClass` enum to proto (9 classes + UNSPECIFIED) with `objectClassFromString()` / `classifyOrConvert()` conversion
- [x] Add ObjectClass conversion tests (`object_class_conversion_test.go`, `VisualiserClientTests.swift`)
- [x] Serialize background snapshot and frame type in `frameBundleToProto(...)` (M3.5)
- [x] Add `TestFrameBundleToProto_TrackFieldCompleteness` test covering all Track fields
- [x] Update proto field `24` to `median_speed_mps`
- [x] Add `p85_speed_mps` and `p98_speed_mps` to `Track`
- [x] Regenerate protobuf bindings (Go + Swift)
- [ ] Compute/populate median/p85/p98 from track speed history in `frameBundleToProto`
- [ ] Regenerate Swift protobuf from updated `.proto` (Swift generated code still has `avgSpeedMps`)
- [ ] Re-add median/p85/p98 inspector rows in `ContentView.swift` once server populates them
- [x] ~~Update Swift visualiser inspector labels and values~~ (rows removed — fields not yet populated)
- [ ] Replace negative debug tests with positive end-to-end serialization tests
