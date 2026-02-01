# Coverage Improvement Status

**Last Updated:** 2026-01-31
**Current Status:** Phases 1-4 Complete, Phase 5 (Maintenance) In Progress

---

## Current Coverage Snapshot

| Package                | Coverage | Target | Status    |
| ---------------------- | -------- | ------ | --------- |
| internal/units         | 100.0%   | 90%    | ✅        |
| internal/monitoring    | 100.0%   | 90%    | ✅        |
| internal/lidar/sweep   | 99.4%    | 90%    | ✅        |
| internal/fsutil        | 99.0%    | 90%    | ✅        |
| internal/httputil      | 96.2%    | 90%    | ✅        |
| internal/timeutil      | 95.5%    | 90%    | ✅        |
| internal/lidar/network | 94.2%    | 90%    | ✅        |
| internal/security      | 90.5%    | 90%    | ✅        |
| internal/lidar         | 89.9%    | 90%    | ⚠️ -0.1%  |
| internal/serialmux     | 89.9%    | 90%    | ⚠️ -0.1%  |
| internal/lidar/parse   | 89.8%    | 90%    | ⚠️ -0.2%  |
| internal/deploy        | 83.8%    | 90%    | ❌ -6.2%  |
| internal/db            | 79.5%    | 90%    | ❌ -10.5% |
| internal/api           | 76.6%    | 90%    | ❌ -13.4% |
| internal/lidar/monitor | 69.0%    | 90%    | ❌ -21.0% |

**Summary:** 8/15 packages at target, 11/15 packages ≥ 85%

---

## Progress Summary

### Baseline → Current

- **Starting coverage:** ~76% (internal/ weighted average)
- **Current coverage:** ~89% (internal/ weighted average)
- **Improvement:** +13 percentage points

### Completed Phases

#### Phase 1: Edge Case Tests ✅

- Added ~175 edge case tests across internal/db, internal/lidar/parse, internal/serialmux
- Fixed build issues blocking api and lidar/monitor tests
- Result: 76% → 85.9%

#### Phase 2: Extract cmd/ Logic to internal/ ✅

- Extracted cmd/sweep → internal/lidar/sweep (99.4% coverage)
- Extracted cmd/radar logic → internal/db/transits_cli.go, internal/lidar/config.go, internal/lidar/background_flusher.go
- Extracted cmd/deploy core → internal/deploy/executor.go, sshconfig.go
- Extracted cmd/tools/scan_transits → internal/db/transit_gaps.go
- cmd/sweep reduced from 742 → 276 lines (63% reduction)
- cmd/radar reduced from 653 → 579 lines

#### Phase 3: Testability Improvements ✅

- Created chart_data.go with separated data preparation (JSON-serialisable)
- Added JSON API endpoints for all chart types
- Created MockBackgroundManager for handler testing
- Added TemplateProvider and AssetProvider abstractions

#### Phase 4: Infrastructure Dependency Injection ✅

- Created CommandExecutor interface for SSH/SCP abstraction
- Created PCAPReader interface for PCAP file reading
- Created UDPSocket interface for network operations
- Enhanced SerialPortFactory abstraction
- Created DataSourceManager interface for WebServer testing

---

## Outstanding Work

### Quick Wins (3-5 hours)

These packages are within 0.2% of target and need 2-5 tests each:

- [ ] internal/lidar: +0.1% (2-3 tests for background manager edge cases)
- [ ] internal/serialmux: +0.1% (2-3 tests for disconnect scenarios)
- [ ] internal/lidar/parse: +0.2% (2-3 tests for malformed packet handling)

### Significant Gaps (Lower Priority)

These packages require more effort or have legitimate reasons for lower coverage:

| Package                | Gap    | Blocker                                         | Recommendation                            |
| ---------------------- | ------ | ----------------------------------------------- | ----------------------------------------- |
| internal/lidar/monitor | -21%   | Requires real LiDAR mocks, embedded HTML assets | Accept current level; infrastructure code |
| internal/api           | -13.4% | E2E test needs Python dependencies              | Fix Python deps in CI                     |
| internal/db            | -10.5% | CLI functions use os.Exit(), need refactoring   | Extract testable logic incrementally      |
| internal/deploy        | -6.2%  | Real SSH/SCP operations                         | Add integration tests or accept           |

### Phase 5: Maintenance (Ongoing)

- [ ] Enforce coverage threshold (85%) in CI
- [ ] Document testing patterns and best practices
- [ ] Create testing guide in CONTRIBUTING.md
- [ ] Review uncovered code quarterly

---

## Key Test Data

**PCAP replay tests:** All tests requiring PCAP replay use `internal/lidar/perf/pcap/kirk0.pcapng`

---

## Changelog

| Date       | Change                                                |
| ---------- | ----------------------------------------------------- |
| 2026-01-31 | Phase 4 complete, consolidated coverage documentation |
| 2026-02-01 | Assessment updated with 89.7% overall coverage        |
| 2026-01-31 | Phases 1-3 complete                                   |
