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
from typing import List, Tuple, Optional
from datetime import datetime, timezone
from zoneinfo import ZoneInfo

import numpy as np

from api_client import RadarStatsClient, SUPPORTED_GROUPS
from date_parser import parse_date_to_unix, is_date_only, parse_server_time
from pdf_generator import generate_pdf_report
from stats_utils import plot_histogram
from chart_builder import TimeSeriesChartBuilder
from chart_saver import save_chart_as_pdf
from report_config import COLORS, FONTS, LAYOUT, SITE_INFO, DEBUG
from data_transformers import (
    MetricsNormalizer,
    extract_start_time_from_row,
    extract_count_from_row,
)


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
    """Return a sequenced prefix like base-1, base-2, ... based on files in CWD.

    This scans the current directory for files beginning with ``base-<n>`` and
    returns the next number in the sequence. Always returns base-<n> (start at 1).
    """
    files = os.listdir(".")
    pat = re.compile(r"^" + re.escape(base) + r"-(\d+)(?:_|$)")
    nums = []
    for fn in files:
        m = pat.match(fn)
        if m:
            try:
                nums.append(int(m.group(1)))
            except Exception:
                continue
    next_n = max(nums) + 1 if nums else 1
    return f"{base}-{next_n}"


def _plot_stats_page(stats, title: str, units: str, tz_name: Optional[str] = None):
    """Create a compact time-series plot (P50/P85/P98/Max + counts bars).

    Returns a matplotlib Figure.
    """
    builder = TimeSeriesChartBuilder()
    return builder.build(stats, title, units, tz_name)


def main(date_ranges: List[Tuple[str, str]], args: argparse.Namespace):
    client = RadarStatsClient()

    for start_date, end_date in date_ranges:
        model_version = None
        if getattr(args, "source", "") == "radar_data_transits":
            model_version = args.model_version or "rebuild-full"

        try:
            start_ts = parse_date_to_unix(
                start_date, end_of_day=False, tz_name=(args.timezone or None)
            )
            end_ts = parse_date_to_unix(
                end_date,
                end_of_day=is_date_only(end_date),
                tz_name=(args.timezone or None),
            )
        except ValueError as e:
            print(f"Bad date range ({start_date} - {end_date}): {e}")
            continue

        # determine file prefix; if the user provided a prefix, create a numbered
        # sequence to avoid clobbering previous runs (out -> out-1, out-2, ...)
        if args.file_prefix:
            prefix_base = args.file_prefix
            prefix = _next_sequenced_prefix(prefix_base)
        else:
            tzobj = ZoneInfo(args.timezone) if args.timezone else timezone.utc
            start_label = datetime.fromtimestamp(start_ts, tz=tzobj).date().isoformat()
            end_label = datetime.fromtimestamp(end_ts, tz=tzobj).date().isoformat()
            prefix = f"{args.source}_{start_label}_to_{end_label}"

        # main granular query
        try:
            metrics, histogram, resp = client.get_stats(
                start_ts=start_ts,
                end_ts=end_ts,
                group=args.group,
                units=args.units,
                source=args.source,
                model_version=model_version,
                timezone=args.timezone or None,
                min_speed=args.min_speed,
                compute_histogram=args.histogram,
                hist_bucket_size=args.hist_bucket_size,
                hist_max=args.hist_max,
            )
        except Exception as e:
            print(f"Request failed for {start_date} - {end_date}: {e}")
            continue

        # overall summary
        try:
            metrics_all, _, _ = client.get_stats(
                start_ts=start_ts,
                end_ts=end_ts,
                group="all",
                units=args.units,
                source=args.source,
                model_version=model_version,
                timezone=args.timezone or None,
                min_speed=args.min_speed,
                compute_histogram=False,
            )
        except Exception as e:
            print(f"Failed to fetch overall summary: {e}")
            metrics_all = []

        # daily summary (optional)
        daily_metrics = None
        if should_produce_daily(args.group):
            try:
                daily_metrics, _, _ = client.get_stats(
                    start_ts=start_ts,
                    end_ts=end_ts,
                    group="24h",
                    units=args.units,
                    source=args.source,
                    model_version=model_version,
                    timezone=args.timezone or None,
                    min_speed=args.min_speed,
                    compute_histogram=False,
                )
            except Exception as e:
                print(f"Failed to fetch daily summary: {e}")
                daily_metrics = None

        # compute ISO strings for generation parameters
        try:
            tzobj = ZoneInfo(args.timezone) if args.timezone else timezone.utc
            start_iso = datetime.fromtimestamp(start_ts, tz=tzobj).isoformat()
            end_iso = datetime.fromtimestamp(end_ts, tz=tzobj).isoformat()
        except Exception:
            start_iso = str(start_date)
            end_iso = str(end_date)

        min_speed_str = (
            f"{args.min_speed} {args.units}" if args.min_speed is not None else "none"
        )
        tz_display = args.timezone or "UTC"

        # Prepare PDF output path and location
        pdf_path = f"{prefix}_report.pdf"
        location = SITE_INFO["location"]

        # Plotting block: generate charts and histograms first so they can be embedded into the PDF
        try:
            # Import check - TimeSeriesChartBuilder will raise ImportError if matplotlib not available
            from chart_builder import TimeSeriesChartBuilder

            charts_available = True
        except ImportError:
            charts_available = False
            if getattr(args, "debug", False):
                print("DEBUG: matplotlib not available, skipping charts")

        if charts_available:
            # Report response metadata in debug mode
            if getattr(args, "debug", False):
                try:
                    ms = resp.elapsed.total_seconds() * 1000.0
                    print(
                        f"DEBUG: API response status={resp.status_code} elapsed={ms:.1f}ms metrics={len(metrics)} histogram_present={bool(histogram)}"
                    )
                except Exception:
                    print("DEBUG: unable to read response metadata")

            # granular stats PDF
            try:
                fig = _plot_stats_page(
                    metrics,
                    f"{prefix} - stats",
                    args.units,
                    tz_name=(args.timezone or None),
                )
                stats_pdf = f"{prefix}_stats.pdf"
                if save_chart_as_pdf(fig, stats_pdf):
                    print(f"Wrote stats PDF: {stats_pdf}")
                else:
                    print("Failed to save stats PDF")
            except Exception as e:
                if getattr(args, "debug", False):
                    print(f"DEBUG: failed to generate stats PDF: {e}")
                else:
                    print("Failed to generate stats PDF")

            # daily PDF
            if daily_metrics:
                try:
                    figd = _plot_stats_page(
                        daily_metrics,
                        f"{prefix} - daily",
                        args.units,
                        tz_name=(args.timezone or None),
                    )
                    daily_pdf = f"{prefix}_daily.pdf"
                    if save_chart_as_pdf(figd, daily_pdf):
                        print(f"Wrote daily PDF: {daily_pdf}")
                    else:
                        print("Failed to save daily PDF")
                except Exception as e:
                    if getattr(args, "debug", False):
                        print(f"DEBUG: failed to generate daily PDF: {e}")
                    else:
                        print("Failed to generate daily PDF")

            # # overall PDF
            # if metrics_all:
            #     try:
            #         fig_all = _plot_stats_page(
            #             metrics_all, f"{prefix} - overall", args.units
            #         )
            #         overall_pdf_path = f"{prefix}_overall.pdf"
            #         if save_chart_as_pdf(fig_all, overall_pdf_path):
            #             print(f"Wrote overall PDF: {overall_pdf_path}")
            #         else:
            #             print("Failed to save overall PDF")
            #     except Exception as e:
            #         if getattr(args, "debug", False):
            #             print(f"DEBUG: failed to generate overall PDF: {e}")
            #         else:
            #             print("Failed to generate overall PDF")

            # histogram PDF if present
            if histogram:
                try:
                    # include sample size from overall metrics if available
                    # Use normalizer for consistent field extraction
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
                        args.units,
                        debug=getattr(args, "debug", False),
                    )
                    hist_pdf = f"{prefix}_histogram.pdf"
                    if save_chart_as_pdf(fig_hist, hist_pdf):
                        print(f"Wrote histogram PDF: {hist_pdf}")
                    else:
                        print("Failed to save histogram PDF")
                except ImportError as ie:
                    if getattr(args, "debug", False):
                        print(f"DEBUG: histogram plotting unavailable: {ie}")
                    else:
                        print("Histogram plotting unavailable")
                except Exception as e:
                    if getattr(args, "debug", False):
                        print(f"DEBUG: failed to generate histogram PDF: {e}")
                    else:
                        print("Failed to generate histogram PDF")

        # Generate PDF report (charts should now exist on disk to be embedded)
        try:
            generate_pdf_report(
                output_path=pdf_path,
                start_iso=start_iso,
                end_iso=end_iso,
                group=args.group,
                units=args.units,
                timezone_display=tz_display,
                min_speed_str=min_speed_str,
                location=location,
                overall_metrics=metrics_all,
                daily_metrics=daily_metrics,
                granular_metrics=metrics,
                histogram=histogram,
                tz_name=(args.timezone or None),
                charts_prefix=prefix,
                speed_limit=SITE_INFO["speed_limit"],
                hist_max=getattr(args, "hist_max", None),
            )
            print(f"Generated PDF report: {pdf_path}")
        except Exception as e:
            print(f"Failed to generate PDF report: {e}")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(
        description="Query radar stats API for date ranges and generate LaTeX table files."
    )
    parser.add_argument(
        "--group",
        default="1h",
        help="Grouping to request from server (15m, 30m, 1h, 2h, ...). Default: 1h",
    )
    parser.add_argument(
        "--units",
        default="mph",
        help="Display units to request (e.g. mph, kph). Default: mph",
    )
    parser.add_argument(
        "--source",
        default="radar_data_transits",
        choices=["radar_objects", "radar_data_transits"],
        help="Data source to query (radar_objects or radar_data_transits).",
    )
    parser.add_argument(
        "--model-version",
        default="rebuild-full",
        help="Transit model version to query when --source=radar_data_transits. Default: rebuild-full",
    )
    parser.add_argument(
        "--timezone",
        default="",
        help="Timezone to request for StartTime conversion (e.g. UTC, America/Los_Angeles). Default: server default",
    )
    parser.add_argument(
        "--min-speed",
        type=float,
        default=None,
        help="Minimum speed filter (in display units). Default: none",
    )
    # legacy alias accepted by older scripts
    parser.add_argument(
        "--min_speed",
        dest="min_speed",
        type=float,
        help=argparse.SUPPRESS,
    )
    parser.add_argument(
        "--file-prefix",
        default="",
        help="File prefix for generated outputs. If not provided, defaults to '{source}_{start}_to_{end}'.",
    )
    parser.add_argument(
        "--histogram",
        action="store_true",
        help="Request histogram data from the server and include histogram in response.",
    )
    parser.add_argument(
        "--hist-bucket-size",
        type=float,
        default=None,
        help="Histogram bucket size in display units (required if --histogram is used)",
    )
    parser.add_argument(
        "--hist-max",
        type=float,
        default=None,
        help="Maximum speed for histogram (optional)",
    )
    parser.add_argument(
        "--debug",
        action="store_true",
        help="Print debug info about parsed datetimes, timestamps, and API responses",
    )
    parser.add_argument(
        "dates",
        nargs="+",
        help="Pairs of start end dates. Each date may be YYYY-MM-DD or unix seconds. Example: 2024-01-01 2024-01-31",
    )

    args = parser.parse_args()
    if not args.dates or len(args.dates) % 2 != 0:
        parser.error(
            "You must provide an even number of date arguments: <start1> <end1> [<start2> <end2> ...]"
        )
    if args.histogram and args.hist_bucket_size is None:
        parser.error("--hist-bucket-size is required when --histogram is used")

    date_ranges = [
        (args.dates[i], args.dates[i + 1]) for i in range(0, len(args.dates), 2)
    ]
    main(date_ranges, args)
