# Offline docs as a separate release asset

Ship the `/docs/` site as one architecture-independent archive beside the `velocity` binary,
installed into the same versioned directory, instead of embedding it in every binary. The release
stays one versioned unit; the binary stops carrying documentation bytes, and CI stops rebuilding
the binary to prove a docs edit.

- **Status:** Proposed
- **Decisions:** D-09 and D-30 (one binary, embedded assets: this plan proposes amending them so
  the docs site is a versioned sibling file, not an embed), D-10 (one image), D-20 (the `/docs/`
  URL is published and does not move), D-22 (operator-run upgrades through `release.json`), D-26
  (static release binaries), D-32 (planning limits)
- **Layers:** Cross-cutting (Go server, device upgrade, release pipeline, Pi image, CI)
- **Target:** v0.7.0 (Deployment and packaging): it changes what a release contains and how the
  device upgrades, which is that slot's theme
- **Companion plans:** [embedded-offline-docs-site.md](embedded-offline-docs-site.md) (Parked;
  owns the site itself: shell, search, link checks),
  [binary-size-reduction-plan.md](binary-size-reduction-plan.md) (Parked; the size budget),
  [deploy-distribution-packaging-plan.md](deploy-distribution-packaging-plan.md)
- **Canonical:** [offline-docs-site.md](../platform/operations/offline-docs-site.md)

## Motivation

The docs site is the largest thing in the binary and the thing that changes most often. Measured
on November 9, 2026 at `b8ea18d4f`:

| Measure                                       | Value                          |
| --------------------------------------------- | ------------------------------ |
| `docs_html/_site`                             | 170 MB, 730 HTML pages         |
| Of which the repeated sidebar tree            | ~130 MB (178 KB of each page)  |
| `velocity-report-local` (macOS, dev build)    | 266 MB                         |
| CI artefact, amd64 + arm64 static, compressed | 139 MB                         |
| Site with the sidebar rendered once           | 45 MB, 22 MB as a deflated zip |

The sidebar fix (rendering the tree once and loading it on the page) removes most of the weight
and is worth doing on its own. It does not remove the coupling:

- **The binary cannot meet its size budget.** The [binary size plan](binary-size-reduction-plan.md)
  keeps < 40 MB as the preferred ceiling; the Go code alone is about 38 MB, so even the fixed 45 MB
  site doubles the binary. The consolidation plan already names the way out for a payload that
  does not fit: install it under `/opt/velocity-report/` instead of embedding it, as proposed for
  Typst.
- **Every upgrade downloads the docs again.** Each architecture's binary carries its own copy of
  the same site, and an operator on a slow or metered link fetches it again for a release that
  changed one line of Go.
- **A docs edit is a binary change.** CI builds and boots both static binaries to prove a docs-only
  pull request, which on #995 took 9.9 minutes against 2.2 for the docs job itself. The binary's
  hash moves whenever any Markdown file in `docs/` or `data/` moves, so reproducibility and
  provenance checks cannot tell a code change from a typo fix.
- **Build order is forced.** `make build-radar-local` needs pnpm and the Eleventy build before the
  Go compile, so a checkout without pnpm cannot build the server at all.

Doing nothing keeps those costs, and they grow with every report added to `docs/` and `data/`.

## Current state

- `assets.go` embeds `all:docs_html/_site` as `DocsSiteFiles` and `docs_html/stub-index.html` as
  `DocsSiteStub`. `internal/docsite` serves either the embed or a disk directory, chosen by
  `--docs-source=embed|disk` (default `embed`, disk path `docs_html/_site`).
- When the embed holds only the stub, `/docs/` serves the stub page. A binary built without docs
  therefore already runs; it just has nothing to show.
- `scripts/write-embedded-docs-manifest.py` writes `.embed-manifest`; the static Docker build
  checks it with `sha256sum --check` before compiling.
- The build stamps `VELOCITY_DOCS_VERSION`, `VELOCITY_DOCS_GIT_SHA` and `VELOCITY_DOCS_BUILD_TIME`
  into the site, and repository source links resolve to that git revision.
- `velocity device upgrade` installs `versions/<v>/velocity` and swaps `current -> versions/<v>`
  atomically; `--binary <file>` is the offline path. Image stage `01-velocity-binaries` installs
  the same layout from CI artefacts, checked against `SHA256`.
- `release.json` lists one binary per architecture with its hash and size; `make
check-release-hashes` validates them.
- The hub doc states that binary, UI, documentation and runtime dependencies remain one versioned
  release unit. This plan keeps that at the release level and changes it at the file level.

## Design / approach

### Options

| Option                                     | What ships                                                                           | Burden it assigns                                                                                             |
| ------------------------------------------ | ------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------- |
| A. Embed, sidebar fixed                    | 45 MB of docs in each binary                                                         | Docs edits still rebuild and re-prove both binaries; each upgrade downloads docs per architecture             |
| B. Embed a compressed archive              | One zip inside each binary (~22 MB)                                                  | Halves the bytes; keeps every coupling above                                                                  |
| C. Versioned sibling archive (recommended) | `velocity-docs-<v>-<sha>.zip` once per release, installed as `versions/<v>/docs.zip` | Upgrade, image and release pipeline each learn a second file                                                  |
| D. Independently versioned docs package    | Docs released on their own cadence                                                   | Breaks the version match and the pinned source links; two things to keep in step on a device with no internet |

Option C keeps what the embed gives (docs that match the running version, available with no
network) and drops what it costs. Option D is rejected: matched versions are the point of offline
docs. Option B is a fallback if C stalls, and could land first as it touches only the build and
`internal/docsite`.

### The archive

- One zip per release, architecture-independent, named per
  [asset naming](../platform/architecture/asset-naming.md) (to be added there):
  `velocity-docs-<version>-<gitsha>.zip`.
- Built by a new `make package-docs-offline` from `docs_html/_site`: entries sorted, timestamps
  fixed from the commit time, no extra fields, so the archive is reproducible byte for byte.
- Carries a `build-info.json` at its root with the version, git SHA and build time already stamped
  into the site, plus the manifest that `.embed-manifest` holds today.

### Serving straight from the zip

The server opens `versions/<v>/docs.zip` with `archive/zip` and serves it through `http.FS`;
`zip.Reader` already implements `fs.FS`. No extraction step, so no path-traversal surface on the
device, no partial trees after a power cut, and the upgrade swaps one file. Entries are deflated;
decompressing a 35 KB page per request is negligible on a Pi 4, and responses can carry `ETag`
from the entry's CRC.

`--docs-source` gains `auto`, the new default:

1. `disk` if `--docs-dir` names a directory (development, unchanged).
2. Otherwise `docs.zip` beside the resolved executable, when its `build-info.json` version matches
   the binary's. A mismatch is served with a banner naming both versions, not refused: wrong-version
   docs on a device with no internet beat none.
3. Otherwise the embedded stub page, which names the missing file and the upgrade command that
   installs it.

The docs are never required for the server to start. The embed of the
full site is removed; the stub stays embedded.

### Release, upgrade and image

- **Release:** the release workflow attaches the zip once; `release.json` gains a `docs` entry
  with URL, SHA-256 and size, validated by `check-release-hashes`.
- **Upgrade:** `velocity device upgrade` downloads and verifies the zip, writes
  `versions/<v>/docs.zip`, and only then swaps `current`, so a rollback restores matching docs.
  The offline path gains `--docs <file>`; without it, the previous version's zip is copied
  forward and the server shows the mismatch banner until the operator supplies the right one.
- **Image:** stage `01-velocity-binaries` installs `docs.zip` next to the binary and checks it
  against `SHA256` the same way.
- **Retention:** pruning old `versions/<v>/` directories removes their zips with them; the docs add
  ~22 MB per retained version rather than ~45 MB per binary.

### CI

- Static build CI builds against the stub and no longer needs pnpm, Eleventy or the docs paths.
- Offline docs CI builds the site, packages the zip, checks reproducibility (build twice, compare
  hashes) and runs the route verifier against a server reading the zip.
- The ARM64 VM gate installs binary and zip together and checks `/docs/` after upgrade and
  rollback.

## Scope

### Item 1: serve the site from a zip

**Summary:** `internal/docsite` serves an `fs.FS` from a zip, with `--docs-source=auto` and the
version check, while the full embed still ships.

**Steps:**

1. Add `make package-docs-offline` and reproducibility tests for the archive.
2. Add the zip-backed handler, `auto` source and mismatch banner, with handler tests.
3. Point `scripts/verify-embedded-docs-server.py` at a server reading the zip.

**Milestone:** v0.7.0

### Item 2: ship the zip and stop embedding the site

**Summary:** Release, `release.json`, image stage 01 and `velocity device upgrade` carry the zip;
the binary embeds only the stub.

**Steps:**

1. Release workflow attaches the zip; `release.json` and `check-release-hashes` learn the entry.
2. Upgrade writes `versions/<v>/docs.zip` before the swap; add `--docs`; extend the upgrade flow
   tests in `internal/ctl` (D-22's keeper) and the ARM64 VM gate.
3. Image stage 01 installs and verifies the zip.
4. Remove `all:docs_html/_site` from `assets.go`; drop the docs build from the Go build
   prerequisites and from Static build CI.
5. Record the decision as a new D entry amending D-09 and D-30, update the hub doc, asset naming
   and the compatibility register if any caller still expects the embed.

**Milestone:** v0.7.0

## Dependencies

- The sidebar fix lands first: it sets the size the zip is measured against and removes the
  per-page tree the route verifier would otherwise crawl 730 times.
- Static build CI's docs path filter (docs-only pull requests no longer run it) lands first and
  moves the route check into Offline docs CI, which Item 1 then points at the zip.
- `release.json` and the upgrade manifest format belong to D-22; a new field must keep older
  `velocity device upgrade` binaries working (they ignore unknown entries; verify).

## Risks

| Risk                                                             | Likelihood | Impact | Mitigation                                                                                                      |
| ---------------------------------------------------------------- | ---------- | ------ | --------------------------------------------------------------------------------------------------------------- |
| Operator upgrades offline with `--binary` only and gets old docs | Medium     | Low    | Copy the previous zip forward, show the version banner, print the `--docs` hint at upgrade time                 |
| Older upgrade binaries choke on the new `release.json` entry     | Low        | High   | Test the current release's upgrader against the new manifest before shipping Item 2                             |
| A malformed or hostile zip                                       | Low        | Medium | Verified hash before install; zip served read-only through `fs.FS`, never extracted; Malory reviews the handler |
| Archive not reproducible across hosts                            | Medium     | Low    | Fixed timestamps and order; CI builds twice and compares                                                        |
| Docs and binary drift in the image build                         | Low        | Medium | Stage 01 checks both against one `SHA256` file written by the same workflow                                     |

## Design decisions

None recorded yet. Item 2 step 5 records the amendment to D-09 and D-30 when the owner accepts
option C.

## Open questions

- Should the mismatch case serve the old docs with a banner, as proposed, or the stub? The banner
  keeps help available on a device with no network; the stub avoids advice about the wrong
  version.
- Does `/public_html/`, already a staged directory on the image, move into the same archive or
  stay a directory? Keeping it separate respects the two-site split in the hub doc; folding it in
  gives one docs asset.
- Is a docs-only re-release (new zip, same binary) worth supporting? It would let a docs fix reach
  devices without a binary upgrade, but it makes the docs version differ from the binary's by
  design.

## Checklist

### Outstanding

- [ ] Zip packaging target with reproducibility test (`S`)
- [ ] Zip-backed handler, `--docs-source=auto` and version banner (`M`)
- [ ] Route verifier against the zip in Offline docs CI (`S`)
- [ ] Release asset, `release.json` entry and hash check (`S`)
- [ ] Upgrade writes the zip before the swap, `--docs` flag, flow tests and VM gate (`M`)
- [ ] Image stage 01 installs the zip (`S`)
- [ ] Remove the site embed; decision entry; hub doc and asset naming updates (`S`)
