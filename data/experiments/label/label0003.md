---
id: label0003
title: "Spans first: road-user spans on label0001's lot-drawn window, before masks"
status: proposed
owner: david
created: 2026-11-05
asks:
  - docs/lidar/operations/experiment-review-2026-11-05.md#f-truth-held-out-budget-and-process
  - docs/lidar/operations/overlap-end-default-cost-2026-11.md#recommendations
requires:
  capabilities: [pcap:s2, macos-review]
reclaim: never
steps:
  - id: select
    kind: manual:capture
    estimate: { human: 15m }
    note: >-
      label0001's draw, made once for both: a 3-minute window of embarcadero-folsom drawn by lot
      (velocity lidar segments --finder random --role held_out), the draw recorded, the clip cut;
      label0001's select step is closed by the same artefact
  - id: spans
    kind: manual:label
    estimate: { human: 2h }
    gate: { labelled_fraction: 1.0 }
    note: >-
      every road user's first and last frame, its class, and whether it is the same vehicle as
      another span (one record per vehicle, as kirk1's reference 60a4774c holds 49 road users);
      no masks, no poses
  - id: freeze
    kind: manual:freeze
    estimate: { human: 15m }
    note: >-
      velocity lidar annotation-split freeze with the role held_out and the span revision; the
      masks and review of label0001 follow on the same window and refreeze it
---

# label0003: spans first on the held-out window

**Question.** label0001, eight hours of a person's time, has been the critical path since October
13 and has not started; every identity, L3 and geometry adoption waits on it. The identity
decision needs less than all of it. kirk1's reference (60a4774c) is 49 road users as spans with a
class and no per-frame masks, and l3bg0022 and l5id0012 read it as "covered when any track
overlaps 30 % of the span". That product decides whether an option loses a real vehicle, and the
count, with truth; it does not give HOTA. What does two hours of spans on label0001's own window
decide, before the masks?

**Method.** The window is label0001's: drawn by lot on embarcadero-folsom, never read label-free
by any screen, the one site the corpus registry holds out. The operator records each road user's
span and class and marks spans that are one vehicle (a long vehicle seen in two pieces is one
record). The split is frozen with the role `held_out` and the span revision; label0001's masks
and second-pass review then proceed on the same window and refreeze it. No second window is
drawn.

**What it decides.** For any arm, read on the window: lost real vehicles (a reference span no
track of the arm overlaps by 30 %), against `a0`'s; the moving count against the number of
vehicle spans; duplicates against spans with two or more overlapping tracks. l5id0014's held-out
clause reads these. It does not decide HOTA, IDF1 or per-frame identity, which remain label0001's.

**Rule.** A labelling product has no pass; it is done when every span in the window is recorded
and the split is frozen. One rule holds: the window is label0001's lot, and nothing about which
arm fails where chooses it.

**What could mislead.** A span is one person's judgement of when a road user is in view; a vehicle
that leaves and re-enters is one span or two by that judgement, and the record says which. An
operator who has seen the overlap end's worst sites could, without meaning to, mark long vehicles
with more care than cars; the second pass of label0001 grades every span as it grades every mask.

**What it does not establish.** Per-frame identity; the box's geometry; anything a mask would
show. Those are label0001's and label0002's.

## The draw (select step, November 8, 2026)

Made once, for label0003 and label0001 both. The only selector that could draw a held-out window at
random was fixed at 10 s, too short to hold the road users a whole-window label needs, so the
draw waited on `random_180s`: the same seeded draw, one window per capture, at 180 s, the one
width the code lets the random draw widen to (`HeldOutRandomWindowSeconds`, commit 4739950d5).
Nothing about the window's contents entered the draw. It placed 180 s windows from 35 s into the
capture by packet time alone and kept the one with the highest seeded score.

| Field      | Value                                                                                                                                                                                       |
| ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Capture    | `sf-street-speeds/raw/lidar/20260903T1317_80858-06c_embarcadero-folsom.pcapng`, the whole 20 minutes                                                                                        |
| Selector   | `random_180s`, digest `sha256:4f18bbad767482a7a78ed3e0da92b549df223c13ce8ec64b16b5fcfc3bf633f8`                                                                                             |
| Parameters | `window_seconds` 180, `random_seed` 1, every other parameter at its default                                                                                                                 |
| Window     | `seg-955587a76c49fe5c6bd735c4`, 575 to 755 s into the capture, draw score 0.700438                                                                                                          |
| Role       | `held_out`                                                                                                                                                                                  |
| Run        | `6324d632-8cff-4b6a-a28d-bfec3ce1b082`: the first 40 s replayed at default parameters (fingerprint `ab0c4035780d`) so that a run named the capture; its tracks were not read                |
| Build      | `velocity` v0.5.1-pre52 at 4739950d5, stamped by `make build-radar-local`                                                                                                                   |
| Pack       | `sha256:ffdee4d37bdd34e78afa5bc91927adc0e4509279e1caad56f7c2dcee41c2a132`: 1,800 samples (version 2, foreground-only coverage with the settled background snapshots, as every clip records) |

The draw report and the clip (its VRLOG and pack) are outside the checkout, in
`/Volumes/lidar/lidar/work/label0003-20261107/`. The pack is labelled blind, with tracker previews
hidden. No second window is drawn: label0001's masks and review go on this one.
