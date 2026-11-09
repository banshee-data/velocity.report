"""Tests for the motion join's label-free merge reference (l4clus0011's measures)."""

import json
import sqlite3
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import lidar_join_merge_reference as ref  # noqa: E402

S = 1_000_000_000


def record(
    frame_s, owner="t1", cls="car", length=4.5, width=1.9, tracks=1, outside=0.0
):
    return {
        "frame_id": f"f{frame_s}",
        "frame_unix_nanos": int(frame_s * S),
        "cluster_id": 1,
        "points": 40,
        "length_m": length,
        "width_m": width,
        "components": [
            {"cluster_id": 1, "points": 20},
            {"cluster_id": 2, "points": 20},
        ],
        "owner_track_id": owner,
        "owner_class": cls,
        "outside_owner_box_share": outside,
        "tracks": tracks,
    }


def build(path, solid, box):
    """solid, box: {track: [(seconds, length), ...]}."""
    con = sqlite3.connect(path)
    con.execute(
        "CREATE TABLE lidar_track_solid_bodies (track_id, frame_unix_nanos, length_m)"
    )
    con.execute(
        "CREATE TABLE lidar_track_observations (track_id, frame_unix_nanos, box_length)"
    )
    for table, rows in (
        ("lidar_track_solid_bodies", solid),
        ("lidar_track_observations", box),
    ):
        for tid, series in rows.items():
            for t, v in series:
                con.execute(f"INSERT INTO {table} VALUES (?,?,?)", (tid, int(t * S), v))
    con.commit()
    con.close()


class OversizeTest(unittest.TestCase):
    def test_limits_follow_the_owner_class(self):
        self.assertFalse(ref.oversize(record(1, cls="car", length=6.9)))
        self.assertTrue(ref.oversize(record(1, cls="car", length=7.1)))
        self.assertFalse(ref.oversize(record(1, cls="truck", length=11.9)))
        self.assertTrue(ref.oversize(record(1, cls="truck", width=3.1)))
        self.assertFalse(ref.oversize(record(1, cls="bus", length=18.0, width=3.1)))
        # An unlisted or missing class reads as a vehicle.
        self.assertTrue(ref.oversize(record(1, cls="", length=12.5)))
        self.assertFalse(ref.oversize(record(1, cls="dynamic", length=11.0)))


class JumpTest(unittest.TestCase):
    def test_before_and_first_row_in_window(self):
        rows = [
            (int(0.8 * S), 4.0),
            (int(0.9 * S), 4.1),
            (int(1.0 * S), 6.5),
            (int(1.1 * S), 9.0),
        ]
        self.assertAlmostEqual(ref.jump(rows, S, int(0.2 * S), 2.0), 2.4)

    def test_unreadable_without_both_sides(self):
        self.assertIsNone(ref.jump([(int(1.5 * S), 9.0)], S, int(0.2 * S), 2.0))
        self.assertIsNone(ref.jump([(int(0.5 * S), 4.0)], S, int(0.2 * S), 2.0))
        # A row after the window is not the join's.
        self.assertIsNone(
            ref.jump([(int(0.5 * S), 4.0), (int(1.5 * S), 9.0)], S, int(0.2 * S), 2.0)
        )


class ScreenTest(unittest.TestCase):
    def test_counts_and_union(self):
        recs = [
            record(1.0),  # clean
            record(2.0, length=9.0),  # M1
            record(3.0, tracks=2, outside=0.25),  # M2
            record(4.0, owner="t2"),  # M3 from the evidence
            record(5.0, length=9.0, tracks=2),  # M1 and M2: counted once in the union
        ]
        with tempfile.TemporaryDirectory() as d:
            db = str(Path(d) / "e.db")
            build(
                db,
                solid={
                    "t1": [(0.9, 4.0), (1.0, 4.2), (1.9, 4.2), (2.0, 4.3)],
                    "t2": [(3.9, 4.0), (4.0, 7.0)],
                    "t3": [(1, 0.0)],
                },
                box={"t1": [(0.9, 4.0), (1.0, 4.1)], "t2": [(3.9, 4.0), (4.0, 4.1)]},
            )
            out = ref.screen(recs, db)
        self.assertEqual(out["joins"], 5)
        self.assertEqual(out["frames_with_joins"], 5)
        self.assertEqual(out["m1_oversize"], 2)
        self.assertEqual(out["m2_two_track"], 2)
        self.assertEqual(out["m3_solid_body_jump"], 1)
        self.assertEqual(out["m3_readable"], 3)
        self.assertEqual(out["any_flag"], 4)
        self.assertEqual(out["box_length_jump"], 0)
        self.assertEqual(out["nonpositive_solid_body_rows"], 1)
        self.assertEqual(out["joins_with_returns_outside_owner_box"], 1)
        self.assertAlmostEqual(out["any_flag_per_1000"], 800.0)

    def test_without_evidence_m3_is_unread(self):
        out = ref.screen([record(1.0)])
        self.assertIsNone(out["m3_solid_body_jump"])
        self.assertEqual(out["any_flag"], 0)

    def test_empty_file(self):
        out = ref.screen([])
        self.assertEqual(out["joins"], 0)
        self.assertIsNone(out["m1_oversize_per_1000"])

    def test_reads_jsonl(self):
        with tempfile.TemporaryDirectory() as d:
            p = Path(d) / "j.jsonl"
            p.write_text(
                json.dumps(record(1.0)) + "\n\n" + json.dumps(record(2.0)) + "\n"
            )
            self.assertEqual(len(ref.read_records(str(p))), 2)


if __name__ == "__main__":
    unittest.main()
