# Serial configuration and testing via UI

- **Status:** Draft
- **Issue:** Serial config + test (baud, port) via UI

Design specification for a web-based interface that lets users configure and test radar serial port settings without manually editing systemd service files.

## Overview

Enable users to configure and test radar serial port settings through the web interface, supporting multiple radar sensors and eliminating the need to manually edit configuration files or systemd service parameters.

## Problem

**Problem Statement:**

Currently, radar serial port configuration is hardcoded via command-line flags (`--port /dev/ttySC1`), requiring:

- Manual editing of systemd service files
- Service restarts for configuration changes
- No validation that serial settings are correct before deployment
- No support for multiple radar sensors
- Technical expertise to diagnose serial communication issues

**User Benefits:**

- **Self-Service Configuration:** Users can configure serial settings through a web interface without SSH access or systemd knowledge
- **Instant Validation:** Test serial port connectivity and baud rate before saving configuration
- **Multi-Sensor Support:** Configure multiple radar sensors for coordinated monitoring
- **Troubleshooting Aid:** Built-in diagnostics help users identify serial communication problems
- **Safer Deployments:** Validate settings before committing changes, reducing downtime

## Current system capabilities

### Serial port management (existing)

**Component:** [internal/serialmux](../../../internal/serialmux)

- **Purpose:** Abstraction over serial port with multiplexing for multiple subscribers
- **Implementation:** Generic `SerialMux[T SerialPorter]` with real, mock, and disabled modes
- **Current Configuration:** Hardcoded at startup via `--port` CLI flag (default: `/dev/ttySC1`)
- **Baud Rate:** Currently hardcoded in serial port initialisation (19200 for OPS243A)

**Initialisation Flow (cmd/radar/radar.go:105-118):**

> **Source:** [cmd/radar/radar.go](../../../cmd/radar/radar.go). Creates a `RealSerialMux` from the CLI port flag, then calls `Initialise()`; fatal on failure.

**Serial Port Interface (internal/serialmux/port.go):**

> **Source:** [internal/serialmux/port.go](../../../internal/serialmux/port.go). `SerialPorter` interface embeds `io.ReadWriter` and `io.Closer`.

### Database configuration (existing)

**Schema:** SQLite database at `/var/lib/velocity-report/sensor_data.db`

- **Site Configuration:** `site` table stores location and report settings
- **Pattern:** Configuration stored in DB, consumed by application at runtime
- **Migration System:** Timestamped SQL files in [internal/db/migrations/](../../../internal/db/migrations)

### Web interface (existing)

**Framework:** Svelte/TypeScript with svelte-ux component library

- **Settings Page:** `/settings` route with units and timezone configuration
- **Pattern:** Auto-save on change with instant feedback
- **API Integration:** Fetch from `/api/config`, update via stores

**Existing Settings Pattern (web/src/routes/settings/+page.svelte):**

- SelectField components with auto-save
- Loading states and user feedback messages
- Server default override with localStorage

### HTTP API (existing)

**Server:** [internal/api/server.go](../../../internal/api/server.go)

- **Config Endpoint:** `/api/config` returns units and timezone
- **Pattern:** JSON responses with error handling
- **Admin Routes:** Attached via `AttachAdminRoutes(*http.ServeMux)`

## Feature requirements

### Functional requirements

#### FR1: database schema for serial configuration

**Requirement:** Create database table to store serial port configurations

**Schema Design:**

> **Source:** Schema in [internal/db/migrations/](../../../internal/db/migrations) (when implemented). Table `radar_serial_config` with columns: id, name, port_path, baud_rate, data_bits, stop_bits, parity, enabled, description, sensor_model, created_at, updated_at. CHECK constraint validates sensor_model. Default config inserts HAT entry (`/dev/ttySC1`, 19200 baud, `ops243-a`).

**Rationale:**

- **Sensor Model Slugs:** Use simple text identifiers (`ops243-a`, `ops243-c`) validated via SQLite CHECK constraint
- **Application-Side Logic:** Sensor capabilities and initialisation commands stored in application code, not database
- **CHECK Constraint:** Validates sensor model values at database level without requiring separate table
- **Migration-Friendly:** Adding new sensor models only requires application update, not database migration
- **Serial Settings (8N1):** Standard configuration for OPS243A radar (8 data bits, No parity, 1 stop bit)
- **Multiple Configs:** Support future multi-radar scenarios
- **Enabled Flag:** Allow disabling sensors without deletion
- **Default HAT Config:** `/dev/ttySC1` is the SC16IS762 HAT default for Raspberry Pi

**Migration File:** `internal/db/migrations/20251106_create_radar_serial_config.sql`

**Sensor Model Information (Application Code):**

> **Source:** Sensor model registry in [internal/radar/](../../../internal/radar) (when implemented). Defines `SensorModel` struct and `SupportedSensorModels` map with entries for `ops243-a` (Doppler-only, 19200 baud, commands: AX/OJ/OS/OM/OH/OC) and `ops243-c` (FMCW + distance, 19200 baud, commands: AX/OJ/OS/oD/OM/oM/OH/OC).

#### FR2: Go API endpoints for serial configuration

Seven REST endpoints under `/api/serial/` manage the configuration lifecycle: CRUD for `radar_serial_config` entries, device enumeration (filtering out already-assigned ports), and sensor model metadata from application code. Implementation in `internal/api/serial_config.go`.

See [serial-configuration-api.md](serial-configuration-api.md) for the full endpoint specification with request/response schemas and error contracts.

#### FR3: serial port testing endpoint

A `POST /api/serial/test` endpoint validates serial port configuration before saving. Sends non-destructive query commands (`??`, `I?`), reads with configurable timeout, optionally auto-corrects baud rate if the device reports a different rate via `I?`. Returns diagnostic information including raw responses (JSON and non-JSON) and actionable suggestions for common failures (port not found, permission denied, timeout, baud mismatch). Implementation in `internal/api/serial_test.go`.

See [serial-configuration-api.md](serial-configuration-api.md) for the full specification including the testing algorithm, diagnostic suggestion table, and OPS243 baud rate commands.

#### FR4: serial auto-detection (port + baud)

Two endpoints help users find connected radar devices: `POST /api/serial/auto-detect` probes all unassigned serial ports across common baud rates to find a responsive OPS243 device, and `POST /api/serial/detect-baud` tests a known port at common rates. Both use non-destructive query commands and return diagnostic data including ports tested and ports excluded. Implementation in `internal/api/serial_test.go`.

See [serial-configuration-api.md](serial-configuration-api.md) for the full specification including the auto-detection algorithm and response schemas.

#### FR5: web UI for serial configuration

**Requirement:** User interface to view, edit, test, and manage serial configurations

**Route:** `/settings/serial` (new sub-route under settings)

**Page Components:**

1. **Configuration List**
   - Table showing all configured serial ports
   - Columns: Name, Port, Baud Rate, Status (Enabled/Disabled), Actions
   - Actions: Edit, Test, Enable/Disable, Delete

2. **Configuration Editor (Modal/Drawer)**
   - Form fields:
     - Name (text, required, unique)
     - Port Path (select + manual override, required)
       - Default dropdown options come from `GET /api/serial/devices`
       - Excludes any paths already assigned to another configuration
       - Allows manual entry for advanced cases (validation still enforces `/dev/tty*` / `/dev/serial*`)
     - Baud Rate (select: 9600, 19200, 38400, 57600, 115200)
     - Description (textarea, optional)
     - Sensor Model (select from `/api/serial/models`, shows capabilities)
     - Advanced: Data Bits, Stop Bits, Parity (defaults to 8N1)
   - Buttons:
     - "Test Connection" - Runs FR3 test with auto-correct option
     - "Detect Device" - Calls `/api/serial/auto-detect` and populates port + baud + model on success
     - "Auto-Detect Baud" - Runs FR4 baud detection when port is chosen manually
     - "Save" - Creates/updates configuration
     - "Cancel" - Discards changes

3. **Test Results Display**
   - Show test results inline with colour-coded success/failure
   - Display diagnostic messages and suggestions
   - Show sample data received from device

4. **Add Configuration Button**
   - Prominent "Add Serial Port" button
   - Opens editor modal with empty form

**Implementation:**

- **Route File:** `web/src/routes/settings/serial/+page.svelte`
- **API Client:** [web/src/lib/api.ts](../../../web/src/lib/api.ts) (extend existing API helpers)
- **TypeScript Types:** `web/src/lib/types/serial.ts` (new file)

**Design Pattern:** Follow existing settings page patterns

- Auto-save option or explicit "Save" button (recommend explicit for hardware config)
- Loading states during API calls
- User feedback messages for actions
- Confirmation dialogs for destructive actions (delete)

**Accessibility:**

- Proper ARIA labels for form fields
- Keyboard navigation support
- Screen reader announcements for test results
- Focus management in modals

#### FR6: server integration with database configuration

**Requirement:** Load serial configuration from database at startup

**Current Behaviour:**

> **Source:** `cmd/radar/radar.go:35`. CLI flag `--port` defaults to `/dev/ttySC1`.

**New Behaviour:**

1. **Startup Sequence:**
   - Initialise database connection
   - Load enabled serial configurations from `radar_serial_config`
   - If no configs found, use CLI flag value (backward compatibility)
   - Create SerialMux instances for each enabled configuration

2. **Configuration Priority:**
   - Database configurations take precedence over CLI flags
   - CLI flag `--port` becomes fallback for legacy deployments
   - New flag: `--ignore-db-serial` to force CLI flag usage

3. **Multi-Sensor Support (Future-Ready):**
   - Store multiple SerialMux instances in a map
   - Subscribe to all active sensors
   - Tag incoming data with sensor ID for multi-radar analytics

**Implementation Changes:**

- **File:** [cmd/radar/radar.go](../../../cmd/radar/radar.go)
- **Function:** New `loadSerialConfigs(db *db.DB) ([]SerialConfig, error)`

> **Source:** `SerialConfig` struct, `SensorModel` struct, and `GetSensorModel()` defined in sensor model registry (see FR1). Application-side model registry eliminates the need for database migrations when adding new sensor support.

**Backward Compatibility:**

- If database is empty, fall back to CLI flag
- Log warning if both database config and CLI flag are present
- Existing deployments continue working without migration

### Non-Functional requirements

#### NFR1: performance

- **API Response Time:** < 200ms for config CRUD operations
- **Test Operation:** < 5 seconds timeout for serial port test
- **Auto-Detection:** < 15 seconds to test all common baud rates
- **UI Responsiveness:** No blocking operations, loading states for all async actions

#### NFR2: security

- **Input Validation:** Sanitize all port paths to prevent command injection
- **Path Restrictions:** Only allow `/dev/tty*` and `/dev/serial*` patterns
- **Permission Checks:** Validate serial port permissions before testing
- **Rate Limiting:** Prevent DoS via repeated test operations

#### NFR3: reliability

- **Port Lock Prevention:** Ensure test operations release port even on timeout/error
- **Concurrent Access:** Mutex protection for serial port access during tests
- **Database Transactions:** Atomic config updates to prevent corruption
- **Graceful Degradation:** Continue serving data even if serial config fails to load

#### NFR4: usability

- **Clear Error Messages:** User-friendly explanations for all failure modes
- **Guided Troubleshooting:** Actionable suggestions for common issues
- **Visual Feedback:** Loading spinners, success/error indicators, progress for long operations
- **Help Documentation:** Inline help text for technical fields (baud rate, parity, etc.)

#### NFR5: maintainability

- **Code Organisation:** Separate concerns (DB, API, UI) into appropriate modules
- **Test Coverage:** Unit tests for all serial testing logic and API endpoints
- **Documentation:** API documentation, user guide, troubleshooting section
- **Consistent Patterns:** Follow existing codebase conventions (migrations, API structure, UI components)

## Implementation phases

### Phase 1: database foundation (minimal viable product)

**Goal:** Enable database-driven serial configuration without UI

**Deliverables:**

1. Migration file with `radar_serial_config` table schema
2. Database initialisation with default HAT configuration
3. Go server loads config from database at startup
4. Backward compatibility with CLI flag fallback

**Testing:**

- Manual database insertion of config
- Server startup with database config
- Server startup with empty database (CLI fallback)
- Server startup with `--ignore-db-serial` flag

**Timeline:** 1-2 days

### Phase 2: API endpoints (backend complete)

**Goal:** Full CRUD operations and testing capabilities via API

**Deliverables:**

1. `/api/serial/configs` CRUD endpoints (FR2)
2. `/api/serial/devices` discovery endpoint with filtering (FR2)
3. `/api/serial/test` testing endpoint (FR3)
4. `/api/serial/auto-detect` device/baud discovery endpoint (FR4)
5. `/api/serial/detect-baud` fallback endpoint for known ports (FR4)
6. Unit tests for all endpoints
7. Integration tests for serial port testing

**Testing:**

- API endpoint tests with mock serial ports
- Serial testing with real hardware (OPS243A)
- Error handling for all failure scenarios
- Concurrent test operation handling

**Timeline:** 3-4 days

### Phase 3: web UI (full feature)

**Goal:** User-friendly interface for all serial configuration tasks

**Deliverables:**

1. `/settings/serial` route and page component (FR5)
2. Configuration list view
3. Edit/Create modal with form validation
4. Test connection button with results display
5. Device discovery workflow (Detect Device button + available ports dropdown)
6. Auto-detect baud rate functionality
7. Delete confirmation dialogs
8. User documentation

**Testing:**

- UI component tests
- E2E tests for configuration workflows
- Mobile responsiveness
- Accessibility compliance
- User acceptance testing

**Timeline:** 4-5 days

### Phase 4: multi-sensor support (future enhancement)

**Goal:** Support multiple radar sensors simultaneously

**Deliverables:**

1. Multiple SerialMux instances in server
2. Data tagging with sensor ID
3. UI for sensor selection in visualisations
4. Documentation for multi-sensor setups

**Timeline:** 5-7 days (future work)

## Technical design decisions

### Decision 1: database vs. configuration file

**Options:**

- **A) SQLite database table** (Selected)
- **B) JSON configuration file**
- **C) TOML/YAML configuration file**

**Rationale for Database:**

- ✅ Consistent with existing pattern (site configuration stored in DB)
- ✅ Easy to expose via REST API
- ✅ No file system permissions issues
- ✅ Atomic updates with transactions
- ✅ Queryable and indexable
- ❌ Slightly more complex than flat file

**Rejected:** Configuration files would require file parsing, permission management, and would be harder to expose via REST API.

### Decision 2: testing strategy

**Options:**

- **A) Non-destructive read-only testing** (Selected)
- **B) Full initialisation sequence**
- **C) No testing, just configuration storage**

**Rationale for Read-Only Testing:**

- ✅ Safe to run multiple times
- ✅ Won't interfere with live data collection
- ✅ Fast feedback for users
- ✅ Detects most common issues (port, permissions, baud rate)
- ❌ Doesn't validate full initialisation sequence

**Rejected:** Full initialisation could disrupt live data collection. No testing provides poor user experience.

### Decision 3: baud rate configuration

**Options:**

- **A) User-selectable from common rates** (Selected)
- **B) Auto-detect only**
- **C) Freeform text entry**

**Rationale for Selectable Rates:**

- ✅ Prevents typos and invalid values
- ✅ Common rates cover 99% of use cases
- ✅ Auto-detect available as helper function
- ✅ Advanced users can still use uncommon rates via database
- ❌ Slightly less flexible than freeform

**Rejected:** Auto-detect only is too slow for every configuration. Freeform entry prone to errors.

### Decision 4: multi-sensor architecture

**Options:**

- **A) Multiple SerialMux instances** (Selected for future)
- **B) Single SerialMux with multiplexing**
- **C) Separate processes per sensor**

**Rationale for Multiple Instances:**

- ✅ Clean separation of concerns
- ✅ Independent error handling per sensor
- ✅ Simpler debugging
- ✅ Future-ready for distributed deployments
- ❌ Higher memory footprint

**Rejected:** Single multiplexed instance is complex. Separate processes complicate deployment.

### Decision 5: sensor model storage (application vs database)

**Options:**

- **A) Application-side with CHECK constraint** (Selected)
- **B) Database reference table with foreign key**
- **C) Freeform text without validation**

**Rationale for Application-Side:**

- ✅ No database migrations needed when adding new sensor models
- ✅ Sensor capabilities and init commands versioned with application code
- ✅ CHECK constraint provides database-level validation
- ✅ Simpler for developers to update sensor definitions
- ✅ Easier to test sensor model logic
- ❌ Cannot add sensor models without application update

**Rejected:** Database reference table requires migrations for new sensors. Freeform text lacks validation and type safety.

The CHECK constraint in the migration validates sensor model slugs at the database level. Adding new sensor models requires both an application update and a migration to update the CHECK constraint.

## Migration path for existing deployments

### For users on systemd (production)

**Current Setup:**

The systemd unit file uses `ExecStart=/usr/local/bin/velocity-report --port /dev/ttySC1 --db-path /var/lib/velocity-report/sensor_data.db`.

**After Migration:**

1. **Database Auto-Migration:** Migration runs on first startup, creates default config
2. **CLI Flag Still Works:** No service file changes required
3. **Optional UI Configuration:** Users can edit via web interface after startup
4. **Recommendation:** Edit via UI, then remove `--port` flag from service file

**Migration Steps (User-Facing Documentation):**

```bash
# 1. Update binary (migration runs automatically)
sudo systemctl stop velocity-report
sudo cp /path/to/new/velocity-report /usr/local/bin/
sudo systemctl start velocity-report

# 2. Configure via web UI (optional)
# Visit http://raspberrypi.local:8080/settings/serial
# Test and verify serial configuration

# 3. Clean up service file (optional)
sudo systemctl edit velocity-report
# Remove --port flag, keep --db-path
sudo systemctl daemon-reload
sudo systemctl restart velocity-report
```

### For users on manual deployment

**Current Setup:**

```bash
./radar --port /dev/ttyUSB0
```

**After Migration:**

1. **CLI Flag Still Works:** No breaking changes
2. **Optional Database Config:** Can configure via UI once running
3. **Auto-Detect Helper:** Use UI to auto-detect baud rate

## Success metrics

### User experience metrics

- **Time to Configure:** < 2 minutes from opening UI to working serial connection
- **Configuration Success Rate:** > 95% of users successfully configure serial port
- **Error Recovery:** < 1 minute from error to solution with diagnostic suggestions
- **Multi-Sensor Adoption:** % of users configuring multiple radars (baseline: 0%, target: 10%)

### Technical metrics

- **API Performance:** < 200ms for config operations, < 5s for testing
- **Test Accuracy:** 100% detection of non-working configurations
- **Auto-Detect Success:** > 90% correct baud rate detection for OPS243A
- **Zero Downtime:** No data collection interruption during config changes

### Support metrics

- **Issue Reduction:** 50% reduction in serial configuration support requests
- **Self-Service Rate:** 80% of serial issues resolved without manual intervention
- **Documentation Clarity:** < 5% of users request additional help after reading guide

## Documentation requirements

### User documentation

1. **Setup Guide:** Step-by-step serial configuration for new deployments
2. **Troubleshooting Guide:** Common issues and solutions
3. **Multi-Sensor Guide:** How to configure multiple radars (Phase 4)
4. **Hardware Compatibility:** Tested serial adapters and HATs

**Location:** `docs/src/guides/serial-configuration.md`

### Developer documentation

1. **API Reference:** OpenAPI/Swagger spec for serial endpoints
2. **Database Schema:** ERD and migration history
3. **Testing Guide:** How to run serial tests without hardware
4. **Architecture Decision Record:** Rationale for key design choices

**Location:** `docs/api/serial-endpoints.md`, [ARCHITECTURE.md](../../../ARCHITECTURE.md) update

### In-App help

1. **Tooltips:** Explain technical terms (baud rate, parity, stop bits)
2. **Field Validation:** Real-time feedback on invalid values
3. **Help Links:** Context-sensitive links to documentation

## Privacy & security considerations

### Privacy (maintained)

- ✅ **No PII Collection:** Serial configuration contains no personally identifiable information
- ✅ **Local-Only Storage:** All data remains in local SQLite database
- ✅ **No External Transmission:** No serial configuration data sent to cloud/external servers

### Security

**Input Validation:**

- Port paths restricted to `/dev/tty*` and `/dev/serial*` patterns
- Prevent command injection via path traversal attacks
- Sanitize all user inputs before database storage

**Permission Management:**

- Serial port testing respects system permissions
- Clear error messages for permission issues
- Documentation for proper permission setup (`dialout` group)

**Rate Limiting:**

- Prevent DoS via repeated test operations
- Concurrent test prevention to avoid port lock

**Audit Trail:**

- Track configuration changes with `created_at` and `updated_at` timestamps
- Future: Add audit log for who changed what (if user management added)

## Open questions & future considerations

### Resolved design questions

| Question                                      | Resolution                                                                                       |
| --------------------------------------------- | ------------------------------------------------------------------------------------------------ |
| Allow configuration of serial timeout values? | No. Hardcoded sensible defaults in `Initialise()`. Add configurability only if users request it. |

### Open questions

1. **Q: Should we support hot-swapping serial configurations without restart?**
   - Current: Changes require server restart
   - Trade-off: Complexity vs. user convenience
   - Recommendation: Phase 2 feature (after basic CRUD)

2. **Q: How do we handle multiple radars pointing at the same location vs. different locations?**
   - Current: Not addressed
   - Trade-off: Simplicity vs. advanced use cases
   - Recommendation: Explore options now; this needs addressing soon, not deferring to Phase 4

### Future enhancements

1. **Serial Port Health Monitoring:**
   - Track connection uptime
   - Alert on serial disconnections
   - Automatic reconnection attempts

2. **Configuration Templates:**
   - Pre-configured profiles for common hardware (HAT, USB adapters)
   - One-click setup for known-good configurations

3. **Firmware Update via UI:**
   - Upload new firmware to OPS243A via serial
   - Guided firmware update process
   - Rollback on failure

4. **Advanced Diagnostics:**
   - Serial port signal strength/quality metrics
   - Packet loss tracking
   - Performance graphs over time

5. **Configuration Export/Import:**
   - Export serial configs as JSON
   - Import configs from another installation
   - Backup/restore functionality

## Appendix: technical references

### OPS243A serial configuration

**Documented Settings:**

- Baud Rate: 9600, 19200, 38400, 57600, 115200 (factory default: 9600)
- Data Bits: 8
- Stop Bits: 1
- Parity: None (8N1 configuration)
- Flow Control: None

**Reference:** OmniPreSense OPS243 Datasheet

### Raspberry Pi HAT (SC16IS762)

**Default Configuration:**

- Device Path: `/dev/ttySC1` (channel 1), `/dev/ttySC0` (channel 0)
- Driver: `sc16is7xx` kernel module
- Dual-channel: Supports two serial devices simultaneously

**Reference:** [Waveshare SC16IS762 HAT Wiki](https://www.waveshare.com/wiki/Serial_Expansion_HAT)

### Common serial adapters

**USB-Serial Adapters:**

- FTDI FT232: `/dev/ttyUSB0` (most common)
- Prolific PL2303: `/dev/ttyUSB0`
- CH340/CH341: `/dev/ttyUSB0`

**Permission Setup:**

```bash
sudo usermod -a -G dialout $USER
sudo reboot
```

## Conclusion

This feature enables self-service serial port configuration through a web interface, eliminating technical barriers and supporting future multi-sensor deployments. The phased implementation approach delivers value incrementally while maintaining backward compatibility and system reliability.

**Next Steps:**

1. Review and approve this specification
2. Create GitHub issues for each implementation phase
3. Assign to Hadaly (implementation agent) for Phase 1 execution
