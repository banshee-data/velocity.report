# Headway report oracle

How to generate the headway report, as a synthetic oracle or as a provisional field run over
persisted estimates, what each shows, and what neither claims.

- **Status:** Synthetic oracle (sprint 0.5.2.3) and provisional field run (sprint 0.5.2.4) implemented; field promotion is not built
- **Layers:** L8 Analytics, L9 Endpoints (PDF report)
- **Related:** [Behaviour analytics plan, Section 10.4](../../plans/lidar-behaviour-analytics-plan.md#104-first-headway-report), [Following metrics](../../platform/architecture/metrics-registry.md#following-metrics), [Label vocabulary](../architecture/label-vocabulary.md), [PDF reporting](../../platform/operations/pdf-reporting.md), [Following-evidence and headway distribution charts](../../ui/DESIGN.md#44-following-evidence-and-headway-distribution-charts), [Refinement criteria](retrospective-refinement-criteria.md)
- **Code:** [headway](../../../internal/report/headway/doc.go), [field run](../../../internal/report/headway/fieldrun/fieldrun.go), [persisted-estimate adapter](../../../internal/lidar/l8behaviour/adapter.go), [following charts](../../../internal/report/chart/following.go), [headway.typ](../../../internal/report/typst/templates/headway.typ), [CLI](../../../internal/cmd/server/headway.go)

## Generate it

```bash
velocity report headway --oracle --output ./reports              # US letter
velocity report headway --oracle --output ./reports --paper a4
```

From a checkout, `go run -tags=pcap ./cmd/velocity report headway --oracle --output ./reports`.
Typst is embedded in release builds; for development, `make install-typst` and add `bin/` to
`PATH`.

The command prints the status, then writes `headway_synthetic_oracle_report.pdf` and
`headway_synthetic_oracle_report_sources.zip`. The archive recompiles on its own with
`typst compile --font-path fonts headway.typ`. Exactly one of `--oracle` and `--db` must be
given; the second is the [provisional field run](#provisional-field-run).

## What it is

The oracle is the first stage of Section 10.4. It runs every frozen `l8behaviour` encounter
scenario through `AnalyseFollowing` with that scenario's own parameters, one capture per scenario,
and renders the result through the real report path: Go SVG charts, the Typst template, the PDF
and its source archive. The scenarios' bumpers, gaps and time gaps are known by construction, so
every number the report prints can be checked by hand against the scenario's comment in
`encounter_fixtures.go`.

| Section                | Shows                                                                                                                                                                                                                                                                      |
| ---------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Status and scope       | The status label and its note; the fixed statements of what the numbers are and are not                                                                                                                                                                                    |
| Method versions, bands | Every method id, and each named band with its registered duration and rate metric and benchmark kind                                                                                                                                                                       |
| Aggregates             | Per version group: pooled band exposure with its opportunity denominator, per-metric distributions of supported encounter values with suppressed counts by reason, and the time-weighted net-time-gap distribution with every suppressed second beside it                  |
| Encounters             | Per encounter: both tracks and their locators, the directed path, stage and value block, provenance, every measurement with its uncertainty and opportunity, endpoint sources, both endpoints at the minimum gap, where the time went, unsupported and predicted intervals |
| Captures and followers | Every track taken as a follower: its path or the conditions that refused it, and its leader, free-flow, suppressed and record-gap time                                                                                                                                     |

## Reading it

- **Status.** `SYNTHETIC ORACLE` is printed in the page header and footer, as a background mark,
  in a banner, in the PDF title and keywords, and on every chart. The builder refuses to label
  analytic fixture trajectories anything else, refuses the synthetic label on anything else, and
  refuses `promoted` until the promotion gates exist.
- **Names.** Every metric, reason, stage, source, role and condition is the registry id or
  vocabulary token, verbatim. A report-side test fails when any printed name is unregistered, and
  when any verdict language (a score, a trait of a road user, a category) appears anywhere.
- **Suppression.** A suppressed value prints as `suppressed: <reason>` and has no number. A printed
  `0 s` is a measured zero, such as the occlusion encounter's time below 1.5 s, never a stand-in.
- **Version groups.** The ambiguous scenario is analysed with a wider grouping bound, so its two
  encounters form group A2 and are never pooled with the other six. Neither has enough valid
  following time for band exposure, so A2's pooled rates are suppressed with
  `insufficient_observation` and its distribution is all suppressed time.
- **Predicted gap.** The occlusion encounter's coasted frames are listed as one `review_only`
  predicted interval with coast age 0.1 to 0.5 s, drawn dashed and hollow, and counted as
  `not_observed` and `model_degraded` time. Nothing in an aggregate reads it.
- **Distribution.** Bars are valid following time of the encounters whose band exposure is
  supported; the columns beside them are the rest of the group's accounted time, by reason.
  Together they are the denominator exactly, and the valid time left of a band's rule is exactly
  the pooled time below that band.
- **Uncertainty.** Encounter minima and medians carry 90 % Monte Carlo intervals over 2,000 draws;
  valid following time declares none; aggregates declare none rather than inventing one.

## Checking a change

The rendered contract is pinned by golden files: the report's `data.json` and every chart.

```bash
go test ./internal/report/headway/                               # contract, goldens, registry and language checks
go test ./internal/report/headway/ -run Golden -update           # rewrite the goldens after reviewing a change
```

Text in the goldens must match exactly and numbers within a tolerance that absorbs only the
last-bit drift a fused multiply-add can put into a Monte Carlo draw on arm64. With `typst` on
`PATH`, `TestGenerateCompilesThePDF` compiles the PDF and recompiles its archive; without it, the
test is skipped and the packaging is still exercised through a stand-in `typst`.

## Provisional field run

The second stage of Section 10.4: persisted estimator output through local path pairing,
interaction persistence and the same renderer, with every page and chart labelled `PROVISIONAL`.
It proves the integration and shows what evidence is missing; it claims no physical accuracy.

### Input: an evidence database

The run reads `lidar_track_estimates` from an evidence database: the offline SQLite file a replay
writes when `replayeval.Config.ObservationDBPath` is set, with each estimate linked to its
immutable observation in `lidar_observations`. The state-estimation baseline tool writes one with
`-evidence-dir` (or `-observations-db`), and `-experiment fixed_lag_rts` adds the refined
`fixed_lag` and `final` rows beside the online ones. `velocity lidar pcap-replay --observations`
writes an observation VRLOG, not an evidence database, and the server writes no estimates at all.

### Run it

```bash
velocity report headway --db evidence.db                      # lists sources and their stages
velocity report headway --db evidence.db --source source/v1/<digest>                  # stage final
velocity report headway --db evidence.db --source source/v1/<digest> --stage online
velocity report headway --db evidence.db --source source/v1/<digest> \
    --stage fixed_lag --param-hash sha256:<hash>              # one fixed_lag horizon of several
velocity report headway --db evidence.db --source source/v1/<digest> \
    --solid-bodies --stage online                             # the solid bodies beside the estimates
```

`--stage` is `final` (the default, the stage a production reader asks for), `fixed_lag` or
`online`. When a source holds several versions at the stage, as a `fixed_lag_rts` replay does
with one `fixed_lag` version per horizon, the run refuses to choose and lists them; name one with
`--estimator`, `--obs-model` or `--param-hash`. `--solid-bodies` reads `lidar_track_solid_bodies`
instead of `lidar_track_estimates`: the near-edge body a `solid_body` replay files beside each
online estimate, which exists at the `online` stage only. The command prints the status and the
run's figures before it renders, then writes `headway_provisional_report.pdf` and its source
archive.

### What one run does

1. Selects exactly one estimate version for the source at the stage, from the point estimates or,
   with `--solid-bodies`, the solid bodies.
2. Builds one trajectory per track from its rows (`l8behaviour.TrajectoriesFromEstimates` or
   `TrajectoriesFromSolidBodies`).
3. Runs `AnalyseFollowing` under the analytic scenarios' bounds, which are uncalibrated; the
   report prints them and their hash.
4. Stores every encounter's event, instants and windows in the same database, write-once per
   version, then requires the store to hold at that version exactly the events this run produced.
   A second run of the same version writes nothing; a method changed without a version change
   fails here, and `InteractionStore.DeleteVersion` clears the stale version.
5. Builds the report from the same trajectories and analysis with status `provisional`. The report
   is always rebuilt from the persisted estimates, which are canonical, and never read back from
   the stored interactions, which hold neither the fitted path nor each follower's timeline.

It refuses a capture whose frame period would let the 150 ms record-gap bound hold a missing row
as observed time: the bounds assume 10 Hz.

### What a persisted row supports

An estimate row carries pose, velocity and covariance, the reference point and support token its
writer stated, and nothing of the solid body held beside them. The adapter claims no more:

| Sample field | Read as                                                                                                  |
| ------------ | -------------------------------------------------------------------------------------------------------- |
| Reference    | The row's `reference_point`; never inferred from `measurement_source`                                    |
| Support      | The row's `support_instant`, which must be `observed`: a row records no last observed time for any other |
| Lifecycle    | `geometry_converging` at or above 0.5 m/s, `initialising` below: no persisted geometry can establish it  |
| Heading      | None persisted, so front and rear cannot be told apart                                                   |
| Extents      | None persisted                                                                                           |
| Class        | `unknown`: the classifier label is not persisted, and following is defined for rigid vehicles only       |
| Stage        | The row's own; the only place a sample is `final`                                                        |
| Acquisition  | The row's `measurement_unix_nanos`, beside the frame's capture time                                      |

Today's writers state the geometry that entered the filter: `visible_obb_centre` when an OBB centre
did, `cluster_medoid` under `medoid_v0` or the OBB model's medoid fallback. Migration 000057 gave
older rows exactly those values and `observed`; a row whose measurement source that mapping does not
know keeps an empty reference, and the run refuses it. Neither reference is a place on the body:
the centre of the box around one frame's returns moves with what the sensor saw, and the medoid is
a point in the cluster. No stage changes that, so no follower's path is fitted from today's point
estimates at any stage, and there is no encounter. A row that states `body_centre` is read as one.

A solid-body row carries the body itself, and its sample says what the reading says:

| Sample field | Read as                                                                                              |
| ------------ | ---------------------------------------------------------------------------------------------------- |
| Reference    | `body_centre` after a near-edge fix; `cluster_medoid` while seeding or after a faceless lapse        |
| Support      | The tracker's token, including an explained absence; `cluster_split` for a fragmented observed frame |
| Heading      | The reading's orientation belief and provenance                                                      |
| Extents      | Length and width beliefs with provenance, converged against the default bounds                       |
| Faces        | Only the end faces a near-edge fix used, on an observed instant with a resolved heading              |
| Class        | The latest row's class belief, at its effective class                                                |
| Stage        | The row's, which must be the reading's own: `online` today                                           |
| Acquisition  | The measurement's acquisition time, which the reading carries                                        |

The shadow filter updates its covariance in float32 without re-symmetrising it, so each
off-diagonal pair is averaged when it agrees within round-off, as for a point estimate, and the row
is refused otherwise.

### kirk0

The pcap test `TestKirk0ProvisionalHeadway` replays kirk0 (20 s of warm-up, then the remaining
63 s) into an evidence database with `fixed_lag_rts`, under both measurement models and with
`solid_body`, and runs each stage. Every figure below is a result, not a failure. Since #618 a
point estimate never refers to the body, so neither measurement model fits a path. The second
table keeps the figures from before #618, when the OBB centre was read as the body centre.

| Input, stage                            | Tracks | Paths fitted | Encounters | Accounted | Valid | Suppressed time by first reason                                                                     |
| --------------------------------------- | -----: | -----------: | ---------: | --------: | ----: | --------------------------------------------------------------------------------------------------- |
| `medoid_v0` estimates, online           |     60 |            0 |          0 |       0 s |   0 s | none; every path `weak_support`                                                                     |
| `obb_centre_v1` estimates, every stage  |     62 |            0 |          0 |       0 s |   0 s | none; every path `weak_support`                                                                     |
| Solid bodies (`--solid-bodies`), online |     60 |            7 |          3 |     2.6 s |   0 s | `no_common_path` 1.3 s, `not_observed` 0.6 s, `class_not_supported` 0.5 s, `ambiguous_leader` 0.2 s |

On the solid bodies 381 of 1,952 samples are on the body centre and 1,524 have a resolved heading;
every sample carries length and width beliefs and an acquisition time. All five evaluated instants
are `class_not_supported`, `insufficient_observation`, `extent_not_converged` and
`estimate_not_final`: only 3 of the 60 tracks end classed as rigid vehicles.

Before #618 (OBB-centre rows read as body centres):

| Measurement model, stage            | Tracks | Paths fitted | Encounters | Accounted | Valid | Suppressed time by first reason                                                                     |
| ----------------------------------- | -----: | -----------: | ---------: | --------: | ----: | --------------------------------------------------------------------------------------------------- |
| `medoid_v0` (production), online    |     60 |            0 |          0 |       0 s |   0 s | none; every path `weak_support`                                                                     |
| `obb_centre_v1`, online             |     62 |           15 |          6 |    13.8 s |   0 s | `class_not_supported` 5.8 s, `ambiguous_leader` 4.3 s, `not_observed` 2.3 s, `no_common_path` 1.4 s |
| `obb_centre_v1`, fixed_lag 3 frames |     62 |           14 |          6 |    12.2 s |   0 s | `class_not_supported` 5.8 s, `no_common_path` 3.9 s, `not_observed` 2.2 s, `ambiguous_leader` 0.3 s |
| `obb_centre_v1`, fixed_lag 0.5 s    |     62 |           13 |          4 |     8.0 s |   0 s | `class_not_supported` 5.8 s, `not_observed` 2.0 s, `ambiguous_leader` 0.2 s                         |
| `obb_centre_v1`, fixed_lag 1 s      |     62 |           14 |          4 |     8.1 s |   0 s | `class_not_supported` 5.8 s, `not_observed` 2.1 s, `ambiguous_leader` 0.2 s                         |
| `obb_centre_v1`, fixed_lag 2 s      |     62 |           15 |          4 |     8.1 s |   0 s | `class_not_supported` 5.8 s, `not_observed` 2.1 s, `ambiguous_leader` 0.2 s                         |
| `obb_centre_v1`, final              |     62 |           16 |          4 |     8.3 s |   0 s | `class_not_supported` 5.8 s, `not_observed` 2.3 s, `ambiguous_leader` 0.2 s                         |

Under the production model the persisted pose is a medoid, so no path is fitted and the report
states that no encounter was found. Before #618, under the OBB-centre candidate, pairs were found
on fitted paths and the same 63 instants were evaluated at every stage; all 63 were
`class_not_supported` and `orientation_unresolved`, and 79 to 89 % were `extent_not_converged`.
Smoothing changed which paths fit and which pairs were found, not whether a value could be
published. The test also checks, at every stage, that valid and suppressed time add up to the
accounted time, that the data file and every stored row pass the surface audit, that nothing prints
verdict language, and that a second run stores nothing and builds an identical `data.json`.

```bash
go test -tags pcap ./internal/report/headway/fieldrun/ -run Kirk0 -v   # about a minute; needs git lfs pull
```

## What it does not claim

- No physical accuracy. The oracle's trajectories are analytic and read no sensor data; a
  provisional report reads sensor data and has not passed the promotion gates.
- No threshold. The bands are descriptive bins with `no_established_threshold`, not a safety
  standard.
- Nothing about any road user beyond the observed passage pair.

## Next stages

1. **Evidence the provisional run lacks.** Class, heading and extent beliefs persisted with each
   estimate, so the adapter can project endpoints; until then every field instant is suppressed.
2. **Field promotion.** After the held-out validation run and G-GEO-1, G-UNC-1, G-SMO-1 and the
   metric gate. `promoted` is refused until a build can assert them.
