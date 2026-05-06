# Embedded offline docs site (`/docs/`)

- **Status:** Milestone 1 shipped in PR #480. Milestone 2 planned: finish the remaining operator polish, add shared public guide/tool surfaces, and ship offline search.
- **Owner:** Architecture (Grace) → handed off to Appius / on-branch implementation in PR #480.
- **Scope mode:** HOLD — minimum viable architecture; no new dependencies on cloud, no new languages.
- **Canonical:** [offline-docs-site.md](../platform/operations/offline-docs-site.md)

See [§8 Milestone plan](#8-milestone-plan) and [§9 Implementation status](#9-implementation-status-2026-04-29) for what shipped in Milestone 1 and what remains in Milestone 2.

---

## 1. Problem statement & use case

Field operators deploy velocity.report on a Raspberry Pi in locations that may have no internet (rural roads, locked-down council networks, temporary monitoring sites). When something goes wrong — a misbehaving radar, a confusing chart, an unexpected percentile — the operator needs to read the docs from the device itself, not from `velocity.report` over the public web. The Go server already binds to `:8080` (HTTP API + Svelte frontend) and `:50051` (gRPC). The internal docs are served from the same HTTP surface at `/docs/`, so localhost, LAN access, and single-port reverse proxies such as Tailscale Serve all reach the same embedded documentation. The on-disk content already exists (`docs/`, `data/structures/`, `data/maths/`, root `*.md`); the work is wrapping it in a navigable, link-safe Eleventy site that ships with the binary and can be browsed on the Pi's local network.

"Offline" is the term of art used throughout this plan. It means the same thing it always does: a deployment with no expectation of internet reachability, where every byte the operator needs must already be on the device.

---

## 2. Recommended approach: separate Eleventy project at `docs_html/`

**Recommendation: keep the offline operator docs as a second, separate Eleventy project.** I am confirming, not pushing back on, the user's intuition.

**Decision:** new project at `docs_html/` (sibling of `public_html/`), distinct config, distinct output, distinct embed.

### Why separate, not merged

Three structural reasons, in priority order:

1. **Audience separation is a privacy boundary, not a stylistic preference.** `public_html/` is the public marketing/docs site that ships to `velocity.report`. `docs/` contains plans, devlogs, design notes, internal architecture decisions, and security threat models. Merging means every commit risks accidentally publishing internal-only material to the open web. A separate project lets the offline-docs build glob `docs/**` freely, while `public_html/` continues to opt-in only the curated public pages it already lists. A merge inverts the safer default.
2. **The two sites have different lifecycle owners.** `public_html/` is versioned with the marketing release cadence and is hand-curated by Terry; `docs_html/` will be a near-mechanical build over the repo's existing markdown trees, refreshed every time the binary is rebuilt. Coupling them means a typo in an internal devlog can block a marketing deploy, and a Tailwind theme update on the public site forces a Go rebuild.
3. **The split test passes for separation.**
   - Different owned surface (operator on-device vs public web).
   - Different long-lived architecture (embedded into a Go binary vs static site deploy).
   - Different stable reader expectation (engineer/operator vs prospect/customer).

### Cost of duplication, and how we keep it small

The legitimate concern with two Eleventy projects is config and theme drift. We mitigate this by **deliberately keeping `docs_html/` minimal**: no Tailwind, no marketing layouts, no `_data/release.json` plumbing, no syntax-highlight plugin until proven necessary. A single layout, a sidebar built from a generated nav data file, and the same `markdown-it` + `markdown-it-anchor` pair already in use. If the two projects ever genuinely converge on shared tooling, we can extract a small `eleventy-shared/` package later — that is a graduation, not a precondition.

### Milestone 2 extension: shared public guide/tool surfaces without duplicated source

Milestone 2 is the one place where the separation between `public_html/` and `docs_html/` should be **porous in content, but not in shell**.

The recommendation is:

1. Keep **two Eleventy shells**: `public_html/` remains the public site, `docs_html/` remains the embedded operator site.
2. Add **one neutral shared source root** for the selected public-facing onboarding surfaces that must also ship offline: the setup guide, the protractor tool body, the protractor JavaScript, and the shared images/diagrams those pages need.
3. Make each site consume that shared source through **thin wrapper pages** that provide the site-local layout, navigation metadata, permalink, and styling.
4. Keep `/docs` as a **deployment mount**, not a content route. Shared pages should target logical URLs like `/tools/protractor/` and `/guides/setup/`; the Go server mount is what turns those into `/docs/tools/protractor/` and `/docs/guides/setup/` externally on the device.
5. Replace hard-coded root-relative asset and page URLs in shared content with **site-local helpers** so the same canonical source resolves correctly in both sites.

That keeps the privacy boundary intact, preserves site-specific styling, and stops the protractor JavaScript and guide images from diverging across two copies.

### Alternatives considered

| Option                                                                 | Effort | Risk | Why rejected / accepted                                                                                                        |
| ---------------------------------------------------------------------- | ------ | ---- | ------------------------------------------------------------------------------------------------------------------------------ |
| **A. Separate `docs_html/` project (recommended)**                     | M      | Low  | Clean audience boundary; minimum viable; matches existing `public_html/` build idiom.                                          |
| B. Merge into `public_html/` with `permalink: false` and exclude rules | S      | High | One config bug exposes internal plans on the public site. Exclusion is opt-out by default — wrong direction for privacy.       |
| C. Render markdown at request time in Go (no Eleventy)                 | M      | Med  | Eliminates a build step but reinvents nav, anchors, syntax highlighting, link rewriting. Loses dev parity with `public_html/`. |
| D. Do nothing; operators read raw `.md` over SSH                       | XS     | Med  | Survives outages but fails non-technical operators. Violates "make the abstract approachable".                                 |

**Do A. Here is why:** it is the only option that maintains the privacy boundary by default, reuses tooling the team already understands, and keeps each site small enough that drift between them is visible at code-review time.

---

## 3. Architecture sketch

### System boundary diagram

```
                      Raspberry Pi (offline)
   ┌──────────────────────────────────────────────────────────────┐
   │  velocity-report (single Go binary, systemd service)         │
   │                                                              │
   │   ┌────────────────────┐  ┌─────────────────────────────┐    │
   │   │ HTTP :8080         │  │ gRPC :50051                 │    │
   │   │ /api/* + Svelte    │  │ FrameBundle stream          │    │
   │   │ /docs/* offline    │  │ (existing)                  │    │
   │   │ docs               │  │                             │    │
   │   └────────────────────┘  └─────────────────────────────┘    │
   │             ▲                                                │
   │             │ go:embed all:docs_html/_site                   │
   │             │                                                │
   └─────────────┼────────────────────────────────────────────────┘
                 │
   build-time:   │
                 │
   ┌─────────────┴────────────────────────────────────────────────┐
   │  make build-docs-offline                                     │
   │  (1) symlink content roots into docs_html/src/ (see §4)      │
   │  (2) eleventy build: docs_html/ → docs_html/_site/           │
   │  (3) check-docs-offline-links: validate the rendered tree    │
   │  (4) go build embeds docs_html/_site/                        │
   └──────────────────────────────────────────────────────────────┘
```

### Where the route lives

A new package `internal/docsite/` owns the embed and the HTTP handler. `cmd/radar/radar.go` mounts that handler on the existing API/frontend mux at `/docs/`.

| Flag            | Default | Purpose                                       |
| --------------- | ------- | --------------------------------------------- |
| `--docs-source` | `embed` | `embed` (default) or `disk` for dev iteration |

The handler is trivial: `http.FileServer(http.FS(subFS))` where `subFS` is the rooted `docs_html/_site/` subtree, mounted via `http.StripPrefix("/docs", handler)`. No authentication; the deployment posture is LAN-only and matches the existing `:8080` UI. Privacy review (Malory) before merge.

### How content is built

A single `make build-docs-offline` target runs `pnpm --dir docs_html build`. The Eleventy config does **not** copy the source markdown trees into `docs_html/src/` via `addPassthroughCopy`. Instead, the make target creates a small set of **filesystem symlinks** under `docs_html/src/` (details in §4) so Eleventy sees the four content roots as ordinary input directories. The symlinks are created and torn down by the make target so they never get committed.

### How content is embedded

`go:embed all:docs_html/_site` in `internal/docsite/embed.go`. The `all:` prefix is required because the build output may contain files starting with `_` or `.` (Eleventy emits `_site/css/style.css` etc.). This mirrors how the Svelte frontend is embedded today; `web/stub-index.html` and `scripts/ensure-web-stub.sh` show the established pattern for "if the build hasn't been run, fall back to a stub". The same trick applies here so a fresh clone can `go build` without first running Eleventy.

---

## 4. Link resolution by symlink (no rewriter, almost)

The four content roots all use relative markdown links written for GitHub: `[arch](../ARCHITECTURE.md)`, `[L4 maths](../../data/maths/l4-perception.md#dbscan)`, `[tenets](TENETS.md)`. The cheap trick: if Eleventy's input layout under `docs_html/src/` mirrors the repo's filesystem layout exactly, **most relative links resolve themselves** and no rewriting is needed. <!-- link-ignore -->

### 4.1 Symlink layout

The make target `build-docs-offline` (and `dev-docs-offline`) creates this tree before invoking Eleventy:

```
docs_html/src/
├── _includes/                  (real, in-tree — layouts and partials)
├── _layouts/                   (real, in-tree)
├── index.njk                   (real, in-tree — landing page)
├── docs           ─→ ../../docs              (symlink to repo /docs)
├── data           ─→ ../../data              (symlink to repo /data; only structures/ and maths/ are linked from sidebar, but the dir is mounted whole for relative-path correctness)
├── ARCHITECTURE.md ─→ ../../ARCHITECTURE.md  (per-file symlinks for repo root *.md)
├── TENETS.md      ─→ ../../TENETS.md
├── README.md      ─→ ../../README.md
├── CLAUDE.md      ─→ ../../CLAUDE.md
└── … one per root *.md we choose to include
```

The key property: a source file's path under `docs_html/src/` matches its path under the repo root. So inside `docs_html/src/docs/platform/lidar-pipeline.md` (which is really `<repo>/docs/platform/lidar-pipeline.md` via symlink), a link like `../../ARCHITECTURE.md` walks up two directories within `docs_html/src/` and lands on the symlink to the real `ARCHITECTURE.md`. Eleventy reads it, renders it, and emits both files into `_site/` at predictable URLs.

### 4.2 Eleventy's URL convention and how it lines up

Eleventy's default permalink for `src/docs/foo/bar.md` is `/docs/foo/bar/index.html`, served at URL `/docs/foo/bar/`. Crucially, this is the same depth as the source path: a sibling source file `src/docs/foo/baz.md` sits at URL `/docs/foo/baz/`, so a relative link `[baz](baz.md)` written for GitHub keeps working at runtime _if_ we strip the trailing `.md`. That `.md` strip is the one residual transform we cannot avoid; see §4.4. <!-- link-ignore -->

For a cross-tree link like `[arch](../../ARCHITECTURE.md)` from `docs/platform/lidar-pipeline.md`:

- Source resolves to `<src>/ARCHITECTURE.md` (via the symlink at the repo root level).
- Output resolves to `/ARCHITECTURE/`.
- The browser, given `<a href="../../ARCHITECTURE.md">` on a page served at `/docs/platform/lidar-pipeline/`, walks up two segments to `/` and looks for `ARCHITECTURE.md`. It needs `/ARCHITECTURE/` instead.

So even with perfectly mirrored symlinks, the browser-side path arithmetic is correct (up two levels) but the **filename ending** is wrong (`ARCHITECTURE.md` vs `ARCHITECTURE/`). That is the entire residual rewriting surface.

### 4.3 Where symlinks alone are not enough

| Case                                                                                                        | Symlinks fix it?          | Notes                                                                                                                                                                                                                                    |
| ----------------------------------------------------------------------------------------------------------- | ------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `[x](sibling.md)` inside the same directory                                                                 | Path: yes. Extension: no. | Browser resolves the relative path correctly; the `.md` suffix needs stripping to match Eleventy's `/sibling/` URL. <!-- link-ignore -->                                                                                                 |
| `[x](../../ARCHITECTURE.md)` cross-tree                                                                     | Path: yes. Extension: no. | Same story: symlink mirroring makes the path math right; only the `.md` -> `/` swap remains.                                                                                                                                             |
| `[x](#section-only)`                                                                                        | Yes                       | Anchors are within-page; `markdown-it-anchor` already produces stable slug ids matching GitHub's.                                                                                                                                        |
| `[x](file.md#section)`                                                                                      | Path: yes. Extension: no. | The rewriter must strip `.md` while preserving `#section`. <!-- link-ignore -->                                                                                                                                                          |
| `[x](https://github.com/.../blob/main/ARCHITECTURE.md)` (GitHub blob URL form)                              | Sometimes                 | Repo-owned blob URLs can be rewritten to local offline pages if their target path exists inside the embedded content set. Third-party blob URLs remain external and should stay warning-only. See §7 "GitHub blob URL rewrite options".  |
| Links that traverse "up and across roots", e.g. `[x](../../../some-other-tree/file.md)` from inside `data/` | Sometimes                 | If "some-other-tree" is one of the four content roots and is symlinked at the same depth, this works. If it points outside the four roots, it's an unresolved external repo path; fail loudly in the link checker. <!-- link-ignore -->  |
| Anchored links inside files Eleventy renames via permalink rules                                            | N/A                       | We do **not** override permalinks for content files. The default `name.md → /name/` mapping is universal. The only files with custom permalinks are `index.njk` (root) and any future search/sitemap pages. So this case does not arise. |
| Image / asset references (`./images/foo.png`, `../diagrams/foo.svg`)                                        | Yes                       | Eleventy's `addPassthroughCopy` over the symlinked roots copies binary assets along the same relative path. The browser's relative-path math then resolves correctly without any rewrite. <!-- link-ignore -->                           |

### 4.4 Residual rewriter: `.md` extension stripping only

Honest answer: yes, a tiny rewriter is still required, for exactly one job. Strip `.md` (preserving any `#anchor`) on links whose target resolves inside the symlinked tree. That is roughly **15 lines** of `markdown-it` `core.ruler` code (or an Eleventy `addTransform` that runs cheerio over the rendered HTML). Specifically:

1. For each `<a href>`:
2. If absolute (`http://`, `https://`, `mailto:`, `tel:`, fragment-only `#…`), leave alone.
3. Split on `#` into `path` and `frag`.
4. If `path` ends in `.md`, strip the `.md` and ensure trailing `/`.
5. Reassemble with `#frag`.

We do **not** resolve, normalise, or repath the URL. The browser's relative-path machinery already does that work because the symlinked source tree has the same shape as the URL tree. We only fix the file-extension mismatch. This is small enough that I would write it inline in `.eleventy.js` and not pull in a plugin.

### 4.5 macOS / Linux symlink portability

Both targets are POSIX; `ln -s` behaves identically. The make recipes use `ln -snf` (force, no-deref) so re-running the target is idempotent. `git` does not follow symlinks during `add` if the symlink itself is the tracked entity, but here we deliberately keep the symlinks **out of git** (the make target creates them in a directory excluded by `.gitignore`). Eleventy follows symlinks transparently when reading input. `pnpm` and Node have no issue with symlinked input directories. Windows is not a target; ignored.

One concrete hazard: `eleventy --serve` watches the input directory. Some watchers (chokidar, Eleventy's default) do not follow symlinks by default, so edits to files outside `docs_html/src/` (i.e. in the real `docs/` tree) may not trigger live reload. Mitigation: pass `--watch=…` or set `eleventyConfig.addWatchTarget("../docs")` etc. so the watcher tracks the canonical paths. This landed in Milestone 1.

### 4.6 How `make check-docs-offline-links` validates the symlinked tree

The link checker runs **after** Eleventy emits `_site/`. It walks every `*.html` file under `_site/` and:

1. Parses each `<a href>`.
2. Skips absolute URLs; for `github.com/.../blob/...` hits the checker either warns or enforces the chosen policy from §7, depending on whether the URL is repo-owned and rewriteable.
3. For relative URLs, resolves against the page's URL within `_site/` and checks that the target exists on disk (either as a directory containing `index.html` or as an asset file).
4. For URLs with `#anchors`, parses the target HTML and verifies the id exists.
5. Exits non-zero on any unresolved internal link.

This is a ~80-line Go tool or a small Node script (since we're already in Node land for Eleventy, prefer Node for proximity). It does **not** know about the source markdown — it validates only what was rendered, which is precisely the surface we ship.

### 4.7 Cross-tree example, end to end

```
Source:  ARCHITECTURE.md           → symlinked at docs_html/src/ARCHITECTURE.md
Output:  docs_html/_site/ARCHITECTURE/index.html    URL: /ARCHITECTURE/

Source:  docs/platform/lidar-pipeline.md  → at docs_html/src/docs/platform/lidar-pipeline.md
Output:  _site/docs/platform/lidar-pipeline/index.html   URL: /docs/platform/lidar-pipeline/

Inside lidar-pipeline.md:
  [arch](../../ARCHITECTURE.md)
    rewriter: only strips .md → href="../../ARCHITECTURE/"
    browser at /docs/platform/lidar-pipeline/ resolves to /ARCHITECTURE/   ✓

  [L4 maths](../../data/maths/l4-perception.md#dbscan) <!-- link-ignore -->
    rewriter: strips .md, preserves #dbscan → href="../../data/maths/l4-perception/#dbscan"
    browser resolves to /data/maths/l4-perception/#dbscan                 ✓

  [tenets](../../TENETS.md)
    rewriter: → href="../../TENETS/"
    browser resolves to /TENETS/                                           ✓
```

No path normalisation, no repository-root rewriting, no `[FOO](../FOO.md)` → `/FOO/` URL synthesis. Just `.md → /` at the end of the path component. <!-- link-ignore -->

---

## 5. Local dev tooling

The dev loop must not require rebuilding the Go binary. Standardise on a single workflow.

### 5.1 The supported workflow: `eleventy --serve`

```
make dev-docs-offline
```

Runs `eleventy --serve --port=8093` inside `docs_html/` after creating the symlinks. Operator-doc authors edit any `.md` file in `docs/`, `data/`, or repo root; Eleventy live-reloads in the browser. Port `8093` (parallel to the existing `8090` for `public_html/`) keeps the authoring preview separate from the Go server's `/docs/` route.

This is the **only** supported dev mode. Authors should not need to start a Go process to write or preview docs.

### 5.2 What `eleventy --serve` does NOT exercise

`eleventy --serve` is a fast loop, not a faithful one. The following classes of bug will pass cleanly under `dev-docs-offline` and only surface in a full `make build-radar-local` or in CI. A developer working on integration plumbing must run the full build:

1. **`go:embed` directive correctness.** A typo in `//go:embed all:docs_html/_site` (missing `all:`, wrong path, trailing slash mistake) compiles and runs cleanly when the embed FS is empty — Go does not error on an empty match for `embed.FS`. Eleventy serve never touches the embed. **Catch:** Milestone 1 integration tests must assert `len(fs.ReadDir(embedded, "."))) > 0` at startup.
2. **`/docs/` route wiring.** Whether `cmd/radar/radar.go` mounts the handler on the main mux and preserves the API/frontend routes around it. Eleventy serve runs on `:8093` from Node and tells you nothing about the Go side.
3. **Coexistence with `:8080` and `:50051`.** Route conflicts, graceful shutdown ordering on SIGTERM, and partial-failure behaviour when the disk docs source is unavailable. Eleventy is alone in its address space.
4. **`embed.FS` vs `os.DirFS` path semantics.** `embed.FS` is rooted at the import path, uses forward slashes always, and treats the prefix differently from `os.DirFS`. A handler that works against `os.DirFS("docs_html/_site")` may serve `/index.html` from the wrong subtree when switched to `embed.FS`. The `--docs-source=disk` knob exists for runtime A/B but the default ship path is embed.
5. **Go-side content-type defaults and 404 handling.** `http.FileServer` infers content types from extensions and returns its own 404 page. Eleventy serve uses Browsersync defaults. The two will differ on (a) `.svg` content-type, (b) directory-index behaviour for paths without trailing slash, (c) the body of a 404 response. Operator UX wants the Go behaviour validated.
6. **Empty embedded FS (fresh-checkout case).** Before Eleventy has ever run, `docs_html/_site/` is empty or only contains the stub. The Go server should serve the stub gracefully, not panic, and not 500. Eleventy serve cannot reproduce this state by definition — it only runs after the input is present.
7. **Symlink resolution differences.** `eleventy --serve` reads source through the symlinks at runtime. `embed.FS` captures whatever was on disk at `go build` time (post-Eleventy build, symlinks already resolved into rendered HTML). They cannot diverge in production but can diverge during a botched build sequence — e.g. a build target that runs `go build` before `make build-docs-offline`. **Catch:** make targets must enforce ordering and CI must run them in the documented sequence.
8. **Watcher coverage gap (§4.5).** If Eleventy's chokidar watcher does not follow the symlinks into `docs/`, `data/`, etc., live reload silently fails on edits to canonical files. Authors will believe their changes are not picked up. **Catch:** `addWatchTarget` for each canonical root.

The honest summary: **Eleventy serve lies to you about anything that isn't pure HTML rendering.** Use it for content authoring; do `make build-radar-local && ./bin/velocity-report --docs-source=embed` for any change that touches the Go side.

### 5.3 Make targets

| Target                     | Purpose                                                       |
| -------------------------- | ------------------------------------------------------------- |
| `install-docs-offline`     | `pnpm install` inside `docs_html/`                            |
| `build-docs-offline`       | Create symlinks, `pnpm build`, run `check-docs-offline-links` |
| `dev-docs-offline`         | Create symlinks, `eleventy --serve --port=8093`               |
| `dev-docs-offline-kill`    | Kill stale Eleventy serve (mirror of `dev-docs-kill`)         |
| `clean-docs-offline`       | Remove `_site/` and the temporary symlinks                    |
| `check-docs-offline-links` | Internal-link checker scoped to `docs_html/_site/` (§4.6)     |

The existing `build-radar-linux` etc. should call `build-docs-offline` as a prerequisite (mirroring `ensure-web-stub.sh`). For fresh clones with no Eleventy installed, ship a `docs_html/stub-index.html` and `scripts/ensure-docs-stub.sh` so `go build` still succeeds — same pattern as the web frontend.

---

## 6. Build & embed strategy

### Decision: pre-built `_site/` via `go:embed`, not runtime markdown rendering

| Option                                                  | Choice   | Reasoning                                                                                                                                  |
| ------------------------------------------------------- | -------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| **A. `go:embed all:docs_html/_site` of pre-built HTML** | Selected | Single binary; no markdown rendering at request time; identical to how Svelte frontend is shipped; trivial to serve via `http.FileServer`. |
| B. `go:embed` raw `.md` and render in Go                | Rejected | Forces a Go markdown stack (anchors, syntax highlight, link rewrite) duplicating Eleventy's; doubles the surface area to maintain.         |
| C. Read `_site/` from disk in production                | Rejected | Defeats the "single self-contained binary on the Pi" deployment model. Acceptable only as the dev escape hatch via `--docs-source=disk`.   |

### Stub fallback for fresh clones

`docs_html/stub-index.html` ships in-tree; `scripts/ensure-docs-stub.sh` (modelled on `ensure-web-stub.sh`) copies it to `docs_html/_site/index.html` if no build has been run. This keeps `go build ./cmd/radar` green for any developer who hasn't installed Node.

### CI gate

`make lint` should grow a `lint-docs-offline` step that:

1. Runs `make build-docs-offline` and fails on any unresolved internal link or unresolved anchor.
2. Runs the existing `make check-md-links` over the content roots (already in place) to catch problems before Eleventy even runs.

The two checks are complementary: `check-md-links` validates the markdown source as authored; `check-docs-offline-links` validates the rewritten output that ships in the binary.

---

## 7. Risks and remaining decisions

### Failure registry

| Component                                | Failure mode                                            | Recovery                                                                              |
| ---------------------------------------- | ------------------------------------------------------- | ------------------------------------------------------------------------------------- |
| `/docs/` route fails to mount            | Log warning, continue without docs site                 | Existing API/frontend and gRPC stay up; operator can SSH and `cat` the markdown       |
| Embedded `_site/` empty or missing files | Stub `index.html` returned with apology copy + repo URL | Build fails earlier in CI via link checker; runtime fallback only matters during dev  |
| Eleventy build fails in CI               | Block PR                                                | `make build-docs-offline` is a quality gate                                           |
| Link rewriter misses the `.md → /` strip | Internal 404 on `/docs/`                                | `check-docs-offline-links` catches before merge                                       |
| Symlink target missing                   | Eleventy build fails loudly                             | `build-docs-offline` recreates symlinks every run; `clean-docs-offline` resets state  |
| GitHub blob URL link offline             | Browser tries to reach github.com, hangs or fails       | Milestone 1 lint warning; Milestone 2 may optionally rewrite blob URLs to local paths |
| `go:embed` size grows large              | Binary bloats over time                                 | Track binary size in CI; if > 50 MB, revisit "embed vs sidecar tarball" trade         |

### Remaining Milestone 2 decisions

1. **Shared public guide/tool ownership.** Milestone 2 should add selected public-facing onboarding surfaces to the offline site without duplicating source. Recommendation: create a neutral shared source root for canonical guide/tool bodies, images, and tool JavaScript, then keep `public_html/` and `docs_html/` as thin wrapper shells.
2. **Route contract for shared surfaces.** Shared content must target logical in-site URLs such as `/tools/protractor/` and `/guides/setup/`; it must never hard-code the external `/docs/` prefix. The offline site emits those logical URLs inside `_site/`; the Go server mount adds `/docs/...` at runtime.
3. **Search.** Milestone 2 should add offline search now rather than defer it again. Recommendation: Pagefind over `docs_html/_site` only; the search UI lives in the offline shell and ships fully embedded in the binary.
4. **Operator documentation.** `docs/platform/operations/` still needs an operator-facing page that documents `/docs/`, the `--docs-source` dev knob, and the ownership split between shared content and the two site shells.
5. **Auth posture.** `/docs/` is unauthenticated. Malory should sign off that this is acceptable given that the main UI and API are also unauthenticated and the deployment is LAN-only. If the answer is "auth required", we have a separate, larger discussion.
6. **GitHub blob URLs in source.** Some authored markdown links to `https://github.com/.../blob/...`. Milestone 1 emits build warnings per occurrence; Milestone 2 may optionally rewrite them to local URLs if the shared public-content rollout touches those surfaces.

### GitHub blob URL rewrite options

This follow-up needs a narrower framing than "rewrite GitHub URLs or not". There are two materially different cases:

1. **Repo-owned blob URLs**: links to `https://github.com/banshee-data/velocity.report/blob/...` that point back into content or assets we already ship.
2. **Third-party blob URLs**: links to other repositories, or to files we do not embed locally.

Only the first category is a good rewrite target for the offline site.

| Option                               | Scope                                                                                                                               | Effort | Risk | Notes                                                                                                                                                   |
| ------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------- | ------ | ---- | ------------------------------------------------------------------------------------------------------------------------------------------------------- |
| A. Keep warning-only behaviour       | All GitHub blob URLs remain external                                                                                                | XS     | Low  | Simplest. Preserves authored links verbatim, but operator UX is poor for repo-owned links that could have resolved locally.                             |
| B. Rewrite repo-owned blob URLs only | `github.com/banshee-data/velocity.report/blob/<ref>/<path>` when `<path>` maps into embedded content or shared public-content roots | M      | Low  | Best balance. Fixes offline UX where we control the source material, keeps third-party links honest, and reuses the existing local-path resolver logic. |
| C. Rewrite all GitHub blob URLs      | Attempt local rewrite or vendored copy for any GitHub repo                                                                          | L      | High | Overreach. External repos are not part of the embedded docs contract, and vendoring them would bloat the binary and complicate ownership/licensing.     |
| D. Author-managed aliases only       | Keep authored blob URLs unless the doc author adds an explicit offline alias/shortcode                                              | M      | Med  | Explicit, but creates author burden and drift. Easy to miss in future edits, especially in shared public/offline content.                               |

**Recommendation: do B in Milestone 2.**

That means:

1. Rewrite only repo-owned blob URLs that target files we already embed, or files brought into the new shared public/offline content root.
2. Leave third-party blob URLs external and warning-only.
3. Escalate repo-owned blob URLs from warning to failure when they appear in shared guide/tool content or other operator-facing surfaces where the offline experience matters.

### Best path forward

Implement the rewrite in the offline-docs HTML transform, not as a broad Markdown-authoring rule.

1. Detect `https://github.com/banshee-data/velocity.report/blob/<ref>/<path>` during the existing `docs_html` href rewrite pass.
2. Ignore the Git ref for offline purposes and map `<path>` through the same source-path-to-output-URL resolver already used for local markdown links.
3. Rewrite only when `<path>` lands inside the embedded source set: the mirrored `docs/`, `data/`, repo-root markdown, or the Milestone 2 shared public-content root.
4. Preserve `#anchor` fragments during the rewrite.
5. If the path cannot be mapped locally, leave the URL external and emit a warning.

This keeps the implementation small, stays inside the current offline-docs transform architecture, and avoids pretending the offline bundle contains third-party GitHub content when it does not.

---

## 8. Milestone plan

Milestones are release-facing checkpoints rather than PR-sized implementation slices. Milestone 1 is what shipped in PR #480. Milestone 2 is the remaining work needed to make the embedded docs operator-complete and to surface selected public guide/tool content offline without duplicating source.

### Milestone 1 — Core offline docs surface (shipped in PR #480)

- Scaffolded `docs_html/` as a separate Eleventy project with a minimal shell, sidebar, stub, and build pipeline.
- Added symlink-based source mirroring plus explicit watch targets so repo-root markdown, `docs/`, and `data/` render through the offline site without copy-pasting canonical content.
- Implemented link rewriting and rendered-output validation so internal paths and anchors resolve correctly in the shipped tree.
- Embedded the built site in the Go binary, mounted it on `/docs/`, and added the `--docs-source` dev knob.
- Landed the first operator-value polish: docs discoverability from the Svelte UI, version footer, KaTeX, dark mode, and client-side Mermaid.
- Output: operators can browse the embedded documentation on-device over the main HTTP surface.

### Milestone 2 — Shared public guide/tool surfaces and offline search

- Add a **neutral shared source root** for the selected public-facing onboarding surfaces that must also ship offline: the setup guide body, the protractor tool body, the protractor JavaScript, and the shared images/diagrams those pages need.
- Keep `public_html/` and `docs_html/` as **separate shells**. Each site gets thin wrapper pages that own their layout, navigation metadata, permalink, and styling, while the canonical guide/tool source stays shared.
- Move the protractor DOM partial and `tool-protractor.js` into the shared source root so both sites publish the same JavaScript without divergence.
- Move the guide images and other shared diagrams used by both sites into that same shared source root so both sites passthrough-copy from one canonical asset tree.
- Refactor the shared guide/tool content so it no longer uses hard-coded root-relative `/img`, `/js`, `/tool`, `/tools`, `/guides`, or `/docs` URLs. Replace them with a **shared route/asset helper contract** implemented separately in each Eleventy config.
- Extend `docs_html` to keep feeding those helper-generated URLs through its relative-path logic so links remain correct when the whole site is mounted under `/docs/` by the Go server.
- Add targeted rewriting for repo-owned GitHub blob URLs so operator-facing pages resolve locally offline when the target file is already embedded; keep third-party GitHub links external and warning-only.
- Extend `scripts/docs-offline-symlinks.sh` and the offline watch-target list so the shared source root is mounted into `docs_html/src/` and live reload follows edits there.
- Add the equivalent wiring in `public_html/` so the public site consumes the same shared content and assets without taking on the offline site's styling or navigation rules.
- Normalise the public tool route to a canonical logical path, preferably `/tools/protractor/`, and keep a compatibility alias from the current `/tool/protractor/` if the route changes. The shared content should consume a route map rather than guessing at final URLs.
- Add **offline search** to `docs_html` using Pagefind (recommended) or equivalent. `build-docs-offline` should generate the search index, embed it in `docs_html/_site`, and expose a search UI in the offline shell.
- Extend validation to cover the shared guide/tool routes and asset paths, including smoke checks that both the public site and the offline site can resolve the shared protractor JavaScript and guide images.
- Document content ownership and editing rules so future changes know whether they belong in the shared source root, the public wrapper, or the offline wrapper.
- Output: operators can open `/docs/tools/protractor/`, `/docs/guides/setup/`, and search the embedded docs locally, while the public site reuses the same canonical source without duplication.

---

## 9. Implementation status (2026-04-29)

This section records what landed against this plan. Source of truth for Milestone 1: PR #480 (branch `codex/embedded-offline-docs-site-phase-1-4-essentials`). Milestone 2 is planned work; it has not landed yet.

### 9.1 Milestone 1 shipped in PR #480

Milestone 1 comprised the original Phase 1, Phase 2, and Phase 3 work, plus the landed subset of the original Phase 4 polish. The historical phase names are preserved below because they match the merged PR and its implementation notes.

**Original Phase 1 — Skeleton site (complete).**

- New Eleventy project at `docs_html/` with `package.json`, `.eleventy.js`, `src/_layouts/base.njk`, `src/_includes/sidebar.njk`, `src/index.md`, `src/assets/site.css`.
- `docs_html/stub-index.html` ships in tree; `scripts/ensure-docs-stub.sh` copies it to `_site/index.html` if no Eleventy build has run, so `go build` is green on a fresh clone.
- Symlinking script `scripts/docs-offline-symlinks.sh` creates `docs_html/src/{docs,data}` plus per-file symlinks for repo-root `*.md`.
- `addWatchTarget` registered for `../docs`, `../data`, `../README.md`, `../ARCHITECTURE.md`, `../TENETS.md` so live reload works through symlinks. <!-- link-ignore -->
- Make targets: `install-docs-offline`, `build-docs-offline`, `dev-docs-offline`, `dev-docs-offline-kill`, `clean-docs-offline`.

**Original Phase 2 — Link rewriter and link checker (complete, with one design adjustment).**

- The internal link rewriter went **beyond** the plan's "~15-line `.md → /` strip" and now resolves Markdown links to actual repo paths (`outputURLForSourcePath` + `rewriteHrefForInput` in `docs_html/.eleventy.js`). It walks candidate targets (direct, `+.md`, `README.md`, `index.md`) inside the input root and only falls back to the bare `.md → /` strip when no source file matches. This is more code than the plan predicted (~80 lines) but it correctly handles `[x](../foo)` (no extension), `[x](dir/)` (directory → README), and `README.md` rewriting that the simple strip would have missed. <!-- link-ignore -->
- `scripts/check-docs-offline-links.js` validates every rendered `<a href>`, including `#anchor` resolution, and warns on `github.com/.../blob/...` URLs (per §7 Q8). The Makefile target `lint-docs-offline` is non-mutating: it delegates to `check-docs-offline-links` against an existing `_site/`. To rebuild before linting, run `make build-docs-offline`. The script skips gracefully when the site contains only the embed stub, so `make lint` works on a clean checkout without `make install-docs-offline`.

**Original Phase 3 — Go embed and `/docs/` route (complete).**

- New package `internal/docsite/` (`docsite.go`, `docsite_test.go`).
- Embed declared at the repo root in `assets.go`: `//go:embed all:docs_html/_site` → `var DocsSiteFiles embed.FS`. The `docsite` package consumes it via `fs.Sub(radar.DocsSiteFiles, "docs_html/_site")`.
- New flag wired in `cmd/radar/radar.go`:
  - `--docs-source` (`embed` default, `disk` for dev iteration)
- The docs handler is mounted on the main HTTP mux at `/docs/`; no separate docs port is opened.
- All Go CI jobs and Makefile test targets that compile code now run `scripts/ensure-docs-stub.sh` after `ensure-web-stub.sh`, so the embed pattern always matches at least one file (the stub on a clean tree, the real site after a build).

**Original Phase 4 — Polish (partial).**

| Item                                                                      | Status                                                                                                                             |
| ------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| "Docs (offline)" nav link from the Svelte UI                              | Done — `web/src/lib/docsUrl.ts` + nav entry in `web/src/routes/+layout.svelte`.                                                    |
| Version footer (`build.version`, `build.gitShort`, `build.buildTime`)     | Done — `docs_html/src/_data/build.js` reads the env vars set by `make build-docs-offline`.                                         |
| Mermaid rendering                                                         | **Diverged from plan.** Client-side render via the bundled `mermaid.esm.min.mjs` rather than build-time SVG pre-render — see §9.3. |
| KaTeX math rendering (not in original plan; raised by `data/maths/` need) | Done — `markdown-it-texmath` + `katex` server-side; `katex.min.css` and `dist/fonts/` shipped in `_site/assets/`.                  |
| Dark mode (not in original plan)                                          | Done — `[data-theme="dark"]` token set; sticky brand+toggle header at the top of the sidebar; mermaid theme tracks the selection.  |
| Operator documentation in `docs/platform/operations/`                     | Not yet written. See §9.4.                                                                                                         |

### 9.2 Milestone 1 decisions now resolved

| Topic                               | Resolution                                                                                                                                                          |
| ----------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Diagrams (Mermaid pre-render vs JS) | Chose client-side via the bundled mermaid ESM. Trade-off in §9.3.                                                                                                   |
| `devlog/` and `plans/` exposure     | Both are exposed; the deployment posture is LAN-only and operators benefit from "why was this designed this way" context.                                           |
| Discoverability from the Svelte UI  | Done; the URL is same-origin `/docs/`, which works for localhost, LAN, and single-port Tailscale Serve access.                                                      |
| Versioning footer                   | Done.                                                                                                                                                               |
| Image asset paths                   | Confirmed working through symlinks and Eleventy passthrough copy.                                                                                                   |
| GitHub blob URLs                    | Milestone 1 ships warning-only behaviour. Milestone 2 recommendation is targeted same-repo rewrites to local offline paths; third-party GitHub links stay external. |

### 9.3 Milestone 1 divergence: Mermaid

The original plan recommended pre-rendering Mermaid fences to SVG at build time. The implementation ships **client-side** rendering instead.

**What was built.** A markdown-it fence rule turns ` ```mermaid ` blocks into `<pre class="mermaid">` elements. `base.njk` imports `mermaid.esm.min.mjs` from `/assets/mermaid.esm.min.mjs` and calls `mermaid.run({ querySelector: "pre.mermaid" })` after page load (and re-runs on theme toggle so colours track the dark/light selection). The full mermaid ESM bundle is split into ~80 chunks under `node_modules/mermaid/dist/chunks/mermaid.esm.min/`; an `eleventy.after` hook copies each `.mjs` chunk to `_site/assets/chunks/mermaid.esm.min/` (skipping the ~11 MB of `.map` sourcemaps).

**Why the divergence.**

- The plan's pre-render path requires headless Chrome or `mermaid-cli` at build time, which adds a substantial transitive dependency to `docs_html/` and to CI. Client-side rendering re-uses the bundle that already exists in the npm dep graph.
- Authoring loop: a content author writing a new Mermaid fence sees it render immediately under `eleventy --serve` with no extra build step.
- Theme tracking: the dark-mode toggle re-renders mermaid with `theme: "dark"` (vs `"neutral"` for light) without a rebuild. Pre-rendered SVG would have to ship two SVG variants per diagram or be styled in CSS only.

**What it costs.**

- ~3.5 MB of mermaid `.mjs` chunks shipped inside the binary's embedded FS.
- The page does ~9 chunk fetches plus a few diagram-type-specific chunks on first render. Network is same-origin and local to the Pi's web UI, so latency is negligible, but it is non-zero JS work.
- A non-JS browser sees the raw mermaid source text inside `<pre class="mermaid">`. Acceptable for the current operator browser baseline; not acceptable if an `elinks` user appears.

**Reversible if needed.** If the chunk size or the JS-required posture becomes a problem, the substitution is local: replace the markdown-it fence rule and the `eleventy.after` hook with a build-time call into `@mermaid-js/mermaid-cli`. No content changes required.

### 9.4 Milestone 2 planned scope

| Workstream                            | Planned scope                                                                                                                                                |
| ------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Shared canonical source root          | One repo-owned source for the setup guide body, protractor body, shared guide images, and `tool-protractor.js`, consumed by both Eleventy projects.          |
| Public/offline wrapper pages          | Thin wrapper pages in `public_html/` and `docs_html/` with site-local layouts, nav metadata, and styling; no duplicated canonical guide/tool source.         |
| Route and asset helper contract       | Common helper names in both Eleventy configs so shared content never hard-codes `/docs`, `/img`, `/js`, or site-specific tool/guide paths.                   |
| Offline/public asset publishing       | Both sites passthrough-copy the same shared images and JavaScript from one source tree while preserving their own asset namespaces and styling rules.        |
| Route normalisation and compatibility | Settle the canonical tool route (`/tools/protractor/` recommended) and keep compatibility aliases if existing public URLs must remain stable.                |
| Repo-owned GitHub blob rewrites       | Rewrite `github.com/banshee-data/velocity.report/blob/...` links to local offline URLs when the target is embedded; leave third-party GitHub links external. |
| Offline search                        | Add Pagefind (recommended) or equivalent to `build-docs-offline`; ship a search UI and embedded index inside `docs_html/_site`.                              |
| Operator documentation                | Document `/docs/`, `--docs-source`, search, and the shared-content ownership model in `docs/platform/operations/`.                                           |
| Validation                            | Extend smoke checks and link validation to cover the shared guide/tool pages and their shared assets in both Eleventy outputs.                               |

### 9.5 Open follow-ups outside Milestone 2

These are tracked outside this plan; do not let merging this PR imply they are done.

| Item                                                                                                                                                                                                                | Where it lives           |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------ |
| Decide Node baseline for offline docs build: chevrotain@12 needs Node ≥22 and cheerio@1.2 needs ≥20.18, but README/CONTRIBUTING document Node 18+. Either downgrade deps or raise the baseline. Backlog **v0.5.2**. | Issue [#483]             |
| Auth posture review by Malory. Current: LAN-only, no auth, matches `:8080`.                                                                                                                                         | Section 7, Auth posture. |

[#482]: https://github.com/banshee-data/velocity.report/issues/482
[#483]: https://github.com/banshee-data/velocity.report/issues/483

---

## Architecture decision record (summary)

- **Decision:** Add an embedded offline operator docs site as a separate Eleventy project at `docs_html/`, served from the existing Go HTTP server at `/docs/`, content embedded via `go:embed`. Milestone 2 extends that design by surfacing selected public-facing guide/tool content from one neutral shared source consumed by both Eleventy projects, plus offline search in `docs_html`.
- **Status:** Milestone 1 shipped in PR #480; Milestone 2 planned.
- **Drivers:** Field operators need offline docs; existing `public_html/` has the wrong audience; `docs/` already has the content; operators also need the setup guide, protractor, and search on-device without duplicating canonical source.
- **Consequences:**
  - - Single binary still deploys; no new runtime dependencies on the Pi.
  - - Privacy boundary preserved: internal docs never reach the public site.
  - - Dev loop is fast (Eleventy serve), separate from Go rebuild.
  - - Shared guide/tool bodies, images, and JavaScript live in one canonical repo location; layouts, navigation, and styling stay site-local.
  - - Shared content needs a route/asset helper contract; it cannot hard-code `/docs`, `/img`, or `/js` paths.
  - - Offline search adds an embedded index and UI to the binary; track size growth in CI.
  - − Two Eleventy configs to maintain; mitigated by keeping `docs_html/` minimal.
  - − Symlinks must be created by the make target on every fresh checkout; Milestone 2 extends that to the shared public-content root.
  - − Watcher coverage requires explicit `addWatchTarget` per canonical root; one-line config.
  - − Binary size grows by the size of the rendered HTML tree; track in CI.
- **Alternatives:** merge into `public_html/` (rejected: privacy boundary), render markdown at request time in Go (rejected: duplicates Eleventy stack), rewrite all links via a markdown-it plugin (rejected: more code than the symlink-first approach), do nothing (rejected: fails non-technical operators).
- **Reviewers required before merge:** Malory (unauthenticated `/docs/` posture), Florence (devlog/plans exposure decision), Appius (implementation hand-off).
