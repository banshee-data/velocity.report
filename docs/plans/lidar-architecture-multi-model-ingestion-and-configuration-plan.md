# LiDAR Multi-Model Ingestion and Configuration

Status: Planned
Purpose/Summary: lidar-architecture-multi-model-ingestion-and-configuration.

**Status:** Proposed
**Author:** Architecture Team
**Related:** [`../lidar/architecture/lidar-data-layer-model.md`](../lidar/architecture/lidar-data-layer-model.md), [`../lidar/architecture/hesai_packet_structure.md`](../lidar/architecture/hesai_packet_structure.md), [`lidar-network-configuration.md`](./lidar-architecture-network-configuration-plan.md)

## Overview

This document outlines the smallest viable architecture change to support an additional 3–10 LiDAR models with different UDP packet formats, while preserving velocity.report's privacy-first and local-only deployment model.

Current state is effectively single-model (`hesai-pandar40p`) parsing with model selection mostly used as an identifier. To scale safely, we should separate:

1. **Model-agnostic ingestion orchestration** (socket, forwarding, stats, frame lifecycle)
2. **Model-specific packet decoding and calibration**
3. **Config persistence and config serving APIs**

## What Should Be Generalised

### 1. L1 parser selection (generalised)

Introduce a `ModelParserFactory` and `ModelRegistry` in `internal/lidar/l1packets/parse`:

- `ModelRegistry` maps `model_key` (e.g. `hesai-pandar40p`, `ouster-os1-64`) to capabilities and parser constructor
- `ModelParserFactory` builds the parser from a resolved model profile
- Existing `network.Parser` interface remains the integration seam for `UDPListener`

This keeps `network.UDPListener` unchanged except for receiving parser instances from the factory path rather than direct Pandar40P construction.

### 2. Calibration/profile loading (generalised)

Move from a single embedded CSV pair to model profiles:

- embedded defaults (YAML or JSON) for known models in repository
- optional local override files under `/var/lib/velocity-report/lidar/models/`
- resolved profile = `embedded default + local override + DB config`

This avoids cloud dependencies and keeps deterministic local operation.

### 3. Runtime configuration selection (generalised)

Bind live ingestion to a specific **enabled ingest profile** instead of hardcoded CLI model assumptions:

- profile includes model key, network bind settings, parser options, calibration source
- `reload` endpoint swaps parser+listener atomically (same hot-reload pattern as serial/network manager docs)

### 4. Model capability metadata (generalised)

Provide standard capability fields for UI and API consumers:

- packet transport (`udp`, `pcap`)
- expected ports
- return modes (single/dual)
- channel count
- timestamp modes
- optional features (foreground-forward compatibility)

## What Remains Model-Specific

- UDP packet binary layout and validation
- calibration schema (angles/fire times/beam intrinsics per vendor)
- return interpretation and timestamp extraction rules
- parser-level quality checks and vendor-specific error handling

These should live behind per-model parser implementations so L2+ layers stay unchanged.

## Configuration Storage

Use SQLite as canonical storage with three focused tables:

1. `lidar_model_catalog`
   - immutable model identity and default capability metadata
   - seeded from embedded defaults on startup/migration
2. `lidar_model_profiles`
   - operator-editable per-site overrides (ports, timestamp mode, calibration paths, parser knobs)
3. `lidar_ingest_config`
   - active ingestion binding (network interface/bind address/port + selected profile), one enabled row

This mirrors existing repository conventions: stable catalogue + editable config + single active runtime binding.

## Configuration Serving

Serve configuration through explicit API resources:

- `GET /api/lidar/models` — list supported models and capabilities
- `GET /api/lidar/model-profiles`, `POST /api/lidar/model-profiles`, `PUT /api/lidar/model-profiles`, `DELETE /api/lidar/model-profiles` — manage site profiles
- `GET /api/lidar/ingest/config` — active binding and resolved runtime config
- `POST /api/lidar/ingest/reload` — apply enabled profile/binding without process restart
- `POST /api/lidar/ingest/test` — validate selected model+network pair against live traffic metadata

UI placement should follow the constrained settings pattern:

- `/settings/lidar-models` for model/profile management
- `/settings/lidar-network` keeps interface/port diagnostics

## How This Works with the Current Single-Binary Deployment

For `cmd/radar` (Go monolith on Raspberry Pi):

1. Startup loads enabled `lidar_ingest_config` and resolves associated model profile.
2. `ModelParserFactory` creates the parser for that model.
3. Existing `UDPListener` runs with unchanged packet stats/forwarding/frame-builder wiring.
4. Reload endpoint rebuilds parser/listener from DB-backed config and swaps atomically.

This keeps deployment shape unchanged: no new services, no cloud coordination, no privacy regression.

## Suggested Delivery Phases

| Phase | Scope                                                                                       | Effort |
| ----- | ------------------------------------------------------------------------------------------- | ------ |
| 1     | Add registry/factory abstractions and migrate Pandar40P into registry-backed implementation | M      |
| 2     | Add SQLite tables and CRUD API for model catalogue/profile/ingest binding                   | M      |
| 3     | Add hot-reload manager wiring parser+listener swaps from active ingest config               | M      |
| 4     | Add settings UI pages and validation/test endpoints                                         | M      |
| 5     | Add first 2 non-Hesai models, then expand to 3–10 models incrementally                      | L      |

**Size key:** S = ½ day, M = 1 day, L = 2 days

## Open Questions

1. Should multiple ingest bindings be active simultaneously (multi-sensor on one host), or remain single-active to match current operational model?
2. Should model catalogue updates be migration-seeded only, or allow import from signed local files?
3. Which minimum capability contract is required before a model is marked "production-ready" in UI?
