# Facet registration: mathematical review, prior work, and the sparse tail

This note reviews the [facet registration experiment plan][plan] as an estimation problem. It
asks what a persistent surface fragment can observe, what the prior literature already settles,
where the proposal's expected gains cannot come from, and which approaches have a better claim on
the cases the plan cares about most: vehicles that are far away, partly occluded, or sampled by a
handful of returns. It ends with the side effects to expect and the changes made to the plan.

- **Status:** Research review; derivations and arithmetic checks, no implementation or measured result
- **Layers:** L1 timing, L3 visibility, L4 geometric evidence, L5 estimation, L8 following evaluation
- **Reviews:** [Facet registration experiment plan][plan], [facet annotation minimum spec](../../../docs/plans/lidar-facet-annotation-minimum-spec.md)
- **Related:** [Visibility-aware tracking maths](20260905-visibility-aware-object-tracking-research.md), [near-edge tracked state](../../../docs/plans/lidar-near-edge-tracked-state-plan.md), [October near-edge campaign](../../../docs/lidar/operations/near-edge-campaign-2026-10.md), [state estimation plan](../../../docs/plans/lidar-state-estimation-plan.md), [tracking maths](../tracking-maths.md)

[plan]: ../../../docs/plans/lidar-facet-registration-experiment-plan.md

## 1. Summary

The plan is careful about evidence and controls, and its own Section 1 already names the limit:
more patches on a visible side cannot create a measurement of an unseen bumper. This review makes
that limit quantitative and extends it to the sparse tail, where it is decisive.

1. **A facet is a constraint along its normal and nothing else.** Every patch on one plane shares
   the same null direction. Four patches on a side have the same rank as one (Section 4.2). The
   constellation hypothesis (arm D) therefore cannot add information in the along-face direction
   unless a patch has a non-parallel normal or a physically resolved edge. Those two things are
   what the near-edge model and an L-shape fit already use.
2. **Localisation along the normal is already solved.** Eight returns at normal incidence place a
   face plane to 7 mm (Section 4.5). The half-extent prior that converts a face into a centre
   carries 0.2 m. The centre error is the prior's, and no density of patches on the same face
   reduces it. The only remedies are extent evidence, a second face, or honest intervals.
3. **In the sparse tail the limiting errors are face identity, the unobserved extent, membership,
   and occlusion boundaries mistaken for edges** (Section 5). Persistent patch identity attacks
   none of them. What does: body-local memory of the shape learned while the vehicle was dense
   (the plan's arm B, properly specified), a visibility model that turns the absence of returns
   into evidence (Section 5.4), multi-hypothesis face identity, and censored-interval output.
4. **Repeatability is the wrong first test for H1.** A sampling boundary repeats as well as a
   physical edge while the viewing geometry changes slowly. E1 must label each extent end as
   physical, occluded, field-of-view, or dropout, and score the labeller, or a passing E1 proves
   nothing about geometry (Section 6).
5. **Cost is the side effect most likely to end the experiment.** The October campaign measured
   the near-edge tracked arm at 3.5 to 8 ms p99 per tracker update on the M1 Pro, 15 to 35 times
   the production tracker and already above the state plan's 3 ms gate. Any facet arm does
   strictly more work per candidate pair (Section 8).

The recommendation is to keep arm D as a bounded challenger for mid-pass lateral steadiness, where
the near-edge campaign's face-transition tail lives, and to make arm B plus a shared visibility
component the candidate for the sparse tail. The plan has been revised accordingly (Section 10).

Four kinds of statement appear below, following the visibility-aware note: **source result**
(attributed to a paper), **derived result** (follows from stated equations, checked in Section 9),
**design proposal** (a choice still needing experiment), and **repository observation** (what the
inspected code or document says).

## 2. The proposal as an estimation problem

The plan estimates planar translation and body yaw, `x = (c_x, c_y, psi)`, for one rigid vehicle
under a road assumption, from returns `y_i` that are samples of visible surfaces. A facet `f` is a
body-local primitive, a line fragment or planar patch, with support that changes every scan. The
five hypotheses reduce to three estimation questions:

| Hypothesis | Estimation question                                                                                     |
| ---------- | ------------------------------------------------------------------------------------------------------- |
| H1         | Does a primitive's fitted geometry depend on the body, or on where the beams happened to strike?        |
| H2         | Does the primitive representation increase the Fisher information about `x`, or reduce bias, per frame? |
| H3         | Does it improve the identity of the two endpoints a following gap is measured between?                  |

H4 and H5 are retrieval and reconstruction questions and are not reviewed here beyond their side
effects (Section 8).

The honest form of H2 is a comparison of information matrices and biases, not of trail smoothness.
Section 4 computes what a facet contributes to the information matrix. Section 5 computes how
much information is available at all in the sparse tail, and from which sources.

## 3. Prior work

The plan cites SOTracker, ET-PMHT for convex polytopes, and Gaussian-process extended-object
tracking. The literature below is older, closer to a static roadside sensor, and settles several
of the plan's open questions. Sources were checked against their publication records; see
Section 11 for full references and `data/maths/references.bib` for BibTeX entries.

| Work                                                                                      | What it supplies                                                                                                                                                     | What it does not                                                                 | Bearing on the plan                                                                                           |
| ----------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------- |
| Petrovskaya and Thrun 2009 [Petrovskaya2009]                                              | One Bayes filter per vehicle over pose and box geometry; anchor points couple shape to motion; a virtual scan gives a ray-cast likelihood with free space            | A moving platform and a 2D scan summary; no shape memory beyond a box            | The anchor-point idea is the plan's body origin; the ray likelihood is Section 5.4                            |
| Granström, Lundquist, Orguner 2011 [Granstrom2011]                                        | A rectangle measurement model for laser scanners: predicted measurements along the visible sides, with innovation covariance, inside an EKF/PHD tracker              | Sparse returns and self-occlusion as such; no shape beyond the rectangle         | The near-edge model is a special case; use it as the reference form for arm A's likelihood                    |
| Wyffels and Campbell 2015 [Wyffels2015]                                                   | Negative information: the absence of detections updates the density of occluded extended objects, existence, and association                                         | Shape detail; a dense multi-object scene                                         | The mechanism Section 5.4 proposes for the sparse tail                                                        |
| Held et al. 2014, 2016 [Held2014; Held2016]                                               | A tracker whose measurement model accounts for sensor resolution and uses every return against an accumulated model; accuracy degrades gracefully to very few points | Rigid primitives; a static sensor is a special case of its moving one            | The strongest evidence that body memory, not patch identity, is what helps when points are few                |
| Kraemer, Stiller, Bouzouraa 2018 [Kraemer2018]                                            | Polyline shape estimation for lidar vehicle tracking with explicit free-space information                                                                            | A roadside geometry; cost on a Pi                                                | Polylines are the plan's line fragments with free space added; the closest published analogue of arm C plus V |
| Pang, Li, Wang 2021 [Pang2021]                                                            | Registration plus accumulated body shape plus motion prior from a supplied initial box                                                                               | Calibrated uncertainty; a static sensor                                          | Arm B's inspiration, as the plan says                                                                         |
| Zhang, Xu, Dong, Dolan 2017 [Zhang2017]                                                   | Efficient L-shape fitting: the two visible faces and their corner from one scan                                                                                      | Temporal identity; uncertainty                                                   | The corner is the only cheap source of tangential information (Section 4.4)                                   |
| Segal, Haehnel, Thrun 2009 [Segal2009]                                                    | Plane-to-plane registration with per-point covariance from local normals                                                                                             | Covariance of the pose estimate                                                  | The residual form arms B and D should share                                                                   |
| Zhang and Singh 2014 [Zhang2014]                                                          | Edge and planar feature selection by curvature within a scan line, point-to-line and point-to-plane residuals                                                        | Moving objects; the features are for ego-motion                                  | The extractor E1 describes, with its scan-line assumption made explicit (Section 4.8)                         |
| Censi 2007; Brossard, Barrau, Bonnabel 2020 [Censi2007; Brossard2020]                     | Closed-form and unscented ICP covariance; shows naive inverse-Hessian covariance is optimistic and that bias and initialisation dominate                             | A rank-deficient object registration                                             | Why the plan's "declared uncertainty approximation" must be scored, never assumed                             |
| Zhang, Kaess, Singh 2016; Tuna et al. 2024 [Zhang2016; Tuna2024]                          | Degeneracy detection and partial solving along well-conditioned directions; per-iteration localisability categories                                                  | Object tracking                                                                  | The engineering form of the plan's "record data-only observable directions"                                   |
| Bonnabel, Barczyk, Goulette 2016; Hatleskog and Alexis 2024 [Bonnabel2016; Hatleskog2024] | ICP covariance under rematching; noise-aware degeneracy tests for point-to-plane                                                                                     | Our sensor                                                                       | Already cited by the visibility-aware note; the spurious-eigenvalue test in Section 4.3                       |
| Granström, Baum, Reuter 2017 [Granstrom2017]                                              | Survey of extended-object tracking: random matrices, star-convex shapes, multi-object filters, lidar applications                                                    | Sparse-tail guidance                                                             | The map of comparators the plan can choose from                                                               |
| Wahlström and Özkan 2015; Kumru and Özkan 2021 [Wahlstrom2015; Kumru2021]                 | Gaussian-process shape with confidence intervals, 2D and 3D                                                                                                          | Rigid box priors; real-time cost on a Pi                                         | The plan's curved-body comparator; keep it as such                                                            |
| Xia et al. 2021 [Xia2021]                                                                 | A hierarchical truncated Gaussian measurement model whose truncation bounds are learned from data                                                                    | Lidar; the truncation is a spatial distribution, not an explicit occlusion model | A learned alternative to the plan's visibility mixture, if the hand-built one fails                           |
| Scheel and Dietmayer 2019 [Scheel2019]                                                    | A variational radar measurement model learned from data inside a random-finite-set tracker                                                                           | Lidar                                                                            | Same role as the row above                                                                                    |
| Rieken and Maurer 2016 [Rieken2016]                                                       | Scan-timing compensation for scanning sensors in grid and object models                                                                                              | Within-object deskew for a static sensor                                         | Sizes the timing effect; Section 4.7 shows it is second order here                                            |
| Mannari et al. 2025 [Mannari2025]                                                         | ET-PMHT for partially visible convex polytopes; adds faces as they become visible                                                                                    | Close-vehicle association                                                        | As the plan says: closest to the surface/visibility model                                                     |

**Repository observation.** Nothing in this list uses persistent identities of small patches with
a relational graph as the pose measurement. The published lidar vehicle trackers that handle sparse
returns best (Held; Kraemer; Pang) all use dense accumulated shape or free space, or both. That is
not proof the constellation cannot work, but it is a prior worth stating before spending F1 and F2.

## 4. What a facet observes

### 4.1 One plane patch: a rank-one constraint

For a body-local point `q_j` on a patch with outward normal `n` (in the site frame after rotation),
the surface residual and its Jacobian with respect to `(delta c_x, delta c_y, delta psi)` are the
visibility-aware note's Section 5.2:

$$
r_i(x)=n_i^\top\bigl(R(\psi)q_j+c-y_i\bigr),\qquad
J_i=\bigl[n_{ix},\ n_{iy},\ n_i^\top R(\psi)Jq_j\bigr],\qquad
J=\begin{bmatrix}0&-1\\1&0\end{bmatrix}.
$$

With unit weights the data information is `H = sum J_i^T J_i`. For a side face at `y = b`, yaw
zero, normal `(0, 1)` and support at `u in {-2, -1, 0, 1, 2}` m:

$$
H_{\rm side}=\begin{bmatrix}0&0&0\\0&5&0\\0&0&10\end{bmatrix}.
$$

Translation along the face is a null direction. This is the note's result and the anchor for the
checks below (derived result, Section 9 check 1).

### 4.2 More patches on the same surface add no tangential information

Split the same side into four patches of five returns each, twenty returns in all, with the same
normal. The first entry of every Jacobian row is still `n_x = 0`, so `H e_x = 0` whatever the
patch count:

$$
H_{\rm 4\ patches}=\begin{bmatrix}0&0&0\\0&20&0\\0&0&29.47\end{bmatrix},\qquad \operatorname{rank}=2.
$$

The constellation's graph of relationships between patches on one plane (adjacency, separation,
relative orientation) is a statement about the body frame, not about the data. It can regularise
a fit; it cannot observe the tangent. A patch on a non-parallel surface can: three returns on a
rear face at `x = -2.25` add `H_xx = 3` and make the problem full rank (check 1). So the useful
content of "a small graph of rigid geometric relationships" is exactly the pair of non-parallel
faces the near-edge model already measures, plus whatever physical edges exist (Section 4.4).

**Design proposal.** E1 must include this as a must-pass test on synthetic data: adding patches
on one plane leaves the data-only tangential eigenvalue at zero. If an implementation reports
otherwise, it is counting fixed correspondences or noisy normals as information.

### 4.3 Noisy normals manufacture information, and biased normals manufacture bias

Perturb each of the twenty normals by an independent angle `eps_i` uniform on plus or minus one
degree. The tangential entry becomes

$$
H_{xx}=\sum_i \sin^2\varepsilon_i \approx N\,\mathbb{E}[\varepsilon^2]=20\cdot\frac{(\pi/180)^2}{3}=0.0020,
$$

measured as 0.0024 in check 1. A naive inverse-Hessian would report a tangential standard
deviation of `sigma_n / sqrt(0.0024)`, about 0.4 m for 2 cm normal noise, for a direction the
data do not constrain at all. If instead every normal tilts the same way because the body yaw is
wrong by `eps`, the entry is `N sin^2(eps)` and the tangential estimate moves with the common
error: a bias presented as a measurement. This is the mechanism behind the visibility-aware
note's warning about rematching, and why Bonnabel et al. and Hatleskog and Alexis exist. A
small-eigenvalue threshold scaled by a stated noise level, as Hatleskog and Alexis derive for
point-to-plane, is the right test; a raw condition number is not.

### 4.4 A corner is worth more than a face, if it is physical

A face constrains one direction. A corner, where two non-parallel faces meet, constrains two. A
physical end of a face with no adjacent face visible, such as the rear of a flat truck side, still
localises the tangent to within one azimuth sample, because the last return lies within one beam
step of the edge:

| Range | Azimuth step `r delta` | Tangential sigma of a resolved edge, `r delta / sqrt(12)` |
| ----: | ---------------------: | --------------------------------------------------------: |
|  20 m |                 0.07 m |                                                    2.0 cm |
|  30 m |                 0.10 m |                                                    3.0 cm |
|  50 m |                 0.17 m |                                                    5.0 cm |

A sampling boundary carries none of this. Asserting a boundary as an edge when a quarter of a
4.5 m side is hidden biases the tangent by `f L / 2 = 0.56 m`; half hidden, 1.1 m (check 8). So
the entire value of an "edge" facet is the classifier that says whether its end is physical. The
plan states that mask boundaries are not assumed physical but supplies no labeller. Section 5.4
supplies one from evidence the pipeline already has.

### 4.5 Localisation along the normal is not the limiting error

With `N` returns at normal incidence and range accuracy `sigma_r = 0.02 m`, the Cramér-Rao bound
on the plane offset is `sigma_r / sqrt(N)`:

| `N` | Plane sigma |
| --: | ----------: |
|   3 |     11.5 mm |
|   8 |      7.1 mm |
|  20 |      4.5 mm |
| 100 |      2.0 mm |

The face enters the centre through the believed half-extent, whose class-prior sigma is 0.4 m on
the dimension and 0.2 m on the half-extent. At the near-edge model's support floor of eight
returns the prior term is 28 times the plane term (check 3). The near-edge campaign's within-run
residuals and face-entry tails are consistent with this: the body is steady while the faces are
fixed and jumps by a half-extent error when a face enters. Patches cannot shrink the 0.2 m; only
extent evidence can, and extent evidence comes from spans, corners, and memory, not from surface
density.

### 4.6 Yaw from a strand

A single straight strand of `M` returns over length `l`, each with noise `sigma_n` along the face
normal, bounds yaw by

$$
\sigma_\psi=\frac{\sigma_n}{\sqrt{\sum_i (u_i-\bar u)^2}}\approx\sigma_n\sqrt{\frac{12}{M\,l^2}}.
$$

With `sigma_n` from the range term and the azimuth quantisation projected by incidence (check 4):

| Range | Face | Incidence | `M` | `sigma_n` | `sigma_psi` |
| ----: | ---- | --------: | --: | --------: | ----------: |
|  20 m | side |        0° |  64 |     20 mm |       0.11° |
|  50 m | side |       60° |  25 |     45 mm |       0.38° |
|  50 m | end  |        0° |  10 |     20 mm |       0.63° |
|  80 m | end  |        0° |   6 |     20 mm |       0.76° |
|  80 m | side |       70° |  16 |     76 mm |       0.79° |

Geometric yaw precision is under one degree everywhere a face reaches the support floor. The
near-edge span search already tolerates ten degrees of axis error, and the campaign's turning
tail came from a heading that lags by far more than a degree. The sparse-tail yaw problem is not
precision. It is which face the strand belongs to, which way the body points, and how the
estimator blends geometry with course. Bumper curvature and wheel arches violate the straight
model at the few-centimetre level and bound the error at a few degrees; the same strand on a
curved shell is an E1 case, not a reason to reject the bound.

### 4.7 Scan timing inside a cluster is second order for the tracker, first order for a near patch

The sensor sweeps 360 degrees in 100 ms at 10 Hz, 0.28 ms per degree. A 4.5 m vehicle subtends
26 degrees at 10 m and 5 degrees at 50 m, so one cluster's returns span:

| Range | Angular width | Sweep inside the cluster | Smear at 10 m/s | Smear at 20 m/s |
| ----: | ------------: | -----------------------: | --------------: | --------------: |
|  10 m |         25.8° |                   7.2 ms |          7.2 cm |         14.3 cm |
|  20 m |         12.9° |                   3.6 ms |          3.6 cm |          7.2 cm |
|  50 m |          5.2° |                   1.4 ms |          1.4 cm |          2.9 cm |

Two clusters at opposite azimuths differ by up to 100 ms, 1 m at 10 m/s, and the tracker already
keys every prediction on per-cluster capture time (repository observation:
`internal/lidar/l5tracks/time_domain.go`). The cluster's capture instant is its first member's
acquisition time (`internal/lidar/l4perception/cluster.go`), which is within the sweep above of
any member. Per-return times exist, with a per-channel firetime correction in the packet parser.
So the plan's timing requirement is sized as follows: the filter's instant is fine as it is; a
body-local patch accumulated at close range must deskew, because 7 cm is ten times the plane
sigma of Section 4.5; beyond 50 m the smear and the plane sigma are the same order and deskew
buys little. Rieken and Maurer's analysis is the general case; this is its static-sensor
specialisation.

### 4.8 Ring strands are straight in plan and curved in elevation

A constant-elevation ring is a cone about the sensor's vertical axis. Its intersection with a
vertical face at perpendicular distance `d` is a hyperbola in the face: height `z(u) = h +
sqrt(d^2 + u^2) tan(e)` along the face coordinate `u`. Its plan-view trace is a straight line.
The sag across a 4.5 m face (check 12):

| Distance `d` | Ring elevation | Sag across the face |
| -----------: | -------------: | ------------------: |
|          5 m |           -15° |              0.13 m |
|          5 m |         -24.6° |              0.22 m |
|         10 m |           -15° |              0.07 m |
|         20 m |            -5° |              0.01 m |

At the deployed 2.3 m mount seven rings, from -9.6 to -24.6 degrees, strike a 1.5 m face at 5 m,
each with its own sag; at 3 m only the two steepest would. A three-dimensional straight-line fit to
such a strand, or a planarity test on a patch spanning rings, would reject a flat door panel near
the sensor. E1's line fragments must be fitted in plan view, or against the cone, and the
non-planarity threshold for patches must allow this known sag. This is a repository-specific
consequence of the 2.3 m mount and the Pandar40P ring table, not a property of lidar in general.

## 5. The sparse tail

### 5.1 Return budget on the deployed ring table

A 4.5 by 1.8 by 1.5 m box on a flat road, sensor 2.3 m up (the deployed height), azimuth step 0.2
degrees, the real Pandar40P elevation table (check 2). The first version of this table assumed a
3 m mount. At 2.3 m the ring counts change at 10, 20 and 100 m. From 30 to 80 m a different set of
rings strikes the body, but as many of them, so the returns are unchanged; the largest gap moves
by at most 3 mm, enough to round 80 m's from 0.48 to 0.47 m.

| Range | Rings on the body | Returns per ring, side | Returns per ring, end | Side returns | End returns | Largest ring gap on the face |
| ----: | ----------------: | ---------------------: | --------------------: | -----------: | ----------: | ---------------------------: |
|  10 m |                11 |                    129 |                    52 |        1,418 |         567 |                       0.21 m |
|  20 m |                11 |                     65 |                    26 |          709 |         284 |                       0.12 m |
|  30 m |                 8 |                     43 |                    17 |          344 |         138 |                       0.18 m |
|  40 m |                 6 |                     32 |                    13 |          193 |          77 |                       0.24 m |
|  50 m |                 5 |                     26 |                    10 |          129 |          52 |                       0.30 m |
|  60 m |                 4 |                     22 |                     9 |           86 |          34 |                       0.36 m |
|  80 m |                 3 |                     16 |                     6 |           48 |          19 |                       0.47 m |
| 100 m |                 2 |                     13 |                     5 |           26 |          10 |                       0.59 m |

The table counts every ring from the road to the roof. That matches the live height band on level
road at 2.3 m, whose -2.8 m floor sits below the road and clips nothing. The slope-aware surface
clip, with its floor 0.2 m above the fitted surface, would remove one ring at 10, 30, 40 and 60 m:
end returns at 60 m fall from 34 to 26, and check 7's expected support there from 24.1 to 18.0,
still above eight. On a down-slope the absolute band removes far more; at Marina's grade its floor
is 0.8 m above the road 30 m down. The first version had the same premise: at 3 m the band's floor
sat 0.2 m above the road and was not counted either.

Three things follow. The number of rings on the body peaks where the dense band of the table,
which sits around the horizon, crosses the body: by 10 to 20 m at this mount, with 11 rings (at
3 m it crossed between 20 and 40 m). Beyond that the count falls with range, apart from
single-ring steps up as the next ring enters. The
ring gap stays below the DBSCAN `foreground_dbscan_eps` of 0.8 m to 100 m, so vertical ring
spacing does not split a vehicle at the ranges the corpus covers; membership failures at range
have other causes. And the near-edge support floor of eight returns is marginal for an end face
beyond 60 m: with a 70 percent detection probability the expected end-face support at 80 m is
13.5 returns and the probability of falling below eight is 4 percent (check 7). The sparse tail
for this sensor and mount begins at about 60 m end-on and 80 to 100 m side-on, and at any range
when another object hides most of the body.

### 5.2 Where the error comes from at range

| Error source                                          | Order of magnitude                                           | Addressed by persistent facets (D)?           | Addressed by                                                               |
| ----------------------------------------------------- | ------------------------------------------------------------ | --------------------------------------------- | -------------------------------------------------------------------------- |
| Unobserved extent behind the visible face             | Half the dimension prior sigma, 0.2 to 0.4 m, one-for-one    | No: same plane, same prior                    | Memory of the dimension from the dense part of the pass; interval output   |
| Null direction along a lone face                      | Unbounded from data; prior-dominated                         | No: Section 4.2                               | A second face, a physical edge, free space, motion prior, memory           |
| Face identity: end or side, front or rear             | Discrete 90 or 180 degrees; a length-width swap              | No: patch identity presupposes the body frame | Multiple hypotheses scored on dimensions and course, collapsed by evidence |
| Membership: dropout, fragment, merge with a neighbour | A fragment's span understates; a merge inflates by a vehicle | Partly: a patch fit can reject gross outliers | Track-conditioned gathering around the prediction; free-space test         |
| Occlusion or field-of-view boundary read as an edge   | `f L / 2`: 0.56 m for a quarter of a side                    | No, unless the boundary is labelled           | Ray casting against the background model and nearer clusters               |
| Azimuth quantisation per return                       | `r delta / sqrt(12)`: 5 cm at 50 m, 8 cm at 80 m             | Not applicable                                | Per-return anisotropic noise, already specified in the state plan          |
| Plane localisation along the normal                   | 7 mm at eight returns                                        | Already negligible                            | Nothing needed                                                             |
| Yaw from geometry                                     | Under one degree                                             | Already adequate                              | Nothing needed; the policy that blends it with course is the lever         |
| Within-cluster scan timing                            | 1.4 cm at 50 m                                               | Not applicable                                | Existing capture time                                                      |

The first five rows are the sparse tail's error budget. None is reduced by subdividing a visible
surface into persistent patches. The table is the reason the state plan's Section 9.3 condition, that
residual error be attributable to edge localisation rather than the dimension prior, will almost
certainly fail in the tail: edge localisation is the smallest row.

### 5.3 Face identity is the ambiguity that matters

At 80 m end-on a vehicle offers about nineteen returns on one 1.8 m strand. Two hypotheses explain
them equally well: the end face of a 4.5 m car, or 1.8 m of a side whose remainder is hidden.
They differ by 90 degrees of yaw and a swap of length and width, and the strand cannot choose
between them. The near-edge model resolves this with the believed axis, from the tracked heading
or the course under remedy T3, and with the believed dimensions. In the tail the course is the
better of the two and the dimensions must be remembered from the pass. The visibility-aware
note's comparative score `s_h` is the right instrument; the plan already asks to preserve
alternative yaw and face hypotheses. The point here is that a persistent patch cannot be
assigned an identity until this choice is made, so patches cannot help make it.

### 5.4 Negative information: absence of returns is evidence, given visibility

The sensor is static and L3 keeps a per-cell range baseline. For each beam in the azimuth and
elevation span of a pose hypothesis, three outcomes are distinguishable: the beam returned from the
body, it returned from nearer (an occluder), or it returned from farther (background or free
space). The third outcome contradicts any hypothesis that places the body across that beam, with
a range margin from the baseline's variance. The second says nothing. The first is the ordinary
positive measurement.

This is the beam model of Petrovskaya and Thrun, the negative information of Wyffels and
Campbell, and the free-space term of Kraemer et al. Its value in the tail is specific: free space
is sampled at the same angular density as returns, so when a strand has six returns, the columns
on either side of it also have returns, from behind the body or from the road, and they bound the
body's extent in those columns. A strand end next to a background-return column is a physical
edge to within one azimuth step. A strand end next to a nearer-return column is an occlusion
boundary and carries no edge information. A strand end at the frame's azimuth limit is a
field-of-view boundary. That is the labeller Section 4.4 needs, and it uses only the background
model, the frame's own returns, and a box cast against a polar grid, whose cost is the product of
the azimuth and elevation spans, a few hundred operations per hypothesis.

It also gives an expected support count. With a pose, a shape, and the visible beams, the expected
number of returns on each face is a Poisson rate; a face that should have carried forty returns
and carried none is evidence about aspect or occlusion, not silence. The plan's observation that a
return model "alone does not model missing detections or return count" is answered by this term.

### 5.5 Membership at range

The cluster-level causes of tail failure are the five-point DBSCAN minimum, the eight-return face
floor, merges with a neighbouring vehicle in a queue, and the fragment guard, not the ring spacing
(Section 5.1). The remedy the plan already sketches in E3, revisable membership, has a concrete
form in the literature: gather returns inside the predicted body footprint, widened by its
covariance, before or instead of relying on the clusterer's partition, and let the free-space test
reject returns the hypothesis cannot explain. SOTracker crops around the predicted box; Petrovskaya
builds the virtual scan around the predicted pose. E3 should test this as a mechanism, not only
as a membership-revision policy.

### 5.6 Why body memory beats patch identity in the tail

A patch needs three non-collinear returns and an identity that survives the scan phase changing
every frame. In the tail a face has one to three strands and the identity question of Section 5.3
is open. A body-local accumulated shape uses every return against a model built when the same
vehicle carried three hundred to eight hundred returns per frame at 10 to 30 m. Held et al. show
a tracker of that form degrading gracefully to a handful of points when its measurement model
accounts for sensor resolution. Pang et al. show the accumulated shape stabilising the pose as
views change.

Arm D is a compression of arm B: a few primitives with relationships instead of a bounded map. Its
hypothesised advantages are memory, runtime, and an explicit relational structure. Those are
engineering claims and should be tested as such: D should match B in the tail at lower cost. The
plan's current framing, that D is tested "against A, B, and C" for a material gain, invites a
result in which D loses in the tail for the reasons above and the constellation idea is written
off for a case it was never suited to. The revised plan separates the two decisions.

## 6. Expected shortcomings of the plan as written

1. **E1's repeatability can be passed by sampling boundaries.** Under slowly changing geometry a
   dropout or occlusion boundary repeats as reliably as a corner. Without the labeller of
   Section 5.4 and a precision-recall score for it, "fragments repeat" does not establish
   "fragments are physical". The plan's clause that mask boundaries are not assumed physical
   needs a test.
2. **Arm B is under-specified.** "Robust point registration with bounded accumulated shape" cannot
   be matched for information against D until it has a stated map cap, provenance, per-return
   noise, residual form, cache freeze rule, and abstention rule. The revised plan states them.
3. **C and D differ in three things at once.** Extraction, persistent identity, and the relational
   graph all change between C and D. A D0 arm, persistent identities without the graph, separates
   the identity claim from the graph claim. The revised plan adds it as optional and requires that
   any D gain be attributed to one of the two.
4. **The H2 criterion sits in the direction a side patch cannot see.** Along-path endpoint error
   is dominated by the unseen extent (Section 5.2). The plan's Section 1 says so in prose; the
   scorecard does not. The revised plan adds an attribution gate between E1 and E2: decompose the
   baseline's along-path error per episode into extent-prior error, edge localisation, and filter
   lag, and fund E2 only if the share a facet can move is large enough to meet the 20 percent
   target.
5. **"A declared uncertainty approximation" will be optimistic.** Inverse-Hessian covariance from
   point-to-plane registration is known to understate error, especially with rematching and in
   weakly constrained directions (Censi; Bonnabel et al.; Brossard et al.). The plan's uncertainty
   row should require that prior-dominated directions are reported as such, never as a Gaussian
   sigma, and that G-UNC-1 is stratified by data-only rank.
6. **Cost is not modelled.** The campaign's measured near-edge tracked arm costs 3.5 to 8 ms p99
   per update on the M1 Pro, 15 to 35 times B0, with the measurement made per candidate pair.
   The plan's runtime row should state a per-pair cost model and a budget before F2, and decide
   whether a facet measurement is made per candidate pair or only per assigned pair.
7. **Patches with near-horizontal normals constrain nothing in plan.** Roof and bonnet patches
   have normals with large vertical components; their in-plane information is their boundary,
   which Section 4.4 applies to. The extractor must classify patches by normal elevation and use
   only steep-normal patches for the planar pose, or the fit mixes constraints of different kinds.
8. **The reanchor budget is a policy, not a model.** The annotation spec's three discrete
   corrections per lifecycle governs operator behaviour and is fine for that. In the estimator the
   discrete event is a hypothesis collapse (Section 5.3), which should be logged as such rather
   than rationed by a counter.
9. **The timing requirement is unsized.** Section 4.7 sizes it: deskew the body-local map at close
   range, accept the cluster instant for the filter, and expect no measurable benefit beyond 50 m.
10. **Twenty decision passages decide a p95 poorly.** The plan says so for p99 and maxima. The
    primary statistic should be the paired per-episode difference, with an interval from episode
    resampling, and a pooled-frame p95 should be reported but not decided on.
11. **Privacy is not mentioned.** A body-local shape map is a description of one vehicle's surface,
    and H4 proposes retrieval against a catalogue. TENETS forbids PII by architecture. See Section 8.

## 7. Approaches to consider, ranked for the sparse tail

1. **A shared visibility component (V).** Ray-cast the pose hypothesis against the L3 baseline and
   the frame's nearer returns; label each extent end as physical, occluded, field of view, or
   dropout; compute expected support per face; add a free-space penalty. Apply it to arm A as the
   `Truncated` flag the state plan's Section 8.1 already wants, and to B, C, and D alike. This is
   the one component that adds information where returns are few, and it is cheap.
2. **Arm B as the sparse-tail candidate.** A body-local map capped at a stated number of returns
   with provenance and per-return anisotropic noise; a robust point-to-plane likelihood with
   normals from the map; the map frozen before each update and de-duplicated against the current
   scan; abstention below a declared overlap; no exported inverse-Hessian covariance. Learn the
   shape at 10 to 30 m, exploit it beyond 50 m.
3. **Multiple face-identity and yaw hypotheses**, scored by the visibility-aware note's `s_h`,
   collapsed by course, dimensions, and the visibility term, never averaged.
4. **Censored output for the unobserved extent.** Report the near face with its centimetre sigma
   and the far face as an interval from the dimension belief. Where the following gap is measured
   between a leader's rear face and a follower's front face and one of them is the near face, use
   it directly; infer the other with its interval. This is a product contract, not an estimator
   change, and may be the largest honest headway gain available.
5. **Per-return anisotropic noise** in every registration residual, from the state plan's
   Section 8.1 model that `adaptive_uncertainty` already implements for the medoid.
6. **Physical-edge features, not patch graphs, as the tangential measurement.** Corners from two
   non-parallel strands (the L-shape of Zhang et al.) and strand ends bounded by free space. These
   are the only cheap sources of along-face information and they need no persistent identity.
7. **Track-conditioned membership** for far objects (Section 5.5), tested in E3 as a mechanism.
8. **Comparators for curved bodies**: the Gaussian-process trackers the plan already names, with a
   random-matrix filter as the floor. Keep both as comparators, not candidates.

Not worth pursuing first, as the plan already says: an all-pairs rigid graph, semantic part
labels, six degrees of freedom, and a general factor graph.

## 8. Side effects

| Side effect                 | Mechanism                                                                                                                                     | Expected size                                                                                             | Mitigation in the revised plan                                                                                                                                                            |
| --------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Runtime                     | Extraction, association, and a robust fit per candidate pair, on top of the near-edge measurement                                             | Above the measured 3.5 to 8 ms p99 of the near-edge arm; above the 3 ms gate before any facet work starts | Cost model and budget in F0; per-assigned-pair measurement; the runtime scorecard row                                                                                                     |
| Memory                      | A body-local map or patch set per live track                                                                                                  | Bounded by the cap; a thousand returns per track at 40 bytes is 40 kB, manageable on the Pi               | Declare the cap; evict on track end                                                                                                                                                       |
| Determinism and replay      | A cache makes the tracker stateful across frames in a new way; iteration counts and float order can differ between runs                       | Byte-identical replay is the campaign's standard and must hold                                            | Fixed iteration counts, ordered reductions, repeat runs as in the campaign                                                                                                                |
| Identity stickiness         | A wrong association or merge enters the cache and keeps pulling the fit toward itself                                                         | A merged neighbour inflates the shape by a vehicle; recovery needs revision                               | Provenance per contribution; rollback test in E3; no silent permanence                                                                                                                    |
| Double counting             | Cached returns re-scored against the current scan; a registration posterior that already holds the motion prior fed back as a measurement     | Confidence grows without information (`n_eff` 1.1 for 100 repeats at correlation 0.9)                     | Frozen pre-update cache, de-duplication, one combined update; the plan's existing rule                                                                                                    |
| Calibration                 | Inverse-Hessian covariance optimistic in weak directions; spurious eigenvalues from noisy normals                                             | Narrower wrong intervals, which the uncertainty row already fails                                         | Report prior-dominated directions as such; G-UNC-1 by data-only rank                                                                                                                      |
| Interaction with T1, T3, T5 | A facet measurement and the near-edge measurement describe the same filter; hysteresis and course alignment were tuned for faces, not patches | Unmeasured; a second measurement model without a declared merger invites a repeat of the T5 ambiguity     | One measurement model per arm; the facet arm replaces the face update rather than adding to it                                                                                            |
| Class prior                 | Half-extents still come from the class prior until extents converge; a patch fit does not change that                                         | The 0.2 m of Section 4.5 persists                                                                         | Extent memory and interval output                                                                                                                                                         |
| Configuration fingerprint   | New tuning keys move the fingerprint and refuse the perf baselines                                                                            | Already handled for the solid body by a nil-by-default block                                              | Same pattern: an options block, absent by default                                                                                                                                         |
| Persistence and VRLOG       | Patch identities, revisions, and body registrations need a schema if they are to be inspected or replayed                                     | A new versioned document, as the annotation spec proposes                                                 | Keep it offline and separately versioned until F4                                                                                                                                         |
| Operator load               | Facet review per informative frame                                                                                                            | The plan's 20 to 40 hours; the labeller of Section 5.4 can propose edge labels and reduce it              | Measure the first five episodes and reforecast, as the plan says                                                                                                                          |
| Privacy                     | A body-local surface map is a per-vehicle description; dents, racks, and trim are identifying over time, and H4 proposes catalogue retrieval  | No PII by architecture is a tenet; a persisted vehicle shape cache is a new class of retained data        | Maps live in memory for the track's lifetime only; nothing body-local is persisted in production; H4 reports family level only; publish aggregate descriptors, never per-vehicle surfaces |

## 9. Numerical checks performed

Twelve arithmetic checks were run for this note with the script `facet_checks.py`, kept in the
session scratch area and not committed; the inputs and expected results are recorded here so they
can be promoted to fixture-backed tests before any implementation. Checks 2 and 12 were rerun on
2026-10-08 at the recorded 2.3 m mount, after first reproducing the 3 m figures; check 7's ranges
are unchanged by it. They are equation checks, not
a sensor simulation or a tracker test.

| Check | Inputs                                                                                 | Verified result                                                                                                                                     |
| ----: | -------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------- |
|     1 | Side at `y = 0.9`, normal `(0, 1)`, support `u = -2..2`, unit weights                  | Information diagonal 0, 5, 10; tangent null                                                                                                         |
|     1 | Same side split into four patches, twenty returns                                      | Diagonal 0, 20, 29.47; rank 2                                                                                                                       |
|     1 | Side plus three returns on a rear face at `x = -2.25`                                  | `H_xx = 3`; rank 3                                                                                                                                  |
|     1 | Twenty normals perturbed uniformly within one degree, seed 20261008                    | `H_xx = 0.0024` against the expected 0.0020                                                                                                         |
|     2 | 4.5 by 1.8 by 1.5 m box, 2.3 m mount, 0.2 degree azimuth, Pandar40P table, 10 to 100 m | Rings 11, 11, 8, 6, 5, 4, 3, 2; end returns 567 down to 10; ring gap under 0.6 m (at 3 m: rings 6, 7, 8, 6, 5, 4, 3, 3; end returns 309 down to 15) |
|     3 | `sigma_r = 0.02 m`, `N = 3, 8, 20, 100`                                                | Plane sigma 11.5, 7.1, 4.5, 2.0 mm; prior to plane ratio 28 at `N = 8`                                                                              |
|     4 | Strand yaw bound at 20, 50, 80 m, incidence 0 to 70 degrees                            | 0.11 to 0.79 degrees                                                                                                                                |
|     5 | 4.5 m body at 10, 20, 50 m; 10 Hz sweep                                                | Sweep 7.2, 3.6, 1.4 ms; smear 7.2, 3.6, 1.4 cm at 10 m/s                                                                                            |
|     6 | 100 repeats at correlation 0.9; 20 at 0.5; 10 at 0                                     | `n_eff` 1.110, 1.905, 10                                                                                                                            |
|     7 | Expected end-face support with detection probability 0.7 at 40, 60, 80 m               | 54.1, 24.1, 13.5 returns; `P(N < 8)` 0, 0, 0.041                                                                                                    |
|     8 | Quarter and half of a 4.5 m side hidden; quarter of a 1.8 m end                        | Midpoint bias 0.562, 1.125, 0.225 m                                                                                                                 |
|     9 | Gap 10 m, speed 10 m/s, gap error 0.5 m, speed error 1 m/s                             | Time-gap error 0.05 s and 0.1 s                                                                                                                     |
|    10 | Azimuth step at 20, 30, 50 m                                                           | Resolved-edge tangential sigma 2.0, 3.0, 5.0 cm                                                                                                     |
|    11 | Plane sigma 0.05 m, width sigma 0.4 m, correlation 0, 0.5, -0.5                        | Centre sigma 0.206, 0.229, 0.180 m                                                                                                                  |
|    12 | Ring sag across a 4.5 m face at 5, 10, 20 m for elevations -5 to -24.6 degrees         | 0.011 to 0.221 m; rings between -24.7 and -9.1 degrees strike the face at 5 m at the 2.3 m mount (-31 to -16.7 at 3 m)                              |

Checks 1, 6, 9, and 11 reproduce or extend the visibility-aware note's own checks and agree with
them.

## 10. Changes made to the plan

The [plan][plan] was revised in the same change as this note:

- Section 1 states the review's conclusion and separates the two decisions: D as a challenger for
  mid-pass lateral steadiness, B with the visibility component for the sparse tail.
- Section 2 adds hypothesis H2s for the sparse tail and ties H1 to a physical-edge labeller.
- Section 3 defines the sparse-tail strata and the extent-end labels, and sizes the timing check.
- E1 gains the must-pass tests of Sections 4.2, 4.3, and 4.8, and the labeller score.
- E2 specifies arm B, adds the shared visibility component V and the optional D0 arm, and adds the
  error-attribution gate between E1 and E2.
- E3 names track-conditioned membership and the cache rollback test as mechanisms under test.
- The scorecard gains sparse-tail, attribution, and cost rows, and the uncertainty row forbids a
  Gaussian sigma on a prior-dominated direction.
- Work packages F0 to F2 absorb the labeller and the B specification, with the estimate ranges
  widened and the reason stated.
- Section 8 adds the retention rule for body-local shape data.
- Section 9 cites the prior work above, and a new Section 10 carries the side-effects register.

## 11. References

Entries are in `data/maths/references.bib` under the keys shown.

| Key             | Reference                                                                                                                                                                                                                                                                                     |
| --------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Petrovskaya2009 | Petrovskaya and Thrun, "Model based vehicle detection and tracking for autonomous urban driving", _Autonomous Robots_ 26(2–3):123–139, 2009. [PDF](https://ai.stanford.edu/manips/publications/pdfs/Petrovskaya_2009_AURO.pdf)                                                                |
| Granstrom2011   | Granström, Lundquist, and Orguner, "Tracking rectangular and elliptical extended targets using laser measurements", _FUSION_ 2011. [PDF](https://people.isy.liu.se/rt/karl/publications/GranstromLO2011.pdf)                                                                                  |
| Wyffels2015     | Wyffels and Campbell, "Negative information for occlusion reasoning in dynamic extended multiobject tracking", _IEEE Trans. Robotics_ 31(2):425–442, 2015. [DOI](https://doi.org/10.1109/TRO.2015.2409413)                                                                                    |
| Held2014        | Held, Levinson, Thrun, and Savarese, "Combining 3D shape, colour, and motion for robust anytime tracking", _RSS_ 2014. [DOI](https://doi.org/10.15607/RSS.2014.X.014)                                                                                                                         |
| Held2016        | Held, Thrun, and Savarese, "Robust real-time tracking combining 3D shape, colour, and motion", _IJRR_ 35(1–3):30–49, 2016. [DOI](https://doi.org/10.1177/0278364915594474)                                                                                                                    |
| Kraemer2018     | Kraemer, Stiller, and Bouzouraa, "LiDAR-based object tracking and shape estimation using polylines and free-space information", _IROS_ 2018, pp. 4515–4522.                                                                                                                                   |
| Pang2021        | Pang, Li, and Wang, "Model-free vehicle tracking and state estimation in point cloud sequences", _IROS_ 2021. [arXiv](https://arxiv.org/abs/2103.06028)                                                                                                                                       |
| Zhang2017       | Zhang, Xu, Dong, and Dolan, "Efficient L-shape fitting for vehicle detection using laser scanners", _IEEE IV_ 2017. [DOI](https://doi.org/10.1109/IVS.2017.7995698)                                                                                                                           |
| Segal2009       | Segal, Haehnel, and Thrun, "Generalized-ICP", _RSS_ 2009. [DOI](https://doi.org/10.15607/RSS.2009.V.021)                                                                                                                                                                                      |
| Zhang2014       | Zhang and Singh, "LOAM: lidar odometry and mapping in real-time", _RSS_ 2014. [DOI](https://doi.org/10.15607/RSS.2014.X.007)                                                                                                                                                                  |
| Censi2007       | Censi, "An accurate closed-form estimate of ICP's covariance", _ICRA_ 2007, pp. 3167–3172.                                                                                                                                                                                                    |
| Brossard2020    | Brossard, Bonnabel, and Barrau, "A new approach to 3D ICP covariance estimation", _IEEE RA-L_ 2020. [DOI](https://doi.org/10.1109/LRA.2020.2965391)                                                                                                                                           |
| Zhang2016       | Zhang, Kaess, and Singh, "On degeneracy of optimisation-based state estimation problems", _ICRA_ 2016, pp. 809–816. [PDF](https://frc.ri.cmu.edu/~zhangji/publications/ICRA_2016.pdf)                                                                                                         |
| Tuna2024        | Tuna, Nubert, Nava, Khattak, and Hutter, "X-ICP: localisability-aware LiDAR registration for robust localisation in extreme environments", _IEEE Trans. Robotics_, 2024. [arXiv](https://arxiv.org/abs/2211.16335)                                                                            |
| Bonnabel2016    | Bonnabel, Barczyk, and Goulette, "On the covariance of ICP-based scan-matching techniques", _ACC_ 2016. [arXiv](https://arxiv.org/abs/1410.7632)                                                                                                                                              |
| Hatleskog2024   | Hatleskog and Alexis, "Probabilistic degeneracy detection for point-to-plane error minimisation", 2024. [arXiv](https://arxiv.org/abs/2410.10784)                                                                                                                                             |
| Granstrom2017   | Granström, Baum, and Reuter, "Extended object tracking: introduction, overview and applications", _J. Advances in Information Fusion_ 12(2):139–174, 2017. [arXiv](https://arxiv.org/abs/1604.00970)                                                                                          |
| Wahlstrom2015   | Wahlström and Özkan, "Extended target tracking using Gaussian processes", _IEEE Trans. Signal Processing_ 63(16):4165–4178, 2015. [DOI](https://doi.org/10.1109/TSP.2015.2424194)                                                                                                             |
| Kumru2021       | Kumru and Özkan, "Three-dimensional extended object tracking and shape learning using Gaussian processes", _IEEE TAES_ 57(5):2795–2814, 2021. [DOI](https://doi.org/10.1109/TAES.2021.3067668)                                                                                                |
| Xia2021         | Xia, Wang, Berntorp, Svensson, Granström, Mansour, Boufounos, and Orlik, "Learning-based extended object tracking using hierarchical truncation measurement model with automotive radar", _IEEE JSTSP_ 15(4):1013–1029, 2021. [DOI](https://doi.org/10.1109/JSTSP.2021.3058062)               |
| Scheel2019      | Scheel and Dietmayer, "Tracking multiple vehicles using a variational radar model", _IEEE Trans. ITS_ 20(10):3721–3736, 2019. [DOI](https://doi.org/10.1109/TITS.2018.2879041)                                                                                                                |
| Rieken2016      | Rieken and Maurer, "Sensor scan timing compensation in environment models for automated road vehicles", _IEEE ITSC_ 2016, pp. 635–642. [PDF](https://www.tu-braunschweig.de/fileadmin/Redaktionsgruppen/Institute_Fakultaet_5/IFR/Dateien_EFS/Publikationen/SensorScanTimingCompensation.pdf) |
| Mannari2025     | Mannari et al., "ET-PMHT for partially visible convex polytopes", _IET Radar, Sonar and Navigation_, 2025. [DOI](https://doi.org/10.1049/rsn2.70061)                                                                                                                                          |
