"""Tests for lidar_figure_sensitivity.py: figure0001's count, p85 and p98 on hand-built rows."""

from __future__ import annotations

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import lidar_figure_sensitivity as fs  # noqa: E402
from test_lidar_flip_cost import S, drive, make_db, park  # noqa: E402


def test_max_speed_ignores_a_one_frame_spike():
    rows = [(int(i * S / 10), 0.0, 0.0, 10.0, None) for i in range(30)]
    rows[15] = (rows[15][0], 0.0, 0.0, 40.0, None)
    assert fs.max_speed(rows) == 10.0


def test_max_speed_needs_three_rows_in_a_window():
    rows = [(0, 0.0, 0.0, 5.0, None), (int(2 * S), 0.0, 0.0, 9.0, None)]
    assert fs.max_speed(rows) is None


def test_figures_and_changes(tmp_path):
    a_tracks = {f"car{i}": drive(0, 5, y=4.0 * i, v=8.0 + i) for i in range(10)}
    a_tracks["parked"] = park(0, 5, 50.0)
    b_tracks = dict(a_tracks)
    del b_tracks["car9"]  # the fastest, 17 m/s, goes
    a = make_db(tmp_path / "a.db", a_tracks)
    b = make_db(tmp_path / "b.db", b_tracks)
    out = fs.compare(a, b)
    assert out["a"]["count"] == 10 and out["b"]["count"] == 9
    assert out["count_change"] == -1 and abs(out["count_change_share"] + 0.1) < 1e-9
    assert out["a"]["p98"] == 17.0 and out["b"]["p98"] == 16.0
    assert out["p98_change"] == -1.0
    assert out["small_case"] is False


def test_identical_arms_move_nothing(tmp_path):
    tracks = {"car": drive(0, 8), "van": drive(2, 9, y=6.0)}
    out = fs.compare(
        make_db(tmp_path / "a.db", tracks), make_db(tmp_path / "b.db", tracks)
    )
    assert out["count_change"] == 0 and out["p85_change"] == 0.0
    assert out["small_case"] is True
