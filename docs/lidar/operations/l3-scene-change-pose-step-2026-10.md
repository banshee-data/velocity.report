# The Lombard–Laguna scene change was a sensor pose step, October 2026

The [scene-change proposal](../../../data/experiments/try/lombard-laguna-scene-change-recovery.md)
asks why 45 tracks start within 0.2 s near the end of the Lombard–Laguna placement, and its
[sprint plan](../../plans/lidar-scene-change-recovery-sprint-plan.md) makes the first decision
(S2) whether the raw evidence accepts or rejects a change of sensor pose. This run answers that
from the raw capture alone, with a robust rigid fit between the static background before and after
the event and the same fit on quiet windows as the null.

- **Status:** Measured, October 16, 2026 (experiment
  [l3bg0013](../../../data/experiments/l3bg/l3bg0013.md)). Nothing shipped changes; label-free
- **Layers:** L1 packets and L2 geometry (raw returns, before any background model)
- **Related:** [scene-change proposal](../../../data/experiments/try/lombard-laguna-scene-change-recovery.md),
  [sprint plan](../../plans/lidar-scene-change-recovery-sprint-plan.md),
  [dwell policy and the arrival regime](l3-dwell-policy-arrival-2026-10.md)

## Summary

- **The sensor moved.** Between the 10 s before the event and the 10 s after it the static
  background fits a rigid motion of 0.262° in yaw, a 0.094° tilt and 0.58 cm of translation. The
  largest of 18 quiet pairs fits 0.009° and 0.035 cm, so the event's rotation is about 30 times
  the null's largest and its yaw 130 times. The fit takes the median range change of the static
  cells from 2.0 cm to 0.35 cm and the share changed by more than 10 cm from 23.6% to 3.9%, in
  every 30° sector, including each sector held out of its own fit. The proposal's H1 holds; a
  model-state fault (H2) or local occlusion (H3) as the cause of the burst is rejected.
- **It happened in about 0.2 s and held.** The first changed rotation begins about 7 ms before
  the pipeline's first burst frame, overshoots in the next, and settles from +0.2 s. Every 5 s
  window from +1 to +115 s fits the same pose (yaw 0.261 to 0.265°, tilt 0.091 to 0.100°), and
  windows within that stretch fit each other to the null.
- **A second pose change came at +121 s, and the sensor was picked up from +285 s.** At +121.2 s
  a 2 s change takes the tilt and translation back to the null and adds 0.07° of yaw, so the last
  pose before pickup is 0.332° of pure yaw from the first. From +285.3 s the static cells fall
  away: the sensor is being carried, 2.2 s before the site's indexed end. Neither change was
  recorded anywhere.
- **The event is at 34:00.1 into the placement, not 33:20.6.** 33:20.6 is the original web
  export's offset, which started 39.6 s after the placement's first packet; on the l3bg0008
  replay's own clock (which starts after its 70 s warm-up) the event is at 32:50.0.

## Method

- **Time.** The anchor is the first frame of the burst in the l3bg0008 shipped-L3 replay's
  evidence: 6 clusters at frame 1788373950223403330, 33 at the next, 1788373950323299330
  (2026-09-02T18:32:30.323Z), in `s2_sf_2_20260902112832_00025.pcap` 238.053 s after its first
  packet; that capture continues for 62.4 s and joins the next without a gap.
- **Cells.** `lidar-pose-step` decodes every return of a window through the pipeline's own
  Pandar40P parser into 40 rings by 0.2° azimuth bins and keeps each cell's median range, its
  interquartile range and its rotation coverage. A cell is static when it returns in at least 80%
  of the window's rotations with an interquartile range within max(8 cm, 0.4% of range), between
  2.5 and 120 m. Every window decoded completely: last-return mode, 600 rpm, about 1,800 packets a
  second.
- **Fit.** Cells static in both windows become points; normals come from the before-window's
  neighbouring cells; a six-degree small-angle point-to-plane fit is iterated under a Huber then
  a Tukey loss with a 3σ trimmed inlier set. Standard errors are given formally and by a jackknife
  over 12 azimuth sectors (the formal ones ignore spatial correlation). The tool abstains when one
  plane dominates the normals, when the information matrix is ill-conditioned, or when support is
  confined to one sector. Its thresholds were fixed on synthetic scenes before any real window was
  read: identical windows, a known 0.3° yaw, 0.2° pitch and 5 cm step (recovered within 0.01° and
  0.5 cm), a car moved 1 m (near-null fit, residual in its two sectors), and the abstentions.
- **Pairs.** The event pair is 10 s ending 1 s before the anchor against 10 s starting 1 s after
  it, with 6 s and 30 s versions, after-windows at +60 and +285 s, and 5 s tracking windows from
  +1 to +280 s against the same before-window. The null is 18 quiet pairs of the same shape (14
  before the event, 4 after +62 s), and slow drift is six pairs 5 to 30 minutes apart.

## Findings

| Pair                                 |    rx° |    ry° |   rz° | t (cm)               | Median \|Δr\| (cm) | Changed > 10 cm | Inliers |
| ------------------------------------ | -----: | -----: | ----: | -------------------- | ------------------ | --------------- | ------: |
| Event, [−11, −1] s against [+1, +11] | −0.073 | −0.059 | 0.262 | (−0.25, 0.44, 0.28)  | 2.0 → 0.35         | 23.6% → 3.9%    |     91% |
| Event, 6 s after                     | −0.073 | −0.059 | 0.261 | (−0.27, 0.59, 0.27)  | 2.0 → 0.35         | 22.4% → 3.4%    |     92% |
| Event, 30 s windows                  | −0.071 | −0.057 | 0.260 | (−0.24, 0.51, 0.28)  | 2.4 → 0.36         | 26.9% → 5.9%    |     90% |
| Before against [+60, +70]            | −0.074 | −0.067 | 0.261 | (−0.29, 0.47, 0.27)  | 2.6 → 0.42         | 29.4% → 13.6%   |     89% |
| Before against [+275, +285]          |  0.008 | −0.000 | 0.332 | (−0.03, 0.04, −0.02) | 1.8 → 0.37         | 17.2% → 6.0%    |     93% |
| [+105, +115] against [+127, +137]    |  0.083 |  0.066 | 0.066 | (0.24, −0.10, −0.28) | 0.8 → 0.29         | 14.3% → 2.3%    |       — |

Jackknife standard errors on the event pair are 0.002 to 0.004° in rotation and 0.03 to 0.28 cm
in translation (y, the axis the scene constrains least). The fit's support spreads over 8 of the
12 sectors with at least 5% each, the largest 18%; a wall about 5 m away dominates the normals at
0.63, below the abstention threshold, and the condition ratio is 0.029 against a floor of 0.001.

The null and drift distributions (minimum, median, maximum):

| Pairs                     | \|rotation\| (°)         | \|yaw\| (°)              | \|t\| (cm)            | Changed > 10 cm |
| ------------------------- | ------------------------ | ------------------------ | --------------------- | --------------- |
| 18 quiet 10 s pairs       | 0.0000 / 0.0007 / 0.0093 | 0.0000 / 0.0001 / 0.0020 | 0.005 / 0.014 / 0.035 | 0 to 0.8%       |
| 6 pairs 5 to 30 min apart | 0.003 / 0.005 / 0.008    | 0.0004 / 0.0014 / 0.0028 | 0.03 / 0.05 / 0.07    | 0.6 to 6.0%     |

Median range change of the static cells per 30° sector, event pair, before the fit and after it
(the after-fit figure is the same with that sector held out to within 0.1 cm):

| Sector (°) | 0–30 | 30–60 | 60–90 | 90–120 | 120–150 | 150–180 | 180–210 | 210–240 | 240–270 | 270–300 | 300–330 | 330–360 |
| ---------- | ---: | ----: | ----: | -----: | ------: | ------: | ------: | ------: | ------: | ------: | ------: | ------: |
| Before     |  9.2 |  22.0 |   1.6 |   16.4 |    17.2 |    10.8 |     6.4 |     1.6 |     0.4 |     0.8 |     4.4 |    11.8 |
| After      |  0.8 |   0.8 |   0.3 |    1.2 |     1.0 |     0.6 |     0.7 |     0.2 |     0.2 |     0.2 |     0.5 |     0.9 |

The sectors at 210 to 300° change least because a yaw slides the surfaces they see face-on along
themselves, yet they carry half the support. After the fit two local remnants stay, the upper
rings at 150 to 180° and the lower rings at 30 to 120°, which look like objects or vegetation, not
pose. The +60 s pair shows the occlusion signature beside an unchanged transform: 5 to 9 m
residuals at 90 to 150° from a large object standing near the sensor from about +31 s.

## Interpretation

A change of 0.26° in yaw with a tilt and a few millimetres of translation, completed within two
rotations and then held, is a physical knock to the mount. No decoding or azimuth offset produces
a tilt and a translation, and return mode, rotation rate and packet rate are unchanged across it.
The horizontal translation is what a tilt about a point about 2.3 m below the head, near the
ground, would produce (predicted −0.24, +0.29 cm against −0.25, +0.44 ± 0.28 cm); the 0.28 cm
vertical shift is not explained by that.

So the burst is the background model doing what it should with a scene that moved: every static
surface now disagrees with the learned grid by a range that grows with distance and incidence, and
the grid relearns at its update rate. The second change at +121 s, which removes the tilt and adds
yaw, reads like someone straightening the sensor; the data cannot say who. A pure yaw on its own
could not be told from an azimuth offset by geometry; the tilt and translation of the intermediate
pose are what make the first change unambiguously physical.

For the dwell report, the replay's last five minutes are not one event's tail: they hold the knock
at 32:50 of its uptime, the second change at 34:51 and, from 37:35, about 77 s of the sensor being
carried, past the site's indexed end. The pipeline's 26 and 22 clusters a frame at +280 and +340 s
sit just before and inside the pickup.

## What this does not establish

- **Physical truth.** There is no operator log for either change; one placement, one sensor.
- **Calibration.** Both windows share the embedded generic calibration, so it cancels in the
  comparison but not in absolute geometry.
- **The near field.** Cells nearer than 2.5 m are excluded; whether the large object moving at 90
  to 180° in the last second before the step touched the mount is not established.
- **A response.** This measures the cause, not a recovery; how the pipeline should respond (a pose
  correction, a new background epoch) is the sprint's S3.

## Recommendations

- **Plan the sprint's S3 as a pose response**, a detected step followed by either a measured
  correction applied to the grid or a bounded rebaseline, rather than a search for a model fault.
  The step is large enough to detect from the raw rotations in well under a second, which the
  tool's per-rotation series already shows.
- **Treat Lombard–Laguna's index as ending before +285 s** for any replay of the placement, or
  trim the captures at the site's indexed end: the last 77 s are the sensor in someone's hands.
- **Correct the dwell report's last-bin reading**, as it now notes, and read any other long replay
  of this placement with both pose changes in mind.
- **Run the same fit as a corpus check**: quiet pairs at every site would show any other unrecorded
  knocks; against this placement's null (at most 0.009°), a change of a few hundredths of a degree
  stands out.

## Provenance

- **Code:** `lidar-pose-step` written by an agent as 27bebaac4 on `agent/pose-step` (from
  1f376de58) and cherry-picked as fed1ef684 on `claude/experiment-followup`; the evidence binary
  built stamped (0.5.1-pre45, fed1ef684). The agent's own run at 27bebaac4 and this one agree in
  every figure.
- **Captures:** the Lombard–Laguna placement, `s2_sf_2_20260902105830_00019.pcap` to
  `s2_sf_2_20260902113332_00026.pcap` under the LiDAR volume's `pcaps/s2`, resolved through
  `tools/s2-archive/site-index.json`.
- **Anchor evidence:** `/Volumes/lidar/lidar/work/velocity-campaign/overnight-20261013/l3bg0008/replay/a0/lombard-laguna/ev/observations.db`.
- **Outputs:** the fit JSON and Markdown for the four runs (event and null pairs, locating the
  second step, the second step, the end of the placement) kept locally, outside the repository.
- **Run:** 6 min 57 s for all four.
