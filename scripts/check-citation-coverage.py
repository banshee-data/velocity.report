#!/usr/bin/env python3
"""Check DOI/arXiv coverage and explicit research-note keys against references.bib.

Author-only mentions still require the human reference audit. This check catches
stable identifiers when they first enter research notes or plans, including
preprint links whose published work already has a bibliography entry.
"""

from __future__ import annotations

import argparse
import re
from pathlib import Path
from urllib.parse import unquote

ROOT = Path(__file__).resolve().parents[1]
DOI = re.compile(r"10\.\d{4,9}/[^\s<>\]\"}`]+", re.I)
ARXIV = re.compile(
    r"(?:arxiv\.org/(?:abs|pdf|html)/|arxiv[:\s]+|10\.48550/arxiv\.)"
    r"(\d{4}\.\d{4,5})(?:v\d+)?",
    re.I,
)
ENTRY = re.compile(r"^@\w+\{([^,\s]+),", re.M)
EPRINT = re.compile(r"^\s*eprint\s*=\s*\{(\d{4}\.\d{4,5})(?:v\d+)?\}", re.M | re.I)


def identifiers(text: str) -> set[str]:
    text = unquote(text)
    result = {"arxiv:" + match[1] for match in ARXIV.finditer(text)}
    for match in DOI.finditer(text):
        value = match[0].rstrip(".,;:")
        # Markdown closing parentheses are not part of a DOI; balanced DOI
        # parentheses, such as older publisher identifiers, are retained.
        while value.endswith(")") and value.count(")") > value.count("("):
            value = value[:-1]
        if not value.lower().startswith("10.48550/arxiv."):
            result.add("doi:" + value.lower())
    return result


def validate(bibliography: str, documents: dict[str, str]) -> list[str]:
    keys = ENTRY.findall(bibliography)
    known = identifiers(bibliography) | {
        "arxiv:" + match[1] for match in EPRINT.finditer(bibliography)
    }
    problems = [
        f"duplicate bibliography key: {key}"
        for key in sorted(set(keys))
        if keys.count(key) > 1
    ]
    work_ids: dict[str, str] = {}
    for block in re.split(r"(?m)(?=^@\w+\{)", bibliography)[1:]:
        key = ENTRY.search(block)[1]
        block_ids = identifiers(block) | {
            "arxiv:" + match[1] for match in EPRINT.finditer(block)
        }
        for value in sorted(block_ids):
            if value in work_ids and work_ids[value] != key:
                problems.append(f"duplicate work {value}: {work_ids[value]} and {key}")
            work_ids[value] = key
    for path, text in sorted(documents.items()):
        for number, line in enumerate(text.splitlines(), 1):
            for value in sorted(identifiers(line) - known):
                problems.append(
                    f"{path}:{number}: missing bibliography entry for {value}"
                )
            if line.startswith("- **References:**"):
                for key in re.findall(r"`([A-Za-z][A-Za-z0-9]+)`", line):
                    if key not in keys:
                        problems.append(
                            f"{path}:{number}: unknown bibliography key {key}"
                        )
    return problems


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", type=Path, default=ROOT)
    args = parser.parse_args()
    documents = {
        str(path.relative_to(args.repo)): path.read_text()
        for folder in ("docs/plans", "docs/research")
        for path in sorted((args.repo / folder).rglob("*.md"))
    }
    problems = validate(
        (args.repo / "data/maths/references.bib").read_text(), documents
    )
    if problems:
        print("\n".join(problems))
        return 1
    print(
        f"Citation coverage OK: {len(documents)} research and plan documents checked."
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
