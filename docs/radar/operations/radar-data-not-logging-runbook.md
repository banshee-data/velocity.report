# Runbook: radar_data Not Logging

Production server is running but no rows are appearing in the `radar_data`
table. This runbook walks through every layer of the ingestion pipeline from
physical hardware to SQLite INSERT.

## Pipeline Overview

```
┌──────────────────────────────────────────────────────────────┐
│  OPS243-A Radar Sensor                                       │
│  USB-to-serial → /dev/ttySC1 (or /dev/ttyUSB0)              │
│  19200 baud, 8N1                                             │
└──────────┬───────────────────────────────────────────────────┘
           │ JSON lines over serial
           ▼
┌──────────────────────────────────────────────────────────────┐
│  SerialMux.Monitor()                                         │
│  bufio.Scanner reads lines, broadcasts to subscriber chans   │
└──────────┬───────────────────────────────────────────────────┘
           │ string payloads via Go channel
           ▼
┌──────────────────────────────────────────────────────────────┐
│  serialmux.HandleEvent(db, payload)                          │
│  ClassifyPayload() → EventTypeRawData / RadarObject / Config │
└──────────┬───────────────────────────────────────────────────┘
           │
     ┌─────┴──────┐
     ▼            ▼
  RawData    RadarObject
     │            │
     ▼            ▼
  INSERT INTO  INSERT INTO
  radar_data   radar_objects
  (raw_event)  (raw_event)
```

A failure at **any** layer silently breaks ingestion. Work from the bottom up
(database) to the top (hardware) to isolate.

---

## Diagnostic Checklist

### 1. Service and Process Health

```bash
# Is the service running?
systemctl is-active velocity-report.service

# Recent logs — look for "failed to create radar port" or "error handling event"
journalctl -u velocity-report.service -n 200 --no-pager

# Confirm the ExecStart line includes the correct --port flag
systemctl cat velocity-report.service | grep ExecStart

# Confirm the process is actually running and not crash-looping
systemctl show velocity-report.service -p NRestarts --value
journalctl -u velocity-report.service --since "1 hour ago" | grep -c "Started Velocity"
```

**What to look for:**

| Log message | Meaning |
|---|---|
| `failed to create radar port: …` | Serial port could not be opened — jump to §2 |
| `failed to initialise device: …` | Port opened but init commands failed — jump to §4 |
| `error handling event: …` | Parse or DB insert failure — jump to §5/§6 |
| `subscribe routine terminated` | Context cancelled (normal shutdown) |
| No radar-related log lines at all | `--disable-radar` flag may be set, or mock/fixture mode |

**Check for accidental headless mode:**

```bash
# If ExecStart contains --disable-radar, no serial I/O occurs
systemctl cat velocity-report.service | grep -E '\-\-disable-radar|\-\-debug|\-\-fixture'
```

If `--disable-radar` is present, the `DisabledSerialMux` is used — it never
reads from hardware. Remove the flag and restart.

---

### 2. Serial Device Exists and Has Correct Permissions

```bash
# Check the device node exists
ls -la /dev/ttySC1 /dev/ttyUSB0 /dev/ttyACM0 2>/dev/null

# Check USB enumeration
dmesg | tail -50 | grep -i -E 'tty|usb|serial|ftdi|ch340|cp210|cdc_acm'

# Full USB device tree
lsusb

# Check the service user can access the port
# (the service runs as the user shown by this command)
SERVICE_USER=$(systemctl show velocity-report.service -p User --value)
echo "Service user: ${SERVICE_USER:-velocity}"

# Check group membership — user must be in 'dialout' (Debian/Ubuntu) or 'uucp' (Arch)
id "${SERVICE_USER:-velocity}"
ls -la /dev/ttySC1 2>/dev/null   # Check device group (usually 'dialout')

# If group is missing:
# sudo usermod -a -G dialout ${SERVICE_USER:-velocity}
# Then restart the service (group change requires new session)
```

**Common failures:**

| Symptom | Cause | Fix |
|---|---|---|
| Device node missing | USB cable disconnected, sensor unpowered, driver not loaded | Reseat cable, check power, `dmesg` |
| `Permission denied` | Service user not in `dialout` group | `sudo usermod -a -G dialout <user>` + restart |
| `/dev/ttyUSB0` expected but `/dev/ttyUSB1` present | Multiple USB-serial adapters, device enumeration changed | Update `--port` flag in service file |
| Device is `/dev/ttyACM0` not `/dev/ttyUSB0` | Different USB-serial chipset (CDC ACM vs FTDI/CH340) | Update `--port` flag |

---

### 3. Direct Serial Validation via stty and screen

**Stop the service first** — only one process can hold a serial port:

```bash
sudo systemctl stop velocity-report.service
```

#### 3a. Verify port parameters with stty

```bash
# Show current serial port settings
stty -F /dev/ttySC1 -a

# Expected values:
#   speed 19200 baud
#   -parenb (no parity)
#   cs8 (8 data bits)
#   -cstopb (1 stop bit)

# If baud rate is wrong, set it explicitly:
stty -F /dev/ttySC1 19200 cs8 -cstopb -parenb raw -echo
```

#### 3b. Raw byte-level read with cat (quick sanity check)

```bash
# Read raw bytes for 10 seconds — should see JSON lines
timeout 10 cat /dev/ttySC1

# If nothing appears:
# - Sensor may not be powered
# - Baud rate mismatch (garbled output = wrong baud)
# - Sensor in idle mode (no targets detected, no output)
```

**Interpreting garbled output:** If you see random binary characters instead
of JSON, the baud rate is wrong. The OPS243-A defaults to 19200 but can be
changed via the `I` command. Try other common rates:

```bash
for BAUD in 9600 19200 38400 57600 115200; do
  echo "=== Testing $BAUD ==="
  stty -F /dev/ttySC1 $BAUD cs8 -cstopb -parenb raw -echo
  timeout 3 cat /dev/ttySC1
  echo
done
```

#### 3c. Interactive session with screen

```bash
# Connect at the correct baud rate (19200, NOT 115200)
screen /dev/ttySC1 19200

# Once connected, type these commands (press Enter after each):
#   ??        → query current configuration (should echo JSON config)
#   OJ        → set JSON output mode
#   OS        → enable speed reporting
#   OM        → enable magnitude

# You should start seeing JSON lines like:
#   {"speed": 12.5, "magnitude": 456, "uptime": 12345.67}
#
# If a vehicle passes, you should also see radar object lines:
#   {"classifier":"object_inbound","end_time":"1750719826.467",...}

# Exit screen: Ctrl-A then K, confirm with Y
```

#### 3d. Send initialisation commands manually

If the sensor is in an unknown state, replicate what the application does on
startup:

```bash
stty -F /dev/ttySC1 19200 cs8 -cstopb -parenb raw -echo

# Reset to factory defaults
echo "AX" > /dev/ttySC1
sleep 1

# Set JSON output
echo "OJ" > /dev/ttySC1
sleep 0.5

# Enable speed (doppler)
echo "OS" > /dev/ttySC1
sleep 0.5

# Enable range (FMCW)
echo "oD" > /dev/ttySC1
sleep 0.5

# Enable magnitude
echo "OM" > /dev/ttySC1
echo "oM" > /dev/ttySC1
sleep 0.5

# Now read — should see JSON
timeout 10 cat /dev/ttySC1
```

**Remember to restart the service after testing:**

```bash
sudo systemctl start velocity-report.service
```

---

### 4. Application-Level Serial Monitor Check

If the device produces data (§3 passes) but the database is still empty, the
problem is between `SerialMux.Monitor()` and the DB insert.

#### 4a. Check the live tail endpoint

While the service is running, use the built-in debug endpoint:

```bash
# SSE stream of raw serial lines (requires localhost or Tailscale access)
curl -N http://127.0.0.1:8080/debug/tail
```

If you see JSON lines streaming here, the serial monitor is working. The
problem is downstream (classification or DB insert).

If `/debug/tail` shows nothing:
- The serial port may have been opened but the scanner is blocked
- The subscriber channel may be full (buffer size is 1) and lines are being
  dropped — but this would affect `/debug/tail` equally, so it more likely
  means the scanner goroutine is stuck

#### 4b. Send a command via the debug API

```bash
# Send the query command to the radar
curl -X POST http://127.0.0.1:8080/debug/send-command-api -d "command=??"

# This confirms writes to the serial port work
# Then check /debug/tail for the response
```

#### 4c. Check journalctl for event handling

Every raw data line is logged by the handler:

```bash
# These log lines come from HandleRawData() and HandleRadarObject()
journalctl -u velocity-report.service --since "5 min ago" --no-pager | grep -E "Raw Data Line|Raw RadarObject Line|error handling event|unknown event type"
```

| Pattern found | Meaning |
|---|---|
| `Raw Data Line: {"speed":…}` | Data is reaching the handler — DB insert might be failing silently |
| `Raw RadarObject Line: {"classifier":…}` | Object events are being processed |
| `error handling event: …` | Handler error — check the specific error message |
| `unknown event type: …` | Payload did not match any classifier — see §5 |
| Nothing at all | Lines are not reaching the subscriber goroutine |

---

### 5. Payload Shape and Semantic Validation

The `ClassifyPayload()` function in `internal/serialmux/parse.go` uses simple
string matching to route payloads:

| Condition | Classification | Destination |
|---|---|---|
| Contains `"end_time"` OR `"classifier"` | `EventTypeRadarObject` | `radar_objects` table |
| Contains `"magnitude"` OR `"speed"` | `EventTypeRawData` | `radar_data` table |
| Starts with `{` | `EventTypeConfig` | In-memory state only |
| None of the above | `EventTypeUnknown` | Logged and discarded |

#### 5a. Validate a sample payload

Capture a line from the sensor and check it manually:

```bash
# Capture one line (service must be stopped)
sudo systemctl stop velocity-report.service
LINE=$(timeout 10 head -1 /dev/ttySC1)
echo "$LINE"
sudo systemctl start velocity-report.service

# Check: is it valid JSON?
echo "$LINE" | python3 -m json.tool 2>&1 || echo "NOT VALID JSON"

# Check: does it contain the expected fields?
echo "$LINE" | python3 -c "
import sys, json
try:
    d = json.load(sys.stdin)
    has_speed = 'speed' in d
    has_mag   = 'magnitude' in d
    has_end   = 'end_time' in d
    has_cls   = 'classifier' in d
    if has_end or has_cls:
        print(f'→ RadarObject: classifier={d.get(\"classifier\")}, end_time={d.get(\"end_time\")}')
    elif has_speed or has_mag:
        print(f'→ RawData: speed={d.get(\"speed\")}, magnitude={d.get(\"magnitude\")}')
    else:
        print(f'→ UNCLASSIFIED (Config?): keys={list(d.keys())}')
except Exception as e:
    print(f'→ PARSE ERROR: {e}')
"
```

#### 5b. Expected payload shapes

**RawData** (goes to `radar_data`):

```json
{"speed": 12.5, "magnitude": 456, "uptime": 98765.432}
```

Required fields for classification: at least one of `speed` or `magnitude`.
The `raw_event` column stores the entire JSON blob. Generated columns extract
`speed`, `magnitude`, and `uptime` via `JSON_EXTRACT()`.

**RadarObject** (goes to `radar_objects`):

```json
{
  "classifier": "object_inbound",
  "start_time": "1750719826.031",
  "end_time": "1750719826.467",
  "delta_time_msec": 736,
  "max_speed_mps": 13.39,
  "min_speed_mps": 11.33,
  "max_magnitude": 55,
  "avg_magnitude": 36,
  "total_frames": 7,
  "frames_per_mps": 0.5228,
  "length_m": 9.86,
  "speed_change": 2.799
}
```

Required for classification: `end_time` or `classifier` present in the string.

#### 5c. Common payload problems

| Problem | Symptom | Fix |
|---|---|---|
| Sensor not in JSON mode | Raw text like `S:-12.5\nM:456` | Send `OJ` command |
| Empty lines | Blank strings → `EventTypeUnknown` | Normal between detections; only a problem if ALL lines are empty |
| Truncated JSON | `{"speed": 12.5, "magnitu` | Check for electrical noise, loose cable, or scanner buffer overflow |
| Config echo lines | `{"mode": "OJ"}` → classified as Config | Normal — init command responses. Not stored in `radar_data` |
| No speed/magnitude fields | `{"uptime": 12345}` → `EventTypeConfig` | Sensor output modes not enabled; send `OS` and `OM` |

---

### 6. Database Checks

#### 6a. Is data actually missing?

```bash
DB=/var/lib/velocity-report/sensor_data.db

# Row count
sqlite3 "$DB" "SELECT COUNT(*) FROM radar_data;"

# Most recent entry
sqlite3 "$DB" "SELECT data_id, datetime(write_timestamp, 'unixepoch', 'localtime'), speed, magnitude FROM radar_data ORDER BY data_id DESC LIMIT 5;"

# Data range
sqlite3 "$DB" "SELECT datetime(MIN(write_timestamp), 'unixepoch', 'localtime') AS first, datetime(MAX(write_timestamp), 'unixepoch', 'localtime') AS latest FROM radar_data;"

# Also check radar_objects
sqlite3 "$DB" "SELECT COUNT(*) FROM radar_objects;"
sqlite3 "$DB" "SELECT datetime(MAX(write_timestamp), 'unixepoch', 'localtime') FROM radar_objects;"
```

#### 6b. Can the database accept writes?

```bash
# Integrity check
sqlite3 "$DB" "PRAGMA integrity_check;"

# Is WAL mode active? (should be)
sqlite3 "$DB" "PRAGMA journal_mode;"

# Check for locks
lsof "$DB"*

# Check disk space
df -h /var/lib/velocity-report/

# Check file permissions
ls -la "$DB"*
# Expected: owned by service user (velocity or the configured user)
```

#### 6c. Test INSERT directly

```bash
# Test insert (use a distinctive test payload)
sqlite3 "$DB" "INSERT INTO radar_data (raw_event) VALUES ('{\"speed\": -999, \"magnitude\": -999, \"test\": true}');"

# Verify it was written
sqlite3 "$DB" "SELECT * FROM radar_data WHERE speed = -999;"

# Clean up test row
sqlite3 "$DB" "DELETE FROM radar_data WHERE speed = -999;"
```

If the INSERT fails, check:
- Disk space (`df -h`)
- WAL corruption (`PRAGMA wal_checkpoint(TRUNCATE);`)
- Schema drift (`sqlite3 "$DB" ".schema radar_data"`)

#### 6d. Schema verification

```bash
# Confirm the radar_data table has the expected structure
sqlite3 "$DB" ".schema radar_data"

# Expected output:
# CREATE TABLE IF NOT EXISTS "radar_data" (
#     data_id INTEGER PRIMARY KEY AUTOINCREMENT
#   , write_timestamp DOUBLE DEFAULT (UNIXEPOCH('subsec'))
#   , raw_event JSON NOT NULL
#   , uptime DOUBLE AS (JSON_EXTRACT(raw_event, '$.uptime')) STORED
#   , magnitude DOUBLE AS (JSON_EXTRACT(raw_event, '$.magnitude')) STORED
#   , speed DOUBLE AS (JSON_EXTRACT(raw_event, '$.speed')) STORED
# );

# Check migration state
velocity-report migrate --db-path "$DB" status
```

---

### 7. Network and Firewall (if applicable)

The radar uses a **local serial connection**, not a network socket. However,
check that nothing interferes with the API layer reporting data:

```bash
# Can the API serve data?
curl -s "http://127.0.0.1:8080/api/events?start=0" | python3 -m json.tool | head -20

# Current config state (includes device settings echoed from sensor)
curl -s http://127.0.0.1:8080/api/config | python3 -m json.tool
```

---

## Repair Procedures

### R1. Serial Port Permission Fix

```bash
SERVICE_USER=$(systemctl show velocity-report.service -p User --value)
SERVICE_USER=${SERVICE_USER:-velocity}

sudo usermod -a -G dialout "$SERVICE_USER"
sudo systemctl restart velocity-report.service
```

### R2. Sensor Factory Reset and Re-Initialisation

```bash
sudo systemctl stop velocity-report.service

stty -F /dev/ttySC1 19200 cs8 -cstopb -parenb raw -echo

# Full factory reset
echo "AX" > /dev/ttySC1
sleep 2

# Set JSON mode
echo "OJ" > /dev/ttySC1
sleep 0.5

# Enable all needed outputs
echo "OS" > /dev/ttySC1    # Speed (doppler)
echo "oD" > /dev/ttySC1    # Range (FMCW)
echo "OM" > /dev/ttySC1    # Magnitude (speed)
echo "oM" > /dev/ttySC1    # Magnitude (range)
echo "OH" > /dev/ttySC1    # Timestamps
echo "OC" > /dev/ttySC1    # Object detection
sleep 1

# Verify output
timeout 10 cat /dev/ttySC1

sudo systemctl start velocity-report.service
```

### R3. Baud Rate Mismatch Recovery

If the sensor's baud rate was changed (e.g. someone sent `I1`, `I2`, `I4`, or
`I5` to set 9600, 19200, 57600, or 115200 respectively):

```bash
sudo systemctl stop velocity-report.service

# Probe all common baud rates to find the current one
for BAUD in 9600 19200 38400 57600 115200; do
  echo "--- $BAUD ---"
  stty -F /dev/ttySC1 $BAUD cs8 -cstopb -parenb raw -echo
  timeout 3 cat /dev/ttySC1
  echo
done

# Once you find the rate that produces readable text, set it and
# reset the sensor to 19200:
stty -F /dev/ttySC1 <CURRENT_BAUD> cs8 -cstopb -parenb raw -echo
echo "I2" > /dev/ttySC1     # I2 = set baud to 19200
sleep 1

# Now switch to 19200 and confirm
stty -F /dev/ttySC1 19200 cs8 -cstopb -parenb raw -echo
timeout 5 cat /dev/ttySC1   # Should produce readable output

sudo systemctl start velocity-report.service
```

### R4. Database Recovery

```bash
DB=/var/lib/velocity-report/sensor_data.db

# Stop the service first
sudo systemctl stop velocity-report.service

# 1. Backup
cp "$DB" "$DB.backup.$(date +%Y%m%d-%H%M%S)"

# 2. Check integrity
sqlite3 "$DB" "PRAGMA integrity_check;"

# 3. If WAL is corrupt, checkpoint and truncate
sqlite3 "$DB" "PRAGMA wal_checkpoint(TRUNCATE);"

# 4. If schema is missing or corrupted, run migrations
velocity-report migrate --db-path "$DB" status
velocity-report migrate --db-path "$DB" up

# 5. Restart
sudo systemctl start velocity-report.service
```

### R5. Device Path Changed (USB Re-enumeration)

If the device moved from `/dev/ttyUSB0` to `/dev/ttyUSB1` (or similar):

```bash
# Find the current device
ls -la /dev/ttyUSB* /dev/ttyACM* /dev/ttySC* 2>/dev/null
dmesg | grep -i tty | tail -10

# Update the service file
sudo systemctl edit velocity-report.service
# Add under [Service]:
#   ExecStart=
#   ExecStart=/usr/local/bin/velocity-report --port /dev/ttyUSB1 --db-path ...

sudo systemctl daemon-reload
sudo systemctl restart velocity-report.service
```

For a permanent fix, create a udev rule to give the OPS243 a stable symlink:

```bash
# Find the USB attributes
udevadm info -a -n /dev/ttyUSB0 | grep -E 'idVendor|idProduct|serial'

# Create a udev rule (adjust vendor/product ID)
sudo tee /etc/udev/rules.d/99-ops243.rules << 'EOF'
SUBSYSTEM=="tty", ATTRS{idVendor}=="0403", ATTRS{idProduct}=="6001", SYMLINK+="radar0"
EOF

sudo udevadm control --reload-rules
sudo udevadm trigger

# Then update --port to /dev/radar0
```

### R6. Subscriber Channel Backpressure

The subscriber channel has a buffer size of 1. If the event handler goroutine
is slow (e.g. due to a slow DB insert under disk I/O pressure), the
`SerialMux.Monitor()` loop will **silently drop** lines for that subscriber:

```go
select {
case ch <- line:
default:
    // dropped — channel full
}
```

**Diagnosis:**

```bash
# Check disk I/O wait
iostat -x 1 5

# Check SQLite WAL file size (large WAL = slow checkpoints)
ls -lh /var/lib/velocity-report/sensor_data.db*

# Force a WAL checkpoint
sqlite3 /var/lib/velocity-report/sensor_data.db "PRAGMA wal_checkpoint(PASSIVE);"
```

**Fix:** Reduce I/O contention. On Raspberry Pi with SD card:
- Ensure WAL mode is active (`PRAGMA journal_mode;` should return `wal`)
- Consider moving the database to a USB SSD
- Reduce transit worker frequency if it competes for write locks

---

## Quick Decision Tree

```
radar_data is empty or stale
│
├── Is the service running?
│   ├── NO  → systemctl start velocity-report.service → check logs
│   └── YES ↓
│
├── Is --disable-radar set?
│   ├── YES → remove flag, restart
│   └── NO  ↓
│
├── Does the device node exist? (ls /dev/ttySC1 or /dev/ttyUSB0)
│   ├── NO  → check USB cable, power, dmesg
│   └── YES ↓
│
├── Can the service user read the device?
│   ├── NO  → add user to dialout group
│   └── YES ↓
│
├── Does `timeout 5 cat /dev/ttySC1` produce readable JSON?
│   ├── NO, garbled → baud rate mismatch (§R3)
│   ├── NO, nothing → sensor not outputting (§R2 factory reset)
│   └── YES ↓
│
├── Does /debug/tail show live lines?
│   ├── NO  → serial monitor stuck or port opened by another process
│   └── YES ↓
│
├── Do journal logs show "Raw Data Line" entries?
│   ├── NO  → ClassifyPayload returning wrong type (§5)
│   └── YES ↓
│
├── Do journal logs show "error handling event"?
│   ├── YES → DB insert failing (§6)
│   └── NO  ↓
│
└── Data should be flowing — check DB directly (§6a)
    └── If rows exist but are old → possible channel backpressure (§R6)
```

---

## Notes

- **Baud rate is 19200**, not 115200. The OPS243-A factory default is 19200.
  The existing TROUBLESHOOTING.md incorrectly references 115200 in some places.
- The **subscriber channel buffer is 1**. Under heavy load or slow disk, lines
  are silently dropped with no log message. This is the most insidious failure
  mode.
- The `Initialise()` method sends `AX` (factory reset) on every service start.
  This means the sensor's output mode is reset each time. If `Initialise()`
  fails partway, the sensor may be in a partial-config state.
- The `ClassifyPayload()` function uses **string contains**, not JSON parsing.
  A payload like `{"note": "speed limit sign"}` would incorrectly classify as
  `EventTypeRawData` because it contains the string `"speed"`. This is by
  design (conservative, fast) but worth noting for debugging edge cases.
