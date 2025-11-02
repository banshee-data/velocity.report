# Data Source Switching Implementation Plan

## Problem Statement

Currently, switching between live LiDAR data and PCAP replay requires restarting the server with the `--lidar-pcap-mode` flag. This is cumbersome during development and testing when frequently switching between data sources.

## Proposed Solution

Replace the CLI flag with a runtime-configurable data source that can be switched via HTTP API without server restart. When switching sources, the system will:

1. Stop current data ingestion (UDP listener or PCAP replay)
2. Reset the background grid to clear old data
3. Start the new data source (live or PCAP)

## Architecture Changes

### 1. New Data Source State Management

**File**: `internal/lidar/monitor/webserver.go`

Add data source state to `WebServer`:

```go
type DataSource string

const (
    DataSourceLive DataSource = "live"
    DataSourcePCAP DataSource = "pcap"
)

type WebServer struct {
    // ... existing fields ...

    // Data source state
    dataSourceMu      sync.RWMutex
    currentSource     DataSource
    currentPCAPFile   string  // Track current PCAP file for status endpoint
    udpListener       *network.UDPListener
    udpListenerCancel context.CancelFunc

    // PCAP state (keep existing)
    pcapMu            sync.Mutex
    pcapInProgress    bool
}
```

**Rationale**: Centralize data source management in WebServer since it already handles PCAP replay and has access to all necessary components.

### 2. New HTTP Endpoint

**Route**: `POST /api/lidar/data_source?source={live|pcap}`

**Request Body** (for PCAP source):

```json
{
  "pcap_file": "/path/to/file.pcap" // Required when source=pcap
}
```

**Response**:

```json
{
  "status": "success",
  "previous_source": "live",
  "current_source": "pcap",
  "message": "Data source switched to pcap, background grid reset"
}
```

**Error Cases**:

- 400: Invalid source type (not "live" or "pcap")
- 400: Missing pcap_file when source=pcap
- 404: PCAP file not found
- 409: **PCAP replay currently in progress - cannot switch until complete**
- 500: Failed to stop/start data source

### 3. Implementation Flow

**Handler Function**: `handleDataSourceSwitch(w http.ResponseWriter, r *http.Request)`

```
1. Parse query param: source (live|pcap)
2. Validate request (source type, pcap_file if needed)
3. Lock dataSourceMu (write lock)
4. Check if PCAP replay in progress:
   - If switching away from PCAP and pcapInProgress=true → return 409 Conflict
   - Client should wait and retry
5. Check if already on requested source → return early if no change needed
6. Stop current data source:
   - If live: cancel UDP listener context, wait for goroutine exit
   - If pcap: already stopped (replay finished or blocked by step 4)
7. Reset background grid via BackgroundManager.ResetBackground()
8. Start new data source:
   - If live: create new UDP listener with new context, start goroutine
   - If pcap: validate file, trigger ReadPCAPFile in goroutine
9. Update currentSource field
10. Unlock dataSourceMu
11. Return success response with previous/current source
```

### 4. UDP Listener Lifecycle Changes

**Current State**: UDP listener started once in `main()`, runs until program exit. Controlled by `--lidar-pcap-mode` flag.

**New State**: UDP listener lifecycle managed by WebServer, always starts in live mode.

**File**: `cmd/radar/radar.go`

**Changes** (BREAKING):

- **Remove** `lidarPCAPMode` CLI flag entirely
- Always pass UDP listener config to WebServer
- WebServer starts UDP listener on initialization (live mode default)
- WebServer manages stopping/starting UDP listener when switching sources
- Cleaner architecture: data source is runtime config, not startup flag

**Migration**: Users relying on `--lidar-pcap-mode` should instead:

```bash
# Old way (removed):
./radar --lidar-pcap-mode

# New way:
./radar  # starts in live mode
# Option A (two-step, recommended change):
# 1) Switch runtime data source to PCAP (mode switch only)
curl -X POST "http://localhost:8081/api/lidar/data_source?source=pcap"

# 2) Separately start PCAP replay (provide the file to replay). Keeping these
# as separate steps allows validation of the file and safer operations.
curl -X POST "http://localhost:8081/api/lidar/pcap/start" \
   -H "Content-Type: application/json" \
   -d '{"pcap_file": "/path/to/file.pcap"}'
```

### 5. WebServer Configuration Updates

**File**: `internal/lidar/monitor/webserver.go`

```go
type WebServerConfig struct {
    // ... existing fields ...

    // UDP listener configuration (WebServer will start it in live mode)
    UDPListenerConfig network.UDPListenerConfig
}

// Remove InitialDataSource - always starts in live mode
```

### 6. Status Endpoint Updates

**File**: `internal/lidar/monitor/webserver.go`

Update `/api/lidar/status` response to include data source information:

```go
type StatusResponse struct {
    // ... existing fields ...

    // New fields
    DataSource    string  `json:"data_source"`     // "live" or "pcap"
    PCAPFile      string  `json:"pcap_file,omitempty"`  // Current PCAP file if source=pcap
    PCAPInProgress bool   `json:"pcap_in_progress"` // Whether PCAP replay is currently running
}
```

This allows clients to:

- Query current data source
- Know if safe to switch (pcap_in_progress=false)
- See which PCAP file is being replayed

### 6. Grid Reset Integration

**File**: `internal/lidar/background.go`

Already has `ResetBackground()` method - use it during source switch.

**Ensure**:

- Reset is synchronous (blocks until complete)
- Reset clears all grid state (times_seen, averages, spreads)
- Reset logged with timestamp for debugging

### 7. Concurrency Safety

**Critical Sections**:

1. **Data source switching**: Write lock on `dataSourceMu` during transition
2. **PCAP replay**: Existing `pcapMu` protects concurrent PCAP starts
3. **Grid access**: BackgroundManager already thread-safe via internal mutex

**Race Conditions to Prevent**:

- Multiple simultaneous source switches
- Source switch during PCAP replay
- Grid access during reset

**Solution**:

```go
func (ws *WebServer) handleDataSourceSwitch(...) {
    ws.dataSourceMu.Lock()
    defer ws.dataSourceMu.Unlock()

    // Source switching logic here
    // This blocks other switches but allows reads via RWMutex
}
```

## API Design Considerations

### Option A: Single Combined Endpoint (Recommended)

```
POST /api/lidar/data_source?source={live|pcap}
Body: {"pcap_file": "..."} (only for pcap)
```

**Pros**:

- Single responsibility: manage data source
- Clear semantics: source is the primary parameter
- Easy to understand and document

**Cons**:

- Breaking change: replaces existing `/api/lidar/pcap/start`

### Option B: Keep Separate Endpoints

```
POST /api/lidar/data_source?source=live
POST /api/lidar/pcap/start (existing, modified to switch source)
```

**Pros**:

- Backward compatible with existing PCAP endpoint
- Gradual migration path

**Cons**:

- Two ways to do the same thing (confusing)
- PCAP endpoint needs to know about live source

**Recommendation**: Go with Option A, update documentation and scripts

## Migration Path

### Phase 1: Implement Core Functionality

1. Add `currentSource`, `currentPCAPFile` state to WebServer
2. Implement `handleDataSourceSwitch` endpoint with 409 blocking
3. Add UDP listener lifecycle management (start/stop)
4. Update status endpoint to expose data source state
5. Add tests for source switching

### Phase 2: Remove CLI Flag (BREAKING)

1. Remove `--lidar-pcap-mode` flag from `cmd/radar/radar.go`
2. Remove all PCAP mode conditionals from main()
3. WebServer always starts in live mode (UDP listener running)
4. Update build targets and documentation

### Phase 3: Update Tools & Scripts

1. Update `plot_grid_heatmap.py` to use new endpoint
2. Update sweep tools to switch source via API
3. Update Makefile targets:
   - `stats-live`: No changes needed (default mode)
   - `stats-pcap`: Call API to switch before running
   - `dev-go-pcap`: Remove this target (no longer needed)
4. Update helper scripts in `scripts/api/lidar/`
5. Add new script: `switch_data_source.sh`

### Phase 4: Documentation & Migration Guide

1. Update `lidar_sidecar_overview.md` to remove PCAP mode flag
2. Update `cmd/radar/README.md` with new workflow
3. Add migration guide for users of `--lidar-pcap-mode`
4. Update API documentation with new endpoint

## File Changes Summary

### New Files

- None (all changes to existing files)

### Modified Files

1. **`internal/lidar/monitor/webserver.go`** (~200 lines added)

   - Add data source state fields (including currentPCAPFile)
   - Add `handleDataSourceSwitch` handler with 409 blocking
   - Add UDP listener lifecycle management
   - Update `setupRoutes()` to register new endpoint
   - Modify `Start()` to initialize in live mode (start UDP listener)
   - Update status endpoint to include data_source, pcap_file, pcap_in_progress

2. **`cmd/radar/radar.go`** (~40 lines changed)

   - **REMOVE** `lidarPCAPMode` flag declaration
   - Remove conditional UDP listener startup logic
   - Always pass UDP listener config to WebServer
   - Remove PCAP mode references from main()
   - WebServer now manages UDP listener lifecycle

3. **`tools/grid-heatmap/plot_grid_heatmap.py`** (~20 lines changed)

   - Add `switch_data_source()` function
   - Call switch API before starting PCAP snapshots
   - Handle 409 responses (retry logic)

4. **`scripts/api/lidar/switch_data_source.sh`** (new helper script, ~20 lines)

   - Wrapper for new API endpoint
   - Usage: `./switch_data_source.sh live` or `./switch_data_source.sh pcap /path/to/file.pcap`

5. **`Makefile`** (~15 lines changed)

   - Remove `dev-go-pcap` target (no longer needed)
   - Update `stats-pcap` to switch source before plotting
   - Simplify - single server startup command for all modes

6. **Documentation Updates** (~100 lines changed)
   - `internal/lidar/docs/lidar_sidecar_overview.md` - remove PCAP mode flag references
   - `cmd/radar/README.md` - update with new API workflow
   - `scripts/api/README.md` - document new endpoint
   - Add migration guide for `--lidar-pcap-mode` users

## Testing Strategy

### Unit Tests

1. `TestDataSourceSwitch_LiveToPCAP` - verify clean transition
2. `TestDataSourceSwitch_PCAPToLive` - verify reverse transition
3. `TestDataSourceSwitch_Concurrent` - verify locking prevents races
4. `TestDataSourceSwitch_InvalidSource` - verify error handling
5. `TestDataSourceSwitch_MissingPCAPFile` - verify validation

### Integration Tests

1. Start server → switch to PCAP → verify grid resets → verify data ingestion
2. Start server → switch to live → verify UDP listener running → verify packets
3. Switch during active PCAP replay → verify blocking/error handling

### Manual Testing

1. Run server, switch between sources multiple times
2. Monitor grid reset timing
3. Verify no memory leaks from UDP goroutine cleanup
4. Test with sweep tools using new API

## Performance Considerations

### Grid Reset Cost

- **Current**: ~1-5ms for 72,000 cells (measured in existing code)
- **Impact**: Negligible for interactive switching
- **Mitigation**: Already asynchronous in background manager

### UDP Listener Restart

- **Cost**: Socket bind/unbind + goroutine creation
- **Time**: <10ms typically
- **Impact**: Minimal, acceptable for manual switches

### PCAP Transition

- **Cost**: Wait for current replay to finish (if in progress)
- **Mitigation**: Report 409 Conflict if PCAP running, suggest retry

## Decisions Made

1. **`--lidar-pcap-mode` flag: REMOVE** ✅

   - Server always starts in live mode by default
   - Use API to switch to PCAP mode as needed
   - Breaking change, but cleaner architecture

2. **Switching to PCAP automatically starts replay** ✅

   - Yes, if `pcap_file` provided in request body
   - Matches current behavior, intuitive UX

3. **Block switching during PCAP replay** ✅

   - Return 409 Conflict if PCAP currently running
   - Client should wait and retry
   - Prevents incomplete PCAP data issues

4. **Expose current source in `/api/lidar/status`** ✅

   - Add `data_source` field to status response
   - Include `pcap_file` if currently running PCAP
   - Enables clients to query current state

5. **Log source switches to database** ✅
   - Add to existing API timing logs for audit trail
   - Track source switches for debugging

## Benefits

1. **No server restarts** - dramatically improves dev workflow
2. **Consistent API** - single endpoint for data source management
3. **Clean transitions** - automatic grid reset prevents stale data
4. **Better testing** - easier to test both modes in same session
5. **Tool integration** - sweep tools can manage source internally

## Risks & Mitigations

| Risk                              | Impact                   | Mitigation                                          |
| --------------------------------- | ------------------------ | --------------------------------------------------- |
| Race condition during switch      | Data corruption          | Strict mutex locking, well-tested                   |
| UDP socket leak                   | Resource exhaustion      | Proper context cancellation, defer cleanup          |
| **Breaking change removes flag**  | **User workflows break** | **Clear migration guide, version notes**            |
| PCAP blocking (409) during switch | User confusion           | Clear error message, document retry pattern         |
| Grid reset timing                 | Lost recent data         | Document behavior, provide confirmation in response |

## Timeline Estimate

- **Phase 1** (Core Implementation): 5-7 hours

  - Data source state + status endpoint: 1.5 hours
  - API endpoint with 409 blocking: 2 hours
  - UDP lifecycle management: 2.5 hours
  - Testing: 1 hour

- **Phase 2** (Remove CLI Flag): 2-3 hours

  - Remove flag from cmd/radar: 1 hour
  - Update conditionals: 1 hour
  - Testing: 1 hour

- **Phase 3** (Tool Integration): 2-3 hours

  - Update tools: 1 hour
  - Update scripts: 1 hour
  - Makefile changes: 1 hour

- **Phase 4** (Documentation): 2-3 hours
  - Migration guide: 1 hour
  - API docs: 1 hour
  - Update existing docs: 1 hour

**Total**: 11-16 hours

## Success Criteria

1. ✅ Can switch from live to PCAP without restart
2. ✅ Can switch from PCAP to live without restart
3. ✅ Grid automatically resets on switch
4. ✅ No UDP socket leaks after multiple switches
5. ✅ Switching blocked (409) during active PCAP replay
6. ✅ Status endpoint shows current data source and PCAP state
7. ✅ `--lidar-pcap-mode` flag completely removed
8. ✅ All existing tools continue to work with updates
9. ✅ Makefile targets simplified (single server mode)
10. ✅ Migration guide available for users
11. ✅ Documentation reflects new workflow

## Next Steps

1. ✅ **Reviewed & Decided** - Decisions confirmed:
   - Remove `--lidar-pcap-mode` flag
   - Block switching during PCAP (409 response)
   - Expose data source in status endpoint
2. **Implement** Phase 1 (core functionality + status endpoint)
3. **Implement** Phase 2 (remove CLI flag)
4. **Test** thoroughly with existing tools
5. **Update** tools, scripts, and Makefile
6. **Write** migration guide
7. **Update** documentation
8. **Deploy** and monitor for issues

---

**Created**: 2025-11-01
**Updated**: 2025-11-01 (decisions finalized)
**Author**: Development Team
**Status**: Ready for Implementation ✅
