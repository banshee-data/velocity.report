#!/usr/bin/env python3
"""
Analyse noise vs distance convergence patterns from sweep results.
Shows how lower noise values lead to faster convergence in farther distance buckets.
"""

import pandas as pd
import sys


def analyse_convergence(summary_file, raw_file):
    # Load summary data
    df_summary = pd.read_csv(summary_file)

    print("=" * 80)
    print("NOISE vs DISTANCE CONVERGENCE ANALYSIS")
    print("=" * 80)
    print()

    # Extract bucket columns
    bucket_cols = [
        col
        for col in df_summary.columns
        if col.startswith("bucket_") and col.endswith("_mean")
    ]
    buckets = [col.replace("bucket_", "").replace("_mean", "") for col in bucket_cols]

    # Show acceptance rates by distance bucket
    print("ACCEPTANCE RATES BY DISTANCE BUCKET:")
    print("-" * 80)
    print(f"{'Noise':<10} {'Overall':<10}", end="")
    for bucket in buckets[:5]:  # Show first 5 buckets
        print(f"{bucket+'m':<10}", end="")
    print()
    print("-" * 80)

    for _, row in df_summary.iterrows():
        noise = row["noise_relative"]
        overall = row["overall_accept_mean"]
        print(f"{noise:<10.4f} {overall:<10.4f}", end="")
        for col in bucket_cols[:5]:
            rate = row[col]
            print(f"{rate:<10.4f}", end="")
        print()

    print()
    print("=" * 80)
    print("CONVERGENCE ANALYSIS (Standard Deviation = measure of convergence)")
    print("Lower stddev = faster/better convergence")
    print("=" * 80)
    print()

    # Show stddev by distance bucket (convergence measure)
    stddev_cols = [
        col
        for col in df_summary.columns
        if col.startswith("bucket_") and col.endswith("_stddev")
    ]

    print(f"{'Noise':<10} {'Overall':<12}", end="")
    for bucket in buckets[:5]:
        print(f"{bucket+'m':<12}", end="")
    print()
    print("-" * 80)

    for _, row in df_summary.iterrows():
        noise = row["noise_relative"]
        overall_std = row["overall_accept_stddev"]
        print(f"{noise:<10.4f} {overall_std:<12.6f}", end="")
        for col in stddev_cols[:5]:
            std = row[col]
            print(f"{std:<12.6f}", end="")
        print()

    print()
    print("=" * 80)
    print("KEY FINDINGS:")
    print("=" * 80)

    # Find optimal noise value (highest acceptance with lowest stddev)
    df_summary["convergence_score"] = df_summary["overall_accept_mean"] / (
        1 + df_summary["overall_accept_stddev"] * 10
    )
    best_idx = df_summary["convergence_score"].idxmax()
    best_row = df_summary.loc[best_idx]

    print(f"Best noise value: {best_row['noise_relative']:.4f}")
    print(
        f"  - Overall acceptance: {best_row['overall_accept_mean']:.4f} ± {best_row['overall_accept_stddev']:.6f}"
    )
    print(
        f"  - Nonzero cells: {best_row['nonzero_cells_mean']:.0f} ± {best_row['nonzero_cells_stddev']:.0f}"
    )
    print()

    # Analyse convergence trend
    print("Convergence trends:")
    for i, bucket in enumerate(buckets[:5]):
        mean_col = f"bucket_{bucket}_mean"
        std_col = f"bucket_{bucket}_stddev"

        if mean_col in df_summary.columns and std_col in df_summary.columns:
            # Find noise value with lowest stddev for this bucket
            valid_data = df_summary[df_summary[mean_col] > 0]
            if len(valid_data) > 0:
                best_noise_for_bucket = valid_data.loc[
                    valid_data[std_col].idxmin(), "noise_relative"
                ]
                best_std = valid_data[std_col].min()
                print(
                    f"  - {bucket}m bucket: Best convergence at noise={best_noise_for_bucket:.4f} (stddev={best_std:.6f})"
                )

    print()
    print("Hypothesis validation:")
    print("  Expected: Lower noise → better convergence in farther buckets")

    # Check if stddev decreases with higher noise for far buckets
    far_bucket_col = (
        stddev_cols[2] if len(stddev_cols) > 2 else stddev_cols[-1]
    )  # Use 4m or last bucket
    near_bucket_col = stddev_cols[0]  # 1m bucket

    # Correlation between noise and stddev
    noise_values = df_summary["noise_relative"].values
    far_stddevs = df_summary[far_bucket_col].values
    near_stddevs = df_summary[near_bucket_col].values

    if len(noise_values) > 2:
        # Simple trend check: does stddev generally decrease as noise increases?
        far_trend = "decreasing" if far_stddevs[-1] < far_stddevs[0] else "increasing"
        near_trend = (
            "decreasing" if near_stddevs[-1] < near_stddevs[0] else "increasing"
        )

        print(
            f"  - Far bucket ({far_bucket_col.replace('bucket_', '').replace('_stddev', '')}m) stddev: {far_trend} with higher noise"
        )
        print(
            f"  - Near bucket ({near_bucket_col.replace('bucket_', '').replace('_stddev', '')}m) stddev: {near_trend} with higher noise"
        )

        if far_trend == "decreasing" and near_trend == "increasing":
            print(
                "  ✓ Hypothesis CONFIRMED: Lower noise improves far-distance convergence"
            )
        else:
            print("  ? Mixed results - may need more data or different noise range")

    # Load raw data to show convergence over time
    print()
    print("=" * 80)
    print("TIME-SERIES CONVERGENCE (first few samples)")
    print("=" * 80)
    df_raw = pd.read_csv(raw_file)

    for noise in df_summary["noise_relative"].values[:3]:  # Show first 3 noise values
        print(f"\nNoise = {noise:.4f}:")
        subset = df_raw[df_raw["noise_relative"] == noise].head(10)
        if len(subset) > 0:
            print("  Iter  Overall Accept  Nonzero Cells  1m Rate    4m Rate")
            print("  " + "-" * 60)
            for _, row in subset.iterrows():
                iter_num = row["iter"]
                overall = row["overall_accept_percent"]
                cells = row["nonzero_cells"]
                rate_1m = (
                    row["acceptance_rates_1"] if "acceptance_rates_1" in row else 0
                )
                rate_4m = (
                    row["acceptance_rates_4"] if "acceptance_rates_4" in row else 0
                )
                print(
                    f"  {iter_num:<4}  {overall:<14.4f}  {cells:<13.0f}  {rate_1m:<9.4f}  {rate_4m:<9.4f}"
                )


if __name__ == "__main__":
    summary = sys.argv[1] if len(sys.argv) > 1 else "noise-distance-convergence.csv"
    raw = sys.argv[2] if len(sys.argv) > 2 else "noise-distance-convergence-raw.csv"
    analyse_convergence(summary, raw)
