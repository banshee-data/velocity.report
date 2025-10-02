#!/usr/bin/env python3

import argparse
import requests
import sys
from typing import List, Tuple, Union, Optional, Any, Dict
from datetime import datetime, timezone, time
from zoneinfo import ZoneInfo, ZoneInfoNotFoundError
import numpy as np
import re

# Matplotlib imports are optional at runtime; fail gracefully with guidance
try:
    import matplotlib
    import matplotlib.dates as mdates
    import matplotlib.pyplot as plt
    from matplotlib.backends.backend_pdf import PdfPages
except Exception:  # pragma: no cover - environment dependent
    matplotlib = None
    mdates = None
    plt = None
    PdfPages = None

API_URL = "http://localhost:8080/api/radar_stats"

# mirror of server supported groups (seconds)
SUPPORTED_GROUPS = {
    "15m": 15 * 60,
    "30m": 30 * 60,
    "1h": 60 * 60,
    "2h": 2 * 60 * 60,
    "3h": 3 * 60 * 60,
    "4h": 4 * 60 * 60,
    "6h": 6 * 60 * 60,
    "8h": 8 * 60 * 60,
    "12h": 12 * 60 * 60,
    "24h": 24 * 60 * 60,
}


def parse_date_to_unix(
    d: Union[str, int], end_of_day: bool = False, tz_name: Optional[str] = None
) -> int:
    """Parse a YYYY-MM-DD date, ISO datetime, or numeric timestamp and return unix seconds.

    If d is an int or numeric string, return it as int (assumed unix seconds).
    If d is YYYY-MM-DD, interpret midnight in the provided tz_name (or UTC if none).
    If d is an ISO datetime string, parse it and preserve its timezone; if it lacks
    a timezone and tz_name is provided, apply that timezone.
    """
    if isinstance(d, int):
        return d
    s = str(d).strip()
    # numeric string -> treat as unix seconds already
    if s.isdigit():
        return int(s)

    tzobj = None
    if tz_name:
        try:
            tzobj = ZoneInfo(tz_name)
        except ZoneInfoNotFoundError:
            raise ValueError(f"unknown timezone: {tz_name}")

    # Try YYYY-MM-DD first
    try:
        dt_date = datetime.strptime(s, "%Y-%m-%d")
        if end_of_day:
            dt = datetime.combine(dt_date.date(), time(23, 59, 59))
        else:
            dt = datetime.combine(dt_date.date(), time(0, 0, 0))
        # apply timezone (or UTC default)
        if tzobj is not None:
            dt = dt.replace(tzinfo=tzobj)
        else:
            dt = dt.replace(tzinfo=timezone.utc)
        return int(dt.timestamp())
    except ValueError:
        # not YYYY-MM-DD, try full ISO datetime
        pass

    # Try ISO datetime parsing (with optional trailing Z)
    try:
        iso = s
        if iso.endswith("Z"):
            iso = iso[:-1] + "+00:00"
        dt = datetime.fromisoformat(iso)
        # if naive and tz provided, apply it; else, if naive and no tz, assume UTC
        if dt.tzinfo is None:
            if tzobj is not None:
                dt = dt.replace(tzinfo=tzobj)
            else:
                dt = dt.replace(tzinfo=timezone.utc)
        return int(dt.timestamp())
    except Exception:
        raise ValueError(
            f"Invalid date format, expected YYYY-MM-DD, ISO datetime, or unix seconds: {d}"
        )


def get_stats(
    start_ts: int,
    end_ts: int,
    group: str = "1h",
    units: str = "mph",
    source: str = "radar_objects",
    timezone: Optional[str] = None,
    min_speed: Optional[float] = None,
):
    params = {
        "start": start_ts,
        "end": end_ts,
        "group": group,
        "units": units,
        "source": source,
    }
    if timezone:
        params["timezone"] = timezone
    if min_speed is not None:
        params["min_speed"] = min_speed

    # make request and return the parsed json along with response metadata
    resp = requests.get(API_URL, params=params)
    resp.raise_for_status()
    return resp.json(), resp


def _parse_server_time(t: Any) -> datetime:
    """Parse time value returned by server into a timezone-aware datetime (UTC).

    Server returns RFC3339 strings (e.g. 2024-01-01T00:00:00Z) for StartTime.
    This helper also accepts numeric unix seconds.
    """
    if isinstance(t, (int, float)):
        return datetime.fromtimestamp(float(t), tz=timezone.utc)
    if not isinstance(t, str):
        raise ValueError(f"unsupported time format: {t!r}")
    s = t.strip()
    # RFC3339 'Z' -> +00:00 for fromisoformat
    if s.endswith("Z"):
        s = s[:-1] + "+00:00"
    # Python's fromisoformat accepts offset like +00:00
    # Preserve whatever timezone offset the server included; do not force conversion to UTC.
    return datetime.fromisoformat(s)


def plot_stats_page(
    stats: List[Dict[str, Any]],
    title: str,
    units: str,
    debug: bool = False,
    expected_delta_seconds: Optional[int] = None,
) -> Any:
    """Create a matplotlib Figure for one stats series.

    Expects stats to be a list of dicts containing StartTime, P50Speed, P85Speed, P98Speed, MaxSpeed.
    """
    if matplotlib is None or plt is None:
        raise RuntimeError(
            "matplotlib is required to generate plots. Install with: pip install matplotlib"
        )

    times = []
    p50 = []
    p85 = []
    p98 = []
    mx = []
    counts = []

    for row in stats:
        # server returns StartTime as RFC3339 string
        try:
            t = _parse_server_time(
                row.get("StartTime") or row.get("start_time") or row.get("starttime")
            )
        except Exception:
            # Try other common keys or skip
            t = None
        if t is None:
            continue
        times.append(t)

        # Use None/np.nan for missing speeds so lines break
        def val(key_list):
            for k in key_list:
                if k in row and row[k] is not None:
                    return float(row[k])
            return np.nan

        p50.append(val(["P50Speed", "p50speed", "p50"]))
        p85.append(val(["P85Speed", "p85speed"]))
        p98.append(val(["P98Speed", "p98speed"]))
        mx.append(val(["MaxSpeed", "maxspeed"]))
        # count may be missing in some payloads
        try:
            counts.append(int(row.get("Count") if row.get("Count") is not None else 0))
        except Exception:
            counts.append(0)

    fig, ax = plt.subplots(figsize=(10, 4))
    if not times:
        ax.text(0.5, 0.5, "No data", ha="center", va="center")
        ax.set_title(title)
        return fig

    # Keep times as Python datetimes (they may be timezone-aware). Use numpy
    # arrays only for numeric speed values for masking and nan handling.
    times_list = times
    p50_arr = np.array(p50, dtype=float)
    p85_arr = np.array(p85, dtype=float)
    p98_arr = np.array(p98, dtype=float)
    mx_arr = np.array(mx, dtype=float)

    # precompute times as unix seconds for gap detection
    times_seconds = np.array([t.timestamp() for t in times_list], dtype=float)

    # Plot each series as contiguous valid segments. For gaps between segments,
    # draw a dashed connector between the last point before the gap and the
    # first point after the gap so the gap is visually indicated.

    def plot_segments(x_times, y_arr, label, marker, default_linestyle="-"):
        # present indices where we have numeric values
        present_idx = np.where(~np.isnan(y_arr))[0]
        if debug:
            print(
                f"DEBUG: series={label} len={len(y_arr)} present_count={present_idx.size}"
            )
            sample_mask = "".join("1" if not np.isnan(v) else "0" for v in y_arr[:200])
            print(f"DEBUG: {label} present mask (sample up to 200): {sample_mask}")
        if present_idx.size == 0:
            return

        # find contiguous runs using timestamps (detect gaps larger than expected_delta_seconds)
        runs = []
        start = present_idx[0]
        last = present_idx[0]
        for j in present_idx[1:]:
            dt = times_seconds[j] - times_seconds[last]
            if expected_delta_seconds is not None:
                # allow a small tolerance (50%) for timing jitter
                if dt <= expected_delta_seconds * 1.5:
                    last = j
                else:
                    runs.append((start, last))
                    start = j
                    last = j
            else:
                # fallback to index adjacency
                if j == last + 1:
                    last = j
                else:
                    runs.append((start, last))
                    start = j
                    last = j
        runs.append((start, last))

        if debug:
            print(f"DEBUG: {label} runs={runs}")

        color = None
        first_plot = True
        for s, e in runs:
            xs = [x_times[i] for i in range(s, e + 1)]
            ys = y_arr[s : e + 1]
            if first_plot:
                line = ax.plot(
                    xs, ys, label=label, marker=marker, linestyle=default_linestyle
                )[0]
                color = line.get_color()
                first_plot = False
            else:
                ax.plot(xs, ys, marker=marker, color=color, linestyle=default_linestyle)

        # dashed connectors between runs
        connectors = []
        for i in range(len(runs) - 1):
            s1, e1 = runs[i]
            s2, e2 = runs[i + 1]
            x_conn = [x_times[e1], x_times[s2]]
            y_conn = [y_arr[e1], y_arr[s2]]
            connectors.append((x_conn, y_conn))
            # Do not draw connectors (invisible). Keep connector info for debug only.
        if debug:
            print(
                f"DEBUG: {label} connectors={[( (c[0][0], c[0][1]), (c[1][0], c[1][1]) ) for c in connectors]}"
            )

    plot_segments(times_list, p50_arr, "P50", marker="o")
    plot_segments(times_list, p85_arr, "P85", marker="^")
    plot_segments(times_list, p98_arr, "P98", marker="s")
    plot_segments(times_list, mx_arr, "Max", marker="x", default_linestyle="--")

    # Plot counts on right axis as bars
    try:
        counts_arr = np.array(counts, dtype=float)
    except Exception:
        counts_arr = np.zeros(len(times_list), dtype=float)

    # Undercount warnings: full-height translucent orange bars behind the counts
    try:
        counts_arr = np.array(counts, dtype=float)
    except Exception:
        counts_arr = np.zeros(len(times_list), dtype=float)

    ax2 = ax.twinx()
    # determine bar width in days for matplotlib (dates are in days units)
    if expected_delta_seconds is not None:
        width_days = (expected_delta_seconds * 0.9) / (24.0 * 3600.0)
    elif len(times_seconds) > 1:
        avg_dt = np.median(np.diff(times_seconds))
        width_days = (avg_dt * 0.9) / (24.0 * 3600.0)
    else:
        width_days = (60 * 60) / (24.0 * 3600.0)  # 1 hour default in days

    # compute highlight height (top of right axis)
    try:
        if counts_arr.size > 0:
            max_c = float(np.nanmax(counts_arr))
            # make highlights go to the full top we will set for the axis (1.5x)
            highlight_h = max_c * 1.5 if max_c > 0 else 1.0
        else:
            highlight_h = 1.0
    except Exception:
        highlight_h = 1.0

    try:
        under_mask = counts_arr < 50
        if np.any(under_mask):
            ux = [times_list[i] for i in np.where(under_mask)[0]]
            uh = [highlight_h] * len(ux)
            # draw solid translucent orange highlight bars at low zorder (behind counts)
            patches = ax2.bar(
                ux,
                uh,
                width=width_days,
                align="center",
                facecolor="orange",
                edgecolor="none",
                alpha=0.2,
                zorder=0,
                label="Undercount (<50)",
            )
            if debug:
                print(
                    f"DEBUG: undercount indices={[int(i) for i in np.where(under_mask)[0]]} highlight_h={highlight_h}"
                )
    except Exception:
        pass

    # now draw the actual count bars on top
    bars = ax2.bar(
        times_list,
        counts_arr,
        width=width_days * 0.8,
        align="center",
        alpha=0.4,
        color="gray",
        label="Count",
        zorder=2,
    )
    ax2.set_ylabel("Count")

    # Ensure left axis starts at zero (speeds)
    try:
        ax.set_ylim(bottom=0)
    except Exception:
        pass

    # Set right axis top to 1.5x max count so bars don't touch the top
    try:
        if counts_arr.size > 0:
            max_c = float(np.nanmax(counts_arr))
            if max_c <= 0:
                ax2.set_ylim(top=1.0)
            else:
                ax2.set_ylim(top=max_c * 1.5)
    except Exception:
        pass

    # merge legends from both axes and place in lower-left to avoid overlap
    h1, l1 = ax.get_legend_handles_labels()
    h2, l2 = ax2.get_legend_handles_labels()
    if h1 or h2:
        ax.legend(h1 + h2, l1 + l2, loc="lower right")

    ax.set_ylabel(f"Speed ({units})")
    ax.set_title(title)
    ax.grid(True, linestyle=":", linewidth=0.5)

    # format dates
    # If the datetimes are timezone-aware, use that tz for the locator/formatter so
    # tick labels display in the server-provided timezone instead of UTC.
    tz_for_fmt = None
    if times and getattr(times[0], "tzinfo", None) is not None:
        tz_for_fmt = times[0].tzinfo

    locator = mdates.AutoDateLocator(tz=tz_for_fmt)
    formatter = mdates.ConciseDateFormatter(locator, tz=tz_for_fmt)
    ax.xaxis.set_major_locator(locator)
    ax.xaxis.set_major_formatter(formatter)
    fig.autofmt_xdate()
    return fig


def main(date_ranges: List[Tuple[str, str]], args: argparse.Namespace):
    pdf = None
    if getattr(args, "pdf", None):
        if PdfPages is None:
            print(
                "matplotlib is required to generate PDF output. Install with: pip install matplotlib"
            )
            return
        pdf = PdfPages(args.pdf)

    for start_date, end_date in date_ranges:
        # Validate and convert to unix seconds (UTC). Start => 00:00:00, End => 23:59:59
        try:
            # If the end date is a plain YYYY-MM-DD string we want the end of day
            # (23:59:59). If the user provided an ISO datetime (e.g. 2025-06-01T01:00:00Z)
            # treat it as an exact timestamp and do not force end-of-day.
            def _is_date_only(s: str) -> bool:
                try:
                    return bool(re.match(r"^\d{4}-\d{2}-\d{2}$", str(s).strip()))
                except Exception:
                    return False

            start_ts = parse_date_to_unix(
                start_date, end_of_day=False, tz_name=(args.timezone or None)
            )
            end_ts = parse_date_to_unix(
                end_date,
                end_of_day=_is_date_only(end_date),
                tz_name=(args.timezone or None),
            )
        except ValueError as e:
            print(f"Bad date range ({start_date} - {end_date}): {e}")
            continue

            # Set compress gaps flag
            data._compress_gaps = args.compress_gaps
        print(
            f"Querying stats from {start_date} ({start_ts}) to {end_date} ({end_ts})..."
        )
        if args.debug:
            # Show both UTC and the requested-local representation so the mapping
            # from YYYY-MM-DD (in requested timezone) -> unix seconds is clear.
            try:
                utc_start = datetime.fromtimestamp(start_ts, tz=timezone.utc)
                utc_end = datetime.fromtimestamp(end_ts, tz=timezone.utc)
            except Exception:
                utc_start = start_ts
                utc_end = end_ts

            # local representation in requested timezone (if provided)
            try:
                if args.timezone:
                    tz_local = ZoneInfo(args.timezone)
                else:
                    tz_local = timezone.utc
                local_start = datetime.fromtimestamp(start_ts, tz=tz_local)
                local_end = datetime.fromtimestamp(end_ts, tz=tz_local)
            except Exception:
                local_start = None
                local_end = None

            print("DEBUG: computed start_ts ->", start_ts)
            print("       UTC ->", utc_start)
            if local_start is not None:
                print("       local (", args.timezone or "UTC", ") ->", local_start)

            print("DEBUG: computed end_ts   ->", end_ts)
            print("       UTC ->", utc_end)
            if local_end is not None:
                print("       local (", args.timezone or "UTC", ") ->", local_end)
        try:
            data, resp = get_stats(
                start_ts,
                end_ts,
                group=args.group,
                units=args.units,
                source=args.source,
                timezone=args.timezone or None,
                min_speed=args.min_speed,
            )
            # no attribute-setting on returned list; pass compress flag directly to plot
        except requests.HTTPError as e:
            print(f"Request failed: {e}")
            continue

        # Print a compact log similar to server logs: timestamp [status] GET /api/..?start=..&end=..&group=..&units=..
        elapsed_ms = resp.elapsed.total_seconds() * 1000.0
        request_url = resp.request.url
        now_str = datetime.now(timezone.utc).strftime("%Y/%m/%d %H:%M:%S %Z")
        print(f"{now_str} [{resp.status_code}] GET {request_url} {elapsed_ms:.3f}ms")
        print(data)

        # If requested, generate a plot page and save to PDF
        if pdf is not None:
            tz_label = args.timezone or "UTC"
            title = f"{start_date} to {end_date} ({args.source}, group={args.group}, tz={tz_label})"
            try:
                expected_delta = SUPPORTED_GROUPS.get(args.group)
                fig = plot_stats_page(
                    data,
                    title,
                    args.units,
                    debug=args.debug,
                    expected_delta_seconds=expected_delta,
                )
                pdf.savefig(fig)
                # close the figure to free memory
                try:
                    plt.close(fig)
                except Exception:
                    pass
            except Exception as e:
                print(f"failed to generate plot for {start_date} - {end_date}: {e}")

    if pdf is not None:
        pdf.close()
        print(f"Saved PDF: {args.pdf}")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(
        description="Query radar stats API for one or more date ranges. Dates are YYYY-MM-DD or unix seconds."
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
        default="radar_objects",
        choices=["radar_objects", "radar_data_transits"],
        help="Data source to query (radar_objects or radar_data_transits). Default: radar_objects",
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
        help="Minimum speed filter (in display units). If provided, will be converted by server/client to mps. Default: none",
    )
    parser.add_argument(
        "--pdf",
        default="",
        help="Path to output PDF file. If provided, a plot page will be saved for each date range.",
    )
    parser.add_argument(
        "--debug",
        action="store_true",
        help="Print debug info about parsed datetimes and timestamps before requesting",
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

    date_ranges = [
        (args.dates[i], args.dates[i + 1]) for i in range(0, len(args.dates), 2)
    ]
    main(date_ranges, args)
