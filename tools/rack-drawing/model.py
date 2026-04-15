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
      braces    — 45° supports, one each side, mitre-cut at both ends
    """
    lbr = cfg["lumber"]
    W = lbr["actual_width_in"] * INCH  # 1.5" = 38.1 mm  (narrow face)
    D = lbr["actual_depth_in"] * INCH  # 3.5" = 88.9 mm  (wide face)
    CB = lbr["crossbar_length_in"] * INCH  # 32" crossbar
    UL = lbr["upright_length_in"] * INCH  # 24" upright
    BTE = lbr["brace_top_edge_in"] * INCH  # 11" brace top edge
    BA = math.radians(lbr["brace_angle_deg"])  # 45°

    # ── Crossbar (horizontal, lies on ground, wide face up) ──────────
    crossbar = Box(CB, D, W)
    crossbar = crossbar.move(Location((0, 0, W / 2)))

    # ── Upright (vertical, centred on crossbar) ──────────────────────
    upright = Box(W, D, UL)
    upright = upright.move(Location((0, 0, W + UL / 2)))

    # ── 45° braces with mitre cuts ──────────────────────────────────
    # Start with an over-length brace, rotate and position at 45°,
    # then boolean-subtract the crossbar and upright volumes.
    # The intersection removes the overlapping ends, leaving clean
    # mitre faces where the brace meets each member.

    # At 45°, the brace's width W projects ±W/√2 in both X and Z.
    # For the boolean subtraction to produce clean mitre faces, each
    # brace end must fully penetrate the adjacent member.  The required
    # centre-line length is BTE minus the extra caused by both miter
    # cuts: W·(2√2 − 1).  Using D here is wrong — it leaves one corner
    # of each end outside the subtraction volume, producing raw tails.
    brace_center = BTE - W * (2 * math.sqrt(2) - 1)
    h_span = brace_center * math.cos(BA)
    v_span = brace_center * math.sin(BA)

    # Over-length raw brace ensures both ends penetrate the
    # neighbouring members so the boolean cut produces mating faces.
    brace_raw = Box(W, D, BTE)

    # Right brace (+X side)
    right = brace_raw.rotate(Axis.Y, -45)
    right = right.move(Location((W / 2 + h_span / 2, 0, W + v_span / 2)))
    right = right - crossbar - upright

    # Left brace (-X side)
    left = brace_raw.rotate(Axis.Y, 45)
    left = left.move(Location((-W / 2 - h_span / 2, 0, W + v_span / 2)))
    left = left - crossbar - upright

    return Compound(children=[crossbar, upright, right, left])


# ── Clamp and fastener holes ──────────────────────────────────────────────────


def _drill_crossbar_clamp_holes(frame: Compound, cfg: dict) -> Compound:
    """
    Subtract two vertical clearance holes through the crossbar's top face
    (along Z, the thickness axis = W) for the 3/8-16 roof-rack bolts.

    Hole positions are centred on the crossbar X-midline (X=0) and spaced
    ±spacing/2 along X.  The crossbar occupies Z=0 to Z=W.
    """
    lbr = cfg["lumber"]
    W = lbr["actual_width_in"] * INCH  # 1.5"  (crossbar thickness)
    h_cfg = cfg["holes"]["crossbar_clamp"]
    r = h_cfg["dia_in"] * INCH / 2
    spacing = h_cfg["spacing_in"] * INCH
    # Extend drill 2 mm each side so booleans don't leave a paper-thin skin
    length = W + 4

    for x_sign in (+1, -1):
        hole = Cylinder(r, length)
        hole = hole.move(Location((x_sign * spacing / 2, 0, W / 2)))
        frame = frame - hole

    return frame


def _drill_pipe_clamp_holes(frame: Compound, cfg: dict) -> Compound:
    """
    Subtract two pilot holes through the upright on the centreline (X=0),
    drilled along Y.  These accept the screws that pin the PVC pipe to the
    upright: the screw passes through both walls of the pipe and bites into
    the wood.

    Positions are measured down from the top end of the upright:
      6"  from top  — upper clamp zone
      18" from top  — lower clamp zone

    Both holes sit at X=0 (centreline) and penetrate the full 3.5" depth
    of the upright plus 2 mm each side.
    """
    lbr = cfg["lumber"]
    W = lbr["actual_width_in"] * INCH  # crossbar thickness; upright bottom is at Z=W
    D = lbr["actual_depth_in"] * INCH  # 3.5" upright depth (Y axis)
    UL = lbr["upright_length_in"] * INCH
    h_cfg = cfg["holes"]["upright_pipe_screw"]
    r = h_cfg["dia_in"] * INCH / 2
    length = D + 4  # full penetration through depth + 2 mm each side

    top_z = W + UL  # Z-coordinate of the upright's top face

    for depth_in in h_cfg["depths_from_top_in"]:
        z = top_z - depth_in * INCH
        hole = Cylinder(r, length)
        hole = hole.rotate(Axis.X, 90)  # align cylinder axis along Y
        hole = hole.move(Location((0, 0, z)))
        frame = frame - hole

    return frame


def _drill_brace_crossbar_holes(frame: Compound, cfg: dict) -> Compound:
    """
    Subtract pilot holes through each diagonal brace into the crossbar.

    The hole axis is perpendicular to the 45° brace face, i.e. it is tilted
    45° from vertical (one component each in X and Z).

    For the right brace (+X side) the axis points in the direction
    (+sin45°, 0, -cos45°) — down and outward — so the cylinder is built
    along Z then rotated -45° around Y.  The left brace mirrors in X.

    The drill entry point is at the centroid of the brace–crossbar contact
    face, approximately:
        X = ±(W/2 + h_span/2)   (centre of brace footprint)
        Z = W                    (top of crossbar)
    """
    lbr = cfg["lumber"]
    W = lbr["actual_width_in"] * INCH
    D = lbr["actual_depth_in"] * INCH
    BA = math.radians(lbr["brace_angle_deg"])
    BTE = lbr["brace_top_edge_in"] * INCH
    h_cfg = cfg["holes"]["brace_screw"]
    r = h_cfg["dia_in"] * INCH / 2
    # Penetrate through the full brace depth plus into crossbar — D + 4 suffices
    length = D + 4

    # Reproduce brace centre-line span from make_frame so the entry X is correct
    brace_center = BTE - W * (2 * math.sqrt(2) - 1)
    h_span = brace_center * math.cos(BA)

    for x_sign in (+1, -1):
        entry_x = x_sign * (W / 2 + h_span / 2)
        hole = Cylinder(r, length)
        # Rotate so hole axis is perpendicular to the 45° brace face
        hole = hole.rotate(Axis.Y, x_sign * -45)
        hole = hole.move(Location((entry_x, 0, W)))
        frame = frame - hole

    return frame


def _drill_brace_upright_holes(frame: Compound, cfg: dict) -> Compound:
    """
    Subtract pilot holes through each diagonal brace into the upright.

    The hole axis is perpendicular to the 45° brace face at its upper end,
    i.e. tilted 45° inward and upward.

    For the right brace (+X side) the axis points in the direction
    (-sin45°, 0, +cos45°) — up and inward toward the upright.  The cylinder
    is built along Z then rotated +45° around Y.  The left brace mirrors.

    The drill entry point is at the centroid of the brace–upright contact
    face, approximately:
        X = ±W/2     (side of upright)
        Z = W + v_span/2
    """
    lbr = cfg["lumber"]
    W = lbr["actual_width_in"] * INCH
    D = lbr["actual_depth_in"] * INCH
    BA = math.radians(lbr["brace_angle_deg"])
    BTE = lbr["brace_top_edge_in"] * INCH
    h_cfg = cfg["holes"]["brace_screw"]
    r = h_cfg["dia_in"] * INCH / 2
    length = D + 4

    brace_center = BTE - W * (2 * math.sqrt(2) - 1)
    v_span = brace_center * math.sin(BA)

    for x_sign in (+1, -1):
        entry_x = x_sign * W / 2
        entry_z = W + v_span / 2
        hole = Cylinder(r, length)
        # Rotate so hole axis is perpendicular to the 45° brace face (opposite
        # direction to the crossbar holes: angling up into the upright)
        hole = hole.rotate(Axis.Y, x_sign * 45)
        hole = hole.move(Location((entry_x, 0, entry_z)))
        frame = frame - hole

    return frame


def _drill_upright_clamp_holes(frame: Compound, cfg: dict) -> Compound:
    """
    Subtract hose-clamp band pass-through holes at each end of the upright.

    Two holes per end, drilled along Y (through the 3.5" face), spaced
    ±spacing/2 around X=0.  One pair near the bottom of the upright (to wrap
    the crossbar beneath) and one pair near the top (for the roof-rack bar).

    Each hole is a clearance hole for the clamp band width, not a pilot.
    """
    lbr = cfg["lumber"]
    W = lbr["actual_width_in"] * INCH
    D = lbr["actual_depth_in"] * INCH
    UL = lbr["upright_length_in"] * INCH
    h_cfg = cfg["holes"]["upright_clamp_slot"]
    r = h_cfg["dia_in"] * INCH / 2
    spacing = h_cfg["spacing_in"] * INCH
    offset = h_cfg["offset_from_end_in"] * INCH
    length = D + 4  # through full depth + clearance

    top_z = W + UL
    z_positions = (W + offset, top_z - offset)  # bottom end, top end

    for z in z_positions:
        for x_sign in (+1, -1):
            hole = Cylinder(r, length)
            hole = hole.rotate(Axis.X, 90)  # align along Y
            hole = hole.move(Location((x_sign * spacing / 2, 0, z)))
            frame = frame - hole

    return frame


def _drill_lbracket_holes(frame: Compound, cfg: dict) -> Compound:
    """
    Subtract pilot holes for the L-bracket screws at the upright/crossbar
    T-joint.

    Four brackets total (two on each ±Y face of the T-joint).  Each bracket
    has four screw holes: two into the crossbar top face (along Z, going down)
    and two into the upright face (along Y).  Holes are at 28% and 72% of the
    3-inch arm length, matching the component.py corner_brace() drawing.

    Crossbar holes (along Z):
        X=0, Y = ±arm × {0.28, 0.72}, Z = W (top of crossbar) drilling down

    Upright holes (along Y):
        X=0, Z = W + arm × {0.28, 0.72} (measured up from crossbar top)
        drilled along Y through the 3.5" face
    """
    lbr = cfg["lumber"]
    W = lbr["actual_width_in"] * INCH
    D = lbr["actual_depth_in"] * INCH
    h_cfg = cfg["holes"]["corner_brace_screw"]
    r = h_cfg["dia_in"] * INCH / 2
    arm = h_cfg["arm_in"] * INCH

    fractions = (0.28, 0.72)

    # Crossbar holes — drilled vertically (along Z) into the crossbar top face
    crossbar_drill_len = W + 4
    for frac in fractions:
        for y_sign in (+1, -1):
            hole = Cylinder(r, crossbar_drill_len)
            hole = hole.move(Location((0, y_sign * arm * frac, W / 2)))
            frame = frame - hole

    # Upright holes — drilled horizontally (along Y) into the upright ±Y face
    upright_drill_len = D + 4
    for frac in fractions:
        z = W + arm * frac
        hole = Cylinder(r, upright_drill_len)
        hole = hole.rotate(Axis.X, 90)  # align along Y
        hole = hole.move(Location((0, 0, z)))
        frame = frame - hole

    return frame


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
    Pipe slides over the top of the upright and is screwed to it.
    """
    lbr = cfg["lumber"]
    W = lbr["actual_width_in"] * INCH
    UL = lbr["upright_length_in"] * INCH
    pipe_len = cfg["pipe"]["length_in"] * INCH
    pipe_od = cfg["pipe"]["od_in"] * INCH / 2

    frame = make_frame(cfg)

    # ── Drill fastener holes into the frame ───────────────────────────────
    if "holes" in cfg:
        frame = _drill_crossbar_clamp_holes(frame, cfg)
        frame = _drill_pipe_clamp_holes(frame, cfg)
        frame = _drill_brace_crossbar_holes(frame, cfg)
        frame = _drill_brace_upright_holes(frame, cfg)
        frame = _drill_upright_clamp_holes(frame, cfg)
        frame = _drill_lbracket_holes(frame, cfg)

    # Pipe overlaps the top portion of the upright (held by hose clamps
    # and screwed in).  18" of overlap (10" lower than v1) positions the
    # sensor at a better working height.
    PIPE_OVERLAP = 18 * INCH
    pipe_bottom_z = W + UL - PIPE_OVERLAP
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
    # Default build dir sits next to this file; callers may override.
    default_out = Path(__file__).parent / "build"
    out = Path(out_dir) if out_dir else default_out
    out.mkdir(parents=True, exist_ok=True)

    assembly = make_assembly(cfg)

    # camera_direction: unit vector pointing from the scene toward the camera.
    # For front view: camera at +Y → direction = (0, 1, 0)
    # For side view: camera at -X → direction = (-1, 0, 0)
    # For isometric: camera at (-1, -1, 1) normalised
    _iso = math.sqrt(1 / 3)
    views = {
        "front": ((0, 1, 0), (0, 0, 1)),
        "side": ((-1, 0, 0), (0, 0, 1)),
        "top": ((0, 0, 1), (0, -1, 0)),
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
