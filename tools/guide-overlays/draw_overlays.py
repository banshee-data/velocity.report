#!/usr/bin/env python3
"""Generate annotated guide-image SVGs for setup documentation.

Two diagrams are produced:

  cosign-angle  (was: angel)  — green beam cone, white leg lines, angle arc,
                                 dashed extension lines, direction-of-travel T
  aiming        (was: sutro)  — orange truncated triangle, curved far edge,
                                 curved chevrons, white direction arrow

Each SVG embeds the raw JPEG via relative <image> reference. All positions are
percentages of image dimensions so they survive crops and resizes.

Usage:
    python3 tools/guide-overlays/draw_overlays.py

Output (both locations written identically):
  public_html/src/images/guide-cosign-angle.svg  / .png
  public_html/src/images/guide-aiming.svg        / .png
  docs/images/guide-cosign-angle.svg             / .png
  docs/images/guide-aiming.svg                   / .png
"""

import math
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent.parent
IMG_DIRS = [REPO / "public_html" / "src" / "images", REPO / "docs" / "images"]

# ── Shared ────────────────────────────────────────────────────────────
LABEL_FILL = "white"
LABEL_STROKE = "rgba(0,0,0,0.6)"
FONT = "system-ui, -apple-system, sans-serif"

# UI navbar green — hex #047857 (Tailwind emerald-700)
COSIGN_FILL = "rgba(4,120,87,0.75)"
COSIGN_STROKE = "rgba(4,120,87,0.92)"

# Deep orange for aiming beam
AIMING_FILL = "rgba(230,81,0,0.85)"  # deep-orange-900 tint
AIMING_STROKE = "rgba(230,81,0,0.95)"
AIMING_CHEVRON = "rgba(255,138,0,0.90)"  # deep-orange-A200 — orange chevrons
AIMING_ARROW = "rgba(255,204,128,1)"  # deep-orange-100 — very light orange arrow


# ── Geometry helpers ──────────────────────────────────────────────────


def _dir(heading_deg):
    """Unit vector (dx, dy) for heading_deg clockwise from image-up."""
    r = math.radians(heading_deg)
    return (math.sin(r), -math.cos(r))


def _pt(origin, heading_deg, dist):
    """Point at dist pixels from origin along heading_deg."""
    dx, dy = _dir(heading_deg)
    return (origin[0] + dx * dist, origin[1] + dy * dist)


def _pct(pct_x, pct_y, w, h):
    """Percentage position → pixel coords."""
    return (pct_x / 100.0 * w, pct_y / 100.0 * h)


def _pct_len(pct, ref):
    """Percentage length → pixels."""
    return pct / 100.0 * ref


# ═════════════════════════════════════════════════════════════════════
# COSIGN-ANGLE IMAGE
# Shows: green beam cone, white leg lines extending to tips, arc
# connecting the two tips (radius = beam length), dashed extension
# lines from each tip, small angle arc + label at apex,
# direction-of-travel T marker.
# ═════════════════════════════════════════════════════════════════════

COSIGN_CFG = {
    "jpeg": "guide_angel_RAW.jpeg",
    "w": 1200,
    "h": 900,
    # ── Beam cone ──
    "apex_x_pct": 43.50,  # sensor tube opening — % of width
    "apex_y_pct": 73.1,  # % of height
    "beam_heading_deg": 11,  # clockwise from image-up (1° CW nudge from v1)
    "beam_half_angle_deg": 10.5,  # half of 21° cone
    "beam_length_pct": 38,  # % of height
    # ── Angle arc ──
    "arc_radius_pct": 10,  # small decorative arc near apex — % of height
    "label_offset_pct": 12,  # label offset beyond arc — % of height
    "label_font_size": 32,
    # ── Leg extension lines (dashed, past each tip) ──
    "ext_frac": 0.20,  # extend by this fraction of beam length
    # ── Direction-of-travel T marker ──
    "t_bar_half_pct": 12.0,  # half-length of horizontal bar — % of width
    "t_stem_pct": 10.0,  # stem along left leg — % of height
    "t_label_text": "direction of travel →",
    "t_label_font_size": 34,
    "t_label_dy": 44,  # pixels below bar
}


def _cosign_angle_svg():
    c = COSIGN_CFG
    W, H = c["w"], c["h"]
    ax, ay = _pct(c["apex_x_pct"], c["apex_y_pct"], W, H)
    h = c["beam_heading_deg"]
    ha = c["beam_half_angle_deg"]
    length = _pct_len(c["beam_length_pct"], H)

    # Cone tips
    left = _pt((ax, ay), h - ha, length)
    right = _pt((ax, ay), h + ha, length)

    # Small decorative arc near apex
    arc_r = _pct_len(c["arc_radius_pct"], H)
    arc_left = _pt((ax, ay), h - ha, arc_r)
    arc_right = _pt((ax, ay), h + ha, arc_r)
    label_r = arc_r + _pct_len(c["label_offset_pct"], H)
    lbl = _pt((ax, ay), h, label_r)

    # ── Dashed extension lines from each tip ──
    ext = length * c["ext_frac"]
    ext_left_end = (left[0], left[1] - ext)  # straight up from left tip
    ext_right_end = _pt(right, h + ha, ext)  # along right leg heading

    # ── Direction-of-travel T ──
    bar_half = _pct_len(c["t_bar_half_pct"], W)
    stem_len = _pct_len(c["t_stem_pct"], H)
    bl = (ax - bar_half, ay)
    br = (ax + bar_half, ay)
    stem_end = _pt((ax, ay), h - ha, stem_len)

    # Right-angle marker
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

  <!-- Beam cone — dark green (navbar #047857) -->
  <polygon
    points="{ax:.1f},{ay:.1f} {left[0]:.1f},{left[1]:.1f} {right[0]:.1f},{right[1]:.1f}"
    fill="{COSIGN_FILL}" stroke="{COSIGN_STROKE}"
    stroke-width="2.5" stroke-linejoin="round"/>

  <!-- Cone leg lines — white, apex to each tip -->
  <line x1="{ax:.1f}" y1="{ay:.1f}" x2="{left[0]:.1f}" y2="{left[1]:.1f}"
        stroke="{LABEL_FILL}" stroke-width="2.0" opacity="0.85"/>
  <line x1="{ax:.1f}" y1="{ay:.1f}" x2="{right[0]:.1f}" y2="{right[1]:.1f}"
        stroke="{LABEL_FILL}" stroke-width="2.0" opacity="0.85"/>

  <!-- Top arc connecting the two tips — radius = beam length -->
  <path d="M {left[0]:.1f},{left[1]:.1f}
           A {length:.1f},{length:.1f} 0 0,1 {right[0]:.1f},{right[1]:.1f}"
        fill="none" stroke="{LABEL_FILL}" stroke-width="2.0" opacity="0.85"/>

  <!-- Dashed extension lines past each tip -->
  <line x1="{left[0]:.1f}" y1="{left[1]:.1f}"
        x2="{ext_left_end[0]:.1f}" y2="{ext_left_end[1]:.1f}"
        stroke="{LABEL_FILL}" stroke-width="1.5" opacity="0.75"
        stroke-dasharray="10,6"/>
  <line x1="{right[0]:.1f}" y1="{right[1]:.1f}"
        x2="{ext_right_end[0]:.1f}" y2="{ext_right_end[1]:.1f}"
        stroke="{LABEL_FILL}" stroke-width="1.5" opacity="0.75"
        stroke-dasharray="10,6"/>

  <!-- Small decorative angle arc near apex -->
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

  <!-- Direction-of-travel T -->
  <line x1="{bl[0]:.1f}" y1="{bl[1]:.1f}" x2="{br[0]:.1f}" y2="{br[1]:.1f}"
        stroke="{LABEL_FILL}" stroke-width="2.5" opacity="0.9"/>
  <line x1="{ax:.1f}" y1="{ay:.1f}" x2="{stem_end[0]:.1f}" y2="{stem_end[1]:.1f}"
        stroke="{LABEL_FILL}" stroke-width="2.5" opacity="0.9"/>
  <polyline
    points="{ax + bdx * sq:.1f},{ay + bdy * sq:.1f} {sq_corner[0]:.1f},{sq_corner[1]:.1f} {ax + sdx * sq:.1f},{ay + sdy * sq:.1f}"
    fill="none" stroke="{LABEL_FILL}" stroke-width="1.5" opacity="0.7"/>

  <!-- "direction of travel" label -->
  <text x="{ax:.1f}" y="{label_y:.1f}"
        fill="{LABEL_FILL}" font-size="{c['t_label_font_size']}" font-weight="bold"
        font-family="{FONT}" text-anchor="middle" dominant-baseline="hanging"
        paint-order="stroke" stroke="{LABEL_STROKE}" stroke-width="2.5">
    {c['t_label_text']}
  </text>
</svg>
"""


# ═════════════════════════════════════════════════════════════════════
# AIMING IMAGE
# Shows: wide truncated triangle with straight arms and curved far edge,
# curved chevrons, white direction arrow.  Displayed 90° CW rotated.
# ═════════════════════════════════════════════════════════════════════

AIMING_CFG = {
    "jpeg": "guide-aim-sutro_raw.jpg",
    "w": 2000,
    "h": 1500,
    # ── Beam cone ──
    "apex_x_pct": 69.5,  # sensor position — % of width
    "apex_y_pct": 50.5,  # % of height
    "beam_heading_deg": -90,  # pointing left; 90° CW display rotation → up
    "beam_half_angle_deg": 18.0,  # wide cone (36° total)
    "beam_length_pct": 30,  # % of height
    # ── Truncation ──
    "trunc_frac": 0.20,  # near edge cut at 20% of beam length from apex
    # ── Far-edge bow ──
    # Only the far (wide) edge curves; the two arms are straight.
    # Control point pushed forward by bulge × far-edge-length.
    "curve_bulge": 0.18,
    # ── Chevrons ──
    "chevron_count": 5,
    "chevron_start_pct": 22,  # first chevron at this % of beam length
    "chevron_end_pct": 88,  # last chevron at this %
    "chevron_width_frac": 0.55,  # fraction of cone width at that distance
    # ── Direction arrow ──
    "arrow_head_size_pct": 4,  # arrowhead leg length as % of height
    "arrow_head_angle": 64,  # half-angle of arrowhead — wider/flatter
    "arrow_end_frac": 0.85,  # arrow tip at this fraction of beam length (moved up)
}


def _aiming_svg():
    c = AIMING_CFG
    W, H = c["w"], c["h"]
    ax, ay = _pct(c["apex_x_pct"], c["apex_y_pct"], W, H)
    h = c["beam_heading_deg"]
    ha = c["beam_half_angle_deg"]
    length = _pct_len(c["beam_length_pct"], H)
    trunc = c["trunc_frac"]
    bulge = c["curve_bulge"]

    # ── Four corners of the trapezoid ──
    left_far = _pt((ax, ay), h - ha, length)
    right_far = _pt((ax, ay), h + ha, length)
    near_left = (
        ax + trunc * (left_far[0] - ax),
        ay + trunc * (left_far[1] - ay),
    )
    near_right = (
        ax + trunc * (right_far[0] - ax),
        ay + trunc * (right_far[1] - ay),
    )

    # ── Curved far edge (only this edge bows; arms are straight) ──
    far_mid = (
        (left_far[0] + right_far[0]) / 2,
        (left_far[1] + right_far[1]) / 2,
    )
    far_edge_len = math.hypot(right_far[0] - left_far[0], right_far[1] - left_far[1])
    fdx, fdy = _dir(h)  # forward unit vector (outward from sensor)
    ctrl_far = (
        far_mid[0] + fdx * far_edge_len * bulge,
        far_mid[1] + fdy * far_edge_len * bulge,
    )

    # Path: near_left → (straight arm) → left_far
    #       → (curved far edge Q) → right_far
    #       → (straight arm) → near_right → Z
    path_d = (
        f"M {near_left[0]:.1f},{near_left[1]:.1f} "
        f"L {left_far[0]:.1f},{left_far[1]:.1f} "
        f"Q {ctrl_far[0]:.1f},{ctrl_far[1]:.1f} "
        f"{right_far[0]:.1f},{right_far[1]:.1f} "
        f"L {near_right[0]:.1f},{near_right[1]:.1f} "
        f"Z"
    )

    # ── Chevrons — curved arcs matching the far-edge bow ──
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
        chev_len = math.hypot(cr[0] - cl[0], cr[1] - cl[1])
        ctrl_chev = (
            tip[0] + fdx * chev_len * bulge,
            tip[1] + fdy * chev_len * bulge,
        )
        chevrons.append(
            f'  <path d="M {cl[0]:.1f},{cl[1]:.1f} '
            f'Q {ctrl_chev[0]:.1f},{ctrl_chev[1]:.1f} {cr[0]:.1f},{cr[1]:.1f}"'
            f'\n        fill="none" stroke="{AIMING_CHEVRON}"'
            f' stroke-width="4.5" stroke-linecap="round"/>'
        )
    chevron_str = "\n".join(chevrons)

    # ── Direction arrow along centreline ──
    arrow_start = _pt((ax, ay), h, length * trunc)
    arrow_end = _pt((ax, ay), h, length * c["arrow_end_frac"])
    ahs = _pct_len(c["arrow_head_size_pct"], H)
    aha = c["arrow_head_angle"]
    ah_l = _pt(arrow_end, h + 180 - aha, ahs)
    ah_r = _pt(arrow_end, h + 180 + aha, ahs)

    # 90° CW display rotation
    return f"""\
<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg"
     xmlns:xlink="http://www.w3.org/1999/xlink"
     width="{H}" height="{W}"
     viewBox="0 0 {H} {W}">
  <g transform="translate({H}, 0) rotate(90)">
  <image href="{c['jpeg']}" width="{W}" height="{H}"/>

  <!-- Aiming beam — wide truncated triangle, curved far edge, deep orange -->
  <path d="{path_d}"
        fill="{AIMING_FILL}" stroke="{AIMING_STROKE}"
        stroke-width="5.0" stroke-linejoin="round"/>

  <!-- Curved chevrons -->
{chevron_str}

  <!-- Direction arrow (near edge → 78% of beam length) -->
  <line x1="{arrow_start[0]:.1f}" y1="{arrow_start[1]:.1f}"
        x2="{arrow_end[0]:.1f}" y2="{arrow_end[1]:.1f}"
        stroke="{AIMING_ARROW}" stroke-width="5.5" stroke-linecap="round"/>
  <polyline
    points="{ah_l[0]:.1f},{ah_l[1]:.1f} {arrow_end[0]:.1f},{arrow_end[1]:.1f} {ah_r[0]:.1f},{ah_r[1]:.1f}"
    fill="none" stroke="{AIMING_ARROW}" stroke-width="5.5"
    stroke-linecap="round" stroke-linejoin="round"/>
  </g>
</svg>
"""


# ── Main ──────────────────────────────────────────────────────────────

# Output file stems — new names replacing the old angel/sutro names
OUTPUTS = [
    ("guide-cosign-angle", _cosign_angle_svg),
    ("guide-aiming", _aiming_svg),
]


def main():
    import shutil
    import subprocess

    svgs = [(stem, fn()) for stem, fn in OUTPUTS]

    # ── Step 1: write SVGs ────────────────────────────────────────────
    for img_dir in IMG_DIRS:
        for stem, svg in svgs:
            out = img_dir / f"{stem}.svg"
            out.write_text(svg)
            print(f"  ✓ {out.relative_to(REPO)}")

    # ── Step 2: rasterise SVG → intermediate PNG ─────────────────────
    # rsvg-convert resolves relative <image href> from the SVG's directory.
    rsvg = shutil.which("rsvg-convert")
    if not rsvg:
        print("  ⚠ rsvg-convert not found — skipping PNG/JPEG render")
        print("    Install with: brew install librsvg")
        return

    primary = IMG_DIRS[0]
    tmp_pngs = {}  # stem → Path of intermediate PNG (primary dir only)
    for stem, _ in OUTPUTS:
        svg_path = primary / f"{stem}.svg"
        png_path = primary / f"{stem}.png"
        subprocess.run(
            [rsvg, "-w", "1200", str(svg_path), "-o", str(png_path)],
            check=True,
        )
        tmp_pngs[stem] = png_path
        print(f"  ✓ {png_path.relative_to(REPO)}  (intermediate)")

    # ── Step 3: compress PNG → JPEG ≤ 1023 KB ────────────────────────
    # Binary-search JPEG quality from 92 down to find the highest quality
    # that stays under the 1023 KB threshold.
    try:
        from PIL import Image
    except ImportError:
        print("  ⚠ Pillow not found — skipping JPEG compression")
        print("    Install with: pip install Pillow")
        return

    LIMIT_BYTES = 1023 * 1024

    for stem, png_path in tmp_pngs.items():
        img = Image.open(png_path).convert("RGB")
        lo, hi = 50, 92
        best_data = None
        best_q = lo
        import io

        while lo <= hi:
            mid = (lo + hi) // 2
            buf = io.BytesIO()
            img.save(buf, format="JPEG", quality=mid, optimize=True)
            data = buf.getvalue()
            if len(data) <= LIMIT_BYTES:
                best_data, best_q = data, mid
                lo = mid + 1
            else:
                hi = mid - 1

        if best_data is None:
            # Even q=50 is over limit — save at 50 and warn
            buf = io.BytesIO()
            img.save(buf, format="JPEG", quality=50, optimize=True)
            best_data, best_q = buf.getvalue(), 50
            kb = len(best_data) / 1024
            print(f"  ⚠ {stem}.jpg  {kb:.0f} KB at q=50 — still over 1023 KB limit")

        for img_dir in IMG_DIRS:
            jpg_path = img_dir / f"{stem}.jpg"
            jpg_path.write_bytes(best_data)
            kb = len(best_data) / 1024
            print(f"  ✓ {jpg_path.relative_to(REPO)}  ({kb:.0f} KB, q={best_q})")

        # Remove intermediate PNG — it's a build artefact, not the final output
        for img_dir in IMG_DIRS:
            p = img_dir / f"{stem}.png"
            if p.exists():
                p.unlink()
                print(f"  ✗ {p.relative_to(REPO)}  (intermediate removed)")


if __name__ == "__main__":
    main()
