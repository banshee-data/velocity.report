# LiDAR foundations fix-it plan

- **Status:** In Progress; Phase 1 partially complete; Phases 2–3 outstanding
- **Layers:** Cross-cutting (documentation and configuration)
- **Canonical:** [foundations-fixit-progress.md](../lidar/operations/foundations-fixit-progress.md)

Planned follow-up execution to align LiDAR documentation with implementation truth and close outstanding foundation gaps.

## Phase 1: documentation truth alignment

1. Add explicit `Implemented vs Planned` sections in core LiDAR and maths docs.
2. Link all velocity-coherent planning docs to the canonical maths proposal path.
3. Flag dynamic-algorithm-selection doc as non-runtime branch spec unless re-implemented.

Exit criteria:

- No core LiDAR/maths doc claims an algorithm is active unless it is in current runtime path.

## Phase 2: runtime config parity

1. Extend `/api/lidar/params` POST support toward canonical tuning schema fields.
   Missing keys: `buffer_timeout`, `min_frame_points`, `flush_interval`, `background_flush`, `max_tracks`, `height_band_floor`, `height_band_ceiling`, `remove_ground`, `max_cluster_diameter`, `min_cluster_diameter`, `max_cluster_aspect_ratio`. (Consolidated from `webserver-tuning-schema-parity.md`, now deleted.)
2. Add tests for each newly supported key and response echo parity.
3. Keep unsupported keys explicitly listed in handler comments/docs.
4. Reorder POST body JSON-tagged fields to match canonical config order.
5. Once complete, remove `continue-on-error` from `.github/workflows/config-order-ci.yml` to enforce strict mode.

Exit criteria:

- `GET /api/lidar/params` and `POST /api/lidar/params` are symmetric for supported keys.

## Phase 3: vector workstream hardening

1. Keep region-adaptive behaviour validated on `ProcessFramePolarWithMask`.
2. Add regression checks for region override behaviour in replay/golden tests.
3. Document L3->L4 confidence handoff assumptions in maths docs.

Exit criteria:

- Region-adaptive behaviour remains covered by tests on production path.

## Phase 4: velocity workstream pre-implementation gate

1. Define extractor boundary interface and metrics schema.
2. Build side-by-side replay harness before enabling any default mode change.
3. Require acceptance metrics for sparse recall, fragmentation, and false-positive budget.

Exit criteria:

- Velocity path can be evaluated against baseline without altering default runtime mode.

## Phase 5: adoption decision

1. Compare baseline vs velocity-coherent path on agreed replay suite.
2. Decide: keep proposal, ship optional mode, or promote default.
3. Record decision with date, metrics, and rollback plan.

Exit criteria:

- Signed implementation decision with reproducible evidence.
