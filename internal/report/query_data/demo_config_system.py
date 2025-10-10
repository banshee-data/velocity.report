#!/usr/bin/env python3
"""
Quick demo of the unified configuration system.

This demonstrates all the different ways to use the configuration system:
1. Create config from scratch
2. Save to JSON file
3. Load from JSON file
4. Use with CLI
5. Call via API

Run this to see the configuration system in action!
"""

import json
import tempfile
from pathlib import Path

# Import our new configuration system
from config_manager import (
    ReportConfig,
    SiteConfig,
    QueryConfig,
    OutputConfig,
    load_config,
)
from generate_report_api import generate_report_from_dict


def demo_create_config():
    """Demo 1: Create configuration from scratch."""
    print("=" * 70)
    print("DEMO 1: Creating configuration from scratch")
    print("=" * 70)

    config = ReportConfig(
        site=SiteConfig(
            location="Main Street, Springfield",
            surveyor="City Traffic Department",
            contact="traffic@springfield.gov",
            speed_limit=30,
        ),
        query=QueryConfig(
            start_date="2025-06-01",
            end_date="2025-06-07",
            group="1h",
            units="mph",
            timezone="America/Chicago",
            min_speed=5.0,
            histogram=True,
            hist_bucket_size=5.0,
        ),
        output=OutputConfig(
            file_prefix="main-st-demo",
            output_dir="/tmp/demo-reports",
            debug=False,
        ),
    )

    print(f"✅ Created config for: {config.site.location}")
    print(f"   Date range: {config.query.start_date} to {config.query.end_date}")
    print(f"   Units: {config.query.units}")
    print(f"   Histogram: {config.query.histogram}")
    print()

    return config


def demo_save_load_json(config):
    """Demo 2: Save to and load from JSON file."""
    print("=" * 70)
    print("DEMO 2: Save to and load from JSON file")
    print("=" * 70)

    # Create temp file
    temp_file = tempfile.NamedTemporaryFile(mode="w", suffix=".json", delete=False)
    temp_path = temp_file.name
    temp_file.close()

    # Save config
    config.to_json(temp_path, indent=2)
    print(f"✅ Saved config to: {temp_path}")

    # Show first few lines
    with open(temp_path, "r") as f:
        lines = f.readlines()
        print("   First 10 lines:")
        for line in lines[:10]:
            print(f"     {line.rstrip()}")

    # Load config back
    loaded_config = ReportConfig.from_json(temp_path)
    print(f"✅ Loaded config from file")
    print(f"   Location: {loaded_config.site.location}")
    print(
        f"   Dates: {loaded_config.query.start_date} to {loaded_config.query.end_date}"
    )
    print()

    return temp_path


def demo_validation(config):
    """Demo 3: Validation."""
    print("=" * 70)
    print("DEMO 3: Configuration validation")
    print("=" * 70)

    # Valid config
    is_valid, errors = config.validate()
    print(f"✅ Valid config: {is_valid}")
    if errors:
        print(f"   Errors: {errors}")
    print()

    # Invalid config (missing dates)
    invalid_config = ReportConfig()
    is_valid, errors = invalid_config.validate()
    print(f"❌ Invalid config (no dates): {is_valid}")
    print(f"   Errors:")
    for error in errors:
        print(f"      - {error}")
    print()


def demo_dict_conversion(config):
    """Demo 4: Dictionary conversion."""
    print("=" * 70)
    print("DEMO 4: Dictionary conversion (for Go integration)")
    print("=" * 70)

    # Convert to dict
    config_dict = config.to_dict()
    print(f"✅ Converted to dictionary")
    print(f"   Keys: {list(config_dict.keys())}")
    print()

    # Show site section
    print("   Site configuration:")
    print(json.dumps(config_dict["site"], indent=4))
    print()

    # Convert back from dict
    reconstructed = ReportConfig.from_dict(config_dict)
    print(f"✅ Reconstructed from dictionary")
    print(f"   Location: {reconstructed.site.location}")
    print()


def demo_cli_args():
    """Demo 5: CLI argument compatibility."""
    print("=" * 70)
    print("DEMO 5: CLI argument compatibility")
    print("=" * 70)

    # Mock CLI args (like argparse.Namespace)
    args = type("Args", (), {})()
    args.dates = ["2025-06-15", "2025-06-21"]
    args.group = "2h"
    args.units = "kph"
    args.source = "radar_data_transits"
    args.model_version = "rebuild-full"
    args.timezone = "UTC"
    args.min_speed = 10.0
    args.file_prefix = "cli-demo"
    args.histogram = True
    args.hist_bucket_size = 10.0
    args.hist_max = 100.0
    args.debug = False

    config = ReportConfig.from_cli_args(args)

    print(f"✅ Created config from CLI args")
    print(f"   Date range: {config.query.start_date} to {config.query.end_date}")
    print(f"   Group: {config.query.group}")
    print(f"   Units: {config.query.units}")
    print(f"   Min speed: {config.query.min_speed}")
    print()


def demo_priority_system():
    """Demo 6: Configuration priority system."""
    print("=" * 70)
    print("DEMO 6: Configuration priority (CLI > File > Env > Default)")
    print("=" * 70)

    # Create base config with defaults
    base = ReportConfig()
    print(f"Default location: {base.site.location}")

    # Load from environment (if set)
    import os

    os.environ["REPORT_LOCATION"] = "Environment Location"
    env_config = ReportConfig.from_env()
    print(f"Env location: {env_config.site.location}")

    # File config overrides env
    file_config = ReportConfig(
        site=SiteConfig(location="File Location"),
        query=QueryConfig(start_date="2025-01-01", end_date="2025-01-31"),
    )
    print(f"File location: {file_config.site.location}")

    # CLI args would override file (demonstrated in actual CLI)
    print()
    print("Priority order demonstrated:")
    print("  1. CLI arguments (highest priority)")
    print("  2. Config file")
    print("  3. Environment variables")
    print("  4. Default values (lowest priority)")
    print()


def main():
    """Run all demos."""
    print("\n")
    print("╔" + "=" * 68 + "╗")
    print("║" + " " * 15 + "CONFIGURATION SYSTEM DEMO" + " " * 29 + "║")
    print("╚" + "=" * 68 + "╝")
    print()

    # Run demos
    config = demo_create_config()
    config_file = demo_save_load_json(config)
    demo_validation(config)
    demo_dict_conversion(config)
    demo_cli_args()
    demo_priority_system()

    # Summary
    print("=" * 70)
    print("SUMMARY")
    print("=" * 70)
    print()
    print("The configuration system supports:")
    print("  ✅ Creating configs programmatically (from Go server)")
    print("  ✅ Saving/loading JSON files")
    print("  ✅ Validation with helpful error messages")
    print("  ✅ Dictionary conversion (for JSON APIs)")
    print("  ✅ CLI argument compatibility (backward compatible)")
    print("  ✅ Environment variable overrides")
    print("  ✅ Priority-based configuration merging")
    print()
    print("Next steps:")
    print("  1. Use report_config_example.json as a template")
    print("  2. Integrate with Go server using generate_report_api.py")
    print("  3. See CONFIG_SYSTEM.md for full documentation")
    print("  4. See GO_INTEGRATION.md for Go code examples")
    print()
    print(f"Example config saved to: {config_file}")
    print()

    # Cleanup
    import os

    os.unlink(config_file)
    print("✅ Demo complete!\n")


if __name__ == "__main__":
    main()
