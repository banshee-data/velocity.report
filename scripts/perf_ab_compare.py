#!/usr/bin/env python3
"""Compare lidar-bench runs of a base commit and a head commit made on one machine.

A committed baseline cannot catch the regression a change brings with it when the
change also moves the tuning fingerprint: the comparator refuses the old baseline,
the same pull request recaptures it, and the new numbers become the reference
without ever being compared with the old code. #765 did exactly that.

This compares code with code instead. perf-ab.sh runs the base and head binaries
alternately on the same runner, ABBA, so a pair shares whatever the host was doing
at the time; the per-pair ratio cancels slow drift that a median of each side
alone would not. The gate is the median of those ratios.

    perf_ab_compare.py --threshold 0.10 --summary out.md RUN_DIR

RUN_DIR holds <profile>/<n>-base.json and <profile>/<n>-head.json from the same
round n. Exit 0 when every profile is within threshold, 1 when one regressed,
2 when the input cannot answer the question.
"""

import argparse
import json
import statistics
import sys
from pathlib import Path

# The metrics gated and reported. Wall clock is the gate: it is what a replay
# and a deployment pay. Mean and p95 frame time say where it went.
GATED = "wall_clock_ms"
REPORTED = (
    ("wall_clock_ms", "wall clock (ms)", lambda m: m["wall_clock_ms"]),
    ("avg_ms", "mean frame (ms)", lambda m: m["frame_time_stats"]["avg_ms"]),
    ("p95_ms", "p95 frame (ms)", lambda m: m["frame_time_stats"]["p95_ms"]),
)
WORK = ("frames", "foreground_points", "clusters", "distinct_confirmed_tracks")


class InputError(Exception):
    """The runs cannot be compared; the gate must not pass or fail on them."""


def load_pairs(profile_dir):
    """Return [(base, head)] benchmark documents, one pair per round."""
    pairs = []
    for base_path in sorted(profile_dir.glob("*-base.json")):
        head_path = base_path.with_name(base_path.name.replace("-base", "-head"))
        if not head_path.exists():
            raise InputError(f"{base_path} has no matching {head_path.name}")
        pairs.append(
            (json.loads(base_path.read_text()), json.loads(head_path.read_text()))
        )
    if not pairs:
        raise InputError(f"no runs in {profile_dir}")
    return pairs


def check_same_workload(pairs):
    """Refuse pairs that did not replay the same capture under the same profile.

    The tuning fingerprint is deliberately not compared: a change that moves it
    is the case this exists for, and its cost is the answer, not a mismatch.
    """
    for base, head in pairs:
        for key in ("profile", "pcap_file"):
            b, h = base.get(key), head.get(key)
            if key == "pcap_file":
                b, h = Path(b or "").name, Path(h or "").name
            if b != h:
                raise InputError(f"{key} differs: base {b!r}, head {h!r}")


def compare_profile(name, pairs, threshold):
    check_same_workload(pairs)
    rows = []
    for key, label, get in REPORTED:
        base_vals = [get(b["metrics"]) for b, _ in pairs]
        head_vals = [get(h["metrics"]) for _, h in pairs]
        ratios = [h / b for b, h in zip(base_vals, head_vals) if b > 0]
        if not ratios:
            raise InputError(f"{name}: base {key} is zero in every round")
        rows.append(
            {
                "key": key,
                "label": label,
                "base": statistics.median(base_vals),
                "head": statistics.median(head_vals),
                "ratio": statistics.median(ratios),
            }
        )
    base0, head0 = pairs[0]
    work = [
        (k, base0["metrics"]["work"].get(k), head0["metrics"]["work"].get(k))
        for k in WORK
    ]
    gate = next(r for r in rows if r["key"] == GATED)
    return {
        "profile": name,
        "rounds": len(pairs),
        "rows": rows,
        "work": work,
        "tuning": (base0.get("tuning_fingerprint"), head0.get("tuning_fingerprint")),
        "regressed": gate["ratio"] > 1 + threshold,
    }


def render(results, threshold):
    out = [
        "## ⚡ Perf A/B: base vs head on one runner",
        "",
        f"Gate: median per-round wall-clock ratio above {1 + threshold:.2f} fails.",
        "",
    ]
    for r in results:
        verdict = "❌ regressed" if r["regressed"] else "✅ within threshold"
        tb, th = r["tuning"]
        tuning = (
            f"tuning `{tb}` → `{th}`" if tb != th else f"tuning `{tb}` (unchanged)"
        )
        out += [
            f"### {r['profile']}: {verdict}",
            "",
            f"{r['rounds']} rounds, {tuning}",
            "",
            "| | base | head | head ÷ base |",
            "|---|---:|---:|---:|",
        ]
        for row in r["rows"]:
            out.append(
                f"| {row['label']} | {row['base']:.2f} | {row['head']:.2f} | {row['ratio']:.3f} |"
            )
        out += ["", "| work | base | head |", "|---|---:|---:|"]
        out += [f"| {k} | {b} | {h} |" for k, b, h in r["work"]]
        out.append("")
    return "\n".join(out)


def main(argv=None):
    p = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    p.add_argument("run_dir", type=Path)
    p.add_argument("--threshold", type=float, default=0.10)
    p.add_argument("--summary", type=Path, help="also write the report here")
    args = p.parse_args(argv)

    try:
        profiles = sorted(d for d in args.run_dir.iterdir() if d.is_dir())
        if not profiles:
            raise InputError(f"no profile directories in {args.run_dir}")
        results = [
            compare_profile(d.name, load_pairs(d), args.threshold) for d in profiles
        ]
    except (InputError, OSError, KeyError, ValueError) as err:
        print(f"perf A/B cannot compare: {err}", file=sys.stderr)
        return 2

    report = render(results, args.threshold)
    print(report)
    if args.summary:
        args.summary.write_text(report + "\n")
    return 1 if any(r["regressed"] for r in results) else 0


if __name__ == "__main__":
    sys.exit(main())
