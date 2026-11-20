# Facet anchors and lifecycle shell gap review

- **Scope:** Kirk0 development passage, offline proposals and saved reference comparisons
- **Decision:** retain conservative proposals; do not adopt joint anchors as online DRAC geometry

## Question and method

Can confirmed saved facets carry the same vehicle shell between frames, and how do the
lifecycle shells change follower-front to leader-rear gaps against the frozen reviewed
reference? This review uses the existing VelocityVisualiser annotation window, Physical
and Facets modes, saved-frame navigation, body-relative registration and joint pose
proposal. There is no second annotation store or viewer.

The paired comparison fixes both reviewed leader decisions and their entire sample
intervals from the baseline. It retains missing candidate keyframes and unsupported
endpoints. A missing candidate is an abstention, never a zero gap. The baseline is the
saved physical-reference document, not a fresh online replay. A difference from that
reference is not an independently established error reduction: fits and references
share LiDAR returns, and all reviewed cases remain development evidence.

No replay tuning parameters change. Lifecycle proposals are the retained fits from the
[three-site review record](lifecycle-following-three-sites-2026-11.md); no new parameter
arm or capture replay was needed for this comparison. Kirk0 is the reviewed passage,
separate from the three full-capture site exports in that record.

## Frozen provenance and preservation

The common pack digest is
`sha256:67842cb1b22b70e7dff46aa53af48405e44ddd69503d3c43a0c7670450a1507e`.
Membership revision 2233 has digest
`sha256:12b47778fdf9119e83d18e9e8e0ebeadaea1b52b5ba32b35441c83d8e8edfad8`.
Coordinates are the pack's metre-valued ENU sensor frame, not surveyed site coordinates.

The baseline is `kirk0/kirk0-reference/views/body-fit-qa`, physical revision 78. Its
physical-reference file SHA-256 is
`462aea22e62db5f34cde2c70faf562a00174055fb72e64320f19036a264f653d`.
All new edits are isolated in `kirk0/kirk0-reference/views/lifecycle-review-20261118`.
Three body fits were imported with all three replacement-keyframe lists, retaining
473 proposals and no old unmatched keyframes. An intermediate import omitted two
replacement lists; it was corrected before comparison and its receipt is retained.
That intermediate state is not an experiment arm.

Required raw evidence is retained under the attached LiDAR-backed APFS volume's
`lifecycle-following/facet-shell-accuracy` directory. Builds and caches use the approved
WD_BLACK-backed volume. Requests, responses, screenshots, test logs and comparison
rows remain local; this report records the findings and frozen identities.

## Confirmed facets in the existing VV workflow

The source truck is `obj_35c661db`, body `body_c961414d344f`. Its lifecycle body was
visually inspected, then reviewed at physical revision 83. Source pose 467 was reviewed
at revision 84. This confirms an agent's visual inspection of conservative bounds;
it is not surveyed geometry or a calibrated confidence judgement. The body is
10.697 m long with interval [9.651, 10.890] m, and 2.472 m wide with interval
[0.871, 2.916] m. Height remains a lower bound, at least 3.077 m.

Two named corners were selected and checked in the existing orthographic views:

| Saved feature                          | Support frames and return counts | Source registration at sample 467  | Body-relative forward / left | Anchor bound |
| -------------------------------------- | -------------------------------- | ---------------------------------- | ---------------------------- | ------------ |
| `2ab851f4-e332-491b-9e32-32c2f0436829` | 466: 21; 467: 31; 468: 32        | Return 16244; cab side upper joint | 4.07 / −0.51 m               | ±1.19 m      |
| `c7261428-3cf9-498c-8652-74ef87091a2b` | 467: 41; 468: 81                 | Return 592; rear side lower corner | −5.05 / −0.88 m              | ±1.20 m      |

The first support selection for each facet was manual. Subsequent point subsets used
object-mask translation proposals and were visually checked before saving. Each
registration declared a 0.20 m return-selection allowance. This allowance is an
operator input, not calibrated sensor noise. Both anchors share body and pose
uncertainty; their separation does not make their errors independent. Heights are
unresolved. Original stale features in the baseline were neither overwritten nor
silently rebound to current membership.

Feature revision 7 has digest
`sha256:bc0804bff07baff448481658a4757657a20315acce464d0c93cd51d92f9928fa`.
At target sample 468, returns 15786 and 662 were manually confirmed as the corresponding
cab joint and rear corner. The explicit prior holds source pose 467, rounded to
(−3.58, 3.88) m and 62.4°, with allowances of 2 m and 10° across the approximately
0.1-second interval. The target physical pose was not used as a prior or match input.
The anchor/body evidence itself is retrospective and shares information with the
lifecycle fit; this is not a held-out accuracy test.

The read-only joint endpoint returned in 76.015 seconds. It produced centre
(−3.530, 3.952) m ±3.395 m and yaw 62.122° ±17.769°, with data rank 3 and
`supported` position and yaw. Height remained unmeasured. The support state describes
constraint observability, not match probability. These bounds are broader than the
source lifecycle pose (±0.93 m and ±0.8°), so there is no basis to adopt this joint
proposal as a more accurate gap source. Broad source registration and selection
allowances dominate; tightening them without better observations would hide error.

## Endpoint review in VV

At sample 500, the existing Physical overlay was checked against raw labelled returns:

| Object and role                         | Proposed endpoint        | Bound and evidence                             | Review outcome                                    |
| --------------------------------------- | ------------------------ | ---------------------------------------------- | ------------------------------------------------- |
| Truck 2 `obj_8e829a06`, following car 8 | Front (−15.63, −15.59) m | ±1.12 m; front supported by this frame         | Retain proposal; broad endpoint bound             |
| Car 8 `obj_8c728991`, leading truck 2   | Rear (−5.94, 0.75) m     | ±1.50 m; model marks both ends observed        | Retain proposal; full body interval remains broad |
| Car 8, following truck 1                | Front (−3.72, 4.38) m    | ±1.50 m                                        | Retain proposal                                   |
| Truck 1, leading car 8                  | Rear (2.91, 16.80) m     | ±1.08 m; rear supported, hidden front inferred | Retain proposal; no pose approval at sample 500   |

The UI shows a real rear surface on truck 1 and returns reaching truck 2's front.
Those observations do not justify collapsing the displayed uncertainty circles.
No gap-period target poses were promoted to reviewed truth. This keeps source-anchor
review separate from evaluation of the following intervals.

## Gap comparison

Both reviewed leader intervals span samples 472–530 inclusive: 59 samples per pair,
118 pair/frame decisions. The baseline contains 11 scorable saved gaps, not a dense
reference at every sample. Candidate revision 84 has digest
`sha256:5e2c196d3ff34fc5279fbcd80bf17f254fd39f4d59aaa847d9d94e2633635669`.
Neither document has unpinned reviews. The comparison refuses review drift rather
than silently rebinding references.

| Pair            | Fixed cohort | Candidate gaps | Missing follower pose | Missing leader pose | Frozen gaps with candidate |
| --------------- | -----------: | -------------: | --------------------: | ------------------: | -------------------------: |
| Truck 2 → car 8 |           59 |             36 |                    14 |                   9 |                     3 of 6 |
| Car 8 → truck 1 |           59 |             20 |                     9 |                  30 |                     2 of 5 |
| Total           |          118 |             56 |                    23 |                  39 |                    5 of 11 |

Missing counts record the first missing endpoint in a row; both objects may lack
poses. Candidate coverage is 47.5% of this fixed interval, not population coverage.
All 11 frozen samples are retained below, including candidate abstentions. Values
are longitudinal gaps in metres along each version's follower heading. Brackets
are worst-case uncertainty intervals, not statistical confidence intervals.

| Pair            | Sample | Frozen gap [bounds] m   | Lifecycle gap [bounds] m | Candidate minus frozen m / abstention |
| --------------- | -----: | ----------------------- | ------------------------ | ------------------------------------- |
| Truck 2 → car 8 |    491 | 21.166 [17.307, 25.026] | 21.247 [15.244, 27.251]  | +0.081                                |
| Truck 2 → car 8 |    495 | 20.299 [19.151, 21.446] | 20.297 [17.162, 23.431]  | −0.002                                |
| Truck 2 → car 8 |    500 | 18.999 [18.071, 19.927] | 18.997 [16.060, 21.933]  | −0.002                                |
| Truck 2 → car 8 |    502 | 18.419 [17.270, 19.567] | Unavailable              | Follower pose absent                  |
| Truck 2 → car 8 |    504 | 17.897 [16.792, 19.002] | Unavailable              | Follower pose absent                  |
| Truck 2 → car 8 |    509 | 16.398 [15.488, 17.308] | Unavailable              | Follower pose absent                  |
| Car 8 → truck 1 |    491 | 11.608 [8.984, 14.232]  | Unavailable              | Leader pose absent                    |
| Car 8 → truck 1 |    495 | 12.627 [9.979, 15.275]  | Unavailable              | Leader pose absent                    |
| Car 8 → truck 1 |    500 | 14.077 [13.107, 15.048] | 14.050 [11.283, 16.817]  | −0.028                                |
| Car 8 → truck 1 |    504 | 14.998 [13.158, 16.839] | 14.838 [11.206, 18.469]  | −0.161                                |
| Car 8 → truck 1 |    509 | 15.924 [14.000, 17.848] | Unavailable              | Leader pose absent                    |

Across the five paired samples, mean absolute change is 0.055 m and signed change
ranges from −0.161 to +0.081 m. This is agreement with the frozen reference, not
mean absolute physical error. Every paired candidate interval is wider, by factors
of 1.56–3.16. At truck 2 sample 491 the lifecycle model marks the leader's rear as
observed where the frozen reference inferred it; that is a model evidence claim to
review, not an upgraded physical label. It has not been accepted as truth.

The Make-built CLI reports version `v0.5.1-pre52` and source commit
`3c98dc8d807afea4f2f6e79ff9090fe927587bd4`. Two independent executions produced
byte-identical diagnostics, SHA-256
`c2141a9a25d932a508d5f2871ca059f5ec7b570cd4ee0fb8d401c7b031f14736`.
Input file hashes were identical before and after both runs. The comparison shares
the established gap/bound calculation and never imports, reviews or adopts a pose.
Its new implementation file passed all 45 statement checks at 100% coverage.

## Engineering checks and next work

The workflow exposed a native request timeout: two joint requests returned HTTP 200
on the server after 75.583 and 62.972 seconds, after VV's default one-minute timeout.
The client change gives both single and joint requests the same bounded five-minute
budget used by following-review evidence requests. It changes neither source checks
nor solver mathematics, and adds no automatic retries or reference adoption.
A restarted test build completed the native joint request in 114.989 seconds and
visibly displayed both matched anchors, the shell overlay and its broad bounds.
The 17-test native build used the first comparison commit plus the timeout fix;
the committed fix is `ddf9d0115de8a3c9a85e638ff9a8d0132aa605e0`.

The full annotation package passed locally. Two real-capture CLI tests failed:
`TestAnnotationClipOneCommandCutsKirk0PackAndRecord` and
`TestReplayEvalExperimentOnKirk0`. Diagnostic reruns exposed the observation writer
correctly enforcing its durability deadline: records remained uncommitted after
3.487 and 2.101 seconds respectively against the declared 2.1-second bound.
Diagnostic stderr remains on attached storage; the safety gate was not weakened.
Targeted comparison tests passed separately. All 17 selected native tests passed,
including the timeout and request-schema regression.

Before automatic propagation, independently review a few stronger source anchors and
both endpoint positions across the same paired intervals. Retain at least two separated
anchors, challenge each correspondence with neighbouring returns and occlusion, and
abstain when support is weak. Re-run the fixed-cohort comparison with the original
reference frozen. Check endpoint error, heading and relative speed separately; fewer
missing gaps or narrower intervals alone do not establish physical accuracy. Plane-only
constraints must remain partial, and full-shell height remains unresolved in this
point-anchor diagnostic. The next campaign must include independently reviewed or
controlled physical references before claiming a DRAC accuracy improvement.
