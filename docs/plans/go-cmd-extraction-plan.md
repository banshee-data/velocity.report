# Go cmd/ business logic extraction plan (v0.5.2)

- **Status:** Draft
- **Layers:** Cross-cutting (Go server, LiDAR pipeline, configuration)
- **Target:** v0.5.2; extract before `cmd/radar/radar.go` grows past 1,500 LOC
- **Companion plans:**
  [go-god-file-split-plan.md](go-god-file-split-plan.md) (Complete),
  [go-codebase-structural-hygiene-plan.md](go-codebase-structural-hygiene-plan.md) (Active)
- **Canonical:** [go-package-structure.md](../platform/architecture/go-package-structure.md)

## Motivation

The god-file-split plan cleaned the `internal/` tree. The structural hygiene plan
closed import boundaries and fixed error handling. But `cmd/` still holds ~850 lines
of business logic, data transformation, and testable algorithms that belong in
`internal/`.

Code locked inside `package main` cannot be unit-tested without building the full
binary. It cannot be reused by other binaries. It cannot be reviewed in isolation.
Every new consumer must reimplement or duplicate.

`cmd/radar/radar.go` is 1,194 lines. Of those, roughly 350 are business logic
(adapters, CLI dispatch, config resolution) rather than flag parsing and component
wiring. If left, the file will cross 1,500 LOC by v0.6.0 as HINT and sweep features
expand.

## Current state

### Extraction targets by file

| File                                             |       LOC | Extractable LOC | Contents                                    |
| ------------------------------------------------ | --------: | --------------: | ------------------------------------------- |
| `cmd/radar/radar.go`                             |     1,194 |            ~350 | Adapters, transits CLI, TeX flow config     |
| `cmd/radar/lidar_helpers.go`                     |       275 |            ~250 | 24 pure functions, 8 test-double interfaces |
| `cmd/radar/capabilities.go`                      |        75 |              75 | Thread-safe capabilities provider           |
| `cmd/tools/config-migrate/main.go`               |       233 |            ~180 | Legacy config struct + migration function   |
| `cmd/tools/settling-eval/pcap.go`                |       182 |            ~140 | PCAP settling evaluation orchestration      |
| `cmd/tools/backfill_ring_elevations/backfill.go` |        86 |             ~60 | Raw SQL backfill queries                    |
| **Total**                                        | **2,045** |      **~1,055** |                                             |

### What stays in cmd/

- Flag parsing and `main()` wiring (~800 lines in `radar.go`)
- Signal handling and graceful shutdown
- Component construction order
- `cmd/sweep/main.go` (265 lines): sweep CLI orchestration
- `cmd/velocity-ctl/` (121 lines): already delegates to `internal/ctl`
- Small tools under 60 lines: `config-validate`, `gen-vrlog`, `vrlog-analyse`,
  `backfill_lidar_run_config`

## Design

Each extraction is a mechanical file move. No functional changes. No new abstractions
unless an existing interface boundary requires one (e.g. `io.Writer` for testable
prompts). Tests pass unchanged after each move.

Destination packages follow the existing `internal/` structure:

| Source                                                | Destination                      | Rationale                                     |
| ----------------------------------------------------- | -------------------------------- | --------------------------------------------- |
| `capabilitiesProvider`                                | `internal/api/`                  | Already imports only `internal/api`           |
| LiDAR validation helpers                              | `internal/config/`               | Tuning validation belongs with tuning types   |
| LiDAR networking helpers                              | `internal/lidar/server/`         | Port validation belongs with the UDP listener |
| PCAP/visualiser callbacks                             | `internal/lidar/server/`         | Callback factories used by server pipeline    |
| `backgroundManagerBridge`                             | `internal/lidar/l9endpoints/`    | Bridges l3grid → l9endpoints types            |
| HINT adapters                                         | `internal/lidar/sweep/`          | Bridge sqlite stores to sweep interfaces      |
| `runTransitsCommand` dispatch                         | `internal/db/`                   | Sits alongside `TransitCLI`                   |
| `resolvePrecompiledTeXRoot` + `configurePDFLaTeXFlow` | `internal/config/`               | Pure config resolution                        |
| `legacyTuningConfig` + `migrateLegacyConfig`          | `internal/config/`               | Migration logic is config domain              |
| `RunBackfillDB`                                       | `internal/lidar/storage/sqlite/` | SQL belongs with its schema                   |
| `runPCAPEval`                                         | `internal/lidar/l3grid/`         | Background settling evaluation                |

No new packages are created. All destinations already exist.

## Scope

### Item 1: extract `capabilitiesProvider` to `internal/api/`

**Summary:** Move the thread-safe capabilities provider (75 LOC) into the package
whose types it already wraps.

**Steps:**

1. Move `capabilitiesProvider` struct and methods to `internal/api/capabilities.go`
2. Export the type as `CapabilitiesProvider`
3. Update `cmd/radar/radar.go` to use `api.NewCapabilitiesProvider()`
4. Add unit tests for state transitions (disabled → starting → ready → error)

**Milestone:** v0.5.2

### Item 2: extract `lidar_helpers.go` to existing `internal/` packages

**Summary:** Split 275 LOC of pure functions and interfaces across their natural
homes in `internal/config/` and `internal/lidar/server/`.

**Steps:**

1. Move `validateSupportedTuning`, `ensureSupportedTuning` to `internal/config/tuning_validate.go`
2. Move `validateLidarNetworkingFlags`, `validateOptionalLidarPortFlag`,
   `ensureValidLidarNetworkingFlags`, `ensureValidForwardMode` to
   `internal/lidar/server/validate.go`
3. Move `tuningHashOrWarn`, config hash helpers to `internal/config/`
4. Move `mustLoadValidatedPandarConfig`, `ringElevationLogMessage` to
   `internal/lidar/server/`
5. Move PCAP/visualiser callbacks (`handlePCAPStartedVisualiser`,
   `publishPCAPProgress`, `pcapProgressCallback`) to `internal/lidar/server/`
6. Move associated interfaces to the package that owns the consuming functions
7. Move `isNilHelperTarget` to the package that uses it (or inline if only one caller)
8. Add unit tests for each moved function

**Milestone:** v0.5.2

### Item 3: extract adapters from `radar.go` to `internal/lidar/`

**Summary:** Move ~170 LOC of struct-mapping adapters and the `hintRunCreator`
orchestration out of `package main`.

**Steps:**

1. Move `backgroundManagerBridge` to `internal/lidar/l9endpoints/bg_bridge.go`
2. Move `hintSceneAdapter` and `hintLabelAdapter` to `internal/lidar/sweep/hint_adapters.go`
3. Move `hintRunCreator` to `internal/lidar/sweep/hint_run_creator.go`
4. Update `cmd/radar/radar.go` to construct the exported adapter types
5. Add unit tests for the struct field mapping in each adapter
6. Add unit test for `hintRunCreator.CreateSweepRun` parameter construction

**Milestone:** v0.5.2

### Item 4: extract `runTransitsCommand` dispatch to `internal/db/`

**Summary:** Move ~145 LOC of transits CLI dispatch into the package that already
owns `TransitCLI`.

**Steps:**

1. Create `internal/db/transit_cli_dispatch.go` with a `RunTransitsCommand` function
2. Accept `io.Reader` and `io.Writer` for confirmation prompts (testability)
3. Move the `analyse`/`delete`/`migrate`/`rebuild` dispatch logic
4. Reduce `cmd/radar/radar.go` to a one-line call
5. Add unit tests for each subcommand dispatch path
6. Add unit tests for confirmation prompt handling (accept/reject/EOF)

**Milestone:** v0.5.2

### Item 5: extract teX flow configuration to `internal/config/`

**Summary:** Move `resolvePrecompiledTeXRoot` and `configurePDFLaTeXFlow` (~50 LOC)
into the config package.

**Steps:**

1. Move both functions to `internal/config/tex.go`
2. Export as `ResolvePrecompiledTeXRoot` and `ConfigurePDFLaTeXFlow`
3. Update `cmd/radar/radar.go` to call `config.ConfigurePDFLaTeXFlow()`
4. Add unit tests for path resolution edge cases

**Milestone:** v0.5.2

### Item 6: extract `config-migrate` logic to `internal/config/`

**Summary:** Move `legacyTuningConfig` struct and `migrateLegacyConfig` function
(~180 LOC) out of the tool binary.

**Steps:**

1. Move `legacyTuningConfig` and `migrateLegacyConfig` to `internal/config/migrate.go`
2. Export as `LegacyTuningConfig` and `MigrateLegacyConfig`
3. Reduce `cmd/tools/config-migrate/main.go` to flag parsing + one function call
4. Add unit tests for field mapping from flat to nested schema

**Milestone:** v0.5.2

### Item 7: extract `backfill_ring_elevations` SQL to `internal/lidar/storage/sqlite/`

**Summary:** Move `RunBackfillDB` (~60 LOC of raw SQL) into the package that owns
the `lidar_bg_snapshot` table schema.

**Steps:**

1. Move `RunBackfillDB` to `internal/lidar/storage/sqlite/backfill.go`
2. Reduce `cmd/tools/backfill_ring_elevations/backfill.go` to DB-open + one call
3. Add unit test exercising the backfill on a test database

**Milestone:** v0.5.2

### Item 8: extract `settling-eval` orchestration to `internal/lidar/l3grid/`

**Summary:** Move `runPCAPEval` and `backgroundConfigFromTuningConfig` (~140 LOC)
into the background grid package.

**Steps:**

1. Move `backgroundConfigFromTuningConfig` to `internal/config/` (config adapter)
2. Move `runPCAPEval` to `internal/lidar/l3grid/eval.go`
3. Reduce `cmd/tools/settling-eval/pcap.go` to flag parsing + one call
4. Add unit test for the config adapter function

**Milestone:** v0.5.2

## Dependencies

- Go god-file-split plan must remain complete (no regression into merged files)
- Structural hygiene plan's `database/sql` boundary enforcement must stay green
- No external dependencies

## Risks

| Risk                                              | Likelihood | Impact | Mitigation                                                                                             |
| ------------------------------------------------- | ---------- | ------ | ------------------------------------------------------------------------------------------------------ |
| Circular import from adapter moves                | Medium     | Low    | Each destination is already downstream of its dependency; verify with `go build ./...` after each move |
| `hintRunCreator` couples `sweep.Runner` internals | Low        | Medium | Keep adapter as thin bridge; do not absorb Runner lifecycle                                            |
| Large PR blocks review                            | Medium     | Low    | Split into 3–4 PRs following the item groupings below                                                  |

## Suggested PR grouping

| PR   | Items                                                 | Estimated LOC moved |
| ---- | ----------------------------------------------------- | ------------------: |
| PR A | 1 (capabilities) + 5 (TeX flow)                       |                ~125 |
| PR B | 2 (lidar helpers)                                     |                ~250 |
| PR C | 3 (adapters) + 4 (transits dispatch)                  |                ~315 |
| PR D | 6 (config-migrate) + 7 (backfill) + 8 (settling-eval) |                ~380 |

## Checklist

### Outstanding

- [ ] Item 1: Extract `capabilitiesProvider` to `internal/api/` (`S`)
- [ ] Item 2: Extract `lidar_helpers.go` to `internal/config/` + `internal/lidar/server/` (`M`)
- [ ] Item 3: Extract adapters from `radar.go` to `internal/lidar/` (`M`)
- [ ] Item 4: Extract `runTransitsCommand` to `internal/db/` (`M`)
- [ ] Item 5: Extract TeX flow config to `internal/config/` (`S`)
- [ ] Item 6: Extract `config-migrate` logic to `internal/config/` (`S`)
- [ ] Item 7: Extract `backfill_ring_elevations` SQL to `internal/lidar/storage/sqlite/` (`S`)
- [ ] Item 8: Extract `settling-eval` orchestration to `internal/lidar/l3grid/` (`S`)

### Accepted residuals (no action planned)

- [ ] `cmd/radar/radar.go` will still be ~800 LOC after extraction: this is legitimate
      `main()` wiring (flag parsing, component construction, shutdown) and does not
      warrant further splitting
- [ ] `cmd/sweep/main.go` (265 LOC): CLI orchestration, not business logic
- [ ] `cmd/velocity-ctl/upgrade.go`: `loadIncludePrereleases` (30 LOC) is borderline
      but too small to justify a move
