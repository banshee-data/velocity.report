# Phase 5 Usage Examples

This document shows how Phase 5 (Label-Aware Auto-Tuning) is used with ground truth evaluation.

## Current State (Phase 5 Complete)

The auto-tuning system accepts ground truth configuration and actively scores combinations against labelled reference tracks when `objective` is set to `"ground_truth"`.

### API Request Example

```json
POST /api/lidar/sweep/auto

{
  "params": [
    {
      "name": "noise_relative",
      "type": "float64",
      "start": 0.01,
      "end": 0.1
    },
    {
      "name": "closeness_multiplier",
      "type": "float64",
      "start": 2.0,
      "end": 10.0
    }
  ],
  "max_rounds": 3,
  "values_per_param": 5,
  "top_k": 5,
  "objective": "ground_truth",
  "scene_id": "scene-123",
  "ground_truth_weights": {
    "detection_rate": 1.0,
    "fragmentation": 5.0,
    "false_positives": 2.0,
    "velocity_coverage": 0.5,
    "quality_premium": 0.3,
    "truncation_rate": 0.4,
    "velocity_noise_rate": 0.4,
    "stopped_recovery": 0.2
  },
  "iterations": 10,
  "settle_time": "5s",
  "interval": "2s",
  "data_source": "pcap",
  "pcap_file": "urban-intersection.pcap",
  "pcap_start_secs": 0,
  "pcap_duration_secs": 60
}
```

### Current Behaviour

When you make this request:

1. ✅ Validation passes (scene_id set, scorer configured)
2. ✅ Each combination creates an analysis run
3. ✅ Run tracks are persisted to database
4. ✅ Ground truth evaluator scores each run against reference tracks
5. ✅ Auto-tuner uses composite ground truth score to rank combinations
6. ✅ Optimal params saved to scene at completion

## Monitor Layer Integration (Post Phase 2.5)

Here's how the monitor layer will wire up the ground truth scorer:

```go
// In internal/lidar/monitor/webserver.go or sweep_handlers.go

// Create evaluator with scene's reference run
sceneStore := lidar.NewSceneStore(db)
analysisRunStore := lidar.NewAnalysisRunStore(db)

scene, err := sceneStore.GetScene(req.SceneID)
if err != nil {
    return err
}

if scene.ReferenceRunID == "" {
    return fmt.Errorf("scene has no reference run for ground truth evaluation")
}

// Get ground truth weights (use defaults if not provided)
weights := req.GroundTruthWeights
if weights == nil {
    defaultWeights := sweep.DefaultGroundTruthWeights()
    weights = &defaultWeights
}

// Create evaluator (convert sweep weights to lidar weights)
lidarWeights := lidar.GroundTruthWeights{
    DetectionRate:     weights.DetectionRate,
    Fragmentation:     weights.Fragmentation,
    FalsePositives:    weights.FalsePositives,
    VelocityCoverage:  weights.VelocityCoverage,
    QualityPremium:    weights.QualityPremium,
    TruncationRate:    weights.TruncationRate,
    VelocityNoiseRate: weights.VelocityNoiseRate,
    StoppedRecovery:   weights.StoppedRecovery,
}

evaluator := lidar.NewGroundTruthEvaluator(analysisRunStore, lidarWeights)

// Wire up scorer function
autoTuner.SetGroundTruthScorer(func(sceneID, candidateRunID string) (float64, error) {
    score, err := evaluator.Evaluate(scene.ReferenceRunID, candidateRunID)
    if err != nil {
        return 0.0, err
    }
    return score.CompositeScore, nil
})

// Wire up scene store for optimal params persistence
autoTuner.SetSceneStore(sceneStore)

// Start auto-tuning
err = autoTuner.Start(ctx, req)
```

## Ground Truth Score Components

The composite score uses this formula:

```
composite_score =
    w1 × detection_rate
  + w4 × velocity_coverage
  + w5 × quality_premium
  + w8 × stopped_recovery
  - w2 × fragmentation
  - w3 × false_positives
  - w6 × truncation_rate
  - w7 × velocity_noise_rate
```

Each component:

- **detection_rate**: Fraction of labelled "good\_\*" reference tracks matched (0-1)
- **fragmentation**: Fraction of reference tracks split into multiple candidates
- **false_positives**: Fraction of candidate tracks not matching any reference
- **velocity_coverage**: Fraction of matched tracks with velocity data
- **quality_premium**: Fraction of matched tracks with "perfect" quality label
- **truncation_rate**: Fraction of matched tracks with "truncated" quality label
- **velocity_noise_rate**: Fraction of matched tracks with "noisy_velocity" quality label
- **stopped_recovery**: Fraction of stopped tracks with "stopped_recovered" quality label

## Workflow Example

### Step 1: Create Scene and Reference Run

```bash
# Create scene
POST /api/lidar/scenes
{
  "sensor_id": "lidar-01",
  "pcap_file": "urban-intersection.pcap",
  "description": "Busy intersection with mixed traffic"
}
# Returns: {"scene_id": "scene-123"}

# Replay PCAP with current params to create initial run
POST /api/lidar/scenes/scene-123/replay
{
  "params": { "noise_relative": 0.04, "closeness_multiplier": 5.0, ... }
}
# Returns: {"run_id": "run-456"}
```

### Step 2: Label Tracks

```bash
# Label each track in the run
PUT /api/lidar/runs/run-456/tracks/track-001/label
{
  "user_label": "good_vehicle",
  "quality_label": "perfect"
}

PUT /api/lidar/runs/run-456/tracks/track-002/label
{
  "user_label": "good_vehicle",
  "quality_label": "truncated"
}

PUT /api/lidar/runs/run-456/tracks/track-003/label
{
  "user_label": "noise_flora"
}
# ... continue for all tracks
```

### Step 3: Set Reference Run

```bash
PUT /api/lidar/scenes/scene-123
{
  "reference_run_id": "run-456"
}
```

### Step 4: Run Auto-Tuning

```bash
POST /api/lidar/sweep/auto
{
  "objective": "ground_truth",
  "scene_id": "scene-123",
  "params": [...],
  "max_rounds": 3,
  ...
}
```

### Step 5: Retrieve Optimal Params

```bash
GET /api/lidar/scenes/scene-123
# Returns:
{
  "scene_id": "scene-123",
  "optimal_params_json": {
    "noise_relative": 0.035,
    "closeness_multiplier": 6.2,
    ...
  }
}
```

### Step 6: Apply Optimal Params

```bash
# Apply to live system
POST /api/lidar/params
{
  "noise_relative": 0.035,
  "closeness_multiplier": 6.2,
  ...
}
```

## Testing Ground Truth Mode

You can test the validation and parameter persistence today:

```go
// Test in Go
func TestAutoTuneGroundTruthMode(t *testing.T) {
    runner := &sweep.Runner{}
    tuner := sweep.NewAutoTuner(runner)

    // Mock scorer
    tuner.SetGroundTruthScorer(func(sceneID, runID string) (float64, error) {
        return 0.85, nil
    })

    // Mock scene store
    mockStore := &mockSceneStore{}
    tuner.SetSceneStore(mockStore)

    // Create request
    req := sweep.AutoTuneRequest{
        Objective: "ground_truth",
        SceneID:   "test-scene",
        Params: []sweep.SweepParam{
            {Name: "noise_relative", Type: "float64", Start: 0.01, End: 0.1},
        },
        Iterations:       10,
        SettleTime:      "5s",
        Interval:        "2s",
        DataSource:      "pcap",
        PCAPFile:        "test.pcap",
        PCAPDurationSecs: 30,
    }

    // Start (validation will pass, scoring will fall back)
    err := tuner.Start(context.Background(), req)
    if err != nil {
        t.Fatalf("Expected no error, got %v", err)
    }

    // Wait for completion and verify optimal params were saved
    // (actual scoring deferred to Phase 2.5)
}
```

## Dashboard Integration (Phase 5.4-5.6)

The sweep dashboard supports ground truth mode:

### Scene Selection

- Dropdown to select scene (fetches `/api/lidar/scenes`)
- Shows reference run status (labelled/unlabelled)
- "Apply Optimal Params" button

### Objective Selection

- Radio buttons: Acceptance / Weighted / Ground Truth
- Ground Truth option only enabled if scene has reference run
- Shows ground truth weight configuration

### Results Display

- Detection rate by class (vehicle/pedestrian/other)
- Fragmentation score
- False positive rate
- Quality metrics (velocity coverage, quality premium, truncation, noise)
- Composite score with breakdown

## Summary

Phase 5 is **complete and functional** for:

- ✅ Request validation
- ✅ Weight configuration (per-request customisable)
- ✅ Ground truth scoring against labelled reference tracks
- ✅ Optimal parameter persistence
- ✅ Integration with sweep and auto-tune systems
