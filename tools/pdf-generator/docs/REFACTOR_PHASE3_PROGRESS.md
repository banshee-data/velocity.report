# Configuration Refactor - Phase 3 Progress

**Status:** In Progress
**Started:** 2025-10-11
**Current Task:** Updating downstream modules to accept `ReportConfig` parameters

---

## Completed Tasks

### ✅ Phase 1: Extended `config_manager.py` (COMPLETE)
- Added 7 new dataclass sections (ColorConfig, FontConfig, LayoutConfig, PdfConfig, MapConfig, HistogramProcessingConfig, DebugConfig)
- Updated `ReportConfig` to include all new sections
- Updated `from_dict()` to handle all 11 config sections
- Updated `create_example_config()` with complete examples
- All 19 config_manager tests passing
- All 505 total tests passing
- JSON round-trip verified

### ✅ Phase 2: Deprecated `report_config.py` (COMPLETE)
- Removed all 16 `os.getenv()` calls
- Added module-level deprecation warning
- Updated module docstring with migration guide
- Removed environment variable override test
- All 36 report_config tests passing
- All 504 total tests passing (1 removed)
- Deprecation warning properly triggers

### ✅ Phase 3.1: Updated `chart_saver.py` (COMPLETE)
- Removed `from report_config import LAYOUT`
- Added module-level constants: `DEFAULT_MIN_CHART_WIDTH_IN`, `DEFAULT_MAX_CHART_WIDTH_IN`
- Updated `ChartSaver.__init__()` to use new defaults
- All 21 chart_saver tests passing
- No deprecation warning from this module

---

## Remaining Phase 3 Tasks

### Module Update Strategy

For each remaining module, we need to:
1. **Remove deprecated imports** from `report_config`
2. **Accept config parameters** in constructors/functions
3. **Maintain backward compatibility** where possible
4. **Update all call sites** to pass config
5. **Update tests** to use config objects

### Module Dependency Graph

```
get_stats.py (orchestrator)
    ↓ uses
    ├── chart_builder.py (COLORS, FONTS, LAYOUT, DEBUG)
    ├── stats_utils.py (FONTS, LAYOUT, HISTOGRAM_CONFIG)
    ├── pdf_generator.py (PDF_CONFIG, MAP_CONFIG, SITE_INFO)
    │   ↓ uses
    │   ├── document_builder.py (PDF_CONFIG, SITE_INFO)
    │   └── report_sections.py (SITE_INFO)
    └── chart_saver.py ✅ (DONE - uses defaults)
```

### Recommended Update Order

**Leaf modules first (no dependencies):**
1. ✅ `chart_saver.py` - DONE
2. `report_sections.py` - Only imports SITE_INFO
3. `document_builder.py` - Imports PDF_CONFIG, SITE_INFO

**Mid-level modules:**
4. `chart_builder.py` - Imports COLORS, FONTS, LAYOUT, DEBUG
5. `stats_utils.py` - Imports FONTS, LAYOUT, HISTOGRAM_CONFIG

**High-level modules:**
6. `pdf_generator.py` - Imports PDF_CONFIG, MAP_CONFIG, SITE_INFO (uses document_builder, report_sections)
7. `get_stats.py` - Orchestrator (uses everything)

---

## Next Module: `report_sections.py`

**Current imports:**
```python
from report_config import SITE_INFO
```

**Strategy:**
- Add optional `site_config` parameter to all functions
- Default to creating from `config_manager.SiteConfig()` if not provided
- Update docstrings to note new parameter

**Functions to update:**
- Any function that uses `SITE_INFO["location"]` etc.
- Check how many functions there are

---

## Backward Compatibility Approach

**For library-style modules (chart_builder, stats_utils):**
- Make config parameters optional with `None` default
- Create default config on-the-fly if not provided
- Issue deprecation warning when using defaults

**For application modules (get_stats, pdf_generator):**
- Require config parameter (no backward compatibility needed)
- These are internal to the application, not public API

---

## Testing Strategy

**For each module:**
1. Update the module code
2. Run that module's tests
3. Update tests if needed to pass config
4. Run full test suite
5. Fix any integration issues

**After all modules:**
1. Run full test suite (target: 504+ tests passing)
2. Check coverage (target: maintain 86%+)
3. Verify no deprecation warnings from updated code
4. Verify deprecation warnings from legacy imports

---

## Progress Checklist

- [x] Phase 1: Extended config_manager.py
- [x] Phase 2: Deprecated report_config.py
- [ ] Phase 3: Update downstream modules
  - [x] chart_saver.py
  - [ ] report_sections.py
  - [ ] document_builder.py
  - [ ] chart_builder.py
  - [ ] stats_utils.py
  - [ ] pdf_generator.py
  - [ ] get_stats.py
- [ ] Phase 4: Update tests
- [ ] Phase 5: Documentation & cleanup

