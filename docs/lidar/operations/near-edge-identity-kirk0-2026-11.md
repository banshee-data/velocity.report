# Where the near-edge tracked filter loses identity on kirk0

<!-- ignore-style-length -->

The November benchmark (l5id0019, open PR #994) measured `near_edge_track`, the tracked filter
driven by the near-edge face state, on kirk0 at HOTA −0.061 and IDF1 −0.106 against the defaults.
Duplicate vehicle pairs rose from 10 to 23. This run locates the loss, builds a fix for the defect
it finds, and scores the fix. It answers [l5id0022](../../../data/experiments/l5id/l5id0022.md).

- **Status:** Measured on kirk0, 2026-11-09. The fix fails its rule (R3); nothing adopted, no
  default changed
- **Spec:** [l5id0022](../../../data/experiments/l5id/l5id0022.md), rule committed at `817f663a`
  before any replay; the fix arm added at `37d62ece`, before its replay
- **Layers:** L5 association (the plausibility check before A2's gate), birth
- **Parameter sets:** the [defaults](../../../config/tuning.defaults.json), fingerprint
  `ab0c4035780d`; the
  [overlap-end identity set](../../../config/experiments/identity-body-older-tentative.json),
  fingerprint `51a8d98ee30e`. Every option is a replay experiment, not a tuning key
- **Related:** [near-edge tracked state plan](../../plans/lidar-near-edge-tracked-state-plan.md),
  [kirk0's labels in a cloud run](cloud-run-kirk0-labelled-2026-11.md)

## Summary

- **No zero-code control recovers the loss.** Against `near_edge_track`, the A1 medoid gate is
  worse (HOTA −0.079 to −0.024), so A2's distance formula is not the cause. The robust cost and
  tentative priority gain about 0.015 HOTA, with intervals that reach below zero. The overlap-end
  set removes the duplicates (23 to 9 pairs) without recovering HOTA or IDF1.
- **The duplicates come from a check that compares a body centre with a medoid.** Before A2's
  gate, every pairing passes the plausibility check: the distance from the track's prediction to
  the cluster's medoid, held under 30 m/s over the frame, which is 3 m at 10 Hz. A body-centre
  track predicts its centre. A long body's medoid sits further than 3 m from it, so the body's own
  cluster was refused as a jump. The track went unmatched and the cluster seeded a new track on
  the same body. That was 34 of `near_edge_track`'s 183 births, and none of the defaults' 151.
- **The fix removes those births.** `near_edge_track_common_gate` bounds the medoid's distance
  outside the predicted body's rectangle instead. Births fall to 143, vehicle tracks from 66 to 44
  (the defaults' 44), duplicate pairs from 23 to 16 and pair-frames from 425 to 340 (the defaults'
  452). Switches fall from 33 to 24.
- **The fix does not restore identity, and fails R3.** Against `near_edge_track` its HOTA interval
  is −0.055 to +0.006, IDF1 −0.069 to +0.022; duplicate pairs stay over the defaults' 10. So the
  duplicates are a real defect but not the identity loss.
- **The loss is in association accuracy, not detection.** Scored with a 6 m fixed gate, DetA moves
  by −0.008 against the defaults and AssA by −0.163. The fix leaves AssA where `near_edge_track`
  has it (0.704 against 0.711). Something makes body-centre tracks carry the wrong objects'
  frames; this run does not find what.

## Question and rule

The rule is in the [spec](../../../data/experiments/l5id/l5id0022.md#rule-written-before-replay).
In short:

- **R0, anchor.** `a0` and `ne` reproduce l5id0019's kirk0 rows.
- **R1, recovery.** A control recovers the loss when its HOTA and IDF1 5th percentiles against `ne`
  are above zero, and it is non-inferior against `a0` (HOTA and IDF1 5th percentiles at −0.02 or
  above, switches 95th at +10 or below). It removes the duplicates when its duplicate pairs are
  at most `a0`'s.
- **R2, location.** Read in order: `ne-a1` recovering names the A2 distance, `ne-robust` the
  ranking, `ne-tentative` the young tracks; `ne-overlap` removing duplicates without recovering
  puts the cause upstream of the lifecycle; and if none recovers, the diagnosis's largest category
  of births on a live body-centre track names it.
- **R3, the fix.** A default-off option, byte-identical when off, passes when its HOTA and IDF1 5th
  percentiles against `ne` are above zero and its duplicate pairs are at most `a0`'s.
- **R4.** Nothing is adopted: kirk0 is tuning data l5id0019 has already read.

## Method

- **Arms.** `a0` is the defaults. `ne` is l5id0019's `near-edge` arm: `solid_body`,
  `solid_body_face_state`, `near_edge_track`, `solid_body_full_members`. Each control adds one
  thing to `ne`. `ne-a1` adds `near_edge_track_a1`, `ne-robust` adds `robust_likelihood_cost`
  and `ne-tentative` adds `tentative_priority`. `ne-overlap` runs `ne`'s experiments on the
  overlap-end identity set: tentative priority, the overlap end at one frame on the body, and the
  older track kept. `fix` adds `near_edge_track_common_gate`.
- **Replays.** `lidar-state-estimation-baseline` on kirk0 whole: `-warmup 6 -require-settled=false
-sample-points 1024 -campaign-metrics`, evidence per case, repeat-verified. Every arm's first
  and repeat runs agreed.
- **Duplicates.** `lidar-box-eval -variants none -rows -require-settled=false -warmup 6 -duration
0 -sample-points 1024`, with an evidence database.
- **Identity.** `lidar-ground-truth-eval perframe` on the reconstructed reviewed pack
  (`ds_67842cb1b22b70e7`, pack digest `f0ed3414…`), split drafted with `-from-seconds 6 -split
kirk0-tuning` (manifest `7194bb61…`), `obj_788622b5` ignored, online estimates as declared
  baselines. The paired bootstrap drew 2,000 resamples of the 19 scored objects (seed 20261108).
  Every arm ran against `a0` and every `ne` variant against `ne`. A second scoring with
  `-gate fixed -gate-metres 6`, without the bootstrap, separates detection from association. It
  is a diagnostic, not part of the rule.
- **Diagnosis.** A scratch build of `lidar-box-eval`, never committed, counted every pairing within
  10 m of a track by what decided it (fragment guard, plausibility, d² gate, admitted). It also
  classified every birth by the live track whose body its seed cluster lay on. For a body-centre
  track that is the believed rectangle plus 1 m; for a medoid track, 2 m. It then recorded why
  that track did not take the cluster. The instrumented runs' duplicate figures equal the clean
  runs' exactly, so the counting changed nothing.

## The controls

Full comparison over all hypotheses, with `lidar-box-eval`'s duplicates:

| Arm            |   HOTA |   IDF1 | Switches | Frag |  FN | Tracks | Duplicate pairs | Pair-frames | Vehicle tracks |
| -------------- | -----: | -----: | -------: | ---: | --: | -----: | --------------: | ----------: | -------------: |
| `a0`           | 0.4284 | 0.4728 |       90 |   46 | 265 |     79 |              10 |         452 |             44 |
| `ne`           | 0.3539 | 0.3514 |       33 |   51 | 451 |    109 |              23 |         425 |             66 |
| `ne-a1`        | 0.3068 | 0.2926 |      129 |   58 | 451 |    110 |              26 |         696 |             72 |
| `ne-robust`    | 0.3695 | 0.3821 |       29 |   51 | 438 |    110 |              21 |         450 |             66 |
| `ne-tentative` | 0.3677 | 0.3822 |       37 |   52 | 429 |    105 |              26 |         539 |             66 |
| `ne-overlap`   | 0.3471 | 0.3499 |       31 |   62 | 499 |    121 |               9 |         179 |             66 |
| `fix`          | 0.3415 | 0.3443 |       24 |   49 | 447 |     84 |              16 |         340 |             44 |

`a0` and `ne` reproduce l5id0019's rows to the last digit (R0). `a0`'s tracking-baseline digest is
`79d9abff…`, as l5id0019 recorded.

Paired bootstrap, 5th / 50th / 95th percentiles of the difference:

| Arm            | Against | Δ HOTA                      | Δ IDF1                      | Δ switches        | Δ frag          |
| -------------- | ------- | --------------------------- | --------------------------- | ----------------- | --------------- |
| `ne`           | `a0`    | −0.1157 / −0.0612 / −0.0210 | −0.1639 / −0.1059 / −0.0125 | −201 / −55 / +24  | −23 / +5 / +29  |
| `ne-a1`        | `a0`    | −0.1779 / −0.1123 / −0.0665 | −0.2527 / −0.1777 / −0.0725 | −117 / +45 / +188 | −20 / +11 / +39 |
| `ne-robust`    | `a0`    | −0.1044 / −0.0458 / −0.0073 | −0.1406 / −0.0655 / +0.0165 | −189 / −57 / +20  | −23 / +5 / +29  |
| `ne-tentative` | `a0`    | −0.1035 / −0.0468 / −0.0081 | −0.1427 / −0.0652 / +0.0181 | −182 / −51 / +22  | −23 / +5 / +31  |
| `ne-overlap`   | `a0`    | −0.1219 / −0.0727 / −0.0264 | −0.1863 / −0.1072 / −0.0024 | −197 / −50 / +40  | −21 / +14 / +51 |
| `fix`          | `a0`    | −0.1404 / −0.0875 / −0.0394 | −0.1866 / −0.1266 / −0.0505 | −210 / −65 / +12  | −21 / +2 / +25  |
| `ne-a1`        | `ne`    | −0.0786 / −0.0518 / −0.0242 | −0.1086 / −0.0737 / −0.0337 | +20 / +94 / +192  | −3 / +6 / +17   |
| `ne-robust`    | `ne`    | −0.0036 / +0.0149 / +0.0328 | +0.0017 / +0.0366 / +0.0855 | −8 / −2 / +10     | −2 / 0 / +2     |
| `ne-tentative` | `ne`    | −0.0025 / +0.0139 / +0.0329 | −0.0012 / +0.0369 / +0.0914 | −2 / +4 / +17     | −2 / +1 / +4    |
| `ne-overlap`   | `ne`    | −0.0344 / −0.0097 / +0.0176 | −0.0347 / −0.0031 / +0.0238 | −9 / +3 / +25     | −3 / +9 / +28   |
| `fix`          | `ne`    | −0.0546 / −0.0231 / +0.0059 | −0.0686 / −0.0231 / +0.0218 | −22 / −8 / +3     | −18 / −2 / +13  |

**Against R1 and R2.** No control recovers: none has both 5th percentiles against `ne` above
zero, and none is non-inferior against `a0`. `ne-robust`'s IDF1 interval clears zero against
`ne` and its HOTA interval does not. `ne-a1` loses against `ne` on both, so A2's distance is not
the cause, which was l5id0019's falsifier for its fix 4. `ne-overlap` removes the duplicates (9
pairs, at most `a0`'s 10) and recovers nothing, so the duplicates are cleaned up after they form.
None recovers, so the diagnosis names the step.

## The diagnosis

Births on kirk0 (832 frames), classified by the live track whose body the seed cluster lay on and
by why that track did not take it:

| Births                                                              | `a0` | `ne` | `fix` |
| ------------------------------------------------------------------- | ---: | ---: | ----: |
| All                                                                 |  151 |  183 |   143 |
| On no live track's body                                             |  138 |  118 |   113 |
| On a body-centre track, refused by the plausibility check           |    — |   35 |     0 |
| On a body-centre track, admitted but the track took another cluster |    — |   18 |    18 |
| On a body-centre track, refused by A2's d² gate                     |    — |    1 |     5 |
| On a body-centre track, refused by the fragment guard               |    — |    5 |     5 |
| On a medoid track (within 2 m), any reason                          |   13 |    6 |     2 |

Of the 35 plausibility refusals under `ne`, 34 were on a confirmed track and 31 left that track
with no cluster that frame. The distance from the medoid to the predicted body centre ran from
3.00 to 6.66 m, median 3.16 m, against the 3 m bound. For the 32 that left their track unmatched,
the median believed half-length was 2.59 m, the half-width 1.76 m, and the seed cluster 4.0 m
long. So the body's own cluster was refused as a jump, and it became a new track's seed.

The pairings say the same. Under `ne`, 5,981 of the 12,678 pairings between a body-centre track
and a cluster within 10 m were refused by the plausibility check, and four by the d² gate. The
check is the medoid gate's (`implausiblePairing`): it runs before A2 on the cluster's medoid
against the track's position, which under `near_edge_track` is the body centre. The plan's
invariant 4, like with like, held for A2's gate and not for the check in front of it.

## The fix

`TrackerConfig.NearEdgeCommonGate`, reached by `near_edge_track_common_gate`, keeps the bound but
changes what it bounds. For a body-centre track's A2 pairing, the displacement checked is the
least the body must have moved for the cluster to be its observation. That is the medoid's
distance outside the predicted body's rectangle, along the body axes with the believed
half-extents, and zero inside it. A medoid can sit anywhere from the centre to the near surface,
so its offset from the centre is the body's geometry, not motion. Faces are left out of the
bound. A first version took each face's implied-centre residual instead. On a synthetic 9.6 m
truck on the face-state arm it still refused the body's own cluster on 20 of 80 body-centre
frames, because an implied centre carries the half-extent's error one-for-one. A2's d² weighs
the faces as before. A cluster the medoid bound refuses is measured for A2 only if its medoid is
within the bound plus the believed half-diagonal plus 1 m. Medoid-referenced tracks, A1, the cost
and the update are unchanged. The option qualifies `near_edge_track` and is refused without it
and with `near_edge_track_a1`.

**Off, it is bit-identical.** Four solid-body fingerprints pin it on amd64: the default tracker,
plain tracked, face-state tracked and A1, on synthetic passes. They were taken on the code before
the option existed. Full kirk0 replays of `a0` and `ne` at the fix's build reproduce the earlier
builds' tracking baselines byte for byte (see Verification).

**On kirk0** the plausibility-refusal births are gone (35 to 0). The fix's 143 births are under
`a0`'s 151, and its vehicle tracks equal `a0`'s 44. Its duplicate pairs (16) stay over `a0`'s 10;
its pair-frames (340) are under them (452). What remains on a live body is mainly split clusters:
the body's track took one piece and the other seeded a track (18). The fix does not change that
case, and it is not R3's question.

**Against R3 the fix fails.** Its HOTA and IDF1 intervals against `ne` include zero and lie mostly
below it (HOTA −0.055 to +0.006), and its duplicate pairs exceed `a0`'s. It stays default-off.

## Where the identity loss is

Scored again with a 6 m fixed gate (no bootstrap), so that a body-centre position off the
footprint centre still matches:

| Measure         |   `a0` |   `ne` |  `fix` |
| --------------- | -----: | -----: | -----: |
| HOTA            | 0.4918 | 0.4330 | 0.4230 |
| DetA            | 0.2913 | 0.2829 | 0.2730 |
| AssA            | 0.8743 | 0.7110 | 0.7042 |
| IDF1            | 0.4978 | 0.3972 | 0.3973 |
| Switches        |     93 |     41 |     33 |
| False negatives |    189 |    244 |    202 |
| MOTP (m)        |  1.133 |  1.110 |  1.284 |

Measured: at the wide gate the missed-point gap to `a0` shrinks from 186 to 55 and DetA is level
within 0.01. AssA stays 0.16 down, as at the footprint gate (0.8928, 0.7221, 0.7065). So most of
`near_edge_track`'s HOTA loss is association accuracy: a reference object's frames are spread
over tracks that also carry other objects' frames. It is not tracks that are absent. The fix
leaves that where it was.

Inferred, not tested here: with fewer switches, the low AssA points to body-centre tracks
carrying a neighbouring object's frames, more than to objects split across tracks. The idea is
that A2's loose direction, with variance R plus the believed half-extent squared, admits a
neighbour's cluster cheaply when the body has no usable face. Two things fit. `ne-robust`, which
charges ln det S, gains a little. And A1, which drops the loose variance but adds half a body of
bias, loses more. The next measurement is per object: which reference objects lose AssA, and
which tracks carry them. The scorer reports no per-object rows today.

## What this does not establish

- One labelled capture, 19 scored objects, tuning data already read by l5id0019. The intervals
  are over the objects a reviewer chose.
- The diagnosis's on-body test uses the believed rectangle plus 1 m. A split piece of a body is
  counted as on that body, and a birth on a body the belief understates is counted as on no body.
  The categories are comparable between `ne` and `fix`, not with `a0`'s, whose tracks have no
  believed body; only the totals compare with `a0`.
- The bound admits any jump along a long body that the medoid alone cannot rule out. On the
  synthetic truck the whole gate admitted the cluster moved 8 m along the road on 19 of 80 frames.
  A jump of 15 m along, or 8 m across, was refused on every frame. With the option on, a long
  body's protection against a far jump along its length rests on A2's faces.
- Nothing here reads the placements, the held-out window or the published box.

## Recommendations

1. Keep `near_edge_track` and `near_edge_track_common_gate` off. Use the option in any further
   near-edge identity work, since its defect is a plain mismatch of reference points, and without
   it every long body's own cluster can seed a duplicate. It is not yet worth a corpus read.
2. Measure AssA per reviewed object for `a0`, `ne` and `fix` before another fix. That needs
   per-object rows from `perframe`, or a labelled-object join on the evidence. Then test the
   loose-direction hypothesis: whether body-centre tracks take neighbours' clusters on faceless
   frames.
3. Revisit the split-cluster births (18) with l5id0019's fix 3: a longer extent state puts more of
   a split body inside one track's rectangle.

## Verification

- `gofmt -l` clean; `make lint-go` passed; `go test ./internal/lidar/l5tracks/...
./internal/lidar/replayeval/... ./internal/lidar/pipeline/...` passed.
- The off-path fingerprints (`TestNearEdgeCommonGateOffLeavesTheShippedArithmetic`) and the
  existing face-state and box fingerprints pass on amd64.
- On a synthetic truck: one confirmed track at a time on the plain tracked pass and the face-state
  end-on pass, against two and three; the face-state through pass falls from five to two at once.
  The body's own cluster was refused on 0 of 80 body-centre frames, against 37.
- kirk0: every arm's first and repeat runs agreed. At the fix's build, with the option off, `a0`
  and `ne` reproduce the earlier build's tracking baselines byte for byte (`79d9abff…` and
  `93b354c9…`). Their box rows also reproduce: the same 9,273 and 9,877 rows, equal once the
  random track IDs are blanked, and the same duplicate figures.

## Provenance

| Item                 | Value                                                                                                                                                                                                                                                                      |
| -------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Capture              | kirk0, `20251205T1417_80858-75c_kirkham-9th`, sha256 `2864ebde38e7…`                                                                                                                                                                                                       |
| Builds               | Version `0.5.1-pre52` through the Makefile's LDFLAGS with the `pcap` tag. Controls at `704b8e46`, whose tracker code equals the spec commit `817f663a`. The fix and the off-path replays at `37d62ece`. Diagnostic builds were scratch builds and are not cited for scores |
| Labels               | Pack `ds_67842cb1b22b70e7`, reconstructed (pack digest `f0ed3414…`), annotation revision 2233; split manifest `7194bb61…`, reference digest `a784e69d…`                                                                                                                    |
| Tuning fingerprints  | `a0`, `ne`, controls and `fix`: `ab0c4035780d`; `ne-overlap`: `51a8d98ee30e`                                                                                                                                                                                               |
| Arm parameter hashes | `a0` `ab0c4035780d`; `ne` `fa4f7b0d0bea`; `ne-a1` `0c249267a9f0`; `ne-robust` `3ca623b0439e`; `ne-tentative` `27170f6b0beb`; `ne-overlap` `cec4b083c5bb`; `fix` `dd359d157bce` (each folds the replay experiments into the tuning hash)                                    |
| Tracking baselines   | `a0` `79d9abff…`; `ne` `93b354c9…`; `fix` `b577cc7f…` (first and repeat equal)                                                                                                                                                                                             |
| Bootstrap            | 2,000 draws of 19 objects, seed 20261108                                                                                                                                                                                                                                   |

Raw outputs (evidence databases, scores, diagnostic counts) stayed in local scratch space and were
deleted once summarised here.
