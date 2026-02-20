# Velocity-Coherent Foreground Extraction Implementation Plan and Checklist

**Status:** Planning (Not Yet Implemented)
**Date:** February 20, 2026
**Owner:** LIDAR Pipeline Team
**Version:** 2.0

This document is now the implementation plan and execution checklist.

The mathematical model, parameter tradeoffs, and expected benefits are documented in:
- [`docs/lidar/future/velocity-coherent-foreground-extraction-math.md`](./velocity-coherent-foreground-extraction-math.md)

---

## Scope

Build a velocity-coherent foreground extraction path that runs alongside the current background-subtraction path and improves sparse-object continuity.

### In Scope

- Per-point velocity estimation across frames
- Position+velocity clustering (6D metric behavior)
- Long-tail lifecycle states (pre-tail, post-tail)
- Sparse continuation down to 3 points with stricter velocity checks
- Fragment merge heuristics for split tracks
- Dual-source storage/API for side-by-side evaluation

### Out of Scope (for this plan)

- Replacing the existing background-subtraction pipeline
- New ML model training
- New sensor calibration procedures

---

## Baseline (Current System)

- Active extractor: `ProcessFramePolarWithMask` in `internal/lidar/l3grid/foreground.go`
- Active clustering: DBSCAN in `internal/lidar/clustering.go`
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

### Phase 1: Point-Level Velocity Estimation

**Goal:** Compute velocity vectors and confidence for points with stable frame-to-frame correspondence.

Checklist:
- [ ] Create `internal/lidar/velocity_estimation.go`
- [ ] Implement correspondence search with configurable radius and plausibility gates
- [ ] Implement velocity confidence scoring
- [ ] Add `internal/lidar/velocity_estimation_test.go` with synthetic and replayed edge cases
- [ ] Add config wiring for velocity estimation parameters

Exit criteria:
- [ ] Velocity output generated for >95% of matchable points on validation segments
- [ ] Implausible velocity rates bounded by configured threshold
- [ ] Unit tests cover no-match, ambiguous-match, and high-noise cases

### Phase 2: Velocity-Coherent Clustering

**Goal:** Cluster points using position+velocity coherence and support `MinPts=3` mode.

Checklist:
- [ ] Create `internal/lidar/clustering_6d.go`
- [ ] Implement 6D neighborhood metric (position + velocity weighting)
- [ ] Implement minimum-point behavior with sparse guardrails
- [ ] Add `internal/lidar/clustering_6d_test.go`
- [ ] Validate cluster stability versus existing DBSCAN on replay data

Exit criteria:
- [ ] 3-point sparse clusters are accepted only with velocity coherence
- [ ] False-positive growth stays within agreed threshold versus baseline
- [ ] Runtime impact is measured and documented

### Phase 3: Long-Tail Lifecycle (Pre-Tail and Post-Tail)

**Goal:** Extend track continuity at object entry and exit boundaries.

Checklist:
- [ ] Create `internal/lidar/long_tail.go`
- [ ] Add pre-tail predicted entry association logic
- [ ] Add post-tail prediction window and uncertainty growth logic
- [ ] Extend track states and transitions
- [ ] Add `internal/lidar/long_tail_test.go`

Exit criteria:
- [ ] Mean track duration increases on boundary-entry/exit scenarios
- [ ] Recovery after brief occlusions improves without large precision drop
- [ ] State machine transitions are fully test-covered

### Phase 4: Sparse Continuation

**Goal:** Preserve track identity through low-point-count observations.

Checklist:
- [ ] Create `internal/lidar/sparse_continuation.go`
- [ ] Implement adaptive tolerances by point count
- [ ] Enforce confidence and variance gates for 3-5 point frames
- [ ] Integrate sparse continuation decisions into tracker updates
- [ ] Add targeted tests for 3-point continuation and failure boundaries

Exit criteria:
- [ ] Sparse tracks are maintained when motion is coherent
- [ ] No significant increase in ID switches in sparse scenes
- [ ] Parameter sensitivity documented for tuning

### Phase 5: Track Fragment Merging

**Goal:** Merge split track fragments when kinematics are consistent.

Checklist:
- [ ] Create `internal/lidar/track_merge.go`
- [ ] Implement candidate generation using time/position/velocity gates
- [ ] Implement merge scoring and deterministic tie-breaking
- [ ] Add `internal/lidar/track_merge_test.go`
- [ ] Record merge decisions for audit/debug

Exit criteria:
- [ ] Fragmentation rate decreases on occlusion-heavy validation runs
- [ ] Incorrect merge rate remains below agreed threshold
- [ ] Merge audit trail is queryable

### Phase 6: Pipeline, Storage, and API Integration

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
- [ ] No regression in existing source behavior

### Phase 7: Validation and Rollout

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

- Risk: Low `MinPts` increases noise clusters
  - Mitigation: strict velocity-confidence and variance gates in sparse mode
- Risk: Over-aggressive post-tail prediction causes ghost tracks
  - Mitigation: uncertainty growth caps and hard prediction timeout
- Risk: Incorrect fragment merges
  - Mitigation: conservative merge threshold + auditable merge logs
- Risk: Runtime overhead from added matching/clustering
  - Mitigation: bounded neighborhood queries and benchmark gates in CI

---

## Dependencies

- Stable world-frame transform quality from existing pose pipeline
- Representative replay datasets with known corner cases
- Dashboard/query support for dual-source comparison

---

## Milestones

1. M1: Phase 1 complete with validated velocity estimates
2. M2: Phase 2 complete with stable sparse clustering
3. M3: Phase 3-4 complete with long-tail and sparse continuation
4. M4: Phase 5 complete with audited fragment merging
5. M5: Phase 6-7 complete with rollout decision package

---

## Related Docs

- [`docs/lidar/future/velocity-coherent-foreground-extraction-math.md`](./velocity-coherent-foreground-extraction-math.md)
- [`docs/lidar/future/static-pose-alignment-plan.md`](./static-pose-alignment-plan.md)
- [`docs/lidar/future/motion-capture-architecture.md`](./motion-capture-architecture.md)
