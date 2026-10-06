"""Synthetic tests for the replacement review-queue extractor."""

import importlib.util
from pathlib import Path
import sqlite3
import unittest

spec = importlib.util.spec_from_file_location(
    "jump_candidates", Path(__file__).with_name("lidar-jump-candidates.py")
)
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class JumpCandidatesTest(unittest.TestCase):
    def test_known_lateral_excursion_and_stationary_abstention(self):
        points = [(i * 100_000_000, i, 1 if i == 2 else 0) for i in range(5)]
        self.assertAlmostEqual(module.lateral_residual(points), 0.8)
        self.assertIsNone(module.lateral_residual([(0, 0, 0)] * 5))
        self.assertIsNone(module.lateral_residual([(i, 0, 0) for i in range(5)]))

    def test_repeatable_population_and_gap_exclusion(self):
        with sqlite3.connect(":memory:") as conn:
            conn.executescript(
                "CREATE TABLE lidar_tracks(track_id TEXT,sensor_id TEXT,max_speed_mps REAL);"
                "CREATE TABLE lidar_track_observations(track_id TEXT,ts_unix_nanos INTEGER,x REAL,y REAL);"
            )
            for name, speed, gap in [
                ("jump", 10, 100_000_000),
                ("slow", 1, 100_000_000),
                ("gap", 10, 1_000_000_000),
            ]:
                conn.execute(
                    "INSERT INTO lidar_tracks VALUES (?,?,?)", (name, "site", speed)
                )
                conn.executemany(
                    "INSERT INTO lidar_track_observations VALUES (?,?,?,?)",
                    [(name, i * gap, i, 1 if i == 2 else 0) for i in range(5)],
                )
            a = module.extract(conn)
            self.assertEqual(a, module.extract(conn))
            self.assertEqual(a["input_rows"], 10)
            self.assertEqual(a["candidate_count"], 1)
            self.assertEqual(a["candidates"][0]["track_id"], "jump")
            self.assertEqual(a["candidates"][0]["peak_timestamp_ns"], 200_000_000)
            conn.execute(
                "UPDATE lidar_track_observations SET y=0 WHERE track_id='jump'"
            )
            b = module.extract(conn)
            self.assertEqual(b["candidate_count"], 0)
            self.assertNotEqual(a["input_rows_sha256"], b["input_rows_sha256"])


if __name__ == "__main__":
    unittest.main()
