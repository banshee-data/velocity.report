# Codebase Review and Improvement Plan

This document provides a comprehensive review of the velocity.report codebase with recommendations for simplification, testability improvements, CI optimisation, and documentation enhancements.

## Executive Summary

**Codebase Size:**
- **Go**: ~86,000 lines (source + tests), 128 test files
- **Python**: ~18,000 lines (PDF generator)
- **Web**: ~7,600 lines (Svelte/TypeScript)
- **CI Workflows**: 8 workflow files

**Current State:**
- Well-structured multi-component system
- Good test coverage in most areas
- Comprehensive documentation exists but has some gaps
- CI is functional but could be optimised for speed and clarity

---

## Phase 1: Code Simplification Opportunities

### 1.1 Large Files That Could Benefit from Refactoring

| File | Lines | Recommendation | Effort |
|------|-------|----------------|--------|
| `internal/lidar/monitor/webserver.go` | 3,048 | Split into handler groups (grid, snapshot, params, export) | 4h |
| `internal/lidar/background.go` | 1,777 | Extract grid operations into separate package | 3h |
| `internal/api/server.go` | 1,704 | Split into route files by domain (site, radar, reports, admin) | 3h |
| `internal/db/db.go` | 1,267 | Extract query builders into separate files | 2h |
| `cmd/tools/pcap-analyse/main.go` | 1,404 | Split analysis and reporting logic | 2h |

**Total Estimate: 14 hours**

### 1.2 Test File Consolidation

The test files follow a pattern of `*_test.go`, `*_extended_test.go`, `*_edge_test.go`, etc. Consider:

| Package | Current Test Files | Recommendation |
|---------|-------------------|----------------|
| `internal/db` | 28 test files (12,419 lines) | Group related tests; use table-driven tests more consistently |
| `internal/lidar` | 60 test files (27,065 lines) | Consolidate edge/extended tests; extract common fixtures |
| `internal/api` | 8 test files (4,992 lines) | Consider test helper package for common setup |

**Recommendation**: Create a `testutil` package for shared test fixtures and helpers.

**Estimate: 8 hours**

### 1.3 Duplicate Code Patterns

Areas identified for potential DRY improvements:

1. **CI Workflow Setup** - Repeated Node.js/pnpm setup across 4 workflows
2. **Makefile Patterns** - Repeated pnpm/npm fallback logic
3. **HTTP Response Handling** - Similar patterns in `internal/api` and `internal/lidar/monitor`

**Estimate: 4 hours**

---

## Phase 2: Testability Improvements

### 2.1 Interface Extraction

Current architecture has some tight coupling that could be improved:

| Component | Current | Recommendation | Benefit |
|-----------|---------|----------------|---------|
| Database | `*db.DB` passed directly | Extract `Store` interface | Easier mocking in tests |
| Serial Port | `SerialPortManager` | Already has interface, good! | - |
| HTTP Client | Direct `http.Client` usage | Extract `HTTPDoer` interface | Mock external APIs |

**Estimate: 6 hours**

### 2.2 Test Parallelisation

Many tests could run in parallel but don't explicitly use `t.Parallel()`:

```go
// Recommendation: Add at start of tests that don't share state
func TestSomething(t *testing.T) {
    t.Parallel()
    // ...
}
```

**Impact**: Could reduce local test time by 30-50% on multi-core machines.

**Estimate: 2 hours**

### 2.3 Test Data Management

Currently test data is inline or in `testdata/` directories. Consider:

1. Creating a `internal/testdata` package with shared fixtures
2. Using test factories for complex objects
3. Adding golden file testing for report generation

**Estimate: 4 hours**

---

## Phase 3: CI Speed Improvements

### 3.1 Current CI Analysis

| Workflow | Jobs | Estimated Duration | Bottleneck |
|----------|------|-------------------|------------|
| `go-ci.yml` | 2 (test, format) | ~3-5 min | Web build + Python deps |
| `python-ci.yml` | 2 (lint, test) | ~2-3 min | LaTeX not needed for Python tests |
| `web-ci.yml` | 2 (lint, test) | ~1-2 min | pnpm install |
| `docs-ci.yml` | 2 (lint, build) | ~1-2 min | OK |
| `version-check.yml` | 1 | ~1 min | OK |
| `performance.yml` | 1 | ~2 min | Only runs on lidar changes |
| `lint-autofix.yml` | 1 | Weekly only | OK |
| `deploy-gh-pages.yml` | 2 | ~1-2 min | OK |

### 3.2 Recommended CI Improvements

#### 3.2.1 Split Go CI into Parallel Jobs

**Current**: Single `test` job does everything sequentially.

**Proposed Structure**:

```yaml
jobs:
  build:
    name: Build & Lint
    runs-on: ubuntu-latest
    steps:
      - Checkout, Setup Go
      - Cache modules
      - Run `make lint-go`
      - Build binaries
  
  test-core:
    name: Test Core (db, api, radar, serialmux)
    needs: build
    runs-on: ubuntu-latest
    steps:
      - Run tests for internal/db, internal/api, internal/radar, internal/serialmux

  test-lidar:
    name: Test LiDAR
    needs: build
    runs-on: ubuntu-latest
    steps:
      - Run tests for internal/lidar (separate due to size)

  test-integration:
    name: Integration Tests
    needs: [test-core, test-lidar]
    runs-on: ubuntu-latest
    steps:
      - Run E2E tests (requires Python for PDF generation)
```

**Benefits**:
- Parallel execution reduces wall time
- Clearer failure identification
- Faster feedback on partial failures

**Estimate: 4 hours**

#### 3.2.2 Remove Unnecessary Dependencies from Go CI

The Go CI currently installs:
- `libpcap-dev` (needed for build)
- `texlive-xetex` (only needed for PDF E2E tests)
- Web build (needed for embed)
- Python deps (only needed for PDF E2E tests)

**Recommendation**: Move PDF-related E2E tests to a separate job or workflow.

**Estimate: 2 hours**

#### 3.2.3 Shared Setup Action

Create a composite action for common setup:

```yaml
# .github/actions/setup-node-pnpm/action.yml
name: Setup Node.js with pnpm
inputs:
  node-version:
    default: '20'
  working-directory:
    default: 'web'
runs:
  using: composite
  steps:
    - uses: actions/setup-node@v4
      with:
        node-version: ${{ inputs.node-version }}
    - uses: pnpm/action-setup@v4
    - name: Get pnpm store
      id: pnpm-store
      shell: bash
      run: echo "path=$(pnpm store path --silent)" >> $GITHUB_OUTPUT
    - uses: actions/cache@v4
      with:
        path: ${{ steps.pnpm-store.outputs.path }}
        key: ${{ runner.os }}-pnpm-${{ hashFiles('**/pnpm-lock.yaml') }}
```

**Estimate: 2 hours**

#### 3.2.4 Dependency Caching Improvements

| Area | Current | Recommendation |
|------|---------|----------------|
| Go modules | Per-workflow cache | Share cache key across workflows |
| pnpm store | Per-workflow cache | OK |
| Python venv | Re-built each time | Cache `.venv` directory |

**Estimate: 2 hours**

---

## Phase 4: CI Simplification (Breaking into Coherent Tasks)

### 4.1 Proposed Workflow Structure

**Goal**: Make GitHub Actions UI more understandable with clear job names.

#### Current Workflows (8 files):
1. `go-ci.yml` - Go tests + format
2. `python-ci.yml` - Python lint + test
3. `web-ci.yml` - Web lint + test
4. `docs-ci.yml` - Docs lint + build
5. `version-check.yml` - Version bump advisory
6. `performance.yml` - LiDAR benchmarks
7. `lint-autofix.yml` - Weekly cleanup
8. `deploy-gh-pages.yml` - Docs deployment

#### Recommended Structure (Keep 8, but reorganise jobs):

| Workflow | Jobs (Proposed) | Runs On |
|----------|-----------------|---------|
| `go-ci.yml` | `lint`, `build`, `test-core`, `test-lidar`, `test-integration` | Go changes |
| `python-ci.yml` | `lint`, `test`, `coverage` | Python changes |
| `web-ci.yml` | `lint`, `test`, `build` | Web changes |
| `docs-ci.yml` | `lint`, `build` | Docs changes |
| `version-check.yml` | `analyse` | All PRs |
| `performance.yml` | `benchmark` | LiDAR changes |
| `lint-autofix.yml` | `autofix` | Weekly |
| `deploy-gh-pages.yml` | `build`, `deploy` | Main push |

### 4.2 Job Naming Convention

Adopt consistent naming for better web view:

```yaml
jobs:
  lint:
    name: "🔍 Lint"
  test:
    name: "🧪 Test"
  build:
    name: "🔨 Build"
  deploy:
    name: "🚀 Deploy"
```

**Estimate: 3 hours**

---

## Phase 5: Documentation Improvements

### 5.1 Identified Documentation Gaps

| Area | Current State | Recommendation |
|------|---------------|----------------|
| API Reference | None | Generate from Go code comments |
| Database Schema | `schema.sql` + `schema.svg` | Add field descriptions in code |
| Development Workflow | README sections | Consolidate into DEVELOPMENT.md |
| Troubleshooting | `TROUBLESHOOTING.md` exists | Expand with common issues |
| Architecture | `ARCHITECTURE.md` is comprehensive | Keep updated |

### 5.2 Quick Wins

1. **Add `make help` improvements** - Currently 59 targets, could be categorised better
2. **Create `docs/development/` directory** - For internal developer guides
3. **Add inline documentation for complex algorithms** - Especially in `internal/lidar`
4. **Update CONTRIBUTING.md prerequisites** - Go 1.25 is specified, verify current

**Estimate: 4 hours**

### 5.3 Documentation DRY Improvements

Currently duplicated across:
- `README.md` (project structure)
- `ARCHITECTURE.md` (project structure)
- `CONTRIBUTING.md` (setup instructions)

**Recommendation**: Use single source of truth with links:
- Keep detailed structure in `README.md`
- Reference from other docs with "See [README.md](README.md#project-structure)"

**Estimate: 2 hours**

---

## Implementation Roadmap

### Priority 1: Quick Wins (< 1 day)

- [ ] Add `t.Parallel()` to independent tests
- [ ] Create shared pnpm setup action
- [ ] Fix Go CI cache key consistency
- [ ] Add job name emojis for CI clarity

**Total: 4 hours**

### Priority 2: CI Optimisation (1-2 days)

- [ ] Split Go CI into parallel jobs
- [ ] Move PDF E2E tests to separate job
- [ ] Add Python venv caching
- [ ] Create composite actions for common setup

**Total: 10 hours**

### Priority 3: Code Simplification (3-5 days)

- [ ] Split `internal/api/server.go` into route files
- [ ] Split `internal/lidar/monitor/webserver.go`
- [ ] Create `internal/testutil` package
- [ ] Extract HTTP response helpers

**Total: 20 hours**

### Priority 4: Documentation (1 day)

- [ ] Consolidate setup instructions
- [ ] Add API documentation
- [ ] Improve Makefile help output
- [ ] Update TROUBLESHOOTING.md

**Total: 6 hours**

---

## Summary of Estimates

| Phase | Description | Effort |
|-------|-------------|--------|
| 1 | Code Simplification | 26 hours |
| 2 | Testability Improvements | 12 hours |
| 3 | CI Speed Improvements | 10 hours |
| 4 | CI Simplification | 3 hours |
| 5 | Documentation | 6 hours |
| **Total** | | **57 hours** |

**Recommended Approach**: Start with Priority 1 (Quick Wins) to demonstrate value, then tackle Priority 2 (CI Optimisation) for immediate developer experience improvement.

---

## Appendix: Metrics

### Current Test Coverage by Package

| Package | Test Files | Test Lines | Ratio (test:source) |
|---------|------------|------------|---------------------|
| `internal/lidar` | 60 | 27,065 | ~1:1 |
| `internal/db` | 28 | 12,419 | ~2:1 |
| `internal/api` | 8 | 4,992 | ~0.8:1 |
| `internal/serialmux` | 9 | 3,296 | ~1.5:1 |

### CI Workflow Execution Frequency

Based on path filters:
- `go-ci.yml`: Runs on most PRs (Go code changes)
- `web-ci.yml`: Runs on web changes only
- `python-ci.yml`: Runs on tools/data changes
- `docs-ci.yml`: Runs on public_html changes

### File Size Distribution

- Files > 1,000 lines: 20 (primarily in `internal/lidar`)
- Files > 500 lines: 45
- Average file size: ~250 lines

---

*Document generated: 2026-02-01*
*Author: Codebase Review Agent*
