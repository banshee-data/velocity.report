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

    # Date range (REQUIRED)
    start_date: str = ""  # YYYY-MM-DD or unix timestamp
    end_date: str = ""  # YYYY-MM-DD or unix timestamp
    timezone: str = ""  # Timezone for display (REQUIRED, e.g., US/Pacific, UTC)

    # API parameters
    group: str = "1h"  # Time grouping (15m, 30m, 1h, 2h, 6h, 12h, 24h, all)
    units: str = "mph"  # Display units (mph, kph)
    source: str = "radar_data_transits"  # radar_objects or radar_data_transits
    model_version: str = "rebuild-full"  # Transit model version
    min_speed: Optional[float] = None  # Minimum speed filter (optional)

    # Histogram configuration (optional)
    histogram: bool = False  # Generate histogram (default: false)
    hist_bucket_size: Optional[float] = None  # Bucket size in display units
    hist_max: Optional[float] = None  # Maximum bucket value


@dataclass
class OutputConfig:
    """Output file configuration."""

    file_prefix: str = ""  # Output file prefix (REQUIRED - or auto-generated)
    output_dir: str = "."  # Output directory
    run_id: Optional[str] = None  # Unique run identifier (from Go server)
    debug: bool = False  # Enable debug output
    map: bool = False  # Include map in report (default: false, no map)


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

    def validate(self) -> tuple[bool, list[str]]:
        """Validate configuration.

        Returns:
            Tuple of (is_valid, error_messages)
        """
        errors = []

        # Validate query config (REQUIRED)
        if not self.query.start_date:
            errors.append("query.start_date is required")
        if not self.query.end_date:
            errors.append("query.end_date is required")
        if not self.query.timezone:
            errors.append("query.timezone is required")

        if self.query.histogram and not self.query.hist_bucket_size:
            errors.append("hist_bucket_size is required when histogram is enabled")

        if self.query.source not in ["radar_objects", "radar_data_transits"]:
            errors.append(f"Invalid source: {self.query.source}")

        if self.query.units not in ["mph", "kph"]:
            errors.append(f"Invalid units: {self.query.units}")

        # Validate site config (REQUIRED)
        if not self.site.location:
            errors.append("site.location is required")
        if not self.site.surveyor:
            errors.append("site.surveyor is required")
        if not self.site.contact:
            errors.append("site.contact is required")

        # Validate output config (REQUIRED)
        if not self.output.file_prefix:
            errors.append("output.file_prefix is required")

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
