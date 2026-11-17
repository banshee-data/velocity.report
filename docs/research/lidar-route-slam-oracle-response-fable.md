# Oracle run A response: the route SLAM minimum at 10 and 20 Hz

Build the ego track first, on a two-anchor odometry bridge that needs no prior factors, and add
priors registration along the route and the rotation-rate experiment as a second phase. The
archive's drives can carry the first phase alone, provided the stop check, the anchor handover
and the stated uncertainty are each made independent of the trajectory they are meant to test.

- **Status:** Answers and a proposed work order from oracle run A (Euler persona). Nothing was
  executed, built or measured, and nothing is adopted. Every recommendation is input to the
  comparison step, not an owner decision.
- **Scope:** Answers questions 1–32 of the [oracle brief](lidar-route-slam-oracle-brief.md), with
  the gates-and-defaults table, the rate table and a two-phase work order.
- **Baseline:** velocity.report at `ab84a68`; Gerardus at `e04587c`. Read-only review of the
  files the brief names.
- **Tags:** each claim is marked (fact, with its source), (derivation, shown) or (judgement).
  "Assumption" marks a number that a measurement should replace.

## Evidence

### What was read

- velocity.report: the [minimum plan](../plans/lidar-route-slam-minimum-plan.md), the
  [20 Hz audit](lidar-20hz-readiness.md), the
  [route capture plan](../plans/lidar-route-capture-plan.md) (trajectory contract), the
  [timing design](../lidar/architecture/portable-capture-timing.md), the
  [evaluation spec](../../data/experiments/try/route-drive-bridge-first-pair.md),
  `internal/lidar/motionexport/{clock,export,las}.go` and its tests,
  `internal/lidar/egotrack/{kinematics,analysis,trajectory}.go`,
  `internal/lidar/l2frames/frame_builder.go` (`shouldStartNewFrame`),
  `internal/lidar/l1packets/parse/extract.go` (`blockToPoints`),
  `cmd/tools/lidar-motion-export/main.go`, and `tools/s2-archive/site-index.json`.
- Gerardus: `docs/plans/MOBILE-SLAM-REPLAY.md`,
  `docs/research/2026-10-24-registration-and-slam-maths.md` (all sections),
  `docs/REGISTRATION-STATUS.md` §1, `src/register.rs` (`accumulate`, `icp`, `quality`,
  `information_rank`, `register_against`, `evaluate`, `prepare_scan`) and `src/graph.rs` (all).
- Not read: `internal/lidar/l5tracks/smoother.go` (E17 is taken from the brief), the Hesai manual
  (E6 is taken from the audit), and the KISS-ICP paper (numbers attributed to it come from the
  Gerardus note and are marked as such).

### Confirmed as stated

E4 (`pointTime := packetTime.Add(firetimeOffset)`; the block index only sets `BlockID`), E6, E7,
E9, E11, E13 and E16 match the files. E10 is reproduced analytically within 6% by the
continuum formulae in answer 15. The first pair's gap is 16.28 min in the site index.

### Where the files differ from the brief's summary

| #   | Item    | Finding                                                                                                                                                                                                                                                                                                                                                                                                         |
| --- | ------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| D1  | E1      | The σ on accepted records is the formal `sigma2 · H⁻¹` (`sensor_pose`, whose comment says it understates the error). The Gerardus note §1.3 and §3.3 puts the understatement at 7–10 times. Five of the 18 manual records do carry σ. The table was read from published scenes, not the record log. Each pose describes the settled background of the first 300 s of its static capture.                        |
| D2  | E2      | The site index gives 21 same-day consecutive pairs: 20 of 5.0–34.4 min and one of 81.8 min. Eight have exactly one registered end, seven if the 82 min pair is left out. Several gaps are 5.0 or 10.0 min to within a second, so site bounds follow the 5 min file rotation and are not drive times.                                                                                                            |
| D3  | E2      | A second bridge with two registered ends exists if a static site is treated as a long stop: columbus-north-point → embarcadero-bay → columbus-broadway (23.3 min, 15.9 min static, 29.4 min), with a manual site as a mid-route check.                                                                                                                                                                          |
| D4  | E3      | The rotation rate is in every packet: `blockToPoints` reads `tail.MotorSpeed` (rpm). The exporter can report it directly and check its fitted `rotation_hz` against it.                                                                                                                                                                                                                                         |
| D5  | E5      | The synthetic jitter is uniform ±300 µs (σ = 173 µs), not 300 µs RMS. "Under 20 µs" is measured after removing each rotation's own mean (`centredRMS`). The reported residual is taken over all blocks, rejected ones included, and has a 160 µs floor from the block sawtooth alone.                                                                                                                           |
| D6  | E5      | The constant offset's sign and size belong to the test's arrival model (arrival centred on the first block: clock early by 250 µs). A real packet cannot arrive before its last block fires, so the real offset is clock late by 250 µs plus the mean emission-to-capture latency, which is unmeasured.                                                                                                         |
| D7  | E5, E8  | The fixture never completes a rotation (E8), yet E5 quotes a fit on it. `PointTimes` unwraps across rotations, so those numbers (252 µs RMS, period 0.050003 s) describe one line through about 3 s of rotations, not a per-rotation fit. I did not run it; this is inferred from the code.                                                                                                                     |
| D8  | E5      | A rotation whose fit is refused is still exported, with packet times. One segment can therefore mix two time conventions, and only `frames.csv` says which rotation has which.                                                                                                                                                                                                                                  |
| D9  | E8      | The exporter sets `BufferTimeout` and `CleanupInterval` to the maximum duration, so nothing bounds a frame that never completes. The memory failure is the exporter's configuration meeting the 340° rule, not the rule alone.                                                                                                                                                                                  |
| D10 | E12     | The gap rule is relative (3 × the median interval), so it halves in seconds at 20 Hz. With no σ in the trajectory the estimator assumes 0.02 m, which alone sets the reported speed σ. `Analyse` integrates speed through stops, including its noise floor.                                                                                                                                                     |
| D11 | E14     | `hypothesis_separation` is the cost ratio of the two best coarse peaks, which must be at least 4 m or 20° apart (`distinct`). No lot period appears in `src/`; a 3.7 m alias can be merged before it is scored. `evaluate(refine)`, the planned entry point for prior factors, computes no separation. Facade gates are skipped when fewer than 100 facade points match, so such a scan passes on ground alone. |
| D12 | E14     | `icp` and `accumulate` take the concrete `Reference` and `Scan` types. Reuse for odometry needs a small trait over `matched`, not just a new map.                                                                                                                                                                                                                                                               |
| D13 | E15     | `solve` builds a dense numeric Jacobian and `marginal` inverts a (6N−6)-square matrix once per node. That is right for a handful of scans and unusable for one node per rotation. `Between` errors are in site axes, and there is no robust kernel.                                                                                                                                                             |
| D14 | E17     | The route plan's trajectory contract lists velocity and its uncertainty as trajectory fields. Answer 19 recommends removing them.                                                                                                                                                                                                                                                                               |
| D15 | E5, LAS | A clear global-encoding bit 0 means GPS week time in LAS 1.4, as I read the specification; it does not mean "not GPS time". Session seconds are still the right content, but only `run.json` says so. Verify against the specification's global-encoding table.                                                                                                                                                 |

## Restatement

- Turn archive drives between static sites with accepted poses into (a) a trajectory pinned to
  3DEP and DataSF by per-submap prior factors, and (b) the platform's own kinematics with error
  bars.
- There is no IMU, GNSS or loop. Independent truth is the far anchor, standstill, and the priors.
- One drive has two registered ends. It must not be both the tuning set and the test set.
- Point times are arrival times repaired by a per-rotation azimuth clock, on one host clock.
- Gerardus has a robust point-to-plane ICP and a small dense pose graph. velocity.report has an
  exporter and a local-cubic kinematics pass. Odometry, submaps and the bridge solve do not exist.
- The archive is believed to be 10 Hz. A 20 Hz capture may follow. Only temporal decimation is
  available now.
- The answer wanted is the minimum, with the error cost of each shortcut and a falsifier for each
  recommendation.
- Trajectories are the operator's location history. They stay on hydra, and only summary tables
  reach Git.
- I read the hard parts as: along-street observability mid-block; a start anchor that is up to
  39 min old when the car leaves; traffic around a stopped car; and odometry error that is
  correlated, so that an independent-noise σ understates it.
- I do not know how the sensor rode between sites (on its tripod on the vehicle, or carried to
  it). The field notes decide, and several answers depend on it.

## Answers

Illustrative drift numbers used below (assumption, to be replaced by Phase 1): translation
`q_t = 1e-4 m²/m` (1 cm per root-metre) and yaw `q_ψ = 7.5e-8 rad²/m`. The second gives 0.5% of
distance at 1 km, the KISS-ICP figure the Gerardus note §4.5 quotes. Route length `L = 2 km` is
also an assumption; the straight-line distance is 1.3 km (E2).

### A. Observability of a two-anchor bridge

#### 1. Uncertainty along the bridge

**Recommendation:** model odometry error as a random walk plus a scale term and an
anchor-heading lever. Without priors the far anchor always helps; with accepted intersection
factors it stops mattering about two intersections from B.

```text
straight path, ℓ = sL; q_t in m²/m, q_ψ in rad²/m; ε = scale bias; σ_A, σ_ψA = anchor A

(i) A only
  yaw     σ_ψ²(ℓ) = σ_ψA² + q_ψ ℓ
  along   σ_x²(ℓ) = σ_A² + q_t ℓ + (ε ℓ)²
  across  σ_y²(ℓ) = σ_A² + (ℓ σ_ψA)² + q_t ℓ + q_ψ ℓ³ / 3

(ii) A and B, no priors (anchor terms interpolate linearly and are omitted)
  along   σ_x²(s) = q_t L s(1 − s)                    a pure scale bias is removed exactly
  across  σ_y²(s) = q_t L s(1 − s) + q_ψ L³ s³ (1 − s)³ / 3
  yaw     σ_ψ²(½) = q_ψ L / 16

(iii) prior factors every 50 m; full rank only at intersections ℓ_x apart, along σ_px
  across  σ_y ≈ σ_p⊥        yaw σ_ψ ≈ σ_pψ           (set by the prior factors)
  along   σ_x²(mid-block) = σ_px² + q_t ℓ_x / 4
  far-anchor influence passed per intersection:
          ρ = 1 + r/2 − sqrt(r + r²/4),   r = q_t ℓ_x / σ_px²
```

- (derivation) (i) sums independent increments; across-street error integrates yaw, hence `ℓ³/3`.
  (ii) is a Brownian bridge for translation. For yaw-driven across-street error it is an
  integrated Wiener process conditioned on value and slope at both ends: the forward and backward
  informations add, and at the midpoint the sum is `diag(192 / (q_ψ L³), 16 / (q_ψ L))`.
  (iii) is the tridiagonal information chain of a random walk with a fix at every node; `ρ` is
  the smaller root of `ρ² − (2 + r) ρ + 1 = 0`.
- (derivation) With the illustrative values and `L = 2 km`: A only gives, at B, yaw 0.70°, across
  14 m, along 0.45 m. A and B give, mid-route, yaw 0.18°, across 1.8 m, along 0.22 m. The far
  anchor multiplies translation variance by `(1 − s)` and cuts mid-route yaw and across-street
  variance by eight.
- (derivation) Anchor heading has a lever: `σ_ψA × chord`. At 0.004° (formal) and 1.3 km that is
  0.09 m; at ten times that (D1) it is 0.9 m. No held-out test can resolve below this.
- (derivation) Without priors the far anchor keeps mid-route across-street error under a
  tolerance `τ` only while `L < (192 τ² / q_ψ)^(1/3)`: 470 m for 0.2 m, 1.4 km for 1 m.
- (derivation) With intersection factors at `ℓ_x = 125 m` and `σ_px = 0.05 m`, `r = 5` and
  `ρ = 0.15`. Two intersections pass 2% of B's correction. So for `L` above about four blocks
  (500 m) the far anchor adds nothing mid-route and serves as a check and as the fix for the last
  block. Mid-block along-street σ is then 0.08–0.12 m under the random-walk model, or 0.3 m
  under the note's 1% linear model (§4.4).
- (judgement) On a grid route, turns exchange along and across error; quote the major axis.

Defer: a drift model with correlated yaw bias. Falsifier: held-out residuals at checkpoints that
do not grow as `ℓ^0.5` to `ℓ^1.5` but linearly, which means systematic bias dominates and the
random-walk covariance in answer 10 is the wrong shape.

#### 2. The far anchor in place of loop closure

**Recommendation:** sound for a point-to-point drive, and safer than a loop closure in one way:
B's pose does not depend on the odometry and has no one-lot closure alias. Run the held-out solve
in both directions.

- (fact) The A-only residual at B sums four things: drift, prior registration (absent in
  Phase 1), the error of both anchors, and both handovers.
- (fact, D1) Each accepted pose describes the first 300 s of its static capture. Lombard-laguna's
  static capture runs 10:58:30–11:37:18, so pose A is about 34 min old at departure. Pose B is
  measured as the car arrives. A's handover is the weak one.
- (derivation) Separate the terms as follows. Handover: the direct test in answer 4. Anchor and
  systematic terms: a heading error at A rotates the trajectory about A (residual perpendicular
  to the chord, growing with chord length); a scale bias stretches it along the chord; so fit a
  2D similarity about A to the residuals at all checkpoints, and call the remainder drift.
- (derivation) With B as the only checkpoint, one ratio still discriminates:
  `k = e_⊥ / (chord × δψ_B)`. Heading error at A gives `k ≈ 1`; a yaw random walk gives
  `k ≈ ½` on a straight path (`E[y_L | ψ_L] = L ψ_L / 2`); a yaw error in B's own pose gives
  `k ≈ 0` with `δψ_B ≠ 0`. On a route with turns, compute the expected `k` from the path.
- (judgement) Also run the odometry time-reversed. Agreement between forward and reverse
  open-loop residuals points to a common cause (anchor, handover, scale); disagreement of the
  size of the residual points to random drift.

Defer: loop closure and Scan Context (S2) entirely. Falsifier: forward and reverse residuals
that each pass but differ from one another by more than their predicted σ.

#### 3. σ for manually placed ends

**Recommendation:** do not use a manual end for any quantitative held-out claim. In the product
solve give it an anisotropic σ with a wide along-street axis; better, leave it out and let the
drive test the placement.

- (fact, `docs/REGISTRATION-STATUS.md` §1) The records do not say which gate each manual
  placement missed. On 2026-10-24 every non-candidate failed hypothesis separation, nine on that
  gate alone.
- (judgement) The risk is therefore not Gaussian: it is a lot-period alias along the street. No
  single σ represents that. Default for the solve: across-street 0.10 m, along-street 2.5 m,
  height 0.05 m, heading 0.5°, tilt 0.2°. The across and heading values reuse the seed-agreement
  tolerance (`SEED_AGREE_M`, `SEED_AGREE_DEG`); 2.5 m puts a 3.7 m alias at 1.5σ.
- (derivation) The export check cannot give a σ. It compares the score at the pose with the best
  score 2 m or 5° away, on 8,000 samples. Treating the scores as binomial proportions, the
  difference at 0.011 against 0.010 is `0.001 / sqrt(2 × 0.011 × 0.989 / 8000) = 0.6` standard
  errors. By that measure four of the five "narrow" sites are indistinguishable from a 2 m shift
  (z ≤ 1.3); embarcadero-bay is not narrow (z ≈ 5); and columbus-north-point, end B of the first
  drive, sits at z ≈ 2. Samples are spatially correlated, so these z values are upper bounds.
- (derivation) The check says nothing about errors under 2 m and does not probe 3.7 m. It can
  flag an end as unusable; it cannot quantify one.
- (judgement) The one-registered-end drives are best used the other way round: predict the manual
  end from the registered end and read the difference. A residual near 3.7 m or 7.6 m along the
  street is evidence about the placement, and is worth more than a weak anchor.

Defer: per-record σ until `ListRegistrationRecords` supplies the gate outcomes. Falsifier: a
manual end whose predicted-minus-placed residual exceeds 3σ of the anisotropic model.

#### 4. Is the static pose valid for the first moving rotation?

**Recommendation:** not by assumption. Test each end with a margin scan before using its anchor,
and export a thinned view of the whole static capture at A.

- (fact, D1) The pose is up to 39 min old at A. The route plan records a site lost to three bumps
  of a tripod. People entering a car also settle its suspension: about 1° of roll at a 2.3 m
  mount height moves the sensor 4 cm (derivation: `2.3 m × 0.0175`).
- (judgement) Minimal test, in three parts. T1, from the export alone: a pose-free stillness
  statistic (answer 18) shows the margin is static and finds the instant motion starts. T2, with
  the ground grid only: the margin scan's ground normal and height (`prepare_scan` already
  computes both) must match the tilt and height the accepted pose implies; this checks three of
  six directions. T3, with the bundle: `evaluate(refine)` on a 10 s median of the margin, started
  at the accepted pose; compare in the well-constrained eigen-directions only.
- (judgement) Pass at 0.10 m and 0.5° (the existing seed-agreement tolerance). On failure the
  solve replaces that end's `static_pose` factor with the margin scan's own prior factor,
  rank-projected, or with nothing if that fails its gates; the drive then makes no held-out claim.
- (judgement) Export A's full static capture with `-keep-every-rotation 50`: one rotation every
  5 s costs about 1.3 GB and lets Gerardus track the sensor's pose from the registered 300 s to
  departure.
- (fact, `export.go`) Sensor axes put azimuth zero on +y, increasing towards +x. A compass
  heading `H` of azimuth zero is therefore yaw `−H` in an east-north frame (derivation: body +x
  is `(cos H, −sin H)`). Write anchors as the record's quaternion, never a re-typed heading.

Defer: nothing; this is cheap. Falsifier: T3 passes while the A-only and B-only residuals differ
by a rigid offset the size of the handover tolerance.

### B. Timing and de-skew at car speed

#### 5. Constant-velocity de-skew at 0–15 m/s

**Recommendation:** adequate when cruising at either rate; at 100 ms sweeps it is not adequate in
turn-in, brake dive and body bounce beyond about 15 m. The minimum fix is not a two-pose sweep
model but a second offline pass that de-skews with centred, interpolated poses.

```text
lagged constant velocity (note §4.1):  e_trans = 1.5 a T²      e_rot(R) = 1.5 α T² R
centred, linear between poses:         e_trans = a T² / 8      e_rot(R) = α T² R / 8
```

| Case (assumed values)                     | Lagged, 100 ms | Lagged, 50 ms | Centred, 100 ms | Centred, 50 ms |
| ----------------------------------------- | -------------- | ------------- | --------------- | -------------- |
| Braking, a = 3 m/s²                       | 0.045 m        | 0.011 m       | 0.004 m         | 0.001 m        |
| Turn-in, α = 0.5 rad/s², R = 30 m         | 0.225 m        | 0.056 m       | 0.019 m         | 0.005 m        |
| Brisk turn-in, α = 1.0 rad/s², R = 30 m   | 0.45 m         | 0.11 m        | 0.038 m         | 0.009 m        |
| Bounce 1.5 Hz at 0.5°, α = 0.78, R = 20 m | 0.23 m         | 0.058 m       | 0.020 m         | 0.005 m        |

- (derivation) The lagged coefficient is 1.5 when poses are stamped at mid-sweep: the previous
  step's mean velocity is centred `1.5 T` before the sweep's centre. Other stamp conventions give
  1 to 2. The centred figure is the largest gap between a parabola and its chord. Bounce gives
  `α = (2π × 1.5)² × 0.0087 rad`.
- (derivation) The facade gate tolerates this much: a residue growing linearly across the sweep
  to `e` adds `e / √3` RMS; with `σ_A = 0.09 m` (note §2.1) the p75 reaches 0.20 m when
  `1.38 × sqrt(0.09² + e²/3) = 0.20`, that is `e ≈ 0.20 m`. So the lagged model at 100 ms sits at
  or over the gate in the turn-in and bounce rows, exactly where full-rank submaps are made.
- (judgement) Processing is offline, so the future is available. Pass 1 runs lagged de-skew; pass
  2 re-de-skews each sweep from pass 1's poses and re-registers. That is a re-run, not new maths.
- (judgement) Speed alone (15 m/s) is harmless: the smear `v T` is first order and removed.

Defer: a two-pose (CT-ICP style) sweep model. Falsifier: after pass 2, facade p75 or ground p95
of submaps still rises with the submap's peak yaw rate or pitch rate.

#### 6. One line per rotation, or more?

**Recommendation:** keep one line per rotation. Tighten its gates, check continuity across
rotation boundaries, and carry the sensor's own packet time as the later, better clock.

- (derivation) With per-packet jitter `s` and `N` packets, the fit's error is `s / √N` at the
  rotation's centre and `2 s / √N` at its ends. For `s = 173 µs`: 13 and 26 µs at 10 Hz
  (`N = 180`), 18 and 36 µs at 20 Hz (`N = 90`). The fitted period errs by `s × sqrt(12 / N)`:
  0.045% at 10 Hz and 0.13% at 20 Hz.
- (derivation) The geometric effect is `(v + R ω) δt`: at 15 m/s and 0.6 rad/s at 30 m, 36 µs
  moves a point 1.2 mm. Halving the blocks at 20 Hz loses nothing that matters for the 0.20 m
  gates or a 1 cm pose noise.
- (judgement) A multi-rotation fit would buy robustness, not accuracy. D7 suggests one line holds
  over 3 s on the real sensor, so it is viable later; it is not needed now.
- (judgement) Mark a rotation's times untrusted when any of these holds: span under 300° (as
  now); fitted period more than 0.5% from the running median of five (4σ at 20 Hz, 11σ at 10 Hz;
  the spec's 2% allows a 1 ms end error); the fitted time at the boundary differs from the next
  rotation's by more than 100 µs; more than 10% of blocks rejected; RMS over kept blocks above
  500 µs. Report the residual over kept blocks, beside the count of rejected ones.
- (judgement) On an untrusted rotation: if both neighbours are trusted, take its line from their
  mean period and phase and mark it `clock_interpolated`. Otherwise mark it and let odometry skip
  it. Never silently fall back to packet time (D8).
- (fact, `extract.go`) Each packet's tail carries the sensor's own timestamp. Recording it per
  packet gives an independent check of the azimuth clock that the clock's own residual cannot.

Defer: the multi-rotation fit and native-time precedence in L1. Falsifier: odometry residuals
that are worse on rotations with large clock residuals after the gates above.

#### 7. Where the constant offset does not cancel

**Recommendation:** treat it as negligible inside LiDAR-only processing and as swamped by other
lags against any second sensor. Estimate the lag in the radar match; do not assume it.

- (derivation) Inside (a) and (b) every point time and pose time is on one clock, so a constant
  cancels exactly. It does not cancel in three places: a rotation that falls back to packet time
  (a step of 250 µs plus latency, 8 mm at 33 m/s of apparent point speed); a slowly varying
  latency; and any other clock.
- (derivation) Ego speed: a time shift `τ` reads speed at the wrong instant, `δv = a τ`. At
  4 m/s² and 1.5 ms (250 µs plus an assumed 1.25 ms latency) that is 6 mm/s. Rotation-to-rotation
  fit error of 13–18 µs is a position-equivalent `v σ_t` of 0.2–0.3 mm at 15 m/s. A host clock
  rate error of 50 ppm (assumption) is 0.75 mm/s at 15 m/s.
- (derivation) Radar match: the same `δv = a τ`, but `τ` is now the radar's reporting latency and
  any offset between hosts, tens of milliseconds to seconds. At 3 m/s² each millisecond of
  unresolved lag is 3 mm/s; 50 ms is 0.15 m/s.
- (judgement) So drive past the radar at steady speed, and include one deliberate speed change
  from which the lag is estimated by cross-correlation. Report the lag and its uncertainty.

Defer: measuring the real offset (needs native time). Falsifier: a fitted radar lag that changes
between passes by more than 20 ms.

#### 8. Session seconds, and what `run.json` must carry

**Recommendation:** session seconds are correct. Add enough to re-time every point under a
different clock model without the captures.

- (judgement) Asserting GPS time would claim an epoch nothing has validated. Small magnitudes
  also keep the line fit well conditioned (the normal equations lose about five digits at 1,000 s
  and would lose eleven at Unix seconds; derivation from `lineFit`'s sums).
- (fact, D15) LAS has no "arbitrary epoch" flag, so the manifest is the only authority. The
  importer should refuse a LAS file without its `run.json`.
- (judgement) To re-time without the captures, add: per rotation, the fit's intercept and slope,
  the time source actually used, and the kept-block residual (in `frames.csv`); the angle and
  firetime calibration tables with their hash, and the median tail rpm (in `run.json`); and a
  per-packet sidecar with rotation index, UDP sequence, arrival time, first block azimuth and,
  once plumbed, the sensor's own timestamp. At 1,800 packets/s that is about 35 MB for 16 min.
- (fact, `export.go`) `frames.csv` stamps each rotation with the recorded start time, which keeps
  the arrival jitter. Define the pose reference time from the fit instead (its value at a fixed
  azimuth), and state the convention in the manifest.

Defer: the proto form of `run.json`. Falsifier: a re-timing from the sidecar that differs from a
fresh export by more than 1 µs.

### C. Minimum for (a): odometry, submaps and prior factors

#### 9. The smallest new code

**Recommendation:** a trait over `matched`, a local map rebuilt per keyframe from the existing
`Cloud`, a twist-based motion model, a physically derived first gate, and keyframe-level graph
nodes. Skip the voxel hash and the adaptive threshold for the first drive.

- (fact, D12) `icp` is concrete over `Reference`. Introduce a trait for
  `matched(p, gate) -> Option<Match>`; `accumulate`, the Huber weights and the Gauss–Newton step
  stay as they are.
- (judgement) Local map: on each keyframe, rebuild a `Cloud` (k-d tree and 12-neighbour normals,
  already written) from points inserted within 100 m. A rebuild is a fraction of a second, and
  offline that is affordable. Cost: the map lags by up to one keyframe. Falsifier: odometry
  slower than ten times real time, or failures in tight turns; then build the voxel hash with
  running moments (note §4.2).
- (judgement) Insert a scan into the map only after 0.5 m or 2° of motion, and evict by distance.
  Density is then set per metre, at any rate and at any speed, and a stop does not pile traffic
  into the map.
- (derivation) Motion model: predict with the previous twist times the actual time step. KISS-ICP
  predicts with the previous pose delta, which is wrong after a dropped rotation and in the
  decimated arm.
- (derivation) First correspondence gate from the prediction error,
  `g1 = clamp(2 (a_max + α_max R_max) T², 0.3, 2.0) m` with `a_max = 4 m/s²`,
  `α_max = 1 rad/s²`, `R_max = 60 m`: 1.28 m at 10 Hz, 0.32 m at 20 Hz, 2.0 m (clamped) for the
  5 Hz arm. Then 0.3 m (`TRUNCATE_M`). It is stateless and repeatable; its cost is a looser first
  iteration when driving gently.
- (derivation) Graph nodes are keyframes every 10 m or 10°, never rotations: a 3 km drive is
  300 nodes, 1,800 unknowns, which the dense solver handles. Replace the per-node Schur
  complement with one inverse of `H` (D13). Per-rotation poses come from the odometry, corrected
  between keyframes.
- (judgement) Submaps: 50 m of travel, cut early at 30° of turn, as the note proposes. At 15 m/s
  that is 33 rotations, not the 500 of a walk, which is still millions of raw points.
- (judgement) Droppable for a first drive: the adaptive threshold, the two-level downsample,
  point-to-point matching, the per-voxel point cap and the voxel hash itself.

Defer: voxel hash, loop closure. Falsifier: as stated per element.

#### 10. Calibrating the odometry covariance

**Recommendation:** do not use ICP's information as the odometry weight. Use a diagonal
covariance in body axes that grows per metre while moving and per second while still, and scale
it from the drive's own checks.

- (fact, note §3.3) The formal information is too confident by `n / n_eff`, about 100 in
  variance. It remains useful for its shape: inflate a direction whose eigenvalue share is weak.
- (judgement) Initial values: translation `1e-4 m²/m`, height `2.5e-5 m²/m`, yaw
  `7.5e-8 rad²/m`, tilt `2e-8 rad²/m`; at rest `2.5e-5 m²/s` and `1e-8 rad²/s`.
- (derivation) Three checks need nothing but the drive. C1, standstill: the structure function
  `D(τ) = E|p(t + τ) − p(t)|²` over pose-free still intervals is `2 σ_w² + q_s τ`; its intercept
  gives the white pose noise and its slope the per-second term. C2, forward against reverse
  odometry over windows of length ℓ: the variance of their difference grows as
  `2 (1 − c) Q ℓ`, a lower bound on the per-metre term (common errors cancel). C3, the held-out
  anchor: the normalised innovation squared on (x, y, yaw) should lie in [0.115, 11.3], the 1%
  and 99% points of χ² with three degrees of freedom. One sample can reject a covariance; it
  cannot set one.
- (derivation) Phase 2 adds the proper calibration: leave each full-rank prior factor out in
  turn; with 12 such factors there are 36 degrees of freedom, so the variance scale is known to
  `sqrt(2 / 36) = 24%`.
- (derivation) The model is per metre and per second, so the same values apply at 20 Hz. The
  decimated arm tests that claim: a per-metre term fitted there should match native within
  [0.7, 1.4]. If it doubles, the error is per registration, not per metre.

Defer: full 6×6 odometry covariance with correlations. Falsifier: innovations whose normalised
size trends with speed or with yaw rate.

#### 11. Mid-block prior factors and lot-period aliases

**Recommendation:** add the mid-block factor with weak directions projected out. Keep the
along-street direction only when the geometry and the chain both support it.

- (judgement) Without mid-block factors, yaw and across-street position ride on odometry between
  intersections; with them they are pinned every 50 m. No factor would waste that.
- (fact, note §1.4) The rank threshold `RANK_SHARE = 1e-3` is passed by about a hundred points
  and can be propped up by parked cars. Project at `WEAK_SHARE = 1e-2` instead, and count the
  along-street direction only from points at least 2 m above ground.
- (derivation) Keeping an alias out. First, the chain discriminates: after 125 m the predicted
  along-street σ is `sqrt(1e-4 × 125) = 0.11 m`, far below half a 3.7 m period. Gate the
  along-street innovation at `max(3 σ_pred, 0.5 m)` with a hard cap of 1.85 m. Second, the refine
  starts from the chain pose with a first gate of 1.0 m and cannot jump a period. Third, where
  `3 σ_pred` exceeds 1.85 m the chain can no longer tell, so require an explicit test: refine
  from the pose shifted ±3.7 m and ±7.6 m along the street, and require a cost ratio of at least
  1.25 (`GATE_SEPARATION`). This is new code (D11).
- (derivation) That third case arrives after 3.8 km without a fix under the random-walk model,
  but after only 185 m under a 1% linear drift. Phase 1's residuals say which model holds.
- (fact, D13) The graph has no robust kernel. Minimum: solve, drop prior factors with a whitened
  residual above 4, solve again, and report what was dropped.

Defer: feature anchors at intersections. Falsifier: an accepted factor whose leave-one-out
residual is within 0.5 m of 3.7 m or 7.6 m along the street.

#### 12. Moving traffic

**Recommendation:** the robust kernel is not enough. For odometry use ground points for height
and tilt, and only structure at least 2.5 m above ground for horizontal motion.

- (derivation) A vehicle creeping at 0.2–1 m/s moves 2–10 cm per 100 ms sweep. That is inside the
  Huber threshold of 0.10 m (`HUBER_M`), so it keeps full weight; at 20 Hz more of it does.
- (derivation) Vehicle fronts and backs have along-street normals, and along a corridor the
  static scene offers few such surfaces (4–18% of points, note §1.5). Traffic therefore has most
  leverage in the direction the scene constrains least.
- (judgement) The height rule needs no background model: `prepare_scan` already fits the scan's
  ground plane, and `ELEVATED_M = 2.5` exists. It removes cars and people; buses and lorries
  remain for the kernel. Its cost is losing kerbs and parked cars as along-street features.
- (judgement) After convergence, do not insert points with residuals above 0.3 m into the map.

Defer: free-space carving and tracking-based removal. Falsifier: displacement over pose-free
still intervals that is larger when traffic is present than when it is not.

#### 13. Acceptance for (a) on the first drive

**Recommendation:** gate each submap at the solved pose, not the refined one; hold out the far
anchor and each full-rank factor in turn; and say plainly what one drive cannot show.

- (fact, `evaluate`) With `refine` set, the gates describe the pose ICP moved to. Score the
  trajectory with `refine = false` at the solved pose, and report both.
- (fact, D11) A submap with fewer than 100 facade points passes on ground alone. Report it as
  "ungated", not as a pass.
- (judgement) Per submap: the three existing gates, the rank after projection, the alias test.
  Per drive: at least 80% of gated submaps passing; at least one accepted full-rank factor per
  250 m; the held-out far anchor within 0.3 m and 0.3° once priors are in the graph.
- (derivation) Minimum counts. With `n` of `n` leave-one-out residuals inside tolerance, the 95%
  lower bound on the true share is `0.05^(1/n)`: 0.55 for 5, 0.78 for 12, 0.90 for 29. One drive
  cannot support "nine in ten factors are within tolerance".
- (judgement) A pass would show that one car drive, on one day, yields a trajectory consistent
  with the priors at the gates. It would not show absolute accuracy (errors common to 3DEP are
  invisible), mid-block along-street accuracy (nothing held out can see it), or anything about
  sparse-facade streets, other speeds, a backpack or a bike.

Defer: an accuracy statement against independent control. Falsifier: gates that pass at the
refined pose and fail at the solved pose for more than a fifth of submaps.

### D. Minimum for (b): the ego track, speed, jerk and jitter

#### 14. Is a local weighted cubic the right estimator?

**Recommendation:** yes for the spike. Keep it, fix its noise model, and share the uncertainty
language with the tracker rather than the smoother.

- (derivation) The local cubic is a linear filter with a closed form. In the continuum limit the
  estimates are the true signals smoothed by fixed kernels of half-width `h`:

  ```text
  acceleration:  kernel (15/16)(1 − u²)²           bias h² a″ / 14     −3 dB at 0.34 / h
  jerk:          kernel (35/32)(1 − u²)³           bias h² j″ / 18     −3 dB at 0.39 / h
  speed:         kernel (15/32)(1 − u²)(3 − 7u²)   bias −h⁴ x⁽⁵⁾ / 504  −3 dB at 0.62 / h
  white pose noise σ at spacing Δ:
    σ_v = 3.06 σ sqrt(Δ / h³)    σ_a = 4.74 σ sqrt(Δ / h⁵)    σ_j = 28.1 σ sqrt(Δ / h⁷)
  ```

  These follow from the Legendre expansion on [−1, 1] and Rodrigues' formula. With `h` replaced
  by `h + Δ/2` they match E10 within 6%. The speed kernel's negative lobes are the 8.48 m/s
  overshoot in E13.

- (judgement) An RTS smoother on a constant-acceleration or constant-jerk model is a smoothing
  spline and behaves like a kernel smoother whose width is set by a process-noise ratio. It
  handles gaps, edges and zero-velocity updates better (the cubic refuses one-sided windows:
  fact, `deriveAt`). It hides its bandwidth, and the tracker's smoother has four states, so
  nothing would actually be shared yet.
- (judgement) A continuous-time spline over the graph is the ideal and is far from minimal.
- (judgement) Share the language: every derivative carries 1σ, the noise model behind it and a
  −3 dB bandwidth. Revisit a shared family when the tracker gains acceleration states.

Defer: RTS and spline estimators. Falsifier: fit residuals correlated at lag one beyond what the
noise model predicts, or more than 2% of samples lost to one-sided windows.

#### 15. The window for jerk

**Recommendation:** report speed and acceleration at ±0.5 s and jerk at ±0.75 s, at both rates,
and report the change in acceleration across an onset beside peak jerk.

- (derivation) By answer 14, a jerk pulse of height `J` and duration `T_r` is read at its centre
  as `J × F(T_r / 2h)`, with `F(u) = (35/16)(u − u³ + 3u⁵/5 − u⁷/7)` for `u ≤ 1`.

| Onset (assumed)             | h = 0.5 s       | h = 0.75 s      | h = 1.0 s       |
| --------------------------- | --------------- | --------------- | --------------- |
| Read fraction, T_r = 0.5 s  | 0.86            | 0.65            | 0.51            |
| Read fraction, T_r = 1.0 s  | 1.00            | 0.96            | 0.86            |
| Noise σ_j, 10 Hz, 1 cm      | 0.72–0.76 m/s³  | 0.19 m/s³       | 0.075 m/s³      |
| RMSE, J = 6 m/s³ over 0.5 s | 1.1 m/s³ (18%)  | 2.1 m/s³ (35%)  | 2.9 m/s³ (49%)  |
| RMSE, J = 2 m/s³ over 1.0 s | 0.72 m/s³ (36%) | 0.21 m/s³ (10%) | 0.29 m/s³ (15%) |

- (derivation) Minimising `B² h⁴ + V h⁻⁷` gives `h_opt¹¹ = 7 V / (4 B²)`, so
  `h_opt ∝ (σ² Δ)^(1/11)`. Numerically the optimum is 0.8 to 1.0 times the ramp duration. No one
  window serves both rows: ±0.5 s is best for the firm onset, ±0.8 s for the gentle one.
- (derivation) At 20 Hz the optimum moves by `2^(−1/11)`, 6%, if pose noise is unchanged, and
  not at all if it grows by √2. The window in seconds is rate-invariant.
- (judgement) ±0.75 s is the default because real noise is worse than white 1 cm (answer 16),
  which favours the wider side. Its 3σ floor is 0.58 m/s³. State with every jerk value that
  onsets shorter than 0.7 s are under-read by the factor `F`. The step in acceleration across
  the onset is the better-determined quantity.
- (fact, E10) At 5 Hz and ±0.5 s the cubic has five samples for four coefficients. Compare the
  decimated arm at ±1.0 s.

Defer: adaptive windows. Falsifier: a measured white floor above 2 cm at stops, which moves the
default to ±1.0 s.

#### 16. Correlated odometry error and the stated σ

**Recommendation:** replace the independent-noise σ with a two-parameter model, white `σ_w` plus
a random walk of rate `q` in m²/s, propagated exactly through the fit. Pose-graph marginals are
the wrong object.

- (derivation) A random walk in position is white noise in velocity, so each estimate's variance
  is `q` times the integral of the squared kernel (or of its derivatives):

  ```text
  random walk, rate q:   σ_v = 1.12 sqrt(q / h)   σ_a = 1.46 sqrt(q / h³)   σ_j = 5.92 sqrt(q / h⁵)
  ```

  These carry no `Δ`: more poses per second do not reduce them.

- (derivation) With 1 cm of independent error per 0.1 s step, `q = 1e-3 m²/s`. At ±0.5 s the
  random-walk terms are 0.050 m/s, 0.13 m/s² and 1.06 m/s³, against white-noise values of
  0.025, 0.068 and 0.76. The stated σ would then be low by factors of 2.3, 2.2 and 1.7.
- (judgement) I do not know `q`. Scan-to-map registration anti-correlates successive errors, so
  it may be much smaller. C1 in answer 10 measures it at rest (a lower bound); the growth of the
  fit residual with window length measures it in motion.
- (derivation) Slow drift is a third term and is a bias, not noise: a scale error `ε` reads
  speed as `(1 + ε) v`. The far anchor bounds it: 0.5 m over a 1.3 km chord is `4e-4`.
- (judgement) Minimum correction: in `fitLocal`, use
  `Cov(c) = (AᵀWA)⁻¹ AᵀW C W A (AᵀWA)⁻¹` with `C_ij = σ_w² δ_ij + q × min(t_i, t_j)`. It is
  exact for the stated model and about thirty lines. Marginals describe each pose's absolute
  error, which is common to the whole window and cancels in a derivative; what matters is the
  relative covariance within the window, which is this model.

Defer: time-varying `q` from per-rotation quality. Falsifier: at pose-free stops, the empirical
scatter of estimated velocity outside [0.7, 1.4] of the stated σ.

#### 17. What may be called jitter

**Recommendation:** call the 10 Hz result "ride motion, 1.3–5 Hz" and the 20 Hz result
"1.3–10 Hz". Report a peak only when it is significant, corrected only where the gain is at
least 0.5, and tested for aliasing.

- (derivation) The residual sees the motion through `1 − H`, with
  `H = j0(Ω) + 2.5 j2(Ω)`, `Ω = 2π f h`. At ±0.5 s the gain is 0.1 at 0.77 Hz and 0.5 at
  1.27 Hz. Dividing by a gain of 0.1 (the code's floor) multiplies noise power by 100.
- (derivation) A per-rotation pose averages over the sweep, which attenuates by `sinc(f T)`: at
  10 Hz, 0.64 at 5 Hz and 0.37 at 7 Hz. That is a weak anti-alias filter, not a safe one.
- (derivation, sketch) The rotor also mixes. A point's time is locked to its azimuth, so heave at
  the rotation frequency looks like a fixed warp of the ground, and vibration at `f` near a
  multiple of the rotation frequency `f_r` appears at the beat `|f − k f_r|`. Wheel hop at 12 Hz
  would read as 2 Hz at 10 Hz rotation and as 8 Hz at 20 Hz. A line that moves when the rate
  changes is this artefact.
- (judgement) Gain correction is sound for a narrow line that stands at least 1.5 times above
  the median floor, where the gain is at least 0.5, and that is not an alias. It is not sound
  for the highest bin of a noise spectrum, which the code reports today (fact, `peak`).
- (judgement) Guards at 10 Hz: list each peak with its alias family (`10 − f`, `10 + f`); keep
  only lines that persist at the same frequency in both halves of the drive; compare moving
  against stopped spectra; and use the decimated arm, where only lines below 2.5 Hz stay put.
- (judgement) Per-sector 6-DoF poses are not worth it: a 90° sector is rank-deficient. A
  per-sector ground fit is: at each of eight sectors, fit height against range over the
  ground rings, giving heave and look-direction tilt at 80 Hz (160 Hz at 20 Hz rotation), free of
  any trajectory. It is also the measurement the backpack and bike regimes are waiting for.

Defer: the per-sector fit to Phase 2. Falsifier: a "jitter" spectrum that is unchanged between
moving and stopped stretches, which makes it estimator noise.

#### 18. Stops as a check and as factors

**Recommendation:** valid only if stops are found without the trajectory. Apply zero-velocity
holds in the product trajectory, and run the check on the odometry before the graph.

- (fact, `Stops`) A stop is where the trajectory's own speed is under 0.3 m/s. If traffic drags
  the odometry at 0.35 m/s, that interval is never examined. The check selects its own passes.
- (judgement) Pose-free stillness, per rotation: among cells (ring, 0.4° azimuth bin) with range
  over 5 m on rings at or above the horizon, the share whose range is within 0.03 m of its value
  1.0 s earlier. Still when the share is at least 0.8. The window is in seconds and the bin is
  the 20 Hz step, so it runs unchanged at both rates. It belongs in the exporter.
- (judgement) With traffic present the displacement is drift plus traffic bias. That is a useful
  test of answer 12; label it as such, and split still intervals by traffic.
- (judgement) Yes to zero-velocity factors: no keyframe is created at rest, so a stop collapses
  to one node and the per-rotation poses are held. That spoils the check on the solved
  trajectory only. State drift as a rate, at most 0.01 m/s median, not as 0.3 m per stop.

Defer: nothing. Falsifier: any pose-free still interval in which the odometry-only speed exceeds
0.3 m/s.

#### 19. What belongs in the trajectory

**Recommendation:** Gerardus writes poses, quality and a noise model; velocity.report alone
differentiates. Derivatives are taken on odometry poses, placement from solved poses.

- (fact, D14) The route plan's contract puts velocity in the trajectory. If Gerardus writes its
  motion-model velocity and velocity.report also differentiates the poses, two velocities with
  different lags and bandwidths will exist.
- (derivation) Graph corrections interpolated linearly between keyframes bend the path at each
  keyframe. If a correction absorbs 3 cm per 10 m, the slope changes by about 0.5%, a 0.05 m/s
  step at 10 m/s. Through the kernels of answer 14 that is 0.09 m/s² and 0.38 m/s³ at every
  keyframe, once a second: an artefact at the jerk floor.
- (judgement) So Gerardus writes two pose sets per rotation, odometry (locally smooth) and solved
  (placed), with residual, inlier share, regime, clock flag, `σ_w` and `q`, and the keyframe
  covariances. velocity.report takes acceleration, jerk, yaw rate and jitter from the odometry
  set, position and course from the solved set, and multiplies speed by the reported ratio of
  solved to odometry length per inter-fix segment.
- (judgement) For Phase 1 write the two sets as two v0 files; no schema change is needed.
  Gerardus's de-skew twist may be written under that name, as a diagnostic.

Defer: the v1 schema and proto. Falsifier: speed from the solved set and from the odometry set
differing by more than the stated σ away from keyframes.

### E. Evaluation and its traps

#### 20. Ranking the independent references for (b)

**Recommendation:** far anchor first, pose-free standstill second, the fixed radar third,
centreline length last.

| Rank | Reference             | What it tests                                            | What it misses                                    |
| ---- | --------------------- | -------------------------------------------------------- | ------------------------------------------------- |
| 1    | Held-out anchor       | Integrated scale and heading; mean speed to about `4e-4` | Anything that integrates to zero: noise, dynamics |
| 2    | Pose-free standstill  | Speed bias and noise at zero; traffic robustness; `σ_w`  | Scale, de-skew, everything at speed               |
| 3    | Fixed radar (later)   | Instantaneous speed at speed, at one place               | Acceleration and jerk; needs a lag estimate       |
| 4    | DataSF centreline A–B | Gross path errors, a wrong time base                     | 1–3% from lane offset, corner radii and parking   |

- (derivation) Rank 1: a 0.5 m residual on a 1.3 km chord bounds a scale bias at `4e-4`.
- (derivation) Rank 4: a 90° turn on an 8 m arc is `0.43 × 8 = 3.4 m` shorter than the
  centreline corner; six turns and two parking manoeuvres are about 1–2% of 2 km.
- (judgement) Phase 2 adds a better one: the odometry distance between two accepted intersection
  factors against their separation in the priors frame, a scale check per block.

Defer: GNSS. Falsifier: rank 1 and the per-block scale check disagreeing by more than `1e-3`.

#### 21. How each success criterion can pass while the trajectory is wrong

| Criterion in the spec                               | How it passes wrongly                                                                                                             | Extra check                                                                                           |
| --------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------- |
| 99% `clock_ok`, no unexplained period outlier       | `clock_ok` needs only a 300° span and a 4–25 Hz period; the residual has a 160 µs floor and cannot see a constant or slow latency | Period within 0.5%, boundary step under 100 µs, and tail rpm against fitted rate                      |
| Held-out B within 2% of drive distance              | 2% of 2 km is 40 m: a lot-period alias passes, and stop noise lengthens the denominator                                           | 0.5% of moving path, the innovation test of answer 10, the ratio `k` of answer 2, and the reverse run |
| Stops of 5 s move under 0.3 m on the A+B solve      | Stops are defined by the trajectory; with holds applied the test is void                                                          | Pose-free stops, on the odometry-only poses, as a rate; count still intervals the trajectory missed   |
| Distance within 1% of path length; median σ_v < 0.1 | Both lengths come from the same poses; σ is set by the assumed 0.02 m (it is 0.049 m/s by construction)                           | Lengths over moving samples on 1 m chords; empirical against stated σ at stops; reduced χ² of the fit |
| Jitter reported with Nyquist and gain               | It is a reporting criterion; a noise spectrum always has a highest bin                                                            | Significance, split-half persistence, the alias tests of answer 17                                    |
| 5 Hz arm reported beside native                     | The 5 Hz cubic at ±0.5 s is degenerate; any difference may be the estimator's                                                     | A third arm: native poses with every other one dropped, no SLAM re-run; and ±1.0 s windows            |

- (derivation) Path length from raw poses grows at rest: a step between two noisy poses has mean
  length `1.25 × √2 × σ` in the plane, 1.8 cm at 1 cm, which is 0.18 m/s at 10 Hz and 0.35 m/s
  at 20 Hz. Five minutes at rest add 53 m at 10 Hz, more than 1% of the drive, and twice that at
  20 Hz. The integrated speed has its own, smaller floor. The 1% criterion compares two biases.

#### 22. Is one drive enough to choose defaults?

**Recommendation:** no. Fix defaults from the derivations and from development drives, then run
the both-registered drive once as the test.

- (judgement) Choosing a window, a threshold and a submap length on the drive they are judged on
  leaves one held-out sample of three numbers and no test.
- (judgement) Smallest set, five drives. Test: lombard-laguna → columbus-north-point. Second
  test: columbus-north-point → embarcadero-bay → columbus-broadway (D3). Development:
  marina-webster-beach → lombard-laguna (10 min, registered at the end), columbus-broadway →
  van-ness-sacramento (registered at the start; the manual end has a σ and was a candidate on
  2026-10-24), and ashbury-downey → haight-divisadero (another day and district).
- (judgement) The remaining one-ended drives replicate every check that needs no far anchor
  (clock, standstill, leave-one-out factors) and test their manual placements (answer 3).

Defer: the 82 min pair. Falsifier: a default that must change between development and test.

### F. Rotation rate: SLAM at 10 Hz and 20 Hz

#### 23. What 20 Hz changes for scan-to-map odometry

**Recommendation:** for a car, expect little. With centred de-skew nothing binds at 10 Hz. On
foot or on a bike, rotational residue dominates and 20 Hz cuts it by four.

| Quantity                                  | 10 Hz           | 20 Hz            | Scaling    |
| ----------------------------------------- | --------------- | ---------------- | ---------- |
| Inter-scan motion at 15 m/s               | 1.5 m           | 0.75 m           | `v T`      |
| Prediction error, a = 3, α = 1 at 30 m    | 0.03 m, 0.30 m  | 0.008 m, 0.075 m | `T²`       |
| Smear before de-skew at 15 m/s            | 1.5 m           | 0.75 m           | `v T`      |
| Lagged residue, α = 0.5 rad/s² at 30 m    | 0.225 m         | 0.056 m          | `T²`       |
| Points per scan; spacing at 50 m          | 72,000; 0.175 m | 36,000; 0.349 m  | `T`        |
| Per-scan registration noise, if count-led | σ               | about √2 σ       | `T^(−1/2)` |
| Compute per second of capture             | reference       | about equal      | points/s   |

- (derivation) The ICP basin is about 1 m and 4° at the first gate (note §1.7). What must fit
  inside it is the prediction error, not the motion, and it fits at both rates. The 5 Hz arm is
  at the edge (1.2 m at 30 m).
- (derivation) Compute follows points per second, which is fixed; fewer iterations from a better
  start roughly cancel the doubled per-scan overhead.
- (judgement) Car: the lagged residue is the largest term, and pass 2 removes it at either rate.
  What remains for 20 Hz is the 5–10 Hz band and margin in transients, against sparser thin
  structure per scan.
- (derivation) Walking: with the note's sway (`α = 1.4 rad/s²`), lagged residue at 20 m is
  0.42 m at 10 Hz and 0.105 m at 20 Hz.

Defer: nothing. Falsifier: the 5 Hz arm's held-out residual or failure rate more than twice
native, which would mean inter-scan motion already binds at 10 Hz.

#### 24. Single rotations or merged pairs at 20 Hz

**Recommendation:** register each 50 ms rotation on its own. The answer is the same for prior
factors, which come from submaps.

- (derivation) Merging two rotations needs their relative motion, which is the unknown. It
  restores 100 ms of de-skew dependence and halves the pose rate, giving up the 10 Hz band.
- (judgement) Against a map, the map's density matters, and 36,000 points constrain six
  directions amply where the scene does.
- (judgement) Exception: if single 20 Hz scans report a weak along-street eigenvalue where pairs
  do not, use a two-rotation window with a two-pose model. That is the deferred CT-ICP work.

Falsifier: an along-street eigenvalue share below `1e-2` on more than 5% of single 20 Hz scans
in stretches where the 10 Hz drive had none.

#### 25. Parameters to fix in seconds or metres, and inherited rate couplings

**Recommendation:** adopt the rate table below, and fix six couplings before any 20 Hz run.

- (fact, D10) `egotrack` splits at three median intervals: 0.30 s at 10 Hz, 0.15 s at 20 Hz.
  State the gap in seconds (0.35 s).
- (fact, `kinematics.go`) `DefaultSigmaPosM` is per pose. Replace it with `σ_w` and a
  per-second `q` (answer 16).
- (derivation) Path length from raw poses doubles its noise growth at 20 Hz (answer 21).
- (derivation) A previous-delta motion model and a history-based threshold are per scan. Use a
  twist and the `T²` gate (answer 9).
- (fact, E8, D9) The 340° rule and the exporter's disabled timeouts: see answer 30.
- (fact, E7) `SpatialCoverage` and `MaxOcclusionFrames` do not touch this path. They will when
  route tracking reuses them; leave them to the audit's own sequence.
- (fact, `register.rs`) `icp` stops below 30 matches, a count. For odometry use a share of the
  scan's points (at least 0.5) so the test means the same at 36,000 and 72,000 points.

Defer: the audit's tracker constants. Falsifier: any default that must differ between the rates.

#### 26. Are the prior gates rate-invariant?

**Recommendation:** to first order, yes. The exception is de-skew residue in turns at 10 Hz,
which pass 2 should remove. The 5 Hz arm is not comparable on density.

- (derivation) A submap is cut by distance and receives 720,000 points per second at either
  rate, so its density per metre is the same at the same speed.
- (derivation) Residue inside each sweep scales with `T²` (answer 5) and blurs the submap where
  the car turns, which is where full-rank factors come from. That favours 20 Hz until pass 2.
- (derivation) Sparser columns per scan at 20 Hz are filled in by motion within the submap. A
  √2 rise in per-pose noise adds 1.4 cm in quadrature to a 9 cm facade spread: negligible.
- (fact, E11) The 5 Hz arm has half the points per second. Its submaps are half as dense, and a
  failure of the 100-point facade minimum there is an artefact of the ablation.

Falsifier: gate values that differ between rates after stratifying submaps by peak yaw rate.

#### 27. The ego track at 20 Hz

**Recommendation:** use the same windows in seconds. Same units and windows are necessary for a
fair comparison and not sufficient.

- (derivation) White-noise σ scales as `σ sqrt(Δ)`: unchanged if pose noise grows by √2, better
  by √2 if it does not. Random-walk σ has no `Δ` at all. Bias depends on `h` only.
- (judgement) Also required: `σ_w` and `q` measured at each rate, not assumed; empirical scatter
  compared, not stated σ; spectra compared on the common band below 5 Hz; and the same capture
  decimated as a control, since two drives never match in traffic or driving.

Falsifier: stated σ ratios between rates that the empirical scatter at stops does not reproduce.

#### 28. What the ablation predicts, and the minimum 20 Hz capture

**Recommendation:** the ablation bounds the benefit of shorter inter-scan motion and tests the
seconds-and-metres configuration. Everything about 50 ms sweeps needs a native capture: three
drives of one route in the order 10, 20, 10 Hz.

- (derivation) Decimation keeps each sweep at 100 ms. It doubles the pose interval, so the
  lagged residue doubles (`a × 3T × T` against `a × 1.5T × T`), and it halves points per second.
  It does not shorten a sweep or thin its columns.
- (judgement) It predicts: sensitivity to prediction error; whether per-metre covariance holds;
  whether any default hides a rotation count. If the 5 Hz arm matches native, inter-scan motion
  does not bind and 20 Hz will not help a car's convergence.
- (judgement) Only a native capture answers: registration noise at 0.4° columns; residue at
  50 ms; the 5–10 Hz band and the rotor beat of answer 17; the frame builder and clock on real
  full rotations at 20 Hz; and any motor or mount behaviour.
- (judgement) Minimum capture: the first pair's route, three times on one day with one mount, at
  600, 1,200 and 600 rpm. Single return, full 360° field of view, confirmed from the tail rpm and
  azimuth coverage; sensor settings exported into the provenance. At least 60 s static at each
  registered end, three stops of 10 s, one firm brake and two ordinary turns per drive.
- (judgement) Yes, the same route at both rates, and twice at 10 Hz: the two 10 Hz drives
  measure drive-to-drive spread, without which no rate difference can be read.
- (judgement) Arms: N20 (native), D10 (the 20 Hz capture keeping one rotation in two) and N10.
  N20 against D10 isolates pose rate. D10 against N10 isolates sweep length and column density,
  and is read against the N10 spread.

Falsifier: a rate difference smaller than the spread between the two 10 Hz drives.

#### 29. Rate by platform

**Recommendation:** car at 10 Hz; bike and backpack survey walks at 20 Hz; any traffic
measurement stays at 10 Hz until the audit's qualification passes.

| Platform | For SLAM                           | For traffic measurement                         |
| -------- | ---------------------------------- | ----------------------------------------------- |
| Car      | 10 Hz (judgement, after answer 23) | 10 Hz; revisit after the tracker is rate-clean  |
| Bike     | 20 Hz (judgement)                  | 10 Hz until qualified; needs an IMU in any case |
| Backpack | 20 Hz while walking (derivation)   | 10 Hz at stops; choose the rate per session     |

- (derivation) Residue scales as `α T² R`, and a bike or a walker has several times a car's
  angular acceleration (answer 23).
- (fact, audit) Per-object points halve at 20 Hz, and the tracking path still hard-codes 10 Hz
  in places.
- (derivation) The recorder sees no change: 18,000 blocks/s and 1,800 packets/s at both rates,
  and the same bit rate (fact, audit). Imbalance force grows as rate squared, so four times at
  1,200 rpm, and rotor angular momentum doubles. I do not know the sensor's power draw at
  1,200 rpm; measure it and the mount's response before a long session.

Falsifier: the 20 Hz walk showing no lower residue than its own D10 arm.

#### 30. The sector-limited 20 Hz fixture

**Recommendation:** document the 340° rule as the frame builder's contract, and fix the exporter:
bound frame growth, detect a sector capture and refuse it by name.

- (judgement) The fixed pipeline's background grid assumes full rotations, and the builder is
  shared with the live path and its performance baselines. Changing completion here is out of
  proportion to a spike.
- (fact, D9) The defect is that an offline run can grow one frame without limit. Cap a frame at
  0.5 s of point time or four nominal rotations of points, and emit it as incomplete with a
  reason.
- (judgement) The exporter should measure azimuth coverage over the first packets. If it stays
  below 340° with regular wraps, stop with "sector capture, field of view X–Y°", unless a sector
  mode is asked for. Add that mode only if a real drive turns out to be sector-limited.
- (fact, D7) Until a full-rotation 20 Hz excerpt exists, no real-data statement about the
  per-rotation clock at 20 Hz is available.

Falsifier: the first export of a drive reporting coverage under 340°.

### G. Sequencing

#### 31. Order of work

**Recommendation:** two phases, split between the ego track and priors registration. The full
list is under [Two-phase work order](#two-phase-work-order).

- (derivation) The boundary follows from sensitivity. A position error field `e(ℓ)` changes
  speed by `v × de/dℓ`. A bridge without priors leaves about 2 m mid-route, varying over about
  1 km: a slope of `2e-3`, or 0.03 m/s at 15 m/s, at the level of the speed σ. Acceleration,
  jerk and jitter do not see it at all. So (b) does not need (a), while (a) needs everything
  (b) needs.
- (judgement) Phase 1 also measures the drift model, on which the prior-factor weights and the
  alias guard of answer 11 depend, and it meets the risks that would sink both parts first:
  the handover, de-skew and traffic.

#### 32. Decisions for the owner

**Recommendation:** nine decisions, listed with defaults under [Open questions](#open-questions).

## Proposed gates and defaults

All values are proposals. "Rate" says how the value depends on rotation rate.

| Name                             | Value                                             | Units          | Domain                                       | Rate                         | From     |
| -------------------------------- | ------------------------------------------------- | -------------- | -------------------------------------------- | ---------------------------- | -------- |
| Clock period deviation           | ≤ 0.5 of running median of 5                      | %              | Full rotations, 10 or 20 Hz                  | None (4σ at 20, 11σ at 10)   | A6       |
| Clock boundary step              | ≤ 100                                             | µs             | Consecutive trusted rotations                | None                         | A6       |
| Clock kept-block residual        | ≤ 500 RMS (floor 160)                             | µs             | After rejection                              | None                         | A6, D5   |
| Clock rejected share             | ≤ 10                                              | % of blocks    | Per rotation                                 | None                         | A6       |
| Handover tolerance               | 0.10 and 0.5                                      | m, degrees     | Well-constrained directions at each end      | None                         | A4       |
| Registered anchor σ in the solve | 0.05, 0.02, 0.05, 0.02                            | m, m, °, °     | Horizontal, height, yaw, tilt; floors        | None                         | A1, D1   |
| Manual anchor σ in the solve     | 0.10, 2.5, 0.05, 0.5, 0.2                         | m, m, m, °, °  | Across, along, height, yaw, tilt             | None                         | A3       |
| Odometry covariance, moving      | 1e-4, 2.5e-5, 7.5e-8, 2e-8                        | per metre      | Translation, height (m²); yaw, tilt (rad²)   | None; tested by the 5 Hz arm | A10      |
| Odometry covariance, at rest     | 2.5e-5 and 1e-8                                   | per second     | m² and rad²                                  | None                         | A10      |
| Keyframe spacing                 | 10 or 10                                          | m or degrees   | Car; a walk may need 5 m                     | None                         | A9       |
| Map insertion                    | 0.5 or 2                                          | m or degrees   | Any speed                                    | None                         | A9       |
| Local map radius                 | 100                                               | m              | Urban; range cut 60 m                        | None                         | A9       |
| First correspondence gate        | clamp(2 (4 + 60) T², 0.3, 2.0)                    | m              | a ≤ 4 m/s², α ≤ 1 rad/s², R ≤ 60 m           | `T²`                         | A9       |
| Final correspondence gate        | 0.3                                               | m              | As `TRUNCATE_M`                              | None                         | A9       |
| Odometry point set               | Ground band, plus structure ≥ 2.5                 | m above ground | Streets with facades or poles                | None                         | A12      |
| Odometry validity                | Inlier share ≥ 0.5                                | share          | Drive regime                                 | None                         | A25      |
| De-skew                          | Twist × point time; pass 2 centred                | —              | Offline                                      | Residue `T²`                 | A5       |
| Submap length                    | 50, cut at 30                                     | m, degrees     | Car                                          | None                         | A9       |
| Factor projection                | Eigenvalue share ≥ 1e-2                           | share          | Along-street from points ≥ 2 m up            | None                         | A11      |
| Alias guard                      | Innovation ≤ max(3σ, 0.5), cap 1.85; ratio ≥ 1.25 | m; ratio       | Shifts of ±3.7 and ±7.6 m                    | None                         | A11      |
| Submap gates                     | 0.20, 0.20, tan 0.5°                              | m, m, slope    | At the solved pose; ≥ 100 facade points      | None to first order          | A13, A26 |
| Held-out end, no priors          | ≤ 0.5 of moving path; yaw ≤ 1                     | %, degrees     | Both directions; innovation in [0.115, 11.3] | None                         | A2, A10  |
| Held-out end, with priors        | ≤ 0.3 and ≤ 0.3                                   | m, degrees     | Registered ends only                         | None                         | A13      |
| Leave-one-out factors            | Median ≤ 0.15; 90% ≤ 0.30; n ≥ 20                 | m              | Full-rank factors, pooled over drives        | None                         | A13      |
| Stillness                        | Share ≥ 0.8 within 0.03 at 1.0 lag                | share, m, s    | Rings ≥ 0°, range > 5 m, 0.4° bins           | None                         | A18      |
| Standstill drift                 | Median ≤ 0.01; worst ≤ 0.03                       | m/s            | Odometry-only poses, pose-free intervals     | None                         | A18      |
| Speed and acceleration window    | ±0.5                                              | s              | Motion below about 1.2 and 0.7 Hz            | None                         | A14      |
| Jerk window                      | ±0.75                                             | s              | Onsets ≥ 0.7 s; shorter under-read by `F`    | None                         | A15      |
| Ego noise model                  | σ_w = 0.01; q = 1e-3 (initial)                    | m; m²/s        | Until measured at stops                      | σ_w may grow √2 at 20 Hz     | A16      |
| Trajectory gap                   | 0.35                                              | s              | Any rate                                     | None                         | A25      |
| Stated σ consistency             | Empirical / stated in [0.7, 1.4]                  | ratio          | Velocity at pose-free stops                  | None                         | A16      |
| Spectrum                         | 6.4 segment; gain ≥ 0.5; peak ≥ 1.5 floor         | s; ratio       | 1.3 Hz to Nyquist at ±0.5 s                  | Upper edge 5 or 10 Hz        | A17      |

Outside its domain each gate should report "not applicable" with the reason, never a pass.

## Rate table

| Parameter                               | Fixed in      | At 10 Hz                        | At 20 Hz                   |
| --------------------------------------- | ------------- | ------------------------------- | -------------------------- |
| Sweep duration `T`                      | Measured, s   | 0.100                           | 0.050                      |
| Motion model                            | m/s, rad/s    | Twist × step                    | Same                       |
| De-skew                                 | Point time, s | Same                            | Same                       |
| First correspondence gate               | m, via `T²`   | 1.28                            | 0.32                       |
| Final correspondence gate               | m             | 0.3                             | 0.3                        |
| Keyframe spacing                        | m, degrees    | 10, 10 (10 rotations at 10 m/s) | 10, 10 (20 rotations)      |
| Map insertion                           | m, degrees    | 0.5, 2                          | 0.5, 2                     |
| Local map radius                        | m             | 100                             | 100                        |
| Submap length                           | m, degrees    | 50, 30 (50 rotations at 10 m/s) | 50, 30 (100 rotations)     |
| Odometry factor covariance              | Per m, per s  | Table above                     | Same values                |
| Stillness lag and bin                   | s, degrees    | 1.0 (10 rotations), 0.4         | 1.0 (20 rotations), 0.4    |
| Minimum stop                            | s             | 2                               | 2                          |
| Speed and acceleration window           | s             | ±0.5 (11 poses)                 | ±0.5 (21 poses)            |
| Jerk window                             | s             | ±0.75 (15 poses)                | ±0.75 (31 poses)           |
| White pose noise `σ_w`                  | m, measured   | 0.01 assumed                    | 0.01–0.014 expected        |
| Random-walk rate `q`                    | m²/s          | 1e-3 assumed                    | Same if per metre          |
| Trajectory gap                          | s             | 0.35 (splits at 3 missing)      | 0.35 (splits at 7 missing) |
| Spectrum segment and band               | s, Hz         | 6.4 (64 poses); 1.3–5           | 6.4 (128 poses); 1.3–10    |
| Anchor averaging window                 | s             | 5 (50 poses)                    | 5 (100 poses)              |
| Clock fit: blocks, packets, centre σ    | Per rotation  | 1,800; 180; 13 µs               | 900; 90; 18 µs             |
| Clock period gate                       | % of median   | 0.5                             | 0.5                        |
| Frame completion                        | Degrees       | 340                             | 340                        |
| Points per rotation against the minimum | Count         | 72,000 against 10,000           | 36,000 against 10,000      |

The 5 Hz arm keeps `T = 0.100 s` per sweep with a 0.200 s pose interval; its first gate is 2.0 m.

## Two-phase work order

Sizes (judgement): S is up to two days, M up to two weeks, L more. "⅓" marks items that survive
if only a third of the effort is available. "20" marks items that wait on a 20 Hz capture.

### Phase 1: a bridge and an ego track from the archive drives

Runs on existing drives only. It ends in hydra evaluation 1.

| #   | Backlog title                                                                                     | Repo            | Size | Inputs                                | Outputs                                             | Kill or continue                                                              | Mark |
| --- | ------------------------------------------------------------------------------------------------- | --------------- | ---- | ------------------------------------- | --------------------------------------------------- | ----------------------------------------------------------------------------- | ---- |
| 1   | Export: tighten the azimuth-clock gates, record fit coefficients and tail rpm, bound frame growth | velocity.report | S    | Archive captures                      | `run.json`, `frames.csv` with fit and trust columns | Continue if ≥ 99.5% of rotations are trusted and fitted rate matches tail rpm | ⅓    |
| 2   | Export: pose-free stillness flag per rotation                                                     | velocity.report | M    | L2 rotations                          | `still` column, list of still intervals             | Continue if it marks both static margins and no rotation in obvious motion    |      |
| 3   | SLAM import: read LAS, `run.json` and `frames.csv`; write anchors from registration records       | Gerardus        | S    | Export, record log                    | Session in memory, `anchors.json` with quaternions  | Stop if a record's frame cannot be related to EPSG:7131                       | ⅓    |
| 4   | Odometry: point-to-plane scan-to-map on a keyframe-rebuilt local map with twist de-skew           | Gerardus        | M    | Item 3, `icp` behind a trait          | Per-rotation odometry poses, quality, regime label  | Kill or rethink if under 95% of drive-regime rotations register               | ⅓    |
| 5   | Odometry: second pass with centred de-skew, and a time-reversed run                               | Gerardus        | S    | Item 4 poses                          | Pass-2 poses, reverse poses                         | Continue if pass 2 lowers residuals in turns                                  |      |
| 6   | Handover check: margin scan against the accepted pose at each end                                 | Gerardus        | S    | Item 3, bundle, thinned static export | Pass or fail per end, replacement factor            | A failed end removes that anchor and the drive's held-out claim               |      |
| 7   | Bridge solve: keyframe graph, per-metre covariance, A-only, B-only and A+B, one-inverse marginals | Gerardus        | M    | Items 4–6                             | Odometry and solved trajectories (v0), residuals    | Kill if the open-loop residual exceeds 2% of moving path in both directions   | ⅓    |
| 8   | Ego track: correlated-noise σ, jerk window, gap in seconds, external stops, spectrum significance | velocity.report | M    | Item 7 trajectories, item 2 intervals | `summary.json`, `kinematics.csv` per pose set       | Continue if the σ ratio at stops is in [0.7, 1.4] after calibration           |      |
| 9   | Hydra evaluation 1: three drives, three rate arms, authored report                                | Both            | S    | Items 1–8                             | Report with summary tables only                     | The phase gate below                                                          | ⅓    |

At one third of the effort, item 4 runs a single pass, item 7 drops the B-only solve, and the
ego track runs as it is today with its caveats stated; the stop and σ gates are then not
testable.

**Evaluation 1.** Development on marina-webster-beach → lombard-laguna and columbus-broadway →
van-ness-sacramento; then one run on lombard-laguna → columbus-north-point. Arms: native, the
5 Hz export, and native poses decimated.

**Gate 1.** Continue to Phase 2 if, on the test drive: both handovers pass or the failing end is
explained and replaced; the open-loop residual is at most 0.5% of the moving path and 1° in both
directions, with the innovation statistic in [0.115, 11.3]; odometry drift over pose-free still
intervals is at most 0.01 m/s median and 0.03 m/s worst, with none missed; the σ ratio is in
[0.7, 1.4]; and at least 98% of drive-regime rotations register. Change course, not kill, if
stops show traffic bias (do dynamic removal first) or carried stretches break the chain
(restrict to driven stretches and let Phase 2's factors anchor them). Kill the bridge approach
if the residual exceeds 2% in both directions after an external KISS-ICP cross-check agrees
with it: the data, not the code, is then the limit.

### Phase 2: priors along the route, and the rate question

Items 10–15 run on the archive. Items 16 and part of 18 need the new capture.

| #   | Backlog title                                                                          | Repo            | Size | Inputs                        | Outputs                                        | Kill or continue                                                                | Mark       |
| --- | -------------------------------------------------------------------------------------- | --------------- | ---- | ----------------------------- | ---------------------------------------------- | ------------------------------------------------------------------------------- | ---------- |
| 10  | Submaps and prior factors with weak-direction projection                               | Gerardus        | M    | Phase 1 poses, site bundles   | Per-submap fits, gates, projected factors      | Change course if under one full-rank factor per 500 m: add feature anchors      | ⅓          |
| 11  | Alias guard and robust re-solve for prior factors                                      | Gerardus        | S    | Item 10, Phase 1 drift model  | Accepted and dropped factors with reasons      | Stop if any accepted factor sits a lot period from its leave-one-out prediction | ⅓          |
| 12  | Leave-one-out factor residuals, gates at the solved pose, covariance scale             | Gerardus        | S    | Items 10–11                   | Residual table, scale factor, per-block scale  | Continue if the scale is within [0.5, 2] of Phase 1's                           | ⅓          |
| 13  | Sub-rotation heave from per-sector ground fits                                         | velocity.report | M    | Export with point times       | Heave and tilt spectra to 40 Hz                | Drop if road roughness swamps it at stops                                       |            |
| 14  | Export: packet sidecar with arrival and native sensor time                             | velocity.report | S    | L1 tail fields                | `packets.csv`, native-against-azimuth residual | Continue regardless; it is evidence                                             |            |
| 15  | Trajectory v1: two pose sets, noise model and regime; `PoseProvider` reads it          | velocity.report | S    | Phase 1 experience            | Schema, reader, gap refusal                    | Hold until Gerardus's proto is drafted                                          |            |
| 16  | Capture: native 20 Hz drive of the registered pair's route, in the order 10, 20, 10 Hz | Capture         | M    | Owner decision, mount, sensor | Three bundles with settings in provenance      | Void if tail rpm or coverage is not as planned                                  | 20         |
| 17  | Voxel-hash map with running moments                                                    | Gerardus        | M    | Item 4 timings                | Faster map                                     | Build only if item 4 runs slower than ten times real time                       |            |
| 18  | Hydra evaluation 2: every bridge drive, and arms N20, D10 and N10                      | Both            | M    | Items 10–16                   | Report with summary tables only                | The phase gate below                                                            | 20 in part |

**Gate 2, priors.** Continue the series if: at least 80% of gated submaps pass at the solved
pose; at least one accepted full-rank factor per 250 m over 80% of each route; pooled
leave-one-out residuals have a median of at most 0.15 m with 90% within 0.30 m and at least 20
samples; the held-out registered end is within 0.3 m and 0.3°; and no accepted factor is a lot
period off. If along-street factors are too few, change course to feature anchors at
intersections, as the plan already says.

**Gate 2, rate.** Adopt 20 Hz for a platform only if N20 beats both D10 and N10 on that
platform's binding measure by more than the spread between the two 10 Hz drives. Otherwise
keep 10 Hz and record what 20 Hz did show.

## What I am least sure of

| Uncertainty                                                        | Measurement that settles it                                                   |
| ------------------------------------------------------------------ | ----------------------------------------------------------------------------- |
| How the sensor rode between sites, and whether it was carried      | Field notes; ground height and tilt per rotation across the first site bound  |
| The size of the random-walk term `q`, hence the true ego σ         | Structure function at pose-free stops; fit residual against window length     |
| Whether drift is a random walk or systematic                       | Residual against path length at checkpoints; forward against reverse odometry |
| Whether structure above 2.5 m keeps along-street motion observable | Along-street eigenvalue share per rotation on the first drive                 |
| The real clock offset and its stability                            | Native packet time against the azimuth clock (item 14)                        |
| The rotor beat of answer 17 and its size                           | A line that moves between N10 and N20; the per-sector heave spectrum          |
| My reading of the LAS global-encoding bit (D15)                    | The LAS 1.4 specification's global-encoding table                             |
| The real anchor accuracy, beyond formal σ                          | The along-wall residual variogram the Gerardus note §3.3 already recommends   |

## Open questions

Owner decisions, each with my default:

- How did the sensor ride between sites? Default: treat as unknown, and make regime labelling and
  the handover check mandatory.
- Accept the phase boundary (ego track before priors)? Default: yes.
- Reserve lombard-laguna → columbus-north-point as a test drive that is never tuned on, and
  admit the columbus-north-point → embarcadero-bay → columbus-broadway chain as a second test?
  Default: yes to both.
- Use manual ends for held-out claims? Default: no; use the drives to test the placements.
- Floor registered anchors at 0.05 m and 0.05° in the solve, pending real covariances?
  Default: yes.
- Remove velocity from the trajectory contract, so that only velocity.report differentiates?
  Default: yes.
- Report peak jerk with its under-read factor, or the acceleration step alone? Default: both.
- Capture a native 20 Hz session, and when? Default: one 10–20–10 session on the registered
  pair's route, after gate 1 passes.
- Run KISS-ICP's Python release on hydra as an outside cross-check, as the Gerardus plan
  allows? Default: yes, at gate 1 only if the residual is poor.

What only measurement can settle:

- Is the archive 10 Hz throughout? The tail rpm and the fitted rate answer it per drive.
- Which drift model holds, and so whether the chain can exclude a lot-period alias unaided.
- How large the stated-σ correction is: it rests on `q`, which is not known today.
- Whether 10 Hz with centred de-skew keeps the facade gate in turns, or a two-pose sweep model
  is needed after all.
- Whether any vibration above 5 Hz matters for the backpack and bike regimes.
- Whether a second oracle reaches the same gates. Where it differs on a number in the tables
  above, the derivation, not the vote, should decide.
