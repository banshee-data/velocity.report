#!/usr/bin/env python3
"""Score every arm against its own site's baseline, from stored evidence.

run_scorecard_sweep.py predates lidar-track-scorecard's -reference flag, so a
batch it drives produces per-run scorecards with no per-frame comparison in
them. That costs nothing: the sweep keeps each run's immutable observations.db,
and the comparison is a query over two of those databases rather than another
replay. This walks a finished (or partly finished) --out-root and fills the gap.

What a row means. The reference is another *run*, not ground truth, so MOTA,
MOTP, IDSW, FM and HOTA measure divergence from the baseline's output, not
accuracy. The tool says as much in the `note` field of every result it writes,
and that wording is deliberately carried into the summary here.

The reading that matters is the pair:
  - `baseline_again` against `baseline` is the noise floor. Both are the shipped
    configuration, so anything other than MOTA 1.0 / IDSW 0 is measurement
    noise, and every other arm's numbers have to be read against it.
  - every other arm against `baseline` is that option's effect, in units the
    noise floor calibrates.

Idempotent: a comparison already written is skipped, so this can be re-run while
the batch is still going and again when it finishes.

    python3 score_references.py --out-root /Volumes/lidar/lidar/velocity-campaign/pass7-l3
"""

import argparse
import concurrent.futures
import json
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


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--out-root", required=True)
    ap.add_argument("--scorecard-bin", default=str(DEFAULT_BIN))
    ap.add_argument("--baseline-config", default="baseline")
    ap.add_argument("--scoring-start-seconds", type=float, default=70.0)
    ap.add_argument("--match-metres", type=float, default=2.0)
    ap.add_argument("--summary", default="")
    ap.add_argument(
        "--workers",
        type=int,
        default=1,
        help="parallel comparisons; each is an independent read of two finished "
        "databases, so this changes wall time and nothing in the result",
    )
    args = ap.parse_args()

    out_root = Path(args.out_root)
    rows, skipped, failed = [], 0, 0

    # Plan first, then run: the work is independent, so a pool is safe, and a
    # plan makes the skip/redo decision in one place.
    jobs = []
    for site_dir in sorted(p for p in out_root.iterdir() if p.is_dir()):
        base_db = site_dir / args.baseline_config / "observations.db"
        if not base_db.exists():
            continue
        for run_dir in sorted(p for p in site_dir.iterdir() if p.is_dir()):
            config = run_dir.name
            if config == args.baseline_config:
                continue
            obs = run_dir / "observations.db"
            if not obs.exists():
                continue
            dest = run_dir / OUT_NAME
            if dest.exists():
                # An interrupted run can leave a truncated file behind. Trust
                # only a file that parses; redo anything else.
                try:
                    json.loads(dest.read_text())
                    skipped += 1
                    jobs.append((site_dir.name, config, dest, None))
                    continue
                except Exception:
                    log(f"redoing unreadable {dest}")
                    dest.unlink(missing_ok=True)
            cmd = [
                args.scorecard_bin,
                "-observations",
                str(obs),
                "-reference",
                str(base_db),
                "-json",
                str(dest),
                "-scoring-start-seconds",
                str(args.scoring_start_seconds),
                "-reference-match-metres",
                str(args.match_metres),
            ]
            jobs.append((site_dir.name, config, dest, cmd))

    def run_one(job):
        site, config, dest, cmd = job
        if cmd is None:
            return site, config, dest, None
        r = subprocess.run(cmd, capture_output=True, text=True)
        return site, config, dest, (r.stderr.strip()[:200] if r.returncode else "")

    todo = sum(1 for j in jobs if j[3] is not None)
    log(
        f"{len(jobs)} comparison(s): {todo} to run, {skipped} already present, workers={args.workers}"
    )

    with concurrent.futures.ThreadPoolExecutor(max_workers=args.workers) as pool:
        results = list(pool.map(run_one, jobs))

    for site, config, dest, err in results:
        if err:
            failed += 1
            log(f"FAILED {site}/{config}: {err}")
            continue
        if err == "":
            log(f"scored {site}/{config}")
        try:
            doc = json.loads(dest.read_text())
        except Exception as e:  # a truncated file from an interrupted run
            failed += 1
            log(f"UNREADABLE {dest}: {e}")
            continue
        # One comparison per source in the run; a corpus case is normally a
        # single source, but a multi-file case has one entry per file and
        # each is its own reference pair.
        for source in doc.get("sources", []):
            for ref in source.get("reference", []):
                m = ref.get("metrics", {})
                h = ref.get("hota", {})
                rows.append(
                    {
                        "site": site,
                        "config": config,
                        "source_id": source.get("source_id", ""),
                        "mota": m.get("mota"),
                        "motp_metres": m.get("motp_metres"),
                        "idsw": m.get("id_switches"),
                        "fm": m.get("fragmentations"),
                        "fn": m.get("fn"),
                        "fp": m.get("fp"),
                        "matches": m.get("matches"),
                        "num_gt": m.get("num_gt"),
                        "num_frames": m.get("num_frames"),
                        "reference_tracks": ref.get("reference_tracks"),
                        "candidate_tracks": ref.get("candidate_tracks"),
                        "hota": h.get("hota"),
                        "det_a": h.get("det_a"),
                        "ass_a": h.get("ass_a"),
                        "note": ref.get("note", ""),
                    }
                )

    log(f"{len(rows)} comparison(s); {skipped} already present, {failed} failed")

    summary = (
        Path(args.summary) if args.summary else out_root / "reference-summary.json"
    )
    payload = {
        "generated": time.strftime("%Y-%m-%dT%H:%M:%S%z"),
        "out_root": str(out_root),
        "baseline_config": args.baseline_config,
        "scoring_start_seconds": args.scoring_start_seconds,
        "reference_match_metres": args.match_metres,
        "reading": (
            "The reference is another run, not ground truth: every number measures "
            "divergence from the baseline's output, not accuracy. Read baseline_again "
            "as the noise floor and every other arm against it. "
            "The trap: because the reference IS the baseline, every score is maximised "
            "by resembling the baseline, so a high MOTA or HOTA means 'changed little', "
            "never 'better'. Rank arms by how far they move and in which direction — "
            "candidate_tracks against reference_tracks, and fp against fn — and take the "
            "question of which direction is right to labelled evidence, not to this table."
        ),
        "failed": failed,
        "rows": sorted(rows, key=lambda r: (r["site"], r["config"])),
    }
    summary.write_text(json.dumps(payload, indent=2, sort_keys=True) + "\n")
    log(f"summary -> {summary}")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
