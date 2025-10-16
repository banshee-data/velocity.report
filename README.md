# velocity.report

A privacy-focused traffic logging tool for neighborhood change-makers.

Measure vehicle speeds, make streets safer.

[![join-us-on-discord](https://github.com/user-attachments/assets/fa329256-aee7-4751-b3c4-d35bdf9287f5)](https://discord.gg/XXh6jXVFkt)

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

**velocity.report** is a complete neighborhood traffic monitoring system built with privacy as the foundation. The system consists of three main components:

- **Go Server** - High-performance data collection and API server
- **Python PDF Generator** - Professional PDF report generation with LaTeX
- **Web Frontend** - Real-time data visualization (Svelte)

The system collects vehicle speed data from radar/LIDAR sensors, stores it in SQLite, and provides multiple ways to visualize and report on the data—all while maintaining complete privacy (no license plate recognition, no video recording).

## Quick Start

### For Go Server Development

```sh
git clone git@github.com:banshee-data/velocity.report.git
cd velocity.report
make build-local
./app-local -dev
```

If an existing SQLite database is available, place it in `./sensor_data.db`

### For PDF Report Generation

See **[tools/pdf-generator/README.md](tools/pdf-generator/README.md)** for detailed instructions.

Quick version:

```sh
cd tools/pdf-generator
make pdf-setup              # One-time setup
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
│   └── tools/                # Utility tools
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
├── docs/                     # Documentation
├── scripts/                  # Development scripts
└── static/                   # Static server assets
```

## Architecture

### Data Flow

```
Sensors (Radar/LIDAR) → Go Server → SQLite Database
                                          ↓
                                    ┌─────┴──────┐
                                    ↓            ↓
                              Python PDF     Web Frontend
                              Generator      (Svelte)
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
- Creates charts and visualizations with matplotlib
- Processes statistical summaries
- Highly configurable via JSON
- Comprehensive test suite

**3. Web Frontend** (`/web/`)
- Real-time data visualization
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
make build-local
./app-local -dev
```

Run tests:

```sh
make test
```

Build for production (Raspberry Pi):

```sh
make build
# or manually:
GOARCH=arm64 GOOS=linux go build -o app .
```

### Python PDF Generator Development

```sh
cd tools/pdf-generator
make pdf-setup        # Create venv, install dependencies
make pdf-test         # Run test suite
make pdf-demo         # Run interactive demo
```

### Pre-commit Hooks

Enable basic formatting hooks for Python code:

```sh
pip install pre-commit          # Or run scripts/dev-setup.sh
pre-commit install              # Register git hooks
pre-commit run --all-files      # Optional: run across the repo once
```

**What runs on commit:**
- File hygiene (trailing whitespace, large files, etc.)
- Python formatting (ruff + black) for PDF generator code

**What doesn't run on commit:**
- Go formatting/linting - Use your editor/IDE or run `make fmt` manually
- Web linting - Runs in CI on PRs (saves time on local commits)

This keeps commits fast while catching obvious formatting issues early.

### Web Frontend Development

```sh
cd web
pnpm install
pnpm dev
```

See **[web/README.md](web/README.md)** for details.

## Deployment

### Go Server (Raspberry Pi)

The Go server runs as a systemd service on Raspberry Pi.

**Deploy new version:**

```sh
# 1. Build for ARM64
make build

# 2. Copy to Raspberry Pi
scp app pi@raspberrypi:/path/to/deployment/

# 3. Restart service
ssh pi@raspberrypi
sudo systemctl stop go-sensor.service
sudo cp /path/to/deployment/app /usr/local/bin/velocity-server
sudo systemctl start go-sensor.service
```

**Monitor service:**

```sh
# View logs
sudo journalctl -u go-sensor.service -f

# Check status
sudo systemctl status go-sensor.service
```

**Service configuration:**

See `velocity-report.service` for systemd service configuration.

### Python PDF Generator

The PDF generator is deployed as a Python package via PYTHONPATH:

```sh
cd tools/pdf-generator
make pdf-setup
# PDF generator is now ready at tools/pdf-generator/pdf_generator/
```

No installation required - use PYTHONPATH method as documented in [tools/pdf-generator/README.md](tools/pdf-generator/README.md).

## Documentation

- **[ARCHITECTURE.md](ARCHITECTURE.md)** - System architecture and component relationships
- **[tools/pdf-generator/README.md](tools/pdf-generator/README.md)** - PDF generator documentation
- **[web/README.md](web/README.md)** - Web frontend documentation
- **[docs/README.md](docs/README.md)** - Documentation site
- **[data/README.md](data/README.md)** - Data directory and database documentation

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
make pdf-test-coverage
```

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
