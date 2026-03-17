#!/usr/bin/env python3
"""Check that prose lines in Markdown files stay within the column width limit.

Lines inside fenced code blocks, tables, lists, headings, image references,
link definitions, HTML comments, bold-key metadata lines, and horizontal rules
are excluded — only running prose is checked.

Usage:
    python3 scripts/check-prose-line-width.py            # check (exit 1 on violations)
    python3 scripts/check-prose-line-width.py --report    # report only (always exit 0)
    python3 scripts/check-prose-line-width.py --width 80  # override column limit
"""

from __future__ import annotations

import argparse
import os
import re
import sys
from pathlib import Path

# ── Defaults ─────────────────────────────────────────────────────────────

DEFAULT_WIDTH = 100

EXCLUDED_DIRS = {
    ".git",
    ".venv",
    "_site",
    "build",
    "dist",
    "node_modules",
}

DEFAULT_SCAN_PATHS = [
    "docs",
    "data",
    "public_html/src",
    "config",
    "cmd",
    "internal",
    "tools",
    "scripts",
    ".github/knowledge",
    ".github/TENETS.md",
    "README.md",
    "ARCHITECTURE.md",
    "CHANGELOG.md",
    "CODE_OF_CONDUCT.md",
    "CONTRIBUTING.md",
    "TROUBLESHOOTING.md",
]


# ── Helpers ──────────────────────────────────────────────────────────────


def _is_excluded_dir(name: str) -> bool:
    return name in EXCLUDED_DIRS


def iter_markdown_paths(paths: list[str]) -> list[Path]:
    """Collect .md files from the given paths, skipping excluded directories."""
    found: set[Path] = set()
    out: list[Path] = []

    for raw in paths:
        path = Path(raw)
        if not path.exists():
            continue
        if path.is_file():
            if path.suffix == ".md" and path not in found:
                found.add(path)
                out.append(path)
        else:
            for root, dirs, files in os.walk(path):
                dirs[:] = [d for d in dirs if not _is_excluded_dir(d)]
                for fn in sorted(files):
                    if fn.endswith(".md"):
                        fp = Path(root) / fn
                        if fp not in found:
                            found.add(fp)
                            out.append(fp)
    return sorted(out)


# ── Line classification ─────────────────────────────────────────────────

_TABLE_RE = re.compile(r"^\s*\|.*\|\s*$")
_UNORDERED_LIST_RE = re.compile(r"^(\s*)[-*+]\s")
_ORDERED_LIST_RE = re.compile(r"^(\s*)\d+\.\s")
_HEADING_RE = re.compile(r"^#{1,6}\s")
_LINK_DEF_RE = re.compile(r"^\[.+\]:\s")
_BOLD_META_RE = re.compile(r"^\*\*\w[^*]*:\*\*")
_HR_RE = re.compile(r"^[-*_]{3,}\s*$")
_IMAGE_RE = re.compile(r"^!\[")
_FENCE_RE = re.compile(r"^(`{3,}|~{3,})")
_HTML_COMMENT_OPEN = re.compile(r"<!--")
_HTML_COMMENT_CLOSE = re.compile(r"--!?>")


def _is_table_line(stripped: str) -> bool:
    return bool(_TABLE_RE.match(stripped))


def _is_list_line(stripped: str) -> bool:
    return bool(_UNORDERED_LIST_RE.match(stripped) or _ORDERED_LIST_RE.match(stripped))


def _is_list_continuation(line: str, in_list: bool) -> bool:
    """Indented continuation of a list item (4+ leading spaces or 1+ tab while
    we are inside a list context)."""
    if not in_list:
        return False
    if line.startswith("    ") or line.startswith("\t"):
        return True
    return False


def _is_heading(stripped: str) -> bool:
    return bool(_HEADING_RE.match(stripped))


def _is_link_definition(stripped: str) -> bool:
    return bool(_LINK_DEF_RE.match(stripped))


def _is_bold_metadata(stripped: str) -> bool:
    return bool(_BOLD_META_RE.match(stripped))


def _is_hr(stripped: str) -> bool:
    return bool(_HR_RE.match(stripped))


def _is_image_line(stripped: str) -> bool:
    return bool(_IMAGE_RE.match(stripped))


# ── Core checker ─────────────────────────────────────────────────────────


def check_file(filepath: Path, width: int) -> list[tuple[int, int, str]]:
    """Return list of (line_number, length, text) for prose lines over width."""
    violations: list[tuple[int, int, str]] = []

    try:
        text = filepath.read_text(encoding="utf-8", errors="replace")
    except OSError as exc:
        print(f"WARNING: cannot read {filepath}: {exc}", file=sys.stderr)
        return violations

    lines = text.splitlines()
    in_fence = False
    in_html_comment = False
    in_list = False

    for lineno, raw in enumerate(lines, 1):
        stripped = raw.strip()

        # ── fenced code blocks ───────────────────────────────────
        if _FENCE_RE.match(stripped):
            in_fence = not in_fence
            continue
        if in_fence:
            continue

        # ── HTML comments (multi-line) ───────────────────────────
        if not in_html_comment and _HTML_COMMENT_OPEN.search(raw):
            if not _HTML_COMMENT_CLOSE.search(raw[raw.index("<!--") + 4 :]):
                in_html_comment = True
            continue
        if in_html_comment:
            if _HTML_COMMENT_CLOSE.search(raw):
                in_html_comment = False
            continue

        # ── blank lines reset list context ───────────────────────
        if not stripped:
            in_list = False
            continue

        # ── tables ───────────────────────────────────────────────
        if _is_table_line(stripped):
            continue

        # ── lists (unordered and ordered, plus continuations) ────
        if _is_list_line(stripped):
            in_list = True
            continue
        if _is_list_continuation(raw, in_list):
            continue

        # Non-list, non-blank line resets list context
        in_list = False

        # ── headings ─────────────────────────────────────────────
        if _is_heading(stripped):
            continue

        # ── link definitions ─────────────────────────────────────
        if _is_link_definition(stripped):
            continue

        # ── bold metadata lines ──────────────────────────────────
        if _is_bold_metadata(stripped):
            continue

        # ── horizontal rules ─────────────────────────────────────
        if _is_hr(stripped):
            continue

        # ── image / badge lines ──────────────────────────────────
        if _is_image_line(stripped):
            continue

        # ── prose line: check width ──────────────────────────────
        length = len(raw)
        if length > width:
            violations.append((lineno, length, raw))

    return violations


# ── Main ─────────────────────────────────────────────────────────────────


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Check prose line width in Markdown files."
    )
    parser.add_argument(
        "--width",
        type=int,
        default=DEFAULT_WIDTH,
        help=f"Maximum prose line width (default: {DEFAULT_WIDTH})",
    )
    parser.add_argument(
        "--report",
        action="store_true",
        help="Report violations without failing (exit 0)",
    )
    parser.add_argument(
        "paths",
        nargs="*",
        default=DEFAULT_SCAN_PATHS,
        help="Files or directories to check",
    )
    args = parser.parse_args()

    files = iter_markdown_paths(args.paths)
    if not files:
        print("No Markdown files found.")
        return 0

    total_violations = 0
    files_with_issues: list[tuple[Path, list[tuple[int, int, str]]]] = []

    for fp in files:
        violations = check_file(fp, args.width)
        if violations:
            total_violations += len(violations)
            files_with_issues.append((fp, violations))

    if not files_with_issues:
        print(f"prose-width: all {len(files)} files within " f"{args.width} columns ✓")
        return 0

    # ── Report ───────────────────────────────────────────────────────
    for fp, violations in files_with_issues:
        for lineno, length, text in violations:
            print(f"{fp}:{lineno}: line is {length} columns " f"(limit {args.width})")

    clean = len(files) - len(files_with_issues)
    print(
        f"\nprose-width: {total_violations} lines over {args.width} columns "
        f"in {len(files_with_issues)} files "
        f"({clean}/{len(files)} files clean)"
    )

    if args.report:
        return 0
    return 1


if __name__ == "__main__":
    sys.exit(main())
