# velocity.report Raspberry Pi Imager — Design Document

- **Status:** Active — Phase 1 complete (v0.5.1), Phase 2 targeting v0.6.0
- **Layers:** Cross-cutting (deployment infrastructure)
- **Author:** Ictinus (Product Architecture)
- **Related:** [deploy-distribution-packaging-plan.md](./deploy-distribution-packaging-plan.md) § 8.2, [frontend-consolidation.md](./web-frontend-consolidation-plan.md) (LiDAR toggle dependency)
- **Canonical:** [rpi-imager.md](../platform/operations/rpi-imager.md)

---

> **Executive summary and motivation:** see [rpi-imager.md](../platform/operations/rpi-imager.md).

---

## 1. Phased Delivery

This plan is delivered in two phases, reflecting the principle of shipping
working software before optimising.

### Phase 1 — Working Image (v0.5.1) ✅

**Goal:** Produce a flashable Raspberry Pi `.img` file that contains the
velocity.report stack with a minimal vendored TeX tree.

- Installs `texlive-xetex` at build time, extracts a minimal TeX Live tree
  (~143 MB) into `/opt/velocity-report/texlive/`, then purges the APT packages
  (~1 GB saved)
- Image size: ~350–500 MB compressed (.img.xz)
- Build pipeline: pi-gen + GitHub Actions CI
- Distribution: `.img.xz` GitHub Release asset + custom `os-list.json` for
  stock rpi-imager
- All software components bundled as they exist today — Go server, Python PDF
  generator, web frontend, systemd service, udev rules, serial configuration
- On-device update capability: `velocity-ctl upgrade` checks GitHub Releases,
  downloads the latest binary, and applies the upgrade with automatic backup
  and database migration — preserving user data across upgrades

**Acceptance:** A community member can download the `.img.xz`, flash it with
rpi-imager or `dd`, boot a Raspberry Pi 4, and have velocity.report running
with radar collection and PDF report generation functional. The user can
subsequently run `sudo velocity-ctl upgrade` to upgrade to a newer release
without losing their sensor data.

### Phase 2 — Format Pre-compilation (v0.6.0)

**Goal:** Reduce PDF compilation time by shipping pre-compiled `.fmt` format
files alongside the minimal TeX tree shipped in Phase 1.

- Pre-compile `xelatex -ini` format files for each report template
- Audit template dependencies to confirm which `.sty`, `.cls`, and
  font files are needed
- Validate PDF output parity between full and minimal TeX installations
- Measure before/after image sizes and compilation times

**Prerequisite:** Phase 1 shipped and validated on real hardware. The working
image provides the baseline against which Phase 2 size reductions are measured.

---

## 3. Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                   CI Pipeline (GitHub Actions)              │
│                                                             │
│  ┌───────────────┐    ┌──────────────┐    ┌──────────────┐  │
│  │ pi-gen /      │    │ Go cross-    │    │ Python wheel │  │
│  │ rpi-image-gen │◄───│ compile      │◄───│ + LaTeX deps │  │
│  │ (image build) │    │ (ARM64)      │    │ (bundled)    │  │
│  └──────┬────────┘    └──────────────┘    └──────────────┘  │
│         │                                                   │
│         ▼                                                   │
│  ┌──────────────┐    ┌───────────────────────────────────┐  │
│  │ .img.xz file │───►│ GitHub Release (asset upload)     │  │
│  │ (~2-4 GB)    │    │ + os-list JSON for rpi-imager     │  │
│  └──────────────┘    └───────────────┬───────────────────┘  │
│                                      │                      │
└──────────────────────────────────────┼──────────────────────┘
                                       │
            ┌──────────────────────────┼──────────────────┐
            │          End-User Machine                   │
            │                                             │
            │  ┌──────────────────────┐                   │
            │  │ rpi-imager (stock or │   SD Card         │
            │  │ forked) pointed at   │──────────────►    │
            │  │ custom repo JSON     │   velocity.report │
            │  └──────────────────────┘   image flashed   │
            │                                             │
            └─────────────────────────────────────────────┘
```

The solution has two independent concerns:

1. **Image Building** — a CI job that produces a flashable `.img` file
2. **Image Flashing** — a desktop application that writes that image to an SD card

These concerns are **decoupled by design**: the image is a standard Raspberry Pi
`.img` file that can be flashed by _any_ tool (rpi-imager, balenaEtcher, `dd`).

---

## 4. Tier 1 — Image Building Pipeline

### 4.1 Tool Comparison

| Criterion           | `pi-gen`                                     | `rpi-image-gen`                                    |
| ------------------- | -------------------------------------------- | -------------------------------------------------- |
| **Maturity**        | Established (years of use)                   | New (released March 2025)                          |
| **Build Model**     | Stage-based bash scripts, builds from source | Declarative profiles, uses pre-built .deb packages |
| **Build Time**      | 30–90 minutes                                | 5–15 minutes                                       |
| **Customisation**   | Flexible but fragile (shell scripts)         | Modular profiles and layers                        |
| **SBOM Generation** | Manual                                       | Automatic (built-in CVE reporting)                 |
| **CI Friendliness** | Docker-based build support                   | Designed for automation                            |
| **Documentation**   | Good                                         | Growing                                            |
| **Licence**         | BSD                                          | BSD                                                |
| **Target**          | General OS images                            | Production/industrial images                       |
| **Recommendation**  | ✅ Proven, safe default                      | ✅ Better long-term choice                         |

**Recommendation:** Start with **pi-gen** for the initial implementation (proven
CI patterns exist) and plan migration to **rpi-image-gen** once velocity.report
image requirements stabilise.

### 4.2 What the Image Must Contain

The image extends Raspberry Pi OS Lite (64-bit, Bookworm) with:

#### 4.2.1 System Packages (APT)

```
# Core runtime
python3 python3-pip python3-venv

# LaTeX (for PDF generation) — see § 4.6 for size reduction work stream
texlive-xetex texlive-fonts-recommended texlive-latex-extra
texlive-fonts-extra latexmk

# LiDAR support (included but disabled by default at runtime)
libpcap-dev            # packet capture for LiDAR UDP collection

# RS-232 HAT support
raspi-config           # for serial port enable/disable
python3-serial         # serial comms fallback
minicom                # debugging tool

# System utilities
sqlite3                # database inspection
jq                     # JSON processing
curl                   # health checks
```

#### 4.2.2 velocity.report Binaries

| Component                          | Source                                                                            | Install Path                                         |
| ---------------------------------- | --------------------------------------------------------------------------------- | ---------------------------------------------------- |
| `velocity-report` (Go server)      | Cross-compiled ARM64 binary **with pcap support** (`make build-radar-linux-pcap`) | `/usr/local/bin/velocity-report`                     |
| `velocity-ctl` (device management) | Cross-compiled ARM64 binary                                                       | `/usr/local/bin/velocity-ctl`                        |
| PDF generator (Python)             | Wheel + vendored deps in venv                                                     | `/opt/velocity-report/tools/pdf-generator/`          |
| Python venv                        | Pre-built `.venv/`                                                                | `/opt/velocity-report/.venv/`                        |
| Web frontend (static assets)       | Pre-built `web/build/`                                                            | Embedded in Go binary or `/opt/velocity-report/web/` |

The Go binary is built with `CGO_ENABLED=1` and `-tags pcap` so that LiDAR
packet capture is available at runtime. LiDAR is **disabled by default**; users
enable it through the web settings dashboard (see
[frontend-consolidation.md](./web-frontend-consolidation-plan.md) Phase 0 — Capabilities
API). The `--enable-lidar` flag is off unless explicitly toggled.

#### 4.2.2a Update Mechanism

The image ships with **no automatic updates** — this preserves the privacy-first
principle by making zero unsolicited network requests. Instead, users
**explicitly** run `velocity-ctl upgrade` when they choose to upgrade.

**Why in-place upgrade is mandatory for v0.5.1:** Users collect radar data in
SQLite over weeks or months. Re-flashing the SD card destroys that database.
The image must ship with a working upgrade path from day one.

##### Update workflow

```
sudo velocity-ctl upgrade              # check + download + apply latest release
sudo velocity-ctl upgrade --check      # print version comparison only
sudo velocity-ctl upgrade --binary /f  # apply a local binary (offline upgrade)
```

`velocity-ctl` is a purpose-built on-device management binary (no SSH, no
remote execution). The `upgrade` subcommand performs the full sequence:

1. **Check** — query GitHub Releases API (`api.github.com`) for the latest
   release of `banshee-data/velocity.report`; compare to installed version
2. **Download** — fetch the `velocity-report-linux-arm64` asset from the
   GitHub Release; compute SHA-256 of downloaded bytes and print it for
   operator verification (automatic checksum verification against a published
   `SHA256SUMS` file is deferred to v0.6.0)
3. **Backup** — create timestamped backup of current binary and database to
   `/var/lib/velocity-report/backups/`
4. **Stop** — `systemctl stop velocity-report.service`
5. **Install** — replace `/usr/local/bin/velocity-report` with the downloaded
   binary
6. **Migrate** — run `velocity-report migrate up` for any new database schema
   changes
7. **Start** — `systemctl start velocity-report.service`
8. **Verify** — confirm service is active and responding

If `--check` is passed, only step 1 runs and the result is printed. If
`--binary` is passed, steps 1–2 are skipped and the local file is used
(for offline or air-gapped upgrades).

##### Implementation scope for v0.5.1

New binary at `cmd/velocity-ctl/` (~500 lines). This replaces `cmd/deploy/`
entirely — no SSH surface, no remote execution, no install/fix/config
subcommands. Only the on-device capabilities that matter:

- `upgrade` — GitHub release check + download + backup + stop + install +
  migrate + start + verify. `--check` flag for version comparison only.
  `--binary` flag for offline upgrades.
- `rollback` — restore binary + database from most recent timestamped backup
- `backup` — create manual snapshot of binary + database
- `status` — thin wrapper around `systemctl status velocity-report`
- `version` — print installed velocity-ctl and velocity-report versions

The upgrade subcommand includes:

- GitHub release checking: HTTP GET to
  `https://api.github.com/repos/banshee-data/velocity.report/releases/latest`,
  parse JSON for `tag_name` and asset URLs
- Binary download: HTTP GET the `velocity-report-linux-arm64` asset URL,
  write to temp file, compute and print SHA-256 (automatic verification
  against a published `SHA256SUMS` asset is deferred to v0.6.0)
- `--binary` optional: if omitted, auto-download from GitHub

`cmd/deploy/` is deleted in v0.5.1. The SSH surface (`executor.go`,
`sshconfig.go`), remote install, fix, config, and health subcommands, and the
three legacy upgrade steps (`updateSourceCode`, `ensureLaTeX`,
`updatePythonDependencies`) are not carried forward.

##### Privacy guarantees

- **No unsolicited requests** — the tool only contacts GitHub when the user
  explicitly runs `velocity-ctl upgrade`
- **No telemetry** — no analytics, no tracking, no phone-home
- **No background processes** — no cron, no timer, no daemon
- **Public API only** — GitHub Releases API for public repos requires no
  authentication token
- **Verifiable** — SHA-256 checksum verification ensures binary integrity

##### Rollback

If an upgrade fails or causes problems:

```bash
sudo velocity-ctl rollback      # restore most recent backup
```

This restores the binary and database from the timestamped backup created
during the upgrade.

2. **Settings dashboard version banner** — the web UI settings page will
   display the currently installed version. A future "Check for updates"
   button is planned but not yet implemented.

#### 4.2.3 System Configuration

```
# Systemd service (auto-start on boot)
/etc/systemd/system/velocity-report.service

# Data directory (owned by velocity user)
/var/lib/velocity-report/

# Serial port configuration (for RS-232 HAT)
/boot/firmware/config.txt  →  enable_uart=1, dtoverlay=uart0
/boot/firmware/cmdline.txt →  remove console=serial0,115200

# Wi-Fi regulatory domain fallback (US)
# rpi-imager's first-boot flow lets users set Wi-Fi country.  If they
# skip it, the image defaults to the US regulatory domain so wireless
# is functional out of the box.
/etc/default/crda           →  REGDOMAIN=US
/etc/wpa_supplicant/wpa_supplicant.conf  →  country=US

# LiDAR network interface (disabled by default)
# Pre-configured static IP for the LiDAR subnet; the interface is
# brought up only when LiDAR is enabled via the settings dashboard.
/etc/network/interfaces.d/lidar  →  192.168.100.1/24 (manual)

# Dedicated service user
velocity:velocity (no login shell, owns /var/lib/velocity-report)
```

#### 4.2.4 RS-232 HAT Driver Configuration

The OmniPreSense OPS243 radar connects via USB-Serial or RS-232 HAT. The image
must pre-configure:

1. **UART overlay enabled** in `/boot/firmware/config.txt`:

   ```
   enable_uart=1
   dtoverlay=miniuart-bt    # move Bluetooth to mini-UART, free main UART
   ```

2. **Serial console disabled** (frees `/dev/ttyAMA0` for radar use):
   Remove `console=serial0,115200` from `/boot/firmware/cmdline.txt`

3. **USB-Serial permissions** via udev rule:

   ```
   # /etc/udev/rules.d/99-velocity-report.rules
   SUBSYSTEM=="tty", ATTRS{idVendor}=="10c4", ATTRS{idProduct}=="ea60", \
     MODE="0666", SYMLINK+="velocity-radar"
   ```

4. **User group membership**: `velocity` user added to `dialout` group

### 4.3 pi-gen Integration

```
pi-gen/
├── config                          # IMG_NAME=velocity-report
├── stage0/                         # Bootstrap (upstream, untouched)
├── stage1/                         # Minimal system (upstream, untouched)
├── stage2/                         # Lite system (upstream, untouched)
│   └── SKIP_IMAGES                 # Don't produce image at stage2
├── stage-velocity/                 # ★ Custom stage
│   ├── 00-install-packages/
│   │   ├── 00-packages             # APT packages (texlive, libpcap-dev, etc.)
│   │   ├── 01-run.sh              # Build minimal TeX tree, purge APT TeX packages
│   │   └── files/                  # Populated at build time by build-image.sh / CI
│   │       ├── build-minimal-texlive.sh
│   │       ├── dependency-manifest.txt
│   │       └── velocity-report.ini
│   ├── 01-velocity-binaries/
│   │   ├── 00-run.sh              # Copy pre-built Go binaries
│   │   └── files/
│   │       ├── velocity-report     # ARM64 server binary with pcap (from CI artifact)
│   │       └── velocity-ctl        # ARM64 management binary (from CI artifact)
│   ├── 02-velocity-python/
│   │   ├── 00-run.sh              # Set up venv, install PDF generator
│   │   └── files/
│   │       └── pdf-generator/      # Python source + wheel
│   ├── 03-velocity-config/
│   │   ├── 00-run.sh              # Create user, enable service, configure serial
│   │   └── files/
│   │       ├── velocity-report.service
│   │       ├── 99-velocity-report.rules  # udev rules
│   │       ├── config.txt.patch          # UART overlay
│   │       └── cmdline.txt.patch
│   ├── 04-velocity-lidar/
│   │   ├── 00-run.sh              # Configure LiDAR network (disabled by default)
│   │   └── files/
│   │       └── lidar-network.conf  # Static IP for 192.168.100.x subnet
│   ├── 05-velocity-wifi/
│   │   ├── 00-run.sh              # Set US Wi-Fi fallback regulatory domain
│   │   └── files/
│   │       └── wpa_supplicant.conf # country=US fallback
│   └── EXPORT_IMAGE                # Produce final image here
├── stage3/                         # SKIP (desktop — not needed)
│   └── SKIP
├── stage4/                         # SKIP (full desktop — not needed)
│   └── SKIP
└── stage5/                         # SKIP (extras — not needed)
    └── SKIP
```

### 4.4 CI Pipeline (GitHub Actions)

```yaml
# .github/workflows/build-image.yml (conceptual)
name: Build Raspberry Pi Image
on:
  release:
    types: [published]
  workflow_dispatch:

jobs:
  build-binaries:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Cross-compile Go binaries (ARM64)
        run: make build-radar-linux-pcap build-ctl-linux
      - name: Build Python wheel
        run: make build-python-wheel
      - uses: actions/upload-artifact@v4
        with:
          name: velocity-binaries
          path: |
            velocity-report-linux-arm64
            velocity-ctl-linux-arm64
            tools/pdf-generator/dist/*.whl

  build-image:
    needs: build-binaries
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/download-artifact@v4
        with: { name: velocity-binaries }
      - name: Build pi-gen image
        uses: usimd/pi-gen-action@v1
        with:
          image-name: velocity-report
          stage-list: stage0 stage1 stage2 stage-velocity
          # ... additional config
      - name: Compress image
        run: xz -9 deploy/velocity-report.img
      - name: Upload to release
        uses: softprops/action-gh-release@v1
        with:
          files: deploy/velocity-report.img.xz

  update-repo-json:
    needs: build-image
    runs-on: ubuntu-latest
    steps:
      - name: Update os-list JSON with new image URL and checksum
        run: |
          # Generate SHA256 checksum
          # Update os-list-velocity.json with new download URL
          # Commit and push to gh-pages or releases
```

### 4.5 Image Size Budget

> **Phase 1 (v0.5.1)** ships with a minimal vendored TeX tree (~143 MB),
> built from full `texlive-xetex` at image build time.

| Component                                 | Estimated Size  |
| ----------------------------------------- | --------------- |
| Raspberry Pi OS Lite (base)               | ~450 MB         |
| Minimal TeX tree (vendored, see § 4.6)    | ~143 MB         |
| Python 3 + venv + PDF deps                | ~200 MB         |
| Go binaries (velocity-report + ctl, pcap) | ~35 MB          |
| LiDAR support (libpcap + network config)  | ~5 MB           |
| Web frontend (static)                     | ~5 MB           |
| System config + udev rules                | < 1 MB          |
| **Total (uncompressed)**                  | **~840 MB**     |
| **Total (xz compressed)**                 | **~350–500 MB** |

TeX Live was the dominant size contributor; the minimal vendored tree (§ 4.6)
reduced it from ~800 MB to ~143 MB in Phase 1.

### 4.6 LaTeX Size Reduction Work Stream

The full `texlive-xetex` + fonts installation would add ~800 MB to the
uncompressed image. Phase 1 (v0.5.1) already ships a minimal vendored TeX
tree (~143 MB), saving ~1 GB. Phase 2 (v0.6.0) adds pre-compiled `.fmt`
format files for faster PDF generation.

#### 4.6.1 Options

| Option                                     | Approach                                                                                                                           | Estimated Savings                              |
| ------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------- |
| ~~**A: TinyTeX**~~                         | Install TinyTeX (a minimal, portable TeX Live distribution) and add only the LaTeX packages velocity.report actually uses          | ~600–700 MB saved                              |
| **B: Pre-compiled templates** ✅           | Ship pre-compiled `.fmt` files and only the fonts/packages referenced by our report templates; no general-purpose TeX installation | ~700–750 MB saved                              |
| ~~**C: Hybrid (TinyTeX + pre-compiled)**~~ | Install TinyTeX with pre-compiled format files for our templates; users can still install additional packages if needed            | ~650–700 MB saved                              |
| ~~**D: Docker sidecar**~~                  | Run LaTeX compilation inside a Docker container pulled on demand; no TeX in the base image at all                                  | ~800 MB saved (but adds Docker + runtime pull) |

#### 4.6.2 Evaluation Matrix

| Criterion                               | Weight | A: TinyTeX | B: Pre-compiled | C: Hybrid | D: Docker |
| --------------------------------------- | ------ | ---------- | --------------- | --------- | --------- |
| **Image size reduction**                | 5      | 4          | 5               | 4         | 5         |
| **User flexibility** (custom templates) | 3      | 5          | 1               | 4         | 3         |
| **Build complexity**                    | 4      | 3          | 4               | 3         | 2         |
| **Offline operation**                   | 5      | 5          | 5               | 5         | 1         |
| **Maintenance burden**                  | 4      | 3          | 4               | 3         | 2         |
| **PDF output quality**                  | 5      | 5          | 5               | 5         | 5         |
| **Pi 4 performance**                    | 3      | 4          | 5               | 4         | 2         |
| **Weighted Total**                      |        | **119**    | **122**         | **116**   | **85**    |

#### 4.6.3 Recommendation

**Option B (pre-compiled templates)** selected. ✅ Confirmed by D-08. Phase 1
shipped the minimal vendored TeX tree (steps 1–2 below). Phase 2 adds
pre-compiled `.fmt` format files (step 3) to eliminate per-run format-loading
overhead. No package manager, no `tlmgr`, no network fetches.

**Plan migration to Option C (hybrid)** if user feedback indicates demand for
custom LaTeX templates. TinyTeX can be installed on top of the pre-compiled base
without changing the image build pipeline.

#### 4.6.4 Implementation Steps

1. ✅ **Audit template dependencies** — `dependency-manifest.txt` lists every
   `.sty`, `.cls`, font, and binary the PDF generator uses
2. ✅ **Build a minimal TeX tree** — `scripts/build-minimal-texlive.sh` extracts
   only the required files from the full TeX Live distribution into
   `/opt/velocity-report/texlive/` (~143 MB). Pi-gen stage
   `00-install-packages/01-run.sh` runs this at image build time and purges
   the APT packages afterward
3. **Pre-compile format files** (Phase 2) — run `xelatex -ini` to produce
   `.fmt` files for each report template; eliminates per-run format-loading
   overhead
4. ✅ **Update pi-gen stage** — `00-install-packages` installs `texlive-xetex`
   APT packages, builds the minimal tree, and purges the APT packages
5. ✅ **Validate output** — PDF output validated between full TeX Live and
   minimal builds with no rendering regressions
6. ✅ **Measure** — minimal tree: ~143 MB vs full TeX Live: ~800 MB (~1 GB saved)

---

## 5. Tier 2 — Image Flashing (rpi-imager)

### 5.1 Approach Comparison

There are three approaches to getting our image into end users' hands:

| Criterion                    | A: Custom Repo JSON ✅                                                               | ~~B: Fork rpi-imager~~                                                             | ~~C: Custom Flashing Tool~~                                              |
| ---------------------------- | ------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------- | ------------------------------------------------------------------------ |
| **Concept**                  | Host a JSON catalogue; users launch stock rpi-imager with `--repo` flag or paste URL | ~~Fork rpi-imager, rebrand with velocity.report UI, hardcode our image catalogue~~ | ~~Build a new Electron/Tauri app that wraps `dd`/Win32DiskImager logic~~ |
| **User Experience**          | Users must install rpi-imager separately, then configure a custom repo URL           | Users download one branded app, our images appear by default                       | Users download our custom app, our images appear by default              |
| **Development Cost**         | Very low (JSON file + hosting)                                                       | Medium (C++/Qt build chain, cross-platform packaging)                              | High (new codebase, platform-specific disk I/O, security)                |
| **Maintenance Burden**       | Near zero (rpi-imager team maintains the flashing logic)                             | High (must track upstream Qt and rpi-imager changes)                               | Very high (own all platform-specific code)                               |
| **Branding**                 | Minimal (our images show in someone else's tool)                                     | Full (velocity.report look and feel)                                               | Full                                                                     |
| **Cross-Platform**           | ✅ rpi-imager already supports macOS, Windows, Linux                                 | ✅ Inherited from rpi-imager                                                       | ❓ Must implement and test per-platform                                  |
| **First-Boot Customisation** | ✅ rpi-imager supports Wi-Fi, SSH, locale setup                                      | ✅ Can extend with custom fields                                                   | ❓ Must implement from scratch                                           |
| **Licence**                  | N/A (no code changes)                                                                | Apache 2.0 (permissive, fork-friendly)                                             | N/A                                                                      |
| **Time to First Release**    | 1–2 days                                                                             | 4–8 weeks                                                                          | 12–20 weeks                                                              |
| **Ongoing Upstream Sync**    | None needed                                                                          | Regular merges required                                                            | N/A                                                                      |
| **Risk**                     | Low                                                                                  | Medium (upstream breaking changes, Qt version churn)                               | High (security bugs in raw disk writing)                                 |

### 5.2 Recommendation: Phased Approach

**Phase 1 (Immediate):** Use **Approach A — Custom Repository JSON**

This gets images into users' hands with minimal effort. Users install the stock
Raspberry Pi Imager (which many already have), and point it to our repository:

```bash
rpi-imager --repo https://velocity.report/images/os-list.json
```

Or they paste the URL into the Imager settings.

**Phase 2 (Future, if warranted):** Fork rpi-imager (**Approach B**)

Only pursue the fork if:

- User research shows the extra step of configuring a custom repo is a
  significant adoption barrier
- We need custom first-boot fields (e.g., radar port selection, site name)
- We want a fully branded download experience

> **Approach C is not recommended.** Writing raw disk images across three
> operating systems is a solved problem. Re-implementing it introduces security
> risk and diverts engineering effort from the core product.

---

## 6. Decision Matrix — Monorepo vs. Separate Repository

If/when we proceed with the rpi-imager fork (Phase 2), the code must live
somewhere. Here is the analysis:

### 6.1 Comparison Matrix

| Criterion                  | Monorepo (`velocity.report/imager/`)                                                                                    | Separate Repo (`velocity.report-imager`)                         |
| -------------------------- | ----------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------- |
| **Build Isolation**        | ❌ C++/Qt/CMake builds pollute the Go/Python/Node workspace; different toolchains, dependencies, and CI runners         | ✅ Clean separation; its own CI, dependencies, and build cache   |
| **CI Complexity**          | ❌ Must add Qt + CMake + platform SDKs to existing CI matrix; builds become much slower; macOS + Windows runners needed | ✅ Dedicated CI pipeline; no impact on existing Go/Python/Web CI |
| **Clone Size**             | ❌ rpi-imager source + Qt vendored deps add ~50-100 MB to every clone                                                   | ✅ Only cloned by contributors working on the imager             |
| **Language Diversity**     | ❌ Adds C++ and QML to a Go/Python/Svelte repo; confusing for contributors                                              | ✅ Contributors self-select by interest                          |
| **Release Cadence**        | ❌ Imager releases tied to velocity.report server releases; different cadences cause friction                           | ✅ Independent release tags and versioning                       |
| **Cross-Referencing**      | ✅ Easy to reference systemd service files, Go binary names, Python paths                                               | ⚠️ Must document conventions; risk of drift                      |
| **Atomic Changes**         | ✅ Can update image config + server code in one commit                                                                  | ❌ Changes spanning both repos require coordination              |
| **Discoverability**        | ✅ All project code in one place                                                                                        | ⚠️ Users must find two repositories                              |
| **Contributor Experience** | ❌ C++ contributors need Go/Python toolchains installed (or carefully isolated)                                         | ✅ Clean setup: clone → install Qt → build                       |
| **Licence Clarity**        | ⚠️ Must clearly delineate Apache 2.0 (imager) from the rest of the repo licence                                         | ✅ Separate LICENCE file, no ambiguity                           |
| **Upstream Sync**          | ❌ Git subtree/submodule merges are messy in a monorepo                                                                 | ✅ Standard fork workflow; `git remote add upstream` + merge     |
| **GitHub Features**        | ❌ Issues, PRs, and releases for imager mixed with server issues                                                        | ✅ Dedicated issues, PRs, releases, and project board            |
| **Makefile Integration**   | ⚠️ Must add complex CMake targets to existing Makefile                                                                  | ✅ Own Makefile/CMakeLists.txt                                   |

### 6.2 Scoring Summary

| Factor                 | Weight | Monorepo | Separate Repo |
| ---------------------- | ------ | -------- | ------------- |
| Build isolation        | 5      | 1        | 5             |
| CI complexity          | 5      | 1        | 5             |
| Upstream sync ease     | 4      | 1        | 5             |
| Contributor experience | 4      | 2        | 5             |
| Release independence   | 4      | 2        | 5             |
| Clone size impact      | 3      | 2        | 5             |
| Licence clarity        | 3      | 3        | 5             |
| Cross-referencing      | 3      | 5        | 3             |
| Atomic changes         | 2      | 5        | 2             |
| Discoverability        | 2      | 4        | 3             |
| **Weighted Total**     |        | **72**   | **163**       |

### 6.3 Recommendation

**Use a separate repository** (`banshee-data/velocity.report-imager`).

The rpi-imager fork is a fundamentally different technology stack (C++/Qt/CMake)
with a different release cadence, contributor profile, and CI requirement.
Placing it in the monorepo would:

- Slow down CI for every Go/Python/Web contributor
- Complicate the already-large Makefile (101 targets)
- Create confusion about which issues and PRs relate to which component
- Make upstream sync with `raspberrypi/rpi-imager` unnecessarily difficult

The only advantages of the monorepo (atomic changes, cross-referencing) are
easily mitigated by:

- Documenting path conventions in both repos
- Using GitHub release tags to coordinate versions
- Referencing `image/stage-velocity/03-velocity-config/files/velocity-report.service`
  as the canonical service definition

### 6.4 What Stays in the Monorepo

Even with the imager in a separate repository, the following **must** remain in
the `velocity.report` monorepo:

| Asset                   | Location                                                                | Reason                                                             |
| ----------------------- | ----------------------------------------------------------------------- | ------------------------------------------------------------------ |
| pi-gen stage scripts    | `image/stage-velocity/`                                                 | Defines what goes in the image; tightly coupled to server releases |
| OS-list repository JSON | `image/os-list-velocity.json`                                           | Catalogue of available images; updated by CI on release            |
| Image CI workflow       | `.github/workflows/build-image.yml`                                     | Triggered by monorepo releases                                     |
| systemd service file    | `image/stage-velocity/03-velocity-config/files/velocity-report.service` | Canonical source                                                   |
| udev rules              | `image/stage-velocity/03-velocity-config/files/`                        | Device permission rules                                            |
| Management binary       | `cmd/velocity-ctl/`                                                     | `velocity-ctl upgrade`, `rollback`, `backup`, `status`, `version`  |
| LiDAR network config    | `image/stage-velocity/04-velocity-lidar/files/lidar-network.conf`       | Static IP for 192.168.100.x subnet (disabled by default)           |

---

## 7. Pitfalls and Risks

### 7.1 Image Building Pitfalls

| Risk                                                                                     | Severity | Mitigation                                                                                                                                                                             |
| ---------------------------------------------------------------------------------------- | -------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **TeX Live size bloat** — full installation is 4+ GB                                     | High     | Dedicated reduction work stream (§ 4.6): ship pre-compiled templates + minimal TeX tree; target < 200 MB                                                                               |
| **pi-gen build flakiness** — network-dependent APT fetches can fail                      | Medium   | Pin package versions; use local APT mirror in CI; retry logic                                                                                                                          |
| **ARM64 QEMU emulation speed** — pi-gen builds on x86 CI runners use QEMU for ARM chroot | Medium   | Use native ARM64 runners (GitHub now offers them) or cross-compile everything outside the chroot                                                                                       |
| **Python venv portability** — venvs built on x86 may not work on ARM64                   | High     | Build the venv inside the ARM64 chroot (pi-gen stage script) or use wheels with `--platform manylinux_2_28_aarch64`                                                                    |
| **Image size exceeding GitHub release limits** — GitHub has a 2 GB per-asset limit       | Medium   | Use xz compression (typical 3:1 ratio); consider hosting on a dedicated CDN for larger images                                                                                          |
| **Serial port conflicts** — Bluetooth uses the same UART on Pi 4                         | Medium   | Overlay `miniuart-bt` moves Bluetooth to mini-UART; document for users with Bluetooth peripherals                                                                                      |
| **SD card wear** — SQLite WAL mode on SD cards can cause premature failure               | Low      | Document recommended SD card brands; consider moving WAL to tmpfs with periodic sync                                                                                                   |
| **LiDAR pcap binary size** — building with pcap adds ~5 MB to the Go binary              | Low      | Acceptable trade-off; LiDAR hardware support is included but disabled by default; no runtime cost when off                                                                             |
| **First-boot configuration** — users need to set Wi-Fi before the device has a screen    | Medium   | Leverage rpi-imager's built-in Wi-Fi/SSH customisation; image defaults to US regulatory domain (`country=US`) so wireless is functional even if the user skips Wi-Fi country selection |

### 7.2 rpi-imager Fork Pitfalls (Phase 2)

| Risk                                                                                                                       | Severity | Mitigation                                                                             |
| -------------------------------------------------------------------------------------------------------------------------- | -------- | -------------------------------------------------------------------------------------- |
| **Qt version churn** — rpi-imager requires Qt 6.7+; major version upgrades break APIs                                      | High     | Pin Qt version; sync with upstream only on stable releases                             |
| **Cross-platform packaging** — building .dmg (macOS), .exe/.msi (Windows), .AppImage (Linux) requires platform-specific CI | High     | Use upstream's existing packaging scripts; GitHub Actions matrix builds                |
| **Code signing** — macOS and Windows require signed binaries to avoid security warnings                                    | High     | Obtain Apple Developer and Windows Authenticode certificates; budget ~$200/year        |
| **Upstream divergence** — the more we customise, the harder merges become                                                  | Medium   | Minimise changes: branding + default repo URL only; avoid touching core flashing logic |
| **Dependency licensing** — Qt is LGPL; must comply with linking requirements                                               | Medium   | Dynamic linking (already the upstream approach); include LGPL notice                   |
| **User confusion** — two "imager" apps on the system                                                                       | Low      | Clear naming: "velocity.report Imager" vs "Raspberry Pi Imager"                        |

### 7.3 General Risks

| Risk                                                                       | Severity | Mitigation                                                                           |
| -------------------------------------------------------------------------- | -------- | ------------------------------------------------------------------------------------ |
| **Scope creep** — image building project absorbs all engineering time      | High     | Strict phased approach; Phase 1 (JSON repo) delivers value in days, not weeks        |
| **Security** — pre-built images could be tampered with                     | High     | SHA-256 checksums in os-list JSON; GPG-signed releases; reproducible builds          |
| **Support burden** — "it didn't boot" becomes the #1 issue                 | Medium   | Comprehensive first-boot diagnostics; LED status codes; web-based setup wizard       |
| **Raspberry Pi OS upgrades** — new Debian releases break our image scripts | Medium   | Pin to Bookworm; test quarterly against new releases; document supported OS versions |

---

## 8. Deploy Tool Replacement — `velocity-ctl`

`cmd/deploy/` (the `velocity-deploy` binary) is **deleted in v0.5.1** and
replaced by `cmd/velocity-ctl/` (the `velocity-ctl` binary). This is a clean
break, not a gradual deprecation — there are no existing image users to
migrate, and shipping both binaries creates a limbo state where two tools
with overlapping names do different things.

### 8.1 What Changes

| Before (deleted)                           | After (new)                              |
| ------------------------------------------ | ---------------------------------------- |
| `velocity-deploy` (3,678 LOC, 10 Go files) | `velocity-ctl` (~500 LOC, purpose-built) |
| `velocity-update` (21-line bash wrapper)   | _(deleted — no wrapper needed)_          |
| 8 subcommands, SSH surface, legacy steps   | 5 subcommands, local-only, no SSH        |

### 8.2 Subcommand Map

```
velocity-ctl                           # On-device management (root)
  ├── upgrade    (from deploy)         # Check + download + apply release
  ├── rollback   (from deploy)         # Restore previous version from backup
  ├── backup     (from deploy)         # Manual snapshot of binary + database
  ├── status     (new, thin)           # systemctl status wrapper
  └── version    (new)                 # Show installed versions
```

**Not carried forward** from `cmd/deploy/`: `install`, `fix`, `config`,
`health`, SSH execution (`executor.go`, `sshconfig.go`), legacy upgrade steps
(`updateSourceCode`, `ensureLaTeX`, `updatePythonDependencies`).

### 8.3 Why `velocity-ctl` (not `velocity-deploy`)

- **No name collision** — `velocity-deploy` implied pushing code TO somewhere
  over SSH. On-device, the tool pulls an update DOWN to itself. The name was
  actively misleading.
- **Unix convention** — follows `systemctl`, `apachectl`, `rabbitmqctl`.
- **Clean privilege domain** — `velocity-ctl` always runs as root on-device.
  `velocity-report` runs as the `velocity` service user. No mixed privilege
  model.
- **Smaller binary** — ~500 lines vs ~3,700. No dead code ships on the image.

### 8.4 Image Binaries (v0.5.1)

Two Go binaries, no wrapper scripts:

| Binary            | Install Path                     | Runs as  | Purpose                     |
| ----------------- | -------------------------------- | -------- | --------------------------- |
| `velocity-report` | `/usr/local/bin/velocity-report` | velocity | Server (radar, API, web UI) |
| `velocity-ctl`    | `/usr/local/bin/velocity-ctl`    | root     | Upgrade, rollback, backup   |

### 8.5 Deleted Artefacts

The following are removed from the repository in v0.5.1:

- `cmd/deploy/` — entire directory (10 source files, 10 test files, README)
- `image/stage-velocity/01-velocity-binaries/files/velocity-update` — bash wrapper
- Makefile targets: `build-deploy`, `build-deploy-linux`, `deploy-install`,
  `deploy-upgrade`, `deploy-status`, `deploy-health`, `deploy-install-latex`,
  `deploy-install-latex-minimal`, `deploy-update-deps`, `setup-radar`
- `scripts/setup-radar-host.sh`

### 8.6 Future Consolidation

The [distribution-packaging plan](./deploy-distribution-packaging-plan.md)
proposes absorbing `velocity-ctl` subcommands into `velocity-report` as
subcommands (e.g. `velocity-report upgrade`). That remains a future option
but is **not required** — `velocity-ctl` is a stable, permanent name that
can remain indefinitely. The consolidation only makes sense if eliminating
one binary from the image is worth the mixed privilege model (root
subcommands in a non-root binary).

---

## 9. Implementation Phases

### Phase 0 — Prerequisites (1–2 days)

- [ ] Verify `make build-radar-linux-pcap` produces a working ARM64 binary with LiDAR support
- [ ] Verify Python PDF generator works on ARM64 Raspberry Pi OS
- [ ] Document the exact list of APT packages needed (test on clean Pi OS Lite)
- [ ] Test RS-232 HAT configuration manually on a Raspberry Pi 4
- [ ] Verify LiDAR packet capture works on Pi 4 with pcap-enabled binary (disabled by default, enable with `--enable-lidar`)

### Phase 1 — Image Building with pi-gen (1–2 weeks) ✅ Complete

- [x] Create `image/` directory in monorepo
- [x] Write pi-gen `config` file and `stage-velocity/` scripts
- [x] Include `velocity-ctl` binary and version metadata in image
- [x] Configure US Wi-Fi regulatory domain fallback
- [x] Include LiDAR support (libpcap, network config) disabled by default
- [x] Create GitHub Actions workflow for image building
- [x] Create `image/os-list-velocity.json` with schema-compliant entries
- [ ] Test image on physical Raspberry Pi 4 hardware
- [ ] Produce first `.img.xz` release asset

Note: Phase 1 installs `texlive-xetex` at build time, extracts a minimal
vendored TeX tree (~143 MB), then purges the APT packages from the final
image. Further size reduction via pre-compiled `.fmt` files (§ 4.6) is
deferred to Phase 2 (v0.6.0).

### Phase 2 — Custom Repository JSON (2–3 days)

- [x] Create `image/os-list-velocity.json` with schema-compliant entries
- [ ] Host JSON on GitHub Pages or alongside releases
- [ ] Write end-user documentation: "How to flash velocity.report"
- [ ] Add `--repo` instructions to main README
- [ ] Test with stock rpi-imager on macOS, Windows, Linux

### Phase 3 — First-Boot Experience (1 week)

- [ ] Create a first-boot script that validates radar connectivity
- [ ] Add a web-based setup wizard (accessible at `http://velocity.local:8080/setup`)
- [ ] LED status indicator for boot progress (optional, GPIO-dependent)
- [ ] Smoke-test the full flow: flash → boot → radar detected → web UI accessible

### Phase 4 — rpi-imager Fork (4–8 weeks, only if warranted)

- [ ] Fork `raspberrypi/rpi-imager` to `banshee-data/velocity.report-imager`
- [ ] Rebrand UI: velocity.report logo, colour scheme, application name
- [ ] Set default `--repo` to velocity.report's os-list JSON
- [ ] Add custom first-boot fields (site name, radar port override)
- [ ] Set up cross-platform CI (macOS .dmg, Windows .exe, Linux .AppImage)
- [ ] Obtain code-signing certificates (Apple Developer + Windows Authenticode)
- [ ] Publish v1.0.0 release with binaries for all three platforms
- [ ] Establish upstream sync schedule (quarterly merge from `raspberrypi/rpi-imager`)

---

## 10. Repository Layout (Monorepo Additions)

```
velocity.report/
├── image/                              # ★ New directory
│   ├── README.md                       # Image building documentation
│   ├── os-list-velocity.json           # rpi-imager custom repository catalogue
│   ├── config                          # pi-gen configuration
│   ├── stage-velocity/                 # pi-gen custom stage
│   │   ├── 00-packages                 # APT package list (incl. libpcap-dev)
│   │   ├── 01-velocity-binaries/
│   │   │   ├── 00-run.sh
│   │   │   └── files/
│   │   │       └── velocity-update     # Redirect stub (prints "use velocity-ctl upgrade")
│   │   ├── 02-velocity-python/
│   │   │   └── 00-run.sh
│   │   ├── 03-velocity-config/
│   │   │   ├── 00-run.sh
│   │   │   └── files/
│   │   │       ├── velocity-report.service  # systemd unit file
│   │   │       ├── 99-velocity-report.rules
│   │   │       ├── config.txt.patch
│   │   │       └── cmdline.txt.patch
│   │   ├── 04-velocity-lidar/
│   │   │   ├── 00-run.sh              # LiDAR network config (disabled)
│   │   │   └── files/
│   │   │       └── lidar-network.conf
│   │   ├── 05-velocity-wifi/
│   │   │   ├── 00-run.sh              # US Wi-Fi fallback
│   │   │   └── files/
│   │   │       └── wpa_supplicant.conf
│   │   └── EXPORT_IMAGE
│   └── scripts/
│       └── build-image.sh              # Local image build helper
├── .github/workflows/
│   └── build-image.yml                 # ★ New workflow
└── ... (existing structure unchanged)
```

---

## 11. os-list JSON Schema (Phase 2)

A single image entry — the full stack with radar, LiDAR (disabled), PDF
generation, and web dashboard:

```json
{
  "imager": {
    "latest_version": "1.0.0",
    "url": "https://github.com/banshee-data/velocity.report/releases"
  },
  "os_list": [
    {
      "name": "velocity.report",
      "description": "Privacy-first traffic monitoring — full stack with radar, LiDAR (disabled by default), PDF reporting, and web dashboard. Based on Raspberry Pi OS Lite (Bookworm, 64-bit).",
      "url": "https://github.com/banshee-data/velocity.report/releases/download/v1.0.0/velocity-report-v1.0.0.img.xz",
      "extract_size": 1073741824,
      "extract_sha256": "<sha256-of-uncompressed-img>",
      "image_download_size": 419430400,
      "release_date": "2026-03-01",
      "icon": "https://velocity.report/images/icon-256.png",
      "init_format": "systemd",
      "devices": ["pi4-64bit", "pi400-64bit", "pi5-64bit"],
      "url_info": "https://velocity.report/docs/guides/setup"
    }
  ]
}
```

---

## 12. Security Considerations

### 11.1 Image Integrity

- Every release image **must** include SHA-256 checksums in both the GitHub
  release notes and the os-list JSON `extract_sha256` field
- Consider GPG-signing release assets with a project key
- CI builds should be deterministic: same inputs → same image hash

### 11.2 Supply Chain

- Pin all APT package versions in pi-gen scripts
- Use GitHub Actions' built-in artifact attestation
- Generate SBOM for each image release (rpi-image-gen does this automatically)

### 11.3 Runtime Security

- The `velocity` user runs with minimal privileges (no sudo)
- The systemd service uses `DynamicUser=` or a dedicated system user
- Serial port access is granted via udev rules, not blanket permissions
- No default passwords in the image; rpi-imager's first-boot customisation
  handles user creation

### 11.4 Privacy

- The image **must not** contain:
  - Telemetry or phone-home capabilities
  - Automatic update mechanisms (updates are user-initiated only)
  - Pre-configured cloud endpoints
  - SSH keys or credentials
  - Any personally identifiable information
- The os-list JSON is fetched by rpi-imager, but this only reveals that someone
  is _looking at_ the velocity.report catalogue, not using it
- The "Check for updates" functionality in the settings dashboard is planned
  but not yet implemented

---

## 13. References

- [raspberrypi/rpi-imager](https://github.com/raspberrypi/rpi-imager) — Apache 2.0 licence, C++/Qt6/QML/CMake
- [RPi-Distro/pi-gen](https://github.com/RPi-Distro/pi-gen) — Stage-based image builder, BSD licence
- [raspberrypi/rpi-image-gen](https://github.com/raspberrypi/rpi-image-gen) — New declarative image builder (2025), BSD licence
- [rpi-imager custom repository JSON schema](https://github.com/raspberrypi/rpi-imager/blob/main/doc/json-schema/os-list-schema.json)
- [How to add your own images to Imager](https://www.raspberrypi.com/news/how-to-add-your-own-images-to-imager/)
- [velocity.report distribution-packaging-plan.md](./deploy-distribution-packaging-plan.md) § 8.2
- [velocity.report ARCHITECTURE.md](../../ARCHITECTURE.md)
- [velocity-report.service](../../image/stage-velocity/03-velocity-config/files/velocity-report.service) — Canonical systemd unit
- [velocity.report frontend-consolidation.md](./web-frontend-consolidation-plan.md) — LiDAR toggle UI dependency

---

## 14. Summary of Recommendations

| Decision                              | Recommendation                                                                           | Rationale                                                                                                         |
| ------------------------------------- | ---------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------- |
| **Image building tool**               | Start with pi-gen, plan migration to rpi-image-gen                                       | pi-gen is proven; rpi-image-gen is better long-term but newer                                                     |
| **Image flashing (Phase 1)**          | Custom repository JSON for stock rpi-imager                                              | Zero development cost; immediate value                                                                            |
| **Image flashing (Phase 2)**          | Fork rpi-imager into separate repo (only if needed)                                      | Full branding + custom fields; only justified by user research                                                    |
| **Repository for imager fork**        | **Separate repo** (`banshee-data/velocity.report-imager`)                                | Different tech stack (C++/Qt), release cadence, and contributor profile                                           |
| **Image build scripts**               | **Monorepo** (`velocity.report/image/`)                                                  | Tightly coupled to server releases; same CI pipeline                                                              |
| **Image variants**                    | **Single image** with full stack                                                         | LiDAR disabled by default; LaTeX footprint reduced via § 4.6                                                      |
| **LaTeX size reduction**              | Pre-compiled templates (Option B), migrate to hybrid if needed                           | Greatest savings (~750 MB) with simplest runtime                                                                  |
| **LiDAR support**                     | Included (pcap build) but **disabled by default**                                        | Users enable via settings dashboard; depends on [frontend-consolidation.md](./web-frontend-consolidation-plan.md) |
| **Auto-update**                       | **None** — user-initiated `sudo velocity-ctl upgrade`; dashboard version display planned | Preserves privacy-first principle; zero unsolicited network requests                                              |
| **Wi-Fi fallback**                    | US regulatory domain (`country=US`)                                                      | Ensures wireless works out of the box if user skips country selection                                             |
| **Custom flashing tool (Approach C)** | **Do not pursue**                                                                        | Re-implementing disk I/O is high-risk and low-value                                                               |
