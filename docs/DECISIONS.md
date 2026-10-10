# Executive Decisions Register

<!-- ignore-style -->

Closed design decisions across velocity.report. This register records the outcome of each decision and links to its source document. Milestone assignments come from [BACKLOG.md](BACKLOG.md), which is the single source of truth for scheduling.

This file should only be edited once or twice per sprint (2-week period) when there are blockers or open questions that require a recorded decision. It is not updated per-PR.

---

## Decision Register

### D-01 — Fused transit schema

Defer until Phase B — [VISION §4.1](VISION.md), [TDL plan](plans/data-traffic-description-language-plan.md)

### D-02 — FFT radar feed ingestion

Defer to v2.0 — [VISION §3.1](VISION.md)

### D-03 — Transit deduplication

Delete-before-insert with model version tracking — [transit-deduplication.md](radar/architecture/transit-deduplication.md)

### D-04 — Geometry-coherent tracking (P1 maths)

Schedule for v0.6 cycle — [proposal](../data/maths/proposals/20260222-geometry-coherent-tracking.md), [MATHS.md](../data/maths/MATHS.md)

### D-05 — Maths proposal sequencing

P1 → P2 → P4 → P3 confirmed — [MATHS.md](../data/maths/MATHS.md)

### D-06 — OBB heading fixes D/E/F

Keep as guard-stack maintenance only: validate Fix D thresholds before changing
the shipped `0.25` default, leave Fixes E/F as debug-only follow-ups, and let
P1 supersede further guard expansion — [OBB heading review](../data/maths/proposals/20260222-obb-heading-stability-review.md)

### D-07 — Track labelling UI (Phase 9)

Complete Phase 9 Swift UI for v0.7 — [track-labelling plan](plans/lidar-track-labelling-auto-aware-tuning-plan.md)

### D-08 — Report footprint reduction

The report compiler footprint is removed from the Raspberry Pi image by using the embedded Typst engine in the Go binary. No separate report compiler tree ships in the image — [PDF reporting](platform/operations/pdf-reporting.md), [RPi imager](platform/operations/rpi-imager.md)

### D-09 — Single binary architecture

Single binary with subcommands and embedded report compiler assets — [distribution packaging plan](plans/deploy-distribution-packaging-plan.md)

### D-10 — RPi image tier strategy

pi-gen single tier; Typst source bundle in report `.zip` — [RPi imager plan](plans/deploy-rpi-imager-fork-plan.md)

### D-11 — ECharts → LayerChart migration

Report-view charts (time-series, histogram, comparison) are served as SVG by the Go chart package (`internal/report/chart`) and consumed directly by the Svelte frontend — not rewritten in LayerChart. Non-report charts (live dashboard, real-time stats) remain in scope for LayerChart migration in v0.7 — [DESIGN §4](ui/DESIGN.md), [frontend consolidation](plans/web-frontend-consolidation-plan.md), [PDF migration plan](plans/pdf-go-chart-migration-plan.md)

### D-12 — Web palette (percentile colours)

Svelte compliant now; ECharts fixed in v0.7 — [DESIGN §3.3](ui/DESIGN.md), [design review](ui/design-review-and-improvement.md)

### D-13 — Widescreen content containment

Defer to v0.7 frontend consolidation — [DESIGN §5.7](ui/DESIGN.md), [design review](ui/design-review-and-improvement.md)

### D-14 — Simplification & deprecation scope

Plan confirmed; Phase 1 complete in v0.5, removal before v0.6.0 — [simplification plan](plans/platform-simplification-and-deprecation-plan.md)

### D-15 — Time-partitioned data tables

Implement in v0.9.0 — [time-partitioned tables plan](radar/architecture/time-partitioned-data-tables.md)

### D-16 — Speed limit schedules

v0.8 placement (radar theme) — [speed-limit-schedules.md](radar/architecture/speed-limit-schedules.md)

### D-17 — PDF generation migration to Go

Go direct SVG charts (encoding/xml, no plotting helper) + Typst templates; embed Atkinson Hyperlegible font in report sources; dimensions in mm; Typst consumes SVG directly; Go chart package also serves SVG to web frontend; `grid-heatmap` migrated to Go subcommand; Python stack eliminated — [PDF reporting](platform/operations/pdf-reporting.md)

### D-18 — Speed percentile aggregation semantics

Reserve `p50/p85/p98` for grouped/report metrics only; for speed, keep `p98` as the high-end aggregate percentile and treat `p95` as historical-only legacy — [speed percentile plan](plans/speed-percentile-aggregation-alignment-plan.md), [TDL plan](plans/data-traffic-description-language-plan.md)

### D-19 — Track raw max vs future peak naming

Rename the current raw `peak_speed_mps` measure to `max_speed_mps` in unshipped contracts; reserve `peak` for a future outlier-filtered/context-aware top-speed metric — [speed percentile plan](plans/speed-percentile-aggregation-alignment-plan.md), [proto plan](plans/lidar-visualiser-proto-contract-and-debug-overlay-fixes-plan.md)

### D-20 — `v0.5.0` compat-shim removal policy

Ship one coordinated breaking-change sweep; keep no temporary dual-format shims after `v0.5.0` except DB upgrade detection and architecturally necessary aliases — [shim removal plan](plans/v050-backward-compatibility-shim-removal-plan.md), [simplification plan](plans/platform-simplification-and-deprecation-plan.md)

### D-21 — Visualiser debug overlay controls

`include_debug` gates debug payload emission; `SetOverlayModes(...)` remains client-side/advisory — [proto/debug overlay plan](plans/lidar-visualiser-proto-contract-and-debug-overlay-fixes-plan.md)

### D-22 — Image pipeline upgrade path

Reflash-only upgrades in the current plan; over-the-air updates are deferred to a later milestone — [simplification plan](plans/platform-simplification-and-deprecation-plan.md)

### D-23 — TicTacTail platform extraction

Generic cadenced aggregation + live surface + aligned history engine, extracted from VRLOG checker; in-repo `pkg/tictactail` — [platform plan](plans/tictactail-platform-plan.md)

### D-24 — Migration 030 offline percentile policy

Adopt Option A: remove persisted per-track speed percentiles from migration 030 and keep no DB-backed fallback path; revisit offline-only export computation later only if explicitly needed — [schema simplification plan](plans/schema-simplification-migration-030-plan.md)

### D-25 — Agent platform strategy: dual-native personas + shared skills

Use dual-native agent definitions ([.github/agents/](../.github/agents) for Copilot, [.claude/agents/](../.claude/agents) for Claude Code) with persona methodology bounded to ~40–80 lines per agent and drift-checked weekly. Shared project knowledge (Layers 0–2) stays single-source in [.github/knowledge/](../.github/knowledge). Reusable workflows live in [.claude/skills/](../.claude/skills) as slash commands, not in agent bodies. Copilot prompt files are optional thin wrappers only — not canonical workflow definitions — [ops doc](platform/operations/agent-preparedness.md), [plan](plans/agent-claude-preparedness-review-plan.md)

### D-26 — Static linux binaries: zig + musl in Docker

Ship the radar binary as a fully-static linux/{amd64,arm64} ELF via `make build-radar-static`. Toolchain is `zig cc -target <arch>-linux-musl` + a static `libpcap.a` built from the vendored upstream submodule pinned to `libpcap-1.10.6`. The build runs entirely inside a hermetic Docker image ([image/Dockerfile.static-build](../image/Dockerfile.static-build)) with pinned base image digest and SHA-256-verified toolchain tarballs. Contributors need only `docker` and `git`.

**Why zig over gcc:** stock `gcc-aarch64-linux-gnu` is glibc-based; glibc's NSS plugins force dynamic linking even for "static" binaries. Musl-targeting gcc exists (musl-cross-make, Alpine cross packages) but is either heavyweight to build or runs under qemu emulation. Zig is a single ~50 MB binary that cross-compiles to both arches at host speed and ships musl headers. The "another toolchain on contributor machines" objection is neutralised by the Docker hermeticism.

**Why image and release builds share the static path:** image, release, and ad-hoc Linux artifacts now use the same `scripts/build-radar-static.sh` route. Image staging goes through `scripts/stage-image-binary.sh`, and `scripts/verify-static-elf.sh` rejects dynamic ELF output before an artifact can be staged. The old dynamic-libpcap image Dockerfile was removed so a flashable image cannot accidentally pick up `libpcap.so` from the build host or base image. Static artifacts are verified by CI on every PR ([static-build-ci.yml](../.github/workflows/static-build-ci.yml)) including a `--self-check` smoke test in a clean Debian container.

**Other cgo binaries (cmd/sweep, cmd/tools/pcap-analyse, cmd/tools/settling-eval):** dev-host-only by policy. They are never shipped to a Pi and have no cross-compile target. If that changes, they get the same static treatment.

### D-27 — Capture identity and the capture index's rules

A capture is identified by the SHA-256 of its whole file, computed by the probe: the capture digest of the platform vocabulary plan. The content tag stays the scan's check that a file has not changed. A replay case's session and motion period are notes, never foreign keys, until the vocabulary plan drops them in v0.6.7. The probe keeps its first and last packet in file order and adds the earliest and latest packet and a count of backward clock steps. Migration 58 holds the capture queue's two job kinds until the vocabulary plan's one job queue names every job's subject. The identity and clock statements are the target contract: their implementations, and queue consolidation, follow the vocabulary programme's terminology-only Phase 1 in Phase 2 — [data model review](../data/structures/SCHEMA-REVIEW.md#decisions-on-the-remaining-findings), [vocabulary plan](plans/platform-vocabulary-and-data-model-plan.md)

### D-28 — Research briefing decisions, October 2026

Thirteen questions left open by the three research briefings, decided October 10, 2026. Following metrics: the encounter-weighted view (B10) is deferred until B1 measures how often both parties of an interaction are observed; the handheld instrument's measurement geometry stays open until NHTSA's DOT HS 809 811 is read; a class-dependent heavy-vehicle following band is deferred until a field run's L6 class recall meets the classifier plan's acceptance; the following report's and API's names are renamed through the vocabulary plan's P2-F with aliases first, retired under P2-G. Perception: L3 is scored against Baumann et al.'s released point-wise static and dynamic labels as an `S` item once the data is published, no production change; the convergence plan gains the centre-versus-extent error decomposition on kirk0's references and a grid-map extent estimator as a shadow arm, the rectangle fit waiting on the Applied Sciences paper; a range-adaptive DBSCAN eps joins the sweep as one arm scored by the W3 gate, defaults unchanged. Safety reference: the edition pins both pedestrian harm curves, Tefft 2013 and Monfort and Mueller 2025, each with its fleet years, and the report's paragraph uses the all-vehicle curves only; the before-and-after section prints the exponential model's relative change and interval as a model trend with a low-speed caveat citing Ambros and Kieć 2025, never a count and never suppressed; the 2026 pooled 30 km/h effect is a reference now and an intervention-effect entry only after the paper is read, off every page until the `published_model` kind is decided; the annual Traffic Safety Culture Index is the series source for the speeding items in the attitudes entries, one entry per edition year, with the one-off attitudes study as the explanatory source; Campolettano, Kusano and Victor's observed limit-relative shares are cited in the operator guide only; the percentile paragraph gains one clause, that p85 is a statistic of operating speeds and not a rule for setting limits, with the ITE and MUTCD citations in the guide. CAN-bus speed stays parked and the Monfort and Mueller citation stands, as decided in the weekly briefing — [October 9 briefing](lidar/operations/brief/fixed-sensor-research-briefing-2026-10-09.md#decisions), [weekly briefing](lidar/operations/brief/research-briefing-2026-10-10.md#open-questions), [window briefing](lidar/operations/brief/research-briefing-2025-01-to-2026-10.md#decisions), [behaviour plan](plans/lidar-behaviour-analytics-plan.md#13-open-research-questions), [crash-data plan](plans/platform-crash-data-integration-plan.md#open-questions), [attitudes plan](plans/platform-speeding-attitudes-and-aggressive-driving-plan.md#open-questions), [convergence plan](plans/lidar-tracker-geometry-convergence-plan.md#6-work-to-scope-and-schedule), [vocabulary plan](plans/platform-vocabulary-and-data-model-plan.md#phase-2-feature-and-data-model-improvements)

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
