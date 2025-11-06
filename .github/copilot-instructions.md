# velocity.report Development Guidelines

## Core Principles

**Privacy-First Design:**

- No cameras, no license plates, no PII collection
- Velocity measurements only
- Local-only data storage (no cloud transmission)
- User data ownership

**DRY (Don't Repeat Yourself):**

- Avoid duplication across documentation and configuration files
- Reference canonical sources instead of copying
- Link to authoritative docs rather than summarizing
- Update ALL relevant documentation when changing functionality

## Quality Standards (MANDATORY)

**Before any commit or change:**

```bash
make lint      # Check all code formatting (Go, Python, Web)
make format    # Auto-format all code
make test      # Run all test suites
```

All three commands must pass before committing changes.

## Path Conventions

**Critical Paths (use hyphen, not dot):**

- Data directory: `/var/lib/velocity-report/`
- Database: `/var/lib/velocity-report/sensor_data.db`
- Service binary: `/usr/local/bin/velocity-report`
- Python venv: `.venv/` (root level, shared across all Python tools)

**Common Pitfall:** Ensure `/var/lib/velocity-report` (hyphen) not `/var/lib/velocity.report` (dot)

## Documentation Updates

**When changing functionality, update ALL relevant docs:**

- Main `README.md`
- Component READMEs: `cmd/radar/README.md`, `tools/pdf-generator/README.md`, `web/README.md`
- `ARCHITECTURE.md` for system design changes
- `docs/src/guides/setup.md` for user-facing setup instructions

## Active Migrations

**Python venv consolidation (In Progress):**

- Moving from dual-venv to unified `.venv/` at repository root
- Old: `tools/pdf-generator/.venv` (being phased out)
- New: `.venv/` at root (target state)
- Use `.venv/` paths in all new code and documentation

## Build System

**Makefile pattern:** `<action>-<subsystem>[-<variant>]`

- 59 documented targets available
- Cross-compilation for ARM64 (Raspberry Pi 4)
- See `make help` for all targets
