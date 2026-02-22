# LiDAR Analysis Run Infrastructure

## Status: Implemented

## Summary

Reproducible analysis runs with versioned parameter configurations, allowing
comparison across runs with different parameters and detection of track
splits/merges. This was Phase 3.7 of the LiDAR ML pipeline.

## Related Documents

- [Product Roadmap](../ROADMAP.md) — milestone placement
- [Sweep/HINT Mode](lidar-sweep-hint-mode-plan.md) — parameter sweep system built on this infrastructure
- [Track Labelling & Auto-Aware Tuning](lidar-track-labeling-auto-aware-tuning-plan.md) — labelling UI that consumes analysis runs

---

## Implementation Files

| File | Description |
|------|-------------|
| `internal/lidar/analysis_run.go` | Core types and database operations |
| `internal/lidar/analysis_run_test.go` | Unit tests |
| `internal/db/migrations/000010_create_lidar_analysis_runs.up.sql` | Database migration |
| `internal/db/schema.sql` | Updated with analysis run tables |

---

## Schema

```sql
CREATE TABLE IF NOT EXISTS lidar_analysis_runs (
    run_id TEXT PRIMARY KEY,
    created_at INTEGER NOT NULL,
    source_type TEXT NOT NULL,            -- 'pcap' or 'live'
    source_path TEXT,
    sensor_id TEXT NOT NULL,
    params_json TEXT NOT NULL,            -- All LiDAR params in single JSON blob
    duration_secs REAL,
    total_frames INTEGER,
    total_clusters INTEGER,
    total_tracks INTEGER,
    confirmed_tracks INTEGER,
    processing_time_ms INTEGER,
    status TEXT DEFAULT 'running',        -- 'running', 'completed', 'failed'
    error_message TEXT,
    parent_run_id TEXT,
    notes TEXT
);

CREATE TABLE IF NOT EXISTS lidar_run_tracks (
    run_id TEXT NOT NULL,
    track_id TEXT NOT NULL,
    sensor_id TEXT NOT NULL,
    track_state TEXT NOT NULL,
    start_unix_nanos INTEGER NOT NULL,
    end_unix_nanos INTEGER,
    observation_count INTEGER,
    avg_speed_mps REAL,
    peak_speed_mps REAL,
    p50_speed_mps REAL,
    p85_speed_mps REAL,
    p95_speed_mps REAL,
    bounding_box_length_avg REAL,
    bounding_box_width_avg REAL,
    bounding_box_height_avg REAL,
    height_p95_max REAL,
    intensity_mean_avg REAL,
    object_class TEXT,
    object_confidence REAL,
    classification_model TEXT,
    user_label TEXT,
    label_confidence REAL,
    labeler_id TEXT,
    labeled_at INTEGER,
    is_split_candidate INTEGER DEFAULT 0,
    is_merge_candidate INTEGER DEFAULT 0,
    linked_track_ids TEXT,
    PRIMARY KEY (run_id, track_id),
    FOREIGN KEY (run_id) REFERENCES lidar_analysis_runs(run_id) ON DELETE CASCADE
);
```

---

## Params JSON Structure

All LiDAR parameters stored in a single JSON blob for complete reproducibility:

```json
{
  "version": "1.0",
  "timestamp": "2025-12-01T00:00:00Z",
  "background": {
    "background_update_fraction": 0.02,
    "closeness_sensitivity_multiplier": 3.0,
    "safety_margin_meters": 0.5,
    "neighbor_confirmation_count": 3,
    "noise_relative_fraction": 0.315,
    "seed_from_first_observation": true,
    "freeze_duration_nanos": 5000000000
  },
  "clustering": { "eps": 0.6, "min_pts": 12, "cell_size": 0.6 },
  "tracking": {
    "max_tracks": 100,
    "max_misses": 3,
    "hits_to_confirm": 3,
    "gating_distance_squared": 25.0,
    "process_noise": [0.1, 0.1, 0.5, 0.5],
    "measurement_noise": [0.2, 0.2],
    "deleted_track_grace_period_nanos": 5000000000
  },
  "classification": {
    "model_type": "rule_based",
    "thresholds": {
      "pedestrian": { "height_min": 1.0, "height_max": 2.0, "speed_max": 3.0 },
      "car": { "height_min": 1.2, "length_min": 3.0, "speed_min": 5.0 },
      "bird": { "height_max": 0.5, "speed_max": 1.0 }
    }
  }
}
```

---

## Go Types

```go
type AnalysisRun struct {
    RunID           string            `json:"run_id"`
    CreatedAt       time.Time         `json:"created_at"`
    SourceType      string            `json:"source_type"`
    SourcePath      string            `json:"source_path,omitempty"`
    SensorID        string            `json:"sensor_id"`
    ParamsJSON      json.RawMessage   `json:"params_json"`
    DurationSecs    float64           `json:"duration_secs"`
    TotalFrames     int               `json:"total_frames"`
    TotalClusters   int               `json:"total_clusters"`
    TotalTracks     int               `json:"total_tracks"`
    ConfirmedTracks int               `json:"confirmed_tracks"`
    ProcessingTimeMs int64            `json:"processing_time_ms"`
    Status          string            `json:"status"`
    ParentRunID     string            `json:"parent_run_id,omitempty"`
    Notes           string            `json:"notes,omitempty"`
}

type RunParams struct {
    Version        string                     `json:"version"`
    Timestamp      time.Time                  `json:"timestamp"`
    Background     BackgroundParamsExport     `json:"background"`
    Clustering     ClusteringParamsExport     `json:"clustering"`
    Tracking       TrackingParamsExport       `json:"tracking"`
    Classification ClassificationParamsExport `json:"classification"`
}
```

---

## Track Split/Merge Detection

```go
type RunComparison struct {
    Run1ID          string           `json:"run1_id"`
    Run2ID          string           `json:"run2_id"`
    ParamDiff       map[string]any   `json:"param_diff"`
    TracksOnlyRun1  []string         `json:"tracks_only_run1"`
    TracksOnlyRun2  []string         `json:"tracks_only_run2"`
    SplitCandidates []TrackSplit     `json:"split_candidates"`
    MergeCandidates []TrackMerge     `json:"merge_candidates"`
    MatchedTracks   []TrackMatch     `json:"matched_tracks"`
}
```

Algorithm: build spatial-temporal index for each run; for each track in run1
find overlapping tracks in run2; one-to-many = split, many-to-one = merge;
confidence based on overlap percentage.
