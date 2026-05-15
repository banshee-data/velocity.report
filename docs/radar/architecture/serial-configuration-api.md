# Serial configuration API endpoints

- **Status:** Active
- **Parent:** [serial-configuration-ui.md](serial-configuration-ui.md)
- **Implementation plan:** [../../plans/serial-configuration-implementation-plan.md](../../plans/serial-configuration-implementation-plan.md)

API reference for the serial configuration surface in the current implementation. This document separates the endpoints that exist today from the endpoints that remain planned.

## Current endpoints

**Purpose:** Manage saved serial configurations, inspect supported models, list candidate ports, test a connection, and reload the active mux.

**Endpoints:**

| #   | Method   | Path                      | Purpose                                                 |
| --- | -------- | ------------------------- | ------------------------------------------------------- |
| 1   | `GET`    | `/api/serial/configs`     | List all serial configurations                          |
| 2   | `GET`    | `/api/serial/configs/:id` | Get single configuration                                |
| 3   | `POST`   | `/api/serial/configs`     | Create configuration                                    |
| 4   | `PUT`    | `/api/serial/configs/:id` | Update configuration                                    |
| 5   | `DELETE` | `/api/serial/configs/:id` | Delete configuration (returns `204`)                    |
| 6   | `GET`    | `/api/serial/devices`     | List available serial devices (excludes assigned ports) |
| 7   | `GET`    | `/api/serial/models`      | List sensor models from application code                |
| 8   | `POST`   | `/api/serial/test`        | Test a port with the provided serial settings           |
| 9   | `POST`   | `/api/serial/reload`      | Reload the active serial mux from enabled DB config     |

### Response schemas

**Config object** (returned by endpoints 1-5):

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

**Create/Update request body:** `name`, `port_path`, `baud_rate`, `data_bits`, `stop_bits`, `parity`, `enabled`, `description`, `sensor_model`.

The current implementation expects a full object for `PUT`, not a sparse patch.

**Device object** (endpoint 6):

| Field           | Type    | Notes                                         |
| --------------- | ------- | --------------------------------------------- |
| `port_path`     | string  | e.g. `/dev/ttyUSB0`                           |
| `friendly_name` | string  | Friendly label derived from device name       |
| `vendor_id`     | string  | Present in the shape; not currently populated |
| `product_id`    | string  | Present in the shape; not currently populated |
| `last_seen`     | integer | Unix timestamp                                |

Enumerates serial ports via `serial.GetPortsList()` and supplements that list with supported `/dev` device nodes the upstream library misses (for example `ttySC*`) plus `/dev/serial/by-*` links. Filters out paths already saved in `radar_serial_config` and derives a friendly label from the device name.

**Sensor model object** (endpoint 7):

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

**Test response** (endpoint 8):

Success returns `success`, `port_path`, `baud_rate`, `test_duration_ms`, and optional `bytes_received`, `sample_data`, `raw_responses`, `message`, `suggestion`.

Failure still returns HTTP `200` with `success: false`, plus `error`, `message`, and optional `suggestion`.

**Reload response** (endpoint 9):

Success returns `success`, `message`, and a `config` snapshot describing the newly active runtime config.

### Error handling

| Status | Meaning                                                                   |
| ------ | ------------------------------------------------------------------------- |
| `200`  | Serial test completed, including negative test results (`success: false`) |
| `201`  | Serial configuration created                                              |
| `204`  | Serial configuration deleted                                              |
| `400`  | Invalid values or unsupported sensor model                                |
| `404`  | Config ID does not exist                                                  |
| `409`  | Name already exists (unique constraint)                                   |
| `500`  | Database or runtime error                                                 |
| `503`  | Reload endpoint called without a configured serial manager                |

## Serial test behaviour

`POST /api/serial/test` validates a serial port configuration before save or reload.

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

### Algorithm

1. Validate the requested serial options.
2. Open the requested port.
3. Send `??` and `I?` as safe read-oriented commands.
4. Capture raw responses, including plain text.
5. If `auto_correct_baud` is enabled, parse the `I?` response and return the detected rate in the response body.
6. Close the port and return diagnostics.

### Notes

- The current implementation reports the detected baud rate, but does not persist that correction back into the saved configuration automatically.
- When the tested port matches the current `SerialPortManager` snapshot, the response warns that the live connection may be disrupted.

## Reload endpoint

`POST /api/serial/reload` asks the installed `SerialPortManager` to reload the first enabled serial configuration from the database.

### Behaviour

1. Load enabled configs from `radar_serial_config`.
2. Pick the first enabled config.
3. Compare it to the current runtime snapshot.
4. If different, build a new mux and swap it in.
5. Return the active config snapshot in the response.

### Important limitation

The reload manager exists, but the current radar startup path does not yet prove that it is installed in production. Treat `/api/serial/reload` as implemented API surface with rollout work still pending.

## Deferred endpoints

These were part of the original design but are not present in the current implementation:

- `POST /api/serial/auto-detect`
- `POST /api/serial/detect-baud`

They remain tracked in [serial-configuration-implementation-plan.md](../../plans/serial-configuration-implementation-plan.md).
