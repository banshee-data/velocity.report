# Unified scene semantics and temporal regions

- **Status:** Research proposal; not active in the runtime
- **Layers:** L3 Grid, L4 Perception, L5 Tracks, L6 Objects, L7 Scene
- **Related:** [Classification maths](../classification-maths.md), [background settling](../background-grid-settling-maths.md), [L7 scene plan](../../../docs/plans/lidar-l7-scene-plan.md), [vector scene map](../../../docs/lidar/architecture/vector-scene-map.md), [shared settling](20260219-unify-l3-l4-settling.md), [velocity-coherent extraction](20260220-velocity-coherent-foreground-extraction.md), [pose anchors](20260310-reflective-sign-pose-anchor-maths.md)

## 1. Decision and research question

Unify the semantic vocabulary, evidence contract, calibration conventions, and
label resolution rules. Retain specialised estimators at their existing layers.
A shared classifier means a common interpretation of evidence, not one neural
network called by every layer.

The research question is whether object semantics improve temporal region bounds
and track interpretation while preserving detection of real road users. The
candidate design below is a project proposal, not an established result from the
cited papers. No model has yet been evaluated on our captures for this proposal.

A tree can move without becoming a vehicle. A car can stop without becoming a
piece of street furniture. Class, behaviour, and measurement quality therefore
need separate variables, presented through one shared labelling system.

## 2. Current implementation and the gap

The source inspection for this proposal establishes these boundaries:

| Component                                                                     | Implemented evidence                                                                                        | Limitation for this proposal                                                                                      |
| ----------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------- |
| [L3 regions](../../../internal/lidar/l3grid/background_region.go)             | Mean `RangeSpreadMeters` during settling; percentile categories; connected components on ring/azimuth cells | No semantic identities; statistics stop after identification; angular neighbours need not share a physical object |
| [L3 drift](../../../internal/lidar/l3grid/background_drift.go)                | Range shifts from locked baselines and foreground fraction                                                  | Detects change, but does not establish its semantic cause                                                         |
| [L6 classifier](../../../internal/lidar/l6objects/classification.go)          | Priority rules over track dimensions, speed, and observation history; `rule-based-v1.2`                     | Returns one class and a heuristic confidence, not a calibrated distribution                                       |
| [Visualiser protocol](../../../proto/velocity_visualiser/v1/visualiser.proto) | Track classes and human labels, including user-only noise                                                   | Scene parts and persistent region annotations need additional semantics                                           |
| [Scene store](../../../internal/lidar/storage/sqlite/scene_store.go)          | Evaluation replay cases                                                                                     | A replay case is not a persistent semantic object                                                                 |

In particular, L3's `VariancePerCell` averages a spread in metres. It is not a
sample variance in square metres. Do not insert it directly into a covariance
matrix or claim that it measures canopy displacement.

L6 currently falls back to `dynamic` when observations are insufficient. That is
an uncertainty outcome, not proof of motion. Truck and motorcyclist branches are
reserved/disabled in the inspected rule cascade. The proposal must preserve
these current meanings during migration rather than silently reinterpret old data.

## 3. One vocabulary, several independent attributes

Let an entity's state be

$$
X_{o,t}=(C_o,B_{o,t},Q_{o,t},G_{o,t},V_{o,t}).
$$

Here $C$ is semantic class, $B$ behaviour, $Q$ return quality, $G$ geometry, and
$V$ visibility. An entity may be an object, an object part, or a surface patch.
A region is spatial support for observations; it can contain several entities.

### 3.1 Canonical semantic registry

Use one versioned registry with stable identifiers, parent relationships, display
names, supported subject types, and explicit external-taxonomy mappings.
Proposed families are:

| Family          | Initial classes or parts                                    | Interpretation                                                                       |
| --------------- | ----------------------------------------------------------- | ------------------------------------------------------------------------------------ |
| Road users      | car, bus, truck, pedestrian, cyclist, motorcyclist          | Preserve existing identifiers; availability is independent of vocabulary membership  |
| Animals         | bird, animal-unspecified                                    | Existing bird behaviour is broader than a biological species label                   |
| Vegetation      | vegetation-unspecified, tree, shrub; trunk and canopy parts | `part_of` joins tree parts; vegetation evidence alone does not prove a tree instance |
| Structures      | building, wall, fence, awning                               | Awning support and fabric can have different behaviour                               |
| Street fixtures | pole, sign                                                  | A sign can be attached to a pole without merging their identities                    |
| Surfaces        | ground, road, pavement, terrain                             | Surface semantics need not have countable instances                                  |
| Unresolved      | unknown                                                     | Lack of a supported semantic decision                                                |

Behaviour is a separate distribution over stationary, translating, deforming, and
unresolved. Parking is a contextual state derived from a car's stationary dwell
and scene relationship; use `stationary car` until parking is supported.
Presence events such as arrival/departure and visibility states such as occluded
are also separate from class. Quality distinguishes valid returns, suspected
artefacts, and unresolved quality. Vegetation returns can be valid measurements
of irrelevant motion: they are not automatically sensor noise.

Foreground/background remains an L3 decision about an observation relative to a
baseline. It is neither a semantic class nor a permanent object property.

### 3.2 Compatibility and external taxonomies

Legacy `car`, `bus`, and other supported labels retain their identifiers and
recorded meaning. `dynamic` maps to unknown class with a legacy-fallback marker;
behaviour requires independent evidence. A historical `noise` annotation remains
an annotation with its original scope, not an automatic vegetation label.
Do not infer truck from an old car label, or animal species from an old bird label.

For external model probabilities $q_s(k)$, a documented mapping is

$$
q_s^{*}(c)=\sum_k M^{(s)}_{ck}q_s(k),\qquad
M^{(s)}_{ck}\geq0,\quad\sum_cM^{(s)}_{ck}=1.
$$

Unsupported distinctions map to the nearest supported parent or unknown, never
to an invented leaf. A coarse vegetation class does not become tree with high
confidence. Keep parent-level evidence at that level until another source
supports refinement; do not treat parent and child as competing exclusive leaves.
SemanticKITTI's vegetation/trunk/pole/traffic-sign classes are useful starting
points, but its taxonomy has no dedicated awning class [R2].

## 4. Ownership across layers

| Layer | Produces                                                                   | Consumes                                                          | Does not own                            |
| ----- | -------------------------------------------------------------------------- | ----------------------------------------------------------------- | --------------------------------------- |
| L3    | Baseline residuals, occupancy, observation counts, quality evidence        | Immutable, bounded scene-policy snapshot from an earlier revision | Tree/car identity inference             |
| L4    | Geometry, parts, point/patch semantic proposals                            | Raw returns, L3 evidence, optional segmenter output               | Persistent identity                     |
| L5    | Association, rigid motion, track lifecycle, uncertainty                    | Geometry and bounded prior context                                | A separate semantic vocabulary          |
| L6    | Track semantic evidence and resolved track view                            | Track features and eligible semantic evidence                     | Persistent static scene geometry        |
| L7    | Persistent entity association, scene evidence fusion, envelopes, revisions | L3–L6 evidence and human annotations                              | Per-return synchronous neural inference |

A shared semantic contract and deterministic resolver can serve L6 and L7.
Their state stores and update cadences remain separate. Lower layers must not
import L7's mutable state: publish a compact snapshot through an explicit
interface. A stalled semantic worker leaves ordinary L3/L5 processing available.

L3 semantics therefore means semantically informed background policy plus local
measurement evidence. It does not require turning the grid into a second object
classifier. Run optional scene segmentation on background and raw returns too;
foreground-only input would omit the parked cars and fixtures we want to learn.

## 5. Evidence and association

An evidence item needs a subject/support reference, capture-time interval,
coordinate-frame and calibration revision, model/taxonomy version, distribution
or uncalibrated score, observability, and raw-evidence lineage. Human annotations
also carry author, scope, revision, and whether the assertion is locked.

Let $a_{io}$ be observation $i$'s association probability with entity $o$:

$$
a_{io}\geq0,\qquad \sum_o a_{io}\leq1.
$$

The remaining mass represents unassigned evidence. Estimate association using
3D distance, depth/visibility consistency, geometry, and motion. Semantic
compatibility may assist but cannot be its sole basis. Include a new-entity
hypothesis so a strong existing tree label cannot absorb every nearby return.

For a region $R$, a descriptive class mixture is

$$
p_R(c)=\frac{\sum_{i\in R}w_i\sum_o a_{io}p_o(c)}
{\sum_{i\in R}w_i\sum_o a_{io}}.
$$

Use non-negative visibility/coverage weights $w_i$ and report the unassigned
fraction separately. A zero denominator gives unknown. This is a summary of
associated evidence, not an independent classifier vote to feed back into the
same entities. Report mixed regions honestly instead of forcing one label.

## 6. Combining evidence without counting it twice

The conceptual temporal model is

$$
p(X_t\mid E_{1:t})\propto p(E_t\mid X_t)
\int p(X_t\mid X_{t-1})p(X_{t-1}\mid E_{1:t-1})\,dX_{t-1}.
$$

This states the desired inference, not a claim that all factors are independent
or that the current code implements a Bayesian filter. An L3 residual, an L4
cluster, and an L6 feature vector may all derive from the same returns.
Multiplying their probabilities would manufacture confidence.

### 6.1 Conservative first fusion rule

Start with a calibrated linear pool for each attribute:

$$
p_t(c)=w_0p_t^{-}(c)+\sum_{g=1}^{G}w_gq_{g,t}(c),\qquad
w_g\geq0,\quad\sum_{g=0}^{G}w_g=1.
$$

$p_t^{-}$ is a decayed prior and $g$ indexes evidence families with explicit
lineage, not pipeline layer numbers. Correlated products share one family's
weight budget. Each $q_g$ is an eligible distribution for the same subject,
attribute, and taxonomy level. Missing evidence gets no vote; weights are
renormalised. This is an engineering pooling rule, not an exact Bayes posterior.

Do not accumulate repeated snapshots as fresh observations. Update only with
new evidence blocks; overlapping windows must replace their earlier contribution
or share a capped block weight. Decay the retained state towards an appropriate
base prior $\pi$, using capture time:

$$
p_t^{-}=\rho p_{t-1}+(1-\rho)\pi,\qquad
\rho=\exp(-\Delta t/\tau).
$$

Choose different timescales for semantic identity and behaviour. Object class
usually persists longer than motion state. A seek, sensor change, or calibration
change requires explicit state selection/invalidation, not negative elapsed time.

Delay alone does not remove circular evidence. A scene-informed L3 decision
must retain the scene revision in its lineage. Its derived track evidence must
not independently reinforce that same prior. Keep raw geometry/kinematics
available, or exclude the dependent vote when its contribution cannot be separated.

### 6.2 Calibration, abstention, and contradiction

The current L6 confidence must remain marked heuristic until calibrated on
labelled replay cases. Do not construct a categorical distribution by assigning
that number to the winning class and spreading the rest arbitrarily.
An initial adapter can expose the rule result without enabling probabilistic
fusion; subsequently estimate a held-out confusion model or fit a calibrated
multiclass predictor from the features.

For learned logits $z_c$, temperature scaling is a candidate:

$$
q(c)=\frac{\exp(z_c/T)}{\sum_k\exp(z_k/T)},\qquad T>0.
$$

Fit $T$ on held-out local data; it does not correct missing classes or domain
shift by itself [R5]. Measure calibration by class and range, alongside proper
scores such as Brier score. Abstain when maximum probability is low, evidence
coverage is inadequate, or association is ambiguous. Thresholds require validation.

Keep disagreement visible even when the pooled result is decisive. Human locks
control the resolved display/policy label within their scope; preserve the model
posterior alongside them. They are not infinitely many training observations.

## 7. Temporal geometry and noise bounds

Separate sensor noise from actual object deformation. For a scalar range
residual $r_t$, a useful conditional approximation is

$$
\operatorname{Var}(r)=\sigma^2_{\rm sensor}
+\sigma^2_{\rm pose}+\sigma^2_{\rm object}.
$$

This additive form assumes the components are approximately uncorrelated after
conditioning on range, incidence, and object state. Otherwise retain covariance
terms or fit the joint residual distribution. L3 spread alone cannot identify
these components. Rigid fixtures and the existing pose-anchor proposal provide
candidate controls for separating common sensor motion from local deformation.

Let $u_{o,t}(x)$ estimate object occupancy at location $x$, conditional on visibility
and a behavioural regime. Learn it from associated observations with bounded
history; unseen space is not an observed absence. Define a core by high occupancy
and a motion envelope from observed excursions around that core.

For a unit direction $d$, let $D_{o,t}(d)$ be the maximum associated excursion
beyond the core in a visible time block. A proposed directional bound is

$$
b_o(d)=Q_{1-\alpha}\{D_{o,t}(d)\}+m_o(d),
$$

where $m_o$ covers measurement/pose uncertainty. Quantiles use time blocks rather
than treating thousands of correlated points as independent samples. Coverage is
an empirical claim for the sampled regime, not a guarantee for unseen storms.
Maintain a normal envelope and a separate exceptional-excursion history; require
support before promotion, and allow decay. Do not let one outlier set the maximum
forever. Sparse directions remain uncertain rather than receiving zero width.

Tree trunk and canopy have different envelopes. Awning frame and fabric have
different envelopes. A parked car retains rigid geometry while its behaviour
changes from stationary to translating. Avoid assuming all vegetation is a single
Gaussian blob or that every object needs a convex enclosing region.

## 8. Feeding scene knowledge back into L3

Project object geometry into the polar grid using the current calibration,
including expected depth and visibility. A 2D cell mask alone cannot distinguish
a pedestrian in front of a tree from the tree itself. Allow multiple candidate
surfaces per ray in the scene representation even if the current L3 baseline
stores one principal range.

A future suppression decision should minimise expected loss:

$$
a^*=\arg\min_{a\in\{\mathrm{retain},\mathrm{suppress}\}}
\sum_y L(a,y)p(y\mid e_i,S_{t-1}),
$$

where $y$ distinguishes relevant foreground, expected background variation, and
artefact; $e_i$ is current measurement evidence and $S_{t-1}$ an eligible snapshot.
Missing a road user must cost more than retaining some foliage. No numerical
costs or suppression thresholds are established by this proposal.

Envelope membership supplies context only. Require depth agreement, valid
association, and compatible deformation evidence; coherent independent motion
or ambiguous ownership retains the return. Cap semantic policy adjustments and
fall back to baseline processing when the snapshot is stale or incompatible.

## 9. Local re-examination and stable identity

Monitor visible support, envelope exceedance, semantic disagreement, occupancy
change, and rigid-motion evidence. A candidate change detector is

$$
g_t=\max(0,g_{t-1}+s_t-\nu),\qquad g_t>h\Rightarrow\text{review candidate}.
$$

$s_t$ is a calibrated anomaly score per fixed-duration capture-time block;
$\nu$ is tolerated background activity and $h$ controls persistence. This is a
CUSUM-style design candidate; its operating point needs replay measurement.
Occluded blocks do not count as departures. Widespread rigid displacement first
checks pose/calibration rather than revising every object.

Re-examine the affected object and neighbouring volume, retaining a new-entity
hypothesis. Produce an immutable candidate revision; accept only after evidence
and policy checks. Preserve split/merge ancestry and human edits. A disappearing
track does not delete its scene object, and scene identity does not assert that a
similar car returning later is the same vehicle. Identity is local and bounded
to the observation episode; this proposal does not require cross-visit recognition.

## 10. Worked cases and the shared visualiser

| Evidence                                                            | Shared label view                                 | Region consequence                                                          |
| ------------------------------------------------------------------- | ------------------------------------------------- | --------------------------------------------------------------------------- |
| Canopy returns fluctuate around a stable trunk                      | Tree; canopy part; deforming; valid returns       | Update canopy excursion history, preserve trunk bounds                      |
| Person moves through the canopy's projected cells at a nearer depth | Pedestrian; translating; separate association     | Retain foreground despite angular overlap                                   |
| Previously stationary rigid vehicle develops coherent velocity      | Car; translating; departure candidate             | Preserve class, withdraw stationary occupancy only with visibility evidence |
| Small noisy track falls inside a sign region                        | Unknown until geometry/depth association resolves | Region context cannot automatically relabel it sign or noise                |
| User marks a patch as awning                                        | Awning annotation with support and revision       | Apply local semantic correction; learn fabric behaviour separately          |

VelocityVisualiser should use one label picker backed by the registry, filtered
by subject type. Selection can target a track, entity, part, or spatial support.
Show class, behaviour, quality, provenance, and uncertainty as separate fields.
For mixed regions show the mixture and unassigned fraction. Display normal and
exceptional envelopes and jump to the evidence that triggered a revision.

Corrections can relabel, split/merge, constrain geometry, accept/reject a revision,
or lock a scoped assertion. Preserve model output and annotation history; undo
creates a revision. Label transfer from a track to a scene entity requires verified
association, not spatial overlap. Training exports distinguish human labels,
accepted proposals, and untouched predictions. Never evaluate on the same
pseudo-labels used to adapt the system.

## 11. Research comparison and acceptance evidence

MinkowskiEngine supplies sparse tensor operations, including higher-dimensional
networks; it does not supply our taxonomy or fusion policy [R1]. PTv3 is a
candidate semantic producer [R3]. 4DMOS supplies temporal movement evidence,
which must remain separate from object meaning [R4]. Stationary occupancy time
series are a particularly relevant non-semantic baseline [R6]. None establishes
that our proposed fusion improves our sensor's results.

Compare the following on identical replay intervals:

1. Existing L3 regions and L6 rules.
2. Shared vocabulary/adapters with unchanged decisions.
3. Geometry plus temporal occupancy/envelopes, without learned semantics.
4. Semantic proposals plus that temporal model, without feedback to L3.
5. Bounded semantic feedback to L3, with lineage controls.

Measure per-class precision/recall and IoU, instance split/merge errors, calibration,
unknown coverage, foreground false positives, pedestrian/vehicle recall, departure
delay, envelope coverage/spillover, revision churn, and annotation effort. Report
compute latency, peak memory, and snapshot age on the actual intended hardware.

Hold out whole sites or recording periods, including weather regimes. Do not
split adjacent frames between training and evaluation. Include distant poles,
awnings absent from training labels, stationary people, slow cars, foliage beside
traffic, sensor vibration, occlusion, missing returns, replay seeks, and model
worker failure. Use block-level uncertainty intervals for correlated sequences.

Before enabling suppression feedback, predeclare acceptable road-user recall loss,
false-positive improvement, and revision rate with the evaluation owner. A mean
semantic score is not a sufficient acceptance gate. Until measured, all claimed
benefits remain hypotheses.

## 12. Simplification boundary and open decisions

The smallest useful implementation is one vocabulary, one evidence contract,
existing L6 rules behind an adapter, and persistent human-labelled scene objects
with measured temporal envelopes. Learned semantics can join later as another
producer. This avoids making neural deployment a prerequisite for useful labels.

Keep a single authoritative registry and resolver policy; do not introduce
separate tree enums for grid, scene, and visualiser. Keep statistical estimators
where their observations live. Do not unify a region ID with a track ID.

Questions to settle experimentally are the required taxonomy depth, minimum
visible support, normal/exceptional envelope timescales, calibration under sensor
domain shift, association quality near foliage, and the value of learned semantics
above temporal geometry alone. Storage/API evolution and implementation scheduling
belong in a follow-on plan after these contracts are agreed.

## References

- **R1:** [NVIDIA MinkowskiEngine](https://github.com/NVIDIA/MinkowskiEngine), sparse tensor library and installation requirements.
- **R2:** [SemanticKITTI label mappings](https://github.com/PRBonn/semantic-kitti-api/blob/master/config/semantic-kitti.yaml), source taxonomy and learning mappings.
- **R3:** [Point Transformer V3](https://github.com/Pointcept/PointTransformerV3), official model repository. Verify checkpoint/configuration compatibility before experiments; the project page carries weight availability caveats.
- **R4:** [4DMOS](https://github.com/PRBonn/4DMOS), receding moving-object segmentation from LiDAR sequences.
- **R5:** [Guo et al., On Calibration of Modern Neural Networks, ICML 2017](https://proceedings.mlr.press/v70/guo17a.html), calibration and temperature scaling; local applicability requires measurement.
- **R6:** [Kreutz et al., Unsupervised 4D LiDAR Moving Object Segmentation in Stationary Settings With Multivariate Occupancy Time Series, WACV 2023](https://openaccess.thecvf.com/content/WACV2023/html/Kreutz_Unsupervised_4D_LiDAR_Moving_Object_Segmentation_in_Stationary_Settings_With_WACV_2023_paper.html), occupancy changes over spatial neighbourhoods and time.
