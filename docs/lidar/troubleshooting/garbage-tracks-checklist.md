# Garbage Tracks — Consolidated Status and Remaining Priorities

This is the canonical document for garbage-track remediation.
It combines the original review and checklist into one maintained source.

Updated: 2026-02-17

---

## Scope and context

- Reviewed layers: full LiDAR pipeline (foreground → transform → ground filter → clustering → tracking → persistence → rendering).
- Goal: remove trajectory contamination, avoid spaghetti artefacts, fix missing detections, and harden pipeline determinism.
- Canonical reference files:
  - [tracking_pipeline.go](../../../internal/lidar/tracking_pipeline.go)
  - [tracking.go](../../../internal/lidar/tracking.go)
  - [ground.go](../../../internal/lidar/ground.go)
  - [transform.go](../../../internal/lidar/transform.go)
  - [obb.go](../../../internal/lidar/obb.go)
  - [clustering.go](../../../internal/lidar/clustering.go)
  - [track_store.go](../../../internal/lidar/track_store.go)
  - [frame_builder.go](../../../internal/lidar/frame_builder.go)
  - [adapter.go](../../../internal/lidar/visualiser/adapter.go)
  - [track_api.go](../../../internal/lidar/monitor/track_api.go)
  - [MapPane.svelte](../../../web/src/lib/components/lidar/MapPane.svelte)
  - [api.ts](../../../web/src/lib/api.ts)

---

## Current state (implemented)

### Completed P0/P1 remediation

| Item                                   | Status  | Implemented behaviour                                                                                          |
| -------------------------------------- | ------- | -------------------------------------------------------------------------------------------------------------- |
| 1.1 globally unique track identity     | ✅ Done | `initTrack` now emits UUID-based IDs (`trk_<hex8>`), preventing reset/restart ID collisions.                   |
| 1.2 scoped observation/history queries | ✅ Done | `GetTracksInRange` and `GetActiveTracks` now use `GetTrackObservationsInRange` (time-bounded + sensor-scoped). |
| 2.1 dt clamp in predict                | ✅ Done | `predict()` clamps dt to `MaxPredictDt=0.5s`.                                                                  |
| 2.2 covariance inflation cap           | ✅ Done | covariance diagonal is capped (`MaxCovarianceDiag=100`) in predict and occlusion paths.                        |
| 4.1 race-safe confirmed snapshots      | ✅ Done | `GetConfirmedTracks()` returns deep-copied snapshots, not live pointers.                                       |
| 4.2 serialised frame callbacks         | ✅ Done | frame callback now uses a single worker + buffered channel in `FrameBuilder`.                                  |
| 5.1 observation envelope parsing       | ✅ Done | `getTrackObservations()` now returns `data.observations ?? []`.                                                |
| 6.1 polyline gap breaking              | ✅ Done | renderer breaks strokes on temporal (>1s) or spatial (>2m) gaps.                                               |

### Validation already completed

- `go test ./... -count=1` passed.
- `go test -race ./internal/lidar/ -count=1` passed.
- `make test-web` passed.

---

## Remaining backlog (reprioritised)

### P0 — Critical pipeline bugs (new findings 2026-02-16)

#### 8.1 Height band filter operates in sensor frame ~~(CRITICAL)~~ ✅ Done

- **Severity:** P0 — ~~causes loss of nearly all foreground detections~~ **Fixed**
- **Implemented behaviour:** `DefaultHeightBandFilter()` now returns floor=−2.8m, ceiling=+1.5m (sensor-frame coordinates). Documentation and comments updated to reflect sensor-frame semantics.

#### 8.2 OBB heading not sent to web REST API ~~— Svelte has no bounding boxes~~ ✅ Done

- **Severity:** P0 — **Fixed**
- **Implemented behaviour:** `trackToResponse()` now includes `obb_heading_rad` field sourced from `track.OBBHeadingRad`. `BBox` struct extended with per-frame `length`, `width`, `height` alongside existing `*_avg` fields. `MapPane.svelte` now uses `track.obb_heading_rad` (with `heading_rad` fallback) for bounding box rotation, and prefers per-frame dimensions over averages for rendering. TypeScript `Track` interface updated with `obb_heading_rad` and per-frame bbox fields.

#### 8.3 Per-frame OBB dimensions not persisted ~~— averaged dimensions used~~ ✅ Done

- **Severity:** P0/P1 — **Fixed**
- **Implemented behaviour:** `TrackedObject` now stores per-frame `OBBLength`, `OBBWidth`, `OBBHeight` fields updated every frame from the cluster OBB. Both gRPC adapter (`adaptTracks`) and REST API (`trackToResponse`) transmit per-frame and averaged dimensions. `InsertTrackObservation` in the tracking pipeline now persists per-frame OBB dimensions and OBB heading instead of running averages. `MapPane.svelte` prefers per-frame `bbox.length` over `bbox.length_avg` for rendering.

#### 8.4 OBB heading jitter in macOS view (PCA instability) ✅ Done

- **Severity:** Medium — macOS visualiser only — **Fixed**
- **Implemented behaviour:** Three guards added to `update()` in tracking.go:
  1. **Min-points threshold:** clusters with fewer than `MinPointsForPCA` (4) points skip heading update, retaining the previous smoothed heading.
  2. **Aspect-ratio lock:** when `|length − width| / max(length, width) < OBBAspectRatioLockThreshold` (0.25), the heading is locked because the principal axis is ambiguous.
  3. **Reduced smoothing α:** `OBBHeadingSmoothingAlpha` lowered from 0.15 to 0.08 for heavier EMA smoothing.
     Per-frame OBB dimensions are always updated regardless of heading lock.

### R1 — Next high-impact items

#### 2.5 Coasted points persisted as real observations ✅ Done

- **Severity:** Medium — **Fixed**
- **Implemented behaviour:** The persistence loop in `tracking_pipeline.go` now checks `track.Misses == 0` before calling `InsertTrackObservation`. Coasting tracks (Misses > 0) still have their track record updated via `InsertTrack`, but predicted Kalman positions are no longer persisted as observations. This eliminates phantom straight segments from coasted positions.

#### 6.4 Full-epoch default query window ✅ Done

- **Severity:** Medium — **Fixed**
- **Implemented behaviour:** `loadHistoricalData()` in `+page.svelte` now defaults `startTime` to `(Date.now() - 3_600_000) * 1e6` (last 1 hour in nanoseconds) instead of epoch (0). This bounds the initial query to recent data, reducing load time and eliminating exposure to old artefacts.

#### 2.4 NaN/Inf guards after Kalman predict/update ✅ Done

- **Severity:** Medium — **Fixed**
- **Implemented behaviour:** `isFiniteState()` helper checks X, Y, VX, VY and covariance diagonal for NaN/Inf. Called at the end of both `predict()` and `update()` in tracking.go. If any value is non-finite, the state is reset to defaults and the track is marked `TrackDeleted` to prevent corruption from propagating.

#### 2.3 Velocity clamp on Kalman state ✅ Done

- **Severity:** Medium — **Fixed**
- **Implemented behaviour:** `clampVelocity()` helper scales VX/VY proportionally so speed magnitude never exceeds `MaxReasonableSpeedMps` (30 m/s ≈ 108 km/h). Called at the end of both `predict()` and `update()` in tracking.go. This prevents teleport-like extrapolation from noisy Kalman updates.

### R2 — Medium-term hardening

#### 3.1 Cluster size/aspect filtering ✅ Done

- **Severity:** Medium
- **Files:** [clustering.go](../../../internal/lidar/clustering.go)
- **Implemented behaviour:** `buildClusters()` now rejects clusters with longest OBB dimension > `MaxClusterDiameter` (12 m) or < `MinClusterDiameter` (0.05 m). Aspect ratio check (longest/shortest > `MaxClusterAspectRatio` = 15) is only applied when the shortest axis exceeds 0.03 m to avoid rejecting legitimate edge-on detections.

#### 3.3 Merge/split temporal coherence ✅ Done

- **Severity:** Medium
- **Files:** [tracking.go](../../../internal/lidar/tracking.go)
- **Implemented behaviour:** After Hungarian association, Step 3b compares each matched cluster's area to the track's historical average OBB area. `MergeCandidate` is set when the cluster area exceeds 2.5× the average (indicating two objects merged), `SplitCandidate` is set when below 0.3× (indicating an object split). These advisory flags are available on `TrackedObject` for downstream filtering.

#### 4.3 Classification mutation locking ✅ Done

- **Severity:** Medium
- **Files:** [tracking.go](../../../internal/lidar/tracking.go), [tracker_interface.go](../../../internal/lidar/tracker_interface.go), [tracking_pipeline.go](../../../internal/lidar/tracking_pipeline.go)
- **Implemented behaviour:** `UpdateClassification()` method added to `TrackerInterface` and `Tracker`. After `ClassifyAndUpdate()` runs on a snapshot, the pipeline calls `cfg.Tracker.UpdateClassification()` to write the classification result back to the live track under the tracker's mutex. This prevents the previous bug where classification ran on snapshot copies that were never propagated to in-memory state.

#### 5.2 Run filter robustness ✅ Done

- **Severity:** Medium
- **Files:** [+page.svelte](../../../web/src/routes/lidar/tracks/+page.svelte)
- **Implemented behaviour:** `runTrackIds: Set<string>` replaced with `runTrackMap: Map<string, RunTrack>`. `visibleTracks` filter now uses each `RunTrack`'s `start_unix_nanos`/`end_unix_nanos` for time scoping instead of the track's `first_seen`/`last_seen`. This prevents false positives from global ID membership where tracks straddle multiple runs.

#### 7.1 Throttle-related dt spikes ✅ Done

- **Severity:** Medium
- **Files:** [tracking.go](../../../internal/lidar/tracking.go)
- **Implemented behaviour:** In `Update()`, `dt` is clamped to `MaxPredictDt` (0.5 s) before `LastUpdateNanos` is set. This prevents throttle-induced gaps from inflating the dt used for association gating's implied-speed check.

### R3 — UX/polish and housekeeping

#### 1.3 Deleted-track DB pruning ✅ Done

- **Severity:** Medium
- **Files:** [track_store.go](../../../internal/lidar/track_store.go), [tracking_pipeline.go](../../../internal/lidar/tracking_pipeline.go)
- **Implemented behaviour:** `PruneDeletedTracks(db, sensorID, ttl)` function deletes tracks in `state='deleted'` whose end timestamp is older than the TTL (5 minutes), along with their observations, in a single transaction. Called once per minute from the pipeline callback to prevent unbounded storage growth.

#### 6.2 Per-track colour differentiation within class ✅ Done

- **Severity:** Medium
- **Files:** [lidar.ts](../../../web/src/lib/types/lidar.ts), [MapPane.svelte](../../../web/src/lib/components/lidar/MapPane.svelte)
- **Implemented behaviour:** `trackColour(trackId, objectClass, state)` utility function computes a deterministic hue-shifted colour for each track ID (±25° around the class base hue) using a string hash → HSL conversion. `MapPane.svelte`'s `renderTrack()` now uses `trackColour()` instead of the static `TRACK_COLORS` lookup, making same-class tracks visually distinguishable.

#### 6.3 Temporal fade on trails ✅ Done

- **Severity:** Low
- **Files:** [MapPane.svelte](../../../web/src/lib/components/lidar/MapPane.svelte)
- **Implemented behaviour:** Trail rendering replaced from a single polyline stroke at fixed `globalAlpha=0.5` to per-segment drawing with age-based alpha interpolation. Oldest visible segments render at α=0.1, newest at α=0.8, linearly interpolated across the trail's time span.

#### 6.5 Foreground observation sampling bias ✅ Done

- **Severity:** Low
- **Files:** [+page.svelte](../../../web/src/routes/lidar/tracks/+page.svelte)
- **Implemented behaviour:** `loadForegroundObservations()` now scopes the database query to a ±30-second window around the current `selectedTime` instead of the full time range. A reactive reload triggers when playback drifts more than 20 seconds from the last query centre, ensuring observation density stays representative of the visible viewport.

#### 7.2 Miss accounting on throttled frames ✅ Done

- **Severity:** Low
- **Files:** [tracking.go](../../../internal/lidar/tracking.go), [tracker_interface.go](../../../internal/lidar/tracker_interface.go), [tracking_pipeline.go](../../../internal/lidar/tracking_pipeline.go)
- **Implemented behaviour:** `AdvanceMisses(timestamp)` method added to `TrackerInterface` and `Tracker`. Called in the pipeline's throttle path before `return`, incrementing `Misses` and resetting `Hits` for all active tracks. Tracks exceeding their miss budget are deleted. This prevents tracks from being artificially kept alive when frames are skipped.

#### 3.2 Non-convex centroid stability ✅ Done

- **Severity:** Low
- **Files:** [clustering.go](../../../internal/lidar/clustering.go)
- **Implemented behaviour:** `computeClusterMetrics()` now uses the medoid (the actual cluster point closest to the arithmetic mean) instead of the arithmetic mean for the centroid. This ensures the centroid always lies on a real measurement point, preventing association instability for non-convex clusters (L-shapes, arcs).

---

## Delivery order

1. **P0 batch:** 8.1 (height filter — critical, most impact), 8.2 (OBB heading to web API), 8.3 (per-frame OBB dims) ✅
2. **R1 batch + 8.4:** 8.4, 2.5, 6.4, 2.4, 2.3 ✅
3. **R2 batch:** 3.1, 3.3, 4.3, 5.2, 7.1 ✅
4. **R3 batch:** 1.3, 6.2, 6.3, 6.5, 7.2, 3.2 ✅

---

## Validation protocol for remaining work

After each batch:

1. `make format`
2. `make lint`
3. `make test`
4. `go test -race ./internal/lidar/... -count=3`
5. Manual check in tracks UI for long diagonal artefacts and run filtering correctness
