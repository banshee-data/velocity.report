#!/usr/bin/env python3
"""Rebuilds results.csv rows whose runs completed but whose CSV line was lost.

Why this exists: on 2026-09-18 l3_extended_sweep_b (16:54-18:14) lost 344 of
its 456 rows. A git commit at 17:14:49 ran the mixed-line-ending pre-commit hook,
which replaced results.csv; run_sweep.py kept appending to the unlinked old file
(see row_appender.py). Every run still wrote its raw settling-eval report to
raw/, so nothing was lost but the CSV lines.

A row is rebuilt only when all of these hold, so a stray or stale report can
never be promoted to evidence:
  - it is not already in the CSV (idempotent: a second run adds nothing),
  - the report's mtime is inside the --after/--before window of the stage,
  - the report's pcap_file is the site's capture for this ordinal,
  - the config the report says it used (tuning_file) exists and holds the
    requested key at the requested value,
  - the report has a settling result and frame count.
Rows are built with run_sweep.row_from_report, the same function the driver
uses, so the columns are identical. The timestamp is the report's mtime, git_sha
is the commit that was HEAD at that moment, and wall_duration_seconds comes from
the report's own wall_duration. Skips are recorded, never silently dropped.

Usage:
    python3 recover_rows_from_raw.py --results results.csv --raw-dir raw \
        --keys freeze_threshold_multiplier,... --after 2026-09-18T16:54:39-0700 \
        --before 2026-09-18T18:14:52-0700 --record results-recovered-rows.json
"""

import argparse
import json
import re
import subprocess
import sys
import time
from collections import Counter
from datetime import datetime
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from param_types import coerce  # noqa: E402
from row_appender import RowAppender  # noqa: E402
from run_sweep import (  # noqa: E402
    CSV_FIELDS,
    REPO_ROOT,
    load_done,
    load_sites,
    row_from_report,
)

RAW_NAME = re.compile(
    r"^(?P<site>.+?)__(?P<key>.+?)__(?P<value>.+?)__o(?P<ordinal>\d+)\.json$"
)
TS_FORMAT = "%Y-%m-%dT%H:%M:%S%z"


def commits_by_time():
    out = subprocess.run(
        ["git", "log", "--format=%H %ct", "HEAD"],
        cwd=REPO_ROOT,
        capture_output=True,
        text=True,
        check=True,
    ).stdout.split()
    pairs = sorted((int(t), h) for h, t in zip(out[0::2], out[1::2]))
    return pairs


def sha_at(pairs, epoch):
    best = None
    for t, h in pairs:
        if t <= epoch:
            best = h
    return best or ""


def parse_wall(report):
    m = re.fullmatch(r"([0-9.]+)s", str(report.get("wall_duration", "")))
    return float(m.group(1)) if m else 0.0


def config_matches(report, key, value):
    tuning = report.get("tuning_file")
    if not tuning or not Path(tuning).exists():
        return False, "tuning_file missing"
    try:
        got = json.loads(Path(tuning).read_text())["l3"]["ema_baseline_v1"][key]
    except (KeyError, ValueError):
        return False, "tuning_file has no such key"
    want = coerce(key, json.loads(value))
    if abs(float(got) - float(want)) > 1e-9:
        return False, f"tuning_file has {got}, expected {want}"
    return True, ""


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--results", required=True)
    ap.add_argument("--raw-dir", required=True)
    ap.add_argument("--keys", required=True, help="comma-separated param_key values")
    ap.add_argument("--ordinal", type=int, default=0)
    ap.add_argument("--duration-seconds", type=float, default=120)
    ap.add_argument("--after", required=True, help="stage start, %Y-%m-%dT%H:%M:%S%z")
    ap.add_argument("--before", required=True, help="stage end, same format")
    ap.add_argument("--record", required=True)
    ap.add_argument("--dry-run", action="store_true")
    args = ap.parse_args()

    keys = {k for k in args.keys.split(",") if k}
    after = datetime.strptime(args.after, TS_FORMAT).timestamp()
    before = datetime.strptime(args.before, TS_FORMAT).timestamp()
    csv_path, raw_dir = Path(args.results), Path(args.raw_dir)

    sites = load_sites(args.ordinal)
    done = load_done(csv_path)
    pairs = commits_by_time()

    candidates = []
    for f in raw_dir.iterdir():
        m = RAW_NAME.match(f.name)
        if not m or m["key"] not in keys or int(m["ordinal"]) != args.ordinal:
            continue
        if (m["site"], m["key"], m["value"], str(args.ordinal)) in done:
            continue
        candidates.append((f.stat().st_mtime, f, m))
    candidates.sort(key=lambda c: c[0])

    recovered, skipped = [], []
    with RowAppender(csv_path, CSV_FIELDS) as writer:
        for mtime, f, m in candidates:
            site, key, value = m["site"], m["key"], m["value"]
            tag = f"{site} {key}={value}"
            if not after <= mtime <= before:
                skipped.append({"run": tag, "why": "report mtime outside stage window"})
                continue
            if site not in sites:
                skipped.append(
                    {"run": tag, "why": "site has no capture at this ordinal"}
                )
                continue
            try:
                report = json.loads(f.read_text())
            except ValueError:
                skipped.append({"run": tag, "why": "report is not valid JSON"})
                continue
            if "recommended_settling_frame" not in report or not report.get(
                "total_frames"
            ):
                skipped.append({"run": tag, "why": "report has no settling result"})
                continue
            want_pcap = str(Path("/Volumes/lidar/lidar") / sites[site]["relative_path"])
            if report.get("pcap_file") != want_pcap:
                skipped.append(
                    {"run": tag, "why": f"pcap_file is {report.get('pcap_file')}"}
                )
                continue
            ok, why = config_matches(report, key, value)
            if not ok:
                skipped.append({"run": tag, "why": why})
                continue

            row = row_from_report(
                site,
                key,
                value,
                sites[site],
                args.duration_seconds,
                report,
                parse_wall(report),
                f,
                "",
            )
            row["timestamp"] = (
                datetime.fromtimestamp(mtime).astimezone().strftime(TS_FORMAT)
            )
            row["git_sha"] = sha_at(pairs, int(mtime))
            if not args.dry_run:
                writer.writerow(row)
            recovered.append({"site": site, "key": key, "value": value})

    record = {
        "recovered_at": time.strftime(TS_FORMAT),
        "reason": (
            "results.csv was replaced by the mixed-line-ending pre-commit hook "
            "(commit fdb350073, 2026-09-18 17:14:49) while run_sweep.py held it "
            "open; later rows went to the unlinked file. Rows rebuilt from the "
            "surviving raw reports by recover_rows_from_raw.py."
        ),
        "window": {"after": args.after, "before": args.before},
        "ordinal": args.ordinal,
        "dry_run": args.dry_run,
        "n_recovered": len(recovered),
        "n_skipped": len(skipped),
        "recovered_by_key": dict(Counter(r["key"] for r in recovered)),
        "recovered_sites": sorted({r["site"] for r in recovered}),
        "skipped": skipped,
    }
    if not args.dry_run:
        Path(args.record).write_text(json.dumps(record, indent=2) + "\n")
    print(
        f"recovered {len(recovered)} row(s), skipped {len(skipped)}"
        + (" (dry run)" if args.dry_run else "")
    )
    for s in skipped[:10]:
        print(f"  skipped {s['run']}: {s['why']}")
    return 0 if not skipped else 2


if __name__ == "__main__":
    sys.exit(main())
