#!/usr/bin/env python3
"""
Test the grid heatmap plotting with mock data

This creates synthetic heatmap data and generates all visualization types
to demonstrate the plotting capabilities without requiring a running server.
"""

import json
import sys
from pathlib import Path

# Add parent directory to path to import the plotting module
sys.path.insert(0, str(Path(__file__).parent))

try:
    import numpy as np
    from plot_grid_heatmap import (
        plot_polar_heatmap,
        plot_cartesian_heatmap,
        plot_combined_metrics,
    )
except ImportError as e:
    print(f"Error importing modules: {e}")
    print("Make sure matplotlib and numpy are installed:")
    print("  pip install matplotlib numpy")
    sys.exit(1)


def generate_mock_heatmap(rings=40, azimuth_buckets=120, settled_threshold=5):
    """Generate realistic mock heatmap data"""

    buckets = []
    cells_per_bucket = 15  # 1800 azimuth bins / 120 buckets
    azimuth_bucket_deg = 360.0 / azimuth_buckets

    total_filled = 0
    total_settled = 0
    total_frozen = 0

    np.random.seed(42)  # For reproducibility

    for ring in range(rings):
        # Base range for this ring (farther rings = longer range)
        base_range = 5.0 + ring * 2.5

        for az_bucket in range(azimuth_buckets):
            # Simulate some spatial patterns
            # Create a "gap" in one quadrant (simulating occlusion)
            azimuth_deg = az_bucket * azimuth_bucket_deg
            in_gap = azimuth_deg >= 45 and azimuth_deg < 90 and ring < 20

            # Create a "partially settled" region
            in_partial = azimuth_deg >= 180 and azimuth_deg < 270

            if in_gap:
                # Gap region - very few cells
                filled_cells = np.random.randint(0, 3)
                settled_cells = 0
            elif in_partial:
                # Partially settled region
                filled_cells = np.random.randint(10, cells_per_bucket + 1)
                settled_cells = np.random.randint(0, filled_cells // 2)
            else:
                # Normal region - mostly filled and settled
                filled_cells = np.random.randint(
                    cells_per_bucket - 3, cells_per_bucket + 1
                )
                settled_cells = np.random.randint(filled_cells - 2, filled_cells + 1)

            # Calculate times seen (settled cells should have higher counts)
            if filled_cells > 0:
                mean_times_seen = (
                    np.random.uniform(2, 4)
                    if settled_cells < filled_cells // 2
                    else np.random.uniform(7, 15)
                )
            else:
                mean_times_seen = 0

            # Add some frozen cells occasionally
            frozen_cells = (
                np.random.randint(0, 3)
                if np.random.random() < 0.1 and filled_cells > 0
                else 0
            )

            # Range with some variation
            mean_range = base_range + np.random.uniform(-0.5, 0.5)
            min_range = (
                mean_range - np.random.uniform(0.1, 0.3) if filled_cells > 0 else 0
            )
            max_range = (
                mean_range + np.random.uniform(0.1, 0.3) if filled_cells > 0 else 0
            )

            bucket = {
                "ring": ring,
                "azimuth_deg_start": azimuth_deg,
                "azimuth_deg_end": azimuth_deg + azimuth_bucket_deg,
                "total_cells": cells_per_bucket,
                "filled_cells": filled_cells,
                "settled_cells": settled_cells,
                "frozen_cells": frozen_cells,
                "mean_times_seen": mean_times_seen,
                "mean_range_meters": mean_range,
                "min_range_meters": min_range,
                "max_range_meters": max_range,
            }

            buckets.append(bucket)
            total_filled += filled_cells
            total_settled += settled_cells
            total_frozen += frozen_cells

    total_cells = rings * azimuth_buckets * cells_per_bucket

    heatmap = {
        "sensor_id": "mock-hesai-pandar40p",
        "timestamp": "2025-10-31T18:00:00Z",
        "grid_params": {
            "total_rings": rings,
            "total_azimuth_bins": 1800,
            "azimuth_bin_resolution_deg": 0.2,
            "total_cells": total_cells,
        },
        "heatmap_params": {
            "azimuth_bucket_deg": azimuth_bucket_deg,
            "azimuth_buckets": azimuth_buckets,
            "ring_buckets": rings,
            "settled_threshold": settled_threshold,
            "cells_per_bucket": cells_per_bucket,
        },
        "summary": {
            "total_filled": total_filled,
            "total_settled": total_settled,
            "total_frozen": total_frozen,
            "fill_rate": total_filled / total_cells,
            "settle_rate": total_settled / total_cells,
        },
        "buckets": buckets,
    }

    return heatmap


def main():
    print("Generating mock grid heatmap data...")
    heatmap = generate_mock_heatmap()

    print("Generated heatmap with:")
    print(f"  {heatmap['grid_params']['total_cells']:,} total cells")
    print(
        f"  {heatmap['summary']['total_filled']:,} filled cells ({heatmap['summary']['fill_rate']:.1%})"
    )
    print(
        f"  {heatmap['summary']['total_settled']:,} settled cells ({heatmap['summary']['settle_rate']:.1%})"
    )
    print(f"  {heatmap['summary']['total_frozen']:,} frozen cells")
    print()

    # Save mock data for reference
    with open("mock_grid_heatmap.json", "w") as f:
        json.dump(heatmap, f, indent=2)
    print("Saved mock data to: mock_grid_heatmap.json")
    print()

    # Generate all visualization types
    print("Creating visualizations...")

    print("  1. Polar heatmap (unsettled ratio)...")
    plot_polar_heatmap(
        heatmap, metric="unsettled_ratio", output="test_grid_polar_unsettled.png"
    )

    print("  2. Polar heatmap (fill rate)...")
    plot_polar_heatmap(heatmap, metric="fill_rate", output="test_grid_polar_fill.png")

    print("  3. Polar heatmap (mean times seen)...")
    plot_polar_heatmap(
        heatmap, metric="mean_times_seen", output="test_grid_polar_times.png"
    )

    print("  4. Cartesian heatmap (unsettled ratio)...")
    plot_cartesian_heatmap(
        heatmap, metric="unsettled_ratio", output="test_grid_xy_unsettled.png"
    )

    print("  5. Cartesian heatmap (fill rate)...")
    plot_cartesian_heatmap(heatmap, metric="fill_rate", output="test_grid_xy_fill.png")

    print("  6. Combined metrics view...")
    plot_combined_metrics(heatmap, output="test_grid_combined.png")

    print()
    print("✓ Test complete! Generated 6 visualization examples:")
    print("  - test_grid_polar_unsettled.png")
    print("  - test_grid_polar_fill.png")
    print("  - test_grid_polar_times.png")
    print("  - test_grid_xy_unsettled.png")
    print("  - test_grid_xy_fill.png")
    print("  - test_grid_combined.png")


if __name__ == "__main__":
    main()
