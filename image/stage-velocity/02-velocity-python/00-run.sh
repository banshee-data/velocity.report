#!/bin/bash -e
# 02-velocity-python/00-run.sh — Set up Python venv and install PDF generator
#
# Creates a shared venv at /opt/velocity-report/.venv/ and installs the
# PDF generator package into it.

# Copy PDF generator source into the image first (before chroot install)
install -d "${ROOTFS_DIR}/opt/velocity-report/tools/pdf-generator"
cp -r files/pdf-generator/* "${ROOTFS_DIR}/opt/velocity-report/tools/pdf-generator/"

on_chroot << 'CHEOF'
mkdir -p /opt/velocity-report/tools

# Create shared Python venv
python3 -m venv /opt/velocity-report/.venv

# Install PDF generator from vendored source
/opt/velocity-report/.venv/bin/pip install --no-cache-dir \
    /opt/velocity-report/tools/pdf-generator/

CHEOF
