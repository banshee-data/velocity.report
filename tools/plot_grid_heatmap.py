#!/usr/bin/env python3
"""
plot_grid_heatmap.py

Plot grid heatmap visualization from /api/lidar/grid_heatmap endpoint

Creates visualizations showing spatial patterns of filled and settled cells in the LiDAR
background grid. Supports both polar (ring vs azimuth) and cartesian (X-Y) projections.

Usage:
    python3 tools/plot_grid_heatmap.py --url http://localhost:8081 --sensor hesai-pandar40p
    python3 tools/plot_grid_heatmap.py --url http://localhost:8081 --polar --cartesian
    python3 tools/plot_grid_heatmap.py --url http://localhost:8081 --metric unsettled_ratio
"""

import argparse
import sys

try:
    import matplotlib.pyplot as plt
    import numpy as np
    import requests
except Exception as e:
    print(
        "Missing Python dependencies for plotting:\n  pip install matplotlib numpy requests"
    )
    print("Error details:", e)
    raise


def fetch_heatmap(base_url, sensor_id, azimuth_bucket_deg=3, settled_threshold=5):
    """Fetch grid heatmap data from the API endpoint"""
    url = f"{base_url}/api/lidar/grid_heatmap"
    params = {
        "sensor_id": sensor_id,
        "azimuth_bucket_deg": azimuth_bucket_deg,
        "settled_threshold": settled_threshold,
    }

    try:
        resp = requests.get(url, params=params, timeout=30)
        resp.raise_for_status()
        return resp.json()
    except requests.exceptions.RequestException as e:
        print(f"Error fetching heatmap data: {e}")
        sys.exit(1)


def plot_polar_heatmap(
    heatmap, metric="fill_rate", output="grid_heatmap_polar.png", dpi=150
):
    """
    Plot polar heatmap showing ring vs azimuth

    Args:
        heatmap: Heatmap data from API
        metric: Which metric to visualize ('fill_rate', 'settle_rate', 'unsettled_ratio', 'mean_times_seen')
        output: Output filename
        dpi: Image resolution
    """
    buckets = heatmap["buckets"]
    params = heatmap["heatmap_params"]

    rings = params["ring_buckets"]
    az_buckets = params["azimuth_buckets"]

    # Create 2D array for heatmap
    data = np.zeros((rings, az_buckets))

    for bucket in buckets:
        ring = bucket["ring"]
        az_idx = int(bucket["azimuth_deg_start"] / params["azimuth_bucket_deg"])

        if metric == "fill_rate":
            data[ring, az_idx] = bucket["filled_cells"] / bucket["total_cells"]
        elif metric == "settle_rate":
            if bucket["filled_cells"] > 0:
                data[ring, az_idx] = bucket["settled_cells"] / bucket["filled_cells"]
        elif metric == "unsettled_ratio":
            if bucket["filled_cells"] > 0:
                unsettled = bucket["filled_cells"] - bucket["settled_cells"]
                data[ring, az_idx] = unsettled / bucket["filled_cells"]
        elif metric == "mean_times_seen":
            data[ring, az_idx] = bucket["mean_times_seen"]
        elif metric == "frozen_ratio":
            if bucket["total_cells"] > 0:
                data[ring, az_idx] = bucket["frozen_cells"] / bucket["total_cells"]

    fig, ax = plt.subplots(figsize=(16, 8))

    # Choose colormap based on metric
    if metric in ["fill_rate", "settle_rate"]:
        cmap = "YlGn"  # Yellow to Green (higher is better)
        vmin, vmax = 0, 1
    elif metric == "unsettled_ratio":
        cmap = "RdYlGn_r"  # Red (high unsettled) to Green (low unsettled)
        vmin, vmax = 0, 1
    elif metric == "frozen_ratio":
        cmap = "Blues"
        vmin, vmax = 0, 1
    else:  # mean_times_seen
        cmap = "viridis"
        vmin, vmax = 0, np.max(data) if np.max(data) > 0 else 1

    im = ax.imshow(
        data,
        aspect="auto",
        cmap=cmap,
        origin="lower",
        extent=[0, 360, 0, rings],
        vmin=vmin,
        vmax=vmax,
        interpolation="nearest",
    )

    ax.set_xlabel("Azimuth (degrees)", fontsize=13)
    ax.set_ylabel("Ring Index", fontsize=13)

    # Format title
    metric_title = metric.replace("_", " ").title()
    ax.set_title(
        f"Grid Heatmap: {metric_title}\n{heatmap['sensor_id']} at {heatmap['timestamp'][:19]}",
        fontsize=14,
        fontweight="bold",
    )

    # Add colorbar
    cbar = plt.colorbar(im, ax=ax, fraction=0.046, pad=0.04)
    cbar.set_label(metric_title, fontsize=12)

    # Add grid lines every 30 degrees and every 5 rings
    ax.set_xticks(np.arange(0, 361, 30))
    ax.set_yticks(np.arange(0, rings + 1, 5))
    ax.grid(True, alpha=0.3, linewidth=0.5)

    # Add summary text
    summary = heatmap["summary"]
    summary_text = (
        f"Filled: {summary['total_filled']:,} ({summary['fill_rate']:.1%})\n"
        f"Settled: {summary['total_settled']:,} ({summary['settle_rate']:.1%})\n"
        f"Frozen: {summary['total_frozen']:,}"
    )
    ax.text(
        0.02,
        0.98,
        summary_text,
        transform=ax.transAxes,
        fontsize=10,
        verticalalignment="top",
        bbox=dict(boxstyle="round", facecolor="white", alpha=0.9, edgecolor="gray"),
    )

    plt.tight_layout()
    plt.savefig(output, dpi=dpi, bbox_inches="tight")
    print(f"Saved polar heatmap: {output}")
    plt.close()


def plot_cartesian_heatmap(
    heatmap, metric="unsettled_ratio", output="grid_heatmap_xy.png", dpi=150
):
    """
    Plot X-Y heatmap in cartesian coordinates

    Shows physical spatial distribution of cells in meters.

    Args:
        heatmap: Heatmap data from API
        metric: Which metric to visualize
        output: Output filename
        dpi: Image resolution
    """
    buckets = heatmap["buckets"]

    # Convert polar to cartesian
    x_coords = []
    y_coords = []
    values = []
    sizes = []

    for bucket in buckets:
        if bucket["filled_cells"] == 0:
            continue

        # Use mean azimuth and mean range
        az_mid = (bucket["azimuth_deg_start"] + bucket["azimuth_deg_end"]) / 2
        az_rad = np.radians(az_mid)
        r = bucket["mean_range_meters"]

        x = r * np.cos(az_rad)
        y = r * np.sin(az_rad)

        # Calculate metric value
        if metric == "unsettled_ratio":
            if bucket["filled_cells"] > 0:
                unsettled = bucket["filled_cells"] - bucket["settled_cells"]
                val = unsettled / bucket["filled_cells"]
            else:
                val = 0
        elif metric == "fill_rate":
            val = bucket["filled_cells"] / bucket["total_cells"]
        elif metric == "settle_rate":
            if bucket["filled_cells"] > 0:
                val = bucket["settled_cells"] / bucket["filled_cells"]
            else:
                val = 0
        elif metric == "mean_times_seen":
            val = bucket["mean_times_seen"]
        elif metric == "frozen_ratio":
            if bucket["total_cells"] > 0:
                val = bucket["frozen_cells"] / bucket["total_cells"]
            else:
                val = 0
        else:
            val = 0

        x_coords.append(x)
        y_coords.append(y)
        values.append(val)

        # Size based on how many cells are filled
        sizes.append(bucket["filled_cells"] * 5)

    fig, ax = plt.subplots(figsize=(12, 10))

    # Choose colormap
    if metric in ["fill_rate", "settle_rate"]:
        cmap = "YlGn"
        vmin, vmax = 0, 1
    elif metric == "unsettled_ratio":
        cmap = "RdYlGn_r"
        vmin, vmax = 0, 1
    elif metric == "frozen_ratio":
        cmap = "Blues"
        vmin, vmax = 0, 1
    else:  # mean_times_seen
        cmap = "viridis"
        vmin, vmax = 0, max(values) if values else 1

    scatter = ax.scatter(
        x_coords,
        y_coords,
        c=values,
        s=sizes,
        cmap=cmap,
        alpha=0.7,
        edgecolors="k",
        linewidth=0.5,
        vmin=vmin,
        vmax=vmax,
    )

    ax.set_xlabel("X (meters)", fontsize=13)
    ax.set_ylabel("Y (meters)", fontsize=13)

    metric_title = metric.replace("_", " ").title()
    ax.set_title(
        f"Spatial Heatmap: {metric_title}\n{heatmap['sensor_id']}",
        fontsize=14,
        fontweight="bold",
    )
    ax.set_aspect("equal")
    ax.grid(True, alpha=0.3)

    # Add colorbar
    cbar = plt.colorbar(scatter, ax=ax, fraction=0.046, pad=0.04)
    cbar.set_label(metric_title, fontsize=12)

    # Add origin marker
    ax.plot(0, 0, "r*", markersize=15, label="Sensor", zorder=5)
    ax.legend(loc="upper right")

    # Add summary
    summary = heatmap["summary"]
    summary_text = (
        f"Filled: {summary['total_filled']:,} ({summary['fill_rate']:.1%})\n"
        f"Settled: {summary['total_settled']:,} ({summary['settle_rate']:.1%})\n"
        f"Frozen: {summary['total_frozen']:,}"
    )
    ax.text(
        0.02,
        0.98,
        summary_text,
        transform=ax.transAxes,
        fontsize=10,
        verticalalignment="top",
        bbox=dict(boxstyle="round", facecolor="white", alpha=0.9, edgecolor="gray"),
    )

    plt.tight_layout()
    plt.savefig(output, dpi=dpi, bbox_inches="tight")
    print(f"Saved cartesian heatmap: {output}")
    plt.close()


def plot_combined_metrics(heatmap, output="grid_heatmap_combined.png", dpi=150):
    """
    Create a combined view showing multiple metrics side by side

    Args:
        heatmap: Heatmap data from API
        output: Output filename
        dpi: Image resolution
    """
    buckets = heatmap["buckets"]
    params = heatmap["heatmap_params"]

    rings = params["ring_buckets"]
    az_buckets = params["azimuth_buckets"]

    # Prepare data for each metric
    metrics = ["fill_rate", "settle_rate", "unsettled_ratio", "mean_times_seen"]
    data_arrays = []

    for metric in metrics:
        data = np.zeros((rings, az_buckets))

        for bucket in buckets:
            ring = bucket["ring"]
            az_idx = int(bucket["azimuth_deg_start"] / params["azimuth_bucket_deg"])

            if metric == "fill_rate":
                data[ring, az_idx] = bucket["filled_cells"] / bucket["total_cells"]
            elif metric == "settle_rate":
                if bucket["filled_cells"] > 0:
                    data[ring, az_idx] = (
                        bucket["settled_cells"] / bucket["filled_cells"]
                    )
            elif metric == "unsettled_ratio":
                if bucket["filled_cells"] > 0:
                    unsettled = bucket["filled_cells"] - bucket["settled_cells"]
                    data[ring, az_idx] = unsettled / bucket["filled_cells"]
            elif metric == "mean_times_seen":
                data[ring, az_idx] = bucket["mean_times_seen"]

        data_arrays.append(data)

    # Create subplots
    fig, axes = plt.subplots(2, 2, figsize=(18, 10))
    axes = axes.flatten()

    for idx, (metric, data, ax) in enumerate(zip(metrics, data_arrays, axes)):
        # Choose colormap
        if metric in ["fill_rate", "settle_rate"]:
            cmap = "YlGn"
            vmin, vmax = 0, 1
        elif metric == "unsettled_ratio":
            cmap = "RdYlGn_r"
            vmin, vmax = 0, 1
        else:  # mean_times_seen
            cmap = "viridis"
            vmin, vmax = 0, np.max(data) if np.max(data) > 0 else 1

        im = ax.imshow(
            data,
            aspect="auto",
            cmap=cmap,
            origin="lower",
            extent=[0, 360, 0, rings],
            vmin=vmin,
            vmax=vmax,
            interpolation="nearest",
        )

        ax.set_xlabel("Azimuth (degrees)", fontsize=11)
        ax.set_ylabel("Ring Index", fontsize=11)
        ax.set_title(metric.replace("_", " ").title(), fontsize=12, fontweight="bold")

        ax.set_xticks(np.arange(0, 361, 60))
        ax.set_yticks(np.arange(0, rings + 1, 10))
        ax.grid(True, alpha=0.3, linewidth=0.5)

        cbar = plt.colorbar(im, ax=ax, fraction=0.046, pad=0.04)
        cbar.ax.tick_params(labelsize=9)

    fig.suptitle(
        f"Grid Analysis: {heatmap['sensor_id']} at {heatmap['timestamp'][:19]}",
        fontsize=15,
        fontweight="bold",
    )

    plt.tight_layout(rect=[0, 0, 1, 0.97])
    plt.savefig(output, dpi=dpi, bbox_inches="tight")
    print(f"Saved combined metrics plot: {output}")
    plt.close()


def main():
    parser = argparse.ArgumentParser(
        description="Plot grid heatmap from LiDAR monitor API"
    )
    parser.add_argument(
        "--url", default="http://localhost:8081", help="Monitor base URL"
    )
    parser.add_argument("--sensor", default="hesai-pandar40p", help="Sensor ID")
    parser.add_argument(
        "--azimuth-bucket",
        type=float,
        default=3.0,
        help="Azimuth bucket size in degrees",
    )
    parser.add_argument(
        "--settled-threshold",
        type=int,
        default=5,
        help="Min times seen for settled",
    )
    parser.add_argument(
        "--metric",
        default="unsettled_ratio",
        choices=[
            "fill_rate",
            "settle_rate",
            "unsettled_ratio",
            "mean_times_seen",
            "frozen_ratio",
        ],
        help="Metric to visualize",
    )
    parser.add_argument(
        "--polar", action="store_true", help="Create polar heatmap (ring vs azimuth)"
    )
    parser.add_argument(
        "--cartesian", action="store_true", help="Create cartesian heatmap (X-Y)"
    )
    parser.add_argument(
        "--combined", action="store_true", help="Create combined multi-metric view"
    )
    parser.add_argument("--output", default="grid_heatmap.png", help="Output filename")
    parser.add_argument("--dpi", type=int, default=150, help="Output image DPI")

    args = parser.parse_args()

    # Default to polar if nothing specified
    if not args.polar and not args.cartesian and not args.combined:
        args.polar = True

    print(f"Fetching heatmap data from {args.url}...")
    heatmap = fetch_heatmap(
        args.url, args.sensor, args.azimuth_bucket, args.settled_threshold
    )

    print(
        f"Grid: {heatmap['grid_params']['total_rings']} rings × "
        f"{heatmap['grid_params']['total_azimuth_bins']} azimuth bins"
    )
    print(
        f"Aggregation: {heatmap['heatmap_params']['ring_buckets']} rings × "
        f"{heatmap['heatmap_params']['azimuth_buckets']} azimuth buckets"
    )
    print(
        f"Summary: {heatmap['summary']['total_filled']:,} filled, "
        f"{heatmap['summary']['total_settled']:,} settled"
    )
    print()

    if args.polar:
        plot_polar_heatmap(heatmap, args.metric, args.output, args.dpi)

    if args.cartesian:
        xy_output = args.output.replace(".png", "_xy.png")
        plot_cartesian_heatmap(heatmap, args.metric, xy_output, args.dpi)

    if args.combined:
        combined_output = args.output.replace(".png", "_combined.png")
        plot_combined_metrics(heatmap, combined_output, args.dpi)

    print("\n✓ Plotting completed successfully")


if __name__ == "__main__":
    main()
