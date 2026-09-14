# Design: ghost trails and velocity uncertainty cones (feature 5)

- **Status:** Proposed (February 2026)
- **Layers:** L9 Endpoints, L10 Clients
- **Canonical:** [trails-and-uncertainty.md](../ui/visualiser/trails-and-uncertainty.md)

## Objective

**Current priority:** v0.5.3 makes trails faithful to the temporal physical estimate, including
occlusion, before adding decorative prediction overlays. The
[state-estimation plan](lidar-state-estimation-plan.md) owns body anchors, prediction, uncertainty,
expiry and reacquisition. The renderer must not repair a wrong track with independent smoothing.

Improve motion interpretability during review by rendering:

- past and future ghost trails,
- uncertainty cones derived from track covariance,
- confidence-aware visual styling.

## Goals

- Help reviewers detect jitter, implausible motion, and imminent track uncertainty.
- Keep rendering performant in dense scenes.
- Keep UI controls simple and immediately understandable.

## Non-Goals

- Predictive planning.
- Long-horizon trajectory forecasting.
- Physics-rule enforcement (handled by Feature 7).

## Functional design

### Ghost trails

- Align body boxes and trails to the same anchor, estimate stage and capture timestamp.
- Distinguish observed support, coasted prediction, provisional history and revised final history.
  All predicted segments are visibly distinct, regardless of the quality score below.
- Bound prediction by the estimator's coast age/uncertainty limits; remove expired predictions.
  Seeking, source changes and reacquisition must not join unrelated identities or stale history.
- Verify partial/full occlusion, stops, turns and ID changes against frozen recorded-frame
  sequences and numeric position checks. Attractive curves alone are not acceptance evidence.
- Past trail: fade from current position backward up to configurable horizon.
- Future trail: short extrapolation from current state using motion model.
- Default horizons:
  - past: 3.0 seconds
  - future: 1.5 seconds

### Uncertainty cones

- Derive principal axes from velocity covariance (or position covariance fallback).
- Render cone/frustum aligned with heading.
- Colour by uncertainty magnitude:
  - green: low
  - amber: medium
  - red: high

### Visual encoding

- Selected track gets stronger alpha and thicker trail.
- Tracks with quality `< 60` get dashed future trail.
- Hide future trail when speed below threshold (`<0.3 m/s`).

## Data and compute path

Rendering alone need not introduce a DB schema. Stage/support/age and revision provenance come
from the estimator contract; add protocol fields if existing fields cannot distinguish them.
The persisted final trajectory remains the estimator's responsibility.

Use existing streaming fields:

- `TrackTrail.points`
- `Track.vx/vy/vz`
- `Track.covariance4x4`
- `Track.motionModel`

Optional protocol extension (if needed after profiling):

- Add precomputed uncertainty parameters per track to protobuf `Track`.

## Renderer design (macOS)

Files:

- [tools/visualiser-macos/VelocityVisualiser/Rendering/MetalRenderer.swift](../../tools/visualiser-macos/VelocityVisualiser/Rendering/MetalRenderer.swift)
- [tools/visualiser-macos/VelocityVisualiser/UI/ContentView.swift](../../tools/visualiser-macos/VelocityVisualiser/UI/ContentView.swift)
- [tools/visualiser-macos/VelocityVisualiser/App/AppState.swift](../../tools/visualiser-macos/VelocityVisualiser/App/AppState.swift)

Implementation:

- Add new GPU buffers for ghost trail segments and cone meshes.
- Reuse existing trail update loop; split into past and future segments.
- Add frustum culling and track-count caps to protect FPS.
- Add adaptive decimation when point count is high.

## Controls and UX

Add controls near overlay toggles:

- `Ghost Trails` toggle
- `Uncertainty` toggle
- `Past Horizon` slider
- `Future Horizon` slider
- `Cone Scale` slider

Keyboard shortcuts:

- `H` toggle ghost trails
- `U` toggle uncertainty cones

## API and client contract

No new REST endpoints required for MVP.

Add optional fields to local Swift `Track` model if protocol extension is adopted.

## Performance budget

Targets on Apple Silicon baseline:

- maintain >=30 FPS with 70k points and 150 active tracks,
- ghost + uncertainty rendering adds <=3 ms/frame GPU time,
- no more than +150 MB transient memory overhead.

## Task checklist

### Renderer and models

- [ ] Add ghost trail buffers and draw pass in `MetalRenderer.swift`
- [ ] Add uncertainty cone mesh generation and draw pass
- [ ] Add fallback covariance handling when data is missing
- [ ] Add selected-track emphasis and low-quality styling

### UI and state

- [ ] Add toggles/sliders to `OverlayTogglesView` in `ContentView.swift`
- [ ] Add state fields in `AppState.swift` for horizons and cone scale
- [ ] Add keybindings for ghost/uncertainty toggles
- [ ] Persist settings between sessions

### Protocol and mapping (optional)

- [ ] Evaluate need for protobuf `Track` extension
- [ ] If needed, update [proto/velocity_visualiser/v1/visualiser.proto](../../proto/velocity_visualiser/v1/visualiser.proto)
- [ ] Update Go adapter mapping and Swift decoder models

### Web parity

- [ ] Add simplified ghost-trail rendering mode in `MapPane.svelte`
- [ ] Add uncertainty glyph rendering fallback in canvas mode
- [ ] Add UI toggles for parity

### Testing and validation

- [ ] Unit tests for covariance -> cone parameter conversion
- [ ] Snapshot tests for selected and non-selected trail styling
- [ ] FPS/performance benchmark test in `VelocityVisualiserTests`
- [ ] Visual regression test across zoom and camera angles

### Documentation

- [ ] Add operator guidance for interpreting uncertainty cones
- [ ] Add troubleshooting notes for visual clutter/performance tradeoffs

## Acceptance criteria

- Reviewers can toggle trails and uncertainty independently without stutter.
- High-uncertainty tracks are visually obvious in dense scenes.
- Selected-track motion history/future is interpretable within one interaction.
- Performance targets are met on baseline hardware.

## Open questions

- Should future trajectory use CV only or motion-model-specific extrapolation (CV/CA)?
- Should uncertainty cones be clipped by map plane for clearer 2D readability?
- Should low-FPS mode auto-disable future trails first?
