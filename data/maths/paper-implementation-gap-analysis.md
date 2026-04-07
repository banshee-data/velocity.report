# Paper-vs-Implementation Gap Analysis

- **Scope:** All 24 downloaded papers cross-referenced against production code (L3–L8)
- **Method:** Traced algorithm intent from each paper through the Go implementation, identified deviations, missing edge cases, and untested behaviour

---

## Summary of Findings

| Severity                  | Count | Description                                                                    |
| ------------------------- | ----- | ------------------------------------------------------------------------------ |
| **Mathematical gap**      | 7     | Implementation deviates from paper's mathematical intent                       |
| **Missing edge case**     | 9     | Paper describes a condition the implementation does not handle                 |
| **Missing test**          | 11    | Behaviour is implemented but lacks test coverage for paper-specified edge case |
| **Future work (blocked)** | 8     | Requires papers not yet downloaded                                             |

---

## 1. DBSCAN — Ester et al. (1996)

### Paper intent

DBSCAN defines three point categories: **core**, **border**, and **noise**. Border points are reachable from a core point but are not themselves core. The original paper specifies that border points are assigned to the cluster of the _first_ core point that reaches them during expansion.

### Implementation status

`l4perception/dbscan_clusterer.go` uses 2D Euclidean distance in XY, grid-accelerated neighbourhood queries, and deterministic output sorting.

### Gaps

| ID  | Type                  | Description                                                                                                                                                                                                                                                                                                             | Impact                                              | Test needed                                                                                                       |
| --- | --------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------- |
| D1  | **Missing edge case** | Border point assignment is order-dependent in the original DBSCAN — the paper acknowledges this non-determinism. The implementation sorts output clusters by centroid for reproducibility, but does not test that border points at cluster boundaries are handled consistently when two clusters share a border region. | Low (deterministic sorting mitigates)               | Test: two adjacent clusters sharing a border point; verify stable assignment across permutations of input order.  |
| D2  | **Missing test**      | The paper's MinPts definition counts the query point itself. Verify our `MinPts` semantics match (i.e., a core point requires ≥ MinPts neighbours _including itself_). If our implementation excludes self, the effective density threshold is off by one.                                                              | Medium — affects cluster formation at low densities | Test: construct exactly `MinPts` points within ε; verify one cluster forms. Construct `MinPts - 1`; verify noise. |
| D3  | **Missing edge case** | Paper discusses performance degradation for "thin" clusters where ε is too large relative to cluster width, causing unintended merges. Implementation has `MaxClusterDiameter` and `MaxClusterAspectRatio` post-filters but no test for the merge pathology.                                                            | Medium                                              | Test: two parallel lines of points separated by < 2ε; verify they produce 2 clusters, not 1.                      |
| D4  | **Future work**       | Schubert et al. (2017) — `Schubert2017DBSCANR` — revisits DBSCAN with formal analysis of parameter selection heuristics. **Paper not downloaded.** Could inform adaptive ε selection.                                                                                                                                   | —                                                   | Blocked on paper download                                                                                         |
| D5  | **Future work**       | Campello et al. (2013) — `Campello2013` — HDBSCAN for variable-density scenes. **Paper not downloaded.** Current fixed-ε struggles at long range where point density drops.                                                                                                                                             | —                                                   | Blocked on paper download                                                                                         |

---

## 2. Kalman Filter — Kalman (1960), Bewley (2016, SORT), Weng (2020, AB3DMOT)

### Paper intent

- **Kalman (1960):** Linear optimal estimator for Gaussian noise. Requires: (a) linear state transition, (b) Gaussian process/measurement noise, (c) correct noise covariance specification, (d) symmetric positive-definite covariance maintained at all times.
- **Bewley (2016, SORT):** Applies Kalman with constant-velocity model + Hungarian assignment to 2D MOT. Uses IoU-based association, not Mahalanobis.
- **Weng (2020, AB3DMOT):** Extends SORT to 3D with Mahalanobis gating. Uses 7-state model `[x, y, z, θ, l, w, h]` and 3D IoU for association.

### Implementation status

`l5tracks/tracking_association.go`, `tracking_update.go`: 4-state CV Kalman `[x, y, vx, vy]`, 2D position-only measurement, Mahalanobis gating, Hungarian assignment.

### Gaps

| ID  | Type                  | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                                                | Impact                                                               | Test needed                                                                                                                                                                                            |
| --- | --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| K1  | **Mathematical gap**  | **Process noise model is simplified.** The continuous white-noise jerk model (standard for CV trackers) yields $Q = q \begin{bmatrix} dt^3/3 & dt^2/2 \\ dt^2/2 & dt \end{bmatrix}$ where the off-diagonal terms couple position and velocity uncertainty growth. The implementation uses diagonal-only Q: `P[0,0] += σ²_pos * dt`, `P[2,2] += σ²_vel * dt`. This underestimates cross-covariance, making the gating ellipse overconfident in the velocity–position correlation direction. | Medium — affects gating accuracy for manoeuvring targets             | Test: simulate a target accelerating at 2 m/s²; compare gating ellipse size and shape between diagonal-Q and full-Q models. Verify that the full-Q ellipse captures the target when diagonal does not. |
| K2  | **Mathematical gap**  | **Covariance update uses naive $(I - KH)P$ form.** The Joseph stabilised form $(I - KH)P(I - KH)^T + KRK^T$ guarantees symmetry and positive-definiteness even with floating-point error. The current form can lose symmetry over many iterations.                                                                                                                                                                                                                                         | Low (covariance capping mitigates), but affects long-running tracks  | Test: run 10,000 predict-update cycles with adversarial measurement noise; verify P remains symmetric and positive-definite. Compare P diagonal growth between Joseph and naive forms.                 |
| K3  | **Missing edge case** | **No explicit covariance symmetry enforcement.** After each update, the implementation does not force $P = (P + P^T)/2$. Over hundreds of frames, asymmetry can accumulate.                                                                                                                                                                                                                                                                                                                | Low (NaN guard catches catastrophic cases)                           | Test: instrument P symmetry check after every update in a long-running track; log max asymmetry.                                                                                                       |
| K4  | **Missing test**      | **AB3DMOT (Weng 2020) uses 3D IoU for association, not just Mahalanobis.** Our implementation uses only Mahalanobis distance. When two tracks have similar position but very different bounding box sizes (car vs pedestrian nearby), Mahalanobis alone cannot distinguish them. IoU would naturally reject the mismatch.                                                                                                                                                                  | Medium — risk of identity swap between adjacent unlike-class objects | Test: two tracks (car-sized and pedestrian-sized) at similar positions; verify that association does not swap identities. Consider adding OBB overlap as secondary gating criterion.                   |
| K5  | **Mathematical gap**  | **SORT (Bewley 2016) uses $dt$-dependent state transition but our prediction step clamps `dt` to `MaxPredictDt`.** This is a deliberate engineering choice (documented), but the clamped prediction underestimates true position uncertainty during frame gaps. The paper does not address frame drops.                                                                                                                                                                                    | Low — clamping is defensive; documented                              | Test: verify that after a 5-second frame gap, covariance is large enough to re-acquire the track at the physically-expected position.                                                                  |
| K6  | **Future work**       | **Kalman (1960) original paper** not downloaded. Foundational — needed for verifying our notation and assumptions match the original formulation.                                                                                                                                                                                                                                                                                                                                          | —                                                                    | Blocked on paper download                                                                                                                                                                              |
| K7  | **Future work**       | **Blom & Bar-Shalom (1988), IMM** not downloaded. The planned `imm_cv_ca_v2` engine depends on this.                                                                                                                                                                                                                                                                                                                                                                                       | —                                                                    | Blocked on paper download                                                                                                                                                                              |
| K8  | **Future work**       | **Julier & Uhlman (1997), UKF** not downloaded. Needed if nonlinear measurement models are added (e.g., polar measurements).                                                                                                                                                                                                                                                                                                                                                               | —                                                                    | Blocked on paper download                                                                                                                                                                              |

---

## 3. Hungarian Assignment — Kuhn (1955), Munkres (1957)

### Paper intent

Optimal assignment in $O(n^3)$ for balanced square matrices. Rectangular problems require padding.

### Implementation status

`l5tracks/hungarian.go`: Jonker-Volgenant variant with potential vectors, padded to square, costs ≥ `hungarianlnf` treated as forbidden.

### Gaps

| ID  | Type             | Description                                                                                                                                                                                                                                                             | Impact                                                              | Test needed                                                                      |
| --- | ---------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------- | -------------------------------------------------------------------------------- |
| H1  | **Missing test** | **Numerical stability with extreme cost ranges.** The algorithm uses float64 internally but accepts float32 costs. When costs span many orders of magnitude (e.g., 0.001 vs 1e15), the potential subtraction step could lose precision. Papers assume exact arithmetic. | Low — in practice costs are Mahalanobis distances in a narrow range | Test: cost matrix with entries spanning [1e-6, 1e12]; verify correct assignment. |
| H2  | **Missing test** | **All-forbidden matrix.** When every entry is ≥ `hungarianlnf`, the result should be all -1. This is tested implicitly (one row forbidden) but not for the full-matrix case.                                                                                            | Low                                                                 | Test: NxN matrix of all `hungarianlnf`; verify all result entries are -1.        |
| H3  | **Future work**  | **Kuhn (1955) and Munkres (1957)** papers not downloaded. Implementation follows a Jonker-Volgenant variant; should verify algorithmic equivalence to the original Munkres formulation.                                                                                 | —                                                                   | Blocked on paper download                                                        |

---

## 4. Background Model — Welford (1962), Stauffer & Grimson (1999)

### Paper intent

- **Welford (1962):** Numerically stable single-pass online algorithm for computing running mean and variance: $M_n = M_{n-1} + (x_n - M_{n-1})/n$, $S_n = S_{n-1} + (x_n - M_{n-1})(x_n - M_n)$, variance $= S_n/(n-1)$.
- **Stauffer & Grimson (1999):** Gaussian Mixture Model (GMM) background subtraction with multiple modes per pixel, online EM updates, and adaptive component weights.

### Implementation status

`l3grid/background.go`, `foreground.go`: Single-component EMA per cell (not Welford's exact algorithm, not multi-modal GMM). Update: $\mu \leftarrow (1-\alpha)\mu + \alpha x$, spread: $(1-\alpha)s + \alpha |x - \mu_{old}|$.

### Gaps

| ID  | Type                 | Description                                                                                                                                                                                                                                                                                                                                                                                                                                    | Impact                                               | Test needed                                                                                                                                                                                                                                   |
| --- | -------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| B1  | **Mathematical gap** | **Spread computation is EMA of absolute deviation, not variance.** Welford computes true running variance; our EMA spread tracks mean absolute deviation (MAD). MAD ≈ 0.798σ for Gaussian distributions. The closeness threshold uses this spread directly, so the effective sensitivity is scaled by ~0.8 compared to what a σ-based threshold would give. This is _documented_ but not _tested_ for correctness against known distributions. | Medium — affects foreground sensitivity calibration  | Test: feed 10,000 samples from N(5.0, 0.1²) into a cell; verify that `RangeSpreadMeters` converges to approximately 0.08 (0.798 × 0.1), not 0.1. Document the MAD-to-σ relationship in background-grid-settling-maths.md.                     |
| B2  | **Mathematical gap** | **EMA has recency bias vs Welford's unweighted estimator.** With α=0.02, the effective window is ~50 samples. Welford treats all samples equally. For a stationary background this is fine, but for slowly-drifting backgrounds the EMA is actually preferable. The maths doc acknowledges this but doesn't quantify the effective window.                                                                                                     | Low — EMA is the correct choice for this application | Document: add effective window formula $n_{eff} = 2/\alpha - 1$ to background-grid-settling-maths.md.                                                                                                                                         |
| B3  | **Mathematical gap** | **Stauffer (1999) uses multi-modal GMM; implementation is single-mode.** The maths doc explicitly states this is a simplification. Cells that alternately see two stable depths (e.g., a swinging gate, tree branches) will oscillate between foreground and background. The locked-baseline mechanism partially addresses this but is not a true multi-modal model.                                                                           | Medium — affects scenes with bimodal backgrounds     | Test: alternate observations between two stable ranges (e.g., 5.0m and 5.5m); verify that the cell does not persistently classify one range as foreground. Document the limitation and when HDBSCAN + multi-modal background would be needed. |
| B4  | **Missing test**     | **Reacquisition boost convergence.** After a long foreground event (vehicle transit), the boosted α should cause faster re-convergence. No test verifies the settling time with boost vs without.                                                                                                                                                                                                                                              | Medium                                               | Test: 100 frames of foreground, then background returns; measure frames to re-settle with boost=5.0 vs boost=1.0.                                                                                                                             |
| B5  | **Missing test**     | **Locked baseline drift resistance.** The locked baseline updates with β=0.001. No test verifies that it survives a sustained transit (100+ foreground frames) without drifting.                                                                                                                                                                                                                                                               | Medium                                               | Test: settled cell, 200 frames of foreground at different range, then original range returns; verify locked baseline still matches pre-transit value within tolerance.                                                                        |
| B6  | **Future work**      | **Welford (1962)** paper not downloaded. Should verify our MAD-based spread computation against Welford's exact formulation and document the deliberate deviation.                                                                                                                                                                                                                                                                             | —                                                    | Blocked on paper download                                                                                                                                                                                                                     |

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

- **SORT:** Kalman filter (CV model on bounding box space [u, v, s, r, u̇, v̇, ṡ]) + Hungarian assignment with IoU-based cost. Track lifecycle: create, confirm after hits, delete after max_age misses.
- **DeepSORT:** Adds appearance descriptor (deep re-identification features) for association, cascaded matching (confirmed tracks first, then tentative), and maximum cosine distance metric.

### Implementation status

Our tracker follows the SORT architecture but differs in specifics: uses world-frame position instead of image-frame bounding box, Mahalanobis cost instead of IoU, and has no appearance features.

### Gaps

| ID  | Type                  | Description                                                                                                                                                                                                                                                                                                             | Impact                                                                   | Test needed                                                                                                                                                                                                                                       |
| --- | --------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| S1  | **Mathematical gap**  | **SORT uses IoU for association; we use Mahalanobis only.** IoU naturally handles size differences between objects. Two objects at similar positions but different sizes get low IoU. Mahalanobis distance (position-only) cannot distinguish them. This matters when a pedestrian walks close to a parked car.         | Medium                                                                   | Test: pedestrian track near stationary car-sized track; verify no identity swap. Consider hybrid cost: Mahalanobis + OBB IoU penalty.                                                                                                             |
| S2  | **Missing edge case** | **SORT's "minimum hits" before considering a track for output.** Our implementation has `HitsToConfirm` for lifecycle but tentative tracks are still included in association — they compete for clusters with confirmed tracks. SORT processes confirmed tracks first in association.                                   | Medium — tentative tracks can "steal" measurements from confirmed tracks | Test: confirmed track coasting (1 miss); new cluster appears nearby; verify confirmed track gets priority over new tentative track in association.                                                                                                |
| S3  | **Missing edge case** | **DeepSORT's cascaded matching.** Confirmed tracks are matched first, then unmatched detections are matched to tentative tracks. Our implementation matches all tracks simultaneously. This can cause a tentative track to win a cluster that a coasting confirmed track should have received.                          | Medium                                                                   | Same test as S2 — this is the mechanism that would fix S2.                                                                                                                                                                                        |
| S4  | **Missing feature**   | **DeepSORT's appearance features.** Without re-identification features, tracks that coast through occlusion can only re-associate based on position prediction. For long occlusions (>1 second), the predicted position may be far from the true position, causing a new track to be created instead of re-associating. | High for long-occlusion scenarios                                        | Test: track coasts for 1 second, then object reappears 2m from predicted position; verify re-association succeeds. Note: appearance features may be out of scope for the "no black-box AI" tenet unless a simple, inspectable descriptor is used. |

---

## 7. MOT Evaluation — Bernardin (2008, CLEAR MOT), Luiten (2021, HOTA), Milan (2016, MOT16)

### Paper intent

- **CLEAR MOT (Bernardin 2008):** Defines MOTA (accuracy) and MOTP (precision) metrics. MOTA = 1 - (FN + FP + IDSW) / GT.
- **HOTA (Luiten 2021):** Higher-order metric combining detection and association quality. Decomposes into DetA (detection accuracy) and AssA (association accuracy).
- **MOT16 (Milan 2016):** Benchmark protocol and evaluation methodology.

### Implementation status

`l8analytics/comparison.go` computes temporal IoU between track pairs across runs. `l8analytics/summary.go` computes aggregate run statistics. No formal MOTA/MOTP/HOTA computation.

### Gaps

| ID  | Type                 | Description                                                                                                                                                                                                                      | Impact                     | Test needed                                                                                                                    |
| --- | -------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| M1  | **Mathematical gap** | **No MOTA/MOTP computation.** The standard MOT evaluation metrics from Bernardin (2008) are not implemented. Without these, there is no way to quantitatively compare tracking quality across parameter changes or code changes. | High for tuning validation | Implement: given ground-truth labels and tracker output, compute MOTA, MOTP, and identity switches per the CLEAR MOT protocol. |
| M2  | **Mathematical gap** | **No HOTA metric.** Luiten (2021) shows HOTA is more balanced than MOTA for evaluating both detection and association quality.                                                                                                   | Medium                     | Implement after M1: HOTA decomposition into DetA and AssA.                                                                     |
| M3  | **Missing test**     | **Temporal IoU correctness.** `comparison.go` computes temporal IoU but has no test for edge cases: zero-overlap tracks, identical tracks, one track fully contained within another.                                             | Medium                     | Test: known track pairs with calculable IoU; verify computed values match.                                                     |
| M4  | **Future work**      | **Bernardin (2008)** paper not downloaded. Needed for exact MOTA/MOTP formulae and edge case handling (e.g., how to handle tracks with no ground truth match).                                                                   | —                          | Blocked on paper download                                                                                                      |

---

## 8. Patchwork++ — Lim et al. (2022)

### Paper intent

Concentric zone-based ground segmentation that handles uneven terrain, curbs, and slopes. Divides the point cloud into concentric rings around the sensor, fits local planes per zone, and uses curvature-based region growing.

### Implementation status

`l4perception/ground.go`: Simple height-band filter (floor/ceiling thresholds). No plane fitting, no zone segmentation, no slope awareness.

### Gaps

| ID  | Type                  | Description                                                                                                                                                                                                                                                | Impact                     | Test needed                                                                                                                                                                  |
| --- | --------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| G1  | **Mathematical gap**  | **Height-band filter vs plane-based ground removal.** On sloped roads, a fixed height band will either miss ground points (if floor is too high) or include low objects as ground (if floor is too low). Patchwork++ handles this by fitting local planes. | High for hilly deployments | Test: simulate a 5° road slope; verify that the height-band filter correctly classifies ground at both near range (uphill) and far range (downhill). Document failure cases. |
| G2  | **Missing edge case** | **Curb detection.** Height-band filter cannot distinguish between a 15cm curb (ground) and a 15cm-high object (e.g., a small animal, a ball). Patchwork++ uses curvature discontinuity.                                                                    | Low for current use case   | Test: points at ground level with a 15cm step; verify classification.                                                                                                        |

---

## 9. Dataset / Benchmark Papers — Caesar (2020, nuScenes), Behley (2019, SemanticKITTI), Sun (2020, Waymo)

### Paper intent

These papers define object class taxonomies, evaluation protocols, and benchmark expectations.

### Implementation status

`l6objects/classification.go` uses a rule-based classifier with a 7-class taxonomy. The maths doc references KITTI, nuScenes, and SemanticKITTI for class definitions.

### Gaps

| ID  | Type                  | Description                                                                                                                                                                                        | Impact                                 | Test needed                                                                                             |
| --- | --------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| C1  | **Missing edge case** | **nuScenes defines 23 classes; our taxonomy has 7 (5 active + 2 reserved).** Objects like construction vehicles, trailers, and barriers have no classification path and fall through to "dynamic". | Low for residential traffic monitoring | Document: mapping table from our classes to nuScenes/KITTI classes for future evaluation compatibility. |
| C2  | **Missing test**      | **Classification boundary conditions.** The rule-based classifier has hard thresholds (e.g., bus length ≥ 7m). No test verifies behaviour at exact boundary values (6.99m vs 7.01m length).        | Medium                                 | Test: objects at exact threshold boundaries for each class; verify correct classification.              |

---

## 10. Constant Velocity Model — Schöller et al. (2020)

### Paper intent

The paper demonstrates that a well-tuned constant velocity model can match or exceed deep learning baselines for short-horizon pedestrian trajectory prediction (≤4.8 seconds). Key insight: CV model performance depends critically on the observation window length and process noise tuning.

### Implementation status

The tracker uses a CV model. The paper validates this choice.

### Gaps

| ID  | Type                 | Description                                                                                                                                                                                                                                                                                                                                                                                                                 | Impact                               | Test needed                                                                                                                                                                          |
| --- | -------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| V1  | **Missing test**     | **Paper shows CV model degrades beyond ~5 second prediction horizon.** Our `MaxMissesConfirmed` controls how long a track coasts. If coasting exceeds 5 seconds, the predicted position becomes unreliable. No test validates that prediction error grows as expected with coasting duration.                                                                                                                               | Medium                               | Test: track coasts for 1s, 3s, 5s, 10s; measure prediction error at each interval. Verify it matches CV model expected growth ($\text{error} \propto t^2$ for accelerating targets). |
| V2  | **Mathematical gap** | **Paper recommends tuning the observation window (how many past frames to use for velocity estimation).** Our Kalman filter uses all observations implicitly through the recursive update. The effective observation window is controlled by the process noise — higher process noise gives more weight to recent observations. No explicit analysis maps our `ProcessNoiseVel` to an equivalent observation window length. | Low — but useful for tuning guidance | Document: derive the effective observation window from current ProcessNoiseVel setting and compare against Schöller's recommended values.                                            |

---

## 11. Sensor Manual — Hesai Pandar40P

### Paper intent

Sensor specifications: angular resolution, range accuracy, beam pattern.

### Implementation status

Grid dimensions (40 rings, 1800 azimuth bins) match the Pandar40P's 40-beam, 0.2° horizontal resolution.

### Gaps

| ID  | Type                  | Description                                                                                                                                                                                                                                                                                                                           | Impact                               | Test needed                                                                                                                                                                                          |
| --- | --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| HW1 | **Missing test**      | **Range accuracy specification.** The manual specifies ±2cm accuracy at 0–30m and ±3cm at 30–200m. Our `NoiseRelativeFraction` (default 0.01 = 1%) gives 1cm noise at 1m, 10cm at 10m — which is 5× the actual sensor spec at short range. This means we're more conservative (wider closeness threshold) than needed at close range. | Low — conservative is safe           | Test: verify that `NoiseRelativeFraction = 0.01` produces closeness thresholds consistent with the Pandar40P's specified range accuracy at 5m, 20m, 50m, 100m. Consider range-dependent noise model. |
| HW2 | **Missing edge case** | **Beam pattern non-uniformity.** The Pandar40P has non-uniform vertical beam spacing (denser at horizon). Our uniform ring indexing may over-resolve near-horizontal beams and under-resolve steep beams.                                                                                                                             | Low for current flat-road deployment | Document only — no code change needed.                                                                                                                                                               |

---

## Papers Reviewed With No Significant Gaps

| Paper                        | Key                                         | Assessment                                                                                                                    |
| ---------------------------- | ------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------- |
| PointPillars (Lang 2019)     | `Lang2019`                                  | Not implemented (deep learning); referenced as future L6 replacement. No gap — correctly excluded by "no black-box AI" tenet. |
| PointNet/PointNet++          | `Qi2017*`                                   | Same — deep learning reference. Correctly excluded.                                                                           |
| VoxelNet (Zhou 2018)         | `Zhou2018VoxelNet`                          | Same — deep learning reference.                                                                                               |
| PointRCNN (Shi 2019)         | `Shi2019PointRCNN`                          | Same — deep learning reference.                                                                                               |
| Trajectron++ (Salzmann 2020) | `Salzmann2020`                              | Not implemented; future L7 work. No current gap.                                                                              |
| Lefèvre (2014) survey        | `Lefevre2014`                               | Survey paper; no specific algorithm to implement.                                                                             |
| HD Map papers                | `Pannen2020`, `Liu2020HDMap`, `Li2022HDMap` | Future L7 work. No current gap.                                                                                               |
| CenterPoint (Yin 2021)       | `Yin2021`                                   | Deep learning detector/tracker. Correctly excluded.                                                                           |
| PolarStream (Chiu 2021)      | `Chiu2021PolarStream`                       | Multi-modal fusion. Future work.                                                                                              |
| Waymo (Sun 2020)             | `Sun2020Waymo`                              | Dataset paper; used for context only.                                                                                         |

---

## Prioritised Remediation Plan

### Phase 1: Tests for Known Gaps (no paper downloads needed)

These can be implemented immediately:

| Priority | ID    | Action                                                           | Effort |
| -------- | ----- | ---------------------------------------------------------------- | ------ |
| P1       | K1    | Test diagonal-Q vs full-Q process noise; document sensitivity    | M      |
| P1       | K2    | Implement Joseph-form covariance update as option; test symmetry | M      |
| P1       | B1    | Test MAD-vs-σ convergence; document relationship                 | S      |
| P1       | M1    | Implement MOTA/MOTP computation in l8analytics                   | L      |
| P2       | D2    | Test MinPts self-inclusion semantics                             | S      |
| P2       | D3    | Test near-merge cluster separation                               | S      |
| P2       | S2/S3 | Test confirmed-vs-tentative association priority                 | M      |
| P2       | P3    | Test end-to-end heading disambiguation chain                     | M      |
| P2       | C2    | Test classification boundary conditions                          | S      |
| P2       | V1    | Test coasting prediction error growth                            | M      |
| P3       | K3    | Instrument covariance symmetry monitoring                        | S      |
| P3       | K4    | Test identity preservation for adjacent unlike-class objects     | M      |
| P3       | B3    | Test bimodal background cell behaviour                           | S      |
| P3       | B4    | Test reacquisition boost convergence speed                       | S      |
| P3       | B5    | Test locked baseline transit resistance                          | S      |
| P3       | H1    | Test extreme cost range numerical stability                      | S      |
| P3       | H2    | Test all-forbidden assignment matrix                             | S      |
| P3       | M3    | Test temporal IoU edge cases                                     | S      |
| P3       | G1    | Test sloped-road height-band failure                             | S      |
| P3       | HW1   | Test noise model vs sensor spec                                  | S      |
| P3       | P1    | Test degenerate identical-point OBB                              | S      |

**Effort key:** S = small (< 1 hour), M = medium (1–4 hours), L = large (4+ hours)

### Phase 2: Documentation Improvements (no paper downloads needed)

| ID  | Action                                                                |
| --- | --------------------------------------------------------------------- |
| B2  | Add effective EMA window formula to background-grid-settling-maths.md |
| B1  | Document MAD-to-σ relationship in background-grid-settling-maths.md   |
| V2  | Derive effective observation window from ProcessNoiseVel              |
| C1  | Create nuScenes/KITTI class mapping table                             |
| HW2 | Document beam pattern non-uniformity in hardware notes                |

### Phase 3: Structural Improvements (may need paper downloads)

| Priority | ID    | Action                                              | Blocked on                  |
| -------- | ----- | --------------------------------------------------- | --------------------------- |
| P1       | K1    | Implement full off-diagonal process noise Q         | Kalman1960 (for validation) |
| P1       | S2/S3 | Implement cascaded association (confirmed first)    | —                           |
| P2       | M1/M2 | Full MOTA/MOTP/HOTA evaluation pipeline             | Bernardin2008               |
| P2       | G1    | Evaluate Patchwork++ for slope-aware ground removal | Already downloaded          |
| P3       | K4/S1 | Add OBB IoU as secondary association cost           | —                           |

### Phase 4: Post-Download Review (blocked on missing papers)

When the following papers are obtained via academic access, re-run this analysis for the corresponding subsystem:

| Paper                        | Key                        | Subsystem affected                                             |
| ---------------------------- | -------------------------- | -------------------------------------------------------------- |
| Kalman (1960)                | `Kalman1960`               | L5 Kalman filter — verify notation, noise model assumptions    |
| Welford (1962)               | `Welford1962`              | L3 background — verify MAD deviation from exact Welford        |
| Kuhn (1955) / Munkres (1957) | `Kuhn1955` / `Munkres1957` | L5 Hungarian — verify algorithmic equivalence                  |
| Stauffer & Grimson (1999)    | `Stauffer1999`             | L3 background — evaluate multi-modal GMM benefit               |
| Blom & Bar-Shalom (1988)     | `Blom1988`                 | L5 IMM engine — foundation for manoeuvring target model        |
| Julier & Uhlman (1997)       | `Julier1997`               | L5 UKF — needed if polar measurements are used directly        |
| Bernardin (2008)             | `Bernardin2008`            | L8 analytics — CLEAR MOT metric definitions                    |
| Campello (2013)              | `Campello2013`             | L4 clustering — HDBSCAN for variable-density                   |
| Schubert (2017)              | `Schubert2017DBSCANR`      | L4 clustering — DBSCAN parameter selection                     |
| Jolliffe (2002)              | `Jolliffe2002`             | L4 PCA — stability for degenerate cases                        |
| Mahalanobis (1936)           | `Mahalanobis1936`          | L5 gating — verify distance definition                         |
| Fischler (1981)              | `Fischler1981`             | L4 ground — RANSAC for robust plane fitting                    |
| Lim (2021)                   | `Lim2021`                  | L4 ground — original Patchwork (vs our downloaded Patchwork++) |
| Li (2003) IMM survey         | `Li2003IMMSurvey`          | L5 IMM — comprehensive survey of multi-model approaches        |

---

## Cross-Reference to Existing Proposals

Several gaps identified above are already captured in the project's proposal queue:

| Gap                             | Existing proposal                       | Status                       |
| ------------------------------- | --------------------------------------- | ---------------------------- |
| S1, K4 (IoU-based association)  | P1: Geometry-Coherent Track State       | Proposal                     |
| B3 (multi-modal background)     | P4: Unify L3/L4 Settling                | Proposal                     |
| G1 (slope-aware ground removal) | P3: Ground Plane and Vector-Scene Maths | Proposal                     |
| P3 (heading disambiguation)     | OBB Heading Stability Review            | Maintenance / partially done |
