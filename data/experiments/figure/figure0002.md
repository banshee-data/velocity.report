---
id: figure0002
title: "Track speed against kirk0's reviewed physical references: the first independent check on a speed the platform reports"
status: answered
owner: david
created: 2026-11-05
asks:
  - docs/lidar/operations/experiment-review-2026-11-05.md#e-confidence-in-the-outputs
depends_on: [l5id0015]
requires:
  capabilities: [pcap:kirk, labels:kirk, pcap-build]
  disk_gb: 2
reclaim: any
arms:
  - name: a0
    tuning: config/tuning.defaults.json
  - name: body-older-tentative
    tuning: config/experiments/identity-body-older-tentative.json
cases:
  corpus: kirk0
  count: 1
steps:
  - id: implement
    kind: manual:implement
    estimate: { human: 1h }
    note: >-
      scripts/lidar_reference_speed.py: the reference speeds and the matched tracks' speeds by the
      definitions below, from the pack's physical-references.json and an evidence database; fixture
      tests for the span, the bound, the attribution and a track that changes over the span
  - id: replay
    kind: state_estimation_baseline
    over: [arms, cases]
    estimate: { each: 2m, worker: m1pro }
    note: kirk0 whole, 6 s warm-up, evidence per case, as l5id0015 ran
  - id: score
    kind: ground_truth_eval
    over: [arms]
    estimate: { each: 2m, worker: m1pro }
    note: the script over each arm's evidence
  - id: write
    kind: manual:write
    estimate: { human: 1h }
report: docs/lidar/operations/cloud-run-kirk0-labelled-2026-11.md
actual:
  wall: 45m
  note: a Claude Code cloud container, on the reconstructed kirk0 pack and existing replays
---

# figure0002: track speed against kirk0's physical references

**Question.** The platform's headline figures are speeds, and no figure in the repository bounds
how far a LiDAR track's speed is from the vehicle's (the review's E17 and E18). kirk0's physical
references are reviewed, independent fits of three vehicles' bodies at keyframes, each position
with a stated bound. Differenced over a span, they give each vehicle a reference speed with a
bound. How far are the tracks' speeds over the same spans from them, under the defaults and under
the identity candidate?

**Definitions, fixed before any track speed is read (2026-11-05).** From the pack's
`physical-references.json` (revision 77):

- **Reference keyframes.** A vehicle's keyframes whose review status is `reviewed` and origin
  `independent`, with their position observed or inferred.
- **Span.** For each vehicle, the anchor kind (front face, rear face or body centre) with the most
  such keyframes; its first and last keyframe of that kind. Both ends name the same point on the
  body, so the offset between anchor and centre cancels while the body does not turn.
- **Reference speed.** The distance between the span's two positions over the time between them.
  **Its bound** is the sum of the two positions' bounds over the same time.
- **The matched track.** At each of the span's keyframes, the track whose row at that frame is
  nearest the keyframe's position, within 6 m (a truck's anchor can sit half a body from its
  track's centre); the track matched at most of the keyframes, ties to the earlier keyframe. A
  vehicle with no track within 6 m at half its keyframes is unmatched.
- **Track speed.** The mean of the matched track's row speeds whose frame lies within the span, and
  separately its displacement between the rows nearest the span's ends over the time between
  them. The rows are the evidence's `lidar_track_observations` keyed by frame.
- **Error.** Track speed less reference speed, each way; within the bound when its magnitude is no
  more than the reference bound.

**Rule (2026-11-05).** A measurement on three vehicles; it decides no default. It reports, per
vehicle and arm, the reference speed and bound, the matched track, both track speeds, the errors
and whether each is within the bound. The report states it as the first independent check on a
LiDAR speed, on three slow vehicles of one capture, and that a confidence statement about the
platform's speeds needs the radar capture the review recommends.

**What could mislead.** Three vehicles, two of them trucks turning at a junction: if a body turns
over the span, its front or rear face moves further than its centre, and the reference reads fast.
The anchors were fitted to reviewed returns the pipeline also saw, so this is independent of the
tracker but not of the sensor. A span of under two seconds has a wide bound.

**What it does not establish.** Speeds above kirk0's slow traffic; the p85 or p98 of a population;
anything off kirk0.

**Answer (2026-11-05).** The rows' speed was the track's running mean, not the instant speed: over
the three spans it read 0.38 to 0.72 of the reference speeds (2.49, 3.26 and 3.53 m/s against
6.48, 4.50 and 7.15), while each track's displacement speed matched its reference within 0.42 m/s.
The row now records the instant speed; with it the car is within its bound and the two trucks read
0.77 and 0.86 m/s low. See the
[report](../../../docs/lidar/operations/cloud-run-kirk0-labelled-2026-11.md#figure0002-track-speed-against-the-physical-references).
