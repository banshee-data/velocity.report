# Garbage Tracks Review (LiDAR Pipeline + Svelte Tracks App)

Date: 2026-02-16

## Scope reviewed

- LiDAR tracking/persistence pipeline (`internal/lidar/*`, `internal/lidar/monitor/track_api.go`).
- Svelte tracks UI/data flow (`web/src/routes/lidar/tracks/+page.svelte`, `web/src/lib/components/lidar/MapPane.svelte`, `web/src/lib/api.ts`).

## High-confidence bugs likely causing "garbage tracks"

### 1) `track_id` is globally reused and overwritten across sessions/sensors

`track_id` is generated as `track_<counter>` with the counter reset to 1 on tracker reset/startup, so IDs are not globally unique. `InsertTrack` then upserts only on `track_id`, which merges unrelated trajectories into one logical track row over time.

- ID generation + reset behaviour: `NextTrackID` starts at 1 and `initTrack` emits `track_%d`. `Reset()` also resets `NextTrackID` to 1.
- DB key/upsert behaviour: `lidar_tracks.track_id` is the primary key and writes use `ON CONFLICT(track_id) DO UPDATE`.

**Why this creates spaghetti:** different real objects/runs reusing `track_1`, `track_2`, etc. become one "track" in persistence/history views.

### 2) Track history lookup is not sensor-scoped (cross-sensor/cross-run contamination)

`GetTrackObservations(trackID)` queries only by `track_id` and is used to build history in both `GetActiveTracks` and `GetTracksInRange`. If `track_id` is reused, history can contain observations from unrelated contexts.

- `GetTrackObservations`: `WHERE track_id = ?` only.
- `GetActiveTracks` and `GetTracksInRange` then populate each track's `History` from that unscoped query.

**Why this creates spaghetti:** one rendered polyline can include points from multiple sessions or even multiple sensors.

### 3) Range query returns tracks in window, but each track gets full (unbounded) history

When calling history endpoints with a time range, track selection is range-aware, but the per-track history backfill ignores the requested range and pulls recent obs by track ID (limit 1000).

- `GetTracksInRange` selects rows by overlap with `start/end`.
- Then it calls `GetTrackObservations(track.TrackID, 1000)` (not range-filtered) for each track.

**Why this creates spaghetti:** UI can display segments far outside selected playback window/run.

### 4) Frontend requests "all time" by default and compounds ID-collision artifacts

The tracks page loads from epoch (`startTime = 0`) to now and asks for up to 1000 tracks in one pass.

- `loadHistoricalData()` uses `startTime = 0` and `endTime = Date.now()*1e6`.

**Why this creates spaghetti:** if persisted IDs/history are already contaminated, this maximally exposes/overlays them.

### 5) API contract mismatch for `getTrackObservations(trackId)`

Backend `/api/lidar/tracks/{id}/observations` returns an envelope object (`{ track_id, observations, count, timestamp }`), but frontend helper `getTrackObservations` returns raw `res.json()` typed as `TrackObservation[]`.

**Impact:** selected-track overlays can be malformed or fail at runtime; this can hide signal and make debugging trajectory quality much harder.

### 6) Run filter uses track IDs that are not run-unique

Run-mode filtering creates `Set(runTrack.track_id)` and filters global `tracks` by those IDs. If IDs are recycled, run filtering can accidentally include trajectories from other runs.

**Why this creates spaghetti in run view:** unrelated historical tracks can appear when selecting one run.

## Pipeline/visualisation behaviours that amplify bad data

### 7) Coasting points are appended during misses

On unmatched frames, predicted positions are appended to `track.History` until miss threshold deletion.

**Risk:** when associations are bad or missed periods are long, these synthetic segments can draw long straight lines that look like tracker explosions.

### 8) Frontend draws full per-track polylines and connects all valid points

`MapPane` sorts history and draws a continuous polyline across all valid points visible by time; it does not break on large temporal/spatial gaps.

**Risk:** any discontinuity in history (from merged IDs or sparse observations) is rendered as long diagonal lines.

### 9) Foreground overlay query is hard-capped while window can be huge

Foreground observations are loaded for the entire selected range with limit 4000.

**Risk:** heavy truncation/sampling bias can make overlay points appear inconsistent with track trails, making bad tracks seem even more "garbage" and harder to diagnose.

## Prioritised remediation plan

1. **Make track identity globally unique in persistence**
   - Replace `track_<counter>` with UUID/ULID, or make DB keys composite (`sensor_id`, `track_id`) and include run/session namespace.
2. **Sensor- and range-scope all observation/history queries**
   - Add `sensor_id` (and optionally run/session key) filters to `GetTrackObservations` usages.
   - Build track history in-range from `GetTrackObservationsInRange` instead of unbounded `GetTrackObservations`.
3. **Fix frontend observation API parsing**
   - `getTrackObservations(trackId)` should return `payload.observations ?? []`.
4. **Make run filtering robust to recycled IDs**
   - Use run-specific track entities from `lidar_run_tracks` directly for trajectory rendering when run is selected.
5. **Add polyline gap breaking in renderer**
   - Break history stroke when timestamp gap or spatial jump exceeds threshold.
6. **Tighten default query windows**
   - Avoid epoch-to-now queries by default; start with bounded window or selected run/scene.

## Quick validation checks to run after fixes

- Start tracker twice (or reset), produce tracks in two sessions, verify no shared IDs or merged history.
- Query `/api/lidar/tracks/history` for a narrow time window and confirm each `track.history` lies within that window.
- In run mode, ensure only that run's trajectories are visible.
- Select track and confirm observations overlay loads correctly (array shape, no runtime errors).
