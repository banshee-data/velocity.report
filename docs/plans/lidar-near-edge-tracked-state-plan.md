# Near-edge tracked state (0.5.2 S2)

- **Status:** Proposed
- **Layers:** LiDAR pipeline (L4 members, L5 tracker, L8 adapter, storage, replay tools)
- **Target:** v0.5.2, Sprint 0.5.2.1; S2 of the [MVP sprint plan](lidar-052-mvp-sprint-plan.md)
- **Companion plans:** [state estimation](lidar-state-estimation-plan.md) (Phase 2, Sections 5.3, 8.1, 9.1 and G-GEO-1), [VRLOG observation format](lidar-vrlog-observation-format-plan.md)
- **Canonical:** [LiDAR pipeline reference](../lidar/architecture/lidar-pipeline-reference.md)

## Motivation

The tracked filter still measures the medoid, so every persisted point estimate refers to a
point in the cluster, and since #618 the field run fits no follower path from them. The near-edge
solid body places the body centre, but only as a shadow: it never feeds association, seeding or
the tracked state, so identity, coasting and every downstream consumer still run on the medoid.
S2 makes the near-edge measurement the tracked filter's measurement, at the rank it has, without
changing the motion estimator (Section 5.3, Option A), so a G-GEO-1 result stays attributable to
the observation model.

The consequence of stopping at the shadow is two filters that disagree about where each vehicle
is, with the one that decides identity being the biased one.

## Current state

| Area                  | Today                                                                                                                                                                  |
| --------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Tracked measurement   | `measurementForCluster`: medoid (`medoid_v0`, production) or OBB centre (`obb_centre_v1`, opt-in), a 2-D update with isotropic or adaptive R                           |
| Near-edge measurement | `MeasureNearEdge`: one scalar constraint per supported face along its normal, rank 1 or 2; used only by the shadow (`solid_body_nearedge.go`)                          |
| Shadow state machine  | Medoid seed; medoid updates for `HitsToConfirm` frames; first fix re-references to the body centre with covariance widened by the medoid bias; lapse after `MaxMisses` |
| Association           | Mahalanobis of the medoid (or OBB centre) against the tracked prediction, plausibility checks, extent compatibility, fragment guard, exact Hungarian (#600)            |
| Sensor origin         | `SolidBodyOptions.SensorX/Y`, left at (0, 0) because replay tracks in the sensor frame; nothing declares or checks it                                                  |
| Member geometry       | `WorldCluster.RetainedPoints`, a uniform subsample capped by `MaxSamplePoints` (at most 1024); the kirk0 field-run test uses 16, the corpus tool 256                   |
| Persisted reference   | Inferred from `measurement_source` per point-estimate row (#618); the solid-body table stores it per row                                                               |
| Refined stages        | The RTS smoother revises point estimates only; no solid body at `fixed_lag` or `final`                                                                                 |
| Evidence oracle       | The lossless-batch oracle does not cover `lidar_track_solid_bodies`                                                                                                    |

### What the shadow measures on kirk0

S2.0's summary on kirk0 (256 sample points, 1,952 solid bodies beside 1,952 point estimates):

| Rows                            |  Count | Note                                                                             |
| ------------------------------- | -----: | -------------------------------------------------------------------------------- |
| Near-edge fixes                 |    826 | 42 %; rank 2 on 586 of them                                                      |
| No face reached minimum support |    586 | Samples under 122 points (T0); a 16-point cap loses more: 381 rows on the centre |
| Face found, extent only a prior |    454 | Excluded by Section 8.1 until the extent is evidence-backed                      |
| Initialisation window, lapses   | 53, 33 |                                                                                  |

Five-point lateral residual over moving tracks, on the frames where the body is on its centre:

| Stratum                                  | Windows | Point p95 / p99 / max | Body p95 / p99 / max  |
| ---------------------------------------- | ------: | --------------------- | --------------------- |
| All body-centre frames                   |     148 | 0.160 / 0.309 / 0.315 | 0.071 / 0.364 / 0.490 |
| Steady runs (split at reference changes) |     148 | 0.160 / 0.309 / 0.315 | 0.071 / 0.364 / 0.490 |
| Face-stable runs (split at face changes) |     111 | 0.109 / 0.178 / 0.267 | 0.035 / 0.083 / 0.102 |

Within a run of fixes on the same faces the body's p99 is less than half the point estimate's; the
whole p99 tail is in windows where the set of faces changes. Reference changes cost nothing on
kirk0. Face changes are common: 48 steady runs break into 210 face-stable runs. The likely
mechanism is the one the shadow's own header names: a face constrains the centre through a
believed half-extent, whose error is a bias the filter cannot see, so a face appearing or
disappearing moves the centre by that error. Feeding the tracker before fixing this would carry
the tail into association and every persisted estimate.

### What T0 shows on three corpus sites

T0 ran the shadow at 256 and 1,024 sample points on the tuning and held-out cases, on the Mac:
Apple M1 Pro, 25 minutes and 1 GB peak memory per arm for the three cases. The tables and summaries
are on `claude/upbeat-galileo-4xbaat-s2-t0-results`, under `results/s2-t0/`. Five-point lateral
residual p99 in metres, at 256 sample points:

| Case                   | Partition | Fix share | Steady → face-stable runs | Body-centre frames: point / body | Face-stable runs: point / body |
| ---------------------- | --------- | --------: | ------------------------- | -------------------------------- | ------------------------------ |
| `marina-webster-beach` | Tuning    |      37 % | 573 → 3,604               | 0.194 / 0.231                    | 0.153 / 0.074                  |
| `columbus-broadway`    | Tuning    |      52 % | 1,817 → 7,774             | 0.254 / 0.388                    | 0.178 / 0.193                  |
| `embarcadero-folsom`   | Held out  |      55 % | 776 → 4,185               | 0.245 / 0.291                    | 0.210 / 0.182                  |

**The sample cap does not limit the fix rate.** At 1,024 points, these are identical to the 256
arm:

- fixes, fallbacks and lapses;
- steady and face-stable run counts;
- every point-estimate figure.

Only the body moves:

- its p95 and p99 residuals, by under a centimetre;
- its maximum, by up to 5 cm;
- one more converged width on embarcadero.

The rule explains why:

- The face plane is the nearest-rank 95th percentile of the member projections along its normal.
- Every member at or beyond the plane counts as support.
- So a 256-point sample always has 14 supporting returns, and any sample of 122 or more has at least
  the 8 required.

Every row without a supported face therefore had fewer than 122 sampled members. Kirk0's 586 such
rows are small clusters, not the cap. Full member geometry (S2.1) will not raise the fix rate; its
case is keeping the measurement independent of a persistence setting, which a 16-point cap does
break.

**Two things limit the fix rate:**

| Case                   | No face with minimum support | Face found, extent only a prior | Tracks whose width converged |
| ---------------------- | ---------------------------: | ------------------------------: | ---------------------------: |
| `marina-webster-beach` |                         48 % |                            12 % |                         34 % |
| `columbus-broadway`    |                         24 % |                            20 % |                         14 % |
| `embarcadero-folsom`   |                         16 % |                            26 % |                         22 % |

The first two figures in each row are shares of solid-body rows; the third is a share of tracks.

**Face transitions carry the tail on every site, as on kirk0.**

- Over all body-centre frames, the body's p99 is 19 % to 53 % above the point estimate's.
- Its excursion share is 2 to 3.5 times the point estimate's.
- Face changes are common: steady runs break into 4 to 6 times as many face-stable runs.

**Within face-stable runs the body wins on two sites of three.**

- At p95 it wins everywhere: 0.036 against 0.091 m, 0.082 against 0.096 m, and 0.079 against
  0.126 m.
- At p99 it wins on marina and embarcadero.
- On columbus-broadway it loses at p99 even within face-stable runs, so there is a second tail that
  face stability does not explain. Columbus is the busiest site, with 2,671 tracks against 559
  and 946. Heading lag on turns (see Risks) is the first thing to stratify by.

**The shadow never touched the tracker.** The point estimates are the same in both arms, so they
double as B0's point figures for these cases.

## Design

### Invariants

1. **Default unchanged.** A Go-level `TrackerConfig.NearEdgeTracking` option, reached by the
   replay experiment `near_edge_track`, off by default. The default replay stays byte-identical
   and the tuning fingerprint does not move.
2. **Rank respected.** The update is one scalar Joseph-form update per supported face along its
   normal. A rank-one frame moves nothing across its normal, and no reconstructed centre is ever
   fed as a 2-D observation.
3. **A reference change is a translation, not an innovation.** Moving from the medoid to the body
   centre (first fix) or back (lapse) shifts position by the geometric offset measured in that
   frame and widens position covariance; velocity is untouched, and the event is recorded.
4. **Like with like.** Association compares a prediction and a measurement of the same point.
5. **No declared origin, no near-edge.** The sensor origin comes from a declaration or from the
   calibration transform the tracker's frame was built with, never from a default. Without one
   the mode refuses, as the continuity experiments refuse without coverage (#617).
6. **Full members for measurement, capped samples for persistence.** The tracker measures faces
   from every member of the associated cluster for the frame it is in. The persisted sample keeps
   its cap and records it. The reason is independence from a persistence setting, not fix rate:
   T0 found a 256-point sample already serves every face that full members would.
7. **Explicit per-row reference and support.** Every persisted estimate states its reference
   point and support token; no reader infers either.

### One state machine for shadow and tracked

The shadow's logic (initialisation window, re-reference, faceless coast, lapse, extent admission)
moves out of `solidBodyTrack` into a function over a `(state, P, reference, support)` value. The
shadow calls it on its own filter; `NearEdgeTracking` calls it on the tracked filter, and the
solid body then reads the tracked state instead of keeping a second one. With both on, the shadow
is redundant and is not run. This keeps the behaviour G-GEO-1 measures identical between the
shadow arm and the tracked arm, so the difference between them is association and identity alone.

### Association

A body-centre track is gated per candidate cluster against a measurement of the same point. Two
arms, chosen on the tuning partition:

| Arm                         | Gate and cost                                                                                                                                                                                                                                                                                                             |
| --------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| A1, ablation                | Medoid against the predicted centre with the tracked R: the do-nothing option, biased by half a body across                                                                                                                                                                                                               |
| A2, face residual (default) | `MeasureNearEdge` for the pair with the track's heading and extents. Constrained normals use the face residual and R; any unconstrained direction uses the medoid residual projected onto it with R plus the believed half-extent squared. Always 2 DOF, so the chi-square gate and `GatingDistanceSquared` are unchanged |

A medoid-referenced track (initialisation window, or after a lapse) is gated on the medoid as
today. A body-centre track whose pair yields no face uses the medoid in both directions with the
loose variance. The loose term is association evidence only; the update never uses it (invariant
2). Plausibility checks, extent compatibility and the fragment guard are unchanged. The pair's
near-edge result is kept and reused by the update, so each associated cluster is measured once.

### Face transitions

The half-extent behind a face is an accumulated lower bound (Section 9.2) with its own sigma, and
its error enters the implied centre one-for-one. Section 8.1 forbids widening R to cover it on
every frame, because that would lag a real manoeuvre. Two remedies, compared on the tuning
partition by the face-stable-versus-all residual gap:

| Remedy                   | Change                                                                                                                                                                         |
| ------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| T1, admission hysteresis | A face enters the fix only after it is supported on two consecutive frames and leaves after two frames without, so a face at the support threshold stops flickering in and out |
| T2, consider on entry    | The first update from a face that was not in the previous fix carries the half-extent's variance as a consider term (R plus sigma squared); later updates carry R alone        |

Neither changes the rank rule or the steady-state R. The gate is the gap closing: all-frame p99
within 25 % of the face-stable p99 on the held-out case.

S2.1 measured both (see [What S2.1 found](#what-s21-found)). T1 helps but leaves the all-frame
p99 about 2.5 times the face-stable p99 on every site. T2 changes nothing on kirk0, because the
half-extent's error is a bias and softening one update only delays it.

### Seeding and re-reference

A new track is seeded at the medoid with today's covariance and is medoid-referenced. After
`HitsToConfirm` observations with a resolved heading and evidence-backed extents, the first fix
translates the position by the offset between the frame's reconstructed centre and its medoid
(along the constrained normals only, for a rank-one fix), widens position covariance by the
medoid-bias variance, then applies the face updates. A body-centre track that goes `MaxMisses`
associated frames without a usable face lapses: its position is translated back to the medoid,
covariance widened, velocity kept.

### Sensor geometry declaration

`ContinuityCoverage` (#617) already carries a sensor origin beside coverage. It becomes the sensor
geometry declaration used by both the continuity experiments and `near_edge_track`, with the same
validation, content id and manifest record. The live pipeline derives the origin from the
calibration transform used by `TransformToWorld`: identity today, so (0, 0) is exact and recorded
as such rather than assumed.

### Persistence and the adapter

- `lidar_track_estimates` gains `reference_point` and `support_instant` (a migration, with old rows
  backfilled from `measurement_source`, as #618's adapter infers them today). The adapter reads the
  columns and stops inferring.
- Under `near_edge_track`, a point estimate on a body-centre frame is `body_centre`, so the field run
  fits paths from point estimates as well as from `--solid-bodies`, and the two agree on position.
- Refined stages: the RTS smoother ends a chain at a reference change (`reference_changed`), and
  writes a solid-body row per refined estimate with the revised state and the online beliefs.
- The lossless-batch oracle covers `lidar_track_solid_bodies`.

### Configuration surface

Everything above is reached one way: a replay `-experiment` name sets a field on
`TrackerConfig.SolidBody` in `replayeval`. The corpus tool, the pcap tests, the per-frame
evaluator and the headway field run can therefore compare arms; nothing that builds a tracker from
the tuning config can. `velocity serve`, PCAP replay on :8081, the sweep, HINT, the run record and
the visualiser have no way to switch the body on, so no VRLOG or analysis run carries it, and the
tuning fingerprint does not know it exists.

The options move into the tuning config as one block, `l5.cv_kf_v1.solid_body`, with these rules:

| Rule                                   | Why                                                                                                                                                                                                                                                                                                                     |
| -------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| An options block, not an engine        | The body does not change the motion model (Option A). Engines select estimators (state plan, Section 4.3); a `cv_kf_near_edge_v1` engine would copy `L5Common` and fork every sweep, runtime and HINT path that reads it                                                                                                |
| A pointer, `omitempty`, nil by default | `tuning.defaults.json` and the profiles are byte-identical, so the fingerprint does not move (invariant 1) and the perf baselines stand. The unselected engine blocks already take this shape, and the key-order check accepts their absence                                                                            |
| Experiments become aliases             | `solid_body`, `solid_body_face_hysteresis`, `solid_body_course_faces` and the rest set the same block in replay. F1 to F3 stay reproducible from their manifests, and the summary's `experiments` list keeps its meaning                                                                                                |
| One mapping                            | `TrackerConfigFromTuning` reads the block; the runtime apply path (`/api/lidar/params`) gains its keys under `l5.cv_kf_v1.solid_body.*`; the run record and manifest hash it with everything else                                                                                                                       |
| Live parity or refusal                 | The live pipeline hands the tracker capped `RetainedPoints` and no declared origin. Full members need the same L4 hand-off replay uses (S2.1 step 2); the origin comes from the calibration transform and is recorded (Sensor geometry declaration). Until both hold, the block refuses on the live path as replay does |

Fields: `enabled`, `face_hysteresis`, `face_entry_consider`, `course_aligned_faces`,
`course_alignment_min_speed_mps` (2.0 today, a constant) and, when S2.2 lands, `near_edge_track`.
The numeric ones become sweepable; the booleans are arms.

What this buys the sprint: a run of a real capture through the real server with the body on, so
the macOS tool can review it against the kirk0 pack, the visualiser can draw it, and
`lidar_track_solid_bodies` fills from a run rather than a test. What it does not buy: a score on
:8081. The sweep, auto-tune and HINT score `lidar_run_tracks`, which is the tracked state, and
that state stays on the medoid until S2.2. Until then the body's A/B harness is offline: the corpus
tool's per-case summary (label-free), the per-frame evaluator's paired arms with `-a-solid-body`
and `-b-solid-body` on reviewed held-out episodes, and the headway field run with
`--solid-bodies`. After S2.2 the body is the tracked state, and every :8081 harness scores it
without change.

## Scope

### S2.0: corpus instrumentation for the shadow

**Summary:** Make the corpus tool report what the kirk0 pcap test logs, so the observation-model
result can be measured per site before the tracker changes.

**Steps:**

1. Move the kirk0 test's anchor-stability and row-count analysis into
   `replayeval.SummariseSolidBodies`, reading the observation database, and add a `solid_body`
   block to each case summary: fix rate, rank and face mix, fallback reasons, lapses, extent
   convergence, and the five-point lateral residual of point estimates and body-centre solid
   bodies over the same track-frames, on all body-centre frames, steady runs and face-stable runs.
2. Add `-sample-points` (today fixed at 256), `-evidence-per-case` and `-discard-evidence` to
   `lidar-state-estimation-baseline`, so a corpus larger than the free disk runs case by case.

**Exit:** the corpus summary carries the block; the kirk0 figures reproduce #614's. Done: the
strata above are its first output.

### S2.1: face transitions, full member geometry and a declared origin

**Summary:** Close the face-transition tail in the shadow, measure faces from every member, and
refuse without an origin. All of it is in the shadow, where G-GEO-1 is attributable.

**Steps:**

1. T1 and T2 as options on the shadow; S2.0's strata report both on kirk0 and the tuning sites.
2. L4 hands the tracker each cluster's full member points for the frame (not retained, not
   persisted); `RetainedPoints` stays the capped persistence sample.
3. `SolidBodyOptions` takes its origin from the sensor geometry declaration or the calibration
   transform, and refuses without one; the manifest and hash record it.
4. Stratify the face-stable residual by heading rate and range on `columbus-broadway`, where the
   body loses to the point estimate at p99 even within face-stable runs (T0).
5. Move the options into the tuning config as `l5.cv_kf_v1.solid_body`, with the experiments
   as aliases and live parity or refusal (see [Configuration surface](#configuration-surface)).

**Exit:**

- The chosen remedy closes the all-frame p99 to within 25 % of the face-stable p99 on the tuning
  partition.
- Columbus's within-run tail has a named cause.
- Fixes and fallbacks with full members match B1-256.
- Refusal is tested.

#### What S2.1 found

The code is in replay behind five experiments that qualify `solid_body`:

- `solid_body_face_hysteresis` (T1);
- `solid_body_face_consider` (T2);
- `solid_body_course_faces` (T3), on `claude/upbeat-galileo-4xbaat-s2-1-t3` at `0adb33e5`, not
  yet merged;
- `solid_body_full_members`;
- the sensor origin, which `solid_body` now requires. Without a declaration it is derived from the
  identity tracking transform and recorded in the manifest.

Three tests ran on the Mac (Apple M1 Pro, 20 to 26 minutes and under 1 GB per arm), each on the
tuning partition at 256 sample points, two arms at a time: F1 (T1, T2, both, without full
members), F2 (full members, with and without T1; `claude/upbeat-galileo-4xbaat-s2-f2-results`)
and F3 (T3 alone and with T1, full members; #623). Full members change nothing but the
independence from the cap (below), so the F1 rows compare with the rest. kirk0 ran every arm in
the pcap test on the T3 branch. Solid-body lateral residual p99 over moving tracks, in metres,
with the point estimate's p99 over the same body-centre frames beside it:

| Site                   | Arm        |  Fixes | Lapses | Body-centre frames | Face-stable runs | Point, body-centre frames | Gap |
| ---------------------- | ---------- | -----: | -----: | -----------------: | ---------------: | ------------------------: | --: |
| kirk0                  | solid body |    825 |        |              0.364 |            0.087 |                     0.309 | 4.2 |
| kirk0                  | T1         |    698 |        |              0.216 |            0.087 |                     0.303 | 2.5 |
| kirk0                  | T1, T2     |    698 |        |              0.220 |            0.084 |                     0.303 | 2.6 |
| kirk0                  | T3         |  1,110 |        |              0.209 |            0.132 |                     0.309 | 1.6 |
| kirk0                  | T1, T3     |    948 |        |              0.531 |            0.058 |                     0.266 | 9.2 |
| `marina-webster-beach` | solid body | 14,557 |    402 |              0.231 |            0.073 |                     0.194 | 3.2 |
| `marina-webster-beach` | T1         | 13,570 |    587 |              0.159 |            0.064 |                     0.180 | 2.5 |
| `marina-webster-beach` | T2         | 14,457 |    406 |              0.221 |            0.077 |                     0.194 | 2.9 |
| `marina-webster-beach` | T1, T2     | 13,570 |    587 |              0.169 |            0.069 |                     0.180 | 2.4 |
| `marina-webster-beach` | T3         | 18,351 |    514 |              0.182 |            0.069 |                     0.198 | 2.6 |
| `marina-webster-beach` | T1, T3     | 16,785 |    674 |              0.154 |            0.065 |                     0.183 | 2.4 |
| `columbus-broadway`    | solid body | 40,630 |    898 |              0.386 |            0.194 |                     0.254 | 2.0 |
| `columbus-broadway`    | T1         | 37,130 |  1,363 |              0.344 |            0.136 |                     0.197 | 2.5 |
| `columbus-broadway`    | T2         | 40,630 |    898 |              0.336 |            0.198 |                     0.254 | 1.7 |
| `columbus-broadway`    | T1, T2     | 37,130 |  1,363 |              0.307 |            0.135 |                     0.197 | 2.3 |
| `columbus-broadway`    | T3         | 47,664 |  1,039 |              0.401 |            0.164 |                     0.270 | 2.4 |
| `columbus-broadway`    | T1, T3     | 43,294 |  1,680 |              0.287 |            0.139 |                     0.228 | 2.1 |

The point estimate's figure moves between arms because the set of body-centre frames does; the
tracker itself is identical in every arm.

Against the exit:

- **Remedy: not met by T1 or T2.** T1 costs 7 % to 9 % of fixes and adds about half again as many
  lapses, because a held frame is a faceless one, and leaves the gap near 2.5 on every site. T2
  changes nothing on kirk0 or marina; on columbus it takes the all-frame p99 from 0.386 to 0.336 m
  at no cost in fixes, so a face's first update does carry some of that site's tail. Together they
  are no better than T1 alone.
- **Columbus's within-run tail: named, then closed by T3.** It is turning. With T1 alone, at 15
  degrees per second or more the body's face-stable p99 is 0.369 m against the point estimate's
  0.152 m, over 490 of 2,563 windows; below that the body is level with or better than the point.
  Marina shows the same above 15 degrees per second, 0.204 against 0.115 m, on 242 windows. The
  tracked heading lags a turn, and the face normal and the face choice lag with it: the state
  plan's invalidating condition b. Range shows no pattern inside 40 m. T3 takes the face choice
  and normals from the body's own course above 2 m/s: in that stratum the body's face-stable p99
  falls to 0.143 m on columbus (0.136 with T1, against the point's 0.178) and 0.069 m on marina,
  over three to four times as many windows (1,568 and 936), because the turning frames now fix.
  With T3 the body beats the point estimate in every heading-rate stratum on both sites.
- **T3 raises the fix rate, through extent admission.** Fixes rise 26 % on marina and 17 % on
  columbus, and 34 % on kirk0. The rows that had a face but only a prior extent fall from 4,538 to
  1,494 and from 15,337 to 8,857: extent admission already refused spans when the tracked heading
  and the course disagreed by more than 10 degrees, and measuring along the course admits them,
  so widths converge on 239 and 482 tracks instead of 190 and 365. The clusters with no supported
  face are unchanged, as T0 predicted.
- **The transition tail stays open.** T1 with T3 is the best corpus arm on every figure: 0.154 m
  on marina and 0.287 m on columbus over all body-centre frames, at 2.4 and 2.1 times the
  face-stable p99, against the exit's 1.25. Over all body-centre frames the body beats the point
  on marina (0.154 against 0.183 m) and still loses on columbus (0.287 against 0.228 m), with the
  deficit halved from T1's. Excursion share rises with T3 (marina 0.036 to 0.054, columbus 0.043
  to 0.054; 0.043 and 0.037 with T1), but so does the point estimate's over the same frames
  (marina 0.018 to 0.043): the population now includes the admitted turning frames.
- **kirk0 disagrees about T1 with T3.** There T3 alone is the best all-frame arm (0.209 m, gap
  1.6) and T1 with T3 the worst (0.531 m, maximum 0.708 m, from 197 windows). A held face and a
  course-aligned normal interact somewhere in kirk0's 63 s in a way the corpus sites do not show,
  and the pcap test asserts nothing that catches it. Those frames want looking at before T1 with
  T3 is frozen.
- **Full members: met.** Fixes, every fallback, lapses, converged widths and run counts are
  identical to T0's 256-point arm on both sites. p95 and p99 move by 2 mm or less.
- **Refusal: met.** A solid body without a declared origin makes no fix and says
  `missing_calibrated_sensor_origin` on every row (unit test).

#### Decision: the S2.1 configuration

T3 was the proposed next remedy and has now run. The options as they stand:

1. **Freeze T1 with T3 as B1-T (recommended).** It is the best arm on both tuning sites and the
   only one that beats the point estimate in every heading-rate stratum. The exit's 25 % is not
   met, and this records that rather than relaxing it: what remains of the all-frame tail is the
   half-extent bias itself, which no admission rule removes. Before freezing, inspect kirk0's T1
   with T3 excursion (197 windows; one evening), then score `embarcadero-folsom` once with the
   frozen configuration (F4), as the protocol requires.
2. **T3 alone.** Best on kirk0 and loses nothing on fixes, but the corpus says T1 buys 15 % to
   28 % of the all-frame p99 on top of it. Fall back here if kirk0's excursion is real and T1's.
3. **A per-face bias state.** Still the lever for the remaining gap, still closer to Option B, and
   still out of scope here. It moves to S3 beside adaptive R, where the half-extent's sigma is
   already load-bearing.

The remaining gap is carried into S2.2 as a measured starting point, not hidden by it: gate 2 is
scored on the held-out case against B0, and B1-T's held-out figure from F4 says in advance whether
the tracked arm can pass it.

### S2.2: the tracked near-edge update

**Summary:** `near_edge_track` updates the tracked filter through the shared state machine, with
A2 association.

**Steps:**

1. Extract the state machine; the shadow runs through it byte-identically (the kirk0 solid-body
   pcap test is the check).
2. Reference changes as translations, with the event recorded and counted.
3. A2 association, reusing the pair's measurement in the update.
4. Unit tests: rank-one moves nothing across its normal; a re-reference does not change velocity;
   a synthetic lane change keeps at least 90 % of its magnitude (G-GEO-1 row 4, synthetic); the
   K4 and S2 identity scenes still pass.

**Exit:** default replay byte-identical; kirk0 arm runs with a deterministic repeat.

### S2.3: the A1 ablation arm

A1 behind a second experiment, run once on the tuning partition to show what A2 buys.

### S2.4: persistence, refined stages and oracle

Per-row reference and support columns; smoother chain breaks and refined solid-body rows;
oracle coverage; field run over point estimates under `near_edge_track`.

## Evaluation

The corpus runs on a machine with the S2 archive mounted, never in CI. Two are set up:

| Host         | Captures (`-pcap-root`, `-pcap-subdir s2`) | Working directory                | Limits                                                                                                       |
| ------------ | ------------------------------------------ | -------------------------------- | ------------------------------------------------------------------------------------------------------------ |
| arrow-worker | `/mnt/captures/lidar` (NFS, read-only)     | `/srv/banshee/evidence/<run>/`   | 8 cores, 5.8 GB memory, about 17 GB free disk; `/tmp` is a 2.9 GB tmpfs                                      |
| The Mac      | `/Volumes/lidar/lidar`                     | `$HOME/<test>/`, off that volume | Apple M1 Pro, 16 GB; the platform the sprint accepts on. T0 took 25 minutes and 1 GB per arm for three cases |

Neither has an annotation pack, so every figure here is label-free. The protocol assumes the
smaller host:

- Cases run one at a time. Outputs and evidence go to a working directory off the capture volume.
- An evidence database is summarised into the case summary (S2.0) and then deleted. kirk0's 63
  scored seconds wrote 60 MB, so a whole corpus arm would need about 28 GB; the summaries are what
  is kept, and a case is re-run if its rows are needed again.
- The tool's determinism repeat stays on. It replays without the database, so it costs time, not
  disk.
- Results are committed as JSON, with a markdown table per arm, to a results branch per test under
  `results/<test>/`, and pushed. They can then be read and reviewed away from the machine.
- T0's branch is `claude/upbeat-galileo-4xbaat-s2-t0-results`; F2's is
  `claude/upbeat-galileo-4xbaat-s2-f2-results`; F3's is #623. F1's figures are in
  [What S2.1 found](#what-s21-found); its raw results are on the Mac and not yet pushed.

Test T0, the first run, was the shadow at 256 and 1,024 sample points on the tuning and held-out
cases (arms B1-256 and B1-1024 below), before any S2.1 change. It asked how much of the fix rate
and the face-transition tail is the sample cap. The answer is none of either; see
[What T0 shows](#what-t0-shows-on-three-corpus-sites).

| Partition | Cases                                       | Use                                                    |
| --------- | ------------------------------------------- | ------------------------------------------------------ |
| Tuning    | `marina-webster-beach`, `columbus-broadway` | Choose between A1 and A2 and any face-support setting  |
| Held out  | `embarcadero-folsom`                        | Scored once with the frozen choice                     |
| Screen    | The other 21 sites                          | Label-free regression screen; nothing is tuned on them |

| Arm     | Configuration                                                   | Stage |
| ------- | --------------------------------------------------------------- | ----- |
| B0      | Default, `medoid_v0`                                            | S2.0  |
| B1-16   | `solid_body`, 16 sample points                                  | S2.0  |
| B1-256  | `solid_body`, 256 sample points                                 | S2.0  |
| B1-1024 | `solid_body`, 1,024 sample points; T0: the same fixes as B1-256 | S2.0  |
| B1-F    | `solid_body`, full members                                      | S2.1  |
| B1-T    | `solid_body`, full members, T1 and T3 (proposed; F4 scores it)  | S2.1  |
| S2a     | `near_edge_track`, A2, full members                             | S2.2  |
| S2x     | `near_edge_track`, A1, full members                             | S2.3  |

Per case and arm: fix rate, rank mix and fallbacks; lapses and re-references; lateral residual
p50/p95/p99/max and excursion share over moving tracks (five-point fit), on all body-centre frames,
steady runs and face-stable runs; NIS mean per rank;
confirmed tracks, births per confirmation, mean confirmed duration, association rate and
gate-rejection rate; tracker time per frame p50 and p99.

**Gates for S2 (label-free, necessary, not sufficient for promotion):**

1. B0 is byte-identical to main, and every arm's repeat is byte-equal.
2. On the held-out case, S2a's p99 lateral residual is at most half of B0's, and its excursion
   share is under 4 % (G-GEO-1 rows 1 and 2, label-free).
3. S2a is within 5 % of B0 on confirmed tracks, births per confirmation and mean confirmed
   duration on the held-out case, and no screen site regresses by more than 10 % on any of them.
   D2's OBB centre improved bias and lost identity; this gate is what catches a repeat.
4. Mean NIS per face within [0.5, 2] of its rank on the tuning partition, or R is revisited in S3
   rather than here.
5. S2a's tracker time p99 within 1.5x of B0's.

G-GEO-1 rows 3 and 4 on reviewed data, and identity on held-out labels, wait for the frozen
reviewed split (S0, still open). Until then S2 stays default-off and provisional.

## Risks

- **Identity.** The face residual depends on the track's heading and extents; a wrong belief can
  gate out the right cluster. Mitigation: gate 3, and the loose medoid term across unconstrained
  directions.
- **Heading lag on turns** tilts the face normal (the state plan's invalidating condition b).
  Stratify the residual by heading rate before drawing conclusions. It is the first suspect for
  columbus-broadway's within-run tail.
- **Gate 2's reach.** On embarcadero-folsom, even the face-stable body p99 (0.175 to 0.182 m) is
  above half the point estimate's over body-centre frames (0.123 m). So closing the transition
  tail alone does not pass gate 2; the within-run tail must shrink too. The five-point residual
  measures lateral jitter about a local fit, not bias: the medoid's bias is smooth and does not
  show in it, so the gate compares steadiness only.
- **Fix rate.** On the T0 sites, 37 % to 55 % of rows are fixes. Most of the rest are clusters too
  small to support a face (under 122 sampled members) or extents that are still priors (width
  converges on 14 % to 34 % of tracks). S2 changes neither, so a tracked arm still leaves 45 % to
  63 % of updates to the medoid; extent admission is the lever if that proves to matter.
- **Rank-one drift** along the unconstrained direction for long lateral-only runs. It is the
  prediction, not a defect, but the report counts rank-one runs and their length.
- **Cost.** A near-edge measurement per candidate pair. Pairs pass the Euclidean plausibility check
  first; runtime is gate 5.

## Out of scope

Option B (nonlinear state); adaptive per-face R (S3, G-UNC-1); point-to-model residuals;
revisable association (S4 and later); a new default, which waits for labelled G-GEO-1.

## Checklist

- [x] S2.0 corpus solid-body block, strata, `-sample-points` and per-case evidence
- [x] T0: B1-256 and B1-1024 on the tuning and held-out partitions, on the Mac
- [ ] S2.0 B0 and B1-16 on the tuning and held-out partitions, then the screen
- [x] S2.1 T1 and T2 on the solid body; heading-rate and range strata
- [x] S2.1 full member geometry to the tracker; declared origin with refusal
- [x] F2: full members, with and without T1, on the tuning partition, on the Mac
- [x] F1: T1, T2 and both on the tuning partition, on the Mac (figures in the plan; raw results
      not pushed)
- [x] S2.1 T3, course-aligned faces, on the solid body (branch, unmerged)
- [x] F3: T3 alone and with T1 on the tuning partition, on the Mac (#623)
- [ ] S2.1 face-transition remedy frozen: T1 with T3 recommended; inspect kirk0's T1-with-T3
      excursion first; the exit's 25 % is not met on any site and is recorded as such
- [ ] F4: `embarcadero-folsom` once with the frozen B1-T
- [ ] S2.1 configuration surface: `l5.cv_kf_v1.solid_body`, experiments as aliases, runtime
      keys, live parity or refusal
- [ ] T3 branch merged
- [ ] S2.2 shared state machine, reference translations, A2 association, `near_edge_track`
- [ ] S2.3 A1 ablation on the tuning partition
- [ ] S2.4 per-row reference and support columns, refined-stage solid bodies, oracle coverage
- [ ] Label-free gates 1 to 5 on the held-out case; screen report
