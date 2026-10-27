# Executive Decisions Register

<!-- ignore-style -->

Closed design decisions across velocity.report. This register records the outcome of each decision and links to its source document. Milestone assignments come from [BACKLOG.md](BACKLOG.md), which is the single source of truth for scheduling.

This file should only be edited once or twice per sprint (2-week period) when there are blockers or open questions that require a recorded decision. It is not updated per-PR.

Each entry ends with a `Kept by:` line naming the check, test or reviewing skill that would fail if the decision were broken. Where the line says a check is planned, the decision is kept by review until the check exists. Each decision's state (kept, drifted, overtaken, unbuilt, pending, superseded) is recorded in the [decision audit](platform/operations/decision-audit.md), which release preparation refreshes, so that this register keeps its cadence.

---

## Decision Register

### D-01 — Fused transit schema

Defer until Phase B — [VISION §4.1](VISION.md), [TDL plan](plans/data-traffic-description-language-plan.md)

Kept by: `/review-pr` on schema changes (review only).

### D-02 — FFT radar feed ingestion

Defer to v2.0 — [VISION §3.1](VISION.md)

Kept by: `/review-pr` on radar ingest (review only).

### D-03 — Transit deduplication

Delete-before-insert with model version tracking — [transit-deduplication.md](radar/architecture/transit-deduplication.md)

Kept by: none until built; planned: a test that a duplicate transit insert is refused (v0.8.0).

### D-04 — Geometry-coherent tracking (P1 maths)

Geometry-coherent tracking (P1 maths) is the next L5 association candidate after the v0.5.3 corrected-body baseline. It is promoted only if held-out identity and geometry both improve, under the held-out geometry gate. Its placement is in BACKLOG.md (#391), not in this entry. Source: [proposal](../data/maths/proposals/20260222-geometry-coherent-tracking.md), [MATHS.md](../data/maths/MATHS.md).

Kept by: the held-out geometry gate G-GEO-1, scored by `-physical-reference`; the scorer exists, and the heading and containment rows are still to be pinned, per BACKLOG.md.

### D-05 — Maths proposal sequencing

P1 → P2 → P4 → P3 confirmed — [MATHS.md](../data/maths/MATHS.md)

Kept by: `/plan-review` checks plans against the P1, P2, P4, P3 order (review only).

### D-06 — OBB heading fixes D/E/F

Keep the OBB heading guards as maintenance only. Guards 2 and 3 and the velocity and displacement disambiguation stay as they are, and no new guard enters `l5tracks`. The shipped `obb_aspect_ratio_lock_threshold` stays at 0.25 until a replay record supports 0.15 or 0.10; the kirk0 run at 0.15 locked fewer clusters and worsened median course alignment, so it was not adopted. The published box is the box model's, on by default. Heading work continues in the geometry convergence plan (W1 and W2), which retires the guards under W1b, and not in P1 (D-04). The cluster outlines from Fixes E and F are a normal view layer in the macOS visualiser, on by default, because labellers use them. Restated October 20, 2026: the audit found Fixes E and F in use rather than debug-only, and the owner kept them on — [OBB heading review](../data/maths/proposals/20260222-obb-heading-stability-review.md)

Kept by: `/review-pr` on any change to `obb_aspect_ratio_lock_threshold` or to Guards 2 and 3 in `internal/lidar/l5tracks/tracking_update.go` (review only). No test pins the 0.25 default, and none is planned.

### D-07 — Track labelling UI (Phase 9)

The labelling UI is the Swift annotation window, which is delivered (#656). The outstanding work under this entry is the profile comparison UI in Phase 9 of the track-labelling plan (cross-run compare and scene evaluation). Its data layer is built and its UI is pending. It is scheduled in BACKLOG.md. Source: [track-labelling plan](plans/lidar-track-labelling-auto-aware-tuning-plan.md).

Kept by: review only (`/plan-review`) until the profile comparison UI ships; then a screen test on that UI.

### D-08 — Report footprint reduction

The report compiler footprint is removed from the Raspberry Pi image by using the embedded Typst engine in the Go binary. No separate report compiler tree ships in the image — [PDF reporting](platform/operations/pdf-reporting.md), [RPi imager](platform/operations/rpi-imager.md)

Kept by: the `VELOCITY_BUILD_TAGS` default in the Makefile and `make build-velocity`; no test asserts the tag.

### D-09 — Single binary architecture

Single binary with subcommands and embedded report compiler assets — [distribution packaging plan](plans/deploy-distribution-packaging-plan.md)

Kept by: the role closure test planned under D-30 (v0.5.9, not yet built). Until then, `make build-velocity` is the only product build target, and the D-26 exemption applies to dev-host cgo binaries.

### D-10 — RPi image tier strategy

One image, built by pi-gen from `image/` in one CI workflow and published as one os-list entry. The Typst source bundle ships in each report `.zip`. The migration to rpi-image-gen that the imager plan recommends is parked in BACKLOG.md. Restated October 20, 2026 from "pi-gen single tier", since "tier" collided with the plan's own tiers — [RPi imager plan](plans/deploy-rpi-imager-fork-plan.md)

Kept by: `internal/report/typst_generate_test.go` (exists), which fails when the sources ZIP drops a Typst source, the data or the README, and `.github/workflows/build-image.yml` (exists), which runs the one pi-gen build. The one-entry os-list has no check: `/review-pr` on `image/os-list-velocity.json` (review only).

### D-11 — ECharts → LayerChart migration

Report-view charts (time-series, histogram, comparison) are served as SVG by the Go chart package (`internal/report/chart`) and consumed directly by the Svelte frontend — not rewritten in LayerChart. Non-report charts (live dashboard, real-time stats, regions, sweep and HINT) move to Svelte LayerChart in v0.5.2, and ECharts leaves then: the `go-echarts` package and its debug-chart handlers first, then the :8081 dashboards' ECharts JavaScript once their Svelte replacements exist. Restated October 20, 2026 by the owner, from v0.7 — [DESIGN §4](ui/DESIGN.md), [frontend consolidation](plans/web-frontend-consolidation-plan.md), [PDF migration plan](plans/pdf-go-chart-migration-plan.md)

Done in v0.5.2 on branch `claude/echarts-svelte-migration-83fbd5` (#836): `go-echarts` and its debug pages are deleted, `echarts.min.js` left with the rest of `l9endpoints/l10clients/`, and LayerChart 2.8.1 is back in `web/` with shared components in `web/src/lib/components/charts/`. The `/api/lidar/chart/*` JSON routes were kept rather than deleted, because the Svelte LiDAR Debug page reads them. The regions map is drawn on a canvas, not LayerChart, since it has some 72,000 cells.

Kept by: `go.mod`, which has no `go-echarts`, and the tree, which has no ECharts asset; no check fails if either returns, so `/review-pr` on `go.mod` and `web/package.json` keeps it (review only).

### D-12 — Web palette (percentile colours)

Svelte compliant now. ECharts is not fixed: it leaves in v0.5.2 (D-11), and its palette mismatch with it. Restated October 20, 2026 — [DESIGN §3.3](ui/DESIGN.md), [design review](ui/design-review-and-improvement.md)

ECharts left in v0.5.2 (branch `claude/echarts-svelte-migration-83fbd5`, #836), so its palette mismatch is gone; the LiDAR operator charts are now Svelte and LayerChart.

Kept by: `web/src/lib/palette.test.ts` (exists), which fails when the six values in `palette.ts` change. Nothing ties `inlineSvgChart.ts` or `internal/report/chart/palette.go` to them, so `/review-pr` reviews chart colour changes (review only) until the design-tokens row is built.

### D-13 — Widescreen content containment

Defer to v0.7 frontend consolidation — [DESIGN §5.7](ui/DESIGN.md), [design review](ui/design-review-and-improvement.md)

Kept by: `/review-pr` on width rules in `web/src/routes/app.css` (review only). No test asserts containment; the planned check is a 3000 px viewport test under the v0.9.0 end-to-end row.

### D-14 — Simplification & deprecation scope

Plan confirmed; Phase 1 complete. The remaining work is scheduled in BACKLOG.md, not here: the Make target trim to a budget of 150 targets (Project F of the plan), and the CLI flag removals in its section 3. Flags that depend on the frontend consolidation (#252) move with it. Removal is the decision; the milestone is not. Source: [simplification plan](plans/platform-simplification-and-deprecation-plan.md).

Kept by: the Make budget check, planned for v0.5.9 (counts public targets and refuses an undocumented one; not yet built). Until it exists, the count is taken by hand.

### D-15 — Time-partitioned data tables

Implement in v0.9.0 — [time-partitioned tables plan](radar/architecture/time-partitioned-data-tables.md)

Kept by: none until built. Planned with the v0.9.0 row: closure tests that keep a month writable in main while any touching session or transit work remains open and refuse sealed-month writes; a path table test for VR-SEC-002; attach tests for read-only archives and cleanup on success, cancellation, and failure; a single-file oracle for zero, ten, eleven, and twenty-one archived months, with main counted once; crash-injection and concurrent-storage tests; a test that the backup route never returns main alone with partitioning on; and `/security-review` on the implementing pull request. The owner's storage choices remain Proposed: monthly archives, per-query attachments, `flock` plus a mutex, no compression, local disks, a writable hardened USB mount, API authentication before storage actions, and no automatic deletion. Closure waits for finished sessions and transit processing. Crash-safe publication, reader admission, cross-month links, and sealed-month corrections remain explicit [implementation blockers](radar/architecture/time-partitioned-data-tables.md#open-implementation-problems).

### D-16 — Speed limit schedules

Speed limit schedules are time-of-day windows on posted speed limits. They depend on the posted-limit record, so they follow it in BACKLOG.md, and the radar theme no longer decides their placement. Source: [speed-limit-schedules.md](radar/architecture/speed-limit-schedules.md).

Kept by: review only (`/plan-review`), which checks any schedule plan against the posted-limit item it depends on.

### D-17 — PDF generation migration to Go

Go direct SVG charts (encoding/xml, no plotting helper) and Typst templates; the Atkinson Hyperlegible font is embedded in report sources; dimensions are in mm; Typst consumes the SVG directly; the Go chart package also serves SVG to the web frontend. The Python stack is eliminated. The `grid-heatmap` tool retires once a Go-served SVG grid view replaces it on the web, which the owner asked for first on October 20, 2026; no Go subcommand was built. In v0.5.2 (branch `claude/echarts-svelte-migration-83fbd5`, #836) `GET /api/charts/grid` serves that view, the LiDAR Debug page shows it, and `tools/grid-heatmap/` is deleted with its tests, its coverage path and the `stats-live` and `stats-pcap` targets. Source: [PDF reporting](platform/operations/pdf-reporting.md).

Kept by: the release audit's search of the Makefile and `tools/` for `tools/grid-heatmap` and the `stats-live` and `stats-pcap` targets. The `api-grid-heatmap` target is a different thing (a curl wrapper for the `/api/lidar/grid_heatmap` JSON route) and stays.

### D-18 — Speed percentile aggregation semantics

Reserve `p50/p85/p98` for grouped/report metrics only; for speed, keep `p98` as the high-end aggregate percentile and treat `p95` as historical-only legacy. Every published aggregate speed percentile uses the nearest-rank rule, the value at 1-based rank ceil(p·n), through the shared helper `internal/stats.NearestRank`, and is computed from the underlying population, never from bucket percentiles. Amended October 22, 2026 by the owner in the [maths decision review](platform/operations/decision-review-maths-2026-10.md#e-03-q-03) (E-03); diagnostic quantiles of other quantities are outside this entry — [speed percentile plan](plans/speed-percentile-aggregation-alignment-plan.md), [TDL plan](plans/data-traffic-description-language-plan.md)

Kept by: `internal/stats/nearest_rank_test.go` (exists), which fails when the rule changes, and the radar rollup and scene timeline tests that pin their p50, p85 and p98; `/review-pr` on `l6objects` features until the per-track percentiles are renamed or removed; then a search test.

### D-19 — Track raw max vs future peak naming

Rename the current raw `peak_speed_mps` measure to `max_speed_mps` in unshipped contracts; reserve `peak` for a future outlier-filtered/context-aware top-speed metric — [speed percentile plan](plans/speed-percentile-aggregation-alignment-plan.md), [proto plan](plans/lidar-visualiser-proto-contract-and-debug-overlay-fixes-plan.md)

Kept by: `/review-pr` on the speed contract (review only).

### D-20 — `v0.5.0` compat-shim removal policy

Ship one coordinated breaking-change sweep; keep no temporary dual-format shims after `v0.5.0` except DB upgrade detection and architecturally necessary aliases. An old name, field or format is architecturally necessary only while something the release does not replace depends on it: the macOS app, a file on a device that the upgrade does not rewrite, data at rest, or a published URL. Callers inside the release unit are renamed in the same change. A person typing a command gets a tombstone, which prints the new name and does no work; a tombstone is not a dual format. Each surviving name is a row in the [compatibility register](platform/operations/compatibility-register.md), with its removal release. Source: [shim removal plan](plans/v050-backward-compatibility-shim-removal-plan.md), [simplification plan](plans/platform-simplification-and-deprecation-plan.md), [architecture review, D-20 in practice](plans/platform-architecture-review-plan.md#d-20-in-practice-who-can-still-depend-on-an-old-name).

Kept by: the compatibility register test, planned for v0.5.3 as the clip rename's first PR and not yet built, which fails when a surviving name reaches its removal release. Until then, `/review-pr` reads the register.

### D-21 — Visualiser debug overlay controls

`include_debug` gates debug payload emission; `SetOverlayModes(...)` remains client-side/advisory — [proto/debug overlay plan](plans/lidar-visualiser-proto-contract-and-debug-overlay-fixes-plan.md)

Kept by: a test that frames requested without `include_debug` carry no debug payload; the emitter must be traced first.

### D-22 — Image pipeline upgrade path

Upgrades are run by the operator. `velocity device upgrade` reads a release manifest (`release.json`) and installs a versioned binary in place. The path landed in the 0.5.1 line and is covered by the upgrade flow tests in `internal/ctl`. No unattended update is decided here; an unattended path needs a new entry. Source: [simplification plan](plans/platform-simplification-and-deprecation-plan.md), [architecture review, P5](plans/platform-architecture-review-plan.md#p5-decisions-that-the-code-does-not-keep).

Kept by: the upgrade flow tests in `internal/ctl` (`upgrade_flow_test.go`).

### D-23 — TicTacTail platform extraction (superseded)

Superseded by D-35. The original decision stays on the record: generic cadenced aggregation with a live surface and aligned history engine, extracted from the VRLOG checker, in-repo `pkg/tictactail` — [platform plan](plans/tictactail-platform-plan.md)

### D-24 — Migration 030 offline percentile policy

Adopt Option A: remove persisted per-track speed percentiles from migration 030 and keep no DB-backed fallback path; revisit offline-only export computation later only if explicitly needed — [schema simplification plan](plans/schema-simplification-migration-030-plan.md)

Kept by: `/review-pr` on each migration (review only).

### D-25 — Agent platform strategy: dual-native personas + shared skills

Use dual-native agent definitions ([.github/agents/](../.github/agents) for Copilot, [.claude/agents/](../.claude/agents) for Claude Code) with persona methodology kept to about 400 lines per agent and drift-checked in CI (restated October 20, 2026 by the owner, from ~40–80 lines, to what the personas hold). Shared project knowledge (Layers 0–2) stays single-source in [.github/knowledge/](../.github/knowledge). Reusable workflows live in [.claude/skills/](../.claude/skills) as slash commands, not in agent bodies. Copilot prompt files are optional thin wrappers only — not canonical workflow definitions — [ops doc](platform/operations/agent-preparedness.md), [plan](plans/agent-claude-preparedness-review-plan.md)

Kept by: `make check-agent-drift` as a failing CI step. The target exists but only reports, and all seven pairs drift today; the backlog's v0.5.11 row syncs them, adds a failing mode and the CI step.

### D-26 — Static linux binaries: zig + musl in Docker

Ship the radar binary as a fully-static linux/{amd64,arm64} ELF via `make build-radar-static`. Toolchain is `zig cc -target <arch>-linux-musl` + a static `libpcap.a` built from the vendored upstream submodule pinned to `libpcap-1.10.6`. The build runs entirely inside a hermetic Docker image ([image/Dockerfile.static-build](../image/Dockerfile.static-build)) with pinned base image digest and SHA-256-verified toolchain tarballs. Contributors need only `docker` and `git`.

**Why zig over gcc:** stock `gcc-aarch64-linux-gnu` is glibc-based; glibc's NSS plugins force dynamic linking even for "static" binaries. Musl-targeting gcc exists (musl-cross-make, Alpine cross packages) but is either heavyweight to build or runs under qemu emulation. Zig is a single ~50 MB binary that cross-compiles to both arches at host speed and ships musl headers. The "another toolchain on contributor machines" objection is neutralised by the Docker hermeticism.

**Why image and release builds share the static path:** image, release, and ad-hoc Linux artifacts now use the same `scripts/build-radar-static.sh` route. Image staging goes through `scripts/stage-image-binary.sh`, and `scripts/verify-static-elf.sh` rejects dynamic ELF output before an artifact can be staged. The old dynamic-libpcap image Dockerfile was removed so a flashable image cannot accidentally pick up `libpcap.so` from the build host or base image. Static artifacts are verified by CI on every PR ([static-build-ci.yml](../.github/workflows/static-build-ci.yml)) including a `--self-check` smoke test in a clean Debian container.

**Other cgo binaries (cmd/sweep, cmd/tools/pcap-analyse, cmd/tools/settling-eval):** dev-host-only by policy. They are never shipped to a Pi and have no cross-compile target. If that changes, they get the same static treatment.

Kept by: `scripts/verify-static-elf.sh` and the static build CI workflow.

### D-27 — Capture identity and the capture index's rules

A capture is identified by the SHA-256 of its whole file, computed by the probe: the capture digest of the platform vocabulary plan. The content tag stays the scan's check that a file has not changed. A replay case's session and motion period are notes, never foreign keys, until the vocabulary plan drops them in v0.7.5. The probe keeps its first and last packet in file order and adds the earliest and latest packet and a count of backward clock steps. Migration 58 holds the capture queue's two job kinds until the vocabulary plan's one job queue names every job's subject. The identity and clock statements are the target contract: their implementations, and queue consolidation, follow the vocabulary programme's terminology-only Phase 1 in Phase 2 — [data model review](../data/structures/SCHEMA-REVIEW.md#decisions-on-the-remaining-findings), [vocabulary plan](plans/platform-vocabulary-and-data-model-plan.md)

Kept by: migration `000058`'s kind triggers, and the capture identity tests planned with vocabulary P2-A.

### D-28 — Research briefing decisions, October 2026

Thirteen questions left open by the three research briefings, decided October 10, 2026. Following metrics: the encounter-weighted view (B10) was deferred until B1 measured how often both parties of an interaction are observed; B1 was measured on October 20, 2026 ([report](lidar/operations/following-opportunity-corpus-2026-10.md)), so that deferral lapsed, and on October 22, 2026 the owner deferred it again in the [maths decision review](platform/operations/decision-review-maths-2026-10.md#e-02-q-02): it reopens when the valid-following record is pinned and l8fol0004 reports, and the reopened call takes Euler's statistics review; the handheld instrument's measurement geometry stays open until NHTSA's DOT HS 809 811 is read; a class-dependent heavy-vehicle following band is deferred until a field run's L6 class recall meets the classifier plan's acceptance; the following report's and API's names are renamed as cutovers through the vocabulary plan's P2-F, with no alias, as D-31 requires. Perception: L3 is scored against Baumann et al.'s released point-wise static and dynamic labels as an `S` item once the data is published, no production change; the convergence plan gains the centre-versus-extent error decomposition on kirk0's references and a grid-map extent estimator as a shadow arm, the rectangle fit waiting on reading the Applied Sciences paper, which the owner kept on October 20, 2026 after W1a built the fit; a range-adaptive DBSCAN eps joins the sweep as one arm scored by the W3 gate, defaults unchanged. Safety reference: the edition pins both pedestrian harm curves, Tefft 2013 and Monfort and Mueller 2025, each with its fleet years, and the report's paragraph uses the all-vehicle curves only; the before-and-after section prints the exponential model's relative change and interval as a model trend with a low-speed caveat citing Ambros and Kieć 2025, never a count and never suppressed; the 2026 pooled 30 km/h effect is a reference now and an intervention-effect entry only after the paper is read, off every page until the `published_model` kind is decided; the annual Traffic Safety Culture Index is the series source for the speeding items in the attitudes entries, one entry per edition year, with the one-off attitudes study as the explanatory source; Campolettano, Kusano and Victor's observed limit-relative shares are cited in the operator guide only; the percentile paragraph gains one clause, that p85 is a statistic of operating speeds and not a rule for setting limits, with the ITE and MUTCD citations in the guide. CAN-bus speed stays parked and the Monfort and Mueller citation stands, as decided in the weekly briefing — [October 9 briefing](lidar/operations/brief/fixed-sensor-research-briefing-2026-10-09.md#decisions), [weekly briefing](lidar/operations/brief/research-briefing-2026-10-10.md#open-questions), [window briefing](lidar/operations/brief/research-briefing-2025-01-to-2026-10.md#decisions), [behaviour plan](plans/lidar-behaviour-analytics-plan.md#13-open-research-questions), [crash-data plan](plans/platform-crash-data-integration-plan.md#open-questions), [attitudes plan](plans/platform-speeding-attitudes-and-aggressive-driving-plan.md#open-questions), [convergence plan](plans/lidar-tracker-geometry-convergence-plan.md#6-work-to-scope-and-schedule), [vocabulary plan](plans/platform-vocabulary-and-data-model-plan.md#phase-2-feature-and-data-model-improvements). Restated October 20, 2026 after the audit.

Kept by: `/plan-review` on the cited plans and briefings, and, for B10, the l8fol0004 report that is its reopening trigger (review only). The rename clause is kept by the compatibility register test (planned for v0.5.3, not built); the safety-edition clauses by the edition validator (planned, not built); the range-adaptive arm by its report and the W3 report, both of which exist.

### D-29 — Research briefing decisions, October 12, 2026

Two questions left open by the October 12 briefing, decided October 12, 2026. An RTK-equipped probe vehicle, driven past a surveyed static sensor at scripted speeds and gaps, is adopted as a proposed external reference for L5 speed and the following gap: the physical-reference plan names it as a reference source independent of the sensor's returns, and the backlog carries it as an `M` item; nothing in code, the registry or the definitions changes, and fix availability on a residential street is the first thing it must measure. Object point count as a covariate for comparisons across weather is deferred: it reopens when the test corpus first gains a wet or snowy capture, or when a comparison of L3 or L4 across conditions is proposed, and then becomes a method rule, point count per passage reported beside any recall comparison — [October 12 briefing](lidar/operations/brief/research-briefing-2026-10-12.md#decisions), [physical-reference plan](plans/lidar-physical-reference-review-plan.md#proposed-external-reference-rtk-probe-vehicle), [test corpus plan](plans/lidar-test-corpus-plan.md#non-goals)

Kept by: `/plan-review` on the RTK proposal and on the test corpus plan's non-goal (review only). The rule that nothing in code, the registry or the definitions changes has no test.

### D-30 — One binary, closed roles, kinds compiled in

One binary carries every role. A role is a composition root under `internal/cmd/` with a dependency closure that CI asserts, and each job kind is compiled into the binary that runs it. The job queue has one implementation and one claim rule, but each data-owning host runs one authority for its own jobs: the hub authorises analysis jobs and accepts their results, and an edge host authorises its own capture jobs. The two share code and never share a database. Dev-host cgo binaries stay outside this entry under D-26. This amends D-09 with the role model. Source: [architecture review, ADR-1](plans/platform-architecture-review-plan.md#architecture-decision-records-proposed), [worker pool plan §1](plans/lidar-worker-pool-and-results-hub-plan.md#1-decision).

Kept by: the `go list` closure test in `make lint-go`, planned for v0.5.9 and not yet built. Until it exists, `/review-pr` checks each new `cmd/` package against this entry.

### D-31 — Renames are cutovers; tombstones for people, aliases for the app

A rename is one cutover per concept, delivered as reviewable pull requests that together change every consumer. Every in-repo caller changes in the same change, with no alias. An old name survives only while something the release does not replace depends on it: a registered alias while the macOS app calls it, an alias or a rewrite for a device file the upgrade leaves alone, a migration or converter for data at rest, or a permanent redirect for a published URL. A person who types an old command, flag or Make target gets a tombstone that prints the new name and does no work. The vocabulary programme is cut to three Phase 1 renames (clip, sites and labels, terminology only), and each Phase 2 merge waits for the Phase 1 exit. This supersedes the alias method in the vocabulary plan's Phase 1 and in D-28 for the following report's and API's names. Applies D-20. Source: [architecture review, ADR-2](plans/platform-architecture-review-plan.md#architecture-decision-records-proposed), [vocabulary plan, Scope](plans/platform-vocabulary-and-data-model-plan.md#scope).

Kept by: the compatibility register test, planned for v0.5.3 as the clip rename's first PR and not yet built, which fails when a surviving name reaches its removal release. Until then, `/review-pr` reads the register.

### D-32 — Planning limits and backlog governance

A body of work has one canonical hub doc and at most one active plan. At most twelve bodies of work are active at once, counting `Active` and `In progress` alike. A plan that waits carries the status Parked, with a one-line reason and a revisit trigger, and is not deleted. A release slot holds at most twelve outstanding items. Items beyond that move to the next slot in BACKLOG.md, never out of it. An item is one outcome in at most two sentences. Programme phases may be recorded as sequence in their plan, but outstanding items stay in BACKLOG.md until delivered. A version is tagged when the owner cuts it from `main`; the changelog heading names what shipped, and a slot's theme names what is planned. Below 1.0 a patch release may carry a breaking change, as SemVer allows: the changelog lists it first under BREAKING with its migration steps, and the minor number keeps naming the themed series (amended October 20, 2026 by the owner). Milestone placement stays in BACKLOG.md, as this register's own rule says. This refines principle 6 of Milestone Placement. Source: [architecture review, ADR-3](plans/platform-architecture-review-plan.md#architecture-decision-records-proposed), [canonical plan graduation](platform/architecture/canonical-plan-graduation.md), [backlog governance](BACKLOG.md).

Kept by: `make report-plan-hygiene`, which counts the active plans (exists; advisory). The slot and item-length lint is planned as a backlog check and is not yet built.

### D-33 — Hub docs state what ships; plans state intent

A hub doc states shipped behaviour. Where it also carries a design, it says so and links the plan that owns the intent. A hub stub for an unbuilt feature may stay when its header says it is a design and is not built, and links the plan that owns the intent; a stub that presents an unbuilt feature as present is corrected that way or removed. Amended October 20, 2026 by the owner, who kept the visualiser design stubs. The graduation rules in the canonical plan graduation doc apply without change. Source: [architecture review, ADR-4](plans/platform-architecture-review-plan.md#architecture-decision-records-proposed), [canonical plan graduation](platform/architecture/canonical-plan-graduation.md).

Kept by: `/plan-graduation`, reviewing each hub doc against this entry (review only). `make check-plan-hygiene` checks canonical headers, not stubs; no stub lint exists yet.

### D-34 — The public survey site leaves the product repository in two steps

First, the scene assets leave git and the Pi image for an external store, with a pointer left in the repository. The device must run without them, and any fetch is an operator action, under tenets 1 and 4. Second, the public site, `tools/s2-archive` and `tools/s2-hilbert` move to their own repository. The scene player becomes a versioned package only when that repository needs it, and it is pinned to an immutable revision, never copied. `velocity scene export` and its NDJSON format stay here, together with the export test that refuses personal fields and trajectories, because the data and the privacy rule are held here (tenets 1 and 7). Source: [architecture review, ADR-5](plans/platform-architecture-review-plan.md#architecture-decision-records-proposed), [project boundaries](plans/platform-architecture-review-plan.md#project-boundaries-what-to-split-out-and-what-to-keep).

Kept by: a planned image staging check that fails when scene assets reach `image/stage-velocity` or the built public tree (not yet built), and the export privacy test planned with the surveys work.

### D-35 — TicTacTail withdrawn (supersedes D-23)

TicTacTail has no code, no `pkg/` directory and no second consumer. The in-repo `pkg/tictactail` extraction is withdrawn, and the v0.8.0 incubation item moves to the deferred list. A cadenced aggregation engine returns as a plan only when a second consumer exists, under tenet 5. Source: [architecture review, project boundaries](plans/platform-architecture-review-plan.md#project-boundaries-what-to-split-out-and-what-to-keep).

Kept by: review only. `/plan-review` refuses a plan that proposes `pkg/tictactail` without citing this entry.

### D-36 — Physical-measurement qualification and Gerardus/L7 boundary

Keep frozen L1–L10 layer identities. Gerardus owns the static street model and accepted registration; velocity.report L5 owns physical body estimates, L7 composes registered dynamic scene and spatial relations, and L8 alone owns physical interaction equations, uncertainty propagation, suppression, scoring and aggregation. L7 is neither a second body estimator nor a cross-visit reidentification service. Cross-layer S-REF, S-OBS, S-GEO, S-UNC, S-MET, S-SCENE and S-EVID workstreams organise 05x qualification. v0.5.3–v0.5.9 targets static qualification; v0.6.0–v0.6.4 is reserved for SLAM/mobile transfer; other features move to 07x or Parked. Passing a release milestone does not waive predeclared physical, coverage and calibration gates. Source: [BACKLOG](BACKLOG.md), [L7 plan](plans/lidar-l7-scene-plan.md), [state-estimation plan](plans/lidar-state-estimation-plan.md).

Kept by: layer-boundary reviews, preserved migration inventory and scientific gate ledgers; CI enforcement is planned.

### D-37 — Priority-first, workstream-last backlog metadata

Outstanding BACKLOG items use the one-line convention `- [#PR] **P0|P1|P2** (#issue) deliverable: [design](plans/lidar-state-estimation-plan.md) SIZE WORKSTREAM`, where SIZE is the original backtick-wrapped S/M/L/XL and WORKSTREAM is the final backtick-wrapped S-* tag, after optional maths/frontend annotations. Optional PR precedes priority; issue remains after it. P0 blocks the first qualified static measurement, P1 is next product or operational work, and P2 is research/polish/parked. Do not drop agreed work or alter the historical Complete section. Each slot stays within D-32's twelve-item cap. Source: [BACKLOG](BACKLOG.md).

Kept by: `make check-backlog-format` in `lint-docs` and the preserved-item migration audit.

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

6. **Scope milestones for focus.** Each milestone should have a clear theme and a manageable number of items (~10–12 max). When a milestone grows beyond that, split by theme or sequencing into the next milestone slot. Thematic coherence reduces context-switching and improves delivery predictability. D-32 refines this rule: overflow moves to the next slot and never out of BACKLOG.md.
