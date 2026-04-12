# Serial configuration API endpoints

- **Status:** Draft
- **Parent:** [serial-configuration-ui.md](serial-configuration-ui.md)

API endpoint specifications for serial port configuration, testing, and auto-detection. These endpoints support the serial configuration UI described in the parent document.

## FR2: Go API endpoints for serial configuration

**Requirement:** REST endpoints to manage serial configurations

**Endpoints:**

| #   | Method   | Path                      | Purpose                                                 |
| --- | -------- | ------------------------- | ------------------------------------------------------- |
| 1   | `GET`    | `/api/serial/configs`     | List all serial configurations                          |
| 2   | `GET`    | `/api/serial/configs/:id` | Get single configuration                                |
| 3   | `POST`   | `/api/serial/configs`     | Create configuration                                    |
| 4   | `PUT`    | `/api/serial/configs/:id` | Update configuration (partial updates supported)        |
| 5   | `DELETE` | `/api/serial/configs/:id` | Delete configuration (returns `204`)                    |
| 6   | `GET`    | `/api/serial/devices`     | List available serial devices (excludes assigned ports) |
| 7   | `GET`    | `/api/serial/models`      | List sensor models from application code                |

### Response schemas

**Config object** (returned by endpoints 1–5):

| Field          | Type    | Notes                             |
| -------------- | ------- | --------------------------------- |
| `id`           | integer | Auto-assigned                     |
| `name`         | string  | Unique                            |
| `port_path`    | string  | e.g. `/dev/ttySC1`                |
| `baud_rate`    | integer | 9600, 19200, 38400, 57600, 115200 |
| `data_bits`    | integer | Default 8                         |
| `stop_bits`    | integer | Default 1                         |
| `parity`       | string  | `"N"` (8N1 default)               |
| `enabled`      | boolean |                                   |
| `description`  | string  |                                   |
| `sensor_model` | string  | `ops243-a` or `ops243-c`          |
| `created_at`   | integer | Unix timestamp                    |
| `updated_at`   | integer | Unix timestamp                    |

**Create/Update request body:** `name`, `port_path`, `baud_rate`, `description`, `sensor_model`. Remaining fields use 8N1 defaults.

**Device object** (endpoint 6):

| Field           | Type    | Notes                  |
| --------------- | ------- | ---------------------- |
| `port_path`     | string  | e.g. `/dev/ttyUSB0`    |
| `friendly_name` | string  | e.g. `OPS243-A (FTDI)` |
| `vendor_id`     | string  | USB vendor ID          |
| `product_id`    | string  | USB product ID         |
| `last_seen`     | integer | Unix timestamp         |

Enumerates `/dev/tty*` and `/dev/serial*` via udev/sysfs. Filters out paths already in `radar_serial_config`. Includes USB metadata when available.

**Sensor model object** (endpoint 7, sourced from application code):

| Field               | Type     | Notes                          |
| ------------------- | -------- | ------------------------------ |
| `slug`              | string   | `ops243-a` or `ops243-c`       |
| `display_name`      | string   | Full product name              |
| `has_doppler`       | boolean  |                                |
| `has_fmcw`          | boolean  |                                |
| `has_distance`      | boolean  |                                |
| `default_baud_rate` | integer  |                                |
| `init_commands`     | string[] | OPS243 initialisation sequence |
| `description`       | string   |                                |

**Implementation location (planned):** `internal/api/serial_config.go`

### Error handling

| Status | Meaning                                    |
| ------ | ------------------------------------------ |
| `400`  | Invalid values or unsupported sensor model |
| `404`  | Config ID does not exist                   |
| `409`  | Name already exists (unique constraint)    |
| `500`  | Database error                             |

## FR3: serial port testing endpoint

**Requirement:** Validate serial port configuration before saving

| Method | Path               |
| ------ | ------------------ |
| `POST` | `/api/serial/test` |

### Request fields

| Field               | Type    | Notes         |
| ------------------- | ------- | ------------- |
| `port_path`         | string  | Required      |
| `baud_rate`         | integer | Required      |
| `data_bits`         | integer | Default 8     |
| `stop_bits`         | integer | Default 1     |
| `parity`            | string  | Default `"N"` |
| `timeout_seconds`   | integer | Default 5     |
| `auto_correct_baud` | boolean | Optional      |

### Response fields

**Success:** `success: true`, `port_path`, `baud_rate`, `test_duration_ms`, `bytes_received`, `sample_data`, `raw_responses[]` (each with `command`, `response`, `is_json`), `message`.

**Failure:** `success: false`, `port_path`, `baud_rate`, `error`, `test_duration_ms`, `suggestion`.

### Testing algorithm

1. **Open port** with specified settings
2. **Send command:** safe query (`??` for firmware version)
3. **Wait for response** with timeout (default 5 s)
4. **Parse response:** attempt JSON parse; log raw text for non-JSON responses (e.g. `I?` returns a plain number). Both are valid.
5. **Auto-correct baud** (if enabled): query `I?`, compare reported rate to configured, update if different
6. **Close port** and clean up
7. **Return results** with diagnostic information

### Diagnostic suggestions

| Condition          | Suggestion                                                                |
| ------------------ | ------------------------------------------------------------------------- |
| Port not found     | Check device is connected and appears in `/dev/`                          |
| Permission denied  | `sudo usermod -a -G dialout velocity && sudo reboot`                      |
| Timeout            | Device may be at wrong baud rate: try 9600, 115200, or other common rates |
| Invalid response   | Check sensor model and output mode (use `OJ` for JSON mode)               |
| Non-JSON response  | Normal for query commands like `I?`                                       |
| Baud rate mismatch | Configuration updated automatically (detected via `I?`)                   |

### OPS243 baud rate commands

| Command | Rate               |
| ------- | ------------------ |
| `I1`    | 9,600              |
| `I2`    | 19,200 (default)   |
| `I3`    | 57,600             |
| `I4`    | 115,200            |
| `I5`    | 230,400            |
| `I?`    | Query current rate |

### Safety considerations

- Non-destructive: read-only commands only
- Timeout prevents hanging on unresponsive devices
- Automatic cleanup on error
- Concurrent test prevention (mutex on port access)
- Non-JSON responses are expected and valid for certain commands

**Implementation location (planned):** `internal/api/serial_test.go`

## FR4: serial auto-detection (port + baud)

**Requirement:** Help users find connected radar devices without guessing port paths or baud rates

### Endpoints

| #   | Method | Path                      | Purpose                                     |
| --- | ------ | ------------------------- | ------------------------------------------- |
| 1   | `POST` | `/api/serial/auto-detect` | Find connected device (port + baud + model) |
| 2   | `POST` | `/api/serial/detect-baud` | Find baud rate for a known port             |

### Auto-detect request/response

**Request:** `candidate_models[]` (sensor model slugs), `timeout_seconds` (default 15).

**Success response:** `success`, `port_path`, `detected_baud_rate`, `sensor_model`, `raw_responses[]`, `ports_tested[]`, `excluded_assigned_ports[]`, `message`.

**Failure response:** `success: false`, `ports_tested[]`, `excluded_assigned_ports[]`, `error`, `suggestion`.

### Detect-baud request/response

**Request:** `port_path`, `timeout_seconds` (default 10).

**Success response:** `success`, `port_path`, `detected_baud_rate`, `test_duration_ms`, `rates_tested[]`, `message`, `sample_data`.

### Auto-detection algorithm

1. **Enumerate devices** via `GET /api/serial/devices` (unassigned ports only)
2. **Prioritise** by USB metadata (known OPS243 vendor/product IDs) and stable names (`/dev/serial/by-id/*`)
3. **Probe each port** at [9600, 19200, 38400, 57600, 115200] with safe query commands (`??`, `I?`)
4. **First match wins:** return working combination with diagnostic data
5. **On failure:** return actionable suggestion and list of ports tested/excluded

**Implementation location (planned):** `internal/api/serial_test.go` (same file as FR3)

**UX benefit:** "Detect Device" populates the form automatically; "Auto-Detect Baud" works when the port is already known.
