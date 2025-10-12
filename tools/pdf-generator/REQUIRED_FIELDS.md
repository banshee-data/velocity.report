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
| `radar.cosine_error_angle` | number | Radar mounting angle in degrees | 21.0 |

## Derived Fields

These fields are automatically calculated from required fields:

| Field | Type | Calculation | Example |
|-------|------|-------------|---------|
| `radar.cosine_error_factor` | number | 1 / cos(cosine_error_angle) | 1.0711 (from 21°) |

## Optional Fields with Defaults

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
- `site.speed_limit` (default: `25`) - Posted speed limit
- `site.site_description` (default: `""`) - Site-specific narrative (optional, omit section if empty)
- `site.speed_limit_note` (default: `""`) - Speed limit details (optional, omit section if empty)
- `site.latitude` (default: `null`) - Latitude for map generation
- `site.longitude` (default: `null`) - Longitude for map generation
- `site.map_angle` (default: `null`) - Map orientation angle

### Radar Configuration
- `radar.sensor_model` (default: `"OmniPreSense OPS243-A"`)
- `radar.firmware_version` (default: `"v1.2.3"`)
- `radar.transmit_frequency` (default: `"24.125 GHz"`)
- `radar.sample_rate` (default: `"20 kSPS"`)
- `radar.velocity_resolution` (default: `"0.272 mph"`)
- `radar.azimuth_fov` (default: `"20°"`)
- `radar.elevation_fov` (default: `"24°"`)

## Minimal Configuration Example

The absolute minimum configuration requires only 7 fields:

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
  "radar": {
    "cosine_error_angle": 21.0
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
- **Exclude site info section** (site_description and speed_limit_note are empty)
- Calculate cosine error factor: 1.0711 (from 21° angle)

## Common Configuration Examples

### With Site Information Section

```json
{
  "site": {
    "location": "Main Street, Springfield",
    "surveyor": "City Traffic Department",
    "contact": "traffic@springfield.gov",
    "site_description": "Survey conducted from southbound parking lane during school hours.",
    "speed_limit_note": "Posted limit is 25 mph when children are present."
  },
  "query": {
    "start_date": "2025-06-01",
    "end_date": "2025-06-07",
    "timezone": "US/Pacific"
  },
  "radar": {
    "cosine_error_angle": 21.0
  }
}
```

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
  "radar": {
    "cosine_error_angle": 21.0
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
    "contact": "traffic@springfield.gov",
    "latitude": 39.7817,
    "longitude": -89.6501,
    "map_angle": 45.0
  },
  "query": {
    "start_date": "2025-06-01",
    "end_date": "2025-06-07",
    "timezone": "US/Pacific"
  },
  "radar": {
    "cosine_error_angle": 21.0
  },
  "output": {
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
  "radar": {
    "cosine_error_angle": 21.0
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
  "radar": {
    "cosine_error_angle": 21.0
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
1. All 7 required fields are present and non-empty
2. `radar.cosine_error_angle` is not 0.0 (required)
3. `query.source` is either `"radar_objects"` or `"radar_data_transits"`
4. `query.units` is either `"mph"` or `"kph"`
5. If `query.histogram` is `true`, appropriate defaults are applied

Missing or invalid fields will produce clear error messages indicating what needs to be corrected.

## Cosine Error Correction

The cosine error factor corrects for radar mounting angle. For example:
- **0° angle**: factor = 1.0 (no correction needed, radar perpendicular to traffic)
- **15° angle**: factor = 1.0353
- **21° angle**: factor = 1.0711 (default example)
- **30° angle**: factor = 1.1547
- **45° angle**: factor = 1.4142

The factor is automatically calculated from the angle using the formula: `1 / cos(angle_in_radians)`
