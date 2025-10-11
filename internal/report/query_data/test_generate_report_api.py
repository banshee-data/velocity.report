#!/usr/bin/env python3
"""Tests for generate_report_api.py module."""
import unittest
import tempfile
import os
import json
from unittest.mock import patch, MagicMock, call
from pathlib import Path

from generate_report_api import (
    generate_report_from_dict,
    generate_report_from_file,
    generate_report_from_config,
    ReportGenerationError,
)
from config_manager import (
    ReportConfig,
    SiteConfig,
    QueryConfig,
    RadarConfig,
    OutputConfig,
)


class TestGenerateReportFromDict(unittest.TestCase):
    """Test cases for generate_report_from_dict function."""

    def setUp(self):
        """Set up test fixtures."""
        self.valid_config = {
            "site": {
                "location": "Test Location",
                "surveyor": "Test Surveyor",
                "contact": "test@example.com",
            },
            "radar": {"cosine_error_angle": 20.0},
            "query": {
                "start_date": "2025-06-01",
                "end_date": "2025-06-07",
                "timezone": "UTC",
            },
            "output": {"file_prefix": "test"},
        }

    @patch("generate_report_api.get_stats")
    @patch("generate_report_api.os.makedirs")
    @patch("generate_report_api.os.chdir")
    @patch("generate_report_api.os.getcwd")
    def test_generate_report_from_dict_success(
        self, mock_getcwd, mock_chdir, mock_makedirs, mock_get_stats
    ):
        """Test successful report generation via API."""
        mock_getcwd.return_value = "/original/dir"
        mock_get_stats.main.return_value = None

        result = generate_report_from_dict(self.valid_config)

        self.assertTrue(result["success"])
        self.assertEqual(result["prefix"], "test")
        self.assertIsInstance(result["files"], list)
        self.assertEqual(len(result["errors"]), 0)
        self.assertIn("config_used", result)
        mock_get_stats.main.assert_called_once()

    def test_generate_report_from_dict_invalid_config_missing_fields(self):
        """Test API with missing required config fields."""
        incomplete_config = {
            "site": {"location": "Test"},
            # Missing radar, query sections
        }

        result = generate_report_from_dict(incomplete_config)

        self.assertFalse(result["success"])
        self.assertGreater(len(result["errors"]), 0)
        self.assertIn("config_used", result)

    def test_generate_report_from_dict_invalid_dict_structure(self):
        """Test dict API with completely invalid dictionary."""
        result = generate_report_from_dict({"invalid": "config", "random": "data"})
        assert result["success"] is False
        assert "is required" in result["errors"][0]
        assert result["config_used"]["site"]["location"] == ""

    def test_generate_report_from_dict_not_a_dict(self):
        """Test dict API with non-dict input that triggers exception."""
        # Pass something that's not a dict to trigger the exception path
        result = generate_report_from_dict("not a dict")
        assert result["success"] is False
        assert "Invalid configuration dictionary" in result["errors"][0]
        assert result["config_used"] == "not a dict"

    @patch("generate_report_api.get_stats")
    @patch("generate_report_api.os.makedirs")
    @patch("generate_report_api.os.chdir")
    @patch("generate_report_api.os.getcwd")
    def test_generate_report_from_dict_database_error(
        self, mock_getcwd, mock_chdir, mock_makedirs, mock_get_stats
    ):
        """Test API behavior when database query fails."""
        mock_getcwd.return_value = "/original/dir"
        mock_get_stats.main.side_effect = Exception("Database connection failed")

        result = generate_report_from_dict(self.valid_config)

        self.assertFalse(result["success"])
        self.assertGreater(len(result["errors"]), 0)
        self.assertTrue(
            any(
                "Database" in str(e) or "connection" in str(e) for e in result["errors"]
            )
        )

    def test_generate_report_from_dict_validation_failure(self):
        """Test API with config that fails validation (missing required fields)."""
        # Config missing required timezone field (will fail validation)
        invalid_config = self.valid_config.copy()
        invalid_config["query"] = {
            "start_date": "2025-06-01",
            "end_date": "2025-06-07",
            # Missing timezone (required field)
        }

        result = generate_report_from_dict(invalid_config)

        self.assertFalse(result["success"])
        self.assertGreater(len(result["errors"]), 0)


class TestGenerateReportFromFile(unittest.TestCase):
    """Test cases for generate_report_from_file function."""

    def setUp(self):
        """Set up test fixtures."""
        self.valid_config = {
            "site": {
                "location": "Test Location",
                "surveyor": "Test Surveyor",
                "contact": "test@example.com",
            },
            "radar": {"cosine_error_angle": 20.0},
            "query": {
                "start_date": "2025-06-01",
                "end_date": "2025-06-07",
                "timezone": "UTC",
            },
            "output": {"file_prefix": "test"},
        }

    def test_generate_report_from_file_not_found(self):
        """Test API error when config file doesn't exist."""
        result = generate_report_from_file("/nonexistent/config.json")

        self.assertFalse(result["success"])
        self.assertGreater(len(result["errors"]), 0)
        self.assertTrue(any("not found" in str(e).lower() for e in result["errors"]))

    @patch("generate_report_api.get_stats")
    @patch("generate_report_api.os.makedirs")
    @patch("generate_report_api.os.chdir")
    @patch("generate_report_api.os.getcwd")
    def test_generate_report_from_file_success(
        self, mock_getcwd, mock_chdir, mock_makedirs, mock_get_stats
    ):
        """Test successful report generation from file."""
        mock_getcwd.return_value = "/original/dir"
        mock_get_stats.main.return_value = None

        with tempfile.NamedTemporaryFile(mode="w", suffix=".json", delete=False) as f:
            json.dump(self.valid_config, f)
            temp_file = f.name

        try:
            result = generate_report_from_file(temp_file)

            self.assertTrue(result["success"])
            self.assertEqual(result["prefix"], "test")
            mock_get_stats.main.assert_called_once()
        finally:
            os.unlink(temp_file)

    def test_generate_report_from_file_invalid_json(self):
        """Test API with malformed JSON file."""
        with tempfile.NamedTemporaryFile(mode="w", suffix=".json", delete=False) as f:
            f.write("{invalid json content}")
            temp_file = f.name

        try:
            result = generate_report_from_file(temp_file)

            self.assertFalse(result["success"])
            self.assertGreater(len(result["errors"]), 0)
            self.assertTrue(any("Failed to load" in str(e) for e in result["errors"]))
        finally:
            os.unlink(temp_file)


class TestGenerateReportFromConfig(unittest.TestCase):
    """Test cases for generate_report_from_config function."""

    def setUp(self):
        """Set up test fixtures."""
        self.valid_config = ReportConfig(
            site=SiteConfig(
                location="Test Location",
                surveyor="Test Surveyor",
                contact="test@example.com",
            ),
            radar=RadarConfig(cosine_error_angle=20.0),
            query=QueryConfig(
                start_date="2025-06-01",
                end_date="2025-06-07",
                timezone="UTC",
            ),
            output=OutputConfig(file_prefix="test"),
        )

    @patch("generate_report_api.get_stats")
    @patch("generate_report_api.os.makedirs")
    @patch("generate_report_api.os.chdir")
    @patch("generate_report_api.os.getcwd")
    def test_generate_report_from_config_success(
        self, mock_getcwd, mock_chdir, mock_makedirs, mock_get_stats
    ):
        """Test successful report generation from ReportConfig object."""
        mock_getcwd.return_value = "/original/dir"
        mock_get_stats.main.return_value = None

        result = generate_report_from_config(self.valid_config)

        self.assertTrue(result["success"])
        self.assertEqual(result["prefix"], "test")
        self.assertEqual(len(result["errors"]), 0)
        mock_get_stats.main.assert_called_once()

    @patch("generate_report_api.get_stats")
    @patch("generate_report_api.os.makedirs")
    @patch("generate_report_api.os.chdir")
    @patch("generate_report_api.os.getcwd")
    def test_generate_report_from_config_with_output_dir_override(
        self, mock_getcwd, mock_chdir, mock_makedirs, mock_get_stats
    ):
        """Test that output_dir parameter overrides config value."""
        mock_getcwd.return_value = "/original/dir"
        mock_get_stats.main.return_value = None

        custom_output = "/custom/output/dir"
        result = generate_report_from_config(
            self.valid_config, output_dir=custom_output
        )

        self.assertTrue(result["success"])
        # Verify makedirs was called with custom directory
        mock_makedirs.assert_called_with(custom_output, exist_ok=True)
        # Verify chdir was called with custom directory
        self.assertIn(call(custom_output), mock_chdir.call_args_list)

    def test_generate_report_from_config_validation_failure(self):
        """Test with config that fails validation."""
        # Create invalid config (missing required fields)
        invalid_config = ReportConfig()

        result = generate_report_from_config(invalid_config)

        self.assertFalse(result["success"])
        self.assertGreater(len(result["errors"]), 0)

    @patch("generate_report_api.get_stats")
    @patch("generate_report_api.os.makedirs")
    @patch("generate_report_api.os.chdir")
    @patch("generate_report_api.os.getcwd")
    def test_generate_report_from_config_restores_directory_on_error(
        self, mock_getcwd, mock_chdir, mock_makedirs, mock_get_stats
    ):
        """Test that original directory is restored even if generation fails."""
        original_dir = "/original/dir"
        mock_getcwd.return_value = original_dir
        mock_get_stats.main.side_effect = Exception("Generation failed")

        result = generate_report_from_config(self.valid_config)

        self.assertFalse(result["success"])
        # Verify that we restored to original directory after error
        self.assertIn(call(original_dir), mock_chdir.call_args_list)

    @patch("generate_report_api.get_stats")
    @patch("generate_report_api.os.makedirs")
    @patch("generate_report_api.os.chdir")
    @patch("generate_report_api.os.getcwd")
    def test_generate_report_from_config_debug_includes_traceback(
        self, mock_getcwd, mock_chdir, mock_makedirs, mock_get_stats
    ):
        """Test that debug mode includes traceback in errors."""
        mock_getcwd.return_value = "/original/dir"
        mock_get_stats.main.side_effect = Exception("Test error")

        # Enable debug mode
        self.valid_config.output.debug = True

        result = generate_report_from_config(self.valid_config)

        self.assertFalse(result["success"])
        self.assertIn("traceback", result)
        self.assertIsInstance(result["traceback"], str)
        self.assertIn("Test error", result["traceback"])


if __name__ == "__main__":
    unittest.main()
