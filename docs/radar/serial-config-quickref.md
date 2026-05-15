# Serial configuration UI - quick reference

- **Status:** Active; most of the operator surface is landed, with rollout work still open
- **Full specification:** See [docs/radar/architecture/serial-configuration-ui.md](architecture/serial-configuration-ui.md)
- **API reference:** See [docs/radar/architecture/serial-configuration-api.md](architecture/serial-configuration-api.md)
- **Implementation plan:** See [docs/plans/serial-configuration-implementation-plan.md](../plans/serial-configuration-implementation-plan.md)

Quick-reference summary of what the current serial configuration work actually brings, what it still does not do, and what remains before the backlog item can be closed without crossing fingers.

## What this feature enables

Users can manage radar serial settings through a local web interface instead of editing service files by hand. The current implementation lands database-backed storage, a real serial test surface, model metadata, available-port listing, and a Sensor Serial Ports section on `/settings` for create, edit, delete, and test workflows.

It also adds a reload manager that can swap the active serial mux at runtime. What it does not yet do is make the main radar startup path load the database config by default, nor does it ship the auto-detect helpers originally described in the design draft.

## Delivered in the current implementation

1. **Database-backed configuration**: `radar_serial_config` now stores serial port definitions in SQLite.
2. **Shipped API surface**: CRUD, sensor-model listing, available-device listing, serial testing, and reload endpoints are present.
3. **Operator UI**: the Sensor Serial Ports section on `/settings` provides list, create, edit, delete, and test flows.
4. **Application-owned sensor models**: OPS243 model metadata lives in Go code, not in a separate lookup table.
5. **Hot-reload building block**: `SerialPortManager` can reload the active serial mux from DB state.
6. **Backward compatibility**: the CLI `--port` path still exists.

## Remaining work

- Wire [cmd/radar/radar.go](../../cmd/radar/radar.go) to load enabled serial configs from the database at startup, with CLI fallback preserved.
- Install the live `SerialPortManager` into the API server so saved settings can be applied to the real runtime path rather than only to tests.
- Decide whether reload is explicit or automatic after save, then reflect that in the UI.
- Add the deferred `auto-detect` and `detect-baud` endpoints plus the matching UI buttons if issue #290 is to be called fully complete.
- Write the operator guide and run the flow on real Pi hardware.

## What the functionality brings

- Operators can inspect and edit serial settings without SSHing in to rewrite service arguments.
- The test endpoint can validate a port and return raw responses before a config is trusted.
- The UI can keep track of multiple saved configurations, even though the runtime currently still behaves like a single-active-sensor system.
- Device listing filters out already-assigned ports so the UI is less likely to invite a foot-gun.
- The reload manager lays the groundwork for live changes without full service restarts.

## Key design decisions

1. **Database over config files** - Consistent with existing patterns
2. **Application-side sensor models** - Application code owns capabilities; the DB enforces only a basic slug shape
3. **Read-only testing** - Safe and non-disruptive
4. **Selectable baud rates** - Prevents errors; auto-detect remains deferred
5. **Multiple SerialMux instances** - Future-ready for multi-sensor

## Database schema

The `radar_serial_config` table stores serial port configurations:

| Column         | Type    | Constraint / Default                                              |
| -------------- | ------- | ----------------------------------------------------------------- |
| `id`           | INTEGER | PRIMARY KEY AUTOINCREMENT                                         |
| `name`         | TEXT    | NOT NULL UNIQUE                                                   |
| `port_path`    | TEXT    | NOT NULL                                                          |
| `baud_rate`    | INTEGER | NOT NULL DEFAULT 19200                                            |
| `data_bits`    | INTEGER | NOT NULL DEFAULT 8                                                |
| `stop_bits`    | INTEGER | NOT NULL DEFAULT 1                                                |
| `parity`       | TEXT    | NOT NULL DEFAULT `'N'`                                            |
| `enabled`      | INTEGER | NOT NULL DEFAULT 1                                                |
| `description`  | TEXT    |                                                                   |
| `sensor_model` | TEXT    | NOT NULL DEFAULT `'ops243-a'`, basic `LIKE 'ops243-%'` validation |
| `created_at`   | INTEGER | NOT NULL DEFAULT `STRFTIME('%s', 'now')`                          |
| `updated_at`   | INTEGER | NOT NULL DEFAULT `STRFTIME('%s', 'now')`                          |

**Note:** Sensor model capabilities and initialisation commands are stored in application code. The database enforces a basic slug shape; the API validates supported models.

## Current API surface

- `GET /api/serial/configs` - List all configurations
- `GET /api/serial/configs/:id` - Get single configuration
- `POST /api/serial/configs` - Create configuration
- `PUT /api/serial/configs/:id` - Update configuration
- `DELETE /api/serial/configs/:id` - Delete configuration
- `GET /api/serial/devices` - List available serial devices (skips any port_path already in `radar_serial_config`)
- `GET /api/serial/models` - List available sensor models (from application code)
- `POST /api/serial/test` - Test serial port connection (with auto-correct baud option)
- `POST /api/serial/reload` - Reload the live serial mux from the enabled database config

Deferred, not shipped in the current implementation:

- `POST /api/serial/auto-detect`
- `POST /api/serial/detect-baud`

## Testing behaviour

1. Open serial port with specified settings
2. Send safe query commands (`??`, `I?`)
3. Wait for response (5 second timeout)
4. Parse and log response (JSON or non-JSON)
   - Log both JSON and non-JSON responses for diagnostics
   - Non-JSON responses are valid for certain commands (e.g., `I?` returns plain text)
5. Auto-correct baud rate in the response if enabled (query with `I?` command, returns non-JSON response)
6. Return success/failure with diagnostics and captured responses

**Baud Rate Auto-Correction:** When `auto_correct_baud: true` is set in the test request, the response reports the device's detected baud rate when it differs from the requested rate. The branch does not yet persist that correction back into `radar_serial_config` automatically.

## File locations

**Backend:**

- Schema snapshot: [internal/db/schema.sql](../../internal/db/schema.sql)
- Migration files: [internal/db/migrations/000038_create_radar_serial_config.up.sql](../../internal/db/migrations/000038_create_radar_serial_config.up.sql), [internal/db/migrations/000038_create_radar_serial_config.down.sql](../../internal/db/migrations/000038_create_radar_serial_config.down.sql)
- DB helpers: [internal/db/serial_config.go](../../internal/db/serial_config.go)
- Sensor model registry: [internal/api/sensor_models.go](../../internal/api/sensor_models.go)
- CRUD handlers: [internal/api/serial_config.go](../../internal/api/serial_config.go)
- Test/device handlers: [internal/api/serial.go](../../internal/api/serial.go)
- Reload manager: [internal/api/serial_reload.go](../../internal/api/serial_reload.go)
- Server routes: [internal/api/server.go](../../internal/api/server.go)

**Frontend:**

- Route: [web/src/routes/settings/+page.svelte](../../web/src/routes/settings/+page.svelte) (Sensor Serial Ports section)
- API client: [web/src/lib/api.ts](../../web/src/lib/api.ts)

## Close-out view

- Done enough to demonstrate the feature: yes.
- Done enough to close issue #290 without qualifiers: not quite.

The branch has earned the right to close most of the checklist. The remaining work is runtime adoption, apply/reload UX, auto-detect helpers, and operator documentation.
