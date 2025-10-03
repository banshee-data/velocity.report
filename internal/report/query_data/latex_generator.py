#!/usr/bin/env python3

"""LaTeX table generation for radar statistics.

This module provides helpers to render metric lists as LaTeX tables and to
render a histogram (counts + percent) into a LaTeX table matching the
format used in `internal/report/tex/table.tex`.
"""

from typing import Any, Dict, List, Optional
from zoneinfo import ZoneInfo
from datetime import timezone

import numpy as np

from date_parser import parse_server_time


def format_time(tval: Any, tz_name: Optional[str]) -> str:
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
    lines: List[str] = []
    lines.append("\\begin{center}")
    lines.append("\\small")
    lines.append("\\setlength{\\tabcolsep}{4pt}")

    if include_start_time:
        lines.append("\\begin{tabular}{lrrrrr}")
        header_body = (
            "Start Time & Count & "
            + "\\shortstack{p50\\(%s\\)} & "
            + "\\shortstack{p85\\(%s\\)} & "
            + "\\shortstack{p98\\(%s\\)} & "
            + "\\shortstack{Max\\(%s\\)}"
        )
        header = header_body % (units, units, units, units) + " \\\\"
    else:
        lines.append("\\begin{tabular}{rrrrr}")
        header_body = (
            "Count & "
            + "\\shortstack{p50\\(%s\\)} & "
            + "\\shortstack{p85\\(%s\\)} & "
            + "\\shortstack{p98\\(%s\\)} & "
            + "\\shortstack{Max\\(%s\\)}"
        )
        header = header_body % (units, units, units, units) + " \\\\"

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
            body = f"{tstr} & {int(cnt)} & {format_number(p50v)} & {format_number(p85v)} & {format_number(p98v)} & {format_number(maxv)}"
        else:
            body = f"{int(cnt)} & {format_number(p50v)} & {format_number(p85v)} & {format_number(p98v)} & {format_number(maxv)}"
        line = body + " \\\\"
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
    gen_params = (
        "% === Generation parameters ===\n"
        + "\\noindent\\textbf{Start time:} "
        + f"{start_iso} \\\\n+"
        + "\\textbf{End time:} "
        + f"{end_iso} \\\\n+"
        + "\\textbf{Rollup period:} "
        + f"{group} \\\\n+"
        + "\\textbf{Units:} "
        + f"{units} \\\\n+"
        + "\\textbf{Timezone:} "
        + f"{timezone_display} \\\\n+"
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


def plot_histogram(
    histogram: Dict[str, int],
    title: str,
    units: str,
    debug: bool = False,
) -> Optional[object]:
    try:
        import matplotlib.pyplot as plt
    except Exception as e:  # pragma: no cover - environment dependent
        raise ImportError("matplotlib is required to render histograms") from e

    if not histogram:
        fig, ax = plt.subplots(figsize=(10, 4))
        ax.text(0.5, 0.5, "No histogram data", ha="center", va="center")
        ax.set_title(title)
        return fig

    try:
        sorted_items = sorted(histogram.items(), key=lambda x: float(x[0]))
    except Exception:
        sorted_items = sorted(histogram.items(), key=lambda x: str(x[0]))

    labels = [item[0] for item in sorted_items]
    counts = [item[1] for item in sorted_items]

    if debug:
        total = sum(counts)
        print(f"DEBUG: histogram bins={len(labels)} total={total}")

    fig, ax = plt.subplots(figsize=(10, 5))
    x = list(range(len(labels)))
    ax.bar(x, counts, alpha=0.7, color="steelblue", edgecolor="black", linewidth=0.5)
    ax.set_xlabel(f"Speed ({units})")
    ax.set_ylabel("Count")
    ax.set_title(title)

    if len(labels) <= 20:
        ax.set_xticks(x)
        ax.set_xticklabels(labels, rotation=45, ha="right")
    else:
        step = max(1, len(labels) // 15)
        tick_pos = x[::step]
        tick_labels = labels[::step]
        ax.set_xticks(tick_pos)
        ax.set_xticklabels(tick_labels, rotation=45, ha="right")

    try:
        fig.tight_layout(pad=0.5)
    except Exception:
        pass

    return fig


def histogram_to_latex(
    histogram: Dict[str, int],
    units: str,
    cutoff: float = 5.0,
    bucket_size: float = 5.0,
    max_bucket: float = 50.0,
) -> str:
    """Render a histogram mapping into a LaTeX table matching table.tex.

    Produces rows:
      - a cutoff row for values < cutoff rendered in \textit{...}
      - ranges e.g. 5-10, 10-15, ...
      - a final '50+' row for values >= max_bucket

    Each row shows Count and Percent (with a literal LaTeX % escaped as \%).
    """
    # Coerce numeric keys into buckets and compute total
    numeric_buckets: Dict[float, int] = {}
    total = 0
    for k, v in histogram.items():
        try:
            fk = float(k)
            numeric_buckets[fk] = numeric_buckets.get(fk, 0) + int(v)
            total += int(v)
        except Exception:
            try:
                total += int(v)
            except Exception:
                pass

    # Display ranges
    ranges: List[tuple] = []
    s = cutoff
    while s < max_bucket:
        ranges.append((s, s + bucket_size))
        s += bucket_size

    def count_in_range(a: float, b: float) -> int:
        return sum(v for k, v in numeric_buckets.items() if k >= a and k < b)

    def count_ge(a: float) -> int:
        return sum(v for k, v in numeric_buckets.items() if k >= a)

    lines: List[str] = []
    # Header block
    lines.append("\\begin{center}")
    lines.append("\\small")
    lines.append("\\setlength{\\tabcolsep}{6pt}")
    lines.append("\\begin{tabular}{lrr}")
    lines.append("Bucket & Count & Percent " + "\\")
    lines.append("\\hline")

    below_cutoff = sum(v for k, v in numeric_buckets.items() if k < cutoff)
    pct_below = (below_cutoff / total * 100.0) if total > 0 else 0.0
    # Render cutoff row in italics; caller may wrap with color macro if desired
    lines.append(
        f"\\textit{{<{int(cutoff)}}} & \\textit{{{below_cutoff}}} & \\textit{{{pct_below:.1f}\\%}} "
        + "\\"
    )

    for a, b in ranges:
        cnt = count_in_range(a, b)
        pct = (cnt / total * 100.0) if total > 0 else 0.0
        label = f"{int(a)}-{int(b)}"
        lines.append(f"{label} & {cnt} & {pct:.1f}\\% " + "\\")

    cnt_ge = count_ge(max_bucket)
    pct_ge = (cnt_ge / total * 100.0) if total > 0 else 0.0
    lines.append(f"{int(max_bucket)}+ & {cnt_ge} & {pct_ge:.1f}\\% " + "\\")

    lines.append("\\hline")
    lines.append("\\end{tabular}")
    lines.append("\\par\\vspace{2pt}")
    lines.append(
        "\\noindent\\makebox[\\linewidth]{\\textbf{\\small Table 4: Histogram}}"
    )
    lines.append("\\end{center}")

    return "\n".join(lines)
    # Coerce numeric keys into buckets and compute total
