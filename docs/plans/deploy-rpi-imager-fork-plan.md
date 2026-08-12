# velocity.report Raspberry Pi imager: design document

- **Status:** Active; Phase 1 complete (v0.5.1), Phase 2 cancelled (replaced by Typst cutover)
- **Layers:** Cross-cutting (deployment infrastructure)
- **Related:** [deploy-distribution-packaging-plan.md](./deploy-distribution-packaging-plan.md) § 8.2, [frontend-consolidation.md](./web-frontend-consolidation-plan.md) (LiDAR toggle dependency), [deploy-single-binary-image-consolidation-plan.md](./deploy-single-binary-image-consolidation-plan.md) (image surface consolidation)
- **Canonical:** [rpi-imager.md](../platform/operations/rpi-imager.md)

---

> **Executive summary and motivation:** see [rpi-imager.md](../platform/operations/rpi-imager.md).

> **Current image build note (v0.5.1):** the early Phase 1 TeX/`velocity-ctl`
> inventory has been superseded by the consolidation work. The current image
> stages one static ARM64 `velocity` binary built through Docker + zig/musl +
> vendored `libpcap.a`; reports use embedded Typst; `velocity device ...`
> replaces `velocity-ctl`; the repo-level package manifest is down to
> `raspi-config`; and the remaining package-stage debt is the masked Tailscale
> apt install in `07-velocity-tailscale`.

---

## 1. Phased delivery

This plan is delivered in two phases, reflecting the principle of shipping
working software before optimising.

### Phase 1: working image (v0.5.1) ✅

**Goal:** Produce a flashable Raspberry Pi `.img` file that contains the
velocity.report stack as one static multi-call binary plus image-owned system
configuration.

- Stages a static ARM64 `velocity` binary built by Docker + zig/musl and
  vendored `libpcap.a`
- Uses the Go + embedded Typst report pipeline; no TeX Live tree ships in the
  image
- Image size target: ~150–300 MB compressed (.img.xz)
- Build pipeline: static binary staging + pi-gen + GitHub Actions CI
- Distribution: `.img.xz` GitHub Release asset + custom `os-list.json` for
  stock rpi-imager
- All software components bundled as they exist today: Go server, Go + Typst
  PDF generator, web frontend, systemd service, udev rules, serial
  configuration
- On-device update capability: `velocity device upgrade` checks GitHub Releases,
  downloads the latest binary, and applies the upgrade with automatic backup
  and database migration: preserving user data across upgrades

**Acceptance:** A community member can download the `.img.xz`, flash it with
rpi-imager or `dd`, boot a Raspberry Pi 4, and have velocity.report running
with radar collection and PDF report generation functional. The user can
subsequently run `sudo velocity device upgrade` to upgrade to a newer release
without losing their sensor data.

### Phase 2: format pre-compilation (cancelled — replaced by Typst cutover in v0.5.1)

> **Cancelled.** [deploy-single-binary-image-consolidation-plan.md](./deploy-single-binary-image-consolidation-plan.md) § Work unit C replaces xelatex with Typst entirely. There is no `.fmt` artifact in Typst's model and the underlying problem (cold-start LaTeX compile cost) disappears with the pipeline change. The section is preserved below for historical context only.

~~**Goal:** Reduce PDF compilation time by shipping pre-compiled `.fmt` format
files alongside the minimal TeX tree shipped in Phase 1.~~

- ~~Pre-compile `xelatex -ini` format files for each report template~~
- ~~Audit template dependencies to confirm which `.sty`, `.cls`, and
  font files are needed~~
- ~~Validate PDF output parity between full and minimal TeX installations~~
- ~~Measure before/after image sizes and compilation times~~

~~**Prerequisite:** Phase 1 shipped and validated on real hardware. The working
image provides the baseline against which Phase 2 size reductions are measured.~~

---

## 3. Architecture overview

```
┌─────────────────────────────────────────────────────────────┐
│                   CI Pipeline (GitHub Actions)              │
│                                                             │
│  ┌───────────────┐    ┌──────────────┐    ┌──────────────┐  │
│  │ pi-gen /      │    │ static ARM64 │    │ vendored     │  │
│  │ rpi-image-gen │◄───│ Go binary    │◄───│ libpcap.a    │  │
│  │ (image build) │    │ staging      │    │ + Typst      │  │
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

1. **Image Building**: a CI job that produces a flashable `.img` file
2. **Image Flashing**: a desktop application that writes that image to an SD card

These concerns are **decoupled by design**: the image is a standard Raspberry Pi
`.img` file that can be flashed by _any_ tool (rpi-imager, balenaEtcher, `dd`).

---

## 4. Tier 1: image building pipeline

### 4.1 Tool comparison

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

### 4.2 What the image must contain

The image extends Raspberry Pi OS Lite (64-bit, Bookworm) with:

#### 4.2.1 System packages (APT)

```
# RS-232 HAT support
raspi-config           # for serial port enable/disable
```

`07-velocity-tailscale` still installs Tailscale from the upstream apt
repository and masks `tailscaled` until the operator opts in. That stage also
installs `curl` transiently for the keyring fetch. Removing that stage is the
remaining in-binary Tailscale installer work tracked in
[deploy-single-binary-image-consolidation-plan.md](./deploy-single-binary-image-consolidation-plan.md).

#### 4.2.2 velocity.report binaries

| Component                             | Source                                                             | Install Path                         |
| ------------------------------------- | ------------------------------------------------------------------ | ------------------------------------ |
| `velocity` multi-call binary          | Static ARM64 Docker build with zig/musl and vendored `libpcap.a`   | `/opt/velocity-report/versions/<v>/` |
| `velocity` canonical CLI              | Symlink to active version                                          | `/usr/local/bin/velocity`            |
| `velocity-report` compatibility alias | Symlink to active version; server-oriented `argv[0]` compatibility | `/usr/local/bin/velocity-report`     |
| PDF generator                         | Go + embedded Typst                                                | Embedded in Go binary                |
| Web frontend and offline docs         | Pre-built static assets                                            | Embedded in Go binary                |

The Go binary is built with `CGO_ENABLED=1` and `-tags pcap`, but the image
binary links `libpcap.a` statically from the vendored submodule rather than
depending on `libpcap0.8` or `libpcap-dev` in the image. LiDAR is **disabled by
default**; users enable it through the web settings dashboard (see
[frontend-consolidation.md](./web-frontend-consolidation-plan.md) Phase 0: Capabilities
API). The `--enable-lidar` flag is off unless explicitly toggled.

#### 4.2.2a Update mechanism

The image ships with **no automatic updates**: this preserves the privacy-first
principle by making zero unsolicited network requests. Instead, users
**explicitly** run `velocity device upgrade` when they choose to upgrade.

**Why in-place upgrade is mandatory for v0.5.1:** Users collect radar data in
SQLite over weeks or months. Re-flashing the SD card destroys that database.
The image must ship with a working upgrade path from day one.

##### Update workflow

```
sudo velocity device upgrade              # check + download + apply latest release
sudo velocity device upgrade --check      # print version comparison only
sudo velocity device upgrade --binary /f  # apply a local binary (offline upgrade)
```

`velocity device` is the on-device management namespace in the multi-call
binary (no SSH, no remote execution). The `upgrade` subcommand performs the full
sequence:

1. **Check**: query GitHub Releases API (`api.github.com`) for the latest
   release of `banshee-data/velocity.report`; compare to installed version
2. **Download**: fetch the `velocity-report-{version}-linux-arm64` asset from the
   GitHub Release; compute SHA-256 of downloaded bytes and print it for
   operator verification (automatic checksum verification against a published
   release metadata
3. **Backup**: create timestamped backup of current binary and database to
   `/var/lib/velocity-report/backups/`
4. **Stop**: `systemctl stop velocity-report.service`
5. **Install**: add `/opt/velocity-report/versions/<v>/velocity` and atomically
   move the `current` symlink
6. **Migrate**: run `velocity data migrate up` for any new database schema
   changes
7. **Start**: `systemctl start velocity-report.service`
8. **Verify**: confirm service is active and responding

If `--check` is passed, only step 1 runs and the result is printed. If
`--binary` is passed, steps 1–2 are skipped and the local file is used
(for offline or air-gapped upgrades).

##### Implementation scope for v0.5.1

Device lifecycle code at [internal/cmd/device/](../../internal/cmd/device) is
now exposed through the multi-call `velocity device` namespace. This replaces
`cmd/deploy/` entirely: no SSH surface and no remote execution.

- `upgrade`: GitHub release check + download + backup + stop + install +
  migrate + start + verify. `--check` flag for version comparison only.
  `--binary` flag for offline upgrades.
- `rollback`: restore binary + database from most recent timestamped backup
- `backup`: create manual snapshot of binary + database
- `status`: thin wrapper around `systemctl status velocity-report`
- `version`: print installed `velocity` / `velocity-report` version metadata

The upgrade subcommand includes:

- GitHub release checking: HTTP GET to
  `https://api.github.com/repos/banshee-data/velocity.report/releases/latest`,
  parse JSON for `tag_name` and asset URLs
- Binary download: HTTP GET the linux/arm64 static asset URL, write to a temp
  file, and verify SHA-256 against release metadata
- `--binary` optional: if omitted, auto-download from GitHub

`cmd/deploy/` is deleted in v0.5.1. The SSH surface (`executor.go`,
`sshconfig.go`), remote install, fix, config, and health subcommands, and the
three legacy upgrade steps (`updateSourceCode`, `ensureLaTeX`,
`updatePythonDependencies`) are not carried forward.

##### Privacy guarantees

- **No unsolicited requests**: the tool only contacts GitHub when the user
  explicitly runs `velocity device upgrade`
- **No telemetry**: no analytics, no tracking, no phone-home
- **No background processes**: no cron, no timer, no daemon
- **Public API only**: GitHub Releases API for public repos requires no
  authentication token
- **Verifiable**: SHA-256 checksum verification ensures binary integrity

##### Rollback

If an upgrade fails or causes problems:

```bash
sudo velocity device rollback      # restore most recent backup
```

This restores the binary and database from the timestamped backup created
during the upgrade.

2. **Settings dashboard version banner**: the web UI settings page will
   display the currently installed version. A future "Check for updates"
   button is planned but not yet implemented.

#### 4.2.3 System configuration

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

#### 4.2.4 RS-232 HAT driver configuration

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

### 4.3 pi-gen integration

```
pi-gen/
├── config                          # IMG_NAME=velocity-report
├── stage0/                         # Bootstrap (upstream, untouched)
├── stage1/                         # Minimal system (upstream, untouched)
├── stage2/                         # Lite system (upstream, untouched)
│   └── SKIP_IMAGES                 # Don't produce image at stage2
├── stage-velocity/                 # ★ Custom stage
│   ├── 00-install-packages/
│   │   └── 00-packages             # Runtime APT manifest: raspi-config
│   ├── 01-velocity-binaries/
│   │   └── 00-run.sh              # Install staged static multi-call binary
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

### 4.4 CI pipeline (GitHub actions)

The CI pipeline ([.github/workflows/build-image.yml](../../.github/workflows/build-image.yml)) triggers on version-tag pushes and manual `workflow_dispatch`. It runs the static Linux binary build route, stages the ARM64 artifact, then builds the pi-gen image:

| Step                 | Runner          | Purpose                                                                                                              |
| -------------------- | --------------- | -------------------------------------------------------------------------------------------------------------------- |
| Static Linux build   | `ubuntu-latest` | Build linux/arm64 through `scripts/build-radar-static.sh` using Docker, zig/musl, and vendored `libpcap.a`           |
| Binary staging       | `ubuntu-latest` | Copy the ARM64 static ELF into `image/velocity-binaries/` via `scripts/stage-image-binary.sh` and verify static ELF  |
| Image build          | `ubuntu-latest` | Run pi-gen with stages `stage0 stage1 stage2 stage-velocity`; compress with `xz`; upload `.img.xz` to GitHub Release |
| Repository catalogue | `ubuntu-latest` | Generate SHA-256 checksum and update `os-list-velocity.json` release metadata                                        |

### 4.5 Image size budget

> **Current v0.5.1 image** ships no TeX tree and no dynamic libpcap dependency
> from the application binary. The remaining optional-access package surface is
> Tailscale until the in-binary installer lands.

| Component                                  | Estimated Size  |
| ------------------------------------------ | --------------- |
| Raspberry Pi OS Lite (base)                | ~450 MB         |
| Static Go binary with embedded Typst/docs  | ~65 MB          |
| LiDAR/system config plus Tailscale package | ~60 MB          |
| **Total (xz compressed)**                  | **~150–300 MB** |

The historical TeX reduction work stream below is retained only for context;
the current image removed the TeX tree instead.

### 4.6 Historical LaTeX size reduction work stream

This section is superseded by the Go + Typst cutover. The current image ships
no `texlive-xetex`, no minimal TeX tree, and no precompiled `.fmt` files. The
table below is retained only as context for the cancelled TeX reduction path.

#### 4.6.1 Options

| Option                                     | Approach                                                                                                                           | Estimated Savings                              |
| ------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------- |
| ~~**A: TinyTeX**~~                         | Install TinyTeX (a minimal, portable TeX Live distribution) and add only the LaTeX packages velocity.report actually uses          | ~600–700 MB saved                              |
| ~~**B: Pre-compiled templates**~~          | Ship pre-compiled `.fmt` files and only the fonts/packages referenced by our report templates; no general-purpose TeX installation | ~700–750 MB saved                              |
| ~~**C: Hybrid (TinyTeX + pre-compiled)**~~ | Install TinyTeX with pre-compiled format files for our templates; users can still install additional packages if needed            | ~650–700 MB saved                              |
| ~~**D: Docker sidecar**~~                  | Run LaTeX compilation inside a Docker container pulled on demand; no TeX in the base image at all                                  | ~800 MB saved (but adds Docker + runtime pull) |

#### 4.6.2 Evaluation matrix

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

The old recommendation, Option B, is cancelled. Typst eliminated the xelatex
pipeline instead of optimising it.

#### 4.6.4 Implementation steps

1. ~~**Audit template dependencies**: `dependency-manifest.txt` lists every
   `.sty`, `.cls`, font, and binary the PDF generator uses
2. ~~**Build a minimal TeX tree**: `scripts/build-minimal-texlive.sh` extracts
   only the required files from the full TeX Live distribution into
   `/opt/velocity-report/texlive/` (~143 MB). Pi-gen stage
   `00-install-packages/01-run.sh` runs this at image build time and purges
   the APT packages afterward~~
3. ~~**Pre-compile format files** (Phase 2): run `xelatex -ini` to produce
   `.fmt` files for each report template; eliminates per-run format-loading
   overhead~~
4. ~~**Update pi-gen stage**: `00-install-packages` installs `texlive-xetex`
   APT packages, builds the minimal tree, and purges the APT packages~~
5. ~~**Validate output**: PDF output validated between full TeX Live and
   minimal builds with no rendering regressions~~
6. ~~**Measure**: minimal tree: ~143 MB vs full TeX Live: ~800 MB (~1 GB saved)~~

---

## 5. Tier 2: image flashing (rpi-imager)

### 5.1 Approach comparison

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

### 5.2 Recommendation: phased approach

**Phase 1 (Immediate):** Use **Approach A; Custom Repository JSON**

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

## 6. Decision matrix: monorepo vs. separate repository

If/when we proceed with the rpi-imager fork (Phase 2), the code must live
somewhere. Here is the analysis:

### 6.1 Comparison matrix

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

### 6.2 Scoring summary

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
- Referencing [image/stage-velocity/03-velocity-config/files/velocity-report.service](../../image/stage-velocity/03-velocity-config/files/velocity-report.service)
  as the canonical service definition

### 6.4 What stays in the monorepo

Even with the imager in a separate repository, the following **must** remain in
the `velocity.report` monorepo:

| Asset                   | Location                                                                                                                                             | Reason                                                               |
| ----------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------- |
| pi-gen stage scripts    | [image/stage-velocity/](../../image/stage-velocity)                                                                                                  | Defines what goes in the image; tightly coupled to server releases   |
| OS-list repository JSON | [image/os-list-velocity.json](../../image/os-list-velocity.json)                                                                                     | Catalogue of available images; updated by CI on release              |
| Image CI workflow       | [.github/workflows/build-image.yml](../../.github/workflows/build-image.yml)                                                                         | Triggered by monorepo releases                                       |
| systemd service file    | [image/stage-velocity/03-velocity-config/files/velocity-report.service](../../image/stage-velocity/03-velocity-config/files/velocity-report.service) | Canonical source                                                     |
| udev rules              | [image/stage-velocity/03-velocity-config/files/](../../image/stage-velocity/03-velocity-config/files)                                                | Device permission rules                                              |
| Management namespace    | [internal/cmd/device/](../../internal/cmd/device)                                                                                                    | `velocity device upgrade`, `rollback`, `backup`, `status`, `version` |
| LiDAR network config    | [internal/cmd/device/files/lidar-network.conf](../../internal/cmd/device/files/lidar-network.conf)                                                   | Static IP for 192.168.100.x subnet (disabled by default)             |

---

## 7. Pitfalls and risks

### 7.1 Image building pitfalls

| Risk                                                                                    | Severity | Mitigation                                                                                                                                                                             |
| --------------------------------------------------------------------------------------- | -------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Image size regression**: package stages or embedded assets grow unnoticed             | Medium   | Static binary verification, package manifest review, and release image size check before publish                                                                                       |
| **pi-gen build flakiness**: network-dependent APT fetches can fail                      | Medium   | Pin package versions; use local APT mirror in CI; retry logic                                                                                                                          |
| **ARM64 QEMU emulation speed**: pi-gen builds on x86 CI runners use QEMU for ARM chroot | Medium   | Use native ARM64 runners (GitHub now offers them) or cross-compile everything outside the chroot                                                                                       |
| **Python venv portability**: venvs built on x86 may not work on ARM64                   | High     | Build the venv inside the ARM64 chroot (pi-gen stage script) or use wheels with `--platform manylinux_2_28_aarch64`                                                                    |
| **Image size exceeding GitHub release limits**: GitHub has a 2 GB per-asset limit       | Medium   | Use xz compression (typical 3:1 ratio); consider hosting on a dedicated CDN for larger images                                                                                          |
| **Serial port conflicts**: Bluetooth uses the same UART on Pi 4                         | Medium   | Overlay `miniuart-bt` moves Bluetooth to mini-UART; document for users with Bluetooth peripherals                                                                                      |
| **SD card wear**: SQLite WAL mode on SD cards can cause premature failure               | Low      | Document recommended SD card brands; consider moving WAL to tmpfs with periodic sync                                                                                                   |
| **LiDAR pcap binary size**: building with pcap adds ~5 MB to the Go binary              | Low      | Acceptable trade-off; LiDAR hardware support is included but disabled by default; no runtime cost when off                                                                             |
| **First-boot configuration**: users need to set Wi-Fi before the device has a screen    | Medium   | Leverage rpi-imager's built-in Wi-Fi/SSH customisation; image defaults to US regulatory domain (`country=US`) so wireless is functional even if the user skips Wi-Fi country selection |

### 7.2 rpi-imager fork pitfalls (phase 2)

| Risk                                                                                                                      | Severity | Mitigation                                                                             |
| ------------------------------------------------------------------------------------------------------------------------- | -------- | -------------------------------------------------------------------------------------- |
| **Qt version churn**: rpi-imager requires Qt 6.7+; major version upgrades break APIs                                      | High     | Pin Qt version; sync with upstream only on stable releases                             |
| **Cross-platform packaging**: building .dmg (macOS), .exe/.msi (Windows), .AppImage (Linux) requires platform-specific CI | High     | Use upstream's existing packaging scripts; GitHub Actions matrix builds                |
| **Code signing**: macOS and Windows require signed binaries to avoid security warnings                                    | High     | Obtain Apple Developer and Windows Authenticode certificates; budget ~$200/year        |
| **Upstream divergence**: the more we customise, the harder merges become                                                  | Medium   | Minimise changes: branding + default repo URL only; avoid touching core flashing logic |
| **Dependency licensing**: Qt is LGPL; must comply with linking requirements                                               | Medium   | Dynamic linking (already the upstream approach); include LGPL notice                   |
| **User confusion**: two "imager" apps on the system                                                                       | Low      | Clear naming: "velocity.report Imager" vs "Raspberry Pi Imager"                        |

### 7.3 General risks

| Risk                                                                      | Severity | Mitigation                                                                           |
| ------------------------------------------------------------------------- | -------- | ------------------------------------------------------------------------------------ |
| **Scope creep**: image building project absorbs all engineering time      | High     | Strict phased approach; Phase 1 (JSON repo) delivers value in days, not weeks        |
| **Security**: pre-built images could be tampered with                     | High     | SHA-256 checksums in os-list JSON; GPG-signed releases; reproducible builds          |
| **Support burden**: "it didn't boot" becomes the #1 issue                 | Medium   | Comprehensive first-boot diagnostics; LED status codes; web-based setup wizard       |
| **Raspberry Pi OS upgrades**: new Debian releases break our image scripts | Medium   | Pin to Bookworm; test quarterly against new releases; document supported OS versions |

---

## 8. Deploy tool replacement: `velocity device`

`cmd/deploy/` (the `velocity-deploy` binary) is **deleted in v0.5.1** and
replaced by [internal/cmd/device/](../../internal/cmd/device), exposed as the
`velocity device ...` namespace in the multi-call `velocity` binary. This is a
clean break, not a gradual deprecation: there are no existing image users to
migrate, and shipping both command surfaces creates a limbo state where two
tools with overlapping names do different things.

### 8.1 What changes

| Before (deleted)                           | After                               |
| ------------------------------------------ | ----------------------------------- |
| `velocity-deploy` (3,678 LOC, 10 Go files) | `velocity device ...`               |
| `velocity-update` (21-line bash wrapper)   | _(deleted: no wrapper needed)_      |
| 8 subcommands, SSH surface, legacy steps   | Local-only device lifecycle, no SSH |

### 8.2 Subcommand map

```
velocity device                        # On-device management (root)
  ├── upgrade    (from deploy)         # Check + download + apply release
  ├── rollback   (from deploy)         # Restore previous version from backup
  ├── backup     (from deploy)         # Manual snapshot of binary + database
  ├── status     (new, thin)           # systemctl status wrapper
  └── version    (new)                 # Show installed versions
```

**Not carried forward** from `cmd/deploy/`: `install`, `fix`, `config`,
`health`, SSH execution (`executor.go`, `sshconfig.go`), legacy upgrade steps
(`updateSourceCode`, `ensureLaTeX`, `updatePythonDependencies`).

### 8.3 Why `velocity device` (not `velocity-deploy`)

- **No name collision**: `velocity-deploy` implied pushing code TO somewhere
  over SSH. On-device, the tool pulls an update DOWN to itself. The name was
  actively misleading.
- **Clean command surface**: the same binary owns install, upgrade, rollback,
  backup, status, and Tailscale service toggling through one namespace.
- **Scoped privilege domain**: only enumerated `velocity device ...` commands
  are granted in sudoers.
- **Smaller shipped surface**: no second promoted Go binary and no dead SSH
  deploy code ships on the image.

### 8.4 Image binaries (v0.5.1)

One Go binary, two entry-point names:

| Binary            | Install Path                     | Runs as                  | Purpose                            |
| ----------------- | -------------------------------- | ------------------------ | ---------------------------------- |
| `velocity`        | `/usr/local/bin/velocity`        | root/operator or service | Canonical CLI and device lifecycle |
| `velocity-report` | `/usr/local/bin/velocity-report` | velocity                 | Server compatibility alias         |

### 8.5 Deleted artefacts

The following are removed from the repository in v0.5.1:

- `cmd/deploy/`: entire directory (10 source files, 10 test files, README)
- [image/stage-velocity/01-velocity-binaries/files/velocity-update](../../image/stage-velocity/01-velocity-binaries/files/velocity-update): bash wrapper <!-- link-ignore -->
- Makefile targets: `build-deploy`, `build-deploy-linux`, `deploy-install`,
  `deploy-upgrade`, `deploy-status`, `deploy-health`, `deploy-install-latex`,
  `deploy-install-latex-minimal`, `deploy-update-deps`, `setup-radar`
- `scripts/setup-radar-host.sh`

### 8.6 Consolidation status

The future consolidation described in the original plan has landed. The image
ships the multi-call `velocity` binary; `velocity-report` remains only as a
compatibility alias for the server surface.

---

## 9. Implementation phases

### Phase 0: prerequisites (1–2 days)

- [x] Verify `make build-radar-static-arm64` produces a static ARM64 binary with LiDAR pcap support
- [x] Verify Go + Typst PDF generation works on ARM64 Raspberry Pi OS
- [x] Document the exact project APT manifest (`raspi-config`, plus the separate Tailscale stage)
- [ ] Test RS-232 HAT configuration manually on a Raspberry Pi 4
- [ ] Verify LiDAR packet capture works on Pi 4 with pcap-enabled binary (disabled by default, enable with `--enable-lidar`)

### Phase 1: image building with pi-gen (1–2 weeks) ✅ complete

- [x] Create `image/` directory in monorepo
- [x] Write pi-gen `config` file and `stage-velocity/` scripts
- [x] Include static multi-call `velocity` binary and version metadata in image
- [x] Configure US Wi-Fi regulatory domain fallback
- [x] Include LiDAR support (libpcap, network config) disabled by default
- [x] Create GitHub Actions workflow for image building
- [x] Create [image/os-list-velocity.json](../../image/os-list-velocity.json) with schema-compliant entries
- [ ] Test image on physical Raspberry Pi 4 hardware
- [ ] Produce first `.img.xz` release asset

Note: current Phase 1 no longer installs `texlive-xetex` or stages a minimal
TeX tree. The remaining image-size cleanup is the Tailscale package stage.

### Phase 2: custom repository JSON (2–3 days)

- [x] Create [image/os-list-velocity.json](../../image/os-list-velocity.json) with schema-compliant entries
- [ ] Host JSON on GitHub Pages or alongside releases
- [ ] Write end-user documentation: "How to flash velocity.report"
- [ ] Add `--repo` instructions to main README
- [ ] Test with stock rpi-imager on macOS, Windows, Linux

### Phase 3: first-boot experience (1 week)

- [ ] Create a first-boot script that validates radar connectivity
- [ ] Add a web-based setup wizard (accessible at `http://velocity.local/setup`)
- [ ] LED status indicator for boot progress (optional, GPIO-dependent)
- [ ] Smoke-test the full flow: flash → boot → radar detected → web UI accessible

### Phase 4: rpi-imager fork (4–8 weeks, only if warranted)

- [ ] Fork `raspberrypi/rpi-imager` to `banshee-data/velocity.report-imager`
- [ ] Rebrand UI: velocity.report logo, colour scheme, application name
- [ ] Set default `--repo` to velocity.report's os-list JSON
- [ ] Add custom first-boot fields (site name, radar port override)
- [ ] Set up cross-platform CI (macOS .dmg, Windows .exe, Linux .AppImage)
- [ ] Obtain code-signing certificates (Apple Developer + Windows Authenticode)
- [ ] Publish v1.0.0 release with binaries for all three platforms
- [ ] Establish upstream sync schedule (quarterly merge from `raspberrypi/rpi-imager`)

---

## 10. Repository layout (monorepo additions)

```
velocity.report/
├── image/                              # ★ New directory
│   ├── README.md                       # Image building documentation
│   ├── os-list-velocity.json           # rpi-imager custom repository catalogue
│   ├── config                          # pi-gen configuration
│   ├── stage-velocity/                 # pi-gen custom stage
│   │   ├── 00-packages                 # APT package list: raspi-config
│   │   ├── 01-velocity-binaries/
│   │   │   ├── 00-run.sh
│   │   │   └── files/
│   │   │       └── velocity            # Staged static multi-call binary
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

## 11. os-list JSON schema (phase 2)

A single image entry: the full stack with radar, LiDAR (disabled), PDF
generation, and web dashboard:

| Field                            | Value                                                                                                                                                                      | Purpose                          |
| -------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------- |
| `imager.latest_version`          | `"1.0.0"`                                                                                                                                                                  | Imager version                   |
| `imager.url`                     | GitHub releases URL                                                                                                                                                        | Imager download location         |
| `os_list[0].name`                | `"velocity.report"`                                                                                                                                                        | Image display name               |
| `os_list[0].description`         | Privacy-first traffic monitoring — full stack with radar, LiDAR (disabled by default), PDF reporting, and web dashboard. Based on Raspberry Pi OS Lite (Bookworm, 64-bit). | User-facing description          |
| `os_list[0].url`                 | GitHub release `.img.xz` asset URL                                                                                                                                         | Download URL                     |
| `os_list[0].extract_size`        | `1073741824`                                                                                                                                                               | Uncompressed image size (bytes)  |
| `os_list[0].extract_sha256`      | SHA-256 of uncompressed `.img`                                                                                                                                             | Integrity check                  |
| `os_list[0].image_download_size` | `419430400`                                                                                                                                                                | Compressed download size (bytes) |
| `os_list[0].release_date`        | `"2026-03-01"`                                                                                                                                                             | Release date                     |
| `os_list[0].icon`                | `icon-256.png` URL                                                                                                                                                         | 256×256 icon                     |
| `os_list[0].init_format`         | `"systemd"`                                                                                                                                                                | Init system                      |
| `os_list[0].devices`             | `pi4-64bit`, `pi400-64bit`, `pi5-64bit`                                                                                                                                    | Supported hardware               |
| `os_list[0].url_info`            | Setup guide URL                                                                                                                                                            | Documentation link               |

---

## 12. Security considerations

### 11.1 Image integrity

- Every release image **must** include SHA-256 checksums in both the GitHub
  release notes and the os-list JSON `extract_sha256` field
- Consider GPG-signing release assets with a project key
- CI builds should be deterministic: same inputs → same image hash

### 11.2 Supply chain

- Pin all APT package versions in pi-gen scripts
- Use GitHub Actions' built-in artifact attestation
- Generate SBOM for each image release (rpi-image-gen does this automatically)

### 11.3 Runtime security

- The `velocity` service user runs with minimal privileges and only scoped sudoers entries for required Tailscale toggles
- The systemd service uses `DynamicUser=` or a dedicated system user
- Serial port access is granted via udev rules, not blanket permissions
- Current image defaults to `pi` / `report` for initial local setup with SSH
  enabled; the MOTD warns the operator to change it immediately

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

- [raspberrypi/rpi-imager](https://github.com/raspberrypi/rpi-imager); Apache 2.0 licence, C++/Qt6/QML/CMake
- [RPi-Distro/pi-gen](https://github.com/RPi-Distro/pi-gen); Stage-based image builder, BSD licence
- [raspberrypi/rpi-image-gen](https://github.com/raspberrypi/rpi-image-gen); New declarative image builder (2025), BSD licence
- [rpi-imager custom repository JSON schema](https://github.com/raspberrypi/rpi-imager/blob/main/doc/json-schema/os-list-schema.json)
- [How to add your own images to Imager](https://www.raspberrypi.com/news/how-to-add-your-own-images-to-imager/)
- [velocity.report distribution-packaging-plan.md](./deploy-distribution-packaging-plan.md) § 8.2
- [velocity.report ARCHITECTURE.md](../../ARCHITECTURE.md)
- [velocity-report.service](../../image/stage-velocity/03-velocity-config/files/velocity-report.service): Canonical systemd unit
- [velocity.report frontend-consolidation.md](./web-frontend-consolidation-plan.md): LiDAR toggle UI dependency

---

## 14. Summary of recommendations

| Decision                              | Recommendation                                                                             | Rationale                                                                                                         |
| ------------------------------------- | ------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------- |
| **Image building tool**               | Start with pi-gen, plan migration to rpi-image-gen                                         | pi-gen is proven; rpi-image-gen is better long-term but newer                                                     |
| **Image flashing (Phase 1)**          | Custom repository JSON for stock rpi-imager                                                | Zero development cost; immediate value                                                                            |
| **Image flashing (Phase 2)**          | Fork rpi-imager into separate repo (only if needed)                                        | Full branding + custom fields; only justified by user research                                                    |
| **Repository for imager fork**        | **Separate repo** (`banshee-data/velocity.report-imager`)                                  | Different tech stack (C++/Qt), release cadence, and contributor profile                                           |
| **Image build scripts**               | **Monorepo** (`velocity.report/image/`)                                                    | Tightly coupled to server releases; same CI pipeline                                                              |
| **Image variants**                    | **Single image** with full stack                                                           | LiDAR disabled by default; static libpcap and embedded Typst avoid runtime dependency drift                       |
| **Report compiler footprint**         | Go + embedded Typst; no TeX tree                                                           | Smaller image surface and no xelatex runtime packages                                                             |
| **LiDAR support**                     | Included (pcap build) but **disabled by default**                                          | Users enable via settings dashboard; depends on [frontend-consolidation.md](./web-frontend-consolidation-plan.md) |
| **Auto-update**                       | **None**: user-initiated `sudo velocity device upgrade`; dashboard version display planned | Preserves privacy-first principle; zero unsolicited network requests                                              |
| **Wi-Fi fallback**                    | US regulatory domain (`country=US`)                                                        | Ensures wireless works out of the box if user skips country selection                                             |
| **Custom flashing tool (Approach C)** | **Do not pursue**                                                                          | Re-implementing disk I/O is high-risk and low-value                                                               |
