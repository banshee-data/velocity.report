#!/usr/bin/env python3
"""Reject Markdown that points at agent-native document storage as its source.

Documents live in the repository as Markdown (AGENTS.md, "Keep documents in the
repository"). An agent's own store, such as a Claude artifact or doc, a ChatGPT
canvas or a shared chat, is private to one account, unversioned with the code and
invisible to the next agent, so a committed file must not send its reader there
for the content. This check finds links to those stores in tracked Markdown.

A line that must keep such a link, for example to quote one in a migration
note, carries the marker `agent-doc-link: ignore` in an HTML comment.

Usage:
    python3 scripts/check-agent-doc-links.py           # exit non-zero on findings
    python3 scripts/check-agent-doc-links.py --report  # print findings, always exit 0
    python3 scripts/check-agent-doc-links.py FILE...   # check only these paths
"""

from __future__ import annotations

import argparse
import re
import subprocess
import sys
from pathlib import Path

IGNORE_MARKER = "agent-doc-link: ignore"

# (identifier, pattern) for hosts where an agent keeps documents or conversations.
PATTERNS: tuple[tuple[str, str], ...] = (
    ("claude-artifact", r"claude\.ai/(?:code/)?artifacts?/"),
    ("claude-chat", r"claude\.ai/(?:chat|share|project)/"),
    ("chatgpt", r"(?:chatgpt\.com|chat\.openai\.com)/(?:c|g|share|canvas)/"),
    ("gemini", r"gemini\.google\.com/(?:app|share|gem)/"),
    ("copilot-chat", r"(?:copilot\.microsoft\.com|github\.com/copilot/c)/"),
)
COMPILED = tuple(
    (name, re.compile(pattern, re.IGNORECASE)) for name, pattern in PATTERNS
)


def scan_text(text: str) -> list[tuple[int, str, str]]:
    """Return (line number, rule, matched text) for each finding in *text*."""
    findings: list[tuple[int, str, str]] = []
    fenced = False
    for lineno, line in enumerate(text.splitlines(), start=1):
        if line.lstrip().startswith(("```", "~~~")):
            fenced = not fenced
            continue
        if fenced or IGNORE_MARKER in line:
            continue
        for name, pattern in COMPILED:
            for match in pattern.finditer(line):
                findings.append((lineno, name, match.group(0)))
    return findings


def tracked_markdown(repo_root: Path) -> list[str]:
    out = subprocess.run(
        ["git", "ls-files", "-z", "--", "*.md"],
        cwd=repo_root,
        check=True,
        capture_output=True,
    ).stdout.decode()
    return [p for p in out.split("\0") if p]


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument(
        "paths", nargs="*", help="files to check (default: tracked Markdown)"
    )
    parser.add_argument("--report", action="store_true", help="always exit 0")
    args = parser.parse_args()

    repo_root = Path(__file__).resolve().parents[1]
    paths = args.paths or tracked_markdown(repo_root)
    total = 0
    for rel in paths:
        path = repo_root / rel if not Path(rel).is_absolute() else Path(rel)
        try:
            text = path.read_text(encoding="utf-8")
        except (OSError, UnicodeDecodeError):
            continue
        for lineno, name, match in scan_text(text):
            total += 1
            print(
                f"{rel}:{lineno}: [{name}] {match} — move the content into the repository and link the file"
            )
    if total:
        print(
            f"\n{total} link(s) to agent-native document storage. Documents live in the "
            "repository as Markdown; see AGENTS.md, 'Keep documents in the repository'."
        )
        return 0 if args.report else 1
    print("Agent document links OK: no links to agent-native storage.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
