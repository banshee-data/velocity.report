# get_stats.py Refactoring Notes

## Summary of Changes

The `get_stats.py` script has been refactored to:

1. **Handle new API payload shape**: Server now returns `{ metrics: [...], histogram: {...} }` instead of a raw array
2. **Replace multiple flags with unified output approach**: Removed `--pdf` and `--tex-table`, replaced with `--file-prefix`
3. **Add histogram support**: New `--histogram` flag requests histogram data and generates histogram charts
4. **Generate multiple predictable outputs**: All outputs use a common prefix for easy organization

## API Response Changes

### Old Response (before)
```json
[
  { "StartTime": "...", "Count": 42, "P50Speed": 25.5, ... },
  ...
]
```

### New Response (now)
```json
{
  "metrics": [
    { "StartTime": "...", "Count": 42, "P50Speed": 25.5, ... },
    ...
  ],
  "histogram": {
    "10.000": 5,
    "15.000": 12,
    "20.000": 25,
    ...
  }
}
```

## Function Signature Changes

### `get_stats()` - Updated
**Old:**
```python
def get_stats(start_ts, end_ts, group, units, source, timezone, min_speed):
    # Returns: (data, resp) where data is a list
```

**New:**
```python
def get_stats(start_ts, end_ts, group, units, source, timezone, min_speed,
              compute_histogram, hist_bucket_size, hist_max):
    # Returns: (metrics, histogram, resp)
    # - metrics: list of metric rows
    # - histogram: dict mapping bucket labels to counts (empty dict if not requested)
    # - resp: requests.Response object
```

### `plot_histogram()` - New Function
```python
def plot_histogram(histogram, title, units, debug=False):
    """Create a matplotlib Figure for a histogram.

    Args:
        histogram: Dict mapping bucket labels (strings) to counts
        title: Chart title
        units: Speed units for axis label
        debug: Enable debug output

    Returns:
        matplotlib.figure.Figure
    """
```

## Command Line Changes

### Old Flags (removed)
- `--pdf OUTPUT.pdf` - Path to single output PDF
- `--tex-table OUTPUT.tex` - Path to LaTeX table output

### New Flags (added)
- `--file-prefix PREFIX` - Common prefix for all outputs (optional, auto-generated if not provided)
- `--histogram` - Request histogram data from server
- `--hist-bucket-size SIZE` - Histogram bucket size in display units (required with `--histogram`)
- `--hist-max MAX` - Maximum speed for histogram (optional)

## Generated Output Files

For a given prefix `PREFIX`, the script generates:

### Always Generated
1. `{PREFIX}_table.tex` - Combined LaTeX table with generation params, main table, daily summary, and overall summary
2. `{PREFIX}_stats.pdf` - Main stats chart (line+bar combo)
3. `{PREFIX}_daily.pdf` - Daily (24h) rollup chart (if main group < 24h)
4. `{PREFIX}_overall.pdf` - Overall (all) summary chart

### Generated When `--histogram` Used
5. `{PREFIX}_histogram.pdf` - Main histogram chart
6. `{PREFIX}_daily_histogram.pdf` - Daily histogram (if main group < 24h)
7. `{PREFIX}_overall_histogram.pdf` - Overall histogram

## Example Usage

### Basic Usage (No Histogram)
```bash
# Auto-generated prefix: "radar_data_transits_2025-06-02_to_2025-06-04"
python get_stats.py \
  --group 1h \
  --source radar_data_transits \
  --units mph \
  --timezone US/Pacific \
  --min-speed 5 \
  2025-06-02 2025-06-04T23:00:00Z

# Outputs:
# - radar_data_transits_2025-06-02_to_2025-06-04_table.tex
# - radar_data_transits_2025-06-02_to_2025-06-04_stats.pdf
# - radar_data_transits_2025-06-02_to_2025-06-04_daily.pdf
# - radar_data_transits_2025-06-02_to_2025-06-04_overall.pdf
```

### With Custom Prefix
```bash
python get_stats.py \
  --file-prefix my_report \
  --group 1h \
  --units mph \
  2025-06-02 2025-06-04

# Outputs:
# - my_report_table.tex
# - my_report_stats.pdf
# - my_report_daily.pdf
# - my_report_overall.pdf
```

### With Histogram
```bash
python get_stats.py \
  --file-prefix speed_analysis \
  --group 1h \
  --units mph \
  --histogram \
  --hist-bucket-size 5 \
  --hist-max 100 \
  2025-06-02 2025-06-04

# Outputs (7 files total):
# - speed_analysis_table.tex
# - speed_analysis_stats.pdf
# - speed_analysis_histogram.pdf
# - speed_analysis_daily.pdf
# - speed_analysis_daily_histogram.pdf
# - speed_analysis_overall.pdf
# - speed_analysis_overall_histogram.pdf
```

### Replacing Old Commands

**Old command:**
```bash
python get_stats.py \
  --group 1h \
  --pdf out_p_reset10-3.pdf \
  --tex-table table.tex \
  --units mph \
  2025-06-02 2025-06-04
```

**New equivalent:**
```bash
python get_stats.py \
  --file-prefix out_p_reset10-3 \
  --group 1h \
  --units mph \
  2025-06-02 2025-06-04

# Note: Multiple PDFs are now generated instead of single combined PDF
# If you need a single combined PDF, use a PDF merge tool or keep the old script version
```

## Migration Guide

1. **Update scripts/automation**: Replace `--pdf` and `--tex-table` with `--file-prefix`
2. **Expect multiple outputs**: Each run now generates 4-7 files instead of 1-2
3. **Add histogram support**: Use `--histogram --hist-bucket-size N` to enable histogram generation
4. **File naming**: Output files follow predictable pattern `{prefix}_{type}.{ext}`

## Benefits

- ✅ Cleaner output organization with predictable naming
- ✅ Histogram support for speed distribution analysis
- ✅ Automatic generation of all summary views (main, daily, overall)
- ✅ Each output type in separate file for easier inclusion in reports
- ✅ Backward compatible API (metrics array extraction handles both old and new server responses)
