# Experiment E1: lateral-error validation on the three-site corpus

- **Status:** Recorded. E1.1 and E1.3 run and reproduced at three independent placements; the hypothesis in Section 3 is confirmed. A fourth placement, `clar0`, reproduces both — see the addendum below. E1.2 and E1.4 remain open.
- **Scope:** Three-site, 16-capture offline replay corpus; 43,068 scored frames, 195,389 immutable observations, 160,245 linked estimates
- **Repository revision:** `3e9b22fb4e22e14d819ea9c35069b3c83f4cdc38`
- **Source manifest:** `sha256:83c17249c48238abb748fb467ae6229c8b13ff7e5eea5f0cd57f5641c04252ba`
- **Frozen result:** [state-estimation-e1-20260916.json](state-estimation-e1-20260916.json) (`sha256:6ba6475e23674fb4e38433cb7488116cb5aaf0bd0ae5314600f1c3ea8d93bf92`)
- **Related:** [State estimation](../../plans/lidar-state-estimation-plan.md) §3 and §16.5, [Phase 0/1 corpus baseline](state-estimation-phase01-corpus-baseline.md), [archive index](../../../tools/s2-archive/site-index.json)

## Verdict

**The medoid's lateral error is an aspect-conditioned geometric bias, not noise.** It is near zero
when the sensor views a vehicle end-on and grows monotonically toward broadside, reaching **0.35 to
0.41 of the body's half-width** measured against a fitted path, and **0.58 to 0.65 of the
half-width** measured against the near-edge candidate. The trend reproduces at all three placements
and **survives control for range** in every well-populated stratum.

That is the signature Section 3 predicted and the discriminator Section 16.5 specified. Random
error has a conditional mean of zero in every bin; this does not. Phase 2's premise holds: the
defect is in the measurement definition, and no estimator can filter it out.

## What was run

`lidar-state-estimation-baseline` replayed the committed corpus through the production pipeline
with the immutable observation store enabled, then `lidar-e1-analysis` computed the candidate
measurements from the persisted evidence:

```bash
go run -tags=pcap ./cmd/tools/lidar-state-estimation-baseline \
  -existing-source-manifest manifest.json -out out/ -evidence-dir ev/ -duration 0 -warmup 70
go run ./cmd/tools/lidar-e1-analysis -observations ev/observations.db -manifest manifest.json
```

| Site                   | Captures | Scored frames | Observations | Estimates | Repeat identical |
| ---------------------- | -------- | ------------- | ------------ | --------- | ---------------- |
| `marina-webster-beach` | 4        | 11,320        | 44,374       | 39,407    | yes              |
| `columbus-broadway`    | 7        | 20,320        | 102,013      | 77,891    | yes              |
| `embarcadero-folsom`   | 5        | 11,428        | 49,002       | 42,947    | yes              |

The scored-frame total of 43,068 matches the [Phase 0/1
baseline](state-estimation-phase01-corpus-baseline.md) exactly, so this run analyses the same
population that baseline describes. The tuning is the committed default: the baseline tool sets the
observation sample cap itself and overrides the tuning key, so no tuning change was needed and the
config fingerprint is unmoved.

## E1.1: conditional mean by aspect — the decisive test

For every observation on a confirmed moving track, each candidate's signed lateral offset from a
total-least-squares path through that track's own estimates, binned by aspect angle and signed
toward the sensor. 651 tracks accepted of 4,199 (2,511 too short for the two-second window, 937
below the 3 m/s heading-observability floor, 100 not straight enough for a line to be a fair
reference), carrying 17,374 observations.

Medoid conditional mean, as a fraction of the body's half-width:

| Aspect | `columbus-broadway` | `embarcadero-folsom` | `marina-webster-beach` |
| ------ | ------------------- | -------------------- | ---------------------- |
| 0–15°  | 0.05                | −0.08                | 0.08                   |
| 15–30° | 0.11                | 0.29                 | 0.05                   |
| 30–45° | 0.10                | 0.15                 | 0.18                   |
| 45–60° | 0.22                | 0.18                 | 0.36                   |
| 60–75° | 0.33                | 0.38                 | 0.39                   |
| 75–90° | **0.35**            | **0.41**             | **0.37**               |

Near-edge candidate, same bins, for comparison: −0.19 to −0.07 at Columbus, −0.29 to −0.02 at
Embarcadero. Flat, and of the opposite sign.

A near-zero lateral bias at 0–15° is the correct result rather than a weak one: end-on is where the
_end_ face dominates, so the bias there lies along the body's length axis, not across it.

## E1.3: cross-measurement disagreement, stratified by range

Needs no fit and no truth. Mean of (medoid − near edge) along the body's lateral axis, signed toward
the sensor, as a fraction of the half-width. The question Section 16.5 poses is whether the aspect
trend survives _inside_ a range band: the hypothesis predicts it does, a range-correlated artefact
such as the P11 grade defect predicts it vanishes once range is controlled.

**`columbus-broadway`**

| Range   | 0–15° | 15–30° | 30–45° | 45–60° | 60–75° | 75–90° |
| ------- | ----- | ------ | ------ | ------ | ------ | ------ |
| 0–15 m  | 0.27  | 0.41   | 0.41   | 0.49   | 0.56   | 0.59   |
| 15–25 m | 0.23  | 0.57   | 0.48   | 0.59   | 0.57   | 0.60   |
| 25–40 m | 0.25  | 0.29   | 0.35   | 0.48   | 0.43   | 0.33   |

**`embarcadero-folsom`**

| Range   | 0–15° | 15–30° | 30–45° | 45–60° | 60–75° | 75–90° |
| ------- | ----- | ------ | ------ | ------ | ------ | ------ |
| 0–15 m  | 0.33  | 0.44   | 0.49   | 0.56   | 0.62   | 0.67   |
| 15–25 m | 0.26  | 0.53   | 0.46   | 0.49   | 0.54   | 0.56   |
| 25–40 m | –     | 0.43   | 0.41   | 0.53   | 0.47   | 0.56   |

**`marina-webster-beach`**

| Range   | 0–15° | 15–30° | 30–45° | 45–60° | 60–75° | 75–90° |
| ------- | ----- | ------ | ------ | ------ | ------ | ------ |
| 0–15 m  | 0.24  | 0.29   | 0.46   | 0.60   | 0.66   | 0.65   |
| 15–25 m | 0.23  | 0.42   | 0.50   | 0.60   | 0.58   | 0.66   |
| 25–40 m | –     | 0.57   | 0.64   | 0.44   | 0.46   | 0.66   |

The trend holds in every stratum with enough observations to report. It is strongest at Marina,
which is also the steepest placement at a measured 4.35% grade — but because it is equally present
at Embarcadero (0.14%) and Columbus (0.88%), grade is not what produces it.

## Methodological notes, including one correction

**A sign bug in the first E1.3 implementation inverted its conclusion.** The initial version
projected candidates onto the body's lateral axis without orienting that axis toward the sensor.
Vehicles passing on opposite sides of the sensor are then displaced in opposite directions along
that axis, so averaging cancels them. With the bug, Columbus and Embarcadero showed no aspect trend
at all and only Marina did, which read as evidence that the effect was a grade artefact at the one
steep site. Orienting the axis toward the sensor resolved it and brought E1.3 into agreement with
E1.1. Recorded because the wrong version was self-consistent and produced a plausible table: E1.1's
independent reference is what exposed the disagreement.

**A 60-second trial slice was misleading in both directions.** Before the full run, a single
Embarcadero minute showed an apparently clean monotonic trend unstratified, and no trend once
stratified — the reverse of the full result. It was quoted only to justify building the
stratification, never as a finding.

**The OBB centre's flatness in E1.1 is circular and is not evidence of its quality.** The reference
path is fitted to estimates the tracker produced _from_ the OBB centre, so its residuals about that
path are structurally smaller. Its row is reported for completeness. The medoid and near-edge rows
carry the weight, because neither produced the path, and Section 16.5's defence applies to them: a
conditional mean taken over a covariate the fit never sees cannot be manufactured by the fit, and
nothing in a straight-line fit knows the aspect angle.

**The near-edge candidate is handicapped here.** An observation carries no track history by design,
so its dimension prior is the observed OBB extents — which are themselves compressed when only part
of the body is visible. A near-edge measurement driven by an accumulated dimension belief should do
better than these figures show. The synthetic bound is the complementary evidence:
[measurement_model_test.go](../../../internal/lidar/l5tracks/measurement_model_test.go) puts its
lateral error at 0.0000 m against the OBB centre's 0.1391 m with a correct prior.

**The ground-clipped stratum is empty.** `GroundClipped` was false on all 195,389 observations,
because surface-relative ground clipping is off by default and these runs did not enable it. Step 2
of Section 16.5's grade mitigation therefore did no work, and the confound is addressed by range
stratification alone. That is adequate here — the effect is present at the flattest site — but the
mitigation should not be described as having been exercised.

## What this does and does not establish

Established: the medoid's lateral error is deterministic in aspect angle, reproducible across three
placements with different north azimuths, backgrounds and approach geometries, and robust to range
control. Phase 2's measurement-definition premise is validated on real traffic.

Not established. There is **no ground truth** in this corpus — no instrumented probe vehicle, no
survey, no external reference trajectory — so absolute position error is not measurable here and
every figure above is a relative disagreement or an offset from a fitted path. The magnitude of the
bias is ~0.4 of a half-width against a fitted path rather than the ±W/2 Section 3.3 anticipated
from synthetic boxes; real vehicles are not boxes, and a roof visible from the mount height pulls
the medoid inward. These are three placements on two afternoons with one sensor in dry conditions,
so this addresses viewpoint diversity and says nothing about seasons, weather or sensor units.

Open: **E1.2** (straight-segment residual spectrum with autocorrelation) and **E1.4** (stationary
noise floor, the one place real data supplies an exact expected value) are not yet run. G-GEO-1
needs both, plus held-out geometry, excursion and fragmentation criteria.

## Addendum: a fourth placement, `clar0` (2026-09-17)

- **Scope:** One capture, `clar0.pcapng`, replayed and repeat-verified through the same
  `lidar-state-estimation-baseline` / `lidar-e1-analysis` harness as the three-site corpus above.
  7,699 frames first and repeat run, byte-identical (`baseline_equal: true`). 5,815 observations
  read, 2,704 kept after the same exclusion filters.
- **Result:** [state-estimation-e1-clar0-20260917.json](state-estimation-e1-clar0-20260917.json)
  (`sha256:5ec2b626669352d3e64a9e5ea53564dea9bc68b52fd85a114ba97ba78b6a3c4e`), E1.3 only; E1.1's
  table is reproduced below since the tool prints it rather than including it in `-json`.
- **Source manifest:** `sha256:99868074940142f48cd89c7428ccfa2766fe6a14131ef1b2f9b66533b32530e6`.

This is a genuinely independent site, not a fourth S2 window: a different physical deployment,
different mount, different street geometry, run months apart from the three-site corpus.

**E1.3 reproduces cleanly.** The medoid-versus-near-edge bias fraction rises from 0.26 end-on to a
0.50-0.63 plateau from 30° of aspect onward, matching the three-site shape and landing inside its
0.35-0.65 range. It survives range stratification in the well-populated 0-15 m band (0.27 → 0.62
across aspect bins, n from 148 to 421).

**E1.1 also reproduces, on a smaller accepted population.** 16 tracks (429 observations) passed the
speed/straightness/length gates, against the three-site corpus's much larger accepted set. The
medoid's bias-as-half-width climbs from 0.25 (15-30°) to 0.42 (45-60°) before easing to 0.30 at
broadside, while `near_edge` stays within ±0.16 of zero throughout — the same qualitative signature
as the three-site result, on a site that has never contributed a single frame to it before.

**One placement remains too small to read: `kirk0`.** The same harness ran cleanly (131 frames
first/repeat, byte-identical), but only 8 tracks (228 observations) passed E1.1's gates — kirk0 is
an 83-second clip, an order of magnitude shorter than any of the other four placements. Its medoid
bias-as-half-width sits flat around 0.17-0.21 rather than climbing, and `near_edge` drifts to -0.25
at broadside instead of staying near zero. Given the sample size, this reads as underpowered rather
than contradictory: 8 tracks cannot establish or refute a trend the other four placements needed
16-234 tracks to show cleanly. Recorded for completeness, not counted as a fifth confirming
placement.

**Calibration caveat.** Neither site has a measured `north_azimuth_deg` — the "Align from above"
compass bearing the three-site index carries for each placement — so both were registered with a
placeholder of `0`. This only rotates the site frame by a constant, uniform offset; E1.1's
aspect-relative bins and E1.3's pairwise disagreement are both invariant to a global rotation (see
`siteCalibration` in `cmd/tools/lidar-state-estimation-baseline/main.go`), so neither result is
distorted by it. What the placeholder does forbid is any future claim about `clar0` or `kirk0`'s
absolute compass-referenced heading, or a plot overlaying its site frame on the other four
placements' true-north-aligned ones.

Updated tally: **E1.1 and E1.3 now reproduce at four independent placements** (three S2 windows
plus `clar0`), with a fifth (`kirk0`) recorded but underpowered. E1.2 and E1.4 remain open, as
above — no code implements either yet.

## Addendum: E1.2 and E1.4 run on Columbus (2026-09-19)

- **Scope:** `lidar-e1-analysis` now implements both remaining parts of E1 (`residuals.go`), run
  on the Columbus evidence database `evidence/columbus-medoid-20260916/observations/observations.db`
  (102,013 observations, 77,891 estimates over 2,685 tracks) with the shipped
  `measurement_noise` of 0.05 m² for comparison.
- **Result:** [state-estimation-e1-residuals-columbus-20260919.json](state-estimation-e1-residuals-columbus-20260919.json)
  (`sha256:af7f9b1555b0dd531a2e9084b7c3c2c886cf2318f7d15bc22b6e8cf254a013d0`).

**E1.4, the stationary noise floor.** A body that is not moving has a constant true position, so the
scatter of a position candidate on it is measurement noise and nothing else. Tracks with mean speed
under 0.3 m/s over at least 50 frames: 68 accepted, but only three carry the observation-level
candidates (the others fail the 40-point cluster or 30-retained-point rules that E1.3 applies), so the
candidate rows are two tracks at 0-20 m and one at 20-40 m and are read as such.

| Series     | Range   | Tracks |   Obs | σ major | σ minor | σ isotropic | ÷ √R (0.224 m) |
| ---------- | ------- | -----: | ----: | ------: | ------: | ----------: | -------------: |
| medoid     | 0-20 m  |      2 |   963 | 1.129 m | 0.086 m |     0.801 m |           3.58 |
| obb_centre | 0-20 m  |      2 |   963 | 1.161 m | 0.072 m |     0.822 m |           3.68 |
| near_edge  | 0-20 m  |      2 |   963 | 1.204 m | 0.127 m |     0.856 m |           3.83 |
| medoid     | 20-40 m |      1 |    66 | 0.241 m | 0.177 m |     0.211 m |           0.94 |
| estimate   | 0-20 m  |     22 | 4,946 | 0.566 m | 0.107 m |     0.402 m |           1.80 |
| estimate   | 20-40 m |     37 | 7,610 | 0.448 m | 0.062 m |     0.323 m |           1.44 |
| estimate   | 40 m +  |      9 | 1,573 | 0.040 m | 0.019 m |     0.032 m |           0.14 |

The floor is anisotropic by an order of magnitude: on the near stationary bodies every observation
candidate scatters by 1.1-1.2 m along its principal axis and by 0.07-0.13 m across it, against one
isotropic 0.22 m in the filter. The near-edge candidate does not narrow it, which is consistent with
the measurement being the body's visible extent changing under passing occluders rather than a
centre-of-visible-surface bias. The filter posterior halves the along-axis scatter and leaves the
across-axis one; at 40 m and beyond the posterior barely moves (0.03 m), which is under-confidence,
not accuracy. Two tracks are two tracks: this stratum needs the other 65 stationary tracks to be
usable, which means relaxing the point-count rules for the stationary case specifically.

**E1.2, the straight-segment residual spectrum.** Tracks accepted as E1.1 accepts them (speed ≥ 3 m/s,
straight-line residual RMS ≤ 0.5 m, ≥ 20 estimates, and here ≥ 30 frames): 165. The lateral offset
of each series from the fitted path, in frame order, tested for whiteness with the sample
autocorrelation at lags 1-10 and Ljung-Box Q(10) against χ²₁₀ at 95% (18.307):

| Series         | Tracks | White at 95% | Share | Median ρ₁ | Median Q(10) | Median σ |
| -------------- | -----: | -----------: | ----: | --------: | -----------: | -------: |
| medoid         |     42 |            6 |  0.14 |     0.707 |         51.3 |  0.276 m |
| obb_centre     |     42 |            7 |  0.17 |     0.704 |         57.2 |  0.247 m |
| nearest_corner |     42 |           11 |  0.26 |     0.564 |         28.0 |  0.418 m |
| near_edge      |     41 |            9 |  0.22 |     0.607 |         53.6 |  0.268 m |
| estimate       |    165 |           13 |  0.08 |     0.777 |         65.1 |  0.243 m |

No candidate's lateral residual is white. A median lag-1 autocorrelation of 0.56-0.71 on a
10 Hz series is a deterministic component that persists across many frames, which is what an
aspect-dependent bias sliding along a passing vehicle produces and what measurement noise does not.
The filter posterior is the least white of all, as smoothing a red input must make it. The
nearest-corner candidate is the whitest and also the noisiest, the trade the plan's Section 3
anticipated. The candidate series cover 41-42 of the 165 tracks because the observation-level rules
above exclude the rest; the posterior covers all 165.

**What this closes.** E1.2 and E1.4 are run, not open. Both point the same way as E1.1/E1.3 and as
the gap analysis's K9: the error is structured and anisotropic, and a scalar R cannot represent it.
G-GEO-1 still needs held-out geometry, excursion and fragmentation criteria; nothing here supplies
those.
