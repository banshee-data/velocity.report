#!/usr/bin/env python3
"""
plot_multisweep.py

Reads a bg-multisweep raw CSV (expanded per-bucket columns) and creates a grid of line
charts. Grid columns are noise_relative values, grid rows are closeness_multiplier values.
Each subplot shows acceptance rates over iterations for all buckets (one line per bucket).

Usage:
  python3 tools/plot_multisweep.py --file bg-multisweep-...-raw.csv --neighbour 3 --out plot.png

Dependencies:
  pip install pandas matplotlib numpy

"""

import argparse
import sys
from pathlib import Path

try:
    import matplotlib.pyplot as plt
    import numpy as np
    import pandas as pd
except Exception as e:
    print(
        "Missing Python dependencies for plotting:\n  pip install -r tools/requirements.txt"
    )
    print("Error details:", e)
    raise


def detect_bucket_suffixes(cols):
    # find acceptance_rates_* columns and extract bucket labels in order
    rates = [c for c in cols if c.startswith("acceptance_rates_")]
    # keep in header order
    return rates


def parse_args():
    p = argparse.ArgumentParser(
        description="Plot multisweep raw CSV into grid of line charts"
    )
    p.add_argument(
        "--file", "-f", required=True, help="Raw CSV file (expanded per-bucket columns)"
    )
    p.add_argument(
        "--neighbour",
        "-n",
        type=int,
        default=None,
        help="Filter by neighbour_confirmation_count (optional)",
    )
    p.add_argument(
        "--out", "-o", default="multisweep-plot.png", help="Output PNG filename"
    )
    p.add_argument(
        "--max-cols",
        type=int,
        default=6,
        help="Max number of noise columns to plot (will pick central ones if many)",
    )
    p.add_argument(
        "--max-rows",
        type=int,
        default=6,
        help="Max number of closeness rows to plot (will pick central ones if many)",
    )
    p.add_argument(
        "--show-iterations",
        action="store_true",
        help="Plot individual iteration lines (faint) in each subplot",
    )
    p.add_argument("--dpi", type=int, default=150, help="Output image DPI")
    return p.parse_args()


def main():
    args = parse_args()
    csv_path = Path(args.file)
    if not csv_path.exists():
        print("file not found:", csv_path)
        sys.exit(2)

    df = pd.read_csv(csv_path)

    # ensure key columns exist
    for col in [
        "noise_relative",
        "closeness_multiplier",
        "neighbour_confirmation_count",
        "iter",
    ]:
        if col not in df.columns:
            print(f"missing required column: {col}")
            sys.exit(2)

    if args.neighbour is not None:
        df = df[df["neighbour_confirmation_count"] == args.neighbour]
        if df.empty:
            print(f"no rows for neighbour_confirmation_count={args.neighbour}")
            sys.exit(0)

    # detect acceptance_rates columns and corresponding buckets
    rates_cols = [c for c in df.columns if c.startswith("acceptance_rates_")]
    if not rates_cols:
        print("no acceptance_rates_* columns found in CSV")
        sys.exit(2)

    # derive bucket labels from column names
    bucket_labels = [c.replace("acceptance_rates_", "") for c in rates_cols]

    # get unique sorted noise and closeness
    noise_vals = sorted(df["noise_relative"].unique())
    clos_vals = sorted(df["closeness_multiplier"].unique())

    # limit grid size if needed
    if len(noise_vals) > args.max_cols:
        # pick evenly spaced subset centered
        idxs = np.linspace(0, len(noise_vals) - 1, args.max_cols, dtype=int)
        noise_vals = [noise_vals[i] for i in idxs]
    if len(clos_vals) > args.max_rows:
        idxs = np.linspace(0, len(clos_vals) - 1, args.max_rows, dtype=int)
        clos_vals = [clos_vals[i] for i in idxs]

    nrows = len(clos_vals)
    ncols = len(noise_vals)

    fig, axes = plt.subplots(
        nrows=nrows, ncols=ncols, figsize=(3 * ncols, 2.5 * nrows), squeeze=False
    )

    # color map for buckets
    cmap = plt.get_cmap("tab10")
    n_buckets = len(bucket_labels)
    colors = [cmap(i % 10) for i in range(n_buckets)]

    for r_idx, clos in enumerate(clos_vals):
        for c_idx, noise in enumerate(noise_vals):
            ax = axes[r_idx][c_idx]
            sel = df[
                (df["closeness_multiplier"] == clos) & (df["noise_relative"] == noise)
            ]
            if sel.empty:
                ax.text(
                    0.5,
                    0.5,
                    "no data",
                    ha="center",
                    va="center",
                    fontsize=10,
                    color="gray",
                )
                ax.set_xticks([])
                ax.set_yticks([])
                continue

            # sort by iter to maintain line order
            sel = sel.sort_values("iter")
            iters = sel["iter"].values

            # For each bucket, plot acceptance rate over iterations (each row is one iter)
            for bi, colname in enumerate(rates_cols):
                vals = sel[colname].values
                if args.show_iterations:
                    ax.plot(iters, vals, color=colors[bi], alpha=0.15, linewidth=0.8)
                # plot mean as a thicker line
                ax.plot(
                    iters,
                    vals,
                    color=colors[bi],
                    alpha=0.9,
                    linewidth=1.5,
                    label=bucket_labels[bi],
                )

            ax.set_ylim(-0.05, 1.05)
            if r_idx == (nrows - 1):
                ax.set_xlabel("iter")
            if c_idx == 0:
                ax.set_ylabel(f"close={clos}\nrate")
            ax.set_title(f"noise={noise}")
            if r_idx == 0 and c_idx == ncols - 1:
                # place a legend on the top-right subplot only
                ax.legend(
                    loc="upper left", bbox_to_anchor=(1.02, 1.0), fontsize="small"
                )

    fig.suptitle(
        f'Multisweep: neighbour={args.neighbour if args.neighbour is not None else "all"} — lines=acceptance_rates per bucket'
    )
    plt.tight_layout(rect=[0, 0.03, 1, 0.95])
    plt.savefig(args.out, dpi=args.dpi)
    print("wrote", args.out)


if __name__ == "__main__":
    main()
