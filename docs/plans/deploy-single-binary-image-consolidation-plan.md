# Single-binary image consolidation

- **Status:** Active; all named consolidation work units have landed; only the image `:80` smoke-test gate remains outstanding
- **Layers:** Cross-cutting (Go binary, image build, systemd, PDF pipeline, Tailscale, sudoers)
- **Target:** v0.5.1 (single primary binary cutover, embedded Typst, read-only SQL path, static Linux image/release binary route, static Tailscale opt-in, and runtime apt manifest trimmed to `raspi-config`); deliberately ahead of v0.6.0 wide release so the public install path is "one primary binary, one image, one update command" before we hit a wider audience.
- **Companion plans:** [deploy-versioned-binary-plan.md](deploy-versioned-binary-plan.md), [deploy-nginx-removal-plan.md](deploy-nginx-removal-plan.md), [deploy-distribution-packaging-plan.md](deploy-distribution-packaging-plan.md), [cli-restructuring-plan.md](cli-restructuring-plan.md), [deploy-rpi-imager-fork-plan.md](deploy-rpi-imager-fork-plan.md), [binary-size-reduction-plan.md](binary-size-reduction-plan.md), [platform-simplification-and-deprecation-plan.md](platform-simplification-and-deprecation-plan.md), [pdf-latex-precompiled-format-plan.md](pdf-latex-precompiled-format-plan.md)
- **Canonical:** [distribution-packaging.md](../platform/operations/distribution-packaging.md)
- **Supersedes:** the "fold sweep / ctl later" sequencing in [deploy-versioned-binary-plan.md](deploy-versioned-binary-plan.md); the texlive trimming work in [pdf-latex-precompiled-format-plan.md](pdf-latex-precompiled-format-plan.md); the Phase 2 ".fmt precompile" goal in [deploy-rpi-imager-fork-plan.md](deploy-rpi-imager-fork-plan.md) § Phase 2 — replaced wholesale by removing xelatex.

---

## Motivation

The velocity.report Pi image started this cycle as a mixed Debian + Go + apt + bash + python tooling stack. Five things made updates fragile and the image fat:

1. Two Go binaries shipped to `/usr/local/bin` (`velocity-report`, `velocity-ctl`) plus a redirect stub (`velocity-update`), all sharing one Go runtime and embedded web build.
2. A 143 MB vendored TeX Live tree extracted at image-build time from ~1 GB of apt packages that are then purged. xelatex is the only reason it exists.
3. An apt repo and a Debian-codename-conditional shell script just to install Tailscale, which then sits masked until the operator opts in via the web UI.
4. nginx + a self-signed TLS oneshot, present only to terminate TLS on `:443` — already slated for removal in [deploy-nginx-removal-plan.md](deploy-nginx-removal-plan.md).
5. A scatter of single-purpose apt packages (`librsvg2-bin`, `fonts-noto-color-emoji`, `python3-serial`, `minicom`, `jq`) that exist only because legacy code or stage scripts call out to them.

Each of these widens the public install surface, the upgrade surface, and the security surface. Each one is also independently removable: nothing in this plan requires solving all five at once. The single goal is **make the deployment look, from the user's side, like one binary, one image, one update command** before v0.6.0 ships the public install path more widely.

If we do not do this before wide release, every one of these surfaces becomes a public compatibility commitment that is much harder to walk back.

## Current state

### Image layout (current branch)

Stage scripts under [image/stage-velocity/](../../image/stage-velocity/):

| Stage                  | Purpose                                                                                  | apt packages installed |
| ---------------------- | ---------------------------------------------------------------------------------------- | ---------------------- |
| `00-install-packages`  | install baseline runtime package surface                                                 | `raspi-config`         |
| `01-velocity-binaries` | install the staged static ARM64 multi-call `velocity` binary into `/opt/velocity-report` | (none)                 |
| `03-velocity-config`   | systemd, scoped sudoers, aliases, MOTD, UART/SPI overlay, udev install, public docs copy | (none)                 |
| `04-velocity-lidar`    | static IP for LiDAR subnet via embedded `velocity device install network` payload        | (none)                 |
| `05-velocity-wifi`     | regulatory domain fallback via embedded `velocity device install wifi` payload           | (none)                 |
| `06-cleanup`           | purge dev/compiler/desktop/camera/X11/python-dev packages and package-manager cache      | (purges, no installs)  |
| `07-networking`        | finalise NetworkManager defaults                                                         | (none)                 |

The binary surface:

- `cmd/velocity` builds one multi-call binary named `velocity`.
- `/usr/local/bin/velocity` is the canonical CLI entry point.
- `/usr/local/bin/velocity-report` remains a compatibility alias for the server-oriented default.
- The old standalone `velocity-ctl` binary and `velocity-update` redirect stub are removed; operator lifecycle is `velocity device ...`.
- The `tune`, `data`, `report`, `serve`, `version`, and `device` namespaces live in the same binary.

PDF pipeline:

- Report generation uses the Go + Typst path only. The old xelatex/TeX tree,
  rsvg, and report-compiler package surface are removed from the image build.
- Typst is embedded in the Go binary and validated by the server self-check.

Tailscale lifecycle:

- The image has no Tailscale package, repository, daemon service, or state.
- `velocity device tailscale install` downloads a pinned static tarball only
  after the web-UI opt-in, verifies its baked SHA-256, extracts it under
  `/opt/velocity-report/tailscale/<version>/`, records install metadata, and
  writes the binary-owned `tailscaled.service` before the existing narrow
  sudo bridge enables it.
- `internal/tailscale` drives login URL, `tailscale set --operator=velocity`, and `tailscale serve` against `http://127.0.0.1:80` once the daemon is up.

Build toolchain:

- Static Linux image/release binaries are built through
  [scripts/build-radar-static.sh](../../scripts/build-radar-static.sh), using
  the hermetic Docker toolchain in
  [image/Dockerfile.static-build](../../image/Dockerfile.static-build).
- Docker installs pinned Go and zig toolchains, targets musl, and builds
  `libpcap.a` from the vendored [third_party/libpcap](../../third_party/libpcap)
  submodule.
- Image staging uses [scripts/stage-image-binary.sh](../../scripts/stage-image-binary.sh)
  and [scripts/verify-static-elf.sh](../../scripts/verify-static-elf.sh) so a
  dynamic `libpcap.so` dependency cannot enter the image by accident.

## Findings

| Area                                         | Current state                                                                                                                        | Release view                                                                                    |
| -------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------- |
| Single production Go binary                  | Landed. `velocity` owns `serve`, `device`, `data`, `report`, `tune`, and `version`; `velocity-report` is only a compatibility alias. | Keep one promoted binary surface.                                                               |
| Static Linux image/release build             | Landed. Docker + zig/musl builds linux/{amd64,arm64}; vendored `libpcap.a` is linked statically and verified before staging.         | Image and release artifacts must continue through the shared static route.                      |
| `xelatex` + TeX tree                         | Removed. Reports use Go + embedded Typst; no TeX tree or rsvg package surface ships for reporting.                                   | Treat any reintroduction of TeX/image report packages as a release blocker.                     |
| Tailscale install                            | Landed. A version- and checksum-pinned static payload is fetched only after opt-in; image build has no Tailscale apt stage or state. | Keep static payload tuples explicit and test each supported Pi OS release before broad release. |
| `nginx` + self-signed TLS                    | Removed. Go binds `:80` directly with `CAP_NET_BIND_SERVICE`; HTTPS is a Tailscale Serve opt-in.                                     | Keep.                                                                                           |
| `python3-serial`, `minicom`, `jq`, `sqlite3` | Removed from the project apt manifest. SQLite inspection is `velocity data sql --read-only`.                                         | Keep diagnostics inside the supported binary surface.                                           |
| Shell lifecycle aliases                      | `velocity-status`, `velocity-log`, `velocity-start`, `velocity-stop`, `velocity-bounce` remain.                                      | Keep. These are host lifecycle wrappers, not application namespaces.                            |
| Embedded tuning/network/udev/wifi defaults   | Landed via `go:embed` plus `velocity device install ...`.                                                                            | Keep installer payloads binary-owned.                                                           |

## Design / approach

This plan is one direction of travel with five named work units. Each is independently shippable and independently reversible. All five work units have landed.

**Direction of travel.** The Pi image becomes, at runtime:

```
runtime artifact     who owns it
-------------------  -------------------------------------
/opt/velocity-report/versions/<v>/velocity  single Go binary, multi-call
/etc/systemd/system/velocity.service   service unit
/etc/profile.d/velocity-aliases.sh     5 shell wrappers
/var/lib/velocity-report/sensor_data.db  SQLite WAL
/etc/sudoers.d/020_velocity-nopasswd   3 lines: systemctl + tailscaled bridge
```

No xelatex tree. No nginx. No `velocity-ctl`, no `velocity-update`, no vendored Typst tree, no `sqlite3` dependency for routine inspection, no Tailscale apt package, and no runtime `libpcap0.8` dependency from the application binary. The repo-level `00-packages` manifest is down to `raspi-config`; the opt-in Tailscale payload is binary-owned. Goal end-state for v0.6.0 is "the Pi image is Pi OS Lite + one primary Go binary plus only the binary-owned helper payloads needed to avoid runtime dependency drift."

**Compatibility contract.** `velocity-report` survives as the server-oriented alias the systemd unit can call; `velocity-ctl` and `velocity-update` are removed. All other removed surfaces are deleted, not deprecated.

### Work unit A: fold `velocity-ctl` and operator tools into one binary (v0.5.1) `M`

This is the "submodule move" the user wants pulled forward — the same direction already endorsed in [deploy-versioned-binary-plan.md](deploy-versioned-binary-plan.md), just sequenced into v0.5.1 instead of waiting for v0.6.0.

**Steps:**

1. Move `internal/cmd/server/` → `cmd/velocity/`; rename the existing files to live under `internal/cmd/server/` and export `Main(args []string)`. Multi-call dispatcher in `cmd/velocity/main.go` switches on `filepath.Base(os.Args[0])` first, then `os.Args[1]`.
2. Move `internal/cmd/device/` source under `internal/cmd/device/` and wire as the `device` namespace: `velocity device check|upgrade|rollback|backup`.
3. Ship `internal/cmd/device` as a thin redirect shim for one release: when invoked, print a deprecation warning and exec the new binary with `device` prefixed. Delete the shim in v0.5.2.
4. Move `internal/cmd/tune/` under `internal/cmd/tune/` and expose as `velocity tune sweep`. Operator-facing utilities in `cmd/tools/*` stay developer-only until [platform-simplification-and-deprecation-plan.md](platform-simplification-and-deprecation-plan.md) ratifies their promotion.
5. Update [image/stage-velocity/01-velocity-binaries/00-run.sh](../../image/stage-velocity/01-velocity-binaries/00-run.sh) to install one binary at `/opt/velocity-report/versions/<v>/velocity`; create `/usr/local/bin/velocity` and `/usr/local/bin/velocity-report` symlinks; delete `velocity-update`.
6. Tighten `/etc/sudoers.d/020_velocity-nopasswd` so `pi`'s `velocity-ctl *` grant collapses to `velocity device *`; the `velocity` user's tailscale bridge becomes `velocity device tailscale {enable,disable}-tailscaled` (literal argv, no wildcards).

**Milestone:** v0.5.1. Tested on hardware before image cut.

### Work unit B: in-binary Tailscale installer (v0.5.1) `M` ✅

Today the apt-based install runs at image-build time regardless of operator intent. Replace it with a runtime static-binary install, gated on the existing web-UI opt-in:

This is an accepted helper payload, not a second promoted application surface. The public operator contract remains `velocity device tailscale ...`; the downloaded Tailscale binaries are implementation detail owned, versioned, and validated by the main binary.

**Steps:**

1. Delete [image/stage-velocity/07-velocity-tailscale/](../../image/stage-velocity/07-velocity-tailscale/) wholesale. The image ships with no Tailscale state.
2. Add `velocity device tailscale install` to the binary. It:
   - Detects CPU architecture and supported distro family from the running host.
   - Downloads the pinned static-binary tarball from `pkgs.tailscale.com/stable/tailscale_<arch>.tgz` using Go's `net/http` with SHA-256 verification against a manifest baked into the binary.
   - Extracts `tailscale` and `tailscaled` under `/opt/velocity-report/tailscale/<version>/` and refreshes stable symlinks in `/opt/velocity-report/tailscale/current/`.
   - Installs or refreshes the systemd service wiring so the daemon is started from the extracted binary rather than from an apt package.
   - Is idempotent: a second call is a no-op when the requested version is already present and healthy.
3. The web-UI Tailscale flow becomes: (a) install → (b) unmask/enable/start → (c) interactive login URL → (d) `tailscale set --operator=velocity` → (e) `tailscale serve`. Steps (b)–(e) are unchanged from the existing `internal/tailscale` flow.
4. Add the narrow sudoers/systemd hooks needed for the service user to install the static Tailscale payload and manage `tailscaled` through the binary-owned wrapper, with literal argv and no wildcards.
5. Record the selected Tailscale version in the binary-owned install metadata so upgrades and rollbacks can reason about the downloaded artefact without involving apt state.

**Milestone:** v0.5.1. Implementation and focused unit coverage landed; physical Pi OS bookworm/trixie validation remains a release gate.

### Work unit C: replace xelatex with Typst (v0.5.1) `L` ✅

**Why now.** xelatex is the load-bearing reason for ~30% of the apt surface on the image and most of the PDF-side complexity. Typst is a single static binary, ~30 MB, with native SVG support and a Go-callable invocation pattern that mirrors what we already do with xelatex.

There is no production Go library binding to Typst's Rust crate; the credible path is one of:

| Option                                                                                   | Surface                                                                                  | Where the work lives                 | Effort |
| ---------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------- | ------------------------------------ | ------ |
| Embed `typst-cli` ARM64 binary via `go:embed`; extract to a temp dir at first PDF render | One artifact; release pipeline gains a fetch-and-pin step for the upstream Typst release | Release pipeline + `internal/report` | `M`    |
| Vendor the `typst-cli` binary into `/opt/velocity-report/typst/` via the image stage     | Image stage owns the artifact, not the binary                                            | Image stage + `internal/report`      | `S`    |
| Wrap `typst` Rust crate via CGo                                                          | Single artifact, but introduces Rust build-time and CGo on the cross-compile path        | Build system + new wrapper           | `L`    |

**Recommendation.** Land the `go:embed` path (option 1) in v0.5.1 so Typst becomes part of the self-contained application contract immediately, even if that makes the shipped binary materially larger. Keep `< 40 MB` as the default binary-size budget and CI/release gate described in the companion size-reduction work; for this Typst cutover, track the measured size in CI and release notes, and if the embedded binary cannot yet meet that budget, require an explicit temporary exception/waiver rather than silently treating the ceiling as non-blocking. Keep vendor-into-image (option 2) only as a contingency fallback if a target-specific build becomes operationally unshippable. CGo (option 3) is not worth the build-system tax.

**Editable source archive contract.** The current LaTeX source ZIP is a real product feature, not just a build artefact. The Typst cutover therefore replaces it with an equivalent editable Typst source archive rather than dropping source export entirely. The supported archive becomes `report.typ` + assets + fonts + a concise README for educated operators who want to edit and recompile the report outside the product.

**Steps:**

1. Translate `internal/report/tex/templates/*.tex` → `internal/report/typst/templates/*.typ`. Preserve the existing Go `text/template` boundary; only the target syntax changes. Period report, overview, chart, and map sections each port independently.
2. Replace `runXeLatex` in [internal/report/report.go](../../internal/report/report.go) with `runTypst`. Single-pass compile (Typst handles cross-references natively in one run). The default runtime path extracts the embedded Typst binary to a versioned cache dir on first use; the system Typst override remains for local development.
3. Replace the exported source ZIP contract: emit `report.typ` instead of `report.tex`, keep editable chart/source assets, and rewrite the archive README around Typst compile/edit steps. The archive should remain usable by an educated operator without product internals knowledge.
4. Ship a documented ZIP migration path for one release: release notes + setup/docs call out that LaTeX source bundles are replaced by Typst source bundles, include a short "LaTeX to Typst" operator note, and explain the new compile command plus any asset-layout changes.
5. Native SVG: Typst renders SVG directly, so `\includesvg` + `rsvg-convert` shell-outs delete with the manifest.
6. Image stage: rewrite [image/stage-velocity/00-install-packages/](../../image/stage-velocity/00-install-packages/) — drop `texlive-xetex`, `texlive-latex-extra`, `fonts-lmodern`, `librsvg2-bin`, `fonts-noto-color-emoji`; stop vendoring Typst under `/opt/velocity-report/typst/`; delete `scripts/build-minimal-texlive.sh` and `scripts/install-minimal-texlive.sh`.
7. Delete the `Phase 2 .fmt precompile` work from [deploy-rpi-imager-fork-plan.md](deploy-rpi-imager-fork-plan.md) § Phase 2 and [pdf-latex-precompiled-format-plan.md](pdf-latex-precompiled-format-plan.md). The whole problem disappears.
8. CI: add a "PDF parity" job that renders a fixed report.json through both the old (xelatex) and new (Typst) pipelines for one release before deleting the xelatex path; compare page count, dominant glyph fingerprints, chart bounding boxes, and source-archive completeness. Delete the xelatex path after one release of co-existence.

**Milestone:** v0.5.1. Landed; xelatex, report compiler packages, minimal-TeX scripts, and the old parity window are removed.

### Work unit D: pull nginx removal into v0.5.1 (instead of v0.6.0) `S`

This is just sequencing — the design is already done in [deploy-nginx-removal-plan.md](deploy-nginx-removal-plan.md). Bring it forward so the public install path the v0.6.0 release announces is `http://velocity.local` with no self-signed CA dance.

**Steps:**

1. Land [deploy-nginx-removal-plan.md](deploy-nginx-removal-plan.md) work items in v0.5.1: bind Go on `:80` via `AmbientCapabilities=CAP_NET_BIND_SERVICE`; delete the nginx site, the TLS oneshot script, and the cert directory; update the MOTD URL.
2. Update [docs/platform/operations/tls-local-certificates.md](../platform/operations/tls-local-certificates.md) to point to the Tailscale Serve story for HTTPS.

**Milestone:** v0.5.1.

### Work unit E: image apt-surface trim (v0.5.1) `S`

Once C and D land, the apt package list in [image/stage-velocity/00-install-packages/00-packages](../../image/stage-velocity/00-install-packages/00-packages) becomes:

```
raspi-config       # serial port / UART config
```

Everything else either disappears with C/D or moves into the binary via B.

**Steps:**

1. Delete `nginx`, `librsvg2-bin`, `fonts-noto-color-emoji`, `texlive-xetex`, `texlive-latex-extra`, `fonts-lmodern`, `python3-serial`, `minicom`, `jq`, `curl` from `00-packages`.
2. Add `velocity data sql --read-only` as the supported operator inspection path and drop the `sqlite3` apt dependency from the shipped image. Keep the subcommand intentionally narrow: read-only queries, explicit file target controls, and row/output limits suitable for field diagnostics.
3. Embed `tuning.defaults.json` into the binary via `go:embed`. Operator override still works via the existing config flag.
4. Embed `lidar-network.conf` + udev rules into the binary; expose `velocity device install <component>` (`network`, `udev`, `motd`) so the image stage scripts collapse to a single dispatcher call per stage. This makes the image stage scripts shrink to ~10 lines each.
5. Delete `image/stage-velocity/02-velocity-python/` entirely (the directory name lies — no Python ships any more; the output directory creation moves into the binary's first-boot init).

**Milestone:** v0.5.1.

## What else in the rpi image can collapse into the binary

Inventory of every artifact that is not the binary itself, with effort to fold or remove. Items already covered above are marked with their work unit.

| Artifact                                                          | Today                                                                                                                        | Future state                                                                                                                                                | Effort  |
| ----------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------- | ------- |
| `velocity-ctl`                                                    | Removed.                                                                                                                     | `velocity device ...` namespace inside the multi-call binary.                                                                                               | Done    |
| `velocity-update` redirect stub                                   | Shell script at `/usr/local/bin/velocity-update`.                                                                            | Deleted.                                                                                                                                                    | A (`S`) |
| `internal/cmd/tune/`                                              | Local-dev binary; not shipped.                                                                                               | `velocity tune sweep`; shipped inside the binary.                                                                                                           | A (`S`) |
| Tailscale apt repo + install                                      | Removed from the image.                                                                                                      | Static-binary installer inside `velocity device tailscale install`; helper payload owned and versioned by the main binary; deferred until operator opts in. | Done    |
| `tailscaled` mask state                                           | No image-time service or state.                                                                                              | Service wiring is written by the binary at install time and then enabled through the narrow bridge.                                                         | Done    |
| `texlive-xetex` + minimal TeX tree                                | Removed.                                                                                                                     | Deleted; Typst replaces.                                                                                                                                    | Done    |
| `librsvg2-bin` + `fonts-noto-color-emoji` + `fonts-lmodern`       | Removed.                                                                                                                     | Deleted.                                                                                                                                                    | Done    |
| `scripts/build-minimal-texlive.sh` + `install-minimal-texlive.sh` | Removed.                                                                                                                     | Deleted.                                                                                                                                                    | Done    |
| `nginx` + site + TLS oneshot                                      | Reverse proxy for `:443`.                                                                                                    | Deleted; Go binds `:80` directly.                                                                                                                           | D (`S`) |
| `velocity-generate-tls.sh` + service                              | Self-signed cert generation oneshot.                                                                                         | Deleted.                                                                                                                                                    | D (`S`) |
| `tuning.defaults.json`                                            | File at `/opt/velocity-report/config/`.                                                                                      | `go:embed` into the binary; operator override via existing config flag.                                                                                     | E (`S`) |
| `lidar-network.conf`                                              | `/etc/network/interfaces.d/lidar`.                                                                                           | `go:embed` + `velocity device install network`.                                                                                                             | E (`S`) |
| `99-velocity-report.rules` (udev)                                 | `/etc/udev/rules.d/99-velocity-report.rules`.                                                                                | `go:embed` + `velocity device install udev`.                                                                                                                | E (`S`) |
| `wpa_supplicant.conf` fallback                                    | `/etc/wpa_supplicant/wpa_supplicant.conf`.                                                                                   | `go:embed` + `velocity device install wifi`.                                                                                                                | E (`S`) |
| `velocity-aliases.sh` (host lifecycle wrappers)                   | `/etc/profile.d/velocity-aliases.sh`.                                                                                        | Stays — host lifecycle is not a binary concern per [deploy-versioned-binary-plan.md](deploy-versioned-binary-plan.md).                                      | (none)  |
| `velocity-motd.sh` + `velocity-report-build`                      | MOTD shell script + build stamp.                                                                                             | Stays; embed the build stamp into the binary and have the MOTD read it from `velocity version`.                                                             | E (`S`) |
| `sudoers.d/020_velocity-nopasswd`                                 | Grants enumerated `pi → velocity device ...` and `velocity → velocity device tailscale {install,enable,disable}-tailscaled`. | Literal static-installer and lifecycle bridge.                                                                                                              | Done    |
| UART/SPI overlay edits to `/boot/firmware/config.txt`             | Direct file edits in stage script.                                                                                           | Stays in the image stage; this is firmware-boot config, not a runtime concern.                                                                              | (none)  |
| `raspi-config`                                                    | apt package; used for serial port enable.                                                                                    | Stays.                                                                                                                                                      | (none)  |
| `libpcap0.8`                                                      | Previously an apt package runtime dep of the Go binary.                                                                      | Deleted from the image; the image binary uses vendored static libpcap.                                                                                      | E (`S`) |
| `python3-serial`, `minicom`                                       | apt packages; debugging only.                                                                                                | Deleted.                                                                                                                                                    | E (`S`) |
| `jq`, `curl`                                                      | apt packages; build-time only.                                                                                               | Deleted from the image; remain in CI.                                                                                                                       | E (`S`) |
| `sqlite3`                                                         | apt package; operator convenience.                                                                                           | Replaced by `velocity data sql --read-only`; removed from the shipped image once the subcommand lands.                                                      | E (`S`) |
| `os-list-velocity.json`                                           | Custom rpi-imager catalog entry.                                                                                             | Stays.                                                                                                                                                      | (none)  |
| Reports output dir creation                                       | Stage script `02-velocity-python/00-run.sh`.                                                                                 | First-boot init inside the binary.                                                                                                                          | E (`S`) |

End-state apt manifest (target for v0.5.1): **`raspi-config`** plus the base Pi OS Lite packages. The primary velocity-owned artefact at `/opt/velocity-report/` is the `velocity` binary; any extracted Typst runtime cache or Tailscale payload is subordinate, binary-owned implementation detail rather than a separate public surface.

## Scope

### Item 1: fold `velocity-ctl`, sweep, and update stub into one binary

**Summary:** One binary `/opt/velocity-report/versions/<v>/velocity`, with `device`, `serve`, `tune`, `data`, `report`, `version`, `help` namespaces; `velocity-report` survives as the systemd-facing alias; `velocity-ctl` and `velocity-update` are deleted.

**Steps:**

1. Move and rename per work unit A above.
2. Update systemd unit to call `velocity` (or `velocity-report` symlink — same binary).
3. Update sudoers, MOTD, docs to speak the new command surface.
4. Compatibility parity window is closed; invoke `velocity device ...` directly.

**Milestone:** v0.5.1.

### Item 2: in-binary Tailscale installer ✅

**Summary:** Image ships zero Tailscale state. The web UI's "Enable Tailscale" button triggers `velocity device tailscale install` → `enable` → `login` → `set --operator` → `serve`.

**Steps:** see work unit B.

**Milestone:** v0.5.1.

### Item 3: xelatex → Typst

**Summary:** Replace the LaTeX pipeline with Typst. Delete the 143 MB TeX tree, the minimal-texlive build scripts, and the rsvg/emoji apt deps. PDF output is preserved within parity tolerances, and the editable source ZIP becomes a Typst source archive rather than disappearing.

**Steps:** see work unit C.

**Milestone:** v0.5.1. Landed; xelatex path deleted.

### Item 4: nginx + self-signed TLS removal (pulled forward)

**Summary:** Bind Go on `:80`. Delete nginx site, TLS cert script, and oneshot service. HTTPS becomes a Tailscale opt-in.

**Steps:** see work unit D and [deploy-nginx-removal-plan.md](deploy-nginx-removal-plan.md).

**Milestone:** v0.5.1.

### Item 5: image apt-surface trim + `go:embed` config

**Summary:** Drop all apt packages that exist only because of xelatex, Tailscale apt-install, or nginx. Embed tuning defaults, network config, udev rules, and wpa_supplicant fallback into the binary.

**Steps:** see work unit E.

**Milestone:** v0.5.1.

## Dependencies

- Work unit A (`velocity-ctl` fold) has landed.
- Work unit B (Tailscale installer) landed on the `device` namespace.
- Work unit C (Typst) has landed.
- Work unit D (nginx removal) is independent and can land first inside v0.5.1.
- Work unit E (apt-surface trim) depends on C and D; lands in v0.5.1 once the binary-owned SQL and Typst paths are in place.

## Risks

| Risk                                                                                                              | Likelihood | Impact | Mitigation                                                                                                                                                                                                              |
| ----------------------------------------------------------------------------------------------------------------- | ---------- | ------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Typst output parity diverges from legacy reports on edge cases (kerning, glyph fallback)                          | Low        | Medium | Keep report regression fixtures and hardware release checks focused on generated PDF success and source archive completeness.                                                                                           |
| In-binary Tailscale installer fails on an unsupported architecture or leaves us on an untested distro combination | Medium     | Medium | Pin supported archive tuples in the baked manifest, fail loudly on unknown host combinations, and validate the static install flow on the release-supported Pi OS variants before cut.                                  |
| Operators try old `velocity-ctl` commands from stale docs                                                         | Low        | Low    | Current docs use `velocity device ...`; release notes call out the removed shim.                                                                                                                                        |
| nginx removal lands without `:80` binding capability set                                                          | Low        | High   | `AmbientCapabilities=CAP_NET_BIND_SERVICE` is set in the systemd unit at the same commit; image-build CI smoke-tests the bind on a chroot before image export.                                                          |
| Binary size rises materially once Typst and helper payload logic are embedded                                     | High       | Low    | Measure and publish size by target in CI, but treat size as a tradeoff metric rather than a hard release gate for this plan; optimise obvious waste without reintroducing external runtime dependencies.                |
| Static-binary Tailscale install fails mid-download or leaves partial state on disk                                | Low        | Medium | Download into a temp dir, verify checksum before activation, and only switch the stable symlink after the payload passes validation.                                                                                    |
| Existing operator scripts or habits that expect `report.tex` inside the source ZIP break after the Typst cutover  | Medium     | Medium | Keep the source-archive feature, switch it deliberately to `report.typ`, document the archive layout change in release notes and setup docs, and include a short migration guide for users who previously edited LaTeX. |

## Checklist

### Complete

- [x] Plan written and circulated for review.
- [x] Work unit A: fold `velocity-ctl` and sweep into the multi-call binary and pull forward the **full** versioned dispatcher/upgrade machinery (`renameat2` swap, retention, single-artifact release.json). `velocity-update` was already removed (#290).
- [x] Work unit D: nginx removal landed (#517).
- [x] Work unit E: `go:embed` for tuning defaults, network config, udev rules, and wpa_supplicant + `velocity device install`; `velocity data sql --read-only` read-only inspection subcommand replacing `sqlite3`; dropped `python3-serial`, `minicom`, `jq`, `curl`, and `sqlite3` from the apt surface; deleted the `02-velocity-python` stage.
- [x] Removed the transitional `velocity-ctl` shim: the `/usr/local/bin/velocity-ctl` symlink (image stage 01), the `velocity-ctl` sudoers grants (stage 03), and the deprecation-warning path in `cmd/velocity/main.go`.
- [x] Docs: updated distribution-packaging, rpi-imager, setup, asset-naming, COMMANDS, CLAUDE, and coding-standards to the new surface.
- [x] Work unit C: Typst PDF pipeline; deleted `texlive-xetex` apt surface, minimal-texlive build scripts, old report compiler tree, and old parity window.
- [x] Source archive migration: replace LaTeX source ZIP output with the editable Typst source archive.
- [x] Static Linux build route: image/release/ad-hoc Linux artifacts share `scripts/build-radar-static.sh`; image staging verifies static ELF output through `scripts/stage-image-binary.sh` and `scripts/verify-static-elf.sh`.
- [x] Work unit B: pinned SHA-256-verified static Tailscale installer, versioned install metadata, binary-owned systemd service, and literal sudo bridge; deleted `image/stage-velocity/07-velocity-tailscale/`.

### Outstanding

- [ ] CI: image stage smoke test that the bind on `:80` works in a chroot before export (`S`)

### Deferred

- [ ] CGo binding to the Typst Rust crate — explicitly rejected; build-system tax is not worth it.

### Accepted residuals (no action planned)

- [ ] Host lifecycle aliases (`velocity-status`, `velocity-log`, `velocity-start`, `velocity-stop`, `velocity-bounce`) stay outside the binary. Host concerns are not application namespaces per [deploy-versioned-binary-plan.md](deploy-versioned-binary-plan.md).
- [ ] UART/SPI overlay edits to `/boot/firmware/config.txt` stay in the image stage script. Firmware-boot config is not a runtime concern.
- [ ] `raspi-config` survives into v0.6.0 for serial/UART configuration.
