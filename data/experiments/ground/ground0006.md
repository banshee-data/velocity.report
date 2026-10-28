---
id: ground0006
title: "A constrained robust road plane and a kerb split on the settled backgrounds, against the regional ground surface"
status: proposed
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
    note: one settled background per site, exported as PCD by bgasc -format pcd
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

**What it does not establish.** Whether the clip it would drive retains more lower-body returns,
which ground0002 measures; tracks or safety metrics.

**Oracle contract checked.** [OQ-03](../../../docs/lidar/operations/pcl-segmentation-contracts.md)
confirms that pinned PCL reselects plane inliers after coefficient optimisation. The same check
finds that plain RegionGrowing can bridge remote matching-normal patches; its residual threshold
only stops those points seeding further growth. It is not a substitute for bounded connectivity.
PMF performs a true opening but can retain elevated flat support without road evidence. Neither
algorithm is added as an arm here, and this source/fixture check does not satisfy the site rule.
