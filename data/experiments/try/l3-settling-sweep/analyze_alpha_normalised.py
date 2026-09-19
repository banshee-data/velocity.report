#!/usr/bin/env python3
"""Re-scores the background_update_fraction sweep with an alpha-normalised
settling criterion, from the raw reports already on disk. No new compute.

Why: paper-implementation-gap-analysis.md (B2, 2026-09-19) derives that a
perfectly settled cell has a spread-delta floor proportional to the update
fraction alpha: E|delta s| ~= 0.483 * alpha * sigma. The settling criterion
compares the mean per-frame spread delta against a fixed
settling_max_spread_delta, so a grid updated with a larger alpha can be fully
converged and still never pass. The campaign's "background_update_fraction
>= 0.05 delays settling" verdict is therefore partly a property of the
criterion. This script asks how much: it recomputes each run's settling frame
with the spread-delta criterion scaled to the shipped alpha
(spread_delta_rate * (ALPHA_DEFAULT / alpha) <= max_spread_delta; the other
three criteria unchanged, since coverage, stability and confidence carry no
such floor), then re-applies analyze_sensitivity.py's flag rule against the
site's unchanged default baseline.

Deterministic: same raw reports, same output. The recorded settling frame is
re-derived under the original criterion for every row as a self-check and the
script exits 2 if any differs.

Usage:
    python3 analyze_alpha_normalised.py --results results.csv --out bg-alpha-normalised.json
"""

import argparse
import csv
import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from analyze_post_settle import frame_ok  # noqa: E402

ALPHA_DEFAULT = 0.02
KEY = "background_update_fraction"
ABS_FRAME_DELTA = 10
REL_FRAME_DELTA = 0.5
MIN_SITE_FRACTION = 0.2


def first_ok(hist, thresholds, scale):
    """Frame number of the first frame passing all four criteria, with the
    spread-delta criterion scaled; -1 if none."""
    t = dict(thresholds)
    for m in hist:
        mm = dict(m)
        mm["spread_delta_rate"] = m["spread_delta_rate"] * scale
        if frame_ok(mm, t):
            return m["frame_number"]
    return -1


def flagged(frame, base):
    """analyze_sensitivity.py's per-site rule: a row is flagged when its frame
    moves by >= ABS_FRAME_DELTA and >= REL_FRAME_DELTA of the baseline, or
    convergence is lost or gained."""
    if frame < 0 or base < 0:
        return frame != base
    d = abs(frame - base)
    return d >= ABS_FRAME_DELTA and d >= REL_FRAME_DELTA * max(base, 1)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--results", required=True)
    ap.add_argument("--out", required=True)
    args = ap.parse_args()

    rows = [
        r for r in csv.DictReader(open(args.results, newline="")) if not r.get("error")
    ]
    base_rows = {
        (r["site_id"], r.get("capture_ordinal", "0")): r
        for r in rows
        if r["param_key"] == "_baseline"
    }
    sweep_rows = [r for r in rows if r["param_key"] == KEY]

    mismatches = []
    per_value = {}
    base_frames = {}
    for k, r in base_rows.items():
        rep = json.loads(Path(r["raw_report_path"]).read_text())
        f0 = first_ok(rep["metrics_history"], rep["thresholds"], 1.0)
        if f0 != rep["recommended_settling_frame"]:
            mismatches.append(
                {
                    "run": f"{k[0]} _baseline o{k[1]}",
                    "recomputed": f0,
                    "recorded": rep["recommended_settling_frame"],
                }
            )
        base_frames[k] = f0

    for r in sweep_rows:
        k = (r["site_id"], r.get("capture_ordinal", "0"))
        if k not in base_frames:
            continue
        alpha = float(r["param_value"])
        rep = json.loads(Path(r["raw_report_path"]).read_text())
        hist, thr = rep["metrics_history"], rep["thresholds"]
        original = first_ok(hist, thr, 1.0)
        if original != rep["recommended_settling_frame"]:
            mismatches.append(
                {
                    "run": f"{k[0]} {KEY}={r['param_value']} o{k[1]}",
                    "recomputed": original,
                    "recorded": rep["recommended_settling_frame"],
                }
            )
        normalised = first_ok(hist, thr, ALPHA_DEFAULT / alpha)
        base = base_frames[k]
        entry = per_value.setdefault((k[1], r["param_value"]), [])
        entry.append(
            {
                "site": k[0],
                "baseline_frame": base,
                "original_frame": original,
                "normalised_frame": normalised,
                "flagged_original": flagged(original, base),
                "flagged_normalised": flagged(normalised, base),
            }
        )

    out = {
        "alpha_default": ALPHA_DEFAULT,
        "rule": {
            "abs_frame_delta": ABS_FRAME_DELTA,
            "rel_frame_delta": REL_FRAME_DELTA,
            "min_site_fraction": MIN_SITE_FRACTION,
        },
        "by_ordinal": {},
    }
    for (ordinal, value), sites in sorted(
        per_value.items(), key=lambda kv: (kv[0][0], float(kv[0][1]))
    ):
        n = len(sites)
        fo = sum(s["flagged_original"] for s in sites)
        fn = sum(s["flagged_normalised"] for s in sites)
        out["by_ordinal"].setdefault(ordinal, {})[value] = {
            "n_sites": n,
            "flagged_original": fo,
            "flagged_normalised": fn,
            "verdict_original": (
                "sensitive" if fo / n >= MIN_SITE_FRACTION else "insensitive"
            ),
            "verdict_normalised": (
                "sensitive" if fn / n >= MIN_SITE_FRACTION else "insensitive"
            ),
            "never_converged_original": sum(s["original_frame"] < 0 for s in sites),
            "never_converged_normalised": sum(s["normalised_frame"] < 0 for s in sites),
            "sites": sites,
        }
    out["self_check"] = {"mismatches": mismatches}
    Path(args.out).write_text(json.dumps(out, indent=2) + "\n")

    print(f"self-check mismatches: {len(mismatches)}")
    for ordinal, vals in out["by_ordinal"].items():
        for value, v in vals.items():
            print(
                f"o{ordinal} {KEY}={value}: flagged {v['flagged_original']}/{v['n_sites']} -> "
                f"{v['flagged_normalised']}/{v['n_sites']} alpha-normalised "
                f"({v['verdict_original']} -> {v['verdict_normalised']}); never converged "
                f"{v['never_converged_original']} -> {v['never_converged_normalised']}"
            )
    return 2 if mismatches else 0


if __name__ == "__main__":
    sys.exit(main())
