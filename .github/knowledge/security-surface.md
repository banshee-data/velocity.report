# Security Surface

Canonical attack surface map for velocity.report. Shared reference for all agents; Malory owns the methodology but any agent may consult this during review.

## Network

### Go HTTP API (port 8080)

- Endpoint security and input validation
- Rate limiting and CORS configuration
- WebSocket streaming security
- Currently no authentication (local network only) — API key auth planned

### LIDAR UDP Listener (192.168.100.151)

- Packet validation and flood resilience
- Spoofing and replay attacks
- Network isolation requirements

## Hardware

### Radar Serial (/dev/ttyUSB0)

- Command injection via serial interface
- Buffer overflows in serial parsing
- Device spoofing
- Privilege escalation via device permissions

### Sensor Configuration

- Configuration tampering
- Firmware update surface
- Physical access threats

## Storage

### SQLite (/var/lib/velocity-report/sensor_data.db)

- SQL injection (verify parameterised queries are used throughout)
- File permissions and ownership
- Corruption via malformed sensor data
- Data exfiltration (local file access)
- Backup exposure

### Filesystem

- Path traversal (especially in PDF output paths)
- Symlink attacks
- Permission escalation
- Temp file leaks (LaTeX intermediate files)

## Dependencies

Check for known CVEs and supply chain risk:

- `go.mod` — Go modules
- `requirements.txt` — Python packages
- `package.json` — npm packages

`make lint` runs formatting and style checks for Go, Python, web, and documentation code.

## Vulnerability Patterns

### Input Validation

High-risk parse points (priority order):

1. Radar JSON parsing (`internal/radar/`)
2. LIDAR UDP packet parsing (`internal/lidar/`)
3. Serial command handling
4. API request bodies (oversized payloads, malformed JSON, path traversal)
5. Config file parsing (PDF generator, service config)

Test with: overflows, negatives, special characters, null bytes, UTF-8 edge cases, injection payloads.

### Authentication And Access

Questions to answer on every review:

- Is API authentication implemented? Is it enforced on every route?
- Are there privilege levels? Are they checked?
- Can an unauthenticated user reach sensor data?
- Are default credentials present?

### Privacy

Verify these claims hold — they are the project's core promise:

- No licence plate data collected
- No camera/video recording
- No PII in database
- Data stays local

Then look for what the claims don't cover:

- Timing-based vehicle re-identification
- Metadata leakage (sensor location in exports)
- PII in debug logs or error messages
- Data correlation with external sources

**PII is the hard line.** If PII reaches a log, a response body, or an export, the finding is CRITICAL severity.

### Code Execution

High-risk areas:

- LaTeX injection in PDF generation
- Shell commands in scripts
- Deserialisation
- Template injection
- Local privilege escalation via systemd misconfiguration, SUID binaries, file permission gaps

### Denial Of Service

Test:

- API request flooding and large payloads
- Database disk exhaustion
- Memory exhaustion via sensor data streams
- CPU-heavy queries
- Crash paths via malformed sensor data, invalid queries, null derefs, uncaught panics
