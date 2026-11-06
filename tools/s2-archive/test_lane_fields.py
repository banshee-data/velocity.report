"""Cover the lane-fields driver: what it asks the server, and how it follows jobs."""

import contextlib
import importlib.util
import io
import os
import sys
import unittest
from unittest import mock

HERE = os.path.dirname(os.path.abspath(__file__))
spec = importlib.util.spec_from_file_location(
    "lane_fields", os.path.join(HERE, "lane-fields.py")
)
lane_fields = importlib.util.module_from_spec(spec)
sys.modules["lane_fields"] = lane_fields
spec.loader.exec_module(lane_fields)


def plan(queued=(), current=(), skipped=()):
    return {
        "dry_run": True,
        "force": False,
        "queued": [{"clip_id": c, "description": c} for c in queued],
        "current": [{"clip_id": c} for c in current],
        "active": [],
        "skipped": [{"clip_id": c, "reason": r} for c, r in skipped],
    }


class LaneFieldsTests(unittest.TestCase):
    def run_main(self, argv, responses):
        calls = []

        def fake_call(path, method="GET", timeout=120):
            calls.append((method, path))
            return responses.pop(0)

        out = io.StringIO()
        with mock.patch.object(lane_fields, "call", fake_call), mock.patch.object(
            lane_fields.time, "sleep", lambda s: None
        ), contextlib.redirect_stdout(out):
            code = lane_fields.main(argv)
        return code, calls, out.getvalue()

    def test_a_dry_run_queues_nothing(self):
        code, calls, out = self.run_main(
            ["--dry-run"],
            [plan(queued=["a"], skipped=[("k", "ambiguous: 7 accepted records")])],
        )
        self.assertEqual(code, 0)
        self.assertEqual(calls, [("POST", "/clips/lane-proposals/rerun?dry_run=1")])
        self.assertIn("1 to run, 1 skipped", out)
        self.assertIn("name one with Run lanes", out)

    def test_a_run_queues_then_follows_each_job_to_its_end(self):
        queued = {
            "queued": [
                {"clip_id": "a", "description": "Columbus", "job_id": "j1"},
                {"clip_id": "b", "description": "Bush", "job_id": "j2"},
            ]
        }
        responses = [
            plan(queued=["a", "b"]),
            queued,
            {
                "jobs": [
                    {
                        "job_id": "j1",
                        "state": "running",
                        "detail": "replaying the clip: 5s",
                    },
                    {"job_id": "j2", "state": "queued"},
                ]
            },
            {
                "jobs": [
                    {
                        "job_id": "j1",
                        "state": "completed",
                        "detail": "4 high, 2 medium, 6 to review",
                    },
                    {
                        "job_id": "j2",
                        "state": "failed",
                        "error": "replay: no packets\nmore",
                    },
                ]
            },
        ]
        code, calls, out = self.run_main(["--force"], responses)
        self.assertEqual(code, 1)
        self.assertEqual(
            calls[0], ("POST", "/clips/lane-proposals/rerun?dry_run=1&force=1")
        )
        self.assertEqual(calls[1], ("POST", "/clips/lane-proposals/rerun?force=1"))
        self.assertIn("done   Columbus: 4 high", out)
        self.assertIn("FAILED Bush: replay: no packets", out)
        self.assertNotIn("more", out)
        self.assertIn("finished: 1 ok, 1 failed", out)

    def test_nothing_to_run_returns_at_once(self):
        code, calls, _ = self.run_main([], [plan(current=["a"])])
        self.assertEqual(code, 0)
        self.assertEqual(len(calls), 1)


if __name__ == "__main__":
    unittest.main()
