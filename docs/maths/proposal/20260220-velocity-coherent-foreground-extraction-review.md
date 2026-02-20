# Review: Velocity-Coherent Foreground Extraction Math

**Status:** Review of Design Math Proposal
**Date:** February 20, 2026
**Reviewed document:** [`20260220-velocity-coherent-foreground-extraction.md`](20260220-velocity-coherent-foreground-extraction.md)
**Version:** 1.0

---

## Summary Assessment

The proposal identifies a real and important problem: sparse-object recall,
fragmentation at entry/exit boundaries, and short-occlusion recovery are genuine
weaknesses of the current background-subtraction → DBSCAN → Kalman pipeline.
The overall direction—enriching the clustering and association stages with
velocity information—is sound. However, the mathematical formulation contains
several gaps that, left unaddressed, will likely produce disappointing results
or introduce failure modes worse than the status quo.

This review identifies **seven substantive concerns** grouped into three
categories: foundational assumptions (§1–§3), algorithmic design (§4–§5), and
validation methodology (§6–§7). Each section closes with a concrete
recommendation.

---

## 1. Point Correspondence Is Ill-Posed for Sparse Scanning LiDAR

### The problem

The proposal (§2) defines correspondence as a nearest-neighbour search in
position space between consecutive frames:

```
c(i) = argmin_j [ w_pos * ||p_i^t - p_j^{t-1}||_2 + w_ctx * C_ctx(i,j) ]
```

This assumes that a physical surface element hit at frame `t` will produce a
geometrically close return at frame `t-1`. For dense imaging sensors this
assumption is reasonable. For a spinning LiDAR with discrete rings, it fails
routinely because:

1. **Scan-pattern non-repeatability.** Ring/azimuth bins do not revisit the same
   surface patch every revolution. A 0.2° azimuth increment at 20 m range
   produces ~7 cm lateral spacing; at 50 m this grows to ~17 cm. Consecutive
   frames sample different parts of the same object surface.

2. **Sparsity amplifies ambiguity.** When a distant vehicle subtends only 3–8
   points, the probability of the same surface element being sampled in both
   frames is low. The "closest point" in `P_{t-1}` is likely a different
   physical point on the same (or a neighbouring) surface.

3. **Occlusion boundary motion.** At object edges, new surface patches appear
   and old ones disappear between frames. Correspondence there is not just
   noisy—it is structurally absent.

### Why it matters

The velocity `v_ij = (p_i^t - p_j^{t-1}) / Δt` inherits the full error of the
false correspondence. At Δt ≈ 0.1 s (10 Hz) and a matching error of 10 cm, the
velocity error is 1.0 m/s—comparable to pedestrian speed and a significant
fraction of slow-vehicle speed. This noise propagates directly into the 6D
clustering metric (§3) and the confidence score (§2), undermining both.

### Recommendation

Replace point-level correspondence with **cluster-level scene flow** or
**occupancy-grid velocity estimation**:

- **Option A (cluster-level):** After spatial DBSCAN, associate clusters across
  frames using the existing Hungarian machinery (L5 already does this). The
  Kalman-filtered velocity from L5 is a more reliable velocity estimate than
  any point-level correspondence can provide with sparse data.

- **Option B (grid-level):** Estimate a coarse velocity field on the 2D
  occupancy grid using cross-correlation or phase-correlation between
  consecutive foreground masks. This sidesteps the point identity problem
  entirely.

- **Option C (regularised scene flow):** If point-level velocity is truly
  needed, use a regularised scene flow formulation that enforces local
  smoothness (e.g., Dewan et al. 2016, "Rigid Scene Flow for 3D LiDAR
  Scans"). This trades per-point independence for spatial coherence and
  dramatically reduces noise at the cost of a global optimisation per frame.

Option A is the cheapest path: it requires no new correspondence code and
leverages the best velocity estimates the system already produces.

---

## 2. Velocity Noise Model Is Missing

### The problem

The proposal defines velocity confidence (§2):

```
q_i = exp(-d_pos / r_search)
      * exp(-σ²_neighbour / σ²_ref)
      * I(||v_i|| <= v_max)
```

but never models the noise of the velocity estimate itself. The dominant error
source for `v_ij` is the correspondence displacement error divided by Δt.

For this system's concrete parameters:

- Range noise: ~2% relative at distance r (from `noise_relative = 0.02`)
- At r = 30 m: σ_range ≈ 0.6 m
- Angular quantisation: additional ~5–10 cm lateral error at moderate ranges
- Combined position error per point: σ_pos ≈ 0.1–0.6 m (range-dependent)
- Velocity noise: σ_v = √2 · σ_pos / Δt (two independent point errors)

At Δt = 0.1 s and σ_pos = 0.15 m (near range): σ_v ≈ 2.1 m/s ≈ 7.6 km/h
At Δt = 0.1 s and σ_pos = 0.60 m (far range): σ_v ≈ 8.5 m/s ≈ 30.6 km/h

The far-range velocity noise is of the same order as the vehicle speeds being
measured. The confidence function `q_i` does not capture this because it
conflates matching distance (a correlation-quality proxy) with velocity
accuracy (a noise-propagation quantity).

### Recommendation

Define an explicit velocity noise covariance per point:

```
Σ_v(i) = (1/Δt²) * (Σ_pos(i) + Σ_pos(c(i)))
```

where `Σ_pos` is the sensor noise covariance at the measured range. Use
`Σ_v(i)` to weight the velocity contribution in downstream clustering, rather
than the heuristic `q_i`. This also provides a principled basis for the
"velocity-coherence gates" mentioned in §3.

---

## 3. The 6D Metric Has Dimensional and Scaling Defects

### The problem

The 6D clustering metric (§3):

```
D(u_i, u_j) = sqrt( α||Δx||² + β||Δv||² )
```

combines position (metres) and velocity (metres/second) under a single DBSCAN
`eps` threshold. Three issues arise:

1. **Dimensional incompatibility.** `α` and `β` must absorb the unit
   difference, but the proposal gives no guidance on their natural scales.
   Without this, `eps` has no physical interpretation and cannot be tuned from
   first principles.

2. **Curse of dimensionality.** DBSCAN neighbourhood queries in 6D are
   qualitatively different from 2D. The volume of a 6D ball of radius ε scales
   as ε⁶ (versus ε² in the current 2D clustering). For a fixed point density,
   far fewer random neighbours fall within the ball, so MinPts = 3 in 6D is
   not equivalent to MinPts = 3 in 2D—it is substantially more permissive.
   The proposal acknowledges this only informally ("require high confidence").

3. **Anisotropic noise.** Position and velocity have different noise structures
   (see §2 above). Isotropic Euclidean distance in the joint space does not
   respect this; a Mahalanobis-style normalisation per dimension would be more
   appropriate.

### Recommendation

Use a **normalised Mahalanobis-like metric**:

```
D²(u_i, u_j) = Δx^T Σ_x^{-1} Δx + Δv^T Σ_v^{-1} Δv
```

where `Σ_x` and `Σ_v` are diagonal covariance estimates for position and
velocity noise at the relevant range. This:

- removes the dimensional ambiguity (both terms are dimensionless),
- makes `eps` interpretable as a number of standard deviations,
- adapts automatically to range-dependent noise.

If full covariance is too expensive, even per-axis variance normalisation
(`Δx_k² / σ²_{x,k}` etc.) is a large improvement over the raw Euclidean form.

---

## 4. DBSCAN at MinPts = 3 in 6D Is Dangerously Noise-Sensitive

### The problem

Standard DBSCAN classifies a point as a core point if `|N_eps(i)| >= MinPts`.
At MinPts = 3 in 6D, a cluster can form from any triple of points that are
mutually within `eps`. The proposal adds post-hoc "velocity-coherence gates"
(mean `q_i`, velocity variance bounds) to filter these, but post-hoc filtering
does not prevent the core-point expansion that defines DBSCAN cluster
membership.

The failure mode: a noise triple forms a core, expands into nearby points via
density reachability, and absorbs legitimate sparse-object returns into a
spurious cluster. The post-hoc gate may then reject the merged cluster, losing
the real detections along with the noise.

The current production system uses MinPts = 5 in 2D, which is already tight
for noise rejection. Dropping to MinPts = 3 in 6D substantially increases the
effective noise sensitivity.

### Recommendation

Consider one of:

- **HDBSCAN** (hierarchical DBSCAN), which extracts clusters at the density
  level that maximises stability, avoiding a single global MinPts threshold.
  HDBSCAN naturally handles mixed-density scenes.

- **Constrained DBSCAN**: keep MinPts = 5 for the spatial dimensions, but
  allow clusters that also satisfy a velocity-coherence criterion to be
  retained at lower point counts. This is a softer relaxation than a global
  MinPts = 3.

- **Two-stage clustering**: first cluster in 2D (position) with the current
  robust parameters, then split/retain sub-clusters by velocity coherence.
  This preserves the noise rejection of spatial DBSCAN while adding velocity
  discrimination.

The two-stage approach is the most conservative and lowest-risk option. It
reuses the existing, validated 2D DBSCAN path and adds velocity as a
refinement rather than a replacement.

---

## 5. Track Continuity Model Weaknesses

### 5.1 Linear prediction with linear uncertainty growth

The proposal (§4.1) uses:

```
x̂(t+τ) = x_t + vx_t * τ
R(τ) = R0 + kτ
```

The existing L5 tracker already uses a constant-velocity Kalman filter with
**quadratic** position uncertainty growth (from the standard F P F^T + Q
propagation). The proposal's linear `R(τ)` is inconsistent with Kalman
covariance dynamics and is less conservative—it under-predicts uncertainty at
longer horizons.

For a constant-velocity model with process noise σ_a on acceleration, the
full covariance propagation gives (neglecting O(τ³) and higher terms):

```
σ²_pos(τ) = σ²_pos(0) + 2·σ²_vel(0)·τ + σ²_a·τ²
```

Position variance grows at least quadratically in τ, not linearly. Using linear
growth will produce under-sized association gates at longer coasting intervals,
causing missed re-associations precisely in the occlusion recovery scenarios
the proposal targets.

**Recommendation:** Use the existing Kalman covariance propagation from L5 for
post-tail prediction. This is already implemented, tested, and physically
consistent.

### 5.2 Fragment merge score is an ad-hoc average

The merge score (§4.3):

```
S_merge = (S_pos + S_vel + S_traj) / 3
```

assigns equal weight to three sub-scores with potentially different
distributions and informativeness. A better formulation is a
**log-likelihood ratio** under a "same object" vs. "different objects"
hypothesis:

```
Λ = log P(observations | same track) - log P(observations | different tracks)
```

For Gaussian kinematics, this reduces to a Mahalanobis-like score that
naturally weights each signal by its noise. The current formulation could merge
fragments that agree in trajectory but have wildly different velocities (if
`S_traj` happens to compensate), or miss valid merges where two of three scores
are marginal but all point in the same direction.

**Recommendation:** Define the merge criterion as a statistical test, not an
arithmetic average. At minimum, use a weighted sum where weights are inversely
proportional to the variance of each score component.

### 5.3 Adaptive regularisation thresholds are arbitrary

The point-count breakpoints (§4.2):

```
n >= 12: relaxed tolerance
6 <= n <= 11: medium tolerance
3 <= n <= 5: strict tolerance
```

have no mathematical justification. The real quantity that matters is the
uncertainty of the cluster centroid, which is σ_centroid ≈ σ_point / √n. This
gives a continuous, principled scaling rather than ad-hoc breakpoints:

- At n = 3: centroid uncertainty ≈ 0.58 · σ_point
- At n = 12: centroid uncertainty ≈ 0.29 · σ_point
- At n = 50: centroid uncertainty ≈ 0.14 · σ_point

**Recommendation:** Replace the discrete tiers with a continuous tolerance
function: `tolerance(n) = k · σ_point / √n`, clamped to physical plausibility
bounds. This removes arbitrary breakpoints and scales gracefully.

---

## 6. The Proposal Ignores the Existing L3 Bottleneck

### The problem

The document is titled "Foreground Extraction" but the mathematical content
addresses clustering and tracking, not foreground/background classification.
The current system's sparse-object recall is limited primarily by L3: the
background-grid EMA classifier decides which points reach L4. If a distant
vehicle's returns are classified as background (within the EMA baseline ± τ),
no amount of velocity-coherent clustering will recover them—they are discarded
before clustering begins.

The proposal is silent on how velocity information feeds back to the L3
foreground gate. This is the most important architectural question and it
receives no mathematical treatment.

### Why it matters

For a vehicle at 50 m range moving at 10 m/s perpendicular to the sensor line
of sight: frame-to-frame radial displacement ≈ 0 (tangential motion). The L3
range-based foreground detector sees no change in range and classifies the
returns as background. The vehicle is invisible to the existing pipeline
**regardless** of downstream velocity-coherent processing.

For a vehicle approaching head-on at 50 m: frame-to-frame range change ≈
10 m/s × 0.1 s = 1.0 m. The L3 closeness threshold at r = 50 m is roughly:

```
τ = 3.0 · (s_c + 0.02 · 50 + 0.01) + 0 = 3.0 · (s_c + 1.01)
```

For a well-settled cell with s_c ≈ 0.1 m: τ ≈ 3.3 m, which passes. But for
moderate s_c or during warmup, this margin narrows quickly.

### Recommendation

If the goal is truly improved sparse-object foreground extraction, the
mathematical model must address one of:

- **Velocity-gated foreground bypass:** Allow points that fail L3 but have
  velocity-consistent neighbours (from a previous frame's confirmed tracks) to
  be promoted to foreground. This is a feedback path from L5 → L3.

- **Parallel foreground channel:** Run a second foreground classifier that uses
  frame-difference or temporal gradient rather than EMA range baseline. Feed
  both channels to clustering.

- **Deferred foreground:** Cluster all points (foreground and background)
  using the velocity-coherent metric, and remove background clusters as a
  post-clustering step. This inverts the current L3 → L4 dependency.

Without one of these, the velocity-coherent path inherits the same L3
foreground bottleneck that limits the current system.

---

## 7. Validation Methodology Is Insufficient

### The problem

Section 9 states:

> Mathematical assumptions should be accepted only if replay evaluation
> confirms improvement.

The expected-benefits section (§6) gives ranges like "+20% to +40%" without:

1. A definition of the baseline metric.
2. A ground-truth methodology.
3. A statistical framework (confidence intervals, significance tests).
4. A data-quality requirement for the replay segments.

### Why it matters

Without ground truth, "sparse-object recall" is undefined—recall relative to
what? Manual annotation? A heuristic proxy? Different answers give different
numbers. The ±20% range band is wide enough to declare success in almost any
outcome.

### Recommendation

Before implementation, define:

1. **Ground-truth procedure:** Manual annotation of a representative set of
   PCAP segments with known vehicle trajectories (or, ideally, GPS/RTK
   reference from instrumented vehicles).

2. **Primary metrics with mathematical definitions:**
   - Recall: proportion of ground-truth trajectories captured with ≥ X%
     temporal overlap.
   - Precision: proportion of system tracks that correspond to a ground-truth
     trajectory.
   - MOTA/MOTP: standard multi-object tracking metrics (Bernardin & Stiefelhagen 2008).
   - Fragmentation: number of track ID switches per ground-truth trajectory.

3. **Statistical test:** Paired comparison (current vs. velocity-coherent) on
   the same replay segments, with confidence intervals and a pre-registered
   significance threshold.

4. **Runtime budget:** Define the maximum acceptable per-frame latency increase
   in milliseconds, not just "< 20% overhead." The current pipeline has
   concrete timing constraints from the sensor frame rate.

---

## 8. Complexity Estimate for 6D Spatial Index Is Optimistic

The proposal (§7) claims "approximately O(N log N)" for correspondence with
indexed neighbourhood search. This is reasonable for low-dimensional spatial
indices (k-d trees perform well in 2–3D). In 6D, k-d trees degrade toward
O(N) per query due to the curse of dimensionality—the fraction of the tree
pruned per query shrinks exponentially with dimension.

For N ≈ 1000 foreground points (a typical frame after L3 filtering), this is
not a practical concern—brute-force O(N²) ≈ 10⁶ is acceptable at 10 Hz. But
the O(N log N) claim should be corrected to avoid setting false expectations
for larger point counts or denser sensors.

**Recommendation:** State the expected practical complexity as O(N²) for the
correspondence search (with constant small N), and verify empirically that
this meets the frame-rate budget. If N grows substantially (e.g., multi-sensor
fusion), revisit with approximate nearest-neighbour methods (LSH, random
projection trees).

---

## 9. Constructive Summary

The concerns above do not invalidate the proposal's direction. Velocity
enrichment of the clustering and tracking pipeline is a natural and valuable
evolution. The following prioritised changes would substantially strengthen the
mathematical foundation:

| Priority | Concern | Recommended Change |
| -------- | ------- | ------------------ |
| **P0** | L3 bottleneck (§6) | Define the foreground feedback or bypass path mathematically before starting implementation |
| **P1** | Correspondence noise (§1, §2) | Replace point-level correspondence with cluster-level velocity from L5, or add a regularised scene flow formulation |
| **P1** | 6D metric scaling (§3) | Use variance-normalised (Mahalanobis-like) distance instead of raw weighted Euclidean |
| **P2** | MinPts = 3 fragility (§4) | Use two-stage clustering (2D spatial → velocity split) instead of a single 6D DBSCAN pass |
| **P2** | Uncertainty growth (§5.1) | Reuse existing Kalman covariance propagation for post-tail prediction |
| **P2** | Merge criterion (§5.2) | Replace arithmetic average with a log-likelihood ratio or variance-weighted score |
| **P3** | Adaptive thresholds (§5.3) | Replace discrete breakpoints with continuous 1/√n scaling |
| **P3** | Validation (§7) | Define ground-truth procedure and statistical test before implementation |
| **P3** | Complexity (§8) | Correct O(N log N) claim to O(N²) for 6D; verify empirically |

---

## References

- Bernardin, K. & Stiefelhagen, R. (2008). "Evaluating Multiple Object Tracking
  Performance: The CLEAR MOT Metrics." *EURASIP Journal on Image and Video
  Processing.*
- Dewan, A., Caselitz, T., Tipaldi, G. D. & Burgard, W. (2016). "Rigid Scene
  Flow for 3D LiDAR Scans." *IEEE/RSJ International Conference on Intelligent
  Robots and Systems (IROS).*
