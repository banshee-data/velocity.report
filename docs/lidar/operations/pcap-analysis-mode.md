# PCAP analysis mode

PCAP analysis mode replays captured packet data through the LiDAR pipeline while preserving the background grid state for offline inspection and tuning.

## Overview

The LiDAR system supports two modes for PCAP replay:

1. **Normal PCAP Replay** - Replays PCAP file, then automatically resets grid and returns to live data
2. **PCAP Analysis Mode** - Replays PCAP file and **preserves the background grid** for inspection and analysis

## Use cases

### Normal replay mode

- Testing sensor configurations
- Debugging packet parsing issues
- Verifying frame assembly
- Quick validation before returning to live monitoring

### Analysis mode

- **Track Analysis** - Identify and study vehicle trajectories from historical data
- **Background Characterization** - Build accurate background models from known-quiet periods
- **Object Detection Tuning** - Analyse detection thresholds with real-world data
- **Scene Comparison** - Compare PCAP-analysed background with live data overlay
- **Historical Investigation** - Study specific incidents or traffic patterns

## API endpoints

### Start PCAP replay (analysis mode)

```bash
POST /api/lidar/pcap/start?sensor_id=hesai-pandar40p
Content-Type: application/json

{
  "pcap_file": "break-80k.pcapng",
  "analysis_mode": true
}
```

**Response:** returns `status: "started"`, `sensor_id`, `current_source: "pcap"`, `pcap_file` (resolved path), and `analysis_mode: true`.

### Check data source status

```bash
GET /api/lidar/data_source?sensor_id=hesai-pandar40p
```

**Response:** returns `status: "ok"`, `data_source` (one of the values below), `pcap_file` (path), `pcap_in_progress` (boolean), and `analysis_mode` (boolean).

Data source values:

- `live` - Normal live UDP data collection
- `pcap` - PCAP replay in progress
- `pcap_analysis` - PCAP replay completed in analysis mode (grid preserved)

### Resume live data (preserve grid)

```bash
POST /api/lidar/pcap/resume_live?sensor_id=hesai-pandar40p
```

Switches from PCAP analysis mode back to live UDP data **without resetting the grid**. This allows you to overlay live traffic on top of the PCAP-analysed background.

**Response:** returns `status: "resumed_live"`, `sensor_id`, `current_source: "live"`, and `grid_preserved: true`.

### Stop PCAP replay (reset grid)

```bash
POST /api/lidar/pcap/stop?sensor_id=hesai-pandar40p
```

Stops PCAP replay and **resets the grid** before returning to live data.

## Workflow examples

### Basic analysis workflow

1. **Start analysis mode replay:**

   ```bash
   curl -X POST "http://localhost:8082/api/lidar/pcap/start?sensor_id=hesai-pandar40p" \
     -H "Content-Type: application/json" \
     -d '{"pcap_file":"break-80k.pcapng","analysis_mode":true}'
   ```

2. **Wait for completion and inspect results:**
   - Check grid status: `/api/lidar/grid_status?sensor_id=hesai-pandar40p`
   - View heatmap: `/api/lidar/grid_heatmap?sensor_id=hesai-pandar40p`
   - Export snapshot: `/api/lidar/export_snapshot?sensor_id=hesai-pandar40p`

3. **Resume live data (preserving PCAP background):**

   ```bash
   curl "http://localhost:8082/api/lidar/pcap/resume_live?sensor_id=hesai-pandar40p"
   ```

4. **When done, reset grid:**
   ```bash
   curl "http://localhost:8082/api/lidar/grid_reset?sensor_id=hesai-pandar40p"
   ```

### Comparative analysis

1. Load PCAP in analysis mode to build background model
2. Resume live data (grid preserved)
3. Live objects will be detected against PCAP-built background
4. Useful for comparing "then vs now" traffic patterns

### Scene reconstruction

1. Process PCAP with empty parking lot → builds clean background
2. Resume live data with grid preserved
3. All detected objects are vehicles/people (not infrastructure)
4. Export snapshot for offline analysis

## Web UI

The LiDAR status page (`http://localhost:8082/`) includes:

- **PCAP Start Form** with "Analysis Mode" checkbox
- **Resume Live** link (appears when in analysis mode)
- **Stop PCAP** link (resets grid)
- **Grid Status** shows current mode and statistics

## Logging

Analysis mode produces distinct log messages:

```
# Starting in analysis mode
[DataSource] switched to PCAP analysis mode for sensor=hesai-pandar40p file=break-80k.pcapng

# PCAP completion (grid preserved)
[DataSource] PCAP analysis complete for sensor=hesai-pandar40p, grid preserved for inspection

# Resuming live with preserved grid
[DataSource] resumed Live from PCAP analysis for sensor=hesai-pandar40p (grid preserved)
```

Normal mode logs:

```
# Starting normal replay
[DataSource] switched to PCAP replay mode for sensor=hesai-pandar40p file=break-80k.pcapng

# Completion with auto-reset
[ResetGrid] sensor=hesai-pandar40p nonzero_before=45442 nonzero_after=0 ...
[DataSource] auto-switched to Live after PCAP for sensor=hesai-pandar40p
```

## Technical details

### Grid preservation

In analysis mode:

- Background grid statistics remain intact after PCAP completion
- Data source switches to `pcap_analysis` state
- Grid persists through manual live data resume
- Grid survives background flush cycles (persisted to database)

### State transitions

```
Live → PCAP (analysis_mode=true) → PCAP Analysis → Live (grid preserved)
  ↓                                                      ↓
  └────────────────── Grid Reset ──────────────────────┘
```

Normal mode:

```
Live → PCAP (analysis_mode=false) → [auto-reset] → Live
```

### Performance notes

- PCAP replay runs as fast as CPU allows (not real-time throttled)
- Example: 80K packets (28.7M points) processes in ~13 seconds
- Grid preservation has no performance impact
- Resuming live from analysis mode is instantaneous

## Limitations

- Only one PCAP can be in progress at a time
- Analysis mode requires manual resume or stop
- Grid reset is irreversible (must replay PCAP to rebuild)
- PCAP files must be in configured safe directory

## See also

- [LiDAR Architecture](../architecture/LIDAR_ARCHITECTURE.md) - Background subtraction (L3) and grid management
- [Data Source Switching](data-source-switching.md) - PCAP replay implementation
- [Foreground Tracking Status](../architecture/foreground-tracking.md#current-operational-status): Current issues and debugging
- [Settling time optimisation](settling-time-optimisation.md) - Settling convergence tuning
- [Adaptive region parameters](adaptive-region-parameters.md) - Region classification after settling
- [Motion capture](motion-capture.md) - Sensor movement detection in L3

---

## PCAP split tool

Automatically segments LiDAR PCAP files into non-overlapping motion and static periods. Enables separate analysis pipelines for mobile observation (driving) and parked data collection.

**Status:** Implemented. Run it as `velocity lidar pcap-split --pcap capture.pcapng --output ./segments` (or the standalone `cmd/tools/pcap-split` wrapper, which is a thin shim over the same engine). `pcap-split` is the single offline tool for **scan, motion stats, and splits**: its summary reports capture health (duration, frame rate from motor RPM, RPM range, points/frame, foreground %) alongside the motion/static segments. To preview the stats and timeline without writing any PCAP files, add `--dry-run`; `--stats-10s` appends per-10-second frame-rate buckets and `--motion-json timeline.json` writes the motion/static timeline to a file. Both offline tools (`pcap-split`, `settling-eval`) take `--config <tuning.json>` and load it via the same disk-or-embedded fallback as the live server, so offline analysis runs the **same algorithms and tuning as live observation** (the background model, motion thresholds, and sensor id all come from the tuning config; `settling-eval`'s old `--tuning` is a deprecated alias). When `--port` is omitted the sensor's UDP port is auto-detected from the capture. The detailed breakdown lists each segment by offset seconds by default; pass `--timeline-units frames` or `--timeline-units timestamp` for frame indices or absolute capture time (these diverge when a capture has recording gaps). Long reads print progress lines to stderr every 20 s by default; the scan pass reports percentage, packets, points, RPM, points-per-frame, elapsed time, and rate, while the write pass reports only percentage, packets, elapsed time, and rate. Tune or silence both with `--progress N` (0 disables). Pipeline performance regression testing lives in a separate dev/CI tool, `lidar-bench` (see [performance-regression-testing.md](performance-regression-testing.md)).

### Problem

Long PCAP captures from mobile observation sessions contain mixed driving and parked data. The background model only functions during static periods: motion segments are unusable for perception. Today an operator must manually identify transition points and split files with external tools. This is slow, error-prone, and blocks the mobile-observation workflow.

### Split tool architecture

```
┌──────────────────────────────────────────────────┐
│              pcap-split CLI                      │
│           (cmd/tools/pcap-split)                 │
└──────────────────────────────────────────────────┘
         │                │                │
         ▼                ▼                ▼
  ┌────────────┐  ┌──────────────┐  ┌────────────┐
  │ PCAP Reader│  │  Settling    │  │ PCAP Writer│
  │ (l1packets)│  │  Analyser    │  │ (pcapsplit)│
  │            │  │ BackgroundMgr│  │            │
  │ Parse UDP  │  │ Track metrics│  │ Buffer pkts│
  │ Extract pts│  │ Detect state │  │ Write segs │
  └────────────┘  └──────────────┘  └────────────┘
```

**Key packages:**

| Package           | Location                                   | Role                                                            |
| ----------------- | ------------------------------------------ | --------------------------------------------------------------- |
| PCAP reader       | `internal/lidar/l1packets/network/pcap.go` | Reads PCAP, filters UDP, parses packets                         |
| Settling analyser | `internal/lidar/pcapsplit/analyse.go`      | Pass 1: classifies each frame motion/static via BackgroundMgr   |
| Segment writer    | `internal/lidar/pcapsplit/writer.go`       | Pass 2: copies packets into per-segment PCAPs by timestamp      |
| Orchestration     | `internal/lidar/pcapsplit/run.go`          | Two-pass `Run`; shared by the applet and the standalone wrapper |
| CLI               | `internal/cmd/lidar/split.go`              | Flag parsing → `pcapsplit.Run`; `cmd/tools/pcap-split` wraps it |

### Progress reporting

`pcap-split` runs two passes, and each reports progress to stderr paced by
`--progress` (default 20 s; 0 disables): a `[scan]` line during pass-1
classification (with RPM and points-per-frame, since it decodes every frame) and
a `[write]` line during pass-2 segment writing that shows only percentage,
packet count, elapsed time, and packet rate — the writer copies packets without
decoding per-frame points or motor speed. A `--dry-run` performs
only the scan pass.

### Stability detection

Motion is classified per frame by **sensor ego-motion**, from either of two
signals:

- **Foreground fraction ≥ 0.20** (`SensorMovementForegroundThreshold`) catches
  the _onset_ of motion: when the platform first moves, the scene shifts and
  foreground spikes.

- **Background-drift ratio ≥ 0.35** (`SensorMovementDriftRatioThreshold`) catches
  _sustained_ motion. The drift ratio is the fraction of settled cells whose
  range has shifted past `background_drift_threshold_metres` (0.5 m) from its
  locked baseline. Driving shifts most of the grid at once, so the ratio climbs
  to 0.4–1.0 and stays there; a parked sensor only shifts the few cells that
  passing traffic crosses, so it stays near 0.1. Foreground alone goes blind to
  long drives once the per-cell spread saturates and the gate widens; the drift
  ratio does not.

Drift ratio is the discriminator because it keys on **ego-motion** (most of the
grid moving at once), not **scene activity**. The earlier sustained-motion
signal — mean per-cell range spread over the noise floor — conflated the two: a
busy parked scene inflates per-cell spread exactly as driving does, so a sensor
parked in heavy traffic was mislabelled as moving for its entire stay. Drift
ratio separates them cleanly: on real captures a busy parked scene stays at
≤ ~0.23 while driving sits at ≥ ~0.43, so the 0.35 threshold has margin on both
sides.

A parked sensor stays below both thresholds — even while its background model is
still settling at the start of a capture, and even when heavy traffic crosses an
otherwise static scene. Settled-cell % is exported as diagnostic evidence but
does not gate the decision. It uses the shared `locked_baseline_threshold`.

The classifier advances warmup and frozen-cell state from PCAP timestamps, not
wall-clock replay time. Replay mode exposes foreground during warmup but does
not change any L3 tuning value; consequently `pcap-split` uses the same model
parameters as live observation and remains deterministic when replay speed
changes.

**State machine:**

- **Motion → Static:** 60 s sustained stability (configurable via `--settling-sec`)
- **Static → Motion:** 5 s sustained motion
- **Intersection bridging:** pauses < 30 s stay classified as motion (`--max-motion-gap-sec`)

### Split tool CLI

```
pcap-split [options]

Options:
  --pcap FILE             Input PCAP file (required)
  --output DIR            Output directory (default: current dir)
  --prefix NAME           Output filename prefix (default: input file stem)
  --settling-sec N        Settling duration threshold (default: 60)
  --min-segment-sec N     Minimum segment duration (default: 5)
  --max-motion-gap-sec N  Maximum motion gap to bridge (default: 30)
  --export-metrics        Export per-frame metrics CSV
  --export-json           Export segment metadata JSON
  --progress N            Seconds between progress updates on stderr (default: 20; 0 = off)
```

Example:

```bash
pcap-split --pcap capture.pcap --output ./segments --export-json
```

Output:

```
segments/
├── capture-motion-0.pcap
├── capture-static-0.pcap
├── capture-motion-1.pcap
├── capture-static-1.pcap
├── capture-motion-2.pcap
├── segments.json
└── capture-summary.txt
```

### Motion classifier API

The shared L3 background manager provides these read-only motion inputs:

| Method                                      | Purpose                                              |
| ------------------------------------------- | ---------------------------------------------------- |
| `GetFrameSettlingMetrics(settledThreshold)` | Per-frame settled/nonzero/frozen cell counts         |
| `CheckBackgroundDrift()`                    | Drift metrics; the ratio is the sustained-motion cue |
| `EvaluateSensorMotion(mask)`                | Shared foreground/drift-ratio motion decision        |
