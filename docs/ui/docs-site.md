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
│   ├── _layouts/       Nunjucks templates (base, page, doc, redirect)
│   ├── _includes/      Reusable components (header, footer)
│   ├── _data/          Release metadata (release.json)
│   ├── guides/         Setup guide, Tailscale guide, guides index
│   ├── tools/          Practical browser utilities (protractor)
│   ├── notes/          Field Notes: technical articles
│   ├── reference/      Exact technical conventions and profiles
│   ├── community/      Community resources
│   ├── tool/           Redirect stub for the pre-/tools/ protractor URL
│   ├── css/            Tailwind entry point
│   ├── images/         Site images, favicons
│   └── index.njk       Homepage
├── _site/              Build output (generated)
├── .eleventy.js        Eleventy config
├── tailwind.config.js  Tailwind config
└── postcss.config.js   PostCSS config
```

Content flow: `.md` → front matter selects layout → Nunjucks wraps → base layout adds structure → Tailwind styles → HTML.

## Information architecture

The site is organised around what a visitor is trying to do, not around content types. Five destinations, and one rule for deciding between them:

| The page...                           | Goes to                       |
| ------------------------------------- | ----------------------------- |
| helps someone accomplish a task       | `guides/` (Setup) or `tools/` |
| explains something learned or decided | `notes/` (Field Notes)        |
| defines an exact technical contract   | `reference/`                  |
| helps someone participate             | `community/`                  |
| explains the overall project          | the homepage                  |

Primary navigation is deliberately small and stays that way: **Setup, Tools, Field Notes, Community, GitHub**. `reference/` is reached contextually from the pages that cite it, not from the navigation bar. New content types do not earn a navigation entry by existing.

Field Notes are technical articles about building, measuring, and deploying the system: design decisions, experiments, and lessons learned. The category is not a blog, a newsroom, or a research index, and should not be renamed into one. A note explains a decision; the matching reference page, if there is one, states it precisely. Neither duplicates the other.

Do not create empty landing pages, placeholder cards, or "coming soon" entries to make the hierarchy look complete. A clean namespace holding one real page is better than a navigation tree of empty categories. `/tools/` and `/reference/` currently have no index page for exactly this reason: navigation points at the real page instead.

### Content types and front matter

| Type       | Location         | Front matter                                            |
| ---------- | ---------------- | ------------------------------------------------------- |
| Field Note | `src/notes/`     | `title`, `description`, `date`, optional `topics`       |
| Tool       | `src/tools/`     | `title`, `description`                                  |
| Reference  | `src/reference/` | `title`, `description`, `date`, `version` if applicable |
| Guide      | `src/guides/`    | `title`, `description`, `section: guides`, `date`       |

Directory data files (`src/notes/notes.json`, `src/reference/reference.json`) set the shared layout and breadcrumb label, so a new page only declares what is specific to it. There is no tag or category system: the site is too small to benefit from one.

### Moving a URL

GitHub Pages has no server-side rewrites, so a moved page leaves a stub behind using `redirect.njk`. The stub sets `redirectTo` to the new logical URL and the layout emits a meta refresh, a canonical link to the new address, and a visible fallback link. Add the new path to `REQUIRED_FILES` in [scripts/verify-public-html-build.py](../../scripts/verify-public-html-build.py), keeping the old one so the redirect itself stays covered.

## Build

```bash
make build-docs          # Build site → public_html/_site/
make build-docs-public-html # Build the image-mounted site → /public_html/
make install-docs        # Install pnpm dependencies
```

Dev server (hot reload on `:8090`):

```bash
cd public_html && pnpm run dev
```

## Deployment

- **GitHub Pages:** auto-deploys from the `gh-pages` branch
- **On-device:** `make build-image` rebuilds the site with `/public_html/`-prefixed links, installs it to `/opt/velocity-report/public_html/`, and the Go service serves it at `/public_html/`. The app sidebar links it as **Homepage (offline)**; **Git repo docs** remains the separate `/docs/` site.

## OS image list

The site hosts the Raspberry Pi Imager custom OS list at `os-list-velocity.json`. The JSON is defined in [image/os-list-velocity.json](../../image/os-list-velocity.json) and copied to the built site during the Eleventy build. Raspberry Pi Imager fetches this JSON from the public URL to offer velocity.report as a custom OS option.

## Content guidelines

- British English spelling and punctuation (see [.github/STYLE.md](../../.github/STYLE.md))
- Setup guide (`src/guides/setup.md`) must stay in sync with [DEBUGGING.md](../../DEBUGGING.md) and [COMMANDS.md](../../COMMANDS.md)
- Release metadata in `src/_data/release.json` is updated manually at release time
