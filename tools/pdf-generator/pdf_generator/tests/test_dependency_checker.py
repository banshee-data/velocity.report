#!/usr/bin/env python3
"""Unit tests for dependency_checker.py."""

import unittest
from unittest.mock import patch, MagicMock
import sys
import os

from pdf_generator.core.dependency_checker import (
    DependencyChecker,
    DependencyCheckResult,
    check_dependencies,
)


class TestDependencyCheckResult(unittest.TestCase):
    """Tests for DependencyCheckResult class."""

    def test_available_dependency_repr(self):
        """Test string representation of available dependency."""
        result = DependencyCheckResult(
            "matplotlib", True, "Chart generation (v3.0)", critical=True
        )
        repr_str = repr(result)

        self.assertIn("✓", repr_str)
        self.assertIn("matplotlib", repr_str)
        self.assertIn("Chart generation", repr_str)
        self.assertNotIn("CRITICAL", repr_str)  # Don't show critical for available deps

    def test_missing_critical_dependency_repr(self):
        """Test string representation of missing critical dependency."""
        result = DependencyCheckResult(
            "pylatex", False, "Install with pip", critical=True
        )
        repr_str = repr(result)

        self.assertIn("✗", repr_str)
        self.assertIn("pylatex", repr_str)
        self.assertIn("CRITICAL", repr_str)

    def test_missing_optional_dependency_repr(self):
        """Test string representation of missing optional dependency."""
        result = DependencyCheckResult("cairosvg", False, "Optional", critical=False)
        repr_str = repr(result)

        self.assertIn("✗", repr_str)
        self.assertIn("cairosvg", repr_str)
        self.assertNotIn("CRITICAL", repr_str)


class TestDependencyChecker(unittest.TestCase):
    """Tests for DependencyChecker class."""

    def test_check_python_version_success(self):
        """Test Python version check passes for current Python."""
        checker = DependencyChecker()
        checker._check_python_version()

        self.assertEqual(len(checker.results), 1)
        result = checker.results[0]
        self.assertEqual(result.name, "Python Version")
        self.assertTrue(result.available)  # Current Python should be 3.9+

    def test_check_venv_detected(self):
        """Test virtual environment detection."""
        checker = DependencyChecker()

        # Mock venv detection
        with patch.dict(os.environ, {"VIRTUAL_ENV": "/fake/venv"}):
            checker._check_venv()

        self.assertEqual(len(checker.results), 1)
        result = checker.results[0]
        self.assertEqual(result.name, "Virtual Environment")

    def test_check_python_package_available(self):
        """Test checking for an available Python package."""
        checker = DependencyChecker()

        # sys should always be available
        with patch("importlib.util.find_spec") as mock_find_spec:
            mock_find_spec.return_value = MagicMock()

            checker._check_python_package("sys", "System module", critical=True)

        self.assertEqual(len(checker.results), 1)
        result = checker.results[0]
        self.assertEqual(result.name, "sys")

    def test_check_python_package_missing(self):
        """Test checking for a missing Python package."""
        checker = DependencyChecker()

        with patch("importlib.util.find_spec") as mock_find_spec:
            mock_find_spec.return_value = None

            checker._check_python_package("nonexistent", "Fake package", critical=True)

        self.assertEqual(len(checker.results), 1)
        result = checker.results[0]
        self.assertEqual(result.name, "nonexistent")
        self.assertFalse(result.available)
        self.assertTrue(result.critical)

    def test_check_latex_available(self):
        """Test LaTeX check when xelatex is available."""
        checker = DependencyChecker()

        with patch("shutil.which") as mock_which:
            mock_which.return_value = "/usr/bin/xelatex"

            with patch("subprocess.run") as mock_run:
                mock_result = MagicMock()
                mock_result.stdout = "XeTeX 3.14159265\n"
                mock_run.return_value = mock_result

                checker._check_latex()

        self.assertEqual(len(checker.results), 1)
        result = checker.results[0]
        self.assertEqual(result.name, "xelatex (LaTeX)")
        self.assertTrue(result.available)

    def test_check_latex_missing(self):
        """Test LaTeX check when xelatex is not available."""
        checker = DependencyChecker()

        with patch("shutil.which") as mock_which:
            mock_which.return_value = None

            checker._check_latex()

        self.assertEqual(len(checker.results), 1)
        result = checker.results[0]
        self.assertEqual(result.name, "xelatex (LaTeX)")
        self.assertFalse(result.available)
        self.assertTrue(result.critical)
        self.assertIn("Install", result.details)

    def test_check_system_command_available(self):
        """Test system command check when command exists."""
        checker = DependencyChecker()

        with patch("shutil.which") as mock_which:
            mock_which.return_value = "/usr/bin/inkscape"

            checker._check_system_command("inkscape", "SVG tool", critical=False)

        self.assertEqual(len(checker.results), 1)
        result = checker.results[0]
        self.assertEqual(result.name, "inkscape")
        self.assertTrue(result.available)

    def test_check_all_returns_tuple(self):
        """Test that check_all returns proper tuple."""
        checker = DependencyChecker()

        all_ok, results = checker.check_all()

        self.assertIsInstance(all_ok, bool)
        self.assertIsInstance(results, list)
        self.assertGreater(len(results), 0)

    def test_print_results_handles_empty_list(self):
        """Test print_results with empty list."""
        checker = DependencyChecker()

        # Should not crash with empty results
        try:
            checker.print_results([])
        except Exception as e:
            self.fail(f"print_results raised {e} with empty list")

    def test_print_results_with_mixed_results(self):
        """Test print_results with mix of available and missing dependencies."""
        checker = DependencyChecker()

        results = [
            DependencyCheckResult("available", True, "Good", critical=True),
            DependencyCheckResult("missing_critical", False, "Bad", critical=True),
            DependencyCheckResult("missing_optional", False, "Meh", critical=False),
        ]

        # Should return False due to critical missing
        system_ready = checker.print_results(results)
        self.assertFalse(system_ready)


class TestCheckDependenciesFunction(unittest.TestCase):
    """Tests for the check_dependencies convenience function."""

    @patch("pdf_generator.core.dependency_checker.DependencyChecker")
    def test_check_dependencies_returns_bool(self, mock_checker_class):
        """Test that check_dependencies returns a boolean."""
        mock_instance = MagicMock()
        mock_instance.check_all.return_value = (True, [])
        mock_instance.print_results.return_value = True
        mock_checker_class.return_value = mock_instance

        result = check_dependencies(verbose=False)

        self.assertIsInstance(result, bool)


if __name__ == "__main__":
    unittest.main()
