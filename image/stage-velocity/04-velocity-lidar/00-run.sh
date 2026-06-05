#!/bin/bash -e
# 04-velocity-lidar/00-run.sh — Configure LiDAR network interface (disabled by default)
#
# The static IP for the LiDAR subnet (192.168.100.x) ships embedded in the
# velocity binary; `velocity device install network` writes it to
# /etc/network/interfaces.d/lidar. The interface is brought up only when LiDAR
# is enabled via the settings dashboard. The binary is installed in stage 01,
# so it is on PATH in the chroot here.

on_chroot << 'CHEOF'
set -e
/usr/local/bin/velocity device install network
# Fail the build loudly rather than ship a device missing the config.
test -s /etc/network/interfaces.d/lidar
CHEOF
