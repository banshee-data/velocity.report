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
                    NoEscape(r"\AtkinsonMono{" + escape_latex(str(int(cnt))) + r"}"),
                    NoEscape(
                        r"\AtkinsonMono{" + escape_latex(format_number(p50v)) + r"}"
                    ),
                    NoEscape(
                        r"\AtkinsonMono{" + escape_latex(format_number(p85v)) + r"}"
                    ),
                    NoEscape(
                        r"\AtkinsonMono{" + escape_latex(format_number(p98v)) + r"}"
                    ),
                    NoEscape(
                        r"\AtkinsonMono{" + escape_latex(format_number(maxv)) + r"}"
                    ),
                ]
            )
        else:
            table.add_row(
                [
                    NoEscape(r"\AtkinsonMono{" + escape_latex(str(int(cnt))) + r"}"),
                    NoEscape(
                        r"\AtkinsonMono{" + escape_latex(format_number(p50v)) + r"}"
                    ),
                    NoEscape(
                        r"\AtkinsonMono{" + escape_latex(format_number(p85v)) + r"}"
                    ),
                    NoEscape(
                        r"\AtkinsonMono{" + escape_latex(format_number(p98v)) + r"}"
                    ),
                    NoEscape(
                        r"\AtkinsonMono{" + escape_latex(format_number(maxv)) + r"}"
                    ),
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
    table.add_row(["Bucket", "Count", "Percent"])
    table.add_hline()

    # Cutoff row
    below_cutoff = sum(v for k, v in numeric_buckets.items() if k < cutoff)
    pct_below = (below_cutoff / total * 100.0) if total > 0 else 0.0
    table.add_row(
        [
            NoEscape(r"\AtkinsonMono{" + escape_latex(f"<{int(cutoff)}") + r"}"),
            NoEscape(r"\AtkinsonMono{" + escape_latex(str(int(below_cutoff))) + r"}"),
            NoEscape(r"\AtkinsonMono{" + escape_latex(f"{pct_below:.1f}%") + r"}"),
        ]
    )

    # Range rows
    for a, b in ranges:
        cnt = count_in_histogram_range(numeric_buckets, a, b)
        pct = (cnt / total * 100.0) if total > 0 else 0.0
        label = f"{int(a)}-{int(b)}"
        table.add_row(
            [
                NoEscape(r"\AtkinsonMono{" + escape_latex(label) + r"}"),
                NoEscape(r"\AtkinsonMono{" + escape_latex(str(int(cnt))) + r"}"),
                NoEscape(r"\AtkinsonMono{" + escape_latex(f"{pct:.1f}%") + r"}"),
            ]
        )

    # Final row
    cnt_ge = count_histogram_ge(numeric_buckets, max_bucket)
    pct_ge = (cnt_ge / total * 100.0) if total > 0 else 0.0
    table.add_row(
        [
            NoEscape(r"\AtkinsonMono{" + escape_latex(f"{int(max_bucket)}+") + r"}"),
            NoEscape(r"\AtkinsonMono{" + escape_latex(str(int(cnt_ge))) + r"}"),
            NoEscape(r"\AtkinsonMono{" + escape_latex(f"{pct_ge:.1f}%") + r"}"),
        ]
    )

    table.add_hline()
    centered.append(table)

    # Add caption
    centered.append(NoEscape("\\par\\vspace{2pt}"))
    centered.append(
        NoEscape(
            "\\noindent\\makebox[\\linewidth]{\\textbf{\\small Table 1: Histogram}}"
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
            f"velocity for \\textbf{{{total_vehicles}}} vehicles was recorded on \\textbf{{{location}}}."
        )
    )

    doc.append(NoEscape("\\subsection*{Key Metrics}"))
    table = Tabular("ll")
    table.add_row(
        [
            NoEscape(r"\textbf{Maximum Velocity:}"),
            NoEscape(r"\AtkinsonMono{" + escape_latex(f"{max_speed:.2f} mph") + r"}"),
        ]
    )
    table.add_row(
        [
            NoEscape(r"\textbf{98th Percentile Velocity (p98):}"),
            NoEscape(r"\AtkinsonMono{" + escape_latex(f"{p98:.2f} mph") + r"}"),
        ]
    )
    table.add_row(
        [
            NoEscape(r"\textbf{85th Percentile Velocity (p85):}"),
            NoEscape(r"\AtkinsonMono{" + escape_latex(f"{p85:.2f} mph") + r"}"),
        ]
    )
    table.add_row(
        [
            NoEscape(r"\textbf{Median Velocity (p50):}"),
            NoEscape(r"\AtkinsonMono{" + escape_latex(f"{p50:.2f} mph") + r"}"),
        ]
    )
    # Render this parameter table in sans-serif locally
    doc.append(NoEscape("\\sffamily"))
    doc.append(table)

    doc.append(NoEscape("\\par"))
    doc.append(NoEscape("\\par"))


def add_site_specifics(doc: Document) -> None:
    """Add site-specific information section to the document."""

    doc.append(NoEscape("\\subsection*{Site Information}"))

    doc.append(
        NoEscape(
            "This survey was conducted from the southbound parking lane outside 500 Clarendon Avenue. "
            "The posted speed limit at this location is 35 mph, with a reduced limit of 25 mph when school "
            "children are present. Data was collected over three consecutive days from a fixed position. "
            "The survey location sits directly in front of an elementary school and is positioned on a"
            "downhill grade, which may influence driver speed and braking behavior."
        )
    )
    doc.append(NoEscape("\\par"))
    doc.append(NoEscape("\\par"))


def add_science(doc: Document) -> None:

    doc.append(NoEscape("\\subsection*{Citizen Radar}"))

    doc.append(
        NoEscape(
            "velocity.report is a citizen radar tool designed to help communities "
            "measure vehicle speeds with affordable, privacy-preserving Doppler sensors. "
            "It's built on a core physical truth: kinetic energy scales with the square of speed."
        )
    )
    doc.append(NoEscape("\\par"))
    # Use paragraph breaks instead of LineBreak() which can insert
    # vertical-mode breaks and cause "There's no line here to end" errors.
    doc.append(NoEscape("\\par"))
    doc.append(NoEscape(r"\[ K_E = \tfrac{1}{2} m v^2 \]"))
    doc.append(NoEscape("\\par"))
    with doc.create(Center()) as centered:
        centered.append(
            NoEscape("where \\(m\\) is the mass and \\(v\\) is the velocity.")
        )
    doc.append(NoEscape("\\par"))
    doc.append(NoEscape("\\par"))
    doc.append(
        NoEscape(
            "A vehicle traveling at 40 mph has four times the crash energy of the same vehicle at 20 mph, "
            "posing exponentially greater risk to people outside the car. Even small increases in speed dramatically raise the likelihood of severe injury or death in a collision. "
            "By quantifying real-world vehicle speeds, velocity.report produces evidence that exceeds industry standard metrics."
        )
    )
    doc.append(NoEscape("\\par"))
    doc.append(NoEscape("\\par"))

    doc.append(NoEscape("\\subsection*{Aggregation and Percentiles}"))

    doc.append(
        NoEscape(
            "This system uses Doppler radar to measure vehicle speed by detecting frequency shifts caused by motion "
            "toward or away from the sensor. This shift (known as the \\href{https://en.wikipedia.org/wiki/Doppler_effect}{Doppler effect}) "
            "is directly proportional to the object's velocity relative to the sensor."
        )
    )
    doc.append(NoEscape("\\par"))
    doc.append(NoEscape("\\par"))
    doc.append(
        NoEscape(
            "To structure this data, the velocity.report application applies a greedy, local, "
            "univariate algorithm called \\emph{Time-Contiguous Speed Clustering}. In this survey, "
            "individual radar read lines were grouped into sessions based on time proximity and speed similarity. "
            "Each session, or “transit”, represents a short burst of movement consistent with a single "
            "passing object. This approach is efficient and reproducible, but not without limitations: "
            "in dense traffic or when objects overlap, it may undercount the number of vehicles by merging "
            "multiple objects into a single transit. This undercounting can bias percentile metrics (like p85 and p98), "
            "downward as since fewer sessions can give disproportionate weight to slower vehicles. "
            "All reported statistics in this report are derived from these sessionised transits."
        )
    )
    doc.append(NoEscape("\\par"))
    doc.append(NoEscape("\\par"))
    doc.append(
        NoEscape(
            "Percentiles offer a structured way to interpret speed behavior. The 85th percentile (p85) "
            "indicates the speed at or below which 85\\% of vehicles traveled; the 98th percentile (p98) "
            "highlights the fastest 2\\% of vehicle speeds while reducing the influence of extreme outliers. "
            "This helps identify high-risk driving patterns without letting single anomalous readings dominate. "
            "However, percentile values can be unstable in periods with low sample counts. To reflect this, "
            "our charts flag low-sample segments in orange and suppress percentile points when counts fall below "
            "reliability thresholds."
        )
    )
    doc.append(NoEscape("\\par"))


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
    # ensure common math macros like \tfrac are available
    doc.packages.append(Package("amsmath"))
    doc.packages.append(Package("titlesec"))
    doc.packages.append(Package("hyperref"))
    # microtype improves line breaking and protrusion which reduces awkward gaps
    # doc.packages.append(Package("microtype"))
    # ragged2e provides \RaggedRight (better raggedness with hyphenation)
    # doc.packages.append(Package("ragged2e"))
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
            rf"\setsansfont[Path={fonts_path + os.sep},Extension=.ttf,Scale=MatchLowercase,UprightFont=AtkinsonHyperlegible-Regular,ItalicFont=AtkinsonHyperlegible-Italic,BoldFont=AtkinsonHyperlegible-Bold,BoldItalicFont=AtkinsonHyperlegible-BoldItalic]{{AtkinsonHyperlegible-Regular}}"
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
        # Prefer a heavier weight for the monospace family by requesting
        # a higher wght axis value from the variable font via fontspec.
        # Use RawFeature to set the wght axis to 700 (bold-ish). If the
        # engine or font doesn't support it, fontspec will fall back.
        doc.preamble.append(
            NoEscape(
                rf"\newfontfamily\AtkinsonMono[Path={fonts_path + os.sep},Extension=.ttf,ItalicFont=AtkinsonHyperlegibleMono-Italic-VariableFont_wght,Scale=MatchLowercase,RawFeature={{+wght=800}}]{{AtkinsonHyperlegibleMono-VariableFont_wght}}"
            )
        )
    else:
        # Simple fallback: AtkinsonMono -> \texttt
        doc.preamble.append(NoEscape(r"\newcommand{\AtkinsonMono}[1]{\texttt{#1}}"))

    # Document title
    with doc.create(Center()) as title_center:
        # Document title in sans-serif locally
        title_center.append(NoEscape(f"{{\\huge \\sffamily\\textbf{{ {location}}}}}"))
        # Use \par for a safe paragraph break instead of LineBreak()
        title_center.append(NoEscape("\\par"))
        title_center.append(NoEscape("\\vspace{0.1cm}"))
        title_center.append(
            NoEscape(
                "{\\large \\sffamily\\textit{Banshee, INC } \\textbullet \\ \\href{mailto:david@banshee-data.com}{david@banshee-data.com}}"
            )
        )

    # Begin multicolumn layout and use ragged-right to avoid stretched justification
    doc.append(NoEscape("\\begin{multicols}{2}"))
    # Use ragged2e's \RaggedRight to prevent text from stretching to fill the column width
    # doc.append(NoEscape("\\RaggedRight"))
    # # Improve hyphenation and allow mild emergency stretching to avoid
    # # large whitespace at the right edge of ragged paragraphs.
    # # Tune hyphenation/line-breaking to reduce large ragged-right gaps
    # doc.append(NoEscape("\\hyphenpenalty=300"))
    # doc.append(NoEscape("\\exhyphenpenalty=50"))
    # # Allow a modest amount of emergency stretch so TeX can avoid bad breaks
    # doc.append(NoEscape("\\emergencystretch=3em"))
    # # Relax tolerance so hyphenation/expansion is preferred over huge gaps
    # doc.append(NoEscape("\\tolerance=1000"))

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

    add_site_specifics(doc)

    doc.append(NoEscape("\\par"))

    add_science(doc)

    # Small separation after the science section
    doc.append(NoEscape("\\par"))

    # Statistics section
    doc.append(NoEscape("\\section*{Survey Parameters}"))

    # Generation parameters as a two-column table
    table = Tabular("ll")
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
        [
            NoEscape(r"\textbf{Timezone:}"),
            NoEscape(r"\AtkinsonMono{" + escape_latex(timezone_display) + r"}"),
        ]
    )
    table.add_row(
        [
            NoEscape(r"\textbf{Rollup Period:}"),
            NoEscape(r"\AtkinsonMono{" + escape_latex(group) + r"}"),
        ]
    )
    table.add_row(
        [
            NoEscape(r"\textbf{Units:}"),
            NoEscape(r"\AtkinsonMono{" + escape_latex(units) + r"}"),
        ]
    )
    table.add_row(
        [
            NoEscape(r"\textbf{Minimum speed (cutoff):}"),
            NoEscape(r"\AtkinsonMono{" + escape_latex(min_speed_str) + r"}"),
        ]
    )
    table.add_row(
        [
            NoEscape(r"\textbf{Radar Sensor:}"),
            NoEscape(r"\AtkinsonMono{" + escape_latex("OmniPreSense OPS243-A") + r"}"),
        ]
    )
    table.add_row(
        [
            NoEscape(r"\textbf{Radar Firmware version:}"),
            NoEscape(r"\AtkinsonMono{" + escape_latex("v1.2.3") + r"}"),
        ]
    )
    table.add_row(
        [
            NoEscape(r"\textbf{Radar Transmit Frequency:}"),
            NoEscape(r"\AtkinsonMono{" + escape_latex("24.125 GHz") + r"}"),
        ]
    )
    table.add_row(
        [
            NoEscape(r"\textbf{Radar Sample Rate:}"),
            NoEscape(r"\AtkinsonMono{" + escape_latex("20 kSPS") + r"}"),
        ]
    )
    table.add_row(
        [
            NoEscape(r"\textbf{Radar Velocity Resolution:}"),
            NoEscape(r"\AtkinsonMono{" + escape_latex("0.272 mph") + r"}"),
        ]
    )
    table.add_row(
        [
            NoEscape(r"\textbf{Azimuth Field of View:}"),
            NoEscape(r"\AtkinsonMono{" + escape_latex("20°") + r"}"),
        ]
    )
    table.add_row(
        [
            NoEscape(r"\textbf{Elevation Field of View:}"),
            NoEscape(r"\AtkinsonMono{" + escape_latex("24°") + r"}"),
        ]
    )
    table.add_row(
        [
            NoEscape(r"\textbf{Cosine Error Angle:}"),
            NoEscape(r"\AtkinsonMono{" + escape_latex("21°") + r"}"),
        ]
    )
    table.add_row(
        [
            NoEscape(r"\textbf{Cosine Error Factor:}"),
            NoEscape(r"\AtkinsonMono{" + escape_latex("1.0711") + r"}"),
        ]
    )
    doc.append(table)

    # minimize vertical gap after science section
    doc.append(NoEscape("\\vspace{1pt}"))

    # Add tables
    # if overall_metrics:
    #     doc.append(
    #         create_stats_table(
    #             overall_metrics,
    #             tz_name,
    #             units,
    #             "Table 1: Overall Summary",
    #             include_start_time=False,
    #         )
    #     )

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

    # End multicolumn
    doc.append(NoEscape("\\end{multicols}"))

    if chart_exists(charts_prefix, "stats"):
        stats_path = os.path.abspath(f"{charts_prefix}_stats.pdf")
        with doc.create(Center()) as chart_center:
            with chart_center.create(Figure(position="H")) as fig:
                # use full available text width for charts
                fig.add_image(stats_path, width=NoEscape(r"\linewidth"))
                fig.add_caption("Velocity over time")

    # Add main stats chart if available
    # If a map.svg exists next to this module, include it before the stats chart.
    # Convert to PDF if needed using cairosvg (preferred) or external tools if available.
    map_svg = os.path.join(os.path.dirname(__file__), "map.svg")
    map_pdf = os.path.join(os.path.dirname(__file__), "map.pdf")
    if os.path.exists(map_svg):
        # convert if pdf doesn't yet exist or is older than svg
        try:
            need_convert = (not os.path.exists(map_pdf)) or (
                os.path.getmtime(map_svg) > os.path.getmtime(map_pdf)
            )
        except Exception:
            need_convert = not os.path.exists(map_pdf)

        if need_convert:
            converted = False
            # Try Python-based conversion first
            try:
                import importlib

                if importlib.util.find_spec("cairosvg") is not None:
                    from cairosvg import svg2pdf

                    with open(map_pdf, "wb") as out_f:
                        svg2pdf(url=map_svg, write_to=out_f)
                    converted = True
            except Exception:
                converted = False

            # Fallback to command-line tools if cairosvg not present
            if not converted:
                # Try inkscape
                try:
                    import subprocess

                    subprocess.check_call(["inkscape", "--version"])  # quick check
                    # inkscape CLI: inkscape input.svg --export-type=pdf --export-filename=out.pdf
                    subprocess.check_call(
                        [
                            "inkscape",
                            map_svg,
                            "--export-type=pdf",
                            "--export-filename",
                            map_pdf,
                        ]
                    )
                    converted = True
                except Exception:
                    converted = False

            if not converted:
                # Try rsvg-convert
                try:
                    import subprocess

                    subprocess.check_call(["rsvg-convert", "--version"])  # quick check
                    with open(map_pdf, "wb") as out_f:
                        subprocess.check_call(
                            [
                                "rsvg-convert",
                                "-f",
                                "pdf",
                                map_svg,
                            ],
                            stdout=out_f,
                        )
                    converted = True
                except Exception:
                    converted = False

            if not converted:
                print(
                    "Warning: map.svg found but failed to convert to PDF; skipping map inclusion"
                )

        # If map.pdf now exists, include it
        if os.path.exists(map_pdf):
            map_path = os.path.abspath(map_pdf)
            with doc.create(Center()) as map_center:
                with map_center.create(Figure(position="H")) as mf:
                    mf.add_image(map_path, width=NoEscape(r"\linewidth"))
                    mf.add_caption("Site map")

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
