# LiDAR scene-change recovery sprint

- **Status:** Proposed — export analysis complete; raw-capture experiments not run
- **Owner:** LiDAR pipeline maintainer; reviewer for capture labels and survey output to be assigned
- **Scope:** Ten engineer-days, starting when raw captures and baseline provenance are available
- **Layers:** L3 Grid, pipeline coordination, L5 lifecycle, L9 recording, public survey export
- **Canonical:** [Foreground and tracking](../lidar/architecture/foreground-tracking.md)
- **Experiment:** [Lombard–Laguna scene-change recovery](../../data/experiments/try/lombard-laguna-scene-change-recovery.md)
- **Related:** [Static-sensor nudge tolerance](static-sensor-nudge-tolerance-plan.md), [motion/static tuning](motion-static-parameter-tuning-plan.md)

## Outcome and decision

Prepared on 2026-10-03 from the Lombard–Laguna survey investigation.

Recover from a changed static scene without creating minutes of false road-user tracks, while
preserving real stopped pedestrians and vehicles. This sprint first identifies the trigger at
Lombard–Laguna 33:21, then implements and evaluates one bounded recovery strategy. A physical
sensor nudge is the leading explanation, not an established fact.

The sprint ends with either a candidate that passes the experiment's release gates or a documented
rejection with the remaining cause identified. Default-on recovery and regenerated public surveys
depend on those gates; completing implementation alone is insufficient.

## Evidence the sprint must explain

The checked-in [survey header](../../public_html/src/scenes/lombard-laguna/assets/part-000/header.json)
identifies build `0.5.1-pre43`, generated on 2026-10-01. The analysis used repository revision
`b687e89d5c8014e7fdf73a4d24d69906d86ee946`; the generation commit and full resolved run configuration
have not been recovered. Counts below refer to exported tracks, including tentative/coasting
tracks, rather than confirmed physical objects.

| Playback time | Source frame | Tracks | Unclassified | Speed at most 0.1 m/s |
| --- | ---: | ---: | ---: | ---: |
| 33:20.198 | 142195 | 9 | 3 | 6 |
| 33:20.398 | 142197 | 13 | 7 | 8 |
| 33:20.598 | 142199 | 58 | 51 | 52 |
| 33:20.998 | 142203 | 50 | 18 | 31 |
| 33:30.999 | 142303 | 26 | 1 | 22 |

All 13 preceding IDs survive the addition of 45 IDs in 0.2 seconds. The passing bus and older
tracks retain identity. All 11,440 exported frames have increasing timestamps and source IDs
advancing by two; the maximum exported interval is 0.218745 s. The event is inside one part and
one chunk. These observations argue against a full tracker restart or playback discontinuity.

New tracks initialise with zero velocity, so the initial speed count alone proves little. The
stronger evidence is persistence: between 33:23 and 37:30, tracks `9s1` and `9t2` each vary by only
5 cm in X and 5 cm in Y over 1,235 exported samples. Both become pedestrian-labelled tracks with
median width 6 cm and median speed 0.01 m/s. Raw geometry must still establish whether these are
static structures before they become labelled false-positive references.

The [capture index](../../tools/s2-archive/site-index.json) has a roughly 55-second interruption
between static fragments shortly after the burst. This is supporting model evidence, not
independent physical-motion truth. Its historical classifier version differs from the inspected
code. The site export deliberately retains every packet between the visit's outer bounds.

## Work packages

Effort is for one engineer, with brief independent label and design reviews. Data access delay
does not consume the ten-day implementation budget. S4 may proceed after S1 while S3 is evaluated.

| ID | Effort | Deliverable | Depends on | Exit condition |
| --- | ---: | --- | --- | --- |
| S0 | 1 day | Versioned case manifest, original export reproduction, raw-data availability check, review windows and reference labels | None | Capture digests, exact time mapping, baseline identity and unknowns recorded; no fresh-model substitution for the full prefix |
| S1 | 1.5 days | Bounded L3 diagnostics and repeatable replay comparison | S0 | Two equivalent baseline runs; foreground causes, model transitions, L4 clusters and L5 births can be joined by capture timestamp |
| S2 | 1.5 days | Cause decision and diagnostic recovery comparisons | S1 | Evidence accepts or rejects pose change; measured pose correction, model reset and model-fault explanations compared separately |
| S3 | 3 days | One default-off recovery candidate and lifecycle handling | S2 | Mechanism tests pass; recovery has bounded duration, explicit unavailable output and defined coordinate/identity behaviour |
| S4 | 1 day | Survey lifecycle/quality metadata and review display | S1 | Tentative, observed, coasted and recovering output are distinguishable; older exports still load |
| S5 | 1.5 days | Locked-parameter evaluation on reserved captures and target-device performance | S3, S4 | Every release gate scored, including stopped road users, valid coverage, speed and false recoveries |
| S6 | 0.5 day | Decision report and reproducible staged survey export | S5 | Candidate/configuration/provenance archived; decision is enable, keep experimental, or reject |

S0–S2 are the first four-day slice. If raw evidence does not support pose change, S3 addresses the
identified model-state or point-decoding fault instead. Do not spend the remaining sprint building
a pose compensator for an unconfirmed cause. If the fault cannot be isolated in S2, deliver the
instrumented reproduction and a narrower follow-up; report the issue as unresolved.

## Release placement

The [backlog](../BACKLOG.md) schedules independently deliverable slices across the release train:

| Release | Scope |
| --- | --- |
| v0.5.4 | S4 evidence schema and measured/display geometry contract |
| v0.5.5 | S0–S1 full-prefix reproduction and bounded diagnostics |
| v0.5.6 | S2 cause decision and diagnostic comparisons |
| v0.5.7 | S3 default-off recovery candidate |
| v0.5.11 | S5 reserved-case scoring and evidence archive |
| v0.6.6 | S4 playback integration and S6 staged survey refresh |
| v0.6.7 | S5 target-device gate and S6 promotion decision |

These are release placements, not seven separate ten-day sprints. The work-package dependencies
still apply: schema work can precede diagnosis, while recovery, promotion and public asset refresh
wait for their evidence gates. Staging in v0.6.6 may precede the Pi gate; publication waits for it.

## S1: expose the actual decisions

Extend existing [replay](../../internal/lidar/replayeval/replayeval.go) and
[observation-frame](../../internal/lidar/pipeline/observation_frame_tap.go) surfaces. The current
foreground-complete observation log preserves foreground returns and clustering disposition; it
does not contain every background return or all L3 cell decisions. Registration needs raw L2
returns, including the stable background.

Record full-rate frame summaries and bounded point/cell samples around the event:

- Capture time, source ordinal and sequence identity; point count, azimuth/ring coverage and
  packet/frame gaps. Do not identify a replay frame only by a process-local frame counter.
- Foreground fraction by azimuth/range, accepted-background fraction, frozen-cell fraction,
  confidence distribution, drift, region identity and settling state.
- For sampled returns: cell range/spread, locked baseline/spread, confidence, freeze expiry,
  recent foreground count, effective regional parameters and acceptance/rejection reason.
- Every grid reset, restore, region replacement, tuning change and calibration/pose change, with
  cause, old/new identifiers, capture time and wall time. Log absence as well as occurrence.
- Cluster births/splits, association outcomes, track confirmation, support, last observation and
  class changes. Keep inferred/coasted output distinct from measurements.

Bound diagnostic buffers and output size; fail an offline evidence run on missing records rather
than silently sampling away the burst. Snapshot shortcuts are eligible only after their restored
output matches a full-prefix replay. A snapshot must preserve cells, regions, parameters, timing,
freeze state and any required tracker state; an average-range image is insufficient.

## S2–S3: select recovery from evidence

The experiment compares unchanged processing, an independently measured pose correction, and a
known-time background rebuild before trying automatic detection. Known-time interventions are
diagnostic upper bounds and do not qualify an automatic recovery algorithm.

If pose change is supported, prefer the smallest recovery method that passes all gates. A
bounded background rebuild is the initial implementation candidate; pose compensation proceeds
only if its measured benefit justifies its coordinate and observability requirements. If the
raw scene is stable and model state changes instead, repair that transition and retain the same
regression case.

Required recovery contract:

1. **Healthy:** normal foreground extraction and tracking. Collect a recent stable reference.
2. **Suspect:** require spatially distributed static support or a verified model fault. A high
   foreground fraction or a passing bus alone cannot establish sensor motion. Record the decision
   inputs and abstain if registration is unobservable.
3. **Recovering:** stage a new model or apply a qualified correction. Continue recording and
   advance lifecycle on capture time. Describe unsupported intervals as unavailable; an empty
   frame during recovery must not imply that the street contained no road users.
4. **Healthy or unavailable:** resume only after measured validation on subsequent frames. On a
   timeout, remain explicitly unavailable, retain the failure reason and bound retries. Do not
   alternate indefinitely between resets or declare success because a timer expired.

For a rebuild, use persistence and independent static geometry to qualify background support.
Protect reviewed stopped-object regions and recent object occupancy; zero speed alone is never
permission to absorb a pedestrian or queued vehicle into the new background. Score subsequent
departure and reacquisition as well as the stationary interval.

Choose the coordinate contract before coding. Either map new measurements into a persistent
reference frame and transform all affected state consistently, or begin a new pose/model epoch,
end incompatible tracks explicitly and expose the coverage/identity break. Never compare a
new-pose measurement with an old-pose Kalman state to infer speed. Include epoch identity in
observations, recordings and exports; IDs must not imply continuity across an unqualified reset.

L3 classifies polar returns by ring and azimuth. Changing only L4/world coordinates cannot repair
the foreground decision. Pose compensation must reproject the reference into current sensor rays
or otherwise define valid L3 cell correspondence, accounting for occlusion and missing coverage.
Rotated XYZ points must not retain incompatible original ring indices.

## S4: make the survey output inspectable

The [exporter](../../internal/scene/export.go) excludes deleted tracks but discards lifecycle and
support fields. The [viewer](../../public_html/src/js/scene-player.js) draws all supplied tracks
with a minimum extent of 0.9 m. Both amplify the apparent certainty of the burst.

Add compatible lifecycle, support, last-observed time, model epoch and scene-quality fields.
Represent unavailable historical fields as unknown, never fabricated confirmation. Provide a
review view for tentative/coasted boxes and recovering frames; expose measured dimensions when
an object is inspected. Keep marker size distinct from measured geometry. A normal-view filter
may reduce tentative clutter, but evaluation always uses the complete upstream records.

Changing box size, hiding tentative tracks, raising hits-to-confirm or adding a minimum-speed
filter cannot satisfy the recovery gates by themselves. Persistent static clusters can become
confirmed; real road users can remain stopped.

## Release decision

The [experiment protocol](../../data/experiments/try/lombard-laguna-scene-change-recovery.md)
owns the metric definitions, proposed numeric gates, capture split and run commands. Freeze these
before candidate evaluation. Report false detections, valid coverage and true-object retention
separately; a weighted composite must not hide a regression.

Meaningful verification includes injected pose steps, fixed-pose traffic/occlusion, unobservable
registration, grid restore/reset, capture gaps and stopped-object departure. Verify capture-time
behaviour at different replay pacing, repeated runs, dense per-frame recording, lifecycle expiry,
schema compatibility and coordinate consistency. Run the affected Go/web package tests and the
repository's required checks for each implementation change; measure live-path cost on the Pi
before enabling it there.

The final staged Lombard–Laguna export must come from the selected code and full resolved tuning,
carry a source/code/configuration manifest, and preserve any recovery interval in its timeline.
Keep the original export as the baseline. Enable or republish only after a scored release
decision; a failed gate leaves the candidate experimental with a specific follow-up.

The broader [nudge-tolerance plan](static-sensor-nudge-tolerance-plan.md) still owns a general
centimetre/degree tolerance envelope. This sprint establishes one failure mechanism and tests
transfer to reserved examples; it does not claim a universal motion classifier or retune the
archive's site-joining rules.
