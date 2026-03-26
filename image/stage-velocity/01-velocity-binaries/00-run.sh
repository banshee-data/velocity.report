#!/bin/bash -e
# 01-velocity-binaries/00-run.sh — Install pre-built Go binaries and update script
#
# Expects CI artifacts in ${ROOTFS_DIR}/../velocity-binaries/:
#   velocity-report   (ARM64 Go binary, pcap-enabled)
#   velocity-deploy   (ARM64 Go binary)
#
# The velocity-update helper script is shipped from files/.

BINARIES_DIR="${ROOTFS_DIR}/../velocity-binaries"

install -m 755 "${BINARIES_DIR}/velocity-report" "${ROOTFS_DIR}/usr/local/bin/velocity-report"
install -m 755 "${BINARIES_DIR}/velocity-deploy" "${ROOTFS_DIR}/usr/local/bin/velocity-deploy"
install -m 755 files/velocity-update "${ROOTFS_DIR}/usr/local/bin/velocity-update"
