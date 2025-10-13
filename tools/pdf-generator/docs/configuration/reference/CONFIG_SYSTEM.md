# Configuration Management System

## Overview

This directory now includes a unified configuration management system that supports:

1. **CLI Entry Point** - Traditional command-line interface (backward compatible)
2. **Web/API Entry Point** - For Go webserver integration via JSON config files
3. **Environment Variables** - For deployment-specific overrides
4. **Per-Report Configuration** - Site info, parameters, and preferences stored per report

## Architecture

```
┌─────────────────┐
│  Go Webserver   │
│  (Form Data)    │
└────────┬────────┘
         │
         │ 1. Save to SQLite
         │ 2. Write config.json
         │
         ▼
┌─────────────────┐
│  config.json    │
│  (Per-Report)   │
└────────┬────────┘
         │
         │ 3. Call Python API
         │
         ▼
┌─────────────────┐       ┌──────────────┐
│ generate_report │──────▶│  get_stats   │
│     _api.py     │       │     .py      │
└────────┬────────┘       └──────────────┘
         │
         │ 4. Generate PDFs
         │
         ▼
┌─────────────────┐
│  Output Files   │
│  - report.pdf   │
│  - stats.pdf    │
│  - daily.pdf    │
│  - histogram.pdf│
└────────┬────────┘
         │
         │ 5. Transfer to report folder
         │
         ▼
┌─────────────────┐
│ Svelte Frontend │
│ (Download Links)│
└─────────────────┘
```

## Key Files

### Configuration Management

- **`config_manager.py`** - Core configuration system
  - `ReportConfig` - Main configuration dataclass
  - `SiteConfig` - Site-specific information (location, surveyor, etc.)
  - `RadarConfig` - Radar sensor technical parameters
  - `QueryConfig` - API query and processing parameters
  - `OutputConfig` - Output file configuration
  - JSON serialization/deserialization
  - Environment variable merging
  - Validation

### Entry Points

- **`get_stats.py`** - CLI entry point (updated)
  - Traditional argparse interface
  - New `--config` flag for JSON config files
  - New `--save-config` flag to export effective config
  - Backward compatible with existing scripts

- **`generate_report_api.py`** - Web API entry point (NEW)
  - `generate_report_from_file()` - Load from JSON file
  - `generate_report_from_dict()` - Direct dictionary input
  - `generate_report_from_config()` - From ReportConfig object
  - Returns structured result with file paths and errors
  - CLI interface for Go subprocess calls

## Usage Examples

### 1. CLI with Config File (NEW)

```bash
# Generate report from config file
python get_stats.py --config my_report_config.json

# Override specific parameters
python get_stats.py --config my_report_config.json --min-speed 10

# Save effective configuration to file
python get_stats.py --config base_config.json --save-config final_config.json 2025-06-02 2025-06-04
```

### 2. Traditional CLI (Backward Compatible)

```bash
# Original CLI interface still works
python get_stats.py \
  --group 1h \
  --units mph \
  --histogram \
  --hist-bucket-size 5 \
  --min-speed 5 \
  2025-06-02 2025-06-04
```

### 3. Web API Entry Point (For Go Server)

```bash
# Call from Go server as subprocess
python generate_report_api.py /path/to/config.json --json

# Output is JSON:
# {
#   "success": true,
#   "files": ["/path/report.pdf", "/path/stats.pdf", ...],
#   "prefix": "radar_data_transits_20250602_to_20250604",
#   "errors": [],
#   "config_used": {...}
# }
```

### 4. Python Import (Direct Integration)

```python
from generate_report_api import generate_report_from_dict

# From web form data
config_dict = {
    "site": {
        "location": "Main Street, Springfield",
        "surveyor": "City Traffic Dept",
        "contact": "traffic@springfield.gov",
        "speed_limit": 30
    },
    "query": {
        "start_date": "2025-06-01",
        "end_date": "2025-06-07",
        "group": "1h",
        "units": "mph",
        "histogram": true,
        "hist_bucket_size": 5.0
    },
    "output": {
        "file_prefix": "main-st-june",
        "output_dir": "/var/reports/run-123"
    }
}

result = generate_report_from_dict(config_dict)

if result["success"]:
    print(f"Generated {len(result['files'])} files")
    for f in result["files"]:
        print(f"  - {f}")
else:
    print(f"Errors: {result['errors']}")
```

## Configuration File Format

### Complete Example

```json
{
  "site": {
    "location": "Clarendon Avenue, San Francisco",
    "surveyor": "Banshee, INC.",
    "contact": "david@banshee-data.com",
    "speed_limit": 25,
    "site_description": "Survey conducted from southbound parking lane...",
    "speed_limit_note": "Posted speed limit is 35 mph, reduced to 25 mph...",
    "latitude": 37.7749,
    "longitude": -122.4194,
    "map_angle": 32.0
  },
  "radar": {
    "sensor_model": "OmniPreSense OPS243-A",
    "firmware_version": "v1.2.3",
    "transmit_frequency": "24.125 GHz",
    "sample_rate": "20 kSPS",
    "velocity_resolution": "0.272 mph",
    "azimuth_fov": "20°",
    "elevation_fov": "24°",
    "cosine_error_angle": "21°",
    "cosine_error_factor": "1.0711"
  },
  "query": {
    "start_date": "2025-06-02",
    "end_date": "2025-06-04",
    "group": "1h",
    "units": "mph",
    "source": "radar_data_transits",
    "model_version": "rebuild-full",
    "timezone": "US/Pacific",
    "min_speed": 5.0,
    "histogram": true,
    "hist_bucket_size": 5.0,
    "hist_max": 50.0
  },
  "output": {
    "file_prefix": "test-report",
    "output_dir": "/var/reports",
    "run_id": "run-20250610-123456",
    "debug": false
  },
  "created_at": "2025-06-10T12:34:56.789Z",
  "updated_at": null,
  "version": "1.0"
}
```

### Minimal Example

```json
{
  "site": {
    "location": "Main St, Springfield"
  },
  "query": {
    "start_date": "2025-06-01",
    "end_date": "2025-06-07"
  }
}
```

All other fields use sensible defaults.

## Go Server Integration

### Workflow

1. **User submits form** → Go server receives data
2. **Go saves to SQLite** → Persist configuration and metadata
3. **Go writes config.json** → Create JSON file in run-specific directory
4. **Go calls Python API** → Execute as subprocess:
   ```go
   cmd := exec.Command("python", "generate_report_api.py", configPath, "--json")
   output, err := cmd.CombinedOutput()
   result := parseJSON(output)
   ```
5. **Python generates PDFs** → Files written to output directory
6. **Go processes result** → Move files, update database, trigger cleanup
7. **Svelte frontend** → Display download links

### Example Go Integration

```go
package main

import (
    "encoding/json"
    "os/exec"
    "path/filepath"
)

type ReportResult struct {
    Success    bool     `json:"success"`
    Files      []string `json:"files"`
    Prefix     string   `json:"prefix"`
    Errors     []string `json:"errors"`
    ConfigUsed map[string]interface{} `json:"config_used"`
}

func GenerateReport(configPath string) (*ReportResult, error) {
    cmd := exec.Command(
        "python",
        "/path/to/generate_report_api.py",
        configPath,
        "--json",
    )

    output, err := cmd.CombinedOutput()
    if err != nil {
        return nil, fmt.Errorf("command failed: %w, output: %s", err, output)
    }

    var result ReportResult
    if err := json.Unmarshal(output, &result); err != nil {
        return nil, fmt.Errorf("failed to parse result: %w", err)
    }

    return &result, nil
}

// Example usage
func handleReportGeneration(runID string, formData map[string]interface{}) error {
    // 1. Save to database
    if err := db.SaveReportConfig(runID, formData); err != nil {
        return err
    }

    // 2. Create config file
    configPath := filepath.Join("/var/reports", runID, "config.json")
    configData := buildConfigFromForm(formData)
    if err := writeJSON(configPath, configData); err != nil {
        return err
    }

    // 3. Generate report
    result, err := GenerateReport(configPath)
    if err != nil {
        return err
    }

    if !result.Success {
        return fmt.Errorf("generation failed: %v", result.Errors)
    }

    // 4. Update database with file paths
    for _, filePath := range result.Files {
        db.SaveReportFile(runID, filePath)
    }

    return nil
}
```

## Environment Variables

Environment variables provide deployment-specific overrides:

### Site Configuration
- `REPORT_LOCATION` - Survey location
- `REPORT_SURVEYOR` - Surveyor name/organization
- `REPORT_CONTACT` - Contact email/phone
- `REPORT_SPEED_LIMIT` - Speed limit (integer)
- `REPORT_LATITUDE` - GPS latitude (float)
- `REPORT_LONGITUDE` - GPS longitude (float)
- `REPORT_MAP_ANGLE` - Map rotation angle (float)

### Query Configuration
- `REPORT_TIMEZONE` - Display timezone (e.g., "US/Pacific")
- `REPORT_MIN_SPEED` - Minimum speed filter (float)

### Output Configuration
- `REPORT_OUTPUT_DIR` - Default output directory
- `REPORT_DEBUG` - Enable debug output (0 or 1)

### Radar Configuration
- `RADAR_MODEL` - Sensor model name
- `RADAR_FIRMWARE` - Firmware version

## Migration Guide

### For Existing Scripts

No changes required! Existing scripts using CLI arguments continue to work:

```bash
# This still works exactly as before
python get_stats.py --histogram --hist-bucket-size 5 2025-06-01 2025-06-07
```

### For New Workflows

1. **Create config template**:
   ```bash
   python config_manager.py  # Generates report_config_example.json
   ```

2. **Customize for your site**:
   ```json
   {
     "site": {
       "location": "Your Location",
       "surveyor": "Your Organization"
     }
   }
   ```

3. **Use config file**:
   ```bash
   python get_stats.py --config my_site_config.json 2025-06-01 2025-06-07
   ```

## File Versioning

The system supports generating multiple versions of the same report:

1. **Initial generation**: User creates report with config
2. **Config saved**: JSON file stored with `run_id`
3. **User edits**: Modify config (different min_speed, histogram settings, etc.)
4. **Regenerate**: Call API with updated config
5. **New version**: Files use same prefix but overwrite or use new run_id

### Example Workflow

```python
# Version 1: Initial report
config_v1 = {
    "query": {"min_speed": 5.0, "histogram": true},
    "output": {"run_id": "run-001"}
}
result = generate_report_from_dict(config_v1)

# Version 2: User wants to see all speeds
config_v2 = config_v1.copy()
config_v2["query"]["min_speed"] = None
config_v2["output"]["run_id"] = "run-002"
result = generate_report_from_dict(config_v2)

# Both versions stored separately by run_id
```

## Testing

### Generate Example Config

```bash
python config_manager.py
# Creates: report_config_example.json
```

### Test CLI with Config

```bash
python get_stats.py --config report_config_example.json
```

### Test Web API

```bash
python generate_report_api.py report_config_example.json --json
```

### Validate Config

```python
from config_manager import ReportConfig

config = ReportConfig.from_json("my_config.json")
is_valid, errors = config.validate()
if not is_valid:
    print(f"Validation errors: {errors}")
```

## Best Practices

1. **Use config files for production** - Easier to audit and reproduce
2. **Store configs in database** - Link to generated reports
3. **Include run_id** - Track report versions and lineage
4. **Set sensible defaults** - Minimize required fields
5. **Validate early** - Check config before expensive operations
6. **Use environment variables for secrets** - API keys, credentials
7. **Use config files for site data** - Location-specific information

## Future Enhancements

- [ ] Web UI config builder
- [ ] Config templates (residential, school zone, highway, etc.)
- [ ] Config diffing for version comparison
- [ ] Bulk report generation from config directory
- [ ] Real-time validation API endpoint
- [ ] Config inheritance (base config + overrides)
- [ ] Historical config archiving
- [ ] A/B testing support (multiple configs for same data)
