#!/bin/bash
# /etc/profile.d/velocity-motd.sh — Login banner for velocity.report appliance
#
# Shows a warning when the default password is still active.
# Shows a welcome banner with helpful commands once the password is changed.
# Both banners display the image build version, time, and git SHA.

# Only show on interactive terminals.
[ -t 0 ] || return 0

DEFAULT_PASS="report"

# Resolve version/build info from the INSTALLED BINARY so the banner stays
# correct after `velocity device upgrade` swaps it.  The /etc/velocity-report-build
# file is only stamped at image-build time, so reading it would show the
# original image version forever — stale the moment the device is upgraded.
# Querying the binary makes the binary's own ldflags the single source of truth.
# Fall back to the image stamp, then "unknown", if the binary can't be queried.
VR_BIN="${VR_BIN:-/usr/local/bin/velocity-report}"
VR_VERSION=""; VR_GIT_SHA=""; VR_BUILD_TIME=""
if [ -x "$VR_BIN" ]; then
    # `velocity-report --version` prints (internal/version.Print):
    #   velocity-report  v<version>
    #    git sha:  <sha>
    #      built:  <iso8601>
    vr_info="$("$VR_BIN" --version 2>/dev/null)"
    if [ -n "$vr_info" ]; then
        VR_VERSION="$(printf '%s\n' "$vr_info" | awk 'NR==1{print $2}')"
        VR_VERSION="${VR_VERSION#v}"
        VR_GIT_SHA="$(printf '%s\n' "$vr_info" | awk '/git sha:/{print $NF}')"
        VR_BUILD_TIME="$(printf '%s\n' "$vr_info" | awk '/built:/{print $NF}')"
    fi
fi
if [ -z "$VR_VERSION" ] && [ -f /etc/velocity-report-build ]; then
    # shellcheck source=/dev/null
    . /etc/velocity-report-build
fi
VR_VERSION="${VR_VERSION:-unknown}"
VR_BUILD_TIME="${VR_BUILD_TIME:-unknown}"
VR_GIT_SHA="${VR_GIT_SHA:-unknown}"
VR_GIT_SHA_SHORT="${VR_GIT_SHA:0:7}"

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

 v${VR_VERSION}  Built: ${VR_BUILD_TIME}  SHA: ${VR_GIT_SHA_SHORT}

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

 v${VR_VERSION}  Built: ${VR_BUILD_TIME}  SHA: ${VR_GIT_SHA_SHORT}

  Dashboard:  http://velocity.local

  Useful commands:
    velocity-status           Is the service running?
    velocity-log              Follow the live service log
    velocity-bounce           Restart the service
    velocity version          Version and build info
    sudo velocity device      Device management (upgrade/rollback/backup)

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
