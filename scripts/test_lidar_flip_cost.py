"""Tests for lidar_flip_cost.py: the l5id0012 measures on two hand-built evidence databases."""

from __future__ import annotations

import sqlite3
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import lidar_flip_cost as fc  # noqa: E402

S = 1_000_000_000  # nanoseconds a second


def make_db(
    path: Path, tracks: dict[str, list[tuple[float, float, float, float]]]
) -> str:
    """tracks: id -> [(seconds, x, y, speed)]; one row per 0.1 s frame as given."""
    con = sqlite3.connect(path)
    con.execute(
        "CREATE TABLE lidar_tracks (track_id TEXT, start_unix_nanos INTEGER, end_unix_nanos INTEGER, track_duration_secs REAL)"
    )
    con.execute(
        "CREATE TABLE lidar_track_observations (track_id TEXT, ts_unix_nanos INTEGER, x REAL, y REAL, speed_mps REAL)"
    )
    for tid, rows in tracks.items():
        t0, t1 = int(rows[0][0] * S), int(rows[-1][0] * S)
        con.execute(
            "INSERT INTO lidar_tracks VALUES (?,?,?,?)", (tid, t0, t1, (t1 - t0) / S)
        )
        for t, x, y, v in rows:
            con.execute(
                "INSERT INTO lidar_track_observations VALUES (?,?,?,?,?)",
                (tid, int(t * S), x, y, v),
            )
    con.commit()
    con.close()
    return str(path)


def drive(t0: float, t1: float, y: float = 0.0, v: float = 10.0):
    """A vehicle driving along x at v m/s, a row every 0.1 s."""
    n = int(round((t1 - t0) * 10))
    return [(t0 + i / 10, v * (t0 + i / 10), y, v) for i in range(n + 1)]


def park(t0: float, t1: float, x: float):
    n = int(round((t1 - t0) * 10))
    return [(t0 + i / 10, x, 20.0, 0.0) for i in range(n + 1)]


def test_lost_shortened_added_and_speeds(tmp_path):
    a = make_db(
        tmp_path / "a.db",
        {
            "kept": drive(0, 5),
            "lost": drive(0, 5, y=10.0),
            "split": drive(10, 16, y=-10.0),
            "dup": drive(0, 5, y=0.5),  # a duplicate on "kept"'s vehicle
            "parked": park(0, 5, 50.0),
        },
    )
    b = make_db(
        tmp_path / "b.db",
        {
            "kept": [
                (t, x, y, 11.0) for t, x, y, _ in drive(0, 5)
            ],  # p85 speed 1 m/s higher
            "split-1": drive(10, 12.5, y=-10.0),
            "split-2": drive(12.6, 16, y=-10.0),
            "new": drive(20, 25, y=30.0),
            "blip": drive(30, 30.5, y=40.0),
            "parked": park(0, 5, 50.0),
        },
    )
    out = fc.compare(a, b)
    assert out["a"]["moving"] == 4 and out["b"]["moving"] == 4
    assert out["lost"] == 1  # "lost": nothing in B within 2 m
    assert out["shortened"] == 1  # "split": covered whole, by no single track over half
    assert out["added"] == 1  # "new"
    assert out["matched"] == 3  # kept, split and the duplicate (covered by B's "kept")
    assert (
        out["b"]["brief"] == 1
        and out["b"]["near_static"] == 1
        and out["b"]["clutter"] == 2
    )
    assert out["a"]["clutter"] == 1
    # kept and dup are both matched to B's faster "kept" (+1 m/s); split to a piece (0).
    assert abs(out["speed_p85_diff_mean"] - 2.0 / 3) < 1e-9


def test_identical_arms_cost_nothing(tmp_path):
    tracks = {"car": drive(0, 8), "van": drive(2, 9, y=6.0), "parked": park(0, 9, 30.0)}
    a = make_db(tmp_path / "a.db", tracks)
    b = make_db(tmp_path / "b.db", tracks)
    out = fc.compare(a, b)
    assert (out["lost"], out["shortened"], out["added"]) == (0, 0, 0)
    assert out["a"] == out["b"]
    assert out["speed_p85_diff_p95_abs"] == 0.0
