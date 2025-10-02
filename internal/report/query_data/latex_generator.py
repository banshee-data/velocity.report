"""LaTeX table generation for radar statistics."""

import numpy as np
from typing import List, Dict, Any, Optional
from zoneinfo import ZoneInfo
from datetime import timezone

from .date_parser import parse_server_time


def format_time(tval: Any, tz_name: Optional[str]) -> str:
    """Format a server time value for display.

    Args:
        tval: Time value from server
        tz_name: Timezone name for display

    Returns:
        Formatted time string (MM-DD HH:MM)
    """
    try:
        dt = parse_server_time(tval)
        if tz_name:
            try:
                tzobj = ZoneInfo(tz_name)
            except Exception:
                tzobj = timezone.utc
            dt = dt.astimezone(tzobj)
        else:
            dt = dt.astimezone(timezone.utc)
        return dt.strftime("%m-%d %H:%M")
    except Exception:
        return str(tval)


def format_number(v: Any) -> str:
    """Format a numeric value for display.

    Args:
        v: Numeric value (may be None or NaN)

    Returns:
        Formatted string or "--" for missing values
    """
    try:
        if v is None:
            return "--"
        f = float(v)
        if np.isnan(f):
            return "--"
        return f"{f:.2f}"
    except Exception:
        return "--"


def stats_to_latex(
    stats: List[Dict[str, Any]],
    tz_name: Optional[str],
    units: str,
    caption: Optional[str] = None,
    include_start_time: bool = True,
) -> str:
    """Render statistics as a LaTeX table.

    Args:
        stats: List of stat dictionaries from the API
        tz_name: Timezone name for time display
        units: Speed units string
        caption: Optional table caption
        include_start_time: Whether to include StartTime column

    Returns:
        LaTeX table string
    """
    lines: List[str] = []
    lines.append("\\begin{center}")
    lines.append("\\small")
    lines.append("\\setlength{\\tabcolsep}{4pt}")

    if include_start_time:
        lines.append("\\begin{tabular}{lrrrrr}")
        header = (
            f"Start Time & Count & \\shortstack{{p50\\\\({units})}} & \\shortstack{{p85\\\\({units})}}"
            f" & \\shortstack{{p98\\\\({units})}} & \\shortstack{{Max\\\\({units})}} \\\\"
        )
    else:
        lines.append("\\begin{tabular}{rrrrr}")
        header = (
            f"Count & \\shortstack{{p50\\\\({units})}} & \\shortstack{{p85\\\\({units})}}"
            f" & \\shortstack{{p98\\\\({units})}} & \\shortstack{{Max\\\\({units})}} \\\\"
        )

    lines.append(header)
    lines.append("\\hline")

    for row in stats:
        cnt = row.get("Count") if row.get("Count") is not None else 0
        p50v = row.get("P50Speed") or row.get("p50speed") or row.get("p50")
        p85v = row.get("P85Speed") or row.get("p85speed")
        p98v = row.get("P98Speed") or row.get("p98speed")
        maxv = row.get("MaxSpeed") or row.get("maxspeed")

        if include_start_time:
            st = row.get("StartTime") or row.get("start_time") or row.get("starttime")
            tstr = format_time(st, tz_name)
            line = f"{tstr} & {int(cnt)} & {format_number(p50v)} & {format_number(p85v)} & {format_number(p98v)} & {format_number(maxv)} \\\\"
        else:
            line = f"{int(cnt)} & {format_number(p50v)} & {format_number(p85v)} & {format_number(p98v)} & {format_number(maxv)} \\\\"
        lines.append(line)

    lines.append("\\hline")
    lines.append("\\end{tabular}")

    if caption:
        lines.append("\\par\\vspace{2pt}")
        lines.append(
            f"\\noindent\\makebox[\\linewidth]{{\\textbf{{\\small {caption}}}}}"
        )

    lines.append("\\end{center}")
    return "\n".join(lines)


def generate_table_file(
    file_path: str,
    start_iso: str,
    end_iso: str,
    group: str,
    units: str,
    timezone_display: str,
    min_speed_str: str,
    overall_metrics: List[Dict[str, Any]],
    daily_metrics: Optional[List[Dict[str, Any]]],
    granular_metrics: List[Dict[str, Any]],
    tz_name: Optional[str],
) -> None:
    """Generate a complete LaTeX table file with generation parameters.

    Args:
        file_path: Output file path
        start_iso: Start time ISO string
        end_iso: End time ISO string
        group: Aggregation group
        units: Speed units
        timezone_display: Timezone display string
        min_speed_str: Min speed display string
        overall_metrics: Overall summary metrics
        daily_metrics: Daily summary metrics (or None to skip)
        granular_metrics: Granular breakdown metrics
        tz_name: Timezone name for formatting
    """
    gen_params = (
        "% === Generation parameters ===\n"
        + "\\noindent\\textbf{Start time:} "
        + f"{start_iso} \\\\\n"
        + "\\textbf{End time:} "
        + f"{end_iso} \\\\\n"
        + "\\textbf{Rollup period:} "
        + f"{group} \\\\\n"
        + "\\textbf{Units:} "
        + f"{units} \\\\\n"
        + "\\textbf{Timezone:} "
        + f"{timezone_display} \\\\\n"
        + "\\textbf{Min speed (cutoff):} "
        + f"{min_speed_str}\n\n"
    )

    with open(file_path, "w", encoding="utf-8") as f:
        f.write(gen_params)

    # Overall (Table 1)
    overall_tex = stats_to_latex(
        overall_metrics,
        tz_name,
        units,
        caption="Table 1: Overall Summary",
        include_start_time=False,
    )
    with open(file_path, "a", encoding="utf-8") as f:
        f.write("\n% === Overall summary ===\n")
        f.write(overall_tex)
        f.write("\n")

    # Daily (Table 2) if provided
    if daily_metrics:
        daily_tex = stats_to_latex(
            daily_metrics, tz_name, units, caption="Table 2: Daily Summary"
        )
        with open(file_path, "a", encoding="utf-8") as f:
            f.write("\n% === Daily summary ===\n")
            f.write(daily_tex)
            f.write("\n")

    # Granular (Table 3)
    granular_tex = stats_to_latex(
        granular_metrics, tz_name, units, caption="Table 3: Granular breakdown"
    )
    with open(file_path, "a", encoding="utf-8") as f:
        f.write("\n% === Granular breakdown ===\n")
        f.write(granular_tex)
        f.write("\n")
