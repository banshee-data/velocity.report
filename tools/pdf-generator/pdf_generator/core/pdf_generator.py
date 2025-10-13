#!/usr/bin/env python3

"""PDF report generation using PyLaTeX.

This module replaces the custom LaTeX generator with PyLaTeX to create
complete PDF reports including statistics tables, charts, and science sections.
"""

import os
import math

from pathlib import Path

from typing import Any, Dict, List, Optional


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
    )
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


from pdf_generator.core.stats_utils import chart_exists
from pdf_generator.core.data_transformers import (
    MetricsNormalizer,
    extract_count_from_row,
)
from pdf_generator.core.map_utils import MapProcessor, create_marker_from_config
from pdf_generator.core.document_builder import DocumentBuilder
from pdf_generator.core.table_builders import (
    create_param_table,
    create_histogram_table,
    create_twocolumn_stats_table,
)
from pdf_generator.core.report_sections import (
    add_metric_data_intro,
    add_site_specifics,
    add_science,
)
from pdf_generator.core.config_manager import DEFAULT_MAP_CONFIG, _map_to_dict


# Removed MultiCol class - using \twocolumn instead of multicols package
# Table building functions moved to table_builders.py
# Report section builders moved to report_sections.py


def _read_latex_log_excerpt(base_path: Path) -> list[str]:
    """Collect important lines from the LaTeX log for troubleshooting."""

    log_path = base_path.with_suffix(".log")
    if not log_path.exists():
        return []

    try:
        raw_lines = log_path.read_text(errors="ignore").splitlines()
    except Exception:
        return []

    excerpt: list[str] = []
    for line in raw_lines:
        stripped = line.strip()
        if stripped.startswith("!") or "Fatal error" in stripped:
            excerpt.append(stripped)
        elif excerpt and (stripped.startswith("l.") or stripped.startswith("See the")):
            excerpt.append(stripped)
        if len(excerpt) >= 6:
            break
    return excerpt


def _suggest_latex_fixes(engine: str, message: str, excerpt: list[str]) -> list[str]:
    """Derive actionable hints based on the error message and log excerpt."""

    hints: list[str] = []
    lower_message = message.lower()
    combined_text = " ".join(excerpt).lower()

    if (
        isinstance(message, str)
        and "not found" in lower_message
        and engine
        in (
            "xelatex",
            "lualatex",
            "pdflatex",
        )
    ):
        hints.append(
            "The LaTeX engine '{}' is missing. Install TeX Live or MacTeX (macOS) or `sudo apt-get install texlive-xetex`.".format(
                engine
            )
        )

    if "fontspec" in combined_text or "fontspec" in lower_message:
        hints.append(
            "Missing `fontspec` package. Install a full TeX distribution (texlive-full or mactex) or add the package manually."
        )

    if "atkinson" in combined_text:
        hints.append(
            "Atkinson fonts not found. Ensure the fonts/ directory is present in pdf_generator/core or disable map fonts."
        )

    if "file '" in combined_text and ".ttf'" in combined_text:
        hints.append(
            "Font files referenced in the log are missing. Confirm the fonts directory is copied alongside the executable."
        )

    if "undefined control sequence" in combined_text:
        hints.append(
            "LaTeX reported an undefined command. Check recent template edits or review the generated .tex file for typos."
        )

    if not hints:
        hints.append(
            "Inspect the generated .tex and .log files for precise errors. Common fixes include installing XeLaTeX and required fonts."
        )

    # Deduplicate while preserving order
    seen = set()
    deduped: list[str] = []
    for hint in hints:
        if hint not in seen:
            deduped.append(hint)
            seen.add(hint)
    return deduped


def _explain_latex_failure(engine: str, base_path: Path, exc: Exception) -> str:
    """Create a human-friendly explanation for LaTeX build failures."""

    message = str(exc)
    excerpt = _read_latex_log_excerpt(base_path)
    hints = _suggest_latex_fixes(engine, message, excerpt)

    bullet_excerpt = (
        "\n".join(f"    {line}" for line in excerpt)
        if excerpt
        else "    (log excerpt unavailable)"
    )

    details = [
        f"LaTeX compilation with {engine} failed.",
        "Log excerpt:",
        bullet_excerpt,
        "Suggested fixes:",
    ]
    details.extend(f"  - {hint}" for hint in hints)
    if message:
        details.append(f"  - Underlying error: {message}")
    details.append(f"  - Log file: {base_path.with_suffix('.log')}")
    details.append(f"  - TeX file: {base_path.with_suffix('.tex')}")
    return "\n".join(details)


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
    include_map: bool = True,
    site_description: str = "",
    speed_limit_note: str = "",
    surveyor: str = "",
    contact: str = "",
    cosine_error_angle: float = 0.0,
    sensor_model: str = "OmniPreSense OPS243-A",
    firmware_version: str = "v1.2.3",
    transmit_frequency: str = "24.125 GHz",
    sample_rate: str = "20 kSPS",
    velocity_resolution: str = "0.272 mph",
    azimuth_fov: str = "20°",
    elevation_fov: str = "24°",
) -> None:
    """Generate a complete PDF report using PyLaTeX.

    Args:
        include_map: If False, skip map generation even if map.svg exists
        site_description: Optional site description text
        speed_limit_note: Optional speed limit information
        surveyor: Surveyor name/organization
        contact: Contact email/phone
        cosine_error_angle: Radar mounting angle in degrees
        sensor_model: Radar sensor model
        firmware_version: Radar firmware version
        transmit_frequency: Radar transmit frequency
        sample_rate: Radar sample rate
        velocity_resolution: Radar velocity resolution
        azimuth_fov: Radar azimuth field of view
        elevation_fov: Radar elevation field of view
    """

    # Convert map config dataclass to dict for use in this function
    map_config_dict = _map_to_dict(DEFAULT_MAP_CONFIG)

    # Calculate cosine error factor from angle
    cosine_error_factor = 1.0
    if cosine_error_angle != 0:
        angle_rad = math.radians(cosine_error_angle)
        cosine_error_factor = 1.0 / math.cos(angle_rad)

    # Build document with all configuration
    builder = DocumentBuilder()
    doc = builder.build(start_iso, end_iso, location, surveyor, contact)

    # Add science section content using helper function
    if overall_metrics:
        overall = overall_metrics[0]

        # Use normalizer for consistent field extraction
        normalizer = MetricsNormalizer()
        p50 = normalizer.get_numeric(overall, "p50", 0)
        p85 = normalizer.get_numeric(overall, "p85", 0)
        p98 = normalizer.get_numeric(overall, "p98", 0)
        max_speed = normalizer.get_numeric(overall, "max_speed", 0)
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
        # Use basename only to avoid path escaping issues in LaTeX
        hist_path = os.path.basename(f"{charts_prefix}_histogram.pdf")
        with doc.create(Center()) as hist_chart_center:
            with hist_chart_center.create(Figure(position="H")) as fig:
                # use full available text width for histogram as well
                fig.add_image(hist_path, width=NoEscape(r"\linewidth"))
                fig.add_caption("Velocity Distribution Histogram")

    doc.append(NoEscape("\\vspace{-28pt}"))

    add_site_specifics(doc, site_description, speed_limit_note)

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
        {"key": "Radar Sensor", "value": sensor_model},
        {"key": "Radar Firmware version", "value": firmware_version},
        {"key": "Radar Transmit Frequency", "value": transmit_frequency},
        {"key": "Radar Sample Rate", "value": sample_rate},
        {"key": "Radar Velocity Resolution", "value": velocity_resolution},
        {"key": "Azimuth Field of View", "value": azimuth_fov},
        {"key": "Elevation Field of View", "value": elevation_fov},
        {"key": "Cosine Error Angle", "value": f"{cosine_error_angle}°"},
        {"key": "Cosine Error Factor", "value": f"{cosine_error_factor:.4f}"},
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
        # Use basename only to avoid path escaping issues in LaTeX
        stats_path = os.path.basename(f"{charts_prefix}_stats.pdf")
        with doc.create(Center()) as chart_center:
            with chart_center.create(Figure(position="H")) as fig:
                # use full available text width for charts
                fig.add_image(stats_path, width=NoEscape(r"\linewidth"))
                fig.add_caption("Velocity over time")

    # Add main stats chart if available
    # If a map.svg exists next to this module, include it before the stats chart.
    # Use map_utils module for marker injection and PDF conversion.
    # Skip map if include_map=False (e.g., when --no-map flag is used)

    print("\n=== MAP GENERATION DEBUG ===")
    print(f"include_map parameter: {include_map}")

    if include_map:
        print("Map generation ENABLED")
        print("Map config:")
        print(f"  - circle_radius: {map_config_dict['circle_radius']}")
        print(f"  - circle_fill: {map_config_dict['circle_fill']}")
        print(f"  - circle_stroke: {map_config_dict['circle_stroke']}")
        print(f"  - triangle_len: {map_config_dict['triangle_len']}")
        print(f"  - triangle_cx: {map_config_dict['triangle_cx']}")
        print(f"  - triangle_cy: {map_config_dict['triangle_cy']}")
        print(f"  - triangle_angle: {map_config_dict['triangle_angle']}")
        print(f"  - triangle_color: {map_config_dict['triangle_color']}")
        print(f"  - triangle_opacity: {map_config_dict['triangle_opacity']}")

        map_processor = MapProcessor(
            base_dir=os.path.dirname(__file__),
            marker_config={
                "circle_radius": map_config_dict["circle_radius"],
                "circle_fill": map_config_dict["circle_fill"],
                "circle_stroke": map_config_dict["circle_stroke"],
                "circle_stroke_width": map_config_dict["circle_stroke_width"],
            },
        )

        # Create radar marker from config (or None to skip marker)
        marker = None
        if map_config_dict["triangle_len"] and map_config_dict["triangle_len"] > 0:
            print(
                f"Creating radar marker (triangle_len={map_config_dict['triangle_len']} > 0)"
            )
            marker = create_marker_from_config(map_config_dict)
            print(f"Marker created: {marker is not None}")
        else:
            print(
                f"Skipping marker creation (triangle_len={map_config_dict['triangle_len']} <= 0)"
            )

        # Process map (adds marker if provided, converts to PDF)
        print(f"Processing map (marker={'provided' if marker else 'None'})...")
        success, map_pdf_path = map_processor.process_map(marker=marker)
        print(f"Map processing result: success={success}, path={map_pdf_path}")

        # If map PDF was generated, include it in the document
        if success and map_pdf_path:
            print(f"✓ Including map in document: {map_pdf_path}")
            with doc.create(Center()) as map_center:
                with map_center.create(Figure(position="H")) as mf:
                    mf.add_image(map_pdf_path, width=NoEscape(r"\linewidth"))
                    mf.add_caption(
                        "Site map with radar location (circle) and coverage area (red triangle)"
                    )
        else:
            print(
                f"✗ Map NOT included (success={success}, path exists={map_pdf_path is not None})"
            )
    else:
        print("Map generation DISABLED (include_map=False)")
    print("=== END MAP DEBUG ===\n")

    engines = ("xelatex", "lualatex", "pdflatex")
    generated = False
    last_exc: Optional[Exception] = None
    last_failure_message = ""
    base_prefix_path = Path(output_path).with_suffix("")
    for engine in engines:
        try:
            doc.generate_pdf(
                output_path.replace(".pdf", ""), clean_tex=False, compiler=engine
            )
            print(f"Generated PDF: {output_path} (engine={engine})")
            generated = True
            break
        except Exception as e:
            failure_details = _explain_latex_failure(engine, base_prefix_path, e)
            print(failure_details)
            last_exc = e
            last_failure_message = failure_details

    if not generated:
        try:
            doc.generate_tex(output_path.replace(".pdf", ""))
            print(
                f"Generated TEX file for debugging: {output_path.replace('.pdf', '.tex')}"
            )
        except Exception as tex_e:
            print(f"Failed to generate TEX for debugging: {tex_e}")
        if last_exc:
            raise RuntimeError(last_failure_message or str(last_exc)) from last_exc
