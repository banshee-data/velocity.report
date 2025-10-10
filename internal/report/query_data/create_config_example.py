#!/usr/bin/env python3
"""Generate example configuration file with all available options.

This creates a template config.json file that shows all configuration
options with comments explaining their purpose.
"""

import json
from config_manager import (
    ReportConfig,
    SiteConfig,
    QueryConfig,
    OutputConfig,
    RadarConfig,
)


def create_example_config(output_path: str = "config.example.json") -> None:
    """Create an example configuration file with all options documented.

    Args:
        output_path: Path to write example config
    """
    # Create config with example values
    config = ReportConfig(
        site=SiteConfig(
            location="Clarendon Avenue, San Francisco",
            surveyor="Banshee, INC.",
            contact="david@banshee-data.com",
            speed_limit=25,
            site_description=(
                "This survey was conducted from the southbound parking lane outside "
                "500 Clarendon Avenue, directly in front of an elementary school. "
                "The site is located on a downhill grade, which may influence vehicle "
                "speed and braking behavior. Data was collected from a fixed position "
                "over three consecutive days."
            ),
            speed_limit_note=(
                "The posted speed limit at this location is 35 mph, reduced to 25 mph "
                "when school children are present."
            ),
            latitude=37.7833,
            longitude=-122.4167,
            map_angle=45.0,
        ),
        radar=RadarConfig(
            sensor_model="OmniPreSense OPS243-A",
            firmware_version="v1.2.3",
            transmit_frequency="24.125 GHz",
            sample_rate="20 kSPS",
            velocity_resolution="0.272 mph",
            azimuth_fov="20°",
        ),
        query=QueryConfig(
            start_date="2025-06-01",
            end_date="2025-06-07",
            group="1h",
            units="mph",
            source="radar_data_transits",
            model_version="rebuild-full",
            timezone="US/Pacific",
            min_speed=5.0,
            histogram=True,
            hist_bucket_size=5.0,
            hist_max=60.0,
        ),
        output=OutputConfig(
            file_prefix="clarendon-survey",
            output_dir="./reports",
            debug=False,
            no_map=False,
        ),
    )

    # Convert to dict and add comments as a separate structure
    config_dict = config.to_dict()

    # Create commented structure
    commented_config = {
        "_comment": "Velocity Report Configuration File",
        "_instructions": [
            "This file controls all aspects of report generation.",
            "All CLI flags and environment variables have been replaced by this config.",
            "Use: python get_stats.py config.json",
            "",
            "SECTIONS:",
            "  site: Location and survey information",
            "  radar: Technical sensor specifications",
            "  query: Data query parameters and date range",
            "  output: Output file settings and options",
        ],
        "site": {
            "_description": "Site location and survey metadata",
            "location": "Clarendon Avenue, San Francisco",
            "surveyor": "Banshee, INC.",
            "contact": "david@banshee-data.com",
            "speed_limit": 25,
            "site_description": config_dict["site"]["site_description"],
            "speed_limit_note": config_dict["site"]["speed_limit_note"],
            "latitude": 37.7833,
            "longitude": -122.4167,
            "map_angle": 45.0,
            "_field_notes": {
                "latitude": "Optional: GPS latitude for map marker",
                "longitude": "Optional: GPS longitude for map marker",
                "map_angle": "Optional: Rotation angle for map display",
            },
        },
        "radar": {
            "_description": "Radar sensor technical specifications",
            "sensor_model": "OmniPreSense OPS243-A",
            "firmware_version": "v1.2.3",
            "transmit_frequency": "24.125 GHz",
            "sample_rate": "20 kSPS",
            "velocity_resolution": "0.272 mph",
            "azimuth_fov": "20°",
            "_field_notes": {
                "_note": "These fields are included in the report for documentation"
            },
        },
        "query": {
            "_description": "Data query parameters",
            "start_date": "2025-06-01",
            "end_date": "2025-06-07",
            "group": "1h",
            "units": "mph",
            "source": "radar_data_transits",
            "model_version": "rebuild-full",
            "timezone": "US/Pacific",
            "min_speed": 5.0,
            "histogram": True,
            "hist_bucket_size": 5.0,
            "hist_max": 60.0,
            "_field_notes": {
                "start_date": "REQUIRED: Start date (YYYY-MM-DD)",
                "end_date": "REQUIRED: End date (YYYY-MM-DD)",
                "group": "Time bucket size: 15m, 30m, 1h, 2h, 4h, 8h, 12h, 24h",
                "units": "Display units: mph or kph",
                "source": "Data source: radar_objects or radar_data_transits",
                "timezone": "Display timezone (e.g., US/Pacific, UTC)",
                "min_speed": "Optional: Minimum speed filter in display units",
                "histogram": "Generate histogram chart: true or false",
                "hist_bucket_size": "Required if histogram=true: bucket size (e.g., 5.0)",
                "hist_max": "Optional: Maximum speed for histogram",
            },
        },
        "output": {
            "_description": "Output file settings",
            "file_prefix": "clarendon-survey",
            "output_dir": "./reports",
            "debug": False,
            "no_map": False,
            "run_id": "auto-generated",
            "_field_notes": {
                "file_prefix": "Prefix for generated files (default: auto-generated)",
                "output_dir": "Output directory (default: current directory)",
                "debug": "Enable debug output: true or false",
                "no_map": "Skip map generation: true or false (useful when no GPS data)",
                "run_id": "Auto-generated unique ID for this report generation",
            },
        },
        "_metadata": {
            "created_at": "Auto-generated timestamp",
            "updated_at": "Auto-generated timestamp",
            "version": "1.0",
        },
    }

    # Write with nice formatting
    with open(output_path, "w") as f:
        json.dump(commented_config, f, indent=2)

    print(f"✅ Created example configuration: {output_path}")
    print()
    print("To use this config:")
    print(f"  1. Copy to your working config: cp {output_path} my-config.json")
    print(f"  2. Edit my-config.json with your specific values")
    print(f"  3. Generate report: python get_stats.py my-config.json")
    print()
    print("Or use the API:")
    print(f"  python generate_report_api.py my-config.json")


def create_minimal_config(output_path: str = "config.minimal.json") -> None:
    """Create a minimal configuration file with only required fields.

    Args:
        output_path: Path to write minimal config
    """
    minimal = {
        "query": {
            "start_date": "2025-06-01",
            "end_date": "2025-06-07",
            "histogram": True,
            "hist_bucket_size": 5.0,
        },
        "output": {
            "file_prefix": "my-report",
        },
    }

    with open(output_path, "w") as f:
        json.dump(minimal, f, indent=2)

    print(f"✅ Created minimal configuration: {output_path}")
    print("   (Uses defaults for all optional fields)")


if __name__ == "__main__":
    import argparse

    parser = argparse.ArgumentParser(
        description="Generate example configuration files for velocity reports"
    )
    parser.add_argument(
        "--output",
        "-o",
        default="config.example.json",
        help="Output path for example config (default: config.example.json)",
    )
    parser.add_argument(
        "--minimal",
        action="store_true",
        help="Also create a minimal config file (config.minimal.json)",
    )

    args = parser.parse_args()

    create_example_config(args.output)

    if args.minimal:
        print()
        create_minimal_config()

    print()
    print("📚 For more information:")
    print("   - See CONFIG_SYSTEM.md for full documentation")
    print("   - See GO_INTEGRATION.md for Go server integration")
    print("   - See README.md for usage examples")
