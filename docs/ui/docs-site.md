# Documentation site (public_html)

The public-facing documentation site at [velocity.report](https://velocity.report), built with Eleventy and Tailwind CSS.

**Source:** `public_html/`
**Build output:** `public_html/_site/`
**Framework:** Eleventy (11ty) + Tailwind CSS
**Package manager:** pnpm

## Architecture

Markdown content with Nunjucks layouts and Tailwind styling:

```
public_html/
├── src/
│   ├── _layouts/       Nunjucks templates (base, page, doc)
│   ├── _includes/      Reusable components (header, footer)
│   ├── _data/          Release metadata (release.json)
│   ├── guides/         Setup guide and docs index
│   ├── community/      Community resources
│   ├── css/            Tailwind entry point
│   ├── images/         Site images, favicons
│   └── index.njk       Homepage
├── _site/              Build output (generated)
├── .eleventy.js        Eleventy config
├── tailwind.config.js  Tailwind config
└── postcss.config.js   PostCSS config
```

Content flow: `.md` → front matter selects layout → Nunjucks wraps → base layout adds structure → Tailwind styles → HTML.

## Build

```bash
make build-docs          # Build site → public_html/_site/
make install-docs        # Install pnpm dependencies
```

Dev server (hot reload on `:8090`):

```bash
cd public_html && pnpm run dev
```

## Deployment

- **GitHub Pages:** auto-deploys from the `gh-pages` branch
- **On-device:** the built site is installed to `/opt/velocity-report/public_html/` during `make build-image`, served locally for offline reference

## OS image list

The site hosts the Raspberry Pi Imager custom OS list at `os-list-velocity.json`. The JSON is defined in [image/os-list-velocity.json](../../image/os-list-velocity.json) and copied to the built site during the Eleventy build. Raspberry Pi Imager fetches this JSON from the public URL to offer velocity.report as a custom OS option.

## Content guidelines

- British English spelling and punctuation (see [.github/STYLE.md](../../.github/STYLE.md))
- Setup guide (`src/guides/setup.md`) must stay in sync with [TROUBLESHOOTING.md](../../TROUBLESHOOTING.md) and [COMMANDS.md](../../COMMANDS.md)
- Release metadata in `src/_data/release.json` is updated manually at release time
