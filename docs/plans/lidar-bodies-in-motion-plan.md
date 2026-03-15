# Bodies in Motion — Path Prediction and Sparse-Cluster Track Linking

**Status:** 📋 Planned — v2.0+
**Created:** 2026-03-08
**Layers:** L5 Tracks (kinematics), L7 Scene (scene constraints, interaction, scene graph)
**Canonical architecture:** [lidar-data-layer-model.md](../lidar/architecture/lidar-data-layer-model.md)
**Parent plan:** [lidar-l7-scene-plan.md](lidar-l7-scene-plan.md) § 4
**Maths proposal:** to be written — `data/maths/proposals/YYYYMMDD-bodies-in-motion-maths.md`

---

## 1. Motivation

The current L5 tracker uses a constant-velocity (CV) Kalman filter. This is a strong baseline for straight-line freeway traffic but under-performs in three scenarios that matter for neighbourhood-scale intersections:

1. **Braking and acceleration events** — a CV model predicts constant speed through a stop sign; real vehicles decelerate, stop, and re-accelerate. The prediction fan diverges from reality within 1–2 seconds.
2. **Curved trajectories** — vehicles turning at an intersection follow the road polygon; the CV model predicts a straight overshoot into the pavement or building.
3. **Sparse-cluster track fragmentation** — at range (> 40 m) or during partial occlusion, a single vehicle may produce only 2–5 LiDAR points per frame. The resulting clusters are spatially noisy and frequently split into micro-clusters. The tracker creates and kills many short-lived tracks instead of maintaining one continuous track. Better motion priors and scene-aware association can bridge these gaps.

---

## 2. Scope and Layer Placement

| Workstream                                                                             | Layer                        | Why                                                                                                            |
| -------------------------------------------------------------------------------------- | ---------------------------- | -------------------------------------------------------------------------------------------------------------- |
| Richer single-object kinematics (CA, CTRV, IMM)                                        | **L5 Tracks**                | Per-track state estimator; no cross-object or scene knowledge needed                                           |
| Scene-constrained path prediction (road-following, kerb clipping, stop-line awareness) | **L7 Scene**                 | Requires accumulated road polygons, kerb geometry, and structure walls                                         |
| Multi-object interaction prediction (following distance, yield/gap acceptance)         | **L7 Scene**                 | Requires simultaneous visibility of all canonical objects plus road topology                                   |
| Sparse-cluster track linking (bridge fragmented detections at range)                   | **L5 Tracks** + **L7 Scene** | L5 provides kinematic extrapolation; L7 provides scene-constrained corridors that tighten the association gate |
| Post-hoc kinematic analysis (braking events, stopping distances)                       | **L8 Analytics**             | Derived measurements over historical state; not real-time                                                      |

---

## 3. L5 Kinematic Extensions

### 3.1 Constant-acceleration model (CA)

Extend the Kalman state from $\mathbf{x} = [x, y, v_x, v_y]^T$ to $\mathbf{x} = [x, y, v_x, v_y, a_x, a_y]^T$. The prediction step becomes:

$$\hat{x}_{k+1} = \begin{bmatrix} 1 & 0 & \Delta t & 0 & \frac{1}{2}\Delta t^2 & 0 \\ 0 & 1 & 0 & \Delta t & 0 & \frac{1}{2}\Delta t^2 \\ 0 & 0 & 1 & 0 & \Delta t & 0 \\ 0 & 0 & 0 & 1 & 0 & \Delta t \\ 0 & 0 & 0 & 0 & 1 & 0 \\ 0 & 0 & 0 & 0 & 0 & 1 \end{bmatrix} \mathbf{x}_k$$

The CA model captures braking and acceleration but cannot model turns.

### 3.2 Constant turn-rate and velocity model (CTRV)

State: $\mathbf{x} = [x, y, \theta, v, \omega]^T$ where $\theta$ is heading, $v$ is scalar speed, and $\omega$ is yaw rate.

Prediction (nonlinear):

$$x_{k+1} = x_k + \frac{v_k}{\omega_k}\left(\sin(\theta_k + \omega_k \Delta t) - \sin(\theta_k)\right)$$
$$y_{k+1} = y_k + \frac{v_k}{\omega_k}\left(\cos(\theta_k) - \cos(\theta_k + \omega_k \Delta t)\right)$$

When $|\omega_k| < \epsilon$, fall back to straight-line CV prediction to avoid division by zero.

The CTRV model captures turns at constant speed but not acceleration during turns. Requires an extended or unscented Kalman filter (EKF/UKF) due to nonlinearity.

### 3.3 Interacting Multiple Model (IMM)

Rather than choosing one motion model, IMM runs $M$ filters in parallel (e.g. CV, CA, CTRV) and blends their outputs by model probability:

$$\hat{x}_{k|k} = \sum_{j=1}^{M} \mu_j \hat{x}_k^{(j)}$$

where $\mu_j$ is the posterior probability of model $j$ given the observations. A Markov transition matrix governs model switching:

$$\mu_{j,k+1|k} = \sum_{i=1}^{M} \pi_{ij} \mu_{i,k}$$

IMM is the recommended approach because:

- It automatically selects CV for straight-line segments, CA for braking/acceleration, and CTRV for turns
- Model probabilities are useful diagnostic signals (dashboard: "this vehicle is in braking mode")
- The computational cost is 3× the single-model filter — acceptable at our track counts (< 50 concurrent)

### 3.4 Ground-plane constraint in L5

Regardless of the kinematic model, L5 should constrain the predicted state using the L4 `GroundSurface` interface:

```
predicted_z = max(kinematic_z, GroundSurface.HeightAt(predicted_x, predicted_y))
```

This prevents predicted trajectories from going underground. The constraint is applied as a one-sided clamp after the Kalman prediction step, not as a measurement update, to avoid biasing the filter.

---

## 4. L7 Scene-Constrained Prediction

### 4.1 Road-corridor constraint

L7 accumulated road polygons define driveable corridors. A kinematic trajectory fan from L5 is clipped to the corridor:

```
L5 trajectory fan (unconstrained)
    ↓
for each predicted waypoint:
    if waypoint is outside nearest road polygon:
        project waypoint onto polygon boundary
        reduce probability mass for this trajectory branch
    ↓
L7 constrained trajectory fan
```

The corridor width comes from the road polygon geometry. Kerb boundaries act as soft walls — the probability of crossing a kerb is nonzero (vehicles do mount kerbs) but heavily penalised.

### 4.2 Stop-line and intersection awareness

When a track's predicted corridor intersects a known stop-line feature (imported from OSM priors or observed as road markings):

1. Compute time-to-arrival at stop-line from current velocity
2. If vehicle is decelerating (CA/CTRV model): predict a stop event with deceleration $a$ and stopping distance $d = v^2 / 2|a|$
3. If vehicle is at constant velocity: maintain two trajectory hypotheses — "continues through" and "stops at line"
4. Weight hypotheses by observed approach behaviour (binary: has the vehicle started braking?)

This is structurally similar to IMM but at the trajectory level rather than the filter level.

### 4.3 Multi-object interaction

When two canonical objects share the same road corridor:

- **Following constraint** — if B is behind A at the same heading, B's maximum predicted speed is bounded by A's speed plus a reaction-time margin
- **Yielding constraint** — at intersection conflict points, objects on lower-priority approaches yield to higher-priority approaches (if priority is known from road topology)

These constraints modify the L7 trajectory probability distribution, not the L5 kinematic state. L5 tracks remain independent.

---

## 5. Sparse-Cluster Track Linking

### 5.1 The problem

At range (> 40 m) or during partial occlusion, a vehicle produces sparse point returns:

```
Frame t:     ·  ·        (3 points → 1 small cluster)
Frame t+1:              (0 points → no cluster)
Frame t+2:   ·           (1 point → micro-cluster or noise reject)
Frame t+3:    · ·        (2 points → 1 cluster, shifted)
```

The current tracker:

- Creates a tentative track at frame $t$
- Misses at frame $t+1$ (coasts)
- Cannot confidently associate the micro-cluster at $t+2$ (Mahalanobis gate too wide due to coast uncertainty)
- Kills the track at $t+3$ after exceeding `max_consecutive_misses`
- Creates a new tentative track at $t+3$
- Result: two short-lived tracks instead of one continuous track

### 5.2 Solutions by layer

**L5 — relaxed association for sparse clusters:**

Extend the association gate for tracks in "sparse mode" (defined as tracks where the average cluster point count over the last $N$ frames is below a threshold, e.g. < 8 points):

- Widen Mahalanobis gate by a configurable factor (e.g. 1.5×) for sparse tracks
- Allow association with micro-clusters (clusters with < 3 points) that would normally be rejected as noise
- Increase `max_consecutive_misses` for sparse tracks (e.g. from 3 to 6)
- Reduce the promotion threshold (tentative → confirmed) for sparse tracks if velocity is consistent

**L7 — scene-corridor-assisted association:**

When L5 fails to associate a sparse cluster with a coasting track, L7 can provide a tighter prediction by constraining the search to the road corridor:

```
L5 coasting track: predicted position ± large uncertainty ellipse
L7 corridor mask: road polygon between departure point and predicted position
    ↓
effective search region = uncertainty ellipse ∩ corridor mask
    ↓
association gate is tighter → fewer false positives → longer tracks
```

This is the key value of L7 for sparse tracking: scene geometry reduces the association ambiguity that causes fragmentation.

**L7 — retrospective track merger:**

After a vehicle passes through the sparse zone and produces a confirmed track again, L7 can retrospectively merge the two fragments:

1. Track A dies at time $t_1$ at position $p_1$ with velocity $v_1$
2. Track B starts at time $t_2$ at position $p_2$ with velocity $v_2$
3. If $t_2 - t_1 <$ gap threshold **and** $|p_2 - (p_1 + v_1 \cdot (t_2 - t_1))| <$ distance threshold **and** the corridor between $p_1$ and $p_2$ follows a road polygon:
4. Merge A and B into a single canonical object

This is a batch operation, not real-time, and naturally belongs in L7's accumulated scene model.

---

## 6. Scene Graph Relations

The L7 scene graph encodes persistent spatial relationships between features. These relations are accumulated over many frames and carry per-relation confidence.

### 6.1 Relation types

| Relation          | Subject → Object               | Accumulated evidence                                |
| ----------------- | ------------------------------ | --------------------------------------------------- |
| `contacts_ground` | Cluster/Object → GroundPolygon | Base-Z offset mean and variance (Welford)           |
| `follows_road`    | Track/Object → GroundPolygon   | Fraction of trajectory waypoints inside polygon     |
| `occluded_by`     | Cluster → StructureFeature     | Angle range and frequency of line-of-sight blockage |
| `adjacent_to`     | Cluster → VolumeFeature        | Proximity to known clutter source (tree, hedge)     |
| `bounded_by`      | Object → StructureFeature      | Object trajectory never penetrates wall             |
| `rests_on`        | VolumeFeature → GroundPolygon  | Static object base contacts ground                  |

### 6.2 Relation data model

```go
type SceneRelation struct {
    SubjectID   FeatureID
    ObjectID    FeatureID
    Kind        RelationKind
    Confidence  float32
    Params      RelationParams
    ObsCount    uint32
}

type RelationParams struct {
    BaseOffset    float64   // contacts_ground: mean base-Z offset
    CorridorWidth float64   // follows_road: observed corridor width
    OcclusionArc  [2]float64 // occluded_by: azimuth range
}
```

### 6.3 How relations constrain prediction

- **contacts_ground**: predicted base-Z follows the ground polygon's local slope; objects cannot sink below ground
- **follows_road**: predicted trajectory stays within the accumulated corridor width of the road polygon
- **bounded_by**: predicted trajectory clips at wall boundaries; probability of wall penetration is zero
- **occluded_by**: expected detection probability drops in the occlusion arc; tracker should coast rather than kill

---

## 7. Implementation Phases

### Phase 1: L5 kinematic extensions (v2.0)

- Implement CA model alongside existing CV
- Implement CTRV model with EKF
- Implement IMM blending (CV + CA + CTRV)
- Add ground-plane Z-clamping to prediction step
- Add model-probability diagnostic to track state
- Write maths proposal: `data/maths/proposals/YYYYMMDD-bodies-in-motion-maths.md`

### Phase 2: sparse-cluster track linking (v2.0)

- Add sparse-mode detection (average point count < threshold)
- Widen association gate and miss tolerance for sparse tracks
- Allow micro-cluster association for sparse tracks
- Validate on PCAP replays at > 40 m range
- Measure fragmentation reduction vs false-merge rate

### Phase 3: L7 scene-constrained prediction (v2.0+)

- Implement road-corridor clipping of L5 trajectory fans
- Implement stop-line awareness (requires OSM priors or observed stop-line features)
- Add scene-corridor-assisted association for sparse tracks
- Add retrospective track merger in L7

### Phase 4: scene graph and multi-object interaction (v2.0+)

- Implement `SceneRelation` accumulation
- Add `contacts_ground` and `follows_road` relations
- Implement following-distance constraint
- Add relation-based prediction constraints
- Extend sweeps dashboard with trajectory-quality metrics

---

## 8. Build Checklist

- [ ] Choose EKF vs UKF for CTRV (recommend UKF for simpler Jacobian-free implementation)
- [ ] Define IMM transition matrix defaults (CV↔CA, CV↔CTRV switching probabilities)
- [ ] Define sparse-mode point-count threshold and gate widening factor
- [ ] Define retrospective merge gap threshold and distance threshold
- [ ] Implement `contacts_ground` relation accumulation
- [ ] Implement `follows_road` relation accumulation
- [ ] Add corridor-clipping to L7 prediction path
- [ ] Write maths proposal covering CA/CTRV state equations, IMM blending, and corridor probability model
- [ ] Add integration tests: CV→CA model switch on braking PCAP
- [ ] Add integration tests: CTRV heading through intersection turn PCAP
- [ ] Add integration tests: sparse-cluster linking at range > 40 m
- [ ] Add integration tests: retrospective merge of fragmented sparse tracks
- [ ] Validate: fragmentation rate reduction ≥ 30% at > 40 m range
- [ ] Validate: braking-event prediction error ≤ 2 m at 2 s horizon

---

## 9. Non-Goals for First Pass

- Full probabilistic trajectory forecasting (Trajectron++ style) — too complex; simple corridor clipping first
- Pedestrian-specific motion models — vehicle kinematics only for v2.0
- Map-graph topology (lane-level routing) — road polygons as corridors are sufficient initially
- Online learning of interaction parameters — use fixed conservative defaults
- GPU-accelerated prediction — track counts (< 50) do not justify it
