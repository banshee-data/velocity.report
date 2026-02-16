# Garbage Tracks — Consolidated Status and Remaining Priorities

This is the canonical document for garbage-track remediation.
It combines the original review and checklist into one maintained source.

Updated: 2026-02-15

---

## Scope and context

- Reviewed layers: LiDAR tracking/persistence pipeline and Svelte tracks rendering flow.
- Goal: remove trajectory contamination, avoid spaghetti artefacts, and harden pipeline determinism.
- Canonical reference files:
  - [tracking.go](../../../internal/lidar/tracking.go)
  - [track_store.go](../../../internal/lidar/track_store.go)
  - [frame_builder.go](../../../internal/lidar/frame_builder.go)
  - [api.ts](../../../web/src/lib/api.ts)
  - [MapPane.svelte](../../../web/src/lib/components/lidar/MapPane.svelte)

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

## Delivery order from today

1. **R1 batch:** 2.5, 6.4, 2.4, 2.3
2. **R2 batch:** 3.1, 3.3, 4.3, 5.2, 7.1
3. **R3 batch:** 1.3, 6.2, 6.3, 6.5, 7.2, 3.2

---

## Validation protocol for remaining work

After each batch:

1. `make format`
2. `make lint`
3. `make test`
4. `go test -race ./internal/lidar/... -count=3`
5. Manual check in tracks UI for long diagonal artefacts and run filtering correctness
