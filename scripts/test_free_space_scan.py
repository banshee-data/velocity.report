"""Retention recommendations must preserve required evidence without a NAS."""

import os
from pathlib import Path
import subprocess
import unittest

SCRIPT = Path(__file__).with_name("free-space-scan.sh")


class LocalEvidencePolicyTests(unittest.TestCase):
    def recommend(self, path):
        # Arguments remain positional shell data, including spaces and metacharacters.
        result = subprocess.run(
            [
                "bash",
                "-c",
                'source "$1"; destination "$(classify "$2")" "$2"',
                "test",
                str(SCRIPT),
                path,
            ],
            env={**os.environ, "LIDAR_VOL": "/local/lidar"},
            text=True,
            capture_output=True,
            check=True,
        )
        return result.stdout.strip()

    def test_required_internal_files_go_to_local_evidence(self):
        for name in (
            "capture.pcap",
            "site.vrlog",
            "campaign-results",
            "gerardus",
            "roofs",
            "labels",
        ):
            with self.subTest(name=name):
                self.assertEqual(
                    self.recommend("/internal/" + name),
                    "/local/lidar/evidence (verify dependencies)",
                )

    def test_local_files_are_kept_regardless_of_name(self):
        for name in ("results", "capture.pcap", "cache", "unknown", "a b;$(false)"):
            with self.subTest(name=name):
                self.assertEqual(
                    self.recommend("/local/lidar/" + name),
                    "KEEP local copy; review retention",
                )

    def test_similar_path_prefix_is_not_the_local_volume(self):
        self.assertEqual(
            self.recommend("/local/lidar-other/results"),
            "/local/lidar/evidence (verify dependencies)",
        )

    def test_cache_name_does_not_authorise_deletion(self):
        self.assertEqual(
            self.recommend("/internal/cache"),
            "review local rebuild inputs before deletion",
        )

    def test_unknown_files_need_review(self):
        self.assertEqual(
            self.recommend("/internal/unknown"), "review ownership and dependencies"
        )


if __name__ == "__main__":
    unittest.main()
