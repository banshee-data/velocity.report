# Build And Test

Canonical reference for build commands, development servers, testing, and environment setup.

## Prerequisites

```bash
git clone git@github.com:banshee-data/velocity.report.git
cd velocity.report
```

## Initial Setup

| Subsystem                 | Command                  | Purpose                                                          |
| ------------------------- | ------------------------ | ---------------------------------------------------------------- |
| Go server                 | `make build-radar-local` | Build with pcap support (local dev)                              |
| Web frontend              | `make install-web`       | Install pnpm/npm dependencies                                    |
| Documentation             | `make install-docs`      | Install Eleventy dependencies                                    |
| Python tools (DEPRECATED) | `make install-python`    | Create `.venv/` for deprecated pdf-generator reference copy only |

### pcap Build Notes

If the Go build fails due to missing pcap dependencies:

- **Debian/Ubuntu:** `sudo apt-get install libpcap-dev`
- **macOS:** `brew install libpcap`
- **Linux ARM64 cross-compile:** `make build-radar-linux` (pcap required — install `libpcap-dev` first)

## Quality Gate (Mandatory)

Every commit must pass all three:

```bash
make lint      # Check all code formatting (Go, Web)
make format    # Auto-format all code
make test      # Run all test suites
```

## Testing

| Command                         | Scope                                       |
| ------------------------------- | ------------------------------------------- |
| `make test`                     | All tests (Go + Web)                        |
| `make test-go`                  | Go unit tests                               |
| `make test-web`                 | Web tests                                   |
| `make test-python` (DEPRECATED) | Python PDF generator tests — local dev only |

### Per-Language Validation

**Go:**

```bash
make format-go && make lint-go && make test-go
make build-radar-local   # Verify build
```

**Web:**

```bash
make format-web && make lint-web && make test-web
make build-web           # Verify production build
```

## Development Servers

| Command         | Purpose                         |
| --------------- | ------------------------------- |
| `make dev-go`   | Go server (radar disabled)      |
| `make dev-web`  | Web dev server (localhost:5173) |
| `make dev-docs` | Documentation dev server        |

## Makefile Pattern

Target naming: `<action>-<subsystem>[-<variant>]`

- 101+ documented targets available
- Cross-compilation for ARM64 (Raspberry Pi 4)
- Run `make help` for the full list

## Python Virtual Environment (DEPRECATED)

The Python venv at `.venv/` is retained for local development against the deprecated `tools/pdf-generator/` reference copy only. PDF generation in production uses the Go pipeline (`internal/report/`). Run `make install-python` to create the venv if you need it for reference work.

## SQLite

Driver: `modernc.org/sqlite v1.44.3` (pure-Go, bundles SQLite 3.51.2).

`ALTER TABLE ... DROP COLUMN` is fully supported (SQLite 3.35.0+). New migrations should use `DROP COLUMN` directly instead of the legacy table-recreation workaround. Older migrations (000014–000019) still use the workaround and are left as-is.
