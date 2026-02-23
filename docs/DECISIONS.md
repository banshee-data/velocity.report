# Outstanding Executive Decisions

Status: Active
Purpose: Comprehensive register of open design decisions across velocity.report, linking to source documents with options and proposed trade-offs to drive resolution and release coherence.

---

## How to Use This Document

Each decision is numbered (D-*nn*) and grouped by domain. Every entry
links to its source document, states the options considered, and flags a
recommended path where the evidence supports one. Resolve each decision
by recording the outcome inline and removing it from the "open" count in
the summary table below.

### Decision Status Summary

| Domain | Open | Resolved | Source Documents |
| --- | --- | --- | --- |
| Sensor Fusion & Data | 3 | 0 | VISION, TDL plan, radar arch |
| LiDAR Perception | 4 | 0 | maths proposals, ROADMAP |
| Deployment & Packaging | 3 | 0 | RPi imager, LaTeX, distribution plans |
| Frontend & UI | 3 | 0 | DESIGN, frontend consolidation, design review |
| Platform & Infrastructure | 3 | 0 | simplification, SQLite, coverage plans |
| **Total** | **16** | **0** | |

---

## 1  Sensor Fusion & Data

### D-01  Fused Transit Schema Definition

**Source:** [VISION.md §4.1](VISION.md) · [TDL plan](plans/data-track-description-language-plan.md)

**Context:** The vision defines a fused transit record combining radar speed authority with LiDAR spatial authority. The `fused_transits` table is spec'd in the TDL plan (~200 bytes/row, 5 indexes) but the schema has not been implemented.

**Options:**

| Option | Pros | Cons |
| --- | --- | --- |
| **A — Materialised table** (TDL plan proposal) | Fast percentile queries on RPi (<50 ms); simple indexes; clear write boundary | Extra write path; schema migration needed |
| **B — View over existing tables** | No new storage; always consistent | Slower queries on RPi; complex join across radar + LiDAR; hard to index |
| **C — Defer until Phase B** (current state) | No risk of premature schema lock | Blocks TDL, fused PDF reports, and description interface |

**Recommendation:** Option A, scheduled for v0.7 alongside SQLite client standardisation. The TDL plan's schema is well-defined and the storage cost is trivial (~10 MB/year).

**Depends on:** D-02 (FFT ingestion decision), SQLite client standardisation.

---

### D-02  FFT Radar Feed Ingestion

**Source:** [VISION.md §3.1](VISION.md)

**Context:** The OPS243 FFT feed (`OF`/`of` commands) is allowed but ingestion is not implemented. FFT enables multi-target separation and spectral signature analysis.

**Options:**

| Option | Pros | Cons |
| --- | --- | --- |
| **A — Implement in Phase A** (vision recommendation) | Completes radar sensor picture; enables multi-target disambiguation | Development effort (M); unclear user demand for single-sensor deployments |
| **B — Defer to v2.0** | Focus on packaging and deployment first; FFT value is highest with fused scene | Feature gap: some transits lose multi-target resolution |
| **C — Drop from scope** | Simplifies radar pipeline; fewer serial commands | Permanently limits multi-target capability |

**Recommendation:** Option B. FFT is valuable but not blocking for the v1.0 production contract. Revisit when sensor fusion (Phase B) lands.

---

### D-03  Transit Deduplication Strategy

**Source:** [transit-deduplication.md](../docs/radar/architecture/transit-deduplication.md)

**Context:** Duplicate transit records occur during hourly backfills and full reprocessing. The current proposal uses delete-before-insert with explicit model version tracking.

**Options:**

| Option | Pros | Cons |
| --- | --- | --- |
| **A — Delete-before-insert** (proposed) | Simple; deterministic; no duplicate accumulation | Requires version tracking; full backfill is destructive during window |
| **B — Upsert with composite key** | Idempotent; no delete window | Needs stable composite key definition; harder to reason about conflicts |
| **C — Append-only with dedup view** | No mutations; audit trail preserved | Storage growth; query complexity; RPi disk constraints |

**Recommendation:** Option A as proposed. Schedule for v0.7 alongside SQLite client standardisation.

---

## 2  LiDAR Perception

### D-04  Geometry-Coherent Tracking (P1 Maths Proposal)

**Source:** [geometry-coherent-tracking proposal](maths/proposals/20260222-geometry-coherent-tracking.md) · [maths README](maths/README.md)

**Context:** Highest-priority maths proposal. Replaces reactive OBB guards (Guard 2/3, dim sync) with a Bayesian per-track geometry model. Fixes visible spinning bounding boxes. Estimated 6–7 days effort.

**Options:**

| Option | Pros | Cons |
| --- | --- | --- |
| **A — Implement before v0.5 ships** | Fixes most visible LiDAR artefact; no dependencies | Delays v0.5 stabilisation; 6–7 days effort |
| **B — Schedule for v0.6 cycle** | v0.5 ships on time; guards provide partial mitigation | Spinning boxes remain in v0.5 experimental LiDAR |
| **C — Defer to v1.0** (alongside unified settling) | Groups maths work together | Long delay; degrades LiDAR credibility |

**Recommendation:** Option B. Ship v0.5 with current guards, implement geometry-coherent tracking early in the v0.6 cycle. The existing Guard 3 (90° jump rejection) provides adequate interim mitigation.

---

### D-05  Maths Proposal Sequencing (P1–P4)

**Source:** [maths README §Proposals](maths/README.md)

**Context:** Four maths proposals are queued. The README prioritises them P1–P4 but none have started. Sequencing affects what ships when.

**Priority order (current):**

| Priority | Proposal | Effort | Dependencies |
| --- | --- | --- | --- |
| P1 | Geometry-coherent tracking | 6–7 days | None |
| P2 | Velocity-coherent foreground extraction | 5–8 days | Independent (improves P1) |
| P3 | Ground-plane vector-scene maths | 4–6 days | Infrastructure |
| P4 | Unify L3/L4 settling | 3–5 days | Reduces complexity |

**Decision needed:** Confirm sequencing. P1 has no dependencies and the highest visual impact. P2 is independent but improves P1 results. P3 and P4 are infrastructure — P4 is listed for v1.0 in the ROADMAP.

**Recommendation:** Confirm P1 → P2 → P4 → P3. Move P4 (unify settling) before P3 (ground-plane) because it reduces operational complexity that aids P3 implementation. This reorders the last two but preserves the ROADMAP's v1.0 settling target.

---

### D-06  OBB Heading Stability — Remaining Fixes (D, E, F)

**Source:** [OBB heading stability review](maths/proposals/20260222-obb-heading-stability-review.md)

**Context:** Guard 3 and fixes B, C, G are implemented. Fixes D (canonical-axis normalisation), E (EMA-smoothed dimensions), and F (per-axis confidence weighting) are pending — but the geometry-coherent proposal (D-04) supersedes them.

**Options:**

| Option | Pros | Cons |
| --- | --- | --- |
| **A — Implement D/E/F incrementally** | Immediate partial improvement | Throwaway work if P1 lands |
| **B — Skip; let P1 supersede** (proposed in review) | No wasted effort | Spinning boxes persist until P1 ships |

**Recommendation:** Option B. The review document already marks D/E/F as superseded by P1. Confirm and close.

---

### D-07  LiDAR Track Labelling UI Completion

**Source:** [track-labelling plan](plans/lidar-track-labeling-auto-aware-tuning-plan.md)

**Context:** Phases 1–5 approved and complete. Phase 6 (seekable replay enhancements) deferred. Phase 9 partial: data layer done, Swift UI pending. The labelling UI gates the ML classifier (v2.0).

**Options:**

| Option | Pros | Cons |
| --- | --- | --- |
| **A — Complete Phase 9 UI for v0.7** | Unblocks ML training pipeline sooner | Swift development effort; competes with frontend consolidation |
| **B — Defer Phase 9 UI to v2.0** | Focus v0.7 on web frontend; ML isn't needed until v2.0 | Longer gap before labelled training data accumulates |

**Recommendation:** Option B. The ML classifier is a v2.0 feature. Labelled data can accumulate via the existing (partial) UI; the Swift-native enhancements can ship with the QC programme.

---

## 3  Deployment & Packaging

### D-08  LaTeX Footprint Reduction Strategy

**Source:** [precompiled LaTeX plan](plans/pdf-latex-precompiled-format-plan.md) · [RPi imager plan §4.6](plans/deploy-rpi-imager-fork-plan.md)

**Context:** Full TeX Live is ~800 MB. The RPi image needs this under 60 MB. The plan proposes a vendored TeX tree with a precompiled `.fmt` file. This is on the critical path to v0.6.

**Options:**

| Option | Pros | Cons |
| --- | --- | --- |
| **A — Vendored TeX tree + precompiled .fmt** (proposed) | 60 MB footprint; reproducible; works offline | Custom packaging; version-locked to specific TeX packages |
| **B — Thin TeX install with runtime download** | Smaller base image | Requires internet on first use; breaks offline-first principle |
| **C — Switch PDF generation to a non-LaTeX engine** | Eliminates TeX dependency entirely | Major rewrite; LaTeX quality is a differentiator |

**Recommendation:** Option A as proposed. It preserves the offline-first principle and professional-quality output. This is a v0.6 critical-path item.

---

### D-09  Single Binary Architecture

**Source:** [distribution packaging plan](plans/deploy-distribution-packaging-plan.md)

**Context:** The plan proposes consolidating `cmd/radar`, `cmd/deploy`, and `cmd/tools` into a single `velocity-report` binary with subcommands. This simplifies installation and enables the one-line install script.

**Options:**

| Option | Pros | Cons |
| --- | --- | --- |
| **A — Single binary with subcommands** (proposed) | One download; simpler packaging; cleaner path conventions | Larger binary; all features compiled in; harder to test independently |
| **B — Keep separate binaries, package together** | Independent compilation; smaller per-binary size | Harder to install; more files to manage; confusing for non-developers |

**Recommendation:** Option A. The combined binary size is acceptable for a Go application. Ship in v0.6 as planned.

---

### D-10  RPi Image Tier Strategy

**Source:** [RPi imager plan](plans/deploy-rpi-imager-fork-plan.md)

**Context:** The plan defines three flashing tiers (Lite/Standard/Full) with different levels of pre-configuration. The tier model affects image size, user experience, and maintenance burden.

**Decision needed:** Confirm tier definitions and whether all three ship in v0.6 or only the Standard tier, with others following later.

**Recommendation:** Ship Standard tier only in v0.6. Lite and Full tiers add maintenance burden with limited user demand at launch. Revisit for v0.7 or v1.0 based on adoption feedback.

---

## 4  Frontend & UI

### D-11  ECharts-to-LayerChart Migration Scope

**Source:** [DESIGN.md §4](../DESIGN.md) · [frontend consolidation plan](plans/web-frontend-consolidation-plan.md)

**Context:** DESIGN.md mandates LayerChart/d3-scale as the chart rendering stack. Eight ECharts charts in Go-embedded dashboards need rewriting. This is the dominant v0.7 workload (2–3 weeks).

**Options:**

| Option | Pros | Cons |
| --- | --- | --- |
| **A — Rewrite all 8 charts in v0.7** (planned) | Clean break; retire Go dashboards (~2,000 lines) | 2–3 weeks effort; risk of regressions |
| **B — Incremental migration across v0.7–v1.0** | Lower risk per release; can prioritise most-used charts | Dual rendering stacks maintained longer; inconsistent UX |
| **C — Keep ECharts, wrap in Svelte** | Fastest migration; reuses existing chart configs | Two chart libraries in production; fights DESIGN.md mandate |

**Recommendation:** Option A for v0.7 as planned, but establish a clear priority order: migrate the 3–4 most-used charts first, then retire Go dashboards, then migrate remaining charts. This gives an early exit point if v0.7 scope needs trimming.

---

### D-12  Web Palette Migration (Percentile Colours)

**Source:** [DESIGN.md §3.3](../DESIGN.md) · [design review §1.1](ui/design-review-and-improvement.md)

**Context:** The canonical percentile palette is defined (p50 #fbd92f, p85 #f7b32b, p98 #f25f5c, max #2d1e2f). Web `palette.ts` was created in PR #286 with compliant values, but ECharts charts in Go-embedded dashboards still use the old palette. Full compliance requires the ECharts migration (D-11).

**Options:**

| Option | Pros | Cons |
| --- | --- | --- |
| **A — Fix Svelte charts now, ECharts in v0.7** (recommended) | Svelte charts are compliant immediately; ECharts fixed when retired | Inconsistency between Svelte and Go dashboards in v0.5–v0.6 |
| **B — Wait for v0.7 to fix everything at once** | Single palette migration; no transitional inconsistency | Non-compliant palette ships in two more releases |

**Recommendation:** Option A. Svelte palette is already compliant (PR #286). Document the ECharts gap as a known limitation until v0.7.

---

### D-13  Widescreen Content Containment

**Source:** [DESIGN.md §5.7](../DESIGN.md) · [design review §2.2](ui/design-review-and-improvement.md)

**Context:** No max-width centering at ≥3,000 px. The design review flags this as a P2 item with a `vr-page` class placeholder.

**Options:**

| Option | Pros | Cons |
| --- | --- | --- |
| **A — Add max-width to vr-page now** | Quick fix (~1 hr); improves ultra-wide experience immediately | May need revisiting if layout patterns change |
| **B — Defer to v0.7 frontend consolidation** | Part of a coherent layout overhaul | Ultra-wide users see stretched content longer |

**Recommendation:** Option A. It is a small CSS change with no downstream risk.

---

## 5  Platform & Infrastructure

### D-14  Simplification & Deprecation Scope

**Source:** [simplification plan](plans/platform-simplification-and-deprecation-plan.md)

**Context:** The plan inventories Make targets, CLI flags, and `cmd/` applications for deprecation. It is a P1 backlog item and v0.6 feature, but the scope is large and the plan is still in Draft status.

**Decision needed:** Define the Phase 1 scope for v0.6. Which deprecations ship first?

**Recommendation:** Phase 1 should target: (1) retire `cmd/deploy` once single binary lands, (2) deprecation warnings on superseded Make targets, (3) flag old CLI options with "deprecated" messages. Defer removal to v0.7 after one release cycle of warnings.

---

### D-15  Time-Partitioned Data Tables

**Source:** [time-partitioned tables plan](../docs/radar/architecture/time-partitioned-data-tables.md)

**Context:** The plan proposes monthly/quarterly partition rotation for `radar_data`, `radar_objects`, and `lidar_bg_snapshot`. Includes security fixes (CVE-2025-VR-002/003/005/006) and union views for transparent access.

**Options:**

| Option | Pros | Cons |
| --- | --- | --- |
| **A — Implement for v1.0** (ROADMAP placement) | Addresses long-running deployments; security fixes included | Complex migration; query path changes |
| **B — Defer to v2.0** | Focus v1.0 on core stability | Disk usage grows unbounded on long-running RPi deployments |
| **C — Implement partition rotation only, without union views** | Simpler; handles disk growth | API must handle partitions explicitly; less transparent |

**Recommendation:** Option A for v1.0 as planned. Disk growth is a real concern for RPi deployments running months of continuous collection.

---

### D-16  Speed Limit Schedules (Time-Based Limits)

**Source:** [speed-limit-schedules.md](../docs/radar/architecture/speed-limit-schedules.md)

**Context:** Current model stores a single speed limit per site. The proposal adds time-based schedules (school zones, weekday/weekend). Not currently in the ROADMAP or BACKLOG.

**Options:**

| Option | Pros | Cons |
| --- | --- | --- |
| **A — Add to v1.0** | More accurate compliance reporting; school zone support | Schema change; API changes; UI work |
| **B — Add to v2.0** | Focus v1.0 on current single-limit model | Reports near schools are less accurate |
| **C — Add to BACKLOG P2 and score via decision matrix** | Follows established process | Delays prioritisation decision |

**Recommendation:** Option C. Score via the decision matrix (§3 of ROADMAP) and place in the appropriate milestone based on the result.

---

## Cross-Cutting: ROADMAP / BACKLOG Alignment

### Inconsistencies Found

| Item | ROADMAP Says | BACKLOG Says | Resolution |
| --- | --- | --- | --- |
| Python venv consolidation | v0.5 | P2 (Later) | **Promote to P1** in BACKLOG — it is small (S) and listed in v0.5 |
| Documentation standardisation | v0.5 | P3 (Deferred) | **Promote to P2** in BACKLOG — scheduled for v0.5 but not high-urgency; P2 is more appropriate than P3 |
| Profile comparison system | v0.7 | P2 (Later) | **Consistent** — P2 maps to v0.7 correctly |
| Precompiled LaTeX | v0.6 | P2 (Later) | **Promote to P1** in BACKLOG — it is on v0.6 critical path |
| Speed limit schedules | Not listed | Not listed | **Add to BACKLOG P2** and score via decision matrix (see D-16) |
| Cosine error correction | Not listed | Not listed | **Add to BACKLOG P3** — design exists but no implementation plan |

---

## Next Steps

1. **Resolve each D-*nn* decision** — record the outcome inline (strike through rejected options).
2. **Update BACKLOG.md** — apply the alignment fixes above (promote venv and LaTeX; add missing items).
3. **Update ROADMAP.md** — if any decisions change milestone placement.
4. **Promote VISION.md from Draft** — the content is stable; change status to Active once D-01 and D-02 are resolved.
5. **Begin P1 maths work** — geometry-coherent tracking (D-04) is the highest-impact open technical item.
6. **Re-score new items** — run speed limit schedules and cosine correction through the decision matrix.
