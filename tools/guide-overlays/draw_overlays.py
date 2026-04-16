#!/usr/bin/env python3
"""Generate annotated guide-image SVGs for setup documentation.

Each SVG embeds the raw JPEG via relative <image> reference and overlays
vector annotations (beam cones, direction-of-travel markers, chevrons).

All annotation positions are expressed as percentages of image dimensions
so they survive crops and resizes. Edit the config dicts at the top of
this file and re-run.

Usage:
    python3 tools/guide-overlays/draw_overlays.py

Output:
  public_html/src/images/guide-angel.svg      — beam cone + angle + direction T
  public_html/src/images/guide-aim-sutro.svg   — opaque beam cone + chevrons
  docs/images/guide-angel.svg                  — copy
  docs/images/guide-aim-sutro.svg              — copy
"""

import math
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent.parent
IMG_DIRS = [REPO / "public_html" / "src" / "images", REPO / "docs" / "images"]

# ── Shared colour ─────────────────────────────────────────────────────
CONE_FILL = "rgba(255,200,0,0.25)"
CONE_STROKE = "rgba(255,200,0,0.85)"
LABEL_FILL = "white"
LABEL_STROKE = "rgba(0,0,0,0.6)"
FONT = "system-ui, -apple-system, sans-serif"

# UI navbar green — hex #047857 (Tailwind emerald-700)
NAVBAR_GREEN = "#047857"
NAVBAR_GREEN_FILL = "rgba(4,120,87,0.85)"
NAVBAR_GREEN_STROKE = "rgba(4,120,87,0.92)"

# Deep orange for Sutro beam
SUTRO_FILL = "rgba(230,81,0,0.85)"  # deep-orange-900 tint
SUTRO_STROKE = "rgba(230,81,0,0.95)"
SUTRO_CHEVRON = "rgba(255,111,0,1.0)"  # deep-orange-A400


# ── Helpers ───────────────────────────────────────────────────────────


def _dir(heading_deg):
    """(dx, dy) in image coords for heading_deg clockwise from image-up."""
    r = math.radians(heading_deg)
    return (math.sin(r), -math.cos(r))


def _pt(origin, heading_deg, dist):
    dx, dy = _dir(heading_deg)
    return (origin[0] + dx * dist, origin[1] + dy * dist)


def _pct(pct_x, pct_y, w, h):
    """Convert percentage position to pixel coords."""
    return (pct_x / 100.0 * w, pct_y / 100.0 * h)


def _pct_len(pct, ref):
    """Convert a percentage length to pixels relative to ref dimension."""
    return pct / 100.0 * ref


# ═════════════════════════════════════════════════════════════════════
# ANGEL IMAGE — beam cone + inverted-T direction-of-travel marker
# ═════════════════════════════════════════════════════════════════════

ANGEL_CFG = {
    "jpeg": "guide_angel_RAW.jpeg",
    "w": 1200,
    "h": 900,
    # ── Beam cone (all percentages of image dimensions) ──
    "apex_x_pct": 43.20,  # sensor tube opening — % of width
    "apex_y_pct": 73.2,  # % of height
    # Nudge the whole diagram 1° CW relative to previous heading of 10°
    "beam_heading_deg": 11,  # was 10, +1° CW
    "beam_half_angle_deg": 10.5,  # half of 21° cone
    "beam_length_pct": 38,  # % of height
    # ── Angle arc + label ──
    "arc_radius_pct": 10,  # % of height
    "label_offset_pct": 12,  # extra offset beyond arc, % of height
    "label_font_size": 32,
    # ── Direction-of-travel T marker ──
    "t_bar_half_pct": 12.0,  # half-length of horizontal bar — % of width
    "t_stem_pct": 10.0,  # stem length along left edge of cone — % of height
    # Bigger label, pushed lower (dominant-baseline="hanging" + bigger dy)
    "t_label_text": "direction of travel →",
    "t_label_font_size": 34,  # was 26
    "t_label_dy": 28,  # pixels below bar (was 18)
}


def _angel_svg():
    c = ANGEL_CFG
    W, H = c["w"], c["h"]
    ax, ay = _pct(c["apex_x_pct"], c["apex_y_pct"], W, H)
    h = c["beam_heading_deg"]
    ha = c["beam_half_angle_deg"]
    length = _pct_len(c["beam_length_pct"], H)

    left = _pt((ax, ay), h - ha, length)
    right = _pt((ax, ay), h + ha, length)

    arc_r = _pct_len(c["arc_radius_pct"], H)
    arc_left = _pt((ax, ay), h - ha, arc_r)
    arc_right = _pt((ax, ay), h + ha, arc_r)
    label_r = arc_r + _pct_len(c["label_offset_pct"], H)
    lbl = _pt((ax, ay), h, label_r)

    # ── 90° reference line: straight up from apex, extending past triangle tip ──
    # "Up" in image coords = heading 0°. Extends from apex to 130% of beam length.
    ref_line_end = _pt((ax, ay), 0, length * 1.30)

    # ── Direction-of-travel T, anchored at beam apex ──
    bar_half = _pct_len(c["t_bar_half_pct"], W)
    stem_len = _pct_len(c["t_stem_pct"], H)

    bl = (ax - bar_half, ay)
    br = (ax + bar_half, ay)

    stem_end = _pt((ax, ay), h - ha, stem_len)

    sq = 10
    sdx, sdy = _dir(h - ha)
    bdx, bdy = 1.0, 0.0
    sq_corner = (ax + bdx * sq, ay + sdy * sq)

    label_y = ay + c["t_label_dy"]

    return f"""\
<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg"
     xmlns:xlink="http://www.w3.org/1999/xlink"
     width="{W}" height="{H}"
     viewBox="0 0 {W} {H}">
  <image href="{c['jpeg']}" width="{W}" height="{H}"/>

  <!-- Beam cone — dark green (navbar colour #047857) -->
  <polygon
    points="{ax:.1f},{ay:.1f} {left[0]:.1f},{left[1]:.1f} {right[0]:.1f},{right[1]:.1f}"
    fill="{NAVBAR_GREEN_FILL}" stroke="{NAVBAR_GREEN_STROKE}"
    stroke-width="2.5" stroke-linejoin="round"/>

  <!-- 90° reference line: vertical dashed, from apex upward, 130% of beam length -->
  <line x1="{ax:.1f}" y1="{ay:.1f}"
        x2="{ref_line_end[0]:.1f}" y2="{ref_line_end[1]:.1f}"
        stroke="{LABEL_FILL}" stroke-width="1.5" opacity="0.7"
        stroke-dasharray="10,6"/>

  <!-- Angle arc -->
  <path d="M {arc_left[0]:.1f},{arc_left[1]:.1f}
           A {arc_r:.1f},{arc_r:.1f} 0 0,1 {arc_right[0]:.1f},{arc_right[1]:.1f}"
        fill="none" stroke="{LABEL_FILL}" stroke-width="2" opacity="0.9"/>

  <!-- Angle label -->
  <text x="{lbl[0]:.1f}" y="{lbl[1]:.1f}"
        fill="{LABEL_FILL}" font-size="{c['label_font_size']}" font-weight="bold"
        font-family="{FONT}" text-anchor="middle" dominant-baseline="central"
        paint-order="stroke" stroke="{LABEL_STROKE}" stroke-width="3">
    {ha * 2:.0f}°
  </text>

  <!-- Direction-of-travel T anchored at apex -->
  <!-- Horizontal bar (direction of travel) -->
  <line x1="{bl[0]:.1f}" y1="{bl[1]:.1f}"
        x2="{br[0]:.1f}" y2="{br[1]:.1f}"
        stroke="{LABEL_FILL}" stroke-width="2.5" opacity="0.9"/>
  <!-- Stem along left arm of cone -->
  <line x1="{ax:.1f}" y1="{ay:.1f}"
        x2="{stem_end[0]:.1f}" y2="{stem_end[1]:.1f}"
        stroke="{LABEL_FILL}" stroke-width="2.5" opacity="0.9"/>
  <!-- Right-angle marker at apex junction -->
  <polyline points="{ax + bdx * sq:.1f},{ay + bdy * sq:.1f} {sq_corner[0]:.1f},{sq_corner[1]:.1f} {ax + sdx * sq:.1f},{ay + sdy * sq:.1f}"
            fill="none" stroke="{LABEL_FILL}" stroke-width="1.5" opacity="0.7"/>
  <!-- Label below the bar — bigger font, pushed lower -->
  <text x="{ax:.1f}" y="{label_y:.1f}"
        fill="{LABEL_FILL}" font-size="{c['t_label_font_size']}" font-weight="bold"
        font-family="{FONT}" text-anchor="middle" dominant-baseline="hanging"
        paint-order="stroke" stroke="{LABEL_STROKE}" stroke-width="2.5">
    {c['t_label_text']}
  </text>
</svg>
"""


# ═════════════════════════════════════════════════════════════════════
# SUTRO IMAGE — wide truncated-triangle beam with curved edges,
#               direction arrow along centreline, deep-orange colour
# ═════════════════════════════════════════════════════════════════════

SUTRO_CFG = {
    "jpeg": "guide-aim-sutro_raw.jpg",
    "w": 2000,
    "h": 1500,
    # ── Beam cone ──
    "apex_x_pct": 70.0,  # sensor position — % of width
    "apex_y_pct": 50.0,  # % of height
    "beam_heading_deg": -90,  # pointing left after 90° CW display rotation
    "beam_half_angle_deg": 18.0,  # wider triangle (was 10.5)
    "beam_length_pct": 34,  # slightly longer to fill frame better (was 28)
    # ── Truncation (near edge of trapezoid) ──
    "trunc_frac": 0.20,  # cut first 20% of cone from apex
    # ── Curved edges: cubic Bézier control-point pull fraction ──
    # 0 = straight lines; positive = bow outward (wider at mid-point)
    "curve_bulge": 0.18,
    # ── Chevrons ──
    "chevron_count": 5,
    "chevron_start_pct": 22,  # first chevron at this % of beam length
    "chevron_end_pct": 88,  # last chevron at this %
    "chevron_width_frac": 0.55,  # fraction of cone width at that distance
    # ── Direction arrow ──
    "arrow_head_size_pct": 2.8,  # arrowhead size as % of height
}


def _sutro_svg():
    c = SUTRO_CFG
    W, H = c["w"], c["h"]
    ax, ay = _pct(c["apex_x_pct"], c["apex_y_pct"], W, H)
    h = c["beam_heading_deg"]
    ha = c["beam_half_angle_deg"]
    length = _pct_len(c["beam_length_pct"], H)
    trunc = c["trunc_frac"]
    bulge = c["curve_bulge"]

    # Four corners of the trapezoid
    left_far = _pt((ax, ay), h - ha, length)
    right_far = _pt((ax, ay), h + ha, length)
    near_left = (ax + trunc * (left_far[0] - ax), ay + trunc * (left_far[1] - ay))
    near_right = (ax + trunc * (right_far[0] - ax), ay + trunc * (right_far[1] - ay))

    # ── Curved far edge + straight arms ──
    # The far (wide) edge connecting left_far → right_far bows outward —
    # away from the sensor, i.e. further along the beam heading direction.
    # Control point sits at the midpoint of that edge, pushed forward by
    # bulge × edge_length.
    far_mid = ((left_far[0] + right_far[0]) / 2, (left_far[1] + right_far[1]) / 2)
    far_edge_len = math.hypot(right_far[0] - left_far[0], right_far[1] - left_far[1])
    fdx, fdy = _dir(h)  # unit vector pointing forward (outward from sensor)
    ctrl_far = (
        far_mid[0] + fdx * far_edge_len * bulge,
        far_mid[1] + fdy * far_edge_len * bulge,
    )

    # SVG path: near_left → (straight) → left_far → (curved far edge) →
    #           right_far → (straight) → near_right → (straight near edge) → Z
    path_d = (
        f"M {near_left[0]:.1f},{near_left[1]:.1f} "
        f"L {left_far[0]:.1f},{left_far[1]:.1f} "
        f"Q {ctrl_far[0]:.1f},{ctrl_far[1]:.1f} {right_far[0]:.1f},{right_far[1]:.1f} "
        f"L {near_right[0]:.1f},{near_right[1]:.1f} "
        f"Z"
    )

    # ── Chevrons along centre line — curved to match the far-edge bow ──
    n = c["chevron_count"]
    cs = c["chevron_start_pct"] / 100.0
    ce = c["chevron_end_pct"] / 100.0
    wf = c["chevron_width_frac"]
    chevrons = []
    for i in range(n):
        t = cs + (ce - cs) * i / max(n - 1, 1)
        dist = length * t
        tip = _pt((ax, ay), h, dist)
        half_w = dist * math.tan(math.radians(ha)) * wf
        cl = _pt(tip, h + 90, half_w)
        cr = _pt(tip, h - 90, half_w)
        # Control point bows forward by the same bulge fraction as the far edge
        chev_len = math.hypot(cr[0] - cl[0], cr[1] - cl[1])
        ctrl_chev = (tip[0] + fdx * chev_len * bulge, tip[1] + fdy * chev_len * bulge)
        chevrons.append(
            f'  <path d="M {cl[0]:.1f},{cl[1]:.1f} '
            f'Q {ctrl_chev[0]:.1f},{ctrl_chev[1]:.1f} {cr[0]:.1f},{cr[1]:.1f}"'
            f'\n        fill="none" stroke="{SUTRO_CHEVRON}"'
            f' stroke-width="4.5" stroke-linecap="round"/>'
        )

    chevron_str = "\n".join(chevrons)

    # ── Direction arrow along beam centreline ──
    # Runs from the near edge to the far edge of the cone, pointing "forward"
    # (from sensor outward, i.e. in direction h).
    arrow_start = _pt((ax, ay), h, length * trunc)
    arrow_end = _pt((ax, ay), h, length * 0.78)
    ahs = _pct_len(c["arrow_head_size_pct"], H)
    # Arrowhead: two lines back from tip at ±25°
    ah_l = _pt(arrow_end, h + 180 - 25, ahs)
    ah_r = _pt(arrow_end, h + 180 + 25, ahs)

    # 90° CW rotation: swap viewport dimensions, wrap content in transform.
    return f"""\
<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg"
     xmlns:xlink="http://www.w3.org/1999/xlink"
     width="{H}" height="{W}"
     viewBox="0 0 {H} {W}">
  <g transform="translate({H}, 0) rotate(90)">
  <image href="{c['jpeg']}" width="{W}" height="{H}"/>

  <!-- Beam cone — wide truncated triangle, curved edges, deep orange -->
  <path d="{path_d}"
        fill="{SUTRO_FILL}" stroke="{SUTRO_STROKE}"
        stroke-width="5.0" stroke-linejoin="round"/>

  <!-- Forward-facing chevrons (deep orange) -->
{chevron_str}

  <!-- Direction arrow along centreline (bottom → top of beam) -->
  <line x1="{arrow_start[0]:.1f}" y1="{arrow_start[1]:.1f}"
        x2="{arrow_end[0]:.1f}" y2="{arrow_end[1]:.1f}"
        stroke="{SUTRO_STROKE}" stroke-width="5.5"
        stroke-linecap="round"/>
  <polyline points="{ah_l[0]:.1f},{ah_l[1]:.1f} {arrow_end[0]:.1f},{arrow_end[1]:.1f} {ah_r[0]:.1f},{ah_r[1]:.1f}"
            fill="none" stroke="{SUTRO_STROKE}" stroke-width="5.5"
            stroke-linecap="round" stroke-linejoin="round"/>
  </g>
</svg>
"""


# ── Main ──────────────────────────────────────────────────────────────


def main():
    angel_svg = _angel_svg()
    sutro_svg = _sutro_svg()

    for img_dir in IMG_DIRS:
        angel_out = img_dir / "guide-angel.svg"
        angel_out.write_text(angel_svg)
        print(f"  ✓ {angel_out.relative_to(REPO)}")

        sutro_out = img_dir / "guide-aim-sutro.svg"
        sutro_out.write_text(sutro_svg)
        print(f"  ✓ {sutro_out.relative_to(REPO)}")

    # Rasterise to PNG (browsers block external <image> refs in <img> SVGs).
    # rsvg-convert resolves relative <image href> from the SVG's directory.
    import shutil
    import subprocess

    rsvg = shutil.which("rsvg-convert")
    if not rsvg:
        print("  ⚠ rsvg-convert not found — skipping PNG render")
        print("    Install with: brew install librsvg")
        return

    primary = IMG_DIRS[0]  # public_html/src/images/
    pairs = [
        ("guide-angel.svg", "guide-angel.png"),
        ("guide-aim-sutro.svg", "guide-aim-sutro.png"),
    ]
    for svg_name, png_name in pairs:
        svg_path = primary / svg_name
        for img_dir in IMG_DIRS:
            png_path = img_dir / png_name
            subprocess.run(
                [rsvg, "-w", "1200", str(svg_path), "-o", str(png_path)],
                check=True,
            )
            print(f"  ✓ {png_path.relative_to(REPO)}")


if __name__ == "__main__":
    main()
