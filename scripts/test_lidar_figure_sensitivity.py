"""Tests for lidar_figure_sensitivity.py: figure0001's count, p85 and p98 on hand-built rows."""

from __future__ import annotations

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import lidar_figure_sensitivity as fs  # noqa: E402
import lidar_flip_cost as fc  # noqa: E402
from test_lidar_flip_cost import (
    S,
    boxed,
    drive,
    make_box_rows,
    make_db,
    park,
)  # noqa: E402


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


def run(t0: float, t1: float, x0: float, v: float, y: float = 0.0):
    """A body moving along x from x0 at v m/s (v may be 0: standing), a row every 0.1 s."""
    n = int(round((t1 - t0) * 10))
    return [(t0 + i / 10, x0 + v * i / 10, y, v) for i in range(n + 1)]


def car(rows):
    return boxed(rows, 4.5, 1.8)


def test_peak_test_reads_the_rolling_maximum_not_the_life():
    tracks = fc.derive(
        {
            name: {
                "rows": [(int(t * S), x, y, v, None) for t, x, y, v in rows],
                "t0": int(rows[0][0] * S),
                "t1": int(rows[-1][0] * S),
                "dur": rows[-1][0] - rows[0][0],
            }
            for name, rows in {
                # median 1.2 m/s over 12 m: moving by the median test only
                "walker": run(0, 10, 0.0, 1.2),
                # stands for 8 s, then 2 s at 3 m/s: moving by the peak test only
                "starter": run(0, 8, 0.0, 0.0) + run(8.1, 10, 0.3, 3.0),
                # 1.5 s at 10 m/s: too brief for either
                "blip": run(0, 1.5, 0.0, 10.0),
                # two rows 5 s apart: no window of three rows, so no maximum
                "sparse": [(0.0, 0.0, 0.0, 9.0), (5.0, 45.0, 0.0, 9.0)],
            }.items()
        }
    )
    fs.mark_moving(tracks, "median")
    median = {k for k, t in tracks.items() if t["fig_moving"]}
    fs.mark_moving(tracks, "peak")
    peak = {k for k, t in tracks.items() if t["fig_moving"]}
    assert median == {"walker", "sparse"}
    assert peak == {"starter"}
    assert tracks["starter"]["max_speed"] == 3.0
    assert tracks["sparse"]["max_speed"] is None


def test_default_output_is_unchanged(tmp_path):
    tracks = {"car": drive(0, 8), "van": drive(2, 9, y=6.0)}
    a, b = make_db(tmp_path / "a.db", tracks), make_db(tmp_path / "b.db", tracks)
    out = fs.compare(a, b)
    assert set(out) == {
        "a",
        "b",
        "count_change",
        "count_change_share",
        "p85_change",
        "p98_change",
        "small_case",
    }
    assert out == fs.compare(a, b, "median")


def test_stop_and_go_join_moves_the_median_count_not_the_peak_count(tmp_path):
    # One body drives 6 s at 3 m/s, stands 24 s, drives 6 s again. A carries it as
    # two pieces (it lost the body while it stood); B carries it in one track whose
    # whole-life median speed is 0, so the median test drops it once joined.
    first, stand, second = (
        run(0, 6, 0.0, 3.0),
        run(6.1, 29.9, 18.0, 0.0),
        run(30, 36, 18.0, 3.0),
    )
    parked = car(park(0, 36, 60.0))
    a = make_box_rows(
        tmp_path / "a.csv",
        {"piece-1": car(first), "piece-2": car(second), "parked": parked},
    )
    b = make_box_rows(
        tmp_path / "b.csv", {"whole": car(first + stand + second), "parked": parked}
    )

    med = fs.compare(a, b, "median", with_joins=True)
    assert med["a"]["count"] == 2 and med["b"]["count"] == 0
    assert med["count_change"] == -2 and med["join_credit"] == 1
    assert med["credited_count_change"] == -1  # still a loss once joined

    peak = fs.compare(a, b, "peak", with_joins=True)
    assert peak["a"]["count"] == 2 and peak["b"]["count"] == 1
    assert peak["count_change"] == -1 and peak["join_credit"] == 1
    assert (
        peak["credited_count_change"] == 0 and peak["credited_count_change_share"] == 0
    )
    assert peak["p85_change"] == 0.0

    (join,) = peak["joins"]
    assert join["b_track"] == "whole" and join["b_moving"] is True
    assert join["concurrent"] is False
    assert [m["a_track"] for m in join["members"]] == ["piece-1", "piece-2"]
    assert (
        join["members"][0]["start_s"] == 0.0 and join["members"][1]["start_s"] == 30.0
    )
    assert join["b_start_s"] == 0.0 and join["b_end_s"] == 36.0
    assert med["joins"][0]["b_moving"] is False


def test_a_duplicate_removed_is_a_concurrent_join(tmp_path):
    a = make_box_rows(
        tmp_path / "a.csv",
        {"one": car(drive(0, 5)), "twin": car(run(0, 5, 0.5, 10.0))},
    )
    b = make_box_rows(tmp_path / "b.csv", {"one": car(drive(0, 5))})
    out = fs.compare(a, b, "peak", with_joins=True)
    assert out["count_change"] == -1 and out["join_credit"] == 1
    assert out["credited_count_change"] == 0
    assert out["joins"][0]["concurrent"] is True


def test_neighbours_in_two_lanes_are_not_joined(tmp_path):
    tracks = {"near": car(drive(0, 5)), "far": car(drive(0, 5, y=3.5))}
    a = make_box_rows(tmp_path / "a.csv", tracks)
    b = make_box_rows(tmp_path / "b.csv", tracks)
    out = fs.compare(a, b, "peak", with_joins=True)
    assert out["joins"] == [] and out["join_credit"] == 0
    assert out["credited_count_change"] == 0


def test_unknown_moving_test_is_refused():
    try:
        fs.mark_moving({}, "mean")
    except ValueError:
        return
    raise AssertionError("expected a ValueError")


def test_identical_arms_with_a_duplicate_have_no_join(tmp_path):
    # A duplicate lies wholly inside a longer track's footprint: both cover it fully,
    # and the tie goes to the track itself.
    tracks = {"long": car(drive(0, 8)), "twin": car(run(2, 5, 20.0, 10.0))}
    a = make_box_rows(tmp_path / "a.csv", tracks)
    b = make_box_rows(tmp_path / "b.csv", tracks)
    out = fs.compare(a, b, "peak", with_joins=True)
    assert out["joins"] == [] and out["join_credit"] == 0


def test_a_member_still_counted_elsewhere_is_not_credited(tmp_path):
    # B joins A's two pieces in one track but also keeps its own track on the later
    # part of the second piece (60 % of its rows), so B's count already holds both:
    # nothing is credited.
    first, second = run(0, 6, 0.0, 3.0), run(6.1, 12, 18.3, 3.0)
    a = make_box_rows(tmp_path / "a.csv", {"p1": car(first), "p2": car(second)})
    b = make_box_rows(
        tmp_path / "b.csv",
        {"whole": car(first + second), "kept": car(run(8.4, 12, 25.2, 3.0, y=0.2))},
    )
    out = fs.compare(a, b, "peak", with_joins=True)
    assert out["count_change"] == 0
    (join,) = out["joins"]
    assert join["b_track"] == "whole"
    assert join["members"][1]["also_carried_by"] == ["kept"]
    assert join["credit"] == 0 and out["credited_count_change"] == 0
    assert out["members_less_one"] == 1
