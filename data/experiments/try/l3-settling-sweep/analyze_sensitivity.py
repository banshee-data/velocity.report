#!/usr/bin/env python3
"""Deterministic sensitivity analysis over results.csv from run_sweep.py.

This is the "analyze per-site sensitivity and identify insensitive vs
sensitive parameters" step of the 2026-09 parameter experiment campaign
(docs/plans/lidar-parameter-experiment-campaign-2026-09.md). It is a fixed,
documented rule over the data -- not a judgement call made per run -- so the
same results.csv always produces the same verdict, and the rule itself is
auditable here rather than buried in a one-off analysis.

Rule (conservative on purpose -- the goal is to avoid narrowing a sweep
around noise, not to find the most dramatic-looking story):

For each (site, key, value) row, compare against that site's own baseline
row (_baseline=default) at the same capture_ordinal:
  - "frame_regression": recommended_settling_frame increases by both
    >= ABS_FRAME_DELTA frames AND >= REL_FRAME_DELTA fraction of baseline.
    Both bars matter: an absolute floor stops a baseline of ~11 frames from
    flagging on a +2 frame wobble (18% relative, but tiny in absolute terms);
    a relative floor stops a slow/high-baseline site from flagging on noise
    that would be unremarkable at its own scale.
  - "lost_convergence": baseline converged (settling frame recorded) but
    this value did not (settling not detected within the replay window).
  - "quality_regression": final coverage_rate, region_stability, or
    mean_confidence drops below its own configured settling threshold (see
    THRESHOLDS below, sourced from config/tuning.defaults.json) when the
    baseline was at or above it.
A (key, value) pair is "flagged" at a site if any of the three fire.

A key is "sensitive" overall if it is flagged on at least MIN_SITE_FRACTION
of sites (default 20%) for at least one tested value. Otherwise it is
"insensitive" -- the current default is robust across the corpus and no
further sweeping of that key is warranted.

Usage:
    python3 analyze_sensitivity.py --results results.csv --out sensitivity-analysis.json
"""

import argparse
import csv
import json
import statistics
from collections import defaultdict
from pathlib import Path

ABS_FRAME_DELTA = 10
REL_FRAME_DELTA = 0.5
MIN_SITE_FRACTION = 0.20

# From config/tuning.defaults.json l3.ema_baseline_v1 -- duplicated here
# deliberately as plain numbers (not re-parsed from the config) so this
# analysis is reproducible from results.csv alone, pinned to what those
# thresholds were when Batch 1 ran (recorded per-row via git_sha).
THRESHOLDS = {
    "final_coverage_rate": 0.8,  # settling_min_coverage
    "final_region_stability": 0.95,  # settling_min_region_stability
    "final_mean_confidence": 10.0,  # settling_min_confidence
}


def read_rows(results_csv, ordinal=None):
    """ordinal=None means no filter (used by analyze_replicate_consistency.py,
    which pre-splits rows by ordinal itself before calling analyze()
    directly). The CLI below always passes an explicit ordinal: mixing
    ordinal-0 (broad sweep) and ordinal-1 (replicate) rows together here
    silently inflates n_sites per value, since both ordinals' rows for the
    same site/key/value would be pooled into one count instead of compared
    against each other -- caught by hand on the first real run against a
    results.csv that had both ordinals present at once."""
    with open(results_csv) as f:
        rows = [r for r in csv.DictReader(f) if not r.get("error")]
    if ordinal is not None:
        rows = [r for r in rows if r.get("capture_ordinal", "0") == str(ordinal)]
    return rows


def to_float(v):
    try:
        return float(v)
    except (TypeError, ValueError):
        return None


def to_int_or_none(v):
    if v in (None, ""):
        return None
    try:
        return int(float(v))
    except ValueError:
        return None


def analyze(rows):
    # baseline[(site, ordinal)] = row
    baseline = {}
    by_key = defaultdict(list)
    for r in rows:
        ordinal = r.get("capture_ordinal", "0")
        if r["param_key"] == "_baseline":
            baseline[(r["site_id"], ordinal)] = r
        else:
            by_key[r["param_key"]].append(r)

    result = {
        "abs_frame_delta_threshold": ABS_FRAME_DELTA,
        "rel_frame_delta_threshold": REL_FRAME_DELTA,
        "min_site_fraction_threshold": MIN_SITE_FRACTION,
        "n_baseline_rows": len(baseline),
        "keys": {},
    }

    for key, key_rows in sorted(by_key.items()):
        by_value = defaultdict(list)
        for r in key_rows:
            by_value[r["param_value"]].append(r)

        value_verdicts = {}
        flagged_sites_any_value = set()
        for value, vrows in sorted(
            by_value.items(),
            key=lambda kv: to_float(kv[0]) if to_float(kv[0]) is not None else 0,
        ):
            n_sites = 0
            n_flagged = 0
            flagged_site_reasons = {}
            frame_deltas = []
            for r in vrows:
                b = baseline.get((r["site_id"], r.get("capture_ordinal", "0")))
                if b is None:
                    continue
                n_sites += 1
                reasons = []

                bf = to_int_or_none(b["recommended_settling_frame"])
                vf = to_int_or_none(r["recommended_settling_frame"])
                if bf is not None and bf >= 0:
                    if vf is None or vf < 0:
                        reasons.append("lost_convergence")
                    else:
                        delta = vf - bf
                        frame_deltas.append(delta)
                        if delta >= ABS_FRAME_DELTA and delta >= REL_FRAME_DELTA * bf:
                            reasons.append("frame_regression")

                for field, threshold in THRESHOLDS.items():
                    bv, rv = to_float(b[field]), to_float(r[field])
                    if (
                        bv is not None
                        and rv is not None
                        and bv >= threshold
                        and rv < threshold
                    ):
                        reasons.append(f"quality_regression:{field}")

                if reasons:
                    n_flagged += 1
                    flagged_site_reasons[r["site_id"]] = reasons
                    flagged_sites_any_value.add(r["site_id"])

            fraction = (n_flagged / n_sites) if n_sites else 0.0
            value_verdicts[value] = {
                "n_sites": n_sites,
                "n_flagged": n_flagged,
                "flagged_fraction": round(fraction, 3),
                "mean_frame_delta": (
                    round(statistics.mean(frame_deltas), 1) if frame_deltas else None
                ),
                "max_frame_delta": max(frame_deltas) if frame_deltas else None,
                "sensitive_at_this_value": fraction >= MIN_SITE_FRACTION,
                "flagged_sites": sorted(flagged_site_reasons.keys()),
            }

        is_sensitive = any(
            v["sensitive_at_this_value"] for v in value_verdicts.values()
        )
        worst_values = sorted(
            (v for v in value_verdicts.items() if v[1]["sensitive_at_this_value"]),
            key=lambda kv: kv[1]["flagged_fraction"],
            reverse=True,
        )
        result["keys"][key] = {
            "verdict": "sensitive" if is_sensitive else "insensitive",
            "rationale": (
                f"flagged on >= {MIN_SITE_FRACTION:.0%} of sites at value(s) "
                f"{[v for v, _ in worst_values]}"
                if is_sensitive
                else f"no tested value flagged >= {MIN_SITE_FRACTION:.0%} of sites"
            ),
            "n_sites_ever_flagged": len(flagged_sites_any_value),
            "by_value": value_verdicts,
        }

    return result


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--results", required=True)
    ap.add_argument("--out", required=True)
    ap.add_argument(
        "--ordinal",
        default="0",
        help="capture ordinal to analyze (default: 0, the broad sweep)",
    )
    args = ap.parse_args()

    rows = read_rows(args.results, ordinal=args.ordinal)
    result = analyze(rows)
    Path(args.out).write_text(json.dumps(result, indent=2))

    for key, info in result["keys"].items():
        print(f"{key}: {info['verdict']} -- {info['rationale']}")


if __name__ == "__main__":
    main()
