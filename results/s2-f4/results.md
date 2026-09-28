# S2.1 F4: held-out score of T1+T3 (Mac)

Host: hydra, MacBookPro18,3, Apple M1 Pro, 8 CPUs, 16 GiB, macOS 15.7.7. Branch `claude/upbeat-galileo-4xbaat-s2-1-t3` at `f5d1b0c8`; `-experiment solid_body,solid_body_face_hysteresis,solid_body_course_faces,solid_body_full_members`, 256 sample points; both arms run in parallel.

| Arm     | Cases                                  | Exit | Wall clock        | Peak RSS |
| ------- | -------------------------------------- | ---- | ----------------- | -------- |
| heldout | embarcadero-folsom                     | 0    | 694 s (11.6 min)  | 910 MiB  |
| tuning  | marina-webster-beach,columbus-broadway | 0    | 1611 s (26.9 min) | 908 MiB  |

`baseline_equal` per case: embarcadero-folsom: True; marina-webster-beach: True; columbus-broadway: True. Experiments listed in the summary: solid_body, solid_body_course_faces, solid_body_face_hysteresis, solid_body_full_members.

## Table 1: solid-body summary per case

Fallback columns are the `fallbacks` map. Body BC = `anchor_solid_bodies_body_centre_frames`, body FS = `anchor_solid_bodies_face_stable_runs`; point BC / FS / all = the matching `anchor_point_estimates_*`. gap = body BC p99 / body FS p99; ratio = body BC p99 / point BC p99.

| case                 | arm     | solid_bodies | near_edge_fixes | fix_share | body_centre_lapsed | class_prior_extent | face_hysteresis | initialisation_window | no_face_reached_minimum_support | singular_innovation | lapses | tracks_width_converged | steady_runs | face_stable_runs | body BC p95_m | body BC p99_m | body BC max_m | body BC excursion_share | body FS p95_m | body FS p99_m | body FS max_m | point BC p95_m | point BC p99_m | point BC excursion_share | point FS p99_m | point all p99_m | point all excursion_share | gap  | ratio |
| -------------------- | ------- | ------------ | --------------- | --------- | ------------------ | ------------------ | --------------- | --------------------- | ------------------------------- | ------------------- | ------ | ---------------------- | ----------- | ---------------- | ------------- | ------------- | ------------- | ----------------------- | ------------- | ------------- | ------------- | -------------- | -------------- | ------------------------ | -------------- | --------------- | ------------------------- | ---- | ----- |
| embarcadero-folsom   | heldout | 42953        | 24886           | 0.579     | 923                | 8070               | 1692            | 842                   | 6540                            | 0                   | 923    | 307                    | 1213        | 4847             | 0.083         | 0.229         | 1.100         | 0.026                   | 0.059         | 0.118         | 1.002         | 0.114          | 0.204          | 0.004                    | 0.181          | 0.275           | 0.032                     | 1.94 | 1.12  |
| marina-webster-beach | tuning  | 39399        | 16722           | 0.424     | 676                | 1491               | 1295            | 450                   | 18765                           | 0                   | 676    | 238                    | 841         | 3919             | 0.048         | 0.158         | 1.771         | 0.043                   | 0.029         | 0.065         | 0.662         | 0.097          | 0.183          | 0.022                    | 0.150          | 0.227           | 0.056                     | 2.41 | 0.86  |
| columbus-broadway    | tuning  | 77949        | 43106           | 0.553     | 1695               | 8821               | 3713            | 2264                  | 18349                           | 1                   | 1695   | 488                    | 2577        | 9268             | 0.107         | 0.280         | 1.141         | 0.040                   | 0.060         | 0.141         | 0.644         | 0.109          | 0.226          | 0.023                    | 0.188          | 0.287           | 0.037                     | 1.99 | 1.24  |

## Table 2: face-stable strata

Point = point estimates, body = solid bodies, on the face-stable runs.

| case                 | axis         | bin           | point windows | point p95_m | point p99_m | body windows | body p95_m | body p99_m |
| -------------------- | ------------ | ------------- | ------------- | ----------- | ----------- | ------------ | ---------- | ---------- |
| embarcadero-folsom   | heading_rate | lt_5_deg_s    | 769           | 0.102       | 0.160       | 761          | 0.060      | 0.140      |
| embarcadero-folsom   | heading_rate | 5_to_15_deg_s | 1133          | 0.126       | 0.238       | 1128         | 0.061      | 0.112      |
| embarcadero-folsom   | heading_rate | ge_15_deg_s   | 731           | 0.104       | 0.155       | 732          | 0.048      | 0.098      |
| embarcadero-folsom   | heading_rate | unknown       | 0             | 0.000       | 0.000       | 0            | 0.000      | 0.000      |
| embarcadero-folsom   | range        | lt_20_m       | 2327          | 0.109       | 0.177       | 2315         | 0.060      | 0.119      |
| embarcadero-folsom   | range        | 20_to_40_m    | 283           | 0.130       | 0.205       | 283          | 0.047      | 0.083      |
| embarcadero-folsom   | range        | ge_40_m       | 23            | 0.108       | 0.127       | 23           | 0.000      | 0.000      |
| marina-webster-beach | heading_rate | lt_5_deg_s    | 1254          | 0.088       | 0.172       | 1240         | 0.030      | 0.065      |
| marina-webster-beach | heading_rate | 5_to_15_deg_s | 1899          | 0.092       | 0.149       | 1893         | 0.027      | 0.065      |
| marina-webster-beach | heading_rate | ge_15_deg_s   | 637           | 0.079       | 0.137       | 638          | 0.035      | 0.086      |
| marina-webster-beach | heading_rate | unknown       | 0             | 0.000       | 0.000       | 0            | 0.000      | 0.000      |
| marina-webster-beach | range        | lt_20_m       | 3284          | 0.089       | 0.147       | 3281         | 0.028      | 0.058      |
| marina-webster-beach | range        | 20_to_40_m    | 452           | 0.089       | 0.182       | 436          | 0.044      | 0.112      |
| marina-webster-beach | range        | ge_40_m       | 54            | 0.051       | 0.127       | 54           | 0.002      | 0.010      |
| columbus-broadway    | heading_rate | lt_5_deg_s    | 1287          | 0.088       | 0.184       | 1324         | 0.066      | 0.144      |
| columbus-broadway    | heading_rate | 5_to_15_deg_s | 1509          | 0.100       | 0.206       | 1504         | 0.054      | 0.140      |
| columbus-broadway    | heading_rate | ge_15_deg_s   | 1175          | 0.094       | 0.178       | 1183         | 0.057      | 0.137      |
| columbus-broadway    | heading_rate | unknown       | 0             | 0.000       | 0.000       | 0            | 0.000      | 0.000      |
| columbus-broadway    | range        | lt_20_m       | 3356          | 0.098       | 0.194       | 3394         | 0.064      | 0.145      |
| columbus-broadway    | range        | 20_to_40_m    | 573           | 0.083       | 0.170       | 575          | 0.023      | 0.097      |
| columbus-broadway    | range        | ge_40_m       | 42            | 0.039       | 0.052       | 42           | 0.001      | 0.020      |
