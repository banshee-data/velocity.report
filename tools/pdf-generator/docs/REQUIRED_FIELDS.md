# Required Configuration Fields

This document describes the required and optional fields for the velocity report configuration system.

## Required Fields

The following fields **must** be specified in every configuration file:

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `site.location` | string | Physical location of the survey | "Main Street, Springfield" |
| `site.surveyor` | string | Name of person/organization conducting survey | "City Traffic Department" |
| `site.contact` | string | Contact information for survey | "traffic@springfield.gov" |
| `query.start_date` | string | Start date for data query (YYYY-MM-DD) | "2025-06-01" |
| `query.end_date` | string | End date for data query (YYYY-MM-DD) | "2025-06-07" |
| `query.timezone` | string | Timezone for date/time display | "US/Pacific" |

## Optional Fields with Defaults

These fields are optional and will use sensible defaults if not specified:

### Output Configuration
- `output.file_prefix` (default: `"velocity.report"`) - Prefix for generated files
- `output.output_dir` (default: `"."`) - Output directory for files
- `output.debug` (default: `false`) - Enable debug output
- `output.map` (default: `false`) - Include map in report

### Query Configuration
- `query.group` (default: `"1h"`) - Time grouping (15m, 30m, 1h, 2h, 6h, 12h, 24h, all)
- `query.units` (default: `"mph"`) - Display units (mph, kph)
- `query.source` (default: `"radar_data_transits"`) - Data source
- `query.model_version` (default: `"rebuild-full"`) - Transit model version
- `query.min_speed` (default: `5.0`) - Minimum speed filter in display units
- `query.histogram` (default: `true`) - Generate histogram
- `query.hist_bucket_size` (default: `5.0`) - Histogram bucket size
- `query.hist_max` (default: `null`) - Maximum histogram bucket value

### Site Configuration
- `site.title` (default: `"Traffic Survey Report"`) - Report title
- `site.site_description` (default: `""`) - Additional site details
- `site.speed_limit_note` (default: `""`) - Speed limit information

## Minimal Configuration Example

The absolute minimum configuration requires only 6 fields:

```json
{
  "site": {
    "location": "Main Street, Springfield",
    "surveyor": "City Traffic Department",
    "contact": "traffic@springfield.gov"
  },
  "query": {
    "start_date": "2025-06-01",
    "end_date": "2025-06-07",
    "timezone": "US/Pacific"
  }
}
```

This will:
- Generate files with prefix `velocity.report-1`, `velocity.report-2`, etc.
- Include histogram with 5 mph buckets
- Filter out speeds below 5 mph
- Group data by 1-hour intervals
- Display speeds in mph
- Exclude map (map generation is opt-in)

## Common Configuration Examples

### With Custom File Prefix

```json
{
  "site": {
    "location": "Main Street, Springfield",
    "surveyor": "City Traffic Department",
    "contact": "traffic@springfield.gov"
  },
  "query": {
    "start_date": "2025-06-01",
    "end_date": "2025-06-07",
    "timezone": "US/Pacific"
  },
  "output": {
    "file_prefix": "main-street-survey"
  }
}
```

### With Map Included

```json
{
  "site": {
    "location": "Main Street, Springfield",
    "surveyor": "City Traffic Department",
    "contact": "traffic@springfield.gov"
  },
  "query": {
    "start_date": "2025-06-01",
    "end_date": "2025-06-07",
    "timezone": "US/Pacific"
  },
  "output": {
    "file_prefix": "main-street-survey",
    "map": true
  }
}
```

### Without Histogram

```json
{
  "site": {
    "location": "Main Street, Springfield",
    "surveyor": "City Traffic Department",
    "contact": "traffic@springfield.gov"
  },
  "query": {
    "start_date": "2025-06-01",
    "end_date": "2025-06-07",
    "timezone": "US/Pacific",
    "histogram": false
  },
  "output": {
    "file_prefix": "main-street-survey"
  }
}
```

## Migration from Old System

If you previously used CLI flags or environment variables, here's how to migrate:

### Old CLI Flags
```bash
python get_stats.py \
  --file-prefix my-report \
  --group 1h \
  --histogram \
  --hist-bucket-size 5 \
  2025-06-01 2025-06-07
```

### New Config File
```json
{
  "site": {
    "location": "Your Location",
    "surveyor": "Your Name/Org",
    "contact": "your@email.com"
  },
  "query": {
    "start_date": "2025-06-01",
    "end_date": "2025-06-07",
    "timezone": "US/Pacific",
    "group": "1h",
    "histogram": true,
    "hist_bucket_size": 5.0
  },
  "output": {
    "file_prefix": "my-report"
  }
}
```

Then run:
```bash
python get_stats.py config.json
```

## Field Validation

The system validates:
1. All 6 required fields are present and non-empty
2. `query.source` is either `"radar_objects"` or `"radar_data_transits"`
3. `query.units` is either `"mph"` or `"kph"`
4. If `query.histogram` is `true`, appropriate defaults are applied

Missing or invalid fields will produce clear error messages indicating what needs to be corrected.
