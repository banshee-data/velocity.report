# Alpha Release Plan: Radar FFT Classifier & LiDAR Stop-Compliance Survey

## Executive Summary

Deliver an alpha release that adds (1) FFT-based radar observation classification for pedestrian/auto/bike metrics, and (2) LiDAR-powered site survey tools that report the percentage of vehicles making a complete stop at intersections. The plan below inventories current capabilities, gaps, required documentation, workstreams, dependencies, and time estimates.

## Current State (Ground Truth)

- **Radar stack**: Ingests raw JSON events into `radar_data`, hardware classifier outputs to `radar_objects`, and transit sessionization populates `radar_data_transits` (see `ARCHITECTURE.md`). FFT output can be enabled on the OPS243 sensor (`internal/radar/commands.go`), but there is **no pipeline to parse/store FFT bins** or to classify pedestrians/bikes/cars from FFT features.
- **LiDAR stack**: Tracking pipeline is integrated end-to-end (`docs/features/lidar-tracking-integration.md`); foreground extraction, clustering, and tracking populate `lidar_tracks` and `lidar_track_obs`. **Stop-compliance metrics are not implemented**; analysis-run bookkeeping is still TODO and there is no notion of intersection geometry/stop-line zones.
- **Reporting/UI**: PDF generator and web dashboards surface existing transit metrics; no UI/report surfaces FFT-derived classes or stop-compliance.

## Scope & Success Criteria (Alpha)

1. **Radar FFT classifier**: Capture FFT data, classify observations into pedestrian/auto/bike, and surface counts/percentiles in API, dashboard, and PDF.
2. **LiDAR stop-compliance survey**: Define intersection geometry, detect stop events, and output percent of vehicles with complete stops over a survey window (API/CLI + PDF/export).
3. **Docs & comms**: Ship operator-facing setup guides, feature specs, and release collateral suitable for an alpha user cohort.

## Workstreams, Deliverables, Estimates

| Workstream | Key Deliverables | Estimate (person-weeks) | Critical Dependencies |
| --- | --- | --- | --- |
| **A. Radar FFT Classification** | FFT packet capture & storage; feature extraction (per-frame/track FFT bins); model training (ped/auto/bike); realtime inference in Go pipeline; API fields & DB schema for classified observations; dashboard/PDF surfaces | **4** | Hardware in FFT mode; labeled FFT dataset; modest GPU/CPU for training; DSP/ML review |
| **B. LiDAR Stop-Compliance Survey** | Intersection/stop-line geometry config; stop detector (speed/position thresholds on tracks); aggregation to % complete stops; CLI/API endpoint; PDF & CSV export; validation harness with annotated sessions | **3** | Calibrated site geometry; quality LiDAR captures at intersections; tracker stability metrics |
| **C. Validation & Observability** | Benchmarks on recorded sessions; unit/integration tests for classifiers & stop detector; metrics/logging (false positive/negative rates, confidence); field checklist | **1** | Recorded fixtures; baseline metrics targets |
| **D. Docs, Comms, Release Assets** | Feature specs, operator guides, API/web report updates, sample configs, release notes, short demo video script/cut | **1** | Finalized UX/API; dataset screenshots; narration |

**Total:** ~9 person-weeks (parallelizable across radar/ML vs LiDAR/UX tracks).

## Documentation Plan (create/update)

- **Feature specs**: This plan (docs/features/). Add focused specs per feature: `docs/features/radar-fft-classifier.md`, `docs/features/lidar-stop-survey.md`.
- **Operator guides**: Update `docs/src/guides/setup.md` and `cmd/radar/README.md` for FFT enablement and configuration flags; add LiDAR survey how-to in `docs/src/guides/`.
- **API/Schema**: Extend `ARCHITECTURE.md` data tables and any API reference (`docs/api/` if present) for new fields/endpoints.
- **Web/PDF UX**: Update `web/README.md` and `tools/pdf-generator/README.md` to show new cards/sections.
- **Release notes/comms**: Add `docs/product/alpha-fft-lidar-stop.md` (or release note) and a 2–3 minute demo video script + shot list.

## Critical Dependencies & Risks

- **Data availability**: Need labeled FFT captures covering pedestrians, bikes, and autos across speed ranges; need LiDAR captures of intersections with clear stop-line geometry.
- **Sensor configuration**: OPS243 must run in FFT mode with stable baud/throughput; LiDAR must be calibrated to site coordinates for stop-line checks.
- **Model quality**: Risk of misclassifying small bikes vs pedestrians; need threshold tuning and confidence scoring.
- **Performance on Pi 4**: Ensure FFT parsing/inference fits CPU budget; consider lightweight model or quantization.
- **Privacy constraints**: Maintain PII-free processing; no camera inputs.

## Milestones (suggested)

1. **Week 1–2**: Data capture & labeling (FFT + LiDAR stop-line sessions); define geometry/config schema; draft specs.
2. **Week 3–4**: Implement FFT ingestion + offline classifier training; implement stop detector prototype with recorded data; initial API/schema changes.
3. **Week 5**: Integrate realtime inference + stop metrics into API/DB; surface in dashboard/PDF; add tests/fixtures.
4. **Week 6**: Field validation + tuning; finalize docs; produce demo video; cut alpha release notes.

## Release Readiness Checklist (Alpha)

- [ ] Radar FFT classifier produces ped/auto/bike labels with documented confidence and latency targets.
- [ ] LiDAR stop-compliance tool outputs % complete stops with configurable geometry and thresholds.
- [ ] API, PDF, and dashboard show new metrics.
- [ ] Operator docs updated; sample configs and fixtures published.
- [ ] Validation results recorded (accuracy/false-positive rates) from at least two field sites.
- [ ] Comms package prepared (release notes + demo video).
