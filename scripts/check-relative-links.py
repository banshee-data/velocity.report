#!/usr/bin/env python3
"""Check relative Markdown links resolve to existing files.

Scans all .md files under the repository root for relative links
(e.g. [text](../path/to/file.md)) and reports any that point to
non-existent targets.

Usage:
    python3 scripts/check-relative-links.py          # exit non-zero on dead links
    python3 scripts/check-relative-links.py --report  # print report, always exit 0
"""

from __future__ import annotations

import argparse
import os
import re
import sys
from pathlib import Path

# Pattern matches Markdown links: [text](target)
# Captures the target path (group 1).
# Excludes angle-bracket autolinks like <https://...> that may contain parens.
LINK_PATTERN = re.compile(r"\[(?:[^\]]*)\]\(([^)<>]+)\)")

# Directories to skip entirely.
SKIP_DIRS = {
    ".git",
    "node_modules",
    ".venv",
    "__pycache__",
    "vendor",
    ".build",
}


def find_markdown_files(root: Path) -> list[Path]:
    """Walk *root* and return all .md files, skipping SKIP_DIRS."""
    results: list[Path] = []
    for dirpath, dirnames, filenames in os.walk(root):
        dirnames[:] = [d for d in dirnames if d not in SKIP_DIRS]
        for fname in filenames:
            if fname.endswith(".md"):
                results.append(Path(dirpath) / fname)
    results.sort()
    return results


def check_file(filepath: Path, root: Path) -> list[tuple[int, str, str]]:
    """Return list of (line_number, link_target, resolved_path) for dead links."""
    dead: list[tuple[int, str, str]] = []
    try:
        text = filepath.read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError):
        return dead

    for lineno, line in enumerate(text.splitlines(), start=1):
        for match in LINK_PATTERN.finditer(line):
            target = match.group(1)

            # Skip external URLs, anchors-only, and mailto/data URIs.
            if target.startswith(("http://", "https://", "#", "mailto:", "data:")):
                continue

            # Strip anchor fragment for file-existence check.
            path_part = target.split("#")[0]
            if not path_part:
                continue

            # Resolve relative to the directory containing the source file.
            resolved = (filepath.parent / path_part).resolve()
            if not resolved.exists():
                rel_resolved = os.path.relpath(resolved, root)
                dead.append((lineno, target, rel_resolved))

    return dead


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Check relative Markdown links resolve to existing files."
    )
    parser.add_argument(
        "--report",
        action="store_true",
        help="Print report but always exit 0 (advisory mode).",
    )
    parser.add_argument(
        "paths",
        nargs="*",
        help="Files or directories to check (default: repo root).",
    )
    args = parser.parse_args()

    # Determine repository root (parent of scripts/).
    script_dir = Path(__file__).resolve().parent
    repo_root = script_dir.parent

    targets = args.paths if args.paths else [str(repo_root)]

    files: list[Path] = []
    for t in targets:
        p = Path(t)
        if p.is_file() and p.suffix == ".md":
            files.append(p)
        elif p.is_dir():
            files.extend(find_markdown_files(p))

    total_dead = 0
    for filepath in files:
        dead = check_file(filepath, repo_root)
        for lineno, target, resolved in dead:
            rel_file = os.path.relpath(filepath, repo_root)
            print(f"DEAD LINK: {rel_file}:{lineno}: {target} -> {resolved}")
            total_dead += 1

    if total_dead > 0:
        print(f"\n{total_dead} dead link(s) found.")
        if args.report:
            return 0
        return 1

    print("All relative links OK.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
