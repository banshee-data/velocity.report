#!/usr/bin/env bash
# Run the release ARM64 binary under a real ARM64 Linux kernel and validate
# systemd privileges, live libpcap capture, API/database persistence, upgrade,
# and rollback. qemu-user is not sufficient because it cannot emulate the
# SIOCETHTOOL calls used by libpcap.
#
# Two targets run the same checks:
#
#   (default)  boot a pinned Debian 12 ARM64 cloud image under qemu and run
#              them in the guest over ssh. Hardware-independent: works on any
#              host, but an x86 host emulates every instruction (TCG).
#   --host     run them on this machine, which must be an ARM64 Linux host
#              with systemd and passwordless sudo, such as a hosted ARM64 CI
#              runner. It installs a system user, a systemd unit, files under
#              /opt/velocity-report and /usr/local/bin, and binds :80, so it
#              refuses to run outside GitHub Actions unless
#              VELOCITY_HOST_GATE_ALLOW=1, and refuses any host that already
#              has a velocity-report install.
#
# The host target does not prove what the guest's clean image does: a stock
# Debian 12 userland and systemd, and a machine with nothing else installed.
# Static build CI runs the host target on pull requests and the guest on
# main pushes and manual runs; the release workflow runs the guest.

set -euo pipefail

TARGET=vm
if [[ "${1:-}" == "--host" ]]; then
    TARGET=host
    shift
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BIN="${1:-}"
if [[ -z "$BIN" || ! -f "$BIN" ]]; then
    echo "usage: $0 [--host] <static-linux-arm64-velocity-binary>" >&2
    exit 2
fi
BIN="$(cd "$(dirname "$BIN")" && pwd)/$(basename "$BIN")"

wait_api() {
    for _ in $(seq 1 90); do
        if curl -fsS "$API"/api/version >/dev/null 2>&1; then return 0; fi
        sleep 1
    done
    echo "API did not become ready" >&2
    return 1
}
assert_radar_capabilities() {
    curl -fsS "$API"/api/capabilities | python3 -c '
import json
import sys

caps = json.load(sys.stdin)
radar = caps.get("radar", {}).get("default", {})
if radar.get("enabled") is True and radar.get("status") == "receiving":
    raise SystemExit(0)
print(f"unexpected /api/capabilities response: {caps}", file=sys.stderr)
raise SystemExit(1)
'
}

# Each target defines the same three verbs and the API base URL:
#   target_put SRC DST    copy a file to the target (executable)
#   target_sh CMD         run a shell command as root on the target
#   target_sh_retry CMD   the same, retried while the transport settles
# The VM target's globals (SSH, QEMU_PID, the cleanup trap) are set in
# setup_vm so the host target never needs qemu.
setup_vm() {
    for tool in qemu-system-aarch64 qemu-img ssh scp ssh-keygen curl sha512sum genisoimage python3; do
        command -v "$tool" >/dev/null || { echo "error: missing $tool" >&2; exit 1; }
    done

    if [[ -f /usr/share/edk2/aarch64/QEMU_EFI.fd ]]; then
        EFI_FIRMWARE=/usr/share/edk2/aarch64/QEMU_EFI.fd
    elif [[ -f /usr/share/AAVMF/AAVMF_CODE.fd ]]; then
        EFI_FIRMWARE=/usr/share/AAVMF/AAVMF_CODE.fd
    else
        echo "error: ARM64 UEFI firmware not found (install qemu-efi-aarch64)" >&2
        exit 1
    fi

    VM_ROOT="${VM_ROOT:-$REPO_ROOT/build/static-arm64-vm}"
    CACHE_DIR="$VM_ROOT/cache"
    RUN_DIR="$VM_ROOT/run"
    mkdir -p "$CACHE_DIR"
    rm -rf "$RUN_DIR"
    mkdir -p "$RUN_DIR"

    DEBIAN_BUILD=20260806-2562
    BASE_NAME="debian-12-genericcloud-arm64-${DEBIAN_BUILD}.qcow2"
    BASE_URL="https://cloud.debian.org/images/cloud/bookworm/${DEBIAN_BUILD}/${BASE_NAME}"
    BASE_SHA512=0ddd6ae10dad18535fc8a8167065e78565a04b721cae3e946a3ca4fda2ce54ac7a7546a09b9c0bca6bd101db3ab950056da19dc452113631dc7bbce7c96a404f
    BASE_IMAGE="$CACHE_DIR/$BASE_NAME"

    if [[ ! -f "$BASE_IMAGE" ]]; then
        echo "==> downloading pinned Debian ARM64 cloud image"
        curl -fL --retry 3 -o "$BASE_IMAGE.part" "$BASE_URL"
        mv "$BASE_IMAGE.part" "$BASE_IMAGE"
    fi
    echo "$BASE_SHA512  $BASE_IMAGE" | sha512sum -c -

    SSH_KEY="$RUN_DIR/id_ed25519"
    ssh-keygen -q -t ed25519 -N '' -f "$SSH_KEY"
    PUB_KEY="$(cat "$SSH_KEY.pub")"
    cat > "$RUN_DIR/user-data" <<EOF
#cloud-config
disable_root: false
ssh_pwauth: false
users:
  - name: root
    lock_passwd: true
    ssh_authorized_keys:
      - $PUB_KEY
EOF
    cat > "$RUN_DIR/meta-data" <<'EOF'
instance-id: velocity-static-arm64-e2e
local-hostname: velocity-arm64-vm
EOF
    genisoimage -quiet -output "$RUN_DIR/seed.iso" -volid cidata -joliet -rock \
        "$RUN_DIR/user-data" "$RUN_DIR/meta-data"

    qemu-img create -q -f qcow2 -F qcow2 -b "$BASE_IMAGE" "$RUN_DIR/root.qcow2" 8G
    QEMU_LOG="$RUN_DIR/qemu.log"
    # A named CPU, not -cpu max: edk2 2025.11 (ubuntu-26.04) hangs after its banner
    # under QEMU 10.2's max model. See docs/platform/operations/arm64-vm-gate-ubuntu-2604.md.
    qemu-system-aarch64 \
        -machine virt,accel=tcg -cpu neoverse-n1 -smp 2 -m 1536 \
        -bios "$EFI_FIRMWARE" \
        -drive if=virtio,format=qcow2,file="$RUN_DIR/root.qcow2" \
        -drive if=virtio,format=raw,readonly=on,file="$RUN_DIR/seed.iso" \
        -device virtio-net-pci,netdev=net0 \
        -netdev user,id=net0,hostfwd=tcp:127.0.0.1:2222-:22,hostfwd=tcp:127.0.0.1:18082-:80 \
        -nographic -monitor none >"$QEMU_LOG" 2>&1 &
    QEMU_PID=$!
    cleanup() {
        status=$?
        if [[ "$status" -ne 0 ]]; then
            echo "ARM64 VM validation failed; QEMU log follows" >&2
            tail -200 "$QEMU_LOG" >&2 || true
        fi
        kill "$QEMU_PID" 2>/dev/null || true
        wait "$QEMU_PID" 2>/dev/null || true
        return "$status"
    }
    trap cleanup EXIT

    SSH=(ssh -i "$SSH_KEY" -p 2222 -o BatchMode=yes -o ConnectTimeout=10 -o ServerAliveInterval=5 -o ServerAliveCountMax=6 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null root@127.0.0.1)
    SCP=(scp -i "$SSH_KEY" -P 2222 -o BatchMode=yes -o ConnectTimeout=10 -o ServerAliveInterval=5 -o ServerAliveCountMax=6 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null)
    retry_transport() {
        local attempt
        for attempt in $(seq 1 12); do
            if "$@"; then return 0; fi
            if ! kill -0 "$QEMU_PID" 2>/dev/null; then
                echo "QEMU exited while waiting for guest transport" >&2
                return 1
            fi
            echo "guest transport attempt $attempt failed; retrying" >&2
            sleep 2
        done
        return 1
    }

    echo "==> waiting for ARM64 VM ssh"
    ready=0
    for _ in $(seq 1 180); do
        if "${SSH[@]}" true >/dev/null 2>&1; then ready=1; break; fi
        sleep 2
    done
    if [[ "$ready" != 1 ]]; then
        echo "VM did not become ready; log follows" >&2
        tail -200 "$QEMU_LOG" >&2
        exit 1
    fi

    target_sh() { "${SSH[@]}" "$1"; }
    target_sh_retry() { retry_transport "${SSH[@]}" "$1"; }
    target_put() { retry_transport "${SCP[@]}" "$1" "root@127.0.0.1:$2"; }
    API=http://127.0.0.1:18082
}

setup_host() {
    if [[ "$(uname -s)" != Linux || "$(uname -m)" != aarch64 ]]; then
        echo "error: --host needs an ARM64 Linux host (this is $(uname -s) $(uname -m))" >&2
        exit 1
    fi
    if [[ "${GITHUB_ACTIONS:-}" != true && "${VELOCITY_HOST_GATE_ALLOW:-}" != 1 ]]; then
        echo "error: --host installs a system service on this machine; set VELOCITY_HOST_GATE_ALLOW=1 to allow it outside GitHub Actions" >&2
        exit 1
    fi
    for tool in systemctl systemd-run sudo curl python3; do
        command -v "$tool" >/dev/null || { echo "error: missing $tool" >&2; exit 1; }
    done
    if [[ -e /opt/velocity-report || -e /etc/systemd/system/velocity-report.service ]]; then
        echo "error: this host already has a velocity-report install; --host would overwrite it" >&2
        exit 1
    fi
    sudo -n true
    test "$(cat /proc/1/comm)" = systemd
}

if [[ "$TARGET" == host ]]; then
    setup_host
    target_sh() { sudo -n bash -c "$1"; }
    target_sh_retry() { target_sh "$1"; }
    target_put() { sudo -n install -m 0755 "$1" "$2"; }
    API=http://127.0.0.1:80
    cleanup() {
        status=$?
        if [[ "$status" -ne 0 ]]; then
            echo "ARM64 host validation failed; service journal follows" >&2
            sudo -n systemctl status --no-pager velocity-report.service >&2 || true
            sudo -n journalctl --no-pager -u velocity-report.service -n 200 >&2 || true
        fi
        return "$status"
    }
    trap cleanup EXIT
    echo "==> ARM64 host: $(uname -r), $(. /etc/os-release && echo "$PRETTY_NAME"), systemd $(systemctl --version | awk 'NR==1{print $2}')"
else
    setup_vm
fi

target_put "$BIN" /tmp/velocity-candidate
target_put "$REPO_ROOT/image/stage-velocity/03-velocity-config/files/velocity-report.service" \
    /tmp/velocity-report.service

target_sh_retry 'set -eu
id velocity >/dev/null 2>&1 || useradd --system --home /var/lib/velocity-report --shell /usr/sbin/nologin velocity
install -d -o velocity -g velocity /var/lib/velocity-report /var/lib/velocity-report/backups
base_version=$(/tmp/velocity-candidate version | awk "/^velocity/{sub(/^v/, \"\", \$2); print \$2; exit}")
test -n "$base_version"
install -d "/opt/velocity-report/versions/$base_version"
install -m 0755 /tmp/velocity-candidate "/opt/velocity-report/versions/$base_version/velocity"
ln -sfn "versions/$base_version" /opt/velocity-report/current
ln -sfn /opt/velocity-report/current/velocity /usr/local/bin/velocity
ln -sfn /opt/velocity-report/current/velocity /usr/local/bin/velocity-report
install -m 0644 /tmp/velocity-report.service /etc/systemd/system/velocity-report.service
install -d /etc/systemd/system/velocity-report.service.d
printf "%s\n" "[Service]" "ExecStart=" "ExecStart=/usr/local/bin/velocity-report --listen :80 --disable-radar --db-path /var/lib/velocity-report/sensor_data.db" > /etc/systemd/system/velocity-report.service.d/vm.conf
systemctl daemon-reload
systemctl enable --now velocity-report.service
systemctl is-active --quiet velocity-report.service
test "$(uname -m)" = aarch64
test "$(systemctl show -p AmbientCapabilities --value velocity-report.service)" = "cap_net_bind_service cap_net_raw"
systemd-run --quiet --wait --pipe --collect --unit velocity-pcap-selfcheck \
  -p User=velocity -p AmbientCapabilities=CAP_NET_RAW \
  /usr/local/bin/velocity-report -self-check -self-check-live-capture=lo
'

echo "==> validating API and database persistence"
wait_api
assert_radar_capabilities
curl -fsS "$API"/api/sites | grep -q 'Sample Site'
VERSION_BEFORE="$(curl -fsS "$API"/api/version)"
target_sh 'systemctl restart velocity-report.service; systemctl is-active --quiet velocity-report.service; test -s /var/lib/velocity-report/sensor_data.db'
wait_api
curl -fsS "$API"/api/sites | grep -q 'Sample Site'

echo "==> validating offline upgrade and rollback"
target_sh '/usr/local/bin/velocity device upgrade --binary /tmp/velocity-candidate'
target_sh 'test -n "$(readlink /opt/velocity-report/previous)"; systemctl is-active --quiet velocity-report.service'
wait_api
VERSION_AFTER="$(curl -fsS "$API"/api/version)"
test "$VERSION_AFTER" = "$VERSION_BEFORE"
target_sh '/usr/local/bin/velocity device rollback'
target_sh 'case "$(readlink /opt/velocity-report/current)" in versions/local-*) exit 1;; versions/*) :;; *) exit 1;; esac; systemctl is-active --quiet velocity-report.service'
wait_api
curl -fsS "$API"/api/sites | grep -q 'Sample Site'

echo "==> ARM64 full-system release validation passed ($TARGET)"
