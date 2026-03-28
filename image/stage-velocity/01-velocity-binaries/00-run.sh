#!/bin/bash -e
# 01-velocity-binaries/00-run.sh — Install pre-built Go binaries
#
# Expects CI artifacts in ${BASE_DIR}/velocity-binaries/:
#   velocity-report   (ARM64 Go binary, pcap-enabled)
#   velocity-ctl      (ARM64 Go binary)
#
# BASE_DIR is exported by pi-gen and points to the pi-gen root directory.
# The velocity-update redirect stub is shipped from files/.

BINARIES_DIR="${BASE_DIR}/velocity-binaries"

install -D -m 755 "${BINARIES_DIR}/velocity-report" "${ROOTFS_DIR}/usr/local/bin/velocity-report"
install -D -m 755 "${BINARIES_DIR}/velocity-ctl" "${ROOTFS_DIR}/usr/local/bin/velocity-ctl"
install -D -m 755 files/velocity-update "${ROOTFS_DIR}/usr/local/bin/velocity-update"
