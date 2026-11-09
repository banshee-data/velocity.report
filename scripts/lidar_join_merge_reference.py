#!/usr/bin/env python3
"""A merge reference for the motion fragment join that needs no labels (l4clus0011).

Reads the JSONL `lidar-box-eval -join-records` writes, one record per cluster the join built
from two or more DBSCAN components, and optionally the replay's evidence database, and counts
the joins that show a sign of having joined two road users. Definitions are fixed here, before
any arm is replayed, and are the same for every arm:

* M1, oversize: the joined cluster's oriented length or width exceeds the plausible extent for
  the owner's class at the join (CLASS_LIMITS; a class not listed reads as "vehicle").
* M2, two-track: the components are attributed to two or more distinct confirmed tracks (the
  record's `tracks`, from each track's predicted published box; see pipeline.FragmentJoinRecord).
* M3, solid-body length jump: the owner's solid-body believed length at its first row at or
  after the join's frame, within JUMP_WINDOW_S, exceeds its last row before that frame by more
  than JUMP_M. Needs the evidence database and a replay with the solid body.

Also reported, outside any rule: the union of M1 to M3, each per thousand joins, the share of
joined returns outside the owner's predicted box, the same jump on the published box length,
and solid-body rows whose length is not positive. These are signs of a wrong join, not merges
confirmed by a person.

Usage: lidar_join_merge_reference.py JOINS.jsonl [--evidence EVIDENCE.db] [--json OUT]
"""

from __future__ import annotations

import argparse
import json
import sqlite3
import sys

# Plausible (length, width) in metres by the owner's class at the join.
CLASS_LIMITS = {
    "car": (7.0, 3.0),
    "truck": (12.0, 3.0),
    "vehicle": (12.0, 3.0),
    "bus": (19.0, 3.2),
}
JUMP_M = 2.0
JUMP_WINDOW_S = 0.2


def read_records(path):
    records = []
    with open(path, encoding="utf-8") as f:
        for n, line in enumerate(f, 1):
            line = line.strip()
            if not line:
                continue
            try:
                records.append(json.loads(line))
            except json.JSONDecodeError as e:
                raise SystemExit(f"{path}:{n}: {e}")
    return records


def oversize(rec):
    length, width = CLASS_LIMITS.get(
        rec.get("owner_class") or "vehicle", CLASS_LIMITS["vehicle"]
    )
    return rec.get("length_m", 0) > length or rec.get("width_m", 0) > width


def jump(rows, frame_ns, window_ns, threshold):
    """rows: (frame_unix_nanos, length) of one track in frame order. The increase from the last
    row before frame_ns to the first row in [frame_ns, frame_ns + window_ns], or None.
    """
    before = after = None
    for t, length in rows:
        if t < frame_ns:
            before = length
        elif t <= frame_ns + window_ns:
            after = length
            break
        else:
            break
    if before is None or after is None:
        return None
    return after - before


def track_rows(con, table, column, track_ids):
    out = {}
    for tid in sorted(track_ids):
        out[tid] = con.execute(
            f"SELECT frame_unix_nanos, {column} FROM {table} WHERE track_id = ? ORDER BY frame_unix_nanos",
            (tid,),
        ).fetchall()
    return out


def has_table(con, name):
    return (
        con.execute(
            "SELECT 1 FROM sqlite_master WHERE type='table' AND name=?", (name,)
        ).fetchone()
        is not None
    )


def screen(records, evidence=None):
    joins = len(records)
    frames = len({r.get("frame_unix_nanos") for r in records})
    m1 = [r for r in records if oversize(r)]
    m2 = [r for r in records if r.get("tracks", 0) >= 2]
    out = {
        "joins": joins,
        "frames_with_joins": frames,
        "components": sum(len(r.get("components", [])) for r in records),
        "m1_oversize": len(m1),
        "m2_two_track": len(m2),
        "m3_solid_body_jump": None,
        "m3_readable": 0,
        "box_length_jump": None,
        "box_readable": 0,
        "nonpositive_solid_body_rows": None,
        "owner_classes": {},
        "outside_owner_box_share_mean": None,
        "joins_with_returns_outside_owner_box": sum(
            1 for r in records if r.get("outside_owner_box_share", 0) > 0
        ),
    }
    for r in records:
        c = r.get("owner_class") or "none"
        out["owner_classes"][c] = out["owner_classes"].get(c, 0) + 1
    if joins:
        out["outside_owner_box_share_mean"] = (
            sum(r.get("outside_owner_box_share", 0) for r in records) / joins
        )
    flagged = {id(r) for r in m1} | {id(r) for r in m2}
    window = int(JUMP_WINDOW_S * 1e9)
    if evidence:
        con = sqlite3.connect(f"file:{evidence}?mode=ro", uri=True)
        owners = {r["owner_track_id"] for r in records if r.get("owner_track_id")}
        if has_table(con, "lidar_track_solid_bodies"):
            sb = track_rows(con, "lidar_track_solid_bodies", "length_m", owners)
            m3 = 0
            for r in records:
                d = jump(
                    sb.get(r.get("owner_track_id"), []),
                    r["frame_unix_nanos"],
                    window,
                    JUMP_M,
                )
                if d is None:
                    continue
                out["m3_readable"] += 1
                if d > JUMP_M:
                    m3 += 1
                    flagged.add(id(r))
            out["m3_solid_body_jump"] = m3
            out["nonpositive_solid_body_rows"] = con.execute(
                "SELECT COUNT(*) FROM lidar_track_solid_bodies WHERE length_m <= 0"
            ).fetchone()[0]
        box = track_rows(con, "lidar_track_observations", "box_length", owners)
        bj = 0
        for r in records:
            rows = [
                (t, v) for t, v in box.get(r.get("owner_track_id"), []) if v is not None
            ]
            d = jump(rows, r["frame_unix_nanos"], window, JUMP_M)
            if d is None:
                continue
            out["box_readable"] += 1
            if d > JUMP_M:
                bj += 1
        out["box_length_jump"] = bj
        con.close()
    out["any_flag"] = len(flagged)
    for k in ("m1_oversize", "m2_two_track", "m3_solid_body_jump", "any_flag"):
        v = out[k]
        out[k + "_per_1000"] = None if v is None or not joins else 1000.0 * v / joins
    return out


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("joins")
    ap.add_argument("--evidence")
    ap.add_argument("--json")
    a = ap.parse_args(argv)
    out = screen(read_records(a.joins), a.evidence)
    text = json.dumps(out, indent=1, sort_keys=True)
    print(text)
    if a.json:
        with open(a.json, "w", encoding="utf-8") as f:
            f.write(text + "\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
