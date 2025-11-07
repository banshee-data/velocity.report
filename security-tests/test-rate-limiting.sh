#!/bin/bash
# Security Test: Rate Limiting
# Tests if API has rate limiting to prevent DoS

set -e

API_URL="${API_URL:-http://localhost:8080}"
TEST_NAME="Rate Limiting Security Test"
REQUESTS=20  # Number of requests to send
MAX_ALLOWED=10  # Reasonable limit before rate limiting should kick in

echo "========================================"
echo "$TEST_NAME"
echo "========================================"
echo ""
echo "Testing for missing rate limiting on API"
echo "API URL: $API_URL"
echo "Sending $REQUESTS rapid requests..."
echo ""

SUCCESS=0
RATE_LIMITED=0
ERRORS=0

echo "Sending requests in parallel batches..."

# Send requests in parallel to properly test rate limiting
for i in $(seq 1 $REQUESTS); do
    (
        HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" --max-time 5 --connect-timeout 2 "$API_URL/api/config" 2>&1)
        echo "$HTTP_CODE"
    ) &
    
    # Send in batches of 5 to avoid overwhelming the test system
    if [ $((i % 5)) -eq 0 ]; then
        wait
        echo "  Progress: $i/$REQUESTS requests sent"
    fi
done

# Wait for remaining requests
wait

# Collect results from background jobs
# Note: This is a simplified version. In production, you'd capture output properly
echo ""
echo "Testing with rapid sequential requests to detect rate limiting..."

for i in $(seq 1 10); do
    HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" --max-time 5 "$API_URL/api/config" 2>&1)
    
    if [ "$HTTP_CODE" = "200" ]; then
        SUCCESS=$((SUCCESS + 1))
    elif [ "$HTTP_CODE" = "429" ]; then
        RATE_LIMITED=$((RATE_LIMITED + 1))
    else
        ERRORS=$((ERRORS + 1))
    fi
    
    # No delay - test rapid sequential requests
done

echo ""
echo "========================================"
echo "Results:"
echo "  Successful (200):     $SUCCESS"
echo "  Rate Limited (429):   $RATE_LIMITED"
echo "  Other errors:         $ERRORS"
echo "========================================"
echo ""

if [ $SUCCESS -ge 8 ]; then
    echo "❌ VULNERABLE: Most requests succeeded without rate limiting!"
    echo "   No rate limiting detected"
    echo "   System is vulnerable to DoS attacks"
    echo ""
    echo "Impact:"
    echo "  - Attacker can flood API with requests"
    echo "  - System resources can be exhausted"
    echo "  - Expensive operations (PDF generation) can crash server"
    echo ""
    exit 1
elif [ $RATE_LIMITED -gt 0 ]; then
    echo "✅ PROTECTED: Rate limiting active"
    echo "   Blocked $RATE_LIMITED requests"
    echo ""
    exit 0
else
    echo "⚠️  UNKNOWN: Unexpected results"
    echo "   Server may not be running or errors occurred"
    echo ""
    exit 2
fi
