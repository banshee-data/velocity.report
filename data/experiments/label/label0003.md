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
    gate: { reviewed_fraction: 1.0 }
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
