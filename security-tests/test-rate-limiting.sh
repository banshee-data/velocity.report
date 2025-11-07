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

for i in $(seq 1 $REQUESTS); do
    HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$API_URL/api/config")
    
    if [ "$HTTP_CODE" = "200" ]; then
        SUCCESS=$((SUCCESS + 1))
    elif [ "$HTTP_CODE" = "429" ]; then
        RATE_LIMITED=$((RATE_LIMITED + 1))
    else
        ERRORS=$((ERRORS + 1))
    fi
    
    # Visual progress
    if [ $((i % 5)) -eq 0 ]; then
        echo "  Progress: $i/$REQUESTS requests sent"
    fi
done

echo ""
echo "========================================"
echo "Results:"
echo "  Successful (200):     $SUCCESS"
echo "  Rate Limited (429):   $RATE_LIMITED"
echo "  Other errors:         $ERRORS"
echo "========================================"
echo ""

if [ $SUCCESS -eq $REQUESTS ]; then
    echo "❌ VULNERABLE: All $REQUESTS requests succeeded!"
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
    echo "   Server may not be running"
    echo ""
    exit 2
fi
