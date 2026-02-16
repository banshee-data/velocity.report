# Garbage Tracks — Consolidated Status and Remaining Priorities

This is the canonical document for garbage-track remediation.
It combines the original review and checklist into one maintained source.

Updated: 2026-02-16

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

#### 8.1 Height band filter operates in sensor frame (CRITICAL)

- **Severity:** P0 — causes loss of nearly all foreground detections
- **Symptom:** Many foreground cars not identified; objects below the sensor rejected.
- **Root cause:** `DefaultHeightBandFilter(floor=0.2, ceiling=3.0)` assumes Z=0 is ground level. However, `TransformToWorld` is called with a nil pose (identity transform), so Z=0 is the sensor's horizontal plane (~3m above ground). All LiDAR beams pointing below horizontal produce negative Z values via `z = distance × sin(elevation)`. The floor=0.2m check rejects every point with Z < 0.2, which includes **all objects at or below sensor height** — i.e. virtually everything of interest (cars, pedestrians, cyclists).
- **Numerical proof:** Pandar40P elevation channels range from approximately −25° to +15°. At 10m range with −5° elevation: Z = −0.87m → rejected. At 20m with −10°: Z = −3.47m → rejected. Only the few channels pointing above horizontal (+5° to +15°) at short range pass the filter, and those see sky/walls, not road-level objects.
- **Files:** [ground.go](../../../internal/lidar/ground.go), [tracking_pipeline.go](../../../internal/lidar/tracking_pipeline.go) (line 247–253), [transform.go](../../../internal/lidar/transform.go)
- **Fix options:**
  1. **(Preferred)** Adjust the height band to sensor-frame coordinates: set floor ≈ −3.5m (ground surface relative to sensor) and ceiling ≈ 0.3m (nothing of interest above sensor height). This correctly passes objects between ground and sensor.
  2. Supply a real sensor→world pose to `TransformToWorld` so Z=0 becomes ground level, then the existing [0.2, 3.0] band would work as designed.
  3. Disable the height filter entirely and rely on DBSCAN clustering to reject ground-plane noise.
- **Validation:** replay a pcap capture; confirm the foreground cluster count is dramatically higher after the fix. Compare before/after in the macOS visualiser.

#### 8.2 OBB heading not sent to web REST API — Svelte has no bounding boxes

- **Severity:** P0 — Svelte tracks view shows **no bounding boxes** at all
- **Symptom:** Bounding boxes completely absent in the Svelte tracks view. (Note: the _macOS_ visualiser does show boxes but they rotate/split — that is a separate PCA instability issue, see 8.4.)
- **Root cause:** `trackToResponse()` in [track_api.go](../../../internal/lidar/monitor/track_api.go) computes heading via `headingFromVelocity(velX, velY)` → `atan2(VY, VX)`. It does **not** include the PCA-derived `OBBHeadingRad`. The gRPC path (macOS app) correctly sends `BBoxHeadingRad: t.OBBHeadingRad` via [adapter.go](../../../internal/lidar/visualiser/adapter.go). Svelte's [MapPane.svelte](../../../web/src/lib/components/lidar/MapPane.svelte) uses `track.heading_rad` for rotation — which is velocity-derived and potentially zero/NaN for stationary objects.
- **Files:** [track_api.go](../../../internal/lidar/monitor/track_api.go) (line ~940), [MapPane.svelte](../../../web/src/lib/components/lidar/MapPane.svelte) (line ~581)
- **Fix:** Add `obb_heading_rad` field to the REST API track response. In `MapPane.svelte`, prefer `obb_heading_rad` over velocity heading for bounding box rendering.
- **Validation:** confirm bounding boxes appear in the Svelte tracks view with correct orientation matching macOS.

#### 8.3 Per-frame OBB dimensions not persisted — averaged dimensions used

- **Severity:** P0/P1 — bounding boxes use stale running averages, not per-frame measurements
- **Symptom:** Boxes in both macOS and Svelte views don't tightly hug clusters; they lag behind shape changes.
- **Root cause:** `EstimateOBBFromCluster` computes per-frame length/width/height but these values are folded into `BoundingBoxLengthAvg/WidthAvg/HeightAvg` running averages in the tracker. Both gRPC (`adaptTracks`) and REST API (`trackToResponse`) send only these averaged values. Persistence in `InsertTrackObservation` also stores averages.
- **Files:** [tracking_pipeline.go](../../../internal/lidar/tracking_pipeline.go) (line ~369), [tracking.go](../../../internal/lidar/tracking.go) (update method), [track_store.go](../../../internal/lidar/track_store.go)
- **Fix:** Persist and transmit both instantaneous OBB dimensions (for real-time rendering) and averaged dimensions (for classification/reporting). Add `obb_length`, `obb_width`, `obb_height` fields alongside the existing `*_avg` fields.
- **Validation:** bounding boxes should visually snap to cluster shape each frame.

#### 8.4 OBB heading jitter in macOS view (PCA instability)

- **Severity:** Medium — macOS visualiser only
- **Symptom:** Bounding boxes in the macOS visualiser rotate rapidly and sometimes split into smaller boxes on symmetric or small clusters.
- **Root cause:** PCA on small/symmetric point clouds has a 180° heading ambiguity. The velocity-based disambiguation (in `SmoothOBBHeading`) only works when speed > 0.5 m/s. The smoothing factor α=0.15 may be too aggressive for noisy heading estimates.
- **Files:** [obb.go](../../../internal/lidar/obb.go), [tracking.go](../../../internal/lidar/tracking.go)
- **Fix:** Increase heading smoothing (reduce α), add a minimum-points threshold for PCA, and consider locking heading when the cluster aspect ratio is near 1:1 (ambiguous).
- **Validation:** stationary or slow-moving vehicles should maintain stable box orientation.

### R1 — Next high-impact items

#### 2.5 Coasted points persisted as real observations

- **Severity:** Medium
- **Why next:** still creates phantom straight segments and can contaminate quality metrics.
- **Files:** [tracking.go](../../../internal/lidar/tracking.go), [tracking_pipeline.go](../../../internal/lidar/tracking_pipeline.go)
- **Fix:** either skip persisting coasted points or persist with explicit `is_predicted` flag.
- **Validation:** coast for N frames and verify predicted points are absent or explicitly flagged.

#### 6.4 Full-epoch default query window

- **Severity:** Medium
- **Why next:** maximally exposes historical artefacts and increases UI clutter/load.
- **Files:** [+page.svelte](../../../web/src/routes/lidar/tracks/+page.svelte)
- **Fix:** default to bounded window (for example last 1 hour) and expand explicitly.
- **Validation:** initial load query must not use epoch-to-now.

#### 2.4 NaN/Inf guards after Kalman predict/update

- **Severity:** Medium
- **Why next:** protects against singular/inversion edge cases causing permanent track corruption.
- **Files:** [tracking.go](../../../internal/lidar/tracking.go)
- **Fix:** add finite checks and reinitialise/delete on invalid state.
- **Validation:** inject near-singular covariance and assert no NaN/Inf escapes.

#### 2.3 Velocity clamp on Kalman state

- **Severity:** Medium
- **Why next:** reduces teleport-like extrapolation from noisy updates.
- **Files:** [tracking.go](../../../internal/lidar/tracking.go)
- **Fix:** clamp VX/VY (or speed magnitude) to configurable limit.
- **Validation:** outlier update should not push speed beyond cap.

### R2 — Medium-term hardening

#### 3.1 Cluster size/aspect filtering

- **Severity:** Medium
- **Files:** [clustering.go](../../../internal/lidar/clustering.go)
- **Fix:** reject extreme-diameter and extreme-aspect clusters.

#### 3.3 Merge/split temporal coherence

- **Severity:** Medium
- **Files:** [clustering.go](../../../internal/lidar/clustering.go), [tracking_pipeline.go](../../../internal/lidar/tracking_pipeline.go)
- **Fix:** add merge/split continuity heuristics to preserve identities.

#### 4.3 Classification mutation locking

- **Severity:** Medium
- **Files:** [tracking_pipeline.go](../../../internal/lidar/tracking_pipeline.go)
- **Fix:** classify under tracker lock or through protected write path.

#### 5.2 Run filter robustness

- **Severity:** Medium
- **Files:** [+page.svelte](../../../web/src/routes/lidar/tracks/+page.svelte)
- **Fix:** filter from run-scoped entities directly (not global membership by ID only).

#### 7.1 Throttle-related dt spikes

- **Severity:** Medium
- **Files:** [tracking_pipeline.go](../../../internal/lidar/tracking_pipeline.go)
- **Fix:** keep dt bounded to frame-rate assumptions across throttled frames.

### R3 — UX/polish and housekeeping

#### 1.3 Deleted-track DB pruning

- **Severity:** Medium
- **Files:** [tracking.go](../../../internal/lidar/tracking.go), [track_store.go](../../../internal/lidar/track_store.go)
- **Fix:** TTL/periodic prune for deleted tracks and observations.

#### 6.2 Per-track colour differentiation within class

- **Severity:** Medium
- **Files:** [MapPane.svelte](../../../web/src/lib/components/lidar/MapPane.svelte), [lidar.ts](../../../web/src/lib/types/lidar.ts)
- **Fix:** deterministic track-ID hue variation inside class palette.

#### 6.3 Temporal fade on trails

- **Severity:** Low
- **Files:** [MapPane.svelte](../../../web/src/lib/components/lidar/MapPane.svelte)
- **Fix:** age-weighted alpha for trail segments.

#### 6.5 Foreground observation sampling bias

- **Severity:** Low
- **Files:** [+page.svelte](../../../web/src/routes/lidar/tracks/+page.svelte)
- **Fix:** visible-window-scoped query and/or pagination.

#### 7.2 Miss accounting on throttled frames

- **Severity:** Low
- **Files:** [tracking_pipeline.go](../../../internal/lidar/tracking_pipeline.go)
- **Fix:** lightweight miss advancement when frame is throttled.

#### 3.2 Non-convex centroid stability

- **Severity:** Low
- **Files:** [clustering.go](../../../internal/lidar/clustering.go)
- **Fix:** medoid/weighted centroid or temporal smoothing.

---

## Delivery order

1. **P0 batch:** 8.1 (height filter — critical, most impact), 8.2 (OBB heading to web API), 8.3 (per-frame OBB dims)
2. **R1 batch:** 2.5, 6.4, 2.4, 2.3
3. **R2 batch:** 3.1, 3.3, 4.3, 5.2, 7.1, 8.4
4. **R3 batch:** 1.3, 6.2, 6.3, 6.5, 7.2, 3.2

---

## Validation protocol for remaining work

After each batch:

1. `make format`
2. `make lint`
3. `make test`
4. `go test -race ./internal/lidar/... -count=3`
5. Manual check in tracks UI for long diagonal artefacts and run filtering correctness
