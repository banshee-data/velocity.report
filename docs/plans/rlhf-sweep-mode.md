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
│     Labels auto-saved on click (no explicit export step)    │
│     Dashboard shows labelling progress + countdown          │
│     Prior round labels are pre-populated via temporal IoU   │
│     ≥90% label threshold enforced before continuing         │
│     Human can edit next round duration before clicking      │
│     "Continue" (or add an extra round)                      │
│     Browser notification fires when labels are needed       │
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

**Editing durations mid-run:** During the `awaiting_labels` phase, the dashboard
exposes the next sweep-round duration as an editable field. The human can
increase or decrease it before clicking "Continue" — useful when a label window
was missed and the schedule needs adjustment. The updated duration is sent in the
`POST /api/lidar/sweep/rlhf/continue` request body.

**Adding extra rounds:** The same "Continue" card includes an "Add Round" toggle.
When enabled, the system appends one additional label→sweep cycle after the
current round completes, using the edited duration. This lets the human extend
the sweep if intermediate results look promising.

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

    // Minimum fraction of tracks that must be labelled before
    // the human can click "Continue" (default: 0.9 = 90%)
    MinLabelThreshold float64 `json:"min_label_threshold"`

    // Whether to carry over labels from previous round via temporal IoU
    // matching (default: true)
    CarryOverLabels bool `json:"carry_over_labels"`
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

    // Minimum label threshold (echoed from request for UI enforcement)
    MinLabelThreshold float64     `json:"min_label_threshold"`

    // Whether labels were carried over from a previous round
    LabelsCarriedOver int         `json:"labels_carried_over"`

    // Next round's sweep duration (editable by human during awaiting_labels)
    NextSweepDuration int         `json:"next_sweep_duration_mins"`
}

type LabelProgress struct {
    Total    int            `json:"total"`
    Labelled int            `json:"labelled"`
    Pct      float64        `json:"progress_pct"`
    ByClass  map[string]int `json:"by_class"`
}

type RLHFRound struct {
    Round             int                `json:"round"`
    ReferenceRunID    string             `json:"reference_run_id"`
    LabelledAt        *time.Time         `json:"labelled_at,omitempty"`
    LabelProgress     *LabelProgress     `json:"label_progress,omitempty"`
    LabelsCarriedOver int                `json:"labels_carried_over"`
    SweepID           string             `json:"sweep_id,omitempty"`
    BestScore         float64            `json:"best_score"`
    BestParams        map[string]float64 `json:"best_params,omitempty"`
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

        // --- Carry Over Labels (round 2+) ---
        if round > 1 && req.CarryOverLabels {
            carried := carryOverLabels(prevRunID, runID)
            rt.setLabelsCarriedOver(carried)
        }

        // --- Await Labels ---
        rt.setState("awaiting_labels", round)
        labelDuration := getDuration(req.RoundDurations, (round-1)*2)
        rt.setLabelDeadline(now + labelDuration)
        rt.setNextSweepDuration(getDuration(req.RoundDurations, (round-1)*2+1))
        err := waitForLabelsOrDeadline(ctx, runID, labelDuration, req.MinLabelThreshold)
        if err != nil { rt.setState("failed"); return }  // threshold not met at deadline

        // --- Sweep Round ---
        rt.setState("running_sweep", round)
        sweepDuration := rt.getNextSweepDuration()  // may have been edited
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
        prevRunID = runID

        // --- Check for extra round added by human ---
        // (TotalRounds may have been incremented by continueFromLabels)

    // --- Complete ---
    applyBestParams(currentParams)
    sceneStore.SetOptimalParams(req.SceneID, currentParams)
    rt.setState("completed")
```

4. Implement `waitForLabelsOrDeadline`:
   - Poll `analysisRunStore.GetLabelingProgress(runID)` every 10s.
   - Also check for a manual "continue" signal (channel/atomic flag).
   - Enforce `MinLabelThreshold` — the continue signal is rejected if
     `progress.Pct < req.MinLabelThreshold` (default 0.9 = 90%). The HTTP
     handler returns `400 Bad Request` with a message showing the current
     progress and the required threshold.
   - Return when: (a) deadline expires, (b) continue signal received _and_
     threshold met, or (c) context cancelled.
   - On deadline expiry, if threshold is not met, transition to `"failed"`
     with an error message rather than proceeding with insufficient labels.

5. Implement `continueFromLabels(nextDuration int, addRound bool)` — public
   method called by the API handler when the human clicks "Continue".
   - Validates label threshold is met.
   - If `nextDuration > 0`, overrides the next sweep-round duration.
   - If `addRound`, increments `TotalRounds` by 1 and appends a default
     duration entry.
   - Sets the atomic flag and unblocks the wait.

6. Implement `carryOverLabels(prevRunID, newRunID string)`:
   - Fetches labelled tracks from `prevRunID` via
     `analysisRunStore.GetRunTracks(prevRunID)`.
   - Fetches unlabelled tracks from `newRunID`.
   - For each labelled track in the previous run, finds the best-matching
     track in the new run using temporal IoU (overlap of
     `[startUnixNanos, endUnixNanos]` intervals divided by their union).
   - Only carries a label if IoU ≥ 0.5 (sufficient temporal overlap to
     consider them the same real-world event).
   - Writes carried-over labels via
     `analysisRunStore.SetTrackLabel(newRunID, trackID, label)`.
   - Records count in `RLHFState.LabelsCarriedOver` for UI display.
   - The human reviews and corrects any mismatches during the label window.

### Phase 2: Backend — API Endpoints

**File:** `internal/lidar/monitor/sweep_handlers.go` (extend)

| Endpoint                         | Method | Purpose                                           |
| -------------------------------- | ------ | ------------------------------------------------- |
| `/api/lidar/sweep/rlhf`          | POST   | Start RLHF sweep (JSON body = `RLHFSweepRequest`) |
| `/api/lidar/sweep/rlhf`          | GET    | Get current `RLHFState` for polling               |
| `/api/lidar/sweep/rlhf/continue` | POST   | Signal "labels done, continue to sweep"           |
| `/api/lidar/sweep/rlhf/stop`     | POST   | Cancel the RLHF run                               |

**`/api/lidar/sweep/rlhf/continue` request body:**

```json
{
  "next_sweep_duration_mins": 120,
  "add_round": false
}
```

Both fields are optional. If `next_sweep_duration_mins` is provided, it overrides
the scheduled duration for the upcoming sweep round. If `add_round` is `true`,
an extra label→sweep cycle is appended after the current round.

The handler returns `400 Bad Request` if `label_progress.progress_pct` is below
`min_label_threshold` (default 90%). The error message includes the current
progress and the required threshold so the dashboard can show it.

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
┌─────────────────────────────────────────────┐
│  Round 2 of 3                               │
│  Phase: Awaiting Labels                     │
│  ┌─────────────────────────────────┐        │
│  │ ████████████░░  16/18 (89%)     │  ≥90%  │
│  └─────────────────────────────────┘        │
│  Time remaining: 42:15                      │
│  Labels carried over: 14 (from round 1)     │
│  Run: abc123...  [Open Tracks →]            │
│                                             │
│  Next sweep duration: [120] mins            │
│  ☐ Add extra round after this sweep         │
│                                             │
│  [Continue to Sweep]  (disabled until ≥90%) │
└─────────────────────────────────────────────┘
```

Key elements:

- **Progress bar** showing label completion with threshold marker at 90%
- **Threshold enforcement** — "Continue" button disabled until
  `progress_pct >= min_label_threshold`. Tooltip explains the requirement.
- **Carried-over label count** — shows how many labels were pre-populated
  from the previous round.
- **Countdown timer** to the deadline
- **Link to Tracks page** — direct link to
  `/app/lidar/tracks?scene_id=X&run_id=Y` to label
- **Editable next sweep duration** — number input pre-filled from
  `next_sweep_duration_mins` in the state response. Sent in the continue
  request body.
- **Add extra round toggle** — checkbox that sends `add_round: true` in the
  continue request. Useful when the human wants more refinement.
- **Continue button** — calls `POST /api/lidar/sweep/rlhf/continue` with
  `{ next_sweep_duration_mins, add_round }`
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
   - Progress bar includes 90% threshold marker.
   - Shows countdown timer (client-side, from `label_deadline`).
   - Shows carried-over label count if > 0.
   - Shows editable next-sweep-duration field.
   - Shows "Add extra round" checkbox.
   - Enables "Continue to Sweep" button only when `progress_pct ≥ 0.9`.
   - Sends browser notification via Notification API when entering this
     phase (if permission granted and tab is not focused).
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

### Phase 4b: Browser Notifications

The dashboard requests `Notification.requestPermission()` when RLHF mode is
selected. Notifications fire at two transition points:

1. **Sweep round completed → labels needed.** When the poller detects a
   transition to `awaiting_labels`, it fires:

   ```js
   new Notification("Labels needed — Round N", {
     body: "Reference run ready. Label tracks to continue the sweep.",
     tag: "rlhf-labels-needed",
     requireInteraction: true,
   });
   ```

   `requireInteraction: true` keeps the notification visible until dismissed
   (useful for overnight sweeps where the user may have stepped away).

2. **RLHF sweep completed.** When the poller detects `completed`:
   ```js
   new Notification("RLHF Sweep Complete", {
     body: "Best parameters found. Review and apply.",
     tag: "rlhf-complete",
   });
   ```

Clicking either notification calls `window.focus()` to bring the dashboard tab
to the front. The `tag` field ensures duplicate notifications are replaced rather
than stacked.

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

## Design Decisions (Resolved)

### 1. Minimum Label Threshold — 90%

The system enforces a minimum of **90% of tracks labelled** before the human can
click "Continue". The threshold is configurable via `min_label_threshold` in the
request (default `0.9`). Rationale: ground truth scoring with sparse labels
artificially depresses detection rate because unlabelled tracks count as
non-detections. At 90%, the scoring is reliable enough for meaningful parameter
comparison.

Enforcement:

- **Server-side:** The `/api/lidar/sweep/rlhf/continue` handler checks progress
  before accepting the signal. Returns `400 Bad Request` if below threshold.
- **Client-side:** The "Continue" button is disabled with a tooltip showing
  `"Label at least 90% of tracks (currently 67%)"` until the threshold is met.
- **Deadline expiry:** If the label-window deadline expires before the threshold
  is met, the sweep transitions to `"failed"` with a descriptive error rather
  than continuing with insufficient labels.

### 2. Label Carryover — Enabled

When round N+1 creates a new reference run, labels from round N are
automatically **carried over** by matching tracks using temporal IoU
(overlap-over-union of `[startUnixNanos, endUnixNanos]` intervals).

**Algorithm:**

1. Fetch all labelled tracks from the previous round's reference run.
2. Fetch all unlabelled tracks from the new reference run.
3. For each labelled track, compute temporal IoU with every new track.
4. If best IoU ≥ 0.5, copy `user_label`, `quality_label`, and
   `label_confidence` to the new track.
5. Record the number of carried-over labels in `RLHFState.LabelsCarriedOver`
   and display in the progress card.

**Bias mitigation:** The human reviews carried-over labels during the label
window. Changed parameters may produce different track boundaries, so carried-
over labels are a starting point, not final. The dashboard highlights carried-
over labels with a distinct badge so the human knows which ones to review.

**Configuration:** `carry_over_labels` (default `true`). Set to `false` to start
each round from scratch.

### 3. Partial Labelling Strategy

With the 90% threshold, partial labelling is limited to at most 10% of tracks.
Unlabelled tracks are excluded from ground truth evaluation. At this coverage
level, the scoring bias is minimal. The `LabelProgress` struct's `by_class` map
lets the human and the system verify label distribution before continuing.

### 4. Browser Notifications — Enabled

Browser notifications via the `Notification` API fire when:

- A sweep round completes and labels are needed (`awaiting_labels` transition).
- The RLHF sweep completes (`completed` transition).

Notifications use `requireInteraction: true` to persist until dismissed —
critical for overnight sweeps where the user has stepped away. The dashboard
requests `Notification.requestPermission()` when RLHF mode is first selected.
See Phase 4b for implementation details.

### 5. Scoring Weight Adjustments for Early Rounds

The default `GroundTruthWeights` are tuned for full-coverage labels. For RLHF
rounds (where coverage is ≥90% but the labeller may focus on salient tracks), the
tuner applies round-dependent weight adjustments:

| Round | `DetectionRate` weight | `FalsePositiveRate` weight | Rationale                          |
| ----- | ---------------------- | -------------------------- | ---------------------------------- |
| 1     | 1.5× default           | 0.5× default               | Focus on finding all real vehicles |
| 2+    | 1.0× default           | 1.0× default               | Balanced scoring with more labels  |

The multipliers are applied inside `RLHFTuner.buildAutoTuneRequest()` before
passing weights to the ground truth scorer. They are not persisted — the final
recommendation uses default weights for a clean comparison.

### 6. Editable Round Durations + Extra Rounds

During the `awaiting_labels` phase, the human can:

1. **Edit the next sweep duration** via a number input in the progress card.
   The edited value is sent in the `POST /api/lidar/sweep/rlhf/continue` body
   as `next_sweep_duration_mins`. This handles the common case of missing a
   label window overnight and needing to adjust the schedule.

2. **Add an extra round** via a checkbox. When checked, `add_round: true` is
   sent in the continue request. The tuner increments `TotalRounds` by 1 and
   appends a default duration entry. This lets the human extend the sweep if
   intermediate results show the parameter space hasn't converged.

## Prerequisite: macOS Visualiser Label UX Changes

The macOS app's labelling workflow needs three changes before RLHF mode is
viable. These are independent of the RLHF backend and can be shipped first.

### P1. Display Existing Labels in LabelPanelView

**Problem:** `LabelPanelView` does not fetch or display a track's existing
`user_label` / `quality_label` from the database. The checkmark only appears for
labels assigned during the current session (`@State lastAssignedLabel`). When
reviewing carried-over labels or resuming a labelling session, the human has no
visual indication of what's already set.

**Fix:** When a track is selected in run mode (`currentRunID` is set), fetch the
track's label from the run-track list (already loaded by `TrackListView`) and
pre-populate the `LabelPanelView`'s selection state. Specifically:

1. In `LabelPanelView`, accept the selected `RunTrack?` as a binding or
   environment value.
2. When the selected track changes, set `lastAssignedLabel` and
   `lastAssignedQuality` from `runTrack.userLabel` and `runTrack.qualityLabel`.
3. Show the checkmark on the matching button so the human sees the current state.
4. For carried-over labels, add a small "↻ carried" badge next to the checkmark
   (requires a `label_source` field or convention, e.g. `labeler_id = "rlhf-
carryover"`).

**Files:**

- `tools/visualiser-macos/VelocityVisualiser/UI/ContentView.swift` — `LabelPanelView`
- `tools/visualiser-macos/VelocityVisualiser/App/AppState.swift` — track selection

### P2. Remove Export Labels Button

**Problem:** The "Export Labels" button in `SidePanelView` exports free-form
`LabelEvent` records (session-based, via `LabelAPIClient`). In RLHF mode, labels
are always run-track labels saved directly to the database via
`RunTrackLabelAPIClient`. The export button is confusing because it doesn't
export the labels the human just assigned.

**Fix:** Remove the "Export Labels" button and `exportLabels()` method from
`AppState`. Labels are persisted server-side on every click — no export step is
needed. If a bulk-export feature is needed later, it should export run-track
labels via a server endpoint (e.g. `GET /api/lidar/runs/{run_id}/labels/export`).

**Files:**

- `tools/visualiser-macos/VelocityVisualiser/UI/ContentView.swift` — remove button (~line 467)
- `tools/visualiser-macos/VelocityVisualiser/App/AppState.swift` — remove `exportLabels()`

### P3. Auto-Save Labels on Click (Already Implemented)

`assignLabel()` and `assignQuality()` in `AppState` already fire async API calls
immediately on click — no batching or explicit save step. This is the correct
behaviour for RLHF. No changes needed, but worth noting as a confirmed
requirement.

## Implementation Checklist

### Prerequisites (macOS Visualiser)

- [ ] **P1** — Display existing labels in `LabelPanelView`
  - [ ] Accept selected `RunTrack?` in `LabelPanelView`
  - [ ] Pre-populate `lastAssignedLabel` / `lastAssignedQuality` from run-track data
  - [ ] Show checkmark on matching button for current label state
  - [ ] Add "↻ carried" badge for carried-over labels
- [ ] **P2** — Remove Export Labels button
  - [ ] Remove "Export Labels" button from `SidePanelView`
  - [ ] Remove `exportLabels()` from `AppState`
- [ ] **P3** — Confirm auto-save on click works (no changes needed)

### Phase 1: Backend — `RLHFTuner` Engine

- [ ] Define `RLHFSweepRequest` struct (scene ID, rounds, durations, threshold, carryover flag)
- [ ] Define `RLHFState` struct (phase, round, deadlines, label progress, carried-over count)
- [ ] Implement `RLHFTuner` struct with dependency injection
- [ ] Implement `run(ctx, req)` core loop (reference → labels → sweep → narrow → repeat)
- [ ] Implement `waitForLabelsOrDeadline` with 10s polling, threshold enforcement, deadline expiry
- [ ] Implement `continueFromLabels(nextDuration, addRound)` with threshold validation
- [ ] Implement `carryOverLabels(prevRunID, newRunID)` with temporal IoU matching (≥ 0.5)
- [ ] Implement scoring weight adjustments for early rounds
- [ ] Write unit tests (`rlhf_test.go`)

### Phase 2: Backend — API Endpoints

- [ ] `POST /api/lidar/sweep/rlhf` — start RLHF sweep
- [ ] `GET /api/lidar/sweep/rlhf` — poll current `RLHFState`
- [ ] `POST /api/lidar/sweep/rlhf/continue` — signal labels done (with threshold check)
- [ ] `POST /api/lidar/sweep/rlhf/stop` — cancel RLHF run
- [ ] Wire `rlhfTuner` into `WebServer` and `cmd/radar/radar.go`
- [ ] Write API handler tests

### Phase 3: Dashboard UI — Third Mode

- [ ] **3a** — Add "Human-in-the-Loop" mode toggle button
- [ ] Update `setMode()` for three-way switching + CSS body classes
- [ ] **3b** — RLHF config card (scene dropdown, rounds, durations input)
- [ ] **3c** — RLHF progress card
  - [ ] Label progress bar with 90% threshold marker
  - [ ] Countdown timer (from `label_deadline`)
  - [ ] Carried-over label count display
  - [ ] Link to Tracks page for labelling
  - [ ] Editable next-sweep-duration field
  - [ ] "Add extra round" checkbox
  - [ ] "Continue to Sweep" button (disabled until ≥ 90%)
  - [ ] Sweep progress display during `running_sweep` phase
- [ ] **3d** — Round history (collapsible list of completed rounds)
- [ ] Write dashboard tests (`sweep_dashboard.test.ts`)

### Phase 4: Dashboard Polling

- [ ] Implement `pollRLHFStatus()` or extend `pollAutoTuneStatus()`
- [ ] Handle `awaiting_labels` phase (5s poll, progress bar, countdown, continue button)
- [ ] Handle `running_sweep` phase (combo progress, intermediate results)
- [ ] Handle `running_reference` phase (spinner)
- [ ] Handle `completed` phase (recommendation + apply button + round history)
- [ ] Handle `failed` phase (error message)

### Phase 4b: Browser Notifications

- [ ] Request `Notification.requestPermission()` on RLHF mode selection
- [ ] Fire "Labels needed — Round N" notification on `awaiting_labels` transition
- [ ] Fire "RLHF Sweep Complete" notification on `completed` transition
- [ ] Bring dashboard tab to front on notification click

### Phase 5: Svelte Sweeps Page Updates

- [ ] Show RLHF sweeps with distinct `mode = "rlhf"` badge
- [ ] RLHF detail panel: round history with links to reference run tracks
- [ ] RLHF detail panel: label progress and ground truth scores per round
- [ ] Inline "Continue" button for `awaiting_labels` state
- [ ] Add `startRLHF`, `getRLHFState`, `continueRLHF`, `stopRLHF` to `api.ts`
- [ ] Add `RLHFState`, `RLHFRound`, `LabelProgress` types to `lidar.ts`

### Phase 6: Mode Description Updates

- [ ] Add page subtitle shared across modes
- [ ] Add Auto-Tune description text (`.auto-only`)
- [ ] Add RLHF description text (`.rlhf-only`)

## File Manifest

| File                                                             | Action     | Description                                                              |
| ---------------------------------------------------------------- | ---------- | ------------------------------------------------------------------------ |
| `internal/lidar/sweep/rlhf.go`                                   | **Create** | `RLHFTuner`, `RLHFSweepRequest`, `RLHFState`, core loop, label carryover |
| `internal/lidar/sweep/rlhf_test.go`                              | **Create** | Unit tests: state machine, duration parsing, carryover, threshold        |
| `internal/lidar/monitor/sweep_handlers.go`                       | **Modify** | Add 4 RLHF endpoints (with continue body parsing)                        |
| `internal/lidar/monitor/webserver.go`                            | **Modify** | Wire `rlhfTuner` field + routes                                          |
| `cmd/radar/radar.go`                                             | **Modify** | Create `RLHFTuner`, inject dependencies                                  |
| `internal/lidar/monitor/html/sweep_dashboard.html`               | **Modify** | Third mode button, RLHF config/progress cards, notification permission   |
| `internal/lidar/monitor/assets/sweep_dashboard.js`               | **Modify** | `setMode` three-way, `handleStartRLHF`, `pollRLHFStatus`, notifications  |
| `internal/lidar/monitor/assets/sweep_dashboard.css`              | **Modify** | `.rlhf-mode`, `.rlhf-only`, `.auto-or-rlhf` classes                      |
| `web/src/routes/lidar/sweeps/+page.svelte`                       | **Modify** | RLHF mode badge, round history, label link, carryover count              |
| `web/src/lib/api.ts`                                             | **Modify** | Add `startRLHF`, `getRLHFState`, `continueRLHF`, `stopRLHF`              |
| `web/src/lib/types/lidar.ts`                                     | **Modify** | Add `RLHFState`, `RLHFRound`, `LabelProgress` types                      |
| `tools/visualiser-macos/VelocityVisualiser/UI/ContentView.swift` | **Modify** | Show existing labels in panel, remove Export Labels button               |
| `tools/visualiser-macos/VelocityVisualiser/App/AppState.swift`   | **Modify** | Remove `exportLabels()`, wire selected RunTrack to LabelPanelView        |

## Testing Strategy

1. **Unit tests** (`rlhf_test.go`):
   - Duration indexing: verify wrap-around behaviour for short duration lists.
   - State machine transitions: idle → running_reference → awaiting_labels →
     running_sweep → running_reference → … → completed.
   - Continue signal unblocks wait.
   - Continue rejected when below 90% threshold (returns error).
   - Label carryover: temporal IoU matching with threshold 0.5.
   - Label carryover: IoU < 0.5 is not carried over.
   - Context cancellation stops the loop.
   - Bound narrowing carries over between rounds.
   - `continueFromLabels` with `nextDuration` overrides sweep duration.
   - `continueFromLabels` with `addRound` increments total rounds.
   - Scoring weight adjustments applied for round 1 vs round 2+.
   - Deadline expiry with insufficient labels → `"failed"` state.

2. **Integration tests** (manual):
   - Start RLHF with a short PCAP scene, 2 rounds, durations `[1, 1]`.
   - Verify reference run appears in Runs page.
   - Verify browser notification fires on `awaiting_labels` transition.
   - Label < 90% of tracks, verify Continue is disabled/rejected.
   - Label ≥ 90% of tracks, click Continue.
   - Verify sweep starts and produces results.
   - Verify second reference run uses narrowed params.
   - Verify labels are carried over from round 1 (check counts).
   - Verify carried-over labels display in macOS app.
   - Edit next-round duration, verify it takes effect.
   - Add an extra round, verify it executes.
   - Verify final recommendation is applied.

3. **Dashboard tests** (`sweep_dashboard.test.ts`):
   - Mode switching to/from "rlhf" shows/hides correct cards.
   - RLHF start request is built correctly from UI inputs.
   - Continue button disabled when progress < 90%.
   - Continue request includes `next_sweep_duration_mins` and `add_round`.
   - Notification permission requested on mode switch.
   - Polling renders correct phase displays.

4. **macOS visualiser tests** (`AppStateTests.swift`):
   - LabelPanelView shows existing `userLabel` when track is selected.
   - LabelPanelView shows existing `qualityLabel` when track is selected.
   - Export Labels button is removed (UI snapshot test or manual check).
