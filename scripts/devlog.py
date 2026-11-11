#!/usr/bin/env python3
"""Validate per-PR journal fragments and fold landed fragments into DEVLOG."""

import argparse
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import posixpath
import re
import subprocess
import sys
from urllib.parse import urlsplit, urlunsplit

PENDING = "docs/devlog-fragments/pending"
LOG = "docs/DEVLOG.md"
LINK = re.compile(r"(!?\[[^\]\n]*\])\(([^\s()]+)\)")
HEADING = re.compile(r"^## ([A-Z][a-z]+ \d{1,2}, \d{4}) - .+$", re.MULTILINE)


def command(*args):
    return subprocess.check_output(args, text=True).strip()


def fragments():
    directory = Path(PENDING)
    if directory.is_symlink():
        raise ValueError("pending directory must not be a symlink")
    if not directory.exists():
        return []
    entries = sorted(directory.iterdir())
    for path in entries:
        if path.name == ".gitkeep" and path.is_file() and not path.is_symlink():
            continue
        if path.suffix != ".md" or not path.is_file() or path.is_symlink():
            raise ValueError(f"{path}: pending accepts only Markdown fragments")
    return [path for path in entries if path.name != ".gitkeep"]


def validate(path):
    if path.is_symlink() or not re.fullmatch(
        r"[a-z0-9]+(?:-[a-z0-9]+)*\.md", path.name
    ):
        raise ValueError(f"{path}: use a unique lowercase hyphenated filename")
    lines = [line for line in path.read_text().splitlines() if line.strip()]
    if not lines or any(
        not line.startswith("- ") or not line[2:].strip() for line in lines
    ):
        raise ValueError(f"{path}: write only single-line Markdown bullets")
    for line in lines:
        if line.startswith("- {") or re.search(r"\(#\d+(?:, #\d+)*\)\.?$", line):
            raise ValueError(f"{path}: omit branch tags and trailing PR numbers")
        # Reference links and complex destinations cannot be relocated reliably.
        if re.search(r"\]\[", line) or re.search(r"\]\(", LINK.sub("", line)):
            raise ValueError(f"{path}: use inline links with URL-encoded destinations")
    return lines


def relocate(line, source):
    def replace(match):
        parts = urlsplit(match[2])
        if parts.scheme or parts.netloc or match[2].startswith(("/", "#")):
            return match[0]
        target = posixpath.normpath(
            posixpath.join(source.parent.as_posix(), parts.path)
        )
        target = posixpath.relpath(target, "docs")
        return f"{match[1]}({urlunsplit(parts._replace(path=target))})"

    return LINK.sub(replace, line)


def render(original, records):
    """Keep historical bytes and insert new bullets before any legacy branch work."""
    matches = list(HEADING.finditer(original))
    first = matches[0].start() if matches else len(original)
    preamble = original[:first]
    sections = {}
    for i, match in enumerate(matches):
        day = datetime.strptime(match[1], "%B %d, %Y").date()
        if day in sections:
            raise ValueError(f"duplicate DEVLOG date: {day}")
        end = matches[i + 1].start() if i + 1 < len(matches) else len(original)
        sections[day] = original[match.start() : end]
    for merged_at, number, path, lines in sorted(
        records, key=lambda r: (r[0], r[1], str(r[2]))
    ):
        day = (
            datetime.fromisoformat(merged_at.replace("Z", "+00:00"))
            .astimezone(timezone.utc)
            .date()
        )
        bullets = "\n".join(
            f'{relocate(line, path).rstrip().removesuffix(".")} (#{number}).'
            for line in lines
        )
        if day not in sections:
            sections[day] = (
                f'## {day.strftime("%B")} {day.day}, {day.year} - Engineering updates\n\n'
            )
        section = sections[day]
        branch = re.search(r"^- \{", section, re.MULTILINE)
        position = branch.start() if branch else len(section.rstrip())
        sections[day] = (
            section[:position].rstrip()
            + "\n\n"
            + bullets
            + "\n\n"
            + section[position:].lstrip("\n")
        )
    return preamble + "".join(sections[day] for day in sorted(sections, reverse=True))


def check_pr(base, body, maintenance):
    changes = command(
        "git", "diff", "--name-status", "--no-renames", f"{base}...HEAD"
    ).splitlines()
    changed = [line.split("\t", 1) for line in changes]
    if any(path == LOG for _, path in changed) and not maintenance:
        raise ValueError(
            "ordinary PRs must add a fragment instead of editing docs/DEVLOG.md"
        )
    if maintenance:
        return
    if any(
        path.startswith(PENDING + "/") and status != "A" for status, path in changed
    ):
        raise ValueError("ordinary PRs must not edit or delete existing fragments")
    added = any(
        status == "A" and path.startswith(PENDING + "/") and path.endswith(".md")
        for status, path in changed
    )
    if not added and not re.search(r"<!--\s*devlog-skip:\s*\S.*?-->", body, re.DOTALL):
        raise ValueError(
            "add a devlog fragment or explain with <!-- devlog-skip: reason --> in the PR body"
        )


def consolidate():
    if command("git", "status", "--porcelain"):
        raise ValueError("consolidation requires a clean checkout")
    command("git", "merge-base", "--is-ancestor", "HEAD", "origin/main")
    records = []
    paths = fragments()
    repository = (
        command(
            "gh", "repo", "view", "--json", "nameWithOwner", "--jq", ".nameWithOwner"
        )
        if paths
        else None
    )
    for path in paths:
        lines = validate(path)
        sha = command(
            "git", "log", "-1", "--diff-filter=A", "--format=%H", "--", str(path)
        )
        pulls = json.loads(
            command(
                "gh",
                "api",
                f"repos/{{owner}}/{{repo}}/commits/{sha}/pulls?per_page=100",
            )
        )
        pulls = [
            p
            for p in pulls
            if p.get("merged_at")
            and p["base"]["ref"] == "main"
            and p["base"]["repo"]["full_name"] == repository
        ]
        if len(pulls) != 1:
            raise ValueError(
                f"{path}: expected one merged PR, found {len(pulls)}; resolve provenance before folding"
            )
        pull = pulls[0]
        records.append((pull["merged_at"], pull["number"], path, lines))
    if not records:
        print("No pending fragments")
        return
    log = Path(LOG)
    updated = render(log.read_text(), records)
    log.write_text(updated)
    for _, _, path, _ in records:
        path.unlink()
    print(f"Consolidated {len(records)} fragments")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=["check", "consolidate"])
    parser.add_argument("--base", default=os.environ.get("DEVLOG_BASE"))
    args = parser.parse_args()
    try:
        for path in fragments():
            validate(path)
        if args.action == "consolidate":
            consolidate()
        elif args.base:
            check_pr(
                args.base,
                os.environ.get("DEVLOG_PR_BODY", ""),
                os.environ.get("DEVLOG_MAINTENANCE") == "true",
            )
    except (ValueError, subprocess.CalledProcessError) as error:
        print(f"devlog: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
