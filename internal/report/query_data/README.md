# Query Data Module

This module provides tools for querying radar statistics from the API and generating reports in LaTeX and PDF formats.

## Module structure

### Core Components
- `get_stats.py` — **CLI entrypoint** that ties everything together
- `api_client.py` — RadarStatsClient and helpers for fetching data
- `pdf_generator.py` — LaTeX/PyLaTeX based report assembly
- `date_parser.py` — date/time parsing helpers
- `stats_utils.py` — plotting and small data helpers
- `chart_builder.py` — time series and histogram chart generation
- `table_builders.py` — LaTeX table construction

### Configuration System (NEW)
- `config_manager.py` — **Unified configuration system** supporting CLI, JSON files, and environment variables
- `generate_report_api.py` — **Web API entry point** for Go server integration
- `report_config.py` — Site information, colors, fonts, and layout defaults

### Testing
- `test_*.py` — Comprehensive test suite with 95%+ coverage
- `demo_config_system.py` — Interactive demo of configuration system

## CLI: `get_stats.py`

The primary way to run the reporting flow is via the `get_stats.py` CLI. It queries the API, writes chart PDFs, and generates a final PDF report that includes tables, charts and a site map.

### Usage (basic):

```bash
python internal/report/query_data/get_stats.py [OPTIONS] <start1> <end1> [<start2> <end2> ...]
```

### NEW: Configuration File Support

You can now use JSON configuration files instead of (or in addition to) CLI arguments:

```bash
# Use a configuration file
python internal/report/query_data/get_stats.py --config my_report.json

# Override config file with CLI arguments
python internal/report/query_data/get_stats.py --config base.json --min-speed 10

# Save your effective configuration for reuse
python internal/report/query_data/get_stats.py --save-config my_config.json 2025-06-01 2025-06-07
```

See [Configuration System](#configuration-system) below for details.

### CLI Notes:
- Dates must be provided as positional arguments in start/end pairs. Each date can be a simple ISO date (YYYY-MM-DD) or a unix timestamp (seconds). You must provide an even number of date arguments.
- When `--histogram` is supplied, `--hist-bucket-size` is required.
- **All CLI arguments remain backward compatible** - existing scripts work unchanged.

## All CLI flags

### Configuration (NEW)
- `--config`, `--config-file` (path to JSON file)
  - Load configuration from a JSON file. When provided, most other arguments become optional and use values from the config file. CLI arguments can still override config file values.

- `--save-config` (path to JSON file)
  - Save the effective configuration (after merging CLI args, config file, and environment variables) to a JSON file for reuse.

### Query Parameters
- `--group` (default: `1h`)
  - Roll-up period to request from the API. Examples: `15m`, `1h`, `3h`, `1d`.

- `--units` (default: `mph`)
  - Display units to request and show in tables/plots. Typical values: `mph`, `kph`.

- `--source` (default: `radar_data_transits`) — choices: `radar_objects`, `radar_data_transits`
  - Data source to query. Use `radar_data_transits` when you want transit/session rollups (recommended for vehicle counts and percentiles).

- `--model-version` (default: `rebuild-full`)
  - When using `--source radar_data_transits`, the transit model version to request.

- `--timezone` (default: server default)
  - Timezone used for formatting StartTime in generated tables (e.g. `US/Pacific`, `UTC`). If empty, server defaults are used.

- `--min-speed` (float, default: none)
  - Minimum speed filter (in display units) used when querying the API. Deprecated alias: `--min_speed` is still accepted for compatibility.

### Histogram Options
- `--histogram` (flag)
  - Request histogram data from the server and include a histogram chart/table in the report.

- `--hist-bucket-size` (float, required if `--histogram`)
  - Bucket size (in display units) used to compute the histogram (e.g. `5` for 5 mph bins).

- `--hist-max` (float, optional)
  - Maximum speed to include in the histogram. Buckets above this value are grouped into a final `N+` bucket.

### Output Options
- `--file-prefix` (default: empty)
  - If provided, this prefix is used for generated files (charts and the final report). When empty, a sequenced prefix based on source/date range is created (e.g. `radar_data_transits_2025-06-02_to_2025-06-04-1`).

- `--debug` (flag)
  - Print debug information while parsing dates, plotting, and invoking the generator. Useful when developing or troubleshooting.

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

Query transit rollups explicitly:

```bash
python internal/report/query_data/get_stats.py \
  --source radar_data_transits --model-version rebuild-full \
  --group 1h --units mph \
  2025-06-02 2025-06-04
```

### Using configuration files

Create and use a configuration file:

```bash
# Generate an example config
python internal/report/query_data/config_manager.py
# Creates: report_config_example.json

# Edit it for your needs
vim report_config_example.json

# Use it
python internal/report/query_data/get_stats.py --config report_config_example.json
```

Override specific config values from CLI:

```bash
# Use config but change min-speed
python internal/report/query_data/get_stats.py \
  --config base_config.json \
  --min-speed 10

# Use config but change date range
python internal/report/query_data/get_stats.py \
  --config base_config.json \
  2025-07-01 2025-07-31
```

Save your configuration for reproducibility:

```bash
# Run report and save config used
python internal/report/query_data/get_stats.py \
  --group 1h --histogram --hist-bucket-size 5 \
  --save-config my_report_config.json \
  2025-06-02 2025-06-04

# Rerun with exact same settings
python internal/report/query_data/get_stats.py --config my_report_config.json
```

## Configuration System

A unified configuration management system supports both CLI and web-based workflows.

### Configuration Priority

Settings are merged in this order (highest to lowest priority):
1. **CLI arguments** (highest priority)
2. **Configuration file** (via `--config`)
3. **Environment variables**
4. **Default values** (lowest priority)

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
    "output_dir": "/var/reports",
    "debug": false
  },
  "radar": {
    "sensor_model": "OmniPreSense OPS243-A",
    "firmware_version": "v1.2.3"
  }
}
```

### Environment Variables

Override deployment-specific settings via environment variables:

**Site Configuration:**
- `REPORT_LOCATION` — Survey location
- `REPORT_SURVEYOR` — Surveyor name/organization
- `REPORT_CONTACT` — Contact email/phone
- `REPORT_SPEED_LIMIT` — Speed limit (integer)

**Query Configuration:**
- `REPORT_TIMEZONE` — Display timezone (e.g., "US/Pacific")
- `REPORT_MIN_SPEED` — Minimum speed filter (float)

**Output Configuration:**
- `REPORT_OUTPUT_DIR` — Default output directory
- `REPORT_DEBUG` — Enable debug output (0 or 1)

### Creating Configuration Templates

```bash
# Generate example configuration
python internal/report/query_data/config_manager.py
# Creates: report_config_example.json

# See interactive demo
python internal/report/query_data/demo_config_system.py
```

### Documentation

- **`docs/CONFIG_SYSTEM.md`** — Complete configuration system documentation
- **`docs/GO_INTEGRATION.md`** — Go server integration guide
- **`CONFIG_README.md`** — Quick start guide

## Web API Entry Point

For programmatic use (e.g., Go webserver integration), use the web API entry point:

```bash
# Generate report from JSON config file
python internal/report/query_data/generate_report_api.py /path/to/config.json --json
```

Returns JSON with file paths and status:

```json
{
  "success": true,
  "files": ["/path/to/report.pdf", "/path/to/stats.pdf"],
  "prefix": "report-prefix",
  "errors": []
}
```

### Python Integration

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
