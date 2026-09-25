# Headway report oracle

How to generate the synthetic headway report, what it shows, and what it does not claim.

- **Status:** Synthetic oracle implemented (sprint 0.5.2.3); the provisional report over persisted encounters and field promotion are not built
- **Layers:** L8 Analytics, L9 Endpoints (PDF report)
- **Related:** [Behaviour analytics plan, Section 10.4](../../plans/lidar-behaviour-analytics-plan.md#104-first-headway-report), [Following metrics](../../platform/architecture/metrics-registry.md#following-metrics), [Label vocabulary](../architecture/label-vocabulary.md), [PDF reporting](../../platform/operations/pdf-reporting.md), [Following-evidence charts](../../ui/DESIGN.md#44-following-evidence-charts)
- **Code:** [headway](../../../internal/report/headway/doc.go), [following charts](../../../internal/report/chart/following.go), [headway.typ](../../../internal/report/typst/templates/headway.typ), [CLI](../../../internal/cmd/server/headway.go)

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
`typst compile --font-path fonts headway.typ`. Without `--oracle` the command exits 2: nothing but
the oracle exists yet.

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

## What it does not claim

- No physical accuracy. The trajectories are analytic; no sensor data is read.
- No threshold. The bands are descriptive bins with `no_established_threshold`, not a safety
  standard.
- Nothing about any road user beyond the observed passage pair.

## Next stages

1. **Provisional (sprint 0.5.2.4).** Persisted encounters feed `headway.Build` with the
   `provisional` status. The contract already reads an encounter's review-labelled provisional
   block when its estimates are not final, records which block it read, and never pools final and
   non-final values.
2. **Field promotion.** After the held-out validation run and G-GEO-1, G-UNC-1, G-SMO-1 and the
   metric gate. `promoted` is refused until a build can assert them.
