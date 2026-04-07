#!/usr/bin/env bash
# dev-ssh.sh — SSH to velocity@velocity.local, refreshing the known-hosts entry
# if the connection fails (expected after each fresh image flash, because the
# host key rotates on every install).
#
# Usage:
#   ./scripts/dev-ssh.sh [-- <extra ssh args>]
#   make dev-ssh

set -euo pipefail

HOST="velocity.local"
USER="velocity"
TARGET="${USER}@${HOST}"

# Try a quiet probe first (no pty, no banner) so we don't pollute the terminal
# with a "connection refused" trace when the host key is fine.
probe_ok() {
    ssh -o BatchMode=yes \
        -o ConnectTimeout=5 \
        -o StrictHostKeyChecking=yes \
        -o LogLevel=ERROR \
        "$TARGET" true 2>/dev/null
}

refresh_host_key() {
    echo "Host key mismatch or not yet known — refreshing known_hosts for ${HOST}..."

    # Remove every existing entry for this host (IP or mDNS name)
    # ssh-keygen -R is the safe, portable way to do this.
    ssh-keygen -R "${HOST}" -f "${HOME}/.ssh/known_hosts" 2>/dev/null || true

    # Re-scan the current key
    NEW_KEY=$(ssh-keyscan -T 10 "${HOST}" 2>/dev/null)
    if [ -z "$NEW_KEY" ]; then
        echo "Error: could not reach ${HOST}. Is the Pi on the network?" >&2
        exit 1
    fi

    mkdir -p "${HOME}/.ssh"
    chmod 700 "${HOME}/.ssh"
    echo "$NEW_KEY" >> "${HOME}/.ssh/known_hosts"
    echo "Known-hosts entry updated."
}

# --- Main ---

if ! probe_ok; then
    refresh_host_key
fi

echo "Connecting to ${TARGET}..."
exec ssh "$TARGET" "$@"
