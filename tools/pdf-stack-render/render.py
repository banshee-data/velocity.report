#!/usr/bin/env python3
"""Render the angled "stack of pages" PNG used on the velocity.report homepage.

Takes the first N pages of an input PDF, rasterises each, applies rounded
corners, rotates them by a small jitter, and composites them onto a
transparent PNG so the result mimics a casual pile of report pages — same
visual treatment as the CSS-rendered stack in the hero design, but as a
single PNG that can be served as `/img/stack.png` (or anywhere else).

Defaults match the hero design's stack (3 pages, slight CCW lean of the
front sheet, page 1 in front, with progressively dimmer underlays).

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
    from pdf2image import convert_from_path
except ImportError as e:  # pragma: no cover
    sys.stderr.write("error: pdf2image is required (pip install pdf2image)\n")
    sys.stderr.write("       it also needs Poppler installed on your system.\n")
    raise SystemExit(2) from e


# Pages in the stack, from back to front. Each entry is a small dict with:
#   rotate_deg: counter-clockwise rotation applied to the page
#   offset:     (dx, dy) translation in pixels at the page's native size
#   opacity:    0-1 alpha multiplier (back pages fade)
#   tint:       optional (r, g, b) blend to dim the back pages
# The list below mirrors the hero design's CSS transforms:
#   pg:nth-child(1) translate(18,18) rotate(3.5deg) opacity .85
#   pg:nth-child(2) translate(9, 9)  rotate(1.5deg) opacity .95
#   pg:nth-child(3) translate(0, 0)  rotate(0deg)   opacity 1.0
DEFAULT_STACK = [
    {"rotate_deg": 0.0, "offset": (0, 0), "opacity": 0.90, "tint": None},
    {"rotate_deg": 1.5, "offset": (140, 140), "opacity": 0.95, "tint": (240, 243, 248)},
    {"rotate_deg": 3.5, "offset": (280, 280), "opacity": 1.0, "tint": (215, 220, 228)},
]

# Final 3D-feel: rotate the whole stack so it leans like the design.
# Approximates `transform: rotateX(8deg) rotateY(-12deg) rotateZ(2deg)` via a
# simple Z rotation and a horizontal squeeze (Y-axis foreshortening).
DEFAULT_FINAL_ROTATE_DEG = 2.0
DEFAULT_FINAL_X_SCALE = 0.94  # squeeze horizontally to fake the rotateY


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
        help="number of pages in the stack (default: 3). Capped at len(input).",
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

    n_pages = max(1, args.pages)
    print(
        f"[1/4] rasterising {args.pdf.name} @ {args.dpi} dpi (first {n_pages} pages)..."
    )
    pages = convert_from_path(
        str(args.pdf), dpi=args.dpi, first_page=1, last_page=n_pages
    )
    if not pages:
        sys.stderr.write("error: pdf2image returned no pages\n")
        return 1

    print(f"[2/4] applying rounded corners (r={args.corner_radius})...")
    rounded = [round_corners(p, args.corner_radius) for p in pages]

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

    # Composite each page back-to-front
    for idx, (page_img, entry) in enumerate(zip(rounded, stack_entries)):
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

    args.output.parent.mkdir(parents=True, exist_ok=True)
    final.save(args.output, "PNG", optimize=True)
    print(f"wrote {args.output} ({final.width}x{final.height})")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
