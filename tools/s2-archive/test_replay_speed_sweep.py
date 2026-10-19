"""Cover the replay speed sweep's comparisons without a server or a capture.

The sweep's verdicts rest on two things it computes itself: whether two scenes
agree row for row although their track ids are drawn at random per run, and
which capture a case names. These tests pin both.
"""

import gzip
import importlib.util
import json
import os
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))

# The script is named with a hyphen, so it is not importable by name.
spec = importlib.util.spec_from_file_location(
    "replay_speed_sweep", os.path.join(HERE, "replay-speed-sweep.py")
)
sweep = importlib.util.module_from_spec(spec)
sys.modules["replay_speed_sweep"] = sweep
spec.loader.exec_module(sweep)


def write_scene(root, frames, start_ns=1_000_000_000):
    os.makedirs(os.path.join(root, "frames"))
    with open(os.path.join(root, "header.json"), "w") as f:
        json.dump({"start_ns": str(start_ns)}, f)
    with gzip.open(os.path.join(root, "frames", "chunk_0000.ndjson.gz"), "wt") as f:
        for number, tracks in frames:
            # t in microseconds from the scene's start; f a server counter.
            f.write(
                json.dumps({"f": 9000 + number, "t": number * 100_000, "tr": tracks})
                + "\n"
            )


def row(track_id, x, y, c="car"):
    return {
        "id": track_id,
        "x": x,
        "y": y,
        "z": -1.5,
        "vx": 1.0,
        "vy": 0.0,
        "l": 4.2,
        "w": 1.8,
        "h": 1.4,
        "bh": 0.0,
        "c": c,
    }


class CompareTest(unittest.TestCase):
    def compare(self, a, b):
        with tempfile.TemporaryDirectory() as tmp:
            write_scene(os.path.join(tmp, "a"), a)
            write_scene(os.path.join(tmp, "b"), b)
            return sweep.compare(os.path.join(tmp, "a"), os.path.join(tmp, "b"))

    def test_the_same_rows_under_other_ids_are_identical(self):
        a = [(1, [row("1a", 1, 2), row("1b", 5, 6)]), (3, [row("1a", 2, 2)])]
        b = [(1, [row("zz", 1, 2), row("q", 5, 6)]), (3, [row("zz", 2, 2)])]
        result = self.compare(a, b)
        self.assertTrue(result["identical"])
        self.assertEqual(result["tracks_mapped"], 2)
        self.assertEqual(result["rows_matched"], 3)

    def test_scenes_that_start_a_frame_apart_compare_on_capture_time(self):
        # B's scene starts 100 ms later, so its t and f are offset; the rows agree.
        a = [(1, [row("1a", 1, 2)]), (2, [row("1a", 2, 2)])]
        b = [(1, [row("zz", 2, 2)])]
        with tempfile.TemporaryDirectory() as tmp:
            write_scene(os.path.join(tmp, "a"), a, start_ns=1_000_000_000)
            write_scene(os.path.join(tmp, "b"), b, start_ns=1_100_000_000)
            result = sweep.compare(os.path.join(tmp, "a"), os.path.join(tmp, "b"))
        self.assertEqual(result["frames_common"], 1)
        self.assertEqual(result["frames_differing"], 0)

    def test_a_moved_row_is_a_difference(self):
        a = [(1, [row("1a", 1, 2)]), (3, [row("1a", 2, 2)])]
        b = [(1, [row("zz", 1, 2)]), (3, [row("zz", 2.01, 2)])]
        result = self.compare(a, b)
        self.assertFalse(result["identical"])
        self.assertEqual(result["frames_differing"], 1)
        self.assertEqual(result["rows_matched"], 1)

    def test_one_id_split_in_two_is_a_difference(self):
        # The same rows, but B ends the track and starts another: identity differs.
        a = [(1, [row("1a", 1, 2)]), (3, [row("1a", 2, 2)])]
        b = [(1, [row("p", 1, 2)]), (3, [row("r", 2, 2)])]
        result = self.compare(a, b)
        self.assertFalse(result["identical"])

    def test_a_frame_only_one_scene_holds_is_counted(self):
        a = [(1, [row("1a", 1, 2)]), (3, [])]
        b = [(1, [row("zz", 1, 2)])]
        result = self.compare(a, b)
        self.assertEqual(result["frames_only_a"], 1)
        self.assertTrue(result["identical"])


class CorpusCaptureTest(unittest.TestCase):
    def test_the_case_names_its_raw_lidar_capture(self):
        with tempfile.TemporaryDirectory() as tmp:
            with open(os.path.join(tmp, "manifest.json"), "w") as f:
                json.dump(
                    [
                        {
                            "site_slug": "laguna-eddy",
                            "raw_path": "raw/radar/x.json",
                            "duration_seconds": 1,
                        },
                        {
                            "site_slug": "laguna-eddy",
                            "raw_path": "raw/lidar/20260901T1200_8-0_laguna-eddy.pcapng",
                            "duration_seconds": 612.5,
                        },
                    ],
                    f,
                )
            path, seconds = sweep.corpus_capture(tmp, "laguna-eddy")
            self.assertEqual(path, "raw/lidar/20260901T1200_8-0_laguna-eddy.pcapng")
            self.assertEqual(seconds, 612.5)
            with self.assertRaises(SystemExit):
                sweep.corpus_capture(tmp, "fulton-divisadero")

    def test_every_arm_is_the_spec_s_or_the_settling_diagnostic(self):
        self.assertEqual(
            set(sweep.ARMS),
            {
                "analysis",
                "analysis-replay-l3",
                "x0.25",
                "x0.5",
                "x0.5-replay-l3",
                "x1",
                "x2",
                "x4",
                "analysis-settle",
            },
        )
        # Only the two replay-l3 arms run the once-substituted L3, and they do it
        # through their tuning file: no arm asks the server for anything else.
        for name, (_, _, tuning) in sweep.ARMS.items():
            self.assertEqual(
                tuning == sweep.REPLAY_L3, name.endswith("replay-l3"), name
            )
        # Only the settling diagnostic settles first.
        self.assertEqual(sweep.SETTLE_ARMS, {"analysis-settle"})


if __name__ == "__main__":
    unittest.main()
