# Performance Regression Testing

## Overview

The `pcap-analyze` tool includes a performance benchmarking mode to detect regressions when modifying the LIDAR processing pipeline. This ensures that algorithm improvements, new features, or refactoring don't inadvertently degrade processing speed.

**Why performance testing matters:**

- Real-time processing requires consistent throughput (≥10 FPS for Pandar40P)
- Memory usage affects edge deployment on resource-constrained devices
- Pipeline stage timing helps identify bottlenecks during optimization
- Regression detection prevents performance issues from reaching production

## Quick Start

### Create a Baseline Benchmark

```bash
# Build the tool (requires libpcap)
go build -tags=pcap -o pcap-analyze ./cmd/tools/pcap-analyze

# Run benchmark on a gold standard PCAP file
./pcap-analyze -pcap data/gold-standard.pcapng -benchmark -benchmark-output baseline.json -quiet
```

### Compare Against Baseline

```bash
# Run benchmark and compare against baseline
./pcap-analyze -pcap data/gold-standard.pcapng -benchmark -compare-baseline baseline.json -quiet

# Exit code 1 if regression detected
echo "Exit code: $?"
```

## CLI Reference

### Benchmark Flags

| Flag | Alias | Default | Description |
|------|-------|---------|-------------|
| `-benchmark` | `-bench` | `false` | Enable performance measurement mode |
| `-benchmark-output` | — | `{pcap}_benchmark.json` | Output file for benchmark JSON results |
| `-quiet` | `-q` | `false` | Suppress output to reduce measurement noise |
| `-compare-baseline` | — | — | Compare against a baseline benchmark file |
| `-regression-threshold` | — | `0.10` (10%) | Threshold for flagging regressions |

### Standard Flags (also available in benchmark mode)

| Flag | Default | Description |
|------|---------|-------------|
| `-pcap` | (required) | Path to PCAP file |
| `-output` | `.` | Output directory for results |
| `-sensor-id` | `hesai-pandar40p` | Sensor ID for configuration |
| `-port` | `2369` | UDP port for LIDAR data |
| `-fps` | `10.0` | Expected frame rate in Hz |

### Example Commands

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

## Workflow Examples

### Creating a Baseline Benchmark

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

### Interpreting Results

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

## Gold Standard PCAP Files

### Selection Criteria

Choose PCAP files that provide comprehensive pipeline coverage:

1. **Representative traffic mix** — Include vehicles, pedestrians, and background activity
2. **Sufficient duration** — At least 60 seconds for stable statistics (600+ frames at 10 Hz)
3. **Edge cases** — Include complex scenes with multiple simultaneous objects
4. **Consistent sensor configuration** — Same sensor model and mounting as production

### Recommended Test Files

| File | Duration | Description | Use Case |
|------|----------|-------------|----------|
| `gold-standard.pcapng` | 2 min | Mixed traffic, urban intersection | Primary regression testing |
| `high-density.pcapng` | 1 min | Rush hour, 10+ simultaneous vehicles | Stress testing clustering/tracking |
| `pedestrian-focus.pcapng` | 1 min | School zone, multiple pedestrians | Classification accuracy |
| `quiet-baseline.pcapng` | 30 sec | Empty street, background only | Background model validation |

### Maintenance Guidelines

- **Version control** — Store gold standard PCAPs in `data/` or a shared storage location
- **Document provenance** — Record capture date, location, and sensor configuration
- **Periodic refresh** — Update files annually or when sensor models change
- **Size limits** — Keep files under 500 MB for reasonable CI run times

## CI Integration

### GitHub Actions Example

```yaml
name: Performance Regression Test

on:
  pull_request:
    paths:
      - 'internal/lidar/**'
      - 'cmd/tools/pcap-analyze/**'

jobs:
  benchmark:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.22'

      - name: Install libpcap
        run: sudo apt-get update && sudo apt-get install -y libpcap-dev

      - name: Build pcap-analyze
        run: go build -tags=pcap -o pcap-analyze ./cmd/tools/pcap-analyze

      - name: Download gold standard PCAP
        run: |
          # Download from shared storage or use cached file
          curl -L -o gold-standard.pcapng "${{ secrets.PCAP_STORAGE_URL }}/gold-standard.pcapng"

      - name: Run performance benchmark
        run: |
          ./pcap-analyze -pcap gold-standard.pcapng \
            -benchmark \
            -compare-baseline baselines/gold-standard-baseline.json \
            -quiet
        continue-on-error: false

      - name: Upload benchmark results
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: benchmark-results
          path: gold-standard_benchmark.json
```

### Baseline Management

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

## Understanding Metrics

### Benchmark JSON Schema (v1.0)

```json
{
  "version": "1.0",
  "timestamp": "2024-01-15T10:30:00Z",
  "pcap_file": "gold-standard.pcapng",
  "system_info": {
    "goos": "linux",
    "goarch": "amd64",
    "num_cpu": 8,
    "go_version": "go1.22.0",
    "commit_hash": "abc123def456"
  },
  "metrics": {
    "wall_clock_ms": 1523,
    "frame_time_stats": {
      "min_ms": 0.45,
      "max_ms": 12.34,
      "avg_ms": 2.45,
      "p50_ms": 2.12,
      "p95_ms": 5.67,
      "p99_ms": 8.91,
      "samples": 1200
    },
    "frames_per_second": 164.2,
    "packets_per_second": 12500.0,
    "points_per_second": 875000.0,
    "heap_alloc_bytes": 52428800,
    "total_alloc_bytes": 104857600,
    "num_gc": 12,
    "gc_pause_ns": 450000,
    "parse_time_ms": 234,
    "cluster_time_ms": 456,
    "tracking_time_ms": 321,
    "classify_time_ms": 45
  },
  "comparison": {
    "baseline_file": "baseline.json",
    "regressions": [],
    "improvements": []
  }
}
```

### Metric Descriptions

| Metric | Unit | Description | Regression Indicator |
|--------|------|-------------|---------------------|
| `wall_clock_ms` | ms | Total processing time | Higher is worse |
| `frame_time_stats.avg_ms` | ms | Average per-frame processing time | Higher is worse |
| `frame_time_stats.p95_ms` | ms | 95th percentile frame time (tail latency) | Higher is worse |
| `frames_per_second` | FPS | Processing throughput | Lower is worse |
| `packets_per_second` | pkt/s | Packet parsing rate | Lower is worse |
| `points_per_second` | pt/s | Point cloud throughput | Lower is worse |
| `heap_alloc_bytes` | bytes | Current heap memory usage | Higher may indicate leak |
| `total_alloc_bytes` | bytes | Cumulative allocations | Significant increase indicates concern |
| `num_gc` | count | Garbage collection cycles | Many GCs may indicate allocation pressure |
| `gc_pause_ns` | ns | Total GC pause time | Higher causes frame drops |
| `parse_time_ms` | ms | PCAP parsing + frame assembly | Higher is worse |
| `cluster_time_ms` | ms | DBSCAN clustering time | Higher is worse |
| `tracking_time_ms` | ms | Kalman filter tracking time | Higher is worse |
| `classify_time_ms` | ms | Object classification time | Higher is worse |

### Pipeline Stage Analysis

The pipeline stage timing helps identify where regressions occur:

```
Total Processing Time
├── parse_time_ms      — PCAP reading, packet parsing, frame assembly
├── cluster_time_ms    — DBSCAN clustering of foreground points
├── tracking_time_ms   — Kalman filter update and track management
└── classify_time_ms   — Object classification (vehicle, pedestrian, etc.)
```

When a regression is detected:

1. Check which pipeline stage increased
2. Review recent changes to that subsystem
3. Profile that stage in isolation if needed

## Troubleshooting

### Common Issues

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

### Debugging Performance Issues

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

## See Also

- [PCAP Analysis Mode](pcap-analysis-mode.md) — Using pcap-analyze for track extraction
- [LIDAR Sidecar Overview](../architecture/lidar_sidecar_overview.md) — Pipeline architecture
- [Foreground Tracking Plan](../architecture/foreground_tracking_plan.md) — Algorithm details
