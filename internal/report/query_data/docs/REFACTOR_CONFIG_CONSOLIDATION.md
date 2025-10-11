# Configuration Consolidation Refactor Plan

**Status:** In Progress
**Started:** 2025-10-11
**Goal:** Eliminate `os.getenv()` calls and consolidate all configuration into JSON-based `config_manager.py`

---

## Current State Analysis

### Problem
Configuration is split across two systems:
1. **`report_config.py`**: Module-level dictionaries with `os.getenv()` calls (17 environment variables)
2. **`config_manager.py`**: JSON-based dataclass configuration (already has site, radar, query, output)

### Issues
- Environment variables scattered in `report_config.py` override JSON config
- No single source of truth for configuration
- Testing is difficult (module-level mutations via `override_site_info`)
- Config can't be fully serialized to JSON
- Downstream modules import both systems

### Goal
**Unified JSON Configuration System:**
- All config values in `config_manager.py` dataclasses
- No `os.getenv()` calls in production code
- Complete JSON serialization/deserialization
- Config passed explicitly through function parameters
- `report_config.py` becomes legacy compatibility layer (deprecated)

---

## Phase 1: Extend `config_manager.py` ✅

### Task 1.1: Add New Dataclass Sections

Add these dataclasses to `config_manager.py`:

```python
@dataclass
class ColorConfig:
    """Color palette for charts and reports."""
    p50: str = "#fbd92f"  # Yellow - 50th percentile
    p85: str = "#f7b32b"  # Orange - 85th percentile
    p98: str = "#f25f5c"  # Red/Pink - 98th percentile
    max: str = "#2d1e2f"  # Dark purple/black - maximum
    count_bar: str = "#2d1e2f"  # Gray bars for counts
    low_sample: str = "#f7b32b"  # Orange highlight for low-sample periods

@dataclass
class FontConfig:
    """Font sizes for charts and documents."""
    chart_title: int = 14
    chart_label: int = 13
    chart_tick: int = 11
    chart_axis_label: int = 8
    chart_axis_tick: int = 7
    chart_legend: int = 7
    histogram_title: int = 14
    histogram_label: int = 13
    histogram_tick: int = 11

@dataclass
class LayoutConfig:
    """Layout dimensions and constraints."""
    # Figure dimensions (inches) - stored as tuples
    chart_figsize_width: float = 24.0
    chart_figsize_height: float = 8.0
    histogram_figsize_width: float = 3.0
    histogram_figsize_height: float = 2.0

    # Thresholds
    low_sample_threshold: int = 50
    count_missing_threshold: int = 5

    # Bar chart widths (as fractions of bucket spacing)
    bar_width_bg_fraction: float = 0.95
    bar_width_fraction: float = 0.7

    # Chart sizing constraints
    min_chart_width_in: float = 6.0
    max_chart_width_in: float = 11.0

    # Chart margins and spacing (for fig.subplots_adjust)
    chart_left: float = 0.02
    chart_right: float = 0.96
    chart_top: float = 0.995
    chart_bottom: float = 0.16

    # Y-axis scaling
    count_axis_scale: float = 1.6

    # Line and marker styling
    line_width: float = 1.0
    marker_size: int = 4
    marker_edge_width: float = 0.4

    @property
    def chart_figsize(self) -> tuple:
        """Get chart figure size as tuple."""
        return (self.chart_figsize_width, self.chart_figsize_height)

    @property
    def histogram_figsize(self) -> tuple:
        """Get histogram figure size as tuple."""
        return (self.histogram_figsize_width, self.histogram_figsize_height)

@dataclass
class PdfConfig:
    """PDF/LaTeX document settings."""
    geometry_top: str = "1.8cm"
    geometry_bottom: str = "1.0cm"
    geometry_left: str = "1.0cm"
    geometry_right: str = "1.0cm"
    columnsep: str = "14"  # Points
    headheight: str = "12pt"
    headsep: str = "10pt"
    fonts_dir: str = "fonts"

    @property
    def geometry(self) -> Dict[str, str]:
        """Get geometry as dictionary for backward compatibility."""
        return {
            "top": self.geometry_top,
            "bottom": self.geometry_bottom,
            "left": self.geometry_left,
            "right": self.geometry_right,
        }

@dataclass
class MapConfig:
    """SVG map marker configuration."""
    # Triangle marker properties
    triangle_len: float = 0.42
    triangle_cx: float = 0.385
    triangle_cy: float = 0.71
    triangle_apex_angle: float = 20.0
    triangle_angle: float = 32.0
    triangle_color: str = "#f25f5c"
    triangle_opacity: float = 0.9

    # Circle marker at triangle apex
    circle_radius: float = 20.0
    circle_fill: str = "#ffffff"
    circle_stroke: Optional[str] = None  # Defaults to triangle_color
    circle_stroke_width: str = "2"

    def __post_init__(self):
        """Compute circle_stroke default to match triangle_color."""
        if self.circle_stroke is None:
            self.circle_stroke = self.triangle_color

@dataclass
class HistogramProcessingConfig:
    """Histogram processing defaults."""
    default_cutoff: float = 5.0
    default_bucket_size: float = 5.0
    default_max_bucket: float = 50.0

@dataclass
class DebugConfig:
    """Debug settings."""
    plot_debug: bool = False
```

### Task 1.2: Update ReportConfig

Add new fields to `ReportConfig` dataclass:

```python
@dataclass
class ReportConfig:
    """Complete report configuration."""
    site: SiteConfig = field(default_factory=SiteConfig)
    radar: RadarConfig = field(default_factory=RadarConfig)
    query: QueryConfig = field(default_factory=QueryConfig)
    output: OutputConfig = field(default_factory=OutputConfig)

    # Visual/presentation configuration
    colors: ColorConfig = field(default_factory=ColorConfig)
    fonts: FontConfig = field(default_factory=FontConfig)
    layout: LayoutConfig = field(default_factory=LayoutConfig)
    pdf: PdfConfig = field(default_factory=PdfConfig)
    map: MapConfig = field(default_factory=MapConfig)
    histogram_processing: HistogramProcessingConfig = field(
        default_factory=HistogramProcessingConfig
    )
    debug: DebugConfig = field(default_factory=DebugConfig)

    # Metadata
    created_at: Optional[str] = None
    updated_at: Optional[str] = None
    version: str = "1.0"
```

### Task 1.3: Update Helper Methods

Update `to_dict()`, `from_dict()`, `from_json()` to handle new sections.

### Task 1.4: Update Example Config

Modify `create_example_config()` to populate all sections with examples.

---

## Phase 2: Create Compatibility Layer in `report_config.py`

### Goal
Keep existing imports working but deprecate the module.

### Approach
Replace `os.getenv()` calls with hardcoded defaults matching the new dataclasses.
Add deprecation warning at module level.

```python
import warnings

warnings.warn(
    "report_config module is deprecated. Use config_manager.ReportConfig instead. "
    "See docs/REFACTOR_CONFIG_CONSOLIDATION.md for migration guide.",
    DeprecationWarning,
    stacklevel=2
)

# Keep existing dict definitions with hardcoded defaults (no os.getenv)
COLORS: Dict[str, str] = {
    "p50": "#fbd92f",
    "p85": "#f7b32b",
    # ... (all hardcoded)
}
```

---

## Phase 3: Update Downstream Modules

### Pattern: Parameter Injection Instead of Module Imports

**Modules to Update:**
1. `get_stats.py` - Main orchestrator, passes config downstream
2. `pdf_generator.py` - Accept `config: ReportConfig` parameter
3. `chart_builder.py` - Accept `colors`, `fonts`, `layout`, `debug` params
4. `stats_utils.py` - Accept `fonts`, `layout`, `histogram_config` params
5. `document_builder.py` - Accept `pdf_config`, `site_config` params
6. `report_sections.py` - Accept `site_config` param
7. `chart_saver.py` - Accept `layout` param

### Example Migration

**Before:**
```python
from report_config import PDF_CONFIG, MAP_CONFIG, SITE_INFO

def generate_pdf_report(output_path: str, ...):
    geometry = PDF_CONFIG["geometry"]
```

**After:**
```python
from config_manager import ReportConfig

def generate_pdf_report(
    output_path: str,
    config: ReportConfig,
    ...
):
    geometry = config.pdf.geometry
```

---

## Phase 4: Update Tests

### Key Changes

1. **`test_report_config.py`**: Add deprecation warning tests
2. **Create config fixtures** in `conftest.py`:
```python
@pytest.fixture
def base_config():
    """Base report configuration for tests."""
    return ReportConfig(...)
```
3. **Update test files**: Use config fixtures instead of module imports
4. **Test backward compatibility**: Ensure deprecated imports still work

---

## Phase 5: Integration & Validation

### Task 17: Full Test Suite
```bash
pytest --cov=. --cov-report=term-missing
```
- Target: All 505 tests passing
- Target: Maintain/improve 86% coverage

### Task 18: Documentation
- Create `MIGRATION.md` with before/after examples
- Update module docstrings
- Document new JSON config structure

---

## Benefits

1. ✅ **Single Source of Truth**: All config in JSON/dataclasses
2. ✅ **Testability**: Config is explicit parameter, easy to mock/inject
3. ✅ **Serialization**: Complete config can be saved/loaded as JSON
4. ✅ **Type Safety**: Dataclasses provide validation and IDE autocomplete
5. ✅ **No Global State**: No module-level mutations
6. ✅ **Environment Independence**: No `os.getenv()` in production code
7. ✅ **Backward Compatible**: Existing code still works (with warnings)

---

## Risks & Mitigation

| Risk | Mitigation |
|------|------------|
| Breaking changes in downstream code | Phased rollout, deprecation warnings, maintain backward compatibility |
| Tests may fail during transition | Update tests incrementally, use fixtures for config objects |
| Large refactor scope (18 tasks) | Work in phases, validate each phase before proceeding |

---

## Estimated Effort

- **Phase 1 (Config Extension):** 2-3 hours
- **Phase 2 (Compatibility Layer):** 1 hour
- **Phase 3 (Downstream Updates):** 4-5 hours (7 modules)
- **Phase 4 (Test Updates):** 3-4 hours (5 test files)
- **Phase 5 (Integration):** 2-3 hours (validation + docs)

**Total:** ~12-16 hours

---

## Progress Tracking

### Phase 1: Extend config_manager.py
- [ ] Task 1: Add new dataclass sections (ColorConfig, FontConfig, etc.)
- [ ] Task 2: Update ReportConfig with new fields
- [ ] Task 3: Update create_example_config()

### Phase 2: Compatibility Layer
- [ ] Task 4: Deprecate report_config.py module-level constants

### Phase 3: Downstream Updates
- [ ] Task 5: Update get_stats.py
- [ ] Task 6: Update pdf_generator.py
- [ ] Task 7: Update chart_builder.py
- [ ] Task 8: Update stats_utils.py
- [ ] Task 9: Update document_builder.py
- [ ] Task 10: Update report_sections.py
- [ ] Task 11: Update chart_saver.py

### Phase 4: Test Updates
- [ ] Task 12: Update test_report_config.py
- [ ] Task 13: Update test_get_stats.py
- [ ] Task 14: Update test_pdf_generator.py
- [ ] Task 15: Update test_chart_builder.py
- [ ] Task 16: Update test_stats_utils.py

### Phase 5: Integration
- [ ] Task 17: Run full test suite and fix issues
- [ ] Task 18: Create MIGRATION.md and update docs

---

## Notes

- All `os.getenv()` calls will be removed from production code
- Environment variables can still be used at application startup to populate JSON config
- Backward compatibility maintained through deprecation warnings
- Migration path documented for downstream users
