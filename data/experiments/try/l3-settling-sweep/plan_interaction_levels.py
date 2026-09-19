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

--policy picks which flagged value becomes the "bad" level. "worst" (default)
takes the most-flagged value; "mildest" takes the flagged value nearest the
default. Use "mildest" when the worst values already saturate on their own: a
2x2 whose single-key runs all fail to converge cannot show an interaction
(background_update_fraction=0.2 and seed_from_first=False did exactly that on
2026-09-18, and the grid scored 0 of 24 sites).

A key is dropped, under either policy, when no site converges at its chosen
value (mean_frame_delta is None): with no frame to subtract, a grid containing
it cannot be scored, and the shortfall is the finding, not something to
interact with another key.

Usage:
    python3 plan_interaction_levels.py --sensitivity sensitivity-analysis.json --out interaction-levels.json
"""

import argparse
import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from param_types import coerce  # noqa: E402

# Defaults come from the live tuning config (not a hand-copied dict) so keys
# added to the sweep later don't need a second edit here. coerce() applies the
# Go-side types: a blanket float() cast wrote neighbour_confirmation_count=1.0
# on 2026-09-18 and settling-eval's strict unmarshal rejected 24/24 sites.
_TUNING = Path(__file__).resolve().parents[4] / "config" / "tuning.defaults.json"
DEFAULTS = {
    k: coerce(k, v)
    for k, v in json.loads(_TUNING.read_text())["l3"]["ema_baseline_v1"].items()
    if isinstance(v, (int, float, bool))
}

MAX_KEYS = 3


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--sensitivity", required=True)
    ap.add_argument("--out", required=True)
    ap.add_argument(
        "--max-keys",
        type=int,
        default=MAX_KEYS,
        help="analyze_interaction_grid.py only scores exactly 2-key grids, so "
        "campaign stages that feed it pass 2",
    )
    ap.add_argument(
        "--only-keys",
        default="",
        help="comma-separated keys to restrict candidates to (default: any "
        "sensitive key)",
    )
    ap.add_argument(
        "--policy",
        choices=("worst", "mildest"),
        default="worst",
        help="which flagged value is the bad level (see module docstring)",
    )
    args = ap.parse_args()
    only = {k for k in args.only_keys.split(",") if k}

    analysis = json.loads(Path(args.sensitivity).read_text())
    sensitive = []
    unscorable = []
    for key, info in analysis["keys"].items():
        if info["verdict"] != "sensitive":
            continue
        if only and key not in only:
            continue
        flagged = [
            (v, d) for v, d in info["by_value"].items() if d["sensitive_at_this_value"]
        ]
        if not flagged:
            continue
        if args.policy == "mildest":
            flagged.sort(
                key=lambda vd: abs(float(coerce(key, vd[0])) - float(DEFAULTS[key]))
            )
        else:
            flagged.sort(key=lambda vd: vd[1]["flagged_fraction"], reverse=True)
        worst_value, worst_info = flagged[0]
        if worst_info.get("mean_frame_delta") is None:
            unscorable.append(key)
            continue
        sensitive.append((key, worst_info["flagged_fraction"], worst_value))

    sensitive.sort(key=lambda t: t[1], reverse=True)
    chosen = sensitive[: args.max_keys]

    levels = {}
    for key, fraction, worst_value in chosen:
        levels[key] = sorted({DEFAULTS[key], coerce(key, worst_value)})

    Path(args.out).write_text(json.dumps(levels, indent=2) + "\n")
    if unscorable:
        print(f"dropped (no site converges at the chosen value): {sorted(unscorable)}")

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
