#!/usr/bin/env python3
"""CLI wrapper that orchestrates date parsing, API queries, and PDF generation.

Uses:
- date_parser.parse_date_to_unix, is_date_only
- api_client.RadarStatsClient
- pdf_generator.generate_pdf_report
- stats_utils for plotting and data processing

This file intentionally avoids plotting to keep the runtime lightweight and to rely
on the already-tested modules for parsing and API interactions.
"""

import argparse
import os
import re
import sys
import textwrap
import traceback
from typing import Any, Dict, List, Optional, Tuple
from datetime import datetime, timezone as dt_timezone
from zoneinfo import ZoneInfo

import requests

from pdf_generator.core.api_client import RadarStatsClient, SUPPORTED_GROUPS
from pdf_generator.core.config_manager import ReportConfig
from pdf_generator.core.date_parser import (
    parse_date_to_unix,
    is_date_only,
)
from pdf_generator.core.pdf_generator import generate_pdf_report
from pdf_generator.core.zip_utils import create_sources_zip
from pdf_generator.core.stats_utils import plot_histogram, plot_comparison_histogram
from pdf_generator.core.data_transformers import (
    MetricsNormalizer,
    extract_count_from_row,
)

try:  # Optional chart dependencies (matplotlib)
    from pdf_generator.core.chart_builder import TimeSeriesChartBuilder  # type: ignore  # noqa: F401
except ImportError:  # pragma: no cover - optional dependency missing during runtime
    TimeSeriesChartBuilder = None  # type: ignore[assignment]

try:
    from pdf_generator.core.chart_saver import save_chart_as_pdf  # type: ignore  # noqa: F401
except ImportError:  # pragma: no cover - optional dependency missing during runtime
    save_chart_as_pdf = None  # type: ignore[assignment]


def _import_chart_builder():
    """Dynamically import the chart builder to avoid hard matplotlib dependency."""

    global TimeSeriesChartBuilder  # type: ignore[global-variable-not-assigned]

    if TimeSeriesChartBuilder is not None:
        return TimeSeriesChartBuilder

    try:
        from pdf_generator.core.chart_builder import (
            TimeSeriesChartBuilder as _TimeSeriesChartBuilder,
        )
    except ImportError as exc:  # pragma: no cover - optional dependency
        raise ImportError("chart_builder module unavailable") from exc

    TimeSeriesChartBuilder = _TimeSeriesChartBuilder  # type: ignore[assignment]
    return TimeSeriesChartBuilder


def _import_chart_saver():
    """Dynamically import the chart saver helper."""

    global save_chart_as_pdf  # type: ignore[global-variable-not-assigned]

    if save_chart_as_pdf is not None:
        return save_chart_as_pdf

    try:
        from pdf_generator.core.chart_saver import (
            save_chart_as_pdf as _save_chart_as_pdf,
        )
    except ImportError as exc:  # pragma: no cover - optional dependency
        raise ImportError("chart_saver module unavailable") from exc

    save_chart_as_pdf = _save_chart_as_pdf  # type: ignore[assignment]
    return save_chart_as_pdf


def _print_error(message: str) -> None:
    print(message, file=sys.stderr)


def _print_info(message: str) -> None:
    print(message)


def _append_debug_hint(message: str, debug_enabled: bool) -> str:
    if debug_enabled:
        return message
    return f"{message}\n  - Re-run with --debug for traceback details."


def _maybe_print_debug(exc: Exception, debug_enabled: bool) -> None:
    if not debug_enabled:
        return
    stack = "".join(traceback.format_exception(exc)).rstrip()
    _print_error(f"DEBUG: {type(exc).__name__}: {exc}")
    if stack:
        _print_error(stack)


def _format_api_error(action: str, api_url: str, exc: Exception) -> str:
    parts: List[str] = []

    if isinstance(exc, requests.exceptions.HTTPError):
        status = (
            exc.response.status_code if getattr(exc, "response", None) else "unknown"
        )
        parts.append(f"HTTP {status} from {api_url}")
        if status == 400:
            parts.append("Check date range, group, and filters in config.query.*.")
        elif status == 404:
            parts.append(
                "Endpoint not found. Verify the Go API version or base_url override."
            )
        elif status in (401, 403):
            parts.append("Authentication failed. Confirm the API allows your request.")
        elif isinstance(status, int) and status >= 500:
            parts.append(
                "Go service returned a server error. Inspect `journalctl -u velocity-report.service`."
            )
        if getattr(exc, "response", None) is not None:
            body = exc.response.text.strip()
            if body:
                preview = body.splitlines()[0][:200]
                parts.append(f"Response snippet: {preview}")
    elif isinstance(exc, requests.exceptions.ConnectionError):
        parts.append(f"Unable to reach {api_url}. Is the Go API running and reachable?")
        parts.append("Check network connectivity and device firewall rules.")
    elif isinstance(exc, requests.exceptions.Timeout):
        parts.append("Request timed out. The API may be offline or under heavy load.")
    else:
        parts.append(str(exc))

    bullet_lines = "\n  - ".join(parts)
    return textwrap.dedent(
        f"""\
        {action} failed.
          - {bullet_lines}
        """
    ).strip()


def should_produce_daily(group_token: str) -> bool:
    # If the requested group is already daily or larger, don't produce a separate daily table
    provided_group_seconds = SUPPORTED_GROUPS.get(group_token)
    if provided_group_seconds is not None and provided_group_seconds >= 24 * 3600:
        return False

    # Accept formats like '15m', '1h', '2d' as a fallback
    import re

    m = re.match(r"^(\d+)([smhd])$", str(group_token or ""))
    if m:
        num = int(m.group(1))
        unit = m.group(2)
        seconds = None
        if unit == "s":
            seconds = num
        elif unit == "m":
            seconds = num * 60
        elif unit == "h":
            seconds = num * 3600
        elif unit == "d":
            seconds = num * 86400
        if seconds is not None and seconds >= 24 * 3600:
            return False
    return True


def _next_sequenced_prefix(base: str, search_dir: str = ".") -> str:
    """Return a sequenced prefix like base-1-HHMMSS, base-2-HHMMSS, ... based on files in search_dir.

    This scans the specified directory for files beginning with ``base-<n>`` and
    returns the next number in the sequence with a timestamp suffix.
    Always returns base-<n>-HHMMSS (start at 1).
    The timestamp helps avoid caching issues with PDF viewers.

    Args:
        base: Base prefix for the file
        search_dir: Directory to search for existing files (default: current directory)
    """
    # Handle non-existent directory (will be created later)
    if not os.path.exists(search_dir):
        timestamp = datetime.now().strftime("%H%M%S")
        return f"{base}-1-{timestamp}"

    files = os.listdir(search_dir)
    pat = re.compile(r"^" + re.escape(base) + r"-(\d+)(?:-\d{6})?(?:_|$)")
    nums = []
    for fn in files:
        m = pat.match(fn)
        if m:
            try:
                nums.append(int(m.group(1)))
            except Exception:
                continue
    next_n = max(nums) + 1 if nums else 1
    timestamp = datetime.now().strftime("%H%M%S")
    return f"{base}-{next_n}-{timestamp}"


def derive_overall_from_granular(
    granular_metrics: List[Dict[str, Any]],
) -> List[Dict[str, Any]]:
    """Derive overall summary metrics from granular data.

    This computes aggregated statistics from granular time-bucketed data,
    ensuring consistency with boundary hour filtering applied at the DB level.

    Args:
        granular_metrics: List of granular metric dictionaries

    Returns:
        List containing a single overall summary metric (or empty list)
    """
    import math

    if not granular_metrics:
        return []

    # Collect all speeds weighted by count for percentile calculation
    total_count = 0
    max_speed = 0.0
    all_p50s = []
    all_p85s = []
    all_p98s = []

    normalizer = MetricsNormalizer()

    for row in granular_metrics:
        count = normalizer.get_numeric(row, "count") or 0
        if count is None or (isinstance(count, float) and math.isnan(count)):
            count = 0
        if count <= 0:
            continue

        total_count += int(count)

        p50 = normalizer.get_numeric(row, "p50")
        p85 = normalizer.get_numeric(row, "p85")
        p98 = normalizer.get_numeric(row, "p98")
        max_spd = normalizer.get_numeric(row, "max_speed")

        if p50 is not None and not math.isnan(p50):
            all_p50s.extend([p50] * int(count))
        if p85 is not None and not math.isnan(p85):
            all_p85s.extend([p85] * int(count))
        if p98 is not None and not math.isnan(p98):
            all_p98s.extend([p98] * int(count))
        if max_spd is not None and not math.isnan(max_spd) and max_spd > max_speed:
            max_speed = max_spd

    if total_count == 0:
        return []

    # Compute weighted median for each percentile
    def median(values: List[float]) -> float:
        if not values:
            return 0.0
        sorted_vals = sorted(values)
        n = len(sorted_vals)
        mid = n // 2
        if n % 2 == 0:
            return (sorted_vals[mid - 1] + sorted_vals[mid]) / 2
        return sorted_vals[mid]

    # Get start time from first row
    start_time = None
    if granular_metrics:
        start_time = normalizer.get_value(granular_metrics[0], "start_time")

    return [
        {
            "start_time": start_time,
            "count": total_count,
            "p50_speed": median(all_p50s) if all_p50s else 0,
            "p85_speed": median(all_p85s) if all_p85s else 0,
            "p98_speed": median(all_p98s) if all_p98s else 0,
            "max_speed": max_speed,
            "classifier": "all",
        }
    ]


def derive_daily_from_granular(
    granular_metrics: List[Dict[str, Any]],
    timezone: Optional[str] = None,
) -> List[Dict[str, Any]]:
    """Derive daily summary metrics from granular data.

    Groups granular metrics by day and computes daily aggregates,
    ensuring consistency with boundary hour filtering.

    Args:
        granular_metrics: List of granular metric dictionaries
        timezone: Timezone for day boundary calculation

    Returns:
        List of daily summary metrics
    """
    if not granular_metrics:
        return []

    import math
    from collections import defaultdict

    try:
        tzobj = ZoneInfo(timezone) if timezone else dt_timezone.utc
    except Exception:
        tzobj = dt_timezone.utc

    normalizer = MetricsNormalizer()

    # Group by day
    day_data: Dict[str, Dict[str, Any]] = defaultdict(
        lambda: {
            "count": 0,
            "max_speed": 0.0,
            "p50s": [],
            "p85s": [],
            "p98s": [],
            "start_time": None,
        }
    )

    for row in granular_metrics:
        # Parse start time
        start_time_raw = normalizer.get_value(row, "start_time")
        if start_time_raw is None:
            continue

        # Convert to datetime
        try:
            if isinstance(start_time_raw, str):
                dt = datetime.fromisoformat(start_time_raw.replace("Z", "+00:00"))
            elif isinstance(start_time_raw, (int, float)):
                dt = datetime.fromtimestamp(start_time_raw, tz=dt_timezone.utc)
            else:
                continue
            # Convert to target timezone for day grouping
            dt = dt.astimezone(tzobj)
            day_key = dt.strftime("%Y-%m-%d")
        except Exception:
            continue

        count = normalizer.get_numeric(row, "count") or 0
        if count is None or (isinstance(count, float) and math.isnan(count)):
            count = 0
        if count <= 0:
            continue

        day = day_data[day_key]
        day["count"] += int(count)

        p50 = normalizer.get_numeric(row, "p50")
        p85 = normalizer.get_numeric(row, "p85")
        p98 = normalizer.get_numeric(row, "p98")
        max_spd = normalizer.get_numeric(row, "max_speed")

        if p50 is not None and not math.isnan(p50):
            day["p50s"].extend([p50] * int(count))
        if p85 is not None and not math.isnan(p85):
            day["p85s"].extend([p85] * int(count))
        if p98 is not None and not math.isnan(p98):
            day["p98s"].extend([p98] * int(count))
        if (
            max_spd is not None
            and not math.isnan(max_spd)
            and max_spd > day["max_speed"]
        ):
            day["max_speed"] = max_spd

        # Track earliest time in this day
        if day["start_time"] is None:
            day["start_time"] = dt.replace(hour=0, minute=0, second=0, microsecond=0)

    # Convert to list of metrics
    def median(values: List[float]) -> float:
        if not values:
            return 0.0
        sorted_vals = sorted(values)
        n = len(sorted_vals)
        mid = n // 2
        if n % 2 == 0:
            return (sorted_vals[mid - 1] + sorted_vals[mid]) / 2
        return sorted_vals[mid]

    result = []
    for day_key in sorted(day_data.keys()):
        day = day_data[day_key]
        if day["count"] == 0:
            continue
        result.append(
            {
                "start_time": (
                    day["start_time"].isoformat() if day["start_time"] else day_key
                ),
                "count": day["count"],
                "p50_speed": median(day["p50s"]),
                "p85_speed": median(day["p85s"]),
                "p98_speed": median(day["p98s"]),
                "max_speed": day["max_speed"],
                "classifier": "all",
            }
        )

    return result


def _plot_stats_page(stats, title: str, units: str, tz_name: Optional[str] = None):
    """Create a compact time-series plot (P50/P85/P98/Max + counts bars).

    Returns a matplotlib Figure.
    """
    builder_cls = _import_chart_builder()
    builder = builder_cls()
    return builder.build(stats, title, units, tz_name)


# === Configuration & Validation ===


def compute_iso_timestamps(
    start_ts: int, end_ts: int, timezone: Optional[str]
) -> Tuple[str, str]:
    """Convert unix timestamps to ISO strings with timezone.

    Args:
        start_ts: Start timestamp in unix seconds
        end_ts: End timestamp in unix seconds
        timezone: Timezone name (e.g., 'US/Pacific') or None for UTC

    Returns:
        Tuple of (start_iso, end_iso) strings
    """
    try:
        tzobj = ZoneInfo(timezone) if timezone else dt_timezone.utc
        start_iso = datetime.fromtimestamp(start_ts, tz=tzobj).isoformat()
        end_iso = datetime.fromtimestamp(end_ts, tz=tzobj).isoformat()
        return start_iso, end_iso
    except Exception:
        # Fallback to basic string representation
        return str(start_ts), str(end_ts)


def resolve_file_prefix(
    config: ReportConfig, start_ts: int, end_ts: int, output_dir: str = "."
) -> str:
    """Determine output file prefix (sequenced or date-based).

    All files are prefixed with 'velocity.report_' followed by either:
    - User-provided prefix (sequenced)
    - Auto-generated: {source}_{start_date}_to_{end_date}

    Args:
        config: Report configuration
        start_ts: Start timestamp
        end_ts: End timestamp
        output_dir: Directory where files will be created (for sequence checking)

    Returns:
        File prefix string with 'velocity.report_' prefix
    """
    if config.output.file_prefix:
        # User provided a prefix - create numbered sequence
        base_prefix = _next_sequenced_prefix(config.output.file_prefix, output_dir)
        return f"velocity.report_{base_prefix}"
    else:
        # Auto-generate from date range using original date strings (single source of truth)
        # These come directly from the datepicker in the UI via config.query
        start_label = config.query.start_date[:10]
        end_label = config.query.end_date[:10]
        return f"velocity.report_{config.query.source}_{start_label}_to_{end_label}"


# === API Data Fetching ===


def fetch_granular_metrics(
    client: RadarStatsClient,
    start_ts: int,
    end_ts: int,
    config: ReportConfig,
    model_version: Optional[str],
    source_override: Optional[str] = None,
) -> Tuple[List, Optional[dict], Optional[object]]:
    """Fetch main granular metrics and optional histogram.

    Args:
        client: API client instance
        start_ts: Start timestamp
        end_ts: End timestamp
        config: Report configuration
        model_version: Model version for transit data
        source_override: Optional source to use instead of config.query.source

    Returns:
        Tuple of (metrics, histogram, min_speed_used, response_metadata)
    """
    source = source_override or config.query.source
    try:
        metrics, histogram, min_speed_used, resp = client.get_stats(
            start_ts=start_ts,
            end_ts=end_ts,
            group=config.query.group,
            units=config.query.units,
            source=source,
            model_version=model_version,
            timezone=config.query.timezone or None,
            min_speed=config.query.min_speed,
            compute_histogram=config.query.histogram,
            hist_bucket_size=config.query.hist_bucket_size,
            hist_max=config.query.hist_max,
            site_id=config.query.site_id,
            boundary_threshold=config.query.boundary_threshold,
        )
        return metrics, histogram, min_speed_used, resp
    except Exception as exc:
        message = _format_api_error("Fetching granular metrics", client.api_url, exc)
        message = _append_debug_hint(message, config.output.debug)
        _print_error(message)
        _maybe_print_debug(exc, config.output.debug)
        return [], None, None, None


def fetch_overall_summary(
    client: RadarStatsClient,
    start_ts: int,
    end_ts: int,
    config: ReportConfig,
    model_version: Optional[str],
    source_override: Optional[str] = None,
) -> List:
    """Fetch overall 'all' group summary.

    Args:
        client: API client instance
        start_ts: Start timestamp
        end_ts: End timestamp
        config: Report configuration
        model_version: Model version for transit data
        source_override: Optional source to use instead of config.query.source

    Returns:
        List of overall metrics (empty list on failure)
    """
    source = source_override or config.query.source
    try:
        metrics_all, _, _, _ = client.get_stats(
            start_ts=start_ts,
            end_ts=end_ts,
            group="all",
            units=config.query.units,
            source=source,
            model_version=model_version,
            timezone=config.query.timezone or None,
            min_speed=config.query.min_speed,
            compute_histogram=False,
            site_id=config.query.site_id,
            boundary_threshold=config.query.boundary_threshold,
        )
        return metrics_all
    except Exception as exc:
        message = _format_api_error("Fetching overall summary", client.api_url, exc)
        message = _append_debug_hint(message, config.output.debug)
        _print_error(message)
        _maybe_print_debug(exc, config.output.debug)
        return []


def fetch_site_config_periods(
    client: RadarStatsClient,
    site_id: int,
    start_ts: int,
    end_ts: int,
    compare_start_ts: Optional[int] = None,
    compare_end_ts: Optional[int] = None,
) -> List[Dict[str, Any]]:
    """Fetch and filter site configuration periods for report ranges."""
    try:
        periods, _ = client.get_site_config_periods(site_id)
    except Exception as exc:
        message = _format_api_error(
            "Fetching site configuration periods",
            f"{client.base_url}/api/site_config_periods",
            exc,
        )
        _print_error(message)
        return []

    def overlaps(
        period_start: float,
        period_end: Optional[float],
        range_start: int,
        range_end: int,
    ) -> bool:
        end_value = period_end if period_end is not None else float("inf")
        return period_start < range_end and end_value > range_start

    filtered: List[Dict[str, Any]] = []
    for period in periods:
        period_start = float(period.get("effective_start_unix", 0))
        period_end_raw = period.get("effective_end_unix")
        period_end = float(period_end_raw) if period_end_raw is not None else None

        if overlaps(period_start, period_end, start_ts, end_ts):
            filtered.append(period)
            continue
        if compare_start_ts is not None and compare_end_ts is not None:
            if overlaps(period_start, period_end, compare_start_ts, compare_end_ts):
                filtered.append(period)

    filtered.sort(key=lambda p: float(p.get("effective_start_unix", 0)))
    return filtered


def fetch_daily_summary(
    client: RadarStatsClient,
    start_ts: int,
    end_ts: int,
    config: ReportConfig,
    model_version: Optional[str],
    source_override: Optional[str] = None,
) -> Optional[List]:
    """Fetch daily (24h) summary if appropriate for group size.

    Args:
        client: API client instance
        start_ts: Start timestamp
        end_ts: End timestamp
        config: Report configuration
        model_version: Model version for transit data
        source_override: Optional source to use instead of config.query.source

    Returns:
        List of daily metrics or None if not needed/failed
    """
    if not should_produce_daily(config.query.group):
        return None

    source = source_override or config.query.source
    try:
        daily_metrics, _, _, _ = client.get_stats(
            start_ts=start_ts,
            end_ts=end_ts,
            group="24h",
            units=config.query.units,
            source=source,
            model_version=model_version,
            timezone=config.query.timezone or None,
            min_speed=config.query.min_speed,
            compute_histogram=False,
            site_id=config.query.site_id,
            boundary_threshold=config.query.boundary_threshold,
        )
        return daily_metrics
    except Exception as exc:
        message = _format_api_error("Fetching daily summary", client.api_url, exc)
        message = _append_debug_hint(message, config.output.debug)
        _print_error(message)
        _maybe_print_debug(exc, config.output.debug)
        return None


# === Chart Generation ===


def generate_histogram_chart(
    histogram: dict,
    prefix: str,
    units: str,
    metrics_all: List,
    config: ReportConfig,
    compare_histogram: Optional[dict] = None,
    primary_label: Optional[str] = None,
    compare_label: Optional[str] = None,
) -> bool:
    """Generate histogram chart PDF.

    Args:
        histogram: Histogram data dictionary
        prefix: File prefix for output
        units: Display units
        metrics_all: Overall metrics for sample size
        config: Report configuration

    Returns:
        True if chart was created successfully
    """
    try:
        # Extract sample size from overall metrics
        sample_n = None
        normalizer = MetricsNormalizer()
        try:
            if hasattr(metrics_all, "get"):
                sample_n = extract_count_from_row(metrics_all, normalizer)
            elif isinstance(metrics_all, (list, tuple)) and metrics_all:
                first = metrics_all[0]
                if isinstance(first, dict):
                    sample_n = extract_count_from_row(first, normalizer)
        except Exception:
            sample_n = None

        sample_label = ""
        if sample_n is not None:
            try:
                sample_label = f" (n={int(sample_n)})"
            except Exception:
                sample_label = f" (n={sample_n})"

        if compare_histogram:
            primary_desc = primary_label or "Primary period"
            compare_desc = compare_label or "Comparison period"
            fig_hist = plot_comparison_histogram(
                histogram,
                compare_histogram,
                "Velocity Distribution Comparison",
                units,
                primary_desc,
                compare_desc,
                debug=config.output.debug,
            )
        else:
            fig_hist = plot_histogram(
                histogram,
                f"Velocity Distribution: {sample_label}",
                units,
                debug=config.output.debug,
            )
        hist_pdf = f"{prefix}_histogram.pdf"
        save_chart_as_pdf = _import_chart_saver()
        if save_chart_as_pdf(fig_hist, hist_pdf):
            print(f"Wrote histogram PDF: {hist_pdf}")
            return True
        else:
            _print_error(
                "Error: unable to write histogram PDF. Check disk space and permissions."
            )
            return False
    except ImportError as ie:
        message = "Histogram plotting unavailable. Install matplotlib and cairo to enable charts."
        message = f"{message}\n  - Details: {ie}"
        message = _append_debug_hint(message, config.output.debug)
        _print_error(message)
        _maybe_print_debug(ie, config.output.debug)
        return False
    except Exception as exc:
        message = "Error: failed to generate histogram PDF. Verify matplotlib setup and report data."
        message = f"{message}\n  - Details: {exc}"
        message = _append_debug_hint(message, config.output.debug)
        _print_error(message)
        _maybe_print_debug(exc, config.output.debug)
        return False


def generate_timeseries_chart(
    metrics: List,
    prefix: str,
    title: str,
    units: str,
    tz_name: Optional[str],
    config: ReportConfig,
) -> bool:
    """Generate time-series chart PDF.

    Args:
        metrics: Metrics data
        prefix: File prefix for output
        title: Chart title
        units: Display units
        tz_name: Timezone name
        config: Report configuration

    Returns:
        True if chart was created successfully
    """
    try:
        fig = _plot_stats_page(metrics, title, units, tz_name=tz_name)
        stats_pdf = f"{prefix}.pdf"
        save_chart_as_pdf = _import_chart_saver()
        if save_chart_as_pdf(fig, stats_pdf):
            print(f"Wrote {title} PDF: {stats_pdf}")
            return True
        else:
            _print_error(
                f"Error: unable to write {title} PDF. Check disk space and permissions."
            )
            return False
    except Exception as exc:
        message = f"Error: failed to generate {title} PDF. Ensure matplotlib is installed and input data is valid."
        message = f"{message}\n  - Details: {exc}"
        message = _append_debug_hint(message, config.output.debug)
        _print_error(message)
        _maybe_print_debug(exc, config.output.debug)
        return False


# === PDF Assembly ===


def assemble_pdf_report(
    prefix: str,
    start_iso: str,
    end_iso: str,
    overall_metrics: List,
    daily_metrics: Optional[List],
    granular_metrics: List,
    histogram: Optional[dict],
    config: ReportConfig,
    min_speed_used: Optional[float] = None,
    compare_start_iso: Optional[str] = None,
    compare_end_iso: Optional[str] = None,
    compare_overall_metrics: Optional[List] = None,
    compare_histogram: Optional[dict] = None,
    compare_granular_metrics: Optional[List] = None,
    compare_daily_metrics: Optional[List] = None,
    config_periods: Optional[List[Dict[str, Any]]] = None,
    cosine_correction_note: Optional[str] = None,
) -> bool:
    """Assemble complete PDF report.

    Args:
        prefix: File prefix for output
        start_iso: Start date in ISO format
        end_iso: End date in ISO format
        overall_metrics: Overall summary metrics
        daily_metrics: Daily metrics (or None)
        granular_metrics: Granular metrics
        histogram: Histogram data (or None)
        config: Report configuration

    Returns:
        True if PDF was generated successfully
    """
    # Use the actual min_speed_used from the API if available, otherwise fall back to config
    if min_speed_used is not None:
        min_speed_str = f"{min_speed_used:.1f} {config.query.units}"
    elif config.query.min_speed is not None:
        min_speed_str = f"{config.query.min_speed} {config.query.units}"
    else:
        min_speed_str = "none"

    tz_display = config.query.timezone or "UTC"
    pdf_path = f"{prefix}_report.pdf"

    try:
        # Debug: surface overall metrics presence to help diagnose missing speed values
        try:
            debug_enabled = bool(config.output.debug)
        except Exception:
            debug_enabled = False

        if debug_enabled:
            try:
                total_overall = (
                    len(overall_metrics) if overall_metrics is not None else 0
                )
            except Exception:
                total_overall = 0
            print(f"DEBUG: overall_metrics length={total_overall}")
            if total_overall:
                try:
                    print("DEBUG: overall_metrics[0]=", repr(overall_metrics[0]))
                except Exception:
                    print("DEBUG: overall_metrics[0] preview unavailable")

        generate_pdf_report(
            output_path=pdf_path,
            start_iso=start_iso,
            end_iso=end_iso,
            compare_start_iso=compare_start_iso,
            compare_end_iso=compare_end_iso,
            group=config.query.group,
            units=config.query.units,
            timezone_display=tz_display,
            min_speed_str=min_speed_str,
            location=config.site.location,
            overall_metrics=overall_metrics,
            compare_overall_metrics=compare_overall_metrics,
            daily_metrics=daily_metrics,
            compare_daily_metrics=compare_daily_metrics,
            granular_metrics=granular_metrics,
            compare_granular_metrics=compare_granular_metrics,
            histogram=histogram,
            compare_histogram=compare_histogram,
            tz_name=(config.query.timezone or None),
            charts_prefix=prefix,
            speed_limit=config.site.speed_limit,
            hist_max=config.query.hist_max,
            include_map=config.output.map,
            site_description=config.site.site_description,
            speed_limit_note=config.site.speed_limit_note,
            surveyor=config.site.surveyor,
            contact=config.site.contact,
            cosine_error_angle=config.radar.cosine_error_angle,
            sensor_model=config.radar.sensor_model,
            firmware_version=config.radar.firmware_version,
            transmit_frequency=config.radar.transmit_frequency,
            sample_rate=config.radar.sample_rate,
            velocity_resolution=config.radar.velocity_resolution,
            azimuth_fov=config.radar.azimuth_fov,
            elevation_fov=config.radar.elevation_fov,
            config_periods=config_periods,
            cosine_correction_note=cosine_correction_note,
            start_date=config.query.start_date,
            end_date=config.query.end_date,
            compare_start_date=config.query.compare_start_date,
            compare_end_date=config.query.compare_end_date,
        )
        print(f"Generated PDF report: {pdf_path}")
        return True
    except Exception as exc:
        message = "Error: failed to generate PDF report. Ensure XeLaTeX is installed and the output directory is writable."
        message = f"{message}\n  - Details: {exc}"
        message = _append_debug_hint(message, config.output.debug)
        _print_error(message)
        _maybe_print_debug(exc, config.output.debug)
        return False


# === Date Range Processing ===


def parse_date_range(
    start_date: str, end_date: str, timezone: Optional[str]
) -> Tuple[Optional[int], Optional[int]]:
    """Parse start and end dates to unix timestamps.

    Args:
        start_date: Start date string
        end_date: End date string
        timezone: Timezone name or None

    Returns:
        Tuple of (start_ts, end_ts) or (None, None) on error
    """
    try:
        start_ts = parse_date_to_unix(start_date, end_of_day=False, tz_name=timezone)
        end_ts = parse_date_to_unix(
            end_date,
            end_of_day=is_date_only(end_date),
            tz_name=timezone,
        )
        return start_ts, end_ts
    except ValueError as exc:
        message = (
            f"Invalid date range '{start_date}' -> '{end_date}': {exc}. "
            "Use YYYY-MM-DD or unix timestamps."
        )
        _print_error(message)
        return None, None


def get_model_version(config: ReportConfig) -> Optional[str]:
    """Determine model version for transit data source.

    Args:
        config: Report configuration

    Returns:
        Model version string or None
    """
    if config.query.source == "radar_data_transits":
        return config.query.model_version or "hourly-cron"
    return None


def print_api_debug_info(
    resp: object, metrics: List, histogram: Optional[dict]
) -> None:
    """Print API response debug information.

    Args:
        resp: API response object
        metrics: Metrics list
        histogram: Histogram dict or None
    """
    try:
        ms = resp.elapsed.total_seconds() * 1000.0
        print(
            f"DEBUG: API response status={resp.status_code} elapsed={ms:.1f}ms "
            f"metrics={len(metrics)} histogram_present={bool(histogram)}"
        )
    except Exception:
        print("DEBUG: unable to read response metadata")


def check_charts_available() -> bool:
    """Check if chart generation is available (matplotlib installed).

    Returns:
        True if charts can be generated
    """
    try:
        _import_chart_builder()
        _import_chart_saver()

        return True
    except ImportError:
        _print_error(
            "Charts unavailable: install matplotlib, cairo, and associated system libraries to enable chart PDFs."
        )
        return False


def generate_all_charts(
    prefix: str,
    metrics: List,
    daily_metrics: Optional[List],
    histogram: Optional[dict],
    overall_metrics: List,
    config: ReportConfig,
    resp: Optional[object],
    compare_metrics: Optional[List] = None,
    compare_histogram: Optional[dict] = None,
    primary_label: Optional[str] = None,
    compare_label: Optional[str] = None,
) -> None:
    """Generate all charts (stats, daily, histogram) if data available.

    Args:
        prefix: File prefix for outputs
        metrics: Granular metrics
        daily_metrics: Daily metrics or None
        histogram: Histogram data or None
        overall_metrics: Overall summary metrics
        config: Report configuration
        resp: API response object for debug info
    """
    if not check_charts_available():
        if config.output.debug:
            print("DEBUG: matplotlib not available, skipping charts")
        return

    # Debug output for API response
    if config.output.debug and resp:
        print_api_debug_info(resp, metrics, histogram)

    # Generate granular stats chart
    generate_timeseries_chart(
        metrics,
        f"{prefix}_stats",
        f"{prefix} - stats",
        config.query.units,
        config.query.timezone or None,
        config,
    )

    # Generate comparison granular stats chart if available
    if compare_metrics:
        generate_timeseries_chart(
            compare_metrics,
            f"{prefix}_compare_stats",
            f"{prefix} - compare stats",
            config.query.units,
            config.query.timezone or None,
            config,
        )

    # Generate daily chart if available
    if daily_metrics:
        generate_timeseries_chart(
            daily_metrics,
            f"{prefix}_daily",
            f"{prefix} - daily",
            config.query.units,
            config.query.timezone or None,
            config,
        )

    # Generate histogram if available
    if histogram:
        generate_histogram_chart(
            histogram,
            prefix,
            config.query.units,
            overall_metrics,
            config,
            compare_histogram=compare_histogram,
            primary_label=primary_label,
            compare_label=compare_label,
        )


def process_date_range(
    start_date: str, end_date: str, config: ReportConfig, client: RadarStatsClient
) -> None:
    """Process a single date range: fetch data, generate charts, create PDF.

    This is the main orchestrator that coordinates all steps for one date range.

    Args:
        start_date: Start date string
        end_date: End date string
        config: Report configuration
        client: API client instance
    """
    _print_info(
        f"=== Processing {start_date} -> {end_date} (group={config.query.group}, source={config.query.source}) ==="
    )

    # Create output directory if specified
    output_dir = config.output.output_dir or "."
    if output_dir != ".":
        os.makedirs(output_dir, exist_ok=True)
        _print_info(f"Output directory: {output_dir}")

    # Parse dates to timestamps
    start_ts, end_ts = parse_date_range(
        start_date, end_date, config.query.timezone or None
    )
    if start_ts is None or end_ts is None:
        return  # Error already printed

    compare_start_date = config.query.compare_start_date
    compare_end_date = config.query.compare_end_date
    compare_start_ts = None
    compare_end_ts = None
    compare_metrics = None
    compare_histogram = None
    compare_overall = None
    compare_daily_metrics = None
    compare_start_iso = None
    compare_end_iso = None
    compare_label = None
    config_periods: Optional[List[Dict[str, Any]]] = None
    cosine_correction_note: Optional[str] = None
    primary_label = f"{start_date} to {end_date}"

    if compare_start_date and compare_end_date:
        _print_info(
            f"Comparing against {compare_start_date} -> {compare_end_date} (same group and source)."
        )
        compare_start_ts, compare_end_ts = parse_date_range(
            compare_start_date, compare_end_date, config.query.timezone or None
        )
        if compare_start_ts is None or compare_end_ts is None:
            return

    # Determine model version and file prefix
    model_version = get_model_version(config)
    prefix = resolve_file_prefix(config, start_ts, end_ts, output_dir)

    # Prepend output directory to prefix
    prefix = os.path.join(output_dir, prefix)

    _print_info(f"Output prefix: {prefix}")
    if config.query.histogram:
        _print_info(
            f"Histogram: enabled (bucket={config.query.hist_bucket_size}, max={config.query.hist_max})"
        )
    else:
        _print_info("Histogram: disabled")

    # If debug mode is enabled, write the submitted config to the output prefix
    # so it can be included in the sources ZIP for debugging
    if config.output.debug:
        try:
            import json
            import shutil

            # Write the final merged config (with all defaults applied)
            final_config_dest = f"{prefix}_final_config.json"
            with open(final_config_dest, "w") as f:
                json.dump(config.to_dict(), f, indent=2)
            print(f"DEBUG: wrote final config to: {final_config_dest}")

            # Copy the original submitted config file (as passed from Go server)
            # The config_file path is available from the global args parsed in __main__
            # We need to pass it through - for now, check if it's in sys.argv
            submitted_config_source = None
            if len(sys.argv) > 1 and os.path.isfile(sys.argv[-1]):
                submitted_config_source = sys.argv[-1]

            if submitted_config_source:
                submitted_config_dest = f"{prefix}_submitted_config.json"
                shutil.copyfile(submitted_config_source, submitted_config_dest)
                print(f"DEBUG: wrote submitted config to: {submitted_config_dest}")
            else:
                print("DEBUG: could not determine submitted config file path")

        except Exception as e:
            print(f"DEBUG: failed to write config files: {e}")

    # Fetch all data from API
    metrics, histogram, min_speed_used, resp = fetch_granular_metrics(
        client, start_ts, end_ts, config, model_version
    )
    if not metrics and not histogram:
        _print_error(
            f"No data returned for {start_date} - {end_date}. "
            "Check the date range, min_speed filter, and data source."
        )
        return

    # Fetch overall summary from API (proper percentile calculation from raw data)
    overall_metrics = fetch_overall_summary(
        client, start_ts, end_ts, config, model_version
    )
    if not overall_metrics:
        _print_error(
            "Warning: overall metrics empty; PDF will have limited summary data."
        )

    should_daily = should_produce_daily(config.query.group)
    daily_metrics = fetch_daily_summary(client, start_ts, end_ts, config, model_version)
    if should_daily and not daily_metrics:
        _print_error("Warning: daily metrics unavailable; daily chart will be skipped.")
    elif not should_daily:
        _print_info("Daily summary skipped for high-level grouping.")

    if config.query.histogram and histogram is None:
        _print_error("Warning: histogram data unavailable; histogram chart skipped.")

    if compare_start_ts is not None and compare_end_ts is not None:
        # Use compare_source if specified, otherwise fall back to primary source
        compare_source = config.query.compare_source or config.query.source
        if compare_source != config.query.source:
            _print_info(f"Using different source for comparison: {compare_source}")
        compare_metrics, compare_histogram, _compare_min_speed, _compare_resp = (
            fetch_granular_metrics(
                client,
                compare_start_ts,
                compare_end_ts,
                config,
                model_version,
                source_override=compare_source,
            )
        )
        if not compare_metrics and not compare_histogram:
            _print_error(
                f"Warning: no comparison data returned for {compare_start_date} - {compare_end_date}."
            )

        # Fetch comparison overall and daily summaries from API
        compare_overall = fetch_overall_summary(
            client,
            compare_start_ts,
            compare_end_ts,
            config,
            model_version,
            source_override=compare_source,
        )
        if not compare_overall:
            _print_error(
                "Warning: comparison overall metrics empty; summary comparison may be limited."
            )

        if should_daily:
            compare_daily_metrics = fetch_daily_summary(
                client,
                compare_start_ts,
                compare_end_ts,
                config,
                model_version,
                source_override=compare_source,
            )

        compare_start_iso, compare_end_iso = compute_iso_timestamps(
            compare_start_ts, compare_end_ts, config.query.timezone
        )
        # Use original date strings from config (single source of truth from datepicker)
        compare_label = (
            f"t2: {config.query.compare_start_date} to {config.query.compare_end_date}"
        )

    if config.query.site_id is not None:
        config_periods = fetch_site_config_periods(
            client,
            config.query.site_id,
            start_ts,
            end_ts,
            compare_start_ts,
            compare_end_ts,
        )
        if config_periods:
            angles = {
                float(period.get("cosine_error_angle", 0))
                for period in config_periods
                if period.get("cosine_error_angle") is not None
            }
            if len(angles) > 1:
                cosine_correction_note = (
                    "Speeds have been corrected for sensor angle changes."
                )

    # Compute ISO timestamps for report
    start_iso, end_iso = compute_iso_timestamps(start_ts, end_ts, config.query.timezone)

    if compare_start_ts and compare_end_ts:
        # Use original date strings from config (single source of truth from datepicker)
        primary_label = f"t1: {config.query.start_date} to {config.query.end_date}"

    # Generate all charts
    generate_all_charts(
        prefix,
        metrics,
        daily_metrics,
        histogram,
        overall_metrics,
        config,
        resp,
        compare_metrics=compare_metrics,
        compare_histogram=compare_histogram,
        primary_label=primary_label,
        compare_label=compare_label,
    )

    # Assemble final PDF report
    report_generated = assemble_pdf_report(
        prefix,
        start_iso,
        end_iso,
        overall_metrics,
        daily_metrics,
        metrics,
        histogram,
        config,
        min_speed_used=min_speed_used,
        compare_start_iso=compare_start_iso,
        compare_end_iso=compare_end_iso,
        compare_overall_metrics=compare_overall,
        compare_histogram=compare_histogram,
        compare_granular_metrics=compare_metrics,
        compare_daily_metrics=compare_daily_metrics,
        config_periods=config_periods,
        cosine_correction_note=cosine_correction_note,
    )

    if report_generated:
        # Create sources ZIP file after successful PDF generation
        try:
            zip_path = create_sources_zip(prefix)
            _print_info(f"Created sources ZIP: {zip_path}")
        except Exception as exc:
            _print_error(f"Warning: failed to create sources ZIP: {exc}")
            # Don't fail the whole process if ZIP creation fails

        _print_info(
            f"Completed {start_date} -> {end_date}. PDF and charts use prefix '{prefix}'."
        )
    else:
        _print_error(
            f"Failed to complete report for {start_date} -> {end_date}. See errors above."
        )


# === Main Entry Point ===


def main(date_ranges: List[Tuple[str, str]], config: ReportConfig):
    """Main orchestrator: iterate over date ranges.

    Simplified to just client creation and iteration.

    Args:
        date_ranges: List of (start_date, end_date) tuples
        config: Report configuration
    """
    client = RadarStatsClient()

    _print_info(f"API endpoint: {client.api_url}")
    _print_info(
        "Query parameters: units={units}, timezone={tz}, min_speed={min_speed}".format(
            units=config.query.units,
            tz=config.query.timezone or "UTC",
            min_speed=(
                config.query.min_speed if config.query.min_speed is not None else "none"
            ),
        )
    )
    _print_info(
        f"Processing {len(date_ranges)} date range(s) with output.dir={config.output.output_dir}"
    )

    for start_date, end_date in date_ranges:
        process_date_range(start_date, end_date, config, client)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(
        description="Generate radar velocity reports from JSON configuration file. "
        "Use create_config_example.py to generate a template config file."
    )

    # Configuration file - now REQUIRED (unless --check is used)
    parser.add_argument(
        "config_file",
        nargs="?",  # Make optional when --check is used
        help="Path to JSON configuration file (required). Use create_config_example.py to generate a template.",
    )

    # Dependency check flag
    parser.add_argument(
        "--check",
        action="store_true",
        help="Check system dependencies (Python packages, LaTeX, virtual environment) and exit",
    )

    args = parser.parse_args()

    # Handle --check flag
    if args.check:
        from pdf_generator.core.dependency_checker import check_dependencies

        system_ready = check_dependencies(verbose=False)
        sys.exit(0 if system_ready else 1)

    # Validate config_file is provided when not checking
    if not args.config_file:
        parser.error("config_file is required (unless using --check)")

    # Load configuration from JSON file
    from pdf_generator.core.config_manager import load_config, ReportConfig

    # Load config file (required)
    if not os.path.exists(args.config_file):
        parser.error(f"Config file not found: {args.config_file}")

    config = load_config(config_file=args.config_file)

    # Validate configuration
    is_valid, errors = config.validate()
    if not is_valid:
        parser.error(
            "Configuration validation failed:\n" + "\n".join(f"  - {e}" for e in errors)
        )

    # Validate histogram requirements
    if config.query.histogram and config.query.hist_bucket_size is None:
        parser.error("hist_bucket_size is required in config when histogram is true")

    date_ranges = [(config.query.start_date, config.query.end_date)]
    main(date_ranges, config)
