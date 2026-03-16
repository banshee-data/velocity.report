# Coding Standards

Canonical reference for code style, commit conventions, formatting, and documentation rules.

## British English (Mandatory)

Use British English spelling and terminology throughout the repository:

- Symbols, filenames, comments, and commit messages
- Examples: `analyse` not `analyze`, `colour` not `color`, `centre` not `center`, `neighbour` not `neighbor`
- **Exception:** External dependencies or rigid standards that require American spelling

## Git Commit Messages

**Format:**

```
[prefix] Description of change

Optional detailed explanation if needed.
```

### Allowed Prefixes

| Prefix   | Scope                                                         |
| -------- | ------------------------------------------------------------- |
| `[go]`   | Go code, server, APIs                                         |
| `[py]`   | Python code (PDF generator, tools)                            |
| `[js]`   | JavaScript/TypeScript (SvelteKit frontend, Vite)              |
| `[mac]`  | macOS files (Swift, Xcode)                                    |
| `[docs]` | Documentation (Markdown guides, READMEs)                      |
| `[sh]`   | Shell scripts (Makefile, bash utilities)                      |
| `[sql]`  | Database schema or SQL migrations                             |
| `[fs]`   | Filesystem operations (moving files, directory structure)     |
| `[tex]`  | LaTeX/template changes                                        |
| `[ci]`   | CI/CD configuration (GitHub Actions)                          |
| `[make]` | Makefile changes                                              |
| `[git]`  | Git configuration or hooks                                    |
| `[sed]`  | Find-and-replace across multiple files                        |
| `[cfg]`  | Configuration files (tsconfig, package.json, .env)            |
| `[exe]`  | Command execution generating machine edits                    |
| `[ai]`   | AI-authored edits — required **in addition to** language tags |

### Rules

- Use lowercase prefix abbreviations in square brackets
- Multiple tags acceptable: `[go][js] update API and frontend`
- Prefer splitting multi-language changes into separate commits
- Human edits: language prefix(es) only — `[go]`, `[js]`, `[go][js]`
- AI edits: always include `[ai]` plus language tag(s) — `[ai][go]`, `[ai][js][py]`
- `[md]` is deprecated in favour of `[docs]`

## Path Conventions

**Critical: use hyphen, not dot.**

| Path                                      | Purpose                          |
| ----------------------------------------- | -------------------------------- |
| `/var/lib/velocity-report/`               | Data directory                   |
| `/var/lib/velocity-report/sensor_data.db` | Database                         |
| `/usr/local/bin/velocity-report`          | Service binary                   |
| `.venv/`                                  | Python venv (root level, shared) |

## Documentation Updates

When changing functionality, update **all** relevant docs:

- Main `README.md`
- Component READMEs: `cmd/radar/README.md`, `tools/pdf-generator/README.md`, `web/README.md`
- `ARCHITECTURE.md` for system design changes
- `public_html/src/guides/setup.md` for user-facing setup instructions

## Documentation Metadata

Header metadata in Markdown docs uses the canonical bullet-list format:

```
- **Key:** value
```

**No date metadata.** Keys like `Created`, `Date`, `Last Updated`, and `Original Design Date` must not appear in header metadata. Dates go stale immediately and duplicate information already available via `git log` / `git blame`. The linter (`scripts/check-doc-header-metadata.py`) enforces this and the weekly lint-autofix workflow removes any that slip through.

## Formatting

Code formatting is automated. Run `make format` before committing. Per-language:

| Language    | Formatter        | Command              |
| ----------- | ---------------- | -------------------- |
| Go          | `gofmt`          | `make format-go`     |
| Python      | `black` + `ruff` | `make format-python` |
| Web (JS/TS) | `prettier`       | `make format-web`    |
