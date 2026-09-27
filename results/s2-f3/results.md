# S2.1 F3: course-aligned faces on the tuning sites (Mac)

Host: hydra, MacBookPro18,3, Apple M1 Pro, 8 CPUs, 16 GiB, macOS 15.7.7. Branch `claude/upbeat-galileo-4xbaat-s2-1-t3` at `0adb33e5`; cases marina-webster-beach and columbus-broadway, 256 sample points; both arms run in parallel.

| Arm    | -experiment                                                                             | Exit | Wall clock        | Peak RSS |
| ------ | --------------------------------------------------------------------------------------- | ---- | ----------------- | -------- |
| t3fm   | `solid_body,solid_body_course_faces,solid_body_full_members`                            | 0    | 1388 s (23.1 min) | 927 MiB  |
| t1t3fm | `solid_body,solid_body_face_hysteresis,solid_body_course_faces,solid_body_full_members` | 0    | 1389 s (23.1 min) | 962 MiB  |

`baseline_equal` per arm across cases: t3fm: True; t1t3fm: True. Experiments listed in the summary: t3fm: solid_body, solid_body_course_faces, solid_body_full_members; t1t3fm: solid_body, solid_body_course_faces, solid_body_face_hysteresis, solid_body_full_members. The log line "solid bodies are computed from the medoid only and not written" is the generic warning for `solid_body` without an observation database.

## Table 1: solid-body summary per case and arm

Fallback columns are the `fallbacks` map. BC = `anchor_solid_bodies_body_centre_frames`, FS = `anchor_solid_bodies_face_stable_runs`; point BC/FS = the matching `anchor_point_estimates_*` p99; gap = BC p99 / FS p99.

| case                 | arm    | solid_bodies | near_edge_fixes | fix_share | body_centre_lapsed | class_prior_extent | face_hysteresis | initialisation_window | no_face_reached_minimum_support | singular_innovation | lapses | tracks_width_converged | steady_runs | face_stable_runs | BC p95_m | BC p99_m | BC max_m | BC excursion_share | FS p95_m | FS p99_m | FS max_m | point BC p99_m | point FS p99_m | gap  |
| -------------------- | ------ | ------------ | --------------- | --------- | ------------------ | ------------------ | --------------- | --------------------- | ------------------------------- | ------------------- | ------ | ---------------------- | ----------- | ---------------- | -------- | -------- | -------- | ------------------ | -------- | -------- | -------- | -------------- | -------------- | ---- |
| marina-webster-beach | t3fm   | 39399        | 18351           | 0.466     | 514                | 1494               | 0               | 450                   | 18588                           | 2                   | 514    | 239                    | 719         | 4743             | 0.050    | 0.182    | 1.954    | 0.054              | 0.032    | 0.069    | 0.986    | 0.198          | 0.157          | 2.63 |
| marina-webster-beach | t1t3fm | 39399        | 16785           | 0.426     | 674                | 1500               | 1226            | 450                   | 18764                           | 0                   | 674    | 238                    | 840         | 3797             | 0.046    | 0.154    | 1.771    | 0.043              | 0.030    | 0.065    | 0.662    | 0.183          | 0.149          | 2.36 |
| columbus-broadway    | t3fm   | 77949        | 47664           | 0.611     | 1039               | 8857               | 0               | 2264                  | 18123                           | 2                   | 1039   | 482                    | 2230        | 9563             | 0.127    | 0.401    | 1.315    | 0.054              | 0.066    | 0.164    | 1.126    | 0.270          | 0.198          | 2.44 |
| columbus-broadway    | t1t3fm | 77949        | 43294           | 0.555     | 1680               | 8857               | 3505            | 2264                  | 18348                           | 1                   | 1680   | 488                    | 2567        | 8978             | 0.106    | 0.287    | 1.141    | 0.037              | 0.059    | 0.139    | 0.644    | 0.228          | 0.193          | 2.06 |

## Table 2: face-stable strata

Point = point estimates, body = solid bodies, on the face-stable runs.

| case                 | arm    | axis         | bin           | point windows | point p95_m | point p99_m | body windows | body p95_m | body p99_m |
| -------------------- | ------ | ------------ | ------------- | ------------- | ----------- | ----------- | ------------ | ---------- | ---------- |
| marina-webster-beach | t3fm   | heading_rate | lt_5_deg_s    | 1392          | 0.090       | 0.202       | 1384         | 0.033      | 0.069      |
| marina-webster-beach | t3fm   | heading_rate | 5_to_15_deg_s | 2059          | 0.095       | 0.149       | 2060         | 0.029      | 0.069      |
| marina-webster-beach | t3fm   | heading_rate | ge_15_deg_s   | 936           | 0.086       | 0.141       | 935          | 0.037      | 0.069      |
| marina-webster-beach | t3fm   | heading_rate | unknown       | 0             | 0.000       | 0.000       | 0            | 0.000      | 0.000      |
| marina-webster-beach | t3fm   | range        | lt_20_m       | 3859          | 0.092       | 0.151       | 3855         | 0.031      | 0.064      |
| marina-webster-beach | t3fm   | range        | 20_to_40_m    | 470           | 0.090       | 0.183       | 466          | 0.037      | 0.102      |
| marina-webster-beach | t3fm   | range        | ge_40_m       | 58            | 0.051       | 0.127       | 58           | 0.002      | 0.010      |
| marina-webster-beach | t1t3fm | heading_rate | lt_5_deg_s    | 1299          | 0.088       | 0.180       | 1285         | 0.030      | 0.059      |
| marina-webster-beach | t1t3fm | heading_rate | 5_to_15_deg_s | 1902          | 0.092       | 0.147       | 1896         | 0.029      | 0.065      |
| marina-webster-beach | t1t3fm | heading_rate | ge_15_deg_s   | 800           | 0.079       | 0.131       | 801          | 0.034      | 0.109      |
| marina-webster-beach | t1t3fm | heading_rate | unknown       | 0             | 0.000       | 0.000       | 0            | 0.000      | 0.000      |
| marina-webster-beach | t1t3fm | range        | lt_20_m       | 3495          | 0.089       | 0.146       | 3492         | 0.029      | 0.060      |
| marina-webster-beach | t1t3fm | range        | 20_to_40_m    | 450           | 0.089       | 0.182       | 434          | 0.037      | 0.100      |
| marina-webster-beach | t1t3fm | range        | ge_40_m       | 56            | 0.051       | 0.127       | 56           | 0.002      | 0.010      |
| columbus-broadway    | t3fm   | heading_rate | lt_5_deg_s    | 1508          | 0.093       | 0.184       | 1546         | 0.072      | 0.181      |
| columbus-broadway    | t3fm   | heading_rate | 5_to_15_deg_s | 1717          | 0.107       | 0.208       | 1709         | 0.056      | 0.156      |
| columbus-broadway    | t3fm   | heading_rate | ge_15_deg_s   | 1568          | 0.095       | 0.193       | 1574         | 0.065      | 0.143      |
| columbus-broadway    | t3fm   | heading_rate | unknown       | 0             | 0.000       | 0.000       | 0            | 0.000      | 0.000      |
| columbus-broadway    | t3fm   | range        | lt_20_m       | 4115          | 0.100       | 0.202       | 4149         | 0.071      | 0.169      |
| columbus-broadway    | t3fm   | range        | 20_to_40_m    | 626           | 0.090       | 0.173       | 628          | 0.024      | 0.079      |
| columbus-broadway    | t3fm   | range        | ge_40_m       | 52            | 0.046       | 0.081       | 52           | 0.001      | 0.016      |
| columbus-broadway    | t1t3fm | heading_rate | lt_5_deg_s    | 1322          | 0.090       | 0.184       | 1362         | 0.065      | 0.144      |
| columbus-broadway    | t1t3fm | heading_rate | 5_to_15_deg_s | 1523          | 0.099       | 0.206       | 1516         | 0.054      | 0.127      |
| columbus-broadway    | t1t3fm | heading_rate | ge_15_deg_s   | 1323          | 0.092       | 0.178       | 1328         | 0.060      | 0.136      |
| columbus-broadway    | t1t3fm | heading_rate | unknown       | 0             | 0.000       | 0.000       | 0            | 0.000      | 0.000      |
| columbus-broadway    | t1t3fm | range        | lt_20_m       | 3527          | 0.097       | 0.194       | 3563         | 0.064      | 0.143      |
| columbus-broadway    | t1t3fm | range        | 20_to_40_m    | 599           | 0.086       | 0.173       | 601          | 0.024      | 0.083      |
| columbus-broadway    | t1t3fm | range        | ge_40_m       | 42            | 0.039       | 0.052       | 42           | 0.001      | 0.019      |
