# PCL segmentation contracts: distance, membership, and opening

- **Status:** OQ-03 answered by reviewed source and synthetic contract checks
- **Review:** [PCL open questions](pcl-algorithm-review.md#open-questions-and-how-to-continue)
- **Plan:** [Constrained ground and kerbs](../../plans/lidar-pcl-algorithm-candidates-plan.md#item-5-q0-constrained-ground-and-kerbs-ground0006)
- **Affected spec:** [ground0006](../../../data/experiments/ground/ground0006.md)
- **Harness:** [segmentation-check.cpp](../../../tools/pcl-review/segmentation-check.cpp)

All thirteen review questions and the final branch pass are complete. Source findings and
validation below describe this report's recorded checkpoint; subsequent full-suite validation
and remaining implementation prerequisites are recorded in the
[current handover](pcl-astera-handover.md#resume-point). Closing a review question does not
complete or authorise its proposed campaign.

## Decision

PCL 1.15.1 shares the unbounded-neighbour problem identified in SpatialRust's region grower.
It does not share the stale plane-membership or erosion-only behaviours described in that
review. A PCL class name alone is therefore insufficient grounds for accepting or rejecting
an oracle. Keep ground0006's returned, refined membership contract; do not substitute plain
RegionGrowing for spatially bounded connectivity.

No capture or production tuning arm ran. These are hand-authored software fixtures with fixed
parameters in the harness, not a ground-quality experiment. ground0006 remains proposed and its
23-site rule is unchanged. Raw build and execution logs remain outside Git.

## Claim-level source evidence

All links below name PCL commit `3346843ea6b97697a1577293aa9cdf236b5c7ffb` (`pcl-1.15.1`).
The local source HEAD and referenced implementation lines were checked before execution.

### RegionGrowing: neighbour count is not a spatial bound

**Verdict: no built-in maximum edge distance in the default path.**
[findPointNeighbours, lines 324–348][rg-neighbours] invokes nearestKSearch and saves the
neighbour indices, discarding the returned distances. The default search is a KdTree
([lines 303–318][rg-search]). Thus reducing k can avoid a particular bridge but does not impose
any maximum distance in metres. This statement covers RegionGrowing with its default search,
not every subclass or a caller-supplied search that filters its own results.

[validatePoint, lines 475–512][rg-validation] rejects a neighbour for normal-angle disagreement.
Curvature and residual failures instead set is_a_seed to false and still return true.
[growRegion, lines 435–450][rg-growth] then adds that point to the segment but does not enqueue it.
The residual is the absolute projection onto the current seed's normal, not Euclidean distance.
Even a hard bound on that residual alone would not stop a coplanar long-distance bridge.

**Counterexamples, executed:** two pairs at x = 0, 0.1, 100, and 100.1 m, y = z = 0,
with exact upward unit normals and zero curvature. With k = 2 there are two clusters; with
k = 3 there is one. The shortest inter-patch gap is 99.9 m. Both runs enable a 0.001 m residual
threshold. Raising the second pair to z = 20 m and using k = 4 still gives one cluster: the
remote points are accepted as members even though they cannot propagate the region.
All runs use a 0.01 rad smoothness threshold, 0.01 curvature threshold, and cluster sizes 1–10.
K includes the query point for this search and fixture.

**Consequence:** any future RegionGrowing oracle needs an explicit Euclidean edge gate or a
verified bounded-neighbour search, plus finite unit normals. Seed eligibility and membership
must be tested separately. This finding does not change l4clus0005's separate radius graph.

### SACSegmentation: refined membership is reselected

**Verdict: confirmed; the stale-mask objection does not apply to the successful plane path.**
[segment, lines 110–129][sac-segment] first retrieves the consensus inliers and coefficients.
When coefficient optimisation is enabled, it optimises on those inliers and then calls
selectWithinDistance with the refined coefficients and the configured threshold. The plane
selector [clears the old mask and scans all model indices, lines 153–185][plane-select],
accepting distance strictly less than the threshold. Newly qualifying points can enter, as
well as old inliers leave. This is one refinement and reselection, not an iterative fit to a
fixed-point membership set.

The plane optimiser [uses a least-squares covariance fit, lines 323–359][plane-optimise].
It returns the original coefficients if support is too small or the refined model is invalid.
The perpendicular-plane model [checks its angular constraint, lines 93–119][plane-angle], so
an invalid tilt is not silently retained by this optimiser. An empty or failed segmentation
still needs an explicit unsupported result. This is a plane-path finding, not a guarantee for
every model or malformed input accepted by the generic API.

**Counterexample to retaining consensus membership, executed:** a 5 × 5 grid with points
indexed i = 0…24, x = i modulo 5, y = floor(i/5), and
z = (((7i² + 3i) modulo 17) − 8) × 0.004 m. Use perpendicular-plane MSAC, axis +z,
5° converted to radians, threshold 0.027 m, 1,000 maximum iterations, and PCL's deterministic
constructor default. Run with optimisation disabled and enabled. The consensus mask has
15 members; the returned refined mask has 17, adding indices 4 and 18. Directly evaluating
all 25 points against the returned plane reproduces the refined mask exactly. The refined
coefficients, rounded for this report, are (−0.0000756222, −0.001834, 0.999998, 0.0224265).
The harness asserts both a changed mask and agreement with independent residual evaluation.
The threshold was selected during fixture development to expose a membership change; it is
not a proposed road threshold. ground0006 retains 0.05 m.

**Consequence:** ground0006's existing use of the returned inliers was correct. Its oracle note
now also requires checking membership against returned coefficients on the supplied fit indices,
keeping held-back validation points outside that population, and treating failure as unsupported.
Do not replace this with the pre-refinement consensus mask or re-fit again without declaring it.

### ProgressiveMorphologicalFilter: both passes are present

**Verdict: a morphological opening on the supplied point support, not erosion alone.**
[extract, lines 99–129][pmf-extract] copies the current candidate ground points, calls MORPH_OPEN,
and retains original indices where input z minus opened z is strictly less than the current
height threshold. Candidates removed in an earlier iteration do not return.
The [MORPH_OPEN implementation, lines 114–190][morph-open] takes neighbourhood minima, swaps
that eroded cloud into a temporary, then takes maxima from the eroded values. The neighbourhood
is an axis-aligned XY square of the supplied window side length, with unbounded z extent.
Using the original search tree in the second pass does not make it erosion-only: XY positions
are unchanged and the second extrema read the temporary cloud's eroded z values.

**Discriminating fixtures, executed:** three points at x = 0, 1, 2 m, y = 0, and a 3 m square
window. PMF uses cell size 1 m, maximum window size 3, and initial/maximum height thresholds
of 1 m. Its first exponential window is 3 m, so there is exactly one iteration.

| Input heights (m) | Erosion (m) | Opening (m) | PMF retained indices | What it demonstrates                                                       |
| ----------------- | ----------- | ----------- | -------------------- | -------------------------------------------------------------------------- |
| 0, 1, 1           | 0, 0, 1     | 0, 1, 1     | 0, 1, 2              | Dilation restores the plateau edge; opening differs from erosion           |
| 0, 1, 0           | 0, 0, 0     | 0, 0, 0     | 0, 2                 | An isolated high point is removed; equality with the threshold is excluded |
| 2, 2, 2           | 2, 2, 2     | 2, 2, 2     | 0, 1, 2              | A flat elevated patch survives without any observed road                   |

**Consequence:** the third fixture is also a counterexample to calling a retained PMF point
physical road truth. A true opening cannot supply missing ground evidence. A later PMF arm
would need support, slope, window, and roof controls; OQ-03 adds no such arm to ground0006.

## Reproduction, provenance, and limits

The harness explicitly includes the three reviewed segmentation template bodies and links the
existing PCL common, filters, sample_consensus, search, kdtree, and octree libraries. It does not
need the separately built segmentation library. It is an offline executable with no product
linkage. Source preparation and CMake variables are in the [tool README](../../../tools/pcl-review/README.md).

- Repository parent: `4710e5f40` on `codex/pcl-open-question-review`; harness and report are saved together in the OQ-03 commit.
- Harness SHA-256: `ea5a3c8a31e250e8f7b4eb88ca64994d823c2fbb2483ffa72f52c70137377996`.
- PCL source and build: `/private/tmp/velocity-pcl-review/pcl` and `/private/tmp/velocity-pcl-review/build`; source Git SHA above, no source edits.
- Environment: macOS ARM64, Apple clang 17.0.0 (`clang-1700.6.4.2`), Eigen 3.4.1; same external PCL build as [OQ-02](pcl-organised-grid-compatibility.md#findings-and-provenance).
- Local logs: `/private/tmp/velocity-pcl-review/oq03-build.log` and `/private/tmp/velocity-pcl-review/oq03-ctest.log`. The written results above are sufficient without those disposable logs.

```bash
cmake --build /private/tmp/velocity-pcl-review/harness --target pcl-segmentation-check -j2
ctest --test-dir /private/tmp/velocity-pcl-review/harness -R segmentation-contracts -V
```

The executable fails if a contract assertion fails. It checks seven configurations: three
region-growing cases, three opening/PMF cases, and one paired SAC case. It neither benchmarks
runtime nor estimates frequency on the capture corpus. Synthetic normals are supplied exactly;
normal estimation, noisy or non-finite inputs, arbitrary search implementations, held-back
road residuals, vehicle retention, and physical accuracy are not validated here.

## Checkpoint validation

The reviewed source tree is clean; all ten source-link paths and line ranges were verified locally.
The C++ harness builds and CTest passes all fixture assertions. Clang-format and relative-link
checks pass. The full `make test` gate passes: Go, Python, web, public site, offline docs,
macOS, S2 Hilbert, and scene capture. As in OQ-02, the root Node tooling used the existing main
checkout's dependencies through a temporary symlink, with pnpm package-manager auto-switching
and strict version enforcement disabled. No lockfile changed; the temporary link was removed.
The macOS test's generated BuildInfo stamp was restored to its tracked placeholder afterwards.
The full `make lint` gate and the staged no-results guard pass for this checkpoint.

[rg-neighbours]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/segmentation/include/pcl/segmentation/impl/region_growing.hpp#L324-L348
[rg-search]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/segmentation/include/pcl/segmentation/impl/region_growing.hpp#L303-L318
[rg-validation]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/segmentation/include/pcl/segmentation/impl/region_growing.hpp#L475-L512
[rg-growth]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/segmentation/include/pcl/segmentation/impl/region_growing.hpp#L435-L450
[sac-segment]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/segmentation/include/pcl/segmentation/impl/sac_segmentation.hpp#L110-L129
[plane-select]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/sample_consensus/include/pcl/sample_consensus/impl/sac_model_plane.hpp#L153-L185
[plane-optimise]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/sample_consensus/include/pcl/sample_consensus/impl/sac_model_plane.hpp#L323-L359
[plane-angle]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/sample_consensus/include/pcl/sample_consensus/impl/sac_model_perpendicular_plane.hpp#L93-L119
[pmf-extract]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/segmentation/include/pcl/segmentation/impl/progressive_morphological_filter.hpp#L99-L129
[morph-open]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/filters/include/pcl/filters/impl/morphological_filter.hpp#L114-L190
