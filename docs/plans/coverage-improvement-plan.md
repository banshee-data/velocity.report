# Coverage Improvement Plan: 95.5% Target

Status: Planned

**Goal:** Raise every `internal/`, web, Python, and macOS package/module/file
to ≥ 95.5% line coverage.

**Scope:** `cmd/` packages are excluded from the 95.5% target (they are
thin CLI wrappers tracked separately in Codecov as `go-cli`). However,
any testable business logic currently in `cmd/` must be extracted into
`internal/` where it falls under the target. See
[§ cmd/ Logic Extraction](#cmd-logic-extraction) below.

**Baseline measured:** 2026-02-20

---

## Current State Summary

| Component            | Overall | Files ≥ 95.5% | Files < 95.5%   |
| -------------------- | ------- | ------------- | --------------- |
| Go `internal/`       | 90.3%   | 7             | 18              |
| Go `cmd/` (excluded) | 18.6%   | 0             | 3 (+5 untested) |
| Web (statements)     | 96.0%   | 9 of 11       | 2               |
| Python               | 93.6%   | 9 of 19       | 10              |
| macOS Swift          | ~85%\*  | —             | —               |

\* macOS coverage is estimated from CI smoke tests; full XCTest coverage
requires a macOS runner. See [§ macOS Swift App](#macos-swift-app) below.

---

## Tier 1 — Quick Wins (< 2% gap, ~1–3 tests each)

These packages need only a handful of additional test cases.

### Go

| Package                   | Current | Gap  | What to Test                                                |
| ------------------------- | ------- | ---- | ----------------------------------------------------------- |
| `internal/serialmux`      | 94.9%   | 0.6% | Edge cases in port option validation                        |
| `internal/lidar` (root)   | 94.4%   | 1.1% | Uncovered branches in alias helpers                         |
| `internal/lidar/l5tracks` | 94.1%   | 1.4% | Track expiry and miss-count edge paths                      |
| `internal/httputil`       | 93.9%   | 1.6% | Error paths in `Get`/`Post`/`WriteJSON` helpers (mock HTTP) |
| `internal/deploy`         | 93.4%   | 2.1% | `RunSudo` error branch, `ParseSSHConfigFrom` edge cases     |

### Web

| File                 | Current | Gap  | What to Test                                    |
| -------------------- | ------- | ---- | ----------------------------------------------- |
| `sweep_dashboard.js` | 95.1%   | 0.4% | ~7 uncovered branches in chart-update paths     |
| `api.ts`             | 94.8%   | 0.7% | ~5 uncovered error/retry branches in API client |

### Python

| Module                       | Current | Gap  | What to Test                                     |
| ---------------------------- | ------- | ---- | ------------------------------------------------ |
| `core/document_builder.py`   | 94.8%   | 0.7% | LaTeX preamble fallback paths                    |
| `core/map_utils.py`          | 94.5%   | 1.0% | Missing-data guard clauses in map tile fetching  |
| `core/pdf_generator.py`      | 94.3%   | 1.2% | Error handling in subprocess calls               |
| `core/dependency_checker.py` | 94.0%   | 1.5% | Version-parsing edge cases, missing-binary paths |

**Estimated effort:** 2–3 days

---

## Tier 2 — Moderate Work (2–5% gap, ~5–15 tests each)

### Go

| Package                              | Current | Gap  | Key Uncovered Functions                                                                                         |
| ------------------------------------ | ------- | ---- | --------------------------------------------------------------------------------------------------------------- |
| `internal/lidar/l3grid`              | 93.2%   | 2.3% | `serializeGrid` (66.7%), `NewBackgroundManager` (72.2%), `WithForeground*` option funcs                         |
| `internal/lidar/l6objects`           | 92.5%   | 3.0% | `birdConfidence` (66.7%), `NewTrackClassifierWithMinObservations` (66.7%), classification edge cases            |
| `internal/lidar/storage/sqlite`      | 92.1%   | 3.4% | Transaction rollback paths, bulk-insert error recovery                                                          |
| `internal/lidar/visualiser`          | 92.0%   | 3.5% | Protobuf serialisation edge cases, frame rendering                                                              |
| `internal/lidar/sweep`               | 91.7%   | 3.8% | Parameter combination overflow guards, result aggregation                                                       |
| `internal/lidar/l2frames`            | 91.6%   | 3.9% | Frame assembly timeouts, malformed-packet recovery                                                              |
| `internal/lidar/visualiser/recorder` | 91.6%   | 3.9% | File-rotation, write-error handling                                                                             |
| `internal/lidar/adapters`            | 91.4%   | 4.1% | `Evaluate` (0%), `NewGroundTruthEvaluator` (0%), `SetSequenceID` (0%)                                           |
| `internal/lidar/pipeline`            | 91.4%   | 4.1% | Pipeline shutdown ordering, error propagation                                                                   |
| `internal/db`                        | 90.8%   | 4.7% | `NewDBWithMigrationCheck` (77.5%), `DetectSchemaVersion` (82.6%), `withDB` (20%), migration force/rollback      |
| `internal/lidar/monitor`             | 90.4%   | 5.1% | PCAP replay handlers (0%), auto-tune suspend/resume (0%), `handleDataSource` (65%), `WaitForGridSettle` (70.6%) |
| `internal/security`                  | 90.5%   | 5.0% | Path-traversal rejection for edge-case payloads                                                                 |

### Python

| Module                    | Current | Gap  | What to Test                                                                                   |
| ------------------------- | ------- | ---- | ---------------------------------------------------------------------------------------------- |
| `core/report_sections.py` | 91.8%   | 3.7% | Optional section rendering when data is absent                                                 |
| `cli/main.py`             | 91.3%   | 4.2% | CLI argument parsing edge cases, subcommand dispatch errors (~47 missing lines)                |
| `core/chart_builder.py`   | 91.3%   | 4.2% | Empty-dataset guards, axis-formatting fallbacks (~33 missing lines)                            |
| `core/stats_utils.py`     | 90.2%   | 5.3% | Percentile calculation with single-element arrays, division-by-zero guards (~13 missing lines) |

**Estimated effort:** 1–2 weeks

---

## Tier 3 — Significant Effort (> 5% gap or untested)

### Go

| Package           | Current | Gap   | Challenge                                                                                                                       |
| ----------------- | ------- | ----- | ------------------------------------------------------------------------------------------------------------------------------- |
| `internal/api`    | 88.2%   | 7.3%  | Label CRUD handlers (80–84%), `handleExport` (68.4%), `sendCommandHandler` (66.7%) — needs more HTTP handler tests with mock DB |
| `internal/config` | 74.7%   | 20.8% | 40+ `Get*` accessor functions at 0% — trivial to test but high count; `LoadTuningConfig` (90%), `MustLoadDefaultConfig` (80%)   |

### Python

| Module                    | Current | Gap  | Challenge                                                                      |
| ------------------------- | ------- | ---- | ------------------------------------------------------------------------------ |
| `core/tex_environment.py` | 87.5%   | 8.0% | LaTeX environment detection/fallback (~5 missing lines, needs mock filesystem) |
| `core/zip_utils.py`       | 86.4%   | 9.1% | Zip creation error paths, large-file handling (~17 missing lines)              |

### Go `cmd/` — Logic Extraction to `internal/`

<a id="cmd-logic-extraction"></a>

`cmd/` packages are excluded from the 95.5% coverage target. Instead,
testable business logic must be extracted into `internal/` packages where
it is covered by the target. The remaining `cmd/` code should be thin CLI
wiring (flag parsing, `main()`, output formatting).

| Package                       | Testable LOC | Target `internal/` Package           | Extraction Scope                                                                                                                                        |
| ----------------------------- | ------------ | ------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `cmd/deploy`                  | ~2,500       | `internal/deploy` (exists)           | **HIGH** — `Installer`, `Upgrader`, `Rollback`, `Fixer`, `Monitor` are pure business logic. Unify the two `Executor` implementations (cmd vs internal). |
| `cmd/radar`                   | ~200         | `internal/config`                    | **MEDIUM** — `resolvePrecompiledTeXRoot()`, `configurePDFLaTeXFlow()`, environment-config resolution.                                                   |
| `cmd/tools` (scan_transits)   | ~65          | `internal/db`                        | **MEDIUM** — `findTransitGaps()` is pure SQL+result-processing; belongs alongside `TransitWorker`.                                                      |
| `cmd/tools/pcap-analyse`      | ~300         | `internal/lidar/pipeline`            | **LOW** — PCAP processing + ML export pipeline; tightly coupled to build tags.                                                                          |
| `cmd/sweep`                   | ~150         | (already in `internal/lidar/sweep`)  | LOW — only `toFloat64()` helper remains in cmd.                                                                                                         |
| `cmd/transit-backfill`        | ~30          | (already delegates to `internal/db`) | NONE — already a thin wrapper.                                                                                                                          |
| `cmd/tools/gen-vrlog`         | ~100         | `internal/lidar/visualiser/recorder` | LOW — synthetic VRLog generation.                                                                                                                       |
| `cmd/tools/visualiser-server` | ~150         | `internal/lidar/visualiser`          | LOW — gRPC server modes.                                                                                                                                |

**Extraction strategy:**

1. Move business-logic types and functions into the corresponding
   `internal/` package, keeping only flag parsing and `main()` in `cmd/`.
2. Write unit tests for the extracted code in `internal/` (now under the
   95.5% target).
3. The `cmd/` package itself needs only a minimal smoke test
   (`--help` exits 0, invalid flags exit non-zero).
4. **Priority:** `cmd/deploy` → `cmd/radar` → `cmd/tools` (by LOC impact).

### macOS Swift App

<a id="macos-swift-app"></a>

The macOS LiDAR visualiser at `tools/visualiser-macos/` is a SwiftUI +
Metal application (~5,900 LOC across 11 source files) with an existing
test suite (14 unit-test files, 2 UI-test files). Coverage is collected
via `make test-mac-cov` using `xcodebuild -enableCodeCoverage YES` and
reported to Codecov under the `mac` flag.

**Current state:** ~85% estimated (CI runs build-smoke only; full XCTest
coverage requires a macOS runner with Metal support).

| Source File                         | LOC    | Test File                                         | Key Gaps                                                                      |
| ----------------------------------- | ------ | ------------------------------------------------- | ----------------------------------------------------------------------------- |
| `AppState.swift`                    | ~800   | `AppStateTests.swift`, `CoverageBoostTests.swift` | Deep playback state transitions, error-recovery paths                         |
| `MetalRenderer.swift`               | ~1,300 | `MetalRendererTests.swift`                        | GPU pipeline creation fallbacks, draw-call edge cases (requires Metal device) |
| `ContentView.swift`                 | ~1,800 | `ContentViewTests.swift`                          | Complex SwiftUI view hierarchy; limited testability without ViewInspector     |
| `VisualiserClient.swift`            | ~580   | `VisualiserClientTests.swift`                     | gRPC reconnection, stream-error handling                                      |
| `RunTrackLabelAPIClient.swift`      | ~345   | `RunTrackLabelAPIClientTests.swift`               | Error responses, network timeout paths                                        |
| `Models.swift`                      | ~340   | `ModelM35Tests.swift`, `ModelExtendedTests.swift` | Edge cases in protobuf decoding                                               |
| `CompositePointCloudRenderer.swift` | ~320   | `CompositePointCloudRendererTests.swift`          | Cache invalidation, split-stream transitions                                  |
| `RunBrowserView.swift`              | ~180   | `RunBrowserViewTests.swift`                       | File-picker interaction (UI test)                                             |
| `LabelAPIClient.swift`              | ~160   | `LabelAPIClientTests.swift`                       | Server error mapping, auth header injection                                   |
| `VelocityVisualiserApp.swift`       | ~107   | —                                                 | Menu commands, keyboard shortcuts (UI test territory)                         |
| `RunBrowserState.swift`             | ~87    | `RunBrowserStateTests.swift`                      | Already well-tested                                                           |

**Strategy to reach 95.5%:**

1. **Expand unit tests for `AppState`** — Cover remaining playback
   transitions (seek-past-end, rate changes during pause, disconnect
   during replay). The `CoverageBoostTests.swift` file already targets
   these gaps; extend it.

2. **Network error injection for API clients** — `LabelAPIClient` and
   `RunTrackLabelAPIClient` need tests for HTTP 4xx/5xx responses,
   timeout, and malformed JSON. Use `URLProtocol` subclass to intercept
   requests.

3. **Metal renderer** — GPU-dependent code is inherently hard to unit-test.
   Extract pure-logic helpers (matrix maths, colour mapping, buffer
   sizing) into testable structs separate from `MTKViewDelegate`. Accept
   that draw-call paths stay at lower coverage.

4. **ContentView** — Large SwiftUI view with limited unit-test surface.
   Consider adopting [ViewInspector](https://github.com/nalexn/ViewInspector)
   for snapshot-free view testing, or extract complex logic into
   `@Observable` view models that can be tested without a view hierarchy.

5. **gRPC client reconnection** — Inject a mock `GRPCChannel` to test
   stream lifecycle (connect → receive frames → error → reconnect).

6. **CI coverage collection** — Upgrade `mac-ci.yml` to run full XCTest
   (not just build-smoke) on a macOS runner and upload `.xcresult`
   coverage to Codecov.

**Estimated effort:** 1–2 weeks (assuming macOS runner availability).

**Estimated effort (Tier 3 total):** 3–6 weeks

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

### Phase 3: Extract cmd/ Logic + macOS (weeks 4–8)

8. **`cmd/deploy` → `internal/deploy`** — Extract `Installer`, `Upgrader`,
   `Rollback`, `Fixer`, and `Monitor` into `internal/deploy`. Unify
   `Executor` implementations. Write tests against extracted code (~2,500
   LOC gaining coverage under the 95.5% target).

9. **`cmd/radar` → `internal/config`** — Extract `resolvePrecompiledTeXRoot`,
   `configurePDFLaTeXFlow`, and environment-config helpers. Test in
   `internal/config`.

10. **`cmd/tools` → `internal/db`** — Move `findTransitGaps()` to
    `internal/db` alongside `TransitWorker`.

11. **macOS Swift app** — Expand `AppState` tests, add network-error
    injection for API clients, extract testable Metal helpers. Upgrade
    CI to run full XCTest and upload coverage.

12. **Python Tier 3** — `zip_utils.py` and `tex_environment.py` via
    filesystem mocking.

---

## Strategies for Hard-to-Test Code

### SSH/Remote Operations (`cmd/deploy` → `internal/deploy`)

- **Extract an executor interface** (`Executor` with `Run`, `RunSudo`,
  `WriteFile`, `CopyFile` methods) — `internal/deploy` already has a
  partial implementation; unify with the `cmd/deploy` version.
- Implement a `FakeExecutor` that records calls and returns scripted responses.
- Test all deployment logic (Installer, Upgrader, Fixer, Monitor) against
  the fake in `internal/deploy`; integration-test the real executor
  separately (CI with a local SSH server container).
- `cmd/deploy/main.go` remains a thin CLI wrapper — no 95.5% target.

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

### macOS Metal Renderer (`MetalRenderer.swift`)

- **Extract pure-logic helpers** (matrix maths, colour mapping, buffer
  sizing) into standalone structs/functions that can be tested without a
  Metal device.
- Accept that `MTKViewDelegate.draw(in:)` and GPU pipeline creation
  remain at lower coverage — these require a real Metal device.
- Use `MTLCreateSystemDefaultDevice()` availability checks in tests to
  skip GPU-dependent assertions on CI runners without Metal.

### macOS SwiftUI Views (`ContentView.swift`)

- Extract complex logic into `@Observable` view models testable
  without a view hierarchy.
- Consider [ViewInspector](https://github.com/nalexn/ViewInspector) for
  snapshot-free view testing if deeper view coverage is needed.
- Focus on testing the view models and state management rather than the
  SwiftUI view tree itself.

### macOS Network Clients (`LabelAPIClient`, `VisualiserClient`)

- Use a `URLProtocol` subclass to intercept HTTP requests and return
  scripted responses (4xx, 5xx, timeouts, malformed JSON).
- Inject a mock `GRPCChannel` to test gRPC stream lifecycle (connect →
  receive → error → reconnect).

---

## Coverage Infrastructure Improvements

1. **Raise Codecov target** from 1% to 95.5% (in `codecov.yml`), with a
   ramp schedule: 90% → 92% → 95.5% over three milestones. Apply to the
   `go`, `web`, `python`, and `mac` flags (not `go-cli`).

2. **Add per-package threshold enforcement** in CI via
   `go tool cover -func` + a script that fails if any `internal/` package
   drops below the target.

3. **Web coverage thresholds** in `jest.config.js` — raise from 90% to 95.5%
   for `web/src/lib/`.

4. **Python coverage threshold** — add `--cov-fail-under=95.5` to the pytest
   invocation in CI.

5. **macOS CI coverage** — upgrade `mac-ci.yml` to run full XCTest (not
   just build-smoke) on a macOS runner and upload `.xcresult` coverage to
   Codecov via the `mac` flag.

6. **Coverage-gated PR checks** — configure Codecov (or CI script) to block
   merges when any in-scope flag drops below 95.5%.

---

## Packages Already at Target (no action needed)

| Package                            | Coverage |
| ---------------------------------- | -------- |
| `internal/fsutil`                  | 99.0%    |
| `internal/lidar/debug`             | 100.0%   |
| `internal/lidar/l1packets/network` | 97.3%    |
| `internal/lidar/l4perception`      | 97.7%    |
| `internal/monitoring`              | 100.0%   |
| `internal/testutil`                | 100.0%   |
| `internal/timeutil`                | 95.5%    |
| `internal/units`                   | 100.0%   |

### Borderline (just below target — 1–2 tests needed)

| Package                          | Coverage | Gap  |
| -------------------------------- | -------- | ---- |
| `internal/lidar/l1packets/parse` | 95.1%    | 0.4% |

---

## Estimated Total Effort

| Phase     | Scope                                   | Effort        |
| --------- | --------------------------------------- | ------------- |
| Phase 1   | Quick wins + config accessors           | 2–3 days      |
| Phase 2   | Core internal packages + Python         | 1–2 weeks     |
| Phase 3   | cmd/ extraction + macOS + Python Tier 3 | 3–6 weeks     |
| **Total** | **All in-scope packages ≥ 95.5%**       | **5–9 weeks** |

The biggest single investment is extracting `cmd/deploy` business logic
(~2,500 LOC) into `internal/deploy` with an SSH executor fake. The macOS
Swift app adds ~1–2 weeks depending on macOS runner availability in CI.

`cmd/` packages themselves remain out of scope for the 95.5% target — they
need only minimal smoke tests (`--help`, invalid-flag handling) once their
business logic has been extracted into `internal/`.
