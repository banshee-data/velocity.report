# Priority Test Examples - Quick Implementation Guide

**Goal:** Add 15 high-priority tests to increase coverage from 91% → 94%

## 1. API Integration Tests (5 tests)

**File:** `test_generate_report_api.py` (NEW FILE)

```python
"""Tests for generate_report_api.py module."""
import unittest
import tempfile
import os
from unittest.mock import patch, MagicMock
from generate_report_api import generate_report_from_dict


class TestGenerateReportAPI(unittest.TestCase):
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
    def test_generate_report_from_dict_success(self, mock_get_stats):
        """Test successful report generation via API."""
        mock_get_stats.main.return_value = None

        result = generate_report_from_dict(self.valid_config)

        self.assertTrue(result["success"])
        self.assertIn("message", result)
        mock_get_stats.main.assert_called_once()

    def test_generate_report_from_dict_invalid_config(self):
        """Test API with invalid configuration dict."""
        invalid_config = {"invalid": "config"}

        result = generate_report_from_dict(invalid_config)

        self.assertFalse(result["success"])
        self.assertIn("error", result)

    def test_generate_report_from_dict_missing_required_fields(self):
        """Test API with missing required config fields."""
        incomplete_config = {
            "site": {"location": "Test"},
            # Missing radar and query sections
        }

        result = generate_report_from_dict(incomplete_config)

        self.assertFalse(result["success"])
        self.assertIn("error", result)

    @patch("generate_report_api.get_stats")
    def test_generate_report_from_dict_database_error(self, mock_get_stats):
        """Test API behavior when database query fails."""
        mock_get_stats.main.side_effect = Exception("Database connection failed")

        result = generate_report_from_dict(self.valid_config)

        self.assertFalse(result["success"])
        self.assertIn("error", result)
        self.assertIn("Database", result["error"])

    @patch("generate_report_api.get_stats")
    def test_generate_report_from_dict_validation_error(self, mock_get_stats):
        """Test API behavior with config validation failure."""
        invalid_dates = self.valid_config.copy()
        invalid_dates["query"]["start_date"] = "invalid-date"

        result = generate_report_from_dict(invalid_dates)

        self.assertFalse(result["success"])
        self.assertIn("error", result)
```

## 2. CLI Error Path Tests (5 tests)

**File:** `test_cli_errors.py` (NEW FILE)

```python
"""Tests for CLI error handling in get_stats.py."""
import unittest
import sys
import os
import tempfile
from io import StringIO
from unittest.mock import patch


class TestCLIErrorHandling(unittest.TestCase):
    """Test CLI error handling and user messages."""

    def test_cli_config_file_not_found(self):
        """Test CLI error when config file doesn't exist."""
        with patch("sys.argv", ["get_stats.py", "nonexistent.json"]):
            with patch("sys.stderr", new=StringIO()) as mock_stderr:
                with self.assertRaises(SystemExit) as cm:
                    # Import triggers CLI parsing
                    import get_stats

                self.assertEqual(cm.exception.code, 2)
                error_output = mock_stderr.getvalue()
                self.assertIn("not found", error_output.lower())

    def test_cli_invalid_json_format(self):
        """Test CLI error with malformed JSON."""
        with tempfile.NamedTemporaryFile(mode="w", suffix=".json", delete=False) as f:
            f.write("{invalid json}")
            temp_file = f.name

        try:
            with patch("sys.argv", ["get_stats.py", temp_file]):
                with self.assertRaises(SystemExit):
                    import get_stats
        finally:
            os.unlink(temp_file)

    def test_cli_validation_failure(self):
        """Test CLI error when config validation fails."""
        import json

        with tempfile.NamedTemporaryFile(mode="w", suffix=".json", delete=False) as f:
            json.dump({"site": {"location": "Test"}}, f)  # Missing required fields
            temp_file = f.name

        try:
            with patch("sys.argv", ["get_stats.py", temp_file]):
                with patch("sys.stderr", new=StringIO()) as mock_stderr:
                    with self.assertRaises(SystemExit):
                        import get_stats

                    error_output = mock_stderr.getvalue()
                    self.assertIn("validation failed", error_output.lower())
        finally:
            os.unlink(temp_file)

    def test_cli_histogram_without_bucket_size(self):
        """Test CLI error for histogram config missing bucket size."""
        import json

        config = {
            "site": {"location": "Test", "surveyor": "Test", "contact": "test@test.com"},
            "radar": {"cosine_error_angle": 20},
            "query": {
                "start_date": "2025-06-01",
                "end_date": "2025-06-07",
                "timezone": "UTC",
                "histogram": True,
                # Missing hist_bucket_size
            },
        }

        with tempfile.NamedTemporaryFile(mode="w", suffix=".json", delete=False) as f:
            json.dump(config, f)
            temp_file = f.name

        try:
            with patch("sys.argv", ["get_stats.py", temp_file]):
                with patch("sys.stderr", new=StringIO()) as mock_stderr:
                    with self.assertRaises(SystemExit):
                        import get_stats

                    error_output = mock_stderr.getvalue()
                    self.assertIn("hist_bucket_size", error_output)
        finally:
            os.unlink(temp_file)

    def test_cli_help_message(self):
        """Test that help message displays correctly."""
        with patch("sys.argv", ["get_stats.py", "--help"]):
            with patch("sys.stdout", new=StringIO()) as mock_stdout:
                with self.assertRaises(SystemExit) as cm:
                    import get_stats

                self.assertEqual(cm.exception.code, 0)
                help_output = mock_stdout.getvalue()
                self.assertIn("config_file", help_output)
                self.assertIn("JSON configuration", help_output)
```

## 3. Config Manager Error Tests (4 tests)

**File:** `test_config_manager.py` (ADD TO EXISTING FILE)

```python
# Add to existing test_config_manager.py

def test_load_config_with_invalid_json_syntax():
    """Test load_config with syntactically invalid JSON."""
    with tempfile.NamedTemporaryFile(mode="w", suffix=".json", delete=False) as f:
        f.write('{"invalid": json content}')
        temp_file = f.name

    try:
        with self.assertRaises(json.JSONDecodeError):
            load_config(config_file=temp_file)
    finally:
        os.unlink(temp_file)


def test_load_config_with_neither_file_nor_dict():
    """Test load_config when both file and dict are None."""
    with self.assertRaises(ValueError) as cm:
        load_config(config_file=None, config_dict=None)

    self.assertIn("file nor dict", str(cm.exception).lower())


def test_load_config_file_not_found_error_message():
    """Test load_config error message for non-existent file."""
    with self.assertRaises(ValueError) as cm:
        load_config(config_file="/nonexistent/path/config.json")

    self.assertIn("not found", str(cm.exception).lower())
    self.assertIn("config.json", str(cm.exception))


def test_create_example_config_write_permission_error():
    """Test example config creation with write permission error."""
    # Try to write to a read-only directory
    readonly_path = "/System/config_example.json"  # System directory is read-only

    with self.assertRaises((PermissionError, OSError)):
        create_example_config(readonly_path)
```

## 4. Import Fallback Test (1 test)

**File:** `test_get_stats.py` (ADD TO EXISTING FILE)

```python
# Add to existing test_get_stats.py

class TestImportFallbacks(unittest.TestCase):
    """Test import error handling for optional dependencies."""

    def test_chart_builder_import_with_sys_modules_manipulation(self):
        """Test graceful failure when chart_builder cannot be imported."""
        import sys

        # Temporarily remove chart_builder from sys.modules
        original_modules = sys.modules.copy()
        if "chart_builder" in sys.modules:
            del sys.modules["chart_builder"]

        try:
            # Mock the import to raise ImportError
            with patch.dict("sys.modules", {"chart_builder": None}):
                with self.assertRaises(ImportError) as cm:
                    from get_stats import _import_chart_builder
                    _import_chart_builder()

                self.assertIn("chart_builder", str(cm.exception).lower())
        finally:
            # Restore original sys.modules
            sys.modules = original_modules
```

## Implementation Checklist

### Step 1: Create New Test Files
- [ ] Create `test_generate_report_api.py`
- [ ] Create `test_cli_errors.py`

### Step 2: Add Tests to Existing Files
- [ ] Add 4 tests to `test_config_manager.py`
- [ ] Add 1 test to `test_get_stats.py`

### Step 3: Run Tests
```bash
cd /Users/david/code/velocity.report/internal/report/query_data
python -m pytest test_generate_report_api.py -v
python -m pytest test_cli_errors.py -v
python -m pytest test_config_manager.py -v
python -m pytest test_get_stats.py::TestImportFallbacks -v
```

### Step 4: Verify Coverage Improvement
```bash
python -m pytest --cov=. --cov-report=term-missing -q
```

Expected result: Coverage should increase from 91% to approximately 94%.

## Notes

### Potential Issues

1. **CLI Tests May Be Tricky:** Testing CLI argument parsing can be challenging because it runs at import time. Consider using subprocess or mocking sys.argv carefully.

2. **Import Tests:** Testing import failures requires careful manipulation of sys.modules. Use try/finally to ensure cleanup.

3. **File I/O Tests:** Always use tempfile and ensure cleanup in finally blocks to avoid leaving test artifacts.

### Alternative Approaches

If CLI tests prove difficult:
- Use subprocess to test CLI in isolation
- Mock argparse.ArgumentParser to avoid actual parsing
- Test the underlying functions instead of the CLI entry point

### Time Estimate

- Test file creation: 30 minutes
- Writing tests: 1.5 hours
- Debugging and refinement: 1 hour
- **Total: ~3 hours for Phase 1**

