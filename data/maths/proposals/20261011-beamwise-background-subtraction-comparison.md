# Beam-wise background subtraction against L3: comparison and evolution path

This note compares Baumann, Vosshans and Dang's beam-wise background models with the L3 grid,
says where L3 would have to change to reach their results, and prices each change in this
framework. It is written to be challenged: Section 9 lists the claims with what would falsify
each one.

- **Status:** Comparison and proposal. Both code bases and the paper read; no experiment run;
  prepared for adversarial review
- **Layers:** L3 Grid ([internal/lidar/l3grid](../../../internal/lidar/l3grid)), with L4 noise
  rejection as the comparable post-step
- **Subject:** _Beam-Wise Statistical Background Subtraction for Static Roadside LiDAR: A
  Cross-Sensor Benchmark Study_, arXiv 2608.14868, accepted at IEEE ITSC 2026 (`Baumann2026`),
  and its code, [HighwayScene/roadside-lidar-background-subtraction][repo] at commit `9ecf7db`
  (MIT)
- **Related:** [Gap analysis §4 and §11](../paper-implementation-gap-analysis.md) (B1, B3, B7,
  B8, HW1), [background grid maths](../background-grid-settling-maths.md),
  [weekly briefing proposal][weekly], [window briefing][window],
  [scene-change recovery sprint](../../../docs/plans/lidar-scene-change-recovery-sprint-plan.md),
  backlog item "Score L3 foreground extraction against point-wise labels (D-28)"
- **Reviewed:** [adversarial review](20261011-beamwise-background-subtraction-review.md), same day: two
  claims fall, three are revised by simulation, and the review's own finding (L3 has no policy for
  an object that arrives and stays) outranks the note's. Read both

[repo]: https://github.com/HighwayScene/roadside-lidar-background-subtraction
[weekly]: ../../../docs/lidar/operations/brief/research-briefing-2026-10-10.md#proposal-score-l3-against-point-wise-labels
[window]: ../../../docs/lidar/operations/brief/research-briefing-2025-01-to-2026-10.md#background-subtraction-benchmarks-baumann-vosshans-and-dang-2026-iglesias-et-al-2025

## 1. Question and answer

**Question.** Does the paper offer a better background model than L3? Where would L3 have to
evolve to reach its results, and what does each step cost here?

**Answer.**

1. **Better is not demonstrated, and the models are not drop-in.** No number in either
   repository scores the two on the same labels. The paper's models are fitted once on 600
   frames, frozen, tuned per scene against labels, and scored on a highway where "vehicles do
   not stop". L3 has to adapt online for days at sites nobody labels.
2. **The property that matters most is one our own audits already found missing:** an
   acceptance window learned per beam ($z\,\sigma_b$, with a floor) rather than imposed by range.
   At the median settled cell, the window L3 is configured with is 2× wider than a QG window
   with the paper's Ouster parameters within 10 m and 33× wider past 100 m. The production
   region multipliers widen it again, to 6× and 129× (Section 5). On a beam whose background is
   the road, the cost does not grow with range. The window absorbs a fixed height band above
   the road: about 15 cm as configured, 35 to 46 cm under the region multipliers, and 1 to 4 cm
   for QG.
3. **Their scores come from a two-stage recipe.** A permissive model, tuned for recall (F2),
   feeds a spatial density filter tuned for F1, and the filter restores precision. With the
   filter on, the F1 spread between models narrows 1.8 to 3.2 fold. L3 runs the opposite
   recipe: a conservative model, and no density test until DBSCAN.
4. **Three of their mechanisms transfer cheaply to an online model:** the learned window, the
   one-sided occlusion test of the coarse-fine triangle algorithm (CFTA), and a range-image
   support filter. Two more cost moderately: robust far-tail statistics and a per-cell return
   probability. Multi-modal cells, the GMM that wins their highway table, cost the most and have
   the least evidence behind them for our scenes.
5. **Some of it should not transfer:** frozen models, per-scene Bayesian tuning against labels,
   and the 600-frame batch fitted up front. On the last point, our VRLOGs do not carry
   background returns, so a batch fit would have to replay a PCAP.

## 2. What the paper's code does

Read from the repository at `9ecf7db` and the arXiv HTML text, both on October 11, 2026. Code
paths are relative to `src/static_bg_subtraction/`.

### 2.1 Shared front end

- **Range matrix.** A `V × H × T` float32 array stores the **minimum** range per (channel,
  horizontal index) cell per frame, and 0 where the cell returned nothing
  (`processing/range_matrix.py`). Native channel and horizontal IDs are used when the dataset
  provides them; otherwise the indices come from angles. Training uses `T = 600` frames, about a
  minute at 10 Hz. The maximum range is 100 m on HighwayScene and 150 m on CoopScenes.
- **Unmodelled beams.** A beam with returns in fewer than `min_sample_ratio` of the training
  frames gets no model. Any return in it is foreground: in QG and GMM the mask defaults to
  foreground, and CFTA sets the threshold to the maximum range.
- **No update at inference.** QG, GMM and CFTA are fitted once. MOG2 runs with a learning rate
  of zero. The paper names online adaptation as future work.

### 2.2 The models

| Model                  | Per-beam statistic                                                                                                                                                                                                                                                                                                                                              | Decision                                                                                             | Ouster parameters (HighwayScene)                                                                     |
| ---------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------- |
| CFTA, density-aware    | Range histogram. Noise case (valid-return fraction below 0.02): threshold at max range. Coarse 1 m bins: the farthest of the top N peaks holding a minimum share, then trim beyond it plus an offset. Fine 200 bins: the dominant peak, or the farthest qualifying one. Triangle rule: the threshold is the bin farthest below the line from bin 0 to that peak | **One-sided:** background iff $r \ge \theta_b$                                                       | coarse top 2 at 40%, offset 6 m; fine top 7 at 15%, dominance 0.8                                    |
| Quantile Gaussian (QG) | Samples with $r \ge Q_b(q)$, falling back to $q - \Delta$. $\mu_b$ = median; $\sigma_b = 1.4826\,\mathrm{MAD}$, clipped to [0.05, 10] m                                                                                                                                                                                                                         | Background iff $\lvert r - \mu_b\rvert \le z\,\sigma_b$                                              | $q$ 0.48, $\Delta$ 0.15, $z$ 4.8, min sample ratio 0.22                                              |
| Beam-wise GMM          | 1-D mixture, $K \le 3$ chosen by BIC (scikit-learn EM, k-means initialisation). Background components need $w_k \ge w_{min}$ and $\sigma_k \le \sigma_{max}$                                                                                                                                                                                                    | Background iff within $z\,\sigma_k$ of a background component; with none, every return is foreground | $w_{min}$ 0.36, $\sigma_{max}$ 0.22 m, $z$ 5.75, variance regulariser 0.001 m² ($\sigma \ge$ 3.2 cm) |
| Image MOG2             | OpenCV MOG2 on an 8-bit range image (one grey level is 0.39 m at 100 m)                                                                                                                                                                                                                                                                                         | MOG2's own variance test                                                                             | history 600, variance threshold 0.5                                                                  |

CFTA expands to coarse-fine triangle algorithm (Zhang and Jin, 2022); the original is in the
optional MATLAB submodule. "Density-aware" is the paper's contribution: it splits beams by
valid-return fraction into noise, sparse and dense cases before the peak search.

### 2.3 The spatial filters

All three only flip foreground to background, never the reverse. Runtimes are the paper's
Table II for the Aeva and GMM, on an AMD 9950X running multithreaded.

| Filter (config key)                                | Operation                                                                                                                                                     | F1     | ms per frame |
| -------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------ | ------------ |
| None                                               |                                                                                                                                                               | 86.26% |              |
| Radius (`classic_expensive`)                       | 3-D neighbour count among foreground points, radius $\mathrm{clip}(s\,r\tan\Delta\theta,\ r_{min},\ r_{max})$; keep if at least $n_{min}$                     | 97.59% | 5.95         |
| Grid, range similarity (`grid_simple`)             | In index space, nearest foreground return per cell; count foreground cells within $\pm d_v \times \pm d_h$ whose range differs by at most $\tau_0 + \tau_1 r$ | 97.24% | 22.41        |
| Grid, model consistency (`grid_model_consistency`) | Neighbouring beams' background models vote on the return                                                                                                      | 93.36% | 38.74        |

The model-consistency filter is the operation nearest to L3's neighbour confirmation, and it
scores worst. Its QG branch cannot run: it reads `_k_grid` and `gate_k`, which neither the QG
model nor its config has, so only the GMM and CFTA variants were exercised.

### 2.4 Protocol and results

- **HighwayScene.** An Ouster OS0, an Aeva Aeries II and a Blickfeld QB2 at a highway
  construction site with a 40 km/h limit; 4,000, 1,000 and 997 frames for train, validation and
  test. Licence CC BY-NC-SA 4.0, 26.3 GB. Labels are not distributed. They are computed at
  evaluation time: for the Ouster and Blickfeld, every point inside fixed lane volumes is
  dynamic; for the Aeva, a point is dynamic when its **radial** speed exceeds 1 m/s.
- **CoopScenes.** An Ouster OS2 infrastructure sensor at four urban scenes; 76 labelled frames
  (2,455 objects, 857 dynamic), about half for validation and the rest for test, so roughly 9 to
  10 test frames per scene. Foreground is 0.59% to 3.14% of points.
- **Tuning.** Two stages, both on the validation split, by Bayesian sweep (trial counts not
  reported): model parameters for F2 with filtering off, then filter parameters for F1.
  CoopScenes models are fitted and tuned per scene.

HighwayScene, foreground F1 on test, without and with the tuned filter:

| Model               | Ouster        | Blickfeld     | Aeva          |
| ------------------- | ------------- | ------------- | ------------- |
| CFTA (original)     | 93.82 / 93.94 | 81.32 / 81.81 | 81.00 / 81.54 |
| CFTA, density-aware | 95.67 / 98.22 | 93.16 / 94.42 | 85.96 / 97.36 |
| QG                  | 95.29 / 99.25 | 91.19 / 95.98 | 77.93 / 94.40 |
| GMM                 | 96.78 / 99.43 | 94.42 / 96.95 | 86.26 / 97.59 |
| MOG2                | 92.38 / 96.97 | 86.46 / 93.67 | 76.00 / 94.84 |

The original CFTA's "with" column uses MATLAB's `pcdenoise`, not the paper's filters. Without a
filter, precision is the weak side, for example the Aeva GMM's 76.6% precision at 98.7% recall,
which becomes 98.6% and 96.6% with it.

CoopScenes, F1 with the filter: density-aware CFTA 93.82, 93.12, 91.51, 96.61 (mean 93.77);
MOG2 93.94, 92.73, 90.19, 95.73 (93.15); QG 92.38, 91.42, 88.44, 94.91 (91.79); GMM 86.73,
91.97, 85.85, 96.72 (90.32); original CFTA 77.70, 66.48, 64.37, 65.35. The paper attributes
Scene 3's errors to a vehicle present in training and absent at test.

## 3. What L3 does

Covered in full by the [background grid maths](../background-grid-settling-maths.md) and gap
analysis §4. The facts the comparison turns on:

- **Grid.** 40 rings × 1,800 azimuth bins (0.2°), hard-coded at every construction site
  ([radar.go](../../../internal/cmd/server/radar.go), replay, bench, pcap-split,
  settling-eval). At 600 RPM that is about one return per cell per frame.
- **Per point, online, closed loop.** `ProcessFramePolarWithMaskAt`
  ([foreground.go](../../../internal/lidar/l3grid/foreground.go)) classifies each return and
  updates the cell only from returns it accepts as background. The mean is an EMA with
  $\alpha = 0.02$. The spread is an EMA of $\lvert x - \mu_{old}\rvert$ over accepted returns:
  about $0.8\sigma$, truncated at the window (B1). It is not a Welford variance.
- **Symmetric window imposed by range.**
  $W = k\,(s + k_n r + 0.01)\cdot\mathrm{warm}(n) + \text{safety}$ with $k = 3$, $k_n = 0.02$
  and safety 0.15 m. The range term is 86% to 98% of the window (HW1).
- **Region overrides.** At settle, cells are split into spread terciles. `effectiveCellParams`
  then multiplies $k_n$ by 4 for the most stable third, 3 for the middle and 8 for the most
  variable, and adds 2 to the neighbour count for the variable third (B8,
  [background_region.go](../../../internal/lidar/l3grid/background_region.go)).
- **Other acceptance paths, combined by OR.** A locked baseline (snapshot at 50 observations,
  then $\beta = 0.001$), and same-ring neighbour confirmation (at least 3 of the ±3 bins agree).
  Neighbours along one ring only, never across elevation.
- **Control logic.** Freeze for 5 s on strong divergence while $n < 100$, a deadlock breaker at
  the confidence floor, a ×5 reacquisition boost, warmup suppression in live mode, and
  `seed_from_first = true`, so an empty cell's first return becomes its background.
- **Nothing between L3 and DBSCAN tests density.** The ground height band (−2.8 m to 1.5 m in the
  sensor frame) does not reach the road at a 2.3 m mount, voxel downsampling is off live, and
  DBSCAN is XY-only with eps 0.8 m and minPts 5, neither of them range-adaptive
  ([tracking_pipeline.go](../../../internal/lidar/pipeline/tracking_pipeline.go)).

## 4. Algorithmic differences

| #   | Dimension                   | Paper                                                                                                                                                                               | L3                                                                                      | Consequence                                                                                                                                            |
| --- | --------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| D1  | Learning regime             | Batch over 600 frames, then frozen                                                                                                                                                  | Online, per point, updates only from accepted returns                                   | Theirs cannot follow a changed scene. Ours can be captured by what it accepts                                                                          |
| D2  | Background statistic        | Far-tail median (QG), farthest dominant histogram peak (CFTA), weighted mixture (GMM)                                                                                               | EMA mean of accepted returns                                                            | Theirs is robust to traffic in the training data by construction. Ours relies on control logic for the same protection (D9)                            |
| D3  | Spread statistic            | $1.4826\,\mathrm{MAD}$ of the far tail, open loop                                                                                                                                   | EMA of absolute deviation about the old mean, closed loop                               | Ours is biased low by truncation once the window is narrow (Section 6, E1)                                                                             |
| D4  | Acceptance window           | $z\,\sigma_b$, floored (QG ≥ 0.24 m, GMM ≥ 0.18 m), independent of range                                                                                                            | $k(s + k_n r + 0.01) + 0.15$, 86–98% range term, ×3 to ×8 after settle                  | The dominant difference. Section 5                                                                                                                     |
| D5  | Decision symmetry           | CFTA one-sided; QG and GMM symmetric                                                                                                                                                | Symmetric                                                                               | A return farther than the background is "background changed" to CFTA and foreground to L3. The paper's Scene 3 errors are the symmetric case's failure |
| D6  | Modality                    | GMM keeps up to 3 modes; background is the heavy, narrow ones                                                                                                                       | One mode plus a locked baseline                                                         | Two-depth cells (foliage edges, gates, dual-return edges) oscillate or absorb both depths with a wide spread (B3)                                      |
| D7  | Sparse and unmodelled beams | Return fraction is a first-class input; a beam with too few returns has no model, so its returns are foreground                                                                     | No return probability; `seed_from_first` makes an empty cell's first return background  | Tall-vehicle tops in near-horizon cells, and first passes through rarely hit cells, are absorbed by L3 and kept by theirs                              |
| D8  | Spatial consistency         | Post-filter on foreground only, 2-D index window or 3-D range-scaled radius                                                                                                         | Same-ring rescue inside the decision, then XY DBSCAN                                    | Theirs removes isolated false foreground. Ours rescues returns into the background, which lowers recall                                                |
| D9  | Contamination handling      | Statistics. QG's spread stays clean while traffic occupies less than $q$ of training frames, and its median until $(1+q)/2$; CFTA needs the background peak to hold a minimum share | Control logic: freeze, locked baseline, deadlock breaker, reacquisition boost           | Each control path has its own failure. The boost may cost detection (B4)                                                                               |
| D10 | Per-frame aggregation       | Nearest return per cell per frame, for training                                                                                                                                     | Every return; in dual-return mode a firing updates a cell twice, possibly at two depths | L3 averages first and last returns at edges. Whether deployments run dual return is unknown                                                            |
| D11 | Tuning                      | Per scene, Bayesian, against validation labels                                                                                                                                      | Global defaults; the label-free settling criterion                                      | Their parameters do not transfer between scenes (QG $z$ 2.2 to 30; radius filter $n_{min}$ 6 to 88). Ours must work untuned                            |
| D12 | Cold start                  | 600 frames before any output                                                                                                                                                        | 30 s or 50 frames of suppressed output live, then settling                              | Comparable. Theirs needs no clean frames; neither does ours                                                                                            |
| D13 | Cost                        | Inference: one gather and compare (QG), $K$ compares (GMM). Training: per-beam EM in Python                                                                                         | Per point: cell window, locked window, 6 to 10 neighbour tests                          | Their inference is cheaper than ours. Training a GMM per cell on a Pi takes seconds to minutes, offline                                                |

## 5. The window, quantified

Settled-cell windows at each band's median measured spread, from the 2026-09 closeness audit
(gap analysis HW1). QG and GMM use the paper's Ouster HighwayScene parameters, converting our
spread to σ with $\sigma \approx s / 0.798$ (Gaussian noise assumed). Warmup multiplier 1.

| Band      | Range used | L3 as configured | Stable region (×4) | Volatile region (×8) | QG     | GMM    | Configured ÷ QG | ×4 ÷ QG |
| --------- | ---------- | ---------------- | ------------------ | -------------------- | ------ | ------ | --------------- | ------- |
| 0–10 m    | 5 m        | 0.50 m           | 1.40 m             | 2.60 m               | 0.24 m | 0.19 m | 2.1             | 5.8     |
| 10–20 m   | 15 m       | 1.10 m           | 3.80 m             | 7.40 m               | 0.24 m | 0.19 m | 4.6             | 15.8    |
| 20–30 m   | 25 m       | 1.71 m           | 6.21 m             | 12.21 m              | 0.24 m | 0.20 m | 7.1             | 25.9    |
| 30–50 m   | 40 m       | 2.62 m           | 9.82 m             | 19.42 m              | 0.24 m | 0.20 m | 10.9            | 40.9    |
| 50–75 m   | 62 m       | 3.97 m           | 15.13 m            | 30.01 m              | 0.24 m | 0.24 m | 16.5            | 63.0    |
| 75–100 m  | 88 m       | 5.54 m           | 21.38 m            | 42.50 m              | 0.24 m | 0.26 m | 23.1            | 89.1    |
| 100–200 m | 150 m      | 9.32 m           | 36.32 m            | 72.32 m              | 0.28 m | 0.38 m | 33.0            | 128.5   |

QG's 0.05 m σ floor binds in every band but the last: the Pandar40P in situ is quieter than
their floor assumes, so the QG window is the floor's, not the cell's.

**What the window costs is a height, not a range.** Take a beam from a mount at height $H$ that
falls at depression $\theta$ and would hit the road behind a vehicle. A vehicle point at height
$h$ above the road lies $h/\sin\theta$ in front of the road along that beam. It is absorbed
when $h/\sin\theta \le W(r)$. With $W \approx k k_n r + c$, where $c = k(s + 0.01) +$ safety,
and $r \sin\theta \approx H - h$:

$$h_{abs} \approx \frac{k\,k_n\,H + c\sin\theta}{1 + k\,k_n}$$

The range cancels. At $H = 2.3$ m:

| L3 setting           | $k\,k_n$ | Absorbed height above the road | QG at the same beam |
| -------------------- | -------- | ------------------------------ | ------------------- |
| As configured        | 0.06     | about 0.15 m                   | 0.01 to 0.04 m      |
| Variable region (×3) | 0.18     | about 0.36 m                   |                     |
| Stable region (×4)   | 0.24     | about 0.46 m                   |                     |
| Volatile region (×8) | 0.48     | about 0.76 m                   |                     |

Road cells are low-variance, so they most likely fall in the stable or variable tercile. If
they do, the lowest 35 to 46 cm of every vehicle and pedestrian, on every beam that would
otherwise hit the road behind them, is classified as road. Neighbour confirmation can only widen
this. Where the background is a wall or a fence, the absorbed depth is the gap between the
object and the wall, and this arithmetic does not apply. The gap analysis framed HW1 as a
detection floor that grows with range. For road-backed beams it is a constant band, and the
region multipliers set its size.

This is arithmetic on the code and the measured spreads, not a measurement of misses. Section 9,
C2 gives the check.

## 6. Where L3 would evolve, and what it costs

### 6.1 Framework constraints that price every step

- **There is no L3 engine dispatch.** `l3.engine` selects a parameter block. The live server
  refuses anything but `ema_baseline_v1`
  ([lidar_helpers.go](../../../internal/cmd/server/lidar_helpers.go)). Replay, bench and
  settling-eval skip that guard and build from `ActiveCommon()`, so a sweep arm naming
  `ema_track_assist_v2` silently runs the EMA code. A real second engine needs an interface in
  place of `*l3grid.BackgroundManager` in the tracking pipeline config, five construction sites,
  the runtime tuning paths hard-coded to `l3.ema_baseline_v1.*`, about 30 accessors over
  `ActiveCommon()`, and the codec and coverage tests. The `ForegroundExtractor` interface
  (backlog #390) is that seam, and it is not built. **Cheapest route:** default-off keys inside
  `ema_baseline_v1`.
- **Fingerprints.** A changed default moves the tuning fingerprint, and the perf baselines must
  be recaptured in the same change. A code change behind an unchanged config shows only as
  work-counter drift in the perf gate.
- **The evaluation tools do not see L3's misses.** settling-eval measures convergence on the
  batch path (`ProcessFramePolar`), which production does not call. The closeness audit
  measures windows. The sweep and annotation-scored tuning score clusters or tracks. Annotation
  packs are foreground only, so a return L3 absorbed never reaches a label (the
  [weekly briefing][weekly] proposal).
- **VRLOGs carry foreground per frame plus settled snapshots every ~15 s.** A batch-fitted model
  needs raw returns, which means a PCAP replay.
- **Persistence.** The grid is gob-encoded and restored by an exact-match scene signature. Any
  new per-cell state changes the snapshot schema, the exporter, the closeness audit and the
  VRLOG settled-snapshot payload. That is the hidden cost in E5 and E6.

### 6.2 The steps

| Step | Change                                                                                                                                                                                                                                                                             | Where                                                              | Memory per cell (72,000 cells)                         | Size | Main risk                                                                                                                                          |
| ---- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------ | ------------------------------------------------------ | ---- | -------------------------------------------------------------------------------------------------------------------------------------------------- |
| P0   | Point-wise scoring harness on `ProcessFramePolarWithMaskAt`, per range band, post-warmup                                                                                                                                                                                           | New tool beside settling-eval                                      | None                                                   | M    | Scoring the batch path by mistake                                                                                                                  |
| P1   | Default-off arm keys; make replay and bench reject an unsupported engine                                                                                                                                                                                                           | `internal/config`, `l3grid/config.go`                              | None                                                   | S    | None                                                                                                                                               |
| E1   | **Learned window.** $W = z\cdot\max(\sigma_{floor}(r),\ s/0.798) + \text{safety}$, with $\sigma_{floor}$ from the Hesai spec (0.02 m to 30 m, 0.03 m beyond); the same expression for the neighbour and locked windows; region overrides of $k_n$ become inert or are switched off | `background_closeness.go`, `foreground.go`, `background_region.go` | None                                                   | S    | More false foreground, so it ships with E4. Truncation: a narrow gate starves the spread estimate (below)                                          |
| E2   | **One-sided decision.** $r < \mu - W$ is foreground. $r > \mu + W$ counts as evidence of a changed background, and after $N$ consecutive far returns the cell relearns there                                                                                                       | `foreground.go`                                                    | +2 B                                                   | S–M  | See-through backgrounds (hedges, fences, glass): a car behind a hedge is lost. Interacts with freeze, the locked baseline and the deadlock breaker |
| E3   | **Return probability.** Hit count over frames since first seen; below a floor the cell has no background and `seed_from_first` does not apply                                                                                                                                      | `foreground.go`, `BackgroundCell`                                  | +4 to 8 B                                              | S    | Persistence schema; near-horizon cells flicker                                                                                                     |
| E4   | **Range-image support filter.** After the mask, nearest foreground range per cell; count foreground cells within $\pm d_v$ rings (in elevation order) × $\pm d_h$ bins with $\lvert\Delta r\rvert \le \tau_0 + \tau_1 r$; under the support threshold, flip to background          | New function between L3 and L4, reusing the grid index             | None (one reused 72,000-float buffer)                  | S–M  | Pandar40P ring spacing is non-uniform, so the window must be in elevation order. Small, distant objects lose support                               |
| E5   | **Robust far-tail statistic online.** A Robbins–Monro quantile tracker, $q \leftarrow q + \eta(\tau - \mathbb{1}[r < q])$, with an EMA of absolute deviation about it; or a compact per-cell histogram (CFTA-like, 64 bins of uint16)                                              | `foreground.go`, persistence, exporter, audit                      | +8 B (tracker) or +128 B, about 9 MB (histogram)       | M    | Step size versus drift; schema change through every snapshot consumer                                                                              |
| E6   | **Online mixture, $K$ = 2 or 3** (Stauffer–Grimson ranking by $w/\sigma$); generalises the locked baseline                                                                                                                                                                         | `foreground.go`, settling criterion, regions, persistence          | +24 to 36 B, about 2.6 MB                              | L    | Settling and region logic assume one spread per cell; least evidence of need                                                                       |
| E7   | **Offline warm start.** Fit QG or CFTA over the first $N$ frames of a PCAP and hand the result to the online model                                                                                                                                                                 | replay, persistence                                                | Batch only: 40 × 1,800 × 600 × 4 B = 173 MB as float32 | M    | Not available from VRLOGs; the range matrix is too large for a Pi without streaming                                                                |

**Truncation in E1.** A closed-loop spread learned only from returns inside $\pm z\sigma$ is
biased low, because the tails are cut. With Gaussian noise at $z = 4.8$ the loss is negligible.
With heavy-tailed noise (mixed returns at edges) the estimate shrinks, the window shrinks with
it, and the loop can tighten until the floor stops it. QG avoids this by estimating open loop
from all far-tail samples. Three remedies: keep the floor; learn the spread through a wider gate
than the one that classifies; or update it from every return that is not in front of the
background, which pairs naturally with E2.

**Recommended order.** P0, P1, then E1 with E4 and the region overrides off, then E2, then E3.
E5 only if P0 shows the EMA mean captured by slow traffic. E6 only if full-scene labels show
two-depth cells costing recall. E7 is a research arm, not a product path.

## 7. The experiment that answers "better?"

**Arms.** A0 shipped L3. A1 region overrides off. A2 A1 plus E1 ($z$ = 4 and 6). A3 A2 plus E2.
A4 A2 plus E4. R1 and R2 are reference arms: QG and density-aware CFTA ported to Go, fitted on
the training split and frozen, scored by the same evaluator. Every arm runs with and without
L4's DBSCAN noise rejection, because their "with filter" numbers compare to L3 plus L4, not to
L3 alone.

**Data, in order of cost.**

1. **Existing kirk0 and kirk1 PCAPs**, scored on labelled tracks (with the M5 caveat), plus the
   height check in C2 on the reviewed masks. No new labels.
2. **CoopScenes point labels and HighwayScene Ouster test frames**, through P0, with L3
   warm-started on the training split so that it sees the same history as the batch models.
   Report by range band. Report frame-bootstrap intervals, because CoopScenes has about 9 test
   frames per scene.
3. **kirk0 labelled full scene**, not foreground only. This is the evidence that matters for
   the product.
4. **Long duration:** a parked-car arrival and departure, the Lombard–Laguna scene change, and
   CPU per frame on the Pi.

**Decision rule, set before running.** Adopt E1 and E4 as defaults if, on data 2 and 3,
point-wise recall rises by at least 5 points in every band past 10 m. Post-L4 precision must not
fall by more than 1 point, the kirk tracks must not regress, and the Pi frame budget must hold.
Adopt E2 if it shortens false-track persistence at Lombard–Laguna without losing a labelled
vehicle behind a permeable background.

**What it will not establish.** An OS0 at a highway construction site and an OS2 at a Stuttgart
transit stop are not a Pandar40P on a residential street. Their scores say how a method behaves,
not how the product does. Only data 3 says that.

## 8. Where the paper's evidence is weaker than its headline

1. **Traffic never stops on HighwayScene.** A frozen model has no stopped vehicles to absorb,
   and no parked car to leave. The one urban failure the paper describes is exactly that case.
2. **The lane-volume labels measure where a point is, not whether it moves.** Inside a lane
   volume a point is dynamic; outside, static. The method is sound for free-flowing traffic,
   with one geometric question. The main volumes are pitched 0.3° over 100 m. If the road
   follows the fitted ground plane, their floors run from 12 to 27 cm below it to 25 to 40 cm
   above, and 20 to 45% of the plane under them lies inside a dynamic volume (computed from
   the published config). The reported 99.7% recall argues that the volumes follow the road's
   grade instead. One frame would settle it.
3. **The Aeva labels see radial speed only.** A vehicle crossing the beam near its closest point
   of approach reads as static.
4. **Per-scene tuning.** Parameters vary by an order of magnitude between scenes. The transfer
   claim holds for the method family, not for any one configuration.
5. **Small CoopScenes test sets.** About 9 to 10 frames per scene, at 0.6% to 3.1% foreground.
   No intervals are reported, and the top three methods' means sit within 2 points of each
   other.
6. **The filter carries the precision.** Models are tuned for F2 without the filter, then the
   filter is tuned for F1 per model, so the "with filter" column partly measures the filter's
   tuning budget.
7. **Runtime is reported for the filters only,** on a 16-core desktop. Model inference and
   training times are absent.

## 9. Claims for challenge

Confidence is for the claim as stated. Each claim names what would falsify it.

| #   | Claim                                                                                                                                                                                                                   | Confidence                                   | Falsified by                                                                                                                                                                              |
| --- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| C1  | For detection, the window is the decisive difference, more than the statistic behind it                                                                                                                                 | Medium-high                                  | E1 recovering no more labelled tracks than `closeness_multiplier` 1.5 to 2 already does (13 of 16 on kirk0, 22 to 33 of 49 on kirk1), or no point-wise recall gain on data 2              |
| C2  | On road-backed beams L3 absorbs a constant band above the road, about 15 cm as configured and 35 to 46 cm with region overrides                                                                                         | High for the arithmetic; medium for the data | Heights above the road of foreground points in kirk0's reviewed vehicle masks showing no floor near that band. First confirm that the replay which made the pack applied region overrides |
| C3  | Most of their headline gain comes from the spatial filter, not the beam model                                                                                                                                           | Medium                                       | An ablation with one fixed filter configuration across models that restores the no-filter spread between them                                                                             |
| C4  | Narrowing L3's window without a support filter will mostly add false clusters                                                                                                                                           | Medium                                       | A2 without E4 matching A4's post-L4 precision on data 2                                                                                                                                   |
| C5  | A one-sided decision removes the revealed-surface ghosts. L3 holds one for roughly as many observations as the departed object contributed: one count is drained per foreground return, and no test drains a high count | Medium                                       | A3 failing to shorten false-track persistence at Lombard–Laguna, or a replay showing the deadlock breaker firing far sooner than the count predicts                                       |
| C6  | `seed_from_first` and the absence of a return probability absorb returns in rarely hit cells                                                                                                                            | Medium-low on magnitude                      | Foreground recall in cells with a hit ratio under 2% matching recall elsewhere                                                                                                            |
| C7  | Any port must keep online adaptation                                                                                                                                                                                    | High                                         | A frozen R1 or R2 holding its score across a day of kirk captures                                                                                                                         |
| C8  | Their parameters do not transfer between scenes; a deployed L3 cannot be tuned against labels                                                                                                                           | High                                         | One scene's configuration scoring within 2 F1 points on another                                                                                                                           |
| C9  | CoopScenes does not separate density-aware CFTA, MOG2 and QG                                                                                                                                                            | Medium                                       | Frame-bootstrap intervals that do not overlap                                                                                                                                             |
| C10 | CoopScenes, not HighwayScene, is the closer analogue to our sites, and there the one-sided histogram method leads while the highway's winner, GMM, is last of the four tuned models                                     | Medium                                       | Site characteristics (parked vehicles, pedestrians, mount height) that match HighwayScene better                                                                                          |
| C11 | Closed-loop spread under a narrow gate is biased low and can collapse                                                                                                                                                   | Medium-high                                  | A simulation with heavy-tailed edge noise in which the spread stays unbiased without a floor                                                                                              |
| C12 | In dual-return mode L3 averages first and last returns at edges                                                                                                                                                         | High from the code; unknown in deployment    | The ReturnMode byte in kirk0's packets showing single return                                                                                                                              |

## 10. Open questions

1. Which return mode do our deployments run?
2. Which variance tercile do kirk0's road cells fall into, and did the replays behind the
   reviewed masks apply region overrides?
3. How long does a revealed surface stay foreground in practice, and is the 4-minute persistence
   after Lombard–Laguna's scene change that mechanism?
4. How many beams has the HighwayScene OS0, and do its records carry channel and horizontal
   indices, which an adapter needs to map points to (ring, azimuth bin)?
5. HighwayScene is CC BY-NC-SA 4.0. Scoring is non-commercial research use, but derived labels
   would carry share-alike, so data and labels stay outside the repository and only numbers
   come in. The CoopScenes dataset card says MIT. The licence of the `benchmark/` labels is
   unread.
6. HighwayScene's mount height is not stated. Its ground-truth plane puts the OS0 about 2.09 m
   above the road, close to our 2.3 m, if that plane is in the sensor frame.

## 11. Provenance and errata

**Read.** The repository at `9ecf7db`, "Initial public release" (the README, `docs/`, every
model and filter module, the evaluation and ground-truth modules, the paper run configurations,
the search spaces and `configs/paper/reference_results.json`). The arXiv HTML text. The
HighwayScene and CoopScenes dataset cards. This repository at `a1512d63b`. Sections 5 and 8.2 are
arithmetic on the formulas and the published configuration, run as throwaway scripts outside the
repository. No dataset was downloaded and no replay was run.

**Errata found in sibling documents.**

- The [window briefing][window] expanded CFTA as a "cell-free-time baseline". It is the
  coarse-fine triangle algorithm. The same paragraph gave L3 a Welford variance. Both are
  corrected in the same change as this note.
- [AGENTS.md](../../../AGENTS.md) describes L3 as "EMA + Welford variance". The spread is an EMA
  of absolute deviation (B6). Not edited here.
- [av-range-image-format-alignment.md](../../../docs/lidar/architecture/av-range-image-format-alignment.md)
  says only the first return is parsed. L1 parses every block, so both returns reach L3 in
  dual-return mode. Not edited here.
