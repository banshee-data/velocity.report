# Visibility-aware object tracking: mathematical review and declarations

This note defines the maths needed to track one rigid vehicle through changing views without
mistaking a visible surface for the whole object. It audits the proposed single-site tracker and
sets out revisions to the earlier OBB, geometry, annotation, and state-estimation documents.

- **Status:** Research proposal; derivations and numerical checks, not a validated tracker
- **Layers:** L4 observations, L5 estimation, L6 classification, offline evaluation
- **Related:** [Demo sprint](../../../docs/plans/lidar-single-site-shape-demo-sprint-plan.md), [Point annotation](../../../docs/plans/lidar-point-annotation-and-object-dataset-plan.md), [State estimation](../../../docs/plans/lidar-state-estimation-plan.md), [Original D-04](20260222-geometry-coherent-tracking.md), [OBB review](20260222-obb-heading-stability-review.md)

## 1. Recommendation and evidence boundary

Keep the small seeded tracker as an experiment, but add observability checks before treating its
trail as an estimate of physical motion. Point-to-point registration is a comparator; it is not a
licence to infer certainty from a low residual. A visibility-aware surface model is the intended
successor. Neither requires a neural checkpoint.

Four kinds of statement appear here:

- **Source result:** attributed to a primary paper or its authors' implementation.
- **Derived result:** follows from the stated equations; checked examples are in Section 10.
- **Design proposal:** a choice for our next implementation, still requiring experiments.
- **Branch observation:** what the inspected code or document says, not independently reproduced data.

The root `dd/docs/state-est` branch was inspected at `6a9b6ebf6`. Its heading sprint now includes
the replay harness and reports an A/B experiment: course error decreases, lock duration falls,
fragmentation increases slightly, and repeated runs differ. This note does not rerun that experiment
or accept its causal explanations as established. The current planning worktree is a different
checkout; root-only changes are identified in the revision register rather than overwritten.

## 2. What SOTracker supplies, and what it does not

SOTracker estimates translation and yaw from a supplied initial box and scene point clouds. Its
objective combines registration, accumulated shape, and motion terms. The paper explicitly discusses
sparsity, drift, and abrupt motion as difficult cases. It is a useful optimisation-based comparator,
not a semantic classifier or an uncertainty guarantee.
[Pang, Li, and Wang, Sections IV–V](https://arxiv.org/html/2103.06028v2)

The authors' API supplies a custom point-cloud loader and sensor/world transform. A local adapter
must verify frame conventions rather than substitute a Waymo-shaped folder layout. Inspect and pin
the implementation and its reuse terms before importing code; our local baseline remains explicitly
SOTracker-inspired rather than an asserted reproduction.
[Authors' implementation](https://github.com/tusen-ai/LiDAR_SOT)

The equations below are our proposed contract. They do not claim to reproduce the paper's complete
algorithm, losses, calibration, or benchmark results.

## 3. State, coordinates, and the object reference point

Let a recorded sensor return be \(p^S_{ti}\), sensor-to-world transform be \(E_t\), and body-to-world
pose be \(T_t\). Then:

$$
y^W_{ti}=E_t p^S_{ti},\qquad q^B_{ti}=T_t^{-1}y^W_{ti}.
$$

The persistent shape \(G\) lives in body coordinates. Its parameters may initially be dimensions
\((L,W,H)\), later named cuboids or surfaces. The first experiment estimates
\(x_t=(c_x,c_y,\psi)\), with a declared ground-conditioned vertical reference. That is a planar
approximation, not a six-degree-of-freedom estimator. Grade or body articulation can invalidate it.

Choose a stable body origin, such as the ground projection of the physical box centre. A visible
medoid, a fitted footprint centre, and this body origin are different quantities. Store the origin
convention explicitly; a rendered box and a trail must not silently use different ones.

### 3.1 Viewing aspect is derived, not a free rotation state

For sensor origin \(s\), vehicle origin \(c_t\), and body yaw \(\psi_t\):

$$
\phi_t=\operatorname{wrap}_{2\pi}
\left(\operatorname{atan2}(s_y-c_{t,y},s_x-c_{t,x})-\psi_t\right).
$$

Translation past a fixed sensor changes \(\phi_t\) even when \(\psi_t\) is constant. Time provides
additional views, not an independent spatial rotation. Elevation and road orientation extend this
view model when needed. Do not fit an unconstrained extra aspect angle that can absorb pose errors.

For an outward-facing surface normal \(n_f^B\), front-facing support requires
\(n_f^{B\top}(s^B-q_f^B)>0\). This is necessary, not sufficient: self-occlusion, other objects,
field of view, and sensor sampling still determine which returns exist. An absent return is not
automatically evidence of empty space.

### 3.2 Course and body yaw can legitimately differ

For a fixed body offset \(b\), the point's world position and velocity are:

$$
p_b=c+R(\psi)b,\qquad
v_b=v_c+\omega R(\psi)Jb,\qquad
J=\begin{bmatrix}0&-1\\1&0\end{bmatrix}.
$$

The turning term alone changes course at different reference points. At forward speed 4 m/s,
yaw rate 0.35 rad/s, and a 2 m longitudinal offset, it creates 0.7 m/s lateral velocity and a
9.93-degree course difference. No side slip is required. A jumping medoid adds another error.

Therefore course alignment is a diagnostic or a weak, class-conditioned prior. It is not body-yaw
ground truth. Reverse motion requires signed longitudinal speed; body-front identity must not flip
when the vehicle reverses. Pedestrians and articulated objects need different assumptions.

## 4. Observation model: membership, surfaces, and ambiguity

Observed points are samples from visible surfaces plus clutter. We do not observe the same physical
material point in every scan. Human masks specify membership of recorded returns; they do not supply
cross-frame point correspondences or certify unseen shape.

A useful conditional model for a return in a candidate search region is:

$$
p(y_i\mid T,G)=\epsilon p_{\rm out}(y_i)
 +(1-\epsilon)\sum_{f\in V(T,G)}\pi_f\,
 p_f(T^{-1}y_i\mid G).
$$

Here \(V\) represents visible surface hypotheses, \(\pi_f\) are normalised mixture weights, and
\(p_f\) includes normal error, finite tangential support, and sampling assumptions. This conditional
return model alone does not model missing detections or return count. Those need explicit support
or detectability terms; otherwise shrinking the crop or declaring every point an outlier can win.

The full problem has coupled latent pose, shape, membership, and face identity. Alternating between
them is an approximation to joint inference. Do not call the first chosen association truth, then
use that same association as independent proof that the shape model is correct.

### 4.1 Axial orientation and rectangle equivalence

Directed yaw wraps modulo \(2\pi\). An unlabelled body axis wraps modulo \(\pi\):

$$
r_{\rm axis}(a,b)=\tfrac12\operatorname{atan2}
\bigl(\sin 2(a-b),\cos 2(a-b)\bigr).
$$

The same rectangular footprint can be written as
\((L,W,\psi)\), \((L,W,\psi+\pi)\), or \((W,L,\psi+\pi/2)\).
The last equivalence requires swapping dimensions as well as angle. It does not prove a physical
quarter-turn or identify the vehicle's front. With weak evidence, retain alternatives or abstain.
A single Gaussian centred between distinct axis hypotheses can describe neither hypothesis.

For genuinely Gaussian, uncensored features under hypothesis \(h\), a comparative score is:

$$
s_h=r_h^\top S_h^{-1}r_h+\log\det S_h-2\log P(h),
\qquad S_h=P_{\rm predicted}+R_{\rm observation}.
$$

Use consistent coordinates and cross-covariances. The additive covariance expression assumes
independent prediction and observation errors; shared evidence requires correlation terms or a
declared conservative approximation. The log determinant cancels only when the alternatives have
the same covariance. Censored dimensions need Section 7 instead of this Gaussian residual.

A squared Mahalanobis threshold of 6 is not a per-coordinate 2.5-sigma test. Its probability depends
on dimension and the validity of the assumed distribution. A gate value of 36 in current association
code is likewise not a six-metre Euclidean radius.

## 5. Registration objective and observability

### 5.1 A bounded estimator with meaningful residuals

For a body-map point \(q_j\) and associated observed point \(y_i\), point-to-point registration uses
\(Tq_j-y_i\). A surface-normal residual instead uses:

$$
r_i(x)=n_i^\top(R(\psi)q_j+c-y_i).
$$

Proposed regularised objective:

$$
E(x,G)=\sum_i w_i\,\rho\!\left(r_i^2/\sigma_i^2\right)
 +r_{\rm motion}^\top Q^{-1}r_{\rm motion}
 +E_{\rm shape}(G)+E_{\rm support}.
$$

The robust loss \(\rho\), correspondence weights, and support penalty must be specified. A zero
support solution is invalid, not a perfect fit. Distance thresholds, crop sizes, and uncertainty
scales are parameters with units. Normalise rotational and translational residuals through declared
covariances/scales; adding raw squared metres to squared radians is not a portable noise model.

Sensor range/angular uncertainty becomes anisotropic Cartesian uncertainty through its Jacobian.
Project it onto the normal and include reference-surface/normal uncertainty. Points derived from
the same uncertain pose share error; summing their independent information exaggerates confidence.
Without a calibrated noise/correlation model, report this as robust regularised fitting, not a
calibrated Bayesian posterior.

### 5.2 Worked side-plane degeneracy

For local perturbation \((\delta c_x,\delta c_y,\delta\psi)\), the Jacobian row is:

$$
J_i=\left[n_{ix},\ n_{iy},\ n_i^\top R(\psi)Jq_i\right].
$$

Take a flat side at \(q_i=(u_i,b)\), yaw zero, and normal \((0,1)\). Then
\(J_i=[0,1,u_i]\). For \(u_i\in\{-2,-1,0,1,2\}\) with unit weights:

$$
H_{\rm data}=\sum_i J_i^\top J_i=
\begin{bmatrix}0&0&0\\0&5&0\\0&0&10\end{bmatrix}.
$$

Along-side translation is unobservable in this ideal plane model. More points on the same surface
do not change that rank. A resolved physical edge or another nonparallel surface can add information;
a sampling/FOV boundary is not necessarily such an edge. Normal estimation noise can also create
small spurious eigenvalues, which should not be mistaken for useful geometry.

Adding a motion prior makes the optimisation solvable but does not make the missing direction
observed. Record data-only and prior information separately. Scale metres and radians with a stated
characteristic length before comparing eigenvalues or condition numbers. Rank alone is invariant
to nonsingular scaling; numerical thresholds are not.

Point-to-point fixed correspondences can manufacture apparent tangential information on resampled
surfaces. Rematching makes naive inverse-Hessian covariance particularly unreliable. The literature
distinguishes that case from point-to-plane covariance under stated assumptions.
[Bonnabel, Barczyk, and Goulette](https://arxiv.org/html/1410.7632v3)

Noise-aware degeneracy tests explicitly consider point and normal noise. They are a later candidate,
not a claim that our small Hessian threshold already implements them.
[Hatleskog and Alexis](https://arxiv.org/html/2410.10784v2)

### 5.3 Minimum change to the three-day tracker

Keep point-to-point fitting if necessary, but perturb and re-match along XY/yaw directions to test
whether the optimum is stable beyond fixed correspondences. Compare several bounded initial yaw
hypotheses. Add a coarse surface-rank diagnostic where normals have sufficient support. Report weak
modes, loss gaps, and overlap; do not export an inverse-Hessian matrix as validated uncertainty.

If the direction tests disagree or pose alternatives remain close, keep the predicted weak
component, inflate its uncertainty, and mark it prior-dominated. A confidently rendered trail must
not result solely from the regulariser. The raw observations stay available for later refinement.

## 6. Temporal priors, smoothing, and correlated shape memory

Use capture-time intervals. A Cartesian CV prediction and a local angular-rate prediction are
separate possible components: \(c^-_t=c_{t-1}+v_{t-1}\Delta t\) and
\(\psi^-_t=\psi_{t-1}+\omega_{t-1}\Delta t\). The rate may be weakly known; do not assume a turning
model is already implemented. Specify process noise and gap handling in time units.

For a continuous white-acceleration position/velocity model, one-axis discretisation is:

$$
Q(\Delta t)=q\begin{bmatrix}
\Delta t^3/3&\Delta t^2/2\\
\Delta t^2/2&\Delta t
\end{bmatrix}.
$$

This is a proposed model with stated assumptions, not a description of today's diagonal process
noise. A per-frame motion average must similarly account for cadence if used at different rates.

### 6.1 Why the old rejection band can reject a smooth turn

For constant true angular increment \(d\) and an always-accepting angular EMA with gain \(a\), the
steady post-update lag is \((1-a)d/a\), and pre-update innovation is \(d/a\). With \(a=0.08\),
\(d=5\) degrees, these are 57.5 and 62.5 degrees. Five degrees per accepted update is enough to enter
the old 60–120-degree rejection band without a single 90-degree physical jump.

At five accepted updates/s, this is a 25-degree/s turn. The calculation assumes an unwrapped local
angle ramp and acceptance before rejection; it is a counterexample to the guard rationale, not a
fit to the named recording. The guard can stay trapped while measurements remain in its rejection
band. It is not mathematically guaranteed to remain locked forever under every subsequent motion.

The root branch's forced release changes that behaviour, but a counter is not a visibility model.
Repeated releases can create snapping, and output smoothness cannot decide whether it is correct.

### 6.2 Shape-cache reuse is not independent evidence

Align using a frozen pre-update cache, then update the cache only after acceptance. Do not insert
the current points first and score them against themselves. If a recent scan appears in both the
short window and the map, deduplicate it or cap its combined weight; two cost terms do not create
two independent measurements. Retain sample provenance and pose revisions for every contribution.

For equal-variance errors with common pairwise correlation \(r\), the variance of their mean is
\(\sigma^2[1+(n-1)r]/n\). An illustrative effective count is
\(n_{\rm eff}=n/[1+(n-1)r]\): 100 repeats with \(r=0.9\) carry about 1.11 independent samples.
This illustration is not a fitted correlation model for our sensor. It disproves unconditional
\(1/\sqrt n\) confidence growth as a geometry-learning rule.

Do not pass a registration posterior that already contains a motion prior into the same motion
filter as an independent measurement. Either perform one combined update, or pass only an
appropriately derived data likelihood with declared dependence. This integration decision belongs
in the estimator interface, not in an undocumented confidence multiplier.

## 7. Dimensions, shape priors, and classification

### 7.1 Partial extent is censored and noisy

Under correct membership, known axes, and negligible noise, a partial span \(b\) constrains physical
dimension \(d\) by \(d\ge b\). Wrong axes, merged objects, and range noise break a hard bound.
An illustrative likelihood for a reported lower-bound event, with Gaussian bound uncertainty, is:

$$
P(\text{lower-bound event}\mid d)=
\Phi\!\left((d-b)/\sigma_b\right).
$$

It becomes nearly constant when \(d\) is comfortably above \(b\): the thin side return then supplies
almost no information about full width. This is an event likelihood, not a complete density for
the distribution of observed spans. A production model must specify how the event and its noise
are generated, or retain an explicitly approximate interval constraint.

When actual physical endpoints span a dimension and are reliably observed, a two-sided measurement
is possible. Both opposing opaque faces need not be simultaneously visible, and ordinarily cannot
both face a single external sensor on a convex box. Prefer endpoint/edge support tests over that
older face-count criterion.

A running mean of partial widths shrinks the car. A raw maximum ratchets upwards on contamination.
An 80th percentile plus hard lower/upper clamps is a heuristic, not a derived Bayesian update.
If lower-bound evidence exceeds the class range, report model/evidence conflict: do not silently
pick whichever clamp runs last. A class prior can be wrong and must remain revisable.

### 7.2 Shape uncertainty affects pose uncertainty

For a known side normal, let the measured side offset be \(b=c_y-W/2\). Then
\(c_y=b+W/2\), and under independent errors:

$$
\operatorname{Var}(c_y)=\sigma_b^2+\tfrac14\sigma_W^2.
$$

With \(\sigma_b=0.05\) m and \(\sigma_W=0.4\) m, centre uncertainty is 0.206 m, not 0.05 m.
Correlation adds a covariance term; uncertain heading adds further error through its Jacobian.
Option A's separate motion, orientation, and geometry beliefs therefore need uncertainty propagation.
Separate storage does not imply independent errors.

A class-prior sigma is not an immutable lower floor on posterior sigma. Independent informative
views can improve it. Floors should represent irreducible calibration/model error and unresolved
directions; repeated partial views alone do not justify reducing them.

### 7.3 Class scores remain inspectable, not self-confirming

For class \(k\), a small JSON model can store feature centres \(\mu_{kj}\), scales \(s_{kj}\), and
weights. Score valid features by robust normalised residuals and expose each contribution. Require
minimum discriminative support, not merely a low average over whatever features remain. Report
abstention and compare classes on a compatible feature domain.

Condition or stratify descriptors by range, aspect, and support. Whole-cloud eigenvalues describe
the observed subset, not intrinsic full-object shape. Normals and finite plane support aid
registration; a global eigenvalue ratio does not identify point correspondence. Treat intensity as
sensor/view dependent. Fit on human-mask objects, then separately assess predicted-mask performance.

Avoid feeding a class-derived dimension estimate back as independent evidence for the same class.
For the demo, seed geometry remains a stated prior, class scoring is a separate readout, and a poor
class match does not delete the track. Multi-part templates and stronger class/shape coupling follow
only after the held-out surface experiment supports them.

## 8. Proposed declarations for aligned implementation

These declarations govern the proposed next experiment, not existing production behaviour.

| ID  | Declaration                                                                                                          |
| --- | -------------------------------------------------------------------------------------------------------------------- |
| R1  | Physical object identity, recorded point membership, and predicted track identity are separate                       |
| R2  | Shape is body-local; aspect is derived from pose and sensor geometry                                                 |
| R3  | Directed yaw, axial orientation, and course are different variables with explicit wrap rules                         |
| R4  | Partial spans are censored evidence; physical dimensions are not raw OBB averages                                    |
| R5  | Membership and face hypotheses remain revisable; a hard shape gate must not erase visible fragments                  |
| R6  | Data observability is reported separately from prior regularisation and numerical convergence                        |
| R7  | Shared points, cached poses, class priors, and motion priors are not counted twice as independent evidence           |
| R8  | Covariance requires stated noise, correlation, linearisation, and geometry assumptions; otherwise report diagnostics |
| R9  | Time, units, reference origin, ground approximation, and calibration are part of every estimator contract            |
| R10 | Classification scores, model parameters, and bounded accumulated point data are separate products                    |
| R11 | Causal predictions cannot consume future masks or test labels; assisted corrections are counted separately           |
| R12 | Reduced jitter, repeated-run agreement, and attractive shape overlays are not substitutes for held-out accuracy      |

## 9. Experiments that could falsify the design

| Experiment                         | Controlled change                                                     | Failure this must expose                                                |
| ---------------------------------- | --------------------------------------------------------------------- | ----------------------------------------------------------------------- |
| Straight pass with changing aspect | Fixed body shape/yaw; ray-sampled visible surfaces vary               | Apparent acceleration or shrinking from changing surface support        |
| Long single side                   | Slide sampling window along one plane                                 | Low residual with unsupported along-side pose certainty                 |
| Side plus genuine end edge         | Add independently resolved finite boundary                            | Whether the previously weak mode becomes observable                     |
| Circular turn and reverse          | Vary yaw rate and reference-point offset; retain signed velocity      | Course prior forcing wrong body yaw or front/rear flip                  |
| Axis ambiguity                     | Near-square support, 180-degree sign changes, 90-degree relabellings  | Averaging incompatible hypotheses into a fictitious pose                |
| Repeated evidence                  | Duplicate identical scan contributions                                | Confidence improving without new information                            |
| Cache contamination                | One wrong association, then remove its revision                       | Irreversible shape corruption or unchanged derived results              |
| Dimension conflict                 | Wrong class prior or one merged cluster                               | Silent hard clamp rather than model/evidence conflict                   |
| Timing and coordinates             | Change cadence, global rotation, and unit representation consistently | Frame-rate-dependent prior strength or coordinate-dependent motion loss |
| Raw versus oracle masks            | Same source domain, different membership provenance                   | Human segmentation hiding a tracking failure                            |

Use synthetic shapes with known pose to test observability and coverage, then held-out physical
objects from the site. Report mask precision/recall, centre and axial-yaw error where reference is
valid, time-to-loss, reacquisition/interventions, turn lag, and held-out visible-surface fit.
For data-constrained directions, assess stated uncertainty coverage; never score a null direction
as precisely measured. Registration diagnostics alone cannot certify calibrated uncertainty.

Evaluate free-space contradictions only along reliable recorded rays, with range margin and timing
appropriate to the measurement. Do not penalise an unobserved backside as a false surface. Shape
reconstruction evaluated against its own fused points is a consistency display, not ground truth.

Repeat both before and after PCAP arms, varying run order and reporting spread. Two after runs do
not estimate a full noise distribution or prove frame assembly is the only nondeterministic stage.
Exact annotation replay uses immutable arrays and masks even if full pipeline recomputation varies.

## 10. Numerical checks performed for this note

Eight independent arithmetic checks were executed during this review. These are equation checks,
not a sensor simulation, tracker test suite, or reproduction of the root branch's A/B experiment.

| Check                  | Inputs                                                               | Verified result                                           |
| ---------------------- | -------------------------------------------------------------------- | --------------------------------------------------------- |
| EMA ramp               | Gain 0.08, 5 degrees/update, 1,000 unwrapped updates                 | Innovation 62.5 degrees; post-update lag 57.5 degrees     |
| Reference-point course | Speed 4 m/s, yaw rate 0.35 rad/s, 2 m forward offset                 | Course difference 9.9262455 degrees                       |
| Side-plane information | Unit weights, five points at longitudinal coordinates -2 through 2 m | Data information diagonal 0, 5, 10; translation null mode |
| Circular residual      | 179 and -179 degrees                                                 | Shortest difference magnitude 2 degrees                   |
| Axial residual         | 179 and 1 degrees                                                    | Axis difference magnitude 2 degrees                       |
| Centre uncertainty     | Independent plane sigma 0.05 m and width sigma 0.4 m                 | Centre sigma 0.2061553 m                                  |
| Correlated repeats     | 100 samples with equal pairwise correlation 0.9                      | Effective count 1.1098779                                 |
| Aspect-lock inequality | Shape ratio 0.20                                                     | Locks under threshold 0.25, not under 0.15                |

Promote these into fixture-backed tests before implementation. The research note records their
inputs and expected results so the checks do not depend on session scratch files.

## 11. Document revision register

| Document                                | Outdated assumption                                                                                                    | Revision/declaration                                                                      |
| --------------------------------------- | ---------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------- |
| Original D-04, Sections 2–3             | Gaussian extent averages, count-only sigma shrinkage, scalar angle blending form a full Bayesian shape model           | Mark as historical heuristic; R2–R8 replace these assumptions for new work                |
| OBB review, Fix B/D and replay guidance | Raw dimensions plus smoothed angle guarantee containment; smaller aspect threshold locks more; VRLOG reruns perception | Correct geometry guarantee, threshold direction, and replay boundary                      |
| State-est, Sections 5 and 9             | Separate beliefs imply independence; opposing faces must be visible together; quantile/clamp recipe is probabilistic   | Propagate uncertainty, use endpoint support, and declare censored evidence/model conflict |
| Shape-descriptors plan                  | Stable descriptor from retained points necessarily represents stable vehicle geometry                                  | Declare support/aspect dependence and separate registration observability                 |
| Annotation plan                         | Membership can be reused as correspondence or future pose evidence                                                     | R1/R11 preserve per-frame evidence and evaluation boundaries                              |
| Single-site demo                        | Low residual and a regulariser suffice for a credible trail                                                            | Add direction tests, cache de-duplication, and prior-dominated output                     |
| Maths hub and pipeline Q8               | D-04 has no upstream dependencies and quantified expected gains                                                        | Distinguish box-only heuristic from surface model; gains remain hypotheses                |
| Root heading sprint D2.1/D2.2           | Mean dimensions suffice for likelihood; fragments should match full size                                               | Copy-ready changes below; root agent's document is not edited here                        |

### 11.1 Copy-ready declarations for the active root sprint

- **D2.1:** Select rectangle representations against a revisable, uncertainty-bearing geometry
  belief. Do not promote running raw extent means to physical dimensions. Retain ambiguity when
  alternatives cannot be separated by evidence.
- **D2.2:** Compare observations with expected visible support. Distinguish membership acceptance
  from dimension-update acceptance, and measure duplicate identities separately from proximity.
- **D1.4:** Publish a coherent observed envelope or a coherent estimated physical box, explicitly
  identified. Reprojecting an old box can inflate the envelope and is not shape reconstruction.
- **D2.3:** Keep course alignment diagnostic. Optimise a predeclared scorecard including reference
  pose/membership, loss/interventions, and support, not course agreement alone.
- **Day 1 interpretation:** Preserve the reported results, but call them proxy-metric changes from
  the recorded experiment. The table does not demonstrate the below-25-degree target, zero trapped
  tracks, or identity preservation. Increased fragmentation is a tradeoff to measure, not success
  by definition. A shorter maximum lock does not prove every conditional trap disappeared.
- **Runtime readiness:** The replay harness is now committed at `6a9b6ebf6`; verify its current
  parity, provenance, and nondeterminism before using it for new acceptance claims.

No historical DEVLOG entry or production default is changed by these declarations.
