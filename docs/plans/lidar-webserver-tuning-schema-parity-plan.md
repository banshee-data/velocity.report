# Webserver Tuning Schema Parity Backlog

Status: Planned
Purpose/Summary: lidar-webserver-tuning-schema-parity-plan.

## Goal

Bring `/api/lidar/params` POST input schema to full parity with canonical tuning config schema and key order.

Canonical order/source:

- `config/tuning.defaults.json`
- `internal/config/tuning.go` (`TuningConfig` JSON tag order)
- strict verifier: `make check-config-maths-strict`

## Current Gap

`internal/lidar/monitor/webserver.go` `handleTuningParams()` POST body is missing these canonical keys:

- `buffer_timeout`
- `min_frame_points`
- `flush_interval`
- `background_flush`
- `max_tracks`
- `height_band_floor`
- `height_band_ceiling`
- `remove_ground`
- `max_cluster_diameter`
- `min_cluster_diameter`
- `max_cluster_aspect_ratio`

Also, the POST body key declaration order is grouped by subsystem, not canonical config order.

## Backlog Tasks

1. Add the missing 11 JSON-tagged fields to the POST body struct in `internal/lidar/monitor/webserver.go`.
2. Reorder all POST body JSON-tagged fields to match canonical order exactly.
3. Add/update request handling logic so each newly added field is actually applied.
4. Add/update endpoint tests to verify all canonical keys are accepted and applied.
5. Flip CI check back to required strict mode:
   - remove `continue-on-error` in `.github/workflows/config-order-ci.yml`
   - keep `make check-config-maths-strict` as the enforced command

## Definition Of Done

- `make check-config-maths-strict` passes.
- CI strict step is required (not optional).
- POST input schema keys and order match canonical config order.

## TODO Marker

- `@TODO(config-parity)` comments in:
  - `.github/workflows/config-order-ci.yml`
  - `scripts/readme-maths-check`
  - `internal/lidar/monitor/webserver.go`
