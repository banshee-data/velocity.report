#!/usr/bin/env python3
"""Tests for debug config file writing functionality."""

import json
import os
import tempfile
import shutil

import pytest

from pdf_generator.core.config_manager import (
    ReportConfig,
    SiteConfig,
    RadarConfig,
    QueryConfig,
    OutputConfig,
)


def test_output_config_debug_flag():
    """Test OutputConfig debug flag defaults to False."""
    config = OutputConfig()
    assert config.debug is False


def test_output_config_debug_flag_explicit():
    """Test OutputConfig debug flag can be set explicitly."""
    config = OutputConfig(debug=True)
    assert config.debug is True


def test_report_config_includes_debug_in_dict():
    """Test that debug flag is included in config.to_dict()."""
    config = ReportConfig(
        output=OutputConfig(debug=True),
        site=SiteConfig(location="Test", surveyor="Test", contact="test@test.com"),
        radar=RadarConfig(cosine_error_angle=21.0),
        query=QueryConfig(
            start_date="2025-01-01", end_date="2025-01-02", timezone="UTC"
        ),
    )

    data = config.to_dict()
    assert "output" in data
    assert "debug" in data["output"]
    assert data["output"]["debug"] is True


def test_report_config_debug_false_in_dict():
    """Test that debug=False is preserved in config.to_dict()."""
    config = ReportConfig(
        output=OutputConfig(debug=False),
        site=SiteConfig(location="Test", surveyor="Test", contact="test@test.com"),
        radar=RadarConfig(cosine_error_angle=21.0),
        query=QueryConfig(
            start_date="2025-01-01", end_date="2025-01-02", timezone="UTC"
        ),
    )

    data = config.to_dict()
    assert data["output"]["debug"] is False


def test_config_json_serialization_with_debug():
    """Test that config with debug flag can be serialized to JSON."""
    config = ReportConfig(
        output=OutputConfig(debug=True, output_dir="/tmp/test"),
        site=SiteConfig(location="Test", surveyor="Test", contact="test@test.com"),
        radar=RadarConfig(cosine_error_angle=21.0),
        query=QueryConfig(
            start_date="2025-01-01", end_date="2025-01-02", timezone="UTC"
        ),
    )

    # Serialize to JSON
    json_str = json.dumps(config.to_dict())

    # Deserialize back
    data = json.loads(json_str)

    assert data["output"]["debug"] is True


def test_load_config_preserves_debug_flag():
    """Test that loading config from JSON preserves the debug flag."""
    with tempfile.NamedTemporaryFile(mode="w", suffix=".json", delete=False) as f:
        config_data = {
            "site": {
                "location": "Test Location",
                "surveyor": "Test Surveyor",
                "contact": "test@example.com",
                "speed_limit": 25,
            },
            "radar": {
                "cosine_error_angle": 21.0,
            },
            "query": {
                "start_date": "2025-01-01",
                "end_date": "2025-01-02",
                "timezone": "UTC",
                "group": "1h",
                "units": "mph",
            },
            "output": {
                "debug": True,
                "output_dir": "/tmp/test",
            },
        }
        json.dump(config_data, f)
        config_file = f.name

    try:
        from pdf_generator.core.config_manager import load_config

        config = load_config(config_file=config_file)

        assert config.output.debug is True
    finally:
        os.remove(config_file)


def test_submitted_vs_final_config_simulation():
    """Test simulating the difference between submitted and final configs.

    This simulates what happens in the CLI:
    1. Go server submits minimal config
    2. Python loads it and applies defaults
    3. Both should be saved when debug is enabled
    """
    # Simulate submitted config (minimal, from Go server)
    submitted_config = {
        "site": {
            "location": "Test Location",
            "surveyor": "Test Surveyor",
            "contact": "test@example.com",
            "speed_limit": 25,
            "site_description": "",
            "speed_limit_note": "Posted 25 mph",
        },
        "radar": {
            "cosine_error_angle": 21.0,
        },
        "query": {
            "start_date": "2025-01-01",
            "end_date": "2025-01-02",
            "timezone": "UTC",
            "group": "1h",
            "units": "mph",
            "source": "radar_objects",
            "min_speed": 5.0,
            "histogram": True,
            "hist_bucket_size": 5.0,
            "hist_max": 0,
        },
        "output": {
            "output_dir": "output/test",
            "debug": True,
        },
    }

    # Write submitted config
    with tempfile.TemporaryDirectory() as tmpdir:
        submitted_path = os.path.join(tmpdir, "submitted_config.json")
        with open(submitted_path, "w") as f:
            json.dump(submitted_config, f, indent=2)

        # Load into ReportConfig (applies defaults)
        from pdf_generator.core.config_manager import load_config

        config = load_config(config_file=submitted_path)

        # Final config includes all defaults
        final_config = config.to_dict()
        final_path = os.path.join(tmpdir, "final_config.json")
        with open(final_path, "w") as f:
            json.dump(final_config, f, indent=2)

        # Verify both exist
        assert os.path.exists(submitted_path)
        assert os.path.exists(final_path)

        # Verify submitted config is smaller (fewer keys)
        with open(submitted_path) as f:
            submitted_data = json.load(f)
        with open(final_path) as f:
            final_data = json.load(f)

        # Final config should have more keys (colors, fonts, layout, etc.)
        assert "colors" not in submitted_data
        assert "fonts" not in submitted_data
        assert "layout" not in submitted_data

        assert "colors" in final_data
        assert "fonts" in final_data
        assert "layout" in final_data

        # Both should have debug flag set to True
        assert submitted_data["output"]["debug"] is True
        assert final_data["output"]["debug"] is True


def test_config_file_copy_preserves_content():
    """Test that copying config file preserves exact content."""
    original_data = {
        "site": {"location": "Test", "surveyor": "Test", "contact": "test@test.com"},
        "radar": {"cosine_error_angle": 21.0},
        "query": {
            "start_date": "2025-01-01",
            "end_date": "2025-01-02",
            "timezone": "UTC",
        },
        "output": {"debug": True},
    }

    with tempfile.TemporaryDirectory() as tmpdir:
        original_path = os.path.join(tmpdir, "original.json")
        copy_path = os.path.join(tmpdir, "copy.json")

        # Write original
        with open(original_path, "w") as f:
            json.dump(original_data, f, indent=2)

        # Copy using shutil (as done in CLI)
        shutil.copyfile(original_path, copy_path)

        # Verify content is identical
        with open(original_path) as f:
            original_loaded = json.load(f)
        with open(copy_path) as f:
            copy_loaded = json.load(f)

        assert original_loaded == copy_loaded
        assert original_loaded["output"]["debug"] is True
        assert copy_loaded["output"]["debug"] is True


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
