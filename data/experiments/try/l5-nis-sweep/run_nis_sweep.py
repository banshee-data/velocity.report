#!/usr/bin/env python3
"""L5 noise sweep scored by NIS, offline, through lidar-state-estimation-baseline.

Why this exists: the campaign's L5 passes concluded that no available metric
could see process_noise_pos, process_noise_vel or measurement_noise. The
2026-09-19 gap-analysis revision (K9) points out that the normalised innovation
squared can: it scores each accepted measurement against the prediction made
before that measurement was seen, so it is not circular in the RC6 sense; it
needs no labels; and it penalises over-smoothing instead of rewarding it, since
inflating R drives NIS below its consistency value of 2. It is already
instrumented (l5tracks/residuals.go, per speed band) and written by every
replay to tracking_baseline.json.

What one run is: lidar-state-estimation-baseline replays one corpus case with
one tuning file, twice, and refuses to report unless the repeat is
byte-identical, so every row here is a deterministic measurement by
construction. The tool's -out directory receives VRLOG recordings that this
sweep has no use for (about 250 MB per replay), so after the per-band summary
has been read the frames and index are removed and only the JSON stays; the
row records that.

Resumable: a (site, config) already in results.csv without an error is skipped.
Exit 3 with blocked.json if the disk falls under --min-free-gb: a half-written
replay is worse than none.

Usage:
    python3 run_nis_sweep.py --baseline-bin <bin> --site columbus-broadway \
        --configs-json '{"baseline": {}, "pos_x4": {"l5.cv_kf_v1.process_noise_pos": 0.2}}' \
        --out-dir data/experiments/try/l5-nis-sweep/columbus-broadway --duration 120 --warmup 70
"""

import argparse
import copy
import hashlib
import json
import shutil
import subprocess
import sys
import time
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "l3-settling-sweep"))
from row_appender import RowAppender  # noqa: E402

REPO_ROOT = Path(__file__).resolve().parents[4]
TUNING_DEFAULTS = REPO_ROOT / "config" / "tuning.defaults.json"
HERE = Path(__file__).resolve().parent
# First segment of each placement only (derived files beside this script). The
# full corpus declares every segment of a case, and the baseline tool counts
# packets and digests each declared capture before it replays, twice: at
# Columbus-Broadway (seven segments, 5 GB) that made one 20-second scoring
# run take 641 s on 2026-09-19. The sweep scores 190 s, inside one segment.
CORPUS = HERE / "first-segment-corpus.json"
INDEX = HERE / "first-segment-site-index.json"

CSV_FIELDS = [
    "timestamp",
    "git_sha",
    "site",
    "config",
    "overrides",
    "tuning_sha256",
    "duration_seconds",
    "warmup_seconds",
    "speed_floor_mps",
    "count",
    "decomposed",
    "mean_nis",
    "nis_exceedance_ratio",
    "lateral_rms_metres",
    "longitudinal_rms_metres",
    "lateral_bias_metres",
    "longitudinal_bias_metres",
    "assoc_matched",
    "assoc_missed",
    "assoc_rate",
    "first_run_frames",
    "repeat_run_frames",
    "baseline_equal",
    "recordings_removed",
    "wall_seconds",
    "run_dir",
    "corpus",
    "index",
    "error",
]


def git_sha():
    return subprocess.run(
        ["git", "rev-parse", "HEAD"],
        cwd=REPO_ROOT,
        capture_output=True,
        text=True,
        check=True,
    ).stdout.strip()


def set_dotted(cfg, dotted, value):
    node = cfg
    parts = dotted.split(".")
    for part in parts[:-1]:
        node = node[part]
    current = node[parts[-1]]
    # Go's strict unmarshal: keep the default's type.
    if isinstance(current, bool):
        node[parts[-1]] = bool(value)
    elif isinstance(current, int) and not isinstance(current, bool):
        if float(value) != int(float(value)):
            raise ValueError(f"{dotted} is an int key; {value!r} is not integral")
        node[parts[-1]] = int(float(value))
    else:
        node[parts[-1]] = float(value)


def write_tuning(base, overrides, path):
    cfg = copy.deepcopy(base)
    for dotted, value in overrides.items():
        set_dotted(cfg, dotted, value)
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


def remove_recordings(run_dir):
    """Delete the VRLOG frames and index under <run_dir>/<case>/{first,repeat},
    keeping every JSON. The tool nests the case id under -out, so the arms are
    two levels down; an earlier version looked one level up and removed
    nothing (the smoke test recorded recordings_removed=0)."""
    removed = 0
    for arm in ("first", "repeat"):
        for p in run_dir.glob(f"*/{arm}/*"):
            if p.name not in ("frames", "index.bin"):
                continue
            if p.is_dir():
                shutil.rmtree(p)
            else:
                p.unlink()
            removed += 1
    return removed


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--baseline-bin", required=True)
    ap.add_argument("--site", required=True)
    ap.add_argument(
        "--configs-json", required=True, help='{"name": {"dotted.key": value}}'
    )
    ap.add_argument("--out-dir", required=True)
    ap.add_argument("--duration", type=float, default=120)
    ap.add_argument("--warmup", type=float, default=70)
    ap.add_argument("--pcap-root", default="/Volumes/lidar/lidar")
    ap.add_argument("--min-free-gb", type=float, default=3)
    ap.add_argument("--keep-recordings", action="store_true")
    ap.add_argument("--corpus", default=str(CORPUS))
    ap.add_argument("--index", default=str(INDEX))
    args = ap.parse_args()

    configs = json.loads(args.configs_json)
    out_dir = Path(args.out_dir)
    out_dir.mkdir(parents=True, exist_ok=True)
    cfg_dir = out_dir / "configs"
    cfg_dir.mkdir(exist_ok=True)
    csv_path = out_dir / "results.csv"
    base = json.loads(TUNING_DEFAULTS.read_text())
    done = load_done(csv_path)
    sha = git_sha()

    with RowAppender(csv_path, CSV_FIELDS) as writer:
        writer.writeheader()
        for name, overrides in configs.items():
            if (args.site, name) in done:
                continue
            free_gb = shutil.disk_usage(out_dir).free / 2**30
            if free_gb < args.min_free_gb:
                (out_dir / "blocked.json").write_text(
                    json.dumps(
                        {
                            "blocked_at": time.strftime("%Y-%m-%dT%H:%M:%S%z"),
                            "reason": f"{free_gb:.1f} GiB free, below --min-free-gb {args.min_free_gb}; "
                            "not starting another replay",
                        },
                        indent=2,
                    )
                    + "\n"
                )
                print("BLOCKED: disk", file=sys.stderr)
                return 3

            tuning_path = cfg_dir / f"{name}.json"
            tuning_sha = write_tuning(base, overrides, tuning_path)
            run_dir = out_dir / args.site / name
            summary_json = run_dir / args.site / "first" / "tracking_baseline.json"
            if run_dir.exists() and not summary_json.exists():
                shutil.rmtree(
                    run_dir
                )  # a partial earlier attempt; the tool needs an empty dir

            t0 = time.time()
            row = {k: "" for k in CSV_FIELDS}
            row.update(
                timestamp=time.strftime("%Y-%m-%dT%H:%M:%S%z"),
                git_sha=sha,
                site=args.site,
                config=name,
                overrides=json.dumps(overrides, sort_keys=True),
                tuning_sha256=tuning_sha,
                duration_seconds=args.duration,
                warmup_seconds=args.warmup,
                run_dir=str(run_dir),
                corpus=args.corpus,
                index=args.index,
            )
            try:
                if not summary_json.exists():
                    proc = subprocess.run(
                        [
                            args.baseline_bin,
                            "-corpus",
                            args.corpus,
                            "-index",
                            args.index,
                            "-pcap-root",
                            args.pcap_root,
                            "-case",
                            args.site,
                            "-out",
                            str(run_dir),
                            "-tuning",
                            str(tuning_path),
                            "-duration",
                            str(args.duration),
                            "-warmup",
                            str(args.warmup),
                        ],
                        cwd=REPO_ROOT,
                        capture_output=True,
                        text=True,
                        timeout=3600,
                    )
                    (
                        (run_dir / "tool-stdout.txt").write_text(proc.stdout)
                        if run_dir.exists()
                        else None
                    )
                    if proc.returncode != 0:
                        raise RuntimeError(
                            f"baseline tool exit {proc.returncode}: {proc.stderr.strip()[-600:]}"
                        )
                summary = json.loads((run_dir / "phase0-summary.json").read_text())[
                    "cases"
                ][0]
                baseline = json.loads(summary_json.read_text())
                assoc = {
                    b["speed_floor_mps"]: b
                    for b in baseline.get("association_bands", [])
                }
                removed = 0 if args.keep_recordings else remove_recordings(run_dir)
                wall = round(time.time() - t0, 1)
                bands = [
                    b for b in baseline.get("residual_bands", []) if b["count"] > 0
                ]
                if not bands:
                    raise RuntimeError("no residual bands with observations")
                for b in bands:
                    a = assoc.get(b["speed_floor_mps"], {})
                    r = dict(row)
                    r.update(
                        speed_floor_mps=b["speed_floor_mps"],
                        count=b["count"],
                        decomposed=b.get("decomposed", ""),
                        mean_nis=b["mean_nis"],
                        nis_exceedance_ratio=b["nis_exceedance_ratio"],
                        lateral_rms_metres=b.get("lateral_rms_metres", ""),
                        longitudinal_rms_metres=b.get("longitudinal_rms_metres", ""),
                        lateral_bias_metres=b.get("lateral_bias_metres", ""),
                        longitudinal_bias_metres=b.get("longitudinal_bias_metres", ""),
                        assoc_matched=a.get("matched", ""),
                        assoc_missed=a.get("missed", ""),
                        assoc_rate=a.get("rate", ""),
                        first_run_frames=summary["first_run_frames"],
                        repeat_run_frames=summary["repeat_run_frames"],
                        baseline_equal=summary["baseline_equal"],
                        recordings_removed=removed,
                        wall_seconds=wall,
                    )
                    writer.writerow(r)
                print(
                    f"{args.site} {name}: frames={summary['first_run_frames']} "
                    + " ".join(
                        f"b{b['speed_floor_mps']}:n={b['count']},nis={b['mean_nis']:.2f},exc={b['nis_exceedance_ratio']:.3f}"
                        for b in bands
                    )
                    + f" wall={wall}s",
                    file=sys.stderr,
                    flush=True,
                )
            except Exception as e:  # recorded, never silently dropped
                row["error"] = str(e)
                row["wall_seconds"] = round(time.time() - t0, 1)
                writer.writerow(row)
                print(f"{args.site} {name}: error={e}", file=sys.stderr, flush=True)
    return 0


if __name__ == "__main__":
    sys.exit(main())
