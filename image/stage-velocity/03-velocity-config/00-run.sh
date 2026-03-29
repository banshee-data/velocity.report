#!/bin/bash -e
# 03-velocity-config/00-run.sh — Create service user, enable systemd service,
# configure serial port, and install udev rules.

on_chroot << 'CHEOF'
# The login user 'velocity' is created by pi-gen stage2 (FIRST_USER_NAME).
# We reuse it as the service user so there is one account for both
# interactive login and the systemd service.

# Add velocity user to dialout group for serial port access
usermod -aG dialout velocity

# Ensure data directory exists with correct ownership
mkdir -p /var/lib/velocity-report
chown velocity:velocity /var/lib/velocity-report

# Create config directory and copy tuning defaults
mkdir -p /opt/velocity-report/config
CHEOF

# Install tuning defaults
install -m 644 files/config/tuning.defaults.json \
    "${ROOTFS_DIR}/opt/velocity-report/config/tuning.defaults.json"

# Install project documentation
if [ -d files/docs ]; then
    cp -r files/docs "${ROOTFS_DIR}/opt/velocity-report/docs"
fi

# Install systemd service file
install -m 644 files/velocity-report.service \
    "${ROOTFS_DIR}/etc/systemd/system/velocity-report.service"

# Enable the service for auto-start on boot.  The service starts with
# the radar active on /dev/ttySC1 (Waveshare SC16IS752 HAT channel B).
on_chroot << 'CHEOF'
systemctl enable velocity-report.service
CHEOF

# Install udev rules for USB-Serial radar devices
install -m 644 files/99-velocity-report.rules \
    "${ROOTFS_DIR}/etc/udev/rules.d/99-velocity-report.rules"

# Install shell aliases for service management (velocity-log, velocity-status, etc.)
install -m 644 files/velocity-aliases.sh \
    "${ROOTFS_DIR}/etc/profile.d/velocity-aliases.sh"

# Install login MOTD banner (warns about default password, shows help commands)
install -m 644 files/velocity-motd.sh \
    "${ROOTFS_DIR}/etc/profile.d/velocity-motd.sh"

# Suppress the default Debian MOTD so ours is the only one shown.
: > "${ROOTFS_DIR}/etc/motd"

# Install SSH authorized_keys for velocity user (if provided via --ssh-key)
if [ -f files/authorized_keys ]; then
    install -d -m 700 "${ROOTFS_DIR}/home/velocity/.ssh"
    install -m 600 files/authorized_keys \
        "${ROOTFS_DIR}/home/velocity/.ssh/authorized_keys"
    on_chroot << 'CHEOF'
chown -R velocity:velocity /home/velocity/.ssh
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
cancel-rename velocity 2>/dev/null || true
CHEOF
rm -f "${ROOTFS_DIR}/etc/systemd/system/getty@tty1.service.d/userconf.conf"
