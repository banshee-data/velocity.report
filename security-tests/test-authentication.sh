#!/bin/bash
# Security Test: Authentication Bypass
# Verifies that API requires authentication

set -e

API_URL="${API_URL:-http://localhost:8080}"
TEST_NAME="Authentication Security Test"

echo "========================================"
echo "$TEST_NAME"
echo "========================================"
echo ""
echo "Testing for missing authentication on API endpoints"
echo "API URL: $API_URL"
echo ""

# Test sensitive endpoints that should require auth
ENDPOINTS=(
    "/api/events"
    "/api/sites"
    "/api/radar/stats"
    "/api/config"
)

VULNERABLE=0
PROTECTED=0

for endpoint in "${ENDPOINTS[@]}"; do
    echo "[Test] GET $endpoint (no credentials)"
    
    RESPONSE=$(curl -s -w "\n%{http_code}" "$API_URL$endpoint")
    HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
    
    if [ "$HTTP_CODE" = "200" ]; then
        echo "   ❌ VULNERABLE: Endpoint accessible without authentication"
        echo "   HTTP Status: $HTTP_CODE"
        VULNERABLE=$((VULNERABLE + 1))
    elif [ "$HTTP_CODE" = "401" ]; then
        echo "   ✅ PROTECTED: Authentication required"
        echo "   HTTP Status: $HTTP_CODE"
        PROTECTED=$((PROTECTED + 1))
    else
        echo "   ⚠️  UNKNOWN: Unexpected status $HTTP_CODE"
        echo "   (Server may not be running)"
    fi
    echo ""
done

echo "========================================"
echo "Results:"
echo "  Vulnerable endpoints: $VULNERABLE"
echo "  Protected endpoints:  $PROTECTED"
echo "========================================"

if [ $VULNERABLE -gt 0 ]; then
    echo ""
    echo "❌ CRITICAL: API has NO authentication!"
    echo "   Anyone can access sensitive data"
    echo ""
    exit 1
else
    echo ""
    echo "✅ All tested endpoints require authentication"
    echo ""
    exit 0
fi
