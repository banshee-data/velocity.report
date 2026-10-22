#!/usr/bin/env python3
"""Report references.bib sections over their soft caps and entries nothing cites.

Advisory by default: it prints the report and exits 0. With --strict it exits 1
when a section is over its cap, a section has no cap, or a cap names no section.
Uncited entries are always advisory, because a doc may cite a work by author and
year; the reference audit is where those mentions are mapped. The rules are in
data/maths/citation-rubric.md.
"""

import argparse
import json
import re
import subprocess
import sys
from pathlib import Path

BIB = "data/maths/references.bib"
BUDGET = "data/maths/bibliography-budget.json"
# Files that hold the bibliography itself or its budget. The reference graph
# generated from it lives under public_html/, outside SEARCH_ROOTS.
SKIP = {BIB, BUDGET}
SEARCH_ROOTS = [
    "docs",
    "data",
    "internal",
    "cmd",
    "tools",
    "config",
    "proto",
    "scripts",
]
TEXT_SUFFIXES = {
    ".md",
    ".go",
    ".py",
    ".js",
    ".mjs",
    ".ts",
    ".svelte",
    ".swift",
    ".json",
    ".yaml",
    ".yml",
    ".toml",
    ".proto",
    ".sql",
    ".txt",
    ".njk",
    ".html",
    ".sh",
}

SECTION_RULE = re.compile(r"^% =+\s*$")
ENTRY = re.compile(r"^@(\w+)\{([^,\s]+),")
DOI = re.compile(r"^\s*doi\s*=\s*\{([^}]+)\}", re.I)


def parse_bib(text):
    """Return [(key, section, doi)] in file order.

    A section is a comment line framed by two `% ====` rules. The file's own
    title banner (the first framed line) names no section.
    """
    lines = text.splitlines()
    section = None
    entries = []
    current = None
    for i, line in enumerate(lines):
        if (
            SECTION_RULE.match(line)
            and i + 2 < len(lines)
            and SECTION_RULE.match(lines[i + 2])
            and lines[i + 1].startswith("% ")
        ):
            title = lines[i + 1][2:].strip()
            if not title.endswith("references.bib"):
                section = title
            continue
        m = ENTRY.match(line)
        if m:
            current = [m.group(2), section, None]
            entries.append(current)
            continue
        m = DOI.match(line)
        if m and current is not None and current[2] is None:
            current[2] = m.group(1).strip()
    return [tuple(e) for e in entries]


def tracked_text(repo):
    out = subprocess.check_output(
        ["git", "-C", str(repo), "ls-files", *SEARCH_ROOTS, "*.md"], text=True
    )
    chunks = []
    for name in out.splitlines():
        if name in SKIP or Path(name).suffix.lower() not in TEXT_SUFFIXES:
            continue
        try:
            chunks.append((Path(repo) / name).read_text(errors="ignore"))
        except OSError:
            continue
    return "\n".join(chunks)


def report(entries, caps, corpus):
    counts = {}
    for _, section, _ in entries:
        counts[section] = counts.get(section, 0) + 1
    over = [(s, n, caps[s]) for s, n in counts.items() if s in caps and n > caps[s]]
    uncapped = sorted(s for s in counts if s not in caps)
    stale = sorted(s for s in caps if s not in counts)
    lower = corpus.lower()
    uncited = [
        key
        for key, _, doi in entries
        if key not in corpus and not (doi and doi.lower() in lower)
    ]
    return counts, over, uncapped, stale, uncited


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", default=".", help="Git checkout to inspect")
    parser.add_argument("--strict", action="store_true", help="Exit 1 on a cap problem")
    args = parser.parse_args(argv)
    repo = Path(args.repo)
    entries = parse_bib((repo / BIB).read_text())
    caps = json.loads((repo / BUDGET).read_text())["caps"]
    counts, over, uncapped, stale, uncited = report(entries, caps, tracked_text(repo))

    print(f"{len(entries)} entries in {len(counts)} sections")
    for section, n in counts.items():
        cap = caps.get(section)
        mark = " OVER" if cap is not None and n > cap else ""
        print(f"  {n:>3} / {cap if cap is not None else '?':>3}  {section}{mark}")
    for section in uncapped:
        print(f"WARNING: section has no cap in {BUDGET}: {section}")
    for section in stale:
        print(f"WARNING: cap names no section in {BIB}: {section}")
    if uncited:
        print(
            f"Not cited by key or DOI in any tracked file ({len(uncited)}); "
            "check the reference audit for author-year mentions before pruning:"
        )
        for key in uncited:
            print(f"  {key}")
    problems = bool(over or uncapped or stale)
    if problems:
        print(
            "Over cap or unmatched sections: prune, merge, or raise the cap with its reason "
            "in the reference audit (data/maths/citation-rubric.md)."
        )
    return 1 if problems and args.strict else 0


if __name__ == "__main__":
    sys.exit(main())
