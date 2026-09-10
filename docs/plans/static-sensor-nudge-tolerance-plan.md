# Static-sensor nudge tolerance (v0.6.x)

- **Status:** Draft — test designed, not run
- **Layers:** LiDAR pipeline (L3 background model), `pcapsplit`
- **Canonical:** [pcap-analysis-mode.md](../lidar/operations/pcap-analysis-mode.md) for how a capture is classified
- **Companion:** [motion-static-parameter-tuning-plan](motion-static-parameter-tuning-plan.md) owns the wider sweep

## What happened

The 9/2 recording at van Ness and Sacramento produced no site. The classifier
read 21 minutes as five segments — 2.6 min static, 7.4 min motion, 2.4 static,
2.6 motion, 1.1 static — so no static stretch reached the ten-minute minimum
and the junction is absent from the archive.

The operator's account is that the tripod stood on a camping chair and was
nudged a few times. So the sensor did move, several times, by a small amount,
and then stood still again. That is not the platform driving between junctions,
which is what "motion" is supposed to mean here, and it is not noise either.

This matters beyond one junction. A tripod that can be nudged is the normal
case for this kind of survey, and a classifier that discards twenty minutes
because the sensor was bumped three times will keep discarding them.

## The question

Three things, and they are separable:

1. **Should a nudge read as motion at all?** A nudge displaces the sensor and
   then stops. A drive displaces it continuously for minutes. The distinction
   is available in the data — a nudge is a step, a drive is a ramp — and the
   current trigger does not use it.
2. **Would settling have recovered on its own?** After a nudge the background
   model is wrong by the size of the nudge. Whether it re-converges depends on
   how far the sensor moved against the model's tolerance. If it recovers in
   seconds, the segment is static with a blip; if it does not, the model has to
   be rebuilt and the segments really are separate.
3. **Does the movement help or hurt the scene?** A few centimetres of parallax
   between otherwise identical viewpoints is more information about the street,
   not less. It may sharpen the background geometry, or it may smear it. Worth
   knowing before deciding to suppress nudges rather than exploit them.

## The test

One capture set, five files, `s2_sf_4_202609021427*` through `*144716`, which
is the whole van Ness episode: 14:27:14 to 14:48:23.

### A. Characterise the nudges

Measure, per frame, the rigid transform between the current point cloud and the
first settled one. A nudge appears as a step in translation or rotation that
holds; a drive appears as steady accumulation. Report how many nudges there
were, how large each was in centimetres and degrees, and how long the pipeline
took to settle after each.

This is the measurement everything else depends on, and it does not exist yet.

### B. Would settling have recovered?

Replay the episode with the background model's tolerance swept:
`SafetyMarginMetres`, `ClosenessSensitivityMultiplier`, and the post-settle
alpha. For each, record whether the model re-converges after a nudge and how
long it takes. The pass condition is one static segment covering the episode,
not five.

### C. Does the parallax help?

Export the episode twice — once as-is, once with frames from the nudged
intervals discarded — and compare the background point cloud: point count,
surface coverage, and whether the street furniture reads as sharper or doubled.
If the nudged version is better, a nudge is an asset and the pipeline should
keep those frames rather than treating them as an interruption.

## What a good answer looks like

A stated tolerance — in centimetres and degrees — within which a displacement
is a nudge rather than a move, with the van Ness episode classified as one
static site under it, and no day in the archive gaining or losing a site as a
side effect. Plus a yes or no on whether the parallax is worth keeping.

## Risks

| Risk                                                | Mitigation                                                        |
| --------------------------------------------------- | ----------------------------------------------------------------- |
| A tolerance wide enough for nudges hides real drift  | Score against the field map: no day may change its site count      |
| Rigid-transform estimation is itself new machinery   | Only needed offline for the measurement, not in the live pipeline  |
| The nudges are larger than assumed and nothing helps | Then the episode is genuinely three short stops; publish it as one and say so |
