# Version-bump consolidation (v0.5.2)

- **Status:** Complete
- **Layers:** Cross-cutting (build tooling, CI)
- **Target:** v0.5.2; housekeeping and cleanup release
- **Companion plans:** [PDF generation migration to Go](pdf-go-chart-migration-plan.md) (deprecates `tools/pdf-generator/` wholesale — strengthens the case for pinning `pyproject.toml`)
- **Canonical:** this document

## Motivation

Every version bump touches five files across five components via
`scripts/set-version.sh --all`. Two of those files hold versions that
are never read at runtime and never published to a registry
(`public_html/package.json`, `tools/pdf-generator/pyproject.toml`).
A third (`web/package.json`) is read only to populate the
`app-web-version` HTML meta tag — a consumer we can satisfy from the
Makefile `VERSION` instead. Each bump produces a five-file commit diff,
increases merge-conflict surface on release branches, and adds CI noise
via `version-check.yml`.

Collapsing to two files (Makefile + Xcode project) eliminates three
unnecessary version-sync targets with no loss of runtime capability,
provided `web/scripts/set-build-env.js` is updated first so the meta
tag keeps showing a sensible version.

## Current state

| #   | File                                 | Consumer                                                                                 | Runtime?                                                         | Published?   |
| --- | ------------------------------------ | ---------------------------------------------------------------------------------------- | ---------------------------------------------------------------- | ------------ |
| 1   | `Makefile` (`VERSION :=`)            | Go binaries via `ldflags`; web build via `PUBLIC_BUILD_VERSION`                          | Yes                                                              | N/A          |
| 2   | `web/package.json`                   | `set-build-env.js` reads `pkg.version` → `PUBLIC_WEB_VERSION` → `<meta app-web-version>` | Yes — HTML meta tag (redundant with `app-build-version` from #1) | No (private) |
| 3   | `public_html/package.json`           | Eleventy docs site                                                                       | No                                                               | No (private) |
| 4   | `tools/pdf-generator/pyproject.toml` | Python PDF CLI                                                                           | No                                                               | No (private) |
| 5   | `project.pbxproj` (×6 entries)       | Xcode MARKETING_VERSION                                                                  | Yes — About screen                                               | No (local)   |

Files 3 and 4 are never read at runtime. File 2 is read only to feed a
meta tag that duplicates the Makefile-derived `app-build-version` — so
once `set-build-env.js` mirrors `PUBLIC_BUILD_VERSION` into
`PUBLIC_WEB_VERSION`, pinning `web/package.json` becomes safe.

### Version flow today

```
Makefile VERSION :=
  ├─► Go binary (ldflags -X)                     ← runtime ✓
  ├─► web build (env PUBLIC_BUILD_VERSION)       ← runtime ✓ (app-build-version meta)
  ├─► web/package.json → PUBLIC_WEB_VERSION      ← runtime ✓ (app-web-version meta; duplicates above)
  ├─► public_html/package.json                   ← dead metadata
  ├─► pyproject.toml                             ← dead metadata
  └─► project.pbxproj MARKETING_VERSION          ← runtime ✓
```

## Findings

| Area                       | Current state                                                                                                           | Severity   | Release view                               |
| -------------------------- | ----------------------------------------------------------------------------------------------------------------------- | ---------- | ------------------------------------------ |
| `web/package.json`         | Feeds `PUBLIC_WEB_VERSION` → `<meta app-web-version>`; duplicates `app-build-version`                                   | Low        | Noise in every bump commit                 |
| `public_html/package.json` | Pure npm metadata; never displayed                                                                                      | Low        | Noise in every bump commit                 |
| `pyproject.toml`           | Never published; never read at runtime; whole tool slated for removal per [PDF→Go plan](pdf-go-chart-migration-plan.md) | Low        | Noise in every bump commit                 |
| `project.pbxproj`          | Read by Xcode at build time; sed replacement is reliable                                                                | N/A — keep | Required                                   |
| `version-check.yml`        | Checks three files (`Makefile`, `web/package.json`, `pyproject.toml`) for drift                                         | Low        | False-positive surface after consolidation |
| `set-build-env.js`         | Reads `package.json` for `PUBLIC_WEB_VERSION` separately from `PUBLIC_BUILD_VERSION`                                    | Low        | Two version variables where one suffices   |
| `set-version.sh:3`         | Header comment advertises a `--deploy` flag that does not exist in the argument parser                                  | Trivial    | Stale help text; easy to strip             |
| `Makefile:1490`            | `version-exact` example echoes the same nonexistent `--deploy` flag                                                     | Trivial    | Stale help text; easy to strip             |

## Design / approach

Pin the three metadata files to `"0.0.0"` and stop updating them.
Before doing so, sever `web/package.json`'s runtime consumer by having
`set-build-env.js` derive `PUBLIC_WEB_VERSION` from `PUBLIC_BUILD_VERSION`;
this is a **prerequisite**, not a cleanup step — otherwise the shipped
HTML will advertise `app-web-version="0.0.0"`. Remove the corresponding
`--web`, `--docs`, and `--pdf` targets from `set-version.sh`. Update CI
and build scripts to match.

After consolidation, `set-version.sh --all` updates two files:

```
Makefile VERSION :=
  ├─► Go binary (ldflags -X)             ← unchanged
  ├─► web build (env PUBLIC_BUILD_VERSION) ← unchanged
  └─► project.pbxproj MARKETING_VERSION   ← unchanged
```

### What you lose

- `pip show pdf-generator` reports `0.0.0` — irrelevant for an
  unpublished local tool, and especially irrelevant given
  [the PDF→Go migration](pdf-go-chart-migration-plan.md) which
  deprecates `tools/pdf-generator/` entirely in a subsequent release.
- `pnpm list` in `web/` reports `0.0.0` — irrelevant for a private
  package built via `make build-web`.
- `<meta name="app-web-version">` tracks the Makefile `VERSION` rather
  than `web/package.json`. In practice this was the same value before
  (both sources were kept in sync by `set-version.sh`); after Item 3
  it is derived from a single canonical source.
- Running `pnpm build` outside of `make build-web` embeds no version —
  already the case for git SHA and build time, so no new footgun.

### Why keep `project.pbxproj`

Xcode reads MARKETING_VERSION from build settings at compile time. The
alternatives (Run Script build phase, `PlistBuddy` injection) add
fragile Xcode scripting that breaks across Xcode updates. The current
sed replacement in `set-version.sh` is well-understood and reliable.

## Scope

Items are listed in execution order. Item 1 is a hard prerequisite for
Item 2 — swapping the order ships `app-web-version="0.0.0"` to the
live site.

### Item 1: Simplify `set-build-env.js` (prerequisite)

**Summary:** Derive `PUBLIC_WEB_VERSION` from `PUBLIC_BUILD_VERSION`
instead of reading `package.json`. This severs `web/package.json`'s
last runtime consumer and must land before Item 2.

**Steps:**

1. Remove the `readFileSync` / `dirname` / `fileURLToPath` imports and
   the `package.json` read from `web/scripts/set-build-env.js`
2. Set `PUBLIC_WEB_VERSION = buildVersion` (mirror of
   `PUBLIC_BUILD_VERSION`) — or remove the variable entirely if no
   consumer requires it to remain distinct
3. Update the in-file comment that currently calls `package.json` the
   "canonical source" — after this change, the Makefile is canonical
4. Verify `make build-web` still embeds the correct version in
   `app-build-version` and `app-web-version` meta tags

**Milestone:** v0.5.2

### Item 2: Pin dead-metadata versions

**Summary:** Set `web/package.json`, `public_html/package.json`, and
`tools/pdf-generator/pyproject.toml` versions to `"0.0.0"`. Safe only
after Item 1 lands.

**Steps:**

1. Edit `web/package.json`: set `"version": "0.0.0"`
2. Edit `public_html/package.json`: set `"version": "0.0.0"`
3. Edit `tools/pdf-generator/pyproject.toml`: set `version = "0.0.0"`

**Milestone:** v0.5.2

### Item 3: Strip targets from `set-version.sh`

**Summary:** Remove `--web`, `--docs`, and `--pdf` targets and their
update functions. Also strip the stale `--deploy` references that
currently advertise a flag the parser does not implement.

**Steps:**

1. Remove `update_web()`, `update_docs()`, `update_pdf()` functions
2. Remove `--web`, `--docs`, `--pdf` from the argument parser and the
   `--all` expansion
3. Narrow the `--all` expansion so it covers only `--makefile` +
   `--mac` (behaviour change, not a rename)
4. Update usage text and examples
5. Remove the stale `--deploy` mention in the `set-version.sh:3`
   header comment
6. Remove the stale `--deploy` example echoed by `Makefile:1490`
   (`version-exact` help)

**Milestone:** v0.5.2

### Item 4: Update `version-check.yml`

**Summary:** Stop checking pinned files for version drift. Only three
of the original five files are actually checked today
(`Makefile`, `web/package.json`, `tools/pdf-generator/pyproject.toml`);
two of those three drift checks become obsolete.

**Steps:**

1. Remove the PDF generator version-change detection step
2. Remove the web `package.json` version-change detection step
3. Keep the Makefile radar-version check and the migration detection

**Milestone:** v0.5.2

### Item 5: Update `version-bump.sh`

**Summary:** Verify `version-bump.sh` still works with the reduced
target set.

**Steps:**

1. Confirm `version-bump.sh` calls `set-version.sh <VER> --all` — no
   change needed once `--all` is redefined correctly in Item 3
2. Run `make version-bump` on a throwaway branch and confirm only the
   Makefile and `project.pbxproj` are modified

**Milestone:** v0.5.2

## Dependencies

None. All changes are internal to build tooling.

## Risks

| Risk                                                                  | Likelihood | Impact                                                                                                                 | Mitigation                                                                                                                   |
| --------------------------------------------------------------------- | ---------- | ---------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------- |
| Item 2 lands before Item 1                                            | Medium     | Shipped site advertises `app-web-version="0.0.0"` until Item 1 lands; any external consumer of that meta tag is misled | Order items as written; reviewers must block if the PR pins without Item 1                                                   |
| External consumer of `app-web-version` meta tag (support tools, etc.) | Low        | Meta tag value tracks Makefile `VERSION` instead of `web/package.json` — previously the same value, now by derivation  | `grep` showed only `web/src/app.html` consumes `PUBLIC_WEB_VERSION`; flag before merge if new consumers appear               |
| Future decision to publish `pdf-generator` to PyPI                    | Very low   | Would need to re-add `--pdf` target                                                                                    | Superseded by [PDF→Go migration](pdf-go-chart-migration-plan.md) — `pdf-generator` is scheduled for removal, not publication |
| Contributor runs `pnpm build` directly without `make`                 | Low        | Missing version string in web build                                                                                    | Already the case for git SHA and build time; documented in `web/README.md`                                                   |
| `version-check.yml` still references removed files                    | Medium     | CI false positives                                                                                                     | Item 4 addresses directly                                                                                                    |

## Checklist

### Outstanding

Ordered by execution — items under Item 1 must land before Item 2.

**Item 1 — simplify `set-build-env.js` (prerequisite):**

- [x] Remove the `readFileSync` / `dirname` / `fileURLToPath` imports and `package.json` read from `web/scripts/set-build-env.js` (`S`)
- [x] ~~Set `PUBLIC_WEB_VERSION = buildVersion` (mirror `PUBLIC_BUILD_VERSION`)~~ — went one step further: the meta tag was already redundant with `app-build-version`, so `<meta name="app-web-version">` was removed from `web/src/app.html` and the `PUBLIC_WEB_VERSION` env-var assignment dropped. `app-build-version` is now the single canonical version meta tag.
- [x] Update the stale `// canonical source` comment in `set-build-env.js` (`S`)
- [x] Verify `make build-web` embeds the correct version in the `app-build-version` meta tag (`S`)

**Item 2 — pin metadata:**

- [x] Pin `web/package.json` version to `"0.0.0"` (`S`)
- [x] Pin `public_html/package.json` version to `"0.0.0"` (`S`)
- [x] Pin `docs_html/package.json` version to `"0.0.0"` (`S`) — added as an extension to the original plan; the embedded offline-docs site was a fourth dead-metadata file that drifted from the Makefile (was at `0.5.1-pre8` while Makefile was `0.5.1-pre16`)
- [x] ~~Pin `tools/pdf-generator/pyproject.toml` version to `"0.0.0"`~~ — superseded: `tools/pdf-generator/` was removed from the repo entirely as part of the [PDF→Go migration](pdf-go-chart-migration-plan.md), so there is no longer a file to pin

**Item 3 — strip `set-version.sh`:**

- [x] Remove `update_web()`, `update_docs()` from `scripts/set-version.sh` (`S`) — `update_pdf()` did not exist in the current codebase; removed only the `update_web()` and `update_docs()` functions and the `--web`/`--docs` flags
- [x] Update `--all` expansion in `set-version.sh` to cover only `--makefile` + `--mac` (`S`)
- [x] Update usage text and examples in `set-version.sh` (`S`)
- [x] Remove the stale `--deploy` reference in `set-version.sh:3` (`S`)
- [x] Remove the stale `--deploy` example in the `Makefile:1490` `version-exact` help (`S`)

**Item 4 — CI:**

- [x] Remove the web-frontend version-drift step from `.github/workflows/version-check.yml` (`S`)
- [x] ~~Remove the PDF-generator version-drift step from `.github/workflows/version-check.yml`~~ — no such step exists in the current workflow; superseded with the pdf-generator removal
- [x] Keep the Makefile radar-version check and migration detection (`S`)

**Item 5 — verification:**

- [x] Verify `make version-bump` on a throwaway branch modifies only `Makefile` + `project.pbxproj` (`S`)
- [x] Verify `make build-radar-local` is unaffected (`S`)

### Deferred

- [ ] Eliminate `project.pbxproj` version duplication via Xcode build phase script: complexity exceeds value; revisit if Xcode sed replacement becomes unreliable
