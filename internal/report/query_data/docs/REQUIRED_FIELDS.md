# Configuration System - Required Fields & Defaults

## Summary of Changes

We've refined the configuration system to have clear required vs optional fields, and changed the map logic to be opt-in rather than opt-out.

### Key Changes

1. **Required Fields** - These MUST be in every config:
   - `site.location` - Survey location name
   - `site.surveyor` - Surveyor name/organization
   - `site.contact` - Contact email/phone
   - `query.start_date` - Start date (YYYY-MM-DD)
   - `query.end_date` - End date (YYYY-MM-DD)
   - `query.timezone` - Timezone (e.g., US/Pacific, UTC)
   - `output.file_prefix` - Output file prefix

2. **Map Logic Flipped** - Changed from `no_map` to `map`:
   - Old: `no_map: false` (default) = show map, `no_map: true` = hide map
   - New: `map: false` (default) = NO map, `map: true` = show map
   - Maps are now opt-in, not opt-out

3. **Histogram Defaults Changed**:
   - Old: `histogram: true`, `hist_bucket_size: 5.0` (defaults)
   - New: `histogram: false`, `hist_bucket_size: null` (no histogram by default)
   - Histograms are now opt-in

4. **Min Speed Changed**:
   - Old: `min_speed: 5.0` (default)
   - New: `min_speed: null` (no filter by default)

## Minimal Configuration

The absolute minimum required config:

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
    "file_prefix": "my-report"
  }
}
```

This will generate:
- ✅ Time series chart (stats)
- ✅ Daily summary
- ❌ No histogram (histogram=false by default)
- ❌ No map (map=false by default)
- ❌ No min speed filter (min_speed=null by default)

## Common Configuration

Most reports will want histograms:

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
    "min_speed": 5.0,
    "histogram": true,
    "hist_bucket_size": 5.0
  },
  "output": {
    "file_prefix": "main-st-survey"
  }
}
```

This generates:
- ✅ Time series chart
- ✅ Daily summary
- ✅ Histogram chart
- ✅ Min speed filter (5 mph)
- ❌ No map (still false by default)

## Configuration with Map

To include a map, set `map: true`:

```json
{
  "site": {
    "location": "Main Street, Springfield",
    "surveyor": "City Traffic Department",
    "contact": "traffic@springfield.gov",
    "latitude": 39.7817,
    "longitude": -89.6501
  },
  "query": {
    "start_date": "2025-06-01",
    "end_date": "2025-06-07",
    "timezone": "US/Pacific",
    "histogram": true,
    "hist_bucket_size": 5.0
  },
  "output": {
    "file_prefix": "main-st-survey",
    "map": true
  }
}
```

This generates:
- ✅ Time series chart
- ✅ Daily summary
- ✅ Histogram chart
- ✅ **Map with location marker**

## Field Reference

### Required Fields

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `site.location` | string | Survey location | "Main Street, Springfield" |
| `site.surveyor` | string | Surveyor name | "City Traffic Department" |
| `site.contact` | string | Contact info | "traffic@springfield.gov" |
| `query.start_date` | string | Start date | "2025-06-01" |
| `query.end_date` | string | End date | "2025-06-07" |
| `query.timezone` | string | Timezone | "US/Pacific" |
| `output.file_prefix` | string | File prefix | "my-report" |

### Optional Fields with Defaults

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `query.group` | string | "1h" | Time grouping (15m, 30m, 1h, 2h, 6h, 12h, 24h) |
| `query.units` | string | "mph" | Display units (mph, kph) |
| `query.source` | string | "radar_data_transits" | Data source |
| `query.model_version` | string | "rebuild-full" | Transit model version |
| `query.min_speed` | float | null | Minimum speed filter (optional) |
| `query.histogram` | boolean | false | Generate histogram |
| `query.hist_bucket_size` | float | null | Histogram bucket size |
| `query.hist_max` | float | null | Max histogram value |
| `output.output_dir` | string | "." | Output directory |
| `output.debug` | boolean | false | Debug output |
| `output.map` | boolean | false | Include map in report |
| `site.speed_limit` | int | 25 | Posted speed limit |
| `site.site_description` | string | "" | Site description |
| `site.latitude` | float | null | GPS latitude |
| `site.longitude` | float | null | GPS longitude |

## Validation Rules

The system will reject configs that are missing required fields:

```bash
$ python get_stats.py bad-config.json
Configuration validation failed:
  - site.location is required
  - query.timezone is required
  - output.file_prefix is required
```

## Migration from Old Defaults

If you were relying on old defaults:

### Old Behavior → New Behavior

| Feature | Old Default | New Default | Migration |
|---------|-------------|-------------|-----------|
| Histogram | `true` (auto) | `false` (opt-in) | Add `"histogram": true` |
| Histogram bucket | `5.0` | `null` | Add `"hist_bucket_size": 5.0` |
| Min speed | `5.0` | `null` | Add `"min_speed": 5.0` if needed |
| Map | `false` (no_map) | `false` (map) | Add `"map": true` if needed |
| Timezone | `"US/Pacific"` | **REQUIRED** | Must specify explicitly |

### Example Migration

**Old config (implied defaults):**
```json
{
  "query": {
    "start_date": "2025-06-01",
    "end_date": "2025-06-07"
  }
}
```

**New config (explicit required fields):**
```json
{
  "site": {
    "location": "My Location",
    "surveyor": "My Company",
    "contact": "me@example.com"
  },
  "query": {
    "start_date": "2025-06-01",
    "end_date": "2025-06-07",
    "timezone": "US/Pacific",
    "histogram": true,
    "hist_bucket_size": 5.0,
    "min_speed": 5.0
  },
  "output": {
    "file_prefix": "my-report"
  }
}
```

## Testing

Generate example configs:

```bash
# Minimal (required fields only)
python create_config_example.py --minimal
cat config.minimal.json

# Full example (all fields documented)
python create_config_example.py
cat config.example.json

# Test minimal config
python get_stats.py config.minimal.json
```

## Documentation

See these files for more details:
- `config.minimal.json` - Minimal working example
- `config.example.json` - Full example with all options
- `CONFIG_SIMPLIFICATION.md` - Architecture changes
- `README.md` - Usage guide
