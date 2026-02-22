# Product Roadmap and Milestones

## Status: Active

## Summary

Unified product roadmap mapping velocity.report features to versioned release
milestones (v0.5 → v2.0). Includes a decision matrix explaining placement
rationale, a dependency graph, and a future-compatibility strategy for online
geometry-prior services.

## Related Documents

| Document | Scope |
|----------|-------|
| [BACKLOG.md](../../BACKLOG.md) | Priority-ordered work queue (single source of truth) |
| [CHANGELOG.md](../../CHANGELOG.md) | Released feature history (v0.1.0 – v0.4.0) |
| [LiDAR ROADMAP](../lidar/ROADMAP.md) | LiDAR pipeline phases (3.7 – 4.3) |
| [Vector-Scene Map Architecture](../lidar/architecture/vector-scene-map.md) | Geometry-prior data model |
| [Ground-Plane Vector-Scene Maths](../maths/proposals/20260221-ground-plane-vector-scene-maths.md) | Maths for piecewise planar tiles and prior weighting |

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

---

## 2. Release Milestones

### v0.5 — Platform Hardening (current cycle)

**Theme:** Stabilise the build, test, and deployment surface.

| Feature | Plan Reference | Effort |
|---------|---------------|--------|
| SWEEP/HINT platform hardening (Phase 5–6) | [sweep-hint-mode](lidar-sweep-hint-mode-plan.md) | M |
| Settling optimisation Phase 3 | BACKLOG P1 | M |
| Python venv consolidation (.venv/ at root) | [tooling-python-venv](tooling-python-venv-consolidation-plan.md) | S |
| Documentation standardisation (metadata) | [platform-docs](platform-documentation-standardization-plan.md) | S |

**Exit criteria:** All P1 backlog items either completed or promoted to v0.6.

---

### v0.6 — Deployment & Packaging

**Theme:** Make velocity.report installable by a non-developer.

| Feature | Plan Reference | Effort |
|---------|---------------|--------|
| Precompiled LaTeX (800 MB → 60 MB) | [pdf-latex-precompiled](pdf-latex-precompiled-format-plan.md) | M |
| Single `velocity-report` binary + subcommands | [deploy-distribution](deploy-distribution-packaging-plan.md) Phase 1–2 | L |
| GitHub Releases CI pipeline | [deploy-distribution](deploy-distribution-packaging-plan.md) Phase 3 | M |
| One-line install script | [deploy-distribution](deploy-distribution-packaging-plan.md) Phase 4 | S |
| Raspberry Pi imager pipeline (pi-gen) | [deploy-rpi-imager](deploy-rpi-imager-fork-plan.md) | L |
| Platform simplification & deprecation (Phase 1) | [platform-simplification](platform-simplification-and-deprecation-plan.md) | S |

**Exit criteria:** A user can flash an SD card, boot a Raspberry Pi, and see
live speed data in a browser within 15 minutes. GitHub Releases publishes
checksummed artefacts on tag push.

---

### v0.7 — Unified Frontend & Radar Polish

**Theme:** One port, one app, one navigation. Radar side is production-grade.

| Feature | Plan Reference | Effort |
|---------|---------------|--------|
| Frontend consolidation (Phases 0–5) | [web-frontend](web-frontend-consolidation-plan.md) | L |
| Retire port 8081 | [web-frontend](web-frontend-consolidation-plan.md) Phase 5 | S |
| Transit deduplication | BACKLOG P2 | M |
| SQLite client standardisation | [data-sqlite](data-sqlite-client-standardization-plan.md) | M |
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
| Test coverage ≥ 95.5% across all components | [platform-quality](platform-quality-coverage-improvement-plan.md) | L |
| Platform simplification complete (all phases) | [platform-simplification](platform-simplification-and-deprecation-plan.md) | M |
| LiDAR foundations fix-it (doc truth, runtime config) | [lidar-foundations](lidar-architecture-foundations-fixit-plan.md) | M |
| Unified settling (L3/L4 SettlementCore) | [maths: unify-l3-l4](../maths/proposals/20260219-unify-l3-l4-settling.md) | L |
| LaTeX palette cross-reference | BACKLOG P2 | S |
| Time-partitioned raw data tables | BACKLOG P2 | M |
| Geometry-prior local file format (GeoJSON) | [vector-scene-map](../lidar/architecture/vector-scene-map.md) | M |
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
| Visualiser QC programme (Features 1–10) | [qc-overview](lidar-visualiser-labelling-qc-enhancements-overview-plan.md) | XL |
| ML classifier training pipeline (Phase 4.1) | [LiDAR ROADMAP](../lidar/ROADMAP.md) | L |
| Parameter tuning optimisation (Phase 4.2) | [LiDAR ROADMAP](../lidar/ROADMAP.md) | L |
| Ground-plane vector-scene maths (piecewise planar tiles) | [maths: ground-plane](../maths/proposals/20260221-ground-plane-vector-scene-maths.md) | L |
| Velocity-coherent foreground extraction | [maths: velocity-coherent](../maths/proposals/20260220-velocity-coherent-foreground-extraction.md) | L |
| Dynamic algorithm selection (A/B foreground modes) | [lidar-dynamic-algo](lidar-architecture-dynamic-algorithm-selection-plan.md) | M |
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
proposal](../maths/proposals/20260221-ground-plane-vector-scene-maths.md)) but
runtime integration depends on unified settling (also v1.0).

**Why QC programme is v2.0, not v1.0:**
The QC programme spans 10 features with cross-dependencies (M1–M5 milestones
internally). It targets the LiDAR labelling workflow, which is experimental in
v1.0. Shipping v1.0 without QC keeps the release scope achievable.

**Why ML classifier is v2.0:**
Requires labelled training data generated by the QC programme. Also
dependent on Phase 4.1 of the LiDAR ROADMAP, which sequences after track
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
