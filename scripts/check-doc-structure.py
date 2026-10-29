#!/usr/bin/env python3
"""Check research/plan titles, opening summaries, status metadata, and heading levels.

Plans keep topic-specific sections; the template is a guide rather than a fixed
outline. Research assessments expose evidence and open questions separately.
Graduated plan symlinks are checked at their canonical home by the existing docs
checks, not treated as active implementation plans here.
"""

from __future__ import annotations

import argparse
import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
HEADING = re.compile(r"^(#{1,6})\s+(.+)")


def visible_lines(text: str) -> list[tuple[int, str]]:
    lines = []
    fence = None
    comment = False
    for number, line in enumerate(text.splitlines(), 1):
        if fence:
            if re.match(
                r"^\s{0,3}" + re.escape(fence[0]) + "{" + str(len(fence)) + r",}\s*$",
                line,
            ):
                fence = None
            continue
        found = re.match(r"^\s{0,3}(`{3,}|~{3,})", line)
        if found:
            fence = found[1]
            continue
        if comment:
            if "-->" in line:
                comment = False
            continue
        if line.strip().startswith("<!--"):
            comment = "-->" not in line
            continue
        lines.append((number, line))
    return lines


def validate(text: str, research: bool = False) -> list[str]:
    lines = visible_lines(text)
    headings = [(n, len(m[1]), m[2]) for n, line in lines if (m := HEADING.match(line))]
    problems = []
    if sum(level == 1 for _, level, _ in headings) != 1:
        problems.append("expected exactly one document title")
    if not headings or headings[0][1] != 1:
        problems.append("document must start with a title")
    for previous, current in zip(headings, headings[1:]):
        if current[1] > previous[1] + 1:
            problems.append(f"line {current[0]}: heading level skips a parent")
    first_section = next(
        (n for n, level, _ in headings if level > 1), len(text.splitlines()) + 1
    )
    header = "\n".join(
        line for n, line in lines if n < first_section and not HEADING.match(line)
    )
    blocks = [block.strip() for block in header.split("\n\n") if block.strip()]
    prose = [
        block
        for block in blocks
        if not re.match(r"^(?:[-*+>]\s|\d+\.\s|\||<|`{3}|~{3})", block)
    ]
    if not prose:
        problems.append("missing opening narrative before the first section")
    if not re.search(r"^- \*\*Status:\*\*\s+\S", header, re.M):
        problems.append("missing canonical Status metadata")
    if research:
        section_names = [name.casefold() for _, level, name in headings if level == 2]
        if not any(name in {"evidence", "research reviewed"} for name in section_names):
            problems.append("research assessment needs an Evidence section")
        if "open questions" not in section_names:
            problems.append("research assessment needs an Open questions section")
    return problems


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", type=Path, default=ROOT)
    args = parser.parse_args()
    problems = []
    count = 0
    for folder in ("docs/plans", "docs/research"):
        for path in sorted((args.repo / folder).rglob("*.md")):
            if path.is_symlink():
                continue
            count += 1
            research = folder == "docs/research" and path.name != "README.md"
            problems.extend(
                f"{path.relative_to(args.repo)}: {problem}"
                for problem in validate(path.read_text(), research)
            )
    if problems:
        print("\n".join(problems))
        return 1
    print(f"Document structure OK: {count} research and plan documents checked.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
