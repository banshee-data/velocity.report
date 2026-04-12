# Quality coverage: 95.5% target

Active plan: [platform-quality-coverage-improvement-plan.md](../../plans/platform-quality-coverage-improvement-plan.md)

Tracking the programme to raise code coverage across all components to the 95.5% line-coverage target, with per-tier priorities and extraction strategies for hard-to-test code.

## Scope

Raise every `internal/`, web, Python, and macOS package/module/file to
≥ 95.5% line coverage. `cmd/` packages are excluded (thin CLI wrappers
tracked separately as `go-cli`). Testable business logic in `cmd/` must
be extracted into `internal/`.

## Current state

| Component            | Overall | Above target | Below target |
| -------------------- | ------- | ------------ | ------------ |
| Go `internal/`       | 90.3%   | 7 packages   | 18 packages  |
| Go `cmd/` (excluded) | 18.6%   | 0            | 8            |
| Web (statements)     | 96.0%   | 9 of 11      | 2            |
| Python               | 93.6%   | 9 of 19      | 10           |
| macOS Swift          | ~85%    | -            | -            |

## Tiered approach

### Tier 1: quick wins (< 2% gap)

Go: `serialmux` (94.9%), `lidar` root (94.4%), `l5tracks` (94.1%),
`httputil` (93.9%), `deploy` (93.4%).

Web: `sweep_dashboard.js` (95.1%), `api.ts` (94.8%).

Python: `document_builder.py` (94.8%), `map_utils.py` (94.5%),
`pdf_generator.py` (94.3%), `dependency_checker.py` (94.0%).

### Tier 2: moderate work (2–5% gap)

Go: `l3grid`, `l6objects`, `storage/sqlite`, `visualiser`, `sweep`,
`l2frames`, `adapters`, `pipeline`, `db`, `monitor`, `security`.

Python: `report_sections.py`, `cli/main.py`, `chart_builder.py`,
`stats_utils.py`.

### Tier 3: significant effort (> 5% gap)

Go: [internal/api](../../../internal/api) (88.2%), [internal/config](../../../internal/config) (74.7%; 40+ `Get*` at 0%).

Python: `tex_environment.py` (87.5%), `zip_utils.py` (86.4%).

## cmd/ logic extraction strategy

| Package                         | Testable LOC | Target `internal/`                          | Priority |
| ------------------------------- | ------------ | ------------------------------------------- | -------- |
| [cmd/radar](../../../cmd/radar) | ~200         | [internal/config](../../../internal/config) | MEDIUM   |
| [cmd/tools](../../../cmd/tools) | ~65          | [internal/db](../../../internal/db)         | MEDIUM   |

Extraction: Move business-logic types into `internal/`, keep only flag
parsing and `main()` in `cmd/`. Write unit tests against extracted code.

## macOS Swift strategy

1. Expand `AppState` unit tests for remaining playback transitions.
2. Network error injection for API clients via `URLProtocol`.
3. Extract pure-logic helpers from Metal renderer (matrices, colours,
   buffer sizing) into testable structs.
4. Consider ViewInspector for SwiftUI view testing.
5. Upgrade CI to run full XCTest and upload `.xcresult` coverage.

## Hard-to-Test code strategies

- **SSH/remote:** Extract `Executor` interface with `FakeExecutor`.
- **Database errors:** Deliberate schema corruption, closed `*sql.DB`.
- **LiDAR monitor:** `httptest.NewServer` with `FakeBackend`.
- **Config accessors:** Single table-driven test with sub-test per field.
- **Python CLI:** `click.testing.CliRunner` with mocked externals.

## Infrastructure improvements

1. Raise Codecov target from 1% → 90% → 92% → 95.5% (ramp schedule).
2. Per-package threshold enforcement via `go tool cover -func`.
3. Web coverage thresholds in `jest.config.js` raised to 95.5%.
4. Python `--cov-fail-under=95.5`.
5. macOS CI full XCTest.
6. Coverage-gated PR checks.

## Execution order

1. **Phase 1 (weeks 1–2):** [internal/config](../../../internal/config) accessors + all Tier 1.
2. **Phase 2 (weeks 2–4):** [internal/db](../../../internal/db), [internal/api](../../../internal/api), lidar
   sub-packages, Python Tier 2.
3. **Phase 3 (weeks 4–8):** [cmd/radar](../../../cmd/radar)
   extraction, macOS Swift, Python Tier 3.
