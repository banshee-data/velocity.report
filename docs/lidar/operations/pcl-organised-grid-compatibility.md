# PCL organised-grid compatibility: one kirk0 frame

- **Status:** Measured compatibility failure on one kirk0 frame; OQ-02 answered
- **Question:** OQ-02 in the [PCL review](pcl-algorithm-review.md)
- **Plan:** [Oracle at the recorded Git SHA and exporter](../../plans/lidar-pcl-algorithm-candidates-plan.md#item-1-pcd-export-and-the-oracle-at-the-recorded-git-sha)
- **Parameters:** [pcl-organised-grid.json](../../../config/experiments/pcl-organised-grid.json)

## Method and decision rule

Export the first complete scored frame after 20 s of kirk0 using the production replay FrameTap,
with a 20 s warm-up and a 0.5 s scoring window. Keep the nearest return in each native channel
and 0.2° azimuth cell; retain its frame ordinal, measured range, elevation, and Cartesian XYZ.
Missing cells carry NaN and ordinal -1. This diagnostic exporter does not replace the planned
production exporter or assert that one return preserves dual-return provenance.

Run reviewed PCL 1.15.1 covariance integral-image normals on that grid, smoothing size 5 with
depth-dependent smoothing disabled. Repeat after a rigid coordinate rotation (x,y,z) to
(x,-z,y), undoing the rotation on the normals. A Cartesian covariance method with unchanged
neighbour selection should be rotation-equivariant; changes in valid support or normal angle
expose coordinate-dependent neighbourhood handling. This tests the unmodified PCL depth map,
not the proposed future LiDAR adaptation.

Run RangeImageBorderExtractor twice: first on the native grid with a nominal 0.2° resolution,
then on PCL's own spherical RangeImage generated from the same XYZ returns with LASER_FRAME,
0.2° angular resolution, full angular bounds, zero noise/minimum range, and border size 1.
Use the extractor defaults, including border radius 3 and minimum probability 0.8. Record all
three trait bits separately. The native layout is deliberately the naive assumption under test.

Reject direct native-grid compatibility if valid native cells do not project back to their
assigned pixels, or normals change materially under a rigid coordinate transform because the
depth map changes. Record normal coverage, angular differences, finite range-image support, and
border trait counts. Inspect individual discrepant cells. A spherical result that runs is not
proof of correct physical border labels or an ordinal-preserving oracle: resampling can merge,
create, or omit cells. OQ-02 can close with a negative compatibility verdict and corrected spec.

## Findings and provenance

The direct native-grid contract fails. Both PCL algorithms execute, but the integral-image
normal support depends on the coordinate designated as depth, and the base RangeImage does
not interpret the native ring/azimuth layout as its own projection. A file with WIDTH and HEIGHT
is insufficient to establish either contract.

| Measure                   | Native sensor XYZ grid | Same grid, XYZ rigidly rotated | PCL spherical projection |
| ------------------------- | ---------------------- | ------------------------------ | ------------------------ |
| Layout                    | 1800 × 40              | 1800 × 40                      | 1802 × 203               |
| Finite input/range cells  | 69,113                 | 69,113                         | 124,585                  |
| Finite covariance normals | 11,271                 | 24,033                         | Not evaluated            |
| Obstacle trait cells      | 4,376                  | Not evaluated                  | 8,146                    |
| Shadow trait cells        | 4,212                  | Not evaluated                  | 7,813                    |
| Veil trait cells          | 4,367                  | Not evaluated                  | 7,079                    |

The exported frame had 69,253 returns: nearest-per-cell selection retained 69,113 and discarded
140 collisions. There are 2,887 missing native cells. The two normal runs share 8,274 valid
cells; validity differs in 18,756 cells. After undoing the rigid rotation and ignoring normal
sign, the common-cell angular difference is 0° at the median and 12.0837° at p95. The estimator
emits an OrganizedNeighbor warning in each run that the input is not from a projective device
(projection MSE 0.0661068 over 1,276 sampled valid points).

For the native RangeImage, the mean distance between assigned cells and PCL-projected pixels
is 833.434 pixels. Even subtracting the best constant two-dimensional offset leaves an RMS
residual of 781.657 pixels. Both range-image arms use the LASER_FRAME coordinate transform;
the failure is not fixed by a global image offset. This statistic tests the naive assignment;
it is not a lower bound after axis relabelling, cyclic azimuth remapping, or calibrated ring mapping. The spherical arm uses PCL's own z-buffering
and interpolation. Its larger finite population shows why its output cells cannot be joined to
source returns by array position. Trait counts across these layouts are not accuracy comparisons.

### Inspected cells

Normal vectors were inspected numerically after transforming the rotated output back into the
original coordinates. Values below are rounded; sign ambiguity is removed for angular distance.

| Native cell | Ring | Azimuth     | Range   | Source ordinal | Normal angular difference |
| ----------- | ---- | ----------- | ------- | -------------- | ------------------------- |
| 15,305      | 9    | 181.143768° | 5.588 m | 35,902         | 79.2497°                  |
| 14,855      | 9    | 91.143892°  | 6.416 m | 17,911         | 79.0413°                  |
| 17,107      | 10   | 181.463644° | 5.416 m | 35,148         | 76.4717°                  |

At cell 15,305 the native normal is (0.766230, 0.212460, 0.606426); the transformed-back
normal is (-0.773307, 0.258577, 0.578907). These are different axes, not merely a sign flip.
A native veil example at cell 27,152 has XYZ (8.3726, 14.1824, -0.294087) m and range
16.472 m, with neither obstacle nor shadow set. A spherical veil example at cell 160,918 has
XYZ (10.5959, 33.1081, -1.33543) m and range 34.788 m, also with only the veil bit set.
These inspections establish emitted traits and coordinate dependence; neither point has a
reviewed physical-border or mixed-pixel label.

### Frozen provenance

- velocity.report starting revision: `7e35b3705e1287f9789a8b19df57ed3bb8af6fe9`; exporter and harness are preserved on this branch. The first diagnostic run used local additions, not an already committed executable.
- PCL source: `pcl-1.15.1`, `3346843ea6b97697a1577293aa9cdf236b5c7ffb`; built from source in Release mode with Apple Clang 17.0.0 (`clang-1700.6.4.2`), Eigen 3.4.1, FLANN 1.9.2_4, Boost 1.92.0, nanoflann 1.12.1, and the features target. No product dependency was added.
- Capture: kirk0, 200,657,872 bytes, SHA-256 `2864ebde38e736b496d33361e9bcdc9246aa5147459ec48aee0f8f11f1f58b9a`, matching the repository's LFS identity.
- Selected complete frame: `2025-12-05T22:17:24.82685033Z`; 69,253 input returns, 1,800 azimuth bins, 40 native channels.
- Tuning: `config/experiments/pcl-organised-grid.json`, SHA-256 `b2f64d5e2a0364d76af328eafb2d13647606de539e43ca96737d16d0ca0885d8`; only L3 diagnostics are enabled relative to the starting defaults.
- Initial exported PCD SHA-256: `115b7b6eb201c67a8e4c2d412f88d2cd8e428efdb923152b7ee306576f976e4f`. This initial file predates the exporter’s build-stamp comment; the committed repeat below establishes stamped provenance.
- Raw evidence is local under `/private/tmp/velocity-pcl-review/`: PCL build, replay output, frame export, per-cell normal/border CSV files, and logs. This temporary location is disposable; this report contains the findings needed to understand the decision.

## Decision and next work

OQ-02 is answered negatively for a direct native-grid use of unmodified PCL. Correct l3bg0028
and plan Item 1 before implementing the oracle. Preserve Cartesian coordinates, explicit ray
geometry, per-return identity, unknown support, and directional traits.

For covariance normals, the next implementation needs range-aware discontinuity and neighbourhood
selection separate from XYZ covariance, with synthetic plane/edge controls and an equivariance
check. For borders, choose either a calibrated RangeImage adapter with correct projection methods
or spherical resampling with explicit source-to-cell membership and provenance for interpolated
cells. PCL output can then be compared only on a declared supported population. Returning an
array of bits is not sufficient evidence of a correct labeller.

This is one-frame software compatibility evidence. It does not establish physical border accuracy,
coasting improvements, retention safety, held-out behaviour, or M1/Pi timing. No runtime candidate
or default change follows. The full production PCD exporter remains plan work; this tool is a
bounded diagnostic with nearest-return selection and a fixed kirk0 window.

## Committed repeat and validation

Repeated the export and both checks from `439c190aff6220d00455a8f24f0213f97ee92ac7`.
The make-built exporter records version `0.5.1-pre47`, that Git SHA, and build time
`2026-10-27T23:22:34Z`. The stamped PCD SHA-256 is
`26d4791adac4a6da6fdb53a73a2272172af08a3f6325846d1cc4c5a2f705fc60`.
All PCD data rows and all three per-cell CSV outputs are byte-identical to the first run.
The header differs by the added build provenance. This verifies repeatability of this input
and harness, not physical correctness.

The exporter tests pass with the pcap tag, including nearest-return identity, missing cells,
channel/azimuth wrapping, and output-error propagation. The experiment-config tests pass.
The C++ harness builds against the reviewed PCL libraries and passes its formatting check.
Relative links, documentation formatting, spelling, metadata, geometry wording, and the
no-results guard pass. The experiment graph was regenerated after the new report dependency.

The repository Go, Python, web, public-site, offline-doc, and macOS test targets passed during
`make test`. Its final tooling phase encountered a broken pnpm 12.6.0 launcher; installed pnpm
10 also rejected the existing root lockfile as a multi-document stream. Using the main checkout's
existing root dependencies and disabling automatic pnpm version switching, the remaining S2
suite passed all 80 tests. Scene-capture passed all 53 after permitting its local test listener.
Thus all component suites ran successfully, but the original aggregate command exited with an
environment/tooling error. No package-manager or lockfile change is part of this work.
