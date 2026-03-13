# velocity.report Development Guidelines

> **Canonical knowledge lives in `.github/knowledge/`.** This file summarises the essentials that every Copilot interaction needs. For deeper detail, read the referenced module.

## Tenets

See [TENETS.md](TENETS.md) for the full project constitution. The short version:

1. **Privacy above all** — no cameras, no licence plates, no PII, local-only data
2. **Protect the vulnerable** — safety for people who walk, cycle, and play
3. **Evidence over opinion** — measured data, not anecdote
4. **Local-first** — Raspberry Pi, SQLite, offline-capable
5. **Simplicity and durability** — smallest change that solves the problem
6. **British English** — all code, comments, docs, symbols, commit messages
7. **DRY** — every fact has one canonical source

## Project Overview

**velocity.report** is a privacy-focused traffic monitoring system for neighbourhood change-makers.

| Component        | Language          | Location                  |
| ---------------- | ----------------- | ------------------------- |
| Server           | Go                | `cmd/`, `internal/`       |
| PDF reports      | Python + LaTeX    | `tools/pdf-generator/`    |
| Web frontend     | Svelte/TypeScript | `web/`                    |
| macOS visualiser | Swift/Metal       | `tools/visualiser-macos/` |
| Database         | SQLite            | local file                |
| Docs site        | Eleventy          | `public_html/`            |

**Deployment:** Raspberry Pi 4 (ARM64 Linux), systemd service, local-only.

For full architecture, tech stack, data flow, and schema: see [knowledge/architecture.md](knowledge/architecture.md).

## Quality Gate (Mandatory)

Every commit must pass:

```bash
make lint      # Check all code formatting
make format    # Auto-format all code
make test      # Run all test suites
```

## Validation Protocol

| Language | Commands                                                                   |
| -------- | -------------------------------------------------------------------------- |
| Go       | `make format-go && make lint-go && make test-go && make build-radar-local` |
| Python   | `make format-python && make lint-python && make test-python`               |
| Web      | `make format-web && make lint-web && make test-web && make build-web`      |

If the Go build fails due to missing pcap: use `make build-radar-linux` (no pcap support).

For full build setup, dev servers, and testing: see [knowledge/build-and-test.md](knowledge/build-and-test.md).

## Quick Reference

**Critical paths (hyphen, not dot):**

| Path                                      | Purpose                          |
| ----------------------------------------- | -------------------------------- |
| `/var/lib/velocity-report/`               | Data directory                   |
| `/var/lib/velocity-report/sensor_data.db` | Database                         |
| `/usr/local/bin/velocity-report`          | Service binary                   |
| `.venv/`                                  | Python venv (shared, root level) |

**Commit format:** `[prefix] description` — see [knowledge/coding-standards.md](knowledge/coding-standards.md) for prefix table and rules. AI edits always include `[ai]` tag plus language tag.

**Python venv:** Single shared venv at `.venv/`. Run `make install-python` to create.

**SQLite:** `modernc.org/sqlite v1.44.3` (SQLite 3.51.2). Use `DROP COLUMN` directly in new migrations.

**Documentation:** When changing functionality, update README.md, component READMEs, ARCHITECTURE.md, and public_html/src/guides/setup.md as relevant.

## Knowledge Modules

Read these on demand when you need deeper context:

| Module                                                             | Content                                        |
| ------------------------------------------------------------------ | ---------------------------------------------- |
| [TENETS.md](TENETS.md)                                             | Project constitution                           |
| [knowledge/architecture.md](knowledge/architecture.md)             | Tech stack, data flow, schema, deployment      |
| [knowledge/build-and-test.md](knowledge/build-and-test.md)         | Make targets, dev servers, testing, setup      |
| [knowledge/coding-standards.md](knowledge/coding-standards.md)     | British English, commits, paths, formatting    |
| [knowledge/hardware.md](knowledge/hardware.md)                     | Radar/LIDAR specs, serial/UDP, Raspberry Pi    |
| [knowledge/security-surface.md](knowledge/security-surface.md)     | Attack surface map, vulnerability patterns     |
| [knowledge/security-checklist.md](knowledge/security-checklist.md) | Review criteria, severity, gate classification |
| [knowledge/role-technical.md](knowledge/role-technical.md)         | Shared context for technical agents            |
| [knowledge/role-editorial.md](knowledge/role-editorial.md)         | Shared context for editorial agents            |

## Task Guidance

**Good tasks for Copilot:** Bug fixes, unit tests, documentation updates, code refactoring within clear boundaries, new API endpoints with specifications, UI enhancements with clear requirements, accessibility improvements, technical debt reduction.

**Avoid assigning to Copilot:** Complex cross-component refactors, security-critical changes (auth, data privacy, sensor data handling), production incident response, ambiguous tasks without acceptance criteria, large architectural changes, deep radar/LIDAR domain work (signal processing, sensor calibration).
