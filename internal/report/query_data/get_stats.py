#!/usr/bin/env python3

import argparse
import requests
import sys
from typing import List, Tuple, Union, Optional
from datetime import datetime, timezone, time

API_URL = "http://localhost:8080/api/radar_stats"


def parse_date_to_unix(d: Union[str, int], end_of_day: bool = False) -> int:
    """Parse a YYYY-MM-DD date or numeric timestamp and return unix seconds (UTC).

    If d is already an int or numeric string, return it as int. If d matches
    YYYY-MM-DD it is interpreted in UTC; start_of_day -> 00:00:00, end_of_day -> 23:59:59.
    Raises ValueError on bad input.
    """
    if isinstance(d, int):
        return d
    s = str(d).strip()
    # numeric string -> treat as unix seconds already
    if s.isdigit():
        return int(s)
    # expect YYYY-MM-DD
    try:
        dt = datetime.strptime(s, "%Y-%m-%d")
    except ValueError:
        raise ValueError(
            f"Invalid date format, expected YYYY-MM-DD or unix seconds: {d}"
        )
    if end_of_day:
        dt = datetime.combine(dt.date(), time(23, 59, 59))
    else:
        dt = datetime.combine(dt.date(), time(0, 0, 0))
    # treat as UTC
    dt = dt.replace(tzinfo=timezone.utc)
    return int(dt.timestamp())


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


def main(date_ranges: List[Tuple[str, str]], args: argparse.Namespace):
    for start_date, end_date in date_ranges:
        # Validate and convert to unix seconds (UTC). Start => 00:00:00, End => 23:59:59
        try:
            start_ts = parse_date_to_unix(start_date, end_of_day=False)
            end_ts = parse_date_to_unix(end_date, end_of_day=True)
        except ValueError as e:
            print(f"Bad date range ({start_date} - {end_date}): {e}")
            continue

        print(
            f"Querying stats from {start_date} ({start_ts}) to {end_date} ({end_ts})..."
        )
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
