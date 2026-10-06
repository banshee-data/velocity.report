#!/usr/bin/env python3
"""Reject Unicode quarter-block characters (U+2596-U+259F).

Quarter-block glyphs render as blanks on the Raspberry Pi Linux console
because the default framebuffer font does not include them. Only full
blocks, half blocks, shade blocks, and standard box-drawing characters are
safe for console output.

This replaces a shell implementation that matched a literal bracket
expression with grep. That was locale-dependent: outside a UTF-8 locale grep
matches bracket expressions byte-wise rather than by character, so the class
degenerated into the set of bytes making up those glyphs. Every character in
U+2596-U+259F begins with the byte 0xE2, which is also the lead byte of the
em dash, the en dash and the curly quotes, so any line containing an em dash
matched. With no locale set the check reported roughly eleven thousand
findings, nearly all of them false, while passing in CI.

Comparing code points sidesteps locale entirely. It also lets the check
report which character it actually found, and means this file contains no
quarter-block characters of its own, so unlike the shell version it does not
have to exclude itself from the scan.

Usage:
    python3 scripts/check-quarter-blocks.py           # exit non-zero on findings
    python3 scripts/check-quarter-blocks.py --report  # print report, always exit 0
    python3 scripts/check-quarter-blocks.py FILE...   # check only these paths
"""

from __future__ import annotations

import argparse
import os
import subprocess
import sys
import unicodedata
from pathlib import Path

# The forbidden range, inclusive. Defined numerically rather than as literal
# glyphs so that this file is itself clean and can be scanned like any other.
FIRST_QUARTER_BLOCK = 0x2596
LAST_QUARTER_BLOCK = 0x259F

# File types worth scanning: anything that might reach a console or a reader.
# Kept in step with the shell implementation this replaces.
SCANNED_GLOBS = (
    "*.sh",
    "*.md",
    "*.txt",
    "*.py",
    "*.go",
    "*.swift",
    "*.svelte",
    "*.ts",
    "*.js",
)


def is_quarter_block(char: str) -> bool:
    """Return True when *char* is in U+2596-U+259F."""
    return FIRST_QUARTER_BLOCK <= ord(char) <= LAST_QUARTER_BLOCK


def describe(char: str) -> str:
    """Render a character as its code point and name, for the report."""
    try:
        name = unicodedata.name(char)
    except ValueError:  # pragma: no cover - every char in range is named
        name = "unnamed"
    return f"U+{ord(char):04X} {name}"


def scan_text(text: str) -> list[tuple[int, str]]:
    """Return (line number, character) for each quarter block in *text*.

    Line numbers are 1-based. A line containing several offenders yields one
    entry per distinct character, in order of first appearance, so a report
    names every glyph that needs replacing rather than only the first.
    """
    findings: list[tuple[int, str]] = []
    for lineno, line in enumerate(text.splitlines(), start=1):
        seen: set[str] = set()
        for char in line:
            if is_quarter_block(char) and char not in seen:
                seen.add(char)
                findings.append((lineno, char))
    return findings


def scan_file(path: Path) -> list[tuple[int, str]]:
    """Scan one file, tolerating undecodable bytes.

    Undecodable bytes become the replacement character, which is not in the
    forbidden range, so a file that is not valid UTF-8 yields no findings
    rather than crashing the lint.
    """
    try:
        text = path.read_text(encoding="utf-8", errors="replace")
    except OSError:
        return []
    return scan_text(text)


def tracked_files(repo_root: Path) -> list[Path]:
    """List the tracked files worth scanning, via git."""
    # Bytes, decoded explicitly as UTF-8, rather than text=True. Text mode
    # decodes using locale.getpreferredencoding(), which would reintroduce
    # exactly the environmental dependency this script exists to remove: under
    # LC_ALL=C that can be ASCII, and a non-ASCII path would then raise.
    result = subprocess.run(
        ["git", "ls-files", "-z", "--", *SCANNED_GLOBS],
        cwd=repo_root,
        capture_output=True,
        check=True,
    )
    names = result.stdout.decode("utf-8", errors="surrogateescape").split("\0")
    return [repo_root / name for name in names if name]


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--report",
        action="store_true",
        help="print findings but always exit 0",
    )
    parser.add_argument(
        "paths",
        nargs="*",
        help="specific files to check; defaults to every tracked file",
    )
    args = parser.parse_args()

    repo_root = Path(__file__).resolve().parents[1]

    if args.paths:
        files = [
            p if p.is_absolute() else repo_root / p
            for p in (Path(raw) for raw in args.paths)
        ]
    else:
        files = tracked_files(repo_root)

    total = 0
    for path in files:
        for lineno, char in scan_file(path):
            rel = os.path.relpath(path, repo_root)
            print(
                f"  {rel}:{lineno}  contains quarter-block character {describe(char)}"
            )
            total += 1

    if total > 0:
        print()
        print("Quarter-block characters (U+2596-U+259F) do not render on the")
        print("Raspberry Pi console font.  Use only full blocks, half blocks,")
        print("shade blocks, or box-drawing characters instead.")
        if args.report:
            return 0
        return 1

    print("All files OK: no quarter-block characters found.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
