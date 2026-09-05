# LiDAR heading coherence sprint plan

- **Status:** Draft (two-day sprint), evidence gathered from run `baf20f02`
- **Layers:** L4 Perception, L5 Tracks, L8 Analytics, L9 Endpoints, web UI
- **Target:** v0.5.2; a two-day slice, not the full geometry rewrite
- **Evidence run:** `baf20f02-075b-4041-9860-ff090754f94f`, 600 frames, 60 s, 346 distinct tracks, build `6d8c799e6`
- **Canonical maths:** [obb-heading-stability-review](../../data/maths/proposals/20260222-obb-heading-stability-review.md), [geometry-coherent-tracking](../../data/maths/proposals/20260222-geometry-coherent-tracking.md)
- **Related plans:** [lidar-state-estimation-plan](lidar-state-estimation-plan.md), [lidar-analysis-run-infrastructure-plan](lidar-analysis-run-infrastructure-plan.md)

> **Scope.** Two days of work to stop boxes pointing the wrong way, stop one
> vehicle being drawn as two, and put a per-run rotation and alignment metric in
> front of a human. It deliberately does not implement the full Bayesian
> geometry state in [geometry-coherent-tracking](../../data/maths/proposals/20260222-geometry-coherent-tracking.md),
> which is costed at 6 to 7 days. It builds the measurement harness that work
> will need to prove itself, and lands the two fixes that need no new model.

## 1. What the run actually shows

All figures below come from `baf20f02`, read directly from the recorded
`FrameBundle` stream. The tools that produced them are described in Section 5.

| Observation                                                        | Value                                 |
| ------------------------------------------------------------------ | ------------------------------------- |
| Published track-frames in `DELETED` state                          | **45.8 %** (10,610 of 23,185)         |
| Frames containing at least one co-located track pair, within 3 m   | **98.2 %** (589 of 600)               |
| Tracks entering a sustained heading lock, 5 frames or more         | **55 %** (78 of 143)                  |
| Locked tracks that never release, no 5 consecutive unlocked frames | **59 %** of those, 32 % of all tracks |
| Median \|OBB heading − course\| once permanently locked            | **106°**                              |
| Median \|OBB heading − course\| for tracks that never lock         | **74°**                               |
| Frames with dimensions frozen at the lock value                    | median **10**, p90 **44**             |
| Median per-track p95 frame-to-frame OBB step                       | **0.9°**                              |

The two tracks in the report, `trk_18952226` and `trk_04e4ebd5`, are co-located
for **100 frames**. They are the split car.

### 1.1 The boxes are not spinning

This is the finding that redirects the work. The published OBB heading moves by
a median p95 of 0.9° per frame, and the worst track in the run reaches 14°. The
boxes are nearly static. What the eye reads as phantom rotation is three
separate things happening at once:

1. A box locked at a heading roughly 106° away from the direction of travel,
   which looks wrong in a way that reads as rotation when the vehicle moves
   under it.
2. A second, frozen ghost box left by a deleted track, five seconds long.
3. A live box and a fragment box on the same vehicle, moving differently.

Tuning the smoother will not touch any of the three. Tightening the aspect-ratio
lock, which is the open Fix D in the heading stability review, makes the first
one worse.

### 1.2 Track `trk_18952226`, frame by frame

The failure is legible in a single track. Abridged, with the OBB and course
headings in degrees:

| Frame | course | OBB   | Δ   | L × W           | source                     |
| ----- | ------ | ----- | --- | --------------- | -------------------------- |
| 1006  | 54.5   | 155.7 | 101 | 4.45 × 2.07     | locked                     |
| 1011  | 49.5   | 146.0 | 97  | 4.33 × 1.89     | velocity                   |
| 1026  | 28.3   | 141.8 | 114 | **0.11 × 0.08** | velocity                   |
| 1054  | 25.0   | 141.8 | 117 | **0.11 × 0.08** | locked                     |
| 1099  | 23.8   | 138.6 | 115 | 1.46 × 0.38     | `DELETED`, still published |

At frame 1026 a fragment cluster of 0.11 m by 0.08 m is associated to a track
carrying a 4.33 m car, and the car's dimensions are overwritten with the
fragment's. The heading then locks and the dimensions are frozen there for
28 frames. The track is deleted at frame 1099 and keeps being published,
unchanged, for a further 25 frames.

## 2. Root causes

| ID  | Cause                                                                                                                                                                                                                                                                     | Evidence                                                  | Fix in this sprint |
| --- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------- | ------------------ |
| RC1 | **Guard 3 is a one-way ratchet.** It rejects any heading delta between 60° and 120° measured against the _smoothed_ heading. Once the smoothed heading is more than 60° from truth, every correct measurement is rejected as an axis swap, and nothing can ever unlock it | 59 % of locked tracks never release; median residual 106° | Yes, D1.3          |
| RC2 | **Dimensions are not updated while locked.** `tracking_update.go` updates only height when `updateHeading` is false, so a bad frame's dimensions persist for the track's life                                                                                             | Frozen for median 10, p90 44 frames; 0.11 × 0.08 car      | Yes, D1.4          |
| RC3 | **Association has no shape gate.** `gating_distance_squared: 36` is a 6 m radius on position alone, so a 0.11 m fragment can capture a 4.3 m track                                                                                                                        | Frame 1026; 98 % of frames carry a co-located pair        | Yes, D1.5, D2.2    |
| RC4 | **Deleted tracks are published frozen for 5 s.** `deleted_track_grace_period: 5s` at 10 Hz is 50 ghost frames per dead track                                                                                                                                              | 45.8 % of all published track-frames                      | Yes, D1.6          |
| RC5 | **No temporal geometry model.** Every frame recomputes shape from scratch; the guards are reactive patches on a missing model                                                                                                                                             | Median 74° residual even on never-locked tracks           | No, this is D-04   |
| RC6 | **The objective rewards the pathology.** `HeadingJitter` carries a negative weight and a locked heading has near-zero jitter. `ActiveTracks` carries `+0.3` on a log scale, so splitting one vehicle into two scores better than tracking it once                         | `sweep/objective.go:70`, `DefaultObjectiveWeights`        | Yes, D2.3          |
| RC7 | **Nothing measures whether the box points the right way.** `AlignmentMeanRad` compares Kalman velocity against displacement. Both are motion. The OBB is not in the metric                                                                                                | `tracking_update.go:196`                                  | Yes, D1.1, D1.2    |

RC6 and RC7 together are why this survived tuning. The auto-tuner was scored on
a metric that improves as the box stops moving, and no metric in the system
would have noticed a box pointing 106° away from the direction of travel.

## 3. What the outstanding maths gives us

| Proposal                                                                                                                  | Status                           | Use in this sprint                                                                                                                                              |
| ------------------------------------------------------------------------------------------------------------------------- | -------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [geometry-coherent-tracking](../../data/maths/proposals/20260222-geometry-coherent-tracking.md)                           | Proposal, costed L, 6 to 7 days  | Take §2.3 axis-selection likelihood test only. It is the escape hatch RC1 needs and is about 40 lines                                                           |
| [obb-heading-stability-review](../../data/maths/proposals/20260222-obb-heading-stability-review.md)                       | Fixes B, C, G landed; Fix D open | **Do not land Fix D.** Tightening the lock threshold to 0.15 or 0.10 increases lock entry, and 59 % of locks are permanent. Record the measurement and close it |
| [velocity-coherent-foreground-extraction](../../data/maths/proposals/20260220-velocity-coherent-foreground-extraction.md) | Proposal                         | Not in this sprint. Would give a PCA-independent heading signal                                                                                                 |
| [lidar-state-estimation-plan](lidar-state-estimation-plan.md)                                                             | Draft, this PR                   | The near-edge measurement work supersedes the medoid. Out of scope for two days, but D1.4's metric is the gate it will be judged on                             |

### 3.1 The one piece of D-04 worth taking now

Section 2.3 of the geometry proposal replaces the guard stack with a likelihood
test between two interpretations of each observation:

- aligned: `(L_obs, W_obs, θ_obs)`
- swapped: `(W_obs, L_obs, θ_obs + π/2)`

Pick whichever is more consistent with the track's current belief, then update.
This is the correct answer to a PCA axis swap, and unlike Guard 3 it always
produces an answer, so it cannot deadlock. Taking the axis test alone, with a
simple running mean rather than the full Bayesian state, is a half-day change
that removes the ratchet.

## 4. Two-day plan

### Day 1: measure it, then stop the bleeding

Order matters. The metric lands first so every later change is attributable.

| #    | Task                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    | Files                                                                                    | Size |
| ---- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------- | ---- |
| D1.1 | **Course-alignment metric. Done.** Per-frame \|OBB heading − course\| folded to **[0, 90]**, not [0, 180]: an OBB is symmetric, so a box pointing backwards along the course is correctly oriented and folds to 0, while a 90° length/width swap is the worst case. Held as a 20-bin histogram per track so the cost is constant on the Pi. Sampled only on live frames at or above 2 m/s                                                                                                                                                                                                               | `l5tracks/tracking.go`, `tracking_metrics.go`, `analysis/types.go`, `analysis/report.go` | S    |
| D1.2 | **Lock telemetry. Done.** `HeadingLockedFrames`, `LongestLockRun`, `EnteredSustainedLock`, `ReleasedAfterLock` and a derived `LockTrapped` per track, plus a per-run `heading_source` histogram and trapped ratio. A release requires five consecutive unlocked frames: Guard 3 rejects per frame, so a single frame slipping through is not the lock letting go                                                                                                                                                                                                                                        | `l5tracks/tracking.go`, `tracking_update.go`, `analysis/report.go`                       | S    |
| D1.3 | **Break the ratchet. Done.** Guard 3 keeps rejecting, but a counter releases the lock after `N` consecutive rejections (default 5, config `obb_heading_lock_max_rejections`; 0 restores the old behaviour). On release the heading **snaps** to the measurement rather than easing toward it, because the EMA moves 8 % of the gap per update and easing across a delta wide enough to be rejected would re-trigger the guard forever. Guards 1 and 2 do not drive the release: they fire when the measurement is genuinely unusable, and snapping to it would replace a wrong answer with a random one | `l5tracks/tracking_update.go`, `tracking_config.go`, `internal/config/tuning.go`         | S    |
| D1.4 | **Stop freezing dimensions.** While the heading is locked, update length and width from the cluster OBB projected onto the locked axes, rather than not at all. A locked heading is a statement about orientation, not a reason to stop measuring size                                                                                                                                                                                                                                                                                                                                                  | `l5tracks/tracking_update.go`                                                            | S    |
| D1.5 | **Fragment guard. Done.** Forbid the pairing in the association cost matrix, not at update time, so the fragment stays unassociated and can seed its own track instead of being consumed. Fires only when a track has at least 3 observations, believes it is at least 2 m (`FragmentGuardMinTrackExtentMetres`), and the cluster's longest extent is under `min_associable_extent_metres` (default 0.5). The belief is the running average, not the latest frame, because the latest frame is what a fragment would already have corrupted                                                             | `l5tracks/tracking_update.go`, `tracking_association.go`                                 | S    |
| D1.6 | **Ghost fade decoupled. Done.** The fade-out was deliberate, but its duration was `deleted_track_grace_period`: one number doing two unrelated jobs, so a re-association window held a frozen box on screen for five seconds. Split out `deleted_track_render_fade` (default 500 ms). Re-association is untouched                                                                                                                                                                                                                                                                                       | `l9endpoints/adapter.go`, `l5tracks/tracking.go`                                         | S    |

**Day 1 gate.** Re-run `baf20f02` through the pipeline and compare against the
recorded baseline. Targets, all measured by D1.1 and D1.2:

- Median per-track `CourseAlignmentP50` across tracks with samples: **below 25°**, from a measured baseline of **50.3°**.
- Tracks entering a permanent lock: **0 %**, from a measured baseline of **54 tracks, 65 % of those that lock**.
- Published `DELETED` track-frames: **5.0 %**, from 45.8 %, a measured 89 % reduction in ghost frames. Not zero, because a short fade-out is deliberate.
- Frames with a co-located pair: no worse than baseline. D1.5 should improve it; it is not the fix for it.

#### D1.1 baseline, measured

The metric is landed and the baseline is recorded. Across the 55 tracks in
`baf20f02` that produced samples:

| Statistic                                          | Value                              |
| -------------------------------------------------- | ---------------------------------- |
| Per-track median course error, distribution median | **50.3°**                          |
| Per-track median course error, p85                 | **67.2°**                          |
| Worst track (`trk_1c828bcf`, 44 samples)           | **85.0°**                          |
| `trk_18952226`, the split car                      | **66.4°** median, 1.95° OBB jitter |
| `trk_04e4ebd5`, its twin                           | **16.2°** median, 1.01° OBB jitter |

The last two rows are the case for the metric. Both boxes are rock steady, at
about 1° to 2° of frame-to-frame movement, and one of them points 66° away from
where its vehicle is going. Jitter called that track healthy.

Two defects surfaced while wiring it up, both fixed in the same change:

1. **The offline and live `HeadingJitterDeg` measure different quantities.**
   `analysis` computed it from `Track.HeadingRad`, the Kalman course, while the
   tracker computes it from OBB heading deltas. The two paths are what an A/B
   comparison puts side by side. The box quantity is now reported separately as
   `OBBHeadingJitterDeg`, and both fields say in their godoc which is which.
2. **Run-level aggregates keyed on final track state see almost nothing.**
   `acc.state` holds the last state observed, and 211 of 346 tracks end
   `DELETED`, so the confirmed-only rollups covered 13 tracks. Course alignment
   rolls up over every track that produced samples instead, which is 55. The
   existing jitter and alignment aggregates still have this flaw and are
   understated; worth a follow-up, out of scope here.

#### D1.2 baseline, measured

Over the 239 tracks in `baf20f02` that lived at least five live frames, which is
the minimum for a sustained lock to be detectable:

| Statistic                         | Value                            |
| --------------------------------- | -------------------------------- |
| Tracks entering a sustained lock  | **83 (35 %)**                    |
| ...that never released it         | **54, or 65 % of locked tracks** |
| Locked share of live track-frames | **33.7 %**                       |
| Longest single lock run           | **152 frames, 15.2 s**           |

And the link the ratchet fix rests on. Splitting the tracks that produced course
samples by whether they were trapped:

| Population                | Tracks | Median course error |
| ------------------------- | ------ | ------------------- |
| Trapped in a heading lock | 24     | **60.1°**           |
| Not trapped               | 31     | **38.5°**           |

The lock costs about 22° of median course error. That is the measured case for
D1.3, and it is now a number that will move when the ratchet is broken.

**A third defect, found here.** The offline reconstruction initially counted
`DELETED` ghost frames. Those frames carry heading source `pca` rather than the
lock the track died in, so fifty frames of ghost turn a track that was trapped
for its whole life into a clean one: the trapped population read 11 % of locked
tracks instead of 65 %. Lock stats now skip non-live frames. The corroboration
is exact: excluding them dropped the `pca` frame count from 15,069 to 4,459, a
difference of 10,610, against the 10,610 `DELETED` track-frames counted
independently in Section 1.

This is the same defect as RC4 seen from a second direction. Ghost frames do not
only mislead the eye in the visualiser, they corrupt any metric computed over
the recorded stream.

#### D1.3 status

The mechanism is proven on synthetic sequences that reproduce the trap exactly:
a confirmed track whose smoothed heading sits 90° from its course, fed correct
measurements. With the release disabled it stays above 80° of course error
forever; with the release armed it converges below 5°. Nine tests cover the
release firing, not firing early, resetting on an accepted update, snapping
rather than easing, restoring dimension updates, and staying out of the way of
Guards 1 and 2. The two pre-existing Guard 3 tests still pass, so the guard's
legitimate job is intact.

**The end-to-end number is not yet measured.** A VRLOG replays decisions already
made, so it cannot re-run the estimator: confirming the fix on `baf20f02`
requires re-running the pipeline over the source PCAP, which is present at
`sensor_data/lidar/static/s2_sf_4_20260902153250_00003.pcap`. The only routes
are a second server instance on spare ports or a new headless harness built on
`server.DirectBackend`. Both compete with the live recording for CPU, so it is
a decision rather than a step. This is the same limitation Section 5 records
about VRLOG, arriving exactly where the plan said it would.

Three things the change had to touch beyond the guard, all because the config
schema requires every key rather than defaulting silently:

- `config/tuning.{defaults,example,optimised}.json` gain the key.
- `config-migrate` writes the shipped default when migrating a legacy config.
  The zero value disables the release, so a migration would otherwise put a
  config quietly back on the ratchet.
- The runtime tuning endpoint accepts the key, or a POST carrying it is
  rejected with a 400.

#### D1.5 and D1.6 results

**D1.6 is measured exactly.** The render fade is a pure filter on publication,
so its effect is computable from the recorded stream with no pipeline re-run:

|                                                | Frames | Share of all track-frames |
| ---------------------------------------------- | ------ | ------------------------- |
| Published `DELETED` under the 5 s grace period | 10,610 | **45.8 %**                |
| Surviving a 500 ms render fade                 | 1,170  | **5.0 %**                 |
| Removed                                        | 9,440  | **89 % fewer ghosts**     |

The design point worth keeping. Excluding deleted tracks outright was the
obvious reading of this task, and it would have been wrong: the fade-out is a
deliberate rendering feature, and `alpha` already carries it. The defect was
that its duration was borrowed from `deleted_track_grace_period`, which exists
so re-association can still find a track seconds later. Those are different
concerns on different timescales. Splitting them keeps the fade, kills the
ghost, and leaves re-association exactly as it was.

**D1.5 is proven on synthetic cases, not yet on the run.** Like D1.3 it changes
what the tracker does, so its effect needs a pipeline re-run to measure. Eleven
tests cover the guard rejecting the 0.11 m by 0.08 m scrap from run `baf20f02`,
accepting a partly occluded 2.6 m cluster, ignoring pedestrian-scale tracks
entirely, ignoring tracks with too little history, ignoring clusters that report
no extent, consulting the running average rather than the corrupted latest
frame, and leaving the fragment unassociated end to end through `associate()`.

The guard is deliberately narrow: an absurdity check, not a shape gate. It fires
only for metre-scale tracks offered sub-half-metre clusters. Proper dimension
consistency in the assignment cost is D2.2, and applying anything like it to
pedestrians and cyclists would reject their ordinary observations.

### Day 1 gate: measured

The headless harness (Section 5.2) makes this a controlled experiment: both
arms replay the same PCAP through the same binary, differing only in the tuning
file. Comparing against the original `baf20f02` VRLOG would not have been valid,
because that was recorded through the live server with `MaxFrameRate: 25`.

The "before" arm restores the pre-sprint behaviour: `obb_heading_lock_max_rejections: 0`,
`min_associable_extent_metres: 0`, and `deleted_track_render_fade` back on the
5 s grace period.

| Metric                            | Before         | After                 | Change         |
| --------------------------------- | -------------- | --------------------- | -------------- |
| Median per-track course error     | **40.8°**      | **28.3° to 29.6°**    | **−28 %**      |
| Locked share of live track-frames | 24.8 %         | 16.1 % to 16.6 %      | −34 %          |
| Longest single lock run           | **70 frames**  | **20 frames**         | **−71 %**      |
| Published `DELETED` track-frames  | 8,617 (54.0 %) | 951 (11.4 %)          | **−89 %**      |
| Total published track-frames      | 15,970         | 8,377                 | −48 %          |
| Tracks entering a sustained lock  | 63             | 53 to 58              | −11 %          |
| Tracks trapped                    | 37 (20 %)      | 30 to 35 (16 to 19 %) | small          |
| Fragmentation ratio               | 0.286          | 0.291 to 0.296        | slightly worse |

Four things this says that a single number would hide.

**The course error is the outcome metric, and it moved 28 %.** That is the
quantity the whole sprint exists to reduce, and it is now measured rather than
argued.

**The trapped count barely moved, and that is a metric artefact rather than a
failed fix.** `LockTrapped` requires five consecutive unlocked frames to clear.
Once the release starts breaking locks, tracks lock and unlock repeatedly, so
the flag stays set even though the ratchet is gone. The direct evidence that it
is gone is the longest lock run collapsing from 70 frames to 20. The metric was
designed to detect a permanent ratchet and now conflates "never escapes" with
"escapes and re-locks"; it needs splitting before it is used as a gate.

**Twenty frames of locked heading is still two seconds.** The release stops a
track being lost for good, but it does not make the heading right. That is the
case for D2.1, exactly where the plan put it.

**Fragmentation got slightly worse, as intended.** The fragment guard leaves a
scrap unassociated, so it seeds its own short-lived track instead of corrupting
a vehicle's. A correct small track is a better outcome than a corrupted large
one, but the ratio moves the wrong way and should not be read as a regression
without that context.

**Run-to-run variance.** Two identical "after" runs gave 29.6° and 28.3° median,
and 35 versus 30 trapped tracks. Frame assembly is not bit-deterministic, so
the harness reproduces frame counts but not exact per-track outcomes. The
signal here is 11° to 12° against roughly 1.3° of noise, which is comfortable,
but a regression fixture (D2.5) will need a tolerance rather than an equality.

### Day 2: axis coherence and an honest score

| #    | Task                                                                                                                                                                                                                                                                               | Files                                                                                       | Size |
| ---- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------- | ---- |
| D2.1 | **Axis-selection likelihood test.** Implement §2.3 of the geometry proposal against a running length and width mean per track. Replaces Guard 2 and demotes Guard 3 to a diagnostic counter. Keep the old path behind a config flag for A/B                                        | `l5tracks/tracking_update.go`, `tracking_config.go`                                         | M    |
| D2.2 | **Shape-aware association gate.** Extend the gate cost with a dimension-consistency term so a fragment does not win a confirmed track on position alone. Position stays the dominant term                                                                                          | `l5tracks/tracking_association.go`                                                          | M    |
| D2.3 | **Fix the objective.** Set `HeadingJitter` weight to zero and add `CourseAlignment` with a negative weight. Change `ActiveTracks` from a reward to a band, penalising both too few and implausibly many. Document why in the objective godoc                                       | `sweep/objective.go`, `sweep/runner.go`                                                     | S    |
| D2.4 | **Per-run alignment UI on 8080.** New Svelte panel on the existing run detail page: course-alignment distribution, heading-source histogram, lock-run distribution, and a co-located-pair count. Consumes the extended `AnalysisReport` through the run API the page already calls | `web/src/routes/lidar/runs/+page.svelte`, `web/src/lib/types/lidar.ts`, `analysis/types.go` | M    |
| D2.5 | **Regression fixture.** Freeze a 200-frame excerpt of `baf20f02` covering the `18952226` and `04e4ebd5` overlap as a test fixture, and assert the Day 1 gate numbers in Go                                                                                                         | `l5tracks/tracking_coverage_test.go`, `analysis/testdata`                                   | M    |

**Day 2 gate.** The A/B comparison in `analysis.CompareReports` shows the new
path equal or better on course alignment, fragmentation, and co-located pairs,
with no regression in acceptance rate.

## 5. Harness

Most of what is needed already exists and is pointed at the wrong quantity.

| Component                              | State                                                                | Change                                                                                                                               |
| -------------------------------------- | -------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------ |
| `analysis.GenerateReport(vrlogPath)`   | Exists, produces `AnalysisReport` with per-track detail              | Add the D1.1 and D1.2 fields                                                                                                         |
| `analysis.CompareReports(a, b, out)`   | Exists, produces `ComparisonReport` with `QualityDelta`              | Add course alignment to `QualityDelta`                                                                                               |
| `sweep` objective and runner           | Exists, scores `HeadingJitterDeg`                                    | D2.3                                                                                                                                 |
| HINT tuner                             | Exists, human-in-the-loop labelling                                  | Use unchanged for D2.1 A/B. It is the right tool for judging whether a box looks right, which is the actual acceptance question here |
| `/api/lidar/runs/{id}` and `/evaluate` | Exists, already consumed by the Svelte run page                      | No new endpoint needed; extend the payload                                                                                           |
| Web run detail page                    | Exists at `web/src/routes/lidar/runs`                                | D2.4 adds a panel                                                                                                                    |
| macOS visualiser                       | Has `showHeadingSource` colouring in the renderer with no UI toggle  | Out of scope, but the toggle is nearly free and would pay for itself immediately                                                     |
| Headless PCAP replay                   | **Built:** `internal/lidar/replayeval`, `velocity lidar pcap-replay` | See 5.2. This is what makes a tracker change measurable at all                                                                       |

Three throwaway analysers were written against the recorded stream to produce
Section 1 and live in `.scratch-analysis/`. They read `FrameBundle` records
directly and touch nothing. The useful parts are the co-location detector and
the lock-trap detector; both should be folded into `analysis` as part of D1.1
and D1.2 rather than kept as scripts.

### 5.2 The headless replay harness

`internal/lidar/replayeval`, exposed as `velocity lidar pcap-replay`, replays a
PCAP through the full L1 to L6 pipeline and records a VRLOG. No server, no
database, no listening port.

It exists because of the limitation Section 5 records: a VRLOG stores the
decisions the pipeline already made, so replaying one shows what the old code
concluded rather than what the new code would conclude. Measuring a change to
L4, L5, or L6 means re-running perception over the packets, and the only route
to that was the live server's replay endpoint.

The output is a VRLOG directory, so nothing downstream needed changing:
`analysis.GenerateReport` for metrics, `analysis.CompareReports` for A/B, and
the macOS visualiser for looking at it. `--compare-to` runs all three in one
command.

```bash
velocity lidar pcap-replay --pcap capture.pcap --output ./runs/before --config before.json
velocity lidar pcap-replay --pcap capture.pcap --output ./runs/after --compare-to ./runs/before
```

Notes that matter for using it:

- Sixty seconds of capture replays in about 12 seconds on an M-series Mac, so an
  A/B is a two-minute loop rather than a scheduled job.
- The frame-rate throttle is off. It exists to stop a real-time replay flooding
  a live gRPC client, and dropping frames would make two runs disagree for
  reasons unrelated to the change under test.
- Point clouds are omitted unless `--include-points` is passed. They are the
  bulk of the file: 45 s of the SoMa capture is 347 MB with them and 9.1 MB
  without.
- Track persistence is disabled explicitly rather than by leaving the DB nil,
  so a future pipeline change that starts assuming a database fails loudly here
  instead of writing into the production store during an analysis run.

### 5.1 Why not the 8081 UI

The legacy LiDAR web UI in `internal/lidar/server` is not the place for this.
The run detail work belongs in the Svelte app, which already calls
`/api/lidar/runs` through the Vite proxy and already renders run tracks. D2.4
extends a page that exists rather than adding a second place to look.

## 6. Backlog entries

Copy-ready lines for `docs/BACKLOG.md`, in release order. Their link paths
are relative to `docs/BACKLOG.md`, not to this file, so a link checker run
against this document will flag them. <!-- link-ignore -->

**v0.5.2**

- Heading lock ratchet release (heading coherence D1.3): Guard 3 rejects any 60° to 120° heading delta against the _smoothed_ heading, so once the smoothed heading drifts past 60° from truth every correct measurement is rejected and the lock is permanent; 59 % of locked tracks in run `baf20f02` never release, sitting at a median 106° from their direction of travel. Add a rejection counter that releases the lock and snaps to the measurement: [design doc](plans/lidar-heading-coherence-sprint-plan.md) `S`
- Course-alignment metric and lock telemetry (heading coherence D1.1, D1.2): nothing in the system measures whether a bounding box points where the vehicle is going. `AlignmentMeanRad` compares Kalman velocity against displacement, both of which are motion. Add per-track \|OBB heading − course\| percentiles, heading-source histograms and lock-run lengths to `AnalysisReport`: [design doc](plans/lidar-heading-coherence-sprint-plan.md) `S`
- Dimension freeze and fragment capture (heading coherence D1.4, D1.5): while the heading is locked the tracker updates only height, so one bad frame's dimensions persist for the track's life; a 0.11 m by 0.08 m fragment was associated to a 4.33 m car and became its size for 28 frames. Project cluster extents onto the locked axes and refuse sub-0.5 m clusters as measurements for confirmed metre-scale tracks: [design doc](plans/lidar-heading-coherence-sprint-plan.md) `S`
- Stop publishing deleted tracks (heading coherence D1.6): `deleted_track_grace_period` is an internal re-association window, but deleted tracks are streamed to clients with frozen state, and 45.8 % of all published track-frames in run `baf20f02` were ghosts. Exclude or flag them at the L9 adapter: [design doc](plans/lidar-heading-coherence-sprint-plan.md) `S`
- Objective function rewards the defect (heading coherence D2.3): `HeadingJitter` carries a negative weight and a fully locked heading has near-zero jitter, so the auto-tuner scores locking as success; `ActiveTracks` carries a positive log-scale weight, so splitting one vehicle into two scores better than tracking it once. Zero the jitter term, add course alignment, and band the track count: [design doc](plans/lidar-heading-coherence-sprint-plan.md) `S`
- Axis-selection likelihood test (heading coherence D2.1): take §2.3 of the geometry-coherent tracking proposal alone, choosing between the aligned and 90°-swapped interpretation of each OBB observation against a running dimension mean. Unlike Guard 3 it always yields an answer, so it cannot deadlock; replaces Guard 2 and demotes Guard 3 to a counter: [design doc](plans/lidar-heading-coherence-sprint-plan.md), [proposal](../data/maths/proposals/20260222-geometry-coherent-tracking.md) `M` {math}
- Shape-aware association gate (heading coherence D2.2): the gate is a 6 m position radius with no size term, which is how a fragment captures a car; 98.2 % of frames in run `baf20f02` contain a co-located track pair within 3 m. Add a dimension-consistency term to the assignment cost: [design doc](plans/lidar-heading-coherence-sprint-plan.md) `M` {math}
- Per-run alignment panel on the 8080 run page (heading coherence D2.4): course-alignment distribution, heading-source histogram, lock-run lengths and co-located-pair count on the existing Svelte run detail page, consuming the extended run API rather than adding an endpoint: [design doc](plans/lidar-heading-coherence-sprint-plan.md) `M`

**Close without merge**

- Fix D of the OBB heading stability review, tightening `obb_aspect_ratio_lock_threshold` from 0.25 to 0.15 or 0.10, should be closed rather than validated. Measurement on run `baf20f02` shows lock entry is the failure mode, not lock absence: 55 % of tracks lock and 59 % of those never recover. Tightening the threshold increases lock entry: [design doc](plans/lidar-heading-coherence-sprint-plan.md)

## 7. Risks

| Risk                                                                                                  | Handling                                                                                                                                                            |
| ----------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Releasing the lock reintroduces visible 90° snapping, which is what Guard 3 was added to stop         | D2.1 is the real answer and lands the next day. If D1.3 alone looks worse in the visualiser, hold it behind the config default until D2.1 is in                     |
| Excluding `DELETED` tracks changes what the visualiser draws and may look like tracks vanishing early | The grace period stays internal for re-association. If the visual gap matters, flag rather than exclude and let the client decide                                   |
| One 60 s run is a single sample from one site                                                         | The Day 1 gate should be re-measured on `kirk0` and at least one soma static capture before the objective change in D2.3 is treated as settled                      |
| Two days is not enough for D-04                                                                       | It is not attempted. This sprint builds the metric D-04 will be judged on, and removes the deadlock that makes the current system worse than its own design intends |

## 8. Open questions

1. Should `DELETED` tracks be excluded at the adapter or flagged and filtered by each client? Excluding is simpler and fixes both clients at once; flagging preserves the visualiser's ability to show a fading track deliberately.
2. Is `max_misses_confirmed: 15` correct at an effective 5 Hz observation rate? Fifteen misses is three seconds of coasting, and the association pattern in `baf20f02` alternates hit and miss almost every frame.
3. Does the near-edge measurement from [lidar-state-estimation-plan](lidar-state-estimation-plan.md) make the axis-selection test redundant, or do they compose? They address different halves: one fixes where the box is, the other fixes which way it points.
