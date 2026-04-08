#!/bin/bash -e
# 01-run.sh — Build minimal TeX Live tree and purge apt packages
#
# Runs after 00-packages installs the full texlive-xetex and
# texlive-latex-extra packages.  Extracts only the files the PDF
# generator actually uses into /opt/velocity-report/texlive/ (~143 MB),
# then purges the apt TeX packages (~1 GB saved).

# ------------------------------------------------------------------
# 1. Copy build script and dependencies into the rootfs
# ------------------------------------------------------------------
install -D -m 755 files/build-minimal-texlive.sh \
    "${ROOTFS_DIR}/tmp/build-minimal-texlive.sh"
install -D -m 644 files/dependency-manifest.txt \
    "${ROOTFS_DIR}/tmp/dependency-manifest.txt"
install -D -m 644 files/velocity-report.ini \
    "${ROOTFS_DIR}/tmp/velocity-report.ini"

# ------------------------------------------------------------------
# 2. Build the minimal tree inside the chroot
# ------------------------------------------------------------------
on_chroot << 'CHEOF'
TEXLIVE_ROOT=/usr/share/texlive \
MANIFEST=/tmp/dependency-manifest.txt \
INI_FILE=/tmp/velocity-report.ini \
OUTPUT_DIR=/opt/velocity-report/texlive \
COPY_SHARED_LIBS=1 \
    /tmp/build-minimal-texlive.sh

rm -f /tmp/build-minimal-texlive.sh \
      /tmp/dependency-manifest.txt \
      /tmp/velocity-report.ini
CHEOF

# ------------------------------------------------------------------
# 3. Purge the apt TeX packages — the minimal tree is self-contained
# ------------------------------------------------------------------
on_chroot << 'CHEOF'
DEBIAN_FRONTEND=noninteractive apt-get purge -y \
    'texlive*' 'preview-latex*' 'cm-super*' 'tex-gyre*' 'tex-common*' \
    'lmodern' 'fonts-lmodern' 'tipa' 't1utils' 2>/dev/null || true
apt-get autoremove --purge -y 2>/dev/null || true
apt-get clean
CHEOF
