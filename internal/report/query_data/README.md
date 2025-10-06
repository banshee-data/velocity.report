# Query Data Module

This module provides tools for querying radar statistics from the API and generating reports in LaTeX and PDF formats.

## Module structure (brief)

- `date_parser.py` — date/time parsing helpers
- `api_client.py` — RadarStatsClient and helpers for fetching data
- `pdf_generator.py` — LaTeX/PyLaTeX based report assembly
- `stats_utils.py` — plotting and small data helpers
- `get_stats.py` — CLI entrypoint that ties everything together

## CLI: `get_stats.py`

The primary way to run the reporting flow is via the `get_stats.py` CLI. It queries the API, writes chart PDFs, and generates a final PDF report that includes tables, charts and a site map.

### Usage (basic):

```bash
python internal/report/query_data/get_stats.py [OPTIONS] <start1> <end1> [<start2> <end2> ...]
```

Notes:
- Dates must be provided as positional arguments in start/end pairs. Each date can be a simple ISO date (YYYY-MM-DD) or a unix timestamp (seconds). You must provide an even number of date arguments.
- When `--histogram` is supplied, `--hist-bucket-size` is required.

## All CLI flags

- `--group` (default: `1h`)
  - Roll-up period to request from the API. Examples: `15m`, `1h`, `3h`, `1d`.

- `--units` (default: `mph`)
  - Display units to request and show in tables/plots. Typical values: `mph`, `kph`.

- `--source` (default: `radar_objects`) — choices: `radar_objects`, `radar_data_transits`
  - Data source to query. Use `radar_data_transits` when you want transit/session rollups (recommended for vehicle counts and percentiles).

- `--model-version` (default: `rebuild-full`)
  - When using `--source radar_data_transits`, the transit model version to request.

- `--timezone` (default: server default)
  - Timezone used for formatting StartTime in generated tables (e.g. `US/Pacific`, `UTC`). If empty, server defaults are used.

- `--min-speed` (float, default: none)
  - Minimum speed filter (in display units) used when querying the API. Deprecated alias: `--min_speed` is still accepted for compatibility.

- `--file-prefix` (default: empty)
  - If provided, this prefix is used for generated files (charts and the final report). When empty, a sequenced prefix based on source/date range is created (e.g. `radar_data_transits_2025-06-02_to_2025-06-04-1`).

- `--histogram` (flag)
  - Request histogram data from the server and include a histogram chart/table in the report.

- `--hist-bucket-size` (float, required if `--histogram`)
  - Bucket size (in display units) used to compute the histogram (e.g. `5` for 5 mph bins).

- `--hist-max` (float, optional)
  - Maximum speed to include in the histogram. Buckets above this value are grouped into a final `N+` bucket.

- `--debug` (flag)
  - Print debug information while parsing dates, plotting, and invoking the generator. Useful when developing or troubleshooting.

Positional arguments
- `dates` — one or more start/end pairs. Example: `2025-06-02 2025-06-04` or `1622505600 1622678400`.

## Examples

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

## Running tests

```bash
# Install test deps
pip install pytest responses

# Run tests for this module
pytest internal/report/query_data/test_*.py -v
```

Feedback / contributions

If you change CLI flags or add new environment tunables, please update this README and add unit tests for date parsing and the API client as appropriate.
