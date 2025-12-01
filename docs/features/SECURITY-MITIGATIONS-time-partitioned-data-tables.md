# Security Mitigation Guide: Time-Partitioned Data Tables

**Purpose:** Actionable security fixes for developers implementing time-partitioned data tables  
**Last Updated:** 2025-12-01

---

## Quick Reference

| CVE ID | Vulnerability | Priority | Effort | Phase |
|--------|--------------|----------|--------|-------|
| CVE-2025-VR-001 | Unauthenticated APIs | 🔴 CRITICAL | 2-3 days | Before Phase 2 |
| CVE-2025-VR-002 | Path Traversal | 🔴 CRITICAL | 1-2 days | Before Phase 2 |
| CVE-2025-VR-003 | SQL Injection | 🟠 HIGH | 1 day | Before Phase 1 |
| CVE-2025-VR-004 | Resource Exhaustion | 🟠 HIGH | 2-3 days | Before Phase 4 |
| CVE-2025-VR-005 | Race Conditions | 🟠 HIGH | 3-4 days | Before Phase 1 |
| CVE-2025-VR-006 | USB Security | 🟠 HIGH | 2-3 days | Before Phase 3 |

---

## Implementation Order

### BEFORE Writing ANY Code

```go
// 1. Create security package
package security

// 2. Implement authentication
type APIKeyAuthenticator struct {
    hashedKey []byte
}

func (a *APIKeyAuthenticator) Authenticate(apiKey string) error {
    if err := bcrypt.CompareHashAndPassword(a.hashedKey, []byte(apiKey)); err != nil {
        return ErrUnauthorized
    }
    return nil
}

// 3. Implement path validator
func ValidatePartitionPath(path string) error {
    // Implementation below
}

// 4. Implement SQL sanitizer
func SanitizeAlias(alias string) (string, error) {
    // Implementation below
}
```

---

## Mitigation #1: Authentication (CVE-2025-VR-001)

### Problem
All API endpoints accessible without authentication

### Solution

**Step 1: Generate API Key**
```bash
# Generate secure API key during installation
openssl rand -base64 32 > /etc/velocity-report/api-key
chmod 600 /etc/velocity-report/api-key
```

**Step 2: Implement Authentication Middleware**
```go
// internal/api/auth.go
package api

import (
    "crypto/subtle"
    "net/http"
    "golang.org/x/crypto/bcrypt"
)

type AuthMiddleware struct {
    hashedAPIKey []byte
}

func NewAuthMiddleware(apiKey string) (*AuthMiddleware, error) {
    hashed, err := bcrypt.GenerateFromPassword([]byte(apiKey), bcrypt.DefaultCost)
    if err != nil {
        return nil, err
    }
    return &AuthMiddleware{hashedAPIKey: hashed}, nil
}

func (a *AuthMiddleware) Authenticate(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Extract API key from header
        apiKey := r.Header.Get("X-API-Key")
        if apiKey == "" {
            http.Error(w, "Unauthorized: API key required", http.StatusUnauthorized)
            return
        }
        
        // Verify API key (constant-time comparison)
        if err := bcrypt.CompareHashAndPassword(a.hashedAPIKey, []byte(apiKey)); err != nil {
            http.Error(w, "Unauthorized: Invalid API key", http.StatusUnauthorized)
            return
        }
        
        // Authentication successful
        next.ServeHTTP(w, r)
    })
}
```

**Step 3: Apply to All Endpoints**
```go
// internal/api/server.go
func (s *Server) SetupRoutes() {
    // Apply authentication middleware to ALL partition management routes
    authenticated := s.router.Group("/api")
    authenticated.Use(s.authMiddleware.Authenticate)
    
    authenticated.GET("/partitions", s.ListPartitions)
    authenticated.POST("/partitions/attach", s.AttachPartition)
    authenticated.POST("/partitions/detach", s.DetachPartition)
    authenticated.POST("/partitions/consolidate", s.ConsolidatePartitions)
    // ... all other endpoints
}
```

**Step 4: Configuration**
```go
// cmd/radar/main.go
func main() {
    // Load API key from secure location
    apiKey, err := os.ReadFile("/etc/velocity-report/api-key")
    if err != nil {
        log.Fatal("API key not found. Run: velocity-report init")
    }
    
    authMiddleware, err := api.NewAuthMiddleware(string(apiKey))
    if err != nil {
        log.Fatal("Failed to initialize authentication:", err)
    }
    
    server := api.NewServer(db, authMiddleware)
    server.Start()
}
```

**Testing:**
```bash
# Should fail without API key
curl http://localhost:8080/api/partitions
# Expected: 401 Unauthorized

# Should succeed with valid API key
API_KEY=$(cat /etc/velocity-report/api-key)
curl -H "X-API-Key: $API_KEY" http://localhost:8080/api/partitions
# Expected: 200 OK
```

---

## Mitigation #2: Path Validation (CVE-2025-VR-002)

### Problem
User-controlled paths can access arbitrary files

### Solution

```go
// internal/db/partition_security.go
package db

import (
    "fmt"
    "os"
    "path/filepath"
    "regexp"
    "strings"
)

const (
    AllowedPartitionDir = "/var/lib/velocity-report"
    PartitionFilePattern = `^[0-9]{4}-(0[1-9]|1[0-2]|Q[1-4])_data\.db$`
)

type PathValidator struct {
    allowedDir string
    pattern    *regexp.Regexp
}

func NewPathValidator() *PathValidator {
    return &PathValidator{
        allowedDir: AllowedPartitionDir,
        pattern:    regexp.MustCompile(PartitionFilePattern),
    }
}

func (v *PathValidator) ValidatePartitionPath(path string) error {
    // Step 1: Reject paths containing directory traversal
    if strings.Contains(path, "..") {
        return fmt.Errorf("directory traversal not allowed")
    }
    
    // Step 2: Resolve to absolute path and check for symlinks
    absPath, err := filepath.EvalSymlinks(path)
    if err != nil {
        return fmt.Errorf("cannot resolve path: %w", err)
    }
    
    // Step 3: Ensure path is under allowed directory
    if !strings.HasPrefix(absPath, v.allowedDir) {
        return fmt.Errorf("path outside allowed directory: %s", absPath)
    }
    
    // Step 4: Verify file exists
    info, err := os.Lstat(absPath)
    if err != nil {
        return fmt.Errorf("cannot stat file: %w", err)
    }
    
    // Step 5: Verify it's a regular file (not device, socket, etc.)
    if !info.Mode().IsRegular() {
        return fmt.Errorf("not a regular file")
    }
    
    // Step 6: Verify filename matches expected pattern
    filename := filepath.Base(absPath)
    if !v.pattern.MatchString(filename) {
        return fmt.Errorf("filename does not match partition pattern: %s", filename)
    }
    
    // Step 7: Verify it's a valid SQLite database
    if err := v.verifySQLiteDatabase(absPath); err != nil {
        return fmt.Errorf("not a valid SQLite database: %w", err)
    }
    
    return nil
}

func (v *PathValidator) verifySQLiteDatabase(path string) error {
    // Read first 16 bytes (SQLite magic header)
    f, err := os.Open(path)
    if err != nil {
        return err
    }
    defer f.Close()
    
    header := make([]byte, 16)
    if _, err := f.Read(header); err != nil {
        return err
    }
    
    // Check for SQLite magic string
    sqliteMagic := []byte("SQLite format 3\x00")
    if string(header) != string(sqliteMagic) {
        return fmt.Errorf("invalid SQLite header")
    }
    
    return nil
}
```

**Usage in API:**
```go
// internal/api/partitions.go
func (s *Server) AttachPartition(w http.ResponseWriter, r *http.Request) {
    var req AttachRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "Invalid request", http.StatusBadRequest)
        return
    }
    
    // CRITICAL: Validate path before any file operations
    validator := db.NewPathValidator()
    if err := validator.ValidatePartitionPath(req.Path); err != nil {
        http.Error(w, fmt.Sprintf("Invalid path: %v", err), http.StatusBadRequest)
        return
    }
    
    // Path is safe, proceed with attach
    // ...
}
```

**Testing:**
```bash
# Test directory traversal rejection
curl -X POST -H "X-API-Key: $API_KEY" http://localhost:8080/api/partitions/attach \
  -d '{"path": "/var/lib/velocity-report/../../../etc/passwd"}'
# Expected: 400 Bad Request "directory traversal not allowed"

# Test symlink rejection
ln -s /etc/shadow /var/lib/velocity-report/archives/evil.db
curl -X POST -H "X-API-Key: $API_KEY" http://localhost:8080/api/partitions/attach \
  -d '{"path": "/var/lib/velocity-report/archives/evil.db"}'
# Expected: 400 Bad Request "not a valid SQLite database"

# Test valid path acceptance
curl -X POST -H "X-API-Key: $API_KEY" http://localhost:8080/api/partitions/attach \
  -d '{"path": "/var/lib/velocity-report/archives/2024-01_data.db"}'
# Expected: 200 OK
```

---

## Mitigation #3: SQL Injection Prevention (CVE-2025-VR-003)

### Problem
ATTACH DATABASE and union views vulnerable to SQL injection

### Solution

```go
// internal/db/sql_security.go
package db

import (
    "fmt"
    "regexp"
    "strings"
)

var (
    // Only allow alphanumeric and underscore in aliases
    aliasPattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{0,31}$`)
    
    // SQL keywords that should not appear in aliases
    sqlKeywords = []string{
        "DROP", "DELETE", "INSERT", "UPDATE", "CREATE", "ALTER",
        "EXEC", "EXECUTE", "SCRIPT", "UNION", "SELECT",
    }
)

type SQLSanitizer struct{}

func NewSQLSanitizer() *SQLSanitizer {
    return &SQLSanitizer{}
}

func (s *SQLSanitizer) ValidateAlias(alias string) error {
    // Check length
    if len(alias) == 0 || len(alias) > 32 {
        return fmt.Errorf("alias must be 1-32 characters")
    }
    
    // Check pattern
    if !aliasPattern.MatchString(alias) {
        return fmt.Errorf("alias must start with letter and contain only alphanumeric and underscore")
    }
    
    // Check for SQL keywords
    upperAlias := strings.ToUpper(alias)
    for _, keyword := range sqlKeywords {
        if strings.Contains(upperAlias, keyword) {
            return fmt.Errorf("alias contains SQL keyword: %s", keyword)
        }
    }
    
    return nil
}

func (s *SQLSanitizer) QuotePath(path string) string {
    // Escape single quotes by doubling them
    return "'" + strings.ReplaceAll(path, "'", "''") + "'"
}

func (s *SQLSanitizer) QuoteIdentifier(identifier string) string {
    // Escape double quotes by doubling them
    return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func (s *SQLSanitizer) BuildAttachSQL(path, alias string) (string, error) {
    // Validate alias
    if err := s.ValidateAlias(alias); err != nil {
        return "", err
    }
    
    // Build SQL with proper escaping
    quotedPath := s.QuotePath(path)
    quotedAlias := s.QuoteIdentifier(alias)
    
    // Use read-only mode for additional safety
    sql := fmt.Sprintf("ATTACH DATABASE 'file:%s?mode=ro' AS %s", path, quotedAlias)
    
    return sql, nil
}
```

**Usage in Partition Manager:**
```go
// internal/db/partition_manager.go
func (pm *PartitionManager) AttachPartition(path, alias string) error {
    sanitizer := NewSQLSanitizer()
    
    // Build safe SQL
    sql, err := sanitizer.BuildAttachSQL(path, alias)
    if err != nil {
        return fmt.Errorf("invalid alias: %w", err)
    }
    
    // Execute with validated inputs
    _, err = pm.db.Exec(sql)
    return err
}
```

**Testing:**
```bash
# Test SQL injection in alias
curl -X POST -H "X-API-Key: $API_KEY" http://localhost:8080/api/partitions/attach \
  -d '{"path": "/var/lib/velocity-report/archives/2024-01.db", "alias": "m01; DROP TABLE radar_data; --"}'
# Expected: 400 Bad Request "alias contains SQL keyword"

# Test quote injection
curl -X POST -H "X-API-Key: $API_KEY" http://localhost:8080/api/partitions/attach \
  -d '{"path": "/var/lib/velocity-report/archives/2024-01.db", "alias": "m01'\'' OR 1=1 --"}'
# Expected: 400 Bad Request "invalid alias format"

# Test valid alias
curl -X POST -H "X-API-Key: $API_KEY" http://localhost:8080/api/partitions/attach \
  -d '{"path": "/var/lib/velocity-report/archives/2024-01.db", "alias": "m01_2024"}'
# Expected: 200 OK
```

---

## Mitigation #4: Resource Exhaustion Prevention (CVE-2025-VR-004)

### Problem
Unlimited concurrent operations can exhaust disk/CPU/memory

### Solution

```go
// internal/api/resource_limiter.go
package api

import (
    "context"
    "fmt"
    "sync"
    "time"
)

const (
    MaxConcurrentConsolidations = 1
    MaxConcurrentAttachments    = 10
    ConsolidationTimeout        = 30 * time.Minute
    MinFreeDiskSpacePercent     = 15
)

type ResourceLimiter struct {
    consolidationSem chan struct{}
    attachSem        chan struct{}
    mu               sync.Mutex
}

func NewResourceLimiter() *ResourceLimiter {
    return &ResourceLimiter{
        consolidationSem: make(chan struct{}, MaxConcurrentConsolidations),
        attachSem:        make(chan struct{}, MaxConcurrentAttachments),
    }
}

func (rl *ResourceLimiter) AcquireConsolidation() error {
    select {
    case rl.consolidationSem <- struct{}{}:
        return nil
    default:
        return fmt.Errorf("max concurrent consolidations reached")
    }
}

func (rl *ResourceLimiter) ReleaseConsolidation() {
    <-rl.consolidationSem
}

func (rl *ResourceLimiter) CheckDiskSpace(path string) error {
    var stat syscall.Statfs_t
    if err := syscall.Statfs(path, &stat); err != nil {
        return err
    }
    
    // Calculate free space percentage
    total := stat.Blocks * uint64(stat.Bsize)
    free := stat.Bavail * uint64(stat.Bsize)
    usedPercent := float64(total-free) / float64(total) * 100
    
    if usedPercent > (100 - MinFreeDiskSpacePercent) {
        return fmt.Errorf("insufficient disk space: %.1f%% used", usedPercent)
    }
    
    return nil
}

func (rl *ResourceLimiter) ExecuteWithTimeout(ctx context.Context, fn func() error) error {
    ctx, cancel := context.WithTimeout(ctx, ConsolidationTimeout)
    defer cancel()
    
    done := make(chan error, 1)
    go func() {
        done <- fn()
    }()
    
    select {
    case err := <-done:
        return err
    case <-ctx.Done():
        return fmt.Errorf("operation timeout")
    }
}
```

**Usage:**
```go
func (s *Server) ConsolidatePartitions(w http.ResponseWriter, r *http.Request) {
    // Check disk space BEFORE starting
    if err := s.resourceLimiter.CheckDiskSpace("/var/lib/velocity-report"); err != nil {
        http.Error(w, err.Error(), http.StatusInsufficientStorage)
        return
    }
    
    // Acquire consolidation slot (blocks if at limit)
    if err := s.resourceLimiter.AcquireConsolidation(); err != nil {
        http.Error(w, "Too many concurrent consolidations", http.StatusTooManyRequests)
        return
    }
    defer s.resourceLimiter.ReleaseConsolidation()
    
    // Execute with timeout
    err := s.resourceLimiter.ExecuteWithTimeout(r.Context(), func() error {
        return s.performConsolidation(req)
    })
    
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    w.WriteHeader(http.StatusOK)
}
```

---

## Mitigation #5: Race Condition Prevention (CVE-2025-VR-005)

### Problem
Concurrent rotation and detach operations cause data corruption

### Solution

```go
// internal/db/rotation_lock.go
package db

import (
    "database/sql"
    "fmt"
    "sync"
    "time"
)

type RotationLock struct {
    db     *sql.DB
    mu     sync.Mutex
    locked bool
}

func NewRotationLock(db *sql.DB) *RotationLock {
    // Create lock table if not exists
    db.Exec(`
        CREATE TABLE IF NOT EXISTS rotation_lock (
            lock_id INTEGER PRIMARY KEY DEFAULT 1,
            acquired_at REAL,
            expires_at REAL,
            CHECK (lock_id = 1)
        )
    `)
    
    return &RotationLock{db: db}
}

func (rl *RotationLock) Acquire(timeout time.Duration) error {
    rl.mu.Lock()
    defer rl.mu.Unlock()
    
    deadline := time.Now().Add(timeout)
    
    for time.Now().Before(deadline) {
        // Try to acquire lock
        result, err := rl.db.Exec(`
            INSERT OR REPLACE INTO rotation_lock (lock_id, acquired_at, expires_at)
            VALUES (1, UNIXEPOCH('subsec'), UNIXEPOCH('subsec') + ?)
            WHERE NOT EXISTS (
                SELECT 1 FROM rotation_lock 
                WHERE lock_id = 1 AND expires_at > UNIXEPOCH('subsec')
            )
        `, 300.0) // 5 minute expiry
        
        if err != nil {
            return err
        }
        
        rows, err := result.RowsAffected()
        if err != nil {
            return err
        }
        
        if rows > 0 {
            rl.locked = true
            return nil
        }
        
        // Wait before retry
        time.Sleep(100 * time.Millisecond)
    }
    
    return fmt.Errorf("timeout acquiring rotation lock")
}

func (rl *RotationLock) Release() error {
    rl.mu.Lock()
    defer rl.mu.Unlock()
    
    if !rl.locked {
        return nil
    }
    
    _, err := rl.db.Exec(`DELETE FROM rotation_lock WHERE lock_id = 1`)
    rl.locked = false
    return err
}

// Partition reference counting
type PartitionRefCount struct {
    alias    string
    refCount int32
    mu       sync.RWMutex
    detached bool
}

func (p *PartitionRefCount) AcquireRead() error {
    p.mu.RLock()
    defer p.mu.RUnlock()
    
    if p.detached {
        return fmt.Errorf("partition already detached")
    }
    
    atomic.AddInt32(&p.refCount, 1)
    return nil
}

func (p *PartitionRefCount) ReleaseRead() {
    atomic.AddInt32(&p.refCount, -1)
}

func (p *PartitionRefCount) CanDetach() bool {
    p.mu.Lock()
    defer p.mu.Unlock()
    
    return atomic.LoadInt32(&p.refCount) == 0
}

func (p *PartitionRefCount) MarkDetached() {
    p.mu.Lock()
    defer p.mu.Unlock()
    p.detached = true
}
```

**Usage:**
```go
func (pm *PartitionManager) RotatePartitions(month time.Time) error {
    lock := NewRotationLock(pm.db)
    
    // Acquire distributed lock
    if err := lock.Acquire(10 * time.Second); err != nil {
        return fmt.Errorf("cannot acquire rotation lock: %w", err)
    }
    defer lock.Release()
    
    // Perform rotation with lock held
    return pm.performRotation(month)
}

func (pm *PartitionManager) DetachPartition(alias string) error {
    refCount := pm.getPartitionRefCount(alias)
    
    // Wait for queries to complete (with timeout)
    timeout := time.After(30 * time.Second)
    ticker := time.NewTicker(100 * time.Millisecond)
    defer ticker.Stop()
    
    for {
        select {
        case <-timeout:
            return fmt.Errorf("timeout waiting for queries to complete")
        case <-ticker.C:
            if refCount.CanDetach() {
                refCount.MarkDetached()
                return pm.performDetach(alias)
            }
        }
    }
}
```

---

## Testing Checklist

### Authentication Testing
```bash
- [ ] Unauthenticated requests return 401
- [ ] Invalid API key returns 401
- [ ] Valid API key returns 200
- [ ] Brute force rate limiting works
```

### Path Validation Testing
```bash
- [ ] Directory traversal blocked (../)
- [ ] Symlink attacks blocked
- [ ] Absolute path escape blocked
- [ ] Only allowed directory accepted
- [ ] Invalid filename patterns rejected
```

### SQL Injection Testing
```bash
- [ ] Alias with SQL keywords rejected
- [ ] Quote injection blocked
- [ ] Union injection blocked
- [ ] Valid aliases work correctly
```

### Resource Exhaustion Testing
```bash
- [ ] Max concurrent consolidations enforced
- [ ] Disk space checks prevent operations
- [ ] Operations timeout after 30 minutes
- [ ] System remains responsive under load
```

### Race Condition Testing
```bash
- [ ] Concurrent rotations blocked by lock
- [ ] Detach waits for queries to complete
- [ ] No data corruption under concurrent load
- [ ] Idempotent operations work correctly
```

---

## Security Code Review Checklist

Before merging any PR:

```
Authentication:
✅ All endpoints require authentication
✅ API keys properly hashed (bcrypt)
✅ Constant-time comparison used
✅ Rate limiting implemented

Input Validation:
✅ All paths validated with PathValidator
✅ All aliases validated with SQLSanitizer
✅ Length limits enforced
✅ Type checking performed

SQL Safety:
✅ No string concatenation in SQL
✅ Identifiers properly quoted
✅ Read-only mode used where possible
✅ No user input in SQL without validation

Concurrency:
✅ Rotation uses distributed lock
✅ Detach checks reference count
✅ No race conditions in tests
✅ Timeouts on all operations

Resource Management:
✅ Disk space checked before operations
✅ Concurrent operations limited
✅ Timeouts prevent runaway operations
✅ Cleanup on errors
```

---

## Monitoring and Alerts

### Security Events to Monitor

```go
// Log these events for security monitoring
type SecurityEvent string

const (
    AuthFailure         SecurityEvent = "auth_failure"
    PathValidationError SecurityEvent = "path_validation_error"
    SQLValidationError  SecurityEvent = "sql_validation_error"
    ResourceExhausted   SecurityEvent = "resource_exhausted"
    UnauthorizedAccess  SecurityEvent = "unauthorized_access"
)

func LogSecurityEvent(event SecurityEvent, details map[string]interface{}) {
    // Send to security monitoring system
    log.WithFields(logrus.Fields{
        "event": event,
        "details": details,
        "severity": "security",
    }).Warn("Security event detected")
    
    // Alert if critical
    if ShouldAlert(event) {
        SendSecurityAlert(event, details)
    }
}
```

### Alert Thresholds

- **5+ auth failures in 1 minute** - Potential brute force
- **10+ path validation errors** - Potential path traversal scan
- **Resource exhaustion** - Potential DoS attack
- **Any SQL validation error** - Potential SQL injection attempt

---

## Next Steps

1. ✅ Implement authentication BEFORE starting Phase 1
2. ✅ Implement path validation in db package
3. ✅ Implement SQL sanitization in db package
4. ✅ Add resource limiter to API server
5. ✅ Add rotation lock to partition manager
6. ✅ Write comprehensive security tests
7. ✅ Conduct security code review
8. ✅ Perform penetration testing
9. ✅ Document security configuration
10. ✅ Train developers on secure coding practices

---

**Status:** Ready for secure implementation following these guidelines

