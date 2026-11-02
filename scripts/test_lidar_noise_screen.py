"""Tests for the label-free noise screen (l6cls0002's measures)."""

import sqlite3
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import lidar_noise_screen as screen  # noqa: E402


def build(path, tracks, extra_observations=0):
    """tracks: (id, duration, [(x, y, speed, length, width), ...])."""
    con = sqlite3.connect(path)
    con.execute("CREATE TABLE lidar_tracks (track_id, start_unix_nanos, end_unix_nanos, track_duration_secs)")
    con.execute(
        "CREATE TABLE lidar_track_observations (track_id, ts_unix_nanos, x, y, speed_mps, "
        "bounding_box_length, bounding_box_width)"
    )
    con.execute("CREATE TABLE lidar_observations (frame_unix_nanos)")
    frame = 0
    for tid, start, duration, rows in tracks:
        t0 = int(start * 1e9)
        con.execute("INSERT INTO lidar_tracks VALUES (?,?,?,?)", (tid, t0, t0 + int(duration * 1e9), duration))
        for i, (x, y, speed, length, width) in enumerate(rows):
            con.execute(
                "INSERT INTO lidar_track_observations VALUES (?,?,?,?,?,?,?)",
                (tid, t0 + i, x, y, speed, length, width),
            )
            con.execute("INSERT INTO lidar_observations VALUES (?)", (frame,))
            frame += 1
    for _ in range(extra_observations):
        con.execute("INSERT INTO lidar_observations VALUES (?)", (0,))
    con.commit()
    con.close()


class ScreenTest(unittest.TestCase):
    def run_screen(self, tracks, extra=0):
        with tempfile.TemporaryDirectory() as d:
            p = f"{d}/case.db"
            build(p, tracks, extra)
            return screen.screen(p)

    def test_nearest_rank_percentile(self):
        self.assertEqual(screen.pct([1, 2, 3, 4], 0.5), 2)
        self.assertEqual(screen.pct([1, 2, 3, 4], 0.95), 4)
        self.assertEqual(screen.pct([], 0.5), 0.0)

    def test_classes(self):
        still = [(0, 0, 0.1, 0.5, 0.3), (0.5, 0, 0.1, 0.5, 0.3)]
        moved = [(0, 0, 8, 4.5, 1.8), (30, 0, 8, 4.5, 1.8)]
        got = self.run_screen([("s", 0, 5.0, still), ("m", 0, 5.0, moved), ("b", 0, 0.5, still)])
        self.assertEqual(got["tracks"], 3)
        self.assertEqual(got["brief_tracks"], 1)
        self.assertEqual(got["lasting_tracks"], 2)
        self.assertEqual(got["near_static_tracks"], 1)
        self.assertEqual(got["vehicle_tracks"], 1)

    def test_fast_stationary_jitter_is_not_near_static(self):
        jitter = [(0, 0, 3.0, 0.5, 0.3), (0.5, 0, 3.0, 0.5, 0.3)]
        self.assertEqual(self.run_screen([("j", 0, 5.0, jitter)])["near_static_tracks"], 0)

    def test_split_link_needs_time_and_place(self):
        a = [(0, 0, 8, 4.5, 1.8), (20, 0, 8, 4.5, 1.8)]
        near = [(21, 0, 8, 4.5, 1.8), (40, 0, 8, 4.5, 1.8)]
        far = [(90, 0, 8, 4.5, 1.8), (120, 0, 8, 4.5, 1.8)]
        self.assertEqual(self.run_screen([("a", 0, 5.0, a), ("b", 6, 5.0, near)])["candidate_split_links"], 1)
        self.assertEqual(self.run_screen([("a", 0, 5.0, a), ("c", 6, 5.0, far)])["candidate_split_links"], 0)
        self.assertEqual(self.run_screen([("a", 0, 5.0, a), ("d", 20, 5.0, near)])["candidate_split_links"], 0)

    def test_spurious_counts_observations_no_lasting_track_took(self):
        rows = [(0, 0, 8, 4.5, 1.8), (30, 0, 8, 4.5, 1.8)]
        got = self.run_screen([("m", 0, 5.0, rows)], extra=3)
        self.assertEqual(got["observations"], 5)
        self.assertEqual(got["spurious_clusters_per_frame"], round(3 / got["frames_with_observations"], 4))


if __name__ == "__main__":
    unittest.main()
