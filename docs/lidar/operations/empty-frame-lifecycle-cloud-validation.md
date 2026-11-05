# Empty-frame lifecycle validation

- **Status:** Implemented; cloud regression validation
- **Layers:** L5 Tracks, pipeline, L9 Endpoints
- **Scope:** Delivered empty/unavailable frames; no physical accuracy or target-hardware claim

## Problem and change

The callback returned before L5 whenever foreground, ground filtering or clustering
produced no detections. Empty L2 frames were neither counted nor published. A
quiet street therefore stopped the tracker clock: predictions, misses and expiry
froze until another cluster arrived. Recorded placeholder frames contained no
coasted state.

Each delivered frame now advances L5 once at its capture timestamp. Observed
empty detection sets use the configured absence policy. Empty L2 input, missing
L3 processing and replay throttling use `Tracker.UpdateGap` and `missed_unknown`
support: missing processing cannot establish an occlusion or field-of-view exit.
The existing prediction, coast bounds, miss budgets and deletion logic apply.
Reduced-depth profiles do not run L5. The optional empty-frame adapter includes
active and recently deleted track state, retaining its original point policy.
No empty-frame path inserts a detection, observation or observed estimate.

Throttled replay frames now age tracks, too. A preview that deliberately skips
processing can lose tentative tracks; analysis replay continues to disable the
throttle and is the appropriate path for complete tracking evidence.

## Checks and numerical evidence

Synthetic regression cases cover zero points, missing model, no foreground,
ground-only output and noise-only clustering. A confirmed track moving at 2 m/s
advances 0.2 m for each 100 ms empty frame, keeps its observation count and
last-observed timestamp, and expires on the fourth miss under a four-miss budget.
Reduced-depth profile tests leave the supplied tracker untouched.

The acquisition-time case predicts a 5 m/s track from 1.09 s to an empty frame
at 1.20 s: 0.55 m. Duplicate and backward capture timestamps do not reverse its
position, both discontinuities are counted, and the configured capture-time
bound expires the track without adding an observation.

The established kirk0 replay window uses
`config/experiments/l3-pre-consolidation.json`, capture seconds 0–20 as warm-up
and seconds 20–28 as scored input. It delivered 281 frames to L5, with maximum
inter-update spacing 0.10409 s and zero intervals clamped by `MaxPredictDt`.
The earlier test expected missing empty updates to make this a large-gap window;
the regression now requires those artificial gaps to stay absent. The
`capture_gap_predict` arm is numerically identical to the default on this
continuous stream, while `measurement_time` remains distinguishable. An
identical replay checks canonicalised tracks, trails and baseline accounting.

## Provenance and limits

Base: `1ec1ccea5947ec8f72d155b337ed86d066f342c4` (current main at implementation).
Capture: kirk0, 200,657,872 bytes, SHA-256
`2864ebde38e736b496d33361e9bcdc9246aa5147459ec48aee0f8f11f1f58b9a`,
materialised through Git LFS. Host: Linux amd64, Go 1.27.1, extracted libpcap.
The replay parameters are the checked-in test configuration, unchanged across
arms except the named experimental option. Raw logs and recordings remain
outside the checkout.

These checks establish lifecycle and replay behaviour for delivered records.
They do not reconstruct an unknown number of undelivered frames, prove physical
accuracy, qualify live capture durability or establish Pi throughput. Stage-aware
native trail styling and final trajectory persistence remain separately tracked.
