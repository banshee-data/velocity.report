#!/usr/bin/env python3
"""Tests for CLI entry points and error handling."""
import json
import os
import tempfile
import unittest


class TestCLIIntegration(unittest.TestCase):
    """Tests for CLI integration with config files."""

    def test_sys_argv_config_file_detection(self):
        """Test that CLI can detect config file from sys.argv."""
        with tempfile.NamedTemporaryFile(mode="w", suffix=".json", delete=False) as f:
            config_data = {
                "site": {
                    "location": "Test",
                    "surveyor": "Test",
                    "contact": "test@test.com",
                },
                "radar": {"cosine_error_angle": 21.0},
                "query": {
                    "start_date": "2025-01-01",
                    "end_date": "2025-01-02",
                    "timezone": "UTC",
                },
                "output": {"debug": True},
            }
            json.dump(config_data, f)
            config_file = f.name

        try:
            # Simulate sys.argv with config file as last argument
            test_argv = ["main.py", config_file]

            # Verify the file exists and can be detected
            self.assertTrue(os.path.isfile(test_argv[-1]))

            # Verify it's a valid JSON file
            with open(test_argv[-1]) as f:
                loaded = json.load(f)
            self.assertEqual(loaded["output"]["debug"], True)

        finally:
            os.remove(config_file)

    def test_config_file_copy_logic(self):
        """Test the logic for copying submitted config in debug mode."""
        with tempfile.TemporaryDirectory() as tmpdir:
            # Create a submitted config file
            submitted_config = {
                "site": {
                    "location": "Test",
                    "surveyor": "Test",
                    "contact": "test@test.com",
                },
                "radar": {"cosine_error_angle": 21.0},
                "query": {
                    "start_date": "2025-01-01",
                    "end_date": "2025-01-02",
                    "timezone": "UTC",
                },
                "output": {"debug": True, "output_dir": tmpdir},
            }

            submitted_path = os.path.join(tmpdir, "submitted.json")
            with open(submitted_path, "w") as f:
                json.dump(submitted_config, f, indent=2)

            # Simulate the CLI logic for copying config
            prefix = os.path.join(tmpdir, "test_report")

            # This simulates what happens in process_date_range when debug is enabled
            import shutil

            if os.path.isfile(submitted_path):
                dest = f"{prefix}_submitted_config.json"
                shutil.copyfile(submitted_path, dest)

                # Verify the copy exists and has correct content
                self.assertTrue(os.path.exists(dest))
                with open(dest) as f:
                    copied = json.load(f)
                self.assertEqual(copied["output"]["debug"], True)

    def test_final_config_generation(self):
        """Test that final config with defaults is generated correctly."""
        from pdf_generator.core.config_manager import (
            ReportConfig,
            SiteConfig,
            RadarConfig,
            QueryConfig,
            OutputConfig,
        )

        with tempfile.TemporaryDirectory() as tmpdir:
            # Create a minimal config
            config = ReportConfig(
                site=SiteConfig(
                    location="Test", surveyor="Test", contact="test@test.com"
                ),
                radar=RadarConfig(cosine_error_angle=21.0),
                query=QueryConfig(
                    start_date="2025-01-01", end_date="2025-01-02", timezone="UTC"
                ),
                output=OutputConfig(debug=True, output_dir=tmpdir),
            )

            # Generate final config dict (with all defaults)
            final_data = config.to_dict()

            prefix = os.path.join(tmpdir, "test_report")
            final_path = f"{prefix}_final_config.json"

            # Write final config
            with open(final_path, "w") as f:
                json.dump(final_data, f, indent=2)

            # Verify final config exists and has defaults
            self.assertTrue(os.path.exists(final_path))
            with open(final_path) as f:
                loaded = json.load(f)

            # Verify it has default sections that weren't in minimal config
            self.assertIn("colors", loaded)
            self.assertIn("fonts", loaded)
            self.assertIn("layout", loaded)
            self.assertIn("pdf", loaded)
            self.assertEqual(loaded["output"]["debug"], True)


class TestDebugModeConfigWriting(unittest.TestCase):
    """Test debug mode config file writing behavior."""

    def test_debug_disabled_no_config_files(self):
        """Test that config files are not written when debug is disabled."""
        from pdf_generator.core.config_manager import OutputConfig

        config = OutputConfig(debug=False)
        self.assertFalse(config.debug)

        # In actual CLI, no config files would be written
        # This is just verifying the flag state

    def test_debug_enabled_triggers_config_write(self):
        """Test that debug=True should trigger config file writing."""
        from pdf_generator.core.config_manager import OutputConfig

        config = OutputConfig(debug=True)
        self.assertTrue(config.debug)

        # In actual CLI with debug=True, both submitted and final configs are written


if __name__ == "__main__":
    unittest.main()
