# Feature Specification: Serial Configuration and Testing via UI

**Status:** Draft  
**Created:** 2025-11-06  
**Author:** Ictinus (Product Architect)  
**Issue:** Serial config + test (baud, port) via UI

## Executive Summary

Enable users to configure and test radar serial port settings through the web interface, supporting multiple radar sensors and eliminating the need to manually edit configuration files or systemd service parameters.

## User Value Proposition

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

## Target Users

**Primary Users:**
- DIY radar operators managing Raspberry Pi deployments
- Community advocates setting up neighborhood monitoring
- Users with multiple radar sensors for coverage expansion

**User Personas:**

1. **The Tinkerer** - Experimenting with different sensor placements, needs to quickly test serial connections
2. **The Installer** - Setting up new deployments, wants confidence settings are correct before leaving the site
3. **The Expander** - Adding additional radars to existing installation, needs multi-sensor coordination

## Current System Capabilities

### Serial Port Management (Existing)

**Component:** `internal/serialmux`
- **Purpose:** Abstraction over serial port with multiplexing for multiple subscribers
- **Implementation:** Generic `SerialMux[T SerialPorter]` with real, mock, and disabled modes
- **Current Configuration:** Hardcoded at startup via `--port` CLI flag (default: `/dev/ttySC1`)
- **Baud Rate:** Currently hardcoded in serial port initialization (19200 for OPS243A)

**Initialization Flow (cmd/radar/radar.go:105-118):**
```go
radarSerial, err := serialmux.NewRealSerialMux(*port)
if err := radarSerial.Initialize(); err != nil {
    log.Fatalf("failed to initialise device: %v", err)
}
```

**Serial Port Interface (internal/serialmux/port.go):**
```go
type SerialPorter interface {
    io.ReadWriter
    io.Closer
}
```

### Database Configuration (Existing)

**Schema:** SQLite database at `/var/lib/velocity-report/sensor_data.db`
- **Site Configuration:** `site` table stores location and report settings
- **Pattern:** Configuration stored in DB, consumed by application at runtime
- **Migration System:** Timestamped SQL files in `data/migrations/`

### Web Interface (Existing)

**Framework:** Svelte/TypeScript with svelte-ux component library
- **Settings Page:** `/settings` route with units and timezone configuration
- **Pattern:** Auto-save on change with instant feedback
- **API Integration:** Fetch from `/api/config`, update via stores

**Existing Settings Pattern (web/src/routes/settings/+page.svelte):**
- SelectField components with auto-save
- Loading states and user feedback messages
- Server default override with localStorage

### HTTP API (Existing)

**Server:** `internal/api/server.go`
- **Config Endpoint:** `/api/config` returns units and timezone
- **Pattern:** JSON responses with error handling
- **Admin Routes:** Attached via `AttachAdminRoutes(*http.ServeMux)`

## Feature Requirements

### Functional Requirements

#### FR1: Database Schema for Serial Configuration

**Requirement:** Create database table to store serial port configurations

**Schema Design:**

```sql
-- Sensor models reference table with model-specific capabilities and commands
CREATE TABLE IF NOT EXISTS radar_sensor_models (
       id INTEGER PRIMARY KEY AUTOINCREMENT
     , model_name TEXT NOT NULL UNIQUE
     , has_doppler INTEGER NOT NULL DEFAULT 1
     , has_fmcw INTEGER NOT NULL DEFAULT 0
     , has_distance INTEGER NOT NULL DEFAULT 0
     , default_baud_rate INTEGER NOT NULL DEFAULT 19200
     , init_commands TEXT NOT NULL
     , description TEXT
     );

-- Insert known sensor models
INSERT INTO radar_sensor_models (
       model_name
     , has_doppler
     , has_fmcw
     , has_distance
     , default_baud_rate
     , init_commands
     , description
     )
VALUES (
       'OmniPreSense OPS243-A'
     , 1
     , 0
     , 0
     , 19200
     , 'AX,OJ,OS,OM,OH,OC'
     , 'Doppler radar with speed measurement only'
     ),
     (
       'OmniPreSense OPS243-C'
     , 1
     , 1
     , 1
     , 19200
     , 'AX,OJ,OS,oD,OM,oM,OH,OC'
     , 'FMCW radar with both speed and distance measurement'
     );

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
     , sensor_model_id INTEGER NOT NULL DEFAULT 1
     , created_at INTEGER NOT NULL DEFAULT (STRFTIME('%s', 'now'))
     , updated_at INTEGER NOT NULL DEFAULT (STRFTIME('%s', 'now'))
     , FOREIGN KEY (sensor_model_id) REFERENCES radar_sensor_models (id)
     );

CREATE INDEX IF NOT EXISTS idx_radar_serial_config_enabled 
    ON radar_serial_config (enabled);

CREATE TRIGGER IF NOT EXISTS update_radar_serial_config_timestamp 
    AFTER UPDATE ON radar_serial_config 
BEGIN
    UPDATE radar_serial_config 
    SET updated_at = STRFTIME('%s', 'now') 
    WHERE id = NEW.id;
END;

-- Insert default configuration for HAT (Raspberry Pi header)
INSERT INTO radar_serial_config (
       name
     , port_path
     , baud_rate
     , data_bits
     , stop_bits
     , parity
     , enabled
     , description
     , sensor_model_id
     )
VALUES (
       'Default HAT'
     , '/dev/ttySC1'
     , 19200
     , 8
     , 1
     , 'N'
     , 1
     , 'Default serial configuration for Raspberry Pi HAT (SC16IS762)'
     , 1
     );
```

**Rationale:**
- **Sensor Models Table:** Reference table defining capabilities and initialization commands for each radar model (OPS243-A vs OPS243-C require different commands)
- **Model-Specific Init:** OPS243-A uses Doppler only (`OS,OM`), OPS243-C adds FMCW distance reporting (`oD,oM`)
- **Serial Settings (8N1):** Standard configuration for OPS243A radar (8 data bits, No parity, 1 stop bit)
- **Multiple Configs:** Support future multi-radar scenarios
- **Enabled Flag:** Allow disabling sensors without deletion
- **Default HAT Config:** `/dev/ttySC1` is the SC16IS762 HAT default for Raspberry Pi
- **Foreign Key to Models:** `sensor_model_id` references the models table for validation and initialization

**Migration File:** `data/migrations/20251106_create_radar_serial_config.sql`

#### FR2: Go API Endpoints for Serial Configuration

**Requirement:** REST endpoints to manage serial configurations

**Endpoints:**

1. **List Serial Configurations**
   - **Method:** `GET`
   - **Path:** `/api/serial/configs`
   - **Response:**
     ```json
     [
       {
         "id": 1,
         "name": "Default HAT",
         "port_path": "/dev/ttySC1",
         "baud_rate": 19200,
         "data_bits": 8,
         "stop_bits": 1,
         "parity": "N",
         "enabled": true,
         "description": "Default serial configuration for Raspberry Pi HAT",
         "sensor_model": {
           "id": 1,
           "model_name": "OmniPreSense OPS243-A",
           "has_doppler": true,
           "has_fmcw": false,
           "has_distance": false
         },
         "created_at": 1699123456,
         "updated_at": 1699123456
       }
     ]
     ```

2. **Get Single Serial Configuration**
   - **Method:** `GET`
   - **Path:** `/api/serial/configs/:id`
   - **Response:** Single config object (same structure as list item)

3. **Create Serial Configuration**
   - **Method:** `POST`
   - **Path:** `/api/serial/configs`
   - **Body:**
     ```json
     {
       "name": "USB Radar #1",
       "port_path": "/dev/ttyUSB0",
       "baud_rate": 19200,
       "description": "USB-connected OPS243A sensor",
       "sensor_model_id": 1
     }
     ```
   - **Response:** Created config object with assigned ID

4. **Update Serial Configuration**
   - **Method:** `PUT`
   - **Path:** `/api/serial/configs/:id`
   - **Body:** Same as create (partial updates supported)
   - **Response:** Updated config object

5. **Delete Serial Configuration**
   - **Method:** `DELETE`
   - **Path:** `/api/serial/configs/:id`
   - **Response:** `204 No Content` on success

6. **List Sensor Models**
   - **Method:** `GET`
   - **Path:** `/api/serial/models`
   - **Response:**
     ```json
     [
       {
         "id": 1,
         "model_name": "OmniPreSense OPS243-A",
         "has_doppler": true,
         "has_fmcw": false,
         "has_distance": false,
         "default_baud_rate": 19200,
         "init_commands": "AX,OJ,OS,OM,OH,OC",
         "description": "Doppler radar with speed measurement only"
       },
       {
         "id": 2,
         "model_name": "OmniPreSense OPS243-C",
         "has_doppler": true,
         "has_fmcw": true,
         "has_distance": true,
         "default_baud_rate": 19200,
         "init_commands": "AX,OJ,OS,oD,OM,oM,OH,OC",
         "description": "FMCW radar with both speed and distance measurement"
       }
     ]
     ```

**Implementation Location:** `internal/api/serial_config.go` (new file)

**Error Handling:**
- `400 Bad Request`: Invalid configuration values
- `404 Not Found`: Config ID does not exist
- `409 Conflict`: Name already exists (unique constraint)
- `500 Internal Server Error`: Database errors

#### FR3: Serial Port Testing Endpoint

**Requirement:** Validate serial port configuration before saving

**Endpoint:**

- **Method:** `POST`
- **Path:** `/api/serial/test`
- **Body:**
  ```json
  {
    "port_path": "/dev/ttySC1",
    "baud_rate": 19200,
    "data_bits": 8,
    "stop_bits": 1,
    "parity": "N",
    "timeout_seconds": 5,
    "auto_correct_baud": true
  }
  ```
- **Response (Success):**
  ```json
  {
    "success": true,
    "port_path": "/dev/ttySC1",
    "baud_rate": 19200,
    "test_duration_ms": 342,
    "bytes_received": 128,
    "sample_data": "{\"speed\": 15.2, \"magnitude\": 456, ...}",
    "raw_responses": [
      {"command": "??", "response": "{\"module\":\"OPS243-A\",\"version\":\"1.2.3\"}", "is_json": true},
      {"command": "I?", "response": "19200", "is_json": false}
    ],
    "message": "Serial port communication successful"
  }
  ```
- **Response (Failure):**
  ```json
  {
    "success": false,
    "port_path": "/dev/ttySC1",
    "baud_rate": 19200,
    "error": "Failed to open port: device not found",
    "test_duration_ms": 102,
    "suggestion": "Check that the device is connected and permissions are correct"
  }
  ```

**Testing Algorithm:**

1. **Open Port:** Attempt to open serial port with specified settings
2. **Send Command:** Send a safe query command (e.g., `??` for firmware version)
3. **Wait for Response:** Read with timeout (default 5 seconds)
4. **Parse and Log Response:** 
   - Attempt to parse as JSON (OPS243A uses JSON mode after `OJ` command)
   - If JSON parsing fails, log the raw text response (some commands like `I?` return non-JSON text)
   - Store both JSON and non-JSON responses for diagnostics and testing verification
   - Non-JSON responses are valid and expected for certain commands (e.g., `I?` returns just the baud rate number)
5. **Auto-Correct Baud Rate (Optional):** If `auto_correct_baud` is true and connection succeeds:
   - Query current baud rate with `I?` command (returns non-JSON response)
   - Parse the numeric response to determine actual baud rate
   - If device reports different rate than configured, update the configuration
   - This handles cases where user issued `I1`, `I2`, `I4`, or `I5` commands manually
6. **Close Port:** Clean up connection
7. **Return Results:** Success/failure with diagnostic information, including captured responses (both JSON and non-JSON)

**Diagnostic Suggestions:**

- **Port not found:** "Check that the device is connected and appears in /dev/"
- **Permission denied:** "Run: sudo usermod -a -G dialout velocity && sudo reboot"
- **Timeout:** "Device may be at wrong baud rate. Try 9600, 115200, or other common rates"
- **Invalid response:** "Device responded but data format is unexpected. Check sensor model and output mode (use `OJ` for JSON mode)"
- **Non-JSON response:** "Device returned non-JSON response. This is normal for query commands like `I?`. Response logged for diagnostics."
- **Baud rate mismatch:** If timeout at 19200, suggest testing other common rates
- **Baud rate changed:** "Device reports different baud rate (detected via I? command). Configuration updated automatically."

**Baud Rate Commands (OPS243 Series):**
- `I1` - Set to 9,600 baud
- `I2` - Set to 19,200 baud (default)
- `I3` - Set to 57,600 baud
- `I4` - Set to 115,200 baud
- `I5` - Set to 230,400 baud
- `I?` - Query current baud rate

**Implementation Location:** `internal/api/serial_test.go` (new file)

**Safety Considerations:**
- Non-destructive testing only (read-only commands)
- Timeout prevents hanging on unresponsive devices
- Automatic cleanup even on errors
- Concurrent test prevention (mutex on port access)
- Log all responses (JSON and non-JSON) for diagnostics without failing the test
- Non-JSON responses are expected and valid for certain commands (e.g., `I?` returns plain text)

#### FR4: Baud Rate Auto-Detection

**Requirement:** Automatically detect correct baud rate for connected device

**Endpoint:**

- **Method:** `POST`
- **Path:** `/api/serial/detect-baud`
- **Body:**
  ```json
  {
    "port_path": "/dev/ttySC1",
    "timeout_seconds": 10
  }
  ```
- **Response:**
  ```json
  {
    "success": true,
    "port_path": "/dev/ttySC1",
    "detected_baud_rate": 19200,
    "test_duration_ms": 1543,
    "rates_tested": [9600, 19200, 38400, 57600, 115200],
    "message": "Detected working baud rate: 19200",
    "sample_data": "{\"speed\": 0.0, \"magnitude\": 12, ...}"
  }
  ```

**Auto-Detection Algorithm:**

1. **Test Common Rates:** Iterate through [9600, 19200, 38400, 57600, 115200]
2. **For Each Rate:**
   - Open port at rate
   - Send query command (`??`)
   - Wait for valid JSON response (timeout: 2 seconds per rate)
   - If valid response received, return that rate
   - Close port and try next rate
3. **Return Results:** First working rate or failure if none work

**Implementation Location:** `internal/api/serial_test.go` (same file as FR3)

**UX Benefit:** Users can click "Auto-Detect" instead of manually trying different baud rates

#### FR5: Web UI for Serial Configuration

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
     - Port Path (text, required, e.g., `/dev/ttySC1`)
     - Baud Rate (select: 9600, 19200, 38400, 57600, 115200)
     - Description (textarea, optional)
     - Sensor Model (select from `/api/serial/models`, shows capabilities)
     - Advanced: Data Bits, Stop Bits, Parity (defaults to 8N1)
   - Buttons:
     - "Test Connection" - Runs FR3 test with auto-correct option
     - "Auto-Detect Baud" - Runs FR4 detection
     - "Save" - Creates/updates configuration
     - "Cancel" - Discards changes

3. **Test Results Display**
   - Show test results inline with color-coded success/failure
   - Display diagnostic messages and suggestions
   - Show sample data received from device

4. **Add Configuration Button**
   - Prominent "Add Serial Port" button
   - Opens editor modal with empty form

**Implementation:**

- **Route File:** `web/src/routes/settings/serial/+page.svelte`
- **API Client:** `web/src/lib/api.ts` (extend existing API helpers)
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

#### FR6: Server Integration with Database Configuration

**Requirement:** Load serial configuration from database at startup

**Current Behavior:**
```go
// cmd/radar/radar.go:35
port = flag.String("port", "/dev/ttySC1", "Serial port to use")
```

**New Behavior:**

1. **Startup Sequence:**
   - Initialize database connection
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

- **File:** `cmd/radar/radar.go`
- **Function:** New `loadSerialConfigs(db *db.DB) ([]SerialConfig, error)`
- **Structure:**
  ```go
  type SerialConfig struct {
      ID            int
      Name          string
      PortPath      string
      BaudRate      int
      DataBits      int
      StopBits      int
      Parity        string
      Enabled       bool
      Description   string
      SensorModelID int
      SensorModel   SensorModel
  }
  
  type SensorModel struct {
      ID             int
      ModelName      string
      HasDoppler     bool
      HasFMCW        bool
      HasDistance    bool
      DefaultBaud    int
      InitCommands   string  // Comma-separated commands
      Description    string
  }
  ```

**Backward Compatibility:**
- If database is empty, fall back to CLI flag
- Log warning if both database config and CLI flag are present
- Existing deployments continue working without migration

### Non-Functional Requirements

#### NFR1: Performance

- **API Response Time:** < 200ms for config CRUD operations
- **Test Operation:** < 5 seconds timeout for serial port test
- **Auto-Detection:** < 15 seconds to test all common baud rates
- **UI Responsiveness:** No blocking operations, loading states for all async actions

#### NFR2: Security

- **Input Validation:** Sanitize all port paths to prevent command injection
- **Path Restrictions:** Only allow `/dev/tty*` and `/dev/serial*` patterns
- **Permission Checks:** Validate serial port permissions before testing
- **Rate Limiting:** Prevent DoS via repeated test operations

#### NFR3: Reliability

- **Port Lock Prevention:** Ensure test operations release port even on timeout/error
- **Concurrent Access:** Mutex protection for serial port access during tests
- **Database Transactions:** Atomic config updates to prevent corruption
- **Graceful Degradation:** Continue serving data even if serial config fails to load

#### NFR4: Usability

- **Clear Error Messages:** User-friendly explanations for all failure modes
- **Guided Troubleshooting:** Actionable suggestions for common issues
- **Visual Feedback:** Loading spinners, success/error indicators, progress for long operations
- **Help Documentation:** Inline help text for technical fields (baud rate, parity, etc.)

#### NFR5: Maintainability

- **Code Organization:** Separate concerns (DB, API, UI) into appropriate modules
- **Test Coverage:** Unit tests for all serial testing logic and API endpoints
- **Documentation:** API documentation, user guide, troubleshooting section
- **Consistent Patterns:** Follow existing codebase conventions (migrations, API structure, UI components)

## Implementation Phases

### Phase 1: Database Foundation (Minimal Viable Product)

**Goal:** Enable database-driven serial configuration without UI

**Deliverables:**
1. Migration file with `radar_serial_config` table schema
2. Database initialization with default HAT configuration
3. Go server loads config from database at startup
4. Backward compatibility with CLI flag fallback

**Testing:**
- Manual database insertion of config
- Server startup with database config
- Server startup with empty database (CLI fallback)
- Server startup with `--ignore-db-serial` flag

**Timeline:** 1-2 days

### Phase 2: API Endpoints (Backend Complete)

**Goal:** Full CRUD operations and testing capabilities via API

**Deliverables:**
1. `/api/serial/configs` CRUD endpoints (FR2)
2. `/api/serial/test` testing endpoint (FR3)
3. `/api/serial/detect-baud` auto-detection endpoint (FR4)
4. Unit tests for all endpoints
5. Integration tests for serial port testing

**Testing:**
- API endpoint tests with mock serial ports
- Serial testing with real hardware (OPS243A)
- Error handling for all failure scenarios
- Concurrent test operation handling

**Timeline:** 3-4 days

### Phase 3: Web UI (Full Feature)

**Goal:** User-friendly interface for all serial configuration tasks

**Deliverables:**
1. `/settings/serial` route and page component (FR5)
2. Configuration list view
3. Edit/Create modal with form validation
4. Test connection button with results display
5. Auto-detect baud rate functionality
6. Delete confirmation dialogs
7. User documentation

**Testing:**
- UI component tests
- E2E tests for configuration workflows
- Mobile responsiveness
- Accessibility compliance
- User acceptance testing

**Timeline:** 4-5 days

### Phase 4: Multi-Sensor Support (Future Enhancement)

**Goal:** Support multiple radar sensors simultaneously

**Deliverables:**
1. Multiple SerialMux instances in server
2. Data tagging with sensor ID
3. UI for sensor selection in visualizations
4. Documentation for multi-sensor setups

**Timeline:** 5-7 days (future work)

## Technical Design Decisions

### Decision 1: Database vs. Configuration File

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

### Decision 2: Testing Strategy

**Options:**
- **A) Non-destructive read-only testing** (Selected)
- **B) Full initialization sequence**
- **C) No testing, just configuration storage**

**Rationale for Read-Only Testing:**
- ✅ Safe to run multiple times
- ✅ Won't interfere with live data collection
- ✅ Fast feedback for users
- ✅ Detects most common issues (port, permissions, baud rate)
- ❌ Doesn't validate full initialization sequence

**Rejected:** Full initialization could disrupt live data collection. No testing provides poor user experience.

### Decision 3: Baud Rate Configuration

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

### Decision 4: Multi-Sensor Architecture

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

## Migration Path for Existing Deployments

### For Users on systemd (Production)

**Current Setup:**
```ini
ExecStart=/usr/local/bin/velocity-report --port /dev/ttySC1 --db-path /var/lib/velocity-report/sensor_data.db
```

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

### For Users on Manual Deployment

**Current Setup:**
```bash
./radar --port /dev/ttyUSB0
```

**After Migration:**

1. **CLI Flag Still Works:** No breaking changes
2. **Optional Database Config:** Can configure via UI once running
3. **Auto-Detect Helper:** Use UI to auto-detect baud rate

## Success Metrics

### User Experience Metrics

- **Time to Configure:** < 2 minutes from opening UI to working serial connection
- **Configuration Success Rate:** > 95% of users successfully configure serial port
- **Error Recovery:** < 1 minute from error to solution with diagnostic suggestions
- **Multi-Sensor Adoption:** % of users configuring multiple radars (baseline: 0%, target: 10%)

### Technical Metrics

- **API Performance:** < 200ms for config operations, < 5s for testing
- **Test Accuracy:** 100% detection of non-working configurations
- **Auto-Detect Success:** > 90% correct baud rate detection for OPS243A
- **Zero Downtime:** No data collection interruption during config changes

### Support Metrics

- **Issue Reduction:** 50% reduction in serial configuration support requests
- **Self-Service Rate:** 80% of serial issues resolved without manual intervention
- **Documentation Clarity:** < 5% of users request additional help after reading guide

## Documentation Requirements

### User Documentation

1. **Setup Guide:** Step-by-step serial configuration for new deployments
2. **Troubleshooting Guide:** Common issues and solutions
3. **Multi-Sensor Guide:** How to configure multiple radars (Phase 4)
4. **Hardware Compatibility:** Tested serial adapters and HATs

**Location:** `docs/src/guides/serial-configuration.md`

### Developer Documentation

1. **API Reference:** OpenAPI/Swagger spec for serial endpoints
2. **Database Schema:** ERD and migration history
3. **Testing Guide:** How to run serial tests without hardware
4. **Architecture Decision Record:** Rationale for key design choices

**Location:** `docs/api/serial-endpoints.md`, `ARCHITECTURE.md` update

### In-App Help

1. **Tooltips:** Explain technical terms (baud rate, parity, stop bits)
2. **Field Validation:** Real-time feedback on invalid values
3. **Help Links:** Context-sensitive links to documentation

## Privacy & Security Considerations

### Privacy (Maintained)

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

## Open Questions & Future Considerations

### Open Questions

1. **Q: Should we allow configuration of serial timeout values?**
   - Current: Hardcoded in Initialize() sequence
   - Trade-off: Flexibility vs. complexity
   - Recommendation: Start with sensible defaults, add if users request

2. **Q: Should we support hot-swapping serial configurations without restart?**
   - Current: Changes require server restart
   - Trade-off: Complexity vs. user convenience
   - Recommendation: Phase 2 feature (after basic CRUD)

3. **Q: How do we handle multiple radars pointing at the same location vs. different locations?**
   - Current: Not addressed
   - Trade-off: Simplicity vs. advanced use cases
   - Recommendation: Defer to multi-sensor phase (Phase 4)

### Future Enhancements

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

## Appendix: Technical References

### OPS243A Serial Configuration

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

### Common Serial Adapters

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
