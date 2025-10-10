#!/usr/bin/env python3
"""Web API entry point for report generation.

This module provides a simple function-based API that can be called by:
1. Go webserver (via subprocess or HTTP)
2. Flask/FastAPI endpoints
3. Direct Python imports

The simplified workflow:
1. User submits form → Go captures data
2. Go saves config to SQLite + JSON file
3. Go calls this API with config file path or dict
4. Python generates PDF and returns file paths
5. Go moves files to report-specific folder
6. Svelte frontend displays download links

All configuration is in JSON format - no CLI flags or env vars.
"""

import json
import os
import sys
from typing import Dict, Any, List, Optional
from pathlib import Path

# Add parent directory to path for imports
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from config_manager import ReportConfig, load_config
from api_client import RadarStatsClient
import get_stats


class ReportGenerationError(Exception):
    """Custom exception for report generation failures."""

    pass


def generate_report_from_config(
    config: ReportConfig,
    output_dir: Optional[str] = None,
) -> Dict[str, Any]:
    """Generate PDF report from configuration object.

    Args:
        config: ReportConfig instance
        output_dir: Optional output directory (overrides config.output.output_dir)

    Returns:
        Dictionary with:
        {
            "success": bool,
            "files": [list of generated file paths],
            "prefix": str,
            "errors": [list of error messages],
            "config_used": dict (the effective configuration)
        }

    Raises:
        ReportGenerationError: If generation fails
    """
    result = {
        "success": False,
        "files": [],
        "prefix": "",
        "errors": [],
        "config_used": config.to_dict(),
    }

    try:
        # Validate configuration
        is_valid, errors = config.validate()
        if not is_valid:
            result["errors"] = errors
            raise ReportGenerationError(f"Invalid configuration: {', '.join(errors)}")

        # Override output directory if provided
        if output_dir:
            config.output.output_dir = output_dir

        # Ensure output directory exists
        os.makedirs(config.output.output_dir, exist_ok=True)

        # Change to output directory for file generation
        original_dir = os.getcwd()
        os.chdir(config.output.output_dir)

        try:
            # Create argparse.Namespace object from config for backward compatibility
            # TODO: Refactor get_stats.py to use config directly
            args = type("Args", (), {})()
            args.dates = [config.query.start_date, config.query.end_date]
            args.group = config.query.group
            args.units = config.query.units
            args.source = config.query.source
            args.model_version = config.query.model_version
            args.timezone = config.query.timezone
            args.min_speed = config.query.min_speed
            args.file_prefix = config.output.file_prefix
            args.histogram = config.query.histogram
            args.hist_bucket_size = config.query.hist_bucket_size
            args.hist_max = config.query.hist_max
            args.debug = config.output.debug
            args.no_map = config.output.no_map

            # Generate reports using existing get_stats.py logic
            date_ranges = [(config.query.start_date, config.query.end_date)]
            get_stats.main(date_ranges, args)

            # Determine file prefix that was actually used
            client = RadarStatsClient()
            start_ts, end_ts = get_stats.parse_date_range(
                config.query.start_date,
                config.query.end_date,
                config.query.timezone or None,
            )
            prefix = get_stats.resolve_file_prefix(args, start_ts, end_ts)

            # Collect generated files
            expected_files = [
                f"{prefix}_report.pdf",
                f"{prefix}_report.tex",
                f"{prefix}_stats.pdf",
            ]

            if get_stats.should_produce_daily(config.query.group):
                expected_files.append(f"{prefix}_daily.pdf")

            if config.query.histogram:
                expected_files.append(f"{prefix}_histogram.pdf")

            # Build absolute paths and check existence
            generated_files = []
            for fname in expected_files:
                fpath = os.path.join(config.output.output_dir, fname)
                if os.path.exists(fpath):
                    generated_files.append(fpath)

            result["files"] = generated_files
            result["prefix"] = prefix
            result["success"] = len(generated_files) > 0

            if not generated_files:
                result["errors"].append("No output files were generated")

        finally:
            # Return to original directory
            os.chdir(original_dir)

    except Exception as e:
        result["errors"].append(str(e))
        result["success"] = False
        if config.output.debug:
            import traceback

            result["traceback"] = traceback.format_exc()

    return result


def generate_report_from_file(
    config_file: str,
    output_dir: Optional[str] = None,
) -> Dict[str, Any]:
    """Generate PDF report from JSON config file.

    Args:
        config_file: Path to JSON configuration file
        output_dir: Optional output directory (overrides config value)

    Returns:
        Dictionary with generation results (see generate_report_from_config)
    """
    if not os.path.exists(config_file):
        return {
            "success": False,
            "files": [],
            "prefix": "",
            "errors": [f"Config file not found: {config_file}"],
            "config_used": {},
        }

    try:
        config = ReportConfig.from_json(config_file)
        return generate_report_from_config(config, output_dir)
    except Exception as e:
        return {
            "success": False,
            "files": [],
            "prefix": "",
            "errors": [f"Failed to load config file: {str(e)}"],
            "config_used": {},
        }


def generate_report_from_dict(
    config_dict: Dict[str, Any],
    output_dir: Optional[str] = None,
) -> Dict[str, Any]:
    """Generate PDF report from configuration dictionary.

    Useful for Flask/FastAPI endpoints that receive JSON payloads.

    Args:
        config_dict: Configuration dictionary matching ReportConfig schema
        output_dir: Optional output directory (overrides config value)

    Returns:
        Dictionary with generation results (see generate_report_from_config)
    """
    try:
        config = ReportConfig.from_dict(config_dict)
        return generate_report_from_config(config, output_dir)
    except Exception as e:
        return {
            "success": False,
            "files": [],
            "prefix": "",
            "errors": [f"Invalid configuration dictionary: {str(e)}"],
            "config_used": config_dict,
        }


# CLI interface for direct invocation
if __name__ == "__main__":
    import argparse

    parser = argparse.ArgumentParser(
        description="Generate report from configuration file (Web API entry point)"
    )
    parser.add_argument(
        "config_file",
        help="Path to JSON configuration file",
    )
    parser.add_argument(
        "--output-dir",
        help="Override output directory from config file",
    )
    parser.add_argument(
        "--json",
        action="store_true",
        help="Output result as JSON (for programmatic use)",
    )

    args = parser.parse_args()

    result = generate_report_from_file(args.config_file, args.output_dir)

    if args.json:
        # Output as JSON for parsing by Go server
        print(json.dumps(result, indent=2))
    else:
        # Human-readable output
        if result["success"]:
            print(f"✅ Report generated successfully!")
            print(f"   Prefix: {result['prefix']}")
            print(f"   Files generated ({len(result['files'])}):")
            for f in result["files"]:
                print(f"      - {f}")
        else:
            print(f"❌ Report generation failed!")
            print(f"   Errors:")
            for e in result["errors"]:
                print(f"      - {e}")
            sys.exit(1)
