# Development-site following evidence on fixed prefixes

This report records the methods, provenance and observed results of two fixed development-site baseline screens.
It follows the Kirk pilot and tests reproducible evidence generation rather than physical metric
accuracy. Both sources produced byte-identical repeated exports with exactly reconciled interval accounting.

- **Status:** Engineering checks complete; archive verification pending
- **Related:** [Experiment l8fol0007](../../../data/experiments/l8fol/l8fol0007.md),
  [Kirk pilot](following-baseline-reproducibility-2026-11.md),
  [evidence exporter](following-evidence-export.md)

## Question and frozen method

Can unchanged defaults export reproducible, correctly accounted following and experimental DRAC
evidence from Columbus at Broadway and Embarcadero at Bryant? Use each site's fixed first
capture, 70 seconds warm-up and up to 230 seconds scoring, retaining the complete DB and its
actual time extent. The [baseline parameter set](../../../config/experiments/weekend-following-baseline.json)
changes zero keys. The specification defines the predeclared engineering rule and stop criteria.

## Frozen provenance and execution

The two-case rule and hand-authored corpus were committed at
`5275b2f32ab9d24ed391fe35dbbfb96d88a5cad3` before either S2 replay. The runtime was
built once through Make at Kirk preregistration commit
`79d36b80d43c92319d1028febe71276c49f4cb29`, based on fresh main `4591a8e49`.
The S2 commit changes specification, corpus, links and report only; it changes no runtime
algorithm. Both tools report version `0.5.1-pre52` and the same build SHA. This separates
what selected the cases from what computed their output.

The exact-defaults file is `config/experiments/weekend-following-baseline.json`, with byte
SHA-256 `9ad7377fa5d356ba5f9d8756d48721708071da3295702350c835110b38b41675` and
execution parameter fingerprint
`sha256:ab0c4035780dcc678bc21b112890c1db6cdc9c802eb3d1ab5d0f8d442e872d08`.
The corpus SHA-256 is `debc45b55b0799cea427462a739005be91731e26b07980f73117c5701e24dc13`;
the canonical first-segment site index SHA-256 is
`60c4a528a4d49225b42c157324c8016c1bfc853f11758c1fa276e87e037642ea`.
The frozen emitted source-manifest SHA-256 is
`02e282d80e084f6d2f0997b711f78a782c3f89f8a2385baa3a6eab229297d14d`.

| Source                | Capture SHA-256                                                    | Source ID suffix after `source/v1/`                                |
| --------------------- | ------------------------------------------------------------------ | ------------------------------------------------------------------ |
| Columbus at Broadway  | `135bf02a909df5eeee1f2e526914d975e52cdb562781729174082e7fd6b5e6f0` | `170038f79f5d33aa0180958ea7b978404fc6e43f3d3ecc9e8b6542277ea90f7d` |
| Embarcadero at Bryant | `3c55a2292c59ee964927f38689e29ac46cbfd2790880f317e295db5f47c29c33` | `1c6dcdca9426d6d907c93a181d77ed009f31198155ddf0047b4c2833ad610863` |

These are the first captures `s2_sf_3_20260902132037_00002.pcap` (724,578,312 bytes)
and `s2_sf_8_20260903144037_00011.pcap` (724,995,832 bytes), respectively. Both hashes
were verified before replay against cohort manifest revision 2's development membership.
The existing annotation-pack registry, including Kirk tuning split revision 2, was loaded
through the eligibility guard. No empty replacement registry was invented. A fresh campaign
ledger retained authorised operations; it is not a claim to have recovered all past operator
history. The bindings use `cohort-site` namespace and the exact manifest site IDs.
No validation, qualification or sealed holdout source was consumed.

The one owned replay lane ran from 08:39:54.253034 to 08:50:07.724592 UTC on
14 November 2026: 613.471558 seconds including source/hash preflight and database setup.
The first pipeline started at 08:43:24, so this wrapper wall time is not an isolated tracking
throughput benchmark. Source reads were from the mounted capture volume; all active evidence
and temporary writes were on internal `/private/tmp`. The owned watchdog paused only its
own process group if internal free space fell below 10 GiB or capture free space below 20 GiB,
and resumed above 14 and 24 GiB. No pause occurred. Scoped `caffeinate -i -w` assertions
ended with the owned replay/export process; no global power settings were changed.

## Retained populations and deterministic checks

Both automatic raw repeats passed the tool's frozen tracking-summary comparison. This is the
specified `tracking_baseline.json`/analytical-summary check, including the declared fragment-join
fields. It is not byte identity of complete raw replay artefacts, evidence databases or runtime
track IDs across independently persisted runs. Only the first replay writes the retained DB.
The separate same-DB check below compares the complete exported bytes, with no ID rewriting or
post-hoc correspondence rules.

| Population or check                            | Columbus at Broadway | Embarcadero at Bryant |
| ---------------------------------------------- | -------------------: | --------------------: |
| Configured warm-up / scoring seconds           |             70 / 230 |              70 / 230 |
| Scoring frames, first / repeat                 |        2,299 / 2,299 |         2,300 / 2,300 |
| Pipeline capture-time frames including warm-up |                3,000 |                 3,001 |
| Automatic summary repeat equal                 |                  yes |                   yes |
| Pipeline births / confirmations                |          1,158 / 528 |           1,660 / 705 |
| Retained estimate rows / observation-box rows  |      66,198 / 66,198 |       55,390 / 55,390 |
| Retained estimate frame instants               |                2,938 |                 2,939 |
| Retained track IDs / exported trajectories     |            668 / 668 |             907 / 907 |
| Pair records, not independent encounters       |                   81 |                    98 |
| Export bytes per copy                          |          562,303,089 |           462,501,696 |
| Same-DB export A/B byte equal                  |                  yes |                   yes |

Births, confirmations and track IDs are distinct software populations; none is a count of
independently verified physical vehicles. The full retained DB contains part of warm-up as well
as scoring. Its interval is not the 230-second scoring population:

| Retained estimate/observation extent | Columbus at Broadway | Embarcadero at Bryant |
| ------------------------------------ | -------------------: | --------------------: |
| First capture Unix nanoseconds       |  1788380443186460330 |   1788471644027858330 |
| Last capture Unix nanoseconds        |  1788380736886903330 |   1788471937815733330 |
| Extent seconds                       |           293.700443 |            293.787875 |
| Complete DB bytes                    |        1,695,956,992 |         1,207,017,472 |

The exporter reads the complete retained evidence, including those warm-up observations. Its row
counts are 66,198 and 55,390 respectively, including zero-duration terminal samples.
No scoring-only export or capture-start offset is inferred. The capture-time checks report no
backwards steps, duplicate timestamps or clamped gaps; maximum frame gaps were approximately
0.107 and 0.108 seconds. These are software time checks, not synchronisation calibration.

## Exact interval accounting

The total is summed **follower-track time**, including gaps within each readable track span.
Concurrent followers contribute concurrently: it is neither elapsed site time nor unique
physical exposure. Pair-record counts must not be multiplied into an encounter rate. The two
exports for each site are regeneration checks of the same evidence, not additional observations.

| Mutually exclusive timeline bucket (seconds) | Columbus at Broadway | Embarcadero at Bryant |
| -------------------------------------------- | -------------------: | --------------------: |
| `following_eligible`                         |            88.395155 |            202.381849 |
| `following_excluded`                         |           199.809114 |            100.575113 |
| `no_leader`                                  |          1136.217257 |         1141.72589697 |
| `pairing_excluded`                           |          4604.191085 |         3430.95514206 |
| `record_gap`                                 |          2221.795764 |         2500.53612267 |
| **Total follower time**                      |      **8250.408375** |      **7376.1741237** |

| Mutually exclusive experimental DRAC outcome (seconds) | Columbus at Broadway | Embarcadero at Bryant |
| ------------------------------------------------------ | -------------------: | --------------------: |
| `ineligible_evidence`                                  |           177.403812 |              84.18122 |
| `invalid_input`                                        |             3.999406 |                     0 |
| `missing_input`                                        |            16.304202 |             16.193972 |
| `no_candidate`                                         |          5740.408342 |         4572.68103903 |
| `non_closing`                                          |             48.89491 |            138.885825 |
| `record_gap`                                           |          2221.795764 |         2500.53612267 |
| `supported`                                            |            41.601939 |             63.496024 |
| `unresolved_positive_gap`                              |                    0 |              0.199921 |

For each site, summing raw row intervals, the exclusive timeline buckets and the exclusive DRAC
outcomes gave the same integer nanosecond total exactly. Record gaps remain explicit; unknown
or ineligible evidence was not silently removed. No cross-source pooling is needed to pass.

| DRAC outcome row count    | Columbus at Broadway | Embarcadero at Bryant |
| ------------------------- | -------------------: | --------------------: |
| `ineligible_evidence`     |                1,780 |                   846 |
| `invalid_input`           |                   40 |                     0 |
| `missing_input`           |                  172 |                   166 |
| `no_candidate`            |               58,056 |                46,620 |
| `non_closing`             |                  490 |                 1,392 |
| `record_gap`              |                5,244 |                 5,724 |
| `supported`               |                  416 |                   638 |
| `unresolved_positive_gap` |                    0 |                     4 |

The 416 and 638 supported rows carry nominal experimental arithmetic, not validated collision
risk. They are not filtered solely from valid headway series: speed-floor and finality
conditions differ. Missing joint uncertainty bounds stay missing; no confidence interval or
probabilistic safety claim is manufactured. Online estimates, inferred endpoints and unresolved
geometry continue to be review evidence.

## Diagnostic exclusions and review priorities

The primary evidence reason selects one suppression explanation for a row, while point-level
reasons can coexist. Those reason counts do not form an exclusive duration partition; use the
accounting tables above for reconciliation.

| Primary evidence reason, rows | Columbus at Broadway | Embarcadero at Bryant |
| ----------------------------- | -------------------: | --------------------: |
| `ambiguous_leader`            |                  476 |                    40 |
| `class_not_supported`         |                1,738 |                   754 |
| `extent_not_converged`        |                   14 |                    40 |
| `insufficient_observation`    |                    0 |                     1 |
| `no_common_path`              |               46,071 |                34,980 |
| `orientation_unresolved`      |                   10 |                    18 |

No-common-path dominates these named decisions. That motivates inspection of path membership,
identity continuity and leader selection; it is not evidence that the vehicles lacked a physical
following relationship. Class, orientation and extent exclusions warrant the same source-bound
review before any gate is relaxed. Columbus also has 40 `invalid_input` DRAC rows (3.999406
follower seconds); these remain explicit rather than being replaced by nominal values.

The scoring-window tracker summaries record heading-jitter proxies of 51.815041° and 52.848974°
respectively, over 46,484 and 42,107 samples. Speed-jitter proxies are 0.169324 and 0.222580 m/s
(52,109 and 45,029 samples). Course-alignment p50/p90 proxies are 13.5°/63° and 18°/67.5°
(17,051 and 14,006 samples). Their declared population is confirmed tracks at update entry,
including terminated tracks, within the scoring window. These are unlabelled tracker diagnostics,
not physical yaw or speed errors; do not choose a fix or declare either site the better visibility
cohort from these numbers alone. Read them alongside the published heading-transform investigation,
which concerns a separate rendering defect. The existing Kirk report provides source-bound review nominations. Actual
occlusion, visible facets, identity correspondence and vehicle direction still need inspection
against the underlying points or independent references.

### Source-bound review nominations

Two illustrative periods can be nominated from the retained summaries without claiming operator
review. For Columbus, inspect sample 1788380485185991330 Unix nanoseconds: follower
`trk_014c2491-0575-4b49-9a07-a42182667a5a` and leader
`trk_640ffef8-b573-4de7-bce6-91993032fef4` have nominal DRAC 0.694022 m/s² and closing speed
4.281210 m/s. For Bryant, inspect sample 1788471917216158330: follower
`trk_04e6345b-7000-42bb-bf90-1c7c348cda37` and leader
`trk_2c1b55c5-8ef9-4ff1-8b81-ada11967b4f0` have nominal DRAC 0.00170537 m/s² and closing speed
0.242994 m/s. These are source/version-bound tracker IDs and point calculations, not verified
vehicle identities or qualified extremes. They are not a matched site comparison.

Inspect approximately one second either side, verifying seek landing and the same source/version;
capture-relative offsets are not established here. A second nominated sample of either pair is
part of the same period, not another independent encounter. The Kirk shortlist supplies excluded
and record-gap cases; complete physical and negative-case review remains pending.

## Export provenance and resource observations

Both sources select online `cv_kf_v1`, `medoid_v0` observations and the exact baseline parameter
fingerprint above. The exporter uses published boxes (`published_box/medoid_v0`) and preserves
its full analysis method identity in each document. The guard policy identity is
`sha256:3be8da2d17749c47f64165c93f8841d296519b73dd2ca28dfe211bbae53d5f63`.
The source manifest's top-level calibration is the placeholder
`calibration/v1/33b2530bc8f586eba3ceedb3b2c66351e471178d85e417c727f6a9a6ba982edd`,
not either site's actual observation calibration. Read-only inspection of the completed databases
found Columbus calibration
`calibration/v1/9aef58534232a6c8d29a1030b3b153bf0745329e30c22804e0b62b5bc4555f08`
and Bryant calibration
`calibration/v1/2531210ed7059b23ca650ec4330fb9baf0377ef91334efc205b56522a798d8c4`.

This discrepancy is retained as a producer metadata limitation. The source-manifest writer uses
`identityCalibration`, whereas the retained-observation path assigns `siteCalibration` from the
index: north azimuth 35° for Columbus and 353° for Bryant. The code constructs a transform whose
content contributes to observation identity; it does not independently establish physical ENU
alignment. Do not use the top-level manifest placeholder as the calibration of either site's
observations, and do not rewrite the archived manifest after execution. A producer follow-up
should record the actual calibration per case and test manifest/database agreement.

For each exact exported source/estimator/model/parameter/stage group, the retained DB has
`COUNT(DISTINCT calibration_id) = 1` across all 66,198 or 55,390 estimates respectively.
No mixed-calibration row was selected away.

The analysis method is `following_encounter_v1/2789569b98eff6a4`, with the same uncalibrated path,
pairing and exposure parameters described in the Kirk pilot. No field-derived acceptance limits
are adopted here.

| Exact exported content SHA-256 | Value                                                              |
| ------------------------------ | ------------------------------------------------------------------ |
| columbus-broadway A = B        | `35ad14177657d51801830480b1f9a0cc4c9debd81b898994f5b0400e518f4750` |
| embarcadero-bryant A = B       | `36040346da11236453b8a123bef36c0c11c7351796d33a2b3486a73f51efd0ae` |

Columbus DB SHA-256 remained
`9f8b86a05a8f2132849991907df99562f9d484effe85c2c2fa9a9d50bf86f6f6`
from before A to after B. Bryant DB SHA-256 remained
`1c9cf431b102088d9b67de191544734789f3d64535639ad1004e4acefb3e195f`
from before B to after B; no pre-A hash claim is made for Bryant.

Bryant A completed the exporter and retained its finished JSON, but an optional `/usr/bin/time -l`
wrapper then returned status 1 because sandbox policy denied `sysctl kern.clockrate`. This is
an instrumentation failure, not an absent export. A was preserved without rerunning it. Bryant B
ran once with permitted instrumentation access, exited zero, and produced identical bytes.
Its measured wall time was 10.09 seconds, maximum resident set 1,835,892,736 bytes (about 1.71 GiB),
and peak memory footprint 1,930,696,000 bytes (about 1.80 GiB). These measurements include guard,
DB read, analysis and JSON write; they are not isolated algorithm benchmarks. They also cannot
be compared directly with the first export's colder reads or supervisor wall time. Columbus
exports had already started before this optional instrumentation was added; their peak-memory
measurements are unavailable.

## Archive and decision

Archive verification is pending; all completed local evidence is retained.

Both fixed development prefixes satisfy the predeclared same-DB regeneration and exact-accounting
checks. The manifest/database calibration discrepancy above remains a provenance limitation and
a named producer follow-up, rather than an unqualified claim that all producer metadata agrees. Their automatic raw-repeat analytical summaries also agree at the declared scope. This
ends the bounded two-case screen; ample remaining wall budget is not a reason to read additional
sources or tune against observed yield. No parameter, estimator or physical-metric adoption is
made. The useful next work is source-bound review of paths, leader changes, classes and geometry,
then independently referenced accuracy checks.

The present source identity does not encode replay window boundaries. These sources are
operationally separate from the Kirk pilot, and their complete retained timestamps are reported
explicitly. Separate output directories are not a remedy for overlapping source/version identity:
do not pool a later replay of a different window as independent exposure. A source-window identity
contract is follow-up engineering work before automated aggregation of such campaigns. Facet
estimation, 20 Hz, SRAC simulation and physical metric validation remain separate experiments.
