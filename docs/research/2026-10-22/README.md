# October 22, 2026 — driving-behavior research assessments

- **Status:** research review and proposed experiments only. No shipped algorithms, calibrated thresholds, or safety effects claimed.

These notes triage six items from the October 22 briefing and related discussion. Primary-source details from the briefing must be checked against full texts before use in public claims. They are not replacements for the canonical [behaviour analytics plan](../../plans/lidar-behaviour-analytics-plan.md), [decisions register](../../DECISIONS.md), or [backlog](../../BACKLOG.md).

| Area                  | Assessment                                          | Proposed disposition                       |
| --------------------- | --------------------------------------------------- | ------------------------------------------ |
| DRAC / TTC / PET      | [conflict metrics](drac-and-conflict-metrics.md)    | Shadow computation and geometry validation |
| Roadside thresholds   | [Minnesota study](minnesota-roadside-thresholds.md) | Source-pinned reference profile only       |
| Korean driving events | [11-event taxonomy](korean-event-taxonomy.md)       | Feasibility mapping, no threshold adoption |
| Lane keeping          | [naturalistic study](lane-keeping-naturalistic.md)  | Separate departures, changes, variability  |
| AV interactions       | [human–AV interactions](human-av-interactions.md)   | Research design, not causal claims         |
| FutureRisk            | [composite risk](futurerisk.md)                     | Research watchlist, not public score       |

## Repository-specific evidence and blockers

The [October 20 following-opportunity corpus](../../lidar/operations/following-opportunity-corpus-2026-10.md) found 2,111 pair-seconds across 23 sites, but median detected pair duration was just **0.5 seconds**. Track fragments may turn one real episode into multiple apparent pairs. The current selector uses **track centers**, not physical bumper gap, and vehicle-size screening is not a validated class label. Consequently these counts **cannot** be interpreted as 2,264 independent, valid car-following episodes.

Priority reference locations in that report: Embarcadero at Broadway, Bryant and Bay; then Lombard and Van Ness. Held-out material must remain held out.

The existing [D-28 decision](../../DECISIONS.md#d-28--research-briefing-decisions-october-2026) defers publication of encounter-weighted following pending a review after B1; it does not authorize a new public metric. The [D-18 decision](../../DECISIONS.md#d-18--speed-percentile-aggregation-semantics) reserves p50/p85/p98 for grouped/report speed metrics. Do not add new metric implementations, alter production defaults, or change release scheduling through this documentation PR.

## Cross-cutting protocol

1. Separate direct observation, inferred geometry, predicted risk and crash-outcome association.
2. Preserve source jurisdiction, vehicle classes, thresholds, units, sampling frequency and inclusion rules.
3. Validate detection, track continuity, physical extent, temporal alignment and event selection before deriving safety conclusions.
4. Keep frame values and episode-level rows; never roll up percentiles of percentiles.
5. Treat thresholds as externally attributed study definitions, not universal safe/unsafe boundaries.
6. Require an explicit follow-on decision and backlog placement before implementation.
