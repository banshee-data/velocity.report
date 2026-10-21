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
# destination (over it with --replace), `publish-view` makes the view's
# directory, `bind` appends a binding naming the file's fake digest. Every
# call is logged beside it.
FAKE_VELOCITY = """#!/usr/bin/env python3
import json, pathlib, sys
args = sys.argv[1:]
with open(pathlib.Path(sys.argv[0]).parent / "calls.log", "a") as log:
    log.write(" ".join(args[2:3] + [a for a in args if a.startswith("--") and a in ("--replace",)]) + "\\n")
get = lambda flag: args[args.index(flag) + 1]
labels = pathlib.Path(get("--labels"))
if args[2] == "publish":
    dest = pathlib.Path(get("--dest"))
    dest.mkdir(parents=True, exist_ok="--replace" in args)
    for path in labels.rglob("*"):
        if path.is_file():
            target = dest / path.relative_to(labels)
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(path.read_bytes())
elif args[2] == "publish-view":
    dest = pathlib.Path(get("--dest"))
    dest.mkdir(parents=True)
    (dest / "manifest.json").write_text("{}")
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

    def calls(self):
        log = self.velocity.parent / "calls.log"
        return log.read_text().splitlines() if log.exists() else []

    def linked_pack(self):
        pack = self.velocity.parent / "pack-a"
        pack.mkdir()
        (pack / "label-set-link.json").write_text(
            json.dumps({"label_set_id": "id", "path": "../labels/columbus-proposals"})
        )
        # Cut 2026-10-14 21:49:39 UTC from run e44ae459-….
        (pack / "manifest.json").write_text(
            json.dumps(
                {
                    "created_ns": 1792014579304685000,
                    "source": {"vrlog_path": "e44ae459-ca38-43ee-99fd-badc3a3d7d8f"},
                }
            )
        )
        return pack

    def view_argv(self, pack, *extra):
        return [
            "--dataset",
            str(self.dataset),
            "--view",
            str(pack),
            "--velocity",
            str(self.velocity),
            *extra,
        ]

    def stem(self):
        return self.dataset / "labels" / "20260902T1320_80858-0f4_columbus-broadway"

    def test_stages_a_view_in_its_label_set(self):
        pack = self.linked_pack()
        self.assertEqual(
            stage_label_sets.main(self.view_argv(pack, "--capture-id", "a")), 0
        )
        self.assertTrue((self.stem() / "columbus-proposals" / "labels.json").exists())
        view = "columbus-proposals/views/20261014-214939-e44ae459"
        self.assertTrue((self.stem() / view / "manifest.json").exists())
        rows = json.loads((self.dataset / "manifest.json").read_text())
        self.assertEqual(
            rows[0]["annotation_pack_paths"],
            ["labels/20260902T1320_80858-0f4_columbus-broadway/" + view],
        )
        ignore = (self.dataset / ".gitignore").read_text()
        for rule in (
            "labels/**/.annotations.lock",
            "labels/**/proposals/",
            "labels/**/physical-reference-revisions/",
            "labels/.export-*/",
            "labels/clip-*/vrlog/",
        ):
            self.assertIn(rule, ignore)
        self.assertEqual(self.calls(), ["bind", "publish", "publish-view"])
        # A published view is never overwritten.
        self.assertEqual(stage_label_sets.main(self.view_argv(pack)), 1)

    def test_a_set_published_at_the_same_head_is_kept(self):
        m = json.loads((self.labels / "labels.json").read_text())
        m["head"] = {"revision": 2233, "state": "sha256:s"}
        (self.labels / "labels.json").write_text(json.dumps(m))
        self.assertEqual(stage_label_sets.main(self.argv("--capture-id", "a")), 0)
        pack = self.linked_pack()
        self.assertEqual(stage_label_sets.main(self.view_argv(pack)), 0)
        self.assertEqual(self.calls(), ["bind", "publish", "publish-view"])

    def test_replace_refreshes_a_set_that_is_behind(self):
        m = json.loads((self.labels / "labels.json").read_text())
        m["head"] = {"revision": 2232, "state": "sha256:old"}
        (self.labels / "labels.json").write_text(json.dumps(m))
        self.assertEqual(stage_label_sets.main(self.argv("--capture-id", "a")), 0)
        m = json.loads((self.labels / "labels.json").read_text())
        m["head"] = {"revision": 2233, "state": "sha256:new"}
        (self.labels / "labels.json").write_text(json.dumps(m))
        self.assertEqual(stage_label_sets.main(self.argv()), 1)
        self.assertEqual(stage_label_sets.main(self.argv("--replace")), 0)
        self.assertEqual(self.calls(), ["bind", "publish", "publish --replace"])
        published = json.loads(
            (self.stem() / "columbus-proposals" / "labels.json").read_text()
        )
        self.assertEqual(published["head"]["revision"], 2233)

    def test_a_view_needs_a_link(self):
        pack = self.velocity.parent / "unlinked"
        pack.mkdir()
        self.assertEqual(stage_label_sets.main(self.view_argv(pack)), 1)
        self.assertEqual(stage_label_sets.main(["--dataset", str(self.dataset)]), 1)

    def test_retires_the_ndjson_lfs_rule_and_ignores_the_marker(self):
        (self.dataset / ".gitattributes").write_text(
            "*.bin filter=lfs diff=lfs merge=lfs -text\n"
            "labels/** -text\n"
            "labels/**/*.ndjson filter=lfs diff=lfs merge=lfs -text\n"
        )
        self.assertEqual(stage_label_sets.main(self.argv("--capture-id", "a")), 0)
        attrs = (self.dataset / ".gitattributes").read_text()
        self.assertNotIn("*.ndjson filter=lfs", attrs)
        self.assertIn("*.bin filter=lfs", attrs)
        self.assertIn("labels/**/.pending", (self.dataset / ".gitignore").read_text())

    def test_refuses_a_text_file_too_large_for_plain_git(self):
        chunk = self.labels / "masks" / "000000-000099.ndjson"
        chunk.parent.mkdir()
        chunk.write_bytes(b"x" * (stage_label_sets.MAX_PLAIN_GIT_BYTES + 1))
        (self.labels / "packets.bin").write_bytes(
            b"\0" * (stage_label_sets.MAX_PLAIN_GIT_BYTES + 1)
        )
        self.assertEqual(stage_label_sets.main(self.argv("--capture-id", "a")), 1)
        rows = json.loads((self.dataset / "manifest.json").read_text())
        self.assertNotIn("label_set_paths", rows[0])
        chunk.write_bytes(b"x" * 10)
        self.assertEqual(stage_label_sets.main(self.argv("--replace")), 0)

    def test_dry_run_changes_nothing(self):
        before = (self.dataset / "manifest.json").read_text()
        self.assertEqual(
            stage_label_sets.main(self.argv("--capture-id", "a", "--dry-run")), 0
        )
        self.assertEqual((self.dataset / "manifest.json").read_text(), before)
        self.assertFalse((self.dataset / "labels").exists())
        self.assertFalse((self.dataset / ".gitattributes").exists())

    def test_refresh_paths_follows_a_nested_view(self):
        pack = self.linked_pack()
        self.assertEqual(
            stage_label_sets.main(self.view_argv(pack, "--capture-id", "a")), 0
        )
        # 'labels nest' renamed the view; the row still names the old path.
        views = self.stem() / "columbus-proposals" / "views"
        (views / "20261014-214939-e44ae459").rename(views / "renamed")
        argv = ["--dataset", str(self.dataset), "--refresh-paths"]
        (self.dataset / ".gitignore").write_text(".DS_Store\n")
        self.assertEqual(stage_label_sets.main(argv), 0)
        self.assertIn("labels/clip-*/vrlog/", (self.dataset / ".gitignore").read_text())
        rows = json.loads((self.dataset / "manifest.json").read_text())
        stem = "labels/20260902T1320_80858-0f4_columbus-broadway/"
        self.assertEqual(rows[0]["label_set_paths"], [stem + "columbus-proposals"])
        self.assertEqual(
            rows[0]["annotation_pack_paths"],
            [stem + "columbus-proposals/views/renamed"],
        )
        self.assertNotIn("annotation_pack_paths", rows[1])

        # A pack in no set yet, in the capture's packs/, is listed too.
        loose = self.stem() / "packs" / "e44ae459-run"
        loose.mkdir(parents=True)
        (loose / "manifest.json").write_text("{}")
        self.assertEqual(stage_label_sets.main(argv), 0)
        rows = json.loads((self.dataset / "manifest.json").read_text())
        self.assertEqual(
            rows[0]["annotation_pack_paths"],
            [stem + "packs/e44ae459-run", stem + "columbus-proposals/views/renamed"],
        )


if __name__ == "__main__":
    sys.exit(unittest.main())
