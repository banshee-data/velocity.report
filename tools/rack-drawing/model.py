"""
model.py — Parametric build123d model of the Raspberry Pi sensor mount.

Assembly from the actual build:
  - A 32" crossbar (2×4, flat) that sits on the roof rack.
  - A 24" upright (2×4, vertical) centred on the crossbar.
  - Two 45° braces (2×4, 11" top edge each) from the crossbar to the
    upright, one each side.  Both ends are mitre-cut so the brace sits
    flush against crossbar and upright without overlap.
  - A 4" DWV pipe standing vertically, overlapping the top of the
    upright and screwed into it.

All dimensions are in millimetres internally (build123d default).
Config values are read in inches and converted at the top of each function.
"""

import json
import math
from pathlib import Path

from build123d import (
    Axis,
    Box,
    Compound,
    Cylinder,
    Location,
    Part,
)
from OCP.gp import gp_Dir
from ocpsvg.hlr import HiddenLineRenderer

INCH = 25.4  # mm per inch


def _cfg_path() -> Path:
    return Path(__file__).parent / "rack.json"


def load_config(path: Path | None = None) -> dict:
    with open(path or _cfg_path()) as f:
        return json.load(f)


# ── Lumber piece ──────────────────────────────────────────────────────────────


def lumber_box(width_in: float, depth_in: float, length_in: float) -> Part:
    """Plain rectangular lumber piece, long axis along Z."""
    w, d, l = width_in * INCH, depth_in * INCH, length_in * INCH
    return Box(w, d, l)


# ── T-frame assembly ─────────────────────────────────────────────────────────


def make_frame(cfg: dict) -> Compound:
    """
    The T-shaped wooden frame.

    Coordinate origin: centre of the crossbar at ground level (Z=0).

    Members:
      crossbar  — 32" along X, flat (wide face up), bottom at Z=0
      upright   — 24" along Z, centred on crossbar, bottom at Z=W
      braces    — 45° supports, one each side, from crossbar to upright
    """
    lbr = cfg["lumber"]
    W = lbr["actual_width_in"] * INCH  # 1.5" = 38.1 mm  (narrow face)
    D = lbr["actual_depth_in"] * INCH  # 3.5" = 88.9 mm  (wide face)
    CB = lbr["crossbar_length_in"] * INCH  # 32" crossbar
    UL = lbr["upright_length_in"] * INCH  # 24" upright
    BTE = lbr["brace_top_edge_in"] * INCH  # 11" brace top edge
    BA = math.radians(lbr["brace_angle_deg"])  # 45°

    parts = []

    # ── Crossbar (horizontal, lies on ground, wide face up) ──────────
    # Long axis along X, centred at origin.
    # Box(X_len, Y_len, Z_len) centred at origin; move bottom to Z=0.
    crossbar = Box(CB, D, W)
    crossbar = crossbar.move(Location((0, 0, W / 2)))
    parts.append(crossbar)

    # ── Upright (vertical, centred on crossbar) ──────────────────────
    # Sits on top of the crossbar at X=0. Wide face toward ±Y.
    # Bottom at Z=W (top of crossbar).
    upright = Box(W, D, UL)
    upright = upright.move(Location((0, 0, W + UL / 2)))
    parts.append(upright)

    # ── 45° braces ───────────────────────────────────────────────────
    # Each brace: 2×4 with both ends mitered at 45°.
    # Top edge = BTE. For a 45° miter on a D-wide board:
    #   bottom edge = BTE - 2·D  (D removed at each end by the miter)
    # Centre length = BTE - D.
    # When placed at 45°:
    #   horizontal span = (BTE - D) · cos(45°)
    #   vertical span   = (BTE - D) · sin(45°)
    #
    # Simplification: model as a rectangular prism at 45°, length =
    # centre line = BTE - D.  The miter faces are cosmetic; the
    # silhouette from front/side is correct.

    brace_center = BTE - D
    brace = Box(W, D, brace_center)

    # Brace attachment: bottom end sits on crossbar top (Z=W) at some
    # X offset; top end meets the upright face.
    # Horizontal span from upright edge:
    h_span = brace_center * math.cos(BA)
    v_span = brace_center * math.sin(BA)

    # Right brace (+X side): rotates -45° around Y, then position.
    # After rotation the brace centre is at the midpoint of its span.
    # Brace centre X = W/2 + h_span/2, Z = W + v_span/2
    right = brace.rotate(Axis.Y, -45)
    right = right.move(Location((W / 2 + h_span / 2, 0, W + v_span / 2)))
    parts.append(right)

    # Left brace (-X side): mirror of right
    left = brace.rotate(Axis.Y, 45)
    left = left.move(Location((-W / 2 - h_span / 2, 0, W + v_span / 2)))
    parts.append(left)

    return Compound(children=parts)


# ── Pipe ──────────────────────────────────────────────────────────────────────


def make_pipe(cfg: dict) -> Part:
    """
    PVC pipe cylinder (hollow), vertical along Z.
    """
    pc = cfg["pipe"]
    od = pc["od_in"] * INCH / 2  # outer radius
    wall = pc["wall_in"] * INCH
    ir = od - wall
    length = pc["length_in"] * INCH

    outer = Cylinder(od, length)
    inner = Cylinder(ir, length + 2)
    pipe = outer - inner
    return pipe


# ── Full assembly ─────────────────────────────────────────────────────────────


def make_assembly(cfg: dict) -> Compound:
    """
    Complete assembly: T-frame + vertical pipe.
    Pipe sits on top of the upright, centred.
    """
    lbr = cfg["lumber"]
    W = lbr["actual_width_in"] * INCH
    UL = lbr["upright_length_in"] * INCH
    pipe_len = cfg["pipe"]["length_in"] * INCH
    pipe_od = cfg["pipe"]["od_in"] * INCH / 2

    frame = make_frame(cfg)

    # Pipe: vertical, centred on the upright top.
    # Upright top at Z = W + UL.  Pipe bottom rests there.
    pipe_bottom_z = W + UL
    pipe = make_pipe(cfg)
    pipe = pipe.move(Location((0, 0, pipe_bottom_z + pipe_len / 2)))

    return Compound(children=[frame, pipe])


# ── SVG export helpers ────────────────────────────────────────────────────────


def export_view(
    shape,
    path: str,
    camera_direction: tuple[float, float, float],
    up: tuple[float, float, float] = (0, 0, 1),
    width: float = 800,
) -> None:
    """
    Export one orthographic SVG view of *shape* using ocpsvg HLR.

    camera_direction: unit vector pointing FROM the scene TOWARD the camera
                      (i.e. the view direction reversed). For a front view
                      looking along +Y, pass (0, -1, 0).
    up: world-up direction for the projection.
    """
    renderer = HiddenLineRenderer.Orthographic(
        camera_direction=gp_Dir(*camera_direction),
        camera_up=gp_Dir(*up),
    )
    render = renderer([shape.wrapped])
    tree = render.to_svg(width=width)
    tree.write(path, xml_declaration=True, encoding="unicode")


# ── Entry point ───────────────────────────────────────────────────────────────


def main(config_path: str | None = None, out_dir: str | None = None) -> list[str]:
    """
    Build model, export three SVG views.
    Returns list of output file paths.
    """
    cfg = load_config(Path(config_path) if config_path else None)
    out = Path(out_dir or Path(__file__).parent)

    assembly = make_assembly(cfg)

    # camera_direction: unit vector pointing from the scene toward the camera.
    # For front view: camera at +Y → direction = (0, 1, 0)
    # For side view: camera at -X → direction = (-1, 0, 0)
    # For isometric: camera at (-1, -1, 1) normalised
    _iso = math.sqrt(1 / 3)
    views = {
        "front": ((0, 1, 0), (0, 0, 1)),
        "side": ((-1, 0, 0), (0, 0, 1)),
        "isometric": ((-_iso, -_iso, _iso), (0, 0, 1)),
    }

    paths = []
    for name, (cam, up) in views.items():
        p = str(out / f"rack_{name}.svg")
        print(f"  Exporting {name} view → {p}")
        export_view(assembly, p, cam, up)
        paths.append(p)

    return paths


if __name__ == "__main__":
    import sys

    config_arg = sys.argv[1] if len(sys.argv) > 1 else None
    out_arg = sys.argv[2] if len(sys.argv) > 2 else None
    produced = main(config_arg, out_arg)
    for p in produced:
        print(f"Saved: {p}")
