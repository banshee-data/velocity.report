# Test corpus

Five-PCAP test corpus using the Hesai P40 sensor, covering enough road geometry, traffic class, and scene diversity to validate tuning defaults and detect overfitting to a single site.

## Source

- Plan: [docs/plans/lidar-test-corpus-plan.md](../../plans/lidar-test-corpus-plan.md)
- Status: Proposed

## Problem

All provisional config defaults were tuned on kirk0: a single capture at one site. The overfitting risk is real: kirk0 may be flat (sloped-road defaults untested), may over-represent one vehicle class, may lack kerbs/junctions/long-range views, and one capture cannot cover wet/dry/wind conditions.

## Corpus specification

All captures use the **Hesai P40** sensor. Each site needs ≥ 20 manually labelled tracks covering car, truck, cyclist, and pedestrian at minimum.

| #   | Name      | Site Description                          | Validates                                           | Duration | Status     |
| --- | --------- | ----------------------------------------- | --------------------------------------------------- | -------- | ---------- |
| 1   | kirk0     | Flat urban road                           | Baseline defaults, straight-line vehicles           | ~5 min   | ✓ Captured |
| 2   | slope1    | Sloped residential street (≥ 3° gradient) | Ground-plane tiling, height-band limits             | ~5 min   | Planned    |
| 3   | school1   | School zone or park entrance              | Pedestrian/cyclist classification, low-speed tracks | ~10 min  | Planned    |
| 4   | junction1 | Multi-lane road or junction               | Turning vehicles, lane-crossing, merge/split        | ~10 min  | Planned    |
| 5   | rural1    | Rural or semi-rural road                  | Long-range sparse clusters, high-speed vehicles     | ~5 min   | Planned    |

### Site selection criteria

- Each site should exercise a different failure mode of the current pipeline
- At least two sites with visible kerbs for ground-plane validation
- At least one site with pedestrians and cyclists for classification validation
- At least one site with vehicles at > 20 m/s for high-speed tracking validation

### Capture requirements

- Sensor: Hesai P40 at 10 Hz
- Duration: ≥ 5 minutes continuous traffic (≥ 3,000 frames)
- Format: PCAP-NG (`.pcapng`)
- Storage: Git LFS under [internal/lidar/perf/pcap/](../../../internal/lidar/perf/pcap)
- GPS: Record GPS fix alongside capture (for geo-referencing, not pipeline processing)

### Labelling requirements

Per PCAP, create a labelled reference analysis run: ≥ 20 vehicle tracks, ≥ 5 cyclist tracks, ≥ 5 pedestrian tracks (where present). Labels stored via track-labelling UI. Scene `reference_run_id` set for `GroundTruthEvaluator` comparison.

## Usage

**Parameter sweep validation:** Config optimisation sweeps each provisional key across all five PCAPs simultaneously. A default is promoted from "provisional" to "empirical" only when it performs within 10% of optimal across all five sites.

**Regression testing:** `make test-perf` on each PCAP. Per-site and aggregate metrics reported. Regressions on any site block release.

**Algorithm comparison:** Experiment proposals run both configurations on all five PCAPs and report per-site and aggregate results.

## Schedule

| Phase | Work                                     | Depends on               |
| ----- | ---------------------------------------- | ------------------------ |
| 1     | Identify and confirm 4 new capture sites | Site access              |
| 2     | Capture PCAPs 2–5 with P40 sensor        | Phase 1 + hardware       |
| 3     | Label ≥ 20 tracks per PCAP               | Phase 2 + labelling UI   |
| 4     | Create baselines for each PCAP           | Phase 3 + `pcap-analyse` |
| 5     | Integrate into CI nightly run            | Phase 4                  |
| 6     | Run parameter sweeps across full corpus  | Phase 5                  |

## Non-Goals

- Multi-sensor corpus (different LiDAR models): deferred until single-sensor defaults validated
- Weather variation within initial corpus: one clear-weather capture per site
- Synthetic PCAPs: all captures must be real-world data
