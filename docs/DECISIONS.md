# Executive Decisions Register

Closed design decisions across velocity.report. This register records the outcome of each decision and links to its source document. Milestone assignments come from [BACKLOG.md](BACKLOG.md), which is the single source of truth for scheduling.

This file should only be edited once or twice per sprint (2-week period) when there are blockers or open questions that require a recorded decision. It is not updated per-PR.

---

## Decision Register

| ID   | Decision                              | Resolution                                                        | Source                                                                                                                        |
| ---- | ------------------------------------- | ----------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------- |
| D-01 | Fused transit schema                  | Defer until Phase B                                               | [VISION §4.1](VISION.md), [TDL plan](plans/data-track-description-language-plan.md)                                           |
| D-02 | FFT radar feed ingestion              | Defer to v2.0                                                     | [VISION §3.1](VISION.md)                                                                                                      |
| D-03 | Transit deduplication                 | Delete-before-insert with model version tracking                  | [transit-deduplication.md](radar/architecture/transit-deduplication.md)                                                       |
| D-04 | Geometry-coherent tracking (P1 maths) | Schedule for v0.6 cycle                                           | [proposal](maths/proposals/20260222-geometry-coherent-tracking.md), [maths README](maths/README.md)                           |
| D-05 | Maths proposal sequencing             | P1 → P2 → P4 → P3 confirmed                                       | [maths README](maths/README.md)                                                                                               |
| D-06 | OBB heading fixes D/E/F               | Skip; P1 supersedes                                               | [OBB heading review](maths/proposals/20260222-obb-heading-stability-review.md)                                                |
| D-07 | Track labelling UI (Phase 9)          | Complete Phase 9 Swift UI for v0.7                                | [track-labelling plan](plans/lidar-track-labeling-auto-aware-tuning-plan.md)                                                  |
| D-08 | LaTeX footprint reduction             | Vendored TeX tree + precompiled `.fmt`                            | [precompiled LaTeX plan](plans/pdf-latex-precompiled-format-plan.md), [RPi imager §4.6](plans/deploy-rpi-imager-fork-plan.md) |
| D-09 | Single binary architecture            | Single binary with subcommands                                    | [distribution packaging plan](plans/deploy-distribution-packaging-plan.md)                                                    |
| D-10 | RPi image tier strategy               | pi-gen + precompiled `.fmt`, single tier; `.tex` source in `.zip` | [RPi imager plan](plans/deploy-rpi-imager-fork-plan.md)                                                                       |
| D-11 | ECharts → LayerChart migration        | Rewrite all 8 charts in v0.7                                      | [DESIGN §4](ui/DESIGN.md), [frontend consolidation](plans/web-frontend-consolidation-plan.md)                                 |
| D-12 | Web palette (percentile colours)      | Svelte compliant now; ECharts fixed in v0.7                       | [DESIGN §3.3](ui/DESIGN.md), [design review](ui/design-review-and-improvement.md)                                             |
| D-13 | Widescreen content containment        | Defer to v0.7 frontend consolidation                              | [DESIGN §5.7](ui/DESIGN.md), [design review](ui/design-review-and-improvement.md)                                             |
| D-14 | Simplification & deprecation scope    | Plan confirmed; Phase 1 complete in v0.5, removal in v0.7       | [simplification plan](plans/platform-simplification-and-deprecation-plan.md)                                                  |
| D-15 | Time-partitioned data tables          | Implement for v1.0                                                | [time-partitioned tables plan](radar/architecture/time-partitioned-data-tables.md)                                            |
| D-16 | Speed limit schedules                 | v0.8 placement (radar theme)                                      | [speed-limit-schedules.md](radar/architecture/speed-limit-schedules.md)                                                       |
| D-17 | PDF generation migration to Go        | Go SVG charts + `text/template` LaTeX; eliminate Python stack     | [PDF Go chart migration plan](plans/pdf-go-chart-migration-plan.md)                                                           |

### Milestone Rationale

| Milestone | Rationale                                                      |
| --------- | -------------------------------------------------------------- |
| v0.5      | Highest-impact stabilisation work already in progress          |
| v0.6      | Deployment blockers that gate user adoption                    |
| v0.7      | Frontend and data-layer polish for v1.0 readiness              |
| v0.8      | Radar polish, CI automation, and post-frontend follow-through  |
| v1.0      | Everything needed for "production-ready" contract              |
| v2.0      | Advanced features, connected capabilities, research graduation |
| Deferred  | Speculative, targets different users, or prerequisite missing  |

## Milestone Placement

Milestone assignments live in [BACKLOG.md](BACKLOG.md). This section documents the principles that guide placement decisions.

### Principles

1. **Ship the install story early.** Users cannot evaluate the product if they cannot install it. Deployment and packaging (v0.6) takes priority over UI polish (v0.7) and test coverage (v1.0).

2. **Stabilise before expanding.** Each milestone hardens the layer below before building the layer above. v0.5 stabilises internals; v0.6 packages them; v0.7 polishes the interface; v1.0 certifies quality.

3. **Privacy is a feature, not a constraint.** Every milestone must maintain the privacy guarantee. Online features (v2.0) are opt-in and transmit geometry only.

4. **Local-only is the default forever.** The online geometry-prior service (v2.0) enriches the system but is never required. A disconnected Raspberry Pi with local prior files must produce the same quality results.

5. **Defer what targets different users.** AV dataset integration, motion capture, and range-image formats serve autonomous-vehicle researchers, not neighbourhood change-makers. These remain deferred until the core product is mature.

6. **Scope milestones for focus.** Each milestone should have a clear theme and a manageable number of items (~10–12 max). When a milestone grows beyond that, split by theme or sequencing into the next milestone slot. Thematic coherence reduces context-switching and improves delivery predictability.
