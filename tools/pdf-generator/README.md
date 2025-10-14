# PDF Report Generator

A Python tool for querying radar statistics from the API and generating reports in LaTeX and PDF formats.

**Location**: `tools/pdf-generator/`
**Installation**: PYTHONPATH-based (no package installation needed)
**Tests**: 451/451 passing (100% coverage)

## Quick Start

**All configuration is now done via JSON files!** No more CLI flags or environment variables.

```bash
# Option 1: Using Makefile (recommended)
cd tools/pdf-generator
make pdf-setup          # One-time: create virtual environment and install dependencies
make pdf-config         # Create example config file
# Edit config.example.json with your dates and settings
make pdf-report CONFIG=config.example.json

# Option 2: Using Python module directly
cd tools/pdf-generator
python -m pdf_generator.cli.create_config
# Edit config.example.json with your dates and settings
python -m pdf_generator.cli.main config.example.json
```

## Project Structure

```
tools/pdf-generator/              # Project root
├── pdf_generator/                # Python package
│   ├── cli/                      # Command-line entry points
│   │   ├── main.py              # Report generation CLI
│   │   ├── create_config.py     # Config template generator
│   │   └── demo.py              # Interactive demo
│   ├── core/                     # Core functionality (13 modules)
│   │   ├── config_manager.py   # Unified configuration system
│   │   ├── api_client.py       # RadarStatsClient and API helpers
│   │   ├── pdf_generator.py    # LaTeX/PyLaTeX report assembly
│   │   ├── chart_builder.py    # Time series and histogram charts
│   │   ├── table_builders.py   # LaTeX table construction
│   │   └── ...                 # 8 more core modules
│   └── tests/                    # Test suite (30 test files, 451 tests)
├── pyproject.toml                # Project metadata
├── requirements.txt              # Dependencies
├── .venv/                        # Virtual environment (created by make pdf-setup)
├── output/                       # Generated PDFs and assets
└── README.md                     # This file
```

**Note**: The two-level structure (`tools/pdf-generator/` and `pdf_generator/`) is standard Python practice:
- `tools/pdf-generator/` = Project root (configuration, docs, venv)
- `pdf_generator/` = Python package (importable code)

## Makefile Commands

The `Makefile` in the root provides convenient commands:

```bash
# Setup (one-time)
make pdf-setup          # Create venv, install dependencies

# Development
make pdf-test           # Run all 451 tests
make pdf-config         # Create example configuration file
make pdf-demo           # Run interactive demo

# Report Generation
make pdf-report CONFIG=config.example.json    # Generate PDF report

# Utilities
make pdf-clean          # Remove generated outputs
make pdf-help           # Show all available commands
```

## Module structure

### Core Components (in `pdf_generator/core/`)
- `config_manager.py` — **Unified configuration system** with JSON file support
- `api_client.py` — RadarStatsClient and helpers for fetching data
- `pdf_generator.py` — LaTeX/PyLaTeX based report assembly
- `chart_builder.py` — Time series and histogram chart generation
- `table_builders.py` — LaTeX table construction
- `stats_utils.py` — Statistical calculations
- `map_utils.py` — Map generation and utilities
- `date_parser.py` — Date/time parsing helpers
- `document_builder.py` — PDF document assembly

### CLI Entry Points (in `pdf_generator/cli/`)
- `main.py` — **Primary CLI entrypoint** (config-file only)
- `create_config.py` — **Config template generator**
- `demo.py` — Interactive demo of configuration system

### Testing (in `pdf_generator/tests/`)
- `test_*.py` — Comprehensive test suite with 100% pass rate (451 tests)
- All tests use standard pytest and mock patterns

## CLI: `pdf_generator.cli.main`

**Simplified!** The CLI now only accepts a JSON configuration file:

```bash
# Using Makefile (recommended)
make pdf-report CONFIG=<config.json>

# Using Python module directly
cd tools/pdf-generator
python -m pdf_generator.cli.main <config.json>

# With PYTHONPATH (if needed)
PYTHONPATH=. .venv/bin/python -m pdf_generator.cli.main <config.json>
```

### Creating a Configuration File

Use the built-in template generator:

```bash
# Using Makefile
make pdf-config

# Using Python module
python -m pdf_generator.cli.create_config

# Create a minimal example with only required fields
python -m pdf_generator.cli.create_config --minimal
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

**Note**: CLI flags are deprecated. Use JSON configuration files for all options.

## Examples

### Basic report generation with Makefile

```bash
cd tools/pdf-generator

# 1. Setup (one-time)
make pdf-setup

# 2. Create config
make pdf-config

# 3. Edit config.example.json with your settings:
#    - start_date: "2025-06-02"
#    - end_date: "2025-06-04"
#    - group: "1h"
#    - histogram: true
#    - hist_bucket_size: 5.0

# 4. Generate report
make pdf-report CONFIG=config.example.json

# Output will be in output/ directory
```

### Basic report generation with Python module

```bash
cd tools/pdf-generator

# 1. Create and edit config
python -m pdf_generator.cli.create_config

# 2. Generate report
python -m pdf_generator.cli.main config.example.json
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
# Using Makefile
make pdf-report CONFIG=my-config.json

# Using Python module
python -m pdf_generator.cli.main my-config.json
```

### Custom Settings

Use the example generator and customize:

```bash
# Generate full example with all options
make pdf-config
# OR
python -m pdf_generator.cli.create_config

# Copy and customize
cp config.example.json clarendon-survey.json
vim clarendon-survey.json

# Generate report
make pdf-report CONFIG=clarendon-survey.json
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
    "output_dir": "./output",
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

## Go Server Integration

**Note**: Go integration is being updated in a separate PR. The Python module has been restructured to `tools/pdf-generator/`.

For Go server integration, the workflow will be:

1. **User submits form** → Go validates and captures data
2. **Go writes config.json** → Stores configuration
3. **Go calls Python CLI** → Subprocess: executes Python module
4. **Python generates PDFs** → Returns with exit code
5. **Go checks output directory** → Finds generated files
6. **Svelte UI** → Provides download links

Example Go code (to be updated):

```go
// Generate config JSON from form data
configData := map[string]interface{}{
    "site": map[string]interface{}{
        "location": formData.Location,
        "speed_limit": formData.SpeedLimit,
    },
    "query": map[string]interface{}{
        "start_date": formData.StartDate,
        "end_date": formData.EndDate,
        "histogram": true,
        "hist_bucket_size": 5.0,
    },
    "output": map[string]interface{}{
        "file_prefix": reportID,
        "output_dir": outputPath,
    },
}

// Write to file
configPath := filepath.Join(tmpDir, "config.json")
configJSON, _ := json.Marshal(configData)
ioutil.WriteFile(configPath, configJSON, 0644)

// Call Python generator (path to be updated)
cmd := exec.Command(
    "path/to/venv/bin/python",
    "-m", "pdf_generator.cli.main",
    configPath,
)
cmd.Dir = "tools/pdf-generator"
cmd.Env = append(os.Environ(), "PYTHONPATH=.")
output, err := cmd.CombinedOutput()

if err != nil {
    return fmt.Errorf("report generation failed: %v", err)
}

// Check for generated files in output directory
files, _ := filepath.Glob(filepath.Join(outputPath, "*.pdf"))
```

See **`docs/GO_INTEGRATION.md`** for complete integration guide (to be updated).

## Documentation

- **`config.example.json`** — Fully documented configuration template
- **`config.minimal.json`** — Minimal required fields
- **`docs/CONFIG_SYSTEM.md`** — Complete system documentation
- **`docs/GO_INTEGRATION.md`** — Go server integration guide

## Python Integration

```python
from pdf_generator.core.generate_report_api import generate_report_from_dict

# From web form data
config_dict = {
    "site": {"location": "Main St", "speed_limit": 30},
    "query": {
        "start_date": "2025-06-01",
        "end_date": "2025-06-07",
        "histogram": True,
        "hist_bucket_size": 5.0
    }
}

result = generate_report_from_dict(config_dict)
if result["success"]:
    print(f"Generated files: {result['files']}")
```

**Note**: When importing from Python code, ensure `tools/pdf-generator` is in your `PYTHONPATH`:

```python
import sys
sys.path.insert(0, '/path/to/velocity.report/tools/pdf-generator')

from pdf_generator.core.config_manager import ReportConfig, load_config
from pdf_generator.core.api_client import RadarStatsClient
```

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
import sys
sys.path.insert(0, '/path/to/velocity.report/tools/pdf-generator')

from pdf_generator.core.api_client import RadarStatsClient
from pdf_generator.core.date_parser import parse_date_to_unix
from pdf_generator.core.pdf_generator import generate_pdf_report

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

**Or use configuration-based approach:**

```python
from pdf_generator.core.config_manager import ReportConfig, SiteConfig, QueryConfig, load_config

# From JSON file
config = load_config("config.json")

# Or create programmatically
config = ReportConfig(
    site=SiteConfig(location="Main St", speed_limit=30),
    query=QueryConfig(
        start_date="2025-06-01",
        end_date="2025-06-07",
        histogram=True,
        hist_bucket_size=5.0
    )
)

# Use with CLI or call generation functions directly
```
```

## Running tests

```bash
# Using Makefile (recommended)
cd /path/to/velocity.report
make pdf-test                    # Run all 451 tests
make pdf-test PYTEST_ARGS="-v"   # Run with verbose output

# Or directly with pytest
cd tools/pdf-generator
.venv/bin/pytest pdf_generator/tests/ -v

# Run with coverage report
.venv/bin/pytest pdf_generator/tests/ --cov=pdf_generator --cov-report=term-missing

# Run specific test files
.venv/bin/pytest pdf_generator/tests/test_config_manager.py -v
.venv/bin/pytest pdf_generator/tests/test_pdf_integration.py -v
.venv/bin/pytest pdf_generator/tests/test_table_builders.py -v
```

### Test Coverage

Current test status: **451/451 tests passing (100%)**

Module coverage:
- ✅ `stats_utils.py` — 100%
- ✅ `config_manager.py` — 100% (15 tests)
- ✅ `pdf_generator.py` — 99%
- ✅ `table_builders.py` — 95%+
- ✅ `map_utils.py` — 90%
- ✅ `chart_builder.py` — 82%

All tests use standard pytest patterns with comprehensive mocking and fixtures.

## Deployment Notes

### PYTHONPATH Approach

This project uses the PYTHONPATH approach rather than installing as a package:

**Benefits:**
- No package installation needed
- Simpler deployment (just copy files)
- Works well on Raspberry Pi and embedded systems
- All Makefile commands handle PYTHONPATH automatically

**Usage in scripts:**
```bash
# The Makefile sets this automatically
export PYTHONPATH=/path/to/velocity.report/tools/pdf-generator
.venv/bin/python -m pdf_generator.cli.main config.json

# Or use the Makefile
make pdf-report CONFIG=config.json
```

### Raspberry Pi Deployment

The PYTHONPATH approach is particularly useful for Raspberry Pi ARM64 deployment:

1. Copy `tools/pdf-generator/` to target system
2. Run `make pdf-setup` (creates venv, installs dependencies)
3. Use `make pdf-*` commands or set PYTHONPATH manually

No wheel building or package installation needed!

## Feedback / contributions

If you change CLI flags or add new environment tunables, please update this README and add unit tests for date parsing and the API client as appropriate.

### Key Documentation Files

- **`README.md`** (this file) — Module overview and CLI usage
- **`docs/CONFIG_SYSTEM.md`** — Complete configuration system documentation
- **`docs/GO_INTEGRATION.md`** — Go server integration guide (to be updated for new location)
- **`CONFIG_README.md`** — Configuration quick start guide
- **`IMPLEMENTATION_SUMMARY.md`** — Recent implementation details

### Recent Updates

**October 2025** — Major restructure to `tools/pdf-generator/`:
- Moved from `internal/report/query_data/` to `tools/pdf-generator/`
- Reorganized into standard Python package structure
- Added Makefile commands for common tasks
- PYTHONPATH-based approach (no package installation)
- All 451 tests passing (100%)
- Git history preserved from old location

**September 2025** — Added unified configuration management system:
- JSON configuration file support
- Web API entry point for Go server integration
- Environment variable override system
- Configuration priority system (CLI > File > Env > Defaults)
- Full backward compatibility with existing CLI workflows
- Comprehensive documentation and examples
