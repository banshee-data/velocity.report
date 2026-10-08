#!/usr/bin/env python3
"""Re-key the LiDAR capture index after a capture volume is reorganised.

The capture index derives every identifier from paths: a root's ID from its path,
a file's from (root, relative path), a session's from (root, first file), a
motion period's from (session, ordinal). Moving the volume's sources into
pcaps/ and pointing the server there would otherwise make a new, empty root and
re-read every capture to probe it.

This script reads a plan.tsv from plan-lidar-volume-reorg.py and writes, under
the new root, what the server would have derived itself:

  - file rows carrying their probe results, so the first scan finds no drift
    and probes nothing;
  - sessions, derived exactly as internal/lidar/capindex.Sessions does;
  - motion periods, motion-pass jobs, labels and replay-case links moved onto
    sessions whose membership the move did not change;
  - replay-case capture paths, which are relative to --lidar-pcap-dir.

The old root's rows are left as they are. The server marks it disabled once it
is no longer configured, which keeps the provenance of what was indexed there.

Before writing anything it checks that its identifier hashes reproduce every
stored ID, that its session derivation reproduces every stored session, and
that each re-keyed file has the indexed size, modification time and content
tag. Any mismatch stops it.

It always runs the whole transaction. Without --apply it rolls back, so a dry
run also proves the writes apply cleanly. Run it against a copy of the
database (cp -c on APFS) with the server stopped, inspect the copy, then swap
it in.
"""

import argparse
import hashlib
import os
import posixpath
import sqlite3
import subprocess
import sys
import time
from collections import Counter

# internal/lidar/capseq: DefaultSeamlessTolerance, DefaultMaxGap, DefaultMaxOverlap.
SEAMLESS_NS = 10_000_000
MAX_GAP_NS = 1_000_000_000
MAX_OVERLAP_NS = 10_000_000
SEVERITY = {"seamless": 0, "acceptable": 1, "broken": 2, "overlap": 3}

# internal/lidar/capindex.TagChunkBytes.
TAG_CHUNK = 1 << 20


# Identifier derivations, mirroring internal/lidar/storage/sqlite.
def root_id(path):
    return "root-" + hashlib.sha256(path.encode()).hexdigest()[:16]


def file_id(root, rel):
    return "cap-" + hashlib.sha256(f"{root}\x00{rel}".encode()).hexdigest()[:24]


def session_id(root, first_rel):
    return (
        "ses-"
        + hashlib.sha256(f"{root}\x00session\x00{first_rel}".encode()).hexdigest()[:20]
    )


def period_id(session, ordinal):
    return (
        "per-"
        + hashlib.sha256(f"{session}\x00period\x00{ordinal}".encode()).hexdigest()[:20]
    )


def content_tag(path, size):
    """internal/lidar/capindex.ContentTag: length, first MiB, and last MiB when distinct."""
    h = hashlib.sha256(f"{size}\n".encode())
    with open(path, "rb") as f:
        h.update(f.read(min(TAG_CHUNK, size)))
        if size > 2 * TAG_CHUNK:
            f.seek(size - TAG_CHUNK)
            h.update(f.read())
    return h.hexdigest()


def grade_gap(gap):
    if gap < -MAX_OVERLAP_NS:
        return "overlap"
    if gap <= SEAMLESS_NS:
        return "seamless"
    if gap <= MAX_GAP_NS:
        return "acceptable"
    return "broken"


def derive_sessions(files, per_dir):
    """capindex.Sessions over probed files: dicts with rel, first, last, size.

    per_dir mirrors the derivation that sequences each directory on its own
    (claude/web-ui-coherence-fc5dd5, commit f4f26273a); without it, every file
    in the root is sequenced together, as main does. Session identity depends
    only on each run's first file, so the order of the returned list does not
    matter here.
    """
    if not per_dir:
        return sequence(files)
    by_dir = {}
    for f in files:
        by_dir.setdefault(posixpath.dirname(f["rel"]), []).append(f)
    return [s for d in sorted(by_dir) for s in sequence(by_dir[d])]


def sequence(files):
    usable = sorted(
        (f for f in files if f["last"] >= f["first"]),
        key=lambda f: (f["first"], f["rel"]),
    )
    runs, run = [], []
    for f in usable:
        if run and grade_gap(f["first"] - run[-1]["last"]) not in (
            "seamless",
            "acceptable",
        ):
            runs.append(run)
            run = []
        run.append(f)
    if run:
        runs.append(run)

    sessions = []
    for run in runs:
        # capseq.Build re-sorts on the same key, so the run order stands.
        start, end, covered, lost, worst = (
            run[0]["first"],
            run[0]["last"],
            0,
            0,
            "seamless",
        )
        for i, f in enumerate(run):
            covered += f["last"] - f["first"]
            end = max(end, f["last"])
            if i:
                gap = f["first"] - run[i - 1]["last"]
                lost += max(gap, 0)
                grade = grade_gap(gap)
                if SEVERITY[grade] > SEVERITY[worst]:
                    worst = grade
        sessions.append(
            dict(
                files=[f["rel"] for f in run],
                start=start,
                end=end,
                covered=covered,
                lost=lost,
                worst=worst,
                size=sum(f["size"] for f in run),
            )
        )
    return sessions


def load_plan(path):
    ops = []
    with open(path) as f:
        header = f.readline().rstrip("\n").split("\t")
        for line in f:
            row = dict(zip(header, line.rstrip("\n").split("\t")))
            if row["op"] in ("mv", "rm-symlink"):
                ops.append((row["op"], row["source"], row["destination"]))
    return ops


def relocate(rel, ops, by_path=False):
    """Where something relative to the old root ends up after the plan, or None if removed.

    An index row follows its file: a removed symlink takes its row with it. A
    stored path only needs to keep resolving, so with by_path a symlink that a
    later move replaces with the real file (s2/s2-sf-0.pcapng) leaves it valid.
    """
    for i, (op, src, dst) in enumerate(ops):
        if rel == src or rel.startswith(src + "/"):
            if op == "rm-symlink":
                if by_path and any(o == "mv" and d == rel for o, _, d in ops[i + 1 :]):
                    continue
                return None
            rel = dst + rel[len(src) :]
    return rel


def held_open(path):
    try:
        res = subprocess.run(
            ["lsof", "-nP", "--", str(path)], capture_output=True, text=True, timeout=60
        )
    except (OSError, subprocess.TimeoutExpired):
        return None
    return [line for line in res.stdout.splitlines()[1:] if line.strip()]


class Abort(Exception):
    pass


def check_identities(cur, old_root, old_rid):
    """The hashes here must reproduce what the store wrote, or nothing else can be trusted."""
    if root_id(old_root) != old_rid:
        raise Abort(f"root_id({old_root}) = {root_id(old_root)}, stored {old_rid}")
    bad = [
        r
        for r in cur.execute(
            "SELECT capture_file_id, root_id, rel_path FROM lidar_capture_files"
        )
        if file_id(r[1], r[2]) != r[0]
    ]
    if bad:
        raise Abort(
            f"{len(bad)} capture_file_id values do not reproduce, e.g. {bad[0]}"
        )
    bad = [
        r
        for r in cur.execute(
            "SELECT period_id, session_id, ordinal FROM lidar_capture_motion_periods"
        )
        if period_id(r[1], r[2]) != r[0]
    ]
    if bad:
        raise Abort(f"{len(bad)} period_id values do not reproduce, e.g. {bad[0]}")


def probed_files(cur, rid):
    rows = cur.execute(
        """
        SELECT rel_path, first_packet_ns, last_packet_ns, size_bytes
          FROM lidar_capture_files
         WHERE root_id = ? AND present = 1 AND probe_state = 'ok'
               AND first_packet_ns IS NOT NULL AND last_packet_ns IS NOT NULL""",
        (rid,),
    )
    return [dict(rel=r[0], first=r[1], last=r[2], size=r[3]) for r in rows]


def check_derivation(cur, rid):
    """Find the derivation that reproduces the stored sessions exactly, and return it.

    The stored sessions were written by whichever server build last scanned
    the root; the new root's are derived the same way, so the server's first
    scan after the move finds them unchanged.
    """
    failures = {}
    for per_dir in (True, False):
        problems = session_mismatches(cur, rid, per_dir)
        if not problems:
            return (
                per_dir,
                cur.execute(
                    "SELECT COUNT(*) FROM lidar_capture_sessions WHERE root_id = ?",
                    (rid,),
                ).fetchone()[0],
            )
        failures["per-directory" if per_dir else "whole-root"] = problems
    raise Abort(
        "neither derivation reproduces the stored sessions (probes may have changed since the "
        "last scan; scan once with the server build you will run, then retry):\n"
        + "\n".join(f"  {mode}: " + "; ".join(p[:3]) for mode, p in failures.items())
    )


def session_mismatches(cur, rid, per_dir):
    derived = {
        session_id(rid, s["files"][0]): s
        for s in derive_sessions(probed_files(cur, rid), per_dir)
    }
    stored = {
        r[0]: r
        for r in cur.execute(
            """
        SELECT session_id, file_count, start_ns, end_ns, covered_ns, lost_ns, worst_seam, size_bytes
          FROM lidar_capture_sessions WHERE root_id = ?""",
            (rid,),
        )
    }
    members = {}
    for sid, rel in cur.execute(
        "SELECT session_id, rel_path FROM lidar_capture_files "
        "WHERE root_id = ? AND session_id IS NOT NULL",
        (rid,),
    ):
        members.setdefault(sid, set()).add(rel)
    problems = []
    if set(derived) != set(stored):
        problems.append(
            f"session IDs differ: {len(set(derived) - set(stored))} only derived, "
            f"{len(set(stored) - set(derived))} only stored"
        )
    for sid in set(derived) & set(stored):
        d, s = derived[sid], stored[sid]
        want = (
            len(d["files"]),
            d["start"],
            d["end"],
            d["covered"],
            d["lost"],
            d["worst"],
            d["size"],
        )
        if tuple(s[1:]) != want:
            problems.append(f"{sid}: stored {tuple(s[1:])} derived {want}")
        if members.get(sid, set()) != set(d["files"]):
            problems.append(f"{sid}: membership differs")
    return problems


def main():
    ap = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter
    )
    ap.add_argument(
        "--db", required=True, help="a copy of sensor_data.db, never the live file"
    )
    ap.add_argument(
        "--plan", required=True, help="plan.tsv from plan-lidar-volume-reorg.py"
    )
    ap.add_argument("--old-root", default="/Volumes/lidar/lidar")
    ap.add_argument("--new-root", help="default: <old-root>/pcaps")
    ap.add_argument(
        "--apply",
        action="store_true",
        help="commit; without it the transaction is rolled back",
    )
    ap.add_argument(
        "--skip-tags",
        action="store_true",
        help="check size and mtime only, not content tags",
    )
    args = ap.parse_args()

    # Go's filepath.Abs: absolute and cleaned, symlinks not resolved.
    old_root = os.path.abspath(args.old_root)
    new_root = os.path.abspath(args.new_root or os.path.join(old_root, "pcaps"))
    if not new_root.startswith(old_root + "/"):
        sys.exit("--new-root must lie inside --old-root")
    new_prefix = new_root[len(old_root) + 1 :] + "/"
    ops = load_plan(args.plan)
    moved = os.path.isdir(new_root)

    held = held_open(args.db)
    if held:
        sys.exit(
            f"{args.db} is open in another process; stop it or work on a copy:\n  "
            + "\n  ".join(held[:3])
        )
    if held is None and args.apply:
        sys.exit(
            "cannot check whether the database is open (lsof unavailable); refusing --apply"
        )

    con = sqlite3.connect(args.db, isolation_level=None)
    con.execute("PRAGMA foreign_keys = ON")  # no effect once a transaction has begun
    cur = con.cursor()
    now = time.time_ns()

    try:
        row = cur.execute(
            "SELECT root_id, last_scan_at_ns, last_scan_state FROM lidar_capture_roots "
            "WHERE path = ?",
            (old_root,),
        ).fetchone()
        if not row:
            raise Abort(f"no capture root with path {old_root}")
        old_rid, last_scan_at, last_state = row
        new_rid = root_id(new_root)
        if cur.execute(
            "SELECT 1 FROM lidar_capture_roots WHERE root_id = ? OR path = ?",
            (new_rid, new_root),
        ).fetchone():
            raise Abort(f"root {new_root} is already indexed; nothing to re-key into")

        check_identities(cur, old_root, old_rid)
        per_dir, n_stored = check_derivation(cur, old_rid)
        mode = "per-directory" if per_dir else "whole-root"
        print(
            f"identities and {n_stored} stored sessions reproduce ({mode} derivation)"
        )

        # Map each old file into the new root.
        cols = [r[1] for r in cur.execute("PRAGMA table_info(lidar_capture_files)")]
        old_files = [
            dict(zip(cols, r))
            for r in cur.execute(
                "SELECT * FROM lidar_capture_files WHERE root_id = ? ORDER BY rel_path",
                (old_rid,),
            )
        ]
        carried, left, dropped = [], Counter(), 0
        for f in old_files:
            dest = relocate(f["rel_path"], ops)
            if dest is None:
                dropped += 1
            elif dest.startswith(new_prefix):
                carried.append((f, dest[len(new_prefix) :]))
            else:
                left[dest.split("/")[0] + "/"] += 1
        if len({rel for _, rel in carried}) != len(carried):
            raise Abort("two indexed files map to the same new path")

        # The files must be where the plan says, unchanged. Before the move, check them in place.
        mismatches = []
        for f, rel in carried:
            path = (
                os.path.join(new_root, rel)
                if moved
                else os.path.join(old_root, f["rel_path"])
            )
            try:
                st = os.stat(path)
            except OSError as e:
                mismatches.append(f"{path}: {e.strerror}")
                continue
            if st.st_size != f["size_bytes"] or st.st_mtime_ns != f["modified_at_ns"]:
                mismatches.append(
                    f"{path}: size/mtime {st.st_size}/{st.st_mtime_ns} "
                    f"indexed {f['size_bytes']}/{f['modified_at_ns']}"
                )
            elif (
                not args.skip_tags
                and f["content_tag"]
                and content_tag(path, st.st_size) != f["content_tag"]
            ):
                mismatches.append(f"{path}: content tag differs from the index")
        if mismatches:
            raise Abort(
                f"{len(mismatches)} files differ from the index:\n  "
                + "\n  ".join(mismatches[:10])
            )
        print(
            f"{len(carried)} files verified {'at their new paths' if moved else 'in place (pre-move)'}"
        )

        con.execute("BEGIN IMMEDIATE")
        cur.execute(
            """INSERT INTO lidar_capture_roots
                         (root_id, path, label, enabled, last_scan_at_ns, last_scan_state,
                          last_scan_error, created_at_ns, updated_at_ns)
                       VALUES (?, ?, '', 1, ?, ?, '', ?, ?)""",
            (new_rid, new_root, last_scan_at, last_state, now, now),
        )

        fid_map = {}
        keep = [
            c
            for c in cols
            if c not in ("capture_file_id", "root_id", "rel_path", "session_id")
        ]
        for f, rel in carried:
            nid = file_id(new_rid, rel)
            fid_map[f["capture_file_id"]] = nid
            cur.execute(
                f"""INSERT INTO lidar_capture_files
                              (capture_file_id, root_id, rel_path, {", ".join(keep)})
                            VALUES (?, ?, ?, {", ".join("?" * len(keep))})""",
                [nid, new_rid, rel] + [f[c] for c in keep],
            )

        # Sessions, exactly as the server will re-derive them on its first scan.
        old_session_of = {f["rel_path"]: f["session_id"] for f, _ in carried}
        new_to_old_rel = {rel: f["rel_path"] for f, rel in carried}
        old_members = Counter(f["session_id"] for f in old_files if f["session_id"])
        old_sessions = {
            r[0]: r
            for r in cur.execute(
                "SELECT session_id, label, sensor_id FROM lidar_capture_sessions WHERE root_id = ?",
                (old_rid,),
            )
        }
        session_map, fresh = {}, 0
        for s in derive_sessions(probed_files(cur, new_rid), per_dir):
            sid = session_id(new_rid, s["files"][0])
            olds = {old_session_of[new_to_old_rel[rel]] for rel in s["files"]}
            same = (
                len(olds) == 1
                and None not in olds
                and old_members[next(iter(olds))] == len(s["files"])
            )
            label, sensor = ("", "")
            if same:
                old_sid = next(iter(olds))
                session_map[old_sid] = sid
                label, sensor = old_sessions[old_sid][1], old_sessions[old_sid][2]
            else:
                fresh += 1
            cur.execute(
                """INSERT INTO lidar_capture_sessions
                             (session_id, root_id, label, sensor_id, file_count, start_ns, end_ns,
                              covered_ns, lost_ns, worst_seam, size_bytes, derived_at_ns)
                           VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)""",
                (
                    sid,
                    new_rid,
                    label,
                    sensor,
                    len(s["files"]),
                    s["start"],
                    s["end"],
                    s["covered"],
                    s["lost"],
                    s["worst"],
                    s["size"],
                    now,
                ),
            )
            for rel in s["files"]:
                cur.execute(
                    "UPDATE lidar_capture_files SET session_id = ? WHERE capture_file_id = ?",
                    (sid, file_id(new_rid, rel)),
                )

        # Motion periods and the jobs that produced them follow an unchanged session.
        period_map = {}
        for old_sid, new_sid in session_map.items():
            for pid, ordinal in cur.execute(
                "SELECT period_id, ordinal FROM lidar_capture_motion_periods "
                "WHERE session_id = ?",
                (old_sid,),
            ).fetchall():
                period_map[pid] = period_id(new_sid, ordinal)
                cur.execute(
                    "UPDATE lidar_capture_motion_periods SET period_id = ?, session_id = ? "
                    "WHERE period_id = ?",
                    (period_map[pid], new_sid, pid),
                )
            cur.execute(
                "UPDATE lidar_capture_jobs SET session_id = ?, root_id = ? "
                "WHERE session_id = ? AND kind = 'motion_pass'",
                (new_sid, new_rid, old_sid),
            )
        stranded = cur.execute(
            """SELECT COUNT(*) FROM lidar_capture_motion_periods p
                                    JOIN lidar_capture_sessions s USING (session_id)
                                   WHERE s.root_id = ?""",
            (old_rid,),
        ).fetchone()[0]

        # Replay cases: links to sessions, periods and files, and capture paths, which are
        # relative to --lidar-pcap-dir (the old root before, the new root after).
        case_links = 0
        for old_sid, new_sid in session_map.items():
            case_links += cur.execute(
                "UPDATE lidar_replay_cases SET session_id = ? WHERE session_id = ?",
                (new_sid, old_sid),
            ).rowcount
        for old_pid, new_pid in period_map.items():
            case_links += cur.execute(
                "UPDATE lidar_replay_cases SET source_period_id = ? "
                "WHERE source_period_id = ?",
                (new_pid, old_pid),
            ).rowcount
        for old_fid, new_fid in fid_map.items():
            case_links += cur.execute(
                "UPDATE lidar_replay_case_files SET capture_file_id = ? "
                "WHERE capture_file_id = ?",
                (new_fid, old_fid),
            ).rowcount
        dangling = cur.execute(
            """SELECT COUNT(*) FROM lidar_replay_cases WHERE source_period_id IS NOT NULL
                                     AND source_period_id NOT IN (SELECT period_id FROM lidar_capture_motion_periods)
                               """
        ).fetchone()[0]

        repathed, unreachable, missing = 0, [], []
        for table in ("lidar_replay_cases", "lidar_replay_case_files"):
            for (p,) in cur.execute(
                f"SELECT DISTINCT pcap_file FROM {table} WHERE pcap_file <> ''"
            ).fetchall():
                if os.path.isabs(p):
                    continue
                dest = relocate(p, ops, by_path=True)
                if dest is None or not dest.startswith(new_prefix):
                    # Look where the file is now, before or after the move.
                    if os.path.exists(os.path.join(old_root, p)) or (
                        dest and os.path.exists(os.path.join(old_root, dest))
                    ):
                        unreachable.append(f"{table}: {p} -> {dest}")
                    else:
                        missing.append(f"{table}: {p}")
                    continue
                if dest[len(new_prefix) :] != p:
                    repathed += cur.execute(
                        f"UPDATE {table} SET pcap_file = ? WHERE pcap_file = ?",
                        (dest[len(new_prefix) :], p),
                    ).rowcount

        violations = cur.execute("PRAGMA foreign_key_check").fetchall()
        if violations:
            raise Abort(f"foreign key check failed: {violations[:5]}")
        if args.apply:
            con.execute("COMMIT")
        else:
            con.execute("ROLLBACK")
    except Abort as e:
        if con.in_transaction:
            con.execute("ROLLBACK")
        sys.exit(f"stopped, nothing written: {e}")

    print(
        f"""
old root  {old_root}  ({old_rid})
new root  {new_root}  ({new_rid})

files     {len(carried)} re-keyed with their probes, {dropped} dropped by the plan (symlinks)"""
    )
    for where, n in sorted(left.items()):
        print(f"          {n} left behind, moved to {where}")
    print(
        f"""sessions  {len(session_map) + fresh} derived: {len(session_map)} unchanged, {fresh} new
          (new ones need a motion pass; their old periods stay on the retired root: {stranded})
periods   {len(period_map)} moved onto unchanged sessions
cases     {case_links} session/period/file links updated, {repathed} capture paths re-pathed
          {dangling} cases point at a period that no longer exists (already the case before this)"""
    )
    if unreachable:
        print("          capture paths that leave the pcap dir, left unchanged:")
        for line in unreachable:
            print(f"            {line}")
    if missing:
        print(
            f"          {len(missing)} capture paths already missing, left unchanged:"
        )
        for line in missing:
            print(f"            {line}")
    print()
    print(
        "committed."
        if args.apply
        else "dry run: the transaction ran and was rolled back."
    )
    if not moved:
        print(
            "the volume has not been moved yet: re-run after apply.sh so files are checked at their new paths."
        )


if __name__ == "__main__":
    main()
