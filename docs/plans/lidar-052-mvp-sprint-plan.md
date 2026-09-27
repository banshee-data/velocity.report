# 0.5.2 sprint: physical estimates and inspectable following distance

This sprint connects the merged estimation and headway components into one replayable product:
a physical trajectory, a following measurement, and a distance layer that explains every number.
The first exit is a working provisional MVP; field promotion requires separate evidence.

- **Status:** Planned; documentation only in this review
- **Canonical:** [LiDAR pipeline reference](../lidar/architecture/lidar-pipeline-reference.md)
- **Scope:** Remaining 0.5.2.0–0.5.2.4 work after #596–609 and #611
- **Acceptance platform:** macOS on Apple Silicon; stationary, single-sensor captures first
- **Review:** [merged-batch audit and fresh evidence](../lidar/operations/0.5.2-sprint-review.md)
- **Owners:** [state estimation](lidar-state-estimation-plan.md), [behaviour analytics](lidar-behaviour-analytics-plan.md), [VRLOG](lidar-vrlog-observation-format-plan.md), [visual design](../ui/DESIGN.md)
- **Task ledger:** [0.5.2 backlog](../BACKLOG.md#v052---tailgating-metric--distribution-052)

## Sprint outcome and scope

For a selected recorded scene, an operator can replay a named estimate version, select a follower,
see the nearest credible object ahead and the distance between their inferred physical ends,
inspect the ingredients, and open the corresponding encounter and distribution. Unsupported
measurements say why they are missing. The PDF, API, chart and visual layer agree because they
read the same versioned analysis.

Use the existing planar Cartesian CV filter with temporal length/width and orientation beliefs.
Add the face-aware, variable-rank measurement and calibrated uncertainty before changing the
motion model. Keep pedestrian and cyclist continuity tests, but restrict the first field following
report to independently reviewed rigid-vehicle pairs or classes meeting the declared applicability
gate. A partial observed span is a lower bound, not a measured full length.

| Exit                      | Required result                                                                                                                                                                                                     | What it permits                                                                                                  |
| ------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| A: integrated MVP         | Correct anchor semantics, replayable body trajectories, real PCAP-to-interaction persistence, stage-aware distance inspection, scene distribution and real-data report; deterministic accounting and recovery tests | Internal field review with visible `provisional` status. Failed physical gates and unknown truth remain visible. |
| B: qualified 0.5.2 result | G-GEO-1, G-UNC-1, G-SMO-1 and the pinned following-metric gate pass on reviewed held-out data for a stated operating envelope                                                                                       | Promotion for that envelope only. Passing one placement does not establish multi-site or moving-sensor validity. |

Aim to complete A and gather B's evidence this sprint. Do not promise B before the references and
measurements exist, and do not relabel A as completed physical validation. A `final` estimate may
still produce a provisional product; inference finality and validation status are separate axes.

No CA/CTRV/IMM, lane map, global scene graph, shell catalogue, mobile ego-motion or new composite
driver score is needed. Complete the offline Mac path before expanding live deployment. The
independent live L5 worker, bounded reassociation experiment, wider corpus and Pi/power-loss work
remain agreed backlog items, with the scope split below; they are not silently retired.

## Delivery order

Each row is an independently reviewable implementation increment. The sequence is the sprint's
critical path, not an assertion that all packages need rewriting.

| Order | Deliverable and likely owner                                                                                                                                                | Completion evidence                                                                                                                                                                                                          | Dependency                                         |
| ----- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------- |
| S0    | Repair the evidence path: #611 coverage refusal and PCAP test contract; investigate the VRLOG shed test; freeze source/build/config identities and reviewed reference split | Hydrated-PCAP test actually executes; invalid coverage refuses; valid coverage has a working input; frozen split and reference digests, documented unresolved truth                                                          | First; reference review continues throughout       |
| S1    | Honest body anchors and observation support in L5/L8 adapters                                                                                                               | Medoid, partial OBB, physical centre and near-face offset have distinct semantics; support tokens and acquisition times survive a round trip; unsupported geometry cannot acquire a physical label by changing `stage`       | S0                                                 |
| S2    | Corrected CV/body update from immutable L4 evidence                                                                                                                         | Full member geometry, calibrated sensor pose, face attribution, rank-1/rank-2 measurement and covariance feed association, seeding and update consistently; body beliefs retain provenance and ambiguity                     | S1                                                 |
| S3    | Calibrated time, association and uncertainty                                                                                                                                | Eligible pre-gate residuals by measurement rank; measured Q/R and coast limits; choose existing likelihood/cascade/extent options on tuning data; empty/gap frames advance lifecycle; held-out identity and manoeuvre checks | S2 and reviewed tuning data                        |
| S4    | Persisted refined body trajectory and publication                                                                                                                           | Fixed-assignment horizon comparison, full body records at each stage, coast/gap records, observation references, version identity, revision audit, complete-run marker and restart-safe finality frontier                    | S2–S3; work on storage contract can begin after S1 |
| S5    | Real following analysis and storage                                                                                                                                         | Qualified trajectory reader calls the existing `AnalyseFollowing`; persist path geometry, follower decisions, pair instants and unique opportunity; scene window clipping and exact run selection; idempotent regeneration   | S4                                                 |
| S6    | Following distance layer and inspector                                                                                                                                      | Same stored instant gives the same endpoint, gap, speed and reason in API and renderer; select, step, seek and source/version changes tested; optional causal preview clearly distinguished                                  | S5; synthetic UI fixtures can be prepared after S1 |
| S7    | One report/distribution path and field decision                                                                                                                             | Real-data PDF/ZIP and scene API consume one aggregation contract; held-out endpoint, gap, pair choice, interval coverage and opportunity report; gate ledger records pass/fail/insufficient evidence                         | S5–S6                                              |

### S0: make the next experiment trustworthy

- Use the exact solver in both arms. Rerun D2 medoid versus OBB centre and the corpus baseline
  before quoting a historical accuracy improvement; retain both historical and new manifests.
- E1.2/E1.4 already ran on Columbus. Refresh them where tracker grouping or calibration changes,
  rather than reimplementing the experiments. Their purpose is diagnosis, not gate closure.
- Freeze object-disjoint tuning and held-out episodes. Also separate time intervals where possible;
  dense frames of the same object are correlated samples. Review identity and visible geometry
  independently of tracker IDs. Do not certify an unseen bumper from a partial point mask.
- Prepare explicit sensor coverage/origin for continuity experiments and support unknown coverage
  by refusing the claim. Test exit, re-entry, hidden stop, hidden turn and static occluders.
- Resolve the shed-frame test's wait/completion assumption under measured storage latency. A
  dropped frame must remain an explicit gap even when a test's expected frame count was wrong.

### S1–S3: the smallest physical state model

Persist a dynamic state layout identifier separately from the estimator and observation-model
identities. The first layout remains `[x, y, vx, vy]`, with a full covariance; temporal body
beliefs carry length/width, uncertainty, admissible support and provenance. Orientation remains a
belief with uncertainty and front/rear ambiguity, not velocity heading by fiat. The reference
point and any anchor-to-centre offset must be explicit in the same coordinate frame.

`MeasureNearEdge` already exists. Its missing integration must respect rank: a single supported
face constrains one normal, leaving the other direction to prediction.
Do not turn the reconstructed
centre into a fictitious two-dimensional observation. Keep the measured plane, supporting members,
face, extent prior and resulting correction available for inspection. Use complete profile members
where required; a capped visualiser/SQLite sample must not be labelled complete.

Admit extent evidence only under the existing observability/truncation rules. Keep class-prior and
accumulated extents distinct; a history of partial visible spans does not automatically establish
both bumpers. Near-zero speed, unresolved axis sign, merged/split clusters and poor geometry must
remain explicit degradation states. Preserve meaningful stops, turns and braking.

Before extending coast duration, compare the existing likelihood cost and confirmed-first cascade
against the default and tune extent compatibility. #600 solves the assignment optimisation;
it does not remove uncertain-track bidding bias. Propagate capture gaps, out-of-order time and
empty frames explicitly, without adding repeated observations. Pin the common sample time for
each pair: measurement-time tracking must not join two differently timed states as simultaneous.

### S4: finality and recovery for the offline MVP

Start with a bounded local reader of a closed, verified observation log on the Mac. This provides
the same observation/estimator boundary required by the independently scheduled live worker,
without making concurrent live capture a prerequisite for a recorded-scene result. The delivered
RTS experiment remains the baseline; compare three frames, 0.5 s, 1 s, 2 s and whole-track output.
Choose the shortest accepted horizon, not whichever trail looks smoothest.

In #609's group-commit mode an append is admission, not a durability acknowledgement; the current
direct tracker continues after that append. The new reader must consume only the verified
committed prefix, and publication must never claim a final interval beyond that input frontier.

The trajectory record must add the body fields missing from `TrackEstimate`, retain support and
last-observed age, and name the observations or prediction interval behind a state. Preserve
coasted states without inventing an observation ID. A memory-cap release is `fixed_lag`, never
silently `final`; mark truncation and incomplete capture tails. Record which body geometry was
used at each stage rather than attaching the newest extent to old positions without a revision.

Publish a complete immutable analysis version only after trajectories, paths, interactions and
accounting are committed and cross-checked. Before that, expose an explicitly incomplete review
version. A restart either resumes at a verified cursor or produces a new version; it must not mix
old/new intervals or duplicate exposure. Use source-scoped stable trajectory identity or a
deterministic creation-sequence mapping for regeneration; random per-replay track UUIDs alone do
not make repeated interaction writes idempotent.

Keep captured, processed and final frontiers separate. A finality frontier advances only over a
contiguous accounted prefix; an unclassified gap or clock discontinuity cannot be skipped. EOF,
user stop, storage failure and a fully processed capture have different completion states.

Bounded reassociation remains a measured comparison after this baseline. If fixed assignment fails
identity acceptance, revisit it before promotion; its absence is not permission to relax identity
thresholds. Live scheduling, backlog isolation and source-boundary rollover remain separate work
under the [decoupling plan](lidar-cluster-observation-log-and-async-tracking-plan.md).

### S5 and S7: one explainable measurement population

The metric is **spatial bumper gap and net time gap** on one directed empirical path. For a
resolved pair at one capture instant, `g = s(leader rear) − s(follower front)` and
`net_time_gap = g / follower_along_path_speed`. Use the existing projected footprint extent for
oblique bodies, rather than subtracting half-length regardless of orientation. A non-positive gap
is a geometry-review condition; a speed below the declared floor suppresses time gap but may leave
a reviewable spatial gap. These are not front-to-front passage headway or time-to-collision.

Persist the fitted path and its knots/members, all candidate decisions, physical endpoints and
their sources, uncertainty components, method parameters, version, support and suppression.
Free flow, no path, ambiguity and a missing record must survive even when no encounter was formed.

Compute aggregate opportunity once per follower interval, split at leader changes and scene
boundaries. Pair-level accounting can overlap during ambiguity; scene/follower exposure must not.
Clip intervals and recompute windowed statistics rather than selecting only wholly contained
events or copying a whole-encounter minimum into a shorter window. Distinguish valid spatial-gap
time, valid net-time-gap time, predicted-only time and missing evidence, with the denominator
printed on each distribution. Retain the current shared valid-following denominator as a named
version until any intentional population change is versioned and tested.

Connect the real-data report to persisted analysis and consolidate its aggregation with the scene
API. Never let the PDF and histogram use different clipping, eligibility or version-selection
rules. Pin source/run identity explicitly; a capture-time overlap or containment query alone is not
capture provenance. Surface unfinished analysis and failed gates separately from `no_encounters`.

## Following distance debug layer

Add a **Following distance** toggle alongside **Velocity** in the macOS visualiser. It is a
measurement layer for this sprint, with the scene page providing the same encounter/instant
inspection and a route back to the selected tracks. General future ghosts and uncertainty cones
remain v0.5.4 work. Follow
[DESIGN](../ui/DESIGN.md#45-following-distance-inspection) for presentation.

### What the layer draws

At the displayed capture timestamp and selected estimate version:

1. Highlight the follower and its nearest credible same-path leader. Draw their inferred footprints,
   the follower's leading endpoint and the leader's trailing endpoint, the local path segment
   between the projected endpoints, and a distance label in metres. On a curve, draw the measured
   path arc; a straight chord must not carry an arc-length label without distinction.
2. Label the main value **Distance ahead**: the smallest supported positive along-path bumper gap
   to an eligible object ahead. Use the backend's candidate/ambiguity decision. Do not choose the
   smallest noisy number among overlapping candidate uncertainty ranges. Adjacent-lane, crossing,
   oncoming and behind objects do not become a leader merely because they are nearby.
3. For literal geometric inspection, allow **Minimum surface separation** between the selected
   pair's planar footprints as a separate diagnostic, with the closest boundary points joined.
   This is the shortest two-dimensional surface distance; it can differ from the along-path gap
   on a bend or with lateral offset. It must not replace the registered headway value or its
   denominator. State the road-plane assumption and body uncertainty; no unsupported 3D claim.
4. Use solid marks for supported observations and dashed/hollow marks for temporal or coasted
   inference, with text identifying the source. Show uncertainty and coast age. Predicted-only
   distance is review-only and contributes no observed exposure.
5. On ambiguity, show candidate/rejection information and **No reliable leader**. On insufficient
   geometry, retain the reason and available evidence. No numeric zero or distant replacement
   leader should disguise a blocked or unknown nearest object.

For dense scenes, default to the selected follower; an optional all-followers view uses the same
eligibility rules and bounded draw budget. Skipping labels for rendering performance must not
change stored measurements. A visible layer legend carries `online`, `fixed_lag` or `final`, plus
the independent provisional/promoted product status.

### Individual measures in the inspector

| Group         | Values and provenance to inspect                                                                                                                              |
| ------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Identity/time | Follower and leader IDs, capture/frame time, acquisition times and synchronisation policy, source/run/version, coordinate frame and calibration               |
| Body          | Reference point and offset, length/width with sigma or bounds, heading and ambiguity, observed faces and supporting points, extent provenance and convergence |
| Geometry      | Path ID, tangent/corridor, centre arcs, projected half-extents, front/rear arcs, gap and endpoint uncertainty, optional closest-surface segment               |
| Motion        | Follower along-path speed and uncertainty, speed floor, net time-gap calculation and suppression                                                              |
| Selection     | Every relevant candidate, leader decision, exclusion reason, overlap/ambiguity margin and path-condition failure                                              |
| Evidence      | Observed/coasted/occluded/missing state, coast age, estimation lifecycle, stage, revision amount and lineage, finality/completeness watermark                 |
| Exposure      | Interval duration, valid/opportunity membership, named-band membership, suppression counts and the owning encounter                                           |

The backend calculates these quantities once. Extend the protocol/API with typed, versioned
following-debug records and persist enough to reopen them. Do not recompute distance from rendered
OBB boxes in Swift or JavaScript. A historical frame must use that version's geometry and state,
not the current track's body dimensions. Public exports must retain the product status and avoid
inventing public absolute-time or identity requirements beyond existing policy.

### Display and interaction acceptance

- Straight and curved analytic cases reproduce independently known endpoint, arc-gap and minimum
  surface-distance values; oblique and offset bodies distinguish the two distances.
- Selecting an encounter selects both tracks and the same instant in the layer; stepping or
  scrubbing updates the inspector and line together. No interpolation across missing support.
- Standstill shows spatial evidence with time gap suppressed; coasted states are visibly predicted;
  a lane-adjacent distractor, crossing object or ambiguous pair cannot display a confident leader.
- Seek, replay restart, source change, revised history and version switch clear stale pair geometry
  atomically. Out-of-order debug messages cannot resurrect a previous frame's line.
- Displayed values match stored/API values at selected real-PCAP instants. Numeric agreement is
  checked alongside screenshots/manual review; an attractive screenshot alone is not a test.
- Missing capability on legacy recordings produces an explicit unavailable state. Rendering cost
  and frame drops are measured on the acceptance Mac with the layer on and off.

## Evidence and promotion ledger

Do not change the established G-GEO-1, G-UNC-1 or G-SMO-1 thresholds to accommodate this sprint's
result. Historic `0.316 m` and `11.3 %` figures remain provenance, not a substitute for a current
paired baseline using the corrected solver. Define the evaluation population before running.

| Gate                           | Current state                                                             | Remaining evidence                                                                                                                                                                                                    |
| ------------------------------ | ------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| G-PER-1 / observation fidelity | Desktop components and PCAP round trips exist; full persistence gate open | Declared-profile multi-site repeat/round trip, ordered gaps and recovery; keep the week-of-live requirement and hardware qualification separately tracked                                                             |
| G-GEO-1                        | Near-edge candidate exists; no production wiring or held-out pass         | Corrected physical update; at least 50% lateral p99 reduction, excursion fraction <4%, detection/fragmentation limits and at least 90% real lateral-manoeuvre magnitude, plus independently observable body endpoints |
| G-UNC-1                        | Scalar filter noise; diagnostic geometry covariance; not passed           | Eligible pre-gate rank-1/rank-2 NIS, range/support/aspect calibration, genuine-manoeuvre rejection <1%, recovery and empirical state/endpoint coverage                                                                |
| G-SMO-1                        | Existing RTS fails revision bound on current kirk0                        | G-GEO first; at least 20% further lateral p99 improvement, at least 85% peak manoeuvre preservation, per-frame revision p99 <0.3 m with larger revisions flagged, identity non-regression and horizon latency         |
| Following metric gate          | Analytic/scoring harness delivered; no field pass                         | Digest-pinned independent body/pair references; endpoint/gap error, correct leader, expected strata, missing/suppressed opportunity and uncertainty coverage; tune none of these on the held-out set                  |
| Product integration            | Library/report/API pieces exist; no producer or distance layer            | One version from PCAP through persisted body/path/interaction to debug layer, histogram and real report; idempotent restart, clipping and denominator agreement                                                       |

Before looking at held-out following results, freeze numeric endpoint/gap-error limits, nominal
coverage and allowed coverage error, minimum independent encounters per stratum, maximum missing
and suppressed opportunity, and wrong-leader tolerance. Choose these against the display/report
resolution and independently measurable reference precision on tuning data. This review does not
invent field-calibrated limits. A missing threshold or unobservable bumper makes the gate
**insufficient evidence**, never pass.

### Evidence runs to schedule

1. **kirk0:** reproduce the audit's stamped medoid/RTS run and the corrected-solver D2 comparison.
   Use the reviewed split for identity; use only independently supported geometry for endpoints.
2. **Columbus, Marina and Embarcadero:** repeat evidence with source manifests and exact capture
   ordering. Compare immutable observation digests separately from track-linked results. Run Q3,
   the corrected measurement, association and uncertainty arms one change at a time; reserve the
   declared gate placement/objects from tuning.
3. **Following reference episodes:** straight following, partial front/rear view, oblique body,
   bend, lane-adjacent distractor, leader switch, close ambiguous pair, standstill and record gap.
   Record independent leader/no-leader truth and geometry uncertainty. Include genuine braking and
   lateral manoeuvres so smoothness cannot win by removing the event.
4. **Lifecycle and accounting:** empty frames, full occlusion, hidden stop/turn, exit/re-entry,
   split/merge, duplicate/backward timestamps, clock boundary, cap release, end-of-capture, worker
   interruption, seek and version replacement. Check every follower interval exactly once.
5. **Qualified live follow-through:** independent worker pause/restart, capture rollover and L2
   queue drops; target-storage power loss and Pi resource checks before changing live defaults.

Every evidence bundle records source digests, build SHA, calibration, extraction profile, parameter
and method hashes, split/reference digests, warm-up and scored intervals, frame counts, skips,
failures and hardware. Store results and failure examples beside the gate decision. Neither
accepted-only NIS nor repeatability is accuracy. Reusing the same PCAP is permitted; reusing a
tuning episode as supposedly unseen truth is not.

## Work that stays outside the first MVP exit

| Work retained                                                                                | Placement and condition                                                                                                                           |
| -------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------- |
| Independently scheduled live L5 worker and backlog isolation                                 | Remaining 0.5.2.2 live increment; the MVP first consumes a closed verified log locally. Required before claiming pause-independent live analysis. |
| Bounded reassociation, revisable point ownership and shape hypotheses                        | Remaining 0.5.2.2 experiment; promote only if fixed-assignment identity/geometry results require and justify it.                                  |
| Full live persistence week, rollover, upstream gaps, power-loss and Pi capture qualification | Remaining capture/deployment evidence; no live-default promotion on desktop process-kill tests alone.                                             |
| Pi estimator throughput optimisation                                                         | v0.6.7; does not block offline Mac correctness.                                                                                                   |
| Generic ghost trails, future cones and richer diagnostic comparison UI                       | v0.5.4; basic stage/support correctness and the Following distance layer are pulled into this sprint.                                             |
| PET, passing clearance and other behaviour metrics                                           | v0.5.3 or their existing plans; the optional pair surface-distance diagnostic is not a promoted passing-clearance metric.                         |

If time runs short, finish an honest provisional recorded-scene result with every failing gate
named. Keep field promotion, live operation and remaining experiments open in the backlog. The
scope of the evidence is part of the result.
