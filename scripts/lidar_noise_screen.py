#!/usr/bin/env python3
"""Label-free noise and split screen over replay evidence databases.

Reads the evidence database `lidar-state-estimation-baseline -evidence-per-case` writes and
reports, per case, the measures the PCL noise experiments (l3bg0028, l4clus0005, and the
outlier/planarity gate) are scored on. Definitions are fixed here, before any arm is replayed,
and are the same for every arm:

* track: a row of lidar_tracks (the evidence keeps confirmed tracks only).
* lasting: duration >= 2.0 s. brief: duration < 1.0 s.
* near-static: a lasting track whose first-to-last position is under 2.0 m apart and whose p95
  per-observation speed is under 0.5 m/s ("little movement").
* vehicle-sized: a lasting track whose p90 of max(box length, box width) is 3.0 m or more.
* spurious clusters per frame: observations no lasting track takes as a measurement, over the
  frames that have observations. Approximate: lidar_track_observations carries no cluster id,
  so a track's observation count stands in for the clusters it consumed.
* ordered-pair splits: pairs of vehicle-sized lasting tracks whose end/start are within 3.0 s and
  3.0 m (A ends, B starts), a count of candidate fragment links, not truth (l5id0007 owns truth).

Usage: lidar_noise_screen.py EVIDENCE.db [EVIDENCE.db ...] [--json OUT]
"""

from __future__ import annotations

import argparse
import json
import math
import sqlite3
import sys

LASTING_S = 2.0
BRIEF_S = 1.0
STATIC_DISP_M = 2.0
STATIC_P95_MPS = 0.5
VEHICLE_P90_M = 3.0
LINK_GAP_S = 3.0
LINK_DIST_M = 3.0


def pct(values, q):
    if not values:
        return 0.0
    v = sorted(values)
    k = max(0, min(len(v) - 1, math.ceil(q * len(v)) - 1))  # nearest rank
    return v[k]


def screen(path):
    con = sqlite3.connect(f"file:{path}?mode=ro", uri=True)
    tracks = con.execute(
        "SELECT track_id, start_unix_nanos, end_unix_nanos, track_duration_secs FROM lidar_tracks"
    ).fetchall()
    obs = {}
    for tid, ts, x, y, spd, bl, bw in con.execute(
        "SELECT track_id, ts_unix_nanos, x, y, speed_mps, bounding_box_length, bounding_box_width "
        "FROM lidar_track_observations ORDER BY track_id, ts_unix_nanos"
    ):
        obs.setdefault(tid, []).append((ts, x, y, spd, bl, bw))
    frames = con.execute("SELECT COUNT(DISTINCT frame_unix_nanos) FROM lidar_observations").fetchone()[0]
    total_obs = con.execute("SELECT COUNT(*) FROM lidar_observations").fetchone()[0]
    con.close()

    n = brief = lasting = static = vehicle = 0
    lasting_obs = 0
    vehicle_tracks = []
    for tid, t0, t1, dur in tracks:
        n += 1
        rows = obs.get(tid, [])
        if dur is None:
            dur = (t1 - t0) / 1e9
        if dur < BRIEF_S:
            brief += 1
        if dur < LASTING_S:
            continue
        lasting += 1
        lasting_obs += len(rows)
        if not rows:
            continue
        disp = math.hypot(rows[-1][1] - rows[0][1], rows[-1][2] - rows[0][2])
        p95 = pct([r[3] for r in rows], 0.95)
        if disp < STATIC_DISP_M and p95 < STATIC_P95_MPS:
            static += 1
        p90 = pct([max(r[4] or 0, r[5] or 0) for r in rows], 0.90)
        if p90 >= VEHICLE_P90_M:
            vehicle += 1
            vehicle_tracks.append((t0, t1, rows[0][1], rows[0][2], rows[-1][1], rows[-1][2]))

    links = 0
    for a in vehicle_tracks:
        for b in vehicle_tracks:
            if a is b:
                continue
            gap = (b[0] - a[1]) / 1e9
            if 0 <= gap <= LINK_GAP_S and math.hypot(b[2] - a[4], b[3] - a[5]) <= LINK_DIST_M:
                links += 1

    spurious = max(0, total_obs - lasting_obs)
    return {
        "frames_with_observations": frames,
        "observations": total_obs,
        "tracks": n,
        "brief_tracks": brief,
        "lasting_tracks": lasting,
        "near_static_tracks": static,
        "vehicle_tracks": vehicle,
        "candidate_split_links": links,
        "spurious_clusters_per_frame": round(spurious / frames, 4) if frames else None,
    }


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("dbs", nargs="+")
    ap.add_argument("--json")
    a = ap.parse_args()
    out = {}
    for p in a.dbs:
        name = p.rsplit("/", 1)[-1].removesuffix(".db")
        out[name] = screen(p)
    if a.json:
        with open(a.json, "w") as f:
            json.dump(out, f, indent=1, sort_keys=True)
    cols = ["tracks", "brief_tracks", "lasting_tracks", "near_static_tracks", "vehicle_tracks",
            "candidate_split_links", "spurious_clusters_per_frame"]
    print("case".ljust(28) + "".join(c[:14].rjust(15) for c in cols))
    for k, v in sorted(out.items()):
        print(k.ljust(28) + "".join(str(v[c]).rjust(15) for c in cols))
    return 0


if __name__ == "__main__":
    sys.exit(main())
