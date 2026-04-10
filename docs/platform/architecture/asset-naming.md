# Asset Naming Conventions

Canonical naming patterns for release artefacts, development builds, and
platform-specific binaries across the velocity.report project.

## Product Names

| Product          | Asset name           | Case rule  |
| ---------------- | -------------------- | ---------- |
| Server           | `velocity-report`    | kebab-case |
| macOS Visualiser | `VelocityVisualiser` | PascalCase |
| Management CLI   | `velocity-ctl`       | kebab-case |

The macOS visualiser keeps PascalCase because it is the user-facing
application name (Finder, DMG volume, menu bar, About dialog). See
`.github/knowledge/coding-standards.md` § Product Names.

## Version String

Single source of truth: `VERSION` in `Makefile`.

Release versions follow SemVer: `MAJOR.MINOR.PATCH` (e.g. `0.5.1`).
Pre-release tags append a hyphen suffix: `0.5.1-pre1`, `0.6.0-rc1`.

**No leading zeros in version segments.** `web/package.json` and
`public_html/package.json` are validated by npm's strict SemVer parser.
A version like `0.5.04` is invalid — the patch segment `04` has a leading
zero. Use `0.5.4` instead. See § Version Validity Analysis.

## Two Filename Tiers

### Release — tagged GitHub Releases

Human-readable, stable, linkable. No date, no SHA.

```
velocity-report-0.5.1-linux-arm64
velocity-report-0.5.1-darwin-arm64
velocity-ctl-0.5.1-linux-arm64
velocity-report-0.5.1.img.xz
VelocityVisualiser-0.5.1.dmg
```

### Dev — CI artefacts and local builds

Date-time prefix for sortability; 7-char git SHA suffix for traceability.

```
20260407T142345Z-velocity-report-0.5.1.pre1-linux-arm64-a1b2c3d
20260407T142345Z-velocity-ctl-0.5.1.pre1-linux-arm64-a1b2c3d
20260407T142345Z-velocity-report-0.5.1.pre1-a1b2c3d.img.xz
20260407T142345Z-VelocityVisualiser-0.5.1.pre1-a1b2c3d.dmg
```

## Naming Grammar

```
Release:  {product}-{version}-{os}-{arch}{ext}
Dev:      {datetime}-{product}-{version}-{os}-{arch}-{sha7}{ext}
```

| Token      | Format               | Example            | Notes                                                   |
| ---------- | -------------------- | ------------------ | ------------------------------------------------------- |
| `product`  | lowercase-hyphenated | `velocity-report`  | Exception: `VelocityVisualiser` keeps PascalCase        |
| `version`  | SemVer               | `0.5.1`            | Dev: dots replace hyphens in pre-release (`0.5.1.pre1`) |
| `os`       | Go GOOS              | `linux`, `darwin`  | Omitted for single-platform assets (RPi image, DMG)     |
| `arch`     | Go GOARCH            | `arm64`, `amd64`   | Omitted for single-platform assets                      |
| `datetime` | `YYYYMMDDTHHmmssZ`   | `20260407T142345Z` | UTC, compact ISO 8601, filesystem-safe. Dev only        |
| `sha7`     | 7-char short SHA     | `a1b2c3d`          | Dev only                                                |
| `ext`      | file extension       | `.img.xz`, `.dmg`  | Binaries have no extension (Unix convention)            |

## Full Asset Catalogue

### Release filenames

| Asset                      | Filename                           | Checksum         |
| -------------------------- | ---------------------------------- | ---------------- |
| Go server (Linux ARM64)    | `velocity-report-{v}-linux-arm64`  | `.sha256`        |
| Go server (macOS ARM64)    | `velocity-report-{v}-darwin-arm64` | `.sha256`        |
| Go server (macOS Intel)    | `velocity-report-{v}-darwin-amd64` | `.sha256`        |
| velocity-ctl (Linux ARM64) | `velocity-ctl-{v}-linux-arm64`     | `.sha256`        |
| RPi image                  | `velocity-report-{v}.img.xz`       | `.img.xz.sha256` |
| macOS visualiser DMG       | `VelocityVisualiser-{v}.dmg`       | `.dmg.sha256`    |

### Dev filenames

| Asset                      | Filename                                       |
| -------------------------- | ---------------------------------------------- |
| Go server (Linux ARM64)    | `{dt}-velocity-report-{v}-linux-arm64-{sha7}`  |
| Go server (macOS ARM64)    | `{dt}-velocity-report-{v}-darwin-arm64-{sha7}` |
| velocity-ctl (Linux ARM64) | `{dt}-velocity-ctl-{v}-linux-arm64-{sha7}`     |
| RPi image                  | `{dt}-velocity-report-{v}-{sha7}.img.xz`       |
| macOS visualiser DMG       | `{dt}-VelocityVisualiser-{v}-{sha7}.dmg`       |

Local dev binaries (`build-radar-local`, `build-ctl`) keep short names
— they are never published.

## Version Validity Analysis

| Surface         | File                                 | Validates SemVer?    | Effect of `0.5.04`                 |
| --------------- | ------------------------------------ | -------------------- | ---------------------------------- |
| npm (web)       | `web/package.json`                   | **Yes — strict**     | **Rejects with parse error**       |
| npm (docs)      | `public_html/package.json`           | **Yes — strict**     | **Rejects with parse error**       |
| Go version pkg  | `internal/version/version.go`        | No — display only    | Passes                             |
| Makefile        | `Makefile`                           | No — string constant | Passes                             |
| CI workflows    | `.github/workflows/*.yml`            | No — substitution    | Passes                             |
| Xcode           | `project.pbxproj` MARKETING_VERSION  | No — string          | Passes                             |
| Python          | `tools/pdf-generator/pyproject.toml` | Lenient (PEP 440)    | Passes; PyPI normalises to `0.5.4` |
| rpi-imager JSON | `image/os-list-velocity.json`        | No                   | Passes                             |

Two surfaces hard-block: `web/package.json` and `public_html/package.json`.
Every CI run validates the version field against strict SemVer.

## Makefile Variables

All naming derives from `Makefile` § VERSION INFORMATION:

```makefile
VERSION := 0.5.1-pre1
GIT_SHA := $(shell git rev-parse HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GIT_SHA_SHORT := $(shell printf '%.7s' '$(GIT_SHA)')
BUILD_TS_COMPACT := $(subst -,,$(subst :,,$(BUILD_TIME)))
DEV_VERSION := $(subst -,.,$(VERSION))
```

`BUILD_TS_COMPACT` is derived from `BUILD_TIME` via `subst` — one `date`
call, one source of truth. CI and `build-image.sh` must not compute their
own `BUILD_TIME`.

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

## Alternatives Considered

| Alternative                        | Verdict      | Reason                                                            |
| ---------------------------------- | ------------ | ----------------------------------------------------------------- |
| Date at end                        | Rejected     | Cannot `ls`-sort by date                                          |
| SHA in release filenames           | Rejected     | Noisy; version tag is sufficient                                  |
| Keep PascalCase for DMG            | **Accepted** | Brand identity in Finder, menu bar, About dialog                  |
| `+sha` separator (old DMG style)   | Rejected     | `+` is reserved in URLs, causes shell escaping pain               |
| Dots in dev pre-release            | Adopted      | Hyphens conflict with field separator; dots avoid ambiguity       |
| Separate `date` call for timestamp | Rejected     | Two calls can disagree at minute boundary; derive via `subst`     |
| Minute precision in timestamp      | Rejected     | Two builds in the same minute collide; seconds costs 2 characters |

## Failure Registry

| Failure                                  | Impact                           | Recovery                                                       |
| ---------------------------------------- | -------------------------------- | -------------------------------------------------------------- |
| Stale filename reference in script or CI | Build or deploy fails            | Grep audit catches all refs; CI validates                      |
| `os-list-velocity.json` URL mismatch     | rpi-imager cannot find the image | CI computes URL from tag; test with pre-release first          |
| DMG artefact glob mismatch in CI         | Artefact upload fails            | Update glob in `mac-ci.yml`                                    |
| CI computes own `BUILD_TIME` out of sync | Timestamp mismatch across assets | Remove all independent `date` calls; single source in Makefile |

## Implementation Phases

| Phase | Scope                                      | Status      |
| ----- | ------------------------------------------ | ----------- |
| 1     | Makefile variables                         | Not started |
| 2     | Binary output filenames + Go upgrade logic | Not started |
| 3     | DMG naming                                 | Not started |
| 4     | RPi image naming                           | Not started |
| 5     | CI workflow consolidation                  | Not started |
| 6     | Documentation                              | Not started |

Phases 2 and 3 can run in parallel. Phase 4 follows 2. Phase 5 follows all.
