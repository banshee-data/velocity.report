---
id: label0002
title: Physical references and following records on the held-out window
status: proposed
owner: david
created: 2026-10-18
asks:
  - docs/plans/lidar-tracker-geometry-convergence-plan.md#6-work-to-scope-and-schedule
  - docs/plans/lidar-physical-reference-review-plan.md#checklist
  - docs/lidar/operations/end-on-truck-window-spike-2026-10.md#the-capture-request
  - docs/lidar/operations/l5-duplicate-tracks-2026-10.md#recommendations
  - docs/lidar/operations/experiment-review-2026-10-18.md#labelling-and-annotation-priorities
depends_on: [label0001]
requires:
  capabilities: [pcap:s2, macos-review]
reclaim: never
steps:
  - id: fit
    kind: manual:annotate
    estimate: { human: 3h }
    note: every vehicle object with reviewed masks fitted to its returns (annotation-reference fit), each pose reviewed in the window; every long body keyframed by the operator for yaw and extents, as W7's rule asks, so its reference is not a fit to the returns the tracker reads
  - id: following
    kind: manual:annotate
    estimate: { human: 2h }
    note: a following record for every follower interval in the window (leader, no_leader or ambiguous) drafted with draft-following and imported reviewed; both parties of each leader decision reviewed at the samples the gap needs
  - id: freeze
    kind: manual:freeze
    estimate: { human: 15m }
    note: the physical revision pinned in the held-out split beside the membership review, episodes non-overlapping
---

# label0002: physical references and following records on the held-out window

**Question.** label0001 labels a lot-drawn window's road users and freezes it, which is what the
identity (l5id0004) and L3 (l3bg0009) scores need. The geometry line's W7 and the following gate
need more from the same window: the bodies' sizes and poses as references independent of the
tracker, and the operator's decision of who follows whom. What does one operator pass on that
window give all three?

**Method.** The W7 spike's selection rule and label0001's draw are the same lot: one segment of
embarcadero-folsom the corpus never used, chosen by lot, not for what is in it. On the frozen
membership, every vehicle is fitted to its reviewed returns and reviewed; long bodies, end on or
not, are keyframed by hand for yaw and extents; and a following record is drafted for every
follower interval, with `no_leader` and `ambiguous` kept as outcomes. `reclaim: never`: a
person's half-finished references are not work a worker resumes.

**What it gives.** The held-out physical score for the geometry line (W7); the leader truth and
reference gaps the following gate scores (l8fol0005); and bodies for the duplicate count's
proxy to be checked against. One pass instead of three captures.

**What it does not establish.** A second site, or an end-on truck: by the spike's rate a
five-minute segment there holds one or two clean end-on windows and about one chance in four of
a truly end-on one; draw a second segment if the first has none, and record both draws.
