# LiDAR Foundations Fix-It Plan

Status: Planned follow-up execution
Purpose/Summary: 20260221-lidar-foundations-fixit-plan.

## Phase 1: Documentation Truth Alignment

1. Add explicit `Implemented vs Planned` sections in core LiDAR and maths docs.
2. Link all velocity-coherent planning docs to the canonical maths proposal path.
3. Flag dynamic-algorithm-selection doc as non-runtime branch spec unless re-implemented.

Exit criteria:

- No core LiDAR/maths doc claims an algorithm is active unless it is in current runtime path.

## Phase 2: Runtime Config Parity

1. Extend `/api/lidar/params` POST support toward canonical tuning schema fields.
2. Add tests for each newly supported key and response echo parity.
3. Keep unsupported keys explicitly listed in handler comments/docs.

Exit criteria:

- `GET /api/lidar/params` and `POST /api/lidar/params` are symmetric for supported keys.

## Phase 3: Vector Workstream Hardening

1. Keep region-adaptive behavior validated on `ProcessFramePolarWithMask`.
2. Add regression checks for region override behavior in replay/golden tests.
3. Document L3->L4 confidence handoff assumptions in maths docs.

Exit criteria:

- Region-adaptive behavior remains covered by tests on production path.

## Phase 4: Velocity Workstream Pre-Implementation Gate

1. Define extractor boundary interface and metrics schema.
2. Build side-by-side replay harness before enabling any default mode change.
3. Require acceptance metrics for sparse recall, fragmentation, and false-positive budget.

Exit criteria:

- Velocity path can be evaluated against baseline without altering default runtime mode.

## Phase 5: Adoption Decision

1. Compare baseline vs velocity-coherent path on agreed replay suite.
2. Decide: keep proposal, ship optional mode, or promote default.
3. Record decision with date, metrics, and rollback plan.

Exit criteria:

- Signed implementation decision with reproducible evidence.
