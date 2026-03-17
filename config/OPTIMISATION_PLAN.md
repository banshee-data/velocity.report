# Config Value Optimisation Plan

- **Status:** Proposed
- **Related:** [Parameter Tuning Optimisation Plan](../docs/plans/lidar-parameter-tuning-optimisation-plan.md), [Pipeline Review Q7](../data/maths/pipeline-review-open-questions.md)

## Goal

Graduate every tuning parameter from "provisional" to "empirically
validated" by running structured sweeps across multiple sites with
quantified acceptance criteria.

## Current evidence levels

Each config key in `tuning.defaults.json` falls into one of four evidence
categories:

| Level           | Meaning                                                             | Count | Action                                   |
| --------------- | ------------------------------------------------------------------- | ----- | ---------------------------------------- |
| **Theoretical** | Derived from mathematical first principles or sensor specifications | ~5    | No action — document derivation          |
| **Literature**  | Matches published values for the algorithm class                    | ~2    | Verify applicability to our sensor/scene |
| **Empirical**   | Validated on at least one site with measured improvement            | ~3    | Extend to multi-site validation          |
| **Provisional** | Tuned on kirk0 only, no formal comparison                           | ~8    | **Must validate before stable release**  |

### Provisional keys requiring validation

| Config key                      | Default | Risk if wrong                                            | Validation approach                                                  |
| ------------------------------- | ------- | -------------------------------------------------------- | -------------------------------------------------------------------- |
| `closeness_multiplier`          | 3.0     | False foreground/background at range extremes            | Sweep [1.5, 5.0] across 5 sites, measure foreground precision/recall |
| `safety_margin_meters`          | 0.15    | Ground leakage into foreground                           | Sweep [0.05, 0.30], measure ground false-positive rate               |
| `noise_relative`                | 0.02    | Incorrect range-dependent thresholds                     | Measure actual sensor noise vs range curve on each site              |
| `neighbor_confirmation_count`   | 3       | Missed foreground at scene edges                         | Sweep [1, 5], measure edge-case recall                               |
| `foreground_min_cluster_points` | 5       | Missed small objects (cyclists, pedestrians)             | Sweep [2, 8], measure per-class recall                               |
| `process_noise_pos`             | 0.05    | Track instability or sluggish response                   | Sweep [0.01, 0.5], measure track jitter and responsiveness           |
| `process_noise_vel`             | 0.2     | Velocity estimate lag or overshoot                       | Sweep [0.05, 2.0], measure speed accuracy vs ground truth            |
| `measurement_noise`             | 0.05    | Incorrect Kalman gain, over/under-weighting measurements | Derive from sensor spec, validate with innovation consistency check  |

### Theoretical keys (document only)

| Config key                   | Default | Derivation                                                                 |
| ---------------------------- | ------- | -------------------------------------------------------------------------- |
| `background_update_fraction` | 0.02    | EMA α for 50-frame effective window; matches typical settling target       |
| `gating_distance_squared`    | 36.0    | χ²(2) conservative gate (≈6σ); effectively never rejects on distance alone |
| `max_reasonable_speed_mps`   | 30.0    | ~108 km/h; reasonable upper bound for UK residential/urban roads           |

### Literature keys (verify applicability)

| Config key              | Default | Source                                             | Verification                                                 |
| ----------------------- | ------- | -------------------------------------------------- | ------------------------------------------------------------ |
| `foreground_dbscan_eps` | 0.8     | DBSCAN literature, adapted for LiDAR point density | Sweep [0.3, 1.5] across sites with different point densities |

## Validation protocol

### Per-key sweep

1. Define sweep range (see tables above) with ≥ 5 values per key.
2. Run `pcap-analyse` on each of the five corpus PCAPs with each value,
   holding all other keys at defaults.
3. Record quality metrics per run: confirmed track count, fragmentation
   rate, split/merge count, noise track fraction, mean track duration,
   classification accuracy.
4. Compute the optimisation objective (from the parameter tuning plan):
   ```
   objective = 1.0 × confirmed_tracks
             − 5.0 × splits
             − 5.0 × merges
             − 2.0 × noise_tracks
             + 0.1 × mean_duration
   ```
5. Plot objective vs parameter value for each site. The optimal value
   should be within 10% of optimal across all five sites.

### Multi-key interaction

After per-key sweeps identify stable ranges, run a reduced grid search
over the top 3–4 most sensitive keys simultaneously to check for
interactions. Use the same five-PCAP corpus.

### Graduation criteria

A key graduates from "provisional" to "empirical" when:

1. Swept across ≥ 3 sites (target: 5)
2. Optimal value is consistent (within 10% of the chosen default) across
   all sites
3. Objective function sensitivity is documented (slope near the default)
4. Results are recorded in a dated entry under `data/explore/`

## Schedule

| Phase | Work                                        | Depends on                     |
| ----- | ------------------------------------------- | ------------------------------ |
| 1     | Capture PCAPs 2–5 for the test corpus       | Field work                     |
| 2     | Label ≥ 20 tracks per PCAP                  | Manual labelling UI            |
| 3     | Run per-key sweeps for all provisional keys | Phase 2 + sweep infrastructure |
| 4     | Run multi-key interaction grid              | Phase 3                        |
| 5     | Update defaults and document evidence       | Phase 4                        |
| 6     | Graduate keys and update this plan          | Phase 5                        |

## References

- [config/tuning.defaults.json](tuning.defaults.json) — current defaults
- [config/README.maths.md](README.maths.md) — config-to-maths cross-reference
- [Pipeline review Q7](../data/maths/pipeline-review-open-questions.md) — evidence classification
- [Parameter tuning plan](../docs/plans/lidar-parameter-tuning-optimisation-plan.md) — sweep infrastructure
