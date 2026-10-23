# LiDAR-Processing-V2 segmentation and polygonisation review

- **Status:** Source review complete; one experiment spec proposed ([l3bg0026](../../../data/experiments/l3bg/l3bg0026.md)) and
  one arm added to an existing proposal
- **Scope:** The segmentation, clustering, outline and vehicle-gate code of Yevgeniy Simonov's
  LiDAR-Processing-V2 (a ROS 2 pipeline over KITTI frames from a Velodyne HDL-64E on a moving
  car), and what of it applies to velocity.report's L3 and L4 on a fixed Pandar40P. Not an audit
  of its container, build or data-structure library.
- **Upstream revision:** `506aefb70150771b37331d940d97cc1c5bbd363d`
- **Local baseline:** `f9a6a7f4092bd01cd8cba9d3768966db5150d693`
- **Related:** [SpatialRust review](spatialrust-segmentation-review.md),
  [L3 experiment area](../../../data/experiments/l3bg/README.md),
  [L4 experiment area](../../../data/experiments/l4clus/README.md),
  [ground experiment area](../../../data/experiments/ground/README.md),
  [clustering maths](../../../data/maths/clustering-maths.md),
  [ground plane maths](../../../data/maths/ground-plane-maths.md),
  [geometry convergence plan](../../plans/lidar-tracker-geometry-convergence-plan.md)

The repository does what its README says: it segments ground from obstacles, clusters the
obstacles and draws an outline round each, every frame, with no learned model. It does it on a
range image of rings by azimuth, which is the structure our L3 background grid already is, and
that is where it is most useful to us. Its ground method exists because the sensor moves; ours
sits still and lets the background model absorb the road, so the method itself is not what we
lack. What we lack, and what it has, is a pass that decides the ground-to-object boundary by
voting over a range-image neighbourhood in three dimensions, in both directions, instead of by a
per-cell window. That is the one piece worth a spike, and it is the spec this review proposes.

Two things the README advertises are not what the committed code runs. The noise filter is
constructed and sized but never called. The bounding-box fit and the vehicle test behind it are
compiled in but switched off by a constant, so the published outlines are convex hulls only. The
title's "vectorise the scene" is a per-frame polygon per moving cluster, published as a display
marker and forgotten; there is no persistent scene, ground mesh or structure model. Our
[vector scene map](../architecture/vector-scene-map.md) is a different and larger thing.

## What is implemented

The upstream library is five units behind one ROS node. Each row is read from the pinned
[node][node], [segmenter][segmenter], [clusterer][clusterer], [polygoniser][polygonizer],
[noise remover][noise] and [configuration][config]; the timings below are from its committed
[profile][profile].

| Unit                       | Implementation                                                                                                                                                                                                                                                                | Relevance and limits                                                                                                                                                                                                                         |
| -------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Polar grid and range image | Points binned 2 m radially by 1° in azimuth (50 × 360 cells) and, in parallel, into a 64 × 2048 image by ring (or by elevation when no ring is given) and azimuth, nearest return per pixel                                                                                   | Our L3 grid is the same object at 40 × 1800 with channel order as elevation order, kept across frames and persisted. Upstream rebuilds it each frame                                                                                         |
| RECM ground                | Per azimuth slice, outward: the cell's lowest return, bounded above by the previous cell's floor plus a slope allowance, with a sorted-gap guard against returns below the road; a point is an obstacle 0.2 m above its cell's floor                                          | A per-frame lower envelope for a moving sensor. Its floor is bounded above only, so a return below the road drags it down and the road above it reads as obstacle; see below                                                                 |
| Near-field RANSAC          | In the first four radial bins (8 m), a plane through a fixed pivot under the sensor and two random points, 60 draws from a fixed seed, accepted within 11.6° of level; points within 0.2 m of it (and all below it) are relabelled ground                                     | Patches RECM's near-field errors from pitch and mounting, which a fixed mount does not have. Deterministic by seed, which we also require                                                                                                    |
| JCP boundary vote          | Dilate the obstacle mask 5 × 5 in the image; every ground pixel the dilation touches, that has a return, is queued; each is decided by a weighted vote of its 24 neighbours, weight e^(−5d) for 3-D distance d ≤ 1 m, obstacle if obstacle weight exceeds ground, else ground | The one unit that fills a gap here: a two-dimensional, distance-weighted, two-directional decision on the foreground edge. Single pass in row order, decisions written in place, ties to ground. The spike                                   |
| Curved-voxel clustering    | Spherical bins of 0.4 m × 1° × 3°, sized from the frame's own extent, an occupied-voxel hash, breadth-first flood over 26 neighbours with azimuth wrap, then a point-count floor of 3 per cluster                                                                             | Connected components in the sensor's sampling metric, no density core. The angular bins grow with range by construction, which is the idea our range-adaptive ε borrowed from Fontalvo et al. and found not to join the 20 to 35 m fragments |
| Convex hull                | Andrew's monotone chain over a cluster's XY, collinear points dropped, indices returned                                                                                                                                                                                       | We have the same in `convexHull2D`. Nothing to port                                                                                                                                                                                          |
| Antipodal pairs, calipers  | Antipodal vertex pairs of the hull, then for each pair, each of its two vertices and each adjacent edge, all hull points rotated and boxed; smallest area wins                                                                                                                | The same O(h²) as the commented-out edge walk beside it, with each edge tried up to four times; the angle is not folded to a quadrant. Our `MinAreaRectangle` is the plain edge walk                                                         |
| PCA box                    | Jacobi SVD of the 2 × 2 covariance of the hull vertices                                                                                                                                                                                                                       | Ours is the closed form over all members. Both read a strip's long axis and an L's diagonal; neither is `FitRectangle`                                                                                                                       |
| Vehicle gate               | Hull with over 150 points, height 0.9 to 2.5 m, hull area times height inside the class bands' volumes, hull area over box area above 0.4, then length, width and height inside one of five hand-set bands widened by 0.8, 0.5 and 0.5 m                                      | Switched off (`perform_polygon_simplification = false`). The five widened bands overlap heavily and the matching band's index is discarded, so the output is vehicle or not, never a class. Single frame, no kinematics                      |
| DROR noise filter          | A k-d tree rebuilt per frame; a point is noise with fewer than 4 neighbours within max(0.02 r, 0.1 m)                                                                                                                                                                         | Constructed, reserved for 200,000 points, never called from the node. Our L3 support filter and DBSCAN's core count cover the same ground on the grid                                                                                        |
| Containers                 | Every buffer reserved once for 200,000 points; static maps and sets; a bounded queue; an approximate `atan2`                                                                                                                                                                  | The discipline we already keep in the support filter scratch and `expandCluster`. Keep it for any new stage                                                                                                                                  |

The sample data is 154 KITTI `.pcd` frames; the loader reconstructs the ring index by watching
the azimuth wrap, so the organised image is a reconstruction too. The committed `gprof` profile
is from an older build (a recursive label propagation that the current code has replaced with
the queue), so its figures are indicative only: 6.58 s of sampled self time over 228 frames,
about 29 ms a frame on an x86 workstation including ROS publishing, of which the segmentation
stages (grid, RECM with its RANSAC, JCP, labels) are about 9 ms, clustering about 7 ms and the
hull about 3.5 ms across roughly 245 clusters a frame. That build also ran the box fit, about
2.7 ms a frame over the seven clusters a frame that passed the vehicle gate, which the current
code switches off. None of that is a Pi figure.

## How a frame flows

1. The ROS message is converted to one of four PCL point types by field names; a frame of
   another layout throws.
2. Segmentation: polar grid, RECM, near-field RANSAC, transfer to the image, JCP, then labels
   back to points. Points outside 2 to 100 m, or outside −3 to +4 m about the road, are never
   binned and stay unknown.
3. The obstacle points become a new cloud, clustered by curved voxels.
4. For each cluster: the hull, then (disabled) the box fit and vehicle test, then a line-list
   marker extruded between the cluster's lowest and highest return.
5. Ground, obstacle, unknown and clustered clouds and the JCP image are published for RViz.

Nothing persists between frames. There is no tracker, no background model and no map, so the
pipeline's whole answer is a per-frame partition and its outlines.

## The algorithms, read against the code

### Ground: the elevation map and its near-field patch

Each azimuth slice starts at the sensor's known height plus the 0.2 m threshold and walks
outward. An occupied cell's floor is its lowest return, after a guard: the returns are sorted and
scanned downward from the median, and a gap wider than 0.5 m moves the floor to the gap's upper
side, which drops reflections seen below the road. The floor is then the smaller of that and the
previous cell's floor plus an allowance, which is the smaller of the radial spacing times the
tangent of the slope and the threshold less an epsilon. At the shipped values the allowance is
the threshold, 0.2 m per 2 m ring: a grade of 10 %, half the 20 % the parameter names. The code
also takes the tangent of a quantity already given in metres per metre, so the slope it means is
atan(0.2), 11.3°, where tan(0.2) is 11.6°; numerically slight, dimensionally wrong, and repeated
in the RANSAC acceptance test.

The allowance bounds the floor above, never below. A single low return, or a reflection the
0.5 m gap guard does not catch, pulls a cell's floor down and every genuine road return 0.2 m
above it reads as obstacle; the propagated bound then lets the floor recover only at 0.2 m a
ring. Upstream met this near the car and added the RANSAC pass for the first 8 m only, which is
the right diagnosis and a local fix. For us this is a reason not to adopt RECM as an estimator:
the regional surface fitted from settled background in `FitGroundSurface` already drops the upper
three quartiles and then a three-median residual band, which is a two-sided robust fit. What RECM
does have that our fit does not is a constraint between neighbouring cells. On kirk0 the per-cell
planes stepped by up to 0.4 m at shared edges, which is why `BlendedHeightAt` exists; a
slope-allowance check between adjacent cells, failing a cell to the global plane when its
neighbour disagrees by more than the spacing times a grade the site can have, is a test the
regional fit could carry at no runtime cost.

### Boundary refinement: the jump-convolution pass

This is the unit to study. After RECM has coloured each image pixel ground or obstacle, the
obstacle channel is dilated by a 5 × 5 window. Where the dilation covers a ground pixel that
holds a return, that pixel is marked undecided and queued; where it covers an empty pixel it is
discarded. The queue is then drained once, in image row order. Each undecided pixel looks at its
24 neighbours in the 5 × 5 window. A neighbour with no return, or whose return lies more than
1 m away in three dimensions, weighs nothing. Every other neighbour weighs e^(−5d) for its 3-D
distance d, so the weight halves every 0.14 m. The weights are normalised over all contributing
neighbours, including other undecided ones, and summed by label. The pixel becomes obstacle if
the obstacle weight exceeds the ground weight and ground otherwise, including on a tie or when
only undecided neighbours contribute; with no contributing neighbour at all it stays undecided
and its point is published as unknown.

Three properties matter for us.

- **It acts only on the boundary band.** The dilation selects the ground pixels within two
  pixels of an obstacle. In a 40 × 1800 grid at 0.2° that is a ring of cells round each object,
  a few percent of the frame, so a vote over it is cheap whatever its kernel costs.
- **It is two-dimensional and metric.** Our per-cell decision in L3 confirms a foreground return
  by same-ring neighbours only, one-dimensional in azimuth, and the support filter (replay
  experiment `support_filter`, the beam-wise review's E4) counts neighbours over 5 rings by 21
  bins with a range tolerance. Neither weighs a neighbour by 3-D distance to the return in
  question, and neither looks across rings and azimuth at once with a falling weight.
- **It goes both ways.** The support filter only turns foreground into background. The vote
  turns a ground pixel into obstacle when its return sits in 3-D continuity with an obstacle's
  returns. On our grid the analogue is a background-labelled return, inside the cell's acceptance
  window, within a metre of confirmed foreground on neighbouring rings: the lowest returns on a
  body at range, which the window absorbs and the tracker never sees.

The pass also has properties to change. Decisions are written in place as the queue drains, so
a pixel decided early steers its neighbours' votes and the result depends on image order; an
undecided neighbour dilutes both sides equally, so a pixel deep in a boundary band leans to
ground by the tie rule; and the single pass means a decision never propagates further than one
queue entry. A replay-grade version for us should vote from the labels as they stood before the
pass, so the result is order-free and deterministic by construction, and should record which
direction each flip took.

### Clustering: curved voxels

Each obstacle point is binned in range, azimuth and elevation, the bin counts sized from the
frame's own maxima, and the occupied bins go into a hash. A breadth-first flood from each
unlabelled occupied voxel visits its 26 neighbours, wrapping azimuth, and labels every voxel it
reaches; a cluster then keeps its points if it has three or more. There is no core condition:
one occupied voxel joins another whenever they touch. That places it in the family the
SpatialRust review called Euclidean connectivity, with the metric being the sensor's own sampling
grid rather than a Cartesian radius.

The bin sizes decide what joins. At 10 m the voxel is 0.4 m radial by 0.17 m across by 0.52 m
tall at the shipped 3° elevation step; at 40 m it is 0.4 m by 0.70 m by 2.1 m. So two returns
1.4 m apart across the beam connect at 40 m, where our ε of 0.8 m would not; radially nothing
beyond 0.8 m connects at any range, where ε would; and the elevation bin spans whole ring groups,
which is what clustering in XY does anyway. Lane neighbours at range are the obvious casualty, as
the D3 merge row already warns for a wide ε. Its one structural advantage over range-adaptive
DBSCAN is cost: integer bins, a hash, no radius queries.

### Outlines: hull, antipodal pairs, calipers and PCA

The hull is the monotone chain we already have. The box fit enumerates the hull's antipodal
pairs and then, for each pair, each of its two vertices and each adjacent edge, rotates all hull
points and boxes them. Every hull edge is adjacent to two vertices and each vertex can appear in
several pairs, so each edge is tried up to four times, and the cost is O(h²) in the hull size
exactly as the commented-out "unoptimised" edge walk beneath it, which is the classical result
that the smallest rectangle has a side on a hull edge. The antipodal step buys nothing here. The
PCA box fits the hull vertices rather than the members, which weights the boundary; neither
estimator resolves which of an L's two sides is the body, which is what `FitRectangle` is for.
The hull-area over box-area ratio the vehicle gate uses, at 0.4, is a fill ratio: a boxiness
feature the shape descriptors plan already lists as the footprint's fill ratio. That is the only
thing in this unit we do not already have, and the plan has it.

### The vehicle gate

Five hand-set dimension bands (compact, sedan, SUV, truck, minivan), each widened by 0.8 m in
length, 0.5 m in width and 0.5 m in height, their volumes and areas derived, and a cluster is a
vehicle when its hull exceeds 150 points, its height and hull volume fall inside the widest
bands, its fill ratio passes and its box dimensions fit one band. The widened bands overlap so
much that a car-sized box fits three or four of them, and the loop keeps only the fact of a match
and discards which band it was, so the code never emits a class. The README's own figure shows
the limit. Our L6 classifier is a
track-level cascade over averaged box dimensions and speed, which is the right level for a
roadside sensor; the classifier plan's threshold-table tier should derive its bands from the
labelled corpus, not borrow these.

### Noise: dynamic radius outlier removal

A k-d tree over the whole frame, rebuilt each frame, and a radius search whose radius grows with
range. The node never calls it. On our grid the same test is the support filter's count in index
space, without a tree, and DBSCAN's core count applies it again at L4. Not a candidate.

## What maps onto velocity.report

| Upstream                             | Here                                                                                                                                                                | Verdict                                                                                                                                                                                                      |
| ------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Organised 64 × 2048 range image      | L3 `BackgroundGrid`, 40 × 1800, channel order is elevation order                                                                                                    | Have it, in the layer that persists                                                                                                                                                                          |
| RECM lower envelope                  | Background subtraction removes the static road; the clip on moving bodies is the height band on sensor Z, with the P11 regional surface clip in replay (ground0002) | Not adopted as an estimator. Borrow the slope allowance between adjacent cells as a consistency check on the regional fit                                                                                    |
| Near-field RANSAC                    | Two-pass robust plane fit from settled background                                                                                                                   | Not needed: a fixed mount has no pitch to patch                                                                                                                                                              |
| JCP boundary vote                    | Same-ring neighbour confirmation (1-D, foreground only); support filter E4 (2-D count, foreground to background only)                                               | **The spike**: a two-directional, 3-D-weighted vote on the foreground edge of the L3 grid, output only, scored on W3 retention and false foreground ([l3bg0026](../../../data/experiments/l3bg/l3bg0026.md)) |
| Curved-voxel clustering              | DBSCAN XY at ε 0.8 m, MinPts 5; range-adaptive ε answered by l4clus0001                                                                                             | An arm of the SpatialRust review's connectivity comparison (its order 3), not a spec of its own; the merge gate decides it                                                                                   |
| Monotone chain hull                  | `convexHull2D`                                                                                                                                                      | Have it                                                                                                                                                                                                      |
| Calipers minimum-area box            | `MinAreaRectangle`, `BoxAxisMinArea`; `FitRectangle` for the axis                                                                                                   | Have it, and the L-shape fit upstream lacks                                                                                                                                                                  |
| PCA box                              | `EstimateOBBFromCluster`                                                                                                                                            | Have it                                                                                                                                                                                                      |
| Fill ratio (hull area over box area) | Shape descriptors plan, footprint fill ratio                                                                                                                        | Planned; upstream's 0.4 is a data point for its threshold, not a derivation                                                                                                                                  |
| Vehicle gate                         | L6 rule-based track classifier                                                                                                                                      | Not adopted: single-frame, hand-set, binary in effect                                                                                                                                                        |
| DROR                                 | Support filter, DBSCAN core count                                                                                                                                   | Not adopted                                                                                                                                                                                                  |
| Per-frame outline markers            | Published box; vector scene map §6.4 volume extraction for stationary clusters                                                                                      | The per-frame hull is cheap evidence a persistent volume could consume; nothing to port                                                                                                                      |

## Lessons to adopt

### Refine the boundary, not the window

The parameter campaign and the gap analysis's B7 and B8 rows say the same thing from two sides:
the L3 acceptance window is model-imposed, and every change that admits more foreground costs
settling and false clusters across the corpus, while no narrowing cost settling anywhere. The
foreground that matters and is lost is at the object's edge: under CMR, 13 % of the returns in
the 20 to 35 m gaps between a truck's fragments were L3 background
([Stage B return fate](w3-return-fate-stage-b-kirk0-2026-10.md)), and the region-override window
reaches about 5 m at 20 m. A vote confined to the dilated foreground edge is a way to recover an
absorbed return by its 3-D continuity with confirmed foreground on neighbouring rings, without
moving the window for the 97 % of cells that are not at an edge. It is also a way to lose a
return the same way, which is why the spec scores false foreground beside retention.

### The sensor's sampling grid is a metric

Curved voxels, range-adaptive ε and the support filter's index-space window are three ways of
saying that two returns are neighbours when the sensor would have sampled them adjacently. We
have measured one of the three on the fragmentation question and it did not move. The lesson is
not to try the second, but to specify what the fragment gap looks like in the sensor's grid
before choosing any connectivity rule: how many rings and bins separate the cab from the body,
and whether the returns between them exist at all. The SpatialRust review's order 2 is that
measurement, and curved voxels wait on it like everything else.

### A hand-set class table is a gate, not a classifier

Upstream's vehicle bands are a careful piece of work that classifies nothing, because the
tolerances needed to accept real clusters make the bands indistinguishable. The classifier plan's
first tier, data-derived threshold tables, exists to avoid exactly this; the corpus should set
the bands and the scorecard should show they separate.

### Keep stages allocation-free and seeded

Upstream reserves every buffer once and seeds its RANSAC. We do the same in the support filter
scratch, the DBSCAN expansion bitmap and the FNV-seeded subsample. Any new L3 pass has to meet
the same bar: no allocation after warm-up, a result independent of input order, and a benchmark
that pins both.

## What to improve before copying an approach

Source-level observations from reading the pinned code; none is a reproduced runtime failure.

1. **The elevation floor is bounded above only.** A return below the road lowers a cell's floor
   and the floor recovers at one allowance a ring. Any reimplementation needs a lower bound as
   well, or a robust per-cell statistic in place of the minimum.
2. **The slope allowance is capped by the height threshold.** min(spacing × tan(slope),
   threshold) is the threshold at the shipped values, so the stated 20 % grade is 10 %. Separate
   the two parameters, and use the slope as a gradient, not as an angle to take the tangent of.
3. **The vote is order-dependent.** Labels are overwritten while the queue drains, so a pixel's
   vote reads neighbours already flipped this pass. Vote from a frozen copy of the labels.
4. **Undecided neighbours dilute both sides.** Their weight enters the normalisation but no
   label sum, and the tie rule is ground, so the interior of a wide boundary band leans to ground
   for a reason unrelated to geometry. Normalise over decided neighbours, or state the prior.
5. **The voxel grid is sized from the frame.** Bin counts come from the frame's maximum range,
   azimuth and elevation, so the flat voxel index and the azimuth wrap change with the frame's
   extent. Size the grid from the sensor's field of view.
6. **The antipodal step does not reduce the box fit.** It tries each hull edge up to four
   times; the edge walk is simpler and no slower. Fold the angle to a quadrant.
7. **The PCA box fits the hull, not the members.** State which estimator a box is, or the
   extents of two methods on one cluster will not agree.
8. **Two units are dead code.** The noise filter is never called and the vehicle test is behind
   a constant. A capability list for this repository should say what the node runs.
9. **The licence is ambiguous.** The repository's `LICENSE` is GPL-3.0 and its README citation
   says GPL-3.0; every source file's header is the MIT licence. velocity.report is Apache-2.0,
   which cannot take GPL-3.0 code. Nothing here needs porting, and the one unit this review
   proposes is a reimplementation of a published method on our own grid. Do not copy lines.

## Interpret our existing evidence first

| Evidence already in this repository                                                                                                                                | Implication                                                                                                                 |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------- |
| [Stage B return fate](w3-return-fate-stage-b-kirk0-2026-10.md): under CMR, 72 % of the 20 to 35 m gap returns were DBSCAN noise and 13 % L3 background             | A boundary vote can address the 13 %; it cannot touch the 72 %, which is L4's                                               |
| [Range-adaptive ε](l4-range-adaptive-eps-2026-10.md): 20 to 35 m fragmentation stayed at 54 %, 35 to 50 m near 46 %                                                | A range-growing neighbourhood did not join the fragments; curved voxels are that neighbourhood in another coordinate system |
| [Zero-code L3 arms](l3-zero-code-arms-kirk-2026-10.md) and the B7 and B8 rows: wider windows cost settling and false clusters; narrower ones cost nothing measured | The window is the wrong lever for edge returns; a local rule is the one lever not yet tried                                 |
| [l4clus0003](l4-min-cluster-size-kirk-2026-10.md): a lower core threshold added fragments and spurious clusters, not joined bodies                                 | More foreground is not success unless the tracker sees one body; the spec scores fragmented instants beside retention       |
| [L3 consolidation](replay-l3-consolidation-2026-10.md): the defaults are closeness 2.0, margin 0.30 m, neighbour confirmation 5                                    | Every arm needs a fresh current-default control; historical shipped-L3 figures are not today's baseline                     |
| ground0001 to ground0005 are the queued ground questions; the regional surface and `SurfaceHeightFilter` exist                                                     | No ground estimator is wanted from this review; the slope-allowance check is a test on the fit we have                      |

## Proposed experiments and coding sequence

Row 1 is the spike; its spec is written and waits on approval. Row 2 adds an arm to a proposal
the SpatialRust review already made. Rows 3 and 4 are small changes to existing work and need no
spec.

| Order | Question and arms                                                                                                                                                                                                           | Coding work                                                                                                                                                                                                                                                                      | Decision evidence                                                                                                                                                                       |
| ----- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1     | [l3bg0026](../../../data/experiments/l3bg/l3bg0026.md): does a 3-D-weighted vote on the dilated foreground edge of the L3 grid recover absorbed body returns without admitting ground? Arms: defaults, support filter, vote | An output-only L3 pass behind a replay experiment, voting from the frame's pre-pass labels over a ring-by-bin window with e^(−k d) weights and a 3-D distance cut, recording flips by direction; unit tests on a synthetic body with its lowest ring absorbed and on a lone cell | kirk0 W3 retention and fragmented instants in 20 to 35 m, spurious clusters a frame on the 23-site screen, per-frame HOTA, p99 frame time; a rule written in the spec before any replay |
| 2     | Curved-voxel connectivity as an arm of the SpatialRust review's order 3, beside DBSCAN XY and Euclidean XY at matched input, cap and shape filters                                                                          | A named experimental L4 selector with the grid sized from the sensor's field of view, deterministic components, shared membership output and source ordinals                                                                                                                     | Only after that review's order 2 finds a path in the sensor grid between the fragments; the merge gate on side-by-side vehicles decides it                                              |
| 3     | Does the regional surface's step between adjacent cells exceed a slope allowance the site can have?                                                                                                                         | A check in `FitRegionalGroundSurface` that reports, and optionally fails a cell to the global plane on, a neighbour disagreement over spacing × g_max; read in ground0001's measurement step                                                                                     | The p90 and maximum cell step per site that ground0001 already reads                                                                                                                    |
| 4     | Fill ratio as a footprint feature                                                                                                                                                                                           | None new: the shape descriptors plan's footprint section lists hull area and fill ratio                                                                                                                                                                                          | Its Phase 4 range envelope decides whether the ratio is stable at the point counts a cluster has                                                                                        |

For row 1 the promotion rule is in the spec and is repeated here so a reader of this review can
hold the result to it: in 20 to 35 m on kirk0, W3 retention at least five points above the
default with fragmented instants no higher; spurious clusters a frame no more than 1.1 times the
default at every corpus site; per-frame HOTA no lower than the default less 0.005; p99 frame time
no more than 1.1 times the default. Retention bought with spurious clusters means the vote is
pulling the road in, and the next spec restricts it to the background-to-foreground direction
above the surface clip. Passing kirk is a screen, not a default change: a frozen labelled holdout
and the Pi's frame budget come before any default moves.

## Changes to existing plans

- The [L3 experiment area](../../../data/experiments/l3bg/README.md) gains l3bg0026 as its first
  boundary-rule spec; the dwell, tercile and window questions it already holds are unchanged.
- The [SpatialRust review](spatialrust-segmentation-review.md)'s order 3 gains curved-voxel
  connectivity as a third arm when it is specified; its order 2 remains the gate.
- The [geometry convergence plan](../../plans/lidar-tracker-geometry-convergence-plan.md)'s W3
  rule for returns dropped into the background has a candidate mechanism to test, which this
  review names without moving the plan's status.
- The [ground area](../../../data/experiments/ground/README.md) gains no spec. The slope-allowance
  check is a read-out for ground0001's measurement, and `ground-plane-maths.md` records RECM as
  reviewed and not adopted.
- No change to the vector scene map, the classifier plan or the shape descriptors plan beyond the
  references recorded here.

## Review method and limits

Read the pinned README, node, configuration, segmenter, clusterer, polygoniser, noise remover,
k-d tree and point types, and the committed `gprof` profile, against the current L3 grid,
foreground decision, support filter, L4 clustering, box fits, ground filters, L6 classifier and
the October experiment reports. Every algorithmic statement above is from the code as committed;
the two papers upstream names as its sources, Shen et al. (2021) for RECM and JCP and Park et al.
(2019) for curved voxels, were not read for this review, so where the code departs from a paper
this review cannot say. ROS 2, PCL and OpenCV are not available in the review environment and
the node was not built or run; the sample frames are an ego-vehicle dataset and would not have
answered a roadside question if it had been. No replay of our own was run and nothing here is a
measurement. The profile figures are upstream's, from an older build, on their machine.

[node]: https://github.com/YevgeniyEngineer/LiDAR-Processing-V2/blob/506aefb70150771b37331d940d97cc1c5bbd363d/src/processor/src/processor.cpp
[segmenter]: https://github.com/YevgeniyEngineer/LiDAR-Processing-V2/blob/506aefb70150771b37331d940d97cc1c5bbd363d/lidar_processing_lib/src/segmenter.cpp
[clusterer]: https://github.com/YevgeniyEngineer/LiDAR-Processing-V2/blob/506aefb70150771b37331d940d97cc1c5bbd363d/lidar_processing_lib/src/clusterer.cpp
[polygonizer]: https://github.com/YevgeniyEngineer/LiDAR-Processing-V2/blob/506aefb70150771b37331d940d97cc1c5bbd363d/lidar_processing_lib/src/polygonizer.cpp
[noise]: https://github.com/YevgeniyEngineer/LiDAR-Processing-V2/blob/506aefb70150771b37331d940d97cc1c5bbd363d/lidar_processing_lib/src/noise_remover.cpp
[config]: https://github.com/YevgeniyEngineer/LiDAR-Processing-V2/blob/506aefb70150771b37331d940d97cc1c5bbd363d/src/processor/config/processor.param.yaml
[profile]: https://github.com/YevgeniyEngineer/LiDAR-Processing-V2/blob/506aefb70150771b37331d940d97cc1c5bbd363d/analysis.txt
