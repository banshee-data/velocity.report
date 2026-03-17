# Backend Surface Matrix

Complete mapping of components, API endpoints, database tables, data
structures, and pipeline stages across the velocity.report codebase. Shows
which surfaces consume each item: **DB** (SQLite persistence), **Web**
(Svelte UI on `:8080`), **PDF** (Python LaTeX generator), **Mac** (Metal
visualiser via gRPC).

- **Source:** Full-codebase audit (March 2026)
- **Inventory script:** `scripts/list-matrix-fields.py` — `--checklist` generates the LLM-consumable tracing checklist
- **Update workflow:** `@matrix-tracer` agent (`.github/agents/matrix-tracer.agent.md`)
- **Related:** [Remediation plan](../../docs/plans/unpopulated-data-structures-remediation-plan.md) · [Clustering observability](../../docs/plans/lidar-clustering-observability-and-benchmark-plan.md) · [HINT metric observability](../../docs/plans/hint-metric-observability-plan.md)

---

## Legend

| Symbol | Meaning                               |
| ------ | ------------------------------------- |
| ✅     | Implemented and wired to this surface |
| 📋     | Planned — not yet implemented         |
| 🔶     | Partially wired (see notes)           |
| 🗑️     | Deprecated — to be removed            |
| —      | Not applicable to this surface        |

---

## 1. HTTP API Endpoints — Radar / Main Server

**Source:** `cmd/radar/radar.go`, `internal/api/server.go`

| Folder         | File        | Endpoint                             | DB  | Web | PDF | Mac |
| -------------- | ----------- | ------------------------------------ | --- | --- | --- | --- |
| `internal/api` | `server.go` | `GET /events`                        | ✅  | ✅  | —   | —   |
| `internal/api` | `server.go` | `POST /command`                      | ✅  | ✅  | —   | —   |
| `internal/api` | `server.go` | `GET /api/config`                    | —   | ✅  | —   | —   |
| `internal/api` | `server.go` | `GET /api/db_stats`                  | ✅  | ✅  | —   | —   |
| `internal/api` | `server.go` | `GET /api/radar_stats`               | ✅  | ✅  | ✅  | —   |
| `internal/api` | `server.go` | `POST /api/generate_report`          | ✅  | ✅  | ✅  | —   |
| `internal/api` | `server.go` | `GET/POST /api/sites`                | ✅  | ✅  | ✅  | —   |
| `internal/api` | `server.go` | `GET/PUT/DEL /api/sites/{id}`        | ✅  | ✅  | ✅  | —   |
| `internal/api` | `server.go` | `GET/POST /api/site_config_periods`  | ✅  | ✅  | ✅  | —   |
| `internal/api` | `server.go` | `GET /api/timeline`                  | ✅  | ✅  | —   | —   |
| `internal/api` | `server.go` | `GET/POST/DEL /api/reports/`         | ✅  | ✅  | —   | —   |
| `internal/api` | `server.go` | `GET /api/reports/site/{siteId}`     | ✅  | ✅  | —   | —   |
| `internal/api` | `server.go` | `GET /api/reports/{id}/download/{f}` | ✅  | ✅  | —   | —   |
| `internal/api` | `server.go` | `GET/POST /api/transit_worker`       | ✅  | ✅  | —   | —   |

---

## 2. HTTP API Endpoints — LiDAR Monitor

**Source:** `internal/lidar/monitor/webserver.go`, `track_api.go`, `run_track_api.go`, `internal/api/lidar_labels.go`\
**Mac consumers:** `RunTrackLabelAPIClient.swift`, `LabelAPIClient.swift` (HTTP, not gRPC)

| Layer          | File               | Endpoint                                        | DB  | Web | PDF | Mac |
| -------------- | ------------------ | ----------------------------------------------- | --- | --- | --- | --- |
| Status         | `webserver.go`     | `GET /health`                                   | —   | ✅  | —   | —   |
| Status         | `webserver.go`     | `GET /api/lidar/monitor`                        | —   | ✅  | —   | —   |
| Status         | `webserver.go`     | `GET /api/lidar/status`                         | —   | ✅  | —   | —   |
| Status         | `webserver.go`     | `POST /api/lidar/persist`                       | ✅  | ✅  | —   | —   |
| Snapshot       | `webserver.go`     | `GET /api/lidar/snapshot`                       | ✅  | ✅  | —   | —   |
| Snapshot       | `webserver.go`     | `GET /api/lidar/snapshots`                      | ✅  | ✅  | —   | —   |
| Snapshot       | `webserver.go`     | `POST /api/lidar/snapshots/cleanup`             | ✅  | ✅  | —   | —   |
| Export         | `webserver.go`     | `GET /api/lidar/export_snapshot`                | ✅  | ✅  | —   | —   |
| Export         | `webserver.go`     | `GET /api/lidar/export_next_frame`              | —   | ✅  | —   | —   |
| Export         | `webserver.go`     | `GET /api/lidar/export_frame_sequence`          | —   | ✅  | —   | —   |
| Export         | `webserver.go`     | `GET /api/lidar/export_foreground`              | —   | ✅  | —   | —   |
| Traffic        | `webserver.go`     | `GET /api/lidar/traffic`                        | —   | ✅  | —   | —   |
| Traffic        | `webserver.go`     | `GET /api/lidar/acceptance`                     | —   | ✅  | —   | —   |
| Traffic        | `webserver.go`     | `POST /api/lidar/acceptance/reset`              | —   | ✅  | —   | —   |
| Tuning         | `webserver.go`     | `GET/POST /api/lidar/params`                    | ✅  | ✅  | —   | —   |
| Sweep          | `webserver.go`     | `POST /api/lidar/sweep/start`                   | ✅  | ✅  | —   | —   |
| Sweep          | `webserver.go`     | `GET /api/lidar/sweep/status`                   | ✅  | ✅  | —   | —   |
| Sweep          | `webserver.go`     | `POST /api/lidar/sweep/stop`                    | ✅  | ✅  | —   | —   |
| Sweep          | `webserver.go`     | `GET /api/lidar/sweep/explain/`                 | —   | ✅  | —   | —   |
| Auto-tune      | `webserver.go`     | `GET/POST /api/lidar/sweep/auto`                | —   | ✅  | —   | —   |
| Auto-tune      | `webserver.go`     | `POST /api/lidar/sweep/auto/stop`               | —   | ✅  | —   | —   |
| Auto-tune      | `webserver.go`     | `POST /api/lidar/sweep/auto/suspend`            | —   | ✅  | —   | —   |
| Auto-tune      | `webserver.go`     | `POST /api/lidar/sweep/auto/resume`             | —   | ✅  | —   | —   |
| Auto-tune      | `webserver.go`     | `GET /api/lidar/sweep/auto/suspended`           | —   | ✅  | —   | —   |
| HINT           | `webserver.go`     | `POST /api/lidar/sweep/hint/continue`           | ✅  | ✅  | —   | —   |
| HINT           | `webserver.go`     | `POST /api/lidar/sweep/hint/stop`               | —   | ✅  | —   | —   |
| HINT           | `webserver.go`     | `GET /api/lidar/sweep/hint`                     | ✅  | ✅  | —   | —   |
| Background     | `webserver.go`     | `GET /api/lidar/grid_status`                    | —   | ✅  | —   | —   |
| Background     | `webserver.go`     | `GET /api/lidar/settling_eval`                  | —   | ✅  | —   | —   |
| Background     | `webserver.go`     | `POST /api/lidar/grid_reset`                    | ✅  | ✅  | —   | —   |
| Background     | `webserver.go`     | `GET /api/lidar/grid_heatmap`                   | —   | ✅  | —   | —   |
| Background     | `webserver.go`     | `GET /api/lidar/background/grid`                | ✅  | ✅  | —   | —   |
| PCAP           | `webserver.go`     | `GET /api/lidar/data_source`                    | —   | ✅  | —   | —   |
| PCAP           | `webserver.go`     | `POST /api/lidar/pcap/start`                    | —   | ✅  | —   | —   |
| PCAP           | `webserver.go`     | `POST /api/lidar/pcap/stop`                     | —   | ✅  | —   | —   |
| PCAP           | `webserver.go`     | `POST /api/lidar/pcap/resume_live`              | —   | ✅  | —   | —   |
| PCAP           | `webserver.go`     | `GET /api/lidar/pcap/files`                     | —   | ✅  | —   | —   |
| Playback       | `webserver.go`     | `GET /api/lidar/playback/status`                | —   | ✅  | —   | ✅  |
| Playback       | `webserver.go`     | `POST /api/lidar/playback/pause`                | —   | ✅  | —   | —   |
| Playback       | `webserver.go`     | `POST /api/lidar/playback/play`                 | —   | ✅  | —   | —   |
| Playback       | `webserver.go`     | `POST /api/lidar/playback/seek`                 | —   | ✅  | —   | —   |
| Playback       | `webserver.go`     | `POST /api/lidar/playback/rate`                 | —   | ✅  | —   | —   |
| Playback       | `webserver.go`     | `POST /api/lidar/vrlog/load`                    | —   | ✅  | —   | ✅  |
| Playback       | `webserver.go`     | `POST /api/lidar/vrlog/stop`                    | —   | ✅  | —   | ✅  |
| Charts         | `webserver.go`     | `GET /api/lidar/chart/polar`                    | —   | ✅  | —   | —   |
| Charts         | `webserver.go`     | `GET /api/lidar/chart/heatmap`                  | —   | ✅  | —   | —   |
| Charts         | `webserver.go`     | `GET /api/lidar/chart/foreground`               | —   | ✅  | —   | —   |
| Charts         | `webserver.go`     | `GET /api/lidar/chart/clusters`                 | —   | ✅  | —   | —   |
| Charts         | `webserver.go`     | `GET /api/lidar/chart/traffic`                  | —   | ✅  | —   | —   |
| Tracks         | `track_api.go`     | `GET /api/lidar/tracks`                         | ✅  | ✅  | —   | —   |
| Tracks         | `track_api.go`     | `GET /api/lidar/tracks/active`                  | ✅  | ✅  | —   | —   |
| Tracks         | `track_api.go`     | `GET /api/lidar/tracks/{id}`                    | ✅  | ✅  | —   | —   |
| Tracks         | `track_api.go`     | `GET /api/lidar/tracks/{id}/observations`       | ✅  | ✅  | —   | —   |
| Tracks         | `track_api.go`     | `GET /api/lidar/tracks/history`                 | ✅  | ✅  | —   | —   |
| Tracks         | `track_api.go`     | `GET /api/lidar/tracks/summary`                 | ✅  | ✅  | —   | —   |
| Tracks         | `track_api.go`     | `GET /api/lidar/tracks/metrics`                 | —   | ✅  | —   | —   |
| Clusters       | `track_api.go`     | `GET /api/lidar/clusters`                       | ✅  | ✅  | —   | —   |
| Observations   | `track_api.go`     | `GET /api/lidar/observations`                   | ✅  | ✅  | —   | —   |
| Runs           | `run_track_api.go` | `GET/POST/DEL /api/lidar/runs/`                 | ✅  | ✅  | —   | ✅  |
| Runs           | `run_track_api.go` | `GET /api/lidar/runs/{id}/tracks`               | ✅  | ✅  | —   | ✅  |
| Runs           | `run_track_api.go` | `GET/DEL /api/lidar/runs/{id}/tracks/{tid}`     | ✅  | ✅  | —   | ✅  |
| Runs           | `run_track_api.go` | `PUT /api/lidar/runs/{id}/tracks/{tid}/label`   | ✅  | ✅  | —   | ✅  |
| Runs           | `run_track_api.go` | `PUT /api/lidar/runs/{id}/tracks/{tid}/flags`   | ✅  | ✅  | —   | —   |
| Runs           | `run_track_api.go` | `GET /api/lidar/runs/{id}/compare/{other}`      | 📋  | 📋  | —   | —   |
| Runs           | `run_track_api.go` | `GET /api/lidar/runs/{id}/labelling-progress`   | ✅  | ✅  | —   | ✅  |
| Labels         | `lidar_labels.go`  | `GET/POST /api/lidar/labels`                    | ✅  | ✅  | —   | ✅  |
| Labels         | `lidar_labels.go`  | `GET/PUT/DEL /api/lidar/labels/{id}`            | ✅  | ✅  | —   | ✅  |
| Labels         | `lidar_labels.go`  | `GET /api/lidar/labels/export`                  | ✅  | ✅  | —   | ✅  |
| Scenes         | `webserver.go`     | `GET/POST /api/lidar/scenes`                    | ✅  | ✅  | —   | —   |
| Scenes         | `webserver.go`     | `GET/PUT/DEL /api/lidar/scenes/{id}`            | ✅  | ✅  | —   | —   |
| Missed regions | `webserver.go`     | `GET/POST /api/lidar/runs/{id}/missed-regions`  | ✅  | ✅  | —   | —   |
| Missed regions | `webserver.go`     | `DEL /api/lidar/runs/{id}/missed-regions/{rid}` | ✅  | ✅  | —   | —   |
| Sweep history  | `webserver.go`     | `GET /api/lidar/sweeps`                         | ✅  | ✅  | —   | —   |
| Sweep history  | `webserver.go`     | `GET /api/lidar/sweeps/{id}`                    | ✅  | ✅  | —   | —   |
| Sweep history  | `webserver.go`     | `PUT /api/lidar/sweeps/charts`                  | ✅  | ✅  | —   | —   |
| Destructive    | `webserver.go`     | `POST /api/lidar/tracks/clear`                  | ✅  | ✅  | —   | —   |
| Destructive    | `webserver.go`     | `POST /api/lidar/runs/clear`                    | ✅  | ✅  | —   | —   |

---

## 3. gRPC Service — macOS Visualiser

**Source:** `proto/velocity_visualiser/v1/visualiser.proto`

| Layer     | File               | Method                            | DB  | Web | PDF | Mac |
| --------- | ------------------ | --------------------------------- | --- | --- | --- | --- |
| Streaming | `visualiser.proto` | `StreamFrames` (server streaming) | —   | —   | —   | ✅  |
| Playback  | `visualiser.proto` | `Pause`                           | —   | —   | —   | ✅  |
| Playback  | `visualiser.proto` | `Play`                            | —   | —   | —   | ✅  |
| Playback  | `visualiser.proto` | `Seek`                            | —   | —   | —   | ✅  |
| Playback  | `visualiser.proto` | `SetRate`                         | —   | —   | —   | ✅  |
| Debug     | `visualiser.proto` | `SetOverlayModes`                 | —   | —   | —   | ✅  |
| Debug     | `visualiser.proto` | `GetCapabilities`                 | —   | —   | —   | ✅  |
| Recording | `visualiser.proto` | `StartRecording`                  | —   | —   | —   | ✅  |
| Recording | `visualiser.proto` | `StopRecording`                   | —   | —   | —   | ✅  |

---

## 4. Database Tables

**Source:** `internal/db/schema.sql`

| Layer  | Table                  | Web | PDF | Mac | Notes                                      |
| ------ | ---------------------- | --- | --- | --- | ------------------------------------------ |
| LiDAR  | `lidar_analysis_runs`  | ✅  | —   | —   | Run browser + label UI                     |
| LiDAR  | `lidar_clusters`       | ✅  | —   | ✅  | Cluster display, gRPC                      |
| LiDAR  | `lidar_tracks`         | ✅  | —   | ✅  | Track display, gRPC                        |
| LiDAR  | `lidar_track_obs`      | ✅  | —   | —   | Trajectory rendering                       |
| LiDAR  | `lidar_labels`         | ✅  | —   | —   | Training + HINT                            |
| LiDAR  | `lidar_scenes`         | ✅  | —   | —   | HINT labelling                             |
| LiDAR  | `lidar_evaluations`    | ✅  | —   | —   | Sweep/auto-tune                            |
| LiDAR  | `lidar_sweeps`         | ✅  | —   | —   | Sweep history                              |
| LiDAR  | `lidar_bg_snapshot`    | ✅  | —   | 🔶  | Grid visualisation (derived sent via gRPC) |
| LiDAR  | `lidar_bg_regions`     | ✅  | —   | —   | Settling evaluation                        |
| LiDAR  | `lidar_missed_regions` | ✅  | —   | —   | Detection gap annotations                  |
| LiDAR  | `lidar_run_tracks`     | ✅  | —   | —   | Per-run track copies                       |
| Radar  | `radar_data`           | ✅  | ✅  | —   | Raw events + alt stats source              |
| Radar  | `radar_objects`        | ✅  | ✅  | —   | Primary report source                      |
| Radar  | `radar_data_transits`  | ✅  | ✅  | —   | Alternative report source                  |
| Radar  | `radar_transit_links`  | ✅  | —   | —   | Transit chain building                     |
| Radar  | `radar_commands`       | ✅  | —   | —   | Debug history                              |
| Radar  | `radar_command_log`    | ✅  | —   | —   | Debug output                               |
| Site   | `site`                 | ✅  | ✅  | —   | Location, metadata                         |
| Site   | `site_config_periods`  | ✅  | ✅  | —   | Mounting angle changes                     |
| Site   | `site_reports`         | ✅  | ✅  | —   | Report metadata + download                 |
| System | `schema_migrations`    | —   | —   | —   | Internal                                   |

---

## 5. Database Fields — All Columns

| Table                  | Column                            | Type          | DB  | Web | PDF | Mac |
| ---------------------- | --------------------------------- | ------------- | --- | --- | --- | --- |
| `lidar_analysis_runs`  | `run_id`                          | TEXT PK       | ✅  | ✅  | —   | —   |
| `lidar_analysis_runs`  | `created_at`                      | INTEGER       | ✅  | ✅  | —   | —   |
| `lidar_analysis_runs`  | `source_type`                     | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_analysis_runs`  | `source_path`                     | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_analysis_runs`  | `sensor_id`                       | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_analysis_runs`  | `params_json`                     | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_analysis_runs`  | `duration_secs`                   | REAL          | ✅  | ✅  | —   | —   |
| `lidar_analysis_runs`  | `total_frames`                    | INTEGER       | ✅  | ✅  | —   | —   |
| `lidar_analysis_runs`  | `total_clusters`                  | INTEGER       | ✅  | ✅  | —   | —   |
| `lidar_analysis_runs`  | `total_tracks`                    | INTEGER       | ✅  | ✅  | —   | —   |
| `lidar_analysis_runs`  | `confirmed_tracks`                | INTEGER       | ✅  | ✅  | —   | —   |
| `lidar_analysis_runs`  | `processing_time_ms`              | INTEGER       | ✅  | ✅  | —   | —   |
| `lidar_analysis_runs`  | `status`                          | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_analysis_runs`  | `error_message`                   | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_analysis_runs`  | `parent_run_id`                   | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_analysis_runs`  | `notes`                           | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_analysis_runs`  | `statistics_json`                 | TEXT          | 🔶  | 📋  | —   | —   |
| `lidar_analysis_runs`  | `vrlog_path`                      | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_clusters`       | `lidar_cluster_id`                | INTEGER PK    | ✅  | ✅  | —   | ✅  |
| `lidar_clusters`       | `sensor_id`                       | TEXT          | ✅  | ✅  | —   | ✅  |
| `lidar_clusters`       | `world_frame`                     | TEXT          | ✅  | ✅  | —   | ✅  |
| `lidar_clusters`       | `ts_unix_nanos`                   | INTEGER       | ✅  | ✅  | —   | ✅  |
| `lidar_clusters`       | `centroid_x`                      | REAL          | ✅  | ✅  | —   | ✅  |
| `lidar_clusters`       | `centroid_y`                      | REAL          | ✅  | ✅  | —   | ✅  |
| `lidar_clusters`       | `centroid_z`                      | REAL          | ✅  | ✅  | —   | ✅  |
| `lidar_clusters`       | `bounding_box_length`             | REAL          | ✅  | ✅  | —   | ✅  |
| `lidar_clusters`       | `bounding_box_width`              | REAL          | ✅  | ✅  | —   | ✅  |
| `lidar_clusters`       | `bounding_box_height`             | REAL          | ✅  | ✅  | —   | ✅  |
| `lidar_clusters`       | `points_count`                    | INTEGER       | ✅  | ✅  | —   | ✅  |
| `lidar_clusters`       | `height_p95`                      | REAL          | ✅  | ✅  | —   | ✅  |
| `lidar_clusters`       | `intensity_mean`                  | REAL          | ✅  | ✅  | —   | ✅  |
| `lidar_clusters`       | `noise_points_count`              | INTEGER       | 🔶  | —   | —   | —   |
| `lidar_clusters`       | `cluster_density`                 | REAL          | 🔶  | 📋  | —   | —   |
| `lidar_clusters`       | `aspect_ratio`                    | REAL          | 🔶  | 📋  | —   | —   |
| `lidar_tracks`         | `track_id`                        | TEXT PK       | ✅  | ✅  | —   | ✅  |
| `lidar_tracks`         | `sensor_id`                       | TEXT          | ✅  | ✅  | —   | ✅  |
| `lidar_tracks`         | `world_frame`                     | TEXT          | ✅  | ✅  | —   | ✅  |
| `lidar_tracks`         | `track_state`                     | TEXT          | ✅  | ✅  | —   | ✅  |
| `lidar_tracks`         | `start_unix_nanos`                | INTEGER       | ✅  | ✅  | —   | ✅  |
| `lidar_tracks`         | `end_unix_nanos`                  | INTEGER       | ✅  | ✅  | —   | ✅  |
| `lidar_tracks`         | `observation_count`               | INTEGER       | ✅  | ✅  | —   | ✅  |
| `lidar_tracks`         | `avg_speed_mps`                   | REAL          | ✅  | ✅  | —   | ✅  |
| `lidar_tracks`         | `peak_speed_mps`                  | REAL          | ✅  | ✅  | —   | ✅  |
| `lidar_tracks`         | `p50_speed_mps`                   | REAL          | 🗑️  | —   | —   | —   |
| `lidar_tracks`         | `p85_speed_mps`                   | REAL          | 🗑️  | —   | —   | —   |
| `lidar_tracks`         | `p95_speed_mps`                   | REAL          | 🗑️  | —   | —   | —   |
| `lidar_tracks`         | `bounding_box_length_avg`         | REAL          | ✅  | ✅  | —   | ✅  |
| `lidar_tracks`         | `bounding_box_width_avg`          | REAL          | ✅  | ✅  | —   | ✅  |
| `lidar_tracks`         | `bounding_box_height_avg`         | REAL          | ✅  | ✅  | —   | ✅  |
| `lidar_tracks`         | `height_p95_max`                  | REAL          | ✅  | ✅  | —   | ✅  |
| `lidar_tracks`         | `intensity_mean_avg`              | REAL          | ✅  | ✅  | —   | ✅  |
| `lidar_tracks`         | `object_class`                    | TEXT          | ✅  | ✅  | —   | ✅  |
| `lidar_tracks`         | `object_confidence`               | REAL          | ✅  | ✅  | —   | ✅  |
| `lidar_tracks`         | `classification_model`            | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_tracks`         | `track_length_meters`             | REAL          | 🔶  | 📋  | —   | ✅  |
| `lidar_tracks`         | `track_duration_secs`             | REAL          | 🔶  | 📋  | —   | ✅  |
| `lidar_tracks`         | `occlusion_count`                 | INTEGER       | 🔶  | 📋  | —   | ✅  |
| `lidar_tracks`         | `max_occlusion_frames`            | INTEGER       | 🔶  | 📋  | —   | —   |
| `lidar_tracks`         | `spatial_coverage`                | REAL          | 🔶  | 📋  | —   | —   |
| `lidar_tracks`         | `noise_point_ratio`               | REAL          | 🔶  | 📋  | —   | —   |
| `lidar_track_obs`      | `track_id`                        | TEXT PK       | ✅  | ✅  | —   | —   |
| `lidar_track_obs`      | `ts_unix_nanos`                   | INTEGER PK    | ✅  | ✅  | —   | —   |
| `lidar_track_obs`      | `world_frame`                     | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_track_obs`      | `x`                               | REAL          | ✅  | ✅  | —   | —   |
| `lidar_track_obs`      | `y`                               | REAL          | ✅  | ✅  | —   | —   |
| `lidar_track_obs`      | `z`                               | REAL          | ✅  | ✅  | —   | —   |
| `lidar_track_obs`      | `velocity_x`                      | REAL          | ✅  | ✅  | —   | —   |
| `lidar_track_obs`      | `velocity_y`                      | REAL          | ✅  | ✅  | —   | —   |
| `lidar_track_obs`      | `speed_mps`                       | REAL          | ✅  | ✅  | —   | —   |
| `lidar_track_obs`      | `heading_rad`                     | REAL          | ✅  | ✅  | —   | —   |
| `lidar_track_obs`      | `bounding_box_length`             | REAL          | ✅  | ✅  | —   | —   |
| `lidar_track_obs`      | `bounding_box_width`              | REAL          | ✅  | ✅  | —   | —   |
| `lidar_track_obs`      | `bounding_box_height`             | REAL          | ✅  | ✅  | —   | —   |
| `lidar_track_obs`      | `height_p95`                      | REAL          | ✅  | ✅  | —   | —   |
| `lidar_track_obs`      | `intensity_mean`                  | REAL          | ✅  | ✅  | —   | —   |
| `lidar_run_tracks`     | `run_id`                          | TEXT PK       | ✅  | ✅  | —   | —   |
| `lidar_run_tracks`     | `track_id`                        | TEXT PK       | ✅  | ✅  | —   | —   |
| `lidar_run_tracks`     | `sensor_id`                       | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_run_tracks`     | `track_state`                     | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_run_tracks`     | `start_unix_nanos`                | INTEGER       | ✅  | ✅  | —   | —   |
| `lidar_run_tracks`     | `end_unix_nanos`                  | INTEGER       | ✅  | ✅  | —   | —   |
| `lidar_run_tracks`     | `observation_count`               | INTEGER       | ✅  | ✅  | —   | —   |
| `lidar_run_tracks`     | `avg_speed_mps`                   | REAL          | ✅  | ✅  | —   | —   |
| `lidar_run_tracks`     | `peak_speed_mps`                  | REAL          | ✅  | ✅  | —   | —   |
| `lidar_run_tracks`     | `p50_speed_mps`                   | REAL          | 🗑️  | —   | —   | —   |
| `lidar_run_tracks`     | `p85_speed_mps`                   | REAL          | 🗑️  | —   | —   | —   |
| `lidar_run_tracks`     | `p95_speed_mps`                   | REAL          | 🗑️  | —   | —   | —   |
| `lidar_run_tracks`     | `bounding_box_length_avg`         | REAL          | ✅  | ✅  | —   | —   |
| `lidar_run_tracks`     | `bounding_box_width_avg`          | REAL          | ✅  | ✅  | —   | —   |
| `lidar_run_tracks`     | `bounding_box_height_avg`         | REAL          | ✅  | ✅  | —   | —   |
| `lidar_run_tracks`     | `height_p95_max`                  | REAL          | ✅  | ✅  | —   | —   |
| `lidar_run_tracks`     | `intensity_mean_avg`              | REAL          | ✅  | ✅  | —   | —   |
| `lidar_run_tracks`     | `object_class`                    | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_run_tracks`     | `object_confidence`               | REAL          | ✅  | ✅  | —   | —   |
| `lidar_run_tracks`     | `classification_model`            | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_run_tracks`     | `user_label`                      | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_run_tracks`     | `label_confidence`                | REAL          | ✅  | ✅  | —   | —   |
| `lidar_run_tracks`     | `labeler_id`                      | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_run_tracks`     | `labeled_at`                      | INTEGER       | ✅  | ✅  | —   | —   |
| `lidar_run_tracks`     | `is_split_candidate`              | INTEGER       | ✅  | ✅  | —   | —   |
| `lidar_run_tracks`     | `is_merge_candidate`              | INTEGER       | ✅  | ✅  | —   | —   |
| `lidar_run_tracks`     | `linked_track_ids`                | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_run_tracks`     | `quality_label`                   | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_run_tracks`     | `label_source`                    | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_labels`         | `label_id`                        | TEXT PK       | ✅  | ✅  | —   | —   |
| `lidar_labels`         | `track_id`                        | TEXT FK       | ✅  | ✅  | —   | —   |
| `lidar_labels`         | `class_label`                     | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_labels`         | `start_timestamp_ns`              | INTEGER       | ✅  | ✅  | —   | —   |
| `lidar_labels`         | `end_timestamp_ns`                | INTEGER       | ✅  | ✅  | —   | —   |
| `lidar_labels`         | `confidence`                      | REAL          | ✅  | ✅  | —   | —   |
| `lidar_labels`         | `created_by`                      | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_labels`         | `created_at_ns`                   | INTEGER       | ✅  | ✅  | —   | —   |
| `lidar_labels`         | `updated_at_ns`                   | INTEGER       | ✅  | ✅  | —   | —   |
| `lidar_labels`         | `notes`                           | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_labels`         | `scene_id`                        | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_labels`         | `source_file`                     | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_scenes`         | `scene_id`                        | TEXT PK       | ✅  | ✅  | —   | —   |
| `lidar_scenes`         | `sensor_id`                       | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_scenes`         | `pcap_file`                       | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_scenes`         | `pcap_start_secs`                 | REAL          | ✅  | ✅  | —   | —   |
| `lidar_scenes`         | `pcap_duration_secs`              | REAL          | ✅  | ✅  | —   | —   |
| `lidar_scenes`         | `description`                     | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_scenes`         | `reference_run_id`                | TEXT FK       | ✅  | ✅  | —   | —   |
| `lidar_scenes`         | `optimal_params_json`             | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_scenes`         | `created_at_ns`                   | INTEGER       | ✅  | ✅  | —   | —   |
| `lidar_scenes`         | `updated_at_ns`                   | INTEGER       | ✅  | ✅  | —   | —   |
| `lidar_evaluations`    | `evaluation_id`                   | TEXT PK       | ✅  | ✅  | —   | —   |
| `lidar_evaluations`    | `scene_id`                        | TEXT FK       | ✅  | ✅  | —   | —   |
| `lidar_evaluations`    | `reference_run_id`                | TEXT FK       | ✅  | ✅  | —   | —   |
| `lidar_evaluations`    | `candidate_run_id`                | TEXT FK       | ✅  | ✅  | —   | —   |
| `lidar_evaluations`    | `detection_rate`                  | REAL          | ✅  | ✅  | —   | —   |
| `lidar_evaluations`    | `fragmentation`                   | REAL          | ✅  | ✅  | —   | —   |
| `lidar_evaluations`    | `false_positive_rate`             | REAL          | ✅  | ✅  | —   | —   |
| `lidar_evaluations`    | `velocity_coverage`               | REAL          | ✅  | ✅  | —   | —   |
| `lidar_evaluations`    | `quality_premium`                 | REAL          | ✅  | ✅  | —   | —   |
| `lidar_evaluations`    | `truncation_rate`                 | REAL          | ✅  | ✅  | —   | —   |
| `lidar_evaluations`    | `velocity_noise_rate`             | REAL          | ✅  | ✅  | —   | —   |
| `lidar_evaluations`    | `stopped_recovery_rate`           | REAL          | ✅  | ✅  | —   | —   |
| `lidar_evaluations`    | `composite_score`                 | REAL          | ✅  | ✅  | —   | —   |
| `lidar_evaluations`    | `matched_count`                   | INTEGER       | ✅  | ✅  | —   | —   |
| `lidar_evaluations`    | `reference_count`                 | INTEGER       | ✅  | ✅  | —   | —   |
| `lidar_evaluations`    | `candidate_count`                 | INTEGER       | ✅  | ✅  | —   | —   |
| `lidar_evaluations`    | `params_json`                     | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_evaluations`    | `created_at`                      | INTEGER       | ✅  | ✅  | —   | —   |
| `lidar_sweeps`         | `id`                              | INTEGER PK    | ✅  | —   | —   | —   |
| `lidar_sweeps`         | `sweep_id`                        | TEXT UNIQUE   | ✅  | ✅  | —   | —   |
| `lidar_sweeps`         | `sensor_id`                       | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_sweeps`         | `mode`                            | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_sweeps`         | `status`                          | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_sweeps`         | `request`                         | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_sweeps`         | `results`                         | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_sweeps`         | `charts`                          | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_sweeps`         | `recommendation`                  | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_sweeps`         | `round_results`                   | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_sweeps`         | `error`                           | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_sweeps`         | `started_at`                      | DATETIME      | ✅  | ✅  | —   | —   |
| `lidar_sweeps`         | `completed_at`                    | DATETIME      | ✅  | ✅  | —   | —   |
| `lidar_sweeps`         | `created_at`                      | DATETIME      | ✅  | —   | —   | —   |
| `lidar_sweeps`         | `objective_name`                  | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_sweeps`         | `objective_version`               | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_sweeps`         | `transform_pipeline_name`         | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_sweeps`         | `transform_pipeline_version`      | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_sweeps`         | `score_components_json`           | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_sweeps`         | `recommendation_explanation_json` | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_sweeps`         | `label_provenance_summary_json`   | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_sweeps`         | `checkpoint_round`                | INTEGER       | ✅  | ✅  | —   | —   |
| `lidar_sweeps`         | `checkpoint_bounds`               | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_sweeps`         | `checkpoint_results`              | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_sweeps`         | `checkpoint_request`              | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_bg_snapshot`    | `snapshot_id`                     | INTEGER PK    | ✅  | ✅  | —   | —   |
| `lidar_bg_snapshot`    | `sensor_id`                       | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_bg_snapshot`    | `taken_unix_nanos`                | INTEGER       | ✅  | ✅  | —   | —   |
| `lidar_bg_snapshot`    | `rings`                           | INTEGER       | ✅  | ✅  | —   | —   |
| `lidar_bg_snapshot`    | `azimuth_bins`                    | INTEGER       | ✅  | ✅  | —   | —   |
| `lidar_bg_snapshot`    | `params_json`                     | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_bg_snapshot`    | `ring_elevations_json`            | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_bg_snapshot`    | `grid_blob`                       | BLOB          | ✅  | ✅  | —   | —   |
| `lidar_bg_snapshot`    | `changed_cells_count`             | INTEGER       | ✅  | ✅  | —   | —   |
| `lidar_bg_snapshot`    | `snapshot_reason`                 | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_bg_regions`     | `region_set_id`                   | INTEGER PK    | ✅  | ✅  | —   | —   |
| `lidar_bg_regions`     | `snapshot_id`                     | INTEGER FK    | ✅  | ✅  | —   | —   |
| `lidar_bg_regions`     | `sensor_id`                       | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_bg_regions`     | `created_unix_nanos`              | INTEGER       | ✅  | ✅  | —   | —   |
| `lidar_bg_regions`     | `region_count`                    | INTEGER       | ✅  | ✅  | —   | —   |
| `lidar_bg_regions`     | `regions_json`                    | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_bg_regions`     | `variance_data_json`              | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_bg_regions`     | `settling_frames`                 | INTEGER       | ✅  | ✅  | —   | —   |
| `lidar_bg_regions`     | `scene_hash`                      | TEXT          | ✅  | —   | —   | —   |
| `lidar_bg_regions`     | `source_path`                     | TEXT          | ✅  | —   | —   | —   |
| `lidar_missed_regions` | `region_id`                       | TEXT PK       | ✅  | ✅  | —   | —   |
| `lidar_missed_regions` | `run_id`                          | TEXT FK       | ✅  | ✅  | —   | —   |
| `lidar_missed_regions` | `center_x`                        | REAL          | ✅  | ✅  | —   | —   |
| `lidar_missed_regions` | `center_y`                        | REAL          | ✅  | ✅  | —   | —   |
| `lidar_missed_regions` | `radius_m`                        | REAL          | ✅  | ✅  | —   | —   |
| `lidar_missed_regions` | `time_start_ns`                   | INTEGER       | ✅  | ✅  | —   | —   |
| `lidar_missed_regions` | `time_end_ns`                     | INTEGER       | ✅  | ✅  | —   | —   |
| `lidar_missed_regions` | `expected_label`                  | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_missed_regions` | `labeler_id`                      | TEXT          | ✅  | ✅  | —   | —   |
| `lidar_missed_regions` | `labeled_at`                      | INTEGER       | ✅  | ✅  | —   | —   |
| `lidar_missed_regions` | `notes`                           | TEXT          | ✅  | ✅  | —   | —   |
| `radar_data`           | `write_timestamp`                 | DOUBLE        | ✅  | —   | —   | —   |
| `radar_data`           | `raw_event`                       | JSON          | ✅  | —   | —   | —   |
| `radar_data`           | `uptime`                          | DOUBLE STORED | ✅  | ✅  | —   | —   |
| `radar_data`           | `magnitude`                       | DOUBLE STORED | ✅  | ✅  | —   | —   |
| `radar_data`           | `speed`                           | DOUBLE STORED | ✅  | ✅  | ✅  | —   |
| `radar_objects`        | `write_timestamp`                 | DOUBLE        | ✅  | ✅  | ✅  | —   |
| `radar_objects`        | `raw_event`                       | JSON          | ✅  | —   | —   | —   |
| `radar_objects`        | `classifier`                      | TEXT STORED   | ✅  | ✅  | ✅  | —   |
| `radar_objects`        | `start_time`                      | DOUBLE STORED | ✅  | —   | —   | —   |
| `radar_objects`        | `end_time`                        | DOUBLE STORED | ✅  | —   | —   | —   |
| `radar_objects`        | `delta_time_ms`                   | BIGINT STORED | ✅  | —   | —   | —   |
| `radar_objects`        | `max_speed`                       | DOUBLE STORED | ✅  | ✅  | ✅  | —   |
| `radar_objects`        | `min_speed`                       | DOUBLE STORED | ✅  | —   | —   | —   |
| `radar_objects`        | `speed_change`                    | DOUBLE STORED | ✅  | —   | —   | —   |
| `radar_objects`        | `max_magnitude`                   | BIGINT STORED | ✅  | —   | —   | —   |
| `radar_objects`        | `avg_magnitude`                   | BIGINT STORED | ✅  | —   | —   | —   |
| `radar_objects`        | `total_frames`                    | BIGINT STORED | ✅  | —   | —   | —   |
| `radar_objects`        | `frames_per_mps`                  | DOUBLE STORED | ✅  | —   | —   | —   |
| `radar_objects`        | `length_m`                        | DOUBLE STORED | ✅  | —   | —   | —   |
| `radar_data_transits`  | `transit_id`                      | INTEGER PK    | ✅  | —   | —   | —   |
| `radar_data_transits`  | `transit_key`                     | TEXT UNIQUE   | ✅  | —   | —   | —   |
| `radar_data_transits`  | `threshold_ms`                    | INTEGER       | ✅  | —   | —   | —   |
| `radar_data_transits`  | `transit_start_unix`              | DOUBLE        | ✅  | ✅  | ✅  | —   |
| `radar_data_transits`  | `transit_end_unix`                | DOUBLE        | ✅  | —   | —   | —   |
| `radar_data_transits`  | `transit_max_speed`               | DOUBLE        | ✅  | ✅  | ✅  | —   |
| `radar_data_transits`  | `transit_min_speed`               | DOUBLE        | ✅  | —   | —   | —   |
| `radar_data_transits`  | `transit_max_magnitude`           | BIGINT        | ✅  | —   | —   | —   |
| `radar_data_transits`  | `transit_min_magnitude`           | BIGINT        | ✅  | —   | —   | —   |
| `radar_data_transits`  | `point_count`                     | INTEGER       | ✅  | —   | —   | —   |
| `radar_data_transits`  | `model_version`                   | TEXT          | ✅  | ✅  | ✅  | —   |
| `radar_data_transits`  | `created_at`                      | DOUBLE        | ✅  | —   | —   | —   |
| `radar_data_transits`  | `updated_at`                      | DOUBLE        | ✅  | —   | —   | —   |
| `radar_transit_links`  | `link_id`                         | INTEGER PK    | ✅  | —   | —   | —   |
| `radar_transit_links`  | `transit_id`                      | INTEGER FK    | ✅  | —   | —   | —   |
| `radar_transit_links`  | `data_rowid`                      | INTEGER FK    | ✅  | —   | —   | —   |
| `radar_transit_links`  | `link_score`                      | DOUBLE        | ✅  | —   | —   | —   |
| `radar_transit_links`  | `created_at`                      | DOUBLE        | ✅  | —   | —   | —   |
| `radar_commands`       | `command_id`                      | BIGINT PK     | ✅  | —   | —   | —   |
| `radar_commands`       | `command`                         | TEXT          | ✅  | —   | —   | —   |
| `radar_commands`       | `write_timestamp`                 | DOUBLE        | ✅  | —   | —   | —   |
| `radar_command_log`    | `log_id`                          | BIGINT PK     | ✅  | —   | —   | —   |
| `radar_command_log`    | `command_id`                      | BIGINT FK     | ✅  | —   | —   | —   |
| `radar_command_log`    | `log_data`                        | TEXT          | ✅  | —   | —   | —   |
| `radar_command_log`    | `write_timestamp`                 | DOUBLE        | ✅  | —   | —   | —   |
| `site`                 | `id`                              | INTEGER PK    | ✅  | ✅  | ✅  | —   |
| `site`                 | `name`                            | TEXT UNIQUE   | ✅  | ✅  | ✅  | —   |
| `site`                 | `location`                        | TEXT          | ✅  | ✅  | ✅  | —   |
| `site`                 | `description`                     | TEXT          | ✅  | ✅  | —   | —   |
| `site`                 | `surveyor`                        | TEXT          | ✅  | ✅  | ✅  | —   |
| `site`                 | `contact`                         | TEXT          | ✅  | ✅  | ✅  | —   |
| `site`                 | `address`                         | TEXT          | ✅  | ✅  | —   | —   |
| `site`                 | `latitude`                        | REAL          | ✅  | ✅  | ✅  | —   |
| `site`                 | `longitude`                       | REAL          | ✅  | ✅  | ✅  | —   |
| `site`                 | `map_angle`                       | REAL          | ✅  | ✅  | ✅  | —   |
| `site`                 | `include_map`                     | INTEGER       | ✅  | ✅  | ✅  | —   |
| `site`                 | `site_description`                | TEXT          | ✅  | ✅  | ✅  | —   |
| `site`                 | `bbox_ne_lat`                     | REAL          | ✅  | ✅  | ✅  | —   |
| `site`                 | `bbox_ne_lng`                     | REAL          | ✅  | ✅  | ✅  | —   |
| `site`                 | `bbox_sw_lat`                     | REAL          | ✅  | ✅  | ✅  | —   |
| `site`                 | `bbox_sw_lng`                     | REAL          | ✅  | ✅  | ✅  | —   |
| `site`                 | `map_svg_data`                    | BLOB          | ✅  | ✅  | —   | —   |
| `site`                 | `created_at`                      | INTEGER       | ✅  | ✅  | —   | —   |
| `site`                 | `updated_at`                      | INTEGER       | ✅  | ✅  | —   | —   |
| `site_config_periods`  | `id`                              | INTEGER PK    | ✅  | ✅  | —   | —   |
| `site_config_periods`  | `site_id`                         | INTEGER FK    | ✅  | ✅  | ✅  | —   |
| `site_config_periods`  | `effective_start_unix`            | DOUBLE        | ✅  | ✅  | —   | —   |
| `site_config_periods`  | `effective_end_unix`              | DOUBLE        | ✅  | ✅  | —   | —   |
| `site_config_periods`  | `is_active`                       | INTEGER       | ✅  | ✅  | ✅  | —   |
| `site_config_periods`  | `notes`                           | TEXT          | ✅  | ✅  | —   | —   |
| `site_config_periods`  | `cosine_error_angle`              | DOUBLE        | ✅  | ✅  | ✅  | —   |
| `site_config_periods`  | `created_at`                      | DOUBLE        | ✅  | ✅  | —   | —   |
| `site_config_periods`  | `updated_at`                      | DOUBLE        | ✅  | ✅  | —   | —   |
| `site_reports`         | `id`                              | INTEGER PK    | ✅  | ✅  | —   | —   |
| `site_reports`         | `site_id`                         | INTEGER FK    | ✅  | ✅  | ✅  | —   |
| `site_reports`         | `start_date`                      | TEXT          | ✅  | ✅  | ✅  | —   |
| `site_reports`         | `end_date`                        | TEXT          | ✅  | ✅  | ✅  | —   |
| `site_reports`         | `filepath`                        | TEXT          | ✅  | ✅  | ✅  | —   |
| `site_reports`         | `filename`                        | TEXT          | ✅  | ✅  | ✅  | —   |
| `site_reports`         | `zip_filepath`                    | TEXT          | ✅  | ✅  | ✅  | —   |
| `site_reports`         | `zip_filename`                    | TEXT          | ✅  | ✅  | ✅  | —   |
| `site_reports`         | `run_id`                          | TEXT          | ✅  | ✅  | ✅  | —   |
| `site_reports`         | `timezone`                        | TEXT          | ✅  | ✅  | ✅  | —   |
| `site_reports`         | `units`                           | TEXT          | ✅  | ✅  | ✅  | —   |
| `site_reports`         | `source`                          | TEXT          | ✅  | ✅  | ✅  | —   |
| `site_reports`         | `created_at`                      | DATETIME      | ✅  | ✅  | —   | —   |
| `schema_migrations`    | `version`                         | UINT64        | ✅  | —   | —   | —   |
| `schema_migrations`    | `dirty`                           | BOOLEAN       | ✅  | —   | —   | —   |

---

## 6. Pipeline Stages

| Folder                        | File               | Stage                                      | DB  | Web | PDF | Mac |
| ----------------------------- | ------------------ | ------------------------------------------ | --- | --- | --- | --- |
| `internal/lidar/l2frames`     | `frame_builder.go` | L2 Frame Builder (UDP → point clouds)      | —   | —   | —   | —   |
| `internal/lidar/l3grid`       | `background.go`    | L3 Background Grid (foreground/background) | ✅  | ✅  | —   | —   |
| `internal/lidar/l3grid`       | `foreground.go`    | L3 FrameMetrics (foreground fraction)      | —   | —   | —   | —   |
| `internal/lidar/l4perception` | `dbscan.go`        | L4 Clustering (DBSCAN → world clusters)    | ✅  | ✅  | —   | ✅  |
| `internal/lidar/l5tracks`     | `tracking.go`      | L5 Tracking (Kalman → tracked objects)     | ✅  | ✅  | —   | ✅  |
| `internal/lidar/l5tracks`     | `tracking.go`      | L5 TrackingMetrics (fragmentation, jitter) | —   | 🔶  | —   | —   |
| `internal/lidar/adapters`     | `ground_truth.go`  | L6 Evaluation (quality metrics)            | ✅  | ✅  | —   | —   |
| `internal/lidar/l6objects`    | `quality.go`       | L6 RunStatistics (12 fields)               | 📋  | 📋  | —   | —   |
| `internal/lidar/l6objects`    | `quality.go`       | L6 TrackQualityMetrics (8 fields)          | ✅  | 📋  | —   | —   |
| `internal/lidar/l6objects`    | `quality.go`       | L6 NoiseCoverageMetrics (7 fields)         | 📋  | 📋  | —   | —   |
| `internal/lidar/l6objects`    | `quality.go`       | L6 TrainingDatasetSummary (7 fields)       | —   | —   | —   | —   |
| `internal/lidar/l6objects`    | `features.go`      | L6 TrackFeatures (20 features)             | —   | —   | —   | —   |
| `internal/lidar/l6objects`    | `features.go`      | L6 ClusterFeatures (10 features)           | —   | —   | —   | —   |

---

## 7. Go Data Structures — Computed but Not Persisted

These structs are computed in-memory but have no persistence layer, no API
endpoint, and no export path.

| Folder                     | File            | Struct                              | DB  | Web | PDF | Mac | Notes                                                                         |
| -------------------------- | --------------- | ----------------------------------- | --- | --- | --- | --- | ----------------------------------------------------------------------------- |
| `internal/lidar/l6objects` | `quality.go`    | `NoiseCoverageMetrics` (7 fields)   | 📋  | 📋  | —   | —   | Partially implemented (TODOs)                                                 |
| `internal/lidar/l6objects` | `quality.go`    | `TrainingDatasetSummary` (7 fields) | —   | —   | —   | —   | No consumer; separate project                                                 |
| `internal/lidar/l6objects` | `features.go`   | `TrackFeatures` (20 features)       | —   | —   | —   | —   | Used in-memory by classifier                                                  |
| `internal/lidar/l6objects` | `features.go`   | `ClusterFeatures` (10 features)     | —   | —   | —   | —   | Used in-memory by classifier                                                  |
| `internal/lidar/l3grid`    | `foreground.go` | `FrameMetrics` (5 fields)           | 📋  | 📋  | —   | —   | Transient; [HINT plan C1](../../docs/plans/hint-metric-observability-plan.md) |
| `internal/lidar/l5tracks`  | `tracking.go`   | `TrackAlignmentMetrics` (per-track) | 📋  | 📋  | —   | —   | [HINT plan D2](../../docs/plans/hint-metric-observability-plan.md)            |
| `internal/lidar/sweep`     | `runner.go`     | `ComboResult` (32 fields)           | 🔶  | 🔶  | —   | —   | Only `BestScore` persisted                                                    |

---

## 8. Go Data Structures — Comparison Logic (No Triggering Endpoint)

| Folder                          | File                      | Function                  | DB  | Web | PDF | Mac | Notes                       |
| ------------------------------- | ------------------------- | ------------------------- | --- | --- | --- | --- | --------------------------- |
| `internal/lidar/storage/sqlite` | `analysis_run_compare.go` | `compareParams()`         | ✅  | 📋  | —   | —   | Needs API endpoint          |
| `internal/lidar/storage/sqlite` | `analysis_run_compare.go` | `computeTemporalIoU()`    | ✅  | 📋  | —   | —   | Needs API endpoint          |
| `internal/lidar/storage/sqlite` | `analysis_run_compare.go` | `is_split_candidate` flag | ✅  | ✅  | —   | —   | Written but not triggerable |
| `internal/lidar/storage/sqlite` | `analysis_run_compare.go` | `is_merge_candidate` flag | ✅  | ✅  | —   | —   | Written but not triggerable |

---

## 9. Live Track Fields — Fully Wired (Reference)

Fields that flow correctly from pipeline through all applicable surfaces.

| Folder                    | File          | Field                | DB  | Web | PDF | Mac |
| ------------------------- | ------------- | -------------------- | --- | --- | --- | --- |
| `internal/lidar/l5tracks` | `tracking.go` | `track_id`           | ✅  | ✅  | —   | ✅  |
| `internal/lidar/l5tracks` | `tracking.go` | `sensor_id`          | ✅  | ✅  | —   | ✅  |
| `internal/lidar/l5tracks` | `tracking.go` | `track_state`        | ✅  | ✅  | —   | ✅  |
| `internal/lidar/l5tracks` | `tracking.go` | `position (x, y, z)` | ✅  | ✅  | —   | ✅  |
| `internal/lidar/l5tracks` | `tracking.go` | `velocity (vx, vy)`  | ✅  | ✅  | —   | ✅  |
| `internal/lidar/l5tracks` | `tracking.go` | `speed_mps`          | ✅  | ✅  | —   | ✅  |
| `internal/lidar/l5tracks` | `tracking.go` | `heading_rad`        | ✅  | ✅  | —   | ✅  |
| `internal/lidar/l5tracks` | `tracking.go` | `observation_count`  | ✅  | ✅  | —   | ✅  |
| `internal/lidar/l5tracks` | `tracking.go` | `avg_speed_mps`      | ✅  | ✅  | —   | ✅  |
| `internal/lidar/l5tracks` | `tracking.go` | `peak_speed_mps`     | ✅  | ✅  | —   | ✅  |
| `internal/lidar/l5tracks` | `tracking.go` | `bounding_box_*`     | ✅  | ✅  | —   | ✅  |
| `internal/lidar/l5tracks` | `tracking.go` | `height_p95_max`     | ✅  | ✅  | —   | ✅  |
| `internal/lidar/l5tracks` | `tracking.go` | `intensity_mean_avg` | ✅  | ✅  | —   | ✅  |
| `internal/lidar/l5tracks` | `tracking.go` | `object_class`       | ✅  | ✅  | —   | ✅  |
| `internal/lidar/l5tracks` | `tracking.go` | `object_confidence`  | ✅  | ✅  | —   | ✅  |
| `internal/lidar/l5tracks` | `tracking.go` | `heading_source`     | —   | ✅  | —   | ✅  |

---

## 10. Tuning Parameters

| Folder            | File                   | Parameter Group          | DB  | Web | PDF | Mac |
| ----------------- | ---------------------- | ------------------------ | --- | --- | --- | --- |
| `internal/config` | `tuning.go`            | L3 Background (8 params) | ✅  | ✅  | —   | —   |
| `internal/config` | `tuning.go`            | L4 Perception (3 params) | ✅  | ✅  | —   | —   |
| `internal/config` | `tuning.go`            | L5 Tracker (14 params)   | ✅  | ✅  | —   | —   |
| `config`          | `tuning.defaults.json` | Default values           | —   | —   | —   | —   |

---

## 11. PDF Generator — Python Surfaces

**Source:** `tools/pdf-generator/pdf_generator/core/api_client.py`

| Folder                       | File                  | Consumer                                                 | DB  | Web | PDF | Mac |
| ---------------------------- | --------------------- | -------------------------------------------------------- | --- | --- | --- | --- |
| `tools/pdf-generator/…/core` | `api_client.py`       | `RadarStatsClient.query()` → `/api/radar_stats`          | —   | —   | ✅  | —   |
| `tools/pdf-generator/…/core` | `api_client.py`       | `get_site_config_periods()` → `/api/site_config_periods` | —   | —   | ✅  | —   |
| `tools/pdf-generator/…/core` | `api_client.py`       | `get_site()` → `/api/sites/{id}`                         | —   | —   | ✅  | —   |
| `tools/pdf-generator/…/core` | `chart_builder.py`    | Speed charts + histograms                                | —   | —   | ✅  | —   |
| `tools/pdf-generator/…/core` | `document_builder.py` | LaTeX document assembly                                  | —   | —   | ✅  | —   |
| `tools/pdf-generator/…/core` | `map_utils.py`        | Site map generation                                      | —   | —   | ✅  | —   |

---

## 12. macOS Visualiser — Swift Surfaces

**Source:** `tools/visualiser-macos/VelocityVisualiser/`

| Folder                   | File                         | Consumer                                    | DB  | Web | PDF | Mac |
| ------------------------ | ---------------------------- | ------------------------------------------- | --- | --- | --- | --- |
| `tools/visualiser-macos` | `GRPCClient.swift`           | `StreamFrames` subscriber                   | —   | —   | —   | ✅  |
| `tools/visualiser-macos` | `GRPCClient.swift`           | Playback controls (Pause/Play/Seek/SetRate) | —   | —   | —   | ✅  |
| `tools/visualiser-macos` | `GRPCClient.swift`           | Overlay mode toggles                        | —   | —   | —   | ✅  |
| `tools/visualiser-macos` | `PointCloudRenderer.swift`   | Point cloud rendering (Metal)               | —   | —   | —   | ✅  |
| `tools/visualiser-macos` | `TrackRenderer.swift`        | Track boxes + velocity vectors              | —   | —   | —   | ✅  |
| `tools/visualiser-macos` | `DebugOverlayRenderer.swift` | Gating ellipses, residuals, predictions     | —   | —   | —   | ✅  |

---

## 13. Classification Pipeline — Fully Wired (Reference)

**Go source:** `internal/lidar/l6objects/classification.go` — `TrackClassifier` (27 usages)

| Folder                     | File                | Component                 | DB  | Web | PDF | Mac |
| -------------------------- | ------------------- | ------------------------- | --- | --- | --- | --- |
| `internal/lidar/l6objects` | `classification.go` | `ObjectClass` (8 classes) | ✅  | ✅  | —   | ✅  |
| `internal/lidar/l6objects` | `classification.go` | `ClassificationResult`    | ✅  | ✅  | —   | ✅  |
| `internal/lidar/l6objects` | `classification.go` | `ClassificationFeatures`  | —   | —   | —   | 🔶  |
| `internal/lidar/l6objects` | `classification.go` | `TrackClassifier`         | —   | ✅  | —   | ✅  |

**Notes:** `ObjectClass` + `ClassificationResult` (class + confidence) flow
through the full pipeline: tracker → DB → Web API → gRPC → Mac.
`ClassificationFeatures` (9 inputs) are only exposed via the gRPC
`classifyOrConvert()` replay path — visible to Mac during VRLOG playback
but never persisted. `TrackClassifier` is set on both WebServer and gRPC
server as a service object, not persisted data.

---

## 14. FrameBundle — macOS-Only Proto Fields

**Proto:** `proto/velocity_visualiser/v1/visualiser.proto` — `FrameBundle`
**Consumer:** macOS Metal visualiser via `StreamFrames` gRPC stream (~30 fps)

These fields are **only** consumed by the macOS visualiser. They are not
persisted to SQLite, not exposed to the Web UI, and not used by the PDF
generator.

| Field Group                | Field Count | macOS | DB  | Web | PDF |
| -------------------------- | ----------- | ----- | --- | --- | --- |
| Point cloud (x/y/z/i/c)    | 7           | ✅    | —   | —   | —   |
| Background snapshot (M3.5) | 6           | ✅    | —   | —   | —   |
| Cluster OBB (7-DOF)        | 7           | ✅    | —   | —   | —   |
| Track trails               | 3           | ✅    | —   | —   | —   |
| Track alpha/opacity        | 1           | ✅    | —   | —   | —   |
| Covariance 4×4             | 1           | ✅    | —   | —   | —   |
| Debug: associations        | 4           | ✅    | —   | —   | —   |
| Debug: gating ellipses     | 5           | ✅    | —   | —   | —   |
| Debug: residuals           | 5           | ✅    | —   | —   | —   |
| Debug: predictions         | 5           | ✅    | —   | —   | —   |
| Playback info              | 8           | ✅    | —   | —   | —   |
| Coordinate frame           | 6           | ✅    | —   | —   | —   |

**Status:** These are intentionally Mac-only — they serve real-time
visualisation and debugging, not analysis or reporting. No wiring gap.

---

## 15. ECharts Dashboard Endpoints

**Go source:** `internal/lidar/monitor/chart_api.go` + `webserver.go`
**Consumer:** Embedded ECharts dashboards (served from `/assets/*` via `go:embed`)

| Endpoint                      | Method | Data Source       | DB  | Web | PDF | Mac |
| ----------------------------- | ------ | ----------------- | --- | --- | --- | --- |
| `/api/lidar/chart/polar`      | GET    | Background grid   | —   | ✅  | —   | —   |
| `/api/lidar/chart/heatmap`    | GET    | Background grid   | —   | ✅  | —   | —   |
| `/api/lidar/chart/foreground` | GET    | Foreground points | —   | ✅  | —   | —   |
| `/api/lidar/chart/clusters`   | GET    | Cluster positions | —   | ✅  | —   | —   |
| `/api/lidar/chart/traffic`    | GET    | Track activity    | —   | ✅  | —   | —   |

**Notes:** These endpoints return ECharts-compatible JSON for the debug
dashboards at `/debug/lidar/*`. They are consumed by embedded HTML pages
(not the Svelte SPA). The Svelte frontend uses LayerChart for its own
charts (e.g. `RadarOverviewChart.svelte` consuming `/api/radar_stats`).

---

## 16. cmd/ Entry Points

| Binary                | Location                              | Consumers                                    |
| --------------------- | ------------------------------------- | -------------------------------------------- |
| `velocity-report`     | `cmd/radar/radar.go`                  | Full server: API, DB, serial, LiDAR pipeline |
| `velocity-sweep`      | `cmd/sweep/main.go`                   | LiDAR monitor, sweep engine, PCAP replay     |
| `velocity-deploy`     | `cmd/deploy/main.go`                  | Deployment: install, upgrade, backup         |
| `transit-backfill`    | `cmd/transit-backfill/main.go`        | DB: TransitWorker backfill                   |
| `gen-vrlog`           | `cmd/tools/gen-vrlog/main.go`         | Synthetic VRLOG generation (no DB)           |
| `vrlog-analyse`       | `cmd/tools/vrlog-analyse/main.go`     | VRLOG file analysis and comparison           |
| `visualiser-server`   | `cmd/tools/visualiser-server/main.go` | Standalone gRPC (synthetic/replay/live)      |
| `settling-eval`       | `cmd/tools/settling-eval/main.go`     | Background grid settling evaluation          |
| `pcap-analyse`        | `cmd/tools/pcap-analyse/main.go`      | PCAP file analysis                           |
| `backfill-elevations` | `cmd/tools/backfill_ring_elevations/` | Backfill ring elevation data                 |
| `scan-transits`       | `cmd/tools/scan_transits.go`          | Scan for missing transit backfill gaps       |

**Notes:** Only `velocity-report` and `transit-backfill` write to the
production SQLite database. The sweep and eval tools operate on
temporary/in-memory databases.

---

## 17. Speed Percentile Columns — Design Debt

**DB columns:** `lidar_tracks` and `lidar_run_tracks` both have
`p50_speed_mps`, `p85_speed_mps`, `p95_speed_mps` (marked 🗑️ in §5).

Per the [speed percentile alignment plan](../../docs/plans/speed-percentile-aggregation-alignment-plan.md)
(D-18), per-track percentiles are the **wrong abstraction** — percentiles
are reserved for grouped/report aggregates only. These 6 columns should be
**removed** via migration 000030. See
[schema simplification plan](../../docs/plans/schema-simplification-migration-030-plan.md).

---

## 18. Debug / Admin Routes

Embedded HTML dashboards and diagnostic endpoints. Not part of the
application API but served by the same HTTP servers.

### Radar server (`internal/api/server.go`)

| Route                 | Purpose                            |
| --------------------- | ---------------------------------- |
| `/favicon.ico`        | Static favicon                     |
| `/app/*`              | Svelte SPA (embedded or dev proxy) |
| `/`                   | Redirect to `/app/`                |
| `/debug/pprof/*`      | Go pprof profiling (via tsweb)     |
| `/debug/db-stats`     | Database statistics page           |
| `/debug/backup`       | Database backup download           |
| `/debug/tailsql/*`    | Interactive SQL query interface    |
| `/debug/send-command` | Serial command form (HTML)         |
| `/debug/tail`         | SSE log tail                       |

### LiDAR monitor (`internal/lidar/monitor/webserver.go`)

| Route                                       | Purpose                 |
| ------------------------------------------- | ----------------------- |
| `/debug/lidar/`                             | Main debug dashboard    |
| `/debug/lidar/sweep`                        | Sweep debug page        |
| `/debug/lidar/background/polar`             | Polar chart (ECharts)   |
| `/debug/lidar/background/heatmap`           | Heatmap chart (ECharts) |
| `/debug/lidar/background/regions`           | Regions display         |
| `/debug/lidar/background/regions/dashboard` | Regions dashboard       |
| `/debug/lidar/foreground`                   | Foreground debug        |
| `/debug/lidar/traffic`                      | Traffic debug           |
| `/debug/lidar/clusters`                     | Clusters debug          |
| `/debug/lidar/tracks`                       | Tracks debug            |

**Notes:** The LiDAR debug dashboards consume the chart API endpoints
documented in §15. The radar server debug routes are attached via
`db.AttachAdminRoutes()` → `tsweb.Debugger()`.

---

## Summary

### Counts by surface

| Category                | Total | DB  | Web | PDF | Mac |
| ----------------------- | ----- | --- | --- | --- | --- |
| HTTP endpoints (radar)  | 14    | 12  | 14  | 4   | 0   |
| HTTP endpoints (LiDAR)  | 77    | 39  | 76  | 0   | 11  |
| gRPC methods            | 9     | 0   | 0   | 0   | 9   |
| DB tables               | 22    | —   | 21  | 6   | 3   |
| Pipeline stages         | 7     | 5   | 5   | 0   | 2   |
| Tuning parameter groups | 3     | 3   | 3   | 0   | 0   |
| cmd/ entry points       | 11    | —   | —   | —   | —   |
| Debug/admin routes      | 19    | —   | —   | —   | —   |

### Gap summary

| Category                             | Count | Details                                                                                 |
| ------------------------------------ | ----- | --------------------------------------------------------------------------------------- |
| Schema columns never written         | 10    | `lidar_tracks` quality (6), `lidar_clusters` quality (3), `statistics_json` (1)         |
| Fields live-only (Mac but not in DB) | 3     | `track_length_meters`, `track_duration_secs`, `occlusion_count` (gRPC ✅, DB column 🔶) |
| Structs computed, not persisted      | 4     | NoiseCoverageMetrics, TrainingDatasetSummary, TrackFeatures, ClusterFeatures            |
| Transient pipeline metrics           | 3     | FrameMetrics, TrackAlignmentMetrics, per-track jitter                                   |
| Logic with no triggering endpoint    | 2     | `compareParams()`, `computeTemporalIoU()`                                               |
| Deprecated columns (removal planned) | 6     | p50/p85/p95 on lidar_tracks + lidar_run_tracks                                          |
