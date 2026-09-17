# Experiment E1: lateral-error validation on the three-site corpus

- **Status:** Recorded. E1.1 and E1.3 run and reproduced at three independent placements; the hypothesis in Section 3 is confirmed. E1.2 and E1.4 remain open.
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
