#!/usr/bin/env python3
"""Render the angled "stack of pages" PNG used on the velocity.report homepage.

Takes the first N pages of an input PDF, rasterises each, applies rounded
corners, rotates them by a small jitter, and composites them onto a
transparent PNG so the result mimics a casual pile of report pages — same
visual treatment as the CSS-rendered stack in the hero design, but as a
single PNG that can be served as `/img/stack.png` (or anywhere else).

Defaults match the hero design's stack (3 pages, slight CCW lean of the
front sheet, page 3 in front, with progressively dimmer underlays).

Usage:
    python tools/pdf-stack-render/render.py INPUT.pdf [-o OUTPUT.png] [...]

Dependencies:
    pip install pdf2image Pillow
    # pdf2image needs Poppler:
    #   macOS:        brew install poppler
    #   Debian/Ubuntu: apt-get install poppler-utils
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

try:
    from PIL import Image, ImageDraw, ImageFilter
except ImportError as e:  # pragma: no cover
    sys.stderr.write("error: Pillow is required (pip install Pillow)\n")
    raise SystemExit(2) from e

try:
    from pdf2image import convert_from_path, pdfinfo_from_path
except ImportError as e:  # pragma: no cover
    sys.stderr.write("error: pdf2image is required (pip install pdf2image)\n")
    sys.stderr.write("       it also needs Poppler installed on your system.\n")
    raise SystemExit(2) from e


# Pages in the stack, from back to front. Each entry is a small dict with:
#   rotate_deg: counter-clockwise rotation applied to the page
#   offset:     (dx, dy) translation in pixels at the page's native size
#   opacity:    0-1 alpha multiplier (back pages fade)
#   tint:       optional (r, g, b) blend to dim the back pages
# The CSS stack is widened here so the exported PNG reveals more of the
# underlying report pages, especially the left column of page 1.
DEFAULT_STACK = [
    {"rotate_deg": -6.0, "offset": (0, 0), "opacity": 1.0, "tint": (215, 220, 228)},
    {
        "rotate_deg": -3,
        "offset": (-700, 175),
        "opacity": 1.0,
        "tint": (240, 243, 248),
    },
    {"rotate_deg": 0.0, "offset": (-1400, 350), "opacity": 1.0, "tint": None},
]

# Final 3D-feel: rotate the whole stack so it leans like the design.
# Approximates `transform: rotateX(8deg) rotateY(-12deg) rotateZ(2deg)` via a
# simple Z rotation and a horizontal squeeze (Y-axis foreshortening).
DEFAULT_FINAL_ROTATE_DEG = 2.0
DEFAULT_FINAL_X_SCALE = 0.94  # squeeze horizontally to fake the rotateY
DEFAULT_OUTPUT_SCALE = 0.68
DEFAULT_PNG_COLORS = 160


def parse_args() -> argparse.Namespace:
    p = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    p.add_argument("pdf", type=Path, help="input PDF path")
    p.add_argument(
        "-o",
        "--output",
        type=Path,
        default=Path("stack.png"),
        help="output PNG path (default: stack.png)",
    )
    p.add_argument(
        "--pages",
        type=int,
        default=3,
        help="number of pages in the stack (default: 3). Capped at the PDF page count.",
    )
    p.add_argument(
        "--dpi",
        type=int,
        default=180,
        help="PDF rasterisation DPI (default: 180)",
    )
    p.add_argument(
        "--corner-radius",
        type=int,
        default=14,
        help="rounded-corner radius in px on the rasterised page (default: 14)",
    )
    p.add_argument(
        "--shadow-blur",
        type=int,
        default=24,
        help="drop-shadow blur radius (default: 24, set 0 to disable)",
    )
    p.add_argument(
        "--shadow-offset",
        type=int,
        nargs=2,
        default=(8, 14),
        metavar=("DX", "DY"),
        help="drop-shadow offset in px (default: 8 14)",
    )
    p.add_argument(
        "--final-rotate",
        type=float,
        default=DEFAULT_FINAL_ROTATE_DEG,
        help="final rotation of the assembled stack, degrees CCW",
    )
    p.add_argument(
        "--final-x-scale",
        type=float,
        default=DEFAULT_FINAL_X_SCALE,
        help="horizontal squeeze to fake rotateY (0.85-1.0, default 0.94)",
    )
    p.add_argument(
        "--margin",
        type=int,
        default=80,
        help="canvas margin in px around the assembled stack",
    )
    p.add_argument(
        "--border-width",
        type=int,
        default=2,
        help="subtle page border width in px (default: 2, set 0 to disable)",
    )
    p.add_argument(
        "--border-color",
        type=int,
        nargs=4,
        default=(140, 150, 165, 120),
        metavar=("R", "G", "B", "A"),
        help="page border RGBA color (default: 140 150 165 120)",
    )
    p.add_argument(
        "--output-scale",
        type=float,
        default=DEFAULT_OUTPUT_SCALE,
        help="final output downscale factor for smaller PNGs (default: 0.72)",
    )
    p.add_argument(
        "--png-colors",
        type=int,
        default=DEFAULT_PNG_COLORS,
        help="palette color count for PNG quantization (16-256, default: 192)",
    )
    return p.parse_args()


def round_corners(img: Image.Image, radius: int) -> Image.Image:
    """Apply rounded corners by masking the alpha channel."""
    if radius <= 0:
        return img
    img = img.convert("RGBA")
    w, h = img.size
    mask = Image.new("L", (w, h), 0)
    ImageDraw.Draw(mask).rounded_rectangle(
        (0, 0, w - 1, h - 1), radius=radius, fill=255
    )
    # Multiply existing alpha by mask so a transparent input stays transparent.
    a = img.split()[-1]
    new_alpha = Image.new("L", (w, h), 0)
    new_alpha.paste(a, (0, 0))
    new_alpha = Image.eval(new_alpha, lambda v: v)
    new_alpha = Image.composite(new_alpha, Image.new("L", (w, h), 0), mask)
    img.putalpha(new_alpha)
    return img


def add_page_border(
    img: Image.Image,
    radius: int,
    border_width: int,
    border_color: tuple[int, int, int, int],
) -> Image.Image:
    """Draw a subtle rounded border inside the page bounds."""
    if border_width <= 0:
        return img

    img = img.convert("RGBA")
    w, h = img.size
    inset = max(0, border_width // 2)
    r, g, b, a = border_color
    overlay = Image.new("RGBA", (w, h), (0, 0, 0, 0))
    draw = ImageDraw.Draw(overlay)
    draw.rounded_rectangle(
        (inset, inset, w - 1 - inset, h - 1 - inset),
        radius=max(0, radius - inset),
        outline=(r, g, b, a),
        width=border_width,
    )
    return Image.alpha_composite(img, overlay)


def apply_tint(
    img: Image.Image, tint: tuple[int, int, int] | None, opacity: float
) -> Image.Image:
    """Blend the page towards `tint` and scale its alpha by `opacity`."""
    img = img.convert("RGBA")
    if tint is not None:
        tint_layer = Image.new("RGB", img.size, tint)
        rgb = Image.blend(img.convert("RGB"), tint_layer, 0.35)
        img = Image.merge("RGBA", (*rgb.split(), img.split()[-1]))
    if opacity < 1.0:
        a = img.split()[-1].point(lambda v: int(v * opacity))
        img.putalpha(a)
    return img


def drop_shadow(img: Image.Image, blur: int, offset: tuple[int, int]) -> Image.Image:
    """Composite a soft shadow behind `img` (which must be RGBA).
    Returns a new RGBA image, larger to make room for the shadow."""
    if blur <= 0:
        return img
    dx, dy = offset
    w, h = img.size
    pad = blur * 3
    canvas = Image.new("RGBA", (w + pad * 2, h + pad * 2), (0, 0, 0, 0))
    # shadow: take alpha, blur it, paint as black with alpha
    alpha = img.split()[-1]
    shadow = Image.new("RGBA", canvas.size, (0, 0, 0, 0))
    shadow_alpha = Image.new("L", canvas.size, 0)
    shadow_alpha.paste(alpha, (pad + dx, pad + dy))
    shadow_alpha = shadow_alpha.filter(ImageFilter.GaussianBlur(blur))
    shadow.putalpha(shadow_alpha)
    # darken the shadow tint a touch
    shadow_rgb = Image.new("RGB", canvas.size, (0, 0, 0))
    shadow = Image.merge("RGBA", (*shadow_rgb.split(), shadow.split()[-1]))
    canvas.alpha_composite(shadow)
    canvas.alpha_composite(img, (pad, pad))
    return canvas


def main() -> int:
    args = parse_args()
    if not args.pdf.exists():
        sys.stderr.write(f"error: {args.pdf} not found\n")
        return 1

    pdf_info = pdfinfo_from_path(str(args.pdf))
    page_count = int(pdf_info.get("Pages", 0))
    if page_count < 1:
        sys.stderr.write(f"error: could not determine page count for {args.pdf}\n")
        return 1

    n_pages = min(max(1, args.pages), page_count)
    print(
        f"[1/4] rasterising {args.pdf.name} @ {args.dpi} dpi (first {n_pages} of {page_count} pages)..."
    )
    pages = convert_from_path(
        str(args.pdf), dpi=args.dpi, first_page=1, last_page=n_pages
    )
    if not pages:
        sys.stderr.write("error: pdf2image returned no pages\n")
        return 1

    print(f"[2/4] applying rounded corners (r={args.corner_radius}) and page border...")
    border_color = tuple(max(0, min(255, c)) for c in args.border_color)
    rounded = []
    for p in pages:
        page = round_corners(p, args.corner_radius)
        page = add_page_border(
            page,
            radius=args.corner_radius,
            border_width=max(0, args.border_width),
            border_color=border_color,
        )
        rounded.append(page)

    # Map stack entries to actual pages. If the PDF has fewer pages than the
    # stack length, repeat the last page underneath.
    stack_entries = (
        DEFAULT_STACK[-n_pages:] if n_pages <= len(DEFAULT_STACK) else DEFAULT_STACK
    )
    while len(stack_entries) < len(rounded):
        stack_entries.insert(0, stack_entries[0])

    # Page size after rasterisation
    page_w, page_h = rounded[0].size
    print(f"[3/4] compositing stack (page size {page_w}x{page_h})...")

    # Compute canvas size to hold all rotated/offset pages comfortably.
    # Largest displacement is the back page's offset + a worst-case rotation
    # bounding-box expansion of ~10%.
    max_offset = max(
        max(abs(e["offset"][0]), abs(e["offset"][1])) for e in stack_entries
    )
    bbox_pad = int(max(page_w, page_h) * 0.15) + max_offset
    canvas_w = page_w + bbox_pad * 2
    canvas_h = page_h + bbox_pad * 2
    canvas = Image.new("RGBA", (canvas_w, canvas_h), (0, 0, 0, 0))

    # Composite pages back-to-front so page 1 sits on the front sheet again.
    for page_img, entry in zip(reversed(rounded), stack_entries):
        # Skip rounded corners on already-rounded pages (we did them above).
        tinted = apply_tint(page_img, entry["tint"], entry["opacity"])
        rotated = tinted.rotate(
            entry["rotate_deg"], resample=Image.BICUBIC, expand=True
        )
        rw, rh = rotated.size
        cx = (canvas_w - rw) // 2 + entry["offset"][0]
        cy = (canvas_h - rh) // 2 + entry["offset"][1]
        canvas.alpha_composite(rotated, (cx, cy))

    print("[4/4] applying final lean + drop shadow...")
    # Whole-stack rotate (mimics CSS rotateZ)
    if args.final_rotate:
        canvas = canvas.rotate(args.final_rotate, resample=Image.BICUBIC, expand=True)
    # Horizontal squeeze (cheap rotateY proxy)
    if args.final_x_scale and args.final_x_scale != 1.0:
        new_w = max(1, int(canvas.width * args.final_x_scale))
        canvas = canvas.resize((new_w, canvas.height), resample=Image.LANCZOS)

    # Drop shadow (after final transforms so it follows the lean)
    final = drop_shadow(canvas, args.shadow_blur, tuple(args.shadow_offset))

    # Trim transparent margin and add a clean margin
    bbox = final.getbbox()
    if bbox:
        final = final.crop(bbox)
    margin = max(0, args.margin)
    if margin:
        padded = Image.new(
            "RGBA", (final.width + margin * 2, final.height + margin * 2), (0, 0, 0, 0)
        )
        padded.alpha_composite(final, (margin, margin))
        final = padded

    # Downscale before quantization to keep a good quality:size ratio.
    if args.output_scale and args.output_scale != 1.0:
        scale = max(0.1, min(1.0, args.output_scale))
        new_w = max(1, int(final.width * scale))
        new_h = max(1, int(final.height * scale))
        final = final.resize((new_w, new_h), resample=Image.LANCZOS)

    # Quantize to a palette while preserving transparency to reduce file size.
    colors = max(16, min(256, args.png_colors))
    alpha = final.getchannel("A")
    opaque = final.convert("RGB").quantize(colors=colors, method=Image.FASTOCTREE)
    final = opaque.convert("RGBA")
    final.putalpha(alpha)

    args.output.parent.mkdir(parents=True, exist_ok=True)
    final.save(args.output, "PNG", optimize=True, compress_level=9)
    print(f"wrote {args.output} ({final.width}x{final.height})")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
