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
    w, d, ln = width_in * INCH, depth_in * INCH, length_in * INCH
    return Box(w, d, ln)


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
    Subtract four vertical clearance holes through the crossbar top face for
    the 3/8-16 roof-rack mounting bolts — two per end of the crossbar.

    Positions: 3" and 6" inset from each end along X.
    Crossbar runs from X = -CB/2 to +CB/2.  End insets count from ±CB/2 inward.
    """
    lbr = cfg["lumber"]
    W = lbr["actual_width_in"] * INCH
    CB = lbr["crossbar_length_in"] * INCH
    h_cfg = cfg["holes"]["crossbar_clamp"]
    r = h_cfg["dia_in"] * INCH / 2
    length = W + 4  # full crossbar thickness + clearance

    for inset_in in h_cfg["offsets_from_end_in"]:
        inset = inset_in * INCH
        for x_sign in (+1, -1):
            x = x_sign * (CB / 2 - inset)
            hole = Cylinder(r, length)
            hole = hole.move(Location((x, 0, W / 2)))
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
    Subtract pilot holes entering from the outer lower face of each brace,
    perpendicular to that face (45° from vertical), and exiting through the
    mitre into the crossbar.

    The entry point is located W/√2 back along the brace centreline from the
    lower mitre tip, which keeps the hole well inside the brace body while
    staying close to the end.

      Right brace entry: X = W/2 + h_span − W·cos45°/2
                          Z = W + W·sin45°/2
      Axis direction (right): (+sin45°, 0, −cos45°)
      Rotation: built as Z-cylinder, rotate −45° around Y for right brace,
                +45° around Y for left brace.
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
    h_span = brace_center * math.cos(BA)

    # W/√2 setback from the mitre tip along the brace axis
    setback = W / math.sqrt(2)

    for x_sign in (+1, -1):
        # Entry point: near the lower end of the brace, on its outer face
        entry_x = x_sign * (W / 2 + h_span - setback * math.cos(BA))
        entry_z = W + setback * math.sin(BA)
        hole = Cylinder(r, length)
        # +45° for right brace (+X) gives axis (+sin45°, 0, -cos45°) — correct direction
        hole = hole.rotate(Axis.Y, x_sign * 45)
        hole = hole.move(Location((entry_x, 0, entry_z)))
        frame = frame - hole

    return frame


def _drill_brace_upright_holes(frame: Compound, cfg: dict) -> Compound:
    """
    Subtract pilot holes entering from the outer upper face of each brace,
    perpendicular to that face (45° from vertical), and exiting through the
    mitre into the upright.

    The entry point is located W/√2 back along the brace centreline from the
    upper mitre tip, keeping the hole inside the brace and clear of the upright.

      Right brace entry: X = W/2 + setback·cos45°
                          Z = W + v_span − setback·sin45°
      Axis direction (right): (−sin45°, 0, +cos45°)
      Rotation: built as Z-cylinder, rotate +45° around Y for right brace,
                −45° for left brace.
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

    setback = W / math.sqrt(2)

    for x_sign in (+1, -1):
        # Entry point: near the upper end of the brace, on its outer face
        entry_x = x_sign * (W / 2 + setback * math.cos(BA))
        entry_z = W + v_span - setback * math.sin(BA)
        hole = Cylinder(r, length)
        # +45° for right brace (+X), −45° for left brace (−X)
        hole = hole.rotate(Axis.Y, x_sign * 45)
        hole = hole.move(Location((entry_x, 0, entry_z)))
        frame = frame - hole

    return frame


def _drill_lbracket_holes(frame: Compound, cfg: dict) -> Compound:
    """
    Pilot holes for the L-bracket screws at the upright/crossbar T-joint.

    One bracket each side (±X), centred on Y=0.  Each has:
      - Two holes down into the crossbar top (Z), at X = ±(W/2 + arm*{0.28,0.72})
      - Two holes through the upright narrow face (X), at Z = W + arm*{0.28,0.72}
    """
    lbr = cfg["lumber"]
    W = lbr["actual_width_in"] * INCH
    h_cfg = cfg["holes"]["corner_brace_screw"]
    r = h_cfg["dia_in"] * INCH / 2
    arm = h_cfg["arm_in"] * INCH

    crossbar_drill_len = W + 4
    upright_drill_len = W + 4

    for x_sign in (+1, -1):
        # Crossbar arm holes: vertical (Z)
        for frac in (0.28, 0.72):
            hx = x_sign * (W / 2 + arm * frac)
            hole = Cylinder(r, crossbar_drill_len)
            hole = hole.move(Location((hx, 0, W / 2)))
            frame = frame - hole

        # Upright arm holes: horizontal (X)
        for frac in (0.28, 0.72):
            hz = W + arm * frac
            hole = Cylinder(r, upright_drill_len)
            hole = hole.rotate(Axis.Y, 90)
            hole = hole.move(Location((0, 0, hz)))
            frame = frame - hole

    return frame


# ── L-bracket solid geometry ──────────────────────────────────────────────────


def make_lbrackets(cfg: dict) -> Compound:
    """
    Two steel L-brackets at the upright/crossbar T-joint.

    One bracket on each side (±X) of the upright.  Each is a single thin plate
    in the XZ plane, centred at Y=0, Wb wide in Y.

    Horizontal arm: arm(X) × Wb(Y) × Tp(Z)  — lies flat on crossbar top face
      X centre: x_sign*(W/2 + arm/2)
      Z centre: W + Tp/2

    Vertical arm: Tp(X) × Wb(Y) × arm(Z)  — stands against upright narrow face
      X centre: x_sign*(W/2 - Tp/2)  — flush with outside of upright face
      Z centre: W + arm/2

    arm=3", Tp=3/32" plate thickness, Wb=3/4" plate width.
    """
    lbr = cfg["lumber"]
    W = lbr["actual_width_in"] * INCH

    h_cfg = cfg["holes"]["corner_brace_screw"]
    arm = h_cfg["arm_in"] * INCH
    Tp = 0.09375 * INCH  # 3/32" plate thickness
    Wb = 0.75 * INCH  # 3/4" bracket plate width

    brackets = []

    for x_sign in (+1, -1):
        # Horizontal arm — flat on crossbar top (Z=W), extending outward in X
        horiz = Box(arm, Wb, Tp)
        horiz = horiz.move(Location((x_sign * (W / 2 + arm / 2), 0, W + Tp / 2)))
        brackets.append(horiz)

        # Vertical arm — against upright narrow face, rising up in Z
        vert = Box(Tp, Wb, arm)
        vert = vert.move(Location((x_sign * (W / 2 - Tp / 2), 0, W + arm / 2)))
        brackets.append(vert)

    return Compound(children=brackets)


# ── Crossbar hose clamps ──────────────────────────────────────────────────────


def make_crossbar_hose_clamps(cfg: dict) -> Compound:
    """
    Two hose-clamp strap loops, one per end of the crossbar.

    Each strap threads DOWN through the inner hole (6" from end), runs under
    the crossbar (and around the roof-rack bar below), then comes UP through
    the outer hole (3" from end) and across the top face.

    The strap is modelled as a U-shaped loop in the XZ plane, with:
      - A top segment running along X between the two holes (on the crossbar top)
      - A bottom segment at the same X span, below the crossbar + rack gap
      - Two vertical legs connecting top and bottom at each hole X position

    Holes are at X = ±(CB/2 - 3") and ±(CB/2 - 6") from centre.
    band_t = strap thickness (thin flat band, ~2 mm wide).
    band_w = strap width in Y (real clamp bands are ~1/2" wide).
    """
    lbr = cfg["lumber"]
    W = lbr["actual_width_in"] * INCH  # 1.5" crossbar thickness (Z)
    CB = lbr["crossbar_length_in"] * INCH

    offsets_in = cfg["holes"]["crossbar_clamp"]["offsets_from_end_in"]  # [3.0, 6.0]
    x_inner = CB / 2 - offsets_in[1] * INCH  # 6" from end — inner hole
    x_outer = CB / 2 - offsets_in[0] * INCH  # 3" from end — outer hole
    span = x_outer - x_inner  # distance between the two holes

    band_t = 2.5  # band strip thickness (mm)
    band_w = 12.0  # band width in Y (~1/2")
    rack_gap = 15.0  # clearance below crossbar for rack bar
    loop_h = W + rack_gap

    # Worm-drive tightening boss dimensions
    boss_l = 18.0  # length along X
    boss_h = 10.0  # height above band
    boss_w = band_w + 2.0  # slightly wider than band

    clamps = []

    for x_sign in (+1, -1):
        ix = x_sign * x_inner  # X of inner leg (6" hole)
        ox = x_sign * x_outer  # X of outer leg (3" hole)
        cx = (ix + ox) / 2  # X centre of top/bottom segments

        # Top segment
        top = Box(span, band_w, band_t)
        top = top.move(Location((cx, 0, W + band_t / 2)))
        clamps.append(top)

        # Bottom segment
        bot = Box(span, band_w, band_t)
        bot = bot.move(Location((cx, 0, -rack_gap + band_t / 2)))
        clamps.append(bot)

        # Inner leg (at 6" hole)
        leg_inner = Box(band_t, band_w, loop_h)
        leg_inner = leg_inner.move(Location((ix, 0, (W - rack_gap) / 2)))
        clamps.append(leg_inner)

        # Outer leg (at 3" hole)
        leg_outer = Box(band_t, band_w, loop_h)
        leg_outer = leg_outer.move(Location((ox, 0, (W - rack_gap) / 2)))
        clamps.append(leg_outer)

        # Worm-drive tightening boss on top segment, next to outer leg
        boss_cx = ox - x_sign * (boss_l / 2 + 3.0)
        boss = Box(boss_l, boss_w, boss_h)
        boss = boss.move(Location((boss_cx, 0, W + band_t + boss_h / 2)))
        clamps.append(boss)

        # Screw-head slot on outer face of boss
        slot = Box(1.5, boss_w * 0.5, boss_h * 0.6)
        slot = slot.move(
            Location(
                (boss_cx + x_sign * (boss_l / 2 - 0.75), 0, W + band_t + boss_h / 2)
            )
        )
        clamps.append(slot)

    return Compound(children=clamps)


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

    frame = make_frame(cfg)

    # ── Drill fastener holes into the frame ───────────────────────────────
    if "holes" in cfg:
        frame = _drill_crossbar_clamp_holes(frame, cfg)
        frame = _drill_pipe_clamp_holes(frame, cfg)
        frame = _drill_brace_crossbar_holes(frame, cfg)
        frame = _drill_brace_upright_holes(frame, cfg)
        frame = _drill_lbracket_holes(frame, cfg)

    # Pipe overlaps the top portion of the upright (held by hose clamps
    # and screwed in).  18" of overlap positions the sensor at working height.
    PIPE_OVERLAP = 18 * INCH
    pipe_bottom_z = W + UL - PIPE_OVERLAP
    pipe = make_pipe(cfg)
    pipe = pipe.move(Location((0, 0, pipe_bottom_z + pipe_len / 2)))

    # L-brackets at the T-joint
    lbrackets = make_lbrackets(cfg)

    # Hose clamp loops through the crossbar end holes
    clamps = make_crossbar_hose_clamps(cfg)

    return Compound(children=[frame, pipe, lbrackets, clamps])


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
