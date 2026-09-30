# S2 F6: near_edge_track and the T1+T3 control on the tuning partition (Mac)

Host: hydra, MacBookPro18,3, Apple M1 Pro, 8 CPUs, 16 GiB, macOS 15.7.7.

Source: `main` at `0b540677`. Cases marina-webster-beach and columbus-broadway, 256 sample points.

Capture root: `/Volumes/banshee-captures/lidar` (NAS share `banshee-captures` on `arrow`, read in place over SMB; file sizes checked against the source manifest, which is byte-identical to F5's), `-pcap-subdir s2`. Work directory: `/Volumes/lidar/evidence/s2-f6`; per-case evidence databases were discarded after each summary (`-discard-evidence`).

Not one arm at a time: track was started by hand and worked on the USB disk (`/Volumes/lidar/evidence/s2-f6/track`). control and translate were run by a second session (`run-f6-ssd.sh`) on the internal disk (`/Users/david/s2-f6-ssd/<arm>`), then moved into the work directory. They ran while track's columbus-broadway replay was still running, so the arms' wall clocks and CPU times are not like for like. The same binary, flags, capture root and corpus/index files were used throughout.

| Arm       | -experiment                                                                                                              | Exit                                             | Wall clock          | CPU (user) | Peak RSS |
| --------- | ------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------ | ------------------- | ---------- | -------- |
| track     | `solid_body,solid_body_face_hysteresis,solid_body_course_faces,solid_body_full_members,near_edge_track`                  | not recorded (started by hand; summary complete) | 29505 s (491.8 min) | 2149 s     | 894 MiB  |
| control   | `solid_body,solid_body_face_hysteresis,solid_body_course_faces,solid_body_full_members`                                  | 0                                                | 1302 s (21.7 min)   | 1745 s     | 982 MiB  |
| translate | `solid_body,solid_body_face_hysteresis,solid_body_course_faces,solid_body_full_members,solid_body_reference_translation` | 0                                                | 1109 s (18.5 min)   | 1689 s     | 918 MiB  |

The track arm's timing comes from ps samples once a minute (arm started by hand without /usr/bin/time); wall = first log timestamp to summary write, CPU = user+sys at last sample.

The control arm's timing comes from /usr/bin/time -l, read from control's run.log shortly after 16:15, before it was overwritten. When track exited at 19:36, a command the user's shell had chained after it started a control replay, which refused to run ("output directory must be empty"). The shell redirect had already truncated control/run.log to that one line, so control-run.txt holds only that line. The control summary (written 15:55) is untouched.

`baseline_equal` per case and arm: marina-webster-beach/track: True; marina-webster-beach/control: True; marina-webster-beach/translate: True; columbus-broadway/track: True; columbus-broadway/control: True; columbus-broadway/translate: True.

## Table 1: solid-body summary per case and arm

Fallback columns are the `fallbacks` map. Body BC = `anchor_solid_bodies_body_centre_frames`, body steady = `anchor_solid_bodies_steady_runs`, body FS = `anchor_solid_bodies_face_stable_runs`; point BC / FS = the matching `anchor_point_estimates_*`. gap = body BC p99 / body FS p99; ratio = body BC p99 / point BC p99.

| case                 | arm       | solid_bodies | near_edge_fixes | fix_share | fb:body_centre_lapsed | fb:class_prior_extent | fb:face_hysteresis | fb:initialisation_window | fb:no_face_reached_minimum_support | fb:singular_innovation | lapses | re_references | tracks_width_converged | steady_runs | face_stable_runs | body BC p95_m | body BC p99_m | body BC max_m | body BC excursion_share | body steady p95_m | body steady p99_m | body FS p95_m | body FS p99_m | body FS max_m | point BC p95_m | point BC p99_m | point BC excursion_share | point FS p99_m | gap  | ratio |
| -------------------- | --------- | ------------ | --------------- | --------- | --------------------- | --------------------- | ------------------ | ------------------------ | ---------------------------------- | ---------------------- | ------ | ------------- | ---------------------- | ----------- | ---------------- | ------------- | ------------- | ------------- | ----------------------- | ----------------- | ----------------- | ------------- | ------------- | ------------- | -------------- | -------------- | ------------------------ | -------------- | ---- | ----- |
| marina-webster-beach | track     | 39321        | 14412           | 0.367     | 573                   | 4369                  | 1026               | 463                      | 18478                              | 0                      | 573    | 773           | 239                    | 773         | 3710             | 0.042         | 0.132         | 1.179         | 0.032                   | 0.040             | 0.120             | 0.025         | 0.056         | 0.561         | 0.042          | 0.132          | 0.032                    | 0.056          | 2.34 | 1.00  |
| marina-webster-beach | control   | 39399        | 16722           | 0.424     | 676                   | 1491                  | 1295               | 450                      | 18765                              | 0                      | 676    | 841           | 238                    | 841         | 3919             | 0.048         | 0.158         | 1.771         | 0.043                   | 0.046             | 0.135             | 0.029         | 0.065         | 0.662         | 0.097          | 0.183          | 0.022                    | 0.150          | 2.41 | 0.86  |
| marina-webster-beach | translate | 39399        | 16724           | 0.424     | 676                   | 1491                  | 1292               | 450                      | 18766                              | 0                      | 676    | 841           | 238                    | 841         | 3911             | 0.048         | 0.155         | 1.785         | 0.043                   | 0.046             | 0.139             | 0.030         | 0.068         | 0.663         | 0.097          | 0.183          | 0.022                    | 0.150          | 2.29 | 0.84  |
| columbus-broadway    | track     | 77737        | 41439           | 0.533     | 2091                  | 9879                  | 3510               | 2308                     | 18510                              | 0                      | 2091   | 3057          | 489                    | 3057        | 9849             | 0.105         | 0.260         | 0.889         | 0.026                   | 0.102             | 0.260             | 0.053         | 0.127         | 0.436         | 0.105          | 0.260          | 0.026                    | 0.127          | 2.05 | 1.00  |
| columbus-broadway    | control   | 77949        | 43106           | 0.553     | 1695                  | 8821                  | 3713               | 2264                     | 18349                              | 1                      | 1695   | 2577          | 488                    | 2577        | 9268             | 0.107         | 0.280         | 1.141         | 0.040                   | 0.105             | 0.280             | 0.060         | 0.141         | 0.644         | 0.109          | 0.226          | 0.023                    | 0.188          | 1.99 | 1.24  |
| columbus-broadway    | translate | 77949        | 43103           | 0.553     | 1695                  | 8821                  | 3716               | 2264                     | 18349                              | 1                      | 1695   | 2576          | 486                    | 2576        | 9267             | 0.108         | 0.290         | 1.142         | 0.040                   | 0.107             | 0.294             | 0.060         | 0.142         | 0.641         | 0.109          | 0.226          | 0.023                    | 0.188          | 2.04 | 1.28  |

In the track arm the point estimates are the tracked filter's own, not the per-frame estimates the control arm scores, so its `ratio` compares the solid body against a different reference. The table below puts both arms on the control arm's point estimate: track body BC p99 / control point BC p99, and track point BC p99 / control point BC p99.

| case                 | track body BC p99_m | track point BC p99_m (tracked filter) | control point BC p99_m (per-frame) | track body / control point | track point / control point |
| -------------------- | ------------------- | ------------------------------------- | ---------------------------------- | -------------------------- | --------------------------- |
| marina-webster-beach | 0.132               | 0.132                                 | 0.183                              | 0.72                       | 0.72                        |
| columbus-broadway    | 0.260               | 0.260                                 | 0.226                              | 1.15                       | 1.15                        |

## Table 2: steady-run transitions

Per case and arm, a summary row (windows, p99_m, tail_threshold_m, stable_windows, stable_p99_m), then one row per transition (windows, tail_windows, p99_without_m). Transition groups overlap; p99_without_m is the steady-run p99 with that group's windows removed.

| case                 | arm       | transition               | windows | p99_m | tail_threshold_m | stable_windows | stable_p99_m | tail_windows | p99_without_m |
| -------------------- | --------- | ------------------------ | ------- | ----- | ---------------- | -------------- | ------------ | ------------ | ------------- |
| marina-webster-beach | track     | **all steady**           | 5022    | 0.120 | 0.040            | 2790           | 0.048        |              |               |
| marina-webster-beach | track     | faceless                 | 571     |       |                  |                |              | 55           | 0.119         |
| marina-webster-beach | track     | lateral_face_enters      | 458     |       |                  |                |              | 99           | 0.071         |
| marina-webster-beach | track     | longitudinal_face_enters | 868     |       |                  |                |              | 91           | 0.106         |
| marina-webster-beach | track     | face_leaves              | 1299    |       |                  |                |              | 126          | 0.102         |
| marina-webster-beach | track     | face_swaps               | 0       |       |                  |                |              | 0            | 0.120         |
| marina-webster-beach | track     | width_revised            | 305     |       |                  |                |              | 57           | 0.097         |
| marina-webster-beach | track     | length_revised           | 920     |       |                  |                |              | 94           | 0.084         |
| marina-webster-beach | control   | **all steady**           | 5551    | 0.135 | 0.046            | 3041           | 0.052        |              |               |
| marina-webster-beach | control   | faceless                 | 492     |       |                  |                |              | 42           | 0.126         |
| marina-webster-beach | control   | lateral_face_enters      | 405     |       |                  |                |              | 78           | 0.101         |
| marina-webster-beach | control   | longitudinal_face_enters | 1069    |       |                  |                |              | 110          | 0.122         |
| marina-webster-beach | control   | face_leaves              | 1391    |       |                  |                |              | 134          | 0.112         |
| marina-webster-beach | control   | face_swaps               | 0       |       |                  |                |              | 0            | 0.135         |
| marina-webster-beach | control   | width_revised            | 365     |       |                  |                |              | 73           | 0.111         |
| marina-webster-beach | control   | length_revised           | 1031    |       |                  |                |              | 102          | 0.110         |
| marina-webster-beach | translate | **all steady**           | 5550    | 0.139 | 0.046            | 3038           | 0.052        |              |               |
| marina-webster-beach | translate | faceless                 | 495     |       |                  |                |              | 44           | 0.126         |
| marina-webster-beach | translate | lateral_face_enters      | 408     |       |                  |                |              | 79           | 0.100         |
| marina-webster-beach | translate | longitudinal_face_enters | 1067    |       |                  |                |              | 109          | 0.122         |
| marina-webster-beach | translate | face_leaves              | 1393    |       |                  |                |              | 133          | 0.111         |
| marina-webster-beach | translate | face_swaps               | 0       |       |                  |                |              | 0            | 0.139         |
| marina-webster-beach | translate | width_revised            | 366     |       |                  |                |              | 74           | 0.111         |
| marina-webster-beach | translate | length_revised           | 1035    |       |                  |                |              | 102          | 0.106         |
| columbus-broadway    | track     | **all steady**           | 6037    | 0.260 | 0.102            | 2610           | 0.099        |              |               |
| columbus-broadway    | track     | faceless                 | 856     |       |                  |                |              | 62           | 0.260         |
| columbus-broadway    | track     | lateral_face_enters      | 795     |       |                  |                |              | 167          | 0.153         |
| columbus-broadway    | track     | longitudinal_face_enters | 1393    |       |                  |                |              | 105          | 0.257         |
| columbus-broadway    | track     | face_leaves              | 2056    |       |                  |                |              | 168          | 0.204         |
| columbus-broadway    | track     | face_swaps               | 0       |       |                  |                |              | 0            | 0.260         |
| columbus-broadway    | track     | width_revised            | 874     |       |                  |                |              | 125          | 0.201         |
| columbus-broadway    | track     | length_revised           | 1375    |       |                  |                |              | 134          | 0.215         |
| columbus-broadway    | control   | **all steady**           | 6735    | 0.280 | 0.105            | 3006           | 0.111        |              |               |
| columbus-broadway    | control   | faceless                 | 843     |       |                  |                |              | 68           | 0.272         |
| columbus-broadway    | control   | lateral_face_enters      | 792     |       |                  |                |              | 165          | 0.180         |
| columbus-broadway    | control   | longitudinal_face_enters | 1475    |       |                  |                |              | 115          | 0.240         |
| columbus-broadway    | control   | face_leaves              | 2175    |       |                  |                |              | 176          | 0.240         |
| columbus-broadway    | control   | face_swaps               | 0       |       |                  |                |              | 0            | 0.280         |
| columbus-broadway    | control   | width_revised            | 1088    |       |                  |                |              | 160          | 0.206         |
| columbus-broadway    | control   | length_revised           | 1525    |       |                  |                |              | 130          | 0.240         |
| columbus-broadway    | translate | **all steady**           | 6736    | 0.294 | 0.107            | 2993           | 0.111        |              |               |
| columbus-broadway    | translate | faceless                 | 848     |       |                  |                |              | 67           | 0.280         |
| columbus-broadway    | translate | lateral_face_enters      | 795     |       |                  |                |              | 163          | 0.178         |
| columbus-broadway    | translate | longitudinal_face_enters | 1479    |       |                  |                |              | 117          | 0.250         |
| columbus-broadway    | translate | face_leaves              | 2183    |       |                  |                |              | 174          | 0.240         |
| columbus-broadway    | translate | face_swaps               | 0       |       |                  |                |              | 0            | 0.294         |
| columbus-broadway    | translate | width_revised            | 1094    |       |                  |                |              | 158          | 0.211         |
| columbus-broadway    | translate | length_revised           | 1526    |       |                  |                |              | 128          | 0.249         |

## Table 3: face-stable strata

Point = point estimates, body = solid bodies, on the face-stable runs.

| case                 | arm       | axis         | bin           | point windows | point p95_m | point p99_m | body windows | body p95_m | body p99_m |
| -------------------- | --------- | ------------ | ------------- | ------------- | ----------- | ----------- | ------------ | ---------- | ---------- |
| marina-webster-beach | track     | heading_rate | lt_5_deg_s    | 1127          | 0.028       | 0.074       | 1127         | 0.028      | 0.074      |
| marina-webster-beach | track     | heading_rate | 5_to_15_deg_s | 1701          | 0.021       | 0.050       | 1701         | 0.021      | 0.050      |
| marina-webster-beach | track     | heading_rate | ge_15_deg_s   | 607           | 0.028       | 0.049       | 607          | 0.028      | 0.049      |
| marina-webster-beach | track     | heading_rate | unknown       | 0             | 0.000       | 0.000       | 0            | 0.000      | 0.000      |
| marina-webster-beach | track     | range        | lt_20_m       | 2984          | 0.024       | 0.049       | 2984         | 0.024      | 0.049      |
| marina-webster-beach | track     | range        | 20_to_40_m    | 402           | 0.037       | 0.097       | 402          | 0.037      | 0.097      |
| marina-webster-beach | track     | range        | ge_40_m       | 49            | 0.004       | 0.010       | 49           | 0.004      | 0.010      |
| marina-webster-beach | control   | heading_rate | lt_5_deg_s    | 1254          | 0.088       | 0.172       | 1240         | 0.030      | 0.065      |
| marina-webster-beach | control   | heading_rate | 5_to_15_deg_s | 1899          | 0.092       | 0.149       | 1893         | 0.027      | 0.065      |
| marina-webster-beach | control   | heading_rate | ge_15_deg_s   | 637           | 0.079       | 0.137       | 638          | 0.035      | 0.086      |
| marina-webster-beach | control   | heading_rate | unknown       | 0             | 0.000       | 0.000       | 0            | 0.000      | 0.000      |
| marina-webster-beach | control   | range        | lt_20_m       | 3284          | 0.089       | 0.147       | 3281         | 0.028      | 0.058      |
| marina-webster-beach | control   | range        | 20_to_40_m    | 452           | 0.089       | 0.182       | 436          | 0.044      | 0.112      |
| marina-webster-beach | control   | range        | ge_40_m       | 54            | 0.051       | 0.127       | 54           | 0.002      | 0.010      |
| marina-webster-beach | translate | heading_rate | lt_5_deg_s    | 1254          | 0.088       | 0.172       | 1239         | 0.031      | 0.065      |
| marina-webster-beach | translate | heading_rate | 5_to_15_deg_s | 1897          | 0.092       | 0.149       | 1891         | 0.027      | 0.065      |
| marina-webster-beach | translate | heading_rate | ge_15_deg_s   | 638           | 0.079       | 0.137       | 639          | 0.038      | 0.084      |
| marina-webster-beach | translate | heading_rate | unknown       | 0             | 0.000       | 0.000       | 0            | 0.000      | 0.000      |
| marina-webster-beach | translate | range        | lt_20_m       | 3287          | 0.089       | 0.147       | 3284         | 0.029      | 0.060      |
| marina-webster-beach | translate | range        | 20_to_40_m    | 448           | 0.089       | 0.182       | 431          | 0.043      | 0.111      |
| marina-webster-beach | translate | range        | ge_40_m       | 54            | 0.051       | 0.127       | 54           | 0.002      | 0.010      |
| columbus-broadway    | track     | heading_rate | lt_5_deg_s    | 1125          | 0.056       | 0.133       | 1125         | 0.056      | 0.133      |
| columbus-broadway    | track     | heading_rate | 5_to_15_deg_s | 1343          | 0.049       | 0.116       | 1343         | 0.049      | 0.116      |
| columbus-broadway    | track     | heading_rate | ge_15_deg_s   | 1001          | 0.056       | 0.153       | 1001         | 0.056      | 0.153      |
| columbus-broadway    | track     | heading_rate | unknown       | 0             | 0.000       | 0.000       | 0            | 0.000      | 0.000      |
| columbus-broadway    | track     | range        | lt_20_m       | 2931          | 0.057       | 0.135       | 2931         | 0.057      | 0.135      |
| columbus-broadway    | track     | range        | 20_to_40_m    | 508           | 0.020       | 0.056       | 508          | 0.020      | 0.056      |
| columbus-broadway    | track     | range        | ge_40_m       | 30            | 0.009       | 0.013       | 30           | 0.009      | 0.013      |
| columbus-broadway    | control   | heading_rate | lt_5_deg_s    | 1287          | 0.088       | 0.184       | 1324         | 0.066      | 0.144      |
| columbus-broadway    | control   | heading_rate | 5_to_15_deg_s | 1509          | 0.100       | 0.206       | 1504         | 0.054      | 0.140      |
| columbus-broadway    | control   | heading_rate | ge_15_deg_s   | 1175          | 0.094       | 0.178       | 1183         | 0.057      | 0.137      |
| columbus-broadway    | control   | heading_rate | unknown       | 0             | 0.000       | 0.000       | 0            | 0.000      | 0.000      |
| columbus-broadway    | control   | range        | lt_20_m       | 3356          | 0.098       | 0.194       | 3394         | 0.064      | 0.145      |
| columbus-broadway    | control   | range        | 20_to_40_m    | 573           | 0.083       | 0.170       | 575          | 0.023      | 0.097      |
| columbus-broadway    | control   | range        | ge_40_m       | 42            | 0.039       | 0.052       | 42           | 0.001      | 0.020      |
| columbus-broadway    | translate | heading_rate | lt_5_deg_s    | 1282          | 0.088       | 0.184       | 1319         | 0.067      | 0.146      |
| columbus-broadway    | translate | heading_rate | 5_to_15_deg_s | 1508          | 0.100       | 0.206       | 1503         | 0.053      | 0.136      |
| columbus-broadway    | translate | heading_rate | ge_15_deg_s   | 1175          | 0.094       | 0.178       | 1183         | 0.059      | 0.137      |
| columbus-broadway    | translate | heading_rate | unknown       | 0             | 0.000       | 0.000       | 0            | 0.000      | 0.000      |
| columbus-broadway    | translate | range        | lt_20_m       | 3351          | 0.098       | 0.194       | 3389         | 0.064      | 0.146      |
| columbus-broadway    | translate | range        | 20_to_40_m    | 572           | 0.083       | 0.170       | 574          | 0.025      | 0.091      |
| columbus-broadway    | translate | range        | ge_40_m       | 42            | 0.039       | 0.052       | 42           | 0.001      | 0.016      |

## Table 4: tracker counts

From each case's `continuity` block. The summary has no mean confirmed duration, association or gate-rejection rate, or tracker time per frame, so those are absent.

| case                 | arm       | tracks_born | tracks_confirmed | births per confirmation |
| -------------------- | --------- | ----------- | ---------------- | ----------------------- |
| marina-webster-beach | track     | 1841        | 540              | 3.41                    |
| marina-webster-beach | control   | 1800        | 519              | 3.47                    |
| marina-webster-beach | translate | 1800        | 519              | 3.47                    |
| columbus-broadway    | track     | 7540        | 2508             | 3.01                    |
| columbus-broadway    | control   | 7500        | 2459             | 3.05                    |
| columbus-broadway    | translate | 7500        | 2459             | 3.05                    |

## Notes

- `re_references` equals `steady_runs` in every case and arm, so the two may count the same thing. That has not been checked in code.
- In the track arm, the solid-body and point-estimate anchors are identical in every block (body BC, FS and strata), so its `ratio` is 1.00 by construction. The table under Table 1 compares it with the control arm's per-frame point estimate instead.
- The track arm's tracker counts differ from control's (marina-webster-beach 1841/540 against 1800/519; columbus-broadway 7540/2508 against 7500/2459), so `near_edge_track` changes the tracker's births and confirmations, not only the reported anchor. control and translate have the same tracker counts.
