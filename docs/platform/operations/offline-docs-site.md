# Offline docs site

Active plan: [embedded-offline-docs-site.md](../../plans/embedded-offline-docs-site.md)
Proposed: [deploy-offline-docs-asset-plan.md](../../plans/deploy-offline-docs-asset-plan.md) (ship the site as a release asset beside the binary)

This page describes the two documentation sites carried by the image: the
repository reference at `/docs/` and the curated public homepage at
`/public_html/`. It records what each site contains, how it is built, and how
the main Go service serves it without opening another port.

## Purpose

The repository docs let an operator read technical guidance from the device
itself when the deployment has no internet access or no business trusting the
internet with the job. `/docs/` contains the repository's `docs/` and `data/`
trees, rendered into the binary-embedded Eleventy site. `/public_html/` serves
the image-staged public site, including its curated guides, from the same HTTP
surface. Localhost, LAN access, and single-port reverse proxies reach both.

## Ownership split

| Surface          | Owns                                                                       |
| ---------------- | -------------------------------------------------------------------------- |
| `docs_html/`     | Repository-docs shell, sidebar, build pipeline, search, and link checks    |
| `public_html/`   | Curated public homepage and guides, staged on the image at `/public_html/` |
| `docs/`, `data/` | Technical and research content rendered into the `/docs/` repository site  |

The two Eleventy projects stay separate on purpose. Public docs and operator docs have different audiences, different publication rules, and very different ways to accidentally cause trouble.

## Build and serve model

- `make build-docs-offline` builds the offline Eleventy site into `docs_html/_site`
- `make build-docs-public-html` builds `public_html/_site` with paths rooted at `/public_html/`
- `make dev-docs-offline` runs the offline Eleventy preview for authoring
- `python3 scripts/check-relative-links.py` validates Markdown link integrity across the source tree
- `scripts/check-docs-offline-links.js` and
  `scripts/verify-embedded-docs-server.py` reject every same-origin `/docs/`
  anchor that does not resolve; GitHub and the deliberate `/public_html/`
  surface remain external routes.
- The sidebar tree is rendered once, to `assets/docs-tree.html`, with links
  relative to that file so they resolve under any mount. Each page carries an
  empty `<nav data-doc-tree-src>` placeholder, and `assets/docs-tree.js` loads
  the tree into it, marks the current page and opens its folders. Without
  JavaScript the breadcrumbs and folder index pages still navigate. The link
  checks follow the placeholder to the tree file and check its links, and
  `scripts/verify-docs-offline-build.sh` fails if a page carries the tree
  inline again: rendered into every page it made the site about 180 MB.
- The Go server embeds `docs_html/_site` and serves it at `/docs/`
- Pull requests that change docs content run Offline docs CI
  (`docs-offline-ci.yml`), not the static release build. After
  `make build-docs-offline` it runs `make test-docs-offline-routes`, which
  compiles `internal/docsite` with the built site embedded and fetches every
  generated page and its `/docs/` links through the server's handler under
  the release check's rules, and `make check-docs-embed-context`, which runs
  the static build's manifest check (`embed-check` stage of
  `image/Dockerfile.static-build`) to show every generated file survives its
  Docker context. Static build CI (`static-build-ci.yml`) runs
  `scripts/verify-embedded-docs-server.py` against the release binary on
  PRs that change binary inputs and on every push to `main`.
- Repository source links resolve to the immutable GitHub revision recorded in
  the embedded-docs build metadata. The image does not carry a second source
  tree: binary, UI, documentation, and runtime dependencies remain one
  versioned release unit.
- Image assembly stages `public_html/_site` at `/opt/velocity-report/public_html` and serves it at `/public_html/`
- `--docs-source=embed|disk` switches between embedded and on-disk serving for development

## Scope boundary

This doc is the operational reference for the embedded docs surface itself. Detailed implementation sequencing, Milestone 1 status, and Milestone 2 remaining work stay in the active plan while the work is still landing.
