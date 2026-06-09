# Quality coverage: 95.5% target

Active plan: [platform-quality-coverage-improvement-plan.md](../../plans/platform-quality-coverage-improvement-plan.md)

Tracking the programme to raise code coverage across all components to the 95.5% line-coverage target, with per-tier priorities and extraction strategies for hard-to-test code.

## Scope

Raise every `internal/`, web, and macOS package/module/file to
≥ 95.5% line coverage. `cmd/` packages are excluded (thin CLI wrappers
tracked separately as `go-cli`). Testable business logic in `cmd/` must
be extracted into `internal/`.

## Measuring current coverage

Point-in-time coverage numbers are not recorded here — they drift with every
commit. Derive them on demand:

- **Go:** `make test-go-cov` (writes `coverage.html`; `cmd/` is excluded).
- **Web:** `pnpm --dir web test -- --coverage`.
- **macOS:** XCTest with `.xcresult` coverage export.

## Tiered approach

### Tier 1: quick wins (< 2% gap)

Go: `serialmux`, `lidar` root, `l5tracks`, `httputil`, `deploy`.

Web: `sweep_dashboard.js`, `api.ts`.

### Tier 2: moderate work (2–5% gap)

Go: `l3grid`, `l6objects`, `storage/sqlite`, `visualiser`, `sweep`,
`l2frames`, `adapters`, `pipeline`, `db`, `monitor`, `security`.

### Tier 3: significant effort (> 5% gap)

Go: [internal/api](../../../internal/api), [internal/config](../../../internal/config) (large block of untested `Get*` accessors).

## cmd/ logic extraction strategy

| Package                                             | Testable LOC | Target `internal/`                          | Priority |
| --------------------------------------------------- | ------------ | ------------------------------------------- | -------- |
| [internal/cmd/server](../../../internal/cmd/server) | ~200         | [internal/config](../../../internal/config) | MEDIUM   |
| [cmd/tools](../../../cmd/tools)                     | ~65          | [internal/db](../../../internal/db)         | MEDIUM   |

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

## Infrastructure improvements

1. Raise Codecov target from 1% → 90% → 92% → 95.5% (ramp schedule).
2. Per-package threshold enforcement via `go tool cover -func`.
3. Web coverage thresholds in `jest.config.js` raised to 95.5%.
4. macOS CI full XCTest.
5. Coverage-gated PR checks.

## Execution order

1. **Phase 1 (weeks 1–2):** [internal/config](../../../internal/config) accessors + all Tier 1.
2. **Phase 2 (weeks 2–4):** [internal/db](../../../internal/db), [internal/api](../../../internal/api), lidar
   sub-packages.
3. **Phase 3 (weeks 4–8):** [internal/cmd/server](../../../internal/cmd/server)
   extraction, macOS Swift.
