#!/usr/bin/env python3
"""Render the LOC + coverage chart SVG embedded from the stats branch.

The output is deterministic: identical inputs produce a byte-identical SVG
so the diff on the stats branch is meaningful.

Usage:
    python3 scripts/loc-coverage-chart.py \
        --go-coverage coverage.out \
        --web-coverage web/coverage/lcov.info \
        --mac-coverage tools/visualiser-macos/coverage.info \
        --output dist/loc-coverage.svg

Any missing coverage file degrades that bucket to "no hatch" with a warning
on stderr; the LOC bar still renders.
"""

from __future__ import annotations

import argparse
import json
import shutil
import subprocess
import sys
from dataclasses import dataclass, field
from pathlib import Path
from typing import Iterable

# Bucket → (cloc language names that map into it)
CODED_BUCKETS = ("js", "go", "mac")
LANGUAGE_TO_BUCKET = {
    "TypeScript": "js",
    "JavaScript": "js",
    "Svelte": "js",
    "Go": "go",
    "Swift": "mac",
    "Markdown": "markdown",
    "Bourne Shell": "scripts",
    "Bourne Again Shell": "scripts",
    "Python": "scripts",
    "make": "scripts",
}

# Colours sampled from the attached mock.
COLOURS = {
    "js": "#ff4fb8",
    "go": "#7fdc2a",
    "mac": "#e7b27a",
    "markdown": "#5aa6e8",
    "scripts": "#b4b85a",
}
HATCH_COLOUR = "#d72638"
# In-bar labels sit on bright fills and stay dark in both colour schemes.
TEXT = "#111111"
# Off-bar labels and bar outlines sit on the page background, so they flip
# with the colour scheme via the .lbl / .edge classes in the embedded <style>.
TEXT_LIGHT = "#111111"
TEXT_DARK = "#e8e8e8"
EDGE_LIGHT = "#111111"
EDGE_DARK = "#d0d0d0"

EXCLUDE_DIRS = (
    "web/build",
    "web/node_modules",
    "web/.svelte-kit",
    "coverage",
    "web/coverage",
    "public_html/_site",
    "public_html/dist",
    "tools/visualiser-macos/build",
    "tools/visualiser-macos/DerivedData",
    "tools/visualiser-macos/.build",
    "tools/visualiser-macos/Packages",
    "data",
    "image/.pi-gen",
)


@dataclass
class BucketStats:
    code_loc: int = 0
    cov_found: int = 0
    cov_hit: int = 0
    languages: list[str] = field(default_factory=list)

    @property
    def covered_fraction(self) -> float:
        if self.cov_found <= 0:
            return 0.0
        return self.cov_hit / self.cov_found

    @property
    def has_coverage(self) -> bool:
        return self.cov_found > 0


def run_cloc(repo_root: Path) -> dict[str, int]:
    """Return {language: code_lines} via cloc, restricted to git-tracked files.

    We enumerate git-tracked files ourselves and pipe them to cloc via
    ``--list-file=-`` rather than using cloc's ``--vcs=git`` mode. That
    sidesteps cloc's restriction that ``--exclude-dir`` accepts only bare
    directory names (cloc 2.08 errors out on path-style entries like
    ``web/build``) and gives us deterministic, exact control over which
    tracked files are counted.
    """
    if shutil.which("cloc") is None:
        sys.exit(
            "cloc not found on PATH. Install with: brew install cloc / "
            "apt-get install cloc."
        )
    ls = subprocess.run(
        ["git", "ls-files"],
        cwd=repo_root,
        capture_output=True,
        check=True,
        text=True,
    )
    selected: list[str] = []
    for path in ls.stdout.splitlines():
        if not path:
            continue
        if any(path == p or path.startswith(p + "/") for p in EXCLUDE_DIRS):
            continue
        if path.endswith(".pb.go"):
            continue
        selected.append(path)
    if not selected:
        sys.exit("git ls-files returned no tracked files to count.")
    proc = subprocess.run(
        ["cloc", "--json", "--quiet", "--list-file=-"],
        cwd=repo_root,
        input="\n".join(selected),
        capture_output=True,
        check=True,
        text=True,
    )
    payload = json.loads(proc.stdout)
    return {
        lang: data["code"]
        for lang, data in payload.items()
        if lang not in {"header", "SUM"}
    }


def parse_go_coverage(path: Path) -> tuple[int, int]:
    """Return (lines_hit, lines_found) approximated from Go statement counts."""
    if not path.exists():
        print(
            f"warning: {path} not found; go bucket will render un-hatched.",
            file=sys.stderr,
        )
        return (0, 0)
    hit = found = 0
    with path.open() as fh:
        for raw in fh:
            line = raw.strip()
            if not line or line.startswith("mode:"):
                continue
            try:
                _loc, rest = line.split(":", 1)
                _range, num_stmts_s, count_s = rest.split()
                num_stmts = int(num_stmts_s)
                count = int(count_s)
            except (ValueError, IndexError):
                continue
            found += num_stmts
            if count > 0:
                hit += num_stmts
    return (hit, found)


def parse_lcov(path: Path, label: str) -> tuple[int, int]:
    """Return (lines_hit, lines_found) from a standard LCOV file."""
    if not path.exists():
        print(
            f"warning: {path} not found; {label} bucket will render un-hatched.",
            file=sys.stderr,
        )
        return (0, 0)
    hit = found = 0
    with path.open() as fh:
        for raw in fh:
            line = raw.strip()
            if line.startswith("LF:"):
                found += int(line[3:])
            elif line.startswith("LH:"):
                hit += int(line[3:])
    return (hit, found)


def build_buckets(
    cloc_counts: dict[str, int],
    go_cov: tuple[int, int],
    web_cov: tuple[int, int],
    mac_cov: tuple[int, int],
) -> tuple[dict[str, BucketStats], list[str]]:
    """Group cloc languages into buckets and attach coverage to coded ones."""
    buckets: dict[str, BucketStats] = {
        k: BucketStats() for k in ("js", "go", "mac", "markdown", "scripts")
    }
    other: list[str] = []
    for lang, code in cloc_counts.items():
        bucket = LANGUAGE_TO_BUCKET.get(lang)
        if bucket is None:
            other.append(f"{lang} ({code})")
            continue
        buckets[bucket].code_loc += code
        buckets[bucket].languages.append(lang)
    buckets["go"].cov_hit, buckets["go"].cov_found = go_cov
    buckets["js"].cov_hit, buckets["js"].cov_found = web_cov
    buckets["mac"].cov_hit, buckets["mac"].cov_found = mac_cov
    return buckets, other


# --- SVG rendering ----------------------------------------------------------


VIEWBOX_W = 540
VIEWBOX_H = 114
CHART_X = 8
CHART_W = 440
BAR_H = 26
BAR_GAP = 8
TOP_PAD = 15  # headroom for the labels that ride above the top bar
LABEL_FONT = "'Atkinson Hyperlegible', 'Helvetica Neue', Arial, sans-serif"


def fmt_pct(numerator: int, denominator: int) -> str:
    if denominator <= 0:
        return "0%"
    return f"{round(100 * numerator / denominator)}%"


def label_for(bucket: str, loc: int, total: int) -> str:
    return f"{bucket} ({fmt_pct(loc, total)})"


def svg_text(
    x: float,
    y: float,
    text: str,
    *,
    anchor: str = "start",
    weight: str = "700",
    size: int = 12,
    fill: str = TEXT,
    cls: str | None = None,
) -> str:
    # cls (e.g. "lbl") drives the fill from the embedded <style> so the label
    # flips with the colour scheme; a bare fill is used for in-bar labels that
    # must stay dark on their bright fill in both modes.
    paint = f' class="{cls}"' if cls else f' fill="{fill}"'
    return (
        f'<text x="{x:.1f}" y="{y:.1f}" '
        f'font-family="{LABEL_FONT}" font-size="{size}" font-weight="{weight}"'
        f'{paint} text-anchor="{anchor}">{text}</text>'
    )


def emit_segment(
    x: float,
    y: float,
    w: float,
    h: float,
    fill: str,
    hatch_id: str | None,
    covered_fraction: float,
) -> str:
    """Solid bar in the bucket colour; red diagonal hatch over the uncovered
    fraction; a single colour-scheme-aware outline on top."""
    parts: list[str] = [
        f'<rect x="{x:.2f}" y="{y:.2f}" width="{w:.2f}" height="{h:.2f}" '
        f'fill="{fill}" stroke="none"/>'
    ]
    if hatch_id is not None and covered_fraction < 1.0:
        covered_w = w * covered_fraction
        uncovered_w = w - covered_w
        if uncovered_w > 0:
            # Transparent-background pattern, so the hatch reads as red lines
            # over the bucket colour in either colour scheme.
            parts.append(
                f'<rect x="{x + covered_w:.2f}" y="{y:.2f}" '
                f'width="{uncovered_w:.2f}" height="{h:.2f}" '
                f'fill="url(#{hatch_id})" stroke="none"/>'
            )
    parts.append(
        f'<rect x="{x:.2f}" y="{y:.2f}" width="{w:.2f}" height="{h:.2f}" '
        f'fill="none" class="edge" stroke-width="2"/>'
    )
    return "".join(parts)


def render(buckets: dict[str, BucketStats]) -> str:
    total_loc = sum(b.code_loc for b in buckets.values())
    if total_loc == 0:
        sys.exit("No LOC counted; refusing to render an empty chart.")

    coded_loc = sum(buckets[k].code_loc for k in CODED_BUCKETS)

    def to_width(loc: int) -> float:
        return CHART_W * (loc / total_loc) if total_loc else 0.0

    # coded_loc is retained for callers/tests that reason about the split;
    # it is no longer rendered as a footer.
    _ = coded_loc

    in_y = BAR_H / 2 + 4  # baseline offset to vertically centre an in-bar label

    parts: list[str] = []
    parts.append(
        f'<svg xmlns="http://www.w3.org/2000/svg" '
        f'viewBox="0 0 {VIEWBOX_W} {VIEWBOX_H}" '
        f'width="{VIEWBOX_W}" height="{VIEWBOX_H}" '
        f'role="img" aria-label="Lines of code by language with test '
        f'coverage shown as hatched uncovered regions">'
    )
    parts.append(
        "<defs>"
        "<style>"
        f".lbl{{fill:{TEXT_LIGHT}}}.edge{{stroke:{EDGE_LIGHT}}}"
        "@media (prefers-color-scheme:dark){"
        f".lbl{{fill:{TEXT_DARK}}}.edge{{stroke:{EDGE_DARK}}}}}"
        "</style>"
        # Diagonal red hatch on a transparent background, painted over the
        # bucket colour to mark the uncovered fraction.
        '<pattern id="hatch" patternUnits="userSpaceOnUse" '
        'width="7" height="7" patternTransform="rotate(45)">'
        f'<line x1="0" y1="0" x2="0" y2="7" stroke="{HATCH_COLOUR}" '
        'stroke-width="2.5"/>'
        "</pattern>"
        "</defs>"
    )

    # Top bar: js | go | mac, stacked; labels sit inside wide segments,
    # otherwise above the bar (where they ride the page background).
    cursor_x = CHART_X
    top_y = TOP_PAD
    for bucket in CODED_BUCKETS:
        b = buckets[bucket]
        if b.code_loc == 0:
            continue
        seg_w = to_width(b.code_loc)
        parts.append(
            emit_segment(
                cursor_x,
                top_y,
                seg_w,
                BAR_H,
                COLOURS[bucket],
                "hatch" if b.has_coverage else None,
                b.covered_fraction if b.has_coverage else 1.0,
            )
        )
        text = label_for(bucket, b.code_loc, total_loc)
        if seg_w >= 56:
            parts.append(
                svg_text(cursor_x + seg_w / 2, top_y + in_y, text, anchor="middle")
            )
        else:
            parts.append(
                svg_text(
                    cursor_x + seg_w / 2,
                    top_y - 4,
                    text,
                    anchor="middle",
                    size=11,
                    cls="lbl",
                )
            )
        cursor_x += seg_w

    # Middle and bottom single bars: markdown, then scripts.
    for bucket, inside_min, row in (
        ("markdown", 110, top_y + BAR_H + BAR_GAP),
        ("scripts", 90, top_y + 2 * (BAR_H + BAR_GAP)),
    ):
        b = buckets[bucket]
        seg_w = to_width(b.code_loc)
        parts.append(
            emit_segment(CHART_X, row, seg_w, BAR_H, COLOURS[bucket], None, 1.0)
        )
        text = label_for(bucket, b.code_loc, total_loc)
        if seg_w >= inside_min:
            parts.append(
                svg_text(CHART_X + seg_w / 2, row + in_y, text, anchor="middle")
            )
        else:
            # Beside the bar, on the page background → colour-scheme aware.
            parts.append(
                svg_text(
                    CHART_X + seg_w + 8,
                    row + in_y,
                    text,
                    anchor="start",
                    size=11,
                    cls="lbl",
                )
            )

    parts.append("</svg>")
    return "".join(parts) + "\n"


def main(argv: Iterable[str] | None = None) -> int:
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--go-coverage", type=Path, default=Path("coverage.out"))
    p.add_argument("--web-coverage", type=Path, default=Path("web/coverage/lcov.info"))
    p.add_argument(
        "--mac-coverage",
        type=Path,
        default=Path("tools/visualiser-macos/coverage.info"),
    )
    p.add_argument("--output", type=Path, required=True)
    p.add_argument("--repo-root", type=Path, default=Path("."))
    args = p.parse_args(list(argv) if argv is not None else None)

    cloc_counts = run_cloc(args.repo_root)
    go_cov = parse_go_coverage(args.go_coverage)
    web_cov = parse_lcov(args.web_coverage, "js")
    mac_cov = parse_lcov(args.mac_coverage, "mac")

    buckets, other = build_buckets(cloc_counts, go_cov, web_cov, mac_cov)
    if other:
        print(
            "info: cloc reported unbucketed languages (excluded from chart): "
            + ", ".join(sorted(other)),
            file=sys.stderr,
        )

    svg = render(buckets)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(svg)
    print(f"Wrote {args.output} ({len(svg):,} bytes)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
