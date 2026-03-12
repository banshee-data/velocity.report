# Design: VelocityVisualiser Light Mode for 3D Scene Rendering

**Status:** Proposed (February 2026)

## Objective

Add a dedicated **Light Mode** for the macOS VelocityVisualiser 3D scene that follows the macOS system appearance (Dark/Light) and uses legible, semantically clear colours for:

- point cloud points,
- track trails,
- 3D bounding boxes.

## Why This Matters

- Daylight and office use can make dark backgrounds harder to read.
- Exported screenshots for reports often need a light visual style.
- Current colour tuning is optimised for dark mode, so simple background inversion is not enough.

## Scope

### In Scope

- Drive 3D scene appearance directly from macOS system dark/light mode (no in-app toggle in V1).
- Define and implement a light-mode colour system for:
  - point cloud classes/intensity bands,
  - historical/predictive trails,
  - selected/unselected boxes and labels.
- Update rendering defaults (grid, axes, depth fog, highlight alpha) so overlays remain readable on light backgrounds.
- Ensure trail/box selection and confidence emphasis remain visually distinct in both themes.
- Document visual acceptance criteria and rollout checks.

### Out of Scope

- Full system-wide app theming rewrite outside the 3D scene.
- New track semantics or classifier logic.
- Changes to protobuf or backend transport.

## Functional Design

### Theme Modes

- `Dark` (existing behaviour when macOS is in Dark Appearance).
- `Light` (new high-contrast light background profile when macOS is in Light Appearance).

The visualiser does not expose an in-app appearance selector; it always follows system appearance.

### 3D Rendering Style Targets (Light Mode)

- Background: near-white neutral (not pure white) to preserve depth cues.
- Point cloud: slightly darker/saturated hues versus dark mode to avoid washout.
- Trails:
  - keep temporal fade,
  - increase minimum opacity floor in light mode,
  - selected-track trail thickness + colour accent maintained.
- Boxes:
  - unselected boxes use medium-dark outline,
  - selected box uses high-contrast accent,
  - label text and badge backgrounds remain AA-readable against light scene.

## Color System Proposal

Define a dual-palette token set in Swift (example token names):

- `sceneBackground`
- `sceneGrid`
- `pointForegroundStatic`
- `pointForegroundDynamic`
- `trailPast`
- `trailFuture`
- `boxDefault`
- `boxSelected`
- `textPrimary`
- `textBadgeBackground`

Each token maps to values for `dark` and `light`, so render code references semantic tokens rather than hard-coded per-pass colours.

## UX and Controls

- No user-exposed appearance toggle in app UI.
- Appearance is derived from system dark/light mode changes at runtime.
- Optional future enhancement (post-V1): explicit override for review workflows if needed.

## Implementation Plan

### Primary Files

- `tools/visualiser-macos/VelocityVisualiser/Rendering/MetalRenderer.swift`
- `tools/visualiser-macos/VelocityVisualiser/App/AppState.swift`
- `tools/visualiser-macos/VelocityVisualiser/UI/ContentView.swift`

### Work Breakdown

1. Introduce appearance-mode binding to system dark/light state in `AppState`.
2. Add semantic colour token model consumed by renderer passes.
3. Implement light-mode constants and tune point/trail/box values.
4. React to system appearance changes without requiring app restart.
5. Validate readability and selection prominence in dense scenes.

## Validation and Test Plan

- Swift unit tests for system-appearance mapping and token selection.
- Visual snapshot captures (dark vs light) for representative scenes:
  - sparse scene,
  - dense point cloud,
  - selected-track + trails.
- Manual rendering checks:
  - no loss of depth perception,
  - selected item remains obvious,
  - no low-contrast text badges.
- Performance check: system appearance transition should not materially impact frame time.

## Acceptance Criteria

- 3D viewport correctly follows macOS system dark/light appearance at runtime.
- Point cloud, trails, and boxes remain readable in light mode.
- Selected track/box remains visually dominant in both themes.
- No in-app appearance toggle is required for V1 behaviour.
- No measurable regression in target FPS attributable to light mode.

## Risks and Mitigations

- **Risk:** Colours that look good in isolation fail in dense scenes.
  - **Mitigation:** Validate against high-density recordings before merge.
- **Risk:** Per-pass hard-coded colours cause inconsistent behaviour.
  - **Mitigation:** Centralise semantic tokens and remove ad-hoc constants.
- **Risk:** Light background reduces depth cues.
  - **Mitigation:** tune grid/fog/axis contrast separately from object colours.

## V1 Accessibility Requirement

- A colour-blind-safe palette variant (or equivalently validated colour set) is required for V1 sign-off for point cloud, trails, and boxes.
- Validation should include simulation/review for common deficiencies (deuteranopia, protanopia, tritanopia) and manual readability checks in dense scenes.

## Open Questions

- Should appearance be app-global or scoped to 3D scene only in V1?
- Should screenshots/export pipeline auto-annotate theme metadata?
- Should colour-blind mode be automatic (system accessibility) or explicit app configuration in post-V1 iterations?
