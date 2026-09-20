# Surface consolidation

Active plan: [web-frontend-consolidation-plan.md](../plans/web-frontend-consolidation-plan.md)

Hub document for consolidating the LiDAR surfaces: retiring the Go-embedded
HTML dashboards, giving each concern one home, and removing the duplicate data
lineage behind the scene view.

Revised 2026-09-19 after a route-by-route survey. The previous revision's phase
estimates and several of its factual claims were wrong; the plan records what
changed and why.

## Problem

Three distinct web surfaces serve LiDAR functionality: the Svelte app on 8080,
the Go-embedded HTML dashboards, and the macOS Metal visualiser. The split is
not only a user-experience cost. The same operator task spans surfaces — running
HINT today needs the legacy sweep dashboard to start it, the Svelte tracks page
or the macOS app to label, and the legacy page again to continue — and the same
data is shaped twice for two clients.

## Surface ownership

[DESIGN.md §2.1](DESIGN.md) already names four canonical surfaces. The rule this
plan adds is which concern lands on which:

**Fidelity-bound interaction goes to macOS. Workflow and publishing go to the
Svelte app. Numbers go headless. Public pages stay static.**

| Concern                                                                       | Home                        |
| ----------------------------------------------------------------------------- | --------------------------- |
| Point-cloud interaction, per-point annotation, pose authoring, frame stepping | macOS                       |
| Runs, replay cases, captures, sites, sweep control, status, recording         | Svelte, 8080                |
| Overall scene view (recorded), scene catalogue, publishing                    | Svelte, 8080                |
| Public scene pages                                                            | `public_html/`, static      |
| Aggregate metrics, comparators, evaluation tables                             | Headless: JSON + server SVG |
| Legacy 8081 HTML                                                              | Deleted                     |

## Decision

Option B, one Svelte app with conditional LiDAR sections, weighted 95 against 32
for two apps and 67 for build-time exclusion. The matrix is in the plan.

## The two findings that reshaped the work

**Retiring port 8081 is nearly a no-op.** Every route is already registered on
the 8080 mux, so the listener adds only the status page and pprof. Deleting it
is about a day. What that does _not_ do is remove the legacy surface, because
the same handlers answer on 8080 — deleting the handlers is the separate, real
task.

**The web's data never passes through `FrameBundle`.** The pipeline writes
SQLite observations and adapts a frame bundle in two sibling statements, so the
scene chunk format is produced twice, in Go and in TypeScript, for the same
renderer. Serving the Go projection over HTTP removes the second lineage and is
the highest-value item in the plan. The web renders recorded and published
artefacts only: one decimation path, one artefact, two consumers.

## Revised phases

| Phase | Scope                                                        | Effort      |
| ----- | ------------------------------------------------------------ | ----------- |
| 0     | Capabilities API and conditional nav                         | Mostly done |
| 1     | Delete the consumer-free handlers; drop `go-echarts`         | 1 day       |
| 2     | Retire the 8081 listener and `--lidar-listen`                | 1 day       |
| 3     | PCAP replay and recording form in Svelte                     | 3–4 days    |
| 4     | Regions dashboard in Svelte; rename the `/debug/` JSON route | 2–3 days    |
| 5     | Sweep, auto-tune and HINT control in Svelte                  | 2–3 weeks   |
| 6     | Delete `status.html` and `sweep_dashboard.*`                 | 1 day       |
| 7     | One scene lineage                                            | 1 week      |

Phase 5 dominates, but not for the reason the previous revision gave. The eight
ECharts charts are the visible half; the form model behind them — 26 parameters
with types, bounds and step rules, and three divergent start payloads — is the
larger one.

## Regression gate

Scene publishing must stay byte-identical: 26 scenes are live, and the pipeline
touches neither 8081 nor ECharts today. `make scene-assets` for one site,
compared before and after, is the check that matters most.

## Risks

- Deleting a handler whose only consumer is the macOS app. It makes eleven HTTP
  calls to `/api/lidar/*` and uses one deprecated alias; pin the Swift client
  before any alias cleanup.
- Deleting `status.html` before Phase 3 lands, which would remove the only way
  to record a VRLOG by hand.
- Renaming `/debug/lidar/background/regions`, which returns JSON and is cited in
  operator docs, as though it were a debug page.
