# Asset Naming Convention

- **Status:** Draft
- **Owner:** Grace (Architect)
- **Scope mode:** HOLD — the architecture is sound; this standardises naming across existing assets

## Problem

Four publishable asset types use four different naming conventions:

| Asset                   | Current release name                | Current dev name                            | Issues                                                                                      |
| ----------------------- | ----------------------------------- | ------------------------------------------- | ------------------------------------------------------------------------------------------- |
| Go server (Linux ARM64) | `velocity-report-linux-arm64`       | same                                        | No version in filename; no dev/release distinction                                          |
| Go server (macOS ARM64) | `velocity-report-mac-arm64`         | same                                        | No version in filename                                                                      |
| velocity-ctl            | `velocity-ctl-linux-arm64`          | same                                        | No version in filename                                                                      |
| RPi image               | `velocity-report.img.xz`            | same                                        | No version in filename; CI renames for release but base is unversioned                      |
| macOS DMG (release)     | `VelocityVisualiser-0.5.1-pre1.dmg` | `VelocityVisualiser-0.5.1-pre1+abc1234.dmg` | Only asset with version; uses `+` separator; no date in dev; PascalCase differs from others |

The result: you cannot tell which version a binary is from its filename, dev builds are unsortable, and the naming style is inconsistent across the project.

## Design Decision

### Product name

All assets use the hyphenated product name: **`velocity-report`**.

The macOS visualiser keeps its PascalCase brand name: **`VelocityVisualiser`**. This is the user-facing application name (in Finder, the DMG volume, the menu bar) and part of the product's identity. Asset filenames use this form. See `.github/knowledge/coding-standards.md` § Product Names for the canonical rule.

### Version string

Single source of truth: `VERSION` in `Makefile` (currently `0.5.1-pre1`).

Release versions follow SemVer: `MAJOR.MINOR.PATCH` (e.g. `0.5.1`). Pre-release tags append a hyphen suffix per SemVer: `0.5.1-pre1`, `0.6.0-rc1`.

**Constraint: no leading zeros in version segments.** `web/package.json` and `public_html/package.json` are validated by npm's strict SemVer parser. A version like `0.5.04` is **invalid** SemVer — the patch segment `04` has a leading zero, which npm rejects outright. Use `0.5.4` instead. See § Version Validity Analysis below.

### Two filename tiers

**Release** — tagged releases published to GitHub Releases. Human-readable, stable, linkable. No date, no SHA.

```
velocity-report-0.5.1-linux-arm64
velocity-report-0.5.1-darwin-arm64
velocity-report-0.5.1-darwin-amd64
velocity-ctl-0.5.1-linux-arm64
velocity-report-0.5.1.img.xz
VelocityVisualiser-0.5.1.dmg
```

**Dev** — CI artefacts and local builds. Date-time prefix for sortability; 7-char git SHA suffix for traceability.

```
20260407T142345Z-velocity-report-0.5.1.pre1-linux-arm64-a1b2c3d
20260407T142345Z-velocity-report-0.5.1.pre1-darwin-arm64-a1b2c3d
20260407T142345Z-velocity-ctl-0.5.1.pre1-linux-arm64-a1b2c3d
20260407T142345Z-velocity-report-0.5.1.pre1-a1b2c3d.img.xz
20260407T142345Z-VelocityVisualiser-0.5.1.pre1-a1b2c3d.dmg
```

### Naming grammar

```
Release:  {product}-{version}-{os}-{arch}{ext}
Dev:      {datetime}-{product}-{version}-{os}-{arch}-{sha7}{ext}
```

| Token      | Format               | Example            | Notes                                                                                                                      |
| ---------- | -------------------- | ------------------ | -------------------------------------------------------------------------------------------------------------------------- |
| `product`  | lowercase-hyphenated | `velocity-report`  | `velocity-ctl`. Exception: `VelocityVisualiser` keeps PascalCase (see § Product Names in coding-standards.md)              |
| `version`  | SemVer               | `0.5.1`            | Release: clean SemVer. Dev: dots replace hyphens in pre-release (`0.5.1.pre1`) to avoid ambiguity with the field separator |
| `os`       | Go GOOS              | `linux`, `darwin`  | Omitted for single-platform assets (RPi image is always linux-arm64; DMG is always darwin)                                 |
| `arch`     | Go GOARCH            | `arm64`, `amd64`   | Omitted for single-platform assets                                                                                         |
| `datetime` | `YYYYMMDDTHHmmssZ`   | `20260407T142345Z` | UTC, compact ISO 8601 with seconds, no colons (filesystem-safe). Derived from `BUILD_TIME` via `subst`. Dev only           |
| `sha7`     | 7-char git short SHA | `a1b2c3d`          | Dev only                                                                                                                   |
| `ext`      | file extension       | `.img.xz`, `.dmg`  | Binaries have no extension (Unix convention)                                                                               |

### Full asset catalogue

#### Release filenames

| Asset                      | Filename                           | Checksum         |
| -------------------------- | ---------------------------------- | ---------------- |
| Go server (Linux ARM64)    | `velocity-report-{v}-linux-arm64`  | `.sha256`        |
| Go server (macOS ARM64)    | `velocity-report-{v}-darwin-arm64` | `.sha256`        |
| Go server (macOS Intel)    | `velocity-report-{v}-darwin-amd64` | `.sha256`        |
| velocity-ctl (Linux ARM64) | `velocity-ctl-{v}-linux-arm64`     | `.sha256`        |
| RPi image                  | `velocity-report-{v}.img.xz`       | `.img.xz.sha256` |
| macOS visualiser DMG       | `VelocityVisualiser-{v}.dmg`       | `.dmg.sha256`    |

#### Dev filenames

| Asset                      | Filename                                       | Example                   |
| -------------------------- | ---------------------------------------------- | ------------------------- |
| Go server (Linux ARM64)    | `{dt}-velocity-report-{v}-linux-arm64-{sha7}`  | `dt` = `20260407T142345Z` |
| Go server (macOS ARM64)    | `{dt}-velocity-report-{v}-darwin-arm64-{sha7}` |                           |
| velocity-ctl (Linux ARM64) | `{dt}-velocity-ctl-{v}-linux-arm64-{sha7}`     |                           |
| RPi image                  | `{dt}-velocity-report-{v}-{sha7}.img.xz`       |                           |
| macOS visualiser DMG       | `{dt}-VelocityVisualiser-{v}-{sha7}.dmg`       |                           |

### Alternatives considered

| Alternative                                | Verdict                          | Reason                                                                                                                                                                             |
| ------------------------------------------ | -------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Date at end                                | Rejected                         | Cannot `ls` sort by date                                                                                                                                                           |
| SHA in release filenames                   | Rejected                         | Noisy; version tag is sufficient for release traceability                                                                                                                          |
| Keep PascalCase for DMG                    | **Accepted**                     | Brand identity — `VelocityVisualiser` is the user-facing app name in Finder, menu bar, and About dialog. Canonical rule in `.github/knowledge/coding-standards.md` § Product Names |
| `+sha` separator (current DMG style)       | Rejected                         | `+` is reserved in URLs, causes shell escaping pain                                                                                                                                |
| Dots in pre-release (`0.5.1-pre1`) for dev | Rejected initially, then adopted | Hyphens in pre-release conflict with the hyphen field separator; using dots (`0.5.1.pre1`) in dev filenames avoids ambiguity while keeping clean SemVer for release builds         |
| Separate `date` call for compact timestamp | Rejected                         | Two independent `date` invocations can disagree (minute boundary). Derive `BUILD_TS_COMPACT` from `BUILD_TIME` via Make `subst` — single source, guaranteed same instant           |
| Minute precision in compact timestamp      | Rejected                         | Two builds in the same minute produce identical prefixes. Seconds precision (`YYYYMMDDTHHmmssZ`) costs 2 characters but avoids ambiguity; SHA suffix still disambiguates           |

## Version Validity Analysis

**Question:** Can we use `0.5.04` as a version number?

**Answer: No.** SemVer §2 requires each numeric identifier to have no leading zeros. `04` is invalid — it must be `4`. npm enforces this strictly; `pnpm install` will reject a `package.json` with `"version": "0.5.04"`.

### Surfaces audited

| Surface         | File                                 | Validates SemVer?        | Effect of `0.5.04`                                      |
| --------------- | ------------------------------------ | ------------------------ | ------------------------------------------------------- |
| npm (web)       | `web/package.json`                   | **Yes — strict**         | **Rejects with parse error**                            |
| npm (docs)      | `public_html/package.json`           | **Yes — strict**         | **Rejects with parse error**                            |
| Go version pkg  | `internal/version/version.go`        | No — display only        | Passes (string)                                         |
| Makefile        | `Makefile:177`                       | No — string constant     | Passes                                                  |
| CI workflows    | `.github/workflows/*.yml`            | No — string substitution | Passes                                                  |
| Xcode           | `project.pbxproj` MARKETING_VERSION  | No — string              | Passes                                                  |
| Python          | `tools/pdf-generator/pyproject.toml` | Lenient (PEP 440)        | Passes locally; PyPI would normalise `0.5.04` → `0.5.4` |
| rpi-imager JSON | `image/os-list-velocity.json`        | No                       | Passes                                                  |

**Two surfaces hard-block:** `web/package.json` and `public_html/package.json`. Every CI run and every `pnpm install` invocation validates the version field against SemVer. There is no workaround short of removing the version field (which breaks other tooling).

**Recommendation:** Use `0.5.4` (no leading zero). If the intent was to encode a month or sequence number, use a pre-release suffix: `0.5.4-04` or `0.5.0-rc4`.

## Implementation Plan

### Phase 1: Makefile variables (foundation)

Add computed filename variables to the Makefile `VERSION INFORMATION` section. **One `date` call** — `BUILD_TIME` is the canonical timestamp; `BUILD_TS_COMPACT` is derived by stripping hyphens and colons.

**Files to change:**

- [Makefile](../../Makefile) — lines 176–181

**New variables:**

```makefile
# Existing
VERSION := 0.5.1-pre1
GIT_SHA := $(shell git rev-parse HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GIT_SHA_SHORT := $(shell printf '%.7s' '$(GIT_SHA)')

# New: asset naming (derived from BUILD_TIME — no second date call)
BUILD_TS_COMPACT := $(subst -,,$(subst :,,$(BUILD_TIME)))
# Dev version: replace hyphens with dots in pre-release segment for filename safety
DEV_VERSION := $(subst -,.,$(VERSION))
```

`BUILD_TIME` = `2026-04-07T14:23:45Z` → strip `:` → `2026-04-07T142345Z` → strip `-` → `20260407T142345Z` = `BUILD_TS_COMPACT`.

**Risk:** None. Additive only. Single `date` call; `BUILD_TS_COMPACT` is a pure string transformation.

### Phase 2: Binary output filenames

Update `build-*` targets to use versioned names. All consumers update in the same change — no symlink transition.

**Changes:**

| Target                  | Current output                | New output (dev)                                                                   | New output (release, future)              |
| ----------------------- | ----------------------------- | ---------------------------------------------------------------------------------- | ----------------------------------------- |
| `build-radar-linux`     | `velocity-report-linux-arm64` | `$(BUILD_TS_COMPACT)-velocity-report-$(DEV_VERSION)-linux-arm64-$(GIT_SHA_SHORT)`  | `velocity-report-$(VERSION)-linux-arm64`  |
| `build-radar-mac`       | `velocity-report-mac-arm64`   | `$(BUILD_TS_COMPACT)-velocity-report-$(DEV_VERSION)-darwin-arm64-$(GIT_SHA_SHORT)` | `velocity-report-$(VERSION)-darwin-arm64` |
| `build-radar-mac-intel` | `velocity-report-mac-amd64`   | `$(BUILD_TS_COMPACT)-velocity-report-$(DEV_VERSION)-darwin-amd64-$(GIT_SHA_SHORT)` | `velocity-report-$(VERSION)-darwin-amd64` |
| `build-radar-local`     | `velocity-report-local`       | Keep as `velocity-report-local` (not published)                                    | —                                         |
| `build-ctl`             | `velocity-ctl`                | Keep as `velocity-ctl` (not published)                                             | —                                         |
| `build-ctl-linux`       | `velocity-ctl-linux-arm64`    | `$(BUILD_TS_COMPACT)-velocity-ctl-$(DEV_VERSION)-linux-arm64-$(GIT_SHA_SHORT)`     | `velocity-ctl-$(VERSION)-linux-arm64`     |

**Local dev binaries** (`build-radar-local`, `build-ctl`) keep their short names — they are never published.

**No symlink transition.** All references to old filenames update atomically.

#### Full blast radius (audited 2026-04-08)

Every file that references old binary filenames, grouped by impact class:

**Build outputs (must change — Phase 2 core):**

| File                       | Lines    | What changes                              |
| -------------------------- | -------- | ----------------------------------------- |
| [Makefile](../../Makefile) | 190, 194 | `velocity-report-linux-arm64` → versioned |
| [Makefile](../../Makefile) | 198      | `velocity-report-mac-arm64` → versioned   |
| [Makefile](../../Makefile) | 202      | `velocity-report-mac-amd64` → versioned   |
| [Makefile](../../Makefile) | 226      | `velocity-ctl-linux-arm64` → versioned    |

**Image build (must change — consumes build outputs):**

| File                                                               | Lines | What changes                                |
| ------------------------------------------------------------------ | ----- | ------------------------------------------- |
| [image/scripts/build-image.sh](../../image/scripts/build-image.sh) | 136   | `cp velocity-report-linux-arm64` → new name |
| [image/scripts/build-image.sh](../../image/scripts/build-image.sh) | 137   | `cp velocity-ctl-linux-arm64` → new name    |

**Go upgrade logic (runtime asset lookup):**

| File                                                     | Lines | What changes                                                                                                               |
| -------------------------------------------------------- | ----- | -------------------------------------------------------------------------------------------------------------------------- |
| [internal/ctl/manager.go](../../internal/ctl/manager.go) | 297   | `fmt.Sprintf("velocity-report-%s-%s", GOOS, GOARCH)` → `velocity-report-{version}-{GOOS}-{GOARCH}` using `release.TagName` |
| [internal/ctl/manager.go](../../internal/ctl/manager.go) | 306   | Remove `velocity-report-arm64` fallback                                                                                    |

**Go upgrade tests (must update to match new asset names):**

| File                                                                       | Lines                       | What changes                 |
| -------------------------------------------------------------------------- | --------------------------- | ---------------------------- |
| [cmd/velocity-ctl/upgrade_test.go](../../cmd/velocity-ctl/upgrade_test.go) | 20, 143, 146, 147, 194, 197 | Test asset names → versioned |
| [internal/ctl/manager_test.go](../../internal/ctl/manager_test.go)         | 177, 420, 448, 449, 477     | Test asset names → versioned |

**Download page / release metadata (must change — user-facing URLs):**

| File                                                                     | Lines    | What changes                                                                          |
| ------------------------------------------------------------------------ | -------- | ------------------------------------------------------------------------------------- |
| [public_html/src/index.njk](../../public_html/src/index.njk)             | 97, 119  | Download URLs → `velocity-report-{v}-linux-arm64`, `velocity-report-{v}-darwin-arm64` |
| [public_html/dist/index.html](../../public_html/dist/index.html)         | 216, 238 | Regenerate from njk                                                                   |
| [scripts/check-release-hashes.py](../../scripts/check-release-hashes.py) | 116, 122 | URL construction → new pattern                                                        |

**Documentation (Phase 6, but listed here for completeness):**

| File                                                                                                               | Refs    | Notes                                                         |
| ------------------------------------------------------------------------------------------------------------------ | ------- | ------------------------------------------------------------- |
| [docs/radar/operations/remote-host-upgrade-runbook.md](../../docs/radar/operations/remote-host-upgrade-runbook.md) | 8 refs  | `scp velocity-report-linux-arm64`, `export NEW_BIN=...`, etc. |
| [docs/radar/cli-comprehensive-guide.md](../../docs/radar/cli-comprehensive-guide.md)                               | 5 refs  | `--binary ./velocity-report-linux-arm64`                      |
| [docs/platform/operations/rpi-imager.md](../../docs/platform/operations/rpi-imager.md)                             | 1 ref   | "Downloads the `velocity-report-linux-arm64` asset"           |
| [docs/platform/operations/distribution-packaging.md](../../docs/platform/operations/distribution-packaging.md)     | 1 ref   | "Binary name unchanged: `velocity-report-linux-arm64`"        |
| [docs/plans/deploy-distribution-packaging-plan.md](../../docs/plans/deploy-distribution-packaging-plan.md)         | 12 refs | Future plan — references throughout                           |
| [docs/plans/deploy-rpi-imager-fork-plan.md](../../docs/plans/deploy-rpi-imager-fork-plan.md)                       | 3 refs  | Future plan — references to binary/ctl names                  |
| [docs/plans/binary-size-reduction-plan.md](../../docs/plans/binary-size-reduction-plan.md)                         | 1 ref   | GSA analysis command                                          |
| [tools/visualiser-macos/BUILDING.md](../../tools/visualiser-macos/BUILDING.md)                                     | 4 refs  | DMG naming examples (Phase 3)                                 |
| [public_html/dist/guides/setup/index.html](../../public_html/dist/guides/setup/index.html)                         | 1 ref   | Built output — regenerate from source                         |

**Not affected:**

| File                                                   | Why                                                                       |
| ------------------------------------------------------ | ------------------------------------------------------------------------- |
| [.gitignore](../../.gitignore)                         | Uses wildcard `velocity-report-*` and `/velocity-ctl*` — covers new names |
| [image/Dockerfile.build](../../image/Dockerfile.build) | Outputs `/out/velocity-report` (no OS/arch suffix) — internal to Docker   |

#### `findAssetURL` change

`internal/ctl/manager.go:297` constructs the asset name from `release.TagName`:

```go
// Strip the "v" prefix from the tag to get the version.
version := strings.TrimPrefix(release.TagName, "v")
assetName := fmt.Sprintf("velocity-report-%s-%s-%s", version, m.cfg.GOOS, m.cfg.GOARCH)
```

The version is always known at lookup time — no prefix matching needed.

### Phase 3: DMG naming

Standardise DMG filenames to use the new date/SHA scheme while **keeping PascalCase `VelocityVisualiser`** (brand identity — see `.github/knowledge/coding-standards.md` § Product Names).

**Files to change:**

- [Makefile](../../Makefile) — DMG variables (lines 335–336) and `dmg-mac`/`dmg-mac-release` targets
- [.github/workflows/mac-ci.yml](../../.github/workflows/mac-ci.yml) — artefact glob

**Changes:**

```makefile
# Before (dev)
VISUALISER_DMG = $(VISUALISER_BUILD_DIR)/VelocityVisualiser-$(VERSION)+$(GIT_SHA_SHORT).dmg

# After (dev) — date prefix, SHA suffix, no + separator
VISUALISER_DMG = $(VISUALISER_BUILD_DIR)/$(BUILD_TS_COMPACT)-VelocityVisualiser-$(DEV_VERSION)-$(GIT_SHA_SHORT).dmg

# After (release) — clean version, no date/SHA
# dmg-mac-release overrides:
VISUALISER_DMG_RELEASE = $(VISUALISER_BUILD_DIR)/VelocityVisualiser-$(VERSION).dmg
```

**CI artefact name:** Stays `VelocityVisualiser-dmg`.

**Risk:** Low. DMG is self-contained; no downstream consumers depend on the filename.

### Phase 4: RPi image naming

Version the image filename in both local builds and CI.

**Files to change:**

- [image/scripts/build-image.sh](../../image/scripts/build-image.sh) — if it controls the output name
- [.github/workflows/build-image.yml](../../.github/workflows/build-image.yml) — rename step (line 127), release upload, artefact name
- [image/os-list-velocity.json](../../image/os-list-velocity.json) — download URL pattern

**Changes:**

Release (CI, triggered by tag `v0.5.1`):

```
velocity-report-0.5.1.img.xz
velocity-report-0.5.1.img.xz.sha256
```

Dev (CI, manual dispatch):

```
20260407T142345Z-velocity-report-0.5.1.pre1-a1b2c3d.img.xz
```

The `update-os-list` job already rewrites the URL on release — it just needs to use the versioned filename.

**Risk:** Medium — the `os-list-velocity.json` URL must match exactly. CI already handles this; just ensure the filename substitution is correct.

### Phase 5: CI workflow updates

Ensure all workflows that produce publishable artefacts use the new naming. **No recompute** — CI receives `BUILD_TIME` from the Makefile or passes it as an env var; compact form is always derived, never independently computed.

**Files to change:**

- [.github/workflows/build-image.yml](../../.github/workflows/build-image.yml) — rename step, upload step, os-list update; remove independent `BUILD_TIME` computation
- [.github/workflows/mac-ci.yml](../../.github/workflows/mac-ci.yml) — DMG step, artefact upload
- [image/scripts/build-image.sh](../../image/scripts/build-image.sh) — remove independent `BUILD_TIME` / `BUILD_TIME_STAMP` computation; receive from caller
- Any future Go binary release workflow

**Consolidation:** `image/scripts/build-image.sh` (line 144) and `.github/workflows/build-image.yml` (line 40) currently compute their own `BUILD_TIME` with independent `date` calls. These must be removed — the Makefile is the single source.

**New CI variables** (set in each workflow):

```yaml
env:
  BUILD_TIME: # from Makefile or computed once in workflow setup step
  BUILD_TS_COMPACT: # derived: ${BUILD_TIME//[-:]/}
  GIT_SHA_SHORT: # computed in step
  DEV_VERSION: # VERSION with hyphens → dots
```

### Phase 6: Documentation

**Files to update:**

- [image/README.md](../../image/README.md) — output filename examples
- [docs/platform/operations/rpi-imager.md](../../docs/platform/operations/rpi-imager.md) — asset references
- [ARCHITECTURE.md](../../ARCHITECTURE.md) — if it references binary names
- [README.md](../../README.md) — download/install examples, if any

## Failure Registry

| Failure                                        | Impact                            | Recovery                                                                |
| ---------------------------------------------- | --------------------------------- | ----------------------------------------------------------------------- |
| Stale filename reference in script or workflow | Build or deploy fails             | Grep audit in Phase 2 catches all references; CI validates              |
| `os-list-velocity.json` URL mismatch           | rpi-imager cannot find the image  | CI job computes the URL from the tag; test with a pre-release first     |
| DMG artefact glob mismatch in CI               | Artefact upload fails             | Update glob in `mac-ci.yml` (Phase 3)                                   |
| CI computes own `BUILD_TIME` out of sync       | Timestamp mismatch between assets | Phase 5 removes all independent `date` calls; single source in Makefile |

## Boundary Diagram

```
                Makefile (single source of truth)
                ┌────────────────────────────────┐
                │  VERSION, GIT_SHA, BUILD_TIME  │
                │  GIT_SHA_SHORT, BUILD_TS_COMPACT│
                │  DEV_VERSION                   │
                └──────────┬─────────────────────┘
                           │
          ┌────────────────┼──────────────────┐
          │                │                  │
          ▼                ▼                  ▼
   build-radar-*     dmg-mac[-release]   build-image
   build-ctl-*             │                  │
          │                │                  │
          ▼                ▼                  ▼
   Go binaries         .dmg file        .img.xz file
   (dev or release)   (dev or release)  (dev or release)
          │                │                  │
          └────────┬───────┘                  │
                   ▼                          ▼
            GitHub Actions              GitHub Releases
            (artefacts)                (release assets)
                                              │
                                              ▼
                                    os-list-velocity.json
                                      (URL + SHA-256)
```

## Sequencing

1. **Phase 1** — Makefile variables (`BUILD_TS_COMPACT`, `DEV_VERSION`). Zero risk. Do first.
2. **Phase 2** — Binary filenames. Update all consumers (image build, deploy) atomically. No symlinks.
3. **Phase 3** — DMG naming. Independent of Phase 2.
4. **Phase 4** — RPi image naming. Depends on Phase 2 (binaries feed into image).
5. **Phase 5** — CI. Remove independent `BUILD_TIME` computations. Depends on Phases 2–4.
6. **Phase 6** — Docs. Last, after everything is wired.

Phases 2 and 3 can run in parallel. Phase 4 follows Phase 2. Phase 5 follows everything.

## Open Questions

1. **Should `build-radar-local` get a versioned name?** Current recommendation: no. It is a local dev binary, never published, and a short name is ergonomic. Keep as-is.

2. **Should checksums be generated for all dev artefacts?** Current recommendation: no. Checksums matter for release downloads where integrity verification is expected. Dev artefacts have the SHA in the filename for traceability, which is sufficient.

## Resolved Questions

3. ~~**Separate `BUILD_DATETIME` variable?**~~ **Resolved: derive from `BUILD_TIME`.** `BUILD_TS_COMPACT` is computed via `$(subst -,,$(subst :,,$(BUILD_TIME)))` — one `date` call, one source of truth. Seconds precision (`YYYYMMDDTHHmmssZ`) matches `BUILD_TIME`.

4. ~~**Redundant `BUILD_TIME` in CI and image script?**~~ **Resolved: remove.** Phase 5 consolidates to a single source.
