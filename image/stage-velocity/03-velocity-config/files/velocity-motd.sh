#!/bin/bash
# /etc/profile.d/velocity-motd.sh — Login banner for velocity.report appliance
#
# Shows a warning when the default password is still active.
# Shows a welcome banner with helpful commands once the password is changed.
# Both banners display the image build version, time, and git SHA.

# Only show on interactive terminals.
[ -t 0 ] || return 0

DEFAULT_PASS="report"

# Build metadata stamped at image creation time.
BUILD_INFO_FILE="/etc/velocity-report-build"
if [ -f "$BUILD_INFO_FILE" ]; then
    # shellcheck source=/dev/null
    . "$BUILD_INFO_FILE"
fi
VR_VERSION="${VR_VERSION:-unknown}"
VR_BUILD_TIME="${VR_BUILD_TIME:-unknown}"
VR_GIT_SHA="${VR_GIT_SHA:-unknown}"

# --- Check whether the default password is still in use ----------------------
#
# Read the stored hash from shadow via sudo and compare against the
# default password.  The login user has a NOPASSWD sudoers entry
# for getent (installed by stage-velocity/03-velocity-config).
#
# If sudo or getent fails, assume the password is STILL default and
# show the warning.  Fail towards caution, not silence.

password_is_default() {
    local stored
    stored=$(sudo -n getent shadow "$USER" 2>/dev/null | cut -d: -f2)
    [ -z "$stored" ] && return 0   # Cannot verify — assume default

    python3 -c "
import crypt, sys
stored = sys.argv[1]
result = crypt.crypt(sys.argv[2], stored)
sys.exit(0 if result == stored else 1)
" "$stored" "$DEFAULT_PASS" 2>/dev/null
}

# --- Banners -----------------------------------------------------------------

warning_banner() {
    cat << EOF

 ╔════════════════════════════════════════════════════════════════╗
 ║                                                                ║
 ║  ██     ██  █████  ██████  ███    ██ ██ ███    ██  ██████  ██  ║
 ║  ██     ██ ██   ██ ██   ██ ████   ██ ██ ████   ██ ██       ██  ║
 ║  ██  █  ██ ███████ ██████  ██ ██  ██ ██ ██ ██  ██ ██   ███ ██  ║
 ║  ██ ███ ██ ██   ██ ██   ██ ██  ██ ██ ██ ██  ██ ██ ██    ██     ║
 ║   ███ ███  ██   ██ ██   ██ ██   ████ ██ ██   ████  ██████  ██  ║
 ║                                                                ║
 ║  This device is still using the default password.              ║
 ║  Anyone on your network can log in and muck around.            ║
 ║  That is the sort of arrangement that ends badly.              ║
 ║                                                                ║
 ║  Please change the password now, type:                         ║
 ║                                                                ║
 ║      passwd                                                    ║
 ║                                                                ║
 ╚════════════════════════════════════════════════════════════════╝

  Image: v${VR_VERSION}  Built: ${VR_BUILD_TIME}  SHA: ${VR_GIT_SHA}

EOF
}

welcome_banner() {
    cat << EOF

  ┌──────────────────────────────────────────────────────────┐
  │                                                          │
  │  █ █ ██▀ █  ▄▀▄ ▄▀▀ ▀ ▄█▄ ▀▄▀   █▀▄ ██▀ █▀▄ ▄▀▄ █▀▄ ▄█▄  │
  │  ▀▄▀ █▄▄ █▄ ▀▄▀ ▀▄▄ █  █▄  █  ▄ █▀▄ █▄▄ █▀  ▀▄▀ █▀▄  █▄  │
  │                                                          │
  │              measure traffic, not identity               │
  │                                                          │
  └──────────────────────────────────────────────────────────┘

    v${VR_VERSION}  SHA: ${VR_GIT_SHA}  Built: ${VR_BUILD_TIME}

  Dashboard:  http://velocity.local

  Useful commands:
    velocity-status           Is the service running?
    velocity-log              Follow the live service log
    velocity-bounce           Restart the service
    velocity-report version   Version and build info
    sudo velocity-ctl         Device management

  The service starts automatically at boot.
  Connect a sensor and the data starts flowing.

EOF
}

# --- Main --------------------------------------------------------------------

if password_is_default; then
    warning_banner
else
    welcome_banner
fi
