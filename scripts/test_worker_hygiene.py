"""Safety regressions for worker archival; no live mounts or deletion targets."""

import argparse
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location(
    "hygiene", Path(__file__).with_name("worker-hygiene.py")
)
H = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(H)


class ArchiveTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.source = self.root / "evidence"
        self.source.mkdir()
        (self.source / "sample.db").write_bytes(b"frozen database")
        (self.source / "empty-directory").mkdir()
        self.dest = self.root / "nas"
        self.dest.mkdir()
        self.args = argparse.Namespace(
            source=str(self.source),
            destination=str(self.dest),
            nas_source="nas:/results",
            apply=True,
        )
        for name in ("quiet", "check_nas"):
            mock = patch.object(H, name)
            mock.start()
            self.addCleanup(mock.stop)
        mock = patch.object(H, "source_path", side_effect=lambda p: Path(p))
        mock.start()
        self.addCleanup(mock.stop)

    def archived(self):
        H.archive(self.args)
        return next(self.dest.iterdir())

    def test_verified_copy_preserves_original_and_separate_removal(self):
        target = self.archived()
        receipt = target / H.RECEIPT
        self.assertTrue(self.source.exists())
        args = argparse.Namespace(
            receipt=str(receipt), nas_source="nas:/results", apply=False
        )
        H.remove_verified(args)
        self.assertTrue(self.source.exists())
        args.apply = True
        H.remove_verified(args)
        self.assertFalse(self.source.exists())
        self.assertEqual((target / "sample.db").read_bytes(), b"frozen database")

    def test_source_or_archive_changes_refuse_removal(self):
        for which in ("source", "archive"):
            with self.subTest(which=which):
                target = self.archived()
                changed = self.source if which == "source" else target
                (changed / "sample.db").write_bytes(b"tampered")
                with self.assertRaises(ValueError):
                    H.remove_verified(
                        argparse.Namespace(
                            receipt=str(target / H.RECEIPT),
                            nas_source="nas:/results",
                            apply=True,
                        )
                    )
                self.assertTrue(self.source.exists())
                (self.source / "sample.db").write_bytes(b"frozen database")

    def test_links_repositories_and_special_files_refused(self):
        (self.source / "link").symlink_to(self.root)
        with self.assertRaises(ValueError):
            H.snapshot(self.source)
        (self.source / "link").unlink()
        (self.source / ".git").write_text("gitdir: elsewhere")
        with self.assertRaises(ValueError):
            H.snapshot(self.source)

    def test_failed_archive_keeps_source_without_receipt(self):
        with patch.object(H.shutil, "copyfileobj", side_effect=OSError("NAS lost")):
            with self.assertRaises(OSError):
                H.archive(self.args)
        self.assertTrue(self.source.exists())
        self.assertFalse(list(self.dest.rglob(H.RECEIPT)))

    def test_plan_performs_no_copy(self):
        self.args.apply = False
        H.archive(self.args)
        self.assertEqual(list(self.dest.iterdir()), [])

    def test_wrong_mount_and_soft_mount_rejected(self):
        for entry in (
            {"fstype": "ext4", "source": "/dev/sda", "options": "rw"},
            {"fstype": "nfs", "source": "other:/results", "options": "rw,hard"},
            {"fstype": "nfs", "source": "nas:/results", "options": "rw,soft"},
        ):
            with patch.object(H, "mount", return_value=entry):
                # Test the original function, before setUp's safety mock.
                with self.assertRaises(ValueError):
                    ORIGINAL_CHECK_NAS(self.dest, "nas:/results")


class WorktreeTests(unittest.TestCase):
    def test_automount_wrapper_does_not_hide_nfs(self):
        with tempfile.TemporaryDirectory() as tmp:
            wrapper = {"fstype": "autofs", "source": "systemd-1"}
            actual = {"fstype": "nfs", "source": "nas:/results", "options": "rw,hard"}
            with patch.object(
                H, "run", return_value=json.dumps({"filesystems": [wrapper, actual]})
            ):
                self.assertEqual(H.mount(Path(tmp)), actual)
            with patch.object(
                H, "run", return_value=json.dumps({"filesystems": [wrapper]})
            ):
                with self.assertRaises(ValueError):
                    H.mount(Path(tmp))

    def test_bind_alias_activity_matches_inode(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "tree").mkdir()
            (root / "tree/file").write_text("data")
            (root / "alias").symlink_to(root / "tree", target_is_directory=True)
            self.assertTrue(H.physical_inside(root / "alias/file", root / "tree"))
            self.assertFalse(H.physical_inside(root / "other", root / "tree"))

    def test_worktrees_keep_dirty_unmerged_and_locked(self):
        for extra, dirty, ancestor in (
            ("", "?? untracked", True),
            ("", "!! evidence.db", True),
            ("", "", False),
            ("\nlocked owner", "", True),
        ):
            with self.subTest(extra=extra, dirty=dirty, ancestor=ancestor):
                with tempfile.TemporaryDirectory() as tmp:
                    root = Path(tmp)
                    repo = root / "repo"
                    tree = root / "tree"
                    repo.mkdir()
                    tree.mkdir()
                    calls = []

                    def git(path, *args):
                        calls.append(args)
                        if args[:2] == ("worktree", "list"):
                            return f"worktree {repo}\nHEAD a\n\nworktree {tree}\nHEAD b{extra}"
                        if args[0] == "status":
                            return dirty
                        if args[0] == "merge-base" and not ancestor:
                            raise H.subprocess.CalledProcessError(1, "git")
                        return ""

                    with patch.object(H, "quiet"), patch.object(
                        H, "git_at", side_effect=git
                    ):
                        H.worktrees(
                            argparse.Namespace(repo=str(repo), days=14, apply=True)
                        )
                    self.assertFalse(
                        any(c[:2] == ("worktree", "remove") for c in calls)
                    )

    def test_retirement_only_applies_to_old_clean_merged_tree(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            repo, tree = root / "repo", root / "tree"
            repo.mkdir()
            tree.mkdir()
            H.os.utime(tree, (1, 1))
            calls = []

            def git(path, *args):
                calls.append(args)
                if args[:2] == ("worktree", "list"):
                    return f"worktree {repo}\nHEAD a\n\nworktree {tree}\nHEAD b"
                return ""

            with patch.object(H, "quiet"), patch.object(H, "git_at", side_effect=git):
                args = argparse.Namespace(repo=str(repo), days=14, apply=False)
                H.worktrees(args)
                self.assertFalse(any(c[:2] == ("worktree", "remove") for c in calls))
                args.apply = True
                H.worktrees(args)
            self.assertIn(("worktree", "remove", str(tree.resolve())), calls)

    def test_process_scan_requires_complete_visibility(self):
        with patch.object(H.os, "geteuid", return_value=1000):
            with self.assertRaises(ValueError):
                H.quiet(Path("/srv/banshee/evidence"))

    def test_mount_loss_prevents_copy(self):
        with patch.object(
            H, "mount", side_effect=H.subprocess.CalledProcessError(1, "findmnt")
        ):
            with self.assertRaises(H.subprocess.CalledProcessError):
                ORIGINAL_CHECK_NAS(Path("/missing"), "nas:/results")


ORIGINAL_CHECK_NAS = H.check_nas
if __name__ == "__main__":
    unittest.main()
