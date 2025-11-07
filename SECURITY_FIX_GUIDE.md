# Security Vulnerability Quick Reference

## 🚨 STOP - READ THIS FIRST

**Status:** NOT READY FOR PRODUCTION  
**Critical Issues:** 3  
**Estimated Fix Time:** ~9 hours

---

## Critical Issue #1: LaTeX Injection

**File:** `tools/pdf-generator/pdf_generator/core/document_builder.py`  
**Lines:** 196, 222, 225-226

### The Problem
```python
# VULNERABLE CODE:
doc.append(NoEscape(f"\\fancyhead[R]{{ \\textit{{{location}}}}}"))
```

User can inject: `"location": "\\input{/etc/passwd}"`  
Result: File contents leaked in PDF

### The Fix
```python
from pylatex.utils import escape_latex

# SECURE CODE:
escaped_location = escape_latex(location)
doc.append(NoEscape(f"\\fancyhead[R]{{ \\textit{{{escaped_location}}}}}"))
```

### Files to Modify
1. `tools/pdf-generator/pdf_generator/core/document_builder.py`
   - Line 196: Escape `location` in header
   - Line 222: Escape `location` in title
   - Line 225-226: Escape `surveyor` and `contact`

### Test Fix
```bash
./security-tests/test-latex-injection.sh
# Should return: ✅ PROTECTED
```

---

## Critical Issue #2: No Authentication

**File:** `internal/api/server.go`  
**All endpoints affected**

### The Problem
```go
// CURRENT CODE - NO AUTH:
mux.HandleFunc("/api/events", s.listEvents)
```

Anyone can access: `curl http://server:8080/api/events`

### The Fix
```go
// ADD THIS FUNCTION:
func (s *Server) BasicAuthMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        user, pass, ok := r.BasicAuth()
        if !ok || user != s.apiUser || pass != s.apiPassword {
            w.Header().Set("WWW-Authenticate", `Basic realm="velocity.report"`)
            http.Error(w, "Unauthorized", http.StatusUnauthorized)
            return
        }
        next.ServeHTTP(w, r)
    })
}

// WRAP ALL API ROUTES:
mux.Handle("/api/", s.BasicAuthMiddleware(apiRouter))
```

### Files to Modify
1. `internal/api/server.go`
   - Add `apiUser` and `apiPassword` fields to Server struct
   - Add BasicAuthMiddleware function
   - Wrap all `/api/*` routes with middleware

2. `cmd/radar/main.go`
   - Add `--api-user` and `--api-password` CLI flags
   - Pass to NewServer()

3. `velocity-report.service`
   - Add environment variables for credentials

### Test Fix
```bash
./security-tests/test-authentication.sh
# Should return: ✅ PROTECTED
```

---

## Critical Issue #3: No Rate Limiting

**File:** `internal/api/server.go`  
**All endpoints affected**

### The Problem
```go
// CURRENT CODE - NO LIMITS:
mux.HandleFunc("/api/sites/reports", s.generateReport)
```

Attacker can send 1000s of requests → system crash

### The Fix
```go
import "golang.org/x/time/rate"

// ADD TO Server STRUCT:
type Server struct {
    // ... existing fields
    globalLimiter *rate.Limiter  // 100 req/s
    pdfLimiter    *rate.Limiter  // 1 req/min
}

// IN NewServer():
globalLimiter: rate.NewLimiter(100, 200),
pdfLimiter:    rate.NewLimiter(1, 2),

// ADD MIDDLEWARE:
func (s *Server) RateLimitMiddleware(limiter *rate.Limiter) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if !limiter.Allow() {
                http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}

// APPLY TO ROUTES:
mux.Handle("/api/sites/reports", 
    s.RateLimitMiddleware(s.pdfLimiter)(
        s.BasicAuthMiddleware(http.HandlerFunc(s.generateReport)),
    ),
)
```

### Files to Modify
1. `internal/api/server.go`
   - Add rate limiter fields to Server struct
   - Initialize limiters in NewServer
   - Add RateLimitMiddleware function
   - Apply to all routes (especially PDF generation)

### Test Fix
```bash
./security-tests/test-rate-limiting.sh
# Should return: ✅ PROTECTED
```

---

## Testing Checklist

After implementing fixes:

- [ ] Run `./security-tests/run-all-tests.sh`
- [ ] All 3 tests should pass
- [ ] Verify authentication with curl:
  ```bash
  # Should fail:
  curl http://localhost:8080/api/events
  
  # Should succeed:
  curl -u admin:password http://localhost:8080/api/events
  ```
- [ ] Verify rate limiting:
  ```bash
  # First request succeeds, subsequent rapid requests fail
  for i in {1..10}; do 
    curl -u admin:password -X POST http://localhost:8080/api/sites/reports \
      -H "Content-Type: application/json" \
      -d '{"start_date":"2024-01-01","end_date":"2024-12-31"}'
  done
  ```
- [ ] Verify LaTeX escaping:
  ```bash
  # PDF should contain literal text "\\input{...}", not file contents
  curl -u admin:password -X POST http://localhost:8080/api/sites/reports \
    -H "Content-Type: application/json" \
    -d '{"location":"\\input{/etc/passwd}","start_date":"2024-01-01","end_date":"2024-12-31"}'
  ```

---

## Priority Order

1. **LaTeX Injection** (2 hours) - Prevents remote code execution
2. **Authentication** (4 hours) - Prevents unauthorized access
3. **Rate Limiting** (3 hours) - Prevents denial of service

**Total:** ~9 hours

---

## Questions?

- Full details: `SECURITY_AUDIT_REPORT.md`
- Quick summary: `LAUNCH_BLOCKERS.md`
- Test suite: `security-tests/README.md`

---

## Sign-Off

After fixes are complete and all tests pass:

- [ ] Developer: Fixed all critical issues
- [ ] Tester: All security tests pass
- [ ] Security: Re-audited and approved
- [ ] Manager: Approved for production deployment

**Until all checkboxes are checked: DO NOT DEPLOY**
