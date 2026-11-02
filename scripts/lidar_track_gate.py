#!/usr/bin/env python3
"""Apply a track quality gate to a copy of a replay evidence database (l6cls0002).

Removes the gated tracks and every row naming them from a copy, so the noise screen and the
per-frame scorer read the result without a replay. The input is never written.

Usage: lidar_track_gate.py IN.db OUT.db --gate g1|g2|g3
"""

from __future__ import annotations

import argparse
import math
import shutil
import sqlite3
import sys

STATIC_DISP_M = 2.0
STATIC_P95_MPS = 0.5
BRIEF_S = 1.0
FLICKER_SHARE = 0.5
FRAME_HZ = 10.0
QUIET = {"bird", "dynamic", "", None}
TRACK_TABLES = (
    "lidar_track_observations",
    "lidar_track_residuals",
    "lidar_track_solid_bodies",
    "lidar_track_estimate_revisions",
    "lidar_track_estimates",
    "lidar_run_tracks",
    "lidar_tracks",
)


def p95(values):
    v = sorted(values)
    return v[max(0, math.ceil(0.95 * len(v)) - 1)] if v else 0.0


def decide(gate, cls, duration, disp, speed95, n_obs):
    """Return whether a gate removes a track. The rule of l6cls0002, nothing else."""
    near_static = disp < STATIC_DISP_M and speed95 < STATIC_P95_MPS
    quiet = cls in QUIET
    if near_static and quiet:
        return True
    if gate in ("g2", "g3") and duration < BRIEF_S and quiet and disp < STATIC_DISP_M:
        return True
    if gate == "g3" and near_static and cls == "pedestrian":
        return n_obs < FLICKER_SHARE * duration * FRAME_HZ
    return False


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("src")
    ap.add_argument("dst")
    ap.add_argument("--gate", required=True, choices=["g1", "g2", "g3"])
    a = ap.parse_args()
    ro = sqlite3.connect(f"file:{a.src}?mode=ro", uri=True)
    obs = {}
    for tid, x, y, s in ro.execute(
        "SELECT track_id, x, y, speed_mps FROM lidar_track_observations ORDER BY track_id, ts_unix_nanos"
    ):
        obs.setdefault(tid, []).append((x, y, s))
    gated = []
    for tid, cls, dur, t0, t1 in ro.execute(
        "SELECT track_id, object_class, track_duration_secs, start_unix_nanos, end_unix_nanos FROM lidar_tracks"
    ):
        rows = obs.get(tid, [])
        if not rows:
            continue
        duration = dur if dur is not None else ((t1 or t0) - t0) / 1e9
        disp = math.hypot(rows[-1][0] - rows[0][0], rows[-1][1] - rows[0][1])
        if decide(a.gate, cls, duration, disp, p95([r[2] for r in rows]), len(rows)):
            gated.append(tid)
    ro.close()
    shutil.copyfile(a.src, a.dst)
    con = sqlite3.connect(a.dst)
    tables = {r[0] for r in con.execute("SELECT name FROM sqlite_master WHERE type='table'")}
    con.execute("PRAGMA foreign_keys=OFF")
    for t in TRACK_TABLES:
        if t in tables and any(r[1] == "track_id" for r in con.execute(f"PRAGMA table_info({t})")):
            con.executemany(f"DELETE FROM {t} WHERE track_id = ?", [(g,) for g in gated])
    con.commit()
    con.close()
    print(f"{a.gate}: removed {len(gated)} tracks")
    return 0


if __name__ == "__main__":
    sys.exit(main())
