#!/usr/bin/env python3
"""Bumper sticker generator using Cairo graphics.

Generates road-safety bumper sticker designs as SVG or PNG files.  Each
design is fully described by a :class:`StickerConfig`, so you can iterate on
colours, text, font sizes, and layout without touching the drawing code.

Two built-in designs are provided:

* ``"20-is-plenty"`` — 20 mph campaign sticker
* ``"speed-kills"``  — safety-awareness sticker with cool-S decoration

**Quick start**::

    python3 sticker_generator.py                          # renders both designs as PNG
    python3 sticker_generator.py --output svg             # both as SVG
    python3 sticker_generator.py --design speed-kills     # one design as PNG
    python3 sticker_generator.py --config my.json         # load custom JSON config

**Programmatic use**::

    from sticker_generator import CONFIGS, render_sticker

    cfg = CONFIGS["20-is-plenty"]
    cfg.blocks[0].font_size_pt = 100          # bigger "20"
    cfg.background_colour = "#ffcc00"         # yellow background
    render_sticker(cfg, "my-variant.png", output_format="png")

**Creating a new design from scratch**::

    from sticker_generator import StickerConfig, TextBlock, render_sticker

    cfg = StickerConfig(
        name="slow-down",
        background_colour="#cc0000",
        blocks=[
            TextBlock(
                text="SLOW DOWN",
                font_size_pt=72,
                bold=True,
                colour="#ffffff",
                x_frac=0.5,
                y_frac=0.5,
                anchor="centre",
            ),
        ],
    )
    render_sticker(cfg, "slow-down.png", output_format="png")
"""

from __future__ import annotations

import argparse
import json
import sys
from dataclasses import asdict, dataclass, field
from typing import Optional

try:
    import cairo

    HAVE_CAIRO = True
except ImportError:  # pragma: no cover
    HAVE_CAIRO = False
    cairo = None  # type: ignore[assignment]

# ---------------------------------------------------------------------------
# Unit helpers
# ---------------------------------------------------------------------------

_MM_PER_INCH: float = 25.4
_PT_PER_INCH: float = 72.0


def _mm_to_pt(mm: float) -> float:
    """Convert millimetres to typographic points (72 pt = 1 inch)."""
    return mm * _PT_PER_INCH / _MM_PER_INCH


def _mm_to_px(mm: float, dpi: int) -> float:
    """Convert millimetres to pixels at the given DPI."""
    return mm * dpi / _MM_PER_INCH


# ---------------------------------------------------------------------------
# Colour helpers
# ---------------------------------------------------------------------------


def _parse_colour(hex_str: str) -> tuple[float, float, float, float]:
    """Parse a CSS hex colour string to an (r, g, b, a) tuple in [0, 1].

    Accepts ``#RRGGBB`` and ``#RRGGBBAA`` formats.

    >>> _parse_colour("#ff0000")
    (1.0, 0.0, 0.0, 1.0)
    >>> _parse_colour("#ff000080")
    (1.0, 0.0, 0.0, 0.5019607843137255)
    """
    s = hex_str.lstrip("#")
    if len(s) == 6:
        r, g, b = (int(s[i : i + 2], 16) / 255.0 for i in (0, 2, 4))
        return r, g, b, 1.0
    if len(s) == 8:
        r, g, b, a = (int(s[i : i + 2], 16) / 255.0 for i in (0, 2, 4, 6))
        return r, g, b, a
    raise ValueError(f"Unrecognised colour string: {hex_str!r}")


# ---------------------------------------------------------------------------
# Configuration dataclasses
# ---------------------------------------------------------------------------


@dataclass
class TextBlock:
    """A single text element to render on the sticker.

    Position is specified as fractions of the sticker's width and height, so
    ``(0.5, 0.5)`` is the centre regardless of physical dimensions.

    Example — large "20" centred horizontally, upper half::

        TextBlock(
            text="20",
            font_size_pt=96,
            bold=True,
            colour="#1a1a2e",
            x_frac=0.5,
            y_frac=0.35,
            anchor="centre",
        )

    Attributes:
        text: The string to draw.
        font_size_pt: Font size in typographic points.
        bold: Whether to use a bold weight.
        italic: Whether to use italic style.
        colour: Fill colour as ``#RRGGBB`` or ``#RRGGBBAA``.
        x_frac: Horizontal anchor position as a fraction of sticker width (0–1).
        y_frac: Vertical centre of the text as a fraction of sticker height (0–1).
        anchor: Horizontal alignment — ``"left"``, ``"centre"``, or ``"right"``.
    """

    text: str
    font_size_pt: float = 36.0
    bold: bool = False
    italic: bool = False
    colour: str = "#000000"
    x_frac: float = 0.5
    y_frac: float = 0.5
    anchor: str = "centre"


@dataclass
class CoolSDecoration:
    """Parameters for the classic cool-S path decoration.

    The *cool S* (sometimes called the Super S or graffiti S) is a geometric
    doodle built from six horizontal lines connected to form a blocky S shape.
    It is rendered as a series of Cairo strokes.

    Example — medium cool S on the left margin::

        CoolSDecoration(
            x_frac=0.07,
            y_frac=0.5,
            size_frac=0.7,
            colour="#ffffff",
            stroke_width_frac=0.05,
        )

    Attributes:
        x_frac: Horizontal centre of the S as a fraction of sticker width (0–1).
        y_frac: Vertical centre of the S as a fraction of sticker height (0–1).
        size_frac: Height of the S as a fraction of sticker height.
        colour: Stroke colour as ``#RRGGBB`` or ``#RRGGBBAA``.
        stroke_width_frac: Stroke width as a fraction of the computed S height.
    """

    x_frac: float = 0.1
    y_frac: float = 0.5
    size_frac: float = 0.65
    colour: str = "#000000"
    stroke_width_frac: float = 0.048


@dataclass
class StickerConfig:
    """Complete configuration for one bumper-sticker design.

    Dimensions default to a standard UK bumper-sticker size (305 × 76 mm,
    approximately 12 × 3 inches).  All colours use CSS hex strings.

    Example — minimal custom design::

        cfg = StickerConfig(
            name="slow-down",
            background_colour="#ffdd00",
            blocks=[
                TextBlock(
                    text="SLOW DOWN",
                    font_size_pt=72,
                    bold=True,
                    colour="#cc0000",
                    x_frac=0.5,
                    y_frac=0.5,
                ),
            ],
        )

    Attributes:
        name: Human-readable identifier used as the default output filename stem.
        width_mm: Physical width in millimetres.
        height_mm: Physical height in millimetres.
        dpi: Pixel density for PNG output (has no effect on SVG).
        background_colour: Background fill colour.
        border_colour: Optional border colour (``None`` = no border).
        border_width_mm: Border stroke width in millimetres.
        blocks: Ordered list of text blocks to render.
        cool_s_decorations: Optional cool-S path decorations.
    """

    name: str = "sticker"
    width_mm: float = 305.0
    height_mm: float = 76.0
    dpi: int = 150
    background_colour: str = "#ffffff"
    border_colour: Optional[str] = None
    border_width_mm: float = 1.5
    blocks: list[TextBlock] = field(default_factory=list)
    cool_s_decorations: list[CoolSDecoration] = field(default_factory=list)

    @classmethod
    def from_dict(cls, data: dict) -> "StickerConfig":
        """Construct a :class:`StickerConfig` from a plain dictionary.

        Nested ``TextBlock`` and ``CoolSDecoration`` entries are converted
        automatically.  Unknown top-level keys are silently ignored, making
        partial override configs safe to use.
        """
        d = dict(data)
        d["blocks"] = [TextBlock(**b) for b in d.get("blocks", [])]
        d["cool_s_decorations"] = [
            CoolSDecoration(**s) for s in d.get("cool_s_decorations", [])
        ]
        known = {f.name for f in cls.__dataclass_fields__.values()}  # type: ignore[attr-defined]
        return cls(**{k: v for k, v in d.items() if k in known})

    @classmethod
    def from_json(cls, path: str) -> "StickerConfig":
        """Load a :class:`StickerConfig` from a JSON file."""
        with open(path, encoding="utf-8") as fh:
            return cls.from_dict(json.load(fh))

    def to_dict(self) -> dict:
        """Serialise this config to a plain dictionary."""
        return asdict(self)


# ---------------------------------------------------------------------------
# Low-level drawing helpers
# ---------------------------------------------------------------------------


def _set_source_colour(ctx: "cairo.Context", colour: str) -> None:  # type: ignore[name-defined]
    """Set the Cairo source from a hex colour string."""
    ctx.set_source_rgba(*_parse_colour(colour))


def draw_background(
    ctx: "cairo.Context",  # type: ignore[name-defined]
    width: float,
    height: float,
    colour: str,
) -> None:
    """Fill the canvas with a solid background colour.

    Args:
        ctx: Active Cairo drawing context.
        width: Canvas width in the context's current unit.
        height: Canvas height in the context's current unit.
        colour: Fill colour as a hex string.
    """
    _set_source_colour(ctx, colour)
    ctx.rectangle(0, 0, width, height)
    ctx.fill()


def draw_border(
    ctx: "cairo.Context",  # type: ignore[name-defined]
    width: float,
    height: float,
    colour: str,
    stroke_width: float,
) -> None:
    """Draw a rectangular border inset by half the stroke width.

    Args:
        ctx: Active Cairo drawing context.
        width: Canvas width in the context's current unit.
        height: Canvas height in the context's current unit.
        colour: Stroke colour as a hex string.
        stroke_width: Line width in the context's current unit.
    """
    _set_source_colour(ctx, colour)
    ctx.set_line_width(stroke_width)
    half = stroke_width / 2.0
    ctx.rectangle(half, half, width - stroke_width, height - stroke_width)
    ctx.stroke()


def draw_text_block(
    ctx: "cairo.Context",  # type: ignore[name-defined]
    block: TextBlock,
    canvas_width: float,
    canvas_height: float,
) -> None:
    """Render a :class:`TextBlock` onto the canvas.

    Font size is interpreted in the same units as *canvas_width* /
    *canvas_height* (points for SVG, pixels for PNG).

    Args:
        ctx: Active Cairo drawing context.
        block: Text block configuration.
        canvas_width: Total canvas width in the context's current unit.
        canvas_height: Total canvas height in the context's current unit.
    """
    weight = cairo.FONT_WEIGHT_BOLD if block.bold else cairo.FONT_WEIGHT_NORMAL
    slant = cairo.FONT_SLANT_ITALIC if block.italic else cairo.FONT_SLANT_NORMAL
    ctx.select_font_face("Sans", slant, weight)
    ctx.set_font_size(block.font_size_pt)

    _set_source_colour(ctx, block.colour)

    extents = ctx.text_extents(block.text)
    text_w = extents.width
    text_h = extents.height

    # Compute horizontal position from anchor
    x_centre = canvas_width * block.x_frac
    if block.anchor == "left":
        x = x_centre
    elif block.anchor == "right":
        x = x_centre - text_w
    else:  # centre
        x = x_centre - text_w / 2.0

    # y_frac positions the vertical centre of the text.  Cairo's show_text()
    # draws from the baseline, so we offset upward by half the glyph height
    # to achieve visual centring at the requested fraction.
    y = canvas_height * block.y_frac + text_h / 2.0

    ctx.move_to(x, y)
    ctx.show_text(block.text)


def draw_cool_s(
    ctx: "cairo.Context",  # type: ignore[name-defined]
    cx: float,
    cy: float,
    size: float,
    colour: str,
    stroke_width: Optional[float] = None,
) -> None:
    """Draw the classic cool-S (Super S / graffiti S) at the given position.

    The cool S is constructed from six short horizontal lines — three at the
    top of the shape and three at the bottom — connected to form a blocky S:

    * The **right side** of the top group is closed with a vertical stroke.
    * A **diagonal** crosses from the lower-right of the top group to the
      upper-left of the bottom group (the S crossbar, going upper-right →
      lower-left).
    * The **left side** of the bottom group is closed with a vertical stroke.

    This produces the characteristic two-C-shapes-connected-diagonally
    appearance.

    Example::

        draw_cool_s(ctx, cx=50, cy=38, size=60, colour="#ffffff")

    Args:
        ctx: Active Cairo drawing context.
        cx: Horizontal centre of the S in the context's current unit.
        cy: Vertical centre of the S in the context's current unit.
        size: Total height of the S in the context's current unit.
        colour: Stroke colour as a hex string.
        stroke_width: Override the default stroke width (defaults to 5 % of
            *size*).
    """
    h = size
    # Width ≈ 55 % of height matches the visual proportions of the hand-drawn
    # doodle: two groups of three short lines side-by-side look roughly square,
    # giving width ≈ 3/5 of height once stroke weight is accounted for.
    w = size * 0.55

    sw = stroke_width if stroke_width is not None else h * 0.05

    x0 = cx - w / 2.0  # left edge
    x1 = cx + w / 2.0  # right edge

    # Seven equally-spaced y-positions across the full height.
    # Indices 0–2: top group of three lines.
    # Index  3:    gap (no line drawn here).
    # Indices 4–6: bottom group of three lines.
    # Dividing by 7 gives equal spacing for 3 lines + 1 gap + 3 lines.
    step = h / 7.0
    yt = cy - h / 2.0
    y = [yt + step * i for i in range(7)]

    _set_source_colour(ctx, colour)
    ctx.set_line_width(sw)
    ctx.set_line_cap(cairo.LINE_CAP_BUTT)
    ctx.set_line_join(cairo.LINE_JOIN_MITER)

    def _stroke_line(ax: float, ay: float, bx: float, by: float) -> None:
        ctx.move_to(ax, ay)
        ctx.line_to(bx, by)
        ctx.stroke()

    # Six horizontal lines (indices 0–2 top group, 4–6 bottom group)
    for i in [0, 1, 2, 4, 5, 6]:
        _stroke_line(x0, y[i], x1, y[i])

    # Right-side vertical: closes the top group on the right
    _stroke_line(x1, y[0], x1, y[2])

    # S crossbar diagonal: upper-right → lower-left
    _stroke_line(x1, y[2], x0, y[4])

    # Left-side vertical: closes the bottom group on the left
    _stroke_line(x0, y[4], x0, y[6])


# ---------------------------------------------------------------------------
# Layout helpers
# ---------------------------------------------------------------------------


def proportional_y(
    canvas_height: float,
    frac: float,
    offset_pt: float = 0.0,
) -> float:
    """Return an absolute y-coordinate from a fractional position.

    Proportional placement lets you adjust vertical positions by editing
    a single fraction in the config rather than recalculating pixel offsets.

    Args:
        canvas_height: Total canvas height in the current unit.
        frac: Fractional position (0.0 = top, 1.0 = bottom).
        offset_pt: Optional offset in the current unit (positive = down).

    Returns:
        Absolute y-coordinate in the current unit.
    """
    return canvas_height * frac + offset_pt


def proportional_x(
    canvas_width: float,
    frac: float,
    offset_pt: float = 0.0,
) -> float:
    """Return an absolute x-coordinate from a fractional position.

    Args:
        canvas_width: Total canvas width in the current unit.
        frac: Fractional position (0.0 = left, 1.0 = right).
        offset_pt: Optional offset in the current unit (positive = right).

    Returns:
        Absolute x-coordinate in the current unit.
    """
    return canvas_width * frac + offset_pt


# ---------------------------------------------------------------------------
# Rendering orchestration
# ---------------------------------------------------------------------------


def _render_to_context(
    ctx: "cairo.Context",  # type: ignore[name-defined]
    cfg: StickerConfig,
    canvas_width: float,
    canvas_height: float,
    font_scale: float = 1.0,
) -> None:
    """Draw all elements of *cfg* onto an already-initialised Cairo context.

    This function is format-agnostic: it works for both SVG (point units) and
    PNG (pixel units).  The caller is responsible for surface creation and
    finalisation.

    Args:
        ctx: Active Cairo drawing context.
        cfg: Sticker configuration.
        canvas_width: Canvas width in the context's unit.
        canvas_height: Canvas height in the context's unit.
        font_scale: Multiplier applied to all font sizes (use to convert
            between point-based and pixel-based contexts).
    """
    # Background
    draw_background(ctx, canvas_width, canvas_height, cfg.background_colour)

    # Border (optional)
    if cfg.border_colour:
        border_w = _mm_to_pt(cfg.border_width_mm) * font_scale
        draw_border(ctx, canvas_width, canvas_height, cfg.border_colour, border_w)

    # Cool-S decorations
    for deco in cfg.cool_s_decorations:
        cx = proportional_x(canvas_width, deco.x_frac)
        cy = proportional_y(canvas_height, deco.y_frac)
        size = canvas_height * deco.size_frac
        sw = size * deco.stroke_width_frac
        draw_cool_s(ctx, cx, cy, size, deco.colour, stroke_width=sw)

    # Text blocks
    for block in cfg.blocks:
        # Scale font size from points to the canvas unit
        scaled_block = TextBlock(
            text=block.text,
            font_size_pt=block.font_size_pt * font_scale,
            bold=block.bold,
            italic=block.italic,
            colour=block.colour,
            x_frac=block.x_frac,
            y_frac=block.y_frac,
            anchor=block.anchor,
        )
        draw_text_block(ctx, scaled_block, canvas_width, canvas_height)


def render_sticker(
    cfg: StickerConfig,
    output_path: str,
    output_format: str = "png",
) -> None:
    """Render a sticker design to a file.

    Args:
        cfg: Sticker configuration.
        output_path: Destination file path.  The extension is updated to match
            *output_format* if it does not already match.
        output_format: ``"svg"`` or ``"png"``.  Case-insensitive.

    Raises:
        ImportError: If ``pycairo`` is not installed.
        ValueError: If *output_format* is not ``"svg"`` or ``"png"``.
    """
    if not HAVE_CAIRO:
        raise ImportError(
            "pycairo is required for sticker generation.\n"
            "Install it with: pip install pycairo\n"
            "(Cairo system libraries must also be present: "
            "see tools/sticker-generator/README.md)"
        )

    fmt = output_format.lower()
    if fmt not in {"svg", "png"}:
        raise ValueError(
            f"Unsupported output format {output_format!r}; use 'svg' or 'png'"
        )

    # Ensure the file extension matches the requested format
    stem = output_path
    for ext in (".svg", ".png"):
        if stem.lower().endswith(ext):
            stem = stem[: -len(ext)]
            break
    final_path = f"{stem}.{fmt}"

    if fmt == "svg":
        _render_svg(cfg, final_path)
    else:
        _render_png(cfg, final_path)

    print(f"[sticker-generator] wrote {final_path}")


def _render_svg(cfg: StickerConfig, path: str) -> None:
    """Render the sticker as an SVG file (vector, resolution-independent)."""
    width_pt = _mm_to_pt(cfg.width_mm)
    height_pt = _mm_to_pt(cfg.height_mm)

    surface = cairo.SVGSurface(path, width_pt, height_pt)
    ctx = cairo.Context(surface)

    # SVG dimensions are in points; font sizes in the config are also in
    # points, so font_scale = 1.
    _render_to_context(ctx, cfg, width_pt, height_pt, font_scale=1.0)

    surface.finish()


def _render_png(cfg: StickerConfig, path: str) -> None:
    """Render the sticker as a PNG bitmap at the configured DPI."""
    width_px = int(round(_mm_to_px(cfg.width_mm, cfg.dpi)))
    height_px = int(round(_mm_to_px(cfg.height_mm, cfg.dpi)))

    surface = cairo.ImageSurface(cairo.FORMAT_ARGB32, width_px, height_px)
    ctx = cairo.Context(surface)

    # PNG canvas is in pixels; config font sizes are in points.
    # Scale: px_per_pt = dpi / 72
    font_scale = cfg.dpi / _PT_PER_INCH

    _render_to_context(ctx, cfg, width_px, height_px, font_scale=font_scale)

    surface.write_to_png(path)


# ---------------------------------------------------------------------------
# Built-in design configurations
# ---------------------------------------------------------------------------

#: The "20 Is Plenty" bumper sticker.
#:
#: A road-safety sticker promoting 20 mph speed limits.  The large "20" sits
#: in the upper-centre; "IS PLENTY" is set in smaller caps below, aligned to
#: the right edge of the "20" for a clean typographic block.
#:
#: To change the speed limit text, modify ``blocks[0].text``.
#: To change the sub-text, modify ``blocks[1].text``.
TWENTY_IS_PLENTY: StickerConfig = StickerConfig(
    name="20-is-plenty",
    width_mm=305.0,
    height_mm=76.0,
    dpi=150,
    background_colour="#1a1a2e",  # deep navy
    border_colour="#e8c84a",  # amber border
    border_width_mm=2.0,
    blocks=[
        # Large speed limit numeral — the focal point of the design
        TextBlock(
            text="20",
            font_size_pt=62,
            bold=True,
            colour="#e8c84a",  # amber
            x_frac=0.38,
            y_frac=0.38,
            anchor="centre",
        ),
        # "IS PLENTY" — right-aligned to the numeral block
        #
        # Alignment rule: x_frac matches the numeral so "IS PLENTY" sits
        # directly beneath it as a typographic unit.  Adjust x_frac together
        # with the numeral's x_frac to reposition the whole text block.
        TextBlock(
            text="IS PLENTY",
            font_size_pt=22,
            bold=True,
            colour="#ffffff",
            x_frac=0.38,
            y_frac=0.72,
            anchor="centre",
        ),
        # Right-hand tagline
        TextBlock(
            text="slowdown.today",
            font_size_pt=13,
            bold=False,
            italic=True,
            colour="#aaaacc",
            x_frac=0.74,
            y_frac=0.55,
            anchor="centre",
        ),
    ],
    cool_s_decorations=[
        # A cool S on the left gives the sticker a street-art aesthetic
        CoolSDecoration(
            x_frac=0.10,
            y_frac=0.50,
            size_frac=0.72,
            colour="#e8c84a",
            stroke_width_frac=0.048,
        ),
    ],
)

#: The "Speed Kills" bumper sticker.
#:
#: A high-contrast safety-awareness sticker.  White text on red with a cool-S
#: decoration and a secondary tag line.
#:
#: To change the main message, modify ``blocks[0].text``.
SPEED_KILLS: StickerConfig = StickerConfig(
    name="speed-kills",
    width_mm=305.0,
    height_mm=76.0,
    dpi=150,
    background_colour="#cc0000",  # safety red
    border_colour="#ffffff",
    border_width_mm=2.5,
    blocks=[
        # Primary message
        TextBlock(
            text="SPEED KILLS",
            font_size_pt=58,
            bold=True,
            colour="#ffffff",
            x_frac=0.54,
            y_frac=0.42,
            anchor="centre",
        ),
        # Secondary tag line
        TextBlock(
            text="slow down · save lives",
            font_size_pt=16,
            bold=False,
            italic=True,
            colour="#ffcccc",
            x_frac=0.54,
            y_frac=0.74,
            anchor="centre",
        ),
    ],
    cool_s_decorations=[
        # Cool S on the left margin — a nod to road-safety street art
        CoolSDecoration(
            x_frac=0.08,
            y_frac=0.50,
            size_frac=0.72,
            colour="#ffffff",
            stroke_width_frac=0.048,
        ),
    ],
)

#: Registry mapping design names to their configs.
#:
#: Add new designs here to make them available via the ``--design`` CLI flag
#: and via ``CONFIGS["my-design"]`` in programmatic use.
CONFIGS: dict[str, StickerConfig] = {
    "20-is-plenty": TWENTY_IS_PLENTY,
    "speed-kills": SPEED_KILLS,
}


# ---------------------------------------------------------------------------
# CLI entry point
# ---------------------------------------------------------------------------


def _build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="sticker_generator",
        description=(
            "Generate bumper sticker designs using Cairo.\n\n"
            "Built-in designs: " + ", ".join(CONFIGS)
        ),
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=(
            "Examples:\n"
            "  python3 sticker_generator.py\n"
            "  python3 sticker_generator.py --output svg\n"
            "  python3 sticker_generator.py --design speed-kills --output png\n"
            "  python3 sticker_generator.py --config my-design.json\n"
        ),
    )
    parser.add_argument(
        "--design",
        choices=list(CONFIGS),
        default=None,
        help="Built-in design to render (default: render all designs).",
    )
    parser.add_argument(
        "--config",
        metavar="JSON_FILE",
        default=None,
        help="Path to a JSON config file.  Overrides --design.",
    )
    parser.add_argument(
        "--output",
        choices=["png", "svg"],
        default="png",
        help="Output format (default: png).",
    )
    parser.add_argument(
        "--outdir",
        metavar="DIR",
        default=".",
        help="Output directory (default: current directory).",
    )
    return parser


def main(argv: Optional[list[str]] = None) -> int:
    """CLI entry point.

    Returns:
        Exit code (0 = success).
    """
    parser = _build_parser()
    args = parser.parse_args(argv)

    if not HAVE_CAIRO:
        print(
            "ERROR: pycairo is not installed.\n"
            "Install it with: pip install pycairo\n"
            "(Cairo system libraries must also be present; "
            "see tools/sticker-generator/README.md)",
            file=sys.stderr,
        )
        return 1

    import os

    os.makedirs(args.outdir, exist_ok=True)

    if args.config:
        designs = [StickerConfig.from_json(args.config)]
    elif args.design:
        designs = [CONFIGS[args.design]]
    else:
        designs = list(CONFIGS.values())

    for cfg in designs:
        out_path = os.path.join(args.outdir, f"{cfg.name}.{args.output}")
        render_sticker(cfg, out_path, output_format=args.output)

    return 0


if __name__ == "__main__":
    sys.exit(main())
