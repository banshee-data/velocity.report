# Speed reporting by direction and lane plan

How every reported speed, radar or LiDAR, is attributed to a direction of travel and, where lanes
exist, to a lane with its own posted limit, so that a report at an intersection signed 25 mph one way
and 30 mph the other prints two histograms, two sets of percentiles and two shares above the limit,
each against the right sign. The same per-direction summary feeds the PDF report, the web report
view, the JSON API and the published scene export the public survey pages read.

- **Status:** Proposed; waits on the lane snapshot (companion plan) for LiDAR lanes, and on nothing
  for the radar's two directions. Not Active (D-32)
- **Decisions:** relies on D-16 (schedules follow the posted-limit record), D-18 (nearest-rank
  percentiles, aggregate-only), D-32, D-33, D-34 (the export privacy test), D-37; proposes no new
  register entry of its own. Takes the posted-limits plan's model as settled: a limit is signed in a
  unit, belongs to one direction, and is reported in the sign's unit
- **Layers:** radar transit worker and storage, report pipeline (Go SVG charts, Typst data), HTTP
  API, web report view, scene export
- **Target:** unscheduled; proposed for the v0.7.1 slot (reference speeds); the radar direction
  column can land first
- **Companion plans:** [lane field and proposals](lidar-lane-field-and-lane-proposals-plan.md)
  (the snapshot and attribution), [lane painter](lidar-visualiser-lane-painter-plan.md),
  [public survey direction histograms](public-survey-direction-histograms-plan.md),
  [posted speed limits](posted-speed-limits-plan.md),
  [speeding attitudes and limit-relative shares](platform-speeding-attitudes-and-aggressive-driving-plan.md),
  [speed percentile aggregation](speed-percentile-aggregation-alignment-plan.md),
  [traffic description language](data-traffic-description-language-plan.md) (its `direction` column)
- **Canonical:** [PDF reporting](../platform/operations/pdf-reporting.md)

## Motivation

A report applies one limit, supplied per request and defaulting to 25 mph, to every speed in the
view. Where one street is signed 25 mph and the cross street 30 mph, or where a street's two
directions are signed differently, the headline shares above the limit are wrong for at least one
direction, and the report cannot say which. The posted-limits plan already requires per-direction
attribution and reporting in the sign's unit; what it lacked was lanes to hang limits on and a copy
of them here. The companion plans supply both. This plan is the reporting half: direction as a
first-class grouping for both sensors, and lanes beneath it where a LiDAR deployment has them.

The radar can take the first step on its own. Its readings are signed, so inbound and outbound are
known at the reading, and lost only because the transit worker takes the absolute value. A
radar-only site can report two directions with two limits as soon as the deployment names them,
with no lanes at all.

## Current state

- **One limit per report request.** `ReportRequest.SpeedLimit` (`internal/api/server_reports_generate.go`)
  defaults to 25 with no unit; `report.Config.SpeedLimit` and `typst.SiteData.SpeedLimit` carry it
  to the template, which prints it beside a free-text note. The web `Site` type still reads
  `site.speed_limit`, which the API no longer returns (migration 000014 removed it).
- **The radar reading is signed; the transit is not.** `radar_data.speed` is the signed Doppler
  speed; `radar_objects.classifier` carries `object_inbound` or `object_outbound`; the transit worker
  selects `ABS(speed)` (`internal/db/transit_worker.go`) and `radar_data_transits` has no direction
  column. The OPS243's `R+` and `R-` commands report one direction only (`internal/radar/commands.go`).
- **Percentiles are aggregates over a population of per-vehicle maximum speeds** by nearest rank
  (D-18, `internal/stats.NearestRank`), with 5 mph buckets plotted as shares (DESIGN §4.3). The
  report's rollup and histogram queries take one population per period.
- **Charts are Go SVG:** `RenderHistogram` and `RenderComparison` (two periods, grouped bars) in
  `internal/report/chart/histogram.go`, served to the web by `/api/charts/histogram` and embedded in
  the PDF through Typst.
- **The scene export writes one vehicle speed summary per scene** (`internal/scene/timeline.go`,
  `VehicleSpeedSummary`), with no direction; the public pages read it with one site-level limit from
  `tools/s2-archive/map-marks.json`.
- **The TDL plan reserves `direction`** as `inbound`, `outbound` or `unknown` on the fused transit
  record and cardinal names "requires site calibration"
  ([TDL plan](data-traffic-description-language-plan.md)).
- **Temporal strata masking at 50 transits** is the rule the crash-data plan's report items use
  (BACKLOG, v0.7.7).

## Design

### Direction as the first grouping

A **direction** is the unit every per-direction figure is grouped by. Each deployment has a list
of them, from one of two sources:

| Deployment                  | Directions come from                                                                                                                                                                                                                                                                             |
| --------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| LiDAR, with a lane snapshot | The snapshot's derived directions: lanes grouped by bearing and street, each named ("Kirkham St eastbound"), with a bearing and the limit when every lane in it agrees, else per-lane limits                                                                                                     |
| Radar                       | Exactly two, `inbound` and `outbound`, named and given a bearing and a limit by the operator on the deployment (the config period today, `deployments` after vocabulary Item 14), with the sign convention the sensor is configured with (`R+`, `R-`, or both) recorded as which sign is inbound |
| Either, unset               | One direction, "all", with the request's limit as today, printed as operator-supplied                                                                                                                                                                                                            |

A direction carries: id, name, bearing (degrees clockwise from north, where known), the limit as
signed (value, unit, source, jurisdiction) or none, and, for LiDAR, its lanes.

### Attribution

The companion plan's attribution function answers, for a position, heading and time, the lane,
direction and limit in force, or why there is none. This plan uses it two ways:

- **LiDAR, per track.** A track's direction is the direction of the lane that holds the majority
  of its observed samples; a track with no lane for most of its samples is `unattributed` and
  counted in a direction's population only when the deployment has exactly one. Its reported
  speed is its maximum observed speed, as today; the limit it is compared with is the limit of the
  lane holding the sample where that maximum was observed. A track whose samples fall in lanes with
  two limits (it crossed a split at a sign) is compared per lane, as the posted-limits plan
  requires: its maximum within each lane against that lane's limit, and it counts once in the
  direction's population.
- **Radar, per transit.** The transit worker keeps the sign: a new `direction` column on
  `radar_data_transits` (`inbound`, `outbound`, `mixed`, `unknown`) from the sign of the majority of
  its readings, `mixed` when the signs disagree by more than one reading in ten (two vehicles merged
  into one transit), and the existing speed columns stay absolute. The deployment maps `inbound` and
  `outbound` to its two directions. A `mixed` transit is counted in neither direction and reported
  as such. Where the beam footprint spans a sign the posted-limits plan says compliance for that
  direction is suppressed with the reason; the deployment records the footprint's limit segment, and
  this plan carries the suppression flag without building the footprint geometry.

### A limit that changes over time

A limit is attributed by the time of each measurement, not by the time the operator entered it.
Every lane version in the snapshot carries the date its limit took effect (`speed_limit_effective_from`,
set in the painter, from the sign or the City record), and the snapshots are append-only, so the
history of each lane's limits is the history of its versions. For a measurement at time `t` the
attribution picks the version of that lane whose effective date is the latest not after `t`. Three
cases follow from that rule:

- **The City changes a limit between two surveys.** The operator adds a new version of the same
  lane with the new value and its effective date; the lane keeps its id, so the two surveys are
  compared lane for lane, each against the sign that stood. The survey pages say the limit changed
  (companion plan).
- **A change is entered late.** A limit that took effect in June and is entered in August carries
  June's date. A report for June run after the entry uses the new limit; a report run before it
  carries the snapshot id it read, so the two reports differ for a stated reason.
- **A period spans the change.** The report splits that direction's figures at the effective
  date and prints both limits with their dates, as the posted-limits plan requires, and never
  compares a speed with a limit that was not in force when it was measured.

Every report, API answer and scene export names the snapshot ids it attributed through.

### Aggregates per direction and per lane

For each period in a report, and for each direction:

- count, `p50`, `p85`, `p98`, `max` by nearest rank over that direction's population of per-vehicle
  maximum speeds (D-18, through `internal/stats.NearestRank`, never from buckets);
- the histogram in 5 mph buckets as shares of that direction's population, in the report's display
  unit, with the bucket size in the sign's unit when the sign is in mph and the display is km/h, so
  the limit falls on a bucket edge in its own unit;
- the share above the limit by 5, 10 and 15 units of the sign, in the sign's unit, as the
  limit-relative shares item specifies, per direction;
- per lane within a direction, where lanes exist: count, `p50`, `p85`, `p98`, `max` and the lane's
  own limit, as sub-rows; the histogram is per direction, not per lane, unless a direction's lanes
  carry different limits, in which case each lane gets its own.

The combined population and its figures stay as the report's headline, so a report with directions
adds to today's report and changes none of its existing numbers (the headline populations are the
same tracks and transits).

**Masking.** A direction with fewer than five observations is not reported separately (its
observations stay in the headline); `p98` and `max` print as "--" below 50, the temporal strata
rule, so a quiet side does not publish one driver's speed as a percentile.

### The report

`ReportData` gains a `directions` list per period, each with its name, bearing, limit (value,
unit, source, jurisdiction), overall summary, histogram, shares above the limit and lanes, and
`compare` gains the same. The Typst templates render a **By direction** section after the headline
sections:

```text
By direction                                    Kirkham St eastbound  ·  LAW 25 mph (sign)
                                                Kirkham St westbound  ·  LAW 30 mph (sign)

      share of vehicles                          n     p50    p85    p98    max   >limit
  eastbound  ▐██▌                              1,204   24.1   29.0   34.5   41.2   38.2%
  ────────────┼─────────────────── mph
  westbound   ▐███▌                             987   26.7   31.8   37.0   44.0   29.4%
             5   10   15   20   25   30   35   40
                               ↑ 25        ↑ 30
```

The chart is one **mirrored histogram**: the two directions share the speed axis, one drawn upward
and one downward from it, each with its own limit line and its own shading above the limit. With
more than two directions (an intersection) the section draws one small histogram per direction,
sharing the axis and the share ceiling, in a grid. The Go chart package gains `RenderMirrored` for
the two-direction case and reuses `RenderHistogram` with a shared scale for the grid. The same SVG
is served to the web by `/api/charts/histogram?direction=…` and `…?mirror=a,b`, so the web report
view, the PDF and the public pages (companion plan) show the same form (DESIGN §4.1: meaning and
tick behaviour aligned; §7.2: SVG as the shared intermediate).

Every direction's figures print the limit as signed and its source. A `statutory` limit prints as
"25 mph (default, no sign recorded)"; `operator` prints as "operator-supplied", which is what
today's per-request limit becomes in a report without directions. The cosine correction note and the
radar's hardware table are unchanged.

### The API and the web

- `/api/radar/stats`-family endpoints and the chart endpoints take `direction=<id>`; without it they
  answer the headline population as today.
- A new `GET /api/sites/{id}/directions?at=` lists a deployment's directions with their limits and
  provenance, from the snapshot or the deployment record.
- The reports page shows the directions found for the chosen period, with each limit and source,
  and a mirrored histogram preview; the stale `site.speed_limit` field is removed from the web
  `Site` type.
- The deployment form (site config period today) gains the radar's two direction names, bearings,
  inbound sign and limits.

### The scene export

`VehicleSpeedSummary` gains `directions`: a list of per-direction summaries, each with `id`,
`name`, `bearing_deg`, `speed_limit` (value, unit, source), `track_count`, `p50`, `p85`, `p98`,
`max` and `histogram`, built by the same accumulator from each track's attributed direction. The
headline summary is unchanged. A direction with fewer than five tracks is folded into the headline
and not listed. The export test that refuses personal fields (D-34) gains the case that a direction
summary carries no track id and no per-track speed. The published scene also carries the lanes of
the snapshot in its vectors file (sensor frame), as the kerbs travel today, so the page can draw
them; the vectors exporter already excludes anything but geometry, kind, source, state and height.

### Radar-only sites

A radar deployment with its two directions set reports by direction at once: the transit's new
`direction`, the deployment's names, bearings and limits, and the mirrored histogram. No lane, no
pose and no Gerardus are needed. This is the first deliverable, because it is the smallest and
exercises every surface.

## Scope

### Item 1: radar transit direction

**Summary:** Keep the sign on transits and name the two directions on the deployment.

**Steps:**

1. Migration: `direction` on `radar_data_transits` (`inbound`, `outbound`, `mixed`, `unknown`),
   backfilled by the worker's rebuild from `radar_data`; the `transit_direction` vocabulary
   recorded in the TDL plan's terms.
2. The deployment's two directions: names, bearings, inbound sign, limits as signed; API and form.
3. Tests: a transit of one sign; a `mixed` transit; the backfill on a fixture database.

**Milestone:** v0.7.1 `S`

### Item 2: direction aggregates and the mirrored chart

**Summary:** Per-direction populations, percentiles, histograms and limit-relative shares, the
`RenderMirrored` chart and the grid of small histograms, served to the web.

**Steps:**

1. The direction list per deployment and period, from the deployment record or the lane snapshot.
2. Aggregation per direction through the shared nearest-rank helper; masking.
3. `RenderMirrored`; the chart endpoint's `direction` and `mirror` parameters; golden SVG tests in
   `internal/report/chart/testdata`.

**Milestone:** v0.7.1 `M`

### Item 3: the report's By direction section

**Summary:** `ReportData.directions`, the Typst section, limits printed as signed with their
source, lanes as sub-rows.

**Steps:**

1. Data types and the JSON contract; the templates; the layout checked against
   [PDF report design](../ui/PDF_REPORT_DESIGN.md).
2. Fixtures: two directions with different limits; three directions; a direction masked under five;
   a statutory limit; a lane split at a sign; a limit changed between two periods, and within one
   period so the direction splits at the effective date; a change entered late re-attributing an
   earlier period.

**Milestone:** v0.7.1 `M`

### Item 4: LiDAR attribution into the report and the scene export

**Summary:** Attribute LiDAR tracks to lanes and directions through the snapshot; per-direction
summaries in the scene export; the privacy test case.

**Steps:**

1. Track-to-direction and track-to-lane attribution in the report's LiDAR source path and the scene
   accumulator.
2. `directions` in `VehicleSpeedSummary`; lanes in the vectors file.
3. The export test case; a re-export of one published scene with directions for the companion
   public-site plan to build against.

**Milestone:** v0.7.1 `M`

### Item 5: web report view

**Summary:** Directions and limits shown on the reports page with the mirrored preview; the stale
web field removed.

**Milestone:** v0.7.9 `S`

## Dependencies

- Item 4 depends on the companion plan's snapshot and attribution; Items 1 to 3 do not.
- The limit-relative shares item (v0.7.7) defines the bands this plan prints per direction; this
  plan prints them per direction once that item lands and prints nothing above the limit before.
- Vocabulary Items 14 and 15 (deployments, surveys): until they land the radar directions live on
  the site config period and a report's directions are per report.

## Risks

| Risk                                                                    | Likelihood | Impact | Mitigation                                                                                                                                             |
| ----------------------------------------------------------------------- | ---------- | ------ | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Two vehicles passing in opposite directions merge into one transit      | Medium     | Medium | `mixed` direction, counted in neither, reported with its count                                                                                         |
| The inbound sign convention is recorded wrongly and the directions swap | Medium     | High   | The deployment form shows which sign is inbound beside the sensor's current `R` setting; a report prints the convention; a test fixture per convention |
| A km/h display with mph signs puts the limit between bucket edges       | Medium     | Low    | Bucket in the sign's unit; convert exactly for display (posted-limits plan)                                                                            |
| Per-direction percentiles on small populations read as findings         | Medium     | Medium | Masking at five and fifty; `n` printed on every figure                                                                                                 |
| The mirrored chart is misread as one distribution                       | Low        | Medium | Direction names on each half, the axis through the middle, a legend; the grid form for more than two                                                   |
| Headline numbers change                                                 | Low        | High   | The headline populations are untouched; a test pins today's headline against a fixture before and after                                                |

## Design decisions

- **Direction first, lane beneath.** Reports group by direction because that is what a reader
  compares and what a radar can measure; lanes refine it where a LiDAR deployment has them.
- **The limit travels as signed, with its source**, printed in the sign's unit (posted-limits plan).
  `statutory` and `operator` print as what they are.
- **One chart form on every surface:** the mirrored histogram for two directions, a shared-scale
  grid for more. Chosen over reusing `RenderComparison`'s grouped bars because the mirrored form
  keeps each direction's limit line over its own bars, and over overlaid translucent bars because
  two limits and two shadings overlay badly. The owner may reverse this; the grouped bars are the
  fallback.
- **The radar lands first.** Two directions with limits need no lanes, and prove the data contract
  the LiDAR path then fills.
- **Limits are attributed by measurement time through lane versions with effective dates**, so a
  change between surveys is a new version of the same lane, never an edit of history.
- **Report outputs and re-exported scenes are run outputs, not repository content.** A re-exported
  scene goes to the dataset (D-34); the only committed material is fixtures with made-up numbers.

## Open questions

- Is 50 the right floor for `p98` and `max` per direction, as the strata item uses, or should a
  direction follow the headline's rule?
- Should a track that straddles two lanes of one direction (a lane change) count in both lanes'
  sub-rows, or in the lane holding its maximum only? The direction count is one either way.
- Where do the radar's direction names live until `deployments` exists: a JSON column on the config
  period, or a small table? The JSON column keeps the migration small.
- Does the report print per-lane histograms when lanes in one direction carry different limits, or
  only the sub-rows?

## Checklist

### Complete

- [x] Plan written against the posted-limits model and the companion lane plans

### Outstanding

- [ ] Item 1: radar transit direction and deployment directions (`S`)
- [ ] Item 2: direction aggregates, mirrored chart, chart endpoint parameters (`M`)
- [ ] Item 3: By direction report section and fixtures (`M`)
- [ ] Item 4: LiDAR attribution, scene export directions, privacy test case (`M`)
- [ ] Item 5: web report view (`S`)

### Deferred

- [ ] Schedules per lane (D-16): [speed-limit schedules](../radar/architecture/speed-limit-schedules.md)
- [ ] Radar beam footprint geometry against the lanes: [posted speed limits](posted-speed-limits-plan.md), step 3
- [ ] Cardinal `heading` filters in TDL from the direction bearings: [TDL plan](data-traffic-description-language-plan.md)

### Accepted residuals (no action planned)

- [ ] A radar transit cannot be assigned to a lane; it is assigned to a direction only
- [ ] Before any snapshot or deployment directions exist, a report prints one operator-supplied limit as today
