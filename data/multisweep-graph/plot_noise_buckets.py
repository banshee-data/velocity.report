#!/usr/bin/env python3
"""
plot_noise_buckets.py

Reads a noise sweep raw CSV and creates individual bar charts for each noise value,
showing acceptance rates across all distance buckets.

Usage:
  python3 data/multisweep-graph/plot_noise_buckets.py --file sweep-noise-...-raw.csv --out-dir plots/

Dependencies:
  pip install pandas matplotlib numpy

"""

import argparse
import sys
from pathlib import Path

try:
    import matplotlib.pyplot as plt
    import pandas as pd
    import numpy as np
except Exception as e:
    print(
        "Missing Python dependencies for plotting:\n  pip install pandas matplotlib numpy"
    )
    print("Error details:", e)
    raise


def parse_args():
    p = argparse.ArgumentParser(
        description="Plot individual bar charts for each noise value showing acceptance rates per bucket"
    )
    p.add_argument(
        "--file", "-f", required=True, help="Raw CSV file (expanded per-bucket columns)"
    )
    p.add_argument(
        "--out-dir",
        "-o",
        default="noise-plots",
        help="Output directory for PNG files (default: noise-plots/)",
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
    p.add_argument("--dpi", type=int, default=150, help="Output image DPI")
    p.add_argument(
        "--combined",
        action="store_true",
        help="Also create a combined grid plot with all noise values",
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

    # Create output directory
    out_dir = Path(args.out_dir)
    out_dir.mkdir(parents=True, exist_ok=True)

    # Get closeness and neighbour for titles
    closeness_val = df["closeness_multiplier"].iloc[0] if len(df) > 0 else "unknown"
    neighbour_val = (
        df["neighbour_confirmation_count"].iloc[0] if len(df) > 0 else "unknown"
    )

    # Create individual plots for each noise value
    for noise in noise_vals:
        noise_data = df[df["noise_relative"] == noise]

        # Calculate mean acceptance rate per bucket
        means = noise_data[rates_cols].mean()
        stds = noise_data[rates_cols].std()

        # Create bar chart
        fig, ax = plt.subplots(figsize=(12, 6))

        x_pos = np.arange(len(bucket_labels))
        bars = ax.bar(x_pos, means.values, yerr=stds.values, capsize=5, alpha=0.8)

        # Color bars by value (green = high, red = low)
        norm = plt.Normalize(vmin=0, vmax=1)
        cmap = plt.cm.RdYlGn
        for bar, val in zip(bars, means.values):
            bar.set_color(cmap(norm(val)))

        ax.set_ylim(0, 1.05)
        ax.set_xlabel("Distance Bucket (meters)", fontsize=12)
        ax.set_ylabel("Acceptance Rate", fontsize=12)
        ax.set_title(
            f"Acceptance Rate by Distance Bucket\n"
            f"noise={noise:.4f}, closeness={closeness_val}, neighbour={neighbour_val}",
            fontsize=13,
            fontweight="bold",
        )
        ax.set_xticks(x_pos)
        ax.set_xticklabels(
            [f"{lbl}m" for lbl in bucket_labels], rotation=45, ha="right"
        )
        ax.grid(axis="y", alpha=0.3)
        ax.axhline(
            y=0.95, color="gray", linestyle="--", alpha=0.5, label="95% threshold"
        )
        ax.legend()

        plt.tight_layout()
        out_file = out_dir / f"noise-{noise:.4f}.png"
        plt.savefig(out_file, dpi=args.dpi, bbox_inches="tight")
        print(f"wrote {out_file}")
        plt.close()

    # Optionally create a combined grid plot
    if args.combined:
        n_plots = len(noise_vals)
        ncols = min(3, n_plots)
        nrows = (n_plots + ncols - 1) // ncols

        fig, axes = plt.subplots(
            nrows=nrows, ncols=ncols, figsize=(6 * ncols, 4 * nrows), squeeze=False
        )

        for idx, noise in enumerate(noise_vals):
            row = idx // ncols
            col = idx % ncols
            ax = axes[row][col]

            noise_data = df[df["noise_relative"] == noise]
            means = noise_data[rates_cols].mean()
            stds = noise_data[rates_cols].std()

            x_pos = np.arange(len(bucket_labels))
            bars = ax.bar(x_pos, means.values, yerr=stds.values, capsize=3, alpha=0.8)

            # Color bars
            norm = plt.Normalize(vmin=0, vmax=1)
            cmap = plt.cm.RdYlGn
            for bar, val in zip(bars, means.values):
                bar.set_color(cmap(norm(val)))

            ax.set_ylim(0, 1.05)
            ax.set_title(f"noise={noise:.4f}", fontsize=11)
            ax.set_xticks(x_pos)
            ax.set_xticklabels(
                [f"{lbl}" for lbl in bucket_labels], rotation=45, ha="right", fontsize=8
            )
            ax.grid(axis="y", alpha=0.3)
            ax.axhline(y=0.95, color="gray", linestyle="--", alpha=0.3)

            if col == 0:
                ax.set_ylabel("Rate", fontsize=10)
            if row == nrows - 1:
                ax.set_xlabel("Distance (m)", fontsize=10)

        # Hide unused subplots
        for idx in range(n_plots, nrows * ncols):
            row = idx // ncols
            col = idx % ncols
            axes[row][col].set_visible(False)

        fig.suptitle(
            f"Acceptance Rates Across All Noise Values\n"
            f"closeness={closeness_val}, neighbour={neighbour_val}",
            fontsize=14,
            fontweight="bold",
        )
        plt.tight_layout(rect=[0, 0, 1, 0.96])
        combined_file = out_dir / "combined-all-noise-values.png"
        plt.savefig(combined_file, dpi=args.dpi, bbox_inches="tight")
        print(f"wrote {combined_file}")
        plt.close()

    print(f"\nAll plots saved to: {out_dir}/")


if __name__ == "__main__":
    main()
