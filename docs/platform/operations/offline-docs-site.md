# Offline docs site

Active plan: [embedded-offline-docs-site.md](../../plans/embedded-offline-docs-site.md)

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
- The Go server embeds `docs_html/_site` and serves it at `/docs/`
- Image assembly stages `public_html/_site` at `/opt/velocity-report/public_html` and serves it at `/public_html/`
- `--docs-source=embed|disk` switches between embedded and on-disk serving for development

## Scope boundary

This doc is the operational reference for the embedded docs surface itself. Detailed implementation sequencing, Milestone 1 status, and Milestone 2 remaining work stay in the active plan while the work is still landing.
