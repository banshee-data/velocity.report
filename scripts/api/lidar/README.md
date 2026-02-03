# Local API helper scripts

Small shell helpers for exercising the local monitor API used during PCAP replay debugging.

## Direct script usage

```bash
# Grid operations
./get_grid_status.sh [sensor_id]
./reset_grid.sh [sensor_id]
./get_grid_heatmap.sh [sensor_id] [azimuth_bucket_deg] [settled_threshold]

# Snapshot operations
./get_snapshot.sh [sensor_id]          # Get latest snapshot
./get_snapshots.sh [sensor_id]         # Get recent snapshots list

# Acceptance metrics
./get_acceptance.sh [sensor_id]
./reset_acceptance.sh [sensor_id]

# Parameter management
./get_params.sh [sensor_id]
./set_params.sh <sensor_id> <json_params>
# Example: ./set_params.sh hesai-pandar40p '{"noise_relative": 0.15, "closeness_multiplier": 2.5}'

# Persistence and export
./trigger_persist.sh [sensor_id]
./export_snapshot.sh [sensor_id] [snapshot_id] [output_path]
./export_next_frame.sh [sensor_id] [output_path]

# Data source switching
./switch_data_source.sh live [sensor_id] [base_url]
./switch_data_source.sh pcap /path/to/file.pcap [sensor_id] [base_url]
# Convenience aliases
./start_pcap.sh /path/to/file.pcap [sensor_id] [base_url]
./stop_pcap.sh [sensor_id] [base_url]
./get_status.sh [base_url]
```

## Make targets

Convenient make targets are available from the project root:

```bash
# Grid operations
make api-grid-status [SENSOR=hesai-pandar40p]
make api-grid-reset [SENSOR=hesai-pandar40p]
make api-grid-heatmap [SENSOR=hesai-pandar40p] [AZIMUTH=3.0] [THRESHOLD=5]

# Snapshot operations
make api-snapshot [SENSOR=hesai-pandar40p]      # Get latest snapshot
make api-snapshots [SENSOR=hesai-pandar40p]     # Get recent snapshots list

# Acceptance metrics
make api-acceptance [SENSOR=hesai-pandar40p]
make api-acceptance-reset [SENSOR=hesai-pandar40p]

# Parameter management
make api-params [SENSOR=hesai-pandar40p]
make api-params-set SENSOR=hesai-pandar40p PARAMS='{"noise_relative": 0.15}'

# Persistence and export
make api-persist [SENSOR=hesai-pandar40p]
make api-export-snapshot [SENSOR=hesai-pandar40p] [SNAPSHOT_ID=123] [OUT=/path/to/output.asc]
make api-export-next-frame [SENSOR=hesai-pandar40p] [OUT=/path/to/output.asc]

# Data source switching
make api-switch-data-source SOURCE=live [SENSOR=hesai-pandar40p] [BASE_URL=http://127.0.0.1:8081]
make api-switch-data-source SOURCE=pcap PCAP=/path/to/file.pcap [SENSOR=hesai-pandar40p] [BASE_URL=http://127.0.0.1:8081]
make api-start-pcap PCAP=/path/to/file.pcap [SENSOR=hesai-pandar40p] [BASE_URL=http://127.0.0.1:8081]
make api-stop-pcap [SENSOR=hesai-pandar40p] [BASE_URL=http://127.0.0.1:8081]
make api-status [BASE_URL=http://127.0.0.1:8081]
```

## API Endpoints Reference

All scripts connect to `http://127.0.0.1:8081` and require `jq` for pretty JSON output.

| Endpoint                       | Script                  | Make Target              | Description                               |
| ------------------------------ | ----------------------- | ------------------------ | ----------------------------------------- |
| `/api/lidar/grid_status`       | `get_grid_status.sh`    | `api-grid-status`        | Get grid cell statistics                  |
| `/api/lidar/grid_reset`        | `reset_grid.sh`         | `api-grid-reset`         | Reset grid to zero state                  |
| `/api/lidar/grid_heatmap`      | `get_grid_heatmap.sh`   | `api-grid-heatmap`       | Get aggregated heatmap data               |
| `/api/lidar/snapshot`          | `get_snapshot.sh`       | `api-snapshot`           | Get latest snapshot details               |
| `/api/lidar/snapshots`         | `get_snapshots.sh`      | `api-snapshots`          | List recent snapshots                     |
| `/api/lidar/acceptance`        | `get_acceptance.sh`     | `api-acceptance`         | Get acceptance/rejection metrics          |
| `/api/lidar/acceptance/reset`  | `reset_acceptance.sh`   | `api-acceptance-reset`   | Reset acceptance counters                 |
| `/api/lidar/params`            | `get_params.sh`         | `api-params`             | Get background parameters                 |
| `/api/lidar/params`            | `set_params.sh`         | `api-params-set`         | Update background parameters              |
| `/api/lidar/persist`           | `trigger_persist.sh`    | `api-persist`            | Trigger manual snapshot                   |
| `/api/lidar/export_snapshot`   | `export_snapshot.sh`    | `api-export-snapshot`    | Export snapshot to ASC                    |
| `/api/lidar/export_next_frame` | `export_next_frame.sh`  | `api-export-next-frame`  | Export next frame to ASC                  |
| `/api/lidar/status`            | `get_status.sh`         | `api-status`             | Get current data source + monitor stats   |
| `/api/lidar/pcap/start`        | `start_pcap.sh`         | `api-start-pcap`         | Start PCAP replay (switch to PCAP source) |
| `/api/lidar/pcap/stop`         | `stop_pcap.sh`          | `api-stop-pcap`          | Stop PCAP replay (return to live source)  |
| `/api/lidar/data_source`       | `switch_data_source.sh` | `api-switch-data-source` | Convenience wrapper for start/stop        |
