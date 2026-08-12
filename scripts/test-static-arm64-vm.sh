#!/usr/bin/env bash
# Boot the release ARM64 binary under a real ARM64 Debian kernel and validate
# systemd privileges, live libpcap capture, API/database persistence, upgrade,
# and rollback. This is the hardware-independent release gate; qemu-user is not
# sufficient because it cannot emulate the SIOCETHTOOL calls used by libpcap.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BIN="${1:-}"
if [[ -z "$BIN" || ! -f "$BIN" ]]; then
    echo "usage: $0 <static-linux-arm64-velocity-binary>" >&2
    exit 2
fi
BIN="$(cd "$(dirname "$BIN")" && pwd)/$(basename "$BIN")"

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
qemu-system-aarch64 \
    -machine virt,accel=tcg -cpu max -smp 2 -m 1536 \
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
wait_api() {
    for _ in $(seq 1 90); do
        if curl -fsS http://127.0.0.1:18082/api/version >/dev/null 2>&1; then return 0; fi
        sleep 1
    done
    echo "API did not become ready" >&2
    return 1
}
assert_radar_capabilities() {
    curl -fsS http://127.0.0.1:18082/api/capabilities | python3 -c '
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

retry_transport "${SCP[@]}" "$BIN" root@127.0.0.1:/tmp/velocity-candidate
retry_transport "${SCP[@]}" "$REPO_ROOT/image/stage-velocity/03-velocity-config/files/velocity-report.service" \
    root@127.0.0.1:/tmp/velocity-report.service

retry_transport "${SSH[@]}" 'set -eu
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
curl -fsS http://127.0.0.1:18082/api/sites | grep -q 'Sample Site'
VERSION_BEFORE="$(curl -fsS http://127.0.0.1:18082/api/version)"
"${SSH[@]}" 'systemctl restart velocity-report.service; systemctl is-active --quiet velocity-report.service; test -s /var/lib/velocity-report/sensor_data.db'
wait_api
curl -fsS http://127.0.0.1:18082/api/sites | grep -q 'Sample Site'

echo "==> validating offline upgrade and rollback"
"${SSH[@]}" '/usr/local/bin/velocity device upgrade --binary /tmp/velocity-candidate'
"${SSH[@]}" 'test -n "$(readlink /opt/velocity-report/previous)"; systemctl is-active --quiet velocity-report.service'
wait_api
VERSION_AFTER="$(curl -fsS http://127.0.0.1:18082/api/version)"
test "$VERSION_AFTER" = "$VERSION_BEFORE"
"${SSH[@]}" '/usr/local/bin/velocity device rollback'
"${SSH[@]}" 'case "$(readlink /opt/velocity-report/current)" in versions/local-*) exit 1;; versions/*) :;; *) exit 1;; esac; systemctl is-active --quiet velocity-report.service'
wait_api
curl -fsS http://127.0.0.1:18082/api/sites | grep -q 'Sample Site'

echo "==> ARM64 full-system release validation passed"
