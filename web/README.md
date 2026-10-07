# velocity.report/web

Svelte-based frontend for real-time traffic data visualisation.

- **Location**: `web/`
- **Framework**: Svelte 5
- **Build**: Vite
- **Package Manager**: pnpm

## Caller permissions

The layout loads `/api/access` before mounting protected pages and refreshes it every 30 seconds.
Lookup failures discard previous permissions. Viewing and downloading existing PDFs remain available
to hardened LAN viewers; report creation, settings, source downloads and site changes require
explicit permissions. This response describes the server's policy and does not establish a native
user session. The backend checks every request independently, including after grants change. See the
[access design](../docs/plans/platform-access-control-hardening-plan.md).

## Tech stack

- **[Svelte 5](https://svelte.dev/)** - Fast, reactive UI framework
- **[@sveltejs/adapter-static](https://www.npmjs.com/package/@sveltejs/adapter-static)** - Static
  site generation
- **[Tailwind CSS 4](https://tailwindcss.com/)** - Utility-first styling
- **[svelte-ux 2](https://svelte-ux.techniq.dev/)** - UI components
- **[LayerChart 2](https://www.layerchart.com/)** - Data visualisation

## Getting started

Install [pnpm](https://pnpm.io/installation) if not already installed.

### Development

Start the dev server:

```sh
pnpm run dev
```

App runs at [http://localhost:5173](http://localhost:5173) (or next available port).

### Production build

Create an optimised build:

```sh
pnpm run build
```

Preview the production build:

```sh
pnpm run preview
```

## Maintenance

Update dependencies:

```sh
pnpm run up-deps
```

Format and lint:

```sh
pnpm run format
```
