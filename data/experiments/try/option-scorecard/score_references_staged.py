#!/usr/bin/env python3
"""Score references site by site, staging the evidence on APFS first.

Why staging. score_references.py runs the scorecard against the databases where
they sit. That is fine on a quiet volume and pathological on this one: the
archive is exFAT through a userspace driver, 91 % full, and SQLite's access
pattern is random. Measured 2026-09-20: about 30 comparisons an hour in place,
against a batch that took nine hours to produce. A sequential copy of the same
bytes is an order of magnitude faster than random reads of them, so this copies
each site's databases to APFS, scores them there, writes the small JSON results
back, and deletes the copies before moving on.

One site at a time, and the site's baseline is copied once and reused by all of
its arms, so the peak on the staging volume is one baseline plus one arm. That
matters: the boot volume runs at about 12 GB free.

Idempotent in the same way: a comparison whose output already parses is left
alone, so this can be re-run over a partly finished directory, and a site whose
copies fail is skipped rather than half-done.
"""

import argparse
import json
import shutil
import subprocess
import sys
import time
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO_ROOT = HERE.parents[3]
DEFAULT_BIN = (
    REPO_ROOT
    / "data"
    / "experiments"
    / "try"
    / "campaign"
    / "bin"
    / "lidar-track-scorecard"
)
OUT_NAME = "scorecard-vs-baseline.json"


def log(msg):
    print(f"{time.strftime('%Y-%m-%dT%H:%M:%S%z')} {msg}", flush=True)


def already_good(path):
    if not path.exists():
        return False
    try:
        json.loads(path.read_text())
        return True
    except Exception:
        path.unlink(missing_ok=True)
        return False


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--out-root", required=True)
    ap.add_argument("--stage-dir", default="/private/tmp/velocity-refscore-stage")
    ap.add_argument("--scorecard-bin", default=str(DEFAULT_BIN))
    ap.add_argument("--baseline-config", default="baseline")
    ap.add_argument("--scoring-start-seconds", type=float, default=70.0)
    ap.add_argument("--match-metres", type=float, default=2.0)
    ap.add_argument("--sites", default="", help="comma-separated; empty means all")
    args = ap.parse_args()

    out_root = Path(args.out_root)
    stage = Path(args.stage_dir)
    stage.mkdir(parents=True, exist_ok=True)
    wanted = {s.strip() for s in args.sites.split(",") if s.strip()}

    done = failed = skipped = 0
    for site_dir in sorted(p for p in out_root.iterdir() if p.is_dir()):
        if wanted and site_dir.name not in wanted:
            continue
        base_src = site_dir / args.baseline_config / "observations.db"
        if not base_src.exists():
            continue

        arms = [
            p
            for p in sorted(site_dir.iterdir())
            if p.is_dir()
            and p.name != args.baseline_config
            and (p / "observations.db").exists()
            and not already_good(p / OUT_NAME)
        ]
        skipped += len(
            [
                p
                for p in site_dir.iterdir()
                if p.is_dir()
                and p.name != args.baseline_config
                and already_good(p / OUT_NAME)
            ]
        )
        if not arms:
            continue

        site_stage = stage / site_dir.name
        site_stage.mkdir(parents=True, exist_ok=True)
        base_local = site_stage / "baseline.db"
        t0 = time.time()
        try:
            shutil.copyfile(base_src, base_local)
        except Exception as e:
            log(f"SKIP {site_dir.name}: cannot stage baseline: {e}")
            shutil.rmtree(site_stage, ignore_errors=True)
            continue
        log(
            f"{site_dir.name}: staged baseline ({base_local.stat().st_size/2**20:.0f} MiB) in {time.time()-t0:.0f}s, {len(arms)} arm(s)"
        )

        for arm in arms:
            arm_local = site_stage / "candidate.db"
            try:
                shutil.copyfile(arm / "observations.db", arm_local)
            except Exception as e:
                failed += 1
                log(f"FAILED stage {site_dir.name}/{arm.name}: {e}")
                continue
            dest_local = site_stage / "result.json"
            dest_local.unlink(missing_ok=True)
            t1 = time.time()
            r = subprocess.run(
                [
                    args.scorecard_bin,
                    "-observations",
                    str(arm_local),
                    "-reference",
                    str(base_local),
                    "-json",
                    str(dest_local),
                    "-scoring-start-seconds",
                    str(args.scoring_start_seconds),
                    "-reference-match-metres",
                    str(args.match_metres),
                ],
                capture_output=True,
                text=True,
            )
            arm_local.unlink(missing_ok=True)
            if r.returncode or not dest_local.exists():
                failed += 1
                log(f"FAILED {site_dir.name}/{arm.name}: {r.stderr.strip()[:200]}")
                continue
            # Write the small result back to the archive, where the summary
            # step and anything later will look for it.
            shutil.copyfile(dest_local, arm / OUT_NAME)
            dest_local.unlink(missing_ok=True)
            done += 1
            log(f"  scored {site_dir.name}/{arm.name} in {time.time()-t1:.0f}s")

        shutil.rmtree(site_stage, ignore_errors=True)

    log(f"{done} scored, {skipped} already present, {failed} failed")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
