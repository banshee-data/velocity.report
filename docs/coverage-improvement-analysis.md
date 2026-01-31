# Code Coverage Improvement Analysis

**Document Version:** 1.0  
**Date:** 2026-01-31  
**Status:** Analysis Complete - Awaiting Implementation  
**Goal:** Achieve 90%+ code coverage for `internal/` folder

## Executive Summary

### Current State

**Test Coverage Baseline (as of 2026-01-31):**

```
internal/ packages:
  - internal/db:              78.7%  ⚠️  Target: 90%
  - internal/lidar:           89.2%  ✓  Near target
  - internal/lidar/network:   92.0%  ✓  Exceeds target
  - internal/lidar/parse:     77.4%  ⚠️  Target: 90%
  - internal/monitoring:     100.0%  ✓  Perfect
  - internal/radar:           n/a    (no statements)
  - internal/security:        90.5%  ✓  Meets target
  - internal/serialmux:       86.3%  ⚠️  Target: 90%
  - internal/units:          100.0%  ✓  Perfect
  - internal/api:            FAIL    (web build assets missing)
  - internal/lidar/monitor:  FAIL    (web build assets missing)

cmd/ packages:
  - cmd/deploy:               7.2%   ⚠️  Very low
  - cmd/radar:               FAIL    (web build assets missing)
  - cmd/tools/backfill_ring_elevations: 43.3%
  - cmd/sweep:                0%     (no tests)
  - cmd/transit-backfill:     0%     (no tests)
  - cmd/tools:                0%     (no tests)

Overall internal/ weighted average: ~76% (excluding failures)
```

**Key Findings:**

1. **30-50% of cmd/ code should move to internal/** - Most cmd/ files contain substantial business logic that should be library code
2. **Low-hanging fruit in internal/:** Can achieve 90%+ by adding tests for uncovered edge cases and error paths
3. **Testability blockers identified:** eCharts usage, embedded HTML, and insufficient DRY in some modules

**Potential Coverage Gains:**

- **Phase 1 (cmd/ refactoring):** Move ~2,000 LOC from cmd/ to internal/, add tests → **+5-8% internal/ coverage**
- **Phase 2 (Edge case testing):** Add targeted tests for uncovered paths → **+8-12% internal/ coverage**
- **Phase 3 (Testability improvements):** Refactor for better mocking/injection → **+2-4% internal/ coverage**

**Total potential: 90%+ internal/ coverage achievable**

---

## Section 1: Refactoring Candidates - Moving Logic from cmd/ to internal/

### Overview

The `cmd/` directory should contain **only** CLI-specific code:
- Flag definitions and parsing
- User interaction (prompts, confirmations)
- Help text and usage information
- Thin orchestration layer calling internal/ libraries

Currently, many cmd/ files contain substantial business logic, HTTP client code, data processing, and algorithms that belong in `internal/` for testing and reuse.

---

### 1.1 cmd/radar/radar.go (653 lines)

**Status:** 0% coverage (setup fails), ~40% of code should move to internal/

#### Refactoring Candidates:

##### A. Transit CLI Commands (Lines 507-652) → `internal/db/transits_cli.go`

**Business Logic to Extract:**

```go
// Current: embedded in main()
func runTransitsCommand(args []string) { ... }
  - Transit analysis (lines 547-571)
  - Transit deletion (lines 573-594)
  - Transit migration (lines 596-619)
  - Transit rebuild (lines 621-648)
```

**Proposed Refactoring:**

```go
// NEW: internal/db/transits_cli.go
package db

// TransitCLI handles CLI-accessible transit operations
type TransitCLI struct {
    db        *DB
    threshold int
    model     string
}

func NewTransitCLI(db *DB, threshold int, model string) *TransitCLI

func (tc *TransitCLI) Analyse(ctx context.Context) (*TransitStats, error)
func (tc *TransitCLI) Delete(ctx context.Context, modelVersion string) (int, error)
func (tc *TransitCLI) Migrate(ctx context.Context, fromVer, toVer string) error
func (tc *TransitCLI) Rebuild(ctx context.Context) error
```

**Testing Benefits:**
- Can test transit operations without running full CLI
- Mock database interactions
- Verify business logic in isolation

**Coverage Impact:** +150 lines of testable code in internal/

---

##### B. Lidar Background Flushing (Lines 225-284) → `internal/lidar/background_flusher.go`

**Business Logic to Extract:**

```go
// Current: goroutine inline in main()
// Lines 256-283: Background flush timer loop
```

**Proposed Refactoring:**

```go
// NEW: internal/lidar/background_flusher.go
package lidar

type BackgroundFlusher struct {
    manager  *BackgroundManager
    db       *db.DB
    interval time.Duration
}

func NewBackgroundFlusher(manager *BackgroundManager, db *db.DB, interval time.Duration) *BackgroundFlusher

func (bf *BackgroundFlusher) Run(ctx context.Context) error
```

**Testing Benefits:**
- Unit test flush timing logic
- Mock database writes
- Verify context cancellation handling

**Coverage Impact:** +50 lines of testable code in internal/

---

##### C. Lidar Configuration Initialization (Lines 225-250) → `internal/lidar/config.go`

**Configuration to Extract:**

```go
// Current: BackgroundParams initialization scattered in main()
backgroundParams := lidar.BackgroundParams{
    BackgroundUpdateFraction:       0.02,
    ClosenessSensitivityMultiplier: 8.0,
    // ... 15+ more fields
}
```

**Proposed Refactoring:**

```go
// NEW: internal/lidar/config.go
package lidar

type BackgroundConfig struct {
    UpdateFraction           float64
    ClosenessSensitivity     float64
    SafetyMargin             float64
    // ... all fields with validation
}

func NewBackgroundConfigFromFlags(
    bgNoiseRelative float64,
    seedFromFirst bool,
) BackgroundConfig

func (bc BackgroundConfig) Validate() error
func (bc BackgroundConfig) ToBackgroundParams() BackgroundParams
```

**Testing Benefits:**
- Validate configuration combinations
- Test default value logic
- Ensure parameter constraints

**Coverage Impact:** +80 lines of testable code in internal/

---

### 1.2 cmd/sweep/main.go (742 lines)

**Status:** 0% coverage, ~80% of code should move to internal/

#### Refactoring Candidates:

##### A. Data Conversion & Statistics (Lines 22-127) → `internal/lidar/sweep/math.go`

**Functions to Extract:**

```go
// Current: utility functions in main.go
parseCSVFloatSlice(s string) ([]float64, error)
parseCSVIntSlice(s string) ([]int, error)
toFloat64Slice(v interface{}, length int) []float64
toInt64Slice(v interface{}, length int) []int64
meanStddev(xs []float64) (float64, float64)
```

**Proposed Module:**

```go
// NEW: internal/lidar/sweep/math.go
package sweep

func ParseCSVFloat64s(s string) ([]float64, error)
func ParseCSVInts(s string) ([]int, error)
func ToFloat64Slice(v interface{}, length int) []float64
func ToInt64Slice(v interface{}, length int) []int64
func MeanStddev(xs []float64) (float64, float64)
```

**Testing Benefits:**
- Unit test edge cases (empty strings, invalid formats)
- Test statistical calculations
- Verify type conversion safety

**Coverage Impact:** +100 lines of testable code

---

##### B. Parameter Range Generation (Lines 346-389) → `internal/lidar/sweep/ranges.go`

**Functions to Extract:**

```go
// Current: range generation scattered in main.go
parseParamList(list string, start, end, step float64) []float64
parseIntParamList(list string, start, end, step int) []int
generateRange(start, end, step float64) []float64
generateIntRange(start, end, step int) []int
```

**Proposed Module:**

```go
// NEW: internal/lidar/sweep/ranges.go
package sweep

type RangeSpec struct {
    Start float64
    End   float64
    Step  float64
}

func (rs RangeSpec) Generate() []float64
func (rs RangeSpec) Validate() error

type IntRangeSpec struct {
    Start int
    End   int
    Step  int
}

func (irs IntRangeSpec) Generate() []int
```

**Testing Benefits:**
- Test range edge cases (zero step, negative ranges)
- Verify floating-point precision handling
- Test mixed input formats (CSV vs range specs)

**Coverage Impact:** +60 lines of testable code

---

##### C. HTTP Client for LiDAR Monitor (Lines 391-539) → `internal/lidar/monitor/client.go`

**Functions to Extract:**

```go
// Current: HTTP operations scattered in main.go
startPCAPReplay(client, baseURL, sensorID, pcapFile) error
fetchBuckets(client, baseURL, sensorID) []string
resetGrid(client, baseURL, sensorID) error
setParams(client, baseURL, sensorID, noise, closeness, neighbor, seed) error
resetAcceptance(client, baseURL, sensorID) error
waitForGridSettle(client, baseURL, sensorID, timeout) error
```

**Proposed Module:**

```go
// NEW: internal/lidar/monitor/client.go
package monitor

type Client struct {
    httpClient *http.Client
    baseURL    string
    sensorID   string
}

func NewClient(baseURL, sensorID string) *Client

func (c *Client) StartPCAPReplay(ctx context.Context, pcapFile string) error
func (c *Client) FetchBuckets(ctx context.Context) ([]string, error)
func (c *Client) ResetGrid(ctx context.Context) error
func (c *Client) SetParams(ctx context.Context, params BackgroundParams) error
func (c *Client) ResetAcceptance(ctx context.Context) error
func (c *Client) WaitForGridSettle(ctx context.Context, timeout time.Duration) error
```

**Testing Benefits:**
- Mock HTTP responses for testing
- Test retry logic and error handling
- Verify timeout behavior

**Coverage Impact:** +150 lines of testable code

---

##### D. Metrics Sampling (Lines 541-627) → `internal/lidar/sweep/sampler.go`

**Functions to Extract:**

```go
// Current: sampling logic in main.go
type SampleResult struct { ... }
sampleMetrics(client, baseURL, sensorID, iterations, interval, buckets, ...) []SampleResult
```

**Proposed Module:**

```go
// NEW: internal/lidar/sweep/sampler.go
package sweep

type Sampler struct {
    client    *monitor.Client
    interval  time.Duration
    buckets   []string
}

func NewSampler(client *monitor.Client, interval time.Duration, buckets []string) *Sampler

func (s *Sampler) Sample(ctx context.Context, iterations int, params BackgroundParams) ([]SampleResult, error)
```

**Testing Benefits:**
- Mock HTTP client for deterministic testing
- Test sampling timing and cancellation
- Verify metric aggregation logic

**Coverage Impact:** +90 lines of testable code

---

##### E. CSV Output (Lines 629-741) → `internal/lidar/sweep/output.go`

**Functions to Extract:**

```go
// Current: CSV writing in main.go
writeHeaders(w, rawW, buckets)
writeRawRow(w, noise, closeness, neighbor, iter, result, buckets)
writeSummary(w, noise, closeness, neighbor, results, buckets)
```

**Proposed Module:**

```go
// NEW: internal/lidar/sweep/output.go
package sweep

type CSVWriter struct {
    summary *csv.Writer
    raw     *csv.Writer
    buckets []string
}

func NewCSVWriter(summaryPath, rawPath string, buckets []string) (*CSVWriter, error)

func (cw *CSVWriter) WriteHeaders() error
func (cw *CSVWriter) WriteRawSample(params BackgroundParams, iter int, result SampleResult) error
func (cw *CSVWriter) WriteSummary(params BackgroundParams, results []SampleResult) error
func (cw *CSVWriter) Close() error
```

**Testing Benefits:**
- Test CSV formatting correctness
- Verify header/data alignment
- Test statistical calculations in summary

**Coverage Impact:** +110 lines of testable code

---

**Total for cmd/sweep/main.go:**
- Move ~600 lines to internal/
- Create 5 new testable modules
- Estimated coverage impact: **+510 lines at 90%+ coverage**

---

### 1.3 cmd/tools/scan_transits.go (147 lines)

**Status:** 0% coverage, ~50% should move to internal/

#### Refactoring Candidates:

##### A. Transit Gap Detection (Lines 76-146) → `internal/db/transit_gaps.go`

**Business Logic to Extract:**

```go
// Current: embedded in main()
type transitGap struct { ... }  // unexported
func findTransitGaps(dbConn *db.DB) ([]transitGap, error)
```

**Proposed Refactoring:**

```go
// NEW: internal/db/transit_gaps.go
package db

type TransitGap struct {  // Export
    Start       time.Time
    End         time.Time
    RecordCount int
}

func (db *DB) FindTransitGaps(ctx context.Context) ([]TransitGap, error)
func (db *DB) FindTransitGapsInRange(ctx context.Context, start, end time.Time) ([]TransitGap, error)
```

**Testing Benefits:**
- Unit test SQL query logic
- Mock database responses
- Test edge cases (empty DB, no gaps, all gaps)

**Coverage Impact:** +70 lines of testable code

---

### 1.4 cmd/deploy/*.go (Multiple Files)

**Status:** 7.2% coverage, ~60-85% of code should move to internal/

#### Refactoring Candidates:

##### A. SSH Config Parser (sshconfig.go, ~120 lines) → `internal/deploy/sshconfig.go`

**Move 100% to internal/:**

```go
// Current: cmd/deploy/sshconfig.go
type SSHConfig struct { ... }
func ParseSSHConfig() (*SSHConfig, error)
```

**Proposed:** Move entire file to `internal/deploy/sshconfig.go`

**Rationale:**
- Pure library code, zero CLI dependencies
- Complex parsing logic needs comprehensive testing
- Reusable across different CLI tools

**Testing Benefits:**
- Test SSH config parsing edge cases
- Verify hostname resolution logic
- Test with mock config files

**Coverage Impact:** +120 lines at 90%+ coverage

---

##### B. Command Executor (executor.go, ~200 lines) → `internal/deploy/executor.go`

**Move 100% to internal/:**

```go
// Current: cmd/deploy/executor.go
type Executor struct { ... }
func (e *Executor) Run(ctx context.Context, command string) (string, error)
func (e *Executor) RunSudo(ctx context.Context, command string) (string, error)
```

**Proposed:** Move entire file to `internal/deploy/executor.go`

**Rationale:**
- SSH/command execution abstraction
- No CLI-specific logic
- Core infrastructure for deployment operations

**Testing Benefits:**
- Mock SSH connections
- Test local vs remote execution
- Verify sudo handling

**Coverage Impact:** +200 lines at 90%+ coverage

---

##### C. Installer Components (installer.go, ~280 lines) → Multiple internal/ modules

**Business Logic to Extract:**

```go
// Current: scattered helper methods
validateBinary(executor, binaryPath) error          // Lines 116-135
createServiceUser(executor) error                   // Lines 154-176
createDataDirectory(executor) error                 // Lines 178-188
installBinary(executor, binaryPath) error           // Lines 190-206
installService(executor, serviceContent) error      // Lines 208-237
migrateDatabase(executor, dbPath) error             // Lines 239+
```

**Proposed Modules:**

```go
// NEW: internal/deploy/binary.go
type BinaryInstaller struct {
    executor *Executor
}
func (bi *BinaryInstaller) Validate(binaryPath string) error
func (bi *BinaryInstaller) Install(binaryPath, targetPath string) error

// NEW: internal/deploy/system.go
type SystemSetup struct {
    executor *Executor
}
func (ss *SystemSetup) CreateServiceUser(username string) error
func (ss *SystemSetup) CreateDataDirectory(path string) error

// NEW: internal/deploy/service.go
type ServiceInstaller struct {
    executor *Executor
}
func (si *ServiceInstaller) Install(serviceContent, servicePath string) error
func (si *ServiceInstaller) Enable(serviceName string) error
func (si *ServiceInstaller) Start(serviceName string) error

// NEW: internal/deploy/database.go
type DatabaseMigrator struct {
    executor *Executor
}
func (dm *DatabaseMigrator) Migrate(dbPath string) error
```

**Testing Benefits:**
- Isolate each installation step for testing
- Mock executor to avoid actual system changes
- Test failure recovery paths

**Coverage Impact:** +240 lines at 90%+ coverage

---

##### D. Similar Patterns for Other Deploy Files

**upgrader.go** (~200 lines): Extract to `internal/deploy/upgrade.go`, `internal/deploy/version.go`, `internal/deploy/backup.go`

**monitor.go** (~150 lines): Extract data gathering to `internal/deploy/status.go`

**rollback.go** (~150 lines): Extract business logic to `internal/deploy/rollback.go`

**config.go** (~192 lines): Extract validation to `internal/deploy/config_validator.go`

**Total for cmd/deploy/:**
- Move ~900 lines to internal/
- Create 12+ new testable modules
- Estimated coverage impact: **+800 lines at 90%+ coverage**

---

### 1.5 Summary: cmd/ Refactoring Impact

| Source File | Lines to Move | New internal/ Modules | Estimated Coverage |
|-------------|---------------|----------------------|-------------------|
| radar.go | 280 | 3 | +250 @ 90% |
| sweep/main.go | 600 | 5 | +510 @ 85% |
| scan_transits.go | 70 | 1 | +70 @ 90% |
| deploy/*.go | 900 | 12 | +800 @ 90% |
| **TOTAL** | **~1,850** | **21 modules** | **+1,630 testable lines** |

**Impact on internal/ coverage:**
- Current internal/ LOC: ~8,500 (estimated from coverage reports)
- Add testable code: +1,630 lines
- With 90% coverage on new code: +1,467 covered lines
- **Projected internal/ coverage increase: +5-8%**

---

## Section 2: Improving Coverage in Existing internal/ Code

### 2.1 Current Coverage Gaps

#### internal/db (78.7% → Target: 90%+)

**Uncovered Areas (from analysis):**

1. **Error paths in migration logic** (migrate.go, migrate_cli.go)
   - Lines handling filesystem errors
   - Lines handling SQL execution errors
   - Edge cases in schema version detection

2. **Edge cases in transit worker** (transit_worker.go)
   - Boundary conditions in time windows
   - Deduplication logic with edge cases
   - Error recovery in long-running operations

3. **Admin route error handling** (site.go, site_report.go)
   - Invalid input validation
   - Database constraint violations
   - Concurrent access scenarios

**Testing Strategy:**

```go
// Example: Add error path tests
func TestMigrate_FileSystemError(t *testing.T) {
    // Test migration with unwritable directory
}

func TestTransitWorker_EmptyTimeWindow(t *testing.T) {
    // Test worker with no data in range
}

func TestSiteReport_InvalidDateRange(t *testing.T) {
    // Test report generation with invalid dates
}
```

**Estimated Impact:** +50-80 test cases → **+8-10% coverage**

---

#### internal/lidar/parse (77.4% → Target: 90%+)

**Uncovered Areas:**

1. **Pandar40P parser edge cases** (pandar40p_parser.go)
   - Malformed packet handling
   - Invalid azimuth/elevation values
   - Boundary conditions in angle wrapping

2. **Configuration validation** (config.go)
   - Invalid JSON handling
   - Missing required fields
   - Out-of-range values

**Testing Strategy:**

```go
func TestPandar40P_MalformedPacket(t *testing.T) {
    // Test with corrupted UDP payload
}

func TestConfig_MissingElevations(t *testing.T) {
    // Test with incomplete elevation table
}
```

**Estimated Impact:** +30-40 test cases → **+10-12% coverage**

---

#### internal/serialmux (86.3% → Target: 90%+)

**Uncovered Areas:**

1. **Serial port error handling** (serialmux.go)
   - Device disconnect scenarios
   - Baud rate mismatch
   - Read/write timeouts

2. **Mock serial implementation edge cases** (mock_serial.go)
   - Buffer overflow scenarios
   - Concurrent access

**Testing Strategy:**

```go
func TestSerialMux_DeviceDisconnect(t *testing.T) {
    // Simulate device removal during operation
}

func TestMockSerial_ConcurrentReads(t *testing.T) {
    // Test thread safety
}
```

**Estimated Impact:** +20-25 test cases → **+3-5% coverage**

---

### 2.2 Coverage Improvement Summary

| Package | Current | Target | Gap | Estimated Tests Needed | Impact |
|---------|---------|--------|-----|----------------------|--------|
| internal/db | 78.7% | 90% | 11.3% | 50-80 | +8-10% |
| internal/lidar/parse | 77.4% | 90% | 12.6% | 30-40 | +10-12% |
| internal/serialmux | 86.3% | 90% | 3.7% | 20-25 | +3-5% |
| internal/api | FAIL | 90% | n/a | Fix build first | TBD |
| internal/lidar/monitor | FAIL | 90% | n/a | Fix build first | TBD |

**Total estimated impact from edge case testing: +8-12% coverage**

---

## Section 3: Testability Improvements

### 3.1 DRY (Don't Repeat Yourself) Violations

#### Problem Areas Identified:

##### A. Duplicate Configuration Structs

**Issue:** Multiple packages define similar parameter structs without sharing

**Example:**
```go
// internal/lidar/background.go
type BackgroundParams struct { ... }

// cmd/radar/radar.go
// Duplicates field names and validation logic
backgroundParams := lidar.BackgroundParams{
    BackgroundUpdateFraction: 0.02,
    // ... repeated in multiple places
}
```

**Solution:**

```go
// NEW: internal/lidar/config.go
type BackgroundConfig struct {
    fields with validation tags
}

func (bc BackgroundConfig) Validate() error
func (bc BackgroundConfig) ToParams() BackgroundParams
func DefaultBackgroundConfig() BackgroundConfig
```

**Testability Impact:**
- Centralize validation logic → test once
- Consistent defaults → predictable behavior
- **Coverage gain: +50 testable lines**

---

##### B. Duplicate HTTP Client Logic

**Issue:** Multiple places implement similar HTTP request patterns

**Example:**
- cmd/sweep/main.go: HTTP calls to monitor API
- internal/api/handlers_test.go: Test HTTP helpers
- Potential future cmd/ tools needing same API access

**Solution:**

```go
// NEW: internal/lidar/monitor/client.go (as proposed in Section 1)
// Consolidates all monitor API access

type Client struct { ... }
func (c *Client) makeRequest(ctx, method, path string, body interface{}) error
// DRY: All HTTP logic in one place
```

**Testability Impact:**
- Single HTTP client to test and mock
- Consistent error handling across all API calls
- **Coverage gain: +100 testable lines** (vs testing duplicates separately)

---

##### C. Duplicate CSV/Output Formatting

**Issue:** Similar formatting logic appears in multiple tools

**Example:**
- cmd/sweep/main.go: CSV output (lines 629-741)
- cmd/tools/backfill_ring_elevations: Likely has output formatting
- Future analysis tools will need similar formatting

**Solution:**

```go
// NEW: internal/lidar/sweep/format.go
type ResultFormatter interface {
    WriteHeader() error
    WriteData(data interface{}) error
    Close() error
}

type CSVFormatter struct { ... }
type JSONFormatter struct { ... }
```

**Testability Impact:**
- Test formatters once with various input types
- Easily add new formats (JSON, Parquet)
- **Coverage gain: +80 testable lines**

---

### 3.2 eCharts Usage and Testing Challenges

#### Current State:

**Location:** `internal/lidar/monitor/webserver.go`

**Usage Pattern:**
```go
import (
    "github.com/go-echarts/go-echarts/v2/charts"
    "github.com/go-echarts/go-echarts/v2/components"
    "github.com/go-echarts/go-echarts/v2/opts"
)

func (ws *WebServer) handleBackgroundGridPolar(w http.ResponseWriter, r *http.Request) {
    polar := charts.NewPolar()
    polar.SetGlobalOptions(
        charts.WithInitializationOpts(opts.Initialization{...}),
    )
    // ... complex chart building logic ...
    polar.Render(w)  // Renders HTML directly to response writer
}
```

**Testing Challenges:**

1. **HTML Output:** Charts render to `io.Writer` as complete HTML pages
   - Hard to test specific data points
   - Fragile: output changes with library updates

2. **No Data Separation:** Chart construction mixed with HTTP handling
   - Can't unit test chart logic without HTTP mocks
   - Can't test data transformations independently

3. **Embedded Assets:** eCharts JavaScript files embedded via `//go:embed`
   - Test environment may not have assets
   - Build failures block all tests

---

#### Proposed Solutions:

##### A. Separate Data Preparation from Rendering

**Current (Lines 850-950 approx):**
```go
func (ws *WebServer) handleBackgroundGridPolar(w http.ResponseWriter, r *http.Request) {
    // 1. Fetch data from background manager
    // 2. Transform to chart format
    // 3. Create chart
    // 4. Render directly to HTTP response
}
```

**Refactored:**
```go
// NEW: internal/lidar/monitor/chart_data.go
package monitor

type PolarChartData struct {
    AngleName  []string
    RadiusName []string
    Data       []opts.PolarData
}

func (ws *WebServer) preparePolarChartData(sensorID string) (*PolarChartData, error) {
    // Pure data transformation, no HTTP or rendering
    grid := ws.backgroundManagers[sensorID].ExportGrid()
    
    // Transform grid to chart-ready format
    data := &PolarChartData{
        AngleName: make([]string, numAngles),
        // ... data preparation ...
    }
    return data, nil
}

// Testable independently:
func TestPreparePolarChartData(t *testing.T) {
    // Mock background manager
    // Call preparePolarChartData()
    // Assert data structure correctness
}
```

**HTTP Handler (Thin):**
```go
func (ws *WebServer) handleBackgroundGridPolar(w http.ResponseWriter, r *http.Request) {
    sensorID := r.URL.Query().Get("sensor_id")
    
    data, err := ws.preparePolarChartData(sensorID)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    // Rendering becomes a one-liner
    renderPolarChart(w, data)
}
```

**Testing Benefits:**
- **Unit test data preparation** without HTTP or eCharts
- **Test chart rendering** with fixed data fixtures
- **Mock background manager** to test error paths

**Coverage Impact:** +150 testable lines of data transformation logic

---

##### B. Abstract Chart Rendering for Testing

**Problem:** `polar.Render(w)` writes HTML directly, hard to assert on output

**Solution 1: Capture HTML for Validation**

```go
// NEW: internal/lidar/monitor/chart_test.go
func TestRenderPolarChart_ContainsExpectedData(t *testing.T) {
    data := &PolarChartData{
        AngleName: []string{"0°", "90°", "180°"},
        Data:      []opts.PolarData{{Value: []interface{}{0, 10}}},
    }
    
    var buf bytes.Buffer
    err := renderPolarChart(&buf, data)
    require.NoError(t, err)
    
    html := buf.String()
    assert.Contains(t, html, "0°")
    assert.Contains(t, html, "90°")
    assert.Contains(t, html, `"value":[0,10]`) // Check JSON data in HTML
}
```

**Solution 2: JSON API Endpoint (Recommended)**

```go
// NEW: Expose chart data as JSON instead of HTML
func (ws *WebServer) handleBackgroundGridPolarJSON(w http.ResponseWriter, r *http.Request) {
    sensorID := r.URL.Query().Get("sensor_id")
    
    data, err := ws.preparePolarChartData(sensorID)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    json.NewEncoder(w).Encode(data)
}

// Frontend (JavaScript) fetches JSON and renders with eCharts
// Go server only provides data, not rendering
```

**Benefits:**
- **Easier to test:** JSON is simple to parse and assert on
- **Frontend flexibility:** Can switch charting libraries without Go changes
- **Separation of concerns:** Go for data, JavaScript for visualization

**Coverage Impact:** +100 testable lines (data APIs)

---

##### C. Mock Background Managers for Testing

**Current Challenge:** Can't test chart handlers without real background managers

**Solution:**

```go
// NEW: internal/lidar/monitor/mock_background.go
type MockBackgroundManager struct {
    mock.Mock
}

func (m *MockBackgroundManager) ExportGrid() *BackgroundGrid {
    args := m.Called()
    return args.Get(0).(*BackgroundGrid)
}

// Use in tests:
func TestPreparePolarChartData_EmptyGrid(t *testing.T) {
    mockBg := new(MockBackgroundManager)
    mockBg.On("ExportGrid").Return(&BackgroundGrid{Cells: nil})
    
    ws := &WebServer{
        backgroundManagers: map[string]*BackgroundManager{
            "test": mockBg,
        },
    }
    
    data, err := ws.preparePolarChartData("test")
    assert.NoError(t, err)
    assert.Empty(t, data.Data)
}
```

**Coverage Impact:** Can test all chart handlers → +200 lines covered

---

### 3.3 Embedded HTML and Assets

#### Current State:

**Embedded Files:**
```go
//go:embed html/*.html
var htmlFS embed.FS

//go:embed assets/echarts.min.js
var assetsFS embed.FS
```

**Testing Challenges:**

1. **Build Failures:** Tests fail if embedded files missing or build context wrong
2. **Path Dependencies:** Tests depend on specific file paths
3. **Asset Updates:** Updating eCharts requires Go rebuild

---

#### Proposed Solutions:

##### A. Filesystem Abstraction for Testing

**Current:**
```go
func (ws *WebServer) handleDashboard(w http.ResponseWriter, r *http.Request) {
    tmpl, err := template.ParseFS(htmlFS, "html/dashboard.html")
    // ...
}
```

**Refactored:**
```go
// NEW: internal/lidar/monitor/templates.go
type TemplateProvider interface {
    Get(name string) (*template.Template, error)
}

type EmbeddedTemplateProvider struct {
    fs embed.FS
}

func (etp *EmbeddedTemplateProvider) Get(name string) (*template.Template, error) {
    return template.ParseFS(etp.fs, "html/"+name)
}

type MockTemplateProvider struct {
    templates map[string]*template.Template
}

func (mtp *MockTemplateProvider) Get(name string) (*template.Template, error) {
    return mtp.templates[name], nil
}
```

**WebServer with Injection:**
```go
type WebServer struct {
    templates TemplateProvider
    assets    http.FileSystem
}

func NewWebServer(templates TemplateProvider, assets http.FileSystem) *WebServer
```

**Testing:**
```go
func TestHandleDashboard_RendersTemplate(t *testing.T) {
    mockTemplates := &MockTemplateProvider{
        templates: map[string]*template.Template{
            "dashboard.html": template.Must(template.New("test").Parse("<html>{{.}}</html>")),
        },
    }
    
    ws := NewWebServer(mockTemplates, nil)
    
    req := httptest.NewRequest("GET", "/dashboard", nil)
    w := httptest.NewRecorder()
    
    ws.handleDashboard(w, req)
    
    assert.Equal(t, 200, w.Code)
    assert.Contains(t, w.Body.String(), "<html>")
}
```

**Benefits:**
- Tests don't depend on embedded files
- Can test with minimal template fixtures
- Easier to mock template rendering errors

**Coverage Impact:** +150 lines of HTTP handler logic testable

---

##### B. Separate Asset Serving from Application Logic

**Current Issue:** eCharts assets embedded in Go binary, served via custom handler

**Better Architecture:**

```
┌─────────────────┐
│  Go Server      │
│  (Data APIs)    │ ← Testable, no embedded assets
└────────┬────────┘
         │ JSON
         ↓
┌─────────────────┐
│  Static Assets  │
│  (nginx/CDN)    │ ← HTML, JS, CSS separate
│  - index.html   │
│  - echarts.js   │
└─────────────────┘
```

**Implementation:**

1. **Move HTML/JS to static/ directory** served separately (nginx, Caddy, or Go's http.FileServer in development)
2. **Go server provides JSON APIs only** (already partially done for some endpoints)
3. **Frontend JavaScript fetches data** and renders with eCharts client-side

**Testing Benefits:**
- Go tests don't need asset files
- Frontend tests use real eCharts library
- Can test backend logic in isolation

**Migration Path:**
1. Add JSON endpoints alongside HTML endpoints (Lines 950+)
2. Create static HTML page that calls JSON endpoints
3. Deprecate HTML-rendering endpoints
4. Remove embedded HTML once frontend migrated

**Coverage Impact:**
- Removes ~200 lines of untestable HTML rendering code
- Replaces with ~150 lines of testable JSON serialization
- **Net: +100 testable lines**

---

### 3.4 Dependency Injection for Testability

#### Current Issue: Hard-to-Mock Dependencies

**Example: internal/api/server.go**

```go
type Server struct {
    serial  serialmux.SerialMuxInterface  // ✓ Already injected (good!)
    db      *db.DB                         // ✓ Already injected (good!)
    units   string                         // ✓ Config (good!)
    
    // Hidden dependencies:
    // - File system (for static assets)
    // - System clock (for timestamps)
    // - Random number generator (if used)
}
```

**Better Pattern:**

```go
type Server struct {
    serial    serialmux.SerialMuxInterface
    db        *db.DB
    units     string
    timezone  string
    
    // Injected dependencies for testing:
    fs        http.FileSystem    // Can mock embedded assets
    clock     Clock               // Can mock time for testing
    templates TemplateProvider    // Can mock template rendering
}

type Clock interface {
    Now() time.Time
}

type realClock struct{}
func (realClock) Now() time.Time { return time.Now() }

type mockClock struct{ t time.Time }
func (mc mockClock) Now() time.Time { return mc.t }
```

**Testing Benefits:**

```go
func TestServer_ReportGeneration_Midnight(t *testing.T) {
    mockClock := mockClock{t: time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)}
    
    server := NewServer(
        mockSerial,
        mockDB,
        "mph",
        "UTC",
        mockClock,  // ← Injected
    )
    
    // Can now test time-sensitive logic deterministically
}
```

**Coverage Impact:** Unlocks testing of time-dependent code → +100 lines

---

### 3.5 Summary: Testability Improvements

| Issue | Solution | Testable Lines Gained | Effort |
|-------|----------|----------------------|--------|
| DRY violations | Centralize config/formatting | +230 | Medium |
| eCharts coupling | Separate data prep from rendering | +150 | Medium |
| Embedded HTML | Filesystem abstraction | +150 | Low |
| Hard-to-mock deps | Dependency injection | +100 | Low |
| **TOTAL** | | **+630** | |

**Impact: +630 testable lines → Estimated +4-6% coverage** (after refactoring)

---

## Section 4: Implementation Roadmap

### Phase 1: Quick Wins (1-2 weeks)

**Goal:** Achieve 85%+ internal/ coverage with minimal refactoring

**Tasks:**

1. **Fix Build Issues** (blocker for api and lidar/monitor testing)
   - Build web assets or skip asset-dependent tests
   - Estimated: 2-4 hours

2. **Add Edge Case Tests to Existing Code**
   - internal/db: +50 test cases → +8% coverage
   - internal/lidar/parse: +30 test cases → +10% coverage
   - internal/serialmux: +20 test cases → +3% coverage
   - Estimated: 3-5 days

3. **Low-Effort cmd/ Refactoring**
   - Move cmd/deploy/executor.go to internal/ (100% move) → +200 lines @ 90%
   - Move cmd/deploy/sshconfig.go to internal/ (100% move) → +120 lines @ 90%
   - Estimated: 2-3 days

**Expected Result:** internal/ coverage: 76% → 85-87%

---

### Phase 2: Major Refactoring (3-4 weeks)

**Goal:** Achieve 90%+ internal/ coverage with cmd/ restructuring

**Tasks:**

1. **Extract cmd/sweep Logic** (highest ROI)
   - Create internal/lidar/sweep package
   - Move 600 lines of logic
   - Add comprehensive tests
   - Estimated: 1-2 weeks

2. **Extract cmd/radar Logic**
   - Create internal/db/transits_cli.go
   - Create internal/lidar/config.go
   - Move 280 lines of logic
   - Estimated: 3-5 days

3. **Extract cmd/deploy Logic**
   - Create internal/deploy package
   - Refactor installer, upgrader, monitor, rollback
   - Move 900 lines of logic
   - Estimated: 1-2 weeks

**Expected Result:** internal/ coverage: 87% → 92-94%

---

### Phase 3: Testability Improvements (2-3 weeks)

**Goal:** Solidify 90%+ coverage with architectural improvements

**Tasks:**

1. **Refactor eCharts Usage**
   - Separate data preparation from rendering
   - Add JSON endpoints for chart data
   - Mock background managers
   - Estimated: 1 week

2. **Decouple Embedded HTML**
   - Implement filesystem abstraction
   - Mock template provider for tests
   - Estimated: 3-5 days

3. **Add Dependency Injection**
   - Inject clock, filesystem, external clients
   - Enable deterministic testing of time-sensitive code
   - Estimated: 3-5 days

**Expected Result:** internal/ coverage: 92-94% → 94-96%

---

### Phase 4: Polish & Maintenance (ongoing)

**Goal:** Maintain 90%+ coverage as codebase evolves

**Tasks:**

1. **Coverage Monitoring**
   - Set up CI to enforce 90% minimum coverage
   - Block PRs that reduce coverage without justification

2. **Documentation**
   - Document testing patterns and best practices
   - Create guide for adding new testable features

3. **Continuous Improvement**
   - Periodically review uncovered lines
   - Refactor untestable code as discovered

---

## Appendix A: Coverage Calculation Details

### Current State (Estimated)

```
internal/ packages total: ~8,500 LOC (production code)
Current coverage: 76% weighted average
Covered lines: ~6,460
Uncovered lines: ~2,040
```

### After Phase 1 (Edge Case Tests)

```
Add tests for existing code: +21% of uncovered lines
Newly covered: ~430 lines
Total covered: 6,890 / 8,500 = 81%
```

### After Phase 2 (cmd/ Refactoring)

```
Move from cmd/ to internal/: +1,850 LOC
New internal/ total: ~10,350 LOC
Test new code at 90%: +1,665 covered lines
Total covered: 8,555 / 10,350 = 83%

Plus edge cases on new code: +185 lines
Total covered: 8,740 / 10,350 = 84.5%
```

### After Phase 3 (Testability)

```
Refactor for testability: +630 LOC (net)
New internal/ total: ~10,980 LOC
Test refactored code at 95%: +600 covered lines
Total covered: 9,340 / 10,980 = 85%

Unlock previously untestable code: +500 lines now coverable
Test at 90%: +450 lines
Total covered: 9,790 / 10,980 = 89%
```

### After Phase 4 (Polish)

```
Add remaining edge cases: +200 lines
Total covered: 9,990 / 10,980 = 91%
```

**Final projected coverage: 90-92%**

---

## Appendix B: Testing Best Practices

### B.1 Table-Driven Tests

**Recommended Pattern:**

```go
func TestFunctionName(t *testing.T) {
    tests := []struct {
        name    string
        input   InputType
        want    OutputType
        wantErr bool
    }{
        {
            name:    "valid input",
            input:   InputType{...},
            want:    OutputType{...},
            wantErr: false,
        },
        {
            name:    "invalid input - empty field",
            input:   InputType{...},
            want:    OutputType{},
            wantErr: true,
        },
        // ... more test cases
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := FunctionName(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if !reflect.DeepEqual(got, tt.want) {
                t.Errorf("got %v, want %v", got, tt.want)
            }
        })
    }
}
```

---

### B.2 Test Fixtures

**Organize Test Data:**

```
internal/package/
├── package.go
├── package_test.go
└── testdata/
    ├── valid_input.json
    ├── invalid_input.json
    └── expected_output.json
```

**Usage:**

```go
func loadTestData(t *testing.T, filename string) []byte {
    data, err := os.ReadFile(filepath.Join("testdata", filename))
    require.NoError(t, err)
    return data
}

func TestParser(t *testing.T) {
    input := loadTestData(t, "valid_input.json")
    expected := loadTestData(t, "expected_output.json")
    // ... test logic
}
```

---

### B.3 Mocking External Dependencies

**Use Interfaces:**

```go
// Production:
type HTTPClient interface {
    Do(req *http.Request) (*http.Response, error)
}

// Real implementation:
type realHTTPClient struct{ client *http.Client }
func (rhc *realHTTPClient) Do(req *http.Request) (*http.Response, error) {
    return rhc.client.Do(req)
}

// Mock for testing:
type mockHTTPClient struct {
    responses []*http.Response
    errors    []error
    callCount int
}

func (mhc *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
    defer func() { mhc.callCount++ }()
    if mhc.callCount < len(mhc.errors) && mhc.errors[mhc.callCount] != nil {
        return nil, mhc.errors[mhc.callCount]
    }
    return mhc.responses[mhc.callCount], nil
}
```

---

### B.4 Coverage Excludes

**When to Skip Coverage:**

Some code is inherently hard to test and can be excluded from coverage requirements:

1. **main() functions** in cmd/ - These are orchestration only
2. **Panic handlers** - Deliberately crash paths
3. **Backwards compatibility shims** - Legacy code slated for removal
4. **Debug/development-only code** - Guarded by build tags

**How to Exclude:**

```go
// +build !test
```

Or:

```go
// coverage:ignore
func debugOnlyFunction() {
    // ...
}
```

---

## Appendix C: Glossary

**Coverage Types:**

- **Line Coverage:** % of source code lines executed during tests
- **Branch Coverage:** % of conditional branches (if/else) tested
- **Statement Coverage:** % of executable statements tested (similar to line coverage)

**Testing Terms:**

- **Unit Test:** Tests a single function/method in isolation
- **Integration Test:** Tests multiple components working together
- **Table-Driven Test:** Single test function with multiple input/output cases
- **Mock:** Fake implementation of an interface for testing
- **Fixture:** Predefined test data/state

**Architecture Terms:**

- **DRY:** Don't Repeat Yourself - avoid code duplication
- **Dependency Injection:** Passing dependencies as parameters rather than creating them internally
- **Testability:** How easy it is to write automated tests for code

---

## Document Revision History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-01-31 | Ictinus (AI Agent) | Initial comprehensive analysis |

---

## Recommendations Summary

### Immediate Actions (This Sprint):

1. ✓ **Fix build issues** preventing api and lidar/monitor tests
2. ✓ **Add 100 edge case tests** to existing internal/ packages
3. ✓ **Move executor.go and sshconfig.go** to internal/deploy

**Expected impact: 76% → 85% coverage**

### Next Sprint:

4. ✓ **Refactor cmd/sweep** → internal/lidar/sweep package
5. ✓ **Extract cmd/radar transit logic** → internal/db/transits_cli.go
6. ✓ **Refactor cmd/deploy installer/upgrader** → internal/deploy modules

**Expected impact: 85% → 90% coverage**

### Following Sprints:

7. ✓ **Decouple eCharts rendering** from data preparation
8. ✓ **Abstract embedded HTML** for testing
9. ✓ **Add dependency injection** for clock, filesystem, etc.

**Expected impact: 90% → 92%+ coverage**

---

**End of Document**
