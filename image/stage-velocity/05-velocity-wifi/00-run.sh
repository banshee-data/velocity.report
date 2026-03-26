#!/bin/bash -e
# 05-velocity-wifi/00-run.sh — Set US Wi-Fi regulatory domain fallback
#
# rpi-imager's first-boot flow lets users set Wi-Fi country. If they skip
# it, the image defaults to the US regulatory domain so wireless is
# functional out of the box.

# Set CRDA regulatory domain fallback
install -d "${ROOTFS_DIR}/etc/default"
echo 'REGDOMAIN=US' > "${ROOTFS_DIR}/etc/default/crda"

# Install wpa_supplicant fallback configuration
install -m 600 files/wpa_supplicant.conf \
    "${ROOTFS_DIR}/etc/wpa_supplicant/wpa_supplicant.conf"
