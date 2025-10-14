"""Date and time parsing utilities for radar stats queries."""

import re
from typing import Union, Optional
from datetime import datetime, timezone, time
from zoneinfo import ZoneInfo, ZoneInfoNotFoundError


def parse_date_to_unix(
    d: Union[str, int], end_of_day: bool = False, tz_name: Optional[str] = None
) -> int:
    """Parse a YYYY-MM-DD date, ISO datetime, or numeric timestamp and return unix seconds.

    Args:
        d: Date string, ISO datetime, or unix timestamp (int)
        end_of_day: If True and d is YYYY-MM-DD, parse as 23:59:59
        tz_name: Timezone name for interpretation (e.g., 'US/Pacific')

    Returns:
        Unix timestamp in seconds

    Raises:
        ValueError: If date format is invalid or timezone is unknown
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


def parse_server_time(t) -> datetime:
    """Parse time value returned by server into a timezone-aware datetime (UTC).

    Args:
        t: Time value (RFC3339 string, unix timestamp, etc.)

    Returns:
        Timezone-aware datetime object

    Raises:
        ValueError: If time format is unsupported
    """
    if isinstance(t, (int, float)):
        return datetime.fromtimestamp(float(t), tz=timezone.utc)
    if not isinstance(t, str):
        raise ValueError(f"unsupported time format: {t!r}")
    s = t.strip()
    # RFC3339 'Z' -> +00:00 for fromisoformat
    if s.endswith("Z"):
        s = s[:-1] + "+00:00"
    return datetime.fromisoformat(s)


def is_date_only(s: str) -> bool:
    """Check if string is a plain YYYY-MM-DD date (not a full datetime).

    Args:
        s: String to check

    Returns:
        True if string matches YYYY-MM-DD pattern
    """
    try:
        return bool(re.match(r"^\d{4}-\d{2}-\d{2}$", str(s).strip()))
    except Exception:
        return False
