# LiDAR L2 Dual-Representation Plan

**Status:** ✅ Implemented
**Created:** 2026-03-11
**Layers:** L1 Packets, L2 Frames, L3 Grid, L4 Perception
**Related audit:** [20260311-coordinate-flow-audit.md](../lidar/architecture/20260311-coordinate-flow-audit.md)

## Decision

Adopt this runtime representation strategy:

> **Store both polar and Cartesian once at L2, then have L3 consume the stored polar slice instead of rebuilding it from `frame.Points`.**

This removes the current hot-path re-wrap from `LiDARFrame.Points -> []PointPolar` inside the tracking pipeline while preserving:

- L2 Cartesian access for visualisation and ASC export
- L3 pure polar classification
- L4+ world-Cartesian clustering and tracking

## Why this option

From the coordinate audit:

- the current critical path is `polar -> sensor Cartesian -> polar(copy) -> world Cartesian`
- the middle `Cartesian -> polar` step is not lossy, but it is redundant hot-path work
- the cleanest near-term fix is to persist both representations once in L2 and stop rebuilding the polar slice per frame

This option is lower-risk than making `LiDARFrame` polar-only, because existing L2/L9 consumers already rely on `frame.Points` Cartesian geometry.

## Goals

- Eliminate per-frame pipeline reconstruction of `[]PointPolar` from `LiDARFrame.Points`
- Keep exactly one canonical stored polar slice per frame
- Keep exactly one canonical stored sensor-Cartesian slice per frame
- Preserve current external behaviour for gRPC, ASC export, LidarView forwarding, and tracking
- Make the L2/L3/L4 boundary explicit in code and docs

## Non-goals

- No change to clustering / tracking maths
- No pose-system redesign
- No change to DB schema
- No forced move to polar-only or Cartesian-only frames
- No semantic change to current identity-pose world transform behaviour

## Proposed Shape

`LiDARFrame` should carry both:

- `PolarPoints []PointPolar`
- `Points []Point`

Rules:

- L1 parser emits `[]PointPolar`
- L2 frame builder stores the incoming `[]PointPolar` once on the frame
- L2 frame builder computes `[]Point` once from that polar slice
- L3 consumes `frame.PolarPoints`
- L4 world transform consumes foreground `[]PointPolar` derived from `frame.PolarPoints`
- L2/L9 visualisation/export continues to consume `frame.Points`

## Build Steps

### Phase 1: L2 data model

- [ ] Add canonical L2 ownership for `PointPolar`, `Point`, `SphericalToCartesian`, and `ApplyPose` if not already completed by the dependency-hygiene work
- [ ] Extend `LiDARFrame` to store `PolarPoints []PointPolar` alongside `Points []Point`
- [ ] Document representation ownership on `LiDARFrame` fields: `PolarPoints` is the sensor-polar view, `Points` is the sensor-Cartesian view

### Phase 2: Frame builder population

- [ ] Update `FrameBuilder.AddPointsPolar()` to populate both frame representations from the same input batch
- [ ] Avoid a second derived-polar copy later in the pipeline; the frame must already own the polar slice
- [ ] Decide and document copy semantics clearly:
  - `AddPointsPolar()` must copy packet/parser-owned input before storing on the frame
  - `Points` must be generated once from the stored polar values
- [ ] Ensure frame finalisation / buffering / backfill paths preserve both slices consistently

### Phase 3: Pipeline cutover

- [ ] Update `TrackingPipelineConfig.NewFrameCallback()` to consume `frame.PolarPoints` directly
- [ ] Delete the per-frame `frame.Points -> []PointPolar` rebuild block
- [ ] Keep L3 foreground extraction on polar only
- [ ] Keep L4 `TransformToWorld()` input as `[]PointPolar`

### Phase 4: Consumer audit

- [ ] Verify L2/L9 consumers that should remain Cartesian still use `frame.Points`
- [ ] Verify L3 consumers that should be polar now use `frame.PolarPoints`
- [ ] Verify debug/forwarding paths still operate on the intended form:
  - foreground UDP forwarding remains polar
  - foreground snapshot store remains polar-first
  - visualiser point cloud remains Cartesian
  - ASC frame export remains Cartesian

### Phase 5: Tests

- [ ] Add frame-builder tests proving both `PolarPoints` and `Points` are populated and aligned index-for-index
- [ ] Add pipeline tests proving L3 uses `frame.PolarPoints` and no local polar rebuild occurs
- [ ] Add regression tests for foreground extraction count parity before/after refactor
- [ ] Add regression tests for visualiser/export consumers that still rely on `frame.Points`
- [ ] Add benchmarks comparing old vs new callback allocations and CPU time

### Phase 6: Documentation

- [ ] Update the coordinate audit to mark this plan as implemented when done
- [ ] Update `docs/lidar/architecture/lidar-data-layer-model.md` so L2 explicitly owns both stored frame views
- [ ] Update any architecture docs that currently imply L3 receives polar by reconstructing it from Cartesian frame points

## Implementation Checklist

### Code checklist

- [ ] `LiDARFrame` has both `PolarPoints` and `Points`
- [ ] `AddPointsPolar()` stores both once
- [ ] No `frame.Points -> []PointPolar` rebuild remains in the tracking callback
- [ ] L3 hot path reads frame-owned polar data
- [ ] L2/L9 Cartesian consumers remain unchanged unless they benefit from direct polar access

### Correctness checklist

- [ ] `len(frame.PolarPoints) == len(frame.Points)` for every completed frame
- [ ] Per-index metadata matches between views: `Channel`, `Distance`, `Azimuth`, `Elevation`, `Timestamp`, `BlockID`, `UDPSequence`
- [ ] Foreground mask output is unchanged on replay fixtures
- [ ] Cluster counts and track counts remain within expected replay tolerance

### Performance checklist

- [ ] Per-frame allocation count is reduced or unchanged in the tracking callback
- [ ] Per-frame callback CPU time does not regress
- [ ] No new long-lived duplicate copies beyond the intended two frame-owned views

## Risks

### Memory growth

Storing both views on every frame increases frame object size. This is intentional, but we should measure:

- active frame memory during live operation
- buffered frame memory during replay / backfill

This trade is acceptable only if it removes transient hot-path allocations and callback work.

### Partial migration bugs

If some consumers read `frame.PolarPoints` and others still silently rebuild their own polar slice, the code becomes harder to audit rather than easier. The cutover should remove the old rebuild path completely.

### Future ownership overlap

This plan complements, but does not replace, the L2 type-ownership migration. The long-term target remains:

- sensor-frame primitives live in L2
- L3 consumes L2 polar forms
- L4 transforms L2 sensor-frame forms into world-frame forms

## Acceptance Criteria

This plan is complete when:

1. `LiDARFrame` stores both canonical views.
2. The tracking callback no longer reconstructs `[]PointPolar` from `frame.Points`.
3. L3 consumes frame-owned polar data directly.
4. Replay-based regression tests show no behavioural drift beyond normal clustering tolerance.
5. Documentation and backlog references point to the new steady-state design.
