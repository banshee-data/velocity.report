# velocity.report/web

Svelte-based frontend for real-time traffic data visualisation.

- **Location**: `web/`
- **Framework**: Svelte 5
- **Build**: Vite
- **Package Manager**: pnpm

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

The dev server proxies every `/api` route, radar and LiDAR alike, to the Go server on
`http://localhost:8080`. To reach a server on another port, set `VELOCITY_API_URL`, for example
`VELOCITY_API_URL=http://localhost:18080 pnpm run dev`.

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

LiDAR evaluation windows are [clips](../docs/lidar/operations/clip-management-implementation.md).
The API uses `/api/lidar/clips` and `clip_id`; ordered capture rows supply each clip’s paths.
