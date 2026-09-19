#!/usr/bin/env python3
"""Run-to-run noise floor of settling-eval, from repeat-results.csv
(run_repeat_check.py), and whether the campaign's sensitivity verdicts
survive once unstable sites are set aside.

Per (site, label): a site is *bit-identical* if every repeat reports the same
value for every outcome field, and *unstable* if its settling frames differ by
>= ABS_FRAME_DELTA between repeats or if some repeats converged and others did
not (the same 10-frame bar analyze_sensitivity.py uses to call something a
real effect -- a site that wobbles by that much on an unchanged config can't
support a per-site claim of that size).

Then, given a --sweep-results results.csv, recomputes analyze_sensitivity's
per-key verdict with every unstable site removed, and reports whether each
key's verdict changes. The verdict rule itself is untouched; only the site
set differs, so any change is attributable to the noisy sites.

Usage:
    python3 analyze_repeat_check.py --results repeat-results.csv \
        --sweep-results results.csv --out repeat-check-analysis.json
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
    OUTCOME_FIELDS,
    analyze,
    to_int_or_none,
)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--results", required=True)
    ap.add_argument("--sweep-results", default="")
    ap.add_argument("--out", required=True)
    args = ap.parse_args()

    with open(args.results) as f:
        rows = [r for r in csv.DictReader(f) if not r.get("error")]

    groups = defaultdict(list)
    for r in rows:
        groups[(r["label"], r["site_id"])].append(r)

    labels = defaultdict(dict)
    unstable_sites = set()
    for (label, site), reps in sorted(groups.items()):
        if len(reps) < 2:
            continue
        frames = [to_int_or_none(r["recommended_settling_frame"]) for r in reps]
        converged = [fr is not None and fr >= 0 for fr in frames]
        good = [fr for fr, c in zip(frames, converged) if c]
        identical = all(
            all(r[fld] == reps[0][fld] for fld in OUTCOME_FIELDS) for r in reps
        )
        flip = any(converged) and not all(converged)
        spread = (max(good) - min(good)) if len(good) >= 2 else 0
        unstable = flip or spread >= ABS_FRAME_DELTA
        if unstable:
            unstable_sites.add(site)
        labels[label][site] = {
            "n_repeats": len(reps),
            "frames": frames,
            "bit_identical": identical,
            "frame_spread": spread,
            "convergence_flip": flip,
            "unstable": unstable,
        }

    summary = {}
    for label, sites in labels.items():
        spreads = [s["frame_spread"] for s in sites.values()]
        summary[label] = {
            "n_sites": len(sites),
            "n_bit_identical": sum(s["bit_identical"] for s in sites.values()),
            "n_unstable": sum(s["unstable"] for s in sites.values()),
            "max_frame_spread": max(spreads) if spreads else None,
            "unstable_sites": sorted(k for k, v in sites.items() if v["unstable"]),
        }

    verdict_check = None
    if args.sweep_results:
        with open(args.sweep_results) as f:
            all_rows = [
                r
                for r in csv.DictReader(f)
                if not r.get("error") and r.get("capture_ordinal", "0") == "0"
            ]
        before = analyze(all_rows)
        after = analyze([r for r in all_rows if r["site_id"] not in unstable_sites])
        verdict_check = {
            "excluded_sites": sorted(unstable_sites),
            "keys": {
                key: {
                    "verdict_all_sites": before["keys"][key]["verdict"],
                    "verdict_without_unstable_sites": after["keys"][key]["verdict"],
                    "changed": before["keys"][key]["verdict"]
                    != after["keys"][key]["verdict"],
                }
                for key in before["keys"]
                if key in after["keys"]
            },
        }

    result = {
        "abs_frame_delta_threshold": ABS_FRAME_DELTA,
        "labels": summary,
        "unstable_sites_any_label": sorted(unstable_sites),
        "verdict_check": verdict_check,
        "per_site": labels,
    }
    Path(args.out).write_text(json.dumps(result, indent=2) + "\n")

    for label, s in summary.items():
        print(
            f"{label}: {s['n_bit_identical']}/{s['n_sites']} sites bit-identical across "
            f"repeats, {s['n_unstable']} unstable, max frame spread {s['max_frame_spread']}"
        )
    if verdict_check:
        changed = [k for k, v in verdict_check["keys"].items() if v["changed"]]
        print(
            f"verdicts changed after excluding {len(unstable_sites)} unstable site(s): "
            f"{changed if changed else 'none'}"
        )


if __name__ == "__main__":
    main()
