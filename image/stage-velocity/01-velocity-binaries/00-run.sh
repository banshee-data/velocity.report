#!/bin/bash -e
# 01-velocity-binaries/00-run.sh — Install the single multi-call velocity binary
#
# Expects CI artifacts in ${BASE_DIR}/velocity-binaries/:
#   velocity   (ARM64 Go binary, pcap-enabled, multi-call dispatcher)
#   VERSION    (the version string, used to name the on-disk versions/<v>/ dir)
#
# The binary is installed into the versioned layout that
# `velocity device upgrade` swaps atomically at runtime:
#
#   /opt/velocity-report/versions/<v>/velocity     real binary
#   /opt/velocity-report/current  -> versions/<v>   active version (symlink)
#   /usr/local/bin/velocity        -> current/velocity   canonical entry point
#   /usr/local/bin/velocity-report -> current/velocity   server-compat alias
#   /usr/local/bin/velocity-ctl    -> current/velocity   deprecation shim
#
# velocity-report and velocity-ctl are argv[0] aliases resolved by the
# dispatcher; velocity-ctl additionally prints a deprecation warning and is
# removed next release. BASE_DIR is exported by pi-gen.

BINARIES_DIR="${BASE_DIR}/velocity-binaries"
VERSION="$(cat "${BINARIES_DIR}/VERSION")"

if [ -z "${VERSION}" ]; then
    echo "stage-01 velocity-binaries: missing version string in ${BINARIES_DIR}/VERSION" >&2
    exit 1
fi

# Install the real binary under its versioned directory.
install -D -m 755 "${BINARIES_DIR}/velocity" \
    "${ROOTFS_DIR}/opt/velocity-report/versions/${VERSION}/velocity"

# current -> versions/<v> (relative target, resolved within /opt/velocity-report).
ln -sfn "versions/${VERSION}" "${ROOTFS_DIR}/opt/velocity-report/current"

# Entry points in /usr/local/bin all resolve to the active binary.
install -d -m 755 "${ROOTFS_DIR}/usr/local/bin"
ln -sfn /opt/velocity-report/current/velocity "${ROOTFS_DIR}/usr/local/bin/velocity"
ln -sfn /opt/velocity-report/current/velocity "${ROOTFS_DIR}/usr/local/bin/velocity-report"
ln -sfn /opt/velocity-report/current/velocity "${ROOTFS_DIR}/usr/local/bin/velocity-ctl"
