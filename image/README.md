# Raspberry Pi Image Pipeline

Build infrastructure for producing flashable `.img` files of
velocity.report for Raspberry Pi 4/400/5.

## Phase 1 — Working Image (v0.5.1)

Ships the current codebase as-is with full `texlive-xetex` APT packages
(~800 MB uncompressed). No LaTeX size reduction; that is Phase 2 (v0.6.0).

### What the Image Contains

| Component                            | Install Path                                |
| ------------------------------------ | ------------------------------------------- |
| `velocity-report` (Go, pcap-enabled) | `/usr/local/bin/velocity-report`            |
| `velocity-ctl` (device management)   | `/usr/local/bin/velocity-ctl`               |
| PDF generator (Python)               | `/opt/velocity-report/tools/pdf-generator/` |
| Python venv                          | `/opt/velocity-report/.venv/`               |
| Web frontend                         | Embedded in Go binary                       |

LiDAR packet capture is compiled in (pcap build) but **disabled by default**.

### System Configuration

- Systemd service auto-starts on boot
- Data directory `/var/lib/velocity-report/` owned by `velocity` user
- UART overlay enabled for RS-232 HAT radar connection
- Serial console removed from kernel command line
- USB-Serial udev rule creates `/dev/velocity-radar` symlink
- LiDAR network interface pre-configured but disabled
- US Wi-Fi regulatory domain fallback

### No Automatic Updates

The image makes zero unsolicited network requests. Updates are user-initiated
via `sudo velocity-ctl upgrade`, which checks GitHub Releases for a newer
version, downloads the binary, verifies the SHA-256 checksum, and upgrades
in-place — preserving the sensor database and all collected data.

```bash
sudo velocity-ctl upgrade              # check + download + apply latest release
sudo velocity-ctl upgrade --check      # print version comparison only
sudo velocity-ctl upgrade --binary /f  # apply a local binary (offline upgrade)
```

Rollback: `sudo velocity-ctl rollback` restores the previous version.

## Directory Layout

```
image/
├── config                          # pi-gen configuration
├── os-list-velocity.json           # rpi-imager custom repository catalogue
├── README.md                       # This file
├── scripts/
│   └── build-image.sh              # Local build helper
└── stage-velocity/                 # pi-gen custom stage
    ├── 00-packages                 # APT package list
    ├── 01-velocity-binaries/       # Go binaries + update script
    │   ├── 00-run.sh
    │   └── files/
    │       └── velocity-update              # Redirect stub (prints "use velocity-ctl upgrade")
    ├── 02-velocity-python/         # Python venv + PDF generator
    │   └── 00-run.sh
    ├── 03-velocity-config/         # User, service, serial, udev
    │   ├── 00-run.sh
    │   └── files/
    │       ├── 99-velocity-report.rules
    │       └── velocity-report.service → cmd/deploy/ (symlink)
    ├── 04-velocity-lidar/          # LiDAR network (disabled by default)
    │   ├── 00-run.sh
    │   └── files/
    │       └── lidar-network.conf
    ├── 05-velocity-wifi/           # US Wi-Fi regulatory fallback
    │   ├── 00-run.sh
    │   └── files/
    │       └── wpa_supplicant.conf
    └── EXPORT_IMAGE
```

## Building Locally

```bash
./image/scripts/build-image.sh
```

Requires Docker. See the script header for prerequisites.

## CI Pipeline

The GitHub Actions workflow at `.github/workflows/build-image.yml` builds
the image on release publication or manual dispatch. It cross-compiles Go
binaries, bundles the Python PDF generator, runs pi-gen inside Docker,
compresses the output with xz, and uploads the `.img.xz` to the GitHub
Release.

## Flashing

Users flash with stock Raspberry Pi Imager pointed at the custom repository:

```bash
rpi-imager --repo https://velocity.report/images/os-list.json
```

Or use any image-writing tool (`dd`, balenaEtcher) with the `.img.xz` file
downloaded from the GitHub Release.

## Image Size Budget (Phase 1)

| Component                                 | Estimated Size  |
| ----------------------------------------- | --------------- |
| Raspberry Pi OS Lite (base)               | ~450 MB         |
| TeX Live (full, before Phase 2 reduction) | ~800 MB         |
| Python 3 + venv + PDF deps                | ~200 MB         |
| Go binaries (server + deploy, pcap)       | ~35 MB          |
| LiDAR + web + system config               | ~11 MB          |
| **Total (xz compressed)**                 | **~600–900 MB** |

## Design Document

Full design: [deploy-rpi-imager-fork-plan.md](../docs/plans/deploy-rpi-imager-fork-plan.md)
