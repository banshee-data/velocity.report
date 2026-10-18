"""Exercise the perf A/B comparator on synthetic lidar-bench documents."""

import json
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
import perf_ab_compare as ab  # noqa: E402


def doc(wall, avg=6.0, p95=24.0, fingerprint="aaaa", profile="full", clusters=2863):
    return {
        "profile": profile,
        "pcap_file": "kirk0.pcapng",
        "tuning_fingerprint": fingerprint,
        "metrics": {
            "wall_clock_ms": wall,
            "frame_time_stats": {"avg_ms": avg, "p95_ms": p95},
            "work": {
                "frames": 832,
                "foreground_points": 985224,
                "clusters": clusters,
                "distinct_confirmed_tracks": 77,
            },
        },
    }


class PerfABCompareTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)

    def write_rounds(self, profile, rounds):
        d = self.root / profile
        d.mkdir(parents=True, exist_ok=True)
        for i, (base, head) in enumerate(rounds):
            (d / f"{i:02d}-base.json").write_text(json.dumps(base))
            (d / f"{i:02d}-head.json").write_text(json.dumps(head))

    def run_main(self, threshold=0.10):
        return ab.main([str(self.root), "--threshold", str(threshold)])

    def test_unchanged_code_passes(self):
        self.write_rounds("full", [(doc(7700), doc(7750)), (doc(7800), doc(7700))])
        self.assertEqual(self.run_main(), 0)

    def test_a_slower_head_fails(self):
        self.write_rounds("full", [(doc(7700), doc(9000)), (doc(7800), doc(9100))])
        self.assertEqual(self.run_main(), 1)

    # The case a committed baseline cannot see: the change moves the tuning
    # fingerprint and the work it does. A/B still compares what it costs.
    def test_a_tuning_change_is_compared_not_refused(self):
        self.write_rounds(
            "full",
            [
                (doc(7700), doc(9000, fingerprint="bbbb", clusters=3100)),
                (doc(7800), doc(9100, fingerprint="bbbb", clusters=3100)),
            ],
        )
        self.assertEqual(self.run_main(), 1)
        result = ab.compare_profile("full", ab.load_pairs(self.root / "full"), 0.10)
        self.assertEqual(result["tuning"], ("aaaa", "bbbb"))

    # A noisy runner slows every run in a round together. The per-round
    # ratio cancels it; comparing each side's median alone would not.
    def test_drift_shared_within_a_round_cancels(self):
        self.write_rounds(
            "full",
            [
                (doc(7000), doc(7100)),
                (doc(11000), doc(11200)),
                (doc(11500), doc(11600)),
            ],
        )
        result = ab.compare_profile("full", ab.load_pairs(self.root / "full"), 0.10)
        wall = next(r for r in result["rows"] if r["key"] == "wall_clock_ms")
        self.assertLess(wall["ratio"], 1.02)
        self.assertFalse(result["regressed"])

    def test_every_profile_is_reported_and_any_regression_fails(self):
        self.write_rounds("full", [(doc(7700), doc(7700))])
        self.write_rounds(
            "l3-only",
            [(doc(4800, profile="l3-only"), doc(6000, profile="l3-only"))],
        )
        self.assertEqual(self.run_main(), 1)

    def test_a_different_capture_cannot_be_compared(self):
        head = doc(7700)
        head["pcap_file"] = "soma1.pcapng"
        self.write_rounds("full", [(doc(7700), head)])
        self.assertEqual(self.run_main(), 2)

    def test_a_missing_head_run_cannot_be_compared(self):
        d = self.root / "full"
        d.mkdir()
        (d / "00-base.json").write_text(json.dumps(doc(7700)))
        self.assertEqual(self.run_main(), 2)

    def test_no_runs_cannot_be_compared(self):
        self.assertEqual(self.run_main(), 2)

    def test_summary_is_written(self):
        self.write_rounds("full", [(doc(7700), doc(7750))])
        summary = self.root.parent / f"{self.root.name}-summary.md"
        self.addCleanup(lambda: summary.unlink(missing_ok=True))
        self.assertEqual(ab.main([str(self.root), "--summary", str(summary)]), 0)
        text = summary.read_text()
        self.assertIn("full: ✅ within threshold", text)
        self.assertIn("| wall clock (ms) |", text)


if __name__ == "__main__":
    unittest.main()
