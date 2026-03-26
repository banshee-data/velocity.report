#!/bin/bash -e
# 04-velocity-lidar/00-run.sh — Configure LiDAR network interface (disabled by default)
#
# Pre-configures a static IP for the LiDAR subnet. The interface is brought
# up only when LiDAR is enabled via the settings dashboard.

install -d "${ROOTFS_DIR}/etc/network/interfaces.d"
install -m 644 files/lidar-network.conf \
    "${ROOTFS_DIR}/etc/network/interfaces.d/lidar"
