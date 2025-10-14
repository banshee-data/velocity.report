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

from pdf_generator.core.date_parser import parse_server_time
from pdf_generator.core.config_manager import DEFAULT_HISTOGRAM_PROCESSING_CONFIG

# Optional matplotlib dependencies
try:
    from pdf_generator.core.chart_builder import HistogramChartBuilder

    HAVE_CHARTS = True
except ImportError:
    HistogramChartBuilder = None  # type: ignore
    HAVE_CHARTS = False


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
    cutoff: float = None,
    bucket_size: float = None,
    max_bucket: float = None,
) -> Tuple[Dict[float, int], int, List[tuple]]:
    """Process histogram data for display.

    Returns:
        - numeric_buckets: Dictionary of float keys to counts
        - total: Total count across all buckets
        - ranges: List of (start, end) tuples for display ranges
    """
    # Use config defaults if not provided
    if cutoff is None:
        cutoff = DEFAULT_HISTOGRAM_PROCESSING_CONFIG.default_cutoff
    if bucket_size is None:
        bucket_size = DEFAULT_HISTOGRAM_PROCESSING_CONFIG.default_bucket_size
    if max_bucket is None:
        max_bucket = DEFAULT_HISTOGRAM_PROCESSING_CONFIG.default_max_bucket

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
    if not HAVE_CHARTS or HistogramChartBuilder is None:
        return None

    builder = HistogramChartBuilder()
    return builder.build(histogram, title, units, debug)


def chart_exists(charts_prefix: str, chart_type: str) -> bool:
    """Check if a chart file exists for the given prefix and type."""
    return os.path.exists(f"{charts_prefix}_{chart_type}.pdf")
