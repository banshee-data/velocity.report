```
▄▄▄      ▄▄▄   ▄▄▄▄   ▄▄▄▄▄▄▄▄▄ ▄▄▄▄▄▄▄   ▄▄▄▄▄ ▄▄▄   ▄▄▄
████▄  ▄████ ▄██▀▀██▄ ▀▀▀███▀▀▀ ███▀▀███▄  ███  ████▄████
███▀████▀███ ███  ███    ███    ███▄▄███▀  ███   ▀█████▀
███  ▀▀  ███ ███▀▀███    ███    ███▀▀██▄   ███  ▄███████▄
███      ███ ███  ███    ███    ███  ▀███ ▄███▄ ███▀ ▀███
```

# Matrix

This is the codebase's cross-reference ledger: components, API endpoints, database tables, data
structures, and pipeline stages, with the surfaces that use each one. It answers "where does this
go?" before that question has the chance to become a scavenger hunt. **DB** means SQLite
persistence, **Web** the Svelte UI on `:8080`, and **Mac** the Metal visualiser via gRPC and selected HTTP routes. Section
11 inventories the Go report pipeline separately.

- **Source:** Full-codebase consumer audit (March 2026); SQLite schema and segment/capture routes refreshed September 27, 2026
- **Inventory script:** [scripts/list-matrix-fields.py](../../scripts/list-matrix-fields.py): `--checklist` generates the LLM-consumable tracing checklist
- **Update workflow:** `/trace-matrix` skill ([.claude/skills/trace-matrix/SKILL.md](../../.claude/skills/trace-matrix/SKILL.md))
- **Related:** [SQLite model review](SCHEMA-REVIEW.md) · [Remediation plan](../../docs/plans/unpopulated-data-structures-remediation-plan.md) · [Clustering observability](../../docs/plans/lidar-clustering-observability-and-benchmark-plan.md) · [HINT metric observability](../../docs/plans/hint-metric-observability-plan.md)

---

## Legend

| Symbol | Meaning                               |
| ------ | ------------------------------------- |
| ✅     | Implemented and wired to this surface |
| 📋     | Planned: not yet implemented          |
| 🔶     | Partially wired (see notes)           |
| 🗑️     | Deprecated: to be removed             |
| -      | No direct consumer                    |
| ?      | Consumer trace pending                |

---

## 1. HTTP API endpoints: radar / main server

**Source:** [internal/cmd/server/radar.go](../../internal/cmd/server/radar.go), [internal/api/server.go](../../internal/api/server.go), [internal/api/server_admin.go](../../internal/api/server_admin.go), [internal/api/server_charts.go](../../internal/api/server_charts.go), [internal/api/server_scenes_headway.go](../../internal/api/server_scenes_headway.go), [internal/api/server_reports.go](../../internal/api/server_reports.go), [internal/api/server_sites.go](../../internal/api/server_sites.go)

| Folder                             | File                       | Endpoint                                 | DB  | Web | Mac |
| ---------------------------------- | -------------------------- | ---------------------------------------- | --- | --- | --- |
| [internal/api](../../internal/api) | `server.go`                | `GET /api/events`                        | ✅  | ✅  | -   |
| [internal/api](../../internal/api) | `server.go`                | `POST /admin/radar/command`              | -   | ✅  | -   |
| [internal/api](../../internal/api) | `server_admin.go`          | `GET /api/config`                        | -   | ✅  | -   |
| [internal/api](../../internal/api) | `server_admin.go`          | `GET /api/capabilities`                  | -   | ✅  | -   |
| [internal/api](../../internal/api) | `server_admin.go`          | `GET /api/db_stats`                      | ✅  | ✅  | -   |
| [internal/api](../../internal/api) | `server.go`                | `GET /api/radar_stats`                   | ✅  | ✅  | -   |
| [internal/api](../../internal/api) | `server.go`                | `POST /api/generate_report`              | ✅  | ✅  | -   |
| [internal/api](../../internal/api) | `server_sites.go`          | `GET/POST /api/sites`                    | ✅  | ✅  | -   |
| [internal/api](../../internal/api) | `server_sites.go`          | `GET/PUT/DEL /api/sites/{id}`            | ✅  | ✅  | -   |
| [internal/api](../../internal/api) | `server_sites.go`          | `GET/POST /api/site_config_periods`      | ✅  | ✅  | -   |
| [internal/api](../../internal/api) | `server_sites.go`          | `GET /api/timeline`                      | ✅  | ✅  | -   |
| [internal/api](../../internal/api) | `server_reports.go`        | `GET /api/reports`                       | ✅  | ✅  | -   |
| [internal/api](../../internal/api) | `server_reports.go`        | `GET /api/reports/site/{siteId}`         | ✅  | ✅  | -   |
| [internal/api](../../internal/api) | `server_reports.go`        | `GET/DELETE /api/reports/{id}`           | ✅  | ✅  | -   |
| [internal/api](../../internal/api) | `server_reports.go`        | `GET /api/reports/{id}/download/{f}`     | ✅  | ✅  | -   |
| [internal/api](../../internal/api) | `server.go`                | `GET/POST /api/transit_worker`           | ✅  | ✅  | -   |
| [internal/api](../../internal/api) | `server_charts.go`         | `GET /api/charts/timeseries`             | ✅  | ✅  | -   |
| [internal/api](../../internal/api) | `server_charts.go`         | `GET /api/charts/histogram`              | ✅  | ✅  | -   |
| [internal/api](../../internal/api) | `server_charts_headway.go` | `GET /api/charts/histogram?kind=headway` | ✅  | ✅  | -   |
| [internal/api](../../internal/api) | `server_charts.go`         | `GET /api/charts/comparison`             | ✅  | ✅  | -   |
| [internal/api](../../internal/api) | `server_scenes_headway.go` | `GET /api/scenes/{id}/headway`           | ✅  | ✅  | -   |

---

## 2. HTTP API endpoints: LiDAR server

**Source:** `internal/lidar/server/routes.go`, `track_api.go`, `run_track_api.go`, `scene_api.go`, [internal/api/lidar_labels.go](../../internal/api/lidar_labels.go)\
**Mac consumers:** `RunTrackLabelAPIClient.swift`, `LabelAPIClient.swift` (HTTP, not gRPC)

| Layer          | File                   | Endpoint                                        | DB  | Web | Mac |
| -------------- | ---------------------- | ----------------------------------------------- | --- | --- | --- |
| Status         | `routes.go`            | `GET /health`                                   | -   | ✅  | -   |
| Status         | `routes.go`            | `GET /api/lidar/server`                         | -   | -   | -   |
| Status         | `routes.go`            | `GET /api/lidar/monitor`                        | -   | ✅  | -   |
| Status         | `routes.go`            | `GET /api/lidar/status`                         | -   | ✅  | -   |
| Status         | `routes.go`            | `POST /api/lidar/persist`                       | ✅  | ✅  | -   |
| Snapshot       | `routes.go`            | `GET /api/lidar/snapshot`                       | ✅  | ✅  | -   |
| Snapshot       | `routes.go`            | `GET /api/lidar/snapshots`                      | ✅  | ✅  | -   |
| Snapshot       | `routes.go`            | `POST /api/lidar/snapshots/cleanup`             | ✅  | ✅  | -   |
| Export         | `routes.go`            | `GET /api/lidar/export_snapshot`                | ✅  | ✅  | -   |
| Export         | `routes.go`            | `GET /api/lidar/export_next_frame`              | -   | ✅  | -   |
| Export         | `routes.go`            | `GET /api/lidar/export_frame_sequence`          | -   | ✅  | -   |
| Export         | `routes.go`            | `GET /api/lidar/export_foreground`              | -   | ✅  | -   |
| Traffic        | `routes.go`            | `GET /api/lidar/traffic`                        | -   | ✅  | -   |
| Traffic        | `routes.go`            | `GET /api/lidar/acceptance`                     | -   | ✅  | -   |
| Traffic        | `routes.go`            | `POST /api/lidar/acceptance/reset`              | -   | ✅  | -   |
| Tuning         | `routes.go`            | `GET/POST /api/lidar/params`                    | ✅  | ✅  | -   |
| Sweep          | `routes.go`            | `POST /api/lidar/sweep/start`                   | ✅  | ✅  | -   |
| Sweep          | `routes.go`            | `GET /api/lidar/sweep/status`                   | ✅  | ✅  | -   |
| Sweep          | `routes.go`            | `POST /api/lidar/sweep/stop`                    | ✅  | ✅  | -   |
| Sweep          | `routes.go`            | `GET /api/lidar/sweep/explain/`                 | -   | ✅  | -   |
| Auto-tune      | `routes.go`            | `GET/POST /api/lidar/sweep/auto`                | -   | ✅  | -   |
| Auto-tune      | `routes.go`            | `POST /api/lidar/sweep/auto/stop`               | -   | ✅  | -   |
| Auto-tune      | `routes.go`            | `POST /api/lidar/sweep/auto/suspend`            | -   | ✅  | -   |
| Auto-tune      | `routes.go`            | `POST /api/lidar/sweep/auto/resume`             | -   | ✅  | -   |
| Auto-tune      | `routes.go`            | `GET /api/lidar/sweep/auto/suspended`           | -   | ✅  | -   |
| HINT           | `routes.go`            | `POST /api/lidar/sweep/hint/continue`           | ✅  | ✅  | -   |
| HINT           | `routes.go`            | `POST /api/lidar/sweep/hint/stop`               | -   | ✅  | -   |
| HINT           | `routes.go`            | `GET/POST /api/lidar/sweep/hint`                | ✅  | ✅  | -   |
| Background     | `routes.go`            | `GET /api/lidar/grid_status`                    | -   | ✅  | -   |
| Background     | `routes.go`            | `GET /api/lidar/settling_eval`                  | -   | ✅  | -   |
| Background     | `routes.go`            | `POST /api/lidar/grid_reset`                    | ✅  | ✅  | -   |
| Background     | `routes.go`            | `GET /api/lidar/grid_heatmap`                   | -   | ✅  | -   |
| Background     | `routes.go`            | `GET /api/lidar/background/grid`                | ✅  | ✅  | -   |
| PCAP           | `routes.go`            | `GET /api/lidar/data_source`                    | -   | ✅  | -   |
| PCAP           | `routes.go`            | `POST /api/lidar/pcap/start`                    | -   | ✅  | -   |
| PCAP           | `routes.go`            | `POST /api/lidar/pcap/stop`                     | -   | ✅  | -   |
| PCAP           | `routes.go`            | `POST /api/lidar/pcap/resume_live`              | -   | ✅  | -   |
| PCAP           | `routes.go`            | `GET /api/lidar/pcap/files`                     | -   | ✅  | -   |
| Playback       | `routes.go`            | `GET /api/lidar/playback/status`                | -   | -   | 🔶  |
| Playback       | `routes.go`            | `POST /api/lidar/playback/pause`                | -   | -   | -   |
| Playback       | `routes.go`            | `POST /api/lidar/playback/play`                 | -   | -   | -   |
| Playback       | `routes.go`            | `POST /api/lidar/playback/seek`                 | -   | -   | -   |
| Playback       | `routes.go`            | `POST /api/lidar/playback/rate`                 | -   | -   | -   |
| Playback       | `routes.go`            | `POST /api/lidar/vrlog/load`                    | -   | -   | ✅  |
| Playback       | `routes.go`            | `POST /api/lidar/vrlog/stop`                    | -   | -   | ✅  |
| Charts         | `routes.go`            | `GET /api/lidar/chart/polar`                    | -   | ✅  | -   |
| Charts         | `routes.go`            | `GET /api/lidar/chart/heatmap`                  | -   | ✅  | -   |
| Charts         | `routes.go`            | `GET /api/lidar/chart/foreground`               | -   | ✅  | -   |
| Charts         | `routes.go`            | `GET /api/lidar/chart/clusters`                 | -   | ✅  | -   |
| Charts         | `routes.go`            | `GET /api/lidar/chart/traffic`                  | -   | ✅  | -   |
| Tracks         | `track_api.go`         | `GET /api/lidar/tracks`                         | ✅  | ✅  | -   |
| Tracks         | `track_api.go`         | `GET /api/lidar/tracks/active`                  | ✅  | ✅  | -   |
| Tracks         | `track_api.go`         | `GET /api/lidar/tracks/{id}`                    | ✅  | ✅  | -   |
| Tracks         | `track_api.go`         | `PUT /api/lidar/tracks/{id}`                    | ✅  | -   | -   |
| Tracks         | `track_api.go`         | `GET /api/lidar/tracks/{id}/observations`       | ✅  | ✅  | -   |
| Tracks         | `track_api.go`         | `GET /api/lidar/tracks/history`                 | ✅  | ✅  | -   |
| Tracks         | `track_api.go`         | `GET /api/lidar/tracks/summary`                 | ✅  | ✅  | -   |
| Tracks         | `track_api.go`         | `GET /api/lidar/tracks/metrics`                 | -   | ✅  | -   |
| Clusters       | `track_api.go`         | `GET /api/lidar/clusters`                       | ✅  | ✅  | -   |
| Observations   | `track_api.go`         | `GET /api/lidar/observations`                   | ✅  | ✅  | -   |
| Runs           | `run_track_api.go`     | `GET /api/lidar/runs`                           | ✅  | ✅  | ✅  |
| Runs           | `run_track_api.go`     | `GET /api/lidar/runs/{id}`                      | ✅  | -   | ✅  |
| Runs           | `run_track_api.go`     | `DELETE /api/lidar/runs/{id}`                   | ✅  | ✅  | -   |
| Runs           | `run_track_api.go`     | `GET /api/lidar/runs/{id}/tracks`               | ✅  | ✅  | ✅  |
| Runs           | `run_track_api.go`     | `GET/DEL /api/lidar/runs/{id}/tracks/{tid}`     | ✅  | ✅  | ✅  |
| Runs           | `run_track_api.go`     | `PUT /api/lidar/runs/{id}/tracks/{tid}/label`   | ✅  | ✅  | ✅  |
| Runs           | `run_track_api.go`     | `PUT /api/lidar/runs/{id}/tracks/{tid}/flags`   | ✅  | ✅  | -   |
| Runs           | `run_track_api.go`     | `GET /api/lidar/runs/{id}/labelling-progress`   | ✅  | ✅  | ✅  |
| Runs           | `run_track_api.go`     | `POST /api/lidar/runs/{id}/reprocess`           | ✅  | -   | -   |
| Runs           | `run_track_api.go`     | `POST /api/lidar/runs/{id}/evaluate`            | ✅  | -   | -   |
| Labels         | `lidar_labels.go`      | `GET/POST /api/lidar/labels`                    | ✅  | ✅  | ✅  |
| Labels         | `lidar_labels.go`      | `GET/PUT/DEL /api/lidar/labels/{id}`            | ✅  | ✅  | ✅  |
| Labels         | `lidar_labels.go`      | `GET /api/lidar/labels/export`                  | ✅  | ✅  | ✅  |
| Scenes         | `scene_api.go`         | `GET/POST /api/lidar/scenes`                    | ✅  | ✅  | -   |
| Scenes         | `scene_api.go`         | `GET/PUT/DEL /api/lidar/scenes/{id}`            | ✅  | ✅  | -   |
| Scenes         | `scene_api.go`         | `POST /api/lidar/scenes/{id}/replay`            | ✅  | -   | -   |
| Scenes         | `scene_api.go`         | `GET/POST /api/lidar/scenes/{id}/evaluations`   | ✅  | -   | -   |
| Missed regions | `run_track_api.go`     | `GET/POST /api/lidar/runs/{id}/missed-regions`  | ✅  | ✅  | -   |
| Missed regions | `run_track_api.go`     | `DEL /api/lidar/runs/{id}/missed-regions/{rid}` | ✅  | ✅  | -   |
| Sweep history  | `routes.go`            | `GET /api/lidar/sweeps`                         | ✅  | ✅  | -   |
| Sweep history  | `routes.go`            | `GET /api/lidar/sweeps/{id}`                    | ✅  | ✅  | -   |
| Sweep history  | `routes.go`            | `PUT /api/lidar/sweeps/charts`                  | ✅  | ✅  | -   |
| Destructive    | `track_api.go`         | `POST /api/lidar/tracks/clear`                  | ✅  | ✅  | -   |
| Destructive    | `routes.go`            | `POST /api/lidar/runs/clear`                    | ✅  | ✅  | -   |
| Capture        | `capture_api.go`       | `GET /api/lidar/capture/roots`                  | ✅  | ✅  | -   |
| Capture        | `capture_api.go`       | `POST /api/lidar/capture/scan`                  | ✅  | ✅  | -   |
| Capture        | `capture_api.go`       | `GET /api/lidar/capture/sessions`               | ✅  | ✅  | -   |
| Capture        | `capture_api.go`       | `GET /api/lidar/capture/files`                  | ✅  | ✅  | -   |
| Capture        | `capture_api.go`       | `POST /api/lidar/capture/session/label`         | ✅  | ✅  | -   |
| Capture        | `capture_api.go`       | `POST /api/lidar/capture/motion-pass`           | ✅  | ✅  | -   |
| Capture        | `capture_api.go`       | `GET /api/lidar/capture/periods`                | ✅  | ✅  | -   |
| Capture        | `capture_api.go`       | `GET /api/lidar/capture/jobs`                   | ✅  | ✅  | -   |
| Capture        | `capture_api.go`       | `POST /api/lidar/capture/jobs/cancel`           | ✅  | ✅  | -   |
| Scene map      | `scene_geo_api.go`     | `GET /api/lidar/scene-map`                      | ✅  | ✅  | -   |
| Segments       | `segments_api.go`      | `GET /api/lidar/segments/finders`               | -   | ✅  | -   |
| Segments       | `segments_api.go`      | `GET /api/lidar/segments`                       | ✅  | ✅  | -   |
| Segments       | `segments_api.go`      | `GET /api/lidar/segments/strip`                 | ✅  | ✅  | -   |
| Segments       | `segments_api.go`      | `POST /api/lidar/segments/{id}/case`            | ✅  | ✅  | -   |
| Segments       | `segment_clip_api.go`  | `POST /api/lidar/scenes/{id}/clip`              | ✅  | ✅  | -   |
| Annotations    | `segment_packs_api.go` | `GET /api/annotations/packs`                    | -   | ✅  | -   |

**Playback row notes.** No web code calls any `/api/lidar/playback/*` or
`/api/lidar/vrlog/*` endpoint; those rows previously carried Web ✅ marks that
nothing backed. `GET /api/lidar/playback/status` is 🔶 on Mac: the Swift client
defines `RunTrackLabelAPIClient.getPlaybackStatus()` but no app code path calls
it. The visualiser learns playback state from `FrameBundle.playback_info` on the
gRPC stream instead. `POST /api/lidar/vrlog/load|stop` are genuinely Mac-wired
via `RunBrowserState`.

---

## 3. gRPC service: macOS visualiser

**Source:** [proto/velocity_visualiser/v1/visualiser.proto](../../proto/velocity_visualiser/v1/visualiser.proto)

| Layer     | File               | Method                            | DB  | Web | Mac |
| --------- | ------------------ | --------------------------------- | --- | --- | --- |
| Streaming | `visualiser.proto` | `StreamFrames` (server streaming) | -   | -   | ✅  |
| Playback  | `visualiser.proto` | `Pause`                           | -   | -   | ✅  |
| Playback  | `visualiser.proto` | `Play`                            | -   | -   | ✅  |
| Playback  | `visualiser.proto` | `Seek`                            | -   | -   | ✅  |
| Playback  | `visualiser.proto` | `SetRate`                         | -   | -   | ✅  |
| Debug     | `visualiser.proto` | `SetOverlayModes`                 | -   | -   | ✅  |
| Debug     | `visualiser.proto` | `GetCapabilities`                 | -   | -   | ✅  |
| Recording | `visualiser.proto` | `StartRecording`                  | -   | -   | ✅  |
| Recording | `visualiser.proto` | `StopRecording`                   | -   | -   | ✅  |

---

## 4. Database tables

**Source:** [internal/db/schema.sql](../../internal/db/schema.sql). All current tables are inventoried; `?` records a consumer trace still to do.

| Layer  | Table                            | Web | Mac | Notes                                                  |
| ------ | -------------------------------- | --- | --- | ------------------------------------------------------ |
| LiDAR  | `lidar_param_sets`               | ✅  | -   | Immutable requested/effective/legacy parameter assets  |
| LiDAR  | `lidar_run_configs`              | ✅  | -   | Immutable executed config assets shown in run metadata |
| LiDAR  | `lidar_run_records`              | ✅  | ✅  | Run browser + label UI                                 |
| LiDAR  | `lidar_clusters`                 | ✅  | ✅  | Cluster display, gRPC                                  |
| LiDAR  | `lidar_tracks`                   | ✅  | ✅  | Track display, gRPC                                    |
| LiDAR  | `lidar_track_observations`       | ✅  | -   | Trajectory rendering                                   |
| LiDAR  | `lidar_replay_annotations`       | ✅  | ✅  | Replay-case labels and Mac labelling                   |
| LiDAR  | `lidar_replay_cases`             | ✅  | ✅  | Replay-case browser and Mac labelling                  |
| LiDAR  | `lidar_replay_evaluations`       | ✅  | -   | Replay evaluation and compare UI                       |
| LiDAR  | `lidar_tuning_sweeps`            | ✅  | -   | Sweep history                                          |
| LiDAR  | `lidar_bg_snapshot`              | ✅  | 🔶  | Grid visualisation (derived sent via gRPC)             |
| LiDAR  | `lidar_bg_regions`               | ✅  | -   | Settling evaluation                                    |
| LiDAR  | `lidar_run_missed_regions`       | ✅  | -   | Detection gap annotations                              |
| LiDAR  | `lidar_run_tracks`               | ✅  | ✅  | Per-run track copies and Mac run browser               |
| LiDAR  | `lidar_capture_files`            | ✅  | ?   | Captures page; Mac trace pending                       |
| LiDAR  | `lidar_capture_jobs`             | ✅  | ?   | Capture and clip worker queue                          |
| LiDAR  | `lidar_capture_motion_periods`   | ✅  | ?   | Captures page; Mac trace pending                       |
| LiDAR  | `lidar_capture_roots`            | ✅  | ?   | Captures page; Mac trace pending                       |
| LiDAR  | `lidar_capture_sessions`         | ✅  | ?   | Captures page; Mac trace pending                       |
| LiDAR  | `lidar_exposure_windows`         | ?   | ?   | Exposure evidence windows                              |
| LiDAR  | `lidar_interaction_events`       | ?   | ?   | Following interaction events                           |
| LiDAR  | `lidar_interaction_instants`     | ?   | ?   | Interaction time samples                               |
| LiDAR  | `lidar_migration_rejects`        | -   | -   | Rows a migration set aside, kept whole with the reason |
| LiDAR  | `lidar_observations`             | ?   | ?   | Typed observation evidence                             |
| LiDAR  | `lidar_replay_case_files`        | ✅  | ?   | Ordered source files for replay cases                  |
| LiDAR  | `lidar_scenes`                   | ✅  | ?   | Scene player; Mac trace pending                        |
| LiDAR  | `lidar_segment_clip_jobs`        | ✅  | -   | Clip job, its segment, and the pack it made            |
| LiDAR  | `lidar_segment_selections`       | ✅  | -   | Chosen window, its replay case and what chose it       |
| LiDAR  | `lidar_sites`                    | ✅  | ?   | Scene map; Mac trace pending                           |
| LiDAR  | `lidar_track_estimate_revisions` | ?   | ?   | Smoother revision evidence                             |
| LiDAR  | `lidar_track_estimates`          | ?   | ?   | Track state estimates                                  |
| LiDAR  | `lidar_track_residuals`          | ?   | ?   | Observation residual evidence                          |
| LiDAR  | `lidar_track_solid_bodies`       | ?   | ?   | Estimated solid bodies                                 |
| Radar  | `radar_serial_config`            | ✅  | ?   | Serial configuration UI; Mac trace pending             |
| Radar  | `radar_data`                     | ✅  | -   | Raw events + alt stats source                          |
| Radar  | `radar_objects`                  | ✅  | -   | Primary report source                                  |
| Radar  | `radar_data_transits`            | ✅  | -   | Alternative report source                              |
| Radar  | `radar_transit_links`            | ✅  | -   | Transit chain building                                 |
| Radar  | `radar_commands`                 | ✅  | -   | Debug history                                          |
| Radar  | `radar_command_log`              | ✅  | -   | Debug output                                           |
| Site   | `site`                           | ✅  | -   | Location, metadata                                     |
| Site   | `site_config_periods`            | ✅  | -   | Mounting angle, speed limit                            |
| Site   | `site_reports`                   | ✅  | -   | Report metadata + download                             |
| System | `schema_migrations`              | -   | -   | Internal                                               |

---

## 5. Database fields: all columns

The current schema has 607 fields, including generated fields. Newly added rows use `?` where consumer tracing remains open. DB ✅ confirms the schema field; the data model review identifies write and constraint risks.

| Table                            | Column                              | Type          | DB  | Web | Mac |
| -------------------------------- | ----------------------------------- | ------------- | --- | --- | --- |
| `lidar_param_sets`               | `param_set_id`                      | TEXT PK       | ✅  | ✅  | -   |
| `lidar_param_sets`               | `params_hash`                       | TEXT          | ✅  | ✅  | -   |
| `lidar_param_sets`               | `schema_version`                    | TEXT          | ✅  | ✅  | -   |
| `lidar_param_sets`               | `param_set_type`                    | TEXT          | ✅  | 🔶  | -   |
| `lidar_param_sets`               | `params_json`                       | TEXT          | ✅  | -   | -   |
| `lidar_param_sets`               | `created_at`                        | INTEGER       | ✅  | -   | -   |
| `lidar_run_configs`              | `run_config_id`                     | TEXT PK       | ✅  | ✅  | -   |
| `lidar_run_configs`              | `config_hash`                       | TEXT          | ✅  | ✅  | -   |
| `lidar_run_configs`              | `param_set_id`                      | TEXT FK       | ✅  | ✅  | -   |
| `lidar_run_configs`              | `build_version`                     | TEXT          | ✅  | ✅  | -   |
| `lidar_run_configs`              | `build_git_sha`                     | TEXT          | ✅  | ✅  | -   |
| `lidar_run_configs`              | `created_at`                        | INTEGER       | ✅  | -   | -   |
| `lidar_run_records`              | `run_id`                            | TEXT PK       | ✅  | ✅  | -   |
| `lidar_run_records`              | `created_at`                        | INTEGER       | ✅  | ✅  | -   |
| `lidar_run_records`              | `source_type`                       | TEXT          | ✅  | ✅  | -   |
| `lidar_run_records`              | `source_path`                       | TEXT          | ✅  | ✅  | -   |
| `lidar_run_records`              | `sensor_id`                         | TEXT          | ✅  | ✅  | -   |
| `lidar_run_records`              | `duration_secs`                     | REAL          | ✅  | ✅  | -   |
| `lidar_run_records`              | `total_frames`                      | INTEGER       | ✅  | ✅  | -   |
| `lidar_run_records`              | `total_clusters`                    | INTEGER       | ✅  | ✅  | -   |
| `lidar_run_records`              | `total_tracks`                      | INTEGER       | ✅  | ✅  | -   |
| `lidar_run_records`              | `confirmed_tracks`                  | INTEGER       | ✅  | ✅  | -   |
| `lidar_run_records`              | `processing_time_ms`                | INTEGER       | ✅  | ✅  | -   |
| `lidar_run_records`              | `status`                            | TEXT          | ✅  | ✅  | -   |
| `lidar_run_records`              | `error_message`                     | TEXT          | ✅  | ✅  | -   |
| `lidar_run_records`              | `parent_run_id`                     | TEXT FK       | ✅  | ✅  | -   |
| `lidar_run_records`              | `notes`                             | TEXT          | ✅  | ✅  | -   |
| `lidar_run_records`              | `statistics_json`                   | TEXT          | 🔶  | 📋  | -   |
| `lidar_run_records`              | `vrlog_path`                        | TEXT          | ✅  | ✅  | -   |
| `lidar_run_records`              | `run_config_id`                     | TEXT FK       | ✅  | ✅  | -   |
| `lidar_run_records`              | `requested_param_set_id`            | TEXT FK       | ✅  | ✅  | -   |
| `lidar_run_records`              | `replay_case_id`                    | TEXT FK       | ✅  | ✅  | -   |
| `lidar_run_records`              | `completed_at`                      | INTEGER       | ✅  | ✅  | -   |
| `lidar_run_records`              | `frame_start_ns`                    | INTEGER       | ✅  | ✅  | -   |
| `lidar_run_records`              | `frame_end_ns`                      | INTEGER       | ✅  | ✅  | -   |
| `lidar_clusters`                 | `lidar_cluster_id`                  | INTEGER PK    | ✅  | ✅  | ✅  |
| `lidar_clusters`                 | `sensor_id`                         | TEXT          | ✅  | ✅  | ✅  |
| `lidar_clusters`                 | `frame_id`                          | TEXT          | ✅  | -   | -   |
| `lidar_clusters`                 | `ts_unix_nanos`                     | INTEGER       | ✅  | ✅  | ✅  |
| `lidar_clusters`                 | `centroid_x`                        | REAL          | ✅  | ✅  | ✅  |
| `lidar_clusters`                 | `centroid_y`                        | REAL          | ✅  | ✅  | ✅  |
| `lidar_clusters`                 | `centroid_z`                        | REAL          | ✅  | ✅  | ✅  |
| `lidar_clusters`                 | `bounding_box_length`               | REAL          | ✅  | ✅  | ✅  |
| `lidar_clusters`                 | `bounding_box_width`                | REAL          | ✅  | ✅  | ✅  |
| `lidar_clusters`                 | `bounding_box_height`               | REAL          | ✅  | ✅  | ✅  |
| `lidar_clusters`                 | `points_count`                      | INTEGER       | ✅  | ✅  | ✅  |
| `lidar_clusters`                 | `height_p95`                        | REAL          | ✅  | ✅  | ✅  |
| `lidar_clusters`                 | `intensity_mean`                    | REAL          | ✅  | ✅  | ✅  |
| `lidar_clusters`                 | `noise_points_count`                | INTEGER       | 🔶  | -   | -   |
| `lidar_clusters`                 | `cluster_density`                   | REAL          | 🔶  | 📋  | -   |
| `lidar_clusters`                 | `aspect_ratio`                      | REAL          | 🔶  | 📋  | -   |
| `lidar_tracks`                   | `track_id`                          | TEXT PK       | ✅  | ✅  | ✅  |
| `lidar_tracks`                   | `sensor_id`                         | TEXT          | ✅  | ✅  | ✅  |
| `lidar_tracks`                   | `frame_id`                          | TEXT          | ✅  | -   | -   |
| `lidar_tracks`                   | `track_state`                       | TEXT          | ✅  | ✅  | ✅  |
| `lidar_tracks`                   | `start_unix_nanos`                  | INTEGER       | ✅  | ✅  | ✅  |
| `lidar_tracks`                   | `end_unix_nanos`                    | INTEGER       | ✅  | ✅  | ✅  |
| `lidar_tracks`                   | `observation_count`                 | INTEGER       | ✅  | ✅  | ✅  |
| `lidar_tracks`                   | `avg_speed_mps`                     | REAL          | ✅  | ✅  | ✅  |
| `lidar_tracks`                   | `max_speed_mps`                     | REAL          | ✅  | ✅  | ✅  |
| `lidar_tracks`                   | `bounding_box_length_avg`           | REAL          | ✅  | ✅  | ✅  |
| `lidar_tracks`                   | `bounding_box_width_avg`            | REAL          | ✅  | ✅  | ✅  |
| `lidar_tracks`                   | `bounding_box_height_avg`           | REAL          | ✅  | ✅  | ✅  |
| `lidar_tracks`                   | `height_p95_max`                    | REAL          | ✅  | ✅  | ✅  |
| `lidar_tracks`                   | `intensity_mean_avg`                | REAL          | ✅  | ✅  | ✅  |
| `lidar_tracks`                   | `object_class`                      | TEXT          | ✅  | ✅  | ✅  |
| `lidar_tracks`                   | `object_confidence`                 | REAL          | ✅  | ✅  | ✅  |
| `lidar_tracks`                   | `classification_model`              | TEXT          | ✅  | ✅  | -   |
| `lidar_tracks`                   | `track_length_meters`               | REAL          | 🔶  | 📋  | ✅  |
| `lidar_tracks`                   | `track_duration_secs`               | REAL          | 🔶  | 📋  | ✅  |
| `lidar_tracks`                   | `occlusion_count`                   | INTEGER       | 🔶  | 📋  | ✅  |
| `lidar_tracks`                   | `max_occlusion_frames`              | INTEGER       | 🔶  | 📋  | -   |
| `lidar_tracks`                   | `spatial_coverage`                  | REAL          | 🔶  | 📋  | -   |
| `lidar_tracks`                   | `noise_point_ratio`                 | REAL          | 🔶  | 📋  | -   |
| `lidar_track_observations`       | `track_id`                          | TEXT PK       | ✅  | ✅  | -   |
| `lidar_track_observations`       | `ts_unix_nanos`                     | INTEGER PK    | ✅  | ✅  | -   |
| `lidar_track_observations`       | `frame_id`                          | TEXT          | ✅  | -   | -   |
| `lidar_track_observations`       | `x`                                 | REAL          | ✅  | ✅  | -   |
| `lidar_track_observations`       | `y`                                 | REAL          | ✅  | ✅  | -   |
| `lidar_track_observations`       | `z`                                 | REAL          | ✅  | ✅  | -   |
| `lidar_track_observations`       | `velocity_x`                        | REAL          | ✅  | ✅  | -   |
| `lidar_track_observations`       | `velocity_y`                        | REAL          | ✅  | ✅  | -   |
| `lidar_track_observations`       | `speed_mps`                         | REAL          | ✅  | ✅  | -   |
| `lidar_track_observations`       | `heading_rad`                       | REAL          | ✅  | ✅  | -   |
| `lidar_track_observations`       | `bounding_box_length`               | REAL          | ✅  | ✅  | -   |
| `lidar_track_observations`       | `bounding_box_width`                | REAL          | ✅  | ✅  | -   |
| `lidar_track_observations`       | `bounding_box_height`               | REAL          | ✅  | ✅  | -   |
| `lidar_track_observations`       | `height_p95`                        | REAL          | ✅  | ✅  | -   |
| `lidar_track_observations`       | `intensity_mean`                    | REAL          | ✅  | ✅  | -   |
| `lidar_run_tracks`               | `run_id`                            | TEXT PK       | ✅  | ✅  | -   |
| `lidar_run_tracks`               | `track_id`                          | TEXT PK       | ✅  | ✅  | -   |
| `lidar_run_tracks`               | `sensor_id`                         | TEXT          | ✅  | ✅  | -   |
| `lidar_run_tracks`               | `track_state`                       | TEXT          | ✅  | ✅  | -   |
| `lidar_run_tracks`               | `start_unix_nanos`                  | INTEGER       | ✅  | ✅  | -   |
| `lidar_run_tracks`               | `end_unix_nanos`                    | INTEGER       | ✅  | ✅  | -   |
| `lidar_run_tracks`               | `observation_count`                 | INTEGER       | ✅  | ✅  | -   |
| `lidar_run_tracks`               | `avg_speed_mps`                     | REAL          | ✅  | ✅  | -   |
| `lidar_run_tracks`               | `max_speed_mps`                     | REAL          | ✅  | ✅  | -   |
| `lidar_run_tracks`               | `bounding_box_length_avg`           | REAL          | ✅  | ✅  | -   |
| `lidar_run_tracks`               | `bounding_box_width_avg`            | REAL          | ✅  | ✅  | -   |
| `lidar_run_tracks`               | `bounding_box_height_avg`           | REAL          | ✅  | ✅  | -   |
| `lidar_run_tracks`               | `height_p95_max`                    | REAL          | ✅  | ✅  | -   |
| `lidar_run_tracks`               | `intensity_mean_avg`                | REAL          | ✅  | ✅  | -   |
| `lidar_run_tracks`               | `object_class`                      | TEXT          | ✅  | ✅  | -   |
| `lidar_run_tracks`               | `object_confidence`                 | REAL          | ✅  | ✅  | -   |
| `lidar_run_tracks`               | `classification_model`              | TEXT          | ✅  | ✅  | -   |
| `lidar_run_tracks`               | `user_label`                        | TEXT          | ✅  | ✅  | -   |
| `lidar_run_tracks`               | `label_confidence`                  | REAL          | ✅  | ✅  | -   |
| `lidar_run_tracks`               | `labeler_id`                        | TEXT          | ✅  | ✅  | -   |
| `lidar_run_tracks`               | `labeled_at`                        | INTEGER       | ✅  | ✅  | -   |
| `lidar_run_tracks`               | `is_split_candidate`                | INTEGER       | ✅  | ✅  | -   |
| `lidar_run_tracks`               | `is_merge_candidate`                | INTEGER       | ✅  | ✅  | -   |
| `lidar_run_tracks`               | `linked_track_ids`                  | TEXT          | ✅  | ✅  | -   |
| `lidar_run_tracks`               | `quality_label`                     | TEXT          | ✅  | ✅  | -   |
| `lidar_run_tracks`               | `label_source`                      | TEXT          | ✅  | ✅  | -   |
| `lidar_replay_annotations`       | `annotation_id`                     | TEXT PK       | ✅  | ✅  | -   |
| `lidar_replay_annotations`       | `replay_case_id`                    | TEXT FK       | ✅  | ✅  | -   |
| `lidar_replay_annotations`       | `run_id`                            | TEXT FK       | ✅  | ✅  | -   |
| `lidar_replay_annotations`       | `track_id`                          | TEXT FK       | ✅  | ✅  | -   |
| `lidar_replay_annotations`       | `class_label`                       | TEXT          | ✅  | ✅  | -   |
| `lidar_replay_annotations`       | `start_timestamp_ns`                | INTEGER       | ✅  | ✅  | -   |
| `lidar_replay_annotations`       | `end_timestamp_ns`                  | INTEGER       | ✅  | ✅  | -   |
| `lidar_replay_annotations`       | `confidence`                        | REAL          | ✅  | ✅  | -   |
| `lidar_replay_annotations`       | `created_by`                        | TEXT          | ✅  | ✅  | -   |
| `lidar_replay_annotations`       | `created_at_ns`                     | INTEGER       | ✅  | ✅  | -   |
| `lidar_replay_annotations`       | `updated_at_ns`                     | INTEGER       | ✅  | ✅  | -   |
| `lidar_replay_annotations`       | `notes`                             | TEXT          | ✅  | ✅  | -   |
| `lidar_replay_annotations`       | `source_file`                       | TEXT          | ✅  | ✅  | -   |
| `lidar_replay_cases`             | `replay_case_id`                    | TEXT PK       | ✅  | ✅  | -   |
| `lidar_replay_cases`             | `sensor_id`                         | TEXT          | ✅  | ✅  | -   |
| `lidar_replay_cases`             | `pcap_file`                         | TEXT          | ✅  | ✅  | -   |
| `lidar_replay_cases`             | `pcap_start_secs`                   | REAL          | ✅  | ✅  | -   |
| `lidar_replay_cases`             | `pcap_duration_secs`                | REAL          | ✅  | ✅  | -   |
| `lidar_replay_cases`             | `description`                       | TEXT          | ✅  | ✅  | -   |
| `lidar_replay_cases`             | `reference_run_id`                  | TEXT FK       | ✅  | ✅  | -   |
| `lidar_replay_cases`             | `created_at_ns`                     | INTEGER       | ✅  | ✅  | -   |
| `lidar_replay_cases`             | `updated_at_ns`                     | INTEGER       | ✅  | ✅  | -   |
| `lidar_replay_cases`             | `recommended_param_set_id`          | TEXT FK       | ✅  | ✅  | -   |
| `lidar_replay_evaluations`       | `evaluation_id`                     | TEXT PK       | ✅  | ✅  | -   |
| `lidar_replay_evaluations`       | `replay_case_id`                    | TEXT FK       | ✅  | ✅  | -   |
| `lidar_replay_evaluations`       | `reference_run_id`                  | TEXT FK       | ✅  | ✅  | -   |
| `lidar_replay_evaluations`       | `candidate_run_id`                  | TEXT FK       | ✅  | ✅  | -   |
| `lidar_replay_evaluations`       | `detection_rate`                    | REAL          | ✅  | ✅  | -   |
| `lidar_replay_evaluations`       | `fragmentation`                     | REAL          | ✅  | ✅  | -   |
| `lidar_replay_evaluations`       | `false_positive_rate`               | REAL          | ✅  | ✅  | -   |
| `lidar_replay_evaluations`       | `velocity_coverage`                 | REAL          | ✅  | ✅  | -   |
| `lidar_replay_evaluations`       | `quality_premium`                   | REAL          | ✅  | ✅  | -   |
| `lidar_replay_evaluations`       | `truncation_rate`                   | REAL          | ✅  | ✅  | -   |
| `lidar_replay_evaluations`       | `velocity_noise_rate`               | REAL          | ✅  | ✅  | -   |
| `lidar_replay_evaluations`       | `stopped_recovery_rate`             | REAL          | ✅  | ✅  | -   |
| `lidar_replay_evaluations`       | `composite_score`                   | REAL          | ✅  | ✅  | -   |
| `lidar_replay_evaluations`       | `matched_count`                     | INTEGER       | ✅  | ✅  | -   |
| `lidar_replay_evaluations`       | `reference_count`                   | INTEGER       | ✅  | ✅  | -   |
| `lidar_replay_evaluations`       | `candidate_count`                   | INTEGER       | ✅  | ✅  | -   |
| `lidar_replay_evaluations`       | `created_at`                        | INTEGER       | ✅  | ✅  | -   |
| `lidar_tuning_sweeps`            | `id`                                | INTEGER PK    | ✅  | -   | -   |
| `lidar_tuning_sweeps`            | `sweep_id`                          | TEXT UNIQUE   | ✅  | ✅  | -   |
| `lidar_tuning_sweeps`            | `sensor_id`                         | TEXT          | ✅  | ✅  | -   |
| `lidar_tuning_sweeps`            | `mode`                              | TEXT          | ✅  | ✅  | -   |
| `lidar_tuning_sweeps`            | `status`                            | TEXT          | ✅  | ✅  | -   |
| `lidar_tuning_sweeps`            | `request`                           | TEXT          | ✅  | ✅  | -   |
| `lidar_tuning_sweeps`            | `results`                           | TEXT          | ✅  | ✅  | -   |
| `lidar_tuning_sweeps`            | `charts`                            | TEXT          | ✅  | ✅  | -   |
| `lidar_tuning_sweeps`            | `recommendation`                    | TEXT          | ✅  | ✅  | -   |
| `lidar_tuning_sweeps`            | `round_results`                     | TEXT          | ✅  | ✅  | -   |
| `lidar_tuning_sweeps`            | `error`                             | TEXT          | ✅  | ✅  | -   |
| `lidar_tuning_sweeps`            | `started_at`                        | DATETIME      | ✅  | ✅  | -   |
| `lidar_tuning_sweeps`            | `completed_at`                      | DATETIME      | ✅  | ✅  | -   |
| `lidar_tuning_sweeps`            | `created_at`                        | DATETIME      | ✅  | -   | -   |
| `lidar_tuning_sweeps`            | `objective_name`                    | TEXT          | ✅  | ✅  | -   |
| `lidar_tuning_sweeps`            | `objective_version`                 | TEXT          | ✅  | ✅  | -   |
| `lidar_tuning_sweeps`            | `transform_pipeline_name`           | TEXT          | ✅  | ✅  | -   |
| `lidar_tuning_sweeps`            | `transform_pipeline_version`        | TEXT          | ✅  | ✅  | -   |
| `lidar_tuning_sweeps`            | `score_components_json`             | TEXT          | ✅  | ✅  | -   |
| `lidar_tuning_sweeps`            | `recommendation_explanation_json`   | TEXT          | ✅  | ✅  | -   |
| `lidar_tuning_sweeps`            | `label_provenance_summary_json`     | TEXT          | ✅  | ✅  | -   |
| `lidar_tuning_sweeps`            | `checkpoint_round`                  | INTEGER       | ✅  | ✅  | -   |
| `lidar_tuning_sweeps`            | `checkpoint_bounds`                 | TEXT          | ✅  | ✅  | -   |
| `lidar_tuning_sweeps`            | `checkpoint_results`                | TEXT          | ✅  | ✅  | -   |
| `lidar_tuning_sweeps`            | `checkpoint_request`                | TEXT          | ✅  | ✅  | -   |
| `lidar_bg_snapshot`              | `snapshot_id`                       | INTEGER PK    | ✅  | ✅  | -   |
| `lidar_bg_snapshot`              | `sensor_id`                         | TEXT          | ✅  | ✅  | -   |
| `lidar_bg_snapshot`              | `taken_unix_nanos`                  | INTEGER       | ✅  | ✅  | -   |
| `lidar_bg_snapshot`              | `rings`                             | INTEGER       | ✅  | ✅  | -   |
| `lidar_bg_snapshot`              | `azimuth_bins`                      | INTEGER       | ✅  | ✅  | -   |
| `lidar_bg_snapshot`              | `params_json`                       | TEXT          | ✅  | ✅  | -   |
| `lidar_bg_snapshot`              | `ring_elevations_json`              | TEXT          | ✅  | ✅  | -   |
| `lidar_bg_snapshot`              | `grid_blob`                         | BLOB          | ✅  | ✅  | -   |
| `lidar_bg_snapshot`              | `changed_cells_count`               | INTEGER       | ✅  | ✅  | -   |
| `lidar_bg_snapshot`              | `snapshot_reason`                   | TEXT          | ✅  | ✅  | -   |
| `lidar_bg_regions`               | `region_set_id`                     | INTEGER PK    | ✅  | ✅  | -   |
| `lidar_bg_regions`               | `snapshot_id`                       | INTEGER FK    | ✅  | ✅  | -   |
| `lidar_bg_regions`               | `sensor_id`                         | TEXT          | ✅  | ✅  | -   |
| `lidar_bg_regions`               | `created_unix_nanos`                | INTEGER       | ✅  | ✅  | -   |
| `lidar_bg_regions`               | `region_count`                      | INTEGER       | ✅  | ✅  | -   |
| `lidar_bg_regions`               | `regions_json`                      | TEXT          | ✅  | ✅  | -   |
| `lidar_bg_regions`               | `variance_data_json`                | TEXT          | ✅  | ✅  | -   |
| `lidar_bg_regions`               | `settling_frames`                   | INTEGER       | ✅  | ✅  | -   |
| `lidar_bg_regions`               | `grid_hash`                         | TEXT          | ✅  | -   | -   |
| `lidar_bg_regions`               | `source_path`                       | TEXT          | ✅  | -   | -   |
| `lidar_run_missed_regions`       | `region_id`                         | TEXT PK       | ✅  | ✅  | -   |
| `lidar_run_missed_regions`       | `run_id`                            | TEXT FK       | ✅  | ✅  | -   |
| `lidar_run_missed_regions`       | `center_x`                          | REAL          | ✅  | ✅  | -   |
| `lidar_run_missed_regions`       | `center_y`                          | REAL          | ✅  | ✅  | -   |
| `lidar_run_missed_regions`       | `radius_m`                          | REAL          | ✅  | ✅  | -   |
| `lidar_run_missed_regions`       | `time_start_ns`                     | INTEGER       | ✅  | ✅  | -   |
| `lidar_run_missed_regions`       | `time_end_ns`                       | INTEGER       | ✅  | ✅  | -   |
| `lidar_run_missed_regions`       | `expected_label`                    | TEXT          | ✅  | ✅  | -   |
| `lidar_run_missed_regions`       | `labeler_id`                        | TEXT          | ✅  | ✅  | -   |
| `lidar_run_missed_regions`       | `labeled_at`                        | INTEGER       | ✅  | ✅  | -   |
| `lidar_run_missed_regions`       | `notes`                             | TEXT          | ✅  | ✅  | -   |
| `radar_data`                     | `data_id`                           | INTEGER PK    | ✅  | -   | -   |
| `radar_data`                     | `write_timestamp`                   | DOUBLE        | ✅  | -   | -   |
| `radar_data`                     | `raw_event`                         | JSON          | ✅  | -   | -   |
| `radar_data`                     | `uptime`                            | DOUBLE STORED | ✅  | ✅  | -   |
| `radar_data`                     | `magnitude`                         | DOUBLE STORED | ✅  | ✅  | -   |
| `radar_data`                     | `speed`                             | DOUBLE STORED | ✅  | ✅  | -   |
| `radar_objects`                  | `write_timestamp`                   | DOUBLE        | ✅  | ✅  | -   |
| `radar_objects`                  | `raw_event`                         | JSON          | ✅  | -   | -   |
| `radar_objects`                  | `classifier`                        | TEXT STORED   | ✅  | ✅  | -   |
| `radar_objects`                  | `start_time`                        | DOUBLE STORED | ✅  | -   | -   |
| `radar_objects`                  | `end_time`                          | DOUBLE STORED | ✅  | -   | -   |
| `radar_objects`                  | `delta_time_ms`                     | BIGINT STORED | ✅  | -   | -   |
| `radar_objects`                  | `max_speed`                         | DOUBLE STORED | ✅  | ✅  | -   |
| `radar_objects`                  | `min_speed`                         | DOUBLE STORED | ✅  | -   | -   |
| `radar_objects`                  | `speed_change`                      | DOUBLE STORED | ✅  | -   | -   |
| `radar_objects`                  | `max_magnitude`                     | BIGINT STORED | ✅  | -   | -   |
| `radar_objects`                  | `avg_magnitude`                     | BIGINT STORED | ✅  | -   | -   |
| `radar_objects`                  | `total_frames`                      | BIGINT STORED | ✅  | -   | -   |
| `radar_objects`                  | `frames_per_mps`                    | DOUBLE STORED | ✅  | -   | -   |
| `radar_objects`                  | `length_m`                          | DOUBLE STORED | ✅  | -   | -   |
| `radar_data_transits`            | `transit_id`                        | INTEGER PK    | ✅  | -   | -   |
| `radar_data_transits`            | `transit_key`                       | TEXT UNIQUE   | ✅  | -   | -   |
| `radar_data_transits`            | `threshold_ms`                      | INTEGER       | ✅  | -   | -   |
| `radar_data_transits`            | `transit_start_unix`                | DOUBLE        | ✅  | ✅  | -   |
| `radar_data_transits`            | `transit_end_unix`                  | DOUBLE        | ✅  | -   | -   |
| `radar_data_transits`            | `transit_max_speed`                 | DOUBLE        | ✅  | ✅  | -   |
| `radar_data_transits`            | `transit_min_speed`                 | DOUBLE        | ✅  | -   | -   |
| `radar_data_transits`            | `transit_max_magnitude`             | BIGINT        | ✅  | -   | -   |
| `radar_data_transits`            | `transit_min_magnitude`             | BIGINT        | ✅  | -   | -   |
| `radar_data_transits`            | `point_count`                       | INTEGER       | ✅  | -   | -   |
| `radar_data_transits`            | `model_version`                     | TEXT          | ✅  | ✅  | -   |
| `radar_data_transits`            | `created_at`                        | DOUBLE        | ✅  | -   | -   |
| `radar_data_transits`            | `updated_at`                        | DOUBLE        | ✅  | -   | -   |
| `radar_transit_links`            | `link_id`                           | INTEGER PK    | ✅  | -   | -   |
| `radar_transit_links`            | `transit_id`                        | INTEGER FK    | ✅  | -   | -   |
| `radar_transit_links`            | `data_rowid`                        | INTEGER FK    | ✅  | -   | -   |
| `radar_transit_links`            | `link_score`                        | DOUBLE        | ✅  | -   | -   |
| `radar_transit_links`            | `created_at`                        | DOUBLE        | ✅  | -   | -   |
| `radar_commands`                 | `command_id`                        | BIGINT PK     | ✅  | -   | -   |
| `radar_commands`                 | `command`                           | TEXT          | ✅  | -   | -   |
| `radar_commands`                 | `write_timestamp`                   | DOUBLE        | ✅  | -   | -   |
| `radar_command_log`              | `log_id`                            | BIGINT PK     | ✅  | -   | -   |
| `radar_command_log`              | `command_id`                        | BIGINT FK     | ✅  | -   | -   |
| `radar_command_log`              | `log_data`                          | TEXT          | ✅  | -   | -   |
| `radar_command_log`              | `write_timestamp`                   | DOUBLE        | ✅  | -   | -   |
| `site`                           | `id`                                | INTEGER PK    | ✅  | ✅  | -   |
| `site`                           | `name`                              | TEXT UNIQUE   | ✅  | ✅  | -   |
| `site`                           | `location`                          | TEXT          | ✅  | ✅  | -   |
| `site`                           | `description`                       | TEXT          | ✅  | ✅  | -   |
| `site`                           | `surveyor`                          | TEXT          | ✅  | ✅  | -   |
| `site`                           | `contact`                           | TEXT          | ✅  | ✅  | -   |
| `site`                           | `address`                           | TEXT          | ✅  | ✅  | -   |
| `site`                           | `latitude`                          | REAL          | ✅  | ✅  | -   |
| `site`                           | `longitude`                         | REAL          | ✅  | ✅  | -   |
| `site`                           | `map_angle`                         | REAL          | ✅  | ✅  | -   |
| `site`                           | `include_map`                       | INTEGER       | ✅  | ✅  | -   |
| `site`                           | `site_description`                  | TEXT          | ✅  | ✅  | -   |
| `site`                           | `bbox_ne_lat`                       | REAL          | ✅  | ✅  | -   |
| `site`                           | `bbox_ne_lng`                       | REAL          | ✅  | ✅  | -   |
| `site`                           | `bbox_sw_lat`                       | REAL          | ✅  | ✅  | -   |
| `site`                           | `bbox_sw_lng`                       | REAL          | ✅  | ✅  | -   |
| `site`                           | `map_svg_data`                      | BLOB          | ✅  | ✅  | -   |
| `site`                           | `created_at`                        | INTEGER       | ✅  | ✅  | -   |
| `site`                           | `updated_at`                        | INTEGER       | ✅  | ✅  | -   |
| `site_config_periods`            | `id`                                | INTEGER PK    | ✅  | ✅  | -   |
| `site_config_periods`            | `site_id`                           | INTEGER FK    | ✅  | ✅  | -   |
| `site_config_periods`            | `effective_start_unix`              | DOUBLE        | ✅  | ✅  | -   |
| `site_config_periods`            | `effective_end_unix`                | DOUBLE        | ✅  | ✅  | -   |
| `site_config_periods`            | `is_active`                         | INTEGER       | ✅  | ✅  | -   |
| `site_config_periods`            | `notes`                             | TEXT          | ✅  | ✅  | -   |
| `site_config_periods`            | `cosine_error_angle`                | DOUBLE        | ✅  | ✅  | -   |
| `site_config_periods`            | `created_at`                        | DOUBLE        | ✅  | ✅  | -   |
| `site_config_periods`            | `updated_at`                        | DOUBLE        | ✅  | ✅  | -   |
| `site_config_periods`            | `speed_limit_kph`                   | DOUBLE        | ✅  | ✅  | -   |
| `site_config_periods`            | `speed_limit_unit`                  | TEXT          | ✅  | ✅  | -   |
| `site_config_periods`            | `jurisdiction`                      | TEXT          | ✅  | ✅  | -   |
| `site_reports`                   | `id`                                | INTEGER PK    | ✅  | ✅  | -   |
| `site_reports`                   | `site_id`                           | INTEGER FK    | ✅  | ✅  | -   |
| `site_reports`                   | `start_date`                        | TEXT          | ✅  | ✅  | -   |
| `site_reports`                   | `end_date`                          | TEXT          | ✅  | ✅  | -   |
| `site_reports`                   | `filepath`                          | TEXT          | ✅  | ✅  | -   |
| `site_reports`                   | `filename`                          | TEXT          | ✅  | ✅  | -   |
| `site_reports`                   | `zip_filepath`                      | TEXT          | ✅  | ✅  | -   |
| `site_reports`                   | `zip_filename`                      | TEXT          | ✅  | ✅  | -   |
| `site_reports`                   | `run_id`                            | TEXT          | ✅  | ✅  | -   |
| `site_reports`                   | `timezone`                          | TEXT          | ✅  | ✅  | -   |
| `site_reports`                   | `units`                             | TEXT          | ✅  | ✅  | -   |
| `site_reports`                   | `source`                            | TEXT          | ✅  | ✅  | -   |
| `site_reports`                   | `created_at`                        | DATETIME      | ✅  | ✅  | -   |
| `schema_migrations`              | `version`                           | UINT64        | ✅  | -   | -   |
| `schema_migrations`              | `dirty`                             | BOOLEAN       | ✅  | -   | -   |
| `lidar_capture_files`            | `capture_file_id`                   | TEXT PK       | ✅  | ?   | ?   |
| `lidar_capture_files`            | `root_id`                           | TEXT          | ✅  | ✅  | ?   |
| `lidar_capture_files`            | `rel_path`                          | TEXT          | ✅  | ✅  | ?   |
| `lidar_capture_files`            | `size_bytes`                        | INTEGER       | ✅  | ✅  | ?   |
| `lidar_capture_files`            | `modified_at_ns`                    | INTEGER       | ✅  | ?   | ?   |
| `lidar_capture_files`            | `content_tag`                       | TEXT          | ✅  | ?   | ?   |
| `lidar_capture_files`            | `first_packet_ns`                   | INTEGER       | ✅  | ✅  | ?   |
| `lidar_capture_files`            | `last_packet_ns`                    | INTEGER       | ✅  | ✅  | ?   |
| `lidar_capture_files`            | `packet_count`                      | INTEGER       | ✅  | ?   | ?   |
| `lidar_capture_files`            | `udp_port`                          | INTEGER       | ✅  | ?   | ?   |
| `lidar_capture_files`            | `probe_state`                       | TEXT          | ✅  | ✅  | ?   |
| `lidar_capture_files`            | `probe_error`                       | TEXT          | ✅  | ?   | ?   |
| `lidar_capture_files`            | `probed_at_ns`                      | INTEGER       | ✅  | ?   | ?   |
| `lidar_capture_files`            | `present`                           | INTEGER       | ✅  | ?   | ?   |
| `lidar_capture_files`            | `first_seen_at_ns`                  | INTEGER       | ✅  | ?   | ?   |
| `lidar_capture_files`            | `last_seen_at_ns`                   | INTEGER       | ✅  | ?   | ?   |
| `lidar_capture_files`            | `session_id`                        | TEXT          | ✅  | ✅  | ?   |
| `lidar_capture_jobs`             | `job_id`                            | TEXT PK       | ✅  | ✅  | ?   |
| `lidar_capture_jobs`             | `kind`                              | TEXT          | ✅  | ✅  | ?   |
| `lidar_capture_jobs`             | `session_id`                        | TEXT          | ✅  | ✅  | ?   |
| `lidar_capture_jobs`             | `root_id`                           | TEXT          | ✅  | ✅  | ?   |
| `lidar_capture_jobs`             | `state`                             | TEXT          | ✅  | ✅  | ?   |
| `lidar_capture_jobs`             | `progress_current`                  | INTEGER       | ✅  | ✅  | ?   |
| `lidar_capture_jobs`             | `progress_total`                    | INTEGER       | ✅  | ✅  | ?   |
| `lidar_capture_jobs`             | `detail`                            | TEXT          | ✅  | ✅  | ?   |
| `lidar_capture_jobs`             | `error`                             | TEXT          | ✅  | ✅  | ?   |
| `lidar_capture_jobs`             | `queued_at_ns`                      | INTEGER       | ✅  | ?   | ?   |
| `lidar_capture_jobs`             | `started_at_ns`                     | INTEGER       | ✅  | ?   | ?   |
| `lidar_capture_jobs`             | `finished_at_ns`                    | INTEGER       | ✅  | ?   | ?   |
| `lidar_capture_motion_periods`   | `period_id`                         | TEXT PK       | ✅  | ✅  | ?   |
| `lidar_capture_motion_periods`   | `session_id`                        | TEXT          | ✅  | ✅  | ?   |
| `lidar_capture_motion_periods`   | `ordinal`                           | INTEGER       | ✅  | ?   | ?   |
| `lidar_capture_motion_periods`   | `period_type`                       | TEXT          | ✅  | ✅  | ?   |
| `lidar_capture_motion_periods`   | `label`                             | TEXT          | ✅  | ✅  | ?   |
| `lidar_capture_motion_periods`   | `start_ns`                          | INTEGER       | ✅  | ✅  | ?   |
| `lidar_capture_motion_periods`   | `end_ns`                            | INTEGER       | ✅  | ✅  | ?   |
| `lidar_capture_motion_periods`   | `duration_ns`                       | INTEGER       | ✅  | ?   | ?   |
| `lidar_capture_motion_periods`   | `start_secs`                        | REAL          | ✅  | ?   | ?   |
| `lidar_capture_motion_periods`   | `end_secs`                          | REAL          | ✅  | ?   | ?   |
| `lidar_capture_motion_periods`   | `start_frame`                       | INTEGER       | ✅  | ?   | ?   |
| `lidar_capture_motion_periods`   | `end_frame`                         | INTEGER       | ✅  | ?   | ?   |
| `lidar_capture_motion_periods`   | `created_at_ns`                     | INTEGER       | ✅  | ?   | ?   |
| `lidar_capture_roots`            | `root_id`                           | TEXT PK       | ✅  | ✅  | ?   |
| `lidar_capture_roots`            | `path`                              | TEXT          | ✅  | ✅  | ?   |
| `lidar_capture_roots`            | `label`                             | TEXT          | ✅  | ✅  | ?   |
| `lidar_capture_roots`            | `enabled`                           | INTEGER       | ✅  | ?   | ?   |
| `lidar_capture_roots`            | `last_scan_at_ns`                   | INTEGER       | ✅  | ?   | ?   |
| `lidar_capture_roots`            | `last_scan_state`                   | TEXT          | ✅  | ✅  | ?   |
| `lidar_capture_roots`            | `last_scan_error`                   | TEXT          | ✅  | ?   | ?   |
| `lidar_capture_roots`            | `created_at_ns`                     | INTEGER       | ✅  | ?   | ?   |
| `lidar_capture_roots`            | `updated_at_ns`                     | INTEGER       | ✅  | ?   | ?   |
| `lidar_capture_sessions`         | `session_id`                        | TEXT PK       | ✅  | ✅  | ?   |
| `lidar_capture_sessions`         | `root_id`                           | TEXT          | ✅  | ✅  | ?   |
| `lidar_capture_sessions`         | `label`                             | TEXT          | ✅  | ✅  | ?   |
| `lidar_capture_sessions`         | `sensor_id`                         | TEXT          | ✅  | ?   | ?   |
| `lidar_capture_sessions`         | `file_count`                        | INTEGER       | ✅  | ✅  | ?   |
| `lidar_capture_sessions`         | `start_ns`                          | INTEGER       | ✅  | ✅  | ?   |
| `lidar_capture_sessions`         | `end_ns`                            | INTEGER       | ✅  | ✅  | ?   |
| `lidar_capture_sessions`         | `covered_ns`                        | INTEGER       | ✅  | ?   | ?   |
| `lidar_capture_sessions`         | `lost_ns`                           | INTEGER       | ✅  | ?   | ?   |
| `lidar_capture_sessions`         | `worst_seam`                        | TEXT          | ✅  | ?   | ?   |
| `lidar_capture_sessions`         | `size_bytes`                        | INTEGER       | ✅  | ✅  | ?   |
| `lidar_capture_sessions`         | `derived_at_ns`                     | INTEGER       | ✅  | ?   | ?   |
| `lidar_exposure_windows`         | `window_id`                         | TEXT PK       | ✅  | ?   | ?   |
| `lidar_exposure_windows`         | `event_id`                          | TEXT          | ✅  | ?   | ?   |
| `lidar_exposure_windows`         | `window_json`                       | JSON          | ✅  | ?   | ?   |
| `lidar_exposure_windows`         | `source_id`                         | TEXT          | ✅  | ?   | ?   |
| `lidar_exposure_windows`         | `kind`                              | TEXT          | ✅  | ?   | ?   |
| `lidar_exposure_windows`         | `basis`                             | TEXT          | ✅  | ?   | ?   |
| `lidar_exposure_windows`         | `track_id`                          | TEXT          | ✅  | ?   | ?   |
| `lidar_exposure_windows`         | `counterpart_track_id`              | TEXT          | ✅  | ?   | ?   |
| `lidar_exposure_windows`         | `start_unix_nanos`                  | INTEGER       | ✅  | ?   | ?   |
| `lidar_exposure_windows`         | `end_unix_nanos`                    | INTEGER       | ✅  | ?   | ?   |
| `lidar_exposure_windows`         | `duration_nanos`                    | INTEGER       | ✅  | ?   | ?   |
| `lidar_exposure_windows`         | `estimate_stage`                    | TEXT          | ✅  | ?   | ?   |
| `lidar_exposure_windows`         | `estimator_id`                      | TEXT          | ✅  | ?   | ?   |
| `lidar_exposure_windows`         | `obs_model_id`                      | TEXT          | ✅  | ?   | ?   |
| `lidar_exposure_windows`         | `method_id`                         | TEXT          | ✅  | ?   | ?   |
| `lidar_exposure_windows`         | `param_hash`                        | TEXT          | ✅  | ?   | ?   |
| `lidar_exposure_windows`         | `inserted_at_ns`                    | INTEGER       | ✅  | ?   | ?   |
| `lidar_interaction_events`       | `event_id`                          | TEXT PK       | ✅  | ?   | ?   |
| `lidar_interaction_events`       | `event_json`                        | JSON          | ✅  | ?   | ?   |
| `lidar_interaction_events`       | `source_id`                         | TEXT          | ✅  | ?   | ?   |
| `lidar_interaction_events`       | `interaction_type`                  | TEXT          | ✅  | ?   | ?   |
| `lidar_interaction_events`       | `primary_track_id`                  | TEXT          | ✅  | ?   | ?   |
| `lidar_interaction_events`       | `secondary_track_id`                | TEXT          | ✅  | ?   | ?   |
| `lidar_interaction_events`       | `start_unix_nanos`                  | INTEGER       | ✅  | ?   | ?   |
| `lidar_interaction_events`       | `end_unix_nanos`                    | INTEGER       | ✅  | ?   | ?   |
| `lidar_interaction_events`       | `estimate_stage`                    | TEXT          | ✅  | ?   | ?   |
| `lidar_interaction_events`       | `estimator_id`                      | TEXT          | ✅  | ?   | ?   |
| `lidar_interaction_events`       | `obs_model_id`                      | TEXT          | ✅  | ?   | ?   |
| `lidar_interaction_events`       | `method_id`                         | TEXT          | ✅  | ?   | ?   |
| `lidar_interaction_events`       | `param_hash`                        | TEXT          | ✅  | ?   | ?   |
| `lidar_interaction_events`       | `worst_support`                     | TEXT          | ✅  | ?   | ?   |
| `lidar_interaction_events`       | `inserted_at_ns`                    | INTEGER       | ✅  | ?   | ?   |
| `lidar_interaction_instants`     | `event_id`                          | TEXT PK       | ✅  | ?   | ?   |
| `lidar_interaction_instants`     | `capture_unix_nanos`                | INTEGER PK    | ✅  | ?   | ?   |
| `lidar_interaction_instants`     | `instant_json`                      | JSON          | ✅  | ?   | ?   |
| `lidar_interaction_instants`     | `basis`                             | TEXT          | ✅  | ?   | ?   |
| `lidar_interaction_instants`     | `valid`                             | INTEGER       | ✅  | ?   | ?   |
| `lidar_interaction_instants`     | `reason`                            | TEXT          | ✅  | ?   | ?   |
| `lidar_migration_rejects`        | `reject_id`                         | INTEGER PK    | ✅  | -   | -   |
| `lidar_migration_rejects`        | `migration`                         | INTEGER       | ✅  | -   | -   |
| `lidar_migration_rejects`        | `source_table`                      | TEXT          | ✅  | -   | -   |
| `lidar_migration_rejects`        | `source_key`                        | TEXT          | ✅  | -   | -   |
| `lidar_migration_rejects`        | `reason`                            | TEXT          | ✅  | -   | -   |
| `lidar_migration_rejects`        | `row_json`                          | TEXT          | ✅  | -   | -   |
| `lidar_migration_rejects`        | `rejected_at_ns`                    | INTEGER       | ✅  | -   | -   |
| `lidar_observations`             | `observation_id`                    | TEXT PK       | ✅  | ?   | ?   |
| `lidar_observations`             | `schema_version`                    | INTEGER       | ✅  | ?   | ?   |
| `lidar_observations`             | `source_id`                         | TEXT          | ✅  | ?   | ?   |
| `lidar_observations`             | `calibration_id`                    | TEXT          | ✅  | ?   | ?   |
| `lidar_observations`             | `sensor_id`                         | TEXT          | ✅  | ?   | ?   |
| `lidar_observations`             | `frame_id`                          | TEXT          | ✅  | ?   | ?   |
| `lidar_observations`             | `frame_unix_nanos`                  | INTEGER       | ✅  | ?   | ?   |
| `lidar_observations`             | `cluster_unix_nanos`                | INTEGER       | ✅  | ?   | ?   |
| `lidar_observations`             | `cluster_id`                        | INTEGER       | ✅  | ?   | ?   |
| `lidar_observations`             | `record_json`                       | BLOB          | ✅  | ?   | ?   |
| `lidar_observations`             | `inserted_at_ns`                    | INTEGER       | ✅  | ?   | ?   |
| `lidar_replay_case_files`        | `replay_case_id`                    | TEXT PK       | ✅  | ?   | ?   |
| `lidar_replay_case_files`        | `ordinal`                           | INTEGER PK    | ✅  | ?   | ?   |
| `lidar_replay_case_files`        | `capture_file_id`                   | TEXT          | ✅  | ?   | ?   |
| `lidar_replay_case_files`        | `pcap_file`                         | TEXT          | ✅  | ?   | ?   |
| `lidar_replay_cases`             | `session_id`                        | TEXT          | ✅  | ?   | ?   |
| `lidar_replay_cases`             | `source_period_id`                  | TEXT          | ✅  | ?   | ?   |
| `lidar_replay_cases`             | `origin_lat`                        | REAL          | ✅  | ?   | ?   |
| `lidar_replay_cases`             | `origin_lon`                        | REAL          | ✅  | ?   | ?   |
| `lidar_replay_cases`             | `s2_l10_token`                      | TEXT          | ✅  | ?   | ?   |
| `lidar_replay_cases`             | `s2_l13_token`                      | TEXT          | ✅  | ?   | ?   |
| `lidar_replay_cases`             | `s2_l16_token`                      | TEXT          | ✅  | ?   | ?   |
| `lidar_replay_cases`             | `geographic_source`                 | TEXT          | ✅  | ?   | ?   |
| `lidar_replay_cases`             | `geographic_status`                 | TEXT          | ✅  | ?   | ?   |
| `lidar_replay_cases`             | `site_id`                           | TEXT          | ✅  | ?   | ?   |
| `lidar_scenes`                   | `scene_id`                          | TEXT PK       | ✅  | ?   | ?   |
| `lidar_scenes`                   | `site_id`                           | INTEGER       | ✅  | ?   | ?   |
| `lidar_scenes`                   | `title`                             | TEXT          | ✅  | ?   | ?   |
| `lidar_scenes`                   | `description`                       | TEXT          | ✅  | ?   | ?   |
| `lidar_scenes`                   | `latitude`                          | REAL          | ✅  | ?   | ?   |
| `lidar_scenes`                   | `longitude`                         | REAL          | ✅  | ?   | ?   |
| `lidar_scenes`                   | `captured_start_ns`                 | INTEGER       | ✅  | ?   | ?   |
| `lidar_scenes`                   | `captured_end_ns`                   | INTEGER       | ✅  | ?   | ?   |
| `lidar_scenes`                   | `duration_secs`                     | REAL          | ✅  | ?   | ?   |
| `lidar_scenes`                   | `source_capture`                    | TEXT          | ✅  | ?   | ?   |
| `lidar_scenes`                   | `source_vrlog_sha256`               | TEXT          | ✅  | ?   | ?   |
| `lidar_scenes`                   | `frame_count`                       | INTEGER       | ✅  | ?   | ?   |
| `lidar_scenes`                   | `frame_stride`                      | INTEGER       | ✅  | ?   | ?   |
| `lidar_scenes`                   | `asset_path`                        | TEXT          | ✅  | ?   | ?   |
| `lidar_scenes`                   | `published`                         | INTEGER       | ✅  | ?   | ?   |
| `lidar_scenes`                   | `created_at`                        | INTEGER       | ✅  | ?   | ?   |
| `lidar_scenes`                   | `updated_at`                        | INTEGER       | ✅  | ?   | ?   |
| `lidar_segment_clip_jobs`        | `job_id`                            | TEXT PK       | ✅  | ✅  | -   |
| `lidar_segment_clip_jobs`        | `segment_id`                        | TEXT          | ✅  | -   | -   |
| `lidar_segment_clip_jobs`        | `pack_dir`                          | TEXT          | ✅  | 🔶  | -   |
| `lidar_segment_clip_jobs`        | `pack_digest`                       | TEXT          | ✅  | -   | -   |
| `lidar_segment_selections`       | `segment_id`                        | TEXT PK       | ✅  | 🔶  | -   |
| `lidar_segment_selections`       | `run_id`                            | TEXT          | ✅  | -   | -   |
| `lidar_segment_selections`       | `replay_case_id`                    | TEXT          | ✅  | ✅  | -   |
| `lidar_segment_selections`       | `document_version`                  | INTEGER       | ✅  | -   | -   |
| `lidar_segment_selections`       | `parameters_json`                   | TEXT          | ✅  | -   | -   |
| `lidar_segment_selections`       | `window_json`                       | TEXT          | ✅  | -   | -   |
| `lidar_segment_selections`       | `source`                            | TEXT          | ✅  | -   | -   |
| `lidar_segment_selections`       | `role`                              | TEXT          | ✅  | -   | -   |
| `lidar_segment_selections`       | `finder`                            | TEXT          | ✅  | -   | -   |
| `lidar_segment_selections`       | `finder_version`                    | INTEGER       | ✅  | -   | -   |
| `lidar_segment_selections`       | `capture`                           | TEXT          | ✅  | -   | -   |
| `lidar_segment_selections`       | `window_start_ns`                   | INTEGER       | ✅  | -   | -   |
| `lidar_segment_selections`       | `window_end_ns`                     | INTEGER       | ✅  | -   | -   |
| `lidar_segment_selections`       | `created_at_ns`                     | INTEGER       | ✅  | -   | -   |
| `lidar_segment_selections`       | `selector_json`                     | TEXT          | ✅  | -   | -   |
| `lidar_sites`                    | `site_id`                           | TEXT PK       | ✅  | ?   | ?   |
| `lidar_sites`                    | `s2_l13_token`                      | TEXT          | ✅  | ?   | ?   |
| `lidar_sites`                    | `s2_l10_token`                      | TEXT          | ✅  | ?   | ?   |
| `lidar_sites`                    | `label`                             | TEXT          | ✅  | ?   | ?   |
| `lidar_sites`                    | `canonical_lat`                     | REAL          | ✅  | ?   | ?   |
| `lidar_sites`                    | `canonical_lon`                     | REAL          | ✅  | ?   | ?   |
| `lidar_sites`                    | `canonical_source`                  | TEXT          | ✅  | ?   | ?   |
| `lidar_sites`                    | `created_at_ns`                     | INTEGER       | ✅  | ?   | ?   |
| `lidar_sites`                    | `updated_at_ns`                     | INTEGER       | ✅  | ?   | ?   |
| `lidar_track_estimate_revisions` | `estimate_id`                       | TEXT PK       | ✅  | ?   | ?   |
| `lidar_track_estimate_revisions` | `revises_estimate_id`               | TEXT          | ✅  | ?   | ?   |
| `lidar_track_estimate_revisions` | `smoother_id`                       | TEXT          | ✅  | ?   | ?   |
| `lidar_track_estimate_revisions` | `lag`                               | TEXT          | ✅  | ?   | ?   |
| `lidar_track_estimate_revisions` | `lookahead_steps`                   | INTEGER       | ✅  | ?   | ?   |
| `lidar_track_estimate_revisions` | `lookahead_secs`                    | REAL          | ✅  | ?   | ?   |
| `lidar_track_estimate_revisions` | `released_at_unix_nanos`            | INTEGER       | ✅  | ?   | ?   |
| `lidar_track_estimate_revisions` | `release_reason`                    | TEXT          | ✅  | ?   | ?   |
| `lidar_track_estimate_revisions` | `chain_end_reason`                  | TEXT          | ✅  | ?   | ?   |
| `lidar_track_estimate_revisions` | `flags`                             | TEXT          | ✅  | ?   | ?   |
| `lidar_track_estimate_revisions` | `previous_x`                        | REAL          | ✅  | ?   | ?   |
| `lidar_track_estimate_revisions` | `previous_y`                        | REAL          | ✅  | ?   | ?   |
| `lidar_track_estimate_revisions` | `previous_vx`                       | REAL          | ✅  | ?   | ?   |
| `lidar_track_estimate_revisions` | `previous_vy`                       | REAL          | ✅  | ?   | ?   |
| `lidar_track_estimate_revisions` | `revision_position_m`               | REAL          | ✅  | ?   | ?   |
| `lidar_track_estimate_revisions` | `revision_velocity_mps`             | REAL          | ✅  | ?   | ?   |
| `lidar_track_estimate_revisions` | `evidence_count`                    | INTEGER       | ✅  | ?   | ?   |
| `lidar_track_estimate_revisions` | `evidence_first_frame_unix_nanos`   | INTEGER       | ✅  | ?   | ?   |
| `lidar_track_estimate_revisions` | `evidence_last_frame_unix_nanos`    | INTEGER       | ✅  | ?   | ?   |
| `lidar_track_estimate_revisions` | `strongest_evidence_observation_id` | TEXT          | ✅  | ?   | ?   |
| `lidar_track_estimate_revisions` | `strongest_evidence_nis`            | REAL          | ✅  | ?   | ?   |
| `lidar_track_estimate_revisions` | `inserted_at_ns`                    | INTEGER       | ✅  | ?   | ?   |
| `lidar_track_estimates`          | `estimate_id`                       | TEXT PK       | ✅  | ?   | ?   |
| `lidar_track_estimates`          | `track_id`                          | TEXT          | ✅  | ?   | ?   |
| `lidar_track_estimates`          | `creation_sequence`                 | INTEGER       | ✅  | ?   | ?   |
| `lidar_track_estimates`          | `observation_id`                    | TEXT          | ✅  | ?   | ?   |
| `lidar_track_estimates`          | `source_id`                         | TEXT          | ✅  | ?   | ?   |
| `lidar_track_estimates`          | `calibration_id`                    | TEXT          | ✅  | ?   | ?   |
| `lidar_track_estimates`          | `frame_unix_nanos`                  | INTEGER       | ✅  | ?   | ?   |
| `lidar_track_estimates`          | `measurement_unix_nanos`            | INTEGER       | ✅  | ?   | ?   |
| `lidar_track_estimates`          | `estimator_id`                      | TEXT          | ✅  | ?   | ?   |
| `lidar_track_estimates`          | `observation_model_id`              | TEXT          | ✅  | ?   | ?   |
| `lidar_track_estimates`          | `param_hash`                        | TEXT          | ✅  | ?   | ?   |
| `lidar_track_estimates`          | `stage`                             | TEXT          | ✅  | ?   | ?   |
| `lidar_track_estimates`          | `measurement_source`                | TEXT          | ✅  | ?   | ?   |
| `lidar_track_estimates`          | `x`                                 | REAL          | ✅  | ?   | ?   |
| `lidar_track_estimates`          | `y`                                 | REAL          | ✅  | ?   | ?   |
| `lidar_track_estimates`          | `vx`                                | REAL          | ✅  | ?   | ?   |
| `lidar_track_estimates`          | `vy`                                | REAL          | ✅  | ?   | ?   |
| `lidar_track_estimates`          | `covariance_json`                   | BLOB          | ✅  | ?   | ?   |
| `lidar_track_estimates`          | `inserted_at_ns`                    | INTEGER       | ✅  | ?   | ?   |
| `lidar_track_estimates`          | `state_model`                       | TEXT          | ✅  | ?   | ?   |
| `lidar_track_estimates`          | `reference_point`                   | TEXT          | ✅  | ?   | ?   |
| `lidar_track_estimates`          | `support_instant`                   | TEXT          | ✅  | ?   | ?   |
| `lidar_track_observations`       | `frame_unix_nanos`                  | INTEGER       | ✅  | ?   | ?   |
| `lidar_track_observations`       | `measurement_source`                | TEXT          | ✅  | ?   | ?   |
| `lidar_track_residuals`          | `estimate_id`                       | TEXT PK       | ✅  | ?   | ?   |
| `lidar_track_residuals`          | `observation_id`                    | TEXT          | ✅  | ?   | ?   |
| `lidar_track_residuals`          | `predicted_x`                       | REAL          | ✅  | ?   | ?   |
| `lidar_track_residuals`          | `predicted_y`                       | REAL          | ✅  | ?   | ?   |
| `lidar_track_residuals`          | `measurement_x`                     | REAL          | ✅  | ?   | ?   |
| `lidar_track_residuals`          | `measurement_y`                     | REAL          | ✅  | ?   | ?   |
| `lidar_track_residuals`          | `innovation_x`                      | REAL          | ✅  | ?   | ?   |
| `lidar_track_residuals`          | `innovation_y`                      | REAL          | ✅  | ?   | ?   |
| `lidar_track_residuals`          | `nis`                               | REAL          | ✅  | ?   | ?   |
| `lidar_track_residuals`          | `geometry_cov_xx`                   | REAL          | ✅  | ?   | ?   |
| `lidar_track_residuals`          | `geometry_cov_xy`                   | REAL          | ✅  | ?   | ?   |
| `lidar_track_residuals`          | `geometry_cov_yy`                   | REAL          | ✅  | ?   | ?   |
| `lidar_track_residuals`          | `disposition`                       | TEXT          | ✅  | ?   | ?   |
| `lidar_track_residuals`          | `reason`                            | TEXT          | ✅  | ?   | ?   |
| `lidar_track_residuals`          | `inserted_at_ns`                    | INTEGER       | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `estimate_id`                       | TEXT PK       | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `track_id`                          | TEXT          | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `creation_sequence`                 | INTEGER       | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `observation_id`                    | TEXT          | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `source_id`                         | TEXT          | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `calibration_id`                    | TEXT          | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `frame_unix_nanos`                  | INTEGER       | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `measurement_unix_nanos`            | INTEGER       | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `estimator_id`                      | TEXT          | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `observation_model_id`              | TEXT          | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `param_hash`                        | TEXT          | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `stage`                             | TEXT          | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `state_model`                       | TEXT          | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `reference_point`                   | TEXT          | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `x`                                 | REAL          | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `y`                                 | REAL          | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `vx`                                | REAL          | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `vy`                                | REAL          | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `covariance_json`                   | BLOB          | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `heading_rad`                       | REAL          | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `heading_variance_rad2`             | REAL          | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `heading_ambiguous_weight`          | REAL          | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `heading_provenance`                | TEXT          | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `length_m`                          | REAL          | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `length_sigma_m`                    | REAL          | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `length_frames`                     | INTEGER       | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `length_provenance`                 | TEXT          | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `width_m`                           | REAL          | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `width_sigma_m`                     | REAL          | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `width_frames`                      | INTEGER       | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `width_provenance`                  | TEXT          | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `height_m`                          | REAL          | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `height_sigma_m`                    | REAL          | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `height_frames`                     | INTEGER       | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `height_provenance`                 | TEXT          | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `ground_z`                          | REAL          | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `ground_surface_model`              | TEXT          | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `motion_class`                      | TEXT          | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `motion_posterior`                  | REAL          | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `estimation_state`                  | TEXT          | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `last_observed_unix_nanos`          | INTEGER       | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `support_points`                    | INTEGER       | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `coasted_frames`                    | INTEGER       | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `measurement_source`                | TEXT          | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `measurement_rank`                  | INTEGER       | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `visible_faces`                     | TEXT          | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `inferred_extent`                   | INTEGER       | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `aspect_rad`                        | REAL          | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `nis`                               | REAL          | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `fallback_reason`                   | TEXT          | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `inserted_at_ns`                    | INTEGER       | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `support_instant`                   | TEXT          | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `support_fragmented`                | INTEGER       | ✅  | ?   | ?   |
| `lidar_track_solid_bodies`       | `support_truncated`                 | INTEGER       | ✅  | ?   | ?   |
| `radar_serial_config`            | `id`                                | INTEGER PK    | ✅  | ?   | ?   |
| `radar_serial_config`            | `port_path`                         | TEXT          | ✅  | ?   | ?   |
| `radar_serial_config`            | `baud_rate`                         | INTEGER       | ✅  | ?   | ?   |
| `radar_serial_config`            | `data_bits`                         | INTEGER       | ✅  | ?   | ?   |
| `radar_serial_config`            | `stop_bits`                         | INTEGER       | ✅  | ?   | ?   |
| `radar_serial_config`            | `parity`                            | TEXT          | ✅  | ?   | ?   |
| `radar_serial_config`            | `enabled`                           | INTEGER       | ✅  | ?   | ?   |
| `radar_serial_config`            | `sensor_model`                      | TEXT          | ✅  | ?   | ?   |
| `radar_serial_config`            | `created_at`                        | INTEGER       | ✅  | ?   | ?   |
| `radar_serial_config`            | `updated_at`                        | INTEGER       | ✅  | ?   | ?   |
| `site`                           | `radar_svg_x`                       | REAL          | ✅  | ?   | ?   |
| `site`                           | `radar_svg_y`                       | REAL          | ✅  | ?   | ?   |

`lidar_segment_selections.segment_id` is 🔶 on Web: the page receives the computed window ID; the stored ID joins selection state to that window, but the handler does not serialise the column itself.

`lidar_segment_clip_jobs.pack_dir` is 🔶 on Web: the column holds a path relative to the annotation packs directory, and the ranking sends the page the absolute path made from it. The page shows the pack it finds in the inventory, which is read from disk.

The segment tables' Mac column is `-`, not `?`: the macOS tool opens a pack directory and reads `segment.json` from it. It calls none of the segment, clip or inventory endpoints.

`lidar_segment_selections` derives `source`, `role`, `finder`, `finder_version`, `capture`, `window_start_ns` and `window_end_ns` from `window_json` as stored generated columns. They are read and indexed, never written. `selector_json` records the selector that chose the window, as it ran; a trigger requires it on every new row, and it is null only for windows chosen before migration 000056.

---

## 6. Pipeline stages

| Folder                                                           | File                  | Stage                                      | DB  | Web | Mac |
| ---------------------------------------------------------------- | --------------------- | ------------------------------------------ | --- | --- | --- |
| [internal/lidar/l2frames](../../internal/lidar/l2frames)         | `frame_builder.go`    | L2 Frame Builder (UDP → point clouds)      | -   | -   | -   |
| [internal/lidar/l3grid](../../internal/lidar/l3grid)             | `background.go`       | L3 Background Grid (foreground/background) | ✅  | ✅  | -   |
| [internal/lidar/l3grid](../../internal/lidar/l3grid)             | `foreground.go`       | L3 FrameMetrics (foreground fraction)      | -   | -   | -   |
| [internal/lidar/l4perception](../../internal/lidar/l4perception) | `dbscan_clusterer.go` | L4 Clustering (DBSCAN → world clusters)    | ✅  | ✅  | ✅  |
| [internal/lidar/l5tracks](../../internal/lidar/l5tracks)         | `tracking.go`         | L5 Tracking (Kalman → tracked objects)     | ✅  | ✅  | ✅  |
| [internal/lidar/l5tracks](../../internal/lidar/l5tracks)         | `tracking.go`         | L5 TrackingMetrics (fragmentation, jitter) | -   | ✅  | -   |
| [internal/lidar/adapters](../../internal/lidar/adapters)         | `ground_truth.go`     | L6 Evaluation (quality metrics)            | ✅  | ✅  | -   |
| [internal/lidar/l6objects](../../internal/lidar/l6objects)       | `quality.go`          | L6 RunStatistics (12 fields)               | 📋  | 📋  | -   |
| [internal/lidar/l6objects](../../internal/lidar/l6objects)       | `quality.go`          | L6 TrackQualityMetrics (8 fields)          | ✅  | 📋  | -   |
| [internal/lidar/l6objects](../../internal/lidar/l6objects)       | `quality.go`          | L6 NoiseCoverageMetrics (7 fields)         | 📋  | 📋  | -   |
| [internal/lidar/l6objects](../../internal/lidar/l6objects)       | `quality.go`          | L6 TrainingDatasetSummary (7 fields)       | -   | -   | -   |
| [internal/lidar/l6objects](../../internal/lidar/l6objects)       | `features.go`         | L6 TrackFeatures (20 features)             | -   | -   | -   |
| [internal/lidar/l6objects](../../internal/lidar/l6objects)       | `features.go`         | L6 ClusterFeatures (10 features)           | -   | -   | -   |

---

## 7. Go data structures: computed but not persisted

These structs are computed in-memory but have no persistence layer, no API
endpoint, and no export path.

| Folder                                                     | File            | Struct                              | DB  | Web | Mac | Notes                                                                                                                                |
| ---------------------------------------------------------- | --------------- | ----------------------------------- | --- | --- | --- | ------------------------------------------------------------------------------------------------------------------------------------ |
| [internal/lidar/l6objects](../../internal/lidar/l6objects) | `quality.go`    | `NoiseCoverageMetrics` (7 fields)   | 📋  | 📋  | -   | Partially implemented (TODOs)                                                                                                        |
| [internal/lidar/l6objects](../../internal/lidar/l6objects) | `quality.go`    | `TrainingDatasetSummary` (7 fields) | -   | -   | -   | No consumer; separate project                                                                                                        |
| [internal/lidar/l6objects](../../internal/lidar/l6objects) | `features.go`   | `TrackFeatures` (20 features)       | -   | -   | -   | Used in-memory by classifier                                                                                                         |
| [internal/lidar/l6objects](../../internal/lidar/l6objects) | `features.go`   | `ClusterFeatures` (10 features)     | -   | -   | -   | Used in-memory by classifier                                                                                                         |
| [internal/lidar/l3grid](../../internal/lidar/l3grid)       | `foreground.go` | `FrameMetrics` (5 fields)           | 📋  | 📋  | -   | Transient; [HINT plan C1](../../docs/plans/hint-metric-observability-plan.md)                                                        |
| [internal/lidar/l5tracks](../../internal/lidar/l5tracks)   | `tracking.go`   | `TrackAlignmentMetrics` (per-track) | 📋  | ✅  | -   | [HINT plan D2](../../docs/plans/hint-metric-observability-plan.md); nested in `GET /api/lidar/tracks/metrics?include_per_track=true` |
| [internal/lidar/sweep](../../internal/lidar/sweep)         | `runner.go`     | `ComboResult` (32 fields)           | 🔶  | 🔶  | -   | Only `BestScore` persisted                                                                                                           |

---

## 8. Go data structures: comparison logic (no triggering endpoint)

| Folder                                                               | File                      | Function                  | DB  | Web | Mac | Notes                       |
| -------------------------------------------------------------------- | ------------------------- | ------------------------- | --- | --- | --- | --------------------------- |
| [internal/lidar/storage/sqlite](../../internal/lidar/storage/sqlite) | `analysis_run_compare.go` | `compareParams()`         | ✅  | 📋  | -   | Needs API endpoint          |
| [internal/lidar/storage/sqlite](../../internal/lidar/storage/sqlite) | `analysis_run_compare.go` | `computeTemporalIoU()`    | ✅  | 📋  | -   | Needs API endpoint          |
| [internal/lidar/storage/sqlite](../../internal/lidar/storage/sqlite) | `analysis_run_compare.go` | `is_split_candidate` flag | ✅  | ✅  | -   | Written but not triggerable |
| [internal/lidar/storage/sqlite](../../internal/lidar/storage/sqlite) | `analysis_run_compare.go` | `is_merge_candidate` flag | ✅  | ✅  | -   | Written but not triggerable |

---

## 9. Live track fields: fully wired (reference)

Fields that flow correctly from pipeline through all applicable surfaces.

| Folder                                                   | File          | Field                  | DB  | Web | Mac |
| -------------------------------------------------------- | ------------- | ---------------------- | --- | --- | --- |
| [internal/lidar/l5tracks](../../internal/lidar/l5tracks) | `tracking.go` | `track_id`             | ✅  | ✅  | ✅  |
| [internal/lidar/l5tracks](../../internal/lidar/l5tracks) | `tracking.go` | `sensor_id`            | ✅  | ✅  | ✅  |
| [internal/lidar/l5tracks](../../internal/lidar/l5tracks) | `tracking.go` | `track_state`          | ✅  | ✅  | ✅  |
| [internal/lidar/l5tracks](../../internal/lidar/l5tracks) | `tracking.go` | `position (x, y, z)`   | ✅  | ✅  | ✅  |
| [internal/lidar/l5tracks](../../internal/lidar/l5tracks) | `tracking.go` | `velocity (vx, vy)`    | ✅  | ✅  | ✅  |
| [internal/lidar/l5tracks](../../internal/lidar/l5tracks) | `tracking.go` | `speed_mps`            | ✅  | ✅  | ✅  |
| [internal/lidar/l5tracks](../../internal/lidar/l5tracks) | `tracking.go` | `heading_rad`          | ✅  | ✅  | ✅  |
| [internal/lidar/l5tracks](../../internal/lidar/l5tracks) | `tracking.go` | `observation_count`    | ✅  | ✅  | ✅  |
| [internal/lidar/l5tracks](../../internal/lidar/l5tracks) | `tracking.go` | `avg_speed_mps`        | ✅  | ✅  | ✅  |
| [internal/lidar/l5tracks](../../internal/lidar/l5tracks) | `tracking.go` | `max_speed_mps`        | ✅  | ✅  | ✅  |
| [internal/lidar/l5tracks](../../internal/lidar/l5tracks) | `tracking.go` | `bounding_box_*`       | ✅  | ✅  | ✅  |
| [internal/lidar/l5tracks](../../internal/lidar/l5tracks) | `tracking.go` | `height_p95_max`       | ✅  | ✅  | ✅  |
| [internal/lidar/l5tracks](../../internal/lidar/l5tracks) | `tracking.go` | `intensity_mean_avg`   | ✅  | ✅  | ✅  |
| [internal/lidar/l5tracks](../../internal/lidar/l5tracks) | `tracking.go` | `object_class`         | ✅  | ✅  | ✅  |
| [internal/lidar/l5tracks](../../internal/lidar/l5tracks) | `tracking.go` | `object_confidence`    | ✅  | ✅  | ✅  |
| [internal/lidar/l5tracks](../../internal/lidar/l5tracks) | `tracking.go` | `classification_model` | ✅  | ✅  | -   |
| [internal/lidar/l5tracks](../../internal/lidar/l5tracks) | `tracking.go` | `heading_source`       | -   | ✅  | ✅  |
| [internal/lidar/l5tracks](../../internal/lidar/l5tracks) | `tracking.go` | `track_length_meters`  | 🔶  | 📋  | ✅  |
| [internal/lidar/l5tracks](../../internal/lidar/l5tracks) | `tracking.go` | `track_duration_secs`  | 🔶  | 📋  | ✅  |
| [internal/lidar/l5tracks](../../internal/lidar/l5tracks) | `tracking.go` | `occlusion_count`      | 🔶  | 📋  | ✅  |
| [internal/lidar/l5tracks](../../internal/lidar/l5tracks) | `tracking.go` | `max_occlusion_frames` | 🔶  | 📋  | -   |
| [internal/lidar/l5tracks](../../internal/lidar/l5tracks) | `tracking.go` | `spatial_coverage`     | 🔶  | 📋  | -   |
| [internal/lidar/l5tracks](../../internal/lidar/l5tracks) | `tracking.go` | `noise_point_ratio`    | 🔶  | 📋  | -   |

---

## 10. Tuning parameters

| Folder                                   | File                   | Parameter Group          | DB  | Web | Mac |
| ---------------------------------------- | ---------------------- | ------------------------ | --- | --- | --- |
| [internal/config](../../internal/config) | `tuning.go`            | L3 Background (8 params) | ✅  | ✅  | -   |
| [internal/config](../../internal/config) | `tuning.go`            | L4 Perception (3 params) | ✅  | ✅  | -   |
| [internal/config](../../internal/config) | `tuning.go`            | L5 Tracker (14 params)   | ✅  | ✅  | -   |
| `config`                                 | `tuning.defaults.json` | Default values           | -   | -   | -   |

---

## 11. Go report pipeline reference

**Source:** [internal/report/](../../internal/report/)

This is a subsystem reference only. The trace matrix does not treat the Go
report pipeline as a DB/Web/Mac consumer surface.

| Package                                              | File                | Role                                          |
| ---------------------------------------------------- | ------------------- | --------------------------------------------- |
| [internal/report](../../internal/report)             | `typst_generate.go` | Direct DB query → `GeneratePDF(ctx, db, cfg)` |
| [internal/report/chart](../../internal/report/chart) | `timeseries.go`     | Speed percentile + count time-series SVG      |
| [internal/report/chart](../../internal/report/chart) | `histogram.go`      | Speed distribution histogram SVG              |
| [internal/report/typst](../../internal/report/typst) | `render.go`         | Typst templates + JSON data → `.pdf`          |

---

## 12. macOS visualiser: Swift surfaces

**Source:** [tools/visualiser-macos/VelocityVisualiser/](../../tools/visualiser-macos/VelocityVisualiser)

| Folder                                                 | File                                | Consumer                                    | DB  | Web | Mac |
| ------------------------------------------------------ | ----------------------------------- | ------------------------------------------- | --- | --- | --- |
| [tools/visualiser-macos](../../tools/visualiser-macos) | `VisualiserClient.swift`            | `StreamFrames` subscriber                   | -   | -   | ✅  |
| [tools/visualiser-macos](../../tools/visualiser-macos) | `VisualiserClient.swift`            | Playback controls (Pause/Play/Seek/SetRate) | -   | -   | ✅  |
| [tools/visualiser-macos](../../tools/visualiser-macos) | `VisualiserClient.swift`            | Overlay mode toggles                        | -   | -   | ✅  |
| [tools/visualiser-macos](../../tools/visualiser-macos) | `CompositePointCloudRenderer.swift` | Split-stream point cloud composition        | -   | -   | ✅  |
| [tools/visualiser-macos](../../tools/visualiser-macos) | `MetalRenderer.swift`               | Track boxes, trails, velocity vectors       | -   | -   | ✅  |
| [tools/visualiser-macos](../../tools/visualiser-macos) | `MetalRenderer.swift`               | Gating ellipses, residuals, predictions     | -   | -   | ✅  |

---

## 13. Classification pipeline: fully wired (reference)

**Go source:** [internal/lidar/l6objects/classification.go](../../internal/lidar/l6objects/classification.go): `TrackClassifier` (27 usages)

| Folder                                                     | File                | Component                 | DB  | Web | Mac |
| ---------------------------------------------------------- | ------------------- | ------------------------- | --- | --- | --- |
| [internal/lidar/l6objects](../../internal/lidar/l6objects) | `classification.go` | `ObjectClass` (8 classes) | ✅  | ✅  | ✅  |
| [internal/lidar/l6objects](../../internal/lidar/l6objects) | `classification.go` | `ClassificationResult`    | ✅  | ✅  | ✅  |
| [internal/lidar/l6objects](../../internal/lidar/l6objects) | `classification.go` | `ClassificationFeatures`  | -   | -   | 🔶  |
| [internal/lidar/l6objects](../../internal/lidar/l6objects) | `classification.go` | `TrackClassifier`         | -   | ✅  | ✅  |

**Notes:** `ObjectClass` + `ClassificationResult` (class + confidence) flow
through the full pipeline: tracker → DB → Web API → gRPC → Mac.
`ClassificationFeatures` (9 inputs) are only exposed via the gRPC
`classifyOrConvert()` replay path: visible to Mac during VRLOG playback
but never persisted. `TrackClassifier` is set on both WebServer and gRPC
server as a service object, not persisted data.

---

## 14. FrameBundle: macOS-only proto fields

**Proto:** [proto/velocity_visualiser/v1/visualiser.proto](../../proto/velocity_visualiser/v1/visualiser.proto): `FrameBundle`
**Consumer:** macOS Metal visualiser via `StreamFrames` gRPC stream (~30 fps)

These fields are **only** consumed by the macOS visualiser. They are not
persisted to SQLite, not exposed to the Web UI, and outside the Go
report pipeline.

| Field Group                | Field Count | macOS | DB  | Web |
| -------------------------- | ----------- | ----- | --- | --- |
| Point cloud (x/y/z/i/c)    | 7           | ✅    | -   | -   |
| Background snapshot (M3.5) | 6           | ✅    | -   | -   |
| Cluster OBB (7-DOF)        | 7           | ✅    | -   | -   |
| Track trails               | 3           | ✅    | -   | -   |
| Track alpha/opacity        | 1           | ✅    | -   | -   |
| Covariance 4×4             | 1           | ✅    | -   | -   |
| Debug: associations        | 4           | ✅    | -   | -   |
| Debug: gating ellipses     | 6           | ✅    | -   | -   |
| Debug: residuals           | 6           | ✅    | -   | -   |
| Debug: predictions         | 5           | ✅    | -   | -   |
| Playback info              | 9           | ✅    | -   | -   |
| Coordinate frame           | 6           | ✅    | -   | -   |

**Status:** These are intentionally Mac-only: they serve real-time
visualisation and debugging, not analysis or reporting. No wiring gap.

---

## 15. ECharts dashboard endpoints

**Go source:** `internal/lidar/server/chart_api.go` + `routes.go`
**Consumer:** Embedded ECharts dashboards (served from `/assets/*` via `go:embed`)

| Endpoint                      | Method | Data Source       | DB  | Web | Mac |
| ----------------------------- | ------ | ----------------- | --- | --- | --- |
| `/api/lidar/chart/polar`      | GET    | Background grid   | -   | ✅  | -   |
| `/api/lidar/chart/heatmap`    | GET    | Background grid   | -   | ✅  | -   |
| `/api/lidar/chart/foreground` | GET    | Foreground points | -   | ✅  | -   |
| `/api/lidar/chart/clusters`   | GET    | Cluster positions | -   | ✅  | -   |
| `/api/lidar/chart/traffic`    | GET    | Track activity    | -   | ✅  | -   |

**Notes:** These endpoints return ECharts-compatible JSON for the debug
dashboards at `/debug/lidar/*`. They are consumed by embedded HTML pages
(not the Svelte SPA). The Svelte frontend uses LayerChart for its own
charts (e.g. `RadarOverviewChart.svelte` consuming `/api/radar_stats`).

---

## 16. cmd/ entry points

| Binary                           | Location                                                                                         | Consumers                                                |
| -------------------------------- | ------------------------------------------------------------------------------------------------ | -------------------------------------------------------- |
| `velocity-report`                | [internal/cmd/server/radar.go](../../internal/cmd/server/radar.go)                               | Full server: API, DB, serial, LiDAR pipeline             |
| `velocity-sweep`                 | [internal/cmd/tune/sweep.go](../../internal/cmd/tune/sweep.go)                                   | LiDAR monitor, sweep engine, PCAP replay                 |
| `velocity-ctl`                   | [internal/cmd/device/main.go](../../internal/cmd/device/main.go)                                 | Device management: upgrade, rollback, backup             |
| `gen-vrlog`                      | [cmd/tools/gen-vrlog/main.go](../../cmd/tools/gen-vrlog/main.go)                                 | Synthetic VRLOG generation (no DB)                       |
| `vrlog-analyse`                  | [cmd/tools/vrlog-analyse/main.go](../../cmd/tools/vrlog-analyse/main.go)                         | VRLOG file analysis and comparison                       |
| `visualiser-server`              | [cmd/tools/visualiser-server/main.go](../../cmd/tools/visualiser-server/main.go)                 | Standalone gRPC (synthetic/replay/live)                  |
| `settling-eval`                  | [cmd/tools/settling-eval/main.go](../../cmd/tools/settling-eval/main.go)                         | Background grid settling evaluation                      |
| `pcap-split`                     | [cmd/tools/pcap-split/main.go](../../cmd/tools/pcap-split/main.go)                               | PCAP scan stats, motion timeline, and segmenting         |
| `lidar-bench`                    | [cmd/tools/lidar-bench/main.go](../../cmd/tools/lidar-bench/main.go)                             | L1–L6 pipeline perf-regression benchmark                 |
| `backfill_ring_elevations`       | [cmd/tools/backfill_ring_elevations/main.go](../../cmd/tools/backfill_ring_elevations/main.go)   | Backfill ring elevation data                             |
| `backfill_lidar_run_config`      | [cmd/tools/backfill_lidar_run_config/main.go](../../cmd/tools/backfill_lidar_run_config/main.go) | Backfill run config JSON onto historic LiDAR runs        |
| `config-migrate`                 | [cmd/tools/config-migrate/main.go](../../cmd/tools/config-migrate/main.go)                       | Migrate runtime config layouts                           |
| `config-validate`                | [cmd/tools/config-validate/main.go](../../cmd/tools/config-validate/main.go)                     | Validate runtime config files                            |
| `velocity lidar segments`        | [internal/cmd/lidar/segments.go](../../internal/cmd/lidar/segments.go)                           | Rank run or evidence windows (reads SQLite)              |
| `velocity lidar annotation-clip` | [internal/cmd/lidar/clip.go](../../internal/cmd/lidar/clip.go)                                   | Replay a selection and write its pack and segment record |

**Notes:** `velocity lidar segments` reads the selected evidence database. `velocity lidar annotation-clip` writes files, not SQLite. Other tools may write local databases; the table describes their primary purpose.

---

## 17. Speed percentile columns: resolved design debt

The per-track percentile columns have been removed from the active schema.
`lidar_tracks` and `lidar_run_tracks` no longer carry `p50_speed_mps`,
`p85_speed_mps`, or `p95_speed_mps`.

Per the [speed percentile alignment plan](../../docs/plans/speed-percentile-aggregation-alignment-plan.md)
(D-18), percentiles are reserved for grouped/report aggregates only. This
design debt is now retired in the live database schema.

---

## 18. Debug / admin routes

Embedded HTML dashboards and diagnostic endpoints. Not part of the
application API but served by the same HTTP servers.

### Radar server ([internal/api/server.go](../../internal/api/server.go))

| Route                     | Purpose                            |
| ------------------------- | ---------------------------------- |
| `/favicon.ico`            | Static favicon                     |
| `/app/*`                  | Svelte SPA (embedded or dev proxy) |
| `/`                       | Redirect to `/app/`                |
| `/debug/pprof/*`          | Go pprof profiling (via tsweb)     |
| `/debug/db-stats`         | Database statistics page           |
| `/debug/backup`           | Database backup download           |
| `/debug/tailsql/*`        | Interactive SQL query interface    |
| `/debug/send-command`     | Serial command form (HTML)         |
| `/debug/send-command-api` | Serial command POST endpoint       |
| `/debug/tail`             | SSE log tail                       |
| `/debug/tail.js`          | Debug dashboard JavaScript         |

### LiDAR server (`internal/lidar/server/routes.go`)

| Route                                       | Purpose                 |
| ------------------------------------------- | ----------------------- |
| `/debug/lidar`                              | Main debug dashboard    |
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

| Category                | Total | DB  | Web | Mac |
| ----------------------- | ----- | --- | --- | --- |
| HTTP endpoints (radar)  | 19    | 16  | 19  | 0   |
| HTTP endpoints (LiDAR)  | 100   | 60  | 86  | 11  |
| gRPC methods            | 9     | 0   | 0   | 9   |
| DB tables               | 44    | -   | 34  | 6   |
| Pipeline stages         | 13    | 5   | 5   | 2   |
| Tuning parameter groups | 3     | 3   | 3   | 0   |
| cmd/ entry points       | 15    | -   | -   | -   |
| Debug/admin routes      | 21    | -   | -   | -   |

### Gap summary

The pre-existing gap counts below come from the March consumer audit and have not been rechecked as part of the schema inventory refresh. The new segment and capture relationship findings are in the [SQLite model review](SCHEMA-REVIEW.md).

| Category                             | Count | Details                                                                                 |
| ------------------------------------ | ----- | --------------------------------------------------------------------------------------- |
| Schema columns never written         | 10    | `lidar_tracks` quality (6), `lidar_clusters` quality (3), `statistics_json` (1)         |
| Fields live-only (Mac but not in DB) | 3     | `track_length_meters`, `track_duration_secs`, `occlusion_count` (gRPC ✅, DB column 🔶) |
| Structs computed, not persisted      | 3     | NoiseCoverageMetrics, TrainingDatasetSummary, ClusterFeatures                           |
| Structs in-memory, classifier only   | 1     | TrackFeatures (20 features; computed on-demand, never stored)                           |
| Transient pipeline metrics           | 2     | FrameMetrics (HINT plan C1), per-track jitter                                           |
| Metrics with Web endpoint only       | 2     | TrackingMetrics + TrackAlignmentMetrics via `GET /api/lidar/tracks/metrics`             |
| Logic with no triggering endpoint    | 2     | `compareParams()`, `computeTemporalIoU()`                                               |
| Deprecated columns (removal landed)  | 0     | Per-track speed percentile columns are no longer present in the active schema           |
