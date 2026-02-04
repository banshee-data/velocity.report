# M1 Visualiser Features - Mac Frontend Implementation Guide

## Status Overview

**Go Backend**: ✅ **COMPLETE** (91.9% test coverage)  
**Mac Frontend**: 🚧 **IN PROGRESS** (Swift implementation needed)

---

## Completed (Go Backend)

### Core Infrastructure
- ✅ `internal/lidar/visualiser/replay.go` - ReplayServer for streaming `.vrlog` files
- ✅ `internal/lidar/visualiser/recorder/recorder.go` - Record/replay with deterministic playback
- ✅ `internal/lidar/visualiser/grpc_server.go` - Pause/Play/Seek/SetRate RPCs
- ✅ `cmd/tools/replay-server` - Command-line tool for replaying recordings
- ✅ Comprehensive test suite (91.9% coverage)

### gRPC API Contracts (Fully Implemented)
- `Pause(PauseRequest) → PlaybackStatus`
- `Play(PlayRequest) → PlaybackStatus`
- `Seek(SeekRequest) → PlaybackStatus`
- `SetRate(SetRateRequest) → PlaybackStatus`
- `StreamFrames(StreamRequest) → stream FrameBundle`
- `GetCapabilities() → CapabilitiesResponse`

---

## Remaining Work (Mac Frontend)

### Track A: 3D Camera Controls

**Location**: `tools/visualiser-macos/VelocityVisualiser/Rendering/`

**Requirements**:

1. **Orbit (Rotate) Camera**
   - Implement arcball rotation around target point
   - Use quaternions for smooth rotation
   - Mouse drag: rotate around center of scene
   - Trackpad two-finger drag: rotate around center

2. **Pan (Translate) Camera**
   - Move camera parallel to view plane
   - Right mouse drag or Shift+left drag: pan
   - Trackpad two-finger scroll: pan

3. **Zoom (Dolly) Camera**
   - Move camera toward/away from target
   - Mouse scroll wheel: zoom in/out
   - Trackpad pinch gesture: zoom in/out
   - Maintain target point during zoom

4. **Keyboard Shortcuts**
   - `R` key: Reset camera to default view
   - Arrow keys: Nudge camera position
   - `+`/`-` keys: Zoom in/out

**Files to Modify**:
- `tools/visualiser-macos/VelocityVisualiser/Rendering/MetalRenderer.swift`
  - Add `viewMatrix` and `projectionMatrix` properties
  - Implement `updateCamera()` method
  - Add gesture recognizers
- `tools/visualiser-macos/VelocityVisualiser/App/AppState.swift`
  - Add camera state properties (position, target, up vector)
  - Add methods for camera manipulation

**Test Requirements**:
- Unit tests for camera matrix calculations
- Gesture handler tests
- Keyboard shortcut tests
- Target: 90%+ coverage for camera control code

---

### Track A: Playback Controls UI

**Location**: `tools/visualiser-macos/VelocityVisualiser/UI/`

**Requirements**:

1. **Playback Controls**
   - Play/Pause button (Space bar shortcut)
   - Frame step buttons (`,` for previous, `.` for next)
   - Rate adjustment buttons (`[` slower, `]` faster)
   - Display current rate (0.25x, 0.5x, 1.0x, 2.0x, 4.0x)

2. **Timeline Scrubber**
   - Horizontal slider showing playback position
   - Click to seek to specific frame
   - Drag to scrub through recording
   - Display current timestamp and frame ID
   - Show log start/end timestamps

3. **Playback Position Display**
   - Current frame: `Frame 42 / 1000`
   - Current time: `00:04.2 / 01:40.0`
   - Playback rate indicator: `⏩ 2.0x` or `⏸ Paused`
   - Progress bar (visual indicator)

**Files to Modify**:
- `tools/visualiser-macos/VelocityVisualiser/UI/ContentView.swift`
  - Add playback control panel
  - Add timeline scrubber component
  - Wire up keyboard shortcuts
- `tools/visualiser-macos/VelocityVisualiser/App/AppState.swift`
  - Implement `togglePlayPause()` (send `Pause`/`Play` RPC)
  - Implement `stepForward()` (send `Seek` RPC with +1 frame)
  - Implement `stepBackward()` (send `Seek` RPC with -1 frame)
  - Implement `increaseRate()` (send `SetRate` RPC)
  - Implement `decreaseRate()` (send `SetRate` RPC)
  - Implement `seek(to: Double)` (send `Seek` RPC with timestamp)
- `tools/visualiser-macos/VelocityVisualiser/gRPC/VisualiserClient.swift`
  - Add `pause()`, `play()`, `seek()`, `setRate()` methods
  - Handle RPC responses and update AppState

**Test Requirements**:
- Unit tests for playback control logic
- RPC call verification tests
- UI state update tests
- Target: 90%+ coverage for playback control code

---

## Testing Workflow

### Go Backend (Already Complete)

```bash
# Run tests with coverage
go test -cover ./internal/lidar/visualiser/...

# Expected output:
# ok  	github.com/banshee-data/velocity.report/internal/lidar/visualiser	2.3s	coverage: 91.9% of statements
# ok  	github.com/banshee-data/velocity.report/internal/lidar/visualiser/recorder	0.1s	coverage: 88.4% of statements
```

### Mac Frontend (To Be Implemented)

```bash
# From repository root
make test-mac

# Or with Xcode
xcodebuild test -project tools/visualiser-macos/VelocityVisualiser.xcodeproj \
                -scheme VelocityVisualiser

# Generate coverage report
make test-mac-cov
```

**Expected Coverage**: 90%+ for new camera and playback control code

---

## Integration Testing

### Record and Replay Test

1. **Record Synthetic Data**:
   ```bash
   # Start synthetic server
   go run ./cmd/tools/visualiser-server -rate 10 -points 5000 -tracks 10
   
   # TODO: Implement StartRecording RPC or use recorder programmatically
   # For now, recordings can be created manually in tests
   ```

2. **Replay Recording**:
   ```bash
   # Start replay server
   go run ./cmd/tools/replay-server -log /path/to/recording.vrlog
   
   # Connect Mac visualiser to localhost:50051
   open tools/visualiser-macos/build/Build/Products/Release/VelocityVisualiser.app
   ```

3. **Verify Playback Controls**:
   - Press Space to pause/play
   - Press `,` and `.` to step backward/forward
   - Press `[` and `]` to adjust playback rate
   - Click timeline scrubber to seek
   - Verify frame counter and timestamp display

4. **Verify Camera Controls**:
   - Drag with mouse to orbit camera
   - Scroll wheel to zoom
   - Right-click drag to pan
   - Press `R` to reset camera
   - Verify smooth camera movement

---

## Acceptance Criteria (M1 Milestone)

### Must Have (Blocking)
- ✅ Go backend: Pause/Play/Seek/SetRate RPCs (DONE)
- ✅ Go backend: 90%+ test coverage (91.9% DONE)
- ⏳ Swift: 3D camera controls (orbit, pan, zoom)
- ⏳ Swift: Mouse/trackpad gesture support
- ⏳ Swift: Playback controls UI (pause/play/seek/rate)
- ⏳ Swift: Timeline scrubber with timestamps
- ⏳ Swift: Frame stepping (previous/next)
- ⏳ Swift: 90%+ test coverage for new code

### Nice to Have (Optional)
- Recording UI in Mac visualiser
- Loop playback mode
- Bookmark specific frames
- Export frame screenshots

---

## Next Steps

1. **Implement Camera Controls** (Priority 1)
   - Start with basic orbit/pan/zoom
   - Add gesture recognizers
   - Add keyboard shortcuts
   - Write unit tests

2. **Implement Playback UI** (Priority 2)
   - Wire up RPC calls to AppState methods
   - Add timeline scrubber component
   - Add playback control panel
   - Write unit tests

3. **Integration Testing** (Priority 3)
   - Test with synthetic server
   - Test with replay server
   - Record test cases
   - Document known issues

4. **Documentation** (Priority 4)
   - Update `tools/visualiser-macos/README.md`
   - Add camera control usage guide
   - Add playback control usage guide
   - Update codecov badge when Swift CI is set up

---

## References

- **Design Docs**: `docs/lidar/visualiser/`
  - `01-problem-and-user-workflows.md` - User workflows
  - `02-api-contracts.md` - gRPC API specification
  - `03-architecture.md` - System architecture
  - `04-implementation-plan.md` - Milestone breakdown (this is M1)
- **Go Implementation**: `internal/lidar/visualiser/`
  - `replay.go` - ReplayServer implementation
  - `recorder/recorder.go` - Record/replay logic
  - `grpc_server.go` - gRPC service methods
- **Swift Skeleton**: `tools/visualiser-macos/VelocityVisualiser/`
  - `App/AppState.swift` - TODO: Wire up RPC calls
  - `Rendering/MetalRenderer.swift` - TODO: Add camera controls
  - `UI/ContentView.swift` - TODO: Add playback UI
- **Test Scripts**: 
  - `test-m1-replay.sh` - Integration test template

---

## Questions or Issues?

File an issue on GitHub or reach out on Discord: https://discord.gg/XXh6jXVFkt
