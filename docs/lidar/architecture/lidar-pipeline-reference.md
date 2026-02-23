# LiDAR Pipeline Reference

Complete reference for the velocity.report LiDAR processing pipeline: data flow, component inventory, production deployment architecture, and ML pipeline design.

---

## Current Data Flow

```
PCAP/Live UDP → Parse → Frame → Background → Foreground → Cluster → Track → Classify → API
                                                                                 ↓
                                                                          JSON/CSV Export
                                                                                 ↓
                                                                        Training Data Blobs
```

## Existing Components

| Component             | Location                                           | Status      |
| --------------------- | -------------------------------------------------- | ----------- |
| PCAP Reader           | `internal/lidar/l1packets/network/pcap.go`         | ✅ Complete |
| Frame Builder         | `internal/lidar/l2frames/frame_builder.go`         | ✅ Complete |
| Background Manager    | `internal/lidar/l3grid/background.go`              | ✅ Complete |
| Foreground Extraction | `internal/lidar/l3grid/foreground.go`              | ✅ Complete |
| DBSCAN Clustering     | `internal/lidar/l4perception/cluster.go`           | ✅ Complete |
| Kalman Tracking       | `internal/lidar/l5tracks/tracking.go`              | ✅ Complete |
| Rule-Based Classifier | `internal/lidar/l6objects/classification.go`       | ✅ Complete |
| Track Store           | `internal/lidar/storage/sqlite/track_store.go`     | ✅ Complete |
| REST API              | `internal/lidar/monitor/track_api.go`              | ✅ Complete |
| PCAP Analyse Tool     | `cmd/tools/pcap-analyze/main.go`                   | ✅ Complete |
| Training Data Export  | `internal/lidar/adapters/training_data.go`         | ✅ Complete |
| Analysis Run Store    | `internal/lidar/storage/sqlite/analysis_run.go`    | ✅ Complete |
| Sweep Runner          | `internal/lidar/sweep/runner.go`                   | ✅ Complete |
| Auto-Tuner            | `internal/lidar/sweep/auto.go`                     | ✅ Complete |
| Sweep Scoring         | `internal/lidar/sweep/scoring.go`                  | ✅ Complete |
| Sweep Dashboard       | `internal/lidar/monitor/html/sweep_dashboard.html` | ✅ Complete |
| Hungarian Solver      | `internal/lidar/l5tracks/hungarian.go`             | ✅ Complete |
| Ground Removal        | `internal/lidar/l4perception/ground.go`            | ✅ Complete |
| OBB Estimation        | `internal/lidar/l4perception/obb.go`               | ✅ Complete |
| Debug Collector       | `internal/lidar/debug/collector.go`                | ✅ Complete |

## Production Deployment Architecture (Phase 4.3)

```
┌─────────────────────────────────────────────────────────────────┐
│                      Edge Node (Raspberry Pi)                   │
│                                                                 │
│  [UDP:2369] → [LIDAR Pipeline] → [Local SQLite] → [REST API]   │
│                      ↓                   ↓                      │
│                [ML Classifier]    [Training Data]               │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
                                 ↓
                        [Data Consolidation]
                                 ↓
┌─────────────────────────────────────────────────────────────────┐
│                      Central Server                             │
│                                                                 │
│  [Consolidated DB] → [Labelling UI] → [Model Training]         │
│                           ↓              ↓                      │
│                    [Labelled Tracks] → [New Model]              │
│                                           ↓                     │
│                              [Model Distribution]               │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

**Model Update Flow:**

1. Collect labelled tracks from labelling UI
2. Train new model version
3. Evaluate on validation set
4. If metrics improve: version the model, distribute to edge nodes, update
   `classification_model` field in new tracks
5. Monitor production metrics → collect new edge cases → repeat

## Complete ML Pipeline Data Flow

```
┌──────────────────────────────────────────────────────────────────────────┐
│                         COMPLETE ML PIPELINE                            │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌─────────┐    ┌───────────┐    ┌──────────┐    ┌──────────────────┐   │
│  │  PCAP   │───→│   Parse   │───→│  Frame   │───→│    Background    │   │
│  │  Live   │    │           │    │ Builder  │    │    Subtraction   │   │
│  └─────────┘    └───────────┘    └──────────┘    └────────┬─────────┘   │
│                                                           │             │
│                                                           ▼             │
│  ┌─────────────────┐    ┌──────────┐    ┌──────────────────────────┐    │
│  │   Foreground    │◄───│  Mask    │◄───│   ProcessFramePolarWith  │    │
│  │     Points      │    │          │    │         Mask()           │    │
│  └────────┬────────┘    └──────────┘    └──────────────────────────┘    │
│           │                                                             │
│           ▼                                                             │
│  ┌─────────────────┐    ┌──────────────────┐    ┌──────────────────┐    │
│  │  TransformTo    │───→│     DBSCAN       │───→│     Tracker      │    │
│  │    World()      │    │   Clustering     │    │    Update()      │    │
│  └─────────────────┘    └──────────────────┘    └────────┬─────────┘    │
│                                                          │              │
│                                                          ▼              │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                     ANALYSIS RUN (Phase 3.7)                    │    │
│  │                                                                 │    │
│  │   ┌─────────────┐    ┌────────────────┐    ┌─────────────────┐  │    │
│  │   │  params_json│    │  lidar_run_    │    │ Split/Merge     │  │    │
│  │   │   (all cfg) │    │    tracks      │    │   Detection     │  │    │
│  │   └─────────────┘    └────────────────┘    └─────────────────┘  │    │
│  │                                                                 │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                    │                                    │
│                                    ▼                                    │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                    LABELLING UI (Phase 4.0)                     │    │
│  │                                                                 │    │
│  │   ┌─────────────┐    ┌────────────────┐    ┌─────────────────┐  │    │
│  │   │   Track     │    │    Label       │    │   Quality       │  │    │
│  │   │  Browser    │───→│   Assignment   │───→│   Marking       │  │    │
│  │   └─────────────┘    └────────────────┘    └─────────────────┘  │    │
│  │                                                                 │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                    │                                    │
│                                    ▼                                    │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                  ML TRAINING (Phase 4.1)                        │    │
│  │                                                                 │    │
│  │   ┌─────────────┐    ┌────────────────┐    ┌─────────────────┐  │    │
│  │   │  Feature    │    │    Model       │    │   Deployed      │  │    │
│  │   │ Extraction  │───→│   Training     │───→│    Model        │  │    │
│  │   └─────────────┘    └────────────────┘    └─────────────────┘  │    │
│  │                                                                 │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                    │                                    │
│                                    ▼                                    │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │               PARAMETER TUNING (Phase 4.2)                      │    │
│  │                                                                 │    │
│  │   ┌─────────────┐    ┌────────────────┐    ┌─────────────────┐  │    │
│  │   │  Parameter  │    │   Run          │    │   Optimal       │  │    │
│  │   │   Grid      │───→│  Comparison    │───→│  Parameters     │  │    │
│  │   └─────────────┘    └────────────────┘    └─────────────────┘  │    │
│  │                                                                 │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```
