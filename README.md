# velocity.report

<div align="center">

[![Discord](https://img.shields.io/discord/XXh6jXVFkt)](https://discord.gg/XXh6jXVFkt)
[![🧭 Go CI](https://github.com/banshee-data/velocity.report/actions/workflows/go-ci.yml/badge.svg?branch=main)](https://github.com/banshee-data/velocity.report/actions/workflows/go-ci.yml)
[![🌐 Web CI](https://github.com/banshee-data/velocity.report/actions/workflows/web-ci.yml/badge.svg?branch=main)](https://github.com/banshee-data/velocity.report/actions/workflows/web-ci.yml)
[![🐍 Python CI](https://github.com/banshee-data/velocity.report/actions/workflows/python-ci.yml/badge.svg?branch=main)](https://github.com/banshee-data/velocity.report/actions/workflows/python-ci.yml)
[![Go Coverage](https://codecov.io/gh/banshee-data/velocity.report/branch/main/graph/badge.svg?flag=go)](https://codecov.io/gh/banshee-data/velocity.report?flag=go)
[![Web Coverage](https://codecov.io/gh/banshee-data/velocity.report/branch/main/graph/badge.svg?flag=web)](https://codecov.io/gh/banshee-data/velocity.report?flag=web)
[![Python Coverage](https://codecov.io/gh/banshee-data/velocity.report/branch/main/graph/badge.svg?flag=python)](https://codecov.io/gh/banshee-data/velocity.report?flag=python)

</div>

A privacy-focused traffic logging tool for neighborhood change-makers.

Measure vehicle speeds, make streets safer.

```
                                                ░░░░
                                               ▒▓███▓▓▓▓▒
                                                      ▒▓▒▒
                    ░▓▓▓▓▓▓▓▓▓▓▓▓░                    ░▓▒▒
                    ▒▓▓▓▓▓██████▓▓░                ▒▓██▓▒
                      ▒▒▓▒▓▓░                      ▒▓▒░
                         ░▓▓░                       ▓▒▒
                          ░▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓██████▓░
                          ▓▓█▓▒▒▒▒▒▒░░░░            ░▓▓▒
                        ░▓▓▒▓▓░                   ░▒▓▓▓▓░
           ░░▒▒▒▒░     ░▓▒░ ▒▓▒                  ▒▓▓▒ ▓▓▒ ░▒▒▓▓▒▒░
        ▒▓▓▓██▓▓▓██▓▓▓▓▓▒   ░▓▓░               ▒▓▓▒   ▒██▓▓█▓▓▓▓▓▓▒▓▓▒░
     ░▓▓▓▓▓▒░ ░    ▒▒██▓▒▒   ▒▓▒░            ░▓▓▒   ▓▓█▓█▓    ░░   ▒▒▓▒▓▒
    ▒▓▓▓▓░    ░░   ░▓▓░▒▓▓▒   ▒▓▒           ▓▓▓   ░▓▓▓░░▓▓░   ░░      ▒█▓▒▒
  ░▒█▒▓░▒     ░░  ░▓▒░  ░▓▓▒░ ░▓▓ ░████▓  ▒█▓░   ▒▓█▒   ░▓▓░  ░░     ▒░░▒█▓▒
  ▒▓▒▓   ▒░   ░░ ▒▓▓     ░▓▓▓░ ░▓▒ ░▓▒  ░▓▓▒    ▒▓█▒░▒   ▒█▒  ░    ▒░   ░▒▓▓▒
 ░▓█▓     ░▒░ ░░▒▓▒       ░▓▓▒░▒▓████▒ ▒▓▓░    ▒▓▓▒    ▒▒ ▓▓▒ ░  ░▒      ░▓▓▒░
 ▒▓▓▒       ░▒▒▓█▓▓▓███▓▓▓▓████▓█▓▓▒▒▓▓▓▒      ▒█▓░      ░▓▓▓▒▒▒▒         ▒▓▒▒
 ▒▓▓▒░░▒▒▒▓▒▒▓▓██▓▒▒░▒░░░  ▒▓▒▒▓▓▓▓▓█▓▓▓░      ▒█▓░      ░░░▒█▓▓▒▒▒░░░░░░░▒█▓░
 ▒▓▓▒       ░▒▒▓▒░         ▓▓▓░▒▓▓▓▓▓▒▒▓░      ▒▓▓▒▒░░░░░   ░▒▓░▒░        ▓▓▓░
 ░▓█▓░    ░▒░ ░▒  ▒░      ▒▓▓▒  ▒███▓▓▓░       ░▓▓▒       ░░  ░  ░▒░     ░▓▓▓░
  ▒▓▓▓░  ▒░    ▒    ▒░   ░▓▓▓░    ▒▓            ▓▓▓░     ▒    ░░   ░▒░  ░▓█▓░
   ▒▓▓▓▒░     ░▒     ░▒ ▒▓▓▒     ░▓▒            ░▒▓▓░  ░░     ░░      ▒▒▓▓▓░
    ▒▓▓▓▒░     ▒      ░▓▓▓▒     ▓█▓▓█░            ▒▓▓▓▒░      ▒░     ░▓█▓▓░
     ░▒▓██▓▒▒  ▒  ░▒▓▓█▓▒░                         ░▓▓█▓▓░    ░░ ░▒▓█▓▓▓░
      ░░░▒▒▓▓████▓██▓▓░                ░░             ▒▓▓▓▓██▓▓▓████▓▒░
  ░░░░░░░░░░░░▒▒░░░░░░░░░░░░░░░░░░░░░ ░░░░░░░░░░░░░░░░ ░░░▒▒▒▒▓▒▒░░░░░░░░░░
      ░░░░░░░░░░░░░░░░░░░░░░ ░░░░ ░░░░░░░░░░  ░░░░░░░░░░░░░░░░░░░░░ ░░░░░░░░░
   ░░░ ░░░░░░   ░░░░ ░░░░░░░░░░░░ ░░░░░   ░░░░░░   ░░░░░░ ░░░░░░░░░░░ ░░░░
     ░░░    ░░░░   ░░░░ ░░░░    ░░░    ░░░░    ░░░░░   ░░░░░   ░░░░░
```

## Overview

**velocity.report** is a complete citizen radar system for neighborhood traffic monitoring. The system consists of three main components:

- **Go Server** - High-performance data collection and API server
- **Python PDF Generator** - Professional PDF report generation with LaTeX
- **Web Frontend** - Real-time data visualisation (Svelte)

The system collects vehicle speed data from radar/LIDAR sensors, stores it in SQLite, and provides multiple ways to visualise and report on the data—all while maintaining complete privacy (no license plate recognition, no video recording).

## Quick Start

### For Go Server Development

```sh
git clone git@github.com:banshee-data/velocity.report.git
cd velocity.report
make build-radar-local
./velocity-report-local --disable-radar
```

If an existing SQLite database is available, place it in `./sensor_data.db` (the default location for development). For production deployments, use the `--db-path` flag to specify a different location (see Deployment section).

### For PDF Report Generation

See **[tools/pdf-generator/README.md](tools/pdf-generator/README.md)** for detailed instructions.

Quick version:

```sh
cd tools/pdf-generator
make install-python         # One-time setup
make pdf-config             # Create config template
make pdf-report CONFIG=config.json
```

### For Web Frontend Development

See **[web/README.md](web/README.md)** for detailed instructions.

## Project Structure

```
velocity.report/
├── cmd/                      # Go CLI applications
│   ├── radar/                # Radar sensor integration
│   ├── bg-sweep/             # Background sweep utilities
│   └── tools/                # Go utility tools
├── internal/                 # Go server internals (private packages)
│   ├── api/                  # HTTP API endpoints
│   ├── db/                   # SQLite database layer
│   ├── radar/                # Radar sensor logic
│   ├── lidar/                # LIDAR sensor logic
│   ├── monitoring/           # System monitoring
│   └── units/                # Unit conversion utilities
├── web/                      # Svelte web frontend
│   ├── src/                  # Frontend source code
│   └── static/               # Static assets
├── tools/                    # Python tooling
│   └── pdf-generator/        # PDF report generation
│       ├── pdf_generator/    # Python package
│       │   ├── cli/          # CLI tools
│       │   ├── core/         # Core modules
│       │   └── tests/        # Test suite
│       └── output/           # Generated PDFs
├── data/                     # Data directory
│   ├── migrations/           # Database migrations
│   └── align/                # Data alignment utilities
├── docs/                     # Documentation Site
├── scripts/                  # Development shell scripts
└── static/                   # Static server assets
```

## Architecture

### Data Flow

```
   ┌───────────────────┐     ┌───────────────────┐     ┌───────────────────┐
   │     Sensors       │────►│     Go Server     │◄───►│  SQLite Database  │
   │ (Radar / LIDAR)   │     │ (API/Processing)  │     │ (Time-series)     │
   └───────────────────┘     └───────────────────┘     └───────────────────┘
                                     │
                                     │
                   ┌─────────────────┼─────────────────┐
                   │                                   │
                   ▼                                   ▼
   ┌─────────────────────────────┐     ┌─────────────────────────────┐
   │        Web Frontend         │     │    Python PDF Generator     │
   │   (Real-time via Svelte)    │     │ (Offline Reports via LaTeX) │
   └─────────────────────────────┘     └─────────────────────────────┘
```

### Components

**1. Go Server** (`/cmd/`, `/internal/`)

- Collects data from radar/LIDAR sensors
- Stores time-series data in SQLite
- Provides HTTP API for data access
- Handles background processing tasks
- Runs as systemd service on Raspberry Pi

**2. Python PDF Generator** (`/tools/pdf-generator/`)

- Generates professional PDF reports using LaTeX
- Creates charts and visualisations with matplotlib
- Processes statistical summaries
- Highly configurable via JSON
- Comprehensive test suite

**3. Web Frontend** (`/web/`)

- Real-time data visualisation
- Interactive charts and graphs
- Built with Svelte and TypeScript
- Responsive design

See **[ARCHITECTURE.md](ARCHITECTURE.md)** for detailed architecture documentation.

## Development

### Prerequisites

**For Go Development:**

- Go 1.21+ ([installation guide](https://go.dev/doc/install))
- SQLite3

**For Python PDF Generation:**

- Python 3.9+
- LaTeX distribution (XeLaTeX)
- See [tools/pdf-generator/README.md](tools/pdf-generator/README.md)

**For Web Frontend:**

- Node.js 18+
- pnpm
- See [web/README.md](web/README.md)

### Go Server Development

Build the development server:

```sh
make build-radar-local
./velocity-report-local --disable-radar
```

Run tests:

```sh
make test
```

Build for production (Raspberry Pi):

```sh
make build-radar-linux
# or manually:
GOOS=linux GOARCH=arm64 go build -o velocity-report-linux-arm64 ./cmd/radar
```

### Python PDF Generator Development

The repository uses a **single shared Python virtual environment** for all Python tools (PDF generator, data visualisation, analysis scripts).

**Setup:**

```sh
make install-python  # Creates .venv and installs all dependencies
```

**Activate manually (optional):**

```sh
source .venv/bin/activate
```

**What's installed:**

- PDF generation: PyLaTeX, reportlab
- Data analysis: pandas, numpy, scipy
- Visualisation: matplotlib, seaborn
- Testing: pytest, pytest-cov
- Formatting: black, ruff

**Run PDF Generator:**

```sh
make pdf-test         # Run test suite
make pdf-demo         # Run interactive demo
make pdf-config       # Create config template
make pdf-report CONFIG=config.json  # Generate PDF report
```

### Code Formatting

**Option 1: Format on demand (recommended for new contributors)**

```sh
make format        # Format all code before commit
make lint          # Verify formatting (what CI checks)
```

**Option 2: Editor integration**

- VS Code: Install Prettier, ESLint, Go extensions
- Format-on-save handles most cases

**Option 3: Pre-commit hooks (recommended for regular contributors)**

```sh
pip install pre-commit
pre-commit install
```

Hooks auto-format code on every commit — no manual `make format` needed.

**What runs on commit (if hooks enabled):**

- File hygiene (trailing whitespace, large files, etc.)
- Go formatting (gofmt)
- Python formatting (ruff + black) for PDF generator code
- Web formatting (prettier)

**Note:** CI lint jobs are advisory (non-blocking), so PRs can merge even without perfect formatting. A weekly automated workflow cleans up any missed formatting issues. See [`.github/workflows/lint-autofix.yml`](.github/workflows/lint-autofix.yml) for details.

### Web Frontend Development

```sh
cd web
pnpm install
pnpm dev
```

See **[web/README.md](web/README.md)** for details.

## Deployment

### Go Server (Raspberry Pi)

The Go server runs as a systemd service on Raspberry Pi. Use the new `velocity-deploy` tool for comprehensive deployment management.

**Quick Start - Deploy to Raspberry Pi:**

```sh
# Build the binary and deployment tool
make build-radar-linux
make build-deploy

# Deploy to remote Pi
./velocity-deploy install \
  --target pi@192.168.1.100 \
  --ssh-key ~/.ssh/id_rsa \
  --binary ./velocity-report-linux-arm64
```

**Or use Make shortcuts for local deployment:**

```sh
make build-radar-linux
make deploy-install
```

The deployment will:

- Install the binary to `/usr/local/bin/velocity-report`
- Create a dedicated service user and working directory
- Install and enable the systemd service
- Optionally migrate existing database

**Upgrade to new version:**

```sh
make build-radar-linux
./velocity-deploy upgrade --target pi@192.168.1.100 --binary ./velocity-report-linux-arm64
```

**Monitor service health:**

```sh
# Comprehensive health check
./velocity-deploy health --target pi@192.168.1.100

# Check status
./velocity-deploy status --target pi@192.168.1.100

# View logs
sudo journalctl -u velocity-report.service -f
```

**See also:**

- **[docs/deployment-guide.md](docs/deployment-guide.md)** - Complete deployment guide
- **[cmd/deploy/README.md](cmd/deploy/README.md)** - velocity-deploy CLI reference

**Legacy deployment:**

The previous `scripts/setup-radar-host.sh` script is still available but the new `velocity-deploy` tool is recommended for all deployments.

### Python PDF Generator

The PDF generator is deployed as a Python package via PYTHONPATH:

```sh
cd tools/pdf-generator
make install-python
# PDF generator is now ready at tools/pdf-generator/pdf_generator/
```

No installation required - use PYTHONPATH method as documented in [tools/pdf-generator/README.md](tools/pdf-generator/README.md).

## Documentation

- **[ARCHITECTURE.md](ARCHITECTURE.md)** - System architecture and component relationships
- **[docs/coverage.md](docs/coverage.md)** - Code coverage setup and usage guide
- **[internal/db/migrations/README.md](internal/db/migrations/README.md)** - Database migration guide and reference
- **[CHANGELOG.md](CHANGELOG.md)** - Version history and release notes
- **[web/README.md](web/README.md)** - Web frontend documentation
- **[tools/pdf-generator/README.md](tools/pdf-generator/README.md)** - PDF generator documentation
- **[docs/README.md](docs/README.md)** - Documentation site

## Testing

### Go Tests

```sh
make test
```

### Python Tests (PDF Generator)

```sh
cd tools/pdf-generator
make pdf-test
# or with coverage:
make test-python-cov
```

## Make Targets

The project uses a consistent naming scheme for all make targets: `<action>-<subsystem>[-<variant>]`

### Core Subsystem Targets

| Action             | Go                                     | Python            | Web           | Docs           |
| ------------------ | -------------------------------------- | ----------------- | ------------- | -------------- |
| **install**        | -                                      | `install-python`  | `install-web` | `install-docs` |
| **dev**            | `dev-go`                               | -                 | `dev-web`     | `dev-docs`     |
| **dev (variant)**  | `dev-go-lidar`<br>`dev-go-kill-server` | -                 | -             | -              |
| **test**           | `test-go`                              | `test-python`     | `test-web`    | -              |
| **test (variant)** | -                                      | `test-python-cov` | -             | -              |
| **format**         | `format-go`                            | `format-python`   | `format-web`  | -              |
| **lint**           | `lint-go`                              | `lint-python`     | `lint-web`    | -              |
| **clean**          | -                                      | `clean-python`    | -             | -              |

### Aggregate Targets

- `test` - Run all tests (Go + Python + Web)
- `format` - Format all code (Go + Python + Web)
- `lint` - Lint all code (Go + Python + Web), fails if formatting needed

### Build Targets (Go cross-compilation)

- `build-radar-linux` - Build for Linux ARM64 (no pcap)
- `build-radar-linux-pcap` - Build for Linux ARM64 with pcap
- `build-radar-mac` - Build for macOS ARM64 with pcap
- `build-radar-mac-intel` - Build for macOS AMD64 with pcap
- `build-radar-local` - Build for local development with pcap
- `build-tools` - Build sweep tool
- `build-web` - Build web frontend (SvelteKit)
- `build-docs` - Build documentation site (Eleventy)

### Deployment Targets

- `setup-radar` - Install server on this host (requires sudo)

### PDF Generator Targets

- `pdf-report` - Generate PDF from config file
- `pdf-config` - Create example configuration
- `pdf-demo` - Run configuration demo
- `pdf-test` - Run PDF tests (alias for test-python)
- `pdf` - Convenience alias for pdf-report

### Utility Targets

- `log-go-tail` - Tail most recent Go server log
- `log-go-cat` - Cat most recent Go server log

### Data Visualisation Targets

- `plot-noise-sweep` - Generate noise sweep line plot
- `plot-multisweep` - Generate multi-parameter grid plot
- `plot-noise-buckets` - Generate per-noise bar charts
- `stats-live` - Capture live LiDAR grid snapshots
- `stats-pcap` - Capture snapshots during PCAP replay

### API Shortcut Targets (LiDAR HTTP API)

Grid operations: `api-grid-status`, `api-grid-reset`, `api-grid-heatmap`
Snapshots: `api-snapshot`, `api-snapshots`
Acceptance metrics: `api-acceptance`, `api-acceptance-reset`
Parameters: `api-params`, `api-params-set`
Export: `api-persist`, `api-export-snapshot`, `api-export-next-frame`
Status & PCAP: `api-status`, `api-start-pcap`, `api-stop-pcap`, `api-switch-data-source`

Run `make help` or `make` to see all available targets with descriptions.

## Contributing

We welcome contributions! Please see **[CONTRIBUTING.md](CONTRIBUTING.md)** for:

- Development workflow (Go + Python + Web)
- Testing requirements
- Code style guidelines
- Pull request process

## License

Apache License 2.0 - See [LICENSE](LICENSE) for details.

## Community

[![join-us-on-discord](https://github.com/user-attachments/assets/fa329256-aee7-4751-b3c4-d35bdf9287f5)](https://discord.gg/XXh6jXVFkt)

Join our Discord community to discuss the project, get help, and contribute to making streets safer.

## Privacy & Ethics

This project is designed with privacy as a core principle:

- ✅ No license plate recognition
- ✅ No video recording
- ✅ No personally identifiable information

The goal is to empower communities to make data-driven decisions about street safety without compromising individual privacy.
