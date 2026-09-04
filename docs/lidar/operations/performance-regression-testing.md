# Performance regression testing

The `lidar-bench` tool measures the LiDAR L1–L6 tracking pipeline over a PCAP to detect processing-speed regressions before they reach production. It is the end-to-end perf gate run by `make test-perf` and nightly CI.

## Overview

`lidar-bench` times the full pipeline (foreground extraction → DBSCAN clustering → Kalman tracking → classification) and compares the result against a committed baseline, so algorithm improvements, new features, or refactoring don't inadvertently degrade processing speed. It is a dev/CI tool, separate from the operational `velocity lidar pcap-split` (scan, motion stats, splits).

**Why performance testing matters:**

- Real-time processing requires consistent throughput (≥10 FPS for Pandar40P)
- Memory usage affects edge deployment on resource-constrained devices
- Pipeline stage timing helps identify bottlenecks during optimisation
- Regression detection prevents performance issues from reaching production

## Profiles

A **profile** is a label for how far up the layer stack the pipeline runs. It is
**not a config setting**. Each layer already carries an `engine` selector saying what
runs there, and `"none"` is a legitimate answer:

```json
"l4": { "engine": "none" },
"l5": { "engine": "none" }
```

That config runs L3 and stops. The profile is read off those selectors, so there is
one source of truth for depth and nothing that can disagree with it. A disabled layer
carries no parameter block at all — the codec rejects one — so the file shows the
layer is inert rather than listing nine clustering parameters that have no effect.

| Profile   | `l4.engine` | `l5.engine` | Gated | Why it exists                                                                    |
| --------- | ----------- | ----------- | ----- | -------------------------------------------------------------------------------- |
| `l3-only` | `none`      | `none`      | yes   | Background and settling tuning in isolation; sensor health; constrained hardware |
| `detect`  | an engine   | `none`      | no    | Cluster-level tuning with no tracker state and no track persistence              |
| `full`    | an engine   | an engine   | yes   | What ships. The default.                                                         |

Profiles and engines stay orthogonal: `detect` running `hdbscan_adaptive_v1` is the
same depth as `detect` running `dbscan_xy_v1`, and the profile label does not move.

Ready-made configs live in `config/profiles/<name>.json`. They are derived, not
authored: each is `config/tuning.defaults.json` with layers switched off, and a test
asserts they match exactly that — two profiles are comparable only if depth is the
sole difference between them. They sit outside `config/tuning*.json` because the
key-order tooling requires those to carry every key, which a config with disabled
layers deliberately does not.

**Validation.** Disabled layers must form a suffix: `l5.engine` must be `none` when
`l4.engine` is. Tracking cannot consume clusters that were never produced, so that is
not a configuration with surprising behaviour, it is one with no coherent meaning,
and it is rejected at load rather than discovered at runtime. That rule is what keeps
the depths a closed set without enumerating the legal combinations anywhere.

**Why `detect` exists.** It is 0.3% cheaper than `full` on an 83-second benchmark,
which is not a reason to keep it. It holds no Kalman state and performs no track
persistence — neither shows up in a benchmark, and both matter to a Pi running for
weeks, where `max_tracks` state and per-frame DB writes accumulate. Do not optimise
it away on the benchmark evidence alone.

**Why only two are gated.** `detect` sits within run-to-run noise of `full`. Gating it
would produce a second set of numbers that moves with the first and explains nothing.
An unexercised gated profile is a set of numbers nobody can account for when it moves.

**Turning L6 off** is not currently expressible: L6 has no `engine` selector, and its
one parameter lives in `l5.cv_kf_v1`. Giving L2 and L6 their own blocks is a schema
v3 change tracked in the backlog; a `track` profile falls out of it for free.

## What makes two runs comparable

A benchmark records **cost** (how long) and **work** (how much). Without both, a
run that quietly stopped detecting anything is indistinguishable from a fast one.

Every document carries a workload identity:

| Field                | Meaning                                                        |
| -------------------- | -------------------------------------------------------------- |
| `profile`            | Which layers ran                                               |
| `tuning_fingerprint` | Hash of the whole resolved tuning config                       |
| `pcap_file`          | Which capture                                                  |
| `system_info`        | Platform, CPU count, Go version                                |
| `metrics.work`       | Frames, foreground points, background points, clusters, tracks |

The comparator **refuses** to compare runs whose identity differs, and says which
field diverged. It does not emit a delta between incomparable things. A baseline
with no `profile` or no `tuning_fingerprint` is refused outright.

Work counters carry a 10% tolerance rather than requiring equality: L3's settling
is wall-clock dependent, so counts drift slightly with replay speed. Measured drift
across five repeats on one machine is under 0.01%; between profiles it is 33%,
which the profile check catches first.

This is the check that was missing. The CI baseline committed in June 2026 recorded
832 frames, **zero** foreground points and **zero** clusters — a pipeline whose
background model never finished settling, so nothing downstream of L3 ever ran. For
three months it read as a healthy full-pipeline run, and when detection started
working the gate reported the cost of it as a 7028% heap regression.

## The frame budget

`pipeline.frame_budget_ms` (default **98 ms**) is the per-frame ceiling. Beyond it a
frame is in alarm — lag — territory: the pipeline is no longer keeping up with a
10 Hz sensor, and the next frame is already waiting.

This check needs no baseline, and runs whether or not one is supplied. "Slower than
last time" and "fast enough for the sensor" are different questions, and only the
second has a fixed answer. A relative gate can only ever answer the first.

`-max-frames-over-budget-pct` (default 1.0) is the share of frames allowed past the
ceiling before the run fails. It is not zero because the tail is genuinely noisy:
identical code over the same capture on one machine produced worst-frame times from
86 ms to 329 ms. Tighten it as headroom improves; treat a rise in
`frame_budget.frames_over` as the signal, not the worst single frame.

## Quick start

### Create a baseline benchmark

Baselines are named `baseline-<capture>-<profile>[-ci].json`, with the `-ci` suffix
for runner hardware and no suffix for local. Capture them with the Makefile rather
than by hand, so the repeat count and output path stay consistent:

```bash
# Capture one profile locally as the median of five runs
make perf-baseline PROFILE=full

# Capture every gated profile
make perf-baseline-all
```

Capture CI baselines on the runner, not locally — a local M1 run is roughly 1.4x
faster, so a local baseline measures the wrong machine. The **📏 Capture Perf
Baseline** workflow does it and uploads the result as an artifact to commit.

It runs two ways. Dispatch it by hand (`workflow_dispatch`) to re-baseline on
demand, choosing the profile, capture and repeat count. It also runs automatically
on any pull request touching the perf harness, the tuning config or the committed
baselines — which is exactly when a baseline needs recapturing, since a tuning
change moves the fingerprint and the gate will refuse the old file. That automatic
run is also the only way to capture before a merge: GitHub registers
`workflow_dispatch` from the default branch alone, so a change that needs a fresh
baseline cannot dispatch one until after it has landed, and it needs the baseline
to land.

Download the artifact, drop the `-ci` files into
`internal/lidar/perf/baseline/`, and commit them in the same pull request.

Under the Makefile, this is what runs:

```bash
# Build the tool (requires libpcap)
go build -tags=pcap -o lidar-bench ./cmd/tools/lidar-bench

# Median of five runs at the full profile
./lidar-bench -pcap data/gold-standard.pcapng -profile full -repeat 5 \
  -benchmark-output baseline.json
```

### Compare against baseline

```bash
# Gate one profile
make test-perf PROFILE=full

# Gate every gated profile, reporting all failures
make test-perf-all
```

Exit code 1 means one of three things, and the output says which: a regression
against the baseline, a frame-budget breach, or a refusal to compare because the
baseline measured a different workload.

## CLI reference

`lidar-bench` always benchmarks; there is no mode flag to enable. Most runs go
through `make test-perf`, which builds the tool and compares against the
committed baseline.

| Flag                          | Alias | Default                 | Description                                       |
| ----------------------------- | ----- | ----------------------- | ------------------------------------------------- |
| `-pcap`                       | -     | (required)              | Path to PCAP file                                 |
| `-start-seconds`              | -     | `0`                     | Capture offset at which to begin replay           |
| `-duration-seconds`           | -     | `-1`                    | Replay duration (`-1` = remainder)                |
| `-benchmark-output`           | -     | `{pcap}_benchmark.json` | Output file for benchmark JSON results            |
| `-compare-baseline`           | -     | -                       | Compare against a baseline benchmark file         |
| `-regression-threshold`       | -     | `0.10` (10%)            | Threshold for flagging regressions                |
| `-quiet`                      | `-q`  | `false`                 | Suppress output to reduce measurement noise       |
| `-config`                     | -     | `config/tuning…json`    | Tuning config (falls back to embedded)            |
| `-sensor-id`                  | -     | from `l1.sensor`        | Sensor ID                                         |
| `-port`                       | -     | `0` (auto-detect)       | UDP port for LiDAR data                           |
| `-output`                     | -     | `.`                     | Output directory for benchmark JSON               |
| `-progress`                   | -     | `10`                    | Seconds between progress updates (0 = off)        |
| `-profile`                    | -     | from the config         | Reduce depth to `l3-only` or `detect`             |
| `-repeat`                     | -     | `1`                     | Run N times and emit the median run by wall clock |
| `-max-frames-over-budget-pct` | -     | `1.0`                   | Share of frames allowed past the frame budget     |

### Example commands

```bash
# Write a fresh baseline
./lidar-bench -pcap capture.pcapng -benchmark-output perf/baseline.json -quiet

# Compare with a stricter threshold (5% instead of 10%)
./lidar-bench -pcap capture.pcapng -compare-baseline baseline.json -regression-threshold 0.05
```

## Workflow examples

### Creating a baseline benchmark

Establish a baseline on the main branch before making changes:

```bash
# Checkout main branch
git checkout main

# Build and run baseline benchmark
go build -tags=pcap -o lidar-bench ./cmd/tools/lidar-bench
./lidar-bench -pcap data/gold-standard.pcapng -benchmark-output baseline.json -quiet

# Commit baseline for CI use
git add baseline.json
git commit -m "[go] add performance baseline for gold-standard.pcapng"
```

### Comparing in CI

After making algorithm changes, compare against the baseline:

```bash
# Build with your changes
go build -tags=pcap -o lidar-bench ./cmd/tools/lidar-bench

# Compare against baseline (exits with code 1 on regression)
./lidar-bench -pcap data/gold-standard.pcapng -compare-baseline baseline.json -quiet
```

### Interpreting results

**Successful comparison (no regression):**

```
========== Benchmark Comparison ==========
Baseline: baseline.json
Regression threshold: 10%

✓ No significant changes detected.
===========================================
```

**Regression detected:**

```
========== Benchmark Comparison ==========
Baseline: baseline.json
Regression threshold: 10%

⚠️  REGRESSIONS DETECTED:
  - frame_time_avg_ms: 2.45 → 3.12 (+27.3%)
  - cluster_time_ms: 145 → 198 (+36.6%)

=========================================
```

**Baseline refused (not a regression):**

```
[BASELINE REFUSED] baseline workload mismatch: the baseline predates pipeline
profiles and cannot state which layers it measured; the baseline carries no
tuning fingerprint; frames 0 vs 832; foreground_points 0 vs 1626922;
clusters 0 vs 14109
Not reporting a regression: these runs did not measure the same workload.
Regenerate the baseline for this profile before reading any comparison.
```

This is not a performance problem. The two runs measured different things, and any
delta between them would be meaningless. Recapture the baseline for that profile.

**Frame budget breached:**

```
[FRAME BUDGET] 40 frames (4.80%) exceeded the 98 ms budget, above the 1.00%
allowance; worst frame 320.5 ms
```

**Performance improvement:**

```
========== Benchmark Comparison ==========
Baseline: baseline.json
Regression threshold: 10%

✓ Improvements:
  - wall_clock_ms: 1523 → 1287 (-15.5%)
  - cluster_time_ms: 612 → 388 (-36.6%)

===========================================
```

### Which metrics are gated

`wall_clock_ms`, `frame_time_avg_ms`, `frame_time_p95_ms`, `heap_alloc_bytes`,
`total_alloc_bytes`, `cluster_time_ms`, `tracking_time_ms`.

`frames_per_second` is recorded but **not** gated: it is frames divided by the same
wall clock already gated, so including it reported one runner slowdown as two
independent regressions.

`heap_alloc_bytes` is live heap after a forced collection. Read without one it was
whatever the heap happened to be mid-GC-cycle — five runs of identical code spanned
18.8 to 40.0 MB. It now reads 17.0 MiB on every run of the full profile.

A metric whose baseline is zero is compared, not skipped. Zero to non-zero is an
unbounded increase and is reported as such. The old skip meant `cluster_time_ms` and
`tracking_time_ms` — the two fields that would have exposed the June 2026 baseline —
were the two never checked.

## Gold standard PCAP files

### Selection criteria

Choose PCAP files that provide comprehensive pipeline coverage:

1. **Representative traffic mix**: Include vehicles, pedestrians, and background activity
2. **Sufficient duration**: At least 60 seconds for stable statistics (600+ frames at 10 Hz)
3. **Edge cases**: Include complex scenes with multiple simultaneous objects
4. **Consistent sensor configuration**: Same sensor model and mounting as production

### Recommended test files

| File                      | Duration | Description                          | Use Case                           |
| ------------------------- | -------- | ------------------------------------ | ---------------------------------- |
| `gold-standard.pcapng`    | 2 min    | Mixed traffic, urban intersection    | Primary regression testing         |
| `high-density.pcapng`     | 1 min    | Rush hour, 10+ simultaneous vehicles | Stress testing clustering/tracking |
| `pedestrian-focus.pcapng` | 1 min    | School zone, multiple pedestrians    | Classification accuracy            |
| `quiet-baseline.pcapng`   | 30 sec   | Empty street, background only        | Background model validation        |

### Maintenance guidelines

- **Version control**: Store gold standard PCAPs in `data/` or a shared storage location
- **Document provenance**: Record capture date, location, and sensor configuration
- **Periodic refresh**: Update files annually or when sensor models change
- **Size limits**: Keep files under 500 MB for reasonable CI run times

## CI integration

### GitHub actions example

A GitHub Actions workflow triggers on pull requests that modify `internal/lidar/**` or `cmd/tools/lidar-bench/**`. The job runs on `ubuntu-latest` with Go 1.22 and `libpcap-dev` installed. Steps:

1. Check out the repository.
2. Build `lidar-bench` with the `pcap` build tag.
3. Download the gold standard PCAP file from shared storage (`$PCAP_STORAGE_URL`).
4. Run the performance benchmark with `-compare-baseline` pointing at the committed baseline JSON; the step fails on regression.
5. Upload the benchmark results JSON as a build artifact (always, regardless of pass/fail).

### Baseline management

Store baselines in the repository for reproducibility:

```
baselines/
├── gold-standard-baseline.json
├── high-density-baseline.json
└── README.md  # Documents baseline creation date and hardware
```

Update baselines when:

- Intentional performance improvements are merged
- Hardware or Go version changes affect measurements
- Gold standard PCAP files are updated

## Understanding metrics

### Benchmark JSON schema (v1.0)

The benchmark output file (version `1.0`) contains three top-level sections:

**Top-level fields:**

| Field       | Type   | Description                        |
| ----------- | ------ | ---------------------------------- |
| `version`   | string | Schema version (`"1.0"`)           |
| `timestamp` | string | ISO 8601 time of the benchmark run |
| `pcap_file` | string | PCAP filename used                 |

**`system_info` section:**

| Field         | Type   | Description                 |
| ------------- | ------ | --------------------------- |
| `goos`        | string | OS (e.g. `linux`)           |
| `goarch`      | string | Architecture (e.g. `amd64`) |
| `num_cpu`     | int    | CPU count                   |
| `go_version`  | string | Go version                  |
| `commit_hash` | string | Git commit SHA              |

**`metrics` section:**

| Field                | Type  | Description                                                       |
| -------------------- | ----- | ----------------------------------------------------------------- |
| `wall_clock_ms`      | int   | Total processing time                                             |
| `frame_time_stats.*` | float | Per-frame stats: min, max, avg, p50, p95, p99 (ms), samples (int) |
| `frames_per_second`  | float | Processing throughput                                             |
| `packets_per_second` | float | Packet parsing rate                                               |
| `points_per_second`  | float | Point cloud throughput                                            |
| `heap_alloc_bytes`   | int   | Current heap memory                                               |
| `total_alloc_bytes`  | int   | Cumulative allocation                                             |
| `num_gc`             | int   | GC cycle count                                                    |
| `gc_pause_ns`        | int   | GC pause duration                                                 |
| `pipeline_time_ms`   | int   | Pipeline stage time                                               |
| `cluster_time_ms`    | int   | Clustering stage time                                             |
| `tracking_time_ms`   | int   | Tracking stage time                                               |
| `classify_time_ms`   | int   | Classification stage time                                         |

**`comparison` section:** Contains `baseline_file` (string), `regressions` (array), and `improvements` (array).

### Metric descriptions

| Metric                    | Unit  | Description                               | Regression Indicator                      |
| ------------------------- | ----- | ----------------------------------------- | ----------------------------------------- |
| `wall_clock_ms`           | ms    | Total processing time                     | Higher is worse                           |
| `frame_time_stats.avg_ms` | ms    | Average per-frame processing time         | Higher is worse                           |
| `frame_time_stats.p95_ms` | ms    | 95th percentile frame time (tail latency) | Higher is worse                           |
| `frames_per_second`       | FPS   | Processing throughput                     | Lower is worse                            |
| `packets_per_second`      | pkt/s | Packet parsing rate                       | Lower is worse                            |
| `points_per_second`       | pt/s  | Point cloud throughput                    | Lower is worse                            |
| `heap_alloc_bytes`        | bytes | Current heap memory usage                 | Higher may indicate leak                  |
| `total_alloc_bytes`       | bytes | Cumulative allocations                    | Significant increase indicates concern    |
| `num_gc`                  | count | Garbage collection cycles                 | Many GCs may indicate allocation pressure |
| `gc_pause_ns`             | ns    | Total GC pause time                       | Higher causes frame drops                 |
| `pipeline_time_ms`        | ms    | Total PCAP reading + frame processing     | Higher is worse                           |
| `cluster_time_ms`         | ms    | DBSCAN clustering time                    | Higher is worse                           |
| `tracking_time_ms`        | ms    | Kalman filter tracking time               | Higher is worse                           |
| `classify_time_ms`        | ms    | Object classification time                | Higher is worse                           |

### Pipeline stage analysis

The pipeline stage timing helps identify where regressions occur:

```
Total Processing Time
├── pipeline_time_ms   — PCAP reading, packet parsing, frame processing
├── cluster_time_ms    — DBSCAN clustering of foreground points
├── tracking_time_ms   — Kalman filter update and track management
└── classify_time_ms   — Object classification (vehicle, pedestrian, etc.)
```

When a regression is detected:

1. Check which pipeline stage increased
2. Review recent changes to that subsystem
3. Profile that stage in isolation if needed

## Troubleshooting

### Common issues

**Build error: missing libpcap**

```
# Linux (Debian/Ubuntu)
sudo apt-get install libpcap-dev

# macOS
brew install libpcap
```

`lidar-bench` always requires the `pcap` build tag; build it with
`go build -tags=pcap -o lidar-bench ./cmd/tools/lidar-bench`.

**Baseline comparison fails with "file not found"**

Ensure the baseline file path is correct relative to the working directory:

```bash
# Check file exists
ls -la baseline.json

# Use absolute path if needed
./lidar-bench -pcap data/test.pcapng -compare-baseline /full/path/to/baseline.json
```

**Inconsistent benchmark results**

Reduce noise by:

1. Using `-quiet` flag to suppress output
2. Closing other applications during benchmarking
3. Running multiple iterations and averaging
4. Using dedicated CI runners with consistent hardware

```bash
# Run 3 iterations and compare
for i in 1 2 3; do
  ./lidar-bench -pcap data/gold-standard.pcapng -quiet \
    -benchmark-output "run-$i.json"
done
```

**False positive regressions**

If hardware or environment changes cause expected differences:

1. Verify the change is environmental (different CPU, Go version)
2. Re-establish baseline on the new environment
3. Document the environment in baseline metadata

```bash
# Create new baseline after environment change
./lidar-bench -pcap data/gold-standard.pcapng -quiet \
  -benchmark-output baseline.json

# Add note about environment change
echo "Baseline updated for Go 1.22 and new CI runner" >> baselines/CHANGELOG.md
```

**Exit code 1 but no output**

When using `-quiet`, comparison results are still printed. Check stderr:

```bash
./lidar-bench -pcap data/test.pcapng -compare-baseline baseline.json -quiet 2>&1
```

### Debugging performance issues

**Identify slow frames:**

The `p99_ms` metric highlights worst-case performance. If p99 is much higher than avg:

```bash
# Run without -quiet to print the full benchmark summary
./lidar-bench -pcap data/problem.pcapng
```

**Memory investigation:**

High `total_alloc_bytes` or many GC cycles suggest allocation pressure:

```bash
# Run with memory profiling
GODEBUG=gctrace=1 ./lidar-bench -pcap data/test.pcapng 2>&1 | grep gc
```

**Pipeline stage profiling:**

If a specific stage regresses, use Go's built-in profiling:

```bash
# CPU profile
go build -tags=pcap -o lidar-bench ./cmd/tools/lidar-bench
./lidar-bench -pcap data/test.pcapng -quiet &
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30
```

## See also

- [PCAP Analysis Mode](pcap-analysis-mode.md): scan, motion stats, and splits via `pcap-split`
- [LiDAR Architecture](../architecture/LIDAR_ARCHITECTURE.md): Pipeline architecture
- [Foreground Tracking Plan](../architecture/foreground-tracking.md): Algorithm details
