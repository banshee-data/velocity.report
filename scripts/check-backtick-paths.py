#!/usr/bin/env python3
"""Check backtick-quoted file paths in Markdown for stale or missing targets.

Scans all .md files under the repository root for inline code spans that look
like relative file paths (e.g. ``.github/TENETS.md``, ``internal/db/migrations/``)
and reports any whose targets cannot be resolved to existing files or directories.

Complements check-relative-links.py, which covers only Markdown link syntax
[text](path).  This script catches the prose-style path references that appear
in agent definitions, knowledge modules, and documentation.

What is checked
---------------
Only *repo-relative* paths are verified — tokens that start with ``.`` (e.g.
``.github/TENETS.md``, ``./scripts/``) or relative traversals (``../foo``).

Absolute paths (``/var/lib/...``, ``/etc/...``) and URL-like tokens are skipped
because they refer to target-system or runtime locations that do not exist in
the development checkout.

Resolution strategy
-------------------
Paths starting with ``.github/`` or ``.claude/`` are always resolved relative
to the repository root, because that is the conventional meaning of those
prefixes in this codebase.  All other relative paths are tried in two ways:

1. Relative to the repo root.
2. Relative to the directory containing the source file.

A path is considered valid if *either* resolution exists.  Glob patterns
(containing ``*`` or ``<``) and pure file-extension tokens (e.g. ``.venv/``,
``.img.xz``) are skipped — they are descriptive, not literal paths.

Usage
-----
    python3 scripts/check-backtick-paths.py            # exit 1 on failures
    python3 scripts/check-backtick-paths.py --report   # advisory, always exit 0
    python3 scripts/check-backtick-paths.py path/      # limit to subtree
"""

from __future__ import annotations

import argparse
import os
import re
import sys
from pathlib import Path

# Matches a backtick-quoted token that looks like a repo-relative file path.
# Only matches tokens that start with "." (relative) — absolute paths starting
# with "/" are excluded here because they refer to target-system locations.
# Groups: (1) the raw path token.
BACKTICK_PATH_RE = re.compile(
    r"`("
    # Explicitly repo-root-relative prefixes (.github/, .claude/)
    r"(?:\.github|\.claude)/[^`\s]+"
    r"|"
    # Same-directory relative paths with a file extension (e.g. ./foo.sh, ./test.py)
    r"\./[^`\s/]+\.[a-zA-Z0-9]{1,10}"
    r"|"
    # Deeper relative paths with a file extension (e.g. ../reference/schema.sql)
    r"\.[./][^`\s]*/[^`\s]*\.[a-zA-Z0-9]{1,10}"
    r"|"
    # Relative paths ending in "/" (e.g. ./scripts/, ../data/)
    r"\.[./][^`\s]*/"
    r")`"
)

# Skip tokens that are purely a file-type placeholder with no path component —
# e.g. a bare `.img.xz` or `.vti` suffix used as a type label in prose.
# Do NOT skip these when they appear as suffixes of a real path like
# `internal/db/migrations/000031_table_naming.down.sql`.
PLACEHOLDER_SUFFIXES = {
    ".img.xz", ".img.gz",
    ".vti", ".vts", ".vtp",
}

# Directories to skip when walking the repo.
SKIP_DIRS = {
    ".git", "node_modules", ".venv", "__pycache__",
    "vendor", ".build", "DerivedData", "build",
}


def _is_placeholder(token: str) -> bool:
    """Return True if the token is a descriptive placeholder, not a real path."""
    # Contains glob meta-characters or template placeholders.
    if any(c in token for c in ("*", "<", ">")):
        return True
    # Pure extension token like `.venv/`, `.down.sql`.
    stripped = token.rstrip("/")
    if stripped.startswith(".") and "/" not in stripped:
        return True
    # Known multi-part placeholder suffixes.
    for suffix in PLACEHOLDER_SUFFIXES:
        if token.endswith(suffix):
            return True
    return False


def _resolve(token: str, source_file: Path, repo_root: Path) -> bool:
    """Return True if *token* resolves to an existing path on disk."""
    # Strip anchor fragment.
    path_part = token.split("#")[0].rstrip("/")
    if not path_part:
        return True

    # Repo-root-relative prefixes: always resolve from root.
    if token.startswith((".github/", ".claude/")):
        return (repo_root / path_part).exists()

    # General case: try repo-root first, then file-relative.
    if (repo_root / path_part).exists():
        return True
    if (source_file.parent / path_part).resolve().exists():
        return True
    return False


def find_markdown_files(root: Path) -> list[Path]:
    results: list[Path] = []
    for dirpath, dirnames, filenames in os.walk(root):
        dirnames[:] = [d for d in dirnames if d not in SKIP_DIRS]
        for fname in filenames:
            if fname.endswith(".md"):
                results.append(Path(dirpath) / fname)
    results.sort()
    return results


def check_file(
    filepath: Path, repo_root: Path
) -> list[tuple[int, str]]:
    """Return (line_number, token) pairs for stale backtick paths."""
    findings: list[tuple[int, str]] = []
    try:
        text = filepath.read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError) as exc:
        print(f"warning: could not read {filepath}: {exc}", file=sys.stderr)
        return findings

    in_fence = False
    for lineno, line in enumerate(text.splitlines(), start=1):
        # Track fenced code blocks (``` or ~~~) — skip their contents entirely.
        # Paths inside fences are illustrative examples, not live references.
        stripped = line.strip()
        if stripped.startswith("```") or stripped.startswith("~~~"):
            in_fence = not in_fence
            continue
        if in_fence:
            continue

        # Lines annotated with <!-- link-ignore --> are intentionally stale
        # (e.g. references to planned-but-not-yet-created files, or historical
        # paths in completed plan docs).  Skip them entirely.
        if "<!-- link-ignore -->" in line:
            continue

        for match in BACKTICK_PATH_RE.finditer(line):
            token = match.group(1)
            if _is_placeholder(token):
                continue
            if not _resolve(token, filepath, repo_root):
                findings.append((lineno, token))

    return findings


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Check backtick-quoted file paths in Markdown for stale targets."
    )
    parser.add_argument(
        "--report",
        action="store_true",
        help="Print findings but always exit 0 (advisory mode).",
    )
    parser.add_argument(
        "paths",
        nargs="*",
        help="Files or directories to check (default: repo root).",
    )
    args = parser.parse_args()

    script_dir = Path(__file__).resolve().parent
    repo_root = script_dir.parent

    if args.paths:
        targets: list[Path] = []
        for t in args.paths:
            p = Path(t)
            if not p.is_absolute():
                p = repo_root / t
            targets.append(p)
    else:
        targets = [repo_root]

    files: list[Path] = []
    seen: set[Path] = set()
    for p in targets:
        if p.is_file() and p.suffix == ".md":
            if p not in seen:
                files.append(p)
                seen.add(p)
        elif p.is_dir():
            for fp in find_markdown_files(p):
                if fp not in seen:
                    files.append(fp)
                    seen.add(fp)

    total = 0
    for filepath in files:
        findings = check_file(filepath, repo_root)
        for lineno, token in findings:
            rel = os.path.relpath(filepath, repo_root)
            print(f"STALE PATH: {rel}:{lineno}: `{token}`")
            total += 1

    if total > 0:
        print(f"\n{total} stale backtick path(s) found.")
        if args.report:
            return 0
        return 1

    print("All backtick paths OK.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
