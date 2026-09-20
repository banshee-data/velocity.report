#!/usr/bin/env python3
"""Read a scorecard sweep: each config against the baseline at the same site.

What this does and does not do. It reports, per config and headline metric, the
direction of the change at each site and how many sites agree. It never ranks
configs, never picks a value, and never pools sites into one number: a change
that helps at 20 of 24 sites and one that helps on average because of three
busy sites are different findings, and only the first is reported as such.

Determinism is checked before anything is compared. Every site has the baseline
run twice as two independent replays with independently written evidence
(`baseline` and `baseline_again`). Their scorecards must be byte-identical. A
site where they are not is excluded and listed, and the exit code is 2: every
difference this script reports rests on the noise floor being zero, so a site
where it is not zero has no readable result.

Output is canonical (sorted keys, no timestamps) so the same results.csv always
gives the same bytes.

Usage:
    python3 analyze_scorecard_sweep.py --results <out-root>/results.csv \
        --out scorecard-analysis.json
"""

import argparse
import csv
import json
import sys
from pathlib import Path

BASELINE = "baseline"
DUPLICATE = "baseline_again"

# A site "moves" on a metric when the change clears both floors. The noise
# floor is zero by construction, so these are about materiality, not noise.
REL_FLOOR = 0.05
ABS_FLOORS = {
    "tracks": 3,
    "lifetime_median_seconds": 0.1,
    "lifetime_p90_seconds": 0.5,
    "share_under_one_second": 0.02,
    "association_density": 0.01,
    "contested_share": 0.02,
    "replaced_share": 0.02,
    "vanished_share": 0.02,
    "unassigned_nearby_share": 0.02,
    "clusters": 10,
    "clusters_unassigned": 10,
    "coast_1s_p50_metres": 0.05,
    "coast_1s_p95_metres": 0.10,
    "moving_mean_nis": 0.1,
    "moving_nis_exceedance": 0.01,
}


def load_scorecard(path):
    doc = json.loads(Path(path).read_text())
    if len(doc["sources"]) != 1:
        raise ValueError(
            f"{path}: expected one evidence source, got {len(doc['sources'])}"
        )
    return doc["sources"][0]["scorecard"]


def weighted(cells, key, weight="count"):
    total = sum(c[weight] for c in cells)
    if total == 0:
        return None
    return sum(c[key] * c[weight] for c in cells) / total


def headline(sc):
    """The metrics configs are compared on. Everything else stays in the
    per-run scorecard.json for anyone who wants it."""
    pop = sc["population"]
    term = {c["class"]: c for c in sc["termination"]["by_class"]}
    out = {
        "tracks": pop["tracks"],
        "lifetime_median_seconds": pop["lifetime_median_seconds"],
        "lifetime_p90_seconds": pop["lifetime_p90_seconds"],
        "share_under_one_second": pop["share_under_one_second"],
        "association_density": pop["association_density"],
        "clusters": pop["clusters"],
        "clusters_unassigned": pop["clusters_unassigned"],
    }
    for name in ("contested", "replaced", "vanished", "unassigned_nearby"):
        out[f"{name}_share"] = term[name]["share"]
    # Coast error at 1 s for tracks moving at 2 m/s or more, all deceleration
    # classes pooled by count. Percentiles cannot be pooled exactly; the
    # count-weighted mean of per-cell percentiles is a summary, and the cells
    # themselves are in the scorecard.
    coast = [
        c
        for c in sc["coast"]
        if c["horizon_seconds"] == 1.0 and c["speed_floor_mps"] >= 2
    ]
    out["coast_1s_p50_metres"] = weighted(coast, "p50_metres")
    out["coast_1s_p95_metres"] = weighted(coast, "p95_metres")
    moving = [c for c in sc["residuals"] if c["moving"]]
    out["moving_mean_nis"] = weighted(moving, "mean_nis")
    out["moving_nis_exceedance"] = weighted(moving, "nis_exceedance_ratio")
    return out


def direction(metric, base, value):
    if base is None or value is None:
        return "unavailable"
    delta = value - base
    floor = max(ABS_FLOORS[metric], REL_FLOOR * abs(base))
    if abs(delta) < floor:
        return "unchanged"
    return "up" if delta > 0 else "down"


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--results", action="append", required=True)
    ap.add_argument("--out", required=True)
    args = ap.parse_args()

    rows = []
    for path in args.results:
        with open(path, newline="") as f:
            rows += list(csv.DictReader(f))
    # Last good row wins for a (site, config) that was retried.
    good = {}
    errors = []
    for r in rows:
        if r.get("error"):
            errors.append(
                {"site": r["site"], "config": r["config"], "error": r["error"][:300]}
            )
            continue
        if not r.get("scorecard_path"):
            errors.append(
                {"site": r["site"], "config": r["config"], "error": "no scorecard"}
            )
            continue
        good[(r["site"], r["config"])] = r

    sites = sorted({s for s, _ in good})
    configs = sorted({c for _, c in good} - {BASELINE, DUPLICATE})

    determinism = {"verified_sites": [], "failed_sites": [], "unchecked_sites": []}
    usable = []
    for site in sites:
        a, b = good.get((site, BASELINE)), good.get((site, DUPLICATE))
        if a is None:
            determinism["unchecked_sites"].append(
                {"site": site, "reason": "no baseline"}
            )
            continue
        if b is None:
            determinism["unchecked_sites"].append(
                {"site": site, "reason": "no duplicate baseline"}
            )
            continue
        if a["scorecard_sha256"] != b["scorecard_sha256"]:
            determinism["failed_sites"].append(
                {
                    "site": site,
                    "baseline": a["scorecard_sha256"],
                    "baseline_again": b["scorecard_sha256"],
                }
            )
            continue
        determinism["verified_sites"].append(site)
        usable.append(site)

    # Each scorecard is read once.
    heads = {
        key: headline(load_scorecard(row["scorecard_path"]))
        for key, row in sorted(good.items())
        if key[0] in usable
    }
    baselines = {site: heads[(site, BASELINE)] for site in usable}
    metrics = sorted(ABS_FLOORS)

    by_config = {}
    for config in configs:
        per_metric = {}
        n_sites = 0
        for site in usable:
            if (site, config) in good:
                n_sites += 1
        for metric in metrics:
            counts = {"up": 0, "down": 0, "unchanged": 0, "unavailable": 0}
            per_site = {}
            for site in usable:
                if (site, config) not in heads:
                    continue
                value = heads[(site, config)][metric]
                base = baselines[site][metric]
                d = direction(metric, base, value)
                counts[d] += 1
                per_site[site] = {"baseline": base, "value": value, "direction": d}
            per_metric[metric] = {"counts": counts, "sites": per_site}
        by_config[config] = {"n_sites": n_sites, "metrics": per_metric}

    result = {
        "rule": {
            "relative_floor": REL_FLOOR,
            "absolute_floors": ABS_FLOORS,
            "note": "a site moves on a metric when |delta| >= max(absolute floor, relative floor x |baseline|)",
        },
        "determinism": determinism,
        "n_sites_usable": len(usable),
        "errors": errors,
        "baselines": baselines,
        "configs": by_config,
    }
    Path(args.out).write_text(json.dumps(result, indent=2, sort_keys=True) + "\n")

    print(
        f"determinism: {len(determinism['verified_sites'])} site(s) verified, "
        f"{len(determinism['failed_sites'])} failed, {len(determinism['unchecked_sites'])} unchecked; "
        f"{len(errors)} errored run(s)"
    )
    for config in configs:
        m = by_config[config]["metrics"]
        parts = []
        for metric in (
            "tracks",
            "lifetime_median_seconds",
            "contested_share",
            "vanished_share",
        ):
            c = m[metric]["counts"]
            parts.append(
                f"{metric} up {c['up']} / down {c['down']} / same {c['unchanged']}"
            )
        print(f"{config} ({by_config[config]['n_sites']} sites): " + "; ".join(parts))
    return 2 if determinism["failed_sites"] else 0


if __name__ == "__main__":
    sys.exit(main())
