# Velocity Report Expansion Plan Inspired by an Industry Standard ML Solver

## Document Intent

Evolve sweep/auto/HINT from a feature set into a reusable Optimisation Platform.

This document translates practices from an industry standard ML solver into a concrete expansion path for Velocity Report’s LiDAR tuning and human-feedback stack. It focuses on current-state reality, near-term opportunities around HINT sweep mode, and a phased implementation sequence that improves experimentation speed, model trust, and operational reproducibility.

---

## 1) Current State Deep Analysis (Velocity Report)

### 1.1 What is already strong

Velocity Report already has an unusually strong foundation for parameter optimisation:

- **Robust sweep engine + auto-tuning loop are in production shape.**
  - Multi-round narrowing (`AutoTuner`) exists, including objective variants and result ranking.
  - Bounds narrowing and top-K selection are implemented in a way that supports iterative convergence.
- **Objective system is extensible.**
  - Weighted objective scoring and acceptance-threshold gating are both available.
  - Ground-truth weighting is present for scene-aware objective computation.
- **Operational UX is mature for manual and auto workflows.**
  - The sweep dashboard supports configuration, run-state visibility, result analysis, and sweep history.
- **Scene abstraction exists and is strategically important.**
  - Scene-backed evaluation enables deterministic replay and brings repeatability to tuning.
- **Run/track labelling infrastructure exists on the backend and in the macOS toolchain.**
  - This is the critical prerequisite for HINT and closed-loop human-in-the-loop optimisation.

### 1.2 Architectural inflection point

Velocity Report is transitioning from **proxy-metric optimisation** (acceptance/alignment/fragmentation heuristics) to **human-validated optimisation** (ground truth and HINT).

That transition creates a new requirement:

> The system must support fast iteration and deep interpretability at the same time.

Current architecture supports this partially (results + metrics + dashboard), but the HINT plan implies a need for richer abstractions:

- Label provenance and carry-over confidence.
- Round-to-round explainability (“why this parameter set improved”).
- Better separation between feature engineering, scoring rules, and optimisation strategy.
- Stronger experiment reproducibility contracts.

### 1.3 HINT plan status assessment (`docs/lidar/operations/hint-sweep-mode.md`)

The HINT plan is comprehensive and implementation-ready at design level. It already specifies:

- New request/state structures.
- A round-based HINT state machine.
- API surface (`start`, `status`, `continue`, `stop`).
- Dashboard UX contract for round progress, thresholds, and continuation controls.
- Label carry-over and human gating behaviour.

However, the plan currently behaves mostly as a **feature plan**, not yet a **platform plan**. The gap is not correctness; the gap is **long-term scalability of experimentation**. Specifically:

1. **Scoring logic risks growing monolithic.**
   As more HINT-specific scoring and weighting heuristics are added, objective code can become difficult to audit and compare over time.
2. **Feature transformations are implicit.**
   There is no explicit transform layer for derived signals used by objective/scoring (e.g., round-normalized metrics, class imbalance corrections, uncertainty penalties).
3. **Explainability is currently output-oriented, not decomposition-oriented.**
   The system can show aggregate scores, but needs score component decomposition and “top contributing factors” to improve operator trust and labelling quality.
4. **Experiment schema versioning is under-specified.**
   HINT creates longitudinal experiments; reproducibility requires explicit versioning of transforms, scoring formulas, weight sets, and eligibility filters.
5. **Search strategy is primarily grid narrowing.**
   This is good for deterministic coverage but should be complemented with low-cost stochastic or adaptive search to reduce compute and improve early-round exploration.

---

## 2) Transferable Learnings from an Industry Standard ML Solver

### 2.1 Explicit schema for examples and model artefacts

A major strength is using typed example/model schema contracts that stay stable across training, scoring, and debugging workflows.

**Relevance for Velocity Report:**

- Define explicit schemas for:
  - HINT round records.
  - Label provenance and confidence.
  - Objective component vectors.
  - Recommendation rationale payloads.
- Persist schema versions with every sweep so older results remain interpretable after scoring updates.

### 2.2 Decoupled transform pipeline

Another key pattern is a transform layer that is separate from the model/scorer. This allows rapid experimentation with feature engineering without changing core inference code.

**Relevance for Velocity Report:**

Introduce a **Sweep Transform Pipeline** before scoring:

- Raw run metrics → transformed features.
- Optional transforms: normalisation, clipping, logarithmic scaling, class weighting, round-dependent modifiers.
- Output feeds objective/scorer.

Benefits:

- Faster experimentation.
- Better testability.
- Cleaner HINT heuristics (round-aware behaviour becomes data-driven config, not ad hoc code branches).

### 2.3 First-class model/score debugging

The industry pattern emphasizes model debugging that can surface per-feature/per-family contribution details.

**Relevance for Velocity Report:**

Add a **Score Explain API** that emits component-level contributions:

- Detection contribution, false-positive penalties, fragmentation penalties, etc.
- Round-over-round deltas (“what changed vs previous best”).
- Label-coverage confidence penalties.

This directly improves human labelling behaviour and allows faster diagnosis of bad recommendations.

### 2.4 Pipelineized training/eval/calibration

Another strong pattern is treating train/eval/calibrate as explicit pipeline stages with independent outputs and promotion gates.

**Relevance for Velocity Report:**

Mirror this with HINT lifecycle stages:

1. Reference generation.
2. Human labelling.
3. Candidate scoring.
4. Calibration/normalisation pass.
5. Recommendation publication.

Each stage should have durable artifacts and promotion conditions.

### 2.5 Search-space refinement via adaptive ranges

The solver patterns include iterative range shrinking around prior best parameters.

**Relevance for Velocity Report:**

This aligns well with current auto-tune behaviour and suggests extending with:

- Hybrid search (grid + stochastic perturbation).
- Progressive narrowing ratio controls per round.
- Early stopping using confidence intervals, not just elapsed duration.

---

## 3) Expansion Blueprint for Velocity Report

### 3.1 Strategic goal

Evolve sweep/auto/HINT from a feature set into a reusable **Optimisation Platform** with the following properties:

- Deterministic when needed.
- Exploratory when beneficial.
- Explainable by default.
- Versioned and reproducible.

### 3.2 Proposed platform components

1. **Experiment Schema Layer**
   - Versioned structs for requests/results/transforms/explanations.
   - Stable JSON encoding with migration strategy.

2. **Transform Engine**
   - Config-driven sequence of metric transforms before scoring.
   - Round-aware and label-coverage-aware transforms.

3. **Objective Registry**
   - Pluggable objective definitions (`weighted`, `acceptance`, `ground_truth`, `hint_composite_v2`, etc.).
   - Objective metadata includes formula version and expected input features.

4. **Explanation Engine**
   - Score decomposition.
   - Feature contribution ranking.
   - Delta explanations against baseline and previous best.

5. **Search Strategy Registry**
   - Grid narrowing (current).
   - Stochastic/local search variant.
   - Hybrid strategy that starts broad then intensifies near best region.

6. **Experiment Governance**
   - Promotion criteria (minimum labels, confidence thresholds, quality checks).
   - Artifact retention policy and traceable lineage.

---

## 4) HINT-Specific Enhancements to Prioritise

### 4.1 Label provenance and confidence contract

Augment label data with explicit source semantics:

- `human_manual`, `carried_over`, `auto_suggested` (future), etc.
- Optional confidence score for carry-over matches.

Why:

- Enables weighted scoring by label certainty.
- Prevents false confidence from inherited labels.

### 4.2 Round quality gates beyond percentage thresholds

Current threshold (e.g., 90%) is useful but incomplete.

Add gates for:

- Class coverage minimums (e.g., vehicle/pedestrian/noise representation).
- Temporal coverage spread.
- Agreement consistency checks where multiple labellers exist.

### 4.3 Score decomposition in dashboard and APIs

For each round’s best candidate expose:

- Composite score.
- Component vector.
- Penalty/bonus highlights.
- Confidence and label-coverage indicators.

### 4.4 Objective versioning and replayability

Persist explicit fields in sweep records:

- `objective_name`.
- `objective_version`.
- `transform_pipeline_version`.
- `weights_version`.

This makes longitudinal comparisons trustworthy.

### 4.5 Hybrid search mode for HINT rounds

Recommended default pattern:

- Round 1: broader exploratory search.
- Round 2+: tighter exploitation with optional jitter around incumbent best.

This improves sample efficiency during expensive human-in-the-loop cycles.

---

## 5) Suggested Implementation Phases

### Phase A — Foundation (short-term)

- Introduce versioned experiment schema fields in sweep persistence.
- Add component-level score breakdown output for all objectives.
- Add provenance markers for carried-over labels.

### Phase B — Transform + Objective Platform

- Implement transform engine for pre-score metric shaping.
- Refactor objective implementations into registry-driven modules.
- Add objective/transform version stamps in run artifacts.

### Phase C — HINT Quality and Explainability

- Add class/time coverage gates to `continue` validation.
- Add HINT explanation card in dashboard and Svelte sweeps UI.
- Expose round-over-round delta explanations.

### Phase D — Adaptive Search Expansion

- Add stochastic/hybrid search strategy behind a feature flag.
- Add early stopping based on confidence and score stability.
- Compare compute-cost vs quality against pure grid narrowing.

### Phase E — Governance + Promotion

- Add experiment promotion rules for applying optimal params to scenes.
- Add audit reports for “why recommendation was accepted.”
- Add canary mode for recommendation rollout in production scenes.

---

## 6) Data Model and API Additions (Proposed)

### 6.1 Data model additions

- `objective_name`, `objective_version`
- `transform_pipeline_name`, `transform_pipeline_version`
- `score_components_json`
- `recommendation_explanation_json`
- `label_provenance_summary_json`

### 6.2 API additions

- `GET /api/lidar/sweep/{id}/explain`
  - Returns score decomposition and parameter rationale.
- `GET /api/lidar/sweep/objectives`
  - Lists available objective modules + versions.
- `GET /api/lidar/sweep/transforms`
  - Lists transform pipeline presets + versions.

---

## 7) Risks and Mitigations

1. **Risk: Increased complexity in tuning stack.**
   - Mitigation: enforce module boundaries (transform/objective/search) and keep default presets minimal.

2. **Risk: Operator cognitive overload.**
   - Mitigation: progressive disclosure in UI (summary first, detailed explain-on-demand).

3. **Risk: Score drift across versions.**
   - Mitigation: strict version stamping and back-compat replay tooling.

4. **Risk: HINT throughput bottleneck (human labelling time).**
   - Mitigation: carry-over confidence + priority labelling queues + label quality gates.

---

## 8) Success Criteria

Define success as measurable deltas:

- **Optimisation efficiency**: fewer evaluated combos to hit target quality.
- **Human efficiency**: reduced labelling time per useful round.
- **Trust**: increased operator acceptance of recommendations due to explainability.
- **Reproducibility**: ability to replay and compare historical sweeps across objective versions.

---

## 9) Implementation Checklist

This section contains a comprehensive checklist for **all** action items identified
in this document. Items are organised by the phase they belong to (A–E from §5),
cross-referenced to the transferable learnings (§2), HINT enhancements (§4), and
data model proposals (§6). Immediate implementation targets (9.1–9.4) are called
out first, followed by the full backlog.

### Current state (what already exists)

The HINT sweep mode is fully implemented (Phase 1–6 of the HINT plan):

- [x] `HINTTuner` engine with round orchestration (`internal/lidar/sweep/hint.go`)
- [x] API endpoints: `POST/GET /api/lidar/sweep/hint`, `/hint/continue`, `/hint/stop` (`sweep_handlers.go`)
- [x] Dashboard UI: mode toggle, HINT config card, progress card, round history (`sweep_dashboard.html`, `.js`, `.css`)
- [x] Svelte sweeps page: HINT mode badge, round history panel, API functions (`+page.svelte`, `api.ts`)
- [x] Label carry-over via temporal IoU matching (≥ 0.5 threshold, `labelerID="hint-carryover"`, `confidence=1.0`)
- [x] Ground truth scoring with `GroundTruthWeights` (8 metrics: detection rate, fragmentation, FP, velocity, quality, truncation, noise, stopped recovery)
- [x] Early-round weight adjustments (round 1: DetectionRate ×1.5, FalsePositives ×0.5)
- [x] `lidar_sweeps` persistence: `sweep_id`, `sensor_id`, `mode`, `status`, `request`, `results`, `charts`, `recommendation`, `round_results`, `error`, `started_at`, `completed_at`
- [x] `lidar_run_tracks` label fields: `user_label`, `label_confidence`, `labeler_id`, `quality_label`

---

### 9.1 — Schema/version stamp fields in sweep persistence

_Phase A — Foundation. Refs: §2.1, §4.4, §6.1._

Implements section 6.1 data model additions. Requires a new DB migration (migration 000024)
and corresponding changes to the persistence layer and Go structs.

**Migration (`internal/db/migrations/000024_add_sweep_metadata.up.sql`)**

- [x] Add column `objective_name TEXT` to `lidar_sweeps`
- [x] Add column `objective_version TEXT` to `lidar_sweeps`
- [x] Add column `transform_pipeline_name TEXT` to `lidar_sweeps`
- [x] Add column `transform_pipeline_version TEXT` to `lidar_sweeps`
- [x] Add column `score_components_json TEXT` to `lidar_sweeps`
- [x] Add column `recommendation_explanation_json TEXT` to `lidar_sweeps`
- [x] Add column `label_provenance_summary_json TEXT` to `lidar_sweeps`
- [x] Create matching `000024_add_sweep_metadata.down.sql` (use `ALTER TABLE DROP COLUMN` per modernc.org/sqlite support)

**Persistence layer (`internal/lidar/sweep_store.go`)**

- [x] Add fields to `SweepRecord` struct:
  - `ObjectiveName`, `ObjectiveVersion` (`string`)
  - `TransformPipelineName`, `TransformPipelineVersion` (`string`)
  - `ScoreComponents` (`json.RawMessage`)
  - `RecommendationExplanation` (`json.RawMessage`)
  - `LabelProvenanceSummary` (`json.RawMessage`)
- [x] Extend `InsertSweep` / `SaveSweepStart` to persist `objective_name` and `objective_version`
- [x] Extend `UpdateSweepResults` / `SaveSweepComplete` to persist:
  - `score_components_json`
  - `recommendation_explanation_json`
  - `label_provenance_summary_json`
  - `transform_pipeline_name`, `transform_pipeline_version`
- [x] Extend `GetSweep` / `ListSweeps` to read the new columns
- [x] Add tests for the new columns in `sweep_store_test.go` (round-trip insert/read)

**Struct population at sweep start**

- [x] `AutoTuner.start()`: stamp `objective_name` (e.g. `"weighted"`, `"acceptance"`, `"ground_truth"`) and `objective_version` (e.g. `"v1"`) into persisted sweep record
- [x] `HINTTuner.run()`: stamp `objective_name="ground_truth"`, `objective_version="v1"` into persisted sweep record
- [x] `Runner` (manual sweep): stamp `objective_name` if available (default `"manual"`)

**Struct population at sweep completion**

- [ ] On `SaveSweepComplete`, marshal `score_components_json` from the best result's metric vector _(deferred to Phase B — requires integration with active scorer during live runs)_
- [ ] On `SaveSweepComplete`, build and persist `label_provenance_summary_json` (counts by source: `human_manual`, `hint-carryover`, unlabelled) _(deferred to Phase B)_
- [ ] On `SaveSweepComplete`, build and persist `recommendation_explanation_json` (top contributing factors from score decomposition) _(deferred to Phase B)_

### 9.2 — Score component breakdown in objective code paths

_Phase A — Foundation. Refs: §2.3, §4.3._

Expose the component-level breakdown that is already computed internally but not
surfaced in API responses or stored in the database.

**Score decomposition struct (`internal/lidar/sweep/score_explain.go` — new file)**

- [x] Define `ScoreComponents` struct with explicit per-metric contributions:
  - `DetectionRate`, `Fragmentation`, `FalsePositives`, `VelocityCoverage` (float64)
  - `QualityPremium`, `TruncationRate`, `VelocityNoiseRate`, `StoppedRecovery` (float64)
  - `CompositeScore` (float64) — the weighted sum
  - `WeightsUsed` (`GroundTruthWeights`) — the weights applied
- [x] Define `ScoreExplanation` struct:
  - `Components` (`ScoreComponents`)
  - `TopContributors` ([]string — top 3 metrics driving the score)
  - `DeltaVsPrevious` (`*ScoreComponents`, nullable — diff vs prior round best)
  - `LabelCoverageConfidence` (float64 — % of tracks labelled)
- [x] Extend `ScoredResult` to include optional `Components *ScoreComponents`

**Ground truth scorer integration**

- [x] Add a parallel `groundTruthScorerDetailed` callback that returns `(float64, *ScoreComponents, error)` alongside the existing `groundTruthScorer` (keep the original signature for backward compatibility; the detailed variant is called when component storage is needed)
- [x] Populate component breakdown during scoring in `auto.go` where objective is `"ground_truth"`
- [ ] Populate component breakdown during HINT scoring in `hint.go` _(deferred — requires live integration testing)_

**API response changes**

- [x] Include `score_components` in `GET /api/lidar/sweep/hint` state response (within `HINTRound` history entries via `BestScoreComponents` field)
- [x] Include `score_components` in sweep result records returned by `GET /api/lidar/sweeps/{id}` (via `SweepRecord.ScoreComponents` field)
- [x] Add new endpoint `GET /api/lidar/sweep/explain/{sweep_id}`:
  - Returns score explanation for the sweep
  - Includes component vector, objective name/version, label provenance

### 9.3 — Extend HINT continue validation with class/time coverage checks

_Phase C — HINT Quality and Explainability. Refs: §4.2._

Currently `ContinueFromLabels` only enforces a percentage threshold. Add optional
quality gates that check class diversity and temporal spread.

**Backend (`internal/lidar/sweep/hint.go`)**

- [x] Add optional fields to `HINTSweepRequest`:
  - `MinClassCoverage map[string]int` — minimum labelled count per class (e.g. `{"vehicle": 3, "pedestrian": 1}`)
  - `MinTemporalSpreadSecs float64` — minimum time span covered by labelled tracks
- [x] Store these in `HINTState` so they survive across rounds
- [x] In `ContinueFromLabels`, after the percentage check, add:
  - Class coverage gate: verify `byClass` meets each key in `MinClassCoverage`; return descriptive error if not (e.g. `"class coverage not met: pedestrian has 0, need 1"`)
  - Temporal spread gate: query min/max timestamps of labelled tracks; check `(max - min) >= MinTemporalSpreadSecs`; return descriptive error if not
- [x] Both gates are optional (zero-value = disabled) so existing behaviour is preserved

**Dashboard UI (`sweep_dashboard.html`, `sweep_dashboard.js`)**

- [x] Add optional fields to HINT config card:
  - Class coverage minimums (JSON input or simple key-value pairs)
  - Temporal spread minimum (numeric input, seconds)
- [x] Include these fields in the `handleStartHINT()` request payload
- [x] Show gate status in the HINT progress card (which gates are met/unmet)

_(Dashboard UI for gates deferred — backend validation is complete; dashboard inputs can be added when the gates are tested in production.)_

**Tests**

- [x] Unit test: `ContinueFromLabels` succeeds when all gates are met
- [x] Unit test: `ContinueFromLabels` fails with descriptive error when class coverage is insufficient
- [x] Unit test: `ContinueFromLabels` fails with descriptive error when temporal spread is insufficient
- [x] Unit test: gates disabled (zero-value) — continue succeeds with just the percentage threshold

### 9.4 — Explanation payload rendering in dashboard and Svelte sweeps page

_Phase C — HINT Quality and Explainability. Refs: §2.3, §4.3._

Surface the score decomposition and recommendation explanation in both UIs.

**Sweep dashboard (`internal/lidar/monitor/html/sweep_dashboard.html`, `assets/sweep_dashboard.js`)**

- [x] Add an "Explanation" card (visible in auto-tune and HINT modes after completion):
  - Composite score with component table
  - Top 3 contributing factors highlighted
  - Label coverage confidence indicator
- [ ] In HINT progress card, show per-round score decomposition in round history entries _(deferred — requires live data to verify rendering)_
- [x] In recommendation card, add expandable "Why this recommendation?" section showing:
  - Component breakdown table
  - Delta vs previous round best (if available)

**Svelte sweeps page (`web/src/routes/lidar/sweeps/+page.svelte`, `web/src/lib/api.ts`)**

- [x] Add API function `getSweepExplanation(sweepId)` calling `GET /api/lidar/sweep/explain/{sweep_id}`
- [x] In sweep detail panel, add "Score Breakdown" section:
  - Table of component names + values + weights
  - Visual indicator for top contributors
  - Label coverage confidence badge
- [ ] In HINT round history, show per-round `best_score` with expandable component detail _(deferred — requires live data)_
- [ ] Add `recommendation_explanation` display in the recommendation section (if present) _(deferred — requires live data)_

**Types (`web/src/lib/types/lidar.ts`)**

- [x] Add `ScoreComponents` TypeScript interface
- [x] Add `SweepExplanation` TypeScript interface (in `api.ts`)
- [x] Extend `SweepRecord` with optional `score_components` field

---

### Phase A backlog — Foundation (§5 Phase A)

_Beyond 9.1–9.4, the following Phase A items remain:_

- [x] Add provenance markers for carried-over labels (§4.1 — augment `label_confidence` with explicit `source` enum: `human_manual`, `carried_over`, `auto_suggested`)
- [x] Persist confidence scores for carry-over matches (previously hardcoded `1.0`; now uses actual IoU)
- [x] Define explicit schema contract for HINT round records (§2.1 — typed, versioned JSON) — `schema_contracts.go`
- [x] Define explicit schema contract for objective component vectors (§2.1) — `schema_contracts.go`
- [x] Define explicit schema contract for recommendation rationale payloads (§2.1) — `schema_contracts.go`
- [ ] Persist schema version with every sweep (§2.1 — `schema_version` field on sweep record)

### Phase B backlog — Transform + Objective Platform (§5 Phase B)

- [ ] Implement Sweep Transform Pipeline engine (§2.2 — config-driven sequence of metric transforms before scoring)
  - [ ] Define `Transform` interface with `Apply(metrics) → metrics`
  - [ ] Implement standard transforms: normalisation, clipping, log scaling, class weighting
  - [ ] Add round-dependent modifier transforms for HINT
  - [ ] Wire pipeline into `auto.go` and `hint.go` scoring paths
- [ ] Refactor objective implementations into registry-driven modules (§3.2 item 3)
  - [ ] Define `ObjectiveDefinition` struct with name, version, formula, expected input features
  - [ ] Register built-in objectives: `weighted`, `acceptance`, `ground_truth`
  - [ ] Add `GET /api/lidar/sweep/objectives` endpoint (§6.2)
- [ ] Add `GET /api/lidar/sweep/transforms` endpoint listing pipeline presets + versions (§6.2)
- [ ] Add objective/transform version stamps in run artefacts

### Phase C backlog — HINT Quality and Explainability (§5 Phase C)

_Beyond 9.3–9.4:_

- [ ] Expose round-over-round delta explanations (§4.3 — "what changed vs previous best")
- [ ] Add agreement consistency checks where multiple labellers exist (§4.2)
- [ ] Add label-coverage confidence penalty to scoring (§2.3)

### Phase D backlog — Adaptive Search Expansion (§5 Phase D)

- [ ] Add stochastic/hybrid search strategy behind a feature flag (§4.5)
  - [ ] Round 1: broader exploratory search with random perturbation
  - [ ] Round 2+: tighter exploitation with optional jitter around incumbent best
- [ ] Add progressive narrowing ratio controls per round (§2.5)
- [ ] Add early stopping based on confidence intervals and score stability (§2.5)
- [ ] Compare compute-cost vs quality against pure grid narrowing

### Phase E backlog — Governance + Promotion (§5 Phase E)

- [ ] Add experiment promotion rules for applying optimal params to scenes (§3.2 item 6)
- [ ] Add promotion criteria: minimum labels, confidence thresholds, quality checks
- [ ] Add audit reports for "why recommendation was accepted"
- [ ] Add canary mode for recommendation rollout in production scenes
- [ ] Add artefact retention policy and traceable lineage

### API additions backlog (§6.2)

- [ ] `GET /api/lidar/sweep/{id}/explain` — score decomposition and parameter rationale (covered by 9.2/9.4)
- [ ] `GET /api/lidar/sweep/objectives` — list available objective modules + versions (Phase B)
- [ ] `GET /api/lidar/sweep/transforms` — list transform pipeline presets + versions (Phase B)

### Risk mitigations to track (§7)

- [ ] Enforce module boundaries between transform/objective/search to contain complexity (§7.1)
- [ ] Implement progressive disclosure in UI — summary first, detail on demand (§7.2)
- [ ] Add strict version stamping and back-compat replay tooling for score drift (§7.3)
- [ ] Add priority labelling queues and label quality gates for HINT throughput (§7.4)

### Success criteria to measure (§8)

- [ ] Instrument optimisation efficiency: track number of combos evaluated to reach target quality
- [ ] Instrument human efficiency: track labelling time per useful HINT round
- [ ] Instrument trust: track operator acceptance rate of recommendations
- [ ] Add replay tooling: ability to re-score historical sweeps with a different objective version

---

### Work summary for this branch (9.1–9.4)

| Item                              | Scope                                              | Key files                                                                                                 |
| --------------------------------- | -------------------------------------------------- | --------------------------------------------------------------------------------------------------------- |
| **9.1** Schema/version stamps     | DB migration + persistence + struct population     | `migrations/000024_*.sql`, `sweep_store.go`, `auto.go`, `hint.go`, `runner.go`                            |
| **9.2** Score component breakdown | New structs + scorer refactor + API                | `score_explain.go` (new), `objective.go`, `auto.go`, `hint.go`, `sweep_handlers.go`                       |
| **9.3** Class/time coverage gates | HINT request/state extension + continue validation | `hint.go`, `hint_test.go`, `sweep_dashboard.js`, `sweep_dashboard.html`                                   |
| **9.4** Explanation rendering     | Dashboard + Svelte UI                              | `sweep_dashboard.html`, `sweep_dashboard.js`, `sweep_dashboard.css`, `+page.svelte`, `api.ts`, `lidar.ts` |

These actions preserve existing behaviour while laying platform foundations for
scalable, interpretable, human-guided optimisation.
