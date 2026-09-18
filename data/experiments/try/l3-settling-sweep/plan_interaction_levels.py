#!/usr/bin/env python3
"""Turns sensitivity-analysis.json into a --levels-json for
run_interaction_grid.py: picks up to the top 3 sensitive keys (ranked by
their worst value's flagged_fraction) and, for each, two levels -- its
default and its worst flagged value from Batch 1/2. This is a deliberately
narrower question than the full 3-level low/default/high grid
multi-key-interaction-grid.md originally specified: with N sensitive keys,
a 2-level joint grid (2^N combos) answers "do the single-key worst cases
compound when combined, or do they cancel/plateau" -- the core question the
experiment doc asks about interactions -- at a fraction of the cost. If that
grid finds a real interaction, a finer 3-level follow-up is the natural next
step, not something to precompute speculatively here.

Only sensitive keys are included. Fewer than 2 sensitive keys means there's
no interaction question to ask yet -- this writes an empty {} and the
supervisor should skip the interaction stage entirely.

Usage:
    python3 plan_interaction_levels.py --sensitivity sensitivity-analysis.json --out interaction-levels.json
"""

import argparse
import json
from pathlib import Path

DEFAULTS = {
    "closeness_multiplier": 3.0,
    "safety_margin_metres": 0.15,
    "noise_relative": 0.02,
    "neighbour_confirmation_count": 3,
}

MAX_KEYS = 3


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--sensitivity", required=True)
    ap.add_argument("--out", required=True)
    args = ap.parse_args()

    analysis = json.loads(Path(args.sensitivity).read_text())
    sensitive = []
    for key, info in analysis["keys"].items():
        if info["verdict"] != "sensitive":
            continue
        flagged = [
            (v, d) for v, d in info["by_value"].items() if d["sensitive_at_this_value"]
        ]
        if not flagged:
            continue
        flagged.sort(key=lambda vd: vd[1]["flagged_fraction"], reverse=True)
        worst_value, worst_info = flagged[0]
        sensitive.append((key, worst_info["flagged_fraction"], worst_value))

    sensitive.sort(key=lambda t: t[1], reverse=True)
    chosen = sensitive[:MAX_KEYS]

    levels = {}
    for key, fraction, worst_value in chosen:
        default = DEFAULTS[key]
        levels[key] = sorted({default, float(worst_value)})

    Path(args.out).write_text(json.dumps(levels, indent=2))

    if len(levels) < 2:
        print(
            f"only {len(levels)} sensitive key(s) with a usable worst value -- no interaction question to ask, wrote empty/trivial levels"
        )
    else:
        print(
            f"wrote {args.out}: {len(levels)} keys, {2 ** len(levels)} combos -- {levels}"
        )


if __name__ == "__main__":
    main()
