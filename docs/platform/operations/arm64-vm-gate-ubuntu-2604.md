# ARM64 VM gate on ubuntu-26.04

This report covers why the ARM64 full-system release gate (`scripts/test-static-arm64-vm.sh`)
stopped booting when GitHub's `ubuntu-latest` runner image moved to ubuntu-26.04 on 20 October 2026. It records which part of the new runner is responsible and what fix it points to.

## Problem

The "Static build + smoke test" job in `.github/workflows/static-build-ci.yml` runs the gate on
`ubuntu-latest`. Since 14:45 UTC on 20 October 2026 the job has failed on `main` and on every pull
request, including PRs that touch nothing near it.

| Run                       | Runner image                  | Result                                |
| ------------------------- | ----------------------------- | ------------------------------------- |
| 39373875954 (main, 05:15) | ubuntu-24.04                  | passed                                |
| 39427720792 (PR, 13:07)   | ubuntu-24.04                  | passed                                |
| 39442305002, 39442217175  | ubuntu-26.04                  | failed                                |
| 39487502624 (PR 809)      | ubuntu-26.04 (20261012.163.1) | failed after 36 min waiting for `ssh` |

On the failed runs the QEMU log holds exactly one line, the edk2 banner
`UEFI firmware (version 2025.11-3ubuntu7.3 …)`. GRUB never appears. The script waits out its
`ssh` loop and exits with "VM did not become ready".

The two runner images install different packages for the same `apt-get install` line:

| Package            | ubuntu-24.04           | ubuntu-26.04           |
| ------------------ | ---------------------- | ---------------------- |
| `qemu-system-arm`  | 1:8.2.2+ds-0ubuntu1.18 | 1:10.2.1+ds-1ubuntu3.2 |
| `qemu-efi-aarch64` | 2024.02-2ubuntu0.10    | 2025.11-3ubuntu7.3     |

Both images ship `/usr/share/AAVMF/AAVMF_CODE.fd` as a symlink to the 64 MiB
`AAVMF_CODE.no-secboot.fd`, and the script finds it on both, so firmware discovery is not the
fault.

## Method

Each case boots the gate's pinned image, `debian-12-genericcloud-arm64-20260806-2562.qcow2`, with
the script's QEMU arguments: `-machine virt,accel=tcg -smp 2 -m 1536`, a qcow2 overlay on virtio,
and `virtio-net-pci` with user networking. Only the varied argument changes. QEMU and the firmware
come from the Ubuntu archive in `ubuntu:24.04` and `ubuntu:26.04` containers. Each case runs for
300 s (240 s for the first three) and counts as booting if GRUB and the kernel's
`Linux version` line appear in the serial log.

The containers ran on an Apple-silicon Mac (arm64 Linux under Docker Desktop), not on an amd64
GitHub runner. TCG emulates the guest CPU either way, so the guest code path is the same, but
timings are not comparable with CI. Three cases ran at a time.

## Findings

| Case | QEMU   | Firmware (edk2)         | `-cpu`        | Other             | Result                    |
| ---- | ------ | ----------------------- | ------------- | ----------------- | ------------------------- |
| 1    | 8.2.2  | 2024.02 (`AAVMF_CODE`)  | `max`         | —                 | boots to kernel (control) |
| 2    | 10.2.1 | 2025.11 (`AAVMF_CODE`)  | `max`         | `-bios`           | hangs after banner        |
| 3    | 10.2.1 | 2025.11 (`QEMU_EFI.fd`) | `max`         | `-bios`, 2 MiB fd | hangs after banner        |
| 4    | 10.2.1 | 2025.11 (`AAVMF`)       | `max`         | pflash CODE+VARS  | hangs after banner        |
| 5    | 10.2.1 | 2025.11 (`AAVMF_CODE`)  | `max`         | `-smp 1`          | hangs after banner        |
| 6    | 10.2.1 | 2025.11 (`AAVMF_CODE`)  | `max`         | no NIC            | hangs after banner        |
| 7    | 10.2.1 | 2024.02 (from 24.04)    | `max`         | —                 | boots to kernel           |
| 8    | 10.2.1 | 2025.11 (`AAVMF_CODE`)  | `cortex-a72`  | —                 | boots to kernel           |
| 9    | 10.2.1 | 2025.11 (`AAVMF_CODE`)  | `neoverse-n1` | —                 | boots to kernel           |

What the cases show:

- **The new QEMU on its own is not the fault.** Case 7 runs QEMU 10.2.1 with the 24.04 firmware
  and boots.
- **The firmware layout is not the fault.** Cases 2–4 try both firmware files, through `-bios` and
  through split pflash with a writable variable store, and all three hang the same way.
- **CPU count and the network ROM are not the fault.** Cases 5 and 6 still hang.
- **edk2 2025.11 hangs under `-cpu max`, and named CPU models boot.** Under cases 8 and 9 the same
  firmware reaches GRUB and the Debian 6.1 kernel (`6.1.0-52-cloud-arm64`).
  QEMU's `max` model turns on every architectural feature TCG implements. Some feature that QEMU
  10.2 adds to `max` stops edk2 2025.11 before it prints anything after the banner. These tests
  did not establish which feature it is.

A tenth case repeated case 9 alone, with no other VM running, and waited for the guest's console
`login:` prompt. The prompt appeared about 330 s after QEMU started. The script's `ssh` loop makes
180 tries, and in CI each try waits out its 10 s connect timeout before a 2 s sleep. The failed
runs spent about 36 minutes in that loop, so a boot of this length fits within the wait.

## Interpretation

The regression is the combination of `-cpu max` with the edk2 build Ubuntu 26.04 ships. QEMU's
version matters only because it defines what `max` contains. The narrowest fix keeps the runner
on `ubuntu-latest` and gives the script a named CPU in place of `max`. `neoverse-n1` is a good
choice: it is an Armv8.2 server core, close to the Raspberry Pi 5's Cortex-A76, and it boots under
both firmware generations.

[PR 814](https://github.com/banshee-data/velocity.report/pull/814) pinned the static-build job to
`ubuntu-24.04` as a stopgap. The pin did not cover `.github/workflows/build-image.yml`, which runs
the same script on `ubuntu-latest` in its "Cross-compile Go binaries (Linux ARM64)" job. The next
version tag would have hung there too.

## Applied fix

PR 818 makes the change this evidence points to:

- `scripts/test-static-arm64-vm.sh` boots the guest with `-cpu neoverse-n1` instead of `-cpu max`.
- The static-build job goes back to `ubuntu-latest`, which lifts the PR 814 pin.
- `build-image.yml` needs no edit: its ARM64 job calls the same script, so it picks up the CPU
  change.

The static-build job on PR 818 is the verification: it runs the whole gate under the named CPU on
an ubuntu-26.04 runner. That covers systemd privileges, live libpcap capture, API and database
persistence, upgrade and rollback. The release workflow runs only on tags and manual dispatch, so
no PR exercises it directly. It shares the script and the runner image with the static-build job.

## Limitations

- The local cases ran on arm64 Linux under Docker Desktop, not on an amd64 GitHub runner. The CI
  run on PR 818 is the check that the named CPU works there too.
- The `max` feature that edk2 2025.11 trips over was not identified, so a later QEMU or edk2 update
  may change the result.
- Each local case ran once.

## Recommendations

1. Leave `-cpu max` out of the gate. If a guest feature is needed later, add it to a named model
   rather than going back to `max`.
2. When the runner image next changes edk2 or QEMU, check the gate's first `main` run on that
   image before assuming a failure belongs to the PR it runs on.

## Provenance

- Failing CI runs: 39442305002, 39442217175 (main), 39487502624 (PR 809). Last passing on 24.04: 39373875954 (last on `main`, 05:15 UTC) and 39427720792 (a pull request, 13:07 UTC).
- Runner image: ubuntu-26.04 20261012.163.1, kernel 7.0.0-1017-azure.
- Guest image: Debian 12 genericcloud arm64 build 20260806-2562, the SHA-512 pinned in the script.
- Local harness: Docker `ubuntu:24.04` and `ubuntu:26.04` images (digest
  `sha256:c098726dd9b9733f3e885f8de0ef375206a311b0a317eadc40d347b1115fb944` for 26.04), with
  `qemu-system-arm qemu-efi-aarch64 qemu-utils ipxe-qemu` installed. Run on 20 October 2026.
  The serial logs were scratch output and are not kept.
