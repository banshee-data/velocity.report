#!/usr/bin/env python3
"""Dry-run planner for reorganising a LiDAR capture volume.

Reads the volume and writes a plan; it never moves, renames or deletes anything.

    pcaps/      what the pipeline reads: source captures, and the published
                sf-street-speeds corpus that scene publishing replays
    work/       derived output, experiments, scratch: never scanned
    manifests/  hand-authored provenance, left where it is

pcaps/ is the new LIDAR_PCAP_DIR, and the s2-archive tools treat it as their
archive root. So s2/ keeps its analysis/, analysis-continuous/ and
static-huggingface/ beside the captures, where those tools read them (the
scanner already skips analysis output), and sf-street-speeds/ sits under it
because the server refuses to replay anything outside LIDAR_PCAP_DIR.

Outputs, in --out:
    plan.tsv         one row per step: op, source, destination, bytes, category, note
    apply.sh         the same steps as shell, for review; this script never runs it
    repo-refs.txt    repository lines that name a path this plan moves

apply.sh can be run again after a failure: a step already done is skipped, and
a step that would overwrite something stops it.

Relative paths under pcaps/s2/ are unchanged, so provenance manifests and replay
cases that name `s2/...` stay valid once --lidar-pcap-dir points at pcaps/. The
capture index and the replay cases that name moved files are carried across by
rekey-lidar-capture-index.py, which reads plan.tsv.
"""

import argparse
import os
import re
import shlex
import subprocess
import sys
from pathlib import Path

# Mirrors internal/lidar/capindex/scan.go, so "before" and "after" counts are
# what the captures view would index.
CAPTURE_EXT = {".pcap", ".pcapng"}
EXCLUDED_DIRS = {"analysis", "segments", "vrlog", "plots"}


def scanner_skips(name):
    return (
        name.startswith(".")
        or name in EXCLUDED_DIRS
        or name.startswith("pcap_split_analysis_")
    )


def scan_count(root, skip_names=()):
    """Capture files and bytes the scanner would index under root."""
    root = Path(root)
    if not root.is_dir():
        paths = [root]
    else:
        paths = []
        for dirpath, dirnames, filenames in os.walk(root):
            dirnames[:] = [
                d
                for d in dirnames
                if not scanner_skips(d)
                and not (Path(dirpath) == root and d in skip_names)
            ]
            paths += [Path(dirpath) / f for f in filenames]
    count, size = 0, 0
    for p in paths:
        if p.suffix.lower() not in CAPTURE_EXT:
            continue
        try:
            size += p.stat().st_size  # follows links, as the scanner does
        except OSError:
            continue
        count += 1
    return count, size


def tree_size(path):
    path = Path(path)
    if path.is_symlink():
        return 0
    if path.is_file():
        return path.stat().st_size
    total = 0
    for dirpath, _, filenames in os.walk(path):
        for f in filenames:
            try:
                total += os.lstat(os.path.join(dirpath, f)).st_size
            except OSError:
                pass
    return total


# Top-level capture files, by name, to the group folder under pcaps/.
GROUPS = [
    (re.compile(r"^(kirk\d+|krik-stable-\d+)\.pcapng(\.gz)?$"), "kirk"),
    (re.compile(r"^soma\d+\.pcapng(\.gz)?$"), "soma"),
    (re.compile(r"^morg\d+\.pcapng(\.gz)?$"), "morg"),
    (re.compile(r"^clar(en)?\d+\.pcapng(\.gz)?$"), "clar"),
    (re.compile(r"^broadway_columbus.*\.pcap$"), "broadway-columbus"),
    (re.compile(r"^lidar-(capture-2025-10-08_|chior-break).*"), "2025-10-08-chior"),
    (re.compile(r"^big-lidarr-hesai-2025-09-26.*"), "hesai-2025-09-26"),
]

# Top-level directories, by name pattern, to their new home.
DIR_HOMES = [
    (re.compile(r"^sf-street-speeds$"), "pcaps"),
    (re.compile(r"^(velocity-campaign|seg|vrlog)$"), "work"),
    (re.compile(r"^state-estimation-.*"), "work/state-estimation"),
    (
        re.compile(r"^(pcap_split_analysis_.*|scene-backup-.*|orphaned-scene-.*)$"),
        "work/scratch",
    ),
]

# Top-level files that are not captures.
FILE_HOMES = [
    (re.compile(r"^state-estimation-.*\.log$"), "work/state-estimation"),
]

# Left exactly where they are.
STAY = {
    "manifests",
    "pcaps",
    "work",
    "banshee-data-organization-card-README.md",
}

# Inside s2/, entries the scanner would index that are not source chunks, and
# where they go. Analysis output stays: the scanner skips it, and the archive
# tools read it beside the captures.
S2_EXTRACT = [
    # Same chunk names as s2_sf_7_*, renamed 00-..04-. Verify by hash before deleting.
    (re.compile(r"^embarcadero-folsom-41cfqfsw$"), "work/duplicates-to-verify"),
]


def home_for_file(name):
    for pat, group in GROUPS:
        if pat.match(name):
            return (
                f"pcaps/{group}/compressed/{name}"
                if name.endswith(".gz")
                else f"pcaps/{group}/{name}"
            )
    for pat, home in FILE_HOMES:
        if pat.match(name):
            return f"{home}/{name}"
    return None


def build_plan(root):
    root = Path(root)
    plan = []  # dicts: op, src, dst, bytes, category, note
    top = sorted(p for p in root.iterdir() if not p.name.startswith("."))

    # s2: rename the directory first, then lift the non-source entries out of it.
    s2 = root / "s2"
    # A link in s2/ to a top-level file is replaced by that file, keeping the s2/ path.
    linked_in = {}
    if s2.is_dir():
        extracted = {
            c.name
            for c in s2.iterdir()
            if any(pat.match(c.name) for pat, _ in S2_EXTRACT)
        }
        plan.append(
            dict(
                op="mv",
                src="s2",
                dst="pcaps/s2",
                bytes=scan_count(s2, extracted)[1],
                category="source",
                note="source chunks; relative paths under s2/ unchanged",
            )
        )
        for child in sorted(s2.iterdir()):
            if child.is_symlink():
                target = Path(os.path.realpath(child))
                if target.parent == root and target.is_file():
                    linked_in[target.name] = child.name
                continue
            for pat, dest_dir in S2_EXTRACT:
                if pat.match(child.name):
                    note = ""
                    if "duplicates" in dest_dir:
                        note = "looks like renamed copies of s2_sf_7_* chunks; verify by hash, do not delete blind"
                    plan.append(
                        dict(
                            op="mv",
                            src=f"pcaps/s2/{child.name}",
                            dst=f"{dest_dir}/{child.name}",
                            bytes=tree_size(child),
                            category="derived",
                            note=note,
                        )
                    )
                    break

    for p in top:
        name = p.name
        if name in STAY or name == "s2":
            continue
        if p.is_file() or p.is_symlink():
            if name in linked_in and not p.is_symlink():
                link = linked_in[name]
                plan.append(
                    dict(
                        op="rm-symlink",
                        src=f"pcaps/s2/{link}",
                        dst="",
                        bytes=0,
                        category="cleanup",
                        note=f"symlink to ../{name}; replaced by the real file",
                    )
                )
                plan.append(
                    dict(
                        op="mv",
                        src=name,
                        dst=f"pcaps/s2/{link}",
                        bytes=p.stat().st_size,
                        category="source",
                        note=f"was linked into s2/; keeps the s2/{link} path",
                    )
                )
                continue
            dst = home_for_file(name)
            if dst is None:
                plan.append(
                    dict(
                        op="skip",
                        src=name,
                        dst="",
                        bytes=p.stat().st_size,
                        category="unclassified",
                        note="no rule; decide by hand",
                    )
                )
                continue
            note = ""
            if not dst.startswith("pcaps/"):
                plan.append(
                    dict(
                        op="mv",
                        src=name,
                        dst=dst,
                        bytes=p.stat().st_size,
                        category="derived",
                        note="",
                    )
                )
                continue
            if name.endswith(".gz"):
                note = "compressed; the scanner does not index .gz"
                twin = root / name[:-3]
                if twin.exists():
                    note += (
                        f"; uncompressed twin {twin.name} exists, candidate for removal"
                    )
            plan.append(
                dict(
                    op="mv",
                    src=name,
                    dst=dst,
                    bytes=p.stat().st_size,
                    category="source-compressed" if name.endswith(".gz") else "source",
                    note=note,
                )
            )
        elif p.is_dir():
            for pat, home in DIR_HOMES:
                if pat.match(name):
                    plan.append(
                        dict(
                            op="mv",
                            src=name,
                            dst=f"{home}/{name}",
                            bytes=tree_size(p),
                            category=(
                                "derived" if home.startswith("work") else "corpus"
                            ),
                            note=(
                                "published dataset; scene publishing replays it, so it "
                                "stays under LIDAR_PCAP_DIR"
                                if home == "pcaps"
                                else (
                                    "contains pcaps the scanner would otherwise index"
                                    if scan_count(p)[0]
                                    else ""
                                )
                            ),
                        )
                    )
                    break
            else:
                plan.append(
                    dict(
                        op="skip",
                        src=name,
                        dst="",
                        bytes=tree_size(p),
                        category="unclassified",
                        note="no rule; decide by hand",
                    )
                )
    if s2.is_dir():
        # The s2 row's bytes counted links through to their targets, which have rows of their own.
        plan[0]["bytes"] -= sum((root / name).stat().st_size for name in linked_in)
    return plan


def write_outputs(plan, root, out):
    out.mkdir(parents=True, exist_ok=True)
    with open(out / "plan.tsv", "w") as f:
        f.write("op\tsource\tdestination\tbytes\tcategory\tnote\n")
        for e in plan:
            f.write(
                "\t".join(
                    [
                        e["op"],
                        e["src"],
                        e["dst"],
                        str(e["bytes"]),
                        e["category"],
                        e["note"],
                    ]
                )
                + "\n"
            )

    # A directory that is itself a move destination must not exist beforehand, or
    # `mv s2 pcaps/s2` would nest it as pcaps/s2/s2.
    destinations = {e["dst"] for e in plan if e["op"] == "mv" and e["dst"]}
    dirs = sorted(
        ({str(Path(d).parent) for d in destinations} | {"pcaps"}) - destinations
    )
    q = shlex.quote
    with open(out / "apply.sh", "w") as f:
        f.write(f"""#!/bin/sh
# Generated by plan-lidar-volume-reorg.py. Review, then run by hand.
# Stop the server, and anything else reading this volume, first.
# Safe to run again after a failure: finished steps are skipped.
set -eu
cd {q(str(root))}

# mvx SRC DST: move, or skip when already moved; never overwrite.
mvx() {{
  if [ -L "$1" ] && [ "$(readlink "$1")" = "$2" ]; then return 0; fi  # compat link left by a finished run
  if [ ! -e "$1" ] && [ ! -L "$1" ]; then
    if [ -e "$2" ]; then return 0; fi
    echo "missing: $1" >&2; exit 1
  fi
  if [ -e "$2" ] || [ -L "$2" ]; then echo "exists: $2" >&2; exit 1; fi
  mv "$1" "$2"
}}

""")
        for d in dirs:
            f.write(f"mkdir -p {q(d)}\n")
        f.write("\n")
        for e in plan:
            if e["op"] == "mv":
                f.write(f"mvx {q(e['src'])} {q(e['dst'])}\n")
            elif e["op"] == "rm-symlink":
                f.write(f"if [ -L {q(e['src'])} ]; then rm {q(e['src'])}; fi\n")
        f.write(
            "\n# Compatibility: keeps /Volumes/lidar/lidar/s2 paths in older run records and docs\n"
        )
        f.write(
            "# resolvable. It sits outside pcaps/, so a scanner rooted at pcaps/ never sees it.\n"
        )
        f.write("[ -L s2 ] || ln -s pcaps/s2 s2\n")
        f.write(
            "echo 'moved; next: rekey-lidar-capture-index.py on a copy of the database'\n"
        )


def repo_refs(plan, root, repo, out):
    names = sorted(
        {e["src"].split("/")[0] for e in plan if e["op"] in ("mv", "rm-symlink")}
        - {"pcaps"}
    )
    prefix = str(root)
    hits = []
    for name in names:
        needle = f"{prefix}/{name}"
        try:
            res = subprocess.run(
                ["git", "-C", str(repo), "grep", "-nF", needle, "--", ".", ":!*.lock"],
                capture_output=True,
                text=True,
                check=False,
            )
        except OSError:
            continue
        hits += [f"{name}\t{line}" for line in res.stdout.splitlines()]
    (out / "repo-refs.txt").write_text("\n".join(hits) + ("\n" if hits else ""))
    return len(hits)


def open_files(root):
    try:
        res = subprocess.run(
            ["lsof", "-nP"], capture_output=True, text=True, timeout=60
        )
    except (OSError, subprocess.TimeoutExpired):
        return None
    return [line for line in res.stdout.splitlines() if str(root) in line]


def human(n):
    for unit in ("B", "KB", "MB", "GB", "TB"):
        if n < 1024:
            return f"{n:.1f} {unit}" if unit != "B" else f"{n} B"
        n /= 1024
    return f"{n:.1f} PB"


def main():
    ap = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter
    )
    ap.add_argument("--root", default="/Volumes/lidar/lidar")
    ap.add_argument(
        "--out", required=True, help="directory for the plan files (created)"
    )
    ap.add_argument("--repo", default=str(Path(__file__).resolve().parent.parent))
    args = ap.parse_args()

    root = Path(args.root).resolve()
    out = Path(args.out)
    if not root.is_dir():
        sys.exit(f"{root} is not a directory")
    if (root / "pcaps").exists():
        sys.exit(
            f"{root}/pcaps already exists; this planner is for the first reorganisation"
        )

    before_n, before_b = scan_count(root)
    plan = build_plan(root)

    # What the scanner sees afterwards: s2 chunks (minus extracted dirs, minus the symlink) + moved top-level files.
    extracted = {
        Path(e["src"]).name
        for e in plan
        if e["src"].startswith("pcaps/s2/") and e["op"] == "mv" and e["dst"]
    }
    s2_n, s2_b = (
        scan_count(root / "s2", skip_names=extracted)
        if (root / "s2").is_dir()
        else (0, 0)
    )
    # A link replaced by its target is counted once, as the target that moves in.
    for e in plan:
        if e["op"] == "rm-symlink":
            s2_n -= 1
            s2_b -= (root / e["src"][len("pcaps/") :]).stat().st_size
    after_n, after_b = s2_n, s2_b
    for e in plan:
        if (
            e["op"] == "mv"
            and "/" not in e["src"]
            and e["dst"].startswith("pcaps/")
            and e["src"] != "s2"
        ):
            c, b = scan_count(root / e["src"])
            after_n, after_b = after_n + c, after_b + b

    write_outputs(plan, root, out)
    nrefs = repo_refs(plan, root, args.repo, out)

    print(f"root                  {root}")
    print(f"scanner sees today    {before_n} captures, {human(before_b)}")
    print(
        f"scanner sees after    {after_n} captures, {human(after_b)}   (root = {root}/pcaps)"
    )
    print()
    by_cat = {}
    for e in plan:
        by_cat.setdefault(e["category"], [0, 0])
        by_cat[e["category"]][0] += 1
        by_cat[e["category"]][1] += e["bytes"]
    for cat, (n, b) in sorted(by_cat.items()):
        print(f"  {cat:<18} {n:>4} entries  {human(b):>10}")
    skipped = [e for e in plan if e["op"] == "skip"]
    if skipped:
        print("\nunclassified, left alone:")
        for e in skipped:
            print(f"  {e['src']}")
    notes = [
        e
        for e in plan
        if e["note"]
        and e["category"] in ("source-compressed", "derived")
        and ("twin" in e["note"] or "verify" in e["note"])
    ]
    if notes:
        print("\nneeds your decision (not touched):")
        for e in notes:
            print(f"  {e['src']}: {e['note']}")
    print(f"\nrepo lines naming a moved path: {nrefs}  (see repo-refs.txt)")
    held = open_files(root)
    if held is None:
        print("open files on the volume: could not check (lsof unavailable)")
    elif held:
        print(
            f"open files on the volume: {len(held)} held, apply nothing until they close"
        )
        for line in held[:5]:
            print("  " + line[:160])
    else:
        print("open files on the volume: none")
    print(f"\nplan written to {out}; nothing on {root} was changed")
    print(f"""
next, in order:
  1. stop the server; review {out}/apply.sh and run it
  2. sqlite3 sensor_data.db ".backup <copy>" (a bare cp misses the WAL), then
     scripts/rekey-lidar-capture-index.py --db <copy> --plan {out}/plan.tsv
     (dry run first, then --apply), and swap the copy in
  3. set LIDAR_PCAP_DIR = {root}/pcaps in local.mk, and pass -pcap-root there to the
     state-estimation baseline tool
  4. start the server; a Quick scan should report no drift and probe nothing""")


if __name__ == "__main__":
    main()
