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
