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
from document_builder import DocumentBuilder
from table_builders import (
    create_stats_table,
    create_param_table,
    create_histogram_table,
    create_twocolumn_stats_table,
)
from report_sections import (
    add_metric_data_intro,
    add_site_specifics,
    add_science,
)


# Removed MultiCol class - using \twocolumn instead of multicols package
# Table building functions moved to table_builders.py
# Report section builders moved to report_sections.py


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

    # Calculate cosine error factor from angle
    import math

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
    if include_map:
        map_processor = MapProcessor(
            base_dir=os.path.dirname(__file__),
            marker_config={
                "circle_radius": MAP_CONFIG["circle_radius"],
                "circle_fill": MAP_CONFIG["circle_fill"],
                "circle_stroke": MAP_CONFIG["circle_stroke"],
                "circle_stroke_width": MAP_CONFIG["circle_stroke_width"],
            },
        )

        # Create radar marker from config (or None to skip marker)
        marker = None
        if MAP_CONFIG["triangle_len"] and MAP_CONFIG["triangle_len"] > 0:
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
