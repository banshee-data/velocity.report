# Query Data Module

This module provides tools for querying radar statistics from the API and generating reports in LaTeX and PDF formats.

## Quick Start

**All configuration is now done via JSON files!** No more CLI flags or environment variables.

```bash
# 1. Create example config file
python internal/report/query_data/create_config_example.py

# 2. Edit config.example.json with your dates and settings

# 3. Generate report
python internal/report/query_data/get_stats.py config.example.json
```

## Module structure

### Core Components
- `get_stats.py` — **CLI entrypoint** (now config-file only)
- `generate_report_api.py` — **Web API entry point** for Go server integration
- `config_manager.py` — **Unified configuration system** with JSON file support
- `create_config_example.py` — **Config template generator**
- `api_client.py` — RadarStatsClient and helpers for fetching data
- `pdf_generator.py` — LaTeX/PyLaTeX based report assembly
- `chart_builder.py` — time series and histogram chart generation
- `table_builders.py` — LaTeX table construction
- `report_config.py` — Site information, colors, fonts, and layout defaults

### Testing
- `test_*.py` — Comprehensive test suite with 95%+ coverage
- `demo_config_system.py` — Interactive demo of configuration system

## CLI: `get_stats.py`

**Simplified!** The CLI now only accepts a JSON configuration file:

```bash
python internal/report/query_data/get_stats.py <config.json>
```

### Creating a Configuration File

Use the built-in template generator:

```bash
# Create a full example with all options documented
python internal/report/query_data/create_config_example.py

# Create a minimal example with only required fields
python internal/report/query_data/create_config_example.py --minimal
```

### Example Configuration

```json
{
  "query": {
    "start_date": "2025-06-01",
    "end_date": "2025-06-07",
    "group": "1h",
    "units": "mph",
    "timezone": "US/Pacific",
    "min_speed": 5.0,
    "histogram": true,
    "hist_bucket_size": 5.0
  },
  "output": {
    "file_prefix": "my-report",
    "no_map": false,
    "debug": false
  }
}
```

See `config.example.json` for all available options and documentation.

## Configuration Options

All configuration is in JSON format with four sections:

### `query` - Data Query Parameters (REQUIRED)
- `start_date` (string, required): Start date in YYYY-MM-DD format
- `end_date` (string, required): End date in YYYY-MM-DD format
- `group` (string, default: "1h"): Time bucket size (15m, 30m, 1h, 2h, 4h, 8h, 12h, 24h)
- `units` (string, default: "mph"): Display units (mph or kph)
- `source` (string, default: "radar_data_transits"): Data source
- `timezone` (string, default: "US/Pacific"): Display timezone
- `min_speed` (float, optional): Minimum speed filter
- `histogram` (boolean, default: false): Generate histogram
- `hist_bucket_size` (float, required if histogram=true): Histogram bucket size
- `hist_max` (float, optional): Maximum histogram value

### `output` - Output Options
- `file_prefix` (string, default: auto-generated): Prefix for output files
- `output_dir` (string, default: "."): Output directory
- `debug` (boolean, default: false): Enable debug output
- `no_map` (boolean, default: false): Skip map generation

### `site` - Location Information (Optional)
- `location` (string): Survey location name
- `surveyor` (string): Surveyor name/organization
- `contact` (string): Contact email
- `speed_limit` (int): Posted speed limit
- `site_description` (string): Site description for report
- `latitude`, `longitude`, `map_angle` (float): GPS coordinates for map

### `radar` - Sensor Specifications (Optional)
- Technical sensor specifications included in report
- See `config.example.json` for all fields

### Positional Arguments
- `dates` — one or more start/end pairs. Example: `2025-06-02 2025-06-04` or `1622505600 1622678400`.

## Examples

### Basic report generation

Generate a one-hour rollup report with histogram:

```bash
python internal/report/query_data/get_stats.py \
  --group 1h --units mph --timezone US/Pacific \
  --min-speed 5 --histogram --hist-bucket-size 5 \
  2025-06-02 2025-06-04
```


## Example Usage

### Basic Report

Create a simple configuration file:

```json
{
  "query": {
    "start_date": "2025-06-02",
    "end_date": "2025-06-04",
    "histogram": true,
    "hist_bucket_size": 5.0
  },
  "output": {
    "file_prefix": "my-report"
  }
}
```

Generate the report:

```bash
python internal/report/query_data/get_stats.py my-config.json
```

### Custom Settings

Use the example generator and customize:

```bash
# Generate full example with all options
python internal/report/query_data/create_config_example.py

# Copy and customize
cp config.example.json clarendon-survey.json
vim clarendon-survey.json

# Generate report
python internal/report/query_data/get_stats.py clarendon-survey.json
```

### Report Without Map

For surveys without GPS data:

```json
{
  "query": {
    "start_date": "2025-06-02",
    "end_date": "2025-06-04"
  },
  "output": {
    "file_prefix": "no-map-report",
    "no_map": true
  }
}
```

## Configuration System

All configuration is JSON-based - no CLI flags or environment variables needed!

### Creating Configs

Use the template generator:

```bash
# Full example with documentation
python internal/report/query_data/create_config_example.py

# Minimal example (only required fields)
python internal/report/query_data/create_config_example.py --minimal
```

### Configuration File Format

JSON format with four main sections:

```json
{
  "site": {
    "location": "Main Street, Springfield",
    "surveyor": "City Traffic Department",
    "contact": "traffic@springfield.gov",
    "speed_limit": 30
  },
  "query": {
    "start_date": "2025-06-01",
    "end_date": "2025-06-07",
    "group": "1h",
    "units": "mph",
    "timezone": "US/Pacific",
    "min_speed": 5.0,
    "histogram": true,
    "hist_bucket_size": 5.0
  },
  "output": {
    "file_prefix": "main-st-june",
    "output_dir": "./reports",
    "debug": false,
    "no_map": false
  },
  "radar": {
    "sensor_model": "OmniPreSense OPS243-A",
    "firmware_version": "v1.2.3"
  }
}
```

See `config.example.json` for a complete, documented example.

## Web API Entry Point

For programmatic use (e.g., Go webserver integration), use the web API:

### Command Line

```bash
# Generate report from JSON config file
python internal/report/query_data/generate_report_api.py config.json

# With JSON output for parsing
python internal/report/query_data/generate_report_api.py config.json --json
```

Returns JSON with status:

```json
{
  "success": true,
  "prefix": "report-prefix",
  "files": [],
  "errors": []
}
```

### Python Integration

Import directly for use in web frameworks:

```python
from internal.report.query_data.generate_report_api import (
    generate_report_from_file,
    generate_report_from_dict
)

# From config file
result = generate_report_from_file("config.json")

# From web form data (dict)
config_dict = {
    "query": {
        "start_date": "2025-06-01",
        "end_date": "2025-06-07",
        "histogram": True,
        "hist_bucket_size": 5.0
    },
    "output": {
        "file_prefix": "web-report"
    }
}
result = generate_report_from_dict(config_dict)

if result["success"]:
    print(f"Report generated: {result['prefix']}")
else:
    print(f"Errors: {result['errors']}")
```

### Go Server Integration

The Go server workflow:

1. **User submits form** → Go validates and captures data
2. **Go writes config.json** → Stores in SQLite + file
3. **Go calls Python API** → Subprocess or HTTP call
4. **Python generates PDFs** → Returns file paths
5. **Go moves files** → To report-specific directory
6. **Svelte UI** → Provides download links

See `docs/GO_INTEGRATION.md` for complete Go code examples.

## Documentation

- **`config.example.json`** — Fully documented configuration template
- **`config.minimal.json`** — Minimal required fields
- **`docs/CONFIG_SYSTEM.md`** — Complete system documentation
- **`docs/GO_INTEGRATION.md`** — Go server integration guide

## Python Integration

```python
from internal.report.query_data.generate_report_api import generate_report_from_dict

# From web form data
config_dict = {
    "site": {"location": "Main St", "speed_limit": 30},
    "query": {
        "start_date": "2025-06-01",
        "end_date": "2025-06-07",
        "histogram": true,
        "hist_bucket_size": 5.0
    }
}

result = generate_report_from_dict(config_dict)
if result["success"]:
    print(f"Generated files: {result['files']}")
```

### Go Server Integration

The system is designed for integration with a Go webserver workflow:

1. User submits form → Go server captures data
2. Go saves config to SQLite + JSON file
3. Go calls Python API via subprocess
4. Python generates PDFs and returns file paths
5. Go moves files to report-specific folder
6. Svelte frontend displays download links

See **`docs/GO_INTEGRATION.md`** for complete Go code examples, database schema, and deployment instructions.

## Environment variables affecting PDF/layout

Advanced: environment variables affecting PDF/layout

The PDF generator (`pdf_generator.py`) reads a few environment variables that affect layout and map marker placement. These are optional, but useful for tuning report appearance:

- `REPORT_TABLE_COLUMNS` (default `2`) — number of side-by-side table columns when splitting large granular tables.
- `REPORT_TABLE_ROWS_PER_COLUMN` (default `48`) — how many rows to put in each column before paginating.
- `REPORT_COLUMNSEP_PT` (default `14`) — column gap in points (used to set `\columnsep`).
- `MAP_TRIANGLE_*` family — control the map overlay triangle (e.g. `MAP_TRIANGLE_CX`, `MAP_TRIANGLE_CY`, `MAP_TRIANGLE_LEN`, `MAP_TRIANGLE_COLOR`, `MAP_TRIANGLE_OPACITY`, `MAP_TRIANGLE_CIRCLE_RADIUS`). See `pdf_generator.py` for exact names and defaults.

## Library integration

If you want to use the pieces programmatically, import the client and generator helpers:

```python
from internal.report.query_data.api_client import RadarStatsClient
from internal.report.query_data.date_parser import parse_date_to_unix
from internal.report.query_data.pdf_generator import generate_pdf_report

# Query example
client = RadarStatsClient()
# ... call client.get_stats(...) to get overall_metrics, daily_metrics, granular_metrics, histogram ...

# Generate PDF (high-level example):
generate_pdf_report(
    output_path="out-report.pdf",
    start_iso="2025-06-02T00:00:00-07:00",
    end_iso="2025-06-04T23:59:59-07:00",
    group="1h",
    units="mph",
    timezone_display="US/Pacific",
    min_speed_str="5.0 mph",
    location="Clarendon Avenue, San Francisco",
    overall_metrics=overall_metrics,
    daily_metrics=daily_metrics,
    granular_metrics=granular_metrics,
    histogram=histogram,
    tz_name="US/Pacific",
    charts_prefix="out"
)
```

**Or use the new configuration-based API:**

```python
from internal.report.query_data.config_manager import ReportConfig, SiteConfig, QueryConfig
from internal.report.query_data.generate_report_api import generate_report_from_config

# Create configuration
config = ReportConfig(
    site=SiteConfig(location="Main St", speed_limit=30),
    query=QueryConfig(
        start_date="2025-06-01",
        end_date="2025-06-07",
        histogram=True,
        hist_bucket_size=5.0
    )
)

# Generate report
result = generate_report_from_config(config)
if result["success"]:
    for file in result["files"]:
        print(f"Generated: {file}")
```

## Running tests

```bash
# Install test dependencies (if not already installed)
pip install pytest pytest-cov responses

# Run all tests for this module
pytest internal/report/query_data/test_*.py -v

# Run with coverage report
pytest internal/report/query_data/test_*.py --cov=internal/report/query_data --cov-report=term-missing

# Run specific test files
pytest internal/report/query_data/test_config_manager.py -v  # Configuration tests
pytest internal/report/query_data/test_pdf_integration.py -v  # PDF generation tests
pytest internal/report/query_data/test_table_builders.py -v   # Table building tests
```

### Test Coverage

Current coverage status:
- ✅ `stats_utils.py` — 100%
- ✅ `pdf_generator.py` — 99%
- ✅ `map_utils.py` — 90%
- ✅ `config_manager.py` — 100% (15 tests)
- ✅ `table_builders.py` — 95%+
- ✅ `chart_builder.py` — 82%

## Feedback / contributions

If you change CLI flags or add new environment tunables, please update this README and add unit tests for date parsing and the API client as appropriate.

### Key Documentation Files

- **`README.md`** (this file) — Module overview and CLI usage
- **`docs/CONFIG_SYSTEM.md`** — Complete configuration system documentation
- **`docs/GO_INTEGRATION.md`** — Go server integration guide with code examples
- **`CONFIG_README.md`** — Configuration quick start guide
- **`IMPLEMENTATION_SUMMARY.md`** — Recent implementation details

### Recent Updates

**October 2025** — Added unified configuration management system:
- JSON configuration file support
- Web API entry point for Go server integration
- Environment variable override system
- Configuration priority system (CLI > File > Env > Defaults)
- Full backward compatibility with existing CLI workflows
- Comprehensive documentation and examples
