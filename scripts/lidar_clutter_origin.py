#!/usr/bin/env python3
"""What one replay's clutter is, and where its unowned clusters recur (experiment l6cls0004).

Reads one evidence database (`lidar-state-estimation-baseline -evidence-per-case`) and reports the
classes l6cls0004 fixed before any evidence was read. A clutter track is brief (under 1.0 s) or
near-static (lasting, under 2.0 m first to last, p95 speed under 0.5 m/s), as lidar_noise_screen.py
defines them. A footprint is a row's box grown by 0.25 m. A moving track is lidar_flip_cost.py's.

* artefact: a clutter track whose rows lie within 1.0 m of no moving track's footprint at the same
  frames, and whose mean box is under 1.5 m long and under 1.5 m wide.
* stationary road user: a near-static track whose mean box is 1.5 m or more long or wide, or
  classed pedestrian and observed on half or more of the frames of its life.
* fragment of a mover: a clutter track at least one of whose rows lies inside, or within 1.0 m
  of, a moving track's footprint at the same frame.
* unclassed: the rest. A track is put in the first class that holds, in the order fragment,
  stationary road user, artefact.

Unowned clusters: a cluster (lidar_clusters, or the raw cluster of each lidar_observations record
where that table is empty, as replay evidence keeps them) is owned when a lasting track has a row at its frame
within 1.0 m of its centroid; the others are binned in 0.5 m cells. A cell recurs when it holds
unowned clusters in 10 or more distinct seconds of the replay. A recurring cell is beside a path
when within 1.5 m of a moving track's footprint at any frame, otherwise in the background.

Usage: lidar_clutter_origin.py EVIDENCE.db [--json OUT]
"""

from __future__ import annotations

import argparse
import json
import math
import sqlite3
import sys

import lidar_flip_cost as fc

NEAR_M = 1.0
SMALL_M = 1.5
OWN_M = 1.0
CELL_M = 0.5
RECUR_S = 10
PATH_M = 1.5


def near_footprint(box, x, y, margin):
    return box is not None and fc.inside(box, x, y, fc.FOOTPRINT_MARGIN_M + margin)


def classes(path):
    con = sqlite3.connect(f"file:{path}?mode=ro", uri=True)
    meta = {
        tid: (cls, n)
        for tid, cls, n in con.execute(
            "SELECT track_id, object_class, observation_count FROM lidar_tracks"
        )
    }
    clusters = con.execute(
        "SELECT ts_unix_nanos, centroid_x, centroid_y FROM lidar_clusters"
    ).fetchall()
    tables = {r[0] for r in con.execute("SELECT name FROM sqlite_master")}
    if not clusters and "lidar_observations" in tables:
        # Replay evidence keeps its clusters as observation records, stamped
        # with the frame time lidar_flip_cost.load keys track rows by.
        clusters = con.execute(
            "SELECT frame_unix_nanos,"
            " json_extract(record_json, '$.raw_cluster.CentroidX'),"
            " json_extract(record_json, '$.raw_cluster.CentroidY')"
            " FROM lidar_observations"
        ).fetchall()
    con.close()
    tracks = fc.load(path)
    idx = fc.by_time(tracks)
    frames = sorted(idx)
    frame_dt = (frames[-1] - frames[0]) / (len(frames) - 1) if len(frames) > 1 else 1e8
    moving = {tid for tid, t in tracks.items() if t["moving"]}
    counts = {
        "artefact": 0,
        "stationary_road_user": 0,
        "fragment_of_mover": 0,
        "unclassed": 0,
    }
    for tid, t in tracks.items():
        if not (t["brief"] or t["static"]) or not t["rows"]:
            continue
        near_mover = False
        for ts, x, y, _, _ in t["rows"]:
            for oid, _, _, obox in idx.get(ts, ()):
                if oid != tid and oid in moving and near_footprint(obox, x, y, NEAR_M):
                    near_mover = True
                    break
            if near_mover:
                break
        boxes = [r[4] for r in t["rows"] if r[4] is not None]
        mean_l = sum(b[3] for b in boxes) / len(boxes) if boxes else 0.0
        mean_w = sum(b[4] for b in boxes) / len(boxes) if boxes else 0.0
        cls, n_obs = meta.get(tid, (None, 0))
        life_frames = max(1, round((t["t1"] - t["t0"]) / frame_dt) + 1)
        if near_mover:
            counts["fragment_of_mover"] += 1
        elif t["static"] and (
            mean_l >= SMALL_M
            or mean_w >= SMALL_M
            or (cls == "pedestrian" and (n_obs or 0) * 2 >= life_frames)
        ):
            counts["stationary_road_user"] += 1
        elif mean_l < SMALL_M and mean_w < SMALL_M:
            counts["artefact"] += 1
        else:
            counts["unclassed"] += 1

    lasting_at = {
        ts: [(x, y) for oid, x, y, _ in rows if tracks[oid]["lasting"]]
        for ts, rows in idx.items()
    }
    cells = {}
    unowned = 0
    for ts, cx, cy in clusters:
        if any(math.hypot(x - cx, y - cy) <= OWN_M for x, y in lasting_at.get(ts, ())):
            continue
        unowned += 1
        key = (math.floor(cx / CELL_M), math.floor(cy / CELL_M))
        cells.setdefault(key, set()).add(ts // 1_000_000_000)
    recurring = {k for k, secs in cells.items() if len(secs) >= RECUR_S}
    beside = set()
    for ts, rows in idx.items():
        for oid, _, _, box in rows:
            if oid not in moving or box is None:
                continue
            for k in recurring - beside:
                cx, cy = (k[0] + 0.5) * CELL_M, (k[1] + 0.5) * CELL_M
                if near_footprint(box, cx, cy, PATH_M):
                    beside.add(k)
    in_recurring = sum(
        1
        for ts, cx, cy in clusters
        if (math.floor(cx / CELL_M), math.floor(cy / CELL_M)) in recurring
        and not any(
            math.hypot(x - cx, y - cy) <= OWN_M for x, y in lasting_at.get(ts, ())
        )
    )
    return {
        "clutter_tracks": sum(counts.values()),
        "classes": counts,
        "unowned_clusters": unowned,
        "recurring_cells": len(recurring),
        "recurring_cells_beside_a_path": len(beside),
        "recurring_cells_in_background": len(recurring) - len(beside),
        "unowned_in_recurring_cells_share": in_recurring / unowned if unowned else 0.0,
    }


def main():
    ap = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter
    )
    ap.add_argument("db")
    ap.add_argument("--json")
    args = ap.parse_args()
    out = classes(args.db)
    text = json.dumps(out, indent=1, sort_keys=True)
    if args.json:
        with open(args.json, "w") as f:
            f.write(text + "\n")
    print(text)
    return 0


if __name__ == "__main__":
    sys.exit(main())
