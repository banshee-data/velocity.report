```
      __     _
     / /  __| |    ___     __      ___
    / /  / _  |   / _ \   / _|    (_-<
  _/_/_  \__,_|   \___/   \__|    /__/
_|"""""|_|"""""|_|"""""|_|"""""|_|"""""|
"`-o-o-'"`-o-o-'"`-o-o-'"`-o-o-'"`-o-o-'
```

# /docs

Status: Active
Purpose: Documentation structure and conventions

## /docs root structure

- `docs/lidar/` # LiDAR pipeline and sensor
- `docs/radar/` # Radar sensor processing
- `docs/ui/` # Web, mac, PDF clients
- `docs/maths/` # Algorithms and signal theory
- `docs/plans/` # Implementation plans, roadmap
- `docs/COVERAGE.md` # Test coverage tracking
- `docs/DEVLOG.md` # Chronological development notes

## Scope Rules

- Capability docs live under the owning root hub (`lidar`, `radar`).
- UI/client surface docs for web/mac/pdf live under `docs/ui/`.
- `docs/plans/` contains implementation plans and deferred roadmap work (including previously proposal-scoped non-maths docs).
- Maths stays separate under `docs/maths/`.
- Maths proposals live only in `docs/maths/proposals/`.

## Naming Conventions

- Plans (flat): `<hub>-<area>-<topic>-plan.md` in `docs/plans/`
- Maths proposals: domain-meaningful names in `docs/maths/proposals/`

## Metadata Conventions

Required in all docs:

- `Status:` required
- `Purpose:` or `Summary:` required (one of, not both)
- `Version:` optional

Additional rules:

- `Date:` metadata fields are not allowed

> Template/Style Quick Box
>
> - `Status:` required
> - `Purpose:` required
> - `Version:` optional
> - `Date:` not allowed (remove if present)

Use directory listings for file-level navigation to avoid stale index maintenance.

## Public Documentation Site

Public-facing docs are in [`public_html/`](../public_html/).
