# PCL oracle versions and executable coverage

- **Status:** OQ-04 answered; experiment harness contracts specified, campaign implementation still open
- **Review:** [PCL open questions](pcl-algorithm-review.md#open-questions-and-how-to-continue)
- **Plan:** [Item 1](../../plans/lidar-pcl-algorithm-candidates-plan.md#item-1-pcd-export-and-the-pinned-oracle)
- **Tools:** [Offline diagnostics](../../../tools/pcl-review/README.md)

## Installation decision

Keep the oracle at PCL `pcl-1.15.1`, commit
`3346843ea6b97697a1577293aa9cdf236b5c7ffb`, built from source outside the checkout.
The OQ-02 and OQ-03 results used that source, not a package-manager installation. Replacing it
with a same-named package would change provenance and requires repeating the contract checks.
PCL remains an offline reference, never a product dependency.

The public package listings were checked on October 28, 2026 (UTC):

| Distribution                                                                | Published version          | Relevance to this oracle                                                                                                                                                                                                                     |
| --------------------------------------------------------------------------- | -------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [Homebrew pcl](https://formulae.brew.sh/formula/pcl)                        | 1.15.1, formula revision 8 | Same upstream release number; formula dependencies include Eigen 5.0.1. This is not the recorded unmodified source build with Eigen 3. Do not equate the bottle with the source pin without checking formula changes and repeating fixtures. |
| [Debian trixie pcl-tools](https://packages.debian.org/trixie/pcl-tools)     | 1.15.0+dfsg-2              | Earlier upstream release and Debian packaging; not this pin.                                                                                                                                                                                 |
| [Debian bookworm pcl-tools](https://packages.debian.org/bookworm/pcl-tools) | 1.13.0+dfsg-3              | Earlier upstream release; not this pin.                                                                                                                                                                                                      |

Homebrew's binary inventory and Debian's [trixie ARM64 file list](https://packages.debian.org/trixie/arm64/pcl-tools/filelist)
include `pcl_cluster_extraction`, `pcl_compute_hull`, `pcl_normal_estimation`,
`pcl_sac_segmentation_plane`, and `pcl_progressive_morphological_filter`. This verifies published
availability, not a successful installation or compatible oracle semantics. No Debian host was
used. Package listings can change; freeze the version, architecture, package revision, and source
patches if a later run chooses that route.

On this Mac, `brew list --versions` reports Eigen 3.4.1, FLANN 1.9.2_4, Boost 1.92.0, and
nanoflann 1.12.1, but no installed `pcl` formula. None of the three checked stock CLIs
(cluster extraction, hull, or plane segmentation) resolves on PATH. The external PCL build is
Release, with features, filters, common, search, kdtree, octree, and sample_consensus libraries;
I/O, segmentation, surface, and tools are disabled. Enabling a module in CMake is not proof that
its library was built: record actual target completion and linked libraries for each later tool.

## Development inputs and qualification isolation

As clarified by [OQ-11](pcl-original-discussion-reconciliation.md), exporting frames and inspecting
oracle outputs is development use. The [cohort-isolation plan](../../plans/lidar-qualification-cohort-isolation-plan.md)
owns the shared eligibility guard, site/capture assignments, and isolated qualification scoring.
It is still Proposed; these diagnostics do not implement that guard.

For a local diagnostic, name a known development capture and its digest explicitly; the kirk0
checks in this review are tuning evidence. Do not claim enforced isolation from the presence of
an annotation split alone. Before a qualification claim, the campaign must record its authorised
cohort role, frozen manifest/protocol and input hashes, and prevent qualification inputs or
feedback entering tuning/export inspection through an unguarded path. Unknown eligibility is not
permission to inspect a qualification capture. Ordinary production processing and eventual public
publication retain the separate policy in the cohort plan; predictions are not reference labels.

## Source-build recipe

The existing verified source/build directories are `/private/tmp/velocity-pcl-review/pcl` and
`/private/tmp/velocity-pcl-review/build`. They are disposable local evidence. For a fresh build,
use an external directory, obtain the pinned revision, and verify its HEAD and clean status.
A sparse checkout must contain the selected modules' sources and CMake files, not headers alone.
The following macOS recipe describes the diagnostic build, with Eigen 3 explicitly selected:

```bash
brew install eigen@3 flann boost nanoflann cmake
cmake -S /private/tmp/velocity-pcl-review/pcl -B /private/tmp/velocity-pcl-review/build \
  -DCMAKE_BUILD_TYPE=Release \
  -DEigen3_DIR=/opt/homebrew/opt/eigen@3/share/eigen3/cmake \
  -DBUILD_apps=OFF -DBUILD_examples=OFF -DBUILD_tools=OFF -DBUILD_global_tests=OFF \
  -DBUILD_io=OFF -DBUILD_visualization=OFF -DBUILD_registration=OFF \
  -DBUILD_segmentation=OFF -DBUILD_surface=OFF -DBUILD_recognition=OFF \
  -DBUILD_tracking=OFF -DBUILD_people=OFF -DBUILD_stereo=OFF \
  -DBUILD_outofcore=OFF -DBUILD_simulation=OFF \
  -DWITH_OPENNI=OFF -DWITH_OPENNI2=OFF -DWITH_QT=OFF -DWITH_VTK=OFF \
  -DWITH_PCAP=OFF -DWITH_LIBUSB=OFF -DWITH_QHULL=OFF -DWITH_CUDA=OFF
cmake --build /private/tmp/velocity-pcl-review/build --target pcl_features -j4
cmake -S tools/pcl-review -B /private/tmp/velocity-pcl-review/harness \
  -DPCL_SOURCE_DIR=/private/tmp/velocity-pcl-review/pcl \
  -DPCL_BUILD_DIR=/private/tmp/velocity-pcl-review/build \
  -DEigen3_DIR=/opt/homebrew/opt/eigen@3/share/eigen3/cmake
cmake --build /private/tmp/velocity-pcl-review/harness \
  --target pcl-grid-check pcl-segmentation-check -j2
ctest --test-dir /private/tmp/velocity-pcl-review/harness -R segmentation-contracts -V
```

This records the working setup; OQ-04 did not reinstall dependencies or rebuild PCL from scratch.
The two existing harness targets were rebuilt and the contract test rerun. The base PCL pin
requires Eigen 3 at configuration time; the ordinary Homebrew Eigen 5 package is not a drop-in
substitute for that recipe. A future hull comparator needs surface/Qhull support in a separately
recorded build, and PCD loading through PCL needs I/O support. Do not silently change the
existing diagnostic build when adding those dependencies.

## One executable contract per oracle step

The names marked **planned** below are proposed harness targets, not installed commands. Their
implementation belongs to the specs' existing manual steps. None of the four campaign oracles
is complete merely because a nearby `pcl_*` executable exists.

| Spec and oracle step                         | Selected executable or harness                                                          | Required contract and present status                                                                                                                                                                                                                                                                                                                                     |
| -------------------------------------------- | --------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Shared export                                | `pcl-frame-export`, built by `make build-pcl-frame-export`                              | Built for one fixed kirk0 window, with native-grid ordinals and a build stamp. It is not the general L4 member-set, tracked-observation, or settled-background exporter. `bgasc -format pcd` remains proposed.                                                                                                                                                           |
| l3bg0028 normals and border traits           | **Planned:** `pcl-border-oracle`; existing `pcl-grid-check` is its diagnostic reference | Call IntegralImageNormalEstimation and RangeImageBorderExtractor with the declared LiDAR adapter/resampling contract. Preserve XYZ, missing support, all trait bits, and many-to-one source membership. OQ-02 blocks direct native-grid reuse. The working diagnostic runs one PCD and writes CSVs; it does not establish reviewed boundary labels or coasting evidence. |
| l4clus0005 radius and conditional partitions | **Planned:** `pcl-join-oracle`                                                          | EuclideanClusterExtraction and ConditionalEuclideanClustering with explicit XY search, original z/member intervals, a symmetric 0.3 m interval-gap predicate, and ordinal-keyed partitions including rejected/noise members. Verify equivalence on each declared cap treatment; do not flatten z and then evaluate the condition on the flattened points.                |
| geom0013 convex and optional alpha hull      | **Planned:** `pcl-hull-oracle`                                                          | Read each observation's exact members, project a copy to XY, set dimension 2, retain original identities, and report directional support alongside direct extrema. Optional alpha 0.3 m reports dropped extremes and empty/disconnected output. Needs surface/Qhull; no existing diagnostic implements this contract.                                                    |
| ground0006 constrained fit                   | **Planned:** `pcl-ground-oracle`                                                        | Per-region low-return selection, gravity-aligned perpendicular-plane MSAC, 5° in radians, 0.05 m threshold, explicit optimisation, returned coefficients and reselected ordinal membership. Hold validation points out and report unsupported fits. `pcl-segmentation-check` verifies semantics on fixtures only.                                                        |
| ground0006 normals, DoN, and line candidates | **Planned:** `pcl-ground-oracle`, subsequent stages                                     | Separate 0.3/1.2 m neighbourhoods, consistent orientation, DifferenceOfNormalsEstimation threshold 0.25, exclusion from the fit, and SACMODEL_LINE on candidate boundaries. Record scales, membership, line-fit parameters, and region transforms before execution. A kerb label still requires comparison with reviewed/registered geometry.                            |

The later replay and scoring steps use repository evaluators, not PCL executables. They remain
responsible for reviewed object joins, extent metrics, ground residuals, and coasting outcomes.
No new runtime port, replay flag implementation, or campaign run is claimed by this inventory.

## Why the stock tools do not fulfil these contracts

The pinned [tools/CMakeLists.txt][tool-targets] declares the nearest available targets. Their
implementation bodies, rather than names or help text alone, determine suitability:

- [cluster_extraction.cpp, lines 94–123][cluster]: converts to PointXYZ and uses an ordinary
  XYZ KdTree. It has no conditional overlap predicate or explicit XY representation. It extracts
  original cloud fields for output, but that does not fix the search metric.
- [normal_estimation.cpp, lines 89–118][normal]: organised input takes integral-image covariance
  normals with depth-dependent smoothing enabled; its radius argument becomes a smoothing size.
  Unorganised input takes KdTree normal estimation. Neither branch supplies the declared
  LiDAR adaptation or the complete two-scale, oriented DoN/kerb pipeline.
- [compute_hull.cpp, lines 65–83][hull]: selects convex or alpha hull, without forcing XY or
  dimension 2. It loads PointXYZ and saves a VTK mesh (lines 125–137), so per-observation
  identities and directional comparison still need a wrapper.
- [sac_segmentation_plane.cpp, lines 106–123][plane]: uses an unconstrained plane model and
  RandomSampleConsensus, then refineModel(2, 50). It is not ground0006's perpendicular-plane
  MSAC with SACSegmentation's one optimisation/reselection step.
- [progressive_morphological_filter.cpp, lines 120–159][pmf]: supports approximate and exact
  morphological ground filters and extracts PointXYZ ground points. PMF is not an arm in
  ground0006. The OQ-03 fixture check is sufficient for its present reference role.

Searching the pinned tools sources found no RangeImageBorderExtractor,
ConditionalEuclideanClustering, or DifferenceOfNormals invocation. Organised-edge/viewer tools
are not evidence of an interchangeable range-image border oracle. Adding any custom harness
requires input/output identity tests and the exact spec parameters before a scored run.

## Checkpoint and next work

OQ-04 closes the installation/tool-selection question, not plan Item 1's implementation. Record
PCL HEAD, clean/modified status, compiler, dependency versions, CMake options, executable hashes,
input/config hashes, and invoked arguments in every actual experiment report. Keep raw logs,
PCDs, and labels outside Git. Next is OQ-05's capped/uncapped joining comparison design; no
experiment rule changes in OQ-04.

[tool-targets]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/tools/CMakeLists.txt#L166-L280
[cluster]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/tools/cluster_extraction.cpp#L94-L123
[normal]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/tools/normal_estimation.cpp#L89-L118
[hull]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/tools/compute_hull.cpp#L65-L137
[plane]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/tools/sac_segmentation_plane.cpp#L106-L123
[pmf]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/tools/progressive_morphological_filter.cpp#L120-L159
