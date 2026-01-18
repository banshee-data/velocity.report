#!/usr/bin/env python3
"""Report section builders for PDF generation.

This module handles all content section generation for velocity reports, including:
- Velocity overview with key metrics
- Site-specific information
- Science and methodology sections
- Survey parameters

The module is designed to work with PyLaTeX but is independent of the overall
document assembly logic, making sections reusable and testable.
"""


try:
    from pylatex import Document, NoEscape, Center
    from pylatex.utils import escape_latex

    HAVE_PYLATEX = True
except Exception:  # pragma: no cover
    HAVE_PYLATEX = False
    Document = None
    NoEscape = str
    Center = None

    def escape_latex(s: str) -> str:
        return s


import math
from typing import Optional

from pdf_generator.core.table_builders import (
    create_param_table,
    create_comparison_summary_table,
)


class VelocityOverviewSection:
    """Builds the velocity overview section with key metrics.

    Creates a section showing:
    - Study period and location
    - Total vehicle count
    - Key percentile metrics (p50, p85, p98, max)
    """

    def __init__(self):
        """Initialise velocity overview section builder."""
        if not HAVE_PYLATEX:
            raise ImportError(
                "PyLaTeX is required for section generation. "
                "Install it with: pip install pylatex"
            )

    def build(
        self,
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
        units: str = "mph",
        compare_start_date: Optional[str] = None,
        compare_end_date: Optional[str] = None,
        compare_total_vehicles: Optional[int] = None,
        compare_p50: Optional[float] = None,
        compare_p85: Optional[float] = None,
        compare_p98: Optional[float] = None,
        compare_max_speed: Optional[float] = None,
    ) -> None:
        """Add velocity overview section to document.

        Args:
            doc: PyLaTeX Document to append to
            start_date: Start date string (YYYY-MM-DD)
            end_date: End date string (YYYY-MM-DD)
            location: Location name
            speed_limit: Posted speed limit
            total_vehicles: Total number of vehicles detected
            p50: Median speed (50th percentile)
            p85: 85th percentile speed
            p98: 98th percentile speed
            max_speed: Maximum speed recorded
        """
        # Section heading
        doc.append(NoEscape("\\section*{Velocity Overview}"))

        # Format total vehicles with thousands separator for readability
        try:
            total_disp = f"{int(total_vehicles):,}"
        except Exception:
            total_disp = str(total_vehicles)

        # Overview paragraph
        if compare_start_date and compare_end_date:
            doc.append(
                NoEscape(
                    f"Primary period: \\textbf{{{start_date}}} to \\textbf{{{end_date}}} "
                    f"(\\textbf{{{escape_latex(total_disp)}}} vehicles) at \\textbf{{{escape_latex(location)}}}. "
                    f"Comparison period: \\textbf{{{compare_start_date}}} to \\textbf{{{compare_end_date}}}."
                )
            )
        else:
            doc.append(
                NoEscape(
                    f"Between \\textbf{{{start_date}}} and \\textbf{{{end_date}}}, "
                    f"velocity for \\textbf{{{escape_latex(total_disp)}}} vehicles was recorded on \\textbf{{{escape_latex(location)}}}."
                )
            )

        # Key metrics subsection
        doc.append(NoEscape("\\subsection*{Key Metrics}"))

        # Use parameter table for consistent formatting
        def format_speed(value: Optional[float]) -> str:
            if value is None:
                return "--"
            try:
                if math.isnan(float(value)):
                    return "--"
            except Exception:
                return "--"
            return f"{value:.2f} {units}"

        def format_count(value: Optional[int]) -> str:
            if value is None:
                return "--"
            try:
                return f"{int(value):,}"
            except Exception:
                return "--"

        def format_change(
            primary_value: Optional[float], compare_value: Optional[float]
        ) -> str:
            if (
                primary_value is None
                or compare_value is None
                or abs(primary_value) < 1e-9
            ):
                return "--"
            try:
                change_pct = (compare_value - primary_value) / primary_value * 100.0
                return f"{change_pct:+.1f}%"
            except Exception:
                return "--"

        if compare_start_date and compare_end_date:
            primary_label = f"{start_date} to {end_date}"
            compare_label = f"{compare_start_date} to {compare_end_date}"
            summary_entries = [
                {
                    "label": "Maximum Velocity",
                    "primary": format_speed(max_speed),
                    "compare": format_speed(compare_max_speed),
                    "change": format_change(max_speed, compare_max_speed),
                },
                {
                    "label": "98th Percentile Velocity (p98)",
                    "primary": format_speed(p98),
                    "compare": format_speed(compare_p98),
                    "change": format_change(p98, compare_p98),
                },
                {
                    "label": "85th Percentile Velocity (p85)",
                    "primary": format_speed(p85),
                    "compare": format_speed(compare_p85),
                    "change": format_change(p85, compare_p85),
                },
                {
                    "label": "Median Velocity (p50)",
                    "primary": format_speed(p50),
                    "compare": format_speed(compare_p50),
                    "change": format_change(p50, compare_p50),
                },
                {
                    "label": "Total Vehicles",
                    "primary": format_count(total_vehicles),
                    "compare": format_count(compare_total_vehicles),
                    "change": format_change(
                        float(total_vehicles),
                        (
                            float(compare_total_vehicles)
                            if compare_total_vehicles is not None
                            else None
                        ),
                    ),
                },
            ]
            doc.append(
                create_comparison_summary_table(
                    summary_entries, primary_label, compare_label
                )
            )
        else:
            key_metric_entries = [
                {"key": "Maximum Velocity", "value": format_speed(max_speed)},
                {"key": "98th Percentile Velocity (p98)", "value": format_speed(p98)},
                {"key": "85th Percentile Velocity (p85)", "value": format_speed(p85)},
                {"key": "Median Velocity (p50)", "value": format_speed(p50)},
            ]

            doc.append(create_param_table(key_metric_entries))
        doc.append(NoEscape("\\par"))


class SiteInformationSection:
    """Builds the site-specific information section.

    Includes:
    - Site description
    - Speed limit notes
    - Location-specific context
    """

    def __init__(self):
        """Initialise site information section builder."""
        if not HAVE_PYLATEX:
            raise ImportError(
                "PyLaTeX is required for section generation. "
                "Install it with: pip install pylatex"
            )

    def build(
        self, doc: Document, site_description: str = "", speed_limit_note: str = ""
    ) -> None:
        """Add site information section to document.

        Args:
            doc: PyLaTeX Document to append to
            site_description: Optional site description text
            speed_limit_note: Optional speed limit information
        """
        # Skip section if both are empty (don't use SITE_INFO fallback)
        if not site_description and not speed_limit_note:
            return

        doc.append(NoEscape("\\subsection*{Site Information}"))

        if site_description:
            doc.append(NoEscape(escape_latex(site_description)))
            doc.append(NoEscape("\\par"))

        if speed_limit_note:
            doc.append(NoEscape(escape_latex(speed_limit_note)))


class ScienceMethodologySection:
    """Builds the science and methodology section.

    Explains:
    - Kinetic energy and speed relationship
    - Doppler radar principles
    - Transit clustering algorithm
    - Percentile interpretation
    - Data reliability considerations
    """

    def __init__(self):
        """Initialise science section builder."""
        if not HAVE_PYLATEX:
            raise ImportError(
                "PyLaTeX is required for section generation. "
                "Install it with: pip install pylatex"
            )

    def build(self, doc: Document) -> None:
        """Add science and methodology section to document.

        Args:
            doc: PyLaTeX Document to append to
        """
        self._add_citizen_radar_intro(doc)
        self._add_aggregation_percentiles(doc)

    def _add_citizen_radar_intro(self, doc: Document) -> None:
        """Add citizen radar introduction with kinetic energy explanation."""
        doc.append(NoEscape("\\subsection*{Citizen Radar}"))

        doc.append(
            NoEscape(
                "\\href{https://velocity.report}{velocity.report} is a citizen radar tool designed to help communities "
                "measure vehicle speeds with affordable, privacy-preserving Doppler sensors. "
                "It's built on a core physical truth: kinetic energy scales with the square of speed."
            )
        )
        doc.append(NoEscape("\\par"))
        doc.append(NoEscape("\\par"))

        # Kinetic energy formula
        doc.append(NoEscape(r"\[ K_E = \tfrac{1}{2} m v^2 \]"))
        doc.append(NoEscape("\\par"))

        with doc.create(Center()) as centered:
            centered.append(
                NoEscape("where \\(m\\) is the mass and \\(v\\) is the velocity.")
            )
        doc.append(NoEscape("\\par"))

        # Safety implications
        doc.append(
            NoEscape(
                "A vehicle traveling at 40 mph has four times the crash energy of the same vehicle at 20 mph, "
                "posing exponentially greater risk to people outside the car. Even small increases in speed dramatically raise the likelihood of severe injury or death in a collision. "
                "By quantifying real-world vehicle speeds, \\href{https://velocity.report}{velocity.report} produces evidence that exceeds industry standard metrics."
            )
        )
        doc.append(NoEscape("\\par"))

    def _add_aggregation_percentiles(self, doc: Document) -> None:
        """Add aggregation and percentiles methodology explanation."""
        doc.append(NoEscape("\\subsection*{Aggregation and Percentiles}"))

        # Doppler radar explanation
        doc.append(
            NoEscape(
                "This system uses Doppler radar to measure vehicle speed by detecting frequency shifts in waves "
                "reflected from objects in motion. This shift (known as the \\href{https://en.wikipedia.org/wiki/Doppler_effect}{Doppler effect}) "
                "is directly proportional to the object's relative velocity. When the sensor is stationary, the Doppler effect "
                "reports the true speed of an object moving toward or away from the radar."
            )
        )
        doc.append(NoEscape("\\par"))

        # Transit clustering algorithm
        doc.append(
            NoEscape(
                "To structure this data, the \\href{https://velocity.report}{velocity.report} application first records individual "
                "radar readings, then applies a greedy, local, univariate \\emph{Time-Contiguous Speed Clustering} algorithm to "
                "group log lines into sessions based on time proximity and speed similarity. Each session, or ``transit,'' represents "
                "a short burst of movement consistent with a single passing object. This approach is efficient and reproducible, "
                "but in dense traffic or where objects overlap it may undercount vehicles by merging multiple objects into one transit."
            )
        )
        doc.append(NoEscape("\\par"))

        # Bias considerations
        doc.append(
            NoEscape(
                "Undercounting can bias percentile metrics (like p85 and p98) downward, since fewer sessions can give "
                "disproportionate weight to slower vehicles. All reported statistics in this report are derived from "
                "these sessionised transits."
            )
        )
        doc.append(NoEscape("\\par"))

        # Percentile interpretation
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

        # Data reliability
        doc.append(
            NoEscape(
                "However, percentile metrics can be unstable in periods with low sample counts. To reflect this, our charts "
                "flag low-sample segments in orange and suppress percentile points when counts fall below reliability thresholds "
                "(fewer than 50 samples per roll-up period)."
            )
        )
        doc.append(NoEscape("\\par"))


class SurveyParametersSection:
    """Builds the survey parameters section.

    Lists all technical parameters including:
    - Time range and timezone
    - Roll-up period and units
    - Minimum speed cutoff
    - Radar sensor specifications
    """

    def __init__(self):
        """Initialise survey parameters section builder."""
        if not HAVE_PYLATEX:
            raise ImportError(
                "PyLaTeX is required for section generation. "
                "Install it with: pip install pylatex"
            )

    def build(
        self,
        doc: Document,
        start_iso: str,
        end_iso: str,
        timezone_display: str,
        group: str,
        units: str,
        min_speed_str: str,
    ) -> None:
        """Add survey parameters section to document.

        Args:
            doc: PyLaTeX Document to append to
            start_iso: Start time in ISO format
            end_iso: End time in ISO format
            timezone_display: Timezone name for display
            group: Roll-up period (e.g., "1h", "15m")
            units: Speed units (e.g., "mph", "kph")
            min_speed_str: Minimum speed cutoff description
        """
        doc.append(NoEscape("\\subsection*{Survey Parameters}"))

        # Generation parameters as a two-column table
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


# =============================================================================
# Convenience Functions
# =============================================================================


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
    units: str = "mph",
    compare_start_date: Optional[str] = None,
    compare_end_date: Optional[str] = None,
    compare_total_vehicles: Optional[int] = None,
    compare_p50: Optional[float] = None,
    compare_p85: Optional[float] = None,
    compare_p98: Optional[float] = None,
    compare_max_speed: Optional[float] = None,
) -> None:
    """Add velocity overview section (convenience wrapper)."""
    builder = VelocityOverviewSection()
    builder.build(
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
        units,
        compare_start_date,
        compare_end_date,
        compare_total_vehicles,
        compare_p50,
        compare_p85,
        compare_p98,
        compare_max_speed,
    )


def add_site_specifics(
    doc: Document, site_description: str = "", speed_limit_note: str = ""
) -> None:
    """Add site information section (convenience wrapper).

    Args:
        doc: PyLaTeX Document to append to
        site_description: Optional site description text
        speed_limit_note: Optional speed limit information
    """
    builder = SiteInformationSection()
    builder.build(doc, site_description, speed_limit_note)


def add_science(doc: Document) -> None:
    """Add science and methodology section (convenience wrapper)."""
    builder = ScienceMethodologySection()
    builder.build(doc)


def add_survey_parameters(
    doc: Document,
    start_iso: str,
    end_iso: str,
    timezone_display: str,
    group: str,
    units: str,
    min_speed_str: str,
) -> None:
    """Add survey parameters section (convenience wrapper)."""
    builder = SurveyParametersSection()
    builder.build(
        doc, start_iso, end_iso, timezone_display, group, units, min_speed_str
    )
