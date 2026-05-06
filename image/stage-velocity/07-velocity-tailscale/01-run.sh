#!/bin/bash -e
# 07-velocity-tailscale/01-run.sh — Install Tailscale and leave it
# masked.  All enable/disable/login flow happens at runtime through the
# velocity.report web UI (see internal/tailscale and the Settings page).
#
# Why mask-by-default:
#   - Privacy tenet: the device must not phone home to an external
#     service unless the operator explicitly opts in.
#   - The image is published publicly, so we cannot ship credentials
#     baked in.  The operator opts in via the web UI; the Go server
#     unmasks tailscaled, kicks off interactive login, and applies the
#     device policy (Tailscale SSH on, tailscale serve publishing the
#     local HTTP server on :443) once the node is up on the tailnet.

on_chroot << 'CHEOF'
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive

# Tailscale apt repo — fetched for the chroot's actual Debian codename
# rather than a hardcoded "bookworm".  The pi-gen base may move
# (bullseye → bookworm → trixie → ...) and the wrong repo would
# silently 404 on `apt update` later.
codename=""
if [ -r /etc/os-release ]; then
    # shellcheck source=/dev/null
    . /etc/os-release
    codename="${VERSION_CODENAME:-}"
fi
if [ -z "$codename" ] && command -v lsb_release >/dev/null 2>&1; then
    codename="$(lsb_release -cs 2>/dev/null || true)"
fi
if [ -z "$codename" ]; then
    echo "stage-07 tailscale: cannot detect Debian codename from /etc/os-release or lsb_release" >&2
    exit 1
fi

case "$codename" in
    bullseye|bookworm|trixie) ;;
    *)
        echo "stage-07 tailscale: unsupported Debian codename '$codename' — Tailscale apt repo may not exist" >&2
        exit 1
        ;;
esac

install -d -m 755 /usr/share/keyrings
curl -fsSL "https://pkgs.tailscale.com/stable/debian/${codename}.noarmor.gpg" \
    -o /usr/share/keyrings/tailscale-archive-keyring.gpg
curl -fsSL "https://pkgs.tailscale.com/stable/debian/${codename}.tailscale-keyring.list" \
    -o /etc/apt/sources.list.d/tailscale.list

apt-get update -qq
apt-get install -y --no-install-recommends tailscale

# Mask tailscaled so it does not start on boot.  velocity-ctl tailscale
# enable-tailscaled (invoked by the Go server via sudo when the operator
# toggles Tailscale on) will unmask, enable, start, and configure the
# socket operator so the velocity service user can drive it.
systemctl mask tailscaled.service
CHEOF
