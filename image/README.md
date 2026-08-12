# Raspberry Pi image pipeline

Build infrastructure for producing flashable `.img` files of
velocity.report for Raspberry Pi 4/400/5.

## Phase 1: working image (v0.5.1)

Builds a single fully static, pcap-enabled Go binary with vendored libpcap
and the Typst PDF engine embedded. The image does not install legacy report
compiler packages or SVG-to-PDF conversion packages for report generation.

### What the image contains

| Component                            | Install Path                     |
| ------------------------------------ | -------------------------------- |
| `velocity` multi-call binary         | `/opt/velocity-report/current`   |
| `velocity-report` compatibility link | `/usr/local/bin/velocity-report` |
| `velocity` canonical link            | `/usr/local/bin/velocity`        |
| Web frontend                         | Embedded in Go binary            |

LiDAR packet capture is compiled in (pcap build) but **disabled by default**.

### System configuration

- Systemd service auto-starts on boot
- Data directory `/var/lib/velocity-report/` owned by `velocity` user
- Primary wired LAN pre-configured for NetworkManager DHCP
- UART overlay enabled for RS-232 HAT radar connection
- Serial console removed from kernel command line
- USB-Serial udev rule creates `/dev/velocity-radar` symlink
- LiDAR network interface pre-configured but disabled
- US Wi-Fi regulatory domain fallback

### No automatic updates

The image makes zero unsolicited network requests. Updates are user-initiated
via `sudo velocity device upgrade`, which checks GitHub Releases for a newer
version, downloads the binary, verifies the SHA-256 checksum, and upgrades
in-place: preserving the sensor database and all collected data.

```bash
sudo velocity device upgrade              # check + download + apply latest release
sudo velocity device upgrade --check      # print version comparison only
sudo velocity device upgrade --binary /f  # apply a local binary (offline upgrade)
```

Rollback: `sudo velocity device rollback` restores the previous version.

## Directory layout

```
image/
├── config                          # pi-gen configuration
├── os-list-velocity.json           # rpi-imager custom repository catalogue
├── README.md                       # This file
├── scripts/
│   └── build-image.sh              # Local build helper
└── stage-velocity/                 # pi-gen custom stage
    ├── 00-install-packages/        # Runtime APT package list
    │   └── 00-packages             # Device support packages
    ├── 01-velocity-binaries/       # Go binaries
    │   ├── 00-run.sh
    ├── 03-velocity-config/         # User, service, serial, udev
    │   ├── 00-run.sh
    │   └── files/
    │       ├── 99-velocity-report.rules
    │       └── velocity-report.service          # systemd unit file (canonical source)
    ├── 04-velocity-lidar/          # LiDAR network (disabled by default)
    │   ├── 00-run.sh
    │   └── files/
    │       └── lidar-network.conf
    ├── 05-velocity-wifi/           # US Wi-Fi regulatory fallback
    │   ├── 00-run.sh
    │   └── files/
    │       └── wpa_supplicant.conf
    ├── 06-cleanup/                 # Remove build-time and developer packages
    │   └── 00-run.sh
    ├── 07-networking/              # Final NetworkManager defaults
    │   ├── 00-run.sh
    │   └── files/
    │       ├── NetworkManager.state
    │       └── velocity-wired-dhcp.nmconnection
    ├── 07-velocity-tailscale/      # Tailscale package install, masked by default
    │   └── 01-run.sh
    └── EXPORT_IMAGE
```

## Building locally

```bash
make build-image                           # full build (Docker compile + image)
make build-image SKIP_BINARIES=1           # reuse previously compiled binaries
make build-image SSH_KEY=~/.ssh/id_ed25519.pub  # install SSH key for velocity user
```

Requires Docker (Docker Desktop on macOS). The script:

1. Stages a fully static ARM64 image binary through `scripts/stage-image-binary.sh`
2. Clones [pi-gen](https://github.com/RPi-Distro/pi-gen) into `image/.pi-gen/`
3. Copies stage scripts and binaries into the pi-gen tree
4. Runs pi-gen's `build-docker.sh` to produce the image
5. Compresses the output with `xz` and generates a SHA-256 checksum

`SKIP_BINARIES=1` still verifies that the staged binary is a statically linked
Linux ELF before pi-gen can include it in an image.

The staged binary is produced by the same static build route used for Linux
release assets: Docker runs the pinned zig/musl toolchain in
`image/Dockerfile.static-build`, builds `libpcap.a` from the vendored
`third_party/libpcap` submodule, and `scripts/verify-static-elf.sh` rejects
dynamic ELF output before the image stage can consume it.

Build artifacts (`image/.pi-gen/`, `image/velocity-binaries/`, `*.img*`) are
gitignored.

## CI pipeline

The GitHub Actions workflow at `.github/workflows/build-image.yml` builds
the image on version-tag pushes or manual dispatch. Linux image/release
binaries are built through the static Docker route, the ARM64 static binary is
staged in `image/velocity-binaries/`, pi-gen consumes that staged artifact, and
the final `.img.xz` plus checksum are uploaded to the matching GitHub Release.

## Flashing

Users flash with stock Raspberry Pi Imager pointed at the custom repository:

```bash
rpi-imager --repo https://velocity.report/images/os-list.json
```

Or use any image-writing tool (`dd`, balenaEtcher) with the `.img.xz` file
downloaded from the GitHub Release.

## First-boot networking checks

The image installs a NetworkManager DHCP profile for wired LAN. If the device
boots with only `127.0.0.1`, check the physical link and NetworkManager state:

```bash
ip -br link
ip -br addr
nmcli device status
systemctl status NetworkManager --no-pager
journalctl -u NetworkManager -b --no-pager
```

If `ip -br link` shows `NO-CARRIER` for `eth0`, fix the cable, switch port,
or upstream Ethernet link before chasing DHCP. NetworkManager cannot request
an address until the Pi detects carrier.

To force a wired DHCP retry from the console:

```bash
sudo nmcli networking on
sudo nmcli device connect eth0
```

## Image size budget (phase 1)

| Component                                     | Estimated Size  |
| --------------------------------------------- | --------------- |
| Raspberry Pi OS Lite (base)                   | ~450 MB         |
| Static Go binary with embedded Typst/docs/web | ~65 MB          |
| LiDAR + system config + Tailscale package     | ~60 MB          |
| **Total (xz compressed)**                     | **~150–300 MB** |

## Design document

Full design: [deploy-rpi-imager-fork-plan.md](../docs/plans/deploy-rpi-imager-fork-plan.md)
