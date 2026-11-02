"""Tests for the track quality gates (l6cls0002)."""

import sqlite3
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import lidar_track_gate as gate  # noqa: E402

SCRIPT = Path(__file__).resolve().parent / "lidar_track_gate.py"
STILL = {"disp": 0.5, "speed95": 0.1}
MOVED = {"disp": 30.0, "speed95": 8.0}


def decide(name, cls, duration, motion, n_obs=100, length=0.5, width=0.3):
    return gate.decide(name, cls, duration, motion["disp"], motion["speed95"], n_obs, length, width)


class DecideTest(unittest.TestCase):
    def test_moved_tracks_are_never_gated(self):
        for name in ("g1", "g2", "g3", "g4"):
            for cls in ("bird", "dynamic", None, "pedestrian", "car"):
                self.assertFalse(decide(name, cls, 0.5, MOVED), (name, cls))

    def test_g1_reads_near_static_quiet_tracks_only(self):
        self.assertTrue(decide("g1", "bird", 5.0, STILL))
        self.assertTrue(decide("g1", None, 5.0, STILL))
        self.assertFalse(decide("g1", "pedestrian", 5.0, STILL))
        self.assertFalse(decide("g1", "car", 5.0, STILL))

    def test_g2_adds_brief_quiet_tracks_under_two_metres(self):
        drifted = {"disp": 1.0, "speed95": 3.0}
        self.assertFalse(decide("g1", "bird", 0.5, drifted))
        self.assertTrue(decide("g2", "bird", 0.5, drifted))
        self.assertFalse(decide("g2", "bird", 2.0, drifted))
        self.assertFalse(decide("g2", "pedestrian", 0.5, drifted))

    def test_g3_reads_only_flickering_pedestrians(self):
        self.assertTrue(decide("g3", "pedestrian", 10.0, STILL, n_obs=40))
        self.assertFalse(decide("g3", "pedestrian", 10.0, STILL, n_obs=60))
        self.assertFalse(decide("g2", "pedestrian", 10.0, STILL, n_obs=40))

    def test_g4_keeps_anything_as_long_or_wide_as_a_car_is_wide(self):
        self.assertTrue(decide("g4", "dynamic", 5.0, STILL, length=1.4, width=0.8))
        self.assertFalse(decide("g4", "dynamic", 39.0, STILL, length=3.8, width=1.7))
        self.assertFalse(decide("g4", "dynamic", 5.0, STILL, length=1.0, width=1.5))
        self.assertFalse(decide("g4", "pedestrian", 10.0, STILL, n_obs=10))


class ApplyTest(unittest.TestCase):
    def test_copy_loses_the_gated_track_and_the_source_keeps_it(self):
        with tempfile.TemporaryDirectory() as d:
            src, dst = f"{d}/in.db", f"{d}/out.db"
            con = sqlite3.connect(src)
            con.execute(
                "CREATE TABLE lidar_tracks (track_id, object_class, track_duration_secs, start_unix_nanos, "
                "end_unix_nanos, bounding_box_length_avg, bounding_box_width_avg)"
            )
            con.execute("CREATE TABLE lidar_track_observations (track_id, ts_unix_nanos, x, y, speed_mps)")
            con.execute("CREATE TABLE lidar_track_estimates (track_id)")
            con.executemany(
                "INSERT INTO lidar_tracks VALUES (?,?,?,?,?,?,?)",
                [("clutter", "bird", 5.0, 0, 5, 0.5, 0.3), ("car", "car", 5.0, 0, 5, 4.5, 1.8)],
            )
            con.executemany(
                "INSERT INTO lidar_track_observations VALUES (?,?,?,?,?)",
                [("clutter", 0, 0, 0, 0.1), ("clutter", 1, 0.2, 0, 0.1), ("car", 0, 0, 0, 8), ("car", 1, 30, 0, 8)],
            )
            con.executemany("INSERT INTO lidar_track_estimates VALUES (?)", [("clutter",), ("car",)])
            con.commit()
            con.close()
            subprocess.run([sys.executable, str(SCRIPT), src, dst, "--gate", "g1"], check=True, capture_output=True)
            for table in ("lidar_tracks", "lidar_track_observations", "lidar_track_estimates"):
                out = sqlite3.connect(dst).execute(f"SELECT DISTINCT track_id FROM {table}").fetchall()
                self.assertEqual(out, [("car",)], table)
            kept = sqlite3.connect(src).execute("SELECT COUNT(*) FROM lidar_tracks").fetchone()[0]
            self.assertEqual(kept, 2)


if __name__ == "__main__":
    unittest.main()
