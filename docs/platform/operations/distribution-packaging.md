# Distribution and packaging

- **Status:** The shipped v0.5.1 model is described below and is canonical. The
  original D-09 subcommand proposal is retained as historical context further
  down. In-flight packaging work continues in
  [deploy-single-binary-image-consolidation-plan.md](../../plans/deploy-single-binary-image-consolidation-plan.md)
  and [deploy-nginx-removal-plan.md](../../plans/deploy-nginx-removal-plan.md).

Distribution and packaging strategy for velocity.report: ship one signed binary
with a consistent release process and atomic, reversible on-disk upgrades.

## Shipped model (v0.5.1)

velocity.report ships a single busybox-style `velocity` binary. `argv[0]`
selects behaviour for compatibility, but the canonical command surface is
`velocity <namespace> ...`:

| Namespace         | Purpose                                         |
| ----------------- | ----------------------------------------------- |
| `serve` (default) | Start the HTTP/gRPC server                      |
| `device`          | On-device version lifecycle (upgrade, rollback) |
| `data`            | Database and data utilities                     |
| `report`          | PDF report generation                           |
| `tune`            | Parameter sweep / tuning tools                  |

- **Compatibility aliases:** `velocity-report` is retained as a server-oriented
  alias because systemd units and operator habits depend on it. The former
  `velocity-ctl` binary has been removed in favour of the `velocity device …`
  namespace.
- **Host lifecycle stays outside the binary:** `velocity-status`,
  `velocity-log`, `velocity-start`, `velocity-stop`, and `velocity-bounce`
  remain shell wrappers around `systemctl`/`journalctl` (in
  `/etc/profile.d/velocity-aliases.sh`) — host concerns, not application
  namespaces.
- **Versioned on-disk layout:** installs live under
  `/opt/velocity-report/versions/<v>/` with `current` and `previous` symlinks,
  and `/usr/local/bin/velocity` as the canonical entry point. Upgrade and
  rollback are a single atomic `renameat2(2)` symlink swap; the installer keeps
  the last three versions and prunes the rest. Updates never write to
  `/usr/local/bin/`.
- **Build identity:** `GET /api/version` exposes the running build; `velocity
version` reports the same from the CLI.
- **Release artefacts:** the binary asset is `velocity-<v>-<os>-arm64`; the
  Raspberry Pi image is `velocity-report-<v>.img.xz`. A tightened sudoers policy
  scopes what the service account may invoke.

This model landed in v0.5.1, replacing the multi-binary D-09 proposal below.

## Historical proposal (D-09 subcommand model, superseded)

The sections below are the original D-09 proposal — a `velocity-report` binary
with subcommands plus separate `velocity-report-sweep` and `velocity-ctl`
artefacts, installed flat under `/usr/local/bin/`. They are **superseded** by
the shipped model above and retained for context only.

## Problem

Multiple scattered tools, no release process, complex Python setup. The Go
server, Python PDF generator, sweep tool, and utility scripts each have
different build and distribution paths.

## Chosen architecture: subcommand model (D-09)

Single `velocity-report` binary with subcommands, plus separate power-user
binaries.

```
velocity-report                        # Main binary (all users)
  ├── serve      (default)            # Start server
  ├── migrate    (existing)           # DB migrations
  ├── pdf        (new)                # Generate PDF
  ├── backfill   (moved)              # Transit backfill
  └── version    (new)                # Version info

velocity-ctl                           # On-device management (root)
  ├── upgrade                        # In-place upgrade from GitHub Releases
  ├── rollback                       # Restore previous version
  ├── backup                         # Snapshot binary + database
  ├── status                         # Service status
  └── version                        # Show installed versions

velocity-report-sweep                  # Power user tool
velocity-report-backfill-rings         # Developer tool
```

`velocity-ctl` replaces the deleted `velocity-deploy` binary (see
[deploy-rpi-imager-fork-plan.md § 8](../../plans/deploy-rpi-imager-fork-plan.md#8-deploy-tool-replacement-velocity-ctl)).
It is a purpose-built on-device management tool with no SSH surface.

### Key changes

| What               | Before                                               | After                                               |
| ------------------ | ---------------------------------------------------- | --------------------------------------------------- |
| **Main binary**    | [internal/cmd/server/](../../../internal/cmd/server) | `cmd/velocity-report/`                              |
| **Start server**   | `velocity-report`                                    | `velocity-report serve` (or just `velocity-report`) |
| **PDF generation** | `PYTHONPATH=... python -m ...`                       | `velocity-report pdf config.json`                   |
| **Sweep tool**     | `./app-sweep`                                        | `velocity-report-sweep`                             |
| **Installation**   | Manual build + scp + script                          | `curl install.sh \| sudo bash`                      |
| **Releases**       | None                                                 | GitHub Releases with CI/CD                          |

## Components inventory

| Component                    | Type          | Location                                                                           | Current Distribution              |
| ---------------------------- | ------------- | ---------------------------------------------------------------------------------- | --------------------------------- |
| **Main Server**              | Go            | [internal/cmd/server/](../../../internal/cmd/server)                               | Manual build + setup script       |
| **Migrate CLI**              | Go subcommand | [internal/db/migrate_cli.go](../../../internal/db/migrate_cli.go)                  | Part of main binary               |
| **Sweep Tool**               | Go            | [internal/cmd/tune/](../../../internal/cmd/tune)                                   | Manual build (`make build-tools`) |
| **PDF Generator**            | Go            | [internal/report/](../../../internal/report)                                       | Built into main binary            |
| **Transit Backfill**         | Go            | `cmd/transit-backfill/`                                                            | Manual `go build`                 |
| **Ring Elevations Backfill** | Go            | [cmd/tools/backfill_ring_elevations/](../../../cmd/tools/backfill_ring_elevations) | Manual `go build`                 |
| **Grid Heatmap**             | Python        | [tools/grid-heatmap/](../../../tools/grid-heatmap)                                 | Manual invocation                 |
| **Web Frontend**             | Svelte        | `web/`                                                                             | `//go:embed` in assets.go         |

## User personas

| Persona                    | Needs                                                                   |
| -------------------------- | ----------------------------------------------------------------------- |
| **Neighbourhood Advocate** | Single binary, web UI, PDF reports, systemd auto-start                  |
| **Traffic Engineer**       | All tools (sweep, heatmap, backfill), Python available, CLI proficiency |
| **Developer**              | Source repo with Makefile, all build targets, dev convenience           |

## Tool categorisation

- **Core tools** (in main binary): serve, migrate, pdf, basic backfill
- **Power user tools** (separate): sweep, grid-heatmap
- **Developer tools** (not installed): ring elevations backfill, dev scripts

## Installed system layout

```
/usr/local/bin/
  ├── velocity-report                    # Main binary (~30 MB)
  ├── velocity-report-sweep              # Sweep binary (~15 MB)
  └── velocity-report-backfill-rings     # Utility binary (~15 MB)

/usr/local/share/velocity-report/
  └── docs/

/var/lib/velocity-report/                # Data directory
  └── sensor_data.db                     # SQLite database

/etc/systemd/system/
  └── velocity-report.service            # Systemd unit

/etc/velocity-report/                    # Configuration (optional)
  └── config.yaml
```

## Command structure

### Main binary: `velocity-report`

```
velocity-report                  # Start server (default, backward compat)
velocity-report serve            # Start server (explicit)
velocity-report migrate up       # Database migrations (existing)
velocity-report pdf config.json  # Generate PDF report
velocity-report backfill ...     # Transit backfill
velocity-report version          # Show version info
velocity-report help             # Show help
```

### Additional binaries

```
velocity-report-sweep --mode multi --iterations 30
velocity-report-backfill-rings --db sensor_data.db
```

## Version management

The `version` package (`internal/version/`) exports three variables: `Version` (default `"dev"`), `GitSHA` (default `"unknown"`), and `BuildTime` (default `"unknown"`). These are set via Makefile linker flags under `github.com/banshee-data/velocity.report/internal/version`.

## Source layout (proposed)

```
cmd/
  ├── velocity-report/           # Main binary (was internal/cmd/server)
  │   ├── main.go               # Subcommand dispatcher
  │   ├── serve.go              # Server logic
  │   ├── pdf.go                # PDF wrapper
  │   ├── backfill.go           # Backfill (moved from separate cmd)
  │   └── version.go            # Version info
  ├── velocity-report-sweep/    # Sweep tool (renamed)
  └── velocity-report-backfill-rings/  # Utility (renamed)
internal/
  └── version/                   # Version management
```

## Migration compatibility

- Old binary still works (starts server by default)
- New binary backward compatible (no args = serve)
- Systemd service file: change `ExecStart` to include `serve` subcommand
- All existing Makefile targets preserved
- All existing flags for `serve` preserved

## Rollback plan

```bash
sudo systemctl stop velocity-report
sudo cp /path/to/old/velocity-report /usr/local/bin/velocity-report
# Restore old service file (remove "serve" from ExecStart)
sudo systemctl daemon-reload
sudo systemctl start velocity-report
```

## Breaking changes summary

### End users: no breaking changes

- `velocity-report` (no args) still starts server
- All existing flags preserved

### Developers: minor

- [internal/cmd/server/](../../../internal/cmd/server) moves to `cmd/velocity-report/`
- Binary name includes version: `velocity-report-{version}-linux-arm64`
- Import paths unchanged (only `cmd/` structure changes)

### Advanced users

- `app-sweep` renamed to `velocity-report-sweep`
- All features preserved, consistent naming convention
