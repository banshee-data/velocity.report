# RLHF Sweep Mode — Human-in-the-Loop Parameter Tuning

## Summary

A new sweep mode alongside Manual Sweep and Auto-Tune that integrates human
labelling into the tuning loop. Each round consists of: (1) run the tracker with
the current best parameters and capture an analysis run, (2) present tracks to a
human for labelling, (3) use the labelled ground truth to score a parameter sweep,
(4) narrow bounds and repeat. The human's labels directly drive the objective
function, closing the feedback loop between subjective track quality and automated
optimisation.

## Motivation

| Existing Mode                          | Strength                                              | Weakness                                                                                                                            |
| -------------------------------------- | ----------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- |
| **Manual Sweep**                       | Full control over param grid and metric visualisation | No automated narrowing; human reads CSV manually                                                                                    |
| **Auto-Tune**                          | Automated multi-round narrowing with weighted metrics | Metrics are proxy measures (empty_box_ratio, fragmentation) — they never test whether a real vehicle was actually tracked correctly |
| **Ground Truth** (hidden, scene-based) | Uses labelled tracks as the objective                 | Requires a pre-labelled reference run. Can't adapt labels round-by-round as params change                                           |

RLHF mode fills the gap: the human evaluates track quality _after_ each parameter
change, so labels always reflect the _current_ parameter regime. This avoids the
stale-reference problem and makes subjective judgement (split? noise? truncated?)
part of the optimisation loop.

## User Flow

```
┌─────────────────────────────────────────────────────────────┐
│  1. CONFIGURE                                               │
│     Select scene, number of rounds, round durations         │
│     Optionally set param bounds and objective weights       │
│     Click "Start RLHF"                                      │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  2. REFERENCE RUN  (automated)                              │
│     System replays PCAP with current best params            │
│     An analysis run is created and tracks are captured      │
│     Status: "awaiting_labels" — human notified              │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  3. HUMAN LABELS  (manual, time-boxed)                      │
│     Human opens the Runs/Tracks page and labels tracks      │
│     Detection: vehicle, pedestrian, noise, split, etc.      │
│     Quality: perfect, good, truncated, noisy_velocity       │
│     Dashboard shows labelling progress + countdown          │
│     When done (or round duration elapsed): click "Continue" │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  4. SWEEP ROUND  (automated, may be long-running)           │
│     Auto-tuner runs a parameter sweep using the just-       │
│     labelled run as the ground truth reference              │
│     Each combo creates a candidate run, scored via          │
│     EvaluateGroundTruth against the labelled reference      │
│     Bounds narrow around top-K results                      │
│     Duration governed by round_durations[i]                 │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
              ┌─────── More rounds? ───────┐
              │ YES                        │ NO
              ▼                            ▼
        Go to step 2                 ┌─────────────────┐
        (new reference run           │ 5. COMPLETE     │
         with narrowed params)       │ Best params     │
                                     │ applied & saved │
                                     │ to scene        │
                                     └─────────────────┘
```

### Round Durations Example

Input: `round_durations: [60, 60, 600, 60]` (minutes)

| Phase           | Duration | Wall clock (if started 8pm) | Activity                           |
| --------------- | -------- | --------------------------- | ---------------------------------- |
| Reference run 1 | ~2 min   | 8:00 pm                     | Automated PCAP replay              |
| Label window 1  | 60 min   | 8:02 – 9:02 pm              | Human labels tracks                |
| Sweep round 1   | 60 min   | 9:02 – 10:02 pm             | Automated sweep                    |
| Reference run 2 | ~2 min   | 10:02 pm                    | Automated replay with round-1 best |
| Label window 2  | 60 min   | 10:04 – 11:04 pm            | Human labels tracks                |
| Sweep round 2   | 600 min  | 11:04 pm – 9:04 am          | Overnight sweep (deep)             |
| Reference run 3 | ~2 min   | 9:04 am                     | Automated replay with round-2 best |
| Label window 3  | 60 min   | 9:06 – 10:06 am             | Human labels (morning)             |
| Complete        | —        | 10:06 am                    | Best params applied                |

The `round_durations` list has `2 × N - 1` entries where N = number of label
rounds. Odd-indexed entries are sweep durations; even-indexed entries are label
windows. If the list is shorter than needed, the last value repeats. This means a
single value `[60]` means "60-minute label window, 60-minute sweep" for every
round.

**Important:** The round durations govern _maximum_ time allowed. Label windows
can be completed early by clicking "Continue". Sweep rounds may finish early if
all combinations are evaluated before the time limit.

## Data Model Changes

### New `RLHFSweepRequest` (Go struct in `sweep/auto.go`)

```go
type RLHFSweepRequest struct {
    // Scene to use (provides PCAP, sensor_id)
    SceneID string `json:"scene_id"`

    // Number of label-then-sweep cycles
    NumRounds int `json:"num_rounds"`

    // Alternating label–sweep durations in minutes.
    // Index 0 = first label window, 1 = first sweep, 2 = second label window, …
    // If shorter than 2*NumRounds-1, the last value repeats.
    RoundDurations []int `json:"round_durations"`

    // Parameters to sweep (same format as AutoTuneRequest.Params)
    Params []SweepParam `json:"params"`

    // Auto-tune settings (reused from AutoTuneRequest)
    ValuesPerParam int    `json:"values_per_param"`
    TopK           int    `json:"top_k"`
    Iterations     int    `json:"iterations"`
    Interval       string `json:"interval"`
    SettleTime     string `json:"settle_time"`
    Seed           string `json:"seed"`
    SettleMode     string `json:"settle_mode"`

    // Ground truth scoring weights
    GroundTruthWeights *GroundTruthWeights `json:"ground_truth_weights"`

    // Acceptance criteria (optional hard thresholds)
    AcceptanceCriteria *AcceptanceCriteria `json:"acceptance_criteria"`
}
```

### New `RLHFState` (Go struct in `sweep/auto.go`)

```go
type RLHFState struct {
    Status         string          `json:"status"`
    // "idle" | "running_reference" | "awaiting_labels" | "running_sweep" | "completed" | "failed"
    Mode           string          `json:"mode"`           // always "rlhf"
    CurrentRound   int             `json:"current_round"`
    TotalRounds    int             `json:"total_rounds"`
    ReferenceRunID string          `json:"reference_run_id,omitempty"`
    LabelProgress  *LabelProgress  `json:"label_progress,omitempty"`
    LabelDeadline  *time.Time      `json:"label_deadline,omitempty"`
    SweepDeadline  *time.Time      `json:"sweep_deadline,omitempty"`
    AutoTuneState  *AutoTuneState  `json:"auto_tune_state,omitempty"`
    Recommendation map[string]any  `json:"recommendation,omitempty"`
    RoundHistory   []RLHFRound     `json:"round_history"`
    Error          string          `json:"error,omitempty"`
}

type LabelProgress struct {
    Total    int            `json:"total"`
    Labelled int            `json:"labelled"`
    Pct      float64        `json:"progress_pct"`
    ByClass  map[string]int `json:"by_class"`
}

type RLHFRound struct {
    Round          int                `json:"round"`
    ReferenceRunID string             `json:"reference_run_id"`
    LabelledAt     *time.Time         `json:"labelled_at,omitempty"`
    LabelProgress  *LabelProgress     `json:"label_progress,omitempty"`
    SweepID        string             `json:"sweep_id,omitempty"`
    BestScore      float64            `json:"best_score"`
    BestParams     map[string]float64 `json:"best_params,omitempty"`
}
```

### Database: `lidar_sweeps` table

No schema changes required. The existing table already stores:

- `mode` — will be `"rlhf"` instead of `"auto"` or `"params"`
- `request` — JSON blob of the `RLHFSweepRequest`
- `results` — JSON blob of final combo results
- `recommendation` — JSON blob of best params
- `round_results` — JSON blob of `RLHFRound[]` history

The `status` column gains new values: `"awaiting_labels"`,
`"running_reference"`, `"running_sweep"`.

### Scene `reference_run_id`

Each round updates the scene's `reference_run_id` to point to the latest labelled
run. This is already supported by `SceneStore.SetReferenceRun()`. On completion
the scene's `optimal_params_json` is updated with the best parameters (already
supported by `SceneStore.SetOptimalParams()`).

## Implementation Plan

### Phase 1: Backend — `RLHFTuner` Engine

**File:** `internal/lidar/sweep/rlhf.go` (new)

1. Define `RLHFSweepRequest` and `RLHFState` structs.
2. Implement `RLHFTuner` struct:
   - Embeds/wraps `AutoTuner` for the sweep-round part.
   - Holds `analysisRunManager` for creating reference runs.
   - Holds `analysisRunStore` for querying label progress.
   - Holds `sceneStore` for updating reference run IDs.
   - Holds `groundTruthScorer` (reuse same injection pattern).
3. Implement the core loop in `run(ctx, req)`:

```
func (rt *RLHFTuner) run(ctx context.Context, req RLHFSweepRequest):
    scene := sceneStore.GetScene(req.SceneID)
    currentParams := loadCurrentParams()  // or use scene.optimal_params_json
    bounds := req.Params  // initial param bounds

    for round := 1..req.NumRounds:
        // --- Reference Run ---
        rt.setState("running_reference", round)
        runID := createReferenceRun(scene, currentParams)
        sceneStore.SetReferenceRun(req.SceneID, runID)
        rt.setReferenceRunID(runID)

        // --- Await Labels ---
        rt.setState("awaiting_labels", round)
        labelDuration := getDuration(req.RoundDurations, (round-1)*2)
        rt.setLabelDeadline(now + labelDuration)
        waitForLabelsOrDeadline(ctx, runID, labelDuration)

        // --- Sweep Round ---
        rt.setState("running_sweep", round)
        sweepDuration := getDuration(req.RoundDurations, (round-1)*2 + 1)
        rt.setSweepDeadline(now + sweepDuration)

        autoReq := buildAutoTuneRequest(bounds, req, scene)
        autoReq.SceneID = req.SceneID  // ground truth uses this scene's reference_run_id
        autoReq.MaxRounds = 1  // single round per RLHF cycle
        autoReq.Objective = "ground_truth"

        runAutoTuneRound(ctx, autoReq, sweepDuration)

        // --- Narrow Bounds ---
        bestResult := getBestResult()
        currentParams = bestResult.params
        bounds = narrowBounds(bounds, topKResults)
        rt.recordRound(round, runID, bestResult)

    // --- Complete ---
    applyBestParams(currentParams)
    sceneStore.SetOptimalParams(req.SceneID, currentParams)
    rt.setState("completed")
```

4. Implement `waitForLabelsOrDeadline`:
   - Poll `analysisRunStore.GetLabelingProgress(runID)` every 10s.
   - Also check for a manual "continue" signal (channel/atomic flag).
   - Return when: (a) deadline expires, (b) continue signal received, or
     (c) context cancelled.

5. Implement `continueFromLabels()` — public method called by the API handler
   when the human clicks "Continue". Sets the atomic flag and unblocks the wait.

### Phase 2: Backend — API Endpoints

**File:** `internal/lidar/monitor/sweep_handlers.go` (extend)

| Endpoint                         | Method | Purpose                                           |
| -------------------------------- | ------ | ------------------------------------------------- |
| `/api/lidar/sweep/rlhf`          | POST   | Start RLHF sweep (JSON body = `RLHFSweepRequest`) |
| `/api/lidar/sweep/rlhf`          | GET    | Get current `RLHFState` for polling               |
| `/api/lidar/sweep/rlhf/continue` | POST   | Signal "labels done, continue to sweep"           |
| `/api/lidar/sweep/rlhf/stop`     | POST   | Cancel the RLHF run                               |

The RLHF state is polled by the dashboard alongside the existing auto-tune
polling. The `RLHFState` response includes enough information for the dashboard
to show: which round, what phase, label progress, deadline countdown, link to the
labelling UI.

**Wiring** in `webserver.go`:

- Add `rlhfTuner *RLHFTuner` field to `WebServer`.
- Wire dependencies (analysisRunManager, analysisRunStore, sceneStore,
  groundTruthScorer) in `cmd/radar/radar.go`.
- Register routes with `mux.HandleFunc`.

### Phase 3: Dashboard UI — Third Mode

**File:** `sweep_dashboard.html` + `sweep_dashboard.js` + `sweep_dashboard.css`

#### 3a. Mode Toggle

Update the mode toggle to three buttons:

```html
<div class="mode-toggle">
  <button id="mode-manual" class="active" onclick="setMode('manual')">
    Manual Sweep
  </button>
  <button id="mode-auto" onclick="setMode('auto')">Auto-Tune</button>
  <button id="mode-rlhf" onclick="setMode('rlhf')">Human-in-the-Loop</button>
</div>
```

CSS adds `body.rlhf-mode` alongside `body.auto-mode`:

- `.manual-only` hidden in auto and rlhf modes
- `.auto-only` visible only in auto mode
- `.rlhf-only` visible only in rlhf mode
- `.auto-or-rlhf` visible in both auto and rlhf modes (shared config cards)

Update `setMode()` to handle three states and toggle appropriate body classes.

#### 3b. RLHF Config Card (`.rlhf-only`)

```
┌─────────────────────────────────────┐
│  Human-in-the-Loop Settings         │
├─────────────────────────────────────┤
│  Scene:       [scene dropdown ▾]    │
│  Rounds:      [3        ]           │
│  Durations:   [60, 60, 600, 60]     │
│    (minutes, alternating            │
│     label / sweep windows)          │
│                                     │
│  [Start RLHF]                       │
└─────────────────────────────────────┘
```

The durations field is a comma-separated text input. The scene dropdown reuses
the existing `scene_select` element (shared with auto-tune scene source).

The Sweep Parameters card (param bounds) is shared with auto mode
(`.auto-or-rlhf`), as is the Sweep Configuration card (iterations, interval,
settle time). The Data Source card is hidden in RLHF mode because the scene
provides the PCAP source.

#### 3c. RLHF Progress Card

Replaces the generic progress section when in RLHF mode:

```
┌─────────────────────────────────────┐
│  Round 2 of 3                       │
│  Phase: Awaiting Labels             │
│  ┌─────────────────────────────┐    │
│  │ ████████░░░░  12/18 (67%)   │    │
│  └─────────────────────────────┘    │
│  Time remaining: 42:15              │
│  Run: abc123...  [Open Tracks →]    │
│                                     │
│  [Continue to Sweep]                │
└─────────────────────────────────────┘
```

Key elements:

- **Progress bar** showing label completion
- **Countdown timer** to the deadline
- **Link to Tracks page** — direct link to
  `/app/lidar/tracks?scene_id=X&run_id=Y` to label
- **Continue button** — calls `POST /api/lidar/sweep/rlhf/continue`
- During "running_sweep" phase, shows sweep progress instead (reuses existing
  auto-tune progress rendering)

#### 3d. Round History

Below the progress card, a collapsible list of completed rounds:

```
▶ Round 1: score=0.847 — 15/18 labelled — eps=0.3, noise=0.9
▶ Round 2: score=0.912 — 18/18 labelled — eps=0.45, noise=0.7
```

### Phase 4: Dashboard Polling

Extend `pollAutoTuneStatus()` (or add `pollRLHFStatus()`) to poll
`GET /api/lidar/sweep/rlhf` when in RLHF mode. The poller:

1. Renders the RLHF progress card with current phase / round info.
2. During `awaiting_labels`:
   - Shows label progress bar (re-poll every 5s for progress updates).
   - Shows countdown timer (client-side, from `label_deadline`).
   - Enables "Continue to Sweep" button.
3. During `running_sweep`:
   - Shows sweep progress (combo N of M, time remaining).
   - Shows intermediate results chart if available.
4. During `running_reference`:
   - Shows "Creating reference run…" spinner.
5. On `completed`:
   - Shows final recommendation with Apply button.
   - Shows round history summary.
6. On `failed`:
   - Shows error message.

### Phase 5: Svelte Sweeps Page Updates

Extend the existing `/app/lidar/sweeps` page:

1. Show RLHF sweeps with `mode = "rlhf"` — distinct badge colour.
2. In the detail panel for RLHF sweeps, show:
   - Round history with links to each reference run's tracks page.
   - Label progress per round.
   - Ground truth scores per round.
   - Final recommendation with Apply button.
3. When an RLHF sweep is in `awaiting_labels` state:
   - Show a prominent "Label Tracks" link to the tracks page.
   - Show "Continue" button inline.

### Phase 6: Mode Description Updates

Update the page subtitle and add mode-specific descriptions:

**Page subtitle** (shared):

> Sweep tuning parameters and visualise results to identify optimal tuning.

**Manual Sweep** — no additional description (fits current pattern).

**Auto-Tune description** (`.auto-only`, above config card):

> Automated multi-round parameter optimisation. The tuner generates a parameter
> grid, evaluates each combination, and narrows bounds around the best results.
> Uses proxy metrics (acceptance rate, fragmentation, empty box ratio) or ground
> truth scoring with a pre-labelled reference run.

**RLHF description** (`.rlhf-only`, above config card):

> Human-in-the-loop tuning. Each round: the system creates a reference run, you
> label the tracks to establish ground truth, then the tuner sweeps parameters
> using your labels as the objective. Repeats for multiple rounds, progressively
> narrowing towards parameters that produce tracks you judge as correct.

## Open Questions

1. **Minimum label threshold:** Should the system enforce a minimum number of
   labelled tracks before allowing "Continue"? Suggested: require ≥50% labelled
   or at least 5 tracks to have meaningful ground truth.

2. **Label carryover:** When round N+1 creates a new reference run with different
   params, should we attempt to pre-populate labels by matching tracks from the
   previous round (using temporal IoU), or always start fresh? Pre-populating
   would speed up labelling but might introduce bias.

3. **Partial labelling strategy:** If the human only labels 60% of tracks, the
   unlabelled tracks are excluded from ground truth evaluation. Document expected
   biases: detection rate may appear lower because some good tracks are simply
   unlabelled.

4. **Notifications:** Should the system send a notification (browser
   notification, webhook, or email) when a sweep round completes and labels are
   needed? For overnight sweeps this is valuable. Suggested: browser notification
   via the Notification API if the dashboard tab is open.

5. **Scoring composit adjustments:** The default `GroundTruthWeights` were
   designed for a single pre-labelled reference. For iterative RLHF with
   potentially sparse labels, the weights may need adjustment. Consider bumping
   `DetectionRate` weight and reducing `FalsePositiveRate` weight for early rounds
   when few tracks are labelled.

## File Manifest

| File                                                | Action     | Description                                                                |
| --------------------------------------------------- | ---------- | -------------------------------------------------------------------------- |
| `internal/lidar/sweep/rlhf.go`                      | **Create** | `RLHFTuner`, `RLHFSweepRequest`, `RLHFState`, core loop                    |
| `internal/lidar/sweep/rlhf_test.go`                 | **Create** | Unit tests for state machine, duration parsing, bound narrowing            |
| `internal/lidar/monitor/sweep_handlers.go`          | **Modify** | Add 4 RLHF endpoints                                                       |
| `internal/lidar/monitor/webserver.go`               | **Modify** | Wire `rlhfTuner` field + routes                                            |
| `cmd/radar/radar.go`                                | **Modify** | Create `RLHFTuner`, inject dependencies                                    |
| `internal/lidar/monitor/html/sweep_dashboard.html`  | **Modify** | Third mode button, RLHF config card, RLHF progress card                    |
| `internal/lidar/monitor/assets/sweep_dashboard.js`  | **Modify** | `setMode` three-way, `handleStartRLHF`, `pollRLHFStatus`, continue handler |
| `internal/lidar/monitor/assets/sweep_dashboard.css` | **Modify** | `.rlhf-mode`, `.rlhf-only`, `.auto-or-rlhf` classes                        |
| `web/src/routes/lidar/sweeps/+page.svelte`          | **Modify** | RLHF mode badge, round history, label link                                 |
| `web/src/lib/api.ts`                                | **Modify** | Add `startRLHF`, `getRLHFState`, `continueRLHF`, `stopRLHF`                |
| `web/src/lib/types/lidar.ts`                        | **Modify** | Add `RLHFState`, `RLHFRound`, `LabelProgress` types                        |

## Testing Strategy

1. **Unit tests** (`rlhf_test.go`):
   - Duration indexing: verify wrap-around behaviour for short duration lists.
   - State machine transitions: idle → running_reference → awaiting_labels →
     running_sweep → running_reference → … → completed.
   - Continue signal unblocks wait.
   - Context cancellation stops the loop.
   - Bound narrowing carries over between rounds.

2. **Integration tests** (manual):
   - Start RLHF with a short PCAP scene, 2 rounds, durations `[1, 1]`.
   - Verify reference run appears in Runs page.
   - Label a few tracks, click Continue.
   - Verify sweep starts and produces results.
   - Verify second reference run uses narrowed params.
   - Verify final recommendation is applied.

3. **Dashboard tests** (`sweep_dashboard.test.ts`):
   - Mode switching to/from "rlhf" shows/hides correct cards.
   - RLHF start request is built correctly from UI inputs.
   - Polling renders correct phase displays.
