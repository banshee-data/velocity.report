#!/bin/bash -e
# 07-networking/00-run.sh - Finalise NetworkManager defaults
#
# Runs after 06-cleanup, which can purge and reinstall NetworkManager via APT
# cascades. Keep first-boot LAN DHCP deterministic by installing our network
# defaults only after that package churn is complete.

# Install an explicit NetworkManager DHCP profile for the primary LAN.
# NetworkManager usually creates auto-default Ethernet connections, but this
# appliance should not depend on generated runtime state for first boot.
install -d "${ROOTFS_DIR}/etc/NetworkManager/system-connections"
install -m 600 files/velocity-wired-dhcp.nmconnection \
    "${ROOTFS_DIR}/etc/NetworkManager/system-connections/velocity-wired-dhcp.nmconnection"

# stage2 disables wireless when no WPA_COUNTRY is provided to avoid radiating
# before a regulatory domain is set. 05-velocity-wifi sets the fallback country,
# so reset NetworkManager's persisted state to allow configured interfaces.
install -d "${ROOTFS_DIR}/var/lib/NetworkManager"
install -m 600 files/NetworkManager.state \
    "${ROOTFS_DIR}/var/lib/NetworkManager/NetworkManager.state"

on_chroot << 'CHEOF'
systemctl enable NetworkManager.service
systemctl disable NetworkManager-wait-online.service || true
CHEOF
