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
from typing import List, Tuple, Optional
from datetime import datetime, timezone as dt_timezone
from zoneinfo import ZoneInfo

import numpy as np

from pdf_generator.core.api_client import RadarStatsClient, SUPPORTED_GROUPS
from pdf_generator.core.config_manager import ReportConfig
from pdf_generator.core.date_parser import parse_date_to_unix, is_date_only, parse_server_time
from pdf_generator.core.pdf_generator import generate_pdf_report
from pdf_generator.core.stats_utils import plot_histogram
from pdf_generator.core.data_transformers import (
    MetricsNormalizer,
    extract_start_time_from_row,
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
        from pdf_generator.core.chart_builder import TimeSeriesChartBuilder as _TimeSeriesChartBuilder
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
        from pdf_generator.core.chart_saver import save_chart_as_pdf as _save_chart_as_pdf
    except ImportError as exc:  # pragma: no cover - optional dependency
        raise ImportError("chart_saver module unavailable") from exc

    save_chart_as_pdf = _save_chart_as_pdf  # type: ignore[assignment]
    return save_chart_as_pdf


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


def _next_sequenced_prefix(base: str) -> str:
    """Return a sequenced prefix like base-1-HHMMSS, base-2-HHMMSS, ... based on files in CWD.

    This scans the current directory for files beginning with ``base-<n>`` and
    returns the next number in the sequence with a timestamp suffix.
    Always returns base-<n>-HHMMSS (start at 1).
    The timestamp helps avoid caching issues with PDF viewers.
    """
    files = os.listdir(".")
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


def resolve_file_prefix(config: ReportConfig, start_ts: int, end_ts: int) -> str:
    """Determine output file prefix (sequenced or date-based).

    Args:
        config: Report configuration
        start_ts: Start timestamp
        end_ts: End timestamp

    Returns:
        File prefix string
    """
    if config.output.file_prefix:
        # User provided a prefix - create numbered sequence
        return _next_sequenced_prefix(config.output.file_prefix)
    else:
        # Auto-generate from date range
        tzobj = (
            ZoneInfo(config.query.timezone)
            if config.query.timezone
            else dt_timezone.utc
        )
        start_label = datetime.fromtimestamp(start_ts, tz=tzobj).date().isoformat()
        end_label = datetime.fromtimestamp(end_ts, tz=tzobj).date().isoformat()
        return f"{config.query.source}_{start_label}_to_{end_label}"


# === API Data Fetching ===


def fetch_granular_metrics(
    client: RadarStatsClient,
    start_ts: int,
    end_ts: int,
    config: ReportConfig,
    model_version: Optional[str],
) -> Tuple[List, Optional[dict], Optional[object]]:
    """Fetch main granular metrics and optional histogram.

    Args:
        client: API client instance
        start_ts: Start timestamp
        end_ts: End timestamp
        config: Report configuration
        model_version: Model version for transit data

    Returns:
        Tuple of (metrics, histogram, response_metadata)
    """
    try:
        metrics, histogram, resp = client.get_stats(
            start_ts=start_ts,
            end_ts=end_ts,
            group=config.query.group,
            units=config.query.units,
            source=config.query.source,
            model_version=model_version,
            timezone=config.query.timezone or None,
            min_speed=config.query.min_speed,
            compute_histogram=config.query.histogram,
            hist_bucket_size=config.query.hist_bucket_size,
            hist_max=config.query.hist_max,
        )
        return metrics, histogram, resp
    except Exception as e:
        print(f"Request failed: {e}")
        return [], None, None


def fetch_overall_summary(
    client: RadarStatsClient,
    start_ts: int,
    end_ts: int,
    config: ReportConfig,
    model_version: Optional[str],
) -> List:
    """Fetch overall 'all' group summary.

    Args:
        client: API client instance
        start_ts: Start timestamp
        end_ts: End timestamp
        config: Report configuration
        model_version: Model version for transit data

    Returns:
        List of overall metrics (empty list on failure)
    """
    try:
        metrics_all, _, _ = client.get_stats(
            start_ts=start_ts,
            end_ts=end_ts,
            group="all",
            units=config.query.units,
            source=config.query.source,
            model_version=model_version,
            timezone=config.query.timezone or None,
            min_speed=config.query.min_speed,
            compute_histogram=False,
        )
        return metrics_all
    except Exception as e:
        print(f"Failed to fetch overall summary: {e}")
        return []


def fetch_daily_summary(
    client: RadarStatsClient,
    start_ts: int,
    end_ts: int,
    config: ReportConfig,
    model_version: Optional[str],
) -> Optional[List]:
    """Fetch daily (24h) summary if appropriate for group size.

    Args:
        client: API client instance
        start_ts: Start timestamp
        end_ts: End timestamp
        config: Report configuration
        model_version: Model version for transit data

    Returns:
        List of daily metrics or None if not needed/failed
    """
    if not should_produce_daily(config.query.group):
        return None

    try:
        daily_metrics, _, _ = client.get_stats(
            start_ts=start_ts,
            end_ts=end_ts,
            group="24h",
            units=config.query.units,
            source=config.query.source,
            model_version=model_version,
            timezone=config.query.timezone or None,
            min_speed=config.query.min_speed,
            compute_histogram=False,
        )
        return daily_metrics
    except Exception as e:
        print(f"Failed to fetch daily summary: {e}")
        return None


# === Chart Generation ===


def generate_histogram_chart(
    histogram: dict,
    prefix: str,
    units: str,
    metrics_all: List,
    config: ReportConfig,
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
            print("Failed to save histogram PDF")
            return False
    except ImportError as ie:
        if config.output.debug:
            print(f"DEBUG: histogram plotting unavailable: {ie}")
        else:
            print("Histogram plotting unavailable")
        return False
    except Exception as e:
        if config.output.debug:
            print(f"DEBUG: failed to generate histogram PDF: {e}")
        else:
            print("Failed to generate histogram PDF")
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
            print(f"Failed to save {title} PDF")
            return False
    except Exception as e:
        if config.output.debug:
            print(f"DEBUG: failed to generate {title} PDF: {e}")
        else:
            print(f"Failed to generate {title} PDF")
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
    min_speed_str = (
        f"{config.query.min_speed} {config.query.units}"
        if config.query.min_speed is not None
        else "none"
    )
    tz_display = config.query.timezone or "UTC"
    pdf_path = f"{prefix}_report.pdf"

    try:
        generate_pdf_report(
            output_path=pdf_path,
            start_iso=start_iso,
            end_iso=end_iso,
            group=config.query.group,
            units=config.query.units,
            timezone_display=tz_display,
            min_speed_str=min_speed_str,
            location=config.site.location,
            overall_metrics=overall_metrics,
            daily_metrics=daily_metrics,
            granular_metrics=granular_metrics,
            histogram=histogram,
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
        )
        print(f"Generated PDF report: {pdf_path}")
        return True
    except Exception as e:
        print(f"Failed to generate PDF report: {e}")
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
    except ValueError as e:
        print(f"Bad date range ({start_date} - {end_date}): {e}")
        return None, None


def get_model_version(config: ReportConfig) -> Optional[str]:
    """Determine model version for transit data source.

    Args:
        config: Report configuration

    Returns:
        Model version string or None
    """
    if config.query.source == "radar_data_transits":
        return config.query.model_version or "rebuild-full"
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
        return False


def generate_all_charts(
    prefix: str,
    metrics: List,
    daily_metrics: Optional[List],
    histogram: Optional[dict],
    overall_metrics: List,
    config: ReportConfig,
    resp: Optional[object],
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
            histogram, prefix, config.query.units, overall_metrics, config
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
    # Parse dates to timestamps
    start_ts, end_ts = parse_date_range(
        start_date, end_date, config.query.timezone or None
    )
    if start_ts is None or end_ts is None:
        return  # Error already printed

    # Determine model version and file prefix
    model_version = get_model_version(config)
    prefix = resolve_file_prefix(config, start_ts, end_ts)

    # Fetch all data from API
    metrics, histogram, resp = fetch_granular_metrics(
        client, start_ts, end_ts, config, model_version
    )
    if not metrics and not histogram:
        print(f"No data returned for {start_date} - {end_date}")
        return

    overall_metrics = fetch_overall_summary(
        client, start_ts, end_ts, config, model_version
    )
    daily_metrics = fetch_daily_summary(client, start_ts, end_ts, config, model_version)

    # Compute ISO timestamps for report
    start_iso, end_iso = compute_iso_timestamps(start_ts, end_ts, config.query.timezone)

    # Generate all charts
    generate_all_charts(
        prefix, metrics, daily_metrics, histogram, overall_metrics, config, resp
    )

    # Assemble final PDF report
    assemble_pdf_report(
        prefix,
        start_iso,
        end_iso,
        overall_metrics,
        daily_metrics,
        metrics,
        histogram,
        config,
    )


# === Main Entry Point ===


def process_date_ranges(date_ranges: List[Tuple[str, str]], config: ReportConfig):
    """Main orchestrator: iterate over date ranges.

    Simplified to just client creation and iteration.

    Args:
        date_ranges: List of (start_date, end_date) tuples
        config: Report configuration
    """
    client = RadarStatsClient()

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
            f"Configuration validation failed:\n"
            + "\n".join(f"  - {e}" for e in errors)
        )

    # Validate histogram requirements
    if config.query.histogram and config.query.hist_bucket_size is None:
        parser.error("hist_bucket_size is required in config when histogram is true")

    date_ranges = [(config.query.start_date, config.query.end_date)]
    main(date_ranges, config)
