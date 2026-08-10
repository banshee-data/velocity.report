```
  ░███▒  ▓██▓  █▒  ▒█ █▒  ▒█   ██   ██   █ ████▒   ▓███▒
 ░█▒ ░█ ▒█  █▒ ██  ██ ██  ██   ██   ██░  █ █  ▒█░ █▓  ░█
 █▒     █░  ░█ ██░░██ ██░░██  ▒██▒  █▒▓  █ █   ▒█ █
 █      █    █ █▒▓▓▒█ █▒▓▓▒█  ▓▒▒▓  █ █  █ █    █ █▓░
 █      █    █ █ ██ █ █ ██ █  █░░█  █ ▓▓ █ █    █  ▓██▓
 █      █    █ █ █▓ █ █ █▓ █  █  █  █  █ █ █    █     ▓█
 █▒     █░  ░█ █    █ █    █ ▒████▒ █  ▓▒█ █   ▒█      █
 ░█▒ ░▓ ▒█  █▒ █    █ █    █ ▓▒  ▒▓ █  ░██ █  ▒█░ █░  ▓█
  ▒███▒  ▓██▓  █    █ █    █ █░  ░█ █   ██ ████▒  ▒████░
```

All make targets for building, testing, formatting, deploying, and operating velocity.report.

Run `make help` or `make` to see all available targets with descriptions.

## Naming convention

The project uses a consistent naming scheme: `<action>-<subsystem>[-<variant>]`

- `action`: imperative verb (e.g. `build`, `test`, `check`, `sync`)
- `subsystem`: functional surface (e.g. `go`, `web`, `config`)
- `variant`: optional narrowing (e.g. `strict`, `cov`, `linux`)

For config consistency workflows, canonical targets are verb-first (`check-*`, `sync-*`).
Legacy aliases are kept for compatibility.

## Core subsystem targets

| Action             | Go                                                            | Python            | Web            | Docs           | macOS        |
| ------------------ | ------------------------------------------------------------- | ----------------- | -------------- | -------------- | ------------ |
| **install**        | -                                                             | `install-python`  | `install-web`  | `install-docs` | -            |
| **build**          | `build-radar-*`                                               | -                 | `build-web`    | `build-docs`   | `build-mac`  |
| **dev**            | `dev-go`                                                      | -                 | `dev-web`      | `dev-docs`     | `dev-mac`    |
| **dev (variant)**  | `dev-go-lidar`<br>`dev-go-lidar-both`<br>`dev-go-kill-server` | -                 | -              | -              | -            |
| **run**            | -                                                             | -                 | -              | -              | `run-mac`    |
| **test**           | `test-go`                                                     | `test-python`     | `test-web`     | -              | `test-mac`   |
| **test (variant)** | `test-go-cov`<br>`test-go-coverage-summary`                   | `test-python-cov` | `test-web-cov` | -              | -            |
| **format**         | `format-go`                                                   | `format-python`   | `format-web`   | `format-docs`  | `format-mac` |
| **lint**           | `lint-go`                                                     | `lint-python`     | `lint-web`     | -              | -            |
| **clean**          | -                                                             | -                 | -              | -              | `clean-mac`  |

**Cross-cutting formatting targets:**

- `format-sql`: Format SQL files (migrations and schema)

## Aggregate targets

- `test`: Run aggregate tests (Go + Web + macOS)
- `format`: Format all code (Go + Python + Web + macOS + SQL + Markdown)
- `lint`: Lint all code (Go + Python + Web); fails if formatting needed
- `coverage`: Generate coverage reports for all components

## Build targets (Go cross-compilation)

- `build-velocity`: Build the single multi-call `velocity` binary (local, pcap)
- `build-velocity-linux`: Build `velocity` for Linux ARM64 with pcap
- `build-velocity-mac`: Build `velocity` for macOS ARM64 with pcap
- `build-radar-linux` / `build-radar-mac` / `build-radar-mac-intel` / `build-radar-local`: compatibility aliases that build the single `velocity` binary
- `build-tools`: alias for `build-velocity` (sweep is now `velocity tune sweep`)
- `build-ctl` / `build-ctl-linux`: aliases for `build-velocity` (device tools folded into `velocity device`)
- `build-web`: Build web frontend (SvelteKit)
- `build-docs`: Build documentation site (Eleventy)

## Testing targets

- `test`: Run aggregate tests (Go + Web + macOS)
- `test-go`: Run Go unit tests
- `test-go-cov`: Run Go tests with coverage
- `test-go-coverage-summary`: Show coverage summary for cmd/ and internal/
- `test-python`: Run Python script/tool tests
- `test-python-cov`: Run Python script/tool tests with coverage
- `test-web`: Run web tests (Jest)
- `test-web-cov`: Run web tests with coverage
- `test-mac`: Run macOS visualiser tests (XCTest)
- `test-perf`: Run performance regression tests (NAME=kirk0)
- `coverage`: Generate coverage reports for all components

## macOS visualiser targets

- `build-mac`: Build macOS LiDAR visualiser (Xcode)
- `clean-mac`: Clean macOS visualiser build artifacts
- `run-mac`: Run macOS visualiser (requires build-mac)
- `dev-mac`: Kill, build, and run macOS visualiser
- `test-mac`: Run macOS visualiser tests (XCTest)
- `format-mac`: Format macOS Swift code (swift-format)
- `dmg-mac`: Build if needed and create a timestamped development DMG
- `dmg-mac-release`: Build if needed and create `VelocityVisualiser-<VERSION>.dmg`
- `sign-mac`: Sign the built app with Developer ID Application
- `notarise-mac`: Submit the release DMG to Apple, wait, then staple the ticket
- `verify-mac`: Verify Developer ID signature, Gatekeeper assessment, and staple
- `verify-mac-dmg`: Verify a downloaded DMG (`DMG=/path/to/file.dmg`)
- `release-mac`: Build, sign, package, notarise, and verify the release DMG

## Protobuf code generation

- `proto-gen`: Generate protobuf stubs for all languages
- `proto-gen-go`: Generate Go protobuf stubs
- `proto-gen-swift`: Generate Swift protobuf stubs (macOS visualiser)

## Deployment targets (removed)

> These targets were removed in v0.5.1. `velocity-deploy` has been replaced by `velocity-ctl`.
> See [deploy-rpi-imager-fork-plan.md §8](docs/plans/deploy-rpi-imager-fork-plan.md).

- `setup-radar`: Install server on this host (requires sudo, **removed**)
- `deploy-install`: Removed: use RPi image or manual install
- `deploy-upgrade`: Removed: use `sudo velocity device upgrade`
- `deploy-status`: Removed: use `sudo velocity device status`
- `deploy-health`: Removed: use `sudo velocity device status`
- `deploy-install-report-deps`: Install legacy report dependencies on remote target (**removed**)
- `deploy-update-deps`: Update source and runtime deps on remote target (**removed**)

## Formatting targets

- `format`: Format all code (Go + Python + Web + macOS + SQL + Markdown)
- `format-go`: Format Go code (gofmt)
- `format-python`: Format Python code (black + ruff)
- `format-web`: Format web code (prettier)
- `format-mac`: Format macOS Swift code (swift-format)
- `format-docs`: Format Markdown files (prettier)
- `format-sql`: Format SQL files (sql-formatter)

## Linting targets

- `lint`: Lint all code (Go + Python + Web); fails if formatting needed
- `lint-go`: Check Go formatting
- `lint-python`: Check Python formatting
- `lint-web`: Check web formatting

## Config schema consistency targets

- `check-config-order`: Validate canonical tuning key order across config and docs surfaces
- `sync-config-order`: Rewrite config/docs targets to canonical tuning key order
- `check-config-maths`: Validate config maths keys against docs JSON, `tuning*.json`, and Go schema sources
- `check-config-maths-strict`: Strict parity mode; also requires full webserver POST schema parity
  Current status: optional in CI until webserver schema parity backlog is complete.
- Compatibility aliases: `config-order-check`, `config-order-sync`, `readme-maths-check`, `readme-maths-check-strict`

## Database migration targets

- `migrate-up`: Apply all pending migrations
- `migrate-down`: Rollback one migration
- `migrate-status`: Show current migration status
- `migrate-detect`: Detect schema version (for legacy databases)
- `migrate-version`: Migrate to specific version (VERSION=N)
- `migrate-force`: Force version (recovery, VERSION=N)
- `migrate-baseline`: Set baseline version (VERSION=N)
- `schema-sync`: Regenerate schema.sql from latest migrations

Production Linux note: `velocity-report` defaults to `sensor_data.db` in the current working
directory. Development on macOS or Linux can keep using `./sensor_data.db`, but deployed Linux <!-- link-ignore -->
service installs should target `/var/lib/velocity-report/sensor_data.db` explicitly and use the
service account or `root`, for example `sudo -u velocity /usr/local/bin/velocity-report --db-path /var/lib/velocity-report/sensor_data.db migrate status`.

## Utility targets

- `set-version`: Update version across codebase (VER=0.4.0 TARGETS='--all')
- `log-go-tail`: Tail most recent Go server log
- `log-go-cat`: Cat most recent Go server log
- `log-go-tail-all`: Tail most recent Go server log plus debug log
- `git-fs`: Show the git files that differ from main

## Data visualisation targets

- `plot-noise-sweep`: Generate noise sweep line plot (FILE=data.csv)
- `plot-multisweep`: Generate multi-parameter grid (FILE=data.csv)
- `plot-noise-buckets`: Generate per-noise bar charts (FILE=data.csv)
- `stats-live`: Capture live LiDAR snapshots (INTERVAL=10 DURATION=60)
- `stats-pcap`: Capture PCAP replay snapshots (PCAP=file.pcap INTERVAL=5)

## API shortcut targets (LiDAR HTTP API)

**Grid endpoints:**

- `api-grid-status`: Get grid status
- `api-grid-reset`: Reset background grid
- `api-grid-heatmap`: Get grid heatmap

**Snapshot endpoints:**

- `api-snapshot`: Get current snapshot
- `api-snapshots`: List all snapshots

**Acceptance endpoints:**

- `api-acceptance`: Get acceptance metrics
- `api-acceptance-reset`: Reset acceptance counters

**Parameter endpoints:**

- `api-params`: Get algorithm parameters
- `api-params-set`: Set parameters (PARAMS='{}')

**Persistence and export endpoints:**

- `api-persist`: Trigger snapshot persistence
- `api-export-snapshot`: Export specific snapshot
- `api-export-next-frame`: Export next LiDAR frame

**Status & data source endpoints:**

- `api-status`: Get server status
- `api-start-pcap`: Start PCAP replay (PCAP=file.pcap)
- `api-stop-pcap`: Stop PCAP replay
- `api-switch-data-source`: Switch live/pcap (SOURCE=live|pcap)
