#!/usr/bin/env python3
"""Option-against-baseline sweep through the offline harness, with evidence kept.

Why this exists: by pass 6 the campaign had stopped being able to choose L4/L5
values (data/experiments/try/campaign/OBJECTIVES.md): the measurement, its
covariance and the association cost are all scheduled to change, so a tuned
value does not survive. What survives is a comparison of a default-off option
on against off, at many sites, on deterministic replays. The live-server
harness cannot reach Go-level options; lidar-state-estimation-baseline can
(-experiment), needs no server or port, and refuses to report a replay whose
repeat is not byte-identical.

What one run is: one corpus case, one config, replayed twice by the tool, with
the first arm's immutable evidence (every cluster, every accepted estimate,
every innovation) written to observations.db. The scorecard and anything asked
later are queries over that database, not another night of replays.

Determinism, which is a hard requirement here:
  - the tool enforces a byte-identical repeat; a failure is a row with `error`;
  - nothing in a scorecard depends on wall time, a path or the random track_id
    (lidar-track-scorecard keys tracks by creation_sequence);
  - run the same config twice under different names and the two scorecard
    digests must match: analyze_scorecard_sweep.py refuses a site where they do
    not, so every batch carries its own proof;
  - workers change wall time and nothing else: rows are written by the parent in
    planned order, and the smoke gate compares a digest made serially against
    one made under load.

SQLite is never written on the archive volume. exFAT has no journal and WAL's
shared-memory file is unreliable over a user-space filesystem driver, so the
database is written under --stage-dir (APFS), closed, hashed, moved to
--out-root and hashed again. Never point --out-root at the volume the PCAPs are
read from: the corpus baseline notes record one replay taking five hours that
way.

Resumable: a (site, config) already in results.csv without an error is skipped.
A partial earlier attempt is renamed aside, never deleted. Exit 3 with
blocked.json if either volume falls under its free-space floor.

Config format (--configs-json or --configs-file):
    {"name": {"overrides": {"l5.cv_kf_v1.hits_to_confirm": 1},
              "experiments": ["likelihood_cost"],
              "surface_ground": false, "measurement_mode": "",
              "keep_vrlog": false}}
Every key is optional; {} is the shipped baseline.

Usage:
    python3 run_scorecard_sweep.py --baseline-bin <bin> --scorecard-bin <bin> \
        --sites columbus-broadway,lombard-laguna --configs-file configs.json \
        --out-root /Volumes/Dolphin2/velocity-campaign/pass7 \
        --stage-dir /tmp/pass7-stage --workers 3
"""

import argparse
import concurrent.futures
import copy
import hashlib
import json
import shutil
import subprocess
import sys
import time
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "l3-settling-sweep"))
from row_appender import RowAppender

REPO_ROOT = Path(__file__).resolve().parents[4]
TUNING_DEFAULTS = REPO_ROOT / "config" / "tuning.defaults.json"
NIS_DIR = Path(__file__).resolve().parents[1] / "l5-nis-sweep"
CORPUS = NIS_DIR / "first-segment-corpus.json"
INDEX = NIS_DIR / "first-segment-site-index.json"

CSV_FIELDS = [
    "timestamp",
    "git_sha",
    "baseline_bin_sha256",
    "scorecard_bin_sha256",
    "site",
    "config",
    "overrides",
    "experiments",
    "surface_ground",
    "measurement_mode",
    "tuning_sha256",
    "duration_seconds",
    "warmup_seconds",
    "first_run_frames",
    "repeat_run_frames",
    "baseline_equal",
    "source_manifest_sha256",
    "evidence_sha256",
    "evidence_bytes",
    "evidence_path",
    "scorecard_sha256",
    "scorecard_path",
    "tracking_baseline_path",
    "vrlog_kept",
    "wall_seconds",
    "run_dir",
    "corpus",
    "index",
    "error",
]

CONFIG_KEYS = {
    "overrides",
    "experiments",
    "surface_ground",
    "measurement_mode",
    "keep_vrlog",
}


def sha256_file(path, chunk=1 << 20):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        while True:
            block = f.read(chunk)
            if not block:
                break
            h.update(block)
    return h.hexdigest()


def git_sha():
    return subprocess.run(
        ["git", "rev-parse", "HEAD"],
        cwd=REPO_ROOT,
        capture_output=True,
        text=True,
        check=True,
    ).stdout.strip()


# Go-side integer fields of the tuning config, by leaf name, from
# internal/config/tuning.go. The defaults file cannot be used to infer this:
# closeness_multiplier is written there as `3` and is a float64 in Go. The
# tool's JSON unmarshal is strict, so a float for an int field rejects the
# whole config (this voided half an interaction grid on 2026-09-18; see
# l3-settling-sweep/param_types.py, which covers the L3 keys only).
INT_KEYS = {
    "neighbour_confirmation_count",
    "min_confidence_floor",
    "locked_baseline_threshold",
    "warmup_min_frames",
    "warmup_duration_nanos",
    "change_threshold_snapshot",
    "foreground_min_cluster_points",
    "foreground_max_input_points",
    "max_sample_points",
    "hits_to_confirm",
    "max_misses",
    "max_misses_confirmed",
    "max_tracks",
    "min_points_for_pca",
    "obb_heading_lock_max_rejections",
    "max_track_history_length",
    "max_speed_history_length",
    "min_observations_for_classification",
    "min_cluster_size",
    "min_frame_points",
    "min_samples",
    "rts_smoothing_window",
    "version",
}


def set_dotted(cfg, dotted, value):
    node = cfg
    parts = dotted.split(".")
    for part in parts[:-1]:
        node = node[part]
    leaf = parts[-1]
    if leaf not in node:
        raise KeyError(f"{dotted} is not a key of the tuning defaults")
    current = node[leaf]
    if isinstance(current, bool):
        node[leaf] = bool(value)
    elif isinstance(current, str):
        node[leaf] = str(value)
    elif leaf in INT_KEYS:
        if not float(value).is_integer():
            raise ValueError(f"{dotted} is an int field; {value!r} is not integral")
        node[leaf] = int(float(value))
    else:
        node[leaf] = float(value)


def write_tuning(base, overrides, path):
    cfg = copy.deepcopy(base)
    for dotted in sorted(overrides):
        set_dotted(cfg, dotted, overrides[dotted])
    text = json.dumps(cfg, indent=2) + "\n"
    path.write_text(text)
    return hashlib.sha256(text.encode()).hexdigest()


def load_done(csv_path):
    import csv

    done = set()
    if csv_path.exists():
        with csv_path.open(newline="") as f:
            for row in csv.DictReader(f):
                if not row.get("error"):
                    done.add((row["site"], row["config"]))
    return done


def set_aside(path):
    """Rename a leftover directory out of the way. The tool needs an empty
    -out, and a partial attempt may be the only record of what went wrong."""
    if path.exists():
        path.rename(path.with_name(f"{path.name}.stale-{int(time.time() * 1000)}"))


def remove_tree(path):
    """rmtree that tolerates macOS AppleDouble sidecars. On exFAT every file
    gets a `._name` companion, and the filesystem removes it along with its
    file, so a directory walk finds entries that are gone by the time it
    reaches them. Anything else that survives is an error."""
    shutil.rmtree(path, ignore_errors=True)
    if Path(path).exists():
        raise RuntimeError(f"could not remove {path}")


def remove_recordings(run_dir):
    """Delete VRLOG frames and index under <run_dir>/<case>/{first,repeat};
    every JSON stays."""
    removed = 0
    for arm in ("first", "repeat"):
        for p in sorted(run_dir.glob(f"*/{arm}/*")):
            if p.name not in ("frames", "index.bin"):
                continue
            if p.is_dir():
                remove_tree(p)
            elif p.exists():
                p.unlink()
            removed += 1
    return removed


def free_gb(path):
    return shutil.disk_usage(path).free / 2**30


def one_run(job):
    """Runs in a worker thread; returns the finished row. Touches only its own
    directories, so workers cannot interact except through the disks."""
    a = job["args"]
    site, name, cfg = job["site"], job["name"], job["cfg"]
    t0 = time.time()
    run_dir = Path(a.out_root) / site / name
    stage = Path(a.stage_dir) / f"{site}__{name}"
    row = {k: "" for k in CSV_FIELDS}
    row.update(
        timestamp=time.strftime("%Y-%m-%dT%H:%M:%S%z"),
        git_sha=job["git_sha"],
        baseline_bin_sha256=job["baseline_sha"],
        scorecard_bin_sha256=job["scorecard_sha"],
        site=site,
        config=name,
        overrides=json.dumps(cfg.get("overrides", {}), sort_keys=True),
        experiments=",".join(sorted(cfg.get("experiments", []))),
        surface_ground=bool(cfg.get("surface_ground", False)),
        measurement_mode=cfg.get("measurement_mode", ""),
        tuning_sha256=job["tuning_sha"],
        duration_seconds=a.duration,
        warmup_seconds=a.warmup,
        run_dir=str(run_dir),
        corpus=a.corpus,
        index=a.index,
    )
    try:
        set_aside(run_dir)
        set_aside(stage)
        stage.mkdir(parents=True)
        run_dir.parent.mkdir(parents=True, exist_ok=True)
        cmd = [
            a.baseline_bin,
            "-corpus",
            a.corpus,
            "-index",
            a.index,
            "-pcap-root",
            a.pcap_root,
            "-case",
            site,
            "-out",
            str(run_dir),
            "-tuning",
            str(job["tuning_path"]),
            "-duration",
            str(a.duration),
            "-warmup",
            str(a.warmup),
            "-evidence-dir",
            str(stage / "evidence"),
            "-source-manifest",
            str(stage / "source-manifest.json"),
        ]
        experiments = sorted(cfg.get("experiments", []))
        if experiments:
            cmd += ["-experiment", ",".join(experiments)]
        if cfg.get("surface_ground"):
            cmd += ["-surface-ground"]
        if cfg.get("measurement_mode"):
            cmd += ["-measurement-mode", cfg["measurement_mode"]]
        proc = subprocess.run(
            cmd,
            cwd=REPO_ROOT,
            capture_output=True,
            text=True,
            timeout=2 * 3600,
            check=False,
        )
        if run_dir.exists():
            (run_dir / "tool-stdout.txt").write_text(proc.stdout)
            (run_dir / "tool-stderr.txt").write_text(proc.stderr[-20000:])
        if proc.returncode != 0:
            raise RuntimeError(
                f"baseline tool exit {proc.returncode}: {proc.stderr.strip()[-600:]}"
            )

        summary_doc = json.loads((run_dir / "phase0-summary.json").read_text())
        summary = summary_doc["cases"][0]
        if not summary["baseline_equal"]:
            raise RuntimeError("repeat replay was not byte-identical")
        if sorted(summary.get("experiments", [])) != experiments:
            raise RuntimeError(
                f"tool ran experiments {summary.get('experiments')} but {experiments} were asked for"
            )
        row.update(
            first_run_frames=summary["first_run_frames"],
            repeat_run_frames=summary["repeat_run_frames"],
            baseline_equal=summary["baseline_equal"],
            source_manifest_sha256=summary_doc.get("source_manifest_sha256", ""),
            tracking_baseline_path=str(
                run_dir / site / "first" / "tracking_baseline.json"
            ),
        )

        staged_db = stage / "evidence" / "observations.db"
        if not staged_db.exists():
            raise RuntimeError(f"no evidence database at {staged_db}")
        if a.scorecard_bin:
            scorecard_path = run_dir / "scorecard.json"
            sc = subprocess.run(
                [
                    a.scorecard_bin,
                    "-observations",
                    str(staged_db),
                    "-scoring-start-seconds",
                    str(a.warmup),
                    "-json",
                    str(scorecard_path),
                ],
                cwd=REPO_ROOT,
                capture_output=True,
                text=True,
                timeout=3600,
                check=False,
            )
            if sc.returncode != 0:
                # The evidence is what cannot be regenerated cheaply; archive it
                # and record the scorecard failure rather than losing the run.
                row["error"] = (
                    f"scorecard exit {sc.returncode}: {sc.stderr.strip()[-400:]}"
                )
            else:
                row["scorecard_path"] = str(scorecard_path)
                row["scorecard_sha256"] = sha256_file(scorecard_path)

        # A non-empty WAL means the last process to open the database did not
        # close it cleanly; archiving the main file alone would archive a
        # partial database. Checked after the scorecard, which opens it too.
        for suffix in ("-wal", "-shm"):
            side = staged_db.with_name(staged_db.name + suffix)
            if side.exists() and side.stat().st_size > 0:
                raise RuntimeError(f"{side.name} is non-empty; refusing to archive")
        staged_sha = sha256_file(staged_db)
        archived = run_dir / "observations.db"
        shutil.copyfile(staged_db, archived)
        shutil.copyfile(
            stage / "source-manifest.json", run_dir / "source-manifest.json"
        )
        archived_sha = sha256_file(archived)
        if archived_sha != staged_sha:
            raise RuntimeError(
                f"evidence digest changed in the move: {staged_sha} staged, {archived_sha} archived"
            )
        row.update(
            evidence_sha256=staged_sha,
            evidence_bytes=archived.stat().st_size,
            evidence_path=str(archived),
        )
        shutil.rmtree(stage)

        keep = bool(cfg.get("keep_vrlog", False)) or a.keep_recordings
        if not keep:
            remove_recordings(run_dir)
        row["vrlog_kept"] = keep
    except Exception as e:  # recorded, never silently dropped
        row["error"] = str(e)
    row["wall_seconds"] = round(time.time() - t0, 1)
    return row


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--baseline-bin", required=True)
    ap.add_argument(
        "--scorecard-bin",
        default="",
        help="lidar-track-scorecard; empty archives evidence only",
    )
    ap.add_argument("--sites", required=True, help="comma-separated corpus case ids")
    ap.add_argument("--configs-json", default="")
    ap.add_argument("--configs-file", default="")
    ap.add_argument(
        "--out-root",
        required=True,
        help="archive root; not the PCAP volume, may be exFAT",
    )
    ap.add_argument(
        "--stage-dir",
        required=True,
        help="APFS scratch for the live SQLite database",
    )
    ap.add_argument("--duration", type=float, default=200)
    ap.add_argument("--warmup", type=float, default=70)
    ap.add_argument(
        "--budget-hours",
        type=float,
        default=0,
        help="stop starting new replays after this long; 0 is no limit. "
        "Runs already started finish, and the sweep resumes from results.csv",
    )
    ap.add_argument("--pcap-root", default="/Volumes/lidar/lidar")
    ap.add_argument("--workers", type=int, default=1)
    ap.add_argument("--min-free-gb", type=float, default=20, help="floor on --out-root")
    ap.add_argument(
        "--min-stage-free-gb", type=float, default=4, help="floor on --stage-dir"
    )
    ap.add_argument("--keep-recordings", action="store_true")
    ap.add_argument("--corpus", default=str(CORPUS))
    ap.add_argument("--index", default=str(INDEX))
    args = ap.parse_args()

    if bool(args.configs_json) == bool(args.configs_file):
        ap.error("give exactly one of --configs-json and --configs-file")
    configs = json.loads(args.configs_json or Path(args.configs_file).read_text())
    # A leading underscore marks a note rather than a config, so a config file
    # can explain itself. Nothing else may be unrecognised: a typo in an option
    # name would otherwise run as the baseline and be reported as the option.
    configs = {name: cfg for name, cfg in configs.items() if not name.startswith("_")}
    for name, cfg in configs.items():
        if not isinstance(cfg, dict):
            ap.error(f"config {name!r} is {type(cfg).__name__}, not an object")
        unknown = set(cfg) - CONFIG_KEYS
        if unknown:
            ap.error(f"config {name!r} has unknown keys {sorted(unknown)}")
    sites = [s.strip() for s in args.sites.split(",") if s.strip()]

    out_root = Path(args.out_root)
    out_root.mkdir(parents=True, exist_ok=True)
    Path(args.stage_dir).mkdir(parents=True, exist_ok=True)
    cfg_dir = out_root / "configs"
    cfg_dir.mkdir(exist_ok=True)
    csv_path = out_root / "results.csv"
    base = json.loads(TUNING_DEFAULTS.read_text())
    done = load_done(csv_path)
    sha = git_sha()
    baseline_sha = sha256_file(args.baseline_bin)
    scorecard_sha = sha256_file(args.scorecard_bin) if args.scorecard_bin else ""

    # Planned order is site-major so one site's PCAP stays hot in the page
    # cache across its configs, and it is the order rows are written in
    # whatever order workers finish.
    jobs = []
    for site in sites:
        for name in configs:
            if (site, name) in done:
                continue
            tuning_path = cfg_dir / f"{name}.json"
            tuning_sha = write_tuning(
                base, configs[name].get("overrides", {}), tuning_path
            )
            jobs.append(
                {
                    "args": args,
                    "site": site,
                    "name": name,
                    "cfg": configs[name],
                    "tuning_path": tuning_path,
                    "tuning_sha": tuning_sha,
                    "git_sha": sha,
                    "baseline_sha": baseline_sha,
                    "scorecard_sha": scorecard_sha,
                }
            )
    print(
        f"{len(jobs)} run(s) planned, {len(done)} already done, workers={args.workers}",
        file=sys.stderr,
        flush=True,
    )

    blocked = None
    started_at = time.time()
    with RowAppender(csv_path, CSV_FIELDS) as writer:
        writer.writeheader()
        with concurrent.futures.ThreadPoolExecutor(max_workers=args.workers) as pool:
            pending = []
            next_job = 0
            while next_job < len(jobs) or pending:
                while (
                    blocked is None
                    and next_job < len(jobs)
                    and len(pending) < args.workers
                ):
                    if free_gb(out_root) < args.min_free_gb:
                        blocked = f"{free_gb(out_root):.1f} GiB free on --out-root, below {args.min_free_gb}"
                        break
                    if free_gb(args.stage_dir) < args.min_stage_free_gb:
                        blocked = f"{free_gb(args.stage_dir):.1f} GiB free on --stage-dir, below {args.min_stage_free_gb}"
                        break
                    elapsed_hours = (time.time() - started_at) / 3600
                    if args.budget_hours and elapsed_hours >= args.budget_hours:
                        blocked = (
                            f"{elapsed_hours:.2f} h elapsed, at the {args.budget_hours} h budget; "
                            f"{len(jobs) - next_job} run(s) not started"
                        )
                        break
                    pending.append(pool.submit(one_run, jobs[next_job]))
                    next_job += 1
                if not pending:
                    break
                # Oldest first: rows land in planned order.
                row = pending.pop(0).result()
                writer.writerow(row)
                print(
                    f"{row['site']} {row['config']}: "
                    + (
                        f"error={row['error']}"
                        if row["error"]
                        else f"frames={row['first_run_frames']} evidence={int(row['evidence_bytes']) >> 20} MiB "
                        f"scorecard={row['scorecard_sha256'][:12] or '-'}"
                    )
                    + f" wall={row['wall_seconds']}s",
                    file=sys.stderr,
                    flush=True,
                )
    if blocked:
        (out_root / "blocked.json").write_text(
            json.dumps(
                {
                    "blocked_at": time.strftime("%Y-%m-%dT%H:%M:%S%z"),
                    "reason": blocked + "; not starting another replay",
                },
                indent=2,
            )
            + "\n"
        )
        print(f"BLOCKED: {blocked}", file=sys.stderr)
        return 3
    return 0


if __name__ == "__main__":
    sys.exit(main())
