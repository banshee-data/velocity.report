#!/usr/bin/env python3

"""Statistics and data transformation utilities.

This module provides common functions for formatting, processing histogram data,
and creating charts that are used across PDF and LaTeX generation.
"""

import os
from typing import Any, Dict, List, Optional, Tuple
from datetime import timezone
from zoneinfo import ZoneInfo

import numpy as np

from date_parser import parse_server_time


def format_time(tval: Any, tz_name: Optional[str]) -> str:
    """Format a server time value for display."""
    try:
        dt = parse_server_time(tval)
        if tz_name:
            try:
                tzobj = ZoneInfo(tz_name)
            except Exception:
                tzobj = timezone.utc
            dt = dt.astimezone(tzobj)
        else:
            dt = dt.astimezone(timezone.utc)
        return dt.strftime("%m-%d %H:%M")
    except Exception:
        return str(tval)


def format_number(v: Any) -> str:
    """Format a numeric value for display."""
    try:
        if v is None:
            return "--"
        f = float(v)
        if np.isnan(f):
            return "--"
        return f"{f:.2f}"
    except Exception:
        return "--"


def process_histogram(
    histogram: Dict[str, int],
    cutoff: float = 5.0,
    bucket_size: float = 5.0,
    max_bucket: float = 50.0,
) -> Tuple[Dict[float, int], int, List[tuple]]:
    """Process histogram data for display.

    Returns:
        - numeric_buckets: Dictionary of float keys to counts
        - total: Total count across all buckets
        - ranges: List of (start, end) tuples for display ranges
    """
    # Coerce numeric keys into buckets and compute total
    numeric_buckets: Dict[float, int] = {}
    total = 0
    for k, v in histogram.items():
        try:
            fk = float(k)
            numeric_buckets[fk] = numeric_buckets.get(fk, 0) + int(v)
            total += int(v)
        except Exception:
            try:
                total += int(v)
            except Exception:
                pass

    # Display ranges
    ranges: List[tuple] = []
    s = cutoff
    while s < max_bucket:
        ranges.append((s, s + bucket_size))
        s += bucket_size

    return numeric_buckets, total, ranges


def count_in_histogram_range(
    numeric_buckets: Dict[float, int], a: float, b: float
) -> int:
    """Count histogram entries in the range [a, b)."""
    return sum(v for k, v in numeric_buckets.items() if k >= a and k < b)


def count_histogram_ge(numeric_buckets: Dict[float, int], a: float) -> int:
    """Count histogram entries >= a."""
    return sum(v for k, v in numeric_buckets.items() if k >= a)


def plot_histogram(
    histogram: Dict[str, int],
    title: str,
    units: str,
    debug: bool = False,
) -> Optional[object]:
    """Plot histogram data using matplotlib.

    Returns:
        matplotlib figure object or None if matplotlib not available
    """
    try:
        import matplotlib.pyplot as plt
    except Exception as e:  # pragma: no cover - environment dependent
        raise ImportError("matplotlib is required to render histograms") from e

    if not histogram:
        fig, ax = plt.subplots(figsize=(10, 4))
        ax.text(0.5, 0.5, "No histogram data", ha="center", va="center")
        ax.set_title(title)
        return fig

    try:
        sorted_items = sorted(histogram.items(), key=lambda x: float(x[0]))
    except Exception:
        sorted_items = sorted(histogram.items(), key=lambda x: str(x[0]))

    labels = [item[0] for item in sorted_items]
    counts = [item[1] for item in sorted_items]

    if debug:
        total = sum(counts)
        print(f"DEBUG: histogram bins={len(labels)} total={total}")

    fig, ax = plt.subplots(figsize=(4, 3))
    x = list(range(len(labels)))
    ax.bar(x, counts, alpha=0.7, color="steelblue", edgecolor="black", linewidth=0.5)

    # Font sizes
    title_fs = 11
    label_fs = 11
    tick_fs = 10

    ax.set_xlabel(f"Speed ({units})", fontsize=label_fs)
    ax.set_ylabel("Count", fontsize=label_fs)
    ax.set_title(title, fontsize=title_fs)

    # Prepare x-axis labels: format numeric labels with 0 decimal places
    formatted_labels: List[str] = []
    for lbl in labels:
        try:
            f = float(lbl)
            formatted_labels.append(f"{f:.0f}")
        except Exception:
            # fallback to original string label
            formatted_labels.append(str(lbl))

    if len(labels) <= 20:
        ax.set_xticks(x)
        ax.set_xticklabels(formatted_labels, rotation=45, ha="right", fontsize=tick_fs)
    else:
        step = max(1, len(labels) // 15)
        tick_pos = x[::step]
        tick_labels = formatted_labels[::step]
        ax.set_xticks(tick_pos)
        ax.set_xticklabels(tick_labels, rotation=45, ha="right", fontsize=tick_fs)

    # Apply tick params for consistency
    ax.tick_params(axis="both", which="major", labelsize=tick_fs)

    try:
        fig.tight_layout(pad=0.5)
    except Exception:
        pass

    return fig


def save_chart_as_pdf(fig, output_path: str, close_fig: bool = True) -> bool:
    """Save matplotlib figure as PDF and optionally close it.

    Returns:
        True if successful, False otherwise
    """
    try:
        fig.savefig(output_path)
        if close_fig:
            import matplotlib.pyplot as plt

            plt.close(fig)
        return True
    except Exception:
        return False


def chart_exists(charts_prefix: str, chart_type: str) -> bool:
    """Check if a chart file exists for the given prefix and type."""
    return os.path.exists(f"{charts_prefix}_{chart_type}.pdf")
