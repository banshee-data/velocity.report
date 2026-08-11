# Serial configuration implementation plan (v0.5.1)

- **Status:** Active
- **Layers:** Go server, API, database, web frontend, documentation
- **Target:** v0.5.1; most of the operator-facing surface is now landed, so the remaining work is rollout and completion rather than invention
- **Canonical:** [serial-config-quickref.md](../radar/serial-config-quickref.md) <!-- link-ignore -->

## Motivation

Radar serial setup currently asks too much of operators. Editing service files by hand is fine if you already know where the trapdoors are, but it is a poor way to validate baud rate, port choice, or whether the device at the other end is awake and willing.

This work changes the centre of gravity. Serial configuration becomes database-backed, visible in the web UI, and testable through local API calls. The running radar process now adopts saved settings on startup in real radar mode and exposes an explicit apply path for live reloads; the remaining work is discovery polish, operator documentation, and hardware validation.

## Current state

- The schema snapshot now includes `radar_serial_config` in [internal/db/schema.sql](../../internal/db/schema.sql), with migration files at [internal/db/migrations/000038_create_radar_serial_config.up.sql](../../internal/db/migrations/000038_create_radar_serial_config.up.sql) and [internal/db/migrations/000038_create_radar_serial_config.down.sql](../../internal/db/migrations/000038_create_radar_serial_config.down.sql).
- CRUD helpers for serial configurations exist in [internal/db/serial_config.go](../../internal/db/serial_config.go).
- The API server exposes `GET/POST/PUT/DELETE` config routes plus models, devices, test, and reload routes in [internal/api/server.go](../../internal/api/server.go), with handlers in [internal/api/serial_config.go](../../internal/api/serial_config.go), [internal/api/serial.go](../../internal/api/serial.go), and [internal/api/serial_reload.go](../../internal/api/serial_reload.go).
- Sensor model metadata is application-owned in [internal/api/sensor_models.go](../../internal/api/sensor_models.go).
- The web UI ships serial configuration inside the Sensor Serial Ports section of [web/src/routes/settings/+page.svelte](../../web/src/routes/settings/+page.svelte), backed by helpers in [web/src/lib/api.ts](../../web/src/lib/api.ts).
- The current codebase includes test coverage for DB access, API handlers, and reload behaviour in [internal/db/serial_config_test.go](../../internal/db/serial_config_test.go), [internal/api/serial_config_test.go](../../internal/api/serial_config_test.go), and [internal/api/serial_reload_test.go](../../internal/api/serial_reload_test.go).
- [internal/cmd/server/radar.go](../../internal/cmd/server/radar.go) loads the first enabled DB serial config for real radar startup, with CLI `--port` fallback preserved when no enabled config exists.
- The settings page exposes an explicit apply action that calls `POST /api/serial/reload`.
- Auto-detect endpoints and UI affordances described in the original design are not present in the current implementation.

## Findings

| Area             | Current state                                                                                                                              | Severity | Release view                                     |
| ---------------- | ------------------------------------------------------------------------------------------------------------------------------------------ | -------- | ------------------------------------------------ |
| Storage          | Schema, migration, and DB helpers are landed                                                                                               | Low      | Done for this release                            |
| API core         | CRUD, model listing, device listing, test, and reload endpoints are landed                                                                 | Low      | Done for this release                            |
| Runtime adoption | `internal/cmd/server/radar.go` uses DB-backed config first in real radar mode, with CLI fallback when no enabled config exists             | Low      | Needs hardware smoke validation                  |
| UX completion    | The Sensor Serial Ports section on `/settings` supports list/create/edit/delete/test/apply, but not device auto-detect or baud-only detect | Medium   | Discovery polish can defer                       |
| Documentation    | Design docs still read like a proposal and over-promise unshipped endpoints                                                                | Medium   | Fix now so the release notes do not lie politely |

## Design / approach

Treat this as a completion pass, not a second design exercise.

1. Keep the delivered branch scope as the baseline. The database model, API handlers, reload manager, and settings page are already the owning abstractions.
2. Keep runtime adoption at the startup boundary in [internal/cmd/server/radar.go](../../internal/cmd/server/radar.go) rather than inventing more API.
3. Make the docs separate shipped behaviour from planned behaviour. A design document is allowed to dream a little, but it should not claim to ship `POST /api/serial/auto-detect` when the code has never met it.
4. Preserve CLI fallback until the DB-backed startup path is proven on real Pi hardware.

## Scope

### Item 1: close the documentation gap

**Summary:** Update the radar serial docs so they describe what the current implementation already delivers and what remains.

**Steps:**

1. Mark implemented surfaces as delivered.
2. Move unshipped items into explicit remaining work.
3. Update backlog wording so it tracks only the unfinished rollout.

**Milestone:** v0.5.1

### Item 2: wire DB-backed startup

**Summary:** Make the running radar service load the active serial configuration from the database and install the reload manager on the API server.

**Steps:**

1. Read enabled serial configs at startup.
2. Construct the live mux from DB settings when available, with CLI fallback preserved.
3. Pass the manager into the API server so `/api/serial/reload` operates on the real runtime path.

**Milestone:** v0.5.1

### Item 3: finish the operator workflow

**Summary:** Fill the remaining usability holes so saving a config leads to a comprehensible live outcome.

**Steps:**

1. Decide whether reload is explicit or automatic after save.
2. Add the chosen affordance to the Sensor Serial Ports section on `/settings`.
3. Validate the flow on real hardware.

**Milestone:** v0.5.1

### Item 4: defer the discovery extras cleanly

**Summary:** Keep auto-detect and multi-sensor work visible without pretending it landed here.

**Steps:**

1. Track `auto-detect` / `detect-baud` as remaining work.
2. Leave multi-sensor tagging and analytics in future scope.

**Milestone:** v0.5.x follow-up

## Dependencies

- Raspberry Pi or equivalent hardware validation for the real serial path
- Real hardware confirmation that explicit operator-invoked reload is safe enough for the rollout
- Existing migration and DB bootstrap flow remaining unchanged

## Risks

| Risk                                                                    | Likelihood | Impact | Mitigation                                                     |
| ----------------------------------------------------------------------- | ---------- | ------ | -------------------------------------------------------------- |
| Docs mark the feature complete before hardware startup/reload is proven | Medium     | High   | Keep hardware validation in Outstanding until verified         |
| Saving config is mistaken for immediate live apply                      | Medium     | Medium | Keep the explicit apply button and success/error toast visible |
| Auto-detect remains described as shipped                                | Medium     | Medium | Move it into Outstanding/Deferred in every serial-config doc   |
| Real hardware differs from mocked tests                                 | Medium     | Medium | Require one Pi validation pass before closing the rollout item |

## Checklist

### Complete

- [x] Added `radar_serial_config` schema and migration files
- [x] Added DB CRUD helpers for serial configurations
- [x] Added sensor model registry in application code
- [x] Added `/api/serial/configs`, `/api/serial/models`, `/api/serial/devices`, `/api/serial/test`, and `/api/serial/reload`
- [x] Added `SerialPortManager` hot-reload machinery and tests
- [x] Added the Sensor Serial Ports UI on `/settings` for list, create, edit, delete, and test workflows
- [x] Wired DB-backed serial startup and real `SerialPortManager` installation in [internal/cmd/server/radar.go](../../internal/cmd/server/radar.go)
- [x] Added an explicit apply/reload path in the settings UI
- [x] Added branch-level documentation and devlog entries describing the delivered work

### Outstanding

- [ ] Add `POST /api/serial/auto-detect` and `POST /api/serial/detect-baud`, plus UI actions that use them (`M` effort)
- [ ] Write the operator guide and troubleshooting doc for serial configuration (`S` effort)
- [ ] Run a real-hardware validation pass on Pi/HAT and one USB serial adapter (`S` effort)

### Deferred

- [ ] Multi-sensor runtime adoption, data tagging, and analytics surfaces; tracked as future scope in [serial-configuration-ui.md](../radar/architecture/serial-configuration-ui.md) <!-- link-ignore -->

### Accepted residuals (no action planned)

- [ ] CLI flag fallback remains during the rollout window because removing the old path before DB-backed startup is proven would be brisk, memorable, and unhelpful
