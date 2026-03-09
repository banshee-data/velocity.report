#!/usr/bin/env python3
"""Check (and optionally fix) American English spellings in docs/ Markdown files.

Replaces American English words with British English equivalents in prose,
while preserving code identifiers, fenced/indented code blocks, backtick
spans, CLI flags, file paths, and CSS properties.

Usage:
    python3 scripts/check-british-spelling.py          # dry-run (report only)
    python3 scripts/check-british-spelling.py --fix    # apply replacements
"""

from __future__ import annotations

import argparse
import os
import re
import sys

# ── Word-level replacements (case-sensitive) ────────────────────────────

REPLACEMENTS: dict[str, str] = {
    # -ize → -ise  (and derived forms)
    "serialize": "serialise",
    "serialized": "serialised",
    "serialization": "serialisation",
    "Serialization": "Serialisation",
    "serializer": "serialiser",
    "serializers": "serialisers",
    "initialize": "initialise",
    "initialized": "initialised",
    "initializing": "initialising",
    "initialization": "initialisation",
    "Initialization": "Initialisation",
    "visualize": "visualise",
    "visualized": "visualised",
    "visualizer": "visualiser",
    "Visualizer": "Visualiser",
    "visualizers": "visualisers",
    "visualizing": "visualising",
    "visualization": "visualisation",
    "Visualization": "Visualisation",
    "standardize": "standardise",
    "standardized": "standardised",
    "standardization": "standardisation",
    "Standardization": "Standardisation",
    "optimize": "optimise",
    "optimized": "optimised",
    "optimizing": "optimising",
    "optimizer": "optimiser",
    "Optimizer": "Optimiser",
    "optimization": "optimisation",
    "Optimization": "Optimisation",
    "Optimized": "Optimised",
    "specialize": "specialise",
    "specialized": "specialised",
    "Specialized": "Specialised",
    "centralize": "centralise",
    "centralized": "centralised",
    "materialize": "materialise",
    "materialized": "materialised",
    "summarize": "summarise",
    "summarized": "summarised",
    "prioritize": "prioritise",
    "prioritized": "prioritised",
    "prioritizes": "prioritises",
    "Prioritize": "Prioritise",
    "categorize": "categorise",
    "categorization": "categorisation",
    "minimize": "minimise",
    "minimizing": "minimising",
    "normalize": "normalise",
    "normalized": "normalised",
    "normalization": "normalisation",
    "localize": "localise",
    "localization": "localisation",
    "organize": "organise",
    "organized": "organised",
    "organizes": "organises",
    "organization": "organisation",
    "analyze": "analyse",
    "Analyze": "Analyse",
    "analyzed": "analysed",
    "analyzing": "analysing",
    # -or → -our
    "behavior": "behaviour",
    "Behavior": "Behaviour",
    "favor": "favour",
    "color": "colour",
    "Color": "Colour",
    "colors": "colours",
    # -er → -re  (nouns only — not verbs like 'center' used as verb)
    # Only safe standalone nouns; skip metre/litre (too many false positives
    # with code variables).
    # -ling doubling
    "modeling": "modelling",
    "labeling": "labelling",
    "Labeling": "Labelling",
    "traveling": "travelling",
    # -neighbor → -neighbour
    "neighbor": "neighbour",
    "Neighbor": "Neighbour",
    "neighbors": "neighbours",
    "Neighbors": "Neighbours",
    # misc
    "defense": "defence",
}

# Build regex — longest keys first to prevent partial matches.
_sorted_keys = sorted(REPLACEMENTS, key=len, reverse=True)
WORD_RE = re.compile(r"\b(" + "|".join(re.escape(k) for k in _sorted_keys) + r")\b")

FENCE_RE = re.compile(r"^(\s*)(```|~~~)")


# ── Context guards ──────────────────────────────────────────────────────


def _in_backtick_span(line: str, start: int) -> bool:
    """True if *start* falls inside an inline backtick code span."""
    return line[:start].count("`") % 2 == 1


def _in_file_path(line: str, start: int, end: int) -> bool:
    """True if the match is part of a file-path or Markdown link target."""
    after = line[end : end + 60]
    before = line[max(0, start - 80) : start]

    # Followed by .md / .go / .py / .ts / .js  (filename)
    if re.search(r"^[-\w]*\.(md|go|py|ts|js)", after):
        return True
    # Inside a directory path
    if re.search(r"([-/\w]+[-/])$", before) and re.search(r"^[-\w]*[./]", after):
        return True
    # Inside a Markdown link target:  ](some-labeling-plan.md)
    paren = before.rfind("(")
    bracket = before.rfind("]")
    if paren > bracket and paren != -1:
        close = after.find(")")
        if close != -1 and " " not in after[:close]:
            return True
    return False


def _in_code_identifier(line: str, start: int, end: int) -> bool:
    """True if the match is embedded in a code identifier, CLI flag, or CSS prop."""
    # camelCase: preceded or followed by an uppercase letter
    if start > 0 and line[start - 1].isalpha() and line[start - 1].isupper():
        return True
    if end < len(line) and line[end].isalpha() and line[end].isupper():
        return True
    # snake_case
    if start > 0 and line[start - 1] == "_":
        return True
    if end < len(line) and line[end] == "_":
        return True
    # CLI flags:  --neighbor-start, --fixed-neighbor
    if re.search(r"--\w*$", line[max(0, start - 10) : start]):
        return True
    if start >= 2 and line[start - 2 : start] == "--":
        return True
    # key=value notation:  neighbors=3
    if end < len(line) and line[end] == "=":
        return True
    # CSS / colour-adjacent hyphens:  background-color, color:
    word = line[start:end].lower()
    if word in ("color", "colour"):
        if (start > 0 and line[start - 1] == "-") or (
            end < len(line) and line[end] in "-:"
        ):
            return True
    # REST endpoint context
    before30 = line[max(0, start - 30) : start]
    if any(tok in before30 for tok in ("/api/", "GET ", "POST ", "PUT ", "DELETE ")):
        after20 = line[end : end + 20]
        if re.match(r"[-/\w]", after20.lstrip()):
            return True
    return False


# ── File processing ─────────────────────────────────────────────────────


def _process_line(line: str) -> str:
    """Return *line* with American spellings replaced in prose positions."""
    result = line
    offset = 0
    for m in WORD_RE.finditer(line):
        word = m.group(0)
        s, e = m.start(), m.end()
        if _in_backtick_span(line, s):
            continue
        if _in_file_path(line, s, e):
            continue
        if _in_code_identifier(line, s, e):
            continue
        repl = REPLACEMENTS[word]
        adj_s = s + offset
        adj_e = e + offset
        result = result[:adj_s] + repl + result[adj_e:]
        offset += len(repl) - len(word)
    return result


def process_file(filepath: str, *, fix: bool = False) -> list[tuple[int, str, str]]:
    """Scan a Markdown file.  Returns list of (lineno, old, new) changes.

    When *fix* is True the file is rewritten in-place.
    """
    with open(filepath, encoding="utf-8") as f:
        lines = f.readlines()

    changes: list[tuple[int, str, str]] = []
    new_lines: list[str] = []
    in_fence = False

    for lineno, line in enumerate(lines, 1):
        # Track fenced code blocks (``` or ~~~)
        if FENCE_RE.match(line):
            in_fence = not in_fence
            new_lines.append(line)
            continue

        if in_fence:
            new_lines.append(line)
            continue

        # Skip indented code blocks (4+ spaces / tab) that are NOT list items
        if (
            re.match(r"^(    |\t)\S", line)
            and not re.match(r"^\s*[-*+>]\s", line)
            and not re.match(r"^\s*\d+\.\s", line)
        ):
            new_lines.append(line)
            continue

        new_line = _process_line(line)
        if new_line != line:
            changes.append((lineno, line.rstrip("\n"), new_line.rstrip("\n")))
        new_lines.append(new_line)

    if fix and changes:
        with open(filepath, "w", encoding="utf-8") as f:
            f.writelines(new_lines)

    return changes


# ── Entry-point ─────────────────────────────────────────────────────────


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Check/fix American English spellings in docs/ Markdown."
    )
    parser.add_argument(
        "--fix",
        action="store_true",
        help="Rewrite files in-place (default: dry-run report only).",
    )
    parser.add_argument(
        "paths",
        nargs="*",
        help="Files or directories to scan (default: docs/).",
    )
    args = parser.parse_args()

    root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    targets = args.paths or [os.path.join(root, "docs")]

    total_changes = 0
    files_changed = 0

    for target in targets:
        if os.path.isfile(target):
            md_files = [target] if target.endswith(".md") else []
        else:
            md_files = sorted(
                os.path.join(dp, f)
                for dp, _, fnames in os.walk(target)
                for f in fnames
                if f.endswith(".md")
            )

        for fpath in md_files:
            changes = process_file(fpath, fix=args.fix)
            if not changes:
                continue
            files_changed += 1
            total_changes += len(changes)
            rel = os.path.relpath(fpath, root)
            for lineno, old, new in changes:
                action = "fixed" if args.fix else "found"
                print(f"  {rel}:{lineno}: {action}")
                print(f"    - {old.strip()}")
                print(f"    + {new.strip()}")

    mode = "Fixed" if args.fix else "Found"
    print(f"\n{mode} {total_changes} replacement(s) across {files_changed} file(s).")
    return 1 if total_changes > 0 and not args.fix else 0


if __name__ == "__main__":
    raise SystemExit(main())
