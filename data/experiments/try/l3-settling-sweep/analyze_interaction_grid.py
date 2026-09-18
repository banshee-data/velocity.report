#!/usr/bin/env python3
"""Deterministic additive-vs-compounding verdict over interaction-results.csv
from run_interaction_grid.py.

Single-key sensitivity (analyze_sensitivity.py) already tells us each key's
effect in isolation. This answers the actual interaction question: when the
sensitive keys are pushed to their worst values *together*, does the effect
compound (worse than the sum of the single-key effects), cancel/plateau
(better than the sum), or stay additive (close to the sum)?

For each site, using only rows from interaction-results.csv (so the
single-key and joint numbers come from the same run and capture segment):
  baseline_frame   = recommended_settling_frame at the all-defaults combo
  single_delta[k]  = frame at (k=worst, others=default) minus baseline_frame
  predicted_delta  = sum(single_delta.values())            # additive model
  actual_delta     = frame at (all keys=worst) minus baseline_frame
  gap              = actual_delta - predicted_delta

A site is "compounding" if gap >= ABS_FRAME_DELTA and >= REL_FRAME_DELTA *
max(1, abs(predicted_delta)), "cancelling" if gap <= -ABS_FRAME_DELTA and
<= -REL_FRAME_DELTA * max(1, abs(predicted_delta)), else "additive". Same
two-bar shape (absolute + relative) as analyze_sensitivity.py, same
rationale: an absolute floor stops small baselines from flagging on noise, a
relative floor stops large baselines from flagging on unremarkable wobble.
Lost convergence (baseline or joint combo didn't settle) is its own bucket,
not folded into the numeric gap.

Only meaningful for a 2-key grid (single_delta needs one clean one-hot combo
per key); refuses anything else rather than guessing.

Usage:
    python3 analyze_interaction_grid.py --results interaction-results.csv \
        --levels interaction-levels.json --out interaction-grid-analysis.json
"""

import argparse
import csv
import json
import sys
from collections import defaultdict
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from analyze_sensitivity import (  # noqa: E402
    ABS_FRAME_DELTA,
    REL_FRAME_DELTA,
    MIN_SITE_FRACTION,
    to_int_or_none,
)
from plan_interaction_levels import DEFAULTS  # noqa: E402


def combo_id(overrides):
    return "|".join(f"{k}={v}" for k, v in sorted(overrides.items()))


def read_rows(results_csv):
    with open(results_csv) as f:
        return [r for r in csv.DictReader(f) if not r.get("error")]


def analyze(rows, levels):
    keys = sorted(levels.keys())
    if len(keys) != 2:
        return {
            "error": f"analyze_interaction_grid only supports exactly 2 keys, got {keys}"
        }

    by_site_combo = defaultdict(dict)
    for r in rows:
        by_site_combo[r["site_id"]][r["combo"]] = r

    baseline_overrides = {k: DEFAULTS[k] for k in keys}
    worst_overrides = {k: next(v for v in levels[k] if v != DEFAULTS[k]) for k in keys}
    baseline_cid = combo_id(baseline_overrides)
    joint_cid = combo_id(worst_overrides)
    single_cids = {}
    for k in keys:
        one_hot = dict(baseline_overrides)
        one_hot[k] = worst_overrides[k]
        single_cids[k] = combo_id(one_hot)

    site_results = {}
    for site_id, combos in by_site_combo.items():
        b = combos.get(baseline_cid)
        j = combos.get(joint_cid)
        singles = {k: combos.get(single_cids[k]) for k in keys}
        if b is None or j is None or any(v is None for v in singles.values()):
            continue  # incomplete for this site (missing combo, e.g. still erroring)

        bf = to_int_or_none(b["recommended_settling_frame"])
        jf = to_int_or_none(j["recommended_settling_frame"])
        sf = {k: to_int_or_none(singles[k]["recommended_settling_frame"]) for k in keys}

        if bf is None or bf < 0:
            site_results[site_id] = {"verdict": "baseline_did_not_converge"}
            continue
        if jf is None or jf < 0 or any(v is None or v < 0 for v in sf.values()):
            site_results[site_id] = {"verdict": "lost_convergence"}
            continue

        single_delta = {k: sf[k] - bf for k in keys}
        predicted_delta = sum(single_delta.values())
        actual_delta = jf - bf
        gap = actual_delta - predicted_delta
        scale = max(1, abs(predicted_delta))

        if gap >= ABS_FRAME_DELTA and gap >= REL_FRAME_DELTA * scale:
            verdict = "compounding"
        elif gap <= -ABS_FRAME_DELTA and gap <= -REL_FRAME_DELTA * scale:
            verdict = "cancelling"
        else:
            verdict = "additive"

        site_results[site_id] = {
            "verdict": verdict,
            "baseline_frame": bf,
            "single_delta": single_delta,
            "predicted_additive_delta": predicted_delta,
            "actual_joint_delta": actual_delta,
            "gap": gap,
        }

    counts = defaultdict(int)
    for v in site_results.values():
        counts[v["verdict"]] += 1
    n_scored = sum(
        c
        for verdict, c in counts.items()
        if verdict in ("compounding", "cancelling", "additive")
    )

    overall = "inconclusive"
    if n_scored > 0:
        for verdict in ("compounding", "cancelling"):
            if counts.get(verdict, 0) / n_scored >= MIN_SITE_FRACTION:
                overall = verdict
                break
        else:
            overall = "additive"

    return {
        "keys": keys,
        "baseline_combo": baseline_cid,
        "joint_combo": joint_cid,
        "single_combos": single_cids,
        "abs_frame_delta_threshold": ABS_FRAME_DELTA,
        "rel_frame_delta_threshold": REL_FRAME_DELTA,
        "min_site_fraction_threshold": MIN_SITE_FRACTION,
        "n_sites_scored": n_scored,
        "n_sites_total": len(site_results),
        "verdict_counts": dict(counts),
        "overall_verdict": overall,
        "sites": site_results,
    }


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--results", required=True)
    ap.add_argument("--levels", required=True)
    ap.add_argument("--out", required=True)
    args = ap.parse_args()

    rows = read_rows(args.results)
    levels = json.loads(Path(args.levels).read_text())
    result = analyze(rows, levels)
    Path(args.out).write_text(json.dumps(result, indent=2) + "\n")

    if "error" in result:
        print(result["error"], file=sys.stderr)
        sys.exit(1)
    print(
        f"overall_verdict: {result['overall_verdict']} "
        f"(scored {result['n_sites_scored']} sites, counts={result['verdict_counts']})"
    )


if __name__ == "__main__":
    main()
