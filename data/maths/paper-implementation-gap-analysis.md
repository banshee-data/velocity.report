# Paper-vs-Implementation Gap Analysis

- **Scope:** All 24 downloaded papers cross-referenced against production code (L3–L8)
- **Method:** Trace each algorithm from the paper through the Go implementation. Note where the code deviates from the paper's intent, where edge cases go unhandled, and where the behaviour is plausible but untested.

---

## Summary of Findings

| Severity                  | Count | Description                                                                          |
| ------------------------- | ----- | ------------------------------------------------------------------------------------ |
| **Mathematical gap**      | 7     | Implementation deviates from the paper's mathematical formulation                    |
| **Missing edge case**     | 9     | The paper describes a condition the implementation does not handle                   |
| **Missing test**          | 11    | The behaviour is implemented, but the paper-specified edge case has no test coverage |
| **Future work (blocked)** | 8     | Requires papers that are currently behind paywalls                                   |

---

## 1. DBSCAN — Ester et al. (1996)

### Paper intent

DBSCAN defines three point categories: **core**, **border**, and **noise**. Border points are reachable from a core point but are not themselves core. The original paper assigns border points to the cluster of the _first_ core point that reaches them during expansion — a deliberate choice that makes the result order-dependent when a border point sits between two clusters.

### Implementation status

`l4perception/dbscan_clusterer.go` uses 2D Euclidean distance in XY, grid-accelerated neighbourhood queries, and deterministic output sorting. See [clustering-maths.md](clustering-maths.md) for the mathematical specification.

### Gaps

| ID  | Type                  | Description                                                                                                                                                                                                                                                                                                              | Impact                                              | Test needed                                                                                                       |
| --- | --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------- |
| D1  | **Missing edge case** | Border point assignment is order-dependent in the original DBSCAN — the paper acknowledges this. The implementation sorts output clusters by centroid for reproducibility, but has no test for border points at cluster boundaries when two clusters share a border region.                                              | Low (deterministic sorting mitigates)               | Test: two adjacent clusters sharing a border point; verify stable assignment across permutations of input order.  |
| D2  | **Missing test**      | The paper's MinPts definition counts the query point itself. Verify our `MinPts` semantics match — a core point requires ≥ MinPts neighbours _including itself_. If the implementation excludes self, the effective density threshold is off by one.                                                                     | Medium — affects cluster formation at low densities | Test: construct exactly `MinPts` points within ε; verify one cluster forms. Construct `MinPts - 1`; verify noise. |
| D3  | **Missing edge case** | "Thin" clusters where ε is too large relative to cluster width can merge what should be two distinct clusters. The implementation has `MaxClusterDiameter` and `MaxClusterAspectRatio` post-filters, but no test for the merge pathology itself.                                                                         | Medium                                              | Test: two parallel lines of points separated by < 2ε; verify they produce 2 clusters, not 1.                      |
| D4  | **Future work**       | Schubert et al. (2017) — `Schubert2017DBSCANR` — revisits DBSCAN with formal analysis of parameter selection heuristics. **Paper not downloaded.** Could inform adaptive ε selection. See [lidar-clustering-observability-and-benchmark-plan.md](../../docs/plans/lidar-clustering-observability-and-benchmark-plan.md). | —                                                   | Blocked on paper download                                                                                         |
| D5  | **Future work**       | Campello et al. (2013) — `Campello2013` — HDBSCAN for variable-density scenes. **Paper not downloaded.** The current fixed-ε approach struggles at long range where point density drops.                                                                                                                                 | —                                                   | Blocked on paper download                                                                                         |

---

## 2. Kalman Filter — Kalman (1960), Bewley (2016, SORT), Weng (2020, AB3DMOT)

### Paper intent

- **Kalman (1960):** Linear optimal estimator for Gaussian noise. Requirements: linear state transition, Gaussian process and measurement noise, correct noise covariance specification, and a symmetric positive-definite covariance matrix maintained at all times. Any of those four requirements violated, and optimality claims become optimistic.
- **Bewley (2016, SORT):** Kalman with a constant-velocity model plus Hungarian assignment for 2D MOT. Uses IoU-based association cost, not Mahalanobis.
- **Weng (2020, AB3DMOT):** SORT extended to 3D with Mahalanobis gating and a 7-state model `[x, y, z, θ, l, w, h]`. Uses 3D IoU for association.

### Implementation status

`l5tracks/tracking_association.go`, `tracking_update.go`: 4-state CV Kalman `[x, y, vx, vy]`, 2D position-only measurement, Mahalanobis gating, Hungarian assignment. See [tracking-maths.md](tracking-maths.md) for the full specification.

### Gaps

| ID  | Type                  | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                         | Impact                                                               | Test needed                                                                                                                                                                                            |
| --- | --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| K1  | **Mathematical gap**  | **Process noise model is simplified.** The continuous white-noise jerk model yields $Q = q \begin{bmatrix} dt^3/3 & dt^2/2 \\ dt^2/2 & dt \end{bmatrix}$ where the off-diagonal terms couple position and velocity uncertainty growth. The implementation uses diagonal-only Q: `P[0,0] += σ²_pos * dt`, `P[2,2] += σ²_vel * dt`. This underestimates the cross-covariance, making the gating ellipse overconfident in the velocity–position correlation direction. | Medium — affects gating accuracy for manoeuvring targets             | Test: simulate a target accelerating at 2 m/s²; compare gating ellipse size and shape between diagonal-Q and full-Q models. Verify that the full-Q ellipse captures the target when diagonal does not. |
| K2  | **Mathematical gap**  | **Covariance update uses naive $(I - KH)P$ form.** The Joseph stabilised form $(I - KH)P(I - KH)^T + KRK^T$ guarantees symmetry and positive-definiteness even with floating-point error. The current form can lose symmetry over many iterations.                                                                                                                                                                                                                  | Low (covariance capping mitigates), but affects long-running tracks  | Test: run 10,000 predict-update cycles with adversarial measurement noise; verify P remains symmetric and positive-definite. Compare P diagonal growth between Joseph and naive forms.                 |
| K3  | **Missing edge case** | **No explicit covariance symmetry enforcement.** After each update, the implementation does not force $P = (P + P^T)/2$. Over hundreds of frames, asymmetry can accumulate.                                                                                                                                                                                                                                                                                         | Low (NaN guard catches catastrophic cases)                           | Test: instrument P symmetry check after every update in a long-running track; log max asymmetry.                                                                                                       |
| K4  | **Missing test**      | **AB3DMOT uses 3D IoU for association, not just Mahalanobis.** When two tracks have similar position but very different bounding box sizes (car versus pedestrian nearby), Mahalanobis distance alone cannot distinguish them. IoU would naturally reject the mismatch.                                                                                                                                                                                             | Medium — risk of identity swap between adjacent unlike-class objects | Test: two tracks (car-sized and pedestrian-sized) at similar positions; verify that association does not swap identities. Consider adding OBB overlap as secondary gating criterion.                   |
| K5  | **Mathematical gap**  | **SORT uses $dt$-dependent state transition; our prediction step clamps `dt` to `MaxPredictDt`.** This is a deliberate engineering choice (documented), but the clamped prediction underestimates true position uncertainty during frame gaps. The paper does not address frame drops.                                                                                                                                                                              | Low — clamping is defensive; documented                              | Test: verify that after a 5-second frame gap, covariance is large enough to re-acquire the track at the physically-expected position.                                                                  |
| K6  | **Future work**       | **Kalman (1960) original paper** not downloaded. Foundational — needed to verify our notation and assumptions match the original formulation.                                                                                                                                                                                                                                                                                                                       | —                                                                    | Blocked on paper download                                                                                                                                                                              |
| K7  | **Future work**       | **Blom & Bar-Shalom (1988), IMM** not downloaded. The planned `imm_cv_ca_v2` engine depends on this.                                                                                                                                                                                                                                                                                                                                                                | —                                                                    | Blocked on paper download                                                                                                                                                                              |
| K8  | **Future work**       | **Julier & Uhlman (1997), UKF** not downloaded. Needed if nonlinear measurement models are added (e.g., polar measurements).                                                                                                                                                                                                                                                                                                                                        | —                                                                    | Blocked on paper download                                                                                                                                                                              |

---

## 3. Hungarian Assignment — Kuhn (1955), Munkres (1957)

### Paper intent

Optimal assignment in $O(n^3)$ for balanced square matrices. Rectangular problems require padding to square before solving.

### Implementation status

`l5tracks/hungarian.go`: Jonker-Volgenant variant with potential vectors, padded to square, costs ≥ `hungarianlnf` treated as forbidden. See [tracking-maths.md](tracking-maths.md) for the assignment cost formulation.

### Gaps

| ID  | Type             | Description                                                                                                                                                                                                                                                             | Impact                                                              | Test needed                                                                      |
| --- | ---------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------- | -------------------------------------------------------------------------------- |
| H1  | **Missing test** | **Numerical stability with extreme cost ranges.** The algorithm uses float64 internally but accepts float32 costs. When costs span many orders of magnitude (e.g., 0.001 vs 1e15), the potential subtraction step could lose precision. Papers assume exact arithmetic. | Low — in practice costs are Mahalanobis distances in a narrow range | Test: cost matrix with entries spanning [1e-6, 1e12]; verify correct assignment. |
| H2  | **Missing test** | **All-forbidden matrix.** When every entry is ≥ `hungarianlnf`, the result should be all -1. This is tested implicitly (one row forbidden) but not for the full-matrix case.                                                                                            | Low                                                                 | Test: NxN matrix of all `hungarianlnf`; verify all result entries are -1.        |
| H3  | **Future work**  | **Kuhn (1955) and Munkres (1957)** papers not downloaded. The implementation follows a Jonker-Volgenant variant; should verify algorithmic equivalence to the original Munkres formulation.                                                                             | —                                                                   | Blocked on paper download                                                        |

---

## 4. Background Model — Welford (1962), Stauffer & Grimson (1999)

### Paper intent

- **Welford (1962):** Numerically stable single-pass algorithm for running mean and variance: $M_n = M_{n-1} + (x_n - M_{n-1})/n$, $S_n = S_{n-1} + (x_n - M_{n-1})(x_n - M_n)$, variance $= S_n/(n-1)$. All samples weighted equally.
- **Stauffer & Grimson (1999):** GMM background subtraction with multiple modes per pixel, online EM updates, and adaptive component weights. Designed for scenes that have more than one stable state.

### Implementation status

`l3grid/background.go`, `foreground.go`: Single-component EMA per cell — not Welford's exact algorithm, not a multi-modal GMM. $\mu \leftarrow (1-\alpha)\mu + \alpha x$, spread: $(1-\alpha)s + \alpha |x - \mu_{old}|$. See [background-grid-settling-maths.md](background-grid-settling-maths.md) for the full model and [proposals/20260219-unify-l3-l4-settling.md](proposals/20260219-unify-l3-l4-settling.md) for the related settling proposal.

### Gaps

| ID  | Type                 | Description                                                                                                                                                                                                                                                                                                                                                 | Impact                                               | Test needed                                                                                                                                                                                                                                                    |
| --- | -------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| B1  | **Mathematical gap** | **Spread computation tracks mean absolute deviation, not variance.** Welford computes true running variance. The EMA spread tracks MAD. MAD ≈ 0.798σ for Gaussian distributions, so the effective closeness threshold is scaled by ~0.8 compared to what a σ-based threshold would give. This is _documented_ but not _tested_ against known distributions. | Medium — affects foreground sensitivity calibration  | Test: feed 10,000 samples from N(5.0, 0.1²) into a cell; verify that `RangeSpreadMeters` converges to approximately 0.08 (0.798 × 0.1), not 0.1. Document the MAD-to-σ relationship in [background-grid-settling-maths.md](background-grid-settling-maths.md). |
| B2  | **Mathematical gap** | **EMA has recency bias; Welford weights all samples equally.** With α=0.02, the effective window is ~50 samples. For a stably-drifting background this is actually preferable to equal weighting, but the maths doc does not quantify the effective window.                                                                                                 | Low — EMA is the correct choice for this application | Document: add effective window formula $n_{eff} = 2/\alpha - 1$ to [background-grid-settling-maths.md](background-grid-settling-maths.md).                                                                                                                     |
| B3  | **Mathematical gap** | **Stauffer (1999) uses multi-modal GMM; the implementation is single-mode.** The maths doc states this explicitly. Cells that alternately see two stable depths (a swinging gate, tree branches) will oscillate between foreground and background. The locked-baseline mechanism partially addresses this but is not a true multi-modal model.              | Medium — affects scenes with bimodal backgrounds     | Test: alternate observations between two stable ranges (5.0m and 5.5m); verify the cell does not persistently classify one range as foreground. Document when multi-modal background would be needed.                                                          |
| B4  | **Missing test**     | **Reacquisition boost convergence.** After a long foreground event (vehicle transit), the boosted α should cause faster re-convergence. No test verifies the settling time with boost versus without.                                                                                                                                                       | Medium                                               | Test: 100 frames of foreground, then background returns; measure frames to re-settle with boost=5.0 vs boost=1.0.                                                                                                                                              |
| B5  | **Missing test**     | **Locked baseline drift resistance.** The locked baseline updates with β=0.001. No test verifies that it survives a sustained transit (100+ foreground frames) without drifting.                                                                                                                                                                            | Medium                                               | Test: settled cell, 200 frames of foreground at a different range, then original range returns; verify locked baseline still matches pre-transit value within tolerance.                                                                                       |
| B6  | **Future work**      | **Welford (1962)** paper not downloaded. Needed to verify our MAD-based spread computation against Welford's exact formulation and document the deliberate deviation.                                                                                                                                                                                       | —                                                    | Blocked on paper download                                                                                                                                                                                                                                      |

---

## 5. OBB / PCA — Jolliffe (2002)

### Paper intent

PCA finds orthogonal axes of maximum variance. For 2D, the closed-form eigendecomposition of the 2×2 covariance matrix gives the principal direction. The eigenvector corresponding to the _larger_ eigenvalue is the direction of maximum spread.

### Implementation status

`l4perception/obb.go`: Correct closed-form 2×2 eigensolver, heading from atan2 of principal eigenvector, OBB extents from point projection onto eigenvector axes.

### Gaps

| ID  | Type                  | Description                                                                                                                                                                                                                                                                                                                            | Impact                                        | Test needed                                                                                                                           |
| --- | --------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------- |
| P1  | **Missing edge case** | **Degenerate covariance (all points identical).** When all points have the same X,Y, both eigenvalues are 0, discriminant is 0. The implementation falls through to the diagonal branch (`c00 >= c11` → X-axis). This produces a 0×0 OBB, which is correct, but the heading is arbitrary. No test covers this.                         | Low                                           | Test: 5 identical points; verify OBB has zero length/width and no NaN.                                                                |
| P2  | **Missing edge case** | **Negative discriminant guard.** The implementation guards `discriminant < 0` but this should never happen for a real symmetric covariance matrix (eigenvalues are always real). The guard uses lambda1 = c00 as fallback, which is incorrect if c11 > c00.                                                                            | Low (numerical edge only)                     | Test: construct covariance where c11 > c00 and verify correct principal axis even in degenerate branch.                               |
| P3  | **Mathematical gap**  | **PCA heading has 180° ambiguity.** The tracking layer (L5) resolves this using velocity/displacement disambiguation (Guards 1–3 in `tracking_update.go`). However, there is no test that verifies the full disambiguation chain end-to-end: PCA produces ambiguous heading → velocity resolver flips it → EMA smoother stabilises it. | Medium — heading flips cause visual artefacts | Test: track moving right (+X direction); verify OBB heading converges to ~0 rad, not ~π rad. Repeat for all four cardinal directions. |
| P4  | **Future work**       | **Jolliffe (2002)** PCA textbook not downloaded. Implementation is correct for 2×2 but the textbook covers stability considerations for near-degenerate cases that may apply.                                                                                                                                                          | —                                             | Blocked on paper download                                                                                                             |

---

## 6. SORT / DeepSORT — Bewley (2016), Wojke (2017)

### Paper intent

- **SORT:** Kalman filter (CV model on bounding box space [u, v, s, r, u̇, v̇, ṡ]) plus Hungarian assignment with IoU-based cost. Track lifecycle: create, confirm after hits, delete after max_age misses.
- **DeepSORT:** Adds appearance descriptor (deep re-identification features) for association, cascaded matching (confirmed tracks first, then tentative), and maximum cosine distance metric.

### Implementation status

The tracker follows the SORT architecture but differs in three respects: world-frame position instead of image-frame bounding box, Mahalanobis cost instead of IoU, and no appearance features. See [tracking-maths.md](tracking-maths.md) and [proposals/20260222-geometry-coherent-tracking.md](proposals/20260222-geometry-coherent-tracking.md).

### Gaps

| ID  | Type                  | Description                                                                                                                                                                                                                                                                                                                       | Impact                                                                 | Test needed                                                                                                                                                                                                                                 |
| --- | --------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| S1  | **Mathematical gap**  | **SORT uses IoU for association; our implementation uses Mahalanobis only.** IoU naturally handles size differences between objects — two objects at similar positions but different sizes get low IoU. Mahalanobis distance (position-only) cannot distinguish them. This matters when a pedestrian walks close to a parked car. | Medium                                                                 | Test: pedestrian track near stationary car-sized track; verify no identity swap. Consider hybrid cost: Mahalanobis + OBB IoU penalty.                                                                                                       |
| S2  | **Missing edge case** | **SORT's "minimum hits" before considering a track for output.** `HitsToConfirm` handles the lifecycle, but tentative tracks are still included in association — they compete for clusters with confirmed tracks. SORT processes confirmed tracks first.                                                                          | Medium — tentative tracks can displace confirmed tracks in association | Test: confirmed track coasting (1 miss); new cluster appears nearby; verify confirmed track gets priority over new tentative track.                                                                                                         |
| S3  | **Missing edge case** | **DeepSORT's cascaded matching.** Confirmed tracks are matched first; unmatched detections are then matched to tentative tracks. The current implementation matches all tracks simultaneously, which can allow a tentative track to take a cluster that a coasting confirmed track should have received.                          | Medium                                                                 | Same test as S2 — cascaded matching is the mechanism that fixes S2.                                                                                                                                                                         |
| S4  | **Missing feature**   | **DeepSORT's appearance features.** Without re-identification, tracks coasting through occlusion can only re-associate based on predicted position. For long occlusions (>1 second), the prediction may have drifted far enough that a new track is created instead.                                                              | High for long-occlusion scenarios                                      | Test: track coasts for 1 second, then object reappears 2m from predicted position; verify re-association succeeds. Appearance features may be out of scope for the "no black-box AI" tenet unless a simple, inspectable descriptor is used. |

---

## 7. MOT Evaluation — Bernardin (2008, CLEAR MOT), Luiten (2021, HOTA), Milan (2016, MOT16)

### Paper intent

- **CLEAR MOT (Bernardin 2008):** Defines MOTA (accuracy) and MOTP (precision). MOTA = 1 - (FN + FP + IDSW) / GT. Simple, widely used, prone to rewarding detectors over associators.
- **HOTA (Luiten 2021):** A higher-order metric that jointly evaluates detection and association quality, decomposed into DetA (detection accuracy) and AssA (association accuracy). Designed to avoid the biases in MOTA.
- **MOT16 (Milan 2016):** Benchmark protocol and evaluation methodology.

### Implementation status

`l8analytics/comparison.go` computes temporal IoU between track pairs across runs. `l8analytics/summary.go` computes aggregate run statistics. No MOTA, MOTP, or HOTA. See the [lidar-l8-analytics plan](../../docs/plans/lidar-l8-analytics-l9-endpoints-l10-clients-plan.md) for the planned analytics extension.

### Gaps

| ID  | Type                 | Description                                                                                                                                                                           | Impact                     | Test needed                                                                                                                    |
| --- | -------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| M1  | **Mathematical gap** | **No MOTA/MOTP computation.** Without these metrics, there is no quantitative way to compare tracking quality across parameter changes or code changes.                               | High for tuning validation | Implement: given ground-truth labels and tracker output, compute MOTA, MOTP, and identity switches per the CLEAR MOT protocol. |
| M2  | **Mathematical gap** | **No HOTA metric.** Luiten (2021) shows HOTA is more balanced than MOTA for evaluating both detection and association quality.                                                        | Medium                     | Implement after M1: HOTA decomposition into DetA and AssA.                                                                     |
| M3  | **Missing test**     | **Temporal IoU correctness.** `comparison.go` computes temporal IoU but has no tests for edge cases: zero-overlap tracks, identical tracks, one track fully contained within another. | Medium                     | Test: known track pairs with calculable IoU; verify computed values match.                                                     |
| M4  | **Future work**      | **Bernardin (2008)** paper not downloaded. Needed for exact MOTA/MOTP formulae and edge-case handling (e.g., how to handle tracks with no ground-truth match).                        | —                          | Blocked on paper download                                                                                                      |

---

## 8. Patchwork++ — Lim et al. (2022)

### Paper intent

Concentric zone-based ground segmentation that handles uneven terrain, curbs, and slopes. The point cloud is divided into concentric rings around the sensor; a local plane is fitted per zone and curvature-based region growing extends the ground estimate across zone boundaries.

### Implementation status

`l4perception/ground.go`: Height-band filter with floor/ceiling thresholds. No plane fitting, no zone segmentation, no slope awareness. See [ground-plane-maths.md](ground-plane-maths.md) and [docs/lidar/architecture/ground-plane-extraction.md](../../docs/lidar/architecture/ground-plane-extraction.md) for context. The [ground plane vector-scene proposal](proposals/20260221-ground-plane-vector-scene-maths.md) captures the planned improvement.

### Gaps

| ID  | Type                  | Description                                                                                                                                                                                                                                        | Impact                     | Test needed                                                                                                                                                             |
| --- | --------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| G1  | **Mathematical gap**  | **Height-band filter versus plane-based ground removal.** On sloped roads, a fixed height band will either miss ground points (floor too high) or include low objects as ground (floor too low). Patchwork++ handles this by fitting local planes. | High for hilly deployments | Test: simulate a 5° road slope; verify the height-band filter correctly classifies ground at both near range (uphill) and far range (downhill). Document failure cases. |
| G2  | **Missing edge case** | **Curb detection.** A height-band filter cannot distinguish between a 15cm kerb (ground) and a 15cm-high object (a small animal, a football). Patchwork++ uses curvature discontinuity to make this distinction.                                   | Low for current use case   | Test: points at ground level with a 15cm step; verify classification.                                                                                                   |

---

## 9. Dataset / Benchmark Papers — Caesar (2020, nuScenes), Behley (2019, SemanticKITTI), Sun (2020, Waymo)

### Paper intent

These papers define object class taxonomies, evaluation protocols, and benchmark expectations against which perception systems are usually measured.

### Implementation status

`l6objects/classification.go` uses a rule-based classifier with a 7-class taxonomy. See [classification-maths.md](classification-maths.md) for the classification specification.

### Gaps

| ID  | Type                  | Description                                                                                                                                                                                      | Impact                                 | Test needed                                                                                             |
| --- | --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | -------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| C1  | **Missing edge case** | **nuScenes defines 23 classes; our taxonomy has 7 (5 active + 2 reserved).** Construction vehicles, trailers, and barriers have no classification path and fall through to "dynamic".            | Low for residential traffic monitoring | Document: mapping table from our classes to nuScenes/KITTI classes for future evaluation compatibility. |
| C2  | **Missing test**      | **Classification boundary conditions.** The rule-based classifier uses hard thresholds (e.g., bus length ≥ 7m). No test verifies behaviour at exact boundary values (6.99m versus 7.01m length). | Medium                                 | Test: objects at exact threshold boundaries for each class; verify correct classification.              |

---

## 10. Constant Velocity Model — Schöller et al. (2020)

### Paper intent

A well-tuned constant velocity model can match or exceed deep learning baselines for short-horizon pedestrian trajectory prediction (≤4.8 seconds). The key finding: performance depends more on observation window length and process noise tuning than on the choice of model architecture.

### Implementation status

The tracker uses a CV model. The paper validates this choice. See [tracking-maths.md](tracking-maths.md) for the noise parameter specification.

### Gaps

| ID  | Type                 | Description                                                                                                                                                                                                                                                                                                                                                                                  | Impact                               | Test needed                                                                                                                                                                                       |
| --- | -------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| V1  | **Missing test**     | **The paper shows the CV model degrades beyond ~5 second prediction horizon.** `MaxMissesConfirmed` controls coasting duration. If coasting exceeds 5 seconds, the predicted position becomes unreliable. No test validates that prediction error grows as expected.                                                                                                                         | Medium                               | Test: track coasts for 1s, 3s, 5s, 10s; measure prediction error at each interval. Verify growth matches the CV model expectation ($\text{error} \propto t^2$ for accelerating targets).          |
| V2  | **Mathematical gap** | **The paper recommends tuning the observation window — how many past frames are used for velocity estimation.** The Kalman filter uses all observations implicitly through the recursive update; the effective window is controlled by process noise (higher noise weights recent observations more heavily). No analysis maps `ProcessNoiseVel` to an equivalent observation window length. | Low — but useful for tuning guidance | Document: derive the effective observation window from the current `ProcessNoiseVel` setting and compare against Schöller's recommended values. Record in [tracking-maths.md](tracking-maths.md). |

---

## 11. Sensor Manual — Hesai Pandar40P

### Paper intent

Sensor specifications: angular resolution, range accuracy, beam pattern. Useful primarily for validating that model parameters match what the hardware actually guarantees.

### Implementation status

Grid dimensions (40 rings, 1800 azimuth bins) match the Pandar40P's 40-beam, 0.2° horizontal resolution.

### Gaps

| ID  | Type                  | Description                                                                                                                                                                                                                                                                                                                                  | Impact                               | Test needed                                                                                                                                                                                                                  |
| --- | --------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| HW1 | **Missing test**      | **Range accuracy specification.** The manual specifies ±2cm at 0–30m and ±3cm at 30–200m. `NoiseRelativeFraction` (default 0.01 = 1%) gives 1cm noise at 1m and 10cm at 10m — 5× the actual sensor spec at short range. The model is more conservative than the hardware, which widens the closeness threshold unnecessarily at close range. | Low — conservative is safe           | Test: verify `NoiseRelativeFraction = 0.01` produces closeness thresholds consistent with the Pandar40P spec at 5m, 20m, 50m, and 100m. Consider a range-dependent noise model if over-conservatism causes false foreground. |
| HW2 | **Missing edge case** | **Beam pattern non-uniformity.** The Pandar40P has non-uniform vertical beam spacing (denser near the horizon). Uniform ring indexing may over-resolve near-horizontal beams and under-resolve steep beams.                                                                                                                                  | Low for current flat-road deployment | Document only — no code change needed.                                                                                                                                                                                       |

---

## Papers Reviewed With No Significant Gaps

These papers are in [references.bib](references.bib) and were reviewed. The implementation either correctly excludes them (deep learning, scope out) or uses them for context only.

| Paper                        | Key                                         | Assessment                                                                               |
| ---------------------------- | ------------------------------------------- | ---------------------------------------------------------------------------------------- |
| PointPillars (Lang 2019)     | `Lang2019`                                  | Deep learning; future L6 replacement. Correctly excluded by the "no black-box AI" tenet. |
| PointNet/PointNet++          | `Qi2017*`                                   | Deep learning reference. Correctly excluded.                                             |
| VoxelNet (Zhou 2018)         | `Zhou2018VoxelNet`                          | Deep learning reference. Correctly excluded.                                             |
| PointRCNN (Shi 2019)         | `Shi2019PointRCNN`                          | Deep learning reference. Correctly excluded.                                             |
| Trajectron++ (Salzmann 2020) | `Salzmann2020`                              | Not implemented; future L7 work. No current gap.                                         |
| Lefèvre (2014) survey        | `Lefevre2014`                               | Survey paper; no specific algorithm to implement.                                        |
| HD Map papers                | `Pannen2020`, `Liu2020HDMap`, `Li2022HDMap` | Future L7 work. No current gap.                                                          |
| CenterPoint (Yin 2021)       | `Yin2021`                                   | Deep learning detector/tracker. Correctly excluded.                                      |
| PolarStream (Chiu 2021)      | `Chiu2021PolarStream`                       | Multi-modal fusion. Future work.                                                         |
| Waymo (Sun 2020)             | `Sun2020Waymo`                              | Dataset paper; used for context only.                                                    |

---

## Prioritised Remediation Plan

### Phase 1: Tests for Known Gaps (no paper downloads needed)

All of these can be written now against the current codebase.

| Priority | ID    | Action                                                                                    | Effort |
| -------- | ----- | ----------------------------------------------------------------------------------------- | ------ |
| P1       | K1    | Test diagonal-Q versus full-Q process noise; document sensitivity                         | M      |
| P1       | K2    | Implement Joseph-form covariance update as option; test symmetry                          | M      |
| P1       | B1    | Test MAD-versus-σ convergence; document the relationship                                  | S      |
| P1       | M1    | Implement MOTA/MOTP computation in [l8analytics](../../internal/lidar/l8analytics/doc.go) | L      |
| P2       | D2    | Test MinPts self-inclusion semantics                                                      | S      |
| P2       | D3    | Test near-merge cluster separation                                                        | S      |
| P2       | S2/S3 | Test confirmed-versus-tentative association priority                                      | M      |
| P2       | P3    | Test end-to-end heading disambiguation chain                                              | M      |
| P2       | C2    | Test classification boundary conditions                                                   | S      |
| P2       | V1    | Test coasting prediction error growth                                                     | M      |
| P3       | K3    | Instrument covariance symmetry monitoring                                                 | S      |
| P3       | K4    | Test identity preservation for adjacent unlike-class objects                              | M      |
| P3       | B3    | Test bimodal background cell behaviour                                                    | S      |
| P3       | B4    | Test reacquisition boost convergence speed                                                | S      |
| P3       | B5    | Test locked baseline transit resistance                                                   | S      |
| P3       | H1    | Test extreme cost range numerical stability                                               | S      |
| P3       | H2    | Test all-forbidden assignment matrix                                                      | S      |
| P3       | M3    | Test temporal IoU edge cases                                                              | S      |
| P3       | G1    | Test sloped-road height-band failure modes                                                | S      |
| P3       | HW1   | Test noise model against sensor spec                                                      | S      |
| P3       | P1    | Test degenerate identical-point OBB                                                       | S      |

**Effort key:** S = small (< 1 hour), M = medium (1–4 hours), L = large (4+ hours)

### Phase 2: Documentation Improvements (no paper downloads needed)

| ID  | Action                                                                                                         |
| --- | -------------------------------------------------------------------------------------------------------------- |
| B2  | Add effective EMA window formula to [background-grid-settling-maths.md](background-grid-settling-maths.md)     |
| B1  | Document MAD-to-σ relationship in [background-grid-settling-maths.md](background-grid-settling-maths.md)       |
| V2  | Derive effective observation window from `ProcessNoiseVel`; document in [tracking-maths.md](tracking-maths.md) |
| C1  | Create nuScenes/KITTI class mapping table in [classification-maths.md](classification-maths.md)                |
| HW2 | Document beam pattern non-uniformity in hardware notes                                                         |

### Phase 3: Structural Improvements (may need paper downloads)

| Priority | ID    | Action                                                                         | Blocked on                   |
| -------- | ----- | ------------------------------------------------------------------------------ | ---------------------------- |
| P1       | K1    | Implement full off-diagonal process noise Q                                    | Kalman (1960) for validation |
| P1       | S2/S3 | Implement cascaded association (confirmed tracks first)                        | —                            |
| P2       | M1/M2 | Full MOTA/MOTP/HOTA evaluation pipeline                                        | Bernardin (2008)             |
| P2       | G1    | Evaluate Patchwork++ for slope-aware ground removal (paper already downloaded) | —                            |
| P3       | K4/S1 | Add OBB IoU as secondary association cost                                      | —                            |

### Phase 4: Post-Download Review (blocked on missing papers)

When the following papers are obtained via institutional access, re-run this analysis for the corresponding subsystem. BibTeX keys reference [references.bib](references.bib).

| Paper                        | Key                        | Subsystem affected                                                 |
| ---------------------------- | -------------------------- | ------------------------------------------------------------------ |
| Kalman (1960)                | `Kalman1960`               | L5 Kalman — verify notation and noise model assumptions            |
| Welford (1962)               | `Welford1962`              | L3 background — verify the MAD deviation from exact Welford        |
| Kuhn (1955) / Munkres (1957) | `Kuhn1955` / `Munkres1957` | L5 Hungarian — verify algorithmic equivalence                      |
| Stauffer & Grimson (1999)    | `Stauffer1999`             | L3 background — evaluate multi-modal GMM benefit                   |
| Blom & Bar-Shalom (1988)     | `Blom1988`                 | L5 IMM — foundation for manoeuvring target model                   |
| Julier & Uhlman (1997)       | `Julier1997`               | L5 UKF — needed if polar measurements are used directly            |
| Bernardin (2008)             | `Bernardin2008`            | L8 analytics — CLEAR MOT metric definitions                        |
| Campello (2013)              | `Campello2013`             | L4 clustering — HDBSCAN for variable-density scenes                |
| Schubert (2017)              | `Schubert2017DBSCANR`      | L4 clustering — DBSCAN parameter selection                         |
| Jolliffe (2002)              | `Jolliffe2002`             | L4 PCA — stability for degenerate cases                            |
| Mahalanobis (1936)           | `Mahalanobis1936`          | L5 gating — verify distance definition                             |
| Fischler (1981)              | `Fischler1981`             | L4 ground — RANSAC for robust plane fitting                        |
| Lim (2021)                   | `Lim2021`                  | L4 ground — original Patchwork (versus our downloaded Patchwork++) |
| Li (2003) IMM survey         | `Li2003IMMSurvey`          | L5 IMM — comprehensive survey of multi-model approaches            |

---

## Cross-Reference to Existing Proposals

Several gaps identified above are already captured in the proposal queue. Where a proposal exists, prefer extending it rather than opening separate work.

| Gap                             | Existing proposal                                                                                    | Status                       |
| ------------------------------- | ---------------------------------------------------------------------------------------------------- | ---------------------------- |
| S1, K4 (IoU-based association)  | [20260222-geometry-coherent-tracking.md](proposals/20260222-geometry-coherent-tracking.md)           | Proposal                     |
| B3 (multi-modal background)     | [20260219-unify-l3-l4-settling.md](proposals/20260219-unify-l3-l4-settling.md)                       | Proposal                     |
| G1 (slope-aware ground removal) | [20260221-ground-plane-vector-scene-maths.md](proposals/20260221-ground-plane-vector-scene-maths.md) | Proposal                     |
| P3 (heading disambiguation)     | [20260222-obb-heading-stability-review.md](proposals/20260222-obb-heading-stability-review.md)       | Maintenance / partially done |
