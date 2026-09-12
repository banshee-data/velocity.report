#!/usr/bin/env python3
"""Materialise one provenance-preserving static PCAPNG per indexed S2 site.

The archive rolls a capture every few minutes.  An export therefore clips every
source file to the site's exact clock interval and merges the clipped pieces in
capture order.  The site interval, rather than only classifier-static pieces,
is deliberate: site-joins.json may declare a short tripod nudge as still part
of the same parked survey (currently van-ness-sacramento).
"""

import argparse
import hashlib
import json
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

HERE = Path(__file__).resolve().parent
DEFAULT_INDEX = HERE / "site-index.json"
DEFAULT_EDITCAP = "/Applications/Wireshark.app/Contents/MacOS/editcap"
DEFAULT_MERGECAP = "/Applications/Wireshark.app/Contents/MacOS/mergecap"


def executable(name, fallback):
    return shutil.which(name) or (fallback if Path(fallback).is_file() else None)


def sha256(path):
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(block)
    return "sha256:" + digest.hexdigest()


def run(command, dry_run):
    print("+", " ".join(str(part) for part in command))
    if not dry_run:
        subprocess.run(command, check=True)


def export(site, archive, output, editcap, mergecap, dry_run):
    site_id = site["id"]
    start, end = site["start"], site["end"]
    captures = [archive / name for name in site["captures"]]
    missing = [str(path) for path in captures if not path.is_file()]
    if missing:
        raise RuntimeError(
            f"{site_id}: missing source capture(s): {', '.join(missing)}"
        )
    destination = output / f"{site_id}.pcapng"
    sidecar = output / f"{site_id}.json"
    if destination.exists() or sidecar.exists():
        raise RuntimeError(
            f"{site_id}: output already exists; refuse to revise an export"
        )

    print(f"{site_id}: {start} to {end}; {len(captures)} source file(s)")
    if dry_run:
        for capture in captures:
            run(
                [editcap, "-F", "pcapng", "-A", start, "-B", end, capture, destination],
                True,
            )
        return

    with tempfile.TemporaryDirectory(prefix=f"{site_id}-", dir=output) as temp_dir:
        parts = []
        for number, capture in enumerate(captures):
            part = Path(temp_dir) / f"{number:02d}-{capture.stem}.pcapng"
            run([editcap, "-F", "pcapng", "-A", start, "-B", end, capture, part], False)
            parts.append(part)
        run([mergecap, "-F", "pcapng", "-w", destination, *parts], False)

    metadata = {
        "schema_version": 1,
        "site_id": site_id,
        "site": site["site"],
        "where": site.get("where"),
        "source_index": "tools/s2-archive/site-index.json",
        "static_export_policy": site["static_export_policy"],
        "start": start,
        "end": end,
        "operator_overrides": site.get("static_export_overrides", []),
        "source_files": [
            {"name": path.name, "sha256": sha256(path), "bytes": path.stat().st_size}
            for path in captures
        ],
        "output_file": destination.name,
        "output_sha256": sha256(destination),
        "output_bytes": destination.stat().st_size,
    }
    sidecar.write_text(json.dumps(metadata, indent=2) + "\n")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--archive",
        required=True,
        type=Path,
        help="directory containing source S2 PCAP files",
    )
    parser.add_argument(
        "--output",
        required=True,
        type=Path,
        help="new directory for PCAPNG exports and sidecars",
    )
    parser.add_argument("--index", type=Path, default=DEFAULT_INDEX)
    parser.add_argument(
        "--site",
        action="append",
        default=[],
        help="site id to export; repeat. Empty exports all sites",
    )
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args()
    editcap = executable("editcap", DEFAULT_EDITCAP)
    mergecap = executable("mergecap", DEFAULT_MERGECAP)
    if not editcap or not mergecap:
        parser.error("Wireshark editcap and mergecap are required")
    sites = json.loads(args.index.read_text())
    wanted = set(args.site)
    selected = [site for site in sites if not wanted or site["id"] in wanted]
    unknown = wanted - {site["id"] for site in selected}
    if unknown:
        parser.error("unknown site(s): " + ", ".join(sorted(unknown)))
    if not args.dry_run:
        args.output.mkdir(parents=True, exist_ok=True)
    for site in selected:
        export(site, args.archive, args.output, editcap, mergecap, args.dry_run)


if __name__ == "__main__":
    try:
        main()
    except (RuntimeError, subprocess.CalledProcessError) as error:
        print(f"static export: {error}", file=sys.stderr)
        raise SystemExit(1)
