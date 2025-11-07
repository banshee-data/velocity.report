#!/bin/bash
# Security Test Suite Runner
# Runs all security tests and generates report

set -e

API_URL="${API_URL:-http://localhost:8080}"

echo "========================================"
echo "Security Test Suite"
echo "========================================"
echo ""
echo "Target: $API_URL"
echo "Date: $(date)"
echo ""

# Check if server is running
echo "[Pre-check] Testing server connectivity..."
if ! curl -s -o /dev/null -w "%{http_code}" "$API_URL/api/config" | grep -q "200\|401"; then
    echo "❌ ERROR: Cannot connect to server at $API_URL"
    echo "   Make sure the server is running first"
    echo ""
    echo "To start the server:"
    echo "   make dev-go"
    echo ""
    exit 1
fi
echo "✅ Server is reachable"
echo ""

# Run tests
FAILED_TESTS=()
PASSED_TESTS=()

run_test() {
    local test_name="$1"
    local test_script="$2"
    
    echo "========================================"
    echo "Running: $test_name"
    echo "========================================"
    
    if bash "$test_script"; then
        PASSED_TESTS+=("$test_name")
        return 0
    else
        FAILED_TESTS+=("$test_name")
        return 1
    fi
}

# Test 1: Authentication
run_test "Authentication Test" "security-tests/test-authentication.sh" || true
echo ""

# Test 2: Rate Limiting
run_test "Rate Limiting Test" "security-tests/test-rate-limiting.sh" || true
echo ""

# Test 3: LaTeX Injection (if server supports PDF generation)
run_test "LaTeX Injection Test" "security-tests/test-latex-injection.sh" || true
echo ""

# Summary
echo "========================================"
echo "SUMMARY"
echo "========================================"
echo ""
echo "Passed: ${#PASSED_TESTS[@]}"
for test in "${PASSED_TESTS[@]}"; do
    echo "  ✅ $test"
done
echo ""
echo "Failed: ${#FAILED_TESTS[@]}"
for test in "${FAILED_TESTS[@]}"; do
    echo "  ❌ $test"
done
echo ""

if [ ${#FAILED_TESTS[@]} -eq 0 ]; then
    echo "✅ All security tests passed!"
    echo "   System appears to be properly secured"
    exit 0
else
    echo "❌ CRITICAL: Security vulnerabilities detected!"
    echo ""
    echo "DO NOT DEPLOY TO PRODUCTION"
    echo ""
    echo "Next steps:"
    echo "  1. Review LAUNCH_BLOCKERS.md"
    echo "  2. Fix all critical vulnerabilities"
    echo "  3. Re-run this test suite"
    echo "  4. Only deploy after all tests pass"
    echo ""
    exit 1
fi
