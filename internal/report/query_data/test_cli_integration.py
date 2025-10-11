#!/usr/bin/env python3
"""Tests for CLI entry points and error handling."""
import unittest
import subprocess
import sys
import os
import json
import tempfile


class TestGenerateReportAPICLI(unittest.TestCase):
    """Test CLI functionality of generate_report_api.py."""

    def test_cli_help_message(self):
        """Test that --help displays usage information."""
        result = subprocess.run(
            [sys.executable, "generate_report_api.py", "--help"],
            capture_output=True,
            text=True,
        )

        self.assertEqual(result.returncode, 0)
        self.assertIn("config_file", result.stdout)
        self.assertIn("output-dir", result.stdout)
        self.assertIn("json", result.stdout)

    def test_cli_missing_config_file(self):
        """Test CLI error when config file argument is missing."""
        result = subprocess.run(
            [sys.executable, "generate_report_api.py"],
            capture_output=True,
            text=True,
        )

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("required", result.stderr.lower())

    def test_cli_nonexistent_config_file(self):
        """Test CLI with non-existent config file."""
        result = subprocess.run(
            [sys.executable, "generate_report_api.py", "/nonexistent/config.json"],
            capture_output=True,
            text=True,
        )

        self.assertNotEqual(result.returncode, 0)
        output = result.stdout + result.stderr
        self.assertTrue("not found" in output.lower() or "failed" in output.lower())

    def test_cli_with_valid_config_json_output(self):
        """Test CLI with valid config and JSON output format."""
        config_data = {
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
            "output": {"file_prefix": "cli-test"},
        }

        with tempfile.NamedTemporaryFile(mode="w", suffix=".json", delete=False) as f:
            json.dump(config_data, f)
            temp_file = f.name

        try:
            # Note: This will fail if database is not available, but we're testing JSON output format
            result = subprocess.run(
                [sys.executable, "generate_report_api.py", temp_file, "--json"],
                capture_output=True,
                text=True,
                timeout=5,
            )

            # The --json flag should output JSON (may have other text before it)
            # Find where JSON starts by looking for the first complete JSON object
            stdout = result.stdout

            # Try to find JSON by looking for lines that start with {
            lines = stdout.split("\n")
            json_start_line = -1
            for i, line in enumerate(lines):
                if line.strip().startswith("{"):
                    json_start_line = i
                    break

            self.assertNotEqual(
                json_start_line,
                -1,
                "No JSON object found in output when using --json flag",
            )

            # Get everything from that line onwards
            json_portion = "\n".join(lines[json_start_line:])
            output_data = json.loads(json_portion)

            # Verify JSON structure
            self.assertIn("success", output_data)
            self.assertIn("errors", output_data)
            self.assertIn("files", output_data)
        finally:
            os.unlink(temp_file)


if __name__ == "__main__":
    unittest.main()
