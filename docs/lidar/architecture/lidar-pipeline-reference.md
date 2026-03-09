# LiDAR Pipeline Reference

Complete reference for the velocity.report LiDAR processing pipeline: data flow, component inventory, production deployment architecture, and the metrics-first data science boundaries around tuning and future classification work.

---

## Current Data Flow

```
PCAP/Live UDP → Parse → Frame → Background → Foreground → Cluster → Track → Classify → API
                                                                                 ↓
                                                                  JSON/CSV Export + Labelled Runs
                                                                                 ↓
                                                                    Scorecards / Replay Benchmarks
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
| Research Data Export  | `internal/lidar/adapters/training_data.go`         | ✅ Complete |
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
│        [Rule-Based + Tunable L6]  [Runs / Labels / Metrics]    │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
                                 ↓
                   [Replay Packs / Consolidated Analysis]
                                 ↓
┌─────────────────────────────────────────────────────────────────┐
│               Offline Analysis / Research Workstation           │
│                                                                 │
│  [Reference Runs] → [Scorecards / Threshold Studies]            │
│                           ↓                                     │
│             [Optional Classification Research]                  │
│                           ↓                                     │
│     [Deploy Only If It Beats Transparent Baseline]              │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

**Metrics and Threshold Update Flow:**

1. Collect labelled tracks from labelling UI
2. Re-run the same scenes with explicit parameter bundles
3. Compare scorecards: detection, fragmentation, false positives, velocity
   coverage, and stability
4. Version the winning thresholds/params and document the metric deltas
5. Monitor production metrics → collect new edge cases → repeat

**Optional classification research** may use the same labelled runs and exported
features, but it is not on the critical path. Any candidate model must beat the
current rule-based baseline on fixed replay packs before deployment is even
considered.

## Metrics-First Data Science and Optional Classification Flow

```
┌─────────────────────────────────────────────────────────────────────────┐
│                   METRICS-FIRST DATA SCIENCE WORKFLOW                   │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌─────────┐    ┌───────────┐    ┌──────────┐    ┌──────────────────┐   │
│  │  PCAP   │───→│   Parse   │───→│  Frame   │───→│    Background    │   │
│  │  Live   │    │           │    │ Builder  │    │    Subtraction   │   │
│  └─────────┘    └───────────┘    └──────────┘    └────────┬─────────┘   │
│                                                           │             │
│                                                           ▼             │
│  ┌─────────────────┐    ┌──────────────────┐    ┌──────────────────┐    │
│  │   Foreground    │───→│     DBSCAN       │───→│     Tracker      │    │
│  │     Points      │    │   Clustering     │    │    Update()      │    │
│  └─────────────────┘    └──────────────────┘    └────────┬─────────┘    │
│                                                          │              │
│                                                          ▼              │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │              Rule-Based Classify (L6)                            │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                    │                                    │
│                                    ▼                                    │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                     ANALYSIS RUN                                │    │
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
│  │                    LABELLING UI                                 │    │
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
│  │               SCORECARDS / REPLAY BENCHMARKS                    │    │
│  │                                                                 │    │
│  │   ┌─────────────┐    ┌────────────────┐    ┌─────────────────┐  │    │
│  │   │  Threshold  │    │   Parameter    │    │  Report Metric  │  │    │
│  │   │   Studies   │    │    Tuning      │    │   Validation    │  │    │
│  │   └─────────────┘    └────────────────┘    └─────────────────┘  │    │
│  │                                                                 │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                    │                                    │
│                                    ▼                                    │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │           OPTIONAL CLASSIFICATION RESEARCH                      │    │
│  │                                                                 │    │
│  │   Deploy only if benchmark wins are reproducible & explainable  │    │
│  │                                                                 │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```
