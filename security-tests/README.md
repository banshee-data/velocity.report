# Security Test Suite

This directory contains security tests to verify that critical vulnerabilities have been fixed.

## ⚠️ WARNING

**DO NOT run these tests against production systems without authorization.**

These tests attempt to exploit known vulnerabilities and may:
- Create malicious PDF files
- Flood the API with requests
- Trigger resource-intensive operations

## Prerequisites

- Server must be running (use `make dev-go` from repository root)
- `curl` and `bash` must be available
- Tests assume server is at `http://localhost:8080` (override with `API_URL` env var)

## Running Tests

### Run All Tests
```bash
cd /path/to/velocity.report
./security-tests/run-all-tests.sh
```

### Run Individual Tests
```bash
# Test for missing authentication
./security-tests/test-authentication.sh

# Test for missing rate limiting
./security-tests/test-rate-limiting.sh

# Test for LaTeX injection vulnerability
./security-tests/test-latex-injection.sh
```

### Custom API URL
```bash
API_URL=http://192.168.1.100:8080 ./security-tests/run-all-tests.sh
```

## Expected Results

### Before Fixes (VULNERABLE)
All tests should **FAIL**, indicating vulnerabilities:

```
❌ Authentication Test - FAILED
   All endpoints accessible without credentials

❌ Rate Limiting Test - FAILED
   No rate limiting detected

❌ LaTeX Injection Test - FAILED
   Malicious LaTeX commands executed
```

### After Fixes (SECURE)
All tests should **PASS**, indicating proper security:

```
✅ Authentication Test - PASSED
   All endpoints require authentication

✅ Rate Limiting Test - PASSED
   Rate limiting active

✅ LaTeX Injection Test - PASSED
   LaTeX commands properly escaped
```

## Test Descriptions

### 1. Authentication Test (`test-authentication.sh`)
**Tests:** Whether API endpoints require authentication

**Endpoints checked:**
- `/api/events` - Speed detection events
- `/api/sites` - Site management
- `/api/radar/stats` - Radar statistics
- `/api/config` - System configuration

**Pass criteria:** All endpoints return HTTP 401 (Unauthorized)

### 2. Rate Limiting Test (`test-rate-limiting.sh`)
**Tests:** Whether API has rate limiting to prevent DoS

**Method:** Sends 20 rapid requests to `/api/config`

**Pass criteria:** Server returns HTTP 429 (Too Many Requests) after excessive requests

### 3. LaTeX Injection Test (`test-latex-injection.sh`)
**Tests:** Whether user input is properly escaped before LaTeX rendering

**Payloads tested:**
- `\input{/etc/passwd}` - File read attempt
- Special LaTeX commands in `location`, `surveyor`, `contact` fields

**Pass criteria:** 
- Request rejected OR
- Generated PDF contains escaped text (not file contents)

## Integration with CI/CD

Add to your CI/CD pipeline:

```yaml
# .github/workflows/security.yml
name: Security Tests

on: [push, pull_request]

jobs:
  security:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Start server
        run: |
          make dev-go &
          sleep 5
      
      - name: Run security tests
        run: ./security-tests/run-all-tests.sh
      
      - name: Upload results
        if: failure()
        uses: actions/upload-artifact@v3
        with:
          name: security-test-results
          path: security-tests/*.log
```

## Interpreting Results

### Critical Failures
If any test fails, the system has **launch-blocking vulnerabilities**:

1. **DO NOT deploy to production**
2. Review `LAUNCH_BLOCKERS.md` for fix instructions
3. Implement required fixes
4. Re-run tests until all pass
5. Only then approve for deployment

### All Tests Pass
System has basic security measures in place:

1. ✅ Authentication protects API
2. ✅ Rate limiting prevents DoS
3. ✅ Input sanitization prevents injection

**Note:** Passing these tests does NOT mean the system is completely secure. Regular security audits and penetration testing are still recommended.

## Adding New Tests

To add a new security test:

1. Create `test-<vulnerability>.sh` in this directory
2. Follow the pattern of existing tests
3. Add test to `run-all-tests.sh`
4. Document in this README

## Troubleshooting

### "Cannot connect to server"
Server is not running. Start it with:
```bash
make dev-go
```

### "Connection refused"
Wrong API URL. Override with:
```bash
API_URL=http://correct-host:8080 ./security-tests/run-all-tests.sh
```

### Tests hang or timeout
Server may be overloaded. Restart server and run tests individually:
```bash
make dev-go-kill-server
make dev-go
./security-tests/test-authentication.sh  # One at a time
```

## Related Documents

- `../LAUNCH_BLOCKERS.md` - Executive summary of critical vulnerabilities
- `../SECURITY_AUDIT_REPORT.md` - Detailed security audit findings
- `../README.md` - Main project documentation

## Security Contact

For security concerns, do not file public issues. Contact: [Add security contact]
