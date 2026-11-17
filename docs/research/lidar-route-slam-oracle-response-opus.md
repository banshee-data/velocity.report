# Route SLAM oracle response, run B: the two-anchor bridge and the ego track at 10 and 20 Hz

Treat each archive drive as a bridge between two roof-parked poses, each re-anchored to its site's
accepted scan, and never chain odometry through the hand lifts onto and off the car roof; with
two-pass de-skew, alias-gated prior factors and a correlated-noise σ, the minimum runs on the
10 Hz archive alone. Native 20 Hz belongs in a second phase, worth a capture day only if the first
phase fails on de-skew in turns rather than on along-street observability.

- **Status:** Oracle run B's answers and proposed work order, a cross-check run independently of
  run A. Nothing executed, nothing adopted, not owner decisions.
- **Scope:** Answers questions 1–32 of the [oracle brief](lidar-route-slam-oracle-brief.md), with
  the owner's mount fact added.
- **Baseline:** Brief inputs as at velocity.report `ab84a68` and Gerardus `e04587c`.
- **Related:** [route SLAM minimum plan](../plans/lidar-route-slam-minimum-plan.md),
  [first-pair evaluation](../../data/experiments/try/route-drive-bridge-first-pair.md),
  [20 Hz readiness audit](lidar-20hz-readiness.md),
  [route capture plan](../plans/lidar-route-capture-plan.md),
  [portable capture timing](../lidar/architecture/portable-capture-timing.md)

## Evidence

Oracle run B (Euler persona) read the brief, plan and evaluation spec in the versions the first
reviewer had, then the code and documents below. Claims are tagged **fact** (with source),
**derivation** (shown) or **judgement**. The owner's new fact is tagged **(fact, owner)**.

Read in velocity.report: `internal/lidar/motionexport/clock.go`, `export.go`, `las.go` and
`motionexport_test.go`; `internal/lidar/egotrack/kinematics.go`, `analysis.go`, `trajectory.go`;
`internal/lidar/l2frames/frame_builder.go` and `frame_builder_cleanup.go`;
`internal/lidar/l1packets/parse/extract.go` (`blockToPoints`); a grep of
`internal/lidar/l5tracks/smoother.go`; `tools/s2-archive/site-index.json` in full; the 20 Hz
audit, the route capture plan and the timing design. Read in Gerardus: `src/register.rs`,
`src/graph.rs`, `docs/REGISTRATION-STATUS.md` §1, `docs/plans/MOBILE-SLAM-REPLAY.md`, and
`docs/research/2026-10-24-registration-and-slam-maths.md` §1, §3 and §4.

### Evidence items checked

| Item       | Verdict and finding                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| ---------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| E1         | Confirmed (`REGISTRATION-STATUS.md` §1). Five of the 18 `MANUALLY_PLACED` records do carry σ (embarcadero-bay, embarcadero-broadway, fulton-divisadero, marina-broderick, van-ness-sacramento); 13 carry none. All stated σ are the formal `σ² H⁻¹` of `sensor_pose`, which the maths note (§1.3, §3.3) puts at 7–10× too small.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| E2         | **Disagrees.** The site index gives 21 same-day consecutive pairs (6, 8 and 7 on the three days): 20 gaps of 5.0–34.4 min and one of 81.8 min. E2's 20 matches only if ashbury-downey → haight-divisadero is left out; its ends lie in different capture series (`s2-sf-0.pcapng`, then `s2-1_…`). More important: in 6 of the 21 gaps the file names show a recorder restart (new series prefix or counter reset). Only 14 drives lie inside one continuous series, 15 if ashbury → haight joins. Of E2's seven one-registered-end drives, **only two are continuous**: broadway-franklin → laguna-eddy and marina-webster-beach → lombard-laguna. The other five cross a restart. This is inferred from file names and the 5-minute rotation, and the exporter's join grading must confirm it. The first pair is confirmed: 16.3 min, 1.31 km straight line (derivation from the index's lat/lon), files `_00026`–`_00030` consecutive.             |
| E2, extra  | lombard-laguna has a 55 s hole between its last two static parts (11:32:37–11:33:32), just before the drive. On 2 September many site bounds fall exactly on 5-minute file boundaries (columbus-north-point starts at the first instant of `_00030`). So the "30 s static margins" are not shown to be static.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| E3         | Confirmed (`export.go`, `summariseTime`). `rotation_hz` is 1 / median fitted period on the PCAP host clock. The packet tail's motor speed is an independent check that the exporter does not read.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| E4         | Confirmed (`extract.go` lines 610–613: `pointTime := packetTime.Add(firetimeOffset)`; `blockIdx` is unused for time).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| E5         | **Partly wrong.** (1) The rejection cut is `max(4 · 1.4826 · median(abs(r)), 50 µs)`: a robust 4σ, about 5.9× the median absolute residual, not "4×MAD". (2) `RMSResidUS` and `MaxResidUS` loop over all blocks, including rejected ones, so the reported 252 µs RMS and 4.9 ms maximum include the outliers the fit excluded. (3) The synthetic jitter is uniform ±300 µs per packet (σ ≈ 173 µs), not 300 µs RMS. (4) The 10-block staircase alone puts a floor of `55.56 µs · √((10²−1)/12) ≈ 160 µs` RMS on the residual (derivation), so 252 µs implies about 195 µs of per-packet jitter. (5) The 250 µs constant offset is the synthetic's. On real packets the offset is 250 µs plus an unknown arrival latency. (6) If E8 holds, the 20 Hz fixture never yields a complete rotation, so the "real 20 Hz" figures (252 µs, 0.050003 s) must come from a fit across an accumulated multi-rotation frame. I infer this and could not verify it. |
| E6, E7, E9 | Confirmed against the audit text.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| E8         | Confirmed, and it is a code contradiction. `shouldStartNewFrame` has an `azimuth_wrap_large_jump` rule written for sector wraps ("289° -> 61°"), but it fires only when `cov >= MinAzimuthCoverage` (340°), which a 230° sector can never reach. The azimuth-only branch has no duration or size cap.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| E10        | Verified analytically for 10 Hz ±0.5 s (0.0246, 0.068, 0.763) and 20 Hz ±0.5 s speed (0.0182). The code's default per-pose σ is 0.02 m, not the 0.01 m used in the table (`DefaultSigmaPosM`).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| E11        | Confirmed. Decimation counts complete rotations only, so an incomplete rotation shifts the phase.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| E12        | Confirmed. Additional facts: `SpeedSigma` is the per-axis σ, so along-track anisotropy is ignored. `MaxAbsJerkLong` and the other maxima are taken over about 10⁴ noisy samples. `DistanceM` integrates the speed norm through stops, where it carries a Maxwell noise floor.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| E13        | Confirmed in code. The gain correction is applied wherever the gain `abs(1 − H)` exceeds 0.1 (up to 10× amplification). The 8.48 m/s step overshoot was not re-derived.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| E14        | **Partly wrong.** Huber 0.10 m, planarity weights, left perturbation, the 10 m lever and the three static gates are confirmed. But hypothesis separation is computed against whichever _distinct coarse peaks_ the search found (`distinct(all, peaks, 4.0, 20.0)`: at least 4 m **or** 20° apart). It is not computed against ±3.7 m or ±7.6 m aliases, and a 3.7 m alias at the same heading cannot be a distinct coarse peak. `evaluate` (the `EvaluatePose(refine)` path a submap prior would use) computes **no separation at all**. `Reference::matched` classifies points with the 3DEP ground raster, so `icp` cannot run against an odometry map without a trait. `prepare_scan` drops only returns under 1.0 m.                                                                                                                                                                                                                             |
| E15        | Confirmed, with limits that matter for a drive. The Jacobian is numeric and dense, the Cholesky is dense, and `marginal` computes one Schur complement per node, which costs `O(N · (6N)³)`. There is no robust kernel. `Between` errors are in site axes, so odometry information must be rotated into site axes.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| E16        | Confirmed. One small point: §4.1's residue `≈ 1.5 a T²` is conservative. With poses at sweep ends I derive an end-to-end residue `a T (T_s + T)/2 = a T²`, about half of which survives the rigid fit. I use the note's figure for the gates.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| E17        | The smoother's capture-time lag is confirmed by grep. The other items were not read.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |

### The owner's mount fact and what it changes

**(fact, owner)** On every continuous motion ride between sites, the sensor stayed on its tripod,
ratchet-strapped to the roof of a car. Between captures the tripod was lifted onto and off the
roof. Consequences, used throughout:

- **Each site's static capture starts with a new tripod placement** (derivation from the fact).
  Site poses are independent placements. They connect to the drive only through the stationary
  roof interval near each site, never through the tripod pose itself.
- **Each drive window contains, in order:** static ground, a hand lift, strapping (stationary
  with jolts), the drive, unstrapping, a hand lift, and static ground. The lifts are hand-carried
  motion, the hardest case for LiDAR-only odometry (judgement).
- **Self-returns.** The roof, straps and tripod legs are fixed in the sensor frame. They pull
  odometry towards zero motion and must be masked.
- **Lever arm.** The sensor sits about 2.5–3.5 m above the road (judgement; the static captures
  were about 2.3 m, maths note preamble). Body pitch moves it longitudinally by `h · dθ/dt`.
- **Mount dynamics.** Body pitch and bounce (1–2 Hz), plus strap and tripod-leg compliance
  (judgement: modes of about 5–20 Hz), lie partly above both Nyquist limits. The car drives are
  therefore not a bike or backpack vibration proxy (route plan Item 2 assumes they are).

## Restatement

1. Twenty-one consecutive same-day site pairs exist, of which about 14 are continuous recordings.
   One continuous drive has both ends `REGISTERED`, and two have one.
2. On each continuous drive the tripod is lifted onto the roof and off it again. The site poses
   anchor the drive only through the stationary roof intervals beside each site.
3. (a) means estimating the sensor's trajectory from offline LiDAR odometry (no IMU, no GNSS),
   pinned by prior factors against 3DEP from roughly 50 m submaps, plus the two roof anchors. The
   static gates are reported per submap.
4. (b) means differentiating that trajectory into speed, acceleration, jerk, yaw rate, stops and
   vibration with an honest σ. The present σ assumes white per-pose noise, but odometry error is
   correlated.
5. Along-street position is the weak direction for priors in mid-block, and lot-period aliases
   (3.7 m, 7.6 m) threaten any prior factor or check that is off by more than about 1.5 m.
6. The point clock is the rotor (an azimuth line per rotation), good to about 20–30 µs. Constant
   offsets cancel in odometry but not against another clock.
7. The archive is believed to be 10 Hz. 20 Hz halves the sweep and the points per sweep, and
   decimating 10 Hz to 5 Hz changes only the pose rate.
8. Every parameter must be stated in seconds or metres so that one configuration serves both
   rates. Some existing contracts count rotations.
9. Raw data and trajectories stay on hydra. Only authored summaries reach Git.
10. Deliverable: the minimum code, gates fixed before results, and two phases each ending in a
    kill-or-continue evaluation, with phase 1 on the archive alone.

## Answers

### A. Observability of a two-anchor bridge

#### 1. Along, across and yaw uncertainty with A, with A and B, and with priors

**Recommendation:** Rely on priors, not on B, for mid-route accuracy. B removes the mean drift rate
and is the held-out check, but intersection priors screen it after about `9 σ_p²/q` metres.

**Model (derivation).** Treat each straight leg in its own axes. Position along the route is
`x = sL`. Odometry error has three parts: a systematic along-track rate `d`; an along-track
random walk of variance `κ_∥² x`; and a yaw random walk of variance `κ_ψ² x`, whose integral is
an across-track error of variance `κ_ψ² x³ / 3`. Anchors carry `σ_A`, `σ_B` (position) and
`σ_Aψ`, `σ_Bψ` (yaw).

(i) A only:

```text
σ_∥²(x) = σ_A² + (d x)² + κ_∥² x
σ_ψ²(x) = σ_Aψ² + κ_ψ² x
σ_⊥²(x) = σ_A² + (σ_Aψ x)² + κ_ψ² x³ / 3
```

(ii) A and B (position and yaw at both ends). A constant `d` is estimated exactly by the closure.
What remains is its variation `δd` along the route (a parabola with a peak of `δd · L/4`), the
Brownian bridge, and the cubic-Hermite bridge for the integrated yaw walk:

```text
σ_∥²(s) = (1−s)² σ_A² + s² σ_B² + κ_∥² L s(1−s) + (δd L s(1−s))²
σ_⊥²(s) = (1−s)² σ_A² + s² σ_B² + κ_ψ² L³ s³(1−s)³ / 3
          + L² s²(1−s)² [ (1−s)² σ_Aψ² + s² σ_Bψ² ]
σ_ψ²(s) = 1 / [ 1/(σ_Aψ² + κ_ψ² sL) + 1/(σ_Bψ² + κ_ψ² (1−s)L) ]
```

(iii) Priors: across and yaw every `D_⊥ = 50 m` (σ_p⊥, σ_pψ), and all six directions at
intersections spaced `D_∥` (σ_p∥). Mid-way between priors:

```text
σ_∥ ≈ sqrt( σ_p∥²/2 + κ_∥² D_∥/4 + (δd D_∥/4)² )
σ_⊥ ≈ sqrt( σ_p⊥²/2 + κ_ψ² D_⊥³/192 )
σ_ψ ≈ sqrt( σ_pψ²/2 + κ_ψ² D_⊥/4 )
```

Worked numbers. All inputs are judgements to be replaced by measurement: L = 1.6 km (route
factor 1.2 on the 1.31 km straight line); σ_A = σ_B = 0.05 m; σ_Aψ = σ_Bψ = 0.05°; d = 1%,
δd = 0.5%; κ_∥ = 0.01 m/√m; κ_ψ = 1e-4 rad/√m (from the maths note's per-frame facade yaw, §4.5,
at 1 m per frame); σ_p⊥ = 0.05 m; σ_pψ = 0.05°; σ_p∥ = 0.10 m; D_∥ = 125 m.

| Case         | Along, mid               | Along, at B | Across, mid | Across, at B | Yaw, mid |
| ------------ | ------------------------ | ----------- | ----------- | ------------ | -------- |
| (i) A only   | 8.0 m                    | 16 m        | 1.5 m       | 4.0 m        | 0.17°    |
| (ii) A + B   | 2.0 m (0.20 m if δd = 0) | σ_B         | 0.52 m      | σ_B          | 0.12°    |
| (iii) priors | 0.18 m                   | 0.18 m      | 0.035 m     | 0.035 m      | 0.04°    |

**Where B stops helping (derivation).** B's information reaches an intermediate along-street
prior at distance `y` from B with weight `share(y) = σ_p² / (σ_p² + q y)`, where `q` is the
along-track variance growth per metre. Its share falls below 10% at `y ≈ 9 σ_p²/q`. With random
walk alone (`q = κ_∥² = 1e-4 m²/m`, σ_p∥ = 0.1 m) that is about 0.9 km. B therefore stops helping
the mid-route along-street estimate at `L* ≈ 18 σ_p²/q ≈ 1.8 km`. If drift-rate variation
dominates, `q` is effectively `δd² D_∥` and `L*` falls to about one block. For across and yaw,
50 m priors screen B within a few hundred metres. Without priors, B always helps: it halves the
mid-point variance of a random walk and removes the mean drift rate at any L.

A car-specific fact helps here (derivation). At every turn, the previous leg's along-street error
becomes across-street error on the next street, where mid-block priors constrain it. Routes with
turns are better conditioned than straight ones.

- **Defer:** a scale-bias state in the graph; feature anchors.
- **Falsifier:** submap innovations (Q10) whose variance grows faster than linearly in distance
  (systematic, not random-walk), or a B residual outside 3σ of (ii).

#### 2. The far anchor instead of loop closure, and what the A-only residual tests

**Recommendation:** Sound, provided the handover at each end is tested on its own. Separate the
terms with two triangle closures and two odometry-only solves.

A loop closure is a relative constraint between two visits by the same sensor. A far anchor is
absolute: it brings in B's registration error and B's handover. Because A and B are registered to
the same 3DEP survey, a common datum offset cancels, but site-specific registration biases (eave
insets, recesses: the maths note gives 0.1–0.36 m) do not (derivation). So the anchor is a weaker
check than closure on bias and a stronger one on drift, since it needs no place recognition.

The A-only residual at B is the sum of six terms (derivation):

```text
r_B = e_drift + e_prior + e_hand,A + e_hand,B + e_anchor,A + e_anchor,B
```

Separate them like this:

| Measurement                                                                                                   | Isolates                                           |
| ------------------------------------------------------------------------------------------------------------- | -------------------------------------------------- |
| Roof-parked scans at each end registered to the site's accepted scan (`register_against` with the site model) | e_hand                                             |
| The same roof scans registered directly to 3DEP (`register`, site pose as hint)                               | e_hand + e_anchor at that end, by triangle closure |
| Solve O: odometry + roof anchor A, no priors                                                                  | e_drift (+ end terms, known from the rows above)   |
| Solve P: odometry + roof anchor A + priors                                                                    | e_drift after priors + e_prior                     |
| Solve O′, P′: the same from B with A held out                                                                 | symmetry check                                     |

- **Defer:** reverse-time odometry (not independent: same data).
- **Falsifier:** the triangle closure at B disagrees by more than 0.2 m, which would mean the
  handover, not the drive, is under test.

#### 3. σ for `MANUALLY_PLACED` poses

**Recommendation:** Do not give them a Gaussian along-street σ. Enter them with the along-street
direction projected out (σ_⊥ 0.10 m, σ_z 0.05 m, yaw 0.2°). Use their along-street coordinate only
as an alias detector, never as a held-out pass.

A manual placement accepted from an ICP result that failed separation has a bimodal error: either
centimetres (the right peak) or at least a lot period along the street (judgement from the maths
note §1.5 and the 2026-10-24 run). No Gaussian represents both. The export check cannot be turned
into a σ (derivation):

- it compares the pose with poses moved 2 m or turned 5°, far outside the ICP basin's quadratic
  region, so its curvature says nothing at the centimetre scale;
- it is a proportion of 8,000 samples. Unpaired binomial σ of the difference is about 0.9e-3 at
  1st-mission (margin 0.001), 1.6e-3 at hyde-ofarrell (0.001), 1.6e-3 at pierce-haight (0.002) and
  2.0e-3 at marina-broderick (0.002). Four of the five narrow margins are within about 1.3σ.
  embarcadero-bay (0.026) is about 5σ. Pairing on the same samples would tighten this, and the
  records do not say whether they were paired.

As a held-out end, a manual pose is safe only for across, height, tilt and yaw. Its along-street
coordinate becomes a test: an A-only prediction within `σ_pred` of it is consistent, and a
residual near ±3.7 m or ±7.6 m flags an alias. This works only where `σ_pred,∥ ≤ 1.2 m`, so that
a 3.7 m alias lies at least 3σ away (derivation).

For `REGISTERED` anchors use `max(10 × formal, 0.03 m)` horizontally and `max(10 × formal, 0.05°)`
in yaw (judgement, from the 7–10× factor of maths §3.3), until the variogram measures `n_eff`.

- **Falsifier:** the A+B closure χ² per degree of freedom exceeds 3 with these σ.

#### 4. Is the static pose valid for the first moving rotation?

**Recommendation:** No (fact, owner). Break the chain at both lifts. Start and end the bridge at
the stationary roof intervals, anchored by registering their accumulated rotations to the site's
accepted scan. Detect the intervals from the rotations themselves.

Between the static capture and the drive, the tripod was lifted onto the car roof, so the static
pose describes a different placement. The ground tripod's height was about 2.3 m (maths note);
on the roof it will be about 1 m or more higher (judgement). Each site's static capture likewise
begins with a fresh placement on the ground.

Minimal test from the exported rotations alone (derivation; no SLAM needed):

1. Per-rotation sensor height from the ground plane (the RANSAC of `register.rs`
   `ground_plane`). A step of 0.5 m or more marks a lift.
2. Count returns within 2.5 m of the sensor. Car roof, straps and legs appear only on the roof.
3. Stationarity: the fraction of polar cells whose range changes by less than 5 cm against the
   rotation 1 s earlier. Calibrate the threshold on the static ends.

The result labels each rotation as static ground, transition, static roof, moving or stopped.
The handover is `register_against(site model)` on the accumulated static-roof rotations, using the
site's accepted pose as the hint (existing code: `Reference::build_with` plus `Against`). Kirkham
K1's accumulated frames registered 0.007 m from its background pose (fact,
`REGISTRATION-STATUS.md`), which is the same kind of operation.

If the handover fails its gates (rank 6, separation ≥ 1.25, seed agreement ≥ 0.8, pair p75
≤ 0.10 m), fall back to registering the roof scan directly to 3DEP. If that fails too, the end is
unanchored. The solve then runs from the other end only, and the drive yields no held-out result.

Strap settling under the first braking may shift the tripod by millimetres to centimetres
(judgement). That does not matter for (a), which estimates the sensor's pose directly. For (b) it
appears as a one-off sensor motion.

- **Defer:** tracking the lift with odometry (a diagnostic arm at most).
- **Falsifier:** the labeller does not find exactly two transitions on a continuous drive, or the
  handover disagrees with the direct 3DEP registration by more than 0.2 m.

### B. Timing and de-skew at car speed

#### 5. Is constant-velocity de-skew good enough for the 0.20 m gates?

**Recommendation:** At 10 Hz, no: it fails in turns and in braking pitch beyond about 20–30 m. The
minimum fix is two-pass offline de-skew against a smooth trajectory, not CT-ICP. At 20 Hz,
single-pass de-skew passes the same envelope.

The table uses the maths note §4.1 residue `1.5 · x · T²` (x is acceleration or angular
acceleration, conservative), and `x T²/8` for two-pass de-skew, the deviation of a quadratic
from its chord (derivation). Car-plausible inputs are judgements: braking 3 m/s² (hard 6);
yaw acceleration at turn entry 0.5 rad/s² (sharp 1.0); body pitch acceleration 0.6 rad/s² at
braking onset (about a 0.45° pitch at 0.3 g on a 1.2–1.5 Hz body mode).

| Motion                                   | 10 Hz single | 20 Hz single | 10 Hz two-pass |
| ---------------------------------------- | ------------ | ------------ | -------------- |
| Braking 3 m/s²                           | 0.045 m      | 0.011 m      | 0.004 m        |
| Braking 6 m/s²                           | 0.090 m      | 0.023 m      | 0.008 m        |
| Yaw 0.5 rad/s² at 30 m                   | 0.225 m      | 0.056 m      | 0.019 m        |
| Yaw 1.0 rad/s² at 30 m                   | 0.45 m       | 0.11 m       | 0.038 m        |
| Pitch 0.6 rad/s² at 20 m                 | 0.18 m       | 0.045 m      | 0.015 m        |
| Mount vibration of amplitude A above 1/T | `A · r`      | `A · r`      | `A · r`        |

Single-pass at 10 Hz passes 0.20 m only within `r ≈ 0.2 / (1.5 α T²)`: 27 m at 0.5 rad/s².
Turns happen at intersections, so de-skew failure lands on exactly the submaps that carry the
along-street priors (derivation). Two-pass de-skew means running odometry, fitting a smooth
trajectory (the same cubic as `egotrack`, or a spline), re-de-skewing every point at its
time, and re-registering. It removes the lag error and costs about 2× compute offline. CT-ICP
(two poses per sweep) is needed only if mount vibration above 1/T is large. No per-sweep model
removes that, and the gate is `A < 0.20 m / r`: 0.38° at 30 m.

- **Defer:** CT-ICP until the per-sector tilt spectrum (Q17) shows more than 0.2° above 2 Hz.
- **Falsifier:** turn-submap facade p75 and range slope do not improve from single-pass to
  two-pass, or straight-line submaps fail as well.

#### 6. One line per rotation, or across rotations?

**Recommendation:** Keep one line per rotation. Add an inter-rotation continuity check, a
tail-RPM check and neighbour interpolation for failed rotations. At 20 Hz the per-rotation fit
loses nothing that matters.

With per-packet jitter σ_j and N packets per rotation, the line's RMS error over the rotation is
`σ_j √(2/N)` (derivation). With σ_j = 195 µs (see E5), that is 21 µs at 10 Hz (N = 180) and
29 µs at 20 Hz (N = 90). The geometric effect `(v + rω) δt` at 15 m/s, 0.5 rad/s and 50 m is
under 1.2 mm at either rate (derivation). A multi-rotation fit would reduce intercept noise and
absorb latency steps, but it buys nothing at that scale.

Mark a rotation's times as untrusted when any of these holds:

- the fit fails (span under 300°, or period outside 0.04–0.25 s);
- `|P_fit − 60/RPM_tail| > 0.1%`;
- the continuity residual `|t0_{k+1} − t0_k − P_k| > 100 µs`;
- more than 5% of blocks are rejected;
- the kept-block RMS is more than 2× the segment median.

The rotor is speed-controlled relative to the body, so vehicle yaw does not move the period
(derivation).

What the exporter should do: today a failed fit silently falls back to packet times (fact,
`PointTimes` returns `rec`). Instead it should interpolate the clock from the neighbouring
rotations' lines (the motor is regular), write `time_source_used` per rotation in `frames.csv`,
and compute RMS over kept blocks only.

- **Falsifier:** frame-to-frame odometry residuals improve when the flagged rotations use
  interpolated clocks, which would mean the flags are too loose. The plan's own 60 s
  packet-time comparison, step 3, is the reference.

#### 7. Where the constant offset and latency do not cancel

**Recommendation:** Ignore them in (a) and in ego speed. In the radar comparison, compare only
where `|a| < 0.5 m/s²`, or estimate the inter-clock offset by cross-correlation.

The constant is about 250 µs plus the arrival latency L (judgement: 0.05–2 ms). It cancels in
odometry, de-skew and every within-sensor speed (derivation: all times shift together). It does
not cancel in three places:

1. **Comparison with another clock (radar).** A speed mismatch of `|a| δ` (derivation). For
   δ ≤ 2 ms and 3 m/s² that is 6 mm/s. If the radar runs on another host, δ is the host-clock
   offset (judgement: 1–50 ms), up to 0.15 m/s at 3 m/s². Restricting to `|a| < 0.5 m/s²` bounds
   it at 0.025 m/s.
2. **Latency steps between rotations.** A 1 ms step tags one pose wrongly. At 15 m/s that is a
   15 mm position-equivalent, about 0.02 m/s on one ±0.5 s fit. The continuity check catches it.
3. **Absolute time** against any external log.

The per-rotation intercept noise (about 30 µs) is a 0.45 mm position-equivalent at 15 m/s, which
is negligible.

- **Falsifier:** a cross-correlation of LiDAR ego speed against radar speed shows a lag of more
  than 50 ms.

#### 8. Session-seconds time in LAS, and what `run.json` must carry

**Recommendation:** Session seconds is the right choice. Make the LAS files self-describing, and
carry the per-rotation clock and calibration so that a later run can re-time points without the
captures.

LAS 1.4 has no "not GPS" setting. Global-encoding bit 0 clear means GPS week time (fact,
`las.go` comment), and setting it would assert adjusted standard GPS time, which is worse. Keep
bit 0 clear and add a text VLR stating the epoch and the convention. `run.json` and `frames.csv`
must carry:

- `epoch_utc` and its clock (PCAP host, synchronisation unknown);
- per rotation: `t0` at a stated unwrapped-azimuth origin, slope, kept and rejected blocks, kept
  RMS, `time_source_used`, tail RPM and packet gaps;
- the angle and firetime calibration table with its hash, and the motor speed used for the
  firetime azimuth correction;
- the block azimuth per point, or the per-channel azimuth correction, because the LAS
  `azimuth_deg` is the corrected azimuth (fact, `extract.go` line 597);
- the offset convention: the recorded time names the packet, plus arrival latency.

Today `frames.csv` lacks `t0` and the azimuth origin, and the LAS lacks the block azimuth (fact).
So re-timing currently needs the PCAPs.

- **Falsifier:** re-timing from `run.json` plus LAS differs from a fresh export by more than 1 µs.

### C. Minimum for (a): odometry, submaps and prior factors

#### 9. The smallest new code

**Recommendation:** Four Gerardus pieces: a matcher trait, a voxel-hash map, a keyframe bridge
graph with handover and prior factors, and `graph.rs` fixes. Drop KISS-ICP's adaptive threshold,
point-to-point, loop closure and Scan Context.

1. **Matcher trait (S).** `accumulate` and `icp` become generic over `matched(p, gate)`.
   `Reference` implements it unchanged. Needed because `Reference::matched` reads the 3DEP
   ground raster (fact).
2. **Voxel-hash map (M).** Voxels of 0.5 m, each keeping `(count, Σx, Σxxᵀ)` and up to 20 points.
   A plane and planarity weight come from the moments (VGICP-style). Queries scan 27
   neighbours, and voxels beyond 100 m of the current pose are evicted.
3. **Front end (M).** Constant-velocity prediction, the existing gate schedule `gates[1..]`
   (1.0, 0.5, 0.3 m) from the prediction, with one retry through the 2.0 m gate when fewer than
   30% of points match. Two-pass de-skew (Q5). The self and co-moving mask (Q12).
4. **Relative-pose factor (S).** `z = T_i⁻¹ T_j` between keyframes, with covariance from the
   calibrated per-metre model (Q10). Because `Factor::Between` errors are in site axes,
   `info_site = B info_body Bᵀ` with `B = blockdiag(R_i, R_i)` (derivation from `minus`).
5. **Keyframes:** every 5 m, 10° or 2 s, whichever comes first. The 2 s rule keeps nodes at
   stops. Per-rotation poses are odometry poses corrected by SE(3) interpolation of the keyframe
   corrections. Graph nodes number about 400 for 2 km, inside the dense solver's reach
   (6N = 2,400).
6. **Submaps:** 50 m of travel, broken at each heading change over 30°, at most 150 m.
   `SCAN_KIND_MAP` into `evaluate(refine)` (existing).
7. **`graph.rs` (S):** compute covariance by one inversion of H, not per-node Schur complements,
   and return the off-diagonal blocks for neighbouring nodes (needed for Q16); add a
   prior-outlier loop (Q11); solve in a frame local to A.

Defer: KISS-ICP's adaptive threshold (the constant-velocity prediction error at car speed is
centimetres, inside the 0.3 m final gate), two-level downsampling, Scan Context and loop
closure.

- **Falsifier:** more than 1% of moving rotations fail to converge, or the retry rate exceeds 5%.

#### 10. Calibrating the odometry-factor covariance

**Recommendation:** Replace the ICP information's scale with a per-metre, per-radian and
per-second random-walk model. Keep ICP information only for its shape (degeneracy). Calibrate
on submap-to-submap innovations, and check against the held-out anchor and the stops.

```text
Σ_step = Q_m · Δs + Q_r · |Δψ| + Q_s · Δt        (body axes of the earlier keyframe)
Q_m = diag(q_∥, q_⊥, q_z, q_roll, q_pitch, q_ψ)  per metre
Q_r = de-skew residue per radian turned;  Q_s = floor per second (stops)
```

Where ICP's scaled information has an eigen-direction below 1e-2 of its maximum (a corridor),
inflate that direction by 100×.

Calibration on the drive itself (derivation, method of moments): each pair of accepted
consecutive priors i, j gives an innovation `ν = (prior_j ⊖ prior_i) ⊖ odo_ij` in their strong
directions, with `E[ν²] = q Δs + Q_r Δψ + Q_s Δt + σ_p,i² + σ_p,j²`. Thirty submaps give about 30
pairs per across direction, enough for a non-negative least-squares fit of `q_⊥`, `q_ψ` and `q_z`.
`q_∥` comes only from intersection pairs (fewer, about 5–10). `Q_s` comes from the stops.

Check: the A-only B residual's Mahalanobis distance under the calibrated model. The model holds
at both rates because it counts metres and seconds. A 20 Hz run with twice the steps per metre
and per-step noise `√2` larger gives the same `q` (derivation, equal information per second).

- **Falsifier:** normalised innovation χ²/dof outside 0.5–2, or a held-out B Mahalanobis² above
  11.3 (χ²₃ at 0.99).

#### 11. Mid-block prior factors and lot-period aliases

**Recommendation:** Add mid-block factors with weak directions projected out. Keep a direction if
its scaled eigenvalue is at least 1e-2 of the maximum and stays at least 1e-3 when recomputed from
points more than 2 m above ground. Keep aliases out by gating on the chain's predicted σ_∥, with
an explicit shifted-start separation test and a whitened-residual outlier loop.

Mid-block, the across and yaw information is the bulk of what priors offer (maths §1.2). Leaving
it out wastes the factor. The 1e-2 share is `WEAK_SHARE` (fact), stricter than `RANK_SHARE`
(1e-3), because `n_eff` and parked-car pairings (maths §1.4) prop up λ(t_∥). The 2 m rule
excludes cars.

Aliases (`evaluate` has no separation, E14):

1. Accept the along-street component only if the chain's predicted `σ_∥ ≤ 0.6 m`. A 3.7 m alias
   is then more than 6σ away and outside the 1.0 m first gate's basin.
2. Otherwise run `icp` from the chain pose shifted by ±3.7 m and ±7.6 m along the weak
   eigenvector, and accept only if `cost(second)/cost(best) ≥ 1.25` (`GATE_SEPARATION`) and the
   best lies within 3σ of the chain.
3. After each solve, drop any prior whose whitened residual exceeds χ² at 0.99 for its kept rank,
   then re-solve once.

Process submaps in route order from A so that each accepted intersection resets σ_∥.

- **Falsifier:** an accepted prior's along shift lies within 0.3 m of ±3.7 m or ±7.6 m from the
  chain, or B's along residual is near a lot period.

#### 12. Moving traffic

**Recommendation:** The Huber kernel is not enough: it never reaches zero weight, and it cannot
see co-moving vehicles. Add a redescending cut (zero weight beyond 0.30 m) and a sensor-frame
consistency mask. Neither needs the background model.

A vehicle travelling with the car is static in the sensor frame, so it biases odometry towards
zero relative motion with consistent residuals that a kernel cannot detect (derivation). The mask
works per point. Compare the residual against the previous rotation under the predicted
world-frame motion with the residual under no motion (sensor frame). Label a point co-moving or
self when the sensor-frame residual is under 5 cm and the world-frame residual is over 20 cm.
Surfaces parallel to the motion fit both and stay in, which is harmless because they carry no
along-track information. Self-returns get a 2.5 m minimum range plus a sensor-frame mask learned
on the static-roof interval. Note that `prepare_scan` drops only under 1 m (fact).

At stops the sensor is stationary and the background model would work, but its 30 s warm-up rules
it out for the minimum.

- **Falsifier:** residuals and drift at stops differ between high- and low-traffic stops by more
  than 2σ, or straight-line speed changes by more than 1% when the mask is switched off.

#### 13. Acceptance for (a) on the first drive

**Recommendation:** Per-submap static gates reported by yaw rate. Held-out B judged by
Mahalanobis distance and an along-street limit of 1.85 m, not 2% of distance. Minimum counts
stated in advance.

- **Per submap:** ground p95 ≤ 0.20 m, facade p75 ≤ 0.20 m, range slope ≤ 0.0087 (existing), with
  at least 100 facade residuals (`MIN_FACADE_POINTS`). Report rank, weak directions, separation
  where tested, dynamic fraction, mean speed and maximum yaw rate.
- **Per drive:**
  - A-only B residual: Mahalanobis² ≤ 11.3 (horizontal + yaw), |e_∥| ≤ 1.85 m (half the smallest
    lot period), and the 2% figure reported alongside;
  - A+B closure χ²/dof ≤ 3;
  - at least 70% of submaps pass all three gates;
  - both triangle closures (Q2) within 0.2 m.
- **Minimum counts:** at least 8 gated submaps, at least 2 intersection submaps with rank 6 and
  separation ≥ 1.25, and at least 3 stops of 5 s or more.

What a pass establishes: on this route, at 10 Hz, from a roof tripod, the bridge reproduces B
within its stated uncertainty, and the submaps sit on 3DEP. What it does not establish: accuracy
mid-route (nothing independent is observed there); any other route, mount or rate; the absence of
a common 3DEP datum offset; or anything about backpack or bike rigs.

- **Falsifier:** a pass that coexists with a triangle closure over 0.2 m.

### D. Minimum for (b): the ego track, speed, jerk and jitter

#### 14. Local cubic, RTS smoother or spline

**Recommendation:** Keep the local weighted cubic as the minimum, with a corrected σ model (Q16).
Treat a constant-jerk RTS smoother as the shared family to converge on later. Cross-check once
against the L5 smoother.

The local cubic is a fixed, linear-phase, symmetric filter with an exact covariance under its own
noise model, and no process noise to tune (fact, `kinematics.go`). An RTS smoother with a
constant-acceleration model and white jerk is equivalent to a cubic smoothing spline (Wahba;
Kalman–spline equivalence). Its bandwidth is set by `q/r`, which is a window in disguise. It
handles gaps and uneven sampling better and can carry correlated error as augmented state. A
continuous-time spline inside the pose graph is the ideal but costs L. A shared family is worth
having so that "σ" means the same in tracker and ego outputs, but that is a later convergence,
not a precondition.

- **Defer:** CJ-RTS in `egotrack`; the spline in Gerardus.
- **Falsifier:** an RTS-CA run on the same trajectory, tuned to the same bandwidth, differs in
  speed by more than 2σ on more than 5% of samples.

#### 15. Window choice for braking onsets

**Recommendation:** Keep speed, acceleration and yaw rate at ±0.5 s at both rates. Report jerk
from a separate ±0.65 s fit at both rates, scaled by `(σ_pos / 1 cm)^(2/11)` once σ is measured.
Report jerk only above 3σ.

Bias (derivation, continuous uniform window, `u = ωh`): the local cubic's jerk estimate responds to
a sinusoid with gain `R_j(u) = −6 c3(u)/u³`. It is 0.948 at u = 1, 0.797 at 2, 0.591 at 3 and
0.560 at π. For speed, `R_v` is 0.972 at u = 2 and 0.857 at π. For acceleration, `R_a(π/2)` is
0.835. The small-u forms are `1 − u²/18` (jerk), `1 − u²/14` (acceleration) and `1 − u⁴/504`
(speed).

Noise: E10's exact values give `σ_j(h) ≈ 0.763 (0.5/h)^3.33 m/s³` at 10 Hz and 1 cm.
Mean-square error for a braking onset with a 1 s rise to 2 m/s² (peak jerk about 3 m/s³,
content near 0.5 Hz, ω = π), at 10 Hz and 1 cm (derivation):

| Half window | 0.50 s | 0.55 s | 0.60 s | 0.65 s | 0.75 s |
| ----------- | ------ | ------ | ------ | ------ | ------ |
| MSE (m/s³)² | 0.73   | 0.51   | 0.46   | 0.50   | 0.73   |

So the optimum is about 0.6 s, with an RMS error of about 0.7 m/s³ (23% of the peak). Only
h ≥ 0.64 s gives `3σ_j ≤ 1 m/s³`, so I choose ±0.65 s. Asymptotically
`h_opt^11 ∝ σ² / (J² ω⁴ f)` (derivation), so h barely depends on rate (`f^(−1/11)`, 6% from 10 to
20 Hz) and modestly on σ. At 2 cm (the code default) it moves to about 0.7 s. At 20 Hz with √2
noisier poses the optimum is also about 0.6 s.

Acceleration at ±0.5 s under-reads 0.5 Hz onsets by about 16%. Its MSE optimum at 1 cm is about
±0.35 s, but at 2 cm that is too noisy (σ_a ≈ 0.24 m/s²), so state the bias instead.

- **Domain:** onsets with rise times of 0.5–2 s.
- **Falsifier:** on synthetic onsets with the measured σ, the ±0.65 s jerk RMS error exceeds the
  ±0.5 s one.

#### 16. Correlated (random-walk) odometry error

**Recommendation:** Add a three-term σ model to `egotrack`: white (from fit residuals),
random-walk (`q_s` per metre from Q10) and scale bias (from the closure). Use along-track σ, not
per-axis σ. Graph marginals are the later refinement.

For position error that is a random walk with `q = q_s · v` (m²/s), the cubic's estimators give
(derivation: integrate the estimator weights to `G(τ)` and use `Var = q h ∫ G(τ)² dτ`):

```text
σ_v = 1.118 √(q / h)     σ_a = 1.464 √(q / h³)     σ_j = 5.916 √(q / h⁵)
```

Compare white noise: `σ_v = 3.06 σ / (h^1.5 √f)`. The random-walk terms are **independent of pose
rate** and fall more slowly with window length. So a single inflation factor cannot correct both
windows, and wide windows and 20 Hz look better than they are under the white model.

A constant scale error `ε_s` (along-track slip in corridors) biases speed by `ε_s · v`: 1% at
15 m/s is 0.15 m/s. No window removes it, and the closure bounds it as `|e_∥| / segment length`.

Speed is the along-track derivative, and along-track is ICP's weakest direction, so
`SpeedSigma = per-axis σ` understates it (fact plus derivation). The trajectory should carry
along, across and vertical σ, or a 3×3 covariance.

```text
σ_v² = σ_white,fit² + 1.25 q_s v / h + (ε_s v)²
```

Graph marginals (with `graph.rs` returning off-diagonal blocks) capture only the modelled
random-walk part, at keyframe resolution, so they are not a substitute for the white term.

- **Defer:** full window covariance from the graph.
- **Falsifier:** the empirical σ from the odd/even split (Q21) falls outside 0.75–1.33 of the
  model's σ. Also: an Allan deviation of stationary positions at stops with a slope that is
  neither −1/2 (white) nor +1/2 (random walk).

#### 17. What can honestly be called jitter

**Recommendation:** Call the per-rotation residual spectrum "attitude and height residual,
1 Hz to Nyquist" (1–5 Hz at 10 Hz, 1–10 Hz at 20 Hz). Measure the higher band with per-sector
ground-plane tilt, not per-sector 6-DoF poses. Gain-correct only where `|1 − H| ≥ 0.5`.

Below about 1 Hz the ±0.5 s fit follows the motion, so `|1 − H|` is small. Above Nyquist, energy
aliases. The per-rotation pose is a sweep average with a gain of roughly `|sinc(f T)|`
(judgement: ICP is not an exact time average), so aliasing is attenuated:

| Vibration | 10 Hz rotation        | 20 Hz rotation        |
| --------- | --------------------- | --------------------- |
| 7 Hz      | 0.37, aliased to 3 Hz | 0.81, resolved        |
| 12 Hz     | 0.16, aliased to 2 Hz | 0.50, aliased to 8 Hz |
| 15 Hz     | 0.21, aliased to 5 Hz | 0.30, aliased to 5 Hz |

E13's synthetic 7 Hz alias probably over-states real aliased amplitude, if its poses are
instantaneous (judgement). Car body modes (1–2 Hz) are honestly observed at both rates. Wheel hop
(10–15 Hz) is observed at neither. Tripod and strap modes (judgement: 5–20 Hz) fall partly within
the 20 Hz band.

**Per-sector ground tilt (derivation).** Split each rotation into 8 sectors of 45° and fit the
ground plane, 5–25 m away, in the sensor frame. Each sector constrains height and the tilt about
the axis perpendicular to its bearing. That is 80 samples/s for height and at least 40/s per tilt
axis (Nyquist about 20 Hz) from 10 Hz rotations; double that at 20 Hz. With about 2,000–4,000
ground points per sector the formal tilt σ is about 0.002°. Road texture shorter than the 20 m
fit footprint averages out, and longer road shapes appear below 1 Hz at 15 m/s. Subtract a 1 s
running mean per sector index. Cost: one plane fit per sector, no map. Full 6-DoF per-sector poses
need the map and a continuous-time prior (CT-ICP-like, L) and are not the minimum.

Gain correction is sound for a narrow peak where the gain is not small. Below 0.5 it amplifies
noise by more than 2×, so raise the code's 0.1 threshold to 0.5 and always report raw and
corrected values. At 10 Hz, guard against aliases by flagging in-band peaks that coincide with
`|f_k − m f_s|` for peaks the per-sector spectrum finds above 5 Hz.

- **Falsifier:** at stops with the engine idling, the per-sector and per-rotation spectra disagree
  by more than 3 dB in the shared band.

#### 18. Stops as a drift check

**Recommendation:** Detect stops independently of the trajectory, by raw scan differencing. Use
zero-velocity factors in the product solve only, and judge the check on the A-only solve without
them, or hold out alternate stops.

Today stops are found from the trajectory's own speed, which is circular: an odometry drifting at
0.4 m/s during a stop is never labelled a stop, so it can never fail the check (fact,
`analysis.go` `Stops`). Stop displacement tests only time-driven drift (dynamic objects, map
degeneration), not per-metre drift. A stopped car amid crossing traffic is exactly the dynamic-bias
test, so report it per stop with the dynamic fraction.

Rule for the speed threshold: `StopSpeed ≥ max(0.3 m/s, 4σ_v)`. At the 5 Hz ablation with 2 cm σ,
σ_v ≈ 0.095 m/s and the Maxwell 99th percentile (about 0.32 m/s) exceeds 0.3, so stops would
fragment (derivation).

- **Falsifier:** the independent detector and the trajectory disagree on more than 10% of stop
  time.

#### 19. What belongs in which repository

**Recommendation:** Gerardus writes poses, pose covariance (at least along, across and vertical
σ), quality, the de-skew twist it used, and the calibrated noise parameters (`σ_w`, `q_s`, `ε_s`
bound). velocity.report alone derives velocity, acceleration, jerk, yaw rate, stops and jitter.

This keeps one producer for every derivative and one statement of bandwidth (judgement). The de-skew
twist is provenance: an input to point geometry, labelled as such, not an ego-velocity product.
The route plan's trajectory-contract "velocity" field is then filled from `egotrack` output. The
noise parameters let `egotrack` form correlated σ without shipping a full covariance matrix.

- **Falsifier:** any consumer differentiates Gerardus's twist as a speed product.

### E. Evaluation and its traps

#### 20. Ranking the independent references for (b)

**Recommendation:** For the archive: held-out B, then the DataSF route length (moving intervals
only), then stops. Later, the fixed radar is the only direct speed reference and should rank
first.

| Reference                            | Tests                                                   | Misses                                                                                                                                          |
| ------------------------------------ | ------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| Fixed radar (needs a new capture)    | speed directly, at a point                              | everything elsewhere; cosine error; sensor-point versus body speed (the lever arm: `h · dθ/dt` up to about 0.2 m/s at braking onset, judgement) |
| Held-out B                           | integrated position: the mean scale error `ε_s ≈ e_∥/L` | errors that integrate to zero; bandwidth; jerk; it is one sample                                                                                |
| Route length from DataSF centrelines | scale to about 0.5%                                     | needs the route (circular if the trajectory is wrong); lane offsets at turns; parking moves                                                     |
| Stop displacement                    | time-driven drift, dynamic bias                         | anything in motion                                                                                                                              |

Report both sensor-point kinematics and a road-reference point (the sensor pose shifted down its
own z axis by the ground-plane height). Their difference bounds lever-arm contamination
(derivation).

- **Falsifier:** route length and B disagree on scale by more than 1%.

#### 21. How each success criterion could pass while wrong

**Recommendation:** Add one check per criterion, as below. Two of the six are tautological as
written.

| Criterion                                         | Passes while wrong when                                                                                                                         | Extra check                                                                                                  |
| ------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------ |
| 1. 99% `clock_ok`, no unexplained period outliers | the host clock slews (NTP, up to 500 ppm), or latency steps                                                                                     | fitted period against tail RPM within 0.1%; continuity residual p95 ≤ 100 µs                                 |
| 2. B within 2% (32 m on 1.6 km)                   | alias at B; consistent but wrong priors; handover error cancels drift                                                                           | Mahalanobis and 1.85 m along limit; triangle closures; direct roof-to-3DEP at B                              |
| 3. Stop displacement < 0.3 m on A+B               | stops detected from the trajectory (circular); zero-velocity factors; scan-to-map "sticks"                                                      | independent stop detector; A-only without zero-velocity factors                                              |
| 4. Distance within 1%; median speed σ < 0.1 m/s   | σ is stated, not measured (it passes for any σ_pos < 4 cm); stop noise inflates both distances, the solved path length by about 0.02 m per step | empirical σ from the odd/even split; distances over moving intervals only, with path length at ≥ 1 m spacing |
| 5. Jitter reported with Nyquist and gain          | aliased peaks reported as real                                                                                                                  | per-sector tilt spectrum to about 20 Hz                                                                      |
| 6. Ablation reported                              | any table passes                                                                                                                                | predictions stated before the run (Q28) and scored                                                           |

The odd/even split (derivation) works like this. Export `keep-every-rotation 2` at phase 0 and at
phase 1. That gives two disjoint 5 Hz streams and two independent odometry runs. Their speed
difference divided by √2 is the empirical σ of the independent part. Common-mode errors, such as
geometry-driven slip, cancel and stay unmeasured, which is why B remains necessary.

#### 22. Is one drive enough to choose defaults?

**Recommendation:** No. Choose defaults on unanchored continuous drives with anchor-free
diagnostics. Test them on anchored drives. The smallest anchored set is the first pair, the two
continuous one-registered-end drives, and the day-2 chain.

One drive with three to eight stops fits whatever is tuned on it, and leaves no test. Window,
stop threshold and submap length can all be chosen without anchors:

- the window from the odd/even σ and theory;
- stops from the independent detector, calibrated on the static ends;
- submap length from the gate pass rate against length.

Use all 14 continuous drives for this. The test set:

- lombard-laguna → columbus-north-point (both ends registered);
- broadway-franklin → laguna-eddy, and marina-webster-beach → lombard-laguna (registered end as
  anchor, manual end as alias test, Q3);
- the day-2 chain lombard-broderick → marina-broderick → marina-webster-beach → lombard-laguna →
  columbus-north-point (four continuous drives, two registered sites), solved as one chain with
  the manual sites as checks.

The other five of E2's seven cross recorder restarts and cannot be bridged (Evidence, E2).

- **Falsifier:** defaults chosen on unanchored drives fail the anchored tests that defaults chosen
  on the first pair pass.

### F. Rotation rate: SLAM at 10 Hz and 20 Hz

#### 23. How 20 Hz changes scan-to-map odometry

**Recommendation:** At urban car speeds the de-skew residue in turns dominates, and 20 Hz cuts it
by 4×. At walking pace, sway residue dominates the same way. Density loss matters per scan, not
per submap.

Inputs: 15 m/s, a = 3 m/s², ω = 0.5 rad/s, α = 0.5 rad/s², r = 30 m (judgement). The ablation
column is 10 Hz kept one in two (100 ms sweeps, 200 ms spacing).

| Quantity                                         | 10 Hz       | 20 Hz         | Ablation                    | Basis                                                |
| ------------------------------------------------ | ----------- | ------------- | --------------------------- | ---------------------------------------------------- |
| Inter-scan travel, rotation                      | 1.5 m, 2.9° | 0.75 m, 1.4°  | 3.0 m, 5.7°                 | `v T_s`, `ω T_s`                                     |
| Prediction error, translation                    | 0.03 m      | 0.008 m       | 0.12 m                      | `a T_s²`                                             |
| Prediction error, yaw                            | 0.29°       | 0.07°         | 1.15°                       | `α T_s²`                                             |
| Inside the ICP basin (about 1 m, 4°; maths §1.7) | yes         | yes           | yes, unless an abrupt event |                                                      |
| Smear without de-skew                            | 1.5 m       | 0.75 m        | 1.5 m                       | `vT`, `ωTr`                                          |
| Residue, translation                             | 0.045 m     | 0.011 m       | about 1.5× native           | `1.5 aT²`; the ablation's velocity lags twice as far |
| Residue, yaw at 30 m                             | 0.225 m     | 0.056 m       | about 1.5× native           |                                                      |
| Points per scan, ideal                           | 72 k        | 36 k          | 72 k                        | E6                                                   |
| Far spacing at 50 m                              | 0.175 m     | 0.349 m       | 0.175 m                     | E6                                                   |
| Compute per second                               | 1           | about 1.0–1.5 | about 0.5                   | points/s equal; per-scan overhead doubles            |

Per-scan registration σ at 20 Hz is 1.0–1.4× that at 10 Hz (judgement): near range is
voxel-saturated at both rates, far range halves. At walking pace (1.4 m/s) the smear is 0.14 m,
but sway at about 79°/s² (maths §4.1) leaves 0.4 m at 20 m at 10 Hz and 0.1 m at 20 Hz.

- **Falsifier:** at 20 Hz, turn-submap gate pass rates are no better than at 10 Hz.

#### 24. Register each 20 Hz rotation alone, or merge pairs?

**Recommendation:** Register each rotation alone against the map. Merge pairs only as a fallback
where a single scan degenerates. Prior factors are unaffected.

Merging restores 100 ms of motion within the sweep and halves the pose rate, giving back the two
gains 20 Hz buys. The map supplies the density. Prior factors come from submaps, which aggregate
the same points per second either way.

- **Falsifier:** the 20 Hz per-scan failure rate exceeds 1%, or the per-metre `q` exceeds 1.5× the
  10 Hz value.

#### 25. Parameters that must be in seconds or metres, and inherited rate-coupled contracts

**Recommendation:** Fix every SLAM and ego parameter in metres, seconds, degrees or per-metre
rates (see the rate table). Fix four inherited contracts before any 20 Hz run.

Rate-coupled contracts the SLAM path would inherit (fact, code and audit):

- `AzimuthStride` counts blocks (4 means 0.8° at 10 Hz and 1.6° at 20 Hz). State it in degrees.
- `GapFactor` is relative to the median interval (0.3 s at 10 Hz, 0.15 s at 20 Hz, 0.6 s in the
  ablation). Add `MaxGapS = 0.35 s`.
- `DefaultSigmaPosM` is per pose. State it as a white PSD `σ² Δt` (m²·s), so 1 cm at 10 Hz
  becomes 1.41 cm at 20 Hz.
- `MinAzimuthCoverage = 340°` couples to field of view, not rate (Q30).

Outside the SLAM path: `SpatialCoverage` (duration × 10), `MaxOcclusionFrames` (100 ms), the
scene `STRIDE = "2"`, and the frame builder's 36,000-point pre-allocation (performance only).
Derive cadence from the measured `rotation_hz` before route tracking reuses them.

- **Falsifier:** a config diff between the 10 Hz and 20 Hz arms touches anything other than the
  input files.

#### 26. Are the prior gates rate-invariant?

**Recommendation:** Treat them as rate-invariant on straight submaps, but not in turns. Report
gates stratified by maximum yaw rate.

Submaps aggregate the same points per second over the same distance, so ground p95 and facade p75
statistics match to first order (derivation). Two exceptions:

1. **Turns at 10 Hz.** De-skew yaw residue at 0.5 rad/s² is 0.0075 rad, almost the range-slope
   gate (0.0087). Turning submaps may fail the slope gate through de-skew, not misregistration.
   The prediction is 10 Hz worse than 20 Hz, and the ablation worse than 10 Hz.
2. **20 Hz.** Noisier per-scan poses blur the submap by their σ (centimetres), a small rise in
   p75.

Sparse far structure per scan matters for odometry, not for submaps.

- **Falsifier:** straight submaps differ by more than 0.03 m in p75 between rates.

#### 27. Comparable σ and bias across rates

**Recommendation:** Use the same windows in seconds at both rates, and compare the white PSD and
`q_s`, not per-pose σ. Same units and windows are necessary but not sufficient. Comparisons also
need the same route, matched segments, stratification by speed and yaw rate, the same σ model,
and a common 1–5 Hz jitter band.

Under equal information per second (`σ_20 = √2 σ_10`), derivative σ over a fixed window is equal
(derivation, continuum limit; E10 synthetic within 0.8–1.25), and bias depends only on the window
in seconds. Random-walk terms are rate-independent anyway. The 5 Hz ablation's ±0.5 s cubic has
only 5 samples and 1 degree of freedom, so one missing pose breaks it. Use ±0.75 s for the
ablation comparison, or report it as fragile.

- **Falsifier:** empirical σ ratios outside 0.8–1.25 on matched segments.

#### 28. What the ablation predicts, and the minimum native 20 Hz capture

**Recommendation:** The ablation tests sensitivity to inter-scan displacement, and the white
versus random-walk noise structure. It predicts nothing about 50 ms sweeps or half-density scans.
The minimum native capture is a paired ABA (10–20–10 Hz) drive of the first-pair route on one day,
same mount, 360° field of view.

What the ablation measures:

1. Convergence and prediction error at doubled inter-scan motion.
2. The noise structure. Per-pose σ is unchanged, so speed σ at ±1 s rises by 1.35× under white
   noise (E10: 0.0123/0.0091) and about 1.0× under random walk. The measured ratio decides which
   (derivation).
3. A halved jitter band.

It does not reproduce 200 ms or 50 ms sweeps. De-skew residue rises only about 1.5× through the
lagged velocity (derivation, Q23), not 4×.

Only a native 20 Hz capture answers:

- per-scan σ at 36 k points;
- residue with 50 ms sweeps;
- the 5–10 Hz band;
- whether `σ_20 = √2 σ_10`;
- real clock fits on 90-packet rotations;
- frame builder and recorder behaviour at 20 Hz.

**Minimum capture (judgement):**

- the same car and roof tripod, with the mount measured;
- lombard-laguna → columbus-north-point (both sites registered), with at least 60 s stationary on
  the roof and a fresh ground tripod placement at both ends;
- at least 4 turns, at least 3 stops of 10 s or more;
- driven three times back to back, 10, 20 then 10 Hz, to bracket traffic drift;
- single return, as in the archive (confirm the archive's return mode);
- full 360° field of view;
- motor setting confirmed from the tail RPM;
- optionally a fixed radar at one mid-route point.

The same route must be driven at both rates, because route geometry dominates registration
(maths §1.5).

Comparisons:

- native 20 against native 10, on matched segments (physical);
- 20 decimated to 10 against native 10 (same pose rate; differs in sweep length and density, a
  confounded pair, labelled so);
- 20 against 20 decimated to 10 (same drive; pose rate only).

- **Falsifier:** the ABA 10 Hz runs differ from each other more than 10 Hz differs from 20 Hz.
  Then traffic or route variation dominates and the comparison says nothing.

#### 29. Rotation rate by platform

**Recommendation:** For SLAM on any moving platform, use 20 Hz (car, bike, backpack). For traffic
measurement, keep 10 Hz on fixed tripods until the audit's qualification passes. For mobile
traffic measurement, decide after phase 2. Nothing in bandwidth binds the recorder at 20 Hz.

20 Hz quarters the dominant residue (turns, sway, vibration up to its Nyquist) at 1.0–1.5×
compute, which is irrelevant offline. Fixed traffic measurement depends on per-rotation density
for far, small objects, and its 20 Hz qualification is unrun (fact, audit).

The recorder is unchanged at 20 Hz: the same packet rate (about 1,800 packets/s, 18.84 Mbit/s in
single return, derivation from E6). What could bind is per-rotation live processing, if it runs
during capture (48 ms), the frame builder's field-of-view completion (Q30), motor power and heat
(I don't know), and whether the firmware keeps its rate setting.

- **Falsifier:** native 20 Hz shows more than 0.1% packet loss, or tail RPM unstable beyond 0.1%.

#### 30. The sector-limited fixture: defect or property?

**Recommendation:** A defect. Make completion relative to the configured or observed field of view,
and add a hard cap of twice the tail-RPM period. The exporter should accept declared sectors and
refuse undeclared ones by name.

The code already anticipates sector wraps (`azimuth_wrap_large_jump`, commented "289° -> 61°") and
then gates them on 340°, which a sector cannot reach (fact). The missing duration and size cap
lets one frame grow until memory runs out, a robustness defect whatever the field of view (fact,
E8).

The fix: completion at `coverage ≥ FOV_span − 20°`, with FOV from configuration or learned from
the first wraps, and finalise as incomplete after `2 × 60/RPM_tail` seconds or `2 ×` the expected
points. The exporter gains a `-fov a:b` option, fits the clock when the span is at least 0.8 × FOV,
marks `sector` in `frames.csv`, and otherwise refuses with the reason.

- **Falsifier:** after the fix, the 60–290° fixture yields frames at 20 per second, each spanning
  about 230°.

### G. Sequencing

#### 31. Order of work

**Recommendation:** Two phases, as in the work order below. Phase 1 runs on the 10 Hz archive and
asks whether the bridge works at all. Phase 2 adds native 20 Hz and the wider drive set, and runs
only if phase 1's gate says continue.

The boundary falls here because the two phases' failure modes are separable (judgement). If phase
1 fails on along-street observability or handover, 20 Hz cannot rescue it: prior observability is
rate-invariant (Q26). If phase 1 fails only in turning submaps, that is de-skew, which 20 Hz
addresses directly, and it justifies the capture day. Native 20 Hz work therefore lands entirely in
phase 2, including the frame-builder fix, which the archive does not need.

#### 32. Decisions for the owner before implementation

**Recommendation:** Eleven decisions, each with a default. They are listed under
[Open questions](#open-questions).

## Gates and defaults

All values are proposals, to be fixed before results exist. "Rate" says how each depends on
rotation rate.

| Name                            | Value                                                | Units     | Domain             | Rate                        | Ref      | Falsifier                                  |
| ------------------------------- | ---------------------------------------------------- | --------- | ------------------ | --------------------------- | -------- | ------------------------------------------ |
| Period against tail RPM         | ≤ 0.1                                                | %         | every rotation     | none                        | Q6       | flagged rotations show no odometry change  |
| Clock continuity p95            | ≤ 100                                                | µs        | moving and static  | σ ×√2 at 20 Hz, still below | Q6       | odometry improves with interpolated clocks |
| Untrusted rotation              | > 5% rejected, or kept RMS > 2× median               | –, µs     | every rotation     | none                        | Q6       | as above                                   |
| Lift detection                  | height step ≥ 0.5                                    | m         | ground ↔ roof      | none                        | Q4       | not exactly two transitions                |
| Handover registration           | rank 6, separation ≥ 1.25, seeds ≥ 0.8, p75 ≤ 0.10   | m         | static roof ≥ 10 s | none                        | Q4       | direct 3DEP disagrees > 0.2 m              |
| Self mask                       | min range 2.5 + learned mask                         | m         | roof mount         | none                        | Q12      | stop speed not ≈ 0                         |
| Co-moving mask                  | sensor residual < 0.05, world residual > 0.20        | m         | speed > 1 m/s      | none                        | Q12      | speed changes > 1% when off                |
| Redescending cut                | 0.30                                                 | m         | odometry           | none                        | Q12      | as above                                   |
| Keyframes                       | 5 / 10 / 2                                           | m / ° / s | 0–15 m/s           | none                        | Q9       | B residual changes > 10% when halved       |
| Submaps                         | 50, break at 30°, max 150                            | m, °      | route              | none                        | Q9       | pass rate not monotone in length           |
| Kept prior direction            | ≥ 1e-2 of max, and ≥ 1e-3 from points above 2 m      | ratio     | every submap       | none                        | Q11      | parked-car pairings dominate λ(t_∥)        |
| Along-street prior gate         | chain σ_∥ ≤ 0.6, else separation ≥ 1.25 at ±3.7/±7.6 | m         | every submap       | none                        | Q11      | shift within 0.3 m of a period             |
| Prior outlier                   | whitened χ² > 0.99 quantile                          | –         | after each solve   | none                        | Q11      | dropped priors cluster at intersections    |
| Odometry noise, initial         | q_∥ 1e-4 m²/m, q_ψ 1e-8 rad²/m, calibrated           | per m     | car                | none                        | Q10      | innovation χ²/dof outside 0.5–2            |
| Anchor σ, registered            | max(10× formal, 0.03 m; 0.05°)                       | m, °      | `REGISTERED`       | none                        | Q3       | closure χ²/dof > 3                         |
| Anchor, manual                  | along projected out; 0.10 m, 0.2°                    | m, °      | `MANUALLY_PLACED`  | none                        | Q3       | –                                          |
| Held-out B                      | Mahalanobis² ≤ 11.3 and \|e_∥\| ≤ 1.85               | –, m      | A-only solve       | none                        | Q13      | alias near ±3.7 m passes                   |
| Submap static gates             | 0.20 / 0.20 / 0.0087, ≥ 100 facade points            | m, –      | each submap        | turns at 10 Hz              | Q13, Q26 | straight submaps differ by rate            |
| Drive pass rate                 | ≥ 70% of ≥ 8 submaps; ≥ 2 rank-6 intersections       | –         | per drive          | none                        | Q13      | –                                          |
| De-skew                         | two-pass, smooth trajectory                          | –         | all                | needed most at 10 Hz        | Q5       | turns not improved                         |
| Speed, acceleration, yaw window | ±0.5                                                 | s         | onsets 0.5–2 s     | none                        | Q15      | –                                          |
| Jerk window                     | ±0.65 × (σ/1 cm)^(2/11)                              | s         | onsets 0.5–2 s     | f^(−1/11)                   | Q15      | ±0.5 s has lower RMS error                 |
| Jerk report floor               | \|j\| ≥ 3σ_j                                         | m/s³      | every sample       | none                        | spec     | –                                          |
| Speed σ model                   | white + `1.25 q_s v/h` + `(ε_s v)²`, along-track     | m/s       | moving             | random walk rate-free       | Q16      | odd/even ratio outside 0.75–1.33           |
| Stops                           | independent detector ≥ 2 s; speed ≥ max(0.3, 4σ_v)   | s, m/s    | all                | σ_v by rate                 | Q18      | > 10% stop-time disagreement               |
| Gain correction                 | where \|1 − H\| ≥ 0.5                                | –         | jitter             | none                        | Q17      | –                                          |
| Distance check                  | moving intervals, ≥ 1 m spacing, ≤ 1%                | %         | drive              | none                        | Q21      | –                                          |
| Frame completion                | FOV − 20°, cap 2 × period                            | °, s      | any FOV            | period by rate              | Q30      | sector fixture never completes             |

## Rate table

Each parameter is fixed in the unit named in the last column, and its value is identical at both
rates unless shown.

| Parameter                     | 10 Hz                                               | 20 Hz                | Fixed in      |
| ----------------------------- | --------------------------------------------------- | -------------------- | ------------- |
| Keyframes                     | 5 m, 10°, 2 s                                       | same                 | m, °, s       |
| Submaps                       | 50 m, 30° break, 150 m maximum                      | same                 | m, °          |
| Correspondence gates          | 1.0, 0.5, 0.3 m (retry 2.0 m)                       | same                 | m             |
| Adaptive threshold (deferred) | 3σ of the prediction deviation over 2 s (20 scans)  | same (40 scans)      | m, s          |
| Motion model and de-skew      | constant twist per second; two-pass; per-point time | same                 | s             |
| Odometry covariance           | Q_m per metre, Q_r per radian, Q_s per second       | same                 | m, rad, s     |
| Per-pose white noise          | σ 1 cm                                              | σ 1.41 cm            | PSD in m²·s   |
| Speed and acceleration window | ±0.5 s (11 samples)                                 | ±0.5 s (21)          | s             |
| Jerk window                   | ±0.65 s (13 samples)                                | ±0.65 s (27)         | s             |
| Welch segment                 | 6.4 s (64 samples)                                  | 6.4 s (128)          | s             |
| Jitter band                   | about 1–5 Hz                                        | about 1–10 Hz        | Hz, by nature |
| Per-sector tilt band          | about 20 Hz per axis                                | about 40 Hz          | Hz, by nature |
| Maximum fit gap               | 0.35 s (3 missing)                                  | 0.35 s (6 missing)   | s             |
| Stop                          | ≥ 2 s; speed ≥ max(0.3, 4σ_v)                       | same                 | s, m/s        |
| Anchor window                 | static-roof interval                                | same                 | s             |
| Clock period window           | 0.04–0.25 s                                         | same                 | s             |
| Azimuth stride                | 0.8° (4 blocks)                                     | 0.8° (2 blocks)      | °             |
| Frame completion              | FOV − 20°; cap 0.2 s                                | FOV − 20°; cap 0.1 s | °, s          |

## Two-phase work order

### Phase 1: the archive bridge at 10 Hz, no new capture

Run on the first pair, the two continuous one-registered-end drives, and the day-2 chain. Items
marked **(cut)** go if only a third of the effort is available.

| #   | Title                                                                                                                                                           | Repo            | Size | Inputs → outputs                                                                                                       | Continue if                                                                       |
| --- | --------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------- | ---- | ---------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------- |
| 1.1 | Motion export: per-rotation clock coefficients, time-source column, tail-RPM and continuity checks, phase option                                                | velocity.report | S    | archive pcaps → LAS + text VLR, `run.json` with calibration, `frames.csv` with `t0`, slope, source, RPM; `-keep-phase` | re-timing from `run.json` matches a fresh export within 1 µs                      |
| 1.2 | Motion export: per-rotation height, near returns and stationarity labels                                                                                        | velocity.report | S    | rotations → `frames.csv` columns; segment labels (ground, transition, roof, moving, stopped)                           | exactly two lifts found per continuous drive                                      |
| 1.3 | register/graph: matcher trait, covariance by one inversion, site-axis odometry information, prior outlier loop                                                  | Gerardus        | S    | code → unit tests (corridor rank, rotated information)                                                                 | existing tests unchanged; covariance equals the per-node Schur result within 1e-9 |
| 1.4 | slam odometry: voxel-hash map with moments, constant-velocity prediction, two-pass de-skew, self and co-moving mask                                             | Gerardus        | M    | LAS + `run.json` → per-rotation odometry poses and quality                                                             | ≥ 99% of moving rotations converge; stop drift ≤ 0.05 m per 30 s                  |
| 1.5 | slam handover and priors: roof-to-site-model anchors, 50 m submaps, weak-direction projection, alias test                                                       | Gerardus        | M    | odometry + site bundle + site model → roof anchors, submap factors with gates                                          | both handovers pass; ≥ 8 gated submaps                                            |
| 1.6 | slam bridge: solve arms O, P, A+B, keyframe-to-rotation interpolation, innovation-calibrated covariance, trajectory v0 with noise parameters                    | Gerardus        | S–M  | factors → trajectories (EPSG:7131), triangle closures, calibration                                                     | innovation χ²/dof within 0.5–2                                                    |
| 1.7 | egotrack: correlated-noise σ, separate jerk window, moving-only distance, road-reference kinematics, independent stops, combined-σ anchor check on a time range | velocity.report | S    | trajectory + segment labels → `summary.json`, `kinematics.csv`                                                         | synthetic random-walk test: stated σ within 0.75–1.3 of empirical                 |
| 1.8 | egotrack: per-sector ground-tilt vibration spectrum **(cut)**                                                                                                   | velocity.report | M    | LAS → spectrum to about 20 Hz                                                                                          | agrees with per-rotation spectrum within 3 dB in the shared band                  |
| 1.9 | Hydra: first pair, two one-end drives, day-2 chain; native, ablation and odd/even arms; authored report                                                         | evaluation      | S    | all of the above → report in `docs/`                                                                                   | gate G1                                                                           |

Also cut under a third of the effort:

- the continuity and tail-RPM checks in 1.1;
- the shifted-start alias test in 1.5 (keep the σ_∥ gating);
- solve arms O′ and P′, and the chain solve, in 1.6;
- the road-reference kinematics in 1.7.

No phase 1 item waits on a 20 Hz capture.

**Gate G1 (kill or continue), fixed now.** Continue to phase 2 only if every item holds on the
first pair, and items 1–2 also hold on the two one-end drives:

1. The clock passes the Q6 gates on at least 99% of rotations.
2. Both handovers pass, and the triangle closures are within 0.2 m.
3. Odometry converges on at least 99% of moving rotations.
4. Held-out B: Mahalanobis² ≤ 11.3 and |e_∥| ≤ 1.85 m.
5. At least 70% of at least 8 submaps pass the gates, with at least 2 rank-6 intersections.
6. Empirical speed σ from the odd/even split is within 0.75–1.33 of the model's σ.

Outcomes:

- **Kill:** item 2 fails at a registered site, or item 4 fails along-street with straight submaps
  failing too. Stop mobile registration on the archive, and redirect to feature anchors or a
  capture-day design.
- **Continue with 20 Hz first:** only turning submaps fail (5), or the slope gate fails in turns.
- **Continue without 20 Hz urgency:** everything passes.

### Phase 2: native 20 Hz and the wider archive

| #   | Title                                                                                                                                       | Repo                       | Size | Inputs → outputs                                         | Continue if                                                                            |
| --- | ------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------- | ---- | -------------------------------------------------------- | -------------------------------------------------------------------------------------- |
| 2.1 | Frame builder: field-of-view-relative completion and a hard duration cap; exporter sector mode                                              | velocity.report            | S    | code → sector fixture yields frames; tests               | the 60–290° fixture gives about 20 frames/s with bounded memory                        |
| 2.2 | Rate contract: cadence from measured rotation rate for coverage and occlusion metrics; stride in degrees **(cut)**                          | velocity.report            | S    | code → tests at 10 and 20 Hz                             | a 20 Hz stream missing half its rotations is not reported as full coverage             |
| 2.3 | Paired 10–20–10 Hz roof-tripod drive, lombard-laguna to columbus-north-point, roof and ground intervals at both ends (optional fixed radar) | capture (owner)            | M    | route, mount record → pcaps on hydra                     | `rotation_hz` within 0.1% of 10 and 20; packet loss < 0.1%; both handovers registrable |
| 2.4 | slam: rate arms from one metres-and-seconds configuration; per-scan σ and per-metre `q` by rate                                             | Gerardus                   | S    | 2.3 exports → per-arm trajectories and calibrations      | the configuration diff between arms is the input files only                            |
| 2.5 | All continuous archive drives through odometry and submap gates; chain solves with manual-site alias checks **(cut)**                       | Gerardus + velocity.report | M    | 14 drives → defaults chosen without anchors; alias flags | defaults chosen here pass the anchored tests                                           |
| 2.6 | Hydra: native 20, 20 decimated to 10, native 10; rate recommendation report                                                                 | evaluation                 | S–M  | 2.3–2.4 → report                                         | gate G2                                                                                |

Items 2.4 and 2.6 wait on the 20 Hz capture (2.3), and 2.1 must land before it. Item 2.5 can run
in parallel with the capture.

**Gate G2 (kill or continue), fixed now.** Adopt 20 Hz for mobile SLAM only if all of these hold
on matched segments:

- the turn-submap pass rate at 20 Hz is at least the 10 Hz rate;
- per-metre `q` is within 1.25× of 10 Hz;
- per-scan convergence is at least 99%;
- the empirical speed σ ratio is within 0.8–1.25;
- the two 10 Hz runs agree with each other better than 10 Hz agrees with 20 Hz.

**Kill 20 Hz** (keep 10 Hz with two-pass de-skew) if 20 Hz per-scan failures exceed 1% or `q`
exceeds 1.5× the 10 Hz value.

## What I am least sure of

| Uncertainty                                                         | Why it matters                                       | Measurement that settles it                          |
| ------------------------------------------------------------------- | ---------------------------------------------------- | ---------------------------------------------------- |
| Roof-tripod vibration amplitude above 2 Hz                          | could defeat any per-sweep de-skew at both rates     | per-sector ground tilt on the first pair (1.8)       |
| Whether the roof scans register to the site model as well as K1 did | the whole handover rests on it                       | handover gates and triangle closure (1.5)            |
| Along-street drift between intersections (κ_∥, δd)                  | sets whether priors avoid aliases and when B matters | submap innovations (1.6)                             |
| White versus random-walk noise structure                            | decides the σ model and the 20 Hz benefit            | ablation speed-σ ratio at ±1 s; Allan slope at stops |
| My continuity count (14 drives)                                     | sets the usable drive set                            | exporter join grading                                |
| Arrival latency and host-clock offset                               | only for the radar comparison                        | cross-correlation against radar                      |
| Whether `σ_20 = √2 σ_10`                                            | the 20 Hz ego-track claim rests on it                | native 20 Hz (2.3)                                   |
| Pitch-centre height for the lever arm                               | sensor against body speed                            | road-reference comparison; radar                     |
| What happened at lombard-laguna 11:32:37–11:33:32                   | A's last static part may be a different placement    | register part 8 against part 1                       |

## Open questions

### Owner decisions, with my default

1. **Handover policy.** Default: break the chain at both lifts, and re-anchor by registering the
   roof scans to the site model. Never chain odometry through a lift.
2. **Anchor σ for `REGISTERED` sites.** Default: `max(10 × formal, 0.03 m; 0.05°)` until the
   variogram gives `n_eff`.
3. **`MANUALLY_PLACED` ends.** Default: along-street projected out; never held-out evidence; used
   as alias detectors.
4. **DataSF footprints.** Default: reported, not factors, in phase 1. The measured 3DEP–footprint
   shifts (+0.60 E −1.30 N at columbus-broadway; +1.15 E −0.37 N at Kirkham) would fight 3DEP
   priors.
5. **The held-out criterion.** Default: replace "2% of distance" with Mahalanobis plus 1.85 m
   along, and report 2% alongside.
6. **Jerk.** Default: separate ±0.65 s fit, reported only above 3σ, internal only, never in
   public outputs.
7. **Ego reference point.** Default: report both sensor-point and road-reference kinematics; the
   road point is primary for the radar comparison.
8. **Native 20 Hz capture.** Default: yes, the paired 10–20–10 drive on the first-pair route,
   after G1 says continue.
9. **Mount record.** Default: the operator records the tripod's height on the roof and the strap
   layout; until then, measure the height from the ground plane.
10. **Route plan Item 2.** Default: stop treating the roof-tripod car drives as a bike vibration
    proxy.
11. **Privacy.** Default: only per-drive summary statistics in Git. No route geometry, no
    per-stop times or positions.

### What only measurement can settle

- Whether the archive is 10 Hz throughout (`rotation_hz` and tail RPM per drive).
- Mount vibration spectrum above 2 Hz, and strap settling at first braking.
- Handover quality at each site, and whether site start bounds that fall on file boundaries are
  truly static.
- κ_∥, δd, κ_ψ and `n_eff`. White versus random-walk noise. Per-scan σ at 20 Hz.
- The archive's return mode, and 20 Hz motor stability and packet loss on the recorder.
- The arrival latency L, and the radar host's clock offset.
