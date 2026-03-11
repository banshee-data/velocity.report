#!/usr/bin/env python3
"""Validate Mermaid fences in Markdown files.

This is a lightweight structural checker intended for repo linting. It is not
Mermaid's full parser, but it catches the GitHub-incompatible issues that have
already caused rendering failures in this repository:

- empty node labels such as `A[""]`
- unmatched fenced Mermaid blocks
- unsupported or missing Mermaid headers
- unmatched quotes / brackets on Mermaid lines
- unbalanced `subgraph` / `end` blocks in flowcharts
"""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

EXCLUDED_DIRS = {
    ".git",
    ".venv",
    "_site",
    "build",
    "dist",
    "node_modules",
}

FLOWCHART_HEADER_RE = re.compile(r"^(flowchart|graph)\s+(TB|TD|BT|LR|RL)\s*$")
STATE_HEADER_RE = re.compile(r"^stateDiagram-v2\s*$")
COMMENT_RE = re.compile(r"^\s*%%")
SUBGRAPH_RE = re.compile(r"^\s*subgraph\b")
END_RE = re.compile(r"^\s*end\s*$")
EMPTY_NODE_LABEL_RE = re.compile(
    r"\b[A-Za-z_][A-Za-z0-9_]*\s*(\[\s*\]|\[\s*\"\s*\"\s*\]|\{\s*\}|\(\s*\)|\(\(\s*\)\))"
)


def iter_markdown_paths(paths: list[str]) -> list[Path]:
    found: set[Path] = set()
    out: list[Path] = []

    for raw in paths:
        path = Path(raw)
        if not path.exists():
            continue
        if path.is_file():
            if path.suffix == ".md" and path not in found:
                found.add(path)
                out.append(path)
            continue
        for candidate in sorted(path.rglob("*.md")):
            if any(part in EXCLUDED_DIRS for part in candidate.parts):
                continue
            if candidate in found:
                continue
            found.add(candidate)
            out.append(candidate)

    return out


def extract_mermaid_blocks(path: Path) -> list[tuple[int, list[tuple[int, str]]]]:
    text = path.read_text(encoding="utf-8")
    lines = text.splitlines()
    blocks: list[tuple[int, list[tuple[int, str]]]] = []

    in_block = False
    start_line = 0
    block_lines: list[tuple[int, str]] = []

    for lineno, line in enumerate(lines, 1):
        if not in_block and line.strip() == "```mermaid":
            in_block = True
            start_line = lineno
            block_lines = []
            continue
        if in_block and line.strip() == "```":
            blocks.append((start_line, block_lines))
            in_block = False
            block_lines = []
            continue
        if in_block:
            block_lines.append((lineno, line))

    if in_block:
        raise ValueError(f"{path}:{start_line}: unterminated ```mermaid block")

    return blocks


def first_content_line(
    block: list[tuple[int, str]], path: Path, start_line: int
) -> tuple[int, str]:
    for lineno, raw in block:
        if raw.strip():
            return lineno, raw.strip()
    raise ValueError(f"{path}:{start_line}: empty Mermaid block")


def check_balanced_delimiters(line: str) -> str | None:
    pairs = {"]": "[", "}": "{", ")": "("}
    stack: list[tuple[str, int]] = []
    in_quote = False
    escaped = False

    for idx, ch in enumerate(line):
        if in_quote:
            if escaped:
                escaped = False
                continue
            if ch == "\\":
                escaped = True
                continue
            if ch == '"':
                in_quote = False
            continue

        if ch == '"':
            in_quote = True
            continue
        if ch in {"[", "{", "("}:
            stack.append((ch, idx))
            continue
        if ch in pairs:
            if not stack or stack[-1][0] != pairs[ch]:
                return f"unmatched {ch!r}"
            stack.pop()

    if in_quote:
        return "unterminated quote"
    if stack:
        opener, _ = stack[-1]
        return f"unmatched {opener!r}"
    return None


def validate_flowchart_block(
    path: Path, start_line: int, block: list[tuple[int, str]]
) -> list[str]:
    errors: list[str] = []
    subgraph_depth = 0

    for lineno, raw in block[1:]:
        line = raw.strip()
        if not line or COMMENT_RE.match(line):
            continue
        if EMPTY_NODE_LABEL_RE.search(line):
            errors.append(f"{path}:{lineno}: empty Mermaid node label is not allowed")
        if problem := check_balanced_delimiters(line):
            errors.append(f"{path}:{lineno}: {problem}")
        if SUBGRAPH_RE.match(line):
            subgraph_depth += 1
            continue
        if END_RE.match(line):
            subgraph_depth -= 1
            if subgraph_depth < 0:
                errors.append(
                    f"{path}:{lineno}: unexpected 'end' without matching 'subgraph'"
                )
                subgraph_depth = 0

    if subgraph_depth != 0:
        errors.append(
            f"{path}:{start_line}: unbalanced subgraph/end count in flowchart block"
        )

    return errors


def validate_state_block(path: Path, block: list[tuple[int, str]]) -> list[str]:
    errors: list[str] = []

    for lineno, raw in block[1:]:
        line = raw.strip()
        if not line or COMMENT_RE.match(line):
            continue
        if EMPTY_NODE_LABEL_RE.search(line):
            errors.append(f"{path}:{lineno}: empty Mermaid node label is not allowed")
        if problem := check_balanced_delimiters(line):
            errors.append(f"{path}:{lineno}: {problem}")

    return errors


def validate_block(
    path: Path, start_line: int, block: list[tuple[int, str]]
) -> list[str]:
    header_lineno, header = first_content_line(block, path, start_line)

    if FLOWCHART_HEADER_RE.fullmatch(header):
        return validate_flowchart_block(path, start_line, block)
    if STATE_HEADER_RE.fullmatch(header):
        return validate_state_block(path, block)

    return [
        (
            f"{path}:{header_lineno}: unsupported Mermaid header {header!r}; "
            "expected flowchart/graph with direction or stateDiagram-v2"
        )
    ]


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "paths",
        nargs="*",
        default=["docs", "README.md", "CONTRIBUTING.md"],
        help="Markdown files or directories to scan",
    )
    args = parser.parse_args()

    markdown_paths = iter_markdown_paths(args.paths)
    if not markdown_paths:
        print("No Markdown files found for Mermaid validation.")
        return 0

    errors: list[str] = []
    block_count = 0
    for path in markdown_paths:
        try:
            blocks = extract_mermaid_blocks(path)
        except ValueError as exc:
            errors.append(str(exc))
            continue
        for start_line, block in blocks:
            block_count += 1
            errors.extend(validate_block(path, start_line, block))

    if errors:
        print("Mermaid validation failed:")
        for error in errors:
            print(f"  - {error}")
        return 1

    print(f"Mermaid validation passed ({block_count} block(s) checked).")
    return 0


if __name__ == "__main__":
    sys.exit(main())
