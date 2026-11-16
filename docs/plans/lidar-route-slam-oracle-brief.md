# Route SLAM oracle brief: minimum work for priors registration and the ego track

Questions for an independent mathematical review of the route SLAM minimum: the drives between
registered static sites, registered against Gerardus's public priors (a), and the sensor
platform's own track with its speed, acceleration, jerk and vibration (b). The answers set the
Gerardus implementation order and the gates the hydra evaluation is read against.

- **Status:** Proposed; not yet run
- **Plan:** [lidar-route-slam-minimum-plan](lidar-route-slam-minimum-plan.md)
- **Evaluation:** [route-drive-bridge-first-pair](../../data/experiments/try/route-drive-bridge-first-pair.md)
- **Canonical:** [motion-capture.md](../lidar/operations/motion-capture.md)

How to run it: paste everything below the line into the reviewing model, with the Euler
persona (`.claude/agents/euler.md`) as its role, and attach the appendices listed at the end.
Run it on two or three independent oracles and compare the answers question by question.
Record the comparison next to this brief before any answer changes a gate. As with Gerardus's
2026-10-20 oracle round, an oracle's answer is judgement to be checked. It is not evidence.

---

## 1. Your role

You are Euler: a patient, rigorous reviewer of algorithms and statistics for a traffic
measurement project whose numbers go to public meetings. Your task is to plan the **minimum**
implementation, not the ideal one, and to say what each shortcut costs in error.

- Restate the problem in at most ten bullets first, so misunderstandings surface early.
- Separate **fact** (cite the appendix and section) from **derivation** (show it) and
  **judgement** (say so). Say "I don't know" where you don't.
- Every number you propose carries units, a domain where it holds and what happens outside it.
- Prefer reusing what exists (named below) over new machinery. If you propose a new
  dependency, say why the existing code cannot do it.
- Be adversarial about the evaluation: say how each proposed check could pass while the
  result is wrong.

## 2. The problem

**velocity.report** measures traffic with a Hesai Pandar40P (40 rings, 10 Hz, 0.2° azimuth
bins, single return) and a radar, on fixed tripods, without cameras. **Gerardus** is its spatial
priors service: it registers sensor scans against USGS 3DEP aerial LiDAR and DataSF building
footprints and street nodes, and it never uses OpenStreetMap as an anchor or factor.

In September 2026 the sensor was moved by car between 24 tripod sites over three days. Every
site now has an accepted pose: 6 `REGISTERED` by ICP that passed every gate (σ 2–19 mm, heading
σ 0.004–0.024°), and 18 `MANUALLY_PLACED` by an operator, mostly with no covariance. The
recording ran through the drives, so each drive between consecutive sites is a motion segment
whose start and end poses are known. There are 20 such same-day drives, 5–34 minutes long. One,
lombard-laguna → columbus-north-point (16.3 min, about 1.3 km apart in a straight line), has
both ends `REGISTERED`.

The project wants two results from these drives, with the least work:

- **(a) Priors registration along the route:** the drive's trajectory pinned to 3DEP/DataSF by
  per-submap prior factors, with the static registration gates reported per submap.
- **(b) Ego vehicle track:** the platform's position, speed, longitudinal and lateral
  acceleration, jerk, yaw rate and vibration ("jitter") over time, each with a stated
  uncertainty. It feeds later work: ego-velocity de-biasing of other vehicles' speeds, a radar
  speed comparison, and the choice of motion-correction regime for backpack and bike rigs.

There is no IMU, no GNSS log of known quality and no loop. The planned walked block loop
(Gerardus S2) has not been captured. Raw captures and all derived trajectories stay on one local
machine ("hydra"). A trajectory is the operator's location history and is never published.

## 3. What exists (facts)

**velocity.report, including the change this brief accompanies:**

- `lidar-motion-export`: L1 decode and L2 rotations only, over an ordered capture list whose
  joins are graded. Writes sensor-frame LAS 1.4, point format 6, with extra bytes `ring` (u8),
  `azimuth_deg` (f32) and `frame` (u32). Splits into files by capture time and writes
  `run.json` (inputs and digests, build stamp, time convention and its measured quality) and
  `frames.csv`.
- **Point time.** The recorded time per point is PCAP arrival time plus channel firetime. Block
  index contributes nothing, so the ten blocks of a packet share one time (about 0.5 ms of
  sawtooth). The exporter's default `azimuth_clock_v1` fits each rotation's block times to a
  least-squares line in unwrapped block azimuth (one 4×MAD rejection pass), then adds back each
  point's channel offset. On synthetic data with 300 µs arrival jitter it recovers firing times
  to under 20 µs RMS, leaving a constant offset of about half a packet's block span. On a real
  capture the recorded block times scattered 252 µs RMS (4.9 ms maximum) about the line, with a
  fitted period of 0.050003 s (that capture ran at 20 Hz). The LAS time field is session
  seconds, not GPS time.
- `egotrack` / `lidar-ego-track`: reads `velocity.report/ego-trajectory/v0`: per frame, t, p, a
  unit quaternion and optional σ_pos. At each sample it fits a weighted local polynomial (degree
  3, ±0.5 s by default) per axis and takes the derivatives from the coefficients. σ comes from
  the exact coefficient covariance, assuming independent isotropic per-pose noise. Course,
  longitudinal and lateral parts are computed only above 1 m/s and 3σ. Yaw rate comes from the
  unwrapped yaw on the same fit. Stops are speed < 0.3 m/s for ≥ 2 s, with raw displacement
  across each. Jitter is a Welch spectrum (Hann, 64 samples, 50% overlap) of the z, pitch and
  roll residuals about the fit, reported with the fit's residual gain |1 − H(f)| and a
  gain-corrected peak amplitude. End anchors are checked by averaging the first or last 5 s.
  Gaps over three median intervals split the fits. A test found that the residual over-reads a
  2 Hz sine by about 24%, because the cubic's response is negative there. A 0 → 8 m/s step
  overshoots to 8.48 m/s.
- The route plan's trajectory contract: pose, velocity and quality per frame, plus placement,
  provenance, timing and calibration fields. A `PoseProvider` interface is specified but not
  implemented. The 4-state constant-velocity Kalman tracker and a fixed-lag RTS smoother
  experiment exist for other vehicles. A constant-acceleration model and an IMM are next on
  that plan.

**Gerardus (Rust, minimal dependencies, no GTSAM or Ceres):**

- `register.rs`: robust point-to-plane ICP (Huber 0.10 m, planarity weights, SE(3)
  left-perturbation), the 6×6 information matrix and its rank with rotations scaled at 10 m,
  hypothesis separation against along-street aliases (±3.7 m and ±7.6 m lot periods), and the
  static gates: held-out ground p95 ≤ 0.20 m, facade p75 ≤ 0.20 m, facade range slope ≤ tan 0.5°.
- `graph.rs`: a small SE(3) Gauss–Newton/LM pose-graph solver (`solve(initial, factors, iters)`),
  used today to refine several static scans at one site together.
- The mobile plan (S0–S3) and its maths note (§4) cover constant-velocity de-skew residue,
  KISS-ICP structure on the existing ICP, a voxel-hash map with running moments, submaps every
  ~50 m, prior factors with weak eigen-directions projected out, drift budgets and compute. They
  were written for a walked loop, not car drives.

## 4. Questions

Answer every question. For each one, give the minimum you recommend, what you would defer, and
the observable that would tell the team the minimum was not enough.

### A. Observability of a two-anchor bridge

1. With a static pose at each end (σ_A, σ_B) and odometry with per-metre drift rate d, derive the
   along-street, across-street and yaw uncertainty at fraction s of a drive of length L. Do it
   (i) with A only and (ii) with A and B, and then with prior factors every 50 m that constrain
   only across-street translation and yaw mid-block, and all six directions at intersections. At
   what L does the far anchor stop helping mid-route?
2. Replacing loop closure (Gerardus S2) with the far anchor: is that sound for a point-to-point
   drive? What does the A-only held-out residual at B actually test (drift, prior registration,
   anchor error, the static-to-motion handover), and how should those be separated?
3. Most `MANUALLY_PLACED` poses carry no σ. What σ should the solve assign them? Can the
   export check's margin (score at pose against best moved neighbour) be turned into one, and is
   it safe to use such an end as a held-out check at all?
4. Is the static pose valid for the drive's first moving rotation? The sensor sat on a tripod
   during the site capture and then went onto the car, or the car itself was the tripod. What
   minimal test, from the exported frames alone, shows whether the mount moved between the
   static capture and the start of the drive, and what does the solve do if it did?

### B. Timing and de-skew at car speed

5. At 0–15 m/s with turns at intersections, is constant-velocity de-skew from the previous
   odometry step good enough for the 0.20 m prior gates? Use the maths note's §4.1 residue
   formulae with car-plausible acceleration and steering rate. Where it is not, is a two-pose
   per-sweep (CT-ICP-style) model the minimum fix?
6. The azimuth clock fits one line per rotation. Should it fit across several rotations, or the
   whole segment with a slowly varying rate, to share strength? What residual or period
   behaviour should make a rotation's times untrusted, and what should the exporter do then?
7. The constant offset (about 250 µs, half a packet span) and the PCAP arrival latency cancel in
   scan-to-scan odometry. Where in (a) or (b) do they not cancel? Bound their effect on ego
   speed, and on a later radar-to-LiDAR speed match.
8. Is reporting time as session seconds, not GPS time, correct for the LAS export? What must
   `run.json` carry so a later run can re-time points without re-reading the captures?

### C. Minimum for (a): odometry, submaps and prior factors

9. Given `register.rs` and `graph.rs`, what exactly is the smallest new code: a voxel-hash map,
   an adaptive correspondence threshold, a relative-pose factor with which covariance, keyframe
   and submap rules at car speed? Which KISS-ICP elements can be dropped for a first drive?
10. ICP's information matrix is overconfident for odometry factors (correlated residuals, n_eff).
    Propose a calibration of odometry-factor covariance that the first drive can itself check,
    for example against the stops or the held-out anchor.
11. Prior factors: should a mid-block submap add a factor with weak directions projected out, or
    no factor? Choose the rank or eigenvalue threshold, and say how to keep an along-street
    alias (one lot period) from entering the graph.
12. Moving traffic in a car drive: is the robust kernel enough, or must dynamic points be
    removed before odometry? Can that be done without the background model, which does not work
    on a moving sensor?
13. Acceptance for (a) on the first drive: which gates per submap and per drive, which held-out
    quantities, and which minimum counts are needed for the result to mean something? Say what a
    pass would and would not establish about mobile registration in general.

### D. Minimum for (b): the ego track, speed, jerk and jitter

14. Is a local weighted cubic the right minimum estimator for speed, acceleration and jerk from
    10 Hz poses? Compare it with an RTS smoother on a constant-acceleration or constant-jerk
    model, and with a continuous-time spline over the pose graph. Should (b) share one smoother
    family with the project's tracker work so the uncertainty language matches?
15. Derive the jerk detectability floor: the σ_pos needed for |jerk| = 1 m/s³ to be resolved at
    3σ with a ±h s window at 10 Hz, and the h that minimises the error from bias plus noise for
    braking onsets typical of urban driving. Is the ±0.5 s default defensible?
16. The independent-noise assumption: odometry error is closer to a random walk. Derive what that
    does to the stated σ of speed, acceleration and jerk within the window. Propose the minimum
    correction: a different σ model, or σ taken from the pose-graph marginals.
17. Jitter: at 10 Hz the observable band is 0–5 Hz. Car body and mount vibration lie largely
    above it. What can honestly be called "jitter" from per-rotation poses? Would per-sector poses
    within a rotation (intra-sweep ICP) raise the band enough to matter, and at what cost? Is the
    gain-corrected peak amplitude a sound way to report a residual that has gone through
    |1 − H(f)|?
18. Stops: is displacement over a stop a valid drift check when a stopped car still sees moving
    traffic? Should zero-velocity intervals enter the pose graph as factors (a ZUPT), and if so,
    does using them there spoil them as a check?
19. Which quantities belong in the trajectory produced by Gerardus (velocity, its covariance)
    and which in velocity.report's kinematics? Keep the trajectory contract minimal and avoid
    differentiating twice.

### E. Evaluation and its traps

20. Without an IMU or GNSS of known quality, what independent references exist for (b)?
    Candidates are the held-out anchor, stop displacement, the A–B route length from DataSF
    street centrelines, and the later fixed-radar comparison. Rank them and say what each misses.
21. How can each success criterion in the evaluation spec pass while the trajectory is wrong?
    Propose one additional check per criterion that would catch it.
22. Is one drive with two registered ends enough to choose defaults (fit window, stop threshold,
    submap length)? If not, what is the smallest set of drives, and how should the seven
    one-registered-end drives be used?

### F. Sequencing

23. Give the order of work as a numbered list. For each item give the repository
    (velocity.report or Gerardus), its size (S, M or L), its inputs and outputs, and a
    kill-or-continue criterion. Mark anything you would cut if only a third of the effort were
    available.
24. Name any decision the project owner must take before implementation, with your default.

## 5. Output format

1. Restatement (at most ten bullets).
2. Answers A1–F24, each opening with a one-line recommendation, then the derivation or
   reasoning, tagged fact, derivation or judgement.
3. A table of proposed gates and defaults: name, value, units, domain, derivation reference.
4. The ordered work list (F23).
5. What you are least sure of, and the measurement that would settle it.

## 6. Appendices to attach

From velocity.report:

- `docs/plans/lidar-route-slam-minimum-plan.md`
- `docs/plans/lidar-route-capture-plan.md` (trajectory contract, rigs, privacy)
- `docs/lidar/architecture/portable-capture-timing.md` (timing audit and error budget)
- `data/experiments/try/route-drive-bridge-first-pair.md`
- `internal/lidar/motionexport/clock.go` and `internal/lidar/egotrack/kinematics.go`
- `tools/s2-archive/site-index.json` (site times, for the drive list)

From Gerardus:

- `docs/plans/MOBILE-SLAM-REPLAY.md`
- `docs/research/2026-10-24-registration-and-slam-maths.md` (§1 observability, §4 sizing)
- `docs/REGISTRATION-STATUS.md` §1 (accepted poses and σ)
- `src/register.rs` (`icp`, `information_rank`, gates) and `src/graph.rs` (`solve`)
