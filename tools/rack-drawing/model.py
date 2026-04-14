"""
model.py — Parametric build123d model of the Raspberry Pi sensor mount.

Assembly from photo:
  - Two identical L-brackets, each made from 2x 24" 2x4 pieces joined at
    a 45-degree miter, reinforced by a steel corner brace.
  - A 4" DWV pipe spanning 24" between the two brackets, sitting in the
    upright notch and secured by hose clamps.

All dimensions are in millimetres internally (build123d default).
Config values are read in inches and converted at the top of each function.
"""

import json
from pathlib import Path

from build123d import (
    Box,
    Compound,
    Cylinder,
    Location,
    Part,
    Plane,
    export_svg,
)

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


# ── L-bracket (single mount) ──────────────────────────────────────────────────


def make_bracket(cfg: dict) -> Compound:
    """
    One L-shaped mount.

    The foot lies flat (long axis along X).
    The upright stands vertically (long axis along Z).
    They meet at a 45-degree miter at the inside corner.

    Coordinate origin: inside corner of the miter joint at ground level.
    """
    lbr = cfg["lumber"]
    W = lbr["actual_width_in"] * INCH  # 1.5" = 38.1 mm  (narrow face)
    D = lbr["actual_depth_in"] * INCH  # 3.5" = 88.9 mm  (wide face)
    L = lbr["piece_length_in"] * INCH  # 24"  = 609.6 mm

    parts = []

    # ── Foot (horizontal, lies on ground, wide face up) ──────────────────
    # The foot extends L mm in the -X direction from the miter corner.
    # We position a Box centred at its own centre, then move it.
    #
    # Foot box: length=L, width=D (face up), height=W (thin edge down)
    #   Centre: X = -L/2, Y = 0, Z = W/2  (bottom face at Z=0)
    foot = Box(L, D, W)
    foot = foot.move(Location((-L / 2, 0, W / 2)))
    parts.append(foot)

    # ── Upright (vertical, wide face toward -Y, back of bracket) ─────────
    # The upright rises L mm above the miter corner.
    # Its front face (at Y = -D/2 .. +D/2, centred Y=0) aligns with the
    # foot's front face.
    # The miter is approximated as the upright sitting on top of the foot's
    # inner end; a 45° cut would make the faces flush — we show the geometry
    # as a clean butt to keep the model simple, matching typical workshop
    # construction where the brace carries the joint load.
    #
    # Upright box: width=D, depth=W, height=L
    #   Centre: X = W/2, Y = 0, Z = W + L/2
    upright = Box(W, D, L)
    upright = upright.move(Location((W / 2, 0, W + L / 2)))
    parts.append(upright)

    # ── Steel corner brace ────────────────────────────────────────────────
    # Thin flat plate, 3"×3"×0.075" steel, inside corner.
    # Arms lie in the XZ plane at Y = -D/2 (back face of the joint).
    brace_arm = 3.0 * INCH
    brace_thk = 0.075 * INCH
    brace_w = 0.75 * INCH

    # Horizontal arm: along -X from the corner
    h_arm = Box(brace_arm, brace_w, brace_thk)
    h_arm = h_arm.move(
        Location((-brace_arm / 2, -D / 2 + brace_w / 2, W - brace_thk / 2))
    )
    parts.append(h_arm)

    # Vertical arm: along +Z from the corner
    v_arm = Box(brace_thk, brace_w, brace_arm)
    v_arm = v_arm.move(
        Location((brace_thk / 2, -D / 2 + brace_w / 2, W + brace_arm / 2))
    )
    parts.append(v_arm)

    return Compound(children=parts)


# ── Pipe ──────────────────────────────────────────────────────────────────────


def make_pipe(cfg: dict, span_in: float) -> Part:
    """
    PVC pipe cylinder (hollow) along Y axis.
    Positioned so it rests on top of the two uprights.
    """
    pc = cfg["pipe"]
    od = pc["od_in"] * INCH / 2  # outer radius
    wall = pc["wall_in"] * INCH
    ir = od - wall
    length = span_in * INCH

    # Outer cylinder minus inner cylinder
    outer = Cylinder(od, length)
    inner = Cylinder(ir, length + 2)  # slightly longer to ensure clean subtraction
    pipe = outer - inner
    return pipe


# ── Full assembly ─────────────────────────────────────────────────────────────


def make_assembly(cfg: dict) -> Compound:
    """
    Complete assembly: two brackets + pipe.
    Bracket A at Y=0, Bracket B at Y = pipe_length.
    Pipe centred on the upright tops.
    """
    lbr = cfg["lumber"]
    W = lbr["actual_width_in"] * INCH
    D = lbr["actual_depth_in"] * INCH
    L = lbr["piece_length_in"] * INCH

    pipe_span_in = cfg["pipe"]["length_in"]
    pipe_span = pipe_span_in * INCH
    pipe_od = cfg["pipe"]["od_in"] * INCH / 2

    bracket_a = make_bracket(cfg)
    bracket_b = make_bracket(cfg)

    # Place bracket B at +Y = pipe_span
    bracket_b = bracket_b.move(Location((0, pipe_span, 0)))

    # Pipe: rests on top of uprights.
    # Upright top centre: X = W/2, Z = W + L + pipe_od (resting on top)
    pipe_cx = W / 2
    pipe_cz = W + L + pipe_od
    pipe = make_pipe(cfg, pipe_span_in)
    # Cylinder default axis is Z; rotate to Y axis
    pipe = pipe.rotate(
        Axis=Plane.XZ.z_dir, angle=90
    )  # this api varies; use rotation matrix
    pipe = pipe.move(Location((pipe_cx, pipe_span / 2, pipe_cz)))

    return Compound(children=[bracket_a, bracket_b, pipe])


# ── SVG export helpers ────────────────────────────────────────────────────────

_SVG_DEFAULTS = dict(
    line_weight=0.5,
    show_axes=False,
    show_hidden=True,
    hidden_weight=0.25,
)


def export_view(
    shape,
    path: str,
    camera_position: tuple[float, float, float],
    up: tuple[float, float, float] = (0, 0, 1),
    viewport_width: float = 800,
    viewport_height: float = 600,
) -> None:
    """Export one orthographic SVG view of *shape*."""
    export_svg(
        shape,
        path,
        camera_position=camera_position,
        camera_up_direction=up,
        viewport_width=viewport_width,
        viewport_height=viewport_height,
        **_SVG_DEFAULTS,
    )


# ── Entry point ───────────────────────────────────────────────────────────────


def main(config_path: str | None = None, out_dir: str | None = None) -> list[str]:
    """
    Build model, export three SVG views.
    Returns list of output file paths.
    """
    cfg = load_config(Path(config_path) if config_path else None)
    out = Path(out_dir or Path(__file__).parent)

    assembly = make_assembly(cfg)

    lbr = cfg["lumber"]
    D = lbr["actual_depth_in"] * INCH
    L = lbr["piece_length_in"] * INCH
    W = lbr["actual_width_in"] * INCH
    span = cfg["pipe"]["length_in"] * INCH

    # Camera stand-off: large enough to see the whole assembly
    far = (L + span) * 2.5

    views = {
        "front": ((0, -far, W + L / 2), (0, 0, 1)),
        "side": ((-far, span / 2, W + L / 2), (0, 0, 1)),
        "isometric": ((-far * 0.7, -far * 0.7, far * 0.9), (0, 0, 1)),
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
