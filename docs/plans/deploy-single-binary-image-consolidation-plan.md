# Single-binary image consolidation

- **Status:** Draft
- **Layers:** Cross-cutting (Go binary, image build, systemd, PDF pipeline, Tailscale, sudoers)
- **Target:** v0.5.1 (single primary binary cutover, static-binary Tailscale, embedded Typst, Typst source archive, read-only SQL path, and trim runtime apt deps down to `libpcap0.8` + `raspi-config`), v0.5.2 (remove the remaining apt purge/build-stage scaffolding and simplify stages); deliberately ahead of v0.6.0 wide release so the public install path is "one primary binary, one image, one update command" before we hit a wider audience.
- **Companion plans:** [deploy-versioned-binary-plan.md](deploy-versioned-binary-plan.md), [deploy-nginx-removal-plan.md](deploy-nginx-removal-plan.md), [deploy-distribution-packaging-plan.md](deploy-distribution-packaging-plan.md), [cli-restructuring-plan.md](cli-restructuring-plan.md), [deploy-rpi-imager-fork-plan.md](deploy-rpi-imager-fork-plan.md), [binary-size-reduction-plan.md](binary-size-reduction-plan.md), [platform-simplification-and-deprecation-plan.md](platform-simplification-and-deprecation-plan.md), [pdf-latex-precompiled-format-plan.md](pdf-latex-precompiled-format-plan.md)
- **Canonical:** [distribution-packaging.md](../platform/operations/distribution-packaging.md)
- **Supersedes:** the "fold sweep / ctl later" sequencing in [deploy-versioned-binary-plan.md](deploy-versioned-binary-plan.md); the texlive trimming work in [pdf-latex-precompiled-format-plan.md](pdf-latex-precompiled-format-plan.md); the Phase 2 ".fmt precompile" goal in [deploy-rpi-imager-fork-plan.md](deploy-rpi-imager-fork-plan.md) § Phase 2 — replaced wholesale by removing xelatex.

---

## Motivation

The velocity.report Pi image today is a mixed Debian + Go + apt + bash + python tooling stack. Five things make updates fragile and the image fat:

1. Two Go binaries shipped to `/usr/local/bin` (`velocity-report`, `velocity-ctl`) plus a redirect stub (`velocity-update`), all sharing one Go runtime and embedded web build.
2. A 143 MB vendored TeX Live tree extracted at image-build time from ~1 GB of apt packages that are then purged. xelatex is the only reason it exists.
3. An apt repo and a Debian-codename-conditional shell script just to install Tailscale, which then sits masked until the operator opts in via the web UI.
4. nginx + a self-signed TLS oneshot, present only to terminate TLS on `:443` — already slated for removal in [deploy-nginx-removal-plan.md](deploy-nginx-removal-plan.md).
5. A scatter of single-purpose apt packages (`librsvg2-bin`, `fonts-noto-color-emoji`, `python3-serial`, `minicom`, `jq`) that exist only because legacy code or stage scripts call out to them.

Each of these widens the public install surface, the upgrade surface, and the security surface. Each one is also independently removable: nothing in this plan requires solving all five at once. The single goal is **make the deployment look, from the user's side, like one binary, one image, one update command** before v0.6.0 ships the public install path more widely.

If we do not do this before wide release, every one of these surfaces becomes a public compatibility commitment that is much harder to walk back.

## Current state

### Image layout (2026-05)

Stage scripts under [image/stage-velocity/](../../image/stage-velocity/):

| Stage                   | Purpose                                                                                     | apt packages installed                                                                                                                                                                         |
| ----------------------- | ------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `00-install-packages`   | install apt deps; build minimal TeX tree; purge apt TeX                                     | `nginx`, `librsvg2-bin`, `fonts-noto-color-emoji`, `texlive-xetex`, `texlive-latex-extra`, `fonts-lmodern`, `libpcap0.8`, `raspi-config`, `python3-serial`, `minicom`, `sqlite3`, `jq`, `curl` |
| `01-velocity-binaries`  | install Go binaries                                                                         | (none) — installs `velocity-report`, `velocity-ctl`, `velocity-update`                                                                                                                         |
| `02-velocity-python`    | create report output dir (legacy name; no python ships)                                     | (none)                                                                                                                                                                                         |
| `03-velocity-config`    | systemd, sudoers, aliases, MOTD, nginx site, TLS cert oneshot, UART/SPI overlay, udev rules | (none)                                                                                                                                                                                         |
| `04-velocity-lidar`     | static IP for LiDAR subnet                                                                  | (none)                                                                                                                                                                                         |
| `05-velocity-wifi`      | regulatory domain fallback                                                                  | (none)                                                                                                                                                                                         |
| `06-cleanup`            | purge dev/compiler/desktop/camera/X11/python-dev packages                                   | (purges, no installs)                                                                                                                                                                          |
| `07-networking`         | finalise NetworkManager defaults                                                            | (none)                                                                                                                                                                                         |
| `07-velocity-tailscale` | add Tailscale apt repo, install `tailscale`, mask the daemon                                | `tailscale`                                                                                                                                                                                    |

The two-binary surface:

- [internal/cmd/server/](../../internal/cmd/server) — server binary, installed as `velocity-report`. Defaults to `serve`; subcommands `migrate`, `pdf`, `transits`.
- [internal/cmd/device/](../../internal/cmd/device) — operator binary, installed as `velocity-ctl`. Subcommands `upgrade`, `rollback`, `backup`, `status`, `tailscale`.
- [internal/cmd/tune/](../../internal/cmd/tune) — sweep harness, only ever built locally; not shipped to the Pi today.

PDF pipeline:

- `internal/report/report.go` shells out to `xelatex` two passes. Needs `/opt/velocity-report/texlive/` (143 MB) or system TeX Live.
- `internal/report/tex/templates/` holds Go `text/template`-driven `.tex` sources. Output is a single PDF assembled with vendored Latin Modern, Noto Color Emoji, and SVG inclusion via `\includesvg` and `rsvg-convert`.

Tailscale lifecycle:

- `image/stage-velocity/07-velocity-tailscale/01-run.sh` adds `pkgs.tailscale.com/stable/debian/<codename>` to apt sources and `apt install tailscale`. Daemon is then `systemctl mask`-ed.
- `velocity-ctl tailscale enable-tailscaled` (called via a narrow sudoers grant by the Go server) unmasks, enables, and starts the daemon when the operator opts in via the web UI.
- `internal/tailscale` drives login URL, `tailscale set --operator=velocity`, and `tailscale serve` against `http://127.0.0.1:8080` once the daemon is up.

## Findings

| Area                                                                          | Current state                                                                                                                                                                             | Severity | Release view                                                                                                                                                    |
| ----------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Two production Go binaries                                                    | `velocity-report` + `velocity-ctl` share runtime, web embed, version metadata; ship as separate artifacts.                                                                                | High     | Must fold into one before v0.6.0 wide release; otherwise the upgrade surface promises both binaries.                                                            |
| `xelatex` + 143 MB TeX tree                                                   | Required only because the PDF pipeline writes `.tex`. xelatex run cost is ~2–4 s/report on a Pi 4.                                                                                        | High     | Typst is a single statically-linked binary, ~30 MB, native SVG, no external font matrix. Removing TeX removes the largest non-Go surface on the image.          |
| Tailscale apt install at image-build                                          | Apt repo, codename detection, GPG keyring, install, mask. Image carries `tailscale` even for operators who never opt in.                                                                  | Medium   | Move the install to the moment the operator opts in. The image then ships zero Tailscale state.                                                                 |
| `nginx` + self-signed TLS                                                     | Only purpose is TLS termination on `:443`. Browser warning UX is the worst dialog in the project.                                                                                         | Medium   | Already covered by [deploy-nginx-removal-plan.md](deploy-nginx-removal-plan.md); pulled forward into this plan so the v0.5.1 image is the one we ship publicly. |
| `librsvg2-bin`, `fonts-noto-color-emoji`, `fonts-lmodern`, `tipa`, `tex-gyre` | Only needed by the xelatex pipeline (SVG inclusion + map emoji).                                                                                                                          | Medium   | Disappear with xelatex.                                                                                                                                         |
| `python3-serial`, `minicom`                                                   | Debugging only. Never invoked by the service.                                                                                                                                             | Low      | Drop with no replacement.                                                                                                                                       |
| `jq`                                                                          | Only used by build-time shell scripts; not invoked at runtime.                                                                                                                            | Low      | Drop from image; keep in CI runners.                                                                                                                            |
| `sqlite3`                                                                     | Used for ad-hoc DB inspection on the device.                                                                                                                                              | Low      | Replace with `velocity data sql --read-only` in v0.5.1 so routine inspection stays inside the supported binary surface.                                         |
| `curl`                                                                        | Used by Tailscale install today; otherwise nothing on the device calls it.                                                                                                                | Low      | Drops when the Tailscale apt install is removed and the in-binary installer takes over via Go HTTP.                                                             |
| `velocity-update` stub                                                        | Already a one-liner script that prints "use velocity-ctl".                                                                                                                                | Low      | Delete once `velocity-ctl` is the multi-call binary or folded into `velocity-report`.                                                                           |
| Shell lifecycle aliases                                                       | `velocity-status`, `velocity-log`, `velocity-start`, `velocity-stop`, `velocity-bounce` — promoted public surface per [deploy-versioned-binary-plan.md](deploy-versioned-binary-plan.md). | None     | Keep. These are host concerns, not binary concerns.                                                                                                             |
| Embedded tuning defaults                                                      | `/opt/velocity-report/config/tuning.defaults.json` shipped as a separate file.                                                                                                            | Low      | Embed into the binary with `go:embed`; let the operator override via the existing config flag.                                                                  |

## Design / approach

This plan is one direction of travel with five named work units. Each is independently shippable and independently reversible.

**Direction of travel.** The Pi image becomes, at runtime:

```
runtime artifact     who owns it
-------------------  -------------------------------------
/opt/velocity-report/velocity      single Go binary, multi-call
/etc/systemd/system/velocity.service   service unit
/etc/profile.d/velocity-aliases.sh     5 shell wrappers
/var/lib/velocity-report/sensor_data.db  SQLite WAL
/etc/sudoers.d/020_velocity-nopasswd   3 lines: systemctl + tailscaled bridge
```

No xelatex tree. No nginx. No apt tailscale. No `velocity-ctl`, no `velocity-update`, no vendored Typst tree, no `sqlite3` dependency for routine inspection. The only apt packages left at runtime are: the base OS + libc + libpcap + raspi-config + the network stack the OS already ships. Goal end-state for v0.6.0 is "the Pi image is Pi OS Lite + one primary Go binary plus only the binary-owned helper payloads needed to avoid runtime dependency drift."

**Compatibility contract.** `velocity-report` survives as the alias the systemd unit calls; `velocity-ctl` survives only as a transitional redirect into `velocity device ...` for one release. All other surfaces are deleted, not deprecated.

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

### Work unit B: in-binary Tailscale installer (v0.5.1) `M`

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

**Milestone:** v0.5.1. Validated against bookworm and trixie.

### Work unit C: replace xelatex with Typst (v0.5.1) `L`

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
6. Image stage: rewrite [image/stage-velocity/00-install-packages/](../../image/stage-velocity/00-install-packages/) — drop `texlive-xetex`, `texlive-latex-extra`, `fonts-lmodern`, `librsvg2-bin`, `fonts-noto-color-emoji`; stop vendoring Typst under `/opt/velocity-report/typst/`; delete [scripts/build-minimal-texlive.sh](../../scripts/build-minimal-texlive.sh) and [scripts/install-minimal-texlive.sh](../../scripts/install-minimal-texlive.sh).
7. Delete the `Phase 2 .fmt precompile` work from [deploy-rpi-imager-fork-plan.md](deploy-rpi-imager-fork-plan.md) § Phase 2 and [pdf-latex-precompiled-format-plan.md](pdf-latex-precompiled-format-plan.md). The whole problem disappears.
8. CI: add a "PDF parity" job that renders a fixed report.json through both the old (xelatex) and new (Typst) pipelines for one release before deleting the xelatex path; compare page count, dominant glyph fingerprints, chart bounding boxes, and source-archive completeness. Delete the xelatex path after one release of co-existence.

**Milestone:** v0.5.1. xelatex path deleted at v0.5.2 after one release of parity coverage.

### Work unit D: pull nginx removal into v0.5.1 (instead of v0.6.0) `S`

This is just sequencing — the design is already done in [deploy-nginx-removal-plan.md](deploy-nginx-removal-plan.md). Bring it forward so the public install path the v0.6.0 release announces is `http://velocity.local` with no self-signed CA dance.

**Steps:**

1. Land [deploy-nginx-removal-plan.md](deploy-nginx-removal-plan.md) work items in v0.5.1: bind Go on `:80` via `AmbientCapabilities=CAP_NET_BIND_SERVICE`; delete the nginx site, the TLS oneshot script, and the cert directory; update the MOTD URL.
2. Update [docs/platform/operations/tls-local-certificates.md](../platform/operations/tls-local-certificates.md) to point to the Tailscale Serve story for HTTPS.

**Milestone:** v0.5.1.

### Work unit E: image apt-surface trim (v0.5.1) `S`

Once C and D land, the apt package list in [image/stage-velocity/00-install-packages/00-packages](../../image/stage-velocity/00-install-packages/00-packages) becomes:

```
libpcap0.8         # LiDAR support
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

| Artifact                                                          | Today                                                                                    | Future state                                                                                                                                                | Effort          |
| ----------------------------------------------------------------- | ---------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------- |
| `velocity-ctl`                                                    | Separate binary at `/usr/local/bin/velocity-ctl`.                                        | `velocity device ...` namespace inside the multi-call binary.                                                                                               | A (`M`)         |
| `velocity-update` redirect stub                                   | Shell script at `/usr/local/bin/velocity-update`.                                        | Deleted.                                                                                                                                                    | A (`S`)         |
| `internal/cmd/tune/`                                              | Local-dev binary; not shipped.                                                           | `velocity tune sweep`; shipped inside the binary.                                                                                                           | A (`S`)         |
| Tailscale apt repo + install                                      | Image-build stage.                                                                       | Static-binary installer inside `velocity device tailscale install`; helper payload owned and versioned by the main binary; deferred until operator opts in. | B (`M`)         |
| `tailscaled` mask state                                           | Set at image-build.                                                                      | Set by the binary at install time.                                                                                                                          | B (`S`)         |
| `texlive-xetex` + minimal TeX tree                                | 143 MB at `/opt/velocity-report/texlive/`.                                               | Deleted; Typst replaces.                                                                                                                                    | C (`L`)         |
| `librsvg2-bin` + `fonts-noto-color-emoji` + `fonts-lmodern`       | apt packages for the xelatex pipeline.                                                   | Deleted.                                                                                                                                                    | C (`S` after C) |
| `scripts/build-minimal-texlive.sh` + `install-minimal-texlive.sh` | 200+ lines of bash that walk the TeX dependency manifest.                                | Deleted.                                                                                                                                                    | C (`S` after C) |
| `nginx` + site + TLS oneshot                                      | Reverse proxy for `:443`.                                                                | Deleted; Go binds `:80` directly.                                                                                                                           | D (`S`)         |
| `velocity-generate-tls.sh` + service                              | Self-signed cert generation oneshot.                                                     | Deleted.                                                                                                                                                    | D (`S`)         |
| `tuning.defaults.json`                                            | File at `/opt/velocity-report/config/`.                                                  | `go:embed` into the binary; operator override via existing config flag.                                                                                     | E (`S`)         |
| `lidar-network.conf`                                              | `/etc/network/interfaces.d/lidar`.                                                       | `go:embed` + `velocity device install network`.                                                                                                             | E (`S`)         |
| `99-velocity-report.rules` (udev)                                 | `/etc/udev/rules.d/99-velocity-report.rules`.                                            | `go:embed` + `velocity device install udev`.                                                                                                                | E (`S`)         |
| `wpa_supplicant.conf` fallback                                    | `/etc/wpa_supplicant/wpa_supplicant.conf`.                                               | `go:embed` + `velocity device install wifi`.                                                                                                                | E (`S`)         |
| `velocity-aliases.sh` (host lifecycle wrappers)                   | `/etc/profile.d/velocity-aliases.sh`.                                                    | Stays — host lifecycle is not a binary concern per [deploy-versioned-binary-plan.md](deploy-versioned-binary-plan.md).                                      | (none)          |
| `velocity-motd.sh` + `velocity-report-build`                      | MOTD shell script + build stamp.                                                         | Stays; embed the build stamp into the binary and have the MOTD read it from `velocity version`.                                                             | E (`S`)         |
| `sudoers.d/020_velocity-nopasswd`                                 | Grants `pi` broad `velocity-ctl *`; grants `velocity` two literal tailscale subcommands. | Shrinks to: `pi → velocity device *`; `velocity → velocity device tailscale {enable,disable}-tailscaled` plus the static-installer bridge.                  | A + B (`S`)     |
| UART/SPI overlay edits to `/boot/firmware/config.txt`             | Direct file edits in stage script.                                                       | Stays in the image stage; this is firmware-boot config, not a runtime concern.                                                                              | (none)          |
| `raspi-config`                                                    | apt package; used for serial port enable.                                                | Stays.                                                                                                                                                      | (none)          |
| `libpcap0.8`                                                      | apt package; runtime dep of the Go binary.                                               | Stays.                                                                                                                                                      | (none)          |
| `python3-serial`, `minicom`                                       | apt packages; debugging only.                                                            | Deleted.                                                                                                                                                    | E (`S`)         |
| `jq`, `curl`                                                      | apt packages; build-time only.                                                           | Deleted from the image; remain in CI.                                                                                                                       | E (`S`)         |
| `sqlite3`                                                         | apt package; operator convenience.                                                       | Replaced by `velocity data sql --read-only`; removed from the shipped image once the subcommand lands.                                                      | E (`S`)         |
| `os-list-velocity.json`                                           | Custom rpi-imager catalog entry.                                                         | Stays.                                                                                                                                                      | (none)          |
| Reports output dir creation                                       | Stage script `02-velocity-python/00-run.sh`.                                             | First-boot init inside the binary.                                                                                                                          | E (`S`)         |

End-state apt manifest (target for v0.5.1): **`libpcap0.8`, `raspi-config`** plus the base Pi OS Lite packages. The primary velocity-owned artefact at `/opt/velocity-report/` is the `velocity` binary; any extracted Typst runtime cache or Tailscale payload is subordinate, binary-owned implementation detail rather than a separate public surface.

## Scope

### Item 1: fold `velocity-ctl`, sweep, and update stub into one binary

**Summary:** One binary `/opt/velocity-report/versions/<v>/velocity`, with `device`, `serve`, `tune`, `data`, `report`, `version`, `help` namespaces; `velocity-report` survives as the systemd-facing alias; `velocity-ctl` survives as a one-release deprecation shim; `velocity-update` deleted.

**Steps:**

1. Move and rename per work unit A above.
2. Update systemd unit to call `velocity` (or `velocity-report` symlink — same binary).
3. Update sudoers, MOTD, docs to speak the new command surface.
4. Add CI parity job: `velocity-ctl upgrade --check` and `velocity device check` produce byte-identical output for one release.

**Milestone:** v0.5.1.

### Item 2: in-binary Tailscale installer

**Summary:** Image ships zero Tailscale state. The web UI's "Enable Tailscale" button triggers `velocity device tailscale install` → `enable` → `login` → `set --operator` → `serve`.

**Steps:** see work unit B.

**Milestone:** v0.5.1.

### Item 3: xelatex → Typst

**Summary:** Replace the LaTeX pipeline with Typst. Delete the 143 MB TeX tree, the minimal-texlive build scripts, and the rsvg/emoji apt deps. PDF output is preserved within parity tolerances, and the editable source ZIP becomes a Typst source archive rather than disappearing.

**Steps:** see work unit C.

**Milestone:** v0.5.1. xelatex path deleted at v0.5.2 after one release of co-existence.

### Item 4: nginx + self-signed TLS removal (pulled forward)

**Summary:** Bind Go on `:80`. Delete nginx site, TLS cert script, and oneshot service. HTTPS becomes a Tailscale opt-in.

**Steps:** see work unit D and [deploy-nginx-removal-plan.md](deploy-nginx-removal-plan.md).

**Milestone:** v0.5.1.

### Item 5: image apt-surface trim + `go:embed` config

**Summary:** Drop all apt packages that exist only because of xelatex, Tailscale apt-install, or nginx. Embed tuning defaults, network config, udev rules, and wpa_supplicant fallback into the binary.

**Steps:** see work unit E.

**Milestone:** v0.5.1.

## Dependencies

- Work unit A (`velocity-ctl` fold) depends on the multi-call dispatcher landing per [deploy-versioned-binary-plan.md](deploy-versioned-binary-plan.md). The dispatcher is already specified; this plan sequences it into v0.5.1.
- Work unit B (Tailscale installer) depends on A (the `device` namespace). Order: A → B inside v0.5.1.
- Work unit C (Typst) depends on the embedded Typst extraction path, the Typst source-archive migration, and the PDF parity job landing together in v0.5.1, with xelatex retained only for one release of comparison coverage.
- Work unit D (nginx removal) is independent and can land first inside v0.5.1.
- Work unit E (apt-surface trim) depends on C and D; lands in v0.5.1 once the binary-owned SQL and Typst paths are in place.

## Risks

| Risk                                                                                                              | Likelihood | Impact | Mitigation                                                                                                                                                                                                              |
| ----------------------------------------------------------------------------------------------------------------- | ---------- | ------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Typst output parity diverges from xelatex on edge cases (kerning, glyph fallback)                                 | Medium     | Medium | Run xelatex and Typst side-by-side in CI for one release; ship Typst only after a clean diff on the canonical report set.                                                                                               |
| In-binary Tailscale installer fails on an unsupported architecture or leaves us on an untested distro combination | Medium     | Medium | Pin supported archive tuples in the baked manifest, fail loudly on unknown host combinations, and validate the static install flow on the release-supported Pi OS variants before cut.                                  |
| `velocity-ctl` deprecation shim breaks operators who scripted against the old surface                             | Low        | Low    | One-release co-existence with a stderr warning; release notes call it out; sudoers still allows the old name for that release.                                                                                          |
| nginx removal lands without `:80` binding capability set                                                          | Low        | High   | `AmbientCapabilities=CAP_NET_BIND_SERVICE` is set in the systemd unit at the same commit; image-build CI smoke-tests the bind on a chroot before image export.                                                          |
| Binary size rises materially once Typst and helper payload logic are embedded                                     | High       | Low    | Measure and publish size by target in CI, but treat size as a tradeoff metric rather than a hard release gate for this plan; optimise obvious waste without reintroducing external runtime dependencies.                |
| Static-binary Tailscale install fails mid-download or leaves partial state on disk                                | Low        | Medium | Download into a temp dir, verify checksum before activation, and only switch the stable symlink after the payload passes validation.                                                                                    |
| Existing operator scripts or habits that expect `report.tex` inside the source ZIP break after the Typst cutover  | Medium     | Medium | Keep the source-archive feature, switch it deliberately to `report.typ`, document the archive layout change in release notes and setup docs, and include a short migration guide for users who previously edited LaTeX. |

## Checklist

### Complete

- [x] Plan written and circulated for review.
- [x] Work unit A: fold `velocity-ctl` and sweep into the multi-call binary, ship the `velocity-ctl` deprecation shim, and pull forward the **full** versioned dispatcher/upgrade machinery (`renameat2` swap, retention, single-artifact release.json). `velocity-update` was already removed (#290).
- [x] Work unit D: nginx removal landed (#517).
- [x] Work unit E: `go:embed` for tuning defaults, network config, udev rules, and wpa_supplicant + `velocity device install`; `velocity data sql --read-only` read-only inspection subcommand replacing `sqlite3`; dropped `python3-serial`, `minicom`, `jq`, `curl`, and `sqlite3` from the apt surface; deleted the `02-velocity-python` stage.
- [x] Removed the transitional `velocity-ctl` shim: the `/usr/local/bin/velocity-ctl` symlink (image stage 01), the `velocity-ctl` sudoers grants (stage 03), and the deprecation-warning path in `cmd/velocity/main.go`.
- [x] Docs: updated distribution-packaging, rpi-imager, setup, asset-naming, COMMANDS, CLAUDE, and coding-standards to the new surface.

### Outstanding

- [ ] Work unit B: in-binary Tailscale installer; delete `image/stage-velocity/07-velocity-tailscale/` (`M`)
- [ ] Work unit C: Typst PDF pipeline; delete `texlive-xetex` apt surface and the minimal-texlive build scripts (`L`)
- [ ] Source archive migration: replace `report.tex` ZIP output with an editable `report.typ` archive and publish the operator migration note (`M`)
- [ ] CI: PDF parity job (xelatex vs Typst) for one release before xelatex deletion, including source-archive completeness checks (`S`)
- [ ] CI: image stage smoke test that the bind on `:80` works in a chroot before export (`S`)
- [ ] Docs: Typst source-archive migration note (deferred with work unit C)

### Follow-on image cleanup (gated on work units B / C)

The `sqlite3` drop and the `velocity-ctl` shim removal have landed; the
remaining apt-surface trims are each unblocked by a specific later landing and
**should be done in the same change that lands it**, not separately:

- [ ] **When work unit B (in-binary Tailscale installer) lands:** delete
      `image/stage-velocity/07-velocity-tailscale/` _and_ the on-demand
      `apt-get install … curl` it now carries. `curl` was dropped from
      `00-packages` in #519 but stage 07 still installs it on demand for the
      keyring fetch; deleting the stage removes `curl` from the image entirely.
- [ ] **When work unit C (Typst) lands:** drop `texlive-xetex`,
      `texlive-latex-extra`, `fonts-lmodern`, `librsvg2-bin`, and
      `fonts-noto-color-emoji` from `00-install-packages/00-packages`; delete the
      `00-install-packages/01-run.sh` minimal-TeX build stage and
      `scripts/build-minimal-texlive.sh` / `scripts/install-minimal-texlive.sh`.

Once all four land, the `00-packages` end-state is just `libpcap0.8` +
`raspi-config`, and the only operator-facing binary name is `velocity`
(`velocity-report` survives only as the systemd-facing alias).

### Deferred

- [ ] CGo binding to the Typst Rust crate — explicitly rejected; build-system tax is not worth it.

### Accepted residuals (no action planned)

- [ ] Host lifecycle aliases (`velocity-status`, `velocity-log`, `velocity-start`, `velocity-stop`, `velocity-bounce`) stay outside the binary. Host concerns are not application namespaces per [deploy-versioned-binary-plan.md](deploy-versioned-binary-plan.md).
- [ ] UART/SPI overlay edits to `/boot/firmware/config.txt` stay in the image stage script. Firmware-boot config is not a runtime concern.
- [ ] `libpcap0.8` and `raspi-config` apt packages survive into v0.6.0. They are tiny, stable, and have legitimate operator use.
