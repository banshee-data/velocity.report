# Light Mode

- **Source plan:** `docs/plans/lidar-visualiser-light-mode-plan.md`

Light-mode appearance for the macOS VelocityVisualiser 3D scene, following system appearance preference.

## Approach

Dual-palette semantic colour tokens that switch between dark and light variants based on `NSApp.effectiveAppearance`. The palette is defined once and referenced by all rendering and UI code.

## Colour Tokens

| Token             | Dark Value    | Light Value  | Usage                    |
| ----------------- | ------------- | ------------ | ------------------------ |
| `sceneBackground` | near-black    | off-white    | Metal clear colour       |
| `gridLines`       | dim grey      | light grey   | Ground grid              |
| `trackBox`        | accent green  | darker green | Bounding boxes           |
| `selectedTrack`   | bright accent | deep accent  | Selected track highlight |
| `trailPast`       | muted cyan    | steel blue   | Ghost trail (past)       |
| `trailFuture`     | muted orange  | burnt orange | Ghost trail (future)     |
| `textPrimary`     | white         | near-black   | Inspector text           |
| `textSecondary`   | light grey    | dark grey    | Secondary labels         |
| `warningOverlay`  | amber         | amber-dark   | Threshold warnings       |
| `errorOverlay`    | red           | red-dark     | Threshold errors         |

## Files Affected

- `MetalRenderer.swift` — palette lookup for clear colour, grid, overlays
- `AppState.swift` — appearance change observer, palette storage
- `ContentView.swift` — SwiftUI colour bindings for inspector and controls

## Accessibility Requirement

V1 palette must be **colour-blind-safe** (tested against deuteranopia and protanopia simulations). No semantic information may be conveyed by hue alone — shape, pattern, or label must accompany colour.
