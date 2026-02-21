# Internal Project Documentation

Status: Active

This directory holds internal engineering documentation.

## Separation Rules

- Implemented/operational behavior lives outside `proposals/`, `future/`, and `plans/`.
- Draft or deferred design work lives under `docs/proposals/`.
- Explicitly not-yet-implemented LiDAR work lives under `lidar/future/`.
- Cross-project execution planning lives under `plans/`.

## Folder Map

- `architecture/`: cross-system architecture notes and audits
- `features/`: implemented feature docs
- `lidar/`: LiDAR docs (`future/` keeps not-yet-implemented LiDAR work)
- `maths/`: implemented maths notes
- `proposals/`: all proposal docs, grouped by intended target directory
- `guides/`: operator/developer guides
- `plans/`: project-wide plans and migration sequences

Use directory listings for file-level navigation to avoid stale index pages.

## Doc Template

```
Status: <required>
Version: <optional>
Purpose/Summary: <required 1-2 lines>
```

- `Date:` metadata field is not allowed; remove it when found.

## Public Documentation Site

Public-facing docs are in [`public_html/`](../public_html/).
