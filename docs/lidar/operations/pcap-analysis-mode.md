# PCAP Analysis Mode

## Overview

The LiDAR system supports two modes for PCAP replay:

1. **Normal PCAP Replay** - Replays PCAP file, then automatically resets grid and returns to live data
2. **PCAP Analysis Mode** - Replays PCAP file and **preserves the background grid** for inspection and analysis

## Use Cases

### Normal Replay Mode

- Testing sensor configurations
- Debugging packet parsing issues
- Verifying frame assembly
- Quick validation before returning to live monitoring

### Analysis Mode

- **Track Analysis** - Identify and study vehicle trajectories from historical data
- **Background Characterization** - Build accurate background models from known-quiet periods
- **Object Detection Tuning** - Analyze detection thresholds with real-world data
- **Scene Comparison** - Compare PCAP-analyzed background with live data overlay
- **Historical Investigation** - Study specific incidents or traffic patterns

## API Endpoints

### Start PCAP Replay (Analysis Mode)

```bash
POST /api/lidar/pcap/start?sensor_id=hesai-pandar40p
Content-Type: application/json

{
  "pcap_file": "break-80k.pcapng",
  "analysis_mode": true
}
```

**Response:**

```json
{
  "status": "started",
  "sensor_id": "hesai-pandar40p",
  "current_source": "pcap",
  "pcap_file": "/path/to/break-80k.pcapng",
  "analysis_mode": true
}
```

### Check Data Source Status

```bash
GET /api/lidar/data_source?sensor_id=hesai-pandar40p
```

**Response:**

```json
{
  "status": "ok",
  "data_source": "pcap_analysis",
  "pcap_file": "/path/to/break-80k.pcapng",
  "pcap_in_progress": false,
  "analysis_mode": true
}
```

Data source values:

- `live` - Normal live UDP data collection
- `pcap` - PCAP replay in progress
- `pcap_analysis` - PCAP replay completed in analysis mode (grid preserved)

### Resume Live Data (Preserve Grid)

```bash
POST /api/lidar/pcap/resume_live?sensor_id=hesai-pandar40p
```

Switches from PCAP analysis mode back to live UDP data **without resetting the grid**. This allows you to overlay live traffic on top of the PCAP-analyzed background.

**Response:**

```json
{
  "status": "resumed_live",
  "sensor_id": "hesai-pandar40p",
  "current_source": "live",
  "grid_preserved": true
}
```

### Stop PCAP Replay (Reset Grid)

```bash
POST /api/lidar/pcap/stop?sensor_id=hesai-pandar40p
```

Stops PCAP replay and **resets the grid** before returning to live data.

## Workflow Examples

### Basic Analysis Workflow

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

### Comparative Analysis

1. Load PCAP in analysis mode to build background model
2. Resume live data (grid preserved)
3. Live objects will be detected against PCAP-built background
4. Useful for comparing "then vs now" traffic patterns

### Scene Reconstruction

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

## Technical Details

### Grid Preservation

In analysis mode:

- Background grid statistics remain intact after PCAP completion
- Data source switches to `pcap_analysis` state
- Grid persists through manual live data resume
- Grid survives background flush cycles (persisted to database)

### State Transitions

```
Live → PCAP (analysis_mode=true) → PCAP Analysis → Live (grid preserved)
  ↓                                                      ↓
  └────────────────── Grid Reset ──────────────────────┘
```

Normal mode:

```
Live → PCAP (analysis_mode=false) → [auto-reset] → Live
```

### Performance Notes

- PCAP replay runs as fast as CPU allows (not real-time throttled)
- Example: 80K packets (28.7M points) processes in ~13 seconds
- Grid preservation has no performance impact
- Resuming live from analysis mode is instantaneous

## Limitations

- Only one PCAP can be in progress at a time
- Analysis mode requires manual resume or stop
- Grid reset is irreversible (must replay PCAP to rebuild)
- PCAP files must be in configured safe directory

## See Also

- [LIDAR Sidecar Overview](../architecture/lidar_sidecar_overview.md) - Background subtraction and grid management
- [Data Source Switching](DATA-SOURCE-SWITCHING-PLAN.md) - PCAP replay implementation
- [Foreground Tracking Status](lidar-foreground-tracking-status.md) - Current issues and debugging
