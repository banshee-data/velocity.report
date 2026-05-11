# Plan: Velocity Report v2 UI 🦋 Project Butterfly Net

- **Design sketch:** [`20260511-Velocity_Report_Butterfly_Net.html`](../ui/design/20260511-Velocity_Report_Butterfly_Net.html) (open in browser — standalone, no server needed)

## Context

The current web UI conflates two audiences: city-hall-facing traffic reports and pipeline-debugging dashboards. The report generation workflow duplicates controls across the Dashboard and Reports pages, config periods are confusingly split between read-only (Reports) and editable (Site editor), and there is no concept of discrete deployment sessions or sensor utilisation. The design introduces a **Concept B workspace layout** for Reports, a gamified coverage Dashboard, a new Sensor asset page, and an evolved Sites list — all built around the idea that the radar is a reusable asset deployed to recording sessions, each with its own locked calibration angle.

---

## What the Design Introduces

Five navigation sections (Dashboard · Sensor · Sites · Reports · Settings):

| Page      | Status    | Core change                                                                                                                                     |
| --------- | --------- | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| Dashboard | Redesign  | Coverage gamification: sample buckets, weekly consistency dots, time-of-day heatmap, earned badges                                              |
| Sensor    | **New**   | Sensor as asset: deployment timeline coloured by site, blown-period flags, utilisation bar, periods table                                       |
| Sites     | Redesign  | List with expandable rows; per-site bucket progress; before/after callout when angles differ across periods                                     |
| Reports   | Redesign  | Concept B workspace — left rail (site + discrete period picker + cosine info + generate), right panel (chart preview + histogram) + History tab |
| Settings  | Unchanged | Placeholder                                                                                                                                     |

---

## Key Architectural Decisions

### D1 · Recording periods vs. site_config_periods ★ high impact

The design's "recording period" (a discrete deployment session with its own angle, mount type, blown flag, and cached metrics) overlaps with the existing `site_config_periods` table (cosine correction configuration). They are **not** the same concept:

|                    | `site_config_periods`                    | Design's `recording_period`        |
| ------------------ | ---------------------------------------- | ---------------------------------- |
| Purpose            | Cosine correction calibration versioning | Discrete sensor deployment session |
| Angle              | `cosine_error_angle`                     | sensor angle per session           |
| Blown flag         | ✗                                        | ✓                                  |
| Mount type         | ✗ (on site)                              | ✓ per-period                       |
| Transit counts     | ✗ (computed on read)                     | cached (p50, p98)                  |
| Used by report gen | ✓ (cosine correction)                    | ✓ (scopes data range)              |

**Option A — Extend `site_config_periods`:** Add `mount_type`, `blown`, `blown_reason` columns; derive recording sessions from existing config periods. Keeps backward compat; avoids data migration. Risk: mixes two semantically distinct concepts.

**Option B — New `recording_periods` table:** Separate table linked to `site_id`. Cosine correction is derived from the recording period's angle rather than from `site_config_periods`. Requires migrating existing data. Clean model; higher migration cost.

**Option C — Keep both:** `site_config_periods` stays for cosine correction; new `recording_periods` tracks deployment sessions. No migration needed; some redundancy.

**Recommended: Option B.** Recording periods own the angle, so cosine correction should be derived from them, eliminating `site_config_periods` as a separate concept.

---

### D2 · Sensor as singleton or multi-device

The design shows one global sensor. Does the system need to support multiple physical sensors now or in the foreseeable future?

- **Singleton:** `GET /api/sensor` returns one device record, no device-scoping on other endpoints.
- **Multi-sensor:** `sensor_id` FK on recording periods; endpoints gain `?sensor_id=` param.

**Recommended:** Singleton table (`sensor_assets`, single row) with `sensor_id` FK on `recording_periods` for future extensibility without adding query complexity now.

---

### D3 · Badge storage vs. compute-only

- **Computed at read time:** Always current; ~10 lightweight aggregation queries per dashboard load.
- **Persisted:** `badge_awards` table written when threshold first crossed. Enables `earned_date` display.

**Recommended:** Compute badge earned-state at read time; persist `earned_at` in `badge_awards` so the dashboard can show "Earned 2025-09-08" without re-scanning all history.

---

### D4 · Mount type: site or recording period

Current schema: `mount_type` on the `site` row. Design: per recording period (mobile deployments vary by parking spot and angle).

**Recommended:** `mount_type` authoritative on recording periods. Keep it on the site as a default hint for new period creation.

---

### D5 · Transit counts per recording period — computed or cached

Sites and Dashboard show `totalTransits` per period. Computing this at read time requires a date-range aggregate over `radar_data_transits`.

**Recommended:** Cached columns (`cached_transits`, `cached_p50`, `cached_p98`) on recording_periods, refreshed by the transit worker on demand. Avoids repeated aggregation on each dashboard load.

---

### D6 · Gamification scope — Phase 1 or Phase 2

The gamification layer (badges, bucket progress, weekly consistency, heatmap) adds ~6 backend + 5 frontend days on top of the structural UI work.

**Recommended:** Phase 1 = structural layout changes + recording periods model. Phase 2 = gamification (buckets, badges, heatmap, consistency dots). The structural changes are independently valuable for city-hall workflows.

---

### D7 · Reports workspace — period-scoped vs. date-scoped

Current reports accept arbitrary date ranges. Concept B maps to discrete recording periods.

**Recommended:** Add `recording_period_id` as an optional report parameter. When supplied, backend resolves date range and cosine angle from the period row. Backward compat with date-range mode preserved.

---

### D8 · Blown flag write access

The Sensor page lets users flag a period as "blown". No auth gate today.

**Recommended:** No gate; consistent with existing site delete and config period edit behaviour.

---

## Database Migrations

All migrations go in `internal/db/migrations/` using the existing numbered scheme.

### sensor_assets

Single-row table representing the physical radar device. Fields:

- Identity: model name, serial number (from OPS243-A identity response)
- Firmware: version string — updated at runtime when device reports a new version
- Capability flag: `emits_objects` boolean — controls which data sources appear in the report UI (not all firmware versions emit the objects feed)
- Timestamps: acquired_at (when device was purchased), created_at, updated_at

One row seeded by migration from `config.json`. Firmware version written by Go server on startup.

### recording_periods

Replaces `site_config_periods` as the single source of truth for "when was the sensor deployed, where, and with what config." Fields:

- `site_id` FK — which site this session belongs to
- `sensor_id` FK — which physical device (nullable for future multi-sensor support)
- `start_date` / `end_date` (YYYY-MM-DD, end nullable = open-ended) — day-granularity preferred over unix floats for human-facing sessions
- `mount_type` — `permanent` | `mobile`; authoritative per-period rather than per-site
- `cosine_angle` — the angle the sensor was physically set to; drives cosine correction at report-generation time
- `blown` boolean + `blown_reason` text — data-quality flag; blown periods excluded from transit totals and report queries
- `notes` — free-text surveyor notes (carried over from site_config_periods)
- Cached speed metrics: `cached_transits`, `cached_p50`, `cached_p98` — denormalised from radar_data_transits; refreshed by transit worker
- Timestamps: created_at, updated_at

Index on `(site_id)` and `(start_date, end_date)`.

**Data migration:** Port existing `site_config_periods` rows — convert `effective_start_unix` → `start_date`, map `cosine_error_angle` → `cosine_angle`, carry `notes`. Populate cached metrics via a one-off recalculation pass.

### badge_awards (Phase 2)

Minimal table: `badge_id` (text slug, unique), `earned_at` (unix float), `created_at`. Badge definitions (name, icon, threshold rule) live in Go code; the table only records when each was first earned.

---

## New Go API Endpoints

All handlers in `internal/api/`. Follow patterns from `server_sites.go`, `server_reports.go`.

### Sensor

| Method | Path          | Description                                                       |
| ------ | ------------- | ----------------------------------------------------------------- |
| `GET`  | `/api/sensor` | Return sensor_assets row (firmware, model, serial, emits_objects) |
| `PUT`  | `/api/sensor` | Update sensor metadata                                            |

Go struct: `internal/db/sensor.go`. Handler: `internal/api/server_sensor.go`.

### Recording Periods

| Method   | Path                                      | Description                                         |
| -------- | ----------------------------------------- | --------------------------------------------------- |
| `GET`    | `/api/recording_periods`                  | List all; `?site_id=` filter; `?include_blown=true` |
| `POST`   | `/api/recording_periods`                  | Create; validates date non-overlap per site         |
| `PUT`    | `/api/recording_periods/{id}`             | Update (blown flag, dates, angle, notes, mount)     |
| `DELETE` | `/api/recording_periods/{id}`             | Delete                                              |
| `POST`   | `/api/recording_periods/{id}/recalculate` | Refresh cached_transits/p50/p98                     |

Overlap validation enforced in Go, not DB triggers (simpler than the existing site_config_periods trigger approach).

Handler: `internal/api/server_recording_periods.go`. Model: `internal/db/recording_period.go`.

### Gamification (Phase 2)

| Method | Path                       | Description                                                     |
| ------ | -------------------------- | --------------------------------------------------------------- |
| `GET`  | `/api/gamification/stats`  | Sample bucket progress, weekly consistency, time-of-day heatmap |
| `GET`  | `/api/gamification/badges` | All badges with earned state + earned_at timestamps             |

Stats response covers: global transit/day totals, blown-period count, active-site count; sample-bucket progress; 12-week consistency array; 7×24 time-of-day coverage grid; per-site bucket summaries.

Handler: `internal/api/server_gamification.go`.

### Reports — period-scoped generation

Extend `POST /api/generate_report` to accept optional `recording_period_id` / `compare_period_id`. When supplied, backend resolves date range and cosine angle from the period row. Raw `start_date`/`end_date` mode preserved for backward compat.

Changes: `internal/api/server_reports_generate.go`, `internal/report/`.

---

## Frontend Changes

Stack: Svelte 5 + TypeScript + Vite. All files under `web/src/`.

### API types + fetch functions (`lib/api.ts`)

New types: `SensorAsset`, `RecordingPeriod`, `GamificationStats`, `Badge`. Add `recording_period_id` / `compare_period_id` to `ReportRequest`. New fetch wrappers: `getSensor`, `getRecordingPeriods`, `createRecordingPeriod`, `updateRecordingPeriod`, `deleteRecordingPeriod`, `getGamificationStats`, `getBadges`.

### Dashboard (`routes/+page.svelte`) — redesign

Remove: timeseries chart, date range picker, group/source selectors.

Phase 1 adds: stat cards (total transits, recording days, active sites, blown periods).

Phase 2 adds: sample buckets widget, weekly consistency widget (12 week dots + streak + time-of-day heatmap), badges grid.

Data source: `GET /api/gamification/stats` + `GET /api/gamification/badges` (Phase 2).

### Sensor page — new (`routes/(constrained)/sensor/+page.svelte`)

New SvelteKit route. "Sensor" nav item added to `+layout.svelte` between Dashboard and Sites.

Sections:

1. Header: model name, firmware badge, objects-feed badge
2. Stat row: total transits, utilisation %, blown periods, unique sites
3. Deployment timeline: colour-coded site segments (blown = hatched red), clickable to open flag editor
4. Inline flag editor: blown checkbox + reason input + save
5. Utilisation bar
6. Recording periods table: newest-first, rows clickable to open flag editor

Data: `GET /api/sensor` + `GET /api/recording_periods`.

### Sites (`routes/(constrained)/site/+page.svelte`) — redesign

Replace flat table with expandable card list:

- **Summary row:** colour bar, name, active/inactive badge, mount badge, before/after callout (auto-detected when site has periods with different angles), blown count, transit total, bucket mini-bar
- **Expanded row:** recording periods table (dates, mount, angle, transits, p98, notes); before/after warning; "Generate report →" and "Edit site" buttons

Cosine correction history in site editor (`[id]/+page.svelte`) migrated from `site_config_periods` CRUD to `recording_periods` CRUD.

### Reports (`routes/(constrained)/reports/+page.svelte`) — Concept B workspace

Two-panel layout:

**Left rail (320 px, scrollable):**

- Site selector dropdown
- Read-only site callout (speed limit, mount type, surveyor, period count) with "Edit →" link to site editor
- Primary period selector (options: "All valid periods" + each period with dates + angle + transit count)
- Compare period selector + before/after warning when angles differ
- Object type / grouping selectors
- Pinned generate button + download buttons when ready

**Right panel (flex, scrollable):**

- Primary time-series chart (chart preview via existing `/api/charts/timeseries`)
- Comparison chart (if period selected)
- Speed distribution histogram

**History tab:** report list filterable by site; PDF + ZIP download links.

Period selection sends `recording_period_id` to report API; date range derived server-side.

---

## Level of Effort (senior engineer days)

### Phase 1 — Structural + Recording Periods

| Area                  | Task                                            | Days          |
| --------------------- | ----------------------------------------------- | ------------- |
| **Backend**           | Migrations: recording_periods + sensor_assets   | 1.0           |
| **Backend**           | Data migration: port site_config_periods        | 0.5           |
| **Backend**           | Recording periods CRUD API + overlap validation | 2.0           |
| **Backend**           | Sensor API                                      | 0.5           |
| **Backend**           | Reports: recording_period_id param              | 1.0           |
| **Backend**           | Transit worker: cached metric refresh           | 1.0           |
| **Backend subtotal**  |                                                 | **6.0**       |
| **Frontend**          | API types + fetch functions                     | 0.5           |
| **Frontend**          | Sensor page (new route)                         | 2.5           |
| **Frontend**          | Sites redesign (expandable cards)               | 2.0           |
| **Frontend**          | Reports workspace (Concept B)                   | 3.0           |
| **Frontend**          | Dashboard: basic stat redesign                  | 1.0           |
| **Frontend**          | Nav + routing                                   | 0.5           |
| **Frontend subtotal** |                                                 | **9.5**       |
| **Phase 1 total**     |                                                 | **15.5 days** |

### Phase 2 — Gamification

| Area                  | Task                                              | Days         |
| --------------------- | ------------------------------------------------- | ------------ |
| **Backend**           | Migration: badge_awards                           | 0.5          |
| **Backend**           | Gamification stats API (buckets, streak, heatmap) | 2.0          |
| **Backend**           | Badge computation + award persistence             | 1.5          |
| **Backend subtotal**  |                                                   | **4.0**      |
| **Frontend**          | Dashboard gamification widgets                    | 3.5          |
| **Frontend**          | Badge definitions + icons                         | 0.5          |
| **Frontend subtotal** |                                                   | **4.0**      |
| **Phase 2 total**     |                                                   | **8.0 days** |

**Combined total: ~23.5 senior engineer days (~5 weeks)**

---

## Implementation Order (Phase 1)

1. DB migrations (sensor_assets, recording_periods)
2. Data migration (port existing site_config_periods)
3. Go DB model + CRUD functions (`internal/db/recording_period.go`)
4. Go API handlers (`server_recording_periods.go`, `server_sensor.go`)
5. Reports generate: recording_period_id param
6. Transit worker: cached_transits refresh
7. Frontend API types + fetch wrappers
8. Nav update + Sensor page (most isolated, good smoke test)
9. Sites redesign
10. Reports Concept B workspace
11. Dashboard stat redesign (gamification deferred to Phase 2)

---

## Files to Create / Modify

### New files

- `internal/db/migrations/000NNN_create_sensor_assets.sql`
- `internal/db/migrations/000NNN_create_recording_periods.sql`
- `internal/db/sensor.go`
- `internal/db/recording_period.go`
- `internal/api/server_sensor.go`
- `internal/api/server_recording_periods.go`
- `web/src/routes/(constrained)/sensor/+page.svelte`
- _(Phase 2)_ `internal/db/migrations/000NNN_create_badge_awards.sql`
- _(Phase 2)_ `internal/api/server_gamification.go`

### Modified files

- `internal/api/server.go` — register new routes
- `internal/api/server_reports_generate.go` — recording_period_id param
- `internal/report/` — cosine angle from recording_period
- `web/src/lib/api.ts` — new types + fetch functions
- `web/src/routes/+layout.svelte` — Sensor nav item
- `web/src/routes/+page.svelte` — Dashboard stat redesign
- `web/src/routes/(constrained)/site/+page.svelte` — Sites expandable list
- `web/src/routes/(constrained)/site/[id]/+page.svelte` — recording_periods CRUD
- `web/src/routes/(constrained)/reports/+page.svelte` — Concept B workspace

---

## Verification

1. `make test-go` — existing tests pass; add unit tests for recording_period overlap validation and cached metric refresh
2. `make build-radar-local` — Go server compiles
3. `make dev-go` + `make dev-web` — Sensor page renders, Sites list expands, Reports workspace shows period picker
4. Flag a recording period as blown via Sensor page → verify excluded from transit totals on Dashboard and Sites
5. Select a period in Reports workspace → verify generate call uses `recording_period_id` and chart preview updates
6. Generate a report with a locked cosine angle → verify PDF uses that angle
