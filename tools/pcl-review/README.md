# PCL compatibility check

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
