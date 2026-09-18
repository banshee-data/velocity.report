#!/usr/bin/env python3
"""Deterministic summary of run_gt_oat_sweep.py results: which L4/L5 keys the
ground-truth metric can see, and whether any setting is a clear improvement.

This never picks a "best" value from composite_score. That score is dominated
by a false-positive term that counts every candidate track matching an
*unlabelled* reference track as a false positive (55-60% of the reference
runs' tracks are unlabelled), so it is close to a constant. Instead each
setting is compared to the site's own warm baseline on the two quantities the
metric measures cleanly:
  matched_count    reference tracks recovered (recall of human-labelled tracks)
  candidate_count  tracks produced -- a fragmentation/inflation proxy only

Per site the baselines (start/middle/end) give the metric's own noise floor:
  floor_m = range of matched_count over baselines
  floor_c = range of candidate_count over baselines
A site is *reliable* only if the baselines agree (floor_m <= 1 and floor_c <=
max(3, 5% of the median), so a leaking-state or nondeterministic site can't
support a conclusion); unreliable sites are reported but excluded.

Thresholds (per site): T_m = max(2, ceil(10% of reference_count), 2*floor_m),
T_c = max(5, ceil(15% of median baseline candidates), 2*floor_c). A setting
changes recall if |matched - baseline| >= T_m and changes track count if
|candidates - baseline| >= T_c. It is a *Pareto improvement* only if recall
rises by >= T_m while candidates do not increase; the mirror case is a
*Pareto regression*. Everything else is a trade-off, deliberately left
unranked. A key is "visible" if any of its values changes recall or track
count on any reliable site.

Usage:
    python3 analyze_gt_oat.py --out gt-oat-analysis.json \
        --results kirk1/results.csv --results kirk0/results.csv
"""

import argparse
import csv
import json
import math
import statistics
from collections import defaultdict
from pathlib import Path


def classify(dm, dc, t_m, t_c):
    recall_up, recall_down = dm >= t_m, dm <= -t_m
    if recall_up and dc <= 0:
        return "pareto_improvement"
    if recall_down and dc >= 0:
        return "pareto_regression"
    parts = []
    if recall_up:
        parts.append("more_recall")
    if recall_down:
        parts.append("less_recall")
    if dc >= t_c:
        parts.append("more_tracks")
    if dc <= -t_c:
        parts.append("fewer_tracks")
    return "+".join(parts) if parts else "no_visible_change"


def analyze_site(rows):
    rows = [r for r in rows if not r.get("error")]
    base = [r for r in rows if r["param_path"] == "_baseline"]
    if len(base) < 2:
        return {"reliable": False, "reason": f"only {len(base)} usable baseline run(s)"}
    bm = [int(r["matched_count"]) for r in base]
    bc = [int(r["candidate_count"]) for r in base]
    ref = int(base[0]["reference_count"])
    med_m, med_c = statistics.median(bm), statistics.median(bc)
    floor_m, floor_c = max(bm) - min(bm), max(bc) - min(bc)
    reliable = floor_m <= 1 and floor_c <= max(3, 0.05 * med_c)
    out = {
        "reference_count": ref,
        "baseline_matched": bm,
        "baseline_candidates": bc,
        "reliable": reliable,
    }
    if not reliable:
        out["reason"] = (
            f"baselines disagree (matched {bm}, candidates {bc}): state is leaking "
            "between runs or the replay is nondeterministic"
        )
        return out
    t_m = max(2, math.ceil(0.10 * ref), 2 * floor_m)
    t_c = max(5, math.ceil(0.15 * med_c), 2 * floor_c)
    out["thresholds"] = {"matched": t_m, "candidates": t_c}
    keys = defaultdict(dict)
    for r in rows:
        if r["param_path"].startswith("_"):
            continue
        m, c = int(r["matched_count"]), int(r["candidate_count"])
        dm, dc = m - med_m, c - med_c
        keys[r["param_path"]][r["param_value"]] = {
            "matched": m,
            "candidates": c,
            "d_matched": dm,
            "d_candidates": dc,
            "class": classify(dm, dc, t_m, t_c),
        }
    out["keys"] = dict(keys)
    out["visible_keys"] = sorted(
        k
        for k, vals in keys.items()
        if any(v["class"] != "no_visible_change" for v in vals.values())
    )
    out["errors"] = sum(1 for r in rows if r.get("error"))
    return out


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--results", action="append", required=True, dest="paths")
    ap.add_argument("--out", required=True)
    args = ap.parse_args()

    sites = {}
    for p in args.paths:
        with open(p) as f:
            all_rows = list(csv.DictReader(f))
        n_err = sum(1 for r in all_rows if r.get("error"))
        site = analyze_site(all_rows)
        site["error_rows"] = n_err
        sites[Path(p).parent.name] = site

    reliable = {k: v for k, v in sites.items() if v.get("reliable")}
    cross = {}
    all_keys = sorted({k for v in reliable.values() for k in v.get("keys", {})})
    for key in all_keys:
        per_value = defaultdict(dict)
        for site, v in reliable.items():
            for value, d in v.get("keys", {}).get(key, {}).items():
                per_value[value][site] = d["class"]
        cross[key] = {
            "visible_on_sites": sorted(
                s for s, v in reliable.items() if key in v.get("visible_keys", [])
            ),
            "by_value": {
                val: {
                    "classes": classes,
                    "consistent_pareto_improvement": len(classes) == len(reliable)
                    and all(c == "pareto_improvement" for c in classes.values()),
                    "consistent_pareto_regression": len(classes) == len(reliable)
                    and all(c == "pareto_regression" for c in classes.values()),
                }
                for val, classes in sorted(per_value.items())
            },
        }

    result = {
        "n_sites": len(sites),
        "n_reliable_sites": len(reliable),
        "sites": sites,
        "cross_site": cross,
        "consistent_pareto_improvements": sorted(
            f"{k}={v}"
            for k, info in cross.items()
            for v, d in info["by_value"].items()
            if d["consistent_pareto_improvement"]
        ),
    }
    Path(args.out).write_text(json.dumps(result, indent=2) + "\n")

    for name, s in sites.items():
        if not s.get("reliable"):
            print(f"{name}: UNRELIABLE -- {s.get('reason')}")
        else:
            print(
                f"{name}: reliable, T={s['thresholds']}, visible keys: "
                f"{[k.split('.')[-1] for k in s['visible_keys']]}"
            )
    print(f"consistent pareto improvements: {result['consistent_pareto_improvements']}")


if __name__ == "__main__":
    main()
