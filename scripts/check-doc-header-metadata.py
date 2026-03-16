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
5. Date metadata:     ``- **Created:** …``   → (removed)
6. Missing separator: no blank line after metadata → blank line inserted

Banned date keys (case-insensitive): ``Created``, ``Date``,
``Last Updated``, ``Original Design Date``.  Keys containing a
parenthesised date like ``Update (March 13, 2026)`` are rewritten
to strip the date: ``Update``.

Rationale: dates in documentation headers go stale immediately and
duplicate information already available in ``git log``.  See
``.github/knowledge/coding-standards.md`` § Documentation Metadata.

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

# ── Banned date keys ────────────────────────────────────────────────────

#  Keys whose entire line should be deleted (case-insensitive match).
BANNED_DATE_KEYS = {"created", "date", "last updated", "original design date"}

#  Detect a parenthesised date suffix in a key, e.g. ``Update (March 13, 2026)``
RE_KEY_DATE_SUFFIX = re.compile(
    r"^(?P<base>.+?)\s*\("
    r"(?:"
    r"(?:January|February|March|April|May|June|July|August|September|October|November|December)"
    r"\s+\d{1,2},?\s+\d{4}"
    r"|"
    r"\d{4}[-/]\d{2}[-/]\d{2}"
    r")"
    r"\)\s*$"
)


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


def _is_date_key(key: str) -> bool:
    """Return True if *key* is a banned date-only metadata key."""
    return key.strip().lower() in BANNED_DATE_KEYS


def _strip_key_date_suffix(key: str) -> str | None:
    """If *key* has a parenthesised date suffix, return the base key.

    Returns ``None`` if no date suffix is present.
    """
    m = RE_KEY_DATE_SUFFIX.match(key.strip())
    return m.group("base").strip() if m else None


def _ensure_blank_line_after_metadata(lines: list[str]) -> bool:
    """Insert a blank line after the metadata block if one is missing.

    Mutates *lines* in place.  Returns True if a line was inserted.
    """
    title_idx: int | None = None
    for i, line in enumerate(lines):
        s = line.lstrip()
        if s.startswith("# ") and not s.startswith("## "):
            title_idx = i
            break
    if title_idx is None:
        return False

    found_meta = False
    i = title_idx + 1
    limit = min(title_idx + 30, len(lines))
    while i < limit:
        stripped = lines[i].rstrip("\n")
        if not stripped.strip():
            i += 1
            continue
        if RE_BULLET.match(stripped):
            found_meta = True
            i += 1
            # Skip indented continuation lines.
            while i < limit and lines[i].startswith("  ") and lines[i].strip():
                i += 1
            continue
        break  # first body-text line

    if not found_meta:
        return False
    # i is the first body line.  If the preceding line is not blank, insert one.
    if i > 0 and i < len(lines) and lines[i].strip() and lines[i - 1].strip():
        lines.insert(i, "\n")
        return True
    return False


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
    # Index of the last metadata line that will be kept (not deleted).
    last_meta_idx: int | None = None

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
        bm = RE_BULLET.match(stripped)
        if bm:
            bkey = bm.group("key").strip()
            # Banned date key → delete the whole line.
            if _is_date_key(bkey):
                changes.append((i + 1, stripped, "", "date-key"))
                replace_map[i] = None
                i += 1
                # Also delete continuation lines of the deleted bullet.
                while i < limit and lines[i].startswith("  ") and lines[i].strip():
                    replace_map[i] = None
                    i += 1
                continue
            # Key with parenthesised date suffix → strip the date.
            base = _strip_key_date_suffix(bkey)
            if base:
                new_text = _canonical(base, bm.group("val"))
                changes.append((i + 1, stripped, new_text, "date-in-key"))
                replace_map[i] = new_text + "\n"
                last_meta_idx = i
                i += 1
                # Skip continuation lines.
                while i < limit and lines[i].startswith("  ") and lines[i].strip():
                    i += 1
                continue
            last_meta_idx = i
            i += 1
            # Skip continuation lines.
            while i < limit and lines[i].startswith("  ") and lines[i].strip():
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
                if (
                    cont.startswith(">")
                    and not _try_match(cont)
                    and cont.strip() != ">"
                ):
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

        # Check for banned date key (even on non-canonical input).
        if _is_date_key(key):
            changes.append((i + 1, stripped, "", "date-key"))
            replace_map[i] = None
            i = next_i
            continue

        # Strip parenthesised date suffix from key.
        base = _strip_key_date_suffix(key)
        if base:
            key = base

        new_text = _canonical(key, val)
        changes.append((i + 1, stripped, new_text, kind))
        replace_map[i] = new_text + "\n"
        last_meta_idx = i
        i = next_i

    # Detect missing blank line between metadata block and body text.
    if last_meta_idx is not None and i < len(lines) and lines[i].strip():
        has_blank = any(not lines[j].strip() for j in range(last_meta_idx + 1, i))
        if not has_blank:
            changes.append((i + 1, "(no blank line after metadata)", "", "missing-sep"))

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
        _ensure_blank_line_after_metadata(rebuilt)
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
