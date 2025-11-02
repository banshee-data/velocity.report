#!/usr/bin/env python3
"""
plot_grid_heatmap.py

Plot grid heatmap visualization from /api/lidar/grid_heatmap endpoint

Creates visualizations showing spatial patterns of filled and settled cells in the LiDAR
background grid. Supports both polar (ring vs azimuth) and cartesian (X-Y) projections.

Can process data in three modes:
1. Single snapshot - one-time capture
2. Live snapshots - periodic captures from running system
3. PCAP replay - periodic captures during PCAP file replay

Usage:
    # Single snapshot
    python3 tools/grid-heatmap/plot_grid_heatmap.py --url http://localhost:8081 --sensor hesai-pandar40p

    # Live periodic snapshots (from running system)
    python3 tools/grid-heatmap/plot_grid_heatmap.py --url http://localhost:8081 --interval 10 --duration 120

    # PCAP replay with periodic snapshots
    python3 tools/grid-heatmap/plot_grid_heatmap.py --url http://localhost:8081 --pcap file.pcap --interval 30

    # Custom metrics and views
    python3 tools/grid-heatmap/plot_grid_heatmap.py --url http://localhost:8081 --polar --cartesian
    python3 tools/grid-heatmap/plot_grid_heatmap.py --url http://localhost:8081 --metric unsettled_ratio
"""

import argparse
import json
import sys
import time
from pathlib import Path

try:
    import matplotlib.pyplot as plt
    import numpy as np
    import requests
except Exception as e:
    import sys

    if __name__ == "__main__":
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


def plot_full_dashboard(heatmap, metric, output="grid_dashboard.png", dpi=150):
    """
    Create a comprehensive full-screen dashboard with all visualizations
    Optimized for 4K monitors (3840x2160, 16:9 aspect ratio)

    Layout:
    - Top: Polar settle rate + Polar (selected metric) + Spatial XY distance map (background)
    - Bottom: 4 metric panels stacked vertically (fill_rate, settle_rate, unsettled_ratio, mean_times_seen)

    Args:
        heatmap: Heatmap data from API
        metric: Primary metric to highlight in polar/cartesian views
        output: Output filename
        dpi: Image resolution
    """
    buckets = heatmap["buckets"]
    params = heatmap["heatmap_params"]
    summary = heatmap["summary"]

    rings = params["ring_buckets"]
    az_buckets = params["azimuth_buckets"]

    # Create figure optimized for 4K (3840x2160 at 150dpi = 25.6x14.4 inches)
    fig = plt.figure(figsize=(25.6, 14.4))
    # 6 rows: rows 0-2 for top charts (50%), rows 3-6 for bottom metric charts (50%)
    # Height ratios: top section gets 3 units, each bottom chart gets 0.75 units
    gs = fig.add_gridspec(
        7,
        12,
        height_ratios=[1, 1, 1, 0.75, 0.75, 0.75, 0.75],
        hspace=0.15,
        wspace=0.30,
        top=0.95,
        bottom=0.05,
        left=0.05,
        right=0.95,
    )

    # === TOP LEFT: Polar Settle Rate ===
    ax_settle = fig.add_subplot(gs[0:3, :4], projection="polar")

    # Build settle rate data
    data_settle = np.zeros((rings, az_buckets))
    for bucket in buckets:
        ring = bucket["ring"]
        az_idx = int(bucket["azimuth_deg_start"] / params["azimuth_bucket_deg"])

        if bucket["filled_cells"] > 0:
            data_settle[ring, az_idx] = bucket["settled_cells"] / bucket["filled_cells"]
        else:
            data_settle[ring, az_idx] = 0

    # Create polar mesh
    theta = np.linspace(0, 2 * np.pi, az_buckets + 1)
    r = np.arange(rings + 1)
    theta_grid, r_grid = np.meshgrid(theta, r)

    im_settle = ax_settle.pcolormesh(
        theta_grid,
        r_grid,
        data_settle,
        cmap="YlOrRd",
        vmin=0,
        vmax=1,
        shading="auto",
    )
    ax_settle.set_theta_zero_location("N")
    ax_settle.set_theta_direction(-1)
    ax_settle.set_ylim(0, rings)
    ax_settle.set_title(
        "Polar View: Settle Rate", fontsize=16, fontweight="bold", pad=20
    )
    ax_settle.grid(True, alpha=0.3)
    cbar_settle = plt.colorbar(im_settle, ax=ax_settle, fraction=0.046, pad=0.08)
    cbar_settle.set_label("Settle Rate", fontsize=12)

    # === TOP MIDDLE: Polar Heatmap (Selected Metric) ===
    ax_polar = fig.add_subplot(gs[0:3, 4:8], projection="polar")

    metric_map = {
        "fill_rate": ("Fill Rate", "YlGn", (0, 1)),
        "settle_rate": ("Settle Rate", "YlOrRd", (0, 1)),
        "unsettled_ratio": ("Unsettled Ratio", "RdYlGn_r", (0, 1)),
        "mean_times_seen": ("Mean Times Seen", "viridis", (0, None)),
        "frozen_ratio": ("Frozen Ratio", "Blues", (0, 1)),
    }
    metric_title, cmap, (vmin_base, vmax_base) = metric_map.get(
        metric, ("Unknown", "viridis", (0, 1))
    )

    data_polar = np.zeros((rings, az_buckets))
    for bucket in buckets:
        ring = bucket["ring"]
        az_idx = int(bucket["azimuth_deg_start"] / params["azimuth_bucket_deg"])

        if metric == "fill_rate":
            data_polar[ring, az_idx] = bucket["filled_cells"] / bucket["total_cells"]
        elif metric == "settle_rate":
            if bucket["filled_cells"] > 0:
                data_polar[ring, az_idx] = (
                    bucket["settled_cells"] / bucket["filled_cells"]
                )
        elif metric == "unsettled_ratio":
            if bucket["filled_cells"] > 0:
                unsettled = bucket["filled_cells"] - bucket["settled_cells"]
                data_polar[ring, az_idx] = unsettled / bucket["filled_cells"]
        elif metric == "mean_times_seen":
            data_polar[ring, az_idx] = bucket["mean_times_seen"]
        elif metric == "frozen_ratio":
            if bucket["filled_cells"] > 0:
                data_polar[ring, az_idx] = (
                    bucket["frozen_cells"] / bucket["filled_cells"]
                )

    vmax = vmax_base if vmax_base is not None else np.max(data_polar)
    theta = np.linspace(0, 2 * np.pi, az_buckets + 1)
    r = np.arange(rings + 1)
    theta_grid, r_grid = np.meshgrid(theta, r)

    im_polar = ax_polar.pcolormesh(
        theta_grid,
        r_grid,
        data_polar,
        cmap=cmap,
        vmin=vmin_base,
        vmax=vmax,
        shading="auto",
    )
    ax_polar.set_theta_zero_location("N")
    ax_polar.set_theta_direction(-1)
    ax_polar.set_ylim(0, rings)
    ax_polar.set_title(
        f"Polar View: {metric_title}", fontsize=16, fontweight="bold", pad=20
    )
    ax_polar.grid(True, alpha=0.3)
    cbar_polar = plt.colorbar(im_polar, ax=ax_polar, fraction=0.046, pad=0.08)
    cbar_polar.set_label(metric_title, fontsize=12)

    # === TOP RIGHT: Spatial XY Distance Heatmap ===
    ax_spatial = fig.add_subplot(gs[0:3, 8:])

    # Build spatial grid - plot actual distance (range) from sensor in XY space
    x_coords_all, y_coords_all, range_values = [], [], []
    for bucket in buckets:
        # Only plot buckets with actual range data
        if bucket["filled_cells"] == 0 or bucket["mean_range_meters"] == 0:
            continue

        az_start = np.radians(bucket["azimuth_deg_start"])
        az_end = np.radians(bucket["azimuth_deg_end"])
        az_mid = (az_start + az_end) / 2

        # Use actual measured range from background snapshot
        range_m = bucket["mean_range_meters"]

        # Convert to XY coordinates (sensor at origin)
        x = range_m * np.sin(az_mid)
        y = range_m * np.cos(az_mid)

        x_coords_all.append(x)
        y_coords_all.append(y)
        range_values.append(range_m)

    if len(x_coords_all) > 0:
        # Create 2D histogram/heatmap
        x_bins = np.linspace(min(x_coords_all), max(x_coords_all), 80)
        y_bins = np.linspace(min(y_coords_all), max(y_coords_all), 80)

        # Create grid for heatmap
        H, xedges, yedges = np.histogram2d(
            x_coords_all, y_coords_all, bins=[x_bins, y_bins], weights=range_values
        )
        counts, _, _ = np.histogram2d(x_coords_all, y_coords_all, bins=[x_bins, y_bins])

        # Average values in each bin
        with np.errstate(divide="ignore", invalid="ignore"):
            H_avg = np.divide(H, counts)
            H_avg[~np.isfinite(H_avg)] = 0

        # Use viridis colormap for distance (meters)
        im_spatial = ax_spatial.imshow(
            H_avg.T,
            origin="lower",
            extent=[xedges[0], xedges[-1], yedges[0], yedges[-1]],
            cmap="viridis",
            aspect="equal",
            interpolation="bilinear",
        )

        ax_spatial.set_xlabel("X (meters)", fontsize=12)
        ax_spatial.set_ylabel("Y (meters)", fontsize=12)
        ax_spatial.set_title(
            "Spatial XY: Background Distance", fontsize=16, fontweight="bold"
        )
        ax_spatial.grid(True, alpha=0.3)
        ax_spatial.plot(0, 0, "r*", markersize=15, label="Sensor", zorder=5)
        ax_spatial.legend(loc="upper right")
        cbar_spatial = plt.colorbar(im_spatial, ax=ax_spatial, fraction=0.046, pad=0.04)
        cbar_spatial.set_label("Distance (m)", fontsize=12)
    else:
        # No data to plot
        ax_spatial.text(
            0.5,
            0.5,
            "No background data available",
            ha="center",
            va="center",
            transform=ax_spatial.transAxes,
            fontsize=14,
        )
        ax_spatial.set_xlabel("X (meters)", fontsize=12)
        ax_spatial.set_ylabel("Y (meters)", fontsize=12)
        ax_spatial.set_title(
            "Spatial XY: Background Distance", fontsize=16, fontweight="bold"
        )

    # === BOTTOM: 4 Metric Panels (Stacked Vertically) ===
    metric_configs = [
        ("fill_rate", "Fill Rate", "YlGn", (0, 1)),
        ("settle_rate", "Settle Rate", "YlOrRd", (0, 1)),
        ("unsettled_ratio", "Unsettled Ratio", "RdYlGn_r", (0, 1)),
        ("mean_times_seen", "Mean Times Seen", "viridis", (0, None)),
    ]

    # Stack panels vertically, each spanning full width (all 12 columns)
    for idx, (met, title, cm, (vmin, vmax)) in enumerate(metric_configs):
        row = idx + 3  # Rows 3-6 for the 4 metrics
        ax = fig.add_subplot(gs[row, :])  # Span all 12 columns

        data = np.zeros((rings, az_buckets))
        for bucket in buckets:
            ring = bucket["ring"]
            az_idx = int(bucket["azimuth_deg_start"] / params["azimuth_bucket_deg"])

            if met == "fill_rate":
                data[ring, az_idx] = bucket["filled_cells"] / bucket["total_cells"]
            elif met == "settle_rate":
                if bucket["filled_cells"] > 0:
                    data[ring, az_idx] = (
                        bucket["settled_cells"] / bucket["filled_cells"]
                    )
            elif met == "unsettled_ratio":
                if bucket["filled_cells"] > 0:
                    unsettled = bucket["filled_cells"] - bucket["settled_cells"]
                    data[ring, az_idx] = unsettled / bucket["filled_cells"]
            elif met == "mean_times_seen":
                data[ring, az_idx] = bucket["mean_times_seen"]

        vmax_plot = (
            vmax if vmax is not None else (np.max(data) if np.max(data) > 0 else 1)
        )

        im = ax.imshow(
            data,
            aspect="auto",
            cmap=cm,
            origin="lower",
            extent=[0, 360, 0, rings],
            vmin=vmin,
            vmax=vmax_plot,
            interpolation="nearest",
        )

        # Only show x-axis labels on the bottom-most chart (last one)
        if idx == len(metric_configs) - 1:
            ax.set_xlabel("Azimuth (°)", fontsize=10)
            ax.set_xticks(np.arange(0, 361, 90))
        else:
            ax.set_xticks([])
            ax.tick_params(axis="x", which="both", bottom=False, labelbottom=False)

        ax.set_ylabel("Ring", fontsize=10)
        # Title aligned to the left
        ax.set_title(title, fontsize=12, fontweight="bold", loc="left")
        ax.set_yticks(np.arange(0, rings + 1, 10))
        ax.grid(True, alpha=0.3, linewidth=0.5)

        cbar = plt.colorbar(im, ax=ax, fraction=0.015, pad=0.01)
        cbar.ax.tick_params(labelsize=8)

    # === Title and Summary ===
    # Place title in top right corner on single line to make it wider
    title_text = f"{heatmap['sensor_id']} - Grid Analysis Dashboard - {heatmap['timestamp'][:19]}"
    fig.text(
        0.963,
        0.995,
        title_text,
        ha="right",
        va="top",
        fontsize=14,
        fontweight="bold",
        bbox=dict(
            boxstyle="round,pad=0.5",
            facecolor="white",
            alpha=0.9,
            edgecolor="gray",
        ),
    )

    summary_text = (
        f"Filled: {summary['total_filled']:,}/{summary['total_filled'] + (72000 - summary['total_filled']):,} "
        f"({summary['fill_rate']:.1%})  |  "
        f"Settled: {summary['total_settled']:,} ({summary['settle_rate']:.1%})  |  "
        f"Frozen: {summary['total_frozen']:,}"
    )
    fig.text(
        0.5,
        0.02,
        summary_text,
        ha="center",
        fontsize=12,
        bbox=dict(boxstyle="round", facecolor="white", alpha=0.9, edgecolor="gray"),
    )

    plt.savefig(output, dpi=dpi, bbox_inches="tight")
    print(f"Saved full dashboard: {output}")
    plt.close()


def start_pcap_replay(base_url, sensor_id, pcap_file):
    """
    Start PCAP replay via the monitor API

    Args:
        base_url: Monitor base URL
        sensor_id: Sensor ID
        pcap_file: Path to PCAP file

    Returns:
        True if successful, False otherwise
    """
    url = f"{base_url}/api/lidar/pcap/start"
    params = {"sensor_id": sensor_id}
    body = {"pcap_file": pcap_file}

    try:
        resp = requests.post(url, params=params, json=body, timeout=10)
        resp.raise_for_status()
        result = resp.json()
        print(f"PCAP replay started: {result}")
        return True
    except requests.exceptions.RequestException as e:
        print(f"Error starting PCAP replay: {e}")
        return False


def reset_grid(base_url, sensor_id):
    """Reset the grid to empty state"""
    url = f"{base_url}/api/lidar/grid_reset"
    params = {"sensor_id": sensor_id}

    try:
        resp = requests.post(url, params=params, timeout=10)
        resp.raise_for_status()
        print("Grid reset successful")
        return True
    except requests.exceptions.RequestException as e:
        print(f"Error resetting grid: {e}")
        return False


def wait_for_grid_population(base_url, sensor_id, min_filled=1000, timeout=60):
    """
    Wait for grid to start populating

    Args:
        base_url: Monitor base URL
        sensor_id: Sensor ID
        min_filled: Minimum filled cells to consider grid populated
        timeout: Maximum time to wait in seconds
    """
    url = f"{base_url}/api/lidar/grid_heatmap"
    params = {"sensor_id": sensor_id}

    start_time = time.time()
    while time.time() - start_time < timeout:
        try:
            resp = requests.get(url, params=params, timeout=10)
            resp.raise_for_status()
            data = resp.json()
            filled = data["summary"]["total_filled"]

            if filled >= min_filled:
                print(f"Grid populated: {filled:,} filled cells")
                return True

            time.sleep(1)
        except requests.exceptions.RequestException:
            time.sleep(1)

    print("Timeout waiting for grid population")
    return False


def process_pcap_with_snapshots(
    base_url,
    sensor_id,
    pcap_file,
    interval,
    duration,
    output_dir,
    azimuth_bucket,
    settled_threshold,
    metric,
    dpi,
):
    """
    Process PCAP file and generate heatmap snapshots at regular intervals

    Args:
        base_url: Monitor base URL
        sensor_id: Sensor ID
        pcap_file: Path to PCAP file
        interval: Seconds between snapshots
        duration: Total duration to capture (None = until PCAP ends)
        output_dir: Directory for output files
        azimuth_bucket: Azimuth bucket size
        settled_threshold: Settled threshold
        metric: Metric to visualize
        dpi: Image DPI
    """
    # Create output directory
    output_path = Path(output_dir)
    output_path.mkdir(parents=True, exist_ok=True)

    # Save metadata
    metadata = {
        "pcap_file": str(pcap_file),
        "sensor_id": sensor_id,
        "interval": interval,
        "duration": duration,
        "metric": metric,
        "azimuth_bucket_deg": azimuth_bucket,
        "settled_threshold": settled_threshold,
        "snapshots": [],
    }

    print(f"Starting PCAP replay: {pcap_file}")
    print(f"Snapshot interval: {interval}s")
    print(f"Output directory: {output_dir}")
    print()

    # Reset grid to start clean
    if not reset_grid(base_url, sensor_id):
        print("Failed to reset grid, continuing anyway...")

    time.sleep(2)

    # Start PCAP replay
    if not start_pcap_replay(base_url, sensor_id, pcap_file):
        print("Failed to start PCAP replay")
        return

    # Wait for grid to start populating
    print("Waiting for grid to populate...")
    if not wait_for_grid_population(base_url, sensor_id, min_filled=100, timeout=30):
        print("Grid not populating, check PCAP replay status")
        return

    print()
    print("Starting snapshot capture...")

    snapshot_count = 0
    start_time = time.time()
    next_snapshot_time = start_time

    while True:
        current_time = time.time()
        elapsed = current_time - start_time

        # Check if we've exceeded duration
        if duration and elapsed >= duration:
            print(f"\nReached duration limit ({duration}s), stopping")
            break

        # Check if it's time for next snapshot
        if current_time >= next_snapshot_time:
            snapshot_count += 1
            elapsed_str = f"{elapsed:.1f}s"

            print(f"\n[Snapshot {snapshot_count} at {elapsed_str}]")

            # Fetch heatmap
            try:
                heatmap = fetch_heatmap(
                    base_url, sensor_id, azimuth_bucket, settled_threshold
                )
            except SystemExit:
                print("Failed to fetch heatmap, retrying...")
                next_snapshot_time += interval
                continue

            summary = heatmap["summary"]
            print(
                f"  Filled: {summary['total_filled']:,} ({summary['fill_rate']:.1%}), "
                f"Settled: {summary['total_settled']:,} ({summary['settle_rate']:.1%})"
            )

            # Generate filename prefix
            prefix = f"snapshot_{snapshot_count:03d}_t{int(elapsed):04d}s"

            # Save snapshot metadata
            snapshot_meta = {
                "snapshot": snapshot_count,
                "elapsed_seconds": elapsed,
                "timestamp": heatmap["timestamp"],
                "summary": summary,
            }
            metadata["snapshots"].append(snapshot_meta)

            # Generate full dashboard (single comprehensive PNG)
            dashboard_output = output_path / f"{prefix}.png"
            plot_full_dashboard(heatmap, metric, str(dashboard_output), dpi)
            print(f"  Saved: {dashboard_output.name}")

            # Schedule next snapshot
            next_snapshot_time += interval

        # Check if grid is still changing (heuristic: check if PCAP is still replaying)
        # If total_filled hasn't changed in last few snapshots and we have enough data, stop
        if snapshot_count >= 3 and len(metadata["snapshots"]) >= 3:
            last_three_filled = [
                s["summary"]["total_filled"] for s in metadata["snapshots"][-3:]
            ]
            if (
                len(set(last_three_filled)) == 1 and elapsed > interval * 3
            ):  # All same value
                print(
                    "\nGrid appears stable (no changes in last 3 snapshots), stopping"
                )
                break

        # Sleep briefly to avoid busy loop
        time.sleep(0.5)

    # Save final metadata
    metadata["total_snapshots"] = snapshot_count
    metadata["total_duration"] = time.time() - start_time

    metadata_file = output_path / "metadata.json"
    with open(metadata_file, "w") as f:
        json.dump(metadata, f, indent=2)

    print(f"\n✓ Completed {snapshot_count} snapshots")
    print(f"  Total duration: {metadata['total_duration']:.1f}s")
    print(f"  Output directory: {output_dir}")
    print(f"  Metadata: {metadata_file}")


def process_live_snapshots(
    base_url,
    sensor_id,
    interval,
    duration,
    output_dir,
    azimuth_bucket,
    settled_threshold,
    metric,
    dpi,
):
    """
    Process live grid data and generate heatmap snapshots at regular intervals

    Args:
        base_url: Monitor base URL
        sensor_id: Sensor ID
        interval: Seconds between snapshots
        duration: Total duration to capture (None = infinite)
        output_dir: Directory for output files
        azimuth_bucket: Azimuth bucket size
        settled_threshold: Settled threshold
        metric: Metric to visualize
        dpi: Image DPI
    """
    # Create output directory
    output_path = Path(output_dir)
    output_path.mkdir(parents=True, exist_ok=True)

    print("Starting live snapshot capture")
    print(f"Snapshot interval: {interval}s")
    if duration:
        print(f"Total duration: {duration}s")
    else:
        print("Duration: infinite (Ctrl+C to stop)")
    print(f"Output directory: {output_dir}")
    print()

    # Metadata tracking
    metadata = {
        "mode": "live",
        "sensor_id": sensor_id,
        "interval_seconds": interval,
        "duration_seconds": duration,
        "metric": metric,
        "azimuth_bucket_deg": azimuth_bucket,
        "settled_threshold": settled_threshold,
        "snapshots": [],
    }

    snapshot_count = 0
    start_time = time.time()
    next_snapshot_time = start_time
    last_heatmap = None
    stable_count = 0

    print("Starting snapshot capture...")
    print()

    try:
        while True:
            current_time = time.time()
            elapsed = current_time - start_time

            # Check if we've reached the duration limit
            if duration and elapsed >= duration:
                print(f"Reached duration limit of {duration}s")
                break

            # Wait until next snapshot time
            if current_time < next_snapshot_time:
                time.sleep(0.5)
                continue

            snapshot_count += 1
            print(f"[Snapshot {snapshot_count} at {elapsed:.1f}s]")

            # Fetch heatmap
            try:
                heatmap = fetch_heatmap(
                    base_url, sensor_id, azimuth_bucket, settled_threshold
                )
            except Exception as e:
                print(f"Failed to fetch heatmap: {e}")
                print("Retrying in next interval...")
                next_snapshot_time += interval
                continue

            summary = heatmap["summary"]
            print(
                f"  Filled: {summary['total_filled']:,} ({summary['fill_rate']:.1%}), "
                f"Settled: {summary['total_settled']:,} ({summary['settle_rate']:.1%})"
            )

            # Generate filename prefix
            prefix = f"snapshot_{snapshot_count:03d}_t{int(elapsed):04d}s"

            # Save snapshot metadata
            snapshot_meta = {
                "snapshot": snapshot_count,
                "elapsed_seconds": elapsed,
                "timestamp": heatmap["timestamp"],
                "summary": summary,
            }
            metadata["snapshots"].append(snapshot_meta)

            # Generate full dashboard (single comprehensive PNG)
            dashboard_output = output_path / f"{prefix}.png"
            plot_full_dashboard(heatmap, metric, str(dashboard_output), dpi)
            print(f"  Saved: {dashboard_output.name}")

            print()

            # Check for grid stability (auto-stop after 3 stable snapshots)
            if last_heatmap is not None:
                if (
                    summary["total_filled"] == last_heatmap["summary"]["total_filled"]
                    and summary["total_settled"]
                    == last_heatmap["summary"]["total_settled"]
                ):
                    stable_count += 1
                    if stable_count >= 3:
                        print(
                            "Grid appears stable (no changes in last 3 snapshots), stopping"
                        )
                        print()
                        break
                else:
                    stable_count = 0

            last_heatmap = heatmap
            next_snapshot_time += interval

    except KeyboardInterrupt:
        print("\nInterrupted by user")
        print()

    # Save metadata
    metadata["total_duration"] = time.time() - start_time
    metadata["total_snapshots"] = snapshot_count

    metadata_file = output_path / "metadata.json"
    with open(metadata_file, "w") as f:
        json.dump(metadata, f, indent=2)

    print(f"\n✓ Completed {snapshot_count} snapshots")
    print(f"  Total duration: {metadata['total_duration']:.1f}s")
    print(f"  Output directory: {output_dir}")
    print(f"  Metadata: {metadata_file}")


def get_next_run_dir(base_dir):
    """
    Find the next available run number in base_dir.
    Returns path like base_dir/1, base_dir/2, etc.

    Args:
        base_dir: Base directory path (e.g., output/grid-heatmap-filename)

    Returns:
        Path to next run directory
    """
    base_path = Path(base_dir)
    base_path.mkdir(parents=True, exist_ok=True)

    # Find existing numbered subdirectories
    existing_runs = []
    for item in base_path.iterdir():
        if item.is_dir() and item.name.isdigit():
            existing_runs.append(int(item.name))

    # Next run number
    next_run = max(existing_runs) + 1 if existing_runs else 1

    return base_path / str(next_run)


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
        "--output",
        default="grid_heatmap.png",
        help="Output filename (or directory for PCAP mode)",
    )
    parser.add_argument("--dpi", type=int, default=150, help="Output image DPI")

    # PCAP replay options
    parser.add_argument(
        "--pcap",
        type=str,
        default=None,
        help="PCAP file to replay (enables periodic snapshot mode)",
    )
    parser.add_argument(
        "--interval",
        type=int,
        default=30,
        help="Interval between snapshots in seconds (snapshot mode, default: 30)",
    )
    parser.add_argument(
        "--duration",
        type=int,
        default=None,
        help="Total duration to capture in seconds (snapshot mode, default: until stable or infinite)",
    )
    parser.add_argument(
        "--output-dir",
        type=str,
        default=None,
        help="Output directory for snapshots (default: tools/grid-heatmap/output/...)",
    )

    args = parser.parse_args()

    # PCAP mode - process PCAP file with periodic snapshots
    if args.pcap:
        # Default output directory
        if args.output_dir is None:
            pcap_name = Path(args.pcap).stem
            script_dir = Path(__file__).parent
            output_base = script_dir / "output" / f"grid-heatmap-{pcap_name}"
            args.output_dir = str(get_next_run_dir(output_base))

        process_pcap_with_snapshots(
            base_url=args.url,
            sensor_id=args.sensor,
            pcap_file=args.pcap,
            interval=args.interval,
            duration=args.duration,
            output_dir=args.output_dir,
            azimuth_bucket=args.azimuth_bucket,
            settled_threshold=args.settled_threshold,
            metric=args.metric,
            dpi=args.dpi,
        )
        return

    # Live snapshot mode - periodic captures from running system
    # Triggered by --interval or --duration without --pcap
    if args.interval != 30 or args.duration is not None:
        # Default output directory
        if args.output_dir is None:
            script_dir = Path(__file__).parent
            output_base = script_dir / "output" / "live-snapshots"
            args.output_dir = str(get_next_run_dir(output_base))

        process_live_snapshots(
            base_url=args.url,
            sensor_id=args.sensor,
            interval=args.interval,
            duration=args.duration,
            output_dir=args.output_dir,
            azimuth_bucket=args.azimuth_bucket,
            settled_threshold=args.settled_threshold,
            metric=args.metric,
            dpi=args.dpi,
        )
        return

    # Single snapshot mode (original behavior) - generate full dashboard
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

    # Generate full dashboard
    plot_full_dashboard(heatmap, args.metric, args.output, args.dpi)
    print(f"\n✓ Saved: {args.output}")


if __name__ == "__main__":
    main()
