#!/usr/bin/env python3
"""Tests for demo.py CLI tool."""

import json
import os
import tempfile
from unittest.mock import patch
from io import StringIO

import pytest

from pdf_generator.cli.demo import (
    demo_create_config,
    demo_save_load_json,
    demo_validation,
    demo_dict_conversion,
    demo_api_usage,
    main,
)
from pdf_generator.core.config_manager import ReportConfig


class TestDemoCreateConfig:
    """Tests for demo_create_config function."""

    @patch("sys.stdout", new_callable=StringIO)
    def test_creates_and_returns_config(self, mock_stdout):
        """Test that demo creates and returns a ReportConfig."""
        config = demo_create_config()

        assert isinstance(config, ReportConfig)
        assert config.site.location == "Main Street, Springfield"
        assert config.query.units == "mph"

    @patch("sys.stdout", new_callable=StringIO)
    def test_prints_config_summary(self, mock_stdout):
        """Test that demo prints configuration summary."""
        _ = demo_create_config()

        output = mock_stdout.getvalue()
        assert "Creating configuration" in output
        assert "Main Street, Springfield" in output
        assert "mph" in output

    @patch("sys.stdout", new_callable=StringIO)
    def test_config_has_all_sections(self, mock_stdout):
        """Test that created config has all required sections."""
        config = demo_create_config()

        assert config.site is not None
        assert config.query is not None
        assert config.output is not None
        assert config.radar is not None

    @patch("sys.stdout", new_callable=StringIO)
    def test_config_is_valid(self, mock_stdout):
        """Test that created config passes validation."""
        config = demo_create_config()

        is_valid, errors = config.validate()
        assert is_valid
        assert len(errors) == 0


class TestDemoSaveLoadJson:
    """Tests for demo_save_load_json function."""

    @patch("sys.stdout", new_callable=StringIO)
    def test_saves_config_to_file(self, mock_stdout):
        """Test that config is saved to temporary file."""
        config = ReportConfig()
        config.site.location = "Test Location"

        temp_path = demo_save_load_json(config)

        # Check file exists
        assert os.path.exists(temp_path)

        # Cleanup
        os.unlink(temp_path)

    @patch("sys.stdout", new_callable=StringIO)
    def test_returns_temp_file_path(self, mock_stdout):
        """Test that function returns the temp file path."""
        config = ReportConfig()

        temp_path = demo_save_load_json(config)

        assert temp_path.endswith(".json")
        assert os.path.isfile(temp_path)

        # Cleanup
        os.unlink(temp_path)

    @patch("sys.stdout", new_callable=StringIO)
    def test_saved_file_is_valid_json(self, mock_stdout):
        """Test that saved file contains valid JSON."""
        config = ReportConfig()
        config.site.location = "Test Location"

        temp_path = demo_save_load_json(config)

        with open(temp_path) as f:
            data = json.load(f)

        assert isinstance(data, dict)
        assert "site" in data

        # Cleanup
        os.unlink(temp_path)

    @patch("sys.stdout", new_callable=StringIO)
    def test_loads_config_from_file(self, mock_stdout):
        """Test that config is loaded back from file."""
        config = ReportConfig()
        config.site.location = "Unique Test Location"

        temp_path = demo_save_load_json(config)

        # Read the file to verify loading worked
        loaded_config = ReportConfig.from_json(temp_path)
        assert loaded_config.site.location == "Unique Test Location"

        # Cleanup
        os.unlink(temp_path)

    @patch("sys.stdout", new_callable=StringIO)
    def test_prints_file_preview(self, mock_stdout):
        """Test that function prints preview of JSON file."""
        config = ReportConfig()

        temp_path = demo_save_load_json(config)
        output = mock_stdout.getvalue()

        assert "Saved config" in output
        assert "First 10 lines" in output

        # Cleanup
        os.unlink(temp_path)

    @patch("sys.stdout", new_callable=StringIO)
    def test_prints_loaded_confirmation(self, mock_stdout):
        """Test that function confirms loading."""
        config = ReportConfig()
        config.site.location = "Test Location"

        temp_path = demo_save_load_json(config)
        output = mock_stdout.getvalue()

        assert "Loaded config" in output
        assert "Test Location" in output

        # Cleanup
        os.unlink(temp_path)


class TestDemoValidation:
    """Tests for demo_validation function."""

    @patch("sys.stdout", new_callable=StringIO)
    def test_validates_valid_config(self, mock_stdout):
        """Test validation of a valid config."""
        config = demo_create_config()
        demo_validation(config)

        output = mock_stdout.getvalue().lower()
        assert "valid config: true" in output

    @patch("sys.stdout", new_callable=StringIO)
    def test_demonstrates_invalid_config(self, mock_stdout):
        """Test validation of an invalid config."""
        config = demo_create_config()
        demo_validation(config)

        output = mock_stdout.getvalue().lower()
        # Should show both valid and invalid examples
        assert "invalid config" in output
        assert "errors:" in output

    @patch("sys.stdout", new_callable=StringIO)
    def test_shows_error_messages(self, mock_stdout):
        """Test that validation errors are displayed."""
        config = demo_create_config()
        demo_validation(config)

        output = mock_stdout.getvalue()
        # Invalid config section should show errors
        assert "start_date" in output.lower() or "end_date" in output.lower()


class TestDemoDictConversion:
    """Tests for demo_dict_conversion function."""

    @patch("sys.stdout", new_callable=StringIO)
    def test_converts_config_to_dict(self, mock_stdout):
        """Test conversion of config to dictionary."""
        config = demo_create_config()
        config_dict = demo_dict_conversion(config)

        assert isinstance(config_dict, dict)
        assert "site" in config_dict
        assert "query" in config_dict
        assert "output" in config_dict

    @patch("sys.stdout", new_callable=StringIO)
    def test_prints_dict_structure(self, mock_stdout):
        """Test that dict structure is printed."""
        config = demo_create_config()
        demo_dict_conversion(config)

        output = mock_stdout.getvalue()
        assert "dictionary" in output.lower()
        assert "Keys:" in output

    @patch("sys.stdout", new_callable=StringIO)
    def test_shows_site_configuration(self, mock_stdout):
        """Test that site configuration is displayed."""
        config = demo_create_config()
        demo_dict_conversion(config)

        output = mock_stdout.getvalue()
        assert "Site configuration" in output

    @patch("sys.stdout", new_callable=StringIO)
    def test_reconstructs_from_dict(self, mock_stdout):
        """Test that config can be reconstructed from dict."""
        config = demo_create_config()
        config.site.location = "Unique Location"

        _ = demo_dict_conversion(config)

        # Should print confirmation of reconstruction
        output = mock_stdout.getvalue()
        assert "Reconstructed" in output
        assert "Unique Location" in output

    @patch("sys.stdout", new_callable=StringIO)
    def test_returns_config_dict(self, mock_stdout):
        """Test that function returns the config dictionary."""
        config = demo_create_config()
        result = demo_dict_conversion(config)

        assert isinstance(result, dict)
        assert result["site"]["location"] == config.site.location


class TestDemoApiUsage:
    """Tests for demo_api_usage function."""

    @patch("sys.stdout", new_callable=StringIO)
    def test_prints_go_integration_example(self, mock_stdout):
        """Test that Go integration example is printed."""
        config_dict = {"site": {"location": "Test"}}
        demo_api_usage(config_dict)

        output = mock_stdout.getvalue()
        assert "Go server" in output
        assert "map[string]interface{}" in output

    @patch("sys.stdout", new_callable=StringIO)
    def test_shows_example_go_code(self, mock_stdout):
        """Test that example Go code is displayed."""
        config_dict = {}
        demo_api_usage(config_dict)

        output = mock_stdout.getvalue()
        assert "config :=" in output
        assert "site" in output.lower()
        assert "query" in output.lower()

    @patch("sys.stdout", new_callable=StringIO)
    def test_shows_python_cli_usage(self, mock_stdout):
        """Test that Python CLI usage is shown."""
        config_dict = {}
        demo_api_usage(config_dict)

        output = mock_stdout.getvalue()
        assert "get_stats.py" in output


class TestMainFunction:
    """Tests for main demo function."""

    @patch("sys.stdout", new_callable=StringIO)
    def test_runs_all_demos(self, mock_stdout):
        """Test that main runs all demo functions."""
        main()

        output = mock_stdout.getvalue()

        # Should contain output from all demos
        assert "DEMO 1" in output
        assert "DEMO 2" in output
        assert "DEMO 3" in output
        assert "DEMO 4" in output
        assert "DEMO 5" in output

    @patch("sys.stdout", new_callable=StringIO)
    def test_prints_header(self, mock_stdout):
        """Test that demo prints header."""
        main()

        output = mock_stdout.getvalue()
        assert "CONFIGURATION SYSTEM DEMO" in output

    @patch("sys.stdout", new_callable=StringIO)
    def test_prints_summary(self, mock_stdout):
        """Test that demo prints summary."""
        main()

        output = mock_stdout.getvalue()
        assert "SUMMARY" in output
        assert "configuration system supports" in output.lower()

    @patch("sys.stdout", new_callable=StringIO)
    def test_prints_next_steps(self, mock_stdout):
        """Test that demo prints next steps."""
        main()

        output = mock_stdout.getvalue()
        assert "Next steps" in output
        assert "config.example.json" in output

    @patch("sys.stdout", new_callable=StringIO)
    def test_demonstrates_all_features(self, mock_stdout):
        """Test that all configuration features are demonstrated."""
        main()

        output = mock_stdout.getvalue()

        # Check for key features
        assert "Creating configs" in output or "Creating configuration" in output
        assert "Saving" in output or "JSON" in output
        assert "Validation" in output
        assert "Dictionary" in output
        assert "Go server" in output

    @patch("sys.stdout", new_callable=StringIO)
    def test_cleans_up_temp_files(self, mock_stdout):
        """Test that temporary files are cleaned up."""
        # Get initial temp file count
        temp_dir = tempfile.gettempdir()
        initial_files = set(os.listdir(temp_dir))

        main()

        # Check that no new JSON files are left
        final_files = set(os.listdir(temp_dir))
        new_json_files = [
            f for f in (final_files - initial_files) if f.endswith(".json")
        ]

        assert len(new_json_files) == 0

    @patch("sys.stdout", new_callable=StringIO)
    def test_completes_successfully(self, mock_stdout):
        """Test that demo completes without errors."""
        main()

        output = mock_stdout.getvalue()
        assert "Demo complete" in output


class TestDemoOutputFormatting:
    """Tests for demo output formatting."""

    @patch("sys.stdout", new_callable=StringIO)
    def test_uses_section_dividers(self, mock_stdout):
        """Test that demo uses visual section dividers."""
        main()

        output = mock_stdout.getvalue()
        assert "=" * 70 in output  # Section dividers

    @patch("sys.stdout", new_callable=StringIO)
    def test_uses_checkmarks(self, mock_stdout):
        """Test that demo uses checkmarks for completed items."""
        main()

        output = mock_stdout.getvalue()
        assert "✅" in output

    @patch("sys.stdout", new_callable=StringIO)
    def test_indents_nested_output(self, mock_stdout):
        """Test that nested output is properly indented."""
        main()

        output = mock_stdout.getvalue()
        # Check for indentation patterns
        lines = output.split("\n")
        indented_lines = [line for line in lines if line.startswith("  ")]
        assert len(indented_lines) > 5  # Should have several indented lines


class TestDemoIntegration:
    """Integration tests for demo module."""

    @patch("sys.stdout", new_callable=StringIO)
    def test_full_demo_workflow(self, mock_stdout):
        """Test complete demo workflow from start to finish."""
        # Run main demo
        main()

        output = mock_stdout.getvalue()

        # Verify workflow steps
        assert "Creating configuration" in output
        assert "Save to and load from JSON" in output
        assert "validation" in output.lower()
        assert "Dictionary conversion" in output
        assert "Go server integration" in output
        assert "SUMMARY" in output
        assert "Demo complete" in output

    @patch("sys.stdout", new_callable=StringIO)
    def test_demo_shows_all_config_methods(self, mock_stdout):
        """Test that demo shows all ways to create configs."""
        main()

        output = mock_stdout.getvalue()

        # Check for all configuration methods
        assert "JSON files" in output
        assert "Dictionaries" in output
        assert "Programmatic" in output or "from scratch" in output

    @patch("sys.stdout", new_callable=StringIO)
    def test_demo_educational_value(self, mock_stdout):
        """Test that demo provides educational value."""
        main()

        output = mock_stdout.getvalue()

        # Should explain concepts
        assert "configuration system" in output.lower()
        assert "load_config" in output
        assert "from_dict" in output
        assert "ReportConfig" in output


class TestErrorHandlingInDemo:
    """Tests for error handling in demo functions."""

    @patch("sys.stdout", new_callable=StringIO)
    def test_handles_temp_file_creation_gracefully(self, mock_stdout):
        """Test graceful handling of temp file creation issues."""
        config = ReportConfig()

        # Should not raise exception
        try:
            temp_path = demo_save_load_json(config)
            os.unlink(temp_path)
        except Exception as e:
            pytest.fail(f"Demo should handle temp files gracefully: {e}")

    @patch("sys.stdout", new_callable=StringIO)
    def test_validation_demo_handles_errors(self, mock_stdout):
        """Test that validation demo handles errors without crashing."""
        config = ReportConfig()

        # Should not raise exception even with invalid config
        try:
            demo_validation(config)
        except Exception as e:
            pytest.fail(f"Validation demo should not crash: {e}")


class TestDemoAsModule:
    """Tests for running demo as a module."""

    @patch("sys.stdout", new_callable=StringIO)
    def test_can_import_and_run(self, mock_stdout):
        """Test that demo can be imported and run."""
        from pdf_generator.cli import demo

        # Should be able to call main
        demo.main()

        output = mock_stdout.getvalue()
        assert "DEMO" in output

    @patch("sys.stdout", new_callable=StringIO)
    def test_individual_functions_can_be_called(self, mock_stdout):
        """Test that individual demo functions can be called."""
        from pdf_generator.cli.demo import demo_create_config

        config = demo_create_config()
        assert isinstance(config, ReportConfig)
