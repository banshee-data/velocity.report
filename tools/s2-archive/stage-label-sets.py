#!/usr/bin/env python3
"""Stage label sets into the Hugging Face dataset, joined to their captures.

A label set cites returns of a capture by packet key, so it applies to any
file holding the capture's packets. This script attaches one to a dataset
capture only when the label set has verified that capture's published file:
its labels.json `applies_to` must name the manifest row's `raw_sha256`. With
`--capture-id`, it first binds the label set to that row's raw file, which
reads the file and records the result.

For each label set it then publishes the current state (no journal, no lock)
to labels/<capture stem>/<label set name>/, adds the path to the row's
`label_set_paths`, and makes sure the dataset's .gitattributes and .gitignore
carry the label set rules. It never overwrites a published label set.

Usage:
  stage-label-sets.py --dataset DIR --labels DIR [--labels DIR ...]
                      [--capture-id ID] [--velocity BIN] [--port 2369] [--dry-run]
"""

from __future__ import annotations

import argparse
import json
import pathlib
import subprocess
import sys

GITATTRIBUTES = [
    "# Label sets: digests cover exact bytes, so never convert line endings.",
    "labels/** -text",
    "labels/**/*.ndjson filter=lfs diff=lfs merge=lfs -text",
]
GITIGNORE = [
    "# Label set working history stays local; the dataset's Git history is the published history.",
    "labels/**/journal.ndjson",
    "labels/**/journal-base/",
    "labels/**/.lock",
    "labels/**/*.tmp",
]


def load_json(path: pathlib.Path):
    return json.loads(path.read_text(encoding="utf-8"))


def dump_manifest(rows) -> str:
    # Matches the staging script's JSON.stringify(rows, null, 2) byte for byte.
    return json.dumps(rows, indent=2, ensure_ascii=False) + "\n"


def stem_of(row) -> str:
    return pathlib.PurePosixPath(row["raw_path"]).stem


def verified_rows(rows, label_manifest):
    shas = {b.get("sha256") for b in label_manifest.get("applies_to") or []}
    return [r for r in rows if r.get("raw_sha256") in shas]


def ensure_lines(path: pathlib.Path, lines, dry_run: bool) -> bool:
    existing = path.read_text(encoding="utf-8") if path.exists() else ""
    have = set(existing.splitlines())
    missing = [line for line in lines if line not in have]
    if not missing:
        return False
    if not dry_run:
        text = existing
        if text and not text.endswith("\n"):
            text += "\n"
        path.write_text(text + "\n".join(missing) + "\n", encoding="utf-8")
    return True


def run(cmd, dry_run: bool):
    print("$ " + " ".join(str(c) for c in cmd))
    if not dry_run:
        subprocess.run(cmd, check=True)


def stage(args) -> int:
    dataset = pathlib.Path(args.dataset)
    manifest_path = dataset / "manifest.json"
    rows = load_json(manifest_path)
    original = manifest_path.read_text(encoding="utf-8")
    if dump_manifest(rows) != original:
        print(
            "manifest.json would not round-trip unchanged; refusing to rewrite it",
            file=sys.stderr,
        )
        return 1
    changed = False
    for labels_dir in args.labels:
        labels_dir = pathlib.Path(labels_dir)
        label_manifest = load_json(labels_dir / "labels.json")
        name = label_manifest.get("name") or labels_dir.name
        if args.capture_id:
            targets = [r for r in rows if r.get("capture_id") == args.capture_id]
            if not targets:
                print(
                    f"no manifest row has capture_id {args.capture_id}", file=sys.stderr
                )
                return 1
            raw = dataset / targets[0]["raw_path"]
            run(
                [
                    args.velocity,
                    "lidar",
                    "labels",
                    "bind",
                    "--labels",
                    labels_dir,
                    "--pcap",
                    raw,
                    "--port",
                    str(args.port),
                ],
                args.dry_run,
            )
            if not args.dry_run:
                label_manifest = load_json(labels_dir / "labels.json")
        matches = verified_rows(rows, label_manifest)
        if args.dry_run and args.capture_id and not matches:
            matches = [r for r in rows if r.get("capture_id") == args.capture_id]
        if not matches:
            print(
                f"{labels_dir}: its verified captures match no dataset row's raw_sha256; "
                "bind it to a published file first (--capture-id)",
                file=sys.stderr,
            )
            return 1
        if len(matches) > 1:
            print(
                f"{labels_dir}: verified against {len(matches)} dataset captures; stage one at a time",
                file=sys.stderr,
            )
            return 1
        row = matches[0]
        rel = pathlib.PurePosixPath("labels", stem_of(row), name)
        dest = dataset / rel
        if dest.exists():
            print(
                f"{dest} exists; a published label set is never overwritten",
                file=sys.stderr,
            )
            return 1
        run(
            [
                args.velocity,
                "lidar",
                "labels",
                "publish",
                "--labels",
                labels_dir,
                "--dest",
                dest,
            ],
            args.dry_run,
        )
        paths = sorted(set((row.get("label_set_paths") or []) + [str(rel)]))
        row["label_set_paths"] = paths
        changed = True
        print(f"staged {labels_dir} as {rel} for {row['capture_id']}")
    if changed and not args.dry_run:
        manifest_path.write_text(dump_manifest(rows), encoding="utf-8")
    for path, lines in (
        (dataset / ".gitattributes", GITATTRIBUTES),
        (dataset / ".gitignore", GITIGNORE),
    ):
        if ensure_lines(path, lines, args.dry_run):
            print(
                f"{'would add' if args.dry_run else 'added'} label set rules to {path}"
            )
    return 0


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter
    )
    parser.add_argument(
        "--dataset", required=True, help="dataset root holding manifest.json"
    )
    parser.add_argument(
        "--labels",
        required=True,
        action="append",
        help="label set directory (repeatable)",
    )
    parser.add_argument(
        "--capture-id", help="bind to this manifest row's raw file first"
    )
    parser.add_argument(
        "--velocity", default="./velocity", help="velocity binary built by make"
    )
    parser.add_argument("--port", type=int, default=2369, help="LiDAR data UDP port")
    parser.add_argument(
        "--dry-run", action="store_true", help="print what would be done"
    )
    return stage(parser.parse_args(argv))


if __name__ == "__main__":
    sys.exit(main())
