#!/usr/bin/env bash
# dev-ssh-audit.sh — Remote health check for a freshly booted velocity.report Pi.
#
# Checks:
#   1. SSH reachability (refreshes known_hosts if needed)
#   2. Systemd services (velocity-report)
#   3. HTTP service on :80 (serves 200; no nginx, no TLS termination layer)
#   4. API endpoints (/api/capabilities, /api/sites)
#   5. velocity-ctl version + status
#   6. Radar data active (recent journal entries)
#   7. Database integrity
#   8. Disk health
#   9. PDF generation (seeds minimal transit data, POSTs /api/generate_report)
#  10. Cleanup seeded test data
#
# Usage:
#   ./scripts/dev-ssh-audit.sh
#   make dev-ssh-audit

set -euo pipefail

HOST="velocity.local"
USER="velocity"
TARGET="${USER}@${HOST}"
PASS_COUNT=0
FAIL_COUNT=0
WARN_COUNT=0

# --------------------------------------------------------------------------- #
#  Helpers                                                                     #
# --------------------------------------------------------------------------- #

GREEN=$'\033[0;32m'
YELLOW=$'\033[0;33m'
RED=$'\033[0;31m'
BOLD=$'\033[1m'
RESET=$'\033[0m'

pass() { echo "${GREEN}✓${RESET} $*"; (( PASS_COUNT++ )) || true; }
fail() { echo "${RED}✗${RESET} $*"; (( FAIL_COUNT++ )) || true; }
warn() { echo "${YELLOW}~${RESET} $*"; (( WARN_COUNT++ )) || true; }
header() { echo ""; echo "${BOLD}$*${RESET}"; }

ssh_run() {
    # Run a command on the Pi. Suppress the SSH login banner.
    ssh -o BatchMode=yes \
        -o ConnectTimeout=10 \
        -o StrictHostKeyChecking=yes \
        -o LogLevel=ERROR \
        "$TARGET" "$@"
}

# --------------------------------------------------------------------------- #
#  Step 0: Known-hosts / reachability                                          #
# --------------------------------------------------------------------------- #

header "0. SSH reachability"

probe_ok() {
    ssh -o BatchMode=yes \
        -o ConnectTimeout=5 \
        -o StrictHostKeyChecking=yes \
        -o LogLevel=ERROR \
        "$TARGET" true 2>/dev/null
}

if ! probe_ok; then
    echo "   Host key mismatch or not yet known — refreshing known_hosts..."
    ssh-keygen -R "${HOST}" -f "${HOME}/.ssh/known_hosts" 2>/dev/null || true
    NEW_KEY=$(ssh-keyscan -T 15 "${HOST}" 2>/dev/null)
    if [ -z "$NEW_KEY" ]; then
        fail "Cannot reach ${HOST}. Is the Pi on the network?"
        exit 1
    fi
    mkdir -p "${HOME}/.ssh"
    chmod 700 "${HOME}/.ssh"
    echo "$NEW_KEY" >> "${HOME}/.ssh/known_hosts"
    echo "   Known-hosts entry refreshed."
fi

if probe_ok; then
    pass "SSH reachable: ${TARGET}"
else
    fail "SSH still unreachable after key refresh: ${TARGET}"
    exit 1
fi

# --------------------------------------------------------------------------- #
#  Step 1: Systemd services                                                    #
# --------------------------------------------------------------------------- #

header "1. Systemd services"

for SVC in velocity-report.service; do
    STATE=$(ssh_run systemctl is-active "$SVC" 2>/dev/null || echo "inactive")
    if [ "$STATE" = "active" ]; then
        pass "${SVC}: active"
    else
        fail "${SVC}: ${STATE}"
    fi
done

# --------------------------------------------------------------------------- #
#  Step 2: HTTP service on :80                                                 #
# --------------------------------------------------------------------------- #

header "2. HTTP service on :80"

# The Go server binds :80 directly (no nginx, no TLS). GET / 302-redirects to
# /app/, so probe the app entry point directly for a clean 200.
HTTP_CODE=$(ssh_run curl -s -o /dev/null -w '%{http_code}' --max-time 5 http://localhost/app/ 2>/dev/null || echo "000")
if [ "$HTTP_CODE" = "200" ]; then
    pass "HTTP on :80 serves /app/ (${HTTP_CODE})"
else
    fail "HTTP on :80 /app/: expected 200, got ${HTTP_CODE}"
fi

# --------------------------------------------------------------------------- #
#  Step 3: API endpoints                                                       #
# --------------------------------------------------------------------------- #

header "3. API endpoints"

CAPS=$(ssh_run curl -s --max-time 5 http://localhost/api/capabilities 2>/dev/null || echo "")
if echo "$CAPS" | grep -q '"radar"'; then
    pass "/api/capabilities responds (radar field present)"
else
    fail "/api/capabilities: unexpected response: ${CAPS:0:100}"
fi

SITES=$(ssh_run curl -s --max-time 5 "http://localhost/api/sites" 2>/dev/null || echo "")
if echo "$SITES" | grep -q '"id"'; then
    pass "/api/sites responds (at least one site)"
else
    fail "/api/sites: unexpected response: ${SITES:0:100}"
fi

# --------------------------------------------------------------------------- #
#  Step 4: velocity-ctl                                                        #
# --------------------------------------------------------------------------- #

header "4. velocity-ctl"

CTL_VER=$(ssh_run /usr/local/bin/velocity-ctl version 2>/dev/null || echo "")
if [ -n "$CTL_VER" ]; then
    pass "velocity-ctl version: ${CTL_VER}"
else
    fail "velocity-ctl version failed"
fi

CTL_STATUS=$(ssh_run sudo /usr/local/bin/velocity-ctl status 2>/dev/null | head -3 || echo "")
if echo "$CTL_STATUS" | grep -qiE "active|running"; then
    pass "velocity-ctl status: service running"
else
    warn "velocity-ctl status output: ${CTL_STATUS:0:120}"
fi

# --------------------------------------------------------------------------- #
#  Step 5: Radar data                                                          #
# --------------------------------------------------------------------------- #

header "5. Radar data"

RADAR_LINES=$(ssh_run journalctl -u velocity-report.service --no-pager -n 500 2>/dev/null \
    | grep -c "Raw Data Line" 2>/dev/null || echo "0")
if [ "$RADAR_LINES" -gt 0 ]; then
    pass "Radar producing data (${RADAR_LINES} raw lines in last 500 log entries)"
else
    warn "No radar data lines in last 500 log entries (sensor may be unplugged)"
fi

# --------------------------------------------------------------------------- #
#  Step 6: Database integrity                                                  #
# --------------------------------------------------------------------------- #

header "6. Database"

DB_CHECK=$(ssh_run 'sqlite3 /var/lib/velocity-report/sensor_data.db "PRAGMA integrity_check;"' 2>/dev/null || echo "error")
if [ "$DB_CHECK" = "ok" ]; then
    pass "DB integrity_check: ok"
else
    fail "DB integrity_check: ${DB_CHECK}"
fi

DB_SIZE=$(ssh_run du -h /var/lib/velocity-report/sensor_data.db 2>/dev/null | awk '{print $1}' || echo "?")
pass "DB size: ${DB_SIZE}"

# --------------------------------------------------------------------------- #
#  Step 7: Disk                                                                #
# --------------------------------------------------------------------------- #

header "7. Disk"

DISK_INFO=$(ssh_run df -h / 2>/dev/null | tail -1 || echo "")
DISK_PCT=$(echo "$DISK_INFO" | awk '{print $5}' | tr -d '%')
if [ -n "$DISK_PCT" ]; then
    if [ "$DISK_PCT" -lt 80 ]; then
        pass "Disk usage: ${DISK_PCT}% ($(echo "$DISK_INFO" | awk '{print $4}') free)"
    else
        warn "Disk usage high: ${DISK_PCT}%"
    fi
fi

# --------------------------------------------------------------------------- #
#  Step 8: PDF generation                                                      #
# --------------------------------------------------------------------------- #

header "8. PDF generation"

PDF_RESULT=$(ssh_run bash << 'REMOTE'
set -e

DB=/var/lib/velocity-report/sensor_data.db
NOW=$(date +%s)

# Seed minimal test transits (unique key prefix to avoid collision with real data)
sqlite3 "$DB" << SQL
INSERT OR IGNORE INTO radar_data_transits
  (transit_key, threshold_ms, transit_start_unix, transit_end_unix,
   transit_max_speed, transit_min_speed, point_count, model_version)
VALUES
  ('audit-t1', 500, $((NOW-7200)), $((NOW-7194)), 11.2, 9.1, 8, 'hourly-cron'),
  ('audit-t2', 500, $((NOW-3600)), $((NOW-3592)), 12.5, 10.3, 9, 'hourly-cron');
SQL

# Use UTC dates so seeded NOW-relative timestamps always fall within the window,
# regardless of the Pi's local timezone.
YESTERDAY=$(date -u -d "yesterday" +%Y-%m-%d 2>/dev/null || date -u -v-1d +%Y-%m-%d)
TODAY=$(date -u +%Y-%m-%d)

HTTP=$(curl -s -o /tmp/pdf-audit-response.json \
  -w '%{http_code}' \
  -X POST http://localhost/api/generate_report \
  -H "Content-Type: application/json" \
  -d "{\"site_id\":1,\"start_date\":\"$YESTERDAY\",\"end_date\":\"$TODAY\",\"timezone\":\"UTC\",\"source\":\"radar_data_transits\",\"histogram\":true}" \
  --max-time 120 2>/dev/null || echo "000")

echo "http:${HTTP}"
MSG=$(python3 -m json.tool /tmp/pdf-audit-response.json 2>/dev/null | grep '"message"\|"pdf_path"\|"error"' | head -2 || cat /tmp/pdf-audit-response.json 2>/dev/null | head -1)
echo "msg:${MSG}"

# Cleanup seeded records
sqlite3 "$DB" "DELETE FROM radar_data_transits WHERE transit_key LIKE 'audit-%';"
echo "cleanup:ok"
REMOTE
)

PDF_HTTP=$(echo "$PDF_RESULT" | grep "^http:" | cut -d: -f2)
PDF_MSG=$(echo "$PDF_RESULT" | grep "^msg:" | sed 's/^msg://')
PDF_CLEANUP=$(echo "$PDF_RESULT" | grep "^cleanup:")

if [ "$PDF_HTTP" = "200" ]; then
    pass "PDF generation: HTTP 200"
    [ -n "$PDF_MSG" ] && echo "   ${PDF_MSG}"
else
    fail "PDF generation: HTTP ${PDF_HTTP}"
    [ -n "$PDF_MSG" ] && echo "   ${PDF_MSG}"
fi

if echo "$PDF_CLEANUP" | grep -q "ok"; then
    pass "Seeded test data cleaned up"
else
    warn "Could not confirm cleanup of seeded test data"
fi

# --------------------------------------------------------------------------- #
#  Summary                                                                     #
# --------------------------------------------------------------------------- #

echo ""
echo "--------------------------------------"
echo "${BOLD}Audit summary${RESET}"
echo "  ${GREEN}Passed:  ${PASS_COUNT}${RESET}"
[ "$WARN_COUNT" -gt 0 ] && echo "  ${YELLOW}Warnings: ${WARN_COUNT}${RESET}"
[ "$FAIL_COUNT" -gt 0 ]  && echo "  ${RED}Failed:  ${FAIL_COUNT}${RESET}"
echo "--------------------------------------"

if [ "$FAIL_COUNT" -gt 0 ]; then
    exit 1
fi
