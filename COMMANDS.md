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

# Commands

This is the map of the project's make targets: how to build, test, format, deploy, and operate
velocity.report without relying on tribal memory or a terminal incantation passed down at lunch.

Run `make help` or `make` for the command overview. The Makefile also contains internal targets
used by other recipes and CI.

## Naming convention

Most public targets follow `<action>-<subsystem>[-<variant>]`.

- `action`: imperative verb (e.g. `build`, `test`, `check`, `sync`)
- `subsystem`: functional surface (e.g. `go`, `web`, `config`)
- `variant`: optional narrowing (e.g. `strict`, `cov`, `linux`)

For config consistency workflows, canonical targets are verb-first (`check-*`, `sync-*`).
Legacy aliases are kept for compatibility.

## Core subsystem targets

| Subsystem                               | Install                | Build                                 | Develop                                       | Test                |
| --------------------------------------- | ---------------------- | ------------------------------------- | --------------------------------------------- | ------------------- |
| Go server and CLI                       |                        | `build-radar-local`, `build-velocity` | `dev-go`, `dev-go-lidar`, `dev-go-lidar-both` | `test-go`           |
| Python tooling                          | `install-python`       |                                       |                                               | `test-python`       |
| SvelteKit web app (`web/`)              | `install-web`          | `build-web`                           | `dev-web` (port 5173)                         | `test-web`          |
| Public site (`public_html/`)            | `install-public-html`  | `build-public-html`                   | `dev-public-html` (port 8090)                 | `test-public-html`  |
| Embedded repository docs (`docs_html/`) | `install-docs-offline` | `build-docs-offline`                  | `dev-docs-offline` (port 8093)                | `test-docs-offline` |
| macOS visualiser                        |                        | `build-mac`                           | `dev-mac`                                     | `test-mac`          |

## Public site and offline docs

The public site in `public_html/` uses `public-html` as its Make subsystem name.

- `build-public-html` writes `public_html/_site/` for hosting at `/`.
- `build-public-html-image` writes to the same directory with `/public_html/`-prefixed URLs for the Pi
  image. Both builds replace the generated directory. `verify-public-html-image-build` verifies the
  image output without rebuilding it.
- `build-docs-offline` writes `docs_html/_site/` for the Go service to embed and serve at `/docs/`.
- `dev-public-html-kill` and `dev-docs-offline-kill` stop the respective preview processes.
- `format-docs` and `lint-docs` apply to repository Markdown.

Deprecated public-site targets still dispatch to their replacements.

| Old target                      | Replacement                      |
| ------------------------------- | -------------------------------- |
| `install-docs`                  | `install-public-html`            |
| `dev-docs`                      | `dev-public-html`                |
| `dev-docs-kill`                 | `dev-public-html-kill`           |
| `build-docs`                    | `build-public-html`              |
| `build-docs-public-html`        | `build-public-html-image`        |
| `verify-docs-public-html-build` | `verify-public-html-image-build` |

## Aggregate targets

- `test` runs Go, Python, web, public site, offline docs, macOS, S2 Hilbert, and scene-capture tests.
- `format` formats Go, web, macOS, Markdown, and SQL. Run `format-python` for Python tooling.
- `lint` checks Go, web, documentation, and repository hygiene. Run `lint-python` or `lint-mac` for those checks.
- `coverage` runs coverage for Go, Python, web, offline docs, and macOS.

## Go binary builds

- `build-velocity`: Build the multi-call binary as `./velocity` with pcap support
- `build-radar-local`: Build the same program as `velocity-report-local` for development. Override the output with `VELOCITY_LOCAL_BIN`.
- `build-velocity-linux`: Build `velocity` for Linux ARM64 with pcap
- `build-velocity-mac`: Build `velocity` for macOS ARM64 with pcap
- `build-velocity-mac-intel`: Build `velocity` for macOS AMD64 with pcap
- `build-radar-linux`, `build-radar-mac`, and `build-radar-mac-intel` map to the corresponding `build-velocity-*` targets.
- `build-tools`: alias for `build-velocity` (sweep is now `velocity tune sweep`)
- `build-ctl` aliases `build-velocity`, and `build-ctl-linux` aliases `build-velocity-linux`. Device management runs under `velocity device`.

### Static Linux binaries

The static Linux build uses Docker, Zig, musl, and libpcap. The old targets remain callable as
deprecated aliases.

| Canonical target                   | Old target                      | Purpose                                                         |
| ---------------------------------- | ------------------------------- | --------------------------------------------------------------- |
| `build-velocity-static`            | `build-radar-static`            | Build both Linux architectures into `build/static/`             |
| `build-velocity-static-amd64`      | `build-radar-static-amd64`      | Build the static Linux AMD64 binary                             |
| `build-velocity-static-arm64`      | `build-radar-static-arm64`      | Build the static Linux ARM64 binary                             |
| `build-velocity-linux-docker`      | `build-radar-linux-docker`      | Stage the Pi image's ARM64 binary in `image/velocity-binaries/` |
| `test-velocity-static-arm64-vm`    | `test-radar-static-arm64-vm`    | Test an ARM64 build in a QEMU VM                                |
| `test-velocity-static-golden-pcap` | `test-radar-static-golden-pcap` | Test an AMD64 build against the canonical LiDAR capture         |

## Testing targets

- `test-go`: Run Go unit tests
- `test-go-cov`: Run Go tests with coverage
- `test-go-coverage-summary`: Show coverage summary for cmd/ and internal/
- `test-python`: Run Python script/tool tests
- `test-python-cov`: Run Python script/tool tests with coverage
- `test-web`: Run web tests (Jest)
- `test-web-cov`: Run web tests with coverage
- `test-mac`: Run macOS visualiser tests (XCTest)
- `test-perf`: Run the performance gate for one capture and profile (`NAME=kirk0`, `PROFILE=full` by default)

## macOS visualiser targets

- `build-mac`: Build macOS LiDAR visualiser (Xcode)
- `clean-mac`: Clean macOS visualiser build artefacts
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

> These targets were removed in v0.5.1. Device management now runs under `velocity device`.
> See [deploy-rpi-imager-fork-plan.md §8](docs/plans/deploy-rpi-imager-fork-plan.md).

- `setup-radar`: Install server on this host (requires sudo, **removed**)
- `deploy-install`: Removed: use RPi image or manual install
- `deploy-upgrade`: Removed: use `sudo velocity device upgrade`
- `deploy-status`: Removed: use `sudo velocity device status`
- `deploy-health`: Removed: use `sudo velocity device status`
- `deploy-install-report-deps`: Install legacy report dependencies on remote target (**removed**)
- `deploy-update-deps`: Update source and runtime deps on remote target (**removed**)

## Formatting targets

- `format-go`: Format Go code (gofmt)
- `format-python`: Format Python code (black + ruff)
- `format-web`: Format web code (prettier)
- `format-mac`: Format macOS Swift code (swift-format)
- `format-docs`: Format Markdown files (prettier)
- `format-sql`: Format SQL files (sql-formatter)

## Linting targets

- `lint-go`: Check Go formatting
- `lint-python`: Check Python formatting
- `lint-web`: Check web formatting

## Config schema consistency targets

- `check-config-order`: Validate canonical tuning key order across config and docs surfaces
- `sync-config-order`: Rewrite config/docs targets to canonical tuning key order
- `check-config-maths`: Validate config maths keys against docs JSON, `tuning*.json`, and Go schema sources
- Compatibility aliases include `config-order-check`, `config-order-sync`, `check-config-maths-strict`,
  `readme-maths-check`, and `readme-maths-check-strict`. The two names ending in `-strict`
  resolve to `check-config-maths`.

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

- `version-exact`: Update the version across the codebase (`VER=0.5.1`, optional `TARGETS`)
- `log-go-tail`: Tail most recent Go server log
- `log-go-cat`: Cat most recent Go server log
- `log-go-tail-all`: Tail most recent Go server log plus debug log
- `git-fs`: Show the git files that differ from main

## Data visualisation targets

- `plot-noise-sweep`: Generate noise sweep line plot (FILE=data.csv)
- `plot-multisweep`: Generate multi-parameter grid (FILE=data.csv)
- `plot-noise-buckets`: Generate per-noise bar charts (FILE=data.csv)

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
