# Coverage Improvement Plan: 95.5% Target

**Goal:** Raise every package/module/file to ≥ 95.5% line coverage.

**Baseline measured:** 2026-02-20

---

## Current State Summary

| Component | Overall | Files ≥ 95.5% | Files < 95.5% |
|-----------|---------|---------------|----------------|
| Go `internal/` | 90.3% | 7 | 18 |
| Go `cmd/` | 18.6% | 0 | 3 (+5 untested) |
| Web (statements) | 96.0% | 9 of 11 | 2 |
| Python | 93.6% | 9 of 19 | 10 |

---

## Tier 1 — Quick Wins (< 2% gap, ~1–3 tests each)

These packages need only a handful of additional test cases.

### Go

| Package | Current | Gap | What to Test |
|---------|---------|-----|--------------|
| `internal/serialmux` | 94.9% | 0.6% | Edge cases in port option validation |
| `internal/lidar` (root) | 94.4% | 1.1% | Uncovered branches in alias helpers |
| `internal/lidar/l5tracks` | 94.1% | 1.4% | Track expiry and miss-count edge paths |
| `internal/httputil` | 93.9% | 1.6% | Error paths in `Get`/`Post`/`WriteJSON` helpers (mock HTTP) |
| `internal/deploy` | 93.4% | 2.1% | `RunSudo` error branch, `ParseSSHConfigFrom` edge cases |

### Web

| File | Current | Gap | What to Test |
|------|---------|-----|--------------|
| `sweep_dashboard.js` | 95.1% | 0.4% | ~7 uncovered branches in chart-update paths |
| `api.ts` | 94.8% | 0.7% | ~5 uncovered error/retry branches in API client |

### Python

| Module | Current | Gap | What to Test |
|--------|---------|-----|--------------|
| `core/document_builder.py` | 94.8% | 0.7% | LaTeX preamble fallback paths |
| `core/map_utils.py` | 94.5% | 1.0% | Missing-data guard clauses in map tile fetching |
| `core/pdf_generator.py` | 94.3% | 1.2% | Error handling in subprocess calls |
| `core/dependency_checker.py` | 94.0% | 1.5% | Version-parsing edge cases, missing-binary paths |

**Estimated effort:** 2–3 days

---

## Tier 2 — Moderate Work (2–5% gap, ~5–15 tests each)

### Go

| Package | Current | Gap | Key Uncovered Functions |
|---------|---------|-----|------------------------|
| `internal/lidar/l3grid` | 93.2% | 2.3% | `serializeGrid` (66.7%), `NewBackgroundManager` (72.2%), `WithForeground*` option funcs |
| `internal/lidar/l6objects` | 92.5% | 3.0% | `birdConfidence` (66.7%), `NewTrackClassifierWithMinObservations` (66.7%), classification edge cases |
| `internal/lidar/storage/sqlite` | 92.1% | 3.4% | Transaction rollback paths, bulk-insert error recovery |
| `internal/lidar/visualiser` | 92.0% | 3.5% | Protobuf serialisation edge cases, frame rendering |
| `internal/lidar/sweep` | 91.7% | 3.8% | Parameter combination overflow guards, result aggregation |
| `internal/lidar/l2frames` | 91.6% | 3.9% | Frame assembly timeouts, malformed-packet recovery |
| `internal/lidar/visualiser/recorder` | 91.6% | 3.9% | File-rotation, write-error handling |
| `internal/lidar/adapters` | 91.4% | 4.1% | `Evaluate` (0%), `NewGroundTruthEvaluator` (0%), `SetSequenceID` (0%) |
| `internal/lidar/pipeline` | 91.4% | 4.1% | Pipeline shutdown ordering, error propagation |
| `internal/db` | 90.8% | 4.7% | `NewDBWithMigrationCheck` (77.5%), `DetectSchemaVersion` (82.6%), `withDB` (20%), migration force/rollback |
| `internal/lidar/monitor` | 90.4% | 5.1% | PCAP replay handlers (0%), auto-tune suspend/resume (0%), `handleDataSource` (65%), `WaitForGridSettle` (70.6%) |
| `internal/security` | 90.5% | 5.0% | Path-traversal rejection for edge-case payloads |

### Python

| Module | Current | Gap | What to Test |
|--------|---------|-----|--------------|
| `core/report_sections.py` | 91.8% | 3.7% | Optional section rendering when data is absent |
| `cli/main.py` | 91.3% | 4.2% | CLI argument parsing edge cases, subcommand dispatch errors (~47 missing lines) |
| `core/chart_builder.py` | 91.3% | 4.2% | Empty-dataset guards, axis-formatting fallbacks (~33 missing lines) |
| `core/stats_utils.py` | 90.2% | 5.3% | Percentile calculation with single-element arrays, division-by-zero guards (~13 missing lines) |

**Estimated effort:** 1–2 weeks

---

## Tier 3 — Significant Effort (> 5% gap or untested)

### Go

| Package | Current | Gap | Challenge |
|---------|---------|-----|-----------|
| `internal/api` | 88.2% | 7.3% | Label CRUD handlers (80–84%), `handleExport` (68.4%), `sendCommandHandler` (66.7%) — needs more HTTP handler tests with mock DB |
| `internal/config` | 74.7% | 20.8% | 40+ `Get*` accessor functions at 0% — trivial to test but high count; `LoadTuningConfig` (90%), `MustLoadDefaultConfig` (80%) |

### Python

| Module | Current | Gap | Challenge |
|--------|---------|-----|-----------|
| `core/tex_environment.py` | 87.5% | 8.0% | LaTeX environment detection/fallback (~5 missing lines, needs mock filesystem) |
| `core/zip_utils.py` | 86.4% | 9.1% | Zip creation error paths, large-file handling (~17 missing lines) |

### Go `cmd/` Packages

| Package | Current | Gap | Challenge |
|---------|---------|-----|-----------|
| `cmd/tools/backfill_ring_elevations` | 43.3% | 52.2% | `RunBackfill` (0%), `main` (0%) — needs DB fixture + CLI harness |
| `cmd/deploy` | 7.2% | 88.3% | 80+ functions at 0% — SSH operations, service management, system administration; requires SSH mock/fake or integration test harness |
| `cmd/radar` | 5.4% | 90.1% | `main` (0%), sweep/labelling stubs (0%), `configurePDFLaTeXFlow` (50%) — sensor initialisation, deeply integrated |
| `cmd/sweep` | 0% | 95.5% | No test files at all |
| `cmd/tools/gen-vrlog` | 0% | 95.5% | No test files at all |
| `cmd/tools/pcap-analyse` | 0% | 95.5% | No test files at all |
| `cmd/tools/visualiser-server` | 0% | 95.5% | No test files at all |
| `cmd/transit-backfill` | 0% | 95.5% | No test files at all |

**Estimated effort:** 3–6 weeks

---

## Recommended Execution Order

### Phase 1: Raise the Floor (weeks 1–2)

1. **`internal/config`** — Write table-driven tests for all 40+ `Get*` accessors.
   Each accessor reads one field from the config struct; a single
   `TestGetAccessors` with a sub-test per field closes the entire 20.8% gap.

2. **Tier 1 packages** — Pick off every package within 2% of the target.
   These are small, self-contained PRs with clear scope.

3. **Web files** — `sweep_dashboard.js` and `api.ts` each need fewer than
   10 additional test assertions.

### Phase 2: Strengthen the Core (weeks 2–4)

4. **`internal/db`** — Add migration-path tests (`MigrateForce`, `BaselineAtVersion`)
   and error-injection tests for `NewDBWithMigrationCheck` / `withDB`.

5. **`internal/api`** — Expand HTTP handler tests for label CRUD, export, and
   command dispatch. Use existing `httptest` patterns from the package.

6. **`internal/lidar/*`** sub-packages — Work through Tier 2 packages by
   descending gap size. Most uncovered code is error handling and
   edge-case branches in algorithms already well-tested on the happy path.

7. **Python Tier 2** — `cli/main.py`, `chart_builder.py`, `stats_utils.py`,
   `report_sections.py`.

### Phase 3: CLI and Integration (weeks 4–8)

8. **`cmd/deploy`** — Introduce an SSH executor interface + fake for unit tests.
   Break deployment operations into testable steps behind the interface.
   This is the largest single effort item.

9. **`cmd/radar`** — Extract testable logic from `main()` into helper functions.
   Test helpers individually; accept that `main()` itself stays at low coverage.

10. **Remaining `cmd/` packages** — Add basic CLI-invocation tests
    (`flag.Parse`, argument validation, `--help` output) for the five
    untested tools. Prioritise `cmd/sweep` since it shares logic with
    `internal/lidar/sweep`.

11. **Python Tier 3** — `zip_utils.py` and `tex_environment.py` via filesystem
    mocking.

---

## Strategies for Hard-to-Test Code

### SSH/Remote Operations (`cmd/deploy`)
- **Extract an executor interface** (`Executor` with `Run`, `RunSudo`,
  `WriteFile`, `CopyFile` methods).
- Implement a `FakeExecutor` that records calls and returns scripted responses.
- Test all deployment logic against the fake; integration-test the real
  executor separately (CI with a local SSH server container).

### Database Error Paths (`internal/db`)
- Use `testutil.NewTestDB()` with deliberate schema corruption to trigger
  migration errors.
- Inject write failures via `PRAGMA journal_mode=OFF` + disk-full simulation.
- Test `withDB` with a closed `*sql.DB` to cover the error branch.

### LiDAR Monitor Handlers (`internal/lidar/monitor`)
- Use `httptest.NewServer` with the monitor's mux to test HTTP handlers.
- Create a `FakeBackend` implementing `ClientBackend` for PCAP replay and
  auto-tune handlers.

### Config Accessors (`internal/config`)
- Single table-driven test: populate a `TuningConfig` struct with known values,
  call every `Get*` function, assert against expected output.
- Use `go generate` or reflection to ensure new accessors are not forgotten.

### Python CLI (`cli/main.py`)
- Use `click.testing.CliRunner` (or equivalent) to invoke subcommands
  with controlled arguments.
- Mock external dependencies (`subprocess`, `shutil`) for deterministic tests.

---

## Coverage Infrastructure Improvements

1. **Raise Codecov target** from 1% to 95.5% (in `codecov.yml`), with a
   ramp schedule: 90% → 92% → 95.5% over three milestones.

2. **Add per-package threshold enforcement** in CI via
   `go tool cover -func` + a script that fails if any package drops below
   the target.

3. **Web coverage thresholds** in `jest.config.js` — raise from 90% to 95.5%
   for `web/src/lib/`.

4. **Python coverage threshold** — add `--cov-fail-under=95.5` to the pytest
   invocation in CI.

5. **Coverage-gated PR checks** — configure Codecov (or CI script) to block
   merges when any flag drops below 95.5%.

---

## Packages Already at Target (no action needed)

| Package | Coverage |
|---------|----------|
| `internal/fsutil` | 99.0% |
| `internal/lidar/debug` | 100.0% |
| `internal/lidar/l1packets/network` | 97.3% |
| `internal/lidar/l4perception` | 97.7% |
| `internal/monitoring` | 100.0% |
| `internal/testutil` | 100.0% |
| `internal/timeutil` | 95.5% |
| `internal/units` | 100.0% |

### Borderline (just below target — 1–2 tests needed)

| Package | Coverage | Gap |
|---------|----------|-----|
| `internal/lidar/l1packets/parse` | 95.1% | 0.4% |

---

## Estimated Total Effort

| Phase | Scope | Effort |
|-------|-------|--------|
| Phase 1 | Quick wins + config accessors | 2–3 days |
| Phase 2 | Core internal packages + Python | 1–2 weeks |
| Phase 3 | CLI packages + integration tests | 3–6 weeks |
| **Total** | **All packages ≥ 95.5%** | **5–9 weeks** |

The biggest single investment is `cmd/deploy` (80+ untested functions requiring
an SSH executor fake). If `cmd/` packages are excluded from the 95.5% target
(they are separately flagged in Codecov as `go-cli`), total effort drops to
approximately 2–3 weeks.
