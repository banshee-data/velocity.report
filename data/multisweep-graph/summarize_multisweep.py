#!/usr/bin/env python3
"""
Lightweight summary for bg-multisweep raw CSV.
Does not require pandas/numpy; uses only the stdlib.

Usage: python3 tools/summarize_multisweep.py <raw-csv>
"""
import csv
import math
import sys
from collections import defaultdict

if len(sys.argv) < 2:
    print("Usage: python3 tools/summarize_multisweep.py <raw-csv>")
    sys.exit(2)

path = sys.argv[1]

groups = defaultdict(
    lambda: {
        "count": 0,
        "zero_totals_count": 0,
        "first_iter_zero": 0,
        "overall_vals": [],
    }
)

with open(path, newline="") as fh:
    r = csv.DictReader(fh)
    # detect totals_* columns
    totals_cols = [c for c in r.fieldnames if c.startswith("totals_")]
    for row in r:
        key = (
            row["noise_relative"],
            row["closeness_multiplier"],
            row["neighbor_confirmation_count"],
        )
        try:
            iter_n = int(row.get("iter", "0"))
        except Exception:
            iter_n = 0
        # compute sum of totals columns
        total_sum = 0
        for tc in totals_cols:
            v = row.get(tc, "")
            try:
                total_sum += int(v)
            except Exception:
                try:
                    total_sum += int(float(v))
                except Exception:
                    pass

        overall = row.get("overall_accept_percent", "")
        try:
            overall_f = float(overall)
        except Exception:
            overall_f = float("nan")

        g = groups[key]
        g["count"] += 1
        # If the zeroth iteration exists, treat it as warmup and never include it in the mean/std
        if iter_n == 0:
            if total_sum == 0:
                g["first_iter_zero"] += 1
            # do not aggregate iter==0 values into overall_vals
        else:
            if total_sum == 0:
                g["zero_totals_count"] += 1
            else:
                # only collect overall when totals > 0 and iter != 0
                if not math.isnan(overall_f):
                    g["overall_vals"].append(overall_f)

# print summary
print(
    "noise,closeness,neighbor,iters,first_iter_zero_count,zero_totals_count,overall_mean_excl_zero,overall_stddev_excl_zero,min,max"
)
for key, info in sorted(groups.items()):
    noise, closeness, neigh = key
    iters = info["count"]
    zero_totals = info["zero_totals_count"]
    first_zero = info["first_iter_zero"]
    vals = info["overall_vals"]
    if vals:
        mean = sum(vals) / len(vals)
        var = sum((x - mean) ** 2 for x in vals) / len(vals)
        std = math.sqrt(var)
        mn = min(vals)
        mx = max(vals)
        mean_s = f"{mean:.6f}"
        std_s = f"{std:.6f}"
        mn_s = f"{mn:.6f}"
        mx_s = f"{mx:.6f}"
    else:
        mean_s = std_s = mn_s = mx_s = "NaN"

    print(
        f"{noise},{closeness},{neigh},{iters},{first_zero},{zero_totals},{mean_s},{std_s},{mn_s},{mx_s}"
    )
