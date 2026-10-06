"""Synthetic tests for the following-window review queue."""

import contextlib
import importlib.util
import io
import json
from pathlib import Path
import sqlite3
import tempfile
import unittest

spec = importlib.util.spec_from_file_location(
    "following_windows", Path(__file__).with_name("lidar-following-windows.py")
)
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)

PERIOD = 100_000_000  # 10 Hz
# 2026-09-03T20:18:00Z, a window boundary, one minute into a capture that
# started at 13:17:00 local (UTC-7).
BASE = 1788466680 * 1_000_000_000


def estimate_db(rows, path=":memory:"):
    conn = sqlite3.connect(path)
    conn.execute(
        "CREATE TABLE lidar_track_estimates(estimate_id TEXT, creation_sequence INTEGER, "
        "frame_unix_nanos INTEGER, x REAL, y REAL, vx REAL, vy REAL, stage TEXT, source_id TEXT)"
    )
    conn.executemany(
        "INSERT INTO lidar_track_estimates VALUES (?,?,?,?,?,?,?,?,?)",
        [
            (f"e{i}", seq, t, x, y, vx, vy, stage, source)
            for i, (seq, t, x, y, vx, vy, stage, source) in enumerate(rows)
        ],
    )
    conn.commit()
    return conn


def moving(seq, frames, x0, y=0.0, speed=10.0, first=0, stage="online", source="s"):
    """A track driving along +x at speed from x0, one row per frame."""
    return [
        (
            seq,
            BASE + (first + f) * PERIOD,
            x0 + speed * f * 0.1,
            y,
            speed,
            0.0,
            stage,
            source,
        )
        for f in range(frames)
    ]


def pairing():
    return dict(min_speed=3, max_heading_deg=20, min_gap=3, max_gap=40, max_lateral=2.5)


class FollowingWindowsTest(unittest.TestCase):
    def ranked(self, rows):
        estimates, _, _ = module.load_estimates(estimate_db(rows), "online", None)
        return module.rank_windows(estimates, 10, **pairing())

    def test_one_vehicle_following_another(self):
        ranked, period = self.ranked(moving(1, 30, 0) + moving(2, 30, 10))
        self.assertAlmostEqual(period, 0.1)
        self.assertEqual(len(ranked), 1)
        w = ranked[0]
        self.assertEqual(w["window_start_unix_nanos"], BASE)
        self.assertEqual(w["pair_seconds"], 3.0)
        self.assertEqual((w["pairs"], w["followers"], w["leaders"]), (1, 1, 1))
        self.assertEqual(w["leader_changes"], 0)
        self.assertEqual(w["closest_gap_m"], 10.0)

    def test_a_split_leader_reads_as_leader_changes(self):
        # One lead vehicle, tracked as 2 and then as 3: the follower's nearest
        # leader changes once.
        rows = moving(1, 20, 0) + moving(2, 10, 10) + moving(3, 10, 20, first=10)
        w = self.ranked(rows)[0][0]
        self.assertEqual((w["pairs"], w["leaders"], w["leader_changes"]), (2, 2, 1))
        self.assertEqual(w["leader_seqs"], [2, 3])

    def test_what_is_not_following(self):
        rows = moving(1, 10, 0)
        rows += moving(2, 10, 20, y=4.0)  # beside, not ahead
        rows += moving(3, 10, 60)  # beyond the gap bound
        rows += moving(4, 10, 1.5, y=-1.0)  # closer than the minimum gap
        rows += [
            (5, t, x + 10, 0.0, -10.0, 0.0, s, src)
            for (_, t, x, _, _, _, s, src) in moving(5, 10, 0)
        ]  # oncoming
        rows += [
            (6, t, 10.0, 0.0, 0.0, 0.0, "online", "s")
            for (_, t, *_) in moving(6, 10, 0)
        ]  # stopped
        ranked, _ = self.ranked(rows)
        self.assertEqual(ranked, [])

    def test_the_busiest_window_ranks_first(self):
        quiet = moving(1, 5, 0) + moving(2, 5, 10)
        busy = (
            moving(3, 30, 0, first=100)
            + moving(4, 30, 10, first=100)
            + moving(5, 30, 25, first=100)
        )
        ranked, _ = self.ranked(quiet + busy)
        self.assertEqual(
            [w["window_start_unix_nanos"] for w in ranked], [BASE + 10 * 10**9, BASE]
        )

    def test_ranking_uses_exact_counts_not_rounded_seconds(self):
        # At 250 Hz, 2 and 3 pair-frames both round to 0.01 pair-seconds.
        # The later window has more following and must still rank first.
        fast = 4_000_000
        rows = []
        for start, frames in ((0, 2), (10 * 10**9, 3)):
            for f in range(frames):
                t = BASE + start + f * fast
                rows.append((1, t, 0.0, 0.0, 10.0, 0.0, "online", "s"))
                rows.append((2, t, 10.0, 0.0, 10.0, 0.0, "online", "s"))
        ranked, period = self.ranked(rows)
        self.assertAlmostEqual(period, 0.004)
        self.assertEqual([w["pair_seconds"] for w in ranked], [0.01, 0.01])
        self.assertEqual([w["pair_frames"] for w in ranked], [3, 2])

    def test_several_sources_need_one_named(self):
        conn = estimate_db(moving(1, 5, 0, source="a") + moving(2, 5, 0, source="b"))
        with self.assertRaises(SystemExit) as refused:
            module.load_estimates(conn, "online", None)
        self.assertIn("pass --source", str(refused.exception))
        estimates, source, _ = module.load_estimates(conn, "online", "b")
        self.assertEqual(source, "b")
        self.assertEqual({row[0] for rows in estimates.values() for row in rows}, {2})
        with self.assertRaises(SystemExit):
            module.load_estimates(conn, "final", None)

    def test_windows_are_placed_in_their_capture(self):
        site_index = [
            {
                "id": "embarcadero-folsom",
                "start": "2026-09-03T13:17:48.1-07:00",
                "captures": [
                    "s2_sf_7_20260903131700_00001.pcap",
                    "s2_sf_7_20260903132200_00002.pcap",
                ],
            }
        ]
        starts = module.capture_starts(site_index, "embarcadero-folsom")
        self.assertEqual(
            module.place(BASE, starts), ("s2_sf_7_20260903131700_00001.pcap", 60.0)
        )
        self.assertEqual(
            module.place(BASE + 330 * 10**9, starts)[0],
            "s2_sf_7_20260903132200_00002.pcap",
        )
        self.assertIsNone(module.place(BASE - 120 * 10**9, starts))
        with self.assertRaises(SystemExit):
            module.capture_starts(site_index, "columbus-broadway")

    def test_command_line_writes_json_once(self):
        with tempfile.TemporaryDirectory() as tmp:
            db = Path(tmp) / "observations.db"
            estimate_db(moving(1, 30, 0) + moving(2, 30, 10), db).close()
            out = Path(tmp) / "windows.json"
            with contextlib.redirect_stdout(io.StringIO()) as printed:
                self.assertEqual(module.main([str(db), "--json", str(out)]), 0)
            self.assertIn("pair-s", printed.getvalue())
            report = json.loads(out.read_text())
            self.assertEqual(report["schema"], module.SCHEMA)
            self.assertEqual(
                report["status"], "unreviewed_candidates_from_tracker_estimates"
            )
            self.assertEqual(len(report["windows"]), 1)
            with self.assertRaises(FileExistsError), contextlib.redirect_stdout(
                io.StringIO()
            ):
                module.main([str(db), "--json", str(out)])


if __name__ == "__main__":
    unittest.main()
