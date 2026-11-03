#!/usr/bin/env python3
"""What a default flip costs: one arm's evidence against the defaults' on the same segment.

Reads two replays of one case, the defaults' (A) and the candidate's (B), each either an evidence
database (the `lidar_tracks` and `lidar_track_observations` tables that `lidar-box-eval
-evidence-db` and `lidar-state-estimation-baseline -evidence-per-case` write) or a `lidar-box-eval
-rows` CSV (every scored frame of every confirmed track, the published box's centre and the
track's speed; a track's duration is then its span within the scored window), and reports the
measures l5id0012's rule is written on. Definitions are fixed here, before any arm is replayed, and are the same for every
case and arm:

* track: a row of lidar_tracks (the evidence keeps confirmed tracks only); its rows are its
  lidar_track_observations rows.
* brief: duration under 1.0 s. lasting: 2.0 s or more.
* near-static: a lasting track whose first-to-last position is under 2.0 m apart and whose p95
  speed is under 0.5 m/s. clutter: brief and near-static tracks together
  (scripts/lidar_noise_screen.py's definitions).
* moving: a lasting track whose first-to-last position is 5.0 m or more apart and whose median
  speed is 1.0 m/s or more. A count of moving tracks is what a traffic count reads.
* covered: a row of one arm's track is covered by the other arm when some track of the other arm
  has a row at the same frame time within 2.0 m of it. A track's coverage is the share of its
  rows covered; its best single-track coverage is the largest share any one track of the other
  arm covers.
* lost: a moving track of A whose coverage by B is under 0.5. shortened: a moving track of A
  covered 0.5 or more by B but whose best single-track coverage by B is under 0.5 (B carries
  the vehicle only in pieces). added: a moving track of B whose coverage by A is under 0.5.
* matched pair: a moving track of A that is not lost, with the B track of best single-track
  coverage. Its speed difference is the B track's p85 row speed less the A track's.

Usage: lidar_flip_cost.py A.db|A.csv B.db|B.csv [--json OUT]
"""

from __future__ import annotations

import argparse
import csv
import json
import math
import sqlite3
import sys

BRIEF_S = 1.0
LASTING_S = 2.0
STATIC_DISP_M = 2.0
STATIC_P95_MPS = 0.5
MOVING_DISP_M = 5.0
MOVING_P50_MPS = 1.0
MATCH_M = 2.0
COVER_SHARE = 0.5


def pct(values, q):
    """Nearest-rank percentile, as lidar_noise_screen.py takes it."""
    if not values:
        return 0.0
    v = sorted(values)
    k = max(0, min(len(v) - 1, math.ceil(q * len(v)) - 1))
    return v[k]


def load_rows(path):
    """A lidar-box-eval -rows CSV: the published variant's rows, tracks spanning their rows."""
    tracks = {}
    with open(path, newline="") as f:
        for r in csv.DictReader(f):
            if r["variant"] != "published":
                continue
            t = tracks.setdefault(r["track"], {"rows": []})
            t["rows"].append(
                (int(r["t_ns"]), float(r["x"]), float(r["y"]), float(r["speed"]))
            )
    for t in tracks.values():
        t["rows"].sort()
        t["t0"], t["t1"] = t["rows"][0][0], t["rows"][-1][0]
        t["dur"] = (t["t1"] - t["t0"]) / 1e9
    return tracks


def load(path):
    if path.endswith(".csv"):
        return derive(load_rows(path))
    con = sqlite3.connect(f"file:{path}?mode=ro", uri=True)
    tracks = {}
    for tid, t0, t1, dur in con.execute(
        "SELECT track_id, start_unix_nanos, end_unix_nanos, track_duration_secs FROM lidar_tracks"
    ):
        tracks[tid] = {
            "t0": t0,
            "t1": t1,
            "dur": dur if dur is not None else (t1 - t0) / 1e9,
            "rows": [],
        }
    for tid, ts, x, y, spd in con.execute(
        "SELECT track_id, ts_unix_nanos, x, y, speed_mps FROM lidar_track_observations ORDER BY track_id, ts_unix_nanos"
    ):
        if tid in tracks:
            tracks[tid]["rows"].append((ts, x, y, spd or 0.0))
    con.close()
    return derive(tracks)


def derive(tracks):
    for t in tracks.values():
        rows = t["rows"]
        t["disp"] = (
            math.hypot(rows[-1][1] - rows[0][1], rows[-1][2] - rows[0][2])
            if rows
            else 0.0
        )
        speeds = [r[3] for r in rows]
        t["p50"], t["p85"], t["p95"] = (
            pct(speeds, 0.5),
            pct(speeds, 0.85),
            pct(speeds, 0.95),
        )
        t["brief"] = t["dur"] < BRIEF_S
        t["lasting"] = t["dur"] >= LASTING_S
        t["static"] = (
            t["lasting"]
            and bool(rows)
            and t["disp"] < STATIC_DISP_M
            and t["p95"] < STATIC_P95_MPS
        )
        t["moving"] = (
            t["lasting"]
            and bool(rows)
            and t["disp"] >= MOVING_DISP_M
            and t["p50"] >= MOVING_P50_MPS
        )
    return tracks


def by_time(tracks):
    """Frame time -> [(track id, x, y)]."""
    idx = {}
    for tid, t in tracks.items():
        for ts, x, y, _ in t["rows"]:
            idx.setdefault(ts, []).append((tid, x, y))
    return idx


def coverage(track, other_idx):
    """(share of rows covered by any other track, best single-track share, best track id)."""
    rows = track["rows"]
    if not rows:
        return 0.0, 0.0, None
    any_hits = 0
    per = {}
    for ts, x, y, _ in rows:
        hit = set()
        for oid, ox, oy in other_idx.get(ts, ()):
            if math.hypot(ox - x, oy - y) <= MATCH_M:
                hit.add(oid)
        if hit:
            any_hits += 1
        for oid in hit:
            per[oid] = per.get(oid, 0) + 1
    best = max(per.items(), key=lambda kv: kv[1]) if per else (None, 0)
    return any_hits / len(rows), best[1] / len(rows), best[0]


def summary(tracks):
    return {
        "tracks": len(tracks),
        "brief": sum(t["brief"] for t in tracks.values()),
        "near_static": sum(t["static"] for t in tracks.values()),
        "clutter": sum(t["brief"] or t["static"] for t in tracks.values()),
        "moving": sum(t["moving"] for t in tracks.values()),
        "moving_p85_speed_p50": pct(
            [t["p85"] for t in tracks.values() if t["moving"]], 0.5
        ),
        "moving_p85_speed_p85": pct(
            [t["p85"] for t in tracks.values() if t["moving"]], 0.85
        ),
    }


def compare(a_path, b_path):
    a, b = load(a_path), load(b_path)
    ai, bi = by_time(a), by_time(b)
    lost = shortened = 0
    dv = []
    for t in a.values():
        if not t["moving"]:
            continue
        share, best, bid = coverage(t, bi)
        if share < COVER_SHARE:
            lost += 1
            continue
        if best < COVER_SHARE:
            shortened += 1
        dv.append(b[bid]["p85"] - t["p85"])
    added = sum(
        1 for t in b.values() if t["moving"] and coverage(t, ai)[0] < COVER_SHARE
    )
    absdv = [abs(x) for x in dv]
    return {
        "a": summary(a),
        "b": summary(b),
        "lost": lost,
        "shortened": shortened,
        "added": added,
        "matched": len(dv),
        "speed_p85_diff_median_abs": pct(absdv, 0.5),
        "speed_p85_diff_p95_abs": pct(absdv, 0.95),
        "speed_p85_diff_mean": sum(dv) / len(dv) if dv else 0.0,
    }


def main():
    ap = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter
    )
    ap.add_argument("a_db")
    ap.add_argument("b_db")
    ap.add_argument("--json")
    args = ap.parse_args()
    out = compare(args.a_db, args.b_db)
    text = json.dumps(out, indent=1, sort_keys=True)
    if args.json:
        with open(args.json, "w") as f:
            f.write(text + "\n")
    print(text)
    return 0


if __name__ == "__main__":
    sys.exit(main())
