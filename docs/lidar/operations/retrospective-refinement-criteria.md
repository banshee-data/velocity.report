# Retrospective refinement: predeclared criteria

The criteria a refinement horizon, and G-SMO-1, must meet, pinned before any held-out episode is
scored; and the label-free evidence gathered so far.

- **Status:** Predeclared. Fixed-assignment RTS arms implemented and measured on synthetic scenes
  and kirk0; held-out identity and geometry scoring, the abnormal-motion set and the
  revisable-association arm are outstanding
- **Layers:** L5 Tracks, L8 Analytics, storage
- **Related:** [State-estimation plan §10](../../plans/lidar-state-estimation-plan.md#10-retrospective-refinement),
  [asynchronous tracking §2–3](../../plans/lidar-cluster-observation-log-and-async-tracking-plan.md#3-revision-window-late-data-and-finality),
  [shared VRLOG §5](../../plans/lidar-vrlog-observation-format-plan.md#5-joint-estimates-revisions-and-restart),
  [tracking maths §11](../../../data/maths/tracking-maths.md#11-retrospective-refinement),
  [per-frame evaluation](per-frame-evaluation.md)
- **Code:** [smoother](../../../internal/lidar/l5tracks/smoother.go),
  [filter steps](../../../internal/lidar/l5tracks/filter_steps.go),
  [replay harness](../../../internal/lidar/replayeval/refinement.go),
  [metrics](../../../internal/lidar/l8analytics/refinement.go),
  [lidar-refinement-eval](../../../cmd/tools/lidar-refinement-eval/main.go)

## Declaration

Everything in [Criteria](#criteria) and [Selection rule](#selection-rule) was fixed before any
refined arm was scored against reviewed episodes. A threshold changed after a held-out result has
been seen makes that comparison worthless. A change is therefore an amendment: dated, with its
reason, recorded under [Amendments](#amendments) before the next held-out run, and never applied
to a result already produced.

## Arms

One replay runs the shipped tracker. Its filter record (prior, posterior and the interval the
filter actually predicted across, per track per frame) feeds one smoother per horizon, so every arm
refines the same states under the same associations and every comparison is paired state for
state.

| Arm         | Stage       | Look-ahead                         | Version key                                            |
| ----------- | ----------- | ---------------------------------- | ------------------------------------------------------ |
| Online      | `online`    | None                               | `cv_kf_v1`, the replay's parameter hash                |
| 3 frames    | `fixed_lag` | Three subsequent steps: comparator | `cv_kf_v1+rts_fixed_assignment_v1`, a hash per horizon |
| 0.5, 1, 2 s | `fixed_lag` | That much subsequent capture time  | As above                                               |
| Whole track | `final`     | To the track's end                 | As above                                               |

A revisable-association arm (alternative assignments, point ownership and shape belief re-solved
inside the window) joins this table as further rows; see
[Unresolved alternatives](#unresolved-alternatives).

## Evidence sets

| Set                        | What it can decide                  | Status                                                    |
| -------------------------- | ----------------------------------- | --------------------------------------------------------- |
| Synthetic scenes           | Manoeuvre and impact preservation   | In `smoother_test.go`; truth is the scene's analytic path |
| Label-free replay          | Revision size, consistency, latency | kirk0 below; corpus via `EVIDENCE_FLAGS`                  |
| Held-out reviewed episodes | Identity and geometry               | Needs reviewed episodes and a split manifest              |
| Abnormal-motion set        | G-SMO-1 criterion 3 on real data    | Does not exist yet                                        |

A label-free figure is never accuracy. A lower residual or a smaller speed step can come from
flattening the motion it was meant to measure (plan §17.1).

## Criteria

Identity is judged first. **A lower position residual, or any other geometry gain, cannot excuse
an identity regression**: an arm that fails an identity criterion is out, whatever else it does.

A screen row decides whether a horizon is worth scoring at all, on synthetic scenes whose truth is
known. Identity, geometry and manoeuvre rows decide on real data which horizon is chosen. Promotion
rows are G-SMO-1's remaining conditions: they are reported for every arm, and the chosen arm must
meet them, with criterion 1, before its stage is published for reports.

| Kind      | Criterion                                                                                                                                | Evidence                 | Threshold                                                                                                          |
| --------- | ---------------------------------------------------------------------------------------------------------------------------------------- | ------------------------ | ------------------------------------------------------------------------------------------------------------------ |
| Identity  | ID switches and fragmentations, summed over held-out episodes, against the online arm                                                    | Held-out                 | No increase                                                                                                        |
| Identity  | IDF1 and AssA against the online arm                                                                                                     | Held-out                 | Lower by no more than 0.002                                                                                        |
| Geometry  | DetA and MOTP against the online arm                                                                                                     | Held-out                 | DetA lower by no more than 0.002; MOTP no worse                                                                    |
| Promotion | p99 lateral residual against the reference, reduction over online (G-SMO-1 criterion 2)                                                  | Held-out, gate partition | At least 20 %                                                                                                      |
| Manoeuvre | Peak deceleration and peak course (yaw) rate, 0.5 s differences, against the reference (G-SMO-1 criterion 3)                             | Abnormal-motion set      | At least 85 %                                                                                                      |
| Screen    | Peak deceleration, 0.5 s differences, against truth                                                                                      | Synthetic                | At least 85 %                                                                                                      |
| Screen    | Peak course rate, 0.5 s differences, against the online arm's own                                                                        | Synthetic                | At least 90 %                                                                                                      |
| Screen    | Total change preserved: Δspeed and lateral offset                                                                                        | Synthetic                | Within 2 % and 5 cm                                                                                                |
| Screen    | Onset identified at the impact frame from the stored record; signed innovation sum at least 2 for the impact, at most 0.5 for an anomaly | Synthetic                | Exact frame; both bounds                                                                                           |
| Screen    | Velocity change across the impact, ±1 s, against truth and against online                                                                | Synthetic                | At least 80 % of truth; never below online                                                                         |
| Promotion | Per-frame largest observed-state revision, p99 (G-SMO-1 criterion 4)                                                                     | Replay; held-out         | Under 0.3 m; every larger one flagged                                                                              |
| Revision  | Revisions without evidence                                                                                                               | All                      | Zero: a defect, not a tolerance                                                                                    |
| Promotion | Delay to finality, median, of states released by their lag (`lag_release_delay_secs`)                                                    | Replay                   | At most the horizon plus one frame interval; p95 reported, since it includes frames that never reached the tracker |

G-SMO-1 criterion 1, the online estimator at G-GEO-1 first, still governs promotion: none of this
selects a horizon for production before the corrected measurement exists. It does let the horizon
be chosen on evidence, rather than from the three-frame comparator or the one-second suggestion.

## Selection rule

1. Drop every arm that fails an identity criterion.
2. Drop every arm that fails the screen, and, once the abnormal-motion set exists, G-SMO-1
   criterion 3.
3. Drop every arm with a revision without evidence: that is a defect to fix, not an arm to rank.
4. Of the remainder, take the shortest horizon whose geometry gain (p99 lateral residual reduction
   over online) is at least 90 % of the best remaining arm's. Shorter horizons win ties because
   they finalise sooner and flatten less.
5. Promote the chosen arm only if it also meets every promotion row and G-SMO-1 criterion 1.
6. If nothing remains, or the chosen arm fails promotion, no stage is promoted and the finding is
   recorded here. Online stays the only published stage; nothing is relaxed to make an arm pass.

## Unresolved alternatives

A fixed-assignment arm inherits every association the tracker made, right or wrong. It cannot
represent an alternative, and does not pretend to: its revision record names the observation that
moved each state, and a wrong assignment surfaces as identity switches in the held-out criteria,
not as a smaller residual. Where the reference shows two tracks inside one object's gate (the D2
A/B found that on about 17 % of object-frames), the refined arms carry the same ambiguity.

The revisable-association arm must keep alternatives explicit rather than average them:

- Each unresolved group keeps at most four candidate assignments, including misses, births, fragment
  joins and cluster splits, with exclusive point ownership inside each (asynchronous tracking §2).
- Each alternative is its own hypothesis, persisted as its own estimate version and never merged
  into another's rows. The selected hypothesis is marked; the others keep their likelihoods.
- A segment still unresolved when its window closes is published as unresolved, with no final
  state. Metrics over it are suppressed with that reason, and the count of unresolved segments and
  exhausted candidate budgets is reported beside every figure.
- An arm that resolves an ambiguity must pass the identity criteria above against the fixed
  assignment arm at the same horizon.

## Evidence so far

### Synthetic scenes, shipped process noise

A single object at 10 Hz with 0.1 m Gaussian position noise through the shipped tracker. Peak rates
are 0.5 s differences. The table is what the tests pin.

| Scene                         | Truth       | Online | 3 frames | 0.5 s | 1 s  | 2 s  | Whole track |
| ----------------------------- | ----------- | ------ | -------- | ----- | ---- | ---- | ----------- |
| Braking, peak deceleration    | 7.00 m/s²   | 98 %   | 98 %     | 97 %  | 93 % | 87 % | 86 %        |
| Lane change, peak course rate | 0.155 rad/s | 70 %   | 69 %     | 68 %  | 64 % | 48 % | 48 %        |
| Lane change, share of online  | —           | —      | 99 %     | 98 %  | 91 % | 69 % | 69 %        |
| Impact, Δv over ±1 s          | 6.71 m/s    | 80 %   | 89 %     | 92 %  | 93 % | 84 % | 84 %        |

Two findings follow, and both bear on G-SMO-1 before any held-out run.

1. **RTS returns the most probable path under the filter's own model, and at the shipped process
   noise (0.2 m²/s³) that model finds a lane change improbable.** Even the online filter keeps
   only 70 % of the true course rate, so G-SMO-1 criterion 3 is not expected to pass on real
   manoeuvres at any horizon until the noise is recalibrated. Longer horizons flatten further: 2 s
   and whole-track keep 69 % of what online showed. With a manoeuvre-scale noise of 3 m²/s³ every
   horizon keeps at least 85 % of the truth for both manoeuvres. The flattening belongs to the
   model, and calibrating it is G-UNC-1's work, not the smoother's.
2. **At about 10 m/s of instantaneous Δv the shipped association gate loses the object**, and a
   new track begins. A fixed-assignment smoother cannot see across that break; only the revisable
   arm could. Impact preservation above is measured at 6.7 m/s, which the tracker holds.

Only the three-frame, 0.5 s and 1 s arms pass the screen, the last narrowly (91 % of online's
course rate); 2 s and whole-track fail it on the lane change.

### kirk0, label-free

The whole capture after a 20 s warm-up from its start: 63 s scored, 631 frames, the medoid
measurement, shipped tuning and the exact assignment solver. The scored population is 41 confirmed
tracks, 1,479 observed and 936 coasted states. A second run reproduced every figure exactly.

| Arm         | Revision p50 / p95 / p99 (m) | >0.3 m | Frame-max p99 (m) | Coasted revision p95 (m) | Lateral residual p95 (m) | Normalised residual mean | CV prediction error p95 (m) | Speed step RMS (m/s²) | Lag-release delay p50 / p95 (s) |
| ----------- | ---------------------------: | -----: | ----------------: | -----------------------: | -----------------------: | -----------------------: | --------------------------: | --------------------: | ------------------------------: |
| Online      |                            — |      — |                 — |                        — |                    0.170 |                     1.17 |                       1.082 |                  1.74 |                               0 |
| 3 frames    |        0.067 / 0.302 / 0.479 |     74 |             0.629 |                     1.58 |                    0.183 |                     0.80 |                       0.846 |                  1.05 |                     0.30 / 0.30 |
| 0.5 s       |        0.074 / 0.309 / 0.494 |     81 |             0.629 |                     1.58 |                    0.185 |                     0.81 |                       0.819 |                  0.87 |                     0.50 / 0.50 |
| 1 s         |        0.076 / 0.310 / 0.491 |     81 |             0.629 |                     1.56 |                    0.183 |                     0.82 |                       0.790 |                  0.67 |                     1.00 / 1.10 |
| 2 s         |        0.077 / 0.309 / 0.488 |     79 |             0.629 |                     1.56 |                    0.184 |                     0.81 |                       0.749 |                  0.61 |                     2.00 / 2.40 |
| Whole track |        0.078 / 0.315 / 0.489 |     82 |             0.629 |                     1.52 |                    0.184 |                     0.82 |                       0.754 |                  0.44 |              At the track's end |

What it says, and what it does not:

- **G-SMO-1 criterion 4 fails at every horizon.** The largest observed-state revision per frame has
  p99 0.63 m against 0.3 m, and 5–6 % of observed states move more than 0.3 m, all flagged.
  Almost all of it is present at three frames already (p99 0.48 m against 0.49 m at 2 s): the
  corrections are short-range, as the medoid's face hops of plan §3 would produce, though this run
  does not prove that cause. G-GEO-1 comes first, as criterion 1 requires.
- Coasted states move furthest, p95 about 1.6 m: an occlusion's constant-velocity prediction
  corrected by the observation that ends it. That is evidence-driven, and reported apart.
- The lateral residual rises slightly, because a refined state no longer bends towards each
  measurement. Speed steps and constant-velocity self-consistency improve with the horizon; these
  are exactly the figures plan §17.1 says decide nothing alone.
- The normalised residual mean is below its expectation of 2 in every arm: the filter's
  measurement noise, 0.05 m², is larger than the scatter the estimates leave. That is input for
  G-UNC-1, not a smoother setting.
- Every arm persisted exactly the online sink's 1,952 rows (60 tracks, warm-up included), each
  revision naming an online row that exists. No barrier, covariance fallback or revision without
  evidence occurred. Twenty-three transitions where `max_predict_dt` clamped a gap were flagged;
  only look-aheads that cross a gap longer than 0.5 s reach them. At most 60, 93, 168, 294 and 695
  steps were held at once.

The repository's kirk0 replay test scores 2 s after a 6 s warm-up. Its background has not
settled, so it carries many more short noise tracks; it checks invariants, not these figures.

## Running it

The replay tool, with an evidence database so each arm can later be scored per frame:

```sh
go run -tags=pcap ./cmd/tools/lidar-refinement-eval \
  -pcap internal/lidar/perf/pcap/kirk0.pcapng -out /tmp/refinement/kirk0 \
  -evidence-db /tmp/refinement/kirk0-evidence.db -case kirk0 -start 20 -warmup 20
```

It prints the table above, the smoother accounting, and one
[`lidar-ground-truth-eval perframe`](per-frame-evaluation.md) command per refined arm, each paired with the online arm and naming every version field. Every arm
is scored as a declared baseline; the selection rule reads the paired deltas. The corpus evidence
run takes the same experiment: `EVIDENCE_FLAGS="-experiment fixed_lag_rts"`, which also checks
that every horizon's figures reproduce on the repeat run.

## Amendments

None.
