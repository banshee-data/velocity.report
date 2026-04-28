# velocity.report distribution and packaging plan

- **Document Version:** 1.1
- **Status:** Proposed Architecture (partially superseded — see banner)
- **Layers:** Cross-cutting (deployment infrastructure)
- **Canonical:** [distribution-packaging.md](../platform/operations/distribution-packaging.md)
- **Drift:** several Phase-1/3/4 decisions are superseded by [deploy-versioned-binary-plan.md](./deploy-versioned-binary-plan.md) and [deploy-nginx-removal-plan.md](./deploy-nginx-removal-plan.md). Strikethrough sections below preserve the earlier intent for context.

---

> **Drift summary (2026-04):** this plan was written assuming (a) a `cmd/velocity-report/` Cobra-style binary with subcommands, (b) `velocity-ctl` as a separate artifact, (c) sibling utility binaries (`velocity-report-sweep`, `velocity-report-backfill-rings`), and (d) a flat install at `/usr/local/bin/`. The current direction is a single busybox-style `velocity` binary living at `/opt/velocity-report/versions/<v>/velocity`, with `/usr/local/bin/velocity` as the canonical entry point, `/usr/local/bin/velocity-report` as the server-compatibility alias, and shell lifecycle aliases in `/etc/profile.d/velocity-aliases.sh`. Treat [deploy-versioned-binary-plan.md](./deploy-versioned-binary-plan.md) as canonical for the promoted public command surface; the strikethroughs below preserve older packaging intent for context.

> **Architecture, design rationale, and current-state analysis:** see [distribution-packaging.md](../platform/operations/distribution-packaging.md) for the chosen subcommand model, component inventory, user personas, tradeoff analysis, and system layout.

---

## 5. Implementation plan

### Phase 1: ~~restructure Go binaries~~ build the busybox-style `velocity` binary (1-2 weeks)

> **Updated direction:** instead of `cmd/velocity-report/` with Cobra-style subcommands, the canonical entry point is `cmd/velocity/main.go` with `os.Args[0]` dispatch. `velocity-report` survives as the server-compatibility alias. `velocity-ctl` should not remain a first-class public alias; if it is needed during migration, it is only a redirect bridge into the `device` namespace. See [deploy-versioned-binary-plan.md](./deploy-versioned-binary-plan.md) for the command strategy and on-disk layout.

**Goal:** ~~Create unified `velocity-report` binary with subcommands.~~ Create a single `velocity` multi-call binary, preserve `velocity-report` as the compatibility alias, and keep host lifecycle outside the binary via shell wrappers.

**Tasks:**

1. **Rename and restructure main entry point**
   - ~~Move [cmd/radar/](../../cmd/radar) → `cmd/velocity-report/`~~ → **Move to `cmd/velocity/` (busybox shape)**; relocate the existing `cmd/radar/*.go` files under `internal/cmd/radar/` and export `Main(args []string)` so the dispatcher can call them.
   - ~~Rename `radar.go` → `main.go` with subcommand dispatcher~~ → New `cmd/velocity/main.go` switches on `filepath.Base(os.Args[0])` first, then on `os.Args[1]`.
   - Extract server logic to ~~`serve.go`~~ `internal/cmd/radar/`
   - Keep existing flags; the no-arg invocation still starts the server.

2. **Integrate existing subcommands**
   - `migrate` - Already exists in [internal/db/migrate_cli.go](../../internal/db/migrate_cli.go) ✅
   - Keep current integration pattern; expose as the `migrate` applet under both `velocity migrate …` and `velocity-report migrate …` (sudoers compatibility).

3. **Add new subcommands**
   - `velocity report pdf` - ~~Wrapper for Python PDF generator~~ → **Native Go PDF pipeline** (the Python pipeline is deprecated; see [internal/report/](../../internal/report)). The applet shells into the in-process pipeline rather than `exec`-ing Python.
   - `velocity data backfill` - Move operator-facing backfills into the `data` namespace rather than creating more top-level verbs
   - `velocity version` - Show version and build info
   - `velocity help` - Unified help system
   - `velocity tune sweep` - Fold sweep into a tuning namespace instead of shipping a sibling binary

   ~~Note: upgrade/rollback/backup are handled by `velocity-ctl`
   ([cmd/velocity-ctl/](../../cmd/velocity-ctl)) which ships as a separate binary. These may be
   absorbed into `velocity-report` in a future release if eliminating one
   binary is worth the mixed privilege model.~~

   **Decision (2026-04):** absorb, but do not expose `ctl` as the public noun. Installed-version lifecycle moves under `velocity device ...`; service lifecycle stays outside the binary as `velocity-status`, `velocity-start`, `velocity-stop`, `velocity-bounce`, and `velocity-log`. If `velocity-ctl` is shipped during migration, it is only a redirect bridge to `velocity device ...`, not part of the promoted surface. See [deploy-versioned-binary-plan.md](./deploy-versioned-binary-plan.md#overall-cli-strategy).

4. **Update build targets in Makefile**

   > **Updated direction:** one target, one artifact. `build-radar-local`, `build-radar-linux`, `build-ctl`, the sweep target, and the backfill-rings target collapse into `make build-velocity` (linux-arm64 + local variants). Sweep and backfill-rings become applets of the single binary, not standalone Go programs. See [deploy-versioned-binary-plan.md](./deploy-versioned-binary-plan.md#files-to-change).

   Historical targets in the earlier plan:

   ```text
   build-radar-linux
      GOOS=linux GOARCH=arm64 go build -o velocity-report-$(VERSION)-linux-arm64 ./cmd/velocity-report

   build-sweep
      GOOS=linux GOARCH=arm64 go build -o velocity-report-sweep-linux-arm64 ./cmd/velocity-report-sweep

   build-backfill-rings
      GOOS=linux GOARCH=arm64 go build -o velocity-report-backfill-rings-linux-arm64 ./cmd/velocity-report-backfill-rings

   build-all
      umbrella target in the earlier multi-binary plan
   ```

   New shape:

   | Target                 | Output                                                                                                     |
   | ---------------------- | ---------------------------------------------------------------------------------------------------------- |
   | `build-velocity`       | `GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o velocity-$(VERSION)-linux-arm64 ./cmd/velocity` |
   | `build-velocity-local` | local-arch debug build for `make dev-go`                                                                   |

5. **Update systemd service file**

   > **Updated direction:** the no-arg invocation continues to start the server (the busybox dispatcher routes argv[0]=velocity-report and no further args to the radar applet). Two real changes vs. the old plan: (a) the binary at `/usr/local/bin/velocity-report` is a symlink chain into `/opt/velocity-report/current/velocity`; (b) the bind port is `:80` per [deploy-nginx-removal-plan.md](./deploy-nginx-removal-plan.md) — `AmbientCapabilities=CAP_NET_BIND_SERVICE` is added.

   ~~[Service]~~

   ~~# Change from:~~

   ~~# ExecStart=/usr/local/bin/velocity-report --db-path /var/lib/velocity-report/sensor_data.db~~

   ~~# To:~~

   ~~ExecStart=/usr/local/bin/velocity-report serve --db-path /var/lib/velocity-report/sensor_data.db~~

   New shape:

   ```
   [Service]
   User=velocity
   AmbientCapabilities=CAP_NET_BIND_SERVICE
   CapabilityBoundingSet=CAP_NET_BIND_SERVICE
   ExecStart=/usr/local/bin/velocity-report --db-path /var/lib/velocity-report/sensor_data.db
   ```

   No `serve` keyword needed; the symlink dispatches the no-arg form to the server applet.

6. **Update assets.go**
   - ~~Move [assets.go](../../assets.go) from root to `cmd/velocity-report/`~~ → Move to `internal/cmd/radar/` (or wherever the radar applet's `Main` lives). The `embed.FS` directives are unchanged; only the package and import paths shift.
   - Update package declaration
   - Fix import paths in server code

7. **Testing**
   - Verify `velocity serve` starts the server, and `velocity-report` with no args remains the compatibility form
   - Verify `velocity data migrate up` works, and `velocity-report migrate up` remains a temporary compatibility form while old scripts are still supported
   - Add integration tests for new subcommands

**Migration for existing deployments:**

- Old binary still works (starts server by default)
- New binary backward compatible (no args = serve)
- Document migration: keep `velocity-report` as the no-arg compatibility form, but promote `velocity serve` as the canonical explicit command

---

### Phase 2: ~~Python tool integration~~ (no longer applies)

> **Superseded.** The Python PDF generator under `tools/pdf-generator/` is **deprecated** (per [CLAUDE.md](../../CLAUDE.md): "Retained for reference only. Not used in deployed systems since v0.5. Will be removed in v0.6."). The shipping pipeline is the native Go pipeline at [internal/report/](../../internal/report). There is no Python venv in the deployed image, no `pip install`, no `velocity-report pdf` Python wrapper. The `pdf` applet calls the Go pipeline directly.

> **Historical context only.** The numbered list below is preserved to explain what the earlier Python-packaging design contained. It is not current implementation guidance.

~~**Goal:** Make Python tools installable and callable from Go binary.~~

**Historical tasks from the superseded design:**

1. **Create Python wrapper in Go**
   - Implement `cmd/velocity-report/pdf.go`
   - Add `runPDF()` function with Python discovery logic
   - Handle arguments pass-through

2. **Update PDF generator for standalone installation**
   - Add `tools/pdf-generator/setup.py` or use `pyproject.toml`
   - Create console_scripts entry point:
     [project.scripts]
     pdf-generator = "pdf_generator.cli.main:main"
   - Test: `pip install -e tools/pdf-generator/`

3. **Create installation script for Python tools**
   - !/bin/bash: `INSTALL_DIR=${1:-/usr/local/share/velocity-report/python}`
   - `VENV_DIR=$INSTALL_DIR/.venv`
   - Create venv: `python3 -m venv $VENV_DIR`
   - Install dependencies: `$VENV_DIR/bin/pip install -r requirements.txt`
   - Copy Python packages: `cp -r tools/pdf-generator/pdf_generator $INSTALL_DIR/`
   - `cp -r tools/grid-heatmap $INSTALL_DIR/`

4. **Update Makefile targets**

   | Target/Variable         | Description |
   | ----------------------- | ----------- |
   | `install-python-system` |             |
   | `pdf-report`            |             |

5. **Update setup-radar-host.sh**
   - Add step to install Python tools
   - Set up venv in `/usr/local/share/velocity-report/python/.venv`
   - Install dependencies from [requirements.txt](../../requirements.txt)

6. **Testing**
   - Test `velocity-report pdf` from Go binary
   - Test standalone `pdf-generator` command
   - Verify Python path discovery logic
   - Test on Raspberry Pi ARM64

---

### Phase 3: GitHub releases automation (3-5 days)

> **Updated direction:** one Go artifact per platform per channel, not three. The `build-sweep` and `package-python` jobs go away. `release.json` lists a single `velocity-linux-arm64` URL + SHA256 per channel. See [deploy-versioned-binary-plan.md](./deploy-versioned-binary-plan.md).

**Goal:** Automate building and releasing versioned binaries.

**Tasks:**

1.  **Create GitHub Actions release workflow**

    File: `.github/workflows/release.yml` <!-- link-ignore -->

    name: Release

    on:
    push:
    tags: - "v\*"

    jobs:
    build-binaries:
    runs-on: ubuntu-latest
    strategy:
    matrix:
    include: - goos: linux
    goarch: arm64
    output: velocity-report-${VERSION_NUM}-linux-arm64
                - goos: darwin
                  goarch: arm64
                  output: velocity-report-${VERSION_NUM}-darwin-arm64 - goos: darwin
    goarch: amd64
    output: velocity-report-${VERSION_NUM}-darwin-amd64

           steps:
             - uses: actions/checkout@v4

             - name: Derive version (strip v prefix)
               run: echo "VERSION_NUM=${GITHUB_REF_NAME#v}" >> "$GITHUB_ENV"

             - name: Set up Go
               uses: actions/setup-go@v5
               with:
                 go-version-file: go.mod

             - name: Set up Node.js
               uses: actions/setup-node@v4
               with:
                 node-version: "20"

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
                 go-version: "1.25"

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
                 python-version: "3.12"

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
                   Download the ARM64 binary, make executable, and move to /usr/local/bin/velocity-report.

                   ### macOS (Apple Silicon)
                   Download the Darwin ARM64 binary, make executable, and move to /usr/local/bin/velocity-report.

                   ### Python Tools
                   Download velocity-report-python-tools.tar.gz and extract to /usr/local/share/velocity-report/.

                   ## What's Changed
                   See [CHANGELOG.md](https://github.com/banshee-data/velocity.report/blob/main/CHANGELOG.md)

2.  **Create version management**

    File: [internal/version/version.go](../../internal/version/version.go) — defines `Version`, `GitSHA`, and `BuildTime` variables (defaulting to "dev"/"unknown"). The Makefile injects them with `-X github.com/banshee-data/velocity.report/internal/version.<Name>=...`. Exports `Print(binary)` for the CLI version output.

    Use in `velocity version`: call `version.Print("velocity")`; compatibility forms pass their displayed binary name.

3.  **Update Makefile for release builds**

    Add Makefile variables and targets: `VERSION` (from `git describe`), `LDFLAGS` (injecting version into binary via `-X`), `build-radar-linux` (cross-compile with ldflags for ARM64), and `release-tag` (tag and push).

4.  **Create CHANGELOG.md**
    - Document all changes per release
    - Reference in GitHub Release body

5.  **Testing**
    - Create test tag: `git tag v0.1.0-alpha && git push origin v0.1.0-alpha`
    - Verify workflow runs
    - Download and test binaries from release
    - Verify checksums

---

### Phase 4: ~~installation script & documentation~~ (largely superseded)

> **Updated direction:** the install script no longer drops a binary at `/usr/local/bin/velocity-report`. It downloads the single artifact, places it at `/opt/velocity-report/versions/<v>/velocity`, sets the `current` symlink, and creates `/usr/local/bin/velocity` plus `/usr/local/bin/velocity-report` into the chain. If a `velocity-ctl` bridge is needed during migration, it is temporary and not part of the promoted surface. The systemd unit it writes uses port `:80` and `AmbientCapabilities=CAP_NET_BIND_SERVICE`. The Python venv prompt and download are removed (Python pipeline deprecated). See [deploy-versioned-binary-plan.md](./deploy-versioned-binary-plan.md#image-and-install-script-impact) and [deploy-nginx-removal-plan.md](./deploy-nginx-removal-plan.md).

> **Historical context only.** The script and task list below describe the earlier flat-install design (binary directly under `/usr/local/bin/`, optional Python venv, ExecStart with `serve` keyword). They are preserved to explain what was previously contemplated. They are **not** current implementation guidance; the canonical install path is the one in [deploy-versioned-binary-plan.md](./deploy-versioned-binary-plan.md).

~~**Goal:** Create one-command installation for end users.~~

**Historical tasks from the superseded design:**

1. **Create unified installation script**

   File: `scripts/install.sh`
   - Usage: curl -sSL https://velocity.report/install.sh | sudo bash: `set -euo pipefail`
   - Configuration: `INSTALL_DIR="/usr/local"`
   - `DATA_DIR="/var/lib/velocity-report"`
   - `SHARE_DIR="$INSTALL_DIR/share/velocity-report"`
   - `VERSION="${VERSION:-latest}"`
   - Detect architecture: `ARCH=$(uname -m)`
   - `case "$ARCH" in`
   - `aarch64|arm64) GOARCH="arm64" ;;`
   - `x86_64|amd64) GOARCH="amd64" ;;`
   - `*) echo "Unsupported architecture: $ARCH"; exit 1 ;;`
   - `esac`
   - Detect OS: `OS=$(uname -s | tr '[:upper:]' '[:lower:]')`
   - `BINARY="velocity-report-${OS}-${GOARCH}"`
   - `echo "Installing velocity.report ${VERSION} for ${OS}/${GOARCH}"`
   - Download binary: `if [ "$VERSION" = "latest" ]; then`
   - `DOWNLOAD_URL="https://github.com/banshee-data/velocity.report/releases/latest/download/${BINARY}"`
   - `else`
   - `DOWNLOAD_URL="https://github.com/banshee-data/velocity.report/releases/download/${VERSION}/${BINARY}"`
   - `fi`
   - `echo "Downloading from ${DOWNLOAD_URL}..."`
   - `curl -fsSL "$DOWNLOAD_URL" -o /tmp/velocity-report`
   - `chmod +x /tmp/velocity-report`
   - Install binary: `echo "Installing binary to ${INSTALL_DIR}/bin/velocity-report..."`
   - `mv /tmp/velocity-report "${INSTALL_DIR}/bin/velocity-report"`
   - Download Python tools (optional): `read -p "Install Python tools (PDF generator)? [Y/n] " -n 1 -r`
   - `echo`
   - `if [[ ! $REPLY =~ ^[Nn]$ ]]; then`
   - `echo "Installing Python tools..."`
   - `mkdir -p "$SHARE_DIR"`
   - `PYTHON_URL="https://github.com/banshee-data/velocity.report/releases/download/${VERSION}/velocity-report-python-tools...`
   - `curl -fsSL "$PYTHON_URL" | tar xz -C "$SHARE_DIR"`
   - Set up Python venv: `python3 -m venv "${SHARE_DIR}/python/.venv"`
   - `"${SHARE_DIR}/python/.venv/bin/pip" install -r "${SHARE_DIR}/requirements.txt"`
   - `echo "Python tools installed."`
   - `fi`
   - Create service user and data directory: `if ! id velocity &>/dev/null; then`
   - `useradd --system --no-create-home --shell /usr/sbin/nologin velocity`
   - `fi`
   - `mkdir -p "$DATA_DIR"`
   - `chown velocity:velocity "$DATA_DIR"`
   - Install systemd service (Linux only): `if [ "$OS" = "linux" ] && command -v systemctl &>/dev/null; then`
   - `cat > /etc/systemd/system/velocity-report.service <<EOF`
   - `[Unit]`
   - `Description=Velocity.report radar monitor service`
   - `After=network.target`
   - `[Service]`
   - `User=velocity`
   - `Group=velocity`
   - `Type=simple`
   - `ExecStart=${INSTALL_DIR}/bin/velocity-report serve --db-path ${DATA_DIR}/sensor_data.db`
   - `WorkingDirectory=${DATA_DIR}`
   - `Restart=on-failure`
   - `RestartSec=5`
   - `StandardOutput=journal`
   - `StandardError=journal`
   - `SyslogIdentifier=velocity-report`
   - `[Install]`
   - `WantedBy=multi-user.target`
   - `EOF`
   - `systemctl daemon-reload`
   - `systemctl enable velocity-report.service`
   - `echo "Systemd service installed. Start with: sudo systemctl start velocity-report"`
   - `fi`
   - `echo "Installation complete!"`
   - `echo "Run 'velocity help' to get started."`

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

### Phase 5: testing & rollout (1 week)

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

## 6. Migration guide

### 6.1 For existing deployments

**Current Setup:**

/usr/local/bin/velocity-report # Old binary (cmd/radar)
/var/lib/velocity-report/sensor_data.db
**Migration Steps:**

1. **Backup database**
   - `sudo systemctl stop velocity-report`
   - `sudo cp /var/lib/velocity-report/sensor_data.db /var/lib/velocity-report/sensor_data.db.backup`

2. **Download new binary**
   - `VERSION=v1.0.0`
   - `VERSION_NUM="${VERSION#v}"`
   - `curl -LO https://github.com/banshee-data/velocity.report/releases/download/${VERSION}/velocity-report-${VERSION_NUM}-...`
   - `chmod +x velocity-report-${VERSION_NUM}-linux-arm64`

3. **Test new binary**
   - Test migrate command: `./velocity-report-${VERSION_NUM}-linux-arm64 migrate status --db-path /var/lib/velocity-report/sensor_data.db`
   - Test serve (don't background yet): `./velocity-report-${VERSION_NUM}-linux-arm64 serve --db-path /var/lib/velocity-report/sensor_data.db --disable-radar`

4. **Update systemd service**
   - `sudo vi /etc/systemd/system/velocity-report.service`
   - ExecStart=/usr/local/bin/velocity-report serve --db-path /var/lib/velocity-report/sensor_data.db: `sudo systemctl daemon-reload`

5. **Install new binary**
   - `sudo mv velocity-report-${VERSION_NUM}-linux-arm64 /usr/local/bin/velocity-report`
   - `sudo chown root:root /usr/local/bin/velocity-report`
   - `sudo chmod 755 /usr/local/bin/velocity-report`

6. **Restart service**
   - `sudo systemctl start velocity-report`
   - `sudo systemctl status velocity-report`

7. **Verify operation**
   - Check logs: `sudo journalctl -u velocity-report -f`
   - Test web UI: `curl http://localhost:8080/`
   - Test migrate command: `velocity-report migrate status --db-path /var/lib/velocity-report/sensor_data.db`
     **Rollback Plan:**

- If issues occur, restore old binary: `sudo systemctl stop velocity-report`
- `sudo cp /path/to/old/velocity-report /usr/local/bin/velocity-report`
- Restore old service file (remove "serve" from ExecStart): `sudo systemctl daemon-reload`
- `sudo systemctl start velocity-report`

### 6.2 For developers

**Current Workflow:**

- `make build-radar-local`
- `./velocity-report-local --disable-radar`
  **New Workflow:**

- `make build-radar-local`
- `./velocity-report-local serve --disable-radar`
- OR (serve is default): `./velocity-report-local --disable-radar`
  **Makefile Changes:**

- `build-radar-*` targets now build from `cmd/velocity-report/`
- `dev-go` target updated to use `serve` subcommand
- New `build-all` target builds all binaries
- New `install-system` target for local testing of installed layout

**Testing Changes:**

- Update integration tests to use subcommand syntax
- Add tests for new subcommands
- Verify backward compatibility (no args = serve)

### 6.3 For Python tools

**Current Workflow:**

- `cd tools/pdf-generator`
- `PYTHONPATH=. ../../.venv/bin/python -m pdf_generator.cli.main config.json`
  **New Workflow (Development):**

- Option 1: Via Go wrapper: `velocity-report pdf config.json`
- Option 2: Direct Python (still works): `cd tools/pdf-generator`
- `PYTHONPATH=. ../../.venv/bin/python -m pdf_generator.cli.main config.json`
- Option 3: Makefile (still works): `make pdf-report CONFIG=config.json`
  **New Workflow (Production):**

After installation via install.sh: `velocity-report pdf /path/to/config.json`
**No Breaking Changes:**

- Existing Makefile commands still work
- PYTHONPATH-based invocation still works
- Development workflow unchanged

---

## 7. Testing & validation

### 7.1 Unit tests

**New tests to add:**

1. **Subcommand Dispatcher** (`cmd/velocity-report/main_test.go`)

   | Field          | Type       | Description |
   | -------------- | ---------- | ----------- |
   | args           | `[]string` |             |
   | wantSubcommand | `string`   |             |

2. **Python Discovery** (`cmd/velocity-report/pdf_test.go`)
   - `TestFindPython`: verify venv discovery, system python fallback, and error handling

3. **Version Command** (`cmd/velocity-report/version_test.go`)
   - `TestVersionCommand`: verify version output format and git commit inclusion

### 7.2 Integration tests

**Add to existing test suite:**

1. **Subcommand Integration** (`integration_test.go`)
   - `TestServeSubcommand`: start server via "serve" subcommand, verify HTTP endpoints, stop
   - `TestMigrateSubcommand`: run migrate up, verify schema version, run migrate down
   - `TestPDFSubcommand`: create test config, run `velocity-report pdf`, verify PDF generated

2. **Backward Compatibility** (`compat_test.go`)
   - `TestBackwardCompatNoArgs`: run `velocity-report` with no args, verify it starts server (old behaviour)

### 7.3 End-to-End tests

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

### 7.4 Performance tests

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

## 8. Future enhancements

### 8.1 Docker distribution

**Benefits:**

- Simplified deployment
- Consistent environment
- Easy updates

**Architecture:**

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y \
 python3 \
 python3-pip \
 texlive-latex-base \
 && rm -rf /var/lib/apt/lists/\*

COPY velocity-report /usr/local/bin/
COPY python/ /usr/local/share/velocity-report/python/
RUN pip3 install -r /usr/local/share/velocity-report/python/requirements.txt

VOLUME /var/lib/velocity-report
EXPOSE 8080

CMD ["velocity-report", "serve", "--db-path", "/var/lib/velocity-report/sensor_data.db"]
**Usage:**

Run `docker run -d --name velocity-report --device=/dev/ttyUSB0 -v /var/lib/velocity-report:/var/lib/velocity-report -p 8080:8080 velocity-report:latest`

### 8.2 Raspberry Pi image

**Pre-built SD card image with:**

- Raspbian OS
- velocity.report pre-installed
- Systemd service enabled
- Web UI accessible on boot

**Distribution:**

- Download from GitHub releases
- Flash with Raspberry Pi Imager
- Boot and configure via web UI

### 8.3 Package managers

**APT/DEB Package:**

Run `sudo apt-get install velocity-report`
**Homebrew (macOS):**

- `brew tap banshee-data/tap`
- `brew install velocity-report`
  **Implementation:**

- Create `.deb` package in GitHub Actions
- Host on GitHub Releases or packagecloud.io
- Create Homebrew formula

### 8.4 Web-Based configuration

**Goal:** Replace JSON config files with web UI.

**Features:**

- Upload database via browser
- Configure report parameters
- Generate and download PDF
- No CLI required for basic use

**Architecture:**

velocity-report serve --enable-config-ui

# Access at http://localhost:8080/config

### 8.5 Plugin system

**Allow third-party extensions:**

- `velocity-report plugin install lidar-advanced-analytics`
- `velocity-report plugin list`
- `velocity-report lidar-advanced-analytics analyse --input data.csv`

---

## Appendix a: file layout comparison

### Current structure

velocity.report/
├── cmd/
│ ├── radar/ # Main server
│ ├── sweep/ # Sweep tool
│ ├── transit-backfill/ # Backfill utility
│ └── tools/
│ └── backfill_ring_elevations/
├── tools/
│ ├── pdf-generator/ # Python PDF generator
│ └── grid-heatmap/ # Python heatmaps
└── internal/
├── api/
├── db/
└── radar/

Binary outputs (after build):
├── velocity-report-{version}-linux-arm64 # Main server
└── app-sweep # Sweep tool

### ~~Proposed structure~~ (superseded — see below)

> The shape below assumed three Go cmd/ directories and a Python venv on disk. Both assumptions changed. Strikethrough preserved for context.

~~velocity.report/
├── cmd/
│ ├── velocity-report/ # Main binary (was cmd/radar)
│ │ ├── main.go # Subcommand dispatcher
│ │ ├── serve.go # Server logic
│ │ ├── pdf.go # PDF wrapper
│ │ ├── backfill.go # Backfill (moved from separate cmd)
│ │ └── version.go # Version info
│ ├── velocity-report-sweep/ # Sweep tool (renamed)
│ └── velocity-report-backfill-rings/ # Utility (renamed)
├── tools/
│ ├── pdf-generator/ # Python PDF generator
│ └── grid-heatmap/ # Python heatmaps
└── internal/
├── api/
├── db/
├── radar/
└── version/ # New: version management~~

~~Binary outputs (after build):
├── velocity-report-{version}-linux-arm64 # Main binary
├── velocity-report-sweep-linux-arm64 # Sweep binary
└── velocity-report-backfill-rings-linux-arm64 # Utility binary~~

**Current direction (2026-04):**

```
velocity.report/
├── cmd/
│ └── velocity/                           # Single busybox-style entry point
│   └── main.go                           # argv[0] dispatcher → serve | device | data | report | tune | version
├── internal/
│ ├── cmd/
│ │ ├── radar/   (was cmd/radar)
│ │ ├── device/  (public namespace; may reuse cmd/velocity-ctl internals initially)
│ │ ├── data/    (migrate + backfill)
│ │ ├── report/  (pdf)
│ │ ├── tune/    (sweep)
│ │ └── ...
│ ├── api/
│ ├── db/
│ ├── radar/
│ └── version/
└── tools/
  └── visualiser-macos/                   # macOS Swift tool — outside the Go binary
```

Single binary output: `velocity-{version}-linux-arm64`. No sibling Go artifacts. (Python tools deprecated.)

### ~~Installed system layout~~ Updated installed system layout

> Strikethrough preserves the flat-install design; the new layout uses a versioned directory under `/opt/velocity-report/` with symlinks for atomic upgrade and rollback. See [deploy-versioned-binary-plan.md](./deploy-versioned-binary-plan.md#on-disk-layout).

~~/usr/local/bin/
├── velocity-report # Main binary
├── velocity-report-sweep # Sweep binary (optional)
└── velocity-report-backfill-rings # Utility binary (optional)~~

~~/usr/local/share/velocity-report/
├── python/
│ ├── .venv/ # Python virtual environment
│ ├── pdf_generator/ # Python package
│ ├── grid_heatmap/ # Python scripts
│ └── requirements.txt # Python dependencies
└── docs/ # Documentation~~

```
/opt/velocity-report/
├── versions/
│ ├── 0.5.0/velocity
│ └── 0.5.1/velocity
├── current  -> versions/0.5.1
└── previous -> versions/0.5.0

/usr/local/bin/
├── velocity         -> /opt/velocity-report/current/velocity
└── velocity-report  -> /opt/velocity-report/current/velocity

/etc/profile.d/
└── velocity-aliases.sh   # velocity-status, velocity-log, velocity-start, velocity-stop, velocity-bounce
```

/var/lib/velocity-report/
└── sensor_data.db # SQLite database

/etc/systemd/system/
└── velocity-report.service # Systemd service

---

## Appendix b: command reference

### Current commands (before migration)

**Main server:**

- `velocity-report --db-path /path/to/db          # Start server`
- `velocity-report migrate up --db-path /path     # Database migration`
  **PDF generator:**

- `cd tools/pdf-generator`
- `PYTHONPATH=. ../../.venv/bin/python -m pdf_generator.cli.main config.json`
- OR: `make pdf-report CONFIG=config.json`
  **Sweep tool:**

Run `./app-sweep --mode multi --iterations 30`
**Utilities:**

- `go run cmd/transit-backfill/main.go --db sensor_data.db --start 2024-01-01 --end 2024-12-31`
- `go run cmd/tools/backfill_ring_elevations/main.go --db sensor_data.db`

### Promoted command surface (after migration)

#### 1. Binary CLI: promote this in the user guide

| Area             | Canonical commands                                                                                                                                      | Notes                                                                                                                |
| ---------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------- |
| Runtime          | `velocity serve`, `velocity serve --db-path ...`, `velocity version`, `velocity help`                                                                   | `velocity-report` remains as the server-compatibility alias: `velocity-report` with no args still starts the server. |
| Device lifecycle | `velocity device check`, `velocity device upgrade`, `velocity device rollback`, `velocity device backup`                                                | Owns installed-version lifecycle only. Do not use this namespace for `systemctl` or log-following.                   |
| Data maintenance | `velocity data migrate up`, `velocity data migrate status`, `velocity data backfill ring-elevations ...`, `velocity data backfill lidar-run-config ...` | Groups schema and repair tasks under one noun instead of spreading them across top-level verbs.                      |
| Reports          | `velocity report pdf --config report.json --db sensor_data.db`                                                                                          | Native Go pipeline, no Python wrapper in the deployed image.                                                         |
| Tuning           | `velocity tune sweep --mode multi --iterations 30`                                                                                                      | Replaces a sibling sweep binary with a namespaced tuning command.                                                    |

#### 2. Compatibility forms: keep during migration, do not promote

| Compatibility form            | Canonical target                                                     |
| ----------------------------- | -------------------------------------------------------------------- |
| `velocity-report`             | `velocity serve`                                                     |
| `velocity-report migrate ...` | `velocity data migrate ...`                                          |
| `velocity-report pdf ...`     | `velocity report pdf ...`                                            |
| `velocity-report sweep ...`   | `velocity tune sweep ...`                                            |
| `velocity-ctl ...`            | `velocity device ...` only if a temporary redirect bridge is shipped |

#### 3. Outside the binary: host lifecycle wrappers shipped in the image

| Wrapper           | Meaning                                          |
| ----------------- | ------------------------------------------------ |
| `velocity-status` | `systemctl status velocity-report.service`       |
| `velocity-start`  | `sudo systemctl start velocity-report.service`   |
| `velocity-stop`   | `sudo systemctl stop velocity-report.service`    |
| `velocity-bounce` | `sudo systemctl restart velocity-report.service` |
| `velocity-log`    | follow the live service log                      |

These are the right place for service status, service start and stop, restart, and logs. They align with the image as shipped today, and they keep host lifecycle distinct from the application CLI.

#### 4. Outside the binary: HTTP API families

| Family                     | Endpoints                                                                                    |
| -------------------------- | -------------------------------------------------------------------------------------------- |
| Identity and capability    | `GET /api/version`, `GET /api/config`, `GET /api/capabilities`                               |
| Traffic and database state | `GET /api/radar_stats`, `GET /api/timeline`, `GET /api/db_stats`                             |
| Site configuration         | `GET/POST /api/sites`, `GET/POST /api/site_config_periods`                                   |
| Reports                    | `POST /api/generate_report`, `GET/DELETE /api/reports/*`                                     |
| Charts                     | `GET/POST /api/charts/timeseries`, `GET /api/charts/histogram`, `GET /api/charts/comparison` |
| Control                    | `POST /command`, `GET/POST /api/transit_worker`                                              |

#### 5. What we should not promote

- Do not teach `ctl` as the public grouping.
- Do not teach service lifecycle under the binary when the image already ships clearer shell wrappers.
- Do not ship more top-level aliases than `velocity` and `velocity-report` unless a short migration bridge makes removal unsafe.

~~**Python tools (if installed separately):**~~

- ~~`pdf-generator config.json                       # Direct Python command`~~
- ~~`grid-heatmap --input data.csv --output plot.png # Heatmap visualization`~~

> Python tools are deprecated and not installed on deployed devices. See [CLAUDE.md](../../CLAUDE.md).

---

## Appendix c: release checklist

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

## Appendix d: breaking changes summary

### For end users

**✅ No Breaking Changes**

- Existing installations continue to work
- `velocity-report` (no args) still starts server
- All existing flags preserved

**✨ New Features**

- Subcommands for clarity: `velocity-report serve`
- Built-in PDF generation: `velocity-report pdf`
- Version command: `velocity-report version`

### For developers

**⚠️ Minor Breaking Changes**

- [cmd/radar/](../../cmd/radar) moved to `cmd/velocity-report/`
- Binary name includes version: `velocity-report-{version}-linux-arm64`
- Import paths unchanged (only cmd/ structure changed)

**✅ Backward Compatible**

- All Makefile targets work
- All tests pass
- Development workflow unchanged

### For advanced users

**✨ Improvements**

- `app-sweep` renamed to `velocity-report-sweep`
- Better tool discoverability
- Consistent naming convention

**✅ No Functionality Lost**

- All tools still available
- All features preserved
- All flags compatible
