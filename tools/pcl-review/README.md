# PCL compatibility and contract checks

Offline diagnostic for [OQ-02](../../docs/lidar/operations/pcl-organised-grid-compatibility.md).
PCL is an external source build, never linked into the product. Keep all captures, exports,
per-cell CSV files, and build products outside the checkout.

Build PCL at `3346843ea6b97697a1577293aa9cdf236b5c7ffb` with Eigen 3 and FLANN. Enable common,
search, filters, and features and their dependencies; disable applications, examples, tools,
visualisation, I/O, registration, segmentation, surface, recognition, and tracking. The source
pin requires Eigen 3: the installed Eigen 5 CMake package does not satisfy its version check.

Build the exporter with `make build-pcl-frame-export`; `PCL_FRAME_EXPORT_BIN` selects its output
(default `/tmp/pcl-frame-export`). Pass `-pcap`, `-out`, `-replay-output`, and `-tuning`, with the
latter pointing to `config/experiments/pcl-organised-grid.json`. The window and cell policy are
fixed in the tool and report. Existing output PCD files are refused.

Configure this directory with CMake, setting `PCL_SOURCE_DIR`, `PCL_BUILD_DIR`, and `Eigen3_DIR`
to those external directories, then build `pcl-grid-check`. Invoke it with the exported PCD and
an output prefix outside the checkout. Its stdout contains summaries; the prefix receives
per-cell normal and border CSV files. The raw native RangeImage is deliberately inconsistent:
it tests the proposed direct grid mapping. Only the spherical arm uses PCL's own projection.
Neither output constitutes physical border truth.

The checker accepts the exporter's ASCII grid with forty rows, at least five columns, and
at least one finite retained point. Malformed input or a CSV open/write/close failure exits
non-zero; discard partial output from a failed invocation. CTest's `grid-failures` check uses
synthetic files in the external build directory to verify rejected inputs and output failure.

The [oracle tooling runbook](../../docs/lidar/operations/pcl-oracle-tooling.md) records package
versions, the source-build recipe, and the campaign harnesses still required.

## Segmentation contracts (OQ-03)

Build the `pcl-segmentation-check` target in the same CMake build directory, then run CTest
with `-R segmentation-contracts -V`. This executable explicitly includes the pinned segmentation
template implementations and links the existing filters and sample_consensus libraries; the
segmentation library can remain disabled. It needs no capture, exporter, or tuning config.
Its fixed hand-authored fixtures assert RegionGrowing distance/seed behaviour, SAC refined
membership, and the difference between erosion and opening. Parameters and measured summaries
are recorded in the [OQ-03 report](../../docs/lidar/operations/pcl-segmentation-contracts.md).
Keep stdout and CTest logs outside the checkout. A failed assertion exits non-zero.

## Planar estimator contract (OQ-06)

Build `pcl-registration-check` in the same CMake directory and run CTest with
`-R registration-contracts -V`. The public whole-cloud and correspondence overloads must return
zero z translation despite a target z offset. This corrects an earlier source-review error;
it is not an iterative registration benchmark. It uses registration headers and the existing
common library, without building the registration library. See the
[disposition](../../docs/lidar/operations/pcl-planar-registration-disposition.md).
