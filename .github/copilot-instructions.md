# velocity.report Development Guidelines

## Project Overview

**velocity.report** is a privacy-focused traffic monitoring system for neighborhood change-makers. The system measures vehicle speeds using radar/LIDAR sensors and provides visualization and reporting capabilities.

**Technology Stack:**

- **Go** - High-performance server, data collection, HTTP API
- **Python** - PDF report generation with LaTeX
- **Svelte/SvelteKit** - Web frontend for real-time visualization
- **SQLite** - Local data storage
- **Eleventy** - Documentation site

**Architecture:** Multi-component system with Go server handling sensor data, Python tools for report generation, and a Svelte web frontend. See `ARCHITECTURE.md` for detailed design.

## Core Principles

**Privacy-First Design:**

- No cameras, no license plates, no PII collection
- Velocity measurements only
- Local-only data storage (no cloud transmission)
- User data ownership

**DRY (Don't Repeat Yourself):**

- Avoid duplication across documentation and configuration files
- Reference canonical sources instead of copying
- Link to authoritative docs rather than summarizing
- Update ALL relevant documentation when changing functionality

## Quality Standards (MANDATORY)

**Before any commit or change:**

```bash
make lint      # Check all code formatting (Go, Python, Web)
make format    # Auto-format all code
make test      # Run all test suites
```

All three commands must pass before committing changes.

## Setup and Build

**Initial Setup:**

```bash
# Clone repository
git clone git@github.com:banshee-data/velocity.report.git
cd velocity.report

# Go server (local development)
make build-radar-local        # Build with pcap support

# Python PDF generator (one-time setup)
make install-python           # Creates .venv/ and installs dependencies

# Web frontend
make install-web              # Install pnpm/npm dependencies

# Documentation site
make install-docs             # Install Eleventy dependencies
```

**Testing:**

```bash
make test                     # Run all tests (Go + Python + Web)
make test-go                  # Go unit tests only
make test-python              # Python tests only
make test-web                 # Web tests only
```

**Development Servers:**

```bash
make dev-go                   # Start Go server (radar disabled)
make dev-web                  # Start web dev server (localhost:5173)
make dev-docs                 # Start docs dev server
```

## Path Conventions

**Critical Paths (use hyphen, not dot):**

- Data directory: `/var/lib/velocity-report/`
- Database: `/var/lib/velocity-report/sensor_data.db`
- Service binary: `/usr/local/bin/velocity-report`
- Python venv: `.venv/` (root level, shared across all Python tools)

**Common Pitfall:** Ensure `/var/lib/velocity-report` (hyphen) not `/var/lib/velocity.report` (dot)

## Repository Structure

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
│   └── monitoring/           # System monitoring
├── web/                      # Svelte web frontend
│   ├── src/                  # Frontend source code
│   └── static/               # Static assets
├── tools/                    # Python tooling
│   └── pdf-generator/        # PDF report generation
├── docs/                     # Documentation site (Eleventy)
├── data/                     # Test data and fixtures
└── scripts/                  # Utility scripts
```

## Documentation Updates

**When changing functionality, update ALL relevant docs:**

- Main `README.md`
- Component READMEs: `cmd/radar/README.md`, `tools/pdf-generator/README.md`, `web/README.md`
- `ARCHITECTURE.md` for system design changes
- `docs/src/guides/setup.md` for user-facing setup instructions

## Active Migrations

**Python venv consolidation (In Progress):**

- Moving from dual-venv to unified `.venv/` at repository root
- Old: `tools/pdf-generator/.venv` (being phased out)
- New: `.venv/` at root (target state)
- Use `.venv/` paths in all new code and documentation

## Build System

**Makefile pattern:** `<action>-<subsystem>[-<variant>]`

- 59 documented targets available
- Cross-compilation for ARM64 (Raspberry Pi 4)
- See `make help` for all targets

## Task Guidance for Copilot

**Good Tasks for Copilot:**

- Bug fixes in Go, Python, or Web code
- Adding unit tests or improving test coverage
- Updating documentation (README, component docs, guides)
- Code refactoring within well-defined boundaries
- Adding new API endpoints with clear specifications
- UI enhancements with specific requirements
- Accessibility improvements
- Technical debt reduction

**Tasks to Avoid Assigning to Copilot:**

- **Complex, broadly scoped changes** requiring cross-component knowledge
  - Example: Refactoring data flow across Go server, Python PDF generator, and web frontend
- **Security-critical changes** involving authentication, data privacy, or sensor data handling
  - Example: Modifying database schema for sensor data, changing data retention policies
- **Production-critical issues** or incident response
  - Example: Emergency fixes to running systems, debugging production crashes
- **Ambiguous tasks** without clear requirements or acceptance criteria
  - Example: "Make the UI better" or "Improve performance" without metrics
- **Large architectural changes** requiring design consistency across components
  - Example: Migrating from SQLite to PostgreSQL, changing radar data processing pipeline
- **Tasks requiring deep domain knowledge** of radar/LIDAR sensor systems
  - Example: Tuning signal processing algorithms, calibrating sensor thresholds

## Validation Protocol

**For Code Changes:**

1. Run `make format` to auto-format code
2. Run `make lint` to check for issues
3. Run `make test` to verify tests pass
4. Build relevant component(s) to ensure no compilation errors
5. Manually test changes if they affect runtime behavior
6. Update documentation if functionality changed

**For Go Changes:**

```bash
make format-go && make lint-go && make test-go
make build-radar-local   # Verify build
# Note: If build fails due to missing pcap dependencies:
#   - Debian/Ubuntu: sudo apt-get install libpcap-dev
#   - macOS: brew install libpcap
#   - Windows: Use vcpkg (vcpkg install libpcap) or build without pcap (make build-radar-linux)
# Or use make build-radar-linux for build without pcap support
```

**For Python Changes:**

```bash
make format-python && make lint-python && make test-python
```

**For Web Changes:**

```bash
make format-web && make lint-web && make test-web
make build-web           # Verify production build
```
