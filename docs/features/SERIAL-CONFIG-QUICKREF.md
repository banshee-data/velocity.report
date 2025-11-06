# Serial Configuration UI - Quick Reference

**Full Specification:** See [docs/features/serial-configuration-ui.md](./serial-configuration-ui.md)

## What This Feature Enables

Users can configure and test radar serial ports through a web interface instead of editing systemd service files.

## Core Requirements

1. **Database Schema** - Store serial port configurations in SQLite
2. **API Endpoints** - REST API for CRUD operations and testing
3. **Serial Testing** - Validate port connectivity and auto-detect baud rate
4. **Web UI** - User-friendly interface at `/settings/serial`
5. **Server Integration** - Load configs from DB at startup
6. **Backward Compatible** - CLI flags still work

## Implementation Phases

### Phase 1: Database Foundation (1-2 days)
- Create `radar_serial_config` table
- Migration with default HAT configuration
- Server loads from database
- CLI flag fallback for compatibility

### Phase 2: API Endpoints (3-4 days)
- `/api/serial/configs` - CRUD operations
- `/api/serial/test` - Test connection
- `/api/serial/detect-baud` - Auto-detect baud rate
- Unit and integration tests

### Phase 3: Web UI (4-5 days)
- Configuration list page
- Edit/create modal
- Test connection UI
- Auto-detect button
- User documentation

### Phase 4: Multi-Sensor (Future)
- Multiple SerialMux instances
- Data tagging with sensor ID
- Multi-sensor analytics

## Key Design Decisions

1. **Database over config files** - Consistent with existing patterns
2. **Read-only testing** - Safe and non-disruptive
3. **Selectable baud rates** - Prevents errors, auto-detect as helper
4. **Multiple SerialMux instances** - Future-ready for multi-sensor

## Database Schema (FR1)

```sql
CREATE TABLE IF NOT EXISTS radar_serial_config (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    port_path TEXT NOT NULL,
    baud_rate INTEGER NOT NULL DEFAULT 19200,
    data_bits INTEGER NOT NULL DEFAULT 8,
    stop_bits INTEGER NOT NULL DEFAULT 1,
    parity TEXT NOT NULL DEFAULT 'N',
    enabled INTEGER NOT NULL DEFAULT 1,
    description TEXT,
    sensor_model TEXT DEFAULT 'OmniPreSense OPS243-A',
    created_at INTEGER NOT NULL DEFAULT (STRFTIME('%s', 'now')),
    updated_at INTEGER NOT NULL DEFAULT (STRFTIME('%s', 'now'))
);
```

## API Endpoints (FR2-FR4)

- `GET /api/serial/configs` - List all configurations
- `GET /api/serial/configs/:id` - Get single configuration
- `POST /api/serial/configs` - Create configuration
- `PUT /api/serial/configs/:id` - Update configuration
- `DELETE /api/serial/configs/:id` - Delete configuration
- `POST /api/serial/test` - Test serial port connection
- `POST /api/serial/detect-baud` - Auto-detect baud rate

## Testing Algorithm (FR3)

1. Open serial port with specified settings
2. Send safe query command (`??`)
3. Wait for JSON response (5 second timeout)
4. Validate response format
5. Return success/failure with diagnostics

## File Locations

**Backend:**
- Migration: `data/migrations/YYYYMMDD_create_radar_serial_config.sql`
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
