#!/usr/bin/env python3
"""Merge the surveys published from clips into site-index.json.

A clip marked for the dataset on the web Clips page (a dataset id, a location
and a completed extract) is served by the LiDAR API as a site-index entry at
GET /api/lidar/dataset/site-index. This script fetches those entries and
writes them into the checked-in index beside the archive's sites, so the scene
publisher, the scene map and the dataset scripts see them. Each carries
"origin": "clip"; build-site-index.py keeps such entries when it rebuilds the
archive's half.

Entries the server no longer serves are left in place unless --prune is given:
a clip deleted from one database should not silently drop a published survey.

The index is the record a person commits. The script writes it and prints
what changed; it does not commit, push or upload anything.

  LIDAR_API_URL=http://localhost:8080/api/lidar   the server to ask (default)
"""

import argparse
import json
import os
import sys
import urllib.error
import urllib.request

HERE = os.path.dirname(os.path.abspath(__file__))
DEFAULT_INDEX = os.path.join(HERE, "site-index.json")
DEFAULT_API = os.environ.get("LIDAR_API_URL", "http://localhost:8080/api/lidar")
ORIGIN = "clip"


def fetch(api):
    """The server's clip entries and the clips it left out, with reasons."""
    url = api.rstrip("/") + "/dataset/site-index"
    try:
        with urllib.request.urlopen(url, timeout=60) as response:
            body = json.load(response)
    except urllib.error.URLError as error:
        raise SystemExit(f"merge-clip-sites: cannot read {url}: {error}") from error
    return body.get("entries") or [], body.get("skipped") or {}


def merge(index, served, prune=False):
    """Return the merged index and a list of (action, id) changes.

    Archive entries keep their order and come first. Clip entries follow,
    sorted by id; a served entry replaces the stored one for the same id.
    """
    archive = [entry for entry in index if entry.get("origin") != ORIGIN]
    stored = {entry["id"]: entry for entry in index if entry.get("origin") == ORIGIN}
    archive_ids = {entry["id"] for entry in archive}

    changes = []
    merged = dict(stored) if not prune else {}
    for entry in served:
        if entry.get("origin") != ORIGIN:
            raise ValueError(f"{entry.get('id')}: the server sent a non-clip entry")
        entry_id = entry["id"]
        if entry_id in archive_ids:
            raise ValueError(f"{entry_id}: an archive site already has this id")
        if entry_id in merged and merged[entry_id] == entry:
            changes.append(("unchanged", entry_id))
        elif entry_id in stored:
            changes.append(("updated", entry_id))
        else:
            changes.append(("added", entry_id))
        merged[entry_id] = entry
    served_ids = {entry["id"] for entry in served}
    for entry_id in sorted(stored):
        if entry_id not in served_ids:
            changes.append(("removed" if prune else "kept", entry_id))
    return archive + [merged[key] for key in sorted(merged)], changes


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--api", default=DEFAULT_API, help="LiDAR API base URL")
    parser.add_argument(
        "--index", default=DEFAULT_INDEX, help="site-index.json to update"
    )
    parser.add_argument(
        "--prune",
        action="store_true",
        help="drop clip entries the server no longer serves",
    )
    parser.add_argument(
        "--dry-run", action="store_true", help="report the changes without writing"
    )
    args = parser.parse_args(argv)

    with open(args.index) as fh:
        index = json.load(fh)
    served, skipped = fetch(args.api)
    try:
        merged, changes = merge(index, served, prune=args.prune)
    except ValueError as error:
        raise SystemExit(f"merge-clip-sites: {error}") from error

    for action, entry_id in changes:
        print(f"  {action:<9} {entry_id}")
    for entry_id, reason in sorted(skipped.items()):
        print(f"  not ready {entry_id}: {reason}")
    if args.dry_run:
        print("dry run: site-index.json not written")
        return 0
    with open(args.index, "w") as fh:
        json.dump(merged, fh, indent=2)
        fh.write("\n")
    print(f"written: {args.index}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
