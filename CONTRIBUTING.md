# Contributing to velocity.report

Thank you for your interest in contributing to velocity.report! This document outlines our conventions, workflow, and how to get involved.

## Community & Discussion

[![Discord](https://img.shields.io/discord/1387513267496419359?logo=discord&label=chat%20on%20discord)](https://discord.gg/XXh6jXVFkt)

- **Discord** — Join our [Discord server](https://discord.gg/XXh6jXVFkt) for real-time discussion, questions, and community support
- **GitHub Issues** — Report bugs, request features, and track the project roadmap
- **GitHub Discussions** — For longer-form conversations and ideas

## Privacy Principles

This project is built with privacy as a core value:

- ✅ No cameras or video recording
- ✅ No license plate recognition
- ✅ No personally identifiable information
- ✅ Local-only data storage

All contributions must maintain these principles.

## Contributor Personas

velocity.report spans sensor hardware, real-time data pipelines, web visualisation, machine learning, and deployment packaging. Many different skill sets can make a meaningful impact — you don't need to be an expert in all of them. Below are the contributor profiles that align with the project's current and planned work.
If you're new, pick the role that is closest to your background and start with its 3-5 core docs before diving into issues or plans.

### ML & Data Scientist

As the project matures beyond rule-based classification, there is growing scope for machine learning: training vehicle classifiers on labelled track features, automated hyperparameter search for tracking algorithms, and evaluating model performance against ground-truth data. ML contributors work primarily in Python (scikit-learn, pandas) with Go integration for inference. Experience with feature engineering, model evaluation, and small-dataset techniques suits the project's privacy-first, local-only constraints.

**Read next:**

- [docs/plans/lidar-ml-classifier-training-plan.md](docs/plans/lidar-ml-classifier-training-plan.md) — the planned training pipeline and deployment model
- [docs/plans/lidar-track-labeling-auto-aware-tuning-plan.md](docs/plans/lidar-track-labeling-auto-aware-tuning-plan.md) — how labelled runs, ground truth, and tuning fit together
- [docs/maths/classification-maths.md](docs/maths/classification-maths.md) — the current rule-based feature set and decision thresholds
- [docs/plans/label-vocabulary-consolidation-plan.md](docs/plans/label-vocabulary-consolidation-plan.md) — the canonical taxonomy and cross-surface label model

### Designer (UX & Data Visualisation)

Good data visualisation turns speed measurements into stories that drive policy change. Designers contribute to information hierarchy, chart design, colour palettes, layout patterns, and accessibility. The project follows a documented [design contract](docs/ui/DESIGN.md) covering palette standards, component conventions, and UI states. Contributions range from Figma mockups and design system refinement to hands-on Svelte/CSS implementation. Experience with data-dense dashboards and accessibility standards (WCAG) is valuable.

**Read next:**

- [docs/ui/DESIGN.md](docs/ui/DESIGN.md) — the canonical design language across web, macOS, and report outputs
- [docs/VISION.md](docs/VISION.md) — the product goals, target users, and reporting outcomes the UI needs to support
- [docs/ui/VelocityVisualiser.app/01-problem-and-user-workflows.md](docs/ui/VelocityVisualiser.app/01-problem-and-user-workflows.md) — concrete workflows and UX targets for the LiDAR visualiser
- [tools/pdf-generator/README.md](tools/pdf-generator/README.md) — the report surface and configuration model for generated outputs
- [docs/ui/velocity-visualiser-implementation.md](docs/ui/velocity-visualiser-implementation.md) — current implementation milestones and what the visualiser already supports

### Technical Writer

Clear documentation lowers the barrier for new contributors and new deployments alike. Technical writers work on setup guides, architecture documentation, API references, design documents, and the public documentation site (Eleventy). The project maintains docs in Markdown with metadata validation gates. Good technical writing — especially the ability to explain sensor concepts to a non-technical neighbourhood advocate audience — has outsized impact.

**Read next:**

- [README.md](README.md) — project overview, component map, and contributor setup
- [docs/README.md](docs/README.md) — documentation structure, ownership, and naming rules
- [docs/plans/platform-documentation-standardization-plan.md](docs/plans/platform-documentation-standardization-plan.md) — the current documentation quality contract and validation gates
- [public_html/README.md](public_html/README.md) — how the public docs site is built and organised
- [public_html/src/guides/setup.md](public_html/src/guides/setup.md) — a representative public-facing guide to match tone, structure, and audience

### Frontend Engineer (mac:Swift / js:Svelte)

The web frontend provides real-time dashboards, chart visualisation, and configuration interfaces. Frontend engineers work on Svelte 5 components, chart rendering with LayerChart and D3, Tailwind CSS styling, and responsive design. Current priorities include migrating legacy Go-embedded dashboards to Svelte, building new configuration UIs, and enforcing the project's design system. Experience with accessibility testing (axe-core, Playwright) is a plus.

**Read next:**

- [web/README.md](web/README.md) — local frontend setup, build, and maintenance commands
- [docs/ui/DESIGN.md](docs/ui/DESIGN.md) — the design contract for web, macOS, and report charts
- [docs/ui/design-review-and-improvement.md](docs/ui/design-review-and-improvement.md) — current frontend design gaps and concrete follow-up work
- [docs/plans/web-frontend-consolidation-plan.md](docs/plans/web-frontend-consolidation-plan.md) — the roadmap for retiring legacy Go-embedded dashboards

### Perception & Algorithm Engineer

The LiDAR and radar processing pipeline turns raw sensor data into tracked objects with speed, heading, and classification. Perception engineers work on point-cloud clustering, Kalman-filtered tracking, object classification, sensor fusion, and spatial geometry. This includes algorithm design in Go, mathematical modelling (coordinate transforms, ground-plane projection), and optional Swift/Metal work on the macOS LiDAR visualiser. A background in robotics, computer vision, or signal processing is ideal.

**Read next:**

- [docs/lidar/README.md](docs/lidar/README.md) — entry point to the LiDAR subsystem docs
- [docs/lidar/architecture/lidar-pipeline-reference.md](docs/lidar/architecture/lidar-pipeline-reference.md) — the end-to-end LiDAR pipeline and component inventory
- [docs/maths/README.md](docs/maths/README.md) — how the math-heavy layers fit together
- [docs/maths/clustering-maths.md](docs/maths/clustering-maths.md) — clustering assumptions, geometry extraction, and complexity
- [docs/maths/tracking-maths.md](docs/maths/tracking-maths.md) — Kalman filtering, gating, assignment, and lifecycle dynamics

### Systems Engineer (Go)

The Go server is the backbone of velocity.report: it ingests sensor data, manages the SQLite database, serves the HTTP/gRPC API, and orchestrates the processing pipeline. Systems engineers work on data ingestion, API design, database schema and migrations, configuration management, and binary consolidation. Familiarity with concurrency patterns, serial/UDP protocols, and embedded database constraints (single-writer SQLite on a Raspberry Pi) is especially valuable.

**Read next:**

- [ARCHITECTURE.md](ARCHITECTURE.md) — system boundaries, data flow, and deployment shape
- [cmd/radar/README.md](cmd/radar/README.md) — the main binary, runtime flags, and service model
- [docs/radar/cli-comprehensive-guide.md](docs/radar/cli-comprehensive-guide.md) — current CLI surface and planned consolidation
- [internal/db/migrations/README.md](internal/db/migrations/README.md) — schema workflow, migration commands, and production safety
- [config/README.md](config/README.md) — configuration contract and tuning parameter layout

### Platform & DevOps Engineer

velocity.report deploys to Raspberry Pi 4 hardware with no cloud infrastructure. Platform engineers work on CI/CD pipelines (GitHub Actions), cross-compilation for ARM64, Raspberry Pi image builds, one-line installers, release packaging, and systemd service management. The goal is zero-friction deployment: a community member downloads an SD card image, inserts it, and has a working traffic monitor. Shell scripting, Makefile fluency, and Linux systems experience are key.

**Read next:**

- [cmd/deploy/README.md](cmd/deploy/README.md) — deployment workflows, upgrade flow, rollback, and health checks
- [docs/plans/deploy-distribution-packaging-plan.md](docs/plans/deploy-distribution-packaging-plan.md) — release packaging strategy and install model
- [docs/plans/deploy-rpi-imager-fork-plan.md](docs/plans/deploy-rpi-imager-fork-plan.md) — Raspberry Pi image distribution and first-run experience
- [docs/radar/architecture/networking.md](docs/radar/architecture/networking.md) — listener segmentation, trust model, and network hardening
- [config/CONFIG-RESTRUCTURE.md](config/CONFIG-RESTRUCTURE.md) — upcoming config migration that affects packaging and runtime defaults

## Themes of Work

The following broad themes describe the kinds of work available across the project. Specific tasks live in the [backlog](docs/BACKLOG.md); these themes help contributors find the area that best matches their skills.

### Sensor Integration & Data Pipeline

Ingesting, validating, and storing data from radar and LiDAR sensors. This includes serial and UDP protocol handling, data parsing, schema design, and ensuring data integrity on resource-constrained hardware.

### Tracking, Perception & Sensor Fusion

Turning raw sensor feeds into meaningful objects: clustering point clouds, maintaining tracked identities across frames, classifying vehicles, and fusing radar speed measurements with LiDAR spatial tracks into unified transit records.

### Web Frontend & Visualisation

Building and maintaining the Svelte web application: real-time dashboards, interactive charts, configuration interfaces, and design system enforcement. Includes migrating legacy Go-embedded dashboards to Svelte, improving responsiveness, and ensuring accessibility.

### Report Generation & Data Export

Producing professional PDF speed reports suitable for local authority submissions, and providing data export (CSV, GeoJSON) for external analysis. Spans LaTeX templating, chart rendering, and query-scoped report generation.

### Deployment, Packaging & Platform

Making velocity.report easy to install and run: Raspberry Pi image pipelines, cross-compiled binaries, one-line installers, systemd integration, CI/CD automation, and release management.

### Quality, Testing & Accessibility

Raising and maintaining test coverage across Go, Python, and web components. Includes unit testing, E2E testing with Playwright, visual regression testing, accessibility auditing, and code quality tooling.

### Documentation & Community

Writing and maintaining setup guides, architecture docs, design documents, and the public documentation site. Ensuring that documentation stays accurate as the codebase evolves, and helping new contributors get started.

## Roadmap

Project roadmap and planned features are tracked in [GitHub Issues](https://github.com/banshee-data/velocity.report/issues). Look for issues labelled:

- `enhancement` — New features and improvements
- `bug` — Known issues to fix
- `good first issue` — Great starting points for new contributors
- `help wanted` — Issues where we'd especially appreciate contributions

## Getting Started

### Prerequisites

- **Go 1.25+** — For server development
- **Python 3.11+** — For PDF generator
- **Node.js 18+** with pnpm — For web frontend
- **SQLite3** — Database

### Initial Setup

```bash
# Clone the repository
git clone git@github.com:banshee-data/velocity.report.git
cd velocity.report

# Go server
make build-radar-local

# Python environment
make install-python

# Web frontend
make install-web
```

See the [README](README.md) for detailed setup instructions.

## Code Style & Conventions

### Code Formatting

We use automated formatters for consistency:

| Language   | Formatter           |
| ---------- | ------------------- |
| Go         | `gofmt`             |
| Python     | `black` + `ruff`    |
| JavaScript | `prettier` + ESLint |

**Before committing:**

```bash
make format    # Auto-format all code
make lint      # Verify formatting
make test      # Run all tests
```

All three must pass before submitting a PR.

### Pre-commit Hooks (Recommended)

For regular contributors, install pre-commit hooks to auto-format on every commit:

```bash
pip install pre-commit
pre-commit install
```

## Git Workflow

### Branch Naming

Use descriptive branch names with a category prefix:

- `feature/` — New features (e.g., `feature/lidar-tracking`)
- `fix/` — Bug fixes (e.g., `fix/api-timeout`)
- `docs/` — Documentation updates (e.g., `docs/update-setup-guide`)
- `refactor/` — Code refactoring (e.g., `refactor/db-layer`)

### Commit Message Format

Prefix commit messages with the primary language or purpose:

```
[prefix] Description of change
```

**Allowed prefixes:**

| Prefix   | Use Case                                                                               |
| -------- | -------------------------------------------------------------------------------------- |
| `[go]`   | Go code, server, APIs                                                                  |
| `[py]`   | Python code (PDF generator, tools)                                                     |
| `[js]`   | JavaScript/TypeScript (SvelteKit frontend, Vite)                                       |
| `[mac]`  | macOS files (Swift, xcode)                                                             |
| `[docs]` | Documentation (Markdown guides, READMEs)                                               |
| `[sh]`   | Shell scripts (Makefile, bash utilities)                                               |
| `[sql]`  | Database schema or SQL migrations                                                      |
| `[fs]`   | Filesystem operations (moving files, directory structure)                              |
| `[tex]`  | LaTeX/template changes                                                                 |
| `[ci]`   | CI/CD configuration (GitHub Actions, etc.)                                             |
| `[make]` | Makefile changes                                                                       |
| `[git]`  | Git configuration or hooks                                                             |
| `[sed]`  | Find-and-replace across multiple files                                                 |
| `[cfg]`  | Configuration files (tsconfig, package.json, .env, Makefile, etc.)                     |
| `[exe]`  | Command execution which generates machine edits (e.g. npm install)                     |
| `[ai]`   | AI-authored edits (Copilot/Codex only) — Required in addition to language/purpose tags |

**Examples:**

```
[go] add retry logic to serial port manager
[js] fix chart rendering on mobile devices
[docs] update deployment guide for Pi 5
[py][sql] add site configuration schema and report support
```

**Multiple tags:** When a commit affects multiple languages, include all relevant tags. Prefer splitting into separate commits when practical.

## Design Language

All UI and chart work must follow the design contract in **[DESIGN.md](docs/ui/DESIGN.md)**. Key requirements:

- Use the **canonical percentile colour palette** (DESIGN.md §3.3) for all chart stacks.
- Follow the **information hierarchy**: context header → control strip → primary workspace → detail/inspector.
- Use **svelte-ux** components first; only fall back to native HTML with justification.
- Use **LayerChart/d3-scale** for charts; avoid ad-hoc route-level SVG.
- Extract repeated class bundles into **shared standard classes** (DESIGN.md §5.5).
- Include explicit **loading/empty/error states** for charts.

See DESIGN.md §9 for the full UI/chart PR checklist.

## Pull Request Process

1. **Fork & branch** — Create a feature branch from `main`
2. **Make changes** — Follow the code style guidelines
3. **Test locally** — Run `make format && make lint && make test`
4. **Update docs** — If your change affects functionality, update relevant documentation
5. **Submit PR** — Provide a clear description of what and why
6. **Review** — Address any feedback from maintainers

### PR Checklist

- [ ] `make lint` passes
- [ ] `make test` passes
- [ ] Documentation updated (if applicable)
- [ ] Commit messages follow the prefix format

## Testing

### Running Tests

```bash
# All tests
make test

# By component
make test-go       # Go unit tests
make test-python   # Python tests
make test-web      # Web tests (Jest)
```

### Writing Tests

- Go: Place tests in `*_test.go` files alongside the code
- Python: Tests live in `tools/pdf-generator/pdf_generator/tests/`
- Web: Tests use Jest with test files matching `**/__tests__/**/*.[jt]s` or `**/?(*.)+(spec|test).[jt]s`

## Project Structure

```
velocity.report/
├── cmd/                  # Go CLI applications
├── internal/             # Go server internals
├── web/                  # Svelte web frontend
├── tools/pdf-generator/  # Python PDF generation
├── docs/                 # Documentation site (Eleventy)
└── scripts/              # Utility scripts
```

See [ARCHITECTURE.md](ARCHITECTURE.md) for detailed system design.

## Documentation

When changing functionality, update ALL relevant docs:

- Main [README.md](README.md)
- Component READMEs (e.g., `web/README.md`, `tools/pdf-generator/README.md`)
- [ARCHITECTURE.md](ARCHITECTURE.md) for design changes
- [docs/src/guides/](docs/src/guides/) for user-facing guides

## Getting Help

- **Discord** — Best for quick questions and discussion: [discord.gg/XXh6jXVFkt](https://discord.gg/XXh6jXVFkt)
- **GitHub Issues** — For bugs and feature requests
- **Code Review** — We're happy to provide guidance on PRs

## License

By contributing, you agree that your contributions will be licensed under the [Apache License 2.0](LICENSE).

---

Thank you for helping make streets safer!
