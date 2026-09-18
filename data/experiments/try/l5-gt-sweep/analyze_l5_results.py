#!/usr/bin/env python3
"""Deterministic ranking + cross-site agreement check over one or more
results.csv from run_l5_gt_sweep.py.

Each --results is one reference site's sweep (same 27 noise-param combos,
different reference_run_id/pcap). This does not pick a "winner" by judgement
-- it just ranks composite_score per site and reports whether the top combo
agrees across sites, so a future check-in doesn't have to reread every raw
CSV to see whether the signal is even consistent enough to act on.

TOP_K controls both how many rows are shown per site and how "agreement" is
defined: a combo counts as agreed-on if it appears in every site's top-K,
not only if it is each site's single best (single-best agreement across
noisy, low-sample-size ground truth is an unreasonably strict bar -- see
each site's reference_count in the raw CSV).

Usage:
    python3 analyze_l5_results.py --out ranked.json \
        --results kirk1-ref/results.csv --results kirk0-dd98/results.csv ...
"""

import argparse
import csv
import json
from pathlib import Path

TOP_K = 5
PARAM_KEYS = ("process_noise_pos", "process_noise_vel", "measurement_noise")


def combo_key(row):
    return tuple(row[k] for k in PARAM_KEYS)


def load_site(csv_path):
    rows = []
    with open(csv_path) as f:
        for row in csv.DictReader(f):
            if row.get("error"):
                continue
            row["composite_score"] = float(row["composite_score"])
            rows.append(row)
    rows.sort(key=lambda r: r["composite_score"], reverse=True)
    return rows


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--results", action="append", required=True, dest="results_paths")
    ap.add_argument("--out", required=True)
    args = ap.parse_args()

    sites = {}
    for p in args.results_paths:
        path = Path(p)
        site_label = path.parent.name
        rows = load_site(path)
        if not rows:
            sites[site_label] = {"path": str(path), "n_rows": 0}
            continue
        scores = [r["composite_score"] for r in rows]
        sites[site_label] = {
            "path": str(path),
            "n_rows": len(rows),
            "reference_count": rows[0]["reference_count"],
            "score_range": [min(scores), max(scores)],
            "top_k": [
                {
                    "combo": {k: r[k] for k in PARAM_KEYS},
                    "composite_score": r["composite_score"],
                    "matched_count": r["matched_count"],
                    "reference_count": r["reference_count"],
                    "candidate_count": r["candidate_count"],
                }
                for r in rows[:TOP_K]
            ],
            "top_k_combo_keys": [combo_key(r) for r in rows[:TOP_K]],
        }

    scored_sites = [s for s in sites.values() if s.get("n_rows")]
    agreed = None
    if len(scored_sites) >= 2:
        common = set(scored_sites[0]["top_k_combo_keys"])
        for s in scored_sites[1:]:
            common &= set(s["top_k_combo_keys"])
        agreed = sorted(common)

    result = {
        "top_k": TOP_K,
        "param_keys": list(PARAM_KEYS),
        "sites": sites,
        "combos_in_every_sites_top_k": agreed,
        "n_sites_scored": len(scored_sites),
    }
    Path(args.out).write_text(json.dumps(result, indent=2) + "\n")

    print(f"scored {len(scored_sites)} site(s)")
    for label, s in sites.items():
        if s.get("n_rows"):
            print(f"  {label}: n={s['n_rows']} range={s['score_range']}")
        else:
            print(f"  {label}: no usable rows")
    if agreed is not None:
        print(f"combos in every site's top-{TOP_K}: {agreed}")


if __name__ == "__main__":
    main()
