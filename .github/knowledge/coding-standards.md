# Coding Standards

Canonical reference for code style, commit conventions, formatting, and documentation rules.

## Writing Style

See [STYLE.md](../STYLE.md) for British English spelling, punctuation conventions, and prose mechanics.

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

## Product Names

| Product               | Canonical name       | Used in                                                                                         |
| --------------------- | -------------------- | ----------------------------------------------------------------------------------------------- |
| Server + RPi image    | `velocity-report`    | Binary filenames, service name, image filenames, systemd unit                                   |
| Device management CLI | `velocity-ctl`       | Binary filename                                                                                 |
| macOS visualiser      | `VelocityVisualiser` | DMG filename, app bundle, Finder, menu bar. **PascalCase is brand identity — do not lowercase** |

The server and CLI use lowercase-hyphenated names. The macOS app keeps PascalCase because it is the user-facing application name visible in Finder, the Dock, and the About dialog — consistency with macOS conventions takes precedence over filename uniformity.

## Version Format

Versions follow strict SemVer: `MAJOR.MINOR.PATCH` (e.g. `0.5.1`). Pre-release tags use a hyphen suffix: `0.5.1-pre1`, `0.6.0-rc1`.

**No leading zeros in version segments.** `0.5.04` is invalid SemVer and will be rejected by npm (`web/package.json`, `public_html/package.json`). Use `0.5.4` instead.

## Documentation Updates

When changing functionality, update **all** relevant docs:

- Main `README.md`
- Component READMEs: `cmd/radar/README.md`, `web/README.md`
- `ARCHITECTURE.md` for system design changes
- `public_html/src/guides/setup.md` for user-facing setup instructions

## Documentation Metadata

Header metadata in Markdown docs must use the canonical bullet-list format:

```
- **Key:** value
```

Do **not** use standalone bold lines like `**Status:** ...` or `**Owner:** ...` — those are the legacy style and will fail linting. `docs/README.md` has been updated to match this bullet-list convention.

**No date metadata.** Keys like `Created`, `Date`, `Last Updated`, and `Original Design Date` must not appear in header metadata. Dates go stale immediately and duplicate information already available via `git log` / `git blame`. The linter (`scripts/check-doc-header-metadata.py`) enforces this for Markdown files under `docs/`, `config/`, and `data/`, and the weekly lint-autofix workflow removes any that slip through.

## Formatting

Code formatting is automated. Run `make format` before committing. Per-language:

| Language    | Formatter        | Command              |
| ----------- | ---------------- | -------------------- |
| Go          | `gofmt`          | `make format-go`     |
| Python      | `black` + `ruff` | `make format-python` |
| Web (JS/TS) | `prettier`       | `make format-web`    |

## Timestamps

All machine-generated timestamps must be **UTC ISO 8601** with a trailing `Z`:

```
2026-04-07T14:32:08Z
```

This applies to:

- Build metadata (`VR_BUILD_TIME` in `/etc/velocity-report-build`)
- Log output, CI artefacts, and generated reports
- `savedAt` fields in persisted JSON (localStorage, config snapshots)
- Git date attribution when assigning commits to devlog calendar days
- Any tooling or script that stamps a time into a file or output

**Do not use local time for machine timestamps.** Local time introduces ambiguity: a PR merged at `2026-03-31T02:01:38Z` is March 30 in Pacific time but March 31 in UTC. The devlog, build stamps, and all tooling use UTC to avoid this confusion.

**Human-readable dates** in prose (devlog headers, release notes) use `Month DD, YYYY` (e.g. `April 7, 2026`) and are derived from the UTC date of the commit or merge event.

**Internal API timestamps** use Unix nanoseconds (`int64`) as documented in the visualiser API contracts. This is a separate convention for wire formats, not for human-readable or file-stamped timestamps.

## Configuration

Configuration is JSON, version-tagged (`v2`), and validated at load time. See [config/CONFIG.md](../../config/CONFIG.md) for the schema. Do not add CLI flags for values that belong in the tuning file.

## Images and Media

Canonical image assets live in `public_html/src/images/`. The path `docs/images/` is a symlink to that directory, so documentation can reference `docs/images/` for shorter paths. Add new images to `public_html/src/images/`. Use descriptive filenames with hyphens. Keep file sizes under 1 MB where possible. Use SVG for diagrams, PNG for screenshots, GIF for short animations.

## File Naming

Two naming patterns are used across the repository. They carry semantic meaning and are not interchangeable.

### UPPER_CASE.md: canonical reference files

Use `UPPER_CASE.md` for documents that _are_ the canonical answer for their
topic: the one file you open when you need the definitive statement on
something. One per directory or per topic.

| File                                                     | Topic                                    |
| -------------------------------------------------------- | ---------------------------------------- |
| `README.md`                                              | Entry point for a directory or component |
| `ARCHITECTURE.md`                                        | System-wide architecture reference       |
| `CHANGELOG.md`                                           | Release history                          |
| `TENETS.md`                                              | Project principles                       |
| `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`                  | Community governance                     |
| `DEBUGGING.md`, `COMMANDS.md`                            | Operational quick-reference              |
| `docs/BACKLOG.md`, `docs/DECISIONS.md`, `docs/DEVLOG.md` | Project management canon                 |
| `config/CONFIG.md`                                       | Configuration schema reference           |
| `data/maths/MATHS.md`                                    | Maths reference index                    |
| `docs/platform/PLATFORM.md`                              | Platform operations canon                |

**Rule:** if the filename answers "where do I look for X?", it is `UPPER_CASE.md`.

### kebab-case.md: everything else

Use `kebab-case.md` for design plans, operation guides, architecture deep-dives,
specs, and investigations — any document that covers _one aspect_ of a topic
rather than being the definitive reference for it.

Examples: `transit-deduplication.md`, `pdf-go-chart-migration-plan.md`, `auto-tuning.md`.

**Rule:** if the filename describes _what the document covers_ rather than
serving as the index for a topic, it is `kebab-case.md`.
