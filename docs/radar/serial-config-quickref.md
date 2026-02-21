# Serial Configuration UI - Quick Reference

Status: Active
Purpose/Summary: serial-config-quickref.

**Full Specification:** See [docs/plans/radar-feature-serial-configuration-ui-plan.md](../plans/radar-feature-serial-configuration-ui-plan.md)

## What This Feature Enables

Users can configure and test radar serial ports through a web interface instead of editing systemd service files.

## Core Requirements

1. **Database Schema** - Store serial port configurations in SQLite
2. **API Endpoints** - REST API for CRUD operations and testing
3. **Serial Testing** - Validate port connectivity and auto-detect baud rate
4. **Device Discovery** - List unassigned serial devices and auto-detect connected sensors
5. **Web UI** - User-friendly interface at `/settings/serial`
6. **Server Integration** - Load configs from DB at startup
7. **Backward Compatible** - CLI flags still work

## Implementation Phases

### Phase 1: Database Foundation (1-2 days)

- Create `radar_serial_config` table
- Migration with default HAT configuration
- Server loads from database
- CLI flag fallback for compatibility

### Phase 2: API Endpoints (3-4 days)

- `/api/serial/configs` - CRUD operations
- `/api/serial/devices` - Enumerate available serial paths (excludes ones already configured)
- `/api/serial/test` - Test connection
- `/api/serial/auto-detect` - Find connected device + baud
- `/api/serial/detect-baud` - Auto-detect baud rate for known port
- Unit and integration tests

### Phase 3: Web UI (4-5 days)

- Configuration list page
- Edit/create modal
- Test connection UI
- Device detection workflow (Detect Device button + filtered port dropdown)
- Auto-detect baud button
- User documentation

### Phase 4: Multi-Sensor (Future)

- Multiple SerialMux instances
- Data tagging with sensor ID
- Multi-sensor analytics

## Key Design Decisions

1. **Database over config files** - Consistent with existing patterns
2. **Application-side sensor models** - No migrations needed for new sensors, CHECK constraint validates
3. **Read-only testing** - Safe and non-disruptive
4. **Selectable baud rates** - Prevents errors, auto-detect as helper
5. **Multiple SerialMux instances** - Future-ready for multi-sensor

## Database Schema (FR1)

```sql
-- Serial port configurations table
CREATE TABLE IF NOT EXISTS radar_serial_config (
       id INTEGER PRIMARY KEY AUTOINCREMENT
     , name TEXT NOT NULL UNIQUE
     , port_path TEXT NOT NULL
     , baud_rate INTEGER NOT NULL DEFAULT 19200
     , data_bits INTEGER NOT NULL DEFAULT 8
     , stop_bits INTEGER NOT NULL DEFAULT 1
     , parity TEXT NOT NULL DEFAULT 'N'
     , enabled INTEGER NOT NULL DEFAULT 1
     , description TEXT
     , sensor_model TEXT NOT NULL DEFAULT 'ops243-a'
     , created_at INTEGER NOT NULL DEFAULT (STRFTIME('%s', 'now'))
     , updated_at INTEGER NOT NULL DEFAULT (STRFTIME('%s', 'now'))
     , CHECK (sensor_model IN ('ops243-a', 'ops243-c'))
     );
```

**Note:** Sensor model information (capabilities, init commands) is stored in application code, not the database. The CHECK constraint validates that only supported sensor models are used.

## API Endpoints (FR2-FR4)

- `GET /api/serial/configs` - List all configurations
- `GET /api/serial/configs/:id` - Get single configuration
- `POST /api/serial/configs` - Create configuration (sensor_model validated against CHECK constraint)
- `PUT /api/serial/configs/:id` - Update configuration
- `DELETE /api/serial/configs/:id` - Delete configuration
- `GET /api/serial/devices` - List available serial devices (skips any port_path already in `radar_serial_config`)
- `GET /api/serial/models` - List available sensor models (from application code)
- `POST /api/serial/test` - Test serial port connection (with auto-correct baud option)
- `POST /api/serial/auto-detect` - Probe all unassigned devices to find connected OPS243 sensors
- `POST /api/serial/detect-baud` - Auto-detect baud rate for specified port

## Testing Algorithm (FR3)

1. Open serial port with specified settings
2. Send safe query command (`??`)
3. Wait for response (5 second timeout)
4. Parse and log response (JSON or non-JSON)
   - Log both JSON and non-JSON responses for diagnostics
   - Non-JSON responses are valid for certain commands (e.g., `I?` returns plain text)
5. Auto-correct baud rate if enabled (query with `I?` command, returns non-JSON response)
6. Return success/failure with diagnostics and captured responses

**Baud Rate Auto-Correction:**
When `auto_correct_baud: true` is set in test request, the system queries the device's current baud rate using the `I?` command (which returns a non-JSON numeric response). If the device reports a different rate than configured (e.g., user manually issued `I1`, `I2`, `I4`, or `I5` commands), the configuration is automatically updated to match the device's actual setting.

**Response Logging:**
All command responses are logged, including both JSON and non-JSON formats. This is essential because:

- Query commands like `I?` return non-JSON text (e.g., "19200")
- Device may not be in JSON mode before initialization
- Raw responses provide diagnostic information for troubleshooting

## Auto-Detection (FR4)

1. `GET /api/serial/devices` enumerates `/dev/tty*` and `/dev/serial*` entries, removes anything already saved in `radar_serial_config`, and returns USB metadata for labeling
2. `POST /api/serial/auto-detect` iterates through the remaining device paths, probing each at common baud rates using safe commands (`??`, `I?`)
3. On success, returns the detected `port_path`, `detected_baud_rate`, inferred `sensor_model`, and raw responses for diagnostics
4. On failure, reports the ports tested and the ones excluded because they are already assigned, along with troubleshooting suggestions
5. Users can still call `POST /api/serial/detect-baud` when only the baud rate needs to be identified for a known port

## File Locations

**Backend:**

- Migration: `internal/db/migrations/20251106_create_radar_serial_config.sql`
- API handlers: `internal/api/serial_config.go`
- Serial testing: `internal/api/serial_test.go`
- Server changes: `cmd/radar/radar.go`

**Frontend:**

- Route: `web/src/routes/settings/serial/+page.svelte`
- API client: `web/src/lib/api.ts` (extend)
- Types: `web/src/lib/types/serial.ts`

**Documentation:**

- User guide: `docs/src/guides/serial-configuration.md`
- API reference: `docs/api/serial-endpoints.md`

## Success Criteria

- Users can configure serial ports via web UI
- Test connection validates settings before saving
- Auto-detect finds correct baud rate
- Backward compatible with existing deployments
- No breaking changes to CLI flags
- Zero data collection downtime during changes

## Security Considerations

- Restrict port paths to `/dev/tty*` and `/dev/serial*`
- Prevent command injection via input validation
- Rate limit test operations
- Mutex protection for concurrent port access
- Clear error messages for permission issues

## Migration Path

**Existing Users:**

1. Update binary (migration runs automatically)
2. Serial config appears in database
3. CLI flags still work (backward compatible)
4. Optionally configure via UI
5. Eventually remove CLI flags from service file

**No Breaking Changes** - Existing deployments continue working
