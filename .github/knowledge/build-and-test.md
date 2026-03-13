# Build And Test

Canonical reference for build commands, development servers, testing, and environment setup.

## Prerequisites

```bash
git clone git@github.com:banshee-data/velocity.report.git
cd velocity.report
```

## Initial Setup

| Subsystem     | Command                  | Purpose                                  |
| ------------- | ------------------------ | ---------------------------------------- |
| Go server     | `make build-radar-local` | Build with pcap support (local dev)      |
| Python tools  | `make install-python`    | Create `.venv/` and install dependencies |
| Web frontend  | `make install-web`       | Install pnpm/npm dependencies            |
| Documentation | `make install-docs`      | Install Eleventy dependencies            |

### pcap Build Notes

If the Go build fails due to missing pcap dependencies:

- **Debian/Ubuntu:** `sudo apt-get install libpcap-dev`
- **macOS:** `brew install libpcap`
- **Without pcap:** `make build-radar-linux` (no pcap support)

## Quality Gate (Mandatory)

Every commit must pass all three:

```bash
make lint      # Check all code formatting (Go, Python, Web)
make format    # Auto-format all code
make test      # Run all test suites
```

## Testing

| Command            | Scope         |
| ------------------ | ------------- |
| `make test`        | All tests     |
| `make test-go`     | Go unit tests |
| `make test-python` | Python tests  |
| `make test-web`    | Web tests     |

### Per-Language Validation

**Go:**

```bash
make format-go && make lint-go && make test-go
make build-radar-local   # Verify build
```

**Python:**

```bash
make format-python && make lint-python && make test-python
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

## Python Virtual Environment

All Python tools share a **single virtual environment** at the repository root (`.venv/`). Run `make install-python` to create it. There is no per-tool venv.

## SQLite

Driver: `modernc.org/sqlite v1.44.3` (pure-Go, bundles SQLite 3.51.2).

`ALTER TABLE ... DROP COLUMN` is fully supported (SQLite 3.35.0+). New migrations should use `DROP COLUMN` directly instead of the legacy table-recreation workaround. Older migrations (000014–000019) still use the workaround and are left as-is.
