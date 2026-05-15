# QEMU + Headscale Tailscale Dev VM

**Status:** design
**Date:** 2026-05-15
**Owner:** patrickod
**Branch:** patrickod/tailscale-acls (anticipated follow-on PR)

## Why this exists

Work on the Tailscale integration ([internal/tailscale/](../../internal/tailscale/),
[internal/api/auth.go](../../internal/api/auth.go),
[cmd/velocity-ctl/tailscale.go](../../cmd/velocity-ctl/tailscale.go))
currently has no good local test environment. The only options today are:

1. Iterate on a real Pi connected to a real tailnet — slow, requires hardware,
   risks the operator's actual tailnet identity.
2. Unit tests with fakes — already extensive
   ([internal/tailscale/peercaps_test.go](../../internal/tailscale/peercaps_test.go),
   [internal/api/auth_test.go](../../internal/api/auth_test.go)), but they
   cannot exercise the real systemd lifecycle, the real `tailscaled` daemon's
   IPN bus, the real `WhoIs` resolution, the real sudoers boundary, or the
   real-world failure modes of those interfaces.

This plan creates a third option: a locally-bootable amd64 Linux VM that runs
the same userland as the production Pi image, points its `tailscaled` at a
locally-hosted Headscale coordinator, and exposes a Makefile-driven
build/iterate/teardown loop. No external Tailscale account, no external
network calls, no contact with the developer's host-side `tailscaled`.

## Scope (v1)

In scope:

- A QEMU VM that boots Debian (matching the Pi image's apt-tailscale path).
- Cloud-init provisioning that mirrors the production user, group, sudoers,
  and systemd unit layout from [image/stage-velocity/](../../image/stage-velocity/).
- A Docker-hosted Headscale coordinator and at least one Docker-hosted
  "peer" Tailscale daemon, both isolated from the host's `tailscaled`.
- Makefile targets for build, up, down, push, ssh, logs, reset, and peer
  control.
- Documentation under [docs/platform/operations/](../../docs/platform/operations/)
  for day-to-day use.

Out of scope (deferred to follow-on PRs):

- `//go:build integration` Go tests that drive the VM via HTTP.
- CI integration (GitHub Actions).
- aarch64 / pi-gen .img bring-up (potential `qemu-arm64` target later).
- LiDAR / radar hardware emulation. Sensors stay disabled inside the VM, as
  with [`make dev-go`](../../Makefile).
- Multi-VM clustering or fleet simulation.

## Non-goals

- Replacing the existing unit tests. The fake `LocalClient`/`BusWatcher`
  fixtures in [internal/tailscale/manager_test.go](../../internal/tailscale/manager_test.go)
  remain the primary regression net. The VM is for exercising the seams
  those fakes cannot reach.
- Production-faithful hardware behaviour. The VM is an integration target
  for the Tailscale and authorization surfaces only.
- Cross-platform support. Linux host with `/dev/kvm` is required.

## Architecture

```
host (Linux x86_64, /dev/kvm)
├── headscale (Docker)
│   ├── listens on host 127.0.0.1:8085  (control plane API)
│   ├── embedded DERP on 127.0.0.1:3478/udp
│   ├── /var/lib/headscale-velocity/    (state, gitignored)
│   └── policy.yaml with velocity.report/cap/{view,admin} grants
│
├── peer-tailscale (Docker)             ← second tailnet node, used to
│   ├── shares Headscale state but            test peer-cap requests
│   ├── isolated from host tailscaled
│   └── exposes a tailscale CLI for `curl`-from-peer scripts
│
└── velocity-vm (QEMU)
    ├── qemu-system-x86_64 -enable-kvm
    ├── Debian 13 (trixie) generic cloud image
    ├── hostfwd: tcp 2222→22, tcp 8080→8080
    ├── /etc/default/tailscaled FLAGS="--login-server=http://10.0.2.2:8085"
    ├── tailscaled installed (apt) and masked, identical to image stage 07
    ├── velocity user + sudoers identical to image stage 03
    ├── /usr/local/bin/velocity-report  (populated via qemu-push)
    └── /usr/local/bin/velocity-ctl     (populated via qemu-push)
```

`10.0.2.2` is the QEMU user-mode networking gateway IP — the address the
guest uses to reach the host loopback. Headscale binds to host
`127.0.0.1:8085`, so only this dev VM and locally-spawned peer containers
can reach it.

### Why amd64 + KVM, not aarch64

The production Pi is ARM64. The dev VM is amd64 because:

- KVM acceleration on an x86_64 Linux host makes bring-up sub-30-seconds
  vs. multi-minute boot under TCG emulation.
- The code paths under test are arch-agnostic Go: `internal/tailscale/`,
  `internal/api/auth.go`, and the velocity-ctl shell-out are pure userland
  with no architecture-specific call sites.
- The userland (Debian release, systemd version, tailscale apt repo,
  sudoers behaviour) is identical across the two architectures within the
  Debian release we target.

The single fidelity gap is the kernel ABI. If a future bug is suspected to
be ARM64-specific, the existing pi-gen build (`./image/scripts/build-image.sh`)
produces a real ARM64 image that can be inspected; a `qemu-arm64` target is
a possible v2 extension but is not required to land v1.

### Why Headscale, not a Tailscale account

The user constraint is "without interacting with my actual tailscale
daemon" and "in as close an environment to the production thing as
possible." Headscale satisfies both:

- **Hermetic.** No outbound calls to `controlplane.tailscale.com`. State
  lives in a local Docker volume that can be torn down with one command.
- **Production-faithful.** `tailscaled` does not know it is talking to
  Headscale — the protocol is the same, the local API is the same, the
  IPN bus events are the same, `WhoIs` returns the same shape. Every Go
  code path under test exercises real production calls.
- **Capability grants supported.** Headscale's policy format supports the
  `grants` block with `app` capabilities. The grants we need
  (`velocity.report/cap/view`, `velocity.report/cap/admin`) come through
  in `WhoIs.CapMap` exactly as they would in production.
- **Reproducible.** Every developer running this gets the same coordinator
  behaviour. With a real account, ACL drift between developers' tailnets
  becomes a category of test flake.

Trade-off: Headscale is a third-party project and its `tailscale up`
registration flow differs slightly from `controlplane.tailscale.com`
(Headscale issues a `nodekey:...` registration string the operator
runs through `headscale nodes register`). The
`internal/tailscale/manager.go` flow (`StartLoginInteractive` → bus →
`BrowseToURL` → `Running`) still exercises end-to-end correctly; the
operator just registers from a host-side CLI rather than a hosted web
page. The Makefile wraps this into one command.

## Faithful vs. mocked

| Component                                        | Faithful           | Mocked                                  |
| ------------------------------------------------ | ------------------ | --------------------------------------- |
| systemd as PID 1                                 | yes                |                                         |
| `velocity` system user + literal-argv sudoers    | yes                |                                         |
| `tailscaled` (upstream apt), masked-by-default   | yes                |                                         |
| `velocity-ctl tailscale enable-tailscaled` flow  | yes                |                                         |
| `tailscale set --operator=velocity`              | yes                |                                         |
| `velocity-report.service` unit                   | yes                |                                         |
| Tailscale local API socket                       | yes (real daemon)  |                                         |
| IPN bus subscription + `BrowseToURL`             | yes (real daemon)  |                                         |
| `WhoIs` peer resolution + cap grants             | yes (via Headscale)|                                         |
| `tailscale serve` config on :443                 | yes                |                                         |
| Radar serial (`/dev/ttySC1`)                     |                    | `--radar=false`                         |
| LiDAR UDP                                        |                    | `--lidar=false`                         |
| Kernel architecture                              |                    | amd64 host kernel, not ARM64            |
| Pi-specific hardware (HAT, UART)                 |                    | absent; sensors flagged off             |
| Tailscale coordination control plane             | yes (Headscale)    | (Headscale ≠ `controlplane.tailscale.com`, but the wire protocol matches) |

## Files and layout

```
image/qemu/                                  # sibling of image/stage-velocity/
├── README.md                                # quickstart, troubleshooting
├── Makefile                                 # included by top-level Makefile
├── docker-compose.yml                       # headscale + peer-tailscale
├── headscale/
│   ├── config.yaml                          # server config: bind, DERP, MagicDNS
│   ├── policy.yaml                          # grants: cap/view, cap/admin
│   └── derp.yaml                            # embedded DERP, no external relays
├── scripts/
│   ├── build-vm.sh                          # pull cloud image, gen seed ISO, materialise qcow2
│   ├── run-vm.sh                            # launch qemu with KVM + hostfwd
│   ├── stop-vm.sh                           # ssh poweroff with qemu-monitor fallback
│   ├── push-binaries.sh                     # GOOS=linux GOARCH=amd64 build → scp
│   ├── ssh-vm.sh                            # wraps ssh -p 2222 with the generated key
│   ├── logs-vm.sh                           # journalctl -fu velocity-report over ssh
│   ├── headscale-register.sh                # one-shot: create user/preauth-key, register VM
│   └── peer-curl.sh                         # curl <url> from peer-tailscale container
├── cloud-init/
│   ├── user-data.tmpl                       # rendered with SSH key + Headscale URL
│   ├── meta-data
│   └── network-config
├── files/
│   ├── 020_velocity-nopasswd                # symlink → image/stage-velocity/03-velocity-config/
│   ├── velocity-report.service              # symlink → image/stage-velocity/03-velocity-config/files/
│   └── tailscaled.default                   # /etc/default/tailscaled with --login-server=...
├── secrets/                                 # gitignored; populated on first build
│   ├── id_ed25519                           # VM access keypair (auto-generated)
│   ├── id_ed25519.pub
│   └── README.md                            # explains what's here, what's gitignored
└── .gitignore                               # secrets/, *.qcow2, *.iso, .state/
```

The .gitignore at the qemu root excludes everything generated; only the
source-controlled scaffolding (scripts, templates, docker-compose,
policy) is checked in.

The `files/` symlinks are intentional — the sudoers grant and systemd
unit must remain byte-identical to the Pi image. If they drift, the dev
VM stops being a faithful test target. This is enforced by symlink rather
than by a copy-and-trust-CI scheme.

The systemd unit lives at
[image/stage-velocity/03-velocity-config/files/velocity-report.service](../../image/stage-velocity/03-velocity-config/files/velocity-report.service)
already, so symlinking is straightforward. The sudoers content is
currently an inline heredoc inside
[image/stage-velocity/03-velocity-config/00-run.sh](../../image/stage-velocity/03-velocity-config/00-run.sh).
**Step 0 of the implementation order** below extracts that heredoc into
a real file (e.g., `files/020_velocity-nopasswd`) and rewrites stage 03
to `install -m 440` it. Once extracted, both the Pi image and the dev
VM consume the same file.

## Makefile targets (added to top-level Makefile)

```
make qemu-build              # idempotent: pull cloud image, generate seed ISO,
                             #             materialise qcow2, create SSH keypair
make qemu-up                 # start headscale + peer container + VM, wait for ssh
make qemu-down               # graceful stop of VM, stop docker stack
make qemu-reset              # destroy qcow2 + headscale state, then qemu-build
make qemu-ssh                # interactive ssh into the VM
make qemu-logs               # follow velocity-report.service journal
make qemu-status             # tabular: VM state, headscale state, peer state
make qemu-push               # cross-build amd64 binaries, scp, restart unit

make headscale-key           # generate a preauth key, print it
make headscale-policy-apply  # reload headscale/policy.yaml
make headscale-nodes         # list nodes from headscale CLI
make headscale-register-vm   # one-shot: bring VM tailscaled up, register against headscale

make qemu-peer-curl URL=...  # exec into peer-tailscale, curl <URL> against MagicDNS of VM
```

`make qemu-up` will be the typical entry point. It should:

1. Start the docker-compose stack (headscale + peer container).
2. Wait for headscale to be reachable on `127.0.0.1:8085`.
3. Boot the VM in the background with a PID file at `image/qemu/.state/vm.pid`.
4. Poll `ssh -p 2222 pi@localhost true` until success or 60s timeout.
5. Print connection summary: SSH command, web UI URL, headscale URL, peer
   container name.

`make qemu-push` cross-builds via `GOOS=linux GOARCH=amd64 make build-radar-linux`
(extending the existing target) and scp's both `velocity-report` and
`velocity-ctl` into `/usr/local/bin/`, then `sudo systemctl restart
velocity-report.service` and `journalctl -n 20 -u velocity-report` for a
quick smoke check.

`make qemu-reset` removes `image/qemu/secrets/id_ed25519*`, the qcow2,
the Headscale state volume, and re-runs `qemu-build`. The SSH key is
deliberately regenerated on reset so a stale `known_hosts` entry never
silently blocks the new VM.

## Cloud-init contents

`user-data.tmpl` is templated by `build-vm.sh` and renders to a NoCloud
seed ISO. Key sections:

1. **User creation.** `pi` (UID 1000) for SSH and `velocity` (system
   user) matching image stage 03 exactly, including the `dialout`
   secondary group. `pi` is granted the same broad `velocity-ctl *`
   sudoers, `velocity` gets the literal-argv grants — both written to
   `/etc/sudoers.d/020_velocity-nopasswd` via the symlinked source file.
2. **SSH authorized keys.** The auto-generated `id_ed25519.pub` is
   installed in `/home/pi/.ssh/authorized_keys`. Password auth is off.
3. **Tailscale apt repo install.** Reproduces the codename-detect logic
   from `image/stage-velocity/07-velocity-tailscale/01-run.sh`.
4. **Tailscale config override.** `/etc/default/tailscaled` carries
   `FLAGS="--login-server=http://10.0.2.2:8085"` so the daemon, when
   eventually unmasked, contacts Headscale rather than upstream.
5. **Tailscaled mask.** `systemctl mask tailscaled.service` — same as
   stage 07. The operator's `Enable` toggle (via the velocity-report
   web UI) is what unmasks at runtime, which is what we want to test.
6. **Directory and unit install.** `/var/lib/velocity-report` (owned by
   `velocity:velocity`), `/opt/velocity-report/config/`, and the
   systemd unit file via the symlinked source. The unit is `enabled`
   so the next `qemu-push` produces a service that starts on boot, but
   not started yet (no binaries present).
7. **Sensor flags.** A `velocity-report.service.d/override.conf` drop-in
   sets `Environment="VELOCITY_FLAGS=--radar=false --lidar=false"` and
   the `ExecStart=` is overridden to append `$VELOCITY_FLAGS`. The
   override is the only deviation from the production unit; it lives in
   the VM, not in the source unit file.

## Headscale configuration

`headscale/config.yaml`:

```yaml
server_url: http://10.0.2.2:8085
listen_addr: 0.0.0.0:8085
metrics_listen_addr: 127.0.0.1:9090
ip_prefixes:
  - 100.64.0.0/10
  - fd7a:115c:a1e0::/48
derp:
  server:
    enabled: true
    region_id: 999
    region_code: "velocityvm"
    region_name: "VelocityVM Local DERP"
    stun_listen_addr: "0.0.0.0:3478"
  urls: []
  paths: []
disable_check_updates: true
ephemeral_node_inactivity_timeout: 5m
database:
  type: sqlite3
  sqlite:
    path: /var/lib/headscale/db.sqlite
policy:
  mode: file
  path: /etc/headscale/policy.yaml
```

`headscale/policy.yaml` follows Headscale's current policy format
(pinned by the `headscale` Docker image tag — see R1). The policy must
encode three things:

- A `tag:velocity-vm-test` tag that the VM advertises on join.
- A `cap/view`-bearing source group (e.g., all members of a `viewers`
  user/group) targeting `tag:velocity-vm-test`.
- A `cap/admin`-bearing source group (e.g., all members of `admins`)
  targeting `tag:velocity-vm-test`.

The peer container is registered under the `admins` group by default,
so requests from the peer carry the `cap/admin` grant — the most useful
default for poking at admin-only routes during development. To test the
read-only `cap/view` path, the operator either re-registers the peer
container under the `viewers` group via
`make headscale-register-peer USER=viewers`, or runs a second peer
container; both flows are wrapped by Makefile targets.

Concrete keys and syntax (`acls` vs. `policy`, `grants` block shape,
group declarations) are pinned to the Headscale version the docker-compose
file pulls. The exact policy file lands with the implementation PR; the
contract above is what matters for the design.

Default Tailscale ACL on Headscale denies inter-node traffic; the
`accept *→*` ACL above is intentional for this dev tooling. Production
tailnets are expected to lock down further; this dev environment is
deliberately permissive so cap-grant differences are the only variable
under test.

## The peer container

A second Tailscale node is required to exercise the auth gate's
"request from a tailnet peer" branch in
[internal/api/auth.go](../../internal/api/auth.go). The peer is a small
Docker container running `tailscale/tailscale:latest` (the official
upstream image) pointed at the same Headscale URL and joined with a
distinct user. It runs in `--userspace-networking` mode with a SOCKS
proxy, so the operator can `curl` the VM's tailnet IP / MagicDNS from
inside the container without TUN privileges.

`make qemu-peer-curl URL=http://velocity-vm.headscale-local:443/api/...`
wraps `docker exec peer-tailscale tailscale ssh ...` or a direct
`docker exec peer-tailscale curl ...` through the SOCKS proxy.

The peer container has its own state directory mounted from a Docker
volume, fully isolated from the host's `tailscaled`. This is the
mechanism that fulfils "without interacting with my actual tailscale
daemon": there are exactly two tailscaled processes in scope (one in
the VM, one in the peer container), both pointed at Headscale, neither
sharing state with the host.

## Day-in-the-life workflow

The expected daily-driver loop:

```bash
make qemu-build            # one-time, ~2 min (downloads cloud image)
make qemu-up               # ~30 s, runs at the start of each session

# In a separate terminal, open the web UI:
xdg-open http://localhost:8080

# Edit Go code, then push:
make qemu-push             # ~5 s
make qemu-logs             # observe in another tab

# Toggle Tailscale on in the web UI; the registration URL Headscale
# returns is printed to make qemu-logs. Run:
make headscale-register-vm # auto-registers the VM against Headscale

# Now the VM is on the Headscale tailnet. The peer container is too.
# Exercise the peer-cap path:
make qemu-peer-curl URL=http://velocity-vm/api/sites

# At end of day:
make qemu-down
```

## Testing the spec itself

A small set of smoke checks for "did we wire this up correctly" lives in
[image/qemu/scripts/smoke.sh](../../image/qemu/scripts/smoke.sh) and is
invoked as `make qemu-smoke`. It exercises the structural assumptions
without depending on Tailscale semantics:

1. `qemu-up` brings ssh up within timeout.
2. `id velocity` exits zero inside the VM; the system user is present.
3. `systemctl is-enabled tailscaled` returns `masked` on first boot.
4. `sudo -n -u velocity sudo -n -l /usr/local/bin/velocity-ctl
   tailscale enable-tailscaled` returns the literal argv (sudoers grant
   matches argv; `-l` lists permission without executing).
5. `curl -fsS http://localhost:8080/api/health` returns 200 after push.

These do not test the Tailscale integration directly. They test that the
VM environment is the environment we say it is.

## Risks and open questions

**R1: Headscale policy format drift.** Headscale v0.23+ uses policy
format v2 (hujson/yaml grants block). If we pin a Headscale version, a
breaking format change upstream is an upgrade chore, not a constant
worry. Mitigation: pin `headscale` to a specific image tag in the
docker-compose file. Note the version in this plan when implementing.

**R2: Cloud-init re-runs on existing qcow2.** Cloud-init by default
runs only on first boot. A `make qemu-reset` is required to re-run
provisioning; this is mentioned in the README and is the explicit reset
mechanism. Editing `user-data.tmpl` after the VM has booted does
nothing until reset.

**R3: Drift between Pi image and VM provisioning.** The symlinks for
the sudoers file and systemd unit prevent the most obvious drift.
Anything else (e.g., the codename allowlist, the apt repo URL) is
copied conceptually but not byte-identically. A periodic manual check
against `image/stage-velocity/07-velocity-tailscale/01-run.sh` is part
of the workflow; the README notes which sections must remain in sync.

**R4: `--login-server` injection point.** The current
`internal/tailscale/manager.go` neither sets nor cares about the
control plane URL; it talks to the local API socket. The login server
is configured at daemon level via `/etc/default/tailscaled`'s `FLAGS=`,
which is set up in cloud-init. If a future change makes Go code call
`tailscale up` directly with a hard-coded server, this plan breaks.
Mitigation: a comment in the relevant Go code referencing this plan.

**R5: MagicDNS resolution from the peer container.** Headscale's
MagicDNS depends on `--accept-dns=true` and a working DNS path. With
userspace networking the peer container has `tailscaled` resolve names
in-process. Verified to work in similar setups; if it fails, the
fallback is the peer container resolving the VM by tailnet IP from
`headscale nodes list` output (less ergonomic but functional).

**R6: KVM availability.** Linux developers on systems without `/dev/kvm`
(WSL2, containers without `--device`, etc.) get TCG fallback, which is
slow. The Makefile detects this and warns; the workflow remains
functional, just slow. Non-Linux developers are out of scope (see
non-goals).

**R7: Resource use.** Headscale + peer container + VM is roughly
2 GiB RAM and ~3 GiB disk. Documented in README; not a constraint on
modern developer machines.

## Implementation order

Each step a small, reviewable PR.

0. **Sudoers heredoc extraction.** Pull the inline sudoers content out
   of [image/stage-velocity/03-velocity-config/00-run.sh](../../image/stage-velocity/03-velocity-config/00-run.sh)
   into `image/stage-velocity/03-velocity-config/files/020_velocity-nopasswd`,
   and have `00-run.sh` `install -m 440` it. Pure refactor; production
   image build behaviour unchanged. This is a prerequisite for the dev
   VM and the Pi image consuming the same source-of-truth file.
1. **Scaffolding.** `image/qemu/{Makefile,docker-compose.yml,README.md}`,
   the `scripts/` directory, `cloud-init/user-data.tmpl`, the symlinks
   to image stage 03 files, and `.gitignore`. No Tailscale wiring yet
   — just `qemu-build`, `qemu-up`, `qemu-down`, `qemu-ssh`,
   `qemu-push`, `qemu-smoke`. Verify VM boots, SSH works, binaries
   push, service starts.
2. **Headscale stack.** Add `headscale/config.yaml`, `headscale/policy.yaml`,
   wire `docker-compose.yml`, add `headscale-key`,
   `headscale-policy-apply`, `headscale-nodes` Makefile targets. Verify
   `headscale` is reachable from inside the VM.
3. **Login-server override and registration.** Cloud-init drops
   `/etc/default/tailscaled` with `--login-server`; add
   `make headscale-register-vm`. Verify the full enable→register→Running
   path through the web UI.
4. **Peer container + cap testing.** Add the peer-tailscale service in
   docker-compose, the `qemu-peer-curl` target, and the smoke check
   that the peer can reach the VM with `cap/admin`. Document toggling
   to `cap/view` via policy edits.
5. **Documentation.** A new
   `docs/platform/operations/qemu-headscale-dev-vm.md` page linked from
   `docs/platform/operations/tailscale-remote-access.md` and the
   contributor guide.

## Validation criteria

The dev VM is considered working when:

- `make qemu-up && make qemu-push && curl http://localhost:8080/api/health`
  returns 200 from a clean checkout.
- A new tailscale-related Go change can be exercised end-to-end inside
  the VM in under 10 seconds from `make qemu-push` to first journal
  line, no rebuild from scratch.
- The `make qemu-smoke` check passes on a clean Linux developer box
  with `/dev/kvm` available.
- A `cap/view` peer cannot mutate the VM's state (POST returns 403),
  while a `cap/admin` peer can. Both verified through
  `make qemu-peer-curl`.

## What this plan deliberately does not do

- It does not add CI integration. CI work would be a follow-on that
  pins the Headscale version, caches the cloud image, and runs the
  smoke check on every PR. Out of scope for v1.
- It does not produce Go `//go:build integration` tests. Once the dev
  VM is stable, writing those tests is a separate task with its own
  plan. Mentioned here only so that the surface (`qemu-push`,
  `qemu-peer-curl`) is shaped to support it later.
- It does not modify `internal/tailscale/` or `internal/api/auth.go`.
  Those packages are the system under test, not the test harness.

## Variant: arm64 emulation with pi-gen image

An alternative approach exists: instead of cloud-init provisioning a generic
Debian image, boot the actual pi-gen-built `.img` under QEMU aarch64
emulation. This is slower (TCG, no KVM) but byte-for-byte identical to what
ships to customers.

See [qemu-arm64-pi-image-dev-vm-plan.md](qemu-arm64-pi-image-dev-vm-plan.md)
for that variant. **TL;DR:** use the amd64 variant for daily development
(faster), use the arm64 variant for production spot-checks (faithful).
