# Embedded offline docs site (`:8083`)

Status: Draft (Grace, 2026-04-25, rev 2)
Owner: Architecture (Grace) → handoff to Appius for implementation
Scope mode: HOLD — minimum viable architecture; no new dependencies on cloud, no new languages.

---

## 1. Problem statement & use case

Field operators deploy velocity.report on a Raspberry Pi in locations that may have no internet (rural roads, locked-down council networks, temporary monitoring sites). When something goes wrong — a misbehaving radar, a confusing chart, an unexpected percentile — the operator needs to read the docs from the device itself, not from `velocity.report` over the public web. The Go server already binds to `:8080` (HTTP API + Svelte frontend) and `:50051` (gRPC). We want a third listener on `:8083` that serves the project's internal documentation, schema references, and algorithm maths offline. The on-disk content already exists (`docs/`, `data/structures/`, `data/maths/`, root `*.md`); the work is wrapping it in a navigable, link-safe Eleventy site that ships with the binary and can be browsed on the Pi's local network.

"Offline" is the term of art used throughout this plan. It means the same thing it always does: a deployment with no expectation of internet reachability, where every byte the operator needs must already be on the device.

---

## 2. Recommended approach: separate Eleventy project at `docs_html/`

**Recommendation: keep the offline operator docs as a second, separate Eleventy project.** I am confirming, not pushing back on, the user's intuition.

**Decision:** new project at `docs_html/` (sibling of `public_html/`), distinct config, distinct output, distinct embed.

### Why separate, not merged

Three structural reasons, in priority order:

1. **Audience separation is a privacy boundary, not a stylistic preference.** `public_html/` is the public marketing/docs site that ships to `velocity.report`. `docs/` contains plans, devlogs, design notes, internal architecture decisions, and security threat models. Merging means every commit risks accidentally publishing internal-only material to the open web. A separate project lets the build for `:8083` glob `docs/**` freely, while `public_html/` continues to opt-in only the curated public pages it already lists. A merge inverts the safer default.
2. **The two sites have different lifecycle owners.** `public_html/` is versioned with the marketing release cadence and is hand-curated by Terry; `docs_html/` will be a near-mechanical build over the repo's existing markdown trees, refreshed every time the binary is rebuilt. Coupling them means a typo in an internal devlog can block a marketing deploy, and a Tailwind theme update on the public site forces a Go rebuild.
3. **The split test passes for separation.**
   - Different owned surface (operator on-device vs public web).
   - Different long-lived architecture (embedded into a Go binary vs static site deploy).
   - Different stable reader expectation (engineer/operator vs prospect/customer).

### Cost of duplication, and how we keep it small

The legitimate concern with two Eleventy projects is config and theme drift. We mitigate this by **deliberately keeping `docs_html/` minimal**: no Tailwind, no marketing layouts, no `_data/release.json` plumbing, no syntax-highlight plugin until proven necessary. A single layout, a sidebar built from a generated nav data file, and the same `markdown-it` + `markdown-it-anchor` pair already in use. If the two projects ever genuinely converge on shared tooling, we can extract a small `eleventy-shared/` package later — that is a graduation, not a precondition.

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
   │   │ (existing)         │  │ (existing)                  │    │
   │   └────────────────────┘  └─────────────────────────────┘    │
   │                                                              │
   │   ┌──────────────────────────────────────────────────────┐   │
   │   │ HTTP :8083  (NEW)                                    │   │
   │   │ Static file server over embed.FS                     │   │
   │   │ Root = docs_html/_site/  (built by Eleventy)         │   │
   │   │ Read-only; no auth (LAN-only by deployment posture)  │   │
   │   └──────────────────────────────────────────────────────┘   │
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

### Where the listener lives

A new package `internal/docsite/` owns the embed and the HTTP handler. `cmd/radar/radar.go` already wires `:8080`, `:8081` (lidar monitor), `:50051` (gRPC). Add a flag mirroring the existing pattern:

| Flag             | Default | Purpose                                            |
| ---------------- | ------- | -------------------------------------------------- |
| `--docs-listen`  | `:8083` | Address for the embedded docs site                 |
| `--docs-disable` | `false` | Skip starting the docs listener (useful for tests) |
| `--docs-source`  | `embed` | `embed` (default) or `disk` for dev iteration      |

The handler is trivial: `http.FileServer(http.FS(subFS))` where `subFS` is the rooted `docs_html/_site/` subtree. No authentication; the deployment posture is LAN-only and matches the existing `:8080` UI. Privacy review (Malory) before merge.

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

| Case                                                                                                        | Symlinks fix it?          | Notes                                                                                                                                                                                                                                            |
| ----------------------------------------------------------------------------------------------------------- | ------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `[x](sibling.md)` inside the same directory                                                                 | Path: yes. Extension: no. | Browser resolves the relative path correctly; the `.md` suffix needs stripping to match Eleventy's `/sibling/` URL. <!-- link-ignore -->                                                                                                         |
| `[x](../../ARCHITECTURE.md)` cross-tree                                                                     | Path: yes. Extension: no. | Same story: symlink mirroring makes the path math right; only the `.md` -> `/` swap remains.                                                                                                                                                     |
| `[x](#section-only)`                                                                                        | Yes                       | Anchors are within-page; `markdown-it-anchor` already produces stable slug ids matching GitHub's.                                                                                                                                                |
| `[x](file.md#section)`                                                                                      | Path: yes. Extension: no. | The rewriter must strip `.md` while preserving `#section`. <!-- link-ignore -->                                                                                                                                                                  |
| `[x](https://github.com/.../blob/main/ARCHITECTURE.md)` (GitHub blob URL form)                              | No                        | Absolute URL pointing at github.com. Will work online, will silently fail offline. Two options: (a) leave alone (fail open, reach internet if available), or (b) detect and rewrite to local URL. Recommend (a) + lint warning, defer (b) to v2. |
| Links that traverse "up and across roots", e.g. `[x](../../../some-other-tree/file.md)` from inside `data/` | Sometimes                 | If "some-other-tree" is one of the four content roots and is symlinked at the same depth, this works. If it points outside the four roots, it's an unresolved external repo path; fail loudly in the link checker. <!-- link-ignore -->          |
| Anchored links inside files Eleventy renames via permalink rules                                            | N/A                       | We do **not** override permalinks for content files. The default `name.md → /name/` mapping is universal. The only files with custom permalinks are `index.njk` (root) and any future search/sitemap pages. So this case does not arise.         |
| Image / asset references (`./images/foo.png`, `../diagrams/foo.svg`)                                        | Yes                       | Eleventy's `addPassthroughCopy` over the symlinked roots copies binary assets along the same relative path. The browser's relative-path math then resolves correctly without any rewrite. <!-- link-ignore -->                                   |

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

One concrete hazard: `eleventy --serve` watches the input directory. Some watchers (chokidar, Eleventy's default) do not follow symlinks by default, so edits to files outside `docs_html/src/` (i.e. in the real `docs/` tree) may not trigger live reload. Mitigation: pass `--watch=…` or set `eleventyConfig.addWatchTarget("../docs")` etc. so the watcher tracks the canonical paths. This is a one-line config fix; spike in Phase 1.

### 4.6 How `make check-docs-offline-links` validates the symlinked tree

The link checker runs **after** Eleventy emits `_site/`. It walks every `*.html` file under `_site/` and:

1. Parses each `<a href>`.
2. Skips absolute URLs (logs a warning with file:line for any `github.com/.../blob/...` hit so Terry can decide later).
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

Runs `eleventy --serve --port=8093` inside `docs_html/` after creating the symlinks. Operator-doc authors edit any `.md` file in `docs/`, `data/`, or repo root; Eleventy live-reloads in the browser. Port `8093` (parallel to the existing `8090` for `public_html/`) avoids collision with the embedded `:8083` running inside a real Go server.

This is the **only** supported dev mode. Authors should not need to start a Go process to write or preview docs.

### 5.2 What `eleventy --serve` does NOT exercise

`eleventy --serve` is a fast loop, not a faithful one. The following classes of bug will pass cleanly under `dev-docs-offline` and only surface in a full `make build-radar-local` or in CI. A developer working on integration plumbing must run the full build:

1. **`go:embed` directive correctness.** A typo in `//go:embed all:docs_html/_site` (missing `all:`, wrong path, trailing slash mistake) compiles and runs cleanly when the embed FS is empty — Go does not error on an empty match for `embed.FS`. Eleventy serve never touches the embed. **Catch:** Phase 3 tests must assert `len(fs.ReadDir(embedded, "."))) > 0` at startup.
2. **`:8083` listener wiring.** Whether `cmd/radar/radar.go` actually starts the listener, registers the handler, parses `--docs-listen`, and respects `--docs-disable`. Eleventy serve runs on `:8093` from Node and tells you nothing about the Go side.
3. **Coexistence with `:8080` and `:50051`.** Port conflicts (e.g. someone re-uses `:8083` for a future service), graceful shutdown ordering on SIGTERM, partial-failure behaviour when one listener fails to bind. Eleventy is alone in its address space.
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

## 7. Risks and open questions

### Failure registry

| Component                                    | Failure mode                                            | Recovery                                                                             |
| -------------------------------------------- | ------------------------------------------------------- | ------------------------------------------------------------------------------------ |
| `:8083` listener fails to bind (port in use) | Log warning, continue without docs site                 | Existing `:8080` and gRPC stay up; operator can SSH and `cat` the markdown           |
| Embedded `_site/` empty or missing files     | Stub `index.html` returned with apology copy + repo URL | Build fails earlier in CI via link checker; runtime fallback only matters during dev |
| Eleventy build fails in CI                   | Block PR                                                | `make build-docs-offline` is a quality gate                                          |
| Link rewriter misses the `.md → /` strip     | Internal 404 on `:8083`                                 | `check-docs-offline-links` catches before merge                                      |
| Symlink target missing                       | Eleventy build fails loudly                             | `build-docs-offline` recreates symlinks every run; `clean-docs-offline` resets state |
| GitHub blob URL link offline                 | Browser tries to reach github.com, hangs or fails       | Phase 2 lint warning; Phase >2 optionally rewrite blob URLs to local paths           |
| `go:embed` size grows large                  | Binary bloats over time                                 | Track binary size in CI; if > 50 MB, revisit "embed vs sidecar tarball" trade        |

### Open questions

1. **Search.** Operators may want to grep across docs. Eleventy supports plugin-based search (Pagefind, Lunr). Recommend deferring to Phase 5 and shipping a "use Cmd-F or browse the sidebar" v1. Ask Florence for prioritisation against operator feedback.
2. **Diagrams (Mermaid, SVG).** `docs/` contains Mermaid code fences. `public_html/` does not currently render Mermaid. Decide whether the offline site renders Mermaid client-side (extra JS, but no network needed) or pre-renders to SVG at build time (cleaner, matches `make check-mermaid`). Recommend pre-render to SVG.
3. **`devlog/` and `plans/` exposure.** Even on the operator-only site, do we want plans visible? My recommendation: yes — operators benefit from "why was this designed this way" context, and the site never reaches the public internet by deployment posture. But this is Florence's call.
4. **Discoverability.** Should the Svelte frontend on `:8080` link to `:8083`? A small "docs" link in the footer ("Docs (offline)") would be useful and is a one-line change.
5. **Versioning.** The offline site shows the docs as of the binary's git SHA. Add a footer with `version.Version` / `version.GitSHA` so operators know whether they need to upgrade. Trivial; do it in Phase 1.
6. **Image asset paths.** Some `docs/` images are referenced as `./images/foo.png`. Need to confirm Eleventy passthrough copies binary assets alongside the markdown when sources arrive via symlink; spike this in Phase 1. <!-- link-ignore -->
7. **Auth posture.** `:8083` is unauthenticated. Malory should sign off that this is acceptable given that `:8080` (and the API behind it) is also unauthenticated and the deployment is LAN-only. If the answer is "auth required", we have a separate, larger discussion.
8. **GitHub blob URLs in source.** Some authored markdown links to `https://github.com/.../blob/...`. Phase 2 emits a build warning per occurrence; whether to rewrite them to local URLs in v2 is a separate decision.

---

## 8. Phased implementation outline

Each phase is a PR-sized chunk. Sequence is strict: do not start Phase N+1 before Phase N is on `main`.

### Phase 1 — Skeleton site, no Go integration

- Scaffold `docs_html/` with `package.json`, `.eleventy.js`, minimal layout, sidebar template.
- Add `docs_html/stub-index.html` and `scripts/ensure-docs-stub.sh`.
- Symlinking script: `scripts/docs-offline-symlinks.sh` creates `docs_html/src/{docs,data,*.md}` symlinks (§4.1).
- `addWatchTarget` for each canonical root so live reload works through symlinks (§4.5).
- Make targets: `install-docs-offline`, `build-docs-offline`, `dev-docs-offline`, `clean-docs-offline`.
- Output: running `make dev-docs-offline` produces a browseable site at `localhost:8093` with all four content roots reachable. Internal links may still 404 because of the `.md` extension — fix in Phase 2.

### Phase 2 — `.md → /` rewriter and link checker

- Implement the ~15-line markdown-it / cheerio transform that strips `.md` from internal links while preserving `#anchor` (§4.4).
- Implement `check-docs-offline-links` that walks `_site/*.html`, validates relative hrefs and anchors, warns on `github.com/.../blob/...` URLs (§4.6).
- Wire `lint-docs-offline` into `make lint`.
- Spike: confirm image and SVG assets resolve correctly across symlink boundaries; fix any passthrough gaps.
- Output: every internal link in the four content roots resolves on `:8093`, and CI fails on regressions.

### Phase 3 — Go embed and `:8083` listener

- New package `internal/docsite/` with `embed.go` (`go:embed all:docs_html/_site`) and `handler.go` (`http.FileServer` over the subtree).
- New flags `--docs-listen`, `--docs-disable`, `--docs-source` in `cmd/radar/radar.go`.
- `--docs-source=disk` reads from `docs_html/_site/` via `os.DirFS` for dev iteration; default `embed`.
- Update `build-radar-*` targets to call `build-docs-offline` (or its stub equivalent) as a prerequisite, mirroring the `ensure-web-stub.sh` pattern.
- Add startup assertion: `embed.FS` non-empty (§5.2 item 1).
- Add `:8083` to the systemd service notes / firewall guidance in operator docs.
- Output: `velocity-report` binary serves the docs site at `http://<pi>.local:8083/`.

### Phase 4 — Polish and operator value-adds

- Small "Docs (offline)" link in the Svelte frontend footer (§7 Q4).
- Footer on every docs page showing `version.Version` and `git SHA` (§7 Q5).
- Pre-render Mermaid fences to SVG in `build-docs-offline` (§7 Q2).
- Document the new endpoint in `docs/platform/operations/`.
- Output: the operator on the Pi can find and read the right doc without leaving their browser.

### Phase 5 (optional, defer) — Search

- Add Pagefind or equivalent if and only if Florence confirms operator demand.
- This is the kind of thing where "do nothing" is genuinely a valid option for v1.

---

## Architecture decision record (summary)

- **Decision:** Add an embedded offline operator docs site as a separate Eleventy project at `docs_html/`, served by a new HTTP listener on `:8083` from inside the existing Go server, content embedded via `go:embed`. Cross-tree links resolve via filesystem symlinks that mirror the repo layout under `docs_html/src/`; the only residual rewriter strips `.md` from internal hrefs.
- **Status:** Proposed.
- **Drivers:** Field operators need offline docs; existing `public_html/` has the wrong audience; `docs/` already has the content.
- **Consequences:**
  - - Single binary still deploys; no new runtime dependencies on the Pi.
  - - Privacy boundary preserved: internal docs never reach the public site.
  - - Dev loop is fast (Eleventy serve), separate from Go rebuild.
  - - Link rewriting reduced to a 15-line `.md → /` transform; the rest is filesystem layout.
  - − Two Eleventy configs to maintain; mitigated by keeping `docs_html/` minimal.
  - − Symlinks must be created by the make target on every fresh checkout; mitigated by stub fallback and `clean-docs-offline`.
  - − Watcher coverage requires explicit `addWatchTarget` per canonical root; one-line config.
  - − Binary size grows by the size of the rendered HTML tree; track in CI.
- **Alternatives:** merge into `public_html/` (rejected: privacy boundary), render markdown at request time in Go (rejected: duplicates Eleventy stack), rewrite all links via a markdown-it plugin (rejected: more code than the symlink-first approach), do nothing (rejected: fails non-technical operators).
- **Reviewers required before merge:** Malory (unauthenticated `:8083` posture), Florence (devlog/plans exposure decision), Appius (implementation hand-off).
