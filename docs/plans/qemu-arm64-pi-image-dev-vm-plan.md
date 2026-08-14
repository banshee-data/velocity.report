# QEMU + Headscale + pi-gen image (arm64 emulation)

- **Status:** design (variant of qemu-headscale-dev-vm-plan.md)
- **Owner:** patrickod
- **Branch:** patrickod/tailscale-acls (anticipated follow-on PR)
- **Canonical:** [qemu-headscale-dev-vm.md](../platform/operations/qemu-headscale-dev-vm.md)

## What this variant does differently

This plan describes an alternative approach to the amd64 cloud-init variant:
instead of provisioning a generic Debian cloud image with cloud-init,
**boot the actual pi-gen-built `.img` file under QEMU aarch64 emulation**.

This trades off speed for byte-for-byte production fidelity: every artifact
in the VM is the exact binary shipped to customers, down to the kernel and
kernel modules. The downside is that without KVM (aarch64 on an x86_64 host
requires TCG emulation), boot time is 3–5 minutes instead of 30 seconds, and
runtime performance is noticeable slower.

**When to use this variant:**

- You suspect arch-specific behaviour in the Tailscale or auth paths (rare).
- You want to spot-check the full image before release.
- You're verifying a kernel or systemd behaviour specific to the Pi.
- You need reproducibility across multiple runs for deterministic testing.

**When to use the amd64 variant (from the main plan):**

- Day-to-day Tailscale development (faster iteration).
- Testing userland logic only (auth gates, capability grants, manager state).
- Quick cycle time matters more than architectural faithfulness.

Both variants use **Headscale for the coordinator** (offline, reproducible)
and the **peer container for cap/grant testing**. Only the VM's boot path
and disk image differ.

## Architecture

```
host (Linux x86_64)
├── headscale (Docker)
│   ├── listens on 127.0.0.1:8085
│   ├── embedded DERP
│   └── policy.yaml
│
├── peer-tailscale (Docker)
│   └── joined to Headscale, admin user
│
└── velocity-vm (QEMU)
    ├── qemu-system-aarch64 (no KVM, TCG)
    ├── Actual pi-gen .img (rebuilt locally)
    │   ├── Kernel 6.1.21+ (Raspberry Pi)
    │   ├── Full image with radar/lidar drivers (disabled at runtime)
    │   ├── All published artifacts verbatim
    │   └── Matches production byte-for-byte
    ├── hostfwd: tcp 2222→22, tcp 8080→8080
    ├── login-server override: tailscaled.service systemd drop-in
    └── /usr/local/bin/velocity + velocity-report alias (replaced via qemu-push)
```

Note: `qemu-system-aarch64` with TCG is slow. On a typical developer
machine (2023+ laptop), first boot takes 3–5 minutes, subsequent boots 2–3
minutes. Service restart and `make qemu-push` cycles are still under 10 seconds
because the binaries are small.

## Getting the .img file

The variant assumes a pre-built `.img` file from `./image/scripts/build-image.sh`.
If you haven't built the image locally, the first setup step is:

```bash
./image/scripts/build-image.sh
# Output: image/velocity-report-*.img
```

This takes 20–30 minutes on a first build (downloads the full pi-gen bootstrap
and Debian archive). Cached on subsequent builds (re-running after a code change
is much faster).

The resulting `.img` can be used for both:

1. Flashing to a real Pi SD card (as today).
2. Booting in the arm64 QEMU variant (this plan).

## Layout

```
image/qemu-arm64/                      # sibling of image/qemu/
├── README.md
├── Makefile                            # qemu-arm64-{build,up,down,push,ssh,logs,reset}
├── scripts/
│   ├── find-or-build-img.sh            # locate velocity-report-*.img or build it
│   ├── convert-img-to-qcow2.sh         # .img → .qcow2 for QEMU (speeds up sparse I/O)
│   ├── run-vm.sh                       # qemu-system-aarch64 -M virt -m 2G ...
│   ├── stop-vm.sh                      # graceful shutdown
│   ├── push-binaries.sh                # cross-build aarch64 if needed; scp
│   ├── ssh-vm.sh                       # ssh -p 2222
│   ├── logs-vm.sh                      # journalctl -fu velocity-report
│   └── inject-login-server.sh          # one-shot: sed /etc/default/tailscaled inside the running VM
├── cloud-init/                         # minimal; login-server injection only
│   └── login-server-override.yaml      # drops /etc/default/tailscaled FLAGS=
├── files/
│   ├── 020_velocity-nopasswd           # symlink → image/stage-velocity/...
│   └── velocity-report.service         # symlink → image/stage-velocity/...
└── secrets/
    ├── id_ed25519, id_ed25519.pub      # VM SSH key (auto-generated)
    └── headscale-preauthkey            # optional

Top-level symlink:
  image/qemu-arm64 → image/qemu/docker-compose.yml  (shares Headscale + peer)
```

The `qemu-arm64/` variant shares the Headscale + peer stack with `qemu/`,
so `make qemu-up` (from the amd64 plan) and `make qemu-arm64-up` can both
run, though obviously not simultaneously.

## Key differences from the amd64 variant

### Image provisioning

**amd64 (cloud-init):**

- Generic Debian cloud image (small download)
- cloud-init paints on users, sudoers, and the pinned static Tailscale payload
- Can provision any Debian 12/13 image

**arm64 (pi-gen .img):**

- Actual pi-gen-built image from `./image/scripts/build-image.sh`
- No cloud-init provisioning (already contains everything)
- Only override needed: `/etc/default/tailscaled` login-server injection
- Binary-identical to what ships to customers

### Injection of login-server override

The amd64 variant uses cloud-init to set `FLAGS="--login-server=http://10.0.2.2:8085"`.

The arm64 variant cannot inject at boot time (no cloud-init datasource for
raw .img files under QEMU). Instead:

1. **Option A (recommended):** `make qemu-arm64-up` boots the VM, then
   `make qemu-arm64-inject-login-server` scp's into it and sed's
   `/etc/default/tailscaled` to add the `FLAGS=` line. Happens once per
   `make qemu-arm64-reset`.
2. **Option B:** Bake the override into the pi-gen build itself. Add a stage
   that patches `/etc/default/tailscaled` when the image is built. More
   invasive; outside scope of this plan.

Option A is implemented here.

### Boot machine type

`qemu-system-aarch64 -M virt` is used instead of the real `-M raspi3b` or
`-M raspi4b` machine type. Reasons:

- `-M virt` is the standard QEMU generic ARM64 machine and has better
  emulation performance than the Pi-specific types.
- The kernel and drivers are identical (from the pi-gen image).
- Most userland sees no difference.
- The tradeoff: some Pi-specific hardware (HAT SPI, UART on pins) is
  unavailable, but we're not exercising those anyway (sensors are disabled).

The machine still sees `systemd`, a real kernel, the pinned static Tailscale
payload, and all production paths. It's just not a byte-identical Pi
hardware replica.

### Performance implications

On a 2023 MacBook Pro M1 (running Linux via Parallels, so nested virt):

- First boot: ~5 min
- Subsequent boots: ~2–3 min
- `make qemu-arm64-push` (rebuild + scp + restart): ~10 s
- `journalctl -fu velocity-report`: real-time, no noticeable latency

On bare metal Linux with KVM-capable AMD Ryzen:

- First boot: ~3 min
- Subsequent boots: ~90 s
- The amd64 variant is 6–10× faster.

For day-to-day work, the amd64 variant is strongly preferred. The arm64
variant is for verification and spot-checks.

## Makefile targets

```
make qemu-arm64-build                  # locate/build .img, convert to qcow2, gen SSH key
make qemu-arm64-up                     # boot VM, inject login-server
make qemu-arm64-down                   # graceful shutdown
make qemu-arm64-reset                  # destroy qcow2, rebuild from .img
make qemu-arm64-push                   # cross-build aarch64 binaries, scp, restart
make qemu-arm64-ssh
make qemu-arm64-logs
make qemu-arm64-status

make qemu-arm64-inject-login-server    # one-shot: patch /etc/default/tailscaled
```

The `qemu-arm64-inject-login-server` target is called automatically by
`qemu-arm64-up` if not yet applied. Subsequent `make qemu-arm64-reset`
cycles re-apply it.

## .img conversion

The pi-gen script outputs a `.img` file, which is a raw disk image suitable
for flashing to an SD card. QEMU can boot it directly with `-drive
file=velocity-report-*.img`, but for performance and convenience:

- Convert to qcow2 format: `qemu-img convert -f raw -O qcow2 velocity-report-*.img velocity-report.qcow2`
- qcow2 supports sparse writes (the `.img` is typically sparse, padded to 4GB; qcow2 saves disk space)
- qcow2 can be snapshotted; useful if you want to `make qemu-arm64-reset` without rebuilding the image

The conversion happens once in `qemu-arm64-build`.

## Validation criteria

The arm64 variant is considered working when:

- `make qemu-arm64-build && make qemu-arm64-up && make qemu-arm64-push &&
curl http://localhost:8080/api/health` returns 200 from a fresh checkout.
- `make qemu-arm64-logs` shows the service running without errors.
- Headscale peer interactions (`make qemu-peer-curl`) succeed, cap/view and
  cap/admin are enforced correctly.
- Full `make qemu-smoke` (subroutine of `qemu-arm64-up`) verifies the image
  is what we expect (exact binary match against pi-gen build).

## Risks and open questions

**R1: TCG performance.** Without KVM (aarch64 on x86_64), QEMU falls back to
TCG (Tiny Code Generator), which is slow. Acceptable for occasional
spot-checks, not for rapid iteration. If you're on Apple Silicon (M1+) and
can run `qemu-system-aarch64` with KVM (via UTM or Parallels), the arm64
variant becomes much more viable. This is a host-capability question, not a
plan issue.

**R2: qcow2 sparse semantics.** The `.img` file may be partially sparse
(unallocated space). Converting to qcow2 preserves sparse regions, saving
disk space on the host. But if the image is fully materialised (not sparse),
conversion still takes time. Documented in the README; not a blocker.

**R3: Divergence from prod on -M virt.** Using `-M virt` instead of
`-M raspi3b` means the VM is not a 1:1 Pi simulation. This is acceptable
for userland testing but would not catch Pi-specific kernel bugs. If that
becomes a concern, a follow-on `qemu-arm64-raspi` variant using the real
machine type is possible (just slower).

**R4: Login-server injection timing.** The current approach injects
`/etc/default/tailscaled FLAGS=` on the first `qemu-arm64-up` and sticks
across boots until `make qemu-arm64-reset`. This is stable and predictable.
If the pi-gen build ever embeds Headscale coordination by default, this
injection becomes unnecessary.

## Implementation order

1. **Image build + conversion.** `find-or-build-img.sh` and
   `convert-img-to-qcow2.sh`. Test that the qcow2 boots standalone.
2. **VM orchestration.** `run-vm.sh`, `stop-vm.sh`, `ssh-vm.sh`, `logs-vm.sh`.
   Test SSH + basic operations on the running VM.
3. **Login-server injection.** `inject-login-server.sh` wraps the sed
   command; call from `qemu-arm64-up`.
4. **Binary push.** `push-binaries.sh` for aarch64 (native compile on
   the VM, or cross-compile on the host if you have GOARCH=arm64
   toolchain). Test that `make qemu-arm64-push` restarts the service.
5. **Headscale integration.** Symlink to the shared docker-compose.yml
   and peer helpers from the amd64 plan. Verify `make qemu-peer-curl`
   works against the arm64 VM.
6. **Makefile integration.** Merge the arm64 targets into the top-level
   Makefile. Ensure `make qemu-arm64-*` targets coexist with `make qemu-*`.
7. **Documentation.** A README under `image/qemu-arm64/` and update to
   the main Tailscale ops guide referencing both variants.

## What this plan deliberately does not do

- It does not replace the amd64 variant; both coexist. The Makefile has
  `qemu-*` (amd64) and `qemu-arm64-*` targets.
- It does not try to run multiple VMs simultaneously (infrastructure overhead).
- It does not emulate specific Pi hardware (GPIO, HAT, UART on pins) — only
  the userland Tailscale + auth surfaces.
- It does not change the pi-gen build itself. The `.img` is consumed
  as-is; no modifications to `image/stage-*`.

## Relationship to the main plan

These two plans are **parallel strategies for the same goal** (a local
Tailscale dev environment). Use them as follows:

| Goal                                    | Use                   |
| --------------------------------------- | --------------------- |
| Daily Tailscale development             | amd64 plan (fast)     |
| Spot-check production image             | arm64 plan (faithful) |
| Verify auth middleware logic            | amd64 plan (faster)   |
| Verify full stack before release        | arm64 plan (complete) |
| Test peer capability resolution quickly | amd64 plan (iterate)  |
| Reproduce a hard-to-debug arm64 issue   | arm64 plan (exact)    |

Both plans use the same **Headscale coordinator and peer containers**
(docker-compose.yml is shared). The only difference is the VM's boot
mechanism and disk image source.

**Implementation sequencing:**

1. Land the amd64 plan first (simpler, faster iteration).
2. Land the arm64 plan after the amd64 variant is proven.
3. Document both in the Tailscale ops guide so developers can choose.
