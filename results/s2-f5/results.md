# S2.1 F5: T4 half-extent state on the tuning partition (Mac)

Host: hydra, MacBookPro18,3, Apple M1 Pro, 8 CPUs, 16 GiB, macOS 15.7.7.

Source: `claude/upbeat-galileo-4xbaat-s2-1-t4` at `040aa247`. Cases marina-webster-beach and columbus-broadway, 256 sample points, arms run one at a time.

Capture root: `/Volumes/banshee-captures/lidar` (NAS share `banshee-captures` on `arrow`, read in place over SMB; file sizes checked against the source manifest), `-pcap-subdir s2`. Work directory: `/Volumes/lidar/evidence/s2-f5c`; per-case evidence databases were discarded after each summary (`-discard-evidence`).

| Arm    | -experiment                                                                                                          | Exit | Wall clock          | Peak RSS |
| ------ | -------------------------------------------------------------------------------------------------------------------- | ---- | ------------------- | -------- |
| t1t3   | `solid_body,solid_body_face_hysteresis,solid_body_course_faces,solid_body_full_members`                              | 0    | 22804 s (380.1 min) | 1003 MiB |
| t1t3t4 | `solid_body,solid_body_face_hysteresis,solid_body_course_faces,solid_body_full_members,solid_body_half_extent_state` | 0    | 10573 s (176.2 min) | 938 MiB  |

The t1t3 wall clock is not a throughput figure. For about an hour of its first case, the host's swap was nearly exhausted by other applications and the NAS was intermittently unreachable, so the replay spent that hour paging rather than computing. CPU time is the fair comparison: user time was 1716 s for t1t3 and 1702 s for t1t3t4.

`baseline_equal` per case and arm: marina-webster-beach/t1t3: True; marina-webster-beach/t1t3t4: True; columbus-broadway/t1t3: True; columbus-broadway/t1t3t4: True.

## Table 1: solid-body summary per case and arm

Fallback columns are the `fallbacks` map. Body BC = `anchor_solid_bodies_body_centre_frames`, body steady = `anchor_solid_bodies_steady_runs`, body FS = `anchor_solid_bodies_face_stable_runs`; point BC / FS = the matching `anchor_point_estimates_*`. gap = body BC p99 / body FS p99; ratio = body BC p99 / point BC p99.

| case                 | arm    | solid_bodies | near_edge_fixes | fix_share | fb:body_centre_lapsed | fb:class_prior_extent | fb:face_hysteresis | fb:initialisation_window | fb:no_face_reached_minimum_support | fb:singular_innovation | lapses | tracks_width_converged | steady_runs | face_stable_runs | body BC p95_m | body BC p99_m | body BC max_m | body BC excursion_share | body steady p95_m | body steady p99_m | body FS p95_m | body FS p99_m | body FS max_m | point BC p95_m | point BC p99_m | point BC excursion_share | point FS p99_m | gap  | ratio |
| -------------------- | ------ | ------------ | --------------- | --------- | --------------------- | --------------------- | ------------------ | ------------------------ | ---------------------------------- | ---------------------- | ------ | ---------------------- | ----------- | ---------------- | ------------- | ------------- | ------------- | ----------------------- | ----------------- | ----------------- | ------------- | ------------- | ------------- | -------------- | -------------- | ------------------------ | -------------- | ---- | ----- |
| marina-webster-beach | t1t3   | 39399        | 16722           | 0.424     | 676                   | 1491                  | 1295               | 450                      | 18765                              | 0                      | 676    | 238                    | 841         | 3919             | 0.048         | 0.158         | 1.771         | 0.043                   | 0.046             | 0.135             | 0.029         | 0.065         | 0.662         | 0.097          | 0.183          | 0.022                    | 0.150          | 2.41 | 0.86  |
| marina-webster-beach | t1t3t4 | 39399        | 16724           | 0.424     | 675                   | 1491                  | 1295               | 450                      | 18764                              | 0                      | 675    | 238                    | 841         | 3919             | 0.048         | 0.152         | 1.981         | 0.043                   | 0.046             | 0.135             | 0.029         | 0.066         | 0.662         | 0.097          | 0.183          | 0.022                    | 0.150          | 2.31 | 0.83  |
| columbus-broadway    | t1t3   | 77949        | 43106           | 0.553     | 1695                  | 8821                  | 3713               | 2264                     | 18349                              | 1                      | 1695   | 488                    | 2577        | 9268             | 0.107         | 0.280         | 1.141         | 0.040                   | 0.105             | 0.280             | 0.060         | 0.141         | 0.644         | 0.109          | 0.226          | 0.023                    | 0.188          | 1.99 | 1.24  |
| columbus-broadway    | t1t3t4 | 77949        | 43107           | 0.553     | 1696                  | 8821                  | 3710               | 2264                     | 18350                              | 1                      | 1696   | 489                    | 2578        | 9233             | 0.104         | 0.275         | 1.141         | 0.040                   | 0.103             | 0.275             | 0.057         | 0.140         | 0.644         | 0.109          | 0.226          | 0.023                    | 0.188          | 1.96 | 1.22  |

## Table 2: steady-run transitions

Per case and arm, a summary row (windows, p99_m, tail_threshold_m, stable_windows, stable_p99_m), then one row per transition (windows, tail_windows, p99_without_m). Transition groups overlap; p99_without_m is the steady p99 over every window that does not carry that transition.

| case                 | arm    | transition               | windows | p99_m | tail_threshold_m | stable_windows | stable_p99_m | tail_windows | p99_without_m |
| -------------------- | ------ | ------------------------ | ------- | ----- | ---------------- | -------------- | ------------ | ------------ | ------------- |
| marina-webster-beach | t1t3   | **all steady**           | 5551    | 0.135 | 0.046            | 3041           | 0.052        |              |               |
| marina-webster-beach | t1t3   | faceless                 | 492     |       |                  |                |              | 42           | 0.126         |
| marina-webster-beach | t1t3   | lateral_face_enters      | 405     |       |                  |                |              | 78           | 0.101         |
| marina-webster-beach | t1t3   | longitudinal_face_enters | 1069    |       |                  |                |              | 110          | 0.122         |
| marina-webster-beach | t1t3   | face_leaves              | 1391    |       |                  |                |              | 134          | 0.112         |
| marina-webster-beach | t1t3   | face_swaps               | 0       |       |                  |                |              | 0            | 0.135         |
| marina-webster-beach | t1t3   | width_revised            | 365     |       |                  |                |              | 73           | 0.111         |
| marina-webster-beach | t1t3   | length_revised           | 1031    |       |                  |                |              | 102          | 0.110         |
| marina-webster-beach | t1t3t4 | **all steady**           | 5552    | 0.135 | 0.046            | 3042           | 0.052        |              |               |
| marina-webster-beach | t1t3t4 | faceless                 | 492     |       |                  |                |              | 41           | 0.128         |
| marina-webster-beach | t1t3t4 | lateral_face_enters      | 405     |       |                  |                |              | 76           | 0.101         |
| marina-webster-beach | t1t3t4 | longitudinal_face_enters | 1066    |       |                  |                |              | 109          | 0.122         |
| marina-webster-beach | t1t3t4 | face_leaves              | 1389    |       |                  |                |              | 133          | 0.112         |
| marina-webster-beach | t1t3t4 | face_swaps               | 0       |       |                  |                |              | 0            | 0.135         |
| marina-webster-beach | t1t3t4 | width_revised            | 365     |       |                  |                |              | 74           | 0.111         |
| marina-webster-beach | t1t3t4 | length_revised           | 1032    |       |                  |                |              | 101          | 0.103         |
| columbus-broadway    | t1t3   | **all steady**           | 6735    | 0.280 | 0.105            | 3006           | 0.111        |              |               |
| columbus-broadway    | t1t3   | faceless                 | 843     |       |                  |                |              | 68           | 0.272         |
| columbus-broadway    | t1t3   | lateral_face_enters      | 792     |       |                  |                |              | 165          | 0.180         |
| columbus-broadway    | t1t3   | longitudinal_face_enters | 1475    |       |                  |                |              | 115          | 0.240         |
| columbus-broadway    | t1t3   | face_leaves              | 2175    |       |                  |                |              | 176          | 0.240         |
| columbus-broadway    | t1t3   | face_swaps               | 0       |       |                  |                |              | 0            | 0.280         |
| columbus-broadway    | t1t3   | width_revised            | 1088    |       |                  |                |              | 160          | 0.206         |
| columbus-broadway    | t1t3   | length_revised           | 1525    |       |                  |                |              | 130          | 0.240         |
| columbus-broadway    | t1t3t4 | **all steady**           | 6700    | 0.275 | 0.103            | 2998           | 0.109        |              |               |
| columbus-broadway    | t1t3t4 | faceless                 | 837     |       |                  |                |              | 66           | 0.256         |
| columbus-broadway    | t1t3t4 | lateral_face_enters      | 771     |       |                  |                |              | 159          | 0.177         |
| columbus-broadway    | t1t3t4 | longitudinal_face_enters | 1461    |       |                  |                |              | 118          | 0.233         |
| columbus-broadway    | t1t3t4 | face_leaves              | 2151    |       |                  |                |              | 172          | 0.228         |
| columbus-broadway    | t1t3t4 | face_swaps               | 0       |       |                  |                |              | 0            | 0.275         |
| columbus-broadway    | t1t3t4 | width_revised            | 1085    |       |                  |                |              | 161          | 0.196         |
| columbus-broadway    | t1t3t4 | length_revised           | 1523    |       |                  |                |              | 136          | 0.233         |

## Table 3: face-stable strata

Point = point estimates, body = solid bodies, on the face-stable runs.

| case                 | arm    | axis         | bin           | point windows | point p95_m | point p99_m | body windows | body p95_m | body p99_m |
| -------------------- | ------ | ------------ | ------------- | ------------- | ----------- | ----------- | ------------ | ---------- | ---------- |
| marina-webster-beach | t1t3   | heading_rate | lt_5_deg_s    | 1254          | 0.088       | 0.172       | 1240         | 0.030      | 0.065      |
| marina-webster-beach | t1t3   | heading_rate | 5_to_15_deg_s | 1899          | 0.092       | 0.149       | 1893         | 0.027      | 0.065      |
| marina-webster-beach | t1t3   | heading_rate | ge_15_deg_s   | 637           | 0.079       | 0.137       | 638          | 0.035      | 0.086      |
| marina-webster-beach | t1t3   | heading_rate | unknown       | 0             | 0.000       | 0.000       | 0            | 0.000      | 0.000      |
| marina-webster-beach | t1t3   | range        | lt_20_m       | 3284          | 0.089       | 0.147       | 3281         | 0.028      | 0.058      |
| marina-webster-beach | t1t3   | range        | 20_to_40_m    | 452           | 0.089       | 0.182       | 436          | 0.044      | 0.112      |
| marina-webster-beach | t1t3   | range        | ge_40_m       | 54            | 0.051       | 0.127       | 54           | 0.002      | 0.010      |
| marina-webster-beach | t1t3t4 | heading_rate | lt_5_deg_s    | 1254          | 0.088       | 0.172       | 1243         | 0.030      | 0.062      |
| marina-webster-beach | t1t3t4 | heading_rate | 5_to_15_deg_s | 1899          | 0.092       | 0.149       | 1894         | 0.027      | 0.065      |
| marina-webster-beach | t1t3t4 | heading_rate | ge_15_deg_s   | 637           | 0.079       | 0.137       | 638          | 0.035      | 0.086      |
| marina-webster-beach | t1t3t4 | heading_rate | unknown       | 0             | 0.000       | 0.000       | 0            | 0.000      | 0.000      |
| marina-webster-beach | t1t3t4 | range        | lt_20_m       | 3311          | 0.090       | 0.149       | 3308         | 0.028      | 0.058      |
| marina-webster-beach | t1t3t4 | range        | 20_to_40_m    | 425           | 0.084       | 0.178       | 413          | 0.044      | 0.112      |
| marina-webster-beach | t1t3t4 | range        | ge_40_m       | 54            | 0.051       | 0.127       | 54           | 0.002      | 0.010      |
| columbus-broadway    | t1t3   | heading_rate | lt_5_deg_s    | 1287          | 0.088       | 0.184       | 1324         | 0.066      | 0.144      |
| columbus-broadway    | t1t3   | heading_rate | 5_to_15_deg_s | 1509          | 0.100       | 0.206       | 1504         | 0.054      | 0.140      |
| columbus-broadway    | t1t3   | heading_rate | ge_15_deg_s   | 1175          | 0.094       | 0.178       | 1183         | 0.057      | 0.137      |
| columbus-broadway    | t1t3   | heading_rate | unknown       | 0             | 0.000       | 0.000       | 0            | 0.000      | 0.000      |
| columbus-broadway    | t1t3   | range        | lt_20_m       | 3356          | 0.098       | 0.194       | 3394         | 0.064      | 0.145      |
| columbus-broadway    | t1t3   | range        | 20_to_40_m    | 573           | 0.083       | 0.170       | 575          | 0.023      | 0.097      |
| columbus-broadway    | t1t3   | range        | ge_40_m       | 42            | 0.039       | 0.052       | 42           | 0.001      | 0.020      |
| columbus-broadway    | t1t3t4 | heading_rate | lt_5_deg_s    | 1286          | 0.088       | 0.184       | 1323         | 0.066      | 0.145      |
| columbus-broadway    | t1t3t4 | heading_rate | 5_to_15_deg_s | 1511          | 0.100       | 0.206       | 1507         | 0.052      | 0.123      |
| columbus-broadway    | t1t3t4 | heading_rate | ge_15_deg_s   | 1175          | 0.094       | 0.178       | 1174         | 0.057      | 0.140      |
| columbus-broadway    | t1t3t4 | heading_rate | unknown       | 0             | 0.000       | 0.000       | 0            | 0.000      | 0.000      |
| columbus-broadway    | t1t3t4 | range        | lt_20_m       | 3362          | 0.098       | 0.194       | 3392         | 0.062      | 0.145      |
| columbus-broadway    | t1t3t4 | range        | 20_to_40_m    | 568           | 0.083       | 0.170       | 570          | 0.021      | 0.061      |
| columbus-broadway    | t1t3t4 | range        | ge_40_m       | 42            | 0.039       | 0.052       | 42           | 0.001      | 0.021      |
