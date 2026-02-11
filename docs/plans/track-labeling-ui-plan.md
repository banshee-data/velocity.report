# Unified Plan: Seekable Replay and Swift-Native Track Labeling

Unifies:

- `docs/plans/vrlog-recording-replay-plan.md` (VRLOG record/replay plumbing)
- `docs/plans/swift-macos-labeling-playback-alternatives.md` (option analysis)

into one implementation plan with one recommendation.

## Status

**Complete** — Option 2 implementation complete. Backend and Swift/macOS work done.

### Implementation Checklist

#### Go Backend (Complete)

- [x] **Phase 0: Canonical Label Contract** — Run-track labels via `PUT /api/lidar/runs/{run_id}/tracks/{track_id}/label`
- [x] **Phase 1.1: Publisher recording tap** — `FrameRecorder` interface in `publisher.go`
- [x] **Phase 1.2: Persist vrlog_path** — Migration 000022, `AnalysisRun.VRLogPath` field
- [x] **Phase 1.3: Recording callbacks** — `OnRecordingStart`/`OnRecordingStop` in `WebServerConfig`
- [x] **Phase 2.1: VRLOG replay in publisher** — `StartVRLogReplay`, `StopVRLogReplay`, `SeekVRLog`, etc.
- [x] **Phase 2.2: gRPC control delegation** — `vrlogMode`, `Pause`/`Play`/`Seek`/`SetRate` delegation
- [x] **Phase 3.1: Playback callbacks** — `OnPlaybackPause`, `OnPlaybackPlay`, `OnPlaybackSeek`, etc.
- [x] **Phase 3: Playback API** — REST endpoints: `/api/lidar/playback/*`, `/api/lidar/vrlog/*`

#### Swift/macOS (Complete)

- [x] **Phase 4.1: Run browser state** — `RunBrowserState` for listing and loading runs
- [x] **Phase 4.1: Run browser UI** — `RunBrowserView` with run list and replay loader
- [x] **Phase 4.2: RunTrackLabelAPIClient** — REST client for run-track labelling
- [x] **Phase 4.3: Connect labelling to run-track API** — Wire selection to `RunTrackLabelAPIClient`

#### Optional (Deferred)

- [ ] **Phase 5: Web parity** — Web playback controls for secondary fallback

---

## Goal

Enable operators to label tracks while viewing 3D evolution in the macOS Swift app, with reliable timeline scrub/seek and maintainable backend replay architecture.

---

## Options Compared

### Option 1: Existing baseline (VRLOG replay + web-driven timeline/labeling)

- Keep current direction from the original VRLOG plan.
- Web tracks page is primary timeline and labeling UI.
- Swift app mostly consumes streamed frames.

### Option 2: VRLOG-backed replay + Swift-native labeling (recommended)

- Keep VRLOG as canonical seekable replay format.
- Move primary labeling workflow into Swift app.
- Reuse backend run-track labeling APIs and replay controls.

### Option 3: Swift-native labeling + direct PCAP seek

- Skip VRLOG conversion/recording.
- Add true random seek over PCAP replay with state rebuild.

---

## Comparison Matrix

Scoring: `1 (worst)` to `5 (best)`.

| Option                                  | Maintainability | UX (label + scrub) | Performance | Notes                                                                                                                            |
| --------------------------------------- | --------------- | ------------------ | ----------- | -------------------------------------------------------------------------------------------------------------------------------- |
| 1. Baseline VRLOG + web-driven workflow | 3               | 2                  | 4           | Backend is straightforward, but operators context-switch between Swift 3D view and web labeling/timeline.                        |
| 2. VRLOG + Swift-native labeling        | 4               | 5                  | 4           | Best operator flow; robust seek semantics via VRLOG. Requires Swift-side API integration and run browser work.                   |
| 3. Direct PCAP seek                     | 2               | 3                  | 2           | High risk and complexity: seek must rebuild time-dependent state (background/tracker/warmup), likely high latency and fragility. |

## Single Recommendation

Pursue **Option 2: VRLOG-backed replay with Swift-native labeling**.

Why:

- **Maintainability**: one canonical seekable replay substrate (VRLOG).
- **Usability**: labeling happens where 3D behavior is inspected.
- **Performance**: avoids repeated random-access PCAP decode + state reconstruction during scrub.

Option 1 remains a migration path/fallback. Option 3 is explicitly deferred.

---

## Target Architecture (Recommended)

```
Recording during analysis replay:

  PCAP -> Pipeline -> FrameAdapter -> Publisher.Publish()
                                           |
                                      FrameRecorder.Record() -> .vrlog dir
                                           |
                                      frameChan -> broadcastLoop -> gRPC clients

Replay + labeling:

  Publisher.vrlogReplayLoop()
       |
       | reads FrameReader (Replayer), handles pause/rate/seek
       v
  Publisher.Publish() -> gRPC stream (Swift)
                           |
                           +-- Swift timeline scrub/step/rate (Seek/Play/Pause RPCs)
                           +-- Swift track labeling -> REST run-track label API

Optional parity:
  REST /api/lidar/playback/* + /api/lidar/vrlog/* for web controls/status
```

**Key design decision**: VRLOG replay must feed through `Publisher.Publish()` (shared broadcast path) rather than a single-client `streamFromReader` path.

---

## Implementation Plan

## Phase 0: Canonical Label Contract

1. Use run-track labels as canonical for analysis/tuning workflows:
   - `PUT /api/lidar/runs/{run_id}/tracks/{track_id}/label`
   - existing labelling progress endpoints for run-level QA.
2. Keep `/api/lidar/labels` as optional free-form event annotation.
3. Add explicit mapping rules only if both stores are needed in the same workflow.

## Phase 1: Record VRLOG During Analysis PCAP Replay

### 1.1 Publisher recording tap (`internal/lidar/visualiser/publisher.go`)

Problem: `recorder` imports `visualiser`; `publisher.go` cannot import `recorder` without a cycle.

Use interface indirection:

```go
type FrameRecorder interface {
    Record(frame *FrameBundle) error
}
```

Add:

- `recorder FrameRecorder` field
- `SetRecorder(rec FrameRecorder)`
- `ClearRecorder()`

Record in `Publish()` after frame type/background metadata assignment and before enqueue to `frameChan`.

### 1.2 Persist `vrlog_path` on runs

New migration:

- `internal/db/migrations/000023_add_vrlog_path.up.sql`
- `internal/db/migrations/000023_add_vrlog_path.down.sql`

`internal/lidar/analysis_run.go`:

- Add `VRLogPath string` with tag `json:"vrlog_path,omitempty"` to `AnalysisRun`.
- Update `InsertRun`, `GetRun`, `ListRuns`.
- Add `UpdateRunVRLogPath(runID, vrlogPath string) error`.

### 1.3 Wire record lifecycle in replay flow

`internal/lidar/monitor/webserver.go` in `WebServerConfig`:

```go
OnRecordingStart func(runID string)
OnRecordingStop  func(runID string) string
```

In PCAP analysis goroutine:

- call `OnRecordingStart` immediately after successful `StartRun()`
- call `OnRecordingStop` **before** `CompleteRun()`
- persist returned path with `UpdateRunVRLogPath`

`cmd/radar/radar.go`:

- hold active `*recorder.Recorder`
- start recorder under `<pcapDir>/vrlog/<runID>`
- set/clear recorder on publisher

## Phase 2: Seekable VRLOG Replay in Main Runtime

### 2.1 Replay control in publisher (`internal/lidar/visualiser/publisher.go`)

Add VRLOG replay state:

```go
vrlogReader        FrameReader
vrlogStopCh        chan struct{}
vrlogMu            sync.Mutex
vrlogPaused        bool
vrlogRate          float32
vrlogSeekSignal    chan struct{}
suppressBackground bool
```

Add lifecycle/control methods:

- `StartVRLogReplay(reader FrameReader) error`
- `StopVRLogReplay()`
- `IsVRLogActive() bool`
- `VRLogReader() FrameReader`
- `SetVRLogPaused(paused bool)`
- `SetVRLogRate(rate float32)`
- `SeekVRLog(frameIdx uint64) error`
- `SeekVRLogTimestamp(tsNs int64) error`

Replay loop requirements:

- read via `reader.ReadFrame()`
- pause polling (50ms), wake on seek signal
- rate control from frame timestamp deltas
- reset timing after seek
- publish frames through `p.Publish(frame)`
- stop on `io.EOF`, stop signal, or error

Suppress periodic background snapshots during VRLOG replay (`shouldSendBackground()` guard).

### 2.2 gRPC control delegation (`internal/lidar/visualiser/grpc_server.go`)

- add `vrlogMode bool`
- `SetVRLogMode(enabled bool)` should also set `replayMode`
- delegate `Pause()`, `Play()`, `SetRate()` to publisher in VRLOG mode
- implement `Seek()` by calling publisher seek methods (currently unimplemented)

## Phase 3: Playback API Surface and Orchestration

### 3.1 Backend playback callbacks (`internal/lidar/monitor/webserver.go`)

Add callbacks:

```go
OnPlaybackPause   func()
OnPlaybackPlay    func()
OnPlaybackSeek    func(timestampNs int64) error
OnPlaybackRate    func(rate float32)
OnVRLogLoad       func(vrlogPath string) error
OnVRLogStop       func()
GetPlaybackStatus func() PlaybackStatusInfo
```

Add status type:

```go
type PlaybackStatusInfo struct {
    Mode         string  `json:"mode"` // live|pcap|vrlog
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

Routes:

- `GET /api/lidar/playback/status`
- `POST /api/lidar/playback/pause`
- `POST /api/lidar/playback/play`
- `POST /api/lidar/playback/seek`
- `POST /api/lidar/playback/rate`
- `POST /api/lidar/vrlog/load` (input `run_id`, resolves stored `vrlog_path`)
- `POST /api/lidar/vrlog/stop`

`cmd/radar/radar.go` wires callbacks to server/publisher.

## Phase 4: Swift App as Primary Labeling UI

### 4.1 Replay loading in Swift

Use existing Swift playback controls (already implemented, currently gated by `isSeekable`):

- timeline scrub
- frame step forward/back
- pause/play/rate

Add run browser and replay loader:

- list runs
- detect `vrlog_path`
- load selected run replay
- surface run id in app state

### 4.2 Run-track labeling client in Swift

Add `RunTrackLabelAPIClient` for:

- list run tracks
- update run-track labels/quality
- update split/merge flags
- fetch labelling progress

Connect 3D selection to run-track labeling actions in side panel.

### 4.3 Keep existing `LabelAPIClient` scoped

- keep for free-form label events only
- avoid mixing stores for the same labeling workflow

## Phase 5: Web Parity (Optional / Secondary)

Keep or extend web playback controls for parity/fallback:

- `web/src/lib/types/lidar.ts`: playback status type + `vrlog_path` on run
- `web/src/lib/api.ts`: playback/vrlog functions
- `web/src/routes/lidar/tracks/+page.svelte`: optional backend playback sync
- `web/src/routes/lidar/runs/+page.svelte`: replay entry affordance

Web is secondary; Swift is primary labeling workflow.

---

## Deferred Work: Direct PCAP Seek

Do not pursue as primary path.

Only revisit after a dedicated spike proves:

- deterministic state rebuild after arbitrary seek,
- acceptable seek latency under production PCAP sizes,
- correctness parity with VRLOG replay.

---

## Ordering

```
Phase 0 (label contract)
    |
    +--> Phase 1 (recording + DB path persistence)
    |
    +--> Phase 2 (main-runtime VRLOG replay + gRPC seek)
    |
    +--> Phase 3 (REST orchestration/status)
    |
    +--> Phase 4 (Swift replay browser + run-track labeling)
    |
    +--> Phase 5 (optional web parity)
```

---

## Key Gotchas

1. `publisher.go` cannot import `recorder`; use `FrameRecorder` interface.
2. `analysis_run` store code is in `internal/lidar/analysis_run.go`, not a separate store file.
3. `OnRecordingStop` must execute before run completion finalization.
4. During VRLOG replay, suppress periodic background snapshots.
5. Ensure path validation on VRLOG load endpoints.
6. Reject VRLOG load while PCAP replay is active.
7. Keep one canonical labeling store per workflow to avoid data divergence.

---

## Files to Modify

| Area                    | File(s)                                                                                                                                  | Changes                                                              |
| ----------------------- | ---------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------- |
| Replay recording/replay | `internal/lidar/visualiser/publisher.go`                                                                                                 | FrameRecorder tap, VRLOG replay loop/state, seek/rate/pause controls |
| gRPC controls           | `internal/lidar/visualiser/grpc_server.go`                                                                                               | Implement seek delegation, VRLOG mode routing                        |
| Run persistence         | `internal/lidar/analysis_run.go`                                                                                                         | `vrlog_path` field + store updates                                   |
| DB migration            | `internal/db/migrations/000023_add_vrlog_path.*.sql`                                                                                     | Add/drop `vrlog_path`                                                |
| Orchestration API       | `internal/lidar/monitor/webserver.go`                                                                                                    | recording/playback callbacks + routes + status model                 |
| Wiring                  | `cmd/radar/radar.go`                                                                                                                     | recorder lifecycle + replay callback wiring                          |
| Swift app               | `tools/visualiser-macos/VelocityVisualiser/App/AppState.swift` and new API client(s)                                                     | run browser state, replay load flow, run-track labeling integration  |
| Optional web parity     | `web/src/lib/types/lidar.ts`, `web/src/lib/api.ts`, `web/src/routes/lidar/tracks/+page.svelte`, `web/src/routes/lidar/runs/+page.svelte` | playback status sync and replay controls                             |

---

## Verification

1. `go build ./...` passes.
2. `npm run build` passes.
3. Analysis PCAP replay produces VRLOG directory (`header.json`, `index.bin`, `frames/chunk_*.pb`).
4. Completed run row contains `vrlog_path`.
5. Loading replay by run id sets replay mode and `seekable=true`.
6. gRPC `Seek`/`Pause`/`Play`/`SetRate` work in main runtime.
7. Swift app can scrub/step and labels persist via run-track endpoints.
8. Labeling progress reflects Swift-authored labels through existing run APIs.

---

## Exit Criteria

- Operator can load a run in Swift, scrub timeline, step frames, and label tracks without leaving Swift.
- Replay seek is stable and deterministic.
- Run-track labels produced in Swift are visible to analysis/tuning workflows.
- Option 3 (direct PCAP seek) is not required for production workflow.
