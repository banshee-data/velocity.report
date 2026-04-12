#!/usr/bin/env python3
"""Detect and optionally fix backtick-quoted file paths in Markdown prose.

This checker enforces a style rule: file/path references in prose should use
Markdown links instead of standalone backtick spans.

Examples:
- Bad: `internal/config/tuning.go`
- Good: [internal/config/tuning.go](../../internal/config/tuning.go)

Notes:
- Fenced code blocks are ignored.
- Backtick paths already inside a Markdown link are ignored.
- Only resolvable repo paths are fixable automatically.
"""

from __future__ import annotations

import argparse
import os
import re
import sys
from pathlib import Path

INLINE_CODE_RE = re.compile(r"`([^`\n]+)`")
LINK_RE = re.compile(r"\[[^\]]*\]\([^)]*\)")

# File extensions treated as likely file references when there is no slash.
KNOWN_EXTS = {
    "md",
    "go",
    "py",
    "ts",
    "tsx",
    "js",
    "jsx",
    "json",
    "sql",
    "sh",
    "yaml",
    "yml",
    "toml",
    "proto",
    "swift",
    "txt",
    "svg",
    "png",
    "jpg",
    "jpeg",
    "gif",
    "webp",
    "pdf",
    "ini",
    "cfg",
    "conf",
    "lock",
}

SKIP_DIRS = {
    ".git",
    "node_modules",
    ".venv",
    "__pycache__",
    "vendor",
    ".build",
    "DerivedData",
    "build",
    ".pi-gen",
}

NON_FILE_PREFIXES = (
    "http://",
    "https://",
    "mailto:",
    "GET ",
    "POST ",
    "PUT ",
    "PATCH ",
    "DELETE ",
    "DEL ",
)


def find_markdown_files(root: Path) -> list[Path]:
    files: list[Path] = []
    for dirpath, dirnames, filenames in os.walk(root):
        dirnames[:] = [d for d in dirnames if d not in SKIP_DIRS]
        for name in filenames:
            if name.endswith(".md"):
                files.append(Path(dirpath) / name)
    files.sort()
    return files


def _in_link_span(start: int, end: int, link_spans: list[tuple[int, int]]) -> bool:
    for s, e in link_spans:
        if start >= s and end <= e:
            return True
    return False


def _looks_like_file_ref(token: str) -> bool:
    token = token.strip()
    if not token:
        return False
    if any(token.startswith(p) for p in NON_FILE_PREFIXES):
        return False
    if token.startswith("/"):
        # absolute paths are runtime/system paths, not repo docs links
        return False
    if " " in token:
        return False
    if any(ch in token for ch in ("*", "{", "}", "$", "|", "<", ">")):
        return False

    candidate = token.split("#", 1)[0].rstrip("/")
    if not candidate:
        return False

    if "/" in candidate:
        return True

    if "." in candidate:
        ext = candidate.rsplit(".", 1)[1].lower()
        return ext in KNOWN_EXTS

    return False


def _resolve_repo_path(
    token: str, source_file: Path, repo_root: Path
) -> tuple[Path | None, str]:
    """Resolve token to a repository path and return (path, anchor)."""
    anchor = ""
    if "#" in token:
        token_no_anchor, frag = token.split("#", 1)
        anchor = f"#{frag}"
    else:
        token_no_anchor = token

    trailing_slash = token_no_anchor.endswith("/")
    path_part = token_no_anchor.rstrip("/")

    if not path_part:
        return None, anchor

    # First try relative to source file directory.
    source_candidate = (source_file.resolve().parent / path_part).resolve()
    try:
        source_candidate.relative_to(repo_root)
        source_inside = True
    except ValueError:
        source_inside = False

    if source_inside and source_candidate.exists():
        rel = source_candidate.relative_to(repo_root)
        return (
            Path(
                str(rel) + ("/" if trailing_slash and source_candidate.is_dir() else "")
            ),
            anchor,
        )

    # Then try relative to repository root.
    root_candidate = (repo_root / path_part).resolve()
    try:
        root_candidate.relative_to(repo_root)
        root_inside = True
    except ValueError:
        root_inside = False

    if root_inside and root_candidate.exists():
        rel = root_candidate.relative_to(repo_root)
        return (
            Path(
                str(rel) + ("/" if trailing_slash and root_candidate.is_dir() else "")
            ),
            anchor,
        )

    return None, anchor


def _link_target_for(
    source_file: Path, repo_root: Path, repo_path: Path, anchor: str
) -> str:
    source_dir = source_file.resolve().parent
    abs_target = (repo_root / repo_path).resolve()
    rel = os.path.relpath(abs_target, source_dir)
    rel = rel.replace(os.sep, "/")
    return f"{rel}{anchor}"


def process_file(
    filepath: Path, repo_root: Path, fix: bool
) -> tuple[list[tuple[int, str]], bool]:
    findings: list[tuple[int, str]] = []

    try:
        lines = filepath.read_text(encoding="utf-8").splitlines(keepends=True)
    except (OSError, UnicodeDecodeError) as exc:
        print(f"warning: could not read {filepath}: {exc}", file=sys.stderr)
        return findings, False

    in_fence = False
    changed = False
    new_lines: list[str] = []

    for lineno, line in enumerate(lines, start=1):
        stripped = line.strip()
        if stripped.startswith("```") or stripped.startswith("~~~"):
            in_fence = not in_fence
            new_lines.append(line)
            continue

        if in_fence or "<!-- link-ignore -->" in line:
            new_lines.append(line)
            continue

        link_spans = [m.span() for m in LINK_RE.finditer(line)]

        replacements: list[tuple[int, int, str]] = []
        for m in INLINE_CODE_RE.finditer(line):
            token = m.group(1)
            if _in_link_span(m.start(), m.end(), link_spans):
                continue
            if not _looks_like_file_ref(token):
                continue

            resolved, anchor = _resolve_repo_path(token, filepath, repo_root)
            if resolved is None:
                continue

            findings.append((lineno, token))

            if fix:
                target = _link_target_for(filepath, repo_root, resolved, anchor)
                replacements.append((m.start(), m.end(), f"[{token}]({target})"))

        if fix and replacements:
            out = []
            cursor = 0
            for s, e, rep in replacements:
                out.append(line[cursor:s])
                out.append(rep)
                cursor = e
            out.append(line[cursor:])
            line = "".join(out)
            changed = True

        new_lines.append(line)

    if fix and changed:
        filepath.write_text("".join(new_lines), encoding="utf-8")

    return findings, changed


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Check and optionally fix backtick-quoted file paths in Markdown prose."
    )
    parser.add_argument(
        "--report",
        action="store_true",
        help="Print findings but always exit 0 (advisory mode).",
    )
    parser.add_argument(
        "--fix",
        action="store_true",
        help="Auto-convert resolvable backtick file refs to Markdown links.",
    )
    parser.add_argument("paths", nargs="*", help="Files or directories to check.")
    args = parser.parse_args()

    repo_root = Path(__file__).resolve().parent.parent

    if args.paths:
        targets = []
        for raw in args.paths:
            p = Path(raw)
            if not p.is_absolute():
                p = (repo_root / raw).resolve()
            targets.append(p)
    else:
        targets = [repo_root]

    files: list[Path] = []
    seen: set[Path] = set()
    for t in targets:
        if t.is_file() and t.suffix == ".md":
            if t not in seen:
                files.append(t)
                seen.add(t)
        elif t.is_dir():
            for fp in find_markdown_files(t):
                if fp not in seen:
                    files.append(fp)
                    seen.add(fp)

    total = 0
    changed_files = 0
    for fp in files:
        findings, changed = process_file(fp, repo_root, args.fix)
        if changed:
            changed_files += 1
        for lineno, token in findings:
            rel = os.path.relpath(fp, repo_root)
            print(f"BACKTICK FILE REF: {rel}:{lineno}: `{token}`")
            total += 1

    if args.fix:
        print(
            f"\nAuto-fixed {total} backtick file reference(s) across {changed_files} file(s)."
        )
    elif total == 0:
        print("No standalone backtick file references found.")
    else:
        print(f"\n{total} standalone backtick file reference(s) found.")

    if total > 0 and not args.report and not args.fix:
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
