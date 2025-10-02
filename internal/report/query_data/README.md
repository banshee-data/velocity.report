# Query Data Module

This module provides tools for querying radar statistics from the API and generating reports in LaTeX and PDF formats.

## Module Structure

The module has been refactored into separate, focused components:

### Core Modules

- **`date_parser.py`** - Date and time parsing utilities
  - `parse_date_to_unix()` - Parse various date formats to unix timestamps
  - `parse_server_time()` - Parse server time responses
  - `is_date_only()` - Check if a string is a plain date

- **`api_client.py`** - HTTP client for the radar stats API
  - `RadarStatsClient` - Main client class for querying statistics
  - `SUPPORTED_GROUPS` - Dict of supported aggregation periods

- **`latex_generator.py`** - LaTeX table generation
  - `stats_to_latex()` - Convert stats to LaTeX tables
  - `generate_table_file()` - Generate complete table file with parameters
  - `format_time()`, `format_number()` - Formatting utilities

- **`get_stats.py`** - Main CLI tool (orchestrates the above modules)

### Tests

- **`test_date_parser.py`** - Comprehensive tests for date parsing
  - 19 tests covering all date parsing scenarios
  - Tests for unix timestamps, ISO dates, timezones, edge cases

- **`test_api_client.py`** - Tests for the API client
  - 10 tests covering API interactions
  - Uses `responses` library to mock HTTP requests
  - Tests for various query parameters and response formats

## Usage

### As a CLI Tool

```bash
python get_stats.py --group 1h --units mph --timezone US/Pacific \\
    --min-speed 5 --histogram --hist-bucket-size 5 \\
    2025-06-02 2025-06-04
```

### As a Library

```python
from query_data import RadarStatsClient, parse_date_to_unix, generate_table_file

# Create client
client = RadarStatsClient(base_url="http://localhost:8080")

# Parse dates
start_ts = parse_date_to_unix("2025-06-02", tz_name="US/Pacific")
end_ts = parse_date_to_unix("2025-06-04", end_of_day=True, tz_name="US/Pacific")

# Query stats
metrics, histogram, resp = client.get_stats(
    start_ts=start_ts,
    end_ts=end_ts,
    group="1h",
    units="mph",
    min_speed=5.0
)

# Generate table file
generate_table_file(
    file_path="output_table.tex",
    start_iso="2025-06-02T00:00:00-07:00",
    end_iso="2025-06-04T23:59:59-07:00",
    group="1h",
    units="mph",
    timezone_display="US/Pacific",
    min_speed_str="5.0 mph",
    overall_metrics=overall_metrics,
    daily_metrics=daily_metrics,
    granular_metrics=metrics,
    tz_name="US/Pacific"
)
```

## Running Tests

```bash
# Install test dependencies
pip install pytest responses

# Run all tests
pytest internal/report/query_data/test_*.py -v

# Run specific test module
pytest internal/report/query_data/test_date_parser.py -v

# Run with coverage
pytest internal/report/query_data/test_*.py --cov=internal/report/query_data --cov-report=html
```

## Development

### Adding New Features

1. Add functionality to the appropriate module (date_parser, api_client, latex_generator)
2. Write tests in the corresponding test file
3. Update `__init__.py` to export new public APIs
4. Run tests to ensure nothing breaks

### Code Organization Principles

- **date_parser.py**: Pure functions for date/time manipulation, no external dependencies except standard library
- **api_client.py**: HTTP communication only, returns raw data structures
- **latex_generator.py**: LaTeX formatting logic, takes data structures and produces strings
- **get_stats.py**: Orchestration logic, ties everything together for the CLI

This separation makes the code:
- **Testable**: Each module can be tested in isolation
- **Reusable**: Modules can be used independently
- **Maintainable**: Changes are localized to specific modules
- **Clear**: Each file has a single, clear purpose

## Dependencies

- `requests` - HTTP client for API calls
- `numpy` - Numerical operations
- `zoneinfo` - Timezone handling (Python 3.9+)
- `pytest` - Testing framework (dev)
- `responses` - HTTP mocking for tests (dev)
