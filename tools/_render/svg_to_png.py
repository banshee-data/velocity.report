"""Shared SVG → PNG rasterisation for the render scripts.

Prefers `rsvg-convert` (fast, ships with librsvg) and falls back to the
`cairosvg` Python package. Callers supply exactly one of `scale` (multiplier)
or `width` (output pixel width).

Returns True on success, False if no converter is available on the system.
Raises ValueError if neither or both of scale/width are supplied.
"""

from __future__ import annotations

import shutil
import subprocess
from pathlib import Path


def svg_to_png(
    svg_path: Path | str,
    png_path: Path | str,
    *,
    scale: float | None = None,
    width: int | None = None,
) -> bool:
    if (scale is None) == (width is None):
        raise ValueError("provide exactly one of scale= or width=")

    svg_path = str(svg_path)
    png_path = str(png_path)

    rsvg = shutil.which("rsvg-convert")
    if rsvg:
        args = [rsvg]
        if scale is not None:
            args += ["-z", str(scale)]
        else:
            args += ["-w", str(width)]
        args += ["-o", png_path, svg_path]
        subprocess.run(args, check=True)
        return True

    try:
        import cairosvg
    except ImportError:
        return False

    kwargs: dict = {"url": svg_path, "write_to": png_path}
    if scale is not None:
        kwargs["scale"] = scale
    else:
        kwargs["output_width"] = width
    cairosvg.svg2png(**kwargs)
    return True


MISSING_HINT = (
    "install rsvg-convert (brew install librsvg) "
    "or the cairosvg Python package (pip install cairosvg)"
)
