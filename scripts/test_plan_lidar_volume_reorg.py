"""Run the volume planner and its apply.sh against a synthetic capture volume."""

import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).with_name("plan-lidar-volume-reorg.py")


def tree(root):
    return sorted(
        str(Path(d, n).relative_to(root))
        for d, dirs, files in os.walk(root)
        for n in dirs + files
    )


class PlanLidarVolumeReorgTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        base = Path(self.temp.name)
        self.vol, self.out = base / "vol", base / "plan"
        for rel in [
            "s2/s2_sf_1_00001.pcap",
            "s2/s2_sf_1_00002.pcap",
            "s2/analysis/a/segments.json",
            "s2/embarcadero-folsom-41cfqfsw/00-s2_sf_1_00001.pcapng",
            "sf-street-speeds/raw/lidar/site.pcapng",
            "velocity-campaign/run/pcap/copy.pcap",
            "seg/soma0-static-0.pcap",
            "state-estimation-x/out.json",
            "manifests/m.json",
            "kirk0.pcapng",
            "s2-sf-0.pcapng",
            "morg0.pcapng",
            "morg0.pcapng.gz",
            "state-estimation-x.log",
        ]:
            path = self.vol / rel
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(b"x" * 10)
        os.symlink("../s2-sf-0.pcapng", self.vol / "s2" / "s2-sf-0.pcapng")

    def plan(self):
        res = subprocess.run(
            [
                sys.executable,
                str(SCRIPT),
                "--root",
                str(self.vol),
                "--out",
                str(self.out),
                "--repo",
                self.temp.name,
            ],
            capture_output=True,
            text=True,
        )
        self.assertEqual(res.returncode, 0, res.stderr)
        rows = (self.out / "plan.tsv").read_text().splitlines()[1:]
        return res.stdout, {tuple(r.split("\t")[:3]) for r in rows}

    def apply(self):
        return subprocess.run(
            ["sh", str(self.out / "apply.sh")], capture_output=True, text=True
        )

    def test_plan_reads_only(self):
        before = tree(self.vol)
        self.plan()
        self.assertEqual(tree(self.vol), before)

    def test_plan_keeps_tool_inputs_under_pcaps(self):
        stdout, rows = self.plan()
        self.assertIn("scanner sees today    10 captures", stdout)
        self.assertIn("scanner sees after    6 captures", stdout)
        self.assertIn(("mv", "s2", "pcaps/s2"), rows)
        self.assertIn(("mv", "sf-street-speeds", "pcaps/sf-street-speeds"), rows)
        self.assertIn(("mv", "kirk0.pcapng", "pcaps/kirk/kirk0.pcapng"), rows)
        self.assertIn(
            ("mv", "morg0.pcapng.gz", "pcaps/morg/compressed/morg0.pcapng.gz"), rows
        )
        self.assertIn(("rm-symlink", "pcaps/s2/s2-sf-0.pcapng", ""), rows)
        self.assertIn(("mv", "s2-sf-0.pcapng", "pcaps/s2/s2-sf-0.pcapng"), rows)
        self.assertIn(("mv", "velocity-campaign", "work/velocity-campaign"), rows)
        self.assertIn(
            (
                "mv",
                "state-estimation-x.log",
                "work/state-estimation/state-estimation-x.log",
            ),
            rows,
        )
        # Analysis output stays beside the captures, where the archive tools read it.
        self.assertFalse(any(src.startswith("pcaps/s2/analysis") for _, src, _ in rows))
        self.assertFalse(any(src == "manifests" for _, src, _ in rows))

    def test_apply_moves_without_nesting_and_resumes(self):
        self.plan()
        (self.vol / "work" / "seg").mkdir(parents=True)  # blocks one move partway
        res = self.apply()
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("exists: work/seg", res.stderr)
        (self.vol / "work" / "seg").rmdir()
        self.assertEqual(self.apply().returncode, 0)
        after = tree(self.vol)
        self.assertEqual(
            self.apply().returncode, 0, "a finished plan re-runs as a no-op"
        )
        self.assertEqual(tree(self.vol), after)

        self.assertTrue((self.vol / "pcaps/s2/s2_sf_1_00001.pcap").is_file())
        self.assertFalse((self.vol / "pcaps/s2/s2").exists())
        self.assertTrue((self.vol / "pcaps/s2/analysis/a/segments.json").is_file())
        linked = self.vol / "pcaps/s2/s2-sf-0.pcapng"
        self.assertTrue(linked.is_file() and not linked.is_symlink())
        self.assertEqual(os.readlink(self.vol / "s2"), "pcaps/s2")
        self.assertTrue(
            (
                self.vol / "work/duplicates-to-verify/embarcadero-folsom-41cfqfsw"
            ).is_dir()
        )
        self.assertEqual(
            sorted(p.name for p in self.vol.iterdir()),
            ["manifests", "pcaps", "s2", "work"],
        )


if __name__ == "__main__":
    unittest.main()
