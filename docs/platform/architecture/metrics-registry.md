# Metrics registry and observability

Active plan: [metrics-registry-and-observability-plan.md](../../plans/metrics-registry-and-observability-plan.md)

Canonical metric registry for velocity.report: naming conventions, terminology rules, and enforcement strategy to prevent semantic drift across pipeline strata.

## Problem

The repo lets the same metric words drift across multiple meanings and
multiple strata: docs, SQL schema, Go structs, proto/JSON contracts,
TypeScript/Swift models, reports, logs, and future observability surfaces.

The immediate failure case is speed: `peak` is used for a raw track maximum,
`max` already means a raw maximum in aggregate/report surfaces, and aggregate
percentile labels have leaked into per-track work even though they should be
aggregate-only.

## Canonical terminology rules

| Term           | Canonical Meaning                                             | Allowed Level                                  | Notes                                                           |
| -------------- | ------------------------------------------------------------- | ---------------------------------------------- | --------------------------------------------------------------- |
| `avg` / `mean` | Arithmetic mean over the stated sample set                    | Any level if explicitly defined                | `avg` can remain as transport/storage name where already stable |
| `typical`      | Robust central estimate after metric-specific filtering       | Track, scene, or session metrics               | Use when intentionally not a mean or percentile                 |
| `max`          | Raw observed maximum with no outlier rejection                | Any level                                      | Canonical raw-maximum term repo-wide                            |
| `peak`         | Filtered or context-aware top value                           | Only when filtering rule is explicitly defined | Reserve for future filtered metrics, not raw maxima             |
| `p50/p85/p98`  | Percentiles across a population of values                     | Aggregate/report/transit/grouped outputs       | For speed, keep on grouped/report outputs only                  |
| `p95`          | Valid percentile term in general, but not canonical for speed | Family-specific                                | `height_p95` can stay; speed `p95` is historical-only legacy    |
| unit suffixes  | Explicit physical units in the leaf name                      | Any level                                      | Prefer `_mps`, `_mph`, `_ms`, `_count`, `_ratio`, `_m`          |

## Canonical metric shape

Each metric is defined conceptually using these fields:

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

## Seed naming decisions

These are the design anchors the registry must enforce first.

| Concept                       | Canonical Direction                | Notes                                                                            |
| ----------------------------- | ---------------------------------- | -------------------------------------------------------------------------------- |
| Track running mean speed      | `track.avg_observed_speed_mps`     | Stable existing meaning                                                          |
| Track raw maximum speed       | `track.max_observed_speed_mps`     | Current `peak_speed_mps` should migrate to `max_speed_mps` on unshipped surfaces |
| Track robust central speed    | `track.typical_observed_speed_mps` | Future replacement for track-level percentile misuse                             |
| Track filtered top speed      | `track.reliable_peak_speed_mps`    | Future reserved `peak` metric                                                    |
| Aggregate speed percentile 50 | `aggregate.speed_p50_mph`          | Aggregate-only                                                                   |
| Aggregate speed percentile 85 | `aggregate.speed_p85_mph`          | Aggregate-only                                                                   |
| Aggregate speed percentile 98 | `aggregate.speed_p98_mph`          | Canonical high-end speed percentile                                              |
| Aggregate raw max speed       | `aggregate.speed_max_mph`          | Use `max`, not `peak`                                                            |

## Consistency across pipeline strata

| Stratum                   | What Must Stay Consistent                                        | Example Failure                                                                                    |
| ------------------------- | ---------------------------------------------------------------- | -------------------------------------------------------------------------------------------------- |
| Docs and plans            | Name, level, definition, migration status                        | One doc says `peak`, another says `max`                                                            |
| SQL schema and migrations | Stored meaning and units                                         | `peak_speed_mps` treated as filtered in one place and raw in another                               |
| Go domain/storage code    | Computation and field semantics                                  | A percentile helper exported as a track "typical" measure                                          |
| Proto/JSON contracts      | Public field name and level semantics                            | Track proto exposes percentile-labelled speed fields while docs say percentiles are aggregate-only |
| Web/Swift clients         | Labels and field names                                           | UI says "Peak speed" while API means raw max                                                       |
| Reports/TDL/query layers  | Formula and source population                                    | Report `p98` derived from bucket percentiles instead of grouped raw speeds                         |
| Logs and VRLOG overlays   | Diagnostic names aligned with canonical meaning                  | Trace logs keep old alias names after public rename                                                |
| Future exporters          | Exported metric name and label policy derived from canonical ids | Exporter invents `speed_peak` even though the canonical term is `max`                              |

## Enforcement strategy

### Review gate

No new public metric should merge unless:

1. Its level is explicit
2. Its estimator is explicit
3. Its unit is explicit
4. Its aliases are documented
5. Its allowed and forbidden tags are documented

### Planned checks

| Check                       | What It Enforces                                                                             | Delivery Shape                                              |
| --------------------------- | -------------------------------------------------------------------------------------------- | ----------------------------------------------------------- |
| Alias audit                 | Deprecated names only appear where the migration plan allows them                            | Start with `rg`-based review checklist; later add CI        |
| Percentile-boundary audit   | Track speed surfaces cannot introduce aggregate percentile labels publicly                   | Start with design review; later add CI guard                |
| Surface completeness check  | Stable public metrics must list their concrete names across SQL/API/proto/UI/report surfaces | Start in this plan; later move to machine-readable registry |
| Tag cardinality guard       | High-cardinality ids and file paths stay out of exported labels                              | Implement when exporter work begins                         |
| Export name synthesis check | User-defined prefix plus canonical id must remain unique and ASCII-safe                      | Implement with exporter dry-run tooling                     |

### Rollout phases

| Phase   | Outcome                                                                                   |
| ------- | ----------------------------------------------------------------------------------------- |
| Phase A | Use this plan as the naming/design gate during reviews                                    |
| Phase B | Add a small `metrics-lint` task that checks aliases and forbidden public percentile usage |
| Phase C | Introduce a machine-readable registry only when the checks need code ownership            |
| Phase D | Generate exporter/docs/tests from the registry once observability work starts             |

## Source modes and tag strategy

### Source mode vocabulary

| Source Mode     | Meaning                                                    |
| --------------- | ---------------------------------------------------------- |
| `live`          | Real sensor/UDP ingest                                     |
| `pcap`          | Replay intended to mimic live flow or interactive replay   |
| `pcap_analysis` | Offline replay/analysis mode with preserved analysis state |
| `vrlog`         | Replay of recorded frame bundles                           |

### Allowed export/Filter labels

| Label            | Why Allowed                              |
| ---------------- | ---------------------------------------- |
| `site_id`        | Bounded deployment dimension             |
| `sensor_id`      | Bounded hardware/source dimension        |
| `source_mode`    | Core split between live and replay paths |
| `pipeline_stage` | Useful for low-cardinality perf metrics  |
| `stream`         | Useful for log-derived ops counters      |
| `result`         | Useful for success/error/drop outcomes   |

### Forbidden export/Filter labels

| Label                       | Why Forbidden                              |
| --------------------------- | ------------------------------------------ |
| `track_id`                  | Unbounded cardinality                      |
| `run_id`                    | Unbounded cardinality                      |
| `pcap_file`                 | File-name explosion and local-path leakage |
| `vrlog_path`                | Same problem as `pcap_file`                |
| `client_id`                 | Short-lived and effectively unbounded      |
| `frame_id` / `timestamp_ns` | One-series-per-sample failure mode         |
| raw error text              | Free-text cardinality explosion            |

### Source-Mode filtering policy

| Source Mode     | Export Policy                                     |
| --------------- | ------------------------------------------------- |
| `live`          | Include by default                                |
| `pcap`          | Usually excluded from always-on export by default |
| `pcap_analysis` | Optional include                                  |
| `vrlog`         | Exclude from default Prometheus export            |

## Logging and signal placement

The metrics design aligns with the existing three-stream logging model
(see [structured-logging.md](structured-logging.md)).

| Signal Type                                         | Preferred Home                                                    | Reason                              |
| --------------------------------------------------- | ----------------------------------------------------------------- | ----------------------------------- |
| Reliability failures and dropped-data counts        | `ops` logs plus optional low-cardinality counters                 | Actionable and exportable           |
| Queue depth, frame latency, connected-client counts | Prometheus-friendly gauges/histograms plus occasional `diag` logs | Time-series fit                     |
| Per-frame/per-packet detail                         | `trace` logs and VRLOG overlays                                   | Too high-volume for default export  |
| Per-track raw values                                | DB/API/VRLOG                                                      | Too high-cardinality for Prometheus |

## Future prometheus export design

Design stub only — not implemented.

### Naming rule

Prometheus names derived from user-defined prefix plus canonical metric id:

`<prefix>_<normalised_metric_id>`

Normalisation: replace `.` with `_`, preserve canonical leaf name, keep unit
suffixes intact, keep registry id deployment-neutral.

| Canonical ID                              | Prefix            | Exported Name                                         |
| ----------------------------------------- | ----------------- | ----------------------------------------------------- |
| `track.avg_observed_speed_mps`            | `velocity_report` | `velocity_report_track_avg_observed_speed_mps`        |
| `aggregate.speed_p98_mph`                 | `velocity_report` | `velocity_report_aggregate_speed_p98_mph`             |
| `performance.frame_processing_latency_ms` | `main_street`     | `main_street_performance_frame_processing_latency_ms` |

### Proposed config shape

```yaml
observability:
  prometheus:
    enabled: false
    listen_addr: ":9108"
    prefix: "velocity_report"
    include_families: ["ops", "performance", "scene", "aggregate"]
    exclude_source_modes: ["vrlog"]
```

### Default export policy

| Metric Family/Level | Export by Default? | Notes                                                    |
| ------------------- | ------------------ | -------------------------------------------------------- |
| `ops.*`             | Yes                | Best fit for service-health counters                     |
| `performance.*`     | Yes                | Best fit for latency/throughput/queue metrics            |
| `scene.*`           | Usually yes        | Low-cardinality scene gauges can work                    |
| `aggregate.*`       | Sometimes          | Useful for rolling summaries, not historical report rows |
| `track.*`           | No                 | Too high-cardinality                                     |
| `cluster.*`         | Usually no         | Usually too ephemeral unless aggregated first            |
