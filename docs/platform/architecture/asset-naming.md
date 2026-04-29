# Asset naming conventions

Canonical naming patterns for release artefacts, development builds, and
platform-specific binaries across the velocity.report project.

## Product names

| Product          | Asset name           | Case rule  |
| ---------------- | -------------------- | ---------- |
| Server           | `velocity-report`    | kebab-case |
| macOS Visualiser | `VelocityVisualiser` | PascalCase |
| Management CLI   | `velocity-ctl`       | kebab-case |

The macOS visualiser keeps PascalCase because it is the user-facing
application name (Finder, DMG volume, menu bar, About dialog). See
[.github/knowledge/coding-standards.md](../../../.github/knowledge/coding-standards.md) § Product Names.

## Version string

Single source of truth: `VERSION` in `Makefile`.

Release versions follow SemVer: `MAJOR.MINOR.PATCH` (e.g. `0.5.1`).
Pre-release tags append a hyphen suffix: `0.5.1-pre1`, `0.6.0-rc1`.

**No leading zeros in version segments.** [web/package.json](../../../web/package.json) and
[public_html/package.json](../../../public_html/package.json) are validated by npm's strict SemVer parser.
A version like `0.5.04` is invalid: the patch segment `04` has a leading
zero. Use `0.5.4` instead. See § Version Validity Analysis.

## Two filename tiers

### Release: tagged GitHub releases

Human-readable, stable, linkable. No date, no SHA.

```
velocity-report-0.5.1-linux-arm64
velocity-report-0.5.1-darwin-arm64
velocity-ctl-0.5.1-linux-arm64
velocity-report-0.5.1.img.xz
VelocityVisualiser-0.5.1.dmg
```

### Dev: CI artefacts and local builds

Date-time prefix for sortability; 7-char git SHA suffix for traceability.

```
20260407T142345Z-velocity-report-0.5.1.pre1-linux-arm64-a1b2c3d
20260407T142345Z-velocity-ctl-0.5.1.pre1-linux-arm64-a1b2c3d
20260407T142345Z-velocity-report-0.5.1.pre1-a1b2c3d.img.xz
20260407T142345Z-VelocityVisualiser-0.5.1.pre1-a1b2c3d.dmg
```

## Naming grammar

```
Release:  {product}-{version}[-{os}-{arch}]{ext}
Dev:      {datetime}-{product}-{version}[-{os}-{arch}]-{sha7}{ext}
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

## Full asset catalogue

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

## Version validity analysis

| Surface         | File                                                                | Validates SemVer?   | Effect of `0.5.04`           |
| --------------- | ------------------------------------------------------------------- | ------------------- | ---------------------------- |
| npm (web)       | [web/package.json](../../../web/package.json)                       | **Yes: strict**     | **Rejects with parse error** |
| npm (docs)      | [public_html/package.json](../../../public_html/package.json)       | **Yes: strict**     | **Rejects with parse error** |
| Go version pkg  | [internal/version/version.go](../../../internal/version/version.go) | No: display only    | Passes                       |
| Makefile        | `Makefile`                                                          | No: string constant | Passes                       |
| CI workflows    | `.github/workflows/*.yml`                                           | No: substitution    | Passes                       |
| Xcode           | `project.pbxproj` MARKETING_VERSION                                 | No: string          | Passes                       |
| rpi-imager JSON | [image/os-list-velocity.json](../../../image/os-list-velocity.json) | No                  | Passes                       |

Two surfaces hard-block: [web/package.json](../../../web/package.json) and [public_html/package.json](../../../public_html/package.json).
Every CI run validates the version field against strict SemVer.

## Makefile variables

All naming derives from `Makefile` § VERSION INFORMATION:

| Variable           | Value                                                               |
| ------------------ | ------------------------------------------------------------------- |
| `VERSION`          | Current version (SemVer)                                            |
| `GIT_SHA`          | Full commit SHA from `git rev-parse HEAD` (falls back to `unknown`) |
| `BUILD_TIME`       | UTC timestamp in ISO 8601 (`%Y-%m-%dT%H:%M:%SZ`)                    |
| `GIT_SHA_SHORT`    | First 7 characters of `GIT_SHA`                                     |
| `BUILD_TS_COMPACT` | `BUILD_TIME` with hyphens and colons stripped (filesystem-safe)     |
| `DEV_VERSION`      | `VERSION` with hyphens replaced by dots (e.g. `0.5.1.pre1`)         |

`BUILD_TS_COMPACT` is derived from `BUILD_TIME` via `subst`: one `date`
call per build invocation, one source of truth within that invocation.

**Compute-once rule.** Each build environment computes the timestamp
exactly once at the start of its run, then threads it to every consumer:

| Environment        | Where computed                   | Propagation                                |
| ------------------ | -------------------------------- | ------------------------------------------ |
| `make` targets     | `Makefile` § VERSION INFORMATION | Shell variables → Go ldflags, filenames    |
| `build-image.sh`   | Section 2 (after arg parsing)    | `$BUILD_TIME` / `$BUILD_TS_COMPACT` in env |
| CI (`build-image`) | First step → `$GITHUB_ENV`       | All subsequent steps read from env         |
| CI (`mac-ci`)      | Per-job (build vs package)       | Local shell variable within step           |

Different build environments (Make vs CI vs script) may produce
different timestamps: that is expected because they are separate runs.
The invariant is: **within a single run, every artefact carries the
same timestamp.**

## Boundary diagram

```
     ┌──────────────────────┐  ┌──────────────────────┐  ┌──────────────────────┐
     │    make (local/CI)   │  │   build-image.sh     │  │   CI workflow step   │
     │  BUILD_TIME computed │  │  BUILD_TIME computed  │  │  BUILD_TIME computed │
     │  once in Makefile    │  │  once at script start │  │  once → $GITHUB_ENV  │
     └──────────┬───────────┘  └──────────┬───────────┘  └──────────┬───────────┘
                │                         │                         │
     ┌──────────┼──────────┐              │              ┌──────────┼──────────┐
     │          │          │              │              │          │          │
     ▼          ▼          ▼              ▼              ▼          ▼          ▼
  Go bins    .dmg     BuildInfo      .img.xz         binaries    MOTD     .img.xz
  (ld­flags)  (name)   .swift        (name)          (Docker)   (stamp)   (name)
```

Each column is a separate build environment. The invariant is
one `date -u` call per environment, threaded to every consumer.

## Alternatives considered

| Alternative                        | Verdict      | Reason                                                            |
| ---------------------------------- | ------------ | ----------------------------------------------------------------- |
| Date at end                        | Rejected     | Cannot `ls`-sort by date                                          |
| SHA in release filenames           | Rejected     | Noisy; version tag is sufficient                                  |
| Keep PascalCase for DMG            | **Accepted** | Brand identity in Finder, menu bar, About dialog                  |
| `+sha` separator (old DMG style)   | Rejected     | `+` is reserved in URLs, causes shell escaping pain               |
| Dots in dev pre-release            | Adopted      | Hyphens conflict with field separator; dots avoid ambiguity       |
| Separate `date` call for timestamp | Rejected     | Multiple calls within one run can disagree at minute boundary     |
| Minute precision in timestamp      | Rejected     | Two builds in the same minute collide; seconds costs 2 characters |

## Failure registry

| Failure                                  | Impact                           | Recovery                                                        |
| ---------------------------------------- | -------------------------------- | --------------------------------------------------------------- |
| Stale filename reference in script or CI | Build or deploy fails            | Grep audit catches all refs; CI validates                       |
| `os-list-velocity.json` URL mismatch     | rpi-imager cannot find the image | CI computes URL from tag; test with pre-release first           |
| DMG artefact glob mismatch in CI         | Artefact upload fails            | Update glob in `mac-ci.yml`                                     |
| Duplicate `date` calls within one run    | Timestamp drift across artefacts | Compute once at start of each build environment; thread via env |

## Implementation phases

| Phase | Scope                                                | Status   |
| ----- | ---------------------------------------------------- | -------- |
| 1     | Makefile variables                                   | Complete |
| 2     | Binary output filenames + Go upgrade logic           | Complete |
| 3     | DMG naming                                           | Complete |
| 4     | RPi image naming                                     | Complete |
| 5     | CI workflow and release wiring consolidation         | Complete |
| 6     | Documentation sweep across build, release, and setup | Complete |

Phases 1–4 landed in PR #457. Phase 5 consolidated `build-image.sh` and
`build-image.yml` to compute `BUILD_TIME` once per run. Phase 6 is this
document update.
