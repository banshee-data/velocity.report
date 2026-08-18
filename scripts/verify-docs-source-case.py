#!/usr/bin/env python3
"""Reject offline-docs sources whose on-disk spelling differs from Git."""

from __future__ import annotations

import subprocess
from pathlib import Path


def source_path(path: str) -> bool:
    return path.startswith(("docs/", "data/")) or "/" not in path and path.endswith(".md")


def main() -> None:
    repository = Path(__file__).resolve().parent.parent
    result = subprocess.run(
        ["git", "-C", str(repository), "ls-files", "-z"],
        check=True,
        capture_output=True,
    )
    mismatches: list[str] = []
    for tracked in result.stdout.decode("utf-8").split("\0"):
        if not tracked or not source_path(tracked):
            continue
        current = repository
        for component in Path(tracked).parts:
            names = {entry.name for entry in current.iterdir()}
            if component not in names:
                same_name = next(
                    (name for name in names if name.casefold() == component.casefold()),
                    None,
                )
                if same_name:
                    mismatches.append(
                        f"{tracked}: expected {component!r}, found {same_name!r}"
                    )
                else:
                    mismatches.append(f"{tracked}: missing path component {component!r}")
                break
            current /= component

    if mismatches:
        raise SystemExit(
            "offline docs source path casing does not match Git:\n  - "
            + "\n  - ".join(mismatches)
        )
    print("✓ Offline docs source paths match Git casing")


if __name__ == "__main__":
    main()
