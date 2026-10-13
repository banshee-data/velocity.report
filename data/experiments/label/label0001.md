---
id: label0001
title: A labelled, frozen held-out window at embarcadero-folsom
status: proposed
owner: david
created: 2026-10-13
asks:
  - data/maths/proposals/20261011-beamwise-background-subtraction-review.md#7-a-decision-rule-that-can-be-pre-registered
  - docs/plans/lidar-tracker-geometry-convergence-plan.md#6-work-to-scope-and-schedule
requires:
  capabilities: [pcap:s2, macos-review]
reclaim: never
steps:
  - id: select
    kind: manual:capture
    estimate: { human: 1h }
    note: draw a 3-minute window of embarcadero-folsom by lot, not by what is in it; register the clip
  - id: propose
    kind: manual:annotate
    estimate: { human: 4h }
    note: road users and full-scene static and dynamic labels, proposals accepted or corrected in the macOS tool
  - id: review
    kind: manual:review
    estimate: { human: 3h }
    gate: { reviewed_fraction: 1.0 }
    note: a second pass grades every proposal; only graded records count as truth
  - id: freeze
    kind: manual:freeze
    estimate: { human: 15m }
    note: velocity lidar annotation-split freeze with the role held_out; the Tuned ledger keeps it from ever being tuned on
---

# label0001: a labelled, frozen held-out window

**Question.** Every L3 and geometry score so far is on kirk, a tuning split. What is the first
held-out window both lines can be scored on?

**Method.** Entirely manual: select by lot, label, review, freeze. Its product is a frozen split
digest that l3bg0009 and the geometry line's W7 score read. `reclaim: never` because a person's
half-finished labels are not work another worker can resume.
