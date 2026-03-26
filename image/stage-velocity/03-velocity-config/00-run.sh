#!/bin/bash -e
# 03-velocity-config/00-run.sh — Create service user, enable systemd service,
# configure serial port, and install udev rules.

on_chroot << 'CHEOF'
# Create dedicated service user (no login shell)
useradd --system --create-home --home-dir /var/lib/velocity-report \
    --shell /usr/sbin/nologin velocity || true

# Add velocity user to dialout group for serial port access
usermod -aG dialout velocity

# Ensure data directory exists with correct ownership
mkdir -p /var/lib/velocity-report
chown velocity:velocity /var/lib/velocity-report

# Create config directory and copy tuning defaults
mkdir -p /opt/velocity-report/config
CHEOF

# Install systemd service file
install -m 644 files/velocity-report.service \
    "${ROOTFS_DIR}/etc/systemd/system/velocity-report.service"

# Enable service for auto-start on boot
on_chroot << 'CHEOF'
systemctl enable velocity-report.service
CHEOF

# Install udev rules for USB-Serial radar devices
install -m 644 files/99-velocity-report.rules \
    "${ROOTFS_DIR}/etc/udev/rules.d/99-velocity-report.rules"

# Configure UART overlay — enable hardware UART and move Bluetooth to mini-UART
if [ -f "${ROOTFS_DIR}/boot/firmware/config.txt" ]; then
    # Append UART configuration if not already present
    grep -q 'enable_uart=1' "${ROOTFS_DIR}/boot/firmware/config.txt" || \
        echo 'enable_uart=1' >> "${ROOTFS_DIR}/boot/firmware/config.txt"
    grep -q 'dtoverlay=miniuart-bt' "${ROOTFS_DIR}/boot/firmware/config.txt" || \
        echo 'dtoverlay=miniuart-bt' >> "${ROOTFS_DIR}/boot/firmware/config.txt"
fi

# Remove serial console from kernel command line (frees /dev/ttyAMA0 for radar)
if [ -f "${ROOTFS_DIR}/boot/firmware/cmdline.txt" ]; then
    sed -i 's/ console=serial0,[0-9]*//g' "${ROOTFS_DIR}/boot/firmware/cmdline.txt"
fi
