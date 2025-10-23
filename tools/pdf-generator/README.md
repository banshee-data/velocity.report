# PDF Report Generator

A Python tool for generating professional PDF reports from radar statistics with charts, tables, and optional maps.

**Location**: `tools/pdf-generator/`
**Language**: Python 3.13+
**Installation**: PYTHONPATH-based (no package installation needed)

## Quick Start

All configuration is done via JSON files - no CLI flags needed!

```bash
# Using Makefile (recommended)
cd tools/pdf-generator
make pdf-setup          # One-time: create venv and install dependencies
make pdf-config         # Create example config.json
# Edit config.example.json with your dates and settings
make pdf-report CONFIG=config.example.json

# Using Python module directly
cd tools/pdf-generator
python -m pdf_generator.cli.create_config > my_config.json
# Edit my_config.json with your settings
python -m pdf_generator.cli.main my_config.json
```

**Minimal configuration requires only 7 fields:**
- `site.location`, `site.surveyor`, `site.contact`
- `query.start_date`, `query.end_date`, `query.timezone`
- `radar.cosine_error_angle`

## Configuration

### Required Fields

Every configuration file **must** include these 7 fields:

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `site.location` | string | Physical survey location | "Main Street, Springfield" |
| `site.surveyor` | string | Person/organization conducting survey | "City Traffic Department" |
| `site.contact` | string | Contact email or phone | "traffic@springfield.gov" |
| `query.start_date` | string | Start date (YYYY-MM-DD) | "2025-06-01" |
| `query.end_date` | string | End date (YYYY-MM-DD) | "2025-06-07" |
| `query.timezone` | string | Display timezone | "US/Pacific" |
| `radar.cosine_error_angle` | number | Radar mounting angle in degrees | 21.0 |

### Minimal Configuration Example

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

This minimal config will:
- Generate files with prefix `velocity.report`
- Include histogram with 5 mph buckets
- Filter speeds below 5 mph
- Group data by 1-hour intervals
- Display speeds in mph
- Calculate cosine error factor: 1.0711 (from 21° angle)

### Complete Configuration Example

```json
{
  "site": {
    "location": "Clarendon Avenue, San Francisco",
    "surveyor": "Banshee, INC.",
    "contact": "david@banshee-data.com",
    "speed_limit": 25,
    "site_description": "Survey conducted outside elementary school on downhill grade",
    "speed_limit_note": "Posted 35 mph, reduced to 25 mph when children present",
    "latitude": 37.7749,
    "longitude": -122.4194,
    "map_angle": 32.0
  },
  "radar": {
    "cosine_error_angle": 21.0,
    "sensor_model": "OmniPreSense OPS243-A",
    "firmware_version": "v1.2.3",
    "transmit_frequency": "24.125 GHz",
    "sample_rate": "20 kSPS",
    "velocity_resolution": "0.272 mph",
    "azimuth_fov": "20°",
    "elevation_fov": "24°"
  },
  "query": {
    "start_date": "2025-06-01",
    "end_date": "2025-06-07",
    "timezone": "US/Pacific",
    "group": "1h",
    "units": "mph",
    "source": "radar_data_transits",
    "model_version": "rebuild-full",
    "min_speed": 5.0,
    "histogram": true,
    "hist_bucket_size": 5.0,
    "hist_max": 50.0
  },
  "output": {
    "file_prefix": "clarendon-report",
    "output_dir": "./reports",
    "run_id": "auto-generated",
    "debug": false,
    "map": true
  }
}
```

### Configuration Sections Reference

#### Site Information
| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `location` | **Yes** | - | Street or location name |
| `surveyor` | **Yes** | - | Name of person/organization |
| `contact` | **Yes** | - | Contact email or phone |
| `speed_limit` | No | 25 | Posted speed limit in mph |
| `site_description` | No | "" | Detailed location description |
| `speed_limit_note` | No | "" | Speed limit details |
| `latitude` | No | null | GPS latitude for map |
| `longitude` | No | null | GPS longitude for map |
| `map_angle` | No | null | Map rotation angle in degrees |

#### Radar Configuration
| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `cosine_error_angle` | **Yes** | - | Mounting angle in degrees |
| `sensor_model` | No | "OmniPreSense OPS243-A" | Radar model name |
| `firmware_version` | No | "v1.2.3" | Firmware version |
| `transmit_frequency` | No | "24.125 GHz" | Operating frequency |
| `sample_rate` | No | "20 kSPS" | Data sample rate |
| `velocity_resolution` | No | "0.272 mph" | Minimum velocity resolution |
| `azimuth_fov` | No | "20°" | Horizontal field of view |
| `elevation_fov` | No | "24°" | Vertical field of view |

**Note**: The `cosine_error_factor` is auto-calculated as `1/cos(cosine_error_angle)`.

#### Query Parameters
| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `start_date` | **Yes** | - | Start date (YYYY-MM-DD) |
| `end_date` | **Yes** | - | End date (YYYY-MM-DD) |
| `timezone` | **Yes** | - | Display timezone (e.g., "US/Pacific", "UTC") |
| `group` | No | "1h" | Time aggregation (15m, 30m, 1h, 2h, 6h, 12h, 24h) |
| `units` | No | "mph" | Speed units ("mph", "kph", "mps") |
| `source` | No | "radar_data_transits" | Data source |
| `model_version` | No | "rebuild-full" | Transit model version |
| `min_speed` | No | 5.0 | Minimum speed filter |
| `histogram` | No | true | Generate histogram chart |
| `hist_bucket_size` | No | 5.0 | Histogram bucket size |
| `hist_max` | No | null | Maximum histogram bucket |

#### Output Configuration
| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `file_prefix` | No | auto-generated | User-provided prefix (auto-prefixed with `velocity.report_`) |
| `output_dir` | No | "." | Output directory |
| `run_id` | No | auto | Unique run identifier |
| `debug` | No | false | Enable debug output |
| `map` | No | false | Include OpenStreetMap overlay |

### Advanced Visual Customization

#### Colors
```json
{
  "colors": {
    "p50": "#fbd92f",    // Yellow - 50th percentile
    "p85": "#f7b32b",    // Orange - 85th percentile
    "p98": "#f25f5c",    // Red - 98th percentile
    "max": "#2d1e2f",    // Dark purple - maximum
    "count_bar": "#2d1e2f",
    "low_sample": "#f7b32b"
  }
}
```

#### Fonts
```json
{
  "fonts": {
    "chart_title": 14,
    "chart_label": 13,
    "chart_tick": 11,
    "chart_axis_label": 8,
    "chart_axis_tick": 7,
    "chart_legend": 7,
    "histogram_title": 14,
    "histogram_label": 13,
    "histogram_tick": 11
  }
}
```

#### Layout
```json
{
  "layout": {
    "chart_figsize_width": 24.0,
    "chart_figsize_height": 8.0,
    "histogram_figsize_width": 3.0,
    "histogram_figsize_height": 2.0,
    "low_sample_threshold": 50,
    "count_missing_threshold": 5
  }
}
```

#### PDF Document Settings
```json
{
  "pdf": {
    "geometry_top": "1.8cm",
    "geometry_bottom": "1.0cm",
    "geometry_left": "1.0cm",
    "geometry_right": "1.0cm",
    "columnsep": "14",
    "headheight": "12pt",
    "headsep": "10pt"
  }
}
```

#### Map Marker Configuration
```json
{
  "map": {
    "triangle_len": 0.42,
    "triangle_cx": 0.385,
    "triangle_cy": 0.71,
    "triangle_angle": 32.0,
    "triangle_color": "#f25f5c",
    "triangle_opacity": 0.9,
    "circle_radius": 20.0,
    "circle_fill": "#ffffff",
    "circle_stroke": "#f25f5c"
  }
}
```

### Cosine Error Correction

The cosine error factor corrects for radar mounting angle:

| Angle | Factor | Use Case |
|-------|--------|----------|
| 0° | 1.0000 | Perpendicular to traffic (no correction) |
| 15° | 1.0353 | Slight angle mount |
| 21° | 1.0711 | Typical roadside mount |
| 30° | 1.1547 | Significant angle |
| 45° | 1.4142 | Extreme angle mount |

Formula: `factor = 1 / cos(angle_in_radians)`

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
│   │   ├── stats_utils.py      # Statistical calculations
│   │   ├── map_utils.py        # Map generation
│   │   ├── date_parser.py      # Date/time parsing
│   │   └── ...                 # 5 more core modules
│   └── tests/                    # Test suite
├── pyproject.toml                # Project metadata
├── requirements.txt              # Dependencies
├── .venv/                        # Virtual environment (created by make)
├── output/                       # Generated PDFs and assets
├── docs/                         # Additional documentation
│   ├── CONFIG.md                # Complete configuration reference
│   ├── TESTING.md               # Test suite documentation
│   ├── FIXES_SUMMARY.md         # Bug fixes and improvements
│   └── REFACTOR_TIMELINE.md     # Project history
└── README.md                     # This file
```

**Note**: Two-level structure is standard Python practice:
- `tools/pdf-generator/` = Project root (config, docs, venv)
- `pdf_generator/` = Python package (importable code)

## Makefile Commands

```bash
# Setup (one-time)
make pdf-setup          # Create venv, install dependencies

# Development
make pdf-test           # Run all 545 tests
make pdf-config         # Create example configuration file
make pdf-demo           # Run interactive demo

# Report Generation
make pdf-report CONFIG=config.example.json    # Generate PDF report

# Utilities
make pdf-clean          # Remove generated outputs
make pdf-help           # Show all available commands
```

## Usage Examples

### Daily Report
```bash
# Create config
cat > daily-report.json << 'EOF'
{
  "site": {
    "location": "Main Street",
    "surveyor": "City Planning",
    "contact": "traffic@city.gov"
  },
  "radar": {
    "cosine_error_angle": 21.0
  },
  "query": {
    "start_date": "2025-06-15",
    "end_date": "2025-06-15",
    "timezone": "US/Pacific",
    "group": "1h"
  },
  "output": {
    "file_prefix": "daily_2025-06-15"
  }
}
EOF

# Generate report
make pdf-report CONFIG=daily-report.json
```

### Weekly Summary
```bash
cat > weekly-report.json << 'EOF'
{
  "site": {
    "location": "Highway 101",
    "surveyor": "State DOT",
    "contact": "surveys@dot.state.gov",
    "speed_limit": 65
  },
  "radar": {
    "cosine_error_angle": 15.0
  },
  "query": {
    "start_date": "2025-06-08",
    "end_date": "2025-06-14",
    "timezone": "US/Pacific",
    "group": "1d",
    "histogram": true,
    "hist_bucket_size": 5
  }
}
EOF

make pdf-report CONFIG=weekly-report.json
```

### Report with Map
```bash
cat > map-report.json << 'EOF'
{
  "site": {
    "location": "Clarendon Avenue",
    "surveyor": "Banshee Inc",
    "contact": "surveys@banshee.com",
    "speed_limit": 25,
    "latitude": 37.7749,
    "longitude": -122.4194,
    "map_angle": 32.0
  },
  "radar": {
    "cosine_error_angle": 21.0
  },
  "query": {
    "start_date": "2025-06-01",
    "end_date": "2025-06-07",
    "timezone": "US/Pacific"
  },
  "output": {
    "file_prefix": "clarendon_week1",
    "map": true
  }
}
EOF

make pdf-report CONFIG=map-report.json
```

## Testing

```bash
# Using Makefile (recommended)
make pdf-test
make pdf-test PYTEST_ARGS="-v"   # Run with verbose output

# Or directly with pytest
cd tools/pdf-generator
.venv/bin/pytest pdf_generator/tests/ -v

# Run with coverage report
.venv/bin/pytest pdf_generator/tests/ --cov=pdf_generator --cov-report=term-missing

# Run specific test files
.venv/bin/pytest pdf_generator/tests/test_config_manager.py -v
.venv/bin/pytest pdf_generator/tests/test_pdf_integration.py -v
```

## Deployment Notes

### PYTHONPATH Approach

This project uses PYTHONPATH rather than package installation:

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

The PYTHONPATH approach is ideal for Raspberry Pi ARM64:

1. Copy `tools/pdf-generator/` to target system
2. Run `make pdf-setup` (creates venv, installs dependencies)
3. Use `make pdf-*` commands or set PYTHONPATH manually

No wheel building or package installation needed!
