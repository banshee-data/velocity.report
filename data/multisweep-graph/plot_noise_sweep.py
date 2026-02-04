#!/usr/bin/env python3
"""
plot_noise_sweep.py

Reads a sweep raw CSV (expanded per-bucket columns) and creates a line chart showing
acceptance rates over noise values for each distance bucket.

This script is designed for single-parameter sweeps (e.g., varying noise with fixed
closeness and neighbour values). Each line represents a distance bucket, showing how
its acceptance rate changes across noise levels.

Usage:
  python3 data/multisweep-graph/plot_noise_sweep.py --file sweep-noise-...-raw.csv --out plot.png

Dependencies:
  pip install pandas matplotlib numpy

"""

import argparse
import sys
from pathlib import Path

try:
    import matplotlib.pyplot as plt
    import pandas as pd
except Exception as e:
    print(
        "Missing Python dependencies for plotting:\n  pip install pandas matplotlib numpy"
    )
    print("Error details:", e)
    raise


def parse_args():
    p = argparse.ArgumentParser(
        description="Plot noise sweep raw CSV as line chart of acceptance rates per bucket"
    )
    p.add_argument(
        "--file", "-f", required=True, help="Raw CSV file (expanded per-bucket columns)"
    )
    p.add_argument(
        "--out", "-o", default="noise-sweep-plot.png", help="Output PNG filename"
    )
    p.add_argument(
        "--neighbour",
        "-n",
        type=int,
        default=None,
        help="Filter by neighbour_confirmation_count (optional)",
    )
    p.add_argument(
        "--closeness",
        "-c",
        type=float,
        default=None,
        help="Filter by closeness_multiplier (optional)",
    )
    p.add_argument(
        "--show-iterations",
        action="store_true",
        help="Show individual iteration data points (faint) in addition to mean line",
    )
    p.add_argument("--dpi", type=int, default=150, help="Output image DPI")
    p.add_argument(
        "--title",
        type=str,
        default=None,
        help="Custom plot title (default: auto-generated from parameters)",
    )
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

    # Apply filters if specified
    if args.neighbour is not None:
        df = df[df["neighbour_confirmation_count"] == args.neighbour]
        if df.empty:
            print(f"no rows for neighbour_confirmation_count={args.neighbour}")
            sys.exit(0)

    if args.closeness is not None:
        df = df[df["closeness_multiplier"] == args.closeness]
        if df.empty:
            print(f"no rows for closeness_multiplier={args.closeness}")
            sys.exit(0)

    # detect acceptance_rates columns and corresponding buckets
    rates_cols = [c for c in df.columns if c.startswith("acceptance_rates_")]
    if not rates_cols:
        print("no acceptance_rates_* columns found in CSV")
        sys.exit(2)

    # derive bucket labels from column names
    bucket_labels = [c.replace("acceptance_rates_", "") for c in rates_cols]

    # Get unique noise values
    noise_vals = sorted(df["noise_relative"].unique())

    if len(noise_vals) == 0:
        print("no noise values found in data")
        sys.exit(2)

    # Calculate mean acceptance rate per noise value per bucket
    # Group by noise_relative and calculate mean of each bucket's acceptance rate
    grouped = df.groupby("noise_relative")[rates_cols].mean()

    # Create the plot
    fig, ax = plt.subplots(figsize=(12, 7))

    # color map for buckets
    cmap = plt.get_cmap("tab20")
    n_buckets = len(bucket_labels)
    colors = [cmap(i % 20) for i in range(n_buckets)]

    # Plot each bucket as a line
    for bi, colname in enumerate(rates_cols):
        label = f"{bucket_labels[bi]}m"

        # Plot individual iterations if requested (faint)
        if args.show_iterations:
            for noise in noise_vals:
                noise_data = df[df["noise_relative"] == noise][colname]
                if len(noise_data) > 0:
                    ax.scatter(
                        [noise] * len(noise_data),
                        noise_data.values,
                        color=colors[bi],
                        alpha=0.15,
                        s=20,
                    )

        # Plot mean line
        ax.plot(
            grouped.index,
            grouped[colname].values,
            color=colors[bi],
            alpha=0.9,
            linewidth=2,
            marker="o",
            markersize=6,
            label=label,
        )

    ax.set_ylim(-0.05, 1.05)
    ax.set_xlabel("Noise Relative Fraction", fontsize=12)
    ax.set_ylabel("Acceptance Rate", fontsize=12)
    ax.grid(True, alpha=0.3)

    # Create title
    if args.title:
        title = args.title
    else:
        # Auto-generate title based on parameters
        closeness_val = df["closeness_multiplier"].iloc[0] if len(df) > 0 else "unknown"
        neighbour_val = (
            df["neighbour_confirmation_count"].iloc[0] if len(df) > 0 else "unknown"
        )
        title = f"Acceptance Rate vs Noise (closeness={closeness_val}, neighbour={neighbour_val})"

    ax.set_title(title, fontsize=14, fontweight="bold")

    # Place legend outside the plot area
    ax.legend(
        loc="center left",
        bbox_to_anchor=(1.02, 0.5),
        fontsize=10,
        title="Distance Bucket",
        title_fontsize=11,
    )

    plt.tight_layout()
    plt.savefig(args.out, dpi=args.dpi, bbox_inches="tight")
    print(f"wrote {args.out}")


if __name__ == "__main__":
    main()
