#!/usr/bin/env python3
"""Flag wording that describes measured structure geometry as something else (D-38).

Geometry, scene and prior data is measured spatial data: observed or inferred
positions, extents and surfaces at a stated time. docs/geometry-priors-data-purpose.md
is canonical for the vocabulary; this check finds the phrases its Wording
section says to avoid, such as a "building model", a "digital twin", a
structure "reconstructed" or "extruded", or "construction plans".

It reads Markdown, site templates and JSON as prose, less link targets and
inline code spans, and code files only in their comments and string literals,
so a file name or an identifier such as `extrudeBox` is not copy. A match preceded on its line by a negation ("not a building model") is
the wording rule being stated, and passes. A line that must keep a phrase,
such as a quoted title, carries the marker `geometry-wording: ignore` in any
comment syntax.

Usage:
    python3 scripts/check-geometry-wording.py           # exit non-zero on findings
    python3 scripts/check-geometry-wording.py --report  # print findings, always exit 0
    python3 scripts/check-geometry-wording.py FILE...   # check only these paths
"""

from __future__ import annotations

import argparse
import os
import re
import subprocess
import sys
from pathlib import Path

IGNORE_MARKER = "geometry-wording: ignore"

# Nouns that name a structure or a whole scene. "site" is left out: "site
# model" is Gerardus's API term for a site's accepted scans (GetSiteModel,
# against_site_model), glossed where the docs use it.
STRUCTURE = (
    r"\b(?:building|buildings|structure|structures|fa[cç]ade|fa[cç]ades|wall|walls"
    r"|roof|roofs|house|houses|street|streets|city|scene|scenes|world)"
)

# (identifier, pattern, what to say instead)
PATTERNS: tuple[tuple[str, str, str], ...] = (
    ("digital-twin", r"\bdigital[ -]twins?\b", "measured geometry, or the scene"),
    ("as-built", r"\bas-built\b", "measured, at the acquisition time"),
    ("blueprint", r"\bblueprints?\b", "plan (for a plan of work) or measured geometry"),
    ("floor-plan", r"\bfloor[ -]?plans?\b", "footprint or extent"),
    ("photorealistic", r"\bphoto-?realistic\b", "measured surfaces"),
    ("survey-grade", r"\bsurvey[- ]grade\b", "the measured uncertainty"),
    (
        "structure-model",
        STRUCTURE
        + r"[- ](?:model|models|replica|replicas|reconstruction|reconstructions)\b"
        r"(?!-level)",
        "measured or scene geometry",
    ),
    (
        "reconstructed-structure",
        r"\breconstruct(?:ed|ion|ing|s)?\s+(?:the\s+|a\s+|its\s+|each\s+|their\s+)?"
        r"(?:building|buildings|structure|structures|fa[cç]ade|fa[cç]ades|wall|walls"
        r"|roof|roofs|house|houses)\b",
        "derived from returns, or fitted",
    ),
    (
        "reconstructed-structure",
        r"\b(?:building|buildings|structure|structures|fa[cç]ade|fa[cç]ades|wall|walls"
        r"|roof|roofs|house|houses|scene|scenes)\b(?:\s+(?:is|are|was|were|be|been|being))?"
        r"\s+reconstructed\b",
        "derived from returns, or fitted",
    ),
    (
        "lod2-building",
        r"\blod ?2 (?:building|roof)s?\b",
        "building parts with fitted roof planes",
    ),
    (
        "3d-building-model",
        r"\b3d (?:building|city) models?\b",
        "measured footprints and heights",
    ),
    (
        "construction-documents",
        r"\b(?:construction|architectural) (?:plans?|drawings?|documents?)\b",
        "measured spatial data",
    ),
    (
        "extruded-structure",
        r"\bextrud(?:e|ed|es|ing)\b(?=.{0,40}\b(?:building|part|footprint|wall|box|outline|roof)s?\b)"
        r"|\b(?:building|part|footprint|wall|box|outline|roof)s?\b.{0,40}\bextrud(?:e|ed|es|ing)\b",
        "raised to a measured height, an inferred extent",
    ),
)
COMPILED = tuple(
    (name, re.compile(rx, re.IGNORECASE), say) for name, rx, say in PATTERNS
)

# A negation shortly before a match on the same clause states the rule rather
# than breaking it: "not a building model", "rather than reconstructed".
NEGATION = re.compile(
    r"\b(?:not|no|never|nor|rather than|instead of|without|avoid|isn't|aren't)\b[^.;:]{0,60}$",
    re.IGNORECASE,
)

PROSE_GLOBS = ("*.md", "*.njk", "*.html", "*.json")
CODE_GLOBS = ("*.go", "*.swift", "*.ts", "*.js", "*.mjs", "*.py", "*.svelte", "*.proto")

# Paths outside the check, each for a stated reason.
EXCLUDED_PREFIXES: tuple[tuple[str, str], ...] = (
    ("docs/geometry-priors-data-purpose.md", "defines the words to avoid"),
    ("scripts/check-geometry-wording.py", "this check"),
    ("scripts/test_check_geometry_wording.py", "this check's tests"),
    ("docs/DEVLOG.md", "historical record; entries stay as written"),
    ("CHANGELOG.md", "historical release notes"),
    ("proto/gerardus/", "vendored from banshee-data/gerardus (gerardus#48)"),
    ("third_party/", "vendored"),
    (
        "docs/lidar/operations/brief/",
        "literature briefings quote their sources' wording",
    ),
    ("public_html/src/docs/reference-graph/", "citation titles"),
    ("public_html/src/scenes/", "generated scene assets"),
    ("data/maths/references.bib", "citation titles"),
)
EXCLUDED_SUFFIXES = (
    ".pb.go",
    ".pb.swift",
    ".grpc.swift",
    "_pb.ts",
    "_pb.js",
    "pnpm-lock.yaml",
)

# Comment and string-literal spans in a code line. A comment runs to the end of
# the line; a string is a quoted run with escapes allowed.
CODE_SPANS = re.compile(
    r"//.*$|#.*$|/\*.*?(?:\*/|$)|^\s*\*.*$|<!--.*?(?:-->|$)"
    r'|"(?:[^"\\]|\\.)*"|\'(?:[^\'\\]|\\.)*\'|`(?:[^`\\]|\\.)*`'
)


def excluded(rel: str) -> bool:
    """Return True when *rel* is outside the check."""
    return rel.endswith(EXCLUDED_SUFFIXES) or any(
        rel == prefix or rel.startswith(prefix) for prefix, _ in EXCLUDED_PREFIXES
    )


def is_code(rel: str) -> bool:
    return any(Path(rel).match(glob) for glob in CODE_GLOBS)


# In prose, a link's target and an inline code span name a file or an
# identifier, not the thing described; they are blanked, keeping offsets.
PROSE_NON_COPY = re.compile(r"\]\([^)]*\)|`[^`]*`")


def copy_spans(line: str, code: bool) -> list[tuple[int, str]]:
    """Return (offset, text) for the parts of *line* that are copy."""
    if not code:
        return [(0, PROSE_NON_COPY.sub(lambda m: " " * len(m.group(0)), line))]
    return [(m.start(), m.group(0)) for m in CODE_SPANS.finditer(line)]


def scan_text(text: str, code: bool = False) -> list[tuple[int, str, str, str]]:
    """Return (line number, rule, matched phrase, suggestion) for each finding."""
    findings: list[tuple[int, str, str, str]] = []
    for lineno, line in enumerate(text.splitlines(), start=1):
        if IGNORE_MARKER in line:
            continue
        for offset, span in copy_spans(line, code):
            for name, pattern, say in COMPILED:
                for match in pattern.finditer(span):
                    before = line[: offset + match.start()]
                    if NEGATION.search(before):
                        continue
                    findings.append((lineno, name, match.group(0), say))
    return findings


def tracked_files(repo_root: Path) -> list[str]:
    """List tracked, non-symlink files of the scanned types, relative to the root."""
    result = subprocess.run(
        ["git", "ls-files", "-s", "-z", "--", *PROSE_GLOBS, *CODE_GLOBS],
        cwd=repo_root,
        capture_output=True,
        check=True,
    )
    names = []
    for entry in result.stdout.decode("utf-8", errors="surrogateescape").split("\0"):
        if not entry:
            continue
        meta, name = entry.split("\t", 1)
        if meta.startswith("120000"):  # a symlink: its target is scanned once
            continue
        names.append(name)
    return names


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--report", action="store_true", help="print findings but always exit 0"
    )
    parser.add_argument(
        "paths", nargs="*", help="files to check; defaults to every tracked file"
    )
    args = parser.parse_args()

    repo_root = Path(__file__).resolve().parents[1]
    if args.paths:
        rels = [os.path.relpath(Path(p).resolve(), repo_root) for p in args.paths]
    else:
        rels = tracked_files(repo_root)

    total = 0
    for rel in rels:
        if excluded(rel):
            continue
        try:
            text = (repo_root / rel).read_text(encoding="utf-8", errors="replace")
        except OSError:
            continue
        for lineno, name, phrase, say in scan_text(text, code=is_code(rel)):
            print(f"  {rel}:{lineno}  [{name}] {phrase!r}: say {say}")
            total += 1

    if total:
        print()
        print(
            f"{total} finding(s). Geometry is measured spatial data: describe it as measured,"
        )
        print(
            "observed or inferred geometry (docs/geometry-priors-data-purpose.md, D-38)."
        )
        print(f"Keep a quoted title with a '{IGNORE_MARKER}' comment on its line.")
        return 0 if args.report else 1

    print("Geometry wording OK: no avoided phrases found.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
