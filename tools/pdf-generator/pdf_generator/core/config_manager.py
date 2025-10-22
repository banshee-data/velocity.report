#!/usr/bin/env python3
"""Configuration management for report generation.

This module provides a unified configuration system that supports:
1. CLI arguments (via get_stats.py)
2. JSON configuration files (for web/Go server integration)
3. Environment variable overrides
4. Validation and defaults

The workflow:
- Go server captures form data and writes config.json
- Python app loads config from JSON or CLI args
- Report is generated and files are transferred
- Go server provides download links via Svelte UI
"""

import json
import os
from typing import Any, Dict, Optional
from datetime import datetime
from zoneinfo import ZoneInfo
from dataclasses import dataclass, field, asdict


VALID_SOURCES = {"radar_objects", "radar_data_transits"}
VALID_UNITS = {"mph", "kph", "mps"}


@dataclass
class SiteConfig:
    """Site-specific information and content."""

    # REQUIRED fields
    location: str = ""  # Survey location (REQUIRED)
    surveyor: str = ""  # Surveyor name/organization (REQUIRED)
    contact: str = ""  # Contact email/phone (REQUIRED)

    # Optional fields
    speed_limit: int = 25
    site_description: str = ""
    speed_limit_note: str = ""

    # Map/location data (optional)
    latitude: Optional[float] = None
    longitude: Optional[float] = None
    map_angle: Optional[float] = None


@dataclass
class RadarConfig:
    """Radar sensor technical parameters."""

    # REQUIRED field
    cosine_error_angle: Optional[float] = None  # Mounting angle in degrees (REQUIRED)

    # Optional technical parameters
    sensor_model: str = "OmniPreSense OPS243-A"
    firmware_version: str = "v1.2.3"
    transmit_frequency: str = "24.125 GHz"
    sample_rate: str = "20 kSPS"
    velocity_resolution: str = "0.272 mph"
    azimuth_fov: str = "20°"
    elevation_fov: str = "24°"

    @property
    def cosine_error_factor(self) -> float:
        """Calculate cosine error factor from angle: 1/cos(angle_degrees)."""
        import math

        if self.cosine_error_angle is None or self.cosine_error_angle == 0:
            return 1.0
        angle_rad = math.radians(self.cosine_error_angle)
        return 1.0 / math.cos(angle_rad)


@dataclass
class QueryConfig:
    """API query and data processing parameters."""

    # Date range (REQUIRED)
    start_date: str = ""  # YYYY-MM-DD or unix timestamp
    end_date: str = ""  # YYYY-MM-DD or unix timestamp
    timezone: str = ""  # Timezone for display (REQUIRED, e.g., US/Pacific, UTC)

    # API parameters
    group: str = "1h"  # Time grouping (15m, 30m, 1h, 2h, 6h, 12h, 24h, all)
    units: str = "mph"  # Display units (mph, kph)
    source: str = "radar_data_transits"  # radar_objects or radar_data_transits
    model_version: str = "rebuild-full"  # Transit model version
    min_speed: Optional[float] = 5.0  # Minimum speed filter (default: 5.0)

    # Histogram configuration
    histogram: bool = True  # Generate histogram (default: true)
    hist_bucket_size: Optional[float] = (
        5.0  # Bucket size in display units (default: 5.0)
    )
    hist_max: Optional[float] = None  # Maximum bucket value


@dataclass
class OutputConfig:
    """Output file configuration."""

    file_prefix: Optional[str] = (
        None  # Output file prefix (None = auto-generate from date range)
    )
    output_dir: str = "."  # Output directory
    run_id: Optional[str] = None  # Unique run identifier (from Go server)
    debug: bool = False  # Enable debug output
    map: bool = False  # Include map in report (default: false, no map)


@dataclass
class ColorConfig:
    """Color palette for charts and reports."""

    p50: str = "#fbd92f"  # Yellow - 50th percentile
    p85: str = "#f7b32b"  # Orange - 85th percentile
    p98: str = "#f25f5c"  # Red/Pink - 98th percentile
    max: str = "#2d1e2f"  # Dark purple/black - maximum
    count_bar: str = "#2d1e2f"  # Gray bars for counts
    low_sample: str = "#f7b32b"  # Orange highlight for low-sample periods


@dataclass
class FontConfig:
    """Font sizes for charts and documents."""

    chart_title: int = 14
    chart_label: int = 13
    chart_tick: int = 11
    chart_axis_label: int = 8
    chart_axis_tick: int = 7
    chart_legend: int = 7
    histogram_title: int = 14
    histogram_label: int = 13
    histogram_tick: int = 11


@dataclass
class LayoutConfig:
    """Layout dimensions and constraints."""

    # Figure dimensions (stored as separate width/height for JSON serialization)
    chart_figsize_width: float = 24.0
    chart_figsize_height: float = 8.0
    histogram_figsize_width: float = 3.0
    histogram_figsize_height: float = 2.0

    # Thresholds
    low_sample_threshold: int = 50
    count_missing_threshold: int = 5

    # Bar chart widths (as fractions of bucket spacing)
    bar_width_bg_fraction: float = 0.95
    bar_width_fraction: float = 0.7

    # Chart sizing constraints
    min_chart_width_in: float = 6.0
    max_chart_width_in: float = 11.0

    # Chart margins and spacing (for fig.subplots_adjust)
    chart_left: float = 0.02
    chart_right: float = 0.96
    chart_top: float = 0.995
    chart_bottom: float = 0.16

    # Y-axis scaling
    count_axis_scale: float = 1.6

    # Line and marker styling
    line_width: float = 1.0
    marker_size: int = 4
    marker_edge_width: float = 0.4

    @property
    def chart_figsize(self) -> tuple:
        """Get chart figure size as tuple for matplotlib."""
        return (self.chart_figsize_width, self.chart_figsize_height)

    @property
    def histogram_figsize(self) -> tuple:
        """Get histogram figure size as tuple for matplotlib."""
        return (self.histogram_figsize_width, self.histogram_figsize_height)


@dataclass
class PdfConfig:
    """PDF/LaTeX document settings."""

    geometry_top: str = "1.8cm"
    geometry_bottom: str = "1.0cm"
    geometry_left: str = "1.0cm"
    geometry_right: str = "1.0cm"
    columnsep: str = "14"  # Points
    headheight: str = "12pt"
    headsep: str = "10pt"
    fonts_dir: str = "fonts"

    @property
    def geometry(self) -> Dict[str, str]:
        """Get geometry as dictionary for backward compatibility."""
        return {
            "top": self.geometry_top,
            "bottom": self.geometry_bottom,
            "left": self.geometry_left,
            "right": self.geometry_right,
        }


@dataclass
class MapConfig:
    """SVG map marker configuration."""

    # Triangle marker properties
    triangle_len: float = 0.42
    triangle_cx: float = 0.385
    triangle_cy: float = 0.71
    triangle_apex_angle: float = 20.0
    triangle_angle: float = 32.0
    triangle_color: str = "#f25f5c"
    triangle_opacity: float = 0.9

    # Circle marker at triangle apex
    circle_radius: float = 20.0
    circle_fill: str = "#ffffff"
    circle_stroke: Optional[str] = None  # Defaults to triangle_color
    circle_stroke_width: str = "2"

    def __post_init__(self):
        """Compute circle_stroke default to match triangle_color."""
        if self.circle_stroke is None:
            self.circle_stroke = self.triangle_color


@dataclass
class HistogramProcessingConfig:
    """Histogram processing defaults."""

    default_cutoff: float = 5.0
    default_bucket_size: float = 5.0
    default_max_bucket: float = 50.0


@dataclass
class DebugConfig:
    """Debug settings."""

    plot_debug: bool = False


@dataclass
class ReportConfig:
    """Complete report configuration."""

    site: SiteConfig = field(default_factory=SiteConfig)
    radar: RadarConfig = field(default_factory=RadarConfig)
    query: QueryConfig = field(default_factory=QueryConfig)
    output: OutputConfig = field(default_factory=OutputConfig)

    # Visual/presentation configuration
    colors: ColorConfig = field(default_factory=ColorConfig)
    fonts: FontConfig = field(default_factory=FontConfig)
    layout: LayoutConfig = field(default_factory=LayoutConfig)
    pdf: PdfConfig = field(default_factory=PdfConfig)
    map: MapConfig = field(default_factory=MapConfig)
    histogram_processing: HistogramProcessingConfig = field(
        default_factory=HistogramProcessingConfig
    )
    debug: DebugConfig = field(default_factory=DebugConfig)

    # Metadata
    created_at: Optional[str] = None
    updated_at: Optional[str] = None
    version: str = "1.0"

    def __post_init__(self):
        """Initialize timestamp if not provided."""
        if self.created_at is None:
            self.created_at = datetime.now(ZoneInfo("UTC")).isoformat()

    def to_dict(self) -> Dict[str, Any]:
        """Convert config to dictionary."""
        return asdict(self)

    def to_json(self, filepath: str, indent: int = 2) -> None:
        """Save configuration to JSON file.

        Args:
            filepath: Path to JSON file
            indent: JSON indentation level
        """
        with open(filepath, "w") as f:
            json.dump(self.to_dict(), f, indent=indent)

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "ReportConfig":
        """Create config from dictionary.

        Args:
            data: Configuration dictionary

        Returns:
            ReportConfig instance
        """

        # Extract nested configs and filter out documentation fields (starting with _)
        def filter_meta(d: dict) -> dict:
            """Remove keys starting with _ (documentation fields)."""
            return {k: v for k, v in d.items() if not k.startswith("_")}

        site_data = filter_meta(data.get("site", {}))
        radar_data = filter_meta(data.get("radar", {}))
        query_data = filter_meta(data.get("query", {}))
        output_data = filter_meta(data.get("output", {}))
        colors_data = filter_meta(data.get("colors", {}))
        fonts_data = filter_meta(data.get("fonts", {}))
        layout_data = filter_meta(data.get("layout", {}))
        pdf_data = filter_meta(data.get("pdf", {}))
        map_data = filter_meta(data.get("map", {}))
        histogram_processing_data = filter_meta(data.get("histogram_processing", {}))
        debug_data = filter_meta(data.get("debug", {}))

        return cls(
            site=SiteConfig(**site_data),
            radar=RadarConfig(**radar_data),
            query=QueryConfig(**query_data),
            output=OutputConfig(**output_data),
            colors=ColorConfig(**colors_data),
            fonts=FontConfig(**fonts_data),
            layout=LayoutConfig(**layout_data),
            pdf=PdfConfig(**pdf_data),
            map=MapConfig(**map_data),
            histogram_processing=HistogramProcessingConfig(**histogram_processing_data),
            debug=DebugConfig(**debug_data),
            created_at=data.get("created_at"),
            updated_at=data.get("updated_at"),
            version=data.get("version", "1.0"),
        )

    @classmethod
    def from_json(cls, filepath: str) -> "ReportConfig":
        """Load configuration from JSON file.

        Args:
            filepath: Path to JSON file

        Returns:
            ReportConfig instance
        """
        with open(filepath, "r") as f:
            data = json.load(f)
        return cls.from_dict(data)

    def validate(self) -> tuple[bool, list[str]]:
        """Validate configuration.

        Returns:
            Tuple of (is_valid, error_messages)
        """
        errors = []

        # Validate query config (REQUIRED)
        if not self.query.start_date:
            errors.append(
                "query.start_date is required (set to YYYY-MM-DD or unix seconds)."
            )
        if not self.query.end_date:
            errors.append(
                "query.end_date is required (set to YYYY-MM-DD or unix seconds)."
            )
        if not self.query.timezone:
            errors.append(
                "query.timezone is required (example: US/Pacific, UTC, Europe/Berlin)."
            )

        if self.query.histogram and not self.query.hist_bucket_size:
            errors.append(
                "query.hist_bucket_size must be set when histogram=true (e.g., 5.0 for 5 mph buckets)."
            )

        if self.query.source not in VALID_SOURCES:
            errors.append(
                "query.source must be one of {options}. Got '{value}'.".format(
                    options=", ".join(sorted(VALID_SOURCES)), value=self.query.source
                )
            )

        if self.query.units not in VALID_UNITS:
            errors.append(
                "query.units must be one of {options}. Got '{value}'.".format(
                    options=", ".join(sorted(VALID_UNITS)), value=self.query.units
                )
            )

        # Validate site config (REQUIRED)
        if not self.site.location:
            errors.append(
                "site.location is required (e.g., '123 Main St, Springfield')."
            )
        if not self.site.surveyor:
            errors.append(
                "site.surveyor is required (organization or survey team responsible)."
            )
        if not self.site.contact:
            errors.append(
                "site.contact is required (email or phone for follow-up questions)."
            )

        # Validate radar config (REQUIRED)
        if self.radar.cosine_error_angle is None:
            errors.append(
                "radar.cosine_error_angle is required (mounting angle in degrees, e.g., 21.0)."
            )

        return len(errors) == 0, errors


def load_config(
    config_file: Optional[str] = None,
    config_dict: Optional[Dict[str, Any]] = None,
) -> ReportConfig:
    """Load configuration from file or dictionary.

    Args:
        config_file: Path to JSON config file
        config_dict: Configuration dictionary (alternative to file)

    Returns:
        ReportConfig instance

    Raises:
        ValueError: If neither file nor dict provided, or if file doesn't exist
    """
    if config_file:
        if not os.path.exists(config_file):
            raise ValueError(f"Config file not found: {config_file}")
        return ReportConfig.from_json(config_file)
    elif config_dict:
        return ReportConfig.from_dict(config_dict)
    else:
        raise ValueError("Must provide either config_file or config_dict")


# Example usage and template generation
def create_example_config(output_path: str = "report_config_example.json") -> None:
    """Create an example configuration file with all sections populated.

    Args:
        output_path: Path to write example config
    """
    config = ReportConfig(
        site=SiteConfig(
            location="Clarendon Avenue, San Francisco",
            surveyor="Banshee, INC.",
            contact="david@banshee-data.com",
            speed_limit=25,
            latitude=37.7749,
            longitude=-122.4194,
            map_angle=32.0,
        ),
        radar=RadarConfig(
            cosine_error_angle=15.0,
            sensor_model="OmniPreSense OPS243-A",
            firmware_version="v1.2.3",
        ),
        query=QueryConfig(
            start_date="2025-06-02",
            end_date="2025-06-04",
            group="1h",
            units="mph",
            timezone="US/Pacific",
            min_speed=5.0,
            histogram=True,
            hist_bucket_size=5.0,
            hist_max=50.0,
        ),
        output=OutputConfig(
            file_prefix="test-report",
            output_dir="/var/reports",
            run_id="run-20250610-123456",
            debug=False,
            map=True,
        ),
        # Visual configs use defaults - no need to specify them
        # colors, fonts, layout, pdf, map, histogram_processing, debug all use defaults
    )

    config.to_json(output_path)
    print(f"Example configuration written to: {output_path}")


# =============================================================================
# Default Configuration Instances
# These are the single source of truth for all default values.
# Other modules should import and use these instances instead of duplicating values.
# =============================================================================

DEFAULT_SITE_CONFIG = SiteConfig()
DEFAULT_RADAR_CONFIG = RadarConfig()
DEFAULT_QUERY_CONFIG = QueryConfig()
DEFAULT_OUTPUT_CONFIG = OutputConfig()
DEFAULT_COLOR_CONFIG = ColorConfig()
DEFAULT_FONT_CONFIG = FontConfig()
DEFAULT_LAYOUT_CONFIG = LayoutConfig()
DEFAULT_PDF_CONFIG = PdfConfig()
DEFAULT_MAP_CONFIG = MapConfig()
DEFAULT_HISTOGRAM_PROCESSING_CONFIG = HistogramProcessingConfig()
DEFAULT_DEBUG_CONFIG = DebugConfig()


# Helper functions to convert dataclass instances to dictionaries
# These provide backward compatibility with code expecting dict access
def _colors_to_dict(config: ColorConfig) -> Dict[str, str]:
    """Convert ColorConfig to dictionary."""
    return asdict(config)


def _fonts_to_dict(config: FontConfig) -> Dict[str, int]:
    """Convert FontConfig to dictionary."""
    return asdict(config)


def _layout_to_dict(config: LayoutConfig) -> Dict[str, Any]:
    """Convert LayoutConfig to dictionary with tuple figsize."""
    d = asdict(config)
    # Convert separate width/height back to tuple for backward compatibility
    d["chart_figsize"] = config.chart_figsize
    d["histogram_figsize"] = config.histogram_figsize
    return d


def _pdf_to_dict(config: PdfConfig) -> Dict[str, Any]:
    """Convert PdfConfig to dictionary with geometry dict."""
    d = asdict(config)
    d["geometry"] = config.geometry
    return d


def _map_to_dict(config: MapConfig) -> Dict[str, Any]:
    """Convert MapConfig to dictionary."""
    return asdict(config)


def _histogram_processing_to_dict(
    config: HistogramProcessingConfig,
) -> Dict[str, float]:
    """Convert HistogramProcessingConfig to dictionary."""
    return asdict(config)


def _debug_to_dict(config: DebugConfig) -> Dict[str, bool]:
    """Convert DebugConfig to dictionary."""
    return asdict(config)


def _site_to_dict(config: SiteConfig) -> Dict[str, Any]:
    """Convert SiteConfig to dictionary."""
    return asdict(config)


if __name__ == "__main__":
    # Generate example config when run directly
    create_example_config()
