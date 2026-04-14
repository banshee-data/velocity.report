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
    "apex_x_pct": 45.0,  # sensor tube opening — % of width
    "apex_y_pct": 44.4,  # % of height
    "beam_heading_deg": 12,  # clockwise from image-up
    "beam_half_angle_deg": 10,  # half of 20° cone
    "beam_length_pct": 38,  # % of height
    # ── Angle arc + label ──
    "arc_radius_pct": 10,  # % of height
    "label_offset_pct": 2.7,  # extra offset beyond arc, % of height
    "label_font_size": 22,
    # ── Direction-of-travel T marker ──
    # Anchored at the beam apex. Bar = direction of travel (perpendicular
    # to beam, pointing right in image coords). Stem = left arm of the V
    # (the left cone edge direction from the apex).
    "t_bar_half_pct": 12.0,  # half-length of horizontal bar — % of width
    "t_stem_pct": 10.0,  # stem length along left edge of cone — % of height
    "t_label_text": "direction of travel →",
    "t_label_font_size": 16,
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
    centre_far = _pt((ax, ay), h, length)

    arc_r = _pct_len(c["arc_radius_pct"], H)
    arc_left = _pt((ax, ay), h - ha, arc_r)
    arc_right = _pt((ax, ay), h + ha, arc_r)
    label_r = arc_r + _pct_len(c["label_offset_pct"], H)
    lbl = _pt((ax, ay), h, label_r)

    # ── Direction-of-travel T, anchored at beam apex ──
    # Horizontal bar: direction of travel is perpendicular to beam heading.
    # Beam heads roughly "up-right" (heading=12°), so direction of travel
    # is 90° to the right of that = heading + 90° rotated from up = left/right
    # in the image.  We draw the bar purely horizontal (left/right) since the
    # road runs horizontally across the photo.
    bar_half = _pct_len(c["t_bar_half_pct"], W)
    stem_len = _pct_len(c["t_stem_pct"], H)

    # Bar runs horizontally through the apex (direction of travel)
    bl = (ax - bar_half, ay)
    br = (ax + bar_half, ay)

    # Stem goes along the left arm of the cone from the apex.
    # Left arm direction: heading - half_angle (clockwise from image-up).
    stem_end = _pt((ax, ay), h - ha, stem_len)

    # Right-angle marker between bar and stem (small square in the angle)
    # Position the square marker offset from the apex along the bar and stem
    sq = 10
    # Find unit vector along stem direction
    sdx, sdy = _dir(h - ha)
    # Unit vector along bar direction (to the right = 90° from image-up)
    bdx, bdy = 1.0, 0.0
    sq_corner = (ax + bdx * sq, ay + sdy * sq)

    return f"""\
<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg"
     xmlns:xlink="http://www.w3.org/1999/xlink"
     width="{W}" height="{H}"
     viewBox="0 0 {W} {H}">
  <image href="{c['jpeg']}" width="{W}" height="{H}"/>

  <!-- Beam cone -->
  <polygon
    points="{ax:.1f},{ay:.1f} {left[0]:.1f},{left[1]:.1f} {right[0]:.1f},{right[1]:.1f}"
    fill="{CONE_FILL}" stroke="{CONE_STROKE}"
    stroke-width="2.5" stroke-linejoin="round"/>

  <!-- Centre beam (dashed) -->
  <line x1="{ax:.1f}" y1="{ay:.1f}"
        x2="{centre_far[0]:.1f}" y2="{centre_far[1]:.1f}"
        stroke="rgba(255,200,0,0.5)" stroke-width="1.5"
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
    {ha * 2}°
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
  <!-- Label below the bar -->
  <text x="{ax:.1f}" y="{ay + 18:.1f}"
        fill="{LABEL_FILL}" font-size="{c['t_label_font_size']}" font-weight="bold"
        font-family="{FONT}" text-anchor="middle" dominant-baseline="hanging"
        paint-order="stroke" stroke="{LABEL_STROKE}" stroke-width="2.5">
    {c['t_label_text']}
  </text>
</svg>
"""


# ═════════════════════════════════════════════════════════════════════
# SUTRO IMAGE — opaque beam cone with forward-facing chevrons
# ═════════════════════════════════════════════════════════════════════

SUTRO_CFG = {
    "jpeg": "guide-aim-sutro_RAW.JPG",
    "w": 2000,
    "h": 1500,
    # ── Beam cone ──
    "apex_x_pct": 50.0,  # sensor position — % of width
    "apex_y_pct": 88.0,  # % of height
    "beam_heading_deg": 0,  # straight up in image (along road)
    "beam_half_angle_deg": 10,  # half of 20° cone
    "beam_length_pct": 56,  # % of height
    # ── Chevrons (forward markers inside the cone) ──
    "chevron_count": 5,
    "chevron_start_pct": 20,  # first chevron at this % of beam length
    "chevron_end_pct": 90,  # last chevron at this %
    "chevron_width_frac": 0.4,  # fraction of cone width at that distance
}


def _sutro_svg():
    c = SUTRO_CFG
    W, H = c["w"], c["h"]
    ax, ay = _pct(c["apex_x_pct"], c["apex_y_pct"], W, H)
    h = c["beam_heading_deg"]
    ha = c["beam_half_angle_deg"]
    length = _pct_len(c["beam_length_pct"], H)

    left = _pt((ax, ay), h - ha, length)
    right = _pt((ax, ay), h + ha, length)

    # ── Chevrons along centre line ──
    n = c["chevron_count"]
    cs = c["chevron_start_pct"] / 100.0
    ce = c["chevron_end_pct"] / 100.0
    wf = c["chevron_width_frac"]
    chevrons = []
    for i in range(n):
        t = cs + (ce - cs) * i / max(n - 1, 1)
        dist = length * t
        tip = _pt((ax, ay), h, dist)
        # Width of cone at this distance (linear interpolation)
        half_w = dist * math.tan(math.radians(ha)) * wf
        cl = _pt(tip, h + 90, half_w)
        cr = _pt(tip, h - 90, half_w)
        # Chevron points back from tip
        back = _pt(tip, h + 180, half_w * 0.6)
        chevrons.append(
            f'  <polyline points="{cl[0]:.1f},{cl[1]:.1f} '
            f'{tip[0]:.1f},{tip[1]:.1f} {cr[0]:.1f},{cr[1]:.1f}"'
            f'\n            fill="none" stroke="rgba(255,200,0,0.7)"'
            f' stroke-width="2.5" stroke-linecap="round"'
            f' stroke-linejoin="round"/>'
        )

    chevron_str = "\n".join(chevrons)

    return f"""\
<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg"
     xmlns:xlink="http://www.w3.org/1999/xlink"
     width="{W}" height="{H}"
     viewBox="0 0 {W} {H}">
  <image href="{c['jpeg']}" width="{W}" height="{H}"/>

  <!-- Beam cone (opaque fill) -->
  <polygon
    points="{ax:.1f},{ay:.1f} {left[0]:.1f},{left[1]:.1f} {right[0]:.1f},{right[1]:.1f}"
    fill="{CONE_FILL}" stroke="{CONE_STROKE}"
    stroke-width="2.5" stroke-linejoin="round"/>

  <!-- Forward-facing chevrons -->
{chevron_str}
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
