# Track Storage Consolidation

- **Status:** Complete

This document records the decision to keep live and run track tables separate while consolidating their shared column definitions into a single `TrackMeasurement` struct in the Go layer.

## Problem

Two tables with 16 overlapping columns:

| Table              | Purpose                            | PK                   |
| ------------------ | ---------------------------------- | -------------------- |
| `lidar_tracks`     | Live transient buffer (pruned ~5m) | `track_id`           |
| `lidar_run_tracks` | Immutable analysis-run snapshots   | `(run_id, track_id)` |

No JOINs between them anywhere. Completely separate code paths and
consumers.

## Decision: Option B — Keep Separate Tables, Normalise Go Layer

SQL duplication is an intentional snapshot pattern — different lifecycles,
different PKs, different FK relationships. Forcing both into one table
creates mixed-lifecycle management, FK rewrites, and hot-path performance
regression.

The real DRY violation is in Go code: 16 columns spelled out in two
structs, two INSERT functions, and two scan loops.

## Implementation

### Shared `TrackMeasurement` Struct

Embedded in both `TrackedObject` and `RunTrack`:

```go
type TrackMeasurement struct {
    SensorID             string
    TrackState           string
    StartUnixNanos       int64
    EndUnixNanos         int64
    ObservationCount     int
    AvgSpeedMps          float32
    MaxSpeedMps          float32
    BoundingBoxLengthAvg float32
    BoundingBoxWidthAvg  float32
    BoundingBoxHeightAvg float32
    HeightP95Max         float32
    IntensityMeanAvg     float32
    ObjectClass          string
    ObjectConfidence     float32
    ClassificationModel  string
}
```

### Shared Helpers

- `trackMeasurementColumns` — single constant slice used in both INSERTs.
- `scanTrackMeasurementDests()` — used by `GetActiveTracks()`,
  `GetRunTracks()`, `GetTracksInRange()`, `GetRunTrack()`,
  `GetUnlabeledTracks()`.
- `trackMeasurementInsertArgs()` and `trackMeasurementUpdateArgs()`.

### Optional SQL VIEW

`lidar_all_tracks` unions both tables for ad-hoc SQL analysis. No Go code
dependency.

## Why Not Merge (Option A Rejected)

- SQLite PK limitations with nullable `run_id`.
- FK from `lidar_track_observations` assumes `track_id` alone is unique.
- Hot upsert table would contain immutable snapshot rows.
- Mixed lifecycle (ephemeral + permanent) in one table.
- Multi-step data migration with FK rewrites.

## Why Not Slim (Option C Rejected)

Live tracks are pruned after ~5 minutes. Once pruned, JOINs return no
data for historical run analysis. Would require a "pinning" mechanism
that adds more complexity than the duplication it removes.
