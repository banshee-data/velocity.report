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
        t0, t1 = int(round(rows[0][0] * S)), int(round(rows[-1][0] * S))
        con.execute(
            "INSERT INTO lidar_tracks VALUES (?,?,?,?)", (tid, t0, t1, (t1 - t0) / S)
        )
        for t, x, y, v in rows:
            con.execute(
                "INSERT INTO lidar_track_observations VALUES (?,?,?,?,?)",
                (tid, int(round(t * S)), x, y, v),
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
            "split-1": drive(10, 12, y=-10.0),
            "split-2": drive(12.1, 14, y=-10.0),
            "split-3": drive(14.1, 16, y=-10.0),
            "new": drive(20, 25, y=30.0),
            "blip": drive(30, 30.5, y=40.0),
            "parked": park(0, 5, 50.0),
        },
    )
    out = fc.compare(a, b)
    assert out["a"]["moving"] == 4 and out["b"]["moving"] == 3
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


def make_rows(
    path: Path, tracks: dict[str, list[tuple[float, float, float, float]]]
) -> str:
    """A lidar-box-eval -rows CSV with the published variant and one other to be ignored."""
    cols = ["variant", "track", "t_ns", "class", "speed", "x", "y"]
    with open(path, "w") as f:
        f.write(",".join(cols) + "\n")
        for tid, rows in tracks.items():
            for t, x, y, v in rows:
                for variant in ("published", "other"):
                    f.write(f"{variant},{tid},{int(round(t * S))},car,{v},{x},{y}\n")
    return str(path)


def test_rows_csv_reads_as_the_database(tmp_path):
    tracks_a = {
        "kept": drive(0, 5),
        "lost": drive(0, 5, y=10.0),
        "parked": park(0, 5, 50.0),
    }
    tracks_b = {
        "kept": drive(0, 5),
        "blip": drive(30, 30.5, y=40.0),
        "parked": park(0, 5, 50.0),
    }
    from_db = fc.compare(
        make_db(tmp_path / "a.db", tracks_a), make_db(tmp_path / "b.db", tracks_b)
    )
    from_csv = fc.compare(
        make_rows(tmp_path / "a.csv", tracks_a), make_rows(tmp_path / "b.csv", tracks_b)
    )
    assert from_csv == from_db
    assert from_csv["lost"] == 1 and from_csv["b"]["brief"] == 1


def make_box_rows(path: Path, tracks: dict[str, list[tuple]]) -> str:
    """Rows with the published box: (seconds, x, y, speed, length, width), heading along +x."""
    cols = [
        "variant",
        "track",
        "t_ns",
        "class",
        "speed",
        "x",
        "y",
        "psi_deg",
        "length",
        "width",
    ]
    with open(path, "w") as f:
        f.write(",".join(cols) + "\n")
        for tid, rows in tracks.items():
            for t, x, y, v, length, width in rows:
                f.write(
                    f"published,{tid},{int(round(t * S))},bus,{v},{x},{y},0,{length},{width}\n"
                )
    return str(path)


def boxed(rows, length, width, dx=0.0):
    return [(t, x + dx, y, v, length, width) for t, x, y, v in rows]


def test_footprint_match_keeps_the_rear_of_a_long_vehicle(tmp_path):
    # A: a bus carried as two tracks 6 m apart, front and rear. B: one track at the
    # front, its drawn box 12 m long. The point match loses the rear; the footprint
    # match does not.
    bus = drive(0, 5)
    a = make_box_rows(
        tmp_path / "a.csv",
        {"front": boxed(bus, 4.0, 2.5), "rear": boxed(bus, 4.0, 2.5, dx=-6.0)},
    )
    b = make_box_rows(tmp_path / "b.csv", {"bus": boxed(bus, 12.0, 2.5, dx=-3.0)})
    point = fc.compare(a, b, "point")
    foot = fc.compare(a, b, "footprint")
    assert point["lost"] >= 1
    assert foot["lost"] == 0 and foot["matched"] == 2


def test_footprint_match_spares_a_neighbour(tmp_path):
    # A car in the next lane, 3.5 m across, is not covered by a box on this lane.
    a = make_box_rows(tmp_path / "a.csv", {"car": boxed(drive(0, 5, y=3.5), 4.5, 1.8)})
    b = make_box_rows(tmp_path / "b.csv", {"other": boxed(drive(0, 5), 4.5, 1.8)})
    assert fc.compare(a, b, "footprint")["lost"] == 1


def test_brief_origin_classes(tmp_path):
    long = boxed(drive(0, 5), 12.0, 2.5)
    b = make_box_rows(
        tmp_path / "b.csv",
        {
            "bus": long,
            # ended at 2.0 s on the bus's body, and re-born 0.3 s later 1 m from where it ended
            "ended": boxed(drive(1.0, 2.0), 4.0, 2.5),
            "reborn": boxed(drive(2.3, 2.8), 4.0, 2.5, dx=-2.0),
            # a piece on the body with nothing ended near its start
            "piece": boxed(drive(4.0, 4.5), 1.0, 1.0, dx=-3.0),
            # a blip far away
            "blip": boxed(drive(1.0, 1.5, y=40.0), 1.0, 1.0),
        },
    )
    a = make_box_rows(tmp_path / "a.csv", {"bus": long})
    assert fc.compare(a, b)["brief_origin"] == {"re-birth": 1, "split": 1, "other": 1}


def test_speed_tail_by_age(tmp_path):
    a = make_db(tmp_path / "a.db", {"car": drive(0, 6)})
    # B's track starts 2 s into the car and reads 1 m/s fast.
    b = make_db(
        tmp_path / "b.db", {"car": [(t, x, y, 11.0) for t, x, y, _ in drive(0, 6)]}
    )
    out = fc.compare(a, b)
    tail = out["speed_tail_by_age"]
    assert (
        tail["under_1s"]["pairs"] == 1
        and abs(tail["under_1s"]["median_abs"] - 1.0) < 1e-9
    )
    assert tail["3s_or_more"]["pairs"] == 0


def test_database_rows_meet_on_their_frame_time(tmp_path):
    """Replay evidence stamps each row with its cluster's own time; two tracks in one frame
    share only frame_unix_nanos. Coverage must meet on the frame."""

    def db(path, offset_ns):
        con = sqlite3.connect(path)
        con.execute(
            "CREATE TABLE lidar_tracks (track_id TEXT, start_unix_nanos INTEGER, end_unix_nanos INTEGER, track_duration_secs REAL)"
        )
        con.execute(
            "CREATE TABLE lidar_track_observations (track_id TEXT, ts_unix_nanos INTEGER, frame_unix_nanos INTEGER, x REAL, y REAL, speed_mps REAL)"
        )
        rows = drive(0, 5)
        t0, t1 = int(rows[0][0] * S), int(rows[-1][0] * S)
        con.execute("INSERT INTO lidar_tracks VALUES ('car',?,?,?)", (t0, t1, 5.0))
        for t, x, y, v in rows:
            frame = int(round(t * S))
            con.execute(
                "INSERT INTO lidar_track_observations VALUES ('car',?,?,?,?,?)",
                (frame + offset_ns, frame, x, y, v),
            )
        con.commit()
        con.close()
        return str(path)

    out = fc.compare(db(tmp_path / "a.db", 3_000_000), db(tmp_path / "b.db", 7_000_000))
    assert out["lost"] == 0 and out["matched"] == 1
