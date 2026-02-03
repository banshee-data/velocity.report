# LIDAR ML Pipeline Roadmap

**Status:** Phase 3.7 Implemented, Phases 4.0-4.3 Planned
**Date:** December 1, 2025
**Author:** Copilot (Architectural Analysis)
**Version:** 1.1

---

## Executive Summary

This document outlines the evolution of the LIDAR tracking system from rule-based classification to a full ML-driven classification pipeline with labeling UI, parameter tuning, and comparative analysis.

**Current State (Completed through Phase 3.7):**

- ✅ Background subtraction and foreground extraction
- ✅ DBSCAN clustering with spatial indexing
- ✅ Kalman tracking with lifecycle management
- ✅ Rule-based classification (pedestrian, car, bird, other)
- ✅ REST API endpoints for track/cluster access
- ✅ PCAP analysis tool for batch processing
- ✅ Training data export (compact binary encoding)
- ✅ **Analysis Run Infrastructure** (params JSON, run comparison, split/merge detection)

**Roadmap Phases:**

- **Phase 3.7:** ✅ Analysis Run Infrastructure (IMPLEMENTED)
- **Phase 4.0:** Track Labeling UI (web-based annotation)
- **Phase 4.1:** ML Classifier Training Pipeline
- **Phase 4.2:** Parameter Tuning & Optimization
- **Phase 4.3:** Production Deployment

---

## Table of Contents

1. [Current Architecture](#current-architecture)
2. [Phase 3.7: Analysis Run Infrastructure](#phase-37-analysis-run-infrastructure) ✅
3. [Phase 4.0: Track Labeling UI](#phase-40-track-labeling-ui)
4. [Phase 4.1: ML Classifier Training](#phase-41-ml-classifier-training)
5. [Phase 4.2: Parameter Tuning & Optimization](#phase-42-parameter-tuning--optimisation)
6. [Phase 4.3: Production Deployment](#phase-43-production-deployment)
7. [Data Flow Summary](#data-flow-summary)

---

## Current Architecture

### Existing Data Flow

```
PCAP/Live UDP → Parse → Frame → Background → Foreground → Cluster → Track → Classify → API
                                                                                 ↓
                                                                          JSON/CSV Export
                                                                                 ↓
                                                                        Training Data Blobs
```

### Existing Components

| Component              | Location                              | Status      |
| ---------------------- | ------------------------------------- | ----------- |
| PCAP Reader            | `internal/lidar/network/pcap.go`      | ✅ Complete |
| Frame Builder          | `internal/lidar/frame_builder.go`     | ✅ Complete |
| Background Manager     | `internal/lidar/background.go`        | ✅ Complete |
| Foreground Extraction  | `internal/lidar/foreground.go`        | ✅ Complete |
| DBSCAN Clustering      | `internal/lidar/clustering.go`        | ✅ Complete |
| Kalman Tracking        | `internal/lidar/tracking.go`          | ✅ Complete |
| Rule-Based Classifier  | `internal/lidar/classification.go`    | ✅ Complete |
| Track Store            | `internal/lidar/track_store.go`       | ✅ Complete |
| REST API               | `internal/lidar/monitor/track_api.go` | ✅ Complete |
| PCAP Analyze Tool      | `cmd/tools/pcap-analyze/main.go`      | ✅ Complete |
| Training Data Export   | `internal/lidar/training_data.go`     | ✅ Complete |
| **Analysis Run Store** | `internal/lidar/analysis_run.go`      | ✅ Complete |

---

## Phase 3.7: Analysis Run Infrastructure ✅ IMPLEMENTED

### Objective

Enable reproducible analysis runs with versioned parameter configurations, allowing comparison across runs with different parameters and detection of track splits/merges.

### Implementation Files

| File                                                              | Description                        |
| ----------------------------------------------------------------- | ---------------------------------- |
| `internal/lidar/analysis_run.go`                                  | Core types and database operations |
| `internal/lidar/analysis_run_test.go`                             | Unit tests                         |
| `internal/db/migrations/000010_create_lidar_analysis_runs.up.sql` | Database migration                 |
| `internal/db/schema.sql`                                          | Updated with analysis run tables   |

### 3.7.1: Analysis Run Schema

```sql
-- Analysis runs with full parameter configuration
CREATE TABLE IF NOT EXISTS lidar_analysis_runs (
    run_id TEXT PRIMARY KEY,              -- UUID or timestamp-based ID
    created_at INTEGER NOT NULL,          -- Unix timestamp
    source_type TEXT NOT NULL,            -- 'pcap' or 'live'
    source_path TEXT,                     -- PCAP file path (if applicable)
    sensor_id TEXT NOT NULL,

    -- Full parameter configuration as JSON
    params_json TEXT NOT NULL,            -- All LIDAR params in single JSON blob

    -- Run statistics
    duration_secs REAL,
    total_frames INTEGER,
    total_clusters INTEGER,
    total_tracks INTEGER,
    confirmed_tracks INTEGER,

    -- Processing metadata
    processing_time_ms INTEGER,
    status TEXT DEFAULT 'running',        -- 'running', 'completed', 'failed'
    error_message TEXT,

    -- Comparison metadata
    parent_run_id TEXT,                   -- For parameter tuning comparisons
    notes TEXT                            -- User notes about this run
);

CREATE INDEX idx_runs_created ON lidar_analysis_runs(created_at);
CREATE INDEX idx_runs_source ON lidar_analysis_runs(source_path);
CREATE INDEX idx_runs_parent ON lidar_analysis_runs(parent_run_id);

-- Track results per run (extends lidar_tracks with run_id)
CREATE TABLE IF NOT EXISTS lidar_run_tracks (
    run_id TEXT NOT NULL,
    track_id TEXT NOT NULL,

    -- All track fields from lidar_tracks
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

    -- Classification (rule-based or ML)
    object_class TEXT,
    object_confidence REAL,
    classification_model TEXT,

    -- User labels (for training)
    user_label TEXT,                      -- Human-assigned label
    label_confidence REAL,                -- Annotator confidence
    labeler_id TEXT,                      -- Who labeled this
    labeled_at INTEGER,                   -- When labeled

    -- Track quality flags
    is_split_candidate INTEGER DEFAULT 0,   -- Suspected split
    is_merge_candidate INTEGER DEFAULT 0,   -- Suspected merge
    linked_track_ids TEXT,                  -- JSON array of related track IDs

    PRIMARY KEY (run_id, track_id),
    FOREIGN KEY (run_id) REFERENCES lidar_analysis_runs(run_id) ON DELETE CASCADE
);

CREATE INDEX idx_run_tracks_run ON lidar_run_tracks(run_id);
CREATE INDEX idx_run_tracks_class ON lidar_run_tracks(object_class);
CREATE INDEX idx_run_tracks_label ON lidar_run_tracks(user_label);
```

### 3.7.2: Params JSON Structure

All LIDAR parameters stored in a single JSON blob for complete reproducibility:

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

  "clustering": {
    "eps": 0.6,
    "min_pts": 12,
    "cell_size": 0.6
  },

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
      "pedestrian": {
        "height_min": 1.0,
        "height_max": 2.0,
        "speed_max": 3.0
      },
      "car": {
        "height_min": 1.2,
        "length_min": 3.0,
        "speed_min": 5.0
      },
      "bird": {
        "height_max": 0.5,
        "speed_max": 1.0
      }
    }
  }
}
```

### 3.7.3: Go Implementation ✅

The following types and functions are implemented in `internal/lidar/analysis_run.go`:

```go
// AnalysisRun represents a complete analysis session with parameters
type AnalysisRun struct {
    RunID           string            `json:"run_id"`
    CreatedAt       time.Time         `json:"created_at"`
    SourceType      string            `json:"source_type"` // "pcap" or "live"
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

// RunParams captures all configurable parameters for reproducibility
type RunParams struct {
    Version        string                     `json:"version"`
    Timestamp      time.Time                  `json:"timestamp"`
    Background     BackgroundParamsExport     `json:"background"`
    Clustering     ClusteringParamsExport     `json:"clustering"`
    Tracking       TrackingParamsExport       `json:"tracking"`
    Classification ClassificationParamsExport `json:"classification"`
}

// AnalysisRunStore provides database operations for analysis runs
type AnalysisRunStore struct { ... }
func NewAnalysisRunStore(db *sql.DB) *AnalysisRunStore
func (s *AnalysisRunStore) InsertRun(run *AnalysisRun) error
func (s *AnalysisRunStore) CompleteRun(runID string, stats *AnalysisStats) error
func (s *AnalysisRunStore) GetRun(runID string) (*AnalysisRun, error)
func (s *AnalysisRunStore) ListRuns(limit int) ([]*AnalysisRun, error)
func (s *AnalysisRunStore) InsertRunTrack(track *RunTrack) error
func (s *AnalysisRunStore) GetRunTracks(runID string) ([]*RunTrack, error)
func (s *AnalysisRunStore) UpdateTrackLabel(...) error
func (s *AnalysisRunStore) GetLabelingProgress(runID string) (total, labeled int, byClass map[string]int, err error)
func (s *AnalysisRunStore) GetUnlabeledTracks(runID string, limit int) ([]*RunTrack, error)
```

### 3.7.4: Track Split/Merge Detection

Types for detecting when parameter changes cause tracks to split or merge:

```go
// RunComparison shows differences between two analysis runs
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

type TrackSplit struct {
    OriginalTrack  string   `json:"original_track"`
    SplitTracks    []string `json:"split_tracks"`
    SplitX, SplitY float32  `json:"split_location"`
    Confidence     float32  `json:"confidence"`
}

type TrackMerge struct {
    MergedTrack    string   `json:"merged_track"`
    SourceTracks   []string `json:"source_tracks"`
    MergeX, MergeY float32  `json:"merge_location"`
    Confidence     float32  `json:"confidence"`
}

// Future: DetectSplitsMerges compares spatiotemporal overlap between runs
// Algorithm:
// 1. Build spatial-temporal index for each run
// 2. For each track in run1, find overlapping tracks in run2
// 3. If one track maps to multiple: potential split
// 4. If multiple tracks map to one: potential merge
// 5. Compute confidence based on overlap percentage
```

---

## Phase 4.0: Track Labeling UI

### Objective

Provide a web-based interface for human annotators to label tracks, review classifications, and mark quality issues.

### 4.0.1: UI Requirements

**Core Features:**

1. **Track Browser:** List and filter tracks by run, class, time range
2. **Track Viewer:** Visualize track trajectory on 2D map
3. **Labeling Interface:** Assign class labels with confidence
4. **Quality Marking:** Flag splits, merges, noise tracks
5. **Bulk Actions:** Apply labels to multiple similar tracks
6. **Progress Tracking:** Show annotation completion status

### 4.0.2: UI Architecture

Using the existing SvelteKit frontend (`web/`):

```
web/src/routes/
├── lidar/
│   ├── +page.svelte              # Dashboard with run list
│   ├── runs/
│   │   ├── +page.svelte          # Analysis run browser
│   │   └── [run_id]/
│   │       ├── +page.svelte      # Run details with track list
│   │       └── tracks/
│   │           └── [track_id]/
│   │               └── +page.svelte  # Individual track viewer
│   ├── labeling/
│   │   ├── +page.svelte          # Labeling queue interface
│   │   └── +page.server.ts       # Server-side data loading
│   └── compare/
│       └── +page.svelte          # Run comparison tool
```

### 4.0.3: UI Components (svelte-ux based)

```svelte
<!-- web/src/lib/components/lidar/TrackViewer.svelte -->
<script lang="ts">
  import { Canvas } from 'svelte-ux';
  import type { Track, TrackObservation } from '$lib/types/lidar';

  export let track: Track;
  export let observations: TrackObservation[];

  // Render track trajectory as path
  // Show velocity vectors
  // Highlight classification features
</script>

<!-- web/src/lib/components/lidar/LabelingPanel.svelte -->
<script lang="ts">
  import { Select, Button, TextField } from 'svelte-ux';

  export let track: Track;
  export let onLabel: (label: string, confidence: number) => void;

  const classOptions = [
    { value: 'pedestrian', label: 'Pedestrian' },
    { value: 'car', label: 'Car' },
    { value: 'cyclist', label: 'Cyclist' },
    { value: 'bird', label: 'Bird' },
    { value: 'noise', label: 'Noise/Artifact' },
    { value: 'other', label: 'Other' }
  ];
</script>

<!-- web/src/lib/components/lidar/RunComparisonView.svelte -->
<script lang="ts">
  // Side-by-side comparison of two runs
  // Highlight split/merge candidates
  // Show parameter differences
</script>
```

### 4.0.4: REST API Extensions

```go
// Additional endpoints for labeling UI

// Label a track
// PUT /api/lidar/runs/{run_id}/tracks/{track_id}/label
type LabelRequest struct {
    UserLabel       string  `json:"user_label"`
    LabelConfidence float32 `json:"label_confidence"`
    LabelerID       string  `json:"labeler_id"`
    IsSplitCandidate bool   `json:"is_split_candidate,omitempty"`
    IsMergeCandidate bool   `json:"is_merge_candidate,omitempty"`
    LinkedTrackIDs   []string `json:"linked_track_ids,omitempty"`
    Notes           string  `json:"notes,omitempty"`
}

// Get labeling progress
// GET /api/lidar/runs/{run_id}/labeling-progress
type LabelingProgress struct {
    TotalTracks     int     `json:"total_tracks"`
    LabeledTracks   int     `json:"labeled_tracks"`
    UnlabeledTracks int     `json:"unlabeled_tracks"`
    ByClass         map[string]int `json:"by_class"`
    ByLabeler       map[string]int `json:"by_labeler"`
}

// Get tracks needing review (unlabeled or low confidence)
// GET /api/lidar/runs/{run_id}/review-queue
type ReviewQueueParams struct {
    MinConfidence float32 `query:"min_confidence"`
    Class         string  `query:"class"`
    Limit         int     `query:"limit"`
}

// Compare two runs
// GET /api/lidar/runs/compare?run1={id}&run2={id}
// Returns: RunComparison
```

---

## Phase 4.1: ML Classifier Training

### Objective

Train an ML model to replace rule-based classification using labeled track data.

### 4.1.1: Training Data Pipeline

```
Labeled Tracks (DB) → Feature Extraction → Training Dataset → Model Training → Model Deployment
```

### 4.1.2: Feature Vector

Extract features from labeled tracks for ML training:

```python
# tools/ml-training/features.py

class TrackFeatures:
    """Feature vector for track classification"""

    # Spatial features (shape)
    bounding_box_length_avg: float
    bounding_box_width_avg: float
    bounding_box_height_avg: float
    height_p95_max: float
    aspect_ratio_xy: float  # length/width
    aspect_ratio_xz: float  # length/height

    # Kinematic features (motion)
    avg_speed_mps: float
    peak_speed_mps: float
    p50_speed_mps: float
    p85_speed_mps: float
    p95_speed_mps: float
    speed_variance: float
    acceleration_max: float
    heading_variance: float

    # Temporal features
    duration_secs: float
    observation_count: int
    observations_per_second: float

    # Intensity features
    intensity_mean_avg: float
    intensity_variance: float

    @classmethod
    def from_track(cls, track: dict) -> 'TrackFeatures':
        """Extract features from track dictionary"""
        return cls(
            bounding_box_length_avg=track['bounding_box_length_avg'],
            bounding_box_width_avg=track['bounding_box_width_avg'],
            # ... etc
        )

    def to_vector(self) -> np.ndarray:
        """Convert to numpy array for model input"""
        return np.array([
            self.bounding_box_length_avg,
            self.bounding_box_width_avg,
            # ... normalized features
        ])
```

### 4.1.3: Model Training Script

```python
# tools/ml-training/train_classifier.py

import sqlite3
import numpy as np
from sklearn.ensemble import RandomForestClassifier
from sklearn.model_selection import cross_val_score
import joblib

def load_labeled_tracks(db_path: str, min_confidence: float = 0.7) -> tuple:
    """Load labeled tracks from database"""
    conn = sqlite3.connect(db_path)
    query = """
        SELECT * FROM lidar_run_tracks
        WHERE user_label IS NOT NULL
        AND label_confidence >= ?
    """
    tracks = pd.read_sql(query, conn, params=[min_confidence])
    conn.close()

    # Extract features
    X = np.array([TrackFeatures.from_track(t).to_vector() for t in tracks.to_dict('records')])
    y = tracks['user_label'].values

    return X, y, tracks

def train_model(X, y, model_type='random_forest'):
    """Train classification model"""
    if model_type == 'random_forest':
        model = RandomForestClassifier(
            n_estimators=100,
            max_depth=10,
            min_samples_split=5,
            class_weight='balanced'
        )

    # Cross-validation
    scores = cross_val_score(model, X, y, cv=5, scoring='f1_weighted')
    print(f"Cross-validation F1: {scores.mean():.3f} (+/- {scores.std():.3f})")

    # Train final model
    model.fit(X, y)

    return model

def export_model(model, output_path: str, version: str):
    """Export model for deployment"""
    metadata = {
        'version': version,
        'feature_names': TrackFeatures.feature_names(),
        'class_names': model.classes_.tolist(),
        'created_at': datetime.now().isoformat()
    }

    joblib.dump({
        'model': model,
        'metadata': metadata
    }, output_path)

if __name__ == '__main__':
    X, y, tracks = load_labeled_tracks('sensor_data.db')
    model = train_model(X, y)
    export_model(model, 'models/track_classifier_v1.joblib', 'v1.0')
```

### 4.1.4: Model Deployment in Go

```go
// internal/lidar/ml_classifier.go

// MLClassifier wraps a trained model for track classification
type MLClassifier struct {
    modelPath    string
    modelVersion string
    featureNames []string
    classNames   []string
    // For simple models, embed weights directly
    // For complex models, use ONNX runtime or similar
}

// NewMLClassifier loads a trained model
func NewMLClassifier(modelPath string) (*MLClassifier, error)

// Classify predicts class for a track
func (c *MLClassifier) Classify(track *TrackedObject) (string, float32, error)

// ClassifierFactory selects between rule-based and ML classifiers
type ClassifierFactory struct {
    RuleBased  *TrackClassifier
    ML         *MLClassifier
    UseML      bool
}

func (f *ClassifierFactory) Classify(track *TrackedObject) (string, float32) {
    if f.UseML && f.ML != nil {
        class, conf, err := f.ML.Classify(track)
        if err == nil {
            return class, conf
        }
        // Fall back to rule-based on error
    }
    return f.RuleBased.Classify(track)
}
```

---

## Phase 4.2: Parameter Tuning & Optimization

### Objective

Systematically explore parameter space to optimise track quality metrics.

### 4.2.1: Tuning Workflow

```
1. Define parameter grid
2. For each parameter combination:
   a. Run analysis on reference PCAP
   b. Compare to baseline run
   c. Detect splits/merges
   d. Compute quality metrics
3. Analyze results to find optimal parameters
4. Validate on held-out PCAPs
```

### 4.2.2: Parameter Grid Search

```go
// cmd/tools/param-sweep/main.go

type ParameterGrid struct {
    BackgroundNoiseRelative []float32 `json:"background_noise_relative"`
    BackgroundCloseness     []float32 `json:"background_closeness"`
    ClusteringEps           []float32 `json:"clustering_eps"`
    ClusteringMinPts        []int     `json:"clustering_min_pts"`
    TrackingGatingDistance  []float32 `json:"tracking_gating_distance"`
    TrackingHitsToConfirm   []int     `json:"tracking_hits_to_confirm"`
}

func (g *ParameterGrid) Combinations() []RunParams {
    // Generate all parameter combinations
}

// SweepResult stores results for one parameter combination
type SweepResult struct {
    RunID           string      `json:"run_id"`
    Params          RunParams   `json:"params"`
    BaselineRunID   string      `json:"baseline_run_id"`
    Comparison      *RunComparison `json:"comparison"`
    QualityMetrics  *QualityMetrics `json:"quality_metrics"`
}

type QualityMetrics struct {
    TrackCount          int     `json:"track_count"`
    ConfirmedTrackCount int     `json:"confirmed_track_count"`
    SplitCount          int     `json:"split_count"`
    MergeCount          int     `json:"merge_count"`
    NoiseTrackCount     int     `json:"noise_track_count"`
    AvgTrackDuration    float64 `json:"avg_track_duration"`
    AvgObservationsPerTrack float64 `json:"avg_observations_per_track"`
    // Classification metrics if labels available
    ClassificationAccuracy float64 `json:"classification_accuracy,omitempty"`
}
```

### 4.2.3: Optimization Objective

```go
// Define objective function for parameter optimisation
func ComputeObjective(metrics *QualityMetrics, comparison *RunComparison) float64 {
    // Goal: Maximize confirmed tracks, minimize splits/merges/noise
    //
    // objective = w1 * confirmed_tracks
    //           - w2 * split_count
    //           - w3 * merge_count
    //           - w4 * noise_tracks
    //           + w5 * avg_track_duration

    w1, w2, w3, w4, w5 := 1.0, 5.0, 5.0, 2.0, 0.1

    return w1*float64(metrics.ConfirmedTrackCount) -
           w2*float64(comparison.SplitCount()) -
           w3*float64(comparison.MergeCount()) -
           w4*float64(metrics.NoiseTrackCount) +
           w5*metrics.AvgTrackDuration
}
```

### 4.2.4: Interactive Tuning UI

```svelte
<!-- web/src/routes/lidar/tuning/+page.svelte -->
<script lang="ts">
  // Parameter sliders with live preview
  // Run comparison visualization
  // Quality metric charts
  // Parameter recommendation engine
</script>
```

---

## Phase 4.3: Production Deployment

### Objective

Deploy the complete ML pipeline for production use.

### 4.3.1: Deployment Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                      Edge Node (Raspberry Pi)                    │
│                                                                  │
│  [UDP:2369] → [LIDAR Pipeline] → [Local SQLite] → [REST API]   │
│                      ↓                   ↓                       │
│                [ML Classifier]    [Training Data]               │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
                                 ↓
                        [Data Consolidation]
                                 ↓
┌─────────────────────────────────────────────────────────────────┐
│                      Central Server                              │
│                                                                  │
│  [Consolidated DB] → [Labeling UI] → [Model Training]          │
│                           ↓              ↓                       │
│                    [Labeled Tracks] → [New Model]               │
│                                           ↓                      │
│                              [Model Distribution]                │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### 4.3.2: Model Update Flow

```
1. Collect labeled tracks from labeling UI
2. Train new model version
3. Evaluate on validation set
4. If metrics improve:
   a. Version the model (v1.1, v1.2, etc.)
   b. Distribute to edge nodes
   c. Update classification_model field in new tracks
5. Monitor production metrics
6. Collect new edge cases for labeling
7. Repeat
```

---

## Data Flow Summary

```
┌──────────────────────────────────────────────────────────────────────────┐
│                         COMPLETE ML PIPELINE                              │
├──────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌─────────┐    ┌───────────┐    ┌──────────┐    ┌──────────────────┐   │
│  │  PCAP   │───→│   Parse   │───→│  Frame   │───→│    Background    │   │
│  │  Live   │    │           │    │ Builder  │    │    Subtraction   │   │
│  └─────────┘    └───────────┘    └──────────┘    └────────┬─────────┘   │
│                                                            │             │
│                                                            ▼             │
│  ┌─────────────────┐    ┌──────────┐    ┌──────────────────────────┐    │
│  │   Foreground    │◄───│  Mask    │◄───│   ProcessFramePolarWith  │    │
│  │     Points      │    │          │    │         Mask()           │    │
│  └────────┬────────┘    └──────────┘    └──────────────────────────┘    │
│           │                                                              │
│           ▼                                                              │
│  ┌─────────────────┐    ┌──────────────────┐    ┌──────────────────┐    │
│  │  TransformTo    │───→│     DBSCAN       │───→│     Tracker      │    │
│  │    World()      │    │   Clustering     │    │    Update()      │    │
│  └─────────────────┘    └──────────────────┘    └────────┬─────────┘    │
│                                                           │              │
│                                                           ▼              │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                     ANALYSIS RUN (Phase 4.0)                     │    │
│  │                                                                   │    │
│  │   ┌─────────────┐    ┌────────────────┐    ┌─────────────────┐   │    │
│  │   │  params_json│    │  lidar_run_    │    │ Split/Merge     │   │    │
│  │   │   (all cfg) │    │    tracks      │    │   Detection     │   │    │
│  │   └─────────────┘    └────────────────┘    └─────────────────┘   │    │
│  │                                                                   │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                    │                                     │
│                                    ▼                                     │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                    LABELING UI (Phase 4.1)                       │    │
│  │                                                                   │    │
│  │   ┌─────────────┐    ┌────────────────┐    ┌─────────────────┐   │    │
│  │   │   Track     │    │    Label       │    │   Quality       │   │    │
│  │   │  Browser    │───→│   Assignment   │───→│   Marking       │   │    │
│  │   └─────────────┘    └────────────────┘    └─────────────────┘   │    │
│  │                                                                   │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                    │                                     │
│                                    ▼                                     │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                  ML TRAINING (Phase 4.2)                         │    │
│  │                                                                   │    │
│  │   ┌─────────────┐    ┌────────────────┐    ┌─────────────────┐   │    │
│  │   │  Feature    │    │    Model       │    │   Deployed      │   │    │
│  │   │ Extraction  │───→│   Training     │───→│    Model        │   │    │
│  │   └─────────────┘    └────────────────┘    └─────────────────┘   │    │
│  │                                                                   │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                    │                                     │
│                                    ▼                                     │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │               PARAMETER TUNING (Phase 4.2)                       │    │
│  │                                                                   │    │
│  │   ┌─────────────┐    ┌────────────────┐    ┌─────────────────┐   │    │
│  │   │  Parameter  │    │   Run          │    │   Optimal       │   │    │
│  │   │   Grid      │───→│  Comparison    │───→│  Parameters     │   │    │
│  │   └─────────────┘    └────────────────┘    └─────────────────┘   │    │
│  │                                                                   │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                          │
└──────────────────────────────────────────────────────────────────────────┘
```

---

## Implementation Priority

| Phase                           | Priority      | Effort  | Dependencies             |
| ------------------------------- | ------------- | ------- | ------------------------ |
| 3.7 Analysis Run Infrastructure | ✅ Complete   | -       | None                     |
| 4.0 Track Labeling UI           | P0 - Critical | 2 weeks | Phase 3.7                |
| 4.1 ML Classifier Training      | P1 - High     | 1 week  | Phase 4.0 (needs labels) |
| 4.2 Parameter Tuning            | P1 - High     | 1 week  | Phase 3.7                |
| 4.3 Production Deployment       | P2 - Medium   | 1 week  | Phases 4.1, 4.2          |

**Recommended Order:**

1. ✅ Phase 3.7 (COMPLETED - infrastructure for all other phases)
2. Phase 4.0 (critical for getting labels)
3. Phase 4.2 (can be done in parallel with labeling)
4. Phase 4.1 (requires labeled data from 4.0)
5. Phase 4.3 (final deployment)

---

**Document Status:** Phase 3.7 Implemented
**Next Action:** Implement Phase 4.0 (Track Labeling UI)
**Last Updated:** December 1, 2025
