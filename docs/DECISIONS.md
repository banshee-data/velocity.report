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

**Why a separate static path from `Dockerfile.build`:** the legacy image-build path produces glibc + dynamic-libpcap binaries embedded in the RPi image, where the OS provides the runtime. The static path is for distributing standalone binaries (releases, ad-hoc deploys) where no runtime guarantee exists. Both paths coexist for now; the static path is verified by CI on every PR ([static-build-ci.yml](../.github/workflows/static-build-ci.yml)) including a `--self-check` smoke test in a clean Debian container.

**Other cgo binaries (cmd/sweep, cmd/tools/pcap-analyse, cmd/tools/settling-eval):** dev-host-only by policy. They are never shipped to a Pi and have no cross-compile target. If that changes, they get the same static treatment.

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
