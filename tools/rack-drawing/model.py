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
    Pilot holes for the two L-bracket screws at the upright/crossbar T-joint.

    Two brackets, one against each ±Y wide face of the crossbar/upright.
    Each bracket has:
      - Two holes down into the crossbar top (along Z):
          at Y = y_sign * (D/2 - inset), X = 0, centred in crossbar thickness
          insets = [0.5, 1.5] from ±D/2 face
      - Two holes through the upright wide face (along Y, into upright):
          at Z = W + arm * {1/3, 2/3}, X = 0
          These go all the way through — drilled once, serve both brackets.
    """
    lbr = cfg["lumber"]
    W = lbr["actual_width_in"] * INCH  # 1.5" crossbar/upright narrow face
    D = lbr["actual_depth_in"] * INCH  # 3.5" wide face
    h_cfg = cfg["holes"]["corner_brace_screw"]
    r = h_cfg["dia_in"] * INCH / 2
    arm = h_cfg["arm_in"] * INCH
    insets = h_cfg["crossbar_insets_from_face_in"]  # [0.5, 1.5] from ±D/2 face

    crossbar_drill_len = W + 4  # through crossbar thickness (Z)
    upright_drill_len = D + 4  # through upright depth (Y), full penetration

    # Crossbar holes: two per bracket face (×2 faces = 4 total), vertical (Z)
    for y_sign in (+1, -1):
        for inset_in in insets:
            inset = inset_in * INCH
            hy = y_sign * (D / 2 - inset)
            hole = Cylinder(r, crossbar_drill_len)
            hole = hole.move(Location((0, hy, W / 2)))
            frame = frame - hole

    # Upright holes: two heights × full Y penetration (shared by both brackets)
    for frac in (1 / 3, 2 / 3):
        hz = W + arm * frac
        hole = Cylinder(r, upright_drill_len)
        hole = hole.rotate(Axis.X, 90)  # align along Y
        hole = hole.move(Location((0, 0, hz)))
        frame = frame - hole

    return frame


# ── L-bracket solid geometry ──────────────────────────────────────────────────


def make_lbrackets(cfg: dict) -> Compound:
    """
    Two steel L-brackets at the upright/crossbar T-joint.

    Each bracket sits flat against one ±Y wide face of the crossbar/upright,
    straddling the inside corner where the crossbar top meets the upright side.

    Coordinate layout (front bracket, y_sign = +1):
      Bracket plate is in the XZ plane, at Y = +(D/2 + Tp/2)
      Horizontal arm: arm(X) × Tp(Y) × Wb(Z)
        Lies along the crossbar top, Y-face outward, X-span = [-arm/2 .. +arm/2]
        Z centre = W + Wb/2  (sits on top of crossbar, Wb thick in Z)
      Vertical arm: Wb(X) × Tp(Y) × arm(Z)
        Stands up the upright face, X-span = [-Wb/2 .. +Wb/2], centred on X=0
        Z centre = W + arm/2  (rises arm height above the crossbar top)

    Both arms share the same thin Y dimension (Tp = 3/32" plate thickness).
    arm = 3" from rack.json corner_brace_screw.arm_in
    Wb = 3/4" bracket width (plate width)
    """
    lbr = cfg["lumber"]
    W = lbr["actual_width_in"] * INCH
    D = lbr["actual_depth_in"] * INCH

    h_cfg = cfg["holes"]["corner_brace_screw"]
    arm = h_cfg["arm_in"] * INCH
    Tp = 0.09375 * INCH  # 3/32" plate thickness
    Wb = 0.75 * INCH  # 3/4" bracket width

    brackets = []

    for y_sign in (+1, -1):
        # Bracket plate sits just outside the ±D/2 wide face
        bkt_y = y_sign * (D / 2 + Tp / 2)

        # Horizontal arm — arm(X) × Tp(Y) × Wb(Z), lies flat on crossbar top
        # X centre = 0, Y = bkt_y, Z centre = W + Wb/2
        horiz = Box(arm, Tp, Wb)
        horiz = horiz.move(Location((0, bkt_y, W + Wb / 2)))
        brackets.append(horiz)

        # Vertical arm — Wb(X) × Tp(Y) × arm(Z), stands against upright wide face
        # X centre = 0, Y = bkt_y, Z centre = W + arm/2
        vert = Box(Wb, Tp, arm)
        vert = vert.move(Location((0, bkt_y, W + arm / 2)))
        brackets.append(vert)

    return Compound(children=brackets)


# ── Crossbar hose clamps ──────────────────────────────────────────────────────


def make_crossbar_hose_clamps(cfg: dict) -> Compound:
    """
    Two worm-drive hose clamp loops, one per end of the crossbar.

    Each clamp band threads through the two holes (at 3" and 6" from the end)
    and wraps around the crossbar from below, gripping the roof-rack bar.

    Modelled as a rectangular torus-like loop: a thin flat band that forms a
    closed rectangle around the crossbar cross-section (D × W) plus a small
    stand-off for the rack bar beneath.  Built from four thin Box segments.

    The clamp is centred at X = ±(CB/2 − mid_offset) where mid_offset is the
    midpoint between the two hole offsets (4.5" from end → X = ±11.5").
    """
    lbr = cfg["lumber"]
    W = lbr["actual_width_in"] * INCH  # 1.5" crossbar thickness (Z)
    D = lbr["actual_depth_in"] * INCH  # 3.5" crossbar depth (Y)
    CB = lbr["crossbar_length_in"] * INCH

    offsets = cfg["holes"]["crossbar_clamp"]["offsets_from_end_in"]
    mid_offset = (offsets[0] + offsets[1]) / 2 * INCH  # midpoint between holes

    band_t = 1.5  # band thickness in mm (represents clamp band width ~1/16")
    rack_gap = 6 * INCH / 10  # ~15 mm clearance for rack bar beneath crossbar

    # Total loop height: crossbar thickness + rack_gap below + band_t above
    loop_h = W + rack_gap
    # Loop width: crossbar depth + two band thicknesses
    loop_w = D + 2 * band_t

    clamps = []

    for x_sign in (+1, -1):
        cx = x_sign * (CB / 2 - mid_offset)
        # Z centre of the loop: crossbar occupies Z=0..W; clamp wraps from
        # slightly below (Z = -rack_gap) to slightly above (Z = W + band_t).
        # Loop centre Z = (W + band_t - rack_gap) / 2
        cz = (W + band_t - rack_gap) / 2

        # Top segment (over crossbar top)
        top = Box(band_t, loop_w, band_t)
        top = top.move(Location((cx, 0, W + band_t / 2)))
        clamps.append(top)

        # Bottom segment (under crossbar, below rack bar gap)
        bot = Box(band_t, loop_w, band_t)
        bot = bot.move(Location((cx, 0, -rack_gap + band_t / 2)))
        clamps.append(bot)

        # Front side segment (+Y)
        front = Box(band_t, band_t, loop_h)
        front = front.move(Location((cx, D / 2 + band_t / 2, cz)))
        clamps.append(front)

        # Back side segment (-Y)
        back = Box(band_t, band_t, loop_h)
        back = back.move(Location((cx, -D / 2 - band_t / 2, cz)))
        clamps.append(back)

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
    pipe_od = cfg["pipe"]["od_in"] * INCH / 2

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
