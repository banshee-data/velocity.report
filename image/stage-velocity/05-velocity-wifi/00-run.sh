#!/bin/bash -e
# 05-velocity-wifi/00-run.sh — Set US Wi-Fi regulatory domain + wpa_supplicant fallback
#
# rpi-imager's first-boot flow lets users set Wi-Fi country. If they skip it,
# the image defaults to the US regulatory domain so wireless is functional out
# of the box. The wpa_supplicant fallback ships embedded in the velocity binary;
# `velocity device install wifi` writes it to /etc/wpa_supplicant/.

on_chroot << 'CHEOF'
set -e
# CRDA regulatory domain fallback.
install -d /etc/default
echo 'REGDOMAIN=US' > /etc/default/crda

/usr/local/bin/velocity device install wifi
# Fail the build loudly rather than ship a device missing the fallback.
test -s /etc/wpa_supplicant/wpa_supplicant.conf
CHEOF
