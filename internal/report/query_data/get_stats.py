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

        # allow explicit color mapping per series
        color_map = {
            "P50": "#ece111",
            "P85": "#ed7648",
            "P98": "#d50734",
            "Max": "black",
        }
        color = None
        color_override = color_map.get(label)
        first_plot = True
        for s, e in runs:
            xs = [x_times[i] for i in range(s, e + 1)]
            ys = y_arr[s : e + 1]
            if first_plot:
                if color_override is not None:
                    line = ax.plot(
                        xs,
                        ys,
                        label=label,
                        marker=marker,
                        linestyle=default_linestyle,
                        color=color_override,
                    )[0]
                else:
                    line = ax.plot(
                        xs, ys, label=label, marker=marker, linestyle=default_linestyle
                    )[0]
                color = line.get_color()
                first_plot = False
            else:
                ax.plot(
                    xs,
                    ys,
                    marker=marker,
                    color=(color_override or color),
                    linestyle=default_linestyle,
                )

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

    plot_segments(times_list, p50_arr, "P50", marker="s")
    plot_segments(times_list, p85_arr, "P85", marker="^")
    plot_segments(times_list, p98_arr, "P98", marker="o")
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
    # Place the twin axis underneath the main axis so an axes-level legend
    # attached to `ax` will be drawn above the bar artists on `ax2`.
    try:
        ax2.set_zorder(1)
        ax.set_zorder(2)
        # Make the main axis patch invisible so `ax2` remains visible beneath it
        ax.patch.set_visible(False)
    except Exception:
        pass
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
                zorder=1,
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

    # merge legends from both axes and place inside the plot area
    h1, l1 = ax.get_legend_handles_labels()
    h2, l2 = ax2.get_legend_handles_labels()
    if h1 or h2:
        try:
            # Use an axes-level legend so it sits inside the plot (lower right)
            leg = ax.legend(h1 + h2, l1 + l2, loc="lower right", framealpha=0.85)
            # ensure legend draws above plot artists
            try:
                leg.set_zorder(10)
            except Exception:
                pass
        except Exception:
            pass

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
    # Hide the automatic offset/date shown at the lower-left of the axis (we'll
    # show a simplified date range in the title instead).
    try:
        ax.xaxis.get_offset_text().set_visible(False)
    except Exception:
        pass
    fig.autofmt_xdate()
    # Reduce page margins for inclusion in PDFs: keep labels readable but trim left/right whitespace.
    try:
        # Apply tight layout first to respect artist extents, with a small padding.
        fig.tight_layout(pad=0.3)
    except Exception:
        pass
    try:
        # Nudge left/right margins as small as practical while leaving room for ticks/legend.
        fig.subplots_adjust(left=0.05, right=0.94)
    except Exception:
        pass
    return fig


def stats_to_latex(
    stats: List[Dict[str, Any]],
    tz_name: Optional[str],
    units: str,
    caption: Optional[str] = None,
    label: Optional[str] = None,
) -> str:
    """Render the stats payload as a LaTeX tabular string.

    - stats: list of dicts from server (should contain StartTime, Count, P50Speed, P85Speed, P98Speed, MaxSpeed)
    - tz_name: timezone to display times in (or None for UTC)
    - units: speed units string to show in header
    - caption/label: optional LaTeX caption/label
    Returns a complete LaTeX table as a string.
    """

    def fmt_time(tval: Any) -> str:
        try:
            dt = _parse_server_time(tval)
            if tz_name:
                try:
                    tzobj = ZoneInfo(tz_name)
                except Exception:
                    tzobj = timezone.utc
                dt = dt.astimezone(tzobj)
            else:
                dt = dt.astimezone(timezone.utc)
            # shorter time format: MM-DD HH:MM
            return dt.strftime("%m-%d %H:%M")
        except Exception:
            return str(tval)

    def fmt_num(v: Any) -> str:
        try:
            if v is None:
                return "--"
            f = float(v)
            if np.isnan(f):
                return "--"
            return f"{f:.2f}"
        except Exception:
            return "--"

    lines: List[str] = []
    # Produce a non-floating table so it appears inline when \input{} inside multicols
    lines.append("\\begin{center}")
    # reduce font and column padding to make table fit one column
    lines.append("\\small")
    lines.append("\\setlength{\\tabcolsep}{4pt}")
    # tabular columns: time, count, p50, p85, p98, max
    lines.append("\\begin{tabular}{lrrrrr}")
    # put units on a second line within the header cell using \shortstack
    header = (
        f"Start Time & Count & \\shortstack{{p50\\\\({units})}} & \\shortstack{{p85\\\\({units})}}"
        f" & \\shortstack{{p98\\\\({units})}} & \\shortstack{{Max\\\\({units})}} \\\\"
    )
    lines.append(header)
    lines.append("\\hline")

    for row in stats:
        st = row.get("StartTime") or row.get("start_time") or row.get("starttime")
        tstr = fmt_time(st)
        cnt = row.get("Count") if row.get("Count") is not None else 0
        p50v = row.get("P50Speed") or row.get("p50speed") or row.get("p50")
        p85v = row.get("P85Speed") or row.get("p85speed")
        p98v = row.get("P98Speed") or row.get("p98speed")
        maxv = row.get("MaxSpeed") or row.get("maxspeed")
        line = f"{tstr} & {int(cnt)} & {fmt_num(p50v)} & {fmt_num(p85v)} & {fmt_num(p98v)} & {fmt_num(maxv)} \\\\"
        lines.append(line)

    lines.append("\\hline")
    lines.append("\\end{tabular}")
    # Place caption as a full-width centered box so it doesn't wrap beside the table
    if caption:
        lines.append("\\par\\vspace{2pt}")
        # makebox with \linewidth forces the caption to occupy the column width
        lines.append(
            f"\\noindent\\makebox[\\linewidth]{{\\textbf{{\\small {caption}}}}}"
        )
    lines.append("\\end{center}")
    return "\n".join(lines)


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
        if args.debug:
            print(data)

        # If requested, generate a LaTeX table from the stats payload
        if getattr(args, "tex_table", ""):
            try:
                tex = stats_to_latex(
                    data,
                    args.timezone or None,
                    args.units,
                    caption="Table 1: Granular breakdown",
                    label=None,
                )
                outpath = args.tex_table
                # compute ISO-formatted start/end using requested timezone (or UTC)
                try:
                    if args.timezone:
                        tzobj = ZoneInfo(args.timezone)
                    else:
                        tzobj = timezone.utc
                    start_iso = datetime.fromtimestamp(start_ts, tz=tzobj).isoformat()
                    end_iso = datetime.fromtimestamp(end_ts, tz=tzobj).isoformat()
                except Exception:
                    start_iso = str(start_date)
                    end_iso = str(end_date)
                # Determine the resolved output file (out_file) or stdout ('-')
                if outpath == "-":
                    out_file = "-"
                    # print generation parameters before the table (each on its own LaTeX line)
                    gen_params = (
                        "% === Generation parameters ===\n"
                        + "\\noindent\\textbf{Start time:} "
                        + f"{start_iso} \\\\\n"
                        + "\\textbf{End time:} "
                        + f"{end_iso} \\\\\n"
                        + "\\textbf{Rollup period:} "
                        + f"{args.group}\n\n"
                    )
                    print(gen_params)
                    print(tex)
                else:
                    # If multiple ranges specified, avoid clobbering by suffixing with dates
                    if len(date_ranges) > 1:
                        try:
                            if args.timezone:
                                tzobj = ZoneInfo(args.timezone)
                            else:
                                tzobj = timezone.utc
                            start_label = (
                                datetime.fromtimestamp(start_ts, tz=tzobj)
                                .date()
                                .isoformat()
                            )
                            end_label = (
                                datetime.fromtimestamp(end_ts, tz=tzobj)
                                .date()
                                .isoformat()
                            )
                        except Exception:
                            start_label = str(start_date)
                            end_label = str(end_date)

                        if outpath.lower().endswith(".tex"):
                            base = outpath[:-4]
                        else:
                            base = outpath
                        out_file = f"{base}_{start_label}_to_{end_label}.tex"
                    else:
                        out_file = outpath

                    try:
                        # write generation parameters followed by the main table (each param on new line)
                        gen_params = (
                            "% === Generation parameters ===\n"
                            + "\\noindent\\textbf{Start time:} "
                            + f"{start_iso} \\\\\n"
                            + "\\textbf{End time:} "
                            + f"{end_iso} \\\\\n"
                            + "\\textbf{Rollup period:} "
                            + f"{args.group}\n\n"
                        )
                        with open(out_file, "w", encoding="utf-8") as f:
                            f.write(gen_params)
                            f.write(tex)
                        print(f"Wrote LaTeX table: {out_file}")
                    except Exception as e:
                        print(f"Failed to write LaTeX table to {outpath}: {e}")
            except Exception as e:
                print(
                    f"failed to generate LaTeX table for {start_date} - {end_date}: {e}"
                )

            # --- additional summary outputs: daily (24h) and overall (all) ---
            try:
                # determine whether to produce daily + overall or only overall
                provided_group_seconds = SUPPORTED_GROUPS.get(args.group)
                produce_daily = True
                if (
                    provided_group_seconds is not None
                    and provided_group_seconds >= 24 * 3600
                ):
                    produce_daily = False
                else:
                    # If group token isn't in SUPPORTED_GROUPS, try to parse numeric forms like '48h' or '2d'
                    m = re.match(r"^(\d+)([smhd])$", str(args.group or ""))
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
                            produce_daily = False

                # prepare base filename and date labels
                if outpath == "-":
                    base = "summary"
                else:
                    if outpath.lower().endswith(".tex"):
                        base = outpath[:-4]
                    else:
                        base = outpath

                # compute safe date suffix
                try:
                    if args.timezone:
                        tzobj = ZoneInfo(args.timezone)
                    else:
                        tzobj = timezone.utc
                    start_label = (
                        datetime.fromtimestamp(start_ts, tz=tzobj).date().isoformat()
                    )
                    end_label = (
                        datetime.fromtimestamp(end_ts, tz=tzobj).date().isoformat()
                    )
                except Exception:
                    start_label = str(start_date)
                    end_label = str(end_date)

                # helper to write tex string
                def _write_tex(path: str, content: str):
                    try:
                        with open(path, "w", encoding="utf-8") as f:
                            f.write(content)
                        print(f"Wrote LaTeX table: {path}")
                    except Exception as e:
                        print(f"Failed to write LaTeX table to {path}: {e}")

                # produce daily 24h rollup if requested
                if produce_daily:
                    try:
                        data_daily, resp_daily = get_stats(
                            start_ts,
                            end_ts,
                            group="24h",
                            units=args.units,
                            source=args.source,
                            timezone=args.timezone or None,
                            min_speed=args.min_speed,
                        )
                        # table
                        daily_tex = stats_to_latex(
                            data_daily,
                            args.timezone or None,
                            args.units,
                            caption="Table 2: Daily Summary",
                            label=None,
                        )
                        # Append daily table to the chosen output (out_file) or print to stdout
                        if out_file == "-":
                            print("\n% === Daily summary ===\n")
                            print(daily_tex)
                        else:
                            try:
                                with open(out_file, "a", encoding="utf-8") as f:
                                    f.write("\n% === Daily summary ===\n")
                                    f.write(daily_tex)
                                    f.write("\n")
                                print(f"Appended daily table to: {out_file}")
                            except Exception as e:
                                print(
                                    f"Failed to append daily table to {out_file}: {e}"
                                )
                        # chart PDF
                        expected_daily = SUPPORTED_GROUPS.get("24h")
                        fig_daily = plot_stats_page(
                            data_daily,
                            f"Daily summary {start_label} to {end_label}",
                            args.units,
                            debug=args.debug,
                            expected_delta_seconds=(
                                expected_daily
                                if expected_daily and expected_daily > 0
                                else None
                            ),
                        )
                        daily_pdf_path = (
                            f"{base}_{start_label}_to_{end_label}_daily.pdf"
                        )
                        try:
                            fig_daily.savefig(daily_pdf_path)
                            print(f"Wrote PDF: {daily_pdf_path}")
                        except Exception as e:
                            print(f"Failed to write daily PDF: {e}")
                        try:
                            plt.close(fig_daily)
                        except Exception:
                            pass
                    except Exception as e:
                        print(f"failed to generate daily summary: {e}")

                # always produce overall (all) rollup
                try:
                    data_all, resp_all = get_stats(
                        start_ts,
                        end_ts,
                        group="all",
                        units=args.units,
                        source=args.source,
                        timezone=args.timezone or None,
                        min_speed=args.min_speed,
                    )
                    overall_tex = stats_to_latex(
                        data_all,
                        args.timezone or None,
                        args.units,
                        caption="Table 3: Overall Summary",
                        label=None,
                    )
                    # Append overall table to the chosen output (out_file) or print to stdout
                    if out_file == "-":
                        print("\n% === Overall summary ===\n")
                        print(overall_tex)
                    else:
                        try:
                            with open(out_file, "a", encoding="utf-8") as f:
                                f.write("\n% === Overall summary ===\n")
                                f.write(overall_tex)
                                f.write("\n")
                            print(f"Appended overall table to: {out_file}")
                        except Exception as e:
                            print(f"Failed to append overall table to {out_file}: {e}")

                    # overall chart (expected delta None)
                    fig_all = plot_stats_page(
                        data_all,
                        f"Overall summary {start_label} to {end_label}",
                        args.units,
                        debug=args.debug,
                        expected_delta_seconds=None,
                    )
                    overall_pdf_path = (
                        f"{base}_{start_label}_to_{end_label}_overall.pdf"
                    )
                    try:
                        fig_all.savefig(overall_pdf_path)
                        print(f"Wrote PDF: {overall_pdf_path}")
                    except Exception as e:
                        print(f"Failed to write overall PDF: {e}")
                    try:
                        plt.close(fig_all)
                    except Exception:
                        pass
                except Exception as e:
                    print(f"failed to generate overall summary: {e}")
            except Exception:
                # keep going if summaries fail
                pass

        # If requested, generate a plot page and save to PDF
        if pdf is not None:
            tz_label = args.timezone or "UTC"

            def _unix_to_yyyy_mm_dd(ts: int, tz_name: Optional[str]) -> str:
                try:
                    if tz_name:
                        tzobj = ZoneInfo(tz_name)
                    else:
                        tzobj = timezone.utc
                    dt = datetime.fromtimestamp(ts, tz=tzobj)
                    return dt.date().isoformat()
                except Exception:
                    # fallback to UTC date string
                    return (
                        datetime.fromtimestamp(ts, tz=timezone.utc).date().isoformat()
                    )

            start_label = _unix_to_yyyy_mm_dd(start_ts, args.timezone or None)
            end_label = _unix_to_yyyy_mm_dd(end_ts, args.timezone or None)
            title = f"{start_label} to {end_label} ({args.source}, group={args.group}, tz={tz_label})"
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
        "--tex-table",
        default="",
        help="Path to write a LaTeX table for the stats payload. Use '-' for stdout. If multiple ranges are provided, files will be suffixed with the date range.",
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
