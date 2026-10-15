import os
import pathlib
import shutil
import stat
import subprocess
import tempfile
import unittest

HERE = pathlib.Path(__file__).resolve().parent
SCRIPT = HERE / "clone-labels.sh"

# A stand-in for the velocity binary, logging its arguments.
FAKE_VELOCITY = """#!/bin/sh
echo "$@" >> "$(dirname "$0")/calls.log"
"""


def git(cwd, *args):
    subprocess.run(
        [
            "git",
            "-c",
            "user.name=t",
            "-c",
            "user.email=t@example.com",
            "-c",
            "commit.gpgsign=false",
            *args,
        ],
        cwd=cwd,
        check=True,
        capture_output=True,
    )


@unittest.skipUnless(shutil.which("git"), "git not installed")
class CloneLabelsTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        root = pathlib.Path(self.tmp.name)
        self.source = root / "dataset"
        files = {
            "manifest.json": "[]\n",
            "README.md": "# dataset\n",
            ".gitattributes": "labels/** -text\n",
            ".gitignore": "labels/**/journal.ndjson\n",
            "raw/lidar/a.pcapng": "a capture\n",
            "labels/a/set/labels.json": "{}\n",
            "labels/a/set/views/v1/manifest.json": "{}\n",
            "labels/b/set/labels.json": "{}\n",
        }
        for name, body in files.items():
            path = self.source / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(body)
        git(self.source, "init", "-q", "-b", "main")
        git(self.source, "add", "-A")
        git(self.source, "commit", "-q", "-m", "dataset")
        self.velocity = root / "bin" / "velocity"
        self.velocity.parent.mkdir()
        self.velocity.write_text(FAKE_VELOCITY)
        self.velocity.chmod(self.velocity.stat().st_mode | stat.S_IEXEC)
        self.work = root / "work"
        self.work.mkdir()

    def tearDown(self):
        self.tmp.cleanup()

    def clone(self, stem, dest="clone"):
        env = dict(
            os.environ,
            LABELS_DATASET_URL=str(self.source),
            VELOCITY=str(self.velocity),
        )
        return subprocess.run(
            ["sh", str(SCRIPT), stem, dest],
            cwd=self.work,
            env=env,
            capture_output=True,
            text=True,
        )

    def test_clones_one_capture_and_its_rules(self):
        result = self.clone("a")
        self.assertEqual(result.returncode, 0, result.stderr)
        clone = self.work / "clone"
        for kept in (
            "labels/a/set/labels.json",
            "labels/a/set/views/v1/manifest.json",
            "manifest.json",
            "README.md",
            ".gitattributes",
            ".gitignore",
        ):
            self.assertTrue((clone / kept).exists(), kept)
        for left in ("labels/b", "raw"):
            self.assertFalse((clone / left).exists(), left)
        calls = (self.velocity.parent / "calls.log").read_text().split()
        self.assertEqual(calls, ["lidar", "labels", "git-setup", "--repo", "."])

    def test_a_capture_without_labels_lists_those_with(self):
        result = self.clone("c")
        self.assertEqual(result.returncode, 1)
        self.assertIn("no labels/c", result.stderr)
        self.assertIn("  a", result.stderr)
        self.assertIn("  b", result.stderr)

    def test_refuses_paths_and_an_existing_folder(self):
        self.assertEqual(self.clone("../a").returncode, 2)
        (self.work / "there").mkdir()
        self.assertEqual(self.clone("a", "there").returncode, 1)


if __name__ == "__main__":
    unittest.main()
