#!/usr/bin/env python3

"""PDF report generation using PyLaTeX.

This module replaces the custom LaTeX generator with PyLaTeX to create
complete PDF reports including statistics tables, charts, and science sections.
"""

import os
from typing import Any, Dict, List, Optional
from datetime import datetime
from zoneinfo import ZoneInfo
from datetime import timezone

import numpy as np

try:
    from pylatex import (
        Document,
        Section,
        Command,
        Package,
        Tabular,
        Center,
        Figure,
        NoEscape,
        NewPage,
        NewLine,
        LineBreak,
    )
    from pylatex.table import Tabular
    from pylatex.utils import escape_latex
    from pylatex.base_classes import Environment

    HAVE_PYLATEX = True
except Exception:  # pragma: no cover - allow tests to run without pylatex installed
    # Provide minimal fallbacks so module can be imported in test environments
    HAVE_PYLATEX = False

    class NoEscape(str):
        pass

    # Lightweight stand-ins (only so imports don't fail). They won't provide full functionality.
    class Document:  # type: ignore
        def __init__(self, *args, **kwargs):
            pass

    class Section:  # type: ignore
        pass

    class Command:  # type: ignore
        pass

    class Package:  # type: ignore
        def __init__(self, *args, **kwargs):
            pass

    class Tabular:  # type: ignore
        def __init__(self, *args, **kwargs):
            pass

    class Center:  # type: ignore
        def __init__(self, *args, **kwargs):
            pass

    class Figure:  # type: ignore
        def __init__(self, *args, **kwargs):
            pass

    def escape_latex(s: str) -> str:
        return s

    class Environment:  # type: ignore
        pass


from stats_utils import (
    format_time,
    format_number,
    process_histogram,
    count_in_histogram_range,
    count_histogram_ge,
    chart_exists,
)


class MultiCol(Environment):
    """Custom multicol environment for PyLaTeX."""

    _latex_name = "multicols"
    packages = [Package("multicol")]

    def __init__(self, columns=2, **kwargs):
        super().__init__(**kwargs)
        self.columns = columns

    def dumps(self):
        return f"\\begin{{{self._latex_name}}}{{{self.columns}}}\n{self.dumps_content()}\n\\end{{{self._latex_name}}}"


def create_stats_table(
    stats: List[Dict[str, Any]],
    tz_name: Optional[str],
    units: str,
    caption: str,
    include_start_time: bool = True,
) -> Center:
    """Create a statistics table using PyLaTeX."""

    if include_start_time:
        table_spec = "lrrrrr"
        headers = [
            "Start Time",
            "Count",
            f"\\shortstack{{{{p50 \\\\ ({escape_latex(units)})}}}}",
            f"\\shortstack{{{{p85 \\\\ ({escape_latex(units)})}}}}",
            f"\\shortstack{{{{p98 \\\\ ({escape_latex(units)})}}}}",
            f"\\shortstack{{{{Max \\\\ ({escape_latex(units)})}}}}",
        ]
    else:
        table_spec = "rrrrr"
        headers = [
            "Count",
            f"\\shortstack{{{{p50 \\\\ ({escape_latex(units)})}}}}",
            f"\\shortstack{{{{p85 \\\\ ({escape_latex(units)})}}}}",
            f"\\shortstack{{{{p98 \\\\ ({escape_latex(units)})}}}}",
            f"\\shortstack{{{{Max \\\\ ({escape_latex(units)})}}}}",
        ]

    centered = Center()
    # denser table font and tighter horizontal padding
    # centered.append(NoEscape(r"\scriptsize"))
    # centered.append(NoEscape(r"\setlength{\tabcolsep}{3pt}"))

    table = Tabular(table_spec)
    # Add headers
    # table.add_hline()
    table.add_row([NoEscape(h) for h in headers])
    table.add_hline()

    # Add data rows
    for row in stats:
        cnt = row.get("Count") if row.get("Count") is not None else 0
        p50v = row.get("P50Speed") or row.get("p50speed") or row.get("p50")
        p85v = row.get("P85Speed") or row.get("p85speed")
        p98v = row.get("P98Speed") or row.get("p98speed")
        maxv = row.get("MaxSpeed") or row.get("maxspeed")

        if include_start_time:
            st = row.get("StartTime") or row.get("start_time") or row.get("starttime")
            tstr = format_time(st, tz_name)
            table.add_row(
                [
                    tstr,
                    int(cnt),
                    format_number(p50v),
                    format_number(p85v),
                    format_number(p98v),
                    format_number(maxv),
                ]
            )
        else:
            table.add_row(
                [
                    int(cnt),
                    format_number(p50v),
                    format_number(p85v),
                    format_number(p98v),
                    format_number(maxv),
                ]
            )

    table.add_hline()
    centered.append(table)

    # Add caption
    centered.append(NoEscape("\\par\\vspace{2pt}"))
    centered.append(
        NoEscape(f"\\noindent\\makebox[\\linewidth]{{\\textbf{{\\small {caption}}}}}")
    )

    return centered


def create_histogram_table(
    histogram: Dict[str, int],
    units: str,
    cutoff: float = 5.0,
    bucket_size: float = 5.0,
    max_bucket: float = 50.0,
) -> Center:
    """Create a histogram table using PyLaTeX."""

    # Use centralized histogram processing
    numeric_buckets, total, ranges = process_histogram(
        histogram, cutoff, bucket_size, max_bucket
    )

    centered = Center()
    # denser histogram table font and tighter padding
    # centered.append(NoEscape(r"\scriptsize"))
    # centered.append(NoEscape(r"\setlength{\tabcolsep}{3pt}"))

    table = Tabular("lrr")
    table.add_hline()
    table.add_row(["Bucket", "Count", "Percent"])
    table.add_hline()

    # Cutoff row
    below_cutoff = sum(v for k, v in numeric_buckets.items() if k < cutoff)
    pct_below = (below_cutoff / total * 100.0) if total > 0 else 0.0
    table.add_row(
        [
            NoEscape(f"\\textit{{<{int(cutoff)}}}"),
            NoEscape(f"\\textit{{{below_cutoff}}}"),
            NoEscape(f"\\textit{{{pct_below:.1f}\\%}}"),
        ]
    )

    # Range rows
    for a, b in ranges:
        cnt = count_in_histogram_range(numeric_buckets, a, b)
        pct = (cnt / total * 100.0) if total > 0 else 0.0
        label = f"{int(a)}-{int(b)}"
        table.add_row([label, cnt, f"{pct:.1f}%"])

    # Final row
    cnt_ge = count_histogram_ge(numeric_buckets, max_bucket)
    pct_ge = (cnt_ge / total * 100.0) if total > 0 else 0.0
    table.add_row([f"{int(max_bucket)}+", cnt_ge, f"{pct_ge:.1f}%"])

    table.add_hline()
    centered.append(table)

    # Add caption
    centered.append(NoEscape("\\par\\vspace{2pt}"))
    centered.append(
        NoEscape(
            "\\noindent\\makebox[\\linewidth]{\\textbf{\\small Table 4: Histogram}}"
        )
    )

    return centered


def add_science_content(
    doc: Document,
    start_date: str,
    end_date: str,
    location: str,
    speed_limit: int,
    total_vehicles: int,
    p50: float,
    p85: float,
    p98: float,
    max_speed: float,
) -> None:
    """Add science section content to the document."""

    # Speed Metrics Overview
    doc.append(NoEscape("\\section*{Velocity Overview}"))

    doc.append(
        NoEscape(
            f"Between \\textbf{{{start_date}}} and \\textbf{{{end_date}}}, "
            f"the radar sensor recorded velocities for \\textbf{{{total_vehicles}}} vehicles on "
            f"\\textbf{{{location}}}."
        )
    )

    doc.append(NoEscape("\\subsection*{Key Velocity Metrics}"))
    doc.append(NoEscape("\\begin{itemize}"))
    doc.append(NoEscape(f"\\item Median (p50): {p50:.1f} mph"))
    doc.append(NoEscape(f"\\item 85th Percentile (p85): {p85:.1f} mph"))
    doc.append(NoEscape(f"\\item 98th Percentile (p98): {p98:.1f} mph"))
    doc.append(NoEscape(f"\\item Maximum Speed: {max_speed:.1f} mph"))
    doc.append(NoEscape("\\end{itemize}"))

    doc.append(
        NoEscape(
            "The 85th percentile (p85) represents the speed at or below which 85\\% of drivers travel. "
            "In contrast, the 98th percentile (p98) highlights the top 2\\% of driver speeds while "
            "filtering out noise from outliers. This gives a clearer view of high-risk behavior "
            "without letting single outliers dominate."
        )
    )
    doc.append(LineBreak())

    doc.append(
        NoEscape(
            "\\textbf{Note:} Metrics like p85 and p98 can be unstable with small sample sizes "
            "(fewer than 50 samples). In such cases, the difference between percentiles may not show "
            "meaningful distinction. As such, our charts highlight periods with low counts in orange, "
            "drawing attention to the need for more data before making conclusions."
        )
    )

    # # Recommendations
    # doc.append(NoEscape("\section*{Recommendations}"))
    # doc.append(NoEscape("\begin{itemize}"))
    # doc.append(
    #     NoEscape(
    #         "\item Consider physical or policy interventions at locations or hours where p98 "
    #         "significantly exceeds the posted speed limit."
    #     )
    # )
    # doc.append(
    #     NoEscape(
    #         "\item Re-run the survey after interventions to measure shifts in p85 and p98 metrics."
    #     )
    # )
    # doc.append(
    #     NoEscape(
    #         "\item Present detailed tables and charts in the appendix for stakeholders who need raw data."
    #     )
    # )


def generate_pdf_report(
    output_path: str,
    start_iso: str,
    end_iso: str,
    group: str,
    units: str,
    timezone_display: str,
    min_speed_str: str,
    location: str,
    overall_metrics: List[Dict[str, Any]],
    daily_metrics: Optional[List[Dict[str, Any]]],
    granular_metrics: List[Dict[str, Any]],
    histogram: Optional[Dict[str, int]],
    tz_name: Optional[str],
    charts_prefix: str = "out",
    speed_limit: int = 25,
) -> None:
    """Generate a complete PDF report using PyLaTeX."""

    # Create document with very tight geometry so stats occupy most of the page
    geometry_options = {
        "top": "1.8cm",
        "bottom": "1.0cm",
        "left": "1.0cm",
        "right": "1.0cm",
    }

    doc = Document(geometry_options=geometry_options, page_numbers=False)

    # Add required packages
    doc.packages.append(Package("multicol"))
    doc.packages.append(Package("fancyhdr"))
    doc.packages.append(Package("graphicx"))
    doc.packages.append(Package("titlesec"))
    doc.packages.append(Package("hyperref"))
    doc.packages.append(Package("lipsum"))
    # Use supertabular so long tables can span pages/columns
    doc.packages.append(Package("supertabular"))
    doc.packages.append(Package("float"))  # Required for H position

    # Set up header
    doc.append(NoEscape("\\setlength{\\headheight}{6pt}"))
    doc.append(NoEscape("\\pagestyle{fancy}"))
    doc.append(NoEscape("\\fancyhf{}"))
    doc.append(
        NoEscape(
            "\\fancyhead[L]{\\textbf{\\protect\\href{https://velocity.report}{velocity.report}}}"
        )
    )
    doc.append(
        NoEscape(
            f"\\fancyhead[C]{{\\textbullet \\ \\ {start_iso[:10]} to {end_iso[:10]} \\ \\textbullet}}"
        )
    )
    doc.append(NoEscape(f"\\fancyhead[R]{{ \\textit{{{location}}}}}"))
    doc.append(NoEscape("\\renewcommand{\\headrulewidth}{0.8pt}"))

    # Title formatting
    doc.append(NoEscape("\\titleformat{\\section}{\\bfseries\\Large}{}{0em}{}"))
    # reduce column gap to maximize usable width
    doc.append(NoEscape("\\setlength{\\columnsep}{2pt}"))

    # Document title
    with doc.create(Center()) as title_center:
        title_center.append(NoEscape(f"{{\\huge \\textbf{{ {location}}}}}"))
        title_center.append(LineBreak())
        title_center.append(NoEscape("\\vspace{0.1cm}"))
        title_center.append(
            NoEscape(
                "{\\large \\textit{Banshee, INC } \\textbullet \\ \\href{mailto:david@banshee-data.com}{david@banshee-data.com}}"
            )
        )

    # Begin multicolumn layout
    doc.append(NoEscape("\\begin{multicols}{2}"))

    # Add science section content using helper function
    if overall_metrics:
        overall = overall_metrics[0]
        p50 = (
            overall.get("P50Speed")
            or overall.get("p50speed")
            or overall.get("p50")
            or 0
        )
        p85 = (
            overall.get("P85Speed")
            or overall.get("p85speed")
            or overall.get("p85")
            or 0
        )
        p98 = (
            overall.get("P98Speed")
            or overall.get("p98speed")
            or overall.get("p98")
            or 0
        )
        max_speed = overall.get("MaxSpeed") or overall.get("maxspeed") or 0
        total_vehicles = overall.get("Count") or 0

        # Extract dates for display
        start_date = start_iso[:10]
        end_date = end_iso[:10]

        # Use the DRY helper function for science content
        add_science_content(
            doc,
            start_date,
            end_date,
            location,
            speed_limit,
            total_vehicles,
            p50,
            p85,
            p98,
            max_speed,
        )

    doc.append(NoEscape("\\vspace{0.5cm}"))

    # Statistics section
    doc.append(NoEscape("\\section*{Survey Parameters}"))

    # Generation parameters
    doc.append(NoEscape("% === Generation parameters ==="))
    doc.append(NoEscape(f"\\textbf{{Units:}} {units} \\\\"))
    doc.append(NoEscape(f"\\textbf{{Timezone:}} {timezone_display} \\\\"))
    doc.append(NoEscape(f"\\textbf{{Min speed (cutoff):}} {min_speed_str} \\\\"))
    doc.append(NoEscape(f"\\textbf{{Start time:}} {start_iso} \\\\"))
    doc.append(NoEscape(f"\\textbf{{End time:}} {end_iso} \\\\"))
    doc.append(NoEscape(f"\\textbf{{Rollup period:}} {group} \\\\"))
    # minimize vertical gap after science section
    doc.append(NoEscape("\\vspace{1pt}"))

    # Add tables
    if overall_metrics:
        doc.append(
            create_stats_table(
                overall_metrics,
                tz_name,
                units,
                "Table 1: Overall Summary",
                include_start_time=False,
            )
        )

    if daily_metrics:
        doc.append(
            create_stats_table(
                daily_metrics,
                tz_name,
                units,
                "Table 2: Daily Summary",
                include_start_time=True,
            )
        )

    if granular_metrics:
        doc.append(
            create_stats_table(
                granular_metrics,
                tz_name,
                units,
                "Table 3: Granular breakdown",
                include_start_time=True,
            )
        )

    # Add histogram chart if available
    if chart_exists(charts_prefix, "histogram"):
        hist_path = os.path.abspath(f"{charts_prefix}_histogram.pdf")
        with doc.create(Center()) as hist_chart_center:
            with hist_chart_center.create(Figure(position="H")) as fig:
                # use full available text width for histogram as well
                fig.add_image(hist_path, width=NoEscape(r"\linewidth"))
                fig.add_caption("Velocity distribution histogram")

    # Add histogram table if available
    if histogram:
        doc.append(create_histogram_table(histogram, units))

    # End multicolumn
    doc.append(NoEscape("\\end{multicols}"))

    # Add main stats chart if available
    if chart_exists(charts_prefix, "stats"):
        stats_path = os.path.abspath(f"{charts_prefix}_stats.pdf")
        with doc.create(Center()) as chart_center:
            with chart_center.create(Figure(position="H")) as fig:
                # use full available text width for charts
                fig.add_image(stats_path, width=NoEscape(r"\linewidth"))
                fig.add_caption("Velocity over time")

    # Generate PDF
    try:
        doc.generate_pdf(
            output_path.replace(".pdf", ""), clean_tex=False, compiler="pdflatex"
        )
        print(f"Generated PDF: {output_path}")
    except Exception as e:
        print(f"Failed to generate PDF: {e}")
        # Still save the .tex file for debugging
        doc.generate_tex(output_path.replace(".pdf", ""))
        print(
            f"Generated TEX file for debugging: {output_path.replace('.pdf', '.tex')}"
        )
