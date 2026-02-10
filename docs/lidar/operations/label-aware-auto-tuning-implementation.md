# Phase 5 Implementation Notes: Label-Aware Auto-Tuning

**Status**: ✅ Complete (with Phase 2.5 deferred)
**Date**: February 2026
**Implemented by**: Agent Hadaly

## Summary

Phase 5 of the Track Labelling, Ground Truth Evaluation & Label-Aware Auto-Tuning design has been implemented. This phase adds support for ground truth-based parameter optimisation in the auto-tuning system, allowing the tuner to optimise parameters based on labelled reference tracks rather than just acceptance metrics.

## What Was Implemented

### 1. AutoTuneRequest Extensions (`internal/lidar/sweep/auto.go`)

Added two new fields to `AutoTuneRequest`:

```go
SceneID            string               `json:"scene_id,omitempty"`
GroundTruthWeights *GroundTruthWeights `json:"ground_truth_weights,omitempty"`
```

- `SceneID`: When set, enables ground truth evaluation mode
- `GroundTruthWeights`: Configurable weights for the composite ground truth score

### 2. Ground Truth Weights (`internal/lidar/sweep/auto.go`)

Added `GroundTruthWeights` struct to the sweep package (avoiding circular imports):

```go
type GroundTruthWeights struct {
    DetectionRate     float64  // w1: Weight for detection rate
    Fragmentation     float64  // w2: Penalty for track splits
    FalsePositives    float64  // w3: Penalty for unmatched tracks
    VelocityCoverage  float64  // w4: Bonus for velocity data
    QualityPremium    float64  // w5: Bonus for "perfect" tracks
    TruncationRate    float64  // w6: Penalty for truncated tracks
    VelocityNoiseRate float64  // w7: Penalty for noisy velocity
    StoppedRecovery   float64  // w8: Bonus for stopped recovery
}
```

Default weights from design doc: `w1=1.0, w2=5.0, w3=2.0, w4=0.5, w5=0.3, w6=0.4, w7=0.4, w8=0.2`

### 3. AutoTuner Enhancements

Added fields and methods to `AutoTuner`:

```go
type AutoTuner struct {
    // ... existing fields ...
    groundTruthScorer func(sceneID, candidateRunID string) (float64, error)
    sceneStore        SceneStoreSaver
}
```

New methods:

- `SetGroundTruthScorer(scorer func(...) (float64, error))` - Configures the scorer
- `SetSceneStore(store SceneStoreSaver)` - Configures the scene store for saving optimal params

### 4. Objective Validation

Added validation in `AutoTuner.start()`:

- `ground_truth` objective requires `scene_id` to be set
- `ground_truth` objective requires a scorer to be configured
- Default ground truth weights are applied if none provided

### 5. Optimal Parameter Persistence

When auto-tuning completes with a `scene_id` set, the best parameters are automatically saved to the scene's `optimal_params_json` field via `SceneStore.SetOptimalParams()`.

### 6. Interface Definitions

Added `SceneStoreSaver` interface to avoid circular imports:

```go
type SceneStoreSaver interface {
    SetOptimalParams(sceneID string, paramsJSON json.RawMessage) error
}
```

The `SceneStore` in `internal/lidar/scene_store.go` already implements this interface.

### 7. Test Coverage

Added comprehensive tests in `internal/lidar/sweep/auto_test.go`:

- `TestGroundTruthObjectiveValidation` - Validates error handling
- `TestDefaultGroundTruthWeights` - Verifies default weight values
- `TestAutoTuneRequestGroundTruthFields` - Tests JSON parsing

All existing tests continue to pass.

## What Is Deferred (Phase 2.5)

The actual ground truth scoring during auto-tuning is **not yet active** because it depends on Phase 2.5:

- Each sweep combination needs to create an analysis run
- Run tracks must be persisted to the database
- The ground truth evaluator can then compare candidate runs against the reference run

### Current Behavior

When `objective: "ground_truth"` is selected:

1. Validation passes if `scene_id` and scorer are configured ✅
2. Auto-tuner runs the sweep with standard objective scoring ⚠️
3. A warning is logged: "Ground truth objective selected but Phase 2.5 not yet implemented"
4. Optimal params are saved to the scene at completion ✅

The commented-out code in `run()` shows the intended implementation once Phase 2.5 is complete.

## Architecture Notes

### Avoiding Circular Imports

The sweep package depends on the monitor package (for the client), which depends on the lidar package. To avoid a circular dependency:

1. `GroundTruthWeights` is defined in the sweep package (not lidar)
2. `SceneStoreSaver` interface is defined in the sweep package
3. The ground truth scorer is injected as a function, not imported

This allows the monitor layer to:

- Create an `AutoTuner` with `NewAutoTuner(runner)`
- Wire up the scorer: `autoTuner.SetGroundTruthScorer(func(sceneID, runID string) (float64, error) { ... })`
- Wire up the scene store: `autoTuner.SetSceneStore(sceneStore)`

### Integration Points

When Phase 2.5 is complete, the monitor layer will:

1. Configure the scorer function to call `GroundTruthEvaluator.Evaluate(referenceRunID, candidateRunID)`
2. Ensure each sweep combination creates an analysis run with persisted tracks
3. Uncomment the ground truth scoring logic in `AutoTuner.run()`

## Verification

All tests pass:

```bash
$ go test ./internal/lidar/sweep/ -count=1
ok  	github.com/banshee-data/velocity.report/internal/lidar/sweep	6.469s
```

All code is properly formatted:

```bash
$ make lint-go
Checking Go formatting (gofmt -l)...
OK
```

## Future Work

### Phase 2.5: Analysis Run Creation Per Combo

Before ground truth scoring can be activated:

1. Modify `Runner` to create analysis runs per combination during PCAP replay
2. Store `run_id` in `ComboResult` for ground truth evaluation
3. Implement state reset between combinations:
   - `ClearTracks(sensorID)`
   - `ResetBackgroundModel(sensorID)`
   - PCAP replay from start for each combo

### Phase 5.4-5.6: Dashboard Integration

Future UI enhancements (not implemented in this phase):

- Sweep dashboard: show "Ground Truth" objective option when scene has reference run
- Display ground truth score components in results table
- "Apply Scene Params" button to load optimal params for a scene

These are HTML/JavaScript changes that don't affect the backend functionality.

## Related Files

Modified:

- `internal/lidar/sweep/auto.go` - Core implementation
- `internal/lidar/sweep/auto_test.go` - Test coverage

Existing (no changes needed):

- `internal/lidar/scene_store.go` - Already has `SetOptimalParams()`
- `internal/lidar/ground_truth.go` - Evaluator ready for Phase 2.5 integration

## Design Document Reference

This implementation follows the design specification in:
`docs/lidar/future/track-labeling-auto-aware-tuning.md` (Phase 5)
