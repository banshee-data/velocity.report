#!/usr/bin/env python3
"""Check the trimmed S2 corpus against the sizes and digests its manifest records.

A capture that is present but truncated — an interrupted copy, a half-fetched
LFS pointer, a volume that filled — fails in the middle of a replay rather than
at the start of one, which costs an hour to discover and a whole scene to redo.
This reads the manifest and answers the cheap question first.

    S2_CORPUS_DIR=/Volumes/lidar/lidar/sf-street-speeds python3 verify-corpus.py
    SHA=1 ...                       # also verify the digests; reads every byte

Prefer `make scene-corpus-verify`, which supplies the directory.
"""

import hashlib
import json
import os
import sys

MANIFEST = "manifest.json"
BLOCK = 1024 * 1024


def digest(path):
    total = hashlib.sha256()
    with open(path, "rb") as source:
        for block in iter(lambda: source.read(BLOCK), b""):
            total.update(block)
    return "sha256:" + total.hexdigest()


def main():
    root = os.environ.get("S2_CORPUS_DIR", "")
    if not root:
        print("S2_CORPUS_DIR is not set", file=sys.stderr)
        return 2
    manifest_path = os.path.join(root, MANIFEST)
    try:
        with open(manifest_path) as fh:
            entries = json.load(fh)
    except FileNotFoundError:
        print(f"no {MANIFEST} in {root}", file=sys.stderr)
        return 2

    check_sha = os.environ.get("SHA", "") not in ("", "0", "false", "False")
    captures = [e for e in entries if e.get("sensor_type") == "lidar"]
    problems = []
    total_bytes = 0

    for entry in sorted(captures, key=lambda e: e.get("site_slug", "")):
        slug = entry.get("site_slug", "<unnamed>")
        relative = entry.get("raw_path")
        if not relative:
            problems.append(f"{slug}: the manifest names no raw_path")
            continue
        path = os.path.join(root, relative)
        if not os.path.exists(path):
            problems.append(f"{slug}: {relative} is not on disk")
            continue
        size = os.path.getsize(path)
        expected = entry.get("raw_bytes")
        total_bytes += size
        if expected is not None and size != expected:
            problems.append(
                f"{slug}: {relative} is {size} bytes, the manifest says {expected}"
            )
            continue
        if check_sha and entry.get("raw_sha256"):
            found = digest(path)
            if found != entry["raw_sha256"]:
                problems.append(f"{slug}: {relative} does not match its digest")
                continue
        mark = "sha ok" if check_sha else "size ok"
        print(f"  {mark:<7} {slug:<24} {size / 1e9:>6.1f} GB")

    print(
        f"\n{len(captures) - len(problems)} of {len(captures)} captures verified, "
        f"{total_bytes / 1e9:.1f} GB"
    )
    for problem in problems:
        print(f"  PROBLEM {problem}")
    if not check_sha and not problems:
        print("sizes only; SHA=1 verifies the digests as well")
    return 1 if problems else 0


if __name__ == "__main__":
    sys.exit(main())
