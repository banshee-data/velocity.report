# Product Roadmap and Milestones

## Status: Active

## Summary

Unified product roadmap mapping velocity.report features to versioned release
milestones (v0.5 → v2.0). Includes a decision matrix explaining placement
rationale, a dependency graph, a future-compatibility strategy for online
geometry-prior services, and the detailed LiDAR ML pipeline phases.

## Related Documents

| Document | Scope |
|----------|-------|
| [BACKLOG.md](../BACKLOG.md) | Priority-ordered work queue (single source of truth) |
| [CHANGELOG.md](../CHANGELOG.md) | Released feature history (v0.1.0 – v0.4.0) |
| [Vector-Scene Map Architecture](lidar/architecture/vector-scene-map.md) | Geometry-prior data model |
| [Ground-Plane Vector-Scene Maths](maths/proposals/20260221-ground-plane-vector-scene-maths.md) | Maths for piecewise planar tiles and prior weighting |
| [Vector vs Velocity Workstreams](lidar/architecture/20260221-vector-vs-velocity-workstreams.md) | Production vector-grid vs future velocity-coherent extraction |

---

## 1. Current State (v0.5.0-pre11)

**Released:** v0.4.0 (2026-01-29). In development: v0.5.0-pre11.

### What Ships Today

| Capability | Status | Component |
|------------|--------|-----------|
| Radar vehicle detection (OPS243A) | Production | Go server |
| Real-time speed dashboard | Production | Svelte web |
| Professional PDF reports | Production | Python/LaTeX |
| Comparison reports (before/after) | Production | Go + Python |
| Site configuration (SCD Type 6) | Production | Go + SQLite |
| LiDAR background subtraction | Experimental | Go server |
| LiDAR foreground tracking | Experimental | Go server |
| Adaptive region segmentation | Experimental | Go server |
| Parameter sweep / auto-tune | Experimental | Go server |
| PCAP analysis mode | Experimental | Go server |
| macOS 3D visualiser (Metal) | Experimental | Swift app |
| Track labelling + VRLOG replay | Experimental | Swift + Go |

### Architecture Snapshot

```
Sensors ──► Go Server ──► SQLite ──► Web Frontend
  │              │                      PDF Generator
  │              │                      macOS Visualiser (gRPC)
  Radar (serial) │
  LiDAR (UDP)    └── REST API (:8080)
```

### LiDAR Pipeline Status

The LiDAR subsystem has completed Phases 3.7–3.9 of the ML pipeline roadmap.
See [Appendix A](#appendix-a-lidar-ml-pipeline-phases) for detailed phase
specifications.

| Phase | Description | Status |
|-------|-------------|--------|
| 3.7 | Analysis Run Infrastructure | ✅ Implemented |
| 3.8 | Tracking Upgrades (Hungarian, OBB, ground removal) | ✅ Implemented |
| 3.9 | Adaptive Regions & Sweep System | ✅ Implemented |
| 4.0 | Track Labelling UI | Planned |
| 4.1 | ML Classifier Training | Planned |
| 4.2 | Parameter Tuning & Optimisation | Planned |
| 4.3 | Production Deployment | Planned |

---

## 2. Release Milestones

### v0.5 — Platform Hardening (current cycle)

**Theme:** Stabilise the build, test, and deployment surface.

| Feature | Plan Reference | Effort |
|---------|---------------|--------|
| SWEEP/HINT platform hardening (Phase 5–6) | [sweep-hint-mode](plans/lidar-sweep-hint-mode-plan.md) | M |
| Settling optimisation Phase 3 | BACKLOG P1 | M |
| Python venv consolidation (.venv/ at root) | [tooling-python-venv](plans/tooling-python-venv-consolidation-plan.md) | S |
| Documentation standardisation (metadata) | [platform-docs](plans/platform-documentation-standardization-plan.md) | S |

**Exit criteria:** All P1 backlog items either completed or promoted to v0.6.

---

### v0.6 — Deployment & Packaging

**Theme:** Make velocity.report installable by a non-developer.

| Feature | Plan Reference | Effort |
|---------|---------------|--------|
| Precompiled LaTeX (800 MB → 60 MB) | [pdf-latex-precompiled](plans/pdf-latex-precompiled-format-plan.md) | M |
| Single `velocity-report` binary + subcommands | [deploy-distribution](plans/deploy-distribution-packaging-plan.md) Phase 1–2 | L |
| GitHub Releases CI pipeline | [deploy-distribution](plans/deploy-distribution-packaging-plan.md) Phase 3 | M |
| One-line install script | [deploy-distribution](plans/deploy-distribution-packaging-plan.md) Phase 4 | S |
| Raspberry Pi imager pipeline (pi-gen) | [deploy-rpi-imager](plans/deploy-rpi-imager-fork-plan.md) | L |
| Platform simplification & deprecation (Phase 1) | [platform-simplification](plans/platform-simplification-and-deprecation-plan.md) | S |

**Exit criteria:** A user can flash an SD card, boot a Raspberry Pi, and see
live speed data in a browser within 15 minutes. GitHub Releases publishes
checksummed artefacts on tag push.

---

### v0.7 — Unified Frontend & Radar Polish

**Theme:** One port, one app, one navigation. Radar side is production-grade.

| Feature | Plan Reference | Effort |
|---------|---------------|--------|
| Frontend consolidation (Phases 0–5) | [web-frontend](plans/web-frontend-consolidation-plan.md) | L |
| Retire port 8081 | [web-frontend](plans/web-frontend-consolidation-plan.md) Phase 5 | S |
| Transit deduplication | BACKLOG P2 | M |
| SQLite client standardisation | [data-sqlite](plans/data-sqlite-client-standardization-plan.md) | M |
| Accessibility testing pass | BACKLOG P2 | S |
| Widescreen content containment | BACKLOG P2 | S |
| Profile comparison system (cross-run evaluation) | BACKLOG P2 | M |

**Exit criteria:** Single Svelte app on `:8080` with conditional LiDAR sections.
All radar-path SQL routed through canonical DB layer. WCAG 2.1 AA for core
dashboard flows.

---

### v1.0 — Production-Ready Release

**Theme:** velocity.report is a reliable, documented, privacy-first traffic
monitoring system that a neighbourhood group can deploy and trust.

| Feature | Plan Reference | Effort |
|---------|---------------|--------|
| Test coverage ≥ 95.5% across all components | [platform-quality](plans/platform-quality-coverage-improvement-plan.md) | L |
| Platform simplification complete (all phases) | [platform-simplification](plans/platform-simplification-and-deprecation-plan.md) | M |
| LiDAR foundations fix-it (doc truth, runtime config) | [lidar-foundations](plans/lidar-architecture-foundations-fixit-plan.md) | M |
| Unified settling (L3/L4 SettlementCore) | [maths: unify-l3-l4](maths/proposals/20260219-unify-l3-l4-settling.md) | L |
| LaTeX palette cross-reference | BACKLOG P2 | S |
| Time-partitioned raw data tables | BACKLOG P2 | M |
| Geometry-prior local file format (GeoJSON) | [vector-scene-map](lidar/architecture/vector-scene-map.md) | M |
| Data export (CSV, GeoJSON) | — | M |
| Stable public API with versioned endpoints | — | M |

**v1.0 contract:**

1. **Radar is production-grade.** All radar features are stable, tested, and documented.
2. **LiDAR is graduated experimental.** Core pipeline (L1–L5) works with
   known limitations documented. Advanced features (QC programme, ML
   classifier) are post-v1.0.
3. **Privacy guarantee.** No PII, no cameras, no cloud. Local-only by default.
4. **Install in 15 minutes.** Raspberry Pi image or one-line install.
5. **Geometry priors are local files.** GeoJSON vector-scene maps loaded from
   disk, with a schema designed for future online retrieval (§5).

---

### v2.0 — Advanced Perception & Connected Features

**Theme:** LiDAR perception matures; optional connectivity for community
features while preserving local-only as the default.

| Feature | Plan Reference | Effort |
|---------|---------------|--------|
| Visualiser QC programme (Features 1–10) | [qc-overview](plans/lidar-visualiser-labelling-qc-enhancements-overview-plan.md) | XL |
| ML classifier training pipeline (Phase 4.1) | [Appendix A § Phase 4.1](#phase-41-ml-classifier-training) | L |
| Parameter tuning optimisation (Phase 4.2) | [Appendix A § Phase 4.2](#phase-42-parameter-tuning--optimisation) | L |
| Ground-plane vector-scene maths (piecewise planar tiles) | [maths: ground-plane](maths/proposals/20260221-ground-plane-vector-scene-maths.md) | L |
| Velocity-coherent foreground extraction | [maths: velocity-coherent](maths/proposals/20260220-velocity-coherent-foreground-extraction.md) | L |
| Dynamic algorithm selection (A/B foreground modes) | [lidar-dynamic-algo](plans/lidar-architecture-dynamic-algorithm-selection-plan.md) | M |
| Online geometry-prior service (opt-in) | §5 below | L |
| Multi-location aggregate dashboard (privacy-preserving) | — | L |
| Threshold-based speed alerts (local notification) | — | M |
| Peak-hour and seasonal trend analysis | — | M |

**v2.0 principles:**

1. **Local-only remains the default.** Online features are opt-in, never required.
2. **LiDAR perception is production-ready.** QC programme complete, ML
   classifier available, vector-scene maps operational.
3. **Community features respect privacy.** Aggregate statistics only (no
   individual transits) may be shared with consent.

---

### Research / Deferred (post-v2.0)

These features are tracked in the backlog at P3 but have no target milestone.

| Feature | Reason Deferred |
|---------|----------------|
| AV dataset integration (28-class taxonomy, Parquet) | Targets a different user base (AV research, not neighbourhoods) |
| Motion capture architecture (7DOF moving sensors) | Requires hardware not in current deployment model |
| Static pose alignment (7DOF bounding boxes) | Dependent on motion capture; AV-focused |
| AV range-image format alignment | Low urgency for traffic monitoring use case |
| Visual regression testing | Valuable but not user-facing |
| E2E test infrastructure | Valuable but not user-facing |
| Frontend background/debug surfaces (Swift visualiser) | Niche developer tooling |

---

## 3. Decision Matrix

Features are placed into milestones using five weighted criteria. Each scores
0–3 (none / low / medium / high). The placement threshold per milestone is
shown below.

### Criteria Definitions

| Criterion | Weight | Description |
|-----------|--------|-------------|
| **User value** | 3× | Direct benefit to the target user (neighbourhood change-makers) |
| **Privacy alignment** | 2× | Maintains or strengthens the privacy-first guarantee |
| **Dependency unlock** | 2× | Enables or unblocks other high-value features |
| **Effort** | 1× (inverse) | Smaller effort scores higher (3 = S, 2 = M, 1 = L, 0 = XL) |
| **Platform maturity** | 1× | Reduces tech debt, improves reliability, or simplifies operations |

### Milestone Placement Thresholds

| Milestone | Minimum Weighted Score | Rationale |
|-----------|----------------------|-----------|
| v0.5 | ≥ 18 | Highest-impact stabilisation work already in progress |
| v0.6 | ≥ 16 | Deployment blockers that gate user adoption |
| v0.7 | ≥ 14 | Frontend and data-layer polish for v1.0 readiness |
| v1.0 | ≥ 12 | Everything needed for "production-ready" contract |
| v2.0 | ≥ 8 | Advanced features, connected capabilities, research graduation |
| Deferred | < 8 | Speculative, targets different users, or prerequisite missing |

### Scored Examples

| Feature | User Value (3×) | Privacy (2×) | Dep. Unlock (2×) | Effort (1×) | Platform (1×) | Total | Milestone |
|---------|-----------------|-------------|-------------------|-------------|---------------|-------|-----------|
| RPi imager pipeline | 3 (9) | 3 (6) | 3 (6) | 1 (1) | 2 (2) | **24** | v0.6 |
| Precompiled LaTeX | 2 (6) | 2 (4) | 3 (6) | 2 (2) | 3 (3) | **21** | v0.6 |
| Frontend consolidation | 3 (9) | 2 (4) | 2 (4) | 1 (1) | 3 (3) | **21** | v0.7 |
| Single binary + subcommands | 3 (9) | 2 (4) | 2 (4) | 1 (1) | 3 (3) | **21** | v0.6 |
| SWEEP/HINT hardening | 1 (3) | 2 (4) | 3 (6) | 2 (2) | 3 (3) | **18** | v0.5 |
| Python venv consolidation | 1 (3) | 2 (4) | 2 (4) | 3 (3) | 3 (3) | **17** | v0.5 |
| SQLite client standardisation | 2 (6) | 2 (4) | 2 (4) | 2 (2) | 3 (3) | **19** | v0.7 |
| Coverage ≥ 95.5% | 1 (3) | 2 (4) | 1 (2) | 1 (1) | 3 (3) | **13** | v1.0 |
| Unified settling (L3/L4) | 1 (3) | 2 (4) | 3 (6) | 1 (1) | 2 (2) | **16** | v1.0 |
| Geometry-prior local file | 2 (6) | 3 (6) | 2 (4) | 2 (2) | 1 (1) | **19** | v1.0 |
| Visualiser QC programme | 2 (6) | 2 (4) | 1 (2) | 0 (0) | 2 (2) | **14** | v2.0 |
| ML classifier pipeline | 2 (6) | 2 (4) | 2 (4) | 1 (1) | 1 (1) | **16** | v2.0 |
| Online geometry-prior service | 2 (6) | 1 (2) | 2 (4) | 1 (1) | 1 (1) | **14** | v2.0 |
| AV dataset integration | 0 (0) | 2 (4) | 0 (0) | 0 (0) | 1 (1) | **5** | Deferred |
| Motion capture architecture | 0 (0) | 2 (4) | 0 (0) | 0 (0) | 1 (1) | **5** | Deferred |

### Key Placement Rationale

**Why RPi imager is v0.6, not v0.5:**
The imager depends on precompiled LaTeX (image size) and the single binary
(packaging). These prerequisites must land first. v0.5 stabilises the platform
so v0.6 can focus on packaging without chasing regressions.

**Why frontend consolidation is v0.7, not v0.6:**
Phase 3 (ECharts → LayerChart rewrite for 8 charts) requires ~2–3 weeks alone.
Shipping the deployment story (v0.6) first ensures early adopters can install
the system; the unified frontend is polish for v1.0 readiness.

**Why geometry-prior local file is v1.0, not earlier:**
The vector-scene map schema must be stable before the online service (v2.0) can
adopt it. Shipping the local file format in v1.0 locks the data contract. The
maths are well-defined ([ground-plane-vector-scene
proposal](maths/proposals/20260221-ground-plane-vector-scene-maths.md)) but
runtime integration depends on unified settling (also v1.0).

**Why QC programme is v2.0, not v1.0:**
The QC programme spans 10 features with cross-dependencies (M1–M5 milestones
internally). It targets the LiDAR labelling workflow, which is experimental in
v1.0. Shipping v1.0 without QC keeps the release scope achievable.

**Why ML classifier is v2.0:**
Requires labelled training data generated by the QC programme. Also
dependent on Phase 4.1 of the LiDAR ML pipeline, which sequences after track
labelling (Phase 4.0, already complete) and parameter tuning (Phase 4.2).

---

## 4. Dependency Graph

```
v0.5 (Platform Hardening)
  │
  ├── SWEEP/HINT hardening ──────────────────────────────┐
  ├── Settling optimisation Phase 3 ──────────────────────┤
  ├── Python venv consolidation                           │
  └── Documentation standardisation                       │
                                                          │
v0.6 (Deployment & Packaging)                             │
  │                                                       │
  ├── Precompiled LaTeX ─────────────────► RPi imager ◄───┘
  ├── Single binary + subcommands ─────► GitHub Releases
  ├── One-line install script
  └── Platform simplification Phase 1
        │
v0.7 (Unified Frontend)
  │
  ├── Frontend consolidation (Phases 0–5)
  │     └── Retire port 8081
  ├── SQLite client standardisation
  ├── Transit deduplication
  ├── Profile comparison system
  └── Accessibility + widescreen polish
        │
v1.0 (Production-Ready)
  │
  ├── Coverage ≥ 95.5%
  ├── Platform simplification complete
  ├── LiDAR foundations fix-it
  ├── Unified settling (L3/L4) ──────────────────────────┐
  ├── Geometry-prior local file (GeoJSON) ◄──────────────┘
  ├── Data export (CSV, GeoJSON)
  ├── Time-partitioned raw data tables
  └── Stable public API (versioned)
        │
v2.0 (Advanced Perception & Connected)
  │
  ├── Visualiser QC programme ────► ML classifier pipeline
  ├── Ground-plane vector-scene maths
  ├── Velocity-coherent extraction
  ├── Dynamic algorithm selection
  ├── Online geometry-prior service ◄── local file schema (v1.0)
  ├── Multi-location aggregate dashboard
  ├── Speed threshold alerts
  └── Peak-hour / seasonal trend analysis
```

**Critical path to v1.0:**
v0.5 (stabilise) → v0.6 (package) → v0.7 (unify frontend) → v1.0 (harden).
LaTeX precompilation is the single most important dependency: it unblocks the
RPi image, which unblocks non-developer adoption.

---

## 5. Future Online Geometry-Prior Service

### Design Goal

Enable a community-maintained online service that provides geometry priors
(ground polygons, boundary polylines, structure features) for known deployment
locations — while keeping velocity.report fully functional offline.

### Architecture: Local-First with Optional Online Fetch

```
                         ┌────────────────────────────┐
                         │  Online Prior Service (v2+) │
                         │  (community-maintained)     │
                         │                             │
                         │  GET /priors?lat=X&lon=Y    │
                         │  → GeoJSON FeatureCollection│
                         └──────────┬─────────────────┘
                                    │ opt-in fetch
                                    ▼
┌─────────────┐      ┌──────────────────────────┐
│ Local Prior  │─────►│  Prior Loader             │
│ File (v1.0)  │      │  (reads local or remote)  │
│ .geojson     │      │                           │
└─────────────┘      └──────────┬───────────────┘
                                │ w_prior weights
                                ▼
                     ┌──────────────────────────┐
                     │  Ground-Plane Estimator   │
                     │  (L4 Perception)          │
                     │  Region scoring: §4.4     │
                     │  S_R(p) = ... × w_prior   │
                     └──────────────────────────┘
```

### Future-Compatibility Strategy (What We Build Now)

The following design choices in v1.0 ensure the online service is additive, not
a rewrite:

| Decision (v1.0) | Future Benefit (v2.0+) |
|------------------|----------------------|
| **GeoJSON file format** for local priors | Same schema served by HTTP endpoint; no format conversion needed |
| **Prior weights are advisory (0–1)**, not hard constraints | Service can return confidence-weighted priors; client applies them identically to local files |
| **Prior Loader abstraction** separates file I/O from perception maths | Swap file reader for HTTP client behind the same interface |
| **Sensor-local coordinate system** (no GPS required) | GPS is additive: if present, enables location-based prior lookup; if absent, local files still work |
| **Privacy by default** | Online fetch is opt-in; no location data transmitted without explicit user consent |
| **Schema includes `source` field** (`local`, `community`, `survey`) | Provenance tracking from day one; online priors carry their own trust metadata |

### Online Service Specification (v2.0 Scope)

**API contract (draft):**

```
GET /api/v1/priors?lat={lat}&lon={lon}&radius={m}
Accept: application/geo+json

Response: GeoJSON FeatureCollection
  features[]:
    geometry: Polygon | LineString | Point
    properties:
      class: "ground" | "structure" | "volume"
      confidence: 0.0–1.0
      source: "community" | "survey" | "lidar-derived"
      updated_at: ISO 8601
      contributor: <anonymous hash>  # no PII
```

**Privacy safeguards:**

1. Location queries use coarsened coordinates (100 m grid snapping) to prevent
   precise deployment location disclosure.
2. No authentication required for read access (public data).
3. Contributions are anonymised (hash-based contributor IDs, no accounts).
4. All prior data is geometry only — no speed, transit, or vehicle data.

**Deployment options:**

- Community-hosted instance (e.g., `priors.velocity.report`)
- Self-hosted by municipalities or research groups
- Static file hosting (GeoJSON files on any CDN)

### Implementation Phases

| Phase | Milestone | Scope |
|-------|-----------|-------|
| **5a** | v1.0 | Define GeoJSON schema for local prior files |
| **5b** | v1.0 | Implement Prior Loader with file-system backend |
| **5c** | v1.0 | Wire `w_prior` weights into ground-plane region scoring |
| **5d** | v2.0 | Add HTTP backend to Prior Loader (opt-in config flag) |
| **5e** | v2.0 | Build community submission API (anonymous, geometry-only) |
| **5f** | v2.0 | Ship self-hostable prior-service container image |

---

## 6. Timeline Estimate

Estimates assume a single part-time contributor. Dates are indicative, not
commitments.

| Milestone | Target | Duration | Key Dependency |
|-----------|--------|----------|---------------|
| **v0.5** | Q1 2026 | 4–6 weeks | — |
| **v0.6** | Q2 2026 | 6–10 weeks | Precompiled LaTeX unblocks RPi image |
| **v0.7** | Q3 2026 | 6–8 weeks | Frontend consolidation Phase 3 dominates |
| **v1.0** | Q4 2026 | 8–12 weeks | Coverage, unified settling, API stability |
| **v2.0** | 2027 H1 | 16–24 weeks | QC programme, ML pipeline, online priors |

---

## 7. Principles for Milestone Decisions

1. **Ship the install story early.** Users cannot evaluate the product if they
   cannot install it. Deployment and packaging (v0.6) takes priority over UI
   polish (v0.7) and test coverage (v1.0).

2. **Stabilise before expanding.** Each milestone hardens the layer below
   before building the layer above. v0.5 stabilises internals; v0.6 packages
   them; v0.7 polishes the interface; v1.0 certifies quality.

3. **Privacy is a feature, not a constraint.** Every milestone must maintain
   the privacy guarantee. Online features (v2.0) are opt-in and transmit
   geometry only.

4. **Local-only is the default forever.** The online geometry-prior service
   (v2.0) enriches the system but is never required. A disconnected Raspberry
   Pi with local prior files must produce the same quality results.

5. **Defer what targets different users.** AV dataset integration, motion
   capture, and range-image formats serve autonomous-vehicle researchers, not
   neighbourhood change-makers. These remain deferred until the core product
   is mature.

6. **Score, don't guess.** The decision matrix (§3) provides a repeatable
   framework. When a new feature request arrives, score it against the five
   criteria and place it in the milestone whose threshold it meets.

---

## Appendix A: LiDAR ML Pipeline Phases

This appendix preserves the detailed LiDAR ML pipeline specification
previously maintained as a standalone roadmap. Phases 3.7–3.9 are implemented;
Phases 4.0–4.3 are planned and map to v2.0 milestones above.

Workstream note (2026-02-21): velocity-coherent foreground extraction remains
planned future work and is tracked separately from the production vector-grid
baseline. See
[`docs/lidar/architecture/20260221-vector-vs-velocity-workstreams.md`](lidar/architecture/20260221-vector-vs-velocity-workstreams.md).

### Current LiDAR Data Flow

```
PCAP/Live UDP → Parse → Frame → Background → Foreground → Cluster → Track → Classify → API
                                                                                 ↓
                                                                          JSON/CSV Export
                                                                                 ↓
                                                                        Training Data Blobs
```

### Existing Components

| Component              | Location                                           | Status      |
| ---------------------- | -------------------------------------------------- | ----------- |
| PCAP Reader            | `internal/lidar/l1packets/network/pcap.go`         | ✅ Complete |
| Frame Builder          | `internal/lidar/l2frames/frame_builder.go`         | ✅ Complete |
| Background Manager     | `internal/lidar/l3grid/background.go`              | ✅ Complete |
| Foreground Extraction  | `internal/lidar/l3grid/foreground.go`              | ✅ Complete |
| DBSCAN Clustering      | `internal/lidar/l4perception/cluster.go`           | ✅ Complete |
| Kalman Tracking        | `internal/lidar/l5tracks/tracking.go`              | ✅ Complete |
| Rule-Based Classifier  | `internal/lidar/l6objects/classification.go`       | ✅ Complete |
| Track Store            | `internal/lidar/storage/sqlite/track_store.go`     | ✅ Complete |
| REST API               | `internal/lidar/monitor/track_api.go`              | ✅ Complete |
| PCAP Analyse Tool      | `cmd/tools/pcap-analyze/main.go`                   | ✅ Complete |
| Training Data Export   | `internal/lidar/adapters/training_data.go`         | ✅ Complete |
| Analysis Run Store     | `internal/lidar/storage/sqlite/analysis_run.go`    | ✅ Complete |
| Sweep Runner           | `internal/lidar/sweep/runner.go`                   | ✅ Complete |
| Auto-Tuner             | `internal/lidar/sweep/auto.go`                     | ✅ Complete |
| Sweep Scoring          | `internal/lidar/sweep/scoring.go`                  | ✅ Complete |
| Sweep Dashboard        | `internal/lidar/monitor/html/sweep_dashboard.html` | ✅ Complete |
| Hungarian Solver       | `internal/lidar/l5tracks/hungarian.go`             | ✅ Complete |
| Ground Removal         | `internal/lidar/l4perception/ground.go`            | ✅ Complete |
| OBB Estimation         | `internal/lidar/l4perception/obb.go`               | ✅ Complete |
| Debug Collector        | `internal/lidar/debug/collector.go`                | ✅ Complete |

---

### Phase 3.7: Analysis Run Infrastructure ✅ IMPLEMENTED

**Objective:** Enable reproducible analysis runs with versioned parameter
configurations, allowing comparison across runs with different parameters and
detection of track splits/merges.

#### Implementation Files

| File                                                              | Description                        |
| ----------------------------------------------------------------- | ---------------------------------- |
| `internal/lidar/analysis_run.go`                                  | Core types and database operations |
| `internal/lidar/analysis_run_test.go`                             | Unit tests                         |
| `internal/db/migrations/000010_create_lidar_analysis_runs.up.sql` | Database migration                 |
| `internal/db/schema.sql`                                          | Updated with analysis run tables   |

#### 3.7.1: Analysis Run Schema

```sql
-- Analysis runs with full parameter configuration
CREATE TABLE IF NOT EXISTS lidar_analysis_runs (
    run_id TEXT PRIMARY KEY,              -- UUID or timestamp-based ID
    created_at INTEGER NOT NULL,          -- Unix timestamp
    source_type TEXT NOT NULL,            -- 'pcap' or 'live'
    source_path TEXT,                     -- PCAP file path (if applicable)
    sensor_id TEXT NOT NULL,

    -- Full parameter configuration as JSON
    params_json TEXT NOT NULL,            -- All LIDAR params in single JSON blob

    -- Run statistics
    duration_secs REAL,
    total_frames INTEGER,
    total_clusters INTEGER,
    total_tracks INTEGER,
    confirmed_tracks INTEGER,

    -- Processing metadata
    processing_time_ms INTEGER,
    status TEXT DEFAULT 'running',        -- 'running', 'completed', 'failed'
    error_message TEXT,

    -- Comparison metadata
    parent_run_id TEXT,                   -- For parameter tuning comparisons
    notes TEXT                            -- User notes about this run
);

CREATE INDEX idx_runs_created ON lidar_analysis_runs(created_at);
CREATE INDEX idx_runs_source ON lidar_analysis_runs(source_path);
CREATE INDEX idx_runs_parent ON lidar_analysis_runs(parent_run_id);

-- Track results per run (extends lidar_tracks with run_id)
CREATE TABLE IF NOT EXISTS lidar_run_tracks (
    run_id TEXT NOT NULL,
    track_id TEXT NOT NULL,

    -- All track fields from lidar_tracks
    sensor_id TEXT NOT NULL,
    track_state TEXT NOT NULL,
    start_unix_nanos INTEGER NOT NULL,
    end_unix_nanos INTEGER,
    observation_count INTEGER,
    avg_speed_mps REAL,
    peak_speed_mps REAL,
    p50_speed_mps REAL,
    p85_speed_mps REAL,
    p95_speed_mps REAL,
    bounding_box_length_avg REAL,
    bounding_box_width_avg REAL,
    bounding_box_height_avg REAL,
    height_p95_max REAL,
    intensity_mean_avg REAL,

    -- Classification (rule-based or ML)
    object_class TEXT,
    object_confidence REAL,
    classification_model TEXT,

    -- User labels (for training)
    user_label TEXT,                      -- Human-assigned label
    label_confidence REAL,                -- Annotator confidence
    labeler_id TEXT,                      -- Who labeled this
    labeled_at INTEGER,                   -- When labeled

    -- Track quality flags
    is_split_candidate INTEGER DEFAULT 0,   -- Suspected split
    is_merge_candidate INTEGER DEFAULT 0,   -- Suspected merge
    linked_track_ids TEXT,                  -- JSON array of related track IDs

    PRIMARY KEY (run_id, track_id),
    FOREIGN KEY (run_id) REFERENCES lidar_analysis_runs(run_id) ON DELETE CASCADE
);

CREATE INDEX idx_run_tracks_run ON lidar_run_tracks(run_id);
CREATE INDEX idx_run_tracks_class ON lidar_run_tracks(object_class);
CREATE INDEX idx_run_tracks_label ON lidar_run_tracks(user_label);
```

#### 3.7.2: Params JSON Structure

All LiDAR parameters stored in a single JSON blob for complete reproducibility:

```json
{
  "version": "1.0",
  "timestamp": "2025-12-01T00:00:00Z",

  "background": {
    "background_update_fraction": 0.02,
    "closeness_sensitivity_multiplier": 3.0,
    "safety_margin_meters": 0.5,
    "neighbor_confirmation_count": 3,
    "noise_relative_fraction": 0.315,
    "seed_from_first_observation": true,
    "freeze_duration_nanos": 5000000000
  },

  "clustering": {
    "eps": 0.6,
    "min_pts": 12,
    "cell_size": 0.6
  },

  "tracking": {
    "max_tracks": 100,
    "max_misses": 3,
    "hits_to_confirm": 3,
    "gating_distance_squared": 25.0,
    "process_noise": [0.1, 0.1, 0.5, 0.5],
    "measurement_noise": [0.2, 0.2],
    "deleted_track_grace_period_nanos": 5000000000
  },

  "classification": {
    "model_type": "rule_based",
    "thresholds": {
      "pedestrian": {
        "height_min": 1.0,
        "height_max": 2.0,
        "speed_max": 3.0
      },
      "car": {
        "height_min": 1.2,
        "length_min": 3.0,
        "speed_min": 5.0
      },
      "bird": {
        "height_max": 0.5,
        "speed_max": 1.0
      }
    }
  }
}
```

#### 3.7.3: Go Implementation ✅

The following types and functions are implemented in `internal/lidar/analysis_run.go`:

```go
// AnalysisRun represents a complete analysis session with parameters
type AnalysisRun struct {
    RunID           string            `json:"run_id"`
    CreatedAt       time.Time         `json:"created_at"`
    SourceType      string            `json:"source_type"` // "pcap" or "live"
    SourcePath      string            `json:"source_path,omitempty"`
    SensorID        string            `json:"sensor_id"`
    ParamsJSON      json.RawMessage   `json:"params_json"`
    DurationSecs    float64           `json:"duration_secs"`
    TotalFrames     int               `json:"total_frames"`
    TotalClusters   int               `json:"total_clusters"`
    TotalTracks     int               `json:"total_tracks"`
    ConfirmedTracks int               `json:"confirmed_tracks"`
    ProcessingTimeMs int64            `json:"processing_time_ms"`
    Status          string            `json:"status"`
    ParentRunID     string            `json:"parent_run_id,omitempty"`
    Notes           string            `json:"notes,omitempty"`
}

// RunParams captures all configurable parameters for reproducibility
type RunParams struct {
    Version        string                     `json:"version"`
    Timestamp      time.Time                  `json:"timestamp"`
    Background     BackgroundParamsExport     `json:"background"`
    Clustering     ClusteringParamsExport     `json:"clustering"`
    Tracking       TrackingParamsExport       `json:"tracking"`
    Classification ClassificationParamsExport `json:"classification"`
}

// AnalysisRunStore provides database operations for analysis runs
type AnalysisRunStore struct { ... }
func NewAnalysisRunStore(db *sql.DB) *AnalysisRunStore
func (s *AnalysisRunStore) InsertRun(run *AnalysisRun) error
func (s *AnalysisRunStore) CompleteRun(runID string, stats *AnalysisStats) error
func (s *AnalysisRunStore) GetRun(runID string) (*AnalysisRun, error)
func (s *AnalysisRunStore) ListRuns(limit int) ([]*AnalysisRun, error)
func (s *AnalysisRunStore) InsertRunTrack(track *RunTrack) error
func (s *AnalysisRunStore) GetRunTracks(runID string) ([]*RunTrack, error)
func (s *AnalysisRunStore) UpdateTrackLabel(...) error
func (s *AnalysisRunStore) GetLabelingProgress(runID string) (total, labeled int, byClass map[string]int, err error)
func (s *AnalysisRunStore) GetUnlabeledTracks(runID string, limit int) ([]*RunTrack, error)
```

#### 3.7.4: Track Split/Merge Detection

Types for detecting when parameter changes cause tracks to split or merge:

```go
// RunComparison shows differences between two analysis runs
type RunComparison struct {
    Run1ID          string           `json:"run1_id"`
    Run2ID          string           `json:"run2_id"`
    ParamDiff       map[string]any   `json:"param_diff"`
    TracksOnlyRun1  []string         `json:"tracks_only_run1"`
    TracksOnlyRun2  []string         `json:"tracks_only_run2"`
    SplitCandidates []TrackSplit     `json:"split_candidates"`
    MergeCandidates []TrackMerge     `json:"merge_candidates"`
    MatchedTracks   []TrackMatch     `json:"matched_tracks"`
}

type TrackSplit struct {
    OriginalTrack  string   `json:"original_track"`
    SplitTracks    []string `json:"split_tracks"`
    SplitX, SplitY float32  `json:"split_location"`
    Confidence     float32  `json:"confidence"`
}

type TrackMerge struct {
    MergedTrack    string   `json:"merged_track"`
    SourceTracks   []string `json:"source_tracks"`
    MergeX, MergeY float32  `json:"merge_location"`
    Confidence     float32  `json:"confidence"`
}

// Future: DetectSplitsMerges compares spatiotemporal overlap between runs
// Algorithm:
// 1. Build spatial-temporal index for each run
// 2. For each track in run1, find overlapping tracks in run2
// 3. If one track maps to multiple: potential split
// 4. If multiple tracks map to one: potential merge
// 5. Compute confidence based on overlap percentage
```

---

### Phase 4.0: Track Labelling UI

**Objective:** Provide a web-based interface for human annotators to label
tracks, review classifications, and mark quality issues.

#### 4.0.1: UI Requirements

**Core Features:**

1. **Track Browser:** List and filter tracks by run, class, time range
2. **Track Viewer:** Visualise track trajectory on 2D map
3. **Labelling Interface:** Assign class labels with confidence
4. **Quality Marking:** Flag splits, merges, noise tracks
5. **Bulk Actions:** Apply labels to multiple similar tracks
6. **Progress Tracking:** Show annotation completion status

#### 4.0.2: UI Architecture

Using the existing SvelteKit frontend (`web/`):

```
web/src/routes/
├── lidar/
│   ├── +page.svelte              # Dashboard with run list
│   ├── runs/
│   │   ├── +page.svelte          # Analysis run browser
│   │   └── [run_id]/
│   │       ├── +page.svelte      # Run details with track list
│   │       └── tracks/
│   │           └── [track_id]/
│   │               └── +page.svelte  # Individual track viewer
│   ├── labeling/
│   │   ├── +page.svelte          # Labeling queue interface
│   │   └── +page.server.ts       # Server-side data loading
│   └── compare/
│       └── +page.svelte          # Run comparison tool
```

#### 4.0.3: UI Components (svelte-ux based)

```svelte
<!-- web/src/lib/components/lidar/TrackViewer.svelte -->
<script lang="ts">
  import { Canvas } from 'svelte-ux';
  import type { Track, TrackObservation } from '$lib/types/lidar';

  export let track: Track;
  export let observations: TrackObservation[];

  // Render track trajectory as path
  // Show velocity vectors
  // Highlight classification features
</script>

<!-- web/src/lib/components/lidar/LabelingPanel.svelte -->
<script lang="ts">
  import { Select, Button, TextField } from 'svelte-ux';

  export let track: Track;
  export let onLabel: (label: string, confidence: number) => void;

  const classOptions = [
    { value: 'pedestrian', label: 'Pedestrian' },
    { value: 'car', label: 'Car' },
    { value: 'cyclist', label: 'Cyclist' },
    { value: 'bird', label: 'Bird' },
    { value: 'noise', label: 'Noise/Artifact' },
    { value: 'other', label: 'Other' }
  ];
</script>

<!-- web/src/lib/components/lidar/RunComparisonView.svelte -->
<script lang="ts">
  // Side-by-side comparison of two runs
  // Highlight split/merge candidates
  // Show parameter differences
</script>
```

#### 4.0.4: REST API Extensions

```go
// Additional endpoints for labeling UI

// Label a track
// PUT /api/lidar/runs/{run_id}/tracks/{track_id}/label
type LabelRequest struct {
    UserLabel       string  `json:"user_label"`
    LabelConfidence float32 `json:"label_confidence"`
    LabelerID       string  `json:"labeler_id"`
    IsSplitCandidate bool   `json:"is_split_candidate,omitempty"`
    IsMergeCandidate bool   `json:"is_merge_candidate,omitempty"`
    LinkedTrackIDs   []string `json:"linked_track_ids,omitempty"`
    Notes           string  `json:"notes,omitempty"`
}

// Get labeling progress
// GET /api/lidar/runs/{run_id}/labeling-progress
type LabelingProgress struct {
    TotalTracks     int     `json:"total_tracks"`
    LabeledTracks   int     `json:"labeled_tracks"`
    UnlabeledTracks int     `json:"unlabeled_tracks"`
    ByClass         map[string]int `json:"by_class"`
    ByLabeler       map[string]int `json:"by_labeler"`
}

// Get tracks needing review (unlabeled or low confidence)
// GET /api/lidar/runs/{run_id}/review-queue
type ReviewQueueParams struct {
    MinConfidence float32 `query:"min_confidence"`
    Class         string  `query:"class"`
    Limit         int     `query:"limit"`
}

// Compare two runs
// GET /api/lidar/runs/compare?run1={id}&run2={id}
// Returns: RunComparison
```

---

### Phase 4.1: ML Classifier Training

**Objective:** Train an ML model to replace rule-based classification using
labeled track data.

#### 4.1.1: Training Data Pipeline

```
Labeled Tracks (DB) → Feature Extraction → Training Dataset → Model Training → Model Deployment
```

#### 4.1.2: Feature Vector

Extract features from labeled tracks for ML training:

```python
# tools/ml-training/features.py

class TrackFeatures:
    """Feature vector for track classification"""

    # Spatial features (shape)
    bounding_box_length_avg: float
    bounding_box_width_avg: float
    bounding_box_height_avg: float
    height_p95_max: float
    aspect_ratio_xy: float  # length/width
    aspect_ratio_xz: float  # length/height

    # Kinematic features (motion)
    avg_speed_mps: float
    peak_speed_mps: float
    p50_speed_mps: float
    p85_speed_mps: float
    p95_speed_mps: float
    speed_variance: float
    acceleration_max: float
    heading_variance: float

    # Temporal features
    duration_secs: float
    observation_count: int
    observations_per_second: float

    # Intensity features
    intensity_mean_avg: float
    intensity_variance: float

    @classmethod
    def from_track(cls, track: dict) -> 'TrackFeatures':
        """Extract features from track dictionary"""
        return cls(
            bounding_box_length_avg=track['bounding_box_length_avg'],
            bounding_box_width_avg=track['bounding_box_width_avg'],
            # ... etc
        )

    def to_vector(self) -> np.ndarray:
        """Convert to numpy array for model input"""
        return np.array([
            self.bounding_box_length_avg,
            self.bounding_box_width_avg,
            # ... normalized features
        ])
```

#### 4.1.3: Model Training Script

```python
# tools/ml-training/train_classifier.py

import sqlite3
import numpy as np
from sklearn.ensemble import RandomForestClassifier
from sklearn.model_selection import cross_val_score
import joblib

def load_labeled_tracks(db_path: str, min_confidence: float = 0.7) -> tuple:
    """Load labeled tracks from database"""
    conn = sqlite3.connect(db_path)
    query = """
        SELECT * FROM lidar_run_tracks
        WHERE user_label IS NOT NULL
        AND label_confidence >= ?
    """
    tracks = pd.read_sql(query, conn, params=[min_confidence])
    conn.close()

    # Extract features
    X = np.array([TrackFeatures.from_track(t).to_vector() for t in tracks.to_dict('records')])
    y = tracks['user_label'].values

    return X, y, tracks

def train_model(X, y, model_type='random_forest'):
    """Train classification model"""
    if model_type == 'random_forest':
        model = RandomForestClassifier(
            n_estimators=100,
            max_depth=10,
            min_samples_split=5,
            class_weight='balanced'
        )

    # Cross-validation
    scores = cross_val_score(model, X, y, cv=5, scoring='f1_weighted')
    print(f"Cross-validation F1: {scores.mean():.3f} (+/- {scores.std():.3f})")

    # Train final model
    model.fit(X, y)

    return model

def export_model(model, output_path: str, version: str):
    """Export model for deployment"""
    metadata = {
        'version': version,
        'feature_names': TrackFeatures.feature_names(),
        'class_names': model.classes_.tolist(),
        'created_at': datetime.now().isoformat()
    }

    joblib.dump({
        'model': model,
        'metadata': metadata
    }, output_path)

if __name__ == '__main__':
    X, y, tracks = load_labeled_tracks('sensor_data.db')
    model = train_model(X, y)
    export_model(model, 'models/track_classifier_v1.joblib', 'v1.0')
```

#### 4.1.4: Model Deployment in Go

```go
// internal/lidar/ml_classifier.go

// MLClassifier wraps a trained model for track classification
type MLClassifier struct {
    modelPath    string
    modelVersion string
    featureNames []string
    classNames   []string
    // For simple models, embed weights directly
    // For complex models, use ONNX runtime or similar
}

// NewMLClassifier loads a trained model
func NewMLClassifier(modelPath string) (*MLClassifier, error)

// Classify predicts class for a track
func (c *MLClassifier) Classify(track *TrackedObject) (string, float32, error)

// ClassifierFactory selects between rule-based and ML classifiers
type ClassifierFactory struct {
    RuleBased  *TrackClassifier
    ML         *MLClassifier
    UseML      bool
}

func (f *ClassifierFactory) Classify(track *TrackedObject) (string, float32) {
    if f.UseML && f.ML != nil {
        class, conf, err := f.ML.Classify(track)
        if err == nil {
            return class, conf
        }
        // Fall back to rule-based on error
    }
    return f.RuleBased.Classify(track)
}
```

---

### Phase 4.2: Parameter Tuning & Optimisation

**Objective:** Systematically explore parameter space to optimise track quality
metrics.

#### 4.2.1: Tuning Workflow

```
1. Define parameter grid
2. For each parameter combination:
   a. Run analysis on reference PCAP
   b. Compare to baseline run
   c. Detect splits/merges
   d. Compute quality metrics
3. Analyse results to find optimal parameters
4. Validate on held-out PCAPs
```

#### 4.2.2: Parameter Grid Search

```go
// cmd/tools/param-sweep/main.go

type ParameterGrid struct {
    BackgroundNoiseRelative []float32 `json:"background_noise_relative"`
    BackgroundCloseness     []float32 `json:"background_closeness"`
    ClusteringEps           []float32 `json:"clustering_eps"`
    ClusteringMinPts        []int     `json:"clustering_min_pts"`
    TrackingGatingDistance  []float32 `json:"tracking_gating_distance"`
    TrackingHitsToConfirm   []int     `json:"tracking_hits_to_confirm"`
}

func (g *ParameterGrid) Combinations() []RunParams {
    // Generate all parameter combinations
}

// SweepResult stores results for one parameter combination
type SweepResult struct {
    RunID           string      `json:"run_id"`
    Params          RunParams   `json:"params"`
    BaselineRunID   string      `json:"baseline_run_id"`
    Comparison      *RunComparison `json:"comparison"`
    QualityMetrics  *QualityMetrics `json:"quality_metrics"`
}

type QualityMetrics struct {
    TrackCount          int     `json:"track_count"`
    ConfirmedTrackCount int     `json:"confirmed_track_count"`
    SplitCount          int     `json:"split_count"`
    MergeCount          int     `json:"merge_count"`
    NoiseTrackCount     int     `json:"noise_track_count"`
    AvgTrackDuration    float64 `json:"avg_track_duration"`
    AvgObservationsPerTrack float64 `json:"avg_observations_per_track"`
    // Classification metrics if labels available
    ClassificationAccuracy float64 `json:"classification_accuracy,omitempty"`
}
```

#### 4.2.3: Optimisation Objective

```go
// Define objective function for parameter optimisation
func ComputeObjective(metrics *QualityMetrics, comparison *RunComparison) float64 {
    // Goal: Maximise confirmed tracks, minimise splits/merges/noise
    //
    // objective = w1 * confirmed_tracks
    //           - w2 * split_count
    //           - w3 * merge_count
    //           - w4 * noise_tracks
    //           + w5 * avg_track_duration

    w1, w2, w3, w4, w5 := 1.0, 5.0, 5.0, 2.0, 0.1

    return w1*float64(metrics.ConfirmedTrackCount) -
           w2*float64(comparison.SplitCount()) -
           w3*float64(comparison.MergeCount()) -
           w4*float64(metrics.NoiseTrackCount) +
           w5*metrics.AvgTrackDuration
}
```

#### 4.2.4: Interactive Tuning UI

```svelte
<!-- web/src/routes/lidar/tuning/+page.svelte -->
<script lang="ts">
  // Parameter sliders with live preview
  // Run comparison visualisation
  // Quality metric charts
  // Parameter recommendation engine
</script>
```

---

### Phase 4.3: Production Deployment

**Objective:** Deploy the complete ML pipeline for production use.

#### 4.3.1: Deployment Architecture

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

#### 4.3.2: Model Update Flow

```
1. Collect labelled tracks from labelling UI
2. Train new model version
3. Evaluate on validation set
4. If metrics improve:
   a. Version the model (v1.1, v1.2, etc.)
   b. Distribute to edge nodes
   c. Update classification_model field in new tracks
5. Monitor production metrics
6. Collect new edge cases for labelling
7. Repeat
```

---

### LiDAR Pipeline Data Flow Summary

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

### LiDAR Phase Implementation Priority

| Phase                           | Priority      | Dependencies             |
| ------------------------------- | ------------- | ------------------------ |
| 3.7 Analysis Run Infrastructure | ✅ Complete   | None                     |
| 3.8 Tracking Upgrades           | ✅ Complete   | Phase 3.7                |
| 3.9 Sweep/Auto-Tune System      | ✅ Complete   | Phase 3.7                |
| 4.0 Track Labelling UI          | P0 - Critical | Phase 3.7                |
| 4.1 ML Classifier Training      | P1 - High     | Phase 4.0 (needs labels) |
| 4.2 Parameter Tuning            | P1 - High     | Phase 3.7                |
| 4.3 Production Deployment       | P2 - Medium   | Phases 4.1, 4.2          |

**Recommended Order:**

1. ✅ Phase 3.7 (COMPLETED — infrastructure for all other phases)
2. ✅ Phase 3.8 (COMPLETED — tracking quality improvements)
3. ✅ Phase 3.9 (COMPLETED — parameter sweep and auto-tuning)
4. Phase 4.0 (critical for getting labels)
5. Phase 4.2 (can be done in parallel with labelling)
6. Phase 4.1 (requires labelled data from 4.0)
7. Phase 4.3 (final deployment)
