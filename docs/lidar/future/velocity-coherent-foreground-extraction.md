# Velocity-Coherent Foreground Extraction Implementation Plan and Checklist

**Status:** Planning (Not Yet Implemented)
**Date:** February 20, 2026
**Owner:** LIDAR Pipeline Team
**Version:** 2.0

This document is now the implementation plan and execution checklist.

The mathematical model, parameter tradeoffs, and expected benefits are documented in:

- [`docs/maths/proposal/20260220-velocity-coherent-foreground-extraction.md`](../../maths/proposal/20260220-velocity-coherent-foreground-extraction.md)

---

## Scope

Build a velocity-coherent foreground extraction path that runs alongside the current background-subtraction path and improves sparse-object continuity.

### In Scope

- Cluster-level velocity inheritance from Kalman track state
- Two-stage clustering: spatial DBSCAN + velocity refinement/split
- Kalman-based track coasting with uncertainty growth
- Sparse continuation down to 3 points with continuous tolerance scaling
- Variance-aware log-likelihood fragment merge scoring
- Parallel foreground channel union (EMA/range + temporal-gradient)
- Dual-source storage/API for side-by-side evaluation

### Out of Scope (for this plan)

- Replacing the existing background-subtraction pipeline
- New ML model training
- New sensor calibration procedures

---

## Baseline (Current System)

- Active extractor: `ProcessFramePolarWithMask` in `internal/lidar/l3grid/foreground.go`
- Active clustering: DBSCAN in l4perception layer (`internal/lidar/l4perception/cluster.go`, via `internal/lidar/aliases.go`)
- No `VelocityCoherentTracker` implementation currently present

---

## Delivery Plan

### Phase 0: Instrumentation and Fixtures

**Goal:** Establish repeatable evaluation before algorithm changes.

Checklist:

- [ ] Define evaluation datasets (PCAP segments: dense traffic, sparse/distant traffic, occlusion-heavy)
- [ ] Add baseline metrics capture for current pipeline
- [ ] Add benchmark harness for frame throughput and memory
- [ ] Lock acceptance report format for side-by-side comparisons

Exit criteria:

- [ ] Reproducible baseline report generated from one command
- [ ] Baseline includes precision/recall proxy, track duration, fragmentation rate, and throughput

### Phase 1: Cluster-Level Velocity Inheritance

**Goal:** Wire cluster-level velocity from existing Kalman track state into the clustering pipeline.

Checklist:

- [ ] Create `internal/lidar/velocity_model.go`
- [ ] Implement velocity inheritance from L5 Kalman track to L4 cluster
- [ ] Implement velocity noise model: `Σ_v(i) = Σ_v,track(i) + Σ_v,floor`
- [ ] Handle untracked/new clusters with `v_i = 0` fallback
- [ ] Add `internal/lidar/velocity_model_test.go` with tracked/untracked cases
- [ ] Add config wiring for `clustering.split.sigma2_v` and `clustering.split.q_min`

Exit criteria:

- [ ] All tracked clusters inherit velocity from Kalman track state
- [ ] Untracked clusters have zero velocity with floor covariance
- [ ] Velocity covariance model includes track uncertainty + floor
- [ ] Unit tests cover tracked, untracked, and confidence edge cases

### Phase 2: Two-Stage Velocity-Split Clustering

**Goal:** Implement two-stage clustering with spatial DBSCAN followed by velocity refinement/split.

Checklist:

- [ ] Create `internal/lidar/clustering_velocity_split.go`
- [ ] Implement Stage 1: preserve existing spatial DBSCAN (no MinPts changes)
- [ ] Implement Stage 2: velocity variance evaluation within spatial clusters
- [ ] Implement Mahalanobis-like metric: `D²(u_i, u_j) = Δx^T Σ_x^{-1} Δx + Δv^T Σ_v^{-1} Δv`
- [ ] Implement velocity split logic using variance threshold and confidence gates
- [ ] Add config wiring for `clustering.metric.eps_sigma`
- [ ] Add `internal/lidar/clustering_velocity_split_test.go`
- [ ] Validate cluster stability versus existing DBSCAN on replay data

Exit criteria:

- [ ] Spatial clusters remain robust with existing MinPts behaviour
- [ ] Velocity splits occur only when variance exceeds threshold
- [ ] Normalised metric eliminates unit-mismatch issues
- [ ] False-positive growth stays within agreed threshold versus baseline
- [ ] Runtime impact is measured and documented

### Phase 3: Kalman-Based Track Coasting

**Goal:** Extend track continuity using Kalman covariance propagation with uncertainty growth.

Checklist:

- [ ] Create `internal/lidar/track_coasting.go`
- [ ] Implement Kalman propagation: `P(τ) = F P_0 F^T + Q(τ)`
- [ ] Implement Mahalanobis association gating for predicted tracks
- [ ] Add prediction/coast window with configurable horizon (`tracking.predict.tau_max_s`)
- [ ] Extend track states and transitions for coasting mode
- [ ] Add `internal/lidar/track_coasting_test.go`

Exit criteria:

- [ ] Mean track duration increases on boundary-entry/exit scenarios
- [ ] Recovery after brief occlusions improves without large precision drop
- [ ] Uncertainty grows correctly with coast duration
- [ ] State machine transitions are fully test-covered

### Phase 4: Sparse Continuation with Continuous Tolerance Scaling

**Goal:** Preserve track identity through low-point-count observations using continuous scaling.

Checklist:

- [ ] Create `internal/lidar/sparse_continuation.go`
- [ ] Implement continuous tolerance scaling: `tolerance(n) = k / sqrt(n)`
- [ ] Add physically plausible clamping bounds
- [ ] Add config wiring for `tracking.sparse.k_tol`
- [ ] Enforce confidence and variance gates for 3-5 point frames
- [ ] Integrate sparse continuation decisions into tracker updates
- [ ] Add targeted tests for 3-point continuation and failure boundaries

Exit criteria:

- [ ] Sparse tracks are maintained when motion is coherent
- [ ] No significant increase in ID switches in sparse scenes
- [ ] Tolerance scaling is continuous (not discrete tiers)
- [ ] Parameter sensitivity documented for tuning

### Phase 5: Parallel Foreground Channel Union

**Goal:** Implement parallel foreground channels with union policy to improve sparse recall.

Checklist:

- [ ] Create `internal/lidar/l3grid/foreground_temporal.go`
- [ ] Implement Channel A: existing EMA/range foreground logic (no changes)
- [ ] Implement Channel B: temporal-gradient motion foreground
- [ ] Implement union policy: `foreground = A OR B`
- [ ] Add config wiring for `foreground.temporal.enabled`, `foreground.temporal.v_min_mps`, `foreground.channel_union_mode`
- [ ] Add `internal/lidar/l3grid/foreground_temporal_test.go`
- [ ] Validate false-positive rate versus baseline

Exit criteria:

- [ ] Both channels operate independently (no cyclic dependencies)
- [ ] Union output correctly combines both channels
- [ ] Temporal channel triggers on motion exceeding `v_min_mps`
- [ ] False-positive increase stays within agreed threshold
- [ ] Channel switching can be controlled via config

### Phase 6: Variance-Aware Track Fragment Merging

**Goal:** Merge split track fragments using log-likelihood scoring.

Checklist:

- [ ] Create `internal/lidar/track_merge.go`
- [ ] Implement candidate generation using time/position/velocity gates
- [ ] Implement variance-aware log-likelihood merge scoring: `Λ_merge = Σ component terms`
- [ ] Add merge acceptance threshold: `Λ_merge >= Λ_min` (config: `tracking.merge.lambda_min`)
- [ ] Implement deterministic tie-breaking
- [ ] Add `internal/lidar/track_merge_test.go`
- [ ] Record merge decisions for audit/debug

Exit criteria:

- [ ] Fragmentation rate decreases on occlusion-heavy validation runs
- [ ] Merge scoring uses variance weighting (not equal-weight averaging)
- [ ] Incorrect merge rate remains below agreed threshold
- [ ] Merge audit trail is queryable

### Phase 7: Pipeline, Storage, and API Integration

**Goal:** Run current and velocity-coherent paths in parallel and expose both results.

Checklist:

- [ ] Create `internal/lidar/velocity_coherent_tracker.go`
- [ ] Create dual extraction orchestration path (parallel source processing)
- [ ] Add storage schema for velocity-coherent clusters/tracks
- [ ] Add API source selector (`background_subtraction`, `velocity_coherent`, `all`)
- [ ] Add migration and rollback notes

Exit criteria:

- [ ] Both sources can be queried independently and jointly
- [ ] Dashboard comparison can be generated from stored results
- [ ] No regression in existing source behaviour

### Phase 8: Validation and Rollout

**Goal:** Decide production readiness from measured outcomes.

Checklist:

- [ ] Run full replay evaluation across selected PCAP suites
- [ ] Compare against baseline using agreed metrics
- [ ] Document default parameter set and safe bounds
- [ ] Add ops runbook (alerts, fallbacks, troubleshooting)
- [ ] Stage rollout behind feature flag

Exit criteria:

- [ ] Acceptance thresholds met on continuity and sparse-object capture
- [ ] Throughput and memory remain within service budget
- [ ] Rollback path verified in staging

---

## Acceptance Metrics (Target Bands)

These targets are hypotheses to validate, not committed production guarantees.

- Sparse-object recall (3-11 points): target relative lift `+20%` to `+40%`
- Track fragmentation rate: target relative reduction `10%` to `25%`
- Median track duration for boundary-crossing objects: target relative lift `10%` to `30%`
- Additional false positives: keep increase under `+10%` versus baseline
- Throughput impact: keep regression under `20%` at target frame rate

---

## Risks and Mitigations

- Risk: Velocity noise model over/under-confidence
  - Mitigation: explicit floor covariance and empirical tuning of `Σ_v,floor`
- Risk: Velocity splits in Stage 2 increase fragmentation
  - Mitigation: strict variance threshold and confidence gates (`clustering.split.q_min`)
- Risk: Over-aggressive Kalman coasting causes ghost tracks
  - Mitigation: uncertainty growth caps and hard prediction timeout (`tracking.predict.tau_max_s`)
- Risk: Incorrect fragment merges
  - Mitigation: conservative log-likelihood threshold (`tracking.merge.lambda_min`) + auditable merge logs
- Risk: Temporal foreground channel increases false positives
  - Mitigation: motion threshold (`foreground.temporal.v_min_mps`) and union-only policy (no cyclic feedback)
- Risk: Runtime overhead from added matching/clustering
  - Mitigation: two-stage design limits high-dimensional operations + benchmark gates in CI

---

## Dependencies

- Stable world-frame transform quality from existing pose pipeline
- Representative replay datasets with known corner cases
- Dashboard/query support for dual-source comparison

---

## Milestones

1. M1: Phase 1 complete with validated cluster-level velocity inheritance
2. M2: Phase 2 complete with stable two-stage clustering
3. M3: Phase 3-4 complete with Kalman coasting and sparse continuation
4. M4: Phase 5 complete with parallel foreground channel union
5. M5: Phase 6 complete with audited variance-aware fragment merging
6. M6: Phase 7-8 complete with rollout decision package

---

## Related Docs

- [`docs/maths/proposal/20260220-velocity-coherent-foreground-extraction.md`](../../maths/proposal/20260220-velocity-coherent-foreground-extraction.md)
- [`docs/lidar/future/static-pose-alignment-plan.md`](./static-pose-alignment-plan.md)
- [`docs/lidar/future/motion-capture-architecture.md`](./motion-capture-architecture.md)
