# Serial configuration UI - quick reference

- **Status:** Active; Pi/HAT validation is complete, while discovery enhancements and USB-adapter validation remain open
- **Full specification:** See [docs/radar/architecture/serial-configuration-ui.md](architecture/serial-configuration-ui.md)
- **API reference:** See [docs/radar/architecture/serial-configuration-api.md](architecture/serial-configuration-api.md)
- **Implementation plan:** See [docs/plans/serial-configuration-implementation-plan.md](../plans/serial-configuration-implementation-plan.md)

Quick-reference summary of what the current serial configuration work actually brings, what it still does not do, and what remains before the backlog item can be closed without crossing fingers.

## What this feature enables

Users can manage radar serial settings through a local web interface instead of editing service files by hand. The current implementation lands database-backed storage, a real serial test surface, model metadata, available-port listing, and a Sensor Serial Ports section on `/app/settings` for create, edit, delete, test, and apply workflows.

It also adds a reload manager that can swap the active serial mux at runtime. The main radar startup path now loads the first enabled database config when real radar mode is active, preserving CLI `--port` fallback when no enabled config exists. The auto-detect helpers originally described in the design draft remain deferred.

## Delivered in the current implementation

1. **Database-backed configuration**: `radar_serial_config` now stores serial port definitions in SQLite.
2. **Shipped API surface**: CRUD, sensor-model listing, available-device listing, serial testing, and reload endpoints are present.
3. **Operator UI**: the Sensor Serial Ports section on `/app/settings` provides list, create, edit, delete, test, and apply flows.
4. **Application-owned sensor models**: OPS243 model metadata lives in Go code, not in a separate lookup table.
5. **Runtime adoption**: real radar startup uses the first enabled DB config; CLI `--port` remains the fallback when no enabled config exists.
6. **Hot reload**: `SerialPortManager` reloads the active serial mux from DB state and is exposed through the settings page.
7. **Backward compatibility**: the CLI `--port` path still exists.

## Remaining work

- Add the deferred `auto-detect` and `detect-baud` endpoints plus the matching UI buttons if issue #290 is to be called fully complete.
- Run the configuration and reload path with a USB serial adapter; the Pi/HAT path has been validated.

## What the functionality brings

- Operators can inspect and edit serial settings without SSHing in to rewrite service arguments.
- The test endpoint can validate a port and return raw responses before a config is trusted.
- The UI can keep track of multiple saved configurations, even though the runtime still applies the first enabled configuration as the single active radar sensor.
- Device listing filters out already-assigned ports so the UI is less likely to invite a foot-gun.
- The reload manager applies live changes without full service restarts in real radar mode.

## Operator workflow

1. Open `/app/settings`, then create or edit a serial configuration and mark the intended one enabled.
2. Use **Test Connection** before saving when the port is not already owned by the live radar process.
3. For the active port with matching settings, the test endpoint deliberately returns an ownership confirmation instead of opening the device a second time.
4. Save the configuration, then select **Apply enabled config**. An unchanged configuration is reported as already active without reconnecting the port.
5. A changed configuration on the same active port requires a short close-and-reopen cycle. Verify radar traffic after applying it; the current process retains the existing connection if a different-port reload cannot open its replacement.

## Pi/HAT validation

Remote validation on the deployed `v0.5.1-pre27` Pi image confirmed the single-active radar path:

- The enabled `/dev/ttySC1` `19200 8N1` OPS243-A database row was selected at startup.
- Device discovery found the SC16IS762 HAT ports, including `/dev/ttySC1` through the supplemental scan, and correctly filtered the configured port from the selectable list.
- The active-port test returned the safe ownership response, and a mismatched active-port test returned a non-destructive explanation of the live settings.
- `POST /api/serial/reload` returned the active database snapshot without restarting the service.
- The service stayed active and continued logging radar lines and classified objects after the checks.

The remaining hardware gap is a separate USB serial adapter and an intentional changed-setting reload; neither should be simulated by changing the production radar's active port.

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
| `port_path`    | TEXT    | NOT NULL UNIQUE                                                   |
| `baud_rate`    | INTEGER | NOT NULL DEFAULT 19200                                            |
| `data_bits`    | INTEGER | NOT NULL DEFAULT 8                                                |
| `stop_bits`    | INTEGER | NOT NULL DEFAULT 1                                                |
| `parity`       | TEXT    | NOT NULL DEFAULT `'N'`                                            |
| `enabled`      | INTEGER | NOT NULL DEFAULT 1                                                |
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

The branch has earned the right to close most of the checklist. The remaining work is auto-detect helpers, operator documentation, and real hardware validation.
