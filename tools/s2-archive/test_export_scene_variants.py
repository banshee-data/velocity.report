"""Cover the scene variant exporter without a server, a corpus or a binary.

What can go wrong quietly here is a set that runs but compares nothing: a
variant that ran the previous variant's parameters, a file the server did not
apply, two exports of the same parameters, or a window that is not the one
asked for. These tests pin the checks that refuse each of those.
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
    "export_scene_variants", os.path.join(HERE, "export-scene-variants.py")
)
variants = importlib.util.module_from_spec(spec)
sys.modules["export_scene_variants"] = variants
spec.loader.exec_module(variants)
publish = variants.publish


def a_set(**overrides):
    raw = {
        "version": 1,
        "set_id": "pair",
        "baseline": "defaults",
        "variants": [
            {
                "id": "defaults",
                "label": "Defaults",
                "tuning_config": "tuning.defaults.json",
            },
            {"id": "a2", "tuning_config": "experiments/solid-body-a2.json"},
        ],
        "sites": [
            {"site": "1st-mission", "offset_seconds": 100, "duration_seconds": 60}
        ],
    }
    raw.update(overrides)
    return raw


def a_scene(duration=1200.0, start=0.0):
    return {
        "site": "1st-mission",
        "title": "1st at Mission",
        "minutes": duration / 60.0,
        "files": ["sf-street-speeds/raw/1st-mission.pcapng"],
        "duration": duration,
        "start_secs": start,
        "source": "corpus",
    }


def a_listing(**by_path):
    configs = []
    for path, fields in by_path.items():
        entry = {
            "path": path,
            "kind": "tuning",
            "applicable": True,
            "fingerprint": f"fp-{path}",
        }
        entry.update(fields)
        configs.append(entry)
    return {
        "config_dir": "/repo/config",
        "configs": configs,
        "active": {"path": "", "fingerprint": "x"},
    }


class ValidateSetTests(unittest.TestCase):
    def test_a_well_formed_set_is_normalised(self):
        spec = variants.validate_set(a_set())
        self.assertEqual(spec["baseline"], "defaults")
        self.assertEqual([v["id"] for v in spec["variants"]], ["defaults", "a2"])
        # A variant with no label is labelled by its id.
        self.assertEqual(spec["variants"][1]["label"], "a2")
        self.assertEqual(spec["sites"][0]["offset_seconds"], 100)

    def test_a_variant_without_a_file_is_refused(self):
        # "The server's current parameters" is the previous variant's after the
        # first replay, so it cannot be a variant.
        raw = a_set(
            variants=[
                {"id": "defaults", "tuning_config": "tuning.defaults.json"},
                {"id": "current"},
            ]
        )
        with self.assertRaisesRegex(
            variants.SetError, "current.*tuning_config is empty"
        ):
            variants.validate_set(raw)

    def test_paths_the_server_would_refuse_are_refused_here(self):
        for path, reason in (
            ("/etc/tuning.json", "relative"),
            ("../tuning.json", "leaves the config directory"),
            ("experiments/../../x.json", "leaves the config directory"),
            ("experiments/a2.yaml", "not a .json file"),
        ):
            with self.subTest(path=path):
                raw = a_set(baseline="x", variants=[{"id": "x", "tuning_config": path}])
                with self.assertRaisesRegex(variants.SetError, reason):
                    variants.validate_set(raw)

    def test_a_path_is_cleaned_as_the_dropdown_names_it(self):
        raw = a_set(
            baseline="x",
            variants=[{"id": "x", "tuning_config": "experiments/./a2.json"}],
        )
        self.assertEqual(
            variants.validate_set(raw)["variants"][0]["tuning_config"],
            "experiments/a2.json",
        )

    def test_every_problem_is_named_at_once(self):
        raw = a_set(
            set_id="Not A Slug",
            baseline="missing",
            variants=[
                {"id": "a", "tuning_config": "a.json"},
                {"id": "a", "tuning_config": "b.json"},
            ],
            sites=[{"site": "s", "offset_seconds": 10}],
        )
        with self.assertRaises(variants.SetError) as caught:
            variants.validate_set(raw)
        message = str(caught.exception)
        for fragment in ("set_id", "used twice", "baseline", "give both"):
            self.assertIn(fragment, message)

    def test_a_site_may_be_named_alone_for_the_whole_capture(self):
        spec = variants.validate_set(a_set(sites=["1st-mission"]))
        self.assertIsNone(spec["sites"][0]["offset_seconds"])

    def test_the_set_file_digest_is_of_its_bytes(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "set.json")
            with open(path, "w") as fh:
                json.dump(a_set(), fh)
            first = variants.load_set(path)["digest"]
            with open(path, "a") as fh:
                fh.write("\n")
            self.assertNotEqual(first, variants.load_set(path)["digest"])

    def test_the_checked_in_sets_are_valid_and_name_real_files(self):
        root = os.path.join(HERE, "scene-variants")
        config_dir = os.path.join(variants.REPO, "config")
        names = [n for n in os.listdir(root) if n.endswith(".json")]
        self.assertTrue(names)
        for name in names:
            with self.subTest(set=name):
                spec = variants.load_set(os.path.join(root, name))
                for variant in spec["variants"]:
                    self.assertTrue(
                        os.path.isfile(
                            os.path.join(config_dir, variant["tuning_config"])
                        ),
                        variant["tuning_config"],
                    )


class WindowTests(unittest.TestCase):
    def test_a_window_is_replayed_from_its_warm_up(self):
        scene = variants.windowed(
            a_scene(), {"offset_seconds": 100, "duration_seconds": 60}
        )
        self.assertEqual(variants.WARMUP_SECONDS, 40.0)
        self.assertEqual(scene["start_secs"], 60.0)
        self.assertEqual(scene["duration"], 100.0)
        self.assertEqual(scene["window"]["warmup_seconds"], 40.0)

    def test_an_archive_scene_keeps_its_own_start_offset(self):
        scene = variants.windowed(
            a_scene(start=500.0), {"offset_seconds": 100, "duration_seconds": 60}
        )
        self.assertEqual(scene["start_secs"], 560.0)

    def test_a_window_too_early_for_its_warm_up_is_refused(self):
        with self.assertRaisesRegex(variants.SetError, "warm-up"):
            variants.windowed(a_scene(), {"offset_seconds": 20, "duration_seconds": 60})

    def test_a_window_past_the_capture_is_refused(self):
        with self.assertRaisesRegex(variants.SetError, "runs past"):
            variants.windowed(
                a_scene(duration=150.0), {"offset_seconds": 100, "duration_seconds": 60}
            )

    def test_no_window_is_the_whole_site(self):
        scene = variants.windowed(
            a_scene(), {"offset_seconds": None, "duration_seconds": None}
        )
        self.assertIsNone(scene["window"])
        self.assertEqual(scene["duration"], 1200.0)


class CheckConfigsTests(unittest.TestCase):
    def setUp(self):
        self.spec = variants.validate_set(a_set())

    def test_applicable_files_give_their_fingerprints(self):
        listing = a_listing(
            **{"tuning.defaults.json": {}, "experiments/solid-body-a2.json": {}}
        )
        problems, fingerprints = variants.check_configs(self.spec["variants"], listing)
        self.assertEqual(problems, [])
        self.assertEqual(fingerprints["a2"], "fp-experiments/solid-body-a2.json")

    def test_a_file_the_server_cannot_apply_is_refused_with_its_reason(self):
        listing = a_listing(
            **{
                "tuning.defaults.json": {},
                "experiments/solid-body-a2.json": {
                    "applicable": False,
                    "reason": "changes startup-only settings: l4.engine",
                },
            }
        )
        problems, fingerprints = variants.check_configs(self.spec["variants"], listing)
        self.assertEqual(len(problems), 1)
        self.assertIn("l4.engine", problems[0])
        self.assertNotIn("a2", fingerprints)

    def test_a_missing_file_and_a_non_tuning_file_are_refused(self):
        listing = a_listing(
            **{
                "experiments/solid-body-a2.json": {
                    "kind": "other",
                    "error": "unknown key",
                }
            }
        )
        problems, _ = variants.check_configs(self.spec["variants"], listing)
        self.assertEqual(len(problems), 2)
        self.assertIn("is not in /repo/config", problems[0])
        self.assertIn("not a tuning config", problems[1])


def fake_export(header):
    """An export that writes the files a real one does, with the given header."""

    def export(vrlog, out_dir, site, title, kind=None, extra=None):
        os.makedirs(out_dir, exist_ok=True)
        if kind == "background":
            open(os.path.join(out_dir, "background.json.gz"), "wb").close()
        else:
            with open(os.path.join(out_dir, "header.json"), "w") as fh:
                json.dump(dict(header, title=title), fh)
        return "exported\n  frames: 1"

    return export


class ExportVariantTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.vdir = os.path.join(self.tmp.name, "pair", "1st-mission", "a2")
        os.makedirs(self.vdir)
        self.variant = {
            "id": "a2",
            "label": "Solid body A2",
            "tuning_config": "experiments/a2.json",
        }
        self.header = {
            "tuning_hash": "sha256:aaa",
            "build_version": "0.5.1-pre50",
            "frame_count": 300,
            "duration_sec": 60.0,
            "source_vrlog_sha256": "bbb",
        }

    def tearDown(self):
        self.tmp.cleanup()

    def run_variant(self, start, export=None):
        ran = {"vrlog": "/vrlogs/run.vrlog", "run_id": "run-1", "start": start}
        with mock.patch.object(
            publish, "replay", return_value=ran
        ) as replay, mock.patch.object(
            publish, "export", side_effect=export or fake_export(self.header)
        ), contextlib.redirect_stdout(
            io.StringIO()
        ):
            record = variants.export_variant(
                a_scene(), self.variant, self.vdir, "fp-a2"
            )
        return record, replay

    def test_a_variant_is_replayed_under_its_file_and_written_as_a_published_scene(
        self,
    ):
        record, replay = self.run_variant(
            {"tuning_config": "experiments/a2.json", "tuning_fingerprint": "fp-a2"}
        )
        self.assertEqual(
            replay.call_args.kwargs["tuning_config"], "experiments/a2.json"
        )
        assets = os.path.join(self.vdir, "assets")
        with open(os.path.join(assets, "manifest.json")) as fh:
            manifest = json.load(fh)
        self.assertEqual(
            manifest["parts"], [{"url": "./part-000/", "start_seconds": 0}]
        )
        self.assertTrue(
            os.path.exists(os.path.join(assets, "background", "background.json.gz"))
        )
        self.assertFalse(os.path.exists(assets + ".new"))
        self.assertEqual(record["tuning_hash"], "sha256:aaa")
        self.assertEqual(record["applied_fingerprint"], "fp-a2")
        self.assertEqual(record["run_id"], "run-1")
        self.assertTrue(variants.is_done(self.vdir, "fp-a2"))

    def test_the_export_title_names_the_variant(self):
        self.run_variant(
            {"tuning_config": "experiments/a2.json", "tuning_fingerprint": "fp-a2"}
        )
        with open(os.path.join(self.vdir, "assets", "part-000", "header.json")) as fh:
            self.assertEqual(json.load(fh)["title"], "1st at Mission: Solid body A2")

    def test_a_fingerprint_that_changed_during_the_batch_fails_the_variant(self):
        with self.assertRaisesRegex(RuntimeError, "file changed during the batch"):
            self.run_variant(
                {"tuning_config": "experiments/a2.json", "tuning_fingerprint": "other"}
            )
        self.assertFalse(variants.is_done(self.vdir, "fp-a2"))

    def test_a_server_that_applied_another_file_fails_the_variant(self):
        with self.assertRaisesRegex(RuntimeError, "says it applied"):
            self.run_variant({"tuning_config": "", "tuning_fingerprint": "fp-a2"})

    def test_a_failed_export_leaves_the_last_good_assets_in_place(self):
        live = os.path.join(self.vdir, "assets")
        os.makedirs(live)
        with open(os.path.join(live, "manifest.json"), "w") as fh:
            fh.write("{}")

        def broken(*args, **kwargs):
            raise RuntimeError("export failed: disk full")

        with self.assertRaisesRegex(RuntimeError, "disk full"):
            self.run_variant(
                {"tuning_config": "experiments/a2.json", "tuning_fingerprint": "fp-a2"},
                broken,
            )
        with open(os.path.join(live, "manifest.json")) as fh:
            self.assertEqual(fh.read(), "{}")

    def test_a_marker_from_an_edited_file_is_not_done(self):
        self.run_variant(
            {"tuning_config": "experiments/a2.json", "tuning_fingerprint": "fp-a2"}
        )
        self.assertFalse(variants.is_done(self.vdir, "fp-a2-edited"))


class SetChecksTests(unittest.TestCase):
    def record(self, variant_id, tuning_hash, build="0.5.1-pre50"):
        return {"id": variant_id, "tuning_hash": tuning_hash, "build_version": build}

    def test_distinct_parameters_and_one_build_pass(self):
        records = {"s": [self.record("a", "h1"), self.record("b", "h2")]}
        self.assertEqual(variants.set_checks(records), [])

    def test_two_variants_with_one_tuning_hash_void_the_comparison(self):
        records = {"s": [self.record("a", "h1"), self.record("b", "h1")]}
        problems = variants.set_checks(records)
        self.assertEqual(len(problems), 1)
        self.assertIn("did not reach the pipeline", problems[0])

    def test_the_same_parameters_at_two_sites_is_not_a_duplicate(self):
        records = {"s": [self.record("a", "h1")], "t": [self.record("a", "h1")]}
        self.assertEqual(variants.set_checks(records), [])

    def test_variants_from_two_builds_are_reported(self):
        records = {
            "s": [self.record("a", "h1"), self.record("b", "h2", build="0.5.1-pre49")]
        }
        self.assertIn("different builds", variants.set_checks(records)[0])

    def test_an_export_without_a_tuning_hash_is_reported(self):
        records = {"s": [self.record("a", "")]}
        self.assertIn("no tuning_hash", variants.set_checks(records)[0])


class OutputAndRecipeTests(unittest.TestCase):
    def test_an_output_directory_inside_the_checkout_is_refused(self):
        self.assertIn(
            "inside the checkout",
            variants.out_root_problem(os.path.join(variants.REPO, "out")),
        )
        self.assertIn("SCENE_VARIANTS_DIR", variants.out_root_problem(""))
        with tempfile.TemporaryDirectory() as tmp:
            self.assertIsNone(variants.out_root_problem(tmp))

    def test_a_recipe_per_variant_changes_only_source_and_output(self):
        spec = variants.validate_set(a_set())
        base = {
            "version": 1,
            "source": "somewhere",
            "frame": {"frame_index": 120},
            "views": [{"name": "plan"}],
            "output": {"dir": "stills", "contact_sheet": True},
        }
        with tempfile.TemporaryDirectory() as tmp:
            base_path = os.path.join(tmp, "base.json")
            with open(base_path, "w") as fh:
                json.dump(base, fh)
            out = os.path.join(tmp, "out")
            written = variants.write_recipes(spec, base_path, out)
            self.assertEqual(len(written), 2)
            with open(written[1]) as fh:
                recipe = json.load(fh)
        self.assertEqual(recipe["frame"], base["frame"])
        self.assertEqual(recipe["views"], base["views"])
        self.assertTrue(
            recipe["source"].endswith(
                os.path.join("pair", "1st-mission", "a2", "assets")
            )
        )
        self.assertTrue(
            recipe["output"]["dir"].endswith(
                os.path.join("stills", "1st-mission", "a2")
            )
        )
        self.assertTrue(recipe["output"]["contact_sheet"])

    def test_the_build_is_read_as_the_preflight_reads_it(self):
        text = "velocity  v0.5.1-pre50\n git sha:  abc123\n   built:  2026-10-11T08:00:00Z\n"
        self.assertEqual(
            variants.parse_version(text),
            {
                "version": "0.5.1-pre50",
                "git_sha": "abc123",
                "built": "2026-10-11T08:00:00Z",
            },
        )


class MainTests(unittest.TestCase):
    """The batch as a whole, with the replay and the server mocked."""

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.set_path = os.path.join(self.tmp.name, "pair.json")
        with open(self.set_path, "w") as fh:
            json.dump(a_set(), fh)
        self.out = os.path.join(self.tmp.name, "out")
        self.listing = a_listing(
            **{"tuning.defaults.json": {}, "experiments/solid-body-a2.json": {}}
        )
        self.hashes = {
            "tuning.defaults.json": "sha256:d",
            "experiments/solid-body-a2.json": "sha256:a",
        }

    def tearDown(self):
        self.tmp.cleanup()

    def replay_for(self, scene, tuning_config=None):
        self.replayed.append((scene["start_secs"], scene["duration"], tuning_config))
        return {
            "vrlog": f"/vrlogs/{tuning_config}.vrlog",
            "run_id": f"run-{len(self.replayed)}",
            "start": {
                "tuning_config": tuning_config,
                "tuning_fingerprint": f"fp-{tuning_config}",
            },
        }

    def export_for(self, vrlog, out_dir, site, title, kind=None, extra=None):
        tuning_config = os.path.basename(vrlog)[: -len(".vrlog")]
        tuning_config = next(
            path for path in self.hashes if path.endswith(tuning_config)
        )
        header = {
            "tuning_hash": self.hashes[tuning_config],
            "build_version": "0.5.1-pre50",
        }
        return fake_export(header)(vrlog, out_dir, site, title, kind, extra)

    def run_main(self, *extra):
        self.replayed = []
        with mock.patch.object(
            publish, "plan", return_value=([a_scene()], [])
        ), mock.patch.object(
            publish, "db_problem", return_value=None
        ), mock.patch.object(
            variants, "fetch_configs", return_value=self.listing
        ), mock.patch.object(
            publish, "replay", side_effect=self.replay_for
        ), mock.patch.object(
            publish, "export", side_effect=self.export_for
        ), mock.patch.object(
            variants, "velocity_build", return_value={"version": "0.5.1-pre50"}
        ), contextlib.redirect_stdout(
            io.StringIO()
        ):
            return variants.main([self.set_path, "--out", self.out, *extra])

    def manifest(self):
        with open(os.path.join(self.out, "pair", "set.json")) as fh:
            return json.load(fh)

    def test_every_variant_replays_the_same_window_under_its_own_file(self):
        self.assertEqual(self.run_main(), 0)
        self.assertEqual(
            self.replayed,
            [
                (60.0, 100.0, "tuning.defaults.json"),
                (60.0, 100.0, "experiments/solid-body-a2.json"),
            ],
        )
        document = self.manifest()
        site = document["sites"][0]
        self.assertEqual(
            site["window"],
            {"offset_seconds": 100, "duration_seconds": 60, "warmup_seconds": 40.0},
        )
        self.assertEqual(
            [v["tuning_hash"] for v in site["variants"]], ["sha256:d", "sha256:a"]
        )
        self.assertEqual(document["problems"], [])
        self.assertEqual(document["set"]["baseline"], "defaults")

    def test_a_second_run_resumes_and_replays_nothing(self):
        self.assertEqual(self.run_main(), 0)
        self.assertEqual(self.run_main(), 0)
        self.assertEqual(self.replayed, [])
        self.assertEqual(len(self.manifest()["sites"][0]["variants"]), 2)

    def test_force_replays_every_variant_again(self):
        self.run_main()
        self.run_main("--force")
        self.assertEqual(len(self.replayed), 2)

    def test_a_file_the_server_cannot_apply_stops_the_set_before_any_replay(self):
        self.listing["configs"][1]["applicable"] = False
        self.listing["configs"][1]["reason"] = "changes startup-only settings: pipeline"
        self.assertEqual(self.run_main(), 1)
        self.assertEqual(self.replayed, [])

    def test_identical_parameters_fail_the_set(self):
        self.hashes["experiments/solid-body-a2.json"] = "sha256:d"
        self.assertEqual(self.run_main(), 1)
        self.assertIn("did not reach the pipeline", self.manifest()["problems"][0])

    def test_status_reports_without_replaying(self):
        self.assertEqual(self.run_main("--status"), 0)
        self.assertEqual(self.replayed, [])
        self.assertFalse(os.path.exists(os.path.join(self.out, "pair", "set.json")))


if __name__ == "__main__":
    unittest.main()
