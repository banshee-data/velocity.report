# Velocity Report Expansion Plan Inspired by an Industry Standard ML Solver

## Document Intent

This document translates practices from an industry standard ML solver into a concrete expansion path for Velocity Report’s LiDAR tuning and human-feedback stack. It focuses on current-state reality, near-term opportunities around RLHF sweep mode, and a phased implementation sequence that improves experimentation speed, model trust, and operational reproducibility.

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
  - This is the critical prerequisite for RLHF and closed-loop human-in-the-loop optimisation.

### 1.2 Architectural inflection point

Velocity Report is transitioning from **proxy-metric optimisation** (acceptance/alignment/fragmentation heuristics) to **human-validated optimisation** (ground truth and RLHF).

That transition creates a new requirement:

> The system must support fast iteration and deep interpretability at the same time.

Current architecture supports this partially (results + metrics + dashboard), but the RLHF plan implies a need for richer abstractions:

- Label provenance and carry-over confidence.
- Round-to-round explainability (“why this parameter set improved”).
- Better separation between feature engineering, scoring rules, and optimisation strategy.
- Stronger experiment reproducibility contracts.

### 1.3 RLHF plan status assessment (`docs/plans/rlhf-sweep-mode.md`)

The RLHF plan is comprehensive and implementation-ready at design level. It already specifies:

- New request/state structures.
- A round-based RLHF state machine.
- API surface (`start`, `status`, `continue`, `stop`).
- Dashboard UX contract for round progress, thresholds, and continuation controls.
- Label carry-over and human gating behaviour.

However, the plan currently behaves mostly as a **feature plan**, not yet a **platform plan**. The gap is not correctness; the gap is **long-term scalability of experimentation**. Specifically:

1. **Scoring logic risks growing monolithic.**
   As more RLHF-specific scoring and weighting heuristics are added, objective code can become difficult to audit and compare over time.
2. **Feature transformations are implicit.**
   There is no explicit transform layer for derived signals used by objective/scoring (e.g., round-normalized metrics, class imbalance corrections, uncertainty penalties).
3. **Explainability is currently output-oriented, not decomposition-oriented.**
   The system can show aggregate scores, but needs score component decomposition and “top contributing factors” to improve operator trust and labelling quality.
4. **Experiment schema versioning is under-specified.**
   RLHF creates longitudinal experiments; reproducibility requires explicit versioning of transforms, scoring formulas, weight sets, and eligibility filters.
5. **Search strategy is primarily grid narrowing.**
   This is good for deterministic coverage but should be complemented with low-cost stochastic or adaptive search to reduce compute and improve early-round exploration.

---

## 2) Transferable Learnings from an Industry Standard ML Solver

### 2.1 Explicit schema for examples and model artefacts

A major strength is using typed example/model schema contracts that stay stable across training, scoring, and debugging workflows.

**Relevance for Velocity Report:**

- Define explicit schemas for:
  - RLHF round records.
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
- Cleaner RLHF heuristics (round-aware behaviour becomes data-driven config, not ad hoc code branches).

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

Mirror this with RLHF lifecycle stages:

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

Evolve sweep/auto/RLHF from a feature set into a reusable **Optimisation Platform** with the following properties:

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
   - Pluggable objective definitions (`weighted`, `acceptance`, `ground_truth`, `rlhf_composite_v2`, etc.).
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

## 4) RLHF-Specific Enhancements to Prioritise

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

### 4.5 Hybrid search mode for RLHF rounds

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

### Phase C — RLHF Quality and Explainability

- Add class/time coverage gates to `continue` validation.
- Add RLHF explanation card in dashboard and Svelte sweeps UI.
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

4. **Risk: RLHF throughput bottleneck (human labelling time).**
   - Mitigation: carry-over confidence + priority labelling queues + label quality gates.

---

## 8) Success Criteria

Define success as measurable deltas:

- **Optimisation efficiency**: fewer evaluated combos to hit target quality.
- **Human efficiency**: reduced labelling time per useful round.
- **Trust**: increased operator acceptance of recommendations due to explainability.
- **Reproducibility**: ability to replay and compare historical sweeps across objective versions.

---

## 9) Immediate Next Actions

1. Implement schema/version stamp fields in sweep persistence.
2. Add score component breakdown support in objective code paths.
3. Extend RLHF continue validation with optional class/time coverage checks.
4. Add explanation payload rendering in sweep dashboard and Svelte sweeps page.
5. Start a small hybrid-search experiment behind a config flag for RLHF rounds.

These actions preserve existing behaviour while laying platform foundations for scalable, interpretable, human-guided optimisation.
