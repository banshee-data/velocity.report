# Following gap probe with a visible end and detached opposite returns

- **Scope:** Kirk0 development passage, offline fit and comparison of six previously unavailable reference samples
- **Decision:** retain the observed-end proposal as a development experiment and use fresh independent references to evaluate it

## Why six gaps were missing

The [facet and shell review](facet-shell-gap-review-2026-11.md) compared lifecycle fits with
11 saved gaps across two reviewed following intervals. Only five could be paired. In all six
missing rows the lifecycle diagnostics report a resolved heading and detached returns at the
opposite end from the face needed for the gap. The fit identifies the required end as facing
the sensor and the detached end as facing away. These cases test whether a face can still give
a useful gap when the full pose is unavailable.

In VelocityVisualiser, a separate view opened the same reviewed masks with the saved physical
references set aside. The Top, elevation and 3D views showed a coherent rear band and side on
truck 1 at sample 495, and front returns on truck 2 at sample 502. At truck 2 sample 509, a
distinct front band remained visible while separated labelled returns extended behind it.
Screenshots for all six samples are retained with the local evidence. The checks used a
previously examined development passage. Physical reference coverage remains that of the original
review.

## Comparison method

The original arm uses the saved lifecycle proposals. The probe reuses the ordinary
`velocity lidar annotation-reference fit --sample` command with default fit options and the
same reviewed masks. This fit uses the object's reviewed passage for its size and may use
neighbouring frames for heading. Both arms reuse the frozen replay and its tuning.

For each missing keyframe, the fit had to resolve front from rear, classify the needed face as
visible and facing the sensor, and report detached returns only at the hidden opposite end. The
probe kept the fitted face centre, heading and their original bounds. It omitted the offset
from the face to the body centre and left the opposite endpoint unknown. All six keyframes were
imported as proposals into a separate copy of the lifecycle view. The comparison kept the
original body records, prior keyframes, leader decisions and all 118 pair/frame rows. The result
is a diagnostic gap from visible ends, with the comparison's existing longitudinal projection
and worst-case uncertainty propagation.

## Sample evidence

The geometric estimates and frozen references agree closely at the needed end. Five face centres
match exactly and the remaining centre differs by 0.090 m. These matching values reflect shared
fit evidence. Bounds are quoted as horizontal radii in metres. Counts and distances for detached
returns come from the existing diagnostic's longitudinal projection.

| Needed face   | Sample | Reviewed returns | Detached at opposite end | Frozen face bound (m) | Probe face bound (m) |
| ------------- | -----: | ---------------: | ------------------------ | --------------------: | -------------------: |
| Truck 1 rear  |    491 |            1,559 | 1 at 1.31 m              |                 1.474 |                2.162 |
| Truck 1 rear  |    495 |            1,022 | 1 at 1.31 m              |                 1.494 |                1.494 |
| Truck 1 rear  |    509 |              388 | 1 at 0.99 m              |                 1.543 |                1.543 |
| Truck 2 front |    502 |              338 | 2 at 0.57 m              |                 0.275 |                1.149 |
| Truck 2 front |    504 |              406 | 1 at 1.84 m              |                 0.286 |                1.154 |
| Truck 2 front |    509 |              628 | 12 at 1.81 m             |                 0.248 |                1.123 |

## Following gaps

The probe produced all six missing comparisons. On the same 118 pair/frame rows, candidate gap
availability rose from 56 to 62, or 47.5% to 52.5%. Pairing with frozen reference gaps rose from
5 of 11 to 11 of 11. This difference is confined to the six selected samples in this development
passage.

| Follower → leader | Sample | Frozen gap [bounds] (m) | Probe gap [bounds] (m)  | Change (m) |
| ----------------- | -----: | ----------------------- | ----------------------- | ---------: |
| Truck 2 → car 8   |    502 | 18.419 [17.270, 19.567] | 18.417 [15.210, 21.624] |     −0.002 |
| Truck 2 → car 8   |    504 | 17.897 [16.792, 19.002] | 17.895 [14.748, 21.042] |     −0.001 |
| Truck 2 → car 8   |    509 | 16.398 [15.488, 17.308] | 16.343 [13.411, 19.275] |     −0.055 |
| Car 8 → truck 1   |    491 | 11.608 [8.984, 14.232]  | 11.278 [6.847, 15.709]  |     −0.330 |
| Car 8 → truck 1   |    495 | 12.627 [9.979, 15.275]  | 12.644 [8.031, 17.257]  |     +0.017 |
| Car 8 → truck 1   |    509 | 15.924 [14.000, 17.848] | 15.904 [12.090, 19.718] |     −0.020 |

The six restored comparisons differ by at most 0.330 m and have gap intervals 1.69–3.22 times
as wide as the frozen references. Per-version follower heading and the other vehicle's endpoint
can also affect the gap difference. This probe shows the potential coverage of a bounded visible
face in these selected cases. Accuracy and calibration still need separate reference evidence.

## Provenance

- Pack `sha256:67842cb1b22b70e7dff46aa53af48405e44ddd69503d3c43a0c7670450a1507e`, membership revision 2233 with digest `sha256:12b47778fdf9119e83d18e9e8e0ebeadaea1b52b5ba32b35441c83d8e8edfad8`.
- Frozen reference `body-fit-qa`, physical revision 78, digest `sha256:462aea22e62db5f34cde2c70faf562a00174055fb72e64320f19036a264f653d`.
- Lifecycle comparison view `lifecycle-review-20261118`, physical revision 84, digest `sha256:5e2c196d3ff34fc5279fbcd80bf17f254fd39f4d59aaa847d9d94e2633635669`.
- Separate probe view `endpoint-proposal-20261120`, physical revision 85, digest `sha256:0b456e3706a6e0590b1a723eedb2a4724f711481d67c560493ab8487675ad174`.
- Make-built CLI `v0.5.1-pre52`, source commit `3c98dc8d807afea4f2f6e79ff9090fe927587bd4`.
- Raw diagnostics, import, screenshots and comparisons are under `lifecycle-following/endpoint-opposite-support-20261120` on the attached LiDAR-backed evidence volume. The view and its proposal copy are local.

Two executions produced byte-identical comparisons with SHA-256
`c85aa616c2cfd8c9a26aceb7a9bace0ec983b6b4780a91bdd15b87a0e5685fb5`.
Verification matched all 118 row identities and frozen gaps, confirmed equality of the other
112 rows, and checked the original bodies, keyframes, following decisions and input hashes.

The follow-up accuracy test needs unseen vehicle episodes and independently authored bumper
centres and orientation. It should include matching visible-end cases, contamination at the
required end, ambiguous direction, and clipped or occluded faces. Compare endpoint and yaw
agreement, coverage and bound width on the fixed sample cohort. Longitudinal agreement on this
shared-return passage alone provides no basis to change online DRAC geometry.

The [manual bumper and heading review](following-fresh-bumper-review-2026-11.md) checked
the same visibility rule on three fresh development passages and compared both fits with
independently authored endpoints and headings.
