# Campaign objectives from 2026-09-19

For the agent running [supervisor.py](supervisor.py). Read this before queueing a seventh pass.
It replaces the "Next, in order" list at the end of the fifth-pass results in the
[campaign plan](../../../../docs/plans/lidar-parameter-experiment-campaign-2026-09.md). The
evidence behind every statement here is in the
[gap analysis](../../../maths/paper-implementation-gap-analysis.md); row IDs (S3, B8, M5, K9) refer
to it.

## What changes

**The campaign's purpose changes from finding better defaults to measuring structural changes.**
Stop sweeping L4 and L5 tuning keys to choose values. Three things those values are conditional on
are scheduled to change: the position measurement (state-estimation Phase 2), the measurement
covariance (Phase 3, scalar to anisotropic), and the association cost (S3). An optimum found now is
an optimum of a system about to be replaced. Your own results say the same thing from the other
side: no L4 or L5 value is a Pareto improvement at both labelled captures, the gate moved from 9 to
100 without visible effect, and no noise setting is consistent in both speed bands.

What the harness is now for:

1. **Scoring default-off options on against off.** That comparison survives later changes in a way
   a tuned value does not.
2. **L3.** It is upstream of everything in the state-estimation plan and its contract with L4 does
   not change, so L3 results keep their value.
3. **Label-free baselines that a later change has to beat** (objectives 5 and 6).

Keep everything that has made the results trustworthy: predictions fixed before a run, Pareto
classes instead of rankings, repeated baselines as the noise floor, never ranking on the composite
score, the disk guard, path-scoped commits, options default-off with a test pinning the shipped
behaviour beside them.

## Two corrections to the current record

Make these in the campaign plan before anything else, because later stages build on them.

**`CascadedAssociation` is not the S3 remedy, and cannot be scored against `hits_to_confirm` = 1.**
The option orders by confirmation state, which is S2's remedy (a tentative track outbidding a
confirmed one). S3 is between two _confirmed_ tracks: the assignment cost is bare Mahalanobis
distance, a coasting track's covariance is inflated by 0.5 m² per missed frame, so it pays less for
the same miss. Both bidders land in the cascade's first stage under the same cost.
`internal/lidar/l5tracks/association_covariance_bias_test.go` (new, uncommitted, in your
`velocity.report-wt-gapfixes` worktree) pins this on the real `Update` path: the coasting track bids
7.4×, 20.3× and 33.5× cheaper at 1, 3 and 5 misses, takes a cluster twice as far from it as from
the fresh track after one miss, and the outcome is identical with `CascadedAssociation` on. At
`hits_to_confirm` = 1 every track is confirmed on its first hit, so the cascade has a single stage
and is a no-op there by construction. Plumbing it into the runtime params is still worthwhile for
S2, but a null result from it says nothing about S3. The plan's "its mechanism is the one
`hits_to_confirm` 1 exposed" should be withdrawn.

**The ground-truth and label-free sweeps of three L3 keys are void, not null (B8).** Once settling
completes, `effectiveCellParams` replaces the global `noise_relative`,
`neighbour_confirmation_count` and update fraction with per-region values for every covered cell.
They are computed once from the config in force at that moment (`noise_relative` × 4 for the most
stable third of cells, × 3 for the middle, × 8 for the most variable), persisted, and restored. A
runtime POST afterwards never reaches a settled cell. That is the simplest explanation for the
asymmetry the fifth pass left open: `closeness_multiplier` has no override and moves recall from 4
to 13 of 16; the three overridden keys "move nothing at the tracker". Withdraw "the
settling-sensitive settings do not reach the tracker" as a finding about the settings. Objective 2
confirms or refutes the explanation.

## Objectives, in order

Each has a prediction to record before the run and a statement of what each outcome decides.

### 1. Is the association cost bias (S3) material on real traffic?

The test proves the mechanism. It does not show that the mechanism matters at the corpus's
densities, and K1 is the precedent for a real defect that measured as immaterial.

**1a. No code, run first.** `l5.cv_kf_v1.occlusion_cov_inflation` is runtime-settable and zero is
valid. Sweep 0, 0.1, 0.25, 0.5 (shipped) and 1.0 through the label-free harness on the five 300 s
segments and the ground-truth harness on kirk0 and kirk1. At 0 a coasting track's covariance still
grows through Q, but slowly, so the bias nearly vanishes while the 5 m jump guard and the gate at 36
still leave room to reacquire.

- _Prediction:_ track count falls and median track lifetime rises monotonically as inflation goes
  to 0, at every segment. On kirk0 and kirk1, candidates fall without losing matched tracks.
- _If it holds:_ S3 is material and is one cause of the 0.36 s median lifetime. Go to 1b.
- _If track count moves by less than the baseline noise threshold:_ S3 is real and immaterial at
  these densities. Record it as K1 was recorded and drop it below objective 3.
- _If matched tracks fall at 0:_ reacquisition needs the wider covariance, which argues for the
  likelihood cost over simply removing the inflation. Go to 1b either way.

**1b. Code, on `dd/state-est/gap-fixes`.** Add a default-off `LikelihoodAssociationCost`: the
Hungarian cost becomes `d² + ln|S|`, the gate stays on `d²`. The pinned tests already check the
arithmetic against real covariances: it reverses all four pinned cases and still returns a
reappearing object to the coasting track that predicted it. Plumb it into the runtime params and
score on against off on the same seven captures.

- _The mechanism test:_ `hits_to_confirm` = 1 on kirk0 with the option on. If S3 explains the
  collapse from 7 to 0 of 16, matched recovers. If it stays at 0, the collapse has another cause
  and the gap analysis's reading of that result is wrong; say so there.

### 2. Confirm B8, then measure the acceptance window properly

**2a.** Log the effective `noise_relative` per region in the isolated server after a runtime POST,
and check whether regions are re-identified or restored between replays. Add the pin test B8 asks
for (settle a grid, change the key, assert the per-cell value is unchanged).

**2b.** Add a default-off way to disable the region overrides, or to set their three multipliers
to 1. Rerun the window sweep on kirk0 and kirk1 with overrides off: `closeness_multiplier`
1.5 / 2 / 3 / 5 and `noise_relative` at the values giving the same product.

- _Prediction:_ with overrides off, `noise_relative` reproduces `closeness_multiplier`'s recall
  curve at equal product, as B7 says and as your settling-side result (3 / 9 / 16 against 1 / 6 / 13)
  already shows pre-settle.
- _What it decides:_ whether the remedy for HW1 is to lower `closeness_multiplier` or to remove
  the ×3–8 multipliers and move to a range-banded noise term. The second is the principled one:
  `closeness_multiplier` also scales the learned spread, which is the only data-driven term in the
  window, and the multipliers widen the window fourfold even for the most stable third of cells,
  where a learned window would tighten. Do not move either default on the current evidence.

### 3. Price the closeness result: recall or fragmentation? (M5)

`closeness_multiplier` = 2 recovering 13 of 16 and 33 of 49 is the largest effect the campaign has
found, and it comes with 40–80% more candidate tracks. Your lifetime and mean-IoU columns argue the
extra tracks are the same population rather than existing tracks cut up. That is an inference
through an evaluator that matches whole tracks on temporal overlap, scores a track followed in
four pieces as undetected, and hard-codes `Fragmentation` to 0. It cannot settle the question.

Build per-frame matching into `EvaluateGroundTruth` (gap-fixes branch): in each frame, a bijective
match between reference and candidate positions under HOTA's point similarity (one minus distance,
zero past 1 m), then identity switches, MOT16's FM, and fragments per reference track over the
same matches. Drop candidates matched to unlabelled reference tracks instead of counting them as
false positives. First check that per-frame positions are retained for both the reference runs and
the candidates the isolated servers write; if candidates do not keep them, that is the first
change.

Then re-score four configs on both captures: baseline, `closeness_multiplier` 2,
`hits_to_confirm` 6 or 7, and whichever option objective 1 favours.

- _What it decides:_ whether any default moves. A recall gain that is really more fragments is
  not a gain. Until this exists, treat `closeness_multiplier` 2 and `hits_to_confirm` 7 as leads.
  `hits_to_confirm` 7 in particular raises median lifetime by hiding short tracks, which treats
  the symptom in objective 4.

### 4. Explain the 0.36 s median track lifetime

This is the most important track observation the campaign has produced and nothing yet explains
it. 499 of 525 tracks at Columbus-Broadway live under a second, the median is the confirmation
threshold, and it does not move with the acceptance window. Tuning around it (objective 3's
`hits_to_confirm`) is premature.

Classify how short tracks end, label-free, from the immutable observation records, generalising
the three-episode reconstruction in the
[corpus baseline](../../../../docs/lidar/operations/state-estimation-phase01-corpus-baseline.md#representative-episode-inspection)
to the whole population. For each track under 1 s, in the frames around its last association:

| Class           | Test                                                                             | Points at              |
| --------------- | -------------------------------------------------------------------------------- | ---------------------- |
| Contested       | A cluster lay within 1 m of its prediction and was assigned to a different track | S3, S2                 |
| Absorbed        | The nearest cluster was a larger one that another track holds                    | D3 merge               |
| Vanished        | No cluster within 2 m for the rest of its miss budget                            | L3 window, L4 `MinPts` |
| Never an object | Stationary, in a cell with high learned spread or recent foreground churn        | L3 noise, B3           |
| Left the scene  | Last position within 2 m of the field-of-view boundary                           | Correct behaviour      |

- _Prediction, to be recorded before the run:_ write your own. Mine is that Contested plus
  Never-an-object exceed half.
- _What it decides:_ which layer the next structural fix belongs to. It also says whether
  objective 1's effect size is plausible before 1b is built.

### 5. Baseline the coast for G-EST-1

[G-EST-1](../../../../docs/plans/lidar-state-estimation-plan.md#73-recommendation-and-gate) has a
new fifth criterion: constant acceleration must be no worse than constant velocity across a
coast. Measure the CV side now. On stored observations, withhold a confirmed moving track's
observations for 0.5 s, 1.0 s and 1.5 s, predict from the state at the start of the gap, and score
against the first observation after it. Report median and p95 reacquisition error by horizon and
speed band, and separately for gaps that begin while the track is decelerating. No labels, no
server, no tuning. This is also gap-analysis row V1 on real data.

- _What it decides:_ nothing today. It is the number Phase 4 has to beat, and it bounds how long
  sprint 0.5.2.2's coast can be before the prediction is worse than the gate.

### 6. NIS: record, do not select

`process_noise_pos` × 4 helping the moving bands at every site while hurting the slow band at
three is K9 restated: no scalar is consistent in both bands. Do not promote it and do not spend
the ground-truth and label-free runs you had planned for it. Hand the six-site table to Phase 3 as
the case for speed- or class-conditioned Q and R.

One addition would raise the value of every later NIS run. The `all_x0.25` control showed the
metric is censored by association. Accumulate NIS for every gated candidate pair before
assignment, beside the accepted-only figure, which is the pre-gate NIS that G-UNC-1 already asks
for.

### 7. Carry on as planned

`DensityPreservingCap` on the busy segments (option on against off). The heading flip rule waits
for the D2 course-alignment harness. The extra-candidate inspection against unlabelled reference
tracks folds into objective 3.

## Do not

- Sweep `measurement_noise`, `process_noise_*`, `gating_distance_squared`, `max_misses*` or
  `hits_to_confirm` to choose defaults.
- Evaluate S3 through `CascadedAssociation`.
- Cite the tracker-side sweeps of `noise_relative`, `neighbour_confirmation_count` or
  `background_update_fraction` as nulls until objective 2a is done.
- Move any default on temporal-IoU evidence alone.
- Commit the S3 test as part of a sweep commit. It is a separate, path-scoped commit on
  `dd/state-est/gap-fixes`, and it was written by another session, so read it first.

## What each result changes

| Result                                         | Changes                                                                          |
| ---------------------------------------------- | -------------------------------------------------------------------------------- |
| 1a monotone, 1b recovers `hits_to_confirm` = 1 | Backlog "Association cost (S3)" is confirmed as the gate on occlusion continuity |
| 1a flat                                        | S3 drops to a recorded, immaterial gap; occlusion continuity is unblocked        |
| 2b: product holds with overrides off           | HW1's remedy is the multipliers and the noise term, not `closeness_multiplier`   |
| 3: extra tracks at closeness 2 are fragments   | The window result is withdrawn as a recall gain                                  |
| 3: they are distinct objects                   | Strongest evidence yet for HW1; size the change with 2b                          |
| 4: Contested dominates                         | L5 association is the next fix; 1b moves up                                      |
| 4: Vanished or Never-an-object dominates       | L3 and L4 are the next fix; objective 2 moves up                                 |
