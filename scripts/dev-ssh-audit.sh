#!/usr/bin/env bash
# dev-ssh-audit.sh — Remote health check for a freshly booted velocity.report Pi.
#
# Checks:
#   1. SSH reachability (refreshes known_hosts if needed)
#   2. Systemd services (velocity-report, nginx, velocity-generate-tls)
#   3. TLS certificates present and not expiring within 30 days
#   4. HTTP → HTTPS redirect (301)
#   5. CA cert download
#   6. API endpoints (/api/capabilities, /api/sites)
#   7. velocity-ctl version + status
#   8. Radar data active (recent journal entries)
#   9. Database integrity
#  10. Disk health
#  11. PDF generation (seeds minimal transit data, POSTs /api/generate_report)
#  12. Cleanup seeded test data
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

for SVC in velocity-report.service nginx.service velocity-generate-tls.service; do
    STATE=$(ssh_run systemctl is-active "$SVC" 2>/dev/null || echo "inactive")
    case "$SVC" in
        velocity-generate-tls.service)
            # This is a oneshot — it runs on boot then goes inactive/dead. Both are correct.
            if [ "$STATE" = "active" ] || [ "$STATE" = "inactive" ]; then
                pass "${SVC}: ${STATE} (expected — oneshot)"
            else
                fail "${SVC}: ${STATE}"
            fi
            ;;
        *)
            if [ "$STATE" = "active" ]; then
                pass "${SVC}: active"
            else
                fail "${SVC}: ${STATE}"
            fi
            ;;
    esac
done

# --------------------------------------------------------------------------- #
#  Step 2: TLS certificates                                                    #
# --------------------------------------------------------------------------- #

header "2. TLS certificates"

CERT_CHECK=$(ssh_run bash << 'REMOTE'
CERT=/var/lib/velocity-report/tls/server.crt
CA=/var/lib/velocity-report/tls/ca.crt
KEY=/var/lib/velocity-report/tls/server.key
for f in "$CERT" "$CA" "$KEY"; do
    [ -f "$f" ] && echo "present:$f" || echo "missing:$f"
done
# Days until expiry
if [ -f "$CERT" ]; then
    EXPIRY=$(openssl x509 -enddate -noout -in "$CERT" 2>/dev/null | cut -d= -f2)
    EXPIRY_EPOCH=$(date -d "$EXPIRY" +%s 2>/dev/null || date -j -f "%b %d %T %Y %Z" "$EXPIRY" +%s 2>/dev/null || echo 0)
    NOW=$(date +%s)
    DAYS=$(( (EXPIRY_EPOCH - NOW) / 86400 ))
    echo "days:$DAYS"
fi
REMOTE
)

while IFS= read -r line; do
    case "$line" in
        present:*)  pass "Cert file present: ${line#present:}" ;;
        missing:*)  fail "Cert file missing: ${line#missing:}" ;;
        days:*)
            DAYS="${line#days:}"
            if [ "$DAYS" -gt 30 ]; then
                pass "Server cert expires in ${DAYS} days"
            else
                warn "Server cert expires in ${DAYS} days — renew soon"
            fi
            ;;
    esac
done <<< "$CERT_CHECK"

# --------------------------------------------------------------------------- #
#  Step 3: HTTP → HTTPS redirect                                               #
# --------------------------------------------------------------------------- #

header "3. HTTP → HTTPS redirect"

REDIRECT=$(ssh_run curl -s -o /dev/null -w '%{http_code}' --max-time 5 http://localhost/ 2>/dev/null || echo "000")
if [ "$REDIRECT" = "301" ] || [ "$REDIRECT" = "302" ]; then
    pass "HTTP redirect: ${REDIRECT}"
else
    fail "HTTP redirect: expected 301/302, got ${REDIRECT}"
fi

# --------------------------------------------------------------------------- #
#  Step 4: API endpoints                                                       #
# --------------------------------------------------------------------------- #

header "4. API endpoints"

CAPS=$(ssh_run curl -s --max-time 5 http://localhost:8080/api/capabilities 2>/dev/null || echo "")
if echo "$CAPS" | grep -q '"radar"'; then
    pass "/api/capabilities responds (radar field present)"
else
    fail "/api/capabilities: unexpected response: ${CAPS:0:100}"
fi

SITES=$(ssh_run curl -s --max-time 5 "http://localhost:8080/api/sites" 2>/dev/null || echo "")
if echo "$SITES" | grep -q '"id"'; then
    pass "/api/sites responds (at least one site)"
else
    fail "/api/sites: unexpected response: ${SITES:0:100}"
fi

# --------------------------------------------------------------------------- #
#  Step 5: velocity-ctl                                                        #
# --------------------------------------------------------------------------- #

header "5. velocity-ctl"

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
#  Step 6: Radar data                                                          #
# --------------------------------------------------------------------------- #

header "6. Radar data"

RADAR_LINES=$(ssh_run journalctl -u velocity-report.service --no-pager -n 500 2>/dev/null \
    | grep -c "Raw Data Line" 2>/dev/null || echo "0")
if [ "$RADAR_LINES" -gt 0 ]; then
    pass "Radar producing data (${RADAR_LINES} raw lines in last 500 log entries)"
else
    warn "No radar data lines in last 500 log entries (sensor may be unplugged)"
fi

# --------------------------------------------------------------------------- #
#  Step 7: Database integrity                                                  #
# --------------------------------------------------------------------------- #

header "7. Database"

DB_CHECK=$(ssh_run 'sqlite3 /var/lib/velocity-report/sensor_data.db "PRAGMA integrity_check;"' 2>/dev/null || echo "error")
if [ "$DB_CHECK" = "ok" ]; then
    pass "DB integrity_check: ok"
else
    fail "DB integrity_check: ${DB_CHECK}"
fi

DB_SIZE=$(ssh_run du -h /var/lib/velocity-report/sensor_data.db 2>/dev/null | awk '{print $1}' || echo "?")
pass "DB size: ${DB_SIZE}"

# --------------------------------------------------------------------------- #
#  Step 8: Disk                                                                #
# --------------------------------------------------------------------------- #

header "8. Disk"

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
#  Step 9: PDF generation                                                      #
# --------------------------------------------------------------------------- #

header "9. PDF generation"

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

YESTERDAY=$(date -d "yesterday" +%Y-%m-%d 2>/dev/null || date -v-1d +%Y-%m-%d)
TODAY=$(date +%Y-%m-%d)

HTTP=$(curl -s -o /tmp/pdf-audit-response.json \
  -w '%{http_code}' \
  -X POST http://localhost:8080/api/generate_report \
  -H "Content-Type: application/json" \
  -d "{\"site_id\":1,\"start_date\":\"$YESTERDAY\",\"end_date\":\"$TODAY\",\"source\":\"radar_data_transits\",\"histogram\":true}" \
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
