# S2 T0 Mac results: solid_body shadow, 256 and 1024 sample points

Source: `claude/upbeat-galileo-4xbaat-s2-near-edge` at `d4c28200732242993d922513e983c379ec01cd12`.
Cases: marina-webster-beach, columbus-broadway, embarcadero-folsom. Command: `lidar-state-estimation-baseline -experiment solid_body -sample-points {256,1024} -evidence-per-case -discard-evidence`.

## Host

- `uname -a`: `Darwin hydra.dove-harmonic.ts.net 24.6.0 Darwin Kernel Version 24.6.0: Tue Apr 21 20:19:01 PDT 2026; root:xnu-11417.140.69.710.16~1/RELEASE_ARM64_T6000 arm64`
- CPU: Apple M1 Pro
- hw.memsize: 17179869184 bytes

## Run cost

| arm  | exit | wall-clock (s) | peak RSS (bytes) | peak memory footprint (bytes) |
| ---- | ---- | -------------- | ---------------- | ----------------------------- |
| 256  | 0    | 1513.10        | 965984256        | 1093848576                    |
| 1024 | 0    | 1584.92        | 961544192        | 1093275264                    |

## Solid body counts

| case                 | arm  | solid_bodies | near_edge_fixes | fix_share | lapses | fallbacks                                                                                                                                  | tracks | tracks_width_converged | steady_runs | face_stable_runs |
| -------------------- | ---- | ------------ | --------------- | --------- | ------ | ------------------------------------------------------------------------------------------------------------------------------------------ | ------ | ---------------------- | ----------- | ---------------- |
| marina-webster-beach | 256  | 39399        | 14557           | 0.369     | 402    | body_centre_lapsed=402, class_prior_extent=4538, initialisation_window=450, no_face_reached_minimum_support=19098, singular_innovation=354 | 559    | 190                    | 573         | 3604             |
| marina-webster-beach | 1024 | 39399        | 14557           | 0.369     | 402    | body_centre_lapsed=402, class_prior_extent=4538, initialisation_window=450, no_face_reached_minimum_support=19098, singular_innovation=354 | 559    | 190                    | 573         | 3604             |
| columbus-broadway    | 256  | 77949        | 40630           | 0.521     | 898    | body_centre_lapsed=898, class_prior_extent=15337, initialisation_window=2264, no_face_reached_minimum_support=18818, singular_innovation=2 | 2671   | 365                    | 1817        | 7774             |
| columbus-broadway    | 1024 | 77949        | 40630           | 0.521     | 898    | body_centre_lapsed=898, class_prior_extent=15337, initialisation_window=2264, no_face_reached_minimum_support=18818, singular_innovation=2 | 2671   | 365                    | 1817        | 7774             |
| embarcadero-folsom   | 256  | 42953        | 23663           | 0.551     | 495    | body_centre_lapsed=495, class_prior_extent=11149, initialisation_window=842, no_face_reached_minimum_support=6804                          | 946    | 210                    | 776         | 4185             |
| embarcadero-folsom   | 1024 | 42953        | 23663           | 0.551     | 495    | body_centre_lapsed=495, class_prior_extent=11149, initialisation_window=842, no_face_reached_minimum_support=6804                          | 946    | 211                    | 776         | 4185             |

## Anchor residuals (metres)

### anchor_point_estimates_all

| case                 | arm  | windows | p95_m | p99_m | max_m | excursion_share |
| -------------------- | ---- | ------- | ----- | ----- | ----- | --------------- |
| marina-webster-beach | 256  | 7199    | 0.112 | 0.227 | 1.047 | 0.056           |
| marina-webster-beach | 1024 | 7199    | 0.112 | 0.227 | 1.047 | 0.056           |
| columbus-broadway    | 256  | 9996    | 0.132 | 0.287 | 1.174 | 0.037           |
| columbus-broadway    | 1024 | 9996    | 0.132 | 0.287 | 1.174 | 0.037           |
| embarcadero-folsom   | 256  | 5671    | 0.133 | 0.275 | 1.097 | 0.032           |
| embarcadero-folsom   | 1024 | 5671    | 0.133 | 0.275 | 1.097 | 0.032           |

### anchor_point_estimates_body_centre_frames

| case                 | arm  | windows | p95_m | p99_m | max_m | excursion_share |
| -------------------- | ---- | ------- | ----- | ----- | ----- | --------------- |
| marina-webster-beach | 256  | 3989    | 0.101 | 0.194 | 0.776 | 0.018           |
| marina-webster-beach | 1024 | 3989    | 0.101 | 0.194 | 0.776 | 0.018           |
| columbus-broadway    | 256  | 3993    | 0.120 | 0.254 | 1.174 | 0.017           |
| columbus-broadway    | 1024 | 3993    | 0.120 | 0.254 | 1.174 | 0.017           |
| embarcadero-folsom   | 256  | 2470    | 0.137 | 0.245 | 1.097 | 0.012           |
| embarcadero-folsom   | 1024 | 2470    | 0.137 | 0.245 | 1.097 | 0.012           |

### anchor_solid_bodies_body_centre_frames

| case                 | arm  | windows | p95_m | p99_m | max_m | excursion_share |
| -------------------- | ---- | ------- | ----- | ----- | ----- | --------------- |
| marina-webster-beach | 256  | 3984    | 0.061 | 0.231 | 1.882 | 0.036           |
| marina-webster-beach | 1024 | 3984    | 0.061 | 0.231 | 1.932 | 0.036           |
| columbus-broadway    | 256  | 4077    | 0.130 | 0.388 | 1.137 | 0.043           |
| columbus-broadway    | 1024 | 4092    | 0.130 | 0.386 | 1.139 | 0.043           |
| embarcadero-folsom   | 256  | 2464    | 0.127 | 0.291 | 1.519 | 0.042           |
| embarcadero-folsom   | 1024 | 2464    | 0.128 | 0.291 | 1.517 | 0.042           |

### anchor_point_estimates_face_stable_runs

| case                 | arm  | windows | p95_m | p99_m | max_m | excursion_share |
| -------------------- | ---- | ------- | ----- | ----- | ----- | --------------- |
| marina-webster-beach | 256  | 2912    | 0.091 | 0.153 | 0.497 | 0               |
| marina-webster-beach | 1024 | 2912    | 0.091 | 0.153 | 0.497 | 0               |
| columbus-broadway    | 256  | 2508    | 0.096 | 0.178 | 0.517 | 0.003           |
| columbus-broadway    | 1024 | 2508    | 0.096 | 0.178 | 0.517 | 0.003           |
| embarcadero-folsom   | 256  | 1697    | 0.126 | 0.210 | 0.445 | 0               |
| embarcadero-folsom   | 1024 | 1697    | 0.126 | 0.210 | 0.445 | 0               |

### anchor_solid_bodies_face_stable_runs

| case                 | arm  | windows | p95_m | p99_m | max_m | excursion_share |
| -------------------- | ---- | ------- | ----- | ----- | ----- | --------------- |
| marina-webster-beach | 256  | 2896    | 0.036 | 0.074 | 0.570 | 0.004           |
| marina-webster-beach | 1024 | 2896    | 0.034 | 0.074 | 0.570 | 0.004           |
| columbus-broadway    | 256  | 2550    | 0.082 | 0.193 | 0.711 | 0.003           |
| columbus-broadway    | 1024 | 2561    | 0.081 | 0.194 | 0.708 | 0.003           |
| embarcadero-folsom   | 256  | 1690    | 0.079 | 0.182 | 0.706 | 0.005           |
| embarcadero-folsom   | 1024 | 1690    | 0.079 | 0.175 | 0.692 | 0.005           |

## Body versus point p99 (metres)

| case                 | arm  | face-stable: point | face-stable: body | body-centre frames: point | body-centre frames: body |
| -------------------- | ---- | ------------------ | ----------------- | ------------------------- | ------------------------ |
| marina-webster-beach | 256  | 0.153              | 0.074             | 0.194                     | 0.231                    |
| marina-webster-beach | 1024 | 0.153              | 0.074             | 0.194                     | 0.231                    |
| columbus-broadway    | 256  | 0.178              | 0.193             | 0.254                     | 0.388                    |
| columbus-broadway    | 1024 | 0.178              | 0.194             | 0.254                     | 0.386                    |
| embarcadero-folsom   | 256  | 0.210              | 0.182             | 0.245                     | 0.291                    |
| embarcadero-folsom   | 1024 | 0.210              | 0.175             | 0.245                     | 0.291                    |

## Notes

- The repeat run logs `experiment solid_body without an observation database: solid bodies are computed from the medoid only and not written`; evidence databases were discarded per case (`-discard-evidence`).
- Between arms, only the `anchor_solid_bodies_*` residuals, `retained_sample_points` and one `tracks_width_converged` count (embarcadero-folsom 210 → 211) differ. Point-estimate residuals, solid-body and near-edge-fix counts, lapses, fallbacks, run counts and baselines are identical.
- Both arms exited 0 and wrote 3 repeat-verified baselines each. Free disk on `$HOME`: 23 GiB after arm 256, 22 GiB after arm 1024.
