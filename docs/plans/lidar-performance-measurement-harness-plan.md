# LiDAR Pipeline Performance Measurement Harness Plan

- **Status:** Proposed
- **Layers:** Cross-cutting (L1–L6, CI infrastructure)
- **Related:** [Clustering Observability and Benchmark Plan](lidar-clustering-observability-and-benchmark-plan.md), [Pipeline Review Q10](../../data/maths/pipeline-review-open-questions.md)

## Goal

Provide consistent, reproducible measurements of pipeline performance
(throughput, per-layer latency, frame drops) on fixed PCAPs for test
hardware. Integrate with CI to detect performance regressions
automatically.

## Current state

The existing `make test-perf` target runs `pcap-analyse` on a named PCAP
(default: kirk0) and compares timing against a baseline JSON file. The
nightly CI job (`nightly-full-ci.yml`) uploads benchmark results as
artefacts. This provides end-to-end throughput but lacks per-layer
breakdowns and hardware-specific regression detection.

## Proposed extensions

### 1. Per-layer timing instrumentation

Add timing instrumentation to each pipeline layer so that the benchmark
report includes per-layer breakdowns:

| Layer | Operation                           | Measured as                                      |
| ----- | ----------------------------------- | ------------------------------------------------ |
| L1–L2 | Packet parse + frame assembly       | Wall-clock time from first packet to frame ready |
| L3    | Background update + foreground mask | Wall-clock time for `ProcessFramePolarWithMask`  |
| L4    | Ground filter + DBSCAN + OBB        | Wall-clock time for full L4 pass                 |
| L5    | Kalman predict + Hungarian + update | Wall-clock time for tracker step                 |
| L6    | Classification                      | Wall-clock time for classify pass                |
| Total | End-to-end frame processing         | Sum of above                                     |

Each timing is recorded per-frame and aggregated as: mean, p50, p95, p99,
max, and standard deviation. The benchmark JSON output includes both
per-layer and total statistics.

### 2. Frame drop and lag detection

Record the number of frames where total processing time exceeds the frame
budget (100 ms at 10 Hz). Report:

- **Drop count:** Frames where processing exceeded budget
- **Drop rate:** Drop count / total frames (target: < 1%)
- **Max lag:** Longest single-frame processing time
- **Lag distribution:** Histogram of frame processing times

### 3. Hardware-specific baselines

Maintain separate baseline files for each test hardware platform:

| Platform                   | Baseline file             | Where it runs          |
| -------------------------- | ------------------------- | ---------------------- |
| CI (GitHub Actions runner) | `baseline-{name}-ci.json` | Nightly CI             |
| Raspberry Pi 4             | `baseline-{name}-pi.json` | Manual or scheduled    |
| Development machine        | `baseline-{name}.json`    | Local `make test-perf` |

Regression thresholds are platform-specific because absolute timings
differ. The CI baseline detects relative regressions (> 15% slower than
baseline) rather than absolute budget violations.

### 4. CI integration

The harness runs as part of the existing nightly CI job. Extensions:

1. Run `make test-perf` for each PCAP in the test corpus (initially kirk0,
   expanding to 5 PCAPs as the corpus grows).
2. Compare per-layer timings against CI baselines.
3. Report per-layer regressions (> 15% slower) as warnings.
4. Report end-to-end regressions (> 10% slower) as failures.
5. Upload per-layer benchmark JSON as build artefacts.

### 5. Benchmark JSON schema

Extend the current benchmark output with per-layer fields:

```json
{
  "name": "kirk0",
  "platform": "ci",
  "total_frames": 3000,
  "total_duration_ms": 45000,
  "fps": 66.7,
  "frame_budget_ms": 100,
  "drops": 0,
  "drop_rate": 0.0,
  "layers": {
    "l1l2_parse": {
      "mean_ms": 1.2,
      "p50_ms": 1.1,
      "p85_ms": 1.8,
      "p98_ms": 2.5,
      "max_ms": 4.1
    },
    "l3_background": {
      "mean_ms": 3.8,
      "p50_ms": 3.5,
      "p85_ms": 5.2,
      "p98_ms": 6.1,
      "max_ms": 8.3
    },
    "l4_perception": {
      "mean_ms": 8.4,
      "p50_ms": 7.9,
      "p85_ms": 12.1,
      "p98_ms": 15.0,
      "max_ms": 18.2
    },
    "l5_tracking": {
      "mean_ms": 3.1,
      "p50_ms": 2.8,
      "p85_ms": 4.5,
      "p98_ms": 5.8,
      "max_ms": 7.1
    },
    "l6_classification": {
      "mean_ms": 0.3,
      "p50_ms": 0.2,
      "p85_ms": 0.5,
      "p98_ms": 0.7,
      "max_ms": 1.0
    }
  }
}
```

## Implementation phases

| Phase | Work                                               | Effort       |
| ----- | -------------------------------------------------- | ------------ |
| 1     | Add per-layer timing to `pcap-analyse` pipeline    | S (1–2 days) |
| 2     | Extend benchmark JSON schema with per-layer fields | S (1 day)    |
| 3     | Add frame drop/lag detection and reporting         | S (1 day)    |
| 4     | Update CI job to run per-layer comparisons         | S (1 day)    |
| 5     | Create Pi 4 baseline via manual capture            | S (1 day)    |
| 6     | Extend to full 5-PCAP corpus as captures arrive    | Ongoing      |

## Non-goals

- Real-time performance monitoring during live deployment (separate
  concern; see metrics-registry plan)
- Micro-benchmarks for individual functions (covered by Go benchmark
  tests)
- Memory profiling (separate tooling; add if needed)

## References

- [Clustering observability plan](lidar-clustering-observability-and-benchmark-plan.md) — clustering-specific benchmarks
- [Test corpus plan](lidar-test-corpus-plan.md) — five-site PCAP corpus
- [Config evidence levels](../../config/README.maths.md) — evidence classification and sweep experiments
