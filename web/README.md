# velocity.report/web

Welcome to the `velocity.report` frontend!

This directory contains everything you need to build, develop, and maintain our web application.

## Tech Stack

We're using a modern, fast, and flexible stack:
- [Svelte v5](https://svelte.dev/) for fast, reactive UI
  - [@sveltejs/adapter-static](https://www.npmjs.com/package/@sveltejs/adapter-static) for static site generation
- [Tailwind CSS v4](https://tailwindcss.com/) for utility-first styling
- [svelte-ux v2](https://svelte-ux.techniq.dev/) for UI components
- [LayerChart v2](https://www.layerchart.com/) for data visualization

## Getting Started

We use [pnpm](https://pnpm.io/installation) for package management. Make sure it's installed before you begin.

### Start the Dev Server

Spin up the app locally with:

```sh
pnpm run dev
```

Your app will be running at [http://localhost:5173](http://localhost:5173) (or the next available port).

### Build for Production

Create an optimized production build:

```sh
pnpm run build
```

Preview your production build locally:

```sh
pnpm run preview
```

## Maintenance

Keep dependencies up to date with:

```sh
pnpm run up-deps
```

Run linting and formatting to keep your codebase clean:

```sh
pnpm run format
```

---

Feel free to open issues or PRs if you spot anything that can be improved. Happy coding!
