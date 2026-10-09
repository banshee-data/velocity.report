# Physical-reference pilot on kirk0: first frozen score

<!-- ignore-style-length -->

The first end-to-end run of the physical-reference workflow: three vehicles in kirk0's slow lane
fitted to their reviewed returns, reviewed, joined by two following references, frozen in a split
and scored against the two solid-body arms the [F0 attribution run](facet-f0-attribution-kirk0-report.md)
replayed. It is F0's Step 6, which that run could not take because the pack had no references.

- **Status:** Complete, 2026-10-09. Tuning split only: one capture, three vehicles, 22 reviewed
  poses and 11 following gaps. A pilot of the workflow and a first reading, not a result
- **Layers:** L5 estimation, L8 behaviour (following gap), offline evaluation
- **Related:** [solid-body alignment](solid-body-physical-alignment-kirk0-2026-10.md) (the follow-up run), [fitting to reviewed points](../../plans/lidar-physical-fit-to-points-plan.md), [physical-reference review plan](../../plans/lidar-physical-reference-review-plan.md) (P3), [F0 report](facet-f0-attribution-kirk0-report.md), [F0 protocol](facet-f0-attribution-kirk0.md) (Step 6), [per-frame evaluation](per-frame-evaluation.md), [point annotation tool](point-annotation-tool.md#scoring-against-physical-references)
- **Code:** [physical fit](../../../internal/lidar/annotation/physical_fit.go), [draft-following](../../../internal/lidar/annotation/physical_following.go), [lidar-ground-truth-eval](../../../cmd/tools/lidar-ground-truth-eval/perframe.go)

## Summary

The workflow runs end to end. An operator fitted, reviewed and froze references for three
vehicles in the window, two following records were drafted from them, and the frozen score
reproduces from its bundle.

What the first score says, on 22 poses:

- **Car 8 is tracked well.** Both arms put its centre a median 0.25 m from the reference and its
  heading 6° off, with box overlap 0.71.
- **Neither truck is.** Neither arm builds either truck's length. The near-edge arm holds truck 1
  at 5.9 m against a reference 10.6 m, so it places the rear within 0.2 m and the centre 2.2 m
  behind where it is. The control holds truck 1 at 0.4 m with its heading 85° off. Both arms hold
  truck 2, seen end on as it approaches, at 0.6 m long with its heading 76° to 126° off.
- **The gaps follow the trucks.** Where the follower is car 8 and the leader truck 1, the
  near-edge arm's gap is within 0.12 to 0.46 m of the reference at all three scored samples, inside
  each reference's bound; the control's is 7.9 m long at one and 0.3 m at another. Where truck 2
  follows car 8, both arms' gaps are 1.6 to 2.2 m long at all four scored samples, because truck
  2's predicted footprint ends short of its real front.

This is consistent with F0's post-hoc reading, that the along-path error on kirk0 is mostly an
extent too short for the body. It does not close F0's attribution gate: two trucks are the
whole of the large-error evidence.

## Question

1. Can an operator produce reviewed, independent physical references and following gaps with the
   fit-to-points flow, freeze them, and score a tracker against them, with every pin holding?
2. On the first such references, how far are the F0 arms' body centre, heading, size, bumpers and
   following gap from the reference, within the references' own bounds?

## The references

Pack `ad8b9438` is the whole of `kirk0.pcapng`: 832 samples, 83 s, foreground only, recorded with
the sensor about 2.3 m above the road. The three vehicles share one lane through the sensor's
nearest approach, about 6 m/s, in this order:

| Vehicle | Object         | Size fitted, m             | Reviewed poses (samples)                       | Role            |
| ------- | -------------- | -------------------------- | ---------------------------------------------- | --------------- |
| Truck 1 | `obj_35c661db` | 10.63 ± 0.24 × 2.91 ± 0.27 | 9: 479, 490, 491, 492, 494, 495, 500, 504, 509 | Leads           |
| Car 8   | `obj_8c728991` | 4.26 ± 0.23 × 1.73 ± 0.23  | 7: 491, 495, 500, 501, 502, 504, 509           | Follows truck 1 |
| Truck 2 | `obj_8e829a06` | 9.63 ± 0.35 × 2.84 ± 0.20  | 6: 491, 495, 500, 502, 504, 509                | Follows car 8   |

Each size and pose is `fit:mask_v1`, the fit to the object's reviewed returns
([plan](../../plans/lidar-physical-fit-to-points-plan.md)), reviewed by the operator in the
window. Every height is a lower bound only, so height scores nowhere. Car 8 is visible only from
sample 472 to 530, between the trucks; elsewhere the trucks hide it.

The two following records cover samples 472 to 530, car 8's visible span, and were drafted from
the reviewed poses with `annotation-reference draft-following`. A gap is drafted only where both
parties have a reviewed, independent pose that places the bumper it needs:

| Pair                 | Gaps, samples                | Reference gap, m | Mean half-width, m |
| -------------------- | ---------------------------- | ---------------: | -----------------: |
| Car 8 behind truck 1 | 491, 495, 500, 504, 509      |     11.6 to 15.9 |                2.0 |
| Truck 2 behind car 8 | 491, 495, 500, 502, 504, 509 |     21.2 to 16.4 |                1.5 |

Two lateral checks put each pair in one lane: centre to centre, the leader is within 1.6 m of the
follower's axis at every gap sample, and the headings agree within 6.4°.

Three pilot findings bear on the references themselves:

- **A tracker-assisted declaration persists.** An "obs" entry in the window's estimate-disclosure
  field made every truck 1 edit tracker-assisted until it was cleared from the app's settings. Poses fitted while it stood stayed
  assisted after it was cleared, and the one at sample 495 had to be fitted again before its gap
  could be drafted. Truck 1 keeps one assisted pose, at 334, which nothing here uses.
- **Masks held stray returns.** The first fits named returns up to 9.5 m ahead of truck 1 and 5.5 m
  behind truck 2; the operator removed them before these fits. Shorter runs remain, up to 3.3 m
  ahead of truck 1 and 2.3 m ahead of truck 2. Inside the scored samples they lie off truck 1's
  front, which the fit does not take as seen there.
- **The window labels one frame three ways.** Sample 500 is "Frame 501 of 832" at the top,
  "Frame 504 · 501 of 832" in the left column (the recording's frame), and sample 500 in every
  tool and record. It reads as an off-by-one and is not one; the fit plan's F4 owns the fix.

## Method

The split was frozen in the window through the service, `kirk0-ad8b9438-tuning-r1`: one tuning
partition of all 30 reviewed objects and two episodes, `whole-pack` (every object, samples 0 to 831) and `following-472-530` (the three vehicles). The episodes overlap, and a score over both
counts each pose twice, so every physical figure below is from `following-472-530` scored alone
(`-episodes following-472-530`). The identity figures for the whole pack are from a run over both,
read per episode.

The arms are F0's control and `track` replays, at build `e4905133`, read from
`lidar_track_solid_bodies` at stage `online` as declared baselines:

| Arm       | F0 name   | Experiment                                                                              | Tracks | Points |
| --------- | --------- | --------------------------------------------------------------------------------------- | -----: | -----: |
| control   | `control` | `solid_body,solid_body_face_hysteresis,solid_body_course_faces,solid_body_full_members` |     59 |  1,951 |
| near_edge | `track`   | the same and `near_edge_track`                                                          |     67 |  1,924 |

```bash
gt-eval perframe -pack "$PACK" -split-manifest "$LIDAR_ANNOTATION_DIR/splits/kirk0-ad8b9438-tuning-r1.json" \
  -split kirk0-tuning -episodes following-472-530 -allow-tuning-split -physical-reference \
  -a-label control -a-db "$F0/control/evidence/kirk0.db" -a-stage online -a-declared-baseline -a-solid-body \
  -b-label near_edge -b-db "$F0/track/evidence/kirk0.db" -b-stage online -b-declared-baseline -b-solid-body \
  -physical-bundle-dir "$W/bundle-following" -json "$W/physical-following.json" -markdown "$W/physical-following.md"
```

A prediction matches a reviewed pose by capture instant and nearest within 3 m. A component is
scored only where the reference states it, with its bound. A solid-body row that reports the
cluster medoid, not a body centre, counts as a missing centre. `verify-bundle` reproduces both
bundles at the pinned revision.

## Findings

### Every component

Means over the scored poses of the following episode. A bound is the reference's own half-width.

| Component         | control scored | control mean error | near_edge scored | near_edge mean error | Mean reference bound |
| ----------------- | -------------: | -----------------: | ---------------: | -------------------: | -------------------: |
| Centre, m         |             17 |               1.67 |               19 |                 1.61 |         0.89 to 0.96 |
| Heading, °        |             21 |               70.6 |               21 |                 33.2 |                  3.7 |
| Length, m         |             21 |               6.19 |               21 |                 4.47 |                 0.27 |
| Width, m          |             21 |               1.03 |               21 |                 0.27 |                 0.24 |
| Front, m          |             15 |               2.37 |               19 |                 2.95 |         0.90 to 1.21 |
| Rear, m           |             15 |               4.44 |               19 |                 2.42 |                 0.77 |
| Box overlap (IoU) |             17 |               0.26 |               19 |                 0.41 |                    – |
| Following gap, m  |              6 |               2.66 |                7 |                 1.15 |         1.16 to 1.26 |

Of the 22 reviewed poses, one (truck 1, sample 479) has no prediction within the gate in either
arm. The control reports a medoid instead of a body centre at four, the near-edge arm at two, all
of them car 8 at 491 and 495 or truck 1 at 492 and 495, where the solid body falls back to the
medoid.

### By vehicle

Medians. Along is prediction minus reference along the reference's axis, positive forward.

| Vehicle | Arm       | Centre, m | Along, m | Across, m | Heading, ° | Length, m (reference) | Box IoU |
| ------- | --------- | --------: | -------: | --------: | ---------: | --------------------: | ------: |
| Truck 1 | control   |      2.62 |    +2.33 |     −1.02 |       85.0 |          0.38 (10.63) |    0.08 |
| Truck 1 | near_edge |      2.19 |    −2.19 |     +0.19 |        6.7 |          5.88 (10.63) |    0.50 |
| Car 8   | control   |      0.24 |    −0.15 |     −0.15 |        6.0 |           3.88 (4.26) |    0.72 |
| Car 8   | near_edge |      0.25 |    −0.16 |     −0.16 |        6.0 |           3.88 (4.26) |    0.71 |
| Truck 2 | control   |      1.99 |    +1.93 |     −0.01 |      125.9 |           0.62 (9.63) |    0.05 |
| Truck 2 | near_edge |      2.00 |    +1.93 |     −0.01 |       75.8 |           0.62 (9.63) |    0.05 |

The bumpers say where each box sits. The near-edge arm puts truck 1's rear a median 0.18 m from
the reference rear and its front 4.6 m short, so its box is anchored on the seen end and simply
too short. The control puts truck 1's rear 7.6 m forward of the real one: its 0.4 m box sits at the
front of the truck. Both arms put truck 2's front 2.7 to 3.0 m behind the real one along its axis,
with the box turned across the truck. Car 8's ends are within 0.5 m in both.

### Following gaps

Prediction minus reference, m. Blank is not scored, with the reason beneath.

| Pair                 | Sample | Reference    | control | near_edge |
| -------------------- | -----: | ------------ | ------: | --------: |
| Car 8 behind truck 1 |    491 | 11.61 ± 2.62 |         |           |
|                      |    495 | 12.63 ± 2.65 |         |           |
|                      |    500 | 14.08 ± 0.97 |   +7.93 |     +0.46 |
|                      |    504 | 15.00 ± 1.84 |         |     +0.12 |
|                      |    509 | 15.92 ± 1.92 |   +0.29 |     +0.24 |
| Truck 2 behind car 8 |    491 | 21.17 ± 3.86 |         |           |
|                      |    495 | 20.30 ± 1.15 |         |           |
|                      |    500 | 19.00 ± 0.93 |   +2.19 |     +2.09 |
|                      |    502 | 18.42 ± 1.15 |   +1.69 |     +1.65 |
|                      |    504 | 17.90 ± 1.10 |   +1.78 |     +1.71 |
|                      |    509 | 16.40 ± 0.91 |   +2.04 |     +1.77 |

Not scored: at 491 and 495, in both arms, car 8 had too little observation for its own
footprint, as follower of truck 1 and as leader of truck 2. At 504 the control's truck 1 had no
resolved orientation. Every scored gap is longer than the reference, and the four truck 2 gaps by
about the distance its footprint falls short of its front.

### Identity, for context

The per-frame identity score, at visible-mask positions, not body centres:

| Episode           | Metric      | control | near_edge |
| ----------------- | ----------- | ------: | --------: |
| following-472-530 | HOTA        |   0.506 |     0.535 |
|                   | ID switches |      23 |         6 |
| whole-pack        | HOTA        |   0.356 |     0.310 |
|                   | IDF1        |   0.330 |     0.285 |
|                   | ID switches |      96 |       100 |

The near-edge arm is better through the lane of slow traffic and worse over the whole capture,
as F0 found.

## Interpretation

On this lane, both arms track a car well and neither tracks a truck. The trucks fail the same way
in both arms: the body is never as long as the truck. Where the near-edge arm has a seen end to
anchor on (truck 1's rear), the box sits on it and the error moves to the unseen end and the
centre; that is the extent shortfall F0 inferred from the labelled span, now measured against a
body. Where the only seen end faces the sensor and the vehicle approaches end on (truck 2), both
arms hold a face-sized box turned across the truck, and the gap behind car 8 inherits its short
front.

The near-edge arm's advantage here is truck 1: heading 6.7° against 85°, width 0.27 m against
1.03 m, and the three gaps it shares with truck 1 inside their bounds. Car 8 is the same in both.
Truck 2 is wrong in both.

What this does not establish:

- Anything held out. The split is tuning, kirk0 is where the shadow was developed, and physical
  scoring refuses a held-out split until its limits are pinned on tuning data.
- A rate. Three vehicles, two of them the slow trucks that dominate F0's residuals too; no
  interval is reported because there is nothing to resample.
- A physical measurement. The references are bounded fits to reviewed LiDAR returns, not measured
  vehicles. A fitted length is the 90th percentile of the spans that see both ends; it can be
  short if no frame sees the whole truck, and its bound is the fit's stated terms, not a calibrated
  interval.
- Final estimates. Both arms are online-stage positions from yesterday's build, scored as declared
  baselines.
- Why truck 2's heading is wrong. The box is turned across the truck in both arms; this report
  measures that and does not diagnose it.

## Recommendations

1. Read truck 2 frame by frame in Compare (samples 491 to 509): an end-on approaching truck whose
   box turns across it is the case the solid body's extent admission has to handle, and this pack
   now has references for it.
2. Add pairs before reading rates. The next candidates are kirk0's other followed vehicles and a
   second site's slow traffic; each pair needs reviewed poses at shared samples, which
   [fit-to-points F5](../../plans/lidar-physical-fit-to-points-plan.md) is meant to suggest.
3. Freeze revision 2 of the split with non-overlapping episodes, or always score physical
   references with `-episodes`: the pooled table counts an overlapped pose once per episode.
4. Give the window one frame label (fit plan F4): the three labels for one sample cost the
   operator a question during this pilot.

## Provenance

| Identity            | Value                                                                                                                                                                                                                               |
| ------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Pack                | `ad8b9438-c1d4-40f0-bd0a-f1852c984569-20260929-184718.156560000`, `sha256:95be6fb22498b44a…3849d6`                                                                                                                                  |
| Pack manifest       | `sha256:0c06c882a62c845bbe63a3c8dfa39dc598b58d1eafc04e14d30a7dabd206b433`                                                                                                                                                           |
| Annotation revision | 2232, `sha256:ec51b1078c4a29fcc631885b6fb279a94411bde15cae7b4ee830fe1781bd6cb9`                                                                                                                                                     |
| Physical revision   | 76, `sha256:7d80fd2718d64f47f3ee50fa2b7ecce5942ff5ab587db5c7abcde111eeda98c3`; content `sha256:50d7761a…7ea3`                                                                                                                       |
| Split               | `kirk0-ad8b9438-tuning-r1`, revision 1, `split_digest` `sha256:b58ee590e340f5a7e95f23d9c59937f8fd1f41027f722ab28a5298b4e9721f05`, file `sha256:b9b11a26…0e111c`, frozen by dd at 2026-10-09T07:48:25Z with server build `4253fc664` |
| Arms                | F0 replays at `e4905133`, `baseline` binary `sha256:897eaf72…995f`; control `sha256:2faa2a0d…5c2`, near_edge `sha256:5c7fd88a…d58` parameter hashes                                                                                 |
| Evaluator           | `lidar-ground-truth-eval` at `51638d6c9`, stamped, `sha256:a9882e5055e3afcad3de5692b5c460ba489878ace8831d91a52ec479b53197fe`                                                                                                        |
| Bundles             | `bundle` (both episodes) and `bundle-following`; `verify-bundle` reproduces both                                                                                                                                                    |

Raw outputs, none of them in Git: `~/vr-scratch/physical-score-kirk0-r1/` holds the evaluator, both
JSON and Markdown outputs, the logs and both bundles. The split draft is
`kirk0-ad8b9438-tuning-draft.json` beside the packs, and the frozen split is in the annotation
folder's `splits/`. The arms' databases are F0's, under `~/vr-scratch/facet-f0-kirk0/`.
