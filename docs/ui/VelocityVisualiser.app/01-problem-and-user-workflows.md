# Problem Statement and User Workflows

Status: Active
Purpose/Summary: 01-problem-and-user-workflows.

## 1. Problem Statement

### Why LidarView Is Insufficient

The current LiDAR pipeline forwards foreground points to **LidarView** (Hesai's visualisation tool) on port 2370 via `internal/lidar/network/foreground_forwarder.go`. While useful for basic point cloud inspection, LidarView has significant limitations for **tracking development and debugging**:

| Limitation                     | Impact                                                                        |
| ------------------------------ | ----------------------------------------------------------------------------- |
| **No object overlays**         | Cannot render bounding boxes, cluster hulls, or track IDs                     |
| **No velocity visualisation**  | Cannot display speed vectors, heading arrows, or motion trails                |
| **No track lifecycle display** | Cannot distinguish tentative/confirmed/deleted tracks                         |
| **No debug artifacts**         | Cannot visualise association candidates, gating ellipses, or Kalman residuals |
| **No labelling workflow**      | Cannot annotate tracks for classifier training                                |
| **No replay controls**         | Cannot seek, pause, or single-step through recorded data                      |
| **No deterministic replay**    | Cannot reproduce exact frame/track sequences for regression tests             |
| **Packet-level encoding**      | Loses semantic information (tracks, clusters) in Pandar40P packet format      |

### Why a macOS-Native Visualiser

- **Primary development platform**: The project maintainer works on macOS with Apple Silicon
- **GPU performance**: Metal provides efficient point cloud rendering (100k+ points at 60fps)
- **Low-latency debugging**: Local gRPC connection avoids network jitter
- **Integration with Swift tooling**: Native UI controls, smooth animations, Retina support
- **Standalone app**: No browser overhead, direct GPU access, file system integration

### What We Need

A **dedicated 3D visualisation tool** that:

1. Renders point clouds, clusters, and tracks with rich overlays
2. Supports both live streaming and deterministic replay
3. Exposes debug artifacts for algorithm tuning
4. Enables track labelling for classifier training
5. Consumes a **stable, versioned API** from the Go pipeline

---

## 2. User Workflows

### Workflow A: Live Debugging of Tracking

**Scenario**: Developer is tuning association gating thresholds and wants to see why tracks are being missed or merged.

**Steps**:

1. Start the Go radar service with gRPC streaming enabled
2. Launch macOS visualiser, connect to `localhost:50051`
3. Enable debug overlays: "Show Gating Ellipses", "Show Association Lines", "Show Residuals"
4. Watch live point cloud with overlayed:
   - Green circles: confirmed tracks
   - Yellow circles: tentative tracks
   - Dashed lines: association candidates
   - Coloured ellipses: Mahalanobis gating thresholds
   - Red segments: innovation residuals
5. Adjust parameters via web UI (existing `/api/lidar/background/params` endpoint)
6. Observe immediate effect in visualiser

**Performance targets**:

- **Frame rate**: 10-20 Hz (match sensor rotation rate, variable based on motor speed)
- **Latency**: <100ms end-to-end (sensor → pipeline → viewer)
- **Point count**: up to 70,000 points/frame (full Pandar40P rotation)

---

### Workflow B: Offline Replay and Inspection

**Scenario**: A track split was reported in the field. Developer wants to reproduce the exact sequence for debugging.

**Steps**:

1. Obtain recorded log file (protobuf-encoded frames + tracks)
2. Launch visualiser in replay mode, open log file
3. Timeline scrubber shows frame timestamps
4. Seek to reported timestamp
5. Single-step frame-by-frame (keyboard: `[` and `]`)
6. Toggle overlay modes to inspect association decisions
7. Compare with LidarView output (run both in parallel from same log)

**Determinism requirements**:

- **Same input → same output**: Identical tracks/IDs every replay
- **No runtime randomness** in pipeline (seeded RNG if needed)
- **Timestamp-based ordering**: Frames sorted by capture time, not arrival time

---

### Workflow C: Labelling/Classification Iteration Loop

**Scenario**: Training a classifier to distinguish pedestrians from vehicles. Need labelled track segments.

**Steps**:

1. Open recorded log in visualiser
2. Play through scene, pause when object of interest appears
3. Click on track to select it (highlights trail, shows track ID)
4. Press `L` or use label panel to assign class:
   - "pedestrian", "car", "cyclist", "bird", "other"
5. Optionally mark track segment boundaries (start/end frames)
6. Labels stored in Go backend SQLite database via REST API (`POST /api/lidar/labels`)
   - Shared with web UI for cross-platform access
7. Export labels as JSON for training pipeline (via `GET /api/lidar/labels/export`):
   ```json
   {
     "track_id": "track_42",
     "class_label": "pedestrian",
     "start_timestamp_ns": 1234567890000000,
     "end_timestamp_ns": 1234567891000000
   }
   ```
8. Re-run classifier with new training data
9. Visualise classification results overlaid on point cloud

**UX targets**:

- **Quick labelling**: <3 seconds per track annotation
- **Keyboard shortcuts**: `1-9` for common classes
- **Undo support**: `Cmd+Z` to revert last label
- **Export to JSON/CSV**: Shareable with ML pipeline

---

## 3. Goals

### Must-Have (MVP)

- [ ] Live point cloud streaming at 10-20 Hz (variable motor speed)
- [ ] Cluster bounding boxes (AABB)
- [ ] Track IDs and lifecycle states (colours)
- [ ] Velocity vectors (arrows)
- [ ] Track trails (fading polylines)
- [ ] Pause/play/seek controls
- [ ] Playback rate adjustment (0.25x – 2x)
- [ ] Connection status indicator
- [ ] Overlay toggles (points, boxes, trails, vectors)

### Should-Have (V1.0)

- [ ] Oriented bounding boxes (OBB)
- [ ] Debug overlays (gating, association, residuals)
- [ ] Track selection and detail panel
- [ ] Basic labelling (class assignment)
- [ ] Label export to JSON
- [ ] Recording from live stream to local file

### Nice-to-Have (V1.x)

- [ ] Heat map of track density
- [ ] Multi-sensor support (multiple gRPC connections)
- [ ] 3D camera orbit controls
- [ ] Screenshot/video export
- [ ] Dark/light theme

---

## 4. Non-Goals (Guardrails)

**DO NOT**:

| Avoided Scope                             | Rationale                                         |
| ----------------------------------------- | ------------------------------------------------- |
| Full SLAM / global mapping / loop closure | Out of scope for static sensor deployment         |
| Cloud services, auth, accounts, telemetry | Privacy-first design; local-only                  |
| End-to-end pipeline rewrite               | Incremental refactor behind interfaces            |
| Complex ML training UI                    | External ML pipeline; visualiser is for labelling |
| Proprietary SDKs or paid dependencies     | Open-source first                                 |
| Remote networking beyond localhost        | Security and simplicity; future extension only    |
| Web-based visualiser                      | Performance and integration constraints           |

**PRESERVE**:

- Existing LidarView forwarding path (unchanged, regression oracle)
- Current REST API (`/api/lidar/tracks`, `/api/lidar/clusters`, etc.)
- SQLite database schema
- Current tracking algorithm logic (refactor behind interfaces, not rewrite)

---

## 5. Performance Targets

| Metric               | Target     | Rationale                      |
| -------------------- | ---------- | ------------------------------ |
| **Render FPS**       | ≥30 fps    | Smooth animation for debugging |
| **Pipeline latency** | <100ms     | Near-real-time feedback        |
| **Max points/frame** | 100,000    | Full rotation + margin         |
| **Max tracks**       | 200        | Street scene with many objects |
| **Memory usage**     | <500 MB    | Reasonable for laptop          |
| **GPU VRAM**         | <1 GB      | Integrated GPU support         |
| **Startup time**     | <2 seconds | Fast iteration                 |
| **Replay seek**      | <500ms     | Interactive timeline scrubbing |

---

## 6. UX Targets

### Must-Have Controls

| Control                | Interaction            | Shortcut        |
| ---------------------- | ---------------------- | --------------- |
| Connect/Disconnect     | Button                 | `Cmd+Shift+C`   |
| Pause/Play             | Button + Space         | `Space`         |
| Seek                   | Timeline slider + drag | Arrow keys      |
| Playback rate          | Stepper (0.25x – 2x)   | `[` / `]`       |
| Frame step             | Buttons                | `,` / `.`       |
| Toggle: Points         | Checkbox               | `P`             |
| Toggle: Boxes          | Checkbox               | `B`             |
| Toggle: Trails         | Checkbox               | `T`             |
| Toggle: Velocity       | Checkbox               | `V`             |
| Toggle: Debug overlays | Checkbox               | `D`             |
| Reset camera           | Button                 | `R`             |
| Zoom                   | Scroll/pinch           | Mouse wheel     |
| Pan                    | Drag                   | Two-finger drag |
| Rotate                 | Option+drag            | Option+drag     |

### Minimal Labelling UX

| Action        | Interaction         | Shortcut    |
| ------------- | ------------------- | ----------- |
| Select track  | Click on track      | Click       |
| Assign class  | Dropdown / keyboard | `1-5`       |
| Clear label   | Context menu        | `Backspace` |
| Undo          | Menu / keyboard     | `Cmd+Z`     |
| Export labels | Menu                | `Cmd+E`     |

---

## 7. Related Documents

- [02-api-contracts.md](./02-api-contracts.md) – API contract between pipeline and visualiser
- [03-architecture.md](./03-architecture.md) – System architecture and module design
- [04-implementation-plan.md](./04-implementation-plan.md) – Incremental delivery milestones
- [../../lidar/troubleshooting/01-tracking-upgrades.md](../../lidar/troubleshooting/01-tracking-upgrades.md) – Tracking algorithm improvements
