# Configuration System Simplification - Summary

## What Changed

We've simplified the velocity report system to use **JSON configuration files only**. All CLI flags and environment variables have been removed in favor of a single, standardized JSON config format.

### Before (Complex)
- CLI with 15+ flags
- Environment variables for overrides
- Configuration file support (optional)
- Priority system: CLI > file > env > defaults
- Difficult to standardize between CLI and web

### After (Simple)
- **CLI takes only a config file**: `python get_stats.py config.json`
- **Web API takes same config format**: `generate_report_api.py config.json`
- **No environment variables needed**
- **Single source of truth**: JSON configuration

## Benefits

1. **Standardization**: CLI and web use identical config format
2. **Simplicity**: One config file instead of scattered flags/env vars
3. **Reproducibility**: Save config file and get exact same report
4. **Documentation**: Config file is self-documenting with field names
5. **Go Integration**: Easy for Go server to generate and validate JSON

## Migration Guide

### Old Way (CLI flags)
```bash
python get_stats.py \
  --group 1h \
  --units mph \
  --histogram \
  --hist-bucket-size 5 \
  --min-speed 5 \
  --file-prefix my-report \
  --no-map \
  2025-06-02 2025-06-04
```

### New Way (Config file)
```json
{
  "query": {
    "start_date": "2025-06-02",
    "end_date": "2025-06-04",
    "group": "1h",
    "units": "mph",
    "min_speed": 5.0,
    "histogram": true,
    "hist_bucket_size": 5.0
  },
  "output": {
    "file_prefix": "my-report",
    "no_map": true
  }
}
```

```bash
python get_stats.py my-config.json
```

## Files Modified

### Core Changes
- **get_stats.py**: Simplified CLI to only accept config file
- **config_manager.py**: Removed `from_cli_args()`, `from_env()`, `merge_with_env()`
- **generate_report_api.py**: Already config-based, just updated docstrings

### New Files
- **create_config_example.py**: Generates template config files
- **config.example.json**: Full example with all options documented
- **config.minimal.json**: Minimal example with only required fields

### Documentation
- **README.md**: Completely rewritten for config-file-only approach
- **CONFIG_SIMPLIFICATION.md**: This document

## Configuration Format

### Required Fields
```json
{
  "query": {
    "start_date": "YYYY-MM-DD",
    "end_date": "YYYY-MM-DD"
  }
}
```

### Common Fields
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
    "hist_bucket_size": 5.0,
    "hist_max": 60.0,
    "source": "radar_data_transits",
    "model_version": "rebuild-full"
  },
  "output": {
    "file_prefix": "my-report",
    "output_dir": "./reports",
    "debug": false,
    "no_map": false
  },
  "site": {
    "location": "Clarendon Avenue, San Francisco",
    "surveyor": "Banshee, INC.",
    "contact": "david@banshee-data.com",
    "speed_limit": 25
  }
}
```

See `config.example.json` for complete documentation of all fields.

## Usage

### CLI
```bash
# Generate example config
python create_config_example.py

# Edit your config
vim config.example.json

# Generate report
python get_stats.py config.example.json
```

### Web API
```bash
# From command line
python generate_report_api.py config.json --json

# Returns:
# {
#   "success": true,
#   "prefix": "report-name",
#   "files": [],
#   "errors": []
# }
```

### Python Integration
```python
from generate_report_api import generate_report_from_dict

config = {
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

result = generate_report_from_dict(config)
if result["success"]:
    print(f"Report: {result['prefix']}")
```

### Go Server Integration
```go
// 1. Create config from form data
config := ReportConfig{
    Query: QueryConfig{
        StartDate: "2025-06-01",
        EndDate:   "2025-06-07",
        Histogram: true,
        HistBucketSize: 5.0,
    },
    Output: OutputConfig{
        FilePrefix: "user-report",
    },
}

// 2. Write to JSON
configJSON, _ := json.Marshal(config)
configFile := "/tmp/report-123.json"
ioutil.WriteFile(configFile, configJSON, 0644)

// 3. Call Python
cmd := exec.Command("python", "generate_report_api.py", configFile, "--json")
output, _ := cmd.Output()

// 4. Parse result
var result ReportResult
json.Unmarshal(output, &result)
```

## Testing

All existing functionality works with config files:

```bash
# Create test config
cat > test.json << EOF
{
  "query": {
    "start_date": "2025-06-02",
    "end_date": "2025-06-04",
    "histogram": true,
    "hist_bucket_size": 5.0
  },
  "output": {
    "file_prefix": "test",
    "no_map": true
  }
}
EOF

# Test CLI
python get_stats.py test.json

# Test API
python generate_report_api.py test.json

# Verify outputs
ls test-1_*.pdf
```

## Backward Compatibility

**Breaking Change**: Old CLI flags no longer work.

If you have existing scripts using CLI flags, you'll need to:
1. Create config JSON files
2. Update scripts to use: `python get_stats.py config.json`

The config file format is straightforward - most scripts can be automatically converted.

## Next Steps

1. ✅ CLI simplified to config-file only
2. ✅ Environment variables removed
3. ✅ Config manager simplified
4. ✅ Example configs created
5. ✅ Documentation updated
6. ⏳ Update Go server to generate config JSON
7. ⏳ Update Svelte frontend forms

## Questions?

- See `config.example.json` for all available options
- See `README.md` for usage examples
- See `GO_INTEGRATION.md` for Go server integration
- Run `python create_config_example.py --help`
