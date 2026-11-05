"""Tests for lidar_clutter_origin.py: l6cls0004's clutter classes on a hand-built database."""

from __future__ import annotations

import sqlite3
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import lidar_clutter_origin as co  # noqa: E402

S = 1_000_000_000


def make_db(path: Path, tracks, clusters):
    """tracks: id -> (class, [(seconds, x, y, speed, length, width)]); clusters: [(s, x, y)]."""
    con = sqlite3.connect(path)
    con.execute(
        "CREATE TABLE lidar_tracks (track_id TEXT, start_unix_nanos INTEGER, end_unix_nanos INTEGER,"
        " track_duration_secs REAL, object_class TEXT, observation_count INTEGER)"
    )
    con.execute(
        "CREATE TABLE lidar_track_observations (track_id TEXT, ts_unix_nanos INTEGER, x REAL, y REAL,"
        " speed_mps REAL, box_x REAL, box_y REAL, box_heading_rad REAL, box_length REAL, box_width REAL)"
    )
    con.execute(
        "CREATE TABLE lidar_clusters (ts_unix_nanos INTEGER, centroid_x REAL, centroid_y REAL)"
    )
    for tid, (cls, rows) in tracks.items():
        t0, t1 = int(rows[0][0] * S), int(rows[-1][0] * S)
        con.execute(
            "INSERT INTO lidar_tracks VALUES (?,?,?,?,?,?)",
            (tid, t0, t1, (t1 - t0) / S, cls, len(rows)),
        )
        for t, x, y, v, length, width in rows:
            con.execute(
                "INSERT INTO lidar_track_observations VALUES (?,?,?,?,?,?,?,?,?,?)",
                (tid, int(round(t * S)), x, y, v, x, y, 0.0, length, width),
            )
    for t, x, y in clusters:
        con.execute(
            "INSERT INTO lidar_clusters VALUES (?,?,?)", (int(round(t * S)), x, y)
        )
    con.commit()
    con.close()
    return str(path)


def rows(t0, t1, x, y, v=0.0, length=4.5, width=1.8, vx=0.0):
    n = int(round((t1 - t0) * 10))
    return [(t0 + i / 10, x + vx * i / 10, y, v, length, width) for i in range(n + 1)]


def test_clutter_classes_and_recurrence(tmp_path):
    tracks = {
        "car": ("car", rows(0, 20, 0, 0, v=10.0, vx=10.0)),
        # parked car: near-static, car-sized
        "parked": ("car", rows(0, 20, 5, 30)),
        # a brief piece riding the moving car
        "piece": (
            "dynamic",
            rows(5.0, 5.5, 50, 0.5, v=10.0, length=0.8, width=0.8, vx=10.0),
        ),
        # a bird far from anything
        "bird": ("bird", rows(1.0, 1.5, 300, 300, length=0.4, width=0.4)),
    }
    # an unowned cluster recurring in one cell for 12 seconds, far from the path
    clusters = [(float(s), 100.2, 100.2) for s in range(12)]
    db = make_db(tmp_path / "e.db", tracks, clusters)
    out = co.classes(db)
    assert out["classes"] == {
        "artefact": 1,
        "stationary_road_user": 1,
        "fragment_of_mover": 1,
        "unclassed": 0,
    }
    assert out["recurring_cells"] == 1
    assert out["recurring_cells_in_background"] == 1
    assert out["unowned_in_recurring_cells_share"] == 1.0
