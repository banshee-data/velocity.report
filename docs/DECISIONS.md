# Executive Decisions Register

Resolved design decisions across velocity.report. All 16 decisions are closed; the summary table below links each to its source document and records the outcome.

---

## Decision Register

| ID   | Decision                              | Resolution                                                        | Milestone | Source                                                                                                                        |
| ---- | ------------------------------------- | ----------------------------------------------------------------- | --------- | ----------------------------------------------------------------------------------------------------------------------------- |
| D-01 | Fused transit schema                  | Defer until Phase B                                               | —         | [VISION §4.1](VISION.md), [TDL plan](plans/data-track-description-language-plan.md)                                           |
| D-02 | FFT radar feed ingestion              | Defer to v2.0                                                     | v2.0      | [VISION §3.1](VISION.md)                                                                                                      |
| D-03 | Transit deduplication                 | Delete-before-insert with model version tracking                  | v0.7      | [transit-deduplication.md](radar/architecture/transit-deduplication.md)                                                       |
| D-04 | Geometry-coherent tracking (P1 maths) | Schedule for v0.6 cycle                                           | v0.6      | [proposal](maths/proposals/20260222-geometry-coherent-tracking.md), [maths README](maths/README.md)                           |
| D-05 | Maths proposal sequencing             | P1 → P2 → P4 → P3 confirmed                                       | v0.6–v2.0 | [maths README](maths/README.md)                                                                                               |
| D-06 | OBB heading fixes D/E/F               | Skip; P1 supersedes                                               | —         | [OBB heading review](maths/proposals/20260222-obb-heading-stability-review.md)                                                |
| D-07 | Track labelling UI (Phase 9)          | Complete Phase 9 Swift UI for v0.7                                | v0.7      | [track-labelling plan](plans/lidar-track-labeling-auto-aware-tuning-plan.md)                                                  |
| D-08 | LaTeX footprint reduction             | Vendored TeX tree + precompiled `.fmt`                            | v0.6      | [precompiled LaTeX plan](plans/pdf-latex-precompiled-format-plan.md), [RPi imager §4.6](plans/deploy-rpi-imager-fork-plan.md) |
| D-09 | Single binary architecture            | Single binary with subcommands                                    | v0.6      | [distribution packaging plan](plans/deploy-distribution-packaging-plan.md)                                                    |
| D-10 | RPi image tier strategy               | pi-gen + precompiled `.fmt`, single tier; `.tex` source in `.zip` | v0.6      | [RPi imager plan](plans/deploy-rpi-imager-fork-plan.md)                                                                       |
| D-11 | ECharts → LayerChart migration        | Rewrite all 8 charts in v0.7                                      | v0.7      | [DESIGN §4](ui/DESIGN.md), [frontend consolidation](plans/web-frontend-consolidation-plan.md)                                 |
| D-12 | Web palette (percentile colours)      | Svelte compliant now; ECharts fixed in v0.7                       | v0.5–v0.7 | [DESIGN §3.3](ui/DESIGN.md), [design review](ui/design-review-and-improvement.md)                                             |
| D-13 | Widescreen content containment        | Defer to v0.7 frontend consolidation                              | v0.7      | [DESIGN §5.7](ui/DESIGN.md), [design review](ui/design-review-and-improvement.md)                                             |
| D-14 | Simplification & deprecation scope    | Plan confirmed; Phase 1 in v0.6, removal in v0.7                  | v0.6      | [simplification plan](plans/platform-simplification-and-deprecation-plan.md)                                                  |
| D-15 | Time-partitioned data tables          | Implement for v1.0                                                | v1.0      | [time-partitioned tables plan](radar/architecture/time-partitioned-data-tables.md)                                            |
| D-16 | Speed limit schedules                 | Add to BACKLOG P2; scored 14 → v0.7 placement                     | v0.7      | [speed-limit-schedules.md](radar/architecture/speed-limit-schedules.md)                                                       |

---

## Milestone Placement

The decision matrix provides a repeatable framework for prioritising features and determining their milestone placement. Each feature is scored against five criteria, with weighted scores summed to produce a total that determines the milestone assignment.

### Criteria

| Criterion             | Weight       | Description                                                       |
| --------------------- | ------------ | ----------------------------------------------------------------- |
| **User value**        | 3×           | Direct benefit to the target user (neighbourhood change-makers)   |
| **Privacy alignment** | 2×           | Maintains or strengthens the privacy-first guarantee              |
| **Dependency unlock** | 2×           | Enables or unblocks other high-value features                     |
| **Effort**            | 1× (inverse) | Smaller effort scores higher (3 = S, 2 = M, 1 = L, 0 = XL)        |
| **Platform maturity** | 1×           | Reduces tech debt, improves reliability, or simplifies operations |

### Thresholds

| Milestone | Minimum Weighted Score | Rationale                                                      |
| --------- | ---------------------- | -------------------------------------------------------------- |
| v0.5      | ≥ 18                   | Highest-impact stabilisation work already in progress          |
| v0.6      | ≥ 16                   | Deployment blockers that gate user adoption                    |
| v0.7      | ≥ 14                   | Frontend and data-layer polish for v1.0 readiness              |
| v1.0      | ≥ 12                   | Everything needed for "production-ready" contract              |
| v2.0      | ≥ 8                    | Advanced features, connected capabilities, research graduation |
| Deferred  | < 8                    | Speculative, targets different users, or prerequisite missing  |

### Scored Features

| Feature                       | User Value (3×) | Privacy (2×) | Dep. Unlock (2×) | Effort (1×) | Platform (1×) | Total  | Milestone |
| ----------------------------- | --------------- | ------------ | ---------------- | ----------- | ------------- | ------ | --------- |
| RPi imager pipeline           | 3 (9)           | 3 (6)        | 3 (6)            | 1 (1)       | 2 (2)         | **24** | v0.6      |
| Precompiled LaTeX             | 2 (6)           | 2 (4)        | 3 (6)            | 2 (2)       | 3 (3)         | **21** | v0.6      |
| Frontend consolidation        | 3 (9)           | 2 (4)        | 2 (4)            | 1 (1)       | 3 (3)         | **21** | v0.7      |
| Single binary + subcommands   | 3 (9)           | 2 (4)        | 2 (4)            | 1 (1)       | 3 (3)         | **21** | v0.6      |
| SWEEP/HINT hardening          | 1 (3)           | 2 (4)        | 3 (6)            | 2 (2)       | 3 (3)         | **18** | v0.5      |
| Python venv consolidation     | 1 (3)           | 2 (4)        | 2 (4)            | 3 (3)       | 3 (3)         | **17** | v0.5      |
| SQLite client standardisation | 2 (6)           | 2 (4)        | 2 (4)            | 2 (2)       | 3 (3)         | **19** | v0.7      |
| Coverage ≥ 95.5%              | 1 (3)           | 2 (4)        | 1 (2)            | 1 (1)       | 3 (3)         | **13** | v1.0      |
| Unified settling (L3/L4)      | 1 (3)           | 2 (4)        | 3 (6)            | 1 (1)       | 2 (2)         | **16** | v1.0      |
| Geometry-prior local file     | 2 (6)           | 3 (6)        | 2 (4)            | 2 (2)       | 1 (1)         | **19** | v1.0      |
| Visualiser QC programme       | 2 (6)           | 2 (4)        | 1 (2)            | 0 (0)       | 2 (2)         | **14** | v2.0      |
| ML classifier pipeline        | 2 (6)           | 2 (4)        | 2 (4)            | 1 (1)       | 1 (1)         | **16** | v2.0      |
| Online geometry-prior service | 2 (6)           | 1 (2)        | 2 (4)            | 1 (1)       | 1 (1)         | **14** | v2.0      |
| AV dataset integration        | 0 (0)           | 2 (4)        | 0 (0)            | 0 (0)       | 1 (1)         | **5**  | Deferred  |
| Motion capture architecture   | 0 (0)           | 2 (4)        | 0 (0)            | 0 (0)       | 1 (1)         | **5**  | Deferred  |
| Speed limit schedules         | 2 (6)           | 2 (4)        | 1 (2)            | 1 (1)       | 1 (1)         | **14** | v0.7      |

### Key Placement Rationale

**Why RPi imager is v0.6, not v0.5:** The imager depends on precompiled LaTeX (image size) and the single binary (packaging). These prerequisites must land first. v0.5 stabilises the platform so v0.6 can focus on packaging without chasing regressions.

**Why frontend consolidation is v0.7, not v0.6:** Phase 3 (ECharts → LayerChart rewrite for 8 charts) requires ~2–3 weeks alone. Shipping the deployment story (v0.6) first ensures early adopters can install the system; the unified frontend is polish for v1.0 readiness.

**Why speed limit schedules is v0.7 (D-16):** Scores 14 on the decision matrix. School zone monitoring directly serves neighbourhood advocates, fitting the v0.7 "Radar Polish" theme. The existing `site_config_periods` SCD Type 6 pattern provides the schema foundation; schedules extend it with time-of-day/day-of-week rules.

**Why geometry-prior local file is v1.0, not earlier:** The vector-scene map schema must be stable before the online service (v2.0) can adopt it. Shipping the local file format in v1.0 locks the data contract. The maths are well-defined ([ground-plane-vector-scene proposal](maths/proposals/20260221-ground-plane-vector-scene-maths.md)) but runtime integration depends on unified settling (also v1.0).

**Why QC programme is v2.0, not v1.0:** The QC programme spans 10 features with cross-dependencies (M1–M5 milestones internally). It targets the LiDAR labelling workflow, which is experimental in v1.0. Shipping v1.0 without QC keeps the release scope achievable.

**Why ML classifier is v2.0:** Requires labelled training data generated by the QC programme. Also dependent on Phase 4.1 of the LiDAR ML pipeline, which sequences after track labelling (Phase 4.0, already complete) and parameter tuning (Phase 4.2).

### Dependency Graph

```
v0.5 (Platform Hardening)
  │
  ├── SWEEP/HINT hardening ──────────────────────────────┐
  ├── Settling optimisation Phase 3 ──────────────────────┤
  ├── Python venv consolidation                           │
  └── Documentation standardisation                       │
                                                          │
v0.6 (Deployment & Packaging)                             │
  │                                                       │
  ├── Precompiled LaTeX ─────────────────► RPi imager ◄───┘
  ├── Single binary + subcommands ─────► GitHub Releases
  ├── One-line install script
  ├── Platform simplification Phase 1
  └── Geometry-coherent tracking (P1, D-04)
        │
v0.7 (Unified Frontend)
  │
  ├── Frontend consolidation (Phases 0–5)
  │     ├── ECharts → LayerChart rewrite (D-11)
  │     └── Retire port 8081
  ├── Track labelling Phase 9 UI (D-07)
  ├── SQLite client standardisation
  ├── Transit deduplication (D-03)
  ├── Profile comparison system
  ├── Speed limit schedules (D-16)
  └── Accessibility + widescreen polish (D-13)
        │
v1.0 (Production-Ready)
  │
  ├── Coverage ≥ 95.5%
  ├── Platform simplification complete
  ├── LiDAR foundations fix-it
  ├── Velocity-coherent extraction (P2, D-05) ───────────┐
  ├── Unified settling (L3/L4, P4, D-05) ◄──────────────┘
  ├── Geometry-prior local file (GeoJSON) ◄── unified settling
  ├── Data export (CSV, GeoJSON)
  ├── Time-partitioned raw data tables
  └── Stable public API (versioned)
        │
v2.0 (Advanced Perception & Connected)
  │
  ├── Visualiser QC programme ────► ML classifier pipeline
  ├── Ground-plane vector-scene maths (P3, D-05)
  ├── Dynamic algorithm selection
  ├── Online geometry-prior service ◄── local file schema (v1.0)
  ├── Multi-location aggregate dashboard
  ├── Speed threshold alerts
  └── Peak-hour / seasonal trend analysis
```

### Principles

1. **Ship the install story early.** Users cannot evaluate the product if they cannot install it. Deployment and packaging (v0.6) takes priority over UI polish (v0.7) and test coverage (v1.0).

2. **Stabilise before expanding.** Each milestone hardens the layer below before building the layer above. v0.5 stabilises internals; v0.6 packages them; v0.7 polishes the interface; v1.0 certifies quality.

3. **Privacy is a feature, not a constraint.** Every milestone must maintain the privacy guarantee. Online features (v2.0) are opt-in and transmit geometry only.

4. **Local-only is the default forever.** The online geometry-prior service (v2.0) enriches the system but is never required. A disconnected Raspberry Pi with local prior files must produce the same quality results.

5. **Defer what targets different users.** AV dataset integration, motion capture, and range-image formats serve autonomous-vehicle researchers, not neighbourhood change-makers. These remain deferred until the core product is mature.

6. **Score, don't guess.** The decision matrix provides a repeatable framework. When a new feature request arrives, score it against the five criteria and place it in the milestone whose threshold it meets.
