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
from stats_utils import plot_histogram, save_chart_as_pdf

# Optional matplotlib imports for plotting; keep optional so unit tests don't require it
try:
    import matplotlib
    import matplotlib.dates as mdates
    import matplotlib.pyplot as plt
except Exception:
    matplotlib = None
    mdates = None
    plt = None


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
    # Minimal plotting: times on x, speeds lines on left axis, counts on right axis
    fig, ax = plt.subplots(figsize=(24, 8))
    try:
        # Force axes to occupy nearly the full figure so saved output is tight.
        ax.set_position([0.01, 0.02, 0.98, 0.95])
    except Exception:
        pass

    if not stats:
        ax.text(0.5, 0.5, "No data", ha="center", va="center")
        ax.set_title(title)
        return fig

    times = []
    p50 = []
    p85 = []
    p98 = []
    mx = []
    counts = []

    for row in stats:
        st = row.get("StartTime") or row.get("start_time") or row.get("starttime")
        try:
            t = parse_server_time(st)
        except Exception:
            # skip rows with bad time
            continue
        # If the caller requested a timezone, convert the parsed datetime to it.
        if tz_name:
            try:
                tzobj = ZoneInfo(tz_name)
            except Exception:
                tzobj = None
            try:
                if tzobj is not None and getattr(t, "tzinfo", None) is not None:
                    t = t.astimezone(tzobj)
                elif tzobj is not None and getattr(t, "tzinfo", None) is None:
                    # parsed naive -> assume UTC then convert
                    from datetime import timezone as _dt_timezone

                    t = t.replace(tzinfo=_dt_timezone.utc).astimezone(tzobj)
            except Exception:
                # if conversion fails, keep original t
                pass
        times.append(t)

        def _num(keys):
            for k in keys:
                if k in row and row[k] is not None:
                    try:
                        return float(row[k])
                    except Exception:
                        return np.nan
            return np.nan

        p50.append(_num(["P50Speed", "p50speed", "p50"]))
        p85.append(_num(["P85Speed", "p85speed", "p85"]))
        p98.append(_num(["P98Speed", "p98speed", "p98"]))
        mx.append(_num(["MaxSpeed", "maxspeed", "max"]))
        try:
            counts.append(int(row.get("Count") if row.get("Count") is not None else 0))
        except Exception:
            counts.append(0)

    # convert to numpy arrays and mask invalid values so plotting will
    # break lines across regions with missing/null data (NaN).
    p50_a = np.ma.masked_invalid(np.array(p50, dtype=float))
    p85_a = np.ma.masked_invalid(np.array(p85, dtype=float))
    p98_a = np.ma.masked_invalid(np.array(p98, dtype=float))
    mx_a = np.ma.masked_invalid(np.array(mx, dtype=float))

    # Treat periods with zero counts as missing data for speed series so lines
    # don't connect across empty intervals. This handles APIs that return
    # numeric zeros rather than nulls for empty buckets.
    try:
        if len(counts) == len(times):
            import os

            # Configurable threshold: treat counts < threshold as missing.
            try:
                thresh = int(os.environ.get("VELOCITY_COUNT_MISSING_THRESHOLD", "5"))
            except Exception:
                thresh = 5

            zero_mask = np.array(counts) < thresh
            # Combine existing masks (if any) with zero_mask
            p50_a = np.ma.array(p50_a, mask=(np.ma.getmaskarray(p50_a) | zero_mask))
            p85_a = np.ma.array(p85_a, mask=(np.ma.getmaskarray(p85_a) | zero_mask))
            p98_a = np.ma.array(p98_a, mask=(np.ma.getmaskarray(p98_a) | zero_mask))
            mx_a = np.ma.array(mx_a, mask=(np.ma.getmaskarray(mx_a) | zero_mask))

            # Debug info
            try:
                if os.environ.get("VELOCITY_PLOT_DEBUG") == "1":
                    import sys

                    print(f"DEBUG_PLOT: missing_threshold={thresh}", file=sys.stderr)
                    print(
                        f"DEBUG_PLOT: zero_mask_count={int(zero_mask.sum())}",
                        file=sys.stderr,
                    )
            except Exception:
                pass
    except Exception:
        # If anything goes wrong here, fall back to the original masked arrays
        pass

    # Prepare plain float arrays for plotting: masked entries -> np.nan
    p50_f = np.array(p50_a.filled(np.nan), dtype=float)
    p85_f = np.array(p85_a.filled(np.nan), dtype=float)
    p98_f = np.array(p98_a.filled(np.nan), dtype=float)
    mx_f = np.array(mx_a.filled(np.nan), dtype=float)

    # Optional debug output controlled by environment variable
    try:
        import os, sys

        if os.environ.get("VELOCITY_PLOT_DEBUG") == "1":
            print("DEBUG_PLOT: times(len)=%d" % len(times), file=sys.stderr)
            print("DEBUG_PLOT: counts=%r" % (counts,), file=sys.stderr)
            print("DEBUG_PLOT: p50_f=%r" % (p50_f.tolist(),), file=sys.stderr)
    except Exception:
        pass

    # Color palette: p50 (blue), p85 (green), p98 (purple), max (red dashed)
    color_p50 = "#fbd92f"
    color_p85 = "#f7b32b"
    color_p98 = "#f25f5c"
    color_max = "#2d1e2f"

    # Helper: plot a series but break the line at masked/NaN values by
    # detecting contiguous valid segments and plotting each separately.
    def _plot_broken(ax, x, y_filled, label=None, **kwargs):
        # x: list-like of x values (python datetimes); y_filled: numpy float array with np.nan for missing
        x_arr = np.array(x, dtype=object)

        # valid mask where we have finite numeric values
        valid_mask = np.isfinite(y_filled)
        if not np.any(valid_mask):
            return

        # compute base time delta (in seconds) between consecutive points.
        # Use the minimum positive delta as the expected bucket spacing because
        # the server may omit empty buckets entirely; splitting on 2x this
        # spacing will catch omitted ranges.
        try:
            deltas = []
            for a, b in zip(x_arr[:-1], x_arr[1:]):
                if a is None or b is None:
                    continue
                try:
                    delta_s = (b - a).total_seconds()
                except Exception:
                    delta_s = float(
                        (np.datetime64(b) - np.datetime64(a)) / np.timedelta64(1, "s")
                    )
                if delta_s > 0:
                    deltas.append(delta_s)
            if deltas:
                base_delta = float(np.min(deltas))
            else:
                base_delta = None
        except Exception:
            base_delta = None

        # threshold for considering a gap "large" (2x base spacing by default)
        gap_threshold = (base_delta * 2) if base_delta is not None else None

        # Debug: report base delta and threshold
        try:
            if os.environ.get("VELOCITY_PLOT_DEBUG") == "1":
                import sys

                print(
                    f"DEBUG_PLOT: base_delta={base_delta} gap_threshold={gap_threshold}",
                    file=sys.stderr,
                )
        except Exception:
            pass

        # Build indices of valid points, but we'll also break runs when we see a large time gap
        idx = np.where(valid_mask)[0]
        runs = []
        start = idx[0]
        prev = start
        for i in idx[1:]:
            split_on_gap = False
            if gap_threshold is not None:
                try:
                    # compute time gap between prev and i
                    try:
                        gap_s = (x_arr[i] - x_arr[prev]).total_seconds()
                    except Exception:
                        gap_s = float(
                            (np.datetime64(x_arr[i]) - np.datetime64(x_arr[prev]))
                            / np.timedelta64(1, "s")
                        )
                    if gap_s > gap_threshold:
                        split_on_gap = True
                except Exception:
                    split_on_gap = False

            if i != prev + 1 or split_on_gap:
                runs.append((start, prev))
                start = i
            prev = i
        runs.append((start, prev))

        plotted_any = False
        for s, e in runs:
            x_seg = x_arr[s : e + 1]
            y_seg = y_filled[s : e + 1]
            seg_label = label if not plotted_any else None
            ax.plot(x_seg, y_seg, label=seg_label, **kwargs)
            plotted_any = True

    # Use smaller linewidth and marker size to 'zoom out' visually
    _plot_broken(
        ax,
        times,
        p50_f,
        label="p50",
        marker="^",
        color=color_p50,
        linewidth=1.0,
        markersize=4,
        markeredgewidth=0.4,
    )
    _plot_broken(
        ax,
        times,
        p85_f,
        label="p85",
        marker="s",
        color=color_p85,
        linewidth=1.0,
        markersize=4,
        markeredgewidth=0.4,
    )
    _plot_broken(
        ax,
        times,
        p98_f,
        label="p98",
        marker="o",
        color=color_p98,
        linewidth=1.0,
        markersize=4,
        markeredgewidth=0.4,
    )
    _plot_broken(
        ax,
        times,
        mx_f,
        label="Max",
        marker="x",
        linestyle="--",
        color=color_max,
        linewidth=1.0,
        markersize=4,
        markeredgewidth=0.4,
    )

    # Axis label with smaller font for compact appearance
    ax.set_ylabel(f"Velocity ({units})", fontsize=10)
    # Reduce tick label sizes for both axes so the chart appears visually smaller
    try:
        ax.tick_params(axis="both", which="major", labelsize=8)
    except Exception:
        pass

    # Ensure speed axis includes zero at the bottom for clarity
    try:
        ax.set_ylim(bottom=0)
    except Exception:
        # Some matplotlib versions may not support set_ylim with keyword args
        try:
            ymin, ymax = ax.get_ylim()
            ax.set_ylim(0, ymax)
        except Exception:
            pass

    ax2 = ax.twinx()
    # Draw orange full-height background bars behind the count bars for low-sample periods
    try:
        max_count = max(int(c) for c in counts) if counts else 0
    except Exception:
        max_count = 0

    # Positions with low counts (<50) will get an orange background bar reaching to max_count
    try:
        low_mask = [(c is not None and int(c) < 50) for c in counts]
    except Exception:
        low_mask = [False for _ in counts]

    # Orange background bars (full-height highlight), behind other bars
    # Compute top height first (increase by 60%) and use that as the
    # orange highlight height so the orange bar reaches the top of the axis.
    try:
        top = max(1, int(max_count * 1.6))
    except Exception:
        top = max_count if max_count > 0 else 1

    orange_heights = [top if m else 0 for m in low_mask]
    # Track a legend entry for the low-sample highlight so we can add a
    # correctly-colored Patch to the merged legend later. We don't create
    # a dummy bar (which can be rendered inconsistently); instead we store
    # the color/alpha/label and create a Patch at legend time.
    low_sample_legend = None
    # Determine a sensible bar width based on the spacing of the time buckets
    # so that when there are few buckets bars appear wider, and when dense
    # they shrink appropriately. Widths are expressed in Matplotlib date
    # units (days) when times are datetimes.
    try:
        if mdates is not None and len(times) > 1:
            x_nums = mdates.date2num(times)
            deltas = np.diff(x_nums)
            # use the minimum positive delta as the expected spacing
            pos = deltas[deltas > 0]
            base = (
                float(np.min(pos))
                if pos.size > 0
                else float(np.min(deltas) if deltas.size > 0 else 1.0)
            )
        else:
            # Fallback: derive spacing from seconds differences and convert to days
            x_arr = np.array(times, dtype=object)
            deltas_s = []
            for a, b in zip(x_arr[:-1], x_arr[1:]):
                try:
                    ds = (b - a).total_seconds()
                except Exception:
                    try:
                        ds = float(
                            (np.datetime64(b) - np.datetime64(a))
                            / np.timedelta64(1, "s")
                        )
                    except Exception:
                        ds = 86400.0
                if ds > 0:
                    deltas_s.append(ds)
            base = (float(np.min(deltas_s)) / 86400.0) if deltas_s else 1.0
    except Exception:
        base = 1.0

    # Choose bar widths as a fraction of the bucket spacing. Use a slightly
    # larger width for the orange background and a slightly smaller width
    # for the visible count bars so the background peeks around them.
    bar_width_bg = base * 0.95
    bar_width = base * 0.7

    if any(orange_heights) and top > 0:
        ax2.bar(
            times,
            orange_heights,
            width=bar_width_bg,
            alpha=0.25,
            color="#f7b32b",
            zorder=0,
        )
        # store legend data (threshold text kept in sync with low_mask logic)
        try:
            low_sample_legend = ("Low-sample (<50)", "#f7b32b", 0.25)
        except Exception:
            low_sample_legend = None

    # Primary count bars (always gray) drawn on top
    ax2.bar(
        times,
        counts,
        width=bar_width,
        alpha=0.25,
        color="#2d1e2f",
        label="Count",
        zorder=1,
    )

    # Set the right-axis y-limit so orange highlights reach the top
    try:
        ax2.set_ylim(0, top)
    except Exception:
        try:
            ymin, ymax = ax2.get_ylim()
            ax2.set_ylim(0, ymax * 1.4 if ymax > 0 else 1)
        except Exception:
            pass

    ax2.set_ylabel("Count")

    # merge legends
    h1, l1 = ax.get_legend_handles_labels()
    h2, l2 = ax2.get_legend_handles_labels()
    if h1 or h2:
        handles = h1 + h2
        labels = l1 + l2
        # If we previously recorded a low-sample legend entry, append a
        # Patch so the legend shows the correct orange color.
        try:
            if "low_sample_legend" in locals() and low_sample_legend is not None:
                try:
                    from matplotlib.patches import Patch

                    _lbl, _col, _alp = low_sample_legend
                    patch = Patch(facecolor=_col, alpha=_alp, edgecolor="none")
                    handles.append(patch)
                    labels.append(_lbl)
                except Exception:
                    # if Patch isn't available, fall back to nothing
                    pass
        except Exception:
            pass
        try:
            # Place a figure-level legend to the right of the axes so it's
            # positioned in figure coordinates and will not overlap the axes
            # when the axes are shrunk with fig.subplots_adjust(..., right=...).
            # Make the legend horizontal and centered below the chart.
            # Force a single row by using one column per label.
            ncols = len(labels) if labels else 1
            # Reduce font size slightly to help long labels fit in one row.
            leg = fig.legend(
                handles,
                labels,
                loc="lower center",
                bbox_to_anchor=(0.5, -0.12),
                ncol=ncols,
                framealpha=0.9,
                prop={"size": 7},
            )
            try:
                fr = leg.get_frame()
                fr.set_facecolor("white")
                fr.set_alpha(0.9)
                # Ensure the legend frame is drawn above the bars
                try:
                    leg.set_zorder(10)
                    fr.set_zorder(11)
                except Exception:
                    pass
                # Make the frame edge visible so legend text isn't swamped
                try:
                    fr.set_edgecolor("#000000")
                    fr.set_linewidth(1)
                except Exception:
                    pass
            except Exception:
                pass
        except Exception:
            # fallback to default legend if something goes wrong
            try:
                ax.legend(handles, labels, loc="lower right")
            except Exception:
                pass

    try:
        if mdates is not None:
            locator = mdates.AutoDateLocator()
            # Respect the requested timezone when formatting dates. If tz_name
            # is provided and valid, pass it to the formatter so ticks display
            # in the expected local time.
            try:
                if tz_name:
                    tzobj = ZoneInfo(tz_name)
                else:
                    tzobj = None
            except Exception:
                tzobj = None

            try:
                formatter = mdates.ConciseDateFormatter(locator, tz=tzobj)
            except TypeError:
                # Older matplotlib may not accept tz kwarg; fall back.
                formatter = mdates.ConciseDateFormatter(locator)
            ax.xaxis.set_major_locator(locator)
            ax.xaxis.set_major_formatter(formatter)
            fig.autofmt_xdate()
            # Hide the small offset/date annotation (often shown at lower-right)
            try:
                ax.xaxis.get_offset_text().set_visible(False)
            except Exception:
                try:
                    ax.xaxis.set_offset_position("none")
                except Exception:
                    pass
    except Exception:
        pass

    # Reduce axis font sizes (ticks and axis labels) so the plot is compact.
    try:
        # tick labels (x and y)
        ax.tick_params(axis="both", which="major", labelsize=7)
        # right-hand count axis
        try:
            ax2.tick_params(axis="both", which="major", labelsize=7)
        except Exception:
            pass
        # axis label sizes
        try:
            ax.yaxis.label.set_size(8)
        except Exception:
            pass
        try:
            ax.xaxis.label.set_size(8)
        except Exception:
            pass
        try:
            # make the title slightly smaller if present
            ax.title.set_size(10)
        except Exception:
            pass
    except Exception:
        pass

    # Reduce whitespace around the axes so exported PDFs have minimal borders
    try:
        # force tight layout with zero padding. If a figure-level legend
        # (leg) was created above, tell tight_layout about it so it will
        # reserve space and not shrink the axes under the legend.
        if "leg" in locals() and leg is not None:
            try:
                fig.tight_layout(pad=0, bbox_extra_artists=[leg])
            except TypeError:
                # Older matplotlib versions may not support bbox_extra_artists
                fig.tight_layout(pad=0)
        else:
            fig.tight_layout(pad=0)
    except Exception:
        pass
    try:
        # Remove the top gap and make room on the right for the stacked legend.
        # Right is reduced so the legend can sit outside the axes without overlap.
        # Shrink the right edge so the plotting area moves left and avoids
        # overlapping the stacked legend placed to the right of the axes.
        fig.subplots_adjust(left=0.02, right=0.96, top=0.995, bottom=0.16)
    except Exception:
        pass
    # also reduce tick sizes for the count axis
    try:
        ax2.tick_params(axis="both", which="major", labelsize=7)
    except Exception:
        pass

    return fig


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
        location = "Clarendon Avenue, San Francisco"  # TODO: make this configurable

        # Plotting block: generate charts and histograms first so they can be embedded into the PDF
        if matplotlib is None:
            if getattr(args, "debug", False):
                print("DEBUG: matplotlib not available, skipping charts")
        else:
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
                    sample_n = None
                    try:
                        if hasattr(metrics_all, "get"):
                            sample_n = metrics_all.get("Count") or metrics_all.get(
                                "count"
                            )
                        elif isinstance(metrics_all, (list, tuple)) and metrics_all:
                            first = metrics_all[0]
                            if isinstance(first, dict):
                                sample_n = first.get("Count") or first.get("count")
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
                speed_limit=25,  # TODO: make this configurable
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
