# Published scene player yaw correction

The published scene player mirrored box yaw while placing positions correctly.
This correction preserves the stored orientation when rendering; it does not
change exported assets, the tracker, or the interpretation of inferred boxes.

## Coordinate derivation

The shared mapping in `public_html/src/js/scene-coords.js` takes sensor
`(x, y, z)` to Three.js `(x, z, -y)`. A box's length axis starts at local
`+X`. Rotating it by sensor yaw `a` gives `(cos(a), sin(a), 0)`, which maps
to `(cos(a), 0, -sin(a))`. Three.js rotation about `+Y` by **positive** `a`
produces exactly that vector. The previous negative rotation instead produced
`(cos(a), 0, sin(a))`, mirroring the heading across the east axis.

At +45°, the correct axis points towards north (`-Z`); the old axis pointed
towards south (`+Z`). Rectangle footprints at cardinal headings can hide this
error because a half-turn leaves the footprint unchanged. The regression tests
therefore compare transformed corners at positive and negative diagonal headings,
as well as cardinal and wrapped headings, for both fill and edges. They cover the `hdg`
fallback and the precedence of a present, zero-valued `bh`.

## Published evidence

On November 14, 2026 (UTC), validation fetched these public assets afresh:

- [Embarcadero header](https://velocity.report/surveys/embarcadero-broadway/assets/part-000/header.json)
- [Embarcadero chunk 0](https://velocity.report/surveys/embarcadero-broadway/assets/part-000/frames/chunk_0000.ndjson.gz)
- [Lombard chunk 1](https://velocity.report/surveys/lombard-broderick/assets/part-000/frames/chunk_0001.ndjson.gz)

The comparison uses the acute angle between the box's length axis and published
velocity, treating opposite ends of the box as the same axis. Times are seconds
from the start of the recording. These are numerical checks of published inferred
data, not physical vehicle-yaw measurements or browser screenshots.

| Site                    | Track | Time (s)   | Stored yaw (rad) | Previous rendered gap | Corrected rendered gap |
| ----------------------- | ----- | ---------- | ---------------- | --------------------- | ---------------------- |
| Embarcadero at Broadway | `1v`  | 36.819015  | 2.383            | 88.7059°              | 1.7776°                |
| Lombard at Broderick    | `gi`  | 109.807450 | -2.132           | 70.7670°              | 6.4577°                |

Every track sample in both fetched chunks was passed through `TrackVisual.update`.
The rendered local length axis of both fill and edges was compared with the
independently mapped sensor yaw vector. All 8,754 Embarcadero samples and 8,693
Lombard samples agreed within `1e-12` in unit-vector distance; the largest observed
error was `3.51e-16`.

Frozen SHA-256 provenance, in the order linked above:

```text
header:       27e4df861f296d9b87da3dc5f9072b14e438b2c351a2fdfba0d67ec0c3ba3aa3
Embarcadero:  15d8952f882534fb8ba6a169db37fc99f2b09c13b22805d6517ded8b2ef40cfc
Lombard:      7be192c9dbb9303dc5f128c220dad2231337ba0c9560596bc081beee3f3e1d9d
```

The Embarcadero header identifies build `0.5.1-pre50`, commit
`57e9e9f1a3d5e6c43b36371aafbdcc6c4099cefb`, generated at
`2026-11-01T23:35:36Z`, source VRLOG SHA-256
`07371903ef524a2a70377b4c239e186ec4cdb3e0351e2c0a80e9a7eacb737f0d`, and
online trail estimator `cv_kf_v1/online`. Downloads and validation outputs remain
local; the regression tests retain only the bounded heading/velocity examples.

## Other paths and validation limits

`internal/scene/export.go` copies `BBoxHeadingRad` to `bh` with rounding, without
negation. The macOS visualiser stays Z-up and uses a positive `rotationZ` matrix,
whose first column is `(cos(a), sin(a), 0)`. Neither requires this correction.
The synthetic hero scene already computes yaw in Three.js coordinates and applies
the same Y rotation to its points and boxes. Playback, seeking, and deterministic
capture use the published player's shared box update path.

`make test-public-html` passed 311 tests, `make test-scene-capture` passed 53 tests,
and `make build-docs` built the public site. Restoring the old sign made twelve
player regression tests fail. Capture lifecycle tests required permission to bind
a local HTTP server outside the network sandbox.

The tests establish consistency between stored yaw and rendered geometry. They
cannot establish that the stored yaw, velocity, box dimensions, or site pose
matches a physical vehicle. No raw capture replay or physical reference check was
performed. Residual turn lag, centroid drift, and other upstream tracking errors
need separate validation when the capture volume is available. Deploy the player
change to correct existing assets; this rendering correction needs no replay or
asset regeneration.
