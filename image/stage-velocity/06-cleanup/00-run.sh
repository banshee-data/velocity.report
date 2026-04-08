#!/bin/bash -e
# 06-cleanup/00-run.sh — Purge build-time and developer packages
#
# pi-gen's stage0 and stage2 install packages aimed at a general-purpose
# Raspberry Pi desktop/developer image. velocity.report is a headless
# appliance: it needs none of the compiler toolchain, kernel headers,
# GPIO libraries, Lua runtimes, video utilities, or interactive debugging
# tools that ship with Pi OS Lite.
#
# This step runs last in stage-velocity so all prior steps can still use
# Python pip (which depends on build-essential to compile native wheels).
# After this step, only runtime packages remain.

on_chroot << 'CHEOF'
export DEBIAN_FRONTEND=noninteractive

# -------------------------------------------------------------------
# 1. Compiler toolchain and kernel headers
#    Installed by stage0 (linux-headers) and stage2 (build-essential).
#    ~300 MB on disk.
# -------------------------------------------------------------------
apt-get purge -y \
    build-essential \
    dpkg-dev \
    g++ g++-12 \
    gcc gcc-12 \
    cpp cpp-12 \
    make \
    gdb \
    pkg-config \
    manpages-dev \
    libc-dev-bin libc-devtools libc6-dev \
    libstdc++-12-dev \
    libgcc-12-dev \
    linux-libc-dev \
    linux-headers-rpi-v8 \
    linux-headers-rpi-2712 \
    'linux-headers-*' \
    binutils binutils-aarch64-linux-gnu binutils-common libbinutils \
    fakeroot libfakeroot \
    libcrypt-dev \
    2>/dev/null || true

# -------------------------------------------------------------------
# 2. GPIO / hardware-hacking libraries
#    Installed by stage2 for maker projects. The velocity binary
#    accesses hardware directly; these Python/C libraries are unused.
#    ~30 MB on disk.
# -------------------------------------------------------------------
apt-get purge -y \
    pigpio pigpio-tools pigpiod \
    libpigpio-dev libpigpio1 libpigpiod-if-dev libpigpiod-if1 libpigpiod-if2-1 \
    python3-pigpio \
    python3-gpiozero \
    gpiod libgpiod2 python3-libgpiod \
    python3-spidev \
    python3-smbus2 \
    2>/dev/null || true

# -------------------------------------------------------------------
# 3. Lua extras — luajit not used (keep lua5.1 for raspi-config)
# -------------------------------------------------------------------
apt-get purge -y \
    luajit libluajit-5.1-2 libluajit-5.1-common \
    2>/dev/null || true

# -------------------------------------------------------------------
# 4. Video4Linux — USB camera utilities, not needed
#    ~5 MB on disk.
# -------------------------------------------------------------------
apt-get purge -y \
    v4l-utils libv4l-0 libv4l2rds0 libv4lconvert0 \
    2>/dev/null || true

# -------------------------------------------------------------------
# 5. Filesystem and network extras
#    NTFS, NFS, CIFS, SAMBA, UDisks — headless appliance uses ext4 only.
#    ~15 MB on disk.
# -------------------------------------------------------------------
apt-get purge -y \
    ntfs-3g libntfs-3g89 \
    nfs-common \
    cifs-utils \
    udisks2 libudisks2-0 \
    exfatprogs \
    2>/dev/null || true

# -------------------------------------------------------------------
# 6. Bluetooth — headless, no Bluetooth peripherals
#    (Keep ALSA — raspi-config depends on alsa-utils)
# -------------------------------------------------------------------
apt-get purge -y \
    bluez bluez-firmware pi-bluetooth libbluetooth3 \
    2>/dev/null || true

# -------------------------------------------------------------------
# 7. Man pages — useful in dev, waste of space on appliance
#    ~15 MB on disk.
# -------------------------------------------------------------------
apt-get purge -y \
    man-db manpages \
    2>/dev/null || true

# -------------------------------------------------------------------
# 8. Python dev headers and install tools
#    python3-dev/libpython3-dev only needed to compile native wheels.
#    pip/setuptools/wheel only needed during venv creation.
#    ~70 MB on disk.
# -------------------------------------------------------------------
apt-get purge -y \
    python3-dev libpython3-dev 'libpython3.*-dev' \
    python3-pip python3-pip-whl \
    python3-setuptools python3-setuptools-whl \
    python3-wheel \
    python3-lib2to3 python3-distutils \
    python3-lgpio python3-rpi-lgpio \
    2>/dev/null || true

# -------------------------------------------------------------------
# 9. X11, Wayland, Qt — headless appliance, no display
#    ~15 MB on disk.
# -------------------------------------------------------------------
apt-get purge -y \
    x11-common \
    libx11-6 libx11-data libx11-xcb1 \
    libxau6 libxcb1 libxdmcp6 libxext6 libxpm4 \
    libwayland-client0 libwayland-server0 \
    libqt5core5a \
    libjs-sphinxdoc \
    2>/dev/null || true

# -------------------------------------------------------------------
# 10. Camera stack — rpicam, libcamera, libboost, libpisp
#     No camera sensor on a traffic monitoring appliance.
#     ~60 MB on disk.
# -------------------------------------------------------------------
apt-get purge -y \
    rpicam-apps-lite rpicam-apps-core librpicam-app1 \
    'libcamera*' \
    'libpisp*' \
    'libboost*' \
    kms++-utils libkms++0 \
    2>/dev/null || true

# -------------------------------------------------------------------
# 11. Miscellaneous dev/convenience tools and desktop remnants
# -------------------------------------------------------------------
apt-get purge -y \
    ssh-import-id \
    p7zip p7zip-full \
    strace \
    ed \
    dc \
    dos2unix \
    minicom lrzsz \
    pastebinit \
    fbset \
    triggerhappy \
    rpi-keyboard-config rpi-keyboard-fw-update \
    xdg-user-dirs xkb-data \
    dconf-cli libdconf1 \
    shared-mime-info sgml-base xml-core \
    libmtp-common libmtp-runtime libmtp9 \
    ncurses-term \
    usb-modeswitch usb-modeswitch-data \
    iso-codes \
    2>/dev/null || true

# -------------------------------------------------------------------
# 12. Protect runtime packages from autoremove cascade
# -------------------------------------------------------------------
# Many packages needed at runtime were originally auto-installed by
# pi-gen's stage0–stage2.  Purging dev packages above orphans their
# dependency chains, causing autoremove to sweep out critical runtime
# libraries.  Mark them manual so they survive.
apt-mark manual \
    python3 python3-minimal python3.11 python3.11-minimal \
    libpython3.11 libpython3.11-minimal libpython3.11-stdlib \
    libpython3-stdlib python3-venv \
    python3-serial \
    python3-apt python3-debconf python-apt-common \
    nginx nginx-common \
    curl libcurl4 \
    sqlite3 libsqlite3-0 \
    openssl libssl3 libcrypt1 \
    ca-certificates \
    libpcap0.8 \
    openssh-server openssh-client openssh-sftp-server ssh \
    systemd systemd-sysv systemd-timesyncd \
    network-manager \
    isc-dhcp-client isc-dhcp-common \
    iproute2 iputils-ping net-tools \
    wpasupplicant iw rfkill wireless-regdb \
    firmware-brcm80211 raspberrypi-net-mods \
    raspi-config raspberrypi-sys-mods \
    raspi-firmware raspi-gpio \
    avahi-daemon \
    dbus dbus-daemon dbus-bin \
    polkitd policykit-1 \
    udev \
    init initramfs-tools initramfs-tools-core \
    libglib2.0-0 \
    libxml2 libpng16-16 \
    jq librsvg2-bin \
    console-setup keyboard-configuration \
    2>/dev/null || true

# -------------------------------------------------------------------
# 13. Cascade removal and cache clean
# -------------------------------------------------------------------
apt-get autoremove --purge -y 2>/dev/null || true
apt-get clean
# Do NOT remove /var/lib/apt/lists/ — pi-gen's export-image stage
# installs userconf-pi after this and needs the package index.
rm -rf /var/cache/apt/archives/*

# Remove leftover doc/man/locale bloat
rm -rf /usr/share/man/*
rm -rf /usr/share/doc/*
rm -rf /usr/share/info/*
# Keep only en_US and en_GB locales
find /usr/share/locale -mindepth 1 -maxdepth 1 \
    ! -name 'en_US' ! -name 'en_GB' ! -name 'locale.alias' \
    -exec rm -rf {} + 2>/dev/null || true

CHEOF
