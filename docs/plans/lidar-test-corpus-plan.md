# LiDAR Test Corpus Plan

- **Status:** Proposed
- **Layers:** Cross-cutting
- **Related:** [Pipeline Review Q11](../data/maths/pipeline-review-open-questions.md), [Config Optimisation Plan](../config/OPTIMISATION_PLAN.md)

## Goal

Build a five-PCAP test corpus using the Hesai P40 sensor that covers
enough road geometry, traffic class, and scene diversity to validate
tuning defaults and detect overfitting to a single site.

## Problem

All provisional config defaults were tuned on kirk0 — a single capture
at one site. The overfitting risk is real:

- Road geometry: kirk0 may be flat; sloped-road defaults are untested.
- Traffic mix: kirk0 may over-represent one vehicle class.
- Scene geometry: kirk0 may lack kerbs, junctions, or long-range views.
- Weather/lighting: one capture cannot cover wet/dry/wind conditions.

## Corpus specification

All captures use the **Hesai P40** sensor to control for sensor-specific
noise characteristics. Each site needs ≥ 20 manually labelled tracks
covering the major classes (car, truck, cyclist, pedestrian at minimum).

| # | Name | Site description | Validates | Duration | Status |
| - | --- | --- | --- | --- | --- |
| 1 | kirk0 | Flat urban road | Baseline defaults, straight-line vehicles | ~5 min | ✓ Captured |
| 2 | slope1 | Sloped residential street (≥ 3° gradient) | Ground-plane tiling, height-band limits | ~5 min | Planned |
| 3 | school1 | School zone or park entrance | Pedestrian/cyclist classification, low-speed tracks | ~10 min | Planned |
| 4 | junction1 | Multi-lane road or junction | Turning vehicles, lane-crossing, merge/split | ~10 min | Planned |
| 5 | rural1 | Rural or semi-rural road | Long-range sparse clusters, high-speed vehicles | ~5 min | Planned |

### Site selection criteria

- Each site should exercise a different failure mode of the current
  pipeline (see pipeline review Q1, Q5, Q11).
- At least two sites should have visible kerbs for ground-plane
  validation.
- At least one site should have pedestrians and cyclists for
  classification validation.
- At least one site should have vehicles at > 20 m/s for high-speed
  tracking validation.

### Capture requirements

- Sensor: Hesai P40 at 10 Hz
- Duration: ≥ 5 minutes of continuous traffic (≥ 3,000 frames)
- Format: PCAP-NG (`.pcapng`)
- Storage: Git LFS under `internal/lidar/perf/pcap/`
- GPS: Record GPS fix alongside capture (for geo-referencing, not for
  pipeline processing)

### Labelling requirements

Per PCAP, manually label:

- ≥ 20 vehicle tracks (car and/or truck)
- ≥ 5 cyclist tracks (where present)
- ≥ 5 pedestrian tracks (where present)
- Label format: compatible with `analysis.CompareReports` (track ID,
  class, start/end time, bounding box sequence)

## Usage

### Parameter sweep validation

The config optimisation plan sweeps each provisional key across all five
PCAPs simultaneously. A default value is only promoted from "provisional"
to "empirical" when it performs within 10% of optimal across all five
sites.

### Regression testing

The performance measurement harness runs `make test-perf` on each PCAP
in the corpus. Per-site and aggregate metrics are reported. Regressions
on any site block the release.

### Algorithm comparison

Experiment proposals (e.g. velocity-coherent vs baseline) run both
configurations on all five PCAPs and report per-site and aggregate
results.

## Schedule

| Phase | Work | Depends on |
| --- | --- | --- |
| 1 | Identify and confirm 4 new capture sites | Site access |
| 2 | Capture PCAPs 2–5 with P40 sensor | Phase 1 + hardware |
| 3 | Label ≥ 20 tracks per PCAP | Phase 2 + labelling UI |
| 4 | Create baselines for each PCAP | Phase 3 + `pcap-analyse` |
| 5 | Integrate into CI nightly run | Phase 4 |
| 6 | Run parameter sweeps across full corpus | Phase 5 |

## Non-goals

- Multi-sensor corpus (different LiDAR models) — deferred until
  single-sensor defaults are validated
- Weather variation within the initial corpus — one clear-weather
  capture per site; weather studies are a future extension
- Synthetic PCAPs — all captures must be real-world data

## References

- [Pipeline review Q11](../data/maths/pipeline-review-open-questions.md) — overfitting analysis
- [Config optimisation plan](../config/OPTIMISATION_PLAN.md) — sweep methodology
- [Performance harness plan](lidar-performance-measurement-harness-plan.md) — CI integration
- [Parameter tuning plan](lidar-parameter-tuning-optimisation-plan.md) — sweep infrastructure
