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
            f"\\shortstack{{p50 \\\\ ({escape_latex(units)})}}",
            f"\\shortstack{{p85 \\\\ ({escape_latex(units)})}}",
            f"\\shortstack{{p98 \\\\ ({escape_latex(units)})}}",
            f"\\shortstack{{Max \\\\ ({escape_latex(units)})}}",
        ]
    else:
        table_spec = "rrrrr"
        headers = [
            "Count",
            f"\\shortstack{{p50 \\\\ ({escape_latex(units)})}}",
            f"\\shortstack{{p85 \\\\ ({escape_latex(units)})}}",
            f"\\shortstack{{p98 \\\\ ({escape_latex(units)})}}",
            f"\\shortstack{{Max \\\\ ({escape_latex(units)})}}",
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
            # Render start time in a monospaced Atkinson font when available.
            # Use a single backslash so LaTeX sees the command name (previously
            # a double backslash caused literal "AtkinsonMono" text to appear).
            tcell = NoEscape(r"\AtkinsonMono{" + escape_latex(tstr) + r"}")
            table.add_row(
                [
                    tcell,
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
    # (global sans-serif set in preamble)

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
    # (global sans-serif set in preamble)
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


def add_metric_data_intro(
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
            f"velocity for \\textbf{{{total_vehicles}}} vehicles was recorded on "
            f"\\textbf{{{location}}}."
        )
    )

    doc.append(NoEscape("\\sffamily"))
    doc.append(NoEscape("\\subsection*{Key Velocity Metrics}"))
    table = Tabular("ll")
    table.add_row(
        [NoEscape(r"\textbf{Median Velocity (p50):}"), NoEscape(f"{p50:.1f} mph")]
    )
    table.add_row(
        [
            NoEscape(r"\textbf{85th Percentile Velocity (p85):}"),
            NoEscape(f"{p85:.1f} mph"),
        ]
    )
    table.add_row(
        [
            NoEscape(r"\textbf{98th Percentile Velocity (p98):}"),
            NoEscape(f"{p98:.1f} mph"),
        ]
    )
    table.add_row(
        [NoEscape(r"\textbf{Maximum Velocity:}"), NoEscape(f"{max_speed:.1f} mph")]
    )
    # Render this parameter table in sans-serif locally
    doc.append(NoEscape("\\sffamily"))
    doc.append(table)

    doc.append(LineBreak())
    doc.append(LineBreak())


def add_science(doc: Document) -> None:

    doc.append(NoEscape("\\subsection*{Aggregation and Percentiles}"))

    doc.append(
        NoEscape(
            "This system uses Doppler radar to measure vehicle speed. As a vehicle approaches or recedes, "
            "the radar detects a shift in frequency (the \\href{https://en.wikipedia.org/wiki/Doppler_effect}{Doppler effect}) which is directly proportional "
            "to the vehicle's velocity relative to the sensor."
        )
    )
    doc.append(LineBreak())
    doc.append(
        NoEscape(
            "The velocity.report application uses a greedy, local, univariate clustering algorithm called "
            "Time‑Contiguous Speed Clustering to group individual radar detections into transits. "
            "For this survey, raw radar read lines—recorded at ~20Hz—were grouped into sessions based on "
            "temporal proximity and speed similarity. Each resulting transit represents a short burst of "
            "movement consistent with a single passing object. This approach is efficient and reproducible, "
            "but has limitations. In dense traffic or when objects overlap, it may undercount the true number "
            "of vehicles by merging separate objects into a single transit. This undercounting can bias percentile "
            "metrics—particularly the p85 and p98—downward, since fewer transits can inflate the influence of slower "
            "vehicles. All statistics in this report are derived from these sessionized transits."
        )
    )
    doc.append(LineBreak())
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
    # Load fontspec so we can use the bundled Atkinson Hyperlegible TTFs.
    doc.packages.append(Package("fontspec"))
    # Ensure captions use sans-serif
    doc.packages.append(Package("caption", options="font=sf"))
    # Use supertabular so long tables can span pages/columns
    doc.packages.append(Package("supertabular"))
    doc.packages.append(Package("float"))  # Required for H position

    # Set up header
    # Increase headheight to avoid fancyhdr warnings on some templates
    doc.append(NoEscape("\\setlength{\\headheight}{12pt}"))
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

    # Title formatting: make section headings sans-serif (local style)
    doc.preamble.append(
        NoEscape("\\titleformat{\\section}{\\bfseries\\Large}{}{0em}{}")
    )
    # reduce column gap to maximize usable width (preamble)
    doc.preamble.append(NoEscape("\\setlength{\\columnsep}{2pt}"))

    # Set the document's default family to sans-serif globally so all text
    # uses the sans font family without selecting a specific font.
    # Point fontspec at the bundled Atkinson Hyperlegible fonts that live in
    # the `fonts/` directory next to this module. This forces the document
    # to use Atkinson Hyperlegible as the sans font.
    fonts_path = os.path.join(os.path.dirname(__file__), "fonts")
    # Use Path= with trailing os.sep so fontspec can find the files
    doc.preamble.append(
        NoEscape(
            rf"\setsansfont[Path={fonts_path + os.sep},Extension=.ttf,Scale=MatchLowercase]{{AtkinsonHyperlegible-Regular}}"
        )
    )
    # Use sans family as the default
    doc.preamble.append(NoEscape("\\renewcommand{\\familydefault}{\\sfdefault}"))

    # Register Atkinson Hyperlegible Mono (if bundled). Provide a
    # \AtkinsonMono{...} command that switches to the mono font. If the
    # mono font isn't present, fall back to \texttt to keep rendering safe.
    mono_regular = os.path.join(
        fonts_path, "AtkinsonHyperlegibleMono-VariableFont_wght.ttf"
    )
    mono_italic = os.path.join(
        fonts_path, "AtkinsonHyperlegibleMono-Italic-VariableFont_wght.ttf"
    )
    if os.path.exists(mono_regular):
        # Register new font family command using fontspec
        doc.preamble.append(
            NoEscape(
                rf"\newfontfamily\AtkinsonMono[Path={fonts_path + os.sep},Extension=.ttf,ItalicFont=AtkinsonHyperlegibleMono-Italic-VariableFont_wght,Scale=MatchLowercase]{{AtkinsonHyperlegibleMono-VariableFont_wght}}"
            )
        )
    else:
        # Simple fallback: AtkinsonMono -> \texttt
        doc.preamble.append(NoEscape(r"\newcommand{\AtkinsonMono}[1]{\texttt{#1}}"))

    # Document title
    with doc.create(Center()) as title_center:
        # Document title in sans-serif locally
        title_center.append(NoEscape(f"{{\\huge \\sffamily\\textbf{{ {location}}}}}"))
        title_center.append(LineBreak())
        title_center.append(NoEscape("\\vspace{0.1cm}"))
        title_center.append(
            NoEscape(
                "{\\large \\sffamily\\textit{Banshee, INC } \\textbullet \\ \\href{mailto:david@banshee-data.com}{david@banshee-data.com}}"
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
        add_metric_data_intro(
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

    add_science(doc)

    doc.append(NoEscape("\\vspace{0.5cm}"))

    # Statistics section
    doc.append(NoEscape("\\section*{Survey Parameters}"))

    # Generation parameters as a two-column table
    table = Tabular("ll")
    table.add_row(
        [NoEscape(r"\textbf{Radar Sensor:}"), NoEscape("OmniPreSense OPS243-A")]
    )
    table.add_row(
        [
            NoEscape(r"\textbf{Start time:}"),
            NoEscape(r"\AtkinsonMono{" + escape_latex(start_iso) + r"}"),
        ]
    )
    table.add_row(
        [
            NoEscape(r"\textbf{End time:}"),
            NoEscape(r"\AtkinsonMono{" + escape_latex(end_iso) + r"}"),
        ]
    )
    table.add_row(
        [NoEscape(r"\textbf{Timezone:}"), NoEscape(escape_latex(timezone_display))]
    )
    table.add_row([NoEscape(r"\textbf{Rollup period:}"), NoEscape(escape_latex(group))])
    table.add_row([NoEscape(r"\textbf{Units:}"), NoEscape(escape_latex(units))])
    table.add_row(
        [
            NoEscape(r"\textbf{Min speed (cutoff):}"),
            NoEscape(escape_latex(min_speed_str)),
        ]
    )
    doc.append(table)

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

    # Generate PDF: prefer XeLaTeX/LuaLaTeX (required by fontspec) then fall back
    # to pdfLaTeX as a last resort.
    engines = ("xelatex", "lualatex", "pdflatex")
    generated = False
    last_exc: Optional[Exception] = None
    for engine in engines:
        try:
            doc.generate_pdf(
                output_path.replace(".pdf", ""), clean_tex=False, compiler=engine
            )
            print(f"Generated PDF: {output_path} (engine={engine})")
            generated = True
            break
        except Exception as e:
            print(f"PDF generation with {engine} failed: {e}")
            last_exc = e

    if not generated:
        try:
            doc.generate_tex(output_path.replace(".pdf", ""))
            print(
                f"Generated TEX file for debugging: {output_path.replace('.pdf', '.tex')}"
            )
        except Exception as tex_e:
            print(f"Failed to generate TEX for debugging: {tex_e}")
        if last_exc:
            raise last_exc
