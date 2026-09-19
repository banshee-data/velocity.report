#!/usr/bin/env python3
"""Waits for the running (pass 5) supervisor to exit, merges the pass-6 stages
into manifest.json, and launches a fresh supervisor on it.

Why a waiter: supervisor.py loads its handler table at start, and the pass-6
stage types (nis_sweep, analyze_nis_sweep) were added to the file after the
pass-5 process started. A stage of an unknown type would crash that process
mid-campaign, so the merge must not happen while it runs. Nothing here
touches the running supervisor or any stage it owns; this only watches for it
to finish on its own.

Merge is idempotent (stage ids already present are left alone) and atomic
(temp file + rename). The new supervisor gets its own budget clock, set from
pass6-stages.json.

Usage:
    nohup python3 launch_pass6.py --wait-pid <pid> > pass6-launcher.log 2>&1 &
"""

import argparse
import json
import os
import subprocess
import sys
import tempfile
import time
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO_ROOT = HERE.parents[3]


def log(msg):
    print(f"{time.strftime('%Y-%m-%dT%H:%M:%S%z')} {msg}", flush=True)


def pid_alive(pid):
    try:
        os.kill(pid, 0)
        return True
    except ProcessLookupError:
        return False
    except PermissionError:
        return True


def supervisor_pids():
    out = subprocess.run(
        ["ps", "-axo", "pid=,command="], capture_output=True, text=True
    ).stdout
    pids = []
    for line in out.splitlines():
        pid, _, cmd = line.strip().partition(" ")
        if (
            "supervisor.py" in cmd
            and "launch_pass6" not in cmd
            and ("python" in cmd.lower())
        ):
            pids.append(int(pid))
    return pids


def build_stages(spec):
    L = "data/experiments/try/l5-nis-sweep"
    stages = [
        {
            "id": "build_state_estimation_baseline",
            "type": "build_go_tool",
            "status": "pending",
            "depends_on": [],
            "params": {
                "pkg": "./cmd/tools/lidar-state-estimation-baseline/",
                "bin_name": "lidar-state-estimation-baseline",
                "run_tests": False,
            },
        }
    ]
    prev = "build_state_estimation_baseline"
    site_ids = []
    for site in spec["sites"]:
        sid = f"nis_sweep_{site.replace('-', '_')}"
        stages.append(
            {
                "id": sid,
                "type": "nis_sweep",
                "status": "pending",
                "depends_on": [prev],
                "params": {
                    "_comment": (
                        spec["_comment"]
                        if not site_ids
                        else f"Same nine configs on {site}."
                    ),
                    "site": site,
                    "configs": spec["configs"],
                    "duration": spec["duration"],
                    "warmup": spec["warmup"],
                    "pcap_root": "/Volumes/lidar/lidar",
                    "out_dir": f"{L}/{site}",
                },
            }
        )
        site_ids.append(sid)
        prev = sid
    core = site_ids[: spec["core_sites"]]
    stages.append(
        {
            "id": "analyze_nis_sweep_core",
            "type": "analyze_nis_sweep",
            "status": "pending",
            "depends_on": list(core),
            "params": {
                "_comment": "Interim, over the three Phase 0 corpus sites, so a budget cut still leaves an analysis.",
                "source_stages": list(core),
                "out_name": "nis-analysis-core.json",
            },
        }
    )
    stages.append(
        {
            "id": "analyze_nis_sweep",
            "type": "analyze_nis_sweep",
            "status": "pending",
            "depends_on": list(site_ids),
            "params": {
                "source_stages": list(site_ids),
                "out_name": "nis-analysis.json",
            },
        }
    )
    stages.append(
        {
            "id": "finalize_v6",
            "type": "finalize",
            "status": "pending",
            "depends_on": ["analyze_nis_sweep_core", "analyze_nis_sweep"],
            "params": {},
        }
    )
    return stages


def merge(manifest_path, spec):
    manifest = json.loads(manifest_path.read_text())
    have = {s["id"] for s in manifest["stages"]}
    added = 0
    for stage in build_stages(spec):
        if stage["id"] in have:
            continue
        manifest["stages"].append(stage)
        added += 1
    manifest["budget_hours"] = spec["budget_hours"]
    fd, tmp = tempfile.mkstemp(
        dir=str(manifest_path.parent), prefix="manifest.", suffix=".tmp"
    )
    with os.fdopen(fd, "w") as fh:
        fh.write(json.dumps(manifest, indent=2) + "\n")
    os.replace(tmp, manifest_path)
    return added


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument(
        "--wait-pid", type=int, required=True, help="the pass-5 supervisor's PID"
    )
    ap.add_argument("--stages", default=str(HERE / "pass6-stages.json"))
    ap.add_argument("--manifest", default=str(HERE / "manifest.json"))
    ap.add_argument("--poll-seconds", type=int, default=60)
    args = ap.parse_args()

    spec = json.loads(Path(args.stages).read_text())
    log(f"waiting for supervisor pid {args.wait_pid} to exit")
    while pid_alive(args.wait_pid):
        time.sleep(args.poll_seconds)
    log(f"pid {args.wait_pid} has exited")
    # Belt and braces: never start beside another supervisor.
    while supervisor_pids():
        log(f"another supervisor is still running ({supervisor_pids()}); waiting")
        time.sleep(args.poll_seconds)

    added = merge(Path(args.manifest), spec)
    log(
        f"merged {added} pass-6 stage(s) into {args.manifest}; budget_hours={spec['budget_hours']}"
    )
    sha = subprocess.run(
        ["git", "rev-parse", "--short", "HEAD"],
        cwd=REPO_ROOT,
        capture_output=True,
        text=True,
    ).stdout.strip()
    log_path = HERE / "pass6-supervisor.log"
    with log_path.open("a") as out:
        proc = subprocess.Popen(
            ["caffeinate", "-i", sys.executable, str(HERE / "supervisor.py")],
            cwd=REPO_ROOT,
            stdout=out,
            stderr=subprocess.STDOUT,
            start_new_session=True,
        )
    log(
        f"launched pass-6 supervisor pid {proc.pid} at HEAD {sha}; output in {log_path}"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
