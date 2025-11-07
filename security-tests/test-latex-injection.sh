#!/bin/bash
# Security Test: LaTeX Injection Vulnerability
# DO NOT RUN ON PRODUCTION SYSTEMS

set -e

API_URL="${API_URL:-http://localhost:8080}"
TEST_NAME="LaTeX Injection Security Test"

echo "========================================"
echo "$TEST_NAME"
echo "========================================"
echo ""
echo "Testing LaTeX injection vulnerability in PDF generator"
echo "API URL: $API_URL"
echo ""

# Test 1: File read via \input
echo "[Test 1] LaTeX \\input command injection"
echo "Payload: \\input{/etc/passwd}"
echo ""

RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$API_URL/api/sites/reports" \
  -H "Content-Type: application/json" \
  -d '{
    "location": "\\input{/etc/passwd}",
    "surveyor": "Security Test",
    "contact": "test@example.com",
    "start_date": "2024-01-01",
    "end_date": "2024-01-31",
    "source": "test",
    "timezone": "UTC",
    "units": "mph"
  }')

HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
BODY=$(echo "$RESPONSE" | sed '$d')

echo "HTTP Status: $HTTP_CODE"
echo "Response: $BODY"
echo ""

if [ "$HTTP_CODE" = "200" ]; then
    REPORT_ID=$(echo "$BODY" | grep -o '"report_id":[0-9]*' | cut -d':' -f2)
    echo "❌ VULNERABLE: Report generated with ID $REPORT_ID"
    echo "   Malicious LaTeX was NOT escaped"
    echo ""
    echo "To verify exploitation, download the PDF:"
    echo "   curl $API_URL/api/sites/reports/$REPORT_ID/download?file_type=pdf -o exploited.pdf"
    echo "   Search PDF for /etc/passwd contents"
    exit 1
else
    echo "✅ Request failed (expected if vulnerability is patched)"
    echo "   Or server is not running"
fi

echo ""
echo "========================================"
echo "Test complete"
echo "========================================"
