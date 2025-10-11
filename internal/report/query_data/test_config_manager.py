#!/usr/bin/env python3
"""Tests for configuration management system."""

import json
import os
import tempfile
from datetime import datetime

import pytest

from config_manager import (
    ReportConfig,
    SiteConfig,
    RadarConfig,
    QueryConfig,
    OutputConfig,
    load_config,
)


def test_site_config_defaults():
    """Test SiteConfig has sensible defaults."""
    # location, surveyor, contact are required fields (empty string defaults)
    config = SiteConfig(location="Test", surveyor="Test", contact="test@test.com")
    assert config.location == "Test"
    assert config.surveyor == "Test"
    assert config.speed_limit == 25  # This still has a default


def test_query_config_defaults():
    """Test QueryConfig has sensible defaults."""
    # start_date, end_date, timezone are required (empty string defaults)
    config = QueryConfig(start_date="2025-01-01", end_date="2025-01-02", timezone="UTC")
    assert config.group == "1h"
    assert config.units == "mph"
    assert config.source == "radar_data_transits"
    assert config.histogram is True


def test_report_config_to_dict():
    """Test converting ReportConfig to dictionary."""
    config = ReportConfig()
    data = config.to_dict()

    assert "site" in data
    assert "radar" in data
    assert "query" in data
    assert "output" in data
    assert data["version"] == "1.0"


def test_report_config_from_dict():
    """Test creating ReportConfig from dictionary."""
    data = {
        "site": {
            "location": "Test Location",
            "speed_limit": 30,
        },
        "query": {
            "start_date": "2025-06-01",
            "end_date": "2025-06-07",
            "units": "kph",
        },
    }

    config = ReportConfig.from_dict(data)
    assert config.site.location == "Test Location"
    assert config.site.speed_limit == 30
    assert config.query.start_date == "2025-06-01"
    assert config.query.units == "kph"


def test_report_config_json_roundtrip():
    """Test saving and loading config from JSON file."""
    original = ReportConfig(
        site=SiteConfig(
            location="Test Site",
            surveyor="Test Surveyor",
            speed_limit=35,
        ),
        query=QueryConfig(
            start_date="2025-01-01",
            end_date="2025-01-31",
            units="kph",
            min_speed=10.0,
        ),
    )

    with tempfile.NamedTemporaryFile(mode="w", suffix=".json", delete=False) as f:
        original.to_json(f.name)
        temp_path = f.name

    try:
        loaded = ReportConfig.from_json(temp_path)
        assert loaded.site.location == "Test Site"
        assert loaded.site.surveyor == "Test Surveyor"
        assert loaded.site.speed_limit == 35
        assert loaded.query.start_date == "2025-01-01"
        assert loaded.query.units == "kph"
        assert loaded.query.min_speed == 10.0
    finally:
        os.unlink(temp_path)


def test_validation_missing_dates():
    """Test validation fails when dates are missing."""
    config = ReportConfig()
    is_valid, errors = config.validate()

    assert not is_valid
    assert "query.start_date is required" in errors
    assert "query.end_date is required" in errors


def test_validation_invalid_source():
    """Test validation fails for invalid source."""
    config = ReportConfig(
        query=QueryConfig(
            start_date="2025-06-01",
            end_date="2025-06-07",
            source="invalid_source",
        )
    )

    is_valid, errors = config.validate()
    assert not is_valid
    assert any("Invalid source" in e for e in errors)


def test_validation_invalid_units():
    """Test validation fails for invalid units."""
    config = ReportConfig(
        query=QueryConfig(
            start_date="2025-06-01",
            end_date="2025-06-07",
            units="invalid_units",
        )
    )

    is_valid, errors = config.validate()
    assert not is_valid
    assert any("Invalid units" in e for e in errors)


def test_validation_histogram_requires_bucket_size():
    """Test validation requires bucket size when histogram is enabled."""
    config = ReportConfig(
        query=QueryConfig(
            start_date="2025-06-01",
            end_date="2025-06-07",
            histogram=True,
            hist_bucket_size=None,
        )
    )

    is_valid, errors = config.validate()
    assert not is_valid
    assert any("hist_bucket_size is required" in e for e in errors)


def test_validation_success():
    """Test validation passes for valid config."""
    config = ReportConfig(
        site=SiteConfig(
            location="Test Location",
            surveyor="Test Surveyor",
            contact="test@test.com",
        ),
        query=QueryConfig(
            start_date="2025-06-01",
            end_date="2025-06-07",
            timezone="UTC",
            histogram=True,
            hist_bucket_size=5.0,
        ),
        radar=RadarConfig(cosine_error_angle=21.0),
    )

    is_valid, errors = config.validate()
    assert is_valid, f"Validation failed with errors: {errors}"
    assert len(errors) == 0


def test_load_config_from_file(tmp_path):
    """Test load_config function with config file."""
    config_data = {
        "site": {"location": "File Location"},
        "query": {
            "start_date": "2025-06-01",
            "end_date": "2025-06-07",
        },
    }

    config_file = tmp_path / "test_config.json"
    with open(config_file, "w") as f:
        json.dump(config_data, f)

    config = load_config(config_file=str(config_file))
    assert config.site.location == "File Location"
    assert config.query.start_date == "2025-06-01"


def test_created_at_timestamp():
    """Test that created_at timestamp is set automatically."""
    config = ReportConfig()
    assert config.created_at is not None

    # Should be valid ISO format
    datetime.fromisoformat(config.created_at.replace("Z", "+00:00"))


def test_example_config_generation():
    """Test that example config can be generated."""
    with tempfile.NamedTemporaryFile(mode="w", suffix=".json", delete=False) as f:
        temp_path = f.name

    try:
        from config_manager import create_example_config

        create_example_config(temp_path)

        assert os.path.exists(temp_path)

        # Load and validate
        config = ReportConfig.from_json(temp_path)
        assert config.site.location == "Clarendon Avenue, San Francisco"
        assert config.query.start_date == "2025-06-02"
    finally:
        if os.path.exists(temp_path):
            os.unlink(temp_path)


def test_load_config_with_invalid_json_syntax():
    """Test load_config with syntactically invalid JSON."""
    with tempfile.NamedTemporaryFile(mode="w", suffix=".json", delete=False) as f:
        f.write('{"invalid": json content}')  # Invalid syntax
        temp_file = f.name

    try:
        with pytest.raises(json.JSONDecodeError):
            load_config(config_file=temp_file)
    finally:
        os.unlink(temp_file)


def test_load_config_with_neither_file_nor_dict():
    """Test load_config when both file and dict are None."""
    with pytest.raises(ValueError) as exc_info:
        load_config(config_file=None, config_dict=None)

    error_msg = str(exc_info.value).lower()
    assert "either" in error_msg or "neither" in error_msg


def test_load_config_file_not_found_error_message():
    """Test load_config error message for non-existent file."""
    with pytest.raises(ValueError) as exc_info:
        load_config(config_file="/nonexistent/path/config.json")

    error_msg = str(exc_info.value).lower()
    assert "not found" in error_msg
    assert "config.json" in str(exc_info.value)


def test_load_config_with_both_file_and_dict_prefers_file():
    """Test load_config when both file and dict are provided (should prefer file)."""
    config_data = {
        "site": {
            "location": "File Location",
            "surveyor": "Test",
            "contact": "test@test.com",
        },
        "radar": {"cosine_error_angle": 20.0},
        "query": {
            "start_date": "2025-06-01",
            "end_date": "2025-06-07",
            "timezone": "UTC",
        },
    }

    dict_data = {
        "site": {
            "location": "Dict Location",
            "surveyor": "Test",
            "contact": "test@test.com",
        },
        "radar": {"cosine_error_angle": 15.0},
        "query": {
            "start_date": "2025-06-01",
            "end_date": "2025-06-07",
            "timezone": "UTC",
        },
    }

    with tempfile.NamedTemporaryFile(mode="w", suffix=".json", delete=False) as f:
        json.dump(config_data, f)
        temp_file = f.name

    try:
        config = load_config(config_file=temp_file, config_dict=dict_data)
        # Should use file data, not dict data
        assert config.site.location == "File Location"
        assert config.radar.cosine_error_angle == 20.0
    finally:
        os.unlink(temp_file)


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
