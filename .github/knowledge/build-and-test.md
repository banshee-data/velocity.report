# Build And Test

Canonical reference for build commands, development servers, testing, and environment setup.

## Prerequisites

```bash
git clone git@github.com:banshee-data/velocity.report.git
cd velocity.report
```

## Initial Setup

| Subsystem       | Command                     | Purpose                                         |
| --------------- | --------------------------- | ----------------------------------------------- |
| Go server       | `make build-radar-local`    | Build with pcap support (local dev)             |
| Web frontend    | `make install-web`          | Install pnpm/npm dependencies                   |
| Public site     | `make install-public-html`  | Install Eleventy dependencies in `public_html/` |
| Repository docs | `make install-docs-offline` | Install Eleventy dependencies in `docs_html/`   |

### pcap Build Notes

If the Go build fails due to missing pcap dependencies:

- **Debian/Ubuntu:** `sudo apt-get install libpcap-dev`
- **macOS:** `brew install libpcap`
- **Linux ARM64 image/release build:** `make build-velocity-static-arm64` or `make build-velocity-linux-docker` (uses Docker, zig/musl, and vendored libpcap)

## Quality Gate (Mandatory)

Every commit must pass all three:

```bash
make lint      # Check Go, web, docs, and repository hygiene
make format    # Format Go, web, macOS, Markdown, and SQL
make test      # Run maintained test suites
```

## Testing

| Command                  | Scope                                   |
| ------------------------ | --------------------------------------- |
| `make test`              | Maintained suites across the repository |
| `make test-go`           | Go unit tests                           |
| `make test-web`          | Web tests                               |
| `make test-public-html`  | Public site tests                       |
| `make test-docs-offline` | Embedded repository docs tests          |

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

| Command                 | Purpose                                     |
| ----------------------- | ------------------------------------------- |
| `make dev-go`           | Go server (radar disabled)                  |
| `make dev-web`          | Web dev server (localhost:5173)             |
| `make dev-public-html`  | Public site dev server (localhost:8090)     |
| `make dev-docs-offline` | Repository docs dev server (localhost:8093) |

## Makefile Pattern

Target naming: `<action>-<subsystem>[-<variant>]`

- Cross-compilation for ARM64 (Raspberry Pi 4)
- Run `make help` for the command overview and see [COMMANDS.md](../../COMMANDS.md) for workflows and compatibility aliases

## SQLite

Driver: `modernc.org/sqlite` (pure-Go; exact version pinned in `go.mod`).

`ALTER TABLE ... DROP COLUMN` is fully supported (SQLite 3.35.0+). New migrations should use `DROP COLUMN` directly instead of the legacy table-recreation workaround. Older migrations (000014–000019) still use the workaround and are left as-is.
