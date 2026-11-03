---
id: ground0006
title: "A constrained robust road plane and a kerb split on the settled backgrounds, against the regional ground surface"
status: answered
owner: david
created: 2026-10-27
asks:
  - docs/lidar/operations/pcl-algorithm-review.md#q0-can-pcl-improve-road-and-kerb-geometry-enough-to-make-safety-metrics-more-reliable
  - docs/plans/lidar-static-scene-local-fit-plan.md#research-threads
  - docs/lidar/operations/spatialrust-segmentation-review.md#what-to-improve-before-copying-an-approach
depends_on: [ground0001]
report: docs/lidar/operations/ground-constrained-plane-2026-10.md
requires:
  capabilities: [pcap:s2, pcap-build]
  disk_gb: 20
arms:
  - name: a0
    tuning: config/tuning.defaults.json
cases:
  corpus: d2-first-segment
  count: 23
steps:
  - id: replay
    kind: state_estimation_baseline
    over: [arms, cases]
    estimate: { each: 2m, worker: m1pro }
    note: >-
      one settled background per site, exported as PCD by bgasc -format pcd: the placement's
      first 300 s replayed in analysis mode on a scratch server at the defaults, the preserved
      grid persisted and exported
  - id: oracle
    kind: manual:implement
    estimate: { human: 6h }
    note: >-
      Per site and per 10 m region of RegionalGroundSurface: PCL 1.15.1's SACSegmentation with
      SACMODEL_PERPENDICULAR_PLANE in a gravity-aligned frame, non-zero axis z, eps 5 degrees
      converted to radians, MSAC, threshold 0.05 m, on the lowest returns. A roof or van floor
      can pass the angle gate, so the low-return selection remains necessary. Explicitly record
      whether coefficient optimisation is enabled; if enabled, use the reselected inliers
      SACSegmentation returns, not the initial consensus mask. Check the returned mask against
      the returned normalised plane at the same strict distance threshold over the supplied fit
      indices; held-back road points must not enter that fit population. A failed or empty fit is
      unsupported. Refinement is one fit and one reselection, not iteration to stable membership.
      Then consistently oriented normals at 0.3 and 1.2 m and DifferenceOfNormalsEstimation, cells above a DoN
      magnitude of 0.25 withheld from the fit as kerb candidates, and SACMODEL_LINE on their
      boundary cells. Per region: residual of held-back road returns to each surface (median, MAD,
      95th percentile), the share of kerb candidates within 0.3 m of a Gerardus kerb where the site
      is registered.
  - id: write
    kind: manual:write
    estimate: { human: 3h }
actual:
  wall: 1h42m
  note: 24 backgrounds one at a time on arrow-worker (4 vCPU) beside another lane, 156 to 360 s each from the NAS; the oracle 6 to 10 s a site; the oracle was built by an agent in about 1h against the 6h estimate
---

# ground0006: a constrained road plane and kerbs

**Question.** The regional ground surface is a least-squares plane over each region's lower
quartile, so a kerb, island or parked-van floor inside a region pulls it. A plane constrained to
near-vertical normals and scored by MSAC, with kerb faces split out by a difference of normals, is
the road prior the SpatialRust review asked for. On the 23 sites, graded ones first, does it lower
the surface residual of held-back road returns, and do its kerb lines agree with Gerardus's?

**Rule, written before the run (2026-10-27).** The constrained fit is proposed as a replay
estimator behind v0.5.4's slope-aware ground block if, on Marina and Columbus, the 95th percentile
of held-back road residuals falls by 2 cm or more in the regions holding a kerb, and no region at
any site rises by more than 1 cm. Kerb lines are offered to Gerardus whatever the result; this
repository does not keep them (D-36).

**The rule's reading, fixed at approval (2026-11-03), before any oracle run.** Everything below
is fixed now; nothing is changed after a result is seen.

- _Sites._ Marina is `marina-webster-beach` and Columbus is `columbus-broadway`, the corpus's
  marina and columbus cases. The other 21 first-segment sites count for clause 2 only.
  `embarcadero-folsom` is not read.
- _Background._ One per site: the placement's first 300 s replayed in analysis mode at the
  defaults, persisted and exported with `bgasc -format pcd`. At 16 sites this is the first
  five-minute segment; at the seven 2026-09-01 sites the first segment holds 32 to 262 s of the
  placement, and the 300 s read on into the next segment, as ground0001's replay did.
- _Hold-back._ Every fifth exported point by ordinal (ordinal mod 5 = 0) is held back. Both
  surfaces are fitted from the other four fifths only: `RegionalGroundSurface` is refitted on
  them with `l3grid`'s method (10 m cells, lower quartile, 3 × median refit, global fallback),
  ported to the oracle and checked against the Go fit.
- _Region._ Each 10 m cell that has its own `RegionalGroundSurface` plane.
- _Lowest returns._ In a region, fit points at or below the region's lower-quartile height
  (index ⌊(n − 1)/4⌋ of the sorted heights, the regional fit's own selection), less kerb
  candidates.
- _Frame._ The sensor frame, axis +z. These captures carry no gravity reference (no IMU), so the
  5° cone is about the sensor's z; the global planes tilt 0.6 to 5.5 % (at most 3.2°).
- _Fit._ PCL `SACSegmentation`, `SACMODEL_PERPENDICULAR_PLANE`, `SAC_MSAC`, axis (0, 0, 1),
  eps 5° in radians, threshold 0.05 m, 1,000 iterations, PCL's fixed seed, coefficient
  optimisation **enabled**; the returned reselected inliers, checked against the returned
  normalised plane at strict < 0.05 m over the supplied fit indices. Fewer than 12 fit points, an
  empty mask or no coefficients is _unsupported_: the estimator keeps the regional plane there
  (equal residuals), and the count is reported.
- _Kerb candidates._ Normals at radii 0.3 and 1.2 m on every point, from the fit points as the
  search surface, flipped towards the sensor origin; `DifferenceOfNormalsEstimation`; a point
  is a candidate when its DoN magnitude is finite and above 0.25.
- _Held-back road returns._ Held-back points in the region at or below its lower-quartile height
  that are not kerb candidates. A region is _scored_ with 20 or more.
- _Residual._ Vertical: z less the surface's height at (x, y), the regional plane's
  `HeightAt` and the constrained plane solved for z. Per region: median |r|, MAD of r, and p95
  of |r| (linear interpolation).
- _Kerb lines._ Boundary cells are candidates within 0.3 m of a non-candidate point and no more
  than 0.30 m above the region's lower-quartile height. `SACMODEL_LINE`, RANSAC, threshold
  0.10 m, 1,000 iterations, optimised; a line is kept with 10 or more inliers, a direction within
  20° of horizontal and an inlier extent of 2 m or more. A region _holds a kerb_ when it keeps a
  line.
- _Clause 1._ At each of Marina and Columbus, the p95 of |r| pooled over the held-back road
  returns of its scored kerb-holding regions falls by 0.020 m or more (regional less
  constrained).
- _Clause 2._ In every scored region of the 23 sites, the constrained p95 is no more than
  0.010 m above the regional p95.
- _Passing_ proposes the estimator in the report; this run changes no default. The Gerardus
  comparison is descriptive and only where a site has an accepted registration; the kerb lines
  leave this repository only as a description (D-36).

**Answer (2026-11-03).** No. The constrained plane's p95 rises in the kerb regions at Marina
(0.582 to 1.169 m) and Columbus (0.403 to 0.638 m), and by more than 1 cm in 413 of 566 scored
regions, so both clauses fail. The lowest quartile holds several surfaces, which MSAC's single
slab leaves in the tail, and the difference of normals flags 26 to 41 % of a one-return-per-cell
background. See the [report](../../../docs/lidar/operations/ground-constrained-plane-2026-10.md).

**What it does not establish.** Whether the clip it would drive retains more lower-body returns,
which ground0002 measures; tracks or safety metrics.

**Oracle contract checked.** [OQ-03](../../../docs/lidar/operations/pcl-segmentation-contracts.md)
confirms that reviewed PCL reselects plane inliers after coefficient optimisation. The same check
finds that plain RegionGrowing can bridge remote matching-normal patches; its residual threshold
only stops those points seeding further growth. It is not a substitute for bounded connectivity.
PMF performs a true opening but can retain elevated flat support without road evidence. Neither
algorithm is added as an arm here, and this source/fixture check does not satisfy the site rule.
