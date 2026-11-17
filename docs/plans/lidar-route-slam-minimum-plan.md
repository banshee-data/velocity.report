# Route SLAM minimum: priors registration along a drive and the ego track (v0.6.0 spike)

The smallest work that turns the archive's drives between registered static sites into two
results: (a) the drive registered against Gerardus's public priors along its route, and (b) the
sensor platform's own track, with its speed, acceleration, jerk and vibration and the
uncertainty of each. This plan fixes the boundary, records what the first pull request
implements, and lists the questions an independent mathematical review must settle before
the hydra evaluation is read as evidence.

- **Status:** Active. First-pass tools are in #1056. Two independent oracle runs answered the brief, and the [comparison](../research/lidar-route-slam-oracle-comparison.md) reconciles them into the two phase plans below. The owner pinned the gates and decisions on 2026-11-17. The [first exports](../lidar/operations/route-slam-first-exports-2026-11.md) ran on hydra, and a re-export under the tightened clock gate is pending
- **Decisions:** D-36 (Gerardus owns registration; v0.6.0–v0.6.4 is the SLAM/mobile series), D-38 (priors are measured spatial data)
- **Layers:** offline tooling over L1 decode and L2 rotations (`internal/lidar/motionexport`), offline platform kinematics (`internal/lidar/egotrack`); Gerardus odometry, prior factors and pose graph
- **Target:** v0.6.0, the SLAM spike Gerardus runs as S0–S3; this plan narrows it to point-to-point drives
- **Companion plans:** [lidar-route-capture-plan](lidar-route-capture-plan.md) (trajectory contract, rigs, later items), [lidar-motion-capture-architecture-plan](lidar-motion-capture-architecture-plan.md) (`PoseProvider`); Gerardus `docs/plans/MOBILE-SLAM-REPLAY.md` (S0–S3) and `docs/research/2026-10-24-registration-and-slam-maths.md` (§4 sizing)
- **Canonical:** [motion-capture.md](../lidar/operations/motion-capture.md)
- **Phase plans:** [phase 1: bridge and ego track](lidar-route-slam-phase1-bridge-ego-plan.md), [phase 2: priors and rotation rate](lidar-route-slam-phase2-priors-rate-plan.md)
- **Review brief:** [lidar-route-slam-oracle-brief](../research/lidar-route-slam-oracle-brief.md), answered by runs [A](../research/lidar-route-slam-oracle-response-fable.md) and [B](../research/lidar-route-slam-oracle-response-opus.md) and [compared](../research/lidar-route-slam-oracle-comparison.md)
- **Rate audit:** [20 Hz pipeline readiness](../research/lidar-20hz-readiness.md)

## Motivation

Gerardus's mobile mapping plan assumes a new capture day: a walked block loop that starts and
ends at one registered corner, so loop closure fixes the drift. That capture does not exist.
What does exist is three days of car drives, 1–3 September 2026, recorded by the same
Pandar40P between the 24 tripod sites. Each site now has an accepted sensor pose (Gerardus
`docs/REGISTRATION-STATUS.md` §1). So every drive between two consecutive sites is a trajectory
whose two ends are already known: a bridge between two anchors, not a loop. `pcap-split` cuts
these drives off as motion and nothing reads them.

A bridge needs no loop closure. Its far-end anchor is an independent check when the solve
leaves it out, and the stops at traffic lights give zero-velocity intervals that expose drift
for free. The minimum work is therefore smaller than S0–S3: export, odometry, prior factors
and a two-anchor solve, then a kinematics pass that states its own error bars. If this
does not work on car drives with two registered ends, a walked loop will not rescue the
along-street observability problem either, and the spike learns that early.

## Current state

- **Anchors.** Six sites are `REGISTERED` with stated σ of 2–19 mm and 0.004–0.024° heading.
  Eighteen are `MANUALLY_PLACED`, and most of those carry no covariance. Five have a narrow
  export-check margin (1st-mission, hyde-ofarrell, pierce-haight, marina-broderick,
  embarcadero-bay).
- **Drives.** `tools/s2-archive/site-index.json` gives 21 same-day consecutive pairs: 20 of
  5–34 min and one of 82 min. One pair has both ends `REGISTERED`:
  lombard-laguna → columbus-north-point, 16.3 min, inside one rotation series
  (`s2_sf_2_…_00026` to `_00030`). Eight more have one registered end (seven without the 82 min
  pair). Several gaps are exactly 5 or 10 min, so the site bounds follow the capture files'
  rotation, not the moment the car moved.
- **Point time.** The recorded point time is the PCAP arrival time plus the channel firetime
  (`parse/extract.go`, `blockToPoints`), and block index contributes nothing
  ([timing audit](../lidar/architecture/portable-capture-timing.md#what-the-repository-does-today)).
  On the perf capture `lidar_20Hz.pcapng`, recorded block times scattered 252 µs RMS (4.9 ms
  maximum) about a straight line in rotor azimuth (this change's exporter, first 3 s).
- **Pose.** No `PoseProvider`, no per-frame pose rows, no trajectory reader in the pipeline.
- **Gerardus.** Robust point-to-plane ICP with the static gates (`register.rs`), the 6×6
  information and its rank, and an SE(3) Gauss–Newton pose graph (`graph.rs`, used by
  `RefineSite`). It has no odometry map, no de-skew and no `slam` subcommands yet.

## What this change implements

| Piece              | Where                                                          | What it does                                                                                                                                                                                                                                                                   | Tested by                                                                                                                                                                                               |
| ------------------ | -------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Motion export (S0) | `internal/lidar/motionexport`, `cmd/tools/lidar-motion-export` | L1 decode and L2 rotations only, over an ordered capture list graded at each join; sensor-frame LAS 1.4 (point format 6, extra bytes `ring`, `azimuth_deg`, `frame`), split by capture time; `run.json` and `frames.csv`                                                       | Synthetic rotations with arrival jitter and the missing block offset; header layout; independent read with `laspy` on real packets                                                                      |
| Azimuth clock      | `motionexport.PointTimes`                                      | Fits each rotation's block times to a line in unwrapped block azimuth, keeps each point's channel offset, and reports the fit's period and residual                                                                                                                            | Synthetic: error under 20 µs RMS against 300 µs arrival jitter; refuses partial rotations                                                                                                               |
| Ego track (b)      | `internal/lidar/egotrack`, `cmd/tools/lidar-ego-track`         | Reads a provisional trajectory file; local weighted cubic fits with exact covariance give speed, longitudinal and lateral acceleration, jerk and yaw rate with 1σ; stops; Welch spectrum of the vibration the path does not explain, with the fit's gain; end-anchor residuals | Synthetic drive: values within 4σ, jerk integral across each step equals the change in acceleration, stated σ matches the empirical scatter within 0.75–1.3, vibration peak and amplitude, gaps refused |
| Build targets      | `Makefile`                                                     | `make build-lidar-motion-export`, `make build-lidar-ego-track` with the build stamp                                                                                                                                                                                            | —                                                                                                                                                                                                       |

### Choices made, and why

- **Time is session seconds, not GPS time.** The LAS time field holds seconds since the
  manifest's `epoch_utc`. The replay clock is PCAP arrival time, and calling it GPS time would
  assert an epoch nothing has validated. LAS 1.4 has no flag for an arbitrary epoch (a clear
  time-type bit reads as GPS week time), so `run.json` is the only authority and a reader must
  refuse the files without it. Gerardus de-skew needs only relative time.
- **The rotor is the clock.** The Gerardus maths note (§4.1) recommends `τ = azimuth / 360°`
  over the block index. A per-rotation line removes arrival jitter and the block sawtooth
  together. It does not touch the L1 decode path, so the perf gate is unaffected. What remains
  is a constant offset of about half a packet's block span (about 250 µs), because the recorded
  time names the packet, not the block. A constant offset cancels between consecutive scans. It
  does not cancel against another sensor's clock (the radar comparison).
- **No perception in the export.** Running the export as a frame tap on the replay pipeline
  loses every rotation before L3 settles (about 30 s) and pays for L3–L6. On the 100 MB perf
  capture it also ran out of memory in the cloud container. The exporter therefore drives
  `ReadPCAPSequence` and an offline frame builder directly, and needs no tuning config.
- **Uncompressed LAS.** No Go LAZ dependency is added. A full rotation is about 60–70 k returns
  at 39 bytes, roughly 25 MB a second. The first drive (16 min) is about 23 GB at full density,
  or about 6 GB with `-azimuth-stride 4`. Compress on hydra with `laszip` or PDAL if needed.
- **Noise model in the ego track.** σ assumes independent per-pose noise. Correlated odometry
  error (drift, a slow yaw bias) biases the derivatives without widening σ. That is what the
  held-out far anchor and the stop displacement test. The summary says so in its notes.
- **The trajectory schema is provisional.** `velocity.report/ego-trajectory/v0` is the subset of
  the route plan's [trajectory contract](lidar-route-capture-plan.md#trajectory-contract) the
  ego track needs. It is replaced by Gerardus's proto once that exists, and the reader refuses
  any other schema.

## Two phases

Both oracle runs reverse the order this plan first assumed, and the comparison keeps run A's
boundary. The
ego track (b) comes first, because it does not need priors registration, while priors
registration (a) needs everything the ego track needs. A position error that varies slowly along
the route barely changes speed and does not change acceleration, jerk or vibration at all.

| Phase                                            | Delivers                                                                                | Needs                                           | Ends in                                           |
| ------------------------------------------------ | --------------------------------------------------------------------------------------- | ----------------------------------------------- | ------------------------------------------------- |
| [1](lidar-route-slam-phase1-bridge-ego-plan.md)  | Odometry bridged between the two end poses; the ego track with checks independent of it | Archive drives only                             | Hydra evaluation 1 and gate 1, on four drives     |
| [2](lidar-route-slam-phase2-priors-rate-plan.md) | Prior factors along the route with an alias guard; the 10-against-20 Hz answer          | Gate 1; a native 20 Hz capture for the rate arm | Hydra evaluation 2 and gate 2 (priors, then rate) |

The phase plans hold the items, gates, risks and owner decisions. Both runs found that the
first-pass tools' checks are not yet independent of the trajectory they test: stops come from
the trajectory's own speed, and σ assumes independent noise. The owner's roof-mount answer adds
a third problem: the tripod moves from ground to roof at each site, so the anchor has to be
carried across that step. Phase 1 fixes all three before any evaluation result is read as
evidence.

The design and tables below record the plan as first written, before run A. Where they differ
from the phase plans, the phase plans govern.

## Design: the two-anchor bridge

```text
hydra: archive pcaps (ordered, joins graded)
  -> lidar-motion-export             LAS segments + run.json + frames.csv   (velocity.report)
  -> slam import / odometry          de-skew, KISS-ICP-style local map      (Gerardus)
  -> submaps every ~50 m             EvaluatePose(refine) vs the site bundle (3DEP, DataSF)
  -> pose graph                      odometry + prior factors (weak directions projected out)
                                     + static_pose anchor A, + anchor B unless held out
  -> trajectory (EPSG:7131)          per-submap gates, anchor residuals
  -> lidar-ego-track                 speed, accel, jerk, yaw rate, stops, jitter, ±σ  (velocity.report)
```

Two solves per drive: **A-only** (B held out: the honest check of drift and prior registration)
and **A+B** (the product trajectory). For the first pair both ends are `REGISTERED`, so both
solves are possible. With a `MANUALLY_PLACED` end, the held-out comparison needs an assumed σ
for that end, and the review brief asks what σ that should be.

### (a) Minimum for priors registration along the route

| #   | Work                                                                                                                       | Repo                             | Size | State       |
| --- | -------------------------------------------------------------------------------------------------------------------------- | -------------------------------- | ---- | ----------- |
| a1  | Motion export with per-point time and manifest                                                                             | velocity.report                  | M    | this change |
| a2  | Anchors file: A and B `static_pose` from the accepted registration records, in the bundle's CRS                            | Gerardus (reads its own records) | S    | open        |
| a3  | Odometry: voxel-hash local map with running moments, adaptive threshold, constant-velocity de-skew from the exported times | Gerardus                         | M    | open        |
| a4  | Submaps and prior factors with the existing `EvaluatePose(refine)` gates and rank projection                               | Gerardus                         | M    | open        |
| a5  | Bridge solve in `graph.rs` with relative-pose odometry factors and anchor factors; A-only and A+B                          | Gerardus                         | S–M  | open        |
| a6  | Trajectory written in `velocity.report/ego-trajectory/v0` (or a converter), frame EPSG:7131                                | Gerardus                         | S    | open        |

Loop closure (S2), IMU, world-frame foreground and map building (S4+) are not needed for (a).

### (b) Minimum for the ego track

| #   | Work                                                                                                                | Repo            | Size | State       |
| --- | ------------------------------------------------------------------------------------------------------------------- | --------------- | ---- | ----------- |
| b1  | Kinematics with stated σ, stops, jitter, anchor residuals                                                           | velocity.report | M    | this change |
| b2  | File-backed pose lookup with gap refusal (`Trajectory.PoseAt`)                                                      | velocity.report | S    | this change |
| b3  | A trajectory to read: a6 above                                                                                      | Gerardus        | —    | open        |
| b4  | Independent checks on hydra: held-out anchor B, stop displacement, distance against the registered A–B route length | evaluation      | S    | open        |

## Rotation rate: running SLAM at 10 Hz and 20 Hz

The Pandar40P runs at 10 Hz (0.2° steps, about 72,000 points per rotation) or 20 Hz (0.4°, about
36,000), with the same points per second. It has no 5 Hz mode. The
[20 Hz readiness audit](../research/lidar-20hz-readiness.md) found no 5 Hz throttle in the
pipeline; "5 Hz" in the docs is a historical effective observation rate and the scene exports'
one-in-two stride. Run A agreed with the proposals below (one configuration in seconds and
metres, every rotation registered on its own) and added a platform view, which
[phase 2](lidar-route-slam-phase2-priors-rate-plan.md) carries: little gain for a car at 20 Hz
once de-skew is centred, and a fourfold cut in rotational residue for a bike or a walker. All
of it is untested until a native 20 Hz capture exists.

- **One configuration for both rates, fixed in seconds and metres.** Keyframes and submaps by
  distance and turn angle, the motion model and de-skew by point time, odometry-factor covariance
  per metre or second, and ego-track windows and the spectrum segment in seconds. Nothing in the
  SLAM path counts rotations. The tools in this change already do this: the exporter measures
  `rotation_hz`, and the ego track's windows and spectrum segment are in seconds.
- **At 10 Hz (the archive):** register every rotation against the local map. De-skew matters
  most here, because a 100 ms sweep at 15 m/s spans 1.5 m. The first drive runs at native
  10 Hz, and again with `-keep-every-rotation 2` as the 5 Hz temporal ablation, to measure how
  odometry and the ego track degrade as inter-scan motion doubles at unchanged angular density.
- **At 20 Hz (a future capture):** register each 50 ms rotation against the local map on its
  own. Scan-to-map registration keeps the map dense while each scan is half as dense, de-skew
  smear halves and the constant-velocity residue quarters. Prior factors come from submaps, so
  they see the same points per second at either rate. The ego track gains a 0–10 Hz vibration
  band (0–5 Hz at 10 Hz). Speed σ over a fixed window stays about the same (synthetic: within
  0.8–1.25 with √2 noisier poses), so 20 Hz is not an automatic gain in acceleration or jerk.
- **Comparisons:** native 20 Hz, the same drive decimated to 10 Hz, and native 10 Hz on the same
  route. Decimation is temporal only: it does not reproduce the other rate's angular density, and
  replaying a 10 Hz capture faster is not a 20 Hz experiment.
- **Known 20 Hz gaps:**
  - The repository's 20 Hz fixture is a 60–290° sector capture, which never completes a rotation
    in the frame builder (it needs 340°), so it cannot exercise this path.
  - A full-rotation 20 Hz capture is needed.
  - The audit's 10 Hz hard-coding (`SpatialCoverage`, `MaxOcclusionFrames`) does not touch the
    SLAM path, but it matters once route tracking reuses those metrics.

## Evaluation on hydra

The first export is specified in [route-drive-bridge-first-pair](../../data/experiments/try/route-drive-bridge-first-pair.md);
it is the opening step of phase 1's evaluation 1, which adds two development drives and holds
this drive back as the untuned test.
Its outputs (LAS, trajectory, kinematics) are raw evidence and operator location history.
They stay on hydra's local volume, and only the authored report with its summary tables comes
back to Git.

## Out of scope for the minimum

- Correcting point time inside L1 (block offset, native time precedence). It is a separate
  change that recaptures the perf baselines. The azimuth clock makes it unnecessary for the
  spike.
- Loop closure, Scan Context, the walked capture day, IMU and GNSS sidecars.
- World-anchored foreground and tracking other vehicles from the moving rig (route plan Items
  5 and 12).
- Publishing anything derived from a trajectory. The route plan's privacy rule holds: the
  trajectory is the operator's location history.

## Risks

| Risk                                                              | Likelihood | Impact | Mitigation                                                                                      |
| ----------------------------------------------------------------- | ---------- | ------ | ----------------------------------------------------------------------------------------------- |
| Drive files do not join (gaps between rotations)                  | Medium     | High   | The exporter refuses a broken join; pick a pair inside one series                               |
| Car-speed de-skew residue exceeds the 0.20 m prior gates in turns | Medium     | Medium | Exported times allow CT-ICP-style per-sweep poses later; report gates per submap by yaw rate    |
| Along-street priors weak mid-block, so A-only drift dominates     | High       | Medium | Expected; the held-out B residual measures it; add `feature` anchors at intersections if needed |
| Traffic (moving vehicles) biases odometry                         | Medium     | Medium | Robust kernel; compare residuals at stops with and without traffic                              |
| Ego jerk is dominated by pose noise at 10 Hz                      | High       | Medium | σ is stated per sample; the review brief asks for the detectability floor                       |
| `MANUALLY_PLACED` ends carry no σ                                 | High       | Low    | Use only both-registered pairs for held-out claims; treat others as qualitative                 |

## Checklist

### Complete

- [x] Motion export with azimuth clock, LAS 1.4 and manifest (a1)
- [x] Ego-track kinematics, stops, jitter, anchors and pose lookup (b1, b2)
- [x] Oracle brief for the Gerardus implementation questions
- [x] Oracle runs A and B, their comparison, and the two phase plans reconciled from it
- [x] Gates and owner decisions pinned at the reconciled defaults (2026-11-17)
- [x] First exports on hydra and their [report](../lidar/operations/route-slam-first-exports-2026-11.md)

### Outstanding

- [ ] [Phase 1](lidar-route-slam-phase1-bridge-ego-plan.md): items 1–9 and gate 1 (`L`)
- [ ] [Phase 2](lidar-route-slam-phase2-priors-rate-plan.md): items 10–18 and gate 2 (`L`)
