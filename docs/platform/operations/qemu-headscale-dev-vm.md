# QEMU and Headscale development VM

This is the stable operating home for a local Tailscale integration environment: a disposable QEMU
guest, a local Headscale coordinator, and a peer node, all isolated from the operator's real
tailnet. It exists to exercise systemd, `tailscaled`, capability grants, and network boundaries
without turning a production Raspberry Pi into a test fixture.

Active plans: [amd64 development VM](../../plans/qemu-headscale-dev-vm-plan.md) and
[arm64 pi-gen variant](../../plans/qemu-arm64-pi-image-dev-vm-plan.md).

## Delivered foundation

The repository already uses QEMU as part of ARM64 build and release confidence. This is useful
emulation infrastructure, but it is not the local QEMU-and-Headscale environment described by the
active plans.

| Delivered boundary                | Current behaviour                                                                                                                                | Evidence                                                                      |
| --------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------- |
| ARM64 static-binary emulation     | Static-build CI registers ARM64 emulation, runs the ARM64 executable, and exercises it under an ARM64 kernel through the static VM smoke script. | `.github/workflows/static-build-ci.yml` and `scripts/test-static-arm64-vm.sh` |
| Release-binary ARM64 kernel check | The image build workflow validates the exact ARM64 release binary under an ARM64 kernel before it is used by image assembly.                     | `.github/workflows/build-image.yml`                                           |
| pi-gen emulation setup            | The reusable pi-gen setup action installs and verifies the ARM64 user-mode emulator used by image-generation jobs.                               | `.github/actions/setup-qemu-pigen/action.yml`                                 |
| Host dependency guidance          | Image-build prerequisites account for non-ARM Linux hosts and document the ARM-emulation dependency for supported macOS Docker environments.     | `Makefile`                                                                    |

These checks establish that the existing release pipeline can execute ARM64 artefacts under
emulation. They do not exercise Tailscale authentication, Headscale policy, peer connectivity, or
the capability boundaries of a disposable guest.

## Intended operating model

- Use the amd64 cloud-image VM for fast Linux/KVM development and integration checks.
- Use the arm64 pi-gen image variant when production-image fidelity matters more than speed. It
  runs under emulation and is therefore a release-confidence check, not the everyday loop.
- Keep the coordinator and peer local. The environment must not contact the operator's host-side
  `tailscaled` or require a real Tailscale account.

## Not yet delivered

Neither local development environment is implemented yet. In particular, the repository does not
yet provide the following plan-owned deliverables:

| Pending boundary             | What is still required                                                                                                          |
| ---------------------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| amd64 development VM         | Guest image creation, VM lifecycle commands, provisioning, and a repeatable local development loop.                             |
| Local Headscale test network | Coordinator configuration, isolated policy, enrolment credentials, and a peer node that do not use the operator's real tailnet. |
| Tailscale integration checks | Guest `tailscaled` setup plus smoke coverage for enrolment, capability grants, service behaviour, and network isolation.        |
| arm64 pi-gen VM variant      | A bootable pi-gen-derived guest under system emulation, with its own provisioning and acceptance evidence.                      |
| Operator runbook             | Stable commands, troubleshooting guidance, and results from the required smoke checks.                                          |

The active plans define the sequencing, acceptance evidence, and phase status for those
deliverables. This document must not present CI emulation as a substitute for the planned local
environment, and it becomes an operator runbook only after the corresponding implementation and
acceptance evidence are delivered.
