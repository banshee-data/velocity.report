#!/usr/bin/env python3
"""Reflow Markdown prose to target width with orphan-line avoidance.

Handles prose paragraphs only — leaves code blocks, tables, headings,
lists, block quotes, frontmatter, and other structural elements untouched.

Orphan avoidance: if the last line of a paragraph would be very short
(≤ 25 chars), the paragraph is re-wrapped at a slightly narrower width
to redistribute text and eliminate the orphan.

Usage:
    reflow-prose.py [--width 100] [--check] FILE [FILE ...]
"""

from __future__ import annotations

import argparse
import os
import re
import sys
import textwrap
from pathlib import Path

# ── Defaults ─────────────────────────────────────────────────────────────

DEFAULT_WIDTH = 100
MIN_LAST_LINE = 40  # orphan threshold: last line shorter than this triggers fix
MAX_NARROW = 35  # maximum width reduction when fixing orphans
MIN_WIDTH = 60  # never narrow below this

# Basenames only — os.walk() yields directory basenames, not paths.
EXCLUDED_DIRS = {
    ".git",
    ".venv",
    "_site",
    "build",
    "dist",
    "DerivedData",
    "node_modules",
    "public_html",
    "web",
}

# Files excluded from reflow.
EXCLUDED_FILES: set[str] = set()

DEFAULT_SCAN_PATHS = [
    "docs",
    "data",
    "config",
    "cmd",
    "internal",
    "tools",
    "scripts",
    ".github",
    ".claude",
    "agents",
    "TENETS.md",
    "README.md",
    "ARCHITECTURE.md",
    "CHANGELOG.md",
    "CLAUDE.md",
    "CODE_OF_CONDUCT.md",
    "COMMANDS.md",
    "CONTRIBUTING.md",
    "MAGIC_NUMBERS.md",
    "TROUBLESHOOTING.md",
]

# ── Line classification ─────────────────────────────────────────────────

_FENCE_RE = re.compile(r"^(`{3,}|~{3,})")
_TABLE_RE = re.compile(r"^\s*\|")
_HEADING_RE = re.compile(r"^#{1,6}\s")
_UNORDERED_LIST_RE = re.compile(r"^(\s*)[-*+]\s")
_ORDERED_LIST_RE = re.compile(r"^(\s*)\d+[.)]\s")
_HR_RE = re.compile(r"^[-*_]{3,}\s*$")
_LINK_DEF_RE = re.compile(r"^\[.+\]:\s")
_IMAGE_RE = re.compile(r"^!\[")
_BOLD_META_RE = re.compile(r"^\*\*\w[^*]*:\*\*")
_HTML_OPEN_RE = re.compile(r"^<")
_HTML_COMMENT_OPEN = re.compile(r"<!--")
_HTML_COMMENT_CLOSE = re.compile(r"-->")

# Inline elements that should not be broken across lines.
# Protect spaces inside these by replacing with a placeholder before wrapping.
_INLINE_MATH_RE = re.compile(r"\$[^$\n]+?\$")
_INLINE_CODE_RE = re.compile(r"`[^`\n]+?`")

PROTECT_CHAR = "\x00"


def _is_structural(line: str) -> bool:
    """Return True if a line should not be part of a reflowed prose block.

    Only top-level prose lines (starting at column 0, no structural marker)
    are candidates for reflow.
    """
    stripped = line.rstrip()

    # Blank
    if not stripped:
        return True

    # Any leading whitespace → not a top-level prose line
    if line[0] in (" ", "\t"):
        return True

    # Heading
    if _HEADING_RE.match(stripped):
        return True

    # Fenced code marker (state machine also handles this)
    if _FENCE_RE.match(stripped):
        return True

    # Table row
    if _TABLE_RE.match(stripped):
        return True

    # Block quote
    if stripped.startswith(">"):
        return True

    # HTML tag or comment on its own line
    if _HTML_OPEN_RE.match(stripped):
        return True

    # Horizontal rule
    if _HR_RE.match(stripped):
        return True

    # List marker (unordered)
    if _UNORDERED_LIST_RE.match(stripped):
        return True

    # List marker (ordered)
    if _ORDERED_LIST_RE.match(stripped):
        return True

    # Link definition
    if _LINK_DEF_RE.match(stripped):
        return True

    # Image on own line
    if _IMAGE_RE.match(stripped):
        return True

    # Bold metadata line (e.g. **Key:** value)
    if _BOLD_META_RE.match(stripped):
        return True

    # Display math marker
    if stripped.startswith("$$"):
        return True

    return False


# ── Inline element protection ───────────────────────────────────────────


def _protect_inline(text: str) -> str:
    """Replace spaces inside inline math and code with a placeholder."""

    def _protect(m: re.Match) -> str:
        return m.group(0).replace(" ", PROTECT_CHAR)

    text = _INLINE_MATH_RE.sub(_protect, text)
    text = _INLINE_CODE_RE.sub(_protect, text)
    return text


def _unprotect_inline(text: str) -> str:
    return text.replace(PROTECT_CHAR, " ")


# ── Wrapping ─────────────────────────────────────────────────────────────


# Patterns that Prettier interprets as structural markers when they appear
# at the start of a continuation line.  Prettier will join such lines back
# to the previous line (for numbered markers) or insert a blank line before
# them (for list/heading/quote markers), breaking idempotency.
_DANGEROUS_START_RE = re.compile(r"^(\d+[.)]\s|[-*+]\s|#{1,6}\s|>\s)")


def _has_dangerous_starts(lines: list[str]) -> bool:
    """Return True if any continuation line starts with a Prettier-sensitive pattern
    or would be reclassified as structural by our own parser on a subsequent pass."""
    return any(_DANGEROUS_START_RE.match(ln) or _is_structural(ln) for ln in lines[1:])


def _wrap_no_orphan(
    text: str, width: int, original_lines: list[str] | None = None
) -> list[str]:
    """Wrap text to width, avoiding short orphan last lines.

    Also avoids creating line breaks that would place a Markdown structural
    marker (numbered list, bullet, heading, block quote) at the start of a
    continuation line, which Prettier would undo.

    If *original_lines* is provided and no acceptable wrapping can be found,
    falls back to the original lines unchanged (do-no-harm).
    """
    protected = _protect_inline(text)

    def _do_wrap(w: int) -> list[str]:
        return textwrap.wrap(
            protected,
            width=w,
            break_long_words=False,
            break_on_hyphens=False,
        )

    def _is_acceptable(lines: list[str]) -> bool:
        if len(lines) <= 1:
            return True
        if _has_dangerous_starts(lines):
            return False
        if len(lines[-1]) < MIN_LAST_LINE:
            return False
        return True

    lines = _do_wrap(width)

    if _is_acceptable(lines):
        return [_unprotect_inline(ln) for ln in lines]

    # Try progressively narrower widths to fix orphan or dangerous starts
    best = lines
    for w in range(width - 1, max(width - MAX_NARROW, MIN_WIDTH), -1):
        candidate = _do_wrap(w)
        if _is_acceptable(candidate):
            return [_unprotect_inline(ln) for ln in candidate]
        # Track best so far (longest last line without dangerous starts)
        if not _has_dangerous_starts(candidate) and (
            _has_dangerous_starts(best) or len(candidate[-1]) > len(best[-1])
        ):
            best = candidate

    # If every width produces dangerous starts and we have originals, preserve them
    if original_lines is not None and _has_dangerous_starts(best):
        return original_lines

    return [_unprotect_inline(ln) for ln in best]


# ── File processing ──────────────────────────────────────────────────────


def _reflow_file(path: Path, width: int) -> str | None:
    """Reflow prose in a file. Returns new content if changed, else None."""
    original = path.read_text(encoding="utf-8")
    lines = original.split("\n")

    output: list[str] = []
    i = 0
    n = len(lines)
    in_frontmatter = False
    in_fenced = False
    in_display_math = False
    in_html_comment = False

    # Detect YAML frontmatter at start of file
    if n > 0 and lines[0].rstrip() == "---":
        in_frontmatter = True
        output.append(lines[0])
        i = 1

    while i < n:
        line = lines[i]
        stripped = line.rstrip()

        # ── Frontmatter ──────────────────────────────────────────
        if in_frontmatter:
            output.append(line)
            if stripped == "---" or stripped == "...":
                in_frontmatter = False
            i += 1
            continue

        # ── Fenced code blocks ───────────────────────────────────
        if in_fenced:
            output.append(line)
            if _FENCE_RE.match(stripped):
                in_fenced = False
            i += 1
            continue

        if _FENCE_RE.match(stripped):
            in_fenced = True
            output.append(line)
            i += 1
            continue

        # ── Display math blocks ──────────────────────────────────
        if in_display_math:
            output.append(line)
            if stripped.startswith("$$"):
                in_display_math = False
            i += 1
            continue

        if stripped.startswith("$$"):
            in_display_math = True
            output.append(line)
            i += 1
            continue

        # ── Multi-line HTML comments ─────────────────────────────
        if in_html_comment:
            output.append(line)
            if _HTML_COMMENT_CLOSE.search(line):
                in_html_comment = False
            i += 1
            continue

        if _HTML_COMMENT_OPEN.search(line) and not _HTML_COMMENT_CLOSE.search(line):
            in_html_comment = True
            output.append(line)
            i += 1
            continue

        # ── Structural line — pass through ───────────────────────
        if _is_structural(line):
            output.append(line)
            i += 1
            continue

        # ── Collect consecutive prose lines ──────────────────────
        prose_lines: list[str] = []
        while i < n:
            ln = lines[i]
            st = ln.rstrip()

            # Stop at fenced code, display math, or structural line
            if _FENCE_RE.match(st) or st.startswith("$$"):
                break
            if _is_structural(ln):
                break
            # Stop at HTML comment start
            if _HTML_COMMENT_OPEN.search(ln) and not _HTML_COMMENT_CLOSE.search(ln):
                break

            prose_lines.append(st)
            i += 1

        if not prose_lines:
            continue

        # Join and reflow
        text = " ".join(prose_lines)
        # Collapse any multiple spaces from joining
        text = re.sub(r"  +", " ", text)
        reflowed = _wrap_no_orphan(text, width, original_lines=prose_lines)
        output.extend(reflowed)

    result = "\n".join(output)

    if result != original:
        return result
    return None


# ── File discovery ───────────────────────────────────────────────────────


def _resolve_excluded_files() -> set[Path]:
    repo_root = Path(__file__).resolve().parent.parent
    return {(repo_root / p).resolve() for p in EXCLUDED_FILES}


def _iter_markdown_paths(paths: list[str]) -> list[Path]:
    """Collect .md files from the given paths, skipping excluded dirs."""
    skip = _resolve_excluded_files()
    found: set[Path] = set()
    out: list[Path] = []

    for raw in paths:
        path = Path(raw)
        if not path.exists():
            continue
        if path.is_file():
            if (
                path.suffix == ".md"
                and path.resolve() not in skip
                and path not in found
            ):
                found.add(path)
                out.append(path)
        else:
            for root, dirs, files in os.walk(path):
                dirs[:] = [d for d in dirs if d not in EXCLUDED_DIRS]
                for fn in sorted(files):
                    if fn.endswith(".md"):
                        fp = Path(root) / fn
                        if fp.resolve() not in skip and fp not in found:
                            found.add(fp)
                            out.append(fp)
    return sorted(out)


# ── Main ─────────────────────────────────────────────────────────────────


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Reflow Markdown prose with orphan-line avoidance."
    )
    parser.add_argument(
        "paths",
        nargs="*",
        default=DEFAULT_SCAN_PATHS,
        help="Files or directories to process (default: project docs)",
    )
    parser.add_argument(
        "--width",
        type=int,
        default=DEFAULT_WIDTH,
        help=f"Target line width (default: {DEFAULT_WIDTH})",
    )
    parser.add_argument(
        "--check",
        action="store_true",
        help="Check only — do not modify files (exit 1 if changes needed)",
    )
    args = parser.parse_args()

    files = _iter_markdown_paths(args.paths)
    if not files:
        print("No Markdown files found.", file=sys.stderr)
        return 0

    changed = 0
    for f in files:
        try:
            result = _reflow_file(f, args.width)
        except Exception as exc:
            print(f"ERROR: {f}: {exc}", file=sys.stderr)
            continue

        if result is not None:
            changed += 1
            if args.check:
                print(f"  would reflow: {f}")
            else:
                f.write_text(result, encoding="utf-8")

    if args.check and changed:
        print(f"\n{changed} file(s) would be reflowed.", file=sys.stderr)
        return 1

    if not args.check:
        print(f"Reflowed {changed} file(s).")

    return 0


if __name__ == "__main__":
    sys.exit(main())
