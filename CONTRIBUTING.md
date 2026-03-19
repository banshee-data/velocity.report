```
 ▗▄▄▖ ▗▄▖ ▗▖  ▗▖▗▄▄▄▖▗▄▄▖ ▗▄▄▄▖▗▄▄▖ ▗▖ ▗▖▗▄▄▄▖▗▄▄▄▖▗▖  ▗▖ ▗▄▄
▐▌   ▐▌ ▐▌▐▛▚▖▐▌  █  ▐▌ ▐▌  █  ▐▌ ▐▌▐▌ ▐▌  █    █  ▐▛▚▖▐▌▐▌
▐▌   ▐▌ ▐▌▐▌ ▝▜▌  █  ▐▛▀▚▖  █  ▐▛▀▚▖▐▌ ▐▌  █    █  ▐▌ ▝▜▌▐▌▝▜▌
▝▚▄▄▖▝▚▄▞▘▐▌  ▐▌  █  ▐▌ ▐▌▗▄█▄▖▐▙▄▞▘▝▚▄▞▘  █  ▗▄█▄▖▐▌  ▐▌▝▚▄▞▘
```

This is a project that measures how fast vehicles move through
neighbourhoods so the people who live there can do something
about it. It does this without cameras, without licence plates,
and without collecting the sort of personal data that makes a
privacy officer reach for the whisky. If that sounds worth
working on, read on.

## Community & Discussion

[![Discord](https://img.shields.io/discord/1387513267496419359?logo=discord&label=chat%20on%20discord)](https://discord.gg/XXh6jXVFkt)

- **Discord** — [Join the server](https://discord.gg/XXh6jXVFkt)
  for real-time conversation, questions, and the occasional
  digression about sensor calibration
- **GitHub Issues** — Bugs, feature requests, and the project
  roadmap

## Principles

Every contribution must honour these. They are not aspirational
wallpaper; they are load-bearing walls.

- ✅ No cameras or video recording
- ✅ No licence plate recognition
- ✅ No personally identifiable information
- ✅ Local-only data storage
- ✅ Explainable algorithms: no black-box models

If a feature would be clever but would require a camera, or
convenient but would phone home, it does not get built. The
privacy commitment is the product, not a constraint on it.

## Finding Your Way In

The project spans sensor hardware, real-time data pipelines,
web visualisation, data science, and hardware deployment.
Nobody knows all of it. Pick the role closest to what you
already do, read the linked documents, and then find an
issue that fits.

### Data Scientist

The perception pipeline runs on transparent, auditable maths:
polar background settling, ground and cluster geometry,
Kalman-plus-Hungarian tracking, and a rule-based classifier
with explicit features and thresholds. The aim is not to
replace this with something opaque, but to make it sharper
through labelled reference sets, replayable scorecards,
threshold studies, drift analysis, and traffic-engineering
metrics that hold up in front of people who read footnotes.

New research follows a proposal-first discipline: write down
the maths, define the layer boundary, state the evaluation
contract, and compare against the current baseline on fixed
replay packs. Any future model must stay auditable, beat the
baseline on reproducible benchmarks, and preserve a tunable
fallback path at runtime. A model that performs well but
cannot explain itself is not welcome on the critical path.

Current research areas include geometry-coherent tracking,
velocity-coherent foreground extraction, ground-plane and
vector-scene maths, and optional offline classification work.

#### Open questions that need evidence, not opinion:

1. Whether the [2026-02-22 OBB](data/maths/proposals/20260222-obb-heading-stability-review.md) fixes hold up in replay, or
   whether [geometry-coherent tracking](data/maths/proposals/20260222-geometry-coherent-tracking.md) is still needed to stop
   bounding boxes rotating like weathervanes.
1. Whether
   [velocity-coherent extraction](data/maths/proposals/20260220-velocity-coherent-foreground-extraction.md)
   beats the current baseline on fixed PCAP/VRLOG packs
   strongly enough to justify
   [runtime adoption](docs/plans/lidar-velocity-coherent-foreground-extraction-plan.md).
1. Whether highly reflective signs can serve as stable [pose anchors](data/maths/proposals/20260310-reflective-sign-pose-anchor-maths.md),
   how far the intensity gate can be relaxed without them, and
   whether walls or road geometry provide enough fallback
   without confusing the model.
1. How radar + LiDAR fusion should be scored and staged:
   existing [per-track association](data/maths/tracking-maths.md), or
   [scene-level fusion](docs/plans/lidar-l7-scene-plan.md).
1. When the current
   [height-band ground filter](data/maths/ground-plane-maths.md)
   stops being good enough, and what replay evidence justifies
   moving to [tile-plane and vector-scene maths](data/maths/proposals/20260221-ground-plane-vector-scene-maths.md).
1. How [OSM/community geometry priors](docs/plans/lidar-l7-scene-plan.md)
   should be diffed, reviewed, signed, and exported without
   weakening
   [provenance](docs/lidar/architecture/vector-scene-map.md).
1. Which [config values](config/tuning.defaults.json) are actually
   supported by repeatable
   [scorecards](docs/plans/lidar-parameter-tuning-optimisation-plan.md),
   when they were last compared, and with what artefact set.

When contributing here, include the question being answered,
the observed result, the exact parameter bundle, the
validation date, and the replay artefacts used (`.pcap`,
`.vrlog`, scene IDs, run IDs, baselines, and any LFS-backed
files). Claims without artefacts are anecdotes.

Read next:

- [Pipeline Architecture](docs/lidar/architecture/lidar-data-layer-model.md): Ten layer data processing stack, from sensors to visualisation tools
- [data/maths/README.md](data/maths/README.md): mathematical
  foundations across settling, ground modelling, clustering,
  tracking, and proposals
- [docs/plans/platform-data-science-metrics-first-plan.md](docs/plans/platform-data-science-metrics-first-plan.md):
  the repo-wide data science stance: metrics first, no black
  boxes on the critical path
- [docs/plans/lidar-track-labelling-auto-aware-tuning-plan.md](docs/plans/lidar-track-labelling-auto-aware-tuning-plan.md):
  how labelled runs, ground truth, and tuning fit together
- [docs/plans/data-track-description-language-plan.md](docs/plans/data-track-description-language-plan.md):
  metric and schema model for derived transit statistics
- [docs/lidar/operations/auto-tuning.md](docs/lidar/operations/auto-tuning.md):
  collected metrics, objectives, and decision-making for
  tuning
- [data/maths/classification-maths.md](data/maths/classification-maths.md):
  the current boring static classifier

### Designer (UX & Data Visualisation)

Designers turn speed data into clear, persuasive stories that
help people argue for safer streets. This includes information
hierarchy, chart design, colour, layout, accessibility, and
design system consistency across the product. Contributions
range from Figma exploration to hands-on Svelte and CSS.

This also includes the **PDF report pipeline**. The charts and
visuals in generated reports should match the web dashboard in
palette, typography, and overall visual language so every
output looks like it came from the same project, because it
did.

Read next:

- [docs/ui/DESIGN.md](docs/ui/DESIGN.md): the canonical
  design language across web, macOS, and report outputs
- [docs/VISION.md](docs/VISION.md): product goals, target
  users, and reporting outcomes the UI needs to support
- [tools/pdf-generator/README.md](tools/pdf-generator/README.md):
  report surface, chart pipeline, and configuration model
- [docs/ui/velocity-visualiser-app/01-problem-and-user-workflows.md](docs/ui/velocity-visualiser-app/01-problem-and-user-workflows.md):
  concrete workflows and UX targets for the LiDAR visualiser
- [docs/ui/velocity-visualiser-implementation.md](docs/ui/velocity-visualiser-implementation.md):
  current implementation milestones

### Technical Writer

Technical writers make the project easier to understand,
contribute to, and deploy. Setup guides, architecture docs,
API references, design documents, and the public documentation
site all benefit from someone who can explain sensor and
traffic concepts without losing precision or the reader.

The project expects documentation to stay structured, accurate,
and in step with the code. Documentation that falls behind the
implementation is not documentation; it is a trap with good
formatting.

Read next:

- [README.md](README.md): project overview, component map,
  and contributor setup
- [docs/README.md](docs/README.md): documentation structure,
  ownership, and naming rules
- [docs/plans/platform-documentation-standardisation-plan.md](docs/plans/platform-documentation-standardisation-plan.md):
  the current documentation quality contract
- [public_html/README.md](public_html/README.md): how the
  public docs site is built and organised
- [public_html/src/guides/setup.md](public_html/src/guides/setup.md):
  a representative public-facing guide for tone and structure

### Perception & Algorithm Engineer

Perception and algorithm engineers turn raw radar and LiDAR
data into tracked objects with speed, heading, and
classification. Clustering, tracking, classification, sensor
fusion, and the spatial maths that make those steps reliable
are all in scope.

Most of this work happens in Go, with some optional Swift and
Metal for the macOS visualiser. A background in robotics,
computer vision, signal processing, or applied geometry fits
well. A tolerance for point clouds that occasionally contain a
seagull also helps.

Read next:

- [docs/lidar/README.md](docs/lidar/README.md): entry point
  to the LiDAR subsystem docs
- [docs/lidar/architecture/lidar-pipeline-reference.md](docs/lidar/architecture/lidar-pipeline-reference.md):
  end-to-end LiDAR pipeline and component inventory
- [data/maths/README.md](data/maths/README.md): how the
  maths-heavy layers fit together
- [data/maths/clustering-maths.md](data/maths/clustering-maths.md):
  clustering assumptions, geometry extraction, and complexity
- [data/maths/tracking-maths.md](data/maths/tracking-maths.md):
  Kalman filtering, gating, assignment, and lifecycle dynamics

### Platform Engineer

Platform engineers work on the Go server and everything around
it: sensor ingestion, APIs, database work, configuration,
deployment, packaging, CI, and release workflows. The aim is
simple, reliable deployment on low-cost hardware — especially
Raspberry Pi systems used by community advocates who have
better things to do than diagnose why a service failed to start
at three in the morning.

This also covers operational quality: observability, logging,
health checks, and graceful behaviour on constrained devices.
Experience with concurrency, serial or UDP protocols, SQLite,
shell tooling, and deployment automation is useful.

Read next:

- [ARCHITECTURE.md](ARCHITECTURE.md): system boundaries,
  data flow, and deployment shape
- [cmd/radar/README.md](cmd/radar/README.md): the main
  binary, runtime flags, and service model
- [cmd/deploy/README.md](cmd/deploy/README.md): deployment
  workflows, upgrade flow, rollback, and health checks
- [docs/radar/cli-comprehensive-guide.md](docs/radar/cli-comprehensive-guide.md):
  current CLI surface and planned consolidation
- [internal/db/migrations/README.md](internal/db/migrations/README.md):
  schema workflow, migration commands, and production safety
- [config/README.md](config/README.md): configuration
  contract and tuning parameter layout
- [docs/plans/deploy-distribution-packaging-plan.md](docs/plans/deploy-distribution-packaging-plan.md):
  release packaging strategy and install model
- [docs/radar/architecture/networking.md](docs/radar/architecture/networking.md):
  listener segmentation, trust model, and network hardening

### Frontend Engineer (js:Svelte / mac:Swift / py:matplotlib)

Frontend work spans three surfaces: the **Svelte web app**,
the **macOS LiDAR visualiser**, and the **PDF report charts**.
The goal across all three is the same: present complex traffic
data clearly, consistently, and accessibly.

Web contributors build real-time dashboards, charts, and
configuration flows in Svelte. macOS contributors work on the
native visualiser — rendering, playback, and overlays. PDF
chart work uses Python and matplotlib to produce report-ready
visuals that match the project's design system. Experience in
any one area is welcome; nobody is expected to cover all three.

Read next:

- [web/README.md](web/README.md): local frontend setup,
  build, and maintenance commands
- [docs/ui/DESIGN.md](docs/ui/DESIGN.md): design contract
  for web, macOS, and report charts
- [docs/ui/design-review-and-improvement.md](docs/ui/design-review-and-improvement.md):
  current frontend design gaps and follow-up work
- [docs/plans/web-frontend-consolidation-plan.md](docs/plans/web-frontend-consolidation-plan.md):
  roadmap for retiring legacy Go-embedded dashboards
- [tools/pdf-generator/README.md](tools/pdf-generator/README.md):
  PDF report pipeline, chart builders, and configuration
- [tools/visualiser-macos/README.md](tools/visualiser-macos/README.md):
  macOS visualiser setup, build, and architecture
- [docs/ui/velocity-visualiser-app/01-problem-and-user-workflows.md](docs/ui/velocity-visualiser-app/01-problem-and-user-workflows.md):
  concrete workflows and UX targets
- [docs/ui/velocity-visualiser-implementation.md](docs/ui/velocity-visualiser-implementation.md):
  current implementation milestones

## Themes of Work

These are the broad areas of work across the project. Specific
tasks live in the [backlog](docs/BACKLOG.md); the themes help
you find the part that fits your hands.

### Sensor Integration & Data Pipeline

Getting data in from radar and LiDAR sensors, validating it,
and storing it. Serial and UDP protocol handling, data parsing,
schema design, and making sure nothing gets quietly lost on
hardware that costs forty pounds.

### Tracking, Perception & Sensor Fusion

Turning raw sensor feeds into meaningful objects: clustering
point clouds, maintaining tracked identities across frames,
classifying vehicles, and fusing radar speed with LiDAR
spatial tracks. The goal is a unified transit record that
would survive a polite cross-examination.

### Web Frontend & Visualisation

The Svelte web app and the macOS visualiser: real-time
dashboards, interactive charts, configuration interfaces,
native LiDAR playback, overlays, and design system
enforcement. Also includes migrating legacy Go-embedded
dashboards to Svelte, improving responsiveness, and ensuring
the whole thing works for people who use screen readers.

### Report Generation & Data Export

Producing professional PDF speed reports suitable for
submitting to a local authority, and providing data export
(CSV, GeoJSON) for external analysis. This spans the Python
and matplotlib chart pipeline, LaTeX templating, and
query-scoped report generation. The report is often the first
thing a council officer reads, so it needs to look like it was
made on purpose.

### Deployment, Packaging & Platform

Making velocity.report straightforward to install and run:
Raspberry Pi image pipelines, cross-compiled binaries,
one-line installers, systemd integration, CI/CD automation,
and release management. The target user is a neighbourhood
advocate, not a systems administrator.

### Quality, Testing & Accessibility

Raising and maintaining test coverage across Go, Python, and
web components. Unit testing, E2E testing with Playwright,
visual regression testing, accessibility auditing, and code
quality tooling. The test suite is the last thing standing
between a commit and a user having a bad afternoon.

### Documentation & Community

Writing and maintaining setup guides, architecture docs,
design documents, and the public documentation site. Keeping
documentation accurate as the code evolves, and helping new
contributors find their footing without needing to read the
entire commit history first.

## Roadmap

The project roadmap lives in
[GitHub Issues](https://github.com/banshee-data/velocity.report/issues).
Useful labels:

- `enhancement` — New features and improvements
- `bug` — Known problems awaiting attention
- `good first issue` — Manageable starting points that will
  not leave you staring at a screen wondering what just
  happened
- `help wanted` — Issues where extra hands would make a real
  difference

## Getting Started

### Prerequisites

- **Go 1.25+** — server development
- **Python 3.11+** — PDF generator
- **Node.js 18+** with pnpm — web frontend
- **SQLite3** — database (also the entire database strategy,
  which is one of the nicer things about the project)

### Initial Setup

```bash
git clone git@github.com:banshee-data/velocity.report.git
cd velocity.report

make build-radar-local   # Go server
make install-python      # Python environment
make install-web         # Web frontend
make install-docs        # Public docs site
```

See the [README](README.md) for the full story.

## Code Style & Conventions

### Formatting

Each language has a formatter. The formatters are not optional.

| Language   | Formatter           |
| ---------- | ------------------- |
| Go         | `gofmt`             |
| Python     | `black` + `ruff`    |
| JavaScript | `prettier` + ESLint |

Before committing, run all three:

```bash
make format    # Auto-format everything
make lint      # Check it worked
make test      # Make sure nothing caught fire
```

All three must pass before submitting a PR. This is the
quality gate, and it does not have a side entrance.

### Pre-commit Hooks (Recommended)

For regular contributors, install hooks that format on commit
so you do not have to remember:

```bash
pip install pre-commit
pre-commit install
```

### Advisory Linting (Non-Blocking)

Some lint checks are **advisory** — they report issues without
blocking your PR. This is a deliberate low-friction workflow:

1. **Local check** — Run `make lint` to see all warnings
   including line-width reports.
2. **Pre-commit hook** — Auto-formats code on commit if you
   have `pre-commit install` enabled. Width-related prose
   checks are opt-in and advisory.
3. **CI check** — PR checks include an advisory line-width
   job (`continue-on-error: true`). It shows a yellow tick,
   not a red cross. Your PR can merge regardless.
4. **Weekly nag PR** — A scheduled workflow opens a standing
   PR each week with any remaining style fixes. Easy to
   review and merge — no manual effort required.

This means you never need to stop work for a style issue.
Fix what you can locally, let CI flag the rest, and the
weekly nag PR sweeps up anything that slips through.

**Line width:** The repo target is **100 columns** for all
code and prose. Formatters (prettier, swift-format) are
configured to enforce this; Python currently uses `black` with
its default 88-column line length. The prose-width linter checks
Markdown. For details see
[`line-width-standardisation-plan.md`](docs/plans/line-width-standardisation-plan.md).

## Git Workflow

### Branch Naming

Name branches so a stranger can guess the contents:

- `feature/` — New features (e.g., `feature/lidar-tracking`)
- `fix/` — Bug fixes (e.g., `fix/api-timeout`)
- `docs/` — Documentation (e.g., `docs/update-setup-guide`)
- `refactor/` — Tidying (e.g., `refactor/db-layer`)

### Commit Messages

Prefix each commit message with the primary language or
purpose. This makes the log scannable for humans who are not
yet machines.

```
[prefix] Description of change
```

| Prefix   | Use                                                                     |
| -------- | ----------------------------------------------------------------------- |
| `[go]`   | Go code, server, APIs                                                   |
| `[py]`   | Python (PDF generator, tools)                                           |
| `[js]`   | JavaScript/TypeScript (SvelteKit, Vite)                                 |
| `[mac]`  | macOS files (Swift, Xcode)                                              |
| `[docs]` | Documentation (Markdown, READMEs)                                       |
| `[sh]`   | Shell scripts (Makefile, bash)                                          |
| `[sql]`  | Database schema or migrations                                           |
| `[fs]`   | Filesystem operations (moves, renames)                                  |
| `[tex]`  | LaTeX/template changes                                                  |
| `[ci]`   | CI/CD configuration (GitHub Actions)                                    |
| `[make]` | Makefile changes                                                        |
| `[git]`  | Git configuration or hooks                                              |
| `[sed]`  | Find-and-replace across multiple files                                  |
| `[cfg]`  | Configuration files (tsconfig, package.json, etc.)                      |
| `[exe]`  | Machine-generated edits (e.g., npm install)                             |
| `[ai]`   | AI-authored edits (Copilot/Codex) — required alongside the language tag |

Examples:

```
[go] add retry logic to serial port manager
[js] fix chart rendering on mobile devices
[docs] update deployment guide for Pi 5
[py][sql] add site configuration schema and report support
```

Multiple tags are fine when a commit touches more than one
area. Split commits when practical.

## Design Language

All UI and chart work follows the design contract in
[docs/ui/DESIGN.md](docs/ui/DESIGN.md). The short version:

- Use the **canonical percentile colour palette** (§3.3) for
  all chart stacks.
- Follow the **information hierarchy**: context header →
  control strip → primary workspace → detail/inspector.
- Use **svelte-ux** components first; fall back to native HTML
  only with good reason.
- Use **LayerChart/d3-scale** for charts; avoid ad-hoc SVG.
- Extract repeated class bundles into **shared standard
  classes** (§5.5).
- Include explicit **loading/empty/error states** for charts.
  An empty chart without explanation is a mild act of cruelty.

See DESIGN.md §9 for the full UI and chart PR checklist.

## Pull Requests

1. **Fork & branch** — Create a feature branch from `main`.
2. **Make changes** — Follow the code style conventions above.
3. **Test locally** — `make format && make lint && make test`.
4. **Update docs** — If your change affects behaviour, update
   the relevant documentation. Future-you will be grateful.
5. **Submit PR** — Describe what changed and why. The "why"
   matters more than the "what"; the diff already shows the
   what.
6. **Review** — Address feedback from maintainers.

### PR Checklist

- [ ] `make lint` passes
- [ ] `make test` passes
- [ ] Documentation updated (if applicable)
- [ ] Commit messages follow the prefix format

## Testing

### Running Tests

```bash
make test              # Everything
make test-go           # Go unit tests
make test-python       # Python tests
make test-web          # Web tests (Jest)
```

### Writing Tests

- **Go** — `*_test.go` files alongside the code they test.
- **Python** — `tools/pdf-generator/pdf_generator/tests/`.
- **Web** — Jest, with test files matching
  `**/__tests__/**/*.[jt]s` or `**/?(*.)+(spec|test).[jt]s`.

If you change behaviour, write a test that would have caught
the problem. If you fix a bug, write a test that reproduces
it. A bug without a regression test is a bug that will come
back when you are on holiday.

## Project Structure

```
velocity.report/
├── cmd/                  # Go CLI applications
├── internal/             # Go server internals
├── web/                  # Svelte web frontend
├── public_html/          # Public documentation site (Eleventy)
├── tools/pdf-generator/  # Python PDF generation
├── docs/                 # Internal docs, plans, architecture
└── scripts/              # Utility scripts
```

See [ARCHITECTURE.md](ARCHITECTURE.md) for the full picture.

## Documentation

When changing behaviour, update all affected docs:

- The main [README.md](README.md)
- Component READMEs:
  [web/README.md](web/README.md),
  [tools/pdf-generator/README.md](tools/pdf-generator/README.md),
  [public_html/README.md](public_html/README.md)
- [ARCHITECTURE.md](ARCHITECTURE.md) for design changes
- [public_html/src/guides/](public_html/src/guides/) for
  user-facing guides

Documentation that contradicts the code is worse than no
documentation at all, because at least an absence is honest.

## Getting Help

- **Discord** — Best for quick questions:
  [discord.gg/XXh6jXVFkt](https://discord.gg/XXh6jXVFkt)
- **GitHub Issues** — For bugs and feature requests
- **Code Review** — We are happy to guide you through a PR.
  Ask early rather than late; it saves everyone time.

## Licence

By contributing, you agree that your contributions will be
licensed under the [Apache Licence 2.0](LICENSE).

---

Streets are safer when the people who live on them have
evidence. That is the point of the project, and the point of
your contribution.
