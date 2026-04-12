# Performance regression testing

Performance benchmarking mode in the `pcap-analyze` tool, used to detect processing speed regressions in the LiDAR pipeline before they reach production.

## Overview

The `pcap-analyze` tool includes a performance benchmarking mode to detect regressions when modifying the LIDAR processing pipeline. This ensures that algorithm improvements, new features, or refactoring don't inadvertently degrade processing speed.

**Why performance testing matters:**

- Real-time processing requires consistent throughput (≥10 FPS for Pandar40P)
- Memory usage affects edge deployment on resource-constrained devices
- Pipeline stage timing helps identify bottlenecks during optimisation
- Regression detection prevents performance issues from reaching production

## Quick start

### Create a baseline benchmark

```bash
# Build the tool (requires libpcap)
go build -tags=pcap -o pcap-analyze ./cmd/tools/pcap-analyze

# Run benchmark on a gold standard PCAP file
./pcap-analyze -pcap data/gold-standard.pcapng -benchmark -benchmark-output baseline.json -quiet
```

### Compare against baseline

```bash
# Run benchmark and compare against baseline
./pcap-analyze -pcap data/gold-standard.pcapng -benchmark -compare-baseline baseline.json -quiet

# Exit code 1 if regression detected
echo "Exit code: $?"
```

## CLI reference

### Benchmark flags

| Flag                    | Alias    | Default                 | Description                                 |
| ----------------------- | -------- | ----------------------- | ------------------------------------------- |
| `-benchmark`            | `-bench` | `false`                 | Enable performance measurement mode         |
| `-benchmark-output`     | -        | `{pcap}_benchmark.json` | Output file for benchmark JSON results      |
| `-quiet`                | `-q`     | `false`                 | Suppress output to reduce measurement noise |
| `-compare-baseline`     | -        | -                       | Compare against a baseline benchmark file   |
| `-regression-threshold` | -        | `0.10` (10%)            | Threshold for flagging regressions          |

### Standard flags (also available in benchmark mode)

| Flag         | Default           | Description                  |
| ------------ | ----------------- | ---------------------------- |
| `-pcap`      | (required)        | Path to PCAP file            |
| `-output`    | `.`               | Output directory for results |
| `-sensor-id` | `hesai-pandar40p` | Sensor ID for configuration  |
| `-port`      | `2369`            | UDP port for LIDAR data      |
| `-fps`       | `10.0`            | Expected frame rate in Hz    |

### Example commands

```bash
# Basic benchmark with verbose output
./pcap-analyze -pcap capture.pcapng -benchmark

# Quiet benchmark with custom output path
./pcap-analyze -pcap capture.pcapng -benchmark -quiet -benchmark-output perf/baseline.json

# Compare with stricter threshold (5% instead of 10%)
./pcap-analyze -pcap capture.pcapng -benchmark -compare-baseline baseline.json -regression-threshold 0.05

# Short form aliases
./pcap-analyze -pcap capture.pcapng -bench -q -benchmark-output perf.json
```

## Workflow examples

### Creating a baseline benchmark

Establish a baseline on the main branch before making changes:

```bash
# Checkout main branch
git checkout main

# Build and run baseline benchmark
go build -tags=pcap -o pcap-analyze ./cmd/tools/pcap-analyze
./pcap-analyze -pcap data/gold-standard.pcapng -benchmark -benchmark-output baseline.json -quiet

# Commit baseline for CI use
git add baseline.json
git commit -m "[go] add performance baseline for gold-standard.pcapng"
```

### Comparing in CI

After making algorithm changes, compare against the baseline:

```bash
# Build with your changes
go build -tags=pcap -o pcap-analyze ./cmd/tools/pcap-analyze

# Compare against baseline (exits with code 1 on regression)
./pcap-analyze -pcap data/gold-standard.pcapng -benchmark -compare-baseline baseline.json -quiet
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

**Performance improvement:**

```
========== Benchmark Comparison ==========
Baseline: baseline.json
Regression threshold: 10%

✓ Improvements:
  - wall_clock_ms: 1523 → 1287 (-15.5%)
  - frames_per_second: 164.2 → 194.6 (+18.5%)

===========================================
```

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

A GitHub Actions workflow triggers on pull requests that modify `internal/lidar/**` or `cmd/tools/pcap-analyze/**`. The job runs on `ubuntu-latest` with Go 1.22 and `libpcap-dev` installed. Steps:

1. Check out the repository.
2. Build `pcap-analyze` with the `pcap` build tag.
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

# Build without pcap support (for non-benchmark use)
go build -o pcap-analyze ./cmd/tools/pcap-analyze
```

**Baseline comparison fails with "file not found"**

Ensure the baseline file path is correct relative to the working directory:

```bash
# Check file exists
ls -la baseline.json

# Use absolute path if needed
./pcap-analyze -pcap data/test.pcapng -compare-baseline /full/path/to/baseline.json
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
  ./pcap-analyze -pcap data/gold-standard.pcapng -benchmark -quiet \
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
./pcap-analyze -pcap data/gold-standard.pcapng -benchmark -quiet \
  -benchmark-output baseline.json

# Add note about environment change
echo "Baseline updated for Go 1.22 and new CI runner" >> baselines/CHANGELOG.md
```

**Exit code 1 but no output**

When using `-quiet`, comparison results are still printed. Check stderr:

```bash
./pcap-analyze -pcap data/test.pcapng -benchmark -compare-baseline baseline.json -quiet 2>&1
```

### Debugging performance issues

**Identify slow frames:**

The `p99_ms` metric highlights worst-case performance. If p99 is much higher than avg:

```bash
# Run without quiet to see per-frame stats
./pcap-analyze -pcap data/problem.pcapng -benchmark -v
```

**Memory investigation:**

High `total_alloc_bytes` or many GC cycles suggest allocation pressure:

```bash
# Run with memory profiling
GODEBUG=gctrace=1 ./pcap-analyze -pcap data/test.pcapng -benchmark 2>&1 | grep gc
```

**Pipeline stage profiling:**

If a specific stage regresses, use Go's built-in profiling:

```bash
# CPU profile
go build -tags=pcap -o pcap-analyze ./cmd/tools/pcap-analyze
./pcap-analyze -pcap data/test.pcapng -benchmark -quiet &
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30
```

## See also

- [PCAP Analysis Mode](pcap-analysis-mode.md): Using pcap-analyse for track extraction
- [LIDAR Sidecar Overview](../architecture/lidar-sidecar-overview.md): Pipeline architecture
- [Foreground Tracking Plan](../architecture/foreground-tracking.md): Algorithm details
