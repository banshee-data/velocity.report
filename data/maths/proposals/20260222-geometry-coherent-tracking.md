# Geometry-Coherent Track State for Stable Bounding Boxes

- **Status:** Proposal Math (Not Active in Current Runtime)
- **Author:** velocity.report contributors
- **Created:** 2026-02-22
- **Related:**

- [OBB heading stability review](20260222-obb-heading-stability-review.md) — current guards
- [Velocity-coherent foreground extraction](20260220-velocity-coherent-foreground-extraction.md) — upstream per-point velocity
- [Ground plane and vector-scene maths](20260221-ground-plane-vector-scene-maths.md) — ground model

---

## 1. Problem Statement

The current LiDAR tracking system computes oriented bounding boxes (OBB) from Principal Component Analysis (PCA) on each frame's cluster points independently (`l4perception/obb.go`). To address instability, four guards are applied at the tracker level (`l5tracks/tracking.go`):

1. **Guard 1:** Minimum point count threshold
2. **Guard 2:** Aspect-ratio lock threshold
3. **Guard 3:** 90° jump rejection
4. **Guard 4:** Exponential moving average (EMA) heading smoothing

Despite these guards, two fundamental problems persist:

### 1.1 Shape Instability

Dimensions (length, width, height) change dramatically frame-to-frame because PCA axes shift as points appear and disappear due to:

- Partial occlusion by other objects
- Sensor viewing angle changes
- Point cloud sampling noise
- Edge detection variation

A vehicle's bounding box might oscillate between 4.2 m × 1.8 m and 3.8 m × 2.1 m within seconds, even though the physical object is constant.

### 1.2 Heading Rotation

Guards reject discrete 90° jumps but allow gradual drift. Near-square clusters (pedestrians, slow-moving vehicles) cause frequent axis ambiguity:

- PCA returns the axis with greatest variance as "length"
- For aspect ratios near 1:1, noise determines which axis is chosen
- Heading can drift continuously or flip unexpectedly
- EMA smoothing dampens but doesn't eliminate rotation

### 1.3 Root Cause

**The system has no temporal model of object geometry.** Each frame recomputes geometry from scratch. Guards are reactive patches that detect and suppress symptoms, not a coherent model that maintains geometric consistency.

The fundamental insight: **object shapes are persistent**. A vehicle doesn't change from 4.5 m to 4.0 m length in one frame. The correct model is:

- True geometry is stable (evolves slowly)
- Observations are noisy samples of true geometry
- Update beliefs about true geometry using Bayesian evidence from observations

---

## 2. Proposed Model: Geometry-Coherent Track State

We extend each `TrackedObject` with a **geometry prior** — a persistent model of the object's true shape and orientation that evolves over the track lifetime through Bayesian updates.

### 2.1 Track Geometry State

Define per-track geometry state vector $\mathbf{G}$:

$$
\mathbf{G} = \{L_{\text{est}}, W_{\text{est}}, H_{\text{est}}, \theta_{\text{est}}, \sigma_L, \sigma_W, \sigma_H, \sigma_\theta, n_{\text{obs}}, c_{\text{shape}}\}
$$

Where:

- $L_{\text{est}}, W_{\text{est}}, H_{\text{est}}$: estimated dimensions (metres)
- $\theta_{\text{est}}$: estimated heading (radians, zero = +X axis)
- $\sigma_L, \sigma_W, \sigma_H, \sigma_\theta$: uncertainty (standard deviation) for each parameter
- $n_{\text{obs}}$: cumulative observation count (for prior weight)
- $c_{\text{shape}} \in \{\text{elongated}, \text{square}, \text{unknown}\}$: inferred shape classification

### 2.2 Observation Model

Each frame produces a cluster observation $\mathbf{z}$ from DBSCAN clustering followed by PCA-based OBB estimation:

$$
\mathbf{z} = (L_{\text{obs}}, W_{\text{obs}}, H_{\text{obs}}, \theta_{\text{obs}})
$$

**Key challenge:** PCA has inherent 90° ambiguity. The principal axes have no canonical orientation — swapping "length" and "width" axes produces an equally valid OBB rotated by 90°.

Therefore, each observation has **two valid interpretations**:

$$
\begin{aligned}
\mathbf{z}_{\text{aligned}} &= (L_{\text{obs}}, W_{\text{obs}}, \theta_{\text{obs}}) && \text{PCA axes match track axes} \\
\mathbf{z}_{\text{swapped}} &= (W_{\text{obs}}, L_{\text{obs}}, \theta_{\text{obs}} + \pi/2) && \text{PCA axes are swapped}
\end{aligned}
$$

Current guards attempt to detect when the wrong interpretation was chosen. The geometry-coherent model instead **selects the correct interpretation** using the track's prior geometry.

### 2.3 Axis Selection via Likelihood Test

For each interpretation, compute a Mahalanobis-like residual measuring how well the observation matches the track's current geometry estimate:

$$
d_{\text{aligned}} = \frac{(L_{\text{obs}} - L_{\text{est}})^2}{\sigma_L^2} + \frac{(W_{\text{obs}} - W_{\text{est}})^2}{\sigma_W^2} + \frac{\Delta\theta_{\text{aligned}}^2}{\sigma_\theta^2}
$$

$$
d_{\text{swapped}} = \frac{(W_{\text{obs}} - L_{\text{est}})^2}{\sigma_L^2} + \frac{(L_{\text{obs}} - W_{\text{est}})^2}{\sigma_W^2} + \frac{\Delta\theta_{\text{swapped}}^2}{\sigma_\theta^2}
$$

Where angular distance is computed with wraparound:

$$
\Delta\theta = \min(|\theta_{\text{obs}} - \theta_{\text{est}}|, 2\pi - |\theta_{\text{obs}} - \theta_{\text{est}}|)
$$

**Selection rule:**

- If $d_{\text{aligned}} < d_{\text{swapped}}$: accept $\mathbf{z}_{\text{aligned}}$
- If $d_{\text{swapped}} < d_{\text{aligned}}$: accept $\mathbf{z}_{\text{swapped}}$
- If $\min(d_{\text{aligned}}, d_{\text{swapped}}) > d_{\text{threshold}}$: reject as outlier

The threshold $d_{\text{threshold}}$ gates outlier observations (e.g., $d_{\text{threshold}} = 6.0$ corresponds to ~2.5σ in each dimension).

### 2.4 Exponential Moving Average Update

After selecting the best interpretation $\mathbf{z}^* = (L^*, W^*, H^*, \theta^*)$, update the geometry estimate:

$$
\begin{aligned}
L_{\text{est}} &\leftarrow (1 - \alpha_{\text{dim}}) \cdot L_{\text{est}} + \alpha_{\text{dim}} \cdot L^* \\
W_{\text{est}} &\leftarrow (1 - \alpha_{\text{dim}}) \cdot W_{\text{est}} + \alpha_{\text{dim}} \cdot W^* \\
H_{\text{est}} &\leftarrow (1 - \alpha_{\text{dim}}) \cdot H_{\text{est}} + \alpha_{\text{dim}} \cdot H^* \\
\theta_{\text{est}} &\leftarrow \text{smooth\_heading}(\theta_{\text{est}}, \theta^*, \alpha_{\text{heading}})
\end{aligned}
$$

Where:

- $\alpha_{\text{dim}}$: dimension smoothing rate (slow, e.g., 0.05)
- $\alpha_{\text{heading}}$: heading smoothing rate (existing value, e.g., 0.08)
- `smooth_heading()`: existing angular EMA with wraparound handling

Dimension updates are intentionally slower than heading updates because:

- Object dimensions are physically more stable than orientation
- Dimension measurement noise is higher (depends on visible surface area)
- Rapid dimension changes likely indicate observation errors, not true geometry changes

### 2.5 Uncertainty Shrinkage with Observations

Uncertainties shrink as observations accumulate, implementing the Bayesian principle that confidence increases with evidence:

$$
\sigma_{\text{dim}}(n) = \frac{\sigma_{\text{dim,init}}}{\sqrt{\min(n, n_{\max})}}
$$

$$
\sigma_\theta(n) = \frac{\sigma_{\theta,\text{init}}}{\sqrt{\min(n, n_{\max})}}
$$

Where:

- $n = n_{\text{obs}}$: number of accepted observations
- $n_{\max}$: maximum count for uncertainty floor (e.g., 30 frames)
- $\sigma_{\text{dim,init}}$: initial dimension uncertainty (e.g., 1.0 m)
- $\sigma_{\theta,\text{init}}$: initial heading uncertainty (e.g., $\pi/2$ radians)

**Effect:**

- **New tracks** ($n \approx 1$): wide uncertainty → accept diverse observations, explore geometry space
- **Mature tracks** ($n > n_{\max}$): tight uncertainty → reject outlier frames, maintain stable geometry

This naturally handles the exploration-exploitation trade-off: early frames establish geometry; later frames refine it.

---

## 3. Heading-Motion Coupling

When velocity-coherent data is available (from the [velocity-coherent foreground extraction](20260220-velocity-coherent-foreground-extraction.md) proposal), the heading model strengthens significantly.

### 3.1 Motion-Informed Heading Prior

For tracks with estimated speed $v > v_{\min}$ (e.g., $v_{\min} = 0.5$ m/s):

The heading prior shifts toward the velocity direction:

$$
\theta_{\text{prior}} = (1 - \beta_{\text{motion}}) \cdot \theta_{\text{est}} + \beta_{\text{motion}} \cdot \theta_{\text{velocity}}
$$

Where:

- $\theta_{\text{velocity}} = \text{atan2}(v_y, v_x)$: heading from velocity vector
- $\beta_{\text{motion}}$: motion influence weight (e.g., 0.3)

The heading uncertainty narrows proportionally to speed confidence:

$$
\sigma_\theta^{\text{motion}} = \sigma_\theta \cdot \exp\left(-\frac{v}{v_{\text{scale}}}\right)
$$

Where $v_{\text{scale}}$ controls the rate of uncertainty reduction (e.g., 2.0 m/s).

**Effect:**

- **Fast-moving objects:** heading prior is tight and motion-aligned → PCA axis selection is strongly constrained
- **Slow-moving objects:** heading prior is wide → geometry-based axis selection dominates
- **Transition is continuous:** no discrete mode switches

This replaces the current Guard 1 (velocity disambiguation) with a principled Bayesian prior that naturally handles the transition:

$$
\text{stationary} \rightarrow \text{moving} \rightarrow \text{stationary}
$$

### 3.2 Stationary Object Handling

For tracks with speed $v \approx 0$ (below $v_{\min}$):

- **Heading uncertainty grows over time:** $\sigma_\theta(t) = \sigma_\theta(0) \cdot (1 + \gamma_{\text{static}} \cdot t)$
  - No motion signal to anchor heading
  - Uncertainty inflation rate $\gamma_{\text{static}}$ (e.g., 0.01 rad/frame)
  - Caps at $\sigma_{\theta,\text{max}}$ (e.g., $\pi/2$)

- **Observation weight for heading decreases:**
  - Rely on geometry shape fit instead of heading fit
  - Axis selection weighs dimension residuals more heavily
- **Dimensions continue to update normally:**
  - Shape information remains valid regardless of motion
  - Dimension observations are not affected by lack of velocity signal

This prevents stationary objects (parked vehicles, stationary pedestrians) from having unstable or meaningless heading estimates while maintaining accurate bounding box dimensions.

---

## 4. Shape Classification

After $n_{\text{classify}}$ observations (e.g., 10 frames), infer shape classification $c_{\text{shape}}$ from aspect ratio history.

### 4.1 Classification Rule

Compute aspect ratio history:

$$
r(i) = \frac{L_{\text{est}}(i)}{W_{\text{est}}(i)} \quad \text{for } i = 1, \ldots, n_{\text{classify}}
$$

$$
r_{\text{median}} = \text{median}(\{r(i)\})
$$

Classify based on median aspect ratio:

$$
c_{\text{shape}} = \begin{cases}
\text{elongated} & \text{if } r_{\text{median}} > r_{\text{elongated}} \\
\text{square} & \text{if } r_{\text{median}} < r_{\text{square}} \\
\text{unknown} & \text{otherwise}
\end{cases}
$$

Where:

- $r_{\text{elongated}} = 1.5$: threshold for vehicle-like objects
- $r_{\text{square}} = 1.3$: threshold for pedestrian-like objects
- Intermediate ratios remain unclassified (cautious approach)

### 4.2 Shape-Modulated Updates

Shape classification modulates the update rule:

**Elongated objects** (vehicles):

- Heading updates have **full weight** ($w_\theta = 1.0$)
  - Well-defined heading from geometry
  - PCA axes are reliable for aspect ratio > 1.5
  - Motion direction should align with long axis
- Dimension changes are **suspicious**
  - Large dimension changes ($> 0.5$ m in one frame) trigger logging/alerts
  - May indicate tracking errors or mis-association
  - Shape should be stable over track lifetime

**Square objects** (pedestrians):

- Heading updates have **reduced weight** ($w_\theta = 0.3$)
  - Poorly defined heading from geometry
  - PCA axis choice is dominated by noise
  - Rely more heavily on motion direction when available
- Dimension changes are **expected**
  - PCA noise naturally causes dimension variation
  - Body pose changes (arms, walking gait) affect projected shape
  - Accept wider variation without flagging

**Unknown objects:**

- Default behaviour: moderate weight ($w_\theta = 0.7$)
- Transitions to elongated/square after classification threshold

The weight $w_\theta$ multiplies the heading term in the residual calculation:

$$
d = \frac{(L_{\text{obs}} - L_{\text{est}})^2}{\sigma_L^2} + \frac{(W_{\text{obs}} - W_{\text{est}})^2}{\sigma_W^2} + w_\theta \cdot \frac{\Delta\theta^2}{\sigma_\theta^2}
$$

---

## 5. Integration with Existing Pipeline

### 5.1 Pipeline Position

```
L3 Segmentation
  └─ DBSCAN clustering
       ↓
L4 Perception (current)
  └─ EstimateOBBFromCluster() → raw per-frame OBB
       ↓
L5 Tracking (proposed changes)
  └─ updateGeometry() [NEW — replaces updateOBBHeading()]
       ├─ axis_select(): choose aligned vs swapped interpretation
       ├─ outlier_gate(): reject if both interpretations are poor fits
       ├─ ema_update(): update L_est, W_est, H_est, θ_est
       ├─ uncertainty_shrink(): reduce σ values with observations
       ├─ classify_shape(): update c_shape from aspect ratio history
       └─ motion_couple(): incorporate velocity prior (if available)
```

### 5.2 What This Replaces

The geometry-coherent model **replaces four existing guard mechanisms**:

| Current Guard                            | Replacement                                       | Lines in tracking.go |
| ---------------------------------------- | ------------------------------------------------- | -------------------- |
| **Guard 2:** Aspect-ratio lock threshold | Subsumed by axis selection + shape classification | ~950-980             |
| **Guard 3:** 90° jump rejection          | Subsumed by axis selection (lower residual wins)  | ~1000-1020           |
| Dimension sync logic                     | Unified in ema_update()                           | ~1124-1141           |
| HeadingSource enum complexity            | Simplified: aligned / swapped / rejected / motion | Various              |

**Guard 1** (minimum point count) **remains** as a data-quality gate — it prevents OBB estimation when insufficient points are available.

**Guard 4** (EMA heading smoothing) is **retained** but becomes part of the coherent model rather than a reactive patch.

### 5.3 Synergy with Velocity-Coherent Extraction

When [velocity-coherent foreground extraction](20260220-velocity-coherent-foreground-extraction.md) is active:

1. **Per-point velocities provide independent heading signal:**
   - PCA gives shape-based heading
   - Velocity gives motion-based heading
   - Axis selection uses both signals → much more reliable

2. **Heading prior from motion (§3.1) becomes tighter:**
   - Current runtime uses Kalman velocity (track-level, lagged)
   - Velocity-coherent uses per-cluster mean velocity (direct measurement)
   - Uncertainty $\sigma_\theta^{\text{motion}}$ narrows significantly

3. **Axis ambiguity resolution improves:**
   - Square objects (pedestrians) benefit most
   - Motion direction disambiguates PCA axis choice
   - Heading stability increases even for aspect ratio ≈ 1:1

**Without velocity-coherent data** (current runtime), the model **still improves** over current guards:

- Axis selection uses geometry alone (dimensions + heading continuity)
- Kalman velocity provides weak motion prior (existing mechanism)
- Displacement vector as fallback (existing mechanism)
- Shape classification modulates heading trust appropriately

The geometry-coherent model is designed to **work well standalone** and **improve further** when velocity-coherent data becomes available.

---

## 6. Configuration Parameters

New tuning parameters with proposed defaults:

| Parameter                            | Symbol                        | Default | Description                                                  |
| ------------------------------------ | ----------------------------- | ------- | ------------------------------------------------------------ |
| `geometry_dim_alpha`                 | $\alpha_{\text{dim}}$         | 0.05    | EMA rate for dimension updates                               |
| `geometry_heading_alpha`             | $\alpha_{\text{heading}}$     | 0.08    | EMA rate for heading updates (existing)                      |
| `geometry_outlier_threshold`         | $d_{\text{threshold}}$        | 6.0     | Residual threshold for outlier rejection                     |
| `geometry_classify_min_frames`       | $n_{\text{classify}}$         | 10      | Frames before shape classification                           |
| `geometry_elongated_ratio`           | $r_{\text{elongated}}$        | 1.5     | L/W ratio threshold for elongated class                      |
| `geometry_square_ratio`              | $r_{\text{square}}$           | 1.3     | L/W ratio threshold for square class                         |
| `geometry_sigma_dim_init`            | $\sigma_{\text{dim,init}}$    | 1.0     | Initial dimension uncertainty (metres)                       |
| `geometry_sigma_heading_init`        | $\sigma_{\theta,\text{init}}$ | π/2     | Initial heading uncertainty (radians)                        |
| `geometry_sigma_n_max`               | $n_{\max}$                    | 30      | Max observations for uncertainty floor                       |
| `geometry_motion_min_speed`          | $v_{\min}$                    | 0.5     | Min speed for motion-informed heading (m/s)                  |
| `geometry_motion_beta`               | $\beta_{\text{motion}}$       | 0.3     | Motion influence weight on heading prior                     |
| `geometry_motion_v_scale`            | $v_{\text{scale}}$            | 2.0     | Speed scale for uncertainty reduction (m/s)                  |
| `geometry_static_uncertainty_growth` | $\gamma_{\text{static}}$      | 0.01    | Heading uncertainty inflation for static objects (rad/frame) |
| `geometry_heading_weight_elongated`  | $w_\theta^{\text{elong}}$     | 1.0     | Heading residual weight for elongated objects                |
| `geometry_heading_weight_square`     | $w_\theta^{\text{square}}$    | 0.3     | Heading residual weight for square objects                   |
| `geometry_heading_weight_unknown`    | $w_\theta^{\text{unknown}}$   | 0.7     | Heading residual weight for unknown objects                  |

**Defaults are conservative estimates** based on:

- Current EMA rates in existing code
- Typical vehicle dimensions (4-5 m length, 1.8-2.0 m width)
- Typical pedestrian dimensions (0.5-0.7 m both axes)
- Expected sensor noise characteristics

Parameters should be validated and tuned through:

- Synthetic data testing (known geometry ground truth)
- Real-world validation dataset (manual annotation)
- Edge case stress testing (near-square vehicles, large trucks, groups)

---

## 7. Expected Outcomes

### 7.1 Quantitative Improvements

| Metric                                            | Current System | Expected with Geometry-Coherent |
| ------------------------------------------------- | -------------- | ------------------------------- |
| **Dimension stability** (std dev over 5s window)  | 0.3-0.5 m      | < 0.1 m                         |
| **Heading drift** (degrees/second for stationary) | 2-5°/s         | < 0.5°/s                        |
| **90° jump frequency** (per track)                | 0.1-0.3        | < 0.01                          |
| **Aspect ratio flip frequency** (per track)       | 0.2-0.5        | < 0.02                          |
| **Convergence time** (frames to stable geometry)  | 15-20          | 5-10                            |

### 7.2 Qualitative Improvements

1. **Stable shapes:** Dimensions converge to consistent values within 5-10 frames; outlier frames are rejected rather than averaged in

2. **No more spinning:** Axis selection replaces reactive guards; 90° jumps are mathematically impossible because the wrong axis interpretation always has higher residual

3. **Pedestrian-friendly:** Square clusters are classified and handled appropriately — heading trust is reduced, shape variation is expected

4. **Graceful degradation:** Without velocity-coherent data, the model still improves significantly over current guards. With velocity data, heading prior becomes much tighter.

5. **Code simplification:** Replaces 4 guard mechanisms (aspect-ratio lock, 90° jump rejection, dimension sync, heading disambiguation) with a single coherent Bayesian model

6. **Explainability:** Residual values and axis selection decisions are interpretable; can log which interpretation was chosen and why

### 7.3 Edge Cases Handled

- **Near-square vehicles** (vans, SUVs): Shape classification identifies as elongated after 10 frames; heading trust remains high
- **Pedestrian groups:** Initially classified as unknown; may stabilise as square or remain unknown (conservative)
- **Partial occlusion:** Outlier gate rejects frames with poor geometry; shape recovers when occlusion ends
- **Stationary → moving transition:** Heading uncertainty shrinks as speed increases; motion prior smoothly dominates
- **Sensor noise spikes:** High residual triggers outlier rejection; estimate unchanged for that frame

---

## 8. Implementation Estimate

| Component                              | Effort           | Files                                      |
| -------------------------------------- | ---------------- | ------------------------------------------ |
| Track geometry state struct            | **S** (half day) | `l5tracks/tracking.go`                     |
| Axis selection + outlier gate          | **M** (1 day)    | `l5tracks/tracking.go`                     |
| EMA update with shape classification   | **M** (1 day)    | `l5tracks/tracking.go`                     |
| Heading-motion coupling (§3)           | **M** (1 day)    | `l5tracks/tracking.go`                     |
| Uncertainty shrinkage logic            | **S** (half day) | `l5tracks/tracking.go`                     |
| Configuration parameters               | **S** (half day) | `config/tuning.go`, `l5tracks/tracking.go` |
| Remove replaced guards                 | **S** (half day) | `l5tracks/tracking.go`                     |
| Unit tests (axis selection, residuals) | **M** (1 day)    | `l5tracks/tracking_test.go`                |
| Integration tests (track stability)    | **M** (1 day)    | `l5tracks/tracking_coverage_test.go`       |
| Documentation and logging              | **S** (half day) | Various                                    |
| **Total**                              | **L (6-7 days)** |                                            |

### 8.1 Dependencies

**Required:**

- None (can be implemented with current pipeline)

**Enhanced by:**

- [Velocity-coherent foreground extraction](20260220-velocity-coherent-foreground-extraction.md) — provides per-cluster velocity for tighter heading priors

### 8.2 Testing Strategy

1. **Unit tests:**
   - Axis selection with known geometry + observations
   - Residual calculation edge cases (wraparound, large deltas)
   - Uncertainty shrinkage with varying observation counts
   - Shape classification with synthetic aspect ratio sequences

2. **Integration tests:**
   - Track stability over 100-frame sequences
   - Handling of 90° ambiguous inputs (should not flip)
   - Stationary object behaviour (heading uncertainty growth)
   - Moving object behaviour (heading-motion coupling)

3. **Validation dataset:**
   - Manually annotated real-world tracks (ground truth geometry)
   - Metrics: dimension RMSE, heading RMSE, flip frequency
   - Compare against current system quantitatively

4. **Edge case stress tests:**
   - Near-square vehicles (aspect ratio 1.2-1.4)
   - Partial occlusion sequences
   - Pedestrian groups (varying shape)
   - High-speed objects (motion dominance)
   - Stationary periods (uncertainty inflation)

### 8.3 Rollout Plan

**Phase 1:** Implement core geometry state + axis selection

- Replaces Guards 2 & 3
- Can run in parallel with current system for A/B testing
- Measure dimension stability, heading drift, flip frequency

**Phase 2:** Add shape classification

- Enables pedestrian-specific handling
- Measure improvement on square objects

**Phase 3:** Add heading-motion coupling

- Integrate with existing Kalman velocity
- Prepare for velocity-coherent data when available

**Phase 4:** Remove deprecated guards

- Clean up Guard 2, Guard 3, dimension sync logic
- Simplify `HeadingSource` enum
- Reduce code complexity

---

## 9. Alternatives Considered

### 9.1 Alternative: Stricter Guards

**Approach:** Tighten thresholds on existing guards (e.g., reject aspect ratio changes > 10%, reject heading changes > 5°).

**Rejected because:**

- Doesn't address root cause (no temporal model)
- Creates rigidity: true geometry changes (pedestrian arm movement) would be rejected
- Doesn't solve axis ambiguity — just suppresses symptoms more aggressively
- More parameters to tune without principled foundation

### 9.2 Alternative: Kalman Filter for Geometry

**Approach:** Full Kalman filter with state vector $[\mathbf{x}, \mathbf{v}, \mathbf{G}]$ (position, velocity, geometry).

**Rejected because:**

- Overkill: geometry evolves much slower than position/velocity
- Kalman assumes Gaussian noise and linear dynamics — PCA axis ambiguity is discrete, not Gaussian
- Requires process noise covariance tuning (complex)
- EMA with axis selection achieves similar result with less complexity
- Can revisit if EMA proves insufficient

### 9.3 Alternative: RANSAC-Based OBB Fitting

**Approach:** Use RANSAC to fit OBB directly to all points in track history, not just current frame.

**Rejected because:**

- Computationally expensive (quadratic in track length)
- Doesn't handle object geometry changes (e.g., pedestrian pose)
- Outlier rejection is coarse (entire frames, not individual observations)
- Doesn't provide uncertainty estimates
- Useful as an offline validation tool, not real-time tracker

---

## 10. Future Enhancements

### 10.1 Geometry-Aware Data Association

Once tracks maintain stable geometry:

- Use geometry similarity in cost matrix for Hungarian algorithm
- Reject associations with incompatible shapes (dimension mismatch > 1 m)
- Reduce ID switches caused by geometry confusion

### 10.2 Object Class Inference

Shape classification naturally extends to:

- **Pedestrian** (square, slow, small)
- **Bicycle** (elongated, medium speed, small)
- **Car** (elongated, medium-fast, medium)
- **Truck** (elongated, medium-fast, large)

Class priors could inform:

- Expected dimension ranges
- Typical motion patterns
- Sensor detection characteristics

### 10.3 Multi-Hypothesis Tracking

For ambiguous tracks, maintain multiple geometry hypotheses:

- Both axis interpretations tracked in parallel
- Hypotheses pruned as evidence accumulates
- Useful for handling temporary occlusions

---

## 11. References

### 11.1 Theoretical Foundation

- **Kalman Filtering and Data Association:** Bar-Shalom, Y., Fortmann, T. E. (1988)
  Foundation for Bayesian state estimation with uncertain observations.

- **Principal Component Analysis Ambiguity:** Jolliffe, I. T. (2002), _Principal Component Analysis_
  Documents the inherent axis orientation ambiguity in PCA.

- **RANSAC for Robust Estimation:** Fischler, M. A., Bolles, R. C. (1981)
  Alternative approach to outlier rejection (considered but not adopted).

### 11.2 Related Work in LiDAR Tracking

- **L-Shape Fitting for Vehicles:** Zhang, X., et al. (2017), "Real-Time Vehicle Detection and Tracking Using 3D LiDAR"
  Alternative to PCA for elongated objects; assumes L-shaped returns.

- **Track-Level Shape Refinement:** Held, D., et al. (2016), "Robust Real-Time Tracking Combining 3D Shape, Colour, and Motion"
  Uses shape consistency across frames; similar philosophy to this proposal.

### 11.3 Internal References

- `l4perception/obb.go`: Current PCA-based OBB estimation
- `l5tracks/tracking.go`: Current guard mechanisms (lines 950-1141)
- `l5tracks/kalman.go`: Existing Kalman filter for position/velocity
- [OBB heading stability review](20260222-obb-heading-stability-review.md): Current guard analysis
- [Velocity-coherent foreground extraction](20260220-velocity-coherent-foreground-extraction.md): Future enhancement

---

**Proposal Status:** Awaiting review and experimental validation.
**Next Steps:** Implement Phase 1 (core geometry state + axis selection) for A/B testing against current system.
