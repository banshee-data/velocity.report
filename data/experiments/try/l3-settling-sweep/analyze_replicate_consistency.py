#!/usr/bin/env python3
"""Compares the same (site, key, value) sensitivity verdict across two
capture ordinals (0 = Batch 1's broad sweep, 1 = the replicate pass on a
second, independent window of the same site) to check a finding isn't an
artifact of one specific 120s window.

Deterministic rule: re-runs analyze_sensitivity's per-key verdict
separately for each ordinal present in results.csv, then reports, per key,
whether the verdict (sensitive/insensitive) agrees between ordinals. A key
whose verdict flips is flagged explicitly -- that is itself an important,
reportable finding (either the effect is window-dependent, or one of the
two windows is atypical), not something to silently average away.

Usage:
    python3 analyze_replicate_consistency.py --results results.csv --out replicate-consistency.json
"""

import argparse
import csv
import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from analyze_sensitivity import analyze  # noqa: E402


def read_rows(results_csv):
    with open(results_csv) as f:
        return [r for r in csv.DictReader(f) if not r.get("error")]


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--results", required=True)
    ap.add_argument("--out", required=True)
    args = ap.parse_args()

    rows = read_rows(args.results)
    ordinals = sorted({r.get("capture_ordinal", "0") for r in rows})

    per_ordinal = {}
    for ordinal in ordinals:
        subset = [r for r in rows if r.get("capture_ordinal", "0") == ordinal]
        per_ordinal[ordinal] = analyze(subset)

    all_keys = set()
    for a in per_ordinal.values():
        all_keys.update(a["keys"].keys())

    comparison = {}
    for key in sorted(all_keys):
        verdicts = {
            o: per_ordinal[o]["keys"].get(key, {}).get("verdict", "not_tested")
            for o in ordinals
        }
        # A key deliberately replicated only where it is live (see
        # plan_replicate_sweep.py) is "not_tested" at the ordinals it was
        # skipped on; that is absence of a replicate, not disagreement.
        tested = {v for v in verdicts.values() if v != "not_tested"}
        agree = len(tested) <= 1
        comparison[key] = {
            "verdict_by_ordinal": verdicts,
            "agrees_across_ordinals": agree,
            "replicated": sum(v != "not_tested" for v in verdicts.values()) >= 2,
        }

    result = {
        "ordinals_compared": ordinals,
        "keys": comparison,
        "any_disagreement": any(
            not v["agrees_across_ordinals"] for v in comparison.values()
        ),
    }
    Path(args.out).write_text(json.dumps(result, indent=2) + "\n")

    for key, info in comparison.items():
        status = "AGREE" if info["agrees_across_ordinals"] else "DISAGREE"
        if not info["replicated"]:
            status = "NOT-REPLICATED"
        print(f"{key}: {status} -- {info['verdict_by_ordinal']}")


if __name__ == "__main__":
    main()
