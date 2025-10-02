#!/usr/bin/env python3

import argparse
import requests
import sys
from typing import List, Tuple, Union, Optional, Any, Dict
from datetime import datetime, timezone, time
from zoneinfo import ZoneInfo, ZoneInfoNotFoundError
import numpy as np

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


def plot_stats_page(stats: List[Dict[str, Any]], title: str, units: str) -> Any:
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
        p50.append(row.get("P50Speed") or row.get("p50speed") or row.get("p50") or 0)
        p85.append(row.get("P85Speed") or row.get("p85speed") or 0)
        p98.append(row.get("P98Speed") or row.get("p98speed") or 0)
        mx.append(row.get("MaxSpeed") or row.get("maxspeed") or 0)

    fig, ax = plt.subplots(figsize=(10, 4))
    if not times:
        ax.text(0.5, 0.5, "No data", ha="center", va="center")
        ax.set_title(title)
        return fig

    ax.plot(times, p50, label="P50", marker="o")
    ax.plot(times, p85, label="P85", marker="^")
    ax.plot(times, p98, label="P98", marker="s")
    ax.plot(times, mx, label="Max", marker="x", linestyle="--")

    ax.set_ylabel(f"Speed ({units})")
    ax.set_title(title)
    ax.legend()
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
            start_ts = parse_date_to_unix(
                start_date, end_of_day=False, tz_name=(args.timezone or None)
            )
            end_ts = parse_date_to_unix(
                end_date, end_of_day=True, tz_name=(args.timezone or None)
            )
        except ValueError as e:
            print(f"Bad date range ({start_date} - {end_date}): {e}")
            continue

        print(f"Querying stats from {start_date} ({start_ts}) to {end_date} ({end_ts})...")
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
                print("       local (", args.timezone or 'UTC', ") ->", local_start)

            print("DEBUG: computed end_ts   ->", end_ts)
            print("       UTC ->", utc_end)
            if local_end is not None:
                print("       local (", args.timezone or 'UTC', ") ->", local_end)
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
                fig = plot_stats_page(data, title, args.units)
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
