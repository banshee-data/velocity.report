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

## PCAP split tool (planned)

Active plan: [pcap-split-tool-plan.md](../../plans/pcap-split-tool-plan.md)

Automatically segments LiDAR PCAP files into non-overlapping motion and static periods. Enables separate analysis pipelines for mobile observation (driving) and parked data collection.

**Status:** Not yet implemented. Design complete.

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

| Package           | Location                               | Role                                                     |
| ----------------- | -------------------------------------- | -------------------------------------------------------- |
| PCAP reader       | `internal/lidar/network/pcap.go`       | Existing: reads PCAP, filters UDP, parses packets        |
| Settling analyser | `internal/lidar/pcapsplit/analyser.go` | **New**: implements `FrameBuilder`, drives state machine |
| Segment writer    | `internal/lidar/pcapsplit/writer.go`   | **New**: buffers packets, writes segment PCAPs           |
| CLI               | `cmd/tools/pcap-split/main.go`         | **New**: flag parsing, orchestration, summary output     |

### Stability detection

All four criteria must hold to classify a frame as stable:

1. Foreground activity < 5% of total points
2. Settled cells > 70% (`TimesSeenCount` >= threshold)
3. Noise deviation < 2.0 sigma
4. Within expected variance bounds

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
  --prefix NAME           Output filename prefix (default: "out")
  --settling-sec N        Settling duration threshold (default: 60)
  --min-segment-sec N     Minimum segment duration (default: 5)
  --max-motion-gap-sec N  Maximum motion gap to bridge (default: 30)
  --export-metrics        Export per-frame metrics CSV
  --export-json           Export segment metadata JSON
```

Example:

```bash
pcap-split --pcap capture.pcap --output ./segments --export-json
```

Output:

```
segments/
├── out-motion-0.pcap
├── out-static-0.pcap
├── out-motion-1.pcap
├── out-static-1.pcap
├── out-motion-2.pcap
├── segments.json
└── summary.txt
```

### Required API extensions

Three new read-only accessors on `BackgroundManager` (designed, not yet implemented):

| Method                                      | Purpose                                          |
| ------------------------------------------- | ------------------------------------------------ |
| `GetFrameSettlingMetrics(settledThreshold)` | Per-frame settled/nonzero/frozen cell counts     |
| `GetNoiseBoundsDeviation()`                 | Aggregate deviation from expected noise envelope |
| `IsWithinNoiseBounds(threshold)`            | Boolean check for noise envelope compliance      |

### Phased delivery

| Phase | Scope                                                                | Size | Prerequisite      |
| ----- | -------------------------------------------------------------------- | ---- | ----------------- |
| 1     | `--motion` flag in `pcap-analyse`: motion timeline in summary output | S    | None              |
| 2     | `BackgroundManager` API extensions: three new read-only accessors    | S    | Phase 1 validated |
| 3     | Full `pcap-split` tool: analyser, writer, CLI, metadata export       | M    | Phase 2           |
