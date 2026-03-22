# Web Frontend Consolidation

Active plan: [web-frontend-consolidation-plan.md](../../plans/web-frontend-consolidation-plan.md)

## Problem

Three distinct web surfaces serve LiDAR functionality:

1. **Svelte web app** (`/app/*`, port 8080) — radar dashboard, reports, sites,
   settings, plus LiDAR tracks/scenes/runs.
2. **Go-embedded HTML dashboards** (port 8081) — LiDAR status, debug dashboard,
   parameter sweep/auto-tune, background regions.
3. **macOS Metal visualiser** (gRPC 50051) — live 3D point cloud rendering.

Pain points: LiDAR nav visible in radar-only deploys, split ports, two
charting stacks (ECharts + LayerChart), fragmented user experience.

## Decision: Option B — One Svelte App with Conditional LiDAR Sections ✅

Scored 95 vs 32 (two apps) and 67 (build-time exclusion) on a weighted
matrix covering effort, risk, complexity, usability, and maintenance.

### End State

- **One Svelte app** on port 8080 with conditional LiDAR navigation.
- **Go-embedded HTML dashboards retired** — sweep, regions, status migrated.
- **macOS visualiser retained** — unchanged for 3D rendering + debugging.
- **Port 8081 retired** — LiDAR API endpoints consolidated under 8080.
- **LiDAR navigation hidden** when `--enable-lidar` is off.

## Migration Plan

| Phase | Scope                              | Effort     | Charting Rewrite |
| ----- | ---------------------------------- | ---------- | ---------------- |
| 0     | Capabilities API + conditional nav | 2–4 days   | None             |
| 1     | Status page migration              | 2–3 days   | None             |
| 2     | Regions dashboard migration        | 2–3 days   | None (Canvas 2D) |
| 3     | Sweep dashboard migration          | 2–3 weeks  | 8 chart types    |
| 4     | Debug dashboard retirement         | 1 day      | None             |
| 5     | Retire port 8081                   | 3–5 days   | None             |
| 6     | Go embed cleanup                   | 1 day      | None             |
| Total |                                    | ~5–6 weeks |                  |

Phase 3 (sweep dashboard) dominates: 8 ECharts chart types must be
rewritten to LayerChart/d3-scale.

### Phase 0 — Capabilities API

`/api/capabilities` reports runtime sensor state:

```json
{ "radar": true, "lidar": { "enabled": false, "state": "disabled" } }
```

LiDAR navigation hidden when disabled. All `/api/lidar/*` endpoints
return "LiDAR disabled" without initialising hardware.

### Phase 3 — Sweep Dashboard (Critical Path)

| ECharts Chart          | LayerChart Equivalent  | Complexity |
| ---------------------- | ---------------------- | ---------- |
| Acceptance rate line   | Spline + Area          | Low        |
| Nonzero cells line     | Spline                 | Low        |
| Bucket distribution    | Bar chart              | Low        |
| Track count line       | Spline                 | Low        |
| Alignment score line   | Spline                 | Low        |
| Parameter heatmap      | Custom Canvas/SVG grid | High       |
| Multi-round comparison | Group + Spline         | Medium     |
| Recommendation table   | svelte-ux Table        | Low        |

### Phase 5 — Retire Port 8081

Move LiDAR API route registration from `webserver.go` to `server.go`.
Remove `--lidar-listen` flag and 8081 HTTP server. gRPC on 50051 is
unaffected.

## Design Constraints

- macOS visualiser stays (Metal 3D rendering).
- Raspberry Pi 4 is the target (resource-constrained).
- Dynamic LiDAR lifecycle (enable/disable without restart).
- Private LAN deployment (no auth required currently).
- Privacy-first.

## Risks

| Risk                              | Mitigation                                   |
| --------------------------------- | -------------------------------------------- |
| LayerChart lacks heatmap          | Custom Canvas/SVG component                  |
| Sweep polling complexity          | Svelte stores + setInterval; consider SSE    |
| Hot-enable/disable disrupts radar | LiDAR lifecycle manager with isolation tests |
| Sweep UI performance on Pi 4      | Decimation, throttling, lazy render          |
