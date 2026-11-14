#!/usr/bin/env python3
"""Linux worker inventory and verified NAS archival. No implicit deletion."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import stat
import subprocess
import time
import uuid

RECEIPT = ".worker-archive.json"


def run(*args):
    return subprocess.check_output(args, text=True, stderr=subprocess.PIPE).strip()


def inside(path, root):
    return path == root or root in path.parents


def mount(path):
    # Trigger a systemd automount with a read, then ignore its autofs wrapper.
    with os.scandir(path) as entries:
        next(entries, None)
    data = json.loads(
        run("findmnt", "-J", "-T", str(path), "-o", "TARGET,SOURCE,FSTYPE,OPTIONS")
    )
    filesystems = [
        entry for entry in data["filesystems"] if entry["fstype"] != "autofs"
    ]
    if len(filesystems) != 1:
        raise ValueError(f"ambiguous or unavailable filesystem: {data}")
    return filesystems[0]


def check_nas(path, expected):
    entry = mount(path)
    options = entry["options"].split(",")
    if entry["fstype"] not in ("nfs", "nfs4") or entry["source"] != expected:
        raise ValueError(f"unexpected archive filesystem: {entry}")
    if "rw" not in options or "hard" not in options:
        raise ValueError("archive requires a writable hard NFS mount")


def physical_inside(target, root):
    # /home/banshee is a bind alias of /srv/banshee on arrow-worker.
    # Compare ancestor inodes as well as names so open files through either
    # spelling keep the source busy.
    if inside(target, root):
        return True
    identity = root.stat()
    for parent in [target] + list(target.parents):
        try:
            info = parent.stat()
        except FileNotFoundError:
            continue
        if (info.st_dev, info.st_ino) == (identity.st_dev, identity.st_ino):
            return True
    return False


def quiet(root):
    """Require complete Linux process visibility; fail closed on inaccessible PIDs."""
    if os.geteuid() != 0:
        raise ValueError("copy/removal needs sudo for complete /proc activity checks")
    for pid in Path("/proc").iterdir():
        if not pid.name.isdigit() or int(pid.name) == os.getpid():
            continue
        try:
            links = [pid / "cwd", pid / "root"] + list((pid / "fd").iterdir())
            for link in links:
                try:
                    target = os.readlink(link).removesuffix(" (deleted)")
                except FileNotFoundError:
                    continue
                if target.startswith("/") and physical_inside(Path(target), root):
                    raise ValueError(f"in use by PID {pid.name}: {target}")
            # Include mapped files whose original descriptor has been closed.
            for line in (pid / "maps").read_text().splitlines():
                parts = line.split(maxsplit=5)
                if len(parts) == 6 and parts[5].startswith("/"):
                    if physical_inside(Path(parts[5].removesuffix(" (deleted)")), root):
                        raise ValueError(f"mapped by PID {pid.name}")
        except (FileNotFoundError, ProcessLookupError):
            continue


def digest(path):
    before = path.stat(follow_symlinks=False)
    with path.open("rb") as stream:
        value = hashlib.file_digest(stream, "sha256").hexdigest()
    after = path.stat(follow_symlinks=False)
    if (
        before.st_dev,
        before.st_ino,
        before.st_size,
        before.st_mtime_ns,
        before.st_ctime_ns,
    ) != (
        after.st_dev,
        after.st_ino,
        after.st_size,
        after.st_mtime_ns,
        after.st_ctime_ns,
    ):
        raise ValueError(f"changed while hashing: {path}")
    return value


def raise_walk_error(error):
    raise error


def nested_mount(point, root):
    return not point.samefile(root) and physical_inside(point, root)


def snapshot(root, skip_receipt=False):
    """No links, special files, repositories or nested mounts in archived data."""
    result = {}
    device = root.stat().st_dev
    # Bind mounts can share st_dev; inspect mount points as well.
    mountinfo = Path("/proc/self/mountinfo")
    if mountinfo.exists():
        for line in mountinfo.read_text().splitlines():
            point = Path(line.split()[4].replace(r"\040", " ").replace(r"\134", "\\"))
            if nested_mount(point, root):
                raise ValueError(f"nested mount: {point}")
    for directory, dirs, files in os.walk(
        root, followlinks=False, onerror=raise_walk_error
    ):
        for name in sorted(dirs + files):
            path = Path(directory) / name
            rel = str(path.relative_to(root))
            if skip_receipt and rel == RECEIPT:
                continue
            if name == ".git" or rel == RECEIPT:
                raise ValueError(f"repository or reserved receipt in source: {path}")
            info = path.lstat()
            if info.st_dev != device:
                raise ValueError(f"nested filesystem: {path}")
            if stat.S_ISDIR(info.st_mode):
                result[rel] = {"type": "directory"}
            elif stat.S_ISREG(info.st_mode):
                result[rel] = {
                    "type": "file",
                    "size": info.st_size,
                    "sha256": digest(path),
                }
            else:
                raise ValueError(f"symlink or special file: {path}")
    return result


def source_path(value):
    path = Path(value).absolute()
    if path != path.resolve() or not path.is_dir() or path == Path("/"):
        raise ValueError("source must be a real directory with no symlink components")
    if mount(path)["fstype"] in ("nfs", "nfs4", "cifs", "smb3"):
        raise ValueError("source must be on local storage")
    # Never archive an entire checkout, even when .git lives in an ancestor.
    probe = subprocess.run(
        [
            "git",
            "-c",
            "safe.directory=*",
            "-C",
            str(path),
            "rev-parse",
            "--git-dir",
        ],
        capture_output=True,
    )
    if probe.returncode == 0:
        raise ValueError("source is inside a Git checkout; retire worktrees separately")
    return path


def archive(args):
    source = source_path(args.source)
    parent = Path(args.destination).resolve(strict=True)
    if inside(parent, source) or inside(source, parent):
        raise ValueError("source and destination overlap")
    check_nas(parent, args.nas_source)
    if not args.apply:
        print(f"PLAN copy, hash-verify and retain original: {source} -> {parent}")
        return
    quiet(source)
    before = snapshot(source)
    target = parent / (
        source.name
        + "-"
        + time.strftime("%Y%m%dT%H%M%SZ", time.gmtime())
        + "-"
        + uuid.uuid4().hex[:8]
    )
    target.mkdir(mode=0o750)
    # NFS export may refuse O_EXCL, chmod and chown. New private directory;
    # copy bytes without attempting to preserve unsupported metadata.
    for rel, record in before.items():
        dest = target / rel
        if record["type"] == "directory":
            dest.mkdir(parents=True, exist_ok=True)
        else:
            dest.parent.mkdir(parents=True, exist_ok=True)
            with (source / rel).open("rb") as src, dest.open("wb") as out:
                shutil.copyfileobj(src, out)
                out.flush()
                os.fsync(out.fileno())
    quiet(source)
    if snapshot(source) != before or snapshot(target) != before:
        raise ValueError(
            f"verification failed; keep source and inspect partial archive {target}"
        )
    receipt = {
        "source": str(source),
        "destination": str(target),
        "nas_source": args.nas_source,
        "verified_utc": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "entries": before,
    }
    with (target / RECEIPT).open("w") as out:
        json.dump(receipt, out, indent=2)
        out.flush()
        os.fsync(out.fileno())
    print(f"VERIFIED: {target}; original retained. Receipt: {target / RECEIPT}")


def remove_verified(args):
    receipt_path = Path(args.receipt).resolve(strict=True)
    receipt = json.loads(receipt_path.read_text())
    source = source_path(receipt["source"])
    target = Path(receipt["destination"]).resolve(strict=True)
    if (
        receipt_path != target / RECEIPT
        or inside(target, source)
        or inside(source, target)
    ):
        raise ValueError("receipt location or archive overlap is invalid")
    check_nas(target, args.nas_source)
    if receipt["nas_source"] != args.nas_source:
        raise ValueError("receipt names another NAS export")
    quiet(source)
    if (
        snapshot(source) != receipt["entries"]
        or snapshot(target, True) != receipt["entries"]
    ):
        raise ValueError("source or archive changed: original retained")
    print(f"VERIFIED removal {'requested' if args.apply else 'plan'}: {source}")
    if args.apply:
        quiet(source)
        shutil.rmtree(source)


def git_at(path, *args):
    return run("git", "-c", f"safe.directory={path}", "-C", str(path), *args)


def worktrees(args):
    repo = Path(args.repo).resolve(strict=True)
    records = git_at(repo, "worktree", "list", "--porcelain").split("\n\n")
    for record in records[1:]:
        path = Path(record.splitlines()[0].removeprefix("worktree "))
        try:
            if "\nlocked" in record or "\nprunable" in record:
                raise ValueError("locked or missing worktree")
            path = path.resolve(strict=True)
            quiet(path)
            if git_at(
                path, "status", "--porcelain", "--untracked-files=all", "--ignored"
            ):
                raise ValueError("modified, untracked or ignored files")
            git_at(path, "merge-base", "--is-ancestor", "HEAD", "origin/main")
            newest = max(p.lstat().st_mtime for p in [path] + list(path.rglob("*")))
            if time.time() - newest < args.days * 86400:
                raise ValueError(f"changed within {args.days} days")
            print(f"{'REMOVE' if args.apply else 'PLAN remove'} {path}")
            if args.apply:
                # Git rechecks dirt and locks; never force or delete branches.
                git_at(repo, "worktree", "remove", str(path))
        except (ValueError, OSError, subprocess.CalledProcessError) as err:
            print(f"KEEP {path}: {err}")


def scan(args):
    root = Path(args.root).resolve(strict=True)
    print(run("df", "-h", "/", "/var", str(root), "/mnt/results"))
    print(run("findmnt", "-t", "nfs,nfs4", "-o", "TARGET,SOURCE,OPTIONS"))
    print(run("du", "-x", "-h", "--max-depth=1", str(root)))
    print(
        "Review evidence/captures for archival; caches and worktrees need separate review."
    )


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    survey = commands.add_parser("scan")
    survey.add_argument("--root", default="/srv/banshee")
    survey.set_defaults(function=scan)
    for name, function in (
        ("archive", archive),
        ("remove-verified", remove_verified),
        ("worktrees", worktrees),
    ):
        cmd = commands.add_parser(name)
        cmd.add_argument(
            "--apply",
            action="store_true",
            help="perform this operation; default is a plan",
        )
        cmd.set_defaults(function=function)
        if name == "worktrees":
            cmd.add_argument("--repo", required=True)
            cmd.add_argument("--days", type=int, default=14)
        else:
            cmd.add_argument(
                "--nas-source", default="arrow-local:/mnt/arrow/banshee/results"
            )
            if name == "archive":
                cmd.add_argument("source")
                cmd.add_argument(
                    "destination", help="existing directory on the expected NAS export"
                )
            else:
                cmd.add_argument("receipt")
    args = parser.parse_args()
    if getattr(args, "days", 1) < 1:
        parser.error("--days must be positive")
    try:
        args.function(args)
    except (ValueError, OSError, subprocess.CalledProcessError) as err:
        parser.exit(1, f"REFUSED: {err}\n")


if __name__ == "__main__":
    main()
