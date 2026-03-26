#!/bin/bash -e
# 01-velocity-binaries/00-run.sh — Install pre-built Go binaries
#
# Expects CI artifacts in ${ROOTFS_DIR}/../velocity-binaries/:
#   velocity-report   (ARM64 Go binary, pcap-enabled)
#   velocity-ctl      (ARM64 Go binary)
#
# The velocity-update redirect stub is shipped from files/.

BINARIES_DIR="${ROOTFS_DIR}/../velocity-binaries"

install -m 755 "${BINARIES_DIR}/velocity-report" "${ROOTFS_DIR}/usr/local/bin/velocity-report"
install -m 755 "${BINARIES_DIR}/velocity-ctl" "${ROOTFS_DIR}/usr/local/bin/velocity-ctl"
install -m 755 files/velocity-update "${ROOTFS_DIR}/usr/local/bin/velocity-update"
