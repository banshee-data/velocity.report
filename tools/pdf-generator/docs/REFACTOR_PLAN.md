# Refactor Plan: Replace argparse.Namespace with ReportConfig

## Executive Summary

**Scope:** Complete removal of `argparse.Namespace` technical debt across the codebase
**Impact:** 11 functions, 19 test files, 2 demo files
**Code Reduction:** ~100+ lines of duplicate conversion code
**Estimated Effort:** 8-12 hours for complete refactor + testing
**Risk Level:** Medium (extensive test coverage provides safety net)

## Problem Statement

Currently, `get_stats.py` and `generate_report_api.py` both convert `ReportConfig` objects into `argparse.Namespace` objects for backward compatibility. This creates technical debt with duplicate code and makes the codebase harder to maintain.

**Current TODOs:**
1. `get_stats.py` line 733: "TODO: Refactor to use config directly throughout the codebase"
2. `generate_report_api.py` line 91: "TODO: Refactor get_stats.py to use config directly"
3. `generate_report_api.py` line 134: "TODO: Have get_stats.main() return file list"

**Additional Issues Found in Codebase Scan:**
4. `test_get_stats.py`: 19 test functions create `argparse.Namespace` objects (needs updating)
5. `test_config_manager.py`: `test_from_cli_args()` tests deprecated `from_cli_args()` method (doesn't exist)
6. `test_config_manager.py`: `test_load_config_from_file()` uses old `load_config()` signature
7. `test_config_manager.py`: `test_merge_with_env()` tests deprecated `merge_with_env()` method
8. `demo_config_system.py`: References deprecated `from_cli_args()` and `from_env()` methods
9. Fallback to `SITE_INFO` dict still present in `assemble_pdf_report()` (should use config defaults)

## Current Architecture

### Data Flow:
```
JSON Config File
    ↓
ReportConfig (dataclass)
    ↓
argparse.Namespace (28+ field assignments!) ← TECHNICAL DEBT
    ↓
get_stats.main(date_ranges, args)
    ↓
Various functions that access args.* attributes
    ↓
PDF Generation
```

### Files Requiring Changes:

**Production Code (3 files):**
1. `get_stats.py` - 11 functions using args, 35 lines of config→args conversion
2. `generate_report_api.py` - 32 lines of config→args conversion
3. `config_manager.py` - May need deprecated method cleanup

**Test Code (2 files):**
4. `test_get_stats.py` - 19 test functions creating Namespace objects
5. `test_config_manager.py` - 3 tests using deprecated methods

**Demo/Example Code (1 file):**
6. `demo_config_system.py` - Uses deprecated methods in examples
- Keep backward compatibility by having CLI entry point create config from args

**Files to modify:**
- `get_stats.py` (primary changes)

**Example transformation:**
```python
# BEFORE:
def fetch_granular_metrics(client, start_ts, end_ts, args, model_version):
    metrics, histogram, resp = client.get_stats(
        group=args.group,
        units=args.units,
        source=args.source,
        # ... 8 more args.* references
    )

# AFTER:
def fetch_granular_metrics(client, start_ts, end_ts, config, model_version):
    metrics, histogram, resp = client.get_stats(
        group=config.query.group,
        units=config.query.units,
        source=config.query.source,
        timezone=config.query.timezone,
        min_speed=config.query.min_speed,
        compute_histogram=config.query.histogram,
        hist_bucket_size=config.query.hist_bucket_size,
        hist_max=config.query.hist_max,
    )
```

### Phase 2: Remove Namespace Conversion (Medium Risk)
**Goal:** Eliminate the 28+ line config → args conversion

**Changes:**
- In `get_stats.py` main: Pass `config` directly instead of creating `args`
- In `generate_report_api.py`: Pass `config` directly to `get_stats.main()`
- Remove all the `args.field = config.section.field` assignments

**Files to modify:**
- `get_stats.py` (lines 733-768 - delete entire conversion block)
- `generate_report_api.py` (lines 92-123 - delete entire conversion block)

**Example transformation:**
```python
# BEFORE (get_stats.py):
args.dates = [config.query.start_date, config.query.end_date]
args.group = config.query.group
args.units = config.query.units
# ... 25+ more lines
date_ranges = [(args.dates[0], args.dates[1])]
main(date_ranges, args)

# AFTER:
date_ranges = [(config.query.start_date, config.query.end_date)]
main(date_ranges, config)
```

```python
# BEFORE (generate_report_api.py):
args = type("Args", (), {})()
args.dates = [config.query.start_date, config.query.end_date]
# ... 26 more lines
get_stats.main(date_ranges, args)

# AFTER:
date_ranges = [(config.query.start_date, config.query.end_date)]
get_stats.main(date_ranges, config)
```

### Phase 3: Return Generated Files (Low Risk)
**Goal:** Have `main()` return list of generated files for API use

**Changes:**
- Change `main()` return type from `None` to `List[str]`
- Collect filenames from chart generation and PDF assembly
- Return the list at the end

**Files to modify:**
- `get_stats.py`: Update `main()`, `process_date_range()`, `assemble_pdf_report()`
- `generate_report_api.py`: Use returned file list instead of empty array

**Example transformation:**
```python
# BEFORE:
def main(date_ranges, config):
    client = RadarStatsClient()
    for start_date, end_date in date_ranges:
        process_date_range(start_date, end_date, config, client)
    # No return value

# AFTER:
def main(date_ranges, config) -> List[str]:
    """Returns list of generated file paths."""
    client = RadarStatsClient()
    all_files = []
    for start_date, end_date in date_ranges:
        files = process_date_range(start_date, end_date, config, client)
        all_files.extend(files)
    return all_files
```

## Implementation Order

### Step 1: Function Signature Changes
- [ ] Update `resolve_file_prefix(config, start_ts, end_ts)`
- [ ] Update `get_model_version(config)`
- [ ] Update `fetch_granular_metrics(..., config, ...)`
- [ ] Update `fetch_overall_summary(..., config, ...)`
- [ ] Update `fetch_daily_summary(..., config, ...)`
- [ ] Update `generate_histogram_chart(..., config)`
- [ ] Update `generate_timeseries_chart(..., config)`
- [ ] Update `generate_all_charts(..., config, ...)`
- [ ] Update `assemble_pdf_report(..., config)`
- [ ] Update `process_date_range(..., config, ...)`
- [ ] Update `main(date_ranges, config)`

### Step 2: Update Internal References
For each function, replace:
- `args.group` → `config.query.group`
- `args.units` → `config.query.units`
- `args.source` → `config.query.source`
- `args.timezone` → `config.query.timezone`
- `args.min_speed` → `config.query.min_speed`
- `args.histogram` → `config.query.histogram`
- `args.hist_bucket_size` → `config.query.hist_bucket_size`
- `args.hist_max` → `config.query.hist_max`
- `args.file_prefix` → `config.output.file_prefix`
- `args.debug` → `config.output.debug`
- `args.map` → `config.output.map`
- `args.location` → `config.site.location`
- `args.surveyor` → `config.site.surveyor`
- `args.contact` → `config.site.contact`
- `args.speed_limit` → `config.site.speed_limit`
- `args.site_description` → `config.site.site_description`
- `args.speed_limit_note` → `config.site.speed_limit_note`
- `args.cosine_error_angle` → `config.radar.cosine_error_angle`
- `args.sensor_model` → `config.radar.sensor_model`
- `args.firmware_version` → `config.radar.firmware_version`
- `args.transmit_frequency` → `config.radar.transmit_frequency`
- `args.sample_rate` → `config.radar.sample_rate`
- `args.velocity_resolution` → `config.radar.velocity_resolution`
- `args.azimuth_fov` → `config.radar.azimuth_fov`
- `args.elevation_fov` → `config.radar.elevation_fov`

### Step 3: Remove Conversion Code
- [ ] Delete lines 733-768 in `get_stats.py` (args assignment block)
- [ ] Delete lines 92-123 in `generate_report_api.py` (args creation block)
- [ ] Update both files to pass config directly

### Step 4: Add File List Returns
- [ ] Update `assemble_pdf_report()` to return generated files
- [ ] Update `process_date_range()` to collect and return files
- [ ] Update `main()` to collect and return all files
- [ ] Update `generate_report_api.py` to use returned file list

### Step 5: Update Tests
- [ ] Update `test_get_stats.py` to pass config instead of args
- [ ] Update integration tests to verify file list is returned
- [ ] Run full test suite to ensure nothing broke

## Benefits

1. **Code Reduction:** Delete 50+ lines of duplicate conversion code
2. **Type Safety:** Use proper dataclasses instead of dynamic namespace objects
3. **Maintainability:** Single source of truth for configuration structure
4. **Clarity:** `config.radar.sensor_model` is clearer than `args.sensor_model`
5. **API Improvement:** `generate_report_api.py` gets actual file list returned
6. **Future Proof:** Easier to add new config fields without touching multiple places

## Risks & Mitigation

**Risk:** Breaking existing code that calls `get_stats.main()`
**Mitigation:** Only the CLI entry point and API call it - both are in our control

**Risk:** Tests might break
**Mitigation:** Update tests as part of the refactor; comprehensive test coverage exists

**Risk:** Missed args.* references
**Mitigation:** Use grep to find all `args.` references before starting; use IDE refactoring tools

## Testing Strategy

### Pre-Refactor Baseline
1. Run full test suite and record results
2. Generate test PDFs with all three configs (minimal, with-site-info, example)
3. Document current test coverage metrics

### Test Updates Required

#### `test_get_stats.py` (19 functions to update)
**Current pattern:**
```python
args = argparse.Namespace(
    file_prefix="my-prefix",
    timezone=None,
    source="radar_data_transits"
)
result = resolve_file_prefix(args, start_ts, end_ts)
```

**New pattern:**
```python
config = ReportConfig(
    query=QueryConfig(timezone="UTC", source="radar_data_transits"),
    output=OutputConfig(file_prefix="my-prefix")
)
result = resolve_file_prefix(config, start_ts, end_ts)
```

**Functions to update:**
1. `test_with_user_provided_prefix()` - line 90
2. `test_without_user_prefix()` - line 103
3. `test_fetch_granular_metrics_success()` - line 115
4. `test_fetch_granular_metrics_with_histogram()` - line 137
5. `test_fetch_granular_metrics_failure()` - line 162
6. `test_fetch_overall_summary_success()` - line 191
7. `test_fetch_overall_summary_failure()` - line 204
8. `test_fetch_daily_summary_success()` - line 222
9. `test_fetch_daily_summary_no_daily_needed()` - line 238
10. `test_fetch_daily_summary_failure()` - line 256
11. `test_generate_histogram_chart()` - line 282
12. `test_generate_histogram_chart_import_error()` - line 301
13. `test_generate_histogram_chart_save_failure()` - line 313
14. `test_generate_timeseries_chart()` - line 332
15. `test_generate_timeseries_chart_failure()` - line 350
16. `test_assemble_pdf_report()` - line 365
17. `test_assemble_pdf_report_failure()` - line 388
18. `test_process_date_range()` - line 430
19. `test_get_model_version()` - line 454

**Total lines to change:** ~150-200 (estimate)

#### `test_config_manager.py` (3 deprecated tests to fix/remove)

**Option 1: Remove deprecated tests**
- Delete `test_from_cli_args()` (line 186) - tests non-existent method
- Delete `test_merge_with_env()` (line ~249) - tests non-existent method
- Update `test_load_config_from_file()` if signature changed

**Option 2: Mark as deprecated/skip**
```python
@unittest.skip("Deprecated: from_cli_args() removed in favor of JSON-only config")
def test_from_cli_args():
    ...
```

**Recommendation:** Remove deprecated tests to clean up codebase

#### `demo_config_system.py` (2 demos to update/remove)

**Current deprecated demos:**
1. `demo_cli_args()` - line 150 (uses `from_cli_args()`)
2. `demo_priority_system()` - line 179 (uses `from_env()`)

**Options:**
- Remove entirely (these are outdated examples)
- Update to show JSON-only workflow
- Add note that they're deprecated

**Recommendation:** Remove or update to match current JSON-only architecture

### Integration Testing
1. Test with all three config files after refactor:
   - `config.minimal.json`
   - `config.with-site-info.json`
   - `config.example.json`
2. Verify generated PDFs are byte-identical to pre-refactor
3. Test both CLI and API entry points
4. Test error cases (invalid config, missing fields)

### Regression Testing Checklist
- [ ] All unit tests pass
- [ ] All integration tests pass
- [ ] Generated PDFs identical to baseline
- [ ] No new linter warnings
- [ ] No type checking errors
- [ ] CLI still works: `./get_stats.py config.json`
- [ ] API still works: `generate_report_from_file(config.json)`
- [ ] Error messages are clear and helpful

## Estimated Effort (Updated)

| Phase | Task | Time | Risk |
|-------|------|------|------|
| **Phase 1** | Update 11 function signatures in get_stats.py | 3-4 hours | Medium |
| **Phase 2** | Remove conversion code (67 lines total) | 30 min | Low |
| **Phase 3** | Add file list returns | 1-2 hours | Low |
| **Phase 4** | Update 19 test functions in test_get_stats.py | 2-3 hours | Medium |
| **Phase 5** | Fix test_config_manager.py (remove 3 tests) | 30 min | Low |
| **Phase 6** | Update/remove demo_config_system.py | 30 min | Low |
| **Phase 7** | Integration testing & verification | 2-3 hours | Critical |
| **Buffer** | Unexpected issues & cleanup | 1-2 hours | - |
| **TOTAL** | **Complete refactor** | **11-16 hours** | **Medium** |

## Implementation Order (Updated)

### Phase 1: Core Function Refactor (3-4 hours)
- [ ] 1.1: Update `resolve_file_prefix(config, start_ts, end_ts)`
- [ ] 1.2: Update `get_model_version(config)`
- [ ] 1.3: Update `fetch_granular_metrics(..., config, ...)`
- [ ] 1.4: Update `fetch_overall_summary(..., config, ...)`
- [ ] 1.5: Update `fetch_daily_summary(..., config, ...)`
- [ ] 1.6: Update `generate_histogram_chart(..., config)`
- [ ] 1.7: Update `generate_timeseries_chart(..., config)`
- [ ] 1.8: Update `generate_all_charts(..., config, ...)`
- [ ] 1.9: Update `assemble_pdf_report(..., config)` + remove SITE_INFO fallbacks
- [ ] 1.10: Update `process_date_range(..., config, ...)`
- [ ] 1.11: Update `main(date_ranges, config)`

### Phase 2: Remove Conversion Code (30 min)
- [ ] 2.1: Delete lines 733-768 in `get_stats.py` (35 lines)
- [ ] 2.2: Delete lines 92-123 in `generate_report_api.py` (32 lines)
- [ ] 2.3: Update both files to pass config directly
- [ ] 2.4: Remove any remaining args references

### Phase 3: Add File List Returns (1-2 hours)
- [ ] 3.1: Update `assemble_pdf_report()` to return List[str]
- [ ] 3.2: Update `process_date_range()` to collect and return files
- [ ] 3.3: Update `main()` to collect and return all files
- [ ] 3.4: Update `generate_report_api.py` to use returned file list
- [ ] 3.5: Add file tracking to chart generation functions

### Phase 4: Update Test Suite (2-3 hours)
- [ ] 4.1: Create helper function to create test configs
  ```python
  def create_test_config(**overrides) -> ReportConfig:
      """Helper to create config with test defaults."""
      return ReportConfig(
          site=SiteConfig(location="Test", surveyor="Test", contact="test@test.com"),
          query=QueryConfig(start_date="2025-01-01", end_date="2025-01-02", timezone="UTC"),
          radar=RadarConfig(cosine_error_angle=21.0),
          output=OutputConfig(file_prefix="test"),
          **overrides
      )
  ```
- [ ] 4.2: Update all 19 test functions in `test_get_stats.py`
- [ ] 4.3: Verify all tests pass
- [ ] 4.4: Add new tests for file list return values

### Phase 5: Clean Up Test Config Manager (30 min)
- [ ] 5.1: Remove `test_from_cli_args()` (line 186)
- [ ] 5.2: Remove `test_merge_with_env()` (if exists)
- [ ] 5.3: Update `test_load_config_from_file()` if needed
- [ ] 5.4: Update site config defaults test (lines 20-27)
- [ ] 5.5: Update query config defaults test (lines 30-36)
- [ ] 5.6: Update validation test (lines 162-176)

### Phase 6: Update Demo Files (30 min)
- [ ] 6.1: Remove or update `demo_cli_args()` in demo_config_system.py
- [ ] 6.2: Remove or update `demo_priority_system()`
- [ ] 6.3: Add note about JSON-only configuration
- [ ] 6.4: Consider creating new demo showing config workflow

### Phase 7: Integration Testing (2-3 hours)
- [ ] 7.1: Run full test suite (before refactor - baseline)
- [ ] 7.2: Run full test suite (after each phase)
- [ ] 7.3: Generate PDFs with all three config files
- [ ] 7.4: Compare PDFs to baseline (should be identical)
- [ ] 7.5: Test CLI: `./get_stats.py config.minimal.json`
- [ ] 7.6: Test API: Import and call functions
- [ ] 7.7: Test error cases (invalid configs)
- [ ] 7.8: Check for any remaining `argparse.Namespace` references
- [ ] 7.9: Run linter and type checker
- [ ] 7.10: Update documentation if needed

## Success Criteria

- [ ] All TODOs removed
- [ ] No more `argparse.Namespace` in `get_stats.py` functions
- [ ] All tests passing
- [ ] Generated PDFs identical to pre-refactor
- [ ] API returns actual file list
- [ ] Code is cleaner and more maintainable
- [ ] All 19 test functions updated to use ReportConfig
- [ ] Deprecated tests removed from test_config_manager.py
- [ ] Demo file updated or deprecated sections removed
- [ ] No SITE_INFO fallbacks remaining

## Complete Field Mapping

### args.* → config.* Transformations (28 fields)

This comprehensive mapping shows all field conversions from argparse.Namespace to ReportConfig:

#### Site Configuration Fields (7 fields)
| Old (args.*) | New (config.*) | Default | Notes |
|-------------|----------------|---------|-------|
| `args.location` | `config.site.location` | `None` | Site physical location |
| `args.surveyor` | `config.site.surveyor` | `None` | Surveyor name |
| `args.contact` | `config.site.contact` | `None` | Contact email |
| `args.speed_limit` | `config.site.speed_limit` | `None` | Speed limit (mph) |
| `args.speed_limit_note` | `config.site.speed_limit_note` | `None` | Contextual note |
| `args.map_url` | `config.site.map_url` | `None` | Google Maps URL |
| `args.map_caption` | `config.site.map_caption` | `None` | Map caption text |

#### Query Configuration Fields (4 fields)
| Old (args.*) | New (config.*) | Default | Notes |
|-------------|----------------|---------|-------|
| `args.start_date` | `config.query.start_date` | *required* | ISO 8601 date |
| `args.end_date` | `config.query.end_date` | *required* | ISO 8601 date |
| `args.timezone` | `config.query.timezone` | `"America/Los_Angeles"` | Timezone name |
| `args.source` | `config.query.source` | `"sensor_data_transits"` | Data source table |

#### Radar Configuration Fields (11 fields)
| Old (args.*) | New (config.*) | Default | Notes |
|-------------|----------------|---------|-------|
| `args.cosine_error_angle` | `config.radar.cosine_error_angle` | `21.0` | Angle in degrees |
| `args.elevation_fov` | `config.radar.elevation_fov` | `None` | Elevation field of view |
| `args.horizontal_fov` | `config.radar.horizontal_fov` | `None` | Horizontal field of view |
| `args.lanes_covered` | `config.radar.lanes_covered` | `None` | Number of lanes |
| `args.maximum_range` | `config.radar.maximum_range` | `None` | Max detection range |
| `args.azimuth_angle` | `config.radar.azimuth_angle` | `None` | Azimuth angle (°) |
| `args.elevation_angle` | `config.radar.elevation_angle` | `None` | Elevation angle (°) |
| `args.height` | `config.radar.height` | `None` | Sensor height (ft) |
| `args.lateral_distance` | `config.radar.lateral_distance` | `None` | Lateral distance (ft) |
| `args.mounting_side` | `config.radar.mounting_side` | `None` | "left" or "right" |
| `args.setback_distance` | `config.radar.setback_distance` | `None` | Setback distance (ft) |

**Computed Property (not stored in config):**
- `cosine_error_factor = 1 / cos(radians(cosine_error_angle))`

#### Output Configuration Fields (6 fields)
| Old (args.*) | New (config.*) | Default | Notes |
|-------------|----------------|---------|-------|
| `args.file_prefix` | `config.output.file_prefix` | `None` | PDF filename prefix |
| `args.histogram_max_x` | `config.output.histogram_max_x` | `None` | Chart X-axis max |
| `args.histogram_max_y` | `config.output.histogram_max_y` | `None` | Chart Y-axis max |
| `args.timeseries_max_y` | `config.output.timeseries_max_y` | `None` | Chart Y-axis max |
| `args.use_histogram` | `config.output.use_histogram` | `False` | Enable histogram |
| `args.use_timeseries` | `config.output.use_timeseries` | `True` | Enable timeseries |

### Field Usage Examples

#### Before (argparse.Namespace):
```python
def assemble_pdf_report(
    start_ts: datetime,
    end_ts: datetime,
    overall_summary: Dict,
    daily_summary: Optional[List[Dict]],
    granular_metrics: List[Dict],
    args: argparse.Namespace,
    chart_files: List[str] = []
) -> str:
    # Access fields via args.field
    location = getattr(args, 'location', None) or SITE_INFO.get('location')  # ❌ Fallback to SITE_INFO
    surveyor = getattr(args, 'surveyor', None) or SITE_INFO.get('surveyor')

    # 28 fields accessed this way...
```

#### After (ReportConfig):
```python
def assemble_pdf_report(
    start_ts: datetime,
    end_ts: datetime,
    overall_summary: Dict,
    daily_summary: Optional[List[Dict]],
    granular_metrics: List[Dict],
    config: ReportConfig,
    chart_files: List[str] = []
) -> str:
    # Access fields via config.section.field
    location = config.site.location  # ✅ Direct access, uses dataclass defaults
    surveyor = config.site.surveyor

    # All fields accessed cleanly through sections
```

### Access Pattern Changes

#### Old Pattern (with fallbacks):
```python
# ❌ Complex fallback logic scattered throughout code
location = getattr(args, 'location', None) or SITE_INFO.get('location')
surveyor = getattr(args, 'surveyor', None) or SITE_INFO.get('surveyor')
contact = getattr(args, 'contact', None) or SITE_INFO.get('contact')
speed_limit = getattr(args, 'speed_limit', None) or SITE_INFO.get('speed_limit')
```

#### New Pattern (clean defaults):
```python
# ✅ Defaults handled by dataclass at load time
location = config.site.location
surveyor = config.site.surveyor
contact = config.site.contact
speed_limit = config.site.speed_limit
```

### Benefits of New Pattern
1. **Type Safety**: IDE autocomplete and type checking work properly
2. **No Fallbacks**: Defaults defined once in dataclass, not scattered in code
3. **Clearer Structure**: Logical grouping (site, query, radar, output)
4. **Easier Testing**: Mock a dataclass instead of Namespace
5. **Better Documentation**: Dataclass fields document themselves
6. **Immutability**: Dataclasses can be frozen, preventing accidental modification

## File-by-File Transformation Examples

### Example 1: `get_stats.py` - resolve_file_prefix()

**Before:**
```python
def resolve_file_prefix(args: argparse.Namespace, start_ts: datetime, end_ts: datetime) -> str:
    """Build file prefix from args or generate from dates."""
    if args.file_prefix:
        return args.file_prefix
    else:
        start_str = start_ts.strftime("%Y%m%d")
        end_str = end_ts.strftime("%Y%m%d")
        return f"out-{start_str}-{end_str}"
```

**After:**
```python
def resolve_file_prefix(config: ReportConfig, start_ts: datetime, end_ts: datetime) -> str:
    """Build file prefix from config or generate from dates."""
    if config.output.file_prefix:
        return config.output.file_prefix
    else:
        start_str = start_ts.strftime("%Y%m%d")
        end_str = end_ts.strftime("%Y%m%d")
        return f"out-{start_str}-{end_str}"
```

**Changes:**
- Parameter: `args: argparse.Namespace` → `config: ReportConfig`
- Access: `args.file_prefix` → `config.output.file_prefix`
- Lines changed: 2

### Example 2: `get_stats.py` - assemble_pdf_report() (complex case)

**Before (lines 447-460):**
```python
def assemble_pdf_report(
    start_ts: datetime,
    end_ts: datetime,
    overall_summary: Dict,
    daily_summary: Optional[List[Dict]],
    granular_metrics: List[Dict],
    args: argparse.Namespace,
    chart_files: List[str] = []
) -> str:
    """Generate PDF report with LaTeX."""

    # Extract site information with fallbacks
    location = getattr(args, 'location', None) or SITE_INFO.get('location')
    surveyor = getattr(args, 'surveyor', None) or SITE_INFO.get('surveyor')
    contact = getattr(args, 'contact', None) or SITE_INFO.get('contact')
    speed_limit = getattr(args, 'speed_limit', None) or SITE_INFO.get('speed_limit')
    speed_limit_note = getattr(args, 'speed_limit_note', None) or SITE_INFO.get('speed_limit_note')
    map_url = getattr(args, 'map_url', None)
    map_caption = getattr(args, 'map_caption', None)

    # ... 20+ more field extractions with fallbacks ...
```

**After:**
```python
def assemble_pdf_report(
    start_ts: datetime,
    end_ts: datetime,
    overall_summary: Dict,
    daily_summary: Optional[List[Dict]],
    granular_metrics: List[Dict],
    config: ReportConfig,
    chart_files: List[str] = []
) -> str:
    """Generate PDF report with LaTeX."""

    # Site information - no fallbacks needed, dataclass handles defaults
    location = config.site.location
    surveyor = config.site.surveyor
    contact = config.site.contact
    speed_limit = config.site.speed_limit
    speed_limit_note = config.site.speed_limit_note
    map_url = config.site.map_url
    map_caption = config.site.map_caption

    # Radar configuration
    cosine_error_factor = config.radar.cosine_error_factor
    # ... all other fields accessed directly ...
```

**Changes:**
- Parameter: `args: argparse.Namespace` → `config: ReportConfig`
- Removed: All `getattr(args, ...)` and `SITE_INFO.get()` fallbacks
- Simplified: 28 field accesses from complex fallback logic to simple attribute access
- Lines removed: ~15-20 (removed fallback code)

### Example 3: `test_get_stats.py` - test function update

**Before:**
```python
def test_with_user_provided_prefix():
    """Test that user-provided prefix is used."""
    args = argparse.Namespace(
        file_prefix="my-custom-prefix",
        timezone=None
    )
    start_ts = datetime(2025, 1, 1, tzinfo=ZoneInfo("UTC"))
    end_ts = datetime(2025, 1, 2, tzinfo=ZoneInfo("UTC"))

    result = resolve_file_prefix(args, start_ts, end_ts)
    assert result == "my-custom-prefix"
```

**After:**
```python
def test_with_user_provided_prefix():
    """Test that user-provided prefix is used."""
    config = ReportConfig(
        query=QueryConfig(
            start_date="2025-01-01",
            end_date="2025-01-02",
            timezone="UTC"
        ),
        output=OutputConfig(
            file_prefix="my-custom-prefix"
        )
    )
    start_ts = datetime(2025, 1, 1, tzinfo=ZoneInfo("UTC"))
    end_ts = datetime(2025, 1, 2, tzinfo=ZoneInfo("UTC"))

    result = resolve_file_prefix(config, start_ts, end_ts)
    assert result == "my-custom-prefix"
```

**Changes:**
- Replaced: `argparse.Namespace()` → `ReportConfig()` with proper sections
- Grouped: Related fields into logical sections (query, output)
- Function call: `resolve_file_prefix(args, ...)` → `resolve_file_prefix(config, ...)`

### Example 4: Test helper function (recommended)

**Create in `test_get_stats.py`:**
```python
def create_test_config(
    file_prefix: str = "test",
    start_date: str = "2025-01-01",
    end_date: str = "2025-01-02",
    timezone: str = "UTC",
    source: str = "sensor_data_transits",
    **kwargs
) -> ReportConfig:
    """Helper to create test configs with sensible defaults."""
    return ReportConfig(
        site=SiteConfig(
            location="Test Site",
            surveyor="Test Surveyor",
            contact="test@example.com"
        ),
        query=QueryConfig(
            start_date=start_date,
            end_date=end_date,
            timezone=timezone,
            source=source
        ),
        radar=RadarConfig(cosine_error_angle=21.0),
        output=OutputConfig(file_prefix=file_prefix),
        **kwargs
    )
```

**Usage in tests:**
```python
def test_with_user_provided_prefix():
    """Test that user-provided prefix is used."""
    config = create_test_config(file_prefix="my-custom-prefix")
    # ... rest of test ...
```

## Risk Assessment

### Medium Risk Areas
1. **Function Signature Changes**: 11 functions change signatures
   - **Mitigation**: Type checking will catch signature mismatches
   - **Testing**: Comprehensive test suite will verify correctness

2. **Test Updates**: 19 test functions need updating
   - **Mitigation**: Update tests phase-by-phase
   - **Testing**: Run tests after each function update

### Low Risk Areas
1. **Removing Conversion Code**: Simple deletions (67 + 32 lines)
   - **Mitigation**: No logic changes, just removal
   - **Testing**: Integration tests verify end-to-end

2. **File List Returns**: Additive change, doesn't break existing
   - **Mitigation**: Return empty list if not implemented yet
   - **Testing**: Specific tests for return values

3. **Deprecated Test Cleanup**: Removing non-working tests
   - **Mitigation**: Tests already fail, removal can't make it worse
   - **Testing**: Verify remaining tests still pass

### Critical Testing Points
- [ ] Generated PDFs are byte-identical before/after refactor
- [ ] All three config files (minimal, with-site-info, example) work
- [ ] Both CLI and API entry points function correctly
- [ ] No regression in error handling or validation
- [ ] Type checking passes without new errors

## Rollback Plan

If issues are encountered during refactor:

1. **Git Branching Strategy**
   - Create branch: `git checkout -b refactor/remove-argparse-namespace`
   - Commit after each phase
   - Can revert to any phase if issues found

2. **Phase-by-Phase Rollback**
   - Each phase is independently testable
   - Can stop at any phase and ship partial refactor
   - Phases 1-2 are most critical (function updates)

3. **Validation at Each Phase**
   - Run full test suite after each phase
   - Generate test PDFs and compare to baseline
   - If tests fail, investigate before continuing

4. **Complete Rollback**
   - If critical issues found: `git checkout main`
   - Re-evaluate approach
   - Consider smaller incremental changes

## Post-Refactor Improvements

After completing the refactor, consider these follow-up improvements:

1. **Deprecate SITE_INFO Dictionary**
   - Remove `SITE_INFO` constant entirely
   - All defaults now in `SiteConfig` dataclass
   - Eliminates duplicate default definitions

2. **Add Config Validation**
   - Add `__post_init__` validation to config dataclasses
   - Validate date ranges (start < end)
   - Validate numeric ranges (angles, distances)
   - Validate enum values (mounting_side)

3. **Config Schema Documentation**
   - Generate JSON schema from dataclasses
   - Add to documentation
   - Enable IDE autocomplete for config files

4. **Type Annotations**
   - Ensure all functions have complete type hints
   - Run mypy in strict mode
   - Add py.typed marker for library usage

5. **Performance Testing**
   - Benchmark report generation before/after
   - Should be no performance difference
   - Document any findings

