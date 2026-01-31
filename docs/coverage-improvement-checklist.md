# Code Coverage Improvement - Implementation Checklist

**Related Documents:**

- [Executive Summary](./coverage-improvement-summary.md)
- [Full Analysis](./coverage-improvement-analysis.md)

---

## Phase 1: Add Edge Case Tests (Target: 76% → 85% coverage)

**Timeline:** 1-2 weeks
**Goal:** Add comprehensive tests to existing code without any refactoring

### Pre-Work: Fix Build Issues

- [ ] **Fix web build assets** (blocking api and lidar/monitor tests)
  - [ ] Build web assets OR
  - [ ] Skip asset-dependent tests in CI
  - [ ] Verify internal/api tests pass
  - [ ] Verify internal/lidar/monitor tests pass
  - **Estimated:** 2-4 hours

### Task 1.1: Add Edge Case Tests to internal/db

- [ ] **Migration error paths** (migrate.go, migrate_cli.go)
  - [ ] Test filesystem errors during migration
  - [ ] Test SQL execution errors
  - [ ] Test schema version detection edge cases
- [ ] **Transit worker edge cases** (transit_worker.go)
  - [ ] Test empty time windows
  - [ ] Test boundary conditions
  - [ ] Test deduplication with overlaps
  - [ ] Test error recovery
- [ ] **Admin route validation** (site.go, site_report.go)
  - [ ] Test invalid input validation
  - [ ] Test database constraint violations
  - [ ] Test concurrent access
- [ ] **Target:** 50-80 new test cases
- [ ] **Coverage goal:** 78.7% → 88%
- **Estimated:** 3-4 days

### Task 1.2: Add Edge Case Tests to internal/lidar/parse

- [ ] **Pandar40P parser edge cases**
  - [ ] Test malformed packet handling
  - [ ] Test invalid azimuth values
  - [ ] Test invalid elevation values
  - [ ] Test angle wrapping boundaries
- [ ] **Configuration validation**
  - [ ] Test invalid JSON
  - [ ] Test missing required fields
  - [ ] Test out-of-range values
- [ ] **Target:** 30-40 new test cases
- [ ] **Coverage goal:** 77.4% → 88%
- **Estimated:** 2-3 days

### Task 1.3: Add Edge Case Tests to internal/serialmux

- [ ] **Serial port error handling**
  - [ ] Test device disconnect scenarios
  - [ ] Test baud rate mismatches
  - [ ] Test read/write timeouts
- [ ] **Mock serial edge cases**
  - [ ] Test buffer overflow
  - [ ] Test concurrent access
- [ ] **Target:** 20-25 new test cases
- [ ] **Coverage goal:** 86.3% → 91%
- **Estimated:** 1-2 days

### Phase 1 Verification

- [ ] Run `make test-go` and verify all tests pass
- [ ] Check coverage: `go test -cover ./internal/...`
- [ ] Verify internal/ coverage ≥ 85%
- [ ] Update this checklist with actual coverage achieved

**Phase 1 Complete:** ☐ YES ☐ NO
**Achieved Coverage:** **\_\_**%
**Date Completed:** \***\*\_\_\_\_\*\***

---

## Phase 2: Extract cmd/ Logic to internal/ (Target: 85% → 92% coverage)

**Timeline:** 4-6 weeks
**Goal:** Move all business logic from cmd/ packages to internal/ for better testing and reuse

### Task 2.3: Extract cmd/sweep/main.go → internal/lidar/sweep/

#### 2.3.1: Create internal/lidar/sweep/math.go

- [ ] **Move utility functions**
  - [ ] ParseCSVFloat64s (from parseCSVFloatSlice)
  - [ ] ParseCSVInts (from parseCSVIntSlice)
  - [ ] ToFloat64Slice
  - [ ] ToInt64Slice
  - [ ] MeanStddev (from meanStddev)
- [ ] **Add tests**
  - [ ] Test parsing valid inputs
  - [ ] Test parsing empty/invalid inputs
  - [ ] Test type conversions
  - [ ] Test statistical calculations
- [ ] **Target coverage:** 90%+
- **Estimated:** 1-2 days

#### 2.3.2: Create internal/lidar/sweep/ranges.go

- [ ] **Create RangeSpec and IntRangeSpec types**
  - [ ] Move generateRange logic
  - [ ] Move generateIntRange logic
  - [ ] Move parseParamList logic
  - [ ] Move parseIntParamList logic
- [ ] **Add tests**
  - [ ] Test range generation
  - [ ] Test edge cases (zero step, negative)
  - [ ] Test floating-point precision
- [ ] **Target coverage:** 90%+
- **Estimated:** 1 day

#### 2.3.3: Create internal/lidar/monitor/client.go

- [ ] **Create Client struct**
  - [ ] Move startPCAPReplay → StartPCAPReplay
  - [ ] Move fetchBuckets → FetchBuckets
  - [ ] Move resetGrid → ResetGrid
  - [ ] Move setParams → SetParams
  - [ ] Move resetAcceptance → ResetAcceptance
  - [ ] Move waitForGridSettle → WaitForGridSettle
- [ ] **Add tests**
  - [ ] Mock HTTP responses
  - [ ] Test retry logic
  - [ ] Test timeout behavior
  - [ ] Test error handling
- [ ] **Target coverage:** 85%+
- **Estimated:** 2-3 days

#### 2.3.4: Create internal/lidar/sweep/sampler.go

- [ ] **Create Sampler struct**
  - [ ] Move SampleResult type
  - [ ] Move sampleMetrics logic → Sample method
- [ ] **Add tests**
  - [ ] Mock HTTP client
  - [ ] Test sampling timing
  - [ ] Test context cancellation
  - [ ] Test metric aggregation
- [ ] **Target coverage:** 85%+
- **Estimated:** 2 days

#### 2.3.5: Create internal/lidar/sweep/output.go

- [ ] **Create CSVWriter struct**
  - [ ] Move writeHeaders → WriteHeaders
  - [ ] Move writeRawRow → WriteRawSample
  - [ ] Move writeSummary → WriteSummary
- [ ] **Add tests**
  - [ ] Test CSV formatting
  - [ ] Test header/data alignment
  - [ ] Test statistical summaries
- [ ] **Target coverage:** 85%+
- **Estimated:** 1-2 days

#### 2.3.6: Refactor cmd/sweep/main.go

- [ ] **Update to use new internal packages**
  - [ ] Replace inline functions with library calls
  - [ ] Keep only flag parsing and orchestration
  - [ ] Verify CLI still works as expected
- [ ] **Manual testing**
  - [ ] Run sweep with test parameters
  - [ ] Verify output files generated correctly
- **Estimated:** 1 day

### Task 2.4: Extract cmd/radar/radar.go Logic

#### 2.4.1: Create internal/db/transits_cli.go

- [ ] **Create TransitCLI struct**
  - [ ] Move runTransitsCommand logic
  - [ ] Extract Analyse method
  - [ ] Extract Delete method
  - [ ] Extract Migrate method
  - [ ] Extract Rebuild method
- [ ] **Add tests**
  - [ ] Mock database
  - [ ] Test each operation
  - [ ] Test error paths
- [ ] **Target coverage:** 90%+
- **Estimated:** 2-3 days

#### 2.4.2: Create internal/lidar/config.go

- [ ] **Create BackgroundConfig struct**
  - [ ] Move BackgroundParams initialization
  - [ ] Add validation methods
  - [ ] Add ToBackgroundParams converter
- [ ] **Add tests**
  - [ ] Test configuration validation
  - [ ] Test default values
  - [ ] Test parameter constraints
- [ ] **Target coverage:** 85%+
- **Estimated:** 1-2 days

#### 2.4.3: Create internal/lidar/background_flusher.go

- [ ] **Create BackgroundFlusher struct**
  - [ ] Move flush timer loop from main
  - [ ] Add Run method with context support
- [ ] **Add tests**
  - [ ] Test flush timing
  - [ ] Test context cancellation
  - [ ] Mock database writes
- [ ] **Target coverage:** 90%+
- **Estimated:** 1 day

#### 2.4.4: Refactor cmd/radar/radar.go

- [ ] **Update to use new internal packages**
  - [ ] Use TransitCLI for subcommands
  - [ ] Use BackgroundConfig
  - [ ] Use BackgroundFlusher
  - [ ] Keep only main orchestration
- [ ] **Manual testing**
  - [ ] Test transit analyse command
  - [ ] Test transit delete command
  - [ ] Test service startup
- **Estimated:** 1 day

### Task 2.5: Extract cmd/deploy/ Logic to internal/deploy/

#### 2.5.1: Create internal/deploy/binary.go

- [ ] **Create BinaryInstaller struct**
  - [ ] Move validateBinary → Validate
  - [ ] Move installBinary → Install
- [ ] **Add tests**
  - [ ] Test binary validation
  - [ ] Test installation steps
  - [ ] Mock executor
- [ ] **Target coverage:** 90%+
- **Estimated:** 1 day

#### 2.5.2: Create internal/deploy/system.go

- [ ] **Create SystemSetup struct**
  - [ ] Move createServiceUser → CreateServiceUser
  - [ ] Move createDataDirectory → CreateDataDirectory
- [ ] **Add tests**
  - [ ] Test user creation
  - [ ] Test directory creation
  - [ ] Test permissions
- [ ] **Target coverage:** 90%+
- **Estimated:** 1 day

#### 2.5.3: Create internal/deploy/service.go

- [ ] **Create ServiceInstaller struct**
  - [ ] Move installService → Install
  - [ ] Add Enable method
  - [ ] Add Start method
- [ ] **Add tests**
  - [ ] Test service file installation
  - [ ] Test systemd operations
  - [ ] Mock executor
- [ ] **Target coverage:** 90%+
- **Estimated:** 1 day

#### 2.5.4: Create internal/deploy/database.go

- [ ] **Create DatabaseMigrator struct**
  - [ ] Move migrateDatabase → Migrate
- [ ] **Add tests**
  - [ ] Test migration execution
  - [ ] Test error handling
- [ ] **Target coverage:** 90%+
- **Estimated:** 1 day

#### 2.5.5: Create internal/deploy/upgrade.go, version.go, backup.go

- [ ] **Extract upgrader.go logic**
  - [ ] Create UpgradeManager
  - [ ] Create VersionChecker
  - [ ] Create BackupManager
- [ ] **Add tests for each**
- [ ] **Target coverage:** 90%+
- **Estimated:** 2-3 days

#### 2.5.6: Create internal/deploy/status.go, rollback.go

- [ ] **Extract monitor.go logic** → StatusCollector
- [ ] **Extract rollback.go logic** → RollbackManager
- [ ] **Add tests**
- [ ] **Target coverage:** 90%+
- **Estimated:** 2 days

#### 2.5.7: Create internal/deploy/config_validator.go

- [ ] **Extract config.go validation logic**
  - [ ] Move character validation
  - [ ] Move file content validation
- [ ] **Add tests**
- [ ] **Target coverage:** 90%+
- **Estimated:** 1 day

#### 2.5.8: Refactor cmd/deploy/\*.go files

- [ ] **Update all cmd/deploy handlers**
  - [ ] Use new internal/deploy modules
  - [ ] Keep only CLI orchestration
  - [ ] Verify all commands work
- [ ] **Manual testing**
  - [ ] Test install command (dry-run)
  - [ ] Test upgrade command (dry-run)
  - [ ] Test status command
  - [ ] Test rollback command (dry-run)
- **Estimated:** 2 days

### Task 2.6: Extract cmd/tools/scan_transits.go

- [ ] **Create internal/db/transit_gaps.go**
  - [ ] Move TransitGap struct (export)
  - [ ] Move findTransitGaps → FindTransitGaps method
  - [ ] Add FindTransitGapsInRange variant
- [ ] **Add tests**
  - [ ] Test gap detection logic
  - [ ] Test edge cases (no data, all gaps)
  - [ ] Mock database
- [ ] **Update cmd/tools/scan_transits.go**
  - [ ] Use new internal/db method
  - [ ] Keep CLI logic only
- [ ] **Target coverage:** 90%+
- **Estimated:** 1 day

### Phase 2 Verification

- [ ] Run `make test-go` and verify all tests pass
- [ ] Check coverage: `go test -cover ./internal/...`
- [ ] Verify internal/ coverage ≥ 92%
- [ ] Manual test all affected CLI commands:
  - [ ] Test sweep command with various parameters
  - [ ] Test radar transit commands (analyse, delete, migrate, rebuild)
  - [ ] Test deploy commands (install, upgrade, status, rollback)
  - [ ] Test scan_transits tool
- [ ] Update this checklist with actual coverage achieved

**Phase 2 Complete:** ☐ YES ☐ NO
**Achieved Coverage:** **\_\_**%
**Date Completed:** \***\*\_\_\_\_\*\***

---

## Phase 3: Testability Improvements (Target: 92% → 94%+ coverage)

**Timeline:** 2-3 weeks
**Goal:** Improve architecture for better testing

### Task 3.1: Decouple eCharts Rendering

#### 3.1.1: Separate Data Preparation

- [ ] **Create internal/lidar/monitor/chart_data.go**
  - [ ] Extract PolarChartData struct
  - [ ] Extract HeatmapChartData struct
  - [ ] Move data preparation logic from handlers
  - [ ] Create preparePolarChartData method
  - [ ] Create prepareHeatmapChartData method
- [ ] **Add tests**
  - [ ] Test data transformation logic
  - [ ] Mock background managers
  - [ ] Test edge cases (empty grids, invalid data)
- [ ] **Target coverage:** 90%+
- **Estimated:** 3-4 days

#### 3.1.2: Add JSON Endpoints

- [ ] **Create JSON API endpoints**
  - [ ] Add /api/lidar/chart/polar (returns JSON)
  - [ ] Add /api/lidar/chart/heatmap (returns JSON)
  - [ ] Add /api/lidar/chart/clusters (returns JSON)
  - [ ] Keep HTML endpoints for backwards compatibility
- [ ] **Add tests**
  - [ ] Test JSON serialization
  - [ ] Test data correctness
- [ ] **Target coverage:** 95%+
- **Estimated:** 2 days

#### 3.1.3: Mock Background Managers

- [ ] **Create internal/lidar/monitor/mock_background.go**
  - [ ] Implement MockBackgroundManager
  - [ ] Add test helpers
- [ ] **Update existing tests**
  - [ ] Use mocks in chart data tests
  - [ ] Use mocks in handler tests
- **Estimated:** 1-2 days

### Task 3.2: Abstract Embedded HTML/Assets

#### 3.2.1: Create Filesystem Abstraction

- [ ] **Create internal/lidar/monitor/templates.go**
  - [ ] Define TemplateProvider interface
  - [ ] Implement EmbeddedTemplateProvider
  - [ ] Implement MockTemplateProvider
- [ ] **Update WebServer**
  - [ ] Add templates field
  - [ ] Inject TemplateProvider in constructor
- [ ] **Add tests**
  - [ ] Test handlers with MockTemplateProvider
  - [ ] Test without requiring embedded assets
- [ ] **Target coverage:** 85%+
- **Estimated:** 2-3 days

#### 3.2.2: Abstract Asset Serving

- [ ] **Update WebServer**
  - [ ] Add assets http.FileSystem field
  - [ ] Inject in constructor
- [ ] **Add tests**
  - [ ] Mock asset filesystem
  - [ ] Test asset serving logic
- **Estimated:** 1 day

### Task 3.3: Add Dependency Injection

#### 3.3.1: Add Clock Interface

- [ ] **Create internal/utils/clock.go** (or similar)
  - [ ] Define Clock interface
  - [ ] Implement realClock
  - [ ] Implement mockClock
- [ ] **Update time-dependent code**
  - [ ] Inject Clock into structs
  - [ ] Replace time.Now() calls with clock.Now()
- [ ] **Add tests**
  - [ ] Test with fixed time
  - [ ] Test time-dependent logic deterministically
- **Estimated:** 2 days

#### 3.3.2: Inject HTTP Clients

- [ ] **Update code that creates http.Client**
  - [ ] Accept HTTPClient interface instead of creating inline
  - [ ] Add constructor parameters
- [ ] **Update tests**
  - [ ] Use mock HTTP clients
  - [ ] Test without real network calls
- **Estimated:** 1-2 days

#### 3.3.3: Abstract Filesystem Operations

- [ ] **Create filesystem abstraction**
  - [ ] Define FileSystem interface (if not using existing)
  - [ ] Wrap os package operations
- [ ] **Update file-dependent code**
  - [ ] Inject filesystem
  - [ ] Replace direct os calls
- [ ] **Add tests**
  - [ ] Use in-memory filesystem
  - [ ] Test without disk I/O
- **Estimated:** 2-3 days

### Phase 3 Verification

- [ ] Run `make test-go` and verify all tests pass
- [ ] Check coverage: `go test -cover ./internal/...`
- [ ] Verify internal/ coverage ≥ 94%
- [ ] Verify tests run faster (less I/O, no network)
- [ ] Update this checklist with actual coverage achieved

**Phase 3 Complete:** ☐ YES ☐ NO
**Achieved Coverage:** **\_\_**%
**Date Completed:** \***\*\_\_\_\_\*\***

---

## Phase 4: Maintenance & Polish (Ongoing)

**Timeline:** Ongoing
**Goal:** Maintain 90%+ coverage as codebase evolves

### Task 4.1: Set Up Coverage Enforcement

- [ ] **Update CI configuration**
  - [ ] Add coverage check to GitHub Actions
  - [ ] Set minimum coverage threshold (90%)
  - [ ] Block PRs that reduce coverage
- [ ] **Configure codecov.yml**
  - [ ] Set project coverage target
  - [ ] Set patch coverage target
- **Estimated:** 2-4 hours

### Task 4.2: Document Testing Practices

- [ ] **Create testing guide** (docs/testing-guide.md)
  - [ ] Document table-driven test pattern
  - [ ] Document mocking best practices
  - [ ] Document test fixture organization
  - [ ] Provide examples from codebase
- [ ] **Update CONTRIBUTING.md**
  - [ ] Add testing requirements
  - [ ] Link to testing guide
- **Estimated:** 1 day

### Task 4.3: Review and Improve

- [ ] **Monthly coverage review**
  - [ ] Identify newly uncovered code
  - [ ] Add tests for gaps
  - [ ] Refactor untestable code
- [ ] **Quarterly architecture review**
  - [ ] Identify new DRY violations
  - [ ] Identify new testability issues
  - [ ] Plan refactoring sprints

### Phase 4 Verification

- [ ] Coverage remains ≥ 90% for 3 months
- [ ] All new PRs include tests
- [ ] No coverage regressions merged without justification

**Phase 4 Active:** ☐ YES ☐ NO
**Last Review Date:** \***\*\_\_\_\_\*\***

---

## Overall Progress

### Coverage Milestones

- [ ] **Baseline:** 76% (internal/) - Starting point
- [ ] **Milestone 1:** 85% (internal/) - Phase 1 complete
- [ ] **Milestone 2:** 92% (internal/) - Phase 2 complete
- [ ] **Milestone 3:** 94%+ (internal/) - Phase 3 complete
- [ ] **Sustained:** 90%+ for 6 months - Phase 4 success

### Completion Status

| Phase   | Target Date      | Actual Date      | Coverage Goal | Actual Coverage |
| ------- | ---------------- | ---------------- | ------------- | --------------- |
| Phase 1 | \***\*\_\_\*\*** | \***\*\_\_\*\*** | 85%           | **\_\_**%       |
| Phase 2 | \***\*\_\_\*\*** | \***\*\_\_\*\*** | 92%           | **\_\_**%       |
| Phase 3 | \***\*\_\_\*\*** | \***\*\_\_\*\*** | 94%           | **\_\_**%       |
| Phase 4 | Ongoing          | -                | 90%+          | **\_\_**%       |

---

## Notes & Observations

**Blockers Encountered:**

_Record any blockers here..._

**Lessons Learned:**

_Record lessons learned during implementation..._

**Additional Improvements Made:**

_Record any improvements beyond the original plan..._

---

**Document Version:** 1.0
**Last Updated:** 2026-01-31
**Next Review:** \***\*\_\_\_\_\*\***
