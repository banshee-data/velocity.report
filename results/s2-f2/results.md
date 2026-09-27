# S2.1 F2: full cluster members on the tuning sites (Mac)

Host: hydra, MacBookPro18,3, Apple M1 Pro, 8 CPUs, 16 GiB, macOS 15.7.7. Branch `claude/upbeat-galileo-4xbaat-s2-1-faces` at `545c2e0a`; cases marina-webster-beach and columbus-broadway, 256 sample points; both arms run in parallel, after F1 finished.

| Arm  | -experiment                                                     | Exit | Wall clock        | Peak RSS |
| ---- | --------------------------------------------------------------- | ---- | ----------------- | -------- |
| fm   | `solid_body,solid_body_full_members`                            | 0    | 1458 s (24.3 min) | 919 MiB  |
| t1fm | `solid_body,solid_body_face_hysteresis,solid_body_full_members` | 0    | 1460 s (24.3 min) | 937 MiB  |

Both arms report `baseline_equal: true` on both cases (repeat run verified). The log line "solid bodies are computed from the medoid only and not written" is the generic warning for `solid_body` without an observation database; `solid_body_full_members` still sets `KeepClusterMembers` (replayeval.go:800), and `experiments` in the summary lists it.

## Table 1: solid-body summary per case and arm

Fallback columns are the `fallbacks` map. BC = `anchor_solid_bodies_body_centre_frames`, FS = `anchor_solid_bodies_face_stable_runs`; gap = BC p99 / FS p99.

| case                 | arm  | solid_bodies | near_edge_fixes | fix_share | body_centre_lapsed | class_prior_extent | face_hysteresis | initialisation_window | no_face_reached_minimum_support | singular_innovation | lapses | tracks_width_converged | steady_runs | face_stable_runs | BC p95_m | BC p99_m | BC max_m | FS p95_m | FS p99_m | FS max_m | gap  |
| -------------------- | ---- | ------------ | --------------- | --------- | ------------------ | ------------------ | --------------- | --------------------- | ------------------------------- | ------------------- | ------ | ---------------------- | ----------- | ---------------- | -------- | -------- | -------- | -------- | -------- | -------- | ---- |
| marina-webster-beach | fm   | 39399        | 14557           | 0.369     | 402                | 4538               | 0               | 450                   | 19098                           | 354                 | 402    | 190                    | 573         | 3604             | 0.060    | 0.231    | 1.926    | 0.034    | 0.073    | 0.570    | 3.15 |
| marina-webster-beach | t1fm | 39399        | 13570           | 0.344     | 587                | 4555               | 1045            | 450                   | 19192                           | 0                   | 587    | 190                    | 730         | 3062             | 0.055    | 0.159    | 1.821    | 0.032    | 0.064    | 0.560    | 2.50 |
| columbus-broadway    | fm   | 77949        | 40630           | 0.521     | 898                | 15337              | 0               | 2264                  | 18818                           | 2                   | 898    | 365                    | 1817        | 7774             | 0.130    | 0.386    | 1.139    | 0.081    | 0.194    | 0.708    | 1.99 |
| columbus-broadway    | t1fm | 77949        | 37130           | 0.476     | 1363               | 15343              | 2791            | 2264                  | 19057                           | 1                   | 1363   | 369                    | 2039        | 7328             | 0.123    | 0.344    | 1.139    | 0.067    | 0.136    | 0.501    | 2.53 |

## Table 2: face-stable strata

Point = point estimates, body = solid bodies, on the face-stable runs.

| case                 | arm  | axis         | bin           | point windows | point p95_m | point p99_m | body windows | body p95_m | body p99_m |
| -------------------- | ---- | ------------ | ------------- | ------------- | ----------- | ----------- | ------------ | ---------- | ---------- |
| marina-webster-beach | fm   | heading_rate | lt_5_deg_s    | 1053          | 0.096       | 0.197       | 1041         | 0.038      | 0.074      |
| marina-webster-beach | fm   | heading_rate | 5_to_15_deg_s | 1617          | 0.092       | 0.143       | 1613         | 0.032      | 0.069      |
| marina-webster-beach | fm   | heading_rate | ge_15_deg_s   | 242           | 0.077       | 0.115       | 242          | 0.034      | 0.204      |
| marina-webster-beach | fm   | heading_rate | unknown       | 0             | 0.000       | 0.000       | 0            | 0.000      | 0.000      |
| marina-webster-beach | fm   | range        | lt_20_m       | 2534          | 0.095       | 0.151       | 2534         | 0.034      | 0.070      |
| marina-webster-beach | fm   | range        | 20_to_40_m    | 351           | 0.084       | 0.183       | 335          | 0.033      | 0.055      |
| marina-webster-beach | fm   | range        | ge_40_m       | 27            | 0.051       | 0.127       | 27           | 0.276      | 0.570      |
| marina-webster-beach | t1fm | heading_rate | lt_5_deg_s    | 933           | 0.090       | 0.192       | 919          | 0.037      | 0.073      |
| marina-webster-beach | t1fm | heading_rate | 5_to_15_deg_s | 1473          | 0.092       | 0.139       | 1472         | 0.028      | 0.052      |
| marina-webster-beach | t1fm | heading_rate | ge_15_deg_s   | 221           | 0.077       | 0.115       | 221          | 0.027      | 0.201      |
| marina-webster-beach | t1fm | heading_rate | unknown       | 0             | 0.000       | 0.000       | 0            | 0.000      | 0.000      |
| marina-webster-beach | t1fm | range        | lt_20_m       | 2259          | 0.092       | 0.150       | 2259         | 0.031      | 0.060      |
| marina-webster-beach | t1fm | range        | 20_to_40_m    | 343           | 0.081       | 0.170       | 328          | 0.034      | 0.058      |
| marina-webster-beach | t1fm | range        | ge_40_m       | 25            | 0.051       | 0.127       | 25           | 0.270      | 0.560      |
| columbus-broadway    | fm   | heading_rate | lt_5_deg_s    | 944           | 0.096       | 0.164       | 985          | 0.076      | 0.168      |
| columbus-broadway    | fm   | heading_rate | 5_to_15_deg_s | 1089          | 0.099       | 0.212       | 1088         | 0.065      | 0.138      |
| columbus-broadway    | fm   | heading_rate | ge_15_deg_s   | 475           | 0.095       | 0.152       | 490          | 0.130      | 0.369      |
| columbus-broadway    | fm   | heading_rate | unknown       | 0             | 0.000       | 0.000       | 0            | 0.000      | 0.000      |
| columbus-broadway    | fm   | range        | lt_20_m       | 2215          | 0.097       | 0.174       | 2269         | 0.085      | 0.195      |
| columbus-broadway    | fm   | range        | 20_to_40_m    | 288           | 0.096       | 0.197       | 289          | 0.056      | 0.190      |
| columbus-broadway    | fm   | range        | ge_40_m       | 5             | 0.035       | 0.035       | 5            | 0.012      | 0.012      |
| columbus-broadway    | t1fm | heading_rate | lt_5_deg_s    | 793           | 0.089       | 0.146       | 827          | 0.073      | 0.145      |
| columbus-broadway    | t1fm | heading_rate | 5_to_15_deg_s | 960           | 0.091       | 0.172       | 956          | 0.056      | 0.099      |
| columbus-broadway    | t1fm | heading_rate | ge_15_deg_s   | 398           | 0.090       | 0.151       | 411          | 0.094      | 0.238      |
| columbus-broadway    | t1fm | heading_rate | unknown       | 0             | 0.000       | 0.000       | 0            | 0.000      | 0.000      |
| columbus-broadway    | t1fm | range        | lt_20_m       | 1875          | 0.091       | 0.164       | 1917         | 0.067      | 0.135      |
| columbus-broadway    | t1fm | range        | 20_to_40_m    | 271           | 0.085       | 0.170       | 272          | 0.062      | 0.193      |
| columbus-broadway    | t1fm | range        | ge_40_m       | 5             | 0.035       | 0.035       | 5            | 0.013      | 0.013      |
