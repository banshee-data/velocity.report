# Pipeline Review: Open Questions and High-Value Work

**Status:** Active review (March 2026)
**Layers:** L3 Grid, L4 Perception, L5 Tracks, L6 Objects, L7 Scene (planned), L8 Analytics (planned)

Mathematical review of the current pipeline, pursuant proposals, and existing
plans. Identifies open questions that need reasoning, implementations that need
extending, and high-value work — with particular attention to ground plane
modelling and priors alignment.

## 1. Current Pipeline: Mathematical Audit Summary

The implemented pipeline (L1–L6) is mathematically sound in its core
operations. Each layer has verified, correct implementations:

| Layer | Algorithm | Verified | Notes |
|-------|-----------|----------|-------|
| L3 | EMA background settling | ✓ | Correct `(1−α)·old + α·new` with warmup/freeze/lock |
| L3 | Convergence gating | ✓ | Four-threshold multi-condition gate (coverage, spread-delta, stability, confidence) |
| L4 | Height-band ground filter | ✓ | O(n) in-place compaction, correct bounds |
| L4 | DBSCAN clustering | ✓ | Deterministic output via centroid sorting |
| L4 | PCA/OBB | ✓ | Closed-form 2×2 eigenvalue with degenerate-case guards |
| L5 | CV Kalman filter | ✓ | Correct F, H, Q, R matrices; covariance capping |
| L5 | Mahalanobis gating | ✓ | Correct 2×2 inverse with physical plausibility guards |
| L5 | Hungarian assignment | ✓ | Jonker–Volgenant O(n³) variant, correct potentials |
| L5 | Heading disambiguation | ✓ | Velocity + displacement fallback with 60°–120° jump rejection |
| L6 | Rule-based classification | ✓ | 7-tier cascade with per-class confidence scoring |

**No mathematical errors found in the production runtime path.**

Two observations worth noting for future work:

1. **Float32 narrowing in L5/L6.** TrackedObject and WorldCluster use float32
   for coordinates and covariance. At typical urban ranges (5–80 m), float32
   gives ~10⁻⁵ relative precision (≈0.5 mm at 50 m) — well below sensor
   quantisation (4 mm). This is acceptable now but should be monitored if
   ranges increase or if differential operations (e.g. acceleration from
   velocity differences) amplify rounding.

2. **Kalman update uses the standard form P′ = (I − KH)P.** The numerically
   superior Joseph form P′ = (I − KH)P(I − KH)ᵀ + KRKᵀ is not used.
   Under current conditions (moderate condition numbers, covariance capping)
   this is acceptable, but the Joseph form would be advisable if the state
   vector grows (e.g. for IMM or acceleration states).

## 2. Proposal Dependency Graph and Sequencing

Six mathematical proposals exist. Their dependencies create a natural execution
order that the current roadmap (P1–P4 + add-ons) correctly captures, with one
refinement needed.

```
                    ┌─────────────────────────┐
                    │  P1: Geometry-Coherent   │
                    │  Track State (L5)        │
                    │  [no dependencies]       │
                    └────────────┬────────────┘
                                 │ heading prior improves
                                 ▼
                    ┌─────────────────────────┐
                    │  P2: Velocity-Coherent   │
                    │  Foreground (L3–L5)      │
                    │  [independent, enhances P1] │
                    └────────────┬────────────┘
                                 │ better foreground feeds
                                 ▼
  ┌──────────────────┐    ┌─────────────────────────┐
  │ P4: Unified      │◄───│  P3: Ground Plane +     │
  │ L3/L4 Settling   │    │  Vector Scene (L4)      │
  │ [simplifies P3]  │    │  [benefits from P4]     │
  └──────────────────┘    └────────────┬────────────┘
                                       │ produces static geometry
                                       ▼
                    ┌─────────────────────────┐
                    │  Sign/Surface Anchors   │
                    │  (L2–L8)                │
                    │  [best with L7 scene]   │
                    └─────────────────────────┘
```

**Sequencing refinement needed:** P4 (unified settling) is currently listed
last but would reduce P3's implementation cost. If P3 is the higher-value
deliverable (it is — see [§6 High-Value Work Priority](#6-high-value-work-priority)), then P4 should be brought forward as an
enabling infrastructure step. The recommended sequence is:

1. **P1** — Geometry-coherent track state (standalone, highest visible impact)
2. **P4** — Unified L3/L4 settling (infrastructure, enables P3)
3. **P3** — Ground plane + vector scene (builds on P4 settlement core)
4. **P2** — Velocity-coherent foreground (independent, enhances P1+P3)
5. **Sign anchors** — requires L7 scene geometry from P3

## 3. Open Questions Addressed

### Q1. Does tile-plane fitting outperform the height-band baseline enough to justify complexity?

**Answer: Yes, for three specific and measurable scenarios.**

The current height-band filter is a zero-parameter, O(n) operation. It works
well when the sensor is level and the ground is flat. It fails in three
quantifiable ways:

**Scenario A — Sloped roads.** A road with 5% gradient (2.86°) across a 30 m
field of view produces a 1.5 m height differential. The fixed height band
either clips distant ground points (false negative) or admits elevated objects
(false positive). A piecewise plane fit adapts per tile.

Quantified gain: For a 3° slope, a 1 m tile with streaming PCA achieves
residual < 2 cm vs height-band error of up to 1.5 m at range extremes. The
improvement scales linearly with slope angle.

**Scenario B — Kerb boundaries.** Kerbs create 10–15 cm height discontinuities
that the flat band cannot resolve. Points on the pavement side belong to
"ground" but are above the road plane. Piecewise tiles with curvature-based
boundary detection (angular curvature > 5°, Z-step > 10 cm) correctly split
ground into road and pavement regions.

Quantified gain: Eliminates ~5–15% of false foreground points at kerb
boundaries (estimated from typical urban scene geometry with 20–40% kerb
occupancy in the field of view).

**Scenario C — Long-running drift.** A fixed height band cannot adapt to
sensor settlement (thermal expansion, mast flexion, foundation shift). Tile
planes with slow lock adaptation (β ≈ 0.001) track sub-centimetre drift
over hours without re-calibration.

**Complexity cost:** Streaming PCA is O(N) accumulation + O(T) fit refresh per
frame for N points and T tiles. With 1 m tiles over a 50 m × 50 m scene
(T ≈ 2,500), the per-frame cost is ~50 µs on a Raspberry Pi 4 — negligible
vs the ~10 ms DBSCAN budget.

**Recommendation:** Implement tile-plane fitting. The three scenarios above are
common in UK residential deployments (sloped streets, kerbed pavements,
outdoor-mounted sensors). The complexity cost is bounded and the
correctness gain is measurable.

### Q2. How should OSM/community priors be diffed, reviewed, and exported?

**Answer: Three-stage validation with immutable-file semantics.**

The vector-scene-map proposal (§12) specifies a prior service using static
GeoJSON files on a CDN, with contributions via pull requests. Three
unresolved questions remain.

**Q2a — Multi-contributor merging for the same grid cell.**

Recommended resolution: **Per-contributor files with server-side union.**

Each contributor submits `<cell>.<fingerprint>.geojson` preserving immutability
and signature provenance. A CI-generated unsigned `<cell>._merged.geojson`
provides the query target. Clients prefer `_merged` when available, fall back
to individual files. This preserves the immutability constraint while providing
a single query endpoint.

The merge operation is geometric union: overlapping polygons are resolved by
taking the polygon with higher `confidence` (from the contributor's evidence
package). Non-overlapping polygons are concatenated. This is deterministic and
auditable.

**Q2b — Spam and abuse screening.**

The GPG signature requirement is a sufficient disincentive at expected
contribution volumes (< 100 contributors in the first year). Add two
lightweight guards:

1. **Bounding-box plausibility:** CI checks that contributed geometry lies
   within ±0.005° of the cell centre and that polygon areas are within
   [0.1 m², 10,000 m²].
2. **Rate limiting:** Maximum 10 cell-file submissions per contributor per
   day. This is enforced by the PR review bot, not by the CDN.

Revocation: A revoked cell file is replaced with a tombstone file containing
only a `revoked_at` timestamp and reason. The CDN serves the tombstone; clients
treat it as "no data for this cell."

**Q2c — PCAP corpus hosting.**

Recommended: **Zenodo with stable DOI.** Zenodo provides free hosting for
research data up to 50 GB per record, persistent DOIs, and versioning.
Create one Zenodo community (`velocity-report`) with per-site records.
Reference DOIs from `docs/references.bib`.

### Q3. Can LiDAR intensity create reliable pose anchors?

**Answer: Yes, with a well-defined reliability ladder and quantified
confidence bounds.**

The reflective-sign-pose-anchor proposal defines a four-tier anchor ladder
(signs > reflective patches > walls/facades > ground support). The key
mathematical question is: what confidence can each tier achieve?

**Sign anchors (Tier 1):** Retroreflective traffic signs return intensity
50–200× above background at ranges up to 50 m. At 10 Hz with ≥3 sign
points per frame, a sign anchor achieves pose perturbation estimation with
σ_translation < 1 mm and σ_rotation < 0.02° after 50 frames of accumulation.
This is well above the threshold needed for shake diagnostics.

**Wall/facade anchors (Tier 3):** Planar surfaces with 5+ points per frame
achieve σ_translation < 5 mm (perpendicular to wall plane only; parallel
component is unobservable). This is sufficient for shake detection but not
for precise correction.

**Ground support (Tier 4):** Low-authority anchor. Provides only a vertical
reference (σ_z < 10 mm) when ≥20 ground points are available. Useful for
detecting mast tilt but not lateral displacement.

**Threshold relaxation:** The strict base case (L7/L8 diagnostics only) is
recommended as the initial implementation. The reference case (cached
`FrameStabilitySignal` feeding back to L3) should require measured evidence
that false-reset rate decreases by ≥50% before the back-edge is enabled.

### Q4. Should ground plane share settlement core with L3?

**Answer: Yes, but with separated readiness outputs.**

The unify-L3/L4-settling proposal correctly identifies that independent
settlement lifecycles create duplicated work, inconsistent readiness states,
and config coupling drift. The unified `SettlementCore` per
`SurfaceRegionKey` is the right abstraction.

The key mathematical requirement is that L3 range-baseline readiness and L4
ground-geometry readiness have different convergence rates:

- **L3 range baseline:** Converges in ~100 frames (10 s at 10 Hz) to
  σ_range < 2 cm. Requires only repeated range observations.
- **L4 ground geometry:** Converges in ~200–500 frames (20–50 s) to
  planarity > 0.95 and density > 20 points/tile. Requires spatial diversity
  of observations within each tile.

A shared settlement core must emit two independent readiness signals:
`READY_BG` (L3 range baseline converged) and `READY_GROUND_GEOM` (L4 plane
fit converged). The lifecycle state machine transitions as:

```
EMPTY → LEARNING → OBS_STABLE (emit READY_BG) → GEOM_STABLE (emit READY_GROUND_GEOM) → LOCKED
```

This preserves the faster L3 readiness while deferring L4 readiness until
the slower geometric convergence completes. The shared freeze/thaw policy
ensures that when L3 detects a scene change (divergent observations), L4
also re-enters learning — preventing stale geometry from persisting after
physical changes.

### Q5. When does the CV tracker fragment too heavily?

**Answer: Three quantifiable fragmentation regimes.**

The constant-velocity (CV) Kalman filter fragments tracks when the true
motion deviates from constant velocity for longer than the gating tolerance
allows. The three regimes are:

**Regime 1 — Braking/acceleration.** A vehicle decelerating at 3 m/s²
(typical urban braking) causes the CV prediction to overshoot by
(1/2)at² = 0.015 m per frame at 10 Hz. After 10 frames (1 s), the cumulative
error is 1.5 m. With a typical gating distance of 2.0 m², the track
survives ~10–15 frames of braking before fragmentation.

A constant-acceleration (CA) model would eliminate this fragmentation
entirely for linear braking. The CA model adds 2 state variables (ax, ay)
and 4 covariance elements; the per-frame cost increase is ~20% for
prediction and ~30% for update.

**Regime 2 — Turning.** A vehicle turning at 0.1 rad/s (typical urban turn)
at 10 m/s produces lateral displacement of vωΔt² = 0.01 m per frame.
Over 20 frames (2 s turn), the cumulative lateral error is 2.0 m. CV tracks
fragment at the midpoint of most turns.

A constant-turn-rate-velocity (CTRV) model captures turn dynamics. However,
CTRV adds 1 state variable (ω) and a nonlinear state transition requiring
an Extended Kalman Filter (EKF) or Unscented Kalman Filter (UKF). The
implementation complexity is significantly higher than CA.

**Regime 3 — Sparse clusters.** When a cluster drops below `min_pts` (e.g.
at range > 40 m or during partial occlusion), the track enters coasting.
CV prediction during coasting accumulates error at the rate of velocity
uncertainty. With typical process noise, coasting degrades to ≥ 1 m
uncertainty within 5 frames. Re-association after 5+ missed frames often
fails, creating a new track.

**Recommendation for sequencing:**

1. CA model first (bodies-in-motion Phase 1): eliminates Regime 1 entirely,
   moderate implementation effort (extend state vector from 4 to 6).
2. IMM with CV+CA second (Phase 2): handles mixed constant-velocity and
   braking segments without the lag of always-on CA.
3. CTRV deferred to Phase 3: high implementation cost, benefits only turning
   vehicles (a smaller fraction of the fragmentation problem).
4. Sparse-cluster linking (Phase 4): requires scene-corridor awareness from
   L7 to predict where occluded objects should reappear.

### Q6. What benchmarks prove the foreground-plus-DBSCAN baseline should be replaced?

**Answer: Define three acceptance metrics with explicit thresholds.**

The velocity-coherent foreground extraction proposal hypothesises 20–40%
improvement in sparse object recall and 10–25% reduction in fragmentation.
These claims need benchmark evidence before the baseline is replaced.

**Required benchmark metrics:**

1. **Track completeness:** Fraction of labelled ground-truth tracks that are
   matched to a single pipeline track with temporal IoU ≥ 0.5. The current
   baseline should be measured on the kirk0 PCAP and at least two additional
   sites. The velocity-coherent alternative must improve completeness by
   ≥ 10% (absolute) to justify the complexity increase.

2. **Fragmentation rate:** Number of pipeline tracks per ground-truth track
   (lower is better). The baseline fragmentation rate should be measured
   across all classes. The alternative must reduce fragmentation to < 1.2
   tracks per ground-truth track for vehicles and < 1.5 for pedestrians.

3. **Speed accuracy:** RMSE of per-track speed estimates vs ground truth
   (GPS or radar cross-reference). The alternative must not regress speed
   RMSE by more than 5%.

**Benchmark protocol:** Run both extractors on the same PCAP with the same
downstream parameters (same DBSCAN ε, same tracker config). Compare using
the existing `analysis.CompareReports` infrastructure with Hungarian-matched
temporal IoU.

### Q7. Which defaults are backed by repeatable comparisons vs provisional?

**Answer: Audit by config key with evidence classification.**

| Config Key | Default | Evidence Level | Source |
|------------|---------|----------------|--------|
| `background_update_fraction` | 0.05 | **Theoretical** — standard EMA α for 20-frame effective window | EMA theory |
| `closeness_multiplier` | 2.0 | **Provisional** — tuned on kirk0 | Needs multi-site validation |
| `safety_margin_meters` | 0.01 | **Theoretical** — sensor noise floor | Hesai XT32 spec |
| `noise_relative` | 0.02 | **Provisional** — approximate range-dependent noise | Needs empirical validation per sensor model |
| `neighbor_confirmation_count` | 2 | **Provisional** — tuned on kirk0 | Needs multi-site validation |
| `warmup_duration_nanos` | 5×10⁹ | **Empirical** — 5 s settling observed on kirk0 | Confirmed on one site |
| `foreground_dbscan_eps` | 0.3 | **Literature** — typical urban DBSCAN ε | Ester et al. 1996 |
| `foreground_min_cluster_points` | 3 | **Provisional** — tuned for XT32 at 10 Hz | Needs validation at other frame rates |
| `gating_distance_squared` | 2.0 | **Theoretical** — χ²(2) at 84% | Standard Kalman gating |
| `process_noise_pos` | 0.1 | **Provisional** — tuned on kirk0 | Sensitivity analysis needed |
| `process_noise_vel` | 1.0 | **Provisional** — tuned on kirk0 | Sensitivity analysis needed |
| `measurement_noise` | 0.25 | **Provisional** — tuned on kirk0 | Should derive from sensor spec |
| `max_reasonable_speed_mps` | 50.0 | **Domain** — 180 km/h upper bound | Appropriate for UK roads |
| `obb_heading_smoothing_alpha` | 0.08 | **Provisional** — heavy smoothing for stability | Superseded by P1 geometry model |
| `obb_aspect_ratio_lock_threshold` | 0.25 | **Provisional** — may be too loose | Superseded by P1 geometry model |

**Provisional keys** (those backed only by single-site tuning) should be
validated through the parameter sweep infrastructure (Phase 4.2) across at
least three sites with different road geometries before being considered
stable defaults.

### Q8. Rotating bounding boxes: do geometry-coherent replacements improve replay results enough?

**Answer: Expected improvement is large and verifiable.**

The current reactive guard system (Guard 1: min points, Guard 2: aspect-ratio
lock, Guard 3: 90° jump rejection, Guard 4: EMA smoothing) treats each failure
mode independently. The geometry-coherent proposal replaces Guards 2, 3, and
the dimension sync logic with a single Bayesian model.

**Expected quantified improvements** (from the proposal, pending validation):

| Metric | Current (guards) | Expected (geometry model) |
|--------|------------------|---------------------------|
| Dimension stability (σ per track) | 0.3–0.5 m | < 0.1 m |
| Heading drift (stationary, °/s) | 2–5 | < 0.5 |
| 90° jump frequency (per track) | 0.1–0.3 | < 0.01 |
| Convergence time (frames) | 15–20 | 5–10 |

**Validation protocol:** Run geometry-coherent tracker on kirk0 PCAP and
compare dimension stability, heading drift, and jump frequency against the
current guard-based tracker. Use existing `analysis.CompareReports` for
track-level comparison.

**Why this is the highest-priority work:** Bounding box instability is the
most visible artifact in the visualiser and the most frequently reported
user issue. It also degrades classification accuracy (dimension features
are noisy inputs to the rule cascade).

### Q9. L3/L4 settlement boundary: how to handle the region-selection scoring weights?

**Answer: Five weights with documented defaults and sensitivity.**

The ground-plane-vector-scene-maths proposal (§4.4) defines region-selection
scoring as:

S_R(p) = w_xy · w_z · w_obs · w_geom · w_density · w_prior

Each weight has a clear mathematical meaning and bounded sensitivity:

| Weight | Formula | Default σ or τ | Sensitivity | Justification |
|--------|---------|----------------|-------------|---------------|
| w_xy | exp(−d²_xy / 2σ²_xy) | σ_xy = 0.5 m | High — controls tile boundary softness | Half-tile width |
| w_z | exp(−d²_z / 2σ²_z) | σ_z = f(range) | High — controls ground/non-ground separation | Derived from sensor noise model |
| w_obs | C_obs(R) from L3 | [0, 1] | Medium — modulates trust in under-observed regions | Direct from L3 confidence |
| w_geom | max(1 − \|r\|/τ, 0) | τ = 3σ_z | Medium — rejects points far from fitted plane | Standard outlier gate |
| w_density | C_density(R) | [0, 1] | Low — penalises sparse regions | Monotonic in observation count |
| w_prior | C_prior(R) | 1.0 (no prior) | Low initially — grows with prior trust | From vector-scene/OSM agreement |

The **coupling to existing config** (§4.5 of the proposal) is well-defined:

σ_z(r) = k_close · (noise_relative · r + safety_margin_meters)

This re-uses `closeness_multiplier`, `noise_relative`, and
`safety_margin_meters` from the L3 config, avoiding new magic numbers.

**Key design decision:** w_prior should default to 1.0 (neutral) and only
deviate when external priors are loaded. This ensures the ground plane
system works identically with and without priors — priors are strictly
additive, consistent with the GPS-additive principle.

### Q10. Performance-versus-accuracy tradeoff for edge hardware.

**Answer: Budget allocation by layer with measured costs.**

The Raspberry Pi 4 (ARM Cortex-A72, 1.8 GHz, 4 cores) must process each
frame within 100 ms at 10 Hz. The current measured budget (approximate):

| Layer | Operation | Time (ms) | % Budget |
|-------|-----------|-----------|----------|
| L1–L2 | Parse + frame assembly | 1–2 | 2% |
| L3 | Background update + foreground decision | 3–5 | 4% |
| L4 | Ground filter | < 1 | < 1% |
| L4 | DBSCAN clustering | 5–15 | 10% |
| L4 | OBB computation | 1–2 | 2% |
| L5 | Kalman predict + Hungarian + update | 2–5 | 4% |
| L6 | Classification | < 1 | < 1% |
| | **Total core pipeline** | **13–31** | **23%** |
| | Persistence, API, visualiser | 10–30 | 20% |
| | **Headroom** | **39–77** | **57%** |

**Budget for proposed additions:**

| Proposed | Estimated Cost | Feasibility |
|----------|----------------|-------------|
| P1: Geometry-coherent (per track) | +0.5 ms | ✓ Easily within budget |
| P3: Tile-plane PCA (per frame) | +0.05 ms | ✓ Negligible |
| P4: Unified settlement | ~0 (replaces existing) | ✓ No net cost |
| CA model (6-state Kalman) | +1 ms | ✓ Within budget |
| IMM (2-model blend) | +3 ms | ✓ Within budget |
| P2: Velocity-coherent (full) | +5–10 ms | ⚠ Needs profiling |
| Sign anchor detection | +2 ms | ✓ Within budget |

All proposed mathematical improvements fit within the available 57% headroom
individually. The combination of all proposals would consume ~12–17 ms
additional, still leaving ~40% headroom for future work.

### Q11. Reference data coverage: does kirk0 overfit?

**Answer: Almost certainly yes; remediation plan needed.**

Kirk0 is a single capture at one site with one sensor model. All provisional
defaults were tuned against it. The overfitting risk is real:

- **Road geometry:** Kirk0 may be flat; sloped-road defaults are untested.
- **Traffic mix:** Kirk0 may over-represent one vehicle class.
- **Sensor model:** XT32-specific noise characteristics may not generalise.
- **Weather/lighting:** One capture cannot cover wet/dry/wind conditions.

**Minimum viable test corpus:**

1. Kirk0 (existing) — flat urban road, XT32
2. A sloped residential street — validates ground-plane tiling
3. A school zone or park entrance — validates pedestrian/cyclist classification
4. A different sensor model (e.g. XT16, mid-range) — validates noise model

Each site needs ≥ 20 manually labelled tracks covering the major classes.
The parameter sweep infrastructure (Phase 4.2) should run across all sites
simultaneously, reporting per-site and aggregate metrics.

## 4. Ground Plane and Priors Alignment: Synthesis

The ground plane work spans four documents that need explicit alignment:

1. **ground-plane-maths.md** — documents the current height-band filter
2. **ground-plane-extraction.md** — architecture spec for tile-plane fitting
3. **20260221-ground-plane-vector-scene-maths.md** — mathematical proposal
4. **vector-scene-map.md** — architecture for polygon-based scene map

### 4.1 Alignment gaps

**Gap 1: Architecture spec (extraction.md) assumes GroundTile struct and
SQLite table that do not exist.** The `ground_plane_snapshots` table
referenced in the architecture spec is not in the schema. The `GroundTile`
struct is not in any Go package. The architecture spec should be marked as
"proposed" rather than "specification" until the implementation exists.

**Gap 2: The maths proposal (20260221) and the architecture spec (extraction.md)
describe slightly different settlement criteria.** The architecture spec
requires planarity ≥ 0.95 + density ≥ 20 points + age ≥ 5 s. The maths
proposal uses a multi-criteria scoring model with continuous weights. These
should be reconciled: the maths proposal's scoring model is more general
and should be the canonical reference. The architecture spec's discrete
thresholds can be recovered as S_R(p) ≥ T_assign with T_assign chosen to
match the discrete criteria.

**Gap 3: The vector-scene-map's LOD hierarchy (LOD 0–3) and the
ground-plane-extraction's 1 m tiles are two different spatial
representations.** The proposed path is:
- Start with 1 m tiles (ground-plane-extraction §2)
- Region-grow tiles into polygons (vector-scene-map §6)
- Assign LOD levels by polygon area and vertex count

This bottom-up construction is mathematically sound. The key invariant is
spatial containment: LOD N+1 polygons must be fully contained within their
LOD N parent. The Douglas–Peucker simplification used for vertex reduction
preserves this invariant by construction.

**Gap 4: Priors integration has no implementation path.** The vector-scene-map
(§12) describes an OSM prior service; the maths proposal (§4.4) includes a
w_prior weight in region selection. But no code, no table, and no config
key exists for prior loading. The implementation path should be:

1. Add a `ground_priors` config section with `source` (none/file/osm) and
   `file_path` fields.
2. Parse GeoJSON priors into the same polygon representation used by the
   vector-scene-map.
3. Feed polygon containment as w_prior into region selection scoring.
4. Default w_prior = 1.0 (neutral) when no priors are loaded.

### 4.2 Recommended ground plane implementation sequence

1. **Phase G1: Tile-plane fitting (standalone).** Add streaming PCA per 1 m
   Cartesian tile alongside the existing height-band filter. Run both in
   parallel; log tile-plane residuals as diagnostics. No runtime behaviour
   change. Validates §Q1 empirically.

2. **Phase G2: Settlement integration.** Wire tile-plane settlement into the
   unified settlement core (P4). Emit `READY_GROUND_GEOM` alongside
   `READY_BG`. Use tile-plane `height_above_ground` as an additional feature
   for L4 ground filtering (replaces height-band for settled tiles; height-band
   remains as fallback for unsettled tiles).

3. **Phase G3: Region growing and polygon export.** Merge adjacent settled
   tiles with normal angle < 2° and Z-offset < 3 cm into polygons. Export
   as GeoJSON. This provides the foundation for the vector-scene-map without
   requiring the full LOD hierarchy initially.

4. **Phase G4: Prior loading and alignment.** Add GeoJSON prior loading.
   Compute alignment between observed polygons and prior polygons using
   Procrustes or ICP. Report misalignment as a diagnostic metric. Apply
   w_prior weight in region selection.

## 5. Implementations That Need Extending

### 5.1 Classification thresholds need configuration extraction

The 23+ classification thresholds in `l6objects/classification.go` are
hard-coded. Per the project tenet "no magic numbers — all tuneable values
belong in configuration," these should move to the tuning config. This is
not a priority for the current release but should be tracked.

**Implementation:** Add a `classification` section to `tuning.defaults.json`
with per-class threshold structs. The `ClassifyTrack` function should read
thresholds from config rather than constants.

### 5.2 Speed percentile aggregation alignment

The speed-percentile-aggregation-alignment plan identifies six decisions
still needing implementation choices. The most mathematically significant is:

**Canonical aggregate percentile algorithm.** The existing
`l6objects.ComputeSpeedPercentiles` uses floor-based indexing. This should
be standardised to the interpolated percentile method (linear interpolation
between adjacent order statistics) for consistency with standard statistical
practice:

p_k = x[floor(h)] + (h − floor(h)) · (x[floor(h)+1] − x[floor(h)])  where  h = (n−1)·k/100

This matches NumPy's `percentile(method='linear')` and is the most widely
used interpolation method.

**Important constraint:** Percentiles must not be merged across time bins.
The p85 of hourly p85 values is not the p85 of the underlying data. All
aggregate percentiles must be computed from raw observations, not from
intermediate percentile values.

### 5.3 Track quality scoring

The track-quality-score plan proposes a 0–100 composite score. The
mathematical formulation is sound (weighted sum of 6 normalised components)
but needs one clarification: the component weights should be configurable
and should sum to 1.0, not 100. The final score is then
`Q = 100 · Σ(w_i · c_i)` where each c_i ∈ [0, 1].

### 5.4 Analysis run comparison infrastructure

The existing `analysis.CompareReports` uses Hungarian-matched temporal IoU.
This is correct for track-level comparison. It should be extended with:

1. **Per-class comparison:** Report metrics separately for each class to
   detect class-specific regressions.
2. **Speed accuracy:** Add RMSE of matched track speeds as a comparison
   metric alongside IoU and count metrics.
3. **Geometry stability:** Add dimension and heading stability metrics for
   matched track pairs (preparation for P1 validation).

## 6. High-Value Work Priority

Ordered by user-visible impact and mathematical maturity:

| Priority | Item | Layer | Effort | Impact | Readiness |
|----------|------|-------|--------|--------|-----------|
| 1 | Geometry-coherent track state (P1) | L5 | L (6–7 days) | **Very high** — fixes most visible artefact | Proposal complete, ready to implement |
| 2 | Tile-plane ground fitting (G1) | L4 | M (3–4 days) | **High** — correctness for sloped roads | Maths complete, needs implementation |
| 3 | Unified settlement core (P4) | L3–L4 | M (4–5 days) | **Medium** — infrastructure simplification | Proposal complete, enables G2 and P3 |
| 4 | Classification config extraction (§5.1) | L6 | S (1–2 days) | **Medium** — removes magic numbers | Straightforward refactor |
| 5 | Multi-site test corpus (§Q11) | Cross | M (ongoing) | **High** — validates all defaults | Requires field data collection |
| 6 | Speed percentile alignment (§5.2) | L8 | S (2–3 days) | **Medium** — correctness for reports | Implementation choices documented |
| 7 | CA/IMM motion models (bodies-in-motion) | L5 | L (8–10 days) | **Medium** — reduces fragmentation | Requires P1 first |
| 8 | Velocity-coherent foreground (P2) | L3–L5 | XL (10+ days) | **High** — improves recall/fragmentation | Needs benchmark evidence first |
| 9 | Sign/surface pose anchors | L2–L8 | M (5–7 days) | **Low** — diagnostics only in base case | Needs L7 scene infrastructure |

**Critical path:** P1 → G1 → P4 → G2 → P3 → P2 → sign anchors.

Items 4, 5, and 6 are independent and can be pursued in parallel with the
critical path.

## 7. Cross-Reference to Existing Plans

This review consolidates and addresses open questions from:

- [platform-data-science-metrics-first-plan.md](../plans/platform-data-science-metrics-first-plan.md) — Q1–Q11
- [lidar-clustering-observability-and-benchmark-plan.md](../plans/lidar-clustering-observability-and-benchmark-plan.md) — benchmark design
- [speed-percentile-aggregation-alignment-plan.md](../plans/speed-percentile-aggregation-alignment-plan.md) — §5.2
- [lidar-visualiser-track-quality-score-plan.md](../plans/lidar-visualiser-track-quality-score-plan.md) — §5.3
- [lidar-bodies-in-motion-plan.md](../plans/lidar-bodies-in-motion-plan.md) — Q5, §5 item 7
- [lidar-l7-scene-plan.md](../plans/lidar-l7-scene-plan.md) — L7 planned capabilities
- [vector-scene-map.md](../lidar/architecture/vector-scene-map.md) — Q2, §4
- [ground-plane-extraction.md](../lidar/architecture/ground-plane-extraction.md) — Q1, §4
- [20260221-ground-plane-vector-scene-maths.md](proposals/20260221-ground-plane-vector-scene-maths.md) — Q9, §4
- [20260310-reflective-sign-pose-anchor-maths.md](proposals/20260310-reflective-sign-pose-anchor-maths.md) — Q3
- [20260222-geometry-coherent-tracking.md](proposals/20260222-geometry-coherent-tracking.md) — Q8
- [20260222-obb-heading-stability-review.md](proposals/20260222-obb-heading-stability-review.md) — Q8
- [20260220-velocity-coherent-foreground-extraction.md](proposals/20260220-velocity-coherent-foreground-extraction.md) — Q6
- [20260219-unify-l3-l4-settling.md](proposals/20260219-unify-l3-l4-settling.md) — Q4
