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


from stats_utils import chart_exists
from report_config import PDF_CONFIG, MAP_CONFIG, SITE_INFO
from data_transformers import MetricsNormalizer, extract_count_from_row
from map_utils import MapProcessor, create_marker_from_config
from table_builders import (
    create_stats_table,
    create_param_table,
    create_histogram_table,
    create_twocolumn_stats_table,
)


# Removed MultiCol class - using \twocolumn instead of multicols package
# Table building functions moved to table_builders.py


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

    # Format total vehicles with thousands separator for readability
    try:
        total_disp = f"{int(total_vehicles):,}"
    except Exception:
        total_disp = str(total_vehicles)
    doc.append(
        NoEscape(
            f"Between \\textbf{{{start_date}}} and \\textbf{{{end_date}}}, "
            f"velocity for \\textbf{{{escape_latex(total_disp)}}} vehicles was recorded on \\textbf{{{escape_latex(location)}}}."
        )
    )

    doc.append(NoEscape("\\subsection*{Key Metrics}"))
    # Use the DRY helper to render the key metrics as a two-column
    # parameter table so the formatting matches the rest of the
    # generation-parameters block (bold label + mono-formatted value).
    key_metric_entries = [
        {"key": "Maximum Velocity", "value": f"{max_speed:.2f} mph"},
        {"key": "98th Percentile Velocity (p98)", "value": f"{p98:.2f} mph"},
        {"key": "85th Percentile Velocity (p85)", "value": f"{p85:.2f} mph"},
        {"key": "Median Velocity (p50)", "value": f"{p50:.2f} mph"},
    ]

    doc.append(create_param_table(key_metric_entries))

    doc.append(NoEscape("\\par"))


def add_site_specifics(doc: Document) -> None:
    """Add site-specific information section to the document."""

    doc.append(NoEscape("\\subsection*{Site Information}"))

    doc.append(
        NoEscape(
            escape_latex(SITE_INFO['site_description'])
        )
    )
    doc.append(NoEscape("\\par"))

    doc.append(
        NoEscape(
            escape_latex(SITE_INFO['speed_limit_note'])
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
    hist_max: Optional[float] = None,
) -> None:
    """Generate a complete PDF report using PyLaTeX."""

    # Create document with very tight geometry so stats occupy most of the page
    geometry_options = PDF_CONFIG['geometry']

    doc = Document(geometry_options=geometry_options, page_numbers=False)

    # Add required packages (not using multicol - using \twocolumn instead)
    doc.packages.append(Package("fancyhdr"))
    doc.packages.append(Package("graphicx"))
    # ensure common math macros like \tfrac are available
    doc.packages.append(Package("amsmath"))
    doc.packages.append(Package("titlesec"))
    doc.packages.append(Package("hyperref"))
    # Load fontspec so we can use the bundled Atkinson Hyperlegible TTFs.
    doc.packages.append(Package("fontspec"))
    # Ensure captions use sans-serif
    doc.packages.append(Package("caption", options="font=sf"))
    # Make caption labels and text bold so figure/table captions match
    # the document's heading weight and appear visually prominent.
    doc.preamble.append(NoEscape("\\captionsetup{labelfont=bf,textfont=bf}"))
    # Use supertabular for tables that can break across columns with \twocolumn
    doc.packages.append(Package("supertabular"))
    doc.packages.append(Package("float"))  # Required for H position
    # array provides >{...} and <{...} column spec modifiers used to
    # inject font switches into column definitions (e.g. >{\AtkinsonMono}r).
    doc.packages.append(Package("array"))

    # Set up header
    # Increase headheight to avoid fancyhdr warnings on some templates
    doc.append(NoEscape(f"\\setlength{{\\headheight}}{{{PDF_CONFIG['headheight']}}}"))
    doc.append(NoEscape(f"\\setlength{{\\headsep}}{{{PDF_CONFIG['headsep']}}}"))
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
        _colsep_str = PDF_CONFIG['columnsep']
        if _colsep_str and not _colsep_str.endswith('pt'):
            _colsep_str = f"{_colsep_str}pt"
    except Exception:
        _colsep_str = "10pt"
    doc.preamble.append(NoEscape(f"\\setlength{{\\columnsep}}{{{_colsep_str}}}"))

    # Set the document's default family to sans-serif globally so all text
    # uses the sans font family without selecting a specific font.
    # Point fontspec at the bundled Atkinson Hyperlegible fonts that live in
    # the `fonts/` directory next to this module. This forces the document
    # to use Atkinson Hyperlegible as the sans font.
    fonts_path = os.path.join(os.path.dirname(__file__), PDF_CONFIG['fonts_dir'])
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

    # Begin two-column layout using \twocolumn with header in optional argument
    # This allows supertabular to work properly and keeps header on same page
    # The optional argument to \twocolumn creates a spanning header above the columns
    doc.append(NoEscape("\\twocolumn["))
    doc.append(NoEscape("\\vspace{-8pt}"))
    doc.append(NoEscape("\\begin{center}"))
    doc.append(NoEscape(f"{{\\huge \\sffamily\\textbf{{ {location}}}}}"))
    doc.append(NoEscape("\\par"))
    doc.append(NoEscape("\\vspace{0.1cm}"))
    doc.append(
        NoEscape(
            f"{{\\large \\sffamily Surveyor: \\textit{{{SITE_INFO['surveyor']}}} \\ \\textbullet \\ \\ Contact: \\href{{mailto:{SITE_INFO['contact']}}}{{{SITE_INFO['contact']}}}}}"
        )
    )
    doc.append(NoEscape("\\end{center}"))
    doc.append(NoEscape("]"))

    # Add science section content using helper function
    if overall_metrics:
        overall = overall_metrics[0]

        # Use normalizer for consistent field extraction
        normalizer = MetricsNormalizer()
        p50 = normalizer.get_numeric(overall, 'p50', 0)
        p85 = normalizer.get_numeric(overall, 'p85', 0)
        p98 = normalizer.get_numeric(overall, 'p98', 0)
        max_speed = normalizer.get_numeric(overall, 'max_speed', 0)
        total_vehicles = extract_count_from_row(overall, normalizer)

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

    doc.append(NoEscape("\\vspace{-28pt}"))

    add_site_specifics(doc)

    doc.append(NoEscape("\\par"))

    add_science(doc)

    # Small separation after the science section
    doc.append(NoEscape("\\par"))

    # Statistics section
    doc.append(NoEscape("\\subsection*{Survey Parameters}"))

    # Generation parameters as a two-column table (simplified)
    param_entries = [
        {"key": "Start time", "value": start_iso},
        {"key": "End time", "value": end_iso},
        {"key": "Timezone", "value": timezone_display},
        {"key": "Roll-up Period", "value": group},
        {"key": "Units", "value": units},
        {"key": "Minimum speed (cutoff)", "value": min_speed_str},
        {"key": "Radar Sensor", "value": "OmniPreSense OPS243-A"},
        {"key": "Radar Firmware version", "value": "v1.2.3"},
        {"key": "Radar Transmit Frequency", "value": "24.125 GHz"},
        {"key": "Radar Sample Rate", "value": "20 kSPS"},
        {"key": "Radar Velocity Resolution", "value": "0.272 mph"},
        {"key": "Azimuth Field of View", "value": "20°"},
        {"key": "Elevation Field of View", "value": "24°"},
        {"key": "Cosine Error Angle", "value": "21°"},
        {"key": "Cosine Error Factor", "value": "1.0711"},
    ]
    doc.append(create_param_table(param_entries))

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
        doc.append(create_histogram_table(histogram, units, max_bucket=hist_max))

    if daily_metrics:
        # Use supertabular for all daily tables (works with \twocolumn)
        create_twocolumn_stats_table(
            doc,
            daily_metrics,
            tz_name,
            units,
            "Table 2: Daily Percentile Summary",
        )

    if granular_metrics:
        # Use supertabular for granular tables (works with \twocolumn)
        create_twocolumn_stats_table(
            doc,
            granular_metrics,
            tz_name,
            units,
            "Table 3: Granular Percentile Breakdown",
        )

    # Switch back to single column for full-width charts
    doc.append(NoEscape("\\onecolumn"))

    if chart_exists(charts_prefix, "stats"):
        stats_path = os.path.abspath(f"{charts_prefix}_stats.pdf")
        with doc.create(Center()) as chart_center:
            with chart_center.create(Figure(position="H")) as fig:
                # use full available text width for charts
                fig.add_image(stats_path, width=NoEscape(r"\linewidth"))
                fig.add_caption("Velocity over time")

    # Add main stats chart if available
    # If a map.svg exists next to this module, include it before the stats chart.
    # Use map_utils module for marker injection and PDF conversion.
    map_processor = MapProcessor(
        base_dir=os.path.dirname(__file__),
        marker_config={
            "circle_radius": MAP_CONFIG['circle_radius'],
            "circle_fill": MAP_CONFIG['circle_fill'],
            "circle_stroke": MAP_CONFIG['circle_stroke'],
            "circle_stroke_width": MAP_CONFIG['circle_stroke_width'],
        }
    )

    # Create radar marker from config (or None to skip marker)
    marker = None
    if MAP_CONFIG['triangle_len'] and MAP_CONFIG['triangle_len'] > 0:
        marker = create_marker_from_config(MAP_CONFIG)

    # Process map (adds marker if provided, converts to PDF)
    success, map_pdf_path = map_processor.process_map(marker=marker)

    # If map PDF was generated, include it in the document
    if success and map_pdf_path:
        with doc.create(Center()) as map_center:
            with map_center.create(Figure(position="H")) as mf:
                mf.add_image(map_pdf_path, width=NoEscape(r"\linewidth"))
                mf.add_caption(
                    "Site map with radar location (circle) and coverage area (red triangle)"
                )


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
