# Route SLAM oracle comparison: runs A and B, question by question

The two oracle runs agree on the shape of the minimum: a bridge between two anchored ends,
two-pass offline de-skew, prior factors with weak directions projected out and an alias guard,
and an ego track whose σ includes correlated drift and whose stops are found without the
trajectory. They disagree on where priors fall in the two phases, on the handover from a site to
its drive, and on the rotation rate for a car. The first disagreement is settled here by
derivation. The second is settled by the owner's mount fact, which only run B had. The third is
left to the native 20 Hz arm both runs designed in the same way.

- **Status:** Comparison and reconciliation, written in the main session after both runs
  finished. The owner pinned the reconciled gates and took the decisions below at their defaults on
  2026-11-17. They replace neither run's text. The phase plans carry them.
- **Scope:** Questions 1–32 of the [oracle brief](lidar-route-slam-oracle-brief.md), answered by
  [run A](lidar-route-slam-oracle-response-fable.md) and [run B](lidar-route-slam-oracle-response-opus.md).
- **Baseline:** both runs read the brief, plan and evaluation spec as at velocity.report
  `ab84a68` and Gerardus at `e04587c`.
- **Independence:** run B was barred from run A's response, the phase plans built from it and
  their backlog lines, and read the three input documents from copies taken at `ab84a68`. Run B
  alone had the owner's mount fact (below). Neither run executed anything.
- **Method:** where the runs differ, the verdict follows the derivation or a fact checked in the
  files, not a vote; facts the runs disagree on were re-checked against
  `tools/s2-archive/site-index.json` in this session.

## Evidence

### The owner's mount fact

After run A and before run B, the owner said:

> For every ride between sites where the capture is continuous, the sensor stayed on its tripod,
> ratchet-strapped to the roof of a car.

The tripod therefore moved from the ground to the roof at the start of each drive and back at
the end. A site's accepted pose (tripod on the ground) is not the drive's first or last pose.
Run A answered question 4 without this and proposed a check that the pose held. Run B built its
handover on it.

### Facts re-checked in this session

| Claim                                                                                                                  | Source | Checked                                                                                                                                                                |
| ---------------------------------------------------------------------------------------------------------------------- | ------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| The site index has 21 same-day consecutive pairs, not 20 (E2)                                                          | A, B   | True: 20 of 5.0–34.4 min and one of 81.8 min                                                                                                                           |
| Only about 14 drives are continuous; file names show recorder restarts in the rest                                     | B      | True by file name: 7 of 21 cross a series change or a sequence reset (for example `s2_sf_2_…_00033` → `_00002`), leaving 14. Join grading in the exporter must confirm |
| Run A's development drive columbus-broadway → van-ness-sacramento is usable                                            | A      | **False**: it crosses from `s2_sf_3` to `s2_sf_4`                                                                                                                      |
| Run A's second test chain columbus-north-point → embarcadero-bay → columbus-broadway is usable                         | A      | **False**: both legs cross a restart                                                                                                                                   |
| Run B's day-2 chain lombard-broderick → … → lombard-laguna → columbus-north-point is continuous                        | B      | True: one series, files 6 to 30; two registered sites                                                                                                                  |
| The continuous one-registered-end drives are broadway-franklin → laguna-eddy and marina-webster-beach → lombard-laguna | B      | True                                                                                                                                                                   |
| lombard-laguna has a 55 s hole just before the first drive                                                             | B      | True: its static parts end at 11:32:37 and resume at 11:33:32                                                                                                          |
| Site bounds often sit on 5-minute file boundaries, so the spec's 30 s margins are not shown to be static               | A, B   | True: several gaps are exactly 5.0 or 10.0 min                                                                                                                         |
| E10's table is wrong because the code's default σ is 0.02 m                                                            | B      | Partly: the table states σ_pos = 0.01 m by design and is correct for that input; the code default of 0.02 m is what a user gets, and the brief did not say so          |

### Corrections both runs made to the brief

Both found, independently:

- **The clock residual.** Its 4×MAD cut has a 50 µs floor, and the reported residual includes
  rejected blocks. The synthetic jitter is uniform ±300 µs (σ about 173 µs), and the block
  sawtooth alone puts about 160 µs RMS under the 252 µs measured.
- **The 20 Hz figures.** E5's real-packet 20 Hz figures must come from one line across about 3 s
  of rotations, since that capture never completes a rotation.
- **Hypothesis separation** compares coarse peaks at least 4 m or 20° apart, not ±3.7/±7.6 m lot
  aliases, and `evaluate(refine)` computes none.
- **`graph.rs`** inverts a large matrix per node for its marginals.
- **The stop check** in `lidar-ego-track` is circular, and the stated σ is too narrow.

## Summary of the comparison

Of 32 questions, the runs agree on 23, agree in substance with different numbers on 6 (resolved
below), and disagree on 3: questions 4, 29 and 30, plus the phase boundary in 31.

## Question by question

Verdict: **agree** (same recommendation), **merge** (compatible, reconciled value below),
**differ** (resolved as stated).

### A. Observability of a two-anchor bridge

| Q   | Run A                                                                                                       | Run B                                                                                                   | Verdict                                                                                                               |
| --- | ----------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------- |
| 1   | Random walk plus scale and heading lever; with intersection factors B stops mattering two intersections out | Rely on priors mid-route; B screened after about `9 σ_p²/q` metres                                      | Agree                                                                                                                 |
| 2   | Sound; run the held-out solve in both directions                                                            | Sound if each handover is tested on its own; two triangle closures and two odometry-only solves         | Merge: both directions and the triangle closures                                                                      |
| 3   | No quantitative held-out claim from a manual end; wide along-street σ in the product solve                  | Along-street projected out; use it only as an alias detector                                            | Merge: B's projection (cleaner than a 2.5 m σ); yaw σ differs, see the table                                          |
| 4   | Not by assumption: test each end with a margin scan against its pose                                        | No (fact, owner): break the chain at both lifts and anchor the roof-parked intervals to the site's scan | **Differ, settled by the owner's fact:** run B. Run A's check assumes the pose carries over, which the lift rules out |

### B. Timing and de-skew

| Q   | Run A                                                                              | Run B                                                                                   | Verdict                                                                                        |
| --- | ---------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------- |
| 5   | Not adequate in turn-in and brake dive beyond 15 m; second pass with centred poses | Fails in turns beyond 20–30 m; two-pass de-skew, not CT-ICP                             | Agree                                                                                          |
| 6   | One line per rotation; tighter gates and continuity; native packet time later      | Same, plus tail-RPM check and neighbour interpolation for failed rotations              | Merge: both; a failed rotation is interpolated and labelled, never silently packet-timed       |
| 7   | Negligible in LiDAR-only work; estimate the lag in the radar match                 | Ignore in (a) and ego speed; in the radar match use \|a\| < 0.5 m/s² or cross-correlate | Agree                                                                                          |
| 8   | Session seconds; carry enough to re-time without captures                          | Same, plus make the LAS self-describing                                                 | Merge: add B's time-convention VLR, since a clear LAS time-type bit reads as GPS week time (A) |

### C. Priors registration

| Q   | Run A                                                                                      | Run B                                                                                                   | Verdict                                                                                                          |
| --- | ------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| 9   | Trait over `matched`, local map rebuilt per keyframe, twist motion; no voxel hash at first | Matcher trait, voxel-hash map, bridge graph, `graph.rs` fixes; no adaptive threshold                    | Merge: A's keyframe-rebuilt map first (less work), voxel hash only if too slow; both drop the adaptive threshold |
| 10  | Diagonal per-metre and per-second covariance in body axes, scaled from the drive's checks  | Per-metre, per-radian, per-second random walk; ICP information for shape only; calibrate on innovations | Merge: B's (A's plus degeneracy shape and per-radian terms); information rotated into site axes (B)              |
| 11  | Project out weak directions; along-street only when geometry and chain support it          | Eigenvalue ≥ 1e-2 and ≥ 1e-3 from points above 2 m; chain σ_∥ gate and shifted-start test               | Agree; B's numbers                                                                                               |
| 12  | Robust kernel not enough; ground for height and tilt, structure ≥ 2.5 m up for horizontal  | Redescending cut at 0.30 m and a sensor-frame co-moving mask, plus a self mask for the roof             | Merge: both; the self mask follows from the owner's fact                                                         |
| 13  | Gates at the solved pose; hold out B and each factor in turn                               | Gates stratified by yaw rate; Mahalanobis plus 1.85 m along-street, not 2% of distance                  | Merge: see the held-out gate below                                                                               |

### D. The ego track

| Q   | Run A                                                                             | Run B                                                                                      | Verdict                                                                   |
| --- | --------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------- |
| 14  | Keep the local cubic; share language with the tracker                             | Keep it; constant-jerk RTS smoother as the later shared family                             | Agree                                                                     |
| 15  | Jerk at ±0.75 s                                                                   | Jerk at ±0.65 s scaled by `(σ_pos / 1 cm)^(2/11)`, only above 3σ                           | Merge: B's rule (derivation below); A's value is within 15% of it at 1 cm |
| 16  | White plus random walk `q`                                                        | White, random walk `q_s` per metre, and scale bias `ε_s`; along-track σ                    | Merge: B's three terms                                                    |
| 17  | "Ride motion 1.3–5 Hz" at 10 Hz; significance and gain ≥ 0.5                      | "Attitude and height residual 1 Hz to Nyquist"; per-sector ground tilt for the higher band | Agree; both want per-sector ground fits for the band above Nyquist        |
| 18  | Stops found without the trajectory; zero-velocity holds in the product only       | Raw scan differencing; zero-velocity factors in the product only; judge on A-only          | Agree                                                                     |
| 19  | Gerardus writes poses, quality, noise model; velocity.report alone differentiates | Same, plus pose covariance and the de-skew twist                                           | Agree; take B's fuller list                                               |

### E. Evaluation

| Q   | Run A                                                       | Run B                                                                                        | Verdict                                                                           |
| --- | ----------------------------------------------------------- | -------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------- |
| 20  | Far anchor, pose-free standstill, radar, centreline length  | Held-out B, DataSF route length on moving intervals, stops; radar first once it exists       | Agree on the anchor first; the order of standstill and route length is immaterial |
| 21  | One extra check per criterion                               | Same; two of six are tautological                                                            | Agree                                                                             |
| 22  | No; develop on two drives, test once on the registered pair | No; tune on unanchored continuous drives, test on the anchored set including the day-2 chain | Merge: B's drive set, because two of A's choices are not continuous (Evidence)    |

### F. Rotation rate

| Q   | Run A                                                          | Run B                                                                            | Verdict                                                                                                                              |
| --- | -------------------------------------------------------------- | -------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------ |
| 23  | For a car, little: with centred de-skew nothing binds at 10 Hz | In turns de-skew residue dominates and 20 Hz cuts it fourfold                    | **Differ:** both compute 0.225 m at 30 m at 10 Hz and 0.056 m at 20 Hz. A assumes pass 2 removes it, B does not. Phase 1 measures it |
| 24  | Each rotation alone                                            | Each rotation alone; merge pairs only where a scan degenerates                   | Agree                                                                                                                                |
| 25  | Rate table; six couplings                                      | Rate table; four inherited contracts                                             | Agree; union of the couplings                                                                                                        |
| 26  | Rate-invariant to first order; turns the exception             | Rate-invariant on straight submaps, not in turns; stratify by yaw rate           | Agree                                                                                                                                |
| 27  | Same windows; necessary, not sufficient                        | Same, and compare white PSD and `q_s`, not per-pose σ                            | Agree                                                                                                                                |
| 28  | 10–20–10 drive of one route                                    | Same, same mount, 360° field of view                                             | Agree                                                                                                                                |
| 29  | Car 10 Hz; bike and walking 20 Hz; traffic 10 Hz               | 20 Hz for SLAM on every moving platform; traffic 10 Hz on tripods                | **Differ** on the car only; follows question 23 and is settled by gate 2's rate arm                                                  |
| 30  | Document the 340° rule; fix the exporter                       | A defect: completion relative to the field of view, hard cap at twice the period | **Differ**, resolved below                                                                                                           |

### G. Sequencing

| Q   | Run A                                                                | Run B                                                                                    | Verdict                     |
| --- | -------------------------------------------------------------------- | ---------------------------------------------------------------------------------------- | --------------------------- |
| 31  | Phase 1 bridge and ego track without priors; phase 2 priors and rate | Phase 1 bridge, handover and priors at 10 Hz; phase 2 native 20 Hz and the wider archive | **Differ**, resolved below  |
| 32  | Nine decisions                                                       | Eleven decisions                                                                         | Merged under Open questions |

## Resolving the disagreements

### The phase boundary (question 31)

Run A's derivation is that a position error varying as `e(ℓ)` along the route changes speed by
`v × de/dℓ`, about 0.03 m/s at 15 m/s for 2 m over 1 km. Acceleration, jerk and vibration do
not see it at all. Run B does not dispute this. Its reason for priors in phase 1 is different:
the 20 Hz capture is worth making only if 10 Hz fails on de-skew in turns rather than on
along-street observability, and only per-submap gates can tell those apart.

Both hold. The reconciliation keeps run A's boundary for the gate: gate 1 judges the bridge and
the ego track, which need no priors. Phase 2 then runs priors on the archive first (items
10–12) and makes the capture (item 16) conditional on run B's test. Capture only if the 10 Hz
turn submaps fail the gates after the second de-skew pass while the straight submaps pass. Run
B's handover registration against the site model does enter phase 1, because the owner's fact
makes it the anchor, not an optional check.

### The handover (question 4)

Run B, because of the owner's fact. Break the chain at the lifts. Find the stationary
roof-parked intervals from the rotations (a sensor-height step of at least 0.5 m between ground
and roof). Anchor each by registering the accumulated roof rotations to the site's accepted
scan with Gerardus's `register_against`. Check it by triangle closure against a direct 3DEP
registration of the same interval, within 0.2 m.

### The frame builder (question 30)

Both runs agree on what the spike needs: the exporter caps frame growth and names a sector
capture. Run B is right that a frame growing without limit is a defect whatever the field of
view. Run A is right that the builder is shared with the live path and its performance
baselines. Field-of-view-relative completion is therefore a separate change with its own
recaptured baselines, outside the spike. The exporter's cap and sector refusal are phase 1.

### The car's rotation rate (questions 23 and 29)

Not resolvable from the derivations: both give the same residue, and the difference is whether
the second de-skew pass removes it at 10 Hz. Phase 1 measures turn submaps after pass 2 at
10 Hz, and gate 2's rate arm (N20, D10, N10) decides per platform. Until then the provisional
recommendation is 10 Hz for car traffic measurement (both runs) and no default for car SLAM.

### The jerk window (question 15)

Run B's scaling is derived. For a local cubic, the jerk bias from the fifth derivative grows as
`h²`, and the noise as `σ h^(−7/2)`. Minimising `c₁h⁴ + c₂σ²h^(−7)` gives `h ∝ σ^(2/11)`. Its
±0.65 s at 1 cm and run A's ±0.75 s are within 15%. Adopt run B's rule and refit the constant
once `σ_pos` is measured.

## Reconciled gates and defaults

Proposals for the owner to pin. "A" and "B" give each run's value.

| Name                       | Run A                                                             | Run B                                                        | Reconciled                                                     | Basis                                    |
| -------------------------- | ----------------------------------------------------------------- | ------------------------------------------------------------ | -------------------------------------------------------------- | ---------------------------------------- |
| Clock period               | ≤ 0.5% of running median                                          | ≤ 0.1% of tail RPM                                           | Both                                                           | Independent checks of the same quantity  |
| Clock continuity           | step ≤ 100 µs                                                     | p95 ≤ 100 µs                                                 | p95 ≤ 100 µs, maximum reported                                 | B, with A's maximum kept visible         |
| Clock residual             | ≤ 500 µs RMS over kept blocks                                     | over kept blocks only                                        | ≤ 500 µs RMS, kept blocks                                      | Both                                     |
| Failed rotation            | never silently packet-timed                                       | interpolate from neighbours, label the source                | Interpolate and label                                          | B                                        |
| Lift detection             | —                                                                 | sensor-height step ≥ 0.5 m                                   | ≥ 0.5 m                                                        | Owner's fact                             |
| Handover                   | margin scan within 0.10 m, 0.5°                                   | rank 6, separation ≥ 1.25, p75 ≤ 0.10 m; closure ≤ 0.2 m     | B                                                              | Owner's fact                             |
| `REGISTERED` anchor σ      | floor 0.05 m, 0.05°                                               | max(10× formal, 0.03 m), 0.05°                               | max(10× formal, 0.05 m), 0.05°                                 | Both note formal σ is 7–10× too small    |
| Manual anchor              | 0.10 m across, 2.5 m along, 0.5° yaw                              | along-street projected out, 0.10 m, 0.2° yaw                 | Projected out, 0.10 m, yaw 0.5°                                | B's form, A's wider yaw until measured   |
| Self and co-moving masks   | —                                                                 | 2.5 m plus a learned mask; < 5 cm sensor, > 20 cm world      | B                                                              | Owner's fact                             |
| Robust cut                 | —                                                                 | redescending at 0.30 m                                       | 0.30 m                                                         | B                                        |
| Keyframes                  | 10 m or 10°                                                       | 5 m, 10° or 2 s                                              | 5 m, 10° or 2 s                                                | B's is stricter at low speed             |
| Submaps                    | 50 m, cut at 30°                                                  | 50 m, cut at 30°, at most 150 m                              | B                                                              | Agree                                    |
| Prior directions kept      | eigenvalue share ≥ 1e-2                                           | ≥ 1e-2, and ≥ 1e-3 from points above 2 m                     | B                                                              | Agree                                    |
| Alias guard                | innovation ≤ max(3σ, 0.5 m), cost ratio ≥ 1.25 at ±3.7 and ±7.6 m | chain σ_∥ ≤ 0.6 m, else separation ≥ 1.25 at the same shifts | B's chain gate first, A's innovation bound as well             | Same test, two triggers                  |
| Held-out end               | ≤ 0.5% of path and 1°; NIS in [0.115, 11.3]                       | Mahalanobis² ≤ 11.3 and along-street ≤ 1.85 m                | Mahalanobis² in [0.115, 11.3], along-street ≤ 1.85 m, yaw ≤ 1° | Both bounds of χ²(3); half a lot period  |
| Stop detection             | still cells at 1.0 s lag, share ≥ 0.8                             | raw scan differencing; threshold ≥ max(0.3 m/s, 4σ_v)        | A's detector, B's threshold                                    | Same idea                                |
| Standstill drift           | median ≤ 0.01 m/s, worst ≤ 0.03 m/s                               | —                                                            | A                                                              |                                          |
| Speed σ model              | white + random walk `q`                                           | white + random walk `q_s` + scale `ε_s`, along-track         | B                                                              | B adds the scale term                    |
| Speed, acceleration window | ±0.5 s                                                            | ±0.5 s                                                       | ±0.5 s                                                         | Agree                                    |
| Jerk window                | ±0.75 s                                                           | ±0.65 s × (σ/1 cm)^(2/11), above 3σ                          | B                                                              | Derivation above                         |
| Kinematics reference point | sensor                                                            | sensor and a road-reference point                            | Both points                                                    | Pitch lever arm at 2.5–3.5 m roof height |
| Spectrum gain correction   | gain ≥ 0.5; peak ≥ 1.5 × floor                                    | gain ≥ 0.5                                                   | A                                                              | Agree                                    |
| Trajectory gap             | 0.35 s                                                            | 0.35 s                                                       | 0.35 s                                                         | Agree                                    |
| Stated σ consistency       | empirical / stated in [0.7, 1.4]                                  | odd/even empirical within 0.75–1.33                          | B's odd/even method, A's interval                              | Same test                                |

## What changes in the phase plans

- **Drives:**
  - develop on broadway-franklin → laguna-eddy and marina-webster-beach → lombard-laguna;
  - test once on lombard-laguna → columbus-north-point;
  - use the day-2 chain as the second test.

  columbus-broadway → van-ness-sacramento is dropped, because it is not continuous.

- **Phase 1:**
  - lift detection joins the stillness item;
  - the handover item registers roof-parked intervals to the site model and runs a triangle
    closure;
  - the odometry item gains the self and co-moving masks and the redescending cut;
  - the ego-track item gains the scale term, along-track σ, run B's jerk rule and the
    road-reference point;
  - gate 1 uses the reconciled held-out and σ-consistency tests.
- **Phase 2:**
  - the 20 Hz capture becomes conditional on the turn-against-straight submap result;
  - field-of-view-relative frame completion becomes a separate change with recaptured
    performance baselines;
  - DataSF footprints are reported, not used as factors, until their 1–1.4 m offsets from 3DEP
    are explained (run B).

## Open questions

Owner decisions, all taken at these defaults on 2026-11-17:

1. Accept the reconciled phase boundary: ego track gated in phase 1, priors in phase 2, and the
   20 Hz capture conditional on the turn-submap test. Default: yes.
2. Reserve lombard-laguna → columbus-north-point as the never-tuned test, with the day-2 chain as
   the second test. Default: yes.
3. Use manually placed ends only as alias detectors, never for held-out claims. Default: yes.
4. Pin the reconciled gates above, or amend them, before evaluation 1. Default: pin.
5. Report ego kinematics at the sensor and at a road-reference point. Default: both.
6. Treat the roof-tripod car drives as a vibration proxy for the bike, as route plan Item 2
   assumes? Run B says no: a ratchet-strapped tripod on a car roof has its own strap and leg
   modes. Default: no. The backlog item is re-worded when its slot comes up.
7. The operator records the tripod's height on the roof and the strap layout on future drives
   (run B). Default: yes.
8. Make field-of-view-relative completion a separate change? Default: yes, outside the spike.

What only measurement can settle:

- whether the second de-skew pass removes the turn residue at 10 Hz, which is the car's rate
  question;
- the mount's vibration spectrum on the roof, and whether strap and leg modes reach the 5 Hz band;
- the drift model (`q_s`, `ε_s`) and whether odometry noise is white or a random walk;
- how well the roof-to-site handover registers at each of the three registered sites involved;
- whether the archive is 10 Hz and single return throughout.
