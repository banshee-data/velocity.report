#!/usr/bin/env python3
"""Generate annotated guide-image SVGs for setup documentation.

Three diagrams are produced:

  cosign-angle  — green beam cone (circular segment), white leg lines, arc
                   connecting tips, dashed extensions, angle label, direction T
  aiming        — orange truncated-triangle beam, curved far edge, margin-spaced
                   chevrons, light-orange direction arrow
  stack         — three caption labels for the HAT stack (RS232 HAT, PoE HAT,
                   Raspberry Pi 4) with short leader lines to each board.

Each SVG embeds the raw JPEG via a relative <image> reference. All positions are
percentages of image dimensions so they survive crops and resizes.

Usage:
    python3 tools/guide-overlays/draw_overlays.py

Output (written to both locations):
  public_html/src/images/guide-cosign-angle.svg / .jpg
  public_html/src/images/guide-aiming.svg       / .jpg
  public_html/src/images/guide-stack.svg        / .jpg
  docs/images/guide-cosign-angle.svg            / .jpg
  docs/images/guide-aiming.svg                  / .jpg
  docs/images/guide-stack.svg                   / .jpg
"""

import io
import math
import shutil
import subprocess
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent.parent
IMG_DIRS = [REPO / "public_html" / "src" / "images", REPO / "docs" / "images"]

# ── Shared palette ────────────────────────────────────────────────────
LABEL_FILL = "white"
LABEL_STROKE = "rgba(0,0,0,0.6)"
FONT = "system-ui, -apple-system, sans-serif"

# UI navbar green — Tailwind emerald-700 #047857
COSIGN_FILL = "rgba(4,120,87,0.75)"
COSIGN_STROKE = "rgba(4,120,87,0.92)"

# Deep orange — Material deep-orange palette
AIMING_FILL = "rgba(230,81,0,0.85)"  # 900 tint
AIMING_STROKE = "rgba(230,81,0,0.95)"  # 900
AIMING_CHEVRON = "rgba(255,138,0,0.90)"  # A200
AIMING_ARROW = "rgba(255,204,128,1)"  # 100 — very light orange


# ── Geometry helpers ──────────────────────────────────────────────────


def _dir(deg):
    """Unit vector (dx, dy) for heading `deg` clockwise from image-up."""
    r = math.radians(deg)
    return math.sin(r), -math.cos(r)


def _pt(origin, deg, dist):
    """Point `dist` pixels from `origin` along heading `deg`."""
    dx, dy = _dir(deg)
    return origin[0] + dx * dist, origin[1] + dy * dist


def _pct_xy(px, py, w, h):
    """Percentage (px%, py%) -> pixel coords for an image of size w x h."""
    return px / 100 * w, py / 100 * h


def _pct(pct, ref):
    """Percentage of `ref` dimension -> pixels."""
    return pct / 100 * ref


# =====================================================================
# COSIGN-ANGLE
# =====================================================================

COSIGN_CFG = {
    "jpeg": "guide_angel_RAW.jpeg",
    "w": 1200,
    "h": 900,
    # Beam cone — lengths as % of image height unless noted
    "apex_x_pct": 43.50,  # % of width
    "apex_y_pct": 73.1,  # % of height
    "beam_heading_deg": 11,  # clockwise from image-up
    "beam_half_angle_deg": 10.5,  # half of full 21 deg cone
    "beam_length_pct": 38,  # % of height — also used as arc radius
    # Angle label arc (small, decorative, near apex)
    "arc_radius_pct": 10,  # % of height
    "label_offset_pct": 12,  # extra offset beyond arc — % of height
    "label_font_size": 32,
    # Dashed extensions past each tip
    "ext_frac": 0.20,  # fraction of beam_length
    # Direction-of-travel T marker
    "t_bar_half_pct": 12.0,  # half bar length — % of width
    "t_stem_pct": 10.0,  # stem length along left leg — % of height
    "t_label_text": "direction of travel →",
    "t_label_font_size": 34,
    "t_label_dy": 44,  # pixels below bar baseline
}


def _cosign_angle_svg():
    c = COSIGN_CFG
    W, H = c["w"], c["h"]
    ax, ay = _pct_xy(c["apex_x_pct"], c["apex_y_pct"], W, H)
    hdg = c["beam_heading_deg"]
    ha = c["beam_half_angle_deg"]
    blen = _pct(c["beam_length_pct"], H)

    left = _pt((ax, ay), hdg - ha, blen)
    right = _pt((ax, ay), hdg + ha, blen)

    arc_r = _pct(c["arc_radius_pct"], H)
    arc_left = _pt((ax, ay), hdg - ha, arc_r)
    arc_right = _pt((ax, ay), hdg + ha, arc_r)
    lbl = _pt((ax, ay), hdg, arc_r + _pct(c["label_offset_pct"], H))

    ext = blen * c["ext_frac"]
    ext_left_end = (left[0], left[1] - ext)
    ext_right_end = _pt(right, hdg + ha, ext)

    bar_half = _pct(c["t_bar_half_pct"], W)
    stem_end = _pt((ax, ay), hdg - ha, _pct(c["t_stem_pct"], H))
    bl = (ax - bar_half, ay)
    br = (ax + bar_half, ay)

    sq = 10
    sdx, sdy = _dir(hdg - ha)
    sq_corner = (ax + sq, ay + sdy * sq)
    label_y = ay + c["t_label_dy"]

    return f"""\
<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"
     width="{W}" height="{H}" viewBox="0 0 {W} {H}">
  <image href="{c['jpeg']}" width="{W}" height="{H}"/>

  <!-- Beam: circular segment (two straight legs + arc top) -->
  <path d="M {ax:.1f},{ay:.1f}
           L {left[0]:.1f},{left[1]:.1f}
           A {blen:.1f},{blen:.1f} 0 0,1 {right[0]:.1f},{right[1]:.1f} Z"
        fill="{COSIGN_FILL}" stroke="{COSIGN_STROKE}"
        stroke-width="2.5" stroke-linejoin="round"/>

  <!-- Leg lines (white, drawn over fill) -->
  <line x1="{ax:.1f}" y1="{ay:.1f}" x2="{left[0]:.1f}"  y2="{left[1]:.1f}"
        stroke="{LABEL_FILL}" stroke-width="2.0" opacity="0.85"/>
  <line x1="{ax:.1f}" y1="{ay:.1f}" x2="{right[0]:.1f}" y2="{right[1]:.1f}"
        stroke="{LABEL_FILL}" stroke-width="2.0" opacity="0.85"/>

  <!-- Arc top (white, drawn over fill stroke) -->
  <path d="M {left[0]:.1f},{left[1]:.1f}
           A {blen:.1f},{blen:.1f} 0 0,1 {right[0]:.1f},{right[1]:.1f}"
        fill="none" stroke="{LABEL_FILL}" stroke-width="2.0" opacity="0.85"/>

  <!-- Dashed extensions past each tip -->
  <line x1="{left[0]:.1f}"  y1="{left[1]:.1f}"
        x2="{ext_left_end[0]:.1f}"  y2="{ext_left_end[1]:.1f}"
        stroke="{LABEL_FILL}" stroke-width="1.5" opacity="0.75" stroke-dasharray="10,6"/>
  <line x1="{right[0]:.1f}" y1="{right[1]:.1f}"
        x2="{ext_right_end[0]:.1f}" y2="{ext_right_end[1]:.1f}"
        stroke="{LABEL_FILL}" stroke-width="1.5" opacity="0.75" stroke-dasharray="10,6"/>

  <!-- Decorative angle arc near apex -->
  <path d="M {arc_left[0]:.1f},{arc_left[1]:.1f}
           A {arc_r:.1f},{arc_r:.1f} 0 0,1 {arc_right[0]:.1f},{arc_right[1]:.1f}"
        fill="none" stroke="{LABEL_FILL}" stroke-width="2" opacity="0.9"/>

  <!-- Angle label -->
  <text x="{lbl[0]:.1f}" y="{lbl[1]:.1f}"
        fill="{LABEL_FILL}" font-size="{c['label_font_size']}" font-weight="bold"
        font-family="{FONT}" text-anchor="middle" dominant-baseline="central"
        paint-order="stroke" stroke="{LABEL_STROKE}" stroke-width="3">
    {ha * 2:.0f}
  </text>

  <!-- Direction-of-travel T -->
  <line x1="{bl[0]:.1f}" y1="{bl[1]:.1f}" x2="{br[0]:.1f}" y2="{br[1]:.1f}"
        stroke="{LABEL_FILL}" stroke-width="2.5" opacity="0.9"/>
  <line x1="{ax:.1f}" y1="{ay:.1f}" x2="{stem_end[0]:.1f}" y2="{stem_end[1]:.1f}"
        stroke="{LABEL_FILL}" stroke-width="2.5" opacity="0.9"/>
  <polyline points="{ax + sq:.1f},{ay:.1f} {sq_corner[0]:.1f},{sq_corner[1]:.1f} {ax + sdx * sq:.1f},{ay + sdy * sq:.1f}"
            fill="none" stroke="{LABEL_FILL}" stroke-width="1.5" opacity="0.7"/>

  <!-- Direction label -->
  <text x="{ax:.1f}" y="{label_y:.1f}"
        fill="{LABEL_FILL}" font-size="{c['t_label_font_size']}" font-weight="bold"
        font-family="{FONT}" text-anchor="middle" dominant-baseline="hanging"
        paint-order="stroke" stroke="{LABEL_STROKE}" stroke-width="11">
    {c['t_label_text']}
  </text>
</svg>
"""


# =====================================================================
# AIMING
# Portrait image (1038x1384). Sensor (white box) at bottom-centre;
# beam fires straight up (heading 0 = image-up).
# =====================================================================

AIMING_CFG = {
    "jpeg": "guide-aim-sutro_raw.jpg",
    "w": 1038,
    "h": 1384,
    # Beam cone
    "apex_x_pct": 52.0,  # sensor centre — % of width
    "apex_y_pct": 91.5,  # sensor centre — % of height
    "beam_heading_deg": 0,  # straight up (image-up)
    "beam_half_angle_deg": 18.0,  # half of 36 deg cone
    "beam_length_pct": 25,  # % of height
    # Near edge: truncation point and flare angle.
    # near edge sits at trunc_frac * beam_length from apex.
    # near_half_angle_deg must be > beam_half_angle_deg to show the flared mouth.
    # chevron_start_pct must be > trunc_frac*100 (enforced at runtime).
    "trunc_frac": 0.16,  # fraction of beam_length
    "near_half_angle_deg": 50.0,  # wider than beam_half_angle_deg (18)
    # Far edge bow (Bezier bulge as fraction of far-edge length)
    "curve_bulge": 0.18,
    # Chevrons: each arc inset by a constant pixel margin from the cone walls,
    # so the gap is uniform regardless of distance.
    "chevron_count": 5,
    "chevron_start_pct": 20,  # % of beam_length (must be > trunc_frac*100)
    "chevron_end_pct": 88,  # % of beam_length
    "chevron_margin_pct": 1.8,  # constant inset each side — % of height
    # Direction arrow
    "arrow_end_frac": 0.85,  # fraction of beam_length to arrowhead tip
    "arrow_head_size_pct": 4,  # arrowhead leg length — % of height
    "arrow_head_angle": 64,  # half-angle of arrowhead (wide/flat)
}


def _aiming_svg():
    c = AIMING_CFG
    W, H = c["w"], c["h"]
    ax, ay = _pct_xy(c["apex_x_pct"], c["apex_y_pct"], W, H)
    hdg = c["beam_heading_deg"]
    ha = c["beam_half_angle_deg"]
    blen = _pct(c["beam_length_pct"], H)

    assert (
        c["chevron_start_pct"] > c["trunc_frac"] * 100
    ), "chevron_start_pct must exceed trunc_frac*100 or chevrons fall in the cut zone"

    near_dist = blen * c["trunc_frac"]
    near_ha = c["near_half_angle_deg"]

    left_far = _pt((ax, ay), hdg - ha, blen)
    right_far = _pt((ax, ay), hdg + ha, blen)
    near_left = _pt((ax, ay), hdg - near_ha, near_dist)
    near_right = _pt((ax, ay), hdg + near_ha, near_dist)

    fdx, fdy = _dir(hdg)
    far_mid = ((left_far[0] + right_far[0]) / 2, (left_far[1] + right_far[1]) / 2)
    far_len = math.hypot(right_far[0] - left_far[0], right_far[1] - left_far[1])
    ctrl_far = (
        far_mid[0] + fdx * far_len * c["curve_bulge"],
        far_mid[1] + fdy * far_len * c["curve_bulge"],
    )

    path_d = (
        f"M {near_left[0]:.1f},{near_left[1]:.1f} "
        f"L {left_far[0]:.1f},{left_far[1]:.1f} "
        f"Q {ctrl_far[0]:.1f},{ctrl_far[1]:.1f} {right_far[0]:.1f},{right_far[1]:.1f} "
        f"L {near_right[0]:.1f},{near_right[1]:.1f} Z"
    )

    margin = _pct(c["chevron_margin_pct"], H)
    n = c["chevron_count"]
    cs = c["chevron_start_pct"] / 100
    ce = c["chevron_end_pct"] / 100
    chevrons = []
    for i in range(n):
        t = cs + (ce - cs) * i / max(n - 1, 1)
        dist = blen * t
        tip = _pt((ax, ay), hdg, dist)
        half_w = max(0.0, dist * math.tan(math.radians(ha)) - margin)
        cl = _pt(tip, hdg + 90, half_w)
        cr = _pt(tip, hdg - 90, half_w)
        chev_len = math.hypot(cr[0] - cl[0], cr[1] - cl[1])
        ctrl_chev = (
            tip[0] + fdx * chev_len * c["curve_bulge"],
            tip[1] + fdy * chev_len * c["curve_bulge"],
        )
        chevrons.append(
            f'  <path d="M {cl[0]:.1f},{cl[1]:.1f} '
            f'Q {ctrl_chev[0]:.1f},{ctrl_chev[1]:.1f} {cr[0]:.1f},{cr[1]:.1f}"'
            f' fill="none" stroke="{AIMING_CHEVRON}"'
            f' stroke-width="4.5" stroke-linecap="round"/>'
        )
    chevron_str = "\n".join(chevrons)

    arrow_start = _pt((ax, ay), hdg, near_dist)
    arrow_end = _pt((ax, ay), hdg, blen * c["arrow_end_frac"])
    ahs = _pct(c["arrow_head_size_pct"], H)
    aha = c["arrow_head_angle"]
    ah_l = _pt(arrow_end, hdg + 180 - aha, ahs)
    ah_r = _pt(arrow_end, hdg + 180 + aha, ahs)

    return f"""\
<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"
     width="{W}" height="{H}" viewBox="0 0 {W} {H}">
  <image href="{c['jpeg']}" width="{W}" height="{H}"/>

  <!-- Aiming beam — truncated triangle, curved far edge -->
  <path d="{path_d}"
        fill="{AIMING_FILL}" stroke="{AIMING_STROKE}"
        stroke-width="5.0" stroke-linejoin="round"/>

  <!-- Chevrons (margin-inset, curved) -->
{chevron_str}

  <!-- Direction arrow -->
  <line x1="{arrow_start[0]:.1f}" y1="{arrow_start[1]:.1f}"
        x2="{arrow_end[0]:.1f}"   y2="{arrow_end[1]:.1f}"
        stroke="{AIMING_ARROW}" stroke-width="5.5" stroke-linecap="round"/>
  <polyline points="{ah_l[0]:.1f},{ah_l[1]:.1f} {arrow_end[0]:.1f},{arrow_end[1]:.1f} {ah_r[0]:.1f},{ah_r[1]:.1f}"
            fill="none" stroke="{AIMING_ARROW}" stroke-width="5.5"
            stroke-linecap="round" stroke-linejoin="round"/>
</svg>
"""


# =====================================================================
# STACK
# Labels for the three boards visible in the HAT stack enclosure:
#   RS232 HAT (DB9 connector) — top
#   PoE HAT                    — middle
#   Raspberry Pi 4             — bottom
# Each label sits to the left of the stack, right-aligned, with a short
# leader line pointing rightward to the relevant board.
# =====================================================================

STACK_CFG = {
    "jpeg": "guide-stack-raw.jpg",
    "w": 2000,
    "h": 1500,
    "label_font_size": 72,
    "label_stroke_width": 20,
    "leader_stroke_width": 4,
    # Label anchor x (% of width) — right edge of each (right-aligned) text label
    "label_x_pct": 30.0,
    # Leader tip x (% of width) — where the leader line touches the board
    "leader_tip_x_pct": 45.0,
    "labels": [
        {"text": "RS232 HAT", "y_pct": 47.0},
        {"text": "PoE HAT", "y_pct": 53.0},
        {"text": "Raspberry Pi 4", "y_pct": 67.0},
    ],
}


def _stack_svg():
    c = STACK_CFG
    W, H = c["w"], c["h"]
    lx = _pct(c["label_x_pct"], W)
    tip_x = _pct(c["leader_tip_x_pct"], W)

    parts = []
    for lab in c["labels"]:
        ly = _pct(lab["y_pct"], H)
        parts.append(
            f'  <line x1="{lx + 20:.1f}" y1="{ly:.1f}" x2="{tip_x:.1f}" y2="{ly:.1f}" '
            f'stroke="{LABEL_FILL}" stroke-width="{c["leader_stroke_width"]}" '
            f'opacity="0.9" stroke-linecap="round"/>'
        )
        parts.append(
            f'  <text x="{lx:.1f}" y="{ly:.1f}" '
            f'fill="{LABEL_FILL}" font-size="{c["label_font_size"]}" font-weight="bold" '
            f'font-family="{FONT}" text-anchor="end" dominant-baseline="central" '
            f'paint-order="stroke" stroke="{LABEL_STROKE}" '
            f'stroke-width="{c["label_stroke_width"]}">{lab["text"]}</text>'
        )
    body = "\n".join(parts)

    return f"""\
<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"
     width="{W}" height="{H}" viewBox="0 0 {W} {H}">
  <image href="{c['jpeg']}" width="{W}" height="{H}"/>

{body}
</svg>
"""


# ── Output manifest ───────────────────────────────────────────────────

OUTPUTS = [
    ("guide-cosign-angle", _cosign_angle_svg),
    ("guide-aiming", _aiming_svg),
    ("guide-stack", _stack_svg),
]


# ── Main ──────────────────────────────────────────────────────────────


def main():
    svgs = [(stem, fn()) for stem, fn in OUTPUTS]

    # Step 1 — write SVGs
    for img_dir in IMG_DIRS:
        for stem, svg in svgs:
            out = img_dir / f"{stem}.svg"
            out.write_text(svg)
            print(f"  OK {out.relative_to(REPO)}")

    # Step 2 — rasterise SVG -> intermediate PNG via rsvg-convert
    rsvg = shutil.which("rsvg-convert")
    if not rsvg:
        print("  WARN rsvg-convert not found — skipping PNG/JPEG render")
        print("    Install: brew install librsvg")
        return

    primary = IMG_DIRS[0]
    tmp_pngs = {}
    for stem, _ in OUTPUTS:
        png = primary / f"{stem}.png"
        subprocess.run(
            [rsvg, "-w", "1200", str(primary / f"{stem}.svg"), "-o", str(png)],
            check=True,
        )
        tmp_pngs[stem] = png
        print(f"  OK {png.relative_to(REPO)}  (intermediate)")

    # Step 3 — compress PNG -> JPEG <= 1023 KB (binary-search on quality)
    try:
        from PIL import Image
    except ImportError:
        print("  WARN Pillow not found — skipping JPEG compression")
        print("    Install: pip install Pillow")
        return

    LIMIT = 1023 * 1024

    for stem, png in tmp_pngs.items():
        img = Image.open(png).convert("RGB")
        lo, hi, best_data, best_q = 50, 92, None, 50
        while lo <= hi:
            mid = (lo + hi) // 2
            buf = io.BytesIO()
            img.save(buf, format="JPEG", quality=mid, optimize=True)
            data = buf.getvalue()
            if len(data) <= LIMIT:
                best_data, best_q = data, mid
                lo = mid + 1
            else:
                hi = mid - 1
        if best_data is None:
            buf = io.BytesIO()
            img.save(buf, format="JPEG", quality=50, optimize=True)
            best_data, best_q = buf.getvalue(), 50
            print(
                f"  WARN {stem}.jpg still over limit at q=50 ({len(best_data)//1024} KB)"
            )

        for img_dir in IMG_DIRS:
            jpg = img_dir / f"{stem}.jpg"
            jpg.write_bytes(best_data)
            print(
                f"  OK {jpg.relative_to(REPO)}  ({len(best_data)//1024} KB, q={best_q})"
            )

        for img_dir in IMG_DIRS:
            p = img_dir / f"{stem}.png"
            if p.exists():
                p.unlink()
                print(f"  RM {p.relative_to(REPO)}  (intermediate)")


if __name__ == "__main__":
    main()
