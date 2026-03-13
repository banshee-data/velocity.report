```
      __     _
     / /  __| |    ___     __      ___
    / /  / _  |   / _ \   / _|    (_-<
  _/_/_  \__,_|   \___/   \__|    /__/
_|"""""|_|"""""|_|"""""|_|"""""|_|"""""|
"`-o-o-'"`-o-o-'"`-o-o-'"`-o-o-'"`-o-o-'
```

# /docs

Documentation structure, scope, and naming conventions.

## /docs/ root structure

- `/docs/lidar/` # LiDAR pipeline and sensor
- `/docs/radar/` # Radar sensor processing
- `/docs/ui/` # Web, mac, PDF clients
- `/docs/maths/` # Algorithms and signal theory
- `/docs/plans/` # Implementation plans, roadmap
- `/docs/BACKLOG.md` # Prioritised work queue with versioned milestones (v0.5 → v2.0)
- `/docs/DECISIONS.md` # Resolved executive decisions register
- `/docs/VISION.md` # Product vision and backlog alignment guide
- `/docs/COVERAGE.md` # Test coverage tracking
- `/docs/DEVLOG.md` # Chronological development notes
- `/docs/TROUBLESHOOTING.md` # Cross-component debugging guide

## Scope Rules

- Capability docs live under the owning root hub (`lidar`, `radar`).
  - Architecture and design specifications live under `<hub>/architecture/`.
  - Operational guides and implementation status live under `<hub>/operations/`.
- UI/client surface docs for web/mac/pdf live under `docs/ui/`.
- `docs/plans/` contains forward-looking implementation plans and deferred roadmap work only. Completed architecture specs and implementation records belong in their hub folder.
- Maths stays separate under `docs/maths/`.
- Maths proposals live only in `docs/maths/proposals/`.

## Naming Conventions

All documentation files use **lowercase-with-hyphens** (`kebab-case`) with a lowercase `.md` extension.

| Rule                    | Scope                            | Example                                               |
| ----------------------- | -------------------------------- | ----------------------------------------------------- |
| No underscores          | everywhere                       | `foreground-tracking.md` not `foreground_tracking.md` |
| No dates in filenames   | general docs                     | use git history for chronology                        |
| UPPER_CASE filenames    | `docs/data/`, project-level docs | `VRLOG_FORMAT.md`, `README.md`, `BACKLOG.md`          |
| Date prefix `YYYYMMDD-` | `docs/maths/proposals/` only     | `20260222-geometry-coherent-tracking.md`              |

### Path patterns

- `docs/data/` — `UPPER_CASE.md` for specification-grade content.
- `docs/plans/` — `<hub>-<area>-<topic>-plan.md` (flat, no subdirectories).
- `docs/maths/proposals/` — `YYYYMMDD-<topic>.md` (date prefix preserved for proposal chronology).
- `<hub>/architecture/` — `<topic>.md`
- `<hub>/operations/` — `<topic>.md`

## Document Structure

Every doc should open with:

1. **`# Title`** — a clear, descriptive heading.
2. **Metadata list** _(optional)_ — bold key-value items as a bullet list. Use only when the doc needs implementation state or cross-references.
3. **Opening paragraph** — one or two sentences explaining what the document covers (the _goal_, _motivation_, or _scope_).
4. **First `##` section** — use a heading that fits the content: `## Goal`, `## Problem`, `## Summary`, `## Objective`, `## Purpose`, etc.

Metadata uses a bullet list of bold key-value pairs:

```
- **Status:** Proposed (February 2026)
- **Layers:** L5 Tracks, L8 Analytics
- **Related:** [Other Doc](other-doc.md)
```

Common metadata keys: `Status`, `Layers`, `Related`, `Backlog`, `Scope`, `Source`, `Version`.

Use `**Status:**` only when the doc tracks implementation progress (plans, architecture specs). Reference docs, maths notes, and READMEs do not need it.

Additional rules:

- `Date:` metadata fields are not allowed — use git history.
- `Version:` is optional.
- Do not use `## Status:` as a section heading — use a metadata list item instead.

Use directory listings for file-level navigation to avoid stale index maintenance.

## Public Documentation Site

Public-facing docs are in [`public_html/`](../public_html/).
