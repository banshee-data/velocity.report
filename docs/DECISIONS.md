# Executive Decisions Register

Resolved design decisions across velocity.report. All 16 decisions are closed; the summary table below links each to its source document and records the outcome.

---

## Decision Register

| ID   | Decision                              | Resolution                                             | Milestone | Source                                                                                                                        |
| ---- | ------------------------------------- | ------------------------------------------------------ | --------- | ----------------------------------------------------------------------------------------------------------------------------- |
| D-01 | Fused transit schema                  | Defer until Phase B                                    | —         | [VISION §4.1](VISION.md), [TDL plan](plans/data-track-description-language-plan.md)                                           |
| D-02 | FFT radar feed ingestion              | Defer to v2.0                                          | v2.0      | [VISION §3.1](VISION.md)                                                                                                      |
| D-03 | Transit deduplication                 | Delete-before-insert with model version tracking       | v0.7      | [transit-deduplication.md](radar/architecture/transit-deduplication.md)                                                       |
| D-04 | Geometry-coherent tracking (P1 maths) | Schedule for v0.6 cycle                                | v0.6      | [proposal](maths/proposals/20260222-geometry-coherent-tracking.md), [maths README](maths/README.md)                           |
| D-05 | Maths proposal sequencing             | P1 → P2 → P4 → P3 confirmed                            | v0.6–v2.0 | [maths README](maths/README.md)                                                                                               |
| D-06 | OBB heading fixes D/E/F               | Skip; P1 supersedes                                    | —         | [OBB heading review](maths/proposals/20260222-obb-heading-stability-review.md)                                                |
| D-07 | Track labelling UI (Phase 9)          | Complete Phase 9 Swift UI for v0.7                     | v0.7      | [track-labelling plan](plans/lidar-track-labeling-auto-aware-tuning-plan.md)                                                  |
| D-08 | LaTeX footprint reduction             | Vendored TeX tree + precompiled `.fmt`                 | v0.6      | [precompiled LaTeX plan](plans/pdf-latex-precompiled-format-plan.md), [RPi imager §4.6](plans/deploy-rpi-imager-fork-plan.md) |
| D-09 | Single binary architecture            | Single binary with subcommands                         | v0.6      | [distribution packaging plan](plans/deploy-distribution-packaging-plan.md)                                                    |
| D-10 | RPi image tier strategy               | pi-gen + TinyTeX, single tier; `.tex` source in `.zip` | v0.6      | [RPi imager plan](plans/deploy-rpi-imager-fork-plan.md)                                                                       |
| D-11 | ECharts → LayerChart migration        | Rewrite all 8 charts in v0.7                           | v0.7      | [DESIGN §4](ui/DESIGN.md), [frontend consolidation](plans/web-frontend-consolidation-plan.md)                                 |
| D-12 | Web palette (percentile colours)      | Svelte compliant now; ECharts fixed in v0.7            | v0.5–v0.7 | [DESIGN §3.3](ui/DESIGN.md), [design review](ui/design-review-and-improvement.md)                                             |
| D-13 | Widescreen content containment        | Defer to v0.7 frontend consolidation                   | v0.7      | [DESIGN §5.7](ui/DESIGN.md), [design review](ui/design-review-and-improvement.md)                                             |
| D-14 | Simplification & deprecation scope    | Plan confirmed; Phase 1 in v0.6, removal in v0.7       | v0.6      | [simplification plan](plans/platform-simplification-and-deprecation-plan.md)                                                  |
| D-15 | Time-partitioned data tables          | Implement for v1.0                                     | v1.0      | [time-partitioned tables plan](radar/architecture/time-partitioned-data-tables.md)                                            |
| D-16 | Speed limit schedules                 | Add to BACKLOG P2; score via decision matrix           | —         | [speed-limit-schedules.md](radar/architecture/speed-limit-schedules.md)                                                       |

---

## ROADMAP / BACKLOG Alignment

| Item                          | ROADMAP | BACKLOG | Resolution                                                        |
| ----------------------------- | ------- | ------- | ----------------------------------------------------------------- |
| Python venv consolidation     | v0.5    | P1      | ✅ Aligned — promoted to P1                                       |
| Documentation standardisation | v0.5    | P2      | ✅ Aligned — promoted to P2                                       |
| Profile comparison system     | v0.7    | P2      | ✅ Consistent — P2 maps to v0.7                                   |
| Precompiled LaTeX             | v0.6    | P1      | ✅ Aligned — promoted to P1                                       |
| Speed limit schedules         | —       | P2      | ✅ Added to BACKLOG P2 (D-16)                                     |
| Cosine error correction       | —       | P2      | ✅ Largely implemented; document existing feature and review gaps |
| Track labelling UI (D-07)     | v2.0    | P2      | ✅ Phase 9 UI moved to v0.7 per D-07                              |

---

## Remaining Actions

1. **Update ROADMAP.md** — add D-04 (geometry-coherent tracking v0.6), D-07 (track labelling UI v0.7), D-10 (.tex source in report .zip, TinyTeX runtime), D-13 (widescreen v0.7), and single-tier RPi image.
2. **Promote VISION.md from Draft** — change status to Active once fused transit schema work begins.
3. **Begin P1 maths work** — geometry-coherent tracking (D-04) scheduled for v0.6 cycle.
4. **Document cosine error correction** — review existing DB/API/frontend implementation and write user-facing feature documentation.
5. **Score speed limit schedules** — run through the decision matrix (ROADMAP §3) to determine milestone placement.
