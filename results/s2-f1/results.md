# S2.1 F1: face-transition remedies on the tuning sites (Mac)

Branch `claude/upbeat-galileo-4xbaat-s2-1-faces` at `9fb80e61`; cases marina-webster-beach and columbus-broadway; `-sample-points 256`; two arms at a time (sb with t1, then t2 with t1t2).

## Host

- `Darwin hydra.dove-harmonic.ts.net 24.6.0 Darwin Kernel Version 24.6.0: Tue Apr 21 20:19:01 PDT 2026; root:xnu-11417.140.69.710.16~1/RELEASE_ARM64_T6000 arm64`
- CPU: Apple M1 Pro (8 logical cores)
- Memory: 16 GiB

## Arms

| Arm | -experiment | Wall clock (s) | Peak RSS (GiB) |
| --- | --- | ---: | ---: |
| sb | `solid_body` | 1179.8 | 0.91 |
| t1 | `solid_body,solid_body_face_hysteresis` | 1177.6 | 0.90 |
| t2 | `solid_body,solid_body_face_consider` | 1544.2 | 0.90 |
| t1t2 | `solid_body,solid_body_face_hysteresis,solid_body_face_consider` | 1546.8 | 0.91 |

## Table 1: solid body per case and arm

Anchor residuals in metres. BCF = body-centre frames, FS = face-stable runs. Gap = body BCF p99 / body FS p99.

| Case | Arm | solid_bodies | near_edge_fixes | fix_share | fb.face_hysteresis | lapses | steady_runs | face_stable_runs | body BCF p95 | body BCF p99 | body BCF max | body BCF excursion_share | body FS p95 | body FS p99 | body FS max | point BCF p99 | point FS p99 | gap |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| marina-webster-beach | sb | 39399 | 14557 | 0.369 | 0 | 402 | 573 | 3604 | 0.061 | 0.231 | 1.882 | 0.036 | 0.036 | 0.074 | 0.570 | 0.194 | 0.153 | 3.122 |
| marina-webster-beach | t1 | 39399 | 13570 | 0.344 | 1045 | 587 | 730 | 3062 | 0.058 | 0.166 | 1.827 | 0.024 | 0.032 | 0.068 | 0.536 | 0.180 | 0.150 | 2.452 |
| marina-webster-beach | t2 | 39399 | 14457 | 0.367 | 0 | 406 | 577 | 3642 | 0.064 | 0.221 | 1.691 | 0.036 | 0.038 | 0.077 | 0.541 | 0.194 | 0.153 | 2.881 |
| marina-webster-beach | t1t2 | 39399 | 13570 | 0.344 | 1045 | 587 | 730 | 3062 | 0.060 | 0.169 | 1.702 | 0.024 | 0.036 | 0.069 | 0.528 | 0.180 | 0.150 | 2.436 |
| columbus-broadway | sb | 77949 | 40630 | 0.521 | 0 | 898 | 1817 | 7774 | 0.130 | 0.388 | 1.137 | 0.043 | 0.082 | 0.193 | 0.711 | 0.254 | 0.178 | 2.008 |
| columbus-broadway | t1 | 77949 | 37130 | 0.476 | 2791 | 1363 | 2039 | 7328 | 0.122 | 0.336 | 1.136 | 0.042 | 0.069 | 0.140 | 0.389 | 0.197 | 0.164 | 2.401 |
| columbus-broadway | t2 | 77949 | 40630 | 0.521 | 0 | 898 | 1817 | 7773 | 0.128 | 0.336 | 1.135 | 0.040 | 0.081 | 0.198 | 0.632 | 0.254 | 0.178 | 1.694 |
| columbus-broadway | t1t2 | 77949 | 37130 | 0.476 | 2790 | 1363 | 2039 | 7328 | 0.121 | 0.307 | 1.132 | 0.034 | 0.073 | 0.135 | 0.369 | 0.197 | 0.164 | 2.277 |

## Table 2: face-stable strata, arms sb and t1

| Case | Arm | Axis | Bin | point windows | point p95 | point p99 | body windows | body p95 | body p99 |
| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| marina-webster-beach | sb | heading_rate | lt_5_deg_s | 1053 | 0.096 | 0.197 | 1041 | 0.042 | 0.076 |
| marina-webster-beach | sb | heading_rate | 5_to_15_deg_s | 1617 | 0.092 | 0.143 | 1613 | 0.032 | 0.073 |
| marina-webster-beach | sb | heading_rate | ge_15_deg_s | 242 | 0.077 | 0.115 | 242 | 0.034 | 0.204 |
| marina-webster-beach | sb | heading_rate | unknown | 0 | 0.000 | 0.000 | 0 | 0.000 | 0.000 |
| marina-webster-beach | sb | range | lt_20_m | 2534 | 0.095 | 0.151 | 2534 | 0.036 | 0.074 |
| marina-webster-beach | sb | range | 20_to_40_m | 351 | 0.084 | 0.183 | 335 | 0.033 | 0.055 |
| marina-webster-beach | sb | range | ge_40_m | 27 | 0.051 | 0.127 | 27 | 0.276 | 0.570 |
| marina-webster-beach | t1 | heading_rate | lt_5_deg_s | 933 | 0.090 | 0.192 | 919 | 0.041 | 0.070 |
| marina-webster-beach | t1 | heading_rate | 5_to_15_deg_s | 1473 | 0.092 | 0.139 | 1472 | 0.029 | 0.062 |
| marina-webster-beach | t1 | heading_rate | ge_15_deg_s | 221 | 0.077 | 0.115 | 221 | 0.028 | 0.197 |
| marina-webster-beach | t1 | heading_rate | unknown | 0 | 0.000 | 0.000 | 0 | 0.000 | 0.000 |
| marina-webster-beach | t1 | range | lt_20_m | 2258 | 0.092 | 0.150 | 2258 | 0.032 | 0.066 |
| marina-webster-beach | t1 | range | 20_to_40_m | 345 | 0.081 | 0.170 | 330 | 0.033 | 0.058 |
| marina-webster-beach | t1 | range | ge_40_m | 24 | 0.051 | 0.127 | 24 | 0.258 | 0.536 |
| columbus-broadway | sb | heading_rate | lt_5_deg_s | 944 | 0.096 | 0.164 | 977 | 0.081 | 0.167 |
| columbus-broadway | sb | heading_rate | 5_to_15_deg_s | 1089 | 0.099 | 0.212 | 1083 | 0.066 | 0.148 |
| columbus-broadway | sb | heading_rate | ge_15_deg_s | 475 | 0.095 | 0.152 | 490 | 0.125 | 0.369 |
| columbus-broadway | sb | heading_rate | unknown | 0 | 0.000 | 0.000 | 0 | 0.000 | 0.000 |
| columbus-broadway | sb | range | lt_20_m | 2215 | 0.097 | 0.174 | 2256 | 0.085 | 0.195 |
| columbus-broadway | sb | range | 20_to_40_m | 288 | 0.096 | 0.197 | 289 | 0.057 | 0.190 |
| columbus-broadway | sb | range | ge_40_m | 5 | 0.035 | 0.035 | 5 | 0.012 | 0.012 |
| columbus-broadway | t1 | heading_rate | lt_5_deg_s | 793 | 0.089 | 0.146 | 822 | 0.075 | 0.162 |
| columbus-broadway | t1 | heading_rate | 5_to_15_deg_s | 960 | 0.091 | 0.172 | 951 | 0.055 | 0.093 |
| columbus-broadway | t1 | heading_rate | ge_15_deg_s | 398 | 0.090 | 0.151 | 416 | 0.086 | 0.219 |
| columbus-broadway | t1 | heading_rate | unknown | 0 | 0.000 | 0.000 | 0 | 0.000 | 0.000 |
| columbus-broadway | t1 | range | lt_20_m | 1874 | 0.091 | 0.164 | 1911 | 0.070 | 0.135 |
| columbus-broadway | t1 | range | 20_to_40_m | 272 | 0.085 | 0.170 | 273 | 0.068 | 0.193 |
| columbus-broadway | t1 | range | ge_40_m | 5 | 0.035 | 0.035 | 5 | 0.013 | 0.013 |
