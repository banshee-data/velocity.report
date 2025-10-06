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

    # Build column specs programmatically so we can apply the monospace
    # font to numeric columns only (via >{\AtkinsonMono} in the column
    # spec) while keeping header cells in the document sans-serif.
    if include_start_time:
        # one text/time column + 5 numeric columns
        num_numeric = 5
        headers = [
            "Start Time",
            "Count",
            f"\\shortstack{{p50 \\\\ ({escape_latex(units)})}}",
            f"\\shortstack{{p85 \\\\ ({escape_latex(units)})}}",
            f"\\shortstack{{p98 \\\\ ({escape_latex(units)})}}",
            f"\\shortstack{{Max \\\\ ({escape_latex(units)})}}",
        ]
    else:
        num_numeric = 5
        headers = [
            "Count",
            f"\\shortstack{{p50 \\\\ ({escape_latex(units)})}}",
            f"\\shortstack{{p85 \\\\ ({escape_latex(units)})}}",
            f"\\shortstack{{p98 \\\\ ({escape_latex(units)})}}",
            f"\\shortstack{{Max \\\\ ({escape_latex(units)})}}",
        ]

    centered = Center()

    # Create a header-only table (sans-serif) so the header row is not
    # affected by the monospace selection applied to the numeric columns.
    if include_start_time:
        header_spec = "l" + "r" * num_numeric
    else:
        header_spec = "r" * num_numeric

    header_table = Tabular(header_spec)
    header_table.add_row([NoEscape(h) for h in headers])
    header_table.add_hline()
    centered.append(header_table)

    # Create the body table where numeric columns are rendered in the
    # Atkinson monospace family using the >{\AtkinsonMono} column prefix.
    if include_start_time:
        body_spec = "l" + (">{\\AtkinsonMono}r" * num_numeric)
    else:
        body_spec = ">{\\AtkinsonMono}r" * num_numeric

    table = Tabular(body_spec)

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
                    NoEscape(escape_latex(tstr)),
                    NoEscape(escape_latex(str(int(cnt)))),
                    NoEscape(escape_latex(format_number(p50v))),
                    NoEscape(escape_latex(format_number(p85v))),
                    NoEscape(escape_latex(format_number(p98v))),
                    NoEscape(escape_latex(format_number(maxv))),
                ]
            )
        else:
            table.add_row(
                [
                    NoEscape(escape_latex(str(int(cnt)))),
                    NoEscape(escape_latex(format_number(p50v))),
                    NoEscape(escape_latex(format_number(p85v))),
                    NoEscape(escape_latex(format_number(p98v))),
                    NoEscape(escape_latex(format_number(maxv))),
                ]
            )

    table.add_hline()
    centered.append(table)

    # Add caption (outside the mono group so caption styling remains consistent)
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

    # Header-only table (sans-serif)
    header_table = Tabular("lrr")
    header_table.add_row(["Bucket", "Count", "Percent"])
    header_table.add_hline()
    centered.append(header_table)

    # Body table: make numeric columns monospaced using >{\AtkinsonMono}
    body_table = Tabular("l>{\\AtkinsonMono}r>{\\AtkinsonMono}r")

    # Cutoff row
    below_cutoff = sum(v for k, v in numeric_buckets.items() if k < cutoff)
    pct_below = (below_cutoff / total * 100.0) if total > 0 else 0.0
    body_table.add_row(
        [
            NoEscape(escape_latex(f"<{int(cutoff)}")),
            NoEscape(escape_latex(str(int(below_cutoff)))),
            NoEscape(escape_latex(f"{pct_below:.1f}%")),
        ]
    )

    # Range rows
    for a, b in ranges:
        cnt = count_in_histogram_range(numeric_buckets, a, b)
        pct = (cnt / total * 100.0) if total > 0 else 0.0
        label = f"{int(a)}-{int(b)}"
        body_table.add_row(
            [
                NoEscape(escape_latex(label)),
                NoEscape(escape_latex(str(int(cnt)))),
                NoEscape(escape_latex(f"{pct:.1f}%")),
            ]
        )

    # Final row
    cnt_ge = count_histogram_ge(numeric_buckets, max_bucket)
    pct_ge = (cnt_ge / total * 100.0) if total > 0 else 0.0
    body_table.add_row(
        [
            NoEscape(escape_latex(f"{int(max_bucket)}+")),
            NoEscape(escape_latex(str(int(cnt_ge)))),
            NoEscape(escape_latex(f"{pct_ge:.1f}%")),
        ]
    )

    body_table.add_hline()
    centered.append(body_table)

    # Add caption
    centered.append(NoEscape("\\par\\vspace{2pt}"))
    centered.append(
        NoEscape(
            "\\noindent\\makebox[\\linewidth]{\\textbf{\\small Table 1: Velocity Distribution Data}}"
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
    # Use a two-column table where the second (numeric) column is
    # rendered in the Atkinson monospace font via the column spec. This
    # keeps the left-hand labels in the document sans-serif.
    table = Tabular("l>{\\AtkinsonMono}l")
    table.add_row(
        [
            NoEscape(r"\textbf{Maximum Velocity:}"),
            NoEscape(escape_latex(f"{max_speed:.2f} mph")),
        ]
    )
    table.add_row(
        [
            NoEscape(r"\textbf{98th Percentile Velocity (p98):}"),
            NoEscape(escape_latex(f"{p98:.2f} mph")),
        ]
    )
    table.add_row(
        [
            NoEscape(r"\textbf{85th Percentile Velocity (p85):}"),
            NoEscape(escape_latex(f"{p85:.2f} mph")),
        ]
    )
    table.add_row(
        [
            NoEscape(r"\textbf{Median Velocity (p50):}"),
            NoEscape(escape_latex(f"{p50:.2f} mph")),
        ]
    )

    # Append the table directly; global family remains sans as set in the
    # document preamble.
    doc.append(table)

    doc.append(NoEscape("\\par"))


def add_site_specifics(doc: Document) -> None:
    """Add site-specific information section to the document."""

    doc.append(NoEscape("\\subsection*{Site Information}"))

    doc.append(
        NoEscape(
            "This survey was conducted from the southbound parking lane outside 500 Clarendon Avenue, "
            "directly in front of an elementary school. The site is located on a downhill grade, which may "
            "influence vehicle speed and braking behavior. Data was collected from a fixed position over three consecutive days."
        )
    )
    doc.append(NoEscape("\\par"))

    doc.append(
        NoEscape(
            "The posted speed limit at this location is 35 mph, reduced to 25 mph when school children are present. "
        )
    )


def add_science(doc: Document) -> None:

    doc.append(NoEscape("\\subsection*{Citizen Radar}"))

    doc.append(
        NoEscape(
            "\\href{https://velocity.report}{velocity.report} is a citizen radar tool designed to help communities "
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
    doc.append(
        NoEscape(
            "A vehicle traveling at 40 mph has four times the crash energy of the same vehicle at 20 mph, "
            "posing exponentially greater risk to people outside the car. Even small increases in speed dramatically raise the likelihood of severe injury or death in a collision. "
            "By quantifying real-world vehicle speeds, \\href{https://velocity.report}{velocity.report} produces evidence that exceeds industry standard metrics."
        )
    )
    doc.append(NoEscape("\\par"))

    doc.append(NoEscape("\\subsection*{Aggregation and Percentiles}"))

    doc.append(
        NoEscape(
            "This system uses Doppler radar to measure vehicle speed by detecting frequency shifts in waves "
            "reflected from objects in motion. This shift (known as the \\href{https://en.wikipedia.org/wiki/Doppler_effect}{Doppler effect}) "
            "is directly proportional to the object's relative velocity. When the sensor is stationary, the Doppler effect "
            "reports the true speed of an object moving toward or away from the radar."
        )
    )
    doc.append(NoEscape("\\par"))
    doc.append(
        NoEscape(
            "To structure this data, the \\href{https://velocity.report}{velocity.report} application first records individual "
            "radar readings, then applies a greedy, local, univariate \\emph{Time-Contiguous Speed Clustering} algorithm to "
            "group log lines into sessions based on time proximity and speed similarity. Each session, or “transit,” represents "
            "a short burst of movement consistent with a single passing object. This approach is efficient and reproducible, "
            "but in dense traffic or where objects overlap it may undercount vehicles by merging multiple objects into one transit."
        )
    )
    doc.append(NoEscape("\\par"))
    doc.append(
        NoEscape(
            "Undercounting can bias percentile metrics (like p85 and p98) downward, since fewer sessions can give "
            "disproportionate weight to slower vehicles. All reported statistics in this report are derived from "
            "these sessionised transits."
        )
    )
    doc.append(NoEscape("\\par"))
    doc.append(
        NoEscape(
            "Percentiles offer a structured way to interpret speed behaviour. The 85th percentile (p85) indicates the "
            "speed at or below which 85\\% of vehicles traveled. The 98th percentile (p98) exceeds this "
            "industry-standard measure by capturing the fastest 2\\% of vehicle speeds, providing a more robust view "
            "into trends among top speeders. By extending beyond p85, p98 identifies an additional 13\\% of data that "
            "would otherwise be missed when trimming the top 15\\%, offering clearer insight into high-risk driving "
            "patterns without letting single anomalous readings dominate."
        )
    )
    doc.append(NoEscape("\\par"))
    doc.append(
        NoEscape(
            "However, percentile metrics can be unstable in periods with low sample counts. To reflect this, our charts "
            "flag low-sample segments in orange and suppress percentile points when counts fall below reliability thresholds "
            "(fewer than 50 samples per roll-up period)."
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
    # Make caption labels and text bold so figure/table captions match
    # the document's heading weight and appear visually prominent.
    doc.preamble.append(NoEscape("\\captionsetup{labelfont=bf,textfont=bf}"))
    # Use supertabular so long tables can span pages/columns
    doc.packages.append(Package("supertabular"))
    doc.packages.append(Package("float"))  # Required for H position
    # array provides >{...} and <{...} column spec modifiers used to
    # inject font switches into column definitions (e.g. >{\AtkinsonMono}r).
    doc.packages.append(Package("array"))

    # Set up header
    # Increase headheight to avoid fancyhdr warnings on some templates
    doc.append(NoEscape("\\setlength{\\headheight}{12pt}"))
    doc.append(NoEscape("\\setlength{\\headsep}{10pt}"))
    doc.append(NoEscape("\\pagestyle{fancy}"))
    doc.append(NoEscape("\\fancyhf{}"))
    doc.append(
        NoEscape(
            "\\fancyhead[L]{\\textbf{\\protect\\href{https://velocity.report}{velocity.report}}}"
        )
    )
    doc.append(NoEscape(f"\\fancyhead[C]{{ {start_iso[:10]} to {end_iso[:10]} }}"))
    doc.append(NoEscape(f"\\fancyhead[R]{{ \\textit{{{location}}}}}"))
    doc.append(NoEscape("\\renewcommand{\\headrulewidth}{0.8pt}"))

    # Title formatting: make section headings sans-serif (local style)
    doc.preamble.append(
        NoEscape("\\titleformat{\\section}{\\bfseries\\Large}{}{0em}{}")
    )
    # column gap (preamble): configurable via REPORT_COLUMNSEP_PT (points).
    # Default is increased to 10pt for better separation between columns.
    try:
        _colsep_env = os.getenv("REPORT_COLUMNSEP_PT", "14")
        _colsep_val = float(_colsep_env)
        # Format as 'Npt' (integer if whole number)
        if float(_colsep_val).is_integer():
            _colsep_str = f"{int(_colsep_val)}pt"
        else:
            _colsep_str = f"{_colsep_val}pt"
    except Exception:
        _colsep_str = "10pt"
    doc.preamble.append(NoEscape(f"\\setlength{{\\columnsep}}{{{_colsep_str}}}"))

    # Set the document's default family to sans-serif globally so all text
    # uses the sans font family without selecting a specific font.
    # Point fontspec at the bundled Atkinson Hyperlegible fonts that live in
    # the `fonts/` directory next to this module. This forces the document
    # to use Atkinson Hyperlegible as the sans font.
    fonts_path = os.path.join(os.path.dirname(__file__), "fonts")
    # Use Path= with trailing os.sep so fontspec can find the files
    # Break up the fontspec options for readability and maintainability
    sans_font_options = [
        f"Path={fonts_path + os.sep}",
        "Extension=.ttf",
        "Scale=MatchLowercase",
        "UprightFont=AtkinsonHyperlegible-Regular",
        "ItalicFont=AtkinsonHyperlegible-Italic",
        "BoldFont=AtkinsonHyperlegible-Bold",
        "BoldItalicFont=AtkinsonHyperlegible-BoldItalic",
    ]
    sans_font_options_str = ",\n    ".join(sans_font_options)
    doc.preamble.append(
        NoEscape(
            rf"\setsansfont[{sans_font_options_str}]{{AtkinsonHyperlegible-Regular}}"
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
        # Fallback: define \AtkinsonMono as a declarative font switch
        # so it can be used both in column-specs (>{\AtkinsonMono}) and
        # around braced content (\AtkinsonMono{...}). Using \ttfamily
        # keeps behavior acceptable when the bundled mono isn't present.
        doc.preamble.append(NoEscape(r"\newcommand{\AtkinsonMono}{\ttfamily}"))

    # Document title
    with doc.create(Center()) as title_center:
        # Document title in sans-serif locally
        title_center.append(NoEscape(f"{{\\huge \\sffamily\\textbf{{ {location}}}}}"))
        # Use \par for a safe paragraph break instead of LineBreak()
        title_center.append(NoEscape("\\par"))
        title_center.append(NoEscape("\\vspace{0.1cm}"))
        title_center.append(
            NoEscape(
                "{\\large \\sffamily Surveyor: \\textit{Banshee, INC.} \\ \\textbullet \\ \\ Contact: \\href{mailto:david@banshee-data.com}{david@banshee-data.com}}"
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

    # Add histogram chart if available
    if chart_exists(charts_prefix, "histogram"):
        hist_path = os.path.abspath(f"{charts_prefix}_histogram.pdf")
        with doc.create(Center()) as hist_chart_center:
            with hist_chart_center.create(Figure(position="H")) as fig:
                # use full available text width for histogram as well
                fig.add_image(hist_path, width=NoEscape(r"\linewidth"))
                fig.add_caption("Velocity Distribution Histogram")

    add_site_specifics(doc)

    doc.append(NoEscape("\\par"))

    add_science(doc)

    # Small separation after the science section
    doc.append(NoEscape("\\par"))

    # Statistics section
    doc.append(NoEscape("\\subsection*{Survey Parameters}"))

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
            NoEscape(r"\textbf{Roll-up Period:}"),
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

    doc.append(NoEscape("\\par"))

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

    # Add histogram table if available
    if histogram:
        doc.append(create_histogram_table(histogram, units))

    if daily_metrics:
        doc.append(
            create_stats_table(
                daily_metrics,
                tz_name,
                units,
                "Table 2: Daily Percentile Summary",
                include_start_time=True,
            )
        )

    if granular_metrics:
        doc.append(
            create_stats_table(
                granular_metrics,
                tz_name,
                units,
                "Table 3: Granular Percentile Breakdown",
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
        # Decide whether to inject a triangle marker. If MAP_TRIANGLE_LEN <= 0, skip marker.
        try:
            marker_len_frac = float(os.getenv("MAP_TRIANGLE_LEN", "0.42"))
        except Exception:
            marker_len_frac = 0.05

        # We'll produce a temporary SVG (map_with_marker.svg) if marker is requested.
        temp_svg = os.path.join(os.path.dirname(__file__), "map_with_marker.svg")
        source_svg_for_conversion = map_svg

        if marker_len_frac and marker_len_frac > 0:
            # Read original SVG and insert a top-layer polygon marker
            try:
                with open(map_svg, "r", encoding="utf-8") as f:
                    svg_text = f.read()

                import re, math

                vb_match = re.search(
                    r"viewBox\s*=\s*[\"']\s*([0-9.+-eE]+)\s+([0-9.+-eE]+)\s+([0-9.+-eE]+)\s+([0-9.+-eE]+)\s*[\"']",
                    svg_text,
                )
                if vb_match:
                    vb_min_x = float(vb_match.group(1))
                    vb_min_y = float(vb_match.group(2))
                    vb_w = float(vb_match.group(3))
                    vb_h = float(vb_match.group(4))
                else:
                    # fallback to width/height attributes
                    w_match = re.search(r"width\s*=\s*\"?([0-9.+-eE]+)", svg_text)
                    h_match = re.search(r"height\s*=\s*\"?([0-9.+-eE]+)", svg_text)
                    if w_match and h_match:
                        vb_min_x = 0.0
                        vb_min_y = 0.0
                        vb_w = float(w_match.group(1))
                        vb_h = float(h_match.group(1))
                    else:
                        raise RuntimeError(
                            "Unable to determine SVG viewBox/size for marker placement"
                        )

                # Tip position: allow overriding the tip (lower/point) position
                # via environment variables MAP_TRIANGLE_CX and MAP_TRIANGLE_CY
                # which are fractions 0..1 of the SVG viewBox width/height.
                try:
                    cx_frac = float(os.getenv("MAP_TRIANGLE_CX", "0.385"))
                except Exception:
                    cx_frac = 0.5
                try:
                    cy_frac = float(os.getenv("MAP_TRIANGLE_CY", "0.71"))
                except Exception:
                    cy_frac = 0.5

                cx = vb_min_x + vb_w * float(cx_frac)
                cy = vb_min_y + vb_h * float(cy_frac)

                # Length from tip toward the base (controls triangle size)
                L = marker_len_frac * vb_h

                # Compute base width so the apex angle equals MAP_TRIANGLE_APEX_ANGLE
                # (defaults to 20 degrees as requested). Apex angle (alpha) relates
                # to width W via: W = 2 * L * tan(alpha/2)
                try:
                    apex_deg = float(os.getenv("MAP_TRIANGLE_APEX_ANGLE", "20"))
                except Exception:
                    apex_deg = 20.0
                # clamp a tiny bit to avoid pathological values
                if apex_deg <= 0:
                    apex_deg = 1.0
                if apex_deg >= 179:
                    apex_deg = 178.0
                import math

                W = 2.0 * L * math.tan(math.radians(apex_deg / 2.0))

                # Orientation (direction the triangle points) remains configurable
                # via MAP_TRIANGLE_ANGLE (degrees, 0 = up). Default 0.
                try:
                    marker_angle_deg = float(os.getenv("MAP_TRIANGLE_ANGLE", "32"))
                except Exception:
                    marker_angle_deg = 0.0
                try:
                    marker_color = os.getenv("MAP_TRIANGLE_COLOR", "#f25f5c")
                except Exception:
                    marker_color = "#f25f5c"
                try:
                    marker_opacity = float(os.getenv("MAP_TRIANGLE_OPACITY", "0.9"))
                except Exception:
                    marker_opacity = 0.8

                theta = math.radians(marker_angle_deg)
                fx = math.sin(theta)
                fy = -math.cos(theta)
                px = math.cos(theta)
                py = math.sin(theta)

                bx = cx + fx * L
                by = cy + fy * L

                blx = bx + px * (W / 2.0)
                bly = by + py * (W / 2.0)
                brx = bx - px * (W / 2.0)
                bry = by - py * (W / 2.0)

                # Format coordinates to a consistent precision for SVG output
                points = f"{cx:.2f},{cy:.2f} {blx:.2f},{bly:.2f} {brx:.2f},{bry:.2f}"

                # Optional small circle marker at the triangle apex (first point)
                try:
                    circle_radius = float(os.getenv("MAP_TRIANGLE_CIRCLE_RADIUS", "20"))
                except Exception:
                    circle_radius = 6.0
                circle_fill = os.getenv("MAP_TRIANGLE_CIRCLE_FILL", "#ffffff")
                circle_stroke = os.getenv("MAP_TRIANGLE_CIRCLE_STROKE", marker_color)
                circle_stroke_width = os.getenv("MAP_TRIANGLE_CIRCLE_STROKE_WIDTH", "2")

                # Build an insertion snippet for the top-layer marker. Ensure
                # attributes are properly quoted (previously there was a stray
                # quote which produced invalid SVG and could be ignored by
                # converters). Use a visible stroke width and preserve opacity.
                insert_snippet = (
                    f"\n  <!-- radar marker inserted by pdf_generator.py -->\n"
                    f'  <g id="radar-marker" fill="{marker_color}" fill-opacity="{marker_opacity}" stroke="#ffffff" stroke-width="1">\n'
                    f'    <polygon points="{points}" />\n'
                    f'    <circle cx="{cx:.2f}" cy="{cy:.2f}" r="{circle_radius}" fill="{circle_fill}" stroke="{circle_stroke}" stroke-width="{circle_stroke_width}" />\n'
                    f"  </g>\n"
                )

                if svg_text.strip().endswith("</svg>"):
                    svg_text = svg_text.rstrip()[:-6] + insert_snippet + "</svg>"
                else:
                    svg_text = svg_text + insert_snippet

                with open(temp_svg, "w", encoding="utf-8") as tf:
                    tf.write(svg_text)

                # When we successfully wrote a temp SVG containing the
                # marker, make sure we convert it to PDF (force a
                # conversion) rather than re-using an existing map.pdf that
                # may not contain the overlay.
                source_svg_for_conversion = temp_svg
                need_convert = True
            except Exception as e:
                print(f"Warning: failed to create map_with_marker.svg: {e}")
                source_svg_for_conversion = map_svg

        if need_convert:
            converted = False
            # Try Python-based conversion first
            try:
                import importlib

                if importlib.util.find_spec("cairosvg") is not None:
                    from cairosvg import svg2pdf

                    with open(map_pdf, "wb") as out_f:
                        svg2pdf(url=source_svg_for_conversion, write_to=out_f)
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
                            source_svg_for_conversion,
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
                                source_svg_for_conversion,
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
                    mf.add_caption(
                        "Site map with radar location (circle) and coverage area (red triangle)"
                    )

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
