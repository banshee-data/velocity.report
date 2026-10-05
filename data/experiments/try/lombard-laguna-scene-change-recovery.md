# Experiment: Lombard–Laguna scene-change recovery

- **Status:** Proposed — measurements from the web export are available; raw replay not run
- **Owner:** LiDAR pipeline maintainer; independent reference reviewer to be assigned
- **Related plan:** [LiDAR scene-change recovery sprint](../../../docs/plans/lidar-scene-change-recovery-sprint-plan.md)
- **Layers:** L2 evidence, L3 Grid, L4 clustering, L5 lifecycle, survey export

## Goal

Proposed on 2026-10-03 following the Lombard–Laguna web-export investigation.

Identify why 45 new tracks appear within 0.2 seconds at Lombard–Laguna 33:20.598, and decide whether
bounded rebaselining, pose compensation or a model-state repair resolves the failure without
losing real stopped road users. The sprint plan holds the observed counts and implementation
tasks; this document holds the experimental procedure and acceptance gates.

## Hypothesis

H1: a sensor pose step makes static surfaces disagree with the learned polar background. A
qualified correction or a clean model for the new pose will remove sustained static foreground.

H2: raw geometry remains stable while a grid restore/reset, effective parameter change, region
replacement, freeze behaviour or point-decoding error creates the burst. Correcting that specific
transition will remove the false foreground without pose correction.

H3: the new clusters are caused mainly by local traffic occlusion or fragmentation. Support will
be local to the passing object, rather than a rigid change shared by separated static surfaces.

Track-ID continuity already argues against a full tracker restart. Class labels and the archived
motion classification are derived outputs and cannot independently establish any hypothesis.

## Method

1. **Recover provenance and map time.** Resolve the original pre43 generation revision and tuning
   if available. Save raw-file SHA256 values, ordered capture list, calibration, return mode,
   software revision and effective configuration/region parameters for every run. Retain the
   original web export with source VRLOG digest
   `a03832a9dbc1f9b3a85fc1f0d8f9e61ed36f82ab727736a936e8dd2c3d67abfa` and tuning digest
   `sha256:2319c8f8aaf95e70887ced03d7e9f42f631eabf82c0ac593d32feb1880ba2e3f`. If the original
   configuration is unavailable, name the replacement baseline and report the difference; do not
   claim an exact historical reproduction.
2. **Build references before scoring.** Review raw LiDAR sequences around the event, marking
   static structures, real moving and stopped road users, occlusion and uncertain regions. Use
   independent geometry and trajectories, not the current class labels or speed alone. Associate
   references across replays by capture time and geometry; web IDs such as `9s1` are local to one
   export. An independent reviewer checks every stopped-object reference and disputed label.
3. **Reproduce the unchanged pipeline twice.** Process the full capture prefix before the event,
   retain state, and score the same timestamp window. Compare frame order, L3 masks, clusters and
   associations after normalising random track IDs. Fix or quantify nondeterminism before ranking
   candidates. Run a separate pacing check; host wall-clock timing must not decide recovery.
4. **Locate the first changed stage.** Join raw geometry, sampled L3 decisions/model events, L4
   clusters and L5 births at capture time. Measure a robust rigid transform between pre-event
   background surfaces and successive raw rotations. Exclude moving/occluded areas and require
   separated static support, adequate overlap and observable axes. Compare with quiet-window
   residuals; validate the fitted transform on static patches excluded from its fit. Report pose
   uncertainty and abstain when a single plane or passing bus dominates the estimate.
5. **Run the diagnostic arms below.** Change one mechanism at a time, retaining the same input,
   prefix, labels and other parameters. Fit any diagnostic transform independently of the tracker
   score. A lower foreground count alone is not success: learning all road users as background
   would achieve it too.
6. **Implement and freeze one automatic candidate.** Specify its observable evidence, threshold
   units, confidence/abstention rules, maximum recovery time and coordinate contract. Tune only on
   the development case and controls. Save predictions and parameter hashes before reserved-case
   runs. Automatic decisions may use only frames available at the decision time.
7. **Score all gates and publish a decision record.** Evaluate the original output, complete new
   tracker output and rendered survey separately. Save failures and per-case results. Do not
   change thresholds after seeing reserved-case results and still describe those cases as held out.

### Comparison arms

| Arm | Intervention | Question answered |
| --- | --- | --- |
| A0 | Unchanged pipeline, complete prefix | Does this run reproduce the event and how variable is it? |
| A1 | Apply independently measured pose correction at the known event, with valid L3 ray correspondence | Does correcting pose remove static leakage? Diagnostic upper bound only |
| A2 | Start a new background epoch at the known event; protect labelled object occupancy and account for unavailable time | Can rebaselining recover clean geometry, and what coverage/identity does it cost? Diagnostic upper bound only |
| A3 | Cause-specific repair with the sensor pose untouched | If H2 is supported, does the identified state/decoding correction remove the event? |
| A4 | Automatic detection plus the best supported recovery/repair | Can the mechanism work causally at unknown event times and pass reserved cases? |
| V1 | Same recorded tracks, distinguish/filter tentative and coasted display | How much is presentation? Never evidence that L3 recovery works |

Run A1 only if registration is supported and A3 only if a concrete fault is found. A2 may show
that a new model helps under either H1 or H2; it does not by itself prove a physical nudge. If no
diagnostic arm explains the burst, stop candidate selection and retain an unresolved-cause result.

### Cases and split

| Case | Role | Scored evidence |
| --- | --- | --- |
| Lombard–Laguna complete visit | Development and diagnosis | Main event, stationary structures, passing bus/pedestrian, five-minute recovery tail; retain full prefix |
| Lombard–Laguna earlier quiet and busy windows | Development controls | Same sensor/background ageing without the burst; queued/stopped objects and bus occlusions |
| Van Ness–Sacramento documented nudge visit | Reserved physical transfer case | Independently measured nudge times/magnitudes; stationary intervals and genuine road users |
| One different-day static visit from the archive, selected and recorded before tuning | Reserved negative control | Busy traffic, long stops and no independently observed sensor motion |
| Reviewed stop-to-drive transition adjacent to a capture | Reserved out-of-scope-motion control | A sustained drive must remain unavailable or form a new site epoch rather than repeatedly appear recovered |
| Injected pose steps and no-motion counterexamples on stable geometry | Mechanism tests | Known translation/rotation, stopped-object protection, registration degeneracy and clock behaviour |

The existing [site index](../../../tools/s2-archive/site-index.json) and
[operator joins](../../../tools/s2-archive/site-joins.json) select candidates, not ground truth.
Choose exact reserved windows and seal their labels before tuning. Include at least three
independently verified reserved pose changes and at least 60 minutes of reviewed fixed-pose
controls. If the archive cannot supply that evidence, report the gate as unevaluated and schedule
a controlled nudge capture; do not count synthetic trials as physical validation.

Begin synthetic mechanism tests with translations of 2, 5 and 10 cm and rotations of 0.1, 0.5 and
1 degree, plus repeated steps and a continuous ramp. Adjust the envelope to include measured
physical changes before locking evaluation. Synthetic transforms need consistent range/ray
geometry; they are not substitutes for changed visibility in real captures.

## Inputs and setup

Raw captures are required for this experiment and are absent from the investigation workspace.
Use a supplied local archive path and manifest. The planning work needs no remote corpus search.
The relevant source file is `s2_sf_2_20260902112832_00025.pcap`, preceded by the capture sequence
beginning with `s2_sf_2_20260902105830_00019.pcap`. The site index supplies all eight files in order.
A verified merged site PCAPNG is suitable; preserve packets through classifier-motion intervals.

The exported start is `1788371949825003330` ns UTC. Thus:

- Export burst offset: `2000.598285` s, source frame `142199` in this recording.
- Absolute burst time: approximately `2026-09-02T18:32:30.423288Z` (11:32:30.423288 PDT).
- Relative to the indexed site start: approximately `2040.164472` s. This is **not** the export
  offset; the export dropped its opening transient.
- Detailed diagnosis: export offsets 1970–2065 s, with dense raw/L3 evidence for 1995–2010 s.
- Recovery scoring: event through 300 s later, clipped to actual capture end and recording the
  resulting duration. In the present export only about 287.2 s remain after the event.

Compute replay offsets from the actual first packet timestamp, not the filename or rounded site
start. Run on a workstation with Go/libpcap and enough local output space; measure runtime cost
separately on the target Pi. Keep output on a different device from the raw capture when possible.

The existing CLI supports the following baseline command after the case manifest provides the
paths and offsets. These environment values are deliberately supplied by that manifest, not
invented for this workspace:

```bash
make build-velocity

# LL_START_SECONDS is the actual first-packet-relative offset of diagnosis start.
# LL_DURATION_SECONDS reaches the end of the scored tail.
# Equal warmup/start values process the entire prefix from capture offset zero.
./velocity lidar pcap-replay \
  --pcap "$LL_PCAP" --config "$LL_CONFIG" \
  --sensor-id hesai-pandar40p \
  --start-seconds "$LL_START_SECONDS" \
  --warmup-seconds "$LL_START_SECONDS" \
  --duration-seconds "$LL_DURATION_SECONDS" \
  --require-settled --include-points --include-debug \
  --replay-case-id lombard-laguna-scene-change \
  --output "$LL_RUN_ROOT/baseline-a" \
  --observations "$LL_RUN_ROOT/baseline-a-observations"

./velocity lidar observations verify --require foreground-complete \
  "$LL_RUN_ROOT/baseline-a-observations"
```

Repeat with new `baseline-b` output/container paths. Full-prefix observation logging can be large:
the current observation tap includes warm-up. Budget that space, or add an explicit diagnostic
recording window without skipping model processing. The `include-points` output and observation
log are not a complete pre-L3 raw cloud; the bounded raw/L3 instrumentation in S1 is additional
work. Automatic recovery flags and complete-state checkpoint restore are also proposed work,
not existing CLI capabilities.

Do not run a second settling pass over future scored traffic. A fresh model at 33:20 and a model
that has processed the previous 33 minutes answer different questions. Recovered checkpoints may
accelerate comparisons only after equivalence to full-prefix replay is demonstrated.

## Success criteria

These are **proposed engineering gates**, not measured outcomes or statistically established
tolerances. Freeze them with the case/label manifest before A4 evaluation. Report counts and
denominators per case, including uncertain labels, excluded regions and unavailable intervals.

Define false-track time as the sum of capture-time intervals occupied by tracks independently
matched to labelled static structures. Count simultaneous false tracks separately. Report it for
all tracks and for confirmed tracks, with the same timestamp/reference domain in every arm.
Define valid coverage as valid perception time divided by eligible captured time; suppression,
recovery and dropped output reduce coverage. Define recovery time from independently established
event onset until five continuous seconds of valid output meeting the static-leakage criterion.
If baseline false-track time is zero, report the reduction as not applicable and require the
candidate to remain at zero. Label uncertainty is reported separately rather than forced into
either the false-positive or real-object denominator.

- [ ] **Reproduction:** two baseline runs agree on scored timestamps, L3 masks and normalised
      decisions, or any discrepancy is explained and bounded before comparing improvements.
- [ ] **Cause:** H1, H2 or H3 has a reproducible evidence chain from raw input/model transition to
      L3 foreground, L4 clusters and L5 births. A label or lower rendered-box count is insufficient.
- [ ] **False detections:** at least 90% reduction in confirmed false-track time over the scored
      recovery tail, both on Lombard–Laguna and each reserved nudge case; no reviewed static
      exemplar becomes a false confirmed track lasting more than 10 s after the recovery budget.
- [ ] **Recovery and coverage:** recovery within 10 s of each qualified pose step, with at least
      95% valid coverage over the available five-minute tail. Recovery time counts the five-second
      validation interval. Report the first minute separately so a short outage remains visible.
- [ ] **Real objects:** no additional completely missed labelled road user; per-frame matched
      recall decreases by at most one percentage point per case. Stopped objects remain present
      outside explicitly unavailable frames and are reacquired within 1 s of valid processing
      resuming. Report recall both over the full eligible timeline and conditional on valid output.
- [ ] **Identity:** no added identity switches outside declared recovery epochs. Report every
      epoch-induced termination/rebirth separately; it is a cost, not an excluded success.
- [ ] **Speed:** no pose-transition sample is published as a valid speed measurement. On matched
      unaffected vehicles, per-object maximum-speed differences versus baseline have p95 at most
      0.5 m/s; report outliers. Absolute speed-accuracy claims require an independent reference.
- [ ] **False recovery:** zero false recovery events across at least 60 minutes of reviewed
      fixed-pose controls, including bus occlusion and stopped traffic. State the exposure; this
      sample does not establish a deployment-wide false-alarm rate.
- [ ] **Runtime:** no added dropped rotations; normal-path p95 processing cost and steady memory
      increase by at most 10% on the target device, and existing frame-budget/performance gates
      pass. Report recovery-path p95/p99 cost, peak memory and diagnostic overhead separately.
- [ ] **Export:** one timestamped record per intended rotation, explicit quality/lifecycle/epoch
      information, old-format reader compatibility, and a reproducible public export with the
      original data retained for comparison.

A reserved-case regression or missing stopped-object reference prevents default-on acceptance.
Satisfying the gates only by hiding boxes, suppressing long intervals, resetting repeatedly or
absorbing stopped objects fails the experiment.

## Risks and controls

| Risk | Control |
| --- | --- |
| A fresh model erases the failure | Carry the entire prefix; verify any complete-state checkpoint |
| Foreground/motion labels make the diagnosis circular | Use raw static geometry and reviewed physical-object references |
| Regional overrides make a parameter sweep ineffective | Record effective per-cell/region values and restoration history |
| Pose registration follows a bus or is geometrically ambiguous | Hold out static patches, test observability, and abstain |
| Rebaselining learns stopped road users | Protect object occupancy; score stopped intervals and departure |
| Apparent improvement comes from missing output | Count unavailable time, retain dense frame records and score full-timeline recall |
| Thresholds overfit the one known timestamp | Lock configuration before reserved events; score automatic onset detection |
| A clock or schema change corrupts comparison | Use capture timestamps, explicit frame/epoch identity and versioned provenance |

## Outputs

Under the supplied local run root, retain a case manifest, raw-source digests, configuration and
code hashes, labels with uncertainty, raw diagnostic windows, per-frame L3/model-event records,
foreground-complete observations, full tracker records, registration residuals/uncertainty,
per-case scorecards, runtime measurements and the staged web export. Preserve failed runs.

Check a compact decision report and plots into the owning operations documentation when run;
keep large captures/recordings in the evidence store. The report must say which hypothesis
survived, which arm was selected, every gate's result and the measured limits of applicability.

## Result

Not run. The web-export burst and persistent narrow tracks are established. The physical trigger,
recovery performance and target-device cost remain unmeasured.

## Next action

S0: bind the local raw captures and baseline provenance, mark the diagnosis/control windows, and
review the static and stopped-object references. Then run unchanged processing twice before any
recovery intervention. Promote a completed experiment to `data/explore/` with its decision report,
or retain this proposal with a precise rejection/blocker and revised question.
