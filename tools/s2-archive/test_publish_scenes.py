"""Cover how a scene is resolved to packets, which is the part that can be wrong quietly.

A replay that names the wrong file still runs; it just publishes the wrong
junction. These tests pin the join between the site index and the corpus
manifest, and the path arithmetic the replay API needs.
"""

import contextlib
import importlib.util
import io
import json
import os
import sys
import tempfile
import unittest
from unittest import mock

HERE = os.path.dirname(os.path.abspath(__file__))

# The script is named with a hyphen, so it is not importable by name.
spec = importlib.util.spec_from_file_location(
    "publish_scenes", os.path.join(HERE, "publish-scenes.py")
)
publish_scenes = importlib.util.module_from_spec(spec)
sys.modules["publish_scenes"] = publish_scenes
spec.loader.exec_module(publish_scenes)


def site(site_id, where="Somewhere", captures=("cap_20260901120000_00001.pcap",)):
    return {
        "id": site_id,
        "site": "s01",
        "where": where,
        "start": "2026-09-01T12:05:00-07:00",
        "minutes": 20.0,
        "captures": list(captures),
    }


def capture(slug, path, seconds=1200.0):
    return {
        "sensor_type": "lidar",
        "site_slug": slug,
        "raw_path": path,
        "duration_seconds": seconds,
    }


class ReplayRelativeTests(unittest.TestCase):
    def test_path_inside_the_replay_directory_is_made_relative(self):
        self.assertEqual(
            publish_scenes.replay_relative("/vol/data/set/raw/a.pcapng", "/vol/data"),
            os.path.join("set", "raw", "a.pcapng"),
        )

    def test_path_outside_the_replay_directory_is_refused(self):
        self.assertIsNone(publish_scenes.replay_relative("/elsewhere/a.pcapng", "/vol"))

    def test_no_replay_directory_is_refused(self):
        self.assertIsNone(publish_scenes.replay_relative("/vol/a.pcapng", ""))


class LoadCorpusTests(unittest.TestCase):
    def setUp(self):
        held = tempfile.TemporaryDirectory()
        self.addCleanup(held.cleanup)
        self.root = held.name

    def write(self, entries):
        path = os.path.join(self.root, publish_scenes.CORPUS_MANIFEST)
        with open(path, "w") as fh:
            json.dump(entries, fh)
        return path

    def test_indexes_lidar_captures_by_slug(self):
        self.write(
            [
                capture("laguna-eddy", "raw/lidar/a.pcapng"),
                {"sensor_type": "radar", "site_slug": "laguna-eddy"},
            ]
        )
        corpus = publish_scenes.load_corpus(self.root)
        self.assertEqual(list(corpus), ["laguna-eddy"])
        self.assertEqual(corpus["laguna-eddy"]["raw_path"], "raw/lidar/a.pcapng")

    def test_two_captures_for_one_site_are_refused_rather_than_picked_between(self):
        self.write(
            [
                capture("laguna-eddy", "raw/lidar/a.pcapng"),
                capture("laguna-eddy", "raw/lidar/b.pcapng"),
            ]
        )
        with self.assertRaisesRegex(RuntimeError, "appears twice"):
            publish_scenes.load_corpus(self.root)


class ScenesFromCorpusTests(unittest.TestCase):
    def setUp(self):
        held = tempfile.TemporaryDirectory()
        self.addCleanup(held.cleanup)
        self.pcap_dir = held.name
        self.corpus_dir = os.path.join(self.pcap_dir, "dataset")
        os.makedirs(os.path.join(self.corpus_dir, "raw", "lidar"))

    def materialise(self, name):
        path = os.path.join(self.corpus_dir, "raw", "lidar", name)
        with open(path, "wb") as fh:
            fh.write(b"\0")
        return os.path.join("raw", "lidar", name)

    def test_a_site_becomes_one_file_replayed_whole(self):
        relative = self.materialise("a.pcapng")
        scenes, problems = publish_scenes.scenes_from_corpus(
            [site("laguna-eddy", where="Laguna at Eddy")],
            {"laguna-eddy": capture("laguna-eddy", relative, seconds=1308.0)},
            self.corpus_dir,
            self.pcap_dir,
        )
        self.assertEqual(problems, [])
        (scene,) = scenes
        self.assertEqual(scene["files"], [os.path.join("dataset", relative)])
        self.assertEqual(scene["start_secs"], 0.0)
        self.assertEqual(scene["duration"], 1308.0)
        self.assertEqual(scene["title"], "Laguna at Eddy")
        self.assertEqual(scene["source"], "corpus")

    def test_the_manifest_duration_wins_over_the_index_minutes(self):
        relative = self.materialise("a.pcapng")
        scenes, _ = publish_scenes.scenes_from_corpus(
            [site("laguna-eddy")],  # the index says 20.0 minutes
            {"laguna-eddy": capture("laguna-eddy", relative, seconds=1308.0)},
            self.corpus_dir,
            self.pcap_dir,
        )
        self.assertAlmostEqual(scenes[0]["minutes"], 21.8)

    def test_a_site_missing_from_the_corpus_is_reported_not_skipped(self):
        scenes, problems = publish_scenes.scenes_from_corpus(
            [site("laguna-eddy")], {}, self.corpus_dir, self.pcap_dir
        )
        self.assertEqual(scenes, [])
        self.assertIn("not in the corpus manifest", problems[0])

    def test_a_capture_the_manifest_names_but_disk_lacks_is_reported(self):
        scenes, problems = publish_scenes.scenes_from_corpus(
            [site("laguna-eddy")],
            {"laguna-eddy": capture("laguna-eddy", "raw/lidar/gone.pcapng")},
            self.corpus_dir,
            self.pcap_dir,
        )
        self.assertEqual(scenes, [])
        self.assertIn("is not on disk", problems[0])

    def test_a_corpus_outside_the_replay_directory_is_reported(self):
        relative = self.materialise("a.pcapng")
        scenes, problems = publish_scenes.scenes_from_corpus(
            [site("laguna-eddy")],
            {"laguna-eddy": capture("laguna-eddy", relative)},
            self.corpus_dir,
            os.path.join(self.pcap_dir, "somewhere-else"),
        )
        self.assertEqual(scenes, [])
        self.assertIn("outside the replay directory", problems[0])

    def test_a_manifest_without_a_duration_is_reported(self):
        relative = self.materialise("a.pcapng")
        scenes, problems = publish_scenes.scenes_from_corpus(
            [site("laguna-eddy")],
            {"laguna-eddy": capture("laguna-eddy", relative, seconds=0)},
            self.corpus_dir,
            self.pcap_dir,
        )
        self.assertEqual(scenes, [])
        self.assertIn("no duration", problems[0])


class ScenesFromArchiveTests(unittest.TestCase):
    def setUp(self):
        held = tempfile.TemporaryDirectory()
        self.addCleanup(held.cleanup)
        self.pcap_dir = held.name
        os.makedirs(os.path.join(self.pcap_dir, publish_scenes.PCAP_SUBDIR))

    def materialise(self, *names):
        for name in names:
            path = os.path.join(self.pcap_dir, publish_scenes.PCAP_SUBDIR, name)
            with open(path, "wb") as fh:
                fh.write(b"\0")

    def test_the_start_offset_comes_from_the_first_capture_stamp(self):
        self.materialise("cap_20260901120000_00001.pcap")
        scenes, problems = publish_scenes.scenes_from_archive(
            [site("laguna-eddy")], self.pcap_dir
        )
        self.assertEqual(problems, [])
        # The site starts at 12:05 and the capture at 12:00.
        self.assertEqual(scenes[0]["start_secs"], 300.0)
        self.assertEqual(scenes[0]["source"], "archive")

    def test_an_unstamped_capture_starts_where_the_file_does(self):
        self.materialise("one-off.pcapng")
        scenes, _ = publish_scenes.scenes_from_archive(
            [site("laguna-eddy", captures=("one-off.pcapng",))], self.pcap_dir
        )
        self.assertEqual(scenes[0]["start_secs"], 0.0)

    def test_a_capture_that_is_not_on_disk_is_reported(self):
        scenes, problems = publish_scenes.scenes_from_archive(
            [site("laguna-eddy")], self.pcap_dir
        )
        self.assertEqual(scenes, [])
        self.assertIn("not on disk", problems[0])


class ReportStatusTests(unittest.TestCase):
    """The summary an operator reads before committing an evening to a rebuild."""

    def setUp(self):
        held = tempfile.TemporaryDirectory()
        self.addCleanup(held.cleanup)
        self.scenes_dir = held.name
        patched = mock.patch.object(publish_scenes, "SCENES", self.scenes_dir)
        patched.start()
        self.addCleanup(patched.stop)

    def mark(self, site_id):
        os.makedirs(os.path.join(self.scenes_dir, site_id), exist_ok=True)
        open(os.path.join(self.scenes_dir, site_id, ".rebuilt"), "w").close()

    def scene(self, site_id, minutes=20.0):
        return {"site": site_id, "minutes": minutes}

    def capture_report(self, scenes, problems):
        buffer = io.StringIO()
        with contextlib.redirect_stdout(buffer):
            code = publish_scenes.report_status(scenes, problems)
        return code, buffer.getvalue()

    def test_a_marked_scene_reads_as_published(self):
        self.mark("laguna-eddy")
        code, text = self.capture_report([self.scene("laguna-eddy")], [])
        self.assertEqual(code, 0)
        self.assertIn("published", text)
        self.assertIn("1 published, 0 outstanding", text)
        self.assertNotIn("of replay", text)

    def test_an_unmarked_scene_is_outstanding_and_carries_an_estimate(self):
        code, text = self.capture_report([self.scene("laguna-eddy", 20.0)], [])
        self.assertEqual(code, 0)
        self.assertIn("OUTSTANDING", text)
        self.assertIn("0 published, 1 outstanding", text)
        self.assertIn("about 0.7 h of replay", text)

    def test_an_unresolved_site_makes_the_status_fail(self):
        self.mark("laguna-eddy")
        code, text = self.capture_report(
            [self.scene("laguna-eddy")], ["howard-6th: not in the corpus manifest"]
        )
        self.assertEqual(code, 1)
        self.assertIn("UNRESOLVED", text)
        self.assertIn("1 unresolved", text)


class CarryOverTests(unittest.TestCase):
    """A rebuild replaces assets/ whole; what was chosen by hand must survive it."""

    def setUp(self):
        held = tempfile.TemporaryDirectory()
        self.addCleanup(held.cleanup)
        self.live = os.path.join(held.name, "assets")
        self.assets = os.path.join(held.name, "assets.new")
        os.makedirs(self.live)
        os.makedirs(self.assets)
        patched = mock.patch.object(publish_scenes, "export")
        self.export = patched.start()
        self.addCleanup(patched.stop)

    def write(self, relative, content):
        path = os.path.join(self.live, relative)
        os.makedirs(os.path.dirname(path), exist_ok=True)
        with open(path, "w") as fh:
            fh.write(content)
        return content

    def carry_over(self):
        publish_scenes.carry_over(
            "run.vrlog", self.live, self.assets, "columbus-broadway", "Columbus"
        )

    def read(self, relative):
        with open(os.path.join(self.assets, relative)) as fh:
            return fh.read()

    def test_a_clip_is_exported_again_over_its_recorded_frames(self):
        manifest = self.write(
            "clip/manifest.json",
            json.dumps(
                {
                    "fade_out_seconds": 5,
                    "selection": {"source_start_frame": 400, "source_frame_count": 301},
                }
            ),
        )
        self.carry_over()
        self.export.assert_called_once_with(
            "run.vrlog",
            os.path.join(self.assets, "clip", "part-000"),
            "columbus-broadway",
            "Columbus",
            kind="clip",
            extra=[
                "--start-frame",
                "400",
                "--frame-count",
                "301",
                "--max-points",
                "1200",
                "--chunk-seconds",
                "10",
            ],
        )
        # The manifest records the selection, so it is kept, not remade.
        self.assertEqual(self.read("clip/manifest.json"), manifest)

    def test_vantages_are_kept_as_they_are(self):
        vantages = self.write("vantages.json", '{"vantages": []}\n')
        self.carry_over()
        self.assertEqual(self.read("vantages.json"), vantages)
        self.export.assert_not_called()

    def test_a_scene_without_a_clip_does_not_gain_one(self):
        self.carry_over()
        self.export.assert_not_called()
        self.assertEqual(os.listdir(self.assets), [])


class PlanTests(unittest.TestCase):
    def test_an_unknown_site_name_stops_the_run(self):
        with self.assertRaisesRegex(SystemExit, "not in site-index.json"):
            publish_scenes.plan("archive", ["not-a-site"], "", "")

    def test_the_corpus_source_needs_a_corpus(self):
        with self.assertRaisesRegex(SystemExit, "S2_CORPUS_DIR"):
            publish_scenes.plan("corpus", [], "", "/vol")

    def test_a_corpus_directory_without_a_manifest_says_so(self):
        with tempfile.TemporaryDirectory() as empty:
            with self.assertRaisesRegex(SystemExit, "manifest.json"):
                publish_scenes.plan("corpus", [], empty, "/vol")


if __name__ == "__main__":
    unittest.main()
