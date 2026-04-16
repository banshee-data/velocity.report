"""
components.py — Parametric SVG drawing primitives for the rack-mount drawing.

All coordinates are in SVG pixels (Y increases downward).
Callers pass pre-scaled values; this module knows nothing about inches.
"""

import math
import drawsvg as draw

# ── Visual constants ──────────────────────────────────────────────────────────

WOOD_FACE = "#f5deb3"  # wheat — long-grain face
WOOD_ENDGRAIN = "#d2a679"  # darker — end-grain band
WOOD_STROKE = "#7a5c2e"

PIPE_FILL = "#cccccc"
PIPE_STROKE = "#555555"

BRACE_FILL = "#909090"
BRACE_STROKE = "#444444"

CLAMP_FILL = "#b0b0b0"
CLAMP_STROKE = "#333333"

DIM_COLOUR = "#222222"

LINE_W = 1.0
DIM_W = 0.75
SEAM_W = 0.75

FONT = "Arial, sans-serif"
MONO = "Courier New, monospace"

FS_DIM = 9
FS_LABEL = 10
FS_TITLE = 13
FS_HEAD = 15


# ── Arrowhead helper (no SVG markers needed) ──────────────────────────────────


def _arrowhead(
    d: draw.Drawing,
    x: float,
    y: float,
    dx: float,
    dy: float,
    size: float = 7.0,
) -> None:
    """Small filled triangle at (x, y) pointing in direction (dx, dy)."""
    length = math.hypot(dx, dy)
    if length < 1e-9:
        return
    ux, uy = dx / length, dy / length
    px, py = -uy, ux  # perpendicular unit vector
    w = size / 3.0
    pts = [
        x,
        y,
        x - ux * size + px * w,
        y - uy * size + py * w,
        x - ux * size - px * w,
        y - uy * size - py * w,
    ]
    d.append(draw.Lines(*pts, fill=DIM_COLOUR, close=True, stroke="none"))


# ── Lumber ────────────────────────────────────────────────────────────────────


def lumber_rect(
    d: draw.Drawing,
    x: float,
    y: float,
    w: float,
    h: float,
    end_left: bool = False,
    end_right: bool = False,
) -> None:
    """
    Rectangle for a lumber piece (face view).
    end_left / end_right: shade end-grain on that side.
    """
    d.append(
        draw.Rectangle(
            x, y, w, h, fill=WOOD_FACE, stroke=WOOD_STROKE, stroke_width=LINE_W
        )
    )
    eg = min(abs(w), abs(h)) * 0.30
    if end_right and abs(w) > eg:
        d.append(
            draw.Rectangle(x + w - eg, y, eg, h, fill=WOOD_ENDGRAIN, stroke="none")
        )
        d.append(
            draw.Line(x + w, y, x + w, y + h, stroke=WOOD_STROKE, stroke_width=LINE_W)
        )
    if end_left and abs(w) > eg:
        d.append(draw.Rectangle(x, y, eg, h, fill=WOOD_ENDGRAIN, stroke="none"))
        d.append(draw.Line(x, y, x, y + h, stroke=WOOD_STROKE, stroke_width=LINE_W))


def miter_seam(
    d: draw.Drawing,
    x1: float,
    y1: float,
    x2: float,
    y2: float,
) -> None:
    """
    Draw the 45-degree miter joint seam as a dashed line across the
    assembled corner.
    """
    d.append(
        draw.Line(
            x1,
            y1,
            x2,
            y2,
            stroke=WOOD_STROKE,
            stroke_width=SEAM_W,
            stroke_dasharray="3,2",
        )
    )


# ── Pipe ──────────────────────────────────────────────────────────────────────


def pipe_circle(
    d: draw.Drawing,
    cx: float,
    cy: float,
    r: float,
) -> None:
    """
    Draw a pipe cross-section (looking along the axis).
    Outer circle + inner circle for wall thickness + centre mark.
    """
    wall = max(r * 0.08, 2.0)
    d.append(
        draw.Circle(cx, cy, r, fill=PIPE_FILL, stroke=PIPE_STROKE, stroke_width=LINE_W)
    )
    d.append(
        draw.Circle(
            cx, cy, r - wall, fill="#f0f0f0", stroke=PIPE_STROKE, stroke_width=0.5
        )
    )
    cs = 4
    d.append(draw.Line(cx - cs, cy, cx + cs, cy, stroke=PIPE_STROKE, stroke_width=0.5))
    d.append(draw.Line(cx, cy - cs, cx, cy + cs, stroke=PIPE_STROKE, stroke_width=0.5))


# ── Hose clamp (elevation view) ───────────────────────────────────────────────


def hose_clamp(
    d: draw.Drawing,
    cx: float,
    cy: float,
    r: float,
) -> None:
    """
    Draw a worm-drive hose clamp band around a circle of radius r,
    centred at (cx, cy).  Elevation view.
    """
    band_w = 5.0
    # Band arc — draw as a thick circle (full ring) around the pipe
    d.append(
        draw.Circle(cx, cy, r, fill="none", stroke=CLAMP_STROKE, stroke_width=band_w)
    )
    # Worm-screw housing box (at 2 o'clock position)
    angle = math.radians(-30)
    hx = cx + (r + band_w / 2) * math.cos(angle)
    hy = cy + (r + band_w / 2) * math.sin(angle)
    bw, bh = 14, 10
    d.append(
        draw.Rectangle(
            hx - bw / 2,
            hy - bh / 2,
            bw,
            bh,
            fill=CLAMP_FILL,
            stroke=CLAMP_STROKE,
            stroke_width=0.75,
        )
    )
    # Screw slot line
    d.append(
        draw.Line(
            hx, hy - bh / 2, hx, hy + bh / 2, stroke=CLAMP_STROKE, stroke_width=0.5
        )
    )


# ── Steel corner brace ────────────────────────────────────────────────────────


def corner_brace(
    d: draw.Drawing,
    x: float,
    y: float,
    leg: float,
    flip_x: bool = False,
    flip_y: bool = False,
) -> None:
    """
    Draw a flat steel corner brace.
    (x, y) is the inside corner; leg is the arm length in px.
    flip_x / flip_y mirror for the four possible orientations.
    """
    sx = -1 if flip_x else 1
    sy = -1 if flip_y else 1
    thick = 4.5

    # Horizontal arm
    d.append(
        draw.Rectangle(
            x,
            y,
            sx * leg,
            sy * thick,
            fill=BRACE_FILL,
            stroke=BRACE_STROKE,
            stroke_width=0.75,
        )
    )
    # Vertical arm
    d.append(
        draw.Rectangle(
            x,
            y,
            sx * thick,
            sy * leg,
            fill=BRACE_FILL,
            stroke=BRACE_STROKE,
            stroke_width=0.75,
        )
    )
    # Screw holes (two per arm)
    hr = 2.0
    for t in (0.28, 0.72):
        d.append(
            draw.Circle(
                x + sx * leg * t,
                y + sy * thick / 2,
                hr,
                fill="white",
                stroke=BRACE_STROKE,
                stroke_width=0.5,
            )
        )
        d.append(
            draw.Circle(
                x + sx * thick / 2,
                y + sy * leg * t,
                hr,
                fill="white",
                stroke=BRACE_STROKE,
                stroke_width=0.5,
            )
        )


# ── Dimension lines ───────────────────────────────────────────────────────────


def dim_h(
    d: draw.Drawing,
    x1: float,
    x2: float,
    y: float,
    text: str,
    ext: float = 10.0,
    tick: float = 5.0,
) -> None:
    """Horizontal dimension line at y, spanning x1 to x2."""
    d.append(draw.Line(x1, y, x2, y, stroke=DIM_COLOUR, stroke_width=DIM_W))
    _arrowhead(d, x1, y, x1 - x2, 0)
    _arrowhead(d, x2, y, x2 - x1, 0)
    for x in (x1, x2):
        d.append(
            draw.Line(x, y - tick, x, y + tick, stroke=DIM_COLOUR, stroke_width=DIM_W)
        )
    mx = (x1 + x2) / 2
    d.append(
        draw.Text(
            text,
            FS_DIM,
            mx,
            y - ext,
            text_anchor="middle",
            font_family=MONO,
            fill=DIM_COLOUR,
        )
    )


def dim_v(
    d: draw.Drawing,
    x: float,
    y1: float,
    y2: float,
    text: str,
    ext: float = 12.0,
    tick: float = 5.0,
) -> None:
    """Vertical dimension line at x, spanning y1 to y2."""
    d.append(draw.Line(x, y1, x, y2, stroke=DIM_COLOUR, stroke_width=DIM_W))
    _arrowhead(d, x, y1, 0, y1 - y2)
    _arrowhead(d, x, y2, 0, y2 - y1)
    for y in (y1, y2):
        d.append(
            draw.Line(x - tick, y, x + tick, y, stroke=DIM_COLOUR, stroke_width=DIM_W)
        )
    my = (y1 + y2) / 2
    d.append(
        draw.Text(
            text,
            FS_DIM,
            x - ext,
            my,
            text_anchor="end",
            dominant_baseline="middle",
            font_family=MONO,
            fill=DIM_COLOUR,
        )
    )


def leader(
    d: draw.Drawing,
    tip_x: float,
    tip_y: float,
    label_x: float,
    label_y: float,
    text: str,
    anchor: str = "start",
) -> None:
    """Leader line from a feature to a text label."""
    d.append(
        draw.Line(tip_x, tip_y, label_x, label_y, stroke=DIM_COLOUR, stroke_width=DIM_W)
    )
    _arrowhead(d, tip_x, tip_y, tip_x - label_x, tip_y - label_y, size=5)
    pad = 4 if anchor == "start" else -4
    d.append(
        draw.Text(
            text,
            FS_LABEL,
            label_x + pad,
            label_y,
            text_anchor=anchor,
            dominant_baseline="middle",
            font_family=FONT,
            fill=DIM_COLOUR,
        )
    )


# ── BOM table ─────────────────────────────────────────────────────────────────

_COL_WIDTHS = [30, 260, 50, 60, 90]  # icon, desc, qty, unit$, item#
_COL_HEADS = ["", "Item", "Qty", "Price", "Lowe's #"]


def bom_table(
    d: draw.Drawing,
    x: float,
    y: float,
    items: list,
    title: str,
) -> float:
    """
    Render a Bill-of-Materials table.
    Returns the y-coordinate immediately below the last row.
    """
    cw = _COL_WIDTHS
    total_w = sum(cw)
    row_h = 26
    header_h = 28

    # ── Subassembly title bar ─────────────────────────────────────────────
    d.append(
        draw.Rectangle(
            x, y, total_w, header_h, fill="#1e3a5f", stroke="#1e3a5f", stroke_width=0
        )
    )
    d.append(
        draw.Text(
            title,
            FS_TITLE,
            x + total_w / 2,
            y + header_h / 2,
            text_anchor="middle",
            dominant_baseline="middle",
            font_family=FONT,
            fill="white",
            font_weight="bold",
        )
    )
    y += header_h

    # ── Column headers ────────────────────────────────────────────────────
    d.append(
        draw.Rectangle(
            x, y, total_w, row_h, fill="#3a6ea5", stroke="#1e3a5f", stroke_width=0.5
        )
    )
    cx = x
    for i, (hdr, w) in enumerate(zip(_COL_HEADS, cw)):
        anchor = "start" if i == 0 else "middle"
        tx = cx + 3 if i == 0 else cx + w / 2
        d.append(
            draw.Text(
                hdr,
                FS_LABEL,
                tx,
                y + row_h / 2,
                text_anchor=anchor,
                dominant_baseline="middle",
                font_family=FONT,
                fill="white",
                font_weight="bold",
            )
        )
        cx += w
    y += row_h

    # ── Data rows ─────────────────────────────────────────────────────────
    total_cost = 0.0
    for idx, item in enumerate(items):
        row_fill = "#f4f7fb" if idx % 2 == 0 else "#e7edf6"
        d.append(
            draw.Rectangle(
                x, y, total_w, row_h, fill=row_fill, stroke="#aabccc", stroke_width=0.4
            )
        )
        line_total = item["qty"] * item["unit_price"]
        total_cost += line_total
        qty_str = f"{item['qty']} {item['unit']}"
        values = [
            item.get("icon", ""),
            item["description"],
            qty_str,
            f"${item['unit_price']:.2f}",
            item.get("aisle", ""),
            item.get("lowes_item", ""),
        ]
        cx = x
        for i, (val, w) in enumerate(zip(values, cw)):
            anchor = "middle" if i == 0 else ("start" if i == 1 else "middle")
            tx = cx + w / 2 if i == 0 else (cx + 3 if i == 1 else cx + w / 2)
            fs = FS_LABEL + 2 if i == 0 else FS_LABEL
            d.append(
                draw.Text(
                    val,
                    fs,
                    tx,
                    y + row_h / 2,
                    text_anchor=anchor,
                    dominant_baseline="middle",
                    font_family=FONT,
                    fill="#1a1a2e",
                )
            )
            cx += w
        # Vertical separator lines
        cx = x
        for w in cw[:-1]:
            cx += w
            d.append(
                draw.Line(cx, y, cx, y + row_h, stroke="#aabccc", stroke_width=0.3)
            )
        y += row_h

    # ── Total row ─────────────────────────────────────────────────────────
    total_h = row_h + 2
    d.append(
        draw.Rectangle(
            x, y, total_w, total_h, fill="#1e3a5f", stroke="#1e3a5f", stroke_width=0
        )
    )
    d.append(
        draw.Text(
            "TOTAL — Sensor Mount Subassembly",
            FS_LABEL,
            x + 3,
            y + total_h / 2,
            text_anchor="start",
            dominant_baseline="middle",
            font_family=FONT,
            fill="white",
            font_weight="bold",
        )
    )
    total_x = x + sum(cw[:3]) + cw[3] / 2
    d.append(
        draw.Text(
            f"${total_cost:.2f}",
            FS_LABEL,
            total_x,
            y + total_h / 2,
            text_anchor="middle",
            dominant_baseline="middle",
            font_family=FONT,
            fill="white",
            font_weight="bold",
        )
    )
    return y + total_h
