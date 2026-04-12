# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

**velocity.report** is a privacy-preserving traffic monitoring platform. It measures vehicle speeds using radar and LiDAR sensors mounted on a Raspberry Pi. No cameras, no licence plates, no PII: by architecture, not policy.

Canonical tenets: [TENETS.md](TENETS.md). Full architecture: [ARCHITECTURE.md](ARCHITECTURE.md).

## Commands

The Makefile is the canonical entry point. Run `make help` for all 100+ targets.

### Quality gate (every commit must pass)

```bash
make lint      # Check all code formatting (Go, Python, Web)
make format    # Auto-format all code
make test      # Run all test suites
```

### Per-language validation

```bash
# Go
make format-go && make lint-go && make test-go && make build-radar-local

# Python
make format-python && make lint-python && make test-python

# Web
make format-web && make lint-web && make test-web && make build-web
```

### Building

```bash
make build-radar-local     # Go server, local dev (requires libpcap)
make build-radar-linux     # Go server, ARM64 cross-compile (no pcap)
make build-web             # Svelte frontend → web/build/
make build-ctl             # velocity-ctl management binary
make build-mac             # macOS visualiser (requires Xcode)
```

If `make build-radar-local` fails due to missing pcap: `brew install libpcap` (macOS) or `sudo apt-get install libpcap-dev` (Linux).

### Development servers

```bash
make dev-go    # Go server with radar disabled (localhost:8080)
make dev-web   # Vite dev server (localhost:5173)
```

### Running a single test

```bash
# Go — single package or test
go test ./internal/lidar/l4perception/... -v
go test ./internal/lidar/l5tracks -run '^TestKalmanPredict$' -v

# Python — single file or test
source .venv/bin/activate
cd tools/pdf-generator
pytest pdf_generator/tests/test_config_manager.py -v
pytest pdf_generator/tests/ -k "test_name_substring" -v

# Web (Jest) — single file or test name
cd web
pnpm run test -- path/to/file.test.ts
pnpm run test -- -t "test name regex"
```

### Setup (first time)

```bash
make install-python   # Creates .venv/ at repo root and installs Python deps
make install-web      # Installs web deps via pnpm
make install-docs     # Installs Eleventy deps for docs site
```

### Other useful targets

```bash
make proto-gen        # Regenerate Go + Swift protobuf stubs
make pdf-report CONFIG=config.json   # Generate a PDF report
make pdf-test         # Run PDF generator tests
make test-go-cov      # Go tests with coverage (→ coverage.html)
```

## Architecture

The system has four independent components communicating over HTTP and gRPC:

```
Radar (USB-serial) ──┐
                     ├──► Go server (SQLite) ──► HTTP API (:8080) ──► Web frontend (Svelte)
LiDAR (UDP/Ethernet)─┘         │                               └──► Python PDF generator
                                └──► gRPC (:50051) ──────────────────► macOS visualiser (Swift/Metal)
```

### Go server (`cmd/`, `internal/`)

The core. Runs as a systemd service on Raspberry Pi (ARM64 Linux). Handles:

- **Radar ingest** (`internal/radar/`): serial port reader for OmniPreSense OPS243-A → inserts `radar_data` and `radar_objects`
- **LiDAR ingest** (`internal/lidar/`): UDP packet decoder for Hesai Pandar40P → layered perception pipeline (L1–L9)
- **Transit worker**: background sessionisation of `radar_data` → `radar_data_transits`
- **HTTP API** (`internal/api/`): `GET /api/radar_stats`, `GET /api/config`, `POST /command`
- **gRPC server** (`internal/lidar/l9endpoints/`): streams `FrameBundle` protobufs to the macOS visualiser; supports live, replay (`.vrlog`), and synthetic modes

### LiDAR perception pipeline (`internal/lidar/l*`)

Ten-layer pipeline, L1–L6 implemented; L8 and L9 present as analytics and endpoint packages:

| Layer | Package         | Purpose                                                               |
| ----- | --------------- | --------------------------------------------------------------------- |
| L1    | `l1packets/`    | Hesai UDP decode, PCAP replay                                         |
| L2    | `l2frames/`     | Frame assembly, coordinate transforms                                 |
| L3    | `l3grid/`       | Background subtraction (EMA grid, Welford variance, adaptive regions) |
| L4    | `l4perception/` | DBSCAN clustering, oriented bounding boxes (PCA), ground removal      |
| L5    | `l5tracks/`     | Kalman-filtered MOT, Hungarian assignment, occlusion coasting         |
| L6    | `l6objects/`    | Rule-based classification (8 object types)                            |
| L8    | `l8analytics/`  | Run stats, track labelling, HINT parameter tuner                      |
| L9    | `l9endpoints/`  | gRPC streaming, HTTP API, chart rendering                             |

Parameter tuning: `internal/lidar/sweep/`; combinatorial sweep, auto-tuner, and HINT (human-involved) tuner.

Mathematical references for each layer: `data/maths/`.

### Database (`internal/db/`)

SQLite via `modernc.org/sqlite` (pure-Go, bundles SQLite 3.51.2). JSON-first schema: raw sensor events stored as JSON with generated columns for indexed fields. WAL mode enabled.

Key tables: `radar_data`, `radar_objects`, `radar_data_transits`, `radar_transit_links`, `lidar_bg_snapshot`, `site`, `site_config_periods`. Migrations: `internal/db/migrations/`. Use `DROP COLUMN` directly in new migrations (SQLite 3.35+).

### Python PDF generator (`tools/pdf-generator/`)

CLI tool: fetches data from the Go HTTP API, builds matplotlib charts and PyLaTeX tables, compiles with XeLaTeX to produce professional PDF reports. Shared venv at `.venv/` (repo root). Requires XeLaTeX installed separately.

### Web frontend (`web/`)

Svelte 5 + TypeScript + Vite. Fetches from Go HTTP API. Dev server on `:5173`; production build served as static files by the Go server.

### macOS visualiser (`tools/visualiser-macos/`)

Swift/SwiftUI/Metal app (macOS 14+, M1+). gRPC client streaming `FrameBundle` protos from the Go server. Renders 3D point clouds, track bounding boxes, and trails. Supports live, replay, and synthetic modes.

### Protobuf (`proto/`)

`proto/velocity_visualiser/v1/visualiser.proto` defines `VisualiserService` and `FrameBundle`. Regenerate with `make proto-gen`.

## Agents

Seven named agents are defined in `.claude/agents/`. Invoke them with `@AgentName` or via auto-delegation.

| Agent      | Domain                                    | Class     | File                       |
| ---------- | ----------------------------------------- | --------- | -------------------------- |
| **Appius** | Implementation, code review, migrations   | Technical | `.claude/agents/appius.md` |
| **Euler**  | Algorithms, maths, statistical validation | Technical | `.claude/agents/euler.md`  |
| **Grace**  | Architecture, design docs, feature specs  | Technical | `.claude/agents/grace.md`  |
| **Malory** | Security, pen test, privacy verification  | Technical | `.claude/agents/malory.md` |
| **Flo**    | Planning, sequencing, risk, coordination  | Editorial | `.claude/agents/flo.md`    |
| **Terry**  | Documentation, UX copy, release notes     | Editorial | `.claude/agents/terry.md`  |
| **Ruth**   | Scope decisions, tradeoffs, arbitration   | Both      | `.claude/agents/ruth.md`   |

Paired Copilot definitions live in `.github/agents/`. Check drift with `make check-agent-drift`.

Each agent references the shared knowledge modules in `.github/knowledge/` rather than restating project facts. See `docs/platform/operations/agent-preparedness.md` for the full layered knowledge architecture.

## Skills

The following workflow skills are available as slash commands:

| Skill             | Command                           | Purpose                                                                     |
| ----------------- | --------------------------------- | --------------------------------------------------------------------------- |
| plan-graduation   | `/plan-graduation <plan>`         | Graduate a completed plan to symlink, consolidate into hub doc              |
| plan-review       | `/plan-review [plan]`             | Scope, technical, and risk review of a design plan                          |
| review-pr         | `/review-pr [PR/branch]`          | Security, correctness, and maintainability review                           |
| ship-change       | `/ship-change`                    | Format → lint → test → build → commit                                       |
| weekly-retro      | `/weekly-retro`                   | Weekly backlog health, plan consistency, and drift check                    |
| standup           | `/standup`                        | Daily repo and worktree standup with priorities                             |
| security-review   | `/security-review [path]`         | Security audit: static analysis, fuzz targets, checklist                    |
| trace-matrix      | `/trace-matrix [task-group]`      | Trace backend surfaces against MATRIX.md                                    |
| fix-links         | `/fix-links [path]`               | Fix dead links and stale backtick paths in Markdown                         |
| devlog-update     | `/devlog-update`                  | Update devlog from git history since last entry                             |
| backlog-prune     | `/backlog-prune [--scan-all-prs]` | Groom backlog: PR audit, release theme coherence, L/XL splits               |
| docs-release-prep | `/docs-release-prep [--scope X]`  | Links, graduation, simplify, split, questions, disk image prep              |
| release-prep      | `/release-prep [--scope X]`       | Full release gate: format, lint, test, build, drift, style, docs, changelog |

Skill definitions: `.claude/skills/*/SKILL.md`.

## Commit format

See `.github/knowledge/coding-standards.md` for the full prefix table and rules. AI edits always include `[ai]` plus the language tag.

## Key conventions

See `.github/knowledge/coding-standards.md` for production paths, product names, version format, formatting rules, and documentation update policy.

- **Speed percentiles** (`p85`, `p98`) are aggregate over a population of vehicle max speeds, not per-track observations
