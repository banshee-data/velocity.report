# Pipeline Review: Open Questions and High-Value Work

- **Status:** Active review (March 2026)
- **Layers:** L3 Grid, L4 Perception, L5 Tracks, L6 Objects, L7 Scene (planned), L8 Analytics (planned)

Mathematical review of the current pipeline, pursuant to proposals, and existing
plans. Identifies open questions that need reasoning, implementations that need
extending, and high-value work — with particular attention to ground plane
modelling and priors alignment.

## 1. Current Pipeline: Mathematical Audit Summary

The implemented pipeline (L1–L6) is mathematically sound in its core
operations. Each layer has verified, correct implementations:

| Layer | Algorithm                 | Verified | Notes                                                                                                                                                                            |
| ----- | ------------------------- | -------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| L3    | EMA background settling   | ✓        | Correct `(1−α)·old + α·new` with warmup/freeze/lock                                                                                                                              |
| L3    | Convergence gating        | ✓        | Four-threshold multi-condition gate (coverage, spread-delta, stability, confidence)                                                                                              |
| L4    | Height-band ground filter | ✓        | O(n) in-place compaction, correct bounds                                                                                                                                         |
| L4    | DBSCAN clustering         | ✓        | Deterministic ordering when using DBSCANClusterer (centroid sort, no subsampling); production l4perception.DBSCAN path may be non-deterministic under MaxInputPoints subsampling |
| L4    | PCA/OBB                   | ✓        | Closed-form 2×2 eigenvalue with degenerate-case guards                                                                                                                           |
| L5    | CV Kalman filter          | ✓        | Correct F, H, Q, R matrices; covariance capping                                                                                                                                  |
| L5    | Mahalanobis gating        | ✓        | Correct 2×2 inverse with physical plausibility guards                                                                                                                            |
| L5    | Hungarian assignment      | ✓        | Jonker–Volgenant O(n³) variant, correct potentials                                                                                                                               |
| L5    | Heading disambiguation    | ✓        | Velocity + displacement fallback with 60°–120° jump rejection                                                                                                                    |
| L6    | Rule-based classification | ✓        | 7-tier cascade with per-class confidence scoring                                                                                                                                 |

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
                    │  P4: Unified L3/L4      │
                    │  Settling               │
                    │  [enables P3]           │
                    └────────────┬────────────┘
                                 │ settlement core feeds
                                 ▼
                    ┌─────────────────────────┐
                    │  P3: Ground Plane +     │
                    │  Vector Scene (L4)      │
                    │  [builds on P4]         │
                    └────────────┬────────────┘
                                 │ better foreground + geometry
                                 ▼
                    ┌─────────────────────────┐
                    │  P2: Velocity-Coherent   │
                    │  Foreground (L3–L5)      │
                    │  [enhances P1+P3]        │
                    └────────────┬────────────┘
                                 │ produces static geometry
                                 ▼
                    ┌─────────────────────────┐
                    │  Sign/Surface Anchors   │
                    │  + L7 Scene Corridors   │
                    │  [consumes L7 scene]    │
                    └─────────────────────────┘
```

The recommended sequence is:

1. **P1** — Geometry-coherent track state (standalone, highest visible impact)
2. **P4** — Unified L3/L4 settling (infrastructure, enables P3)
3. **P3** — Ground plane + vector scene (builds on P4 settlement core)
4. **P2** — Velocity-coherent foreground (independent, enhances P1+P3)
5. **Sign anchors** — requires L7 scene geometry from P3

## 3. Open Questions Addressed

### Q1. How does tile-plane fitting align with the vector scene map?

**Answer: Yes, for three specific and measurable scenarios — and the
construction path from tiles to vector-scene polygons is well-defined.**

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

**Tile → vector-scene alignment.** The ground plane and vector scene map use
the same underlying geometry but at different granularities:

| Representation       | Source             | Granularity                        | Lifecycle                                  |
| -------------------- | ------------------ | ---------------------------------- | ------------------------------------------ |
| 1 m Cartesian tile   | L4 streaming PCA   | Fixed grid, per-tile plane (n, d)  | Per-frame accumulation, settles in 20–50 s |
| Vector-scene polygon | Region-grown tiles | Variable area, simplified boundary | Constructed from settled tiles, locked     |
| OSM prior polygon    | External GeoJSON   | Road/pavement/building outlines    | Loaded once, used as alignment reference   |

The construction path is bottom-up and additive:

1. **Tiles settle independently** via streaming PCA (O(1) per point per tile).
2. **Region growing** merges adjacent settled tiles with compatible normals
   (angle < 2°) and offsets (ΔZ < 3 cm) into polygons.
3. **Polygon simplification** (Douglas–Peucker) reduces vertex count per LOD.
4. **OSM alignment** computes a translation/rotation offset between observed
   polygons and OSM prior polygons, producing a diff for map editing.

The tile plane and vector scene are **not competing representations** — tiles
are the compute substrate, polygons are the output format. Both use the same
plane equation (n, d) per surface region. Storage uses the same `GroundTile`
struct proposed in `ground-plane-extraction.md`, serialised to SQLite for
tiles and exported as GeoJSON for vector-scene polygons.

This alignment is confirmed across all four source documents:

- `ground-plane-maths.md` → height-band filter (current runtime)
- `ground-plane-extraction.md` → tile-plane fitting (§2, proposed)
- `20260221-ground-plane-vector-scene-maths.md` → streaming PCA + settlement
- `vector-scene-map.md` → region-grown polygons + LOD hierarchy

### Q2. How should observed geometry align with OSM, and how do we propose edits?

**Answer: OSM is the canonical remote store. Our tools diff observed geometry
against the community map and produce real-world OSM edits.**

The initial plan defers a velocity.report-hosted geometry service. Community
members should be improving the OpenStreetMap map first and foremost. The
project's role is to provide tooling that makes it easy to:

1. **Diff** observed geometry (settled tile-plane polygons, kerb lines,
   building footprints visible to the sensor) against the current state of
   OSM for the deployment area.
2. **Propose edits** that align the community map with observed ground truth,
   packaged as standard OSM changesets or JOSM-compatible `.osm` files.
3. **Quantify misalignment** with translation and rotation offset metrics
   between observed and mapped geometry.

**OSM-first workflow:**

```
Sensor observations (settled tiles)
        │
        ▼
Region-grow into polygons (GeoJSON)
        │
        ▼
Download OSM extract for deployment area
        │
        ▼
Compute alignment: translation vector (dx, dy, dz) + rotation offset (dθ)
        │
        ▼
Diff: identify geometry in observations not present in OSM (new features)
       and geometry in OSM not matching observations (misaligned features)
        │
        ▼
Generate proposed edits as OSM-compatible changesets
        │
        ▼
Human review in JOSM/iD → commit to OSM
```

**Translation/rotation offset model.** The alignment between observed
geometry and OSM priors is computed as a rigid transform:

- **Translation vector** (dx, dy, dz) in metres — accounts for GPS offset,
  datum differences, systematic survey error, and altitude discrepancies.
  The z-component is required because OSM Simple 3D Buildings include height
  data (`height=*`, `min_height=*`, `roof:height=*`, `building:levels=*`)
  and because LiDAR observes structures such as footbridges, elevated roads,
  and building facades at their true elevation. The expected/predicted delta
  between OSM map elevation and observed GPS altitude should be maintained
  as a diagnostic metric.
- **Rotation offset** dθ in radians — accounts for sensor heading
  misalignment relative to map north.
- **Confidence** — derived from the number and spatial distribution of
  matched features (buildings, kerbs, road edges).

When an OSM prior exists, the alignment offset tells us how far our
observations differ from the map. If a street has 80% of buildings present
at one fixed set of coordinates, we treat the OSM positions as authoritative
and compute an offset vector to align our observations. The remaining 20%
of unmapped or misaligned features become candidate edits.

**GPS role:** GPS provides an initial translation estimate and takes
precedence when no OSM prior is available. However, GPS may itself be wrong
(multipath, ionospheric error, poor fix). The alignment system uses GPS as
a starting point but refines via feature matching against OSM. If the
GPS-derived position conflicts with a strong OSM prior (many matching
buildings), the OSM prior wins and the GPS offset is recorded as a
diagnostic metric.

**Edit generation principles:**

1. Each set of proposed world edits is identified as a real-world OSM edit
   — not an internal velocity.report artifact.
2. Edits provide geometry and a suggested offset vector; they do not move
   100% of existing objects to a new reference frame (which may itself be
   wrong).
3. Gaps between observed and mapped geometry are aligned to the external
   reference system to minimise spurious changes.
4. The tool reports confidence per edit and flags low-confidence changes for
   manual review.

**Future mode — diff proposals for existing map data.** A planned extension
will allow the system to generate structured diff reports that compare
observed geometry against existing OSM data and propose corrections. This
will include:

- Per-feature alignment quality (translation/rotation residual)
- Suggested geometry updates with provenance (sensor ID, capture time,
  number of observations, confidence)
- Changeset comments referencing the velocity.report evidence package
- A human review step before any upload — the system never modifies OSM
  autonomously

See [docs/plans/lidar-l7-scene-plan.md](../../docs/plans/lidar-l7-scene-plan.md)
§OSM priors service for the architectural context.

**Data persistence and the three data feeds.** While only edits for missing
or misaligned geometry are exported to OSM, the full captured scene is
stored locally. The system maintains a strict separation between three
data feeds:

| Data feed           | Storage                   | Contents                                                                                                 | Retention                                              |
| ------------------- | ------------------------- | -------------------------------------------------------------------------------------------------------- | ------------------------------------------------------ |
| **Raw sensor**      | PCAP/PCAPNG files         | Raw UDP packets from the LiDAR sensor                                                                    | Kept for replay/debug; large, not in production DB     |
| **Debug capture**   | VRLOG files               | Per-frame foreground points, full point clouds, diagnostic overlays                                      | On-demand recording; not retained in production DB     |
| **Production data** | `sensor_data.db` (SQLite) | Background grid, settled ground plane, vector map, bounding boxes, tracks, classification, analysis runs | Persistent; exportable; the canonical production store |

The `sensor_data.db` does **not** store raw PCAP data or VRLOG foreground /
full-frame point data. It stores the processed, accumulated results:
background grid snapshots, region snapshots, track histories, cluster
records, ground-plane tiles, vector-scene polygons, and analysis run
statistics. This keeps the production database compact and self-contained
while the full scene evidence remains accessible for offline analysis via
the raw PCAP and VRLOG files.

### Q3. Can LiDAR intensity create reliable pose anchors?

**Answer: Yes, with a well-defined reliability ladder and quantified
confidence bounds. Flagged for possible future improvement.**

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

**Future improvement note:** The thresholds above are initial estimates.
Once the sign anchor system is implemented and evaluated on multiple sites,
the confidence bounds and tier thresholds should be revisited with empirical
data. In particular, the wall/facade tier may achieve better accuracy than
estimated if surface roughness is characterised per anchor.

### Q4. Should ground plane share settlement core with L3, and how does this align with sign anchors and bodies-in-motion?

**Answer: Yes, but with separated readiness outputs — and the unified
settlement core is designed to be forward-compatible with both sign anchors
and the bodies-in-motion kinematic extensions.**

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

**Forward compatibility with sign anchors.** The reflective-sign-pose-anchor
proposal (§8) defines a `FrameStabilitySignal` state machine with states:
`unknown → stable ↔ shaky → moving → reacquire`. The unified settlement
core's freeze/thaw policy must be compatible with this signal:

- When `FrameStabilitySignal = stable`, settlement proceeds normally.
- When `FrameStabilitySignal = shaky`, settlement pauses learning (freeze)
  but does not reset — transient vibration should not invalidate settled
  geometry.
- When `FrameStabilitySignal = moving`, settlement triggers a hard reset
  (re-enter LEARNING) because the sensor pose has changed.

The unified `SettlementCore` already supports freeze/thaw via its `LOCKED`
state. The sign-anchor integration adds a new freeze trigger (stability
signal) alongside the existing trigger (observation divergence). These
are additive — the settlement core does not need restructuring.

**Forward compatibility with bodies-in-motion.** The bodies-in-motion plan
introduces L7 scene-constrained prediction (road corridors, stop-line
awareness). The unified settlement core produces the static geometry that
L7 corridors are built from:

- Settled ground tiles → region-grown polygons → road/pavement classification
- Road polygons define corridor boundaries for L7 track prediction
- Kerb polygons define lane edges for scene-constrained gating

The settlement core's `READY_GROUND_GEOM` signal is the prerequisite for
L7 corridor construction. If settlement resets (due to scene change or
sensor movement), L7 corridors must also be invalidated. This dependency
is one-directional (settlement → L7) and does not create a feedback loop.

**Summary:** The unified settlement core is a future-forward foundation
that serves three downstream consumers: L4 ground filtering, sign-anchor
stability diagnostics, and L7 scene corridors. No structural changes are
needed to support these future extensions — only additional readiness
signals and freeze triggers.

### Q5. CV tracker fragmentation: what is future-forward vs throwaway work?

**Answer: Only pursue additive work that feeds into L7 corridors. Avoid
intermediate optimisations at layers that will be replaced.**

The constant-velocity (CV) Kalman filter fragments tracks when the true
motion deviates from constant velocity for longer than the gating tolerance
allows. Three fragmentation regimes exist (braking, turning, sparse
clusters), but not all remedies are future-forward.

**Future-forward principle:** If L7 will provide polyline corridors for
expected occlusion, noise, reflection zones, and lane boundaries, then
optimisations that duplicate this corridor awareness at L5 are throwaway
work. Only additive work that either (a) feeds into L7 corridor
construction or (b) remains useful after L7 corridors exist should be
pursued.

**Future-forward work (pursue):**

1. **CA model extension (L5 state vector 4 → 6).** Adding acceleration
   states is purely additive — the CA model is a strict superset of CV.
   It eliminates braking/acceleration fragmentation (Regime 1) and remains
   useful after L7 corridors exist because it improves state estimation
   quality regardless of scene constraints. The per-frame cost increase is
   ~20% for prediction and ~30% for update (~1 ms total).

2. **IMM with CV+CA (L5).** The Interacting Multiple Model blender selects
   the best motion model per track per frame. This is additive over CA and
   future-compatible — IMM can later incorporate CTRV or corridor-aware
   models as additional modes without restructuring.

3. **Settlement core (P4) producing geometry for L7 corridors.** The
   unified settlement core produces settled ground tiles that L7 will
   consume to construct road/pavement corridor polylines. This is enabling
   infrastructure, not an intermediate optimisation.

4. **Benchmark harness for fragmentation measurement.** Measuring
   fragmentation rate on fixed PCAPs is permanently useful — it validates
   both current and future tracker improvements.

**Not future-forward (avoid):**

1. ~~CTRV model at L5.~~ CTRV captures turning dynamics, but L7 corridor-
   constrained prediction will handle turns more robustly by clipping
   predictions to road polygons. Implementing CTRV at L5 alone is effort
   that L7 will supersede.

2. ~~Ad-hoc sparse-cluster linking at L5.~~ Sparse-cluster re-association
   without scene corridors is guesswork. L7 corridors predict where
   occluded objects should reappear; without that context, L5 linking
   heuristics are fragile and will be replaced.

3. ~~L5 gating relaxation for specific road geometries.~~ Tuning L5 gates
   for turns or intersections encodes scene knowledge at the wrong layer.
   This knowledge belongs in L7 corridors.

**Recommended sequencing (future-forward only):**

1. **P1: Geometry-coherent track state** — fixes bounding box instability,
   additive, no L7 dependency.
2. **CA model** — extends L5 state vector, additive, no L7 dependency.
3. **IMM (CV+CA)** — additive over CA, future-compatible with additional
   modes.
4. **P4: Unified settlement core** — enables L7 corridor construction.
5. **L7 corridors** — consumes settled geometry, constrains predictions,
   enables sparse-cluster linking and turn handling.

This sequence avoids dead ends: every item either persists in the final
architecture or enables a downstream capability.

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

**Experiment proposal:** See
[data/experiments/try/velocity-coherent-baseline-comparison.md](../experiments/try/velocity-coherent-baseline-comparison.md)
for the structured experiment design.

### Q7. Which defaults are backed by repeatable comparisons vs provisional?

**Answer: Audit by config key with evidence classification.**

| Config Key                        | Default | Evidence Level                                                 | Source                                      |
| --------------------------------- | ------- | -------------------------------------------------------------- | ------------------------------------------- |
| `background_update_fraction`      | 0.05    | **Theoretical** — standard EMA α for 20-frame effective window | EMA theory                                  |
| `closeness_multiplier`            | 2.0     | **Provisional** — tuned on kirk0                               | Needs multi-site validation                 |
| `safety_margin_meters`            | 0.01    | **Theoretical** — sensor noise floor                           | Hesai XT32 spec                             |
| `noise_relative`                  | 0.02    | **Provisional** — approximate range-dependent noise            | Needs empirical validation per sensor model |
| `neighbor_confirmation_count`     | 2       | **Provisional** — tuned on kirk0                               | Needs multi-site validation                 |
| `warmup_duration_nanos`           | 5×10⁹   | **Empirical** — 5 s settling observed on kirk0                 | Confirmed on one site                       |
| `foreground_dbscan_eps`           | 0.3     | **Literature** — typical urban DBSCAN ε                        | Ester et al. 1996                           |
| `foreground_min_cluster_points`   | 3       | **Provisional** — tuned for XT32 at 10 Hz                      | Needs validation at other frame rates       |
| `gating_distance_squared`         | 2.0     | **Theoretical** — χ²(2) at 84%                                 | Standard Kalman gating                      |
| `process_noise_pos`               | 0.1     | **Provisional** — tuned on kirk0                               | Sensitivity analysis needed                 |
| `process_noise_vel`               | 1.0     | **Provisional** — tuned on kirk0                               | Sensitivity analysis needed                 |
| `measurement_noise`               | 0.25    | **Provisional** — tuned on kirk0                               | Should derive from sensor spec              |
| `max_reasonable_speed_mps`        | 50.0    | **Domain** — 180 km/h upper bound                              | Appropriate for UK roads                    |
| `obb_heading_smoothing_alpha`     | 0.08    | **Provisional** — heavy smoothing for stability                | Superseded by P1 geometry model             |
| `obb_aspect_ratio_lock_threshold` | 0.25    | **Provisional** — may be too loose                             | Superseded by P1 geometry model             |

**Provisional keys** (those backed only by single-site tuning) should be
validated through the parameter sweep infrastructure (Phase 4.2) across at
least three sites with different road geometries before being considered
stable defaults.

**Validation plan:** See
[config/OPTIMISATION_PLAN.md](../../config/OPTIMISATION_PLAN.md) for the
structured plan to validate and graduate provisional defaults.

### Q8. Rotating bounding boxes: do geometry-coherent replacements improve replay results enough?

**Answer: Expected improvement is large and verifiable.**

The current reactive guard system (Guard 1: min points, Guard 2: aspect-ratio
lock, Guard 3: 90° jump rejection, Guard 4: EMA smoothing) treats each failure
mode independently. The geometry-coherent proposal replaces Guards 2, 3, and
the dimension sync logic with a single Bayesian model.

**Expected quantified improvements** (from the proposal, pending validation):

| Metric                            | Current (guards) | Expected (geometry model) |
| --------------------------------- | ---------------- | ------------------------- |
| Dimension stability (σ per track) | 0.3–0.5 m        | < 0.1 m                   |
| Heading drift (stationary, °/s)   | 2–5              | < 0.5                     |
| 90° jump frequency (per track)    | 0.1–0.3          | < 0.01                    |
| Convergence time (frames)         | 15–20            | 5–10                      |

**Validation protocol:** Run geometry-coherent tracker on kirk0 PCAP and
compare dimension stability, heading drift, and jump frequency against the
current guard-based tracker. Use existing `analysis.CompareReports` for
track-level comparison.

**Why this is the highest-priority work:** Bounding box instability is the
most visible artifact in the visualiser and the most frequently reported
user issue. It also degrades classification accuracy (dimension features
are noisy inputs to the rule cascade).

### Q9. L3/L4 settlement boundary: how to handle GPS alignment and priors?

**Answer: GPS is a translation/offset vector only. OSM priors are the
primary alignment reference. Observed geometry aligns to the external
reference system to minimise spurious changes.**

The ground-plane-vector-scene-maths proposal (§4.4) defines region-selection
scoring as:

S_R(p) = w_xy · w_z · w_obs · w_geom · w_density · w_prior

Each weight has a clear mathematical meaning and bounded sensitivity:

| Weight    | Formula              | Default σ or τ | Sensitivity                                        | Justification                   |
| --------- | -------------------- | -------------- | -------------------------------------------------- | ------------------------------- |
| w_xy      | exp(−d²_xy / 2σ²_xy) | σ_xy = 0.5 m   | High — controls tile boundary softness             | Half-tile width                 |
| w_z       | exp(−d²_z / 2σ²_z)   | σ_z = f(range) | High — controls ground/non-ground separation       | Derived from sensor noise model |
| w_obs     | C_obs(R) from L3     | [0, 1]         | Medium — modulates trust in under-observed regions | Direct from L3 confidence       |
| w_geom    | max(1 − \|r\|/τ, 0)  | τ = 3σ_z       | Medium — rejects points far from fitted plane      | Standard outlier gate           |
| w_density | C_density(R)         | [0, 1]         | Low — penalises sparse regions                     | Monotonic in observation count  |
| w_prior   | C_prior(R)           | 1.0 (no prior) | Low initially — grows with prior trust             | From vector-scene/OSM agreement |

The **coupling to existing config** (§4.5 of the proposal) is well-defined:

σ_z(r) = k_close · (noise_relative · r + safety_margin_meters)

This re-uses `closeness_multiplier`, `noise_relative`, and
`safety_margin_meters` from the L3 config, avoiding new magic numbers.

**GPS as offset vector.** GPS provides a translation vector (dx, dy, dz).
It does not define the reference frame for geometry — OSM does. The
z-component (dz) captures altitude offset between GPS-reported elevation
and the map datum; this is essential for aligning structures with height
data in OSM (footbridges, building heights, above-sea-level measurements).
The GPS role is:

1. **Initial alignment** — When no OSM prior is available, GPS provides
   the only geo-reference. The system uses GPS coordinates directly but
   records the fix quality (HDOP, satellite count) as a confidence metric.
2. **Prior alignment bootstrap** — When an OSM prior is available, GPS
   provides the initial guess for the translation/rotation offset between
   sensor frame and map frame. Feature matching against OSM buildings and
   road edges refines this offset.
3. **Conflict resolution** — If GPS position conflicts with a strong OSM
   prior (e.g. 80% of buildings in view match OSM at a different offset),
   the OSM prior takes precedence. The GPS-to-OSM discrepancy is recorded
   as a diagnostic metric.

**OSM alignment model.** When a street has 80% of buildings present at one
fixed set of coordinates in OSM, we assume the OSM positions are
authoritative. Our observations are aligned to the OSM frame using a
rigid transform (translation + rotation). The remaining 20% of unmapped
features become candidate edits — we provide the geometry and a suggested
offset vector, but align gaps to the external reference system to minimise
spurious changes.

**Key design decision:** w_prior defaults to 1.0 (neutral) and only
deviates when external priors are loaded. This ensures the ground plane
system works identically with and without priors — priors are strictly
additive. GPS contributes only as a translation/offset vector, never as
an absolute coordinate source that overrides observed geometry.

**Future mode — map diff proposals.** Once the alignment model is
established, the system can generate structured diff proposals comparing
observed geometry against OSM data. See Q2 above for the proposed OSM edit
workflow. A planned reference document will describe the diff format,
changeset generation, and human review process.

### Q10. Performance-versus-accuracy tradeoff for edge hardware.

**Answer: Budget allocation by layer with measured costs. A performance
measurement harness provides consistent regression detection.**

The Raspberry Pi 4 (ARM Cortex-A72, 1.8 GHz, 4 cores) must process each
frame within 100 ms at 10 Hz. The current measured budget (approximate):

| Layer | Operation                               | Time (ms) | % Budget |
| ----- | --------------------------------------- | --------- | -------- |
| L1–L2 | Parse + frame assembly                  | 1–2       | 2%       |
| L3    | Background update + foreground decision | 3–5       | 4%       |
| L4    | Ground filter                           | < 1       | < 1%     |
| L4    | DBSCAN clustering                       | 5–15      | 10%      |
| L4    | OBB computation                         | 1–2       | 2%       |
| L5    | Kalman predict + Hungarian + update     | 2–5       | 4%       |
| L6    | Classification                          | < 1       | < 1%     |
|       | **Total core pipeline**                 | **13–31** | **23%**  |
|       | Persistence, API, visualiser            | 10–30     | 20%      |
|       | **Headroom**                            | **39–77** | **57%**  |

**Budget for proposed additions:**

| Proposed                          | Estimated Cost         | Feasibility            |
| --------------------------------- | ---------------------- | ---------------------- |
| P1: Geometry-coherent (per track) | +0.5 ms                | ✓ Easily within budget |
| P3: Tile-plane PCA (per frame)    | +0.05 ms               | ✓ Negligible           |
| P4: Unified settlement            | ~0 (replaces existing) | ✓ No net cost          |
| CA model (6-state Kalman)         | +1 ms                  | ✓ Within budget        |
| IMM (2-model blend)               | +3 ms                  | ✓ Within budget        |
| P2: Velocity-coherent (full)      | +5–10 ms               | ⚠ Needs profiling      |
| Sign anchor detection             | +2 ms                  | ✓ Within budget        |

All proposed mathematical improvements fit within the available 57% headroom
individually. The combination of all proposals would consume ~12–17 ms
additional, still leaving ~40% headroom for future work.

**Performance measurement harness.** To detect regressions and validate
budget estimates, a consistent measurement harness runs fixed PCAPs on
test hardware and reports per-layer timing, frame drops, and throughput.
See [docs/plans/lidar-performance-measurement-harness-plan.md](../../docs/plans/lidar-performance-measurement-harness-plan.md)
for the harness design. The existing `make test-perf` target and nightly
CI job provide the infrastructure; the harness plan extends this with
per-layer breakdowns and hardware-specific baselines.

### Q11. Reference data coverage: does kirk0 overfit?

**Answer: Almost certainly yes; plan for five PCAPs with P40 sensor.**

Kirk0 is a single capture at one site with one sensor model. All provisional
defaults were tuned against it. The overfitting risk is real:

- **Road geometry:** Kirk0 may be flat; sloped-road defaults are untested.
- **Traffic mix:** Kirk0 may over-represent one vehicle class.
- **Sensor model:** P40-specific noise characteristics may not generalise.
- **Weather/lighting:** One capture cannot cover wet/dry/wind conditions.

**Five-PCAP test corpus plan (P40 sensor):**

| #   | Site description                         | Validates                                           | Status     |
| --- | ---------------------------------------- | --------------------------------------------------- | ---------- |
| 1   | Kirk0 (existing) — flat urban road       | Baseline defaults                                   | ✓ Captured |
| 2   | Sloped residential street (≥3° gradient) | Ground-plane tiling, height-band limits             | Planned    |
| 3   | School zone or park entrance             | Pedestrian/cyclist classification, low-speed tracks | Planned    |
| 4   | Multi-lane road or junction              | Turning vehicles, lane-crossing, merge/split        | Planned    |
| 5   | Rural or semi-rural road                 | Long-range sparse clusters, high-speed vehicles     | Planned    |

All captures use the Hesai P40 sensor to control for sensor-specific noise
characteristics. Each site needs ≥ 20 manually labelled tracks covering
the major classes (car, truck, cyclist, pedestrian at minimum).

The parameter sweep infrastructure (Phase 4.2) should run across all five
sites simultaneously, reporting per-site and aggregate metrics. Defaults
are only promoted from "provisional" to "empirical" when they perform
within 10% of optimal across all five sites.

See [docs/plans/lidar-test-corpus-plan.md](../../docs/plans/lidar-test-corpus-plan.md)
for the full test corpus plan.

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
guarantees only geometric approximation within a distance tolerance; it does
not by itself ensure containment or preserve topology. The implementation
must enforce or validate the containment invariant separately (for example,
via topology-preserving simplification, clipping simplified polygons to their
parent, and/or post-simplification checks).

**Gap 4: Priors integration should be OSM-first.** The vector-scene-map
(§12) describes a future online geometry-prior service; the maths proposal
(§4.4) includes a w_prior weight in region selection. But no code, no table,
and no config key exists for prior loading. The implementation path should
be OSM-first (see Q2):

1. Add an `osm_priors` config section with `enabled` (boolean) and
   `extract_path` (path to Overpass/JOSM export) fields.
2. Parse OSM GeoJSON exports into the same polygon representation used by
   the vector-scene-map.
3. Compute translation (dx, dy, dz) and rotation offset between observed
   polygons and OSM polygons via feature matching.
4. Feed polygon containment as w_prior into region selection scoring.
5. Default w_prior = 1.0 (neutral) when no priors are loaded.
6. Generate diff reports identifying geometry gaps between observations
   and the OSM map, with suggested edits for human review.

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

4. **Phase G4: OSM prior loading and alignment.** Load OSM GeoJSON exports
   as prior polygons. Compute rigid-transform alignment (translation +
   rotation) between observed polygons and OSM polygons via feature
   matching. Report misalignment as a diagnostic metric. Generate diff
   reports with suggested edits for OSM contributors. Apply w_prior weight
   in region selection scoring.

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

p_k = x[floor(h)] + (h − floor(h)) · (x[floor(h)+1] − x[floor(h)]) where h = (n−1)·k/100

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

| Priority | Item                                    | Layer | Effort       | Impact                                      | Readiness                             |
| -------- | --------------------------------------- | ----- | ------------ | ------------------------------------------- | ------------------------------------- |
| 1        | Geometry-coherent track state (P1)      | L5    | L (6–7 days) | **Very high** — fixes most visible artefact | Proposal complete, ready to implement |
| 2        | Tile-plane ground fitting (G1)          | L4    | M (3–4 days) | **High** — correctness for sloped roads     | Maths complete, needs implementation  |
| 3        | Unified settlement core (P4)            | L3–L4 | M (4–5 days) | **Medium** — infrastructure simplification  | Proposal complete, enables G2 and P3  |
| 4        | Classification config extraction (§5.1) | L6    | S (1–2 days) | **Medium** — removes magic numbers          | Straightforward refactor              |
| 5        | Five-site test corpus (§Q11)            | Cross | M (ongoing)  | **High** — validates all defaults           | Requires field data collection        |
| 6        | Speed percentile alignment (§5.2)       | L8    | S (2–3 days) | **Medium** — correctness for reports        | Implementation choices documented     |
| 7        | CA model (future-forward L5 extension)  | L5    | M (5–6 days) | **Medium** — reduces braking fragmentation  | Additive over CV, no L7 dependency    |
| 8        | IMM CV+CA blend                         | L5    | M (4–5 days) | **Medium** — adaptive model selection       | Additive over CA, future-compatible   |
| 9        | L7 corridors (enables turns + linking)  | L7    | L (10+ days) | **High** — turns, sparse linking, lanes     | Requires P4 settled geometry          |

**Critical path:** P1 → G1 → P4 → G2 → CA → IMM → L7 corridors.

Items 4, 5, and 6 are independent and can be pursued in parallel with the
critical path. Items 7–9 are the future-forward kinematic sequence that
avoids throwaway work (see Q5).

## 7. Cross-Reference to Existing Plans

This review consolidates and addresses open questions from:

- [platform-data-science-metrics-first-plan.md](../../docs/plans/platform-data-science-metrics-first-plan.md) — Q1–Q11
- [lidar-clustering-observability-and-benchmark-plan.md](../../docs/plans/lidar-clustering-observability-and-benchmark-plan.md) — benchmark design
- [speed-percentile-aggregation-alignment-plan.md](../../docs/plans/speed-percentile-aggregation-alignment-plan.md) — §5.2
- [lidar-visualiser-track-quality-score-plan.md](../../docs/plans/lidar-visualiser-track-quality-score-plan.md) — §5.3
- [lidar-bodies-in-motion-plan.md](../../docs/plans/lidar-bodies-in-motion-plan.md) — Q5, §5 item 7
- [lidar-l7-scene-plan.md](../../docs/plans/lidar-l7-scene-plan.md) — L7 planned capabilities
- [vector-scene-map.md](../../docs/lidar/architecture/vector-scene-map.md) — Q2, §4
- [ground-plane-extraction.md](../../docs/lidar/architecture/ground-plane-extraction.md) — Q1, §4
- [20260221-ground-plane-vector-scene-maths.md](proposals/20260221-ground-plane-vector-scene-maths.md) — Q9, §4
- [20260310-reflective-sign-pose-anchor-maths.md](proposals/20260310-reflective-sign-pose-anchor-maths.md) — Q3
- [20260222-geometry-coherent-tracking.md](proposals/20260222-geometry-coherent-tracking.md) — Q8
- [20260222-obb-heading-stability-review.md](proposals/20260222-obb-heading-stability-review.md) — Q8
- [20260220-velocity-coherent-foreground-extraction.md](proposals/20260220-velocity-coherent-foreground-extraction.md) — Q6
- [20260219-unify-l3-l4-settling.md](proposals/20260219-unify-l3-l4-settling.md) — Q4
