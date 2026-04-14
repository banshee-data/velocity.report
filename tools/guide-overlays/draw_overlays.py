#!/usr/bin/env python3
"""Generate annotated guide-image SVGs for setup documentation.

Each SVG embeds the raw JPEG via relative <image> reference and overlays
vector annotations (beam cones, direction lines, angle arcs, labels).

Output:
  public_html/src/images/guide-angel.svg     — radar beam cone overlay
  public_html/src/images/guide-aim-sutro.svg  — traffic/sensor angle overlay
  docs/images/guide-angel.svg                 — copy for docs
  docs/images/guide-aim-sutro.svg             — copy for docs
"""

import math
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent.parent
IMG_DIRS = [REPO / "public_html" / "src" / "images", REPO / "docs" / "images"]

# ── Angel image: radar beam cone ──────────────────────────────────────

ANGEL_W, ANGEL_H = 1200, 900
ANGEL_JPEG = "guide_angel_RAW.jpeg"

# Sensor tube opening (apex of beam triangle) — approximate from photo
ANGEL_APEX = (540, 400)
# Beam aims roughly upward and slightly left in image frame.
# Angle measured clockwise from image-up (negative-Y direction).
ANGEL_BEAM_HEADING_DEG = 12  # degrees clockwise from straight up
ANGEL_BEAM_HALF_ANGLE_DEG = 10  # half-angle of 20° cone
ANGEL_BEAM_LENGTH = 340  # pixels to extend the triangle

# Label
ANGEL_LABEL = "20°"


def _direction(heading_deg):
    """Return (dx, dy) in image coords for heading_deg clockwise from up."""
    r = math.radians(heading_deg)
    return (math.sin(r), -math.cos(r))


def _point_at(origin, heading_deg, dist):
    dx, dy = _direction(heading_deg)
    return (origin[0] + dx * dist, origin[1] + dy * dist)


def _angel_svg():
    ax, ay = ANGEL_APEX
    h = ANGEL_BEAM_HEADING_DEG
    ha = ANGEL_BEAM_HALF_ANGLE_DEG
    length = ANGEL_BEAM_LENGTH

    left = _point_at(ANGEL_APEX, h - ha, length)
    right = _point_at(ANGEL_APEX, h + ha, length)
    centre_far = _point_at(ANGEL_APEX, h, length)

    # Arc for the angle label at a shorter radius
    arc_r = 90
    arc_left = _point_at(ANGEL_APEX, h - ha, arc_r)
    arc_right = _point_at(ANGEL_APEX, h + ha, arc_r)

    # Label position (midway on arc, offset outward)
    label_pos = _point_at(ANGEL_APEX, h, arc_r + 24)

    return f"""\
<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg"
     xmlns:xlink="http://www.w3.org/1999/xlink"
     width="{ANGEL_W}" height="{ANGEL_H}"
     viewBox="0 0 {ANGEL_W} {ANGEL_H}">
  <image href="{ANGEL_JPEG}" width="{ANGEL_W}" height="{ANGEL_H}"/>

  <!-- Beam cone (semi-transparent fill) -->
  <polygon
    points="{ax},{ay} {left[0]:.1f},{left[1]:.1f} {right[0]:.1f},{right[1]:.1f}"
    fill="rgba(255,200,0,0.22)"
    stroke="rgba(255,200,0,0.85)"
    stroke-width="2.5"
    stroke-linejoin="round"/>

  <!-- Centre beam (dashed) -->
  <line x1="{ax}" y1="{ay}"
        x2="{centre_far[0]:.1f}" y2="{centre_far[1]:.1f}"
        stroke="rgba(255,200,0,0.5)"
        stroke-width="1.5"
        stroke-dasharray="10,6"/>

  <!-- Angle arc -->
  <path d="M {arc_left[0]:.1f},{arc_left[1]:.1f}
           A {arc_r},{arc_r} 0 0,1 {arc_right[0]:.1f},{arc_right[1]:.1f}"
        fill="none"
        stroke="white"
        stroke-width="2"
        opacity="0.9"/>

  <!-- Label -->
  <text x="{label_pos[0]:.1f}" y="{label_pos[1]:.1f}"
        fill="white" font-size="22" font-weight="bold"
        font-family="system-ui, -apple-system, sans-serif"
        text-anchor="middle" dominant-baseline="central"
        paint-order="stroke" stroke="rgba(0,0,0,0.6)" stroke-width="3">
    {ANGEL_LABEL}
  </text>
</svg>
"""


# ── Aim-sutro image: traffic/sensor angle ─────────────────────────────

SUTRO_W, SUTRO_H = 2000, 1500
SUTRO_JPEG = "guide-aim-sutro_RAW.JPG"

# Origin point — near sensor in foreground
SUTRO_ORIGIN = (1000, 1330)

# Traffic flow direction (along the road, in image coords).
# The road stretches toward the intersection in the background.
# Heading: clockwise from image-up.
SUTRO_TRAFFIC_HEADING_DEG = -8  # slightly left of straight up
SUTRO_TRAFFIC_LENGTH = 850

# Sensor mount direction — where the tube actually aims.
# Slightly offset from traffic direction.
SUTRO_SENSOR_HEADING_DEG = 8  # slightly right of traffic direction
SUTRO_SENSOR_LENGTH = 850

# Colours
TRAFFIC_COLOUR = "rgba(50,200,255,0.9)"
SENSOR_COLOUR = "rgba(255,80,80,0.9)"


def _sutro_svg():
    ox, oy = SUTRO_ORIGIN
    t_head = SUTRO_TRAFFIC_HEADING_DEG
    s_head = SUTRO_SENSOR_HEADING_DEG
    t_len = SUTRO_TRAFFIC_LENGTH
    s_len = SUTRO_SENSOR_LENGTH

    traffic_end = _point_at(SUTRO_ORIGIN, t_head, t_len)
    sensor_end = _point_at(SUTRO_ORIGIN, s_head, s_len)

    # Angle between the two directions
    angle_deg = abs(s_head - t_head)

    # Arc for angle annotation
    arc_r = 180
    arc_start = _point_at(SUTRO_ORIGIN, t_head, arc_r)
    arc_end = _point_at(SUTRO_ORIGIN, s_head, arc_r)
    # Sweep flag: 1 if sensor heading > traffic heading (clockwise)
    sweep = 1 if s_head > t_head else 0

    # Fill wedge for the angle region
    wedge_r = 160

    # Label at midpoint of arc
    mid_heading = (t_head + s_head) / 2
    label_pos = _point_at(SUTRO_ORIGIN, mid_heading, arc_r + 30)

    # Arrow tips (equilateral triangle heads)
    def _arrowhead(end, heading_deg, size=18):
        """SVG polygon for an arrowhead at `end` pointing along heading."""
        tip = end
        back_l = _point_at(end, heading_deg + 180 - 25, size)
        back_r = _point_at(end, heading_deg + 180 + 25, size)
        return (
            f"{tip[0]:.1f},{tip[1]:.1f} "
            f"{back_l[0]:.1f},{back_l[1]:.1f} "
            f"{back_r[0]:.1f},{back_r[1]:.1f}"
        )

    # Line labels — along each line, offset slightly
    traffic_label_pos = _point_at(SUTRO_ORIGIN, t_head, t_len * 0.55)
    sensor_label_pos = _point_at(SUTRO_ORIGIN, s_head, s_len * 0.55)

    # Offset labels perpendicular to the lines
    t_perp = _direction(t_head + 90)
    s_perp = _direction(s_head - 90)
    tl = (traffic_label_pos[0] + t_perp[0] * 30, traffic_label_pos[1] + t_perp[1] * 30)
    sl = (sensor_label_pos[0] + s_perp[0] * 30, sensor_label_pos[1] + s_perp[1] * 30)

    return f"""\
<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg"
     xmlns:xlink="http://www.w3.org/1999/xlink"
     width="{SUTRO_W}" height="{SUTRO_H}"
     viewBox="0 0 {SUTRO_W} {SUTRO_H}">
  <image href="{SUTRO_JPEG}" width="{SUTRO_W}" height="{SUTRO_H}"/>

  <!-- Angle fill wedge -->
  <path d="M {ox},{oy}
           L {_point_at(SUTRO_ORIGIN, t_head, wedge_r)[0]:.1f},{_point_at(SUTRO_ORIGIN, t_head, wedge_r)[1]:.1f}
           A {wedge_r},{wedge_r} 0 0,{sweep} {_point_at(SUTRO_ORIGIN, s_head, wedge_r)[0]:.1f},{_point_at(SUTRO_ORIGIN, s_head, wedge_r)[1]:.1f}
           Z"
        fill="rgba(255,255,255,0.15)"
        stroke="none"/>

  <!-- Traffic direction line -->
  <line x1="{ox}" y1="{oy}"
        x2="{traffic_end[0]:.1f}" y2="{traffic_end[1]:.1f}"
        stroke="{TRAFFIC_COLOUR}" stroke-width="3"/>
  <polygon points="{_arrowhead(traffic_end, t_head)}"
           fill="{TRAFFIC_COLOUR}"/>

  <!-- Sensor mount direction line -->
  <line x1="{ox}" y1="{oy}"
        x2="{sensor_end[0]:.1f}" y2="{sensor_end[1]:.1f}"
        stroke="{SENSOR_COLOUR}" stroke-width="3"/>
  <polygon points="{_arrowhead(sensor_end, s_head)}"
           fill="{SENSOR_COLOUR}"/>

  <!-- Angle arc -->
  <path d="M {arc_start[0]:.1f},{arc_start[1]:.1f}
           A {arc_r},{arc_r} 0 0,{sweep} {arc_end[0]:.1f},{arc_end[1]:.1f}"
        fill="none"
        stroke="white"
        stroke-width="2.5"
        opacity="0.9"/>

  <!-- Angle label -->
  <text x="{label_pos[0]:.1f}" y="{label_pos[1]:.1f}"
        fill="white" font-size="28" font-weight="bold"
        font-family="system-ui, -apple-system, sans-serif"
        text-anchor="middle" dominant-baseline="central"
        paint-order="stroke" stroke="rgba(0,0,0,0.6)" stroke-width="3">
    {angle_deg:.0f}°
  </text>

  <!-- Traffic direction label -->
  <text x="{tl[0]:.1f}" y="{tl[1]:.1f}"
        fill="{TRAFFIC_COLOUR}" font-size="22" font-weight="bold"
        font-family="system-ui, -apple-system, sans-serif"
        text-anchor="middle" dominant-baseline="central"
        paint-order="stroke" stroke="rgba(0,0,0,0.5)" stroke-width="3">
    traffic direction
  </text>

  <!-- Sensor direction label -->
  <text x="{sl[0]:.1f}" y="{sl[1]:.1f}"
        fill="{SENSOR_COLOUR}" font-size="22" font-weight="bold"
        font-family="system-ui, -apple-system, sans-serif"
        text-anchor="middle" dominant-baseline="central"
        paint-order="stroke" stroke="rgba(0,0,0,0.5)" stroke-width="3">
    sensor direction
  </text>
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


if __name__ == "__main__":
    main()
