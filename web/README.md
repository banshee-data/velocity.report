# velocity.report/web

Svelte-based frontend for real-time traffic data visualization.

**Location**: `web/`
**Framework**: Svelte 5
**Build**: Vite
**Package Manager**: pnpm

## Tech Stack

- **[Svelte 5](https://svelte.dev/)** - Fast, reactive UI framework
- **[@sveltejs/adapter-static](https://www.npmjs.com/package/@sveltejs/adapter-static)** - Static site generation
- **[Tailwind CSS 4](https://tailwindcss.com/)** - Utility-first styling
- **[svelte-ux 2](https://svelte-ux.techniq.dev/)** - UI components
- **[LayerChart 2](https://www.layerchart.com/)** - Data visualization

## Getting Started

Install [pnpm](https://pnpm.io/installation) if not already installed.

### Development

Start the dev server:

```sh
pnpm run dev
```

App runs at [http://localhost:5173](http://localhost:5173) (or next available port).

### Production Build

Create an optimized build:

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
