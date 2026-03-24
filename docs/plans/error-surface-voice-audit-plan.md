# Error Surface Voice Audit Plan

- **Status:** In progress
- **Layers:** Cross-cutting (Go server, Web frontend, Python tools, Shell scripts)
- **Target:** v0.6.0 — consistent, humane voice across all user-facing messages
- **Canonical:** This document

## Motivation

The project's error messages, warnings, status lines, and user feedback currently
read like they were assembled from a parts catalogue. They do the job — they name
the failure — but they do not sound like a system that has met a person before.

A neighbourhood advocate running `velocity-report migrate status` at 10 pm on a
borrowed laptop deserves messages that are calm, concrete, and human. A developer
watching an HTTP 500 scroll past in the console deserves something more diagnostic
than "Failed to render chart." A Python user told to "Install missing dependencies
above" deserves to know what those dependencies actually do.

This plan catalogues every user-facing message surface across the four language
stacks, groups them into workable batches, and rewrites each to meet the project
voice: clear, precise, morally awake, dry where appropriate, and never pompous.

## Scope Rules

**In scope — rewrite these:**

- CLI output shown to operators (log.Printf, log.Fatalf, fmt.Fprintf to stderr)
- Migration CLI display (fmt.Println/Printf in migrate_cli.go)
- HTTP error responses returned to the browser or API client
- Web UI error, success, warning, info, and empty-state text
- Python print() output shown to users running tools
- Shell echo/printf messages shown during setup, validation, or operation
- Confirmation dialogs and destructive-action warnings

**Out of scope — leave these alone:**

- Internal error wrapping (`fmt.Errorf("failed to scan row: %w", err)`) — these
  are plumbing, not conversation. They wrap cause chains for diagnostics.
- Debug/trace logging inside the LiDAR pipeline (Opsf/Diagf/Tracef streams)
- Test-only messages and mock error strings
- Structured log key-value pairs (the keys stay technical)
- Generated report content (markdown tables in shell scripts)
- Argparse help text (this follows its own conventions)
- Console.log debug lines that will be removed by the structured-logging plan

## Batch Plan

### Batch 1 — Go CLI & Startup Messages

The first surface a person meets. Startup failures, version announcements,
configuration errors, and the migration CLI.

| File | Surface | Count |
|------|---------|-------|
| `cmd/radar/radar.go` | Startup log lines, fatal errors, config validation | ~35 |
| `cmd/deploy/main.go` | Deploy tool errors, deprecation warning, status output | ~30 |
| `internal/db/migrate_cli.go` | Migration CLI display, status, help text, warnings | ~60 |

**Priority:** High — operators see these first and most often.

### Batch 2 — Go Database & Migration Errors

Messages shown when the database is unhappy, missing, dirty, or out of date.
These are the messages a person reads at their most confused.

| File | Surface | Count |
|------|---------|-------|
| `internal/db/db.go` | Schema detection, baseline, sync warnings | ~20 |
| `internal/db/migrate.go` | Migration state errors, dirty state, version mismatch | ~15 |
| `internal/db/db_radar.go` | Input validation errors (API boundary) | ~5 |

**Priority:** High — database trouble is the most common support case.

### Batch 3 — Go HTTP API Errors

Error responses returned through the web interface or API calls. These appear in
the browser console, in toast messages, or in raw JSON responses.

| File | Surface | Count |
|------|---------|-------|
| `internal/lidar/server/playback_handlers.go` | Playback control errors | ~35 |
| `internal/lidar/server/status.go` | Grid/sensor status errors | ~20 |
| `internal/lidar/server/run_track_api.go` | Track labelling, run management errors | ~30 |
| `internal/lidar/server/echarts_handlers.go` | Chart rendering errors | ~8 |
| `internal/lidar/server/datasource_handlers.go` | Data source switching errors | ~7 |
| `internal/serialmux/serialmux.go` | Serial command interface errors | ~6 |
| `internal/db/db_admin.go` | Database admin endpoint errors | ~6 |

**Priority:** Medium — developers and power users see these.

### Batch 4 — Web Frontend User-Facing Messages

The UI text that neighbourhood advocates, site operators, and municipal readers
actually see when something goes wrong or right.

| File | Surface | Count |
|------|---------|-------|
| `web/src/lib/api.ts` | API fetch error throws (~50 "Failed to..." messages) | ~50 |
| `web/src/routes/(constrained)/settings/+page.svelte` | Settings feedback | ~15 |
| `web/src/routes/(constrained)/reports/+page.svelte` | Report generation feedback | ~9 |
| `web/src/routes/(constrained)/site/+page.svelte` | Site management errors | ~4 |
| `web/src/routes/(constrained)/site/[id]/+page.svelte` | Site detail errors | ~6 |
| `web/src/routes/+page.svelte` | Dashboard errors | ~4 |
| `web/src/routes/lidar/runs/+page.svelte` | Run management errors & confirmations | ~6 |
| `web/src/routes/lidar/replay-cases/+page.svelte` | Replay case feedback | ~8 |
| `web/src/routes/lidar/sweeps/+page.svelte` | Sweep control feedback | ~15 |
| `web/src/routes/lidar/tracks/+page.svelte` | Track history errors | ~8 |
| `web/src/lib/components/MapEditorInteractive.svelte` | Map/address search errors | ~5 |
| `web/src/lib/stores/capabilities.ts` | Capabilities polling warning | ~1 |

**Priority:** High — this is the primary non-technical user surface.

### Batch 5 — Python Tool Messages

PDF report generation, dependency checking, data alignment tools. These are
used by people setting up the system or generating reports.

| File | Surface | Count |
|------|---------|-------|
| `tools/pdf-generator/pdf_generator/core/dependency_checker.py` | Dependency status display | ~15 |
| `tools/pdf-generator/pdf_generator/core/config_manager.py` | Config validation errors | ~2 |
| `tools/pdf-generator/pdf_generator/core/date_parser.py` | Date parsing errors | ~3 |
| `tools/pdf-generator/pdf_generator/core/chart_builder.py` | Import errors | ~2 |
| `tools/pdf-generator/pdf_generator/core/map_utils.py` | Map utility errors | ~3 |
| `tools/pdf-generator/pdf_generator/cli/main.py` | CLI import fallbacks | ~6 |
| `data/align/get_telraam.py` | Telraam API errors, progress, warnings | ~35 |
| `data/align/get_unifi.py` | UniFi integration errors, progress | ~31 |
| `tools/grid-heatmap/plot_grid_heatmap.py` | Heatmap tool messages | ~30 |

**Priority:** Medium — setup and report generation users.

### Batch 6 — Shell Script Messages

Setup scripts, validation scripts, development tools. The messages people see
when building, configuring, or checking the system.

| File | Surface | Count |
|------|---------|-------|
| `scripts/dev-setup.sh` | Development environment setup | ~25 |
| `scripts/validate-lfs-files.sh` | LFS validation status | ~40 |
| `scripts/sqlite-erd/graph.sh` | ERD generation errors | ~11 |
| `scripts/create-dmg.sh` | macOS DMG creation errors | ~5 |
| `scripts/sync-schema.sh` | Schema sync errors/warnings | ~10 |
| `scripts/mov-to-mp4.sh` | Video conversion messages | ~10 |
| `scripts/format-sql.sh` | SQL formatting errors | ~5 |

**Priority:** Low — developer-facing, but still worth doing.

## Voice Guidelines for This Work

Each rewritten message should:

1. **Name the problem** — what actually went wrong, in concrete terms
2. **Name the consequence** — what does this mean for the person
3. **Name the next step** — what should they do, if anything
4. **Stay calm** — no theatrical drama at the precise moment someone needs help
5. **Stay short** — error messages are not essays
6. **Sound human** — but not like a chatbot wearing a party hat

Dry wit is welcome in success messages, status displays, and informational lines.
It is not welcome in messages about data loss, privacy, or things that are
genuinely broken.

## Completion Criteria

- [ ] Batch 1: Go CLI & startup messages rewritten
- [ ] Batch 2: Go database & migration errors rewritten
- [ ] Batch 3: Go HTTP API errors rewritten
- [ ] Batch 4: Web frontend messages rewritten
- [ ] Batch 5: Python tool messages rewritten
- [ ] Batch 6: Shell script messages rewritten
- [ ] All tests pass after each batch
- [ ] No existing error-handling logic changed (only string content)
