import importlib.util
import json
import pathlib
import stat
import sys
import tempfile
import unittest

HERE = pathlib.Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location(
    "stage_label_sets", HERE / "stage-label-sets.py"
)
stage_label_sets = importlib.util.module_from_spec(spec)
spec.loader.exec_module(stage_label_sets)

# A stand-in for the velocity binary: `publish` copies labels.json to the
# destination, `bind` appends a binding naming the file's fake digest.
FAKE_VELOCITY = """#!/usr/bin/env python3
import json, pathlib, sys
args = sys.argv[1:]
get = lambda flag: args[args.index(flag) + 1]
labels = pathlib.Path(get("--labels"))
if args[2] == "publish":
    dest = pathlib.Path(get("--dest"))
    dest.mkdir(parents=True)
    (dest / "labels.json").write_text((labels / "labels.json").read_text())
elif args[2] == "bind":
    m = json.loads((labels / "labels.json").read_text())
    m.setdefault("applies_to", []).append({"name": pathlib.Path(get("--pcap")).name, "sha256": "sha256:published"})
    (labels / "labels.json").write_text(json.dumps(m))
"""


class StageLabelSetsTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        root = pathlib.Path(self.tmp.name)
        self.dataset = root / "dataset"
        self.dataset.mkdir()
        self.rows = [
            {
                "capture_id": "a",
                "raw_path": "raw/lidar/20260902T1320_80858-0f4_columbus-broadway.pcapng",
                "raw_sha256": "sha256:published",
                "latitude": 37.7638,
            },
            {
                "capture_id": "b",
                "raw_path": "raw/lidar/20260901T1225_80858-0ac_ashbury-downey.pcapng",
                "raw_sha256": "sha256:other",
                "latitude": 37.1,
            },
        ]
        (self.dataset / "manifest.json").write_text(
            stage_label_sets.dump_manifest(self.rows)
        )
        (self.dataset / ".gitignore").write_text(".DS_Store\n")
        self.labels = root / "labels" / "columbus-proposals"
        self.labels.mkdir(parents=True)
        (self.labels / "labels.json").write_text(
            json.dumps(
                {
                    "name": "columbus-proposals",
                    "applies_to": [{"name": "roll.pcap", "sha256": "sha256:roll"}],
                }
            )
        )
        self.velocity = root / "velocity"
        self.velocity.write_text(FAKE_VELOCITY)
        self.velocity.chmod(self.velocity.stat().st_mode | stat.S_IEXEC)

    def tearDown(self):
        self.tmp.cleanup()

    def argv(self, *extra):
        return [
            "--dataset",
            str(self.dataset),
            "--labels",
            str(self.labels),
            "--velocity",
            str(self.velocity),
            *extra,
        ]

    def test_refuses_an_unverified_label_set(self):
        self.assertEqual(stage_label_sets.main(self.argv()), 1)
        self.assertFalse((self.dataset / "labels").exists())

    def test_binds_publishes_and_joins(self):
        self.assertEqual(stage_label_sets.main(self.argv("--capture-id", "a")), 0)
        published = (
            self.dataset
            / "labels"
            / "20260902T1320_80858-0f4_columbus-broadway"
            / "columbus-proposals"
        )
        self.assertTrue((published / "labels.json").exists())
        rows = json.loads((self.dataset / "manifest.json").read_text())
        self.assertEqual(
            rows[0]["label_set_paths"],
            ["labels/20260902T1320_80858-0f4_columbus-broadway/columbus-proposals"],
        )
        self.assertNotIn("label_set_paths", rows[1])
        attrs = (self.dataset / ".gitattributes").read_text()
        ignore = (self.dataset / ".gitignore").read_text()
        self.assertIn("labels/** -text", attrs)
        self.assertIn("labels/**/journal.ndjson", ignore)
        self.assertTrue(ignore.startswith(".DS_Store\n"))
        # A published label set is never overwritten.
        self.assertEqual(stage_label_sets.main(self.argv()), 1)

    def test_dry_run_changes_nothing(self):
        before = (self.dataset / "manifest.json").read_text()
        self.assertEqual(
            stage_label_sets.main(self.argv("--capture-id", "a", "--dry-run")), 0
        )
        self.assertEqual((self.dataset / "manifest.json").read_text(), before)
        self.assertFalse((self.dataset / "labels").exists())
        self.assertFalse((self.dataset / ".gitattributes").exists())


if __name__ == "__main__":
    sys.exit(unittest.main())
