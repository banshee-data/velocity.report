"""Cover the corpus check, whose whole job is to notice what a replay would not.

The interesting cases are the ones that look fine: a file that is present but
short, and a digest that no longer matches. Both pass an existence check and
then fail an hour into a replay.
"""

import hashlib
import importlib.util
import json
import os
import sys
import tempfile
import unittest
from unittest import mock

HERE = os.path.dirname(os.path.abspath(__file__))

spec = importlib.util.spec_from_file_location(
    "verify_corpus", os.path.join(HERE, "verify-corpus.py")
)
verify_corpus = importlib.util.module_from_spec(spec)
sys.modules["verify_corpus"] = verify_corpus
spec.loader.exec_module(verify_corpus)


class VerifyCorpusTests(unittest.TestCase):
    def setUp(self):
        held = tempfile.TemporaryDirectory()
        self.addCleanup(held.cleanup)
        self.root = held.name
        os.makedirs(os.path.join(self.root, "raw", "lidar"))

    def materialise(self, name, payload=b"packets"):
        relative = os.path.join("raw", "lidar", name)
        with open(os.path.join(self.root, relative), "wb") as fh:
            fh.write(payload)
        return relative

    def manifest(self, entries):
        with open(os.path.join(self.root, verify_corpus.MANIFEST), "w") as fh:
            json.dump(entries, fh)

    def run_verify(self, sha=""):
        with mock.patch.dict(
            os.environ, {"S2_CORPUS_DIR": self.root, "SHA": sha}, clear=False
        ):
            return verify_corpus.main()

    def entry(self, slug, relative, payload=b"packets", **overrides):
        record = {
            "sensor_type": "lidar",
            "site_slug": slug,
            "raw_path": relative,
            "raw_bytes": len(payload),
            "raw_sha256": "sha256:" + hashlib.sha256(payload).hexdigest(),
        }
        record.update(overrides)
        return record

    def test_an_intact_corpus_passes(self):
        relative = self.materialise("a.pcapng")
        self.manifest([self.entry("laguna-eddy", relative)])
        self.assertEqual(self.run_verify(), 0)

    def test_a_truncated_capture_fails_on_size_alone(self):
        relative = self.materialise("a.pcapng", payload=b"pac")
        self.manifest([self.entry("laguna-eddy", relative, payload=b"packets")])
        self.assertEqual(self.run_verify(), 1)

    def test_a_corrupted_capture_of_the_right_size_needs_the_digest(self):
        relative = self.materialise("a.pcapng", payload=b"PACKETS")
        self.manifest([self.entry("laguna-eddy", relative, payload=b"packets")])
        self.assertEqual(self.run_verify(), 0, "size alone cannot see this")
        self.assertEqual(self.run_verify(sha="1"), 1)

    def test_a_missing_capture_fails(self):
        self.manifest([self.entry("laguna-eddy", "raw/lidar/gone.pcapng")])
        self.assertEqual(self.run_verify(), 1)

    def test_radar_captures_are_not_the_scene_corpus(self):
        self.manifest(
            [{"sensor_type": "radar", "site_slug": "laguna-eddy", "raw_path": "x"}]
        )
        self.assertEqual(self.run_verify(), 0)

    def test_a_missing_manifest_is_a_configuration_error_not_a_bad_corpus(self):
        self.assertEqual(self.run_verify(), 2)

    def test_an_unset_corpus_directory_is_a_configuration_error(self):
        with mock.patch.dict(os.environ, {"S2_CORPUS_DIR": ""}, clear=False):
            self.assertEqual(verify_corpus.main(), 2)


if __name__ == "__main__":
    unittest.main()
