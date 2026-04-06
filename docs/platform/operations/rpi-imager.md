# Raspberry Pi Imager

Active plan: [deploy-rpi-imager-fork-plan.md](../../plans/deploy-rpi-imager-fork-plan.md)

## Problem

Deploying velocity.report on a Raspberry Pi requires a multi-step manual
process: flashing Raspberry Pi OS, SSHing in, installing Go binaries, setting
up Python venv with LaTeX dependencies, configuring RS-232 HAT drivers, and
enabling systemd services. This is a barrier for neighbourhood change-makers
who are not comfortable with Linux system administration.

## Two-Tier Solution

| Tier               | Problem                                                | Tool                                           |
| ------------------ | ------------------------------------------------------ | ---------------------------------------------- |
| **Image Building** | Create a complete `.img` with the full stack installed | `pi-gen` or `rpi-image-gen` (CI pipeline)      |
| **Image Flashing** | End users write image to SD card                       | Fork of `rpi-imager` or custom repository JSON |

A single image ships the full stack — radar, LiDAR (disabled by default),
PDF generation, and web dashboard.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                   CI Pipeline (GitHub Actions)              │
│  ┌───────────────┐    ┌──────────────┐    ┌──────────────┐  │
│  │ pi-gen /      │    │ Go cross-    │    │ Python wheel │  │
│  │ rpi-image-gen │◄───│ compile      │◄───│ + LaTeX deps │  │
│  └──────┬────────┘    └──────────────┘    └──────────────┘  │
│         ▼                                                   │
│  ┌──────────────┐    ┌───────────────────────────────────┐  │
│  │ .img.xz file │───►│ GitHub Release + os-list JSON     │  │
│  └──────────────┘    └───────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

## Image Contents

The image extends Raspberry Pi OS Lite (64-bit, Bookworm) with:

### Binaries

| Component                            | Install Path                                |
| ------------------------------------ | ------------------------------------------- |
| `velocity-report` (Go, pcap-enabled) | `/usr/local/bin/velocity-report`            |
| `velocity-deploy`                    | `/usr/local/bin/velocity-deploy`            |
| `velocity-update` (shell wrapper)    | `/usr/local/bin/velocity-update`            |
| PDF generator (Python)               | `/opt/velocity-report/tools/pdf-generator/` |
| Python venv                          | `/opt/velocity-report/.venv/`               |
| Web frontend                         | Embedded in Go binary                       |

The Go binary is built with `CGO_ENABLED=1` and `-tags pcap` so that LiDAR
packet capture is available at runtime. LiDAR is **disabled by default**;
users enable it through the web settings dashboard.

### System Configuration

- Systemd service at `/etc/systemd/system/velocity-report.service`
- Data directory `/var/lib/velocity-report/` owned by `velocity` user
- UART overlay enabled in `/boot/firmware/config.txt` (`miniuart-bt`)
- Serial console disabled (frees `/dev/ttyAMA0` for radar)
- USB-Serial udev rule creating `/dev/velocity-radar` symlink
- `velocity` user in `dialout` group
- LiDAR network interface pre-configured (192.168.100.1/24, manual, disabled)
- US Wi-Fi regulatory domain fallback

### Update Mechanism

No automatic updates — preserves privacy-first principle (zero unsolicited
network requests).

- `velocity-update` script: thin wrapper around `velocity-deploy upgrade`,
  user runs explicitly
- Settings dashboard: displays installed version, "Check for updates" makes
  a single GitHub API call when user clicks it

## Image Building: pi-gen Integration

```
pi-gen/
├── stage0–2/                       # Upstream (untouched)
├── stage-velocity/                 # Custom stage
│   ├── 00-packages                 # APT (texlive, libpcap-dev, etc.)
│   ├── 01-velocity-binaries/       # Go binaries + update script
│   ├── 02-velocity-python/         # Venv + PDF generator
│   ├── 03-velocity-config/         # User, service, serial, udev
│   ├── 04-velocity-lidar/          # LiDAR network (disabled)
│   ├── 05-velocity-wifi/           # US regulatory fallback
│   └── EXPORT_IMAGE
├── stage3–5/                       # SKIP (desktop not needed)
```

Tool comparison: start with **pi-gen** (proven), plan migration to
**rpi-image-gen** (faster, declarative, SBOM) once image requirements
stabilise.

## Image Size Budget

| Component                                    | Estimated Size  |
| -------------------------------------------- | --------------- |
| Raspberry Pi OS Lite (base)                  | ~450 MB         |
| TeX Live (before reduction)                  | ~800 MB         |
| Python 3 + venv + PDF deps                   | ~200 MB         |
| Go binaries (server + deploy, pcap)          | ~35 MB          |
| LiDAR + web + system config                  | ~11 MB          |
| **Total (compressed, before TeX reduction)** | **~600–900 MB** |
| **Target (after TeX reduction)**             | **~350–500 MB** |

### LaTeX Size Reduction (Chosen: Pre-compiled Templates)

Full `texlive-xetex` adds ~800 MB. The chosen approach (D-08, Option B)
ships only pre-compiled `.fmt` files, specific fonts used by report
templates, and a minimal XeTeX binary. No package manager, no `tlmgr`, no
network fetches. Savings: ~750 MB.

Steps: audit template deps → build minimal TeX tree in
`/opt/velocity-report/texlive-minimal/` → pre-compile formats → replace
APT packages → validate output byte-for-byte → measure sizes.

## Image Flashing: Phased Approach

### Phase 1 (Immediate): Custom Repository JSON

Host a JSON catalogue; users launch stock rpi-imager with `--repo` flag:

```bash
rpi-imager --repo https://velocity.report/images/os-list.json
```

Zero development cost, immediate value. Single image entry covering the
full stack. Targets: `pi4-64bit`, `pi400-64bit`, `pi5-64bit`.

### Phase 2 (Future, If Warranted): Fork rpi-imager

Only pursue if user research shows the custom repo step is a significant
adoption barrier, or custom first-boot fields are needed (site name, radar
port). Lives in a **separate repository** (`banshee-data/velocity.report-imager`)
— different tech stack (C++/Qt), release cadence, and contributor profile.

## What Stays in the Monorepo

| Asset                   | Location                               | Reason                                              |
| ----------------------- | -------------------------------------- | --------------------------------------------------- |
| pi-gen stage scripts    | `image/`                               | Tightly coupled to server releases                  |
| OS-list repository JSON | `image/os-list-velocity.json`          | Updated by CI on release                            |
| Image CI workflow       | `.github/workflows/build-image.yml`    | Triggered by monorepo releases <!-- link-ignore --> |
| systemd service         | `cmd/deploy/velocity-report.service`   | Canonical source                                    |
| udev rules              | `image/files/99-velocity-report.rules` | Device permissions                                  |
| Update script           | `image/files/velocity-update`          | User-initiated updates                              |
| Minimal TeX tree        | `image/files/texlive-minimal/`         | Pre-compiled templates + fonts                      |
| LiDAR network config    | `image/files/lidar-network.conf`       | Static IP for 192.168.100.x                         |
| First-boot script       | `image/files/velocity-first-boot.sh`   | Optional setup wizard                               |

## Security

- SHA-256 checksums in GitHub release notes and os-list JSON
- GPG-signed release assets considered
- CI builds should be deterministic (same inputs → same hash)
- Pin APT package versions; use GitHub Actions artifact attestation
- `velocity` user runs with minimal privileges (no sudo)
- Serial port access via udev rules, not blanket permissions
- No default passwords — rpi-imager first-boot handles user creation
- **No telemetry, no phone-home, no automatic updates, no cloud endpoints,
  no SSH keys, no PII in the image**

## Key Risks

| Risk                       | Mitigation                                                |
| -------------------------- | --------------------------------------------------------- |
| TeX Live size bloat        | Pre-compiled templates; target < 200 MB                   |
| pi-gen build flakiness     | Pin package versions; local APT mirror in CI; retry logic |
| ARM64 QEMU emulation speed | Native ARM64 runners or cross-compile outside chroot      |
| Python venv portability    | Build inside ARM64 chroot or use platform-specific wheels |
| GitHub 2 GB asset limit    | xz compression (3:1 ratio); CDN for larger images         |
| Serial port conflicts      | `miniuart-bt` overlay moves Bluetooth to mini-UART        |
| "It didn't boot" support   | First-boot diagnostics, LED status, web setup wizard      |
| Scope creep                | Strict phased approach; Phase 1 delivers value in days    |
