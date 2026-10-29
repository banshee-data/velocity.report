#!/usr/bin/env python3
"""Validate priority/workstream metadata and per-release backlog caps (D-32/D-37).

Only outstanding rows are checked; the Complete section is immutable history.
Run from any directory: python3 scripts/check-backlog-format.py
"""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
BACKLOG = ROOT / "docs" / "BACKLOG.md"
ITEM = re.compile(r"^- (?:\[#\d+\] )?\*\*(P[012])\*\* (.+)$")
SIZE = re.compile(r"`(?:S|M|L|XL)`")
STREAM = re.compile(r"`(S-[A-Z]+(?:-[A-Z]+)*)`$")
RELEASE = re.compile(r"^### (v\d+\.\d+\.\d+)\b")
KNOWN_STREAMS = {
    "S-REF",
    "S-OBS",
    "S-GEO",
    "S-UNC",
    "S-MET",
    "S-SCENE",
    "S-EVID",
    "S-MOBILE",
    "S-CLASS",
    "S-RADAR",
    "S-UI",
    "S-PUBLISH",
    "S-PLATFORM",
}
CAP = 12


def validate(data: str) -> list[str]:
    problems: list[str] = []
    release: str | None = None
    counts: dict[str, int] = {}
    for line_number, line in enumerate(data.splitlines(), start=1):
        if line == "## Complete":
            break
        if line.startswith("## "):
            release = None
        found_release = RELEASE.match(line)
        if found_release:
            release = found_release.group(1)
        if not line.startswith("- "):
            if (
                release is not None
                and line.strip()
                and not (
                    line.startswith("#")
                    or re.fullmatch(r'<a\s+id="[^"]+"></a>', line)
                    or (line.startswith("<!--") and line.endswith("-->"))
                )
            ):
                problems.append(
                    f"line {line_number}: milestone content must be a one-line backlog item; "
                    "move rationale and delivery history to the linked plan"
                )
            continue

        if release is not None:
            counts[release] = counts.get(release, 0) + 1

        item = ITEM.match(line)
        if item is None:
            problems.append(
                f"line {line_number}: missing priority directly after optional PR"
            )
            continue
        remainder = item.group(2)
        stream = STREAM.search(remainder)
        if stream is None:
            problems.append(
                f"line {line_number}: final workstream must be backtick S-... token"
            )
            continue
        if stream.group(1) not in KNOWN_STREAMS:
            problems.append(f"line {line_number}: unknown workstream {stream.group(1)}")
        sizes = list(SIZE.finditer(remainder[: stream.start()]))
        if not sizes:
            problems.append(
                f"line {line_number}: missing original size before workstream"
            )
    for name, count in sorted(counts.items()):
        if count > CAP:
            problems.append(f"{name}: {count} outstanding items, maximum {CAP}")
    return problems


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--file", type=Path, default=BACKLOG)
    arguments = parser.parse_args()
    problems = validate(arguments.file.read_text(encoding="utf-8"))
    if problems:
        print("\n".join(problems), file=sys.stderr)
        return 1
    print("Backlog priority, workstream and release-cap checks passed.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
