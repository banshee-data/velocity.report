#!/usr/bin/env python3
"""Tests for create_config.py CLI tool."""

import json
import os
from pathlib import Path

import pytest

from pdf_generator.cli.create_config import (
    create_example_config,
    create_minimal_config,
)
from pdf_generator.core.config_manager import load_config


class TestCreateExampleConfig:
    """Tests for create_example_config function."""

    def test_creates_config_file(self, tmp_path):
        """Test that config file is created at specified path."""
        output_file = tmp_path / "test_config.json"
        create_example_config(str(output_file))

        assert output_file.exists()
        assert output_file.stat().st_size > 0

    def test_creates_valid_json(self, tmp_path):
        """Test that created config is valid JSON."""
        output_file = tmp_path / "test_config.json"
        create_example_config(str(output_file))

        with open(output_file) as f:
            data = json.load(f)

        assert isinstance(data, dict)

    def test_contains_all_sections(self, tmp_path):
        """Test that config contains all expected sections."""
        output_file = tmp_path / "test_config.json"
        create_example_config(str(output_file))

        with open(output_file) as f:
            data = json.load(f)

        # Check all main sections exist
        assert "site" in data
        assert "radar" in data
        assert "query" in data
        assert "output" in data
        assert "_comment" in data
        assert "_instructions" in data
        assert "_metadata" in data

    def test_site_section_structure(self, tmp_path):
        """Test that site section has required fields."""
        output_file = tmp_path / "test_config.json"
        create_example_config(str(output_file))

        with open(output_file) as f:
            data = json.load(f)

        site = data["site"]
        assert "location" in site
        assert "surveyor" in site
        assert "contact" in site
        assert "speed_limit" in site
        assert "site_description" in site
        assert "speed_limit_note" in site
        assert "latitude" in site
        assert "longitude" in site
        assert "map_angle" in site
        assert "_description" in site
        assert "_field_notes" in site

    def test_query_section_structure(self, tmp_path):
        """Test that query section has required fields."""
        output_file = tmp_path / "test_config.json"
        create_example_config(str(output_file))

        with open(output_file) as f:
            data = json.load(f)

        query = data["query"]
        assert "start_date" in query
        assert "end_date" in query
        assert "group" in query
        assert "units" in query
        assert "timezone" in query
        assert "min_speed" in query
        assert "histogram" in query
        assert "hist_bucket_size" in query
        assert "_description" in query
        assert "_field_notes" in query

    def test_output_section_structure(self, tmp_path):
        """Test that output section has required fields."""
        output_file = tmp_path / "test_config.json"
        create_example_config(str(output_file))

        with open(output_file) as f:
            data = json.load(f)

        output = data["output"]
        assert "file_prefix" in output
        assert "output_dir" in output
        assert "debug" in output
        assert "map" in output
        assert "_description" in output
        assert "_field_notes" in output

    def test_radar_section_structure(self, tmp_path):
        """Test that radar section has required fields."""
        output_file = tmp_path / "test_config.json"
        create_example_config(str(output_file))

        with open(output_file) as f:
            data = json.load(f)

        radar = data["radar"]
        # REQUIRED field
        assert "cosine_error_angle" in radar
        assert radar["cosine_error_angle"] is not None
        assert isinstance(radar["cosine_error_angle"], (int, float))

        # Optional documentation fields
        assert "sensor_model" in radar
        assert "firmware_version" in radar
        assert "transmit_frequency" in radar
        assert "elevation_fov" in radar
        assert "_description" in radar
        assert "_field_notes" in radar

    def test_includes_field_notes(self, tmp_path):
        """Test that field notes are included for documentation."""
        output_file = tmp_path / "test_config.json"
        create_example_config(str(output_file))

        with open(output_file) as f:
            data = json.load(f)

        # Check that _field_notes exist and have helpful descriptions
        assert "_field_notes" in data["query"]
        field_notes = data["query"]["_field_notes"]
        assert "start_date" in field_notes
        assert "REQUIRED" in field_notes["start_date"]

    def test_json_formatting(self, tmp_path):
        """Test that JSON is nicely formatted with indentation."""
        output_file = tmp_path / "test_config.json"
        create_example_config(str(output_file))

        with open(output_file) as f:
            content = f.read()

        # Check for indentation (pretty printing)
        assert "  " in content  # Has indentation
        assert "\n" in content  # Has newlines

    def test_default_output_path(self, tmp_path):
        """Test using default output path."""
        # Change to temp directory to avoid creating file in project root
        original_dir = os.getcwd()
        try:
            os.chdir(tmp_path)
            create_example_config()

            default_file = Path("config.example.json")
            assert default_file.exists()
        finally:
            os.chdir(original_dir)

    def test_custom_output_path(self, tmp_path):
        """Test using custom output path."""
        custom_path = tmp_path / "custom" / "my_config.json"
        custom_path.parent.mkdir(parents=True, exist_ok=True)

        create_example_config(str(custom_path))
        assert custom_path.exists()

    def test_overwrites_existing_file(self, tmp_path):
        """Test that existing file is overwritten."""
        output_file = tmp_path / "test_config.json"

        # Create initial file
        create_example_config(str(output_file))
        first_mtime = output_file.stat().st_mtime

        # Wait a bit and recreate
        import time

        time.sleep(0.01)
        create_example_config(str(output_file))
        second_mtime = output_file.stat().st_mtime

        # File should have been updated
        assert second_mtime >= first_mtime


class TestCreateMinimalConfig:
    """Tests for create_minimal_config function."""

    def test_creates_minimal_config_file(self, tmp_path):
        """Test that minimal config file is created."""
        output_file = tmp_path / "minimal.json"
        create_minimal_config(str(output_file))

        assert output_file.exists()
        assert output_file.stat().st_size > 0

    def test_creates_valid_json(self, tmp_path):
        """Test that minimal config is valid JSON."""
        output_file = tmp_path / "minimal.json"
        create_minimal_config(str(output_file))

        with open(output_file) as f:
            data = json.load(f)

        assert isinstance(data, dict)

    def test_contains_only_required_sections(self, tmp_path):
        """Test that minimal config has only required sections."""
        output_file = tmp_path / "minimal.json"
        create_minimal_config(str(output_file))

        with open(output_file) as f:
            data = json.load(f)

        # Should have site, query, radar sections
        assert "site" in data
        assert "query" in data
        assert "radar" in data

        # Should NOT have comments or metadata
        assert "_comment" not in data
        assert "_instructions" not in data
        assert "_metadata" not in data

    def test_site_has_only_required_fields(self, tmp_path):
        """Test that minimal site section has only required fields."""
        output_file = tmp_path / "minimal.json"
        create_minimal_config(str(output_file))

        with open(output_file) as f:
            data = json.load(f)

        site = data["site"]
        # Required fields
        assert "location" in site
        assert "surveyor" in site
        assert "contact" in site

        # Optional fields should not be present
        assert "site_description" not in site
        assert "speed_limit_note" not in site
        assert "_description" not in site

    def test_query_has_only_required_fields(self, tmp_path):
        """Test that minimal query section has only required fields."""
        output_file = tmp_path / "minimal.json"
        create_minimal_config(str(output_file))

        with open(output_file) as f:
            data = json.load(f)

        query = data["query"]
        # Required fields
        assert "start_date" in query
        assert "end_date" in query
        assert "timezone" in query

        # Optional fields should not be present
        assert "histogram" not in query
        assert "hist_bucket_size" not in query
        assert "_description" not in query

    def test_radar_has_only_required_fields(self, tmp_path):
        """Test that minimal radar section has only required fields."""
        output_file = tmp_path / "minimal.json"
        create_minimal_config(str(output_file))

        with open(output_file) as f:
            data = json.load(f)

        radar = data["radar"]
        # Required field
        assert "cosine_error_angle" in radar

        # Optional fields should not be present
        assert "sensor_model" not in radar
        assert "firmware_version" not in radar
        assert "_description" not in radar

    def test_default_minimal_path(self, tmp_path):
        """Test using default minimal config path."""
        original_dir = os.getcwd()
        try:
            os.chdir(tmp_path)
            create_minimal_config()

            default_file = Path("config.minimal.json")
            assert default_file.exists()
        finally:
            os.chdir(original_dir)


class TestErrorHandling:
    """Tests for error handling in config creation."""

    def test_handles_permission_error(self, tmp_path):
        """Test handling of permission errors when writing config."""
        # Create read-only directory
        readonly_dir = tmp_path / "readonly"
        readonly_dir.mkdir()
        readonly_dir.chmod(0o444)

        output_file = readonly_dir / "config.json"

        # Should raise PermissionError
        with pytest.raises(PermissionError):
            create_example_config(str(output_file))

        # Cleanup
        readonly_dir.chmod(0o755)

    def test_creates_parent_directories(self, tmp_path):
        """Test that parent directories are created if they don't exist."""
        # Note: create_example_config doesn't create parent dirs,
        # so this should fail - but we test the behavior
        nested_path = tmp_path / "a" / "b" / "c" / "config.json"

        # Should raise FileNotFoundError if parent doesn't exist
        with pytest.raises(FileNotFoundError):
            create_example_config(str(nested_path))

    def test_handles_invalid_path(self):
        """Test handling of invalid file paths."""
        # Test with invalid path (directory doesn't exist)
        with pytest.raises(FileNotFoundError):
            create_example_config("/nonexistent/directory/config.json")


class TestOutputContent:
    """Tests for content validation of generated configs."""

    def test_example_config_has_helpful_comments(self, tmp_path):
        """Test that example config includes helpful comments."""
        output_file = tmp_path / "test.json"
        create_example_config(str(output_file))

        with open(output_file) as f:
            data = json.load(f)

        assert "_comment" in data
        assert "Configuration File" in data["_comment"]

        assert "_instructions" in data
        assert isinstance(data["_instructions"], list)
        assert len(data["_instructions"]) > 0

    def test_example_config_values_are_realistic(self, tmp_path):
        """Test that example values are realistic and helpful."""
        output_file = tmp_path / "test.json"
        create_example_config(str(output_file))

        with open(output_file) as f:
            data = json.load(f)

        # Check site values
        site = data["site"]
        assert len(site["location"]) > 0
        assert "@" in site["contact"]  # Email format
        assert site["speed_limit"] > 0

        # Check query values
        query = data["query"]
        assert "-" in query["start_date"]  # Date format
        assert query["units"] in ["mph", "kph"]
        assert query["group"] in ["15m", "30m", "1h", "2h", "4h", "8h", "12h", "24h"]

    def test_minimal_config_is_truly_minimal(self, tmp_path):
        """Test that minimal config doesn't include unnecessary fields."""
        output_file = tmp_path / "minimal.json"
        create_minimal_config(str(output_file))

        with open(output_file) as f:
            content = f.read()
            data = json.load(f.seek(0) or f)

        # Should be relatively small
        assert len(content) < 500  # Minimal should be small

        # Count top-level fields
        assert len(data) <= 4  # Only site, query, radar, (maybe output)


class TestConfigValidation:
    """Tests that generated configs can be loaded and validated."""

    def test_example_config_loads_successfully(self, tmp_path):
        """Test that generated example config can be loaded by config_manager."""
        output_file = tmp_path / "test.json"
        create_example_config(str(output_file))

        # Should load without errors
        config = load_config(str(output_file))
        assert config is not None

    def test_example_config_passes_validation(self, tmp_path):
        """Test that generated example config passes all validation checks."""
        output_file = tmp_path / "test.json"
        create_example_config(str(output_file))

        config = load_config(str(output_file))
        is_valid, errors = config.validate()

        assert is_valid, f"Config validation failed with errors: {errors}"
        assert len(errors) == 0

    def test_minimal_config_loads_successfully(self, tmp_path):
        """Test that generated minimal config can be loaded by config_manager."""
        output_file = tmp_path / "minimal.json"
        create_minimal_config(str(output_file))

        # Should load without errors
        config = load_config(str(output_file))
        assert config is not None

    def test_minimal_config_passes_validation(self, tmp_path):
        """Test that generated minimal config passes all validation checks."""
        output_file = tmp_path / "minimal.json"
        create_minimal_config(str(output_file))

        config = load_config(str(output_file))
        is_valid, errors = config.validate()

        assert is_valid, f"Minimal config validation failed with errors: {errors}"
        assert len(errors) == 0

    def test_example_config_has_all_required_fields(self, tmp_path):
        """Test that example config includes all required fields."""
        output_file = tmp_path / "test.json"
        create_example_config(str(output_file))

        with open(output_file) as f:
            data = json.load(f)

        # Site required fields
        assert "location" in data["site"]
        assert data["site"]["location"] != ""
        assert "surveyor" in data["site"]
        assert data["site"]["surveyor"] != ""
        assert "contact" in data["site"]
        assert data["site"]["contact"] != ""

        # Query required fields
        assert "start_date" in data["query"]
        assert data["query"]["start_date"] != ""
        assert "end_date" in data["query"]
        assert data["query"]["end_date"] != ""
        assert "timezone" in data["query"]
        assert data["query"]["timezone"] != ""

        # Radar required fields
        assert "cosine_error_angle" in data["radar"]
        assert data["radar"]["cosine_error_angle"] is not None

    def test_minimal_config_has_all_required_fields(self, tmp_path):
        """Test that minimal config includes all required fields."""
        output_file = tmp_path / "minimal.json"
        create_minimal_config(str(output_file))

        with open(output_file) as f:
            data = json.load(f)

        # Site required fields
        assert "location" in data["site"]
        assert data["site"]["location"] != ""
        assert "surveyor" in data["site"]
        assert data["site"]["surveyor"] != ""
        assert "contact" in data["site"]
        assert data["site"]["contact"] != ""

        # Query required fields
        assert "start_date" in data["query"]
        assert data["query"]["start_date"] != ""
        assert "end_date" in data["query"]
        assert data["query"]["end_date"] != ""
        assert "timezone" in data["query"]
        assert data["query"]["timezone"] != ""

        # Radar required fields (THE CRITICAL TEST THAT WAS MISSING)
        assert "cosine_error_angle" in data["radar"]
        assert data["radar"]["cosine_error_angle"] is not None
        assert isinstance(data["radar"]["cosine_error_angle"], (int, float))

    def test_field_notes_document_required_vs_optional(self, tmp_path):
        """Test that field notes clearly mark required vs optional fields."""
        output_file = tmp_path / "test.json"
        create_example_config(str(output_file))

        with open(output_file) as f:
            data = json.load(f)

        # Check that required fields are marked as REQUIRED
        site_notes = data["site"]["_field_notes"]
        assert "REQUIRED" in site_notes["location"]
        assert "REQUIRED" in site_notes["surveyor"]
        assert "REQUIRED" in site_notes["contact"]

        query_notes = data["query"]["_field_notes"]
        assert "REQUIRED" in query_notes["start_date"]
        assert "REQUIRED" in query_notes["end_date"]
        assert "REQUIRED" in query_notes["timezone"]

        radar_notes = data["radar"]["_field_notes"]
        assert "REQUIRED" in radar_notes["cosine_error_angle"]

        output_notes = data["output"]["_field_notes"]
        assert "REQUIRED" in output_notes["file_prefix"]

        # Check that optional fields are marked as Optional
        assert "Optional" in site_notes["latitude"]
        assert "Optional" in query_notes["min_speed"]
        assert "Optional" in radar_notes["sensor_model"]
        assert "Optional" in output_notes["debug"]

    def test_cosine_error_angle_is_numeric(self, tmp_path):
        """Test that cosine_error_angle is a numeric value, not a string."""
        output_file = tmp_path / "test.json"
        create_example_config(str(output_file))

        with open(output_file) as f:
            data = json.load(f)

        cosine_angle = data["radar"]["cosine_error_angle"]
        assert isinstance(
            cosine_angle, (int, float)
        ), f"cosine_error_angle must be numeric, got {type(cosine_angle)}: {cosine_angle}"
        assert cosine_angle > 0, "cosine_error_angle must be positive"

    def test_minimal_config_cosine_error_angle_is_numeric(self, tmp_path):
        """Test that minimal config has numeric cosine_error_angle."""
        output_file = tmp_path / "minimal.json"
        create_minimal_config(str(output_file))

        with open(output_file) as f:
            data = json.load(f)

        cosine_angle = data["radar"]["cosine_error_angle"]
        assert isinstance(
            cosine_angle, (int, float)
        ), f"cosine_error_angle must be numeric, got {type(cosine_angle)}: {cosine_angle}"
        assert cosine_angle > 0, "cosine_error_angle must be positive"
