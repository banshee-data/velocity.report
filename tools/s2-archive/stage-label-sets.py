#!/usr/bin/env python3
"""Stage label sets and their views into the Hugging Face dataset.

A label set cites returns of a capture by packet key, so it applies to any
file holding the capture's packets. This script attaches one to a dataset
capture only when the label set has verified that capture's published file:
its labels.json `applies_to` must name the manifest row's `raw_sha256`. With
`--capture-id`, it first binds the label set to that row's raw file, which
reads the file and records the result.

For each label set it then publishes the current state (no journal, no lock)
to labels/<capture stem>/<label set name>/ and adds the path to the row's
`label_set_paths`. A set already published at the same head is left as it is;
`--replace` refreshes one that is behind (`labels publish --replace`, which
refuses a copy someone has edited).

A label set holds no points, so it cannot be opened for editing on its own.
Each `--view PACK` is a pack linked to a label set (its own label set is
staged too, found through its link): it is published into the set as
labels/<capture stem>/<label set name>/views/<cut time>-<run>/ by
`labels publish-view`, which checks it as a fresh copy would hold it, and its
path is added to the row's `annotation_pack_paths`. A published view is never
overwritten.

The dataset's .gitattributes and .gitignore get the label set rules: exact
bytes, label set text in plain Git (only `.bin` goes through LFS/Xet), and
every file the editors write that is working state rather than a record.

Usage:
  stage-label-sets.py --dataset DIR [--labels DIR ...] [--view PACK ...]
                      [--capture-id ID] [--replace] [--velocity BIN]
                      [--port 2369] [--dry-run]
"""

from __future__ import annotations

import argparse
import datetime
import json
import pathlib
import subprocess
import sys

GITATTRIBUTES = [
    "# Label sets: digests cover exact bytes, so never convert line endings.",
    "# Their text stays in plain Git, one record per line, so diffs and merges work;",
    "# *.bin goes through LFS/Xet by the dataset-wide rule.",
    "labels/** -text",
]
# Rules an earlier staging wrote that the storage layout retires.
OBSOLETE_GITATTRIBUTES = [
    "labels/**/*.ndjson filter=lfs diff=lfs merge=lfs -text",
]
# Hugging Face refuses a push with any plain-Git file over 10 MiB, in any of
# its commits; binary files go through LFS/Xet at any size.
MAX_PLAIN_GIT_BYTES = 10 << 20
LFS_SUFFIXES = (".bin", ".pb", ".pcapng", ".pdf")
GITIGNORE = [
    "# Label set working history stays local; the dataset's Git history is the published history.",
    "labels/**/journal.ndjson",
    "labels/**/journal-base/",
    "labels/**/.lock",
    "labels/**/.pending",
    "labels/**/*.tmp",
    "# A view's working state: locks, revision archives, regenerated proposal layers, staging.",
    "labels/**/.annotations.lock",
    "labels/**/physical-reference-revisions/",
    "labels/**/feature-proposal-revisions/",
    "labels/**/proposals/",
    "labels/**/.publish-view-*/",
]
LINK_FILE = "label-set-link.json"


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


def drop_lines(path: pathlib.Path, lines, dry_run: bool) -> bool:
    if not path.exists():
        return False
    existing = path.read_text(encoding="utf-8").splitlines()
    kept = [line for line in existing if line not in set(lines)]
    if len(kept) == len(existing):
        return False
    if not dry_run:
        path.write_text("\n".join(kept) + ("\n" if kept else ""), encoding="utf-8")
    return True


def oversized(directory: pathlib.Path):
    """Plain-Git files under directory too large for Hugging Face."""
    out = []
    for path in sorted(directory.rglob("*")):
        if path.is_file() and not path.name.endswith(LFS_SUFFIXES):
            if path.stat().st_size > MAX_PLAIN_GIT_BYTES:
                out.append(path)
    return out


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


def capture_name_of(name: str) -> str:
    """The layout's folder name for a file: annotation.CaptureNameOf in Go."""
    base = name.replace("\\", "/").rsplit("/", 1)[-1]
    for ext in (".gz", ".pcapng", ".pcap"):
        if base.endswith(ext):
            base = base[: -len(ext)]
    safe = "".join(
        c if c.isascii() and (c.isalnum() or c in "._-") else "-" for c in base
    )
    safe = safe.strip(".")
    if not safe or safe in ("splits", "labels", "packs", "views"):
        return "capture"
    return safe


def view_name(pack: pathlib.Path) -> str:
    """The folder a pack takes as a view: annotation.ViewName in Go, the
    time its run was cut, to the second, and the run's first 8 characters."""
    manifest = load_json(pack / "manifest.json")
    when = datetime.datetime.fromtimestamp(
        manifest["created_ns"] // 1_000_000_000, tz=datetime.timezone.utc
    )
    run = capture_name_of((manifest.get("source") or {}).get("vrlog_path", ""))[:8]
    return when.strftime("%Y%m%d-%H%M%S") + "-" + run


def view_label_set(pack: pathlib.Path) -> pathlib.Path:
    """The label set a pack is linked to."""
    link = load_json(pack / LINK_FILE)
    return (pack / link["path"]).resolve()


def same_head(a, b) -> bool:
    ha, hb = a.get("head") or {}, b.get("head") or {}
    return (
        bool(ha)
        and ha.get("revision") == hb.get("revision")
        and ha.get("state") == hb.get("state")
    )


def run(cmd, dry_run: bool):
    print("$ " + " ".join(str(c) for c in cmd))
    if not dry_run:
        subprocess.run(cmd, check=True)


def refresh_paths(args) -> int:
    """Rewrite each row's label_set_paths and annotation_pack_paths from what
    is under labels/<capture stem>/: every folder holding labels.json, and
    every pack in its views/. A row keeps no path that is not there."""
    dataset = pathlib.Path(args.dataset)
    manifest_path = dataset / "manifest.json"
    rows = load_json(manifest_path)
    if dump_manifest(rows) != manifest_path.read_text(encoding="utf-8"):
        print(
            "manifest.json would not round-trip unchanged; refusing to rewrite it",
            file=sys.stderr,
        )
        return 1
    changed = False
    for row in rows:
        stem = dataset / "labels" / stem_of(row)
        sets, views = [], []
        if stem.is_dir():
            for child in sorted(stem.iterdir()):
                if not (child / "labels.json").is_file():
                    continue
                sets.append(str(child.relative_to(dataset).as_posix()))
                views_dir = child / "views"
                if views_dir.is_dir():
                    for view in sorted(views_dir.iterdir()):
                        if (view / "manifest.json").is_file():
                            views.append(str(view.relative_to(dataset).as_posix()))
        for key, paths in (("label_set_paths", sets), ("annotation_pack_paths", views)):
            had = row.get(key)
            if not paths and had is None:
                continue
            if had != paths:
                print(f"{row['capture_id']}: {key} {had} -> {paths}")
                row[key] = paths
                changed = True
    if changed and not args.dry_run:
        manifest_path.write_text(dump_manifest(rows), encoding="utf-8")
    return 0


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
    label_dirs = [pathlib.Path(d) for d in args.labels or []]
    views = {}
    for pack in args.view or []:
        pack = pathlib.Path(pack)
        try:
            source = view_label_set(pack)
        except (OSError, KeyError, ValueError) as error:
            print(f"{pack}: not a linked pack ({error})", file=sys.stderr)
            return 1
        if source not in {d.resolve() for d in label_dirs}:
            label_dirs.append(source)
        views.setdefault(source, []).append(pack)
    if not label_dirs:
        print("nothing to stage: give --labels or --view", file=sys.stderr)
        return 1
    for labels_dir in label_dirs:
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
        publish = [
            args.velocity,
            "lidar",
            "labels",
            "publish",
            "--labels",
            labels_dir,
            "--dest",
            dest,
        ]
        if not dest.exists():
            run(publish, args.dry_run)
        elif args.replace:
            run(publish + ["--replace"], args.dry_run)
        elif same_head(load_json(dest / "labels.json"), label_manifest):
            print(
                f"{rel} is already published at revision {label_manifest['head']['revision']}"
            )
        else:
            print(
                f"{dest} exists at another head; a published label set is never "
                "overwritten (--replace refreshes one that is behind)",
                file=sys.stderr,
            )
            return 1
        big = [] if args.dry_run else oversized(dest)
        if big:
            for path in big:
                print(
                    f"{path} is {path.stat().st_size} bytes, over the {MAX_PLAIN_GIT_BYTES} Hugging Face "
                    "keeps in plain Git",
                    file=sys.stderr,
                )
            print(
                "split it finer: 'velocity lidar labels relayout --window-chunk N' on the source with a "
                "smaller N, then stage again with --replace",
                file=sys.stderr,
            )
            return 1
        paths = sorted(set((row.get("label_set_paths") or []) + [str(rel)]))
        row["label_set_paths"] = paths
        changed = True
        print(f"staged {labels_dir} as {rel} for {row['capture_id']}")
        for pack in views.get(labels_dir.resolve(), []):
            view_rel = pathlib.PurePosixPath(
                "labels", stem_of(row), name, "views", view_name(pack)
            )
            view_dest = dataset / view_rel
            if view_dest.exists():
                print(
                    f"{view_dest} exists; a published view is never overwritten",
                    file=sys.stderr,
                )
                return 1
            run(
                [
                    args.velocity,
                    "lidar",
                    "labels",
                    "publish-view",
                    "--pack",
                    pack,
                    "--labels",
                    dest,
                    "--dest",
                    view_dest,
                ],
                args.dry_run,
            )
            row["annotation_pack_paths"] = sorted(
                set((row.get("annotation_pack_paths") or []) + [str(view_rel)])
            )
            print(f"staged view {pack} as {view_rel}")
    if changed and not args.dry_run:
        manifest_path.write_text(dump_manifest(rows), encoding="utf-8")
    if drop_lines(dataset / ".gitattributes", OBSOLETE_GITATTRIBUTES, args.dry_run):
        print(
            f"{'would remove' if args.dry_run else 'removed'} the retired LFS rule for label set text"
        )
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
        action="append",
        help="label set directory (repeatable)",
    )
    parser.add_argument(
        "--view",
        action="append",
        help="linked pack to publish into its label set's views/ (repeatable)",
    )
    parser.add_argument(
        "--replace",
        action="store_true",
        help="refresh a published label set that is behind its source",
    )
    parser.add_argument(
        "--capture-id", help="bind to this manifest row's raw file first"
    )
    parser.add_argument(
        "--velocity", default="./velocity", help="velocity binary built by make"
    )
    parser.add_argument("--port", type=int, default=2369, help="LiDAR data UDP port")
    parser.add_argument(
        "--refresh-paths",
        action="store_true",
        help="rewrite every row's label set and view paths from labels/ "
        "(after 'velocity lidar labels nest' moved them), and stage nothing",
    )
    parser.add_argument(
        "--dry-run", action="store_true", help="print what would be done"
    )
    args = parser.parse_args(argv)
    if args.refresh_paths:
        return refresh_paths(args)
    return stage(args)


if __name__ == "__main__":
    sys.exit(main())
