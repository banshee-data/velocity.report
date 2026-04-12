# Track storage consolidation

- **Status:** Complete

Decision to keep live and run track tables separate while consolidating their shared column definitions into a single `TrackMeasurement` struct in the Go layer.

## Problem

Two tables with 16 overlapping columns:

| Table              | Purpose                            | PK                   |
| ------------------ | ---------------------------------- | -------------------- |
| `lidar_tracks`     | Live transient buffer (pruned ~5m) | `track_id`           |
| `lidar_run_tracks` | Immutable analysis-run snapshots   | `(run_id, track_id)` |

No JOINs between them anywhere. Completely separate code paths and
consumers.

## Decision: option b; keep separate tables, normalise Go layer

SQL duplication is an intentional snapshot pattern: different lifecycles,
different PKs, different FK relationships. Forcing both into one table
creates mixed-lifecycle management, FK rewrites, and hot-path performance
regression.

The real DRY violation is in Go code: 16 columns spelled out in two
structs, two INSERT functions, and two scan loops.

## Implementation

### Shared `TrackMeasurement` struct

Embedded in both `TrackedObject` and `RunTrack`:

| Field                  | Type      | Purpose                        |
| ---------------------- | --------- | ------------------------------ |
| `SensorID`             | `string`  | Sensor identifier              |
| `TrackState`           | `string`  | Current track state            |
| `StartUnixNanos`       | `int64`   | Track start time (nanoseconds) |
| `EndUnixNanos`         | `int64`   | Track end time (nanoseconds)   |
| `ObservationCount`     | `int`     | Number of observations         |
| `AvgSpeedMps`          | `float32` | Average speed (m/s)            |
| `MaxSpeedMps`          | `float32` | Maximum speed (m/s)            |
| `BoundingBoxLengthAvg` | `float32` | Average bounding box length    |
| `BoundingBoxWidthAvg`  | `float32` | Average bounding box width     |
| `BoundingBoxHeightAvg` | `float32` | Average bounding box height    |
| `HeightP95Max`         | `float32` | 95th percentile max height     |
| `IntensityMeanAvg`     | `float32` | Average mean intensity         |
| `ObjectClass`          | `string`  | Classified object type         |
| `ObjectConfidence`     | `float32` | Classification confidence      |
| `ClassificationModel`  | `string`  | Model used for classification  |

### Shared helpers

- `trackMeasurementColumns`: single constant slice used in both INSERTs.
- `scanTrackMeasurementDests()`: used by `GetActiveTracks()`,
  `GetRunTracks()`, `GetTracksInRange()`, `GetRunTrack()`,
  `GetUnlabeledTracks()`.
- `trackMeasurementInsertArgs()` and `trackMeasurementUpdateArgs()`.

### Optional SQL VIEW

`lidar_all_tracks` unions both tables for ad-hoc SQL analysis. No Go code
dependency.

## Why not merge (option a rejected)

- SQLite PK limitations with nullable `run_id`.
- FK from `lidar_track_observations` assumes `track_id` alone is unique.
- Hot upsert table would contain immutable snapshot rows.
- Mixed lifecycle (ephemeral + permanent) in one table.
- Multi-step data migration with FK rewrites.

## Why not slim (option c rejected)

Live tracks are pruned after ~5 minutes. Once pruned, JOINs return no
data for historical run analysis. Would require a "pinning" mechanism
that adds more complexity than the duplication it removes.
