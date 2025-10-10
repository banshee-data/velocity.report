# Refactoring Tasks 7-10: Implementation Plan

**Project:** velocity.report refactoring
**Date:** October 9, 2025
**Status:** Priority 3 Tasks (Final Phase)

## Overview

This document outlines the detailed implementation plan for completing the final 4 refactoring tasks. These tasks focus on extracting LaTeX document setup logic, breaking down the main orchestration function, and ensuring comprehensive test coverage.

### Completed Work (Tasks 1-6)

| Task | Module | Lines Created | Lines Removed | Status |
|------|--------|--------------|---------------|--------|
| 1 | `report_config.py` | 217 | N/A | ✅ Complete |
| 2 | `data_transformers.py` | 219 | N/A | ✅ Complete |
| 3 | `map_utils.py` | 579 | N/A | ✅ Complete |
| 4 | `chart_builder.py` + `chart_saver.py` | 872 | 698 | ✅ Complete |
| 5 | `table_builders.py` | 593 | 368 | ✅ Complete |
| 6 | `report_sections.py` | 394 | 140 | ✅ Complete |

**Result:** `pdf_generator.py` reduced from 929 → 421 lines (55% reduction)

---

## Task 7: Extract document_builder.py

### Objective
Extract all LaTeX document initialization, package management, and preamble setup from `pdf_generator.py` into a dedicated `DocumentBuilder` class.

### Current State Analysis

**Location:** `pdf_generator.py` lines 100-225
**Lines to Extract:** ~125 lines
**Complexity:** Medium

The document setup code currently handles:
1. Document creation with geometry options
2. Package loading (13 packages)
3. Preamble configuration (headers, fonts, column spacing)
4. Font registration (Atkinson Hyperlegible sans & mono)
5. Two-column layout initialization
6. Title/header setup

### Implementation Specification

#### 7.1 Create `document_builder.py` Module

**File:** `/Users/david/code/velocity.report/internal/report/query_data/document_builder.py`

**Required Classes:**

```python
class DocumentBuilder:
    """Builds PyLaTeX Document with custom configuration.

    Responsibilities:
    - Create Document with geometry settings
    - Add required packages
    - Configure preamble (fonts, headers, spacing)
    - Set up two-column layout with title
    - Provide clean API for document initialization
    """

    def __init__(self, config: Optional[Dict] = None):
        """Initialize with optional config override.

        Args:
            config: Optional dict to override PDF_CONFIG defaults
        """

    def create_document(self, page_numbers: bool = False) -> Document:
        """Create base Document with geometry."""

    def add_packages(self, doc: Document) -> None:
        """Add all required LaTeX packages."""

    def setup_preamble(self, doc: Document) -> None:
        """Configure document preamble (fonts, formatting)."""

    def setup_fonts(self, doc: Document, fonts_path: str) -> None:
        """Register Atkinson Hyperlegible fonts."""

    def setup_header(
        self,
        doc: Document,
        start_iso: str,
        end_iso: str,
        location: str
    ) -> None:
        """Configure page header with fancyhdr."""

    def begin_twocolumn_layout(
        self,
        doc: Document,
        location: str,
        surveyor: str,
        contact: str
    ) -> None:
        """Start two-column layout with spanning title."""

    def build(
        self,
        start_iso: str,
        end_iso: str,
        location: str,
        surveyor: Optional[str] = None,
        contact: Optional[str] = None,
    ) -> Document:
        """Build complete configured document (convenience method).

        This is the main entry point that orchestrates all setup steps.
        """
```

#### 7.2 Key Design Decisions

**Configuration Management:**
- Use `PDF_CONFIG` from `report_config.py` as default
- Allow config override in `__init__` for testing
- Validate font paths exist before registration

**Font Handling:**
- Auto-detect fonts directory relative to module
- Graceful fallback if Atkinson Mono not found
- Support variable font weight configuration

**Error Handling:**
- Wrap font registration in try/except
- Provide informative warnings if fonts missing
- Ensure document can still generate with fallbacks

**Testability:**
- Each method should be independently testable
- Mock PyLaTeX Document in tests
- Verify package/preamble additions

#### 7.3 Extraction Steps

1. **Create `document_builder.py`** with class structure
2. **Move package list** from `pdf_generator.py:125-147`
3. **Move preamble setup** from `pdf_generator.py:149-167`
4. **Move font configuration** from `pdf_generator.py:169-225`
5. **Move header setup** from `pdf_generator.py:147-158`
6. **Move two-column init** from `pdf_generator.py:226-240`
7. **Update `pdf_generator.py`** to use `DocumentBuilder`
8. **Create tests** (see Task 9)

#### 7.4 Usage in pdf_generator.py

**Before:**
```python
def generate_pdf_report(...):
    doc = Document(geometry_options=geometry_options, page_numbers=False)
    doc.packages.append(Package("fancyhdr"))
    # ... 100+ lines of setup ...
```

**After:**
```python
from document_builder import DocumentBuilder

def generate_pdf_report(...):
    builder = DocumentBuilder()
    doc = builder.build(start_iso, end_iso, location,
                        SITE_INFO['surveyor'], SITE_INFO['contact'])

    # Now ready to add content sections...
```

#### 7.5 Expected Metrics

- **Lines Created:** ~200 (document_builder.py)
- **Lines Removed:** ~125 (pdf_generator.py)
- **Final pdf_generator.py size:** ~296 lines (30% reduction)
- **Test Coverage:** 12-15 unit tests

---

## Task 8: Refactor get_stats.py::main()

### Objective
Break down the monolithic `main()` function (190+ lines) into smaller, single-responsibility functions that are easier to test, understand, and maintain.

### Current State Analysis

**Location:** `get_stats.py` lines 90-434
**Lines in main():** ~344 lines
**Complexity:** High
**Cyclomatic Complexity:** ~15 (too high)

The current `main()` handles:
1. Argument validation
2. Multiple API calls (granular, overall, daily)
3. ISO timestamp generation
4. Chart generation (histogram + time-series)
5. PDF assembly
6. Error handling for each step

### Implementation Specification

#### 8.1 Function Decomposition

**Proposed Structure:**

```python
# === Configuration & Validation ===

def validate_args(args: argparse.Namespace) -> None:
    """Validate command-line arguments.

    Raises:
        ValueError: If arguments are invalid/incompatible
    """

def resolve_file_prefix(
    args: argparse.Namespace,
    start_ts: int,
    end_ts: int
) -> str:
    """Determine output file prefix (sequenced or date-based)."""

def compute_iso_timestamps(
    start_ts: int,
    end_ts: int,
    timezone: Optional[str]
) -> Tuple[str, str]:
    """Convert unix timestamps to ISO strings with timezone."""


# === API Data Fetching ===

def fetch_granular_metrics(
    client: RadarStatsClient,
    start_ts: int,
    end_ts: int,
    args: argparse.Namespace,
    model_version: Optional[str]
) -> Tuple[List[Dict], Optional[Dict], Dict]:
    """Fetch main granular metrics and optional histogram.

    Returns:
        Tuple of (metrics, histogram, response_metadata)
    """

def fetch_overall_summary(
    client: RadarStatsClient,
    start_ts: int,
    end_ts: int,
    args: argparse.Namespace,
    model_version: Optional[str]
) -> List[Dict]:
    """Fetch overall 'all' group summary."""

def fetch_daily_summary(
    client: RadarStatsClient,
    start_ts: int,
    end_ts: int,
    args: argparse.Namespace,
    model_version: Optional[str]
) -> Optional[List[Dict]]:
    """Fetch daily (24h) summary if appropriate for group size.

    Returns None if not needed (e.g., group already >= 24h).
    """


# === Chart Generation ===

def generate_histogram_chart(
    histogram: Dict[str, int],
    prefix: str,
    units: str,
    hist_max: Optional[float]
) -> bool:
    """Generate histogram chart PDF.

    Returns:
        True if chart was created successfully
    """

def generate_timeseries_chart(
    metrics: List[Dict],
    prefix: str,
    title: str,
    units: str,
    tz_name: Optional[str]
) -> bool:
    """Generate time-series chart PDF.

    Returns:
        True if chart was created successfully
    """


# === PDF Assembly ===

def assemble_pdf_report(
    prefix: str,
    start_iso: str,
    end_iso: str,
    overall_metrics: List[Dict],
    daily_metrics: Optional[List[Dict]],
    granular_metrics: List[Dict],
    histogram: Optional[Dict],
    args: argparse.Namespace
) -> str:
    """Assemble complete PDF report.

    Returns:
        Path to generated PDF file
    """


# === Date Range Processing ===

def process_date_range(
    start_date: str,
    end_date: str,
    args: argparse.Namespace,
    client: RadarStatsClient
) -> None:
    """Process a single date range: fetch data, generate charts, create PDF.

    This orchestrates all steps for one date range.
    """


# === Main Entry Point ===

def main(date_ranges: List[Tuple[str, str]], args: argparse.Namespace) -> None:
    """Main orchestrator: iterate over date ranges.

    Simplified to just client creation and iteration.
    """
```

#### 8.2 Refactoring Steps

1. **Create helper functions** (start with pure functions first):
   - `compute_iso_timestamps()`
   - `resolve_file_prefix()`

2. **Extract API fetch functions**:
   - `fetch_granular_metrics()`
   - `fetch_overall_summary()`
   - `fetch_daily_summary()`

3. **Extract chart generation**:
   - `generate_histogram_chart()`
   - `generate_timeseries_chart()`

4. **Extract PDF assembly**:
   - `assemble_pdf_report()`

5. **Create orchestrator**:
   - `process_date_range()` - combines all steps

6. **Simplify main()**:
   - Reduce to client creation + loop over `process_date_range()`

#### 8.3 Error Handling Strategy

**Current State:** Try/except blocks scattered throughout main()

**Improved Approach:**
- Each fetch function handles its own errors
- Return `None` or empty list on failure
- Log errors with context
- Allow `process_date_range()` to continue on partial failures
- Only fatal errors (CLI arg validation) should raise

**Example:**
```python
def fetch_overall_summary(...) -> List[Dict]:
    try:
        metrics, _, _ = client.get_stats(...)
        return metrics
    except Exception as e:
        print(f"Failed to fetch overall summary: {e}")
        return []  # Allow PDF generation to continue
```

#### 8.4 Testing Benefits

**Current:** `main()` is untestable (too large, too many side effects)

**After Refactoring:**
- Each function can be unit tested independently
- Mock API client for fetch functions
- Mock file I/O for chart generation
- Test orchestration logic separately
- Integration tests for `process_date_range()`

#### 8.5 Expected Metrics

- **Number of new functions:** 10-12
- **Average function size:** 15-30 lines
- **Main() size:** ~30 lines (90% reduction)
- **Cyclomatic complexity:** <5 per function
- **Test Coverage:** 20-25 new unit tests

---

## Task 9: Add Comprehensive Unit Tests

### Objective
Achieve >80% code coverage across all new modules with comprehensive, maintainable unit tests.

### Current Test Coverage Status

| Module | Test File | Tests | Coverage Est. | Status |
|--------|-----------|-------|---------------|--------|
| `report_config.py` | None | 0 | 0% | ❌ Missing |
| `data_transformers.py` | `test_data_transformers.py` | ? | ~60% | ⚠️ Partial |
| `map_utils.py` | `test_map_utils.py` | ? | ~50% | ⚠️ Partial |
| `chart_builder.py` | None | 0 | 0% | ❌ Missing |
| `chart_saver.py` | `test_chart_saver.py` | 15 | ~85% | ✅ Good |
| `table_builders.py` | `test_table_builders.py` | 23 | ~75% | ✅ Good |
| `report_sections.py` | `test_report_sections.py` | 17 | ~80% | ✅ Good |
| `document_builder.py` | None | 0 | 0% | 🔜 Task 7 |
| `get_stats.py` | None | 0 | 0% | 🔜 Task 8 |

### Implementation Specification

#### 9.1 Test report_config.py

**File:** `test_report_config.py`

**Test Categories:**

```python
class TestPDFConfig(unittest.TestCase):
    """Tests for PDF_CONFIG dictionary."""

    def test_has_required_keys(self):
        """Verify all required PDF config keys exist."""
        required = ['geometry', 'headheight', 'headsep', 'columnsep', 'fonts_dir']
        # ...

    def test_geometry_format(self):
        """Verify geometry is a valid dict."""

    def test_fonts_dir_valid(self):
        """Verify fonts directory path is reasonable."""


class TestMapConfig(unittest.TestCase):
    """Tests for MAP_CONFIG dictionary."""

    def test_has_marker_config(self):
        """Verify marker configuration exists."""

    def test_circle_radius_positive(self):
        """Verify circle radius is positive number."""

    def test_triangle_len_valid(self):
        """Verify triangle length is valid."""


class TestSiteInfo(unittest.TestCase):
    """Tests for SITE_INFO dictionary."""

    def test_has_required_fields(self):
        """Verify required site info fields exist."""
        required = ['location', 'surveyor', 'contact',
                   'site_description', 'speed_limit_note']
        # ...

    def test_contact_email_format(self):
        """Verify contact is valid email format."""


class TestColorConfig(unittest.TestCase):
    """Tests for COLORS configuration."""

    def test_has_line_colors(self):
        """Verify chart line colors exist."""

    def test_colors_are_hex(self):
        """Verify colors are valid hex strings."""


class TestLayoutConfig(unittest.TestCase):
    """Tests for LAYOUT configuration."""

    def test_has_chart_dimensions(self):
        """Verify chart size configuration."""

    def test_dimensions_positive(self):
        """Verify dimensions are positive numbers."""
```

**Test Count:** 15-18 tests
**Coverage Target:** 95% (config is mostly data)

#### 9.2 Test chart_builder.py

**File:** `test_chart_builder.py`

**Test Categories:**

```python
class TestTimeSeriesChartBuilder(unittest.TestCase):
    """Tests for TimeSeriesChartBuilder class."""

    def setUp(self):
        self.builder = TimeSeriesChartBuilder()
        self.sample_metrics = [...]

    def test_initialization(self):
        """Test builder initialization."""

    def test_build_creates_figure(self):
        """Test build() returns matplotlib Figure."""

    def test_extract_data_from_metrics(self):
        """Test data extraction from metrics."""

    def test_create_mask_low_counts(self):
        """Test masking for low sample counts."""

    def test_plot_with_gaps(self):
        """Test broken-line plotting with gaps."""

    def test_plot_without_gaps(self):
        """Test continuous plotting."""

    def test_add_count_bars(self):
        """Test count bars on secondary axis."""

    def test_legend_creation(self):
        """Test legend with all elements."""

    def test_empty_metrics_handling(self):
        """Test handling of empty metrics list."""

    def test_missing_fields_handling(self):
        """Test handling of metrics with missing fields."""

    def test_timezone_formatting(self):
        """Test time axis with timezone."""


class TestHistogramChartBuilder(unittest.TestCase):
    """Tests for HistogramChartBuilder class."""

    def setUp(self):
        self.builder = HistogramChartBuilder()
        self.sample_histogram = {...}

    def test_initialization(self):
        """Test builder initialization."""

    def test_build_creates_figure(self):
        """Test build() returns Figure."""

    def test_compute_bar_widths(self):
        """Test adaptive bar width calculation."""

    def test_plot_bars_centered(self):
        """Test bars are centered on bucket values."""

    def test_empty_histogram_handling(self):
        """Test handling of empty histogram."""

    def test_custom_cutoff(self):
        """Test custom cutoff value."""

    def test_custom_max_bucket(self):
        """Test custom max bucket."""


class TestConvenienceFunctions(unittest.TestCase):
    """Tests for module-level convenience functions."""

    # Tests for any helper functions...
```

**Test Count:** 20-25 tests
**Coverage Target:** 85%

#### 9.3 Test document_builder.py (Task 7)

**File:** `test_document_builder.py`

**Test Categories:**

```python
class TestDocumentBuilder(unittest.TestCase):
    """Tests for DocumentBuilder class."""

    @patch('document_builder.Document')
    def test_create_document(self, mock_doc_class):
        """Test document creation with geometry."""

    @patch('document_builder.Package')
    def test_add_packages(self, mock_package):
        """Test all required packages are added."""

    def test_setup_preamble(self):
        """Test preamble configuration."""

    @patch('os.path.exists')
    def test_setup_fonts_with_mono(self, mock_exists):
        """Test font setup when mono font exists."""

    @patch('os.path.exists')
    def test_setup_fonts_without_mono(self, mock_exists):
        """Test font setup fallback without mono."""

    def test_setup_header(self):
        """Test header/footer configuration."""

    def test_begin_twocolumn_layout(self):
        """Test two-column layout initialization."""

    def test_build_complete_document(self):
        """Test build() orchestrates all steps."""

    def test_custom_config_override(self):
        """Test config override in __init__."""

    def test_font_path_resolution(self):
        """Test fonts directory path resolution."""
```

**Test Count:** 12-15 tests
**Coverage Target:** 80%

#### 9.4 Test Refactored get_stats.py Functions (Task 8)

**File:** `test_get_stats.py`

**Test Categories:**

```python
class TestConfigValidation(unittest.TestCase):
    """Tests for argument validation functions."""

    def test_validate_args_valid(self):
        """Test validation passes for valid args."""

    def test_validate_args_invalid_group(self):
        """Test validation fails for invalid group."""


class TestFilePrefixResolution(unittest.TestCase):
    """Tests for file prefix generation."""

    def test_resolve_prefix_with_user_input(self):
        """Test sequenced prefix from user input."""

    def test_resolve_prefix_auto_generated(self):
        """Test auto-generated date-based prefix."""

    @patch('os.listdir')
    def test_next_sequenced_prefix(self, mock_listdir):
        """Test sequence number increment."""


class TestTimestampGeneration(unittest.TestCase):
    """Tests for ISO timestamp generation."""

    def test_compute_iso_timestamps_utc(self):
        """Test ISO generation with UTC."""

    def test_compute_iso_timestamps_pacific(self):
        """Test ISO generation with timezone."""


class TestAPIFetchFunctions(unittest.TestCase):
    """Tests for API data fetching."""

    @patch('get_stats.RadarStatsClient')
    def test_fetch_granular_metrics_success(self, mock_client):
        """Test successful granular metrics fetch."""

    @patch('get_stats.RadarStatsClient')
    def test_fetch_granular_metrics_failure(self, mock_client):
        """Test error handling in fetch."""

    def test_fetch_overall_summary(self):
        """Test overall summary fetch."""

    def test_fetch_daily_summary_when_needed(self):
        """Test daily fetch when group < 24h."""

    def test_fetch_daily_summary_skipped(self):
        """Test daily skipped when group >= 24h."""


class TestChartGenerationFunctions(unittest.TestCase):
    """Tests for chart generation."""

    @patch('get_stats.plot_histogram')
    @patch('get_stats.save_chart_as_pdf')
    def test_generate_histogram_chart(self, mock_save, mock_plot):
        """Test histogram chart generation."""

    @patch('get_stats.TimeSeriesChartBuilder')
    @patch('get_stats.save_chart_as_pdf')
    def test_generate_timeseries_chart(self, mock_save, mock_builder):
        """Test time-series chart generation."""


class TestPDFAssembly(unittest.TestCase):
    """Tests for PDF assembly."""

    @patch('get_stats.generate_pdf_report')
    def test_assemble_pdf_report(self, mock_generate):
        """Test PDF assembly orchestration."""


class TestDateRangeProcessing(unittest.TestCase):
    """Tests for date range processing orchestration."""

    @patch('get_stats.RadarStatsClient')
    def test_process_date_range_success(self, mock_client):
        """Test successful date range processing."""

    @patch('get_stats.RadarStatsClient')
    def test_process_date_range_api_failure(self, mock_client):
        """Test handling of API failures."""

    def test_process_date_range_invalid_dates(self):
        """Test handling of invalid date inputs."""


class TestMainOrchestrator(unittest.TestCase):
    """Tests for main() function."""

    @patch('get_stats.process_date_range')
    def test_main_single_range(self, mock_process):
        """Test main with single date range."""

    @patch('get_stats.process_date_range')
    def test_main_multiple_ranges(self, mock_process):
        """Test main with multiple date ranges."""
```

**Test Count:** 25-30 tests
**Coverage Target:** 75%

#### 9.5 Testing Best Practices

**Mock Strategy:**
- Mock external dependencies (API, file I/O, matplotlib)
- Use `@patch` decorator for clean mocking
- Verify mock calls with `assert_called_once_with()`

**Test Data:**
- Create reusable fixtures in `setUp()`
- Use realistic sample data
- Include edge cases (empty, None, malformed)

**Assertions:**
- Test both success and failure paths
- Verify return values and types
- Check side effects (file creation, API calls)

**Organization:**
- Group related tests in classes
- Use descriptive test names
- Add docstrings explaining what's tested

#### 9.6 Coverage Measurement

**Run coverage analysis:**
```bash
pytest --cov=internal/report/query_data \
       --cov-report=html \
       --cov-report=term-missing \
       internal/report/query_data/
```

**Target Metrics:**
- **Overall Coverage:** >80%
- **Critical Modules:** >85%
- **Configuration Modules:** >90%

---

## Task 10: Update Existing Tests

### Objective
Update existing test files to work with the new modular structure and verify no regressions were introduced.

### Current Test Inventory

```
test_api_client.py          - API client tests
test_data_transformers.py   - Data transformation tests
test_date_parser.py         - Date parsing tests
test_map_utils.py           - Map processing tests
test_pdf_generator.py       - PDF generation tests
test_stats_utils.py         - Stats utilities tests
test_chart_saver.py         - Chart saving tests (new, complete)
test_table_builders.py      - Table building tests (new, complete)
test_report_sections.py     - Section building tests (new, complete)
```

### Implementation Specification

#### 10.1 Update test_pdf_generator.py

**Current State:** Tests may reference old inline functions

**Required Updates:**

1. **Update imports:**
   ```python
   # Add new imports
   from document_builder import DocumentBuilder
   from report_sections import add_metric_data_intro, add_site_specifics, add_science
   ```

2. **Mock new dependencies:**
   ```python
   @patch('pdf_generator.DocumentBuilder')
   @patch('pdf_generator.add_metric_data_intro')
   @patch('pdf_generator.add_site_specifics')
   @patch('pdf_generator.add_science')
   def test_generate_pdf_report(self, mock_science, mock_site, mock_intro, mock_builder):
       # Test orchestration...
   ```

3. **Update test expectations:**
   - Verify `DocumentBuilder.build()` is called
   - Verify section functions are called with correct args
   - Verify document structure

4. **Add integration tests:**
   - Test full PDF generation end-to-end
   - Verify actual PDF file created
   - Check PDF contains expected content

**Estimated Changes:** 15-20 test updates

#### 10.2 Update test_data_transformers.py

**Current State:** May be incomplete

**Required Updates:**

1. **Add missing tests for:**
   - `extract_start_time_from_row()`
   - Edge cases for `MetricsNormalizer`
   - Field alias resolution
   - Type conversion edge cases

2. **Test error handling:**
   - Missing fields
   - Wrong types
   - Null values

**Estimated Changes:** 5-10 new tests

#### 10.3 Update test_map_utils.py

**Current State:** May have incomplete coverage

**Required Updates:**

1. **Add tests for:**
   - GPS coordinate handling
   - SVG manipulation edge cases
   - PDF conversion failures
   - Marker creation from config

2. **Mock file I/O:**
   - Mock `os.path.exists()`
   - Mock file read/write
   - Test temporary file cleanup

**Estimated Changes:** 8-12 test updates/additions

#### 10.4 Update test_stats_utils.py

**Current State:** May reference moved functions

**Required Updates:**

1. **Remove tests for moved functions:**
   - `plot_histogram()` → now in `chart_builder.py`
   - `save_chart_as_pdf()` → now in `chart_saver.py`

2. **Keep tests for:**
   - `format_time()`
   - `format_number()`
   - `process_histogram()`
   - `count_in_histogram_range()`
   - `count_histogram_ge()`
   - `chart_exists()`

3. **Add missing tests if any**

**Estimated Changes:** 5-8 test removals/updates

#### 10.5 Integration Test Suite

**Create:** `test_integration.py`

**Purpose:** Test complete workflows end-to-end

**Test Categories:**

```python
class TestFullReportGeneration(unittest.TestCase):
    """Integration tests for complete report generation."""

    @patch('get_stats.RadarStatsClient')
    def test_generate_report_from_api_data(self, mock_client):
        """Test generating PDF from real-looking API data."""
        # Mock API responses
        # Call main orchestrator
        # Verify PDF created
        # Verify PDF structure

    def test_generate_report_with_all_features(self):
        """Test report with all features enabled.

        - Histogram
        - Time-series chart
        - Map
        - Daily + granular tables
        """

    def test_generate_report_minimal(self):
        """Test minimal report (no histogram, no daily)."""

    def test_chart_to_pdf_integration(self):
        """Test chart generation → PDF embedding flow."""

    def test_table_to_pdf_integration(self):
        """Test table generation → PDF embedding flow."""


class TestModuleInteractions(unittest.TestCase):
    """Test interactions between refactored modules."""

    def test_data_transformers_to_table_builders(self):
        """Test MetricsNormalizer used by table builders."""

    def test_config_to_document_builder(self):
        """Test config propagation to document builder."""

    def test_sections_to_document(self):
        """Test section builders appending to document."""
```

**Test Count:** 10-15 integration tests
**Runtime:** These may be slower (use `@pytest.mark.slow`)

#### 10.6 Test Execution Strategy

**Pre-commit Testing:**
```bash
# Fast unit tests only
pytest -v -m "not slow" internal/report/query_data/
```

**Full Test Suite:**
```bash
# All tests including integration
pytest -v internal/report/query_data/
```

**Coverage Report:**
```bash
pytest --cov=internal/report/query_data \
       --cov-report=html \
       --cov-report=term-missing \
       internal/report/query_data/
```

**Test Markers:**
```python
# In conftest.py
def pytest_configure(config):
    config.addinivalue_line("markers", "slow: marks tests as slow (deselect with '-m \"not slow\"')")
    config.addinivalue_line("markers", "integration: marks tests as integration tests")
```

---

## Implementation Timeline

### Phase 1: Document Builder (Task 7)
**Duration:** 2-3 hours
**Steps:**
1. Create `document_builder.py` skeleton (30 min)
2. Extract and refactor setup code (60 min)
3. Update `pdf_generator.py` (30 min)
4. Write unit tests (45 min)
5. Smoke test PDF generation (15 min)

### Phase 2: Refactor main() (Task 8)
**Duration:** 3-4 hours
**Steps:**
1. Create helper functions (pure functions first) (60 min)
2. Extract API fetch functions (45 min)
3. Extract chart generation functions (30 min)
4. Create `process_date_range()` orchestrator (45 min)
5. Simplify `main()` (30 min)
6. Manual testing (30 min)

### Phase 3: Add Tests (Task 9)
**Duration:** 4-5 hours
**Steps:**
1. Test `report_config.py` (30 min)
2. Test `chart_builder.py` (90 min)
3. Test `document_builder.py` (60 min)
4. Test refactored `get_stats.py` functions (120 min)
5. Run coverage analysis (30 min)

### Phase 4: Update Existing Tests (Task 10)
**Duration:** 2-3 hours
**Steps:**
1. Update `test_pdf_generator.py` (45 min)
2. Update `test_data_transformers.py` (30 min)
3. Update `test_map_utils.py` (30 min)
4. Update `test_stats_utils.py` (30 min)
5. Create integration tests (45 min)

**Total Estimated Duration:** 11-15 hours

---

## Success Criteria

### Code Quality Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| pdf_generator.py size | <300 lines | `wc -l` |
| get_stats.py::main() size | <50 lines | Manual count |
| Average function size | <30 lines | Manual review |
| Cyclomatic complexity | <8 per function | `radon cc` |
| Test coverage (overall) | >80% | `pytest --cov` |
| Test coverage (critical) | >85% | `pytest --cov` |
| All tests passing | 100% | `pytest` |
| No linting errors | 0 | `flake8`/`pylint` |

### Functional Requirements

- ✅ All existing PDFs still generate correctly
- ✅ No visual regressions in generated reports
- ✅ Chart quality maintained
- ✅ Table formatting preserved
- ✅ Error handling improved (more specific errors)
- ✅ Logging more informative
- ✅ Documentation complete (docstrings, this plan)

### Maintainability Requirements

- ✅ Each module has single, clear responsibility
- ✅ Functions are independently testable
- ✅ Mocking is straightforward
- ✅ Configuration centralized
- ✅ No circular dependencies
- ✅ Clear import structure

---

## Testing Checklist

### Before Starting
- [ ] All Task 1-6 tests passing
- [ ] Current baseline coverage measured
- [ ] Smoke test: Generate PDF successfully

### After Task 7 (Document Builder)
- [ ] `test_document_builder.py` created
- [ ] 12-15 tests passing
- [ ] Smoke test: PDF still generates
- [ ] Visual inspection: PDF looks identical

### After Task 8 (Refactor main())
- [ ] `test_get_stats.py` created
- [ ] 25-30 tests passing
- [ ] All helper functions tested
- [ ] Orchestration logic tested
- [ ] Error handling tested

### After Task 9 (Add Tests)
- [ ] `test_report_config.py` created (15-18 tests)
- [ ] `test_chart_builder.py` created (20-25 tests)
- [ ] Coverage report generated
- [ ] >80% overall coverage achieved
- [ ] Critical modules >85% coverage

### After Task 10 (Update Tests)
- [ ] `test_pdf_generator.py` updated
- [ ] `test_data_transformers.py` updated
- [ ] `test_map_utils.py` updated
- [ ] `test_stats_utils.py` updated
- [ ] `test_integration.py` created (10-15 tests)
- [ ] All tests passing
- [ ] No skipped tests
- [ ] No warnings

### Final Validation
- [ ] Full test suite passing
- [ ] Coverage >80%
- [ ] Generate 5 different PDFs (different date ranges)
- [ ] Visual inspection: all features present
- [ ] Performance: no significant slowdown
- [ ] Memory: no leaks (generate 100 PDFs)
- [ ] Documentation: all docstrings complete
- [ ] Code review: ready for PR

---

## Risk Mitigation

### Risk 1: Breaking Changes in PDF Generation
**Likelihood:** Medium
**Impact:** High
**Mitigation:**
- Generate baseline PDFs before starting
- Visual diff tool for PDF comparison
- Keep original functions commented during transition
- Extensive smoke testing

### Risk 2: Test Coverage Gaps
**Likelihood:** Medium
**Impact:** Medium
**Mitigation:**
- Use coverage tool to identify gaps
- Prioritize critical path testing
- Add integration tests for full workflows
- Manual testing for visual elements

### Risk 3: Performance Regression
**Likelihood:** Low
**Impact:** Medium
**Mitigation:**
- Benchmark current performance
- Profile after refactoring
- Avoid unnecessary object creation
- Cache expensive computations

### Risk 4: Circular Dependencies
**Likelihood:** Low
**Impact:** High
**Mitigation:**
- Draw module dependency graph
- Keep imports at top level
- Avoid cross-imports between new modules
- Use dependency injection

---

## File Structure After Completion

```
internal/report/query_data/
├── Core Modules
│   ├── get_stats.py                 (~150 lines, refactored)
│   ├── api_client.py
│   ├── date_parser.py
│   └── stats_utils.py
│
├── Configuration
│   └── report_config.py             (217 lines)
│
├── Data Processing
│   └── data_transformers.py         (219 lines)
│
├── Chart Generation
│   ├── chart_builder.py             (729 lines)
│   └── chart_saver.py               (143 lines)
│
├── Table Generation
│   └── table_builders.py            (593 lines)
│
├── Document Assembly
│   ├── document_builder.py          (~200 lines, NEW)
│   ├── report_sections.py           (394 lines)
│   └── pdf_generator.py             (~250 lines, refactored)
│
├── Utilities
│   └── map_utils.py                 (579 lines)
│
└── Tests
    ├── test_get_stats.py            (NEW, 25-30 tests)
    ├── test_api_client.py
    ├── test_date_parser.py
    ├── test_stats_utils.py          (updated)
    ├── test_report_config.py        (NEW, 15-18 tests)
    ├── test_data_transformers.py    (updated)
    ├── test_chart_builder.py        (NEW, 20-25 tests)
    ├── test_chart_saver.py          (15 tests)
    ├── test_table_builders.py       (23 tests)
    ├── test_report_sections.py      (17 tests)
    ├── test_document_builder.py     (NEW, 12-15 tests)
    ├── test_map_utils.py            (updated)
    ├── test_pdf_generator.py        (updated)
    └── test_integration.py          (NEW, 10-15 tests)
```

**Total Test Count:** ~180-200 tests
**Total Test Code:** ~4,000-5,000 lines

---

## Appendix: Quick Reference Commands

### Run specific test file
```bash
pytest -v internal/report/query_data/test_document_builder.py
```

### Run tests with coverage
```bash
pytest --cov=internal/report/query_data \
       --cov-report=html \
       internal/report/query_data/test_document_builder.py
```

### Run only fast tests
```bash
pytest -v -m "not slow"
```

### Generate PDF smoke test
```bash
.venv/bin/python internal/report/query_data/get_stats.py \
    --file-prefix smoke-test \
    2025-01-13 2025-01-19
```

### Check line counts
```bash
wc -l internal/report/query_data/{pdf_generator.py,get_stats.py,document_builder.py}
```

### Check cyclomatic complexity
```bash
radon cc internal/report/query_data/get_stats.py -a
```

---

## Conclusion

This implementation plan provides a clear roadmap for completing the final 4 refactoring tasks. The key principles are:

1. **Incremental Progress:** Each task builds on the previous one
2. **Test-Driven:** Write tests as you extract code
3. **Verify Often:** Smoke test after each major change
4. **Document Thoroughly:** Update this plan as you discover issues
5. **Measure Success:** Use metrics to validate improvements

The estimated 11-15 hours assumes familiarity with the codebase and no major unexpected issues. The result will be a highly maintainable, well-tested codebase with clear separation of concerns.

**Next Step:** Begin Task 7 (Extract document_builder.py)
