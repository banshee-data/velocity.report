#!/bin/bash -e
# 03-velocity-config/00-run.sh — Create service user, enable systemd service,
# configure serial port, and install udev rules.

on_chroot << 'CHEOF'
# Create the velocity system account — a non-login service user that owns
# the data directory and runs the systemd service.  This account is immune
# to Raspberry Pi Imager's OS Customisation user-rename because it is a
# system account, not FIRST_USER_NAME.
if ! id velocity >/dev/null 2>&1; then
    useradd --system --home-dir /var/lib/velocity-report --shell /usr/sbin/nologin velocity
fi
usermod -aG dialout velocity

# Ensure data directory exists with correct ownership
mkdir -p /var/lib/velocity-report
chown velocity:velocity /var/lib/velocity-report

# Grant the interactive login user passwordless sudo for service management.
# FIRST_USER_NAME is set in image/config (default: pi).  Hardcoded here
# because the on_chroot heredoc is single-quoted to preserve sudoers
# backslash continuations.
#
# Commands granted to `pi` (enumerated verbs, no wildcards beyond the verb):
#   getent shadow pi          — MOTD default-password check
#   systemctl <verb> service  — shell aliases (start/stop/restart/status)
#   velocity device <verb>    — installed-version lifecycle (runs as root)
#   velocity data migrate ... — operator-facing schema migrations (up/status/version)
#
# Argument-bearing upgrade/migration variants (device upgrade --binary,
# data migrate down/force/...) deliberately require an interactive password.
#
# Commands granted to the `velocity` service user:
#   velocity device tailscale {install,enable,disable}-tailscaled — literal
#       argv that lets the Go server install the pinned static payload and
#       unmask/start/stop it when the operator toggles Tailscale in the web UI.
cat > /etc/sudoers.d/020_velocity-nopasswd <<'SUDOEOF'
pi ALL=(root) NOPASSWD: \
    /usr/bin/getent shadow pi, \
    /usr/bin/systemctl start velocity-report.service, \
    /usr/bin/systemctl stop velocity-report.service, \
    /usr/bin/systemctl restart velocity-report.service, \
    /usr/bin/systemctl status velocity-report.service, \
    /usr/bin/systemctl is-active velocity-report.service, \
    /usr/local/bin/velocity device check, \
    /usr/local/bin/velocity device upgrade, \
    /usr/local/bin/velocity device upgrade --check, \
    /usr/local/bin/velocity device rollback, \
    /usr/local/bin/velocity device backup, \
    /usr/local/bin/velocity data migrate up, \
    /usr/local/bin/velocity data migrate status, \
    /usr/local/bin/velocity data migrate version

velocity ALL=(root) NOPASSWD: \
    /usr/local/bin/velocity device tailscale install, \
    /usr/local/bin/velocity device tailscale enable-tailscaled, \
    /usr/local/bin/velocity device tailscale disable-tailscaled
SUDOEOF
chmod 440 /etc/sudoers.d/020_velocity-nopasswd

# Drop the stock pi-gen fragment that grants the login user full passwordless
# root; the scoped allowlist above is the intended appliance sudo surface.
rm -f /etc/sudoers.d/010_pi-nopasswd

# Add login user to velocity group for read access to sensor data
usermod -aG velocity pi
CHEOF

# Tuning defaults ship embedded in the velocity binary (the server falls back to
# them when no --config file is present), so no tuning.defaults.json is staged
# on disk. Operators can still override via the existing --config flag.

# Install reference data (maths, structures, experiments)
if [ -d files/data ]; then
    cp -r files/data "${ROOTFS_DIR}/opt/velocity-report/data"
fi

# Install built documentation site (velocity.report public pages)
if [ -d files/public_html ]; then
    cp -r files/public_html "${ROOTFS_DIR}/opt/velocity-report/public_html"
fi

# Install systemd service file.  The Go server binds :80 directly via
# CAP_NET_BIND_SERVICE (set in the unit); there is no nginx and no TLS
# termination layer.  HTTPS is an opt-in via Tailscale Serve — see
# docs/platform/operations/tailscale-serve.md.
install -m 644 files/velocity-report.service \
    "${ROOTFS_DIR}/etc/systemd/system/velocity-report.service"

# Enable the service for auto-start on boot.  The service starts with
# the radar active on /dev/ttySC1 (Waveshare SC16IS752 HAT channel B).
on_chroot << 'CHEOF'
systemctl enable velocity-report.service
CHEOF

# Install udev rules for USB-Serial radar devices (embedded in the binary).
on_chroot << 'CHEOF'
set -e
/usr/local/bin/velocity device install udev
test -s /etc/udev/rules.d/99-velocity-report.rules
CHEOF

# Install shell aliases for service management (velocity-log, velocity-status, etc.)
install -m 644 files/velocity-aliases.sh \
    "${ROOTFS_DIR}/etc/profile.d/velocity-aliases.sh"

# Install login MOTD banner (warns about default password, shows help commands)
install -m 644 files/velocity-motd.sh \
    "${ROOTFS_DIR}/etc/profile.d/velocity-motd.sh"

# Install build metadata for MOTD and on-device diagnostics.
# Stamped by build-image.sh; sourced by velocity-motd.sh at login.
if [ -f files/velocity-report-build ]; then
    install -m 644 files/velocity-report-build \
        "${ROOTFS_DIR}/etc/velocity-report-build"
fi

# Suppress the default Debian MOTD so ours is the only one shown.
: > "${ROOTFS_DIR}/etc/motd"

# Install SSH authorized_keys for login user (if provided via --ssh-key)
if [ -f files/authorized_keys ]; then
    install -d -m 700 "${ROOTFS_DIR}/home/${FIRST_USER_NAME}/.ssh"
    install -m 600 files/authorized_keys \
        "${ROOTFS_DIR}/home/${FIRST_USER_NAME}/.ssh/authorized_keys"
    on_chroot <<CHEOF
chown -R "${FIRST_USER_NAME}:${FIRST_USER_NAME}" "/home/${FIRST_USER_NAME}/.ssh"
CHEOF
fi

# Configure UART and SPI overlays for Waveshare RS232/485 HAT (SC16IS752)
if [ -f "${ROOTFS_DIR}/boot/firmware/config.txt" ]; then
    # Enable hardware UART and move Bluetooth to mini-UART
    grep -q 'enable_uart=1' "${ROOTFS_DIR}/boot/firmware/config.txt" || \
        echo 'enable_uart=1' >> "${ROOTFS_DIR}/boot/firmware/config.txt"
    grep -q 'dtoverlay=miniuart-bt' "${ROOTFS_DIR}/boot/firmware/config.txt" || \
        echo 'dtoverlay=miniuart-bt' >> "${ROOTFS_DIR}/boot/firmware/config.txt"
    # Enable SPI bus and SC16IS752 dual-UART overlay (creates /dev/ttySC0, /dev/ttySC1)
    grep -q 'dtparam=spi=on' "${ROOTFS_DIR}/boot/firmware/config.txt" || \
        echo 'dtparam=spi=on' >> "${ROOTFS_DIR}/boot/firmware/config.txt"
    grep -q 'dtoverlay=sc16is752-spi1,int_pin=24' "${ROOTFS_DIR}/boot/firmware/config.txt" || \
        echo 'dtoverlay=sc16is752-spi1,int_pin=24' >> "${ROOTFS_DIR}/boot/firmware/config.txt"
fi

# Remove serial console from kernel command line (frees /dev/ttyAMA0 for radar)
if [ -f "${ROOTFS_DIR}/boot/firmware/cmdline.txt" ]; then
    sed -i 's/ console=serial0,[0-9]*//g' "${ROOTFS_DIR}/boot/firmware/cmdline.txt"
fi

# --- First-boot wizard suppression ------------------------------------------
# DISABLE_FIRST_BOOT_USER_RENAME=1 in image/config tells pi-gen to skip the
# rename-user step during export-image.  As a belt-and-suspenders measure, we
# also cancel any pending rename and remove the getty override that the
# userconf-pi package may install.  This runs before export-image, so
# export-image's own cleanup (removing piwiz.desktop) still applies.
on_chroot << 'CHEOF'
cancel-rename pi 2>/dev/null || true
CHEOF
rm -f "${ROOTFS_DIR}/etc/systemd/system/getty@tty1.service.d/userconf.conf"
