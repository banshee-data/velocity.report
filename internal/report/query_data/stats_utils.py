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
        # If the parsed datetime is naive, assume it is in UTC.
        if dt.tzinfo is None:
            dt = dt.replace(tzinfo=timezone.utc)

        # If a target timezone was requested, convert to it. Otherwise,
        # preserve the datetime's timezone (do not force conversion to UTC),
        # so that times returned by the API in local zones remain local.
        if tz_name:
            try:
                tzobj = ZoneInfo(tz_name)
            except Exception:
                tzobj = timezone.utc
            dt = dt.astimezone(tzobj)
        return dt.strftime("%-m/%-d %H:%M")
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
        fig, ax = plt.subplots(figsize=(3, 2))
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

    fig, ax = plt.subplots(figsize=(3, 2))
    x = list(range(len(labels)))
    ax.bar(x, counts, alpha=0.7, color="steelblue", edgecolor="black", linewidth=0.5)

    # Font sizes
    title_fs = 14
    label_fs = 13
    tick_fs = 11

    ax.set_xlabel(f"Velocity ({units})", fontsize=label_fs)
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

    try:
        fig.tight_layout(pad=0)
    except Exception:
        pass
    try:
        fig.subplots_adjust(left=0.02, right=0.985, top=0.96, bottom=0.08)
    except Exception:
        pass

    return fig


def save_chart_as_pdf(fig, output_path: str, close_fig: bool = True) -> bool:
    """Save matplotlib figure as PDF and optionally close it.

    Returns:
        True if successful, False otherwise
    """
    # Try to save the figure first. Closing the figure is best-effort and
    # should not cause the function to be considered a failure if matplotlib
    # isn't available in the test environment.
    try:
        # Try to compute the figure's tight bounding box and resize the figure
        # so that saving the PDF produces a page that tightly fits the content.
        try:
            import matplotlib.pyplot as plt

            # Force a draw so the renderer has up-to-date sizes
            fig.canvas.draw()
            renderer = fig.canvas.get_renderer()
            try:
                tight_bbox = fig.get_tightbbox(renderer)
            except Exception:
                tight_bbox = None

            if tight_bbox is not None:
                # tight_bbox dimensions are in display units (points). Convert to inches
                dpi = (
                    fig.dpi
                    if hasattr(fig, "dpi")
                    else plt.rcParams.get("figure.dpi", 72)
                )
                width_in = tight_bbox.width / dpi
                height_in = tight_bbox.height / dpi
                # Guard against zero/invalid sizes
                if width_in > 0 and height_in > 0:
                    try:
                        # Prevent creating extremely small PDFs which will later be
                        # upscaled by LaTeX (resulting in 'zoomed' charts). Enforce a
                        # sensible minimum width and an upper cap, scaling height
                        # proportionally.
                        MIN_WIDTH_IN = 6.0
                        MAX_WIDTH_IN = 11.0
                        if width_in < MIN_WIDTH_IN:
                            scale = MIN_WIDTH_IN / width_in
                            width_in = MIN_WIDTH_IN
                            height_in = height_in * scale
                        elif width_in > MAX_WIDTH_IN:
                            scale = MAX_WIDTH_IN / width_in
                            width_in = MAX_WIDTH_IN
                            height_in = height_in * scale
                        fig.set_size_inches(width_in, height_in)
                    except Exception:
                        pass

            # Save with tight bbox and zero padding
            fig.savefig(output_path, bbox_inches="tight", pad_inches=0.0)
        except Exception:
            # Fallback: older matplotlib or unexpected failure; try a simple save
            fig.savefig(output_path)
    except Exception:
        return False

    if close_fig:
        try:
            import matplotlib.pyplot as plt

            plt.close(fig)
        except Exception:
            # Ignore close failures (e.g., matplotlib not installed in test env)
            pass

    return True


def chart_exists(charts_prefix: str, chart_type: str) -> bool:
    """Check if a chart file exists for the given prefix and type."""
    return os.path.exists(f"{charts_prefix}_{chart_type}.pdf")
