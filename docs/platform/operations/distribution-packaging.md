# Distribution and Packaging

Active plan: [deploy-distribution-packaging-plan.md](../../plans/deploy-distribution-packaging-plan.md)

## Problem

Multiple scattered tools, no release process, complex Python setup. The Go
server, Python PDF generator, sweep tool, and utility scripts each have
different build and distribution paths.

## Chosen Architecture: Subcommand Model (D-09)

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
  ├── upgrade    (v0.5.1)             # In-place upgrade from GitHub Releases
  ├── rollback   (v0.5.1)             # Restore previous version
  ├── backup     (v0.5.1)             # Snapshot binary + database
  ├── status     (v0.5.1)             # Service status
  └── version    (v0.5.1)             # Show installed versions

velocity-report-sweep                  # Power user tool
velocity-report-backfill-rings         # Developer tool
```

`velocity-ctl` replaces the deleted `velocity-deploy` binary (see
[deploy-rpi-imager-fork-plan.md § 8](../../plans/deploy-rpi-imager-fork-plan.md#8-deploy-tool-replacement--velocity-ctl)).
It is a purpose-built on-device management tool with no SSH surface.

### Key Changes

| What               | Before                         | After                                               |
| ------------------ | ------------------------------ | --------------------------------------------------- |
| **Main binary**    | `cmd/radar/`                   | `cmd/velocity-report/`                              |
| **Start server**   | `velocity-report`              | `velocity-report serve` (or just `velocity-report`) |
| **PDF generation** | `PYTHONPATH=... python -m ...` | `velocity-report pdf config.json`                   |
| **Sweep tool**     | `./app-sweep`                  | `velocity-report-sweep`                             |
| **Installation**   | Manual build + scp + script    | `curl install.sh \| sudo bash`                      |
| **Releases**       | None                           | GitHub Releases with CI/CD                          |

## Components Inventory

| Component                    | Type          | Location                              | Current Distribution              |
| ---------------------------- | ------------- | ------------------------------------- | --------------------------------- |
| **Main Server**              | Go            | `cmd/radar/`                          | Manual build + setup script       |
| **Migrate CLI**              | Go subcommand | `internal/db/migrate_cli.go`          | Part of main binary               |
| **Sweep Tool**               | Go            | `cmd/sweep/`                          | Manual build (`make build-tools`) |
| **PDF Generator**            | Python        | `tools/pdf-generator/`                | PYTHONPATH + Makefile             |
| **Transit Backfill**         | Go            | `cmd/transit-backfill/`               | Manual `go build`                 |
| **Ring Elevations Backfill** | Go            | `cmd/tools/backfill_ring_elevations/` | Manual `go build`                 |
| **Grid Heatmap**             | Python        | `tools/grid-heatmap/`                 | Manual invocation                 |
| **Web Frontend**             | Svelte        | `web/`                                | `//go:embed` in assets.go         |

## User Personas

| Persona                    | Needs                                                                   |
| -------------------------- | ----------------------------------------------------------------------- |
| **Neighbourhood Advocate** | Single binary, web UI, PDF reports, systemd auto-start                  |
| **Traffic Engineer**       | All tools (sweep, heatmap, backfill), Python available, CLI proficiency |
| **Developer**              | Source repo with Makefile, all build targets, dev convenience           |

## Tool Categorisation

- **Core tools** (in main binary): serve, migrate, pdf, basic backfill
- **Power user tools** (separate): sweep, grid-heatmap
- **Developer tools** (not installed): ring elevations backfill, dev scripts

## Installed System Layout

```
/usr/local/bin/
  ├── velocity-report                    # Main binary (~30 MB)
  ├── velocity-report-sweep              # Sweep binary (~15 MB)
  └── velocity-report-backfill-rings     # Utility binary (~15 MB)

/usr/local/share/velocity-report/
  ├── python/                            # Python packages
  │   ├── .venv/                         # Virtual environment
  │   ├── pdf_generator/
  │   └── grid_heatmap/
  └── docs/

/var/lib/velocity-report/                # Data directory
  └── sensor_data.db                     # SQLite database

/etc/systemd/system/
  └── velocity-report.service            # Systemd unit

/etc/velocity-report/                    # Configuration (optional)
  └── config.yaml
```

## Python Environment Strategy

Python scripts need dependencies (matplotlib, PyLaTeX, etc.). Solution:
virtual environment in a shared location.

```
/usr/local/share/velocity-report/python/.venv/
```

The `velocity-report pdf` subcommand discovers Python via a fallback chain:

1. `/usr/local/share/velocity-report/python/.venv/bin/python3`
2. `$VELOCITY_REPORT_PYTHON` environment variable
3. System `python3`
4. Error with helpful message

## Command Structure

### Main Binary: `velocity-report`

```
velocity-report                  # Start server (default, backward compat)
velocity-report serve            # Start server (explicit)
velocity-report migrate up       # Database migrations (existing)
velocity-report pdf config.json  # Generate PDF report (calls Python)
velocity-report backfill ...     # Transit backfill
velocity-report version          # Show version info
velocity-report help             # Show help
```

### Additional Binaries

```
velocity-report-sweep --mode multi --iterations 30
velocity-report-backfill-rings --db sensor_data.db
```

## Version Management

```go
package version

var (
    Version   = "dev"
    GitCommit = "unknown"
    BuildTime = "unknown"
)
```

Set via linker flags: `-X .../version.Version=$(VERSION)`

Git revision and build time populated from `debug.ReadBuildInfo()` VCS
settings at runtime.

## Source Layout (Proposed)

```
cmd/
  ├── velocity-report/           # Main binary (was cmd/radar)
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

## Migration Compatibility

- Old binary still works (starts server by default)
- New binary backward compatible (no args = serve)
- Systemd service file: change `ExecStart` to include `serve` subcommand
- All existing Makefile targets preserved
- All existing flags for `serve` preserved

## Rollback Plan

```bash
sudo systemctl stop velocity-report
sudo cp /path/to/old/velocity-report /usr/local/bin/velocity-report
# Restore old service file (remove "serve" from ExecStart)
sudo systemctl daemon-reload
sudo systemctl start velocity-report
```

## Breaking Changes Summary

### End Users — No Breaking Changes

- `velocity-report` (no args) still starts server
- All existing flags preserved

### Developers — Minor

- `cmd/radar/` moves to `cmd/velocity-report/`
- Binary name unchanged: `velocity-report-linux-arm64`
- Import paths unchanged (only `cmd/` structure changes)

### Advanced Users

- `app-sweep` renamed to `velocity-report-sweep`
- All features preserved, consistent naming convention
