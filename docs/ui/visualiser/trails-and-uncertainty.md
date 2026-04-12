# Trails and uncertainty visualisation

- **Source plan:** [docs/plans/lidar-visualiser-trails-and-uncertainty-visualisation-plan.md](../../plans/lidar-visualiser-trails-and-uncertainty-visualisation-plan.md)

Ghost trails and uncertainty cones rendered in the 3D scene for tracked objects.

## Visual design

- **Past trail:** last 3 seconds of track history, rendered as fading polyline segments.
- **Future trail:** 1.5-second predicted path from current velocity/heading.
- **Uncertainty cone:** projected from track covariance matrix (position block), rendered as translucent mesh.

### Encoding rules

- Selected track: full-opacity trail, solid future line, visible cone.
- Unselected tracks: reduced-opacity trail, no cone.
- Low-quality tracks (`quality_grade` D–F): dashed future trail to signal uncertainty.
- Speed threshold: trails suppressed for stationary tracks (speed < 0.3 m/s).

## Data source

No persistent database storage required. Trails and cones are computed per frame from:

- Track observation history (position, timestamp): already in memory during replay/live.
- Track covariance matrix (`covariance_4x4`): streamed in `FrameBundle.Track`.
- Current velocity and heading: streamed in `FrameBundle.Track`.

## GPU implementation

- Trail segments: instanced line-strip buffer, one per visible track.
- Uncertainty cones: parametric mesh generated from 2×2 position covariance sub-matrix (eigenvalue decomposition → ellipse semi-axes → cone geometry).
- Buffers updated per frame in the existing render pass.

## Performance budget

- Target: ≥ 30 FPS with 70,000 points and 150 active tracks.
- GPU overhead budget: ≤ 3 ms/frame for trail + cone rendering.
- Buffer allocation: pre-allocated ring buffers sized for max trail length × max track count.
