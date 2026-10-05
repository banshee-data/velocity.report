"""Exercise the no-results guard against real Git indexes."""

import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


SCRIPT = Path(__file__).with_name("check-no-results.py")


class NoResultsTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.repo = Path(self.temp.name)
        self.git("init", "-q")

    def git(self, *args):
        return subprocess.run(
            ["git", "-C", str(self.repo), *args], check=True, capture_output=True
        )

    def write(self, name, content="local evidence\n"):
        path = self.repo / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content)

    def check(self):
        return subprocess.run(
            [sys.executable, str(SCRIPT), "--repo", str(self.repo)],
            capture_output=True,
            text=True,
        )

    def test_empty_index_passes(self):
        self.assertEqual(self.check().returncode, 0)

    def test_documentation_names_and_fixtures_are_allowed(self):
        for name in ["docs/results-policy.md", "tests/fixtures/expected.json"]:
            self.write(name)
            self.git("add", "--", name)
        self.assertEqual(self.check().returncode, 0)

    def test_ignored_local_results_do_not_block_source_commit(self):
        self.write(".gitignore", "results/\n")
        self.write("results/run/report.json")
        self.write("source.py")
        self.git("add", "--all")
        self.assertEqual(self.check().returncode, 0)

    def test_force_added_root_and_nested_results_are_rejected(self):
        self.write(".gitignore", "results/\n")
        names = ["results/run.json", "tools/results/case with spaces\n.json"]
        for name in names:
            self.write(name)
            self.git("add", "--force", "--", name)
        result = self.check()
        self.assertEqual(result.returncode, 1)
        for name in names:
            self.assertIn(repr(name), result.stderr)

    def test_case_variant_is_rejected(self):
        self.write("tools/Results/run.json")
        self.git("add", "--", "tools/Results/run.json")
        self.assertEqual(self.check().returncode, 1)

    def test_unstaging_results_preserves_local_file_and_passes(self):
        name = "results/run.json"
        self.write(name)
        self.git("add", "--", name)
        self.assertEqual(self.check().returncode, 1)
        self.git("rm", "--cached", "--", name)
        self.assertTrue((self.repo / name).is_file())
        self.assertEqual(self.check().returncode, 0)


if __name__ == "__main__":
    unittest.main()
