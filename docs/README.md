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
- `/docs/VISION.md` # Product vision and backlog alignment guide
- `/docs/ROADMAP.md` # Product roadmap and milestones (v0.5 → v2.0)
- `/docs/COVERAGE.md` # Test coverage tracking
- `/docs/DEVLOG.md` # Chronological development notes

## Scope Rules

- Capability docs live under the owning root hub (`lidar`, `radar`).
  - Architecture and design specifications live under `<hub>/architecture/`.
  - Operational guides and implementation status live under `<hub>/operations/`.
- UI/client surface docs for web/mac/pdf live under `docs/ui/`.
- `docs/plans/` contains forward-looking implementation plans and deferred roadmap work only. Completed architecture specs and implementation records belong in their hub folder.
- Maths stays separate under `docs/maths/`.
- Maths proposals live only in `docs/maths/proposals/`.

## Naming Conventions

- Plans (flat): `<hub>-<area>-<topic>-plan.md` in `docs/plans/`
- Maths proposals: `<yyymmdd>-<topic>.md` in `docs/maths/proposals/`

## Document Structure

Every doc should open with:

1. **`# Title`** — a clear, descriptive heading.
2. **Opening paragraph** — one or two sentences explaining what the document covers (the _goal_, _motivation_, or _scope_). This replaces the old `Purpose:` metadata line.
3. **First `##` section** — use a heading that fits the content: `## Goal`, `## Problem`, `## Summary`, `## Objective`, `## Purpose`, etc.

Optional bold metadata may follow the title for docs that need implementation state:

```
**Status:** Proposed (February 2026)
**Related:** [Other Doc](other-doc.md)
```

Use `**Status:**` only when the doc tracks implementation progress (plans, architecture specs). Reference docs, maths notes, and READMEs do not need it.

Additional rules:

- `Date:` metadata fields are not allowed — use git history.
- `Version:` is optional.

Use directory listings for file-level navigation to avoid stale index maintenance.

## Public Documentation Site

Public-facing docs are in [`public_html/`](../public_html/).
