# Contributing to velocity.report

Thank you for your interest in contributing to velocity.report! This document outlines our conventions, workflow, and how to get involved.

## Community & Discussion

[![Discord](https://img.shields.io/discord/1387513267496419359?logo=discord&label=chat%20on%20discord)](https://discord.gg/XXh6jXVFkt)

- Discord — Join our [Discord server](https://discord.gg/XXh6jXVFkt) for real-time discussion, questions, and community support
- GitHub Issues — Report bugs, request features, and track the project roadmap
- GitHub Discussions — For longer-form conversations and ideas

## Privacy Principles

This project is built with privacy as a core value:

- ✅ No cameras or video recording
- ✅ No license plate recognition
- ✅ No personally identifiable information
- ✅ Local-only data storage

All contributions must maintain these principles.

## Contributor Personas

velocity.report covers sensor hardware, real-time data pipelines, web visualisation, data science, and deployment. You do not need to know all of these areas to contribute. Pick the role closest to your background, then start with that section's core documents before diving into issues or plans.

### Data Scientist

Data science in velocity.report is about making the live pipeline more measurable, reproducible, and explainable. The settled foundation is already documented: polar background settling, ground and cluster geometry, Kalman-plus-Hungarian tracking, and a rule-based classifier with explicit features and thresholds. The aim is not to replace that with a black box, but to improve it through labelled reference sets, replayable scorecards, threshold studies, drift analysis, and traffic-engineering metrics that hold up in reports and technical review.

New research follows a proposal-first framework: write down the maths, define the layer boundary, state the evaluation contract, and compare against the current baseline on fixed replay packs. Current research areas include geometry-coherent tracking, velocity-coherent foreground extraction, ground-plane and vector-scene maths, and optional offline classification research. Any future model must stay auditable, beat the transparent baseline on reproducible benchmarks, and preserve a tunable fallback path at runtime.

Open questions that currently need evidence-backed work include:

- whether the 2026-02-22 OBB fixes are good enough in replay, or whether geometry-coherent tracking is still required to stop rotating bounding boxes;
- how radar + LiDAR fusion should be scored and staged: per-track association first, or later L7 scene/canonical-object fusion;
- when the current height-band ground filter is no longer good enough, and what replay/static-export evidence justifies moving to tile-plane and vector-scene maths;
- how OSM/community geometry priors should be diffed, reviewed, signed, and exported without weakening provenance;
- whether highly reflective signs can be promoted into stable pose anchors for shake estimation, how far the intensity gate can be relaxed when signs are absent, whether walls, facades, or road geometry provide enough fallback redundancy without confusing the model with clutter or occlusion, and whether a cached back-edge into lower layers is worth weakening strict one-way layer guarantees;
- whether velocity-coherent extraction beats the current baseline on fixed PCAP/VRLOG packs strongly enough to justify runtime adoption;
- which config values are actually supported by repeatable scorecards, when they were last compared, and what artifact set was used.

When contributing in this area, include the question being answered, the observed result, the exact parameter/config bundle, the validation date, and the replay artifacts used (`.pcap`, `.vrlog`, scene IDs, run IDs, baselines, and any LFS-backed files).

Read next:

- [data/maths/README.md](data/maths/README.md) — the current mathematical foundations across settling, ground modelling, clustering, tracking, and proposals
- [docs/plans/platform-data-science-metrics-first-plan.md](docs/plans/platform-data-science-metrics-first-plan.md) — the repo-wide data science stance: metrics first, no black boxes on the critical path
- [docs/plans/lidar-track-labeling-auto-aware-tuning-plan.md](docs/plans/lidar-track-labeling-auto-aware-tuning-plan.md) — how labelled runs, ground truth, and tuning fit together
- [docs/plans/data-track-description-language-plan.md](docs/plans/data-track-description-language-plan.md) — the metric and schema model for derived transit statistics
- [docs/lidar/operations/auto-tuning.md](docs/lidar/operations/auto-tuning.md) — collected metrics, objectives, and decision-making for tuning
- [data/maths/classification-maths.md](data/maths/classification-maths.md) — the current transparent classification baseline and thresholds

### Designer (UX & Data Visualisation)

Designers help turn speed data into clear, persuasive stories that support safer-street advocacy. This includes information hierarchy, chart design, colour, layout, accessibility, and design system consistency across the product. Contributions can range from Figma exploration to direct Svelte and CSS implementation.

Design work also includes the **PDF report pipeline**. The charts and report visuals should match the web dashboard in palette, typography, and overall visual language so every output feels consistent and professional.

Read next:

- [docs/ui/DESIGN.md](docs/ui/DESIGN.md) — the canonical design language across web, macOS, and report outputs
- [docs/VISION.md](docs/VISION.md) — the product goals, target users, and reporting outcomes the UI needs to support
- [tools/pdf-generator/README.md](tools/pdf-generator/README.md) — the report surface, chart pipeline, and configuration model for generated outputs
- [docs/ui/velocity-visualiser-app/01-problem-and-user-workflows.md](docs/ui/velocity-visualiser-app/01-problem-and-user-workflows.md) — concrete workflows and UX targets for the LiDAR visualiser
- [docs/ui/velocity-visualiser-implementation.md](docs/ui/velocity-visualiser-implementation.md) — current implementation milestones and what the visualiser already supports

### Technical Writer

Technical writers make the project easier to understand, contribute to, and deploy. They work on setup guides, architecture docs, API references, design documents, and the public documentation site. Clear writing has high impact here, especially when it helps non-technical neighbourhood advocates understand how the system works and why it matters.

The project expects documentation to stay structured, accurate, and in step with the code. Writers who can simplify sensor and traffic concepts without losing precision are especially valuable.

Read next:

- [README.md](README.md) — project overview, component map, and contributor setup
- [docs/README.md](docs/README.md) — documentation structure, ownership, and naming rules
- [docs/plans/platform-documentation-standardization-plan.md](docs/plans/platform-documentation-standardization-plan.md) — the current documentation quality contract and validation gates
- [public_html/README.md](public_html/README.md) — how the public docs site is built and organised
- [public_html/src/guides/setup.md](public_html/src/guides/setup.md) — a representative public-facing guide to match tone, structure, and audience

### Perception & Algorithm Engineer

Perception and algorithm engineers turn raw radar and LiDAR data into tracked objects with speed, heading, and classification. This includes clustering, tracking, classification, sensor fusion, and the spatial maths needed to make those steps reliable.

Most of this work happens in Go, with some optional Swift and Metal work for the macOS visualiser. A background in robotics, computer vision, signal processing, or applied geometry is a strong fit.

Read next:

- [docs/lidar/README.md](docs/lidar/README.md) — entry point to the LiDAR subsystem docs
- [docs/lidar/architecture/lidar-pipeline-reference.md](docs/lidar/architecture/lidar-pipeline-reference.md) — the end-to-end LiDAR pipeline and component inventory
- [data/maths/README.md](data/maths/README.md) — how the math-heavy layers fit together
- [data/maths/clustering-maths.md](data/maths/clustering-maths.md) — clustering assumptions, geometry extraction, and complexity
- [data/maths/tracking-maths.md](data/maths/tracking-maths.md) — Kalman filtering, gating, assignment, and lifecycle dynamics

### Platform Engineer

Platform engineers work on the Go server and the systems around it. That includes sensor ingestion, APIs, database work, configuration, deployment, packaging, CI, and release workflows. The aim is simple, reliable deployment on low-cost hardware, especially Raspberry Pi systems used by community advocates.

This role also covers operational quality: observability, logging, health checks, and reliable behaviour on constrained devices. Experience with concurrency, serial or UDP protocols, SQLite, shell tooling, and deployment automation is especially relevant.

Read next:

- [ARCHITECTURE.md](ARCHITECTURE.md) — system boundaries, data flow, and deployment shape
- [cmd/radar/README.md](cmd/radar/README.md) — the main binary, runtime flags, and service model
- [cmd/deploy/README.md](cmd/deploy/README.md) — deployment workflows, upgrade flow, rollback, and health checks
- [docs/radar/cli-comprehensive-guide.md](docs/radar/cli-comprehensive-guide.md) — current CLI surface and planned consolidation
- [internal/db/migrations/README.md](internal/db/migrations/README.md) — schema workflow, migration commands, and production safety
- [config/README.md](config/README.md) — configuration contract and tuning parameter layout
- [docs/plans/deploy-distribution-packaging-plan.md](docs/plans/deploy-distribution-packaging-plan.md) — release packaging strategy and install model
- [docs/radar/architecture/networking.md](docs/radar/architecture/networking.md) — listener segmentation, trust model, and network hardening

### Frontend Engineer (js:Svelte / mac:Swift / py:matplotlib)

Frontend work spans three surfaces: the **Svelte web app**, the **macOS LiDAR visualiser**, and the **PDF report charts**. Across all three, the goal is the same: present complex traffic data clearly, consistently, and accessibly.

Web contributors build real-time dashboards, charts, and configuration flows in Svelte. macOS contributors work on the native visualiser, including rendering, playback, and overlays. PDF chart work uses Python and matplotlib to produce report-ready visuals that match the project's design system. Experience in any one of these areas is useful; contributors do not need to cover all three.

Read next:

- [web/README.md](web/README.md) — local frontend setup, build, and maintenance commands
- [docs/ui/DESIGN.md](docs/ui/DESIGN.md) — the design contract for web, macOS, and report charts
- [docs/ui/design-review-and-improvement.md](docs/ui/design-review-and-improvement.md) — current frontend design gaps and concrete follow-up work
- [docs/plans/web-frontend-consolidation-plan.md](docs/plans/web-frontend-consolidation-plan.md) — the roadmap for retiring legacy Go-embedded dashboards
- [tools/pdf-generator/README.md](tools/pdf-generator/README.md) — PDF report pipeline, chart builders, and configuration
- [tools/visualiser-macos/README.md](tools/visualiser-macos/README.md) — macOS visualiser setup, build, and architecture
- [docs/ui/velocity-visualiser-app/01-problem-and-user-workflows.md](docs/ui/velocity-visualiser-app/01-problem-and-user-workflows.md) — concrete workflows and UX targets for the visualiser
- [docs/ui/velocity-visualiser-implementation.md](docs/ui/velocity-visualiser-implementation.md) — current implementation milestones

## Themes of Work

The following broad themes describe the kinds of work available across the project. Specific tasks live in the [backlog](docs/BACKLOG.md); these themes help contributors find the area that best matches their skills.

### Sensor Integration & Data Pipeline

Ingesting, validating, and storing data from radar and LiDAR sensors. This includes serial and UDP protocol handling, data parsing, schema design, and ensuring data integrity on resource-constrained hardware.

### Tracking, Perception & Sensor Fusion

Turning raw sensor feeds into meaningful objects: clustering point clouds, maintaining tracked identities across frames, classifying vehicles, and fusing radar speed measurements with LiDAR spatial tracks into unified transit records.

### Web Frontend & Visualisation

Building and maintaining the Svelte web app and the macOS visualiser: real-time dashboards, interactive charts, configuration interfaces, native LiDAR playback, overlays, and design system enforcement. This also includes migrating legacy Go-embedded dashboards to Svelte, improving responsiveness, and ensuring accessibility.

### Report Generation & Data Export

Producing professional PDF speed reports suitable for local authority submissions, and providing data export (CSV, GeoJSON) for external analysis. This spans the Python and matplotlib chart pipeline, LaTeX templating, and query-scoped report generation.

### Deployment, Packaging & Platform

Making velocity.report easy to install and run: Raspberry Pi image pipelines, cross-compiled binaries, one-line installers, systemd integration, CI/CD automation, and release management.

### Quality, Testing & Accessibility

Raising and maintaining test coverage across Go, Python, and web components. Includes unit testing, E2E testing with Playwright, visual regression testing, accessibility auditing, and code quality tooling.

### Documentation & Community

Writing and maintaining setup guides, architecture docs, design documents, and the public documentation site. Ensuring that documentation stays accurate as the codebase evolves, and helping new contributors get started.

## Roadmap

Project roadmap and planned features are tracked in [GitHub Issues](https://github.com/banshee-data/velocity.report/issues). Useful labels include:

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

# Public docs site
make install-docs
```

See the [README](README.md) for detailed setup instructions.

## Code Style & Conventions

### Code Formatting

We use standard formatters by language:

| Language   | Formatter           |
| ---------- | ------------------- |
| Go         | `gofmt`             |
| Python     | `black` + `ruff`    |
| JavaScript | `prettier` + ESLint |

Before committing:

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

Allowed prefixes:

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

Examples:

```
[go] add retry logic to serial port manager
[js] fix chart rendering on mobile devices
[docs] update deployment guide for Pi 5
[py][sql] add site configuration schema and report support
```

Multiple tags are fine when a commit affects more than one language or subsystem. Prefer splitting them when practical.

## Design Language

All UI and chart work must follow the design contract in [docs/ui/DESIGN.md](docs/ui/DESIGN.md). Key requirements:

- Use the **canonical percentile colour palette** (DESIGN.md §3.3) for all chart stacks.
- Follow the **information hierarchy**: context header → control strip → primary workspace → detail/inspector.
- Use **svelte-ux** components first; only fall back to native HTML with justification.
- Use **LayerChart/d3-scale** for charts; avoid ad-hoc route-level SVG.
- Extract repeated class bundles into **shared standard classes** (DESIGN.md §5.5).
- Include explicit **loading/empty/error states** for charts.

See `DESIGN.md` section 9 for the full UI and chart PR checklist.

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
├── public_html/          # Public documentation site (Eleventy)
├── tools/pdf-generator/  # Python PDF generation
├── docs/                 # Internal documentation, plans, and architecture notes
└── scripts/              # Utility scripts
```

See [ARCHITECTURE.md](ARCHITECTURE.md) for detailed system design.

## Documentation

When changing functionality, update all relevant docs:

- Main [README.md](README.md)
- Component READMEs such as [web/README.md](web/README.md), [tools/pdf-generator/README.md](tools/pdf-generator/README.md), and [public_html/README.md](public_html/README.md)
- [ARCHITECTURE.md](ARCHITECTURE.md) for design changes
- [public_html/src/guides/](public_html/src/guides/) for user-facing guides

## Getting Help

- Discord — Best for quick questions and discussion: [discord.gg/XXh6jXVFkt](https://discord.gg/XXh6jXVFkt)
- GitHub Issues — For bugs and feature requests
- Code Review — We're happy to provide guidance on PRs

## License

By contributing, you agree that your contributions will be licensed under the [Apache License 2.0](LICENSE).

---

Thank you for helping make streets safer!
