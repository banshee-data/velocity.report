# Auto-Tuning Plan

**Status**: Phases 1–2 Implemented (February 2026)

**Implementation Files:**

| Component               | File                                               |
| ----------------------- | -------------------------------------------------- |
| AutoTuner               | `internal/lidar/sweep/auto.go`                     |
| Runner + Settle Mode    | `internal/lidar/sweep/runner.go`                   |
| Multi-objective Scoring | `internal/lidar/sweep/scoring.go`                  |
| Sweep Dashboard         | `internal/lidar/monitor/html/sweep_dashboard.html` |
| Sampler                 | `internal/lidar/sweep/sampler.go`                  |

**What's Implemented:**

- ✅ Phase 1 — Iterative grid narrowing with configurable rounds, values_per_param, top_k
- ✅ Phase 2 — Multi-objective scoring (acceptance rate, misalignment, alignment, nonzero cells, active tracks)
- ✅ Dashboard integration with auto-tune toggle, round progress, recommendation card
- ✅ Settle mode: `once` (settle once, keep grid) and `per_combo` (reset per combination)
- ✅ PARAM_SCHEMA with sane default ranges for all numeric parameters

**Still Planned:**

- Phase 3 — Bayesian Optimisation (GP-based acquisition function) — deferred until need is demonstrated
- Label-aware auto-tuning using ground truth — see [`track-labeling-auto-aware-tuning.md`](track-labeling-auto-aware-tuning.md)

---

## Problem Statement

The LiDAR parameter sweep dashboard allows operators to manually configure parameter combinations, run sweeps, and analyse results. Finding optimal tuning values requires:

1. Guessing reasonable ranges for each parameter
2. Running a full sweep across the cartesian product
3. Manually inspecting results to identify the best region
4. Narrowing ranges and repeating

This process is slow, tedious, and doesn't scale when tuning more than 2-3 parameters simultaneously. We need an automated system that can iteratively search the parameter space and converge on optimal values.

## Current Behaviour

### Sweep Runner (`internal/lidar/sweep/runner.go`)

- Accepts a `SweepRequest` with a list of `SweepParam` entries (name, type, start/end/step or explicit values)
- Computes cartesian product of all parameter combinations
- For each combination: resets grid/tracker, sets tuning params, waits for settle, samples N iterations
- Records per-combination statistics: acceptance rate, nonzero cells, active tracks, alignment, misalignment
- Reports results via `/api/lidar/sweep/status`

### Limitations

- Single pass only: no ability to refine or narrow based on results
- Cartesian product scales exponentially: 3 params x 5 values = 125 combinations
- No concept of an objective function or optimisation target
- No early stopping for clearly poor parameter regions

## Proposed Solution

### Phase 1: Iterative Grid Narrowing

The simplest approach that delivers immediate value. Runs multiple rounds of grid search, narrowing the parameter space after each round.

**Algorithm:**

1. **Round 1 (Coarse):** Run a coarse sweep across the full parameter space (e.g. 3-5 values per param)
2. **Rank results** by a configurable objective (default: maximise acceptance rate, minimise misalignment)
3. **Narrow bounds:** Take the top K results, compute the bounding box of their parameter values, add 1-step margin
4. **Round 2+ (Fine):** Subdivide the narrowed range into N steps and sweep again
5. **Repeat** until the improvement delta falls below a threshold, or a round budget is exhausted

**Example with 2 parameters:**

```
Round 1: noise_relative [0.01, 0.02, 0.04, 0.06, 0.08]
         closeness_multiplier [2.0, 4.0, 8.0, 12.0, 16.0]
         → 25 combinations, best region: noise ~0.02-0.04, closeness ~4.0-8.0

Round 2: noise_relative [0.02, 0.025, 0.03, 0.035, 0.04]
         closeness_multiplier [4.0, 5.0, 6.0, 7.0, 8.0]
         → 25 combinations, best: noise ~0.025-0.035, closeness ~5.0-7.0

Round 3: noise_relative [0.025, 0.0275, 0.03, 0.0325, 0.035]
         closeness_multiplier [5.0, 5.5, 6.0, 6.5, 7.0]
         → 25 combinations → final recommendation
```

Total: 75 combinations vs 125+ for a fine single-pass sweep, with better convergence.

### Phase 2: Multi-Objective Pareto Front

Extend Phase 1 to handle multiple competing objectives:

- **Maximise:** acceptance rate, nonzero cells
- **Minimise:** misalignment ratio, alignment degree

Instead of a single "best", return a Pareto front of non-dominated solutions. Let the operator choose the trade-off point.

**Ranking function:**

```
score = w1 * accept_rate - w2 * misalignment_ratio - w3 * alignment_deg + w4 * log(nonzero_cells)
```

Default weights: `w1=1.0, w2=0.5, w3=0.01, w4=0.1`. Configurable via API.

### Phase 3: Bayesian Optimisation (Future)

Replace grid search with Gaussian Process (GP) based Bayesian optimisation:

- Builds a surrogate model of the objective from observed results
- Uses an acquisition function (Expected Improvement) to select the next point to evaluate
- Handles noisy observations naturally (multiple iterations per combination)
- Scales much better to higher dimensions (5-10 parameters)

This would require a Go GP library or calling out to a Python subprocess. Deferred until the need is demonstrated.

## Architecture

### New Components

```
AutoTuner
├── wraps sweep.Runner
├── manages multiple rounds
├── narrows parameter bounds between rounds
└── reports per-round and overall progress

API: POST /api/lidar/sweep/auto
├── request: AutoTuneRequest
│   ├── params: []SweepParam (with initial bounds)
│   ├── max_rounds: int (default 3)
│   ├── values_per_param: int (default 5, values per param per round)
│   ├── top_k: int (default 5, best results to narrow from)
│   ├── objective: string (default "acceptance")
│   ├── weights: map[string]float64 (optional)
│   ├── iterations: int (samples per combination)
│   ├── settle_time: string
│   └── ... (same as SweepRequest for data source, seed, etc.)
└── response: standard sweep status with additional fields:
    ├── round: int (current round number)
    ├── total_rounds: int
    ├── round_results: []RoundSummary
    └── recommendation: map[string]interface{} (best param values found)
```

### Files

| File                                               | Purpose                                                  |
| -------------------------------------------------- | -------------------------------------------------------- |
| `internal/lidar/sweep/auto.go`                     | `AutoTuner` struct, round orchestration, narrowing logic |
| `internal/lidar/sweep/objective.go`                | Objective functions and scoring                          |
| `internal/lidar/sweep/auto_test.go`                | Unit tests for narrowing and scoring                     |
| `internal/lidar/monitor/sweep_handlers.go`         | Add `/api/lidar/sweep/auto` handler                      |
| `internal/lidar/monitor/html/sweep_dashboard.html` | Auto-tune button, round progress, recommendation display |

### AutoTuner Lifecycle

```
1. Receive AutoTuneRequest
2. For each round:
   a. Generate parameter grid (initial bounds or narrowed bounds)
   b. Create SweepRequest from grid
   c. Call runner.StartWithRequest()
   d. Wait for completion
   e. Score results using objective function
   f. Select top K results
   g. Compute narrowed bounds from top K
   h. Store round summary
3. Return final recommendation (best overall result)
```

### Narrowing Logic

For each parameter in the top K results:

```go
func narrowBounds(topK []ComboResult, paramName string, margin float64) (start, end float64) {
    min, max := math.Inf(1), math.Inf(-1)
    for _, r := range topK {
        v := r.ParamValues[paramName].(float64)
        if v < min { min = v }
        if v > max { max = v }
    }
    step := (max - min) / float64(valuesPerParam-1)
    return min - step*margin, max + step*margin
}
```

### Dashboard Integration

#### Web Interface

The auto-tuning feature will be accessed through the existing sweep dashboard at `http://localhost:8080/lidar/sweep`:

1. **Auto-Tune Tab**: Add a new tab alongside the existing "Manual Sweep" tab
2. **Parameter Configuration**: Same parameter selection UI as manual sweep, but with simplified bounds entry (min/max instead of step values)
3. **Auto-Tune Settings Panel**:
   - **Max Rounds** (default: 3): Number of refinement rounds
   - **Values Per Param** (default: 5): Grid density for each round
   - **Top K** (default: 5): Number of best results to narrow from
   - **Objective Function** dropdown: "Acceptance Rate", "Misalignment", "Weighted" (custom)
   - **Custom Weights** (shown when "Weighted" selected): Sliders/inputs for acceptance, misalignment, alignment, nonzero cells
4. **Data Source Configuration**: Same as manual sweep (Live/PCAP/Scenario)
5. **Start Auto-Tune** button

#### Scenario Specification

Auto-tuning can be run against:

1. **Live mode**: Uses real-time LiDAR data from the sensor
2. **PCAP replay**: Replays a single PCAP file repeatedly for each parameter combination
3. **Scenario mode**: Runs a sequence of PCAP files representing different traffic conditions

**Scenario JSON Format**:

```json
{
  "name": "Multi-condition sweep",
  "description": "Test parameters across morning rush, midday, and evening conditions",
  "segments": [
    {
      "pcap_path": "/data/pcaps/morning_rush_0730-0800.pcap",
      "iterations": 5,
      "settle_time": "30s"
    },
    {
      "pcap_path": "/data/pcaps/midday_light_1200-1230.pcap",
      "iterations": 5,
      "settle_time": "20s"
    },
    {
      "pcap_path": "/data/pcaps/evening_rush_1730-1800.pcap",
      "iterations": 5,
      "settle_time": "30s"
    }
  ],
  "aggregation": "weighted_average",
  "weights": [0.35, 0.3, 0.35]
}
```

**Scenario Fields**:

- **name**: Human-readable scenario name (displayed in dashboard)
- **description**: Optional description of what the scenario tests
- **segments**: Array of PCAP files to replay sequentially for each parameter combination
  - **pcap_path**: Absolute path to PCAP file
  - **iterations**: Number of samples to take from this segment
  - **settle_time**: How long to wait for background grid to settle before sampling
- **aggregation**: How to combine results across segments
  - `"weighted_average"`: Weighted mean of segment scores (default)
  - `"min"`: Use the worst-performing segment score (conservative)
  - `"max"`: Use the best-performing segment score (optimistic)
- **weights**: Weight for each segment (must sum to 1.0), used with weighted_average

**Dashboard Scenario Selection**:

- Add a "Scenario" radio option alongside "Live" and "PCAP" in the data source selector
- When "Scenario" is selected, show a file picker or dropdown to select a saved scenario JSON file
- Display scenario details (name, description, number of segments) after selection
- Store scenarios in `config/scenarios/` directory for easy access

#### Progress Visualization

During auto-tuning execution, the dashboard displays:

1. **Overall Progress**: "Round 2/3 — 15/25 combinations"
2. **Round Summary Cards**: Shows narrowed bounds for current round
3. **Live Charts**: Update as each combination completes (same charts as manual sweep)
4. **Round History**: Collapsible cards showing previous round results and bounds
5. **Best So Far**: Card showing current best parameter values and score

#### Completion and Recommendation

When auto-tuning completes:

1. **Recommendation Card** displays:
   - Optimal parameter values
   - Final score
   - Individual metrics (acceptance rate, misalignment, etc.)
   - "Apply to Config" button to update running system
   - "Export JSON" button to save as tuning config file
2. **Convergence Chart**: Line chart showing objective score improvement across rounds
3. **Parameter Bounds History**: Visual showing how bounds narrowed each round

### Scenario Mode Integration

When auto-tuning runs in scenario mode:

1. For each parameter combination:
   - Load each segment's PCAP file sequentially
   - Reset grid and tracker between segments
   - Apply settle time and sample N iterations from each segment
   - Record per-segment metrics
2. Aggregate segment results using the specified method
3. Use aggregated score for ranking and narrowing
4. Display per-segment breakdown in results table

This enables robust parameter optimization across diverse traffic conditions without manual multi-run orchestration.

## API Design

### Request

```json
POST /api/lidar/sweep/auto
{
  "params": [
    { "name": "noise_relative", "type": "float64", "start": 0.01, "end": 0.1, "step": 0.01 },
    { "name": "closeness_multiplier", "type": "float64", "start": 2.0, "end": 16.0, "step": 1.0 }
  ],
  "max_rounds": 3,
  "values_per_param": 5,
  "top_k": 5,
  "objective": "weighted",
  "weights": {
    "acceptance": 1.0,
    "misalignment": -0.5
  },
  "iterations": 10,
  "settle_time": "5s",
  "interval": "2s",
  "seed": "true",
  "data_source": "live"
}
```

### Status Response

```json
GET /api/lidar/sweep/status
{
  "status": "running",
  "mode": "auto",
  "round": 2,
  "total_rounds": 3,
  "completed_combos": 15,
  "total_combos": 25,
  "round_results": [
    {
      "round": 1,
      "bounds": { "noise_relative": [0.01, 0.1], "closeness_multiplier": [2.0, 16.0] },
      "best_score": 0.87,
      "best_params": { "noise_relative": 0.04, "closeness_multiplier": 8.0 }
    }
  ],
  "results": [ ... ],
  "recommendation": null
}
```

### Completion

```json
{
  "status": "complete",
  "mode": "auto",
  "recommendation": {
    "noise_relative": 0.035,
    "closeness_multiplier": 6.5,
    "score": 0.92,
    "acceptance_rate": 0.95,
    "misalignment_ratio": 0.05
  }
}
```

## Verification

1. Unit tests for narrowing logic with known parameter sets
2. Unit tests for objective function scoring
3. Integration test: run auto-tune with a mock runner (fast iterations) and verify convergence
4. Manual test: run auto-tune against a live sensor or PCAP and verify the recommendation produces better results than the initial coarse grid center
