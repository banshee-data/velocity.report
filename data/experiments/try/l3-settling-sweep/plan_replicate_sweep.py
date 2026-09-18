#!/usr/bin/env python3
"""Picks which sweep keys are worth replaying on the independent second
capture window (ordinal 1), from an ordinal-0 sensitivity-analysis.json.

Rule: replicate a key unless it is *inert* (analyze_sensitivity.py: every row
at every site was identical to its baseline, i.e. the setting is not being
consumed at all). Sensitive keys need the replicate to confirm the effect is
not window-specific; insensitive-but-live keys need it to confirm "insensitive"
isn't an artifact of one window either -- pass 1's replicate treated both
symmetrically. Only a key with no observable effect anywhere has nothing to
confirm. Writes {key: [values tested at ordinal 0]} for run_sweep.py
--sweep-json, restricted to --keys when given.

Usage:
    python3 plan_replicate_sweep.py --sensitivity sensitivity-analysis-v3.json \
        --keys background_update_fraction,seed_from_first --out replicate-sweep.json
"""

import argparse
import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from param_types import coerce  # noqa: E402


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--sensitivity", required=True)
    ap.add_argument("--keys", default="", help="comma-separated; default: all keys")
    ap.add_argument("--out", required=True)
    args = ap.parse_args()
    only = {k for k in args.keys.split(",") if k}

    analysis = json.loads(Path(args.sensitivity).read_text())
    spec = {}
    skipped_inert = []
    for key, info in analysis["keys"].items():
        if only and key not in only:
            continue
        if info.get("inert"):
            skipped_inert.append(key)
            continue
        spec[key] = [coerce(key, v) for v in info["by_value"]]

    Path(args.out).write_text(json.dumps(spec, indent=2) + "\n")
    print(f"replicate: {sorted(spec)}; skipped inert: {sorted(skipped_inert)}")


if __name__ == "__main__":
    main()
