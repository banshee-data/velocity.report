# Development Log

## September 23, 2025 - Background diagnostics, monitor APIs & bg-sweep

- Centralized runtime diagnostics: added `internal/monitoring` logger and per-manager `EnableDiagnostics` flag.
- BackgroundManager helpers: `SetNoiseRelativeFraction`, `SetEnableDiagnostics`, `GetAcceptanceMetrics`, `ResetAcceptanceMetrics`, `GridStatus`, `ResetGrid` for safe runtime control.
- Monitor API additions: `GET/POST /api/lidar/params`, `GET /api/lidar/acceptance`, `POST /api/lidar/acceptance/reset`, `GET /api/lidar/grid_status`, `POST /api/lidar/grid_reset`.
- New CLI `cmd/bg-sweep`: incremental & settle modes, per-noise grid reset, live bucket discovery, per-bucket CSV expansion, fixed-point numeric formatting (no scientific notation), and `--output` CSV file support.
- Experimental note: acceptance rates are affected by in-memory grid state (TimesSeenCount, frozen cells); use `grid_reset` between steps for reproducible comparisons.

## September 22, 2025 - Background model fixes, snapshot export & backfill

- Wired the `BackgroundManager` into the LiDAR pipeline and made snapshots self-contained by
  persisting per-ring elevation angles (`ring_elevations_json`) with each `lidar_bg_snapshot`.
- Centralized snapshot-to-ASC export so exports prefer snapshot-embedded elevations (fallbacks: caller-supplied, then live manager) and added frame-export fallbacks that recompute Z from polar values when needed.
- Added a small CLI backfill tool to populate `ring_elevations_json` for existing snapshots (used embedded Pandar40P config to backfill many rows).
- Small algorithmic improvements to `ProcessFramePolar` to reduce outward drift:
  - restrict neighbor confirmation to same-ring neighbors (avoid cross-ring elevation leakage),
  - update spread EMA relative to the previous mean (reduces alpha-related bias).
- Added unit tests: export behavior (ensures exported Z is correct when elevations are available) and backfill DB tests; fixed concurrent SQLite update pattern (read candidates first, then write) to avoid SQLITE_BUSY.
- Cleaned up debugging prints and standardized CLI logging in the backfill tool; left data-export writes unchanged.

## September 21, 2025 - Server & SerialMux consolidation, DB unification, tests

- Centralized HTTP server and UI paths into `internal/api` (moved server code out of `cmd/radar`).
- Standardized on a single SQLite DB (`sensor_data.db`) and consolidated DB helpers in `internal/db`.
- LiDAR background snapshots persisted to the DB (insert/get snapshot API) and a manual HTTP persist trigger added for on-demand snapshots.
- Added `--disable-radar` flag and a robust `DisabledSerialMux` that no-ops serial I/O but deterministically closes subscriber channels on Unsubscribe/Close (fixes tight-loop log spam when running without hardware).
- Merged duplicate LiDAR webservers; canonical monitor now accepts an injected `*db.DB` and `SensorID` (wired from CLI) so the same DB is used everywhere.
- Moved radar event handlers into `internal/serialmux/handlers.go` and separated classification logic into `internal/serialmux/parse.go` (small, testable `ClassifyPayload`).
- Added unit tests for `serialmux` (DisabledSerialMux behavior, classification, config parsing, event handlers) and ensured the test suite passes.
- Removed several unnecessary import aliases and normalized imports across packages.

## September 20, 2025 - Snapshot & persistence improvements

- Hardened BackgroundGrid persistence: added RW-mutexes and copy-under-read so serial/frame processing isn't blocked during snapshot serialization; metadata updates now occur under write lock.
- Added DB access for snapshots and monitor inspection: implemented a GetLatestBgSnapshot helper and a monitor endpoint to fetch, gunzip/gob-decode and summarize stored background snapshots (includes sample cells and blob hex prefix).
- Moved manual persist endpoint into the lidar monitor webserver: the handler constructs a minimal BgSnapshot and invokes the BackgroundManager PersistCallback to persist on-demand.

## September 19, 2025 - BackgroundManager, snapshot plumbing, and polar processing

- Introduced a BackgroundManager registry and constructor (`NewBackgroundManager`) to own a sensor's `BackgroundGrid`, timers, and a persistence callback. Managers are discoverable via `GetBackgroundManager`/`RegisterBackgroundManager` so APIs can trigger on-demand snapshots.
- Added serialization for background snapshots (gob + gzip) and a `Persist` method that creates `BgSnapshot` records and writes them via a `BgStore` interface (implemented by the lidar DB). This wires up snapshot persistence into the codebase.
- Implemented `InsertBgSnapshot` in the lidar DB layer to persist `lidar_bg_snapshot` rows and return snapshot IDs.
- Expanded background test coverage (`more bg tests`) to exercise snapshotting behaviors and grid processing invariants.
- Processed polar frames into the BackgroundGrid (`ProcessFramePolar`): bin points by ring/azimuth, update EMA-based averages and spreads, apply neighbor-confirmation and freezing heuristics, and count changed cells for snapshot heuristics.

## September 18, 2025 - Polar-first refactor: parser & frame builder

- Centralized spherical→Cartesian math into a small `transform.go` helper and introduced `PointPolar`.
- Parser now emits `PointPolar` (polar-first) and the UDP listener forwards polar points directly when possible.
- Added `FrameBuilder.AddPointsPolar([]PointPolar)` and removed the legacy `AddPoints([]Point)` API; tests and integration updated.
- Result: `internal/lidar` tests (unit + PCAP integration) pass after the migration.

## September 17, 2025 - Background model & transform refactor

- Sensor-frame background model (ring × azimuth) for foreground masking.
- Two-level settling per cell (fast noise settling, slow parked-object settling).
- Persist BackgroundGrid snapshots and warm-start on load.
- Refactor spherical→Cartesian into a small transform helper and split polar/cartesian point types.
- World-grid (height-map / ground estimate) on masked Cartesian points for semantic ops.
- Next: add transform helpers, update point types, implement sensor-frame ProcessFrame and snapshot tests.

## September 13, 2025 - Test Code Maintainability & Optimization

### Parse Test Improvements

- **Eliminated implementation dependencies**: Replaced external constants (CHANNELS*PER_BLOCK, PACKET_SIZE*\*) with local test constants
- **Enhanced maintainability**: Tests now self-contained and independent of implementation changes
- **Fixed boundary conditions**: Corrected loop bounds in PCAP extraction to include valid edge cases
- **Removed redundant checks**: Eliminated unnecessary bounds checking in packet extraction logic
- **Performance optimization**: Streamlined extractUDPPayloads function by removing redundant conditional checks

### Technical Changes

- **Local test constants**: Added testChannelsPerBlock, testPacketSizeStandard, etc. for test isolation
- **Boundary fix**: Changed `i < len(data)-testPacketSizeStandard` to `i <= len(data)-testPacketSizeStandard`
- **Logic optimization**: Removed redundant `if i+testPacketSizeStandard <= len(data)` check
- **Code clarity**: Added explanatory comments for optimization decisions

## September 12, 2025 - Frame Builder Test Suite Fixes & Validation

### Test Suite Completion

- **All frame builder tests passing**: Fixed 3 previously failing tests using realistic production data patterns
- **Integration test relocation**: Moved PCAP integration test from `cmd/pcap-test/` to `internal/lidar/integration_test.go`
- **Test data organization**: Created `internal/lidar/testdata/` directory following Go conventions
- **Data volume upgrade**: Increased test point counts from ~10,680 to 60,000 points (matching successful PCAP integration test)
- **Production-level validation**: Tests now use MinFramePointsForCompletion = 10,000 threshold with realistic coverage
- **Time-based detection validation**: Confirmed hybrid detection with motor speed adaptation and azimuth wrap fallback
- **Configuration completeness**: Added BufferTimeout and CleanupInterval settings for proper async frame processing

### Fixed Test Cases

- **TestFrameBuilder_TraditionalAzimuthOnly**: Traditional azimuth-only detection (350° → 10°) with 60,000 points
- **TestFrameBuilder_HybridDetection**: Time-based detection with azimuth validation and realistic timing
- **TestFrameBuilder_AzimuthWrapWithTimeBased**: Azimuth wrap in time-based mode with proper configuration

### Test Pattern Analysis

- **Successful data patterns**: 0°-356° azimuth coverage with wrap at 356°→5° triggers completion
- **Timing validation**: ~60ms frame duration matches production expectations (600 RPM motor speed)
- **Point distribution**: Even azimuth distribution across 60,000 points provides adequate coverage
- **Configuration requirements**: BufferTimeout=100ms, CleanupInterval=50ms essential for async processing

## September 12, 2025 - Time-Based Frame Detection & Documentation

### Time-Based Frame Detection Implementation

- **Hybrid frame detection**: Time-based primary trigger with azimuth validation for anomaly prevention
- **Motor speed integration**: Real-time motor speed extraction from packet tail (bytes 8-9)
- **Frame timing adaptation**: Dynamic frame duration based on actual RPM (50ms at 1200 RPM, 100ms at 600 RPM)
- **Azimuth safety checks**: Requires 270° coverage before time-based completion to prevent timing glitches
- **Azimuth wrap secondary**: Respects azimuth wraps (340° → 20°) with minimum half-duration timing constraint
- **Traditional fallback**: Pure azimuth-based detection (350° → 10°) when time-based disabled
- **Motor speed caching**: Parser stores last motor speed for frame builder integration
- **Testing validation**: Confirmed proper frame duration changes during RPM transitions (600→1200→600)

### Code Documentation Enhancement

- **Comment verbosity upgrade**: Comprehensive documentation updates in extract.go
- **Packet structure details**: Complete 22-byte tail parsing documentation with all fields
- **Timestamp mode documentation**: Added detailed explanations for all 5 supported modes
- **Calibration explanations**: Enhanced comments for coordinate transformations and corrections
- **Performance optimization notes**: Documented trigonometric optimizations and memory allocations

### Technical Improvements

- **CLI configurability**: Added --sensor-name flag for flexible deployment scenarios
- **Real-time adaptation**: Frame builder now responds immediately to motor speed changes
- **Accurate timing**: Eliminated hardcoded 600 RPM assumption, uses actual motor speed throughout
- **UDP sequence validation**: Confirmed proper handling of optional 4-byte UDP sequence suffix

## September 11, 2025 - Memory Optimization & Frame Rate Fixes

### Packet Structure Analysis

- **Wireshark investigation**: Analyzed Hesai Pandar40P UDP packet structure
- **Discovered Ethernet tail issue**: Extra 4 bytes appended to UDP packets
- **Tail composition**: 2-byte sequence + 2-byte unknown data (0x00 0x00)
- **Parser fix**: Updated tail offset from last 6 bytes to last 10 bytes
- **Validation**: Confirmed correct UDP sequence extraction and point parsing

### Performance Validation

- **Proper frame characteristics**: ~69,000 points per frame, ~100ms duration
- **Correct LiDAR operation**: Full 360° rotations with expected Hesai Pandar40P output
- **Debug logging**: Added temporary logging to diagnose, then removed for production

### Technical Discoveries

- **Ethernet vs UDP parsing**: Raw UDP data includes Ethernet layer artifacts
- **Tail offset critical**: Incorrect offset leads to malformed sequence numbers
- Frame builder processes points individually, not in packets
- Individual UDP packets contain only 2-3 points with small azimuth ranges
- Azimuth wrap detection must account for accumulated vs instantaneous coverage
- Point-level frame detection requires stricter criteria than packet-level detection
