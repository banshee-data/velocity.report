# Proto contract and debug overlays

- **Source plan:** [docs/plans/lidar-visualiser-proto-contract-and-debug-overlay-fixes-plan.md](../../plans/lidar-visualiser-proto-contract-and-debug-overlay-fixes-plan.md)

gRPC/protobuf contract parity for visualiser streaming, ensuring the proto schema matches actual serialisation.

## Problem statement

The visualiser protobuf schema (`visualiser.proto`) declares fields and controls that are not fully implemented in the gRPC stream path. This creates UI/runtime mismatch and weakens trust in the proto as a contract.

## Current state

### Track field serialisation: complete

All `Track` fields declared in `visualiser.proto` are now serialised by `frameBundleToProto()`, including:

- Covariance matrix (`covariance_4x4`)
- Feature fields (`height_p95_max`, `intensity_mean_avg`)
- Speed fields (`avg_speed_mps`, `max_speed_mps`)
- Quality fields (`class_confidence`, `track_length_metres`, `track_duration_secs`)
- Occlusion state (`occlusion_count`, `occlusion_state`)
- Background snapshot serialisation (M3.5)

Test: `TestFrameBundleToProto_TrackFieldCompleteness` asserts every Track field round-trips correctly.

### ObjectClass enum: complete

Proto field 26 is now `ObjectClass object_class` (typed enum, not string):

```
OBJECT_CLASS_UNSPECIFIED, NOISE, DYNAMIC, PEDESTRIAN, CYCLIST,
BIRD, BUS, CAR, TRUCK, MOTORCYCLIST
```

Conversion: `objectClassFromString()` maps canonical class strings to enum. `classifyOrConvert()` handles VRLOG backward compatibility (re-classifies from per-frame features for legacy recordings).

Tests: 6 dedicated tests including round-trip, empty-to-unspecified, and meta-test ensuring no `l6objects` constant is missed.

### Speed field direction

- Field 24: `avg_speed_mps` (running mean); stable.
- Raw maximum field renamed from `peak_speed_mps` to `max_speed_mps`.
- Aggregate-percentile labels are **not** on the `Track` proto. Percentile computation applies only to grouped/report surfaces.
- Name `peak_speed_mps` is reserved for a future filtered/context-aware top-speed metric.

## Remaining gaps

### Debug overlays (pending)

- `FrameAdapter.adaptDebugFrame()` builds `DebugOverlaySet` correctly.
- `frameBundleToProto()` does **not** yet map `frame.Debug` into `pb.FrameBundle.Debug`.
- Existing tests assert the broken behaviour (`Debug == nil`): these need replacement with positive serialisation tests.

### Overlay mode controls

- Decision recorded: `include_debug` gates server-side emission. `SetOverlayModes()` remains client-side/advisory only; no server-side subset filtering.
- `supports_debug=true` in capabilities should be treated as capability declaration, not per-overlay filtering support.

### Cluster field parity (pending)

Declared but not yet serialised:

- `Cluster.height_p95`
- `Cluster.intensity_mean`
- `Cluster.sample_points` (also needs adapter propagation from `l4perception.WorldCluster`)

## Deferred items

Tracked in BACKLOG.md:

- Debug overlay serialisation in `frameBundleToProto()`
- Positive integration serialisation tests replacing negative debug tests
- Cluster feature field serialisation
- `SeekToTimestamp()` diagnostic logging gated behind debug flag
- `SeekToTimestamp()` O(n) linear scan → O(log n) binary search with prebuilt sorted index

## Key files

| File                                                                                                    | Role                                              |
| ------------------------------------------------------------------------------------------------------- | ------------------------------------------------- |
| [proto/velocity_visualiser/v1/visualiser.proto](../../../proto/velocity_visualiser/v1/visualiser.proto) | Schema definition                                 |
| `internal/lidar/visualiser/grpc_server.go`                                                              | `frameBundleToProto()` serialiser                 |
| `internal/lidar/visualiser/adapter.go`                                                                  | Frame adapter (internal model → visualiser model) |
| `tools/visualiser-macos/.../VisualiserClient.swift`                                                     | Swift gRPC client                                 |
| `tools/visualiser-macos/.../ContentView.swift`                                                          | macOS UI bindings                                 |
