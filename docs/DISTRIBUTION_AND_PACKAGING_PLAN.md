# velocity.report Distribution and Packaging Plan

**Document Version:** 1.0  
**Date:** 2025-11-20  
**Status:** Proposed Architecture

---

## Quick Reference

### 30-Second Pitch

**Problem:** Multiple scattered tools, no release process, complex Python setup  
**Solution:** Single `velocity-report` binary with subcommands + optional power-user tools  
**Result:** Professional distribution with one-line install and GitHub releases

### Recommended Architecture (Hybrid Model)

```
velocity-report                        # Main binary (all users)
  ├── serve      (default)            # Start server
  ├── migrate    (existing)           # DB migrations
  ├── pdf        (new)                # Generate PDF
  ├── backfill   (moved)              # Transit backfill
  └── version    (new)                # Version info

velocity-report-sweep                  # Power user tool
velocity-report-backfill-rings         # Developer tool
```

### Key Changes Summary

| What | Before | After |
|------|--------|-------|
| **Main binary** | `cmd/radar/` | `cmd/velocity-report/` |
| **Start server** | `velocity-report` | `velocity-report serve` (or just `velocity-report`) |
| **PDF generation** | `PYTHONPATH=... python -m ...` | `velocity-report pdf config.json` |
| **Sweep tool** | `./app-sweep` | `velocity-report-sweep` |
| **Installation** | Manual build + scp + script | `curl install.sh \| sudo bash` |
| **Releases** | None | GitHub Releases with CI/CD |

### Timeline Overview

- **Phase 1:** Go restructure (1-2 weeks)
- **Phase 2:** Python integration (1 week)
- **Phase 3:** GitHub Actions (3-5 days)
- **Phase 4:** Install script (3-5 days)
- **Phase 5:** Testing (1 week)
- **Total: 4-6 weeks to v1.0.0**

---

## Executive Summary

This document outlines a comprehensive plan to structure, bundle, and distribute the velocity.report suite, which currently consists of:

1. **Go binary** (`cmd/radar`) with migrate subcommands, radar handler, LIDAR handler
2. **Python PDF generator** (`tools/pdf-generator`)
3. **Go sweep tool** (`cmd/sweep`) for LIDAR parameter tuning
4. **Additional utilities** (transit-backfill, backfill_ring_elevations, grid-heatmap)
5. **Web frontend** (Svelte/SvelteKit, embedded in Go binary)

The plan evaluates multiple distribution approaches and recommends a **hybrid model** that balances ease of use, maintainability, and alignment with Go/Python best practices.

---

## Table of Contents

1. [Quick Reference](#quick-reference)
2. [Current State Analysis](#1-current-state-analysis)
3. [User Personas & Use Cases](#2-user-personas--use-cases)
4. [Distribution Approach Tradeoffs](#3-distribution-approach-tradeoffs)
5. [Recommended Architecture](#4-recommended-architecture)
6. [Implementation Plan](#5-implementation-plan)
7. [Migration Guide](#6-migration-guide)
8. [Testing & Validation](#7-testing--validation)
9. [Future Enhancements](#8-future-enhancements)

---

## 1. Current State Analysis

### 1.1 Components Inventory

| Component | Type | Location | Build Output | Current Distribution |
|-----------|------|----------|-------------|---------------------|
| **Main Server** | Go | `cmd/radar/` | `velocity-report-*` | Manual build + setup script |
| **Migrate CLI** | Go subcommand | `internal/db/migrate_cli.go` | Embedded in main | Part of main binary |
| **Sweep Tool** | Go | `cmd/sweep/` | `app-sweep` | Manual build (`make build-tools`) |
| **PDF Generator** | Python | `tools/pdf-generator/` | Python module | PYTHONPATH + Makefile |
| **Transit Backfill** | Go | `cmd/transit-backfill/` | Not built by default | Manual `go build` |
| **Ring Elevations Backfill** | Go | `cmd/tools/backfill_ring_elevations/` | Not built by default | Manual `go build` |
| **Grid Heatmap** | Python | `tools/grid-heatmap/` | Python scripts | Manual invocation |
| **Web Frontend** | Svelte | `web/` | Embedded in Go binary | `//go:embed` in assets.go |

### 1.2 Current Build Process

**For Production (Raspberry Pi ARM64):**
```bash
# On development machine
make build-radar-linux         # Creates velocity-report-linux-arm64
make build-tools               # Creates app-sweep

# Copy to Pi and install
scp velocity-report-linux-arm64 pi:/tmp/
ssh pi "cd /path/to/repo && sudo make setup-radar"
```

**Current Issues:**
- ❌ No single "release" command to build all user-facing tools
- ❌ Python tools require manual PYTHONPATH setup
- ❌ Utility tools (transit-backfill, backfill_ring_elevations) not easily discoverable
- ❌ No versioned releases or GitHub Release automation
- ❌ Web build must succeed before Go binary can build (assets.go dependency)
- ❌ No standardized installation path for Python scripts
- ❌ Unclear which tools are for end-users vs. developers

### 1.3 Deployment Model

**Target Environment:** Raspberry Pi 4 (ARM64 Linux)

**Current Setup:**
```
/usr/local/bin/velocity-report          # Main server binary
/var/lib/velocity-report/               # Data directory
  └── sensor_data.db                    # SQLite database
/etc/systemd/system/velocity-report.service  # Systemd unit

# Python tools run via Makefile from git repo
# No clear path for "installed" Python scripts
```

---

## 2. User Personas & Use Cases

### 2.1 End User (Neighborhood Monitor)

**Profile:**
- Non-technical community advocate
- Uses pre-built Raspberry Pi image or simple install script
- Needs: Radar monitoring + occasional PDF reports

**Expected Workflow:**
```bash
# One-time setup
curl -sSL https://velocity.report/install.sh | sudo bash

# Generate PDF report
velocity-report pdf-generate --config report.json
# or
pdf-generate --config report.json  # if separate command
```

**Needs from Distribution:**
- ✅ Single binary for radar server
- ✅ Simple command to generate PDF reports
- ✅ Web UI accessible at http://localhost:8080
- ✅ Systemd service auto-start
- ⚠️ No need for sweep tool or developer utilities

### 2.2 Power User (Traffic Engineer)

**Profile:**
- Technical user with traffic engineering background
- Runs parameter sweeps on LIDAR data
- Creates custom reports and analyses

**Expected Workflow:**
```bash
# Run parameter sweep
velocity-report sweep --mode multi --iterations 50 --output results.csv
# or
sweep --mode multi --iterations 50 --output results.csv

# Generate heatmap visualizations
grid-heatmap --input sweep.csv --output heatmap.png

# Backfill transit data
velocity-report migrate up
velocity-report transit-backfill --start 2024-01-01 --end 2024-12-31
```

**Needs from Distribution:**
- ✅ Access to all tools (sweep, grid-heatmap, backfill utilities)
- ✅ Python tools available as commands
- ✅ Documentation for each tool
- ⚠️ Comfortable with CLI tools and configuration

### 2.3 Developer (Contributor)

**Profile:**
- Go/Python developer contributing to project
- Needs full source access and build tooling

**Expected Workflow:**
```bash
# Clone and build
git clone https://github.com/banshee-data/velocity.report.git
cd velocity.report
make build-radar-local
make install-python
make test

# Develop and test
make dev-go
make dev-web
make lint
```

**Needs from Distribution:**
- ✅ Source repository with Makefile
- ✅ Development convenience targets
- ✅ All build targets (local, linux, mac, pcap/no-pcap)
- ⚠️ No need for "packaged" distribution

---

## 3. Distribution Approach Tradeoffs

### Approach A: Monolithic Binary (Go with Embedded Python)

**Description:** Embed Python interpreter and PDF generator into Go binary using tools like `go-python` or by bundling Python as subprocess.

**Pros:**
- ✅ Single binary distribution
- ✅ Simple installation (`cp binary /usr/local/bin/`)
- ✅ No Python dependency management for users

**Cons:**
- ❌ **MAJOR:** Go-Python interop is complex and brittle
- ❌ Cross-compilation nightmare (Python C extensions)
- ❌ Large binary size (50+ MB with Python runtime)
- ❌ LaTeX still required externally (defeats "single binary" goal)
- ❌ Updates require recompiling entire stack
- ❌ Against Go and Python best practices

**Verdict:** ❌ **Not Recommended** - Technical complexity and maintainability issues outweigh benefits.

---

### Approach B: Multi-Binary Suite (Go Binaries + Python Scripts)

**Description:** Build multiple Go binaries and install Python scripts alongside them. All tools available in `$PATH`.

**Structure:**
```
/usr/local/bin/
  ├── velocity-report           # Main server (with migrate subcommand)
  ├── velocity-report-sweep     # Sweep tool
  ├── velocity-report-pdf       # Python wrapper script
  ├── velocity-report-backfill  # Transit backfill
  └── grid-heatmap              # Python script

/usr/local/share/velocity-report/
  ├── python/                   # Python package
  │   └── pdf_generator/
  └── docs/
```

**Pros:**
- ✅ All tools discoverable via `velocity-report-*` naming
- ✅ Each tool independently updatable
- ✅ Clear separation of concerns
- ✅ Standard Unix convention (`/usr/local/{bin,share}`)
- ✅ Python tools via wrapper scripts (handles PYTHONPATH)

**Cons:**
- ⚠️ Multiple files to install/uninstall
- ⚠️ Python venv still required (but manageable)
- ⚠️ Name collision risk (minor: use prefix)

**Verdict:** ✅ **Strong Candidate** - Follows Unix conventions, clear tool separation.

---

### Approach C: Subcommand Architecture (Single Entry Point)

**Description:** Single `velocity-report` binary with subcommands for all functionality.

**Structure:**
```bash
velocity-report serve          # Start radar server
velocity-report migrate up     # Database migrations (already exists)
velocity-report sweep          # Run parameter sweep
velocity-report pdf            # Generate PDF report (calls Python)
velocity-report backfill       # Transit backfill
velocity-report grid-heatmap   # Heatmap visualization
```

**Pros:**
- ✅ Single binary for all Go functionality
- ✅ Familiar CLI pattern (git, kubectl, docker)
- ✅ Consistent help/version output
- ✅ Easy to discover features (`velocity-report --help`)
- ✅ Python tools invoked as subprocess (keeps separation)

**Cons:**
- ⚠️ Python tools still separate (but callable via subcommand)
- ⚠️ Larger binary (includes all Go code)
- ⚠️ Some functionality overlap with `make` commands

**Verdict:** ✅ **Strong Candidate** - Modern CLI pattern, excellent discoverability.

---

### Approach D: Hybrid Model (Subcommands + Separate Utilities)

**Description:** Core functionality in main binary, specialized tools as separate binaries or scripts.

**Structure:**
```
velocity-report                 # Main binary with subcommands
  ├── serve                     # Radar server (default)
  ├── migrate                   # Migrations (exists)
  ├── pdf                       # PDF generation (calls Python)
  └── backfill                  # Transit backfill

velocity-report-sweep           # Separate binary (advanced tool)
grid-heatmap                    # Python script (advanced tool)
backfill-ring-elevations        # Utility script (developer tool)
```

**Categorization:**
- **Core Tools** (in main binary): serve, migrate, pdf, basic backfill
- **Power User Tools** (separate): sweep, grid-heatmap
- **Developer Tools** (not installed): ring elevations backfill, dev scripts

**Pros:**
- ✅ Best of both worlds (simple + powerful)
- ✅ Core tools always available
- ✅ Advanced tools opt-in (install separately or use from repo)
- ✅ Clear user journey (basic → power → dev)
- ✅ Smaller main binary

**Cons:**
- ⚠️ Slightly more complex distribution
- ⚠️ Need to decide categorization for each tool

**Verdict:** ✅ **RECOMMENDED** - Balances simplicity for end users with power for advanced users.

---

## 4. Recommended Architecture

### 4.1 Overview: Hybrid Distribution Model

We recommend **Approach D (Hybrid Model)** with the following structure:

#### Primary Binary: `velocity-report`

**Subcommands:**
```
velocity-report serve          # Start radar/LIDAR server (default)
velocity-report migrate        # Database migrations (EXISTING)
velocity-report pdf            # Generate PDF report (wrapper for Python)
velocity-report backfill       # Transit backfill (basic mode)
velocity-report version        # Show version info
velocity-report help           # Show help
```

#### Secondary Binaries (Power Users):
- `velocity-report-sweep` - LIDAR parameter sweep tool
- `velocity-report-backfill-rings` - Ring elevation backfill (rare use)

#### Python Scripts (Installed):
- `pdf-generator/pdf_generator/` - Python package
- `grid-heatmap/` scripts - Visualization tools

### 4.2 Directory Layout (Installed System)

```
/usr/local/bin/
  ├── velocity-report                    # Main binary (~30 MB)
  ├── velocity-report-sweep              # Sweep binary (~15 MB)
  └── velocity-report-backfill-rings     # Utility binary (~15 MB)

/usr/local/share/velocity-report/
  ├── python/                            # Python packages
  │   ├── pdf_generator/
  │   │   ├── cli/
  │   │   ├── core/
  │   │   └── tests/
  │   └── grid_heatmap/
  │       └── plot_grid_heatmap.py
  ├── web/                               # Static web assets (optional, embedded)
  └── docs/                              # Documentation

/var/lib/velocity-report/                # Data directory
  └── sensor_data.db                     # SQLite database

/etc/systemd/system/
  └── velocity-report.service            # Systemd unit

/etc/velocity-report/                    # Configuration (optional)
  └── config.yaml
```

### 4.3 Python Environment Strategy

**Problem:** Python scripts need dependencies (matplotlib, PyLaTeX, etc.)

**Solution:** Use virtual environment in shared location:

```
/usr/local/share/velocity-report/python/.venv/
  ├── bin/
  │   ├── python3
  │   └── pip
  └── lib/python3.x/site-packages/
```

**Python wrapper scripts:**
```bash
#!/bin/bash
# /usr/local/bin/velocity-report (pdf subcommand)
VENV=/usr/local/share/velocity-report/python/.venv
PYTHONPATH=/usr/local/share/velocity-report/python
exec $VENV/bin/python3 -m pdf_generator.cli.main "$@"
```

**Alternative for end users:** System Python with pip packages:
```bash
pip3 install velocity-report-pdf-generator
```

### 4.4 Command Structure Design

#### Main Binary: `velocity-report`

**cmd/radar/radar.go** becomes **cmd/velocity-report/main.go**:
```go
func main() {
    if len(os.Args) > 1 {
        subcommand := os.Args[1]
        switch subcommand {
        case "serve":
            runServe()  // Current radar.go main() logic
        case "migrate":
            runMigrate()  // EXISTING: internal/db/migrate_cli.go
        case "pdf":
            runPDF()  // NEW: wrapper for Python script
        case "backfill":
            runBackfill()  // Move from cmd/transit-backfill/
        case "version":
            printVersion()
        case "help", "-h", "--help":
            printHelp()
        default:
            fmt.Fprintf(os.Stderr, "Unknown command: %s\n", subcommand)
            os.Exit(1)
        }
    } else {
        // Default behavior: start server
        runServe()
    }
}
```

#### Sweep Binary: `velocity-report-sweep`

Rename `cmd/sweep/` → `cmd/velocity-report-sweep/`:
- Binary name: `velocity-report-sweep`
- Purpose: LIDAR parameter tuning (advanced users)
- Distribution: Included in releases, optional install

#### Python PDF Subcommand: `velocity-report pdf`

**Implementation:**
```go
// cmd/velocity-report/pdf.go
func runPDF() {
    // Find Python interpreter
    pythonPath := findPython()
    
    // Find pdf_generator module
    modulePath := findPDFGenerator()
    
    // Set environment
    env := os.Environ()
    env = append(env, "PYTHONPATH="+modulePath)
    
    // Execute Python CLI
    cmd := exec.Command(pythonPath, "-m", "pdf_generator.cli.main", os.Args[2:]...)
    cmd.Env = env
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    cmd.Run()
}
```

**Fallback chain:**
1. Check `/usr/local/share/velocity-report/python/.venv/bin/python3`
2. Check `$VELOCITY_REPORT_PYTHON` environment variable
3. Check system `python3`
4. Error with helpful message

---

## 5. Implementation Plan

### Phase 1: Restructure Go Binaries (1-2 weeks)

**Goal:** Create unified `velocity-report` binary with subcommands.

**Tasks:**

1. **Rename and restructure main entry point**
   - Move `cmd/radar/` → `cmd/velocity-report/`
   - Rename `radar.go` → `main.go` with subcommand dispatcher
   - Extract server logic to `serve.go`
   - Keep existing flags for `serve` subcommand

2. **Integrate existing subcommands**
   - `migrate` - Already exists in `internal/db/migrate_cli.go` ✅
   - Keep current integration pattern

3. **Add new subcommands**
   - `velocity-report pdf` - Wrapper for Python PDF generator
   - `velocity-report backfill` - Move from `cmd/transit-backfill/`
   - `velocity-report version` - Show version/build info
   - `velocity-report help` - Unified help system

4. **Update build targets in Makefile**
   ```makefile
   build-radar-linux:
       GOOS=linux GOARCH=arm64 go build -o velocity-report-linux-arm64 ./cmd/velocity-report
   
   build-sweep:
       GOOS=linux GOARCH=arm64 go build -o velocity-report-sweep-linux-arm64 ./cmd/velocity-report-sweep
   
   build-backfill-rings:
       GOOS=linux GOARCH=arm64 go build -o velocity-report-backfill-rings-linux-arm64 ./cmd/velocity-report-backfill-rings
   
   build-all:
       $(MAKE) build-radar-linux
       $(MAKE) build-sweep
       $(MAKE) build-backfill-rings
   ```

5. **Update systemd service file**
   ```ini
   [Service]
   # Change from:
   # ExecStart=/usr/local/bin/velocity-report --db-path /var/lib/velocity-report/sensor_data.db
   # To:
   ExecStart=/usr/local/bin/velocity-report serve --db-path /var/lib/velocity-report/sensor_data.db
   ```

6. **Update assets.go**
   - Move `assets.go` from root to `cmd/velocity-report/`
   - Update package declaration
   - Fix import paths in server code

7. **Testing**
   - Verify `velocity-report serve` behaves exactly like old binary
   - Verify `velocity-report migrate up` works (existing tests pass)
   - Add integration tests for new subcommands

**Migration for existing deployments:**
- Old binary still works (starts server by default)
- New binary backward compatible (no args = serve)
- Document migration: `velocity-report` → `velocity-report serve`

---

### Phase 2: Python Tool Integration (1 week)

**Goal:** Make Python tools installable and callable from Go binary.

**Tasks:**

1. **Create Python wrapper in Go**
   - Implement `cmd/velocity-report/pdf.go`
   - Add `runPDF()` function with Python discovery logic
   - Handle arguments pass-through

2. **Update PDF generator for standalone installation**
   - Add `tools/pdf-generator/setup.py` or use `pyproject.toml`
   - Create console_scripts entry point:
     ```python
     [project.scripts]
     pdf-generator = "pdf_generator.cli.main:main"
     ```
   - Test: `pip install -e tools/pdf-generator/`

3. **Create installation script for Python tools**
   ```bash
   # scripts/install-python-tools.sh
   #!/bin/bash
   INSTALL_DIR=${1:-/usr/local/share/velocity-report/python}
   VENV_DIR=$INSTALL_DIR/.venv
   
   # Create venv
   python3 -m venv $VENV_DIR
   
   # Install dependencies
   $VENV_DIR/bin/pip install -r requirements.txt
   
   # Copy Python packages
   cp -r tools/pdf-generator/pdf_generator $INSTALL_DIR/
   cp -r tools/grid-heatmap $INSTALL_DIR/
   ```

4. **Update Makefile targets**
   ```makefile
   install-python-system:
       sudo ./scripts/install-python-tools.sh
   
   pdf-report:
       # Option 1: Use installed tools
       velocity-report pdf $(CONFIG)
       # Option 2: Use development setup (existing)
       cd $(PDF_DIR) && PYTHONPATH=. ../../$(VENV_PYTHON) -m pdf_generator.cli.main $(CONFIG)
   ```

5. **Update setup-radar-host.sh**
   - Add step to install Python tools
   - Set up venv in `/usr/local/share/velocity-report/python/.venv`
   - Install dependencies from `requirements.txt`

6. **Testing**
   - Test `velocity-report pdf` from Go binary
   - Test standalone `pdf-generator` command
   - Verify Python path discovery logic
   - Test on Raspberry Pi ARM64

---

### Phase 3: GitHub Releases Automation (3-5 days)

**Goal:** Automate building and releasing versioned binaries.

**Tasks:**

1. **Create GitHub Actions release workflow**
   
   File: `.github/workflows/release.yml`
   ```yaml
   name: Release
   
   on:
     push:
       tags:
         - 'v*'
   
   jobs:
     build-binaries:
       runs-on: ubuntu-latest
       strategy:
         matrix:
           include:
             - goos: linux
               goarch: arm64
               output: velocity-report-linux-arm64
             - goos: darwin
               goarch: arm64
               output: velocity-report-mac-arm64
             - goos: darwin
               goarch: amd64
               output: velocity-report-mac-amd64
       
       steps:
         - uses: actions/checkout@v4
         
         - name: Set up Go
           uses: actions/setup-go@v5
           with:
             go-version: '1.25'
         
         - name: Set up Node.js
           uses: actions/setup-node@v4
           with:
             node-version: '20'
         
         - name: Build web frontend
           run: |
             cd web
             npm install
             npm run build
         
         - name: Build Go binary
           env:
             GOOS: ${{ matrix.goos }}
             GOARCH: ${{ matrix.goarch }}
           run: |
             go build -tags=pcap -o ${{ matrix.output }} ./cmd/velocity-report
         
         - name: Upload artifact
           uses: actions/upload-artifact@v4
           with:
             name: ${{ matrix.output }}
             path: ${{ matrix.output }}
     
     build-sweep:
       runs-on: ubuntu-latest
       strategy:
         matrix:
           include:
             - goos: linux
               goarch: arm64
               output: velocity-report-sweep-linux-arm64
       
       steps:
         - uses: actions/checkout@v4
         
         - name: Set up Go
           uses: actions/setup-go@v5
           with:
             go-version: '1.25'
         
         - name: Build sweep binary
           env:
             GOOS: ${{ matrix.goos }}
             GOARCH: ${{ matrix.goarch }}
           run: |
             go build -o ${{ matrix.output }} ./cmd/velocity-report-sweep
         
         - name: Upload artifact
           uses: actions/upload-artifact@v4
           with:
             name: ${{ matrix.output }}
             path: ${{ matrix.output }}
     
     package-python:
       runs-on: ubuntu-latest
       steps:
         - uses: actions/checkout@v4
         
         - name: Set up Python
           uses: actions/setup-python@v5
           with:
             python-version: '3.12'
         
         - name: Package Python tools
           run: |
             tar czf velocity-report-python-tools.tar.gz \
               tools/pdf-generator/pdf_generator \
               tools/grid-heatmap \
               requirements.txt
         
         - name: Upload artifact
           uses: actions/upload-artifact@v4
           with:
             name: velocity-report-python-tools.tar.gz
             path: velocity-report-python-tools.tar.gz
     
     create-release:
       needs: [build-binaries, build-sweep, package-python]
       runs-on: ubuntu-latest
       steps:
         - uses: actions/checkout@v4
         
         - name: Download all artifacts
           uses: actions/download-artifact@v4
         
         - name: Create checksums
           run: |
             for file in velocity-report-* *.tar.gz; do
               sha256sum "$file" >> SHA256SUMS.txt
             done
         
         - name: Create GitHub Release
           uses: softprops/action-gh-release@v2
           with:
             files: |
               velocity-report-*
               *.tar.gz
               SHA256SUMS.txt
             body: |
               ## Installation
               
               ### Linux (Raspberry Pi ARM64)
               ```bash
               curl -LO https://github.com/banshee-data/velocity.report/releases/download/${{ github.ref_name }}/velocity-report-linux-arm64
               chmod +x velocity-report-linux-arm64
               sudo mv velocity-report-linux-arm64 /usr/local/bin/velocity-report
               ```
               
               ### macOS (Apple Silicon)
               ```bash
               curl -LO https://github.com/banshee-data/velocity.report/releases/download/${{ github.ref_name }}/velocity-report-mac-arm64
               chmod +x velocity-report-mac-arm64
               sudo mv velocity-report-mac-arm64 /usr/local/bin/velocity-report
               ```
               
               ### Python Tools
               ```bash
               curl -LO https://github.com/banshee-data/velocity.report/releases/download/${{ github.ref_name }}/velocity-report-python-tools.tar.gz
               sudo tar xzf velocity-report-python-tools.tar.gz -C /usr/local/share/velocity-report/
               ```
               
               ## What's Changed
               See [CHANGELOG.md](https://github.com/banshee-data/velocity.report/blob/main/CHANGELOG.md)
   ```

2. **Create version management**
   
   File: `internal/version/version.go`
   ```go
   package version
   
   import "runtime/debug"
   
   var (
       Version   = "dev"
       GitCommit = "unknown"
       BuildTime = "unknown"
   )
   
   func init() {
       if info, ok := debug.ReadBuildInfo(); ok {
           for _, setting := range info.Settings {
               switch setting.Key {
               case "vcs.revision":
                   GitCommit = setting.Value[:8]
               case "vcs.time":
                   BuildTime = setting.Value
               }
           }
       }
   }
   
   func Full() string {
       return Version + " (" + GitCommit + ", " + BuildTime + ")"
   }
   ```
   
   Use in `velocity-report version`:
   ```go
   func runVersion() {
       fmt.Printf("velocity.report %s\n", version.Full())
   }
   ```

3. **Update Makefile for release builds**
   ```makefile
   VERSION ?= $(shell git describe --tags --always --dirty)
   LDFLAGS = -X github.com/banshee-data/velocity.report/internal/version.Version=$(VERSION)
   
   build-radar-linux:
       GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o velocity-report-linux-arm64 ./cmd/velocity-report
   
   release-tag:
       @echo "Creating release tag $(VERSION)"
       git tag -a $(VERSION) -m "Release $(VERSION)"
       git push origin $(VERSION)
   ```

4. **Create CHANGELOG.md**
   - Document all changes per release
   - Reference in GitHub Release body

5. **Testing**
   - Create test tag: `git tag v0.1.0-alpha && git push origin v0.1.0-alpha`
   - Verify workflow runs
   - Download and test binaries from release
   - Verify checksums

---

### Phase 4: Installation Script & Documentation (3-5 days)

**Goal:** Create one-command installation for end users.

**Tasks:**

1. **Create unified installation script**
   
   File: `scripts/install.sh`
   ```bash
   #!/bin/bash
   # Install script for velocity.report
   # Usage: curl -sSL https://velocity.report/install.sh | sudo bash
   
   set -euo pipefail
   
   # Configuration
   INSTALL_DIR="/usr/local"
   DATA_DIR="/var/lib/velocity-report"
   SHARE_DIR="$INSTALL_DIR/share/velocity-report"
   VERSION="${VERSION:-latest}"
   
   # Detect architecture
   ARCH=$(uname -m)
   case "$ARCH" in
       aarch64|arm64) GOARCH="arm64" ;;
       x86_64|amd64) GOARCH="amd64" ;;
       *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
   esac
   
   # Detect OS
   OS=$(uname -s | tr '[:upper:]' '[:lower:]')
   
   BINARY="velocity-report-${OS}-${GOARCH}"
   
   echo "Installing velocity.report ${VERSION} for ${OS}/${GOARCH}"
   
   # Download binary
   if [ "$VERSION" = "latest" ]; then
       DOWNLOAD_URL="https://github.com/banshee-data/velocity.report/releases/latest/download/${BINARY}"
   else
       DOWNLOAD_URL="https://github.com/banshee-data/velocity.report/releases/download/${VERSION}/${BINARY}"
   fi
   
   echo "Downloading from ${DOWNLOAD_URL}..."
   curl -fsSL "$DOWNLOAD_URL" -o /tmp/velocity-report
   chmod +x /tmp/velocity-report
   
   # Install binary
   echo "Installing binary to ${INSTALL_DIR}/bin/velocity-report..."
   mv /tmp/velocity-report "${INSTALL_DIR}/bin/velocity-report"
   
   # Download Python tools (optional)
   read -p "Install Python tools (PDF generator)? [Y/n] " -n 1 -r
   echo
   if [[ ! $REPLY =~ ^[Nn]$ ]]; then
       echo "Installing Python tools..."
       mkdir -p "$SHARE_DIR"
       
       PYTHON_URL="https://github.com/banshee-data/velocity.report/releases/download/${VERSION}/velocity-report-python-tools.tar.gz"
       curl -fsSL "$PYTHON_URL" | tar xz -C "$SHARE_DIR"
       
       # Set up Python venv
       python3 -m venv "${SHARE_DIR}/python/.venv"
       "${SHARE_DIR}/python/.venv/bin/pip" install -r "${SHARE_DIR}/requirements.txt"
       
       echo "Python tools installed."
   fi
   
   # Create service user and data directory
   if ! id velocity &>/dev/null; then
       useradd --system --no-create-home --shell /usr/sbin/nologin velocity
   fi
   
   mkdir -p "$DATA_DIR"
   chown velocity:velocity "$DATA_DIR"
   
   # Install systemd service (Linux only)
   if [ "$OS" = "linux" ] && command -v systemctl &>/dev/null; then
       cat > /etc/systemd/system/velocity-report.service <<EOF
   [Unit]
   Description=Velocity.report radar monitor service
   After=network.target
   
   [Service]
   User=velocity
   Group=velocity
   Type=simple
   ExecStart=${INSTALL_DIR}/bin/velocity-report serve --db-path ${DATA_DIR}/sensor_data.db
   WorkingDirectory=${DATA_DIR}
   Restart=on-failure
   RestartSec=5
   StandardOutput=journal
   StandardError=journal
   SyslogIdentifier=velocity-report
   
   [Install]
   WantedBy=multi-user.target
   EOF
       
       systemctl daemon-reload
       systemctl enable velocity-report.service
       
       echo "Systemd service installed. Start with: sudo systemctl start velocity-report"
   fi
   
   echo "Installation complete!"
   echo "Run 'velocity-report --help' to get started."
   ```

2. **Update setup-radar-host.sh**
   - Simplify to download from GitHub releases
   - Remove local build requirement
   - Add version selection

3. **Create comprehensive installation guide**
   
   File: `docs/INSTALLATION.md`
   - One-line install (recommended)
   - Manual installation steps
   - Raspberry Pi-specific instructions
   - Docker installation (future)
   - Building from source

4. **Update README.md**
   - Add "Quick Install" section at top
   - Link to detailed installation guide
   - Update architecture diagram

5. **Create user guides**
   - `docs/USER_GUIDE.md` - End user documentation
   - `docs/POWER_USER_GUIDE.md` - Advanced tools (sweep, heatmaps)
   - `docs/DEVELOPER_GUIDE.md` - Contributing, building from source

6. **Update website (docs/)**
   - Installation page
   - Download page with version selector
   - Getting started tutorial

---

### Phase 5: Testing & Rollout (1 week)

**Goal:** Validate new distribution model in production-like environments.

**Tasks:**

1. **Create test environments**
   - Fresh Raspberry Pi 4 with Raspbian
   - Ubuntu 22.04 ARM64 VM
   - macOS ARM64 (Apple Silicon)

2. **Test installation methods**
   - [ ] One-line install script
   - [ ] Manual binary download
   - [ ] GitHub release download
   - [ ] Building from source

3. **Test core workflows**
   - [ ] `velocity-report serve` starts server
   - [ ] `velocity-report migrate up` runs migrations
   - [ ] `velocity-report pdf` generates report
   - [ ] `velocity-report-sweep` runs parameter sweep
   - [ ] Web UI accessible at http://localhost:8080
   - [ ] Systemd service starts on boot

4. **Test upgrade path**
   - Install v0.1.0
   - Upgrade to v0.2.0
   - Verify data preserved
   - Verify services restart

5. **Performance validation**
   - Binary size reasonable (<50MB)
   - Startup time acceptable (<2s)
   - Memory usage acceptable (<200MB)

6. **Documentation review**
   - All commands documented
   - Examples work as written
   - Troubleshooting guide complete

7. **Alpha release**
   - Tag `v0.1.0-alpha`
   - Announce to Discord community
   - Gather feedback
   - Iterate

8. **Beta release**
   - Tag `v0.1.0-beta`
   - Broader testing
   - Update based on feedback

9. **Stable release**
   - Tag `v1.0.0`
   - Update all documentation
   - Announce release

---

## 6. Migration Guide

### 6.1 For Existing Deployments

**Current Setup:**
```
/usr/local/bin/velocity-report    # Old binary (cmd/radar)
/var/lib/velocity-report/sensor_data.db
```

**Migration Steps:**

1. **Backup database**
   ```bash
   sudo systemctl stop velocity-report
   sudo cp /var/lib/velocity-report/sensor_data.db /var/lib/velocity-report/sensor_data.db.backup
   ```

2. **Download new binary**
   ```bash
   VERSION=v1.0.0
   curl -LO https://github.com/banshee-data/velocity.report/releases/download/${VERSION}/velocity-report-linux-arm64
   chmod +x velocity-report-linux-arm64
   ```

3. **Test new binary**
   ```bash
   # Test migrate command
   ./velocity-report-linux-arm64 migrate status --db-path /var/lib/velocity-report/sensor_data.db
   
   # Test serve (don't background yet)
   ./velocity-report-linux-arm64 serve --db-path /var/lib/velocity-report/sensor_data.db --disable-radar
   # Ctrl+C to stop
   ```

4. **Update systemd service**
   ```bash
   sudo vi /etc/systemd/system/velocity-report.service
   # Change:
   #   ExecStart=/usr/local/bin/velocity-report --db-path /var/lib/velocity-report/sensor_data.db
   # To:
   #   ExecStart=/usr/local/bin/velocity-report serve --db-path /var/lib/velocity-report/sensor_data.db
   
   sudo systemctl daemon-reload
   ```

5. **Install new binary**
   ```bash
   sudo mv velocity-report-linux-arm64 /usr/local/bin/velocity-report
   sudo chown root:root /usr/local/bin/velocity-report
   sudo chmod 755 /usr/local/bin/velocity-report
   ```

6. **Restart service**
   ```bash
   sudo systemctl start velocity-report
   sudo systemctl status velocity-report
   ```

7. **Verify operation**
   ```bash
   # Check logs
   sudo journalctl -u velocity-report -f
   
   # Test web UI
   curl http://localhost:8080/
   
   # Test migrate command
   velocity-report migrate status --db-path /var/lib/velocity-report/sensor_data.db
   ```

**Rollback Plan:**
```bash
# If issues occur, restore old binary
sudo systemctl stop velocity-report
sudo cp /path/to/old/velocity-report /usr/local/bin/velocity-report
# Restore old service file (remove "serve" from ExecStart)
sudo systemctl daemon-reload
sudo systemctl start velocity-report
```

### 6.2 For Developers

**Current Workflow:**
```bash
make build-radar-local
./velocity-report-local --disable-radar
```

**New Workflow:**
```bash
make build-radar-local
./velocity-report-local serve --disable-radar
# OR (serve is default)
./velocity-report-local --disable-radar
```

**Makefile Changes:**
- `build-radar-*` targets now build from `cmd/velocity-report/`
- `dev-go` target updated to use `serve` subcommand
- New `build-all` target builds all binaries
- New `install-system` target for local testing of installed layout

**Testing Changes:**
- Update integration tests to use subcommand syntax
- Add tests for new subcommands
- Verify backward compatibility (no args = serve)

### 6.3 For Python Tools

**Current Workflow:**
```bash
cd tools/pdf-generator
PYTHONPATH=. ../../.venv/bin/python -m pdf_generator.cli.main config.json
```

**New Workflow (Development):**
```bash
# Option 1: Via Go wrapper
velocity-report pdf config.json

# Option 2: Direct Python (still works)
cd tools/pdf-generator
PYTHONPATH=. ../../.venv/bin/python -m pdf_generator.cli.main config.json

# Option 3: Makefile (still works)
make pdf-report CONFIG=config.json
```

**New Workflow (Production):**
```bash
# After installation via install.sh
velocity-report pdf /path/to/config.json
```

**No Breaking Changes:**
- Existing Makefile commands still work
- PYTHONPATH-based invocation still works
- Development workflow unchanged

---

## 7. Testing & Validation

### 7.1 Unit Tests

**New tests to add:**

1. **Subcommand Dispatcher** (`cmd/velocity-report/main_test.go`)
   ```go
   func TestSubcommandDispatch(t *testing.T) {
       tests := []struct {
           args []string
           wantSubcommand string
       }{
           {[]string{"velocity-report"}, "serve"},
           {[]string{"velocity-report", "serve"}, "serve"},
           {[]string{"velocity-report", "migrate", "up"}, "migrate"},
           {[]string{"velocity-report", "pdf", "config.json"}, "pdf"},
       }
       // ...
   }
   ```

2. **Python Discovery** (`cmd/velocity-report/pdf_test.go`)
   ```go
   func TestFindPython(t *testing.T) {
       // Test venv discovery
       // Test system python fallback
       // Test error handling
   }
   ```

3. **Version Command** (`cmd/velocity-report/version_test.go`)
   ```go
   func TestVersionCommand(t *testing.T) {
       // Verify version output format
       // Verify git commit included
   }
   ```

### 7.2 Integration Tests

**Add to existing test suite:**

1. **Subcommand Integration** (`integration_test.go`)
   ```go
   func TestServeSubcommand(t *testing.T) {
       // Start server via "serve" subcommand
       // Verify HTTP endpoints respond
       // Stop server
   }
   
   func TestMigrateSubcommand(t *testing.T) {
       // Run migrate up
       // Verify schema version
       // Run migrate down
   }
   
   func TestPDFSubcommand(t *testing.T) {
       // Create test config
       // Run velocity-report pdf
       // Verify PDF generated
   }
   ```

2. **Backward Compatibility** (`compat_test.go`)
   ```go
   func TestBackwardCompatNoArgs(t *testing.T) {
       // velocity-report (no args)
       // Should start server (old behavior)
   }
   ```

### 7.3 End-to-End Tests

**Manual testing checklist:**

- [ ] Fresh Raspberry Pi install via `install.sh`
- [ ] Start server: `sudo systemctl start velocity-report`
- [ ] Access web UI: http://localhost:8080
- [ ] Generate PDF: `velocity-report pdf config.json`
- [ ] Run migrate: `velocity-report migrate status`
- [ ] Run sweep: `velocity-report-sweep --mode noise`
- [ ] Service survives reboot
- [ ] Logs to systemd journal
- [ ] Data persists across restarts

### 7.4 Performance Tests

**Benchmarks:**

1. **Binary Size**
   - Target: <50 MB for main binary
   - Target: <20 MB for sweep binary

2. **Startup Time**
   - Target: <2 seconds to HTTP ready
   - Measure with systemd timing

3. **Memory Usage**
   - Idle: <100 MB
   - Under load: <500 MB

4. **Python Invocation Overhead**
   - `velocity-report pdf` vs direct Python
   - Target: <100ms overhead

---

## 8. Future Enhancements

### 8.1 Docker Distribution

**Benefits:**
- Simplified deployment
- Consistent environment
- Easy updates

**Architecture:**
```dockerfile
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y \
    python3 \
    python3-pip \
    texlive-latex-base \
    && rm -rf /var/lib/apt/lists/*

COPY velocity-report /usr/local/bin/
COPY python/ /usr/local/share/velocity-report/python/
RUN pip3 install -r /usr/local/share/velocity-report/python/requirements.txt

VOLUME /var/lib/velocity-report
EXPOSE 8080

CMD ["velocity-report", "serve", "--db-path", "/var/lib/velocity-report/sensor_data.db"]
```

**Usage:**
```bash
docker run -d \
  --name velocity-report \
  --device=/dev/ttyUSB0 \
  -v /var/lib/velocity-report:/var/lib/velocity-report \
  -p 8080:8080 \
  velocity-report:latest
```

### 8.2 Raspberry Pi Image

**Pre-built SD card image with:**
- Raspbian OS
- velocity.report pre-installed
- Systemd service enabled
- Web UI accessible on boot

**Distribution:**
- Download from GitHub releases
- Flash with Raspberry Pi Imager
- Boot and configure via web UI

### 8.3 Package Managers

**APT/DEB Package:**
```bash
sudo apt-get install velocity-report
```

**Homebrew (macOS):**
```bash
brew tap banshee-data/tap
brew install velocity-report
```

**Implementation:**
- Create `.deb` package in GitHub Actions
- Host on GitHub Releases or packagecloud.io
- Create Homebrew formula

### 8.4 Web-Based Configuration

**Goal:** Replace JSON config files with web UI.

**Features:**
- Upload database via browser
- Configure report parameters
- Generate and download PDF
- No CLI required for basic use

**Architecture:**
```
velocity-report serve --enable-config-ui
# Access at http://localhost:8080/config
```

### 8.5 Plugin System

**Allow third-party extensions:**
```bash
velocity-report plugin install lidar-advanced-analytics
velocity-report plugin list
velocity-report lidar-advanced-analytics analyze --input data.csv
```

---

## Appendix A: File Layout Comparison

### Current Structure
```
velocity.report/
├── cmd/
│   ├── radar/                      # Main server
│   ├── sweep/                      # Sweep tool
│   ├── transit-backfill/          # Backfill utility
│   └── tools/
│       └── backfill_ring_elevations/
├── tools/
│   ├── pdf-generator/             # Python PDF generator
│   └── grid-heatmap/              # Python heatmaps
└── internal/
    ├── api/
    ├── db/
    └── radar/

Binary outputs (after build):
├── velocity-report-linux-arm64    # Main server
└── app-sweep                       # Sweep tool
```

### Proposed Structure
```
velocity.report/
├── cmd/
│   ├── velocity-report/           # Main binary (was cmd/radar)
│   │   ├── main.go               # Subcommand dispatcher
│   │   ├── serve.go              # Server logic
│   │   ├── pdf.go                # PDF wrapper
│   │   ├── backfill.go           # Backfill (moved from separate cmd)
│   │   └── version.go            # Version info
│   ├── velocity-report-sweep/    # Sweep tool (renamed)
│   └── velocity-report-backfill-rings/  # Utility (renamed)
├── tools/
│   ├── pdf-generator/            # Python PDF generator
│   └── grid-heatmap/             # Python heatmaps
└── internal/
    ├── api/
    ├── db/
    ├── radar/
    └── version/                   # New: version management

Binary outputs (after build):
├── velocity-report-linux-arm64                 # Main binary
├── velocity-report-sweep-linux-arm64          # Sweep binary
└── velocity-report-backfill-rings-linux-arm64 # Utility binary
```

### Installed System Layout
```
/usr/local/bin/
├── velocity-report                    # Main binary
├── velocity-report-sweep              # Sweep binary (optional)
└── velocity-report-backfill-rings     # Utility binary (optional)

/usr/local/share/velocity-report/
├── python/
│   ├── .venv/                        # Python virtual environment
│   ├── pdf_generator/                # Python package
│   ├── grid_heatmap/                 # Python scripts
│   └── requirements.txt              # Python dependencies
└── docs/                             # Documentation

/var/lib/velocity-report/
└── sensor_data.db                    # SQLite database

/etc/systemd/system/
└── velocity-report.service           # Systemd service
```

---

## Appendix B: Command Reference

### Current Commands (Before Migration)

**Main server:**
```bash
velocity-report --db-path /path/to/db          # Start server
velocity-report migrate up --db-path /path     # Database migration
```

**PDF generator:**
```bash
cd tools/pdf-generator
PYTHONPATH=. ../../.venv/bin/python -m pdf_generator.cli.main config.json
# OR
make pdf-report CONFIG=config.json
```

**Sweep tool:**
```bash
./app-sweep --mode multi --iterations 30
```

**Utilities:**
```bash
go run cmd/transit-backfill/main.go --db sensor_data.db --start 2024-01-01 --end 2024-12-31
go run cmd/tools/backfill_ring_elevations/main.go --db sensor_data.db
```

### Proposed Commands (After Migration)

**Main binary:**
```bash
velocity-report                                 # Start server (default)
velocity-report serve                           # Start server (explicit)
velocity-report migrate up                      # Database migration
velocity-report pdf config.json                 # Generate PDF report
velocity-report backfill --start 2024-01-01 --end 2024-12-31  # Backfill transits
velocity-report version                         # Show version
velocity-report help                            # Show help
```

**Additional binaries:**
```bash
velocity-report-sweep --mode multi --iterations 30           # Parameter sweep
velocity-report-backfill-rings --db sensor_data.db          # Ring elevations
```

**Python tools (if installed separately):**
```bash
pdf-generator config.json                       # Direct Python command
grid-heatmap --input data.csv --output plot.png # Heatmap visualization
```

---

## Appendix C: Release Checklist

**Pre-release:**
- [ ] All tests pass (`make test`)
- [ ] Linting clean (`make lint`)
- [ ] Documentation updated
- [ ] CHANGELOG.md updated
- [ ] Version bumped in code
- [ ] Migration guide tested

**Release process:**
1. Create release branch: `git checkout -b release-v1.0.0`
2. Update version: `sed -i 's/Version = "dev"/Version = "1.0.0"/' internal/version/version.go`
3. Commit: `git commit -am "Release v1.0.0"`
4. Tag: `git tag -a v1.0.0 -m "Release v1.0.0"`
5. Push: `git push origin v1.0.0`
6. GitHub Actions builds and creates release
7. Test release artifacts
8. Announce on Discord/website

**Post-release:**
- [ ] Verify GitHub Release created
- [ ] Download and test binaries
- [ ] Update website documentation
- [ ] Announce on social media
- [ ] Monitor for issues

---

## Appendix D: Breaking Changes Summary

### For End Users

**✅ No Breaking Changes**
- Existing installations continue to work
- `velocity-report` (no args) still starts server
- All existing flags preserved

**✨ New Features**
- Subcommands for clarity: `velocity-report serve`
- Built-in PDF generation: `velocity-report pdf`
- Version command: `velocity-report version`

### For Developers

**⚠️ Minor Breaking Changes**
- `cmd/radar/` moved to `cmd/velocity-report/`
- Binary name unchanged: `velocity-report-linux-arm64`
- Import paths unchanged (only cmd/ structure changed)

**✅ Backward Compatible**
- All Makefile targets work
- All tests pass
- Development workflow unchanged

### For Advanced Users

**✨ Improvements**
- `app-sweep` renamed to `velocity-report-sweep`
- Better tool discoverability
- Consistent naming convention

**✅ No Functionality Lost**
- All tools still available
- All features preserved
- All flags compatible

---

## Conclusion

This distribution and packaging plan provides a clear path to transform velocity.report from a developer-focused build system to a user-friendly, installable suite of tools. The hybrid model (Approach D) balances simplicity for end users with power for advanced users, while maintaining full compatibility with the existing development workflow.

**Key Benefits:**
- ✅ Single binary for core functionality
- ✅ Subcommand architecture for clarity
- ✅ Python tools integrated but separate
- ✅ GitHub Releases for easy distribution
- ✅ One-line installation script
- ✅ Backward compatible with existing deployments
- ✅ Follows Go and Python best practices

**Next Steps:**
1. Review and approve this plan
2. Begin Phase 1: Restructure Go binaries
3. Iterate based on community feedback
4. Release alpha version for testing

**Timeline Estimate:**
- Phase 1: 1-2 weeks (Go restructure)
- Phase 2: 1 week (Python integration)
- Phase 3: 3-5 days (GitHub Actions)
- Phase 4: 3-5 days (Install script & docs)
- Phase 5: 1 week (Testing & rollout)
- **Total: 4-6 weeks to v1.0.0**

This plan prioritizes maintainability, user experience, and alignment with industry best practices while minimizing disruption to existing users and developers.
