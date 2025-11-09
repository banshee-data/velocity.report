# Verification Guide: Event Subscription Preservation Across Reloads

## Quick Verification (5 minutes)

### Prerequisites

- Two serial port configurations in database (or ability to create test config)
- Access to `/api/serial/reload` endpoint
- Running instance with `--enable-lidar` or real serial data source

### Steps

1. **Start Server in Production Mode**

   ```bash
   ./app-radar-local
   ```

   Expected log output:

   ```
   Serial hot-reload available: /api/serial/reload endpoint enabled for production mode
   ```

2. **Verify Events Are Flowing**

   ```bash
   # Check that radar_readings are being inserted
   sqlite3 sensor_data.db "SELECT COUNT(*) FROM radar_readings WHERE timestamp > datetime('now', '-1 minute');"
   ```

   Should show increasing count.

3. **Create Second Serial Config (if needed)**

   ```bash
   curl -X POST http://localhost:8080/api/serial/configs \
     -H "Content-Type: application/json" \
     -d '{
       "name": "Test Config 2",
       "port_path": "/dev/ttySC2",
       "baud_rate": 19200,
       "data_bits": 8,
       "stop_bits": 1,
       "parity": "N"
     }'
   ```

4. **Monitor Event Processing Before Reload**

   ```bash
   # Terminal 1: Watch events
   watch -n 1 "sqlite3 sensor_data.db \"SELECT MAX(timestamp) FROM radar_readings;\""
   ```

   Should show updating timestamp.

5. **Trigger Reload**

   ```bash
   # Terminal 2: Execute reload
   curl -X POST http://localhost:8080/api/serial/reload
   ```

   Should see log:

   ```
   Event fanout: mux subscription closed, reconnecting on next iteration
   ```

6. **Verify Events Continue**
   Terminal 1 watch should show continuing timestamp updates.

   **Expected**: No interruption in event flow. Timestamp continues updating before and after reload.

7. **Check Server Logs for Graceful Reconnection**
   ```
   Event fanout: mux subscription closed, reconnecting on next iteration
   ```
   This confirms the fanout loop detected the reload and reconnected.

---

## Detailed Verification (15 minutes)

### Test 1: Events Don't Skip During Reload

```bash
# Get pre-reload count
BEFORE=$(sqlite3 sensor_data.db "SELECT COUNT(*) FROM radar_readings;")

# Wait for events
sleep 5

# Trigger reload
curl -X POST http://localhost:8080/api/serial/reload > /dev/null 2>&1

# Wait for recovery
sleep 2

# Get post-reload count
AFTER=$(sqlite3 sensor_data.db "SELECT COUNT(*) FROM radar_readings;")

# Check that counts increased continuously (no gap)
echo "Before reload: $BEFORE events"
echo "After reload: $AFTER events"
echo "Events added: $((AFTER - BEFORE))"
```

**Expected**: Events added > 5 (should have new events during and after reload)

### Test 2: Multiple Reloads Without Restart

```bash
# Reload 3 times
for i in 1 2 3; do
  echo "Reload $i..."
  curl -X POST http://localhost:8080/api/serial/reload
  sleep 2
  COUNT=$(sqlite3 sensor_data.db "SELECT COUNT(*) FROM radar_readings WHERE timestamp > datetime('now', '-5 seconds');")
  echo "Events in last 5s: $COUNT"
done
```

**Expected**: All three reloads succeed, events continue after each one

### Test 3: Verify Subscriber Channel Persistence

Check `cmd/radar/radar.go` subscriber loop still has valid channel:

```bash
# Trigger reload and check server doesn't crash
curl -X POST http://localhost:8080/api/serial/reload

# Server should still be responsive
curl http://localhost:8080/api/serial/configs

# Should get 200 OK response
```

**Expected**: API remains responsive before and after reload

### Test 4: Graceful Shutdown After Reload

```bash
# Trigger reload
curl -X POST http://localhost:8080/api/serial/reload
sleep 1

# Check logs show reconnection
grep "Event fanout" <server-logs>

# Graceful shutdown
kill -SIGTERM <pid>

# Should see clean shutdown
```

**Expected**: Logs show:

```
Event fanout loop terminated
subscribe routine terminated
```

---

## Log Analysis

### Expected Log Sequence

1. **Startup (Production Mode)**

   ```
   Serial hot-reload available: /api/serial/reload endpoint enabled for production mode
   initialised device ...
   ```

2. **Normal Operation** (no special logs)

   ```
   [radar data flowing, events being processed]
   ```

3. **During Reload**

   ```
   Event fanout: mux subscription closed, reconnecting on next iteration
   ```

4. **After Reload** (events resume immediately)

   ```
   [radar data flowing, events being processed]
   ```

5. **Graceful Shutdown**
   ```
   Event fanout loop terminated
   subscribe routine terminated
   monitor routine terminated
   ```

### Error Conditions (Should NOT See)

❌ **Do not see these logs during reload:**

- "subscribe routine: channel closed, exiting" (should only appear on shutdown)
- "failed to monitor serial port" (during reload)
- "error handling event: closed channel" (events should flow without error)

---

## Performance Verification

### Event Latency

Events should be processed with minimal latency during reload (~50ms max):

```bash
# Monitor event timestamps before/after reload
sqlite3 sensor_data.db "
  SELECT
    datetime(timestamp, 'localtime') as time,
    COUNT(*) as count
  FROM radar_readings
  WHERE timestamp > datetime('now', '-10 seconds')
  GROUP BY datetime(timestamp, 'localtime')
  ORDER BY timestamp DESC;"
```

**Expected**: No gaps or delays in event timestamps during reload

### Memory Usage

Should not increase significantly after reload:

```bash
# Before reload
ps aux | grep app-radar-local | grep -v grep

# Trigger reload
curl -X POST http://localhost:8080/api/serial/reload

# After reload - check RSS (resident set size)
ps aux | grep app-radar-local | grep -v grep
```

**Expected**: RSS stable (no memory leak from fanout loops)

---

## Troubleshooting

### Issue: Events Stop After Reload

**Check**: Server logs for errors

```bash
# Look for errors
tail -100 <server-logs> | grep -i "error\|fail\|close"
```

**Common causes**:

1. New serial port unavailable - check `/dev/` paths
2. Database locked - check for concurrent connections
3. Fanout loop crashed - should see "panic" in logs

### Issue: Reload Returns 503 Error

**Expected behavior** in debug/fixture modes:

```
curl -X POST http://localhost:8080/api/serial/reload
> HTTP 503
> {"success":false,"message":"Serial reload not available on this instance"}
```

**To get reload working**: Start in production mode without `--debug` or `--fixture` flags

### Issue: Events Intermittent After Reload

**Check**: Fanout subscriber count

```bash
# In code, add temporary logging:
log.Printf("Event fanout: %d subscribers receiving events", len(m.subscribers))
```

**Common causes**:

1. Subscriber channel buffer full - logs show "subscriber channel full, dropping event"
2. Event processing loop crashed - check for panics

---

## Regression Testing

Run full test suite to ensure no regressions:

```bash
make test-go
```

**Expected**: All tests pass, especially:

- `TestSerialPortManager*` tests
- `TestReloadConfig*` tests
- Event fanout integration tests

---

## Commit & Deployment Checklist

Before committing changes:

- [x] All tests pass (`make test-go`)
- [x] Code formatted (`make format-go`)
- [x] Linting clean (`make lint-go`)
- [x] Builds without warnings (`make build-radar-local`)
- [x] Manual reload test passes (events continue)
- [x] Multiple reloads work without restart
- [x] Graceful shutdown works
- [x] Documentation updated

---

## Related Documentation

- `RELOAD_EVENT_PRESERVATION.md` - Architecture and design details
- `RELOAD_FIX_SUMMARY.md` - High-level summary of changes
- `RELOAD_CODE_CHANGES.md` - Detailed code change breakdown
- `internal/api/serial_reload.go` - Implementation source
- `cmd/radar/radar.go` - Subscriber loop with reload resilience
