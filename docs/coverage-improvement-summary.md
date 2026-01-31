# Code Coverage Improvement - Executive Summary

**Full Analysis:** [coverage-improvement-analysis.md](./coverage-improvement-analysis.md)

## Current State

```
internal/ packages: ~76% average coverage (Target: 90%+)
cmd/ packages:      <10% average coverage (Most have no tests)
```

## Top 3 Recommendations

### 1. Move Business Logic from cmd/ to internal/ (Highest Impact)

**What to Move:**

| File                       | Lines to Move   | New Location                  | Priority  |
| -------------------------- | --------------- | ----------------------------- | --------- |
| cmd/sweep/main.go          | 600 lines (80%) | internal/lidar/sweep/         | 🔴 HIGH   |
| cmd/deploy/\*.go           | 900 lines (85%) | internal/deploy/              | 🔴 HIGH   |
| cmd/radar/radar.go         | 280 lines (40%) | internal/db/, internal/lidar/ | 🟡 MEDIUM |
| cmd/tools/scan_transits.go | 70 lines (50%)  | internal/db/                  | 🟢 LOW    |

**Impact:** +1,630 testable lines @ 90% coverage → **+5-8% internal/ coverage**

**Effort:** 3-4 weeks

---

### 2. Add Edge Case Tests to Existing Code (Quick Win)

**Where to Add Tests:**

- **internal/db** (78.7% → 90%): Add error path tests for migrations and transit worker
- **internal/lidar/parse** (77.4% → 90%): Add tests for malformed packets and edge cases
- **internal/serialmux** (86.3% → 90%): Add tests for device disconnect scenarios

**Impact:** **+8-12% internal/ coverage**

**Effort:** 1 week

---

### 3. Improve Testability (Architectural)

**Problems to Fix:**

- **eCharts coupling**: Data preparation mixed with HTML rendering
- **Embedded HTML**: Tests fail without build assets
- **Hard-coded dependencies**: Can't mock time, filesystem, HTTP clients

**Solutions:**

- Separate data preparation from rendering
- Use dependency injection for external dependencies
- Abstract filesystem access for testing

**Impact:** +630 testable lines → **+4-6% internal/ coverage**

**Effort:** 2-3 weeks

---

## Implementation Phases

### Phase 1: Quick Wins (1-2 weeks) → 76% to 85%

- [ ] Fix build issues (api, lidar/monitor packages)
- [ ] Add 100 edge case tests to existing code
- [ ] Move cmd/deploy/executor.go and sshconfig.go to internal/

### Phase 2: Major Refactoring (3-4 weeks) → 85% to 92%

- [ ] Extract cmd/sweep → internal/lidar/sweep
- [ ] Extract cmd/radar logic → internal/db, internal/lidar
- [ ] Refactor cmd/deploy → internal/deploy

### Phase 3: Testability (2-3 weeks) → 92% to 94%

- [ ] Decouple eCharts rendering
- [ ] Abstract embedded HTML
- [ ] Add dependency injection

### Phase 4: Maintenance (Ongoing) → Maintain 90%+

- [ ] Enforce coverage in CI
- [ ] Document testing patterns
- [ ] Review uncovered code periodically

---

## Specific Files to Refactor (Priority Order)

### 🔴 High Priority (Do First)

1. **cmd/sweep/main.go** → **internal/lidar/sweep/**
   - Create 5 new modules: math.go, ranges.go, sampler.go, output.go, client.go
   - Move 600 lines of business logic
   - Expected coverage: 85-90%

2. **cmd/deploy/executor.go** → **internal/deploy/executor.go**
   - Move entire file (200 lines)
   - Pure library code, zero CLI dependencies
   - Expected coverage: 90%+

3. **cmd/deploy/sshconfig.go** → **internal/deploy/sshconfig.go**
   - Move entire file (120 lines)
   - SSH config parsing, highly testable
   - Expected coverage: 90%+

4. **cmd/deploy/installer.go** → **internal/deploy/** (multiple modules)
   - Extract: binary.go, system.go, service.go, database.go
   - Move 240 lines of logic
   - Expected coverage: 90%+

### 🟡 Medium Priority (Do Second)

5. **cmd/radar/radar.go** (transit commands) → **internal/db/transits_cli.go**
   - Extract runTransitsCommand() and sub-operations (150 lines)
   - Expected coverage: 90%+

6. **cmd/radar/radar.go** (lidar config) → **internal/lidar/config.go**
   - Extract BackgroundParams initialization (80 lines)
   - Expected coverage: 85%+

7. **cmd/deploy/upgrader.go** → **internal/deploy/** (multiple modules)
   - Extract version checking, backup, service control
   - Move 180 lines of logic
   - Expected coverage: 90%+

### 🟢 Low Priority (Do Last)

8. **cmd/tools/scan_transits.go** → **internal/db/transit_gaps.go**
   - Extract findTransitGaps() function (70 lines)
   - Expected coverage: 90%+

9. **internal/lidar/monitor/webserver.go** → Refactor eCharts usage
   - Separate data preparation from rendering
   - Add JSON endpoints
   - Add 150 lines of testable data transformation

---

## Quick Reference: What Stays in cmd/, What Moves to internal/

### ✅ Keep in cmd/ (CLI-Specific)

- Flag definitions (`flag.String()`, `flag.Bool()`, etc.)
- User prompts and confirmations
- Help text and usage information
- Thin orchestration (calls internal/ functions)
- Output formatting for CLI (stdout/stderr)

### ❌ Move to internal/ (Business Logic)

- Data parsing and transformation
- HTTP client operations
- Database queries and business logic
- Mathematical/statistical calculations
- Configuration validation
- SSH operations and remote execution
- File operations (beyond CLI I/O)
- Background tasks and workers

---

## Expected Outcomes

| Phase   | Time      | Coverage Gain | Cumulative Coverage |
| ------- | --------- | ------------- | ------------------- |
| Start   | -         | -             | 76%                 |
| Phase 1 | 1-2 weeks | +9-11%        | 85-87%              |
| Phase 2 | 3-4 weeks | +5-7%         | 92-94%              |
| Phase 3 | 2-3 weeks | +0-2%         | 92-96%              |
| Phase 4 | Ongoing   | Maintain      | 90%+ stable         |

**Total Time to 90%+:** 6-8 weeks
**Final Target:** 90-92% sustained coverage

---

## Next Steps

1. **Review this document** with team
2. **Prioritize based on velocity** (suggest starting with Phase 1)
3. **Create GitHub issues** for each refactoring task
4. **Set up coverage tracking** in CI to prevent regressions
5. **Begin with quick wins** to build momentum

---

**Questions?** See full analysis document: [coverage-improvement-analysis.md](./coverage-improvement-analysis.md)
