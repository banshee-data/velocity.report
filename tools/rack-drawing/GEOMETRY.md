# Rack-Drawing Geometry Guide

This document explains how the parametric 3D model is built, where each number
comes from, and how to extend the geometry — whether that means adding features,
changing dimensions, or adding new views.

## Directory layout

```
tools/rack-drawing/
  build/          Intermediate SVGs (one per projection). Gitignored.
  output/         Final PNG — checked into git, referenced by setup.md.
  components.py   2D SVG drawing primitives (dimension lines, BOM table, …)
  draw_rack.py    Assembles views into the combined drawing sheet; rasterises.
  model.py        Parametric build123d 3D model + HLR projection to SVG.
  rack.json       All dimensions, BOM, and hole specs. Single source of truth.
  GEOMETRY.md     This file.
```

## How to rebuild

```sh
# One-time setup
make install-diagrams

# Rebuild everything (model → SVGs in build/ → PNG in output/ + image dirs)
make render-diagrams
```

---

## Model structure (`model.py`)

### Coordinate system

- Origin: centre of the crossbar **at ground level** (Z = 0).
- X: along the crossbar (positive = right when facing the front of the mount).
- Y: depth of the lumber (into / out of the front face).
- Z: vertical (positive = up).

All dimensions are stored in inches inside `rack.json` and converted to
millimetres at the top of each function using `INCH = 25.4`.

### Key dimensions (all driven by `rack.json`)

| Config key                    | Meaning                                |
| ----------------------------- | -------------------------------------- |
| `lumber.actual_width_in`      | Narrow face of 2×4 = 1.5 in (W)        |
| `lumber.actual_depth_in`      | Wide face of 2×4 = 3.5 in (D)          |
| `lumber.crossbar_length_in`   | Overall crossbar span = 32 in          |
| `lumber.upright_length_in`    | Upright height = 24 in                 |
| `lumber.brace_top_edge_in`    | Brace top-edge length = 11 in (BTE)    |
| `lumber.brace_angle_deg`      | Brace angle from horizontal = 45°      |
| `pipe.od_in` / `pipe.wall_in` | Pipe outer diameter and wall thickness |

### Assembly order

1. **Crossbar** — `Box(CB, D, W)` centred at `(0, 0, W/2)`.
2. **Upright** — `Box(W, D, UL)` centred at `(0, 0, W + UL/2)`.
3. **Braces** — over-length raw box, rotated ±45°, positioned so both ends
   penetrate the crossbar and upright volumes. Boolean subtraction
   (`right - crossbar - upright`) produces clean mitre faces.

   The brace centre-line offset formula is:

   ```
   brace_center = BTE - W · (2√2 − 1)
   h_span = brace_center · cos(45°)
   v_span = brace_center · sin(45°)
   ```

   Right brace centre: `(W/2 + h_span/2, 0, W + v_span/2)`.
   Left brace centre: mirror on X.

4. **Pipe** — `Cylinder(od, len)` minus inner cylinder. Positioned so its
   bottom sits at `Z = W + UL − PIPE_OVERLAP` (default overlap 18 in).

5. **Holes** — applied in `make_assembly` after the frame is assembled:
   - `_drill_pipe_clamp_holes`: vertical Cylinders through the crossbar for roof-rack bolts.
   - `_drill_brace_crossbar_holes` / `_drill_brace_upright_holes`: pilot holes through the brace faces.
   - `_drill_lbracket_holes`: pilot holes for corner L-bracket screws at the T-joint.

### Hole specs (`rack.json → "holes"`)

```json
"holes": {
  "crossbar_clamp": {
    "description": "Holes for roof-rack mounting bolts through crossbar (flat face), near each end",
    "dia_in": 0.375,
    "offsets_from_end_in": [3.0, 6.0],
    "qty": 4
  },
  "upright_pipe_screw": {
    "description": "Pilot holes through upright face to pin pipe to upright",
    "dia_in": 0.1875,
    "depths_from_top_in": [6.0, 12.0],
    "qty": 2
  },
  "brace_screw": {
    "description": "Pilot holes through 45-degree brace face into crossbar and upright",
    "dia_in": 0.1875,
    "qty": 8
  }
}
```

If you need to adjust hole positions for a different roof-rack carrier, change
`crossbar_clamp.offsets_from_end_in` and re-run `make render-diagrams`.

---

## How to extend the model

### Add a new feature

1. Write a helper function `_add_<feature>(shape, cfg)` that returns the
   modified `Compound` or `Part`. Boolean subtraction (`shape - cutter`) for
   voids; `Compound(children=[shape, new_part])` for additive features.
2. Call it in `make_assembly()` at the appropriate step.
3. Add any parametric values to `rack.json` so dimensions remain a single
   source of truth.

### Add a new view (projection)

Add an entry to the `views` dict in `model.py:main()`:

```python
views = {
    "front":     ((0,  1, 0), (0, 0, 1)),
    "side":      ((-1, 0, 0), (0, 0, 1)),
    "top":       ((0,  0, 1), (0, -1, 0)),
    "isometric": ((-_iso, -_iso, _iso), (0, 0, 1)),
    # "rear":    ((0, -1, 0), (0, 0, 1)),  # example
}
```

`camera_direction` is the unit vector pointing **from the scene toward the
camera** — the opposite of the viewing direction. `up` is the world-up vector
for the projection.

The new SVG will be written to `build/rack_<name>.svg`. Embed it in
`draw_rack.py` using `embed_svg_as_image(d, ...)` in the appropriate sheet
builder.

### Add a dimension annotation

All dimension helpers live in `components.py`:

| Function                                          | What it draws             |
| ------------------------------------------------- | ------------------------- |
| `dim_h(d, x1, x2, y, text)`                       | Horizontal dimension line |
| `dim_v(d, x, y1, y2, text)`                       | Vertical dimension line   |
| `leader(d, tip_x, tip_y, label_x, label_y, text)` | Leader with arrowhead     |

Annotation positions in `draw_rack.py:build_combined_sheet()` are expressed as
fractions of the view-panel bounding box (e.g. `ox + left_w * 0.50`). After
adding an annotation, run `make render-diagrams` and inspect
`output/rack-drawing-iso-bom.png`.

### Change the BOM

Edit the `bom` array in `rack.json`. Each row:

```json
{
  "icon": "🗜️",
  "description": "3–5\" SS Hose Clamp",
  "qty": 2,
  "unit": "ea",
  "unit_price": 3.78,
  "lowes_item": "1343439",
  "aisle": ""
}
```

The BOM table in `components.py:bom_table()` renders rows automatically.
Column widths are set in `_COL_WIDTHS`.

---

## File relationships

```
rack.json ──► model.py ──► build/rack_*.svg
                │
                └──► draw_rack.py ──► build/sheet_combined.svg
                                   ──► output/rack-drawing-iso-bom.png
                                   ──► public_html/src/images/rack-drawing-iso-bom.png
                                   ──► docs/images/rack-drawing-iso-bom.png
```

`rack.json` is the single source of truth for every dimension, material, and
hole specification. `draw_rack.py` reads it both for passing to the model layer
and for populating BOM rows and dimension-annotation labels.

---

## Common tasks

| Task                            | What to change                                                 |
| ------------------------------- | -------------------------------------------------------------- |
| Different roof-rack bar spacing | `holes.crossbar_clamp.spacing_in` in `rack.json`               |
| Taller / shorter upright        | `lumber.upright_length_in` in `rack.json`                      |
| Longer / shorter crossbar       | `lumber.crossbar_length_in` in `rack.json`                     |
| Different pipe OD               | `pipe.od_in` and `pipe.wall_in` in `rack.json`                 |
| Pipe sits higher/lower          | `PIPE_OVERLAP` constant in `model.py`                          |
| Add pilot hole at new location  | New `_drill_*` function in `model.py`, call in `make_assembly` |
| Widen BOM column                | `_COL_WIDTHS` list in `components.py`                          |
| Larger BOM row text             | `FS_LABEL` constant in `components.py`                         |

After any change: `make render-diagrams` to rebuild.
