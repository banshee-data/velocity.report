# Product Vision

Status: Draft
Purpose: Defines the long-term product direction for velocity.report, guiding backlog pruning and prioritisation decisions.

---

## 1. Mission

Help neighbourhood change-makers measure and report on street-level vehicle behaviour — with no cameras, no licence plates, and no personally identifiable information. Measurements stay local, the user owns the data, and reports are compelling enough to drive policy change.

## 2. End-State Goal

A deployment on a residential street produces:

1. **A professional PDF report** on vehicle speeds, volumes, and behaviour — suitable for submission to a local authority or community meeting.
2. **A queryable database of transits** with a description interface that exposes dynamically generated statistics (driving styles, outlier behaviour, distance to cyclist, percentage coming to complete stop, peak-hour profiles).

Both outputs draw from a fused scene built from radar and LiDAR data.

## 3. Sensor Fusion Architecture

### 3.1 Radar Feeds

The OmniPreSense OPS243 sensor provides three complementary data feeds:

| Feed | OPS243 Command | Description | Current Status |
|------|----------------|-------------|----------------|
| **Speed / magnitude** | `OS`, `OM` | Doppler speed and signal strength per detection | ✅ Ingested (`radar_data`) |
| **Objects** | `OJ` | Sessionised object events with classifier, duration, speed envelope, length estimate | ✅ Ingested (`radar_objects`) |
| **FFT** | `OF` (Doppler) / `of` (FMCW) | Frequency-domain spectrum — enables multi-target separation and signature analysis | ⬜ Command allowed; ingestion not implemented |

All three feeds should be ingested simultaneously so that a single vehicle pass yields:

- a speed trace (magnitude over time),
- an object record (start, end, peak speed, classifier),
- an FFT signature (spectral shape for multi-target disambiguation).

The fused radar record is the **primary speed measurement** for every transit.

### 3.2 LiDAR Feeds

The LiDAR pipeline (L1–L6) progressively adds spatial context:

| Capability | Pipeline Layer | Description | Current Status |
|-----------|----------------|-------------|----------------|
| **Detection & clustering** | L3 grid → L4 perception | Foreground extraction, DBSCAN clustering, OBB geometry | ✅ Implemented |
| **Tracking** | L5 tracks | Kalman-filtered multi-frame identity, speed profile, trail | ✅ Implemented |
| **Classification** | L6 objects | Category, size, vehicle class (rule-based; ML planned) | ✅ Rule-based; ML deferred |
| **Long-track speed profile** | L5 tracks | Per-observation speed, heading, and bounding box over the full transit | ✅ Stored in `lidar_track_obs` |

As LiDAR matures, it contributes:

- **Spatial track** — position trail through the scene.
- **Speed profile** — per-frame speed over the full transit, not just peak/min.
- **Classification** — object category, physical size, and vehicle class.
- **Proximity measurements** — distance between tracked objects (e.g. vehicle-to-cyclist gap).

### 3.3 Fused Scene

A **scene** combines both sensor feeds for a given street segment:

```
Radar feeds (speed, objects, FFT)
        ↘
          Fused Transit Record  →  PDF Report
        ↗                      →  Description Interface
LiDAR feeds (tracks, trails, classification)
```

Fusion associates a radar transit with a LiDAR track by temporal overlap and directional consistency. The fused transit record carries:

- **Speed authority**: radar Doppler (higher accuracy at range).
- **Spatial authority**: LiDAR track trail and bounding box.
- **Classification authority**: LiDAR geometry + radar FFT signature.

When only one sensor is available the record degrades gracefully — radar-only deployments still produce speed reports; LiDAR-only deployments still produce spatial tracks.

## 4. Storage Strategy

### 4.1 Principle: No Long-Term Point Clouds

Raw point clouds are ephemeral processing inputs. They are **never stored beyond the current analysis run**. Long-term storage holds only:

| Data | Representation | Storage |
|------|---------------|---------|
| **Radar events** | JSON (`raw_event`) | `radar_data` |
| **Radar objects** | JSON (classifier, speed envelope, duration) | `radar_objects` |
| **Radar transits** | Aggregate (speed, magnitude, point count) | `radar_data_transits` |
| **LiDAR tracks** | Summary statistics (speed percentiles, bbox, class) | `lidar_tracks` |
| **LiDAR observations** | Per-frame (x, y, z, vx, vy, speed, heading, bbox) | `lidar_track_obs` |
| **Fused transits** | Combined record with sensor provenance | ⬜ Schema not yet defined |

### 4.2 Polyline Vector Scene

For replay and visualisation the stored artefact is a **polyline vector scene** — a compact set of 2D trails with per-vertex timestamps and bounding box dimensions:

```
Trail = [(x, y, ts, heading, length, width), ...]
```

This enables:

- **Replay** of bounding boxes moving through space — no point cloud needed.
- **Minimal storage** — a 30-second transit at 10 Hz is ~300 vertices (~7 KB uncompressed).
- **Low replay compute** — no clustering, no background subtraction, just polyline interpolation.

The existing `lidar_track_obs` table already stores the required per-frame data. The vector scene is a read-time projection, not a separate stored artefact.

## 5. Track Description Language

A **Track Description Language (TDL)** provides a textual query interface over the fused transit database — an abstract schema, a JSON filter API, and an optional DSL for report templates and CLI queries.

Full design: [Track Description Language and Description Interface plan](plans/data-track-description-language-plan.md).

## 6. Reporting

### 6.1 PDF Report

The existing Python PDF generator (`tools/pdf-generator/`) produces professional street-speed reports. The vision extends it to:

- **Pull from the fused transit schema** rather than raw radar tables alone.
- **Include LiDAR-derived metrics** when available (classification breakdown, speed profiles, close-pass counts).
- **Accept TDL filter parameters** to scope the report (date range, direction, time-of-day).
- **Generate comparison sections** (before/after intervention, weekday/weekend, peak/off-peak).

### 6.2 Description Interface

A web-based interface over the transit database for browsing, aggregating, replaying, and exporting transit data.

Full design: [Track Description Language and Description Interface plan](plans/data-track-description-language-plan.md).

## 7. Backlog Alignment

This vision document should inform prioritisation in `BACKLOG.md`:

| Vision pillar | Supports | Deprioritises |
|---------------|----------|---------------|
| **Radar feed expansion** (§3.1) | FFT ingestion, multi-feed simultaneous capture | Features unrelated to sensor data quality |
| **LiDAR maturation** (§3.2) | ML classifier, track labelling QC, sweep system polish | Cosmetic visualiser features without tracking value |
| **Sensor fusion** (§3.3) | Fused transit schema, temporal association logic | Single-sensor features that duplicate fused capabilities |
| **Storage minimalism** (§4) | Polyline vector scene, point-cloud ephemeral policy | Long-term point-cloud storage, large BLOB tables |
| **Track Description Language** (§5) | Abstract transit schema, JSON filter API, aggregation endpoints — [design doc](plans/data-track-description-language-plan.md) | Raw-SQL user interfaces, ad-hoc query endpoints |
| **PDF reporting** (§6.1) | Fused-data report templates, TDL-scoped reports | Report features that only use radar data |
| **Description interface** (§6.2) | Transit browser, aggregate stats, vector replay — [design doc](plans/data-track-description-language-plan.md) | Heavy 3D visualisation in production (development-only) |

## 8. Phasing

| Phase | Focus | Depends On |
|-------|-------|------------|
| **A — Radar completeness** | Ingest FFT data; fuse speed + object + FFT into a single transit record | Existing radar infrastructure |
| **B — Fused transit schema** | Define the fused transit table/view joining radar and LiDAR; expose via API | Phase A + existing LiDAR track storage |
| **C — JSON filter API** | Build the filter/aggregation endpoint over the fused schema; wire to web UI | Phase B |
| **D — TDL and description interface** | Transit browser, aggregate statistics, vector-scene replay — [design doc](plans/data-track-description-language-plan.md) | Phase C |
| **E — Fused PDF reports** | Extend PDF generator to pull from fused schema with TDL filters | Phase C |
| **F — Advanced queries** | Close-pass analysis, driving style classification, stop compliance | Phase D + LiDAR classification maturity |
