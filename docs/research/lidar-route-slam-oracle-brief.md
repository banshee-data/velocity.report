# Oracle brief: the route SLAM minimum at 10 and 20 Hz

Plan the minimum implementation that registers the archive's car drives against Gerardus's
public priors along the route (a) and derives the sensor platform's own track (b): speed,
acceleration, jerk, yaw rate and vibration, with their uncertainty. Say how each should run when
the Pandar40P turns at 10 Hz and at 20 Hz. The answers set the Gerardus implementation order and
fix the gates the hydra evaluation is read against before its results exist.

- **Status:** Answered once, by oracle run A (Euler persona on Fable), in the
  [run A response](lidar-route-slam-oracle-response-fable.md). The owner chose a single run, so
  no cross-check or comparison exists. Its recommendations are laid out in the phase plans and
  are not adopted: no answer has changed a gate or a default.
- **Scope:** Offline odometry, prior factors and a two-anchor solve over drives between registered
  static sites; ego kinematics from the resulting trajectory; rotation rate (10 Hz, 20 Hz and the
  5 Hz temporal ablation of the 10 Hz archive).
- **Plan:** [lidar-route-slam-minimum-plan](../plans/lidar-route-slam-minimum-plan.md)
- **Evaluation:** [route-drive-bridge-first-pair](../../data/experiments/try/route-drive-bridge-first-pair.md)
- **Related:** [20 Hz pipeline readiness audit](lidar-20hz-readiness.md), [route capture plan](../plans/lidar-route-capture-plan.md),
  [portable capture timing](../lidar/architecture/portable-capture-timing.md)
- **Baseline:** branch `claude/determined-tesla-upaoqr` on `main` at `e6fb5d62a8a8f4c2158ce19feb9d55c8e41344b2`;
  record the exact head in the execution record when a run starts.

## Request to the oracle

You are Euler: a patient, rigorous reviewer of algorithms and statistics for a traffic
measurement project whose numbers go to public meetings. Plan the **minimum** implementation, not
the ideal one, and say what each shortcut costs in error.

- Restate the problem in at most ten bullets first, so misunderstandings surface early.
- Tag every claim as **fact** (cite the evidence item, appendix and section), **derivation** (show
  it) or **judgement** (say so). Say "I don't know" where you don't.
- Every number you propose carries units, a domain where it holds and what happens outside it.
- Read the named code before proposing a new implementation of something that already exists.
  If you propose a dependency, say why the existing code cannot do the job.
- Give every recommendation a falsifier: the observation in the hydra evaluation that would show
  it wrong.
- Be adversarial about the evaluation: say how each check could pass while the result is wrong.

## The problem

**velocity.report** measures traffic with a Hesai Pandar40P (40 rings, single return) and a
radar, on fixed tripods, without cameras. **Gerardus** is its spatial priors service. It
registers sensor scans against USGS 3DEP aerial LiDAR and DataSF building footprints and street
nodes, and it never uses OpenStreetMap as an anchor or a factor.

In September 2026 the sensor moved by car between 24 tripod sites over three days, and the
recording ran through the drives. Every site now has an accepted pose, so each drive between
consecutive sites is a motion segment with known ends. The project wants:

- **(a) Priors registration along the route:** the drive's trajectory pinned to 3DEP and DataSF
  by per-submap prior factors, with the static registration gates reported per submap.
- **(b) Ego vehicle track:** position, speed, longitudinal and lateral acceleration, jerk, yaw
  rate and vibration ("jitter") over time, each with stated uncertainty. It feeds later work:
  ego-velocity de-biasing of other vehicles' speeds, a radar speed comparison, and the choice of
  motion-correction regime for backpack and bike rigs.
- **Both at 10 Hz and 20 Hz:** the archive is believed to be 10 Hz. A later capture may run the
  motor at 20 Hz, and the project needs to know how SLAM and the ego track should be configured,
  and compared, at each rate.

There is no IMU, no GNSS log of known quality and no loop. Raw captures and every derived
trajectory stay on one local machine ("hydra"). A trajectory is the operator's location history
and is never published.

## Evidence

Facts the answers may cite. Paths are in velocity.report unless marked Gerardus.

### Anchors and drives

- **E1.** Six sites are `REGISTERED` by ICP that passed every gate (σ 2–19 mm, heading σ
  0.004–0.024°). Eighteen are `MANUALLY_PLACED` by an operator, mostly with no covariance; five
  have a narrow export-check margin (Gerardus `docs/REGISTRATION-STATUS.md` §1).
- **E2.** There are 20 same-day consecutive drives of 5–34 min (one of 82 min). One,
  lombard-laguna → columbus-north-point (16.3 min, about 1.3 km apart in a straight line, files
  `s2_sf_2_…_00026` to `_00030`), has both ends `REGISTERED`; seven have one registered end
  (`tools/s2-archive/site-index.json`).
- **E3.** The archive's rotation rate is not recorded in the site index. It is believed to be
  10 Hz. The exporter now measures it per drive (`rotation_hz` in `run.json`).

### Timing

- **E4.** The recorded point time is PCAP arrival time plus channel firetime; the block index
  contributes nothing, so the ten blocks of a packet share one time
  (`internal/lidar/l1packets/parse/extract.go`, `blockToPoints`;
  `docs/lidar/architecture/portable-capture-timing.md`).
- **E5.** `lidar-motion-export`'s default `azimuth_clock_v1` fits each rotation's block times to a
  least-squares line in unwrapped block azimuth (one 4×MAD rejection pass), then adds back each
  point's channel offset. In synthetic tests with 300 µs arrival jitter it recovers firing times to
  under 20 µs RMS at 10 Hz (1800 blocks of 0.2°) and under 25 µs at 20 Hz (900 blocks of 0.4°). It
  leaves a constant offset of about half a packet's block span (about 250 µs). On real 20 Hz
  packets, the recorded block times scattered 252 µs RMS (4.9 ms maximum) about the line, and the
  fitted period was 0.050003 s. A rotation is refused when its block azimuth spans less than 300°
  or its period falls outside 0.04–0.25 s. The LAS time field is session seconds, not GPS time.

### Rotation rate

- **E6.** The Pandar40P supports 10 Hz (0.2° azimuth step) and 20 Hz (0.4°). Single-return
  output is 720,000 points/s at either rate: an ideal 72,000 points per rotation at 10 Hz and
  36,000 at 20 Hz. Horizontal spacing at 50 m is 0.175 m against 0.349 m. There is no 5 Hz motor
  mode ([20 Hz readiness audit](lidar-20hz-readiness.md), from the Hesai manual §1.5).
- **E7.** The audit (#1030, source only, no measurement) found no 5 Hz throttle in the live or
  analysis path. The "5 Hz" in the docs means two other things: a historical cohort's effective
  successful-observation rate, and scene exports that keep one rotation in two (`STRIDE = "2"` in
  `tools/s2-archive/publish-scenes.py`). The audit also lists 10 Hz hard-coding in
  `SpatialCoverage` (duration × 10) and `MaxOcclusionFrames` (100 ms), and several frame-count
  tuning defaults whose physical duration halves at 20 Hz.
- **E8.** The frame builder completes a rotation only once it covers at least 340° of azimuth
  (`MinAzimuthCoverage`, `internal/lidar/l2frames/frame_builder.go`). The repository's only 20 Hz
  capture (`internal/lidar/perf/pcap/lidar_20Hz.pcapng`) covers a 60–290° sector. It therefore
  never completes a rotation, and a frame accumulated over its whole length until memory ran out
  in a 3 s test. No full-rotation 20 Hz capture exists in the repository.
- **E9.** The audit's derivation: for independent position noise σ at spacing Δ, simple first and
  second differences have noise variance 2σ²/Δ² and 6σ²/Δ⁴. It warns that 20 Hz must not be sold
  as better acceleration or jerk without time-based windows.
- **E10.** Measured with `egotrack`'s local cubic at σ_pos = 0.01 m per axis (stated σ, constant
  velocity; derived from the fit covariance):

  | Rate  | Half window | Speed σ (m/s) | Acceleration σ (m/s²) | Jerk σ (m/s³) |
  | ----- | ----------- | ------------- | --------------------- | ------------- |
  | 5 Hz  | ±0.5 s      | 0.0475        | 0.134                 | 1.98          |
  | 5 Hz  | ±1.0 s      | 0.0123        | 0.017                 | 0.095         |
  | 10 Hz | ±0.5 s      | 0.0246        | 0.068                 | 0.763         |
  | 10 Hz | ±1.0 s      | 0.0091        | 0.013                 | 0.076         |
  | 20 Hz | ±0.5 s      | 0.0182        | 0.053                 | 0.608         |
  | 20 Hz | ±1.0 s      | 0.0066        | 0.010                 | 0.058         |

  With 20 Hz poses given √2 the noise for half the points, speed σ over the same window matched
  10 Hz within 0.8–1.25 in a synthetic test.

- **E11.** The temporal ablation exists. `-keep-every-rotation 2` makes a 10 Hz drive read as 5 Hz
  at 10 Hz angular density, and a future 20 Hz drive read as 10 Hz at 20 Hz density. The audit
  notes that neither decimation nor a faster replay reproduces a sensor running at the other rate.

### The ego-track estimator

- **E12.** `egotrack` reads `velocity.report/ego-trajectory/v0` (per frame: t, p, a unit
  quaternion, optional σ_pos). At each sample it fits a weighted local polynomial (degree 3,
  ±0.5 s by default) per axis and takes the derivatives from the coefficients. σ comes from the
  exact coefficient covariance, assuming independent isotropic per-pose noise. Course,
  longitudinal and lateral parts are computed only above 1 m/s and 3σ. Yaw rate comes from the
  unwrapped yaw on the same fit. Stops are speed < 0.3 m/s for ≥ 2 s, with raw displacement
  across each.
- **E13.** Jitter is a Welch spectrum (Hann, a 6.4 s segment at any rate, 50% overlap) of z,
  pitch and roll residuals about the fit. It is reported with the fit's gain |1 − H(f)| and a
  gain-corrected peak amplitude. A 2 Hz sine is over-read by about 24% before correction, where
  the cubic's response is negative. A 7 Hz vibration is resolved at 20 Hz (Nyquist 10 Hz) and
  aliases to 3 Hz at 10 Hz (Nyquist 5 Hz). An instantaneous 0 → 8 m/s step overshoots to
  8.48 m/s.

### Existing code to build on

- **E14.** Gerardus `src/register.rs` provides:
  - robust point-to-plane ICP (Huber 0.10 m, planarity weights, SE(3) left-perturbation);
  - the 6×6 information matrix and its rank, with rotations scaled at 10 m;
  - hypothesis separation against along-street aliases (±3.7 m and ±7.6 m lot periods);
  - the static gates: held-out ground p95 ≤ 0.20 m, facade p75 ≤ 0.20 m, facade range slope
    ≤ tan 0.5°.
- **E15.** Gerardus `src/graph.rs` provides a small SE(3) Gauss–Newton/LM pose-graph solver
  (`solve(initial, factors, iters)`), used today to refine several static scans at one site.
- **E16.** Gerardus `docs/plans/MOBILE-SLAM-REPLAY.md` (S0–S3) and
  `docs/research/2026-10-24-registration-and-slam-maths.md` §4 cover de-skew residue,
  KISS-ICP-style odometry on the existing ICP, a voxel-hash map with running moments, ~50 m
  submaps, prior factors with weak eigen-directions projected out, drift budgets and compute. They
  were written for a walked loop at 10 Hz, not car drives.
- **E17.** On the velocity.report side:
  - the route plan's trajectory contract (pose, velocity and quality per frame, plus placement,
    provenance, timing and calibration);
  - a specified but unimplemented `PoseProvider`;
  - a 4-state constant-velocity Kalman tracker;
  - a fixed-lag RTS smoother whose capture-time lag already allows 20 Hz
    (`internal/lidar/l5tracks/smoother.go`);
  - constant-acceleration and IMM models planned next.

## Questions

Answer every question. For each, give the minimum you recommend, what you would defer, and the
observable that would show the minimum was not enough.

### A. Observability of a two-anchor bridge

1. With a static pose at each end (σ_A, σ_B) and odometry with per-metre drift rate d, derive the
   along-street, across-street and yaw uncertainty at fraction s of a drive of length L. Do it
   (i) with A only, (ii) with A and B, and (iii) with prior factors every 50 m: across-street and
   yaw only mid-block, all six directions at intersections. At what L does the far anchor stop
   helping mid-route?
2. Is replacing loop closure (Gerardus S2) with the far anchor sound for a point-to-point drive?
   What does the A-only held-out residual at B test (drift, prior registration, anchor error, the
   static-to-motion handover), and how should those be separated?
3. Most `MANUALLY_PLACED` poses carry no σ. What σ should the solve assign them? Can the export
   check's margin be turned into one, and is such an end safe as a held-out check at all?
4. Is the static pose valid for the drive's first moving rotation, if the mount may have moved
   between the static capture and the drive? What minimal test from the exported rotations alone
   shows it, and what does the solve do if it fails?

### B. Timing and de-skew at car speed

5. At 0–15 m/s with turns at intersections, is constant-velocity de-skew from the previous
   odometry step good enough for the 0.20 m prior gates? Use the maths note's §4.1 residue
   formulae with car-plausible acceleration and steering rate. Answer separately for 100 ms and
   50 ms sweeps. Where it fails, is a two-pose per-sweep (CT-ICP-style) model the minimum fix?
6. The azimuth clock fits one line per rotation. Should it fit across several rotations, or the
   whole segment with a slowly varying rate? At 20 Hz each rotation has half the blocks: does the
   per-rotation fit lose anything that matters? What residual or period behaviour should make a
   rotation's times untrusted, and what should the exporter do then?
7. The constant offset (about 250 µs) and the PCAP arrival latency cancel in scan-to-scan
   odometry. Where in (a) or (b) do they not cancel? Bound their effect on ego speed and on a
   later radar-to-LiDAR speed match.
8. Is session-seconds time in the LAS export correct? What must `run.json` carry so a later run
   can re-time the points without re-reading the captures?

### C. Minimum for (a): odometry, submaps and prior factors

9. Given E14 and E15, what is the smallest new code? Cover the voxel-hash map, the adaptive
   correspondence threshold, a relative-pose factor and its covariance, and keyframe and submap
   rules at car speed. Which KISS-ICP elements can be dropped for a first drive?
10. ICP's information matrix is overconfident for odometry factors (correlated residuals, n_eff).
    Propose a covariance calibration the first drive can check by itself, for example against the
    stops or the held-out anchor. State it per second or per metre, not per rotation, so it holds
    at both rates.
11. Prior factors: should a mid-block submap add a factor with weak directions projected out, or
    no factor? Choose the threshold, and say how to keep an along-street alias one lot period away
    out of the graph.
12. Moving traffic: is the robust kernel enough, or must dynamic points be removed before
    odometry? Can that be done without the background model, which does not work on a moving
    sensor?
13. Acceptance for (a) on the first drive: which gates per submap and per drive, which held-out
    quantities, and which minimum counts are needed for the result to mean anything? Say what a
    pass would and would not establish about mobile registration in general.

### D. Minimum for (b): the ego track, speed, jerk and jitter

14. Is a local weighted cubic the right minimum estimator from 10 or 20 Hz poses? Compare it
    with an RTS smoother on a constant-acceleration or constant-jerk model, and with a
    continuous-time spline over the pose graph. Should (b) share one smoother family with the
    tracker work, so the uncertainty language matches?
15. From E10, the default ±0.5 s window cannot resolve 1 m/s³ of jerk at 3σ with 1 cm pose noise
    at 10 Hz, while ±1 s can but passes only motion well below about 0.5 Hz. Derive the window
    that minimises bias plus noise for urban braking onsets, at 10 and at 20 Hz. Should jerk be
    reported at a different window from speed?
16. Odometry error is closer to a random walk than to independent noise. Derive what that does to
    the stated σ of speed, acceleration and jerk within the window. Propose the minimum
    correction: a different σ model, or σ from the pose-graph marginals.
17. Jitter: per-rotation poses observe 0–5 Hz at 10 Hz and 0–10 Hz at 20 Hz, while car body and
    mount vibration extends higher. What can honestly be called jitter at each rate? Would
    per-sector poses within a rotation raise the band enough to matter, and at what cost? Is the
    gain-corrected amplitude a sound way to report a residual seen through |1 − H(f)|? How should
    the 10 Hz report guard against aliased energy (E13)?
18. Stops: is displacement over a stop a valid drift check when the stopped car still sees moving
    traffic? Should zero-velocity intervals enter the pose graph as factors, and if so, does that
    spoil them as a check?
19. Which quantities belong in Gerardus's trajectory (velocity and its covariance) and which in
    velocity.report's kinematics, so the trajectory contract stays minimal and nothing is
    differentiated twice?

### E. Evaluation and its traps

20. Without an IMU or GNSS of known quality, rank the independent references for (b): the
    held-out anchor, stop displacement, the A–B route length from DataSF street centrelines, and
    the later fixed-radar comparison. Say what each misses.
21. How can each success criterion in the evaluation spec pass while the trajectory is wrong?
    Propose one extra check per criterion that would catch it.
22. Is one drive with two registered ends enough to choose defaults (fit window, stop threshold,
    submap length)? If not, what is the smallest set of drives, and how should the seven
    one-registered-end drives be used?

### F. Rotation rate: SLAM at 10 Hz and 20 Hz

23. For scan-to-map odometry at 0–15 m/s, derive how 20 Hz changes each of these against 10 Hz:
    - inter-scan motion and the ICP convergence basin;
    - de-skew smear (∝ vT) and constant-velocity residue (∝ aT²);
    - points per scan and far-range spacing (E6);
    - compute per second.

    Which effect dominates at urban car speeds, and at walking pace?

24. At 20 Hz, should each rotation register against the local map on its own, or should
    consecutive pairs be merged into 100 ms sweeps (twice the density, back to 100 ms of motion)?
    Does the answer change for odometry against prior factors?
25. List every parameter that must be stated in seconds or metres, not rotations, for the same
    configuration to run at both rates: keyframes, submaps, the adaptive threshold, the motion
    model, odometry-factor covariance, the ego-track window and spectrum. Name any rate-coupled
    contract in the existing code or plans that the SLAM path would inherit (E7, E8) and what to do
    about each.
26. Submaps aggregate the same points per second at either rate. Are the prior gates therefore
    rate-invariant? Where not: motion blur at 10 Hz in turns, or sparser far structure per scan at
    20 Hz?
27. Ego track at 20 Hz: per-pose noise may grow with fewer points per scan while the sample count
    doubles (E10). What window keeps σ and bias comparable across rates? Is reporting both rates in
    the same units and windows enough for a fair comparison?
28. The archive allows only the temporal ablation: 10 Hz kept one rotation in two, read as 5 Hz at
    10 Hz density (E11). What does running SLAM and the ego track on that ablation predict about
    a native 20 Hz capture, and what can only a native 20 Hz capture answer? Specify the minimum
    20 Hz capture (route, conditions, motor settings, whether the same route must be driven at
    both rates) and its comparisons: native 20 Hz, 20 Hz decimated to 10 Hz, and native 10 Hz.
29. Recommend a rotation rate for mobile capture by platform (car, bike, backpack), for SLAM and
    for traffic measurement. Processing is offline, so the audit's 48 ms budget does not bind
    SLAM. Does anything bind the recorder at 20 Hz?
30. Is the sector-limited 20 Hz fixture (E8) evidence of a defect to fix in the frame builder
    (completion should not depend on a 340° sweep when the sensor's field of view is configured
    narrower), or a property to document? What should the exporter do on such a capture?

### G. Sequencing

31. Give the order of work as a numbered list. For each item give the repository, its size
    (S, M or L), its inputs and outputs, and a kill-or-continue criterion. Mark anything you would
    cut if only a third of the effort were available, and mark which items wait on a 20 Hz capture.
32. Name any decision the project owner must take before implementation, with your default.

## Output format

1. Restatement (at most ten bullets).
2. Answers 1–32, each opening with a one-line recommendation, then the reasoning, tagged fact,
   derivation or judgement.
3. A table of proposed gates and defaults: name, value, units, domain, rate dependence,
   derivation reference.
4. A rate table: for each parameter in question 25, its value at 10 Hz and 20 Hz and the unit it
   is fixed in.
5. The ordered work list (question 31).
6. What you are least sure of, and the measurement that would settle it.

## Appendices

From velocity.report:

- `docs/plans/lidar-route-slam-minimum-plan.md`
- `docs/research/lidar-20hz-readiness.md`
- `docs/plans/lidar-route-capture-plan.md` (trajectory contract, rigs, privacy)
- `docs/lidar/architecture/portable-capture-timing.md` (timing audit and error budget)
- `data/experiments/try/route-drive-bridge-first-pair.md`
- `internal/lidar/motionexport/clock.go`, `internal/lidar/egotrack/kinematics.go`,
  `internal/lidar/egotrack/analysis.go`, `internal/lidar/l2frames/frame_builder.go` (rotation completion)
- `tools/s2-archive/site-index.json` (site times, for the drive list)

From Gerardus:

- `docs/plans/MOBILE-SLAM-REPLAY.md`
- `docs/research/2026-10-24-registration-and-slam-maths.md` (§1 observability, §4 sizing)
- `docs/REGISTRATION-STATUS.md` §1 (accepted poses and σ)
- `src/register.rs` (`icp`, `information_rank`, gates) and `src/graph.rs` (`solve`)

## Execution record

Run the brief on at least two independent oracles before any answer changes a gate. The primary is
the Euler persona (`.claude/agents/euler.md`) on Fable. The cross-check is a second model with the
same persona, given the same brief and appendices and no sight of the first answer. Write each
response to `docs/research/lidar-route-slam-oracle-response-<oracle>.md` with Status, Evidence and
Open questions sections. Then compare them question by question in
`docs/research/lidar-route-slam-oracle-comparison.md` before the plan adopts anything.

| Run | Oracle                      | Brief at commit                | Started    | Response                                              | Notes                                                                                                              |
| --- | --------------------------- | ------------------------------ | ---------- | ----------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------ |
| A   | Euler persona, Fable        | `ab84a68` (Gerardus `e04587c`) | 2026-11-17 | [response](lidar-route-slam-oracle-response-fable.md) | Read-only: nothing executed. Corrects E1, E2, E3, E5, E8, E14, E15 and the LAS time flag; see its Evidence section |
| B   | Euler persona, second model | —                              | —          | —                                                     | not run: the owner chose a single run                                                                              |

## Open questions

- Is the archive 10 Hz throughout? The first export's `rotation_hz` answers it per drive.
- Will a native 20 Hz drive be captured, and on which route? Question 28 asks for its design;
  the owner decides whether and when.
- Is a second oracle run wanted before the owner pins run A's gates? Only run A exists, and
  its response asks that a differing number be settled by derivation, not by vote.
