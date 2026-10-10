# Fixed-sensor safety research briefing, October 3 to 9, 2026

- **Status:** Complete. No definition changed. Three edits applied: a synonyms and reserved-name line in the metrics registry's following definitions, an evaluation-source row and a reference in the behaviour plan, and open question B10 there.
- **Scope:** Primary studies published October 3 to 9, 2026 on headway, following distance, post-encroachment time, stop compliance and percentile scorecards; the FMCSA Mississippi LiDAR headway-enforcement evaluation, newly surfaced; and a research briefing on both, supplied for review, whose seven recommendations are assessed against what the repository holds.
- **Related:** [Behaviour analytics plan, Sections 8.3, 10.4 and 11](../../../plans/lidar-behaviour-analytics-plan.md#83-following-behaviour), [following metrics](../../../platform/architecture/metrics-registry.md#following-metrics), [headway report oracle](../headway-report-oracle.md), [0.5.2 sprint review](../0.5.2-sprint-review.md)

A weekly briefing on fixed-sensor safety research was supplied for review. It found no primary
study in the week that changes a definition, surfaced one ongoing programme, and made seven
recommendations for the following metrics. This record checks the programme against what can be
read of it, and each recommendation against the metrics registry, the behaviour plan and the
`l8behaviour` package, so that what the repository already holds is not re-specified under new
names and what it lacks is logged where it will be decided.

## Answer

The briefing is right that nothing published in the week changes the definitions, and right that
the FMCSA programme is an intervention evaluation rather than a metrology study. Of its seven
recommendations, four describe what the repository already holds: the bumper-to-bumper clearance
divided by follower speed is `interaction.following_net_time_gap_s`, defined that way in the
registry, the plan and the code; the spatial gap, the time gap and the speed are persisted per
instant and nothing is reconstructed from an aggregate; the episode it asks for is the encounter,
with its minimum, median and valid time; and vehicle-seconds below each band are reported apart
from any per-encounter statistic. One recommendation, renaming the metric, would be churn across
storage, the API and the report contract for a synonym. Two hold: reserve the name passage headway
for a detector quantity this project does not measure, and track the FMCSA report. One is a real
design question the plan had not logged: whether the scene distribution should also give each
encounter one vote, beside the time-weighted histogram, and how such a view would carry an
interval.

The briefing's value is the source and that question. Its scorecard change, replacing an
"ambiguous headway" with two named metrics, misreads a registry that already names the quantity
and already says what it is not.

## The programme

What the search summaries support; the project page, the vendor's release and the trade press were
unreachable from the review environment, so every row is **Confirm** against its primary.

| Claim                                                                                                                                                                                                                                                                                                                                       | Source                                |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------- |
| FMCSA lists an active research project, _Use of Lidar-based Measurements of Headway Gap for Detecting, Enforcing, and Preventing "Following Too Closely" Violations_, to supply the Mississippi Department of Public Safety's commercial-vehicle enforcement divisions with LiDAR headway-gap tools and assess how much they improve safety | [FMCSA active projects][fmcsa-active] |
| The vendor's May 2025 release: hand-held units that compute the time and the distance between two vehicles, with a built-in camera for evidence; officer training March 10 to April 4, 2025                                                                                                                                                 | [Laser Technology release][lti]       |
| 125 LTI 20/20 TruVISION photo and video LiDAR units deployed with FMCSA funding; results so far anecdotal                                                                                                                                                                                                                                   | [Sponsored trade article][officer]    |
| No findings or final report published                                                                                                                                                                                                                                                                                                       | Every source                          |
| A quasi-experimental before-and-after design over following-too-closely citations, speeding citations and rear-end crashes; the project page updated October 8, 2025                                                                                                                                                                        | The supplied briefing; not seen       |

Three things follow from what is and is not known.

1. **The quantity is not stated by the programme.** The briefing is right that FMCSA's description
   does not say whether "headway gap" is a clearance, a clearance divided by follower speed, or a
   passage headway between two fronts at a line. The vendor's method is the likely answer: a
   handheld unit ranges each vehicle in turn along one beam and divides the range difference by
   the trailing vehicle's speed. That is a clearance time gap along the line of sight, bumper to
   bumper only when the officer stands in line with the lane, and never a passage headway. It
   should be read from the instrument's documentation, not assumed.
2. **It can validate an intervention, not a sensor.** Its outcomes are citations and crashes; its
   observations are chosen by officers. It can say whether objective following measurements change
   enforcement and rear-end crash counts in one state. It cannot say anything about a roadside
   sensor's per-observation accuracy, about any band, or about the distribution of following at a
   site, because nothing in it is a sample of traffic. The briefing's confounders, enforcement
   intensity, training, volume and regression to the mean, all hold.
3. **The denominators differ in kind.** A handheld observation has no opportunity denominator: an
   officer measured a pair because it looked close. The repository's denominator is valid
   following time over every encounter the sensor could support, with every suppressed second
   accounted beside it. The two are not comparable, and the programme's eventual crash analysis
   will have a denominator of its own, vehicle-miles or inspections, worth reading when it exists.

## The recommendations against the repository

| Recommendation                                                                                                                      | What the repository holds                                                                                                                                                                                                                                                                                                                                                                            | Verdict                                                                           |
| ----------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------- |
| 1. Rename the generic metric to `clearance_time_gap_seconds`                                                                        | The metric is `interaction.following_net_time_gap_s`: level, family, estimator and unit in the id, aliases `time_headway` and `thw` kept for migration only, the name audited on every stored payload, API response and report data file. The id is not generic                                                                                                                                      | Not adopted. The synonym is recorded in the registry's definitions; no rename     |
| 2. Define it as leader-rear to follower-front clearance along the path over follower speed                                          | Exactly the definition in the [registry](../../../platform/architecture/metrics-registry.md#following-metrics), [Section 8.3](../../../plans/lidar-behaviour-analytics-plan.md#83-following-behaviour) and the sprint plan's S5, with the extent projected onto the path, a non-positive gap sent to geometry review, and the time gap suppressed below the speed floor                              | Already holds                                                                     |
| 3. Keep `passage_headway_seconds` as a separate point-detector metric                                                               | No detector metric exists; the registry and the plan say the net time gap is not front-to-front passage headway. Radar transits carry no direction, so an inter-transit interval at the sensor is not a passage headway until the worker keeps it                                                                                                                                                    | Adopted as a reservation: the name is kept for a detector quantity, not created   |
| 4. Preserve distance gap, time gap and speed together; never reconstruct distance from an aggregated percentile                     | Spatial gap and net time gap are stored per instant with their sigma; the speed is on the persisted estimate; the report is rebuilt from the estimates, never read back from stored interactions (Section 10.3); the distribution bins each quantity separately                                                                                                                                      | Already holds                                                                     |
| 5. Form one stable leader–follower episode; report episode-level P10, median and duration; one vote per episode in site percentiles | The encounter is that episode: one follower and one leader, with its raw minimum and median over instants under a Monte Carlo interval, its valid following time, and its time below each band. The scene distribution gives each valid second one vote, not each encounter, and derives no pooled minimum or median because the per-encounter draws do not give its interval. A P10 is not computed | Partly holds. The encounter-weighted view and the P10 are logged as B10           |
| 6. Report vehicle-seconds below descriptive reference values apart from episode percentiles                                         | Time below 2.0, 1.5 and 1.0 s per encounter and pooled, with the rate over valid time, in the time-weighted histogram; per-encounter statistics listed apart with their own intervals                                                                                                                                                                                                                | Already holds                                                                     |
| 7. Track the FMCSA project for its calibration protocol, error validation and crash denominator                                     | Not tracked anywhere                                                                                                                                                                                                                                                                                                                                                                                 | Adopted: a row in Section 11 and a reference in Section 16                        |
| Scorecard: replace an ambiguous "headway" with `clearance_time_gap` and `passage_headway`                                           | The scorecard names `following_net_time_gap_s` and `following_spatial_gap_m`. The bare word survives in the report's name, `headway_report_v2`, and the API path `/api/scenes/<id>/headway`, whose contents name the quantity                                                                                                                                                                        | Not adopted. The surfaces keep their names; the definitions carry the distinction |

## What the briefing did not know

- The registry names and audits the quantity. `AuditSurfaceJSON` refuses an alias or an unregistered
  id on every persisted payload, API response and report data file, so a metric cannot drift into
  "headway" by accident.
- The distribution is deliberately time-weighted. Section 10.4 and `following_distribution_v1`
  give each valid second one vote and keep the per-encounter minimum and median apart with their
  intervals, for a stated reason. An encounter-weighted view is an addition to decide, not an
  omission to correct.
- The failure modes it lists for continuous sensing are the plan's: lane pairing is the local
  path and leader choice; occluded bumpers are endpoint support and the suppression reasons;
  fragmentation is B1 and the convergence plan; double-counting a long episode is the clipping
  rule and R5's unique follower opportunity.
- The two-second rule and its bands are already `no_established_threshold`, so "descriptive
  reference values" is the posture the registry takes.

## What changes

1. The registry's following definitions say that the net time gap is what other literature calls
   a clearance time gap or a net headway, and that passage headway, which exceeds it by the
   leader's occupancy time, is a detector quantity the registry does not have and a name reserved
   for one.
2. The behaviour plan's Section 11 gains the programme as an evaluation source, with what it can
   and cannot validate, and Section 16 the reference.
3. Section 13 gains B10: whether the scene distribution publishes an encounter-weighted view beside
   the time-weighted one, of the minimum or a low quantile, and how that view carries an interval.

Nothing is renamed, and no code changes.

## Open questions

- B10, above. The P10 the briefing proposes is one candidate for the per-encounter statistic a
  vote would use; the repository's raw minimum carries a Monte Carlo interval with a common-mode
  fraction for the reason an extremum of a correlated series needs one (Section 9.3), and a P10
  would need the same treatment.
- Does the instrument measure a range difference along one beam, so that it is bumper to bumper
  only when the officer is in line with the lane? To be read from the vendor's documentation and
  the programme's report, which may also state a calibration tolerance.
- FMCSA's own following-distance guidance for commercial vehicles, one second per 10 feet of
  vehicle length below 40 mph and one more above it, is a class-dependent descriptive band of the
  two-second rule's kind. Whether a heavy-vehicle band belongs in the registry waits on class
  evidence, which the field run does not yet have. **Confirm** the guidance from FMCSA's page.
- Should the report's and the API's names carry the quantity, so that "headway" never stands
  alone on a surface a reader meets first? The contents name it; the names are stable; a rename
  is not worth its churn today.

## Provenance

- The supplied briefing, pasted in full, author unstated; its claims about the project page's
  update date and evaluation design could not be checked.
- Search summaries read on October 9, 2026: the FMCSA active-projects listing, the Laser
  Technology release of May 2025, and a sponsored trade article; none of the pages was reachable
  from the review environment.
- Repository revision `a8f9fd5`: `internal/lidar/l8behaviour/` (`encounter.go`,
  `distribution.go`, `metrics.go`, `surface.go`), the metrics registry, and the behaviour plan's
  Sections 8.3, 9.3, 10.3, 10.4, 11, 13 and 16.

[fmcsa-active]: https://www.fmcsa.dot.gov/safety/research-and-analysis/active-research-projects
[lti]: https://lasertech.com/mississipppi-tailgating-detection-lidar/
[officer]: https://www.officer.com/sponsored/article/55390832/from-observation-to-evidence-modernizing-tailgating-enforcement
