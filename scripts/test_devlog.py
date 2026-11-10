"""Journal fragments must survive parallel PRs and deterministic consolidation."""

from contextlib import contextmanager
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import devlog


@contextmanager
def checkout():
    with tempfile.TemporaryDirectory() as directory:
        old = Path.cwd()
        os.chdir(directory)
        try:
            subprocess.run(["git", "init", "-q"], check=True)
            subprocess.run(
                ["git", "config", "user.email", "test@example.com"], check=True
            )
            subprocess.run(["git", "config", "user.name", "Test"], check=True)
            Path(devlog.PENDING).mkdir(parents=True)
            Path(devlog.LOG).write_text("# Development log\n\n")
            yield
        finally:
            os.chdir(old)


def commit():
    devlog.command("git", "add", ".")
    devlog.command("git", "commit", "-qm", "fixture")
    return devlog.command("git", "rev-parse", "HEAD")


class Fragments(unittest.TestCase):
    def test_validation(self):
        with checkout():
            path = Path(devlog.PENDING) / "change-ab12.md"
            for text in [
                "",
                "# Heading",
                "- ",
                "- Worked.\n  continued",
                "- {branch} Worked.",
                "- Worked (#42).",
                "- [report][ref]",
                "- [report](a (b).md)",
            ]:
                path.write_text(text)
                with self.subTest(text=text), self.assertRaises(ValueError):
                    devlog.validate(path)
            path.write_text("- Fixed the gate.\n\n- Measured its limits.")
            self.assertEqual(len(devlog.validate(path)), 2)
            alias = path.with_name("alias.md")
            alias.symlink_to(path.name)
            with self.assertRaises(ValueError):
                devlog.validate(alias)

    def test_relocate(self):
        path = Path(devlog.PENDING) / "change-ab12.md"
        line = "- Read [report](../../lidar/a.md#limits), [web](https://example.com/a), [local](#anchor), and ![plot](../../images/a.svg)."
        actual = devlog.relocate(line, path)
        self.assertIn("(lidar/a.md#limits)", actual)
        self.assertIn("(images/a.svg)", actual)
        self.assertIn("(https://example.com/a)", actual)
        self.assertIn("(#anchor)", actual)

    def test_dates_order_and_historical_preservation(self):
        original = "# Development log\n\n## November 10, 2026 - Existing theme\n\n- Existing (#1).\n- {old/branch} Legacy.\n\n## November 8, 2026 - Earlier\n\n- Historical (#2).\n"
        path = Path(devlog.PENDING) / "change-ab12.md"
        records = [
            ("2026-11-11T00:01:00Z", 4, path, ["- New day."]),
            ("2026-11-10T00:01:00Z", 3, path, ["- Filled gap."]),
        ]
        output = devlog.render(original, records)
        self.assertEqual(output.count("## November 10,"), 1)
        self.assertLess(output.index("November 11,"), output.index("November 10,"))
        self.assertLess(output.index("Filled gap (#3)."), output.index("{old/branch}"))
        self.assertTrue(output.endswith(original[original.index("## November 8") :]))
        self.assertEqual(output, devlog.render(original, list(reversed(records))))
        self.assertEqual(output, devlog.render(output, []))
        self.assertEqual(devlog.render(original, []), original)

    def test_pr_policy_against_real_git_diff(self):
        with checkout():
            base = commit()
            Path("code.txt").write_text("change")
            commit()
            with self.assertRaises(ValueError):
                devlog.check_pr(base, "", False)
            devlog.check_pr(base, "<!-- devlog-skip: formatting only -->", False)
            path = Path(devlog.PENDING) / "change-ab12.md"
            path.write_text("- Changed behaviour.")
            commit()
            devlog.check_pr(base, "", False)
            base = devlog.command("git", "rev-parse", "HEAD")
            path.write_text("- Rewrote landed work.")
            commit()
            with self.assertRaises(ValueError):
                devlog.check_pr(base, "", False)
            Path(devlog.LOG).write_text("# Rewritten\n")
            commit()
            with self.assertRaises(ValueError):
                devlog.check_pr(base, "<!-- devlog-skip: journal -->", False)
            devlog.check_pr(base, "", True)

    def test_consolidation_provenance_atomicity_and_rerun(self):
        with checkout():
            path = Path(devlog.PENDING) / "change-ab12.md"
            path.write_text("- Recorded [evidence](../../lidar/report.md).")
            commit()
            devlog.command("git", "update-ref", "refs/remotes/origin/main", "HEAD")
            real = devlog.command

            def github(*args):
                if args[:3] == ("gh", "repo", "view"):
                    return "owner/repository"
                if args[0] == "gh":
                    return '[{"number":42,"merged_at":"2026-11-10T00:01:00Z","base":{"ref":"main","repo":{"full_name":"owner/repository"}}}]'
                return real(*args)

            before = Path(devlog.LOG).read_text()
            with patch.object(devlog, "command", side_effect=github):
                with patch("devlog.json.loads", return_value=[]), self.assertRaises(
                    ValueError
                ):
                    devlog.consolidate()
                self.assertEqual(Path(devlog.LOG).read_text(), before)
                self.assertTrue(path.exists())
                devlog.consolidate()
                output = Path(devlog.LOG).read_text()
                self.assertIn("[evidence](lidar/report.md) (#42).", output)
                self.assertFalse(path.exists())
                with self.assertRaises(ValueError):
                    devlog.consolidate()  # Refuse a dirty checkout.
                commit()
                devlog.command("git", "update-ref", "refs/remotes/origin/main", "HEAD")
                devlog.consolidate()
                self.assertEqual(Path(devlog.LOG).read_text(), output)
                self.assertEqual(real("git", "status", "--porcelain"), "")


if __name__ == "__main__":
    unittest.main()
