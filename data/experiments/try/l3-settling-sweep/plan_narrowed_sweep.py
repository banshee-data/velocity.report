#!/usr/bin/env python3
"""Turn sensitivity-analysis.json (from analyze_sensitivity.py) into a narrowed
--sweep-json for a follow-up run_sweep.py pass, restricted to keys the
deterministic analysis actually flagged as sensitive.

For each sensitive key, adds two new points not already tested:
  - "midpoint": halfway between the default and the worst flagged value --
    finer resolution in the region between "known fine" and "known bad".
  - "beyond": half again as far past the worst flagged value in the same
    direction, clipped to the experiment doc's stated sweep bounds -- checks
    whether the regression keeps getting worse (monotonic) or plateaus.
Insensitive keys are dropped entirely: Batch 1 is already conclusive for
them, so no further sweeping is warranted (recorded as such, not silently).

This is a fixed transform, not a judgement call -- same input always
produces the same narrowed plan.
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

# From l3-background-settling-sweep.md's own stated sweep ranges -- the
# range Batch 1 actually tested.
BOUNDS = {
    "closeness_multiplier": (1.5, 5.0),
    "safety_margin_metres": (0.05, 0.30),
    "noise_relative": (0.005, 0.05),
    "neighbour_confirmation_count": (1, 5),
}

# Wider hard caps used only when the worst flagged value sits at the edge of
# BOUNDS -- i.e. Batch 1's own range was too narrow to see where the effect
# plateaus. Deliberately generous but not unbounded: still real, physically
# plausible parameter values for this sensor/pipeline, not an unconstrained
# extrapolation.
SAFETY_CAPS = {
    "closeness_multiplier": (1.0, 8.0),
    "safety_margin_metres": (0.02, 0.5),
    "noise_relative": (0.002, 0.15),
    "neighbour_confirmation_count": (1, 10),
}

IS_INT = {"neighbour_confirmation_count"}


def round_value(key, v):
    if key in IS_INT:
        return int(round(v))
    return round(v, 4)


def narrow_key(key, default, worst_value, already_tested):
    lo, hi = BOUNDS[key]
    safety_lo, safety_hi = SAFETY_CAPS[key]
    direction = 1 if worst_value >= default else -1
    span = abs(worst_value - default)
    at_edge = worst_value >= hi or worst_value <= lo

    midpoint = default + direction * span * 0.5
    midpoint = max(lo, min(hi, midpoint))

    beyond = worst_value + direction * span * 0.5
    if at_edge:
        # The regression is still visible at the edge of the originally
        # tested range -- extend past it to find where it plateaus, rather
        # than re-clipping straight back to a value we've already measured.
        beyond = max(safety_lo, min(safety_hi, beyond))
    else:
        beyond = max(lo, min(hi, beyond))

    candidates = [round_value(key, midpoint), round_value(key, beyond)]
    tested = {round_value(key, float(v)) for v in already_tested} | {
        round_value(key, default)
    }
    new_values = sorted({c for c in candidates if c not in tested})
    return new_values


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--sensitivity", required=True)
    ap.add_argument("--out", required=True)
    args = ap.parse_args()

    analysis = json.loads(Path(args.sensitivity).read_text())
    narrowed = {}
    report_lines = []

    for key, info in analysis["keys"].items():
        if info["verdict"] != "sensitive":
            report_lines.append(f"{key}: insensitive, skipped (Batch 1 conclusive)")
            continue
        default = DEFAULTS[key]
        already_tested = list(info["by_value"].keys())
        # worst = the flagged value with the highest flagged_fraction, tie-broken
        # by largest mean_frame_delta.
        flagged = [
            (v, d) for v, d in info["by_value"].items() if d["sensitive_at_this_value"]
        ]
        flagged.sort(
            key=lambda vd: (vd[1]["flagged_fraction"], vd[1]["mean_frame_delta"] or 0),
            reverse=True,
        )
        worst_value = float(flagged[0][0])

        new_values = narrow_key(key, default, worst_value, already_tested)
        if new_values:
            narrowed[key] = new_values
            report_lines.append(
                f"{key}: sensitive, narrowing around worst={worst_value} -> {new_values}"
            )
        else:
            report_lines.append(
                f"{key}: sensitive but narrowing produced no new untested points, skipped"
            )

    Path(args.out).write_text(json.dumps(narrowed, indent=2) + "\n")
    for line in report_lines:
        print(line)
    print(
        f"wrote {args.out}: {sum(len(v) for v in narrowed.values())} new (key,value) points across {len(narrowed)} keys"
    )


if __name__ == "__main__":
    main()
