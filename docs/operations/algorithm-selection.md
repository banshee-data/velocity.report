# Algorithm Selection Guide

This document describes how to configure and use different foreground extraction algorithms in the LIDAR tracking pipeline.

## Overview

The velocity.report LIDAR system supports multiple foreground extraction algorithms:

| Algorithm | Description | Use Case |
|-----------|-------------|----------|
| `background` | Background subtraction using EMA-based model | Default, stable for most scenes |
| `velocity` | Velocity-coherent extraction using frame-to-frame correspondence | Sparse objects, trail elimination |
| `hybrid` | Runs both algorithms and merges results | Maximum detection coverage |

## Configuration

### TrackingPipelineConfig Options

When creating a `TrackingPipelineConfig`, you can specify the extractor mode:

```go
cfg := &lidar.TrackingPipelineConfig{
    BackgroundManager: bgManager,
    Tracker:           tracker,
    SensorID:          "lidar-01",
    
    // Algorithm selection
    ExtractorMode:   "hybrid",     // "background", "velocity", or "hybrid"
    HybridMergeMode: "union",      // "union", "intersection", or "primary"
}
```

### Extractor Modes

#### Background Subtraction (`background` or empty)

The default algorithm. Uses an EMA-based background model to detect points that deviate from expected static background.

**Pros:**
- Memory efficient
- Real-time capable
- Stable for static scenes
- Well-tested

**Cons:**
- Trail artifacts after vehicles pass (EMA reconvergence delay)
- High MinPts (12) can miss sparse distant objects
- Freeze mechanism creates 5-second foreground windows

#### Velocity-Coherent (`velocity`)

Tracks motion patterns across frames using point correspondence and velocity estimation.

**Pros:**
- No trail artifacts
- Works with MinPts=3 (velocity confirms cluster identity)
- Captures sparse distant objects
- Pre-entry and post-exit tracking

**Cons:**
- Requires multi-frame history (more memory)
- May miss stationary objects
- Cannot run on first frame

#### Hybrid (`hybrid`)

Runs both algorithms in parallel and merges results.

**Merge Modes:**
- `union`: Point is foreground if ANY algorithm detects it (maximum coverage)
- `intersection`: Point is foreground if ALL algorithms agree (maximum precision)
- `primary`: Uses background subtraction with velocity as fallback

**Pros:**
- Maximum detection coverage
- Redundancy against algorithm failures
- Enables A/B comparison

**Cons:**
- Higher CPU usage (runs 2 algorithms)
- Union mode may increase false positives

### Custom Extractor

You can also inject a completely custom extractor:

```go
customExtractor := &MyCustomExtractor{...}

cfg := &lidar.TrackingPipelineConfig{
    BackgroundManager:   bgManager,
    ForegroundExtractor: customExtractor,  // Overrides ExtractorMode
    // ...
}
```

## CLI Usage

### algo-compare Tool

The `algo-compare` tool runs algorithm comparison on PCAP files:

```bash
# Build with pcap support
make build-tools-pcap

# Run comparison
./algo-compare -pcap capture.pcap -output results/ -json comparison.json

# Options:
#   -pcap string     Path to PCAP file to analyze
#   -output string   Output directory for results
#   -sensor string   Sensor ID (default: lidar-01)
#   -port int        UDP port to filter (default: 2368)
#   -merge string    Merge mode: union, intersection, primary (default: union)
#   -json string     Output JSON filename
#   -verbose         Enable verbose logging
```

### Output

The tool outputs:
- Per-algorithm foreground/background counts
- Processing time comparison
- Agreement percentage between algorithms
- JSON export with detailed metrics

## Monitoring

### Web API Endpoints

When running with hybrid mode, additional metrics are available:

```
GET /api/lidar/extractor/status
{
  "mode": "hybrid",
  "merge_mode": "union",
  "extractors": ["background_subtraction", "velocity_coherent"],
  "per_extractor": {
    "background_subtraction": {
      "foreground_ratio": 0.08,
      "avg_processing_us": 1200
    },
    "velocity_coherent": {
      "foreground_ratio": 0.05,
      "avg_processing_us": 2400
    }
  },
  "agreement_pct": 87.5
}
```

### Metrics to Watch

| Metric | Normal Range | Alert Threshold |
|--------|--------------|-----------------|
| `foreground_ratio` | 5-40% | >50% or <1% |
| `agreement_pct` | 80-99% | <70% |
| `processing_time_us` | 1000-5000 | >10000 |

## Troubleshooting

### High Foreground Ratio

If foreground ratio is consistently >40%:
1. Check if background has settled (`settling_complete: true`)
2. Increase `WarmupMinFrames` if scene is complex
3. Consider reducing `ClosenessSensitivityMultiplier`

### Low Agreement Between Algorithms

If agreement drops below 70%:
1. Check time delta between frames (should be ~100ms at 10Hz)
2. Verify sensor is outputting consistent frames
3. Consider using `intersection` merge mode for higher precision

### Trails Persisting with Hybrid Mode

If trails still appear with hybrid mode:
1. Ensure `MergeMode` is set to `union`
2. Check velocity-coherent extractor is getting valid correspondences
3. Verify `SearchRadius` in velocity config (default: 2.0m)

## Migration Path

### From Background-Only to Hybrid

1. **Testing phase:**
   ```go
   cfg.ExtractorMode = "hybrid"
   cfg.HybridMergeMode = "union"
   cfg.DebugMode = true  // Log comparison metrics
   ```

2. **Monitor for 24h:**
   - Check agreement percentage
   - Verify no increase in false positives
   - Confirm trail reduction

3. **Production deployment:**
   - Keep `DebugMode = false` to reduce logging
   - Monitor `foreground_ratio` for anomalies

### Rollback

To revert to background-only:
```go
cfg.ExtractorMode = ""  // or "background"
```

No restart required if using runtime parameter updates.

## Related Documentation

- [Foreground Trails Investigation](../troubleshooting/foreground-trails-deep-investigation-20260108.md)
- [Velocity-Coherent Algorithm Design](../../internal/lidar/docs/future/velocity-coherent-foreground-extraction.md)
- [Foreground Tracking Plan](../../internal/lidar/docs/architecture/foreground_tracking_plan.md)
