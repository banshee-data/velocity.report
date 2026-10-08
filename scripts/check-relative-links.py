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
from urllib.parse import unquote

# Pattern matches Markdown links: [text](target)
# Captures the target path (group 1).
# Excludes angle-bracket autolinks like <https://...> that may contain parens.
# Known limitation: does not handle balanced parentheses inside link targets
# (e.g. Wikipedia disambiguation pages). These are rare in this repo.
LINK_PATTERN = re.compile(r"\[(?:[^\]]*)\]\(([^)<>]+)\)")

# Directories to skip entirely.
SKIP_DIRS = {
    ".git",
    "node_modules",
    ".venv",
    "__pycache__",
    "vendor",
    ".build",
    "DerivedData",
    "build",
    # pi-gen build output and the upstream pi-gen submodule.
    ".pi-gen",
}


def is_claude_worktree_path(path: Path, repo_root: Path) -> bool:
    """Return True when *path* is within .claude/worktrees/."""
    try:
        rel = path.relative_to(repo_root)
    except ValueError:
        return False
    return rel.parts[:2] == (".claude", "worktrees")


def find_markdown_files(root: Path, repo_root: Path) -> list[Path]:
    """Walk *root* and return all .md files, skipping SKIP_DIRS."""
    results: list[Path] = []
    if is_claude_worktree_path(root, repo_root):
        return results

    for dirpath, dirnames, filenames in os.walk(root):
        current_dir = Path(dirpath)
        # Path of current_dir relative to the repo root, used only to spot the
        # `.claude` directory.  When root is outside repo_root (e.g. an absolute
        # path argument), relative_to raises ValueError; treat that as "not
        # under repo_root" rather than crashing the walk.
        try:
            repo_rel_parts = current_dir.relative_to(repo_root).parts
        except ValueError:
            repo_rel_parts = ()
        dirnames[:] = [
            d
            for d in dirnames
            if d not in SKIP_DIRS
            and not (d == "worktrees" and repo_rel_parts == (".claude",))
            # pi-gen stage package dirs contain bundled copies of repo files
            # whose relative links resolve against the repo root, not the
            # stage directory.  Only skip 'files/' under image/stage-*.
            and not (
                d == "files"
                and len(current_dir.relative_to(root).parts) >= 2
                and current_dir.relative_to(root).parts[0] == "image"
                and current_dir.relative_to(root).parts[1].startswith("stage-")
            )
        ]
        for fname in filenames:
            if fname.endswith(".md"):
                candidate = current_dir / fname
                if not is_claude_worktree_path(candidate, repo_root):
                    results.append(candidate)
    results.sort()
    return results


def check_file(filepath: Path, root: Path) -> list[tuple[int, str, str]]:
    """Return list of (line_number, link_target, resolved_path) for dead links."""
    dead: list[tuple[int, str, str]] = []
    try:
        text = filepath.read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError) as exc:
        print(f"warning: could not read {filepath} as UTF-8: {exc}", file=sys.stderr)
        return dead

    for lineno, line in enumerate(text.splitlines(), start=1):
        # Lines annotated with <!-- link-ignore --> are intentionally stale
        # (e.g. references to planned-but-not-yet-created files, or historical
        # paths in completed plan docs).  Skip them entirely.
        if "<!-- link-ignore -->" in line:
            continue

        for match in LINK_PATTERN.finditer(line):
            target = match.group(1)

            # Skip external URLs, anchors-only, and mailto/data URIs.
            if target.startswith(("http://", "https://", "#", "mailto:", "data:")):
                continue

            # Strip anchor fragment for file-existence check.
            path_part = target.split("#")[0]
            if not path_part:
                continue

            # Decode percent-escapes (e.g. %28 -> "(") so links to paths
            # containing characters that must be escaped in URLs (such as the
            # SvelteKit `(group)` route folders) resolve to real filesystem
            # paths.
            path_part = unquote(path_part)

            # Resolve relative to the directory containing the source file.
            # Use the resolved (real) path so symlinked plan files evaluate
            # links from the canonical hub-doc location, not the symlink dir.
            real_parent = filepath.resolve().parent
            resolved = (real_parent / path_part).resolve()
            # Guard against symlink traversal outside the repo.
            try:
                resolved.relative_to(root.resolve())
            except ValueError:
                rel_resolved = os.path.relpath(resolved, root)
                dead.append((lineno, target, rel_resolved))
                continue
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

    if args.paths:
        targets: list[Path] = []
        for t in args.paths:
            p = Path(t)
            if not p.is_absolute():
                p = repo_root / p
            targets.append(p)
    else:
        targets = [repo_root]

    files: list[Path] = []
    seen: set[Path] = set()
    for p in targets:
        if p.is_file() and p.suffix == ".md":
            if p not in seen and not is_claude_worktree_path(p, repo_root):
                files.append(p)
                seen.add(p)
        elif p.is_dir():
            for fp in find_markdown_files(p, repo_root):
                if fp not in seen:
                    files.append(fp)
                    seen.add(fp)

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
