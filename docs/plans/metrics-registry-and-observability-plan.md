# Metrics Registry and Observability Plan

**Status:** Proposed
**Scope:** canonical metric naming, repo-wide consistency rules, cross-strata enforcement, and future observability/export design
**Related:** [Speed Percentile Aggregation Alignment Plan](speed-percentile-aggregation-alignment-plan.md), [v0.5.0 Backward Compatibility Shim Removal Plan](v050-backward-compatibility-shim-removal-plan.md), [Executive Decisions Register](../DECISIONS.md), [LiDAR Logging Stream Split](../lidar/architecture/lidar-logging-stream-split-and-rubric-design-20260217.md)

## 1. Problem

The repo currently lets the same metric words drift across multiple meanings and
multiple strata:

- docs and plans
- SQL schema and queries
- Go structs and helpers
- proto/JSON contracts
- TypeScript and Swift models
- reports and TDL
- logs and VRLOG overlays
- future observability/export surfaces

The immediate failure case is speed:

- `peak` is used today for a raw track maximum
- `max` already means a raw maximum in aggregate/report surfaces
- `p50/p85/p98` have leaked into per-track work even though they should be
  aggregate-only for speed

Without one design owner for metric names and definitions, every new surface
risks inventing a slightly different meaning.

## 2. Goals

- Define one canonical naming model for metrics across the repo.
- Separate metric level from estimator so track and aggregate metrics stop
  borrowing the same words for different things.
- Define how we check consistency across the pipeline strata.
- Define a low-cardinality tag policy for future export surfaces.
- Prepare a future Prometheus exporter design without committing to
  implementation in this PR.

## 3. Non-Goals

- Implementing a Prometheus exporter now.
- Finalising the formulas for future track metrics such as `typical` and
  filtered `peak`.
- Rewriting all existing code in this document's PR.
- Turning every current metric into a machine-readable registry immediately.

## 4. Canonical Terminology Rules

| Term           | Canonical meaning                                                                                 | Allowed level                                      | Notes                                                             |
| -------------- | ------------------------------------------------------------------------------------------------- | -------------------------------------------------- | ----------------------------------------------------------------- |
| `avg` / `mean` | Arithmetic mean over the stated sample set                                                        | Any level if explicitly defined                    | `avg` can remain as a transport/storage name where already stable |
| `typical`      | Robust central estimate after metric-specific filtering                                           | Track, scene, or session metrics                   | Use when the metric is intentionally not a mean or percentile     |
| `max`          | Raw observed maximum with no outlier rejection                                                    | Any level                                          | `max` is the canonical raw-maximum term repo-wide                 |
| `peak`         | Filtered or context-aware top value                                                               | Only when the filtering rule is explicitly defined | Reserve for future filtered metrics, not raw maxima               |
| `p50/p85/p98`  | Percentiles across a population of values                                                         | Aggregate/report/transit/grouped outputs           | For speed, do not use these on a single track                     |
| `p95`          | Valid percentile term in general, but not the canonical high-speed percentile for speed reporting | Family-specific                                    | `height_p95` can stay; speed `p95` is historical-only legacy      |
| unit suffixes  | Explicit physical units in the leaf name                                                          | Any level                                          | Prefer `_mps`, `_mph`, `_ms`, `_count`, `_ratio`, `_m`            |

## 5. Canonical Metric Shape

Each metric should be defined conceptually using these fields, even before a
machine-readable registry exists.

| Field            | Meaning                                             | Example                                                                               |
| ---------------- | --------------------------------------------------- | ------------------------------------------------------------------------------------- |
| `id`             | Stable repo-wide identifier                         | `track.max_observed_speed_mps`                                                        |
| `family`         | Metric family                                       | `speed`, `height`, `performance`, `ops`                                               |
| `level`          | Observation level                                   | `track`, `transit`, `aggregate`, `cluster`, `scene`, `performance`, `ops`             |
| `estimator`      | Measure type                                        | `mean`, `raw_max`, `filtered_peak`, `p50`, `p85`, `p98`, `count`, `ratio`, `duration` |
| `unit`           | Canonical unit token                                | `mps`, `mph`, `ms`, `count`, `m`                                                      |
| `visibility`     | Surface expectation                                 | `public`, `internal`, `future_stub`, `deprecated`                                     |
| `status`         | Lifecycle state                                     | `stable`, `provisional`, `future_stub`, `deprecated`                                  |
| `aliases`        | Temporary or historical names                       | `peak_speed_mps`                                                                      |
| `source_modes`   | Valid runtime contexts                              | `live`, `pcap`, `pcap_analysis`, `vrlog`                                              |
| `allowed_tags`   | Low-cardinality labels allowed for filtering/export | `site_id`, `sensor_id`, `source_mode`                                                 |
| `forbidden_tags` | Labels that must never become metric tags           | `track_id`, `run_id`, `pcap_file`, `vrlog_path`                                       |

## 6. Seed Naming Decisions

These are the design anchors this plan should enforce first.

| Concept                       | Canonical direction                | Notes                                                                            |
| ----------------------------- | ---------------------------------- | -------------------------------------------------------------------------------- |
| Track running mean speed      | `track.avg_observed_speed_mps`     | Stable existing meaning                                                          |
| Track raw maximum speed       | `track.max_observed_speed_mps`     | Current `peak_speed_mps` should migrate to `max_speed_mps` on unshipped surfaces |
| Track robust central speed    | `track.typical_observed_speed_mps` | Future replacement for track-level percentile misuse                             |
| Track filtered top speed      | `track.reliable_peak_speed_mps`    | Future reserved `peak` metric                                                    |
| Aggregate speed percentile 50 | `aggregate.speed_p50_mph`          | Aggregate-only                                                                   |
| Aggregate speed percentile 85 | `aggregate.speed_p85_mph`          | Aggregate-only                                                                   |
| Aggregate speed percentile 98 | `aggregate.speed_p98_mph`          | Canonical high-end speed percentile                                              |
| Aggregate raw max speed       | `aggregate.speed_max_mph`          | Use `max`, not `peak`                                                            |

## 7. Consistency Across Pipeline Strata

| Stratum                   | What must stay consistent                                        | Example failure                                                                   |
| ------------------------- | ---------------------------------------------------------------- | --------------------------------------------------------------------------------- |
| Docs and plans            | Name, level, definition, migration status                        | One doc says `peak`, another says `max`                                           |
| SQL schema and migrations | Stored meaning and units                                         | `peak_speed_mps` treated as filtered in one place and raw in another              |
| Go domain/storage code    | Computation and field semantics                                  | A percentile helper exported as a track "typical" measure                         |
| Proto/JSON contracts      | Public field name and level semantics                            | Track proto exposes `p98_speed_mps` while docs say percentiles are aggregate-only |
| Web/Swift clients         | Labels and field names                                           | UI says "Peak speed" while API means raw max                                      |
| Reports/TDL/query layers  | Formula and source population                                    | Report `p98` derived from bucket percentiles instead of grouped raw speeds        |
| Logs and VRLOG overlays   | Diagnostic names aligned with canonical meaning                  | Trace logs keep old alias names after public rename                               |
| Future exporters          | Exported metric name and label policy derived from canonical ids | Exporter invents `speed_peak` even though the canonical term is `max`             |

## 8. Enforcement Strategy

### 8.1 Review gate

No new public metric should merge unless:

1. its level is explicit
2. its estimator is explicit
3. its unit is explicit
4. its aliases are documented
5. its allowed and forbidden tags are documented

### 8.2 Planned checks

| Check                       | What it enforces                                                                             | Delivery shape                                              |
| --------------------------- | -------------------------------------------------------------------------------------------- | ----------------------------------------------------------- |
| Alias audit                 | Deprecated names only appear where the migration plan allows them                            | Start with `rg`-based review checklist; later add CI        |
| Percentile-boundary audit   | Track speed surfaces cannot introduce `p50/p85/p98` publicly                                 | Start with design review; later add CI guard                |
| Surface completeness check  | Stable public metrics must list their concrete names across SQL/API/proto/UI/report surfaces | Start in this plan; later move to machine-readable registry |
| Tag cardinality guard       | High-cardinality ids and file paths stay out of exported labels                              | Implement when exporter work begins                         |
| Export name synthesis check | User-defined prefix plus canonical id must remain unique and ASCII-safe                      | Implement with exporter dry-run tooling                     |

### 8.3 Rollout shape

| Phase   | Outcome                                                                                   |
| ------- | ----------------------------------------------------------------------------------------- |
| Phase A | Use this plan as the naming/design gate during reviews                                    |
| Phase B | Add a small `metrics-lint` task that checks aliases and forbidden public percentile usage |
| Phase C | Introduce a machine-readable registry only when the checks need code ownership            |
| Phase D | Generate exporter/docs/tests from the registry once observability work starts             |

## 9. Source Modes and Tag Strategy

The repo already has concrete source concepts that the naming plan should use
consistently:

| Source mode     | Meaning                                                    |
| --------------- | ---------------------------------------------------------- |
| `live`          | Real sensor/UDP ingest                                     |
| `pcap`          | Replay intended to mimic live flow or interactive replay   |
| `pcap_analysis` | Offline replay/analysis mode with preserved analysis state |
| `vrlog`         | Replay of recorded frame bundles                           |

Assumption: the request's `brlog` wording maps to the repo's existing `vrlog`
term.

### 9.1 Allowed export/filter labels

| Label            | Why it is allowed                              |
| ---------------- | ---------------------------------------------- |
| `site_id`        | Bounded deployment dimension                   |
| `sensor_id`      | Bounded hardware/source dimension              |
| `source_mode`    | Core split between live and replay paths       |
| `pipeline_stage` | Useful for low-cardinality performance metrics |
| `stream`         | Useful for log-derived ops counters            |
| `result`         | Useful for success/error/drop outcomes         |

### 9.2 Forbidden export/filter labels

| Label                       | Why it is forbidden                        |
| --------------------------- | ------------------------------------------ |
| `track_id`                  | Unbounded cardinality                      |
| `run_id`                    | Unbounded cardinality                      |
| `pcap_file`                 | File-name explosion and local-path leakage |
| `vrlog_path`                | Same problem as `pcap_file`                |
| `client_id`                 | Short-lived and effectively unbounded      |
| `frame_id` / `timestamp_ns` | One-series-per-sample failure mode         |
| raw error text              | Free-text cardinality explosion            |

### 9.3 Source-mode filtering policy

| Source mode     | Export policy                                     | Notes                                        |
| --------------- | ------------------------------------------------- | -------------------------------------------- |
| `live`          | Include by default                                | Core operational dashboards should see this  |
| `pcap`          | Usually excluded from always-on export by default | Useful for replay validation and local debug |
| `pcap_analysis` | Optional include                                  | Useful for offline tuning/benchmark jobs     |
| `vrlog`         | Exclude from default Prometheus export            | Better suited to logs and replay diagnostics |

## 10. Logging and Filtering

The metrics design must align with the existing logging split rather than
compete with it.

| Stream  | Best for                                                             |
| ------- | -------------------------------------------------------------------- |
| `ops`   | actionable failures, data loss, disconnects, service health events   |
| `diag`  | medium-volume diagnostics, lifecycle state, tuning context           |
| `trace` | per-frame and per-packet telemetry, replay progress, hot-loop detail |

### 10.1 Signal placement rules

| Signal type                                         | Preferred home                                                    | Reason                              |
| --------------------------------------------------- | ----------------------------------------------------------------- | ----------------------------------- |
| reliability failures and dropped-data counts        | `ops` logs plus optional low-cardinality counters                 | actionable and exportable           |
| queue depth, frame latency, connected-client counts | Prometheus-friendly gauges/histograms plus occasional `diag` logs | time-series fit                     |
| per-frame/per-packet detail                         | `trace` logs and VRLOG overlays                                   | too high-volume for default export  |
| per-track raw values                                | DB/API/VRLOG                                                      | too high-cardinality for Prometheus |

## 11. Future Prometheus Export Design

This is a design stub only. It is intentionally not implemented in this PR.

### 11.1 Naming rule

Prometheus names should be derived from a user-defined prefix plus the canonical
metric id.

Pattern:

`<prefix>_<normalized_metric_id>`

Normalization rule:

- replace `.` with `_`
- preserve the canonical leaf name
- keep unit suffixes intact
- keep the registry id itself deployment-neutral

Examples:

| Canonical id                              | Prefix            | Exported name                                         |
| ----------------------------------------- | ----------------- | ----------------------------------------------------- |
| `track.avg_observed_speed_mps`            | `velocity_report` | `velocity_report_track_avg_observed_speed_mps`        |
| `aggregate.speed_p98_mph`                 | `velocity_report` | `velocity_report_aggregate_speed_p98_mph`             |
| `performance.frame_processing_latency_ms` | `main_street`     | `main_street_performance_frame_processing_latency_ms` |

### 11.2 Proposed config shape

```yaml
observability:
  prometheus:
    enabled: false
    listen_addr: ":9108"
    prefix: "velocity_report"
    include_families: ["ops", "performance", "scene", "aggregate"]
    exclude_source_modes: ["vrlog"]
```

Recommended future override path:

- config key: `observability.prometheus.prefix`
- env override: `VELOCITY_PROMETHEUS_PREFIX`

### 11.3 Default export policy

| Metric family/level | Export by default? | Notes                                                    |
| ------------------- | ------------------ | -------------------------------------------------------- |
| `ops.*`             | Yes                | best fit for service-health counters                     |
| `performance.*`     | Yes                | best fit for latency/throughput/queue metrics            |
| `scene.*`           | Usually yes        | low-cardinality scene gauges can work                    |
| `aggregate.*`       | Sometimes          | useful for rolling summaries, not historical report rows |
| `track.*`           | No                 | too high-cardinality                                     |
| `cluster.*`         | Usually no         | usually too ephemeral unless aggregated first            |

## 12. How This Helps Future Work

| Future work                   | Value from this plan                                                                       |
| ----------------------------- | ------------------------------------------------------------------------------------------ |
| Track metric redesign         | Reserves `typical` and filtered `peak` names before code lands                             |
| Raw `peak` to `max` migration | Gives the rename one canonical destination                                                 |
| Aggregate percentile cleanup  | Keeps `p50/p85/p98` tied to aggregate-only semantics                                       |
| Prometheus exporter           | Defines names, labels, prefix policy, and exclusion defaults before instrumentation starts |
| CI/lint enforcement           | Makes alias checks and public-surface audits possible                                      |
| VRLOG diagnostics             | Lets replay overlays reference canonical metric ids instead of ad hoc text labels          |

## 13. Plan

### Phase 0 - Design Alignment

- [x] Write this plan.
- [ ] Use this plan as the naming reference for ongoing speed-metric work.
- [ ] Confirm the source-mode vocabulary (`live`, `pcap`, `pcap_analysis`, `vrlog`) as the canonical tag/filter set.

### Phase 1 - Speed Naming Reset

- [ ] Rename raw public `peak_speed_mps` to `max_speed_mps` on unshipped surfaces.
- [ ] Keep `p50/p85/p98` off all public track-level speed contracts.
- [ ] Record the replacement track metrics and formulas once decided.

### Phase 2 - Consistency Checks

- [ ] Add a lightweight `metrics-lint` task or review script.
- [ ] Fail the check if new public track speed percentiles appear.
- [ ] Fail the check if deprecated aliases appear outside an approved migration window.

### Phase 3 - Observability Stubs

- [ ] Define the first exporter-friendly low-cardinality families (`ops`, `performance`, selected `scene`).
- [ ] Add config support for a user-defined Prometheus metric prefix.
- [ ] Add dry-run validation for metric-name synthesis and label allow/deny rules.

### Phase 4 - Optional Machine-Readable Registry

- [ ] Introduce a machine-readable registry only when enforcement/export generation needs it.
- [ ] Keep the machine-readable form derived from this plan, not an independent source of truth.
