# Offline docs site

Active plan: [embedded-offline-docs-site.md](../../plans/embedded-offline-docs-site.md)

This page is the stable home for the embedded `/docs/` site: what it is for, how it is built, how it is served, and which parts belong to the offline operator shell rather than the public Eleventy site.

## Purpose

The offline docs site lets an operator read technical guidance from the device itself when the deployment has no internet access or no business trusting the internet with the job. It is served from the main Go HTTP surface at `/docs/`, so localhost, LAN access, and single-port reverse proxies all reach the same embedded documentation.

## Ownership split

| Surface        | Owns                                                                 |
| -------------- | -------------------------------------------------------------------- |
| `docs_html/`   | Offline Eleventy shell, sidebar, build pipeline, search, link checks |
| `public_html/` | Public docs shell, marketing-facing layout, public guide wrappers    |
| `docs/`        | Internal technical content consumed by the offline site              |

The two Eleventy projects stay separate on purpose. Public docs and operator docs have different audiences, different publication rules, and very different ways to accidentally cause trouble.

## Build and serve model

- `make build-docs-offline` builds the offline Eleventy site into `docs_html/_site`
- `make dev-docs-offline` runs the offline Eleventy preview for authoring
- `python3 scripts/check-relative-links.py` validates Markdown link integrity across the source tree
- The Go server embeds the built output and serves it at `/docs/`
- `--docs-source=embed|disk` switches between embedded and on-disk serving for development

## Scope boundary

This doc is the operational reference for the embedded docs surface itself. Detailed implementation sequencing, Milestone 1 status, and Milestone 2 remaining work stay in the active plan while the work is still landing.
