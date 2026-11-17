# First route exports on hydra: the azimuth clock on three archive drives

The first real run of `lidar-motion-export` on archive drives found a defect in the azimuth clock,
and the fix it led to brought the clock's residual down to the floor that the missing per-block
offsets set. This report records what was measured, against which criteria, and what the run does
not yet establish.

- **Status:** Measured on hydra by the owner, 2026-11-17; partial against the evaluation spec.
  Only the export and its timing (spec steps 1–3) have run. The clock gate was tightened after this
  run (`866b9d4`), so the post-fix figures need a re-export on the current head
- **Spec:** [route-drive-bridge-first-pair](../../../data/experiments/try/route-drive-bridge-first-pair.md)
- **Plan:** [route SLAM phase 1](../../plans/lidar-route-slam-phase1-bridge-ego-plan.md), item 1
  (export trust) and item 9 (evaluation 1)
- **Scope:** point timing of the motion export only; no odometry, trajectory or ego track exists yet

## Question

Does the exporter's per-rotation azimuth clock give every rotation of a real archive drive a usable
time base for de-skew? The criterion is spec criterion 1: at least 99% of rotations with
`clock_ok`, and no rotation in motion off the median period without a packet gap to explain it.

## Method

`lidar-motion-export` ran at `-azimuth-stride 4` with `-time-source azimuth_clock_v1`. Its inputs
were the three drives phase 1 names, staged from the NAS onto hydra's local volume.

| Drive                                 | Ends registered | Gap between site bounds | Captures                      | Role in phase 1 |
| ------------------------------------- | --------------- | ----------------------- | ----------------------------- | --------------- |
| lombard-laguna → columbus-north-point | both            | 16.3 min                | `s2_sf_2_…_00026` to `_00030` | untuned test    |
| marina-webster-beach → lombard-laguna | one (end)       | 10.0 min                | `s2_sf_2_…_00016` to `_00019` | development     |
| broadway-franklin → laguna-eddy       | one (end)       | 17.5 min                | `s2-1_…_00033` to `_00036`    | development     |

The gaps and capture ranges come from `tools/s2-archive/site-index.json`. The exports produced
28,050 rotations in all. At 10 Hz that is about 2,805 s, which matches the three gaps (2,628 s)
plus the spec's 30 s margin at each end (180 s): 9.99 rotations per second. This is a derived
consistency check that assumes those margins were used, not a measured rate; the per-drive
`rotation_hz` in each `run.json` is the measurement.

The run was repeated after the clock fix in `7cfe581`. The earlier build is the branch head before
that commit; the exact SHA was not recorded with the figures.

## Findings

### Capture integrity

Every capture join graded clean. No rotation was incomplete, and there were no packet gaps.

### The seam defect

The first-pass clock ordered a rotation's blocks by their mean point time before unwrapping
azimuth. A block's mean depends on which channels returned, so within a packet the order
shuffled. Where a packet straddled the 0°/360° seam, some blocks unwrapped a whole turn off. Their
points were timed one to three rotations early, and the robust pass dropped them from the fit
without the gate noticing.

| Measure                                  | First pass                                              | Unwrapped in firing order (`7cfe581`) |
| ---------------------------------------- | ------------------------------------------------------- | ------------------------------------- |
| Points mistimed                          | 0.7%, in 89% of rotations                               | none found                            |
| Median clock residual                    | 9.4–18 ms, with maxima at whole multiples of the period | 176–188 µs                            |
| Rotations more than 2% off median period | 447, 752 and 2,382 across the three drives              | one                                   |
| Period standard deviation                | not reported                                            | 84–100 µs                             |
| `clock_ok`                               | not reported                                            | 100% on all three drives              |

The ranges span the three drives. The review did not attribute each before-figure to a drive.
`TestAzimuthClockUnwrapsTheSeamWhenChannelsDropOut` reproduces the defect on synthetic data with
channels dropped per block. It fails on the old ordering with a 717.8° unwrapped span.

### The remaining period outlier

One rotation, frame 93 of lombard-laguna → columbus-north-point, has a period of 0.1059 s, 5.9%
over the median, with no packet gap. Under spec criterion 1 that is a strict fail on the test
drive. Its cause is not yet known. Either the rotor slowed for one rotation, or a burst of late
packets tilted that rotation's fit.

## Against the spec's criteria

| Criterion                                                                    | State                                                                                      |
| ---------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------ |
| 1. ≥ 99% of rotations `clock_ok`; no unexplained period outlier in motion    | 100% `clock_ok`; **fails strictly** on frame 93 of the test drive, and needs the re-export |
| 2. Held-out end residual within 2% of distance                               | not testable: no trajectory                                                                |
| 3. Stop displacement under 0.3 m                                             | not testable: no trajectory                                                                |
| 4. Ego-track distance within 1% of path length; median speed σ under 0.1 m/s | not testable: no trajectory                                                                |
| 5. Jitter spectrum reported with its Nyquist limit and gain                  | not testable: no trajectory                                                                |
| 6. 5 Hz ablation beside the native arm                                       | not run                                                                                    |

Phase 1 replaces criteria 2–4 with the reconciled gates of the
[oracle comparison](../../research/lidar-route-slam-oracle-comparison.md). Both oracle runs showed
those criteria could pass with a wrong trajectory.

## Interpretation

After the fix, the median residual of 176–188 µs sits just above the roughly 160 µs RMS that the
missing per-block offsets alone put under a rotation. So the scatter that remains is about what
the block sawtooth predicts, plus a little arrival jitter. That is consistent with the clock, not
the packets, carrying the timing.

It is not a measured timing accuracy. The residual is the scatter of the recorded packet times
about each rotation's line, and both are on the PCAP arrival clock. A constant offset of about half
a packet's block span remains, plus an arrival latency nobody has measured.

The defect matters beyond its 0.7% of points. Points timed a rotation early would be de-skewed with
the wrong pose, and nothing downstream would have flagged them. Neither oracle run caught the
defect, though both read the clock's code. Only real captures, with channels dropping out, exposed
it.

## What this does not establish

- **Nothing about registration or the ego track.** No odometry, trajectory or kinematics has run.
- **Not the current gate.** `866b9d4`, which followed the owner's review of this run, changed the
  clock fit:
  - the robust pass now repeats until its kept set settles;
  - a fit is refused above 10% rejected blocks or above 500 µs kept-block RMS;
  - residuals now cover kept blocks only.

  The 100% `clock_ok` and the 176–188 µs median were measured before that, and need the re-export.

- **Not timing at 20 Hz.** These drives are 10 Hz (derived above). The only 20 Hz capture in the
  repository is a sector capture that never completes a rotation.
- **Not the time-source comparison.** The two 60 s full-density exports that compare
  `azimuth_clock_v1` with `packet_firetime` were made before the fix, and are void.

## Provenance

| Item                        | Value                                                                                |
| --------------------------- | ------------------------------------------------------------------------------------ |
| Measured by, where, when    | owner, hydra, 2026-11-17                                                             |
| Captures                    | as listed under Method, from the NAS, staged locally                                 |
| Exporter build (after fix)  | `7cfe58177412eed886f0be1b0ed5cfc82b48049c`                                           |
| Exporter build (first pass) | the branch head before `7cfe581`; not recorded                                       |
| Flags                       | `-azimuth-stride 4`, `-time-source azimuth_clock_v1`; window as in the spec, assumed |
| Capture and output digests  | in each export's `run.json` on hydra; not reproduced here                            |
| Raw outputs                 | LAS segments, `run.json`, `frames.csv` on hydra's local volume; none in Git          |
| Source of the figures       | the owner's review of #1056 and the commit message of `7cfe581`                      |

## Next steps

1. Re-export the three drives on the current head. Record `rotation_hz`, `join_worst`, the share
   of `clock_ok` under the new gate and the largest rejected residual. Re-read frame 93 from its
   rejected blocks and, once phase 1 item 1 records it, its packet-tail motor speed.
2. Redo the two 60 s time-source comparisons on the current head.
3. Add the per-drive rows of both runs to this report.
