#!/usr/bin/env python3
"""Check (and optionally fix) header metadata format in Markdown documentation.

Ensures header metadata lines immediately after the ``# Title`` heading use
the canonical bullet-list format::

    - **Key:** value

Non-conforming formats that are detected and fixed:

1. Bare bold:         ``**Key:** value``     → ``- **Key:** value``
2. Colon outside:     ``**Key**: value``     → ``- **Key:** value``
3. Blockquote bold:   ``> **Key:** value``   → ``- **Key:** value``
4. H2 status:         ``## Status: value``   → ``- **Status:** value``

Position-based detection: ALL bold items before the first ``##`` heading
(or first non-metadata line) are treated as header metadata.  No key
whitelist is used.

Usage:
    python3 scripts/check-doc-header-metadata.py          # dry-run (report only)
    python3 scripts/check-doc-header-metadata.py --fix    # apply fixes
"""

from __future__ import annotations

import argparse
import os
import re
import sys


# ── Patterns ────────────────────────────────────────────────────────────

#  Canonical form: ``- **Key:** value``
RE_BULLET = re.compile(r"^- \*\*(?P<key>[^*]+):\*\*\s*(?P<val>.*)")

#  Bare: ``**Key:** value``
RE_BARE_COLON_INSIDE = re.compile(r"^\*\*(?P<key>[^*:]+):\*\*\s*(?P<val>.*)")

#  Colon-outside: ``**Key**: value``
RE_COLON_OUTSIDE = re.compile(r"^\*\*(?P<key>[^*:]+)\*\*:\s*(?P<val>.*)")

#  Blockquote: ``> **Key:** value``  or  ``> **Key**: value``
RE_BLOCKQUOTE_INSIDE = re.compile(r"^>\s*\*\*(?P<key>[^*:]+):\*\*\s*(?P<val>.*)")
RE_BLOCKQUOTE_OUTSIDE = re.compile(r"^>\s*\*\*(?P<key>[^*:]+)\*\*:\s*(?P<val>.*)")

#  H2 heading: ``## Key: value``
RE_H2 = re.compile(r"^##\s+(?P<key>[^:]+):\s*(?P<val>.*)")


def _canonical(key: str, val: str) -> str:
    """Return the canonical ``- **Key:** value`` line (no trailing newline)."""
    val = val.strip()
    if val:
        return f"- **{key.strip()}:** {val}"
    return f"- **{key.strip()}:**"


# ── Scanning logic ──────────────────────────────────────────────────────

# Directories to scan by default.
DEFAULT_DIRS = ["docs", "config", "data"]

# Files/dirs to skip.
SKIP_DIRS = {".git", "node_modules", ".venv", "__pycache__", "htmlcov", "coverage"}


def _find_md_files(targets: list[str], root: str) -> list[str]:
    """Return sorted list of Markdown file paths."""
    md_files: list[str] = []
    for target in targets:
        path = target if os.path.isabs(target) else os.path.join(root, target)
        if os.path.isfile(path) and path.endswith(".md"):
            md_files.append(path)
        elif os.path.isdir(path):
            for dp, dirs, fnames in os.walk(path):
                dirs[:] = [d for d in dirs if d not in SKIP_DIRS]
                for f in fnames:
                    if f.endswith(".md"):
                        md_files.append(os.path.join(dp, f))
    return sorted(set(md_files))


def _try_match(line: str) -> tuple[str, str, str] | None:
    """Try to match *line* against a non-canonical metadata pattern.

    Returns ``(key, value, kind)`` on match, ``None`` otherwise.
    ``kind`` is one of: ``bare``, ``colon-outside``, ``blockquote``, ``h2``.
    """
    for pattern, kind in [
        (RE_H2, "h2"),
        (RE_BLOCKQUOTE_INSIDE, "blockquote"),
        (RE_BLOCKQUOTE_OUTSIDE, "blockquote"),
        (RE_COLON_OUTSIDE, "colon-outside"),
        (RE_BARE_COLON_INSIDE, "bare"),
    ]:
        m = pattern.match(line)
        if m:
            return m.group("key").strip(), m.group("val"), kind
    return None


def process_file(
    filepath: str, *, fix: bool = False
) -> list[tuple[int, str, str, str]]:
    """Scan a Markdown file for non-canonical header metadata.

    Returns list of ``(lineno, old_text, new_text, kind)`` changes.
    When *fix* is True the file is rewritten in-place.
    """
    with open(filepath, encoding="utf-8") as f:
        lines = f.readlines()

    # Find the first ``# Title`` heading.
    title_idx: int | None = None
    for i, line in enumerate(lines):
        stripped = line.lstrip()
        if stripped.startswith("# ") and not stripped.startswith("## "):
            title_idx = i
            break

    if title_idx is None:
        return []

    changes: list[tuple[int, str, str, str]] = []
    # Map of line-index → replacement text (None = delete line).
    replace_map: dict[int, str | None] = {}

    # Scan up to 30 lines after the title for the metadata block.
    i = title_idx + 1
    limit = min(title_idx + 30, len(lines))
    while i < limit:
        stripped = lines[i].rstrip("\n")

        # Skip blank lines within the metadata block.
        if not stripped.strip():
            i += 1
            continue

        # Blank blockquote separator (``>``) — delete.
        if stripped.strip() == ">":
            changes.append((i + 1, stripped, "", "blank-bq"))
            replace_map[i] = None
            i += 1
            continue

        # Already canonical?
        if RE_BULLET.match(stripped):
            i += 1
            continue

        # Try non-canonical patterns.
        result = _try_match(stripped)
        if result is None:
            # First non-metadata, non-blank line → end of header block.
            break

        key, val, kind = result

        # For blockquote metadata, absorb continuation lines.
        if kind == "blockquote":
            j = i + 1
            while j < limit:
                cont = lines[j].rstrip("\n")
                # Continuation: starts with ``>`` but is NOT a new metadata key.
                if cont.startswith(">") and not _try_match(cont) and cont.strip() != ">":
                    cont_text = re.sub(r"^>\s?", "", cont)
                    if cont_text.strip():
                        val = val.rstrip() + " " + cont_text.strip()
                    replace_map[j] = None
                    j += 1
                else:
                    break
            next_i = j
        else:
            next_i = i + 1

        new_text = _canonical(key, val)
        changes.append((i + 1, stripped, new_text, kind))
        replace_map[i] = new_text + "\n"
        i = next_i

    if fix and changes:
        rebuilt: list[str] = []
        for idx, original in enumerate(lines):
            if idx in replace_map:
                repl = replace_map[idx]
                if repl is not None:
                    rebuilt.append(repl)
                # else: delete line
            else:
                rebuilt.append(original)
        with open(filepath, "w", encoding="utf-8") as f:
            f.writelines(rebuilt)

    return changes


# ── Entry-point ─────────────────────────────────────────────────────────


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Check/fix header metadata format in Markdown docs."
    )
    parser.add_argument(
        "--fix",
        action="store_true",
        help="Rewrite files in-place (default: dry-run report only).",
    )
    parser.add_argument(
        "paths",
        nargs="*",
        help="Files or directories to scan (default: docs/ config/ data/).",
    )
    args = parser.parse_args()

    root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    os.chdir(root)

    targets = args.paths or DEFAULT_DIRS
    md_files = _find_md_files(targets, root)

    total_changes = 0
    files_changed = 0

    for fpath in md_files:
        changes = process_file(fpath, fix=args.fix)
        if not changes:
            continue
        files_changed += 1
        total_changes += len(changes)
        rel = os.path.relpath(fpath, root)
        for lineno, old, new, kind in changes:
            action = "fixed" if args.fix else "found"
            print(f"  {rel}:{lineno}: {action} ({kind})")
            print(f"    - {old.strip()}")
            print(f"    + {new.strip()}")

    mode = "Fixed" if args.fix else "Found"
    print(f"\n{mode} {total_changes} issue(s) across {files_changed} file(s).")
    return 1 if total_changes > 0 and not args.fix else 0


if __name__ == "__main__":
    raise SystemExit(main())
