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


def format_histogram_labels(
    labels: List[Any], max_bucket: Optional[float] = None
) -> List[str]:
    """Format histogram bucket labels to range format (e.g., '5-10', '50+').

    This is the canonical function for formatting histogram labels, used by
    both chart and table builders to ensure consistent display.

    Converts bucket start values to range labels:
    - Single values like '5', '10' → '5-10', '10-15', etc.
    - Detects bucket size from consecutive labels
    - Last bucket formatted as 'N+' (open-ended)
    - If max_bucket is set, the bucket before max_bucket is capped to end at
      max_bucket (e.g., '70-75' not '70-80'), and max_bucket itself shows as 'N+'
    - Non-numeric labels passed through unchanged

    Args:
        labels: List of bucket label values (strings or numbers)
        max_bucket: Optional maximum bucket value for cutoff

    Returns:
        List of formatted label strings
    """
    formatted = []

    # Try to parse labels as floats to detect ranges
    numeric_labels = []
    for lbl in labels:
        try:
            numeric_labels.append(float(lbl))
        except Exception:
            # Non-numeric label - pass through as-is
            formatted.append(str(lbl))
            continue

    # If we have numeric labels, convert to ranges
    if numeric_labels:
        # Detect bucket size from first two consecutive labels
        bucket_size = None
        if len(numeric_labels) >= 2:
            bucket_size = numeric_labels[1] - numeric_labels[0]

        for i, val in enumerate(numeric_labels):
            is_last = i == len(numeric_labels) - 1

            # Check if this bucket should be shown as N+ or as a range
            # If max_bucket is set and this value equals max_bucket, show as N+
            # Otherwise, if it's the last bucket, show as N+
            # Otherwise, show as a range A-B
            if max_bucket is not None and val == max_bucket:
                # This is the max_bucket cutoff - show as "N+"
                formatted.append(f"{int(val)}+")
            elif is_last and (max_bucket is None or val > max_bucket):
                # Last bucket and no max_bucket, or beyond max_bucket: format as "N+"
                formatted.append(f"{int(val)}+")
            elif bucket_size:
                # Regular bucket: format as "A-B"
                # If max_bucket is set, we're below max_bucket, and next bucket
                # would reach or exceed it, cap the range at max_bucket
                # (e.g., "70-75" instead of "70-80")
                next_val = val + bucket_size
                if (
                    max_bucket is not None
                    and val < max_bucket
                    and next_val > max_bucket
                    and not is_last
                ):
                    # Cap at max_bucket
                    formatted.append(f"{int(val)}-{int(max_bucket)}")
                else:
                    formatted.append(f"{int(val)}-{int(next_val)}")
            else:
                # Fallback: just show the value
                formatted.append(f"{int(val)}")

    return formatted


def compute_histogram_ranges(
    numeric_buckets: Dict[float, int],
    bucket_size: float,
    max_bucket: Optional[float] = None,
) -> List[Tuple[float, float]]:
    """Compute bucket ranges from histogram data for table display.

    This is the canonical function for computing histogram ranges, used by
    table builders to ensure consistent bucket boundaries.

    Args:
        numeric_buckets: Dictionary mapping bucket start values to counts
        bucket_size: Width of each bucket
        max_bucket: Optional maximum bucket value for cutoff

    Returns:
        List of (start, end) tuples for display ranges. The last range before
        max_bucket will be capped to end at max_bucket if needed.
    """
    if not numeric_buckets:
        return []

    ranges: List[Tuple[float, float]] = []
    inferred_bucket = float(bucket_size)

    try:
        sorted_keys = sorted(numeric_buckets.keys())

        # Infer bucket size from data if possible
        if len(sorted_keys) > 1:
            diffs = [j - i for i, j in zip(sorted_keys[:-1], sorted_keys[1:])]
            positive_diffs = [d for d in diffs if d > 0]
            if positive_diffs:
                inferred_bucket = float(min(positive_diffs))

        min_k = float(sorted_keys[0])
        max_k = float(sorted_keys[-1])

        # Guard against pathological zero bucket
        if inferred_bucket <= 0:
            inferred_bucket = float(bucket_size) or 5.0

        # Determine the upper limit for ranges
        if max_bucket is not None and max_bucket > min_k:
            upper_limit = max_bucket
        else:
            upper_limit = max_k

        # Build ranges from min_k up to upper_limit
        s = min_k
        while s < upper_limit:
            next_val = s + inferred_bucket
            # If max_bucket is set and next_val would exceed it, cap at max_bucket
            if max_bucket is not None and s < max_bucket and next_val > max_bucket:
                ranges.append((s, max_bucket))
            else:
                ranges.append((s, next_val))
            s += inferred_bucket

    except Exception:
        pass

    return ranges


def plot_histogram(
    histogram: Dict[str, int],
    title: str,
    units: str,
    debug: bool = False,
    max_bucket: Optional[float] = None,
) -> Optional[object]:
    """Plot histogram data using matplotlib.

    Args:
        histogram: Dict mapping bucket labels to counts
        title: Chart title
        units: Units string for axis labels
        debug: Enable debug output
        max_bucket: Maximum bucket value for cutoff (affects label formatting)

    Returns:
        matplotlib figure object or None if matplotlib not available
    """
    if not HAVE_CHARTS or HistogramChartBuilder is None:
        return None

    builder = HistogramChartBuilder()
    return builder.build(histogram, title, units, debug, max_bucket)


def plot_comparison_histogram(
    histogram: Dict[str, int],
    compare_histogram: Dict[str, int],
    title: str,
    units: str,
    primary_label: str,
    compare_label: str,
    debug: bool = False,
) -> Optional[object]:
    """Plot comparison histogram data using matplotlib."""
    if not HAVE_CHARTS or HistogramChartBuilder is None:
        return None

    builder = HistogramChartBuilder()
    return builder.build_comparison(
        histogram, compare_histogram, title, units, primary_label, compare_label, debug
    )


def chart_exists(charts_prefix: str, chart_type: str) -> bool:
    """Check if a chart file exists for the given prefix and type."""
    return os.path.exists(f"{charts_prefix}_{chart_type}.pdf")
