# Survey lane bounds: a first look at 60, 70, 80 and 100 metres

This historical note compares candidate radius caps using existing replay coverage summaries.
The owner subsequently chose to retain every included trail without a radius cap; the
[full scene trail inventory](survey-trail-inventory-2026-11.md) replaces this comparison for the
[survey lane export plan](../../plans/lidar-survey-lane-exports-plan.md).

- **Status:** Historical exploratory summary; radius caps withdrawn, no new replay or physical validation
- **Scope:** 24 replay cases, one summary per case
- **Related:** [lane field at Columbus and Broadway](lane-field-columbus-broadway-2026-11.md), [lane proposals against drawn lanes](lane-proposals-against-drawn-lanes-2026-11.md)

## Question and method

The survey bound is the extent reached by object trails observed within a survey window. If a
candidate maximum radius of 60, 70, 80 or 100 m is applied to that bound, how often do existing
estimates reach each radius, and is the tail common enough to guide the next evaluation?

Read the existing `tools/s2-archive/continuity-coverage.survey.json`: each case reports the
horizontal range of its replay's confirmed, associated online estimates, relative to sensor origin,
with nearest-rank percentiles and the maximum rounded to 0.01 m. For each candidate, count cases
whose stored p95, p99 or maximum is greater than or equal to the radius. Each case contributes one
vote, regardless of its number of estimates. The p95 and p99 use estimate samples as their
population: longer-lived tracks contribute more samples. A maximum needs only one estimate.

This is a screen of existing summary statistics, not four replay arms. There was no change to
tuning or data, no new parameter set, no lane matching and no new export. The original observations
and the actual lane fields are not available in this workspace. The summaries cannot tell whether
the estimates follow the same road or direction, whether distinct objects repeatedly reached the
radius, or how much of any lane is covered. They cannot measure the incremental lane length gained
by a larger cap. The historical capture and replay windows also need to be matched to each target
survey before their statistics can support an export.

## Findings

| Candidate cap | Cases with maximum at or beyond cap | Cases with p99 at or beyond cap | Cases with p95 at or beyond cap |
| ------------: | ----------------------------------: | ------------------------------: | ------------------------------: |
|         100 m |                               13/24 |                            4/24 |                            0/24 |
|          80 m |                               17/24 |                            8/24 |                            0/24 |
|          70 m |                               19/24 |                            9/24 |                            3/24 |
|          60 m |                               21/24 |                           13/24 |                            4/24 |

The median of the 24 case maxima is 120.84 m, while the median case p95 is 36.64 m and median
case p99 is 61.40 m. The large difference shows why a single furthest estimate is a weak coverage
boundary. It does not establish that 60 m is a suitable bound: the area and lane directions near
that radius may still have little repeated support. Likewise the absence of a p95 beyond 80 m does
not mean a lane at 80 m is unobserved; the distribution includes observations at all ranges.

For scale, if a straight lane passed exactly through the sensor, radius caps of 60, 70, 80 and
100 m could contain at most 120, 140, 160 and 200 m of that line respectively. This is a geometric
ceiling, not observed length: an offset between sensor and lane shortens the chord, and actual
trail support can cover less or be broken by gaps. The formula for an infinite straight lane at
perpendicular offset `d` is `2 × sqrt(R² − d²)` when `d < R`; real segment ends can shorten it
further.

The existing lane-field check at Columbus and Broadway found direction-consistent traffic ridges
within 40 m over one ten-minute window. That is useful evidence that the field can describe a road,
but it does not compare any of the four outer radii. The independent comparison of proposals with
hand-drawn lanes is [not yet run](lane-proposals-against-drawn-lanes-2026-11.md).

### Per-case stored statistics

These are distances from the sensor, in metres; they are not the length of a travelled path or the
length of a lane covered. Percentiles are over the case's estimate samples, not over lanes or
independent objects.

| Case                  |   p95 |    p99 | Maximum |
| --------------------- | ----: | -----: | ------: |
| 1st-mission           | 33.03 |  47.83 |   81.96 |
| 3rd-folsom            | 39.17 |  60.45 |   89.71 |
| ashbury-downey        | 28.66 |  34.49 |   54.99 |
| broadway-franklin     | 77.13 | 114.59 |  188.98 |
| bush-powell           | 25.17 |  41.16 |   69.37 |
| california-leon-baker | 72.70 |  85.13 |  191.75 |
| columbus-broadway     | 37.61 |  75.07 |  123.38 |
| columbus-north-point  | 37.30 |  83.91 |  127.46 |
| embarcadero-bay       | 35.98 |  55.05 |   74.88 |
| embarcadero-broadway  | 39.67 |  63.91 |  125.89 |
| embarcadero-bryant    | 64.13 |  81.39 |  208.13 |
| franklin-mcallister   | 54.87 | 109.57 |  192.94 |
| fulton-divisadero     | 70.08 | 107.77 |  171.69 |
| haight-divisadero     | 20.64 |  30.02 |   44.37 |
| howard-6th            | 30.66 |  46.66 |   69.63 |
| hyde-ofarrell         | 31.26 |  53.62 |   58.49 |
| kirk0                 | 50.46 |  91.67 |   91.72 |
| laguna-eddy           | 29.12 |  57.60 |  156.59 |
| lombard-broderick     | 50.64 |  69.09 |  131.52 |
| lombard-laguna        | 35.50 |  57.42 |  145.94 |
| marina-broderick      | 35.63 |  45.08 |   84.68 |
| marina-webster-beach  | 29.42 |  43.70 |   79.41 |
| pierce-haight         | 52.46 | 137.28 |  163.57 |
| van-ness-sacramento   | 34.39 |  62.35 |  118.29 |

## Subsequent direction

The owner chose to include all trails in the selected window, with no radius or percentile cap.
The [scene inventory](survey-trail-inventory-2026-11.md) counts every indexed sample in the
available exports. A later check must use the full original survey input where available and
reconcile every included trail with the bound and lane product.

## Provenance and limits

| Item            | Source                                                                                                                                                                                                                                                                       |
| --------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Summary input   | `tools/s2-archive/continuity-coverage.survey.json`, SHA-256 `ca387488e47ea00a1e3085528b2c64e0e7a5b4e5a773a7586b5a3983634c5f7c`                                                                                                                                               |
| Original method | `lidar-state-estimation-baseline -survey-coverage`; `internal/lidar/replayeval/coverage_survey.go` documents the confirmed, associated online-estimate population and nearest-rank method                                                                                    |
| Source replay   | Per-case capture-set hashes, observation-source IDs and replay parameter hashes are recorded in the summary input. There are 307,525 recorded frames, 972,248 estimates and 31,771 case-local track identities across the 24 cases; these are not independent object counts. |
| Build           | 23 cases name git `3ee42366c79233f5197a059bc226aa6026f9f666`, build version `dev`; `kirk0` has an unknown git SHA and an unstamped build. Its parameter hash also differs from the other 23 cases.                                                                           |
| Follow-up       | The original local evidence databases and recordings were not available here. No raw results were added to this report.                                                                                                                                                      |

No row establishes a detection probability, sensor range, lane geometry accuracy or physical survey
coverage. Associations, duplicated tracks, sample weighting, outliers, occlusion, changing street
layouts and the unstamped case can all affect the comparison. The table is a guide to where to
look, not an acceptance result.
