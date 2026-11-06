#!/usr/bin/env python3
"""Re-run lane proposals across every registered clip, and wait for them.

The server does the work (docs/plans/lidar-lane-field-batch-plan.md): one
lane_proposals job per clip, which replays the clip headlessly, sends its lane
fields to Gerardus against the clip's accepted record and derives the
proposals. This asks for the re-run, says what it will do, and follows the
jobs until none is left, one line per clip as it finishes. The clips view on
the web shows the same jobs with a progress bar.

    make lane-fields                 # stale or missing lanes only
    make lane-fields FORCE=1         # every registered clip
    make lane-fields DRY=1           # say what it would do; queue nothing

A clip whose site holds several accepted records is skipped: name its record
from the clips view's Run lanes, which asks for it. The server reads Gerardus
at its --gerardus-url; point it at the server the visualiser reads.
"""

import argparse
import json
import os
import sys
import time
import urllib.error
import urllib.request

API = os.environ.get("LIDAR_API_URL", "http://localhost:8080/api/lidar").rstrip("/")
POLL_SECONDS = float(os.environ.get("LANE_FIELDS_POLL_SECONDS", "10"))
ACTIVE = ("queued", "running")


def log(message):
    print(f"[{time.strftime('%H:%M:%S')}] {message}", flush=True)


def call(path, method="GET", timeout=120):
    request = urllib.request.Request(
        f"{API}{path}", method=method, data=b"" if method == "POST" else None
    )
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            return json.load(response)
    except urllib.error.HTTPError as error:
        body = error.read()[:300].decode(errors="replace")
        raise RuntimeError(f"{error.code} on {API}{path}: {body}") from None


def describe(plan):
    parts = [f"{len(plan['queued'])} to run"]
    for key, word in (
        ("current", "current"),
        ("active", "already running"),
        ("skipped", "skipped"),
    ):
        if plan.get(key):
            parts.append(f"{len(plan[key])} {word}")
    return ", ".join(parts)


def short_reason(reason):
    state = (reason or "").split(":", 1)[0]
    return {
        "ambiguous": "several accepted records; name one with Run lanes",
        "not_located": "no location",
        "not_registered": "no accepted pose at its site",
        "unknown": "Gerardus did not answer",
    }.get(state, reason)


def first_line(text):
    return (text or "").splitlines()[0] if text else ""


def follow(job_ids, names, poll_seconds=POLL_SECONDS):
    """Wait for the jobs, one line per change and per finish. Returns the failed."""
    waiting = set(job_ids)
    failed = []
    last = {}
    while waiting:
        jobs = call("/capture/jobs?kind=lane_proposals&limit=500").get("jobs", [])
        by_id = {j["job_id"]: j for j in jobs}
        for job_id in sorted(waiting):
            job = by_id.get(job_id)
            if job is None:
                continue
            name = names.get(job_id, job_id)
            if job["state"] in ACTIVE:
                detail = job.get("detail") or job["state"]
                if last.get(job_id) != detail:
                    last[job_id] = detail
                    log(f"  {name}: {detail}")
                continue
            waiting.discard(job_id)
            if job["state"] == "completed":
                log(f"  done   {name}: {job.get('detail', '')}")
            else:
                log(
                    f"  {job['state'].upper():<6} {name}: {first_line(job.get('error'))}"
                )
                failed.append(job_id)
        if waiting:
            time.sleep(poll_seconds)
    return failed


def main(argv=None):
    parser = argparse.ArgumentParser(
        description="Re-run lane proposals across every registered clip."
    )
    parser.add_argument("--force", action="store_true", help="re-run current clips too")
    parser.add_argument(
        "--dry-run", action="store_true", help="say what would run; queue nothing"
    )
    args = parser.parse_args(argv)

    query = "?dry_run=1" + ("&force=1" if args.force else "")
    plan = call(f"/clips/lane-proposals/rerun{query}", method="POST")
    log(f"lane proposals: {describe(plan)}")
    for clip in plan.get("skipped", []):
        name = clip.get("description") or clip["clip_id"][:8]
        log(f"  skip   {name}: {short_reason(clip.get('reason'))}")
    if args.dry_run or not plan["queued"]:
        return 0
    queued = call(
        "/clips/lane-proposals/rerun" + ("?force=1" if args.force else ""),
        method="POST",
    )
    names = {
        c["job_id"]: c.get("description") or c["clip_id"][:8]
        for c in queued["queued"]
        if c.get("job_id")
    }
    log(f"queued {len(names)} job(s); following them (the clips view shows the same)")
    failed = follow(list(names), names)
    log(f"finished: {len(names) - len(failed)} ok, {len(failed)} failed")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
