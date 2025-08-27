# velocity.report/docs

A static documentation site built with [Eleventy](https://www.11ty.dev/) and [Tailwind CSS](https://tailwindcss.com/).

## Prerequisites

- [Node.js](https://nodejs.org/) (v16 or higher)
- [pnpm](https://pnpm.io/) package manager

## Development

```bash
# Install dependencies
pnpm install

# Start development server with hot reload
pnpm run dev
```

This runs Eleventy in watch mode with Tailwind CSS compilation. The site will be available at `http://localhost:8090`.

## Build

```bash
# Build for production
pnpm run build
```

Outputs optimized files to the `_site/` directory.

## Deployment

The site automatically deploys to GitHub Pages when changes are pushed to the `gh-pages` branch.

## Project Structure

```
docs/
├── _site/          # Build output (generated)
├── src/            # Source files
├── package.json    # Dependencies and scripts
└── README.md       # This file
```
