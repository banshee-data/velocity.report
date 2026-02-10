# VRLOG Recording, Replay & Playback Sync

Record VRLOG files during analysis PCAP replay, enable VRLOG replay via gRPC (Mac app gets seekable playback), expose REST playback control so the Svelte timeline syncs with the Mac app.

## Status

**Draft** — not yet implemented.

---

## Architecture

```
Recording (during analysis PCAP replay):

  PCAP -> Pipeline -> FrameAdapter -> Publisher.Publish()
                                           |
                                      FrameRecorder.Record()  -> .vrlog directory
                                           |
                                      frameChan -> broadcastLoop -> gRPC clients

VRLOG Replay (later, on demand):

  Publisher.vrlogReplayLoop()
       |
       |  reads from FrameReader (Replayer)
       |  rate control + pause handling
       |
       v
  Publisher.Publish()  -> frameChan -> broadcastLoop -> gRPC clients
                                                            |
  REST /api/lidar/playback/* <--- Web UI polls status       |
  REST /api/lidar/vrlog/*    <--- Web UI loads/stops        |
                                                       Mac app (gRPC)
```

**Key design decision**: VRLOG replay feeds frames through `Publisher.Publish()` into the existing broadcast path. This means all gRPC clients receive frames (multi-client), and we reuse the existing broadcast infrastructure. The alternative (inline `streamFromReader` on Server, like `ReplayServer`) only supports single-client streaming.

---

## Step 1: Publisher — FrameRecorder Interface + Recording Tap

**Problem**: `recorder` package imports `visualiser` (for `*FrameBundle`), so `publisher.go` (in `visualiser`) cannot import `recorder`. Circular dependency.

**Fix**: Define interface in visualiser; `*recorder.Recorder` satisfies it via duck typing.

**File**: `internal/lidar/visualiser/publisher.go`

```go
// FrameRecorder allows recording frames without importing the recorder package.
type FrameRecorder interface {
    Record(frame *FrameBundle) error
}
```

Add field to `Publisher`:

```go
recorder   FrameRecorder  // nil when not recording
```

Add methods:

```go
func (p *Publisher) SetRecorder(rec FrameRecorder)
func (p *Publisher) ClearRecorder()
```

In `Publish()`, after FrameType/BackgroundSeq are set but before `frameChan` send:

```go
if rec := p.recorder; rec != nil {
    if err := rec.Record(frameBundle); err != nil {
        log.Printf("[Visualiser] Recording error: %v", err)
    }
}
```

**Note**: Recording happens after decimation/frame-type assignment, so VRLOG files contain foreground-only frames with FrameType already set. Replayed frames won't be re-decimated because of the `FrameType == 0` guard in `Publish()`.

---

## Step 2: DB Migration + AnalysisRun Field

**Note**: There is no `analysis_run_store.go`. All store code is in `analysis_run.go`.

### New files

**`internal/db/migrations/000022_add_vrlog_path.up.sql`**

```sql
ALTER TABLE lidar_analysis_runs ADD COLUMN vrlog_path TEXT;
```

**`internal/db/migrations/000022_add_vrlog_path.down.sql`**

```sql
ALTER TABLE lidar_analysis_runs DROP COLUMN vrlog_path;
```

### Modify `internal/lidar/analysis_run.go`

1. Add field to `AnalysisRun` struct (after `Notes`):

   ```go
   VRLogPath    string `json:"vrlog_path,omitempty"`
   ```

2. Update `InsertRun`: Add `vrlog_path` to INSERT columns + `nullString(run.VRLogPath)` to values (16 to 17 columns).

3. Update `GetRun`: Add `vrlog_path` to SELECT + scan as `sql.NullString`.

4. Update `ListRuns`: Same — add column + scan.

5. Add new method:
   ```go
   func (s *AnalysisRunStore) UpdateRunVRLogPath(runID, vrlogPath string) error
   ```

---

## Step 3: Wire Recording During Analysis PCAP Replay

### Modify `internal/lidar/monitor/webserver.go`

Add callbacks to `WebServerConfig`:

```go
OnRecordingStart func(runID string)          // Create recorder, set on Publisher
OnRecordingStop  func(runID string) string   // Clear recorder, close it, return vrlog path
```

In PCAP goroutine, after `StartRun()` succeeds:

```go
if ws.pcapAnalysisMode && runID != "" && ws.onRecordingStart != nil {
    ws.onRecordingStart(runID)
}
```

Before `CompleteRun()`, stop recording and store path:

```go
if ws.pcapAnalysisMode && runID != "" && ws.onRecordingStop != nil {
    vrlogPath := ws.onRecordingStop(runID)
    if vrlogPath != "" {
        store := lidar.NewAnalysisRunStore(ws.db.DB)
        store.UpdateRunVRLogPath(runID, vrlogPath)
    }
}
```

**Critical ordering**: `OnRecordingStop` must fire BEFORE `CompleteRun()` so the recorder is closed and the path is stored while the run is still active.

### Modify `cmd/radar/radar.go`

Add package-level `var activeRecorder *recorder.Recorder` and wire the callbacks:

- `OnRecordingStart`: Create `recorder.NewRecorder()` in `<pcapDir>/vrlog/<runID>`, call `publisher.SetRecorder(rec)`.
- `OnRecordingStop`: Call `publisher.ClearRecorder()`, close recorder, return `recorder.Path()`.

---

## Step 4: VRLOG Replay via Publisher Broadcast

### Modify `internal/lidar/visualiser/publisher.go`

Add fields to `Publisher`:

```go
vrlogReader        FrameReader   // Set during VRLOG replay
vrlogStopCh        chan struct{}  // Signals replay goroutine to stop
vrlogMu            sync.Mutex    // Protects vrlog state
vrlogPaused        bool
vrlogRate          float32
vrlogSeekSignal    chan struct{}  // Signals seek occurred (reset timing)
suppressBackground bool          // Skip background snapshots during VRLOG replay
```

`FrameReader` is already defined in `replay.go` in the same package.

Add lifecycle methods:

```go
func (p *Publisher) StartVRLogReplay(reader FrameReader) error
func (p *Publisher) StopVRLogReplay()
func (p *Publisher) IsVRLogActive() bool
func (p *Publisher) VRLogReader() FrameReader  // For status queries
```

Add playback control methods (called by Server RPCs):

```go
func (p *Publisher) SetVRLogPaused(paused bool)
func (p *Publisher) SetVRLogRate(rate float32)
func (p *Publisher) SeekVRLog(frameIdx uint64) error
func (p *Publisher) SeekVRLogTimestamp(tsNs int64) error
```

Add replay goroutine `vrlogReplayLoop()`:

- Reads frames via `reader.ReadFrame()`
- Handles pause (poll 50ms sleep loop, wake on seek signal for single-frame preview)
- Rate control using frame timestamp deltas divided by playback rate
- Resets timing after seek
- Publishes via `p.Publish(frame)`
- Exits on `io.EOF`, error, or stop signal

Modify `shouldSendBackground()` to return false when `suppressBackground` is set.

### Modify `internal/lidar/visualiser/grpc_server.go`

Add `vrlogMode bool` field to `Server`.

Add `SetVRLogMode(enabled bool)` method that sets both `vrlogMode` and `replayMode`.

Modify `Pause()`, `Play()`, `SetRate()` to delegate to Publisher when `vrlogMode`.

Implement `Seek()` (currently returns Unimplemented) to call `publisher.SeekVRLogTimestamp()` or `publisher.SeekVRLog()`.

---

## Step 5: REST Playback Endpoints

### Modify `internal/lidar/monitor/webserver.go`

Add callbacks to `WebServerConfig`:

```go
OnPlaybackPause   func()
OnPlaybackPlay    func()
OnPlaybackSeek    func(timestampNs int64) error
OnPlaybackRate    func(rate float32)
OnVRLogLoad       func(vrlogPath string) error
OnVRLogStop       func()
GetPlaybackStatus func() PlaybackStatusInfo
```

Add `PlaybackStatusInfo` type:

```go
type PlaybackStatusInfo struct {
    Mode         string  `json:"mode"`            // "live", "pcap", "vrlog"
    Paused       bool    `json:"paused"`
    Rate         float32 `json:"rate"`
    Seekable     bool    `json:"seekable"`
    CurrentFrame uint64  `json:"current_frame"`
    TotalFrames  uint64  `json:"total_frames"`
    TimestampNs  int64   `json:"timestamp_ns"`
    LogStartNs   int64   `json:"log_start_ns"`
    LogEndNs     int64   `json:"log_end_ns"`
    VRLogPath    string  `json:"vrlog_path,omitempty"`
}
```

Register routes:

| Method | Path                         | Body                      | Description                       |
| ------ | ---------------------------- | ------------------------- | --------------------------------- |
| GET    | `/api/lidar/playback/status` | —                         | Returns `PlaybackStatusInfo` JSON |
| POST   | `/api/lidar/playback/pause`  | —                         | Pause playback                    |
| POST   | `/api/lidar/playback/play`   | —                         | Resume playback                   |
| POST   | `/api/lidar/playback/seek`   | `{"timestamp_ns": int64}` | Seek to timestamp                 |
| POST   | `/api/lidar/playback/rate`   | `{"rate": float32}`       | Set playback rate                 |
| POST   | `/api/lidar/vrlog/load`      | `{"run_id": string}`      | Load VRLOG for run                |
| POST   | `/api/lidar/vrlog/stop`      | —                         | Stop VRLOG replay                 |

The `/vrlog/load` handler looks up the run's `vrlog_path` from the DB by `run_id`, validates the path, and calls `OnVRLogLoad`.

### Modify `cmd/radar/radar.go`

Wire the callbacks to the visualiser server/publisher.

---

## Step 6: Web UI API Functions

### Modify `web/src/lib/types/lidar.ts`

Add `PlaybackStatus` interface and `vrlog_path?: string` to `AnalysisRun`.

```typescript
export interface PlaybackStatus {
  mode: "live" | "pcap" | "vrlog";
  paused: boolean;
  rate: number;
  seekable: boolean;
  current_frame?: number;
  total_frames?: number;
  timestamp_ns?: number;
  log_start_ns?: number;
  log_end_ns?: number;
  vrlog_path?: string;
}
```

### Modify `web/src/lib/api.ts`

Add functions following existing fetch pattern:

```typescript
getPlaybackStatus(): Promise<PlaybackStatus>
playbackPause(): Promise<void>
playbackPlay(): Promise<void>
playbackSeek(timestampNs: number): Promise<void>
playbackSetRate(rate: number): Promise<void>
loadVRLog(runId: string): Promise<void>
stopVRLog(): Promise<void>
```

---

## Step 7: Web UI Timeline Sync

### Modify `web/src/routes/lidar/tracks/+page.svelte`

When selected run has `vrlog_path`, show a "Load Recording" button in the header controls area.

On click:

1. Call `loadVRLog(selectedRun.run_id)`
2. Start polling `getPlaybackStatus()` every 200ms
3. Sync `selectedTime` from `backendPlayback.timestamp_ns / 1e6`
4. Sync `isPlaying` from `!backendPlayback.paused`
5. Timeline seek/play/pause send REST commands to backend

On unload or run change:

1. Call `stopVRLog()`
2. Stop polling, revert to local playback mode

### Modify `web/src/routes/lidar/runs/+page.svelte`

In run detail panel:

- Show `vrlog_path` if present
- Add "Replay" button that navigates to tracks page with `?run_id=X&vrlog=1` to auto-load

---

## Implementation Order

```
Step 1 -----+
             +---> Step 3 (wire recording, depends on 1+2)
Step 2 -----+
                   Step 4 (VRLOG replay, depends on 1)
                      |
                      v
                   Step 5 (REST endpoints, depends on 4)
                      |
                      v
                   Step 6 (Web API, depends on 5)
                      |
                      v
                   Step 7 (Web timeline, depends on 6)
```

Steps 1 and 2 are independent. Step 4 only depends on Step 1 (FrameRecorder interface and FrameReader being available). Steps 3 and 4 are independent of each other.

---

## Key Gotchas

1. **Circular dep**: `publisher.go` CANNOT import `recorder`. Use `FrameRecorder` interface.
2. **No `analysis_run_store.go`**: All store code lives in `analysis_run.go`.
3. **Recording stops before CompleteRun**: `OnRecordingStop` must fire before `CompleteRun()` clears the run.
4. **Background suppression**: Set `suppressBackground = true` during VRLOG replay. VRLOG frames already have `FrameType` set, so they won't be re-decimated.
5. **PointCloud ref counting**: VRLOG frames from JSON deserialisation won't have pool-allocated slices. `Retain()`/`Release()` still work correctly — `Release()` at refCount=0 is a no-op for non-pooled slices.
6. **Path validation**: `/api/lidar/vrlog/load` must validate paths to prevent traversal.
7. **VRLOG load conflicts**: Reject VRLOG load if PCAP replay is in progress.

---

## Files to Modify

| File                                       | Changes                                                                                                            |
| ------------------------------------------ | ------------------------------------------------------------------------------------------------------------------ |
| `internal/lidar/visualiser/publisher.go`   | FrameRecorder interface, recorder field, VRLOG replay goroutine + lifecycle + playback control, suppressBackground |
| `internal/lidar/visualiser/grpc_server.go` | vrlogMode field, SetVRLogMode(), delegate Pause/Play/SetRate/Seek to Publisher                                     |
| `internal/lidar/analysis_run.go`           | VRLogPath field, update InsertRun/GetRun/ListRuns scans, add UpdateRunVRLogPath                                    |
| `internal/lidar/monitor/webserver.go`      | Recording callbacks, REST playback/vrlog endpoints, PlaybackStatusInfo type                                        |
| `cmd/radar/radar.go`                       | Wire recording + playback + vrlog callbacks, activeRecorder var                                                    |
| `internal/db/migrations/000022_*.sql`      | Add vrlog_path column (new files)                                                                                  |
| `web/src/lib/api.ts`                       | Playback + VRLOG API functions                                                                                     |
| `web/src/lib/types/lidar.ts`               | PlaybackStatus type, vrlog_path on AnalysisRun                                                                     |
| `web/src/routes/lidar/tracks/+page.svelte` | Load Recording button, backend playback sync polling                                                               |
| `web/src/routes/lidar/runs/+page.svelte`   | Show vrlog_path, Replay button                                                                                     |

---

## Verification

1. `go build ./...` passes
2. `npm run build` passes
3. Start analysis PCAP replay -> confirm VRLOG directory created with header.json, index.bin, chunks
4. Check analysis run in DB -> confirm vrlog_path is populated after completion
5. `POST /api/lidar/vrlog/load` with run_id -> Mac app receives seekable frames via gRPC
6. `GET /api/lidar/playback/status` -> shows mode=vrlog, current position, seekable=true
7. `POST /api/lidar/playback/seek` -> both Mac app and status endpoint reflect new position
8. Tracks page: select run with vrlog_path, click "Load Recording" -> timeline syncs
