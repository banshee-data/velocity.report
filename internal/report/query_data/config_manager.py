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


@dataclass
class SiteConfig:
    """Site-specific information and content."""

    location: str = "Clarendon Avenue, San Francisco"
    surveyor: str = "Banshee, INC."
    contact: str = "david@banshee-data.com"
    speed_limit: int = 25
    site_description: str = (
        "This survey was conducted from the southbound parking lane outside "
        "500 Clarendon Avenue, directly in front of an elementary school. "
        "The site is located on a downhill grade, which may influence vehicle "
        "speed and braking behavior. Data was collected from a fixed position "
        "over three consecutive days."
    )
    speed_limit_note: str = (
        "The posted speed limit at this location is 35 mph, reduced to 25 mph "
        "when school children are present."
    )

    # Map/location data (for future use)
    latitude: Optional[float] = None
    longitude: Optional[float] = None
    map_angle: Optional[float] = None


@dataclass
class RadarConfig:
    """Radar sensor technical parameters."""

    sensor_model: str = "OmniPreSense OPS243-A"
    firmware_version: str = "v1.2.3"
    transmit_frequency: str = "24.125 GHz"
    sample_rate: str = "20 kSPS"
    velocity_resolution: str = "0.272 mph"
    azimuth_fov: str = "20°"
    elevation_fov: str = "24°"
    cosine_error_angle: str = "21°"
    cosine_error_factor: str = "1.0711"


@dataclass
class QueryConfig:
    """API query and data processing parameters."""

    # Date range
    start_date: str = ""  # YYYY-MM-DD or unix timestamp
    end_date: str = ""  # YYYY-MM-DD or unix timestamp

    # API parameters
    group: str = "1h"  # Time grouping (15m, 30m, 1h, 2h, 6h, 12h, 24h, all)
    units: str = "mph"  # Display units (mph, kph)
    source: str = "radar_data_transits"  # radar_objects or radar_data_transits
    model_version: str = "rebuild-full"  # Transit model version
    timezone: str = "US/Pacific"  # Timezone for display
    min_speed: Optional[float] = 5.0  # Minimum speed filter

    # Histogram configuration
    histogram: bool = True  # Generate histogram
    hist_bucket_size: float = 5.0  # Bucket size in display units
    hist_max: Optional[float] = 50.0  # Maximum bucket value


@dataclass
class OutputConfig:
    """Output file configuration."""

    file_prefix: str = ""  # Output file prefix (auto-generated if empty)
    output_dir: str = "."  # Output directory
    run_id: Optional[str] = None  # Unique run identifier (from Go server)
    debug: bool = False  # Enable debug output
    no_map: bool = False  # Skip map generation (when location/GPS not available)


@dataclass
class ReportConfig:
    """Complete report configuration."""

    site: SiteConfig = field(default_factory=SiteConfig)
    radar: RadarConfig = field(default_factory=RadarConfig)
    query: QueryConfig = field(default_factory=QueryConfig)
    output: OutputConfig = field(default_factory=OutputConfig)

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
        # Extract nested configs
        site_data = data.get("site", {})
        radar_data = data.get("radar", {})
        query_data = data.get("query", {})
        output_data = data.get("output", {})

        return cls(
            site=SiteConfig(**site_data),
            radar=RadarConfig(**radar_data),
            query=QueryConfig(**query_data),
            output=OutputConfig(**output_data),
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

    @classmethod
    def from_cli_args(cls, args) -> "ReportConfig":
        """Create config from CLI arguments (argparse.Namespace).

        Args:
            args: Parsed CLI arguments

        Returns:
            ReportConfig instance
        """
        # Import here to avoid circular dependency
        from report_config import SITE_INFO

        # Handle dates - args.dates is a list like ['2024-01-01', '2024-01-31']
        start_date = ""
        end_date = ""
        if hasattr(args, "dates") and args.dates and len(args.dates) >= 2:
            start_date = args.dates[0]
            end_date = args.dates[1]

        return cls(
            site=SiteConfig(
                location=SITE_INFO.get("location", ""),
                surveyor=SITE_INFO.get("surveyor", ""),
                contact=SITE_INFO.get("contact", ""),
                speed_limit=SITE_INFO.get("speed_limit", 25),
                site_description=SITE_INFO.get("site_description", ""),
                speed_limit_note=SITE_INFO.get("speed_limit_note", ""),
            ),
            radar=RadarConfig(),  # Uses defaults
            query=QueryConfig(
                start_date=start_date,
                end_date=end_date,
                group=getattr(args, "group", "1h"),
                units=getattr(args, "units", "mph"),
                source=getattr(args, "source", "radar_data_transits"),
                model_version=getattr(args, "model_version", "rebuild-full"),
                timezone=getattr(args, "timezone", "US/Pacific") or "US/Pacific",
                min_speed=getattr(args, "min_speed", None),
                histogram=getattr(args, "histogram", False),
                hist_bucket_size=getattr(args, "hist_bucket_size", 5.0),
                hist_max=getattr(args, "hist_max", None),
            ),
            output=OutputConfig(
                file_prefix=getattr(args, "file_prefix", ""),
                debug=getattr(args, "debug", False),
                no_map=getattr(args, "no_map", False),
            ),
        )

    @classmethod
    def from_env(cls) -> "ReportConfig":
        """Create config from environment variables.

        This provides fallback defaults from environment.

        Returns:
            ReportConfig instance
        """
        return cls(
            site=SiteConfig(
                location=os.getenv(
                    "REPORT_LOCATION", "Clarendon Avenue, San Francisco"
                ),
                surveyor=os.getenv("REPORT_SURVEYOR", "Banshee, INC."),
                contact=os.getenv("REPORT_CONTACT", "david@banshee-data.com"),
                speed_limit=int(os.getenv("REPORT_SPEED_LIMIT", "25")),
                latitude=(
                    float(os.getenv("REPORT_LATITUDE"))
                    if os.getenv("REPORT_LATITUDE")
                    else None
                ),
                longitude=(
                    float(os.getenv("REPORT_LONGITUDE"))
                    if os.getenv("REPORT_LONGITUDE")
                    else None
                ),
                map_angle=(
                    float(os.getenv("REPORT_MAP_ANGLE"))
                    if os.getenv("REPORT_MAP_ANGLE")
                    else None
                ),
            ),
            radar=RadarConfig(
                sensor_model=os.getenv("RADAR_MODEL", "OmniPreSense OPS243-A"),
                firmware_version=os.getenv("RADAR_FIRMWARE", "v1.2.3"),
            ),
            query=QueryConfig(
                timezone=os.getenv("REPORT_TIMEZONE", "US/Pacific"),
                min_speed=(
                    float(os.getenv("REPORT_MIN_SPEED"))
                    if os.getenv("REPORT_MIN_SPEED")
                    else 5.0
                ),
            ),
            output=OutputConfig(
                output_dir=os.getenv("REPORT_OUTPUT_DIR", "."),
                debug=os.getenv("REPORT_DEBUG", "0") == "1",
            ),
        )

    def merge_with_env(self) -> "ReportConfig":
        """Merge current config with environment variable overrides.

        Returns:
            New ReportConfig instance with env overrides applied
        """
        env_config = self.from_env()

        # Create new config with env overrides where values are set
        return ReportConfig(
            site=SiteConfig(
                location=os.getenv("REPORT_LOCATION", self.site.location),
                surveyor=os.getenv("REPORT_SURVEYOR", self.site.surveyor),
                contact=os.getenv("REPORT_CONTACT", self.site.contact),
                speed_limit=int(
                    os.getenv("REPORT_SPEED_LIMIT", str(self.site.speed_limit))
                ),
                site_description=self.site.site_description,
                speed_limit_note=self.site.speed_limit_note,
                latitude=self.site.latitude,
                longitude=self.site.longitude,
                map_angle=self.site.map_angle,
            ),
            radar=self.radar,
            query=QueryConfig(
                start_date=self.query.start_date,
                end_date=self.query.end_date,
                group=self.query.group,
                units=self.query.units,
                source=self.query.source,
                model_version=self.query.model_version,
                timezone=os.getenv("REPORT_TIMEZONE", self.query.timezone),
                min_speed=(
                    float(os.getenv("REPORT_MIN_SPEED"))
                    if os.getenv("REPORT_MIN_SPEED")
                    else self.query.min_speed
                ),
                histogram=self.query.histogram,
                hist_bucket_size=self.query.hist_bucket_size,
                hist_max=self.query.hist_max,
            ),
            output=OutputConfig(
                file_prefix=self.output.file_prefix,
                output_dir=os.getenv("REPORT_OUTPUT_DIR", self.output.output_dir),
                run_id=self.output.run_id,
                debug=os.getenv("REPORT_DEBUG", "1" if self.output.debug else "0")
                == "1",
            ),
            created_at=self.created_at,
            updated_at=datetime.now(ZoneInfo("UTC")).isoformat(),
            version=self.version,
        )

    def validate(self) -> tuple[bool, list[str]]:
        """Validate configuration.

        Returns:
            Tuple of (is_valid, error_messages)
        """
        errors = []

        # Validate query config
        if not self.query.start_date:
            errors.append("start_date is required")
        if not self.query.end_date:
            errors.append("end_date is required")

        if self.query.histogram and not self.query.hist_bucket_size:
            errors.append("hist_bucket_size is required when histogram is enabled")

        if self.query.source not in ["radar_objects", "radar_data_transits"]:
            errors.append(f"Invalid source: {self.query.source}")

        if self.query.units not in ["mph", "kph"]:
            errors.append(f"Invalid units: {self.query.units}")

        # Validate site config
        if not self.site.location:
            errors.append("site.location is required")

        return len(errors) == 0, errors


def load_config(
    config_file: Optional[str] = None,
    cli_args=None,
    merge_env: bool = True,
) -> ReportConfig:
    """Load configuration from file, CLI args, or environment.

    Priority order (highest to lowest):
    1. Config file (if provided)
    2. CLI arguments (if provided)
    3. Environment variables
    4. Defaults

    Args:
        config_file: Path to JSON config file (optional)
        cli_args: Parsed CLI arguments (optional)
        merge_env: Whether to merge environment variable overrides

    Returns:
        ReportConfig instance
    """
    config = None

    # Load from file if provided
    if config_file and os.path.exists(config_file):
        config = ReportConfig.from_json(config_file)

    # Load from CLI args if provided
    elif cli_args:
        config = ReportConfig.from_cli_args(cli_args)

    # Fall back to environment
    else:
        config = ReportConfig.from_env()

    # Merge environment variable overrides if requested
    if merge_env:
        config = config.merge_with_env()

    return config


# Example usage and template generation
def create_example_config(output_path: str = "report_config_example.json") -> None:
    """Create an example configuration file.

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
        ),
    )

    config.to_json(output_path)
    print(f"Example configuration written to: {output_path}")


if __name__ == "__main__":
    # Generate example config when run directly
    create_example_config()
